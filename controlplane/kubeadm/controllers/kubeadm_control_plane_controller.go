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
	"sort"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
	kubeadmv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/controllers/remote"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/hash"
	capierrors "sigs.k8s.io/cluster-api/errors"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/secret"
)

const (
	// DeleteRequeueAfter is how long to wait before checking again to see if
	// all control plane machines have been deleted.
	DeleteRequeueAfter = 30 * time.Second

	// HealthCheckFailedRequeueAfter is how long to wait before trying to scale
	// up/down if some target cluster health check has failed
	HealthCheckFailedRequeueAfter = 20 * time.Second
)

type managementCluster interface {
	GetMachinesForCluster(ctx context.Context, cluster types.NamespacedName, filters ...func(machine *clusterv1.Machine) bool) ([]*clusterv1.Machine, error)
	TargetClusterControlPlaneIsHealthy(ctx context.Context, clusterKey types.NamespacedName, controlPlaneName string) error
	TargetClusterEtcdIsHealthy(ctx context.Context, clusterKey types.NamespacedName, controlPlaneName string) error
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io;bootstrap.cluster.x-k8s.io;controlplane.cluster.x-k8s.io,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch;create;update;patch;delete

// KubeadmControlPlaneReconciler reconciles a KubeadmControlPlane object
type KubeadmControlPlaneReconciler struct {
	Client     client.Client
	Log        logr.Logger
	scheme     *runtime.Scheme
	controller controller.Controller
	recorder   record.EventRecorder

	remoteClientGetter remote.ClusterClientGetter

	managementCluster managementCluster
}

func (r *KubeadmControlPlaneReconciler) SetupWithManager(mgr ctrl.Manager, options controller.Options) error {
	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&controlplanev1.KubeadmControlPlane{}).
		Owns(&clusterv1.Machine{}).
		Watches(
			&source.Kind{Type: &clusterv1.Cluster{}},
			&handler.EnqueueRequestsFromMapFunc{ToRequests: handler.ToRequestsFunc(r.ClusterToKubeadmControlPlane)},
		).
		WithOptions(options).
		Build(r)

	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	r.scheme = mgr.GetScheme()
	r.controller = c
	r.recorder = mgr.GetEventRecorderFor("kubeadm-control-plane-controller")
	if r.remoteClientGetter == nil {
		r.remoteClientGetter = remote.NewClusterClient
	}

	return nil
}

func (r *KubeadmControlPlaneReconciler) Reconcile(req ctrl.Request) (res ctrl.Result, reterr error) {
	logger := r.Log.WithValues("kubeadmControlPlane", req.Name, "namespace", req.Namespace)
	ctx := context.Background()

	// Fetch the KubeadmControlPlane instance.
	kcp := &controlplanev1.KubeadmControlPlane{}
	if err := r.Client.Get(ctx, req.NamespacedName, kcp); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}

		// Error reading the object - requeue the request.
		logger.Error(err, "Failed to retrieve requested KubeadmControlPlane resource from the API Server")
		return ctrl.Result{Requeue: true}, nil
	}

	// Fetch the Cluster.
	cluster, err := util.GetOwnerCluster(ctx, r.Client, kcp.ObjectMeta)
	if err != nil {
		logger.Error(err, "Failed to retrieve owner Cluster from the API Server")
		return ctrl.Result{}, err
	}
	if cluster == nil {
		logger.Info("Cluster Controller has not yet set OwnerRef")
		return ctrl.Result{}, nil
	}
	logger = logger.WithValues("cluster", cluster.Name)

	if util.IsPaused(cluster, kcp) {
		logger.Info("Reconciliation is paused")
		return ctrl.Result{}, nil
	}
	r.managementCluster = &internal.ManagementCluster{Client: r.Client}

	// Wait for the cluster infrastructure to be ready before creating machines
	if !cluster.Status.InfrastructureReady {
		return ctrl.Result{}, nil
	}

	// Initialize the patch helper.
	patchHelper, err := patch.NewHelper(kcp, r.Client)
	if err != nil {
		logger.Error(err, "Failed to configure the patch helper")
		return ctrl.Result{Requeue: true}, nil
	}

	defer func() {
		// Always attempt to update status.
		if err := r.updateStatus(ctx, kcp, cluster); err != nil {
			logger.Error(err, "Failed to update KubeadmControlPlane Status")
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}

		// Always attempt to Patch the KubeadmControlPlane object and status after each reconciliation.
		if err := patchHelper.Patch(ctx, kcp); err != nil {
			logger.Error(err, "Failed to patch KubeadmControlPlane")
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}
	}()

	if !kcp.ObjectMeta.DeletionTimestamp.IsZero() {
		// Handle deletion reconciliation loop.
		return r.reconcileDelete(ctx, cluster, kcp, logger)
	}

	// Handle normal reconciliation loop.
	return r.reconcile(ctx, cluster, kcp, logger)
}

