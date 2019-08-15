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

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"
	"sigs.k8s.io/cluster-api/api/v1alpha2"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha2"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// controllerName is the name of this controller
const controllerName = "machinedeployment-controller"

var (
	// controllerKind contains the schema.GroupVersionKind for this controller type.
	controllerKind = v1alpha2.GroupVersion.WithKind("MachineDeployment")
)

// ReconcileMachineDeployment reconciles a MachineDeployment object.
type ReconcileMachineDeployment struct {
	client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
}

// newReconciler returns a new reconcile.Reconciler.
func newReconciler(mgr manager.Manager) *ReconcileMachineDeployment {
	return &ReconcileMachineDeployment{
		Client:   mgr.GetClient(),
		scheme:   mgr.GetScheme(),
		recorder: mgr.GetEventRecorderFor(controllerName),
	}
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
		Type: &v1alpha2.MachineDeployment{}},
		&handler.EnqueueRequestForObject{},
	)
	if err != nil {
		return err
	}

	// Watch for changes to MachineSet and reconcile the owner MachineDeployment.
	err = c.Watch(
		&source.Kind{Type: &v1alpha2.MachineSet{}},
		&handler.EnqueueRequestForOwner{OwnerType: &v1alpha2.MachineDeployment{}, IsController: true},
	)
	if err != nil {
		return err
	}

	// Watch for changes to MachineSets using a mapping function to MachineDeployment.
	// This watcher is required for use cases like adoption. In case a MachineSet doesn't have
	// a controller reference, it'll look for potential matching MachineDeployments to reconcile.
	err = c.Watch(
		&source.Kind{Type: &v1alpha2.MachineSet{}},
		&handler.EnqueueRequestsFromMapFunc{ToRequests: mapFn},
	)
	if err != nil {
		return err
	}

	return nil
}

// Reconcile reads that state of the cluster for a MachineDeployment object and makes changes based on the state read
// and what is in the MachineDeployment.Spec.
func (r *ReconcileMachineDeployment) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the MachineDeployment instance
	d := &v1alpha2.MachineDeployment{}
	ctx := context.TODO()
	if err := r.Get(ctx, request.NamespacedName, d); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// Ignore deleted MachineDeployments, this can happen when foregroundDeletion
	// is enabled
	if d.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}

	result, err := r.reconcile(ctx, d)
	if err != nil {
		klog.Errorf("Failed to reconcile MachineDeployment %q: %v", request.NamespacedName, err)
		r.recorder.Eventf(d, corev1.EventTypeWarning, "ReconcileError", "%v", err)
	}

	return result, err
}

func (r *ReconcileMachineDeployment) reconcile(ctx context.Context, d *v1alpha2.MachineDeployment) (reconcile.Result, error) {
	v1alpha2.PopulateDefaultsMachineDeployment(d)

	everything := metav1.LabelSelector{}
	if reflect.DeepEqual(d.Spec.Selector, &everything) {
		if d.Status.ObservedGeneration < d.Generation {
			patch := client.MergeFrom(d.DeepCopy())
			d.Status.ObservedGeneration = d.Generation
			if err := r.Status().Patch(ctx, d, patch); err != nil {
				klog.Warningf("Failed to patch status for MachineDeployment %q: %v", d.Name, err)
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{Requeue: true}, nil
	}

	// Make sure that label selector can match the template's labels.
	// TODO(vincepri): Move to a validation (admission) webhook when supported.
	selector, err := metav1.LabelSelectorAsSelector(&d.Spec.Selector)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to parse MachineDeployment %q label selector", d.Name)
	}

	if !selector.Matches(labels.Set(d.Spec.Template.Labels)) {
		return reconcile.Result{}, errors.Errorf("failed validation on MachineDeployment %q label selector, cannot match Machine template labels", d.Name)
	}

	// Cluster might be nil as some providers might not require a cluster object
	// for machine management.
	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, d.ObjectMeta)
	if errors.Cause(err) == util.ErrNoCluster {
		klog.Infof("MachineDeployment %q in namespace %q doesn't specify %q label, assuming nil Cluster", d.Name, d.Namespace, v1alpha2.MachineClusterLabelName)
	} else if err != nil {
		return reconcile.Result{}, err
	}

	if cluster != nil && shouldAdopt(d) {
		d.OwnerReferences = util.EnsureOwnerRef(d.OwnerReferences, metav1.OwnerReference{
			APIVersion: cluster.APIVersion,
			Kind:       cluster.Kind,
			Name:       cluster.Name,
			UID:        cluster.UID,
		})
	}

	msList, err := r.getMachineSetsForDeployment(d)
	if err != nil {
		return reconcile.Result{}, err
	}

	machineMap, err := r.getMachineMapForDeployment(d, msList)
	if err != nil {
		return reconcile.Result{}, err
	}

	if d.Spec.Paused {
		return reconcile.Result{}, r.sync(d, msList, machineMap)
	}

	switch d.Spec.Strategy.Type {
	case v1alpha2.RollingUpdateMachineDeploymentStrategyType:
		return reconcile.Result{}, r.rolloutRolling(d, msList, machineMap)
	}

	return reconcile.Result{}, errors.Errorf("unexpected deployment strategy type: %s", d.Spec.Strategy.Type)
}

