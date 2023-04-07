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
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	kubedrain "k8s.io/kubectl/pkg/drain"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/api/v1beta1/index"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/controllers/noderefutil"
	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/cluster-api/internal/util/ssa"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	clog "sigs.k8s.io/cluster-api/util/log"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
)

var (
	errNilNodeRef                 = errors.New("noderef is nil")
	errLastControlPlaneNode       = errors.New("last control plane member")
	errNoControlPlaneNodes        = errors.New("no control plane members")
	errClusterIsBeingDeleted      = errors.New("cluster is being deleted")
	errControlPlaneIsBeingDeleted = errors.New("control plane is being deleted")
)

// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io;bootstrap.cluster.x-k8s.io,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status;machines/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch

// Reconciler reconciles a Machine object.
type Reconciler struct {
	Client                    client.Client
	UnstructuredCachingClient client.Client
	APIReader                 client.Reader
	Tracker                   *remote.ClusterCacheTracker

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string

	// NodeDrainClientTimeout timeout of the client used for draining nodes.
	NodeDrainClientTimeout time.Duration

	controller      controller.Controller
	recorder        record.EventRecorder
	externalTracker external.ObjectTracker

	// nodeDeletionRetryTimeout determines how long the controller will retry deleting a node
	// during a single reconciliation.
	nodeDeletionRetryTimeout time.Duration
	ssaCache                 ssa.Cache
}

