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
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	runtimeclient "sigs.k8s.io/cluster-api/exp/runtime/client"
	"sigs.k8s.io/cluster-api/feature"
	capicontrollerutil "sigs.k8s.io/cluster-api/internal/util/controller"
	"sigs.k8s.io/cluster-api/internal/util/inplace"
	"sigs.k8s.io/cluster-api/internal/util/ssa"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/cache"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
	"sigs.k8s.io/cluster-api/util/finalizers"
	clog "sigs.k8s.io/cluster-api/util/log"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/paused"
	"sigs.k8s.io/cluster-api/util/predicates"
	"sigs.k8s.io/cluster-api/util/secret"
	"sigs.k8s.io/cluster-api/util/version"
)

const (
	kcpManagerName          = "capi-kubeadmcontrolplane"
	kcpMetadataManagerName  = "capi-kubeadmcontrolplane-metadata"
	kubeadmControlPlaneKind = "KubeadmControlPlane"
)

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io;bootstrap.cluster.x-k8s.io;controlplane.cluster.x-k8s.io,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinepools,verbs=get;list;watch
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch

// KubeadmControlPlaneReconciler reconciles a KubeadmControlPlane object.
type KubeadmControlPlaneReconciler struct {
	Client              client.Client
	APIReader           client.Reader
	SecretCachingClient client.Client
	RuntimeClient       runtimeclient.Client
	controller          capicontrollerutil.Controller
	recorder            record.EventRecorder
	ClusterCache        clustercache.ClusterCache

	EtcdDialTimeout time.Duration
	EtcdCallTimeout time.Duration
	EtcdLogger      *zap.Logger

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string

	RemoteConditionsGracePeriod time.Duration

	managementCluster         internal.ManagementCluster
	managementClusterUncached internal.ManagementCluster
	ssaCache                  ssa.Cache

	// Only used for testing.
	overrideTryInPlaceUpdateFunc       func(ctx context.Context, controlPlane *internal.ControlPlane, machineToInPlaceUpdate *clusterv1.Machine, machineUpToDateResult internal.UpToDateResult) (bool, ctrl.Result, error)
	overrideScaleUpControlPlaneFunc    func(ctx context.Context, controlPlane *internal.ControlPlane) (ctrl.Result, error)
	overrideScaleDownControlPlaneFunc  func(ctx context.Context, controlPlane *internal.ControlPlane, machineToDelete *clusterv1.Machine) (ctrl.Result, error)
	overridePreflightChecksFunc        func(ctx context.Context, controlPlane *internal.ControlPlane, excludeFor ...*clusterv1.Machine) ctrl.Result
	overrideCanUpdateMachineFunc       func(ctx context.Context, machine *clusterv1.Machine, machineUpToDateResult internal.UpToDateResult) (bool, error)
	overrideCanExtensionsUpdateMachine func(ctx context.Context, machine *clusterv1.Machine, machineUpToDateResult internal.UpToDateResult, extensionHandlers []string) (bool, []string, error)
	overrideTriggerInPlaceUpdate       func(ctx context.Context, machine *clusterv1.Machine, machineUpToDateResult internal.UpToDateResult) error
	// Note: This field is only used for unit tests that use fake client because the fake client does not properly set resourceVersion
	//       on BootstrapConfig/InfraMachine after ssa.Patch and then ssa.RemoveManagedFieldsForLabelsAndAnnotations would fail.
	disableRemoveManagedFieldsForLabelsAndAnnotations bool
}

func (r *KubeadmControlPlaneReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	if r.Client == nil || r.SecretCachingClient == nil || r.ClusterCache == nil ||
		r.EtcdDialTimeout == time.Duration(0) || r.EtcdCallTimeout == time.Duration(0) ||
		r.RemoteConditionsGracePeriod < 2*time.Minute {
		// A minimum of 2m is enforced to ensure the ClusterCache always drops the connection before the grace period is reached.
		// In the worst case the ClusterCache will take FailureThreshold x (Interval + Timeout) = 5x(10s+5s) = 75s to drop a
		// connection. There might be some additional delays in health checking under high load. So we use 2m as a minimum
		// to have some buffer.
		return errors.New("Client, SecretCachingClient and ClusterCache must not be nil and " +
			"EtcdDialTimeout and EtcdCallTimeout must not be 0 and " +
			"RemoteConditionsGracePeriod must not be < 2m")
	}
	if feature.Gates.Enabled(feature.InPlaceUpdates) && r.RuntimeClient == nil {
		return errors.New("RuntimeClient must not be nil when InPlaceUpdates feature gate is enabled")
	}

	predicateLog := ctrl.LoggerFrom(ctx).WithValues("controller", "kubeadmcontrolplane")
	c, err := capicontrollerutil.NewControllerManagedBy(mgr, predicateLog).
		For(&controlplanev1.KubeadmControlPlane{}).
		Owns(&clusterv1.Machine{}).
		WithOptions(options).
		WithEventFilter(predicates.ResourceHasFilterLabel(mgr.GetScheme(), predicateLog, r.WatchFilterValue)).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(r.ClusterToKubeadmControlPlane),
			predicates.ResourceHasFilterLabel(mgr.GetScheme(), predicateLog, r.WatchFilterValue),
			predicates.Any(mgr.GetScheme(), predicateLog,
				predicates.ClusterPausedTransitionsOrInfrastructureProvisioned(mgr.GetScheme(), predicateLog),
				predicates.ClusterTopologyVersionChanged(mgr.GetScheme(), predicateLog),
			),
		).
		WatchesRawSource(r.ClusterCache.GetClusterSource("kubeadmcontrolplane", r.ClusterToKubeadmControlPlane,
			clustercache.WatchForProbeFailure(r.RemoteConditionsGracePeriod))).
		Build(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	r.controller = c
	r.recorder = mgr.GetEventRecorderFor("kubeadmcontrolplane-controller")
	r.ssaCache = ssa.NewCache("kubeadmcontrolplane")

	if r.managementCluster == nil {
		r.managementCluster = &internal.Management{
			Client:              r.Client,
			SecretCachingClient: r.SecretCachingClient,
			ClusterCache:        r.ClusterCache,
			EtcdDialTimeout:     r.EtcdDialTimeout,
			EtcdCallTimeout:     r.EtcdCallTimeout,
			EtcdLogger:          r.EtcdLogger,
			ClientCertCache:     cache.New[internal.ClientCertEntry](24 * time.Hour),
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
		return ctrl.Result{}, err
	}

	// Add finalizer first if not set to avoid the race condition between init and delete.
	if finalizerAdded, err := finalizers.EnsureFinalizer(ctx, r.Client, kcp, controlplanev1.KubeadmControlPlaneFinalizer); err != nil || finalizerAdded {
		return ctrl.Result{}, err
	}

	// Fetch the Cluster.
	cluster, err := util.GetOwnerCluster(ctx, r.Client, kcp.ObjectMeta)
	if err != nil {
		// It should be an issue to be investigated if the controller get the NotFound status.
		// So, it should return the error.
		return ctrl.Result{}, errors.Wrapf(err, "failed to retrieve owner Cluster")
	}
	if cluster == nil {
		log.Info("Cluster Controller has not yet set OwnerRef")
		return ctrl.Result{}, nil
	}

	log = log.WithValues("Cluster", klog.KObj(cluster))
	ctx = ctrl.LoggerInto(ctx, log)

	// Initialize the patch helper.
	patchHelper, err := patch.NewHelper(kcp, r.Client)
	if err != nil {
		log.Error(err, "Failed to configure the patch helper")
		return ctrl.Result{Requeue: true}, nil
	}

	if isPaused, requeue, err := paused.EnsurePausedCondition(ctx, r.Client, cluster, kcp); err != nil || isPaused || requeue {
		return ctrl.Result{}, err
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
				log.Info(fmt.Sprintf("Could not connect to workload cluster to fetch status: %s", err.Error()))
			} else {
				reterr = kerrors.NewAggregate([]error{reterr, errors.Wrap(err, "failed to update KubeadmControlPlane status")})
			}
		}

		if err := r.updateV1Beta1Status(ctx, controlPlane); err != nil {
			var connFailure *internal.RemoteClusterConnectionError
			if errors.As(err, &connFailure) {
				log.Info(fmt.Sprintf("Could not connect to workload cluster to fetch deprecated v1beta1 status: %s", err.Error()))
			} else {
				reterr = kerrors.NewAggregate([]error{reterr, errors.Wrap(err, "failed to update KubeadmControlPlane deprecated v1beta1 status")})
			}
		}

		// Always attempt to Patch the KubeadmControlPlane object and status after each reconciliation.
		patchOpts := []patch.Option{}
		if reterr == nil {
			patchOpts = append(patchOpts, patch.WithStatusObservedGeneration{})
		}
		if err := patchKubeadmControlPlane(ctx, patchHelper, kcp, patchOpts...); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, errors.Wrap(err, "failed to patch KubeadmControlPlane")})
		}

		// Only requeue if there is no error, Requeue or RequeueAfter and the object does not have a deletion timestamp.
		if reterr == nil && res.IsZero() && kcp.DeletionTimestamp.IsZero() {
			// Make KCP requeue in case node status is not ready, so we can check for node status without waiting for a full
			// resync (by default 10 minutes).
			// The alternative solution would be to watch the control plane nodes in the Cluster - similar to how the
			// MachineSet and MachineHealthCheck controllers watch the nodes under their control.
			if !ptr.Deref(kcp.Status.Initialization.ControlPlaneInitialized, false) {
				res = ctrl.Result{RequeueAfter: 20 * time.Second}
			}

			// Make KCP requeue if ControlPlaneComponentsHealthyCondition is false so we can check for control plane component
			// status without waiting for a full resync (by default 10 minutes).
			// Otherwise this condition can lead to a delay in provisioning MachineDeployments when MachineSet preflight checks are enabled.
			// The alternative solution to this requeue would be watching the relevant pods inside each workload cluster which would be very expensive.
			if conditions.IsFalse(kcp, controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyCondition) {
				res = ctrl.Result{RequeueAfter: 20 * time.Second}
			}
		}
	}()

	if !kcp.DeletionTimestamp.IsZero() {
		// Handle deletion reconciliation loop.
		return r.reconcileDelete(ctx, controlPlane)
	}

	// Handle normal reconciliation loop.
	return r.reconcile(ctx, controlPlane)
}

