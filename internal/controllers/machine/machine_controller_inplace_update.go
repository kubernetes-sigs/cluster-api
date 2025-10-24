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
	"time"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/hooks"
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
				return ctrl.Result{}, errors.Wrap(err, "failed to clean up orphaned UpdateMachine hook and annotations")
			}
		}

		return ctrl.Result{}, nil
	}

	// If hook is not pending, we're waiting for the owner controller to mark it as pending.
	if !hasUpdateMachinePending {
		log.Info("In-place update annotations are set, waiting for UpdateMachine hook to be marked as pending")
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

	if s.infraMachine == nil {
		s.updatingReason = clusterv1.MachineUpdateFailedReason
		s.updatingMessage = "In-place update not possible: InfraMachine not found"
		return ctrl.Result{}, errors.New("in-place update failed: InfraMachine not found")
	}

	infraReady := r.isInfraMachineReadyForUpdate(s)
	bootstrapReady := r.isBootstrapConfigReadyForUpdate(s)

	if !infraReady || !bootstrapReady {
		log.Info("Waiting for InfraMachine and BootstrapConfig to be marked for in-place update")
		return ctrl.Result{}, nil
	}

	log.Info("UpdateMachine hook is pending, calling runtime hook")
	result, message, err := r.callUpdateMachineHook(ctx, s)
	if err != nil {
		s.updatingReason = clusterv1.MachineUpdateFailedReason
		s.updatingMessage = fmt.Sprintf("UpdateMachine hook failed: %v", err)
		return ctrl.Result{}, err
	}

	if result.RequeueAfter > 0 {
		s.updatingReason = clusterv1.MachineWaitingForUpdateMachineHookReason
		if message != "" {
			s.updatingMessage = fmt.Sprintf("UpdateMachine hook in progress: %s", message)
		} else {
			s.updatingMessage = "UpdateMachine hook in progress"
		}
		return result, nil
	}

	log.Info("In-place update completed successfully")
	if err := r.completeInPlaceUpdate(ctx, s); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to complete in-place update")
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

// callUpdateMachineHook calls the UpdateMachine runtime hook for the machine.
func (r *Reconciler) callUpdateMachineHook(ctx context.Context, s *scope) (ctrl.Result, string, error) {
	log := ctrl.LoggerFrom(ctx)

	// Validate that exactly one extension is registered for the UpdateMachine hook.
	// For the current iteration, we only support a single extension to ensure safe behavior.
	// Support for multiple extensions will be introduced in a future iteration.
	extensions, err := r.RuntimeClient.GetAllExtensions(ctx, runtimehooksv1.UpdateMachine, s.machine)
	if err != nil {
		return ctrl.Result{}, "", err
	}

	if len(extensions) == 0 {
		return ctrl.Result{}, "", errors.New("no extensions registered for UpdateMachine hook")
	}

	if len(extensions) > 1 {
		return ctrl.Result{}, "", errors.Errorf("multiple extensions registered for UpdateMachine hook: only one extension is supported in the current iteration, found %d extensions: %v", len(extensions), extensions)
	}

	request := &runtimehooksv1.UpdateMachineRequest{
		Desired: runtimehooksv1.UpdateMachineRequestObjects{
			Machine:               *cleanupMachine(s.machine),
			InfrastructureMachine: runtime.RawExtension{Object: cleanupUnstructured(s.infraMachine)},
		},
	}

	if s.bootstrapConfig != nil {
		request.Desired.BootstrapConfig = runtime.RawExtension{Object: cleanupUnstructured(s.bootstrapConfig)}
	}

	response := &runtimehooksv1.UpdateMachineResponse{}

	if err := r.RuntimeClient.CallAllExtensions(ctx, runtimehooksv1.UpdateMachine, s.machine, request, response); err != nil {
		return ctrl.Result{}, "", errors.Wrap(err, "failed to call UpdateMachine hook")
	}

	if response.GetRetryAfterSeconds() != 0 {
		log.Info(fmt.Sprintf("UpdateMachine hook requested retry after %d seconds", response.GetRetryAfterSeconds()))
		return ctrl.Result{RequeueAfter: time.Duration(response.GetRetryAfterSeconds()) * time.Second}, response.GetMessage(), nil
	}

	log.Info("UpdateMachine hook completed successfully")
	return ctrl.Result{}, response.GetMessage(), nil
}

// completeInPlaceUpdate removes in-place update annotations from InfraMachine, BootstrapConfig, Machine,
// and then marks the UpdateMachine hook as done (removes it from pending-hooks annotation).
func (r *Reconciler) completeInPlaceUpdate(ctx context.Context, s *scope) error {
	log := ctrl.LoggerFrom(ctx)

	if err := r.removeInPlaceUpdateAnnotation(ctx, s.machine); err != nil {
		return err
	}

	if s.infraMachine == nil {
		return errors.New("InfraMachine must exist to complete in-place update")
	}

	if err := r.removeInPlaceUpdateAnnotation(ctx, s.infraMachine); err != nil {
		return err
	}

	if s.bootstrapConfig != nil {
		if err := r.removeInPlaceUpdateAnnotation(ctx, s.bootstrapConfig); err != nil {
			return err
		}
	}

	if err := hooks.MarkAsDone(ctx, r.Client, s.machine, runtimehooksv1.UpdateMachine); err != nil {
		return err
	}

	log.Info("Removed in-place update annotations and marked hook as done")
	return nil
}

// removeInPlaceUpdateAnnotation removes the in-place update annotation from an object and patches it immediately.
func (r *Reconciler) removeInPlaceUpdateAnnotation(ctx context.Context, obj client.Object) error {
	annotations := obj.GetAnnotations()
	if _, exists := annotations[clusterv1.UpdateInProgressAnnotation]; !exists {
		return nil
	}

	gvk, err := apiutil.GVKForObject(obj, r.Client.Scheme())
	if err != nil {
		return errors.Wrapf(err, "failed to remove %s annotation from object %s", clusterv1.UpdateInProgressAnnotation, klog.KObj(obj))
	}

	orig := obj.DeepCopyObject().(client.Object)
	delete(annotations, clusterv1.UpdateInProgressAnnotation)
	obj.SetAnnotations(annotations)

	if err := r.Client.Patch(ctx, obj, client.MergeFrom(orig)); err != nil {
		return errors.Wrapf(err, "failed to remove %s annotation from %s %s", clusterv1.UpdateInProgressAnnotation, gvk.Kind, klog.KObj(obj))
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
