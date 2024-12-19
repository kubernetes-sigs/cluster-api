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

package cluster

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	"sigs.k8s.io/cluster-api/controllers/external"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/hooks"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta2conditions "sigs.k8s.io/cluster-api/util/conditions/v1beta2"
	"sigs.k8s.io/cluster-api/util/finalizers"
	clog "sigs.k8s.io/cluster-api/util/log"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/paused"
	"sigs.k8s.io/cluster-api/util/predicates"
)

const (
	// deleteRequeueAfter is how long to wait before checking again to see if the cluster still has children during
	// deletion.
	deleteRequeueAfter = 5 * time.Second
)

// Update permissions on /finalizers subresrouce is required on management clusters with 'OwnerReferencesPermissionEnforcement' plugin enabled.
// See: https://kubernetes.io/docs/reference/access-authn-authz/admission-controllers/#ownerreferencespermissionenforcement
//
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;patch;update
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io;bootstrap.cluster.x-k8s.io;controlplane.cluster.x-k8s.io,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status;clusters/finalizers,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch

// Reconciler reconciles a Cluster object.
type Reconciler struct {
	Client       client.Client
	APIReader    client.Reader
	ClusterCache clustercache.ClusterCache

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string

	RemoteConnectionGracePeriod time.Duration

	recorder        record.EventRecorder
	externalTracker external.ObjectTracker
}

func (r *Reconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	if r.Client == nil || r.APIReader == nil || r.ClusterCache == nil || r.RemoteConnectionGracePeriod == time.Duration(0) {
		return errors.New("Client, APIReader and ClusterCache must not be nil and RemoteConnectionGracePeriod must not be 0")
	}

	predicateLog := ctrl.LoggerFrom(ctx).WithValues("controller", "cluster")
	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.Cluster{}).
		WatchesRawSource(r.ClusterCache.GetClusterSource("cluster", func(_ context.Context, o client.Object) []ctrl.Request {
			return []ctrl.Request{{NamespacedName: client.ObjectKeyFromObject(o)}}
		}, clustercache.WatchForProbeFailure(r.RemoteConnectionGracePeriod))).
		Watches(
			&clusterv1.Machine{},
			handler.EnqueueRequestsFromMapFunc(r.controlPlaneMachineToCluster),
			builder.WithPredicates(predicates.ResourceIsChanged(mgr.GetScheme(), predicateLog)),
		).
		Watches(
			&clusterv1.MachineDeployment{},
			handler.EnqueueRequestsFromMapFunc(r.machineDeploymentToCluster),
			builder.WithPredicates(predicates.ResourceIsChanged(mgr.GetScheme(), predicateLog)),
		).
		Watches(
			&expv1.MachinePool{},
			handler.EnqueueRequestsFromMapFunc(r.machinePoolToCluster),
			builder.WithPredicates(predicates.ResourceIsChanged(mgr.GetScheme(), predicateLog)),
		).
		WithOptions(options).
		WithEventFilter(predicates.ResourceHasFilterLabel(mgr.GetScheme(), predicateLog, r.WatchFilterValue)).
		Build(r)

	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	r.recorder = mgr.GetEventRecorderFor("cluster-controller")
	r.externalTracker = external.ObjectTracker{
		Controller:      c,
		Cache:           mgr.GetCache(),
		Scheme:          mgr.GetScheme(),
		PredicateLogger: &predicateLog,
	}
	return nil
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (retRes ctrl.Result, reterr error) {
	log := ctrl.LoggerFrom(ctx)

	// Fetch the Cluster instance.
	cluster := &clusterv1.Cluster{}
	if err := r.Client.Get(ctx, req.NamespacedName, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}

		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	// Add finalizer first if not set to avoid the race condition between init and delete.
	if finalizerAdded, err := finalizers.EnsureFinalizer(ctx, r.Client, cluster, clusterv1.ClusterFinalizer); err != nil || finalizerAdded {
		return ctrl.Result{}, err
	}

	if isPaused, conditionChanged, err := paused.EnsurePausedCondition(ctx, r.Client, cluster, cluster); err != nil || isPaused || conditionChanged {
		return ctrl.Result{}, err
	}

	s := &scope{
		cluster: cluster,
	}

	// Initialize the patch helper.
	patchHelper, err := patch.NewHelper(cluster, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	defer func() {
		// Always reconcile the Status.
		r.updateStatus(ctx, s)

		// Always attempt to Patch the Cluster object and status after each reconciliation.
		// Patch ObservedGeneration only if the reconciliation completed successfully
		patchOpts := []patch.Option{}
		if reterr == nil {
			patchOpts = append(patchOpts, patch.WithStatusObservedGeneration{})
		}
		if err := patchCluster(ctx, patchHelper, cluster, patchOpts...); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}

		if reterr != nil {
			retRes = ctrl.Result{}
		}
	}()

	lastProbeSuccessTime := r.ClusterCache.GetLastProbeSuccessTimestamp(ctx, client.ObjectKeyFromObject(cluster))
	if time.Since(lastProbeSuccessTime) > r.RemoteConnectionGracePeriod {
		var msg string
		if lastProbeSuccessTime.IsZero() {
			msg = "Remote connection probe failed"
		} else {
			msg = fmt.Sprintf("Remote connection probe failed, probe last succeeded at %s", lastProbeSuccessTime.Format(time.RFC3339))
		}
		v1beta2conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterRemoteConnectionProbeV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.ClusterRemoteConnectionProbeFailedV1Beta2Reason,
			Message: msg,
		})
	} else {
		v1beta2conditions.Set(cluster, metav1.Condition{
			Type:   clusterv1.ClusterRemoteConnectionProbeV1Beta2Condition,
			Status: metav1.ConditionTrue,
			Reason: clusterv1.ClusterRemoteConnectionProbeSucceededV1Beta2Reason,
		})
	}

	alwaysReconcile := []clusterReconcileFunc{
		r.reconcileInfrastructure,
		r.reconcileControlPlane,
		r.getDescendants,
	}

	// Handle deletion reconciliation loop.
	if !cluster.ObjectMeta.DeletionTimestamp.IsZero() {
		reconcileDelete := append(
			alwaysReconcile,
			r.reconcileDelete,
		)

		return doReconcile(ctx, reconcileDelete, s)
	}

	// Handle normal reconciliation loop.
	if cluster.Spec.Topology != nil {
		if cluster.Spec.ControlPlaneRef == nil || cluster.Spec.InfrastructureRef == nil {
			// TODO: add a condition to surface this scenario
			log.Info("Waiting for the topology to be generated")
			return ctrl.Result{}, nil
		}
	}

	reconcileNormal := append(
		alwaysReconcile,
		r.reconcileKubeconfig,
		r.reconcileControlPlaneInitialized,
	)
	return doReconcile(ctx, reconcileNormal, s)
}