// getCluster reuturns the Cluster associated with the MachineDeployment, if any.
func (r *ReconcileMachineDeployment) getCluster(d *v1alpha2.MachineDeployment) (*v1alpha2.Cluster, error) {
	if d.Spec.Template.Labels[v1alpha2.MachineClusterLabelName] == "" {
		klog.Infof("Deployment %q in namespace %q doesn't specify %q label, assuming nil cluster", d.Name, d.Namespace, v1alpha2.MachineClusterLabelName)
		return nil, nil
	}

	cluster := &v1alpha2.Cluster{}
	key := client.ObjectKey{
		Namespace: d.Namespace,
		Name:      d.Spec.Template.Labels[v1alpha2.MachineClusterLabelName],
	}

	if err := r.Client.Get(context.Background(), key, cluster); err != nil {
		return nil, err
	}

	return cluster, nil
}

// getMachineSetsForDeployment returns a list of MachineSets associated with a MachineDeployment.
func (r *ReconcileMachineDeployment) getMachineSetsForDeployment(d *v1alpha2.MachineDeployment) ([]*v1alpha2.MachineSet, error) {
	// List all MachineSets to find those we own but that no longer match our selector.
	machineSets := &v1alpha2.MachineSetList{}
	if err := r.Client.List(context.Background(), machineSets, client.InNamespace(d.Namespace)); err != nil {
		return nil, err
	}

	filtered := make([]*v1alpha2.MachineSet, 0, len(machineSets.Items))
	for idx := range machineSets.Items {
		ms := &machineSets.Items[idx]

		selector, err := metav1.LabelSelectorAsSelector(&d.Spec.Selector)
		if err != nil {
			klog.Errorf("Skipping MachineSet %q, failed to get label selector from spec selector: %v", ms.Name, err)
			continue
		}

		// If a MachineDeployment with a nil or empty selector creeps in, it should match nothing, not everything.
		if selector.Empty() {
			klog.Warningf("Skipping MachineSet %q as the selector is empty", ms.Name)
			continue
		}

		if !selector.Matches(labels.Set(ms.Labels)) {
			klog.V(4).Infof("Skipping MachineSet %v, label mismatch", ms.Name)
			continue
		}

		// Attempt to adopt machine if it meets previous conditions and it has no controller references.
		if metav1.GetControllerOf(ms) == nil {
			if err := r.adoptOrphan(d, ms); err != nil {
				r.recorder.Eventf(d, corev1.EventTypeWarning, "FailedAdopt", "Failed to adopt MachineSet %q: %v", ms.Name, err)
				klog.Warningf("Failed to adopt MachineSet %q into MachineDeployment %q: %v", ms.Name, d.Name, err)
				continue
			}
			r.recorder.Eventf(d, corev1.EventTypeNormal, "SuccessfulAdopt", "Adopted MachineSet %q", ms.Name)
		}

		if !metav1.IsControlledBy(ms, d) {
			continue
		}

		filtered = append(filtered, ms)
	}

	return filtered, nil
}

