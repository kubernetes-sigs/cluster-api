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
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	"sigs.k8s.io/cluster-api/exp/topology/scope"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/internal/controllers/topology/cluster/structuredmerge"
	"sigs.k8s.io/cluster-api/internal/hooks"
	"sigs.k8s.io/cluster-api/internal/topology/check"
	"sigs.k8s.io/cluster-api/internal/topology/clustershim"
	topologynames "sigs.k8s.io/cluster-api/internal/topology/names"
	"sigs.k8s.io/cluster-api/internal/topology/ownerrefs"
	"sigs.k8s.io/cluster-api/util"
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
	createdInfraCluster, errInfraCluster := r.reconcileInfrastructureCluster(ctx, s)
	if errInfraCluster != nil {
		return errInfraCluster
	}

	// Reconcile desired state of the ControlPlane object.
	createdControlPlane, errControlPlane := r.reconcileControlPlane(ctx, s)
	if errControlPlane != nil {
		// NOTE: report control plane error immediately only if we did not just create the infrastructure cluster; otherwise attempt reconcile cluster before returning.
		if !createdInfraCluster {
			return errControlPlane
		}

		// In this case (reconcileInfrastructureCluster reported creation of the infrastructure cluster object, reconcileControlPlane - which is expected to create the control plane object - failed),
		// if the creation of the control plane actually did not happen, blank out ControlPlaneRef from desired cluster.
		if s.Current.Cluster.Spec.ControlPlaneRef == nil && !createdControlPlane {
			s.Desired.Cluster.Spec.ControlPlaneRef = nil
		}
	}

	// Reconcile desired state of the Cluster object.
	errCluster := r.reconcileCluster(ctx, s)
	err := kerrors.NewAggregate([]error{errControlPlane, errCluster})
	if err != nil {
		return err
	}

	// Reconcile desired state of the MachineDeployment objects.
	if err := r.reconcileMachineDeployments(ctx, s); err != nil {
		return err
	}

	// Reconcile desired state of the MachinePool object and return.
	return r.reconcileMachinePools(ctx, s)
}

