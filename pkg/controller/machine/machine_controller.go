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
	"path"
	"sync"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"
	"sigs.k8s.io/cluster-api/api/v1alpha2"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha2"
	"sigs.k8s.io/cluster-api/pkg/controller/external"
	"sigs.k8s.io/cluster-api/pkg/controller/remote"
	capierrors "sigs.k8s.io/cluster-api/pkg/errors"
	"sigs.k8s.io/cluster-api/pkg/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const controllerName = "machine-controller"

// Add creates a new Machine Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	r := newReconciler(mgr)
	c, err := addController(mgr, r)
	r.controller = c
	return err
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) *ReconcileMachine {
	return &ReconcileMachine{
		Client:   mgr.GetClient(),
		scheme:   mgr.GetScheme(),
		recorder: mgr.GetEventRecorderFor(controllerName),
	}
}

// addController adds a new Controller to mgr with r as the reconcile.Reconciler
func addController(mgr manager.Manager, r reconcile.Reconciler) (controller.Controller, error) {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return nil, err
	}

	// Watch for changes to Machine
	err = c.Watch(
		&source.Kind{Type: &clusterv1.Machine{}},
		&handler.EnqueueRequestForObject{},
	)
	if err != nil {
		return nil, err
	}

	return c, nil
}

// ReconcileMachine reconciles a Machine object
type ReconcileMachine struct {
	client.Client
	scheme           *runtime.Scheme
	controller       controller.Controller
	recorder         record.EventRecorder
	externalWatchers sync.Map
}

// Reconcile reads that state of the cluster for a Machine object and makes changes based on the state read
// and what is in the Machine.Spec
func (r *ReconcileMachine) Reconcile(request reconcile.Request) (_ reconcile.Result, reterr error) {
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

	// Store Machine early state to allow patching.
	patchMachine := client.MergeFrom(m.DeepCopy())

	// Always issue a Patch for the Machine object and its status after each reconciliation.
	// TODO(vincepri): Figure out if we should bubble up the errors from Patch to the controller.
	defer func() {
		gvk := m.GroupVersionKind()
		if err := r.Client.Patch(ctx, m, patchMachine); err != nil {
			klog.Errorf("Error Patching Machine %q in namespace %q: %v", m.Name, m.Namespace, err)
			if reterr == nil {
				reterr = err
			}
			return
		}
		// TODO(vincepri): This is a hack because after a Patch, the object loses TypeMeta information.
		// Remove when https://github.com/kubernetes-sigs/controller-runtime/issues/526 is fixed.
		m.SetGroupVersionKind(gvk)
		if err := r.Client.Status().Patch(ctx, m, patchMachine); err != nil {
			klog.Errorf("Error Patching Machine status %q in namespace %q: %v", m.Name, m.Namespace, err)
			if reterr == nil {
				reterr = err
			}
		}
	}()

	// Cluster might be nil as some providers might not require a cluster object
	// for machine management.
	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, m.ObjectMeta)
	if errors.Cause(err) == util.ErrNoCluster {
		klog.Infof("Machine %q in namespace %q doesn't specify %q label, assuming nil cluster",
			m.Name, m.Namespace, clusterv1.MachineClusterLabelName)
	} else if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to get cluster %q for machine %q in namespace %q",
			m.Labels[clusterv1.MachineClusterLabelName], m.Name, m.Namespace)
	}

	if cluster != nil && shouldAdopt(m) {
		m.OwnerReferences = util.EnsureOwnerRef(m.OwnerReferences, metav1.OwnerReference{
			APIVersion: cluster.APIVersion,
			Kind:       cluster.Kind,
			Name:       cluster.Name,
			UID:        cluster.UID,
		})
	}

	// If the Machine hasn't been deleted and doesn't have a finalizer, add one.
	if m.ObjectMeta.DeletionTimestamp.IsZero() {
		if !util.Contains(m.Finalizers, clusterv1.MachineFinalizer) {
			m.Finalizers = append(m.ObjectMeta.Finalizers, clusterv1.MachineFinalizer)
			if err := r.Client.Patch(ctx, m, patchMachine); err != nil {
				return reconcile.Result{}, errors.Wrapf(err, "failed to add finalizer to Machine %q in namespace %q", m.Name, m.Namespace)
			}
			// Since adding the finalizer updates the object return to avoid later update issues
			return reconcile.Result{Requeue: true}, nil
		}
	}

	if err := r.reconcile(ctx, cluster, m); err != nil {
		if requeueErr, ok := errors.Cause(err).(capierrors.HasRequeueAfterError); ok {
			klog.Infof("Reconciliation for Machine %q in namespace %q asked to requeue: %v", m.Name, m.Namespace, err)
			return reconcile.Result{Requeue: true, RequeueAfter: requeueErr.GetRequeueAfter()}, nil
		}
		return reconcile.Result{}, err
	}

	if !m.ObjectMeta.DeletionTimestamp.IsZero() {
		if err := r.isDeleteNodeAllowed(context.Background(), m); err != nil {
			switch err {
			case errNilNodeRef:
				klog.V(2).Infof("Deleting node is not allowed for machine %q: %v", m.Name, err)
			case errNoControlPlaneNodes, errLastControlPlaneNode:
				klog.V(2).Infof("Deleting node %q is not allowed for machine %q: %v", m.Status.NodeRef.Name, m.Name, err)
			default:
				klog.Errorf("IsDeleteNodeAllowed check failed for machine %q: %v", m.Name, err)
				return reconcile.Result{}, err
			}
		} else {
			klog.Infof("Deleting node %q for machine %q", m.Status.NodeRef.Name, m.Name)
			if err := r.deleteNode(ctx, cluster, m.Status.NodeRef.Name); err != nil && !apierrors.IsNotFound(err) {
				klog.Errorf("Error deleting node %q for machine %q: %v", m.Status.NodeRef.Name, m.Name, err)
				return reconcile.Result{}, err
			}
		}

		if err := r.isDeleteReady(ctx, m); err != nil {
			if requeueErr, ok := errors.Cause(err).(capierrors.HasRequeueAfterError); ok {
				klog.Infof("Reconciliation for Machine %q in namespace %q asked to requeue: %v", m.Name, m.Namespace, err)
				return reconcile.Result{Requeue: true, RequeueAfter: requeueErr.GetRequeueAfter()}, nil
			}
			return reconcile.Result{}, err
		}

		m.ObjectMeta.Finalizers = util.Filter(m.ObjectMeta.Finalizers, clusterv1.MachineFinalizer)
	}

	return reconcile.Result{}, nil
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

	if err := r.Client.List(ctx, machineList, client.InNamespace(namespace), client.MatchingLabels(labels)); err != nil {
		return nil, errors.Wrap(err, "failed to list machines")
	}

	machines := make([]*clusterv1.Machine, len(machineList.Items))
	for i := range machineList.Items {
		machines[i] = &machineList.Items[i]
	}

	return machines, nil
}

