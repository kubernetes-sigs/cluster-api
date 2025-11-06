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
	"cmp"
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/controllers/noderefutil"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/internal/controllers/machine"
	"sigs.k8s.io/cluster-api/internal/hooks"
	topologynames "sigs.k8s.io/cluster-api/internal/topology/names"
	clientutil "sigs.k8s.io/cluster-api/internal/util/client"
	"sigs.k8s.io/cluster-api/internal/util/inplace"
	"sigs.k8s.io/cluster-api/internal/util/ssa"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
	"sigs.k8s.io/cluster-api/util/finalizers"
	"sigs.k8s.io/cluster-api/util/labels/format"
	clog "sigs.k8s.io/cluster-api/util/log"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/paused"
	"sigs.k8s.io/cluster-api/util/predicates"
)

var (
	// machineSetKind contains the schema.GroupVersionKind for the MachineSet type.
	machineSetKind = clusterv1.GroupVersion.WithKind("MachineSet")
)

const (
	machineSetManagerName         = "capi-machineset"
	machineSetMetadataManagerName = "capi-machineset-metadata"
)

// Update permissions on /finalizers subresrouce is required on management clusters with 'OwnerReferencesPermissionEnforcement' plugin enabled.
// See: https://kubernetes.io/docs/reference/access-authn-authz/admission-controllers/#ownerreferencespermissionenforcement
//
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io;bootstrap.cluster.x-k8s.io,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinesets;machinesets/status;machinesets/finalizers,verbs=get;list;watch;create;update;patch;delete

// Reconciler reconciles a MachineSet object.
type Reconciler struct {
	Client       client.Client
	APIReader    client.Reader
	ClusterCache clustercache.ClusterCache

	PreflightChecks sets.Set[clusterv1.MachineSetPreflightCheck]

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string

	ssaCache ssa.Cache
	recorder record.EventRecorder

	// Note: This field is only used for unit tests that use fake client because the fake client does not properly set resourceVersion
	//       on BootstrapConfig/InfraMachine after ssa.Patch and then ssa.RemoveManagedFieldsForLabelsAndAnnotations would fail.
	disableRemoveManagedFieldsForLabelsAndAnnotations bool

	// Those fields are only used for test purposes.
	overrideCreateMachines func(ctx context.Context, s *scope, machinesToAdd int) (ctrl.Result, error)
	overrideMoveMachines   func(ctx context.Context, s *scope, targetMSName string, machinesToMove int) (ctrl.Result, error)
	overrideDeleteMachines func(ctx context.Context, s *scope, machinesToDelete int) (ctrl.Result, error)
}

func (r *Reconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	if r.Client == nil || r.APIReader == nil || r.ClusterCache == nil {
		return errors.New("Client, APIReader and ClusterCache must not be nil")
	}

	predicateLog := ctrl.LoggerFrom(ctx).WithValues("controller", "machineset")
	clusterToMachineSets, err := util.ClusterToTypedObjectsMapper(mgr.GetClient(), &clusterv1.MachineSetList{}, mgr.GetScheme())
	if err != nil {
		return err
	}
	mdToMachineSets, err := util.MachineDeploymentToObjectsMapper(mgr.GetClient(), &clusterv1.MachineSetList{}, mgr.GetScheme())
	if err != nil {
		return err
	}

	err = ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.MachineSet{}).
		Owns(&clusterv1.Machine{}, builder.WithPredicates(predicates.ResourceIsChanged(mgr.GetScheme(), predicateLog))).
		// Watches enqueues MachineSet for corresponding Machine resources, if no managed controller reference (owner) exists.
		Watches(
			&clusterv1.Machine{},
			handler.EnqueueRequestsFromMapFunc(r.MachineToMachineSets),
			builder.WithPredicates(predicates.ResourceIsChanged(mgr.GetScheme(), predicateLog)),
		).
		Watches(
			&clusterv1.MachineDeployment{},
			handler.EnqueueRequestsFromMapFunc(mdToMachineSets),
			builder.WithPredicates(predicates.ResourceIsChanged(mgr.GetScheme(), predicateLog)),
		).
		WithOptions(options).
		WithEventFilter(predicates.ResourceHasFilterLabel(mgr.GetScheme(), predicateLog, r.WatchFilterValue)).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(clusterToMachineSets),
			builder.WithPredicates(
				// TODO: should this wait for Cluster.Status.InfrastructureReady similar to Infra Machine resources?
				predicates.All(mgr.GetScheme(), predicateLog,
					predicates.ResourceIsChanged(mgr.GetScheme(), predicateLog),
					predicates.ClusterPausedTransitions(mgr.GetScheme(), predicateLog),
					predicates.ResourceHasFilterLabel(mgr.GetScheme(), predicateLog, r.WatchFilterValue),
				),
			),
		).
		WatchesRawSource(r.ClusterCache.GetClusterSource("machineset", clusterToMachineSets)).
		Complete(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	r.recorder = mgr.GetEventRecorderFor("machineset-controller")
	r.ssaCache = ssa.NewCache("machineset")
	return nil
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (retres ctrl.Result, reterr error) {
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

	ctx = ctrl.LoggerInto(ctx, ctrl.LoggerFrom(ctx).WithValues("Cluster", klog.KRef(machineSet.Namespace, machineSet.Spec.ClusterName)))

	// Add finalizer first if not set to avoid the race condition between init and delete.
	if finalizerAdded, err := finalizers.EnsureFinalizer(ctx, r.Client, machineSet, clusterv1.MachineSetFinalizer); err != nil || finalizerAdded {
		return ctrl.Result{}, err
	}

	// AddOwners adds the owners of MachineSet as k/v pairs to the logger.
	// Specifically, it will add MachineDeployment.
	ctx, _, err := clog.AddOwners(ctx, r.Client, machineSet)
	if err != nil {
		return ctrl.Result{}, err
	}

	cluster, err := util.GetClusterByName(ctx, r.Client, machineSet.Namespace, machineSet.Spec.ClusterName)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Initialize the patch helper
	patchHelper, err := patch.NewHelper(machineSet, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	if isPaused, requeue, err := paused.EnsurePausedCondition(ctx, r.Client, cluster, machineSet); err != nil || isPaused || requeue {
		return ctrl.Result{}, err
	}

	s := &scope{
		cluster:            cluster,
		machineSet:         machineSet,
		reconciliationTime: time.Now(),
	}

	defer func() {
		if err := r.updateStatus(ctx, s); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, errors.Wrapf(err, "failed to update status")})
		}

		if err := r.reconcileV1Beta1Status(ctx, s); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, errors.Wrapf(err, "failed to update deprecated v1beta1 status")})
		}

		// Always attempt to patch the object and status after each reconciliation.
		patchOpts := []patch.Option{}
		if reterr == nil {
			patchOpts = append(patchOpts, patch.WithStatusObservedGeneration{})
		}
		if err := patchMachineSet(ctx, patchHelper, s.machineSet, patchOpts...); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}

		if reterr != nil {
			retres = ctrl.Result{}
			return
		}

		// Adjust requeue when scaling up
		if s.machineSet.DeletionTimestamp.IsZero() && reterr == nil {
			retres = util.LowestNonZeroResult(retres, shouldRequeueForReplicaCountersRefresh(s))
		}
	}()

	if isDeploymentChild(s.machineSet) {
		// If the MachineSet is in a MachineDeployment, try to get the owning MachineDeployment.
		s.owningMachineDeployment, err = r.getOwnerMachineDeployment(ctx, s.machineSet)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	alwaysReconcile := []machineSetReconcileFunc{
		wrapErrMachineSetReconcileFunc(r.reconcileMachineSetOwnerAndLabels, "failed to set MachineSet owner and labels"),
		wrapErrMachineSetReconcileFunc(r.reconcileInfrastructure, "failed to reconcile infrastructure"),
		wrapErrMachineSetReconcileFunc(r.reconcileBootstrapConfig, "failed to reconcile bootstrapConfig"),
		wrapErrMachineSetReconcileFunc(r.getAndAdoptMachinesForMachineSet, "failed to get and adopt Machines for MachineSet"),
	}

	// Handle deletion reconciliation loop.
	if !s.machineSet.DeletionTimestamp.IsZero() {
		reconcileDelete := append(
			alwaysReconcile,
			wrapErrMachineSetReconcileFunc(r.reconcileDelete, "failed to reconcile delete"),
		)
		return doReconcile(ctx, s, reconcileDelete)
	}

	reconcileNormal := append(alwaysReconcile,
		wrapErrMachineSetReconcileFunc(r.reconcileUnhealthyMachines, "failed to reconcile unhealthy machines"),
		wrapErrMachineSetReconcileFunc(r.syncMachines, "failed to sync Machines"),
		wrapErrMachineSetReconcileFunc(r.triggerInPlaceUpdate, "failed to trigger in-place update"),
		wrapErrMachineSetReconcileFunc(r.syncReplicas, "failed to sync replicas"),
	)

	return doReconcile(ctx, s, reconcileNormal)
}

type scope struct {
	machineSet                                *clusterv1.MachineSet
	cluster                                   *clusterv1.Cluster
	machines                                  []*clusterv1.Machine
	bootstrapObjectNotFound                   bool
	infrastructureObjectNotFound              bool
	getAndAdoptMachinesForMachineSetSucceeded bool
	owningMachineDeployment                   *clusterv1.MachineDeployment
	scaleUpPreflightCheckErrMessages          []string
	reconciliationTime                        time.Time
}

type machineSetReconcileFunc func(ctx context.Context, s *scope) (ctrl.Result, error)

func wrapErrMachineSetReconcileFunc(f machineSetReconcileFunc, msg string) machineSetReconcileFunc {
	return func(ctx context.Context, s *scope) (ctrl.Result, error) {
		res, err := f(ctx, s)
		return res, errors.Wrap(err, msg)
	}
}

func doReconcile(ctx context.Context, s *scope, phases []machineSetReconcileFunc) (ctrl.Result, kerrors.Aggregate) {
	res := ctrl.Result{}
	errs := []error{}
	for _, phase := range phases {
		// Call the inner reconciliation methods.
		phaseResult, err := phase(ctx, s)
		if err != nil {
			errs = append(errs, err)
		}
		if len(errs) > 0 {
			continue
		}
		res = util.LowestNonZeroResult(res, phaseResult)
	}

	if len(errs) > 0 {
		return ctrl.Result{}, kerrors.NewAggregate(errs)
	}

	return res, nil
}

