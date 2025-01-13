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
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/controllers/noderefutil"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/internal/controllers/machine"
	"sigs.k8s.io/cluster-api/internal/controllers/machinedeployment/mdutil"
	"sigs.k8s.io/cluster-api/internal/util/ssa"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta2conditions "sigs.k8s.io/cluster-api/util/conditions/v1beta2"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
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

	// stateConfirmationTimeout is the amount of time allowed to wait for desired state.
	stateConfirmationTimeout = 10 * time.Second

	// stateConfirmationInterval is the amount of time between polling for the desired state.
	// The polling is against a local memory cache.
	stateConfirmationInterval = 100 * time.Millisecond
)

const machineSetManagerName = "capi-machineset"

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

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string

	// Deprecated: DeprecatedInfraMachineNaming. Name the InfraStructureMachines after the InfraMachineTemplate.
	DeprecatedInfraMachineNaming bool

	ssaCache ssa.Cache
	recorder record.EventRecorder
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
	r.ssaCache = ssa.NewCache()
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

	log := ctrl.LoggerFrom(ctx).WithValues("Cluster", klog.KRef(machineSet.Namespace, machineSet.Spec.ClusterName))
	ctx = ctrl.LoggerInto(ctx, log)

	// Add finalizer first if not set to avoid the race condition between init and delete.
	if finalizerAdded, err := finalizers.EnsureFinalizer(ctx, r.Client, machineSet, clusterv1.MachineSetFinalizer); err != nil || finalizerAdded {
		return ctrl.Result{}, err
	}

	// AddOwners adds the owners of MachineSet as k/v pairs to the logger.
	// Specifically, it will add MachineDeployment.
	ctx, log, err := clog.AddOwners(ctx, r.Client, machineSet)
	if err != nil {
		return ctrl.Result{}, err
	}

	cluster, err := util.GetClusterByName(ctx, r.Client, machineSet.ObjectMeta.Namespace, machineSet.Spec.ClusterName)
	if err != nil {
		return ctrl.Result{}, err
	}

	if isPaused, conditionChanged, err := paused.EnsurePausedCondition(ctx, r.Client, cluster, machineSet); err != nil || isPaused || conditionChanged {
		return ctrl.Result{}, err
	}

	s := &scope{
		cluster:            cluster,
		machineSet:         machineSet,
		reconciliationTime: time.Now(),
	}

	// Initialize the patch helper
	patchHelper, err := patch.NewHelper(s.machineSet, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	defer func() {
		if err := r.reconcileStatus(ctx, s); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, errors.Wrapf(err, "failed to update status")})
		}

		r.updateStatus(ctx, s)

		// Always attempt to patch the object and status after each reconciliation.
		if err := patchMachineSet(ctx, patchHelper, s.machineSet); err != nil {
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
		wrapErrMachineSetReconcileFunc(r.syncReplicas, "failed to sync replicas"),
	)

	result, kerr := doReconcile(ctx, s, reconcileNormal)
	if kerr != nil {
		// Requeue if the reconcile failed because connection to workload cluster was down.
		if errors.Is(kerr, clustercache.ErrClusterNotConnected) {
			if len(kerr.Errors()) > 1 {
				log.Error(kerr, "Requeuing because connection to the workload cluster is down")
			} else {
				log.V(5).Info("Requeuing because connection to the workload cluster is down")
			}
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		err = kerr
	}
	return result, err
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

func patchMachineSet(ctx context.Context, patchHelper *patch.Helper, machineSet *clusterv1.MachineSet) error {
	// Always update the readyCondition by summarizing the state of other conditions.
	conditions.SetSummary(machineSet,
		conditions.WithConditions(
			clusterv1.MachinesCreatedCondition,
			clusterv1.ResizedCondition,
			clusterv1.MachinesReadyCondition,
		),
	)

	// Patch the object, ignoring conflicts on the conditions owned by this controller.
	options := []patch.Option{
		patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
			clusterv1.ReadyCondition,
			clusterv1.MachinesCreatedCondition,
			clusterv1.ResizedCondition,
			clusterv1.MachinesReadyCondition,
		}},
		patch.WithOwnedV1Beta2Conditions{Conditions: []string{
			clusterv1.MachineSetScalingUpV1Beta2Condition,
			clusterv1.MachineSetScalingDownV1Beta2Condition,
			clusterv1.MachineSetMachinesReadyV1Beta2Condition,
			clusterv1.MachineSetMachinesUpToDateV1Beta2Condition,
			clusterv1.MachineSetRemediatingV1Beta2Condition,
			clusterv1.MachineSetDeletingV1Beta2Condition,
		}},
	}
	return patchHelper.Patch(ctx, machineSet, options...)
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
	s.infrastructureObjectNotFound, err = r.reconcileExternalTemplateReference(ctx, cluster, machineSet, s.owningMachineDeployment, &machineSet.Spec.Template.Spec.InfrastructureRef)
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *Reconciler) reconcileBootstrapConfig(ctx context.Context, s *scope) (ctrl.Result, error) {
	cluster := s.cluster
	machineSet := s.machineSet
	// Make sure to reconcile the external bootstrap reference, if any.
	if s.machineSet.Spec.Template.Spec.Bootstrap.ConfigRef != nil {
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
			log.Info("Deleting Machine", "Machine", klog.KObj(machine))
			if err := r.Client.Delete(ctx, machine); err != nil && !apierrors.IsNotFound(err) {
				return ctrl.Result{}, errors.Wrapf(err, "failed to delete Machine %s", klog.KObj(machine))
			}
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
// Note: It also cleans up managed fields of all Machines so that Machines that were
// created/patched before (< v1.4.0) the controller adopted Server-Side-Apply (SSA) can also work with SSA.
// Note: For InfrastructureMachines and BootstrapConfigs it also drops ownership of "metadata.labels" and
// "metadata.annotations" from "manager" so that "capi-machineset" can own these fields and can work with SSA.
// Otherwise fields would be co-owned by our "old" "manager" and "capi-machineset" and then we would not be
// able to e.g. drop labels and annotations.
func (r *Reconciler) syncMachines(ctx context.Context, s *scope) (ctrl.Result, error) {
	machineSet := s.machineSet
	machines := s.machines
	if !s.getAndAdoptMachinesForMachineSetSucceeded {
		return ctrl.Result{}, nil
	}

	log := ctrl.LoggerFrom(ctx)
	for i := range machines {
		m := machines[i]

		upToDateCondition := newMachineUpToDateCondition(s)

		// If the machine is already being deleted, we only need to sync
		// the subset of fields that impact tearing down a machine
		if !m.DeletionTimestamp.IsZero() {
			patchHelper, err := patch.NewHelper(m, r.Client)
			if err != nil {
				return ctrl.Result{}, err
			}

			// Set all other in-place mutable fields that impact the ability to tear down existing machines.
			m.Spec.ReadinessGates = machineSet.Spec.Template.Spec.ReadinessGates
			m.Spec.NodeDrainTimeout = machineSet.Spec.Template.Spec.NodeDrainTimeout
			m.Spec.NodeDeletionTimeout = machineSet.Spec.Template.Spec.NodeDeletionTimeout
			m.Spec.NodeVolumeDetachTimeout = machineSet.Spec.Template.Spec.NodeVolumeDetachTimeout

			// Set machine's up to date condition
			if upToDateCondition != nil {
				v1beta2conditions.Set(m, *upToDateCondition)
			}

			if err := patchHelper.Patch(ctx, m, patch.WithOwnedV1Beta2Conditions{Conditions: []string{clusterv1.MachineUpToDateV1Beta2Condition}}); err != nil {
				return ctrl.Result{}, err
			}
			continue
		}

		// Patch the machine's up-to-date condition.
		// Note: for the time being we continue to rely on the patch helper for setting conditions; In the future, if
		// we will improve patch helper to support SSA, we can revisit this code and perform both this change and the others in place mutations in a single operation.
		if upToDateCondition != nil {
			patchHelper, err := patch.NewHelper(m, r.Client)
			if err != nil {
				return ctrl.Result{}, err
			}
			v1beta2conditions.Set(m, *upToDateCondition)
			if err := patchHelper.Patch(ctx, m, patch.WithOwnedV1Beta2Conditions{Conditions: []string{clusterv1.MachineUpToDateV1Beta2Condition}}); err != nil {
				return ctrl.Result{}, err
			}
		}

		// Cleanup managed fields of all Machines.
		// We do this so that Machines that were created/patched before the controller adopted Server-Side-Apply (SSA)
		// (< v1.4.0) can also work with SSA. Otherwise, fields would be co-owned by our "old" "manager" and
		// "capi-machineset" and then we would not be able to e.g. drop labels and annotations.
		if err := ssa.CleanUpManagedFieldsForSSAAdoption(ctx, r.Client, m, machineSetManagerName); err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to update machine: failed to adjust the managedFields of the Machine %q", m.Name)
		}

		// Update Machine to propagate in-place mutable fields from the MachineSet.
		updatedMachine := r.computeDesiredMachine(machineSet, m)
		err := ssa.Patch(ctx, r.Client, machineSetManagerName, updatedMachine, ssa.WithCachingProxy{Cache: r.ssaCache, Original: m})
		if err != nil {
			log.Error(err, "Failed to update Machine", "Machine", klog.KObj(updatedMachine))
			return ctrl.Result{}, errors.Wrapf(err, "failed to update Machine %q", klog.KObj(updatedMachine))
		}
		machines[i] = updatedMachine

		infraMachine, err := external.Get(ctx, r.Client, &updatedMachine.Spec.InfrastructureRef)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to get InfrastructureMachine %s",
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
			return ctrl.Result{}, errors.Wrapf(err, "failed to update machine: failed to adjust the managedFields of the InfrastructureMachine %s", klog.KObj(infraMachine))
		}
		// Update in-place mutating fields on InfrastructureMachine.
		if err := r.updateExternalObject(ctx, infraMachine, machineSet); err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to update InfrastructureMachine %s", klog.KObj(infraMachine))
		}

		if updatedMachine.Spec.Bootstrap.ConfigRef != nil {
			bootstrapConfig, err := external.Get(ctx, r.Client, updatedMachine.Spec.Bootstrap.ConfigRef)
			if err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "failed to get BootstrapConfig %s",
					klog.KRef(updatedMachine.Spec.Bootstrap.ConfigRef.Namespace, updatedMachine.Spec.Bootstrap.ConfigRef.Name))
			}
			// Cleanup managed fields of all BootstrapConfigs to drop ownership of labels and annotations
			// from "manager". We do this so that BootstrapConfigs that are created using the Create method
			// can also work with SSA. Otherwise, labels and annotations would be co-owned by our "old" "manager"
			// and "capi-machineset" and then we would not be able to e.g. drop labels and annotations.
			if err := ssa.DropManagedFields(ctx, r.Client, bootstrapConfig, machineSetManagerName, labelsAndAnnotationsManagedFieldPaths); err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "failed to update machine: failed to adjust the managedFields of the BootstrapConfig %s", klog.KObj(bootstrapConfig))
			}
			// Update in-place mutating fields on BootstrapConfig.
			if err := r.updateExternalObject(ctx, bootstrapConfig, machineSet); err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "failed to update BootstrapConfig %s", klog.KObj(bootstrapConfig))
			}
		}
	}
	return ctrl.Result{}, nil
}

