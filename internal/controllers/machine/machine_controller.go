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

package machine

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/api/core/v1beta2/index"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/controllers/noderefutil"
	runtimeclient "sigs.k8s.io/cluster-api/exp/runtime/client"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/internal/controllers/machine/drain"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/cache"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
	"sigs.k8s.io/cluster-api/util/finalizers"
	clog "sigs.k8s.io/cluster-api/util/log"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/paused"
	"sigs.k8s.io/cluster-api/util/predicates"
)

const (
	drainRetryInterval               = time.Duration(20) * time.Second
	waitForVolumeDetachRetryInterval = time.Duration(20) * time.Second
)

var (
	errNilNodeRef                 = errors.New("noderef is nil")
	errLastControlPlaneNode       = errors.New("last control plane member")
	errNoControlPlaneNodes        = errors.New("no control plane members")
	errClusterIsBeingDeleted      = errors.New("cluster is being deleted")
	errControlPlaneIsBeingDeleted = errors.New("control plane is being deleted")
)

// Update permissions on /finalizers subresrouce is required on management clusters with 'OwnerReferencesPermissionEnforcement' plugin enabled.
// See: https://kubernetes.io/docs/reference/access-authn-authz/admission-controllers/#ownerreferencespermissionenforcement
//
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io;bootstrap.cluster.x-k8s.io,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status;machines/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinedrainrules,verbs=get;list;watch
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch

// Reconciler reconciles a Machine object.
type Reconciler struct {
	Client        client.Client
	APIReader     client.Reader
	ClusterCache  clustercache.ClusterCache
	RuntimeClient runtimeclient.Client

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string

	RemoteConditionsGracePeriod time.Duration

	AdditionalSyncMachineLabels      []*regexp.Regexp
	AdditionalSyncMachineAnnotations []*regexp.Regexp

	controller      controller.Controller
	recorder        record.EventRecorder
	externalTracker external.ObjectTracker

	// nodeDeletionRetryTimeout determines how long the controller will retry deleting a node
	// during a single reconciliation.
	nodeDeletionRetryTimeout time.Duration

	// reconcileDeleteCache is used to store when reconcileDelete should not be executed before a
	// specific time for a specific Request. This is used to implement rate-limiting to avoid
	// e.g. spamming workload clusters with eviction requests during Node drain.
	reconcileDeleteCache cache.Cache[cache.ReconcileEntry]

	predicateLog *logr.Logger
}

func (r *Reconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	if r.Client == nil || r.APIReader == nil || r.ClusterCache == nil || r.RemoteConditionsGracePeriod < 2*time.Minute {
		// A minimum of 2m is enforced to ensure the ClusterCache always drops the connection before the grace period is reached.
		// In the worst case the ClusterCache will take FailureThreshold x (Interval + Timeout) = 5x(10s+5s) = 75s to drop a
		// connection. There might be some additional delays in health checking under high load. So we use 2m as a minimum
		// to have some buffer.
		return errors.New("Client, APIReader and ClusterCache must not be nil and RemoteConditionsGracePeriod must not be < 2m")
	}
	if feature.Gates.Enabled(feature.InPlaceUpdates) && r.RuntimeClient == nil {
		return errors.New("RuntimeClient must not be nil when InPlaceUpdates feature gate is enabled")
	}

	r.predicateLog = ptr.To(ctrl.LoggerFrom(ctx).WithValues("controller", "machine"))
	clusterToMachines, err := util.ClusterToTypedObjectsMapper(mgr.GetClient(), &clusterv1.MachineList{}, mgr.GetScheme())
	if err != nil {
		return err
	}
	msToMachines, err := util.MachineSetToObjectsMapper(mgr.GetClient(), &clusterv1.MachineList{}, mgr.GetScheme())
	if err != nil {
		return err
	}
	mdToMachines, err := util.MachineDeploymentToObjectsMapper(mgr.GetClient(), &clusterv1.MachineList{}, mgr.GetScheme())
	if err != nil {
		return err
	}

	if r.nodeDeletionRetryTimeout.Nanoseconds() == 0 {
		r.nodeDeletionRetryTimeout = 10 * time.Second
	}
	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.Machine{}).
		WithOptions(options).
		WithEventFilter(predicates.ResourceHasFilterLabel(mgr.GetScheme(), *r.predicateLog, r.WatchFilterValue)).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(clusterToMachines),
			builder.WithPredicates(
				// TODO: should this wait for Cluster.Status.InfrastructureReady similar to Infra Machine resources?
				predicates.All(mgr.GetScheme(), *r.predicateLog,
					predicates.ResourceIsChanged(mgr.GetScheme(), *r.predicateLog),
					predicates.ClusterControlPlaneInitialized(mgr.GetScheme(), *r.predicateLog),
					predicates.ResourceHasFilterLabel(mgr.GetScheme(), *r.predicateLog, r.WatchFilterValue),
				),
			)).
		WatchesRawSource(r.ClusterCache.GetClusterSource("machine", clusterToMachines, clustercache.WatchForProbeFailure(r.RemoteConditionsGracePeriod))).
		Watches(
			&clusterv1.MachineSet{},
			handler.EnqueueRequestsFromMapFunc(msToMachines),
			builder.WithPredicates(predicates.ResourceIsChanged(mgr.GetScheme(), *r.predicateLog)),
		).
		Watches(
			&clusterv1.MachineDeployment{},
			handler.EnqueueRequestsFromMapFunc(mdToMachines),
			builder.WithPredicates(predicates.ResourceIsChanged(mgr.GetScheme(), *r.predicateLog)),
		).
		Build(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	r.controller = c
	r.recorder = mgr.GetEventRecorderFor("machine-controller")
	r.externalTracker = external.ObjectTracker{
		Controller:      c,
		Cache:           mgr.GetCache(),
		Scheme:          mgr.GetScheme(),
		PredicateLogger: r.predicateLog,
	}
	r.reconcileDeleteCache = cache.New[cache.ReconcileEntry](cache.DefaultTTL)
	return nil
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	// Fetch the Machine instance
	m := &clusterv1.Machine{}
	if err := r.Client.Get(ctx, req.NamespacedName, m); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}

		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	ctx = ctrl.LoggerInto(ctx, ctrl.LoggerFrom(ctx).WithValues("Cluster", klog.KRef(m.Namespace, m.Spec.ClusterName)))

	// Add finalizer first if not set to avoid the race condition between init and delete.
	if finalizerAdded, err := finalizers.EnsureFinalizer(ctx, r.Client, m, clusterv1.MachineFinalizer); err != nil || finalizerAdded {
		return ctrl.Result{}, err
	}

	// AddOwners adds the owners of Machine as k/v pairs to the logger.
	// Specifically, it will add KubeadmControlPlane, MachineSet and MachineDeployment.
	ctx, _, err := clog.AddOwners(ctx, r.Client, m)
	if err != nil {
		return ctrl.Result{}, err
	}

	cluster, err := util.GetClusterByName(ctx, r.Client, m.Namespace, m.Spec.ClusterName)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to get cluster %q for machine %q in namespace %q",
			m.Spec.ClusterName, m.Name, m.Namespace)
	}

	// Initialize the patch helper
	patchHelper, err := patch.NewHelper(m, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	if isPaused, requeue, err := paused.EnsurePausedCondition(ctx, r.Client, cluster, m); err != nil || isPaused || requeue {
		return ctrl.Result{}, err
	}

	if !m.DeletionTimestamp.IsZero() {
		// Check reconcileDeleteCache to ensure we won't run reconcileDelete too frequently.
		// Note: The reconcileDelete func will add entries to the cache.
		if cacheEntry, ok := r.reconcileDeleteCache.Has(cache.NewReconcileEntryKey(m)); ok {
			if requeueAfter, requeue := cacheEntry.ShouldRequeue(time.Now()); requeue {
				return ctrl.Result{RequeueAfter: requeueAfter}, nil
			}
		}
	}

	s := &scope{
		cluster: cluster,
		machine: m,
	}

	defer func() {
		r.updateStatus(ctx, s)

		// Always attempt to patch the object and status after each reconciliation.
		// Patch ObservedGeneration only if the reconciliation completed successfully
		patchOpts := []patch.Option{}
		if reterr == nil {
			patchOpts = append(patchOpts, patch.WithStatusObservedGeneration{})
		}
		if err := patchMachine(ctx, patchHelper, m, patchOpts...); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}
	}()

	alwaysReconcile := []machineReconcileFunc{
		r.reconcileMachineOwnerAndLabels,
		r.reconcileBootstrap,
		r.reconcileInfrastructure,
		r.reconcileNode,
		r.reconcileCertificateExpiry,
	}

	// Handle deletion reconciliation loop.
	if !m.DeletionTimestamp.IsZero() {
		reconcileDelete := append(
			alwaysReconcile,
			r.reconcileDelete,
		)

		return doReconcile(ctx, reconcileDelete, s)
	}

	// Handle normal reconciliation loop.
	reconcileNormal := append(
		alwaysReconcile,
		r.reconcileInPlaceUpdate,
	)

	return doReconcile(ctx, reconcileNormal, s)
}

