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
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha2"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/util"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var (
	// machineSetKind contains the schema.GroupVersionKind for the MachineSet type.
	machineSetKind = clusterv1.GroupVersion.WithKind("MachineSet")

	// stateConfirmationTimeout is the amount of time allowed to wait for desired state.
	stateConfirmationTimeout = 10 * time.Second

	// stateConfirmationInterval is the amount of time between polling for the desired state.
	// The polling is against a local memory cache.
	stateConfirmationInterval = 100 * time.Millisecond
)

// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io;bootstrap.cluster.x-k8s.io,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinesets;machinesets/status,verbs=get;list;watch;create;update;patch;delete

// MachineSetReconciler reconciles a MachineSet object
type MachineSetReconciler struct {
	Client client.Client
	Log    logr.Logger

	recorder record.EventRecorder
}

func (r *MachineSetReconciler) SetupWithManager(mgr ctrl.Manager, options controller.Options) error {
	err := ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.MachineSet{}).
		Owns(&clusterv1.Machine{}).
		Watches(
			&source.Kind{Type: &clusterv1.Machine{}},
			&handler.EnqueueRequestsFromMapFunc{ToRequests: handler.ToRequestsFunc(r.MachineToMachineSets)},
		).
		WithOptions(options).
		Complete(r)

	r.recorder = mgr.GetEventRecorderFor("machineset-controller")
	return err
}

func (r *MachineSetReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	_ = r.Log.WithValues("machineset", req.NamespacedName)

	machineSet := &clusterv1.MachineSet{}
	if err := r.Client.Get(ctx, req.NamespacedName, machineSet); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return. Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	// Ignore deleted MachineSets, this can happen when foregroundDeletion
	// is enabled
	if machineSet.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	result, err := r.reconcile(ctx, machineSet)
	if err != nil {
		klog.Errorf("Failed to reconcile MachineSet %q: %v", req.NamespacedName, err)
		r.recorder.Eventf(machineSet, corev1.EventTypeWarning, "ReconcileError", "%v", err)
	}
	return result, err
}

