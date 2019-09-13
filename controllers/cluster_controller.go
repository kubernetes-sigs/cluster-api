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
	"k8s.io/klog"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha2"
	"sigs.k8s.io/cluster-api/controllers/external"
	capierrors "sigs.k8s.io/cluster-api/errors"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"
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
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io;bootstrap.cluster.x-k8s.io,resources=*,verbs=get;list;watch;create;update;patch;delete
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
	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.Cluster{}).
		Watches(
			&source.Kind{Type: &clusterv1.Machine{}},
			&handler.EnqueueRequestsFromMapFunc{ToRequests: handler.ToRequestsFunc(r.controlPlaneMachineToCluster)},
		).
		WithOptions(options).
		Build(r)

	r.controller = c
	r.recorder = mgr.GetEventRecorderFor("cluster-controller")
	return err
}

func (r *ClusterReconciler) Reconcile(req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx := context.Background()
	_ = r.Log.WithValues("cluster", req.NamespacedName)

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
	// If object doesn't have a finalizer, add one.
	if !util.Contains(cluster.Finalizers, clusterv1.ClusterFinalizer) {
		cluster.Finalizers = append(cluster.ObjectMeta.Finalizers, clusterv1.ClusterFinalizer)
	}

	// Call the inner reconciliation methods.
	reconciliationErrors := []error{
		r.reconcileInfrastructure(ctx, cluster),
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
				klog.Infof("Reconciliation for Cluster %q in namespace %q asked to requeue: %v", cluster.Name, cluster.Namespace, err)
			}
			continue
		}

		errs = append(errs, err)
	}
	return res, kerrors.NewAggregate(errs)
}

// reconcileDelete handles cluster deletion.
func (r *ClusterReconciler) reconcileDelete(ctx context.Context, cluster *clusterv1.Cluster) (reconcile.Result, error) {
	children, err := r.listChildren(ctx, cluster)
	if err != nil {
		klog.Errorf("Failed to list children of cluster %s/%s: %v", cluster.Namespace, cluster.Name, err)
		return reconcile.Result{}, err
	}

	if len(children) > 0 {
		klog.Infof("Cluster %s/%s still has %d children - deleting them first", cluster.Namespace, cluster.Name, len(children))

		var errs []error

		for _, child := range children {
			accessor, err := meta.Accessor(child)
			if err != nil {
				klog.Errorf("Cluster %s/%s: couldn't create accessor for type %T: %v", cluster.Namespace, cluster.Name, child, err)
				continue
			}

			if !accessor.GetDeletionTimestamp().IsZero() {
				// Don't handle deleted child
				continue
			}

			gvk := child.GetObjectKind().GroupVersionKind().String()

			klog.Infof("Cluster %s/%s: deleting child %s %s", cluster.Namespace, cluster.Name, gvk, accessor.GetName())
			if err := r.Client.Delete(context.Background(), child); err != nil {
				err = errors.Wrapf(err, "error deleting cluster %s/%s: failed to delete %s %s", cluster.Namespace, cluster.Name, gvk, accessor.GetName())
				klog.Errorf(err.Error())
				errs = append(errs, err)
			}
		}

		if len(errs) > 0 {
			return ctrl.Result{}, kerrors.NewAggregate(errs)
		}

		// Requeue so we can check the next time to see if there are still any children left.
		return ctrl.Result{RequeueAfter: deleteRequeueAfter}, nil
	}

	if cluster.Spec.InfrastructureRef != nil {
		obj, err := external.Get(r.Client, cluster.Spec.InfrastructureRef, cluster.Namespace)
		switch {
		case apierrors.IsNotFound(err):
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

// listChildren returns a list of MachineDeployments, MachineSets, and Machines than have an owner reference to cluster
func (r *ClusterReconciler) listChildren(ctx context.Context, cluster *clusterv1.Cluster) ([]runtime.Object, error) {
	listOptions := []client.ListOption{
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(map[string]string{clusterv1.MachineClusterLabelName: cluster.Name}),
	}

	machineDeployments := &clusterv1.MachineDeploymentList{}
	if err := r.Client.List(ctx, machineDeployments, listOptions...); err != nil {
		return nil, errors.Wrapf(err, "failed to list MachineDeployments for cluster %s/%s", cluster.Namespace, cluster.Name)
	}

	machineSets := &clusterv1.MachineSetList{}
	if err := r.Client.List(ctx, machineSets, listOptions...); err != nil {
		return nil, errors.Wrapf(err, "failed to list MachineSets for cluster %s/%s", cluster.Namespace, cluster.Name)
	}

	allMachines := &clusterv1.MachineList{}
	if err := r.Client.List(ctx, allMachines, listOptions...); err != nil {
		return nil, errors.Wrapf(err, "failed to list Machines for cluster %s/%s", cluster.Namespace, cluster.Name)
	}
	controlPlaneMachines, machines := splitMachineList(allMachines)

	var children []runtime.Object
	eachFunc := func(o runtime.Object) error {
		acc, err := meta.Accessor(o)
		if err != nil {
			klog.Errorf("Cluster %s/%s: couldn't create accessor for type %T: %v", cluster.Namespace, cluster.Name, o, err)
			return nil
		}

		if util.PointsTo(acc.GetOwnerReferences(), &cluster.ObjectMeta) {
			children = append(children, o)
		}

		return nil
	}

	lists := []runtime.Object{
		machineDeployments,
		machineSets,
		machines,
		controlPlaneMachines,
	}
	for _, list := range lists {
		if err := meta.EachListItem(list, eachFunc); err != nil {
			return nil, errors.Wrapf(err, "error finding children of cluster %s/%s", cluster.Namespace, cluster.Name)
		}
	}

	return children, nil
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
	if cluster.Status.ControlPlaneInitialized {
		return nil
	}

	machines, err := getActiveMachinesInCluster(ctx, r.Client, cluster.Namespace, cluster.Name)
	if err != nil {
		r.Log.Error(err, "error getting machines in cluster", "cluster", cluster.Name, "namespace", cluster.Namespace)
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
		r.Log.Error(errors.New("did not get machine"), "got", fmt.Sprintf("%T", o.Object))
		return nil
	}
	if !util.IsControlPlaneMachine(m) {
		return nil
	}
	if m.Status.NodeRef == nil {
		return nil
	}

	cluster, err := util.GetClusterFromMetadata(context.TODO(), r.Client, m.ObjectMeta)
	if err != nil {
		r.Log.Error(err, "failed to get cluster", "machine", m.Name, "cluster", m.ClusterName, "namespace", m.Namespace)
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