func patchMachineSet(ctx context.Context, patchHelper *patch.Helper, machineSet *clusterv1.MachineSet, options ...patch.Option) error {
	// Always update the readyCondition by summarizing the state of other conditions.
	v1beta1conditions.SetSummary(machineSet,
		v1beta1conditions.WithConditions(
			clusterv1.MachinesCreatedV1Beta1Condition,
			clusterv1.ResizedV1Beta1Condition,
			clusterv1.MachinesReadyV1Beta1Condition,
		),
	)

	// Patch the object, ignoring conflicts on the conditions owned by this controller.
	options = append(options,
		patch.WithOwnedV1Beta1Conditions{Conditions: []clusterv1.ConditionType{
			clusterv1.ReadyV1Beta1Condition,
			clusterv1.MachinesCreatedV1Beta1Condition,
			clusterv1.ResizedV1Beta1Condition,
			clusterv1.MachinesReadyV1Beta1Condition,
		}},
		patch.WithOwnedConditions{Conditions: []string{
			clusterv1.PausedCondition,
			clusterv1.MachineSetScalingUpCondition,
			clusterv1.MachineSetScalingDownCondition,
			clusterv1.MachineSetMachinesReadyCondition,
			clusterv1.MachineSetMachinesUpToDateCondition,
			clusterv1.MachineSetRemediatingCondition,
			clusterv1.MachineSetDeletingCondition,
		}},
	)
	return patchHelper.Patch(ctx, machineSet, options...)
}

// triggerInPlaceUpdate func completes the move started when scaling down and then trigger the in-place update.
// Note: Usually it is the new MS that receives Machines after a move operation.
func (r *Reconciler) triggerInPlaceUpdate(ctx context.Context, s *scope) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	errs := []error{}
	machinesTriggeredInPlace := []*clusterv1.Machine{}
	for _, machine := range s.machines {
		// If a machine is not updating in place, or if the in place upgrade has been already triggered, no-op
		if _, ok := machine.Annotations[clusterv1.UpdateInProgressAnnotation]; !ok || hooks.IsPending(runtimehooksv1.UpdateMachine, machine) {
			continue
		}

		// If the existing machine is pending acknowledge from the MD controller after a move operation,
		// wait until if it possible to drop the PendingAcknowledgeMove annotation.
		orig := machine.DeepCopy()
		if _, ok := machine.Annotations[clusterv1.PendingAcknowledgeMoveAnnotation]; ok {
			// Check if this MachineSet is still accepting machines moved from other MachineSets.
			if sourceMSs, ok := s.machineSet.Annotations[clusterv1.MachineSetReceiveMachinesFromMachineSetsAnnotation]; ok && sourceMSs != "" {
				// Get the list of machines acknowledged by the MD controller.
				acknowledgedMoveReplicas := sets.Set[string]{}
				if replicaNames, ok := s.machineSet.Annotations[clusterv1.AcknowledgedMoveAnnotation]; ok && replicaNames != "" {
					acknowledgedMoveReplicas.Insert(strings.Split(replicaNames, ",")...)
				}

				// If the current machine is in not yet in the list, it is not possible to trigger in-place yet.
				if !acknowledgedMoveReplicas.Has(machine.Name) {
					continue
				}

				// If the current machine is in the list, drop the annotation.
				delete(machine.Annotations, clusterv1.PendingAcknowledgeMoveAnnotation)
			} else {
				// If this MachineSet is not accepting anymore machines from other MS (e.g. because of MD spec changes),
				// then drop the PendingAcknowledgeMove annotation; this machine will be treated as any other machine and either
				// deleted or moved to another MS after completing the in-place upgrade.
				delete(machine.Annotations, clusterv1.PendingAcknowledgeMoveAnnotation)
			}
		}

		// Complete the move operation started by the source MachinesSet by updating machine, infraMachine and boostrapConfig
		// to align to the desiredState for the current MachineSet.
		if err := r.completeMoveMachine(ctx, s, machine); err != nil {
			errs = append(errs, err)
			continue
		}

		// Note: Once we write PendingHooksAnnotation the Machine controller will start with the in-place update.
		hooks.MarkObjectAsPending(machine, runtimehooksv1.UpdateMachine)

		// Note: Intentionally using client.Patch instead of SSA. Otherwise we would
		//       have to ensure we preserve PendingHooksAnnotation on existing Machines in MachineSet and that would lead to race
		//       conditions when the Machine controller tries to remove the annotation and MachineSet adds it back.
		if err := r.Client.Patch(ctx, machine, client.MergeFrom(orig)); err != nil {
			errs = append(errs, errors.Wrapf(err, "failed to start in-place update for Machine %s", klog.KObj(machine)))
			continue
		}

		machinesTriggeredInPlace = append(machinesTriggeredInPlace, machine)
		log.Info("Completed triggering in-place update", "Machine", klog.KObj(machine))
		r.recorder.Event(machine, corev1.EventTypeNormal, "SuccessfulStartInPlaceUpdate", "Machine starting in-place update")
	}

	// Wait until the cache observed the Machine with PendingHooksAnnotation to ensure subsequent reconciles
	// will observe it as well and won't repeatedly call the logic to trigger in-place update.
	if err := clientutil.WaitForCacheToBeUpToDate(ctx, r.Client, "starting in-place updates", machinesTriggeredInPlace...); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return ctrl.Result{}, kerrors.NewAggregate(errs)
	}
	return ctrl.Result{}, nil
}

func (r *Reconciler) completeMoveMachine(ctx context.Context, s *scope, currentMachine *clusterv1.Machine) error {
	desiredMachine, err := r.computeDesiredMachine(s.machineSet, currentMachine)
	if err != nil {
		return errors.Wrap(err, "could not compute desired Machine")
	}
	// Note: spec.version and spec.failureDomain are not mutated in-place by syncMachines and accordingly
	//       not updated by r.computeDesiredMachine, so we have to update them here.
	// Note: for MachineSets we have to explicitly also set spec.failureDomain (this is a difference from what happens in KCP
	//       where the field is set only on create and never updated)
	desiredMachine.Spec.Version = currentMachine.Spec.Version
	desiredMachine.Spec.FailureDomain = currentMachine.Spec.FailureDomain

	// Compute desiredInfraMachine.
	currentInfraMachine, err := external.GetObjectFromContractVersionedRef(ctx, r.Client, currentMachine.Spec.InfrastructureRef, currentMachine.Namespace)
	if err != nil {
		return errors.Wrapf(err, "failed to get InfraMachine %s", klog.KRef(currentMachine.Namespace, currentMachine.Spec.InfrastructureRef.Name))
	}
	desiredInfraMachine, err := r.computeDesiredInfraMachine(ctx, s.machineSet, currentMachine, currentInfraMachine)
	if err != nil {
		return errors.Wrap(err, "could not compute desired InfraMachine")
	}

	// Make sure we drop the fields that should be continuously updated by syncMachines using the capi-machineset-metadata field owner
	// from the desiredInfraMachine (drop all labels and annotations except clonedFrom).
	desiredInfraMachine.SetLabels(nil)
	desiredInfraMachine.SetAnnotations(map[string]string{
		clusterv1.TemplateClonedFromNameAnnotation:      desiredInfraMachine.GetAnnotations()[clusterv1.TemplateClonedFromNameAnnotation],
		clusterv1.TemplateClonedFromGroupKindAnnotation: desiredInfraMachine.GetAnnotations()[clusterv1.TemplateClonedFromGroupKindAnnotation],
		// Machine controller waits for this annotation to exist on Machine and related objects before starting the in-place update.
		clusterv1.UpdateInProgressAnnotation: "",
	})

	var desiredBootstrapConfig, currentBootstrapConfig *unstructured.Unstructured
	if currentMachine.Spec.Bootstrap.ConfigRef.IsDefined() {
		// Compute desiredBootstrapConfig.
		currentBootstrapConfig, err = external.GetObjectFromContractVersionedRef(ctx, r.Client, currentMachine.Spec.Bootstrap.ConfigRef, currentMachine.Namespace)
		if err != nil {
			return errors.Wrapf(err, "failed to get BootstrapConfig %s", klog.KRef(currentMachine.Namespace, currentMachine.Spec.Bootstrap.ConfigRef.Name))
		}

		desiredBootstrapConfig, err = r.computeDesiredBootstrapConfig(ctx, s.machineSet, currentMachine, currentBootstrapConfig)
		if err != nil {
			return errors.Wrap(err, "could not compute desired BootstrapConfig")
		}

		// Make sure we drop the fields that should be continuously updated by syncMachines using the capi-machineset-metadata field owner
		// from the desiredBootstrapConfig (drop all labels and annotations except clonedFrom).
		desiredBootstrapConfig.SetLabels(nil)
		desiredBootstrapConfig.SetAnnotations(map[string]string{
			clusterv1.TemplateClonedFromNameAnnotation:      desiredBootstrapConfig.GetAnnotations()[clusterv1.TemplateClonedFromNameAnnotation],
			clusterv1.TemplateClonedFromGroupKindAnnotation: desiredBootstrapConfig.GetAnnotations()[clusterv1.TemplateClonedFromGroupKindAnnotation],
			// Machine controller waits for this annotation to exist on Machine and related objects before starting the in-place update.
			clusterv1.UpdateInProgressAnnotation: "",
		})
	}

	// Write InfraMachine.
	// Note: Let's update InfraMachine first because that is the call that is most likely to fail.
	if err := ssa.Patch(ctx, r.Client, machineSetManagerName, desiredInfraMachine); err != nil {
		return errors.Wrapf(err, "failed to update InfraMachine %s", klog.KObj(desiredInfraMachine))
	}

	// Write BootstrapConfig.
	if desiredMachine.Spec.Bootstrap.ConfigRef.IsDefined() {
		if err := ssa.Patch(ctx, r.Client, machineSetManagerName, desiredBootstrapConfig); err != nil {
			return errors.Wrapf(err, "failed to update BootstrapConfig %s", klog.KObj(desiredBootstrapConfig))
		}
	}

	// Write Machine.
	if err := ssa.Patch(ctx, r.Client, machineSetManagerName, desiredMachine); err != nil {
		return errors.Wrap(err, "failed to update Machine")
	}

	return nil
}

