/*
Copyright 2021 The Kubernetes Authors.

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

package cluster

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/storage/names"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/internal/controllers/topology/cluster/scope"
	"sigs.k8s.io/cluster-api/internal/controllers/topology/cluster/structuredmerge"
	"sigs.k8s.io/cluster-api/internal/hooks"
	tlog "sigs.k8s.io/cluster-api/internal/log"
	"sigs.k8s.io/cluster-api/internal/topology/check"
)

const (
	createEventReason = "TopologyCreate"
	updateEventReason = "TopologyUpdate"
	deleteEventReason = "TopologyDelete"
)

// reconcileState reconciles the current and desired state of the managed Cluster topology.
// NOTE: We are assuming all the required objects are provided as input; also, in case of any error,
// the entire reconcile operation will fail. This might be improved in the future if support for reconciling
// subset of a topology will be implemented.
func (r *Reconciler) reconcileState(ctx context.Context, s *scope.Scope) error {
	log := tlog.LoggerFrom(ctx)
	log.Infof("Reconciling state for topology owned objects")

	// Reconcile the Cluster shim, a temporary object used a mean to collect
	// objects/templates that can be orphaned in case of errors during the
	// remaining part of the reconcile process.
	if err := r.reconcileClusterShim(ctx, s); err != nil {
		return err
	}

	if feature.Gates.Enabled(feature.RuntimeSDK) {
		if err := r.callAfterHooks(ctx, s); err != nil {
			return err
		}
	}

	// Reconcile desired state of the InfrastructureCluster object.
	if err := r.reconcileInfrastructureCluster(ctx, s); err != nil {
		return err
	}

	// Reconcile desired state of the ControlPlane object.
	if err := r.reconcileControlPlane(ctx, s); err != nil {
		return err
	}

	// Reconcile desired state of the Cluster object.
	if err := r.reconcileCluster(ctx, s); err != nil {
		return err
	}

	// Reconcile desired state of the MachineDeployment objects.
	return r.reconcileMachineDeployments(ctx, s)
}

// Reconcile the Cluster shim, a temporary object used a mean to collect objects/templates
// that might be orphaned in case of errors during the remaining part of the reconcile process.
func (r *Reconciler) reconcileClusterShim(ctx context.Context, s *scope.Scope) error {
	shim := clusterShim(s.Current.Cluster)

	// If we are going to create the InfrastructureCluster or the ControlPlane object, then
	// add a temporary cluster-shim object and use it as an additional owner.
	// This will ensure the objects will be garbage collected in case of errors in between
	// creating InfrastructureCluster/ControlPlane objects and updating the Cluster with the
	// references to above objects.
	if s.Current.InfrastructureCluster == nil || s.Current.ControlPlane.Object == nil {
		// Note: we are using Patch instead of create for ensuring consistency in managedFields for the entire controller
		// but in this case it isn't strictly necessary given that we are not using server side apply for modifying the
		// object afterwards.
		helper, err := r.patchHelperFactory(nil, shim)
		if err != nil {
			return errors.Wrap(err, "failed to create the patch helper for the cluster shim object")
		}
		if err := helper.Patch(ctx); err != nil {
			return errors.Wrap(err, "failed to create the cluster shim object")
		}

		if err := r.APIReader.Get(ctx, client.ObjectKeyFromObject(shim), shim); err != nil {
			return errors.Wrap(err, "get shim after creation")
		}

		// Enforce type meta back given that it gets blanked out by Get.
		shim.Kind = "Secret"
		shim.APIVersion = corev1.SchemeGroupVersion.String()

		// Add the shim as a temporary owner for the InfrastructureCluster.
		ownerRefs := s.Desired.InfrastructureCluster.GetOwnerReferences()
		ownerRefs = append(ownerRefs, *ownerReferenceTo(shim))
		s.Desired.InfrastructureCluster.SetOwnerReferences(ownerRefs)

		// Add the shim as a temporary owner for the ControlPlane.
		ownerRefs = s.Desired.ControlPlane.Object.GetOwnerReferences()
		ownerRefs = append(ownerRefs, *ownerReferenceTo(shim))
		s.Desired.ControlPlane.Object.SetOwnerReferences(ownerRefs)
	}

	// If the InfrastructureCluster and the ControlPlane objects have been already created
	// in previous reconciliation, check if they have already been reconciled by the ClusterController
	// by verifying the ownerReference for the Cluster is present.
	//
	// When the Cluster and the shim object are both owners,
	// it's safe for us to remove the shim and garbage collect any potential orphaned resource.
	if s.Current.InfrastructureCluster != nil && s.Current.ControlPlane.Object != nil {
		clusterOwnsAll := hasOwnerReferenceFrom(s.Current.InfrastructureCluster, s.Current.Cluster) &&
			hasOwnerReferenceFrom(s.Current.ControlPlane.Object, s.Current.Cluster)
		shimOwnsAtLeastOne := hasOwnerReferenceFrom(s.Current.InfrastructureCluster, shim) ||
			hasOwnerReferenceFrom(s.Current.ControlPlane.Object, shim)

		if clusterOwnsAll && shimOwnsAtLeastOne {
			if err := r.Client.Delete(ctx, shim); err != nil {
				if !apierrors.IsNotFound(err) {
					return errors.Wrapf(err, "failed to delete the cluster shim object")
				}
			}
		}
	}
	return nil
}

func clusterShim(c *clusterv1.Cluster) *corev1.Secret {
	shim := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-shim", c.Name),
			Namespace: c.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*ownerReferenceTo(c),
			},
		},
		Type: clusterv1.ClusterSecretType,
	}
	return shim
}

func hasOwnerReferenceFrom(obj, owner client.Object) bool {
	for _, o := range obj.GetOwnerReferences() {
		if o.Kind == owner.GetObjectKind().GroupVersionKind().Kind && o.Name == owner.GetName() {
			return true
		}
	}
	return false
}

func getOwnerReferenceFrom(obj, owner client.Object) *metav1.OwnerReference {
	for _, o := range obj.GetOwnerReferences() {
		if o.Kind == owner.GetObjectKind().GroupVersionKind().Kind && o.Name == owner.GetName() {
			return &o
		}
	}
	return nil
}

func (r *Reconciler) callAfterHooks(ctx context.Context, s *scope.Scope) error {
	if err := r.callAfterControlPlaneInitialized(ctx, s); err != nil {
		return err
	}

	if err := r.callAfterClusterUpgrade(ctx, s); err != nil {
		return err
	}

	return nil
}

func (r *Reconciler) callAfterControlPlaneInitialized(ctx context.Context, s *scope.Scope) error {
	// If the cluster topology is being created then track to intent to call the AfterControlPlaneInitialized hook so that we can call it later.
	if s.Current.Cluster.Spec.InfrastructureRef == nil && s.Current.Cluster.Spec.ControlPlaneRef == nil {
		if err := hooks.MarkAsPending(ctx, r.Client, s.Current.Cluster, runtimehooksv1.AfterControlPlaneInitialized); err != nil {
			return errors.Wrapf(err, "failed to remove the %s hook from pending hooks tracker", runtimecatalog.HookName(runtimehooksv1.AfterControlPlaneInitialized))
		}
	}

	// Call the hook only if we are tracking the intent to do so. If it is not tracked it means we don't need to call the
	// hook because already called the hook after the control plane is initialized.
	if hooks.IsPending(runtimehooksv1.AfterControlPlaneInitialized, s.Current.Cluster) {
		if isControlPlaneInitialized(s.Current.Cluster) {
			// The control plane is initialized for the first time. Call all the registered extensions for the hook.
			hookRequest := &runtimehooksv1.AfterControlPlaneInitializedRequest{
				Cluster: *s.Current.Cluster,
			}
			hookResponse := &runtimehooksv1.AfterControlPlaneInitializedResponse{}
			if err := r.RuntimeClient.CallAllExtensions(ctx, runtimehooksv1.AfterControlPlaneInitialized, s.Current.Cluster, hookRequest, hookResponse); err != nil {
				return err
			}
			s.HookResponseTracker.Add(runtimehooksv1.AfterControlPlaneInitialized, hookResponse)
			if err := hooks.MarkAsDone(ctx, r.Client, s.Current.Cluster, runtimehooksv1.AfterControlPlaneInitialized); err != nil {
				return err
			}
		}
	}

	return nil
}

func isControlPlaneInitialized(cluster *clusterv1.Cluster) bool {
	for _, condition := range cluster.GetConditions() {
		if condition.Type == clusterv1.ControlPlaneInitializedCondition {
			if condition.Status == corev1.ConditionTrue {
				return true
			}
		}
	}
	return false
}

func (r *Reconciler) callAfterClusterUpgrade(ctx context.Context, s *scope.Scope) error {
	// Call the hook only if we are tracking the intent to do so. If it is not tracked it means we don't need to call the
	// hook because we didn't go through an upgrade or we already called the hook after the upgrade.
	if hooks.IsPending(runtimehooksv1.AfterClusterUpgrade, s.Current.Cluster) {
		// Call the registered extensions for the hook after the cluster is fully upgraded.
		// A clusters is considered fully upgraded if:
		// - Control plane is not upgrading
		// - Control plane is not scaling
		// - Control plane is not pending an upgrade
		// - MachineDeployments are not currently rolling out
		// - MAchineDeployments are not about to roll out
		// - MachineDeployments are not pending an upgrade

		// Check if the control plane is upgrading.
		cpUpgrading, err := contract.ControlPlane().IsUpgrading(s.Current.ControlPlane.Object)
		if err != nil {
			return errors.Wrap(err, "failed to check if control plane is upgrading")
		}

		// Check if the control plane is scaling. If the control plane does not support replicas
		// it will be considered as not scaling.
		var cpScaling bool
		if s.Blueprint.Topology.ControlPlane.Replicas != nil {
			cpScaling, err = contract.ControlPlane().IsScaling(s.Current.ControlPlane.Object)
			if err != nil {
				return errors.Wrap(err, "failed to check if the control plane is scaling")
			}
		}

		if !cpUpgrading && !cpScaling && !s.UpgradeTracker.ControlPlane.PendingUpgrade && // Control Plane checks
			len(s.UpgradeTracker.MachineDeployments.RolloutNames()) == 0 && // Machine deployments are not rollout out or not about to roll out
			!s.UpgradeTracker.MachineDeployments.PendingUpgrade() { // Machine Deployments are not pending an upgrade
			// Everything is stable and the cluster can be considered fully upgraded.
			hookRequest := &runtimehooksv1.AfterClusterUpgradeRequest{
				Cluster:           *s.Current.Cluster,
				KubernetesVersion: s.Current.Cluster.Spec.Topology.Version,
			}
			hookResponse := &runtimehooksv1.AfterClusterUpgradeResponse{}
			if err := r.RuntimeClient.CallAllExtensions(ctx, runtimehooksv1.AfterClusterUpgrade, s.Current.Cluster, hookRequest, hookResponse); err != nil {
				return err
			}
			s.HookResponseTracker.Add(runtimehooksv1.AfterClusterUpgrade, hookResponse)
			// The hook is successfully called; we can remove this hook from the list of pending-hooks.
			if err := hooks.MarkAsDone(ctx, r.Client, s.Current.Cluster, runtimehooksv1.AfterClusterUpgrade); err != nil {
				return err
			}
		}
	}

	return nil
}

// reconcileInfrastructureCluster reconciles the desired state of the InfrastructureCluster object.
func (r *Reconciler) reconcileInfrastructureCluster(ctx context.Context, s *scope.Scope) error {
	ctx, _ = tlog.LoggerFrom(ctx).WithObject(s.Desired.InfrastructureCluster).Into(ctx)

	ignorePaths, err := contract.InfrastructureCluster().IgnorePaths(s.Desired.InfrastructureCluster)
	if err != nil {
		return errors.Wrap(err, "failed to calculate ignore paths")
	}

	return r.reconcileReferencedObject(ctx, reconcileReferencedObjectInput{
		cluster:     s.Current.Cluster,
		current:     s.Current.InfrastructureCluster,
		desired:     s.Desired.InfrastructureCluster,
		ignorePaths: ignorePaths,
	})
}

// reconcileControlPlane works to bring the current state of a managed topology in line with the desired state. This involves
// updating the cluster where needed.
func (r *Reconciler) reconcileControlPlane(ctx context.Context, s *scope.Scope) error {
	// If the clusterClass mandates the controlPlane has infrastructureMachines, reconcile it.
	if s.Blueprint.HasControlPlaneInfrastructureMachine() {
		ctx, _ := tlog.LoggerFrom(ctx).WithObject(s.Desired.ControlPlane.InfrastructureMachineTemplate).Into(ctx)

		cpInfraRef, err := contract.ControlPlane().MachineTemplate().InfrastructureRef().Get(s.Desired.ControlPlane.Object)
		if err != nil {
			return errors.Wrapf(err, "failed to reconcile %s", tlog.KObj{Obj: s.Desired.ControlPlane.InfrastructureMachineTemplate})
		}

		// Create or update the MachineInfrastructureTemplate of the control plane.
		if err = r.reconcileReferencedTemplate(ctx, reconcileReferencedTemplateInput{
			cluster:              s.Current.Cluster,
			ref:                  cpInfraRef,
			current:              s.Current.ControlPlane.InfrastructureMachineTemplate,
			desired:              s.Desired.ControlPlane.InfrastructureMachineTemplate,
			compatibilityChecker: check.ObjectsAreCompatible,
			templateNamePrefix:   controlPlaneInfrastructureMachineTemplateNamePrefix(s.Current.Cluster.Name),
		},
		); err != nil {
			return err
		}

		// The controlPlaneObject.Spec.machineTemplate.infrastructureRef has to be updated in the desired object
		err = contract.ControlPlane().MachineTemplate().InfrastructureRef().Set(s.Desired.ControlPlane.Object, refToUnstructured(cpInfraRef))
		if err != nil {
			return errors.Wrapf(err, "failed to reconcile %s", tlog.KObj{Obj: s.Desired.ControlPlane.Object})
		}
	}

	// Create or update the ControlPlaneObject for the ControlPlaneState.
	ctx, _ = tlog.LoggerFrom(ctx).WithObject(s.Desired.ControlPlane.Object).Into(ctx)
	if err := r.reconcileReferencedObject(ctx, reconcileReferencedObjectInput{
		cluster:       s.Current.Cluster,
		current:       s.Current.ControlPlane.Object,
		desired:       s.Desired.ControlPlane.Object,
		versionGetter: contract.ControlPlane().Version().Get,
	}); err != nil {
		return err
	}

	// If the controlPlane has infrastructureMachines and the InfrastructureMachineTemplate has changed on this reconcile
	// delete the old template.
	// This is a best effort deletion only and may leak templates if an error occurs during reconciliation.
	if s.Blueprint.HasControlPlaneInfrastructureMachine() && s.Current.ControlPlane.InfrastructureMachineTemplate != nil {
		if s.Current.ControlPlane.InfrastructureMachineTemplate.GetName() != s.Desired.ControlPlane.InfrastructureMachineTemplate.GetName() {
			if err := r.Client.Delete(ctx, s.Current.ControlPlane.InfrastructureMachineTemplate); err != nil {
				return errors.Wrapf(err, "failed to delete oldinfrastructure machine template %s of control plane %s",
					tlog.KObj{Obj: s.Current.ControlPlane.InfrastructureMachineTemplate},
					tlog.KObj{Obj: s.Current.ControlPlane.Object},
				)
			}
		}
	}

	// If the ControlPlane has defined a current or desired MachineHealthCheck attempt to reconcile it.
	if s.Desired.ControlPlane.MachineHealthCheck != nil || s.Current.ControlPlane.MachineHealthCheck != nil {
		// Reconcile the current and desired state of the MachineHealthCheck.
		if err := r.reconcileMachineHealthCheck(ctx, s.Current.ControlPlane.MachineHealthCheck, s.Desired.ControlPlane.MachineHealthCheck); err != nil {
			return err
		}
	}
	return nil
}

// reconcileMachineHealthCheck creates, updates, deletes or leaves untouched a MachineHealthCheck depending on the difference between the
// current state and the desired state.
func (r *Reconciler) reconcileMachineHealthCheck(ctx context.Context, current, desired *clusterv1.MachineHealthCheck) error {
	log := tlog.LoggerFrom(ctx)

	// If a current MachineHealthCheck doesn't exist but there is a desired MachineHealthCheck attempt to create.
	if current == nil && desired != nil {
		log.Infof("Creating %s", tlog.KObj{Obj: desired})
		helper, err := r.patchHelperFactory(nil, desired)
		if err != nil {
			return errors.Wrapf(err, "failed to create patch helper for %s", tlog.KObj{Obj: desired})
		}
		if err := helper.Patch(ctx); err != nil {
			return errors.Wrapf(err, "failed to create %s", tlog.KObj{Obj: desired})
		}
		r.recorder.Eventf(desired, corev1.EventTypeNormal, createEventReason, "Created %q", tlog.KObj{Obj: desired})
		return nil
	}

	// If a current MachineHealthCheck exists but there is no desired MachineHealthCheck attempt to delete.
	if current != nil && desired == nil {
		log.Infof("Deleting %s", tlog.KObj{Obj: current})
		if err := r.Client.Delete(ctx, current); err != nil {
			// If the object to be deleted is not found don't throw an error.
			if !apierrors.IsNotFound(err) {
				return errors.Wrapf(err, "failed to delete %s", tlog.KObj{Obj: current})
			}
		}
		r.recorder.Eventf(current, corev1.EventTypeNormal, deleteEventReason, "Deleted %q", tlog.KObj{Obj: current})
		return nil
	}

	ctx, log = log.WithObject(current).Into(ctx)

	// Check differences between current and desired MachineHealthChecks, and patch if required.
	// NOTE: we want to be authoritative on the entire spec because the users are
	// expected to change MHC fields from the ClusterClass only.
	patchHelper, err := r.patchHelperFactory(current, desired)
	if err != nil {
		return errors.Wrapf(err, "failed to create patch helper for %s", tlog.KObj{Obj: current})
	}
	if !patchHelper.HasChanges() {
		log.V(3).Infof("No changes for %s", tlog.KObj{Obj: current})
		return nil
	}

	log.Infof("Patching %s", tlog.KObj{Obj: current})
	if err := patchHelper.Patch(ctx); err != nil {
		return errors.Wrapf(err, "failed to patch %s", tlog.KObj{Obj: current})
	}
	r.recorder.Eventf(current, corev1.EventTypeNormal, updateEventReason, "Updated %q", tlog.KObj{Obj: current})
	return nil
}

// reconcileCluster reconciles the desired state of the Cluster object.
// NOTE: this assumes reconcileInfrastructureCluster and reconcileControlPlane being already completed;
// most specifically, after a Cluster is created it is assumed that the reference to the InfrastructureCluster /
// ControlPlane objects should never change (only the content of the objects can change).
func (r *Reconciler) reconcileCluster(ctx context.Context, s *scope.Scope) error {
	ctx, log := tlog.LoggerFrom(ctx).WithObject(s.Desired.Cluster).Into(ctx)

	// Check differences between current and desired state, and eventually patch the current object.
	patchHelper, err := r.patchHelperFactory(s.Current.Cluster, s.Desired.Cluster)
	if err != nil {
		return errors.Wrapf(err, "failed to create patch helper for %s", tlog.KObj{Obj: s.Current.Cluster})
	}
	if !patchHelper.HasChanges() {
		log.V(3).Infof("No changes for %s", tlog.KObj{Obj: s.Current.Cluster})
		return nil
	}

	log.Infof("Patching %s", tlog.KObj{Obj: s.Current.Cluster})
	if err := patchHelper.Patch(ctx); err != nil {
		return errors.Wrapf(err, "failed to patch %s", tlog.KObj{Obj: s.Current.Cluster})
	}
	r.recorder.Eventf(s.Current.Cluster, corev1.EventTypeNormal, updateEventReason, "Updated %q", tlog.KObj{Obj: s.Current.Cluster})
	return nil
}

// reconcileMachineDeployments reconciles the desired state of the MachineDeployment objects.
func (r *Reconciler) reconcileMachineDeployments(ctx context.Context, s *scope.Scope) error {
	diff := calculateMachineDeploymentDiff(s.Current.MachineDeployments, s.Desired.MachineDeployments)

	// Create MachineDeployments.
	for _, mdTopologyName := range diff.toCreate {
		md := s.Desired.MachineDeployments[mdTopologyName]
		if err := r.createMachineDeployment(ctx, s.Current.Cluster, md); err != nil {
			return err
		}
	}

	// Update MachineDeployments.
	for _, mdTopologyName := range diff.toUpdate {
		currentMD := s.Current.MachineDeployments[mdTopologyName]
		desiredMD := s.Desired.MachineDeployments[mdTopologyName]
		if err := r.updateMachineDeployment(ctx, s.Current.Cluster, mdTopologyName, currentMD, desiredMD); err != nil {
			return err
		}
	}

	// Delete MachineDeployments.
	for _, mdTopologyName := range diff.toDelete {
		md := s.Current.MachineDeployments[mdTopologyName]
		if err := r.deleteMachineDeployment(ctx, s.Current.Cluster, md); err != nil {
			return err
		}
	}
	return nil
}

// createMachineDeployment creates a MachineDeployment and the corresponding Templates.
func (r *Reconciler) createMachineDeployment(ctx context.Context, cluster *clusterv1.Cluster, md *scope.MachineDeploymentState) error {
	log := tlog.LoggerFrom(ctx).WithMachineDeployment(md.Object)

	infraCtx, _ := log.WithObject(md.InfrastructureMachineTemplate).Into(ctx)
	if err := r.reconcileReferencedTemplate(infraCtx, reconcileReferencedTemplateInput{
		cluster: cluster,
		desired: md.InfrastructureMachineTemplate,
	}); err != nil {
		return errors.Wrapf(err, "failed to create %s", md.Object.Kind)
	}

	bootstrapCtx, _ := log.WithObject(md.BootstrapTemplate).Into(ctx)
	if err := r.reconcileReferencedTemplate(bootstrapCtx, reconcileReferencedTemplateInput{
		cluster: cluster,
		desired: md.BootstrapTemplate,
	}); err != nil {
		return errors.Wrapf(err, "failed to create %s", md.Object.Kind)
	}

	log = log.WithObject(md.Object)
	log.Infof(fmt.Sprintf("Creating %s", tlog.KObj{Obj: md.Object}))
	helper, err := r.patchHelperFactory(nil, md.Object)
	if err != nil {
		return createErrorWithoutObjectName(err, md.Object)
	}
	if err := helper.Patch(ctx); err != nil {
		return createErrorWithoutObjectName(err, md.Object)
	}
	r.recorder.Eventf(cluster, corev1.EventTypeNormal, createEventReason, "Created %q", tlog.KObj{Obj: md.Object})

	// If the MachineDeployment has defined a MachineHealthCheck reconcile it.
	if md.MachineHealthCheck != nil {
		if err := r.reconcileMachineHealthCheck(ctx, nil, md.MachineHealthCheck); err != nil {
			return err
		}
	}
	return nil
}

// updateMachineDeployment updates a MachineDeployment. Also rotates the corresponding Templates if necessary.
func (r *Reconciler) updateMachineDeployment(ctx context.Context, cluster *clusterv1.Cluster, mdTopologyName string, currentMD, desiredMD *scope.MachineDeploymentState) error {
	log := tlog.LoggerFrom(ctx).WithMachineDeployment(desiredMD.Object)

	infraCtx, _ := log.WithObject(desiredMD.InfrastructureMachineTemplate).Into(ctx)
	if err := r.reconcileReferencedTemplate(infraCtx, reconcileReferencedTemplateInput{
		cluster:              cluster,
		ref:                  &desiredMD.Object.Spec.Template.Spec.InfrastructureRef,
		current:              currentMD.InfrastructureMachineTemplate,
		desired:              desiredMD.InfrastructureMachineTemplate,
		templateNamePrefix:   infrastructureMachineTemplateNamePrefix(cluster.Name, mdTopologyName),
		compatibilityChecker: check.ObjectsAreCompatible,
	}); err != nil {
		return errors.Wrapf(err, "failed to reconcile %s", tlog.KObj{Obj: currentMD.Object})
	}

	bootstrapCtx, _ := log.WithObject(desiredMD.BootstrapTemplate).Into(ctx)
	if err := r.reconcileReferencedTemplate(bootstrapCtx, reconcileReferencedTemplateInput{
		cluster:              cluster,
		ref:                  desiredMD.Object.Spec.Template.Spec.Bootstrap.ConfigRef,
		current:              currentMD.BootstrapTemplate,
		desired:              desiredMD.BootstrapTemplate,
		templateNamePrefix:   bootstrapTemplateNamePrefix(cluster.Name, mdTopologyName),
		compatibilityChecker: check.ObjectsAreInTheSameNamespace,
	}); err != nil {
		return errors.Wrapf(err, "failed to reconcile %s", tlog.KObj{Obj: currentMD.Object})
	}

	// Patch MachineHealthCheck for the MachineDeployment.
	if desiredMD.MachineHealthCheck != nil || currentMD.MachineHealthCheck != nil {
		if err := r.reconcileMachineHealthCheck(ctx, currentMD.MachineHealthCheck, desiredMD.MachineHealthCheck); err != nil {
			return err
		}
	}

	// Check differences between current and desired MachineDeployment, and eventually patch the current object.
	log = log.WithObject(desiredMD.Object)
	patchHelper, err := r.patchHelperFactory(currentMD.Object, desiredMD.Object)
	if err != nil {
		return errors.Wrapf(err, "failed to create patch helper for %s", tlog.KObj{Obj: currentMD.Object})
	}
	if !patchHelper.HasChanges() {
		log.V(3).Infof("No changes for %s", tlog.KObj{Obj: currentMD.Object})
		return nil
	}

	log.Infof("Patching %s", tlog.KObj{Obj: currentMD.Object})
	if err := patchHelper.Patch(ctx); err != nil {
		return errors.Wrapf(err, "failed to patch %s", tlog.KObj{Obj: currentMD.Object})
	}
	r.recorder.Eventf(cluster, corev1.EventTypeNormal, updateEventReason, "Updated %q%s", tlog.KObj{Obj: currentMD.Object}, logMachineDeploymentVersionChange(currentMD.Object, desiredMD.Object))

	// We want to call both cleanup functions even if one of them fails to clean up as much as possible.
	return nil
}

func logMachineDeploymentVersionChange(current, desired *clusterv1.MachineDeployment) string {
	if current.Spec.Template.Spec.Version == nil || desired.Spec.Template.Spec.Version == nil {
		return ""
	}

	if *current.Spec.Template.Spec.Version != *desired.Spec.Template.Spec.Version {
		return fmt.Sprintf(" with version change from %s to %s", *current.Spec.Template.Spec.Version, *desired.Spec.Template.Spec.Version)
	}
	return ""
}

// deleteMachineDeployment deletes a MachineDeployment.
func (r *Reconciler) deleteMachineDeployment(ctx context.Context, cluster *clusterv1.Cluster, md *scope.MachineDeploymentState) error {
	log := tlog.LoggerFrom(ctx).WithMachineDeployment(md.Object).WithObject(md.Object)

	// delete MachineHealthCheck for the MachineDeployment.
	if md.MachineHealthCheck != nil {
		if err := r.reconcileMachineHealthCheck(ctx, md.MachineHealthCheck, nil); err != nil {
			return err
		}
	}
	log.Infof("Deleting %s", tlog.KObj{Obj: md.Object})
	if err := r.Client.Delete(ctx, md.Object); err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrapf(err, "failed to delete %s", tlog.KObj{Obj: md.Object})
	}
	r.recorder.Eventf(cluster, corev1.EventTypeNormal, deleteEventReason, "Deleted %q", tlog.KObj{Obj: md.Object})
	return nil
}

type machineDeploymentDiff struct {
	toCreate, toUpdate, toDelete []string
}

// calculateMachineDeploymentDiff compares two maps of MachineDeploymentState and calculates which
// MachineDeployments should be created, updated or deleted.
func calculateMachineDeploymentDiff(current, desired map[string]*scope.MachineDeploymentState) machineDeploymentDiff {
	var diff machineDeploymentDiff

	for md := range desired {
		if _, ok := current[md]; ok {
			diff.toUpdate = append(diff.toUpdate, md)
		} else {
			diff.toCreate = append(diff.toCreate, md)
		}
	}

	for md := range current {
		if _, ok := desired[md]; !ok {
			diff.toDelete = append(diff.toDelete, md)
		}
	}

	return diff
}

type unstructuredVersionGetter func(obj *unstructured.Unstructured) (*string, error)

type reconcileReferencedObjectInput struct {
	cluster       *clusterv1.Cluster
	current       *unstructured.Unstructured
	desired       *unstructured.Unstructured
	versionGetter unstructuredVersionGetter
	ignorePaths   []contract.Path
}

// reconcileReferencedObject reconciles the desired state of the referenced object.
// NOTE: After a referenced object is created it is assumed that the reference should
// never change (only the content of the object can eventually change). Thus, we are checking for strict compatibility.
func (r *Reconciler) reconcileReferencedObject(ctx context.Context, in reconcileReferencedObjectInput) error {
	log := tlog.LoggerFrom(ctx)

	// If there is no current object, create it.
	if in.current == nil {
		log.Infof("Creating %s", tlog.KObj{Obj: in.desired})
		helper, err := r.patchHelperFactory(nil, in.desired)
		if err != nil {
			return errors.Wrap(createErrorWithoutObjectName(err, in.desired), "failed to create patch helper")
		}
		if err := helper.Patch(ctx); err != nil {
			return createErrorWithoutObjectName(err, in.desired)
		}
		r.recorder.Eventf(in.cluster, corev1.EventTypeNormal, createEventReason, "Created %q", tlog.KObj{Obj: in.desired})
		return nil
	}

	// Check if the current and desired referenced object are compatible.
	if allErrs := check.ObjectsAreStrictlyCompatible(in.current, in.desired); len(allErrs) > 0 {
		return allErrs.ToAggregate()
	}

	// Check differences between current and desired state, and eventually patch the current object.
	patchHelper, err := r.patchHelperFactory(in.current, in.desired, structuredmerge.IgnorePaths(in.ignorePaths))
	if err != nil {
		return errors.Wrapf(err, "failed to create patch helper for %s", tlog.KObj{Obj: in.current})
	}
	if !patchHelper.HasChanges() {
		log.V(3).Infof("No changes for %s", tlog.KObj{Obj: in.desired})
		return nil
	}

	log.Infof("Patching %s", tlog.KObj{Obj: in.desired})
	if err := patchHelper.Patch(ctx); err != nil {
		return errors.Wrapf(err, "failed to patch %s", tlog.KObj{Obj: in.current})
	}
	r.recorder.Eventf(in.cluster, corev1.EventTypeNormal, updateEventReason, "Updated %q%s", tlog.KObj{Obj: in.desired}, logUnstructuredVersionChange(in.current, in.desired, in.versionGetter))
	return nil
}

func logUnstructuredVersionChange(current, desired *unstructured.Unstructured, versionGetter unstructuredVersionGetter) string {
	if versionGetter == nil {
		return ""
	}

	currentVersion, err := versionGetter(current)
	if err != nil || currentVersion == nil {
		return ""
	}
	desiredVersion, err := versionGetter(desired)
	if err != nil || desiredVersion == nil {
		return ""
	}

	if *currentVersion != *desiredVersion {
		return fmt.Sprintf(" with version change from %s to %s", *currentVersion, *desiredVersion)
	}
	return ""
}

type reconcileReferencedTemplateInput struct {
	cluster              *clusterv1.Cluster
	ref                  *corev1.ObjectReference
	current              *unstructured.Unstructured
	desired              *unstructured.Unstructured
	templateNamePrefix   string
	compatibilityChecker func(current, desired client.Object) field.ErrorList
}

// reconcileReferencedTemplate reconciles the desired state of a referenced Template.
// NOTE: According to Cluster API operational practices, when a referenced Template changes a template rotation is required:
// 1. create a new Template
// 2. update the reference
// 3. delete the old Template
// This function specifically takes care of the first step and updates the reference locally. So the remaining steps
// can be executed afterwards.
// NOTE: This func has a side effect in case of template rotation, changing both the desired object and the object reference.
func (r *Reconciler) reconcileReferencedTemplate(ctx context.Context, in reconcileReferencedTemplateInput) error {
	log := tlog.LoggerFrom(ctx)

	// If there is no current object, create the desired object.
	if in.current == nil {
		log.Infof("Creating %s", tlog.KObj{Obj: in.desired})
		helper, err := r.patchHelperFactory(nil, in.desired)
		if err != nil {
			return errors.Wrap(createErrorWithoutObjectName(err, in.desired), "failed to create patch helper")
		}
		if err := helper.Patch(ctx); err != nil {
			return createErrorWithoutObjectName(err, in.desired)
		}
		r.recorder.Eventf(in.cluster, corev1.EventTypeNormal, createEventReason, "Created %q", tlog.KObj{Obj: in.desired})
		return nil
	}

	if in.ref == nil {
		return errors.Errorf("failed to rotate %s: ref should not be nil", in.desired.GroupVersionKind())
	}

	// Check if the current and desired referenced object are compatible.
	if allErrs := in.compatibilityChecker(in.current, in.desired); len(allErrs) > 0 {
		return allErrs.ToAggregate()
	}

	// Check differences between current and desired objects, and if there are changes eventually start the template rotation.
	patchHelper, err := r.patchHelperFactory(in.current, in.desired)
	if err != nil {
		return errors.Wrapf(err, "failed to create patch helper for %s", tlog.KObj{Obj: in.current})
	}

	// Return if no changes are detected.
	if !patchHelper.HasChanges() {
		log.V(3).Infof("No changes for %s", tlog.KObj{Obj: in.desired})
		return nil
	}

	// If there are no changes in the spec, and thus only changes in metadata, instead of doing a full template
	// rotation we patch the object in place. This avoids recreating machines.
	if !patchHelper.HasSpecChanges() {
		log.Infof("Patching %s", tlog.KObj{Obj: in.desired})
		if err := patchHelper.Patch(ctx); err != nil {
			return errors.Wrapf(err, "failed to patch %s", tlog.KObj{Obj: in.desired})
		}
		r.recorder.Eventf(in.cluster, corev1.EventTypeNormal, updateEventReason, "Updated %q (metadata changes)", tlog.KObj{Obj: in.desired})
		return nil
	}

	// Create the new template.

	// NOTE: it is required to assign a new name, because during compute the desired object name is enforced to be equal to the current one.
	// TODO: find a way to make side effect more explicit
	newName := names.SimpleNameGenerator.GenerateName(in.templateNamePrefix)
	in.desired.SetName(newName)

	log.Infof("Rotating %s, new name %s", tlog.KObj{Obj: in.current}, newName)
	log.Infof("Creating %s", tlog.KObj{Obj: in.desired})
	helper, err := r.patchHelperFactory(nil, in.desired)
	if err != nil {
		return errors.Wrap(createErrorWithoutObjectName(err, in.desired), "failed to create patch helper")
	}
	if err := helper.Patch(ctx); err != nil {
		return createErrorWithoutObjectName(err, in.desired)
	}
	r.recorder.Eventf(in.cluster, corev1.EventTypeNormal, createEventReason, "Created %q as a replacement for %q (template rotation)", tlog.KObj{Obj: in.desired}, in.ref.Name)

	// Update the reference with the new name.
	// NOTE: Updating the object hosting reference to the template is executed outside this func.
	// TODO: find a way to make side effect more explicit
	in.ref.Name = newName

	return nil
}

// createErrorWithoutObjectName removes the name of the object from the error message. As each new Create call involves an
// object with a unique generated name each error appears to be a different error. As the errors are being surfaced in a condition
// on the Cluster, the name is removed here to prevent each creation error from triggering a new reconciliation.
func createErrorWithoutObjectName(err error, obj client.Object) error {
	var statusError *apierrors.StatusError
	if errors.As(err, &statusError) {
		if statusError.Status().Details != nil {
			var causes []string
			for _, cause := range statusError.Status().Details.Causes {
				causes = append(causes, fmt.Sprintf("%s: %s: %s", cause.Type, cause.Field, cause.Message))
			}
			var msg string
			if len(causes) > 0 {
				msg = fmt.Sprintf("failed to create %s.%s: %s", statusError.Status().Details.Kind, statusError.Status().Details.Group, strings.Join(causes, " "))
			} else {
				msg = fmt.Sprintf("failed to create %s.%s", statusError.Status().Details.Kind, statusError.Status().Details.Group)
			}
			// Replace the statusError message with the constructed message.
			statusError.ErrStatus.Message = msg
			return statusError
		}
	}
	// If this isn't a StatusError return a more generic error with the object details.
	return errors.Wrapf(err, "failed to create %s", tlog.KObj{Obj: obj})
}
