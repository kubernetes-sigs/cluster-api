/*
Copyright 2026 The Kubernetes Authors.

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

package docker

import (
	"context"
	"fmt"
	"math/rand"
	"sort"

	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/kind/pkg/cluster/constants"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/internal/util/ssa"
	"sigs.k8s.io/cluster-api/test/infrastructure/container"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta2"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/docker"
	"sigs.k8s.io/cluster-api/test/infrastructure/kind"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
	"sigs.k8s.io/cluster-api/util/labels/format"
	"sigs.k8s.io/cluster-api/util/patch"
)

const (
	// devMachinePoolLabel is the label used to identify the DevMachinePool.
	devMachinePoolLabel = "dev.cluster.x-k8s.io/machine-pool"

	devMachinePoolControllerName = "devmachinepool-controller"
)

// MachinePoolBackEndReconciler reconciles a DockerMachinePool object.
type MachinePoolBackEndReconciler struct {
	client.Client
	ContainerRuntime container.Runtime
	SsaCache         ssa.Cache
}

// ReconcileNormal handle docker backend for DevMachinePool not yet deleted.
func (r *MachinePoolBackEndReconciler) ReconcileNormal(ctx context.Context, cluster *clusterv1.Cluster, machinePool *clusterv1.MachinePool, devMachinePool *infrav1.DevMachinePool) (ctrl.Result, error) {
	if devMachinePool.Spec.Template.Docker == nil {
		return ctrl.Result{}, errors.New("DockerMachinePoolBackEndReconciler can't be called for DevMachinePools without a Docker backend")
	}

	log := ctrl.LoggerFrom(ctx)

	// Make sure bootstrap data is available and populated.
	if machinePool.Spec.Template.Spec.Bootstrap.DataSecretName == nil {
		log.Info("Waiting for the Bootstrap provider controller to set bootstrap data")
		return ctrl.Result{}, nil
	}

	if machinePool.Spec.Replicas == nil {
		machinePool.Spec.Replicas = ptr.To[int32](1)
	}

	// First, reconcile the Docker containers, but do not delete any as we need to delete the Machine to ensure node cordon/drain.
	// Similarly, providers implementing MachinePool Machines will need to reconcile their analogous infrastructure instances (aside
	// from deletion) before reconciling InfraMachinePoolMachines.
	if err := r.reconcileDockerContainers(ctx, cluster, machinePool, devMachinePool); err != nil {
		return ctrl.Result{}, err
	}

	// Second, once the Docker containers are created, reconcile the DevMachines. This function creates a DevMachine for each newly created Docker
	// container, and handles container deletion. Instead of deleting an infrastructure instance directly, we want to delete the owner Machine. This will
	// trigger a cordon and drain of the node, as well as trigger the deletion of the DevMachine, which in turn causes the Docker container to be deleted.
	// Similarly, providers will need to create InfraMachines for each instance, and instead of deleting instances directly, delete the owner Machine.
	if err := r.reconcileDevMachines(ctx, cluster, machinePool, devMachinePool); err != nil {
		return ctrl.Result{}, err
	}

	// Fetch the list of DevMachines to ensure the provider IDs are up to date.
	devMachineList, err := getDevMachines(ctx, r.Client, *cluster, *machinePool, *devMachinePool)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Derive providerIDList from the provider ID on each DevMachine if it exists. The providerID is set by the DevMachine controller.
	devMachinePool.Spec.ProviderIDList = []string{}
	for _, devMachine := range devMachineList.Items {
		if devMachine.Spec.ProviderID != "" {
			devMachinePool.Spec.ProviderIDList = append(devMachinePool.Spec.ProviderIDList, devMachine.Spec.ProviderID)
		}
	}
	// Ensure the providerIDList is deterministic (getDevMachines doesn't guarantee a specific order)
	sort.Strings(devMachinePool.Spec.ProviderIDList)

	devMachinePool.Status.Replicas = int32(len(devMachineList.Items))

	if devMachinePool.Spec.ProviderID == "" {
		// This is a fake provider ID which does not tie back to any docker infrastructure. In cloud providers,
		// this ID would tie back to the resource which manages the machine pool implementation. For example,
		// Azure uses a VirtualMachineScaleSet to manage a set of like machines.
		devMachinePool.Spec.ProviderID = getDevMachinePoolProviderID(cluster.Name, devMachinePool.Name)
	}

	if len(devMachinePool.Spec.ProviderIDList) == int(*machinePool.Spec.Replicas) && len(devMachineList.Items) == int(*machinePool.Spec.Replicas) {
		devMachinePool.Status.Ready = true
		conditions.Set(devMachinePool, metav1.Condition{
			Type:   infrav1.ReplicasReadyCondition,
			Status: metav1.ConditionTrue,
			Reason: infrav1.ReplicasReadyReason,
		})

		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

// ReconcileDelete handle docker backend for delete DevMachinePool.
func (r *MachinePoolBackEndReconciler) ReconcileDelete(ctx context.Context, cluster *clusterv1.Cluster, machinePool *clusterv1.MachinePool, devMachinePool *infrav1.DevMachinePool) (ctrl.Result, error) {
	if devMachinePool.Spec.Template.Docker == nil {
		return ctrl.Result{}, errors.New("DockerMachinePoolBackEndReconciler can't be called for DevMachinePools without a Docker backend")
	}

	log := ctrl.LoggerFrom(ctx)

	devMachineList, err := getDevMachines(ctx, r.Client, *cluster, *machinePool, *devMachinePool)
	if err != nil {
		return ctrl.Result{}, err
	}

	if len(devMachineList.Items) > 0 {
		log.Info("DevMachinePool still has dependent DevMachines, deleting them first and requeuing", "count", len(devMachineList.Items))

		var errs []error

		for _, devMachine := range devMachineList.Items {
			if !devMachine.GetDeletionTimestamp().IsZero() {
				// Don't handle deleted child
				continue
			}

			if err := r.deleteMachinePoolMachine(ctx, devMachine); err != nil {
				err = errors.Wrapf(err, "error deleting DevMachinePool %s/%s: failed to delete %s %s", devMachinePool.Namespace, devMachinePool.Name, devMachine.Namespace, devMachine.Name)
				errs = append(errs, err)
			}
		}

		if len(errs) > 0 {
			return ctrl.Result{}, kerrors.NewAggregate(errs)
		}
		return ctrl.Result{}, nil
	}

	// Once there are no DevMachines left, ensure there are no Docker containers left behind.
	// This can occur if deletion began after containers were created but before the DevMachines were created, or if creation of a DevMachine failed.
	log.Info("DevMachines have been deleted, deleting any remaining Docker containers")

	labelFilters := map[string]string{devMachinePoolLabel: devMachinePool.Name}
	// List Docker containers, i.e. external machines in the cluster.
	externalMachines, err := docker.ListMachinesByCluster(ctx, cluster, labelFilters)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to list all machines in the cluster with label \"%s:%s\"", devMachinePoolLabel, devMachinePool.Name)
	}

	// Providers should similarly ensure that all infrastructure instances are deleted even if the InfraMachine has not been created yet.
	for _, externalMachine := range externalMachines {
		log.Info("Deleting Docker container", "container", externalMachine.Name())
		if err := externalMachine.Delete(ctx); err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to delete machine %s", externalMachine.Name())
		}
	}

	// Once all DockerMachines and Docker containers are deleted, remove the finalizer.
	controllerutil.RemoveFinalizer(devMachinePool, infrav1.MachinePoolFinalizer)

	return ctrl.Result{}, nil
}

// PatchDevMachinePool patch a DevMachinePool.
func (r *MachinePoolBackEndReconciler) PatchDevMachinePool(ctx context.Context, patchHelper *patch.Helper, devMachinePool *infrav1.DevMachinePool) error {
	if devMachinePool.Spec.Template.Docker == nil {
		return errors.New("DockerMachinePoolBackEndReconciler can't be called for DevMachinePools without a Docker backend")
	}

	// Always update the readyCondition by summarizing the state of other conditions.
	// A step counter is added to represent progress during the provisioning process (instead we are hiding it during the deletion process).
	if err := conditions.SetSummaryCondition(devMachinePool, devMachinePool, infrav1.DevMachinePoolReadyCondition,
		conditions.ForConditionTypes{
			infrav1.ReplicasReadyCondition,
		},
		// Using a custom merge strategy to override reasons applied during merge.
		conditions.CustomMergeStrategy{
			MergeStrategy: conditions.DefaultMergeStrategy(
				// Use custom reasons.
				conditions.ComputeReasonFunc(conditions.GetDefaultComputeMergeReasonFunc(
					infrav1.DevMachinePoolNotReadyReason,
					infrav1.DevMachinePoolReadyUnknownReason,
					infrav1.DevMachinePoolReadyReason,
				)),
			),
		},
	); err != nil {
		return errors.Wrapf(err, "failed to set %s condition", infrav1.DevMachinePoolReadyCondition)
	}

	// Patch the object, ignoring conflicts on the conditions owned by this controller.
	return patchHelper.Patch(
		ctx,
		devMachinePool,
		patch.WithOwnedConditions{Conditions: []string{
			clusterv1.PausedCondition,
			infrav1.DevMachinePoolReadyCondition,
			infrav1.ReplicasReadyCondition,
		}},
	)
}

// reconcileDockerContainers manages the Docker containers for a MachinePool such that it
// - Ensures the number of up-to-date Docker containers is equal to the MachinePool's desired replica count.
// - Does not delete any containers as that must be triggered in reconcileDockerMachines to ensure node cordon/drain.
//
// Providers should similarly create their infrastructure instances and reconcile any additional logic.
func (r *MachinePoolBackEndReconciler) reconcileDockerContainers(ctx context.Context, cluster *clusterv1.Cluster, machinePool *clusterv1.MachinePool, devMachinePool *infrav1.DevMachinePool) error {
	log := ctrl.LoggerFrom(ctx)

	log.V(2).Info("Reconciling Docker containers", "DevMachinePool", klog.KObj(devMachinePool))

	labelFilters := map[string]string{devMachinePoolLabel: devMachinePool.Name}

	machines, err := docker.ListMachinesByCluster(ctx, cluster, labelFilters)
	if err != nil {
		return errors.Wrapf(err, "failed to list all machines in the cluster")
	}

	matchingMachineCount := len(machinesMatchingInfrastructureSpec(ctx, machines, machinePool, devMachinePool))
	numToCreate := int(*machinePool.Spec.Replicas) - matchingMachineCount
	for range numToCreate {
		log.V(2).Info("Creating a new Docker container for machinePool", "MachinePool", klog.KObj(machinePool))
		name := fmt.Sprintf("worker-%s", util.RandomString(6))
		if err := createDockerContainer(ctx, name, cluster, machinePool, devMachinePool); err != nil {
			return errors.Wrap(err, "failed to create a new docker machine")
		}
	}

	return nil
}

// reconcileDevMachines creates and deletes DevMachines to match the MachinePool's desired number of replicas and infrastructure spec.
// It is responsible for
// - Ensuring each Docker container has an associated DevMachine by creating one if it doesn't already exist.
// - Ensuring that deletion for Docker container happens by calling delete on the associated Machine so that the node is cordoned/drained and the infrastructure is cleaned up.
// - Deleting DevMachines referencing a container whose Kubernetes version or custom image no longer matches the spec.
// - Deleting DevMachines that correspond to a deleted/non-existent Docker container.
// - Deleting DevMachines when scaling down such that DevMachines whose owner Machine has the clusterv1.DeleteMachineAnnotation is given priority.
func (r *MachinePoolBackEndReconciler) reconcileDevMachines(ctx context.Context, cluster *clusterv1.Cluster, machinePool *clusterv1.MachinePool, devMachinePool *infrav1.DevMachinePool) error {
	log := ctrl.LoggerFrom(ctx)

	log.V(2).Info("Reconciling DevMachines", "DevMachinePool", klog.KObj(devMachinePool))

	devMachineList, err := getDevMachines(ctx, r.Client, *cluster, *machinePool, *devMachinePool)
	if err != nil {
		return err
	}

	devMachineMap := make(map[string]infrav1.DevMachine)
	for _, devMachine := range devMachineList.Items {
		devMachineMap[devMachine.Name] = devMachine
	}

	// List the Docker containers. This corresponds to a InfraMachinePool instance for providers.
	labelFilters := map[string]string{devMachinePoolLabel: devMachinePool.Name}
	externalMachines, err := docker.ListMachinesByCluster(ctx, cluster, labelFilters)
	if err != nil {
		return errors.Wrapf(err, "failed to list all machines in the cluster")
	}

	externalMachineMap := make(map[string]*docker.Machine)
	for _, externalMachine := range externalMachines {
		externalMachineMap[externalMachine.Name()] = externalMachine
	}

	// Step 1:
	// Create a DevMachine for each Docker container so we surface the information to the user. Use the same name as the Docker container for the Dev Machine for ease of lookup.
	// Providers should iterate through their infrastructure instances and ensure that each instance has a corresponding InfraMachine.
	for _, machine := range externalMachines {
		if existingMachine, ok := devMachineMap[machine.Name()]; ok {
			log.V(2).Info("Patching existing DevMachine", "DevMachine", klog.KObj(&existingMachine))
			desiredMachine := computeDesiredDevMachine(machine.Name(), cluster, machinePool, devMachinePool, &existingMachine)
			if err := ssa.Patch(ctx, r.Client, devMachinePoolControllerName, desiredMachine, ssa.WithCachingProxy{Cache: r.SsaCache, Original: &existingMachine}); err != nil {
				return errors.Wrapf(err, "failed to update DockerMachine %q", klog.KObj(desiredMachine))
			}

			devMachineMap[desiredMachine.Name] = *desiredMachine
		} else {
			log.V(2).Info("Creating a new DevMachine for Docker container", "container", machine.Name())
			desiredMachine := computeDesiredDevMachine(machine.Name(), cluster, machinePool, devMachinePool, nil)
			if err := ssa.Patch(ctx, r.Client, devMachinePoolControllerName, desiredMachine); err != nil {
				return errors.Wrap(err, "failed to create a new dev machine")
			}

			devMachineMap[desiredMachine.Name] = *desiredMachine
		}
	}

	// Step 2:
	// Delete any DevMachine that correspond to a deleted Docker container.
	// Providers should iterate through the InfraMachines to ensure each one still corresponds to an existing infrastructure instance.
	// This allows the InfraMachine (and owner Machine) to be deleted and avoid hanging resources when a user deletes an instance out-of-band.
	for _, devMachine := range devMachineMap {
		if _, ok := externalMachineMap[devMachine.Name]; !ok {
			devMachine := devMachine
			log.V(2).Info("Deleting DevMachine with no underlying infrastructure", "DevMachine", klog.KObj(&devMachine))
			if err := r.deleteMachinePoolMachine(ctx, devMachine); err != nil {
				return err
			}

			delete(devMachineMap, devMachine.Name)
		}
	}

	// Step 3:
	// This handles the scale down/excess replicas case and the case where a rolling upgrade is needed.
	// If there are more ready DevMachines than desired replicas, start to delete the excess DevMachines such that
	// - DevMachines with an outdated Kubernetes version or custom image are deleted first (i.e. the rolling upgrade).
	// - DevMachines whose owner Machine contains the clusterv1.DeleteMachineAnnotation are deleted next (to support cluster autoscaler).
	// Note: we want to ensure that there are always enough ready DevMachines before deleting anything or scaling down.

	// For each DevMachine, fetch the owner Machine and copy the clusterv1.DeleteMachineAnnotation to the DevMachine if it exists before sorting the DevMachines.
	// This is done just before sorting to guarantee we have the latest copy of the Machine annotations.
	devMachinesWithAnnotation, err := r.propagateMachineDeleteAnnotation(ctx, devMachineMap)
	if err != nil {
		return err
	}

	// Sort DockerMachines with the clusterv1.DeleteMachineAnnotation to the front of each list.
	// If providers already have a sorting order for instance deletion, i.e. oldest first or newest first, the clusterv1.DeleteMachineAnnotation must take priority.
	// For example, if deleting by oldest, we expect the InfraMachines with clusterv1.DeleteMachineAnnotation to be deleted first followed by the oldest, and the second oldest, etc.
	orderedDevMachines := orderByDeleteMachineAnnotation(devMachinesWithAnnotation)

	// Note: this includes DockerMachines that are out of date but still ready. This is to ensure we always have enough ready DockerMachines before deleting anything.
	totalReadyMachines := 0
	for i := range orderedDevMachines {
		devMachine := orderedDevMachines[i]
		// TODO (v1beta2): test for v1beta2 conditions
		if ptr.Deref(devMachine.Status.Initialization.Provisioned, false) || v1beta1conditions.IsTrue(&devMachine, clusterv1.ReadyV1Beta1Condition) {
			totalReadyMachines++
		}
	}

	outdatedMachines, readyMachines, err := r.getDeletionCandidates(ctx, orderedDevMachines, externalMachineMap, machinePool, devMachinePool)
	if err != nil {
		return err
	}

	desiredReplicas := int(*machinePool.Spec.Replicas)
	overProvisionCount := totalReadyMachines - desiredReplicas

	// Loop through outdated DevMachines first and decrement the overProvisionCount until it reaches 0.
	for _, devMachine := range outdatedMachines {
		if overProvisionCount > 0 {
			devMachine := devMachine
			log.V(2).Info("Deleting DevMachine because it is outdated", "DevMachine", klog.KObj(&devMachine))
			if err := r.deleteMachinePoolMachine(ctx, devMachine); err != nil {
				return err
			}

			overProvisionCount--
		}
	}

	// Then, loop through the ready DevMachines first and decrement the overProvisionCount until it reaches 0.
	for _, devMachine := range readyMachines {
		if overProvisionCount > 0 {
			devMachine := devMachine
			log.V(2).Info("Deleting Devmachine because it is an excess replica", "DevMachine", klog.KObj(&devMachine))
			if err := r.deleteMachinePoolMachine(ctx, devMachine); err != nil {
				return err
			}

			overProvisionCount--
		}
	}

	return nil
}

// deleteMachinePoolMachine attempts to delete a DevMachine and its associated owner Machine if it exists.
func (r *MachinePoolBackEndReconciler) deleteMachinePoolMachine(ctx context.Context, devMachine infrav1.DevMachine) error {
	log := ctrl.LoggerFrom(ctx)

	machine, err := util.GetOwnerMachine(ctx, r.Client, devMachine.ObjectMeta)
	if err != nil {
		return errors.Wrapf(err, "error getting owner Machine for DevMachine %s/%s", devMachine.Namespace, devMachine.Name)
	}
	// util.GetOwnerMachine() returns a nil Machine without error if there is no Machine kind in the ownerRefs, so we must verify that machine is not nil.
	if machine == nil {
		log.V(2).Info("No owner Machine exists for DevMachine", "devMachine", klog.KObj(&devMachine))

		// If the DevMachine does not have an owner Machine, do not attempt to delete the DevMachine as the MachinePool controller will create the
		// Machine and we want to let it catch up. If we are too hasty to delete, that introduces a race condition where the DevMachine could be deleted
		// just as the Machine comes online.

		// In the case where the MachinePool is being deleted and the Machine will never come online, the DevMachine will be deleted via its ownerRef to the
		// DevMachinePool, so that is covered as well.

		return nil
	}

	log.Info("Deleting Machine for DevMachine", "Machine", klog.KObj(machine), "DevMachine", klog.KObj(&devMachine))

	if err := r.Client.Delete(ctx, machine); err != nil {
		return errors.Wrapf(err, "failed to delete Machine %s/%s", machine.Namespace, machine.Name)
	}

	return nil
}

// propagateMachineDeleteAnnotation returns the DevMachines for a MachinePool and for each DevMachine, it copies the owner
// Machine's delete annotation to each DevMachine if it's present. This is done just in time to ensure that the annotations are
// up to date when we sort for DevMachine deletion.
func (r *MachinePoolBackEndReconciler) propagateMachineDeleteAnnotation(ctx context.Context, devMachineMap map[string]infrav1.DevMachine) ([]infrav1.DevMachine, error) {
	_ = ctrl.LoggerFrom(ctx)

	devMachines := []infrav1.DevMachine{}
	for _, devMachine := range devMachineMap {
		machine, err := util.GetOwnerMachine(ctx, r.Client, devMachine.ObjectMeta)
		if err != nil {
			return nil, errors.Wrapf(err, "error getting owner Machine for DockerMachine %s/%s", devMachine.Namespace, devMachine.Name)
		}
		if machine != nil && machine.Annotations != nil {
			if devMachine.Annotations == nil {
				devMachine.Annotations = map[string]string{}
			}
			if _, hasDeleteAnnotation := machine.Annotations[clusterv1.DeleteMachineAnnotation]; hasDeleteAnnotation {
				devMachine.Annotations[clusterv1.DeleteMachineAnnotation] = machine.Annotations[clusterv1.DeleteMachineAnnotation]
			}
		}

		devMachines = append(devMachines, devMachine)
	}

	return devMachines, nil
}

// getDeletionCandidates returns the DevMachines for a MachinePool that do not match the infrastructure spec followed by any DevMachines that are ready and up to date, i.e. matching the infrastructure spec.
func (r *MachinePoolBackEndReconciler) getDeletionCandidates(ctx context.Context, devMachines []infrav1.DevMachine, externalMachineSet map[string]*docker.Machine, machinePool *clusterv1.MachinePool, devMachinePool *infrav1.DevMachinePool) (outdatedMachines []infrav1.DevMachine, readyMatchingMachines []infrav1.DevMachine, err error) {
	for i := range devMachines {
		devMachine := devMachines[i]
		externalMachine, ok := externalMachineSet[devMachine.Name]
		if !ok {
			// Note: Since we deleted any DevMachines that do not have an associated Docker container earlier, we should never hit this case.
			return nil, nil, errors.Errorf("failed to find externalMachine for DevMachine %s/%s", devMachine.Namespace, devMachine.Name)
		}

		// TODO (v1beta2): test for v1beta2 conditions
		if !isMachineMatchingInfrastructureSpec(ctx, externalMachine, machinePool, devMachinePool) {
			outdatedMachines = append(outdatedMachines, devMachine)
		} else if ptr.Deref(devMachine.Status.Initialization.Provisioned, false) || v1beta1conditions.IsTrue(&devMachine, clusterv1.ReadyV1Beta1Condition) {
			readyMatchingMachines = append(readyMatchingMachines, devMachine)
		}
	}

	return outdatedMachines, readyMatchingMachines, nil
}

// machinesMatchingInfrastructureSpec returns the  Docker containers matching the custom image in the DockerMachinePool spec.
func machinesMatchingInfrastructureSpec(ctx context.Context, machines []*docker.Machine, machinePool *clusterv1.MachinePool, devMachinePool *infrav1.DevMachinePool) []*docker.Machine {
	var matchingMachines []*docker.Machine
	for _, machine := range machines {
		if isMachineMatchingInfrastructureSpec(ctx, machine, machinePool, devMachinePool) {
			matchingMachines = append(matchingMachines, machine)
		}
	}

	return matchingMachines
}

// isMachineMatchingInfrastructureSpec returns true if the Docker container image matches the custom image in the DevMachinePool spec.
func isMachineMatchingInfrastructureSpec(_ context.Context, machine *docker.Machine, machinePool *clusterv1.MachinePool, dockerMachinePool *infrav1.DevMachinePool) bool {
	// NOTE: With the current implementation we are checking if the machine is using a kindest/node image for the expected version,
	// but not checking if the machine has the expected extra.mounts or pre.loaded images.

	semVer, err := semver.ParseTolerant(machinePool.Spec.Template.Spec.Version)
	if err != nil {
		// TODO: consider if to return an error
		panic(errors.Wrap(err, "failed to parse DockerMachine version").Error())
	}

	kindMapping := kind.GetMapping(semVer, dockerMachinePool.Spec.Template.Docker.CustomImage)

	return machine.ContainerImage() == kindMapping.Image
}

// createDockerContainer creates a Docker container to serve as a replica for the MachinePool.
func createDockerContainer(ctx context.Context, name string, cluster *clusterv1.Cluster, machinePool *clusterv1.MachinePool, devMachinePool *infrav1.DevMachinePool) error {
	log := ctrl.LoggerFrom(ctx)
	labelFilters := map[string]string{devMachinePoolLabel: devMachinePool.Name}
	externalMachine, err := docker.NewMachine(ctx, cluster, name, labelFilters)
	if err != nil {
		return errors.Wrapf(err, "failed to create helper for managing the externalMachine named %s", name)
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
		for k, v := range docker.FailureDomainLabel(machinePool.Spec.FailureDomains[randomIndex]) {
			labels[k] = v
		}
	}

	// If re-entering the reconcile loop and reaching this point, the container is expected to be running. If it is not, delete it so we can try to create it again.
	if externalMachine.Exists() && !externalMachine.IsRunning() {
		// This deletes the machine and results in re-creating it below.
		if err := externalMachine.Delete(ctx); err != nil {
			return errors.Wrap(err, "Failed to delete not running DockerMachine")
		}
	}

	log.Info("Creating container for machinePool", "name", name, "MachinePool", klog.KObj(machinePool), "machinePool.Spec.Template.Spec.Version", machinePool.Spec.Template.Spec.Version)
	if err := externalMachine.Create(ctx, devMachinePool.Spec.Template.Docker.CustomImage, constants.WorkerNodeRoleValue, machinePool.Spec.Template.Spec.Version, labels, devMachinePool.Spec.Template.Docker.ExtraMounts); err != nil {
		return errors.Wrapf(err, "failed to create docker machine with name %s", name)
	}
	return nil
}

func getDevMachines(ctx context.Context, c client.Client, cluster clusterv1.Cluster, machinePool clusterv1.MachinePool, devMachinePool infrav1.DevMachinePool) (*infrav1.DevMachineList, error) {
	devMachineList := &infrav1.DevMachineList{}
	labels := map[string]string{
		clusterv1.ClusterNameLabel:     cluster.Name,
		clusterv1.MachinePoolNameLabel: machinePool.Name,
	}
	if err := c.List(ctx, devMachineList, client.InNamespace(devMachinePool.Namespace), client.MatchingLabels(labels)); err != nil {
		return nil, err
	}

	return devMachineList, nil
}

// computeDesiredDevMachine creates a Devmachine to represent a Docker container in a DevMachinePool.
// These DevMachines have the clusterv1.ClusterNameLabel and clusterv1.MachinePoolNameLabel to support MachinePool Machines.
func computeDesiredDevMachine(name string, cluster *clusterv1.Cluster, machinePool *clusterv1.MachinePool, devMachinePool *infrav1.DevMachinePool, existingDevMachine *infrav1.DevMachine) *infrav1.DevMachine {
	devMachine := &infrav1.DevMachine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   devMachinePool.Namespace,
			Name:        name,
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
		},
		Spec: infrav1.DevMachineSpec{
			Backend: infrav1.DevMachineBackendSpec{
				Docker: &infrav1.DockerMachineBackendSpec{
					CustomImage:   devMachinePool.Spec.Template.Docker.CustomImage,
					PreLoadImages: devMachinePool.Spec.Template.Docker.PreLoadImages,
					ExtraMounts:   devMachinePool.Spec.Template.Docker.ExtraMounts,
				},
			},
		},
	}

	if existingDevMachine != nil {
		devMachine.SetUID(existingDevMachine.UID)
		devMachine.SetOwnerReferences(existingDevMachine.OwnerReferences)
	}

	// Note: Since the MachinePool controller has not created its owner Machine yet, we want to set the DevMachinePool as the owner so it's not orphaned.
	devMachine.SetOwnerReferences(util.EnsureOwnerRef(devMachine.OwnerReferences, metav1.OwnerReference{
		APIVersion: infrav1.GroupVersion.String(),
		Kind:       "DevMachinePool",
		Name:       devMachinePool.Name,
		UID:        devMachinePool.UID,
	}))
	devMachine.Labels[clusterv1.ClusterNameLabel] = cluster.Name
	devMachine.Labels[clusterv1.MachinePoolNameLabel] = format.MustFormatValue(machinePool.Name)

	return devMachine
}

// orderByDeleteMachineAnnotation will sort DevrMachines with the clusterv1.DeleteMachineAnnotation to the front of the list.
// It will preserve the existing order of the list otherwise so that it respects the existing delete priority otherwise.
func orderByDeleteMachineAnnotation(machines []infrav1.DevMachine) []infrav1.DevMachine {
	sort.SliceStable(machines, func(i, _ int) bool {
		_, iHasAnnotation := machines[i].Annotations[clusterv1.DeleteMachineAnnotation]

		return iHasAnnotation
	})

	return machines
}

func getDevMachinePoolProviderID(clusterName, devMachinePoolName string) string {
	return fmt.Sprintf("dev:////%s-dmp-%s", clusterName, devMachinePoolName)
}