func (r *Reconciler) reconcileMachineSetOwnerAndLabels(_ context.Context, s *scope) (ctrl.Result, error) {
	machineSet := s.machineSet
	cluster := s.cluster
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

	// Make sure selector and template to be in the same cluster.
	if machineSet.Spec.Selector.MatchLabels == nil {
		machineSet.Spec.Selector.MatchLabels = make(map[string]string)
	}

	if machineSet.Spec.Template.Labels == nil {
		machineSet.Spec.Template.Labels = make(map[string]string)
	}

	machineSet.Spec.Selector.MatchLabels[clusterv1.ClusterNameLabel] = machineSet.Spec.ClusterName
	machineSet.Spec.Template.Labels[clusterv1.ClusterNameLabel] = machineSet.Spec.ClusterName

	return ctrl.Result{}, nil
}

func (r *Reconciler) reconcileInfrastructure(ctx context.Context, s *scope) (ctrl.Result, error) {
	cluster := s.cluster
	machineSet := s.machineSet
	// Make sure to reconcile the external infrastructure reference.
	var err error
	s.infrastructureObjectNotFound, err = r.reconcileExternalTemplateReference(ctx, cluster, machineSet, s.owningMachineDeployment, machineSet.Spec.Template.Spec.InfrastructureRef)
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *Reconciler) reconcileBootstrapConfig(ctx context.Context, s *scope) (ctrl.Result, error) {
	cluster := s.cluster
	machineSet := s.machineSet
	// Make sure to reconcile the external bootstrap reference, if any.
	if s.machineSet.Spec.Template.Spec.Bootstrap.ConfigRef.IsDefined() {
		var err error
		s.bootstrapObjectNotFound, err = r.reconcileExternalTemplateReference(ctx, cluster, machineSet, s.owningMachineDeployment, machineSet.Spec.Template.Spec.Bootstrap.ConfigRef)
		if err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *Reconciler) reconcileDelete(ctx context.Context, s *scope) (ctrl.Result, error) {
	machineSet := s.machineSet
	machineList := s.machines
	if !s.getAndAdoptMachinesForMachineSetSucceeded {
		return ctrl.Result{}, nil
	}

	log := ctrl.LoggerFrom(ctx)

	// If all the descendant machines are deleted, then remove the machineset's finalizer.
	if len(machineList) == 0 {
		controllerutil.RemoveFinalizer(machineSet, clusterv1.MachineSetFinalizer)
		return ctrl.Result{}, nil
	}

	// else delete owned machines.
	for _, machine := range machineList {
		if machine.DeletionTimestamp.IsZero() {
			if err := r.Client.Delete(ctx, machine); err != nil && !apierrors.IsNotFound(err) {
				return ctrl.Result{}, errors.Wrapf(err, "failed to delete Machine %s", klog.KObj(machine))
			}
			// Note: We intentionally log after Delete because we want this log line to show up only after DeletionTimestamp has been set.
			// Also, setting DeletionTimestamp doesn't mean the Machine is actually deleted (deletion takes some time).
			log.Info("Deleting Machine (MachineSet deleted)", "Machine", klog.KObj(machine))
		}
	}

	log.Info("Waiting for Machines to be deleted", "Machines", clog.ObjNamesString(machineList))
	return ctrl.Result{}, nil
}

func (r *Reconciler) getAndAdoptMachinesForMachineSet(ctx context.Context, s *scope) (ctrl.Result, error) {
	machineSet := s.machineSet
	log := ctrl.LoggerFrom(ctx)
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
		ctx := ctrl.LoggerInto(ctx, log)
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

	s.machines = filteredMachines
	s.getAndAdoptMachinesForMachineSetSucceeded = true

	return ctrl.Result{}, nil
}

// syncMachines updates Machines, InfrastructureMachine and BootstrapConfig to propagate in-place mutable fields
// from the MachineSet.
func (r *Reconciler) syncMachines(ctx context.Context, s *scope) (ctrl.Result, error) {
	machineSet := s.machineSet
	machines := s.machines
	if !s.getAndAdoptMachinesForMachineSetSucceeded {
		return ctrl.Result{}, nil
	}

	log := ctrl.LoggerFrom(ctx)
	for i := range machines {
		m := machines[i]

		// If the machine is already being deleted, we only need to sync
		// the subset of fields that impact tearing down a machine
		if !m.DeletionTimestamp.IsZero() {
			patchHelper, err := patch.NewHelper(m, r.Client)
			if err != nil {
				return ctrl.Result{}, err
			}

			// Set all other in-place mutable fields that impact the ability to tear down existing machines.
			m.Spec.ReadinessGates = machineSet.Spec.Template.Spec.ReadinessGates
			m.Spec.Deletion.NodeDrainTimeoutSeconds = machineSet.Spec.Template.Spec.Deletion.NodeDrainTimeoutSeconds
			m.Spec.Deletion.NodeDeletionTimeoutSeconds = machineSet.Spec.Template.Spec.Deletion.NodeDeletionTimeoutSeconds
			m.Spec.Deletion.NodeVolumeDetachTimeoutSeconds = machineSet.Spec.Template.Spec.Deletion.NodeVolumeDetachTimeoutSeconds
			m.Spec.MinReadySeconds = machineSet.Spec.Template.Spec.MinReadySeconds

			if err := patchHelper.Patch(ctx, m); err != nil {
				return ctrl.Result{}, err
			}
			continue
		}

		// Update Machine to propagate in-place mutable fields from the MachineSet.
		updatedMachine, err := r.computeDesiredMachine(machineSet, m)
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to update Machine: failed to compute desired Machine")
		}
		err = ssa.Patch(ctx, r.Client, machineSetManagerName, updatedMachine, ssa.WithCachingProxy{Cache: r.ssaCache, Original: m})
		if err != nil {
			log.Error(err, "Failed to update Machine", "Machine", klog.KObj(updatedMachine))
			return ctrl.Result{}, errors.Wrapf(err, "failed to update Machine %q", klog.KObj(updatedMachine))
		}
		machines[i] = updatedMachine

		infraMachine, err := external.GetObjectFromContractVersionedRef(ctx, r.Client, updatedMachine.Spec.InfrastructureRef, updatedMachine.Namespace)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to get InfrastructureMachine %s %s",
				updatedMachine.Spec.InfrastructureRef.Kind, klog.KRef(updatedMachine.Namespace, updatedMachine.Spec.InfrastructureRef.Name))
		}
		// Drop managedFields for manager:Update and capi-machineset:Apply for all objects created with CAPI <= v1.11.
		// Starting with CAPI v1.12 we have a new managedField structure where capi-machineset-metadata will own
		// labels and annotations and capi-machineset everything else.
		// Note: We have to call ssa.MigrateManagedFields for every Machine created with CAPI <= v1.11 once.
		//       Given that this was introduced in CAPI v1.12 and our n-3 upgrade policy this can
		//       be removed with CAPI v1.15.
		if err := ssa.MigrateManagedFields(ctx, r.Client, infraMachine, machineSetManagerName, machineSetMetadataManagerName); err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to clean up managedFields of InfrastructureMachine %s", klog.KObj(infraMachine))
		}
		// Update in-place mutating fields on InfrastructureMachine.
		if err := r.updateLabelsAndAnnotations(ctx, infraMachine, machineSet); err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to update InfrastructureMachine %s", klog.KObj(infraMachine))
		}

		if updatedMachine.Spec.Bootstrap.ConfigRef.IsDefined() {
			bootstrapConfig, err := external.GetObjectFromContractVersionedRef(ctx, r.Client, updatedMachine.Spec.Bootstrap.ConfigRef, updatedMachine.Namespace)
			if err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "failed to get BootstrapConfig %s %s",
					updatedMachine.Spec.Bootstrap.ConfigRef.Kind, klog.KRef(updatedMachine.Namespace, updatedMachine.Spec.Bootstrap.ConfigRef.Name))
			}
			// Drop managedFields for manager:Update and capi-machineset:Apply for all objects created with CAPI <= v1.11.
			// Starting with CAPI v1.12 we have a new managedField structure where capi-machineset-metadata will own
			// labels and annotations and capi-machineset everything else.
			// Note: We have to call ssa.MigrateManagedFields for every Machine created with CAPI <= v1.11 once.
			//       Given that this was introduced in CAPI v1.12 and our n-3 upgrade policy this can
			//       be removed with CAPI v1.15.
			if err := ssa.MigrateManagedFields(ctx, r.Client, bootstrapConfig, machineSetManagerName, machineSetMetadataManagerName); err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "failed to clean up managedFields of BootstrapConfig %s", klog.KObj(bootstrapConfig))
			}
			// Update in-place mutating fields on BootstrapConfig.
			if err := r.updateLabelsAndAnnotations(ctx, bootstrapConfig, machineSet); err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "failed to update BootstrapConfig %s", klog.KObj(bootstrapConfig))
			}
		}
	}
	return ctrl.Result{}, nil
}

