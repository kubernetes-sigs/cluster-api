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

	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
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
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/remote"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/internal/util/ssa"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
	"sigs.k8s.io/cluster-api/util/secret"
	"sigs.k8s.io/cluster-api/util/version"
)

const (
	kcpManagerName          = "capi-kubeadmcontrolplane"
	kubeadmControlPlaneKind = "KubeadmControlPlane"
)

// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io;bootstrap.cluster.x-k8s.io;controlplane.cluster.x-k8s.io,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinepools,verbs=list
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch

// KubeadmControlPlaneReconciler reconciles a KubeadmControlPlane object.
type KubeadmControlPlaneReconciler struct {
	Client              client.Client
	SecretCachingClient client.Client
	controller          controller.Controller
	recorder            record.EventRecorder
	Tracker             *remote.ClusterCacheTracker

	EtcdDialTimeout time.Duration
	EtcdCallTimeout time.Duration

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string

	managementCluster         internal.ManagementCluster
	managementClusterUncached internal.ManagementCluster
	ssaCache                  ssa.Cache
}

func (r *KubeadmControlPlaneReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&controlplanev1.KubeadmControlPlane{}).
		Owns(&clusterv1.Machine{}).
		WithOptions(options).
		WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(ctrl.LoggerFrom(ctx), r.WatchFilterValue)).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(r.ClusterToKubeadmControlPlane),
			builder.WithPredicates(
				predicates.All(ctrl.LoggerFrom(ctx),
					predicates.ResourceHasFilterLabel(ctrl.LoggerFrom(ctx), r.WatchFilterValue),
					predicates.ClusterUnpausedAndInfrastructureReady(ctrl.LoggerFrom(ctx)),
				),
			),
		).Build(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	r.controller = c
	r.recorder = mgr.GetEventRecorderFor("kubeadmcontrolplane-controller")
	r.ssaCache = ssa.NewCache()

	if r.managementCluster == nil {
		if r.Tracker == nil {
			return errors.New("cluster cache tracker is nil, cannot create the internal management cluster resource")
		}
		r.managementCluster = &internal.Management{
			Client:              r.Client,
			SecretCachingClient: r.SecretCachingClient,
			Tracker:             r.Tracker,
			EtcdDialTimeout:     r.EtcdDialTimeout,
			EtcdCallTimeout:     r.EtcdCallTimeout,
		}
	}

	if r.managementClusterUncached == nil {
		r.managementClusterUncached = &internal.Management{Client: mgr.GetAPIReader()}
	}

	return nil
}

func (r *KubeadmControlPlaneReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, reterr error) {
	log := ctrl.LoggerFrom(ctx)

	// Fetch the KubeadmControlPlane instance.
	kcp := &controlplanev1.KubeadmControlPlane{}
	if err := r.Client.Get(ctx, req.NamespacedName, kcp); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Fetch the Cluster.
	cluster, err := util.GetOwnerCluster(ctx, r.Client, kcp.ObjectMeta)
	if err != nil {
		log.Error(err, "Failed to retrieve owner Cluster from the API Server")
		return ctrl.Result{}, err
	}
	if cluster == nil {
		log.Info("Cluster Controller has not yet set OwnerRef")
		return ctrl.Result{}, nil
	}
	log = log.WithValues("Cluster", klog.KObj(cluster))
	ctx = ctrl.LoggerInto(ctx, log)

	if annotations.IsPaused(cluster, kcp) {
		log.Info("Reconciliation is paused for this object")
		return ctrl.Result{}, nil
	}

	// Initialize the patch helper.
	patchHelper, err := patch.NewHelper(kcp, r.Client)
	if err != nil {
		log.Error(err, "Failed to configure the patch helper")
		return ctrl.Result{Requeue: true}, nil
	}

	// Add finalizer first if not set to avoid the race condition between init and delete.
	// Note: Finalizers in general can only be added when the deletionTimestamp is not set.
	if kcp.ObjectMeta.DeletionTimestamp.IsZero() && !controllerutil.ContainsFinalizer(kcp, controlplanev1.KubeadmControlPlaneFinalizer) {
		controllerutil.AddFinalizer(kcp, controlplanev1.KubeadmControlPlaneFinalizer)

		// patch and return right away instead of reusing the main defer,
		// because the main defer may take too much time to get cluster status
		// Patch ObservedGeneration only if the reconciliation completed successfully
		patchOpts := []patch.Option{patch.WithStatusObservedGeneration{}}
		if err := patchHelper.Patch(ctx, kcp, patchOpts...); err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to add finalizer")
		}

		return ctrl.Result{}, nil
	}

	// Initialize the control plane scope; this includes also checking for orphan machines and
	// adopt them if necessary.
	controlPlane, adoptableMachineFound, err := r.initControlPlaneScope(ctx, cluster, kcp)
	if err != nil {
		return ctrl.Result{}, err
	}
	if adoptableMachineFound {
		// if there are no errors but at least one CP machine has been adopted, then requeue and
		// wait for the update event for the ownership to be set.
		return ctrl.Result{}, nil
	}

	defer func() {
		// Always attempt to update status.
		if err := r.updateStatus(ctx, controlPlane); err != nil {
			var connFailure *internal.RemoteClusterConnectionError
			if errors.As(err, &connFailure) {
				log.Info("Could not connect to workload cluster to fetch status", "err", err.Error())
			} else {
				log.Error(err, "Failed to update KubeadmControlPlane Status")
				reterr = kerrors.NewAggregate([]error{reterr, err})
			}
		}

		// Always attempt to Patch the KubeadmControlPlane object and status after each reconciliation.
		if err := patchKubeadmControlPlane(ctx, patchHelper, kcp); err != nil {
			log.Error(err, "Failed to patch KubeadmControlPlane")
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}

		// Only requeue if there is no error, Requeue or RequeueAfter and the object does not have a deletion timestamp.
		if reterr == nil && res.IsZero() && kcp.ObjectMeta.DeletionTimestamp.IsZero() {
			// Make KCP requeue in case node status is not ready, so we can check for node status without waiting for a full
			// resync (by default 10 minutes).
			// The alternative solution would be to watch the control plane nodes in the Cluster - similar to how the
			// MachineSet and MachineHealthCheck controllers watch the nodes under their control.
			if !kcp.Status.Ready {
				res = ctrl.Result{RequeueAfter: 20 * time.Second}
			}

			// Make KCP requeue if ControlPlaneComponentsHealthyCondition is false so we can check for control plane component
			// status without waiting for a full resync (by default 10 minutes).
			// Otherwise this condition can lead to a delay in provisioning MachineDeployments when MachineSet preflight checks are enabled.
			// The alternative solution to this requeue would be watching the relevant pods inside each workload cluster which would be very expensive.
			if conditions.IsFalse(kcp, controlplanev1.ControlPlaneComponentsHealthyCondition) {
				res = ctrl.Result{RequeueAfter: 20 * time.Second}
			}
		}
	}()

	if !kcp.ObjectMeta.DeletionTimestamp.IsZero() {
		// Handle deletion reconciliation loop.
		res, err = r.reconcileDelete(ctx, controlPlane)
		// Requeue if the reconcile failed because the ClusterCacheTracker was locked for
		// the current cluster because of concurrent access.
		if errors.Is(err, remote.ErrClusterLocked) {
			log.V(5).Info("Requeuing because another worker has the lock on the ClusterCacheTracker")
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		return res, err
	}

	// Handle normal reconciliation loop.
	res, err = r.reconcile(ctx, controlPlane)
	// Requeue if the reconcile failed because the ClusterCacheTracker was locked for
	// the current cluster because of concurrent access.
	if errors.Is(err, remote.ErrClusterLocked) {
		log.V(5).Info("Requeuing because another worker has the lock on the ClusterCacheTracker")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}
	return res, err
}

