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
	"encoding/json"
	"fmt"
	"time"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/hooks"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
)

// reconcileInPlaceUpdate handles the in-place update workflow for a Machine.
func (r *Reconciler) reconcileInPlaceUpdate(ctx context.Context, s *scope) (ctrl.Result, error) {
	if !feature.Gates.Enabled(feature.InPlaceUpdates) {
		return ctrl.Result{}, nil
	}

	log := ctrl.LoggerFrom(ctx)

	machineAnnotations := s.machine.GetAnnotations()
	_, inPlaceUpdateInProgress := machineAnnotations[clusterv1.InPlaceUpdateInProgressAnnotation]
	hasUpdateMachinePending := hooks.IsPending(runtimehooksv1.UpdateMachine, s.machine)

	if !inPlaceUpdateInProgress {
		// Clean up any orphaned pending hooks before exiting.
		if hasUpdateMachinePending {
			log.Info("In-place update annotation removed but UpdateMachine hook still pending, cleaning up orphaned hook")
			if err := hooks.MarkAsDone(ctx, r.Client, s.machine, runtimehooksv1.UpdateMachine); err != nil { // this patches the machine, should we defer that patch until the end of reconciliation?
				return ctrl.Result{}, errors.Wrap(err, "failed to clean up orphaned UpdateMachine hook")
			}
		}

		conditions.Set(s.machine, metav1.Condition{
			Type:   clusterv1.MachineUpdatingCondition,
			Status: metav1.ConditionFalse,
			Reason: clusterv1.MachineNotUpdatingReason,
		})
		return ctrl.Result{}, nil
	}

	infraReady := r.isInfraMachineReadyForUpdate(s)
	bootstrapReady := r.isBootstrapConfigReadyForUpdate(s)

	if !infraReady || !bootstrapReady {
		log.Info("Waiting for InfraMachine and BootstrapConfig to be marked for in-place update")
		conditions.Set(s.machine, metav1.Condition{
			Type:    clusterv1.MachineUpdatingCondition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.MachineWaitingForInPlaceUpdateAnnotationsReason,
			Message: "Waiting for InfraMachine and BootstrapConfig to be marked for update",
		})
		return ctrl.Result{}, nil
	}

	if hasUpdateMachinePending {
		log.Info("UpdateMachine hook is pending, calling runtime hook")
		result, err := r.callUpdateMachineHook(ctx, s)
		if err != nil {
			conditions.Set(s.machine, metav1.Condition{
				Type:    clusterv1.MachineUpdatingCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineUpdateFailedReason,
				Message: fmt.Sprintf("UpdateMachine hook failed: %v", err),
			})
			return ctrl.Result{}, err
		}

		if result.RequeueAfter > 0 {
			conditions.Set(s.machine, metav1.Condition{
				Type:    clusterv1.MachineUpdatingCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineWaitingForUpdateMachineHookReason,
				Message: "UpdateMachine hook in progress",
			})
			return result, nil
		}

		if err := hooks.MarkAsDone(ctx, r.Client, s.machine, runtimehooksv1.UpdateMachine); err != nil { // this patches the machine, should we defer that patch until the end of reconciliation?
			return ctrl.Result{}, errors.Wrap(err, "failed to mark UpdateMachine hook as done")
		}

		log.Info("In-place update completed successfully")
		if err := r.completeInPlaceUpdate(ctx, s); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to complete in-place update")
		}

		conditions.Set(s.machine, metav1.Condition{
			Type:   clusterv1.MachineUpdatingCondition,
			Status: metav1.ConditionFalse,
			Reason: clusterv1.MachineNotUpdatingReason,
		})

		return ctrl.Result{}, nil
	}

	// If we reach here, annotations are set but hook is not pending.
	// This means we're waiting for the owner controller to mark the hook as pending.
	log.Info("In-place update annotations are set, waiting for UpdateMachine hook to be marked as pending")
	conditions.Set(s.machine, metav1.Condition{
		Type:    clusterv1.MachineUpdatingCondition,
		Status:  metav1.ConditionFalse,
		Reason:  clusterv1.MachineWaitingForInPlaceUpdateAnnotationsReason,
		Message: "Waiting for UpdateMachine hook to be marked as pending",
	})

	return ctrl.Result{}, nil
}

// isInfraMachineReadyForUpdate checks if the InfraMachine has the in-place update annotation.
func (r *Reconciler) isInfraMachineReadyForUpdate(s *scope) bool {
	if s.infraMachine == nil {
		return false
	}
	infraMachineAnnotations := s.infraMachine.GetAnnotations()
	if infraMachineAnnotations == nil {
		return false
	}
	_, hasAnnotation := infraMachineAnnotations[clusterv1.InPlaceUpdateInProgressAnnotation]
	return hasAnnotation
}

