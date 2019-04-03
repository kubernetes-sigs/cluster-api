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

package machine

import (
	"context"
	"os"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	controllerError "sigs.k8s.io/cluster-api/pkg/controller/error"
	"sigs.k8s.io/cluster-api/pkg/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	NodeNameEnvVar = "NODE_NAME"
)

var DefaultActuator Actuator

func AddWithActuator(mgr manager.Manager, actuator Actuator) error {
	return add(mgr, newReconciler(mgr, actuator))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, actuator Actuator) reconcile.Reconciler {
	r := &ReconcileMachine{
		Client:   mgr.GetClient(),
		scheme:   mgr.GetScheme(),
		nodeName: os.Getenv(NodeNameEnvVar),
		actuator: actuator,
	}

	if r.nodeName == "" {
		klog.Warningf("Environment variable %q is not set, this controller will not protect against deleting its own machine", NodeNameEnvVar)
	}

	return r
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("machine-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Machine
	return c.Watch(
		&source.Kind{Type: &clusterv1.Machine{}},
		&handler.EnqueueRequestForObject{},
	)
}

// ReconcileMachine reconciles a Machine object
type ReconcileMachine struct {
	client.Client
	scheme *runtime.Scheme

	actuator Actuator

	// nodeName is the name of the node on which the machine controller is running, if not present, it is loaded from NODE_NAME.
	nodeName string
}

// Reconcile reads that state of the cluster for a Machine object and makes changes based on the state read
// and what is in the Machine.Spec
// +kubebuilder:rbac:groups=cluster.k8s.io,resources=machines;machines/status,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileMachine) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// TODO(mvladev): Can context be passed from Kubebuilder?
	ctx := context.TODO()

	// Fetch the Machine instance
	m := &clusterv1.Machine{}
	if err := r.Client.Get(ctx, request.NamespacedName, m); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}

		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// Implement controller logic here
	name := m.Name
	klog.Infof("Reconciling Machine %q", name)

	// Cluster might be nil as some providers might not require a cluster object
	// for machine management.
	cluster, err := r.getCluster(ctx, m)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Set the ownerRef with foreground deletion if there is a linked cluster.
	if cluster != nil && len(m.OwnerReferences) == 0 {
		blockOwnerDeletion := true
		m.OwnerReferences = append(m.OwnerReferences, metav1.OwnerReference{
			APIVersion:         cluster.APIVersion,
			Kind:               cluster.Kind,
			Name:               cluster.Name,
			UID:                cluster.UID,
			BlockOwnerDeletion: &blockOwnerDeletion,
		})
	}

	// If object hasn't been deleted and doesn't have a finalizer, add one
	// Add a finalizer to newly created objects.
	if m.ObjectMeta.DeletionTimestamp.IsZero() {
		finalizerCount := len(m.Finalizers)

		if cluster != nil && !util.Contains(m.Finalizers, metav1.FinalizerDeleteDependents) {
			m.Finalizers = append(m.ObjectMeta.Finalizers, metav1.FinalizerDeleteDependents)
		}

		if !util.Contains(m.Finalizers, clusterv1.MachineFinalizer) {
			m.Finalizers = append(m.ObjectMeta.Finalizers, clusterv1.MachineFinalizer)
		}

		if len(m.Finalizers) > finalizerCount {
			if err := r.Client.Update(ctx, m); err != nil {
				klog.Infof("Failed to add finalizers to machine %q: %v", name, err)
				return reconcile.Result{}, err
			}

			// Since adding the finalizer updates the object return to avoid later update issues
			return reconcile.Result{}, nil
		}
	}

	if !m.ObjectMeta.DeletionTimestamp.IsZero() {
		// no-op if finalizer has been removed.
		if !util.Contains(m.ObjectMeta.Finalizers, clusterv1.MachineFinalizer) {
			klog.Infof("Reconciling machine %q causes a no-op as there is no finalizer", name)
			return reconcile.Result{}, nil
		}

		if !r.isDeleteAllowed(m) {
			klog.Infof("Skipping reconciling of machine %q", name)
			return reconcile.Result{}, nil
		}

		klog.Infof("Reconciling machine %q triggers delete", name)
		if err := r.actuator.Delete(ctx, cluster, m); err != nil {
			if requeueErr, ok := err.(*controllerError.RequeueAfterError); ok {
				klog.Infof("Actuator returned requeue-after error: %v", requeueErr)
				return reconcile.Result{Requeue: true, RequeueAfter: requeueErr.RequeueAfter}, nil
			}

			klog.Errorf("Failed to delete machine %q: %v", name, err)
			return reconcile.Result{}, err
		}

		if m.Status.NodeRef != nil {
			klog.Infof("Deleting node %q for machine %q", m.Status.NodeRef.Name, m.Name)
			if err := r.deleteNode(ctx, m.Status.NodeRef.Name); err != nil {
				klog.Errorf("Error deleting node %q for machine %q", name, err)
				return reconcile.Result{}, err
			}
		}

		// Remove finalizer on successful deletion.
		m.ObjectMeta.Finalizers = util.Filter(m.ObjectMeta.Finalizers, clusterv1.MachineFinalizer)
		if err := r.Client.Update(context.Background(), m); err != nil {
			klog.Errorf("Failed to remove finalizer from machine %q: %v", name, err)
			return reconcile.Result{}, err
		}

		klog.Infof("Machine %q deletion successful", name)
		return reconcile.Result{}, nil
	}

	exist, err := r.actuator.Exists(ctx, cluster, m)
	if err != nil {
		klog.Errorf("Failed to check if machine %q exists: %v", name, err)
		return reconcile.Result{}, err
	}

	if exist {
		klog.Infof("Reconciling machine %q triggers idempotent update", name)
		if err := r.actuator.Update(ctx, cluster, m); err != nil {
			if requeueErr, ok := err.(*controllerError.RequeueAfterError); ok {
				klog.Infof("Actuator returned requeue-after error: %v", requeueErr)
				return reconcile.Result{Requeue: true, RequeueAfter: requeueErr.RequeueAfter}, nil
			}

			klog.Errorf(`Error updating machine "%s/%s": %v`, m.Namespace, name, err)
			return reconcile.Result{}, err
		}

		return reconcile.Result{}, nil
	}

	// Machine resource created. Machine does not yet exist.
	klog.Infof("Reconciling machine object %v triggers idempotent create.", m.ObjectMeta.Name)
	if err := r.actuator.Create(ctx, cluster, m); err != nil {
		if requeueErr, ok := err.(*controllerError.RequeueAfterError); ok {
			klog.Infof("Actuator returned requeue-after error: %v", requeueErr)
			return reconcile.Result{Requeue: true, RequeueAfter: requeueErr.RequeueAfter}, nil
		}

		klog.Warningf("Failed to create machine %q: %v", name, err)
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileMachine) getCluster(ctx context.Context, machine *clusterv1.Machine) (*clusterv1.Cluster, error) {
	if machine.Labels[clusterv1.MachineClusterLabelName] == "" {
		klog.Infof("Machine %q in namespace %q doesn't specify %q label, assuming nil cluster", machine.Name, machine.Namespace, clusterv1.MachineClusterLabelName)
		return nil, nil
	}

	cluster := &clusterv1.Cluster{}
	key := client.ObjectKey{
		Namespace: machine.Namespace,
		Name:      machine.Labels[clusterv1.MachineClusterLabelName],
	}

	if err := r.Client.Get(ctx, key, cluster); err != nil {
		return nil, err
	}

	return cluster, nil
}

func (r *ReconcileMachine) isDeleteAllowed(machine *clusterv1.Machine) bool {
	if r.nodeName == "" || machine.Status.NodeRef == nil {
		return true
	}

	if machine.Status.NodeRef.Name != r.nodeName {
		return true
	}

	node := &corev1.Node{}
	if err := r.Client.Get(context.Background(), client.ObjectKey{Name: r.nodeName}, node); err != nil {
		klog.Infof("Failed to determine if controller's node %q is associated with machine %q: %v", r.nodeName, machine.Name, err)
		return true
	}

	// When the UID of the machine's node reference and this controller's actual node match then then the request is to
	// delete the machine this machine-controller is running on. Return false to not allow machine controller to delete its
	// own machine.
	return node.UID != machine.Status.NodeRef.UID
}

func (r *ReconcileMachine) deleteNode(ctx context.Context, name string) error {
	var node corev1.Node
	if err := r.Client.Get(ctx, client.ObjectKey{Name: name}, &node); err != nil {
		if apierrors.IsNotFound(err) {
			klog.V(2).Infof("Node %q not found", name)
			return nil
		}
		klog.Errorf("Failed to get node %q: %v", name, err)
		return err
	}
	return r.Client.Delete(ctx, &node)
}