// syncReplicas scales Machine resources up or down.
func (r *Reconciler) syncReplicas(ctx context.Context, s *scope) (ctrl.Result, error) {
	ms := s.machineSet
	machines := s.machines

	if !s.getAndAdoptMachinesForMachineSetSucceeded {
		return ctrl.Result{}, nil
	}

	log := ctrl.LoggerFrom(ctx)
	if ms.Spec.Replicas == nil {
		return ctrl.Result{}, errors.Errorf("the Replicas field in Spec for MachineSet %v is nil, this should not be allowed", ms.Name)
	}
	diff := len(machines) - int(ptr.Deref(ms.Spec.Replicas, 0))
	switch {
	case diff < 0:
		// If there are not enough Machines, create missing Machines unless Machine creation is disabled.
		machinesToAdd := -diff
		log.Info(fmt.Sprintf("MachineSet is scaling up to %d replicas by creating %d machines", *(ms.Spec.Replicas), machinesToAdd), "replicas", *(ms.Spec.Replicas), "machineCount", len(machines))
		if ms.Annotations != nil {
			if value, ok := ms.Annotations[clusterv1.DisableMachineCreateAnnotation]; ok && value == "true" {
				log.Info("Automatic creation of new machines disabled for machine set")
				return ctrl.Result{}, nil
			}
		}
		return r.createMachines(ctx, s, machinesToAdd)

	case diff > 0:
		// if too many replicas, delete or move exceeding machines.

		// If the MachineSet is accepting replicas from other MachineSets (and thus this is the newMS controlled by a MD),
		// detect if there are replicas still pending AcknowledgedMove.
		// Note: replicas still pending AcknowledgeMove should not be counted when computing the numbers of machines to delete, because those machines are not included in ms.Spec.Replicas yet.
		// Without this check, the following logic would try to align the number of replicas to "an incomplete" ms.Spec.Replicas and as a consequence wrongly delete replicas that should be preserved.
		notAcknowledgeMoveReplicas := sets.Set[string]{}
		if sourceMSs, ok := ms.Annotations[clusterv1.MachineSetReceiveMachinesFromMachineSetsAnnotation]; ok && sourceMSs != "" {
			for _, m := range machines {
				if _, ok := m.Annotations[clusterv1.PendingAcknowledgeMoveAnnotation]; !ok {
					continue
				}
				notAcknowledgeMoveReplicas.Insert(m.Name)
			}
		}
		if notAcknowledgeMoveReplicas.Len() > 0 {
			log.Info(fmt.Sprintf("Replicas %s moved from an old MachineSet still pending acknowledge from MachineDeployment", notAcknowledgeMoveReplicas.UnsortedList()))
		}

		machinesToDeleteOrMove := len(machines) - notAcknowledgeMoveReplicas.Len() - int(ptr.Deref(ms.Spec.Replicas, 0))
		if machinesToDeleteOrMove == 0 {
			return ctrl.Result{}, nil
		}

		// Move machines to the target MachineSet if the current MachineSet is instructed to do so.
		if targetMSName, ok := ms.Annotations[clusterv1.MachineSetMoveMachinesToMachineSetAnnotation]; ok && targetMSName != "" {
			// Note: The number of machines actually moved could be less than expected e.g. because some machine still updating in-place from a previous move.
			return r.startMoveMachines(ctx, s, targetMSName, machinesToDeleteOrMove)
		}

		// Otherwise the current MachineSet is not instructed to move machines to another MachineSet,
		// then delete all the exceeding machines.
		return r.deleteMachines(ctx, s, machinesToDeleteOrMove)
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) createMachines(ctx context.Context, s *scope, machinesToAdd int) (ctrl.Result, error) {
	if r.overrideCreateMachines != nil {
		return r.overrideCreateMachines(ctx, s, machinesToAdd)
	}

	log := ctrl.LoggerFrom(ctx)
	ms := s.machineSet
	cluster := s.cluster

	preflightCheckErrMessages, err := r.runPreflightChecks(ctx, cluster, ms, "Scale up")
	if err != nil || len(preflightCheckErrMessages) > 0 {
		if err != nil {
			// If err is not nil use that as the preflightCheckErrMessage
			preflightCheckErrMessages = append(preflightCheckErrMessages, err.Error())
		}

		s.scaleUpPreflightCheckErrMessages = preflightCheckErrMessages
		v1beta1conditions.MarkFalse(ms, clusterv1.MachinesCreatedV1Beta1Condition, clusterv1.PreflightCheckFailedV1Beta1Reason, clusterv1.ConditionSeverityError, "%s", strings.Join(preflightCheckErrMessages, "; "))
		if err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: preflightFailedRequeueAfter}, nil
	}

	machinesAdded := []*clusterv1.Machine{}
	for i := range machinesToAdd {
		// Create a new logger so the global logger is not modified.
		log := log
		machine, computeMachineErr := r.computeDesiredMachine(ms, nil)
		if computeMachineErr != nil {
			v1beta1conditions.MarkFalse(ms, clusterv1.MachinesCreatedV1Beta1Condition, clusterv1.MachineCreationFailedV1Beta1Reason,
				clusterv1.ConditionSeverityError, "%s", computeMachineErr.Error())
			return ctrl.Result{}, errors.Wrap(computeMachineErr, "failed to create Machine: failed to compute desired Machine")
		}

		var (
			infraRef, bootstrapRef        clusterv1.ContractVersionedObjectReference
			infraMachine, bootstrapConfig *unstructured.Unstructured
		)

		// Create the BootstrapConfig if necessary.
		if ms.Spec.Template.Spec.Bootstrap.ConfigRef.IsDefined() {
			bootstrapConfig, bootstrapRef, err = r.createBootstrapConfig(ctx, ms, machine)
			if err != nil {
				v1beta1conditions.MarkFalse(ms, clusterv1.MachinesCreatedV1Beta1Condition, clusterv1.BootstrapTemplateCloningFailedV1Beta1Reason, clusterv1.ConditionSeverityError, "%s", err.Error())
				return ctrl.Result{}, errors.Wrapf(err, "failed to clone bootstrap configuration from %s %s while creating a Machine",
					ms.Spec.Template.Spec.Bootstrap.ConfigRef.Kind,
					klog.KRef(ms.Namespace, ms.Spec.Template.Spec.Bootstrap.ConfigRef.Name))
			}
			machine.Spec.Bootstrap.ConfigRef = bootstrapRef
			log = log.WithValues(bootstrapRef.Kind, klog.KRef(ms.Namespace, bootstrapRef.Name))
		}

		// Create the InfraMachine.
		infraMachine, infraRef, err = r.createInfraMachine(ctx, ms, machine)
		if err != nil {
			var deleteErr error
			if bootstrapRef.IsDefined() {
				// Cleanup the bootstrap resource if we can't create the InfraMachine; or we might risk to leak it.
				if err := r.Client.Delete(ctx, bootstrapConfig); err != nil && !apierrors.IsNotFound(err) {
					deleteErr = errors.Wrapf(err, "failed to cleanup %s %s after %s creation failed", bootstrapRef.Kind, klog.KRef(ms.Namespace, bootstrapRef.Name), ms.Spec.Template.Spec.InfrastructureRef.Kind)
				}
			}

			v1beta1conditions.MarkFalse(ms, clusterv1.MachinesCreatedV1Beta1Condition, clusterv1.InfrastructureTemplateCloningFailedV1Beta1Reason, clusterv1.ConditionSeverityError, "%s", err.Error())
			return ctrl.Result{}, kerrors.NewAggregate([]error{errors.Wrapf(err, "failed to clone infrastructure machine from %s %s while creating a Machine",
				ms.Spec.Template.Spec.InfrastructureRef.Kind,
				klog.KRef(ms.Namespace, ms.Spec.Template.Spec.InfrastructureRef.Name)), deleteErr})
		}
		log = log.WithValues(infraRef.Kind, klog.KRef(ms.Namespace, infraRef.Name))
		machine.Spec.InfrastructureRef = infraRef

		// Create the Machine.
		if err := ssa.Patch(ctx, r.Client, machineSetManagerName, machine); err != nil {
			// Try to cleanup the external objects if the Machine creation failed.
			errs := []error{err}
			if err := r.Client.Delete(ctx, infraMachine); !apierrors.IsNotFound(err) {
				errs = append(errs, errors.Wrapf(err, "failed to cleanup %s %s after Machine creation failed", infraRef.Kind, klog.KRef(ms.Namespace, infraRef.Name)))
			}
			if bootstrapRef.IsDefined() {
				if err := r.Client.Delete(ctx, bootstrapConfig); !apierrors.IsNotFound(err) {
					errs = append(errs, errors.Wrapf(err, "failed to cleanup %s %s after Machine creation failed", bootstrapRef.Kind, klog.KRef(ms.Namespace, bootstrapRef.Name)))
				}
			}

			v1beta1conditions.MarkFalse(ms, clusterv1.MachinesCreatedV1Beta1Condition, clusterv1.MachineCreationFailedV1Beta1Reason,
				clusterv1.ConditionSeverityError, "%s", err.Error())
			return ctrl.Result{}, kerrors.NewAggregate(errs)
		}

		machinesAdded = append(machinesAdded, machine)
		log.Info(fmt.Sprintf("Machine created (scale up, creating %d of %d)", i+1, machinesToAdd), "Machine", klog.KObj(machine))
		r.recorder.Eventf(ms, corev1.EventTypeNormal, "SuccessfulCreate", "Created Machine %q", machine.Name)
	}

	// Wait for cache update to ensure following reconcile gets latest change.
	return ctrl.Result{}, clientutil.WaitForCacheToBeUpToDate(ctx, r.Client, "Machine creation", machinesAdded...)
}

func (r *Reconciler) deleteMachines(ctx context.Context, s *scope, machinesToDelete int) (ctrl.Result, error) {
	if r.overrideDeleteMachines != nil {
		return r.overrideDeleteMachines(ctx, s, machinesToDelete)
	}

	log := ctrl.LoggerFrom(ctx)
	ms := s.machineSet
	machines := s.machines

	log.Info(fmt.Sprintf("MachineSet is scaling down to %d replicas by deleting %d Machines", *(ms.Spec.Replicas), machinesToDelete), "replicas", *(ms.Spec.Replicas), "machineCount", len(machines), "order", cmp.Or(ms.Spec.Deletion.Order, clusterv1.RandomMachineSetDeletionOrder))

	// Pick the list of machines to be deleted according to the criteria defined in ms.Spec.Deletion.Order.
	// Note: In case the system has to delete some machine, and there are machines are still updating in place, those machine must be deleted first.
	//
	// This prevents the system to perform unnecessary in-place updates. e.g.
	// - In place rollout of MD with 3 Replicas, maxSurge 1, MaxUnavailable 0
	//   - First create m4 to create a buffer for doing in place
	//   - Move old machines (m1, m2, m3)
	// - Resulting new MS at this point has 4 replicas m1, m2, m3 (updating in place) and (m4).
	// - The system scales down MS, and the system does this getting rid of m3 - the last replica that started in place.
	deletePriorityFunc, err := getDeletePriorityFunc(ms)
	if err != nil {
		return ctrl.Result{}, err
	}
	machinesToDeleteByPriority := getMachinesToDeletePrioritized(machines, machinesToDelete, deletePriorityFunc)

	var errs []error
	machinesDeleted := []*clusterv1.Machine{}
	for i, machine := range machinesToDeleteByPriority {
		log := log.WithValues("Machine", klog.KObj(machine))
		if machine.GetDeletionTimestamp().IsZero() {
			if err := r.Client.Delete(ctx, machine); err != nil {
				errs = append(errs, err)
				continue
			}

			machinesDeleted = append(machinesDeleted, machine)
			log.Info(fmt.Sprintf("Deleting Machine (scale down, deleting %d of %d)", i+1, machinesToDelete))
			r.recorder.Eventf(ms, corev1.EventTypeNormal, "SuccessfulDelete", "Deleted Machine %q", machine.Name)
		} else {
			log.Info(fmt.Sprintf("Waiting for Machine to be deleted (scale down, deleting %d of %d)", i+1, machinesToDelete))
		}
	}

	// Wait for cache update to ensure following reconcile gets latest change.
	if err := clientutil.WaitForObjectsToBeDeletedFromTheCache(ctx, r.Client, "Machine deletion", machinesDeleted...); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return ctrl.Result{}, kerrors.NewAggregate(errs)
	}
	return ctrl.Result{}, nil
}