func (r *Reconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
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
		WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(ctrl.LoggerFrom(ctx), r.WatchFilterValue)).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(clusterToMachines),
			builder.WithPredicates(
				// TODO: should this wait for Cluster.Status.InfrastructureReady similar to Infra Machine resources?
				predicates.All(ctrl.LoggerFrom(ctx),
					predicates.Any(ctrl.LoggerFrom(ctx),
						predicates.ClusterUnpaused(ctrl.LoggerFrom(ctx)),
						predicates.ClusterControlPlaneInitialized(ctrl.LoggerFrom(ctx)),
					),
					predicates.ResourceHasFilterLabel(ctrl.LoggerFrom(ctx), r.WatchFilterValue),
				),
			)).
		Watches(
			&clusterv1.MachineSet{},
			handler.EnqueueRequestsFromMapFunc(msToMachines),
		).
		Watches(
			&clusterv1.MachineDeployment{},
			handler.EnqueueRequestsFromMapFunc(mdToMachines),
		).
		Build(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	r.controller = c
	r.recorder = mgr.GetEventRecorderFor("machine-controller")
	r.externalTracker = external.ObjectTracker{
		Controller: c,
		Cache:      mgr.GetCache(),
	}
	r.ssaCache = ssa.NewCache()
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

	// AddOwners adds the owners of Machine as k/v pairs to the logger.
	// Specifically, it will add KubeadmControlPlane, MachineSet and MachineDeployment.
	ctx, log, err := clog.AddOwners(ctx, r.Client, m)
	if err != nil {
		return ctrl.Result{}, err
	}

	log = log.WithValues("Cluster", klog.KRef(m.ObjectMeta.Namespace, m.Spec.ClusterName))
	ctx = ctrl.LoggerInto(ctx, log)

	cluster, err := util.GetClusterByName(ctx, r.Client, m.ObjectMeta.Namespace, m.Spec.ClusterName)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to get cluster %q for machine %q in namespace %q",
			m.Spec.ClusterName, m.Name, m.Namespace)
	}

	// Return early if the object or Cluster is paused.
	if annotations.IsPaused(cluster, m) {
		log.Info("Reconciliation is paused for this object")
		return ctrl.Result{}, nil
	}

	// Initialize the patch helper
	patchHelper, err := patch.NewHelper(m, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	defer func() {
		r.reconcilePhase(ctx, m)

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

	// Reconcile labels.
	if m.Labels == nil {
		m.Labels = make(map[string]string)
	}
	m.Labels[clusterv1.ClusterNameLabel] = m.Spec.ClusterName

	// Handle deletion reconciliation loop.
	if !m.ObjectMeta.DeletionTimestamp.IsZero() {
		res, err := r.reconcileDelete(ctx, cluster, m)
		// Requeue if the reconcile failed because the ClusterCacheTracker was locked for
		// the current cluster because of concurrent access.
		if errors.Is(err, remote.ErrClusterLocked) {
			log.V(5).Info("Requeuing because another worker has the lock on the ClusterCacheTracker")
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		return res, err
	}

	// Add finalizer first if not set to avoid the race condition between init and delete.
	// Note: Finalizers in general can only be added when the deletionTimestamp is not set.
	if !controllerutil.ContainsFinalizer(m, clusterv1.MachineFinalizer) {
		controllerutil.AddFinalizer(m, clusterv1.MachineFinalizer)
		return ctrl.Result{}, nil
	}

	// Handle normal reconciliation loop.
	res, err := r.reconcile(ctx, cluster, m)
	// Requeue if the reconcile failed because the ClusterCacheTracker was locked for
	// the current cluster because of concurrent access.
	if errors.Is(err, remote.ErrClusterLocked) {
		log.V(5).Info("Requeuing because another worker has the lock on the ClusterCacheTracker")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}
	return res, err
}

func patchMachine(ctx context.Context, patchHelper *patch.Helper, machine *clusterv1.Machine, options ...patch.Option) error {
	// Always update the readyCondition by summarizing the state of other conditions.
	// A step counter is added to represent progress during the provisioning process (instead we are hiding it
	// after provisioning - e.g. when a MHC condition exists - or during the deletion process).
	conditions.SetSummary(machine,
		conditions.WithConditions(
			// Infrastructure problems should take precedence over all the other conditions
			clusterv1.InfrastructureReadyCondition,
			// Bootstrap comes after, but it is relevant only during initial machine provisioning.
			clusterv1.BootstrapReadyCondition,
			// MHC reported condition should take precedence over the remediation progress
			clusterv1.MachineHealthCheckSucceededCondition,
			clusterv1.MachineOwnerRemediatedCondition,
			clusterv1.DrainingSucceededCondition,
		),
		conditions.WithStepCounterIf(machine.ObjectMeta.DeletionTimestamp.IsZero() && machine.Spec.ProviderID == nil),
		conditions.WithStepCounterIfOnly(
			clusterv1.BootstrapReadyCondition,
			clusterv1.InfrastructureReadyCondition,
		),
	)

	// Patch the object, ignoring conflicts on the conditions owned by this controller.
	// Also, if requested, we are adding additional options like e.g. Patch ObservedGeneration when issuing the
	// patch at the end of the reconcile loop.
	options = append(options,
		patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
			clusterv1.ReadyCondition,
			clusterv1.BootstrapReadyCondition,
			clusterv1.InfrastructureReadyCondition,
			clusterv1.DrainingSucceededCondition,
			clusterv1.MachineHealthCheckSucceededCondition,
			clusterv1.MachineOwnerRemediatedCondition,
		}},
	)

	return patchHelper.Patch(ctx, machine, options...)
}

func (r *Reconciler) reconcile(ctx context.Context, cluster *clusterv1.Cluster, m *clusterv1.Machine) (ctrl.Result, error) {
	// If the machine is a stand-alone one, meaning not originated from a MachineDeployment, then set it as directly
	// owned by the Cluster (if not already present).
	if r.shouldAdopt(m) {
		m.SetOwnerReferences(util.EnsureOwnerRef(m.GetOwnerReferences(), metav1.OwnerReference{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "Cluster",
			Name:       cluster.Name,
			UID:        cluster.UID,
		}))
	}

	phases := []func(context.Context, *scope) (ctrl.Result, error){
		r.reconcileBootstrap,
		r.reconcileInfrastructure,
		r.reconcileNode,
		r.reconcileCertificateExpiry,
	}

	res := ctrl.Result{}
	errs := []error{}
	s := &scope{
		cluster: cluster,
		machine: m,
	}
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
	return res, kerrors.NewAggregate(errs)
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

	// bootstrapConfig is the BootstrapConfig object that is referenced by the
	// Machine. It is set after reconcileBootstrap is called.
	bootstrapConfig *unstructured.Unstructured
}