// adoptOrphan sets the MachineDeployment as a controller OwnerReference to the MachineSet.
func (r *ReconcileMachineDeployment) adoptOrphan(deployment *v1alpha2.MachineDeployment, machineSet *v1alpha2.MachineSet) error {
	patch := client.MergeFrom(machineSet.DeepCopy())
	newRef := *metav1.NewControllerRef(deployment, controllerKind)
	machineSet.OwnerReferences = append(machineSet.OwnerReferences, newRef)
	return r.Client.Patch(context.Background(), machineSet, patch)
}

// getMachineMapForDeployment returns the Machines managed by a Deployment.
//
// It returns a map from MachineSet UID to a list of Machines controlled by that MachineSet,
// according to the Machine's ControllerRef.
func (r *ReconcileMachineDeployment) getMachineMapForDeployment(d *v1alpha2.MachineDeployment, msList []*v1alpha2.MachineSet) (map[types.UID]*v1alpha2.MachineList, error) {
	// TODO(droot): double check if previous selector maps correctly to new one.
	// _, err := metav1.LabelSelectorAsSelector(&d.Spec.Selector)

	// Get all Machines that potentially belong to this Deployment.
	selector, err := metav1.LabelSelectorAsMap(&d.Spec.Selector)
	if err != nil {
		return nil, err
	}

	machines := &v1alpha2.MachineList{}
	if err = r.Client.List(context.Background(), machines, client.InNamespace(d.Namespace), client.MatchingLabels(selector)); err != nil {
		return nil, err
	}

	// Group Machines by their controller (if it's in msList).
	machineMap := make(map[types.UID]*v1alpha2.MachineList, len(msList))
	for _, ms := range msList {
		machineMap[ms.UID] = &v1alpha2.MachineList{}
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

// getMachineDeploymentsForMachineSet returns a list of MachineDeployments that could potentially match a MachineSet.
func (r *ReconcileMachineDeployment) getMachineDeploymentsForMachineSet(ms *v1alpha2.MachineSet) []*v1alpha2.MachineDeployment {
	if len(ms.Labels) == 0 {
		klog.Warningf("No MachineDeployments found for MachineSet %q because it has no labels", ms.Name)
		return nil
	}

	dList := &v1alpha2.MachineDeploymentList{}
	if err := r.Client.List(context.Background(), dList, client.InNamespace(ms.Namespace)); err != nil {
		klog.Warningf("Failed to list MachineDeployments: %v", err)
		return nil
	}

	deployments := make([]*v1alpha2.MachineDeployment, 0, len(dList.Items))
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

// MachineSetTodeployments is a handler.ToRequestsFunc to be used to enqeue requests for reconciliation
// for MachineDeployments that might adopt an orphaned MachineSet.
func (r *ReconcileMachineDeployment) MachineSetToDeployments(o handler.MapObject) []reconcile.Request {
	result := []reconcile.Request{}

	ms, ok := o.Object.(*clusterv1.MachineSet)
	if !ok {
		klog.Errorf("Expected a MachineSet but got %T instead: %v", o.Object, o.Object)
		return nil
	}

	// Check if the controller reference is already set and
	// return an empty result when one is found.
	for _, ref := range ms.ObjectMeta.OwnerReferences {
		if ref.Controller != nil && *ref.Controller {
			return result
		}
	}

	mds := r.getMachineDeploymentsForMachineSet(ms)
	if len(mds) == 0 {
		klog.V(4).Infof("Found no MachineDeployment for MachineSet: %v", ms.Name)
		return nil
	}

	for _, md := range mds {
		name := client.ObjectKey{Namespace: md.Namespace, Name: md.Name}
		result = append(result, reconcile.Request{NamespacedName: name})
	}

	return result
}

func shouldAdopt(md *v1alpha2.MachineDeployment) bool {
	return !util.HasOwner(md.OwnerReferences, v1alpha2.GroupVersion.String(), []string{"Cluster"})
}
