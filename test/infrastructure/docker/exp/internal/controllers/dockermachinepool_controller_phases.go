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

	"github.com/blang/semver"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kind/pkg/cluster/constants"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1"
	infraexpv1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/docker"
	"sigs.k8s.io/cluster-api/test/infrastructure/kind"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/labels/format"
)

// CreateNewInstances creates a Docker container for each new replica needed for the MachinePool as well as a DockerMachine to represent it.
func (r *DockerMachinePoolReconciler) CreateNewInstances(ctx context.Context, cluster *clusterv1.Cluster, machinePool *expv1.MachinePool, dockerMachinePool *infraexpv1.DockerMachinePool) error {
	log := ctrl.LoggerFrom(ctx)

	labelFilters := map[string]string{dockerMachinePoolLabel: dockerMachinePool.Name}

	machines, err := docker.ListMachinesByCluster(ctx, cluster, labelFilters)
	if err != nil {
		return errors.Wrapf(err, "failed to list all machines in the cluster")
	}

	matchingMachineCount := len(machinesMatchingInfrastructureSpec(ctx, machines, machinePool, dockerMachinePool))
	numToCreate := int(*machinePool.Spec.Replicas) - matchingMachineCount
	for i := 0; i < numToCreate; i++ {
		log.V(2).Info("Creating a new Docker container for machinePool", "machinePool", machinePool.Name)
		createdMachine, err := createDockerContainer(ctx, cluster, machinePool, dockerMachinePool)
		if err != nil {
			return errors.Wrap(err, "failed to create a new docker machine")
		}

		log.V(2).Info("Creating a new DockerMachine for Docker container", "container", createdMachine.Name())
		// Note: we want the DockerMachine to have the same name as the container so that we can easily find it later.
		if err := r.createDockerMachine(ctx, createdMachine.Name(), cluster, machinePool, dockerMachinePool); err != nil {
			return errors.Wrap(err, "failed to create a new docker machine")
		}
	}

	return nil
}

// createDockerContainer creates a Docker container to serve as a replica for the MachinePool.
func createDockerContainer(ctx context.Context, cluster *clusterv1.Cluster, machinePool *expv1.MachinePool, dockerMachinePool *infraexpv1.DockerMachinePool) (*docker.Machine, error) {
	log := ctrl.LoggerFrom(ctx)
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

	log.Info("Creating container for machinePool", "name", name, "machinePool", machinePool.Name)
	if err := externalMachine.Create(ctx, dockerMachinePool.Spec.Template.CustomImage, constants.WorkerNodeRoleValue, machinePool.Spec.Template.Spec.Version, labels, dockerMachinePool.Spec.Template.ExtraMounts); err != nil {
		return nil, errors.Wrapf(err, "failed to create docker machine with name %s", name)
	}
	return externalMachine, nil
}

// createMissingDockerMachines creates a DockerMachine for each replica that does not already have one associated.
// The DockerMachines are needed to bootstrap and reconcile the replicas.
func (r *DockerMachinePoolReconciler) createMissingDockerMachines(ctx context.Context, cluster *clusterv1.Cluster, machinePool *expv1.MachinePool, dockerMachinePool *infraexpv1.DockerMachinePool) error {
	log := ctrl.LoggerFrom(ctx)

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
		} else {
			log.Info("DockerMachine already exists, nothing to do", "name", machine.Name())
		}
	}

	return nil
}

// createDockerMachine creates a DockerMachine to represent a Docker container in a DockerMachinePool.
// These DockerMachines have the clusterv1.ClusterNameLabel and clusterv1.MachinePoolNameLabel to support MachinePool Machines.
func (r *DockerMachinePoolReconciler) createDockerMachine(ctx context.Context, name string, cluster *clusterv1.Cluster, machinePool *expv1.MachinePool, dockerMachinePool *infraexpv1.DockerMachinePool) error {
	log := ctrl.LoggerFrom(ctx)

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

	log.V(2).Info("Creating DockerMachine", "dockerMachine", dockerMachine.Name)

	if err := r.Client.Create(ctx, dockerMachine); err != nil {
		return errors.Wrap(err, "failed to create dockerMachine")
	}

	return nil
}