// reconcile handles KubeadmControlPlane reconciliation.
func (r *KubeadmControlPlaneReconciler) reconcile(ctx context.Context, cluster *clusterv1.Cluster, kcp *controlplanev1.KubeadmControlPlane, logger logr.Logger) (_ ctrl.Result, reterr error) {
	// If object doesn't have a finalizer, add one.
	controllerutil.AddFinalizer(kcp, controlplanev1.KubeadmControlPlaneFinalizer)

	// Make sure to reconcile the external infrastructure reference.
	if err := r.reconcileExternalReference(ctx, cluster, kcp.Spec.InfrastructureTemplate); err != nil {
		return ctrl.Result{}, err
	}

	// Generate Cluster Certificates if needed
	config := kcp.Spec.KubeadmConfigSpec.DeepCopy()
	config.JoinConfiguration = nil
	if config.ClusterConfiguration == nil {
		config.ClusterConfiguration = &kubeadmv1.ClusterConfiguration{}
	}
	certificates := secret.NewCertificatesForInitialControlPlane(config.ClusterConfiguration)
	controllerRef := metav1.NewControllerRef(kcp, controlplanev1.GroupVersion.WithKind("KubeadmControlPlane"))
	if err := certificates.LookupOrGenerate(ctx, r.Client, util.ObjectKey(cluster), *controllerRef); err != nil {
		logger.Error(err, "unable to lookup or create cluster certificates")
		return ctrl.Result{}, err
	}

	// If ControlPlaneEndpoint is not set, return early
	if cluster.Spec.ControlPlaneEndpoint.IsZero() {
		logger.Info("Cluster does not yet have a ControlPlaneEndpoint defined")
		return ctrl.Result{}, nil
	}

	// Generate Cluster Kubeconfig if needed
	if err := r.reconcileKubeconfig(ctx, util.ObjectKey(cluster), cluster.Spec.ControlPlaneEndpoint, kcp); err != nil {
		if requeueErr, ok := errors.Cause(err).(capierrors.HasRequeueAfterError); ok {
			logger.Error(err, "required certificates not found, requeueing")
			return ctrl.Result{
				RequeueAfter: requeueErr.GetRequeueAfter(),
			}, nil
		}
		logger.Error(err, "failed to reconcile Kubeconfig")
		return ctrl.Result{}, err
	}

	// TODO: handle proper adoption of Machines
	ownedMachines, err := r.managementCluster.GetMachinesForCluster(ctx, util.ObjectKey(cluster), internal.OwnedControlPlaneMachines(kcp.Name))
	if err != nil {
		return ctrl.Result{}, err
	}

	currentConfigurationHash := hash.Compute(&kcp.Spec)
	requireUpgrade := internal.FilterMachines(
		ownedMachines,
		internal.HasOutdatedConfiguration(currentConfigurationHash),
		internal.OlderThan(kcp.Spec.UpgradeAfter),
	)

	// Upgrade takes precedence over other operations
	if len(requireUpgrade) > 0 {
		logger.Info("Upgrading Control Plane")
		if err := r.upgradeControlPlane(ctx, cluster, kcp, requireUpgrade); err != nil {
			logger.Error(err, "Failed to upgrade the Control Plane")
			r.recorder.Eventf(kcp, corev1.EventTypeWarning, "FailedUpgrade", "Failed to upgrade the control plane: %v", err)
			return ctrl.Result{}, err
		}
		// TODO: need to requeue if there are additional operations to perform
		return ctrl.Result{}, nil
	}

	// If we've made it this far, we don't need to worry about Machines that are older than kcp.Spec.UpgradeAfter
	currentMachines := internal.FilterMachines(ownedMachines, internal.MatchesConfigurationHash(currentConfigurationHash))
	numMachines := len(currentMachines)
	desiredReplicas := int(*kcp.Spec.Replicas)

	switch {
	// We are creating the first replica
	case numMachines < desiredReplicas && numMachines == 0:
		// Create new Machine w/ init
		logger.Info("Initializing control plane", "Desired", desiredReplicas, "Existing", numMachines)
		result, err := r.initializeControlPlane(ctx, cluster, kcp)
		if err != nil {
			logger.Error(err, "Failed to initialize control plane")
			r.recorder.Eventf(kcp, corev1.EventTypeWarning, "FailedInitialization", "Failed to initialize cluster %s/%s control plane: %v", cluster.Namespace, cluster.Name, err)
		}
		// TODO: return the error if it is unexpected and should cause an immediate requeue
		return result, nil
	// We are scaling up
	case numMachines < desiredReplicas && numMachines > 0:
		// Create a new Machine w/ join
		logger.Info("Scaling up control plane", "Desired", desiredReplicas, "Existing", numMachines)
		result, err := r.scaleUpControlPlane(ctx, cluster, kcp)
		if err != nil {
			logger.Error(err, "Failed to scale up control plane")
			r.recorder.Eventf(kcp, corev1.EventTypeWarning, "FailedScaleUp", "Failed to scale up cluster %s/%s control plane: %v", cluster.Namespace, cluster.Name, err)
		}
		// TODO: return the error if it is unexpected and should cause an immediate requeue
		return result, nil
	// We are scaling down
	case numMachines > desiredReplicas:
		logger.Info("Scaling down control plane", "Desired", desiredReplicas, "Existing", numMachines)
		result, err := r.scaleDownControlPlane(ctx, cluster, kcp)
		if err != nil {
			logger.Error(err, "Failed to scale down control plane")
			r.recorder.Eventf(kcp, corev1.EventTypeWarning, "FailedScaleDown", "Failed to scale down cluster %s/%s control plane: %v", cluster.Namespace, cluster.Name, err)
		}
		// TODO: return the error if it is unexpected and should cause an immediate requeue
		return result, nil
	}

	return ctrl.Result{}, nil
}

