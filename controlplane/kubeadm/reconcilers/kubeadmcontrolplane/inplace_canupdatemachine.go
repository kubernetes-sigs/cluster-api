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

package kubeadmcontrolplane

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	pkgerrors "github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/pkg"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/util/compare"
	"sigs.k8s.io/cluster-api/internal/util/inplace"
	"sigs.k8s.io/cluster-api/internal/util/patch"
	"sigs.k8s.io/cluster-api/internal/util/ssa"
	"sigs.k8s.io/cluster-api/util/cache"
)

// CanUpdateMachineCacheEntry is an entry for the CanUpdateMachine hook cache.
type CanUpdateMachineCacheEntry struct {
	// Machine is the machine for which we cache the CanUpdateMachine result.
	Machine client.ObjectKey

	// KCPGeneration is the generation for which we cache the CanUpdateMachine result.
	KCPGeneration int64

	// canUpdateMachineResult is the cached CanUpdateMachine result.
	canUpdateMachineResult
}

// Key returns the cache key of a CanUpdateMachineCacheEntry.
func (r CanUpdateMachineCacheEntry) Key() string {
	return r.Machine.String()
}

// canUpdateMachineCacheDuration is the duration for which CanUpdateMachine results are cached
// (if the extension did not return retryAfterSeconds).
const canUpdateMachineCacheDuration = 5 * time.Minute

type canUpdateMachineResult struct {
	// retryAfter is the time at which the result expires and we should reconcile again (and call CanUpdateMachine extensions again).
	// Note: This is either the retry requested by an extension via retryAfterSeconds (see retryAfterFromExtension)
	// or the time at which the cached result expires.
	retryAfter time.Time

	// retryAfterFromExtension is true if an extension returned retryAfterSeconds, i.e. the decision is still in progress.
	// Note: This is needed to differentiate between retryAfterSeconds>0 and other cases.
	retryAfterFromExtension bool

	canUpdateMachine    bool
	affectsAvailability bool

	// allowedOperations are the operations that are allowed by all extensions.
	// Note: The zero value (nil) means that the extensions have no opinion, i.e. all operations are allowed.
	// An empty list means that no operation is allowed.
	allowedOperations *[]runtimehooksv1.MachineUpdateOperation

	// messages are the messages from the CanUpdateMachine responses.
	messages []string

	// reasons are the reasons why the Machine cannot be updated in-place.
	reasons []string
}

