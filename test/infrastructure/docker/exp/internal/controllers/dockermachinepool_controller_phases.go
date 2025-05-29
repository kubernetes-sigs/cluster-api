/*
Copyright 2023 The Kubernetes Authors.

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

	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/kind/pkg/cluster/constants"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/internal/util/ssa"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta2"
	infraexpv1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/exp/api/v1beta2"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/docker"
	"sigs.k8s.io/cluster-api/test/infrastructure/kind"
	"sigs.k8s.io/cluster-api/util"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
	"sigs.k8s.io/cluster-api/util/labels/format"
)

// reconcileDockerContainers manages the Docker containers for a MachinePool such that it
// - Ensures the number of up-to-date Docker containers is equal to the MachinePool's desired replica count.
// - Does not delete any containers as that must be triggered in reconcileDockerMachines to ensure node cordon/drain.
//
// Providers should similarly create their infrastructure instances and reconcile any additional logic.
func (r *DockerMachinePoolReconciler) reconcileDockerContainers(ctx context.Context, cluster *clusterv1.Cluster, machinePool *clusterv1.MachinePool, dockerMachinePool *infraexpv1.DockerMachinePool) error {
	log := ctrl.LoggerFrom(ctx)

	log.V(2).Info("Reconciling Docker containers", "DockerMachinePool", klog.KObj(dockerMachinePool))

	labelFilters := map[string]string{dockerMachinePoolLabel: dockerMachinePool.Name}

	machines, err := docker.ListMachinesByCluster(ctx, cluster, labelFilters)
	if err != nil {
		return errors.Wrapf(err, "failed to list all machines in the cluster")
	}

	matchingMachineCount := len(machinesMatchingInfrastructureSpec(ctx, machines, machinePool, dockerMachinePool))
	numToCreate := int(*machinePool.Spec.Replicas) - matchingMachineCount
	for range numToCreate {
		log.V(2).Info("Creating a new Docker container for machinePool", "MachinePool", klog.KObj(machinePool))
		name := fmt.Sprintf("worker-%s", util.RandomString(6))
		if err := createDockerContainer(ctx, name, cluster, machinePool, dockerMachinePool); err != nil {
			return errors.Wrap(err, "failed to create a new docker machine")
		}
	}

	return nil
}

// createDockerContainer creates a Docker container to serve as a replica for the MachinePool.
func createDockerContainer(ctx context.Context, name string, cluster *clusterv1.Cluster, machinePool *clusterv1.MachinePool, dockerMachinePool *infraexpv1.DockerMachinePool) error {
	log := ctrl.LoggerFrom(ctx)
	labelFilters := map[string]string{dockerMachinePoolLabel: dockerMachinePool.Name}
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
		for k, v := range docker.FailureDomainLabel(&machinePool.Spec.FailureDomains[randomIndex]) {
			labels[k] = v
		}
	}

	log.Info("Creating container for machinePool", "name", name, "MachinePool", klog.KObj(machinePool))
	if err := externalMachine.Create(ctx, dockerMachinePool.Spec.Template.CustomImage, constants.WorkerNodeRoleValue, machinePool.Spec.Template.Spec.Version, labels, dockerMachinePool.Spec.Template.ExtraMounts); err != nil {
		return errors.Wrapf(err, "failed to create docker machine with name %s", name)
	}
	return nil
}

// reconcileDockerMachines creates and deletes DockerMachines to match the MachinePool's desired number of replicas and infrastructure spec.
// It is responsible for
// - Ensuring each Docker container has an associated DockerMachine by creating one if it doesn't already exist.
// - Ensuring that deletion for Docker container happens by calling delete on the associated Machine so that the node is cordoned/drained and the infrastructure is cleaned up.
// - Deleting DockerMachines referencing a container whose Kubernetes version or custom image no longer matches the spec.
// - Deleting DockerMachines that correspond to a deleted/non-existent Docker container.
// - Deleting DockerMachines when scaling down such that DockerMachines whose owner Machine has the clusterv1.DeleteMachineAnnotation is given priority.
func (r *DockerMachinePoolReconciler) reconcileDockerMachines(ctx context.Context, cluster *clusterv1.Cluster, machinePool *clusterv1.MachinePool, dockerMachinePool *infraexpv1.DockerMachinePool) error {
	log := ctrl.LoggerFrom(ctx)

	log.V(2).Info("Reconciling DockerMachines", "DockerMachinePool", klog.KObj(dockerMachinePool))

	dockerMachineList, err := getDockerMachines(ctx, r.Client, *cluster, *machinePool, *dockerMachinePool)
	if err != nil {
		return err
	}

	dockerMachineMap := make(map[string]infrav1.DockerMachine)
	for _, dockerMachine := range dockerMachineList.Items {
		dockerMachineMap[dockerMachine.Name] = dockerMachine
	}

	// List the Docker containers. This corresponds to a InfraMachinePool instance for providers.
	labelFilters := map[string]string{dockerMachinePoolLabel: dockerMachinePool.Name}
	externalMachines, err := docker.ListMachinesByCluster(ctx, cluster, labelFilters)
	if err != nil {
		return errors.Wrapf(err, "failed to list all machines in the cluster")
	}

	externalMachineMap := make(map[string]*docker.Machine)
	for _, externalMachine := range externalMachines {
		externalMachineMap[externalMachine.Name()] = externalMachine
	}

	// Step 1:
	// Create a DockerMachine for each Docker container so we surface the information to the user. Use the same name as the Docker container for the Docker Machine for ease of lookup.
	// Providers should iterate through their infrastructure instances and ensure that each instance has a corresponding InfraMachine.
	for _, machine := range externalMachines {
		if existingMachine, ok := dockerMachineMap[machine.Name()]; ok {
			log.V(2).Info("Patching existing DockerMachine", "DockerMachine", klog.KObj(&existingMachine))
			desiredMachine := computeDesiredDockerMachine(machine.Name(), cluster, machinePool, dockerMachinePool, &existingMachine)
			if err := ssa.Patch(ctx, r.Client, dockerMachinePoolControllerName, desiredMachine, ssa.WithCachingProxy{Cache: r.ssaCache, Original: &existingMachine}); err != nil {
				return errors.Wrapf(err, "failed to update DockerMachine %q", klog.KObj(desiredMachine))
			}

			dockerMachineMap[desiredMachine.Name] = *desiredMachine
		} else {
			log.V(2).Info("Creating a new DockerMachine for Docker container", "container", machine.Name())
			desiredMachine := computeDesiredDockerMachine(machine.Name(), cluster, machinePool, dockerMachinePool, nil)
			if err := ssa.Patch(ctx, r.Client, dockerMachinePoolControllerName, desiredMachine); err != nil {
				return errors.Wrap(err, "failed to create a new docker machine")
			}

			dockerMachineMap[desiredMachine.Name] = *desiredMachine
		}
	}

	// Step 2:
	// Delete any DockerMachines that correspond to a deleted Docker container.
	// Providers should iterate through the InfraMachines to ensure each one still corresponds to an existing infrastructure instance.
	// This allows the InfraMachine (and owner Machine) to be deleted and avoid hanging resources when a user deletes an instance out-of-band.
	for _, dockerMachine := range dockerMachineMap {
		if _, ok := externalMachineMap[dockerMachine.Name]; !ok {
			dockerMachine := dockerMachine
			log.V(2).Info("Deleting DockerMachine with no underlying infrastructure", "DockerMachine", klog.KObj(&dockerMachine))
			if err := r.deleteMachinePoolMachine(ctx, dockerMachine); err != nil {
				return err
			}

			delete(dockerMachineMap, dockerMachine.Name)
		}
	}

	// Step 3:
	// This handles the scale down/excess replicas case and the case where a rolling upgrade is needed.
	// If there are more ready DockerMachines than desired replicas, start to delete the excess DockerMachines such that
	// - DockerMachines with an outdated Kubernetes version or custom image are deleted first (i.e. the rolling upgrade).
	// - DockerMachines whose owner Machine contains the clusterv1.DeleteMachineAnnotation are deleted next (to support cluster autoscaler).
	// Note: we want to ensure that there are always enough ready DockerMachines before deleting anything or scaling down.

	// For each DockerMachine, fetch the owner Machine and copy the clusterv1.DeleteMachineAnnotation to the DockerMachine if it exists before sorting the DockerMachines.
	// This is done just before sorting to guarantee we have the latest copy of the Machine annotations.
	dockerMachinesWithAnnotation, err := r.propagateMachineDeleteAnnotation(ctx, dockerMachineMap)
	if err != nil {
		return err
	}

	// Sort DockerMachines with the clusterv1.DeleteMachineAnnotation to the front of each list.
	// If providers already have a sorting order for instance deletion, i.e. oldest first or newest first, the clusterv1.DeleteMachineAnnotation must take priority.
	// For example, if deleting by oldest, we expect the InfraMachines with clusterv1.DeleteMachineAnnotation to be deleted first followed by the oldest, and the second oldest, etc.
	orderedDockerMachines := orderByDeleteMachineAnnotation(dockerMachinesWithAnnotation)

	// Note: this includes DockerMachines that are out of date but still ready. This is to ensure we always have enough ready DockerMachines before deleting anything.
	totalReadyMachines := 0
	for i := range orderedDockerMachines {
		dockerMachine := orderedDockerMachines[i]
		// TODO (v1beta2): test for v1beta2 conditions
		if dockerMachine.Status.Ready || v1beta1conditions.IsTrue(&dockerMachine, clusterv1.ReadyV1Beta1Condition) {
			totalReadyMachines++
		}
	}

	outdatedMachines, readyMachines, err := r.getDeletionCandidates(ctx, orderedDockerMachines, externalMachineMap, machinePool, dockerMachinePool)
	if err != nil {
		return err
	}

	desiredReplicas := int(*machinePool.Spec.Replicas)
	overProvisionCount := totalReadyMachines - desiredReplicas

	// Loop through outdated DockerMachines first and decrement the overProvisionCount until it reaches 0.
	for _, dockerMachine := range outdatedMachines {
		if overProvisionCount > 0 {
			dockerMachine := dockerMachine
			log.V(2).Info("Deleting DockerMachine because it is outdated", "DockerMachine", klog.KObj(&dockerMachine))
			if err := r.deleteMachinePoolMachine(ctx, dockerMachine); err != nil {
				return err
			}

			overProvisionCount--
		}
	}

	// Then, loop through the ready DockerMachines first and decrement the overProvisionCount until it reaches 0.
	for _, dockerMachine := range readyMachines {
		if overProvisionCount > 0 {
			dockerMachine := dockerMachine
			log.V(2).Info("Deleting DockerMachine because it is an excess replica", "DockerMachine", klog.KObj(&dockerMachine))
			if err := r.deleteMachinePoolMachine(ctx, dockerMachine); err != nil {
				return err
			}

			overProvisionCount--
		}
	}

	return nil
}

// computeDesiredDockerMachine creates a DockerMachine to represent a Docker container in a DockerMachinePool.
// These DockerMachines have the clusterv1.ClusterNameLabel and clusterv1.MachinePoolNameLabel to support MachinePool Machines.
func computeDesiredDockerMachine(name string, cluster *clusterv1.Cluster, machinePool *clusterv1.MachinePool, dockerMachinePool *infraexpv1.DockerMachinePool, existingDockerMachine *infrav1.DockerMachine) *infrav1.DockerMachine {
	dockerMachine := &infrav1.DockerMachine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   dockerMachinePool.Namespace,
			Name:        name,
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
		},
		Spec: infrav1.DockerMachineSpec{
			CustomImage:   dockerMachinePool.Spec.Template.CustomImage,
			PreLoadImages: dockerMachinePool.Spec.Template.PreLoadImages,
			ExtraMounts:   dockerMachinePool.Spec.Template.ExtraMounts,
		},
	}

	if existingDockerMachine != nil {
		dockerMachine.SetUID(existingDockerMachine.UID)
		dockerMachine.SetOwnerReferences(existingDockerMachine.OwnerReferences)
	}

	// Note: Since the MachinePool controller has not created its owner Machine yet, we want to set the DockerMachinePool as the owner so it's not orphaned.
	dockerMachine.SetOwnerReferences(util.EnsureOwnerRef(dockerMachine.OwnerReferences, metav1.OwnerReference{
		APIVersion: infraexpv1.GroupVersion.String(),
		Kind:       "DockerMachinePool",
		Name:       dockerMachinePool.Name,
		UID:        dockerMachinePool.UID,
	}))
	dockerMachine.Labels[clusterv1.ClusterNameLabel] = cluster.Name
	dockerMachine.Labels[clusterv1.MachinePoolNameLabel] = format.MustFormatValue(machinePool.Name)

	return dockerMachine
}

// deleteMachinePoolMachine attempts to delete a DockerMachine and its associated owner Machine if it exists.
func (r *DockerMachinePoolReconciler) deleteMachinePoolMachine(ctx context.Context, dockerMachine infrav1.DockerMachine) error {
	log := ctrl.LoggerFrom(ctx)

	machine, err := util.GetOwnerMachine(ctx, r.Client, dockerMachine.ObjectMeta)
	if err != nil {
		return errors.Wrapf(err, "error getting owner Machine for DockerMachine %s/%s", dockerMachine.Namespace, dockerMachine.Name)
	}
	// util.GetOwnerMachine() returns a nil Machine without error if there is no Machine kind in the ownerRefs, so we must verify that machine is not nil.
	if machine == nil {
		log.V(2).Info("No owner Machine exists for DockerMachine", "dockerMachine", klog.KObj(&dockerMachine))

		// If the DockerMachine does not have an owner Machine, do not attempt to delete the DockerMachine as the MachinePool controller will create the
		// Machine and we want to let it catch up. If we are too hasty to delete, that introduces a race condition where the DockerMachine could be deleted
		// just as the Machine comes online.

		// In the case where the MachinePool is being deleted and the Machine will never come online, the DockerMachine will be deleted via its ownerRef to the
		// DockerMachinePool, so that is covered as well.

		return nil
	}

	log.Info("Deleting Machine for DockerMachine", "Machine", klog.KObj(machine), "DockerMachine", klog.KObj(&dockerMachine))

	if err := r.Client.Delete(ctx, machine); err != nil {
		return errors.Wrapf(err, "failed to delete Machine %s/%s", machine.Namespace, machine.Name)
	}

	return nil
}

// propagateMachineDeleteAnnotation returns the DockerMachines for a MachinePool and for each DockerMachine, it copies the owner
// Machine's delete annotation to each DockerMachine if it's present. This is done just in time to ensure that the annotations are
// up to date when we sort for DockerMachine deletion.
func (r *DockerMachinePoolReconciler) propagateMachineDeleteAnnotation(ctx context.Context, dockerMachineMap map[string]infrav1.DockerMachine) ([]infrav1.DockerMachine, error) {
	_ = ctrl.LoggerFrom(ctx)

	dockerMachines := []infrav1.DockerMachine{}
	for _, dockerMachine := range dockerMachineMap {
		machine, err := util.GetOwnerMachine(ctx, r.Client, dockerMachine.ObjectMeta)
		if err != nil {
			return nil, errors.Wrapf(err, "error getting owner Machine for DockerMachine %s/%s", dockerMachine.Namespace, dockerMachine.Name)
		}
		if machine != nil && machine.Annotations != nil {
			if dockerMachine.Annotations == nil {
				dockerMachine.Annotations = map[string]string{}
			}
			if _, hasDeleteAnnotation := machine.Annotations[clusterv1.DeleteMachineAnnotation]; hasDeleteAnnotation {
				dockerMachine.Annotations[clusterv1.DeleteMachineAnnotation] = machine.Annotations[clusterv1.DeleteMachineAnnotation]
			}
		}

		dockerMachines = append(dockerMachines, dockerMachine)
	}

	return dockerMachines, nil
}

// orderByDeleteMachineAnnotation will sort DockerMachines with the clusterv1.DeleteMachineAnnotation to the front of the list.
// It will preserve the existing order of the list otherwise so that it respects the existing delete priority otherwise.
func orderByDeleteMachineAnnotation(machines []infrav1.DockerMachine) []infrav1.DockerMachine {
	sort.SliceStable(machines, func(i, _ int) bool {
		_, iHasAnnotation := machines[i].Annotations[clusterv1.DeleteMachineAnnotation]

		return iHasAnnotation
	})

	return machines
}

// isMachineMatchingInfrastructureSpec returns true if the Docker container image matches the custom image in the DockerMachinePool spec.
func isMachineMatchingInfrastructureSpec(_ context.Context, machine *docker.Machine, machinePool *clusterv1.MachinePool, dockerMachinePool *infraexpv1.DockerMachinePool) bool {
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

// machinesMatchingInfrastructureSpec returns the  Docker containers matching the custom image in the DockerMachinePool spec.
func machinesMatchingInfrastructureSpec(ctx context.Context, machines []*docker.Machine, machinePool *clusterv1.MachinePool, dockerMachinePool *infraexpv1.DockerMachinePool) []*docker.Machine {
	var matchingMachines []*docker.Machine
	for _, machine := range machines {
		if isMachineMatchingInfrastructureSpec(ctx, machine, machinePool, dockerMachinePool) {
			matchingMachines = append(matchingMachines, machine)
		}
	}

	return matchingMachines
}

// getDeletionCandidates returns the DockerMachines for a MachinePool that do not match the infrastructure spec followed by any DockerMachines that are ready and up to date, i.e. matching the infrastructure spec.
func (r *DockerMachinePoolReconciler) getDeletionCandidates(ctx context.Context, dockerMachines []infrav1.DockerMachine, externalMachineSet map[string]*docker.Machine, machinePool *clusterv1.MachinePool, dockerMachinePool *infraexpv1.DockerMachinePool) (outdatedMachines []infrav1.DockerMachine, readyMatchingMachines []infrav1.DockerMachine, err error) {
	for i := range dockerMachines {
		dockerMachine := dockerMachines[i]
		externalMachine, ok := externalMachineSet[dockerMachine.Name]
		if !ok {
			// Note: Since we deleted any DockerMachines that do not have an associated Docker container earlier, we should never hit this case.
			return nil, nil, errors.Errorf("failed to find externalMachine for DockerMachine %s/%s", dockerMachine.Namespace, dockerMachine.Name)
		}

		// TODO (v1beta2): test for v1beta2 conditions
		if !isMachineMatchingInfrastructureSpec(ctx, externalMachine, machinePool, dockerMachinePool) {
			outdatedMachines = append(outdatedMachines, dockerMachine)
		} else if dockerMachine.Status.Ready || v1beta1conditions.IsTrue(&dockerMachine, clusterv1.ReadyV1Beta1Condition) {
			readyMatchingMachines = append(readyMatchingMachines, dockerMachine)
		}
	}

	return outdatedMachines, readyMatchingMachines, nil
}
