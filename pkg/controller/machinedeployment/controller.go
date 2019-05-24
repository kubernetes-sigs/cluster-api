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

package machinedeployment

import (
	"context"
	"reflect"

	"github.com/openshift/cluster-api/pkg/apis/cluster/v1alpha1"
	"github.com/openshift/cluster-api/pkg/apis/machine/common"
	"github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
	"github.com/openshift/cluster-api/pkg/util"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var (
	// controllerKind contains the schema.GroupVersionKind for this controller type.
	controllerKind = v1beta1.SchemeGroupVersion.WithKind("MachineDeployment")
)

// controllerName is the name of this controller
const controllerName = "machinedeployment_controller"

// ReconcileMachineDeployment reconciles a MachineDeployment object.
type ReconcileMachineDeployment struct {
	client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
}

// newReconciler returns a new reconcile.Reconciler.
func newReconciler(mgr manager.Manager) *ReconcileMachineDeployment {
	return &ReconcileMachineDeployment{Client: mgr.GetClient(), scheme: mgr.GetScheme(), recorder: mgr.GetEventRecorderFor(controllerName)}
}

// Add creates a new MachineDeployment Controller and adds it to the Manager with default RBAC.
func Add(mgr manager.Manager) error {
	r := newReconciler(mgr)
	return add(mgr, newReconciler(mgr), r.MachineSetToDeployments)
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler.
func add(mgr manager.Manager, r reconcile.Reconciler, mapFn handler.ToRequestsFunc) error {
	// Create a new controller.
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to MachineDeployment.
	err = c.Watch(&source.Kind{
		Type: &v1beta1.MachineDeployment{}},
		&handler.EnqueueRequestForObject{},
	)
	if err != nil {
		return err
	}

	// Watch for changes to MachineSet and reconcile the owner MachineDeployment.
	err = c.Watch(
		&source.Kind{Type: &v1beta1.MachineSet{}},
		&handler.EnqueueRequestForOwner{OwnerType: &v1beta1.MachineDeployment{}, IsController: true},
	)
	if err != nil {
		return err
	}

	// Map MachineSet changes to MachineDeployment.
	err = c.Watch(
		&source.Kind{Type: &v1beta1.MachineSet{}},
		&handler.EnqueueRequestsFromMapFunc{ToRequests: mapFn},
	)
	if err != nil {
		return err
	}

	return nil
}

func (r *ReconcileMachineDeployment) getMachineSetsForDeployment(d *v1beta1.MachineDeployment) ([]*v1beta1.MachineSet, error) {
	// List all MachineSets to find those we own but that no longer match our selector.
	machineSets := &v1beta1.MachineSetList{}
	if err := r.Client.List(context.Background(), machineSets, client.InNamespace(d.Namespace)); err != nil {
		return nil, err
	}

	filteredMS := make([]*v1beta1.MachineSet, 0, len(machineSets.Items))
	for idx := range machineSets.Items {
		ms := &machineSets.Items[idx]

		selector, err := metav1.LabelSelectorAsSelector(&d.Spec.Selector)
		if err != nil {
			klog.Errorf("Skipping machineset %v, failed to get label selector from spec selector.", ms.Name)
			continue
		}

		// If a deployment with a nil or empty selector creeps in, it should match nothing, not everything.
		if selector.Empty() {
			klog.Warningf("Skipping machineset %v as the selector is empty.", ms.Name)
			continue
		}

		if !selector.Matches(labels.Set(ms.Labels)) {
			klog.V(4).Infof("Skipping machineset %v, label mismatch.", ms.Name)
			continue
		}

		// Attempt to adopt machine if it meets previous conditions and it has no controller references.
		if metav1.GetControllerOf(ms) == nil {
			if err := r.adoptOrphan(d, ms); err != nil {
				klog.Warningf("Failed to adopt MachineSet %q into MachineDeployment %q: %v", ms.Name, d.Name, err)
				continue
			}
		}

		if !metav1.IsControlledBy(ms, d) {
			continue
		}

		filteredMS = append(filteredMS, ms)
	}

	return filteredMS, nil
}

func (r *ReconcileMachineDeployment) adoptOrphan(deployment *v1beta1.MachineDeployment, machineSet *v1beta1.MachineSet) error {
	newRef := *metav1.NewControllerRef(deployment, controllerKind)
	machineSet.OwnerReferences = append(machineSet.OwnerReferences, newRef)
	return r.Client.Update(context.Background(), machineSet)
}

// Reconcile reads that state of the cluster for a MachineDeployment object and makes changes based on the state read
// and what is in the MachineDeployment.Spec
// +kubebuilder:rbac:groups=machine.openshift.io,resources=machinedeployments,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileMachineDeployment) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the MachineDeployment instance
	d := &v1beta1.MachineDeployment{}
	ctx := context.TODO()
	if err := r.Get(context.TODO(), request.NamespacedName, d); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}
	result, err := r.reconcile(ctx, d)
	if err != nil {
		klog.Errorf("Failed to reconcile MachineDeployment %q: %v", request.NamespacedName, err)
		r.recorder.Eventf(d, corev1.EventTypeWarning, "ReconcileError", "%v", err)
	}

	return result, err
}

