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

package topology

import (
	"context"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	tlog "sigs.k8s.io/cluster-api/controllers/topology/internal/log"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io;bootstrap.cluster.x-k8s.io,resources=*,verbs=delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters,verbs=get;list;watch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinedeployments;machinedeployments/finalizers,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinesets,verbs=get;list;watch

// MachineDeploymentReconciler deletes referenced templates during deletion of topology-owned MachineDeployments.
// The templates are only deleted, if they are not used in other MachineDeployments or MachineSets which are not in deleting state,
// i.e. the templates would otherwise be orphaned after the MachineDeployment deletion completes.
// Note: To achieve this the cluster topology controller sets a finalizer to hook into the MachineDeployment deletions.
type MachineDeploymentReconciler struct {
	Client client.Client
	// APIReader is used to list MachineSets directly via the API server to avoid
	// race conditions caused by an outdated cache.
	APIReader        client.Reader
	WatchFilterValue string
}

func (r *MachineDeploymentReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	err := ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.MachineDeployment{}).
		Named("topology/machinedeployment").
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

// Reconcile deletes referenced templates during deletion of topology-owned MachineDeployments.
// The templates are only deleted, if they are not used in other MachineDeployments or MachineSets which are not in deleting state,
// i.e. the templates would otherwise be orphaned after the MachineDeployment deletion completes.
// Additional context:
// * MachineDeployment deletion:
//   * MachineDeployments are deleted and garbage collected first (without waiting until all MachineSets are also deleted).
//   * After that, deletion of MachineSets is automatically triggered by Kubernetes based on owner references.
// Note: We assume templates are not reused by different MachineDeployments, which is only true for topology-owned
//       MachineDeployments.
// We don't have to set the finalizer, as it's already set during MachineDeployment creation
// in the cluster topology controller.
func (r *MachineDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
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

	cluster, err := util.GetClusterByName(ctx, r.Client, md.Namespace, md.Spec.ClusterName)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Return early if the object or Cluster is paused.
	if annotations.IsPaused(cluster, md) {
		log.Info("Reconciliation is paused for this object")
		return ctrl.Result{}, nil
	}

	// Handle deletion reconciliation loop.
	if !md.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, md)
	}

	// Nothing to do.
	return ctrl.Result{}, nil
}

// reconcileDelete deletes templates referenced in a MachineDeployment, if the templates are not used by other
// MachineDeployments or MachineSets.
func (r *MachineDeploymentReconciler) reconcileDelete(ctx context.Context, md *clusterv1.MachineDeployment) (ctrl.Result, error) {
	// Get the corresponding MachineSets.
	msList, err := getMachineSetsForDeployment(ctx, r.APIReader, client.ObjectKeyFromObject(md))
	if err != nil {
		return ctrl.Result{}, err
	}

	// Calculate which templates are still in use by MachineDeployments or MachineSets which are not in deleting state.
	templatesInUse, err := calculateTemplatesInUse(md, msList)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Delete unused templates.
	ref := md.Spec.Template.Spec.Bootstrap.ConfigRef
	if err := deleteTemplateIfUnused(ctx, r.Client, templatesInUse, ref); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to delete bootstrap template for %s", tlog.KObj{Obj: md})
	}
	ref = &md.Spec.Template.Spec.InfrastructureRef
	if err := deleteTemplateIfUnused(ctx, r.Client, templatesInUse, ref); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to delete infrastructure template for %s", tlog.KObj{Obj: md})
	}

	// Remove the finalizer so the MachineDeployment can be garbage collected by Kubernetes.
	patchHelper, err := patch.NewHelper(md, r.Client)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to create patch helper for %s", tlog.KObj{Obj: md})
	}
	controllerutil.RemoveFinalizer(md, clusterv1.MachineDeploymentTopologyFinalizer)
	if err := patchHelper.Patch(ctx, md); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to patch %s", tlog.KObj{Obj: md})
	}

	return ctrl.Result{}, nil
}