func (r *KubeadmControlPlaneReconciler) updateStatus(ctx context.Context, kcp *controlplanev1.KubeadmControlPlane, cluster *clusterv1.Cluster) error {
	labelSelector := internal.ControlPlaneSelectorForCluster(cluster.Name)
	selector, err := metav1.LabelSelectorAsSelector(labelSelector)
	if err != nil {
		// Since we are building up the LabelSelector above, this should not fail
		return errors.Wrap(err, "failed to parse label selector")
	}
	// Copy label selector to its status counterpart in string format.
	// This is necessary for CRDs including scale subresources.
	kcp.Status.Selector = selector.String()

	ownedMachines, err := r.managementCluster.GetMachinesForCluster(ctx, util.ObjectKey(cluster), internal.OwnedControlPlaneMachines(kcp.Name))
	if err != nil {
		return errors.Wrap(err, "failed to get list of owned machines")
	}

	currentMachines := internal.FilterMachines(ownedMachines, internal.MatchesConfigurationHash(hash.Compute(&kcp.Spec)))
	kcp.Status.UpdatedReplicas = int32(len(currentMachines))

	replicas := int32(len(ownedMachines))
	kcp.Status.Replicas = replicas

	remoteClient, err := r.remoteClientGetter(ctx, r.Client, util.ObjectKey(cluster), r.scheme)
	if err != nil && !apierrors.IsNotFound(errors.Cause(err)) {
		return errors.Wrap(err, "failed to create remote cluster client")
	}

	readyMachines := int32(0)
	for i := range ownedMachines {
		node, err := getMachineNode(ctx, remoteClient, ownedMachines[i])
		if err != nil {
			return errors.Wrap(err, "failed to get referenced Node")
		}
		if node == nil {
			continue
		}
		if node.Spec.ProviderID != "" {
			readyMachines++
		}
	}
	kcp.Status.ReadyReplicas = readyMachines
	kcp.Status.UnavailableReplicas = replicas - readyMachines

	if !kcp.Status.Initialized {
		if kcp.Status.ReadyReplicas > 0 {
			kcp.Status.Initialized = true
		}
	}
	return nil
}

