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
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/blang/semver"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/kind/pkg/cluster/constants"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/controllers/remote"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	utilexp "sigs.k8s.io/cluster-api/exp/util"
	"sigs.k8s.io/cluster-api/test/infrastructure/container"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1"
	infraexpv1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/docker"
	"sigs.k8s.io/cluster-api/test/infrastructure/kind"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/labels/format"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
)

const (
	dockerMachinePoolLabel = "docker.cluster.x-k8s.io/machine-pool"
)

// DockerMachinePoolReconciler reconciles a DockerMachinePool object.
type DockerMachinePoolReconciler struct {
	Client           client.Client
	Scheme           *runtime.Scheme
	ContainerRuntime container.Runtime
	Tracker          *remote.ClusterCacheTracker

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string

	controller      controller.Controller
	recorder        record.EventRecorder
	externalTracker external.ObjectTracker
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=dockermachinepools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=dockermachinepools/status;dockermachinepools/finalizers,verbs=get;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinepools;machinepools/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines,verbs=get;list;watch;create;update;patch;delete
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

	// Handle deleted machines
	if !dockerMachinePool.ObjectMeta.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, r.reconcileDelete(ctx, cluster, machinePool, dockerMachinePool)
	}

	// Add finalizer first if not set to avoid the race condition between init and delete.
	// Note: Finalizers in general can only be added when the deletionTimestamp is not set.
	if !controllerutil.ContainsFinalizer(dockerMachinePool, infraexpv1.MachinePoolFinalizer) {
		controllerutil.AddFinalizer(dockerMachinePool, infraexpv1.MachinePoolFinalizer)
		return ctrl.Result{}, nil
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
	clusterToDockerMachinePools, err := util.ClusterToTypedObjectsMapper(mgr.GetClient(), &infraexpv1.DockerMachinePoolList{}, mgr.GetScheme())
	if err != nil {
		return err
	}

	c, err := ctrl.NewControllerManagedBy(mgr).
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
		).Build(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	r.controller = c
	r.recorder = mgr.GetEventRecorderFor("dockermachinepool-controller")
	r.externalTracker = external.ObjectTracker{
		Controller: c,
		Cache:      mgr.GetCache(),
	}

	return nil
}