func patchMachine(ctx context.Context, patchHelper *patch.Helper, machine *clusterv1.Machine, options ...patch.Option) error {
	// Always update the readyCondition by summarizing the state of other conditions.
	// A step counter is added to represent progress during the provisioning process (instead we are hiding it
	// after provisioning - e.g. when a MHC condition exists - or during the deletion process).
	v1beta1conditions.SetSummary(machine,
		v1beta1conditions.WithConditions(
			// Infrastructure problems should take precedence over all the other conditions
			clusterv1.InfrastructureReadyV1Beta1Condition,
			// Bootstrap comes after, but it is relevant only during initial machine provisioning.
			clusterv1.BootstrapReadyV1Beta1Condition,
			// MHC reported condition should take precedence over the remediation progress
			clusterv1.MachineHealthCheckSucceededV1Beta1Condition,
			clusterv1.MachineOwnerRemediatedV1Beta1Condition,
			clusterv1.DrainingSucceededV1Beta1Condition,
		),
		v1beta1conditions.WithStepCounterIf(machine.DeletionTimestamp.IsZero() && machine.Spec.ProviderID == ""),
		v1beta1conditions.WithStepCounterIfOnly(
			clusterv1.BootstrapReadyV1Beta1Condition,
			clusterv1.InfrastructureReadyV1Beta1Condition,
		),
	)

	// Patch the object, ignoring conflicts on the conditions owned by this controller.
	// Also, if requested, we are adding additional options like e.g. Patch ObservedGeneration when issuing the
	// patch at the end of the reconcile loop.
	options = append(options,
		patch.WithOwnedV1Beta1Conditions{Conditions: []clusterv1.ConditionType{
			clusterv1.ReadyV1Beta1Condition,
			clusterv1.BootstrapReadyV1Beta1Condition,
			clusterv1.InfrastructureReadyV1Beta1Condition,
			clusterv1.DrainingSucceededV1Beta1Condition,
		}},
		patch.WithOwnedConditions{Conditions: []string{
			clusterv1.PausedCondition,
			clusterv1.MachineAvailableCondition,
			clusterv1.MachineReadyCondition,
			clusterv1.MachineBootstrapConfigReadyCondition,
			clusterv1.MachineInfrastructureReadyCondition,
			clusterv1.MachineNodeReadyCondition,
			clusterv1.MachineNodeHealthyCondition,
			clusterv1.MachineDeletingCondition,
			clusterv1.MachineUpdatingCondition,
		}},
	)

	return patchHelper.Patch(ctx, machine, options...)
}

type machineReconcileFunc func(context.Context, *scope) (ctrl.Result, error)

