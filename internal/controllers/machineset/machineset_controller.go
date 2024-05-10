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

package machineset

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/controllers/noderefutil"
	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/internal/controllers/machine"
	"sigs.k8s.io/cluster-api/internal/util/ssa"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/cluster-api/util/labels/format"
	clog "sigs.k8s.io/cluster-api/util/log"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
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

const machineSetManagerName = "capi-machineset"

// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io;bootstrap.cluster.x-k8s.io,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinesets;machinesets/status;machinesets/finalizers,verbs=get;list;watch;create;update;patch;delete

// Reconciler reconciles a MachineSet object.
type Reconciler struct {
	Client                    client.Client
	UnstructuredCachingClient client.Client
	APIReader                 client.Reader
	Tracker                   *remote.ClusterCacheTracker

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string

	// Deprecated: DeprecatedInfraMachineNaming. Name the InfraStructureMachines after the InfraMachineTemplate.
	DeprecatedInfraMachineNaming bool

	ssaCache ssa.Cache
	recorder record.EventRecorder
}

func (r *Reconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	clusterToMachineSets, err := util.ClusterToTypedObjectsMapper(mgr.GetClient(), &clusterv1.MachineSetList{}, mgr.GetScheme())
	if err != nil {
		return err
	}

	err = ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.MachineSet{}).
		Owns(&clusterv1.Machine{}).
		Watches(
			&clusterv1.Machine{},
			handler.EnqueueRequestsFromMapFunc(r.MachineToMachineSets),
		).
		WithOptions(options).
		WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(ctrl.LoggerFrom(ctx), r.WatchFilterValue)).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(clusterToMachineSets),
			builder.WithPredicates(
				// TODO: should this wait for Cluster.Status.InfrastructureReady similar to Infra Machine resources?
				predicates.All(ctrl.LoggerFrom(ctx),
					predicates.ClusterUnpaused(ctrl.LoggerFrom(ctx)),
					predicates.ResourceHasFilterLabel(ctrl.LoggerFrom(ctx), r.WatchFilterValue),
				),
			),
		).Complete(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	r.recorder = mgr.GetEventRecorderFor("machineset-controller")
	r.ssaCache = ssa.NewCache()
	return nil
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
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

	// AddOwners adds the owners of MachineSet as k/v pairs to the logger.
	// Specifically, it will add MachineDeployment.
	ctx, log, err := clog.AddOwners(ctx, r.Client, machineSet)
	if err != nil {
		return ctrl.Result{}, err
	}

	log = log.WithValues("Cluster", klog.KRef(machineSet.ObjectMeta.Namespace, machineSet.Spec.ClusterName))
	ctx = ctrl.LoggerInto(ctx, log)

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
		// Requeue if the reconcile failed because the ClusterCacheTracker was locked for
		// the current cluster because of concurrent access.
		if errors.Is(err, remote.ErrClusterLocked) {
			log.V(5).Info("Requeuing because another worker has the lock on the ClusterCacheTracker")
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
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

func (r *Reconciler) reconcile(ctx context.Context, cluster *clusterv1.Cluster, machineSet *clusterv1.MachineSet) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// Reconcile and retrieve the Cluster object.
	if machineSet.Labels == nil {
		machineSet.Labels = make(map[string]string)
	}
	machineSet.Labels[clusterv1.ClusterNameLabel] = machineSet.Spec.ClusterName

	// If the machine set is a stand alone one, meaning not originated from a MachineDeployment, then set it as directly
	// owned by the Cluster (if not already present).
	if r.shouldAdopt(machineSet) {
		machineSet.SetOwnerReferences(util.EnsureOwnerRef(machineSet.GetOwnerReferences(), metav1.OwnerReference{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "Cluster",
			Name:       cluster.Name,
			UID:        cluster.UID,
		}))
	}

	// Make sure to reconcile the external infrastructure reference.
	if err := reconcileExternalTemplateReference(ctx, r.UnstructuredCachingClient, cluster, &machineSet.Spec.Template.Spec.InfrastructureRef); err != nil {
		return ctrl.Result{}, err
	}
	// Make sure to reconcile the external bootstrap reference, if any.
	if machineSet.Spec.Template.Spec.Bootstrap.ConfigRef != nil {
		if err := reconcileExternalTemplateReference(ctx, r.UnstructuredCachingClient, cluster, machineSet.Spec.Template.Spec.Bootstrap.ConfigRef); err != nil {
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

	machineSet.Spec.Selector.MatchLabels[clusterv1.ClusterNameLabel] = machineSet.Spec.ClusterName
	machineSet.Spec.Template.Labels[clusterv1.ClusterNameLabel] = machineSet.Spec.ClusterName

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

	// Filter out irrelevant machines (i.e. IsControlledBy something else) and claim orphaned machines.
	// Machines in deleted state are deliberately not excluded https://github.com/kubernetes-sigs/cluster-api/pull/3434.
	filteredMachines := make([]*clusterv1.Machine, 0, len(allMachines.Items))
	for idx := range allMachines.Items {
		machine := &allMachines.Items[idx]
		log := log.WithValues("Machine", klog.KObj(machine))
		if shouldExcludeMachine(machineSet, machine) {
			continue
		}

		// Attempt to adopt machine if it meets previous conditions and it has no controller references.
		if metav1.GetControllerOf(machine) == nil {
			if err := r.adoptOrphan(ctx, machineSet, machine); err != nil {
				log.Error(err, "Failed to adopt Machine")
				r.recorder.Eventf(machineSet, corev1.EventTypeWarning, "FailedAdopt", "Failed to adopt Machine %q: %v", machine.Name, err)
				continue
			}
			log.Info("Adopted Machine")
			r.recorder.Eventf(machineSet, corev1.EventTypeNormal, "SuccessfulAdopt", "Adopted Machine %q", machine.Name)
		}

		filteredMachines = append(filteredMachines, machine)
	}

	result := ctrl.Result{}

	reconcileUnhealthyMachinesResult, err := r.reconcileUnhealthyMachines(ctx, cluster, machineSet, filteredMachines)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to reconcile unhealthy machines")
	}
	result = util.LowestNonZeroResult(result, reconcileUnhealthyMachinesResult)

	if err := r.syncMachines(ctx, machineSet, filteredMachines); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to update Machines")
	}

	syncReplicasResult, syncErr := r.syncReplicas(ctx, cluster, machineSet, filteredMachines)
	result = util.LowestNonZeroResult(result, syncReplicasResult)

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
		minReadyResult := ctrl.Result{RequeueAfter: time.Duration(machineSet.Spec.MinReadySeconds) * time.Second}
		result = util.LowestNonZeroResult(result, minReadyResult)
		return result, nil
	}

	// Quickly reconcile until the nodes become Ready.
	if machineSet.Status.ReadyReplicas != replicas {
		result = util.LowestNonZeroResult(result, ctrl.Result{RequeueAfter: 15 * time.Second})
		return result, nil
	}

	return result, nil
}

