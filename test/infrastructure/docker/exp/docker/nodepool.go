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

// Package docker implements docker functionality.
package docker

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	clusterv1exp "sigs.k8s.io/cluster-api/exp/api/v1alpha4"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/docker"
	infrav1exp "sigs.k8s.io/cluster-api/test/infrastructure/docker/exp/api/v1alpha4"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/container"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kind/pkg/cluster/constants"
)

const (
	dockerMachinePoolLabel = "docker.cluster.x-k8s.io/machine-pool"
)

// NodePool is a wrapper around a collection of like machines which are owned by a DockerMachinePool. A node pool
// provides a friendly way of managing (adding, deleting, reimaging) a set of docker machines. The node pool will also
// sync the docker machine pool status Instances field with the state of the docker machines.
type NodePool struct {
	client            client.Client
	cluster           *clusterv1.Cluster
	machinePool       *clusterv1exp.MachinePool
	dockerMachinePool *infrav1exp.DockerMachinePool
	labelFilters      map[string]string
	machines          []*docker.Machine
}

// NewNodePool creates a new node pool instances.
func NewNodePool(c client.Client, cluster *clusterv1.Cluster, mp *clusterv1exp.MachinePool, dmp *infrav1exp.DockerMachinePool) (*NodePool, error) {
	np := &NodePool{
		client:            c,
		cluster:           cluster,
		machinePool:       mp,
		dockerMachinePool: dmp,
		labelFilters:      map[string]string{dockerMachinePoolLabel: dmp.Name},
	}

	if err := np.refresh(); err != nil {
		return np, errors.Wrapf(err, "failed to refresh the node pool")
	}
	return np, nil
}

// ReconcileMachines will build enough machines to satisfy the machine pool / docker machine pool spec
// eventually delete all the machine in excess, and update the status for all the machines.
//
// NOTE: The goal for the current implementation is to verify MachinePool construct; accordingly,
// currently the nodepool supports only a recreate strategy for replacing old nodes with new ones
// (all existing machines are killed before new ones are created).
// TODO: consider if to support a Rollout strategy (a more progressive node replacement).
func (np *NodePool) ReconcileMachines(ctx context.Context) (ctrl.Result, error) {
	desiredReplicas := int(*np.machinePool.Spec.Replicas)

	// Delete all the machines in excess (outdated machines or machines exceeding desired replica count).
	machineDeleted := false
	totalNumberOfMachines := 0
	for _, machine := range np.machines {
		totalNumberOfMachines++
		if totalNumberOfMachines > desiredReplicas || !np.isMachineMatchingInfrastructureSpec(machine) {
			externalMachine, err := docker.NewMachine(np.cluster, machine.Name(), np.dockerMachinePool.Spec.Template.CustomImage, np.labelFilters)
			if err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "failed to create helper for managing the externalMachine named %s", machine.Name())
			}
			if err := externalMachine.Delete(ctx); err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "failed to delete machine %s", machine.Name())
			}
			machineDeleted = true
			totalNumberOfMachines-- // remove deleted machine from the count
		}
	}
	if machineDeleted {
		if err := np.refresh(); err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to refresh the node pool")
		}
	}

	// Add new machines if missing.
	machineAdded := false
	matchingMachineCount := len(np.machinesMatchingInfrastructureSpec())
	if matchingMachineCount < desiredReplicas {
		for i := 0; i < desiredReplicas-matchingMachineCount; i++ {
			if err := np.addMachine(ctx); err != nil {
				return ctrl.Result{}, errors.Wrap(err, "failed to create a new docker machine")
			}
			machineAdded = true
		}
	}
	if machineAdded {
		if err := np.refresh(); err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to refresh the node pool")
		}
	}

	// First remove instance status for machines no longer existing, then reconcile the existing machines.
	// NOTE: the status is the only source of truth for understanding if the machine is already bootstrapped, ready etc.
	// so we are preserving the existing status and using it as a bases for the next reconcile machine.
	instances := make([]infrav1exp.DockerMachinePoolInstanceStatus, 0, len(np.machines))
	for i := range np.dockerMachinePool.Status.Instances {
		instance := np.dockerMachinePool.Status.Instances[i]
		for j := range np.machines {
			if instance.InstanceName == np.machines[j].Name() {
				instances = append(instances, instance)
				break
			}
		}
	}
	np.dockerMachinePool.Status.Instances = instances

	result := ctrl.Result{}
	for i := range np.machines {
		machine := np.machines[i]
		if res, err := np.reconcileMachine(ctx, machine); err != nil || !res.IsZero() {
			if err != nil {
				return ctrl.Result{}, errors.Wrap(err, "failed to reconcile machine")
			}
			result = util.LowestNonZeroResult(result, res)
		}
	}
	return result, nil
}