func (r *Reconciler) startMoveMachines(ctx context.Context, s *scope, targetMSName string, machinesToMove int) (ctrl.Result, error) {
	if r.overrideMoveMachines != nil {
		return r.overrideMoveMachines(ctx, s, targetMSName, machinesToMove)
	}

	log := ctrl.LoggerFrom(ctx)
	ms := s.machineSet
	machines := s.machines

	// Check that everything is set for the move operation by validating that the target MS is expecting to
	// receive replicas from the current one.
	targetMS := &clusterv1.MachineSet{}
	if err := r.Client.Get(ctx, client.ObjectKey{Name: targetMSName, Namespace: ms.Namespace}, targetMS); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to get MachineSet %s, which is the target for the move operation", targetMSName)
	}

	validSourceMSs := targetMS.Annotations[clusterv1.MachineSetReceiveMachinesFromMachineSetsAnnotation]
	sourcesSet := sets.Set[string]{}
	sourcesSet.Insert(strings.Split(validSourceMSs, ",")...)
	if !sourcesSet.Has(ms.Name) {
		return ctrl.Result{}, errors.Errorf("MachineSet %s is set to move replicas to %s, but %[2]s only accepts Machines from %s", ms.Name, targetMS.Name, validSourceMSs)
	}

	log.Info(fmt.Sprintf("MachineSet is scaling down to %d replicas by moving %d Machines to %s", *(ms.Spec.Replicas), machinesToMove, targetMSName), "replicas", *(ms.Spec.Replicas), "machineCount", len(machines), "order", cmp.Or(ms.Spec.Deletion.Order, clusterv1.RandomMachineSetDeletionOrder))

	// Sort to Move machine in deterministic and predictable order.
	// Note: For convenience we sort machine using the ordering criteria defined in ms.Spec.Deletion.Order.
	deletePriorityFunc, err := getDeletePriorityFunc(ms)
	if err != nil {
		return ctrl.Result{}, err
	}
	machinesToMoveByPriority := getMachinesToMovePrioritized(machines, deletePriorityFunc)

	errs := []error{}
	machinesMoved := []*clusterv1.Machine{}
	for _, machine := range machinesToMoveByPriority {
		if machinesToMove <= 0 {
			break
		}

		log := log.WithValues("Machine", klog.KObj(machine))

		// Make sure we are not moving machines still updating in place from a previous move (this includes also machines still pending AcknowledgeMove).
		if inplace.IsUpdateInProgress(machine) {
			continue
		}

		// Note. Machines with the DeleteMachineAnnotation are going to be moved and the new MS
		//       will take care of fulfilling this intent as soon as it scales down.
		// Note. Also Machines marked as unhealthy by MHC are going to be moved, because otherwise
		//       remediation will not complete as the Machine is owned by an old MS.

		machinesToMove--

		// If there are machines already deleting, wait for them to go away before further scaling down with move.
		if !machine.DeletionTimestamp.IsZero() {
			continue
		}

		// Perform the first part of a move operation, the part under the responsibility of the oldMS.
		orig := machine.DeepCopy()

		// Change the ownerReference from current MS to target MS
		// Note: After move, when the first reconcile of the target MS will happen, there will be co-ownership on
		//       this ownerReference between "manager" and "capi-machineset", but this is not an issue because the MS controller will never unset this field.
		machine.OwnerReferences = util.ReplaceOwnerRef(machine.OwnerReferences, ms, metav1.OwnerReference{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "MachineSet",
			Name:       targetMS.Name,
			UID:        targetMS.UID,
			Controller: ptr.To(true),
		})

		// Change machine's labels from current MS to target MS
		// Note: The implementation assumes that current and target MS are originated from the same MachineDeployment,
		//       and thus this change can be performed in a surgical way by patching only the MachineDeploymentUniqueLabel.
		//       Notably, minimizing label changes, simplifies impacts and considerations about managedFields and co-ownership.
		// Note: After move, when the first reconcile of the target MS will happen, there will be co-ownership on
		//       this label between "manager" and "capi-machineset", but this is not an issue because the MS controller will never unset this label.
		targetUniqueLabel, ok := targetMS.Labels[clusterv1.MachineDeploymentUniqueLabel]
		if !ok {
			return ctrl.Result{}, errors.Errorf("MachineSet %s does not have the %s label", targetMS.Name, clusterv1.MachineDeploymentUniqueLabel)
		}
		machine.Labels[clusterv1.MachineDeploymentUniqueLabel] = targetUniqueLabel

		// Apply the UpdateInProgress and PendingAcknowledgeMove annotation
		if machine.Annotations == nil {
			machine.Annotations = map[string]string{}
		}
		machine.Annotations[clusterv1.UpdateInProgressAnnotation] = ""
		machine.Annotations[clusterv1.PendingAcknowledgeMoveAnnotation] = ""

		// Note: Intentionally using client.Patch instead of SSA. Otherwise we would have to ensure we preserve
		//       UpdateInProgressAnnotation on existing Machines and that would lead to race conditions when
		//       the Machine controller tries to remove the annotation and then the MachineSet controller adds it back.
		if err := r.Client.Patch(ctx, machine, client.MergeFrom(orig)); err != nil {
			log.Error(err, "Failed to start moving Machine")
			errs = append(errs, errors.Wrapf(err, "failed to start moving Machine %s", klog.KObj(machine)))
			continue
		}
		machinesMoved = append(machinesMoved, machine)
	}

	// Wait for cache update to ensure following reconcile gets latest change.
	if err := clientutil.WaitForCacheToBeUpToDate(ctx, r.Client, "moving Machines", machinesMoved...); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return ctrl.Result{}, kerrors.NewAggregate(errs)
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
func (r *Reconciler) computeDesiredMachine(machineSet *clusterv1.MachineSet, existingMachine *clusterv1.Machine) (*clusterv1.Machine, error) {
	nameTemplate := "{{ .machineSet.name }}-{{ .random }}"
	if machineSet.Spec.MachineNaming.Template != "" {
		nameTemplate = machineSet.Spec.MachineNaming.Template
		// This should never happen as this is validated on admission.
		if !strings.Contains(nameTemplate, "{{ .random }}") {
			return nil, errors.New("cannot generate Machine name: {{ .random }} is missing in machineNaming.template")
		}
	}

	generatedMachineName, err := topologynames.MachineSetMachineNameGenerator(nameTemplate, machineSet.Spec.ClusterName, machineSet.Name).GenerateName()
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate Machine name")
	}

	desiredMachine := &clusterv1.Machine{
		TypeMeta: metav1.TypeMeta{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      generatedMachineName,
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
	desiredMachine.Spec.InfrastructureRef = clusterv1.ContractVersionedObjectReference{}
	desiredMachine.Spec.Bootstrap.ConfigRef = clusterv1.ContractVersionedObjectReference{}

	// If we are updating an existing Machine reuse the name, uid, infrastructureRef and bootstrap.configRef
	// from the existingMachine.
	// Note: we use UID to force SSA to update the existing Machine and to not accidentally create a new Machine.
	// infrastructureRef and bootstrap.configRef remain the same for an existing Machine.
	if existingMachine != nil {
		desiredMachine.SetName(existingMachine.Name)
		desiredMachine.SetUID(existingMachine.UID)

		// Preserve all existing finalizers (including foregroundDeletion finalizer).
		finalizers := existingMachine.Finalizers
		// Ensure MachineFinalizer is set.
		if !sets.New[string](finalizers...).Has(clusterv1.MachineFinalizer) {
			finalizers = append(finalizers, clusterv1.MachineFinalizer)
		}
		desiredMachine.Finalizers = finalizers

		// Make sure sync machines do not change the fields that are not in-place mutable
		desiredMachine.Spec.Bootstrap.ConfigRef = existingMachine.Spec.Bootstrap.ConfigRef
		desiredMachine.Spec.InfrastructureRef = existingMachine.Spec.InfrastructureRef
		desiredMachine.Spec.Version = existingMachine.Spec.Version
		desiredMachine.Spec.FailureDomain = existingMachine.Spec.FailureDomain
	}
	// Set the in-place mutable fields.
	// When we create a new Machine we will just create the Machine with those fields.
	// When we update an existing Machine will we update the fields on the existing Machine (in-place mutate).

	// Set Labels
	desiredMachine.Labels = machineLabelsFromMachineSet(machineSet)

	// Set Annotations
	desiredMachine.Annotations = machineAnnotationsFromMachineSet(machineSet)

	// Set all other in-place mutable fields.
	desiredMachine.Spec.ReadinessGates = machineSet.Spec.Template.Spec.ReadinessGates
	desiredMachine.Spec.Deletion.NodeDrainTimeoutSeconds = machineSet.Spec.Template.Spec.Deletion.NodeDrainTimeoutSeconds
	desiredMachine.Spec.Deletion.NodeDeletionTimeoutSeconds = machineSet.Spec.Template.Spec.Deletion.NodeDeletionTimeoutSeconds
	desiredMachine.Spec.Deletion.NodeVolumeDetachTimeoutSeconds = machineSet.Spec.Template.Spec.Deletion.NodeVolumeDetachTimeoutSeconds
	desiredMachine.Spec.MinReadySeconds = machineSet.Spec.Template.Spec.MinReadySeconds

	return desiredMachine, nil
}

// updateLabelsAndAnnotations updates the external object passed in with the
// updated labels and annotations from the MachineSet.
func (r *Reconciler) updateLabelsAndAnnotations(ctx context.Context, obj client.Object, machineSet *clusterv1.MachineSet) error {
	updatedObject := &unstructured.Unstructured{}
	updatedObject.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
	updatedObject.SetNamespace(obj.GetNamespace())
	updatedObject.SetName(obj.GetName())
	// Set the UID to ensure that Server-Side-Apply only performs an update
	// and does not perform an accidental create.
	updatedObject.SetUID(obj.GetUID())

	updatedObject.SetLabels(machineLabelsFromMachineSet(machineSet))
	updatedObject.SetAnnotations(machineAnnotationsFromMachineSet(machineSet))

	return ssa.Patch(ctx, r.Client, machineSetMetadataManagerName, updatedObject, ssa.WithCachingProxy{Cache: r.ssaCache, Original: obj})
}

func (r *Reconciler) getOwnerMachineDeployment(ctx context.Context, machineSet *clusterv1.MachineSet) (*clusterv1.MachineDeployment, error) {
	mdName := machineSet.Labels[clusterv1.MachineDeploymentNameLabel]
	if mdName == "" {
		return nil, fmt.Errorf("no owner MachineDeployment found for MachineSet %s", klog.KObj(machineSet))
	}

	md := &clusterv1.MachineDeployment{}
	if err := r.Client.Get(ctx, client.ObjectKey{Namespace: machineSet.Namespace, Name: mdName}, md); err != nil {
		return nil, fmt.Errorf("failed to retrieve owner MachineDeployment for MachineSet %s: %w", klog.KObj(machineSet), err)
	}
	return md, nil
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
	for _, ref := range m.GetOwnerReferences() {
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

// isDeploymentChild returns true if the MachineSet originated from a MachineDeployment by checking its labels.
func isDeploymentChild(ms *clusterv1.MachineSet) bool {
	_, ok := ms.Labels[clusterv1.MachineDeploymentNameLabel]
	return ok
}

// isCurrentMachineSet returns true if the MachineSet's and MachineDeployments revision are equal.
func isCurrentMachineSet(ms *clusterv1.MachineSet, md *clusterv1.MachineDeployment) bool {
	if md == nil {
		return false
	}
	return md.Annotations[clusterv1.RevisionAnnotation] == ms.Annotations[clusterv1.RevisionAnnotation]
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
	return !isDeploymentChild(ms)
}

// reconcileV1Beta1Status updates the Status field for the MachineSet
// It checks for the current state of the replicas and updates the Status of the MachineSet.
func (r *Reconciler) reconcileV1Beta1Status(ctx context.Context, s *scope) error {
	if !s.getAndAdoptMachinesForMachineSetSucceeded {
		return nil
	}

	ms := s.machineSet
	filteredMachines := s.machines
	cluster := s.cluster

	if ms.Spec.Replicas == nil {
		return errors.New("Cannot update status when MachineSet spec.replicas is not set")
	}

	log := ctrl.LoggerFrom(ctx)

	// Count the number of machines that have labels matching the labels of the machine
	// template of the replica set, the matching machines may have more
	// labels than are in the template. Because the label of machineTemplateSpec is
	// a superset of the selector of the replica set, so the possible
	// matching machines must be part of the filteredMachines.
	fullyLabeledReplicasCount := 0
	readyReplicasCount := 0
	availableReplicasCount := 0
	desiredReplicas := *ms.Spec.Replicas
	if !ms.DeletionTimestamp.IsZero() {
		desiredReplicas = 0
	}
	currentReplicas := ptr.Deref(ms.Status.Replicas, 0)
	templateLabel := labels.Set(ms.Spec.Template.Labels).AsSelectorPreValidated()

	for _, machine := range filteredMachines {
		log := log.WithValues("Machine", klog.KObj(machine))

		if templateLabel.Matches(labels.Set(machine.Labels)) {
			fullyLabeledReplicasCount++
		}

		if !machine.Status.NodeRef.IsDefined() {
			log.V(4).Info("Waiting for the machine controller to set status.NodeRef on the Machine")
			continue
		}

		node, err := r.getMachineNode(ctx, cluster, machine)
		if err != nil && machine.GetDeletionTimestamp().IsZero() {
			log.Error(err, "Unable to retrieve Node status", "Node", klog.KObj(node))
			continue
		}

		if noderefutil.IsNodeReady(node) {
			readyReplicasCount++
			if noderefutil.IsNodeAvailable(node, ptr.Deref(ms.Spec.Template.Spec.MinReadySeconds, 0), metav1.Now()) {
				availableReplicasCount++
			}
		} else if machine.GetDeletionTimestamp().IsZero() {
			log.V(4).Info("Waiting for the Kubernetes node on the machine to report ready state")
		}
	}

	if ms.Status.Deprecated == nil {
		ms.Status.Deprecated = &clusterv1.MachineSetDeprecatedStatus{}
	}
	if ms.Status.Deprecated.V1Beta1 == nil {
		ms.Status.Deprecated.V1Beta1 = &clusterv1.MachineSetV1Beta1DeprecatedStatus{}
	}
	ms.Status.Deprecated.V1Beta1.FullyLabeledReplicas = int32(fullyLabeledReplicasCount)
	ms.Status.Deprecated.V1Beta1.ReadyReplicas = int32(readyReplicasCount)
	ms.Status.Deprecated.V1Beta1.AvailableReplicas = int32(availableReplicasCount)

	switch {
	// We are scaling up
	case currentReplicas < desiredReplicas:
		v1beta1conditions.MarkFalse(ms, clusterv1.ResizedV1Beta1Condition, clusterv1.ScalingUpV1Beta1Reason, clusterv1.ConditionSeverityWarning, "Scaling up MachineSet to %d replicas (actual %d)", desiredReplicas, currentReplicas)
	// We are scaling down
	case currentReplicas > desiredReplicas:
		v1beta1conditions.MarkFalse(ms, clusterv1.ResizedV1Beta1Condition, clusterv1.ScalingDownV1Beta1Reason, clusterv1.ConditionSeverityWarning, "Scaling down MachineSet to %d replicas (actual %d)", desiredReplicas, currentReplicas)
		// This means that there was no error in generating the desired number of machine objects
		v1beta1conditions.MarkTrue(ms, clusterv1.MachinesCreatedV1Beta1Condition)
	default:
		// Make sure last resize operation is marked as completed.
		// NOTE: we are checking the number of machines ready so we report resize completed only when the machines
		// are actually provisioned (vs reporting completed immediately after the last machine object is created). This convention is also used by KCP.
		if ms.Status.Deprecated.V1Beta1.ReadyReplicas == currentReplicas {
			if v1beta1conditions.IsFalse(ms, clusterv1.ResizedV1Beta1Condition) {
				log.Info("All the replicas are ready", "replicas", ms.Status.Deprecated.V1Beta1.ReadyReplicas)
			}
			v1beta1conditions.MarkTrue(ms, clusterv1.ResizedV1Beta1Condition)
		}
		// This means that there was no error in generating the desired number of machine objects
		v1beta1conditions.MarkTrue(ms, clusterv1.MachinesCreatedV1Beta1Condition)
	}

	// Aggregate the operational state of all the machines; while aggregating we are adding the
	// source ref (reason@machine/name) so the problem can be easily tracked down to its source machine.
	if len(filteredMachines) > 0 {
		v1beta1conditions.SetAggregate(ms, clusterv1.MachinesReadyV1Beta1Condition, collections.FromMachines(filteredMachines...).ConditionGetters(), v1beta1conditions.AddSourceRef())
	} else {
		v1beta1conditions.MarkTrue(ms, clusterv1.MachinesReadyV1Beta1Condition)
	}

	return nil
}

func shouldRequeueForReplicaCountersRefresh(s *scope) ctrl.Result {
	minReadySeconds := ptr.Deref(s.machineSet.Spec.Template.Spec.MinReadySeconds, 0)
	replicas := ptr.Deref(s.machineSet.Spec.Replicas, 0)

	// Resync the MachineSet after minReadySeconds as a last line of defense to guard against clock-skew.
	// Clock-skew is an issue as it may impact whether an available replica is counted as a ready replica.
	// A replica is available if the amount of time since last transition exceeds minReadySeconds.
	// If there was a clock skew, checking whether the amount of time since last transition to ready state
	// exceeds minReadySeconds could be incorrect.
	// To avoid an available replica stuck in the ready state, we force a reconcile after minReadySeconds,
	// at which point it should confirm any available replica to be available.
	if minReadySeconds > 0 &&
		ptr.Deref(s.machineSet.Status.ReadyReplicas, 0) == replicas &&
		ptr.Deref(s.machineSet.Status.AvailableReplicas, 0) != replicas {
		minReadyResult := ctrl.Result{RequeueAfter: time.Duration(minReadySeconds) * time.Second}
		return minReadyResult
	}

	// Quickly reconcile until the nodes become Ready.
	if ptr.Deref(s.machineSet.Status.ReadyReplicas, 0) != replicas {
		return ctrl.Result{RequeueAfter: 15 * time.Second}
	}

	return ctrl.Result{}
}

func (r *Reconciler) getMachineNode(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) (*corev1.Node, error) {
	remoteClient, err := r.ClusterCache.GetClient(ctx, util.ObjectKey(cluster))
	if err != nil {
		return nil, err
	}
	node := &corev1.Node{}
	if err := remoteClient.Get(ctx, client.ObjectKey{Name: machine.Status.NodeRef.Name}, node); err != nil {
		return nil, errors.Wrapf(err, "error retrieving node %s for machine %s/%s", machine.Status.NodeRef.Name, machine.Namespace, machine.Name)
	}
	return node, nil
}

func (r *Reconciler) reconcileUnhealthyMachines(ctx context.Context, s *scope) (ctrl.Result, error) {
	if !s.getAndAdoptMachinesForMachineSetSucceeded {
		return ctrl.Result{}, nil
	}

	cluster := s.cluster
	ms := s.machineSet
	machines := s.machines
	owner := s.owningMachineDeployment
	log := ctrl.LoggerFrom(ctx)

	// Remove OwnerRemediated condition from Machines that have HealthCheckSucceeded condition true
	// and OwnerRemediated condition false
	errList := []error{}
	for _, m := range machines {
		if !m.DeletionTimestamp.IsZero() {
			continue
		}

		shouldCleanupV1Beta1 := v1beta1conditions.IsTrue(m, clusterv1.MachineHealthCheckSucceededV1Beta1Condition) && v1beta1conditions.IsFalse(m, clusterv1.MachineOwnerRemediatedV1Beta1Condition)
		shouldCleanup := conditions.IsTrue(m, clusterv1.MachineHealthCheckSucceededCondition) && conditions.IsFalse(m, clusterv1.MachineOwnerRemediatedCondition)

		if !shouldCleanupV1Beta1 && !shouldCleanup {
			continue
		}

		patchHelper, err := patch.NewHelper(m, r.Client)
		if err != nil {
			errList = append(errList, err)
			continue
		}

		if shouldCleanupV1Beta1 {
			v1beta1conditions.Delete(m, clusterv1.MachineOwnerRemediatedV1Beta1Condition)
		}

		if shouldCleanup {
			conditions.Delete(m, clusterv1.MachineOwnerRemediatedCondition)
		}

		if err := patchHelper.Patch(ctx, m, patch.WithOwnedV1Beta1Conditions{Conditions: []clusterv1.ConditionType{
			clusterv1.MachineOwnerRemediatedV1Beta1Condition,
		}}, patch.WithOwnedConditions{Conditions: []string{
			clusterv1.MachineOwnerRemediatedCondition,
		}}); err != nil {
			errList = append(errList, err)
		}
	}
	if len(errList) > 0 {
		return ctrl.Result{}, errors.Wrapf(kerrors.NewAggregate(errList), "failed to remove OwnerRemediated condition from healhty Machines")
	}

	// Calculates the Machines to be remediated.
	// Note: Machines already deleting are not included, there is no need to trigger remediation for them again.
	machinesToRemediate := collections.FromMachines(machines...).Filter(collections.IsUnhealthyAndOwnerRemediated, collections.Not(collections.HasDeletionTimestamp)).UnsortedList()

	// If there are no machines to remediate return early.
	if len(machinesToRemediate) == 0 {
		return ctrl.Result{}, nil
	}

	// Calculate how many in flight machines we should remediate.
	// By default, we allow all machines to be remediated at the same time.
	maxInFlight := math.MaxInt

	// If the MachineSet is part of a MachineDeployment, only allow remediations if
	// it's the desired revision.
	if isDeploymentChild(ms) {
		if owner.Annotations[clusterv1.RevisionAnnotation] != ms.Annotations[clusterv1.RevisionAnnotation] {
			// MachineSet is part of a MachineDeployment but isn't the current revision, no remediations allowed.
			if err := patchMachineConditions(ctx, r.Client, machinesToRemediate, metav1.Condition{
				Type:    clusterv1.MachineOwnerRemediatedCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineSetMachineCannotBeRemediatedReason,
				Message: "Machine won't be remediated because it is pending removal due to rollout",
			}, nil); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}

		if owner.Spec.Remediation.MaxInFlight != nil {
			var err error
			replicas := int(ptr.Deref(owner.Spec.Replicas, 1))
			maxInFlight, err = intstr.GetScaledValueFromIntOrPercent(owner.Spec.Remediation.MaxInFlight, replicas, true)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to calculate maxInFlight to remediate machines: %v", err)
			}
			log = log.WithValues("maxInFlight", maxInFlight, "replicas", replicas)
		}
	}

	// Update maxInFlight based on remediations that are in flight.
	// A Machine has a remediation in flight when Machine's OwnerRemediated condition
	// reports that remediation has been completed and the Machine has been deleted.
	for _, m := range machines {
		if !m.DeletionTimestamp.IsZero() {
			if c := conditions.Get(m, clusterv1.MachineOwnerRemediatedCondition); c != nil && c.Status == metav1.ConditionFalse && c.Reason == clusterv1.MachineSetMachineRemediationMachineDeletingReason {
				// Remediation for this Machine has been triggered by this controller but it is still in flight,
				// i.e. it still goes through the deletion workflow and exists in etcd.
				maxInFlight--
			}
		}
	}

	// Check if we can remediate any machines.
	if maxInFlight <= 0 {
		// No tokens available to remediate machines.
		log.V(3).Info("Remediation strategy is set, and maximum in flight has been reached", "machinesToBeRemediated", len(machinesToRemediate))
		if err := patchMachineConditions(ctx, r.Client, machinesToRemediate, metav1.Condition{
			Type:    clusterv1.MachineOwnerRemediatedCondition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.MachineSetMachineRemediationDeferredReason,
			Message: fmt.Sprintf("Waiting because there are already too many remediations in progress (spec.strategy.remediation.maxInFlight is %s)", owner.Spec.Remediation.MaxInFlight),
		}, nil); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Sort the machines from newest to oldest.
	// We are trying to remediate machines failing to come up first because
	// there is a chance that they are not hosting any workloads (minimize disruption).
	sortMachinesToRemediate(machinesToRemediate)

	// Check if we should limit the in flight operations.
	if len(machinesToRemediate) > maxInFlight {
		log.V(5).Info("Remediation strategy is set, limiting in flight operations", "machinesToBeRemediated", len(machinesToRemediate))
		// We have more machines to remediate than tokens available.
		allMachinesToRemediate := machinesToRemediate
		machinesToRemediate = allMachinesToRemediate[:maxInFlight]
		machinesToDeferRemediation := allMachinesToRemediate[maxInFlight:]

		if err := patchMachineConditions(ctx, r.Client, machinesToDeferRemediation, metav1.Condition{
			Type:    clusterv1.MachineOwnerRemediatedCondition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.MachineSetMachineRemediationDeferredReason,
			Message: fmt.Sprintf("Waiting because there are already too many remediations in progress (spec.strategy.remediation.maxInFlight is %s)", owner.Spec.Remediation.MaxInFlight),
		}, nil); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Run preflight checks.
	preflightCheckErrMessages, err := r.runPreflightChecks(ctx, cluster, ms, "Machine remediation")
	if err != nil || len(preflightCheckErrMessages) > 0 {
		if err != nil {
			// If err is not nil use that as the preflightCheckErrMessage
			preflightCheckErrMessages = append(preflightCheckErrMessages, err.Error())
		}

		listMessages := make([]string, len(preflightCheckErrMessages))
		for i, msg := range preflightCheckErrMessages {
			listMessages[i] = fmt.Sprintf("* %s", msg)
		}

		// PreflightChecks did not pass. Update the MachineOwnerRemediated condition on the unhealthy Machines with
		// WaitingForRemediationReason reason.
		if patchErr := patchMachineConditions(ctx, r.Client, machinesToRemediate, metav1.Condition{
			Type:    clusterv1.MachineOwnerRemediatedCondition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.MachineSetMachineRemediationDeferredReason,
			Message: strings.Join(listMessages, "\n"),
		}, &clusterv1.Condition{
			Type:     clusterv1.MachineOwnerRemediatedV1Beta1Condition,
			Status:   corev1.ConditionFalse,
			Reason:   clusterv1.WaitingForRemediationV1Beta1Reason,
			Severity: clusterv1.ConditionSeverityWarning,
			Message:  strings.Join(preflightCheckErrMessages, "; "),
		}); patchErr != nil {
			return ctrl.Result{}, kerrors.NewAggregate([]error{err, patchErr})
		}

		if err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: preflightFailedRequeueAfter}, nil
	}

	// PreflightChecks passed, so it is safe to remediate unhealthy machines by deleting them.

	// Note: We intentionally patch the Machines before we delete them to make this code reentrant.
	// If we delete the Machine first, the Machine would be filtered out on next reconcile because
	// it has a deletionTimestamp so it would never get the condition.
	// Instead if we set the condition but the deletion does not go through on next reconcile either the
	// condition will be fixed/updated or the Machine deletion will be retried.
	if err := patchMachineConditions(ctx, r.Client, machinesToRemediate, metav1.Condition{
		Type:    clusterv1.MachineOwnerRemediatedCondition,
		Status:  metav1.ConditionFalse,
		Reason:  clusterv1.MachineSetMachineRemediationMachineDeletingReason,
		Message: "Machine is deleting",
	}, &clusterv1.Condition{
		Type:   clusterv1.MachineOwnerRemediatedV1Beta1Condition,
		Status: corev1.ConditionTrue,
	}); err != nil {
		return ctrl.Result{}, err
	}
	var errs []error
	for _, m := range machinesToRemediate {
		if err := r.Client.Delete(ctx, m); err != nil && !apierrors.IsNotFound(err) {
			errs = append(errs, errors.Wrapf(err, "failed to delete Machine %s", klog.KObj(m)))
		}
		// Note: We intentionally log after Delete because we want this log line to show up only after DeletionTimestamp has been set.
		// Also, setting DeletionTimestamp doesn't mean the Machine is actually deleted (deletion takes some time).
		log.Info("Deleting Machine (remediating unhealthy Machine)", "Machine", klog.KObj(m))
	}
	if len(errs) > 0 {
		return ctrl.Result{}, errors.Wrapf(kerrors.NewAggregate(errs), "failed to delete unhealthy Machines")
	}

	return ctrl.Result{}, nil
}

func patchMachineConditions(ctx context.Context, c client.Client, machines []*clusterv1.Machine, condition metav1.Condition, v1beta1condition *clusterv1.Condition) error {
	var errs []error
	for _, m := range machines {
		patchHelper, err := patch.NewHelper(m, c)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		if v1beta1condition != nil {
			v1beta1conditions.Set(m, v1beta1condition)
		}
		conditions.Set(m, condition)

		if err := patchHelper.Patch(ctx, m,
			patch.WithOwnedV1Beta1Conditions{Conditions: []clusterv1.ConditionType{
				clusterv1.MachineOwnerRemediatedV1Beta1Condition,
			}}, patch.WithOwnedConditions{Conditions: []string{
				clusterv1.MachineOwnerRemediatedCondition,
			}}); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Wrapf(kerrors.NewAggregate(errs), "failed to patch Machines")
	}

	return nil
}

func (r *Reconciler) reconcileExternalTemplateReference(ctx context.Context, cluster *clusterv1.Cluster, ms *clusterv1.MachineSet, owner *clusterv1.MachineDeployment, ref clusterv1.ContractVersionedObjectReference) (objectNotFound bool, err error) {
	if !strings.HasSuffix(ref.Kind, clusterv1.TemplateSuffix) {
		return false, nil
	}

	obj, err := external.GetObjectFromContractVersionedRef(ctx, r.Client, ref, ms.Namespace)
	if err != nil {
		if apierrors.IsNotFound(err) {
			if !ms.DeletionTimestamp.IsZero() {
				// Tolerate object not found when the machineSet is being deleted.
				return true, nil
			}

			if owner == nil {
				// If the MachineSet is not in a MachineDeployment, return the error immediately.
				return true, err
			}
			// When the MachineSet is part of a MachineDeployment but isn't the current revision, we should
			// ignore the not found references and allow the controller to proceed.
			if !isCurrentMachineSet(ms, owner) {
				return true, nil
			}
			return true, err
		}
		return false, err
	}

	desiredOwnerRef := metav1.OwnerReference{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Cluster",
		Name:       cluster.Name,
		UID:        cluster.UID,
	}

	if util.HasExactOwnerRef(obj.GetOwnerReferences(), desiredOwnerRef) {
		return false, nil
	}

	patchHelper, err := patch.NewHelper(obj, r.Client)
	if err != nil {
		return false, err
	}

	obj.SetOwnerReferences(util.EnsureOwnerRef(obj.GetOwnerReferences(), desiredOwnerRef))

	return false, patchHelper.Patch(ctx, obj)
}

func (r *Reconciler) createBootstrapConfig(ctx context.Context, ms *clusterv1.MachineSet, machine *clusterv1.Machine) (*unstructured.Unstructured, clusterv1.ContractVersionedObjectReference, error) {
	bootstrapConfig, err := r.computeDesiredBootstrapConfig(ctx, ms, machine, nil)
	if err != nil {
		return nil, clusterv1.ContractVersionedObjectReference{}, errors.Wrapf(err, "failed to create BootstrapConfig")
	}

	// Create the full object with capi-machineset.
	// Below ssa.RemoveManagedFieldsForLabelsAndAnnotations will drop ownership for labels and annotations
	// so that in a subsequent syncMachines call capi-machineset-metadata can take ownership for them.
	// Note: This is done in way that it does not rely on managedFields being stored in the cache, so we can optimize
	// memory usage by dropping managedFields before storing objects in the cache.
	if err := ssa.Patch(ctx, r.Client, machineSetManagerName, bootstrapConfig); err != nil {
		return nil, clusterv1.ContractVersionedObjectReference{}, errors.Wrapf(err, "failed to create BootstrapConfig")
	}

	// Note: This field is only used for unit tests that use fake client because the fake client does not properly set resourceVersion
	//       on BootstrapConfig/InfraMachine after ssa.Patch and then ssa.RemoveManagedFieldsForLabelsAndAnnotations would fail.
	if !r.disableRemoveManagedFieldsForLabelsAndAnnotations {
		if err := ssa.RemoveManagedFieldsForLabelsAndAnnotations(ctx, r.Client, r.APIReader, bootstrapConfig, machineSetManagerName); err != nil {
			return nil, clusterv1.ContractVersionedObjectReference{}, errors.Wrapf(err, "failed to create BootstrapConfig")
		}
	}

	return bootstrapConfig, clusterv1.ContractVersionedObjectReference{
		APIGroup: bootstrapConfig.GroupVersionKind().Group,
		Kind:     bootstrapConfig.GetKind(),
		Name:     bootstrapConfig.GetName(),
	}, nil
}

func (r *Reconciler) computeDesiredBootstrapConfig(ctx context.Context, ms *clusterv1.MachineSet, machine *clusterv1.Machine, existingBootstrapConfig *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	var ownerReference *metav1.OwnerReference
	if existingBootstrapConfig == nil || !util.HasOwner(existingBootstrapConfig.GetOwnerReferences(), clusterv1.GroupVersion.String(), []string{"Machine"}) {
		ownerReference = &metav1.OwnerReference{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "MachineSet",
			Name:       ms.Name,
			UID:        ms.UID,
		}
	}

	apiVersion, err := contract.GetAPIVersion(ctx, r.Client, ms.Spec.Template.Spec.Bootstrap.ConfigRef.GroupKind())
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute desired BootstrapConfig")
	}
	templateRef := &corev1.ObjectReference{
		APIVersion: apiVersion,
		Kind:       ms.Spec.Template.Spec.Bootstrap.ConfigRef.Kind,
		Namespace:  ms.Namespace,
		Name:       ms.Spec.Template.Spec.Bootstrap.ConfigRef.Name,
	}

	template, err := external.Get(ctx, r.Client, templateRef)
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute desired BootstrapConfig")
	}
	generateTemplateInput := &external.GenerateTemplateInput{
		Template:    template,
		TemplateRef: templateRef,
		Namespace:   machine.Namespace,
		Name:        machine.Name,
		ClusterName: machine.Spec.ClusterName,
		OwnerRef:    ownerReference,
		Labels:      machine.Labels,
		Annotations: machine.Annotations,
	}
	bootstrapConfig, err := external.GenerateTemplate(generateTemplateInput)
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute desired BootstrapConfig")
	}

	if existingBootstrapConfig != nil {
		bootstrapConfig.SetUID(existingBootstrapConfig.GetUID())
	}
	return bootstrapConfig, nil
}