func (r *Reconciler) reconcileDelete(ctx context.Context, cluster *clusterv1.Cluster, m *clusterv1.Machine) (ctrl.Result, error) { //nolint:gocyclo
	log := ctrl.LoggerFrom(ctx)

	err := r.isDeleteNodeAllowed(ctx, cluster, m)
	isDeleteNodeAllowed := err == nil
	if err != nil {
		switch err {
		case errNoControlPlaneNodes, errLastControlPlaneNode, errNilNodeRef, errClusterIsBeingDeleted, errControlPlaneIsBeingDeleted:
			nodeName := ""
			if m.Status.NodeRef != nil {
				nodeName = m.Status.NodeRef.Name
			}
			log.Info("Skipping deletion of Kubernetes Node associated with Machine as it is not allowed", "Node", klog.KRef("", nodeName), "cause", err.Error())
		default:
			return ctrl.Result{}, errors.Wrapf(err, "failed to check if Kubernetes Node deletion is allowed")
		}
	}

	if isDeleteNodeAllowed {
		// pre-drain.delete lifecycle hook
		// Return early without error, will requeue if/when the hook owner removes the annotation.
		if annotations.HasWithPrefix(clusterv1.PreDrainDeleteHookAnnotationPrefix, m.ObjectMeta.Annotations) {
			conditions.MarkFalse(m, clusterv1.PreDrainDeleteHookSucceededCondition, clusterv1.WaitingExternalHookReason, clusterv1.ConditionSeverityInfo, "")
			return ctrl.Result{}, nil
		}
		conditions.MarkTrue(m, clusterv1.PreDrainDeleteHookSucceededCondition)

		// Drain node before deletion and issue a patch in order to make this operation visible to the users.
		if r.isNodeDrainAllowed(m) {
			patchHelper, err := patch.NewHelper(m, r.Client)
			if err != nil {
				return ctrl.Result{}, err
			}

			log.Info("Draining node", "Node", klog.KRef("", m.Status.NodeRef.Name))
			// The DrainingSucceededCondition never exists before the node is drained for the first time,
			// so its transition time can be used to record the first time draining.
			// This `if` condition prevents the transition time to be changed more than once.
			if conditions.Get(m, clusterv1.DrainingSucceededCondition) == nil {
				conditions.MarkFalse(m, clusterv1.DrainingSucceededCondition, clusterv1.DrainingReason, clusterv1.ConditionSeverityInfo, "Draining the node before deletion")
			}

			if err := patchMachine(ctx, patchHelper, m); err != nil {
				return ctrl.Result{}, errors.Wrap(err, "failed to patch Machine")
			}

			if result, err := r.drainNode(ctx, cluster, m.Status.NodeRef.Name); !result.IsZero() || err != nil {
				if err != nil {
					conditions.MarkFalse(m, clusterv1.DrainingSucceededCondition, clusterv1.DrainingFailedReason, clusterv1.ConditionSeverityWarning, err.Error())
					r.recorder.Eventf(m, corev1.EventTypeWarning, "FailedDrainNode", "error draining Machine's node %q: %v", m.Status.NodeRef.Name, err)
				}
				return result, err
			}

			conditions.MarkTrue(m, clusterv1.DrainingSucceededCondition)
			r.recorder.Eventf(m, corev1.EventTypeNormal, "SuccessfulDrainNode", "success draining Machine's node %q", m.Status.NodeRef.Name)
		}

		// After node draining is completed, and if isNodeVolumeDetachingAllowed returns True, make sure all
		// volumes are detached before proceeding to delete the Node.
		if r.isNodeVolumeDetachingAllowed(m) {
			// The VolumeDetachSucceededCondition never exists before we wait for volume detachment for the first time,
			// so its transition time can be used to record the first time we wait for volume detachment.
			// This `if` condition prevents the transition time to be changed more than once.
			if conditions.Get(m, clusterv1.VolumeDetachSucceededCondition) == nil {
				conditions.MarkFalse(m, clusterv1.VolumeDetachSucceededCondition, clusterv1.WaitingForVolumeDetachReason, clusterv1.ConditionSeverityInfo, "Waiting for node volumes to be detached")
			}

			if ok, err := r.shouldWaitForNodeVolumes(ctx, cluster, m.Status.NodeRef.Name); ok || err != nil {
				if err != nil {
					r.recorder.Eventf(m, corev1.EventTypeWarning, "FailedWaitForVolumeDetach", "error waiting for node volumes detaching, Machine's node %q: %v", m.Status.NodeRef.Name, err)
					return ctrl.Result{}, err
				}
				log.Info("Waiting for node volumes to be detached", "Node", klog.KRef("", m.Status.NodeRef.Name))
				return ctrl.Result{}, nil
			}
			conditions.MarkTrue(m, clusterv1.VolumeDetachSucceededCondition)
			r.recorder.Eventf(m, corev1.EventTypeNormal, "NodeVolumesDetached", "success waiting for node volumes detaching Machine's node %q", m.Status.NodeRef.Name)
		}
	}

	// pre-term.delete lifecycle hook
	// Return early without error, will requeue if/when the hook owner removes the annotation.
	if annotations.HasWithPrefix(clusterv1.PreTerminateDeleteHookAnnotationPrefix, m.ObjectMeta.Annotations) {
		conditions.MarkFalse(m, clusterv1.PreTerminateDeleteHookSucceededCondition, clusterv1.WaitingExternalHookReason, clusterv1.ConditionSeverityInfo, "")
		return ctrl.Result{}, nil
	}
	conditions.MarkTrue(m, clusterv1.PreTerminateDeleteHookSucceededCondition)

	// Return early and don't remove the finalizer if we got an error or
	// the external reconciliation deletion isn't ready.

	patchHelper, err := patch.NewHelper(m, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}
	conditions.MarkFalse(m, clusterv1.MachineNodeHealthyCondition, clusterv1.DeletingReason, clusterv1.ConditionSeverityInfo, "")
	if err := patchMachine(ctx, patchHelper, m); err != nil {
		conditions.MarkFalse(m, clusterv1.MachineNodeHealthyCondition, clusterv1.DeletionFailedReason, clusterv1.ConditionSeverityInfo, "")
		return ctrl.Result{}, errors.Wrap(err, "failed to patch Machine")
	}

	infrastructureDeleted, err := r.reconcileDeleteInfrastructure(ctx, cluster, m)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !infrastructureDeleted {
		log.Info("Waiting for infrastructure to be deleted", m.Spec.InfrastructureRef.Kind, klog.KRef(m.Spec.InfrastructureRef.Namespace, m.Spec.InfrastructureRef.Name))
		return ctrl.Result{}, nil
	}

	bootstrapDeleted, err := r.reconcileDeleteBootstrap(ctx, cluster, m)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !bootstrapDeleted {
		log.Info("Waiting for bootstrap to be deleted", m.Spec.Bootstrap.ConfigRef.Kind, klog.KRef(m.Spec.Bootstrap.ConfigRef.Namespace, m.Spec.Bootstrap.ConfigRef.Name))
		return ctrl.Result{}, nil
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
			conditions.MarkFalse(m, clusterv1.MachineNodeHealthyCondition, clusterv1.DeletionFailedReason, clusterv1.ConditionSeverityWarning, "")
			r.recorder.Eventf(m, corev1.EventTypeWarning, "FailedDeleteNode", "error deleting Machine's node: %v", deleteNodeErr)

			// If the node deletion timeout is not expired yet, requeue the Machine for reconciliation.
			if m.Spec.NodeDeletionTimeout == nil || m.Spec.NodeDeletionTimeout.Nanoseconds() == 0 || m.DeletionTimestamp.Add(m.Spec.NodeDeletionTimeout.Duration).After(time.Now()) {
				return ctrl.Result{}, deleteNodeErr
			}
			log.Info("Node deletion timeout expired, continuing without Node deletion.")
		}
	}

	controllerutil.RemoveFinalizer(m, clusterv1.MachineFinalizer)
	return ctrl.Result{}, nil
}