func (r *Reconciler) canUpdateMachine(ctx context.Context, controlPlane *pkg.ControlPlane, machine *clusterv1.Machine, machineUpToDateResult pkg.UpToDateResult) (canUpdateMachineResult, error) {
	if r.overrideCanUpdateMachineFunc != nil {
		return r.overrideCanUpdateMachineFunc(ctx, machine, machineUpToDateResult)
	}

	log := ctrl.LoggerFrom(ctx).WithValues("Machine", klog.KObj(machine))

	// Machine cannot be updated in-place if the feature gate is not enabled.
	if !feature.Gates.Enabled(feature.InPlaceUpdates) {
		return canUpdateMachineResult{canUpdateMachine: false}, nil
	}

	// Machine cannot be updated in-place if the UpToDate func was not able to provide all objects,
	// e.g. if the InfraMachine or KubeadmConfig was deleted.
	if machineUpToDateResult.DesiredMachine == nil ||
		machineUpToDateResult.CurrentInfraMachine == nil ||
		machineUpToDateResult.DesiredInfraMachine == nil ||
		machineUpToDateResult.CurrentKubeadmConfig == nil ||
		machineUpToDateResult.DesiredKubeadmConfig == nil {
		return canUpdateMachineResult{canUpdateMachine: false}, nil
	}

	extensionHandlers, err := r.RuntimeClient.GetAllExtensions(ctx, runtimehooksv1.CanUpdateMachine, machine)
	if err != nil {
		controlPlane.CanUpdateMachineResults.InternalErrorMessage = fmt.Sprintf("Blocked by CanUpdateMachine hook for Machine %s: %s", machine.Name, err.Error())
		return canUpdateMachineResult{canUpdateMachine: false}, err
	}
	// Machine cannot be updated in-place if no CanUpdateMachine extensions are registered.
	if len(extensionHandlers) == 0 {
		return canUpdateMachineResult{canUpdateMachine: false}, nil
	}
	if len(extensionHandlers) > 1 {
		err := pkgerrors.Errorf("found multiple CanUpdateMachine hooks (%s): only one hook is supported", strings.Join(extensionHandlers, ","))
		controlPlane.CanUpdateMachineResults.InternalErrorMessage = fmt.Sprintf("Blocked by CanUpdateMachine hook for Machine %s: %s", machine.Name, err.Error())
		return canUpdateMachineResult{canUpdateMachine: false}, err
	}

	var res canUpdateMachineResult
	logSuffix := ""
	if entry := tryToGetResultsFromCache(r.now(), r.canUpdateMachineCache, machine, controlPlane.KCP); entry != nil {
		// Get result from cache.
		res = entry.canUpdateMachineResult

		// Refresh the cache entry, so only tryToGetResultsFromCache decides when a cache entry should not be used anymore.
		r.canUpdateMachineCache.Add(*entry)

		// Note: Cached results are only logged on log level 5.
		log = log.V(5)
		logSuffix = " (cached)"
	} else {
		// Get result from extensions.
		var err error
		res, err = r.canExtensionsUpdateMachine(ctx, machine, machineUpToDateResult, extensionHandlers)
		if err != nil {
			controlPlane.CanUpdateMachineResults.InternalErrorMessage = fmt.Sprintf("Blocked by CanUpdateMachine hook for Machine %s: %s", machine.Name, err.Error())
			return canUpdateMachineResult{canUpdateMachine: false}, err
		}

		r.canUpdateMachineCache.Add(CanUpdateMachineCacheEntry{
			Machine:                client.ObjectKeyFromObject(machine),
			KCPGeneration:          controlPlane.KCP.Generation,
			canUpdateMachineResult: res,
		})
	}

	if len(res.reasons) > 0 {
		log = log.WithValues("reason", strings.Join(res.reasons, ","))
	}
	if len(res.messages) > 0 {
		log = log.WithValues("messages", strings.Join(res.messages, ","))
	}
	switch {
	case res.retryAfterFromExtension:
		msg := fmt.Sprintf("Blocked by CanUpdateMachine hook for Machine %s", machine.Name)
		if len(res.messages) > 0 {
			msg = fmt.Sprintf("%s: %s", msg, strings.Join(res.messages, ", "))
		}
		controlPlane.CanUpdateMachineResults.WaitingForDecisionMessage = msg
		log.Info(fmt.Sprintf("Machine %s cannot be rolled out yet: extensions returned retryAfterSeconds, requeuing%s", klog.KObj(machine), logSuffix))
	case !res.canUpdateMachine:
		log.Info(fmt.Sprintf("Machine %s cannot be updated in-place by extensions%s", klog.KObj(machine), logSuffix))
	case res.affectsAvailability:
		log.Info(fmt.Sprintf("Machine %s can be updated in-place by extensions (affects availability)%s", klog.KObj(machine), logSuffix))
	default:
		log.Info(fmt.Sprintf("Machine %s can be updated in-place by extensions (does not affect availability)%s", klog.KObj(machine), logSuffix))
	}
	return res, nil
}

func tryToGetResultsFromCache(now time.Time, cache cache.Cache[CanUpdateMachineCacheEntry], machine *clusterv1.Machine, kcp *controlplanev1.KubeadmControlPlane) *CanUpdateMachineCacheEntry {
	entry, ok := cache.Has(CanUpdateMachineCacheEntry{
		Machine: client.ObjectKeyFromObject(machine),
	}.Key())
	// If cache entry does not exist, don't use the cache.
	if !ok {
		return nil
	}

	// If KCP generation changed, don't use the cache.
	// I.e. as soon as KCP generation is changed we will call CanUpdateMachine again.
	if kcp.Generation != entry.KCPGeneration {
		return nil
	}

	// If retryAfter is in the past, don't use the cache.
	if !entry.retryAfter.After(now) {
		return nil
	}

	// If retryAfter is in the future, use the cache.
	return &entry
}

