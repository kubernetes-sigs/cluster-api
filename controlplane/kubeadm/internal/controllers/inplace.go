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

package controllers

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	"sigs.k8s.io/cluster-api/internal/hooks"
	"sigs.k8s.io/cluster-api/internal/util/ssa"
)

func (r *KubeadmControlPlaneReconciler) tryInPlaceUpdate(
	ctx context.Context,
	controlPlane *internal.ControlPlane,
	machineToInPlaceUpdate *clusterv1.Machine,
	machineUpToDateResult internal.UpToDateResult,
) (fallbackToScaleDown bool, _ ctrl.Result, _ error) {
	if r.overrideTryInPlaceUpdateFunc != nil {
		return r.overrideTryInPlaceUpdateFunc(ctx, controlPlane, machineToInPlaceUpdate, machineUpToDateResult)
	}

	// Run preflight checks to ensure that the control plane is stable before proceeding with in-place update operation.
	if resultForAllMachines := r.preflightChecks(ctx, controlPlane); !resultForAllMachines.IsZero() {
		// If the control plane is not stable, check if the issues are only for machineToInPlaceUpdate.
		if result := r.preflightChecks(ctx, controlPlane, machineToInPlaceUpdate); result.IsZero() {
			// The issues are only for machineToInPlaceUpdate, fallback to scale down.
			// Note: The consequence of this is that a Machine with issues is scaled down and not in-place updated.
			return true, ctrl.Result{}, nil
		}

		return false, resultForAllMachines, nil
	}

	// Note: Usually canUpdateMachine is only called once for a single Machine rollout.
	// If it returns true, the code below will mark the in-place update as in progress via the
	// clusterv1.UpdateInProgressAnnotation. From this point forward we are not going to call canUpdateMachine again.
	// If it returns false, we are going to fall back to scale down which will delete the Machine.
	// We only have to repeat the canUpdateMachine call if the write call to set the clusterv1.UpdateInProgressAnnotation
	// fails or if we fail to delete the Machine.
	canUpdate, err := r.canUpdateMachine(ctx, machineToInPlaceUpdate, machineUpToDateResult)
	if err != nil {
		return false, ctrl.Result{}, errors.Wrapf(err, "failed to determine if Machine %s can be updated in-place", machineToInPlaceUpdate.Name)
	}

	if !canUpdate {
		return true, ctrl.Result{}, nil
	}

	// Mark Machine for in-place update
	// Note: Once we reach this point and write UpdateInProgressAnnotation we will always continue with the in-place update.
	// Note: Intentionally using client.Patch instead of SSA. Otherwise we would have race-conditions when the Machine controller
	// tries to remove the annotation and KCP adds it back.
	if _, ok := machineToInPlaceUpdate.Annotations[clusterv1.UpdateInProgressAnnotation]; !ok {
		orig := machineToInPlaceUpdate.DeepCopy()
		if machineToInPlaceUpdate.Annotations == nil {
			machineToInPlaceUpdate.Annotations = map[string]string{}
		}
		machineToInPlaceUpdate.Annotations[clusterv1.UpdateInProgressAnnotation] = ""
		if err := r.Client.Patch(ctx, machineToInPlaceUpdate, client.MergeFrom(orig)); err != nil {
			return false, ctrl.Result{}, err
		}
		// Wait until the cache observed the change to ensure subsequent reconciles will observe it as well.
		if err := wait.PollUntilContextTimeout(ctx, 5*time.Millisecond, 5*time.Second, true, func(ctx context.Context) (bool, error) {
			m := &clusterv1.Machine{}
			if err := r.Client.Get(ctx, client.ObjectKeyFromObject(machineToInPlaceUpdate), m); err != nil {
				return false, err
			}
			_, annotationSet := m.Annotations[clusterv1.UpdateInProgressAnnotation]
			return annotationSet, nil
		}); err != nil {
			return false, ctrl.Result{}, errors.Wrapf(err, "failed waiting for Machine %s to be updated in the cache after setting the %s annotation", klog.KObj(machineToInPlaceUpdate), clusterv1.UpdateInProgressAnnotation)
		}
	}

	return false, ctrl.Result{}, r.completeTriggerInPlaceUpdate(ctx, machineUpToDateResult)
}