func (r *MachineSetReconciler) reconcile(ctx context.Context, machineSet *clusterv1.MachineSet) (ctrl.Result, error) {
	klog.V(4).Infof("Reconcile MachineSet %q in namespace %q", machineSet.Name, machineSet.Namespace)

	// Make sure that label selector can match template's labels.
	// TODO(vincepri): Move to a validation (admission) webhook when supported.
	selector, err := metav1.LabelSelectorAsSelector(&machineSet.Spec.Selector)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to parse MachineSet %q label selector", machineSet.Name)
	}

	if !selector.Matches(labels.Set(machineSet.Spec.Template.Labels)) {
		return ctrl.Result{}, errors.Errorf("failed validation on MachineSet %q label selector, cannot match any machines ", machineSet.Name)
	}

	selectorMap, err := metav1.LabelSelectorAsMap(&machineSet.Spec.Selector)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to convert MachineSet %q label selector to a map", machineSet.Name)
	}

	// Copy label selector to its status counterpart in string format.
	// This is necessary for CRDs including scale subresources.
	machineSet.Status.Selector = selector.String()

	// Get all Machines linked to this MachineSet.
	allMachines := &clusterv1.MachineList{}
	err = r.Client.List(
		context.Background(), allMachines,
		client.InNamespace(machineSet.Namespace),
		client.MatchingLabels(selectorMap),
	)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to list machines")
	}

	// Cluster might be nil as some providers might not require a cluster object
	// for machine management.
	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, machineSet.ObjectMeta)
	if errors.Cause(err) == util.ErrNoCluster {
		klog.V(2).Infof("MachineSet %q in namespace %q doesn't specify %q label, assuming nil cluster", machineSet.Name, machineSet.Namespace, clusterv1.MachineClusterLabelName)
	} else if err != nil {
		return ctrl.Result{}, err
	}

	if cluster != nil && r.shouldAdopt(machineSet) {
		machineSet.OwnerReferences = util.EnsureOwnerRef(machineSet.OwnerReferences, metav1.OwnerReference{
			APIVersion: cluster.APIVersion,
			Kind:       cluster.Kind,
			Name:       cluster.Name,
			UID:        cluster.UID,
		})
	}

	// Filter out irrelevant machines (deleting/mismatch labels) and claim orphaned machines.
	filteredMachines := make([]*clusterv1.Machine, 0, len(allMachines.Items))
	for idx := range allMachines.Items {
		machine := &allMachines.Items[idx]
		if shouldExcludeMachine(machineSet, machine) {
			continue
		}

		// Attempt to adopt machine if it meets previous conditions and it has no controller references.
		if metav1.GetControllerOf(machine) == nil {
			if err := r.adoptOrphan(machineSet, machine); err != nil {
				klog.Warningf("Failed to adopt Machine %q into MachineSet %q: %v", machine.Name, machineSet.Name, err)
				r.recorder.Eventf(machineSet, corev1.EventTypeWarning, "FailedAdopt", "Failed to adopt Machine %q: %v", machine.Name, err)
				continue
			}
			klog.Infof("Adopted Machine %q into MachineSet %q", machine.Name, machineSet.Name)
			r.recorder.Eventf(machineSet, corev1.EventTypeNormal, "SuccessfulAdopt", "Adopted Machine %q", machine.Name)
		}

		filteredMachines = append(filteredMachines, machine)
	}

	syncErr := r.syncReplicas(machineSet, filteredMachines)

	ms := machineSet.DeepCopy()
	newStatus := r.calculateStatus(ms, filteredMachines)

	// Always updates status as machines come up or die.
	updatedMS, err := updateMachineSetStatus(r.Client, machineSet, newStatus)
	if err != nil {
		if syncErr != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to sync machines: %v. failed to update machine set status", syncErr)
		}
		return ctrl.Result{}, errors.Wrap(err, "failed to update machine set status")
	}

	if syncErr != nil {
		return ctrl.Result{}, errors.Wrapf(syncErr, "failed to sync Machineset replicas")
	}

	var replicas int32
	if updatedMS.Spec.Replicas != nil {
		replicas = *updatedMS.Spec.Replicas
	}

	// Resync the MachineSet after MinReadySeconds as a last line of defense to guard against clock-skew.
	// Clock-skew is an issue as it may impact whether an available replica is counted as a ready replica.
	// A replica is available if the amount of time since last transition exceeds MinReadySeconds.
	// If there was a clock skew, checking whether the amount of time since last transition to ready state
	// exceeds MinReadySeconds could be incorrect.
	// To avoid an available replica stuck in the ready state, we force a reconcile after MinReadySeconds,
	// at which point it should confirm any available replica to be available.
	if updatedMS.Spec.MinReadySeconds > 0 &&
		updatedMS.Status.ReadyReplicas == replicas &&
		updatedMS.Status.AvailableReplicas != replicas {

		return ctrl.Result{RequeueAfter: time.Duration(updatedMS.Spec.MinReadySeconds) * time.Second}, nil
	}

	// Quickly rereconcile until the nodes become Ready.
	if updatedMS.Status.ReadyReplicas != replicas {
		klog.V(4).Info("Some nodes are not ready yet, requeuing until they are ready")
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

// syncReplicas scales Machine resources up or down.
func (r *MachineSetReconciler) syncReplicas(ms *clusterv1.MachineSet, machines []*clusterv1.Machine) error {
	if ms.Spec.Replicas == nil {
		return errors.Errorf("the Replicas field in Spec for machineset %v is nil, this should not be allowed", ms.Name)
	}

	diff := len(machines) - int(*(ms.Spec.Replicas))

	if diff < 0 {
		diff *= -1
		klog.Infof("Too few replicas for %v %s/%s, need %d, creating %d",
			machineSetKind, ms.Namespace, ms.Name, *(ms.Spec.Replicas), diff)

		var machineList []*clusterv1.Machine
		var errstrings []string
		for i := 0; i < diff; i++ {
			klog.Infof("Creating machine %d of %d, ( spec.replicas(%d) > currentMachineCount(%d) )",
				i+1, diff, *(ms.Spec.Replicas), len(machines))

			machine := r.getNewMachine(ms)

			// Clone and set the infrastructure and bootstrap references.
			var (
				infraConfig, bootstrapConfig *unstructured.Unstructured
				err                          error
			)

			infraConfig, err = external.CloneTemplate(r.Client, &machine.Spec.InfrastructureRef, machine.Namespace)
			if err != nil {
				return errors.Wrapf(err, "failed to clone infrastructure configuration for MachineSet %q in namespace %q", ms.Name, ms.Namespace)
			}
			machine.Spec.InfrastructureRef = corev1.ObjectReference{
				APIVersion: infraConfig.GetAPIVersion(),
				Kind:       infraConfig.GetKind(),
				Namespace:  infraConfig.GetNamespace(),
				Name:       infraConfig.GetName(),
			}

			if machine.Spec.Bootstrap.ConfigRef != nil {
				bootstrapConfig, err = external.CloneTemplate(r.Client, machine.Spec.Bootstrap.ConfigRef, machine.Namespace)
				if err != nil {
					return errors.Wrapf(err, "failed to clone bootstrap configuration for MachineSet %q in namespace %q", ms.Name, ms.Namespace)
				}
				machine.Spec.Bootstrap.ConfigRef = &corev1.ObjectReference{
					APIVersion: bootstrapConfig.GetAPIVersion(),
					Kind:       bootstrapConfig.GetKind(),
					Namespace:  bootstrapConfig.GetNamespace(),
					Name:       bootstrapConfig.GetName(),
				}
			}

			if err := r.Client.Create(context.TODO(), machine); err != nil {
				klog.Errorf("Unable to create Machine %q: %v", machine.Name, err)
				r.recorder.Eventf(ms, corev1.EventTypeWarning, "FailedCreate", "Failed to create machine %q: %v", machine.Name, err)
				errstrings = append(errstrings, err.Error())
				if err := r.Client.Delete(context.TODO(), infraConfig); !apierrors.IsNotFound(err) {
					klog.Errorf("Failed to cleanup infrastructure configuration object after Machine creation error: %v", err)
				}
				if bootstrapConfig != nil {
					if err := r.Client.Delete(context.TODO(), bootstrapConfig); !apierrors.IsNotFound(err) {
						klog.Errorf("Failed to cleanup bootstrap configuration object after Machine creation error: %v", err)
					}
				}
				continue
			}
			klog.Infof("Created machine %d of %d with name %q", i+1, diff, machine.Name)
			r.recorder.Eventf(ms, corev1.EventTypeNormal, "SuccessfulCreate", "Created machine %q", machine.Name)

			machineList = append(machineList, machine)
		}

		if len(errstrings) > 0 {
			return errors.New(strings.Join(errstrings, "; "))
		}

		return r.waitForMachineCreation(machineList)
	} else if diff > 0 {
		klog.Infof("Too many replicas for %v %s/%s, need %d, deleting %d",
			machineSetKind, ms.Namespace, ms.Name, *(ms.Spec.Replicas), diff)

		deletePriorityFunc, err := getDeletePriorityFunc(ms)
		if err != nil {
			return err
		}
		klog.Infof("Found %q delete policy", ms.Spec.DeletePolicy)
		// Choose which Machines to delete.
		machinesToDelete := getMachinesToDeletePrioritized(machines, diff, deletePriorityFunc)

		errCh := make(chan error, diff)
		var wg sync.WaitGroup
		wg.Add(diff)
		for _, machine := range machinesToDelete {
			go func(targetMachine *clusterv1.Machine) {
				defer wg.Done()
				err := r.Client.Delete(context.Background(), targetMachine)
				if err != nil {
					klog.Errorf("Unable to delete Machine %s: %v", targetMachine.Name, err)
					r.recorder.Eventf(ms, corev1.EventTypeWarning, "FailedDelete", "Failed to delete machine %q: %v", targetMachine.Name, err)
					errCh <- err
				}
				klog.Infof("Deleted machine %q", targetMachine.Name)
				r.recorder.Eventf(ms, corev1.EventTypeNormal, "SuccessfulDelete", "Deleted machine %q", targetMachine.Name)
			}(machine)
		}
		wg.Wait()

		select {
		case err := <-errCh:
			// all errors have been reported before and they're likely to be the same, so we'll only return the first one we hit.
			if err != nil {
				return err
			}
		default:
		}

		return r.waitForMachineDeletion(machinesToDelete)
	}

	return nil
}

// getNewMachine creates a new Machine object. The name of the newly created resource is going
// to be created by the API server, we set the generateName field.
func (r *MachineSetReconciler) getNewMachine(machineSet *clusterv1.MachineSet) *clusterv1.Machine {
	gv := clusterv1.GroupVersion
	machine := &clusterv1.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind:       gv.WithKind("Machine").Kind,
			APIVersion: gv.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Labels:      machineSet.Spec.Template.Labels,
			Annotations: machineSet.Spec.Template.Annotations,
		},
		Spec: machineSet.Spec.Template.Spec,
	}
	machine.ObjectMeta.GenerateName = fmt.Sprintf("%s-", machineSet.Name)
	machine.ObjectMeta.OwnerReferences = []metav1.OwnerReference{*metav1.NewControllerRef(machineSet, machineSetKind)}
	machine.Namespace = machineSet.Namespace
	return machine
}