func (r *DockerMachinePoolReconciler) reconcileDelete(ctx context.Context, cluster *clusterv1.Cluster, machinePool *expv1.MachinePool, dockerMachinePool *infraexpv1.DockerMachinePool) error {
	_ = ctrl.LoggerFrom(ctx)

	labelFilters := map[string]string{dockerMachinePoolLabel: dockerMachinePool.Name}
	machines, err := docker.ListMachinesByCluster(ctx, cluster, labelFilters)
	if err != nil {
		return errors.Wrapf(err, "failed to list all machines in the cluster")
	}

	for _, machine := range machines {
		key := client.ObjectKey{
			Namespace: dockerMachinePool.Namespace,
			Name:      machine.Name(),
		}
		dockerMachine := &infrav1.DockerMachine{}
		if err := r.Client.Get(ctx, key, dockerMachine); err != nil {
			if err := machine.Delete(ctx); err != nil {
				return errors.Wrapf(err, "failed to delete machine %s", machine.Name())
			}
		}

		if err := r.deleteMachinePoolMachine(ctx, *dockerMachine, *machinePool); err != nil {
			return errors.Wrapf(err, "failed to delete machine %s", dockerMachine.Name)
		}
	}

	controllerutil.RemoveFinalizer(dockerMachinePool, infraexpv1.MachinePoolFinalizer)
	return nil
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

	if err := r.CreateNewReplicas(ctx, cluster, machinePool, dockerMachinePool); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.DeleteExtraDockerMachines(ctx, cluster, machinePool, dockerMachinePool); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.CreateNewDockerMachines(ctx, cluster, machinePool, dockerMachinePool); err != nil {
		return ctrl.Result{}, err
	}

	dockerMachineList, err := getDockerMachines(ctx, r.Client, *cluster, *machinePool, *dockerMachinePool)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Derive info from DockerMachines
	dockerMachinePool.Spec.ProviderIDList = []string{}
	for _, dockerMachine := range dockerMachineList.Items {
		if dockerMachine.Spec.ProviderID != nil {
			dockerMachinePool.Spec.ProviderIDList = append(dockerMachinePool.Spec.ProviderIDList, *dockerMachine.Spec.ProviderID)
		}
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

	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	// return res, nil
}

func getDockerMachines(ctx context.Context, c client.Client, cluster clusterv1.Cluster, machinePool expv1.MachinePool, dockerMachinePool infraexpv1.DockerMachinePool) (*infrav1.DockerMachineList, error) {
	dockerMachineList := &infrav1.DockerMachineList{}
	labels := map[string]string{
		clusterv1.ClusterNameLabel:     cluster.Name,
		clusterv1.MachinePoolNameLabel: machinePool.Name,
	}
	if err := c.List(ctx, dockerMachineList, client.InNamespace(dockerMachinePool.Namespace), client.MatchingLabels(labels)); err != nil {
		return nil, err
	}

	return dockerMachineList, nil
}

// DeleteExtraDockerMachines deletes any DockerMachines owned by the DockerMachinePool that reference an invalid providerID, i.e. not in the latest copy of the node pool instances.
func (r *DockerMachinePoolReconciler) DeleteExtraDockerMachines(ctx context.Context, cluster *clusterv1.Cluster, machinePool *expv1.MachinePool, dockerMachinePool *infraexpv1.DockerMachinePool) error {
	log := ctrl.LoggerFrom(ctx)
	// log.V(2).Info("Deleting orphaned machines", "dockerMachinePool", dockerMachinePool.Name, "namespace", dockerMachinePool.Namespace, "nodePoolMachineStatuses", nodePoolMachineStatuses)
	dockerMachineList, err := getDockerMachines(ctx, r.Client, *cluster, *machinePool, *dockerMachinePool)
	if err != nil {
		return err
	}

	log.V(2).Info("DockerMachineList kind is", "kind", dockerMachineList.GetObjectKind())

	dockerMachinesToDelete, err := getDockerMachinesToDelete(ctx, dockerMachineList.Items, cluster, machinePool, dockerMachinePool)
	if err != nil {
		return err
	}

	for _, dockerMachine := range dockerMachinesToDelete {
		log.V(2).Info("Deleting DockerMachine", "dockerMachine", dockerMachine.Name, "namespace", dockerMachine.Namespace)
		if err := r.deleteMachinePoolMachine(ctx, dockerMachine, *machinePool); err != nil {
			return err
		}
	}

	return nil
}

func getDockerMachinesToDelete(ctx context.Context, dockerMachines []infrav1.DockerMachine, cluster *clusterv1.Cluster, machinePool *expv1.MachinePool, dockerMachinePool *infraexpv1.DockerMachinePool) ([]infrav1.DockerMachine, error) {
	log := ctrl.LoggerFrom(ctx)

	// TODO: sort these by delete priority
	dockerMachinesToDelete := []infrav1.DockerMachine{}
	numToDelete := len(dockerMachines) - int(*machinePool.Spec.Replicas)
	// TODO: sort list of dockerMachines
	labelFilters := map[string]string{dockerMachinePoolLabel: dockerMachinePool.Name}

	// Sort priority delete to the front of the list
	sort.Slice(dockerMachines, func(i, j int) bool {
		_, iHasAnnotation := dockerMachines[i].Annotations[clusterv1.DeleteMachineAnnotation]
		_, jHasAnnotation := dockerMachines[j].Annotations[clusterv1.DeleteMachineAnnotation]

		if iHasAnnotation && jHasAnnotation {
			return dockerMachines[i].Name < dockerMachines[j].Name
		}

		return iHasAnnotation
	})

	for _, dockerMachine := range dockerMachines {
		// externalMachine, err := docker.NewMachine(ctx, cluster, dockerMachine.Name, labelFilters)
		if numToDelete > 0 {
			dockerMachinesToDelete = append(dockerMachinesToDelete, dockerMachine)
			numToDelete--
		} else {
			externalMachine, err := docker.NewMachine(ctx, cluster, dockerMachine.Name, labelFilters)
			if err != nil {
				// TODO: should we delete anyways
				return nil, err
			}
			if !isMachineMatchingInfrastructureSpec(ctx, externalMachine, machinePool, dockerMachinePool) {
				dockerMachinesToDelete = append(dockerMachinesToDelete, dockerMachine)
			}
		}

		log.V(2).Info("Keeping DockerMachine, nothing to do", "dockerMachine", dockerMachine.Name, "namespace", dockerMachine.Namespace)
	}

	return dockerMachinesToDelete, nil
}

func (r *DockerMachinePoolReconciler) deleteMachinePoolMachine(ctx context.Context, dockerMachine infrav1.DockerMachine, machinePool expv1.MachinePool) error {
	log := ctrl.LoggerFrom(ctx)
	machine, err := util.GetOwnerMachine(ctx, r.Client, dockerMachine.ObjectMeta)
	// Not found doesn't return an error, so we need to check for nil.
	if err != nil {
		return errors.Wrapf(err, "error getting owner Machine for DockerMachine %s/%s", dockerMachine.Namespace, dockerMachine.Name)
	}
	if machine == nil {
		// If MachinePool is deleted, the DockerMachine and owner Machine doesn't already exist, then it will never come online.
		if mpDeleted := isMachinePoolDeleted(ctx, r.Client, &machinePool); mpDeleted {
			log.Info("DockerMachine is orphaned and MachinePool is deleted, deleting DockerMachine", "dockerMachine", dockerMachine.Name, "namespace", dockerMachine.Namespace)
			if err := r.Client.Delete(ctx, &dockerMachine); err != nil {
				return errors.Wrapf(err, "failed to delete orphaned DockerMachine %s/%s", dockerMachine.Namespace, dockerMachine.Name)
			}
		} else { // If the MachinePool still exists, then the Machine will be created, so we need to wait for that to happen.
			return errors.Errorf("DockerMachine %s/%s has no parent Machine, will reattempt deletion once parent Machine is present", dockerMachine.Namespace, dockerMachine.Name)
		}
	} else {
		if err := r.Client.Delete(ctx, machine); err != nil {
			return errors.Wrapf(err, "failed to delete Machine %s/%s", machine.Namespace, machine.Name)
		}
	}

	return nil
}

// CreateNewReplicas creates a DockerMachine for each instance returned by the node pool if it doesn't exist.
func (r *DockerMachinePoolReconciler) CreateNewReplicas(ctx context.Context, cluster *clusterv1.Cluster, machinePool *expv1.MachinePool, dockerMachinePool *infraexpv1.DockerMachinePool) error {
	log := ctrl.LoggerFrom(ctx)

	labelFilters := map[string]string{dockerMachinePoolLabel: dockerMachinePool.Name}

	machines, err := docker.ListMachinesByCluster(ctx, cluster, labelFilters)
	if err != nil {
		return errors.Wrapf(err, "failed to list all machines in the cluster")
	}

	matchingMachineCount := len(machinesMatchingInfrastructureSpec(ctx, machines, machinePool, dockerMachinePool))
	log.Info("MatchingMachineCount", "count", matchingMachineCount)
	numToCreate := int(*machinePool.Spec.Replicas) - matchingMachineCount
	for i := 0; i < numToCreate; i++ {
		createdMachine, err := createReplica(ctx, cluster, machinePool, dockerMachinePool)
		if err != nil {
			return errors.Wrap(err, "failed to create a new docker machine")
		}

		if err := r.createDockerMachine(ctx, createdMachine.Name(), cluster, machinePool, dockerMachinePool); err != nil {
			return errors.Wrap(err, "failed to create a new docker machine")
		}
	}

	return nil
}

// CreateNewDockerMachines creates a DockerMachine for each instance returned by the node pool if it doesn't exist.
func (r *DockerMachinePoolReconciler) CreateNewDockerMachines(ctx context.Context, cluster *clusterv1.Cluster, machinePool *expv1.MachinePool, dockerMachinePool *infraexpv1.DockerMachinePool) error {
	_ = ctrl.LoggerFrom(ctx)

	labelFilters := map[string]string{dockerMachinePoolLabel: dockerMachinePool.Name}

	machines, err := docker.ListMachinesByCluster(ctx, cluster, labelFilters)
	if err != nil {
		return errors.Wrapf(err, "failed to list all machines in the cluster")
	}

	matchingMachines := machinesMatchingInfrastructureSpec(ctx, machines, machinePool, dockerMachinePool)
	for _, machine := range matchingMachines {
		if err := r.Client.Get(ctx, client.ObjectKey{Namespace: machinePool.Namespace, Name: machine.Name()}, &infrav1.DockerMachine{}); err != nil {
			if apierrors.IsNotFound(err) {
				if err := r.createDockerMachine(ctx, machine.Name(), cluster, machinePool, dockerMachinePool); err != nil {
					return errors.Wrap(err, "failed to create a new docker machine")
				}
			} else {
				return errors.Wrap(err, "failed to get docker machine")
			}
		}
	}

	return nil
}

func createReplica(ctx context.Context, cluster *clusterv1.Cluster, machinePool *expv1.MachinePool, dockerMachinePool *infraexpv1.DockerMachinePool) (*docker.Machine, error) {
	name := fmt.Sprintf("worker-%s", util.RandomString(6))
	labelFilters := map[string]string{dockerMachinePoolLabel: dockerMachinePool.Name}
	externalMachine, err := docker.NewMachine(ctx, cluster, name, labelFilters)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create helper for managing the externalMachine named %s", name)
	}

	// NOTE: FailureDomains don't mean much in CAPD since it's all local, but we are setting a label on
	// each container, so we can check placement.
	labels := map[string]string{}
	for k, v := range labelFilters {
		labels[k] = v
	}

	if len(machinePool.Spec.FailureDomains) > 0 {
		// For MachinePools placement is expected to be managed by the underlying infrastructure primitive, but
		// given that there is no such an thing in CAPD, we are picking a random failure domain.
		randomIndex := rand.Intn(len(machinePool.Spec.FailureDomains)) //nolint:gosec
		for k, v := range docker.FailureDomainLabel(&machinePool.Spec.FailureDomains[randomIndex]) {
			labels[k] = v
		}
	}

	if err := externalMachine.Create(ctx, dockerMachinePool.Spec.Template.CustomImage, constants.WorkerNodeRoleValue, machinePool.Spec.Template.Spec.Version, labels, dockerMachinePool.Spec.Template.ExtraMounts); err != nil {
		return nil, errors.Wrapf(err, "failed to create docker machine with name %s", name)
	}
	return externalMachine, nil
}