// syncMachines updates Machines, InfrastructureMachine and BootstrapConfig to propagate in-place mutable fields
// from the MachineSet.
// Note: It also cleans up managed fields of all Machines so that Machines that were
// created/patched before (< v1.4.0) the controller adopted Server-Side-Apply (SSA) can also work with SSA.
// Note: For InfrastructureMachines and BootstrapConfigs it also drops ownership of "metadata.labels" and
// "metadata.annotations" from "manager" so that "capi-machineset" can own these fields and can work with SSA.
// Otherwise fields would be co-owned by our "old" "manager" and "capi-machineset" and then we would not be
// able to e.g. drop labels and annotations.
func (r *Reconciler) syncMachines(ctx context.Context, machineSet *clusterv1.MachineSet, machines []*clusterv1.Machine) error {
	log := ctrl.LoggerFrom(ctx)
	for i := range machines {
		m := machines[i]
		// If the machine is already being deleted, we don't need to update it.
		if !m.DeletionTimestamp.IsZero() {
			continue
		}

		// Cleanup managed fields of all Machines.
		// We do this so that Machines that were created/patched before the controller adopted Server-Side-Apply (SSA)
		// (< v1.4.0) can also work with SSA. Otherwise, fields would be co-owned by our "old" "manager" and
		// "capi-machineset" and then we would not be able to e.g. drop labels and annotations.
		if err := ssa.CleanUpManagedFieldsForSSAAdoption(ctx, r.Client, m, machineSetManagerName); err != nil {
			return errors.Wrapf(err, "failed to update machine: failed to adjust the managedFields of the Machine %q", m.Name)
		}

		// Update Machine to propagate in-place mutable fields from the MachineSet.
		updatedMachine := r.computeDesiredMachine(machineSet, m)
		err := ssa.Patch(ctx, r.Client, machineSetManagerName, updatedMachine, ssa.WithCachingProxy{Cache: r.ssaCache, Original: m})
		if err != nil {
			log.Error(err, "failed to update Machine", "Machine", klog.KObj(updatedMachine))
			return errors.Wrapf(err, "failed to update Machine %q", klog.KObj(updatedMachine))
		}
		machines[i] = updatedMachine

		infraMachine, err := external.Get(ctx, r.UnstructuredCachingClient, &updatedMachine.Spec.InfrastructureRef, updatedMachine.Namespace)
		if err != nil {
			return errors.Wrapf(err, "failed to get InfrastructureMachine %s",
				klog.KRef(updatedMachine.Spec.InfrastructureRef.Namespace, updatedMachine.Spec.InfrastructureRef.Name))
		}
		// Cleanup managed fields of all InfrastructureMachines to drop ownership of labels and annotations
		// from "manager". We do this so that InfrastructureMachines that are created using the Create method
		// can also work with SSA. Otherwise, labels and annotations would be co-owned by our "old" "manager"
		// and "capi-machineset" and then we would not be able to e.g. drop labels and annotations.
		labelsAndAnnotationsManagedFieldPaths := []contract.Path{
			{"f:metadata", "f:annotations"},
			{"f:metadata", "f:labels"},
		}
		if err := ssa.DropManagedFields(ctx, r.Client, infraMachine, machineSetManagerName, labelsAndAnnotationsManagedFieldPaths); err != nil {
			return errors.Wrapf(err, "failed to update machine: failed to adjust the managedFields of the InfrastructureMachine %s", klog.KObj(infraMachine))
		}
		// Update in-place mutating fields on InfrastructureMachine.
		if err := r.updateExternalObject(ctx, infraMachine, machineSet); err != nil {
			return errors.Wrapf(err, "failed to update InfrastructureMachine %s", klog.KObj(infraMachine))
		}

		if updatedMachine.Spec.Bootstrap.ConfigRef != nil {
			bootstrapConfig, err := external.Get(ctx, r.UnstructuredCachingClient, updatedMachine.Spec.Bootstrap.ConfigRef, updatedMachine.Namespace)
			if err != nil {
				return errors.Wrapf(err, "failed to get BootstrapConfig %s",
					klog.KRef(updatedMachine.Spec.Bootstrap.ConfigRef.Namespace, updatedMachine.Spec.Bootstrap.ConfigRef.Name))
			}
			// Cleanup managed fields of all BootstrapConfigs to drop ownership of labels and annotations
			// from "manager". We do this so that BootstrapConfigs that are created using the Create method
			// can also work with SSA. Otherwise, labels and annotations would be co-owned by our "old" "manager"
			// and "capi-machineset" and then we would not be able to e.g. drop labels and annotations.
			if err := ssa.DropManagedFields(ctx, r.Client, bootstrapConfig, machineSetManagerName, labelsAndAnnotationsManagedFieldPaths); err != nil {
				return errors.Wrapf(err, "failed to update machine: failed to adjust the managedFields of the BootstrapConfig %s", klog.KObj(bootstrapConfig))
			}
			// Update in-place mutating fields on BootstrapConfig.
			if err := r.updateExternalObject(ctx, bootstrapConfig, machineSet); err != nil {
				return errors.Wrapf(err, "failed to update BootstrapConfig %s", klog.KObj(bootstrapConfig))
			}
		}
	}
	return nil
}

