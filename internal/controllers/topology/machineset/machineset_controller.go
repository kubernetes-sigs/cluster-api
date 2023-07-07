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

package machineset

import (
	"context"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	tlog "sigs.k8s.io/cluster-api/internal/log"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	clog "sigs.k8s.io/cluster-api/util/log"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
)

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io;bootstrap.cluster.x-k8s.io,resources=*,verbs=delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters,verbs=get;list;watch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinedeployments,verbs=get;list;watch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinesets;machinesets/finalizers,verbs=get;list;watch;update;patch

// Reconciler deletes referenced templates during deletion of topology-owned MachineSets.
// The templates are only deleted, if they are not used in other MachineDeployments or MachineSets which are not in deleting state,
// i.e. the templates would otherwise be orphaned after the MachineSet deletion completes.
// Note: To achieve this the reconciler sets a finalizer to hook into the MachineSet deletions.
type Reconciler struct {
	Client client.Client
	// APIReader is used to list MachineSets directly via the API server to avoid
	// race conditions caused by an outdated cache.
	APIReader        client.Reader
	WatchFilterValue string
}

func (r *Reconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	err := ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.MachineSet{}).
		Named("topology/machineset").
		WithOptions(options).
		WithEventFilter(predicates.All(ctrl.LoggerFrom(ctx),
			predicates.ResourceNotPausedAndHasFilterLabel(ctrl.LoggerFrom(ctx), r.WatchFilterValue),
			predicates.ResourceIsTopologyOwned(ctrl.LoggerFrom(ctx)),
		)).
		Complete(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	return nil
}

// Reconcile deletes referenced templates during deletion of topology-owned MachineSets.
// The templates are only deleted, if they are not used in other MachineDeployments or MachineSets which are not in deleting state,
// i.e. the templates would otherwise be orphaned after the MachineSet deletion completes.
// Additional context:
// * MachineSet deletion:
//   - MachineSets are deleted and garbage collected first (without waiting until all Machines are also deleted)
//   - After that, deletion of Machines is automatically triggered by Kubernetes based on owner references.
//
// Note: We assume templates are not reused by different MachineDeployments, which is (only) true for topology-owned
//
//	MachineDeployments.
//
// We don't have to set the finalizer, as it's already set during MachineSet creation
// in the MachineSet controller.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	// Fetch the MachineSet instance.
	ms := &clusterv1.MachineSet{}
	if err := r.Client.Get(ctx, req.NamespacedName, ms); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, errors.Wrapf(err, "failed to get MachineSet/%s", req.NamespacedName.Name)
	}

	// AddOwners adds the owners of MachineSet as k/v pairs to the logger.
	// Specifically, it will add MachineDeployment.
	ctx, log, err := clog.AddOwners(ctx, r.Client, ms)
	if err != nil {
		return ctrl.Result{}, err
	}

	log = log.WithValues("Cluster", klog.KRef(ms.Namespace, ms.Spec.ClusterName))
	ctx = ctrl.LoggerInto(ctx, log)

	cluster, err := util.GetClusterByName(ctx, r.Client, ms.Namespace, ms.Spec.ClusterName)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Return early if the object or Cluster is paused.
	if annotations.IsPaused(cluster, ms) {
		log.Info("Reconciliation is paused for this object")
		return ctrl.Result{}, nil
	}

	// Create a patch helper to add or remove the finalizer from the MachineSet.
	patchHelper, err := patch.NewHelper(ms, r.Client)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to create patch helper for %s", tlog.KObj{Obj: ms})
	}
	defer func() {
		if err := patchHelper.Patch(ctx, ms); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, errors.Wrapf(err, "failed to patch %s", tlog.KObj{Obj: ms})})
		}
	}()

	// Handle deletion reconciliation loop.
	if !ms.ObjectMeta.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, r.reconcileDelete(ctx, ms)
	}

	// Add finalizer first if not set to avoid the race condition between init and delete.
	// Note: Finalizers in general can only be added when the deletionTimestamp is not set.
	if !controllerutil.ContainsFinalizer(ms, clusterv1.MachineSetTopologyFinalizer) {
		controllerutil.AddFinalizer(ms, clusterv1.MachineSetTopologyFinalizer)
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

// reconcileDelete deletes templates referenced in a MachineSet, if the templates are not used by other
// MachineDeployments or MachineSets.
func (r *Reconciler) reconcileDelete(ctx context.Context, ms *clusterv1.MachineSet) error {
	// Gets the name of the MachineDeployment that controls this MachineSet.
	mdName, err := getMachineDeploymentName(ms)
	if err != nil {
		return err
	}

	// Get all the MachineSets for the MachineDeployment.
	msList, err := GetMachineSetsForDeployment(ctx, r.APIReader, *mdName)
	if err != nil {
		return err
	}

	// Fetch the MachineDeployment instance, if it still exists.
	// Note: This can happen because MachineDeployments are deleted before their corresponding MachineSets.
	md := &clusterv1.MachineDeployment{}
	if err := r.Client.Get(ctx, *mdName, md); err != nil {
		if !apierrors.IsNotFound(err) {
			// Error reading the object - requeue the request.
			return errors.Wrapf(err, "failed to get MachineDeployment/%s", mdName.Name)
		}
		// If the MachineDeployment doesn't exist anymore, set md to nil, so we can handle that case correctly below.
		md = nil
	}

	// Calculate which templates are still in use by MachineDeployments or MachineSets which are not in deleting state.
	templatesInUse, err := CalculateTemplatesInUse(md, msList)
	if err != nil {
		return err
	}

	// Delete unused templates.
	ref := ms.Spec.Template.Spec.Bootstrap.ConfigRef
	if err := DeleteTemplateIfUnused(ctx, r.Client, templatesInUse, ref); err != nil {
		return errors.Wrapf(err, "failed to delete bootstrap template for %s", tlog.KObj{Obj: ms})
	}
	ref = &ms.Spec.Template.Spec.InfrastructureRef
	if err := DeleteTemplateIfUnused(ctx, r.Client, templatesInUse, ref); err != nil {
		return errors.Wrapf(err, "failed to delete infrastructure template for %s", tlog.KObj{Obj: ms})
	}

	// Remove the finalizer so the MachineSet can be garbage collected by Kubernetes.
	controllerutil.RemoveFinalizer(ms, clusterv1.MachineSetTopologyFinalizer)

	return nil
}

// getMachineDeploymentName calculates the MachineDeployment name based on owner references.
func getMachineDeploymentName(ms *clusterv1.MachineSet) (*types.NamespacedName, error) {
	for _, ref := range ms.GetOwnerReferences() {
		if ref.Kind != "MachineDeployment" {
			continue
		}
		gv, err := schema.ParseGroupVersion(ref.APIVersion)
		if err != nil {
			return nil, errors.Errorf("could not calculate MachineDeployment name for %s: invalid apiVersion %q: %v",
				tlog.KObj{Obj: ms}, ref.APIVersion, err)
		}
		if gv.Group == clusterv1.GroupVersion.Group {
			return &client.ObjectKey{Namespace: ms.Namespace, Name: ref.Name}, nil
		}
	}

	// Note: Once we set an owner reference to a MachineDeployment in a MachineSet it stays there
	// and is not deleted when the MachineDeployment is deleted. So we assume there's something wrong,
	// if we couldn't find a MachineDeployment owner reference.
	return nil, errors.Errorf("could not calculate MachineDeployment name for %s", tlog.KObj{Obj: ms})
}