func (r *Reconciler) isNodeDrainAllowed(m *clusterv1.Machine) bool {
	if _, exists := m.ObjectMeta.Annotations[clusterv1.ExcludeNodeDrainingAnnotation]; exists {
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
	if _, exists := m.ObjectMeta.Annotations[clusterv1.ExcludeWaitForNodeVolumeDetachAnnotation]; exists {
		return false
	}

	if r.nodeVolumeDetachTimeoutExceeded(m) {
		return false
	}

	return true
}

func (r *Reconciler) nodeDrainTimeoutExceeded(machine *clusterv1.Machine) bool {
	// if the NodeDrainTimeout type is not set by user
	if machine.Spec.NodeDrainTimeout == nil || machine.Spec.NodeDrainTimeout.Seconds() <= 0 {
		return false
	}

	// if the draining succeeded condition does not exist
	if conditions.Get(machine, clusterv1.DrainingSucceededCondition) == nil {
		return false
	}

	now := time.Now()
	firstTimeDrain := conditions.GetLastTransitionTime(machine, clusterv1.DrainingSucceededCondition)
	diff := now.Sub(firstTimeDrain.Time)
	return diff.Seconds() >= machine.Spec.NodeDrainTimeout.Seconds()
}

// nodeVolumeDetachTimeoutExceeded returns False if either NodeVolumeDetachTimeout is set to nil or <=0 OR
// VolumeDetachSucceededCondition is not set on the Machine. Otherwise returns true if the timeout is expired
// since the last transition time of VolumeDetachSucceededCondition.
func (r *Reconciler) nodeVolumeDetachTimeoutExceeded(machine *clusterv1.Machine) bool {
	// if the NodeVolumeDetachTimeout type is not set by user
	if machine.Spec.NodeVolumeDetachTimeout == nil || machine.Spec.NodeVolumeDetachTimeout.Seconds() <= 0 {
		return false
	}

	// if the volume detaching succeeded condition does not exist
	if conditions.Get(machine, clusterv1.VolumeDetachSucceededCondition) == nil {
		return false
	}

	now := time.Now()
	firstTimeDetach := conditions.GetLastTransitionTime(machine, clusterv1.VolumeDetachSucceededCondition)
	diff := now.Sub(firstTimeDetach.Time)
	return diff.Seconds() >= machine.Spec.NodeVolumeDetachTimeout.Seconds()
}

// isDeleteNodeAllowed returns nil only if the Machine's NodeRef is not nil
// and if the Machine is not the last control plane node in the cluster.
func (r *Reconciler) isDeleteNodeAllowed(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	log := ctrl.LoggerFrom(ctx)
	// Return early if the cluster is being deleted.
	if !cluster.DeletionTimestamp.IsZero() {
		return errClusterIsBeingDeleted
	}

	// Cannot delete something that doesn't exist.
	if machine.Status.NodeRef == nil {
		return errNilNodeRef
	}

	// controlPlaneRef is an optional field in the Cluster so skip the external
	// managed control plane check if it is nil
	if cluster.Spec.ControlPlaneRef != nil {
		controlPlane, err := external.Get(ctx, r.Client, cluster.Spec.ControlPlaneRef, cluster.Spec.ControlPlaneRef.Namespace)
		if apierrors.IsNotFound(err) {
			// If control plane object in the reference does not exist, log and skip check for
			// external managed control plane
			log.Error(err, "control plane object specified in cluster spec.controlPlaneRef does not exist", "kind", cluster.Spec.ControlPlaneRef.Kind, "name", cluster.Spec.ControlPlaneRef.Name)
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

func (r *Reconciler) drainNode(ctx context.Context, cluster *clusterv1.Cluster, nodeName string) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx, "Node", klog.KRef("", nodeName))

	restConfig, err := r.Tracker.GetRESTConfig(ctx, util.ObjectKey(cluster))
	if err != nil {
		if errors.Is(err, remote.ErrClusterLocked) {
			log.V(5).Info("Requeuing drain Node because another worker has the lock on the ClusterCacheTracker")
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		log.Error(err, "Error creating a remote client for cluster while draining Node, won't retry")
		return ctrl.Result{}, nil
	}
	restConfig = rest.CopyConfig(restConfig)
	restConfig.Timeout = r.NodeDrainClientTimeout
	kubeClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		log.Error(err, "Error creating a remote client while deleting Machine, won't retry")
		return ctrl.Result{}, nil
	}

	node, err := kubeClient.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			// If an admin deletes the node directly, we'll end up here.
			log.Error(err, "Could not find node from noderef, it may have already been deleted")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, errors.Wrapf(err, "unable to get node %v", nodeName)
	}

	drainer := &kubedrain.Helper{
		Client:              kubeClient,
		Ctx:                 ctx,
		Force:               true,
		IgnoreAllDaemonSets: true,
		DeleteEmptyDirData:  true,
		GracePeriodSeconds:  -1,
		// If a pod is not evicted in 20 seconds, retry the eviction next time the
		// machine gets reconciled again (to allow other machines to be reconciled).
		Timeout: 20 * time.Second,
		OnPodDeletedOrEvicted: func(pod *corev1.Pod, usingEviction bool) {
			verbStr := "Deleted"
			if usingEviction {
				verbStr = "Evicted"
			}
			log.Info(fmt.Sprintf("%s pod from Node", verbStr),
				"Pod", klog.KObj(pod))
		},
		Out: writer{log.Info},
		ErrOut: writer{func(msg string, keysAndValues ...interface{}) {
			log.Error(nil, msg, keysAndValues...)
		}},
	}

	if noderefutil.IsNodeUnreachable(node) {
		// When the node is unreachable and some pods are not evicted for as long as this timeout, we ignore them.
		drainer.SkipWaitForDeleteTimeoutSeconds = 60 * 5 // 5 minutes
	}

	if err := kubedrain.RunCordonOrUncordon(drainer, node, true); err != nil {
		// Machine will be re-reconciled after a cordon failure.
		log.Error(err, "Cordon failed")
		return ctrl.Result{}, errors.Wrapf(err, "unable to cordon node %v", node.Name)
	}

	if err := kubedrain.RunNodeDrain(drainer, node.Name); err != nil {
		// Machine will be re-reconciled after a drain failure.
		log.Error(err, "Drain failed, retry in 20s")
		return ctrl.Result{RequeueAfter: 20 * time.Second}, nil
	}

	log.Info("Drain successful")
	return ctrl.Result{}, nil
}

// shouldWaitForNodeVolumes returns true if node status still have volumes attached
// pod deletion and volume detach happen asynchronously, so pod could be deleted before volume detached from the node
// this could cause issue for some storage provisioner, for example, vsphere-volume this is problematic
// because if the node is deleted before detach success, then the underline VMDK will be deleted together with the Machine
// so after node draining we need to check if all volumes are detached before deleting the node.
func (r *Reconciler) shouldWaitForNodeVolumes(ctx context.Context, cluster *clusterv1.Cluster, nodeName string) (bool, error) {
	log := ctrl.LoggerFrom(ctx, "Node", klog.KRef("", nodeName))

	remoteClient, err := r.Tracker.GetClient(ctx, util.ObjectKey(cluster))
	if err != nil {
		return true, err
	}

	node := &corev1.Node{}
	if err := remoteClient.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
		if apierrors.IsNotFound(err) {
			log.Error(err, "Could not find node from noderef, it may have already been deleted")
			return false, nil
		}
		return true, err
	}

	return len(node.Status.VolumesAttached) != 0, nil
}