func patchCluster(ctx context.Context, patchHelper *patch.Helper, cluster *clusterv1.Cluster, options ...patch.Option) error {
	// Always update the readyCondition by summarizing the state of other conditions.
	conditions.SetSummary(cluster,
		conditions.WithConditions(
			clusterv1.ControlPlaneReadyCondition,
			clusterv1.InfrastructureReadyCondition,
		),
	)

	// Patch the object, ignoring conflicts on the conditions owned by this controller.
	// Also, if requested, we are adding additional options like e.g. Patch ObservedGeneration when issuing the
	// patch at the end of the reconcile loop.
	options = append(options,
		patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
			clusterv1.ReadyCondition,
			clusterv1.ControlPlaneReadyCondition,
			clusterv1.InfrastructureReadyCondition,
		}},
		patch.WithOwnedV1Beta2Conditions{Conditions: []string{
			clusterv1.ClusterInfrastructureReadyV1Beta2Condition,
			clusterv1.ClusterControlPlaneAvailableV1Beta2Condition,
			clusterv1.ClusterControlPlaneInitializedV1Beta2Condition,
			clusterv1.ClusterControlPlaneMachinesReadyV1Beta2Condition,
			clusterv1.ClusterControlPlaneMachinesUpToDateV1Beta2Condition,
			clusterv1.ClusterWorkersAvailableV1Beta2Condition,
			clusterv1.ClusterWorkerMachinesReadyV1Beta2Condition,
			clusterv1.ClusterWorkerMachinesUpToDateV1Beta2Condition,
			clusterv1.ClusterRemoteConnectionProbeV1Beta2Condition,
			clusterv1.ClusterRollingOutV1Beta2Condition,
			clusterv1.ClusterScalingUpV1Beta2Condition,
			clusterv1.ClusterScalingDownV1Beta2Condition,
			clusterv1.ClusterRemediatingV1Beta2Condition,
			clusterv1.ClusterDeletingV1Beta2Condition,
			clusterv1.ClusterAvailableV1Beta2Condition,
		}},
	)
	return patchHelper.Patch(ctx, cluster, options...)
}