func (r *KubeadmControlPlaneReconciler) upgradeControlPlane(ctx context.Context, cluster *clusterv1.Cluster, kcp *controlplanev1.KubeadmControlPlane, requireUpgrade []*clusterv1.Machine) error {

	// TODO: verify health for each existing replica
	// TODO: mark an old Machine via the label kubeadm.controlplane.cluster.x-k8s.io/selected-for-upgrade
	// TODO: check full cluster health
	// TODO: provision new Machine to replace marked Machine
	// TODO: wait for health of all existing replicas, ideally in a re-entrant way to avoid waiting in process
	// TODO: wait for full cluster health, ideally in a re-entrant way to avoid waiting in process
	// TODO: Remove etcd membership for old Machine instance
	// TODO: Update the kubeadm configmap
	// TODO: Delete the Marked ControlPlane machine
	// TODO: Continue with next OldMachine

	return nil
}

func (r *KubeadmControlPlaneReconciler) initializeControlPlane(ctx context.Context, cluster *clusterv1.Cluster, kcp *controlplanev1.KubeadmControlPlane) (ctrl.Result, error) {
	bootstrapSpec := kcp.Spec.KubeadmConfigSpec.DeepCopy()
	bootstrapSpec.JoinConfiguration = nil

	if err := r.cloneConfigsAndGenerateMachine(ctx, cluster, kcp, bootstrapSpec); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to create control plane Machine for cluster %s/%s", cluster.Name, cluster.Namespace)
	}

	// Requeue the control plane, in case we are going to scale up
	return ctrl.Result{Requeue: true}, nil
}

func (r *KubeadmControlPlaneReconciler) scaleUpControlPlane(ctx context.Context, cluster *clusterv1.Cluster, kcp *controlplanev1.KubeadmControlPlane) (ctrl.Result, error) {
	if err := r.managementCluster.TargetClusterControlPlaneIsHealthy(ctx, util.ObjectKey(cluster), kcp.Name); err != nil {
		return ctrl.Result{RequeueAfter: HealthCheckFailedRequeueAfter}, errors.Wrap(err, "control plane is not healthy")
	}

	if err := r.managementCluster.TargetClusterEtcdIsHealthy(ctx, util.ObjectKey(cluster), kcp.Name); err != nil {
		return ctrl.Result{RequeueAfter: HealthCheckFailedRequeueAfter}, errors.Wrap(err, "etcd cluster is not healthy")
	}

	// Create the bootstrap configuration
	bootstrapSpec := kcp.Spec.KubeadmConfigSpec.DeepCopy()
	bootstrapSpec.InitConfiguration = nil
	bootstrapSpec.ClusterConfiguration = nil

	if err := r.cloneConfigsAndGenerateMachine(ctx, cluster, kcp, bootstrapSpec); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to create control plane Machine for cluster %s/%s", cluster.Name, cluster.Namespace)
	}

	// Requeue the control plane, in case we are not done scaling up
	return ctrl.Result{Requeue: true}, nil
}

func (r *KubeadmControlPlaneReconciler) scaleDownControlPlane(ctx context.Context, cluster *clusterv1.Cluster, kcp *controlplanev1.KubeadmControlPlane) (ctrl.Result, error) {
	if err := r.managementCluster.TargetClusterControlPlaneIsHealthy(ctx, util.ObjectKey(cluster), kcp.Name); err != nil {
		return ctrl.Result{RequeueAfter: HealthCheckFailedRequeueAfter}, errors.Wrap(err, "control plane is not healthy")
	}

	if err := r.managementCluster.TargetClusterEtcdIsHealthy(ctx, util.ObjectKey(cluster), kcp.Name); err != nil {
		return ctrl.Result{RequeueAfter: HealthCheckFailedRequeueAfter}, errors.Wrap(err, "etcd cluster is not healthy")
	}

	ownedMachines, err := r.managementCluster.GetMachinesForCluster(ctx, util.ObjectKey(cluster), internal.OwnedControlPlaneMachines(kcp.Name))
	if err != nil {
		return ctrl.Result{}, err
	}

	// Wait for any delete in progress to complete before deleting another Machine
	if len(internal.FilterMachines(ownedMachines, internal.HasDeletionTimestamp())) > 0 {
		return ctrl.Result{RequeueAfter: DeleteRequeueAfter}, nil
	}

	machineToDelete, err := oldestMachine(ownedMachines)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to pick control plane Machine to delete")
	}

	if err := r.Client.Delete(ctx, machineToDelete); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, errors.Wrapf(err, "failed to delete control plane Machine %s/%s", machineToDelete.Namespace, machineToDelete.Name)
	}

	// Requeue the control plane, in case we are not done scaling down
	return ctrl.Result{Requeue: true}, nil
}