func (r *Reconciler) deleteNode(ctx context.Context, cluster *clusterv1.Cluster, name string) error {
	log := ctrl.LoggerFrom(ctx)

	remoteClient, err := r.Tracker.GetClient(ctx, util.ObjectKey(cluster))
	if err != nil {
		if errors.Is(err, remote.ErrClusterLocked) {
			return errors.Wrapf(err, "failed deleting Node because another worker has the lock on the ClusterCacheTracker")
		}
		log.Error(err, "Error creating a remote client for cluster while deleting Node, won't retry")
		return nil
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

func (r *Reconciler) reconcileDeleteBootstrap(ctx context.Context, cluster *clusterv1.Cluster, m *clusterv1.Machine) (bool, error) {
	obj, err := r.reconcileDeleteExternal(ctx, cluster, m, m.Spec.Bootstrap.ConfigRef)
	if err != nil {
		return false, err
	}

	if obj == nil {
		// Marks the bootstrap as deleted
		conditions.MarkFalse(m, clusterv1.BootstrapReadyCondition, clusterv1.DeletedReason, clusterv1.ConditionSeverityInfo, "")
		return true, nil
	}

	// Report a summary of current status of the bootstrap object defined for this machine.
	conditions.SetMirror(m, clusterv1.BootstrapReadyCondition,
		conditions.UnstructuredGetter(obj),
		conditions.WithFallbackValue(false, clusterv1.DeletingReason, clusterv1.ConditionSeverityInfo, ""),
	)
	return false, nil
}

func (r *Reconciler) reconcileDeleteInfrastructure(ctx context.Context, cluster *clusterv1.Cluster, m *clusterv1.Machine) (bool, error) {
	obj, err := r.reconcileDeleteExternal(ctx, cluster, m, &m.Spec.InfrastructureRef)
	if err != nil {
		return false, err
	}

	if obj == nil {
		// Marks the infrastructure as deleted
		conditions.MarkFalse(m, clusterv1.InfrastructureReadyCondition, clusterv1.DeletedReason, clusterv1.ConditionSeverityInfo, "")
		return true, nil
	}

	// Report a summary of current status of the bootstrap object defined for this machine.
	conditions.SetMirror(m, clusterv1.InfrastructureReadyCondition,
		conditions.UnstructuredGetter(obj),
		conditions.WithFallbackValue(false, clusterv1.DeletingReason, clusterv1.ConditionSeverityInfo, ""),
	)
	return false, nil
}

// reconcileDeleteExternal tries to delete external references.
func (r *Reconciler) reconcileDeleteExternal(ctx context.Context, cluster *clusterv1.Cluster, m *clusterv1.Machine, ref *corev1.ObjectReference) (*unstructured.Unstructured, error) {
	if ref == nil {
		return nil, nil
	}

	// get the external object
	obj, err := external.Get(ctx, r.UnstructuredCachingClient, ref, m.Namespace)
	if err != nil && !apierrors.IsNotFound(errors.Cause(err)) {
		return nil, errors.Wrapf(err, "failed to get %s %q for Machine %q in namespace %q",
			ref.GroupVersionKind(), ref.Name, m.Name, m.Namespace)
	}

	if obj != nil {
		// reconcileExternal ensures that we set the object's OwnerReferences correctly and watch the object.
		// The machine delete logic depends on reconciling the machine when the external objects are deleted.
		// This avoids a race condition where the machine is deleted before the external objects are ever reconciled
		// by this controller.
		if _, err := r.ensureExternalOwnershipAndWatch(ctx, cluster, m, ref); err != nil {
			return nil, err
		}

		// Issue a delete request.
		if err := r.Client.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
			return obj, errors.Wrapf(err,
				"failed to delete %v %q for Machine %q in namespace %q",
				obj.GroupVersionKind(), obj.GetName(), m.Name, m.Namespace)
		}
	}

	// Return true if there are no more external objects.
	return obj, nil
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

	if !conditions.IsTrue(cluster, clusterv1.ControlPlaneInitializedCondition) {
		log.V(5).Info("Skipping node watching setup because control plane is not initialized")
		return nil
	}

	// If there is no tracker, don't watch remote nodes
	if r.Tracker == nil {
		return nil
	}

	return r.Tracker.Watch(ctx, remote.WatchInput{
		Name:         "machine-watchNodes",
		Cluster:      util.ObjectKey(cluster),
		Watcher:      r.controller,
		Kind:         &corev1.Node{},
		EventHandler: handler.EnqueueRequestsFromMapFunc(r.nodeToMachine),
	})
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

// writer implements io.Writer interface as a pass-through for klog.
type writer struct {
	logFunc func(msg string, keysAndValues ...interface{})
}

// Write passes string(p) into writer's logFunc and always returns len(p).
func (w writer) Write(p []byte) (n int, err error) {
	w.logFunc(string(p))
	return len(p), nil
}