func (r *DockerMachinePoolReconciler) createDockerMachine(ctx context.Context, name string, cluster *clusterv1.Cluster, machinePool *expv1.MachinePool, dockerMachinePool *infraexpv1.DockerMachinePool) error {
	labels := map[string]string{
		clusterv1.ClusterNameLabel:     cluster.Name,
		clusterv1.MachinePoolNameLabel: format.MustFormatValue(machinePool.Name),
	}
	dockerMachine := &infrav1.DockerMachine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   dockerMachinePool.Namespace,
			Name:        name,
			Labels:      labels,
			Annotations: make(map[string]string),
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: dockerMachinePool.APIVersion,
					Kind:       dockerMachinePool.Kind,
					Name:       dockerMachinePool.Name,
					UID:        dockerMachinePool.UID,
				},
				// Note: Since the MachinePool controller has not created its parent Machine yet, we want to set the DockerMachinePool as the owner so it's not orphaned.
			},
		},
		Spec: infrav1.DockerMachineSpec{
			CustomImage:   dockerMachinePool.Spec.Template.CustomImage,
			PreLoadImages: dockerMachinePool.Spec.Template.PreLoadImages,
		},
	}

	// log.V(2).Info("Instance name for dockerMachine is", "instanceName", nodePoolMachineStatus.Name, "dockerMachine", dockerMachine.GetName())

	if err := r.Client.Create(ctx, dockerMachine); err != nil {
		return errors.Wrap(err, "failed to create dockerMachine")
	}

	return nil
}