func newMachineUpToDateCondition(s *scope) *metav1.Condition {
	// If the current MachineSet is a stand-alone MachineSet, the MachineSet controller does not set an up-to-date condition
	// on Machines, allowing tools managing higher level abstractions to set this condition.
	// This is also consistent with the fact that the MachineSet controller primarily takes care of the number of Machine
	// replicas, it doesn't reconcile them (even if we have a few exceptions like in-place propagation of a few selected
	// fields and remediation).
	if s.owningMachineDeployment == nil {
		return nil
	}

	// Determine current and desired state.
	// If the current MachineSet is owned by a MachineDeployment, we mirror what is implemented in the MachineDeployment controller
	// to trigger rollouts (by creating new MachineSets).
	// More specifically:
	// - desired state for the Machine is the spec.Template of the MachineDeployment
	// - current state for the Machine is the spec.Template of the MachineSet who owns the Machine
	// Note: We are intentionally considering current spec from the MachineSet instead of spec from the Machine itself in
	// order to surface info consistent with what the MachineDeployment controller uses to take decisions about rollouts.
	// The downside is that the system will ignore out of band changes applied to controlled Machines, which is
	// considered an acceptable trade-off given that out of band changes are the exception (users should not change
	// objects owned by the system).
	// However, if out of band changes happen, at least the system will ignore out of band changes consistently, both in the
	// MachineDeployment controller and in the condition computed here.
	current := &s.machineSet.Spec.Template
	desired := &s.owningMachineDeployment.Spec.Template

	upToDate, _, conditionMessages := mdutil.MachineTemplateUpToDate(current, desired)

	if s.owningMachineDeployment.Spec.RolloutAfter != nil {
		if s.owningMachineDeployment.Spec.RolloutAfter.Time.Before(s.reconciliationTime) && !s.machineSet.CreationTimestamp.After(s.owningMachineDeployment.Spec.RolloutAfter.Time) {
			upToDate = false
			conditionMessages = append(conditionMessages, "MachineDeployment spec.rolloutAfter expired")
		}
	}

	if !upToDate {
		for i := range conditionMessages {
			conditionMessages[i] = fmt.Sprintf("* %s", conditionMessages[i])
		}
		return &metav1.Condition{
			Type:   clusterv1.MachineUpToDateV1Beta2Condition,
			Status: metav1.ConditionFalse,
			Reason: clusterv1.MachineNotUpToDateV1Beta2Reason,
			// Note: the code computing the message for MachineDeployment's RolloutOut condition is making assumptions on the format/content of this message.
			Message: strings.Join(conditionMessages, "\n"),
		}
	}

	return &metav1.Condition{
		Type:   clusterv1.MachineUpToDateV1Beta2Condition,
		Status: metav1.ConditionTrue,
		Reason: clusterv1.MachineUpToDateV1Beta2Reason,
	}
}

