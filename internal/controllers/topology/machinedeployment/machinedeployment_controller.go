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

package machinedeployment

import (
	"context"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/controllers/topology/machineset"
	tlog "sigs.k8s.io/cluster-api/internal/log"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
)

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io;bootstrap.cluster.x-k8s.io,resources=*,verbs=delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters,verbs=get;list;watch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinedeployments;machinedeployments/finalizers,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinesets,verbs=get;list;watch

// Reconciler deletes referenced templates during deletion of topology-owned MachineDeployments.
// The templates are only deleted, if they are not used in other MachineDeployments or MachineSets which are not in deleting state,
// i.e. the templates would otherwise be orphaned after the MachineDeployment deletion completes.
// Note: To achieve this the cluster topology controller sets a finalizer to hook into the MachineDeployment deletions.
type Reconciler struct {
	Client client.Client
	// APIReader is used to list MachineSets directly via the API server to avoid
	// race conditions caused by an outdated cache.
	APIReader        client.Reader
	WatchFilterValue string
}

func (r *Reconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	clusterToMachineDeployments, err := util.ClusterToTypedObjectsMapper(mgr.GetClient(), &clusterv1.MachineDeploymentList{}, mgr.GetScheme())
	if err != nil {
		return err
	}

	err = ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.MachineDeployment{},
			builder.WithPredicates(
				predicates.All(ctrl.LoggerFrom(ctx),
					predicates.ResourceIsTopologyOwned(ctrl.LoggerFrom(ctx)),
					predicates.ResourceNotPaused(ctrl.LoggerFrom(ctx))),
			),
		).
		Named("topology/machinedeployment").
		WithOptions(options).
		WithEventFilter(predicates.ResourceHasFilterLabel(ctrl.LoggerFrom(ctx), r.WatchFilterValue)).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(clusterToMachineDeployments),
			builder.WithPredicates(
				predicates.All(ctrl.LoggerFrom(ctx),
					predicates.ClusterUnpaused(ctrl.LoggerFrom(ctx)),
					predicates.ClusterHasTopology(ctrl.LoggerFrom(ctx)),
				),
			),
		).
		Complete(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	return nil
}

// Reconcile deletes referenced templates during deletion of topology-owned MachineDeployments.
// The templates are only deleted, if they are not used in other MachineDeployments or MachineSets which are not in deleting state,
// i.e. the templates would otherwise be orphaned after the MachineDeployment deletion completes.
// Additional context:
// * MachineDeployment deletion:
//   - MachineDeployments are deleted and garbage collected first (without waiting until all MachineSets are also deleted).
//   - After that, deletion of MachineSets is automatically triggered by Kubernetes based on owner references.
//
// Note: We assume templates are not reused by different MachineDeployments, which is only true for topology-owned
//
//	MachineDeployments.
//
// We don't have to set the finalizer, as it's already set during MachineDeployment creation
// in the cluster topology controller.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	log := ctrl.LoggerFrom(ctx)

	// Fetch the MachineDeployment instance.
	md := &clusterv1.MachineDeployment{}
	if err := r.Client.Get(ctx, req.NamespacedName, md); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, errors.Wrapf(err, "failed to get MachineDeployment/%s", req.NamespacedName.Name)
	}

	log = log.WithValues("Cluster", klog.KRef(md.Namespace, md.Spec.ClusterName))
	ctx = ctrl.LoggerInto(ctx, log)

	cluster, err := util.GetClusterByName(ctx, r.Client, md.Namespace, md.Spec.ClusterName)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Return early if the object or Cluster is paused.
	if annotations.IsPaused(cluster, md) {
		log.Info("Reconciliation is paused for this object")
		return ctrl.Result{}, nil
	}

	// Create a patch helper to add or remove the finalizer from the MachineDeployment.
	patchHelper, err := patch.NewHelper(md, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}
	defer func() {
		if err := patchHelper.Patch(ctx, md); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}
	}()

	// Handle deletion reconciliation loop.
	if !md.ObjectMeta.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, r.reconcileDelete(ctx, md)
	}

	// Add finalizer first if not set to avoid the race condition between init and delete.
	// Note: Finalizers in general can only be added when the deletionTimestamp is not set.
	if !controllerutil.ContainsFinalizer(md, clusterv1.MachineDeploymentTopologyFinalizer) {
		controllerutil.AddFinalizer(md, clusterv1.MachineDeploymentTopologyFinalizer)
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

// reconcileDelete deletes templates referenced in a MachineDeployment, if the templates are not used by other
// MachineDeployments or MachineSets.
func (r *Reconciler) reconcileDelete(ctx context.Context, md *clusterv1.MachineDeployment) error {
	// Get the corresponding MachineSets.
	msList, err := machineset.GetMachineSetsForDeployment(ctx, r.APIReader, client.ObjectKeyFromObject(md))
	if err != nil {
		return err
	}

	// Calculate which templates are still in use by MachineDeployments or MachineSets which are not in deleting state.
	templatesInUse, err := machineset.CalculateTemplatesInUse(md, msList)
	if err != nil {
		return err
	}

	// Delete unused templates.
	ref := md.Spec.Template.Spec.Bootstrap.ConfigRef
	if err := machineset.DeleteTemplateIfUnused(ctx, r.Client, templatesInUse, ref); err != nil {
		return errors.Wrapf(err, "failed to delete bootstrap template for %s", tlog.KObj{Obj: md})
	}
	ref = &md.Spec.Template.Spec.InfrastructureRef
	if err := machineset.DeleteTemplateIfUnused(ctx, r.Client, templatesInUse, ref); err != nil {
		return errors.Wrapf(err, "failed to delete infrastructure template for %s", tlog.KObj{Obj: md})
	}

	// If the MachineDeployment has a MachineHealthCheck delete it.
	if err := r.deleteMachineHealthCheckForMachineDeployment(ctx, md); err != nil {
		return err
	}

	controllerutil.RemoveFinalizer(md, clusterv1.MachineDeploymentTopologyFinalizer)

	return nil
}

func (r *Reconciler) deleteMachineHealthCheckForMachineDeployment(ctx context.Context, md *clusterv1.MachineDeployment) error {
	// MachineHealthCheck will always share the name and namespace of the MachineDeployment which created it.
	// Create a barebones MachineHealthCheck with the MachineDeployment name and namespace and delete the object.

	mhc := &clusterv1.MachineHealthCheck{}
	mhc.SetName(md.Name)
	mhc.SetNamespace(md.Namespace)
	if err := r.Client.Delete(ctx, mhc); err != nil {
		// If there is no MachineHealthCheck associated with the machineDeployment return no error.
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to delete %s", tlog.KObj{Obj: mhc})
	}
	return nil
}