func (r *KubeadmControlPlaneReconciler) cloneConfigsAndGenerateMachine(ctx context.Context, cluster *clusterv1.Cluster, kcp *controlplanev1.KubeadmControlPlane, bootstrapSpec *bootstrapv1.KubeadmConfigSpec) error {
	var errs []error

	// Since the cloned resource should eventually have a controller ref for the Machine, we create an
	// OwnerReference here without the Controller field set
	infraCloneOwner := &metav1.OwnerReference{
		APIVersion: controlplanev1.GroupVersion.String(),
		Kind:       "KubeadmControlPlane",
		Name:       kcp.Name,
		UID:        kcp.UID,
	}

	// Clone the infrastructure template
	infraRef, err := external.CloneTemplate(ctx, &external.CloneTemplateInput{
		Client:      r.Client,
		TemplateRef: &kcp.Spec.InfrastructureTemplate,
		Namespace:   kcp.Namespace,
		OwnerRef:    infraCloneOwner,
		ClusterName: cluster.Name,
		Labels:      internal.ControlPlaneLabelsForClusterWithHash(cluster.Name, hash.Compute(&kcp.Spec)),
	})
	if err != nil {
		// Safe to return early here since no resources have been created yet.
		return errors.Wrap(err, "failed to clone infrastructure template")
	}

	// Clone the bootstrap configuration
	bootstrapRef, err := r.generateKubeadmConfig(ctx, kcp, cluster, bootstrapSpec)
	if err != nil {
		errs = append(errs, errors.Wrap(err, "failed to generate bootstrap config"))
	}

	// Only proceed to generating the Machine if we haven't encountered an error
	if len(errs) == 0 {
		if err := r.generateMachine(ctx, kcp, cluster, infraRef, bootstrapRef); err != nil {
			errs = append(errs, errors.Wrap(err, "failed to create Machine"))
		}
	}

	// If we encountered any errors, attempt to clean up any dangling resources
	if len(errs) > 0 {
		if err := r.cleanupFromGeneration(ctx, infraRef, bootstrapRef); err != nil {
			errs = append(errs, errors.Wrap(err, "failed to cleanup generated resources"))
		}

		return kerrors.NewAggregate(errs)
	}

	return nil
}

func (r *KubeadmControlPlaneReconciler) cleanupFromGeneration(ctx context.Context, remoteRefs ...*corev1.ObjectReference) error {
	var errs []error

	for _, ref := range remoteRefs {
		if ref != nil {
			config := &unstructured.Unstructured{}
			config.SetKind(ref.Kind)
			config.SetAPIVersion(ref.APIVersion)
			config.SetNamespace(ref.Namespace)
			config.SetName(ref.Name)

			if err := r.Client.Delete(ctx, config); err != nil && !apierrors.IsNotFound(err) {
				errs = append(errs, errors.Wrap(err, "failed to cleanup generated resources after error"))
			}
		}
	}

	return kerrors.NewAggregate(errs)
}