func isMachineMatchingInfrastructureSpec(ctx context.Context, machine *docker.Machine, machinePool *expv1.MachinePool, dockerMachinePool *infraexpv1.DockerMachinePool) bool {
	// NOTE: With the current implementation we are checking if the machine is using a kindest/node image for the expected version,
	// but not checking if the machine has the expected extra.mounts or pre.loaded images.

	semVer, err := semver.Parse(strings.TrimPrefix(*machinePool.Spec.Template.Spec.Version, "v"))
	if err != nil {
		// TODO: consider if to return an error
		panic(errors.Wrap(err, "failed to parse DockerMachine version").Error())
	}

	kindMapping := kind.GetMapping(semVer, dockerMachinePool.Spec.Template.CustomImage)

	return machine.ContainerImage() == kindMapping.Image
}

func machinesMatchingInfrastructureSpec(ctx context.Context, machines []*docker.Machine, machinePool *expv1.MachinePool, dockerMachinePool *infraexpv1.DockerMachinePool) []*docker.Machine {
	var matchingMachines []*docker.Machine
	for _, machine := range machines {
		if isMachineMatchingInfrastructureSpec(ctx, machine, machinePool, dockerMachinePool) {
			matchingMachines = append(matchingMachines, machine)
		}
	}

	return matchingMachines
}

