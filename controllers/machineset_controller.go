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
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/controllers/noderefutil"
	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
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
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinesets;machinesets/status;machinesets/finalizers,verbs=get;list;watch;create;update;patch;delete

// MachineSetReconciler reconciles a MachineSet object.
type MachineSetReconciler struct {
	Client  client.Client
	Tracker *remote.ClusterCacheTracker

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string

	recorder record.EventRecorder
}

func (r *MachineSetReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	clusterToMachineSets, err := util.ClusterToObjectsMapper(mgr.GetClient(), &clusterv1.MachineSetList{}, mgr.GetScheme())
	if err != nil {
		return err
	}

	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.MachineSet{}).
		Owns(&clusterv1.Machine{}).
		Watches(
			&source.Kind{Type: &clusterv1.Machine{}},
			handler.EnqueueRequestsFromMapFunc(r.MachineToMachineSets),
		).
		WithOptions(options).
		WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(ctrl.LoggerFrom(ctx), r.WatchFilterValue)).
		Build(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	err = c.Watch(
		&source.Kind{Type: &clusterv1.Cluster{}},
		handler.EnqueueRequestsFromMapFunc(clusterToMachineSets),
		// TODO: should this wait for Cluster.Status.InfrastructureReady similar to Infra Machine resources?
		predicates.All(ctrl.LoggerFrom(ctx),
			predicates.ClusterUnpaused(ctrl.LoggerFrom(ctx)),
			predicates.ResourceHasFilterLabel(ctrl.LoggerFrom(ctx), r.WatchFilterValue),
		),
	)
	if err != nil {
		return errors.Wrap(err, "failed to add Watch for Clusters to controller manager")
	}

	r.recorder = mgr.GetEventRecorderFor("machineset-controller")
	return nil
}

func (r *MachineSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	log := ctrl.LoggerFrom(ctx)

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

	cluster, err := util.GetClusterByName(ctx, r.Client, machineSet.ObjectMeta.Namespace, machineSet.Spec.ClusterName)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Return early if the object or Cluster is paused.
	if annotations.IsPaused(cluster, machineSet) {
		log.Info("Reconciliation is paused for this object")
		return ctrl.Result{}, nil
	}

	// Initialize the patch helper
	patchHelper, err := patch.NewHelper(machineSet, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	defer func() {
		// Always attempt to patch the object and status after each reconciliation.
		if err := patchMachineSet(ctx, patchHelper, machineSet); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}
	}()

	// Ignore deleted MachineSets, this can happen when foregroundDeletion
	// is enabled
	if !machineSet.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	result, err := r.reconcile(ctx, cluster, machineSet)
	if err != nil {
		log.Error(err, "Failed to reconcile MachineSet")
		r.recorder.Eventf(machineSet, corev1.EventTypeWarning, "ReconcileError", "%v", err)
	}
	return result, err
}

func patchMachineSet(ctx context.Context, patchHelper *patch.Helper, machineSet *clusterv1.MachineSet, options ...patch.Option) error {
	// Always update the readyCondition by summarizing the state of other conditions.
	conditions.SetSummary(machineSet,
		conditions.WithConditions(
			clusterv1.MachinesCreatedCondition,
			clusterv1.ResizedCondition,
			clusterv1.MachinesReadyCondition,
		),
	)

	// Patch the object, ignoring conflicts on the conditions owned by this controller.
	options = append(options,
		patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
			clusterv1.ReadyCondition,
			clusterv1.MachinesCreatedCondition,
			clusterv1.ResizedCondition,
			clusterv1.MachinesReadyCondition,
		}},
	)
	return patchHelper.Patch(ctx, machineSet, options...)
}