func (r *KubeadmControlPlaneReconciler) generateKubeadmConfig(ctx context.Context, kcp *controlplanev1.KubeadmControlPlane, cluster *clusterv1.Cluster, spec *bootstrapv1.KubeadmConfigSpec) (*corev1.ObjectReference, error) {
	// Since the generated KubeadmConfig should eventually have a controller ref for the Machine, we create an
	// OwnerReference here without the Controller field set
	owner := metav1.OwnerReference{
		APIVersion: controlplanev1.GroupVersion.String(),
		Kind:       "KubeadmControlPlane",
		Name:       kcp.Name,
		UID:        kcp.UID,
	}

	bootstrapConfig := &bootstrapv1.KubeadmConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:            names.SimpleNameGenerator.GenerateName(kcp.Name + "-"),
			Namespace:       kcp.Namespace,
			Labels:          internal.ControlPlaneLabelsForClusterWithHash(cluster.Name, hash.Compute(&kcp.Spec)),
			OwnerReferences: []metav1.OwnerReference{owner},
		},
		Spec: *spec,
	}

	if err := r.Client.Create(ctx, bootstrapConfig); err != nil {
		return nil, errors.Wrap(err, "Failed to create bootstrap configuration")
	}

	bootstrapRef := &corev1.ObjectReference{
		APIVersion: bootstrapv1.GroupVersion.String(),
		Kind:       "KubeadmConfig",
		Name:       bootstrapConfig.GetName(),
		Namespace:  bootstrapConfig.GetNamespace(),
		UID:        bootstrapConfig.GetUID(),
	}

	return bootstrapRef, nil
}

func (r *KubeadmControlPlaneReconciler) failureDomainForScaleUp(ctx context.Context, kcp *controlplanev1.KubeadmControlPlane, cluster *clusterv1.Cluster) (*string, error) {
	// Don't do anything if there are no failure domains defined on the cluster.
	if len(cluster.Status.FailureDomains) == 0 {
		return nil, nil
	}
	ownedMachines, err := r.managementCluster.GetMachinesForCluster(ctx, util.ObjectKey(cluster), internal.OwnedControlPlaneMachines(kcp.Name))
	if err != nil {
		return nil, err
	}
	failureDomain := internal.PickFewest(cluster.Status.FailureDomains.FilterControlPlane(), ownedMachines)
	return &failureDomain, nil
}

func (r *KubeadmControlPlaneReconciler) generateMachine(ctx context.Context, kcp *controlplanev1.KubeadmControlPlane, cluster *clusterv1.Cluster, infraRef, bootstrapRef *corev1.ObjectReference) error {
	fd, err := r.failureDomainForScaleUp(ctx, kcp, cluster)
	if err != nil {
		return err
	}

	machine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.SimpleNameGenerator.GenerateName(kcp.Name + "-"),
			Namespace: kcp.Namespace,
			Labels:    internal.ControlPlaneLabelsForClusterWithHash(cluster.Name, hash.Compute(&kcp.Spec)),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(kcp, controlplanev1.GroupVersion.WithKind("KubeadmControlPlane")),
			},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName:       cluster.Name,
			Version:           &kcp.Spec.Version,
			InfrastructureRef: *infraRef,
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: bootstrapRef,
			},
			FailureDomain: fd,
		},
	}

	if err := r.Client.Create(ctx, machine); err != nil {
		return errors.Wrap(err, "Failed to create machine")
	}

	return nil
}

// reconcileDelete handles KubeadmControlPlane deletion.
// The implementation does not take non-control plane workloads into
// consideration. This may or may not change in the future. Please see
// https://github.com/kubernetes-sigs/cluster-api/issues/2064
func (r *KubeadmControlPlaneReconciler) reconcileDelete(ctx context.Context, cluster *clusterv1.Cluster, kcp *controlplanev1.KubeadmControlPlane, logger logr.Logger) (_ ctrl.Result, reterr error) {
	allMachines, err := r.managementCluster.GetMachinesForCluster(ctx, util.ObjectKey(cluster))
	if err != nil {
		return ctrl.Result{}, err
	}
	ownedMachines, err := r.managementCluster.GetMachinesForCluster(ctx, util.ObjectKey(cluster), internal.OwnedControlPlaneMachines(kcp.Name))
	if err != nil {
		return ctrl.Result{}, err
	}

	// Verify that only control plane machines remain
	if len(allMachines) != len(ownedMachines) {
		logger.Info("Non control plane machines exist and must be removed before control plane machines are removed")
		return ctrl.Result{Requeue: true}, nil
	}

	// If no control plane machines remain, remove the finalizer
	if len(ownedMachines) == 0 {
		controllerutil.RemoveFinalizer(kcp, controlplanev1.KubeadmControlPlaneFinalizer)
		return ctrl.Result{}, nil
	}

	// Delete control plane machines in parallel
	var errs []error
	for i := range ownedMachines {
		if err := r.Client.Delete(ctx, ownedMachines[i]); err != nil && !apierrors.IsNotFound(err) {
			errs = append(errs, errors.Wrap(err, "failed to cleanup owned machines"))
		}
	}
	if errs != nil {
		return ctrl.Result{}, kerrors.NewAggregate(errs)
	}
	return ctrl.Result{RequeueAfter: DeleteRequeueAfter}, nil
}