func doReconcile(ctx context.Context, phases []machineReconcileFunc, s *scope) (ctrl.Result, error) {
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

// scope holds the different objects that are read and used during the reconcile.
type scope struct {
	// cluster is the Cluster object the Machine belongs to.
	// It is set at the beginning of the reconcile function.
	cluster *clusterv1.Cluster

	// machine is the Machine object. It is set at the beginning
	// of the reconcile function.
	machine *clusterv1.Machine

	// infraMachine is the Infrastructure Machine object that is referenced by the
	// Machine. It is set after reconcileInfrastructure is called.
	infraMachine *unstructured.Unstructured

	// infraMachineNotFound is true if getting the infra machine object failed with an NotFound err
	infraMachineIsNotFound bool

	// bootstrapConfig is the BootstrapConfig object that is referenced by the
	// Machine. It is set after reconcileBootstrap is called.
	bootstrapConfig *unstructured.Unstructured

	// bootstrapConfigNotFound is true if getting the BootstrapConfig object failed with an NotFound err
	bootstrapConfigIsNotFound bool

	// node is the Kubernetes node hosted on the machine.
	node *corev1.Node

	// nodeGetError is the error that occurred when trying to get the Node.
	nodeGetError error

	// reconcileDeleteExecuted will be set to true if the logic in reconcileDelete is executed.
	// We might requeue early in reconcileDelete because of rate-limiting.
	// If the Machine has the deletionTimestamp set and this field is false we don't update the
	// Deleting condition.
	reconcileDeleteExecuted bool

	// deletingReason is the reason that should be used when setting the Deleting condition.
	deletingReason string

	// deletingMessage is the message that should be used when setting the Deleting condition.
	deletingMessage string

	// updatingReason is the reason that should be used when setting the Updating condition.
	updatingReason string

	// updatingMessage is the message that should be used when setting the Updating condition.
	updatingMessage string
}

func (r *Reconciler) reconcileMachineOwnerAndLabels(_ context.Context, s *scope) (ctrl.Result, error) {
	// If the machine is a stand-alone Machine, then set it as directly
	// owned by the Cluster (if not already present).
	if r.shouldAdopt(s.machine) {
		s.machine.SetOwnerReferences(util.EnsureOwnerRef(s.machine.GetOwnerReferences(), metav1.OwnerReference{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "Cluster",
			Name:       s.cluster.Name,
			UID:        s.cluster.UID,
		}))
	}

	// Always add the cluster label.
	if s.machine.Labels == nil {
		s.machine.Labels = make(map[string]string)
	}
	s.machine.Labels[clusterv1.ClusterNameLabel] = s.machine.Spec.ClusterName

	return ctrl.Result{}, nil
}

func (r *Reconciler) reconcileDelete(ctx context.Context, s *scope) (ctrl.Result, error) { //nolint:gocyclo
	log := ctrl.LoggerFrom(ctx)
	cluster := s.cluster
	m := s.machine

	s.reconcileDeleteExecuted = true

	// Add entry to the reconcileDeleteCache so we won't run reconcileDelete more than once per second.
	// Under certain circumstances the ReconcileAfter time will be set to a later time, e.g. when we're waiting
	// for Pods to terminate or volumes to detach.
	// This is done to ensure we're not spamming the workload cluster API server.
	r.reconcileDeleteCache.Add(cache.NewReconcileEntry(s.machine, time.Now().Add(1*time.Second)))

	// Set "fallback" reason and message. This is used if we don't set a more specific reason and message below.
	s.deletingReason = clusterv1.MachineDeletingReason
	s.deletingMessage = "Deletion started"

	err := r.isDeleteNodeAllowed(ctx, cluster, m, s.infraMachine)
	isDeleteNodeAllowed := err == nil
	if err != nil {
		switch err {
		case errNoControlPlaneNodes, errLastControlPlaneNode, errNilNodeRef, errClusterIsBeingDeleted, errControlPlaneIsBeingDeleted:
			nodeName := ""
			if m.Status.NodeRef.IsDefined() {
				nodeName = m.Status.NodeRef.Name
			}
			log.Info("Skipping deletion of Kubernetes Node associated with Machine as it is not allowed", "Node", klog.KRef("", nodeName), "cause", err.Error())
		default:
			s.deletingReason = clusterv1.MachineDeletingInternalErrorReason
			s.deletingMessage = "Please check controller logs for errors" //nolint:goconst // Not making this a constant for now
			return ctrl.Result{}, errors.Wrapf(err, "failed to check if Kubernetes Node deletion is allowed")
		}
	}

	if isDeleteNodeAllowed {
		// pre-drain.delete lifecycle hook
		// Return early without error, will requeue if/when the hook owner removes the annotation.
		if annotations.HasWithPrefix(clusterv1.PreDrainDeleteHookAnnotationPrefix, m.Annotations) {
			var hooks []string
			for key := range m.Annotations {
				if strings.HasPrefix(key, clusterv1.PreDrainDeleteHookAnnotationPrefix) {
					hooks = append(hooks, key)
				}
			}
			slices.Sort(hooks)
			log.Info("Waiting for pre-drain hooks to succeed", "hooks", strings.Join(hooks, ","))
			v1beta1conditions.MarkFalse(m, clusterv1.PreDrainDeleteHookSucceededV1Beta1Condition, clusterv1.WaitingExternalHookV1Beta1Reason, clusterv1.ConditionSeverityInfo, "")
			s.deletingReason = clusterv1.MachineDeletingWaitingForPreDrainHookReason
			s.deletingMessage = fmt.Sprintf("Waiting for pre-drain hooks to succeed (hooks: %s)", strings.Join(hooks, ","))
			return ctrl.Result{}, nil
		}
		v1beta1conditions.MarkTrue(m, clusterv1.PreDrainDeleteHookSucceededV1Beta1Condition)

		// Drain node before deletion and issue a patch in order to make this operation visible to the users.
		if r.isNodeDrainAllowed(m) {
			patchHelper, err := patch.NewHelper(m, r.Client)
			if err != nil {
				s.deletingReason = clusterv1.MachineDeletingInternalErrorReason
				s.deletingMessage = "Please check controller logs for errors"
				return ctrl.Result{}, err
			}

			if m.Status.Deletion == nil {
				m.Status.Deletion = &clusterv1.MachineDeletionStatus{}
			}
			if m.Status.Deletion.NodeDrainStartTime.IsZero() {
				m.Status.Deletion.NodeDrainStartTime = metav1.Now()
			}

			// The DrainingSucceededCondition never exists before the node is drained for the first time.
			if v1beta1conditions.Get(m, clusterv1.DrainingSucceededV1Beta1Condition) == nil {
				v1beta1conditions.MarkFalse(m, clusterv1.DrainingSucceededV1Beta1Condition, clusterv1.DrainingV1Beta1Reason, clusterv1.ConditionSeverityInfo, "Draining the node before deletion")
			}
			s.deletingReason = clusterv1.MachineDeletingDrainingNodeReason
			s.deletingMessage = fmt.Sprintf("Drain not completed yet (started at %s):", m.Status.Deletion.NodeDrainStartTime.Format(time.RFC3339))

			if err := patchMachine(ctx, patchHelper, m); err != nil {
				s.deletingReason = clusterv1.MachineDeletingInternalErrorReason
				s.deletingMessage = "Please check controller logs for errors"
				return ctrl.Result{}, err
			}

			result, err := r.drainNode(ctx, s)
			if err != nil {
				v1beta1conditions.MarkFalse(m, clusterv1.DrainingSucceededV1Beta1Condition, clusterv1.DrainingFailedV1Beta1Reason, clusterv1.ConditionSeverityWarning, "%s", err.Error())
				s.deletingReason = clusterv1.MachineDeletingDrainingNodeReason
				s.deletingMessage = "Error draining Node, please check controller logs for errors"
				r.recorder.Eventf(m, corev1.EventTypeWarning, "FailedDrainNode", "error draining Machine's node %q: %v", m.Status.NodeRef.Name, err)
				return ctrl.Result{}, err
			}
			if !result.IsZero() {
				// Note: For non-error cases where the drain is not completed yet the DrainingSucceeded condition is updated in drainNode.
				return result, nil
			}

			v1beta1conditions.MarkTrue(m, clusterv1.DrainingSucceededV1Beta1Condition)
			r.recorder.Eventf(m, corev1.EventTypeNormal, "SuccessfulDrainNode", "success draining Machine's node %q", m.Status.NodeRef.Name)
		}

		// After node draining is completed, and if isNodeVolumeDetachingAllowed returns True, make sure all
		// volumes are detached before proceeding to delete the Node.
		// In case the node is unreachable, the detachment is skipped.
		if r.isNodeVolumeDetachingAllowed(m) {
			if m.Status.Deletion == nil {
				m.Status.Deletion = &clusterv1.MachineDeletionStatus{}
			}
			if m.Status.Deletion.WaitForNodeVolumeDetachStartTime.IsZero() {
				m.Status.Deletion.WaitForNodeVolumeDetachStartTime = metav1.Now()
			}

			// The VolumeDetachSucceededCondition never exists before we wait for volume detachment for the first time.
			if v1beta1conditions.Get(m, clusterv1.VolumeDetachSucceededV1Beta1Condition) == nil {
				v1beta1conditions.MarkFalse(m, clusterv1.VolumeDetachSucceededV1Beta1Condition, clusterv1.WaitingForVolumeDetachV1Beta1Reason, clusterv1.ConditionSeverityInfo, "Waiting for node volumes to be detached")
			}
			s.deletingReason = clusterv1.MachineDeletingWaitingForVolumeDetachReason
			s.deletingMessage = fmt.Sprintf("Waiting for Node volumes to be detached (started at %s)", m.Status.Deletion.WaitForNodeVolumeDetachStartTime.Format(time.RFC3339))

			result, err := r.shouldWaitForNodeVolumes(ctx, s)
			if err != nil {
				s.deletingReason = clusterv1.MachineDeletingWaitingForVolumeDetachReason
				s.deletingMessage = "Error waiting for volumes to be detached from Node, please check controller logs for errors"
				r.recorder.Eventf(m, corev1.EventTypeWarning, "FailedWaitForVolumeDetach", "error waiting for node volumes detaching, Machine's node %q: %v", m.Status.NodeRef.Name, err)
				return ctrl.Result{}, err
			}
			if !result.IsZero() {
				return result, nil
			}
			v1beta1conditions.MarkTrue(m, clusterv1.VolumeDetachSucceededV1Beta1Condition)
			r.recorder.Eventf(m, corev1.EventTypeNormal, "NodeVolumesDetached", "success waiting for node volumes detaching Machine's node %q", m.Status.NodeRef.Name)
		}
	}

	// pre-term.delete lifecycle hook
	// Return early without error, will requeue if/when the hook owner removes the annotation.
	if annotations.HasWithPrefix(clusterv1.PreTerminateDeleteHookAnnotationPrefix, m.Annotations) {
		var hooks []string
		for key := range m.Annotations {
			if strings.HasPrefix(key, clusterv1.PreTerminateDeleteHookAnnotationPrefix) {
				hooks = append(hooks, key)
			}
		}
		slices.Sort(hooks)
		log.Info("Waiting for pre-terminate hooks to succeed", "hooks", strings.Join(hooks, ","))
		v1beta1conditions.MarkFalse(m, clusterv1.PreTerminateDeleteHookSucceededV1Beta1Condition, clusterv1.WaitingExternalHookV1Beta1Reason, clusterv1.ConditionSeverityInfo, "")
		s.deletingReason = clusterv1.MachineDeletingWaitingForPreTerminateHookReason
		s.deletingMessage = fmt.Sprintf("Waiting for pre-terminate hooks to succeed (hooks: %s)", strings.Join(hooks, ","))
		return ctrl.Result{}, nil
	}
	v1beta1conditions.MarkTrue(m, clusterv1.PreTerminateDeleteHookSucceededV1Beta1Condition)

	infrastructureDeleted, err := r.reconcileDeleteInfrastructure(ctx, s)
	if err != nil {
		s.deletingReason = clusterv1.MachineDeletingInternalErrorReason
		s.deletingMessage = fmt.Sprintf("Failed to delete %s, please check controller logs for errors", m.Spec.InfrastructureRef.Kind)
		return ctrl.Result{}, err
	}
	if !infrastructureDeleted {
		log.Info("Waiting for infrastructure to be deleted", m.Spec.InfrastructureRef.Kind, klog.KRef(m.Namespace, m.Spec.InfrastructureRef.Name))
		s.deletingReason = clusterv1.MachineDeletingWaitingForInfrastructureDeletionReason
		s.deletingMessage = fmt.Sprintf("Waiting for %s to be deleted", m.Spec.InfrastructureRef.Kind)
		return ctrl.Result{}, nil
	}

	if m.Spec.Bootstrap.ConfigRef.IsDefined() {
		bootstrapDeleted, err := r.reconcileDeleteBootstrap(ctx, s)
		if err != nil {
			s.deletingReason = clusterv1.MachineDeletingInternalErrorReason
			s.deletingMessage = fmt.Sprintf("Failed to delete %s, please check controller logs for errors", m.Spec.Bootstrap.ConfigRef.Kind)
			return ctrl.Result{}, err
		}
		if !bootstrapDeleted {
			log.Info("Waiting for bootstrap to be deleted", m.Spec.Bootstrap.ConfigRef.Kind, klog.KRef(m.Namespace, m.Spec.Bootstrap.ConfigRef.Name))
			s.deletingReason = clusterv1.MachineDeletingWaitingForBootstrapDeletionReason
			s.deletingMessage = fmt.Sprintf("Waiting for %s to be deleted", m.Spec.Bootstrap.ConfigRef.Kind)
			return ctrl.Result{}, nil
		}
	}

	// We only delete the node after the underlying infrastructure is gone.
	// https://github.com/kubernetes-sigs/cluster-api/issues/2565
	if isDeleteNodeAllowed {
		log.Info("Deleting node", "Node", klog.KRef("", m.Status.NodeRef.Name))

		var deleteNodeErr error
		waitErr := wait.PollUntilContextTimeout(ctx, 2*time.Second, r.nodeDeletionRetryTimeout, true, func(ctx context.Context) (bool, error) {
			if deleteNodeErr = r.deleteNode(ctx, cluster, m.Status.NodeRef.Name); deleteNodeErr != nil && !apierrors.IsNotFound(errors.Cause(deleteNodeErr)) {
				return false, nil
			}
			return true, nil
		})
		if waitErr != nil {
			log.Error(deleteNodeErr, "Timed out deleting node", "Node", klog.KRef("", m.Status.NodeRef.Name))
			v1beta1conditions.MarkFalse(m, clusterv1.MachineNodeHealthyV1Beta1Condition, clusterv1.DeletionFailedV1Beta1Reason, clusterv1.ConditionSeverityWarning, "")
			r.recorder.Eventf(m, corev1.EventTypeWarning, "FailedDeleteNode", "error deleting Machine's node: %v", deleteNodeErr)

			// If the node deletion timeout is not expired yet, requeue the Machine for reconciliation.
			if m.Spec.Deletion.NodeDeletionTimeoutSeconds == nil || *m.Spec.Deletion.NodeDeletionTimeoutSeconds == 0 || m.DeletionTimestamp.Add(time.Duration(*m.Spec.Deletion.NodeDeletionTimeoutSeconds)*time.Second).After(time.Now()) {
				s.deletingReason = clusterv1.MachineDeletingDeletingNodeReason
				s.deletingMessage = "Error deleting Node, please check controller logs for errors"
				return ctrl.Result{}, deleteNodeErr
			}
			log.Info("Node deletion timeout expired, continuing without Node deletion.")
		}
	}

	s.deletingReason = clusterv1.MachineDeletingDeletionCompletedReason
	s.deletingMessage = "Deletion completed"

	controllerutil.RemoveFinalizer(m, clusterv1.MachineFinalizer)
	return ctrl.Result{}, nil
}

const (
	// KubeadmControlPlanePreTerminateHookCleanupAnnotation inlined from KCP (we want to avoid importing the KCP API package).
	KubeadmControlPlanePreTerminateHookCleanupAnnotation = clusterv1.PreTerminateDeleteHookAnnotationPrefix + "/kcp-cleanup"
)

func (r *Reconciler) isNodeDrainAllowed(m *clusterv1.Machine) bool {
	if util.IsControlPlaneMachine(m) && util.HasOwner(m.GetOwnerReferences(), clusterv1.GroupVersionControlPlane.String(), []string{"KubeadmControlPlane"}) {
		if _, exists := m.Annotations[KubeadmControlPlanePreTerminateHookCleanupAnnotation]; !exists {
			return false
		}
	}

	if _, exists := m.Annotations[clusterv1.ExcludeNodeDrainingAnnotation]; exists {
		return false
	}

	if r.nodeDrainTimeoutExceeded(m) {
		return false
	}

	return true
}

// isNodeVolumeDetachingAllowed returns False if either ExcludeWaitForNodeVolumeDetachAnnotation annotation is set OR
// nodeVolumeDetachTimeoutExceeded timeout is exceeded, otherwise returns True.
func (r *Reconciler) isNodeVolumeDetachingAllowed(m *clusterv1.Machine) bool {
	if util.IsControlPlaneMachine(m) && util.HasOwner(m.GetOwnerReferences(), clusterv1.GroupVersionControlPlane.String(), []string{"KubeadmControlPlane"}) {
		if _, exists := m.Annotations[KubeadmControlPlanePreTerminateHookCleanupAnnotation]; !exists {
			return false
		}
	}

	if _, exists := m.Annotations[clusterv1.ExcludeWaitForNodeVolumeDetachAnnotation]; exists {
		return false
	}

	if r.nodeVolumeDetachTimeoutExceeded(m) {
		return false
	}

	return true
}

func (r *Reconciler) nodeDrainTimeoutExceeded(machine *clusterv1.Machine) bool {
	// if the NodeDrainTimeoutSeconds type is not set by user
	if machine.Status.Deletion == nil || machine.Spec.Deletion.NodeDrainTimeoutSeconds == nil || *machine.Spec.Deletion.NodeDrainTimeoutSeconds <= 0 {
		return false
	}

	// if the NodeDrainStartTime does not exist
	if machine.Status.Deletion.NodeDrainStartTime.IsZero() {
		return false
	}

	now := time.Now()
	diff := now.Sub(machine.Status.Deletion.NodeDrainStartTime.Time)
	return diff.Seconds() >= float64(*machine.Spec.Deletion.NodeDrainTimeoutSeconds)
}

// nodeVolumeDetachTimeoutExceeded returns False if either NodeVolumeDetachTimeoutSeconds is set to nil or <=0 OR
// WaitForNodeVolumeDetachStartTime is not set on the Machine. Otherwise returns true if the timeout is expired
// since the WaitForNodeVolumeDetachStartTime.
func (r *Reconciler) nodeVolumeDetachTimeoutExceeded(machine *clusterv1.Machine) bool {
	// if the NodeVolumeDetachTimeoutSeconds type is not set by user
	if machine.Status.Deletion == nil || machine.Spec.Deletion.NodeVolumeDetachTimeoutSeconds == nil || *machine.Spec.Deletion.NodeVolumeDetachTimeoutSeconds <= 0 {
		return false
	}

	// if the WaitForNodeVolumeDetachStartTime does not exist
	if machine.Status.Deletion.WaitForNodeVolumeDetachStartTime.IsZero() {
		return false
	}

	now := time.Now()
	diff := now.Sub(machine.Status.Deletion.WaitForNodeVolumeDetachStartTime.Time)
	return diff.Seconds() >= float64(*machine.Spec.Deletion.NodeVolumeDetachTimeoutSeconds)
}

// isDeleteNodeAllowed returns nil only if the Machine's NodeRef is not nil
// and if the Machine is not the last control plane node in the cluster.
func (r *Reconciler) isDeleteNodeAllowed(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine, infraMachine *unstructured.Unstructured) error {
	log := ctrl.LoggerFrom(ctx)
	// Return early if the cluster is being deleted.
	if !cluster.DeletionTimestamp.IsZero() {
		return errClusterIsBeingDeleted
	}

	var providerID string
	if machine.Spec.ProviderID != "" {
		providerID = machine.Spec.ProviderID
	} else if infraMachine != nil {
		// Fallback to retrieve from infraMachine.
		if providerIDFromInfraMachine, err := contract.InfrastructureMachine().ProviderID().Get(infraMachine); err == nil {
			providerID = *providerIDFromInfraMachine
		}
	}

	if !machine.Status.NodeRef.IsDefined() && providerID != "" {
		// If we don't have a node reference, but a provider id has been set,
		// try to retrieve the node one more time.
		//
		// NOTE: The following is a best-effort attempt to retrieve the node,
		// errors are logged but not returned to ensure machines are deleted
		// even if the node cannot be retrieved.
		remoteClient, err := r.ClusterCache.GetClient(ctx, util.ObjectKey(cluster))
		if err != nil {
			log.Error(err, "Failed to get cluster client while deleting Machine and checking for nodes")
		} else {
			node, err := r.getNode(ctx, remoteClient, providerID)
			if err != nil && err != ErrNodeNotFound {
				log.Error(err, "Failed to get node while deleting Machine")
			} else if err == nil {
				machine.Status.NodeRef = clusterv1.MachineNodeReference{
					Name: node.Name,
				}
			}
		}
	}

	if !machine.Status.NodeRef.IsDefined() {
		// Cannot delete something that doesn't exist.
		return errNilNodeRef
	}

	// controlPlaneRef is an optional field in the Cluster so skip the external
	// managed control plane check if it is nil
	if cluster.Spec.ControlPlaneRef.IsDefined() {
		controlPlane, err := external.GetObjectFromContractVersionedRef(ctx, r.Client, cluster.Spec.ControlPlaneRef, cluster.Namespace)
		if apierrors.IsNotFound(err) {
			// If control plane object in the reference does not exist, log and skip check for
			// external managed control plane
			log.Error(err, "Control plane object specified in cluster spec.controlPlaneRef does not exist", cluster.Spec.ControlPlaneRef.Kind, klog.KRef(cluster.Namespace, cluster.Spec.ControlPlaneRef.Name))
		} else {
			if err != nil {
				// If any other error occurs when trying to get the control plane object,
				// return the error so we can retry
				return err
			}

			// Return early if the object referenced by controlPlaneRef is being deleted.
			if !controlPlane.GetDeletionTimestamp().IsZero() {
				return errControlPlaneIsBeingDeleted
			}

			// Check if the ControlPlane is externally managed (AKS, EKS, GKE, etc)
			// and skip the following section if control plane is externally managed
			// because there will be no control plane nodes registered
			if util.IsExternalManagedControlPlane(controlPlane) {
				return nil
			}
		}
	}

	// Get all of the active machines that belong to this cluster.
	machines, err := collections.GetFilteredMachinesForCluster(ctx, r.Client, cluster, collections.ActiveMachines)
	if err != nil {
		return err
	}

	// Whether or not it is okay to delete the NodeRef depends on the
	// number of remaining control plane members and whether or not this
	// machine is one of them.
	numControlPlaneMachines := len(machines.Filter(collections.ControlPlaneMachines(cluster.Name)))
	if numControlPlaneMachines == 0 {
		// Do not delete the NodeRef if there are no remaining members of
		// the control plane.
		return errNoControlPlaneNodes
	}
	// Otherwise it is okay to delete the NodeRef.
	return nil
}

func (r *Reconciler) drainNode(ctx context.Context, s *scope) (ctrl.Result, error) {
	cluster := s.cluster
	machine := s.machine
	nodeName := s.machine.Status.NodeRef.Name

	log := ctrl.LoggerFrom(ctx, "Node", klog.KRef("", nodeName))
	ctx = ctrl.LoggerInto(ctx, log)

	remoteClient, err := r.ClusterCache.GetClient(ctx, util.ObjectKey(cluster))
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to drain Node %s", nodeName)
	}

	node := &corev1.Node{}
	if err := remoteClient.Get(ctx, client.ObjectKey{Name: nodeName}, node); err != nil {
		if apierrors.IsNotFound(err) {
			// If an admin deletes the node directly, we'll end up here.
			log.Info("Could not find Node from Machine.status.nodeRef, skipping Node drain.")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, errors.Wrapf(err, "unable to get Node %s", nodeName)
	}

	drainer := &drain.Helper{
		Client:             r.Client,
		RemoteClient:       remoteClient,
		GracePeriodSeconds: -1,
	}

	if noderefutil.IsNodeUnreachable(node) {
		// Kubelet is unreachable, pods will never disappear.

		// SkipWaitForDeleteTimeoutSeconds ensures the drain completes
		// even if pod objects are not deleted.
		drainer.SkipWaitForDeleteTimeoutSeconds = 1

		// kube-apiserver sets the `deletionTimestamp` to a future date computed using the grace period.
		// We are effectively waiting for GracePeriodSeconds + SkipWaitForDeleteTimeoutSeconds.
		// Override the grace period of pods to reduce the time needed to skip them.
		drainer.GracePeriodSeconds = 1

		// Our drain code still respects PDBs when evicting Pods, but that does not mean they are respected
		// in general by the entire system.
		// When a Node becomes unreachable the following happens:
		// * node.kubernetes.io/unreachable:NoExecute taint is set on the Node
		// * taint manager will evict Pods immediately because of the NoExecute taint (without respecting PDBs)
		//   * https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/#concepts
		//     "NoExecute": "Pods that do not tolerate the taint are evicted immediately""
		// * our drain code will now ignore the Pods (as they quickly have a deletionTimestamp older than 2 seconds)
		log.V(3).Info("Node is unreachable, draining will use 1s GracePeriodSeconds and will ignore all Pods that have a deletionTimestamp > 1s old")
	}

	if err := drainer.CordonNode(ctx, node); err != nil {
		// Machine will be re-reconciled after a cordon failure.
		return ctrl.Result{}, errors.Wrapf(err, "failed to cordon Node %s", node.Name)
	}

	podDeleteList, err := drainer.GetPodsForEviction(ctx, cluster, machine, nodeName)
	if err != nil {
		return ctrl.Result{}, err
	}

	podsToBeDrained := podDeleteList.Pods()
	if len(podsToBeDrained) == 0 {
		log.Info("Drain completed")
		return ctrl.Result{}, nil
	}

	log.Info("Draining Node")

	evictionResult := drainer.EvictPods(ctx, podDeleteList)

	if evictionResult.DrainCompleted() {
		log.Info("Drain completed, remaining Pods on the Node have been evicted")
		return ctrl.Result{}, nil
	}

	// Add entry to the reconcileDeleteCache so we won't retry drain again before drainRetryInterval.
	r.reconcileDeleteCache.Add(cache.NewReconcileEntry(machine, time.Now().Add(drainRetryInterval)))

	conditionMessage := evictionResult.ConditionMessage(machine.Status.Deletion.NodeDrainStartTime)
	v1beta1conditions.MarkFalse(machine, clusterv1.DrainingSucceededV1Beta1Condition, clusterv1.DrainingV1Beta1Reason, clusterv1.ConditionSeverityInfo, "%s", conditionMessage)
	s.deletingReason = clusterv1.MachineDeletingDrainingNodeReason
	s.deletingMessage = conditionMessage
	podsFailedEviction := []*corev1.Pod{}
	for _, p := range evictionResult.PodsFailedEviction {
		podsFailedEviction = append(podsFailedEviction, p...)
	}
	log.Info(fmt.Sprintf("Drain not completed yet, requeuing in %s", drainRetryInterval),
		"podsFailedEviction", drain.PodListToString(podsFailedEviction, 5),
		"podsWithDeletionTimestamp", drain.PodListToString(evictionResult.PodsDeletionTimestampSet, 5),
		"podsToTriggerEvictionLater", drain.PodListToString(evictionResult.PodsToTriggerEvictionLater, 5),
		"podsToWaitCompletedNow", drain.PodListToString(evictionResult.PodsToWaitCompletedNow, 5),
		"podsToWaitCompletedLater", drain.PodListToString(evictionResult.PodsToWaitCompletedLater, 5),
	)
	return ctrl.Result{RequeueAfter: drainRetryInterval}, nil
}

// shouldWaitForNodeVolumes returns true if node status still have volumes attached and the node is reachable
// pod deletion and volume detach happen asynchronously, so pod could be deleted before volume detached from the node
// this could cause issue for some storage provisioner, for example, vsphere-volume this is problematic
// because if the node is deleted before detach success, then the underline VMDK will be deleted together with the Machine
// so after node draining we need to check if all volumes are detached before deleting the node.
func (r *Reconciler) shouldWaitForNodeVolumes(ctx context.Context, s *scope) (ctrl.Result, error) {
	nodeName := s.machine.Status.NodeRef.Name
	log := ctrl.LoggerFrom(ctx, "Node", klog.KRef("", nodeName))
	ctx = ctrl.LoggerInto(ctx, log)

	cluster := s.cluster
	machine := s.machine

	remoteClient, err := r.ClusterCache.GetClient(ctx, util.ObjectKey(cluster))
	if err != nil {
		return ctrl.Result{}, err
	}

	node := &corev1.Node{}
	if err := remoteClient.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Could not find Node from Machine.status.nodeRef, skip waiting for volume detachment.")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if noderefutil.IsNodeUnreachable(node) {
		// If a node is unreachable, we can't detach the volume.
		// We need to skip the detachment as we otherwise block deletions
		// of unreachable nodes when a volume is attached.
		log.Info("Node is unreachable, skip waiting for volume detachment.")
		return ctrl.Result{}, nil
	}

	// Get two sets of information about volumes currently attached to the node:
	// * VolumesAttached names from node.Status.VolumesAttached
	// * PersistentVolume names from VolumeAttachments with status.Attached set to true
	attachedNodeVolumeNames, attachedPVNames, err := getAttachedVolumeInformation(ctx, remoteClient, node)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Return early if there are no volumes to wait for getting detached.
	if len(attachedNodeVolumeNames) == 0 && len(attachedPVNames) == 0 {
		return ctrl.Result{}, nil
	}

	// Get all PVCs we want to ignore because they belong to Pods for which we skipped drain.
	pvcsToIgnoreFromPods, err := getPersistentVolumeClaimsToIgnore(ctx, r.Client, remoteClient, cluster, machine, nodeName)
	if err != nil {
		return ctrl.Result{}, err
	}

	// List all PersistentVolumes and return the ones we want to wait for.
	attachedVolumeInformation, err := getPersistentVolumesWaitingForDetach(ctx, remoteClient, attachedNodeVolumeNames, attachedPVNames, pvcsToIgnoreFromPods)
	if err != nil {
		return ctrl.Result{}, err
	}

	// If no pvcs were found we have to wait for and there is no unmatched information, then we are finished with waiting for volumes.
	if attachedVolumeInformation.isEmpty() {
		return ctrl.Result{}, nil
	}

	// Add entry to the reconcileDeleteCache so we won't retry shouldWaitForNodeVolumes again before waitForVolumeDetachRetryInterval.
	r.reconcileDeleteCache.Add(cache.NewReconcileEntry(machine, time.Now().Add(waitForVolumeDetachRetryInterval)))

	s.deletingReason = clusterv1.MachineDeletingWaitingForVolumeDetachReason
	s.deletingMessage = attachedVolumeInformation.conditionMessage(machine)

	log.Info("Waiting for Node volumes to be detached",
		attachedVolumeInformation.logKeys()...,
	)
	return ctrl.Result{RequeueAfter: waitForVolumeDetachRetryInterval}, nil
}