type clusterReconcileFunc func(context.Context, *scope) (ctrl.Result, error)

func doReconcile(ctx context.Context, phases []clusterReconcileFunc, s *scope) (ctrl.Result, error) {
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
	// cluster is the Cluster object being reconciled.
	// It is set at the beginning of the reconcile function.
	cluster *clusterv1.Cluster

	// infraCluster is the Infrastructure Cluster object that is referenced by the
	// Cluster. It is set after reconcileInfrastructure is called.
	infraCluster *unstructured.Unstructured

	// infraClusterNotFound is true if getting the infra cluster object failed with an NotFound err
	infraClusterIsNotFound bool

	// controlPlane is the ControlPlane object that is referenced by the
	// Cluster. It is set after reconcileControlPlane is called.
	controlPlane *unstructured.Unstructured

	// controlPlaneNotFound is true if getting the ControlPlane object failed with an NotFound err
	controlPlaneIsNotFound bool

	// descendants is the list of objects related to this Cluster
	descendants clusterDescendants

	// getDescendantsSucceeded documents if getDescendants succeeded.
	getDescendantsSucceeded bool

	// deletingReason is the reason that should be used when setting the Deleting condition.
	deletingReason string

	// deletingMessage is the message that should be used when setting the Deleting condition.
	deletingMessage string
}

