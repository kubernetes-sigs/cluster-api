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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
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
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinedeployments,verbs=get;list;watch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinesets;machinesets/finalizers,verbs=get;list;watch;update;patch

// MachineSetReconciler deletes referenced templates during deletion of topology-owned MachineSets.
// The templates are only deleted, if they are not used in other MachineDeployments or MachineSets which are not in deleting state,
// i.e. the templates would otherwise be orphaned after the MachineSet deletion completes.
// Note: To achieve this the reconciler sets a finalizer to hook into the MachineSet deletions.
type MachineSetReconciler struct {
	Client client.Client
	// APIReader is used to list MachineSets directly via the API server to avoid
	// race conditions caused by an outdated cache.
	APIReader        client.Reader
	WatchFilterValue string
}

func (r *MachineSetReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
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
//   * MachineSets are deleted and garbage collected first (without waiting until all Machines are also deleted)
//   * After that, deletion of Machines is automatically triggered by Kubernetes based on owner references.
// Note: We assume templates are not reused by different MachineDeployments, which is (only) true for topology-owned
//       MachineDeployments.
// We don't have to set the finalizer, as it's already set during MachineSet creation
// in the MachineSet controller.
func (r *MachineSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

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

	cluster, err := util.GetClusterByName(ctx, r.Client, ms.Namespace, ms.Spec.ClusterName)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Return early if the object or Cluster is paused.
	if annotations.IsPaused(cluster, ms) {
		log.Info("Reconciliation is paused for this object")
		return ctrl.Result{}, nil
	}

	// Handle deletion reconciliation loop.
	if !ms.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, ms)
	}

	// Nothing to do.
	return ctrl.Result{}, nil
}

// reconcileDelete deletes templates referenced in a MachineSet, if the templates are not used by other
// MachineDeployments or MachineSets.
func (r *MachineSetReconciler) reconcileDelete(ctx context.Context, ms *clusterv1.MachineSet) (ctrl.Result, error) {
	// Gets the name of the MachineDeployment that controls this MachineSet.
	mdName, err := getMachineDeploymentName(ms)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Get all the MachineSets for the MachineDeployment.
	msList, err := getMachineSetsForDeployment(ctx, r.APIReader, *mdName)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Fetch the MachineDeployment instance, if it still exists.
	// Note: This can happen because MachineDeployments are deleted before their corresponding MachineSets.
	md := &clusterv1.MachineDeployment{}
	if err := r.Client.Get(ctx, *mdName, md); err != nil {
		if !apierrors.IsNotFound(err) {
			// Error reading the object - requeue the request.
			return ctrl.Result{}, errors.Wrapf(err, "failed to get MachineDeployment/%s", mdName.Name)
		}
		// If the MachineDeployment doesn't exist anymore, set md to nil, so we can handle that case correctly below.
		md = nil
	}

	// Calculate which templates are still in use by MachineDeployments or MachineSets which are not in deleting state.
	templatesInUse, err := calculateTemplatesInUse(md, msList)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Delete unused templates.
	ref := ms.Spec.Template.Spec.Bootstrap.ConfigRef
	if err := deleteTemplateIfUnused(ctx, r.Client, templatesInUse, ref); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to delete bootstrap template for %s", tlog.KObj{Obj: ms})
	}
	ref = &ms.Spec.Template.Spec.InfrastructureRef
	if err := deleteTemplateIfUnused(ctx, r.Client, templatesInUse, ref); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to delete infrastructure template for %s", tlog.KObj{Obj: ms})
	}

	// Remove the finalizer so the MachineSet can be garbage collected by Kubernetes.
	patchHelper, err := patch.NewHelper(ms, r.Client)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to create patch helper for %s", tlog.KObj{Obj: ms})
	}
	controllerutil.RemoveFinalizer(ms, clusterv1.MachineSetTopologyFinalizer)
	if err := patchHelper.Patch(ctx, ms); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to patch %s", tlog.KObj{Obj: ms})
	}

	return ctrl.Result{}, nil
}

// getMachineDeploymentName calculates the MachineDeployment name based on owner references.
func getMachineDeploymentName(ms *clusterv1.MachineSet) (*types.NamespacedName, error) {
	for _, ref := range ms.OwnerReferences {
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
