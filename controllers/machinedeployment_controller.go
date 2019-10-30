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

package controllers

import (
	"context"
	"reflect"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha2"
	"sigs.k8s.io/cluster-api/util"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var (
	// machineDeploymentKind contains the schema.GroupVersionKind for the MachineDeployment type.
	machineDeploymentKind = clusterv1.GroupVersion.WithKind("MachineDeployment")
)

// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io;bootstrap.cluster.x-k8s.io,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinedeployments;machinedeployments/status,verbs=get;list;watch;create;update;patch;delete

// MachineDeploymentReconciler reconciles a MachineDeployment object
type MachineDeploymentReconciler struct {
	Client client.Client
	Log    logr.Logger

	recorder record.EventRecorder
}

func (r *MachineDeploymentReconciler) SetupWithManager(mgr ctrl.Manager, options controller.Options) error {
	err := ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.MachineDeployment{}).
		Owns(&clusterv1.MachineSet{}).
		Watches(
			&source.Kind{Type: &clusterv1.MachineSet{}},
			&handler.EnqueueRequestsFromMapFunc{ToRequests: handler.ToRequestsFunc(r.MachineSetToDeployments)},
		).
		WithOptions(options).
		Complete(r)

	r.recorder = mgr.GetEventRecorderFor("machinedeployment-controller")
	return err
}

func (r *MachineDeploymentReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	_ = r.Log.WithValues("machinedeployment", req.NamespacedName)

	// Fetch the MachineDeployment instance
	d := &clusterv1.MachineDeployment{}
	if err := r.Client.Get(ctx, req.NamespacedName, d); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	// Ignore deleted MachineDeployments, this can happen when foregroundDeletion
	// is enabled
	if d.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	result, err := r.reconcile(ctx, d)
	if err != nil {
		klog.Errorf("Failed to reconcile MachineDeployment %q: %v", req.NamespacedName, err)
		r.recorder.Eventf(d, corev1.EventTypeWarning, "ReconcileError", "%v", err)
	}

	return result, nil
}

func (r *MachineDeploymentReconciler) reconcile(ctx context.Context, d *clusterv1.MachineDeployment) (ctrl.Result, error) {
	clusterv1.PopulateDefaultsMachineDeployment(d)

	// Test for an empty LabelSelector and short circuit if that is the case
	// TODO: When we have validation webhooks, we should likely reject on an empty LabelSelector
	everything := metav1.LabelSelector{}
	if reflect.DeepEqual(d.Spec.Selector, &everything) {
		if d.Status.ObservedGeneration < d.Generation {
			patch := client.MergeFrom(d.DeepCopy())
			d.Status.ObservedGeneration = d.Generation
			if err := r.Client.Status().Patch(ctx, d, patch); err != nil {
				klog.Warningf("Failed to patch status for MachineDeployment %q: %v", d.Name, err)
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Make sure that label selector can match the template's labels.
	// TODO(vincepri): Move to a validation (admission) webhook when supported.
	selector, err := metav1.LabelSelectorAsSelector(&d.Spec.Selector)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to parse MachineDeployment %q label selector", d.Name)
	}

	if !selector.Matches(labels.Set(d.Spec.Template.Labels)) {
		return ctrl.Result{}, errors.Errorf("failed validation on MachineDeployment %q label selector, cannot match Machine template labels", d.Name)
	}

	// Cluster might be nil as some providers might not require a cluster object
	// for machine management.
	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, d.ObjectMeta)
	if errors.Cause(err) == util.ErrNoCluster {
		klog.V(2).Infof("MachineDeployment %q in namespace %q doesn't specify %q label, assuming nil Cluster", d.Name, d.Namespace, clusterv1.MachineClusterLabelName)
	} else if err != nil {
		return ctrl.Result{}, err
	}

	if cluster != nil && r.shouldAdopt(d) {
		patch := client.MergeFrom(d.DeepCopy())
		d.OwnerReferences = util.EnsureOwnerRef(d.OwnerReferences, metav1.OwnerReference{
			APIVersion: cluster.APIVersion,
			Kind:       cluster.Kind,
			Name:       cluster.Name,
			UID:        cluster.UID,
		})
		// Patch using a deep copy to avoid overwriting any unexpected Status changes from the returned result
		if err := r.Client.Patch(ctx, d.DeepCopy(), patch); err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "Failed to add OwnerReference to MachineDeployment %s/%s", d.Namespace, d.Name)
		}
	}

	msList, err := r.getMachineSetsForDeployment(d)
	if err != nil {
		return ctrl.Result{}, err
	}

	machineMap, err := r.getMachineMapForDeployment(d, msList)
	if err != nil {
		return ctrl.Result{}, err
	}

	if d.Spec.Paused {
		return ctrl.Result{}, r.sync(d, msList, machineMap)
	}

	switch d.Spec.Strategy.Type {
	case clusterv1.RollingUpdateMachineDeploymentStrategyType:
		return ctrl.Result{}, r.rolloutRolling(d, msList, machineMap)
	}

	return ctrl.Result{}, errors.Errorf("unexpected deployment strategy type: %s", d.Spec.Strategy.Type)
}

