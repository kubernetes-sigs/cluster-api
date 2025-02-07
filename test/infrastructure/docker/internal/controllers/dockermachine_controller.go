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
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	"sigs.k8s.io/cluster-api/test/infrastructure/container"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1"
	dockerbackend "sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/controllers/backends/docker"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
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
	if err := r.Client.Get(ctx, req.NamespacedName, dockerMachine); err != nil {
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
	if err := r.Client.Get(ctx, dockerClusterName, dockerCluster); err != nil {
		log.Info("DockerCluster is not available yet")
		return ctrl.Result{}, nil
	}

	log = log.WithValues("DockerCluster", klog.KObj(dockerCluster))
	ctx = ctrl.LoggerInto(ctx, log)

	if isPaused, conditionChanged, err := paused.EnsurePausedCondition(ctx, r.Client, cluster, dockerMachine); err != nil || isPaused || conditionChanged {
		return ctrl.Result{}, err
	}

	if cluster.Spec.InfrastructureRef == nil {
		log.Info("Cluster infrastructureRef is not available yet")
		return ctrl.Result{}, nil
	}

	// Initialize the patch helper
	patchHelper, err := patch.NewHelper(dockerMachine, r)
	if err != nil {
		return ctrl.Result{}, err
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
	if !devMachine.ObjectMeta.DeletionTimestamp.IsZero() {
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

	err = ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.DockerMachine{}).
		WithOptions(options).
		WithEventFilter(predicates.ResourceHasFilterLabel(mgr.GetScheme(), predicateLog, r.WatchFilterValue)).
		Watches(
			&clusterv1.Machine{},
			handler.EnqueueRequestsFromMapFunc(util.MachineToInfrastructureMapFunc(infrav1.GroupVersion.WithKind("DockerMachine"))),
			builder.WithPredicates(predicates.ResourceIsChanged(mgr.GetScheme(), predicateLog)),
		).
		Watches(
			&infrav1.DockerCluster{},
			handler.EnqueueRequestsFromMapFunc(r.dockerClusterToDockerMachines),
			builder.WithPredicates(predicates.ResourceIsChanged(mgr.GetScheme(), predicateLog)),
		).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(clusterToDockerMachines),
			builder.WithPredicates(predicates.All(mgr.GetScheme(), predicateLog,
				predicates.ResourceIsChanged(mgr.GetScheme(), predicateLog),
				predicates.ClusterPausedTransitionsOrInfrastructureReady(mgr.GetScheme(), predicateLog),
			)),
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
		NewPatchHelperFunc: func(obj client.Object, crClient client.Client) (*patch.Helper, error) {
			devMachine, ok := obj.(*infrav1.DevMachine)
			if !ok {
				panic(fmt.Sprintf("Expected obj to be *infrav1.DevMachine, got %T", obj))
			}
			dockerMachine := &infrav1.DockerMachine{}
			devMachineToDockerMachine(devMachine, dockerMachine)
			return patch.NewHelper(dockerMachine, crClient)
		},
		PatchDevMachineFunc: func(ctx context.Context, patchHelper *patch.Helper, devMachine *infrav1.DevMachine, _ bool) error {
			dockerMachine := &infrav1.DockerMachine{}
			devMachineToDockerMachine(devMachine, dockerMachine)
			return patchDockerMachine(ctx, patchHelper, dockerMachine)
		},
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
	if err := r.Client.List(ctx, machineList, client.InNamespace(c.Namespace), client.MatchingLabels(labels)); err != nil {
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
	conditions.SetSummary(dockerMachine,
		conditions.WithConditions(
			infrav1.ContainerProvisionedCondition,
			infrav1.BootstrapExecSucceededCondition,
		),
		conditions.WithStepCounterIf(dockerMachine.ObjectMeta.DeletionTimestamp.IsZero() && dockerMachine.Spec.ProviderID == nil),
	)

	// Patch the object, ignoring conflicts on the conditions owned by this controller.
	return patchHelper.Patch(
		ctx,
		dockerMachine,
		patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
			clusterv1.ReadyCondition,
			infrav1.ContainerProvisionedCondition,
			infrav1.BootstrapExecSucceededCondition,
		}},
	)
}

func dockerMachineToDevMachine(dockerMachine *infrav1.DockerMachine) *infrav1.DevMachine {
	var v1Beta2Status *infrav1.DevMachineV1Beta2Status
	if dockerMachine.Status.V1Beta2 != nil {
		v1Beta2Status = &infrav1.DevMachineV1Beta2Status{
			Conditions: dockerMachine.Status.V1Beta2.Conditions,
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
			Ready:      dockerMachine.Status.Ready,
			Addresses:  dockerMachine.Status.Addresses,
			Conditions: dockerMachine.Status.Conditions,
			V1Beta2:    v1Beta2Status,
			Backend: &infrav1.DevMachineBackendStatus{
				Docker: &infrav1.DockerMachineBackendStatus{
					LoadBalancerConfigured: dockerMachine.Status.LoadBalancerConfigured,
				},
			},
		},
	}
}

func devMachineToDockerMachine(devMachine *infrav1.DevMachine, dockerMachine *infrav1.DockerMachine) {
	var v1Beta2Status *infrav1.DockerMachineV1Beta2Status
	if devMachine.Status.V1Beta2 != nil {
		v1Beta2Status = &infrav1.DockerMachineV1Beta2Status{
			Conditions: devMachine.Status.V1Beta2.Conditions,
		}
	}

	dockerMachine.ObjectMeta = devMachine.ObjectMeta
	dockerMachine.Spec.ProviderID = devMachine.Spec.ProviderID
	dockerMachine.Spec.CustomImage = devMachine.Spec.Backend.Docker.CustomImage
	dockerMachine.Spec.PreLoadImages = devMachine.Spec.Backend.Docker.PreLoadImages
	dockerMachine.Spec.ExtraMounts = devMachine.Spec.Backend.Docker.ExtraMounts
	dockerMachine.Spec.Bootstrapped = devMachine.Spec.Backend.Docker.Bootstrapped
	dockerMachine.Spec.BootstrapTimeout = devMachine.Spec.Backend.Docker.BootstrapTimeout
	dockerMachine.Status.Ready = devMachine.Status.Ready
	dockerMachine.Status.Addresses = devMachine.Status.Addresses
	dockerMachine.Status.Conditions = devMachine.Status.Conditions
	dockerMachine.Status.V1Beta2 = v1Beta2Status
	dockerMachine.Status.LoadBalancerConfigured = devMachine.Status.Backend.Docker.LoadBalancerConfigured
}
