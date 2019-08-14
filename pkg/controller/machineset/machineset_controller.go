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

package machineset

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha2"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// controllerName is the name of this controller
const controllerName = "machineset-controller"

var (
	// controllerKind contains the schema.GroupVersionKind for this controller type.
	controllerKind = clusterv1.GroupVersion.WithKind("MachineSet")

	// stateConfirmationTimeout is the amount of time allowed to wait for desired state.
	stateConfirmationTimeout = 10 * time.Second

	// stateConfirmationInterval is the amount of time between polling for the desired state.
	// The polling is against a local memory cache.
	stateConfirmationInterval = 100 * time.Millisecond
)

// Add creates a new MachineSet Controller and adds it to the Manager with default RBAC.
// The Manager will set fields on the Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	r := newReconciler(mgr)
	return add(mgr, r, r.MachineToMachineSets)
}

// newReconciler returns a new reconcile.Reconciler.
func newReconciler(mgr manager.Manager) *ReconcileMachineSet {
	return &ReconcileMachineSet{
		Client:   mgr.GetClient(),
		scheme:   mgr.GetScheme(),
		recorder: mgr.GetEventRecorderFor(controllerName),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler.
func add(mgr manager.Manager, r reconcile.Reconciler, mapFn handler.ToRequestsFunc) error {
	// Create a new controller.
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to MachineSet.
	err = c.Watch(
		&source.Kind{Type: &clusterv1.MachineSet{}},
		&handler.EnqueueRequestForObject{},
	)
	if err != nil {
		return err
	}

	// Watch for changes to Machines and reconcile the owner MachineSet.
	err = c.Watch(
		&source.Kind{Type: &clusterv1.Machine{}},
		&handler.EnqueueRequestForOwner{IsController: true, OwnerType: &clusterv1.MachineSet{}},
	)
	if err != nil {
		return err
	}

	// Watch for changes to Machines using a mapping function to MachineSets.
	// This watcher is required for use cases like adoption. In case a Machine doesn't have
	// a controller reference, it'll look for potential matching MachineSet to reconcile.
	return c.Watch(
		&source.Kind{Type: &clusterv1.Machine{}},
		&handler.EnqueueRequestsFromMapFunc{ToRequests: mapFn},
	)
}

// ReconcileMachineSet reconciles a MachineSet object
type ReconcileMachineSet struct {
	client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
}

// Reconcile reads that state of the cluster for a MachineSet object and makes changes based on the state read
// and what is in the MachineSet.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
func (r *ReconcileMachineSet) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the MachineSet instance
	ctx := context.TODO()
	machineSet := &clusterv1.MachineSet{}
	if err := r.Get(ctx, request.NamespacedName, machineSet); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return. Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// Ignore deleted MachineSets, this can happen when foregroundDeletion
	// is enabled
	if machineSet.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}

	result, err := r.reconcile(ctx, machineSet)
	if err != nil {
		klog.Errorf("Failed to reconcile MachineSet %q: %v", request.NamespacedName, err)
		r.recorder.Eventf(machineSet, corev1.EventTypeWarning, "ReconcileError", "%v", err)
	}
	return result, err
}

func (r *ReconcileMachineSet) reconcile(ctx context.Context, machineSet *clusterv1.MachineSet) (reconcile.Result, error) {
	klog.V(4).Infof("Reconcile MachineSet %q in namespace %q", machineSet.Name, machineSet.Namespace)

	// Make sure that label selector can match template's labels.
	// TODO(vincepri): Move to a validation (admission) webhook when supported.
	selector, err := metav1.LabelSelectorAsSelector(&machineSet.Spec.Selector)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to parse MachineSet %q label selector", machineSet.Name)
	}

	if !selector.Matches(labels.Set(machineSet.Spec.Template.Labels)) {
		return reconcile.Result{}, errors.Errorf("failed validation on MachineSet %q label selector, cannot match any machines ", machineSet.Name)
	}

	selectorMap, err := metav1.LabelSelectorAsMap(&machineSet.Spec.Selector)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to convert MachineSet %q label selector to a map", machineSet.Name)
	}

	allMachines := &clusterv1.MachineList{}
	err = r.Client.List(
		context.Background(), allMachines,
		client.InNamespace(machineSet.Namespace),
		client.MatchingLabels(selectorMap),
	)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to list machines")
	}

	// Cluster might be nil as some providers might not require a cluster object
	// for machine management.
	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, machineSet.ObjectMeta)
	if errors.Cause(err) == util.ErrNoCluster {
		klog.Infof("MachineSet %q in namespace %q doesn't specify %q label, assuming nil cluster", machineSet.Name, machineSet.Namespace, clusterv1.MachineClusterLabelName)
	} else if err != nil {
		return reconcile.Result{}, err
	}

	if cluster != nil && shouldAdopt(machineSet) {
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
			return reconcile.Result{}, errors.Wrapf(err, "failed to sync machines: %v. failed to update machine set status", syncErr)
		}
		return reconcile.Result{}, errors.Wrap(err, "failed to update machine set status")
	}

	if syncErr != nil {
		return reconcile.Result{}, errors.Wrapf(syncErr, "failed to sync Machineset replicas")
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

		return reconcile.Result{RequeueAfter: time.Duration(updatedMS.Spec.MinReadySeconds) * time.Second}, nil
	}

	// Quickly rereconcile until the nodes become Ready.
	if updatedMS.Status.ReadyReplicas != replicas {
		klog.V(4).Info("Some nodes are not ready yet, requeuing until they are ready")
		return reconcile.Result{RequeueAfter: 15 * time.Second}, nil
	}

	return reconcile.Result{}, nil
}