// isBootstrapConfigReadyForUpdate checks if the BootstrapConfig has the in-place update annotation.
func (r *Reconciler) isBootstrapConfigReadyForUpdate(s *scope) bool {
	if s.bootstrapConfig == nil {
		return true
	}
	bootstrapConfigAnnotations := s.bootstrapConfig.GetAnnotations()
	if bootstrapConfigAnnotations == nil {
		return false
	}
	_, hasAnnotation := bootstrapConfigAnnotations[clusterv1.InPlaceUpdateInProgressAnnotation]
	return hasAnnotation
}

// callUpdateMachineHook calls the UpdateMachine runtime hook for the machine.
func (r *Reconciler) callUpdateMachineHook(ctx context.Context, s *scope) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	request := &runtimehooksv1.UpdateMachineRequest{}
	request.Desired.Machine = *s.machine.DeepCopy()

	if s.infraMachine != nil { // should it return an error if infraMachine is nil?
		infraMachineRaw, err := runtime.DefaultUnstructuredConverter.ToUnstructured(s.infraMachine)
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to convert InfraMachine to unstructured")
		}
		request.Desired.InfrastructureMachine.Raw, err = json.Marshal(infraMachineRaw)
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to marshal InfraMachine")
		}
	}

	if s.bootstrapConfig != nil { // should it return an error if bootstrapConfig is nil?
		bootstrapConfigRaw, err := runtime.DefaultUnstructuredConverter.ToUnstructured(s.bootstrapConfig)
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to convert BootstrapConfig to unstructured")
		}
		request.Desired.BootstrapConfig.Raw, err = json.Marshal(bootstrapConfigRaw)
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to marshal BootstrapConfig")
		}
	}

	response := &runtimehooksv1.UpdateMachineResponse{}

	if err := r.RuntimeClient.CallAllExtensions(ctx, runtimehooksv1.UpdateMachine, s.machine, request, response); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to call UpdateMachine hook")
	}

	if response.GetRetryAfterSeconds() != 0 {
		log.Info(fmt.Sprintf("UpdateMachine hook requested retry after %d seconds", response.GetRetryAfterSeconds()))
		return ctrl.Result{RequeueAfter: time.Duration(response.GetRetryAfterSeconds()) * time.Second}, nil
	}

	log.Info("UpdateMachine hook completed successfully")
	return ctrl.Result{}, nil
}

// completeInPlaceUpdate removes in-place update annotations from InfraMachine, BootstrapConfig and Machine.
func (r *Reconciler) completeInPlaceUpdate(ctx context.Context, s *scope) error {
	log := ctrl.LoggerFrom(ctx)

	if s.infraMachine != nil {
		if err := r.removeInPlaceUpdateAnnotation(ctx, s.infraMachine); err != nil {
			return errors.Wrap(err, "failed to remove in-place update annotation from InfraMachine")
		}
	}

	if s.bootstrapConfig != nil {
		if err := r.removeInPlaceUpdateAnnotation(ctx, s.bootstrapConfig); err != nil {
			return errors.Wrap(err, "failed to remove in-place update annotation from BootstrapConfig")
		}
	}

	// Only remove from Machine if all child object patches succeeded.
	r.removeInPlaceUpdateAnnotationFromMachine(s.machine)

	log.Info("Removed in-place update annotations from all objects")
	return nil
}

// removeInPlaceUpdateAnnotationFromMachine removes the in-place update annotation from the Machine.
func (r *Reconciler) removeInPlaceUpdateAnnotationFromMachine(machine *clusterv1.Machine) {
	annotations := machine.GetAnnotations()
	if annotations == nil {
		return
	}

	if _, exists := annotations[clusterv1.InPlaceUpdateInProgressAnnotation]; !exists {
		return
	}

	delete(annotations, clusterv1.InPlaceUpdateInProgressAnnotation)
	machine.SetAnnotations(annotations)
}

// removeInPlaceUpdateAnnotation removes the in-place update annotation from an object and patches it.
func (r *Reconciler) removeInPlaceUpdateAnnotation(ctx context.Context, obj client.Object) error {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return nil
	}

	if _, exists := annotations[clusterv1.InPlaceUpdateInProgressAnnotation]; !exists {
		return nil
	}

	patchHelper, err := patch.NewHelper(obj, r.Client)
	if err != nil {
		return errors.Wrap(err, "failed to create patch helper")
	}

	delete(annotations, clusterv1.InPlaceUpdateInProgressAnnotation)
	obj.SetAnnotations(annotations)

	if err := patchHelper.Patch(ctx, obj); err != nil {
		return errors.Wrap(err, "failed to patch object")
	}

	return nil
}
