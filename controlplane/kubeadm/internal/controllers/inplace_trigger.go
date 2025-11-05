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
	"fmt"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	"sigs.k8s.io/cluster-api/internal/hooks"
	clientutil "sigs.k8s.io/cluster-api/internal/util/client"
	"sigs.k8s.io/cluster-api/internal/util/ssa"
)

func (r *KubeadmControlPlaneReconciler) triggerInPlaceUpdate(ctx context.Context, machine *clusterv1.Machine, machineUpToDateResult internal.UpToDateResult) error {
	if r.overrideTriggerInPlaceUpdate != nil {
		return r.overrideTriggerInPlaceUpdate(ctx, machine, machineUpToDateResult)
	}

	log := ctrl.LoggerFrom(ctx)
	log.Info("Triggering in-place update", "Machine", klog.KObj(machine))

	// Mark Machine for in-place update.
	// Note: Once we write UpdateInProgressAnnotation we will always continue with the in-place update.
	// Note: Intentionally using client.Patch instead of SSA. Otherwise we would have to ensure we preserve
	//       UpdateInProgressAnnotation on existing Machines in KCP and that would lead to race conditions when
	//       the Machine controller tries to remove the annotation and then KCP adds it back.
	if _, ok := machine.Annotations[clusterv1.UpdateInProgressAnnotation]; !ok {
		orig := machine.DeepCopy()
		if machine.Annotations == nil {
			machine.Annotations = map[string]string{}
		}
		machine.Annotations[clusterv1.UpdateInProgressAnnotation] = ""
		if err := r.Client.Patch(ctx, machine, client.MergeFrom(orig)); err != nil {
			return errors.Wrapf(err, "failed to trigger in-place update for Machine %s by setting the %s annotation", klog.KObj(machine), clusterv1.UpdateInProgressAnnotation)
		}

		// Wait until the cache observed the Machine with UpdateInProgressAnnotation to ensure subsequent reconciles
		// will observe it as well and accordingly don't trigger another in-place update concurrently.
		if err := clientutil.WaitForCacheToBeUpToDate(ctx, r.Client, fmt.Sprintf("setting the %s annotation", clusterv1.UpdateInProgressAnnotation), machine); err != nil {
			return err
		}
	}

	// TODO: If this func fails below we are going to reconcile again and call triggerInPlaceUpdate again. If KCP
	// spec changed in the meantime desired objects might change and then we would use different desired objects
	// for UpdateMachine compared to what we used in CanUpdateMachine.
	// If we want to account for that we could consider writing desired InfraMachine/KubeadmConfig/Machine with
	// the in-progress annotation on the Machine and use it if necessary (and clean it up when we set the pending
	// annotation). This might lead to issues with the maximum object size supported by etcd though (so we might
	// have to write the objects somewhere else).

	desiredMachine := machineUpToDateResult.DesiredMachine
	desiredInfraMachine := machineUpToDateResult.DesiredInfraMachine
	desiredKubeadmConfig := machineUpToDateResult.DesiredKubeadmConfig

	// Machine cannot be updated in-place if the UpToDate func was not able to provide all objects,
	// e.g. if the InfraMachine or KubeadmConfig was deleted.
	// Note: As canUpdateMachine also checks these fields for nil this can only happen if the initial
	//      triggerInPlaceUpdate call failed after setting UpdateInProgressAnnotation.
	if desiredInfraMachine == nil {
		return errors.Errorf("failed to complete triggering in-place update for Machine %s, could not compute desired InfraMachine", klog.KObj(machine))
	}
	if desiredKubeadmConfig == nil {
		return errors.Errorf("failed to complete triggering in-place update for Machine %s, could not compute desired KubeadmConfig", klog.KObj(machine))
	}

	// Write InfraMachine without the labels & annotations that are written continuously by updateLabelsAndAnnotations.
	// Note: Let's update InfraMachine first because that is the call that is most likely to fail.
	desiredInfraMachine.SetLabels(nil)
	desiredInfraMachine.SetAnnotations(map[string]string{
		// ClonedFrom annotations are initially written by createInfraMachine and then managedField ownership is
		// removed via ssa.RemoveManagedFieldsForLabelsAndAnnotations.
		// updateLabelsAndAnnotations is intentionally not updating them as they should be only updated as part
		// of an in-place update here, e.g. for the case where the InfraMachineTemplate was rotated.
		clusterv1.TemplateClonedFromNameAnnotation:      desiredInfraMachine.GetAnnotations()[clusterv1.TemplateClonedFromNameAnnotation],
		clusterv1.TemplateClonedFromGroupKindAnnotation: desiredInfraMachine.GetAnnotations()[clusterv1.TemplateClonedFromGroupKindAnnotation],
		// Machine controller waits for this annotation to exist on Machine and related objects before starting the in-place update.
		clusterv1.UpdateInProgressAnnotation: "",
	})
	if err := ssa.Patch(ctx, r.Client, kcpManagerName, desiredInfraMachine); err != nil {
		return errors.Wrapf(err, "failed to complete triggering in-place update for Machine %s", klog.KObj(machine))
	}

	// Write KubeadmConfig without the labels & annotations that are written continuously by updateLabelsAndAnnotations.
	desiredKubeadmConfig.Labels = nil
	desiredKubeadmConfig.Annotations = map[string]string{
		// Machine controller waits for this annotation to exist on Machine and related objects before starting the in-place update.
		clusterv1.UpdateInProgressAnnotation: "",
	}
	if err := ssa.Patch(ctx, r.Client, kcpManagerName, desiredKubeadmConfig); err != nil {
		return errors.Wrapf(err, "failed to complete triggering in-place update for Machine %s", klog.KObj(machine))
	}
	if desiredKubeadmConfig.Spec.InitConfiguration.IsDefined() {
		if err := r.removeInitConfiguration(ctx, desiredKubeadmConfig); err != nil {
			return errors.Wrapf(err, "failed to complete triggering in-place update for Machine %s", klog.KObj(machine))
		}
	}

	// Write Machine.
	if err := ssa.Patch(ctx, r.Client, kcpManagerName, desiredMachine); err != nil {
		return errors.Wrapf(err, "failed to complete triggering in-place update for Machine %s", klog.KObj(machine))
	}

	// Note: Once we write PendingHooksAnnotation the Machine controller will start with the in-place update.
	// Note: Intentionally using client.Patch (via hooks.MarkAsPending + patchHelper) instead of SSA. Otherwise we would
	//       have to ensure we preserve PendingHooksAnnotation on existing Machines in KCP and that would lead to race
	//       conditions when the Machine controller tries to remove the annotation and KCP adds it back.
	if err := hooks.MarkAsPending(ctx, r.Client, desiredMachine, runtimehooksv1.UpdateMachine); err != nil {
		return errors.Wrapf(err, "failed to complete triggering in-place update for Machine %s", klog.KObj(machine))
	}

	log.Info("Completed triggering in-place update", "Machine", klog.KObj(machine))
	r.recorder.Event(machine, corev1.EventTypeNormal, "SuccessfulStartInPlaceUpdate", "Machine starting in-place update")

	// Wait until the cache observed the Machine with PendingHooksAnnotation to ensure subsequent reconciles
	// will observe it as well and won't repeatedly call triggerInPlaceUpdate.
	return clientutil.WaitForCacheToBeUpToDate(ctx, r.Client, "marking the UpdateMachine hook as pending", desiredMachine)
}

