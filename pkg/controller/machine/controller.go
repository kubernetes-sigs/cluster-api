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
	"errors"
	"os"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

const NodeNameEnvVar = "NODE_NAME"

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
		klog.Warningf("environment variable %v is not set, this controller will not protect against deleting its own machine", NodeNameEnvVar)
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
	err = c.Watch(&source.Kind{Type: &clusterv1.Machine{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileMachine{}

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
// +kubebuilder:rbac:groups=cluster.k8s.io,resources=machines,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileMachine) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// TODO(mvladev): Can context be passed from Kubebuilder?
	ctx := context.TODO()
	// Fetch the Machine instance
	m := &clusterv1.Machine{}
	err := r.Get(ctx, request.NamespacedName, m)
	if err != nil {
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
	klog.Infof("Running reconcile Machine for %s\n", name)

	// If object hasn't been deleted and doesn't have a finalizer, add one
	// Add a finalizer to newly created objects.
	if m.ObjectMeta.DeletionTimestamp.IsZero() &&
		!util.Contains(m.ObjectMeta.Finalizers, clusterv1.MachineFinalizer) {
		m.Finalizers = append(m.Finalizers, clusterv1.MachineFinalizer)
		if err = r.Update(ctx, m); err != nil {
			klog.Infof("failed to add finalizer to machine object %v due to error %v.", name, err)
			return reconcile.Result{}, err
		}
	}

	if !m.ObjectMeta.DeletionTimestamp.IsZero() {
		// no-op if finalizer has been removed.
		if !util.Contains(m.ObjectMeta.Finalizers, clusterv1.MachineFinalizer) {
			klog.Infof("reconciling machine object %v causes a no-op as there is no finalizer.", name)
			return reconcile.Result{}, nil
		}
		if !r.isDeleteAllowed(m) {
			klog.Infof("Skipping reconciling of machine object %v", name)
			return reconcile.Result{}, nil
		}
		klog.Infof("reconciling machine object %v triggers delete.", name)
		if err := r.delete(ctx, m); err != nil {
			klog.Errorf("Error deleting machine object %v; %v", name, err)
			return reconcile.Result{}, err
		}

		// Remove finalizer on successful deletion.
		klog.Infof("machine object %v deletion successful, removing finalizer.", name)
		m.ObjectMeta.Finalizers = util.Filter(m.ObjectMeta.Finalizers, clusterv1.MachineFinalizer)
		if err := r.Client.Update(context.Background(), m); err != nil {
			klog.Errorf("Error removing finalizer from machine object %v; %v", name, err)
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	cluster, err := r.getCluster(ctx, m)
	if err != nil {
		return reconcile.Result{}, err
	}

	exist, err := r.actuator.Exists(ctx, cluster, m)
	if err != nil {
		klog.Errorf("Error checking existence of machine instance for machine object %v; %v", name, err)
		return reconcile.Result{}, err
	}
	if exist {
		klog.Infof("Reconciling machine object %v triggers idempotent update.", name)
		err := r.update(ctx, m)
		if err != nil {
			if requeueErr, ok := err.(*controllerError.RequeueAfterError); ok {
				klog.Infof("Actuator returned requeue-after error: %v", requeueErr)
				return reconcile.Result{Requeue: true, RequeueAfter: requeueErr.RequeueAfter}, nil
			}
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}
	// Machine resource created. Machine does not yet exist.
	klog.Infof("Reconciling machine object %v triggers idempotent create.", m.ObjectMeta.Name)
	if err := r.create(ctx, m); err != nil {
		klog.Warningf("unable to create machine %v: %v", name, err)
		if requeueErr, ok := err.(*controllerError.RequeueAfterError); ok {
			klog.Infof("Actuator returned requeue-after error: %v", requeueErr)
			return reconcile.Result{Requeue: true, RequeueAfter: requeueErr.RequeueAfter}, nil
		}
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (c *ReconcileMachine) create(ctx context.Context, machine *clusterv1.Machine) error {
	cluster, err := c.getCluster(ctx, machine)
	if err != nil {
		return err
	}

	return c.actuator.Create(ctx, cluster, machine)
}

func (c *ReconcileMachine) update(ctx context.Context, new_machine *clusterv1.Machine) error {
	cluster, err := c.getCluster(ctx, new_machine)
	if err != nil {
		return err
	}

	// TODO: Assume single master for now.
	// TODO: Assume we never change the role for the machines. (Master->Node, Node->Master, etc)
	return c.actuator.Update(ctx, cluster, new_machine)
}

func (c *ReconcileMachine) delete(ctx context.Context, machine *clusterv1.Machine) error {
	cluster, err := c.getCluster(ctx, machine)
	if err != nil {
		return err
	}

	return c.actuator.Delete(ctx, cluster, machine)
}

func (c *ReconcileMachine) getCluster(ctx context.Context, machine *clusterv1.Machine) (*clusterv1.Cluster, error) {
	clusterList := clusterv1.ClusterList{}
	err := c.Client.List(ctx, client.InNamespace(machine.Namespace), &clusterList)
	if err != nil {
		return nil, err
	}

	switch len(clusterList.Items) {
	case 0:
		return nil, errors.New("no clusters defined")
	case 1:
		return &clusterList.Items[0], nil
	default:
		return nil, errors.New("multiple clusters defined")
	}
}

func (c *ReconcileMachine) isDeleteAllowed(machine *clusterv1.Machine) bool {
	if c.nodeName == "" || machine.Status.NodeRef == nil {
		return true
	}
	if machine.Status.NodeRef.Name != c.nodeName {
		return true
	}
	node := &corev1.Node{}
	err := c.Client.Get(context.Background(), client.ObjectKey{Name: c.nodeName}, node)
	if err != nil {
		klog.Infof("unable to determine if controller's node is associated with machine '%v', error getting node named '%v': %v", machine.Name, c.nodeName, err)
		return true
	}
	// When the UID of the machine's node reference and this controller's actual node match then then the request is to
	// delete the machine this machine-controller is running on. Return false to not allow machine controller to delete its
	// own machine.
	return node.UID != machine.Status.NodeRef.UID
}
