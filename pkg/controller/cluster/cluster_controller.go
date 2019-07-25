/*
Copyright 2018 The Kubernetes Authors.

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
	"path"
	"sync"
	"time"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha2"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha2"
	"sigs.k8s.io/cluster-api/pkg/controller/external"
	capierrors "sigs.k8s.io/cluster-api/pkg/errors"
	"sigs.k8s.io/cluster-api/pkg/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const controllerName = "cluster-controller"

// Add creates a new Cluster Controller and adds it to the Manager with default RBAC.
// The Manager will set fields on the Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	r := newReconciler(mgr)
	c, err := addController(mgr, r)
	r.controller = c
	return err
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) *ReconcileCluster {
	return &ReconcileCluster{
		Client:   mgr.GetClient(),
		scheme:   mgr.GetScheme(),
		recorder: mgr.GetEventRecorderFor(controllerName),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func addController(mgr manager.Manager, r reconcile.Reconciler) (controller.Controller, error) {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return nil, err
	}

	// Watch for changes to Cluster
	err = c.Watch(&source.Kind{Type: &clusterv1.Cluster{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return nil, err
	}

	return c, nil
}

var _ reconcile.Reconciler = &ReconcileCluster{}

// ReconcileCluster reconciles a Cluster object
type ReconcileCluster struct {
	client.Client
	scheme           *runtime.Scheme
	controller       controller.Controller
	recorder         record.EventRecorder
	externalWatchers sync.Map
}

func (r *ReconcileCluster) Reconcile(request reconcile.Request) (_ reconcile.Result, reterr error) {
	ctx := context.TODO()

	// Fetch the Cluster instance.
	cluster := &clusterv1.Cluster{}
	err := r.Get(ctx, request.NamespacedName, cluster)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}

		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// Store Cluster early state to allow patching.
	patchCluster := client.MergeFrom(cluster.DeepCopy())

	// Always issue a Patch for the Cluster object and its status after each reconciliation.
	defer func() {
		gvk := cluster.GroupVersionKind()
		if err := r.Client.Patch(ctx, cluster, patchCluster); err != nil {
			klog.Errorf("Error Patching Cluster %q in namespace %q: %v", cluster.Name, cluster.Namespace, err)
			if reterr == nil {
				reterr = err
			}
			return
		}
		// TODO(vincepri): This is a hack because after a Patch, the object loses TypeMeta information.
		// Remove when https://github.com/kubernetes-sigs/controller-runtime/issues/526 is fixed.
		cluster.SetGroupVersionKind(gvk)
		if err := r.Client.Status().Patch(ctx, cluster, patchCluster); err != nil {
			klog.Errorf("Error Patching Cluster status %q in namespace %q: %v", cluster.Name, cluster.Namespace, err)
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
			return reconcile.Result{Requeue: true, RequeueAfter: requeueErr.GetRequeueAfter()}, nil
		}
		return reconcile.Result{}, err
	}

	if !cluster.ObjectMeta.DeletionTimestamp.IsZero() {
		if err := r.isDeleteReady(ctx, cluster); err != nil {
			if requeueErr, ok := errors.Cause(err).(capierrors.HasRequeueAfterError); ok {
				klog.Infof("Reconciliation for Cluster %q in namespace %q asked to requeue: %v", cluster.Name, cluster.Namespace, err)
				return reconcile.Result{Requeue: true, RequeueAfter: requeueErr.GetRequeueAfter()}, nil
			}
			return reconcile.Result{}, err
		}

		cluster.ObjectMeta.Finalizers = util.Filter(cluster.ObjectMeta.Finalizers, clusterv1.ClusterFinalizer)
	}

	return reconcile.Result{}, nil
}

// isDeleteReady returns an error if InfrastructureRef referenced object still exist
// or if any resources linked to this cluster aren't yet deleted.
func (r *ReconcileCluster) isDeleteReady(ctx context.Context, cluster *v1alpha2.Cluster) error {
	// TODO(vincepri): List and delete MachineDeployments, MachineSets, Machines, and
	// block deletion until they're all deleted.

	if cluster.Spec.InfrastructureRef != nil {
		_, err := external.Get(r.Client, cluster.Spec.InfrastructureRef, cluster.Namespace)
		if err != nil && !apierrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to get %s %q for Cluster %q in namespace %q",
				path.Join(cluster.Spec.InfrastructureRef.APIVersion, cluster.Spec.InfrastructureRef.Kind),
				cluster.Spec.InfrastructureRef.Name, cluster.Name, cluster.Namespace)
		} else if err == nil {
			return &capierrors.RequeueAfterError{RequeueAfter: 10 * time.Second}
		}
	}

	return nil
}