// initControlPlaneScope initializes the control plane scope; this includes also checking for orphan machines and
// adopt them if necessary.
// The func also returns a boolean indicating if adoptableMachine have been found and processed, but this doesn't imply those machines
// have been actually adopted).
func (r *KubeadmControlPlaneReconciler) initControlPlaneScope(ctx context.Context, cluster *clusterv1.Cluster, kcp *controlplanev1.KubeadmControlPlane) (*internal.ControlPlane, bool, error) {
	log := ctrl.LoggerFrom(ctx)

	// Return early if the cluster is not yet in a state where control plane machines exists
	if !ptr.Deref(cluster.Status.Initialization.InfrastructureProvisioned, false) || !cluster.Spec.ControlPlaneEndpoint.IsValid() {
		controlPlane, err := internal.NewControlPlane(ctx, r.managementCluster, r.Client, cluster, kcp, collections.Machines{})
		if err != nil {
			log.Error(err, "Failed to initialize control plane scope")
			return nil, false, err
		}
		return controlPlane, false, nil
	}

	// Read control plane machines
	controlPlaneMachines, err := r.managementClusterUncached.GetMachinesForCluster(ctx, cluster, collections.ControlPlaneMachines(cluster.Name))
	if err != nil {
		log.Error(err, "Failed to retrieve control plane machines for cluster")
		return nil, false, err
	}

	// If we are not deleting the CP, adopt stand alone CP machines if any
	adoptableMachines := controlPlaneMachines.Filter(collections.AdoptableControlPlaneMachines(cluster.Name))
	if kcp.DeletionTimestamp.IsZero() && len(adoptableMachines) > 0 {
		return nil, true, r.adoptMachines(ctx, kcp, adoptableMachines, cluster)
	}

	ownedMachines := controlPlaneMachines.Filter(collections.OwnedMachines(kcp, controlplanev1.GroupVersion.WithKind("KubeadmControlPlane").GroupKind()))
	if kcp.DeletionTimestamp.IsZero() && len(ownedMachines) != len(controlPlaneMachines) {
		err := errors.New("not all control plane machines are owned by this KubeadmControlPlane, refusing to operate in mixed management mode")
		log.Error(err, "KCP cannot reconcile")
		return nil, false, err
	}

	controlPlane, err := internal.NewControlPlane(ctx, r.managementCluster, r.Client, cluster, kcp, ownedMachines)
	if err != nil {
		log.Error(err, "Failed to initialize control plane scope")
		return nil, false, err
	}
	return controlPlane, false, nil
}

func patchKubeadmControlPlane(ctx context.Context, patchHelper *patch.Helper, kcp *controlplanev1.KubeadmControlPlane, options ...patch.Option) error {
	// Always update the readyCondition by summarizing the state of other conditions.
	v1beta1conditions.SetSummary(kcp,
		v1beta1conditions.WithConditions(
			controlplanev1.MachinesCreatedV1Beta1Condition,
			controlplanev1.MachinesSpecUpToDateV1Beta1Condition,
			controlplanev1.ResizedV1Beta1Condition,
			controlplanev1.MachinesReadyV1Beta1Condition,
			controlplanev1.AvailableV1Beta1Condition,
			controlplanev1.CertificatesAvailableV1Beta1Condition,
		),
	)

	// Patch the object, ignoring conflicts on the conditions owned by this controller.
	// Also, if requested, we are adding additional options like e.g. Patch ObservedGeneration when issuing the
	// patch at the end of the reconcile loop.
	options = append(options,
		patch.WithOwnedV1Beta1Conditions{Conditions: []clusterv1.ConditionType{
			controlplanev1.MachinesCreatedV1Beta1Condition,
			clusterv1.ReadyV1Beta1Condition,
			controlplanev1.MachinesSpecUpToDateV1Beta1Condition,
			controlplanev1.ResizedV1Beta1Condition,
			controlplanev1.MachinesReadyV1Beta1Condition,
			controlplanev1.AvailableV1Beta1Condition,
			controlplanev1.CertificatesAvailableV1Beta1Condition,
		}},
		patch.WithOwnedConditions{Conditions: []string{
			clusterv1.PausedCondition,
			controlplanev1.KubeadmControlPlaneAvailableCondition,
			controlplanev1.KubeadmControlPlaneInitializedCondition,
			controlplanev1.KubeadmControlPlaneCertificatesAvailableCondition,
			controlplanev1.KubeadmControlPlaneEtcdClusterHealthyCondition,
			controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyCondition,
			controlplanev1.KubeadmControlPlaneMachinesReadyCondition,
			controlplanev1.KubeadmControlPlaneMachinesUpToDateCondition,
			controlplanev1.KubeadmControlPlaneRollingOutCondition,
			controlplanev1.KubeadmControlPlaneScalingUpCondition,
			controlplanev1.KubeadmControlPlaneScalingDownCondition,
			controlplanev1.KubeadmControlPlaneRemediatingCondition,
			controlplanev1.KubeadmControlPlaneDeletingCondition,
		}},
	)

	return patchHelper.Patch(ctx, kcp, options...)
}

