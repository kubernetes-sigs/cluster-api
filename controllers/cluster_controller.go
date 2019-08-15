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
	"path"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha2"
	"sigs.k8s.io/cluster-api/controllers/external"
	capierrors "sigs.k8s.io/cluster-api/errors"
	"sigs.k8s.io/cluster-api/util"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// deleteRequeueAfter is how long to wait before checking again to see if the cluster still has children during
	// deletion.
	deleteRequeueAfter = 5 * time.Second
)

// ClusterReconciler reconciles a Cluster object
type ClusterReconciler struct {
	client.Client
	Log logr.Logger

	controller       controller.Controller
	recorder         record.EventRecorder
	externalWatchers sync.Map
}

func (r *ClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.Cluster{}).
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
	err := r.Get(ctx, req.NamespacedName, cluster)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}

		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	// Store Cluster early state to allow patching.
	patchCluster := client.MergeFrom(cluster.DeepCopy())

	// Always issue a Patch for the Cluster object and its status after each reconciliation.
	defer func() {
		if err := r.patchCluster(ctx, cluster, patchCluster); err != nil {
			if reterr == nil {
				reterr = err
			}
		}
	}()

	// If object hasn't been deleted and doesn't have a finalizer, add one.
	if cluster.ObjectMeta.DeletionTimestamp.IsZero() {
		if !util.Contains(cluster.Finalizers, clusterv1.ClusterFinalizer) {
			cluster.Finalizers = append(cluster.ObjectMeta.Finalizers, clusterv1.ClusterFinalizer)
		}
	}

	if err := r.reconcile(ctx, cluster); err != nil {
		if requeueErr, ok := errors.Cause(err).(capierrors.HasRequeueAfterError); ok {
			klog.Infof("Reconciliation for Cluster %q in namespace %q asked to requeue: %v", cluster.Name, cluster.Namespace, err)
			return ctrl.Result{Requeue: true, RequeueAfter: requeueErr.GetRequeueAfter()}, nil
		}
		return ctrl.Result{}, err
	}

	if !cluster.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, cluster)
	}

	return ctrl.Result{}, nil
}

// reconcileDelete handles cluster deletion.
// TODO(ncdc): consolidate all deletion logic in here.
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
			if err := r.Delete(context.Background(), child); err != nil {
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
		_, err := external.Get(r.Client, cluster.Spec.InfrastructureRef, cluster.Namespace)
		switch {
		case apierrors.IsNotFound(err):
			// All good - the infra resource has been deleted
		case err != nil:
			return ctrl.Result{}, errors.Wrapf(err, "failed to get %s %q for Cluster %s/%s",
				path.Join(cluster.Spec.InfrastructureRef.APIVersion, cluster.Spec.InfrastructureRef.Kind),
				cluster.Spec.InfrastructureRef.Name, cluster.Namespace, cluster.Name)
		default:
			// The infra resource still exists. Once it's been deleted, the cluster will get processed again.
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

	machines := &clusterv1.MachineList{}
	if err := r.Client.List(ctx, machines, listOptions...); err != nil {
		return nil, errors.Wrapf(err, "failed to list Machines for cluster %s/%s", cluster.Namespace, cluster.Name)
	}

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

	lists := map[string]runtime.Object{
		"MachineDeployment": machineDeployments,
		"MachineSet":        machineSets,
		"Machine":           machines,
	}
	for name, list := range lists {
		if err := meta.EachListItem(list, eachFunc); err != nil {
			return nil, errors.Wrapf(err, "error finding %s children of cluster %s/%s", name, cluster.Namespace, cluster.Name)
		}
	}

	return children, nil
}

func (r *ClusterReconciler) patchCluster(ctx context.Context, cluster *clusterv1.Cluster, patch client.Patch) error {
	log := r.Log.WithValues("cluster-namespace", cluster.Namespace, "cluster-name", cluster.Name)
	// Always patch the status before the spec
	if err := r.Client.Status().Patch(ctx, cluster, patch); err != nil {
		log.Error(err, "Error patching cluster status")
		return err
	}
	if err := r.Client.Patch(ctx, cluster, patch); err != nil {
		log.Error(err, "Error patching cluster")
		return err
	}
	return nil
}