func (r *KubeadmControlPlaneReconciler) reconcileKubeconfig(ctx context.Context, clusterName types.NamespacedName, endpoint clusterv1.APIEndpoint, kcp *controlplanev1.KubeadmControlPlane) error {
	if endpoint.IsZero() {
		return nil
	}

	_, err := secret.GetFromNamespacedName(ctx, r.Client, clusterName, secret.Kubeconfig)
	switch {
	case apierrors.IsNotFound(err):
		createErr := kubeconfig.CreateSecretWithOwner(
			ctx,
			r.Client,
			clusterName,
			endpoint.String(),
			*metav1.NewControllerRef(kcp, controlplanev1.GroupVersion.WithKind("KubeadmControlPlane")),
		)
		if createErr != nil {
			if createErr == kubeconfig.ErrDependentCertificateNotFound {
				return errors.Wrapf(&capierrors.RequeueAfterError{RequeueAfter: 30 * time.Second},
					"could not find secret %q for Cluster %q in namespace %q, requeuing",
					secret.ClusterCA, clusterName.Name, clusterName.Namespace)
			}
			return createErr
		}
	case err != nil:
		return errors.Wrapf(err, "failed to retrieve Kubeconfig Secret for Cluster %q in namespace %q", clusterName.Name, clusterName.Namespace)
	}

	return nil
}

func (r *KubeadmControlPlaneReconciler) reconcileExternalReference(ctx context.Context, cluster *clusterv1.Cluster, ref corev1.ObjectReference) error {
	if !strings.HasSuffix(ref.Kind, external.TemplateSuffix) {
		return nil
	}

	obj, err := external.Get(ctx, r.Client, &ref, cluster.Namespace)
	if err != nil {
		return err
	}

	// Note: We intentionally do not handle checking for the paused label on an external template reference

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

// ClusterToKubeadmControlPlane is a handler.ToRequestsFunc to be used to enqeue requests for reconciliation
// for KubeadmControlPlane based on updates to a Cluster.
func (r *KubeadmControlPlaneReconciler) ClusterToKubeadmControlPlane(o handler.MapObject) []ctrl.Request {
	c, ok := o.Object.(*clusterv1.Cluster)
	if !ok {
		r.Log.Error(nil, fmt.Sprintf("Expected a Cluster but got a %T", o.Object))
		return nil
	}

	controlPlaneRef := c.Spec.ControlPlaneRef
	if controlPlaneRef != nil && controlPlaneRef.Kind == "KubeadmControlPlane" {
		name := client.ObjectKey{Namespace: controlPlaneRef.Namespace, Name: controlPlaneRef.Name}
		return []ctrl.Request{{NamespacedName: name}}
	}

	return nil
}

func getMachineNode(ctx context.Context, crClient client.Client, machine *clusterv1.Machine) (*corev1.Node, error) {
	nodeRef := machine.Status.NodeRef
	if nodeRef == nil {
		return nil, nil
	}

	node := &corev1.Node{}
	err := crClient.Get(
		ctx,
		types.NamespacedName{Name: nodeRef.Name},
		node,
	)
	if err != nil {
		if apierrors.IsNotFound(errors.Cause(err)) {
			return nil, nil
		}
		return nil, err
	}

	return node, nil
}

func oldestMachine(machines []*clusterv1.Machine) (*clusterv1.Machine, error) {
	if len(machines) == 0 {
		return &clusterv1.Machine{}, errors.New("no machines given")
	}
	sort.Sort(util.MachinesByCreationTimestamp(machines))
	return machines[0], nil
}