func (r *Reconciler) createInfraMachine(ctx context.Context, ms *clusterv1.MachineSet, machine *clusterv1.Machine) (*unstructured.Unstructured, clusterv1.ContractVersionedObjectReference, error) {
	infraMachine, err := r.computeDesiredInfraMachine(ctx, ms, machine, nil)
	if err != nil {
		return nil, clusterv1.ContractVersionedObjectReference{}, errors.Wrapf(err, "failed to create InfraMachine")
	}

	// Create the full object with capi-machineset.
	// Below ssa.RemoveManagedFieldsForLabelsAndAnnotations will drop ownership for labels and annotations
	// so that in a subsequent syncMachines call capi-machineset-metadata can take ownership for them.
	// Note: This is done in way that it does not rely on managedFields being stored in the cache, so we can optimize
	// memory usage by dropping managedFields before storing objects in the cache.
	if err := ssa.Patch(ctx, r.Client, machineSetManagerName, infraMachine); err != nil {
		return nil, clusterv1.ContractVersionedObjectReference{}, errors.Wrapf(err, "failed to create InfraMachine")
	}

	// Note: This field is only used for unit tests that use fake client because the fake client does not properly set resourceVersion
	//       on BootstrapConfig/InfraMachine after ssa.Patch and then ssa.RemoveManagedFieldsForLabelsAndAnnotations would fail.
	if !r.disableRemoveManagedFieldsForLabelsAndAnnotations {
		if err := ssa.RemoveManagedFieldsForLabelsAndAnnotations(ctx, r.Client, r.APIReader, infraMachine, machineSetManagerName); err != nil {
			return nil, clusterv1.ContractVersionedObjectReference{}, errors.Wrapf(err, "failed to create InfraMachine")
		}
	}

	return infraMachine, clusterv1.ContractVersionedObjectReference{
		APIGroup: infraMachine.GroupVersionKind().Group,
		Kind:     infraMachine.GetKind(),
		Name:     infraMachine.GetName(),
	}, nil
}

