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

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	controllerError "sigs.k8s.io/cluster-api/pkg/controller/error"
	"sigs.k8s.io/cluster-api/pkg/controller/remote"
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

	if reconcileFinalizers(m, cluster) {
		if err := r.Client.Update(ctx, m); err != nil {
			klog.Infof("Failed to add finalizers to machine %q: %v", name, err)
			return reconcile.Result{}, err
		}
		// Since adding the finalizer updates the object return to avoid later update issues
		return reconcile.Result{Requeue: true}, nil
	}

	if !m.ObjectMeta.DeletionTimestamp.IsZero() {
		// no-op if finalizer has been removed.
		if !util.Contains(m.ObjectMeta.Finalizers, clusterv1.MachineFinalizer) {
			klog.Infof("Reconciling machine %q causes a no-op as there is no finalizer", name)
			return reconcile.Result{}, nil
		}

		if !r.isDeleteAllowed(m) {
			klog.Infof("Deleting machine hosting this controller is not allowed. Skipping reconciliation of machine %q", name)
			return reconcile.Result{}, nil
		}

		klog.Infof("Reconciling machine %q triggers delete", name)
		if err := r.actuator.Delete(ctx, cluster, m); err != nil {
			if requeueErr, ok := errors.Cause(err).(controllerError.HasRequeueAfterError); ok {
				klog.Infof("Actuator returned requeue-after error: %v", requeueErr)
				return reconcile.Result{Requeue: true, RequeueAfter: requeueErr.GetRequeueAfter()}, nil
			}

			klog.Errorf("Failed to delete machine %q: %v", name, err)
			return reconcile.Result{}, err
		}

		if err := r.isDeleteNodeAllowed(context.Background(), m); err != nil {
			switch err {
			case errNilNodeRef:
				klog.V(2).Infof("Deleting node is not allowed for machine %q: %v", m.Name, err)
			case errNoControlPlaneNodes, errLastControlPlaneNode:
				klog.V(2).Infof("Deleting node %q is not allowed for machine %q: %v", m.Status.NodeRef.Name, m.Name, err)
			default:
				klog.Errorf("IsDeleteNodeAllowed check failed for machine %q: %v", name, err)
				return reconcile.Result{}, err
			}
		} else {
			klog.Infof("Deleting node %q for machine %q", m.Status.NodeRef.Name, m.Name)
			if err := r.deleteNode(ctx, cluster, m.Status.NodeRef.Name); err != nil && !apierrors.IsNotFound(err) {
				klog.Errorf("Error deleting node %q for machine %q: %v", m.Status.NodeRef.Name, name, err)
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
			if requeueErr, ok := errors.Cause(err).(controllerError.HasRequeueAfterError); ok {
				klog.Infof("Actuator returned requeue-after error: %v", requeueErr)
				return reconcile.Result{Requeue: true, RequeueAfter: requeueErr.GetRequeueAfter()}, nil
			}

			klog.Errorf(`Error updating machine "%s/%s": %v`, m.Namespace, name, err)
			return reconcile.Result{}, err
		}

		return reconcile.Result{}, nil
	}

	// Machine resource created. Machine does not yet exist.
	klog.Infof("Reconciling machine object %v triggers idempotent create.", m.ObjectMeta.Name)
	if err := r.actuator.Create(ctx, cluster, m); err != nil {
		if requeueErr, ok := errors.Cause(err).(controllerError.HasRequeueAfterError); ok {
			klog.Infof("Actuator returned requeue-after error: %v", requeueErr)
			return reconcile.Result{Requeue: true, RequeueAfter: requeueErr.GetRequeueAfter()}, nil
		}

		klog.Warningf("Failed to create machine %q: %v", name, err)
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

// reconcileFinalizers appends any missing finalizers to the machine
// and returns true if the api server needs to be updated.
func reconcileFinalizers(m *clusterv1.Machine, cluster *clusterv1.Cluster) bool {
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
			return true
		}
	}
	return false
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

// isDeletedAllowed returns false if the Machine we're trying to delete is the
// Machine hosting this controller. This method is meant to be functional
// only when the controllers are running in the workload cluster.
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

var (
	errNilNodeRef           = errors.New("noderef is nil")
	errLastControlPlaneNode = errors.New("last control plane member")
	errNoControlPlaneNodes  = errors.New("no control plane members")
)

// isDeleteNodeAllowed returns nil only if the Machine's NodeRef is not nil
// and if the Machine is not the last control plane node in the cluster.
func (r *ReconcileMachine) isDeleteNodeAllowed(ctx context.Context, machine *clusterv1.Machine) error {
	// Cannot delete something that doesn't exist.
	if machine.Status.NodeRef == nil {
		return errNilNodeRef
	}

	// Get all of the machines that belong to this cluster.
	machines, err := r.getMachinesInCluster(ctx, machine.Namespace, machine.Labels[clusterv1.MachineClusterLabelName])
	if err != nil {
		return err
	}

	// Whether or not it is okay to delete the NodeRef depends on the
	// number of remaining control plane members and whether or not this
	// machine is one of them.
	switch numControlPlaneMachines := len(util.GetControlPlaneMachines(machines)); {
	case numControlPlaneMachines == 0:
		// Do not delete the NodeRef if there are no remaining members of
		// the control plane.
		return errNoControlPlaneNodes
	case numControlPlaneMachines == 1 && util.IsControlPlaneMachine(machine):
		// Do not delete the NodeRef if this is the last member of the
		// control plane.
		return errLastControlPlaneNode
	default:
		// Otherwise it is okay to delete the NodeRef.
		return nil
	}
}

func (r *ReconcileMachine) deleteNode(ctx context.Context, cluster *clusterv1.Cluster, name string) error {
	if cluster == nil {
		// Try to retrieve the Node from the local cluster, if no Cluster reference is found.
		var node corev1.Node
		if err := r.Client.Get(ctx, client.ObjectKey{Name: name}, &node); err != nil {
			return err
		}
		return r.Client.Delete(ctx, &node)
	}

	// Otherwise, proceed to get the remote cluster client and get the Node.
	remoteClient, err := remote.NewClusterClient(r.Client, cluster)
	if err != nil {
		klog.Errorf("Error creating a remote client for cluster %q while deleting Machine %q, won't retry: %v",
			cluster.Name, name, err)
		return nil
	}

	corev1Remote, err := remoteClient.CoreV1()
	if err != nil {
		klog.Errorf("Error creating a remote client for cluster %q while deleting Machine %q, won't retry: %v",
			cluster.Name, name, err)
		return nil
	}

	return corev1Remote.Nodes().Delete(name, &metav1.DeleteOptions{})
}

// getMachinesInCluster returns all of the Machine objects that belong to the
// same cluster as the provided Machine
func (r *ReconcileMachine) getMachinesInCluster(ctx context.Context, namespace, name string) ([]*clusterv1.Machine, error) {
	if name == "" {
		return nil, nil
	}

	machineList := &clusterv1.MachineList{}
	labels := map[string]string{clusterv1.MachineClusterLabelName: name}
	listOptions := client.InNamespace(namespace).MatchingLabels(labels)

	if err := r.Client.List(ctx, listOptions, machineList); err != nil {
		return nil, errors.Wrap(err, "failed to list machines")
	}

	machines := make([]*clusterv1.Machine, len(machineList.Items))
	for i := range machineList.Items {
		machines[i] = &machineList.Items[i]
	}

	return machines, nil
}