// Reconcile the Cluster shim, a temporary object used a mean to collect objects/templates
// that might be orphaned in case of errors during the remaining part of the reconcile process.
func (r *Reconciler) reconcileClusterShim(ctx context.Context, s *scope.Scope) error {
	shim := clustershim.New(s.Current.Cluster)

	// If we are going to create the InfrastructureCluster or the ControlPlane object, then
	// add a temporary cluster-shim object and use it as an additional owner.
	// This will ensure the objects will be garbage collected in case of errors in between
	// creating InfrastructureCluster/ControlPlane objects and updating the Cluster with the
	// references to above objects.
	if s.Current.InfrastructureCluster == nil || s.Current.ControlPlane.Object == nil {
		// Given that the cluster shim is a temporary object which is only modified
		// by this controller, it is not necessary to use the SSA patch helper.
		if err := r.Client.Create(ctx, shim); err != nil {
			if !apierrors.IsAlreadyExists(err) {
				return errors.Wrap(err, "failed to create the cluster shim object")
			}
			if err := r.Client.Get(ctx, client.ObjectKeyFromObject(shim), shim); err != nil {
				return errors.Wrapf(err, "failed to read the cluster shim object")
			}
		}

		// Enforce type meta back given that it gets blanked out by Get.
		shim.Kind = "Secret"
		shim.APIVersion = corev1.SchemeGroupVersion.String()

		// Add the shim as a temporary owner for the InfrastructureCluster.
		s.Desired.InfrastructureCluster.SetOwnerReferences(
			util.EnsureOwnerRef(s.Desired.InfrastructureCluster.GetOwnerReferences(),
				*ownerrefs.OwnerReferenceTo(shim, corev1.SchemeGroupVersion.WithKind("Secret")),
			),
		)

		// Add the shim as a temporary owner for the ControlPlane.
		s.Desired.ControlPlane.Object.SetOwnerReferences(
			util.EnsureOwnerRef(s.Desired.ControlPlane.Object.GetOwnerReferences(),
				*ownerrefs.OwnerReferenceTo(shim, corev1.SchemeGroupVersion.WithKind("Secret")),
			),
		)
	}

	// If the InfrastructureCluster and the ControlPlane objects have been already created
	// in previous reconciliation, check if they have already been reconciled by the ClusterController
	// by verifying the ownerReference for the Cluster is present.
	//
	// When the Cluster and the shim object are both owners,
	// it's safe for us to remove the shim and garbage collect any potential orphaned resource.
	if s.Current.InfrastructureCluster != nil && s.Current.ControlPlane.Object != nil {
		clusterOwnsAll := ownerrefs.HasOwnerReferenceFrom(s.Current.InfrastructureCluster, s.Current.Cluster) &&
			ownerrefs.HasOwnerReferenceFrom(s.Current.ControlPlane.Object, s.Current.Cluster)
		shimOwnsAtLeastOne := ownerrefs.HasOwnerReferenceFrom(s.Current.InfrastructureCluster, shim) ||
			ownerrefs.HasOwnerReferenceFrom(s.Current.ControlPlane.Object, shim)

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

func (r *Reconciler) callAfterHooks(ctx context.Context, s *scope.Scope) error {
	if err := r.callAfterControlPlaneInitialized(ctx, s); err != nil {
		return err
	}

	return r.callAfterClusterUpgrade(ctx, s)
}

func (r *Reconciler) callAfterControlPlaneInitialized(ctx context.Context, s *scope.Scope) error {
	// If the cluster topology is being created then track to intent to call the AfterControlPlaneInitialized hook so that we can call it later.
	if s.Current.Cluster.Spec.InfrastructureRef == nil && s.Current.Cluster.Spec.ControlPlaneRef == nil {
		if err := hooks.MarkAsPending(ctx, r.Client, s.Current.Cluster, runtimehooksv1.AfterControlPlaneInitialized); err != nil {
			return err
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
		// - Control plane is stable (not upgrading, not scaling, not about to upgrade)
		// - MachineDeployments/MachinePools are not currently upgrading
		// - MachineDeployments/MachinePools are not pending an upgrade
		// - MachineDeployments/MachinePools are not pending create
		if s.UpgradeTracker.ControlPlane.IsControlPlaneStable() && // Control Plane stable checks
			len(s.UpgradeTracker.MachineDeployments.UpgradingNames()) == 0 && // Machine deployments are not upgrading or not about to upgrade
			!s.UpgradeTracker.MachineDeployments.IsAnyPendingCreate() && // No MachineDeployments are pending create
			!s.UpgradeTracker.MachineDeployments.IsAnyPendingUpgrade() && // No MachineDeployments are pending an upgrade
			!s.UpgradeTracker.MachineDeployments.DeferredUpgrade() && // No MachineDeployments have deferred an upgrade
			len(s.UpgradeTracker.MachinePools.UpgradingNames()) == 0 && // Machine pools are not upgrading or not about to upgrade
			!s.UpgradeTracker.MachinePools.IsAnyPendingCreate() && // No MachinePools are pending create
			!s.UpgradeTracker.MachinePools.IsAnyPendingUpgrade() && // No MachinePools are pending an upgrade
			!s.UpgradeTracker.MachinePools.DeferredUpgrade() { // No MachinePools have deferred an upgrade
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
func (r *Reconciler) reconcileInfrastructureCluster(ctx context.Context, s *scope.Scope) (bool, error) {
	log := ctrl.LoggerFrom(ctx).WithValues(s.Desired.InfrastructureCluster.GetKind(), klog.KObj(s.Desired.InfrastructureCluster))
	ctx = ctrl.LoggerInto(ctx, log)

	ignorePaths, err := contract.InfrastructureCluster().IgnorePaths(s.Desired.InfrastructureCluster)
	if err != nil {
		return false, errors.Wrap(err, "failed to calculate ignore paths")
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
func (r *Reconciler) reconcileControlPlane(ctx context.Context, s *scope.Scope) (bool, error) {
	// If the ControlPlane has defined a current or desired MachineHealthCheck attempt to reconcile it.
	// MHC changes are not Kubernetes version dependent, therefore proceed with MHC reconciliation
	// even if the Control Plane is pending an upgrade.
	if s.Desired.ControlPlane.MachineHealthCheck != nil || s.Current.ControlPlane.MachineHealthCheck != nil {
		// Reconcile the current and desired state of the MachineHealthCheck.
		if err := r.reconcileMachineHealthCheck(ctx, s.Current.ControlPlane.MachineHealthCheck, s.Desired.ControlPlane.MachineHealthCheck); err != nil {
			return false, err
		}
	}

	// Return early if the control plane is pending an upgrade.
	// Do not reconcile the control plane yet to avoid updating the control plane while it is still pending a
	// version upgrade. This will prevent the control plane from performing a double rollout.
	if s.UpgradeTracker.ControlPlane.IsPendingUpgrade {
		return false, nil
	}
	// If the clusterClass mandates the controlPlane has infrastructureMachines, reconcile it.
	infrastructureMachineCleanupFunc := func() {}
	if s.Blueprint.HasControlPlaneInfrastructureMachine() {
		log := ctrl.LoggerFrom(ctx).WithValues(s.Desired.ControlPlane.InfrastructureMachineTemplate.GetKind(), klog.KObj(s.Desired.ControlPlane.InfrastructureMachineTemplate))
		ctx := ctrl.LoggerInto(ctx, log)

		cpInfraRef, err := contract.ControlPlane().MachineTemplate().InfrastructureRef().Get(s.Desired.ControlPlane.Object)
		if err != nil {
			return false, errors.Wrapf(err, "failed to reconcile %s %s", s.Desired.ControlPlane.InfrastructureMachineTemplate.GetKind(), klog.KObj(s.Desired.ControlPlane.InfrastructureMachineTemplate))
		}

		// Create or update the MachineInfrastructureTemplate of the control plane.
		createdInfrastructureTemplate, err := r.reconcileReferencedTemplate(ctx, reconcileReferencedTemplateInput{
			cluster:              s.Current.Cluster,
			ref:                  cpInfraRef,
			current:              s.Current.ControlPlane.InfrastructureMachineTemplate,
			desired:              s.Desired.ControlPlane.InfrastructureMachineTemplate,
			compatibilityChecker: check.ObjectsAreCompatible,
			templateNamePrefix:   topologynames.ControlPlaneInfrastructureMachineTemplateNamePrefix(s.Current.Cluster.Name),
		})
		if err != nil {
			return false, err
		}

		if createdInfrastructureTemplate {
			infrastructureMachineCleanupFunc = func() {
				// Best effort cleanup of the InfrastructureMachineTemplate;
				// If this fails, the object will be garbage collected when the cluster is deleted.
				if err := r.Client.Delete(ctx, s.Desired.ControlPlane.InfrastructureMachineTemplate); err != nil {
					log.Error(err, "WARNING! Failed to cleanup InfrastructureMachineTemplate for control plane while handling creation or update error. The object will be garbage collected when the cluster is deleted.")
				}
			}
		}

		// The controlPlaneObject.Spec.machineTemplate.infrastructureRef has to be updated in the desired object
		err = contract.ControlPlane().MachineTemplate().InfrastructureRef().Set(s.Desired.ControlPlane.Object, refToUnstructured(cpInfraRef))
		if err != nil {
			// Best effort cleanup of the InfrastructureMachineTemplate (only on creation).
			infrastructureMachineCleanupFunc()
			return false, errors.Wrapf(err, "failed to reconcile %s %s", s.Desired.ControlPlane.Object.GetKind(), klog.KObj(s.Desired.ControlPlane.Object))
		}
	}

	// Create or update the ControlPlaneObject for the ControlPlaneState.
	log := ctrl.LoggerFrom(ctx).WithValues(s.Desired.ControlPlane.Object.GetKind(), klog.KObj(s.Desired.ControlPlane.Object))
	ctx = ctrl.LoggerInto(ctx, log)
	created, err := r.reconcileReferencedObject(ctx, reconcileReferencedObjectInput{
		cluster:       s.Current.Cluster,
		current:       s.Current.ControlPlane.Object,
		desired:       s.Desired.ControlPlane.Object,
		versionGetter: contract.ControlPlane().Version().Get,
	})
	if err != nil {
		// Best effort cleanup of the InfrastructureMachineTemplate (only on creation).
		infrastructureMachineCleanupFunc()
		return created, err
	}

	// If the controlPlane has infrastructureMachines and the InfrastructureMachineTemplate has changed on this reconcile
	// delete the old template.
	// This is a best effort deletion only and may leak templates if an error occurs during reconciliation.
	if s.Blueprint.HasControlPlaneInfrastructureMachine() && s.Current.ControlPlane.InfrastructureMachineTemplate != nil {
		if s.Current.ControlPlane.InfrastructureMachineTemplate.GetName() != s.Desired.ControlPlane.InfrastructureMachineTemplate.GetName() {
			if err := r.Client.Delete(ctx, s.Current.ControlPlane.InfrastructureMachineTemplate); err != nil {
				return created, errors.Wrapf(err, "failed to delete old %s %s of %s %s",
					s.Current.ControlPlane.InfrastructureMachineTemplate.GetKind(),
					klog.KObj(s.Current.ControlPlane.InfrastructureMachineTemplate),
					s.Current.ControlPlane.Object.GetKind(),
					klog.KObj(s.Current.ControlPlane.Object),
				)
			}
		}
	}

	return created, nil
}

// reconcileMachineHealthCheck creates, updates, deletes or leaves untouched a MachineHealthCheck depending on the difference between the
// current state and the desired state.
func (r *Reconciler) reconcileMachineHealthCheck(ctx context.Context, current, desired *clusterv1.MachineHealthCheck) error {
	log := ctrl.LoggerFrom(ctx)

	// If a current MachineHealthCheck doesn't exist but there is a desired MachineHealthCheck attempt to create.
	if current == nil && desired != nil {
		log.Info("Creating MachineHealthCheck", "MachineHealthCheck", klog.KObj(desired))
		helper, err := r.patchHelperFactory(ctx, nil, desired)
		if err != nil {
			return errors.Wrapf(err, "failed to create patch helper for MachineHealthCheck %s", klog.KObj(desired))
		}
		if err := helper.Patch(ctx); err != nil {
			return errors.Wrapf(err, "failed to create MachineHealthCheck %s", klog.KObj(desired))
		}
		r.recorder.Eventf(desired, corev1.EventTypeNormal, createEventReason, "Created MachineHealthCheck %q", klog.KObj(desired))
		return nil
	}

	// If a current MachineHealthCheck exists but there is no desired MachineHealthCheck attempt to delete.
	if current != nil && desired == nil {
		log.Info("Deleting MachineHealthCheck", "MachineHealthCheck", klog.KObj(current))
		if err := r.Client.Delete(ctx, current); err != nil {
			// If the object to be deleted is not found don't throw an error.
			if !apierrors.IsNotFound(err) {
				return errors.Wrapf(err, "failed to delete MachineHealthCheck %s", klog.KObj(current))
			}
		}
		r.recorder.Eventf(current, corev1.EventTypeNormal, deleteEventReason, "Deleted MachineHealthCheck %q", klog.KObj(current))
		return nil
	}

	log = log.WithValues("MachineHealthCheck", klog.KObj(current))
	ctx = ctrl.LoggerInto(ctx, log)

	// Check differences between current and desired MachineHealthChecks, and patch if required.
	// NOTE: we want to be authoritative on the entire spec because the users are
	// expected to change MHC fields from the ClusterClass only.
	patchHelper, err := r.patchHelperFactory(ctx, current, desired)
	if err != nil {
		return errors.Wrapf(err, "failed to create patch helper for MachineHealthCheck %s", klog.KObj(current))
	}
	if !patchHelper.HasChanges() {
		log.V(3).Info("No changes for MachineHealthCheck")
		return nil
	}

	log.Info("Patching MachineHealthCheck")
	if err := patchHelper.Patch(ctx); err != nil {
		return errors.Wrapf(err, "failed to patch MachineHealthCheck %s", klog.KObj(current))
	}
	r.recorder.Eventf(current, corev1.EventTypeNormal, updateEventReason, "Updated MachineHealthCheck %q", klog.KObj(current))
	return nil
}

// reconcileCluster reconciles the desired state of the Cluster object.
// NOTE: this assumes reconcileInfrastructureCluster and reconcileControlPlane being already completed;
// most specifically, after a Cluster is created it is assumed that the reference to the InfrastructureCluster /
// ControlPlane objects should never change (only the content of the objects can change).
func (r *Reconciler) reconcileCluster(ctx context.Context, s *scope.Scope) error {
	log := ctrl.LoggerFrom(ctx)

	// Check differences between current and desired state, and eventually patch the current object.
	patchHelper, err := r.patchHelperFactory(ctx, s.Current.Cluster, s.Desired.Cluster)
	if err != nil {
		return errors.Wrapf(err, "failed to create patch helper for Cluster %s", klog.KObj(s.Current.Cluster))
	}
	if !patchHelper.HasChanges() {
		log.V(3).Info("No changes for Cluster")
		return nil
	}

	changes := patchHelper.Changes()
	if len(changes) == 0 {
		log.Info("Patching Cluster")
	} else {
		log.Info("Patching Cluster", "diff", string(changes))
	}
	if err := patchHelper.Patch(ctx); err != nil {
		return errors.Wrapf(err, "failed to patch Cluster %s", klog.KObj(s.Current.Cluster))
	}
	r.recorder.Eventf(s.Current.Cluster, corev1.EventTypeNormal, updateEventReason, "Updated Cluster %q", klog.KObj(s.Current.Cluster))

	// Wait until Cluster is updated in the cache.
	// Note: We have to do this because otherwise using a cached client in the Reconcile func could
	// return a stale state of the Cluster we just patched (because the cache might be stale).
	// Note: It is good enough to check that the resource version changed. Other controllers might have updated the
	// Cluster as well, but the combination of the patch call above without a conflict and a changed resource
	// version here guarantees that we see the changes of our own update.
	err = wait.PollUntilContextTimeout(ctx, 5*time.Millisecond, 5*time.Second, true, func(ctx context.Context) (bool, error) {
		key := client.ObjectKey{Namespace: s.Current.Cluster.GetNamespace(), Name: s.Current.Cluster.GetName()}
		cachedCluster := &clusterv1.Cluster{}
		if err := r.Client.Get(ctx, key, cachedCluster); err != nil {
			return false, err
		}
		return s.Current.Cluster.GetResourceVersion() != cachedCluster.GetResourceVersion(), nil
	})
	if err != nil {
		return errors.Wrapf(err, "failed waiting for Cluster %s to be updated in the cache after patch", klog.KObj(s.Current.Cluster))
	}
	return nil
}

// reconcileMachineDeployments reconciles the desired state of the MachineDeployment objects.
func (r *Reconciler) reconcileMachineDeployments(ctx context.Context, s *scope.Scope) error {
	diff := calculateMachineDeploymentDiff(s.Current.MachineDeployments, s.Desired.MachineDeployments)

	// Create MachineDeployments.
	if len(diff.toCreate) > 0 {
		// In current state we only got the MD list via a cached call.
		// As a consequence, in order to prevent the creation of duplicate MD due to stale reads,
		// we are now using a live client to double-check here that the MachineDeployment
		// to be created doesn't exist yet.
		currentMDTopologyNames, err := r.getCurrentMachineDeployments(ctx, s)
		if err != nil {
			return err
		}
		for _, mdTopologyName := range diff.toCreate {
			md := s.Desired.MachineDeployments[mdTopologyName]

			// Skip the MD creation if the MD already exists.
			if currentMDTopologyNames.Has(mdTopologyName) {
				log := ctrl.LoggerFrom(ctx).WithValues("MachineDeployment", klog.KObj(md.Object),
					"machineDeploymentTopology", mdTopologyName)
				log.V(3).Info(fmt.Sprintf("Skipping creation of MachineDeployment, because MachineDeployment for topology %s already exists (only considered creation because of stale cache)", mdTopologyName))
				continue
			}

			if err := r.createMachineDeployment(ctx, s, md); err != nil {
				return err
			}
		}
	}

	// Update MachineDeployments.
	for _, mdTopologyName := range diff.toUpdate {
		currentMD := s.Current.MachineDeployments[mdTopologyName]
		desiredMD := s.Desired.MachineDeployments[mdTopologyName]
		if err := r.updateMachineDeployment(ctx, s, mdTopologyName, currentMD, desiredMD); err != nil {
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

// getCurrentMachineDeployments gets the current list of MachineDeployments via the APIReader.
func (r *Reconciler) getCurrentMachineDeployments(ctx context.Context, s *scope.Scope) (sets.Set[string], error) {
	// TODO: We should consider using PartialObjectMetadataList here. Currently this doesn't work as our
	// implementation for topology dryrun doesn't support PartialObjectMetadataList.
	mdList := &clusterv1.MachineDeploymentList{}
	err := r.APIReader.List(ctx, mdList,
		client.MatchingLabels{
			clusterv1.ClusterNameLabel:          s.Current.Cluster.Name,
			clusterv1.ClusterTopologyOwnedLabel: "",
		},
		client.InNamespace(s.Current.Cluster.Namespace),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read MachineDeployments for managed topology")
	}

	currentMDs := sets.Set[string]{}
	for _, md := range mdList.Items {
		mdTopologyName, ok := md.ObjectMeta.Labels[clusterv1.ClusterTopologyMachineDeploymentNameLabel]
		if ok || mdTopologyName != "" {
			currentMDs.Insert(mdTopologyName)
		}
	}
	return currentMDs, nil
}

// createMachineDeployment creates a MachineDeployment and the corresponding Templates.
func (r *Reconciler) createMachineDeployment(ctx context.Context, s *scope.Scope, md *scope.MachineDeploymentState) error {
	// Do not create the MachineDeployment if it is marked as pending create.
	// This will also block MHC creation because creating the MHC without the corresponding
	// MachineDeployment is unnecessary.
	mdTopologyName, ok := md.Object.Labels[clusterv1.ClusterTopologyMachineDeploymentNameLabel]
	if !ok || mdTopologyName == "" {
		// Note: This is only an additional safety check and should not happen. The label will always be added when computing
		// the desired MachineDeployment.
		return errors.Errorf("new MachineDeployment is missing the %q label", clusterv1.ClusterTopologyMachineDeploymentNameLabel)
	}
	// Return early if the MachineDeployment is pending create.
	if s.UpgradeTracker.MachineDeployments.IsPendingCreate(mdTopologyName) {
		return nil
	}

	log := ctrl.LoggerFrom(ctx).WithValues(
		"MachineDeployment", klog.KObj(md.Object),
		"machineDeploymentTopology", mdTopologyName)
	ctx = ctrl.LoggerInto(ctx, log)
	cluster := s.Current.Cluster
	infraLog := log.WithValues(md.InfrastructureMachineTemplate.GetKind(), klog.KObj(md.InfrastructureMachineTemplate))
	infraCtx := ctrl.LoggerInto(ctx, infraLog)
	infrastructureMachineCleanupFunc := func() {}
	createdInfra, err := r.reconcileReferencedTemplate(infraCtx, reconcileReferencedTemplateInput{
		cluster: cluster,
		desired: md.InfrastructureMachineTemplate,
	})
	if err != nil {
		return errors.Wrapf(err, "failed to create %s", md.Object.Kind)
	}

	if createdInfra {
		infrastructureMachineCleanupFunc = func() {
			// Best effort cleanup of the InfrastructureMachineTemplate;
			// If this fails, the object will be garbage collected when the cluster is deleted.
			if err := r.Client.Delete(ctx, md.InfrastructureMachineTemplate); err != nil {
				infraLog.Error(err, "WARNING! Failed to cleanup InfrastructureMachineTemplate for MachineDeployment while handling creation error. The object will be garbage collected when the cluster is deleted.")
			}
		}
	}

	bootstrapLog := log.WithValues(md.BootstrapTemplate.GetKind(), klog.KObj(md.BootstrapTemplate))
	bootstrapCtx := ctrl.LoggerInto(ctx, bootstrapLog)
	bootstrapCleanupFunc := func() {}
	createdBootstrap, err := r.reconcileReferencedTemplate(bootstrapCtx, reconcileReferencedTemplateInput{
		cluster: cluster,
		desired: md.BootstrapTemplate,
	})
	if err != nil {
		// Best effort cleanup of the InfrastructureMachineTemplate (only on creation).
		infrastructureMachineCleanupFunc()
		return errors.Wrapf(err, "failed to create %s", md.Object.Kind)
	}

	if createdBootstrap {
		bootstrapCleanupFunc = func() {
			// Best effort cleanup of the BootstrapTemplate;
			// If this fails, the object will be garbage collected when the cluster is deleted.
			if err := r.Client.Delete(ctx, md.BootstrapTemplate); err != nil {
				bootstrapLog.Error(err, "WARNING! Failed to cleanup BootstrapTemplate for MachineDeployment while handling creation error. The object will be garbage collected when the cluster is deleted.")
			}
		}
	}

	log.Info("Creating MachineDeployment")
	helper, err := r.patchHelperFactory(ctx, nil, md.Object)
	if err != nil {
		// Best effort cleanup of the InfrastructureMachineTemplate & BootstrapTemplate (only on creation).
		infrastructureMachineCleanupFunc()
		bootstrapCleanupFunc()
		return createErrorWithoutObjectName(ctx, err, md.Object)
	}
	if err := helper.Patch(ctx); err != nil {
		// Best effort cleanup of the InfrastructureMachineTemplate & BootstrapTemplate (only on creation).
		infrastructureMachineCleanupFunc()
		bootstrapCleanupFunc()
		return createErrorWithoutObjectName(ctx, err, md.Object)
	}
	r.recorder.Eventf(cluster, corev1.EventTypeNormal, createEventReason, "Created MachineDeployment %q", klog.KObj(md.Object))

	// Wait until MachineDeployment is visible in the cache.
	// Note: We have to do this because otherwise using a cached client in current state could
	// miss a newly created MachineDeployment (because the cache might be stale).
	err = wait.PollUntilContextTimeout(ctx, 5*time.Millisecond, 5*time.Second, true, func(ctx context.Context) (bool, error) {
		key := client.ObjectKey{Namespace: md.Object.Namespace, Name: md.Object.Name}
		if err := r.Client.Get(ctx, key, &clusterv1.MachineDeployment{}); err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	})
	if err != nil {
		return errors.Wrapf(err, "failed waiting for MachineDeployment %s to be visible in the cache after create", md.Object.Kind)
	}

	// If the MachineDeployment has defined a MachineHealthCheck reconcile it.
	if md.MachineHealthCheck != nil {
		if err := r.reconcileMachineHealthCheck(ctx, nil, md.MachineHealthCheck); err != nil {
			return err
		}
	}
	return nil
}

// updateMachineDeployment updates a MachineDeployment. Also rotates the corresponding Templates if necessary.
func (r *Reconciler) updateMachineDeployment(ctx context.Context, s *scope.Scope, mdTopologyName string, currentMD, desiredMD *scope.MachineDeploymentState) error {
	log := ctrl.LoggerFrom(ctx).WithValues("MachineDeployment", klog.KObj(desiredMD.Object),
		"machineDeploymentTopology", mdTopologyName)

	// Patch MachineHealthCheck for the MachineDeployment.
	// MHC changes are not Kubernetes version dependent, therefore proceed with MHC reconciliation
	// even if the MachineDeployment is pending an upgrade.
	if desiredMD.MachineHealthCheck != nil || currentMD.MachineHealthCheck != nil {
		if err := r.reconcileMachineHealthCheck(ctx, currentMD.MachineHealthCheck, desiredMD.MachineHealthCheck); err != nil {
			return err
		}
	}

	if !currentMD.Object.DeletionTimestamp.IsZero() {
		return nil
	}

	// Return early if the MachineDeployment is pending an upgrade.
	// Do not reconcile the MachineDeployment yet to avoid updating the MachineDeployment while it is still pending a
	// version upgrade. This will prevent the MachineDeployment from performing a double rollout.
	if s.UpgradeTracker.MachineDeployments.IsPendingUpgrade(currentMD.Object.Name) {
		return nil
	}

	cluster := s.Current.Cluster
	infraLog := log.WithValues(desiredMD.InfrastructureMachineTemplate.GetKind(), klog.KObj(desiredMD.InfrastructureMachineTemplate))
	infraCtx := ctrl.LoggerInto(ctx, infraLog)
	infrastructureMachineCleanupFunc := func() {}
	createdInfra, err := r.reconcileReferencedTemplate(infraCtx, reconcileReferencedTemplateInput{
		cluster:              cluster,
		ref:                  &desiredMD.Object.Spec.Template.Spec.InfrastructureRef,
		current:              currentMD.InfrastructureMachineTemplate,
		desired:              desiredMD.InfrastructureMachineTemplate,
		templateNamePrefix:   topologynames.InfrastructureMachineTemplateNamePrefix(cluster.Name, mdTopologyName),
		compatibilityChecker: check.ObjectsAreCompatible,
	})
	if err != nil {
		return errors.Wrapf(err, "failed to reconcile MachineDeployment %s", klog.KObj(currentMD.Object))
	}

	if createdInfra {
		infrastructureMachineCleanupFunc = func() {
			// Best effort cleanup of the InfrastructureMachineTemplate;
			// If this fails, the object will be garbage collected when the cluster is deleted.
			if err := r.Client.Delete(ctx, desiredMD.InfrastructureMachineTemplate); err != nil {
				infraLog.Error(err, "WARNING! Failed to cleanup InfrastructureMachineTemplate for MachineDeployment while handling update error. The object will be garbage collected when the cluster is deleted.")
			}
		}
	}

	bootstrapLog := log.WithValues(desiredMD.BootstrapTemplate.GetKind(), klog.KObj(desiredMD.BootstrapTemplate))
	bootstrapCtx := ctrl.LoggerInto(ctx, bootstrapLog)
	bootstrapCleanupFunc := func() {}
	createdBootstrap, err := r.reconcileReferencedTemplate(bootstrapCtx, reconcileReferencedTemplateInput{
		cluster:              cluster,
		ref:                  desiredMD.Object.Spec.Template.Spec.Bootstrap.ConfigRef,
		current:              currentMD.BootstrapTemplate,
		desired:              desiredMD.BootstrapTemplate,
		templateNamePrefix:   topologynames.BootstrapTemplateNamePrefix(cluster.Name, mdTopologyName),
		compatibilityChecker: check.ObjectsAreInTheSameNamespace,
	})
	if err != nil {
		// Best effort cleanup of the InfrastructureMachineTemplate (only on template rotation).
		infrastructureMachineCleanupFunc()
		return errors.Wrapf(err, "failed to reconcile MachineDeployment %s", klog.KObj(currentMD.Object))
	}

	if createdBootstrap {
		bootstrapCleanupFunc = func() {
			// Best effort cleanup of the BootstrapTemplate;
			// If this fails, the object will be garbage collected when the cluster is deleted.
			if err := r.Client.Delete(ctx, desiredMD.BootstrapTemplate); err != nil {
				bootstrapLog.Error(err, "WARNING! Failed to cleanup BootstrapTemplate for MachineDeployment while handling update error. The object will be garbage collected when the cluster is deleted.")
			}
		}
	}

	// Check differences between current and desired MachineDeployment, and eventually patch the current object.
	patchHelper, err := r.patchHelperFactory(ctx, currentMD.Object, desiredMD.Object)
	if err != nil {
		// Best effort cleanup of the InfrastructureMachineTemplate & BootstrapTemplate (only on template rotation).
		infrastructureMachineCleanupFunc()
		bootstrapCleanupFunc()
		return errors.Wrapf(err, "failed to create patch helper for MachineDeployment %s", klog.KObj(currentMD.Object))
	}
	if !patchHelper.HasChanges() {
		log.V(3).Info("No changes for MachineDeployment")
		return nil
	}

	changes := patchHelper.Changes()
	if len(changes) == 0 {
		log.Info("Patching MachineDeployment")
	} else {
		log.Info("Patching MachineDeployment", "diff", string(changes))
	}
	if err := patchHelper.Patch(ctx); err != nil {
		// Best effort cleanup of the InfrastructureMachineTemplate & BootstrapTemplate (only on template rotation).
		infrastructureMachineCleanupFunc()
		bootstrapCleanupFunc()
		return errors.Wrapf(err, "failed to patch MachineDeployment %s", klog.KObj(currentMD.Object))
	}
	r.recorder.Eventf(cluster, corev1.EventTypeNormal, updateEventReason, "Updated MachineDeployment %q%s", klog.KObj(currentMD.Object), logMachineDeploymentVersionChange(currentMD.Object, desiredMD.Object))

	// Wait until MachineDeployment is updated in the cache.
	// Note: We have to do this because otherwise using a cached client in current state could
	// return a stale state of a MachineDeployment we just patched (because the cache might be stale).
	// Note: It is good enough to check that the resource version changed. Other controllers might have updated the
	// MachineDeployment as well, but the combination of the patch call above without a conflict and a changed resource
	// version here guarantees that we see the changes of our own update.
	err = wait.PollUntilContextTimeout(ctx, 5*time.Millisecond, 5*time.Second, true, func(ctx context.Context) (bool, error) {
		key := client.ObjectKey{Namespace: currentMD.Object.GetNamespace(), Name: currentMD.Object.GetName()}
		cachedMD := &clusterv1.MachineDeployment{}
		if err := r.Client.Get(ctx, key, cachedMD); err != nil {
			return false, err
		}
		return currentMD.Object.GetResourceVersion() != cachedMD.GetResourceVersion(), nil
	})
	if err != nil {
		return errors.Wrapf(err, "failed waiting for MachineDeployment %s to be updated in the cache after patch", klog.KObj(currentMD.Object))
	}

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
	log := ctrl.LoggerFrom(ctx).WithValues(
		"MachineDeployment", klog.KObj(md.Object),
		"machineDeploymentTopology", md.Object.Labels[clusterv1.ClusterTopologyMachineDeploymentNameLabel])

	// delete MachineHealthCheck for the MachineDeployment.
	if md.MachineHealthCheck != nil {
		if err := r.reconcileMachineHealthCheck(ctx, md.MachineHealthCheck, nil); err != nil {
			return err
		}
	}
	if md.Object.DeletionTimestamp.IsZero() {
		log.Info("Deleting MachineDeployment")
		if err := r.Client.Delete(ctx, md.Object); err != nil && !apierrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to delete MachineDeployment %s", klog.KObj(md.Object))
		}
		r.recorder.Eventf(cluster, corev1.EventTypeNormal, deleteEventReason, "Deleted MachineDeployment %q", klog.KObj(md.Object))
	}
	return nil
}

// reconcileMachinePools reconciles the desired state of the MachinePool objects.
func (r *Reconciler) reconcileMachinePools(ctx context.Context, s *scope.Scope) error {
	diff := calculateMachinePoolDiff(s.Current.MachinePools, s.Desired.MachinePools)

	// Create MachinePools.
	if len(diff.toCreate) > 0 {
		// In current state we only got the MP list via a cached call.
		// As a consequence, in order to prevent the creation of duplicate MP due to stale reads,
		// we are now using a live client to double-check here that the MachinePool
		// to be created doesn't exist yet.
		currentMPTopologyNames, err := r.getCurrentMachinePools(ctx, s)
		if err != nil {
			return err
		}
		for _, mpTopologyName := range diff.toCreate {
			mp := s.Desired.MachinePools[mpTopologyName]

			// Skip the MP creation if the MP already exists.
			if currentMPTopologyNames.Has(mpTopologyName) {
				log := ctrl.LoggerFrom(ctx).WithValues("MachinePool", klog.KObj(mp.Object),
					"machinePoolTopology", mpTopologyName)
				log.V(3).Info(fmt.Sprintf("Skipping creation of MachinePool, because MachinePool for topology %s already exists (only considered creation because of stale cache)", mpTopologyName))
				continue
			}

			if err := r.createMachinePool(ctx, s, mp); err != nil {
				return err
			}
		}
	}

	// Update MachinePools.
	for _, mpTopologyName := range diff.toUpdate {
		currentMP := s.Current.MachinePools[mpTopologyName]
		desiredMP := s.Desired.MachinePools[mpTopologyName]
		if err := r.updateMachinePool(ctx, s, mpTopologyName, currentMP, desiredMP); err != nil {
			return err
		}
	}

	// Delete MachinePools.
	for _, mpTopologyName := range diff.toDelete {
		mp := s.Current.MachinePools[mpTopologyName]
		if err := r.deleteMachinePool(ctx, s.Current.Cluster, mp); err != nil {
			return err
		}
	}

	return nil
}

// getCurrentMachinePools gets the current list of MachinePools via the APIReader.
func (r *Reconciler) getCurrentMachinePools(ctx context.Context, s *scope.Scope) (sets.Set[string], error) {
	// TODO: We should consider using PartialObjectMetadataList here. Currently this doesn't work as our
	// implementation for topology dryrun doesn't support PartialObjectMetadataList.
	mpList := &expv1.MachinePoolList{}
	err := r.APIReader.List(ctx, mpList,
		client.MatchingLabels{
			clusterv1.ClusterNameLabel:          s.Current.Cluster.Name,
			clusterv1.ClusterTopologyOwnedLabel: "",
		},
		client.InNamespace(s.Current.Cluster.Namespace),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read MachinePools for managed topology")
	}

	currentMPs := sets.Set[string]{}
	for _, mp := range mpList.Items {
		mpTopologyName, ok := mp.ObjectMeta.Labels[clusterv1.ClusterTopologyMachinePoolNameLabel]
		if ok || mpTopologyName != "" {
			currentMPs.Insert(mpTopologyName)
		}
	}
	return currentMPs, nil
}

// createMachinePool creates a MachinePool and the corresponding templates.
func (r *Reconciler) createMachinePool(ctx context.Context, s *scope.Scope, mp *scope.MachinePoolState) error {
	// Do not create the MachinePool if it is marked as pending create.
	mpTopologyName, ok := mp.Object.Labels[clusterv1.ClusterTopologyMachinePoolNameLabel]
	if !ok || mpTopologyName == "" {
		// Note: This is only an additional safety check and should not happen. The label will always be added when computing
		// the desired MachinePool.
		return errors.Errorf("new MachinePool is missing the %q label", clusterv1.ClusterTopologyMachinePoolNameLabel)
	}
	// Return early if the MachinePool is pending create.
	if s.UpgradeTracker.MachinePools.IsPendingCreate(mpTopologyName) {
		return nil
	}

	log := ctrl.LoggerFrom(ctx).WithValues(
		"MachinePool", klog.KObj(mp.Object),
		"machinePoolTopology", mpTopologyName)
	ctx = ctrl.LoggerInto(ctx, log)
	cluster := s.Current.Cluster
	infraLog := log.WithValues(mp.InfrastructureMachinePoolObject.GetKind(), klog.KObj(mp.InfrastructureMachinePoolObject))
	infraCtx := ctrl.LoggerInto(ctx, infraLog)
	infrastructureMachineMachinePoolCleanupFunc := func() {}
	createdInfrastructureMachinePool, err := r.reconcileReferencedObject(infraCtx, reconcileReferencedObjectInput{
		cluster: cluster,
		desired: mp.InfrastructureMachinePoolObject,
	})
	if err != nil {
		return errors.Wrapf(err, "failed to create MachinePool %s", klog.KObj(mp.Object))
	}

	if createdInfrastructureMachinePool {
		infrastructureMachineMachinePoolCleanupFunc = func() {
			// Best effort cleanup of the InfrastructureMachinePool;
			// If this fails, the object will be garbage collected when the cluster is deleted.
			if err := r.Client.Delete(ctx, mp.InfrastructureMachinePoolObject); err != nil {
				infraLog.Info("WARNING! Failed to cleanup InfrastructureMachinePoolObject for MachinePool while handling creation error. The object will be garbage collected when the cluster is deleted.")
			}
		}
	}

	bootstrapLog := log.WithValues(mp.BootstrapObject.GetKind(), klog.KObj(mp.BootstrapObject))
	bootstrapCtx := ctrl.LoggerInto(ctx, bootstrapLog)
	bootstrapCleanupFunc := func() {}
	createdBootstrap, err := r.reconcileReferencedObject(bootstrapCtx, reconcileReferencedObjectInput{
		cluster: cluster,
		desired: mp.BootstrapObject,
	})
	if err != nil {
		// Best effort cleanup of the InfrastructureMachinePool (only on creation).
		infrastructureMachineMachinePoolCleanupFunc()
		return errors.Wrapf(err, "failed to create MachinePool %s", mp.Object.Kind)
	}

	if createdBootstrap {
		bootstrapCleanupFunc = func() {
			// Best effort cleanup of the BootstrapConfig;
			// If this fails, the object will be garbage collected when the cluster is deleted.
			if err := r.Client.Delete(ctx, mp.BootstrapObject); err != nil {
				bootstrapLog.Error(err, "WARNING! Failed to cleanup BootstrapObject for MachinePool while handling creation error. The object will be garbage collected when the cluster is deleted.")
			}
		}
	}

	log.Info("Creating MachinePool")
	helper, err := r.patchHelperFactory(ctx, nil, mp.Object)
	if err != nil {
		// Best effort cleanup of the InfrastructureMachinePool & BootstrapConfig (only on creation).
		infrastructureMachineMachinePoolCleanupFunc()
		bootstrapCleanupFunc()
		return createErrorWithoutObjectName(ctx, err, mp.Object)
	}
	if err := helper.Patch(ctx); err != nil {
		// Best effort cleanup of the InfrastructureMachinePool & BootstrapConfig (only on creation).
		infrastructureMachineMachinePoolCleanupFunc()
		bootstrapCleanupFunc()
		return createErrorWithoutObjectName(ctx, err, mp.Object)
	}
	r.recorder.Eventf(cluster, corev1.EventTypeNormal, createEventReason, "Created MachinePool %q", klog.KObj(mp.Object))

	// Wait until MachinePool is visible in the cache.
	// Note: We have to do this because otherwise using a cached client in current state could
	// miss a newly created MachinePool (because the cache might be stale).
	err = wait.PollUntilContextTimeout(ctx, 5*time.Millisecond, 5*time.Second, true, func(ctx context.Context) (bool, error) {
		key := client.ObjectKey{Namespace: mp.Object.Namespace, Name: mp.Object.Name}
		if err := r.Client.Get(ctx, key, &expv1.MachinePool{}); err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	})
	if err != nil {
		return errors.Wrapf(err, "failed waiting for MachinePool %s to be visible in the cache after create", mp.Object.Kind)
	}

	return nil
}

// updateMachinePool updates a MachinePool. Also updates the corresponding objects if necessary.
func (r *Reconciler) updateMachinePool(ctx context.Context, s *scope.Scope, mpTopologyName string, currentMP, desiredMP *scope.MachinePoolState) error {
	log := ctrl.LoggerFrom(ctx).WithValues("MachinePool", klog.KObj(desiredMP.Object),
		"machinePoolTopology", mpTopologyName)

	// Return early if the MachinePool is pending an upgrade.
	// Do not reconcile the MachinePool yet to avoid updating the MachinePool while it is still pending a
	// version upgrade. This will prevent the MachinePool from performing a double rollout.
	if s.UpgradeTracker.MachinePools.IsPendingUpgrade(currentMP.Object.Name) {
		return nil
	}

	cluster := s.Current.Cluster
	infraCtx := ctrl.LoggerInto(ctx, log.WithValues(desiredMP.InfrastructureMachinePoolObject.GetKind(), klog.KObj(desiredMP.InfrastructureMachinePoolObject)))
	if _, err := r.reconcileReferencedObject(infraCtx, reconcileReferencedObjectInput{
		cluster: cluster,
		current: currentMP.InfrastructureMachinePoolObject,
		desired: desiredMP.InfrastructureMachinePoolObject,
	}); err != nil {
		return errors.Wrapf(err, "failed to reconcile MachinePool %s", klog.KObj(currentMP.Object))
	}

	bootstrapCtx := ctrl.LoggerInto(ctx, log.WithValues(desiredMP.BootstrapObject.GetKind(), klog.KObj(desiredMP.BootstrapObject)))
	if _, err := r.reconcileReferencedObject(bootstrapCtx, reconcileReferencedObjectInput{
		cluster: cluster,
		current: currentMP.BootstrapObject,
		desired: desiredMP.BootstrapObject,
	}); err != nil {
		return errors.Wrapf(err, "failed to reconcile MachinePool %s", klog.KObj(currentMP.Object))
	}

	// Check differences between current and desired MachinePool, and eventually patch the current object.
	patchHelper, err := r.patchHelperFactory(ctx, currentMP.Object, desiredMP.Object)
	if err != nil {
		return errors.Wrapf(err, "failed to create patch helper for MachinePool %s", klog.KObj(currentMP.Object))
	}
	if !patchHelper.HasChanges() {
		log.V(3).Info("No changes for MachinePool")
		return nil
	}

	changes := patchHelper.Changes()
	if len(changes) == 0 {
		log.Info("Patching MachinePool")
	} else {
		log.Info("Patching MachinePool", "diff", string(changes))
	}
	if err := patchHelper.Patch(ctx); err != nil {
		return errors.Wrapf(err, "failed to patch MachinePool %s", klog.KObj(currentMP.Object))
	}
	r.recorder.Eventf(cluster, corev1.EventTypeNormal, updateEventReason, "Updated MachinePool %q%s", klog.KObj(currentMP.Object), logMachinePoolVersionChange(currentMP.Object, desiredMP.Object))

	// Wait until MachinePool is updated in the cache.
	// Note: We have to do this because otherwise using a cached client in current state could
	// return a stale state of a MachinePool we just patched (because the cache might be stale).
	// Note: It is good enough to check that the resource version changed. Other controllers might have updated the
	// MachinePool as well, but the combination of the patch call above without a conflict and a changed resource
	// version here guarantees that we see the changes of our own update.
	err = wait.PollUntilContextTimeout(ctx, 5*time.Millisecond, 5*time.Second, true, func(ctx context.Context) (bool, error) {
		key := client.ObjectKey{Namespace: currentMP.Object.GetNamespace(), Name: currentMP.Object.GetName()}
		cachedMP := &expv1.MachinePool{}
		if err := r.Client.Get(ctx, key, cachedMP); err != nil {
			return false, err
		}
		return currentMP.Object.GetResourceVersion() != cachedMP.GetResourceVersion(), nil
	})
	if err != nil {
		return errors.Wrapf(err, "failed waiting for MachinePool %s to be updated in the cache after patch", klog.KObj(currentMP.Object))
	}

	// We want to call both cleanup functions even if one of them fails to clean up as much as possible.
	return nil
}

func logMachinePoolVersionChange(current, desired *expv1.MachinePool) string {
	if current.Spec.Template.Spec.Version == nil || desired.Spec.Template.Spec.Version == nil {
		return ""
	}

	if *current.Spec.Template.Spec.Version != *desired.Spec.Template.Spec.Version {
		return fmt.Sprintf(" with version change from %s to %s", *current.Spec.Template.Spec.Version, *desired.Spec.Template.Spec.Version)
	}
	return ""
}

// deleteMachinePool deletes a MachinePool.
func (r *Reconciler) deleteMachinePool(ctx context.Context, cluster *clusterv1.Cluster, mp *scope.MachinePoolState) error {
	log := ctrl.LoggerFrom(ctx).WithValues(
		"MachinePool", klog.KObj(mp.Object),
		"machinePoolTopology", mp.Object.Labels[clusterv1.ClusterTopologyMachinePoolNameLabel])

	log.Info("Deleting MachinePool")
	if err := r.Client.Delete(ctx, mp.Object); err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrapf(err, "failed to delete MachinePool %s", klog.KObj(mp.Object))
	}
	r.recorder.Eventf(cluster, corev1.EventTypeNormal, deleteEventReason, "Deleted MachinePool %q", klog.KObj(mp.Object))
	return nil
}

type machineDiff struct {
	toCreate, toUpdate, toDelete []string
}

// calculateMachineDeploymentDiff compares two maps of MachineDeploymentState and calculates which
// MachineDeployments should be created, updated or deleted.
func calculateMachineDeploymentDiff(current, desired map[string]*scope.MachineDeploymentState) machineDiff {
	var diff machineDiff

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

// calculateMachinePoolDiff compares two maps of MachinePoolState and calculates which
// MachinePools should be created, updated or deleted.
func calculateMachinePoolDiff(current, desired map[string]*scope.MachinePoolState) machineDiff {
	var diff machineDiff

	for mp := range desired {
		if _, ok := current[mp]; ok {
			diff.toUpdate = append(diff.toUpdate, mp)
		} else {
			diff.toCreate = append(diff.toCreate, mp)
		}
	}

	for mp := range current {
		if _, ok := desired[mp]; !ok {
			diff.toDelete = append(diff.toDelete, mp)
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
// Returns true if the referencedObject is created.
// NOTE: After a referenced object is created it is assumed that the reference should
// never change (only the content of the object can eventually change). Thus, we are checking for strict compatibility.
func (r *Reconciler) reconcileReferencedObject(ctx context.Context, in reconcileReferencedObjectInput) (bool, error) {
	log := ctrl.LoggerFrom(ctx)

	// If there is no current object, create it.
	if in.current == nil {
		log.Info(fmt.Sprintf("Creating %s", in.desired.GetKind()), in.desired.GetKind(), klog.KObj(in.desired))
		helper, err := r.patchHelperFactory(ctx, nil, in.desired, structuredmerge.IgnorePaths(in.ignorePaths))
		if err != nil {
			return false, errors.Wrap(createErrorWithoutObjectName(ctx, err, in.desired), "failed to create patch helper")
		}
		if err := helper.Patch(ctx); err != nil {
			return false, createErrorWithoutObjectName(ctx, err, in.desired)
		}
		r.recorder.Eventf(in.cluster, corev1.EventTypeNormal, createEventReason, "Created %s %q", in.desired.GetKind(), klog.KObj(in.desired))
		return true, nil
	}

	// Check if the current and desired referenced object are compatible.
	if allErrs := check.ObjectsAreStrictlyCompatible(in.current, in.desired); len(allErrs) > 0 {
		return false, allErrs.ToAggregate()
	}

	log = log.WithValues(in.current.GetKind(), klog.KObj(in.current))
	ctx = ctrl.LoggerInto(ctx, log)

	// Check differences between current and desired state, and eventually patch the current object.
	patchHelper, err := r.patchHelperFactory(ctx, in.current, in.desired, structuredmerge.IgnorePaths(in.ignorePaths))
	if err != nil {
		return false, errors.Wrapf(err, "failed to create patch helper for %s %s", in.current.GetKind(), klog.KObj(in.current))
	}
	if !patchHelper.HasChanges() {
		log.V(3).Info(fmt.Sprintf("No changes for %s", in.desired.GetKind()))
		return false, nil
	}

	changes := patchHelper.Changes()
	if len(changes) == 0 {
		log.Info(fmt.Sprintf("Patching %s", in.desired.GetKind()))
	} else {
		log.Info(fmt.Sprintf("Patching %s", in.desired.GetKind()), "diff", string(changes))
	}
	if err := patchHelper.Patch(ctx); err != nil {
		return false, errors.Wrapf(err, "failed to patch %s %s", in.current.GetKind(), klog.KObj(in.current))
	}
	r.recorder.Eventf(in.cluster, corev1.EventTypeNormal, updateEventReason, "Updated %s %q%s", in.desired.GetKind(), klog.KObj(in.desired), logUnstructuredVersionChange(in.current, in.desired, in.versionGetter))
	return false, nil
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
// Returns true if the referencedTemplate is created.
// NOTE: According to Cluster API operational practices, when a referenced Template changes a template rotation is required:
// 1. create a new Template
// 2. update the reference
// 3. delete the old Template
// This function specifically takes care of the first step and updates the reference locally. So the remaining steps
// can be executed afterwards.
// NOTE: This func has a side effect in case of template rotation, changing both the desired object and the object reference.
func (r *Reconciler) reconcileReferencedTemplate(ctx context.Context, in reconcileReferencedTemplateInput) (bool, error) {
	log := ctrl.LoggerFrom(ctx)

	// If there is no current object, create the desired object.
	if in.current == nil {
		log.Info(fmt.Sprintf("Creating %s", in.desired.GetKind()), in.desired.GetKind(), klog.KObj(in.desired))
		helper, err := r.patchHelperFactory(ctx, nil, in.desired)
		if err != nil {
			return false, errors.Wrap(createErrorWithoutObjectName(ctx, err, in.desired), "failed to create patch helper")
		}
		if err := helper.Patch(ctx); err != nil {
			return false, createErrorWithoutObjectName(ctx, err, in.desired)
		}
		r.recorder.Eventf(in.cluster, corev1.EventTypeNormal, createEventReason, "Created %s %q", in.desired.GetKind(), klog.KObj(in.desired))
		return true, nil
	}

	if in.ref == nil {
		return false, errors.Errorf("failed to rotate %s: ref should not be nil", in.desired.GetKind())
	}

	// Check if the current and desired referenced object are compatible.
	if allErrs := in.compatibilityChecker(in.current, in.desired); len(allErrs) > 0 {
		return false, allErrs.ToAggregate()
	}

	log = log.WithValues(in.current.GetKind(), klog.KObj(in.current))
	ctx = ctrl.LoggerInto(ctx, log)

	// Check differences between current and desired objects, and if there are changes eventually start the template rotation.
	patchHelper, err := r.patchHelperFactory(ctx, in.current, in.desired)
	if err != nil {
		return false, errors.Wrapf(err, "failed to create patch helper for %s %s", in.current.GetKind(), klog.KObj(in.current))
	}

	// Return if no changes are detected.
	if !patchHelper.HasChanges() {
		log.V(3).Info(fmt.Sprintf("No changes for %s", in.desired.GetKind()))
		return false, nil
	}

	// If there are no changes in the spec, and thus only changes in metadata, instead of doing a full template
	// rotation we patch the object in place. This avoids recreating machines.
	if !patchHelper.HasSpecChanges() {
		changes := patchHelper.Changes()
		if len(changes) == 0 {
			log.Info(fmt.Sprintf("Patching %s", in.desired.GetKind()))
		} else {
			log.Info(fmt.Sprintf("Patching %s", in.desired.GetKind()), "diff", string(changes))
		}
		if err := patchHelper.Patch(ctx); err != nil {
			return false, errors.Wrapf(err, "failed to patch %s %s", in.desired.GetKind(), klog.KObj(in.desired))
		}
		r.recorder.Eventf(in.cluster, corev1.EventTypeNormal, updateEventReason, "Updated %s %q (metadata changes)", in.desired.GetKind(), klog.KObj(in.desired))
		return false, nil
	}

	// Create the new template.

	// NOTE: it is required to assign a new name, because during compute the desired object name is enforced to be equal to the current one.
	// TODO: find a way to make side effect more explicit
	newName := names.SimpleNameGenerator.GenerateName(in.templateNamePrefix)
	in.desired.SetName(newName)

	changes := patchHelper.Changes()
	if len(changes) == 0 {
		log.Info(fmt.Sprintf("Rotating %s, new name %s", in.current.GetKind(), newName))
	} else {
		log.Info(fmt.Sprintf("Rotating %s, new name %s", in.current.GetKind(), newName), "diff", string(changes))
	}
	log.Info(fmt.Sprintf("Creating %s", in.current.GetKind()))
	helper, err := r.patchHelperFactory(ctx, nil, in.desired)
	if err != nil {
		return false, errors.Wrap(createErrorWithoutObjectName(ctx, err, in.desired), "failed to create patch helper")
	}
	if err := helper.Patch(ctx); err != nil {
		return false, createErrorWithoutObjectName(ctx, err, in.desired)
	}
	r.recorder.Eventf(in.cluster, corev1.EventTypeNormal, createEventReason, "Created %s %q as a replacement for %q (template rotation)", in.desired.GetKind(), klog.KObj(in.desired), in.ref.Name)

	// Update the reference with the new name.
	// NOTE: Updating the object hosting reference to the template is executed outside this func.
	// TODO: find a way to make side effect more explicit
	in.ref.Name = newName

	return true, nil
}

// createErrorWithoutObjectName removes the name of the object from the error message. As each new Create call involves an
// object with a unique generated name each error appears to be a different error. As the errors are being surfaced in a condition
// on the Cluster, the name is removed here to prevent each creation error from triggering a new reconciliation.
func createErrorWithoutObjectName(ctx context.Context, err error, obj client.Object) error {
	log := ctrl.LoggerFrom(ctx)
	if obj != nil {
		log = log.WithValues(obj.GetObjectKind().GroupVersionKind().Kind, klog.KObj(obj))
	}
	log.Error(err, "Failed to create object")

	var statusError *apierrors.StatusError
	if errors.As(err, &statusError) {
		var msg string
		if statusError.Status().Details != nil {
			var causes []string
			for _, cause := range statusError.Status().Details.Causes {
				causes = append(causes, fmt.Sprintf("%s: %s: %s", cause.Type, cause.Field, cause.Message))
			}
			if len(causes) > 0 {
				msg = fmt.Sprintf("failed to create %s.%s: %s", statusError.Status().Details.Kind, statusError.Status().Details.Group, strings.Join(causes, " "))
			} else {
				msg = fmt.Sprintf("failed to create %s.%s", statusError.Status().Details.Kind, statusError.Status().Details.Group)
			}
			statusError.ErrStatus.Message = msg
			return statusError
		}

		if statusError.Status().Message != "" {
			if obj != nil {
				msg = fmt.Sprintf("failed to create %s", obj.GetObjectKind().GroupVersionKind().GroupKind().String())
			} else {
				msg = "failed to create object"
			}
		}
		statusError.ErrStatus.Message = msg
		return statusError
	}
	// If this isn't a StatusError return a more generic error with the object details.
	if obj != nil {
		return errors.Errorf("failed to create %s", obj.GetObjectKind().GroupVersionKind().GroupKind().String())
	}
	return errors.New("failed to create object")
}