// reconcileDelete handles cluster deletion.
func (r *Reconciler) reconcileDelete(ctx context.Context, s *scope) (reconcile.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	cluster := s.cluster

	// If the RuntimeSDK and ClusterTopology flags are enabled, for clusters with managed topologies
	// only proceed with delete if the cluster is marked as `ok-to-delete`
	if feature.Gates.Enabled(feature.RuntimeSDK) && feature.Gates.Enabled(feature.ClusterTopology) {
		if cluster.Spec.Topology != nil && !hooks.IsOkToDelete(cluster) {
			s.deletingReason = clusterv1.ClusterDeletingWaitingForBeforeDeleteHookV1Beta2Reason
			s.deletingMessage = "Waiting for BeforeClusterDelete hook"
			return ctrl.Result{}, nil
		}
	}

	// If it failed to get descendants, no-op.
	if !s.getDescendantsSucceeded {
		s.deletingReason = clusterv1.ClusterDeletingInternalErrorV1Beta2Reason
		s.deletingMessage = "Please check controller logs for errors" //nolint:goconst // Not making this a constant for now
		return reconcile.Result{}, nil
	}

	children, err := s.descendants.filterOwnedDescendants(cluster)
	if err != nil {
		s.deletingReason = clusterv1.ClusterDeletingInternalErrorV1Beta2Reason
		s.deletingMessage = "Please check controller logs for errors"
		return reconcile.Result{}, errors.Wrapf(err, "failed to extract direct descendants")
	}

	if len(children) > 0 {
		log.Info("Cluster still has children - deleting them first", "count", len(children))

		var errs []error

		for _, child := range children {
			if !child.GetDeletionTimestamp().IsZero() {
				// Don't handle deleted child
				continue
			}

			gvk, err := apiutil.GVKForObject(child, r.Client.Scheme())
			if err != nil {
				errs = append(errs, errors.Wrapf(err, "error getting gvk for child object"))
				continue
			}

			log := log.WithValues(gvk.Kind, klog.KObj(child))
			log.Info("Deleting child object")
			if err := r.Client.Delete(ctx, child); err != nil {
				err = errors.Wrapf(err, "error deleting cluster %s/%s: failed to delete %s %s", cluster.Namespace, cluster.Name, gvk, child.GetName())
				log.Error(err, "Error deleting resource")
				errs = append(errs, err)
			}
		}

		if len(errs) > 0 {
			s.deletingReason = clusterv1.ClusterDeletingInternalErrorV1Beta2Reason
			s.deletingMessage = "Please check controller logs for errors"
			return ctrl.Result{}, kerrors.NewAggregate(errs)
		}
	}

	if descendantCount := s.descendants.objectsPendingDeleteCount(cluster); descendantCount > 0 {
		indirect := descendantCount - len(children)
		names := s.descendants.objectsPendingDeleteNames(cluster)

		log.Info("Cluster still has descendants - need to requeue", "descendants", strings.Join(names, "; "), "indirect descendants count", indirect)

		s.deletingReason = clusterv1.ClusterDeletingWaitingForWorkersDeletionV1Beta2Reason
		for i := range names {
			names[i] = "* " + names[i]
		}
		s.deletingMessage = strings.Join(names, "\n")

		// Requeue so we can check the next time to see if there are still any descendants left.
		return ctrl.Result{RequeueAfter: deleteRequeueAfter}, nil
	}

	if cluster.Spec.ControlPlaneRef != nil {
		if s.controlPlane == nil {
			if !s.controlPlaneIsNotFound {
				// In case there was a generic error (different than isNotFound) in reading the InfraCluster, do not continue with deletion.
				// Note: this error surfaces in reconcile reconcileControlPlane.
				s.deletingReason = clusterv1.ClusterDeletingInternalErrorV1Beta2Reason
				s.deletingMessage = "Please check controller logs for errors"

				return ctrl.Result{}, nil
			}
			// All good - the control plane resource has been deleted
			conditions.MarkFalse(cluster, clusterv1.ControlPlaneReadyCondition, clusterv1.DeletedReason, clusterv1.ConditionSeverityInfo, "")
		}

		if s.controlPlane != nil {
			if s.controlPlane.GetDeletionTimestamp().IsZero() {
				// Issue a deletion request for the control plane object.
				// Once it's been deleted, the cluster will get processed again.
				if err := r.Client.Delete(ctx, s.controlPlane); err != nil {
					if !apierrors.IsNotFound(err) {
						s.deletingReason = clusterv1.ClusterDeletingInternalErrorV1Beta2Reason
						s.deletingMessage = "Please check controller logs for errors"
						return ctrl.Result{}, errors.Wrapf(err,
							"failed to delete %v %q for Cluster %q in namespace %q",
							s.controlPlane.GroupVersionKind().Kind, s.controlPlane.GetName(), cluster.Name, cluster.Namespace)
					}
				}
			}

			// Return here so we don't remove the finalizer yet.
			s.deletingReason = clusterv1.ClusterDeletingWaitingForControlPlaneDeletionV1Beta2Reason
			s.deletingMessage = fmt.Sprintf("Waiting for %s to be deleted", cluster.Spec.ControlPlaneRef.Kind)

			log.Info("Cluster still has descendants - need to requeue", "controlPlaneRef", cluster.Spec.ControlPlaneRef.Name)
			return ctrl.Result{}, nil
		}
	}

	if cluster.Spec.InfrastructureRef != nil {
		if s.infraCluster == nil {
			if !s.infraClusterIsNotFound {
				// In case there was a generic error (different than isNotFound) in reading the InfraCluster, do not continue with deletion.
				// Note: this error surfaces in reconcile reconcileInfrastructure.
				s.deletingReason = clusterv1.ClusterDeletingInternalErrorV1Beta2Reason
				s.deletingMessage = "Please check controller logs for errors"

				return ctrl.Result{}, nil
			}
			// All good - the infra resource has been deleted
			conditions.MarkFalse(cluster, clusterv1.InfrastructureReadyCondition, clusterv1.DeletedReason, clusterv1.ConditionSeverityInfo, "")
		}

		if s.infraCluster != nil {
			if s.infraCluster.GetDeletionTimestamp().IsZero() {
				// Issue a deletion request for the infrastructure object.
				// Once it's been deleted, the cluster will get processed again.
				if err := r.Client.Delete(ctx, s.infraCluster); err != nil {
					if !apierrors.IsNotFound(err) {
						s.deletingReason = clusterv1.ClusterDeletingInternalErrorV1Beta2Reason
						s.deletingMessage = "Please check controller logs for errors"
						return ctrl.Result{}, errors.Wrapf(err,
							"failed to delete %v %q for Cluster %q in namespace %q",
							s.infraCluster.GroupVersionKind().Kind, s.infraCluster.GetName(), cluster.Name, cluster.Namespace)
					}
				}
			}

			// Return here so we don't remove the finalizer yet.
			s.deletingReason = clusterv1.ClusterDeletingWaitingForInfrastructureDeletionV1Beta2Reason
			s.deletingMessage = fmt.Sprintf("Waiting for %s to be deleted", cluster.Spec.InfrastructureRef.Kind)

			log.Info("Cluster still has descendants - need to requeue", "infrastructureRef", cluster.Spec.InfrastructureRef.Name)
			return ctrl.Result{}, nil
		}
	}

	s.deletingReason = clusterv1.ClusterDeletingDeletionCompletedV1Beta2Reason
	s.deletingMessage = "Deletion completed"

	controllerutil.RemoveFinalizer(cluster, clusterv1.ClusterFinalizer)
	r.recorder.Eventf(cluster, corev1.EventTypeNormal, "Deleted", "Cluster %s has been deleted", cluster.Name)
	return ctrl.Result{}, nil
}