// syncReplicas scales Machine resources up or down.
func (r *Reconciler) syncReplicas(ctx context.Context, cluster *clusterv1.Cluster, ms *clusterv1.MachineSet, machines []*clusterv1.Machine) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	if ms.Spec.Replicas == nil {
		return ctrl.Result{}, errors.Errorf("the Replicas field in Spec for machineset %v is nil, this should not be allowed", ms.Name)
	}
	diff := len(machines) - int(*(ms.Spec.Replicas))
	switch {
	case diff < 0:
		diff *= -1
		log.Info(fmt.Sprintf("MachineSet is scaling up to %d replicas by creating %d machines", *(ms.Spec.Replicas), diff), "replicas", *(ms.Spec.Replicas), "machineCount", len(machines))
		if ms.Annotations != nil {
			if _, ok := ms.Annotations[clusterv1.DisableMachineCreateAnnotation]; ok {
				log.Info("Automatic creation of new machines disabled for machine set")
				return ctrl.Result{}, nil
			}
		}

		result, preflightCheckErrMessage, err := r.runPreflightChecks(ctx, cluster, ms, "Scale up")
		if err != nil || !result.IsZero() {
			if err != nil {
				// If the error is not nil use that as the message for the condition.
				preflightCheckErrMessage = err.Error()
			}
			conditions.MarkFalse(ms, clusterv1.MachinesCreatedCondition, clusterv1.PreflightCheckFailedReason, clusterv1.ConditionSeverityError, preflightCheckErrMessage)
			return result, err
		}

		var (
			machineList []*clusterv1.Machine
			errs        []error
		)

		for i := 0; i < diff; i++ {
			// Create a new logger so the global logger is not modified.
			log := log
			machine := r.computeDesiredMachine(ms, nil)
			// Clone and set the infrastructure and bootstrap references.
			var (
				infraRef, bootstrapRef *corev1.ObjectReference
				err                    error
			)

			// Create the BootstrapConfig if necessary.
			if ms.Spec.Template.Spec.Bootstrap.ConfigRef != nil {
				bootstrapRef, err = external.CreateFromTemplate(ctx, &external.CreateFromTemplateInput{
					Client:      r.UnstructuredCachingClient,
					TemplateRef: ms.Spec.Template.Spec.Bootstrap.ConfigRef,
					Namespace:   machine.Namespace,
					Name:        machine.Name,
					ClusterName: machine.Spec.ClusterName,
					Labels:      machine.Labels,
					Annotations: machine.Annotations,
					OwnerRef: &metav1.OwnerReference{
						APIVersion: clusterv1.GroupVersion.String(),
						Kind:       "MachineSet",
						Name:       ms.Name,
						UID:        ms.UID,
					},
				})
				if err != nil {
					conditions.MarkFalse(ms, clusterv1.MachinesCreatedCondition, clusterv1.BootstrapTemplateCloningFailedReason, clusterv1.ConditionSeverityError, err.Error())
					return ctrl.Result{}, errors.Wrapf(err, "failed to clone bootstrap configuration from %s %s while creating a machine",
						ms.Spec.Template.Spec.Bootstrap.ConfigRef.Kind,
						klog.KRef(ms.Spec.Template.Spec.Bootstrap.ConfigRef.Namespace, ms.Spec.Template.Spec.Bootstrap.ConfigRef.Name))
				}
				machine.Spec.Bootstrap.ConfigRef = bootstrapRef
				log = log.WithValues(bootstrapRef.Kind, klog.KRef(bootstrapRef.Namespace, bootstrapRef.Name))
			}

			infraMachineName := machine.Name
			if r.DeprecatedInfraMachineNaming {
				infraMachineName = names.SimpleNameGenerator.GenerateName(ms.Spec.Template.Spec.InfrastructureRef.Name + "-")
			}
			// Create the InfraMachine.
			infraRef, err = external.CreateFromTemplate(ctx, &external.CreateFromTemplateInput{
				Client:      r.UnstructuredCachingClient,
				TemplateRef: &ms.Spec.Template.Spec.InfrastructureRef,
				Namespace:   machine.Namespace,
				Name:        infraMachineName,
				ClusterName: machine.Spec.ClusterName,
				Labels:      machine.Labels,
				Annotations: machine.Annotations,
				OwnerRef: &metav1.OwnerReference{
					APIVersion: clusterv1.GroupVersion.String(),
					Kind:       "MachineSet",
					Name:       ms.Name,
					UID:        ms.UID,
				},
			})
			if err != nil {
				conditions.MarkFalse(ms, clusterv1.MachinesCreatedCondition, clusterv1.InfrastructureTemplateCloningFailedReason, clusterv1.ConditionSeverityError, err.Error())
				return ctrl.Result{}, errors.Wrapf(err, "failed to clone infrastructure machine from %s %s while creating a machine",
					ms.Spec.Template.Spec.InfrastructureRef.Kind,
					klog.KRef(ms.Spec.Template.Spec.InfrastructureRef.Namespace, ms.Spec.Template.Spec.InfrastructureRef.Name))
			}
			log = log.WithValues(infraRef.Kind, klog.KRef(infraRef.Namespace, infraRef.Name))
			machine.Spec.InfrastructureRef = *infraRef

			// Create the Machine.
			if err := ssa.Patch(ctx, r.Client, machineSetManagerName, machine); err != nil {
				log.Error(err, "Error while creating a machine")
				r.recorder.Eventf(ms, corev1.EventTypeWarning, "FailedCreate", "Failed to create machine: %v", err)
				errs = append(errs, err)
				conditions.MarkFalse(ms, clusterv1.MachinesCreatedCondition, clusterv1.MachineCreationFailedReason,
					clusterv1.ConditionSeverityError, err.Error())

				// Try to cleanup the external objects if the Machine creation failed.
				if err := r.Client.Delete(ctx, util.ObjectReferenceToUnstructured(*infraRef)); !apierrors.IsNotFound(err) {
					log.Error(err, "Failed to cleanup infrastructure machine object after Machine creation error", infraRef.Kind, klog.KRef(infraRef.Namespace, infraRef.Name))
				}
				if bootstrapRef != nil {
					if err := r.Client.Delete(ctx, util.ObjectReferenceToUnstructured(*bootstrapRef)); !apierrors.IsNotFound(err) {
						log.Error(err, "Failed to cleanup bootstrap configuration object after Machine creation error", bootstrapRef.Kind, klog.KRef(bootstrapRef.Namespace, bootstrapRef.Name))
					}
				}
				continue
			}

			log.Info(fmt.Sprintf("Created machine %d of %d", i+1, diff), "Machine", klog.KObj(machine))
			r.recorder.Eventf(ms, corev1.EventTypeNormal, "SuccessfulCreate", "Created machine %q", machine.Name)
			machineList = append(machineList, machine)
		}

		if len(errs) > 0 {
			return ctrl.Result{}, kerrors.NewAggregate(errs)
		}
		return ctrl.Result{}, r.waitForMachineCreation(ctx, machineList)
	case diff > 0:
		log.Info(fmt.Sprintf("MachineSet is scaling down to %d replicas by deleting %d machines", *(ms.Spec.Replicas), diff), "replicas", *(ms.Spec.Replicas), "machineCount", len(machines), "deletePolicy", ms.Spec.DeletePolicy)

		deletePriorityFunc, err := getDeletePriorityFunc(ms)
		if err != nil {
			return ctrl.Result{}, err
		}

		var errs []error
		machinesToDelete := getMachinesToDeletePrioritized(machines, diff, deletePriorityFunc)
		for i, machine := range machinesToDelete {
			log := log.WithValues("Machine", klog.KObj(machine))
			if machine.GetDeletionTimestamp().IsZero() {
				log.Info(fmt.Sprintf("Deleting machine %d of %d", i+1, diff))
				if err := r.Client.Delete(ctx, machine); err != nil {
					log.Error(err, "Unable to delete Machine")
					r.recorder.Eventf(ms, corev1.EventTypeWarning, "FailedDelete", "Failed to delete machine %q: %v", machine.Name, err)
					errs = append(errs, err)
					continue
				}
				r.recorder.Eventf(ms, corev1.EventTypeNormal, "SuccessfulDelete", "Deleted machine %q", machine.Name)
			} else {
				log.Info(fmt.Sprintf("Waiting for machine %d of %d to be deleted", i+1, diff))
			}
		}

		if len(errs) > 0 {
			return ctrl.Result{}, kerrors.NewAggregate(errs)
		}
		return ctrl.Result{}, r.waitForMachineDeletion(ctx, machinesToDelete)
	}

	return ctrl.Result{}, nil
}