// canExtensionsUpdateMachine calls CanUpdateMachine extensions to decide if a Machine can be updated in-place.
// Note: This is following the same general structure that is used in the Apply func in
// internal/controllers/topology/cluster/patches/engine.go.
func (r *Reconciler) canExtensionsUpdateMachine(ctx context.Context, machine *clusterv1.Machine, machineUpToDateResult pkg.UpToDateResult, extensionHandlers []string) (canUpdateMachineResult, error) {
	if r.overrideCanExtensionsUpdateMachine != nil {
		return r.overrideCanExtensionsUpdateMachine(ctx, machine, machineUpToDateResult, extensionHandlers)
	}

	// Create the CanUpdateMachine request.
	cannotUpdateMachineReason, req, err := createRequest(ctx, r.Client, machine, machineUpToDateResult)
	if err != nil {
		return canUpdateMachineResult{canUpdateMachine: false}, pkgerrors.Wrapf(err, "failed to generate CanUpdateMachine request")
	}
	if cannotUpdateMachineReason != "" {
		return canUpdateMachineResult{
			retryAfter:       r.now().Add(canUpdateMachineCacheDuration),
			canUpdateMachine: false,
			reasons:          []string{cannotUpdateMachineReason},
		}, nil
	}

	// Note: We always call all extensions (instead of stopping as soon as the current and desired objects are matching),
	// because every extension can have an opinion about the allowed operations and every extension
	// is able to block (independent of what it is able to update in-place).
	var messages []string
	var affectsAvailability bool
	var allowedOperations *[]runtimehooksv1.MachineUpdateOperation
	for _, extensionHandler := range extensionHandlers {
		// Call CanUpdateMachine extension.
		resp := &runtimehooksv1.CanUpdateMachineResponse{}
		if err := r.RuntimeClient.CallExtension(ctx, runtimehooksv1.CanUpdateMachine, machine, extensionHandler, req, resp); err != nil {
			return canUpdateMachineResult{canUpdateMachine: false}, err
		}

		if resp.Message != "" {
			messages = append(messages, resp.Message)
		}

		// If the extension asks to be called again later, stop here and requeue.
		// Note: In this case the allowedOperations of the extension are not considered, the decision is still in progress.
		if resp.RetryAfterSeconds > 0 {
			return canUpdateMachineResult{
				retryAfter:              r.now().Add(time.Duration(resp.RetryAfterSeconds) * time.Second),
				retryAfterFromExtension: true,
				messages:                messages,
			}, nil
		}

		// An operation is only allowed if it is allowed by all extensions.
		allowedOperations = intersectAllowedOperations(allowedOperations, resp.AllowedOperations)

		// If no operation is allowed, the rollout is blocked: stop here, no other extension can change that.
		if allowedOperations != nil && len(*allowedOperations) == 0 {
			return canUpdateMachineResult{
				retryAfter:        r.now().Add(canUpdateMachineCacheDuration),
				canUpdateMachine:  false,
				allowedOperations: allowedOperations,
				messages:          messages,
				reasons:           []string{"no operation is allowed by extensions"},
			}, nil
		}

		affectsAvailability = affectsAvailability || ptr.Deref(resp.AffectsAvailability, true)

		// Apply patches from the CanUpdateMachine response to the request.
		if err := applyPatchesToRequest(ctx, req, resp); err != nil {
			return canUpdateMachineResult{canUpdateMachine: false}, pkgerrors.Wrapf(err, "failed to apply patches from extension %s to the CanUpdateMachine request", extensionHandler)
		}
	}

	// Check if current and desired objects are now matching.
	matches, reasons, err := matchesMachine(req)
	if err != nil {
		return canUpdateMachineResult{canUpdateMachine: false}, pkgerrors.Wrapf(err, "failed to compare current and desired objects after calling extensions %s", strings.Join(extensionHandlers, ","))
	}

	return canUpdateMachineResult{
		retryAfter:          r.now().Add(canUpdateMachineCacheDuration),
		canUpdateMachine:    matches,
		affectsAvailability: matches && affectsAvailability,
		allowedOperations:   allowedOperations,
		messages:            messages,
		reasons:             reasons,
	}, nil
}

