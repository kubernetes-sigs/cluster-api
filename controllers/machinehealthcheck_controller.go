/*
Copyright 2020 The Kubernetes Authors.

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

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
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
	mhcClusterNameIndex = "spec.clusterName"
)

// MachineHealthCheckReconciler reconciles a MachineHealthCheck object
type MachineHealthCheckReconciler struct {
	Client client.Client
	Log    logr.Logger

	controller controller.Controller
	recorder   record.EventRecorder
}

func (r *MachineHealthCheckReconciler) SetupWithManager(mgr ctrl.Manager, options controller.Options) error {
	controller, err := ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.MachineHealthCheck{}).
		Watches(
			&source.Kind{Type: &clusterv1.Cluster{}},
			&handler.EnqueueRequestsFromMapFunc{ToRequests: handler.ToRequestsFunc(r.clusterToMachineHealthCheck)},
		).
		WithOptions(options).
		Build(r)

	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	// Add index to MachineHealthCheck for listing by Cluster Name
	if err := mgr.GetCache().IndexField(&clusterv1.MachineHealthCheck{},
		mhcClusterNameIndex,
		r.indexMachineHealthCheckByClusterName,
	); err != nil {
		return errors.Wrap(err, "error setting index fields")
	}

	r.controller = controller
	r.recorder = mgr.GetEventRecorderFor("machinehealthcheck-controller")
	return nil
}

func (r *MachineHealthCheckReconciler) Reconcile(req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx := context.Background()
	logger := r.Log.WithValues("machinehealthcheck", req.Name, "namespace", req.Namespace)

	// Fetch the MachineHealthCheck instance
	m := &clusterv1.MachineHealthCheck{}
	if err := r.Client.Get(ctx, req.NamespacedName, m); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}

		// Error reading the object - requeue the request.
		logger.Error(err, "Failed to fetch MachineHealthCheck")
		return ctrl.Result{}, err
	}

	cluster, err := util.GetClusterByName(ctx, r.Client, m.Namespace, m.Spec.ClusterName)
	if err != nil {
		logger.Error(err, "Failed to fetch Cluster for MachineHealthCheck")
		return ctrl.Result{}, errors.Wrapf(err, "failed to get Cluster %q for MachineHealthCheck %q in namespace %q",
			m.Spec.ClusterName, m.Name, m.Namespace)
	}

	// Return early if the object or Cluster is paused.
	if util.IsPaused(cluster, m) {
		logger.V(3).Info("reconciliation is paused for this object")
		return ctrl.Result{}, nil
	}

	// Initialize the patch helper
	patchHelper, err := patch.NewHelper(m, r.Client)
	if err != nil {
		logger.Error(err, "Failed to build patch helper")
		return ctrl.Result{}, err
	}

	defer func() {
		// Always attempt to patch the object and status after each reconciliation.
		if err := patchHelper.Patch(ctx, m); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}
	}()

	// Reconcile labels.
	if m.Labels == nil {
		m.Labels = make(map[string]string)
	}
	m.Labels[clusterv1.ClusterLabelName] = m.Spec.ClusterName

	result, err := r.reconcile(ctx, cluster, m)
	if err != nil {
		logger.Error(err, "Failed to reconcile MachineHealthCheck")
		r.recorder.Eventf(m, corev1.EventTypeWarning, "ReconcileError", "%v", err)

		//TODO(JoelSpeed): Determine how/when to requeue requests if errors occur within r.reconcile
	}

	return result, nil
}

func (r *MachineHealthCheckReconciler) reconcile(_ context.Context, cluster *clusterv1.Cluster, m *clusterv1.MachineHealthCheck) (ctrl.Result, error) {
	// Ensure the MachineHealthCheck is owned by the Cluster it belongs to
	m.OwnerReferences = util.EnsureOwnerRef(m.OwnerReferences, metav1.OwnerReference{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Cluster",
		Name:       cluster.Name,
		UID:        cluster.UID,
	})

	return ctrl.Result{}, fmt.Errorf("controller not yet implemented")
}

func (r *MachineHealthCheckReconciler) indexMachineHealthCheckByClusterName(object runtime.Object) []string {
	mhc, ok := object.(*clusterv1.MachineHealthCheck)
	if !ok {
		r.Log.Error(errors.New("incorrect type"), "expected a MachineHealthCheck", "type", fmt.Sprintf("%T", object))
		return nil
	}

	return []string{mhc.Spec.ClusterName}
}

// clusterToMachineHealthCheck maps events from Cluster objects to
// MachineHealthCheck objects that belong to the Cluster
func (r *MachineHealthCheckReconciler) clusterToMachineHealthCheck(o handler.MapObject) []reconcile.Request {
	c, ok := o.Object.(*clusterv1.Cluster)
	if !ok {
		r.Log.Error(errors.New("incorrect type"), "expected a Cluster", "type", fmt.Sprintf("%T", o))
		return nil
	}

	mhcList := &clusterv1.MachineHealthCheckList{}
	if err := r.Client.List(
		context.TODO(),
		mhcList,
		client.InNamespace(c.Namespace),
		client.MatchingFields{mhcClusterNameIndex: c.Name},
	); err != nil {
		r.Log.Error(err, "Unable to list MachineHealthChecks", "cluster", c.Name, "namespace", c.Namespace)
		return nil
	}

	// This list should only contain MachineHealthChecks which belong to the given Cluster
	requests := []reconcile.Request{}
	for _, mhc := range mhcList.Items {
		key := types.NamespacedName{Namespace: mhc.Namespace, Name: mhc.Name}
		requests = append(requests, reconcile.Request{NamespacedName: key})
	}
	return requests
}
