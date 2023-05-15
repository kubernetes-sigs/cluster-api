/*
Copyright 2020 The Kubernetes Authors.

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

// Package controllers implements controller functionality.
package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/remote"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	utilexp "sigs.k8s.io/cluster-api/exp/util"
	"sigs.k8s.io/cluster-api/test/infrastructure/container"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1"
	infraexpv1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/exp/internal/docker"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
)

// DockerMachinePoolReconciler reconciles a DockerMachinePool object.
type DockerMachinePoolReconciler struct {
	Client           client.Client
	Scheme           *runtime.Scheme
	ContainerRuntime container.Runtime
	Tracker          *remote.ClusterCacheTracker
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=dockermachinepools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=dockermachinepools/status;dockermachinepools/finalizers,verbs=get;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinepools;machinepools/status,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets;,verbs=get;list;watch

func (r *DockerMachinePoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, rerr error) {
	log := ctrl.LoggerFrom(ctx)
	ctx = container.RuntimeInto(ctx, r.ContainerRuntime)

	// Fetch the DockerMachinePool instance.
	dockerMachinePool := &infraexpv1.DockerMachinePool{}
	if err := r.Client.Get(ctx, req.NamespacedName, dockerMachinePool); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Fetch the MachinePool.
	machinePool, err := utilexp.GetOwnerMachinePool(ctx, r.Client, dockerMachinePool.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if machinePool == nil {
		log.Info("Waiting for MachinePool Controller to set OwnerRef on DockerMachinePool")
		return ctrl.Result{}, nil
	}

	log = log.WithValues("MachinePool", machinePool.Name)
	ctx = ctrl.LoggerInto(ctx, log)

	// Fetch the Cluster.
	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, machinePool.ObjectMeta)
	if err != nil {
		log.Info("DockerMachinePool owner MachinePool is missing cluster label or cluster does not exist")
		return ctrl.Result{}, err
	}

	if cluster == nil {
		log.Info(fmt.Sprintf("Please associate this machine pool with a cluster using the label %s: <name of cluster>", clusterv1.ClusterNameLabel))
		return ctrl.Result{}, nil
	}

	log = log.WithValues("Cluster", klog.KObj(cluster))
	ctx = ctrl.LoggerInto(ctx, log)

	// Initialize the patch helper
	patchHelper, err := patch.NewHelper(dockerMachinePool, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	dockerMachinePool.Status.InfrastructureMachineSelector = metav1.LabelSelector{
		MatchLabels: map[string]string{
			clusterv1.ClusterNameLabel:            cluster.Name,
			infraexpv1.DockerMachinePoolNameLabel: dockerMachinePool.Name,
		},
	}
	dockerMachinePool.Status.InfrastructureMachineKind = "DockerMachine"
	// Patch now so that the status and selectors are available.
	if err := patchHelper.Patch(ctx, dockerMachinePool); err != nil {
		return ctrl.Result{}, err
	}

	// Always attempt to Patch the DockerMachinePool object and status after each reconciliation.
	defer func() {
		if err := patchDockerMachinePool(ctx, patchHelper, dockerMachinePool); err != nil {
			log.Error(err, "failed to patch DockerMachinePool")
			if rerr == nil {
				rerr = err
			}
		}
	}()

	// Add finalizer first if not exist to avoid the race condition between init and delete
	if !controllerutil.ContainsFinalizer(dockerMachinePool, infraexpv1.MachinePoolFinalizer) {
		controllerutil.AddFinalizer(dockerMachinePool, infraexpv1.MachinePoolFinalizer)
		return ctrl.Result{}, nil
	}

	// Handle deleted machines
	if !dockerMachinePool.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, cluster, machinePool, dockerMachinePool)
	}

	// Handle non-deleted machines
	res, err = r.reconcileNormal(ctx, cluster, machinePool, dockerMachinePool)
	// Requeue if the reconcile failed because the ClusterCacheTracker was locked for
	// the current cluster because of concurrent access.
	if errors.Is(err, remote.ErrClusterLocked) {
		log.V(5).Info("Requeuing because another worker has the lock on the ClusterCacheTracker")
		return ctrl.Result{Requeue: true}, nil
	}
	return res, err
}

// SetupWithManager will add watches for this controller.
func (r *DockerMachinePoolReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	clusterToDockerMachinePools, err := util.ClusterToObjectsMapper(mgr.GetClient(), &infraexpv1.DockerMachinePoolList{}, mgr.GetScheme())
	if err != nil {
		return err
	}

	err = ctrl.NewControllerManagedBy(mgr).
		For(&infraexpv1.DockerMachinePool{}).
		WithOptions(options).
		WithEventFilter(predicates.ResourceNotPaused(ctrl.LoggerFrom(ctx))).
		Watches(
			&expv1.MachinePool{},
			handler.EnqueueRequestsFromMapFunc(utilexp.MachinePoolToInfrastructureMapFunc(
				infraexpv1.GroupVersion.WithKind("DockerMachinePool"), ctrl.LoggerFrom(ctx))),
		).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(clusterToDockerMachinePools),
			builder.WithPredicates(
				predicates.ClusterUnpausedAndInfrastructureReady(ctrl.LoggerFrom(ctx)),
			),
		).Complete(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}
	return nil
}

func (r *DockerMachinePoolReconciler) reconcileDelete(ctx context.Context, cluster *clusterv1.Cluster, machinePool *expv1.MachinePool, dockerMachinePool *infraexpv1.DockerMachinePool) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	dockerMachineList, err := getDockerMachines(ctx, r.Client, *dockerMachinePool)
	if err != nil {
		return ctrl.Result{}, err
	}

	nodePoolInstances := make([]docker.NodePoolInstance, len(dockerMachineList.Items))
	for i, dockerMachine := range dockerMachineList.Items {
		machine, err := util.GetOwnerMachine(ctx, r.Client, dockerMachine.ObjectMeta)
		if err != nil {
			log.V(2).Error(err, "failed to get owner machine, skipping")
			continue
		}
		if machine == nil {
			log.V(2).Info("owner machine not found, skipping", "docker-machine", dockerMachine.Name)
			continue
		}

		hasDeleteAnnotation := false
		if machine.Annotations != nil {
			_, hasDeleteAnnotation = machine.Annotations[clusterv1.DeleteMachineAnnotation]
		}

		nodePoolInstances[i] = docker.NodePoolInstance{
			InstanceName:     dockerMachine.Spec.InstanceName,
			Bootstrapped:     dockerMachine.Spec.Bootstrapped,
			ProviderID:       dockerMachine.Spec.ProviderID,
			PrioritizeDelete: hasDeleteAnnotation,
			Addresses:        dockerMachine.Status.Addresses,
		}
	}

	pool, err := docker.NewNodePool(ctx, r.Client, cluster, machinePool, dockerMachinePool, nodePoolInstances)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to build new node pool")
	}

	if err := pool.Delete(ctx); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to delete all machines in the node pool")
	}

	controllerutil.RemoveFinalizer(dockerMachinePool, infraexpv1.MachinePoolFinalizer)
	return ctrl.Result{}, nil
}

func (r *DockerMachinePoolReconciler) reconcileNormal(ctx context.Context, cluster *clusterv1.Cluster, machinePool *expv1.MachinePool, dockerMachinePool *infraexpv1.DockerMachinePool) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// Make sure bootstrap data is available and populated.
	if machinePool.Spec.Template.Spec.Bootstrap.DataSecretName == nil {
		log.Info("Waiting for the Bootstrap provider controller to set bootstrap data")
		return ctrl.Result{}, nil
	}

	if machinePool.Spec.Replicas == nil {
		machinePool.Spec.Replicas = pointer.Int32(1)
	}

	dockerMachineList, err := getDockerMachines(ctx, r.Client, *dockerMachinePool)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Since nodepools don't persist the instances list, we need to construct it from the list of DockerMachines.
	nodePoolInstances := make([]docker.NodePoolInstance, len(dockerMachineList.Items))
	for i, dockerMachine := range dockerMachineList.Items {
		machine, err := util.GetOwnerMachine(ctx, r.Client, dockerMachine.ObjectMeta)
		if err != nil {
			log.V(2).Error(err, "Failed to get owner machine, skipping")
		}
		var hasDeleteAnnotation bool
		if machine != nil {
			_, hasDeleteAnnotation = machine.Annotations[clusterv1.DeleteMachineAnnotation]
		}

		nodePoolInstances[i] = docker.NodePoolInstance{
			InstanceName:     dockerMachine.Spec.InstanceName,
			Bootstrapped:     dockerMachine.Spec.Bootstrapped,
			ProviderID:       dockerMachine.Spec.ProviderID,
			PrioritizeDelete: hasDeleteAnnotation,
		}
	}

	pool, err := docker.NewNodePool(ctx, r.Client, cluster, machinePool, dockerMachinePool, nodePoolInstances)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to build new node pool")
	}

	// Reconcile machines and updates Status.Instances
	remoteClient, err := r.Tracker.GetClient(ctx, client.ObjectKeyFromObject(cluster))
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to generate workload cluster client")
	}

	res, err := pool.ReconcileMachines(ctx, remoteClient)
	if err != nil {
		return res, err
	}

	nodePoolInstancesResult := pool.GetNodePoolInstances()

	// Derive info from Status.Instances
	dockerMachinePool.Spec.ProviderIDList = []string{}
	for _, instance := range nodePoolInstancesResult {
		if instance.ProviderID != nil {
			dockerMachinePool.Spec.ProviderIDList = append(dockerMachinePool.Spec.ProviderIDList, *instance.ProviderID)
		}
	}

	// Delete all DockerMachines that are not in the list of instances returned by the node pool.
	if err := r.DeleteDanglingDockerMachines(ctx, dockerMachinePool, nodePoolInstancesResult); err != nil {
		conditions.MarkFalse(dockerMachinePool, clusterv1.ReadyCondition, "FailedToDeleteOrphanedMachines", clusterv1.ConditionSeverityWarning, err.Error())
		return ctrl.Result{}, errors.Wrap(err, "failed to delete orphaned machines")
	}

	// Create a DockerMachine for each instance returned by the node pool if it doesn't exist.
	if err := r.CreateDockerMachinesIfNotExists(ctx, machinePool, dockerMachinePool, nodePoolInstancesResult); err != nil {
		conditions.MarkFalse(dockerMachinePool, clusterv1.ReadyCondition, "FailedToCreateNewMachines", clusterv1.ConditionSeverityWarning, err.Error())
		return ctrl.Result{}, errors.Wrap(err, "failed to create missing machines")
	}

	dockerMachinePool.Status.Replicas = int32(len(dockerMachineList.Items))

	if dockerMachinePool.Spec.ProviderID == "" {
		// This is a fake provider ID which does not tie back to any docker infrastructure. In cloud providers,
		// this ID would tie back to the resource which manages the machine pool implementation. For example,
		// Azure uses a VirtualMachineScaleSet to manage a set of like machines.
		dockerMachinePool.Spec.ProviderID = getDockerMachinePoolProviderID(cluster.Name, dockerMachinePool.Name)
	}

	if len(dockerMachinePool.Spec.ProviderIDList) == int(*machinePool.Spec.Replicas) {
		dockerMachinePool.Status.Ready = true
		conditions.MarkTrue(dockerMachinePool, expv1.ReplicasReadyCondition)

		return ctrl.Result{}, nil
	}

	dockerMachinePool.Status.Ready = false
	conditions.MarkFalse(dockerMachinePool, expv1.ReplicasReadyCondition, expv1.WaitingForReplicasReadyReason, clusterv1.ConditionSeverityInfo, "")

	// if some machine is still provisioning, force reconcile in few seconds to check again infrastructure.
	if res.IsZero() {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	return res, nil
}

func getDockerMachines(ctx context.Context, c client.Client, dockerMachinePool infraexpv1.DockerMachinePool) (*infrav1.DockerMachineList, error) {
	dockerMachineList := &infrav1.DockerMachineList{}
	labels := dockerMachinePool.Status.InfrastructureMachineSelector.MatchLabels
	if err := c.List(ctx, dockerMachineList, client.InNamespace(dockerMachinePool.Namespace), client.MatchingLabels(labels)); err != nil {
		return nil, err
	}

	return dockerMachineList, nil
}

// DeleteDanglingDockerMachines deletes any DockerMachines owned by the DockerMachinePool that reference an invalid providerID, i.e. not in the latest copy of the node pool instances.
func (r *DockerMachinePoolReconciler) DeleteDanglingDockerMachines(ctx context.Context, dockerMachinePool *infraexpv1.DockerMachinePool, instances []docker.NodePoolInstance) error {
	log := ctrl.LoggerFrom(ctx)
	log.V(2).Info("Deleting orphaned machines", "dockerMachinePool", dockerMachinePool.Name, "namespace", dockerMachinePool.Namespace, "instances", instances)
	dockerMachineList, err := getDockerMachines(ctx, r.Client, *dockerMachinePool)
	if err != nil {
		return err
	}

	log.V(2).Info("DockerMachineList kind is", "kind", dockerMachineList.GetObjectKind())

	instanceNameSet := map[string]struct{}{}
	for _, instance := range instances {
		instanceNameSet[instance.InstanceName] = struct{}{}
	}

	for i := range dockerMachineList.Items {
		dockerMachine := &dockerMachineList.Items[i]
		if _, ok := instanceNameSet[dockerMachine.Spec.InstanceName]; !ok {
			log.V(2).Info("Deleting orphaned DockerMachine", "dockerMachine", dockerMachine.Name, "namespace", dockerMachine.Namespace)
			if err := r.Client.Delete(ctx, dockerMachine); err != nil {
				return err
			}
		} else {
			log.V(2).Info("Keeping DockerMachine, nothing to do", "dockerMachine", dockerMachine.Name, "namespace", dockerMachine.Namespace)
		}
	}

	return nil
}

// CreateDockerMachinesIfNotExists creates a DockerMachine for each instance returned by the node pool if it doesn't exist.
func (r *DockerMachinePoolReconciler) CreateDockerMachinesIfNotExists(ctx context.Context, machinePool *expv1.MachinePool, dockerMachinePool *infraexpv1.DockerMachinePool, instances []docker.NodePoolInstance) error {
	log := ctrl.LoggerFrom(ctx)

	log.V(2).Info("Creating missing machines", "dockerMachinePool", dockerMachinePool.Name, "namespace", dockerMachinePool.Namespace, "instances", instances)

	dockerMachineList, err := getDockerMachines(ctx, r.Client, *dockerMachinePool)
	if err != nil {
		return err
	}

	instanceNameToDockerMachine := make(map[string]infrav1.DockerMachine)
	for _, dockerMachine := range dockerMachineList.Items {
		instanceNameToDockerMachine[dockerMachine.Spec.InstanceName] = dockerMachine
	}

	for _, instance := range instances {
		if _, exists := instanceNameToDockerMachine[instance.InstanceName]; exists {
			continue
		}

		labels := dockerMachinePool.Status.InfrastructureMachineSelector.MatchLabels
		labels[clusterv1.MachinePoolNameLabel] = machinePool.Name
		dockerMachine := &infrav1.DockerMachine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    dockerMachinePool.Namespace,
				GenerateName: fmt.Sprintf("%s-", dockerMachinePool.Name),
				Labels:       labels,
				Annotations:  make(map[string]string),
				// Note: This DockerMachine will be owned by the DockerMachinePool until the MachinePool controller creates its parent Machine.
			},
			Spec: infrav1.DockerMachineSpec{
				InstanceName: instance.InstanceName,
			},
		}

		log.V(2).Info("Instance name for dockerMachine is", "instanceName", instance.InstanceName, "dockerMachine", dockerMachine.Name)

		if err := r.Client.Create(ctx, dockerMachine); err != nil {
			return errors.Wrap(err, "failed to create dockerMachine")
		}
	}

	return nil
}

func getDockerMachinePoolProviderID(clusterName, dockerMachinePoolName string) string {
	return fmt.Sprintf("docker:////%s-dmp-%s", clusterName, dockerMachinePoolName)
}

func patchDockerMachinePool(ctx context.Context, patchHelper *patch.Helper, dockerMachinePool *infraexpv1.DockerMachinePool) error {
	conditions.SetSummary(dockerMachinePool,
		conditions.WithConditions(
			expv1.ReplicasReadyCondition,
		),
	)

	// Patch the object, ignoring conflicts on the conditions owned by this controller.
	return patchHelper.Patch(
		ctx,
		dockerMachinePool,
		patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
			clusterv1.ReadyCondition,
			expv1.ReplicasReadyCondition,
		}},
	)
}