// initControlPlaneScope initializes the control plane scope; this includes also checking for orphan machines and
// adopt them if necessary.
// The func also returns a boolean indicating if adoptableMachine have been found and processed, but this doesn't imply those machines
// have been actually adopted).
func (r *KubeadmControlPlaneReconciler) initControlPlaneScope(ctx context.Context, cluster *clusterv1.Cluster, kcp *controlplanev1.KubeadmControlPlane) (*internal.ControlPlane, bool, error) {
	log := ctrl.LoggerFrom(ctx)

	// Return early if the cluster is not yet in a state where control plane machines exists
	if !cluster.Status.InfrastructureReady || !cluster.Spec.ControlPlaneEndpoint.IsValid() {
		controlPlane, err := internal.NewControlPlane(ctx, r.managementCluster, r.Client, cluster, kcp, collections.Machines{})
		if err != nil {
			log.Error(err, "failed to initialize control plane scope")
			return nil, false, err
		}
		return controlPlane, false, nil
	}

	// Read control plane machines
	controlPlaneMachines, err := r.managementClusterUncached.GetMachinesForCluster(ctx, cluster, collections.ControlPlaneMachines(cluster.Name))
	if err != nil {
		log.Error(err, "failed to retrieve control plane machines for cluster")
		return nil, false, err
	}

	// If we are not deleting the CP, adopt stand alone CP machines if any
	adoptableMachines := controlPlaneMachines.Filter(collections.AdoptableControlPlaneMachines(cluster.Name))
	if kcp.ObjectMeta.DeletionTimestamp.IsZero() && len(adoptableMachines) > 0 {
		return nil, true, r.adoptMachines(ctx, kcp, adoptableMachines, cluster)
	}

	ownedMachines := controlPlaneMachines.Filter(collections.OwnedMachines(kcp))
	if kcp.ObjectMeta.DeletionTimestamp.IsZero() && len(ownedMachines) != len(controlPlaneMachines) {
		err := errors.New("not all control plane machines are owned by this KubeadmControlPlane, refusing to operate in mixed management mode")
		log.Error(err, "KCP cannot reconcile")
		return nil, false, err
	}

	controlPlane, err := internal.NewControlPlane(ctx, r.managementCluster, r.Client, cluster, kcp, ownedMachines)
	if err != nil {
		log.Error(err, "failed to initialize control plane scope")
		return nil, false, err
	}
	return controlPlane, false, nil
}