// DeleteExtraDockerMachines deletes the DockerMachines needed to reach the MachinePool's desired replica count in a scale down as well as any referencing Docker containers that do not match the infrastructure spec.
func (r *DockerMachinePoolReconciler) DeleteExtraDockerMachines(ctx context.Context, cluster *clusterv1.Cluster, machinePool *expv1.MachinePool, dockerMachinePool *infraexpv1.DockerMachinePool) error {
	log := ctrl.LoggerFrom(ctx)
	log.V(2).Info("Deleting extra machines if needed", "dockerMachinePool", dockerMachinePool.Name, "namespace", dockerMachinePool.Namespace)
	dockerMachineList, err := getDockerMachines(ctx, r.Client, *cluster, *machinePool, *dockerMachinePool)
	if err != nil {
		return err
	}

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

// deleteMachinePoolMachine attempts to delete a DockerMachine and its associated parent Machine if it exists.
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

// getDockerMachinesToDelete returns a list of DockerMachines representing excess replicas and Docker containers that do not match the infrastructure spec.
// In the case of excess replicas, DockerMachines with the clusterv1.DeleteMachineAnnotation are prioritized to ensure they are deleted first.
func getDockerMachinesToDelete(ctx context.Context, dockerMachines []infrav1.DockerMachine, cluster *clusterv1.Cluster, machinePool *expv1.MachinePool, dockerMachinePool *infraexpv1.DockerMachinePool) ([]infrav1.DockerMachine, error) {
	log := ctrl.LoggerFrom(ctx)

	dockerMachinesToDelete := []infrav1.DockerMachine{}
	labelFilters := map[string]string{dockerMachinePoolLabel: dockerMachinePool.Name}

	// Sort priority delete to end of list
	sort.Slice(dockerMachines, func(i, j int) bool {
		_, iHasAnnotation := dockerMachines[i].Annotations[clusterv1.DeleteMachineAnnotation]
		_, jHasAnnotation := dockerMachines[j].Annotations[clusterv1.DeleteMachineAnnotation]

		if iHasAnnotation && jHasAnnotation {
			return dockerMachines[i].Name < dockerMachines[j].Name
		}

		return jHasAnnotation
	})

	desiredReplicas := int(*machinePool.Spec.Replicas)
	totalNumMachines := 0
	for _, dockerMachine := range dockerMachines {
		totalNumMachines++
		if totalNumMachines > desiredReplicas {
			dockerMachinesToDelete = append(dockerMachinesToDelete, dockerMachine)
			log.V(2).Info("Selecting DockerMachine for deletion because it is an excess replica", "dockerMachine", dockerMachine.Name, "namespace", dockerMachine.Namespace)
			totalNumMachines--
		} else {
			externalMachine, err := docker.NewMachine(ctx, cluster, dockerMachine.Name, labelFilters)
			if err != nil {
				return nil, err
			}
			if !isMachineMatchingInfrastructureSpec(ctx, externalMachine, machinePool, dockerMachinePool) {
				log.V(2).Info("Selecting DockerMachine for deletion because it does not match infrastructure spec", "dockerMachine", dockerMachine.Name)
				dockerMachinesToDelete = append(dockerMachinesToDelete, dockerMachine)
				totalNumMachines--
			} else {
				log.V(2).Info("Keeping DockerMachine, nothing to do", "dockerMachine", dockerMachine.Name, "namespace", dockerMachine.Namespace)
			}
		}
	}

	return dockerMachinesToDelete, nil
}

// isMachineMatchingInfrastructureSpec returns true if the Docker container image matches the custom image in the DockerMachinePool spec.
func isMachineMatchingInfrastructureSpec(_ context.Context, machine *docker.Machine, machinePool *expv1.MachinePool, dockerMachinePool *infraexpv1.DockerMachinePool) bool {
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
func machinesMatchingInfrastructureSpec(ctx context.Context, machines []*docker.Machine, machinePool *expv1.MachinePool, dockerMachinePool *infraexpv1.DockerMachinePool) []*docker.Machine {
	var matchingMachines []*docker.Machine
	for _, machine := range machines {
		if isMachineMatchingInfrastructureSpec(ctx, machine, machinePool, dockerMachinePool) {
			matchingMachines = append(matchingMachines, machine)
		}
	}

	return matchingMachines
}