// Delete will delete all of the machines in the node pool.
func (np *NodePool) Delete(ctx context.Context) error {
	for _, machine := range np.machines {
		externalMachine, err := docker.NewMachine(np.cluster, machine.Name(), np.dockerMachinePool.Spec.Template.CustomImage, np.labelFilters)
		if err != nil {
			return errors.Wrapf(err, "failed to create helper for managing the externalMachine named %s", machine.Name())
		}

		if err := externalMachine.Delete(ctx); err != nil {
			return errors.Wrapf(err, "failed to delete machine %s", machine.Name())
		}
	}

	return nil
}

func (np *NodePool) isMachineMatchingInfrastructureSpec(machine *docker.Machine) bool {
	return machine.ImageVersion() == container.SemverToOCIImageTag(*np.machinePool.Spec.Template.Spec.Version)
}

// machinesMatchingInfrastructureSpec returns all of the docker.Machines which match the machine pool / docker machine pool spec.
func (np *NodePool) machinesMatchingInfrastructureSpec() []*docker.Machine {
	var matchingMachines []*docker.Machine
	for _, machine := range np.machines {
		if np.isMachineMatchingInfrastructureSpec(machine) {
			matchingMachines = append(matchingMachines, machine)
		}
	}

	return matchingMachines
}

// addMachine will add a new machine to the node pool and update the docker machine pool status.
func (np *NodePool) addMachine(ctx context.Context) error {
	instanceName := fmt.Sprintf("worker-%s", util.RandomString(6))
	externalMachine, err := docker.NewMachine(np.cluster, instanceName, np.dockerMachinePool.Spec.Template.CustomImage, np.labelFilters)
	if err != nil {
		return errors.Wrapf(err, "failed to create helper for managing the externalMachine named %s", instanceName)
	}

	if err := externalMachine.Create(ctx, constants.WorkerNodeRoleValue, np.machinePool.Spec.Template.Spec.Version, np.dockerMachinePool.Spec.Template.ExtraMounts); err != nil {
		return errors.Wrapf(err, "failed to create docker machine with instance name %s", instanceName)
	}
	return nil
}

// refresh asks docker to list all the machines matching the node pool label and updates the cached list of node pool
// machines.
func (np *NodePool) refresh() error {
	machines, err := docker.ListMachinesByCluster(np.cluster, np.labelFilters)
	if err != nil {
		return errors.Wrapf(err, "failed to list all machines in the cluster")
	}

	np.machines = make([]*docker.Machine, 0, len(machines))
	for i := range machines {
		machine := machines[i]
		// makes sure no control plane machines gets selected by chance.
		if !machine.IsControlPlane() {
			np.machines = append(np.machines, machine)
		}
	}
	return nil
}