func (r *Reconciler) deleteNode(ctx context.Context, cluster *clusterv1.Cluster, name string) error {
	remoteClient, err := r.ClusterCache.GetClient(ctx, util.ObjectKey(cluster))
	if err != nil {
		return errors.Wrapf(err, "failed deleting Node because connection to the workload cluster is down")
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	if err := remoteClient.Delete(ctx, node); err != nil {
		return errors.Wrapf(err, "error deleting node %s", name)
	}
	return nil
}

func (r *Reconciler) reconcileDeleteBootstrap(ctx context.Context, s *scope) (bool, error) {
	if s.bootstrapConfig == nil && s.bootstrapConfigIsNotFound {
		v1beta1conditions.MarkFalse(s.machine, clusterv1.BootstrapReadyV1Beta1Condition, clusterv1.DeletedV1Beta1Reason, clusterv1.ConditionSeverityInfo, "")
		return true, nil
	}

	if s.bootstrapConfig != nil && s.bootstrapConfig.GetDeletionTimestamp().IsZero() {
		if err := r.Client.Delete(ctx, s.bootstrapConfig); err != nil && !apierrors.IsNotFound(err) {
			return false, errors.Wrapf(err,
				"failed to delete %v %q for Machine %q in namespace %q",
				s.bootstrapConfig.GroupVersionKind().Kind, s.bootstrapConfig.GetName(), s.machine.Name, s.machine.Namespace)
		}
	}

	return false, nil
}

func (r *Reconciler) reconcileDeleteInfrastructure(ctx context.Context, s *scope) (bool, error) {
	if s.infraMachine == nil && s.infraMachineIsNotFound {
		v1beta1conditions.MarkFalse(s.machine, clusterv1.InfrastructureReadyV1Beta1Condition, clusterv1.DeletedV1Beta1Reason, clusterv1.ConditionSeverityInfo, "")
		return true, nil
	}

	if s.infraMachine != nil && s.infraMachine.GetDeletionTimestamp().IsZero() {
		if err := r.Client.Delete(ctx, s.infraMachine); err != nil && !apierrors.IsNotFound(err) {
			return false, errors.Wrapf(err,
				"failed to delete %v %q for Machine %q in namespace %q",
				s.infraMachine.GroupVersionKind().Kind, s.infraMachine.GetName(), s.machine.Name, s.machine.Namespace)
		}
	}

	return false, nil
}

// shouldAdopt returns true if the Machine should be adopted as a stand-alone Machine directly owned by the Cluster.
func (r *Reconciler) shouldAdopt(m *clusterv1.Machine) bool {
	// if the machine is controlled by something (MS or KCP), or if it is a stand-alone machine directly owned by the Cluster, then no-op.
	if metav1.GetControllerOf(m) != nil || util.HasOwner(m.GetOwnerReferences(), clusterv1.GroupVersion.String(), []string{"Cluster"}) {
		return false
	}

	// Note: following checks are required because after restore from a backup both the Machine controller and the
	// MachineSet, MachinePool, or ControlPlane controller are racing to adopt Machines, see https://github.com/kubernetes-sigs/cluster-api/issues/7529

	// If the Machine is originated by a MachineSet, it should not be adopted directly by the Cluster as a stand-alone Machine.
	if _, ok := m.Labels[clusterv1.MachineSetNameLabel]; ok {
		return false
	}

	// If the Machine is originated by a MachinePool object, it should not be adopted directly by the Cluster as a stand-alone Machine.
	if _, ok := m.Labels[clusterv1.MachinePoolNameLabel]; ok {
		return false
	}

	// If the Machine is originated by a ControlPlane object, it should not be adopted directly by the Cluster as a stand-alone Machine.
	if _, ok := m.Labels[clusterv1.MachineControlPlaneNameLabel]; ok {
		return false
	}
	return true
}

func (r *Reconciler) watchClusterNodes(ctx context.Context, cluster *clusterv1.Cluster) error {
	log := ctrl.LoggerFrom(ctx)

	if !conditions.IsTrue(cluster, clusterv1.ClusterControlPlaneInitializedCondition) {
		log.V(5).Info("Skipping node watching setup because control plane is not initialized")
		return nil
	}

	return r.ClusterCache.Watch(ctx, util.ObjectKey(cluster), clustercache.NewWatcher(clustercache.WatcherOptions{
		Name:         "machine-watchNodes",
		Watcher:      r.controller,
		Kind:         &corev1.Node{},
		EventHandler: handler.EnqueueRequestsFromMapFunc(r.nodeToMachine),
		Predicates:   []predicate.TypedPredicate[client.Object]{predicates.TypedResourceIsChanged[client.Object](r.Client.Scheme(), *r.predicateLog)},
	}))
}

func (r *Reconciler) nodeToMachine(ctx context.Context, o client.Object) []reconcile.Request {
	node, ok := o.(*corev1.Node)
	if !ok {
		panic(fmt.Sprintf("Expected a Node but got a %T", o))
	}

	var filters []client.ListOption
	// Match by clusterName when the node has the annotation.
	if clusterName, ok := node.GetAnnotations()[clusterv1.ClusterNameAnnotation]; ok {
		filters = append(filters, client.MatchingLabels{
			clusterv1.ClusterNameLabel: clusterName,
		})
	}

	// Match by namespace when the node has the annotation.
	if namespace, ok := node.GetAnnotations()[clusterv1.ClusterNamespaceAnnotation]; ok {
		filters = append(filters, client.InNamespace(namespace))
	}

	// Match by nodeName and status.nodeRef.name.
	machineList := &clusterv1.MachineList{}
	if err := r.Client.List(
		ctx,
		machineList,
		append(filters, client.MatchingFields{index.MachineNodeNameField: node.Name})...); err != nil {
		return nil
	}

	// There should be exactly 1 Machine for the node.
	if len(machineList.Items) == 1 {
		return []reconcile.Request{{NamespacedName: util.ObjectKey(&machineList.Items[0])}}
	}

	// Otherwise let's match by providerID. This is useful when e.g the NodeRef has not been set yet.
	// Match by providerID
	if node.Spec.ProviderID == "" {
		return nil
	}
	machineList = &clusterv1.MachineList{}
	if err := r.Client.List(
		ctx,
		machineList,
		append(filters, client.MatchingFields{index.MachineProviderIDField: node.Spec.ProviderID})...); err != nil {
		return nil
	}

	// There should be exactly 1 Machine for the node.
	if len(machineList.Items) == 1 {
		return []reconcile.Request{{NamespacedName: util.ObjectKey(&machineList.Items[0])}}
	}

	return nil
}

// getAttachedVolumeInformation returns information about volumes attached to the node:
// * VolumesAttached names from node.Status.VolumesAttached.
// * PersistentVolume names from VolumeAttachments with status.Attached set to true.
func getAttachedVolumeInformation(ctx context.Context, remoteClient client.Client, node *corev1.Node) (sets.Set[string], sets.Set[string], error) {
	attachedVolumeName := sets.Set[string]{}
	attachedPVNames := sets.Set[string]{}

	for _, attachedVolume := range node.Status.VolumesAttached {
		attachedVolumeName.Insert(string(attachedVolume.Name))
	}

	if feature.Gates.Enabled(feature.MachineWaitForVolumeDetachConsiderVolumeAttachments) {
		volumeAttachments, err := getVolumeAttachmentForNode(ctx, remoteClient, node.GetName())
		if err != nil {
			return nil, nil, errors.Wrap(err, "failed to list VolumeAttachments")
		}

		for _, va := range volumeAttachments {
			// Return an error if a VolumeAttachments does not refer a PersistentVolume.
			if va.Spec.Source.PersistentVolumeName == nil {
				return nil, nil, errors.Errorf("spec.source.persistentVolumeName for VolumeAttachment %s is not set", va.GetName())
			}
			attachedPVNames.Insert(*va.Spec.Source.PersistentVolumeName)
		}
	}

	return attachedVolumeName, attachedPVNames, nil
}

// getPersistentVolumeClaimsToIgnore gets all pods which have been ignored by drain and returns a list of
// NamespacedNames for all PersistentVolumeClaims referred by the pods.
// Note: this does not require us to list PVC's directly.
func getPersistentVolumeClaimsToIgnore(ctx context.Context, c client.Client, remoteClient client.Client, cluster *clusterv1.Cluster, machine *clusterv1.Machine, nodeName string) (sets.Set[string], error) {
	drainHelper := drain.Helper{
		Client:       c,
		RemoteClient: remoteClient,
	}

	pods, err := drainHelper.GetPodsForEviction(ctx, cluster, machine, nodeName)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find PersistentVolumeClaims from Pods ignored during drain")
	}

	ignoredPods := pods.SkippedPods()

	pvcsToIgnore := sets.Set[string]{}

	for _, pod := range ignoredPods {
		for _, volume := range pod.Spec.Volumes {
			if volume.PersistentVolumeClaim == nil {
				continue
			}

			key := types.NamespacedName{Namespace: pod.GetNamespace(), Name: volume.PersistentVolumeClaim.ClaimName}.String()
			pvcsToIgnore.Insert(key)
		}
	}

	return pvcsToIgnore, nil
}