// getCluster reuturns the Cluster associated with the MachineSet, if any.
func (r *ReconcileMachineSet) getCluster(ms *clusterv1.MachineSet) (*clusterv1.Cluster, error) {
	if ms.Spec.Template.Labels[clusterv1.MachineClusterLabelName] == "" {
		klog.Infof("MachineSet %q in namespace %q doesn't specify %q label, assuming nil cluster", ms.Name, ms.Namespace, clusterv1.MachineClusterLabelName)
		return nil, nil
	}

	cluster := &clusterv1.Cluster{}
	key := client.ObjectKey{
		Namespace: ms.Namespace,
		Name:      ms.Spec.Template.Labels[clusterv1.MachineClusterLabelName],
	}

	if err := r.Client.Get(context.Background(), key, cluster); err != nil {
		return nil, err
	}

	return cluster, nil
}

// syncReplicas scales Machine resources up or down.
func (r *ReconcileMachineSet) syncReplicas(ms *clusterv1.MachineSet, machines []*clusterv1.Machine) error {
	if ms.Spec.Replicas == nil {
		return errors.Errorf("the Replicas field in Spec for machineset %v is nil, this should not be allowed", ms.Name)
	}

	diff := len(machines) - int(*(ms.Spec.Replicas))

	if diff < 0 {
		diff *= -1
		klog.Infof("Too few replicas for %v %s/%s, need %d, creating %d",
			controllerKind, ms.Namespace, ms.Name, *(ms.Spec.Replicas), diff)

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
			controllerKind, ms.Namespace, ms.Name, *(ms.Spec.Replicas), diff)

		deletePriorityFunc, err := getDeletePriorityFunc(ms)
		if err != nil {
			return err
		}
		klog.Infof("Found %s delete policy", ms.Spec.DeletePolicy)
		// Choose which Machines to delete.
		machinesToDelete := getMachinesToDeletePrioritized(machines, diff, deletePriorityFunc)

		// TODO: Add cap to limit concurrent delete calls.
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
func (r *ReconcileMachineSet) getNewMachine(machineSet *clusterv1.MachineSet) *clusterv1.Machine {
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
	machine.ObjectMeta.OwnerReferences = []metav1.OwnerReference{*metav1.NewControllerRef(machineSet, controllerKind)}
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
func (r *ReconcileMachineSet) adoptOrphan(machineSet *clusterv1.MachineSet, machine *clusterv1.Machine) error {
	newRef := *metav1.NewControllerRef(machineSet, controllerKind)
	machine.OwnerReferences = append(machine.OwnerReferences, newRef)
	return r.Client.Update(context.Background(), machine)
}

func (r *ReconcileMachineSet) waitForMachineCreation(machineList []*clusterv1.Machine) error {
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

func (r *ReconcileMachineSet) waitForMachineDeletion(machineList []*clusterv1.Machine) error {
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
func (r *ReconcileMachineSet) MachineToMachineSets(o handler.MapObject) []reconcile.Request {
	result := []reconcile.Request{}

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
		result = append(result, reconcile.Request{NamespacedName: name})
	}

	return result
}

func shouldAdopt(ms *clusterv1.MachineSet) bool {
	return !util.HasOwner(ms.OwnerReferences, clusterv1.GroupVersion.String(), []string{"MachineDeployment", "Cluster"})
}