// computeDesiredMachine computes the desired Machine.
// This Machine will be used during reconciliation to:
// * create a Machine
// * update an existing Machine
// Because we are using Server-Side-Apply we always have to calculate the full object.
// There are small differences in how we calculate the Machine depending on if it
// is a create or update. Example: for a new Machine we have to calculate a new name,
// while for an existing Machine we have to use the name of the existing Machine.
func (r *Reconciler) computeDesiredMachine(machineSet *clusterv1.MachineSet, existingMachine *clusterv1.Machine) *clusterv1.Machine {
	desiredMachine := &clusterv1.Machine{
		TypeMeta: metav1.TypeMeta{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.SimpleNameGenerator.GenerateName(fmt.Sprintf("%s-", machineSet.Name)),
			Namespace: machineSet.Namespace,
			// Note: By setting the ownerRef on creation we signal to the Machine controller that this is not a stand-alone Machine.
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(machineSet, machineSetKind)},
			Labels:          map[string]string{},
			Annotations:     map[string]string{},
			Finalizers:      []string{clusterv1.MachineFinalizer},
		},
		Spec: *machineSet.Spec.Template.Spec.DeepCopy(),
	}
	// Set ClusterName.
	desiredMachine.Spec.ClusterName = machineSet.Spec.ClusterName

	// Clean up the refs to the incorrect objects.
	// The InfrastructureRef and the Bootstrap.ConfigRef in Machine should point to the InfrastructureMachine
	// and the BootstrapConfig objects. In the MachineSet these values point to InfrastructureMachineTemplate
	// BootstrapConfigTemplate. Drop the values that were copied over from MachineSet during DeepCopy
	// to make sure to not point to incorrect refs.
	// Note: During Machine creation, these refs will be updated with the correct values after the corresponding
	// objects are created.
	desiredMachine.Spec.InfrastructureRef = corev1.ObjectReference{}
	desiredMachine.Spec.Bootstrap.ConfigRef = nil

	// If we are updating an existing Machine reuse the name, uid, infrastructureRef and bootstrap.configRef
	// from the existingMachine.
	// Note: we use UID to force SSA to update the existing Machine and to not accidentally create a new Machine.
	// infrastructureRef and bootstrap.configRef remain the same for an existing Machine.
	if existingMachine != nil {
		desiredMachine.SetName(existingMachine.Name)
		desiredMachine.SetUID(existingMachine.UID)
		desiredMachine.Spec.Bootstrap.ConfigRef = existingMachine.Spec.Bootstrap.ConfigRef
		desiredMachine.Spec.InfrastructureRef = existingMachine.Spec.InfrastructureRef
	}

	// Set the in-place mutable fields.
	// When we create a new Machine we will just create the Machine with those fields.
	// When we update an existing Machine will we update the fields on the existing Machine (in-place mutate).

	// Set Labels
	desiredMachine.Labels = machineLabelsFromMachineSet(machineSet)

	// Set Annotations
	desiredMachine.Annotations = machineAnnotationsFromMachineSet(machineSet)

	// Set all other in-place mutable fields.
	desiredMachine.Spec.NodeDrainTimeout = machineSet.Spec.Template.Spec.NodeDrainTimeout
	desiredMachine.Spec.NodeDeletionTimeout = machineSet.Spec.Template.Spec.NodeDeletionTimeout
	desiredMachine.Spec.NodeVolumeDetachTimeout = machineSet.Spec.Template.Spec.NodeVolumeDetachTimeout

	return desiredMachine
}