func (r *Reconciler) computeDesiredInfraMachine(ctx context.Context, ms *clusterv1.MachineSet, machine *clusterv1.Machine, existingInfraMachine *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	var ownerReference *metav1.OwnerReference
	if existingInfraMachine == nil || !util.HasOwner(existingInfraMachine.GetOwnerReferences(), clusterv1.GroupVersion.String(), []string{"Machine"}) {
		ownerReference = &metav1.OwnerReference{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "MachineSet",
			Name:       ms.Name,
			UID:        ms.UID,
		}
	}

	apiVersion, err := contract.GetAPIVersion(ctx, r.Client, ms.Spec.Template.Spec.InfrastructureRef.GroupKind())
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute desired InfraMachine")
	}
	templateRef := &corev1.ObjectReference{
		APIVersion: apiVersion,
		Kind:       ms.Spec.Template.Spec.InfrastructureRef.Kind,
		Namespace:  ms.Namespace,
		Name:       ms.Spec.Template.Spec.InfrastructureRef.Name,
	}

	template, err := external.Get(ctx, r.Client, templateRef)
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute desired InfraMachine")
	}
	generateTemplateInput := &external.GenerateTemplateInput{
		Template:    template,
		TemplateRef: templateRef,
		Namespace:   machine.Namespace,
		Name:        machine.Name,
		ClusterName: machine.Spec.ClusterName,
		OwnerRef:    ownerReference,
		Labels:      machine.Labels,
		Annotations: machine.Annotations,
	}
	infraMachine, err := external.GenerateTemplate(generateTemplateInput)
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute desired InfraMachine")
	}

	if existingInfraMachine != nil {
		infraMachine.SetUID(existingInfraMachine.GetUID())
	}
	return infraMachine, nil
}

// Returns the machines to be remediated in the following order
//   - Machines with RemediateMachineAnnotation annotation if any,
//   - Machines failing to come up first because
//     there is a chance that they are not hosting any workloads (minimize disruption).
func sortMachinesToRemediate(machines []*clusterv1.Machine) {
	sort.SliceStable(machines, func(i, j int) bool {
		if annotations.HasRemediateMachine(machines[i]) && !annotations.HasRemediateMachine(machines[j]) {
			return true
		}
		if !annotations.HasRemediateMachine(machines[i]) && annotations.HasRemediateMachine(machines[j]) {
			return false
		}
		// Use newest (and Name) as a tie-breaker criteria.
		if machines[i].CreationTimestamp.Equal(&machines[j].CreationTimestamp) {
			return machines[i].Name < machines[j].Name
		}
		return machines[i].CreationTimestamp.After(machines[j].CreationTimestamp.Time)
	})
}
