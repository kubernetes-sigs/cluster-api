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

package docker

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	clusterv1exp "sigs.k8s.io/cluster-api/exp/api/v1alpha3"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/docker"
	infrav1exp "sigs.k8s.io/cluster-api/test/infrastructure/docker/exp/api/v1alpha3"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kind/pkg/cluster/constants"
)

const (
	dockerMachinePoolLabel = "docker.cluster.x-k8s.io/machine-pool"
)

// TransientError represents an error from docker provisioning which should be retried
type TransientError struct {
	InstanceName string
	Reason       string
}

func (e *TransientError) Error() string {
	return fmt.Sprintf("container addresses for instance %s due to %s", e.InstanceName, e.Reason)
}

func (e *TransientError) Is(target error) bool {
	_, ok := target.(*TransientError)
	return ok
}

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
	logger            logr.Logger
}

// NewNodePool creates a new node pool instances
func NewNodePool(kClient client.Client, cluster *clusterv1.Cluster, mp *clusterv1exp.MachinePool, dmp *infrav1exp.DockerMachinePool, log logr.Logger) (*NodePool, error) {
	np := &NodePool{
		client:            kClient,
		cluster:           cluster,
		machinePool:       mp,
		dockerMachinePool: dmp,
		labelFilters:      map[string]string{dockerMachinePoolLabel: dmp.Name},
		logger:            log.WithValues("node-pool", dmp.Name),
	}

	if err := np.refresh(); err != nil {
		return np, errors.Wrapf(err, "failed to refresh the node pool")
	}
	return np, nil
}

// ReconcileMachines will build enough machines to satisfy the machine pool / docker machine pool spec and update the
// docker machine pool status
func (np *NodePool) ReconcileMachines(ctx context.Context) error {
	matchingMachineCount := int32(len(np.machinesMatchingInfrastructureSpec()))
	if matchingMachineCount < *np.machinePool.Spec.Replicas {
		for i := int32(0); i < *np.machinePool.Spec.Replicas-matchingMachineCount; i++ {
			if err := np.addMachine(ctx); err != nil {
				return errors.Wrap(err, "failed to create a new docker machine")
			}
		}
	}

	for _, machine := range np.machinesMatchingInfrastructureSpec() {
		if err := np.reconcileMachine(ctx, machine); err != nil {
			if errors.Is(err, &TransientError{}) {
				return err
			}
			return errors.Wrap(err, "failed to reconcile machine")
		}
	}

	return np.refresh()
}

// DeleteExtraMachines will delete all of the machines outside of the machine pool / docker machine pool spec and update
// the docker machine pool status.
func (np *NodePool) DeleteExtraMachines(ctx context.Context) error {
	outOfSpecMachineNames := map[string]interface{}{}
	for _, outOfSpecMachine := range np.machinesNotMatchingInfrastructureSpec() {
		externalMachine, err := docker.NewMachine(np.cluster.Name, np.cluster.Annotations, outOfSpecMachine.Name(), np.dockerMachinePool.Spec.Template.CustomImage, np.labelFilters, np.logger)
		if err != nil {
			return errors.Wrapf(err, "failed to create helper for managing the externalMachine named %s", outOfSpecMachine.Name())
		}

		if err := externalMachine.Delete(ctx); err != nil {
			return errors.Wrapf(err, "failed to delete machine %s", outOfSpecMachine.Name())
		}

		outOfSpecMachineNames[outOfSpecMachine.Name()] = nil
	}

	var stats []*infrav1exp.DockerMachinePoolInstanceStatus
	for _, machineStatus := range np.dockerMachinePool.Status.Instances {
		if _, ok := outOfSpecMachineNames[machineStatus.InstanceName]; !ok {
			stats = append(stats, machineStatus)
		}
	}

	for _, overprovisioned := range stats[*np.machinePool.Spec.Replicas:] {
		externalMachine, err := docker.NewMachine(np.cluster.Name, np.cluster.Annotations, overprovisioned.InstanceName, np.dockerMachinePool.Spec.Template.CustomImage, np.labelFilters, np.logger)
		if err != nil {
			return errors.Wrapf(err, "failed to create helper for managing the externalMachine named %s", overprovisioned.InstanceName)
		}

		if err := externalMachine.Delete(ctx); err != nil {
			return errors.Wrapf(err, "failed to delete machine %s", overprovisioned.InstanceName)
		}
	}

	np.dockerMachinePool.Status.Instances = stats[:*np.machinePool.Spec.Replicas]
	return np.refresh()
}