func patchKubeadmControlPlane(ctx context.Context, patchHelper *patch.Helper, kcp *controlplanev1.KubeadmControlPlane) error {
	// Always update the readyCondition by summarizing the state of other conditions.
	conditions.SetSummary(kcp,
		conditions.WithConditions(
			controlplanev1.MachinesCreatedCondition,
			controlplanev1.MachinesSpecUpToDateCondition,
			controlplanev1.ResizedCondition,
			controlplanev1.MachinesReadyCondition,
			controlplanev1.AvailableCondition,
			controlplanev1.CertificatesAvailableCondition,
		),
	)

	// Patch the object, ignoring conflicts on the conditions owned by this controller.
	return patchHelper.Patch(
		ctx,
		kcp,
		patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
			controlplanev1.MachinesCreatedCondition,
			clusterv1.ReadyCondition,
			controlplanev1.MachinesSpecUpToDateCondition,
			controlplanev1.ResizedCondition,
			controlplanev1.MachinesReadyCondition,
			controlplanev1.AvailableCondition,
			controlplanev1.CertificatesAvailableCondition,
		}},
		patch.WithStatusObservedGeneration{},
	)
}

// reconcile handles KubeadmControlPlane reconciliation.
func (r *KubeadmControlPlaneReconciler) reconcile(ctx context.Context, controlPlane *internal.ControlPlane) (res ctrl.Result, reterr error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Reconcile KubeadmControlPlane")

	// Make sure to reconcile the external infrastructure reference.
	if err := r.reconcileExternalReference(ctx, controlPlane.Cluster, &controlPlane.KCP.Spec.MachineTemplate.InfrastructureRef); err != nil {
		return ctrl.Result{}, err
	}

	// Wait for the cluster infrastructure to be ready before creating machines
	if !controlPlane.Cluster.Status.InfrastructureReady {
		log.Info("Cluster infrastructure is not ready yet")
		return ctrl.Result{}, nil
	}

	// Reconcile cluster certificates.
	if err := r.reconcileClusterCertificates(ctx, controlPlane); err != nil {
		return ctrl.Result{}, err
	}

	// If ControlPlaneEndpoint is not set, return early
	if !controlPlane.Cluster.Spec.ControlPlaneEndpoint.IsValid() {
		log.Info("Cluster does not yet have a ControlPlaneEndpoint defined")
		return ctrl.Result{}, nil
	}

	// Generate Cluster Kubeconfig if needed
	if result, err := r.reconcileKubeconfig(ctx, controlPlane); !result.IsZero() || err != nil {
		if err != nil {
			log.Error(err, "failed to reconcile Kubeconfig")
		}
		return result, err
	}

	if err := r.syncMachines(ctx, controlPlane); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to sync Machines")
	}

	// Aggregate the operational state of all the machines; while aggregating we are adding the
	// source ref (reason@machine/name) so the problem can be easily tracked down to its source machine.
	conditions.SetAggregate(controlPlane.KCP, controlplanev1.MachinesReadyCondition, controlPlane.Machines.ConditionGetters(), conditions.AddSourceRef())

	// Updates conditions reporting the status of static pods and the status of the etcd cluster.
	// NOTE: Conditions reporting KCP operation progress like e.g. Resized or SpecUpToDate are inlined with the rest of the execution.
	if err := r.reconcileControlPlaneConditions(ctx, controlPlane); err != nil {
		return ctrl.Result{}, err
	}

	// Ensures the number of etcd members is in sync with the number of machines/nodes.
	// NOTE: This is usually required after a machine deletion.
	if err := r.reconcileEtcdMembers(ctx, controlPlane); err != nil {
		return ctrl.Result{}, err
	}

	// Reconcile unhealthy machines by triggering deletion and requeue if it is considered safe to remediate,
	// otherwise continue with the other KCP operations.
	if result, err := r.reconcileUnhealthyMachines(ctx, controlPlane); err != nil || !result.IsZero() {
		return result, err
	}

	// Control plane machines rollout due to configuration changes (e.g. upgrades) takes precedence over other operations.
	machinesNeedingRollout, rolloutReasons := controlPlane.MachinesNeedingRollout()
	switch {
	case len(machinesNeedingRollout) > 0:
		var reasons []string
		for _, rolloutReason := range rolloutReasons {
			reasons = append(reasons, rolloutReason)
		}
		log.Info(fmt.Sprintf("Rolling out Control Plane machines: %s", strings.Join(reasons, ",")), "machinesNeedingRollout", machinesNeedingRollout.Names())
		conditions.MarkFalse(controlPlane.KCP, controlplanev1.MachinesSpecUpToDateCondition, controlplanev1.RollingUpdateInProgressReason, clusterv1.ConditionSeverityWarning, "Rolling %d replicas with outdated spec (%d replicas up to date)", len(machinesNeedingRollout), len(controlPlane.Machines)-len(machinesNeedingRollout))
		return r.upgradeControlPlane(ctx, controlPlane, machinesNeedingRollout)
	default:
		// make sure last upgrade operation is marked as completed.
		// NOTE: we are checking the condition already exists in order to avoid to set this condition at the first
		// reconciliation/before a rolling upgrade actually starts.
		if conditions.Has(controlPlane.KCP, controlplanev1.MachinesSpecUpToDateCondition) {
			conditions.MarkTrue(controlPlane.KCP, controlplanev1.MachinesSpecUpToDateCondition)
		}
	}

	// If we've made it this far, we can assume that all ownedMachines are up to date
	numMachines := len(controlPlane.Machines)
	desiredReplicas := int(*controlPlane.KCP.Spec.Replicas)

	switch {
	// We are creating the first replica
	case numMachines < desiredReplicas && numMachines == 0:
		// Create new Machine w/ init
		log.Info("Initializing control plane", "Desired", desiredReplicas, "Existing", numMachines)
		conditions.MarkFalse(controlPlane.KCP, controlplanev1.AvailableCondition, controlplanev1.WaitingForKubeadmInitReason, clusterv1.ConditionSeverityInfo, "")
		return r.initializeControlPlane(ctx, controlPlane)
	// We are scaling up
	case numMachines < desiredReplicas && numMachines > 0:
		// Create a new Machine w/ join
		log.Info("Scaling up control plane", "Desired", desiredReplicas, "Existing", numMachines)
		return r.scaleUpControlPlane(ctx, controlPlane)
	// We are scaling down
	case numMachines > desiredReplicas:
		log.Info("Scaling down control plane", "Desired", desiredReplicas, "Existing", numMachines)
		// The last parameter (i.e. machines needing to be rolled out) should always be empty here.
		return r.scaleDownControlPlane(ctx, controlPlane, collections.Machines{})
	}

	// Get the workload cluster client.
	workloadCluster, err := controlPlane.GetWorkloadCluster(ctx)
	if err != nil {
		log.V(2).Info("cannot get remote client to workload cluster, will requeue", "cause", err)
		return ctrl.Result{Requeue: true}, nil
	}

	// Ensure kubeadm role bindings for v1.18+
	if err := workloadCluster.AllowBootstrapTokensToGetNodes(ctx); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to set role and role binding for kubeadm")
	}

	// We intentionally only parse major/minor/patch so that the subsequent code
	// also already applies to beta versions of new releases.
	parsedVersion, err := version.ParseMajorMinorPatchTolerant(controlPlane.KCP.Spec.Version)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to parse kubernetes version %q", controlPlane.KCP.Spec.Version)
	}

	// Update kube-proxy daemonset.
	if err := workloadCluster.UpdateKubeProxyImageInfo(ctx, controlPlane.KCP, parsedVersion); err != nil {
		log.Error(err, "failed to update kube-proxy daemonset")
		return ctrl.Result{}, err
	}

	// Update CoreDNS deployment.
	if err := workloadCluster.UpdateCoreDNS(ctx, controlPlane.KCP, parsedVersion); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to update CoreDNS deployment")
	}

	// Reconcile certificate expiry for Machines that don't have the expiry annotation on KubeadmConfig yet.
	// Note: This requires that all control plane machines are working. We moved this to the end of the reconcile
	// as nothing in the same reconcile depends on it and to ensure it doesn't block anything else,
	// especially MHC remediation and rollout of changes to recover the control plane.
	if err := r.reconcileCertificateExpiries(ctx, controlPlane); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// reconcileClusterCertificates ensures that all the cluster certificates exists and