// reconcile handles KubeadmControlPlane reconciliation.
func (r *KubeadmControlPlaneReconciler) reconcile(ctx context.Context, controlPlane *internal.ControlPlane) (res ctrl.Result, reterr error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Reconcile KubeadmControlPlane")

	// Make sure to reconcile the external infrastructure reference.
	if err := r.reconcileExternalReference(ctx, controlPlane); err != nil {
		return ctrl.Result{}, err
	}

	// Wait for the cluster infrastructure to be ready before creating machines
	if !ptr.Deref(controlPlane.Cluster.Status.Initialization.InfrastructureProvisioned, false) {
		// Note: in future we might want to move this inside reconcileControlPlaneAndMachinesConditions.
		conditions.Set(controlPlane.KCP, metav1.Condition{
			Type:    controlplanev1.KubeadmControlPlaneEtcdClusterHealthyCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  controlplanev1.KubeadmControlPlaneEtcdClusterInspectionFailedReason,
			Message: "Waiting for Cluster status.infrastructureReady to be true",
		})

		conditions.Set(controlPlane.KCP, metav1.Condition{
			Type:    controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  controlplanev1.KubeadmControlPlaneControlPlaneComponentsInspectionFailedReason,
			Message: "Waiting for Cluster status.infrastructureReady to be true",
		})

		log.Info("Cluster infrastructure is not ready yet")
		return ctrl.Result{}, nil
	}

	// Reconcile cluster certificates.
	if err := r.reconcileClusterCertificates(ctx, controlPlane); err != nil {
		return ctrl.Result{}, err
	}

	// If ControlPlaneEndpoint is not set, return early
	if !controlPlane.Cluster.Spec.ControlPlaneEndpoint.IsValid() {
		// Note: in future we might want to move this inside reconcileControlPlaneAndMachinesConditions.
		conditions.Set(controlPlane.KCP, metav1.Condition{
			Type:    controlplanev1.KubeadmControlPlaneEtcdClusterHealthyCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  controlplanev1.KubeadmControlPlaneEtcdClusterInspectionFailedReason,
			Message: "Waiting for Cluster spec.controlPlaneEndpoint to be set",
		})

		conditions.Set(controlPlane.KCP, metav1.Condition{
			Type:    controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  controlplanev1.KubeadmControlPlaneControlPlaneComponentsInspectionFailedReason,
			Message: "Waiting for Cluster spec.controlPlaneEndpoint to be set",
		})

		log.Info("Cluster does not yet have a ControlPlaneEndpoint defined")
		return ctrl.Result{}, nil
	}

	// Generate Cluster Kubeconfig if needed
	if result, err := r.reconcileKubeconfig(ctx, controlPlane); err != nil || !result.IsZero() {
		if err != nil {
			log.Error(err, "Failed to reconcile Kubeconfig")
		}
		return result, err
	}

	if err := r.syncMachines(ctx, controlPlane); err != nil {
		// Note: If any of the calls got a NotFound error, it means that at least one Machine got deleted.
		// Let's return here so that the next Reconcile will get the updated list of Machines.
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil // Note: Requeue is not needed, changes to Machines trigger another reconcile.
		}
		return ctrl.Result{}, errors.Wrap(err, "failed to sync Machines")
	}

	// Aggregate the operational state of all the machines; while aggregating we are adding the
	// source ref (reason@machine/name) so the problem can be easily tracked down to its source machine.
	v1beta1conditions.SetAggregate(controlPlane.KCP, controlplanev1.MachinesReadyV1Beta1Condition, controlPlane.Machines.ConditionGetters(), v1beta1conditions.AddSourceRef())

	// Updates conditions reporting the status of static pods and the status of the etcd cluster.
	// NOTE: Conditions reporting KCP operation progress like e.g. Resized or SpecUpToDate are inlined with the rest of the execution.
	if err := r.reconcileControlPlaneAndMachinesConditions(ctx, controlPlane); err != nil {
		return ctrl.Result{}, err
	}

	// Ensures the number of etcd members is in sync with the number of machines/nodes.
	// NOTE: This is usually required after a machine deletion.
	if err := r.reconcileEtcdMembers(ctx, controlPlane); err != nil {
		return ctrl.Result{}, err
	}

	// Handle machines in deletion phase; when drain and wait for volume detach completed, forward etcd leadership
	// and remove the etcd member, then unblock deletion.
	if result, err := r.reconcilePreTerminateHook(ctx, controlPlane); err != nil || !result.IsZero() {
		return result, err
	}

	// Complete triggering in-place update if necessary, for reentrancy if triggerInPlaceUpdate failed
	// when triggering the in-place update initially.
	if machines := controlPlane.MachinesToCompleteTriggerInPlaceUpdate(); len(machines) > 0 {
		_, machinesUpToDateResults := controlPlane.NotUpToDateMachines()
		for _, m := range machines {
			if err := r.triggerInPlaceUpdate(ctx, m, machinesUpToDateResults[m.Name]); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil // Note: Changes to Machines trigger another reconcile.
	}

	// Reconcile unhealthy machines by triggering deletion and requeue if it is considered safe to remediate,
	// otherwise continue with the other KCP operations.
	if result, err := r.reconcileUnhealthyMachines(ctx, controlPlane); err != nil || !result.IsZero() {
		return result, err
	}

	// Wait for in-place update to complete.
	// Note: If a Machine becomes unhealthy during in-place update reconcileUnhealthyMachines above remediates it.
	// Note: We have to wait here even if there are no more Machines that need rollout (in-place update in
	// progress is not counted as needs rollout).
	if machines := controlPlane.MachinesToCompleteInPlaceUpdate(); machines.Len() > 0 {
		for _, machine := range machines {
			log.Info(fmt.Sprintf("Waiting for in-place update of Machine %s to complete", machine.Name), "Machine", klog.KObj(machine))
		}
		return ctrl.Result{}, nil // Note: Changes to Machines trigger another reconcile.
	}

	// Control plane machines rollout due to configuration changes (e.g. upgrades) takes precedence over other operations.
	machinesNeedingRollout, machinesUpToDateResults := controlPlane.MachinesNeedingRollout()
	switch {
	case len(machinesNeedingRollout) > 0:
		var allMessages []string
		machinesNeedingRolloutNames := machinesNeedingRollout.Names()
		slices.Sort(machinesNeedingRolloutNames)
		for _, name := range machinesNeedingRolloutNames {
			allMessages = append(allMessages, fmt.Sprintf("Machine %s needs rollout: %s", name, strings.Join(machinesUpToDateResults[name].LogMessages, ", ")))
		}
		log.Info(fmt.Sprintf("Machines need rollout: %s", strings.Join(machinesNeedingRolloutNames, ",")), "reason", strings.Join(allMessages, ", "))
		v1beta1conditions.MarkFalse(controlPlane.KCP, controlplanev1.MachinesSpecUpToDateV1Beta1Condition, controlplanev1.RollingUpdateInProgressV1Beta1Reason, clusterv1.ConditionSeverityWarning, "Rolling %d replicas with outdated spec (%d replicas up to date)", len(machinesNeedingRollout), len(controlPlane.Machines)-len(machinesNeedingRollout))
		return r.updateControlPlane(ctx, controlPlane, machinesNeedingRollout, machinesUpToDateResults)
	default:
		// make sure last upgrade operation is marked as completed.
		// NOTE: we are checking the condition already exists in order to avoid to set this condition at the first
		// reconciliation/before a rolling upgrade actually starts.
		if v1beta1conditions.Has(controlPlane.KCP, controlplanev1.MachinesSpecUpToDateV1Beta1Condition) {
			v1beta1conditions.MarkTrue(controlPlane.KCP, controlplanev1.MachinesSpecUpToDateV1Beta1Condition)
		}
	}

	// If we've made it this far, we can assume that all ownedMachines are up to date
	numMachines := len(controlPlane.Machines)
	desiredReplicas := int(*controlPlane.KCP.Spec.Replicas)

	switch {
	// We are creating the first replica
	case numMachines < desiredReplicas && numMachines == 0:
		// Create new Machine w/ init
		log.Info("Initializing control plane", "desired", desiredReplicas, "existing", numMachines)
		v1beta1conditions.MarkFalse(controlPlane.KCP, controlplanev1.AvailableV1Beta1Condition, controlplanev1.WaitingForKubeadmInitV1Beta1Reason, clusterv1.ConditionSeverityInfo, "")
		return r.initializeControlPlane(ctx, controlPlane)
	// We are scaling up
	case numMachines < desiredReplicas && numMachines > 0:
		// Create a new Machine w/ join
		log.Info("Scaling up control plane", "desired", desiredReplicas, "existing", numMachines)
		return r.scaleUpControlPlane(ctx, controlPlane)
	// We are scaling down
	case numMachines > desiredReplicas:
		log.Info("Scaling down control plane", "desired", desiredReplicas, "existing", numMachines)
		// The last parameter (i.e. machines needing to be rolled out) should always be empty here.
		// Pick the Machine that we should scale down.
		machineToDelete, err := selectMachineForInPlaceUpdateOrScaleDown(ctx, controlPlane, collections.Machines{})
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to select machine for scale down")
		}
		return r.scaleDownControlPlane(ctx, controlPlane, machineToDelete)
	}

	// Get the workload cluster client.
	workloadCluster, err := controlPlane.GetWorkloadCluster(ctx)
	if err != nil {
		log.V(2).Info("cannot get remote client to workload cluster, will requeue", "cause", err)
		return ctrl.Result{Requeue: true}, nil
	}

	// Update kube-proxy daemonset.
	if err := workloadCluster.UpdateKubeProxyImageInfo(ctx, controlPlane.KCP); err != nil {
		log.Error(err, "Failed to update kube-proxy daemonset")
		return ctrl.Result{}, err
	}

	// Update CoreDNS deployment.
	if err := workloadCluster.UpdateCoreDNS(ctx, controlPlane.KCP); err != nil {
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
	// Generate Cluster Certificates if needed
	clusterConfiguration := controlPlane.KCP.Spec.KubeadmConfigSpec.ClusterConfiguration.DeepCopy()
	certificates := secret.NewCertificatesForInitialControlPlane(clusterConfiguration)
	controllerRef := metav1.NewControllerRef(controlPlane.KCP, controlplanev1.GroupVersion.WithKind(kubeadmControlPlaneKind))
	if err := certificates.LookupOrGenerateCached(ctx, r.SecretCachingClient, r.Client, util.ObjectKey(controlPlane.Cluster), *controllerRef); err != nil {
		v1beta1conditions.MarkFalse(controlPlane.KCP, controlplanev1.CertificatesAvailableV1Beta1Condition, controlplanev1.CertificatesGenerationFailedV1Beta1Reason, clusterv1.ConditionSeverityWarning, "%s", err.Error())

		conditions.Set(controlPlane.KCP, metav1.Condition{
			Type:    controlplanev1.KubeadmControlPlaneCertificatesAvailableCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  controlplanev1.KubeadmControlPlaneCertificatesInternalErrorReason,
			Message: "Please check controller logs for errors",
		})
		return errors.Wrap(err, "error in look up or create cluster certificates")
	}

	if err := r.ensureCertificatesOwnerRef(ctx, certificates, *controllerRef); err != nil {
		conditions.Set(controlPlane.KCP, metav1.Condition{
			Type:    controlplanev1.KubeadmControlPlaneCertificatesAvailableCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  controlplanev1.KubeadmControlPlaneCertificatesInternalErrorReason,
			Message: "Please check controller logs for errors",
		})

		return errors.Wrap(err, "error in ensuring cluster certificates ownership")
	}

	v1beta1conditions.MarkTrue(controlPlane.KCP, controlplanev1.CertificatesAvailableV1Beta1Condition)

	conditions.Set(controlPlane.KCP, metav1.Condition{
		Type:   controlplanev1.KubeadmControlPlaneCertificatesAvailableCondition,
		Status: metav1.ConditionTrue,
		Reason: controlplanev1.KubeadmControlPlaneCertificatesAvailableReason,
	})
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
		controlPlane.DeletingReason = controlplanev1.KubeadmControlPlaneDeletingDeletionCompletedReason
		controlPlane.DeletingMessage = "Deletion completed"

		controllerutil.RemoveFinalizer(controlPlane.KCP, controlplanev1.KubeadmControlPlaneFinalizer)
		return ctrl.Result{}, nil
	}

	// Updates conditions reporting the status of static pods and the status of the etcd cluster.
	// NOTE: Ignoring failures given that we are deleting
	if err := r.reconcileControlPlaneAndMachinesConditions(ctx, controlPlane); err != nil {
		log.Error(err, "Failed to reconcile conditions")
	}

	// Aggregate the operational state of all the machines; while aggregating we are adding the
	// source ref (reason@machine/name) so the problem can be easily tracked down to its source machine.
	// However, during delete we are hiding the counter (1 of x) because it does not make sense given that
	// all the machines are deleted in parallel.
	v1beta1conditions.SetAggregate(controlPlane.KCP, controlplanev1.MachinesReadyV1Beta1Condition, controlPlane.Machines.ConditionGetters(), v1beta1conditions.AddSourceRef())

	// Gets all machines, not just control plane machines.
	allMachines, err := r.managementCluster.GetMachinesForCluster(ctx, controlPlane.Cluster)
	if err != nil {
		controlPlane.DeletingReason = controlplanev1.KubeadmControlPlaneDeletingInternalErrorReason
		controlPlane.DeletingMessage = "Please check controller logs for errors" //nolint:goconst // Not making this a constant for now
		return ctrl.Result{}, err
	}

	allMachinePools := &clusterv1.MachinePoolList{}
	// Get all machine pools.
	if feature.Gates.Enabled(feature.MachinePool) {
		allMachinePools, err = r.managementCluster.GetMachinePoolsForCluster(ctx, controlPlane.Cluster)
		if err != nil {
			controlPlane.DeletingReason = controlplanev1.KubeadmControlPlaneDeletingInternalErrorReason
			controlPlane.DeletingMessage = "Please check controller logs for errors"
			return ctrl.Result{}, err
		}
	}
	// Verify that only control plane machines remain
	if len(allMachines) != len(controlPlane.Machines) || len(allMachinePools.Items) != 0 {
		log.Info("Waiting for worker nodes to be deleted first")
		v1beta1conditions.MarkFalse(controlPlane.KCP, controlplanev1.ResizedV1Beta1Condition, clusterv1.DeletingV1Beta1Reason, clusterv1.ConditionSeverityInfo, "Waiting for worker nodes to be deleted first")

		controlPlane.DeletingReason = controlplanev1.KubeadmControlPlaneDeletingWaitingForWorkersDeletionReason
		names := objectsPendingDeleteNames(allMachines, allMachinePools, controlPlane.Cluster)
		for i := range names {
			names[i] = "* " + names[i]
		}
		controlPlane.DeletingMessage = fmt.Sprintf("KubeadmControlPlane deletion blocked because following objects still exist:\n%s", strings.Join(names, "\n"))
		return ctrl.Result{RequeueAfter: deleteRequeueAfter}, nil
	}

	// Delete control plane machines in parallel
	machines := controlPlane.Machines
	var errs []error
	for _, machineToDelete := range machines {
		log := log.WithValues("Machine", klog.KObj(machineToDelete))
		ctx := ctrl.LoggerInto(ctx, log)

		// During KCP deletion we don't care about forwarding etcd leadership or removing etcd members.
		// So we are removing the pre-terminate hook.
		// This is important because when deleting KCP we will delete all members of etcd and it's not possible
		// to forward etcd leadership without any member left after we went through the Machine deletion.
		// Also in this case the reconcileDelete code of the Machine controller won't execute Node drain
		// and wait for volume detach.
		if err := r.removePreTerminateHookAnnotationFromMachine(ctx, machineToDelete); err != nil {
			errs = append(errs, err)
			continue
		}

		if !machineToDelete.DeletionTimestamp.IsZero() {
			// Nothing to do, Machine already has deletionTimestamp set.
			continue
		}

		if err := r.Client.Delete(ctx, machineToDelete); err != nil && !apierrors.IsNotFound(err) {
			errs = append(errs, errors.Wrapf(err, "failed to delete control plane Machine %s", klog.KObj(machineToDelete)))
		}
		// Note: We intentionally log after Delete because we want this log line to show up only after DeletionTimestamp has been set.
		// Also, setting DeletionTimestamp doesn't mean the Machine is actually deleted (deletion takes some time).
		log.WithValues(controlPlane.StatusToLogKeyAndValues(nil, machineToDelete)...).
			Info("Deleting Machine (KubeadmControlPlane deleted)")
	}
	if len(errs) > 0 {
		err := kerrors.NewAggregate(errs)
		r.recorder.Eventf(controlPlane.KCP, corev1.EventTypeWarning, "FailedDelete",
			"Failed to delete control plane Machines for cluster %s control plane: %v", klog.KObj(controlPlane.Cluster), err)

		controlPlane.DeletingReason = controlplanev1.KubeadmControlPlaneDeletingInternalErrorReason
		controlPlane.DeletingMessage = "Please check controller logs for errors"
		return ctrl.Result{}, err
	}

	log.Info("Waiting for control plane Machines to not exist anymore")

	v1beta1conditions.MarkFalse(controlPlane.KCP, controlplanev1.ResizedV1Beta1Condition, clusterv1.DeletingV1Beta1Reason, clusterv1.ConditionSeverityInfo, "")

	message := ""
	if len(machines) > 0 {
		if len(machines) == 1 {
			message = fmt.Sprintf("Deleting %d Machine", len(machines))
		} else {
			message = fmt.Sprintf("Deleting %d Machines", len(machines))
		}
		staleMessage := aggregateStaleMachines(machines)
		if staleMessage != "" {
			message += fmt.Sprintf(" and %s", staleMessage)
		}
	}
	controlPlane.DeletingReason = controlplanev1.KubeadmControlPlaneDeletingWaitingForMachineDeletionReason
	controlPlane.DeletingMessage = message
	return ctrl.Result{RequeueAfter: deleteRequeueAfter}, nil
}