type clusterDescendants struct {
	machineDeployments     clusterv1.MachineDeploymentList
	machineSets            clusterv1.MachineSetList
	allMachines            collections.Machines
	controlPlaneMachines   collections.Machines
	workerMachines         collections.Machines
	machinesToBeRemediated collections.Machines
	unhealthyMachines      collections.Machines
	machinePools           expv1.MachinePoolList
}

// objectsPendingDeleteCount returns the number of descendants pending delete.
// Note: infrastructure cluster, control plane object and its controlled machines are not included.
func (c *clusterDescendants) objectsPendingDeleteCount(cluster *clusterv1.Cluster) int {
	n := len(c.machinePools.Items) +
		len(c.machineDeployments.Items) +
		len(c.machineSets.Items) +
		len(c.workerMachines)

	if cluster.Spec.ControlPlaneRef == nil {
		n += len(c.controlPlaneMachines)
	}

	return n
}

// objectsPendingDeleteNames return the names of descendants pending delete.
// Note: infrastructure cluster, control plane object and its controlled machines are not included.
func (c *clusterDescendants) objectsPendingDeleteNames(cluster *clusterv1.Cluster) []string {
	descendants := make([]string, 0)
	if cluster.Spec.ControlPlaneRef == nil {
		controlPlaneMachineNames := make([]string, len(c.controlPlaneMachines))
		for i, controlPlaneMachine := range c.controlPlaneMachines.UnsortedList() {
			controlPlaneMachineNames[i] = controlPlaneMachine.Name
		}
		if len(controlPlaneMachineNames) > 0 {
			sort.Strings(controlPlaneMachineNames)
			descendants = append(descendants, "Control plane Machines: "+clog.StringListToString(controlPlaneMachineNames))
		}
	}
	machineDeploymentNames := make([]string, len(c.machineDeployments.Items))
	for i, machineDeployment := range c.machineDeployments.Items {
		machineDeploymentNames[i] = machineDeployment.Name
	}
	if len(machineDeploymentNames) > 0 {
		sort.Strings(machineDeploymentNames)
		descendants = append(descendants, "MachineDeployments: "+clog.StringListToString(machineDeploymentNames))
	}
	machineSetNames := make([]string, len(c.machineSets.Items))
	for i, machineSet := range c.machineSets.Items {
		machineSetNames[i] = machineSet.Name
	}
	if len(machineSetNames) > 0 {
		sort.Strings(machineSetNames)
		descendants = append(descendants, "MachineSets: "+clog.StringListToString(machineSetNames))
	}
	if feature.Gates.Enabled(feature.MachinePool) {
		machinePoolNames := make([]string, len(c.machinePools.Items))
		for i, machinePool := range c.machinePools.Items {
			machinePoolNames[i] = machinePool.Name
		}
		if len(machinePoolNames) > 0 {
			sort.Strings(machinePoolNames)
			descendants = append(descendants, "MachinePools: "+clog.StringListToString(machinePoolNames))
		}
	}
	workerMachineNames := make([]string, len(c.workerMachines))
	for i, workerMachine := range c.workerMachines.UnsortedList() {
		workerMachineNames[i] = workerMachine.Name
	}
	if len(workerMachineNames) > 0 {
		sort.Strings(workerMachineNames)
		descendants = append(descendants, "Worker Machines: "+clog.StringListToString(workerMachineNames))
	}
	return descendants
}

