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

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	"sigs.k8s.io/cluster-api/test/infrastructure/container"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta2"
	dockerbackend "sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/controllers/backends/docker"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
	capicontrollerutil "sigs.k8s.io/cluster-api/util/controller"
	"sigs.k8s.io/cluster-api/util/finalizers"
	clog "sigs.k8s.io/cluster-api/util/log"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/paused"
	"sigs.k8s.io/cluster-api/util/predicates"
)

// DockerMachineReconciler reconciles a DockerMachine object.
type DockerMachineReconciler struct {
	client.Client
	ContainerRuntime  container.Runtime
	ClusterCache      clustercache.ClusterCache
	backendReconciler *dockerbackend.MachineBackendReconciler

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=dockermachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=dockermachines/status;dockermachines/finalizers,verbs=get;list;watch;patch;update
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;machinesets;machines,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets;,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch

// Reconcile handles DockerMachine events.
func (r *DockerMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx = container.RuntimeInto(ctx, r.ContainerRuntime)

	// Fetch the DockerMachine instance.
	dockerMachine := &infrav1.DockerMachine{}
	if err := r.Get(ctx, req.NamespacedName, dockerMachine); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Add finalizer first if not set to avoid the race condition between init and delete.
	if finalizerAdded, err := finalizers.EnsureFinalizer(ctx, r.Client, dockerMachine, infrav1.MachineFinalizer); err != nil || finalizerAdded {
		return ctrl.Result{}, err
	}

	// AddOwners adds the owners of DockerMachine as k/v pairs to the logger.
	// Specifically, it will add KubeadmControlPlane, MachineSet and MachineDeployment.
	ctx, log, err := clog.AddOwners(ctx, r.Client, dockerMachine)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Fetch the Machine.
	machine, err := util.GetOwnerMachine(ctx, r.Client, dockerMachine.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if machine == nil {
		// Note: If ownerRef was not set, there is nothing to delete. Remove finalizer so deletion can succeed.
		if !dockerMachine.DeletionTimestamp.IsZero() {
			if controllerutil.ContainsFinalizer(dockerMachine, infrav1.MachineFinalizer) {
				dockerMachineWithoutFinalizer := dockerMachine.DeepCopy()
				controllerutil.RemoveFinalizer(dockerMachineWithoutFinalizer, infrav1.MachineFinalizer)
				if err := r.Client.Patch(ctx, dockerMachineWithoutFinalizer, client.MergeFrom(dockerMachine)); err != nil {
					return ctrl.Result{}, errors.Wrapf(err, "failed to patch DockerMachine %s", klog.KObj(dockerMachine))
				}
			}
			return ctrl.Result{}, nil
		}

		log.Info("Waiting for Machine Controller to set OwnerRef on DockerMachine")
		return ctrl.Result{}, nil
	}

	log = log.WithValues("Machine", klog.KObj(machine))
	ctx = ctrl.LoggerInto(ctx, log)

	// Fetch the Cluster.
	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, machine.ObjectMeta)
	if err != nil {
		log.Info("DockerMachine owner Machine is missing cluster label or cluster does not exist")
		return ctrl.Result{}, err
	}
	if cluster == nil {
		log.Info(fmt.Sprintf("Please associate this machine with a cluster using the label %s: <name of cluster>", clusterv1.ClusterNameLabel))
		return ctrl.Result{}, nil
	}

	log = log.WithValues("Cluster", klog.KObj(cluster))
	ctx = ctrl.LoggerInto(ctx, log)

	// Fetch the Docker Cluster.
	dockerCluster := &infrav1.DockerCluster{}
	dockerClusterName := client.ObjectKey{
		Namespace: dockerMachine.Namespace,
		Name:      cluster.Spec.InfrastructureRef.Name,
	}
	if err := r.Get(ctx, dockerClusterName, dockerCluster); err != nil {
		log.Info("DockerCluster is not available yet")
		return ctrl.Result{}, nil
	}

	log = log.WithValues("DockerCluster", klog.KObj(dockerCluster))
	ctx = ctrl.LoggerInto(ctx, log)

	// Initialize the patch helper
	patchHelper, err := patch.NewHelper(dockerMachine, r)
	if err != nil {
		return ctrl.Result{}, err
	}

	if isPaused, requeue, err := paused.EnsurePausedCondition(ctx, r.Client, cluster, dockerMachine); err != nil || isPaused || requeue {
		return ctrl.Result{}, err
	}

	if !cluster.Spec.InfrastructureRef.IsDefined() {
		log.Info("Cluster infrastructureRef is not available yet")
		return ctrl.Result{}, nil
	}

	devCluster := dockerClusterToDevCluster(dockerCluster)
	devMachine := dockerMachineToDevMachine(dockerMachine)

	// Always attempt to Patch the DockerMachine object and status after each reconciliation.
	defer func() {
		devMachineToDockerMachine(devMachine, dockerMachine)
		if err := patchDockerMachine(ctx, patchHelper, dockerMachine); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}
	}()

	// Handle deleted machines
	if !devMachine.DeletionTimestamp.IsZero() {
		return r.backendReconciler.ReconcileDelete(ctx, cluster, devCluster, machine, devMachine)
	}

	// Handle non-deleted machines
	return r.backendReconciler.ReconcileNormal(ctx, cluster, devCluster, machine, devMachine)
}