// getVolumeAttachmentForNode does a paged list of VolumeAttachments and returns a list
// of VolumeAttachments attached to the given node.
func getVolumeAttachmentForNode(ctx context.Context, c client.Client, nodeName string) ([]*storagev1.VolumeAttachment, error) {
	volumeAttachments := []*storagev1.VolumeAttachment{}
	volumeAttachmentList := &storagev1.VolumeAttachmentList{}
	for {
		listOpts := []client.ListOption{
			client.Continue(volumeAttachmentList.GetContinue()),
			client.Limit(100),
		}
		if err := c.List(ctx, volumeAttachmentList, listOpts...); err != nil {
			return nil, errors.Wrap(err, "failed to list VolumeAttachments")
		}

		for _, volumeAttachment := range volumeAttachmentList.Items {
			// Skip VolumeAttachments which are not in attached state.
			if !volumeAttachment.Status.Attached {
				continue
			}
			// Skip VolumeAttachments which are not for the given node.
			if volumeAttachment.Spec.NodeName != nodeName {
				continue
			}
			volumeAttachments = append(volumeAttachments, &volumeAttachment)
		}

		if volumeAttachmentList.GetContinue() == "" {
			break
		}
	}

	return volumeAttachments, nil
}

type attachedVolumeInformation struct {
	// Attached PersistentVolumeClaims filtered by references from pods that don't get drained.
	persistentVolumeClaims []string

	// Attached PersistentVolumes without a corresponding PersistentVolumeClaim.
	persistentVolumesWithoutPVCClaimRef []string

	// Entries in Node.Status.AttachedVolumes[].Name without a corresponding PersistentVolume.
	nodeStatusVolumeNamesWithoutPV []string
	// Names of PersistentVolumes from VolumeAttachments which don't have a corresponding PersistentVolume.
	persistentVolumeNamesWithoutPV []string
}