func (r *MachineSetReconciler) reconcile(ctx context.Context, cluster *clusterv1.Cluster, machineSet *clusterv1.MachineSet) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.V(4).Info("Reconcile MachineSet")

	// Reconcile and retrieve the Cluster object.
	if machineSet.Labels == nil {
		machineSet.Labels = make(map[string]string)
	}
	machineSet.Labels[clusterv1.ClusterLabelName] = machineSet.Spec.ClusterName

	if r.shouldAdopt(machineSet) {
		machineSet.OwnerReferences = util.EnsureOwnerRef(machineSet.OwnerReferences, metav1.OwnerReference{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "Cluster",
			Name:       cluster.Name,
			UID:        cluster.UID,
		})
	}

	// Make sure to reconcile the external infrastructure reference.
	if err := reconcileExternalTemplateReference(ctx, r.Client, cluster, &machineSet.Spec.Template.Spec.InfrastructureRef); err != nil {
		return ctrl.Result{}, err
	}
	// Make sure to reconcile the external bootstrap reference, if any.
	if machineSet.Spec.Template.Spec.Bootstrap.ConfigRef != nil {
		if err := reconcileExternalTemplateReference(ctx, r.Client, cluster, machineSet.Spec.Template.Spec.Bootstrap.ConfigRef); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Make sure selector and template to be in the same cluster.
	if machineSet.Spec.Selector.MatchLabels == nil {
		machineSet.Spec.Selector.MatchLabels = make(map[string]string)
	}

	if machineSet.Spec.Template.Labels == nil {
		machineSet.Spec.Template.Labels = make(map[string]string)
	}

	machineSet.Spec.Selector.MatchLabels[clusterv1.ClusterLabelName] = machineSet.Spec.ClusterName
	machineSet.Spec.Template.Labels[clusterv1.ClusterLabelName] = machineSet.Spec.ClusterName

	selectorMap, err := metav1.LabelSelectorAsMap(&machineSet.Spec.Selector)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to convert MachineSet %q label selector to a map", machineSet.Name)
	}

	// Get all Machines linked to this MachineSet.
	allMachines := &clusterv1.MachineList{}
	err = r.Client.List(ctx,
		allMachines,
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
		if shouldExcludeMachine(machineSet, machine) {
			continue
		}

		// Attempt to adopt machine if it meets previous conditions and it has no controller references.
		if metav1.GetControllerOf(machine) == nil {
			if err := r.adoptOrphan(ctx, machineSet, machine); err != nil {
				log.Error(err, "Failed to adopt Machine", "machine", machine.Name)
				r.recorder.Eventf(machineSet, corev1.EventTypeWarning, "FailedAdopt", "Failed to adopt Machine %q: %v", machine.Name, err)
				continue
			}
			log.Info("Adopted Machine", "machine", machine.Name)
			r.recorder.Eventf(machineSet, corev1.EventTypeNormal, "SuccessfulAdopt", "Adopted Machine %q", machine.Name)
		}

		filteredMachines = append(filteredMachines, machine)
	}

	var errs []error
	for _, machine := range filteredMachines {
		// filteredMachines contains machines in deleting status to calculate correct status.
		// skip remediation for those in deleting status.
		if !machine.DeletionTimestamp.IsZero() {
			continue
		}
		if conditions.IsFalse(machine, clusterv1.MachineOwnerRemediatedCondition) {
			log.Info("Deleting unhealthy machine", "machine", machine.GetName())
			patch := client.MergeFrom(machine.DeepCopy())
			if err := r.Client.Delete(ctx, machine); err != nil {
				errs = append(errs, errors.Wrap(err, "failed to delete"))
				continue
			}
			conditions.MarkTrue(machine, clusterv1.MachineOwnerRemediatedCondition)
			if err := r.Client.Status().Patch(ctx, machine, patch); err != nil && !apierrors.IsNotFound(err) {
				errs = append(errs, errors.Wrap(err, "failed to update status"))
			}
		}
	}

	err = kerrors.NewAggregate(errs)
	if err != nil {
		log.Info("Failed while deleting unhealthy machines", "err", err)
		return ctrl.Result{}, errors.Wrap(err, "failed to remediate machines")
	}

	syncErr := r.syncReplicas(ctx, machineSet, filteredMachines)

	// Always updates status as machines come up or die.
	if err := r.updateStatus(ctx, cluster, machineSet, filteredMachines); err != nil {
		return ctrl.Result{}, errors.Wrapf(kerrors.NewAggregate([]error{err, syncErr}), "failed to update MachineSet's Status")
	}

	if syncErr != nil {
		return ctrl.Result{}, errors.Wrapf(syncErr, "failed to sync MachineSet replicas")
	}

	var replicas int32
	if machineSet.Spec.Replicas != nil {
		replicas = *machineSet.Spec.Replicas
	}

	// Resync the MachineSet after MinReadySeconds as a last line of defense to guard against clock-skew.
	// Clock-skew is an issue as it may impact whether an available replica is counted as a ready replica.
	// A replica is available if the amount of time since last transition exceeds MinReadySeconds.
	// If there was a clock skew, checking whether the amount of time since last transition to ready state
	// exceeds MinReadySeconds could be incorrect.
	// To avoid an available replica stuck in the ready state, we force a reconcile after MinReadySeconds,
	// at which point it should confirm any available replica to be available.
	if machineSet.Spec.MinReadySeconds > 0 &&
		machineSet.Status.ReadyReplicas == replicas &&
		machineSet.Status.AvailableReplicas != replicas {
		return ctrl.Result{RequeueAfter: time.Duration(machineSet.Spec.MinReadySeconds) * time.Second}, nil
	}

	// Quickly reconcile until the nodes become Ready.
	if machineSet.Status.ReadyReplicas != replicas {
		log.V(4).Info("Some nodes are not ready yet, requeuing until they are ready")
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

// syncReplicas scales Machine resources up or down.
func (r *MachineSetReconciler) syncReplicas(ctx context.Context, ms *clusterv1.MachineSet, machines []*clusterv1.Machine) error {
	log := ctrl.LoggerFrom(ctx)
	if ms.Spec.Replicas == nil {
		return errors.Errorf("the Replicas field in Spec for machineset %v is nil, this should not be allowed", ms.Name)
	}
	diff := len(machines) - int(*(ms.Spec.Replicas))
	switch {
	case diff < 0:
		diff *= -1
		log.Info("Too few replicas", "need", *(ms.Spec.Replicas), "creating", diff)
		if ms.Annotations != nil {
			if _, ok := ms.Annotations[clusterv1.DisableMachineCreate]; ok {
				log.V(2).Info("Automatic creation of new machines disabled for machine set")
				return nil
			}
		}
		var (
			machineList []*clusterv1.Machine
			errs        []error
		)

		for i := 0; i < diff; i++ {
			log.Info(fmt.Sprintf("Creating machine %d of %d, ( spec.replicas(%d) > currentMachineCount(%d) )",
				i+1, diff, *(ms.Spec.Replicas), len(machines)))

			machine := r.getNewMachine(ms)

			// Clone and set the infrastructure and bootstrap references.
			var (
				infraRef, bootstrapRef *corev1.ObjectReference
				err                    error
			)

			if machine.Spec.Bootstrap.ConfigRef != nil {
				bootstrapRef, err = external.CloneTemplate(ctx, &external.CloneTemplateInput{
					Client:      r.Client,
					TemplateRef: machine.Spec.Bootstrap.ConfigRef,
					Namespace:   machine.Namespace,
					ClusterName: machine.Spec.ClusterName,
					Labels:      machine.Labels,
					Annotations: machine.Annotations,
				})
				if err != nil {
					conditions.MarkFalse(ms, clusterv1.MachinesCreatedCondition, clusterv1.BootstrapTemplateCloningFailedReason, clusterv1.ConditionSeverityError, err.Error())
					return errors.Wrapf(err, "failed to clone bootstrap configuration for MachineSet %q in namespace %q", ms.Name, ms.Namespace)
				}
				machine.Spec.Bootstrap.ConfigRef = bootstrapRef
			}

			infraRef, err = external.CloneTemplate(ctx, &external.CloneTemplateInput{
				Client:      r.Client,
				TemplateRef: &machine.Spec.InfrastructureRef,
				Namespace:   machine.Namespace,
				ClusterName: machine.Spec.ClusterName,
				Labels:      machine.Labels,
				Annotations: machine.Annotations,
			})
			if err != nil {
				conditions.MarkFalse(ms, clusterv1.MachinesCreatedCondition, clusterv1.InfrastructureTemplateCloningFailedReason, clusterv1.ConditionSeverityError, err.Error())
				return errors.Wrapf(err, "failed to clone infrastructure configuration for MachineSet %q in namespace %q", ms.Name, ms.Namespace)
			}
			machine.Spec.InfrastructureRef = *infraRef

			if err := r.Client.Create(ctx, machine); err != nil {
				log.Error(err, "Unable to create Machine", "machine", machine.Name)
				r.recorder.Eventf(ms, corev1.EventTypeWarning, "FailedCreate", "Failed to create machine %q: %v", machine.Name, err)
				errs = append(errs, err)
				conditions.MarkFalse(ms, clusterv1.MachinesCreatedCondition, clusterv1.MachineCreationFailedReason,
					clusterv1.ConditionSeverityError, err.Error())

				// Try to cleanup the external objects if the Machine creation failed.
				if err := r.Client.Delete(ctx, util.ObjectReferenceToUnstructured(*infraRef)); !apierrors.IsNotFound(err) {
					log.Error(err, "Failed to cleanup infrastructure configuration object after Machine creation error")
				}
				if bootstrapRef != nil {
					if err := r.Client.Delete(ctx, util.ObjectReferenceToUnstructured(*bootstrapRef)); !apierrors.IsNotFound(err) {
						log.Error(err, "Failed to cleanup bootstrap configuration object after Machine creation error")
					}
				}
				continue
			}

			log.Info(fmt.Sprintf("Created machine %d of %d with name %q", i+1, diff, machine.Name))
			r.recorder.Eventf(ms, corev1.EventTypeNormal, "SuccessfulCreate", "Created machine %q", machine.Name)
			machineList = append(machineList, machine)
		}

		if len(errs) > 0 {
			return kerrors.NewAggregate(errs)
		}
		return r.waitForMachineCreation(ctx, machineList)
	case diff > 0:
		log.Info("Too many replicas", "need", *(ms.Spec.Replicas), "deleting", diff)

		deletePriorityFunc, err := getDeletePriorityFunc(ms)
		if err != nil {
			return err
		}
		log.Info("Found delete policy", "delete-policy", ms.Spec.DeletePolicy)

		var errs []error
		machinesToDelete := getMachinesToDeletePrioritized(machines, diff, deletePriorityFunc)
		for _, machine := range machinesToDelete {
			if err := r.Client.Delete(ctx, machine); err != nil {
				log.Error(err, "Unable to delete Machine", "machine", machine.Name)
				r.recorder.Eventf(ms, corev1.EventTypeWarning, "FailedDelete", "Failed to delete machine %q: %v", machine.Name, err)
				errs = append(errs, err)
				continue
			}
			log.Info("Deleted machine", "machine", machine.Name)
			r.recorder.Eventf(ms, corev1.EventTypeNormal, "SuccessfulDelete", "Deleted machine %q", machine.Name)
		}

		if len(errs) > 0 {
			return kerrors.NewAggregate(errs)
		}
		return r.waitForMachineDeletion(ctx, machinesToDelete)
	}

	return nil
}

// getNewMachine creates a new Machine object. The name of the newly created resource is going
// to be created by the API server, we set the generateName field.
func (r *MachineSetReconciler) getNewMachine(machineSet *clusterv1.MachineSet) *clusterv1.Machine {
	gv := clusterv1.GroupVersion
	machine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName:    fmt.Sprintf("%s-", machineSet.Name),
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(machineSet, machineSetKind)},
			Namespace:       machineSet.Namespace,
			Labels:          machineSet.Spec.Template.Labels,
			Annotations:     machineSet.Spec.Template.Annotations,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       gv.WithKind("Machine").Kind,
			APIVersion: gv.String(),
		},
		Spec: machineSet.Spec.Template.Spec,
	}
	machine.Spec.ClusterName = machineSet.Spec.ClusterName
	if machine.Labels == nil {
		machine.Labels = make(map[string]string)
	}
	return machine
}

