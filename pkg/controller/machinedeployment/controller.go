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
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/common"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// controllerKind contains the schema.GroupVersionKind for this controller type.
var controllerKind = v1alpha1.SchemeGroupVersion.WithKind("MachineDeployment")

// ReconcileMachineDeployment reconciles a MachineDeployment object
type ReconcileMachineDeployment struct {
	client.Client
	scheme *runtime.Scheme
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) *ReconcileMachineDeployment {
	return &ReconcileMachineDeployment{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// Add creates a new MachineDeployment Controller and adds it to the Manager with default RBAC.
func Add(mgr manager.Manager) error {
	r := newReconciler(mgr)
	return add(mgr, newReconciler(mgr), r.MachineSetToDeployments)
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler, mapFn handler.ToRequestsFunc) error {
	// Create a new controller
	c, err := controller.New("machinedeployment-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to MachineDeployment
	err = c.Watch(&source.Kind{Type: &v1alpha1.MachineDeployment{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to MachineSet and reconcile the owner MachineDeployment
	err = c.Watch(&source.Kind{Type: &v1alpha1.MachineSet{}},
		&handler.EnqueueRequestForOwner{OwnerType: &v1alpha1.MachineDeployment{}, IsController: true})
	if err != nil {
		return err
	}

	// Map MachineSet changes to MachineDeployment
	err = c.Watch(
		&source.Kind{Type: &v1alpha1.MachineSet{}},
		&handler.EnqueueRequestsFromMapFunc{ToRequests: mapFn})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileMachineDeployment{}

func (r *ReconcileMachineDeployment) getMachineSetsForDeployment(d *v1alpha1.MachineDeployment) ([]*v1alpha1.MachineSet, error) {
	// List all MachineSets to find those we own but that no longer match our
	// selector.
	machineSets := &v1alpha1.MachineSetList{}
	err := r.List(context.Background(), client.InNamespace(d.Namespace), machineSets)
	if err != nil {
		return nil, err
	}

	// TODO: flush out machine set adoption.

	var filteredMS []*v1alpha1.MachineSet
	for idx, _ := range machineSets.Items {
		ms := &machineSets.Items[idx]
		if metav1.GetControllerOf(ms) == nil || (metav1.GetControllerOf(ms) != nil && !metav1.IsControlledBy(ms, d)) {
			klog.V(4).Infof("%s not controlled by %v", ms.Name, d.Name)
			continue
		}
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
		filteredMS = append(filteredMS, ms)
	}
	return filteredMS, nil
}

// Reconcile reads that state of the cluster for a MachineDeployment object and makes changes based on the state read
// and what is in the MachineDeployment.Spec
// +kubebuilder:rbac:groups=cluster.k8s.io,resources=machinedeployments,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileMachineDeployment) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the MachineDeployment instance
	d := &v1alpha1.MachineDeployment{}
	err := r.Get(context.TODO(), request.NamespacedName, d)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	v1alpha1.PopulateDefaultsMachineDeployment(d)

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

	return reconcile.Result{}, fmt.Errorf("unexpected deployment strategy type: %s", d.Spec.Strategy.Type)
}

// getMachineDeploymentsForMachineSet returns a list of Deployments that potentially
// match a MachineSet.
func (r *ReconcileMachineDeployment) getMachineDeploymentsForMachineSet(ms *v1alpha1.MachineSet) []*v1alpha1.MachineDeployment {
	if len(ms.Labels) == 0 {
		klog.Warningf("no machine deployments found for MachineSet %v because it has no labels", ms.Name)
		return nil
	}

	dList := &v1alpha1.MachineDeploymentList{}
	err := r.Client.List(context.Background(), client.InNamespace(ms.Namespace), dList)
	if err != nil {
		klog.Warningf("failed to list machine deployments, %v", err)
		return nil
	}

	var deployments []*v1alpha1.MachineDeployment
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
func (r *ReconcileMachineDeployment) getMachineMapForDeployment(d *v1alpha1.MachineDeployment, msList []*v1alpha1.MachineSet) (map[types.UID]*v1alpha1.MachineList, error) {
	// TODO(droot): double check if previous selector maps correctly to new one.
	// _, err := metav1.LabelSelectorAsSelector(&d.Spec.Selector)

	// Get all Machines that potentially belong to this Deployment.
	selector, err := metav1.LabelSelectorAsMap(&d.Spec.Selector)
	if err != nil {
		return nil, err
	}
	machines := &v1alpha1.MachineList{}
	err = r.List(context.Background(), client.InNamespace(d.Namespace).MatchingLabels(selector), machines)
	if err != nil {
		return nil, err
	}
	// Group Machines by their controller (if it's in msList).
	machineMap := make(map[types.UID]*v1alpha1.MachineList, len(msList))
	for _, ms := range msList {
		machineMap[ms.UID] = &v1alpha1.MachineList{}
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
	ms := &v1alpha1.MachineSet{}
	key := client.ObjectKey{Namespace: o.Meta.GetNamespace(), Name: o.Meta.GetName()}
	err := r.Client.Get(context.Background(), key, ms)
	if err != nil {
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
		result = append(result, reconcile.Request{
			NamespacedName: client.ObjectKey{Namespace: md.Namespace, Name: md.Name}})
	}

	return result
}