func (r *KubeadmControlPlaneReconciler) completeTriggerInPlaceUpdate(ctx context.Context, machinesNeedingRolloutResult internal.UpToDateResult) error {
	// TODO: If this func fails "in the middle" we are going to reconcile again, if KCP changed in the mean time
	// desired objects might change and then we would use different desired objects for UpdateMachine compared to
	// what we used in CanUpdateMachine.
	// If we want to account for that we could consider writing desired InfraMachine/KubeadmConfig/Machine with the in-progress annotation
	// on the Machine and use it if necessary (and clean it up if necessary when we set the pending annotation).

	// Machine cannot be updated in-place if the UpToDate func was not able to provide all objects,
	// e.g. if the InfraMachine or KubeadmConfig was deleted.
	if machinesNeedingRolloutResult.DesiredInfraMachine == nil {
		return errors.Errorf("failed to complete triggering in-place update, could not compute desired InfraMachine")
	}
	if machinesNeedingRolloutResult.DesiredKubeadmConfig == nil {
		return errors.Errorf("failed to complete triggering in-place update, could not compute desired KubeadmConfig")
	}

	// Write InfraMachine without the labels & annotations that are written continuously by applyExternalObjectLabelsAnnotations.
	// Note: Let's update InfraMachine first because that is the call that is most likely to fail.
	machinesNeedingRolloutResult.DesiredInfraMachine.SetLabels(nil)
	machinesNeedingRolloutResult.DesiredInfraMachine.SetAnnotations(map[string]string{
		// ClonedFrom annotations are written by createInfraMachine so we have to send them here again to not unset them.
		// They also have to be updated here if the InfraMachineTemplate was rotated.
		clusterv1.TemplateClonedFromNameAnnotation:      machinesNeedingRolloutResult.DesiredInfraMachine.GetAnnotations()[clusterv1.TemplateClonedFromNameAnnotation],
		clusterv1.TemplateClonedFromGroupKindAnnotation: machinesNeedingRolloutResult.DesiredInfraMachine.GetAnnotations()[clusterv1.TemplateClonedFromGroupKindAnnotation],
		clusterv1.UpdateInProgressAnnotation:            "",
	})
	if err := ssa.Patch(ctx, r.Client, kcpManagerName, machinesNeedingRolloutResult.DesiredInfraMachine); err != nil {
		return errors.Wrapf(err, "failed to apply %s", machinesNeedingRolloutResult.DesiredInfraMachine.GetKind())
	}

	// Write KubeadmConfig without labels & annotations.
	machinesNeedingRolloutResult.DesiredKubeadmConfig.Labels = nil
	machinesNeedingRolloutResult.DesiredKubeadmConfig.Annotations = map[string]string{
		clusterv1.UpdateInProgressAnnotation: "",
	}
	if err := ssa.Patch(ctx, r.Client, kcpManagerName, machinesNeedingRolloutResult.DesiredKubeadmConfig); err != nil {
		return errors.Wrapf(err, "failed to apply KubeadmConfig")
	}

	if machinesNeedingRolloutResult.DesiredKubeadmConfig.Spec.InitConfiguration.IsDefined() {
		// Remove initConfiguration with Patch if necessary.
		// This is only necessary if ssa.Patch cannot remove the initConfiguration field because
		// capi-kubeadmcontrolplane does not own it.
		//
		// This happens only on KubeadmConfigs (for kubeadm init) created with CAPI <= v1.11, because the initConfiguration
		// field is not owned by anyone there (i.e. orphaned) after we called ssa.MigrateManagedFields.
		//
		// In KubeadmConfigs created with CAPI >= v1.12 capi-kubeadmcontrolplane owns the initConfiguration field
		// and accordingly the ssa.Patch above removes it.
		//
		// There are two ways this can be resolved:
		// - Machine goes through an in-place rollout
		// - Machine is rolled out (re-created). CAPI v1.11 supported up to Kubernetes v1.34. We assume the Machine
		//   has to be either rolled out or in-place updated before CAPI drops support for Kubernetes v1.34.
		origKubeadmConfig := machinesNeedingRolloutResult.DesiredKubeadmConfig.DeepCopy()
		machinesNeedingRolloutResult.DesiredKubeadmConfig.Spec.InitConfiguration = bootstrapv1.InitConfiguration{}
		if err := r.Client.Patch(ctx, machinesNeedingRolloutResult.DesiredKubeadmConfig, client.MergeFrom(origKubeadmConfig)); err != nil {
			return errors.Wrapf(err, "failed to apply KubeadmConfig: failed to remove initConfiguration")
		}
	}

	if err := ssa.Patch(ctx, r.Client, kcpManagerName, machinesNeedingRolloutResult.DesiredMachine); err != nil {
		return errors.Wrap(err, "failed to apply Machine")
	}

	// Note: We set this annotation intentionally in a separate call with the "manager" fieldManager.
	// If we combine it with the SSA call above we would have to preserve it in ComputeDesiredMachine.
	// If we do that we can encounter race conditions where the Machine controller removes the pending annotation
	// and KCP that is running concurrently is re-adding it.
	if err := hooks.MarkAsPending(ctx, r.Client, machinesNeedingRolloutResult.DesiredMachine, runtimehooksv1.UpdateMachine); err != nil {
		return errors.Wrap(err, "failed to apply Machine: mark Machine as pending update")
	}
	// Wait until the cache observed the change to ensure subsequent reconciles will observe it as well.
	if err := wait.PollUntilContextTimeout(ctx, 5*time.Millisecond, 5*time.Second, true, func(ctx context.Context) (bool, error) {
		m := &clusterv1.Machine{}
		if err := r.Client.Get(ctx, client.ObjectKeyFromObject(machinesNeedingRolloutResult.DesiredMachine), m); err != nil {
			return false, err
		}
		return hooks.IsPending(runtimehooksv1.UpdateMachine, m), nil
	}); err != nil {
		return errors.Wrapf(err, "failed waiting for Machine %s to be updated in the cache after marking the UpdateMachine hook as pending", klog.KObj(machinesNeedingRolloutResult.DesiredInfraMachine))
	}

	return nil
}