// updateExternalObject updates the external object passed in with the
// updated labels and annotations from the MachineSet.
func (r *Reconciler) updateExternalObject(ctx context.Context, obj client.Object, machineSet *clusterv1.MachineSet) error {
	updatedObject := &unstructured.Unstructured{}
	updatedObject.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
	updatedObject.SetNamespace(obj.GetNamespace())
	updatedObject.SetName(obj.GetName())
	// Set the UID to ensure that Server-Side-Apply only performs an update
	// and does not perform an accidental create.
	updatedObject.SetUID(obj.GetUID())

	updatedObject.SetLabels(machineLabelsFromMachineSet(machineSet))
	updatedObject.SetAnnotations(machineAnnotationsFromMachineSet(machineSet))

	if err := ssa.Patch(ctx, r.Client, machineSetManagerName, updatedObject, ssa.WithCachingProxy{Cache: r.ssaCache, Original: obj}); err != nil {
		return errors.Wrapf(err, "failed to update %s", klog.KObj(obj))
	}
	return nil
}

// machineLabelsFromMachineSet computes the labels the Machine created from this MachineSet should have.
func machineLabelsFromMachineSet(machineSet *clusterv1.MachineSet) map[string]string {
	machineLabels := map[string]string{}
	// Note: We can't just set `machineSet.Spec.Template.Labels` directly and thus "share" the labels
	// map between Machine and machineSet.Spec.Template.Labels. This would mean that adding the
	// MachineSetNameLabel and MachineDeploymentNameLabel later on the Machine would also add the labels
	// to machineSet.Spec.Template.Labels and thus modify the labels of the MachineSet.
	for k, v := range machineSet.Spec.Template.Labels {
		machineLabels[k] = v
	}
	// Always set the MachineSetNameLabel.
	// Note: If a client tries to create a MachineSet without a selector, the MachineSet webhook
	// will add this label automatically. But we want this label to always be present even if the MachineSet
	// has a selector which doesn't include it. Therefore, we have to set it here explicitly.
	machineLabels[clusterv1.MachineSetNameLabel] = format.MustFormatValue(machineSet.Name)
	// Propagate the MachineDeploymentNameLabel from MachineSet to Machine if it exists.
	if mdName, ok := machineSet.Labels[clusterv1.MachineDeploymentNameLabel]; ok {
		machineLabels[clusterv1.MachineDeploymentNameLabel] = mdName
	}
	return machineLabels
}