// shouldExcludeMachine returns true if the machine should be filtered out, false otherwise.
func shouldExcludeMachine(machineSet *clusterv1.MachineSet, machine *clusterv1.Machine) bool {
	if metav1.GetControllerOf(machine) != nil && !metav1.IsControlledBy(machine, machineSet) {
		klog.V(4).Infof("%s not controlled by %v", machine.Name, machineSet.Name)
		return true
	}
	return machine.ObjectMeta.DeletionTimestamp != nil
}

// adoptOrphan sets the MachineSet as a controller OwnerReference to the Machine.
func (r *MachineSetReconciler) adoptOrphan(machineSet *clusterv1.MachineSet, machine *clusterv1.Machine) error {
	patch := client.MergeFrom(machine.DeepCopy())
	newRef := *metav1.NewControllerRef(machineSet, machineSetKind)
	machine.OwnerReferences = append(machine.OwnerReferences, newRef)
	return r.Client.Patch(context.Background(), machine, patch)
}

func (r *MachineSetReconciler) waitForMachineCreation(machineList []*clusterv1.Machine) error {
	for _, machine := range machineList {
		pollErr := util.PollImmediate(stateConfirmationInterval, stateConfirmationTimeout, func() (bool, error) {
			key := client.ObjectKey{Namespace: machine.Namespace, Name: machine.Name}

			if err := r.Client.Get(context.Background(), key, &clusterv1.Machine{}); err != nil {
				if apierrors.IsNotFound(err) {
					return false, nil
				}
				klog.Error(err)
				return false, err
			}

			return true, nil
		})

		if pollErr != nil {
			klog.Error(pollErr)
			return errors.Wrap(pollErr, "failed waiting for machine object to be created")
		}
	}

	return nil
}