func (r *ReconcileMachineDeployment) reconcile(ctx context.Context, d *v1beta1.MachineDeployment) (reconcile.Result, error) {
	v1beta1.PopulateDefaultsMachineDeployment(d)

	everything := metav1.LabelSelector{}
	if reflect.DeepEqual(d.Spec.Selector, &everything) {
		if d.Status.ObservedGeneration < d.Generation {
			d.Status.ObservedGeneration = d.Generation
			if err := r.Status().Update(context.Background(), d); err != nil {
				klog.Warningf("Failed to update status for deployment %v. %v", d.Name, err)
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{}, nil
	}

	// Make sure that label selector can match template's labels.
	// TODO(vincepri): Move to a validation (admission) webhook when supported.
	selector, err := metav1.LabelSelectorAsSelector(&d.Spec.Selector)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to parse MachineDeployment %q label selector", d.Name)
	}

	if !selector.Matches(labels.Set(d.Spec.Template.Labels)) {
		return reconcile.Result{}, errors.Errorf("failed validation on MachineDeployment %q label selector, cannot match any machines ", d.Name)
	}

	// Cluster might be nil as some providers might not require a cluster object
	// for machine management.
	cluster, err := r.getCluster(d)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Set the ownerRef with foreground deletion if there is a linked cluster.
	if cluster != nil && len(d.OwnerReferences) == 0 {
		blockOwnerDeletion := true
		d.OwnerReferences = append(d.OwnerReferences, metav1.OwnerReference{
			APIVersion:         cluster.APIVersion,
			Kind:               cluster.Kind,
			Name:               cluster.Name,
			UID:                cluster.UID,
			BlockOwnerDeletion: &blockOwnerDeletion,
		})
	}

	// Add foregroundDeletion finalizer if MachineDeployment isn't deleted and linked to a cluster.
	if cluster != nil &&
		d.ObjectMeta.DeletionTimestamp.IsZero() &&
		!util.Contains(d.Finalizers, metav1.FinalizerDeleteDependents) {

		d.Finalizers = append(d.ObjectMeta.Finalizers, metav1.FinalizerDeleteDependents)

		if err := r.Client.Update(context.Background(), d); err != nil {
			klog.Infof("Failed to add finalizers to MachineSet %q: %v", d.Name, err)
			return reconcile.Result{}, err
		}

		// Since adding the finalizer updates the object return to avoid later update issues
		return reconcile.Result{}, nil
	}

	msList, err := r.getMachineSetsForDeployment(d)
	if err != nil {
		return reconcile.Result{}, err
	}

	machineMap, err := r.getMachineMapForDeployment(d, msList)
	if err != nil {
		return reconcile.Result{}, err
	}

	if d.DeletionTimestamp != nil {
		return reconcile.Result{}, r.sync(d, msList, machineMap)
	}

	if d.Spec.Paused {
		return reconcile.Result{}, r.sync(d, msList, machineMap)
	}

	switch d.Spec.Strategy.Type {
	case common.RollingUpdateMachineDeploymentStrategyType:
		return reconcile.Result{}, r.rolloutRolling(d, msList, machineMap)
	}

	return reconcile.Result{}, errors.Errorf("unexpected deployment strategy type: %s", d.Spec.Strategy.Type)
}

func (r *ReconcileMachineDeployment) getCluster(d *v1beta1.MachineDeployment) (*v1alpha1.Cluster, error) {
	if d.Spec.Template.Labels[v1beta1.MachineClusterLabelName] == "" {
		klog.Infof("Deployment %q in namespace %q doesn't specify %q label, assuming nil cluster", d.Name, d.Namespace, v1beta1.MachineClusterLabelName)
		return nil, nil
	}

	cluster := &v1alpha1.Cluster{}
	key := client.ObjectKey{
		Namespace: d.Namespace,
		Name:      d.Spec.Template.Labels[v1beta1.MachineClusterLabelName],
	}

	if err := r.Client.Get(context.Background(), key, cluster); err != nil {
		return nil, err
	}

	return cluster, nil
}

// getMachineDeploymentsForMachineSet returns a list of Deployments that potentially
// match a MachineSet.
func (r *ReconcileMachineDeployment) getMachineDeploymentsForMachineSet(ms *v1beta1.MachineSet) []*v1beta1.MachineDeployment {
	if len(ms.Labels) == 0 {
		klog.Warningf("No machine deployments found for MachineSet %v because it has no labels", ms.Name)
		return nil
	}

	dList := &v1beta1.MachineDeploymentList{}
	if err := r.Client.List(context.Background(), dList, client.InNamespace(ms.Namespace)); err != nil {
		klog.Warningf("Failed to list machine deployments: %v", err)
		return nil
	}

	deployments := make([]*v1beta1.MachineDeployment, 0, len(dList.Items))
	for idx, d := range dList.Items {
		selector, err := metav1.LabelSelectorAsSelector(&d.Spec.Selector)
		if err != nil {
			continue
		}

		// If a deployment with a nil or empty selector creeps in, it should match nothing, not everything.
		if selector.Empty() || !selector.Matches(labels.Set(ms.Labels)) {
			continue
		}

		deployments = append(deployments, &dList.Items[idx])
	}

	return deployments
}

// getMachineMapForDeployment returns the Machines managed by a Deployment.
//
// It returns a map from MachineSet UID to a list of Machines controlled by that MS,
// according to the Machine's ControllerRef.
func (r *ReconcileMachineDeployment) getMachineMapForDeployment(d *v1beta1.MachineDeployment, msList []*v1beta1.MachineSet) (map[types.UID]*v1beta1.MachineList, error) {
	// TODO(droot): double check if previous selector maps correctly to new one.
	// _, err := metav1.LabelSelectorAsSelector(&d.Spec.Selector)

	// Get all Machines that potentially belong to this Deployment.
	selector, err := metav1.LabelSelectorAsMap(&d.Spec.Selector)
	if err != nil {
		return nil, err
	}

	machines := &v1beta1.MachineList{}
	if err = r.Client.List(context.Background(), machines, client.InNamespace(d.Namespace), client.MatchingLabels(selector)); err != nil {
		return nil, err
	}

	// Group Machines by their controller (if it's in msList).
	machineMap := make(map[types.UID]*v1beta1.MachineList, len(msList))
	for _, ms := range msList {
		machineMap[ms.UID] = &v1beta1.MachineList{}
	}

	for idx := range machines.Items {
		machine := &machines.Items[idx]

		// Do not ignore inactive Machines because Recreate Deployments need to verify that no
		// Machines from older versions are running before spinning up new Machines.
		controllerRef := metav1.GetControllerOf(machine)
		if controllerRef == nil {
			continue
		}

		// Only append if we care about this UID.
		if machineList, ok := machineMap[controllerRef.UID]; ok {
			machineList.Items = append(machineList.Items, *machine)
		}
	}

	return machineMap, nil
}

func (r *ReconcileMachineDeployment) MachineSetToDeployments(o handler.MapObject) []reconcile.Request {
	result := []reconcile.Request{}

	ms := &v1beta1.MachineSet{}
	key := client.ObjectKey{Namespace: o.Meta.GetNamespace(), Name: o.Meta.GetName()}
	if err := r.Client.Get(context.Background(), key, ms); err != nil {
		klog.Errorf("Unable to retrieve Machineset %v from store: %v", key, err)
		return nil
	}

	for _, ref := range ms.ObjectMeta.OwnerReferences {
		if ref.Controller != nil && *ref.Controller {
			return result
		}
	}

	mds := r.getMachineDeploymentsForMachineSet(ms)
	if len(mds) == 0 {
		klog.V(4).Infof("Found no machine set for machine: %v", ms.Name)
		return nil
	}

	for _, md := range mds {
		name := client.ObjectKey{Namespace: md.Namespace, Name: md.Name}
		result = append(result, reconcile.Request{NamespacedName: name})
	}

	return result
}