// enforces all the expected owner ref on them.
func (r *KubeadmControlPlaneReconciler) reconcileClusterCertificates(ctx context.Context, controlPlane *internal.ControlPlane) error {
	log := ctrl.LoggerFrom(ctx)

	// Generate Cluster Certificates if needed
	config := controlPlane.KCP.Spec.KubeadmConfigSpec.DeepCopy()
	config.JoinConfiguration = nil
	if config.ClusterConfiguration == nil {
		config.ClusterConfiguration = &bootstrapv1.ClusterConfiguration{}
	}
	certificates := secret.NewCertificatesForInitialControlPlane(config.ClusterConfiguration)
	controllerRef := metav1.NewControllerRef(controlPlane.KCP, controlplanev1.GroupVersion.WithKind(kubeadmControlPlaneKind))
	if err := certificates.LookupOrGenerateCached(ctx, r.SecretCachingClient, r.Client, util.ObjectKey(controlPlane.Cluster), *controllerRef); err != nil {
		log.Error(err, "unable to lookup or create cluster certificates")
		conditions.MarkFalse(controlPlane.KCP, controlplanev1.CertificatesAvailableCondition, controlplanev1.CertificatesGenerationFailedReason, clusterv1.ConditionSeverityWarning, err.Error())
		return err
	}

	if err := r.ensureCertificatesOwnerRef(ctx, certificates, *controllerRef); err != nil {
		return err
	}

	conditions.MarkTrue(controlPlane.KCP, controlplanev1.CertificatesAvailableCondition)
	return nil
}