// intersectAllowedOperations returns the operations that are allowed by both a and b.
// Note: nil means no opinion, i.e. all operations are allowed. An empty list means that no operation is allowed.
// The result is nil if both a and b are nil. Otherwise the result is a pointer to a list that only contains
// known operations without duplicates. Unknown operations are ignored.
func intersectAllowedOperations(a, b *[]runtimehooksv1.MachineUpdateOperation) *[]runtimehooksv1.MachineUpdateOperation {
	if a == nil && b == nil {
		return nil
	}

	result := []runtimehooksv1.MachineUpdateOperation{}
	for _, operation := range runtimehooksv1.AllMachineUpdateOperations {
		if isOperationAllowed(a, operation) && isOperationAllowed(b, operation) {
			result = append(result, operation)
		}
	}
	return &result
}

// isOperationAllowed returns true if the operation is allowed according to allowedOperations.
// Note: If allowedOperations is not set the extensions have no opinion, i.e. all operations are allowed.
func isOperationAllowed(allowedOperations *[]runtimehooksv1.MachineUpdateOperation, operation runtimehooksv1.MachineUpdateOperation) bool {
	return allowedOperations == nil || slices.Contains(*allowedOperations, operation)
}

func createRequest(ctx context.Context, c client.Client, currentMachine *clusterv1.Machine, machineUpToDateResult pkg.UpToDateResult) (string, *runtimehooksv1.CanUpdateMachineRequest, error) {
	// DeepCopy objects to avoid mutations.
	currentMachineForDiff := currentMachine.DeepCopy()
	currentKubeadmConfigForDiff := machineUpToDateResult.CurrentKubeadmConfig.DeepCopy()
	currentInfraMachineForDiff := machineUpToDateResult.CurrentInfraMachine.DeepCopy()

	desiredMachineForDiff := machineUpToDateResult.DesiredMachine.DeepCopy()
	desiredKubeadmConfigForDiff := machineUpToDateResult.DesiredKubeadmConfig.DeepCopy()
	desiredInfraMachineForDiff := machineUpToDateResult.DesiredInfraMachine.DeepCopy()

	// Sync in-place mutable changes from desired to current KubeadmConfig / InfraMachine.
	// Note: Writing these fields is handled by syncMachines and not the responsibility of in-place updates.
	// Note: Desired KubeadmConfig / InfraMachine already contain the latest labels & annotations.
	currentKubeadmConfigForDiff.SetLabels(desiredKubeadmConfigForDiff.GetLabels())
	currentKubeadmConfigForDiff.SetAnnotations(desiredKubeadmConfigForDiff.GetAnnotations())
	currentInfraMachineForDiff.SetLabels(desiredInfraMachineForDiff.GetLabels())
	currentInfraMachineForDiff.SetAnnotations(desiredInfraMachineForDiff.GetAnnotations())

	// Apply defaulting to current / desired Machine / KubeadmConfig / InfraMachine.
	// Machine
	// Note: currentMachineForDiff doesn't need a dry-run as it was just written in syncMachines and then
	//       update in controlPlane to ensure the Machine we get here is the latest version of the Machine.
	// Note: desiredMachineForDiff needs a dry-run because otherwise we have unintended diffs, e.g. dataSecretName,
	//       providerID and nodeDeletionTimeout don't exist on the newly computed desired Machine.
	if err := ssa.Patch(ctx, c, kcpManagerName, desiredMachineForDiff, ssa.WithDryRun{}); err != nil {
		// Note: While we check for specific errors for InfraMachine below we don't do the same here for Machine
		// as we don't expect validating webhook errors for the Machine. If these errors actually occur
		// we consider this a bug that should be fixed.
		return "", nil, pkgerrors.Wrap(err, "server side apply dry-run failed for desired Machine")
	}
	// InfraMachine
	// Note: Both currentInfraMachineForDiff and desiredInfraMachineForDiff need a dry-run to ensure changes
	//       in defaulting logic and fields added by other controllers don't lead to an unintended diff.
	if err := ssa.Patch(ctx, c, kcpManagerName, currentInfraMachineForDiff, ssa.WithDryRun{}); err != nil {
		// Note: If ssa.Patch fails because a validating webhook returns an error we cannot in-place update the InfraMachine
		// as we would also later not be able to apply the InfraMachine object when triggering the in-place update.
		// We expect that this only happens for desiredInfraMachineForDiff below but added it here as well as a safeguard.
		if apierrors.IsInvalid(err) || apierrors.IsForbidden(err) {
			return fmt.Sprintf("server side apply dry-run for current InfraMachine failed: %s", err), nil, nil
		}
		return "", nil, pkgerrors.Wrap(err, "server side apply dry-run failed for current InfraMachine")
	}
	if err := ssa.Patch(ctx, c, kcpManagerName, desiredInfraMachineForDiff, ssa.WithDryRun{}); err != nil {
		// Note: If ssa.Patch fails because a validating webhook returns an error we cannot in-place update the InfraMachine
		// as we would also later not be able to apply the InfraMachine object when triggering the in-place update.
		if apierrors.IsInvalid(err) || apierrors.IsForbidden(err) {
			return fmt.Sprintf("server side apply dry-run for desired InfraMachine failed: %s", err), nil, nil
		}
		return "", nil, pkgerrors.Wrap(err, "server side apply dry-run failed for desired InfraMachine")
	}
	// KubeadmConfig
	// Note: Both currentKubeadmConfigForDiff and desiredKubeadmConfigForDiff don't need a dry-run as
	//       PrepareKubeadmConfigsForDiff already has to perfectly handle differences between current / desired
	//       KubeadmConfig. Otherwise the regular rollout logic would not detect correctly if a Machine needs a rollout.
	// Note: KubeadmConfig doesn't have a defaulting webhook and no API defaulting anymore.
	desiredKubeadmConfigForDiff, currentKubeadmConfigForDiff = pkg.PrepareKubeadmConfigsForDiff(desiredKubeadmConfigForDiff, currentKubeadmConfigForDiff, true)

	// Cleanup objects and create request.
	req := &runtimehooksv1.CanUpdateMachineRequest{
		Current: runtimehooksv1.CanUpdateMachineRequestObjects{
			Machine: *cleanupMachine(currentMachineForDiff),
		},
		Desired: runtimehooksv1.CanUpdateMachineRequestObjects{
			Machine: *cleanupMachine(desiredMachineForDiff),
		},
	}
	var err error
	req.Current.BootstrapConfig, err = patch.ConvertToRawExtension(cleanupKubeadmConfig(currentKubeadmConfigForDiff))
	if err != nil {
		return "", nil, err
	}
	req.Desired.BootstrapConfig, err = patch.ConvertToRawExtension(cleanupKubeadmConfig(desiredKubeadmConfigForDiff))
	if err != nil {
		return "", nil, err
	}
	req.Current.InfrastructureMachine, err = patch.ConvertToRawExtension(cleanupUnstructured(currentInfraMachineForDiff))
	if err != nil {
		return "", nil, err
	}
	req.Desired.InfrastructureMachine, err = patch.ConvertToRawExtension(cleanupUnstructured(desiredInfraMachineForDiff))
	if err != nil {
		return "", nil, err
	}

	return "", req, nil
}