// shouldExcludeMachine returns true if the machine should be filtered out, false otherwise.
func shouldExcludeMachine(machineSet *clusterv1.MachineSet, machine *clusterv1.Machine) bool {
	if metav1.GetControllerOf(machine) != nil && !metav1.IsControlledBy(machine, machineSet) {
		return true
	}

	return false
}

// adoptOrphan sets the MachineSet as a controller OwnerReference to the Machine.
func (r *MachineSetReconciler) adoptOrphan(ctx context.Context, machineSet *clusterv1.MachineSet, machine *clusterv1.Machine) error {
	patch := client.MergeFrom(machine.DeepCopy())
	newRef := *metav1.NewControllerRef(machineSet, machineSetKind)
	machine.OwnerReferences = append(machine.OwnerReferences, newRef)
	return r.Client.Patch(ctx, machine, patch)
}

func (r *MachineSetReconciler) waitForMachineCreation(ctx context.Context, machineList []*clusterv1.Machine) error {
	log := ctrl.LoggerFrom(ctx)

	for i := 0; i < len(machineList); i++ {
		machine := machineList[i]
		pollErr := util.PollImmediate(stateConfirmationInterval, stateConfirmationTimeout, func() (bool, error) {
			key := client.ObjectKey{Namespace: machine.Namespace, Name: machine.Name}
			if err := r.Client.Get(ctx, key, &clusterv1.Machine{}); err != nil {
				if apierrors.IsNotFound(err) {
					return false, nil
				}
				return false, err
			}

			return true, nil
		})

		if pollErr != nil {
			log.Error(pollErr, "Failed waiting for machine object to be created")
			return errors.Wrap(pollErr, "failed waiting for machine object to be created")
		}
	}

	return nil
}