// machineAnnotationsFromMachineSet computes the annotations the Machine created from this MachineSet should have.
func machineAnnotationsFromMachineSet(machineSet *clusterv1.MachineSet) map[string]string {
	annotations := map[string]string{}
	for k, v := range machineSet.Spec.Template.Annotations {
		annotations[k] = v
	}
	return annotations
}

// shouldExcludeMachine returns true if the machine should be filtered out, false otherwise.
func shouldExcludeMachine(machineSet *clusterv1.MachineSet, machine *clusterv1.Machine) bool {
	if metav1.GetControllerOf(machine) != nil && !metav1.IsControlledBy(machine, machineSet) {
		return true
	}

	return false
}

// adoptOrphan sets the MachineSet as a controller OwnerReference to the Machine.
func (r *Reconciler) adoptOrphan(ctx context.Context, machineSet *clusterv1.MachineSet, machine *clusterv1.Machine) error {
	patch := client.MergeFrom(machine.DeepCopy())
	newRef := *metav1.NewControllerRef(machineSet, machineSetKind)
	machine.SetOwnerReferences(util.EnsureOwnerRef(machine.GetOwnerReferences(), newRef))
	return r.Client.Patch(ctx, machine, patch)
}

func (r *Reconciler) waitForMachineCreation(ctx context.Context, machineList []*clusterv1.Machine) error {
	log := ctrl.LoggerFrom(ctx)

	for i := 0; i < len(machineList); i++ {
		machine := machineList[i]
		pollErr := wait.PollUntilContextTimeout(ctx, stateConfirmationInterval, stateConfirmationTimeout, true, func(ctx context.Context) (bool, error) {
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

func (r *Reconciler) waitForMachineDeletion(ctx context.Context, machineList []*clusterv1.Machine) error {
	log := ctrl.LoggerFrom(ctx)

	for i := 0; i < len(machineList); i++ {
		machine := machineList[i]
		pollErr := wait.PollUntilContextTimeout(ctx, stateConfirmationInterval, stateConfirmationTimeout, true, func(ctx context.Context) (bool, error) {
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

// MachineToMachineSets is a handler.ToRequestsFunc to be used to enqueue requests for reconciliation
// for MachineSets that might adopt an orphaned Machine.
func (r *Reconciler) MachineToMachineSets(ctx context.Context, o client.Object) []ctrl.Request {
	result := []ctrl.Request{}

	m, ok := o.(*clusterv1.Machine)
	if !ok {
		panic(fmt.Sprintf("Expected a Machine but got a %T", o))
	}

	log := ctrl.LoggerFrom(ctx, "Machine", klog.KObj(m))

	// Check if the controller reference is already set and
	// return an empty result when one is found.
	for _, ref := range m.ObjectMeta.GetOwnerReferences() {
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

func (r *Reconciler) getMachineSetsForMachine(ctx context.Context, m *clusterv1.Machine) ([]*clusterv1.MachineSet, error) {
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
		if machine.HasMatchingLabels(ms.Spec.Selector, m.Labels) {
			mss = append(mss, ms)
		}
	}

	return mss, nil
}

// shouldAdopt returns true if the MachineSet should be adopted as a stand-alone MachineSet directly owned by the Cluster.
func (r *Reconciler) shouldAdopt(ms *clusterv1.MachineSet) bool {
	// if the MachineSet is controlled by a MachineDeployment, or if it is a stand-alone MachinesSet directly owned by the Cluster, then no-op.
	if util.HasOwner(ms.GetOwnerReferences(), clusterv1.GroupVersion.String(), []string{"MachineDeployment", "Cluster"}) {
		return false
	}

	// If the MachineSet is originated by a MachineDeployment object, it should not be adopted directly by the Cluster as a stand-alone MachineSet.
	// Note: this is required because after restore from a backup both the MachineSet controller and the
	// MachineDeployment controller are racing to adopt MachineSets, see https://github.com/kubernetes-sigs/cluster-api/issues/7529
	if _, ok := ms.Labels[clusterv1.MachineDeploymentNameLabel]; ok {
		return false
	}
	return true
}

// updateStatus updates the Status field for the MachineSet
// It checks for the current state of the replicas and updates the Status of the MachineSet.
func (r *Reconciler) updateStatus(ctx context.Context, cluster *clusterv1.Cluster, ms *clusterv1.MachineSet, filteredMachines []*clusterv1.Machine) error {
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
		log := log.WithValues("Machine", klog.KObj(machine))

		if templateLabel.Matches(labels.Set(machine.Labels)) {
			fullyLabeledReplicasCount++
		}

		if machine.Status.NodeRef == nil {
			log.V(4).Info("Waiting for the machine controller to set status.NodeRef on the Machine")
			continue
		}

		node, err := r.getMachineNode(ctx, cluster, machine)
		if err != nil && machine.GetDeletionTimestamp().IsZero() {
			log.Error(err, "Unable to retrieve Node status", "node", klog.KObj(node))
			continue
		}

		if noderefutil.IsNodeReady(node) {
			readyReplicasCount++
			if noderefutil.IsNodeAvailable(node, ms.Spec.MinReadySeconds, metav1.Now()) {
				availableReplicasCount++
			}
		} else if machine.GetDeletionTimestamp().IsZero() {
			log.V(4).Info("Waiting for the Kubernetes node on the machine to report ready state")
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
		log.V(4).Info("Updating status: " +
			fmt.Sprintf("replicas %d->%d (need %d), ", ms.Status.Replicas, newStatus.Replicas, desiredReplicas) +
			fmt.Sprintf("fullyLabeledReplicas %d->%d, ", ms.Status.FullyLabeledReplicas, newStatus.FullyLabeledReplicas) +
			fmt.Sprintf("readyReplicas %d->%d, ", ms.Status.ReadyReplicas, newStatus.ReadyReplicas) +
			fmt.Sprintf("availableReplicas %d->%d, ", ms.Status.AvailableReplicas, newStatus.AvailableReplicas) +
			fmt.Sprintf("observedGeneration %v->%v", ms.Status.ObservedGeneration, ms.Generation))

		// Save the generation number we acted on, otherwise we might wrongfully indicate
		// that we've seen a spec update when we retry.
		newStatus.ObservedGeneration = ms.Generation
		newStatus.DeepCopyInto(&ms.Status)
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
			if conditions.IsFalse(ms, clusterv1.ResizedCondition) {
				log.Info("All the replicas are ready", "replicas", newStatus.ReadyReplicas)
			}
			conditions.MarkTrue(ms, clusterv1.ResizedCondition)
		}
		// This means that there was no error in generating the desired number of machine objects
		conditions.MarkTrue(ms, clusterv1.MachinesCreatedCondition)
	}

	// Aggregate the operational state of all the machines; while aggregating we are adding the
	// source ref (reason@machine/name) so the problem can be easily tracked down to its source machine.
	conditions.SetAggregate(ms, clusterv1.MachinesReadyCondition, collections.FromMachines(filteredMachines...).ConditionGetters(), conditions.AddSourceRef())

	return nil
}

func (r *Reconciler) getMachineNode(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) (*corev1.Node, error) {
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

func (r *Reconciler) reconcileUnhealthyMachines(ctx context.Context, cluster *clusterv1.Cluster, ms *clusterv1.MachineSet, filteredMachines []*clusterv1.Machine) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	// List all unhealthy machines.
	machinesToRemediate := make([]*clusterv1.Machine, 0, len(filteredMachines))
	for _, m := range filteredMachines {
		// filteredMachines contains machines in deleting status to calculate correct status.
		// skip remediation for those in deleting status.
		if !m.DeletionTimestamp.IsZero() {
			continue
		}
		if conditions.IsFalse(m, clusterv1.MachineOwnerRemediatedCondition) {
			machinesToRemediate = append(machinesToRemediate, m)
		}
	}

	// If there are no machines to remediate return early.
	if len(machinesToRemediate) == 0 {
		return ctrl.Result{}, nil
	}

	preflightChecksResult, preflightCheckErrMessage, err := r.runPreflightChecks(ctx, cluster, ms, "Machine Remediation")
	if err != nil {
		// If err is not nil use that as the preflightCheckErrMessage
		preflightCheckErrMessage = err.Error()
	}

	preflightChecksFailed := err != nil || !preflightChecksResult.IsZero()
	if preflightChecksFailed {
		// PreflightChecks did not pass. Update the MachineOwnerRemediated condition on the unhealthy Machines with
		// WaitingForRemediationReason reason.
		var errs []error
		for _, m := range machinesToRemediate {
			patchHelper, err := patch.NewHelper(m, r.Client)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			conditions.MarkFalse(m, clusterv1.MachineOwnerRemediatedCondition, clusterv1.WaitingForRemediationReason, clusterv1.ConditionSeverityWarning, preflightCheckErrMessage)
			if err := patchHelper.Patch(ctx, m); err != nil {
				errs = append(errs, err)
			}
		}

		if len(errs) > 0 {
			return ctrl.Result{}, errors.Wrapf(kerrors.NewAggregate(errs), "failed to patch unhealthy Machines")
		}
		return preflightChecksResult, nil
	}

	// PreflightChecks passed, so it is safe to remediate unhealthy machines.
	// Remediate unhealthy machines by deleting them.
	var errs []error
	for _, m := range machinesToRemediate {
		log.Info(fmt.Sprintf("Deleting Machine %s because it was marked as unhealthy by the MachineHealthCheck controller", klog.KObj(m)))
		patch := client.MergeFrom(m.DeepCopy())
		if err := r.Client.Delete(ctx, m); err != nil {
			errs = append(errs, errors.Wrapf(err, "failed to delete Machine %s", klog.KObj(m)))
			continue
		}
		conditions.MarkTrue(m, clusterv1.MachineOwnerRemediatedCondition)
		if err := r.Client.Status().Patch(ctx, m, patch); err != nil && !apierrors.IsNotFound(err) {
			errs = append(errs, errors.Wrapf(err, "failed to update status of Machine %s", klog.KObj(m)))
		}
	}

	if len(errs) > 0 {
		return ctrl.Result{}, errors.Wrapf(kerrors.NewAggregate(errs), "failed to delete unhealthy Machines")
	}

	return ctrl.Result{}, nil
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