// getMachineSetsForDeployment returns a list of MachineSets associated with a MachineDeployment.
func (r *MachineDeploymentReconciler) getMachineSetsForDeployment(d *clusterv1.MachineDeployment) ([]*clusterv1.MachineSet, error) {
	// List all MachineSets to find those we own but that no longer match our selector.
	machineSets := &clusterv1.MachineSetList{}
	if err := r.Client.List(context.Background(), machineSets, client.InNamespace(d.Namespace)); err != nil {
		return nil, err
	}

	filtered := make([]*clusterv1.MachineSet, 0, len(machineSets.Items))
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

		// Skip this MachineSet unless either selector matches or it has a controller ref pointing to this MachineDeployment
		if !selector.Matches(labels.Set(ms.Labels)) && !metav1.IsControlledBy(ms, d) {
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
func (r *MachineDeploymentReconciler) adoptOrphan(deployment *clusterv1.MachineDeployment, machineSet *clusterv1.MachineSet) error {
	patch := client.MergeFrom(machineSet.DeepCopy())
	newRef := *metav1.NewControllerRef(deployment, machineDeploymentKind)
	machineSet.OwnerReferences = append(machineSet.OwnerReferences, newRef)
	return r.Client.Patch(context.Background(), machineSet, patch)
}

// getMachineMapForDeployment returns the Machines managed by a Deployment.
//
// It returns a map from MachineSet UID to a list of Machines controlled by that MachineSet,
// according to the Machine's ControllerRef.
func (r *MachineDeploymentReconciler) getMachineMapForDeployment(d *clusterv1.MachineDeployment, msList []*clusterv1.MachineSet) (map[types.UID]*clusterv1.MachineList, error) {
	// TODO(droot): double check if previous selector maps correctly to new one.
	// _, err := metav1.LabelSelectorAsSelector(&d.Spec.Selector)

	// Get all Machines that potentially belong to this Deployment.
	selector, err := metav1.LabelSelectorAsMap(&d.Spec.Selector)
	if err != nil {
		return nil, err
	}

	machines := &clusterv1.MachineList{}
	if err = r.Client.List(context.Background(), machines, client.InNamespace(d.Namespace), client.MatchingLabels(selector)); err != nil {
		return nil, err
	}

	// Group Machines by their controller (if it's in msList).
	machineMap := make(map[types.UID]*clusterv1.MachineList, len(msList))
	for _, ms := range msList {
		machineMap[ms.UID] = &clusterv1.MachineList{}
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
func (r *MachineDeploymentReconciler) getMachineDeploymentsForMachineSet(ms *clusterv1.MachineSet) []*clusterv1.MachineDeployment {
	if len(ms.Labels) == 0 {
		klog.Warningf("No MachineDeployments found for MachineSet %q because it has no labels", ms.Name)
		return nil
	}

	dList := &clusterv1.MachineDeploymentList{}
	if err := r.Client.List(context.Background(), dList, client.InNamespace(ms.Namespace)); err != nil {
		klog.Warningf("Failed to list MachineDeployments: %v", err)
		return nil
	}

	deployments := make([]*clusterv1.MachineDeployment, 0, len(dList.Items))
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
func (r *MachineDeploymentReconciler) MachineSetToDeployments(o handler.MapObject) []ctrl.Request {
	result := []ctrl.Request{}

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
		result = append(result, ctrl.Request{NamespacedName: name})
	}

	return result
}

func (r *MachineDeploymentReconciler) shouldAdopt(md *clusterv1.MachineDeployment) bool {
	return !util.HasOwner(md.OwnerReferences, clusterv1.GroupVersion.String(), []string{"Cluster"})
}