// objectsPendingDeleteNames return the names of worker Machines and MachinePools pending delete.
func objectsPendingDeleteNames(allMachines collections.Machines, allMachinePools *clusterv1.MachinePoolList, cluster *clusterv1.Cluster) []string {
	controlPlaneMachines := allMachines.Filter(collections.ControlPlaneMachines(cluster.Name))
	workerMachines := allMachines.Difference(controlPlaneMachines)

	descendants := make([]string, 0)
	if feature.Gates.Enabled(feature.MachinePool) {
		machinePoolNames := make([]string, len(allMachinePools.Items))
		for i, machinePool := range allMachinePools.Items {
			machinePoolNames[i] = machinePool.Name
		}
		if len(machinePoolNames) > 0 {
			sort.Strings(machinePoolNames)
			descendants = append(descendants, "MachinePools: "+clog.StringListToString(machinePoolNames))
		}
	}

	workerMachineNames := make([]string, len(workerMachines))
	for i, workerMachine := range workerMachines.UnsortedList() {
		workerMachineNames[i] = workerMachine.Name
	}
	if len(workerMachineNames) > 0 {
		sort.Strings(workerMachineNames)
		descendants = append(descendants, "Machines: "+clog.StringListToString(workerMachineNames))
	}
	return descendants
}