// Delete will delete all of the machines in the node pool
func (np *NodePool) Delete(ctx context.Context) error {
	for _, machine := range np.machines {
		externalMachine, err := docker.NewMachine(np.cluster.Name, np.cluster.Annotations, machine.Name(), np.dockerMachinePool.Spec.Template.CustomImage, np.labelFilters, np.logger)
		if err != nil {
			return errors.Wrapf(err, "failed to create helper for managing the externalMachine named %s", machine.Name())
		}

		if err := externalMachine.Delete(ctx); err != nil {
			return errors.Wrapf(err, "failed to delete machine %s", machine.Name())
		}
	}

	return nil
}

// machinesMatchingInfrastructureSpec returns all of the docker.Machines which match the machine pool / docker machine pool spec
func (np *NodePool) machinesMatchingInfrastructureSpec() []*docker.Machine {
	var matchingMachines []*docker.Machine
	for _, machine := range np.machines {
		if !machine.IsControlPlane() && machine.ImageVersion() == *np.machinePool.Spec.Template.Spec.Version {
			matchingMachines = append(matchingMachines, machine)
		}
	}

	return matchingMachines
}

// machinesNotMatchingInfrastructureSpec returns all of the machines which do not match the machine pool / docker machine pool spec
func (np *NodePool) machinesNotMatchingInfrastructureSpec() []*docker.Machine {
	var matchingMachines []*docker.Machine
	for _, machine := range np.machines {
		if !machine.IsControlPlane() && machine.ImageVersion() != *np.machinePool.Spec.Template.Spec.Version {
			matchingMachines = append(matchingMachines, machine)
		}
	}

	return matchingMachines
}

// addMachine will add a new machine to the node pool and update the docker machine pool status
func (np *NodePool) addMachine(ctx context.Context) error {
	instanceName := fmt.Sprintf("worker-%s", util.RandomString(6))
	externalMachine, err := docker.NewMachine(np.cluster.Name, np.cluster.Annotations, instanceName, np.dockerMachinePool.Spec.Template.CustomImage, np.labelFilters, np.logger)
	if err != nil {
		return errors.Wrapf(err, "failed to create helper for managing the externalMachine named %s", instanceName)
	}

	if err := externalMachine.Create(ctx, constants.WorkerNodeRoleValue, np.machinePool.Spec.Template.Spec.Version, np.dockerMachinePool.Spec.Template.ExtraMounts); err != nil {
		return errors.Wrapf(err, "failed to create docker machine with instance name %s", instanceName)
	}

	np.dockerMachinePool.Status.Instances = append(np.dockerMachinePool.Status.Instances, &infrav1exp.DockerMachinePoolInstanceStatus{
		InstanceName: instanceName,
		Version:      np.machinePool.Spec.Template.Spec.Version,
	})

	return np.refresh()
}

// refresh asks docker to list all the machines matching the node pool label and updates the cached list of node pool
// machines
func (np *NodePool) refresh() error {
	machines, err := docker.ListMachinesByCluster(np.cluster.Name, np.labelFilters, np.logger)
	if err != nil {
		return errors.Wrapf(err, "failed to list all machines in the cluster")
	}

	np.machines = machines
	return nil
}