func (r *MachineSetReconciler) waitForMachineDeletion(ctx context.Context, machineList []*clusterv1.Machine) error {
	log := ctrl.LoggerFrom(ctx)

	for i := 0; i < len(machineList); i++ {
		machine := machineList[i]
		pollErr := util.PollImmediate(stateConfirmationInterval, stateConfirmationTimeout, func() (bool, error) {
			m := &clusterv1.Machine{}
			key := client.ObjectKey{Namespace: machine.Namespace, Name: machine.Name}
			err := r.Client.Get(ctx, key, m)
			if apierrors.IsNotFound(err) || !m.DeletionTimestamp.IsZero() {
				return true, nil
			}
			return false, err
		})

		if pollErr != nil {
			log.Error(pollErr, "Failed waiting for machine object to be deleted")
			return errors.Wrap(pollErr, "failed waiting for machine object to be deleted")
		}
	}
	return nil
}

// MachineToMachineSets is a handler.ToRequestsFunc to be used to enqeue requests for reconciliation
// for MachineSets that might adopt an orphaned Machine.
func (r *MachineSetReconciler) MachineToMachineSets(o client.Object) []ctrl.Request {
	ctx := context.Background()
	// This won't log unless the global logger is set
	log := ctrl.LoggerFrom(ctx, "object", client.ObjectKeyFromObject(o))
	result := []ctrl.Request{}

	m, ok := o.(*clusterv1.Machine)
	if !ok {
		panic(fmt.Sprintf("Expected a Machine but got a %T", o))
	}

	// Check if the controller reference is already set and
	// return an empty result when one is found.
	for _, ref := range m.ObjectMeta.OwnerReferences {
		if ref.Controller != nil && *ref.Controller {
			return result
		}
	}

	mss, err := r.getMachineSetsForMachine(ctx, m)
	if err != nil {
		log.Error(err, "Failed getting MachineSets for Machine")
		return nil
	}
	if len(mss) == 0 {
		return nil
	}

	for _, ms := range mss {
		name := client.ObjectKey{Namespace: ms.Namespace, Name: ms.Name}
		result = append(result, ctrl.Request{NamespacedName: name})
	}

	return result
}

