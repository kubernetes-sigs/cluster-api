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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"
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

type TemplateCloner interface {
	CloneTemplate(ctx context.Context, c client.Client, ref *corev1.ObjectReference, namespace, clusterName string, owner *metav1.OwnerReference) (*corev1.ObjectReference, error)
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io;bootstrap.cluster.x-k8s.io,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinesets;machinesets/status,verbs=get;list;watch;create;update;patch;delete

// MachineSetReconciler reconciles a MachineSet object
type MachineSetReconciler struct {
	Client client.Client
	Log    logr.Logger

	TemplateCloner TemplateCloner

	recorder record.EventRecorder
	scheme   *runtime.Scheme
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

	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	r.recorder = mgr.GetEventRecorderFor("machineset-controller")

	r.scheme = mgr.GetScheme()
	return nil
}

func (r *MachineSetReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	logger := r.Log.WithValues("machineset", req.Name, "namespace", req.Namespace)

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
	if !machineSet.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	result, err := r.reconcile(ctx, machineSet)
	if err != nil {
		logger.Error(err, "Failed to reconcile MachineSet")
		r.recorder.Eventf(machineSet, corev1.EventTypeWarning, "ReconcileError", "%v", err)
	}
	return result, err
}

func (r *MachineSetReconciler) reconcile(ctx context.Context, machineSet *clusterv1.MachineSet) (ctrl.Result, error) {
	logger := r.Log.WithValues("machineset", machineSet.Name, "namespace", machineSet.Namespace)
	logger.V(4).Info("Reconcile MachineSet")

	// Reconcile and retrieve the Cluster object.
	if machineSet.Labels == nil {
		machineSet.Labels = make(map[string]string)
	}
	machineSet.Labels[clusterv1.ClusterLabelName] = machineSet.Spec.ClusterName

	cluster, err := util.GetClusterByName(ctx, r.Client, machineSet.ObjectMeta.Namespace, machineSet.Spec.ClusterName)
	if err != nil {
		return ctrl.Result{}, err
	}

	if r.shouldAdopt(machineSet) {
		patch := client.MergeFrom(machineSet.DeepCopy())
		machineSet.OwnerReferences = util.EnsureOwnerRef(machineSet.OwnerReferences, metav1.OwnerReference{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "Cluster",
			Name:       cluster.Name,
			UID:        cluster.UID,
		})
		// Patch using a deep copy to avoid overwriting any unexpected Status changes from the returned result
		if err := r.Client.Patch(ctx, machineSet.DeepCopy(), patch); err != nil {
			return ctrl.Result{}, errors.Wrapf(
				err,
				"failed to add OwnerReference to MachineSet %s/%s",
				machineSet.Namespace,
				machineSet.Name,
			)
		}
	}

	// Make sure to reconcile the external infrastructure reference.
	if err := r.reconcileExternalReference(ctx, cluster, machineSet.Spec.Template.Spec.InfrastructureRef); err != nil {
		return ctrl.Result{}, err
	}
	// Make sure to reconcile the external bootstrap reference, if any.
	if machineSet.Spec.Template.Spec.Bootstrap.ConfigRef != nil {
		if err := r.reconcileExternalReference(ctx, cluster, *machineSet.Spec.Template.Spec.Bootstrap.ConfigRef); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Make sure selector and template to be in the same cluster.
	machineSet.Spec.Selector.MatchLabels[clusterv1.ClusterLabelName] = machineSet.Spec.ClusterName
	machineSet.Spec.Template.Labels[clusterv1.ClusterLabelName] = machineSet.Spec.ClusterName

	// Add label and selector based on the MachineSet's name.
	machineSet.Spec.Selector.MatchLabels[clusterv1.MachineSetLabelName] = machineSet.Name
	machineSet.Spec.Template.Labels[clusterv1.MachineSetLabelName] = machineSet.Name

	selector, err := metav1.LabelSelectorAsSelector(&machineSet.Spec.Selector)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to parse MachineSet %q label selector", machineSet.Name)
	}
	// Copy label selector to its status counterpart in string format.
	// This is necessary for CRDs including scale subresources.
	machineSet.Status.Selector = selector.String()

	selectorMap, err := metav1.LabelSelectorAsMap(&machineSet.Spec.Selector)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to convert MachineSet %q label selector to a map", machineSet.Name)
	}

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

	// Filter out irrelevant machines (deleting/mismatch labels) and claim orphaned machines.
	filteredMachines := make([]*clusterv1.Machine, 0, len(allMachines.Items))
	for idx := range allMachines.Items {
		machine := &allMachines.Items[idx]
		if shouldExcludeMachine(machineSet, machine, logger) {
			continue
		}

		// Attempt to adopt machine if it meets previous conditions and it has no controller references.
		if metav1.GetControllerOf(machine) == nil {
			if err := r.adoptOrphan(ctx, machineSet, machine); err != nil {
				logger.Error(err, "Failed to adopt Machine", "machine", machine.Name)
				r.recorder.Eventf(machineSet, corev1.EventTypeWarning, "FailedAdopt", "Failed to adopt Machine %q: %v", machine.Name, err)
				continue
			}
			logger.Info("Adopted Machine", "machine", machine.Name)
			r.recorder.Eventf(machineSet, corev1.EventTypeNormal, "SuccessfulAdopt", "Adopted Machine %q", machine.Name)
		}

		filteredMachines = append(filteredMachines, machine)
	}

	syncErr := r.syncReplicas(ctx, machineSet, filteredMachines)

	ms := machineSet.DeepCopy()
	newStatus := r.calculateStatus(ms, filteredMachines)

	// Always updates status as machines come up or die.
	updatedMS, err := updateMachineSetStatus(r.Client, machineSet, newStatus, logger)
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
		logger.V(4).Info("Some nodes are not ready yet, requeuing until they are ready")
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

func (r *MachineSetReconciler) reconcileExternalReference(ctx context.Context, cluster *clusterv1.Cluster, ref corev1.ObjectReference) error {
	if !strings.HasSuffix(ref.Kind, external.TemplateSuffix) {
		return nil
	}

	obj, err := external.Get(ctx, r.Client, &ref, cluster.Namespace)
	if err != nil {
		return err
	}

	patchHelper, err := patch.NewHelper(obj, r.Client)
	if err != nil {
		return err
	}

	obj.SetOwnerReferences(util.EnsureOwnerRef(obj.GetOwnerReferences(), metav1.OwnerReference{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Cluster",
		Name:       cluster.Name,
		UID:        cluster.UID,
	}))

	if err := patchHelper.Patch(ctx, obj); err != nil {
		return err
	}
	return nil
}

// syncReplicas scales Machine resources up or down.
func (r *MachineSetReconciler) syncReplicas(ctx context.Context, ms *clusterv1.MachineSet, machines []*clusterv1.Machine) error {
	logger := r.Log.WithValues("machineset", ms.Name, "namespace", ms.Namespace)
	if ms.Spec.Replicas == nil {
		return errors.Errorf("the Replicas field in Spec for machineset %v is nil, this should not be allowed", ms.Name)
	}

	diff := len(machines) - int(*(ms.Spec.Replicas))

	if diff < 0 {
		diff *= -1
		logger.Info("Too few replicas", "need", *(ms.Spec.Replicas), "creating", diff)

		var machineList []*clusterv1.Machine
		var errstrings []string
		for i := 0; i < diff; i++ {
			logger.Info(fmt.Sprintf("Creating machine %d of %d, ( spec.replicas(%d) > currentMachineCount(%d) )",
				i+1, diff, *(ms.Spec.Replicas), len(machines)))

			machine := r.getNewMachine(ms)

			// Clone and set the infrastructure and bootstrap references.
			var (
				infraRef, bootstrapRef *corev1.ObjectReference
				err                    error
			)

			if machine.Spec.Bootstrap.ConfigRef != nil {
				bootstrapRef, err = r.TemplateCloner.CloneTemplate(ctx, r.Client, machine.Spec.Bootstrap.ConfigRef, machine.Namespace, machine.Spec.ClusterName, nil)
				if err != nil {
					return errors.Wrapf(err, "failed to clone bootstrap configuration for MachineSet %q in namespace %q", ms.Name, ms.Namespace)
				}
				machine.Spec.Bootstrap.ConfigRef = bootstrapRef
			}

			infraRef, err = r.TemplateCloner.CloneTemplate(ctx, r.Client, &machine.Spec.InfrastructureRef, machine.Namespace, machine.Spec.ClusterName, nil)
			if err != nil {
				return errors.Wrapf(err, "failed to clone infrastructure configuration for MachineSet %q in namespace %q", ms.Name, ms.Namespace)
			}
			machine.Spec.InfrastructureRef = *infraRef

			if err := r.Client.Create(context.TODO(), machine); err != nil {
				logger.Error(err, "Unable to create Machine", "machine", machine.Name)
				r.recorder.Eventf(ms, corev1.EventTypeWarning, "FailedCreate", "Failed to create machine %q: %v", machine.Name, err)
				errstrings = append(errstrings, err.Error())
				infraConfig := &unstructured.Unstructured{}
				infraConfig.SetKind(infraRef.Kind)
				infraConfig.SetAPIVersion(infraRef.APIVersion)
				infraConfig.SetNamespace(infraRef.Namespace)
				infraConfig.SetName(infraRef.Name)
				if err := r.Client.Delete(context.TODO(), infraConfig); !apierrors.IsNotFound(err) {
					logger.Error(err, "Failed to cleanup infrastructure configuration object after Machine creation error")
				}
				if bootstrapRef != nil {
					bootstrapConfig := &unstructured.Unstructured{}
					bootstrapConfig.SetKind(bootstrapRef.Kind)
					bootstrapConfig.SetAPIVersion(bootstrapRef.APIVersion)
					bootstrapConfig.SetNamespace(bootstrapRef.Namespace)
					bootstrapConfig.SetName(bootstrapRef.Name)
					if err := r.Client.Delete(context.TODO(), bootstrapConfig); !apierrors.IsNotFound(err) {
						logger.Error(err, "Failed to cleanup bootstrap configuration object after Machine creation error")
					}
				}
				continue
			}
			logger.Info(fmt.Sprintf("Created machine %d of %d with name %q", i+1, diff, machine.Name))
			r.recorder.Eventf(ms, corev1.EventTypeNormal, "SuccessfulCreate", "Created machine %q", machine.Name)

			machineList = append(machineList, machine)
		}

		if len(errstrings) > 0 {
			return errors.New(strings.Join(errstrings, "; "))
		}

		return r.waitForMachineCreation(machineList)
	} else if diff > 0 {
		logger.Info("Too many replicas", "need", *(ms.Spec.Replicas), "deleting", diff)

		deletePriorityFunc, err := getDeletePriorityFunc(ms)
		if err != nil {
			return err
		}
		logger.Info("Found delete policy", "delete-policy", ms.Spec.DeletePolicy)
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
					logger.Error(err, "Unable to delete Machine", "machine", targetMachine.Name)
					r.recorder.Eventf(ms, corev1.EventTypeWarning, "FailedDelete", "Failed to delete machine %q: %v", targetMachine.Name, err)
					errCh <- err
				}
				logger.Info("Deleted machine", "machine", targetMachine.Name)
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
	machine.Spec.ClusterName = machineSet.Spec.ClusterName
	if machine.Labels == nil {
		machine.Labels = make(map[string]string)
	}
	return machine
}

// shouldExcludeMachine returns true if the machine should be filtered out, false otherwise.
func shouldExcludeMachine(machineSet *clusterv1.MachineSet, machine *clusterv1.Machine, logger logr.Logger) bool {
	if metav1.GetControllerOf(machine) != nil && !metav1.IsControlledBy(machine, machineSet) {
		logger.V(4).Info("Machine is not controlled by machineset", "machine", machine.Name)
		return true
	}
	return !machine.ObjectMeta.DeletionTimestamp.IsZero()
}

// adoptOrphan sets the MachineSet as a controller OwnerReference to the Machine.
func (r *MachineSetReconciler) adoptOrphan(ctx context.Context, machineSet *clusterv1.MachineSet, machine *clusterv1.Machine) error {
	patch := client.MergeFrom(machine.DeepCopy())
	newRef := *metav1.NewControllerRef(machineSet, machineSetKind)
	machine.OwnerReferences = append(machine.OwnerReferences, newRef)
	return r.Client.Patch(ctx, machine, patch)
}

func (r *MachineSetReconciler) waitForMachineCreation(machineList []*clusterv1.Machine) error {
	for i := 0; i < len(machineList); i++ {
		machine := machineList[i]
		pollErr := util.PollImmediate(stateConfirmationInterval, stateConfirmationTimeout, func() (bool, error) {
			key := client.ObjectKey{Namespace: machine.Namespace, Name: machine.Name}

			if err := r.Client.Get(context.Background(), key, &clusterv1.Machine{}); err != nil {
				if apierrors.IsNotFound(err) {
					return false, nil
				}
				r.Log.Error(err, "Error getting machines")
				return false, err
			}

			return true, nil
		})

		if pollErr != nil {
			r.Log.Error(pollErr, "Failed waiting for machine object to be created")
			return errors.Wrap(pollErr, "failed waiting for machine object to be created")
		}
	}

	return nil
}

func (r *MachineSetReconciler) waitForMachineDeletion(machineList []*clusterv1.Machine) error {
	for i := 0; i < len(machineList); i++ {
		machine := machineList[i]
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
			r.Log.Error(pollErr, "Failed waiting for machine object to be deleted")
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
		r.Log.Error(nil, fmt.Sprintf("Expected a Machine but got a %T", o.Object))
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
		r.Log.V(4).Info("Found no MachineSet for Machine", "machine", m.Name)
		return nil
	}

	for _, ms := range mss {
		name := client.ObjectKey{Namespace: ms.Namespace, Name: ms.Name}
		result = append(result, ctrl.Request{NamespacedName: name})
	}

	return result
}

func (r *MachineSetReconciler) getMachineSetsForMachine(m *clusterv1.Machine) []*clusterv1.MachineSet {
	logger := r.Log.WithValues("machine", m.Name, "namespace", m.Namespace)

	if len(m.Labels) == 0 {
		logger.Info("No machine sets found because it has no labels")
		return nil
	}

	msList := &clusterv1.MachineSetList{}
	err := r.Client.List(context.Background(), msList, client.InNamespace(m.Namespace))
	if err != nil {
		logger.Error(err, "Failed to list machine sets")
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
	logger := r.Log.WithValues("machineset", machineSet.Name, "namespace", machineSet.Namespace, "machine", machine.Name)

	selector, err := metav1.LabelSelectorAsSelector(&machineSet.Spec.Selector)
	if err != nil {
		logger.Error(err, "Unable to convert selector")
		return false
	}

	// If a deployment with a nil or empty selector creeps in, it should match nothing, not everything.
	if selector.Empty() {
		logger.V(2).Info("Machineset has empty selector")
		return false
	}

	if !selector.Matches(labels.Set(machine.Labels)) {
		logger.V(4).Info("Machine has mismatch labels")
		return false
	}

	return true
}

func (r *MachineSetReconciler) shouldAdopt(ms *clusterv1.MachineSet) bool {
	return !util.HasOwner(ms.OwnerReferences, clusterv1.GroupVersion.String(), []string{"MachineDeployment", "Cluster"})
}