// SetupWithManager will add watches for this controller.
func (r *DockerMachineReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	if r.Client == nil || r.ContainerRuntime == nil || r.ClusterCache == nil {
		return errors.New("Client, ContainerRuntime and ClusterCache must not be nil")
	}

	predicateLog := ctrl.LoggerFrom(ctx).WithValues("controller", "dockermachine")
	clusterToDockerMachines, err := util.ClusterToTypedObjectsMapper(mgr.GetClient(), &infrav1.DockerMachineList{}, mgr.GetScheme())
	if err != nil {
		return err
	}

	err = capicontrollerutil.NewControllerManagedBy(mgr, predicateLog).
		For(&infrav1.DockerMachine{}).
		WithOptions(options).
		WithEventFilter(predicates.ResourceHasFilterLabel(mgr.GetScheme(), predicateLog, r.WatchFilterValue)).
		Watches(
			&clusterv1.Machine{},
			handler.EnqueueRequestsFromMapFunc(util.MachineToInfrastructureMapFunc(infrav1.GroupVersion.WithKind("DockerMachine"))),
		).
		Watches(
			&infrav1.DockerCluster{},
			handler.EnqueueRequestsFromMapFunc(r.dockerClusterToDockerMachines),
		).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(clusterToDockerMachines),
			predicates.ClusterPausedTransitionsOrInfrastructureProvisioned(mgr.GetScheme(), predicateLog),
		).
		WatchesRawSource(r.ClusterCache.GetClusterSource("dockermachine", clusterToDockerMachines)).
		Complete(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	r.backendReconciler = &dockerbackend.MachineBackendReconciler{
		Client:           r.Client,
		ContainerRuntime: r.ContainerRuntime,
		ClusterCache:     r.ClusterCache,
	}

	return nil
}

// dockerClusterToDockerMachines is a handler.ToRequestsFunc to be used to enqueue
// requests for reconciliation of DockerMachines.
func (r *DockerMachineReconciler) dockerClusterToDockerMachines(ctx context.Context, o client.Object) []ctrl.Request {
	result := []ctrl.Request{}
	c, ok := o.(*infrav1.DockerCluster)
	if !ok {
		panic(fmt.Sprintf("Expected a DockerCluster but got a %T", o))
	}

	cluster, err := util.GetOwnerCluster(ctx, r.Client, c.ObjectMeta)
	switch {
	case apierrors.IsNotFound(err) || cluster == nil:
		return result
	case err != nil:
		return result
	}

	labels := map[string]string{clusterv1.ClusterNameLabel: cluster.Name}
	machineList := &clusterv1.MachineList{}
	if err := r.List(ctx, machineList, client.InNamespace(c.Namespace), client.MatchingLabels(labels)); err != nil {
		return nil
	}
	for _, m := range machineList.Items {
		if m.Spec.InfrastructureRef.Name == "" {
			continue
		}
		name := client.ObjectKey{Namespace: m.Namespace, Name: m.Name}
		result = append(result, ctrl.Request{NamespacedName: name})
	}

	return result
}

func patchDockerMachine(ctx context.Context, patchHelper *patch.Helper, dockerMachine *infrav1.DockerMachine) error {
	// Always update the readyCondition by summarizing the state of other conditions.
	// A step counter is added to represent progress during the provisioning process (instead we are hiding the step counter during the deletion process).
	v1beta1conditions.SetSummary(dockerMachine,
		v1beta1conditions.WithConditions(
			infrav1.ContainerProvisionedV1Beta1Condition,
			infrav1.BootstrapExecSucceededV1Beta1Condition,
		),
		v1beta1conditions.WithStepCounterIf(dockerMachine.DeletionTimestamp.IsZero() && dockerMachine.Spec.ProviderID == ""),
	)
	if err := conditions.SetSummaryCondition(dockerMachine, dockerMachine, infrav1.DevMachineReadyCondition,
		conditions.ForConditionTypes{
			infrav1.DevMachineDockerContainerProvisionedCondition,
			infrav1.DevMachineDockerContainerBootstrapExecSucceededCondition,
		},
		// Using a custom merge strategy to override reasons applied during merge.
		conditions.CustomMergeStrategy{
			MergeStrategy: conditions.DefaultMergeStrategy(
				// Use custom reasons.
				conditions.ComputeReasonFunc(conditions.GetDefaultComputeMergeReasonFunc(
					infrav1.DevMachineNotReadyReason,
					infrav1.DevMachineReadyUnknownReason,
					infrav1.DevMachineReadyReason,
				)),
			),
		},
	); err != nil {
		return errors.Wrapf(err, "failed to set %s condition", infrav1.DevMachineReadyCondition)
	}

	// Patch the object, ignoring conflicts on the conditions owned by this controller.
	return patchHelper.Patch(
		ctx,
		dockerMachine,
		patch.WithOwnedV1Beta1Conditions{Conditions: []clusterv1.ConditionType{
			clusterv1.ReadyV1Beta1Condition,
			infrav1.ContainerProvisionedV1Beta1Condition,
			infrav1.BootstrapExecSucceededV1Beta1Condition,
		}},
		patch.WithOwnedConditions{Conditions: []string{
			clusterv1.PausedCondition,
			infrav1.DevMachineReadyCondition,
			infrav1.DevMachineDockerContainerProvisionedCondition,
			infrav1.DevMachineDockerContainerBootstrapExecSucceededCondition,
		}},
	)
}