func (r *KubeadmControlPlaneReconciler) removePreTerminateHookAnnotationFromMachine(ctx context.Context, machine *clusterv1.Machine) error {
	if _, exists := machine.Annotations[controlplanev1.PreTerminateHookCleanupAnnotation]; !exists {
		// Nothing to do, the annotation is not set (anymore) on the Machine
		return nil
	}

	log := ctrl.LoggerFrom(ctx)
	log.Info("Removing pre-terminate hook from control plane Machine")

	machineOriginal := machine.DeepCopy()
	delete(machine.Annotations, controlplanev1.PreTerminateHookCleanupAnnotation)
	if err := r.Client.Patch(ctx, machine, client.MergeFrom(machineOriginal)); err != nil {
		return errors.Wrapf(err, "failed to remove pre-terminate hook from control plane Machine %s", klog.KObj(machine))
	}
	return nil
}

// ClusterToKubeadmControlPlane is a handler.ToRequestsFunc to be used to enqueue requests for reconciliation
// for KubeadmControlPlane based on updates to a Cluster.
func (r *KubeadmControlPlaneReconciler) ClusterToKubeadmControlPlane(_ context.Context, o client.Object) []ctrl.Request {
	c, ok := o.(*clusterv1.Cluster)
	if !ok {
		panic(fmt.Sprintf("Expected a Cluster but got a %T", o))
	}

	if c.Spec.ControlPlaneRef.Kind == kubeadmControlPlaneKind {
		return []ctrl.Request{{NamespacedName: client.ObjectKey{Namespace: c.Namespace, Name: c.Spec.ControlPlaneRef.Name}}}
	}

	return nil
}