// getDescendants collects all MachineDeployments, MachineSets, MachinePools and Machines for the cluster.
func (r *Reconciler) getDescendants(ctx context.Context, s *scope) (reconcile.Result, error) {
	var descendants clusterDescendants
	cluster := s.cluster

	listOptions := []client.ListOption{
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(map[string]string{clusterv1.ClusterNameLabel: cluster.Name}),
	}

	if err := r.Client.List(ctx, &descendants.machineDeployments, listOptions...); err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to list MachineDeployments for cluster %s/%s", cluster.Namespace, cluster.Name)
	}

	if err := r.Client.List(ctx, &descendants.machineSets, listOptions...); err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to list MachineSets for cluster %s/%s", cluster.Namespace, cluster.Name)
	}

	if feature.Gates.Enabled(feature.MachinePool) {
		if err := r.Client.List(ctx, &descendants.machinePools, listOptions...); err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "failed to list MachinePools for the cluster %s/%s", cluster.Namespace, cluster.Name)
		}
	}
	var machines clusterv1.MachineList
	if err := r.Client.List(ctx, &machines, listOptions...); err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to list Machines for cluster %s/%s", cluster.Namespace, cluster.Name)
	}

	// Split machines into control plane and worker machines
	descendants.allMachines = collections.FromMachineList(&machines)
	descendants.controlPlaneMachines = descendants.allMachines.Filter(collections.ControlPlaneMachines(cluster.Name))
	descendants.workerMachines = descendants.allMachines.Difference(descendants.controlPlaneMachines)
	descendants.machinesToBeRemediated = descendants.allMachines.Filter(collections.IsUnhealthyAndOwnerRemediated)
	descendants.unhealthyMachines = descendants.allMachines.Filter(collections.IsUnhealthy)

	s.descendants = descendants
	s.getDescendantsSucceeded = true

	return reconcile.Result{}, nil
}

// filterOwnedDescendants returns an array of runtime.Objects containing only those descendants that have the cluster
// as an owner reference, with control plane machines sorted last.
// Note: this list must include stand-alone MachineSets and stand-alone Machines; instead MachineSets or Machines controlled
// by higher level abstractions like e.g. MachineDeployment are not be included (if owner references are properly set on those machines).
func (c *clusterDescendants) filterOwnedDescendants(cluster *clusterv1.Cluster) ([]client.Object, error) {
	var ownedDescendants []client.Object
	eachFunc := func(o runtime.Object) error {
		obj := o.(client.Object)
		acc, err := meta.Accessor(obj)
		if err != nil {
			return nil //nolint:nilerr // We don't want to exit the EachListItem loop, just continue
		}

		if util.IsOwnedByObject(acc, cluster) {
			ownedDescendants = append(ownedDescendants, obj)
		}

		return nil
	}

	toObjectList := func(c collections.Machines) client.ObjectList {
		l := &clusterv1.MachineList{}
		for _, m := range c.UnsortedList() {
			l.Items = append(l.Items, *m)
		}
		sort.Slice(l.Items, func(i, j int) bool {
			return l.Items[i].Name < l.Items[j].Name
		})
		return l
	}

	lists := []client.ObjectList{
		&c.machineDeployments,
		&c.machineSets,
		toObjectList(c.workerMachines),
	}
	if feature.Gates.Enabled(feature.MachinePool) {
		lists = append([]client.ObjectList{&c.machinePools}, lists...)
	}

	// Make sure that control plane machines are included only if there is no control plane object
	// responsible to manage them.
	// Note: Excluding machines controlled by a control plane object is an additional safeguard to ensure
	// that the control plane is deleted only after all the workers machine are done.
	// Note: Using stand-alone control plane machines is not yet officially deprecated, however this approach
	// has well known limitations that have been address by the introduction of control plane objects. One of those
	// limitation is about the deletion workflow, which is governed by this function.
	// More specifically, when deleting a Cluster with stand-alone control plane machines, stand-alone control plane machines
	// will be decommissioned in parallel with other machines, no matter of them being added as last in this list.
	if cluster.Spec.ControlPlaneRef == nil {
		lists = append(lists, toObjectList(c.controlPlaneMachines))
	}

	for _, list := range lists {
		if err := meta.EachListItem(list, eachFunc); err != nil {
			return nil, errors.Wrapf(err, "error finding owned descendants of cluster %s/%s", cluster.Namespace, cluster.Name)
		}
	}

	return ownedDescendants, nil
}