func (r *KubeadmControlPlaneReconciler) removeInitConfiguration(ctx context.Context, desiredKubeadmConfig *bootstrapv1.KubeadmConfig) error {
	// Remove initConfiguration with Patch if necessary.
	// This is only necessary if ssa.Patch above cannot remove the initConfiguration field because
	// capi-kubeadmcontrolplane does not own it.
	// Note: desiredKubeadmConfig here will always contain a joinConfiguration instead of an initConfiguration.
	//
	// This happens only on KubeadmConfigs (for kubeadm init) created with CAPI <= v1.11, because the initConfiguration
	// field is not owned by anyone there (i.e. orphaned) after we called ssa.MigrateManagedFields in syncMachines.
	//
	// In KubeadmConfigs created with CAPI >= v1.12 capi-kubeadmcontrolplane owns the initConfiguration field
	// and accordingly the ssa.Patch above is able to remove it.
	//
	// There are two ways this can be resolved:
	// - Machine goes through an in-place rollout and this code removes the initConfiguration.
	// - Machine is rolled out (re-created) which will use the new managedField structure.
	//
	// As CAPI v1.11 supported up to Kubernetes v1.34. We assume the Machine has to be either rolled out
	// or in-place updated before CAPI drops support for Kubernetes v1.34. So this code can be removed
	// once CAPI doesn't support Kubernetes v1.34 anymore.
	origKubeadmConfig := desiredKubeadmConfig.DeepCopy()
	desiredKubeadmConfig.Spec.InitConfiguration = bootstrapv1.InitConfiguration{}
	if err := r.Client.Patch(ctx, desiredKubeadmConfig, client.MergeFrom(origKubeadmConfig)); err != nil {
		return errors.Wrap(err, "failed to patch KubeadmConfig: failed to remove initConfiguration")
	}
	return nil
}