// func (r *DockerMachinePoolReconciler) initNodePoolMachineStatusList(ctx context.Context, dockerMachines []infrav1.DockerMachine, dockerMachinePool *infraexpv1.DockerMachinePool) ([]dockerexp.NodePoolMachineStatus, error) {
// 	log := ctrl.LoggerFrom(ctx)

// 	nodePoolInstances := make([]dockerexp.NodePoolMachineStatus, len(dockerMachines))
// 	for i := range dockerMachines {
// 		// Needed to avoid implicit memory aliasing of the loop variable.
// 		dockerMachine := dockerMachines[i]

// 		// Try to get the owner machine to see if it has a delete annotation. If it doesn't exist, we'll requeue until it does.
// 		machine, err := util.GetOwnerMachine(ctx, r.Client, dockerMachine.ObjectMeta)
// 		if err != nil {
// 			return nil, errors.Wrapf(err, "failed to get owner Machine for DockerMachine %s/%s", dockerMachine.Namespace, dockerMachine.Name)
// 		}

// 		hasDeleteAnnotation := false
// 		if machine != nil {
// 			if machine.Annotations != nil {
// 				_, hasDeleteAnnotation = machine.Annotations[clusterv1.DeleteMachineAnnotation]
// 			}
// 		} else {
// 			sampleMachine := &clusterv1.Machine{}
// 			sampleMachine.APIVersion = clusterv1.GroupVersion.String()
// 			sampleMachine.Kind = "Machine"
// 			log.Info("DockerMachine has no parent Machine, will set up a watch and initNodePoolMachineStatusList", "dockerMachine", dockerMachine.Name, "namespace", dockerMachine.Namespace)
// 			// If machine == nil, then no Machine was found in the ownerRefs at all. Don't block nodepool reconciliation, but set up a Watch() instead.
// 			if err := r.externalTracker.Watch(log, sampleMachine, handler.EnqueueRequestsFromMapFunc(r.machineToDockerMachinePoolMapper(ctx, &dockerMachine, dockerMachinePool))); err != nil {
// 				return nil, errors.Wrapf(err, "failed to set watch for Machines %s/%s", dockerMachine.Namespace, dockerMachine.Name)
// 			}
// 		}

// 		nodePoolInstances[i] = dockerexp.NodePoolMachineStatus{
// 			Name:             dockerMachine.Name,
// 			PrioritizeDelete: hasDeleteAnnotation,
// 		}
// 	}

// 	return nodePoolInstances, nil
// }

func isMachinePoolDeleted(ctx context.Context, c client.Client, machinePool *expv1.MachinePool) bool {
	mp := &expv1.MachinePool{}
	key := client.ObjectKey{Name: machinePool.Name, Namespace: machinePool.Namespace}
	if err := c.Get(ctx, key, mp); apierrors.IsNotFound(err) || !mp.DeletionTimestamp.IsZero() {
		return true
	}

	return false
}

// machineToDockerMachinePoolMapper is a mapper function that maps an InfraMachine to the MachinePool that owns it.
// This is used to trigger an update of the MachinePool when a InfraMachine is changed.
func (r *DockerMachinePoolReconciler) machineToDockerMachinePoolMapper(_ context.Context, dockerMachine *infrav1.DockerMachine, dockerMachinePool *infraexpv1.DockerMachinePool) func(context.Context, client.Object) []ctrl.Request {
	return func(ctx context.Context, o client.Object) []ctrl.Request {
		machine, ok := o.(*clusterv1.Machine)
		if !ok {
			return nil
		}

		if machine.Spec.InfrastructureRef.Name == dockerMachine.Name {
			return []ctrl.Request{
				{
					NamespacedName: client.ObjectKey{
						Namespace: dockerMachinePool.Namespace,
						Name:      dockerMachinePool.Name,
					},
				},
			}
		}

		return nil
	}
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