func (a *attachedVolumeInformation) isEmpty() bool {
	return len(a.persistentVolumeClaims) == 0 &&
		len(a.persistentVolumesWithoutPVCClaimRef) == 0 &&
		len(a.nodeStatusVolumeNamesWithoutPV) == 0 &&
		len(a.persistentVolumeNamesWithoutPV) == 0
}

func (a *attachedVolumeInformation) logKeys() []any {
	logKeys := []any{}
	if len(a.persistentVolumeClaims) > 0 {
		slices.Sort(a.persistentVolumeClaims)
		logKeys = append(logKeys, "PersistentVolumeClaims", clog.StringListToString(a.persistentVolumeClaims))
	}

	if len(a.persistentVolumesWithoutPVCClaimRef) > 0 {
		slices.Sort(a.persistentVolumesWithoutPVCClaimRef)
		logKeys = append(logKeys, "PersistentVolumesWithoutPVCClaimRef", clog.StringListToString(a.persistentVolumesWithoutPVCClaimRef))
	}

	if len(a.nodeStatusVolumeNamesWithoutPV) > 0 {
		slices.Sort(a.nodeStatusVolumeNamesWithoutPV)
		logKeys = append(logKeys, "NodeStatusVolumeNamesWithoutPV", clog.StringListToString(a.nodeStatusVolumeNamesWithoutPV))
	}

	if len(a.persistentVolumeNamesWithoutPV) > 0 {
		slices.Sort(a.persistentVolumeNamesWithoutPV)
		logKeys = append(logKeys, "PersistentVolumeNamesWithoutPV", clog.StringListToString(a.persistentVolumeNamesWithoutPV))
	}

	return logKeys
}