func (r *MachineSetReconciler) getMachineSetsForMachine(ctx context.Context, m *clusterv1.Machine) ([]*clusterv1.MachineSet, error) {
	if len(m.Labels) == 0 {
		return nil, fmt.Errorf("machine %v has no labels, this is unexpected", client.ObjectKeyFromObject(m))
	}

	msList := &clusterv1.MachineSetList{}
	if err := r.Client.List(ctx, msList, client.InNamespace(m.Namespace)); err != nil {
		return nil, errors.Wrapf(err, "failed to list MachineSets")
	}

	var mss []*clusterv1.MachineSet
	for idx := range msList.Items {
		ms := &msList.Items[idx]
		if hasMatchingLabels(ms.Spec.Selector, m.Labels) {
			mss = append(mss, ms)
		}
	}

	return mss, nil
}

func (r *MachineSetReconciler) shouldAdopt(ms *clusterv1.MachineSet) bool {
	return !util.HasOwner(ms.OwnerReferences, clusterv1.GroupVersion.String(), []string{"MachineDeployment", "Cluster"})
}

// updateStatus updates the Status field for the MachineSet
// It checks for the current state of the replicas and updates the Status of the MachineSet.
func (r *MachineSetReconciler) updateStatus(ctx context.Context, cluster *clusterv1.Cluster, ms *clusterv1.MachineSet, filteredMachines []*clusterv1.Machine) error {
	log := ctrl.LoggerFrom(ctx)
	newStatus := ms.Status.DeepCopy()

	// Copy label selector to its status counterpart in string format.
	// This is necessary for CRDs including scale subresources.
	selector, err := metav1.LabelSelectorAsSelector(&ms.Spec.Selector)
	if err != nil {
		return errors.Wrapf(err, "failed to update status for MachineSet %s/%s", ms.Namespace, ms.Name)
	}
	newStatus.Selector = selector.String()

	// Count the number of machines that have labels matching the labels of the machine
	// template of the replica set, the matching machines may have more
	// labels than are in the template. Because the label of machineTemplateSpec is
	// a superset of the selector of the replica set, so the possible
	// matching machines must be part of the filteredMachines.
	fullyLabeledReplicasCount := 0
	readyReplicasCount := 0
	availableReplicasCount := 0
	desiredReplicas := *ms.Spec.Replicas
	templateLabel := labels.Set(ms.Spec.Template.Labels).AsSelectorPreValidated()

	for _, machine := range filteredMachines {
		if templateLabel.Matches(labels.Set(machine.Labels)) {
			fullyLabeledReplicasCount++
		}

		if machine.Status.NodeRef == nil {
			log.V(2).Info("Unable to retrieve Node status, missing NodeRef", "machine", machine.Name)
			continue
		}

		node, err := r.getMachineNode(ctx, cluster, machine)
		if err != nil {
			log.Error(err, "Unable to retrieve Node status")
			continue
		}

		if noderefutil.IsNodeReady(node) {
			readyReplicasCount++
			if noderefutil.IsNodeAvailable(node, ms.Spec.MinReadySeconds, metav1.Now()) {
				availableReplicasCount++
			}
		}
	}

	newStatus.Replicas = int32(len(filteredMachines))
	newStatus.FullyLabeledReplicas = int32(fullyLabeledReplicasCount)
	newStatus.ReadyReplicas = int32(readyReplicasCount)
	newStatus.AvailableReplicas = int32(availableReplicasCount)

	// Copy the newly calculated status into the machineset
	if ms.Status.Replicas != newStatus.Replicas ||
		ms.Status.FullyLabeledReplicas != newStatus.FullyLabeledReplicas ||
		ms.Status.ReadyReplicas != newStatus.ReadyReplicas ||
		ms.Status.AvailableReplicas != newStatus.AvailableReplicas ||
		ms.Generation != ms.Status.ObservedGeneration {
		// Save the generation number we acted on, otherwise we might wrongfully indicate
		// that we've seen a spec update when we retry.
		newStatus.ObservedGeneration = ms.Generation
		newStatus.DeepCopyInto(&ms.Status)

		log.V(4).Info(fmt.Sprintf("Updating status for %v: %s/%s, ", ms.Kind, ms.Namespace, ms.Name) +
			fmt.Sprintf("replicas %d->%d (need %d), ", ms.Status.Replicas, newStatus.Replicas, desiredReplicas) +
			fmt.Sprintf("fullyLabeledReplicas %d->%d, ", ms.Status.FullyLabeledReplicas, newStatus.FullyLabeledReplicas) +
			fmt.Sprintf("readyReplicas %d->%d, ", ms.Status.ReadyReplicas, newStatus.ReadyReplicas) +
			fmt.Sprintf("availableReplicas %d->%d, ", ms.Status.AvailableReplicas, newStatus.AvailableReplicas) +
			fmt.Sprintf("sequence No: %v->%v", ms.Status.ObservedGeneration, newStatus.ObservedGeneration))
	}
	switch {
	// We are scaling up
	case newStatus.Replicas < desiredReplicas:
		conditions.MarkFalse(ms, clusterv1.ResizedCondition, clusterv1.ScalingUpReason, clusterv1.ConditionSeverityWarning, "Scaling up MachineSet to %d replicas (actual %d)", desiredReplicas, newStatus.Replicas)
	// We are scaling down
	case newStatus.Replicas > desiredReplicas:
		conditions.MarkFalse(ms, clusterv1.ResizedCondition, clusterv1.ScalingDownReason, clusterv1.ConditionSeverityWarning, "Scaling down MachineSet to %d replicas (actual %d)", desiredReplicas, newStatus.Replicas)
		// This means that there was no error in generating the desired number of machine objects
		conditions.MarkTrue(ms, clusterv1.MachinesCreatedCondition)
	default:
		// Make sure last resize operation is marked as completed.
		// NOTE: we are checking the number of machines ready so we report resize completed only when the machines
		// are actually provisioned (vs reporting completed immediately after the last machine object is created). This convention is also used by KCP.
		if newStatus.ReadyReplicas == newStatus.Replicas {
			conditions.MarkTrue(ms, clusterv1.ResizedCondition)
		}
		// This means that there was no error in generating the desired number of machine objects
		conditions.MarkTrue(ms, clusterv1.MachinesCreatedCondition)
	}

	// Aggregate the operational state of all the machines; while aggregating we are adding the
	// source ref (reason@machine/name) so the problem can be easily tracked down to its source machine.
	conditions.SetAggregate(ms, clusterv1.MachinesReadyCondition, collections.FromMachines(filteredMachines...).ConditionGetters(), conditions.AddSourceRef(), conditions.WithStepCounterIf(false))

	return nil
}