// reconcileDelete handles KubeadmControlPlane deletion.
// The implementation does not take non-control plane workloads into consideration. This may or may not change in the future.
// Please see https://github.com/kubernetes-sigs/cluster-api/issues/2064.
func (r *KubeadmControlPlaneReconciler) reconcileDelete(ctx context.Context, controlPlane *internal.ControlPlane) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Reconcile KubeadmControlPlane deletion")

	// If no control plane machines remain, remove the finalizer
	if len(controlPlane.Machines) == 0 {
		controllerutil.RemoveFinalizer(controlPlane.KCP, controlplanev1.KubeadmControlPlaneFinalizer)
		return ctrl.Result{}, nil
	}

	// Updates conditions reporting the status of static pods and the status of the etcd cluster.
	// NOTE: Ignoring failures given that we are deleting
	if err := r.reconcileControlPlaneConditions(ctx, controlPlane); err != nil {
		log.Info("failed to reconcile conditions", "error", err.Error())
	}

	// Aggregate the operational state of all the machines; while aggregating we are adding the
	// source ref (reason@machine/name) so the problem can be easily tracked down to its source machine.
	// However, during delete we are hiding the counter (1 of x) because it does not make sense given that
	// all the machines are deleted in parallel.
	conditions.SetAggregate(controlPlane.KCP, controlplanev1.MachinesReadyCondition, controlPlane.Machines.ConditionGetters(), conditions.AddSourceRef())

	// Gets all machines, not just control plane machines.
	allMachines, err := r.managementCluster.GetMachinesForCluster(ctx, controlPlane.Cluster)
	if err != nil {
		return ctrl.Result{}, err
	}

	allMachinePools := &expv1.MachinePoolList{}
	// Get all machine pools.
	if feature.Gates.Enabled(feature.MachinePool) {
		allMachinePools, err = r.managementCluster.GetMachinePoolsForCluster(ctx, controlPlane.Cluster)
		if err != nil {
			return ctrl.Result{}, err
		}
	}
	// Verify that only control plane machines remain
	if len(allMachines) != len(controlPlane.Machines) || len(allMachinePools.Items) != 0 {
		log.Info("Waiting for worker nodes to be deleted first")
		conditions.MarkFalse(controlPlane.KCP, controlplanev1.ResizedCondition, clusterv1.DeletingReason, clusterv1.ConditionSeverityInfo, "Waiting for worker nodes to be deleted first")
		return ctrl.Result{RequeueAfter: deleteRequeueAfter}, nil
	}

	// Delete control plane machines in parallel
	machinesToDelete := controlPlane.Machines.Filter(collections.Not(collections.HasDeletionTimestamp))
	var errs []error
	for i := range machinesToDelete {
		m := machinesToDelete[i]
		logger := log.WithValues("Machine", klog.KObj(m))
		if err := r.Client.Delete(ctx, machinesToDelete[i]); err != nil && !apierrors.IsNotFound(err) {
			logger.Error(err, "Failed to cleanup owned machine")
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		err := kerrors.NewAggregate(errs)
		r.recorder.Eventf(controlPlane.KCP, corev1.EventTypeWarning, "FailedDelete",
			"Failed to delete control plane Machines for cluster %s control plane: %v", klog.KObj(controlPlane.Cluster), err)
		return ctrl.Result{}, err
	}
	conditions.MarkFalse(controlPlane.KCP, controlplanev1.ResizedCondition, clusterv1.DeletingReason, clusterv1.ConditionSeverityInfo, "")
	return ctrl.Result{RequeueAfter: deleteRequeueAfter}, nil
}

// ClusterToKubeadmControlPlane is a handler.ToRequestsFunc to be used to enqueue requests for reconciliation
// for KubeadmControlPlane based on updates to a Cluster.
func (r *KubeadmControlPlaneReconciler) ClusterToKubeadmControlPlane(_ context.Context, o client.Object) []ctrl.Request {
	c, ok := o.(*clusterv1.Cluster)
	if !ok {
		panic(fmt.Sprintf("Expected a Cluster but got a %T", o))
	}

	controlPlaneRef := c.Spec.ControlPlaneRef
	if controlPlaneRef != nil && controlPlaneRef.Kind == kubeadmControlPlaneKind {
		return []ctrl.Request{{NamespacedName: client.ObjectKey{Namespace: controlPlaneRef.Namespace, Name: controlPlaneRef.Name}}}
	}

	return nil
}