func (r *MachineSetReconciler) waitForMachineDeletion(machineList []*clusterv1.Machine) error {
	for _, machine := range machineList {
		pollErr := util.PollImmediate(stateConfirmationInterval, stateConfirmationTimeout, func() (bool, error) {
			m := &clusterv1.Machine{}
			key := client.ObjectKey{Namespace: machine.Namespace, Name: machine.Name}

			err := r.Client.Get(context.Background(), key, m)
			if apierrors.IsNotFound(err) || !m.DeletionTimestamp.IsZero() {
				return true, nil
			}

			return false, err
		})

		if pollErr != nil {
			klog.Error(pollErr)
			return errors.Wrap(pollErr, "failed waiting for machine object to be deleted")
		}
	}
	return nil
}

// MachineToMachineSets is a handler.ToRequestsFunc to be used to enqeue requests for reconciliation
// for MachineSets that might adopt an orphaned Machine.
func (r *MachineSetReconciler) MachineToMachineSets(o handler.MapObject) []ctrl.Request {
	result := []ctrl.Request{}

	m, ok := o.Object.(*clusterv1.Machine)
	if !ok {
		klog.Errorf("expected a Machine but got a %T", o.Object)
		return nil
	}

	// Check if the controller reference is already set and
	// return an empty result when one is found.
	for _, ref := range m.ObjectMeta.OwnerReferences {
		if ref.Controller != nil && *ref.Controller {
			return result
		}
	}

	mss := r.getMachineSetsForMachine(m)
	if len(mss) == 0 {
		klog.V(4).Infof("Found no MachineSet for Machine %q", m.Name)
		return nil
	}

	for _, ms := range mss {
		name := client.ObjectKey{Namespace: ms.Namespace, Name: ms.Name}
		result = append(result, ctrl.Request{NamespacedName: name})
	}

	return result
}

func (r *MachineSetReconciler) getMachineSetsForMachine(m *clusterv1.Machine) []*clusterv1.MachineSet {
	if len(m.Labels) == 0 {
		klog.Warningf("No machine sets found for Machine %v because it has no labels", m.Name)
		return nil
	}

	msList := &clusterv1.MachineSetList{}
	err := r.Client.List(context.Background(), msList, client.InNamespace(m.Namespace))
	if err != nil {
		klog.Errorf("Failed to list machine sets, %v", err)
		return nil
	}

	var mss []*clusterv1.MachineSet
	for idx := range msList.Items {
		ms := &msList.Items[idx]
		if r.hasMatchingLabels(ms, m) {
			mss = append(mss, ms)
		}
	}

	return mss
}

func (r *MachineSetReconciler) hasMatchingLabels(machineSet *clusterv1.MachineSet, machine *clusterv1.Machine) bool {
	selector, err := metav1.LabelSelectorAsSelector(&machineSet.Spec.Selector)
	if err != nil {
		klog.Warningf("unable to convert selector: %v", err)
		return false
	}

	// If a deployment with a nil or empty selector creeps in, it should match nothing, not everything.
	if selector.Empty() {
		klog.V(2).Infof("%v machineset has empty selector", machineSet.Name)
		return false
	}

	if !selector.Matches(labels.Set(machine.Labels)) {
		klog.V(4).Infof("%v machine has mismatch labels", machine.Name)
		return false
	}

	return true
}

func (r *MachineSetReconciler) shouldAdopt(ms *clusterv1.MachineSet) bool {
	return !util.HasOwner(ms.OwnerReferences, clusterv1.GroupVersion.String(), []string{"MachineDeployment", "Cluster"})
}