func (r *MachineSetReconciler) getMachineNode(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) (*corev1.Node, error) {
	remoteClient, err := r.Tracker.GetClient(ctx, util.ObjectKey(cluster))
	if err != nil {
		return nil, err
	}
	node := &corev1.Node{}
	if err := remoteClient.Get(ctx, client.ObjectKey{Name: machine.Status.NodeRef.Name}, node); err != nil {
		return nil, errors.Wrapf(err, "error retrieving node %s for machine %s/%s", machine.Status.NodeRef.Name, machine.Namespace, machine.Name)
	}
	return node, nil
}

func reconcileExternalTemplateReference(ctx context.Context, c client.Client, cluster *clusterv1.Cluster, ref *corev1.ObjectReference) error {
	if !strings.HasSuffix(ref.Kind, clusterv1.TemplateSuffix) {
		return nil
	}

	if err := utilconversion.UpdateReferenceAPIContract(ctx, c, ref); err != nil {
		return err
	}

	obj, err := external.Get(ctx, c, ref, cluster.Namespace)
	if err != nil {
		return err
	}

	patchHelper, err := patch.NewHelper(obj, c)
	if err != nil {
		return err
	}

	obj.SetOwnerReferences(util.EnsureOwnerRef(obj.GetOwnerReferences(), metav1.OwnerReference{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Cluster",
		Name:       cluster.Name,
		UID:        cluster.UID,
	}))

	return patchHelper.Patch(ctx, obj)
}