// syncMachines updates Machines, InfrastructureMachines and KubeadmConfigs to propagate in-place mutable fields from KCP.
func (r *KubeadmControlPlaneReconciler) syncMachines(ctx context.Context, controlPlane *internal.ControlPlane) error {
	patchHelpers := map[string]*patch.Helper{}
	for machineName := range controlPlane.Machines {
		m := controlPlane.Machines[machineName]
		// If the Machine is already being deleted, we only need to sync
		// the subset of fields that impact tearing down the Machine.
		if !m.DeletionTimestamp.IsZero() {
			patchHelper, err := patch.NewHelper(m, r.Client)
			if err != nil {
				return err
			}

			// Set all other in-place mutable fields that impact the ability to tear down existing machines.
			m.Spec.Deletion.NodeDrainTimeoutSeconds = controlPlane.KCP.Spec.MachineTemplate.Spec.Deletion.NodeDrainTimeoutSeconds
			m.Spec.Deletion.NodeDeletionTimeoutSeconds = controlPlane.KCP.Spec.MachineTemplate.Spec.Deletion.NodeDeletionTimeoutSeconds
			m.Spec.Deletion.NodeVolumeDetachTimeoutSeconds = controlPlane.KCP.Spec.MachineTemplate.Spec.Deletion.NodeVolumeDetachTimeoutSeconds

			// Note: We intentionally don't set "minReadySeconds" on Machines because we consider it enough to have machine availability driven by readiness of control plane components.
			if err := patchHelper.Patch(ctx, m); err != nil {
				return err
			}

			controlPlane.Machines[machineName] = m
			patchHelper, err = patch.NewHelper(m, r.Client)
			if err != nil {
				return err
			}
			patchHelpers[machineName] = patchHelper
			continue
		}

		// Update Machine to propagate in-place mutable fields from KCP.
		updatedMachine, err := r.updateMachine(ctx, m, controlPlane.KCP, controlPlane.Cluster)
		if err != nil {
			return errors.Wrapf(err, "failed to update Machine: %s", klog.KObj(m))
		}
		// Note: Ensure ControlPlane has the latest version of the Machine. This is required because
		//       e.g. the in-place update code that is called later has to use the latest version of the Machine.
		controlPlane.Machines[machineName] = updatedMachine
		if _, ok := controlPlane.MachinesNotUpToDate[machineName]; ok {
			controlPlane.MachinesNotUpToDate[machineName] = updatedMachine
		}
		// Since the machine is updated, re-create the patch helper so that any subsequent
		// Patch calls use the correct base machine object to calculate the diffs.
		// Example: reconcileControlPlaneAndMachinesConditions patches the machine objects in a subsequent call
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

		infraMachine, infraMachineFound := controlPlane.InfraResources[machineName]
		// Only update the InfraMachine if it is already found, otherwise just skip it.
		// This could happen e.g. if the cache is not up-to-date yet.
		if infraMachineFound {
			// Drop managedFields for manager:Update and capi-kubeadmcontrolplane:Apply for all objects created with CAPI <= v1.11.
			// Starting with CAPI v1.12 we have a new managedField structure where capi-kubeadmcontrolplane-metadata will own
			// labels and annotations and capi-kubeadmcontrolplane everything else.
			// Note: We have to call ssa.MigrateManagedFields for every Machine created with CAPI <= v1.11 once.
			//       Given that this was introduced in CAPI v1.12 and our n-3 upgrade policy this can
			//       be removed with CAPI v1.15.
			if err := ssa.MigrateManagedFields(ctx, r.Client, infraMachine, kcpManagerName, kcpMetadataManagerName); err != nil {
				return errors.Wrapf(err, "failed to clean up managedFields of InfrastructureMachine %s", klog.KObj(infraMachine))
			}
			// Update in-place mutating fields on InfrastructureMachine.
			if err := r.updateLabelsAndAnnotations(ctx, infraMachine, infraMachine.GroupVersionKind(), controlPlane.KCP, controlPlane.Cluster); err != nil {
				return errors.Wrapf(err, "failed to update InfrastructureMachine %s", klog.KObj(infraMachine))
			}
		}

		kubeadmConfig, kubeadmConfigFound := controlPlane.KubeadmConfigs[machineName]
		// Only update the KubeadmConfig if it is already found, otherwise just skip it.
		// This could happen e.g. if the cache is not up-to-date yet.
		if kubeadmConfigFound {
			// Drop managedFields for manager:Update and capi-kubeadmcontrolplane:Apply for all objects created with CAPI <= v1.11.
			// Starting with CAPI v1.12 we have a new managedField structure where capi-kubeadmcontrolplane-metadata will own
			// labels and annotations and capi-kubeadmcontrolplane everything else.
			// Note: We have to call ssa.MigrateManagedFields for every Machine created with CAPI <= v1.11 once.
			//       Given that this was introduced in CAPI v1.12 and our n-3 upgrade policy this can
			//       be removed with CAPI v1.15.
			if err := ssa.MigrateManagedFields(ctx, r.Client, kubeadmConfig, kcpManagerName, kcpMetadataManagerName); err != nil {
				return errors.Wrapf(err, "failed to clean up managedFields of KubeadmConfig %s", klog.KObj(kubeadmConfig))
			}
			// Update in-place mutating fields on BootstrapConfig.
			if err := r.updateLabelsAndAnnotations(ctx, kubeadmConfig, bootstrapv1.GroupVersion.WithKind("KubeadmConfig"), controlPlane.KCP, controlPlane.Cluster); err != nil {
				return errors.Wrapf(err, "failed to update KubeadmConfig %s", klog.KObj(kubeadmConfig))
			}
		}
	}
	// Update the patch helpers.
	controlPlane.SetPatchHelpers(patchHelpers)
	return nil
}

// reconcileControlPlaneAndMachinesConditions is responsible of reconciling conditions reporting the status of static pods and
// the status of the etcd cluster both on the KubeadmControlPlane and on machines.
// It also reconciles the UpToDate condition on Machines, so we can update them with a single patch operation.
func (r *KubeadmControlPlaneReconciler) reconcileControlPlaneAndMachinesConditions(ctx context.Context, controlPlane *internal.ControlPlane) (reterr error) {
	defer func() {
		// Patch machines with the updated conditions.
		reterr = kerrors.NewAggregate([]error{reterr, controlPlane.PatchMachines(ctx)})
	}()

	// Always reconcile machine's UpToDate condition
	reconcileMachineUpToDateCondition(ctx, controlPlane)

	// If the cluster is not yet initialized, there is no way to connect to the workload cluster and fetch information
	// for updating conditions. Return early.
	// We additionally check for the ControlPlaneInitialized condition. The ControlPlaneInitialized condition is set at the same time
	// as .status.initialization.controlPlaneInitialized and is never changed to false again. Below we'll need the transition time of the
	// ControlPlaneInitialized condition to check if the remote conditions grace period is already reached.
	// Note: The Machine controller uses the ControlPlaneInitialized condition on the Cluster instead for
	// the same check. We don't use the ControlPlaneInitialized condition from the Cluster here because KCP
	// Reconcile does (currently) not get triggered from condition changes to the Cluster object.
	controlPlaneInitialized := conditions.Get(controlPlane.KCP, controlplanev1.KubeadmControlPlaneInitializedCondition)
	if !ptr.Deref(controlPlane.KCP.Status.Initialization.ControlPlaneInitialized, false) ||
		controlPlaneInitialized == nil || controlPlaneInitialized.Status != metav1.ConditionTrue {
		// Overwrite conditions to InspectionFailed.
		setConditionsToUnknown(setConditionsToUnknownInput{
			ControlPlane:                        controlPlane,
			Overwrite:                           true,
			EtcdClusterHealthyReason:            controlplanev1.KubeadmControlPlaneEtcdClusterInspectionFailedReason,
			ControlPlaneComponentsHealthyReason: controlplanev1.KubeadmControlPlaneControlPlaneComponentsInspectionFailedReason,
			StaticPodReason:                     controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason,
			EtcdMemberHealthyReason:             controlplanev1.KubeadmControlPlaneMachineEtcdMemberInspectionFailedReason,
			Message:                             "Waiting for Cluster control plane to be initialized",
		})
		return nil
	}

	healthCheckingState := r.ClusterCache.GetHealthCheckingState(ctx, client.ObjectKeyFromObject(controlPlane.Cluster))

	// ClusterCache did not try to connect often enough yet, either during controller startup or when a new Cluster is created.
	if healthCheckingState.LastProbeSuccessTime.IsZero() && healthCheckingState.ConsecutiveFailures < 5 {
		// If conditions are not set, set them to ConnectionDown.
		// Note: This will allow to keep reporting last known status in case there are temporary connection errors.
		setConditionsToUnknown(setConditionsToUnknownInput{
			ControlPlane:                        controlPlane,
			Overwrite:                           false, // Don't overwrite.
			EtcdClusterHealthyReason:            controlplanev1.KubeadmControlPlaneEtcdClusterConnectionDownReason,
			ControlPlaneComponentsHealthyReason: controlplanev1.KubeadmControlPlaneControlPlaneComponentsConnectionDownReason,
			StaticPodReason:                     controlplanev1.KubeadmControlPlaneMachinePodConnectionDownReason,
			EtcdMemberHealthyReason:             controlplanev1.KubeadmControlPlaneMachineEtcdMemberConnectionDownReason,
			Message:                             "Remote connection not established yet",
		})
		return errors.Errorf("connection to the workload cluster not established yet")
	}

	// Remote conditions grace period is counted from the later of last probe success and control plane initialized.
	if time.Since(maxTime(healthCheckingState.LastProbeSuccessTime, controlPlaneInitialized.LastTransitionTime.Time)) > r.RemoteConditionsGracePeriod {
		// Overwrite conditions to ConnectionDown.
		setConditionsToUnknown(setConditionsToUnknownInput{
			ControlPlane:                        controlPlane,
			Overwrite:                           true,
			EtcdClusterHealthyReason:            controlplanev1.KubeadmControlPlaneEtcdClusterConnectionDownReason,
			ControlPlaneComponentsHealthyReason: controlplanev1.KubeadmControlPlaneControlPlaneComponentsConnectionDownReason,
			StaticPodReason:                     controlplanev1.KubeadmControlPlaneMachinePodConnectionDownReason,
			EtcdMemberHealthyReason:             controlplanev1.KubeadmControlPlaneMachineEtcdMemberConnectionDownReason,
			Message:                             lastProbeSuccessMessage(healthCheckingState.LastProbeSuccessTime),
		})
		return errors.Errorf("connection to the workload cluster is down")
	}

	workloadCluster, err := controlPlane.GetWorkloadCluster(ctx)
	if err != nil {
		if errors.Is(err, clustercache.ErrClusterNotConnected) {
			// If conditions are not set, set them to ConnectionDown.
			// Note: This will allow to keep reporting last known status in case there are temporary connection errors.
			// However, if connection errors persist more than r.RemoteConditionsGracePeriod, conditions will be overridden.
			// Note: Usually EtcdClusterHealthy and ControlPlaneComponentsHealthy have already been set before we reach this code,
			// which means that usually we don't set any conditions here (because we use Overwrite: false).
			setConditionsToUnknown(setConditionsToUnknownInput{
				ControlPlane:                        controlPlane,
				Overwrite:                           false, // Don't overwrite.
				EtcdClusterHealthyReason:            controlplanev1.KubeadmControlPlaneEtcdClusterConnectionDownReason,
				ControlPlaneComponentsHealthyReason: controlplanev1.KubeadmControlPlaneControlPlaneComponentsConnectionDownReason,
				StaticPodReason:                     controlplanev1.KubeadmControlPlaneMachinePodConnectionDownReason,
				EtcdMemberHealthyReason:             controlplanev1.KubeadmControlPlaneMachineEtcdMemberConnectionDownReason,
				Message:                             lastProbeSuccessMessage(healthCheckingState.LastProbeSuccessTime),
			})
			return errors.Wrap(err, "cannot get client for the workload cluster")
		}

		// Overwrite conditions to InspectionFailed.
		setConditionsToUnknown(setConditionsToUnknownInput{
			ControlPlane:                        controlPlane,
			Overwrite:                           true,
			EtcdClusterHealthyReason:            controlplanev1.KubeadmControlPlaneEtcdClusterInspectionFailedReason,
			ControlPlaneComponentsHealthyReason: controlplanev1.KubeadmControlPlaneControlPlaneComponentsInspectionFailedReason,
			StaticPodReason:                     controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason,
			EtcdMemberHealthyReason:             controlplanev1.KubeadmControlPlaneMachineEtcdMemberInspectionFailedReason,
			Message:                             "Please check controller logs for errors",
		})
		return errors.Wrap(err, "cannot get client for the workload cluster")
	}

	// Update conditions status
	workloadCluster.UpdateStaticPodConditions(ctx, controlPlane)
	workloadCluster.UpdateEtcdConditions(ctx, controlPlane)

	// KCP will be patched at the end of Reconcile to reflect updated conditions, so we can return now.
	return nil
}