func (r *Reconciler) reconcileControlPlaneInitialized(ctx context.Context, s *scope) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	cluster := s.cluster

	// Skip checking if the control plane is initialized when using a Control Plane Provider (this is reconciled in
	// reconcileControlPlane instead).
	if cluster.Spec.ControlPlaneRef != nil {
		log.V(4).Info("Skipping reconcileControlPlaneInitialized because cluster has a controlPlaneRef")
		return ctrl.Result{}, nil
	}

	if conditions.IsTrue(cluster, clusterv1.ControlPlaneInitializedCondition) {
		log.V(4).Info("Skipping reconcileControlPlaneInitialized because control plane already initialized")
		return ctrl.Result{}, nil
	}

	log.V(4).Info("Checking for control plane initialization")

	machines, err := collections.GetFilteredMachinesForCluster(ctx, r.Client, cluster, collections.ActiveMachines)
	if err != nil {
		log.Error(err, "unable to determine ControlPlaneInitialized")
		return ctrl.Result{}, err
	}

	for _, m := range machines {
		if util.IsControlPlaneMachine(m) && m.Status.NodeRef != nil {
			conditions.MarkTrue(cluster, clusterv1.ControlPlaneInitializedCondition)
			return ctrl.Result{}, nil
		}
	}

	conditions.MarkFalse(cluster, clusterv1.ControlPlaneInitializedCondition, clusterv1.MissingNodeRefReason, clusterv1.ConditionSeverityInfo, "Waiting for the first control plane machine to have its status.nodeRef set")

	return ctrl.Result{}, nil
}

// controlPlaneMachineToCluster is a handler.ToRequestsFunc to be used to enqueue requests for reconciliation
// for Cluster to update its status.controlPlaneInitialized field.
func (r *Reconciler) controlPlaneMachineToCluster(ctx context.Context, o client.Object) []ctrl.Request {
	m, ok := o.(*clusterv1.Machine)
	if !ok {
		panic(fmt.Sprintf("Expected a Machine but got a %T", o))
	}
	if !util.IsControlPlaneMachine(m) {
		return nil
	}
	if m.Status.NodeRef == nil {
		return nil
	}

	cluster, err := util.GetClusterByName(ctx, r.Client, m.Namespace, m.Spec.ClusterName)
	if err != nil {
		return nil
	}

	if conditions.IsTrue(cluster, clusterv1.ControlPlaneInitializedCondition) {
		return nil
	}

	return []ctrl.Request{{
		NamespacedName: util.ObjectKey(cluster),
	}}
}

// machineDeploymentToCluster is a handler.ToRequestsFunc to be used to enqueue requests for reconciliation
// for Cluster to update when one of its own MachineDeployments gets updated.
func (r *Reconciler) machineDeploymentToCluster(_ context.Context, o client.Object) []ctrl.Request {
	md, ok := o.(*clusterv1.MachineDeployment)
	if !ok {
		panic(fmt.Sprintf("Expected a MachineDeployment but got a %T", o))
	}
	if md.Spec.ClusterName == "" {
		return nil
	}

	return []ctrl.Request{{
		NamespacedName: types.NamespacedName{
			Namespace: md.Namespace,
			Name:      md.Spec.ClusterName,
		},
	}}
}

// machinePoolToCluster is a handler.ToRequestsFunc to be used to enqueue requests for reconciliation
// for Cluster to update when one of its own MachinePools gets updated.
func (r *Reconciler) machinePoolToCluster(_ context.Context, o client.Object) []ctrl.Request {
	mp, ok := o.(*expv1.MachinePool)
	if !ok {
		panic(fmt.Sprintf("Expected a MachinePool but got a %T", o))
	}
	if mp.Spec.ClusterName == "" {
		return nil
	}

	return []ctrl.Request{{
		NamespacedName: types.NamespacedName{
			Namespace: mp.Namespace,
			Name:      mp.Spec.ClusterName,
		},
	}}
}