// reconcileMachine will build and provision a docker machine and update the docker machine pool status for that instance
func (np *NodePool) reconcileMachine(ctx context.Context, machine *docker.Machine) error {
	machineStatus := getInstanceStatusByMachineName(np.dockerMachinePool, machine.Name())
	if machineStatus == nil {
		machineStatus = &infrav1exp.DockerMachinePoolInstanceStatus{
			InstanceName: machine.Name(),
			Version:      np.machinePool.Spec.Template.Spec.Version,
		}
		np.dockerMachinePool.Status.Instances = append(np.dockerMachinePool.Status.Instances, machineStatus)
	}

	externalMachine, err := docker.NewMachine(np.cluster.Name, np.cluster.Annotations, machine.Name(), np.dockerMachinePool.Spec.Template.CustomImage, np.labelFilters, np.logger)
	if err != nil {
		return errors.Wrapf(err, "failed to create helper for managing the externalMachine named %s", machine.Name())
	}

	// if the machine isn't bootstrapped, only then run bootstrap scripts
	if !machineStatus.Bootstrapped {
		if err := externalMachine.PreloadLoadImages(ctx, np.dockerMachinePool.Spec.Template.PreLoadImages); err != nil {
			return errors.Wrapf(err, "failed to pre-load images into the docker machine with instance name %s", machine.Name())
		}

		bootstrapData, err := getBootstrapData(ctx, np.client, np.machinePool)
		if err != nil {
			return errors.Wrapf(err, "failed to get bootstrap data for instance named %s", machine.Name())
		}

		timeoutctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		defer cancel()
		// Run the bootstrap script. Simulates cloud-init.
		if err := externalMachine.ExecBootstrap(timeoutctx, bootstrapData); err != nil {
			return errors.Wrapf(err, "failed to exec DockerMachinePool instance bootstrap for instance named %s", machine.Name())
		}
		machineStatus.Bootstrapped = true
	}

	if machineStatus.Addresses == nil {
		// set address in machine status
		machineAddress, err := externalMachine.Address(ctx)
		if err != nil {
			return &TransientError{
				InstanceName: machine.Name(),
				Reason:       "failed to fetch addresses for container",
			}
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
		// Usually a cloud provider will do this, but there is no docker-cloud provider.
		// Requeue if there is an error, as this is likely momentary load balancer
		// state changes during control plane provisioning.
		if err := externalMachine.SetNodeProviderID(ctx); err != nil {
			np.logger.V(4).Info("transient error setting the provider id")
			return &TransientError{
				InstanceName: machine.Name(),
				Reason:       "failed to patch the Kubernetes node with the machine providerID",
			}
		}
		// Set ProviderID so the Cluster API Machine Controller can pull it
		providerID := externalMachine.ProviderID()
		machineStatus.ProviderID = &providerID
	}

	machineStatus.Ready = true
	return nil
}

// getBootstrapData fetches the bootstrap data for the machine pool
func getBootstrapData(ctx context.Context, kClient client.Client, machinePool *clusterv1exp.MachinePool) (string, error) {
	if machinePool.Spec.Template.Spec.Bootstrap.DataSecretName == nil {
		return "", errors.New("error retrieving bootstrap data: linked MachinePool's bootstrap.dataSecretName is nil")
	}

	s := &corev1.Secret{}
	key := client.ObjectKey{Namespace: machinePool.GetNamespace(), Name: *machinePool.Spec.Template.Spec.Bootstrap.DataSecretName}
	if err := kClient.Get(ctx, key, s); err != nil {
		return "", errors.Wrapf(err, "failed to retrieve bootstrap data secret for DockerMachinePool instance %s/%s", machinePool.GetNamespace(), machinePool.GetName())
	}

	value, ok := s.Data["value"]
	if !ok {
		return "", errors.New("error retrieving bootstrap data: secret value key is missing")
	}

	return base64.StdEncoding.EncodeToString(value), nil
}

// getInstanceStatusByMachineName returns the instance status for a given machine by name or nil if it doesn't exist
func getInstanceStatusByMachineName(dockerMachinePool *infrav1exp.DockerMachinePool, machineName string) *infrav1exp.DockerMachinePoolInstanceStatus {
	for _, machine := range dockerMachinePool.Status.Instances {
		if machine.InstanceName == machineName {
			return machine
		}
	}

	return nil
}