// syncReplicas scales Machine resources up or down.
func (r *Reconciler) syncReplicas(ctx context.Context, s *scope) (ctrl.Result, error) {
	ms := s.machineSet
	machines := s.machines
	cluster := s.cluster
	if !s.getAndAdoptMachinesForMachineSetSucceeded {
		return ctrl.Result{}, nil
	}

	log := ctrl.LoggerFrom(ctx)
	if ms.Spec.Replicas == nil {
		return ctrl.Result{}, errors.Errorf("the Replicas field in Spec for MachineSet %v is nil, this should not be allowed", ms.Name)
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

		preflightCheckErrMessages, err := r.runPreflightChecks(ctx, cluster, ms, "Scale up")
		if err != nil || len(preflightCheckErrMessages) > 0 {
			if err != nil {
				// If err is not nil use that as the preflightCheckErrMessage
				preflightCheckErrMessages = append(preflightCheckErrMessages, err.Error())
			}

			s.scaleUpPreflightCheckErrMessages = preflightCheckErrMessages
			conditions.MarkFalse(ms, clusterv1.MachinesCreatedCondition, clusterv1.PreflightCheckFailedReason, clusterv1.ConditionSeverityError, strings.Join(preflightCheckErrMessages, "; "))
			if err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: preflightFailedRequeueAfter}, nil
		}

		var (
			machineList []*clusterv1.Machine
			errs        []error
		)

		for i := range diff {
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
					Client:      r.Client,
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
				Client:      r.Client,
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

		// Preserve all existing finalizers (including foregroundDeletion finalizer).
		finalizers := existingMachine.Finalizers
		// Ensure MachineFinalizer is set.
		if !sets.New[string](finalizers...).Has(clusterv1.MachineFinalizer) {
			finalizers = append(finalizers, clusterv1.MachineFinalizer)
		}
		desiredMachine.Finalizers = finalizers

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
	desiredMachine.Spec.ReadinessGates = machineSet.Spec.Template.Spec.ReadinessGates
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

func (r *Reconciler) waitForMachineCreation(ctx context.Context, machineList []*clusterv1.Machine) error {
	log := ctrl.LoggerFrom(ctx)

	for i := range len(machineList) {
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

	for i := range len(machineList) {
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

// reconcileStatus updates the Status field for the MachineSet
// It checks for the current state of the replicas and updates the Status of the MachineSet.
func (r *Reconciler) reconcileStatus(ctx context.Context, s *scope) error {
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
	if !ms.DeletionTimestamp.IsZero() {
		desiredReplicas = 0
	}
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
			log.Error(err, "Unable to retrieve Node status", "Node", klog.KObj(node))
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

func shouldRequeueForReplicaCountersRefresh(s *scope) ctrl.Result {
	replicas := ptr.Deref(s.machineSet.Spec.Replicas, 0)

	// Resync the MachineSet after MinReadySeconds as a last line of defense to guard against clock-skew.
	// Clock-skew is an issue as it may impact whether an available replica is counted as a ready replica.
	// A replica is available if the amount of time since last transition exceeds MinReadySeconds.
	// If there was a clock skew, checking whether the amount of time since last transition to ready state
	// exceeds MinReadySeconds could be incorrect.
	// To avoid an available replica stuck in the ready state, we force a reconcile after MinReadySeconds,
	// at which point it should confirm any available replica to be available.
	if s.machineSet.Spec.MinReadySeconds > 0 &&
		s.machineSet.Status.ReadyReplicas == replicas &&
		s.machineSet.Status.AvailableReplicas != replicas {
		minReadyResult := ctrl.Result{RequeueAfter: time.Duration(s.machineSet.Spec.MinReadySeconds) * time.Second}
		return minReadyResult
	}

	// Quickly reconcile until the nodes become Ready.
	if s.machineSet.Status.ReadyReplicas != replicas {
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

		shouldCleanup := conditions.IsTrue(m, clusterv1.MachineHealthCheckSucceededCondition) && conditions.IsFalse(m, clusterv1.MachineOwnerRemediatedCondition)
		shouldCleanupV1Beta2 := v1beta2conditions.IsTrue(m, clusterv1.MachineHealthCheckSucceededV1Beta2Condition) && v1beta2conditions.IsFalse(m, clusterv1.MachineOwnerRemediatedV1Beta2Condition)

		if !(shouldCleanup || shouldCleanupV1Beta2) {
			continue
		}

		patchHelper, err := patch.NewHelper(m, r.Client)
		if err != nil {
			errList = append(errList, err)
			continue
		}

		if shouldCleanup {
			conditions.Delete(m, clusterv1.MachineOwnerRemediatedCondition)
		}

		if shouldCleanupV1Beta2 {
			v1beta2conditions.Delete(m, clusterv1.MachineOwnerRemediatedV1Beta2Condition)
		}

		if err := patchHelper.Patch(ctx, m, patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
			clusterv1.MachineOwnerRemediatedCondition,
		}}, patch.WithOwnedV1Beta2Conditions{Conditions: []string{
			clusterv1.MachineOwnerRemediatedV1Beta2Condition,
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
				Type:    clusterv1.MachineOwnerRemediatedV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineSetMachineCannotBeRemediatedV1Beta2Reason,
				Message: "Machine won't be remediated because it is pending removal due to rollout",
			}, nil); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}

		if owner.Spec.Strategy != nil && owner.Spec.Strategy.Remediation != nil {
			if owner.Spec.Strategy.Remediation.MaxInFlight != nil {
				var err error
				replicas := int(ptr.Deref(owner.Spec.Replicas, 1))
				maxInFlight, err = intstr.GetScaledValueFromIntOrPercent(owner.Spec.Strategy.Remediation.MaxInFlight, replicas, true)
				if err != nil {
					return ctrl.Result{}, fmt.Errorf("failed to calculate maxInFlight to remediate machines: %v", err)
				}
				log = log.WithValues("maxInFlight", maxInFlight, "replicas", replicas)
			}
		}
	}

	// Update maxInFlight based on remediations that are in flight.
	// A Machine has a remediation in flight when Machine's OwnerRemediated condition
	// reports that remediation has been completed and the Machine has been deleted.
	for _, m := range machines {
		if !m.DeletionTimestamp.IsZero() {
			// TODO: Check for Status: False and Reason: MachineSetMachineRemediationMachineDeletingV1Beta2Reason
			// instead when starting to use v1beta2 conditions for control flow.
			if conditions.IsTrue(m, clusterv1.MachineOwnerRemediatedCondition) {
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
			Type:    clusterv1.MachineOwnerRemediatedV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.MachineSetMachineRemediationDeferredV1Beta2Reason,
			Message: fmt.Sprintf("Waiting because there are already too many remediations in progress (spec.strategy.remediation.maxInFlight is %s)", owner.Spec.Strategy.Remediation.MaxInFlight),
		}, nil); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Sort the machines from newest to oldest.
	// We are trying to remediate machines failing to come up first because
	// there is a chance that they are not hosting any workloads (minimize disruption).
	sort.SliceStable(machinesToRemediate, func(i, j int) bool {
		return machinesToRemediate[i].CreationTimestamp.After(machinesToRemediate[j].CreationTimestamp.Time)
	})

	// Check if we should limit the in flight operations.
	if len(machinesToRemediate) > maxInFlight {
		log.V(5).Info("Remediation strategy is set, limiting in flight operations", "machinesToBeRemediated", len(machinesToRemediate))
		// We have more machines to remediate than tokens available.
		allMachinesToRemediate := machinesToRemediate
		machinesToRemediate = allMachinesToRemediate[:maxInFlight]
		machinesToDeferRemediation := allMachinesToRemediate[maxInFlight:]

		if err := patchMachineConditions(ctx, r.Client, machinesToDeferRemediation, metav1.Condition{
			Type:    clusterv1.MachineOwnerRemediatedV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.MachineSetMachineRemediationDeferredV1Beta2Reason,
			Message: fmt.Sprintf("Waiting because there are already too many remediations in progress (spec.strategy.remediation.maxInFlight is %s)", owner.Spec.Strategy.Remediation.MaxInFlight),
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
			Type:    clusterv1.MachineOwnerRemediatedV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.MachineSetMachineRemediationDeferredV1Beta2Reason,
			Message: strings.Join(listMessages, "\n"),
		}, &clusterv1.Condition{
			Type:     clusterv1.MachineOwnerRemediatedCondition,
			Status:   corev1.ConditionFalse,
			Reason:   clusterv1.WaitingForRemediationReason,
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
		Type:    clusterv1.MachineOwnerRemediatedV1Beta2Condition,
		Status:  metav1.ConditionFalse,
		Reason:  clusterv1.MachineSetMachineRemediationMachineDeletingV1Beta2Reason,
		Message: "Machine is deleting",
	}, &clusterv1.Condition{
		Type:   clusterv1.MachineOwnerRemediatedCondition,
		Status: corev1.ConditionTrue,
	}); err != nil {
		return ctrl.Result{}, err
	}
	var errs []error
	for _, m := range machinesToRemediate {
		log.Info("Deleting unhealthy Machine", "Machine", klog.KObj(m))
		if err := r.Client.Delete(ctx, m); err != nil && !apierrors.IsNotFound(err) {
			errs = append(errs, errors.Wrapf(err, "failed to delete Machine %s", klog.KObj(m)))
		}
	}
	if len(errs) > 0 {
		return ctrl.Result{}, errors.Wrapf(kerrors.NewAggregate(errs), "failed to delete unhealthy Machines")
	}

	return ctrl.Result{}, nil
}

func patchMachineConditions(ctx context.Context, c client.Client, machines []*clusterv1.Machine, v1beta2Condition metav1.Condition, condition *clusterv1.Condition) error {
	var errs []error
	for _, m := range machines {
		patchHelper, err := patch.NewHelper(m, c)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		if condition != nil {
			conditions.Set(m, condition)
		}
		v1beta2conditions.Set(m, v1beta2Condition)

		if err := patchHelper.Patch(ctx, m,
			patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
				clusterv1.MachineOwnerRemediatedCondition,
			}}, patch.WithOwnedV1Beta2Conditions{Conditions: []string{
				clusterv1.MachineOwnerRemediatedV1Beta2Condition,
			}}); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Wrapf(kerrors.NewAggregate(errs), "failed to patch Machines")
	}

	return nil
}

func (r *Reconciler) reconcileExternalTemplateReference(ctx context.Context, cluster *clusterv1.Cluster, ms *clusterv1.MachineSet, owner *clusterv1.MachineDeployment, ref *corev1.ObjectReference) (objectNotFound bool, err error) {
	if !strings.HasSuffix(ref.Kind, clusterv1.TemplateSuffix) {
		return false, nil
	}

	if err := utilconversion.UpdateReferenceAPIContract(ctx, r.Client, ref); err != nil {
		return false, err
	}

	// Ensure the ref namespace is populated for objects not yet defaulted by webhook
	if ref.Namespace == "" {
		ref = ref.DeepCopy()
		ref.Namespace = cluster.Namespace
	}

	obj, err := external.Get(ctx, r.Client, ref)
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