// syncMachines updates Machines, InfrastructureMachines and KubeadmConfigs to propagate in-place mutable fields from KCP.
// Note: It also cleans up managed fields of all Machines so that Machines that were
// created/patched before (< v1.4.0) the controller adopted Server-Side-Apply (SSA) can also work with SSA.
// Note: For InfrastructureMachines and KubeadmConfigs it also drops ownership of "metadata.labels" and
// "metadata.annotations" from "manager" so that "capi-kubeadmcontrolplane" can own these fields and can work with SSA.
// Otherwise, fields would be co-owned by our "old" "manager" and "capi-kubeadmcontrolplane" and then we would not be
// able to e.g. drop labels and annotations.
func (r *KubeadmControlPlaneReconciler) syncMachines(ctx context.Context, controlPlane *internal.ControlPlane) error {
	patchHelpers := map[string]*patch.Helper{}
	for machineName := range controlPlane.Machines {
		m := controlPlane.Machines[machineName]
		// If the machine is already being deleted, we don't need to update it.
		if !m.DeletionTimestamp.IsZero() {
			continue
		}

		// Cleanup managed fields of all Machines.
		// We do this so that Machines that were created/patched before the controller adopted Server-Side-Apply (SSA)
		// (< v1.4.0) can also work with SSA. Otherwise, fields would be co-owned by our "old" "manager" and
		// "capi-kubeadmcontrolplane" and then we would not be able to e.g. drop labels and annotations.
		if err := ssa.CleanUpManagedFieldsForSSAAdoption(ctx, r.Client, m, kcpManagerName); err != nil {
			return errors.Wrapf(err, "failed to update Machine: failed to adjust the managedFields of the Machine %s", klog.KObj(m))
		}
		// Update Machine to propagate in-place mutable fields from KCP.
		updatedMachine, err := r.updateMachine(ctx, m, controlPlane.KCP, controlPlane.Cluster)
		if err != nil {
			return errors.Wrapf(err, "failed to update Machine: %s", klog.KObj(m))
		}
		controlPlane.Machines[machineName] = updatedMachine
		// Since the machine is updated, re-create the patch helper so that any subsequent
		// Patch calls use the correct base machine object to calculate the diffs.
		// Example: reconcileControlPlaneConditions patches the machine objects in a subsequent call
		// and, it should use the updated machine to calculate the diff.
		// Note: If the patchHelpers are not re-computed based on the new updated machines, subsequent
		// Patch calls will fail because the patch will be calculated based on an outdated machine and will error
		// because of outdated resourceVersion.
		// TODO: This should be cleaned-up to have a more streamline way of constructing and using patchHelpers.
		patchHelper, err := patch.NewHelper(updatedMachine, r.Client)
		if err != nil {
			return err
		}
		patchHelpers[machineName] = patchHelper

		labelsAndAnnotationsManagedFieldPaths := []contract.Path{
			{"f:metadata", "f:annotations"},
			{"f:metadata", "f:labels"},
		}
		infraMachine, infraMachineFound := controlPlane.InfraResources[machineName]
		// Only update the InfraMachine if it is already found, otherwise just skip it.
		// This could happen e.g. if the cache is not up-to-date yet.
		if infraMachineFound {
			// Cleanup managed fields of all InfrastructureMachines to drop ownership of labels and annotations
			// from "manager". We do this so that InfrastructureMachines that are created using the Create method
			// can also work with SSA. Otherwise, labels and annotations would be co-owned by our "old" "manager"
			// and "capi-kubeadmcontrolplane" and then we would not be able to e.g. drop labels and annotations.
			if err := ssa.DropManagedFields(ctx, r.Client, infraMachine, kcpManagerName, labelsAndAnnotationsManagedFieldPaths); err != nil {
				return errors.Wrapf(err, "failed to clean up managedFields of InfrastructureMachine %s", klog.KObj(infraMachine))
			}
			// Update in-place mutating fields on InfrastructureMachine.
			if err := r.updateExternalObject(ctx, infraMachine, controlPlane.KCP, controlPlane.Cluster); err != nil {
				return errors.Wrapf(err, "failed to update InfrastructureMachine %s", klog.KObj(infraMachine))
			}
		}

		kubeadmConfig, kubeadmConfigFound := controlPlane.KubeadmConfigs[machineName]
		// Only update the KubeadmConfig if it is already found, otherwise just skip it.
		// This could happen e.g. if the cache is not up-to-date yet.
		if kubeadmConfigFound {
			// Note: Set the GroupVersionKind because updateExternalObject depends on it.
			kubeadmConfig.SetGroupVersionKind(m.Spec.Bootstrap.ConfigRef.GroupVersionKind())
			// Cleanup managed fields of all KubeadmConfigs to drop ownership of labels and annotations
			// from "manager". We do this so that KubeadmConfigs that are created using the Create method
			// can also work with SSA. Otherwise, labels and annotations would be co-owned by our "old" "manager"
			// and "capi-kubeadmcontrolplane" and then we would not be able to e.g. drop labels and annotations.
			if err := ssa.DropManagedFields(ctx, r.Client, kubeadmConfig, kcpManagerName, labelsAndAnnotationsManagedFieldPaths); err != nil {
				return errors.Wrapf(err, "failed to clean up managedFields of KubeadmConfig %s", klog.KObj(kubeadmConfig))
			}
			// Update in-place mutating fields on BootstrapConfig.
			if err := r.updateExternalObject(ctx, kubeadmConfig, controlPlane.KCP, controlPlane.Cluster); err != nil {
				return errors.Wrapf(err, "failed to update KubeadmConfig %s", klog.KObj(kubeadmConfig))
			}
		}
	}
	// Update the patch helpers.
	controlPlane.SetPatchHelpers(patchHelpers)
	return nil
}

// reconcileControlPlaneConditions is responsible of reconciling conditions reporting the status of static pods and
// the status of the etcd cluster.
func (r *KubeadmControlPlaneReconciler) reconcileControlPlaneConditions(ctx context.Context, controlPlane *internal.ControlPlane) error {
	// If the cluster is not yet initialized, there is no way to connect to the workload cluster and fetch information
	// for updating conditions. Return early.
	if !controlPlane.KCP.Status.Initialized {
		return nil
	}

	workloadCluster, err := controlPlane.GetWorkloadCluster(ctx)
	if err != nil {
		return errors.Wrap(err, "cannot get remote client to workload cluster")
	}

	// Update conditions status
	workloadCluster.UpdateStaticPodConditions(ctx, controlPlane)
	workloadCluster.UpdateEtcdConditions(ctx, controlPlane)

	// Patch machines with the updated conditions.
	if err := controlPlane.PatchMachines(ctx); err != nil {
		return err
	}

	// KCP will be patched at the end of Reconcile to reflect updated conditions, so we can return now.
	return nil
}

