/*
Copyright 2019 The Kubernetes Authors.

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
	"path"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/controllers/metrics"
	capierrors "sigs.k8s.io/cluster-api/errors"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/secret"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	// deleteRequeueAfter is how long to wait before checking again to see if the cluster still has children during
	// deletion.
	deleteRequeueAfter = 5 * time.Second
)

// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;patch
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io;bootstrap.cluster.x-k8s.io;controlplane.cluster.x-k8s.io,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch;create;update;patch;delete

// ClusterReconciler reconciles a Cluster object
type ClusterReconciler struct {
	Client client.Client
	Log    logr.Logger

	controller       controller.Controller
	recorder         record.EventRecorder
	externalWatchers sync.Map
}

func (r *ClusterReconciler) SetupWithManager(mgr ctrl.Manager, options controller.Options) error {
	controller, err := ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.Cluster{}).
		Watches(
			&source.Kind{Type: &clusterv1.Machine{}},
			&handler.EnqueueRequestsFromMapFunc{ToRequests: handler.ToRequestsFunc(r.controlPlaneMachineToCluster)},
		).
		WithOptions(options).
		Build(r)

	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	r.controller = controller
	r.recorder = mgr.GetEventRecorderFor("cluster-controller")
	return nil
}

func (r *ClusterReconciler) Reconcile(req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx := context.Background()

	// Fetch the Cluster instance.
	cluster := &clusterv1.Cluster{}
	if err := r.Client.Get(ctx, req.NamespacedName, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}

		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	// Initialize the patch helper.
	patchHelper, err := patch.NewHelper(cluster, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	defer func() {
		// Always reconcile the Status.Phase field.
		r.reconcilePhase(ctx, cluster)
		r.reconcileMetrics(ctx, cluster)

		// Always attempt to Patch the Cluster object and status after each reconciliation.
		if err := patchHelper.Patch(ctx, cluster); err != nil {
			if reterr == nil {
				reterr = err
			}
		}
	}()

	// Handle deletion reconciliation loop.
	if !cluster.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, cluster)
	}

	// Handle normal reconciliation loop.
	return r.reconcile(ctx, cluster)
}

// reconcile handles cluster reconciliation.
func (r *ClusterReconciler) reconcile(ctx context.Context, cluster *clusterv1.Cluster) (ctrl.Result, error) {
	logger := r.Log.WithValues("cluster", cluster.Name, "namespace", cluster.Namespace)

	// If object doesn't have a finalizer, add one.
	if !util.Contains(cluster.Finalizers, clusterv1.ClusterFinalizer) {
		cluster.Finalizers = append(cluster.Finalizers, clusterv1.ClusterFinalizer)
	}

	// Call the inner reconciliation methods.
	reconciliationErrors := []error{
		r.reconcileInfrastructure(ctx, cluster),
		r.reconcileControlPlane(ctx, cluster),
		r.reconcileKubeconfig(ctx, cluster),
		r.reconcileControlPlaneInitialized(ctx, cluster),
	}

	// Parse the errors, making sure we record if there is a RequeueAfterError.
	res := ctrl.Result{}
	errs := []error{}
	for _, err := range reconciliationErrors {
		if requeueErr, ok := errors.Cause(err).(capierrors.HasRequeueAfterError); ok {
			// Only record and log the first RequeueAfterError.
			if !res.Requeue {
				res.Requeue = true
				res.RequeueAfter = requeueErr.GetRequeueAfter()
				logger.Error(err, "Reconciliation for Cluster asked to requeue")
			}
			continue
		}

		errs = append(errs, err)
	}
	return res, kerrors.NewAggregate(errs)
}

func (r *ClusterReconciler) reconcileMetrics(_ context.Context, cluster *clusterv1.Cluster) {

	if cluster.Status.ControlPlaneInitialized {
		metrics.ClusterControlPlaneReady.WithLabelValues(cluster.Name, cluster.Namespace).Set(1)
	} else {
		metrics.ClusterControlPlaneReady.WithLabelValues(cluster.Name, cluster.Namespace).Set(0)
	}

	if cluster.Status.InfrastructureReady {
		metrics.ClusterInfrastructureReady.WithLabelValues(cluster.Name, cluster.Namespace).Set(1)
	} else {
		metrics.ClusterInfrastructureReady.WithLabelValues(cluster.Name, cluster.Namespace).Set(0)
	}

	// TODO: [wfernandes] pass context here
	_, err := secret.Get(r.Client, cluster, secret.Kubeconfig)
	if err != nil {
		metrics.ClusterKubeconfigReady.WithLabelValues(cluster.Name, cluster.Namespace).Set(0)
	} else {
		metrics.ClusterKubeconfigReady.WithLabelValues(cluster.Name, cluster.Namespace).Set(1)
	}

	if cluster.Status.FailureReason != nil || cluster.Status.FailureMessage != nil {
		metrics.ClusterFailureSet.WithLabelValues(cluster.Name, cluster.Namespace).Set(1)
	} else {
		metrics.ClusterFailureSet.WithLabelValues(cluster.Name, cluster.Namespace).Set(0)
	}
}

// reconcileDelete handles cluster deletion.
func (r *ClusterReconciler) reconcileDelete(ctx context.Context, cluster *clusterv1.Cluster) (reconcile.Result, error) {
	logger := r.Log.WithValues("cluster", cluster.Name, "namespace", cluster.Namespace)

	descendants, err := r.listDescendants(ctx, cluster)
	if err != nil {
		logger.Error(err, "Failed to list descendants")
		return reconcile.Result{}, err
	}

	children, err := descendants.filterOwnedDescendants(cluster)
	if err != nil {
		logger.Error(err, "Failed to extract direct descendants")
		return reconcile.Result{}, err
	}

	if len(children) > 0 {
		logger.Info("Cluster still has children - deleting them first", "count", len(children))

		var errs []error

		for _, child := range children {
			accessor, err := meta.Accessor(child)
			if err != nil {
				logger.Error(err, "Couldn't create accessor", "type", fmt.Sprintf("%T", child))
				continue
			}

			if !accessor.GetDeletionTimestamp().IsZero() {
				// Don't handle deleted child
				continue
			}

			gvk := child.GetObjectKind().GroupVersionKind().String()

			logger.Info("Deleting child", "gvk", gvk, "name", accessor.GetName())
			if err := r.Client.Delete(context.Background(), child); err != nil {
				err = errors.Wrapf(err, "error deleting cluster %s/%s: failed to delete %s %s", cluster.Namespace, cluster.Name, gvk, accessor.GetName())
				logger.Error(err, "Error deleting resource", "gvk", gvk, "name", accessor.GetName())
				errs = append(errs, err)
			}
		}

		if len(errs) > 0 {
			return ctrl.Result{}, kerrors.NewAggregate(errs)
		}
	}

	if descendantCount := descendants.length(); descendantCount > 0 {
		indirect := descendantCount - len(children)
		logger.Info("Cluster still has descendants - need to requeue", "direct", len(children), "indirect", indirect)
		// Requeue so we can check the next time to see if there are still any descendants left.
		return ctrl.Result{RequeueAfter: deleteRequeueAfter}, nil
	}

	if cluster.Spec.InfrastructureRef != nil {
		obj, err := external.Get(ctx, r.Client, cluster.Spec.InfrastructureRef, cluster.Namespace)
		switch {
		case apierrors.IsNotFound(errors.Cause(err)):
			// All good - the infra resource has been deleted
		case err != nil:
			return ctrl.Result{}, errors.Wrapf(err, "failed to get %s %q for Cluster %s/%s",
				path.Join(cluster.Spec.InfrastructureRef.APIVersion, cluster.Spec.InfrastructureRef.Kind),
				cluster.Spec.InfrastructureRef.Name, cluster.Namespace, cluster.Name)
		default:
			// Issue a deletion request for the infrastructure object.
			// Once it's been deleted, the cluster will get processed again.
			if err := r.Client.Delete(ctx, obj); err != nil {
				return ctrl.Result{}, errors.Wrapf(err,
					"failed to delete %v %q for Cluster %q in namespace %q",
					obj.GroupVersionKind(), obj.GetName(), cluster.Name, cluster.Namespace)
			}

			// Return here so we don't remove the finalizer yet.
			return ctrl.Result{}, nil
		}
	}

	cluster.Finalizers = util.Filter(cluster.Finalizers, clusterv1.ClusterFinalizer)
	return ctrl.Result{}, nil
}

type clusterDescendants struct {
	machineDeployments   clusterv1.MachineDeploymentList
	machineSets          clusterv1.MachineSetList
	controlPlaneMachines clusterv1.MachineList
	workerMachines       clusterv1.MachineList
}

// length returns the number of descendants
func (c *clusterDescendants) length() int {
	return len(c.machineDeployments.Items) +
		len(c.machineSets.Items) +
		len(c.controlPlaneMachines.Items) +
		len(c.workerMachines.Items)
}

// listDescendants returns a list of all MachineDeployments, MachineSets, and Machines for the cluster.
func (r *ClusterReconciler) listDescendants(ctx context.Context, cluster *clusterv1.Cluster) (clusterDescendants, error) {
	var descendants clusterDescendants

	listOptions := []client.ListOption{
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(map[string]string{clusterv1.ClusterLabelName: cluster.Name}),
	}

	if err := r.Client.List(ctx, &descendants.machineDeployments, listOptions...); err != nil {
		return descendants, errors.Wrapf(err, "failed to list MachineDeployments for cluster %s/%s", cluster.Namespace, cluster.Name)
	}

	if err := r.Client.List(ctx, &descendants.machineSets, listOptions...); err != nil {
		return descendants, errors.Wrapf(err, "failed to list MachineSets for cluster %s/%s", cluster.Namespace, cluster.Name)
	}

	var machines clusterv1.MachineList
	if err := r.Client.List(ctx, &machines, listOptions...); err != nil {
		return descendants, errors.Wrapf(err, "failed to list Machines for cluster %s/%s", cluster.Namespace, cluster.Name)
	}

	// Split machines into control plane and worker machines so we make sure we delete control plane machines last
	controlPlaneMachines, workerMachines := splitMachineList(&machines)
	descendants.controlPlaneMachines = *controlPlaneMachines
	descendants.workerMachines = *workerMachines

	return descendants, nil
}

// filterOwnedDescendants returns an array of runtime.Objects containing only those descendants that have the cluster
// as an owner reference, with control plane machines sorted last.
func (c clusterDescendants) filterOwnedDescendants(cluster *clusterv1.Cluster) ([]runtime.Object, error) {
	var ownedDescendants []runtime.Object
	eachFunc := func(o runtime.Object) error {
		acc, err := meta.Accessor(o)
		if err != nil {
			return nil
		}

		if util.PointsTo(acc.GetOwnerReferences(), &cluster.ObjectMeta) {
			ownedDescendants = append(ownedDescendants, o)
		}

		return nil
	}

	lists := []runtime.Object{
		&c.machineDeployments,
		&c.machineSets,
		&c.workerMachines,
		&c.controlPlaneMachines,
	}
	for _, list := range lists {
		if err := meta.EachListItem(list, eachFunc); err != nil {
			return nil, errors.Wrapf(err, "error finding owned descendants of cluster %s/%s", cluster.Namespace, cluster.Name)
		}
	}

	return ownedDescendants, nil
}

// splitMachineList separates the machines running the control plane from other worker nodes.
func splitMachineList(list *clusterv1.MachineList) (*clusterv1.MachineList, *clusterv1.MachineList) {
	nodes := &clusterv1.MachineList{}
	controlplanes := &clusterv1.MachineList{}
	for i := range list.Items {
		machine := &list.Items[i]
		if util.IsControlPlaneMachine(machine) {
			controlplanes.Items = append(controlplanes.Items, *machine)
		} else {
			nodes.Items = append(nodes.Items, *machine)
		}
	}
	return controlplanes, nodes
}

func (r *ClusterReconciler) reconcileControlPlaneInitialized(ctx context.Context, cluster *clusterv1.Cluster) error {
	logger := r.Log.WithValues("cluster", cluster.Name, "namespace", cluster.Namespace)

	if cluster.Status.ControlPlaneInitialized {
		return nil
	}

	machines, err := getActiveMachinesInCluster(ctx, r.Client, cluster.Namespace, cluster.Name)
	if err != nil {
		logger.Error(err, "Error getting machines in cluster")
		return err
	}

	for _, m := range machines {
		if util.IsControlPlaneMachine(m) && m.Status.NodeRef != nil {
			cluster.Status.ControlPlaneInitialized = true
			return nil
		}
	}

	return nil
}

// controlPlaneMachineToCluster is a handler.ToRequestsFunc to be used to enqueue requests for reconciliation
// for Cluster to update its status.controlPlaneInitialized field
func (r *ClusterReconciler) controlPlaneMachineToCluster(o handler.MapObject) []ctrl.Request {
	m, ok := o.Object.(*clusterv1.Machine)
	if !ok {
		r.Log.Error(nil, fmt.Sprintf("Expected a Machine but got a %T", o.Object))
		return nil
	}
	if !util.IsControlPlaneMachine(m) {
		return nil
	}
	if m.Status.NodeRef == nil {
		return nil
	}

	cluster, err := util.GetClusterByName(context.TODO(), r.Client, m.ObjectMeta.Namespace, m.Spec.ClusterName)
	if err != nil {
		r.Log.Error(err, "Failed to get cluster", "machine", m.Name, "cluster", m.ClusterName, "namespace", m.Namespace)
		return nil
	}

	if cluster.Status.ControlPlaneInitialized {
		return nil
	}

	return []ctrl.Request{{
		NamespacedName: types.NamespacedName{
			Namespace: cluster.Namespace,
			Name:      cluster.Name,
		},
	}}
}
