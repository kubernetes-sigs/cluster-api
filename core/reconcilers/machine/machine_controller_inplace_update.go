/*
Copyright 2025 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package machine

import (
	"context"
	"fmt"
	"strings"
	"time"

	pkgerrors "github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/hooks"
	"sigs.k8s.io/cluster-api/internal/util/inplace"
	"sigs.k8s.io/cluster-api/util/cache"
)

// reconcileInPlaceUpdate handles the in-place update workflow for a Machine.
func (r *Reconciler) reconcileInPlaceUpdate(ctx context.Context, s *scope) (ctrl.Result, error) {
	if !feature.Gates.Enabled(feature.InPlaceUpdates) {
		return ctrl.Result{}, nil
	}

	log := ctrl.LoggerFrom(ctx)

	machineAnnotations := s.machine.GetAnnotations()
	_, inPlaceUpdateInProgress := machineAnnotations[clusterv1.UpdateInProgressAnnotation]
	hasUpdateMachinePending := hooks.IsPending(runtimehooksv1.UpdateMachine, s.machine)

	if !inPlaceUpdateInProgress {
		// Clean up any orphaned pending hooks and annotations before exiting.
		// This can happen if the in-place update annotation was removed from Machine
		// but the UpdateMachine hook is still pending or annotations are still on InfraMachine/BootstrapConfig.
		if hasUpdateMachinePending {
			log.Info("In-place update annotation removed but UpdateMachine hook still pending, cleaning up orphaned hook and annotations")
			if err := r.completeInPlaceUpdate(ctx, s); err != nil {
				return ctrl.Result{}, pkgerrors.Wrap(err, "failed to clean up orphaned UpdateMachine hook and annotations")
			}
		}

		return ctrl.Result{}, nil
	}

	// If hook is not pending, we're waiting for the owner controller to mark it as pending.
	if !hasUpdateMachinePending {
		log.Info("Machine marked for in-place update, waiting for owning controller to mark UpdateMachine hook as pending")
		return ctrl.Result{}, nil
	}

	if !ptr.Deref(s.machine.Status.Initialization.InfrastructureProvisioned, false) {
		log.V(5).Info("Infrastructure not yet provisioned, skipping in-place update")
		return ctrl.Result{}, nil
	}
	if !ptr.Deref(s.machine.Status.Initialization.BootstrapDataSecretCreated, false) {
		log.V(5).Info("Bootstrap data secret not yet created, skipping in-place update")
		return ctrl.Result{}, nil
	}

	if !s.machine.Status.NodeRef.IsDefined() {
		log.V(5).Info("Machine status.nodeRef is not yet set, skipping in-place update")
		return ctrl.Result{}, nil
	}

	if s.infraMachine == nil {
		s.updatingReason = clusterv1.MachineInPlaceUpdateInternalErrorReason
		s.updatingMessage = "In-place update not possible: InfraMachine not found"
		return ctrl.Result{}, pkgerrors.New("in-place update failed: InfraMachine not found")
	}

	infraReady := r.isInfraMachineReadyForUpdate(s)
	bootstrapReady := r.isBootstrapConfigReadyForUpdate(s)

	if !infraReady || !bootstrapReady {
		log.Info("Waiting for InfraMachine and BootstrapConfig to be marked for in-place update")
		return ctrl.Result{}, nil
	}

	// Note: updateMachine sets updatingReason and updatingMessage on the scope.
	updateMachineResult, err := r.updateMachine(ctx, s)
	if err != nil {
		return ctrl.Result{}, pkgerrors.Wrap(err, "in-place update failed")
	}
	if updateMachineResult.retryAfterFromExtension {
		// Note: retryAfter can already be in the past (e.g. if a cached result expired in the meantime), a RequeueAfter <= 0 would not requeue at all.
		return ctrl.Result{RequeueAfter: max(updateMachineResult.retryAfter.Sub(r.now()), time.Second)}, nil
	}

	if err := r.completeInPlaceUpdate(ctx, s); err != nil {
		return ctrl.Result{}, pkgerrors.Wrap(err, "failed to complete in-place update")
	}

	return ctrl.Result{}, nil
}

// isInfraMachineReadyForUpdate checks if the InfraMachine has the in-place update annotation.
func (r *Reconciler) isInfraMachineReadyForUpdate(s *scope) bool {
	_, hasAnnotation := s.infraMachine.GetAnnotations()[clusterv1.UpdateInProgressAnnotation]
	return hasAnnotation
}

// isBootstrapConfigReadyForUpdate checks if the BootstrapConfig has the in-place update annotation.
func (r *Reconciler) isBootstrapConfigReadyForUpdate(s *scope) bool {
	if s.bootstrapConfig == nil {
		return true
	}
	_, hasAnnotation := s.bootstrapConfig.GetAnnotations()[clusterv1.UpdateInProgressAnnotation]
	return hasAnnotation
}

// updateMachineCacheDuration is the duration for which UpdateMachine results are cached
// (if the extension did not return retryAfterSeconds).
const updateMachineCacheDuration = 5 * time.Minute

// UpdateMachineCacheEntry is an entry for the UpdateMachine hook cache.
type UpdateMachineCacheEntry struct {
	// Machine is the machine for which we cache the UpdateMachine result.
	Machine client.ObjectKey

	// updateMachineResult is the cached UpdateMachine result.
	updateMachineResult
}

// Key returns the cache key of a UpdateMachineCacheEntry.
func (r UpdateMachineCacheEntry) Key() string {
	return r.Machine.String()
}

type updateMachineResult struct {
	// retryAfter is the time at which the result expires and we should reconcile again (and call UpdateMachine extensions again).
	// Note: This is either the retry requested by an extension via retryAfterSeconds (see retryAfterFromExtension)
	// or the time at which the cached result expires.
	retryAfter time.Time

	// retryAfterFromExtension is true if an extension returned retryAfterSeconds, i.e. the in-place update is still in progress.
	// Note: This is needed to differentiate between retryAfterSeconds>0 and other cases.
	retryAfterFromExtension bool

	// messages are the messages from the UpdateMachine responses.
	messages []string
}

// updateMachine calls the UpdateMachine runtime hook for the machine.
func (r *Reconciler) updateMachine(ctx context.Context, s *scope) (updateMachineResult, error) {
	log := ctrl.LoggerFrom(ctx)

	// Validate that exactly one extension is registered for the UpdateMachine hook.
	// For the current iteration, we only support a single extension to ensure safe behavior.
	// Support for multiple extensions will be introduced in a future iteration.
	extensionHandlers, err := r.RuntimeClient.GetAllExtensions(ctx, runtimehooksv1.UpdateMachine, s.machine)
	if err != nil {
		s.updatingReason = clusterv1.MachineInPlaceUpdateInternalErrorReason
		s.updatingMessage = fmt.Sprintf("UpdateMachine hook failed: %s", err.Error())
		return updateMachineResult{}, err
	}
	if len(extensionHandlers) == 0 {
		err := pkgerrors.New("no extensions registered for UpdateMachine hook")
		s.updatingReason = clusterv1.MachineInPlaceUpdateInternalErrorReason
		s.updatingMessage = fmt.Sprintf("UpdateMachine hook failed: %s", err.Error())
		return updateMachineResult{}, err
	}
	if len(extensionHandlers) > 1 {
		err := pkgerrors.Errorf("found multiple UpdateMachine hooks (%s): only one hook is supported", strings.Join(extensionHandlers, ","))
		s.updatingReason = clusterv1.MachineInPlaceUpdateInternalErrorReason
		s.updatingMessage = fmt.Sprintf("UpdateMachine hook failed: %s", err.Error())
		return updateMachineResult{}, err
	}

	var res updateMachineResult
	logSuffix := ""
	if entry := tryToGetUpdateMachineResultFromCache(r.now(), r.updateMachineCache, s.machine); entry != nil {
		// Get result from cache.
		res = entry.updateMachineResult

		// Refresh the cache entry, so only tryToGetUpdateMachineResultFromCache decides when a cache entry should not be used anymore.
		r.updateMachineCache.Add(*entry)

		// Note: Cached results are only logged on log level 5.
		log = log.V(5)
		logSuffix = " (cached)"
	} else {
		// Get result from extensions.
		var err error
		res, err = r.extensionsUpdateMachine(ctx, s, extensionHandlers)
		if err != nil {
			s.updatingReason = clusterv1.MachineInPlaceUpdateInternalErrorReason
			s.updatingMessage = "UpdateMachine hook failed: please check controller logs for errors"
			return updateMachineResult{}, err
		}

		// Note: The cache entry is deleted at the end of completeInPlaceUpdate, otherwise a later in-place update
		// of the same Machine could be skipped because we would use the cached result instead of calling the
		// UpdateMachine extension.
		r.updateMachineCache.Add(UpdateMachineCacheEntry{
			Machine:             client.ObjectKeyFromObject(s.machine),
			updateMachineResult: res,
		})
	}

	if len(res.messages) > 0 {
		log = log.WithValues("messages", strings.Join(res.messages, ","))
	}
	switch {
	case res.retryAfterFromExtension:
		s.updatingReason = clusterv1.MachineInPlaceUpdatingReason
		if inplace.IsUpdateInProgressAndAffectsAvailability(s.machine) {
			s.updatingMessage = "In-place update in progress (affects availability)"
		} else {
			s.updatingMessage = "In-place update in progress (does not affect availability)"
		}
		if len(res.messages) > 0 {
			s.updatingMessage += fmt.Sprintf(": %s", strings.Join(res.messages, ", "))
		}
		log.Info(fmt.Sprintf("Machine %s is still updating in-place: extensions returned retryAfterSeconds, requeuing%s", klog.KObj(s.machine), logSuffix))
	default:
		log.Info(fmt.Sprintf("Machine %s has been updated in-place by extensions%s", klog.KObj(s.machine), logSuffix))
	}
	return res, nil
}

func tryToGetUpdateMachineResultFromCache(now time.Time, cache cache.Cache[UpdateMachineCacheEntry], machine *clusterv1.Machine) *UpdateMachineCacheEntry {
	entry, ok := cache.Has(UpdateMachineCacheEntry{
		Machine: client.ObjectKeyFromObject(machine),
	}.Key())
	// If cache entry does not exist, don't use the cache.
	if !ok {
		return nil
	}

	// If retryAfter is in the past, don't use the cache.
	if !entry.retryAfter.After(now) {
		return nil
	}

	// If retryAfter is in the future, use the cache.
	return &entry
}

func (r *Reconciler) extensionsUpdateMachine(ctx context.Context, s *scope, extensionHandlers []string) (updateMachineResult, error) {
	// Note: We are not caching the full InfraMachine object, only the contract InfraMachine in the dynamicCache.
	// During in-place update we need the full object so we get it here and cache it temporarily.
	infraMachineCacheEntry, ok := r.externalObjectsCache.Has(externalObjectsCacheKey(s.infraMachineGVK, s.infraMachine))
	if !ok || s.infraMachine.GetResourceVersion() != infraMachineCacheEntry.obj.GetResourceVersion() {
		var err error
		infraMachineCacheEntry.obj, err = external.GetObjectFromContractVersionedRef(ctx, r.APIReader, s.machine.Spec.InfrastructureRef, s.machine.Namespace,
			&client.GetOptions{Raw: &metav1.GetOptions{ResourceVersion: s.infraMachine.GetResourceVersion()}})
		if err != nil {
			return updateMachineResult{}, pkgerrors.Wrapf(err, "failed to generate UpdateMachine request")
		}
	}
	r.externalObjectsCache.Add(infraMachineCacheEntry) // Add to cache or extend expiry of cache entry on cache hit.

	// Note: When building request message, dropping status; Runtime extension should treat UpdateMachine
	// requests as desired state; it is up to them to compare with current state and perform necessary actions.
	req := &runtimehooksv1.UpdateMachineRequest{
		Desired: runtimehooksv1.UpdateMachineRequestObjects{
			Machine:               *cleanupMachine(s.machine),
			InfrastructureMachine: runtime.RawExtension{Object: cleanupUnstructured(infraMachineCacheEntry.obj)},
		},
	}

	var bootstrapConfigCacheEntry externalObjectsCacheEntry
	if s.bootstrapConfig != nil {
		// Note: We are not caching the full BootstrapConfig object, only the contract BootstrapConfig in the dynamicCache.
		// During in-place update we need the full object so we get it here and cache it temporarily.
		bootstrapConfigCacheEntry, ok = r.externalObjectsCache.Has(externalObjectsCacheKey(s.bootstrapConfigGVK, s.bootstrapConfig))
		if !ok || s.bootstrapConfig.GetResourceVersion() != bootstrapConfigCacheEntry.obj.GetResourceVersion() {
			var err error
			bootstrapConfigCacheEntry.obj, err = external.GetObjectFromContractVersionedRef(ctx, r.APIReader, s.machine.Spec.Bootstrap.ConfigRef, s.machine.Namespace,
				&client.GetOptions{Raw: &metav1.GetOptions{ResourceVersion: s.bootstrapConfig.GetResourceVersion()}})
			if err != nil {
				return updateMachineResult{}, pkgerrors.Wrapf(err, "failed to generate UpdateMachine request")
			}
		}
		r.externalObjectsCache.Add(bootstrapConfigCacheEntry) // Add to cache or extend expiry of cache entry on cache hit.

		req.Desired.BootstrapConfig = runtime.RawExtension{Object: cleanupUnstructured(bootstrapConfigCacheEntry.obj)}
	}

	var messages []string
	for _, extensionHandler := range extensionHandlers {
		// Call UpdateMachine extension.
		resp := &runtimehooksv1.UpdateMachineResponse{}
		if err := r.RuntimeClient.CallExtension(ctx, runtimehooksv1.UpdateMachine, s.machine, extensionHandler, req, resp); err != nil {
			return updateMachineResult{}, err
		}

		if resp.Message != "" {
			messages = append(messages, resp.Message)
		}

		// If the extension asks to be called again later, stop here and requeue.
		if resp.RetryAfterSeconds > 0 {
			return updateMachineResult{
				retryAfter:              r.now().Add(time.Duration(resp.RetryAfterSeconds) * time.Second),
				retryAfterFromExtension: true,
				messages:                messages,
			}, nil
		}
	}

	// Cleanup cache proactively when in-place update is completed instead of waiting for the expiry
	r.externalObjectsCache.Delete(infraMachineCacheEntry)
	if s.bootstrapConfig != nil {
		r.externalObjectsCache.Delete(bootstrapConfigCacheEntry)
	}

	return updateMachineResult{
		retryAfter: r.now().Add(updateMachineCacheDuration),
		messages:   messages,
	}, nil
}

// completeInPlaceUpdate removes in-place update annotations from InfraMachine, BootstrapConfig, Machine,
// and then marks the UpdateMachine hook as done (removes it from pending-hooks annotation).
func (r *Reconciler) completeInPlaceUpdate(ctx context.Context, s *scope) error {
	log := ctrl.LoggerFrom(ctx)

	if err := r.removeInPlaceUpdateAnnotation(ctx, r.Client, "Machine", s.machine); err != nil {
		return err
	}

	if s.infraMachine == nil {
		log.Info("InfraMachine not found during in-place update completion, skipping annotation removal")
	} else {
		c, ok := r.DynamicCache.GetWriter(ctx, s.infraMachineGVK)
		if !ok {
			return pkgerrors.Errorf("failed to remove %s annotation from %s %s: failed to create client", clusterv1.UpdateInProgressAnnotation, s.infraMachineGVK.Kind, klog.KObj(s.infraMachine))
		}
		if err := r.removeInPlaceUpdateAnnotation(ctx, c, s.infraMachineGVK.Kind, s.infraMachine); err != nil {
			return err
		}
	}

	if s.bootstrapConfig != nil {
		c, ok := r.DynamicCache.GetWriter(ctx, s.bootstrapConfigGVK)
		if !ok {
			return pkgerrors.Errorf("failed to remove %s annotation from %s %s: failed to create client", clusterv1.UpdateInProgressAnnotation, s.bootstrapConfigGVK.Kind, klog.KObj(s.bootstrapConfig))
		}
		if err := r.removeInPlaceUpdateAnnotation(ctx, c, s.bootstrapConfigGVK.Kind, s.bootstrapConfig); err != nil {
			return err
		}
	}

	// Note: This call will not update the resourceVersion on machine, so that the patchHelper in the main
	// Reconcile func won't get a conflict.
	if err := hooks.MarkAsDone(ctx, r.Client, s.machine, false, runtimehooksv1.UpdateMachine); err != nil {
		return err
	}

	// Delete the UpdateMachine cache entry, so the UpdateMachine hook is called again for the next in-place update of this Machine.
	r.updateMachineCache.Delete(UpdateMachineCacheEntry{Machine: client.ObjectKeyFromObject(s.machine)})

	log.Info("Completed in-place update")
	return nil
}

// removeInPlaceUpdateAnnotation removes the in-place update annotation from an object and patches it immediately.
func (r *Reconciler) removeInPlaceUpdateAnnotation(ctx context.Context, c client.Writer, kind string, obj client.Object) error {
	annotations := obj.GetAnnotations()
	if _, exists := annotations[clusterv1.UpdateInProgressAnnotation]; !exists {
		return nil
	}

	// Note: DeepCopy object to not modify the passed-in object which can lead to conflict errors later on.
	obj = obj.DeepCopyObject().(client.Object)
	orig := obj.DeepCopyObject().(client.Object)
	delete(annotations, clusterv1.UpdateInProgressAnnotation)
	obj.SetAnnotations(annotations)

	if err := c.Patch(ctx, obj, client.MergeFrom(orig)); err != nil {
		return pkgerrors.Wrapf(err, "failed to remove %s annotation from %s %s", clusterv1.UpdateInProgressAnnotation, kind, klog.KObj(obj))
	}

	return nil
}

func cleanupMachine(machine *clusterv1.Machine) *clusterv1.Machine {
	return &clusterv1.Machine{
		// Set GVK because object is later marshalled with json.Marshal when the hook request is sent.
		TypeMeta: metav1.TypeMeta{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        machine.Name,
			Namespace:   machine.Namespace,
			Labels:      machine.Labels,
			Annotations: machine.Annotations,
		},
		Spec: *machine.Spec.DeepCopy(),
	}
}

func cleanupUnstructured(u *unstructured.Unstructured) *unstructured.Unstructured {
	cleanedUpU := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": u.GetAPIVersion(),
			"kind":       u.GetKind(),
			"spec":       u.Object["spec"],
		},
	}
	cleanedUpU.SetName(u.GetName())
	cleanedUpU.SetNamespace(u.GetNamespace())
	cleanedUpU.SetLabels(u.GetLabels())
	cleanedUpU.SetAnnotations(u.GetAnnotations())
	return cleanedUpU
}

// externalObjectsCacheEntry is an Entry for the contractObjectsCache.
type externalObjectsCacheEntry struct {
	obj *unstructured.Unstructured
}

// Key returns the cache key of a contractObjectsCacheEntry.
func (r externalObjectsCacheEntry) Key() string {
	return fmt.Sprintf("%s: %s", r.obj.GroupVersionKind(), klog.KObj(r.obj))
}

func externalObjectsCacheKey(objGVK schema.GroupVersionKind, obj client.Object) string {
	return fmt.Sprintf("%s: %s", objGVK, klog.KObj(obj))
}