// isDeleteReady returns an error if any of Boostrap.ConfigRef or InfrastructureRef referenced objects still exists.
func (r *ReconcileMachine) isDeleteReady(ctx context.Context, m *v1alpha2.Machine) error {
	if m.Spec.Bootstrap.ConfigRef != nil {
		_, err := external.Get(r.Client, m.Spec.Bootstrap.ConfigRef, m.Namespace)
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return errors.Wrapf(err, "failed to get %s %q for Machine %q in namespace %q",
				path.Join(m.Spec.Bootstrap.ConfigRef.APIVersion, m.Spec.Bootstrap.ConfigRef.Kind),
				m.Spec.Bootstrap.ConfigRef.Name, m.Name, m.Namespace)
		}
		return &capierrors.RequeueAfterError{RequeueAfter: 10 * time.Second}
	}

	if _, err := external.Get(r.Client, &m.Spec.InfrastructureRef, m.Namespace); err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrapf(err, "failed to get %s %q for Machine %q in namespace %q",
			path.Join(m.Spec.InfrastructureRef.APIVersion, m.Spec.InfrastructureRef.Kind),
			m.Spec.InfrastructureRef.Name, m.Name, m.Namespace)
	} else if err == nil {
		return &capierrors.RequeueAfterError{RequeueAfter: 10 * time.Second}
	}

	return nil
}

func shouldAdopt(m *v1alpha2.Machine) bool {
	return !util.HasOwner(m.OwnerReferences, v1alpha2.GroupVersion.String(), []string{"MachineSet", "Cluster"})
}