func cleanupMachine(machine *clusterv1.Machine) *clusterv1.Machine {
	return &clusterv1.Machine{
		// Set GVK because object is later marshalled with json.Marshal.
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

func cleanupKubeadmConfig(kubeadmConfig *bootstrapv1.KubeadmConfig) *bootstrapv1.KubeadmConfig {
	return &bootstrapv1.KubeadmConfig{
		// Set GVK because object is later marshalled with json.Marshal.
		TypeMeta: metav1.TypeMeta{
			APIVersion: bootstrapv1.GroupVersion.String(),
			Kind:       "KubeadmConfig",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        kubeadmConfig.Name,
			Namespace:   kubeadmConfig.Namespace,
			Labels:      kubeadmConfig.Labels,
			Annotations: kubeadmConfig.Annotations,
		},
		Spec: *kubeadmConfig.Spec.DeepCopy(),
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

func applyPatchesToRequest(ctx context.Context, req *runtimehooksv1.CanUpdateMachineRequest, resp *runtimehooksv1.CanUpdateMachineResponse) error {
	if resp.MachinePatch.IsDefined() {
		if err := patch.ApplyPatchToTypedObject(ctx, &req.Current.Machine, resp.MachinePatch, []string{"spec"}); err != nil {
			return err
		}
	}

	if resp.BootstrapConfigPatch.IsDefined() {
		if _, err := patch.ApplyPatchToObject(ctx, &req.Current.BootstrapConfig, resp.BootstrapConfigPatch, []string{"spec"}); err != nil {
			return err
		}
	}

	if resp.InfrastructureMachinePatch.IsDefined() {
		if _, err := patch.ApplyPatchToObject(ctx, &req.Current.InfrastructureMachine, resp.InfrastructureMachinePatch, []string{"spec"}); err != nil {
			return err
		}
	}

	return nil
}

func matchesMachine(req *runtimehooksv1.CanUpdateMachineRequest) (bool, []string, error) {
	var reasons []string
	match, diff, err := matchesMachineSpec(&req.Current.Machine, &req.Desired.Machine)
	if err != nil {
		return false, nil, pkgerrors.Wrapf(err, "failed to match Machine")
	}
	if !match {
		reasons = append(reasons, fmt.Sprintf("Machine cannot be updated in-place: %s", diff))
	}
	match, diff, err = matchesUnstructuredSpec(req.Current.BootstrapConfig, req.Desired.BootstrapConfig)
	if err != nil {
		return false, nil, pkgerrors.Wrapf(err, "failed to match KubeadmConfig")
	}
	if !match {
		reasons = append(reasons, fmt.Sprintf("KubeadmConfig cannot be updated in-place: %s", diff))
	}
	match, diff, err = matchesUnstructuredSpec(req.Current.InfrastructureMachine, req.Desired.InfrastructureMachine)
	if err != nil {
		return false, nil, pkgerrors.Wrapf(err, "failed to match %s", req.Current.InfrastructureMachine.Object.GetObjectKind().GroupVersionKind().Kind)
	}
	if !match {
		reasons = append(reasons, fmt.Sprintf("%s cannot be updated in-place: %s", req.Current.InfrastructureMachine.Object.GetObjectKind().GroupVersionKind().Kind, diff))
	}

	if len(reasons) > 0 {
		return false, reasons, nil
	}

	return true, nil, nil
}

func matchesMachineSpec(patched, desired *clusterv1.Machine) (equal bool, diff string, matchErr error) {
	// Note: Wrapping Machine specs in a Machine for proper formatting of the diff.
	return compare.Diff(
		&clusterv1.Machine{
			Spec: *inplace.CleanupMachineSpecForDiff(&patched.Spec),
		},
		&clusterv1.Machine{
			Spec: *inplace.CleanupMachineSpecForDiff(&desired.Spec),
		},
	)
}

func matchesUnstructuredSpec(patched, desired runtime.RawExtension) (equal bool, diff string, matchErr error) {
	// Note: Both patched and desired objects are always Unstructured as createRequest and
	//       applyPatchToObject are always setting objects as Unstructured.
	patchedUnstructured, ok := patched.Object.(*unstructured.Unstructured)
	if !ok {
		return false, "", pkgerrors.Errorf("patched object is not an Unstructured")
	}
	desiredUnstructured, ok := desired.Object.(*unstructured.Unstructured)
	if !ok {
		return false, "", pkgerrors.Errorf("desired object is not an Unstructured")
	}
	// Note: Wrapping Unstructured specs in an Unstructured for proper formatting of the diff.
	return compare.Diff(
		&unstructured.Unstructured{
			Object: map[string]interface{}{
				"spec": patchedUnstructured.Object["spec"],
			},
		},
		&unstructured.Unstructured{
			Object: map[string]interface{}{
				"spec": desiredUnstructured.Object["spec"],
			},
		},
	)
}