func reconcileMachineUpToDateCondition(_ context.Context, controlPlane *internal.ControlPlane) {
	machinesNotUptoDate, machinesUpToDateResults := controlPlane.NotUpToDateMachines()
	machinesNotUptoDateNames := sets.New(machinesNotUptoDate.Names()...)

	for _, machine := range controlPlane.Machines {
		if machinesNotUptoDateNames.Has(machine.Name) {
			// Note: the code computing the message for KCP's RolloutOut condition is making assumptions on the format/content of this message.
			message := ""
			if machineUpToDateResult, ok := machinesUpToDateResults[machine.Name]; ok && len(machineUpToDateResult.ConditionMessages) > 0 {
				var reasons []string
				for _, conditionMessage := range machineUpToDateResult.ConditionMessages {
					reasons = append(reasons, fmt.Sprintf("* %s", conditionMessage))
				}
				message = strings.Join(reasons, "\n")
			}

			conditions.Set(machine, metav1.Condition{
				Type:    clusterv1.MachineUpToDateCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineNotUpToDateReason,
				Message: message,
			})
			continue
		}

		if inplace.IsUpdateInProgress(machine) {
			msg := "* In-place update in progress"
			if c := conditions.Get(machine, clusterv1.MachineUpdatingCondition); c != nil && c.Status == metav1.ConditionTrue && c.Message != "" {
				msg = fmt.Sprintf("* %s", c.Message)
			}
			conditions.Set(machine, metav1.Condition{
				Type:    clusterv1.MachineUpToDateCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineUpToDateUpdatingReason,
				Message: msg,
			})
			continue
		}

		conditions.Set(machine, metav1.Condition{
			Type:   clusterv1.MachineUpToDateCondition,
			Status: metav1.ConditionTrue,
			Reason: clusterv1.MachineUpToDateReason,
		})
	}
}

type setConditionsToUnknownInput struct {
	ControlPlane                        *internal.ControlPlane
	Overwrite                           bool
	EtcdClusterHealthyReason            string
	ControlPlaneComponentsHealthyReason string
	StaticPodReason                     string
	EtcdMemberHealthyReason             string
	Message                             string
}

func setConditionsToUnknown(input setConditionsToUnknownInput) {
	// Note: We are not checking if conditions on the Machines are already set, we just check the KCP conditions instead.
	// This means if Overwrite is set to false, we only set the EtcdMemberHealthy condition if the EtcdClusterHealthy condition is not set.
	// The same applies to ControlPlaneComponentsHealthy and the control plane component conditions on the Machines.
	etcdClusterHealthySet := conditions.Has(input.ControlPlane.KCP, controlplanev1.KubeadmControlPlaneEtcdClusterHealthyCondition)
	controlPlaneComponentsHealthySet := conditions.Has(input.ControlPlane.KCP, controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyCondition)

	if input.Overwrite || !etcdClusterHealthySet {
		conditions.Set(input.ControlPlane.KCP, metav1.Condition{
			Type:    controlplanev1.KubeadmControlPlaneEtcdClusterHealthyCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  input.EtcdClusterHealthyReason,
			Message: input.Message,
		})
		for _, machine := range input.ControlPlane.Machines {
			if input.ControlPlane.IsEtcdManaged() {
				conditions.Set(machine, metav1.Condition{
					Type:    controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  input.EtcdMemberHealthyReason,
					Message: input.Message,
				})
			}
		}
	}

	if input.Overwrite || !controlPlaneComponentsHealthySet {
		conditions.Set(input.ControlPlane.KCP, metav1.Condition{
			Type:    controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  input.ControlPlaneComponentsHealthyReason,
			Message: input.Message,
		})

		allMachinePodV1beta2Conditions := []string{
			controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition,
			controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition,
			controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition,
		}
		if input.ControlPlane.IsEtcdManaged() {
			allMachinePodV1beta2Conditions = append(allMachinePodV1beta2Conditions, controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition)
		}
		for _, machine := range input.ControlPlane.Machines {
			for _, condition := range allMachinePodV1beta2Conditions {
				conditions.Set(machine, metav1.Condition{
					Type:    condition,
					Status:  metav1.ConditionUnknown,
					Reason:  input.StaticPodReason,
					Message: input.Message,
				})
			}
		}
	}
}

func lastProbeSuccessMessage(lastProbeSuccessTime time.Time) string {
	if lastProbeSuccessTime.IsZero() {
		return ""
	}
	return fmt.Sprintf("Last successful probe at %s", lastProbeSuccessTime.Format(time.RFC3339))
}

func maxTime(t1, t2 time.Time) time.Time {
	if t1.After(t2) {
		return t1
	}
	return t2
}