func (a *attachedVolumeInformation) conditionMessage(machine *clusterv1.Machine) string {
	if a.isEmpty() {
		return ""
	}

	conditionMessage := fmt.Sprintf("Waiting for Node volumes to be detached (started at %s)", machine.Status.Deletion.WaitForNodeVolumeDetachStartTime.Format(time.RFC3339))

	if len(a.persistentVolumeClaims) > 0 {
		slices.Sort(a.persistentVolumeClaims)
		conditionMessage = fmt.Sprintf("%s\n* PersistentVolumeClaims: %s", conditionMessage, clog.StringListToString(a.persistentVolumeClaims))
	}

	if len(a.persistentVolumesWithoutPVCClaimRef) > 0 {
		slices.Sort(a.persistentVolumesWithoutPVCClaimRef)
		conditionMessage = fmt.Sprintf("%s\n* PersistentVolumes without a .spec.claimRef to a PersistentVolumeClaim: %s", conditionMessage, clog.StringListToString(a.persistentVolumesWithoutPVCClaimRef))
	}

	if len(a.nodeStatusVolumeNamesWithoutPV) > 0 {
		slices.Sort(a.nodeStatusVolumeNamesWithoutPV)
		conditionMessage = fmt.Sprintf("%s\n* Node with .status.volumesAttached entries not matching a PersistentVolume: %s", conditionMessage, clog.StringListToString(a.nodeStatusVolumeNamesWithoutPV))
	}

	if len(a.persistentVolumeNamesWithoutPV) > 0 {
		slices.Sort(a.persistentVolumeNamesWithoutPV)
		conditionMessage = fmt.Sprintf("%s\n* VolumeAttachment with .spec.source.persistentVolumeName not matching a PersistentVolume: %s", conditionMessage, clog.StringListToString(a.persistentVolumeNamesWithoutPV))
	}

	return conditionMessage
}