func dockerMachineToDevMachine(dockerMachine *infrav1.DockerMachine) *infrav1.DevMachine {
	// Carry over deprecated v1beta1 status if defined.
	var v1Beta1Status *infrav1.DevMachineDeprecatedStatus
	if dockerMachine.Status.Deprecated != nil && dockerMachine.Status.Deprecated.V1Beta1 != nil {
		v1Beta1Status = &infrav1.DevMachineDeprecatedStatus{
			V1Beta1: &infrav1.DevMachineV1Beta1DeprecatedStatus{
				Conditions: dockerMachine.Status.Deprecated.V1Beta1.Conditions,
			},
		}
	}

	return &infrav1.DevMachine{
		ObjectMeta: dockerMachine.ObjectMeta,
		Spec: infrav1.DevMachineSpec{
			ProviderID: dockerMachine.Spec.ProviderID,
			Backend: infrav1.DevMachineBackendSpec{
				Docker: &infrav1.DockerMachineBackendSpec{
					CustomImage:      dockerMachine.Spec.CustomImage,
					PreLoadImages:    dockerMachine.Spec.PreLoadImages,
					ExtraMounts:      dockerMachine.Spec.ExtraMounts,
					Bootstrapped:     dockerMachine.Spec.Bootstrapped,
					BootstrapTimeout: dockerMachine.Spec.BootstrapTimeout,
				},
			},
		},
		Status: infrav1.DevMachineStatus{
			Initialization: infrav1.DevMachineInitializationStatus{
				Provisioned: dockerMachine.Status.Initialization.Provisioned,
			},
			Addresses:     dockerMachine.Status.Addresses,
			FailureDomain: dockerMachine.Status.FailureDomain,
			Conditions:    dockerMachine.Status.Conditions,
			Deprecated:    v1Beta1Status,
			Backend: &infrav1.DevMachineBackendStatus{
				Docker: &infrav1.DockerMachineBackendStatus{
					LoadBalancerConfigured: dockerMachine.Status.LoadBalancerConfigured,
				},
			},
		},
	}
}

func devMachineToDockerMachine(devMachine *infrav1.DevMachine, dockerMachine *infrav1.DockerMachine) {
	// Carry over deprecated v1beta1 status if defined.
	var v1Beta1Status *infrav1.DockerMachineDeprecatedStatus
	if devMachine.Status.Deprecated != nil && devMachine.Status.Deprecated.V1Beta1 != nil {
		v1Beta1Status = &infrav1.DockerMachineDeprecatedStatus{
			V1Beta1: &infrav1.DockerMachineV1Beta1DeprecatedStatus{
				Conditions: devMachine.Status.Deprecated.V1Beta1.Conditions,
			},
		}
	}

	dockerMachine.ObjectMeta = devMachine.ObjectMeta
	dockerMachine.Spec.ProviderID = devMachine.Spec.ProviderID
	dockerMachine.Spec.CustomImage = devMachine.Spec.Backend.Docker.CustomImage
	dockerMachine.Spec.PreLoadImages = devMachine.Spec.Backend.Docker.PreLoadImages
	dockerMachine.Spec.ExtraMounts = devMachine.Spec.Backend.Docker.ExtraMounts
	dockerMachine.Spec.Bootstrapped = devMachine.Spec.Backend.Docker.Bootstrapped
	dockerMachine.Spec.BootstrapTimeout = devMachine.Spec.Backend.Docker.BootstrapTimeout
	dockerMachine.Status.Initialization = infrav1.DockerMachineInitializationStatus{
		Provisioned: devMachine.Status.Initialization.Provisioned,
	}
	dockerMachine.Status.Addresses = devMachine.Status.Addresses
	dockerMachine.Status.FailureDomain = devMachine.Status.FailureDomain
	dockerMachine.Status.Conditions = devMachine.Status.Conditions
	dockerMachine.Status.Deprecated = v1Beta1Status
	dockerMachine.Status.LoadBalancerConfigured = devMachine.Status.Backend.Docker.LoadBalancerConfigured
}