// reconcileEtcdMembers ensures the number of etcd members is in sync with the number of machines/nodes.
// This is usually required after a machine deletion.
//
// NOTE: this func uses KCP conditions, it is required to call reconcileControlPlaneAndMachinesConditions before this.
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

	// No op if for any reason the etcdMember list is not populated at this stage.
	if controlPlane.EtcdMembers == nil {
		return nil
	}

	// Potential inconsistencies between the list of members and the list of machines/nodes are
	// surfaced using the EtcdClusterHealthyCondition; if this condition is true, meaning no inconsistencies exists, return early.
	if conditions.IsTrue(controlPlane.KCP, controlplanev1.KubeadmControlPlaneEtcdClusterHealthyCondition) {
		return nil
	}

	// Collect all the node names.
	// Note: EtcdClusterHealthyCondition true also implies that there are no machines still provisioning,
	// so we can ignore this case.
	nodeNames := []string{}
	for _, machine := range controlPlane.Machines {
		if !machine.Status.NodeRef.IsDefined() {
			// If there are provisioning machines (machines without a node yet), return.
			return nil
		}
		nodeNames = append(nodeNames, machine.Status.NodeRef.Name)
	}

	workloadCluster, err := controlPlane.GetWorkloadCluster(ctx)
	if err != nil {
		// Failing at connecting to the workload cluster can mean workload cluster is unhealthy for a variety of reasons such as etcd quorum loss.
		return errors.Wrap(err, "cannot get remote client to workload cluster")
	}

	removedMembers, err := workloadCluster.ReconcileEtcdMembersAndControlPlaneNodes(ctx, controlPlane.EtcdMembers, nodeNames)
	if err != nil {
		return errors.Wrap(err, "failed attempt to reconcile etcd members")
	}

	if len(removedMembers) > 0 {
		log.Info("Etcd members without nodes removed from the cluster", "members", removedMembers)
	}

	return nil
}

func (r *KubeadmControlPlaneReconciler) reconcilePreTerminateHook(ctx context.Context, controlPlane *internal.ControlPlane) (ctrl.Result, error) {
	if !controlPlane.HasDeletingMachine() {
		return ctrl.Result{}, nil
	}

	log := ctrl.LoggerFrom(ctx)

	// Return early, if there is already a deleting Machine without the pre-terminate hook.
	// We are going to wait until this Machine goes away before running the pre-terminate hook on other Machines.
	for _, deletingMachine := range controlPlane.DeletingMachines() {
		if _, exists := deletingMachine.Annotations[controlplanev1.PreTerminateHookCleanupAnnotation]; !exists {
			return ctrl.Result{RequeueAfter: deleteRequeueAfter}, nil
		}
	}

	// Pick the Machine with the oldest deletionTimestamp to keep this function deterministic / reentrant
	// so we only remove the pre-terminate hook from one Machine at a time.
	deletingMachine := controlPlane.DeletingMachines().OldestDeletionTimestamp()
	log = log.WithValues("Machine", klog.KObj(deletingMachine))
	ctx = ctrl.LoggerInto(ctx, log)

	parsedVersion, err := semver.ParseTolerant(controlPlane.KCP.Spec.Version)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to parse Kubernetes version %q", controlPlane.KCP.Spec.Version)
	}

	// Return early if there are other pre-terminate hooks for the Machine.
	// The KCP pre-terminate hook should be the one executed last, so that kubelet
	// is still working while other pre-terminate hooks are run.
	// Note: This is done only for Kubernetes >= v1.31 to reduce the blast radius of this check.
	if version.Compare(parsedVersion, semver.MustParse("1.31.0"), version.WithoutPreReleases()) >= 0 {
		if machineHasOtherPreTerminateHooks(deletingMachine) {
			return ctrl.Result{RequeueAfter: deleteRequeueAfter}, nil
		}
	}

	// Return early because the Machine controller is not yet waiting for the pre-terminate hook.
	c := conditions.Get(deletingMachine, clusterv1.MachineDeletingCondition)
	if c == nil || c.Status != metav1.ConditionTrue || c.Reason != clusterv1.MachineDeletingWaitingForPreTerminateHookReason {
		return ctrl.Result{RequeueAfter: deleteRequeueAfter}, nil
	}

	// The following will execute and remove the pre-terminate hook from the Machine.

	// If we have more than 1 Machine and etcd is managed we forward etcd leadership and remove the member
	// to keep the etcd cluster healthy.
	if controlPlane.Machines.Len() > 1 && controlPlane.IsEtcdManaged() {
		workloadCluster, err := controlPlane.GetWorkloadCluster(ctx)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to remove etcd member for deleting Machine %s: failed to create client to workload cluster", klog.KObj(deletingMachine))
		}

		// Note: In regular deletion cases (remediation, scale down) the leader should have been already moved.
		// We're doing this again here in case the Machine became leader again or the Machine deletion was
		// triggered in another way (e.g. a user running kubectl delete machine)
		etcdLeaderCandidate := controlPlane.Machines.Filter(collections.Not(collections.HasDeletionTimestamp)).Newest()
		if etcdLeaderCandidate != nil {
			if err := workloadCluster.ForwardEtcdLeadership(ctx, deletingMachine, etcdLeaderCandidate); err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "failed to move leadership to candidate Machine %s", etcdLeaderCandidate.Name)
			}
		} else {
			log.Info("Skip forwarding etcd leadership, because there is no other control plane Machine without a deletionTimestamp")
		}

		// Note: Removing the etcd member will lead to the etcd and the kube-apiserver Pod on the Machine shutting down.
		// If ControlPlaneKubeletLocalMode is used, the kubelet is communicating with the local apiserver and thus now
		// won't be able to see any updates to e.g. Pods anymore.
		if err := workloadCluster.RemoveEtcdMemberForMachine(ctx, deletingMachine); err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to remove etcd member for deleting Machine %s", klog.KObj(deletingMachine))
		}
	}

	if err := r.removePreTerminateHookAnnotationFromMachine(ctx, deletingMachine); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Waiting for Machines to be deleted", "machines", strings.Join(controlPlane.Machines.Filter(collections.HasDeletionTimestamp).Names(), ", "))
	return ctrl.Result{RequeueAfter: deleteRequeueAfter}, nil
}

func machineHasOtherPreTerminateHooks(machine *clusterv1.Machine) bool {
	for k := range machine.Annotations {
		if strings.HasPrefix(k, clusterv1.PreTerminateDeleteHookAnnotationPrefix) && k != controlplanev1.PreTerminateHookCleanupAnnotation {
			return true
		}
	}
	return false
}

func (r *KubeadmControlPlaneReconciler) reconcileCertificateExpiries(ctx context.Context, controlPlane *internal.ControlPlane) error {
	log := ctrl.LoggerFrom(ctx)

	// Return if there are no KCP-owned control-plane machines.
	if controlPlane.Machines.Len() == 0 {
		return nil
	}

	// Return if KCP is not yet initialized (no API server to contact for checking certificate expiration).
	if !ptr.Deref(controlPlane.KCP.Status.Initialization.ControlPlaneInitialized, false) {
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

		if !m.Status.NodeRef.IsDefined() {
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
		if !ref.IsDefined() || ref.Kind != "KubeadmConfig" {
			return errors.Errorf("unable to adopt Machine %v/%v: expected a ConfigRef of kind KubeadmConfig but instead found %v", m.Namespace, m.Name, ref)
		}

		if m.Spec.Version == "" {
			// if the machine's version is not immediately apparent, assume the operator knows what they're doing
			continue
		}

		machineVersion, err := semver.ParseTolerant(m.Spec.Version)
		if err != nil {
			return errors.Wrapf(err, "failed to parse kubernetes version %q", m.Spec.Version)
		}

		if !util.IsSupportedVersionSkew(kcpVersion, machineVersion) {
			r.recorder.Eventf(kcp, corev1.EventTypeWarning, "AdoptionFailed", "Could not adopt Machine %s/%s: its version (%q) is outside supported +/- one minor version skew from KCP's (%q)", m.Namespace, m.Name, m.Spec.Version, kcp.Spec.Version)
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
		if !util.IsOwnedByObject(&s, currentOwner, bootstrapv1.GroupVersion.WithKind("KubeadmConfig").GroupKind()) {
			continue
		}
		// avoid taking ownership of the bootstrap data secret
		if s.Name == currentOwner.Status.DataSecretName {
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