// reconcileEtcdMembers ensures the number of etcd members is in sync with the number of machines/nodes.
// This is usually required after a machine deletion.
//
// NOTE: this func uses KCP conditions, it is required to call reconcileControlPlaneConditions before this.
func (r *KubeadmControlPlaneReconciler) reconcileEtcdMembers(ctx context.Context, controlPlane *internal.ControlPlane) error {
	log := ctrl.LoggerFrom(ctx)

	// If etcd is not managed by KCP this is a no-op.
	if !controlPlane.IsEtcdManaged() {
		return nil
	}

	// If there is no KCP-owned control-plane machines, then control-plane has not been initialized yet.
	if controlPlane.Machines.Len() == 0 {
		return nil
	}

	// Collect all the node names.
	nodeNames := []string{}
	for _, machine := range controlPlane.Machines {
		if machine.Status.NodeRef == nil {
			// If there are provisioning machines (machines without a node yet), return.
			return nil
		}
		nodeNames = append(nodeNames, machine.Status.NodeRef.Name)
	}

	// Potential inconsistencies between the list of members and the list of machines/nodes are
	// surfaced using the EtcdClusterHealthyCondition; if this condition is true, meaning no inconsistencies exists, return early.
	if conditions.IsTrue(controlPlane.KCP, controlplanev1.EtcdClusterHealthyCondition) {
		return nil
	}

	workloadCluster, err := controlPlane.GetWorkloadCluster(ctx)
	if err != nil {
		// Failing at connecting to the workload cluster can mean workload cluster is unhealthy for a variety of reasons such as etcd quorum loss.
		return errors.Wrap(err, "cannot get remote client to workload cluster")
	}

	parsedVersion, err := semver.ParseTolerant(controlPlane.KCP.Spec.Version)
	if err != nil {
		return errors.Wrapf(err, "failed to parse kubernetes version %q", controlPlane.KCP.Spec.Version)
	}

	removedMembers, err := workloadCluster.ReconcileEtcdMembers(ctx, nodeNames, parsedVersion)
	if err != nil {
		return errors.Wrap(err, "failed attempt to reconcile etcd members")
	}

	if len(removedMembers) > 0 {
		log.Info("Etcd members without nodes removed from the cluster", "members", removedMembers)
	}

	return nil
}

func (r *KubeadmControlPlaneReconciler) reconcileCertificateExpiries(ctx context.Context, controlPlane *internal.ControlPlane) error {
	log := ctrl.LoggerFrom(ctx)

	// Return if there are no KCP-owned control-plane machines.
	if controlPlane.Machines.Len() == 0 {
		return nil
	}

	// Return if KCP is not yet initialized (no API server to contact for checking certificate expiration).
	if !controlPlane.KCP.Status.Initialized {
		return nil
	}

	// Ignore machines which are being deleted.
	machines := controlPlane.Machines.Filter(collections.Not(collections.HasDeletionTimestamp))

	workloadCluster, err := controlPlane.GetWorkloadCluster(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to reconcile certificate expiries: cannot get remote client to workload cluster")
	}

	for _, m := range machines {
		log := log.WithValues("Machine", klog.KObj(m))

		kubeadmConfig, ok := controlPlane.GetKubeadmConfig(m.Name)
		if !ok {
			// Skip if the Machine doesn't have a KubeadmConfig.
			continue
		}

		annotations := kubeadmConfig.GetAnnotations()
		if _, ok := annotations[clusterv1.MachineCertificatesExpiryDateAnnotation]; ok {
			// Skip if annotation is already set.
			continue
		}

		if m.Status.NodeRef == nil {
			// Skip if the Machine is still provisioning.
			continue
		}
		nodeName := m.Status.NodeRef.Name
		log = log.WithValues("Node", klog.KRef("", nodeName))

		log.V(3).Info("Reconciling certificate expiry")
		certificateExpiry, err := workloadCluster.GetAPIServerCertificateExpiry(ctx, kubeadmConfig, nodeName)
		if err != nil {
			return errors.Wrapf(err, "failed to reconcile certificate expiry for Machine/%s", m.Name)
		}
		expiry := certificateExpiry.Format(time.RFC3339)

		log.V(2).Info(fmt.Sprintf("Setting certificate expiry to %s", expiry))
		patchHelper, err := patch.NewHelper(kubeadmConfig, r.Client)
		if err != nil {
			return errors.Wrapf(err, "failed to reconcile certificate expiry for Machine/%s", m.Name)
		}

		if annotations == nil {
			annotations = map[string]string{}
		}
		annotations[clusterv1.MachineCertificatesExpiryDateAnnotation] = expiry
		kubeadmConfig.SetAnnotations(annotations)

		if err := patchHelper.Patch(ctx, kubeadmConfig); err != nil {
			return errors.Wrapf(err, "failed to reconcile certificate expiry for Machine/%s", m.Name)
		}
	}

	return nil
}