// getPersistentVolumesWaitingForDetach returns information about attached volumes
// that correspond either to attachedVolumeNames or attachedPVNames.
// Volumes which refer a PersistentVolumeClaim contained in pvcsToIgnore are filtered out.
func getPersistentVolumesWaitingForDetach(ctx context.Context, c client.Client, attachedNodeVolumeNames, attachedPVNames, pvcsToIgnore sets.Set[string]) (*attachedVolumeInformation, error) {
	attachedPVCs := sets.Set[string]{}
	attachedPVsWithoutPVCClaimRef := []string{}
	foundAttachedNodeVolumeNames := sets.Set[string]{}
	foundAttachedPVNames := sets.Set[string]{}

	// List all PersistentVolumes and preserve the ones we have to wait for.
	// Also store the found VolumeHandles and names of PersistentVolumes to check
	// that all expected PersistentVolumes have been found.
	persistentVolumeList := &corev1.PersistentVolumeList{}
	for {
		listOpts := []client.ListOption{
			client.Continue(persistentVolumeList.GetContinue()),
			client.Limit(100),
		}
		if err := c.List(ctx, persistentVolumeList, listOpts...); err != nil {
			return nil, errors.Wrap(err, "failed to list PersistentVolumes")
		}

		for _, persistentVolume := range persistentVolumeList.Items {
			found := false
			// Lookup if the PersistentVolume matches an entry in attachedVolumeNames.
			if persistentVolume.Spec.CSI != nil {
				attachedVolumeName := fmt.Sprintf("kubernetes.io/csi/%s^%s", persistentVolume.Spec.CSI.Driver, persistentVolume.Spec.CSI.VolumeHandle)
				if attachedNodeVolumeNames.Has(attachedVolumeName) {
					foundAttachedNodeVolumeNames.Insert(attachedVolumeName)
					found = true
				}
			}

			// Lookup if the PersistentVolume matches an entry in attachedPVNames.
			if attachedPVNames.Has(persistentVolume.Name) {
				foundAttachedPVNames.Insert(persistentVolume.Name)
				found = true
			}

			// PersistentVolume which do not match an entry in attachedNodeVolumeNames or
			// attachedPVNames can be ignored.
			if !found {
				continue
			}

			// The ClaimRef should only be nil for unbound volumes and these should not be able to be attached.
			// Also we're unable to map references which are not of Kind PersistentVolumeClaim so we record the PersistentVolume instead.
			if persistentVolume.Spec.ClaimRef == nil || persistentVolume.Spec.ClaimRef.Kind != "PersistentVolumeClaim" {
				attachedPVsWithoutPVCClaimRef = append(attachedPVsWithoutPVCClaimRef, persistentVolume.Name)
				continue
			}

			key := types.NamespacedName{Namespace: persistentVolume.Spec.ClaimRef.Namespace, Name: persistentVolume.Spec.ClaimRef.Name}.String()
			// Add the PersistentVolumeClaim namespaced name to the list we are waiting for being detached.
			attachedPVCs.Insert(key)
		}

		if persistentVolumeList.GetContinue() == "" {
			break
		}
	}

	return &attachedVolumeInformation{
		persistentVolumeClaims:              attachedPVCs.Difference(pvcsToIgnore).UnsortedList(),
		persistentVolumesWithoutPVCClaimRef: attachedPVsWithoutPVCClaimRef,
		nodeStatusVolumeNamesWithoutPV:      attachedNodeVolumeNames.Difference(foundAttachedNodeVolumeNames).UnsortedList(),
		persistentVolumeNamesWithoutPV:      attachedPVNames.Difference(foundAttachedPVNames).UnsortedList(),
	}, nil
}