// reconcileMachine will build and provision a docker machine and update the docker machine pool status for that instance.
func (np *NodePool) reconcileMachine(ctx context.Context, machine *docker.Machine) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	var machineStatus infrav1exp.DockerMachinePoolInstanceStatus
	isFound := false
	for _, instanceStatus := range np.dockerMachinePool.Status.Instances {
		if instanceStatus.InstanceName == machine.Name() {
			machineStatus = instanceStatus
			isFound = true
		}
	}
	if !isFound {
		log.Info("Creating instance record", "instance", machine.Name())
		machineStatus = infrav1exp.DockerMachinePoolInstanceStatus{
			InstanceName: machine.Name(),
			Version:      np.machinePool.Spec.Template.Spec.Version,
		}
		np.dockerMachinePool.Status.Instances = append(np.dockerMachinePool.Status.Instances, machineStatus)
		// return to surface the new machine exists.
		return ctrl.Result{Requeue: true}, nil
	}

	defer func() {
		for i, instanceStatus := range np.dockerMachinePool.Status.Instances {
			if instanceStatus.InstanceName == machine.Name() {
				np.dockerMachinePool.Status.Instances[i] = machineStatus
			}
		}
	}()

	externalMachine, err := docker.NewMachine(np.cluster, machine.Name(), np.dockerMachinePool.Spec.Template.CustomImage, np.labelFilters)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to create helper for managing the externalMachine named %s", machine.Name())
	}

	// if the machine isn't bootstrapped, only then run bootstrap scripts
	if !machineStatus.Bootstrapped {
		log.Info("Bootstrapping instance", "instance", machine.Name())
		if err := externalMachine.PreloadLoadImages(ctx, np.dockerMachinePool.Spec.Template.PreLoadImages); err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to pre-load images into the docker machine with instance name %s", machine.Name())
		}

		bootstrapData, err := getBootstrapData(ctx, np.client, np.machinePool)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to get bootstrap data for instance named %s", machine.Name())
		}

		timeoutctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		defer cancel()
		// Run the bootstrap script. Simulates cloud-init.
		if err := externalMachine.ExecBootstrap(timeoutctx, bootstrapData); err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to exec DockerMachinePool instance bootstrap for instance named %s", machine.Name())
		}
		// Check for bootstrap success
		if err := externalMachine.CheckForBootstrapSuccess(timeoutctx); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to check for existence of bootstrap success file at /run/cluster-api/bootstrap-success.complete")
		}

		machineStatus.Bootstrapped = true
		// return to surface the machine has been bootstrapped.
		return ctrl.Result{Requeue: true}, nil
	}

	if machineStatus.Addresses == nil {
		log.Info("Fetching instance addresses", "instance", machine.Name())
		// set address in machine status
		machineAddress, err := externalMachine.Address(ctx)
		if err != nil {
			// Requeue if there is an error, as this is likely momentary load balancer
			// state changes during control plane provisioning.
			return ctrl.Result{Requeue: true}, nil // nolint:nilerr
		}

		machineStatus.Addresses = []clusterv1.MachineAddress{
			{
				Type:    clusterv1.MachineHostName,
				Address: externalMachine.ContainerName(),
			},
			{
				Type:    clusterv1.MachineInternalIP,
				Address: machineAddress,
			},
			{
				Type:    clusterv1.MachineExternalIP,
				Address: machineAddress,
			},
		}
	}

	if machineStatus.ProviderID == nil {
		log.Info("Fetching instance provider ID", "instance", machine.Name())
		// Usually a cloud provider will do this, but there is no docker-cloud provider.
		// Requeue if there is an error, as this is likely momentary load balancer
		// state changes during control plane provisioning.
		if err := externalMachine.SetNodeProviderID(ctx); err != nil {
			log.V(4).Info("transient error setting the provider id")
			return ctrl.Result{Requeue: true}, nil // nolint:nilerr
		}
		// Set ProviderID so the Cluster API Machine Controller can pull it
		providerID := externalMachine.ProviderID()
		machineStatus.ProviderID = &providerID
	}

	machineStatus.Ready = true
	return ctrl.Result{}, nil
}

// getBootstrapData fetches the bootstrap data for the machine pool.
func getBootstrapData(ctx context.Context, c client.Client, machinePool *clusterv1exp.MachinePool) (string, error) {
	if machinePool.Spec.Template.Spec.Bootstrap.DataSecretName == nil {
		return "", errors.New("error retrieving bootstrap data: linked MachinePool's bootstrap.dataSecretName is nil")
	}

	s := &corev1.Secret{}
	key := client.ObjectKey{Namespace: machinePool.GetNamespace(), Name: *machinePool.Spec.Template.Spec.Bootstrap.DataSecretName}
	if err := c.Get(ctx, key, s); err != nil {
		return "", errors.Wrapf(err, "failed to retrieve bootstrap data secret for DockerMachinePool instance %s/%s", machinePool.GetNamespace(), machinePool.GetName())
	}

	value, ok := s.Data["value"]
	if !ok {
		return "", errors.New("error retrieving bootstrap data: secret value key is missing")
	}

	return base64.StdEncoding.EncodeToString(value), nil
}