func (r *KubeadmControlPlaneReconciler) adoptMachines(ctx context.Context, kcp *controlplanev1.KubeadmControlPlane, machines collections.Machines, cluster *clusterv1.Cluster) error {
	// We do an uncached full quorum read against the KCP to avoid re-adopting Machines the garbage collector just intentionally orphaned
	// See https://github.com/kubernetes/kubernetes/issues/42639
	uncached := controlplanev1.KubeadmControlPlane{}
	err := r.managementClusterUncached.Get(ctx, client.ObjectKey{Namespace: kcp.Namespace, Name: kcp.Name}, &uncached)
	if err != nil {
		return errors.Wrapf(err, "failed to check whether %v/%v was deleted before adoption", kcp.GetNamespace(), kcp.GetName())
	}
	if !uncached.DeletionTimestamp.IsZero() {
		return errors.Errorf("%v/%v has just been deleted at %v", kcp.GetNamespace(), kcp.GetName(), kcp.GetDeletionTimestamp())
	}

	kcpVersion, err := semver.ParseTolerant(kcp.Spec.Version)
	if err != nil {
		return errors.Wrapf(err, "failed to parse kubernetes version %q", kcp.Spec.Version)
	}

	for _, m := range machines {
		ref := m.Spec.Bootstrap.ConfigRef

		// TODO instead of returning error here, we should instead Event and add a watch on potentially adoptable Machines
		if ref == nil || ref.Kind != "KubeadmConfig" {
			return errors.Errorf("unable to adopt Machine %v/%v: expected a ConfigRef of kind KubeadmConfig but instead found %v", m.Namespace, m.Name, ref)
		}

		// TODO instead of returning error here, we should instead Event and add a watch on potentially adoptable Machines
		if ref.Namespace != "" && ref.Namespace != kcp.Namespace {
			return errors.Errorf("could not adopt resources from KubeadmConfig %v/%v: cannot adopt across namespaces", ref.Namespace, ref.Name)
		}

		if m.Spec.Version == nil {
			// if the machine's version is not immediately apparent, assume the operator knows what they're doing
			continue
		}

		machineVersion, err := semver.ParseTolerant(*m.Spec.Version)
		if err != nil {
			return errors.Wrapf(err, "failed to parse kubernetes version %q", *m.Spec.Version)
		}

		if !util.IsSupportedVersionSkew(kcpVersion, machineVersion) {
			r.recorder.Eventf(kcp, corev1.EventTypeWarning, "AdoptionFailed", "Could not adopt Machine %s/%s: its version (%q) is outside supported +/- one minor version skew from KCP's (%q)", m.Namespace, m.Name, *m.Spec.Version, kcp.Spec.Version)
			// avoid returning an error here so we don't cause the KCP controller to spin until the operator clarifies their intent
			return nil
		}
	}

	for _, m := range machines {
		ref := m.Spec.Bootstrap.ConfigRef
		cfg := &bootstrapv1.KubeadmConfig{}

		if err := r.Client.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: kcp.Namespace}, cfg); err != nil {
			return err
		}

		if err := r.adoptOwnedSecrets(ctx, kcp, cfg, cluster.Name); err != nil {
			return err
		}

		patchHelper, err := patch.NewHelper(m, r.Client)
		if err != nil {
			return err
		}

		if err := controllerutil.SetControllerReference(kcp, m, r.Client.Scheme()); err != nil {
			return err
		}

		// Note that ValidateOwnerReferences() will reject this patch if another
		// OwnerReference exists with controller=true.
		if err := patchHelper.Patch(ctx, m); err != nil {
			return err
		}
	}
	return nil
}

func (r *KubeadmControlPlaneReconciler) adoptOwnedSecrets(ctx context.Context, kcp *controlplanev1.KubeadmControlPlane, currentOwner *bootstrapv1.KubeadmConfig, clusterName string) error {
	secrets := corev1.SecretList{}
	if err := r.Client.List(ctx, &secrets, client.InNamespace(kcp.Namespace), client.MatchingLabels{clusterv1.ClusterNameLabel: clusterName}); err != nil {
		return errors.Wrap(err, "error finding secrets for adoption")
	}

	for i := range secrets.Items {
		s := secrets.Items[i]
		if !util.IsOwnedByObject(&s, currentOwner) {
			continue
		}
		// avoid taking ownership of the bootstrap data secret
		if currentOwner.Status.DataSecretName != nil && s.Name == *currentOwner.Status.DataSecretName {
			continue
		}

		ss := s.DeepCopy()

		ss.SetOwnerReferences(util.ReplaceOwnerRef(ss.GetOwnerReferences(), currentOwner, metav1.OwnerReference{
			APIVersion:         controlplanev1.GroupVersion.String(),
			Kind:               "KubeadmControlPlane",
			Name:               kcp.Name,
			UID:                kcp.UID,
			Controller:         ptr.To(true),
			BlockOwnerDeletion: ptr.To(true),
		}))

		if err := r.Client.Update(ctx, ss); err != nil {
			return errors.Wrapf(err, "error changing secret %v ownership from KubeadmConfig/%v to KubeadmControlPlane/%v", s.Name, currentOwner.GetName(), kcp.Name)
		}
	}

	return nil
}

// ensureCertificatesOwnerRef ensures an ownerReference to the owner is added on the Secrets holding certificates.
func (r *KubeadmControlPlaneReconciler) ensureCertificatesOwnerRef(ctx context.Context, certificates secret.Certificates, owner metav1.OwnerReference) error {
	for _, c := range certificates {
		if c.Secret == nil {
			continue
		}

		patchHelper, err := patch.NewHelper(c.Secret, r.Client)
		if err != nil {
			return err
		}

		controller := metav1.GetControllerOf(c.Secret)
		// If the current controller is KCP, ensure the owner reference is up to date.
		// Note: This ensures secrets created prior to v1alpha4 are updated to have the correct owner reference apiVersion.
		if controller != nil && controller.Kind == kubeadmControlPlaneKind {
			c.Secret.SetOwnerReferences(util.EnsureOwnerRef(c.Secret.GetOwnerReferences(), owner))
		}

		// If the Type doesn't match the type used for secrets created by core components continue without altering the owner reference further.
		// Note: This ensures that control plane related secrets created by KubeadmConfig are eventually owned by KCP.
		// TODO: Remove this logic once standalone control plane machines are no longer allowed.
		if c.Secret.Type == clusterv1.ClusterSecretType {
			// Remove the current controller if one exists.
			if controller != nil {
				c.Secret.SetOwnerReferences(util.RemoveOwnerRef(c.Secret.GetOwnerReferences(), *controller))
			}
			c.Secret.SetOwnerReferences(util.EnsureOwnerRef(c.Secret.GetOwnerReferences(), owner))
		}
		if err := patchHelper.Patch(ctx, c.Secret); err != nil {
			return errors.Wrapf(err, "failed to set ownerReference")
		}
	}
	return nil
}
