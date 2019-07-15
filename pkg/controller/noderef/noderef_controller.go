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

package noderef

import (
	"context"
	"time"

	"github.com/pkg/errors"
	apicorev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/controller/noderefutil"
	"sigs.k8s.io/cluster-api/pkg/controller/remote"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// controllerName is the name of this controller
const controllerName = "noderef-controller"

var (
	ErrNodeNotFound = errors.New("cannot find node with matching ProviderID")
)

// Add creates a new NodeRef Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileNodeRef{Client: mgr.GetClient(), scheme: mgr.GetScheme(), recorder: mgr.GetRecorder(controllerName)}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Machines.
	return c.Watch(&source.Kind{Type: &v1alpha1.Machine{}}, &handler.EnqueueRequestForObject{})
}

var _ reconcile.Reconciler = &ReconcileNodeRef{}

// ReconcileNodeRef reconciles a Machine object to assign a NodeRef.
type ReconcileNodeRef struct {
	client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
}

// Reconcile responds to Machine events to assign a NodeRef.
// +kubebuilder:rbac:groups=,resources=secrets,verbs=get;list;watch
func (r *ReconcileNodeRef) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	klog.Infof("Reconcile request for Machine %q in namespace %q", request.Name, request.Namespace)
	ctx := context.Background()

	// Fetch the Machine instance.
	machine := &v1alpha1.Machine{}
	err := r.Get(ctx, request.NamespacedName, machine)
	if err != nil {
		if apierrors.IsNotFound(err) {
			klog.V(2).Infof("Machine %q in namespace %q is not found, won't reconcile", machine.Name, machine.Namespace)
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	// Check that the Machine hasn't been deleted or in the process.
	if !machine.DeletionTimestamp.IsZero() {
		klog.V(2).Infof("Machine %q in namespace %q has been deleted, won't reconcile", machine.Name, machine.Namespace)
		return reconcile.Result{}, nil
	}

	// Check that the Machine doesn't already have a NodeRef.
	if machine.Status.NodeRef != nil {
		klog.V(2).Infof("Machine %q in namespace %q already has a NodeRef, won't reconcile", machine.Name, machine.Namespace)
		return reconcile.Result{}, nil
	}

	// Check that the Machine has a cluster label.
	if machine.Labels[v1alpha1.MachineClusterLabelName] == "" {
		klog.V(2).Infof("Machine %q in namespace %q doesn't specify %q label, won't reconcile", machine.Name, machine.Namespace,
			v1alpha1.MachineClusterLabelName)
		return reconcile.Result{}, nil
	}

	// Check that the Machine has a valid Cluster associated with it.
	cluster, err := r.getCluster(ctx, machine)
	if err != nil {
		if apierrors.IsNotFound(err) {
			klog.Warningf("Cannot find a Cluster for Machine %q in namespace %q, won't reconcile", machine.Name, machine.Namespace)
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, err
	}

	// Check that the Machine has a valid ProviderID.
	if machine.Spec.ProviderID == nil || *machine.Spec.ProviderID == "" {
		klog.Warningf("Machine %q in namespace %q doesn't have a valid ProviderID, retrying later", machine.Name, machine.Namespace)
		return reconcile.Result{RequeueAfter: 30 * time.Second}, nil
	}

	result, err := r.reconcile(ctx, cluster, machine)
	if err != nil {
		if err == ErrNodeNotFound {
			klog.Warningf("Failed to assign NodeRef to Machine %q: cannot find a matching Node in namespace %q, retrying later", machine.Name, machine.Namespace)
			return reconcile.Result{RequeueAfter: 10 * time.Second}, nil
		}

		klog.Errorf("Failed to assign NodeRef to Machine %q: %v", request.NamespacedName, err)
		r.recorder.Event(machine, apicorev1.EventTypeWarning, "FailedSetNodeRef", err.Error())
		return reconcile.Result{}, err
	}

	klog.Infof("Set Machine's (%q in namespace %q) NodeRef to %q", machine.Name, machine.Namespace, machine.Status.NodeRef.Name)
	r.recorder.Event(machine, apicorev1.EventTypeNormal, "SuccessfulSetNodeRef", machine.Status.NodeRef.Name)
	return result, nil
}

func (r *ReconcileNodeRef) reconcile(ctx context.Context, cluster *v1alpha1.Cluster, machine *v1alpha1.Machine) (reconcile.Result, error) {
	providerID, err := noderefutil.NewProviderID(*machine.Spec.ProviderID)
	if err != nil {
		return reconcile.Result{}, err
	}

	clusterClient, err := remote.NewClusterClient(r.Client, cluster)
	if err != nil {
		return reconcile.Result{}, err
	}

	corev1Client, err := clusterClient.CoreV1()
	if err != nil {
		return reconcile.Result{}, err
	}

	// Get the Node reference.
	nodeRef, err := r.getNodeReference(corev1Client, providerID)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Update Machine.
	machine.Status.NodeRef = nodeRef
	if err := r.Client.Status().Update(ctx, machine); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileNodeRef) getCluster(ctx context.Context, machine *v1alpha1.Machine) (*v1alpha1.Cluster, error) {
	cluster := &v1alpha1.Cluster{}
	key := client.ObjectKey{
		Namespace: machine.Namespace,
		Name:      machine.Labels[v1alpha1.MachineClusterLabelName],
	}

	if err := r.Client.Get(ctx, key, cluster); err != nil {
		return nil, err
	}

	return cluster, nil
}

func (r *ReconcileNodeRef) getNodeReference(client corev1.NodesGetter, providerID *noderefutil.ProviderID) (*apicorev1.ObjectReference, error) {
	listOpt := metav1.ListOptions{}

	for {
		nodeList, err := client.Nodes().List(listOpt)
		if err != nil {
			return nil, err
		}

		for _, node := range nodeList.Items {
			nodeProviderID, err := noderefutil.NewProviderID(node.Spec.ProviderID)
			if err != nil {
				klog.V(3).Infof("Failed to parse ProviderID for Node %q: %v", node.Name, err)
				continue
			}

			if providerID.Equals(nodeProviderID) {
				return &apicorev1.ObjectReference{
					Kind:       node.Kind,
					APIVersion: node.APIVersion,
					Name:       node.Name,
					UID:        node.UID,
				}, nil
			}
		}

		listOpt.Continue = nodeList.Continue
		if listOpt.Continue == "" {
			break
		}
	}

	return nil, ErrNodeNotFound
}
