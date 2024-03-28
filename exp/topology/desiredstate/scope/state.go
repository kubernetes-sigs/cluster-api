/*
Copyright 2021 The Kubernetes Authors.

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

package scope

import (
	"context"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/topology/check"
)

// ClusterState holds all the objects representing the state of a managed Cluster topology.
// NOTE: please note that we are going to deal with two different type state, the current state as read from the API server,
// and the desired state resulting from processing the ClusterBlueprint.
type ClusterState struct {
	// Cluster holds the Cluster object.
	Cluster *clusterv1.Cluster

	// InfrastructureCluster holds the infrastructure cluster object referenced by the Cluster.
	InfrastructureCluster *unstructured.Unstructured

	// ControlPlane holds the controlplane object referenced by the Cluster.
	ControlPlane *ControlPlaneState

	// MachineDeployments holds the machine deployments in the Cluster.
	MachineDeployments MachineDeploymentsStateMap

	// MachinePools holds the MachinePools in the Cluster.
	MachinePools MachinePoolsStateMap
}

// ControlPlaneState holds all the objects representing the state of a managed control plane.
type ControlPlaneState struct {
	// Object holds the ControlPlane object.
	Object *unstructured.Unstructured

	// InfrastructureMachineTemplate holds the infrastructure template referenced by the ControlPlane object.
	InfrastructureMachineTemplate *unstructured.Unstructured

	// MachineHealthCheckClass holds the MachineHealthCheck for this ControlPlane.
	// +optional
	MachineHealthCheck *clusterv1.MachineHealthCheck
}

// MachineDeploymentsStateMap holds a collection of MachineDeployment states.
type MachineDeploymentsStateMap map[string]*MachineDeploymentState

// Upgrading returns the list of the machine deployments
// that are upgrading.
func (mds MachineDeploymentsStateMap) Upgrading(ctx context.Context, c client.Reader) ([]string, error) {
	names := []string{}
	for _, md := range mds {
		upgrading, err := md.IsUpgrading(ctx, c)
		if err != nil {
			return nil, errors.Wrap(err, "failed to list upgrading MachineDeployments")
		}
		if upgrading {
			names = append(names, md.Object.Name)
		}
	}
	return names, nil
}

// MachineDeploymentState holds all the objects representing the state of a managed deployment.
type MachineDeploymentState struct {
	// Object holds the MachineDeployment object.
	Object *clusterv1.MachineDeployment

	// BootstrapTemplate holds the bootstrap template referenced by the MachineDeployment object.
	BootstrapTemplate *unstructured.Unstructured

	// InfrastructureMachineTemplate holds the infrastructure machine template referenced by the MachineDeployment object.
	InfrastructureMachineTemplate *unstructured.Unstructured

	// MachineHealthCheck holds a MachineHealthCheck linked to the MachineDeployment object.
	// +optional
	MachineHealthCheck *clusterv1.MachineHealthCheck
}

// IsUpgrading determines if the MachineDeployment is upgrading.
// A machine deployment is considered upgrading if at least one of the Machines of this
// MachineDeployment has a different version.
func (md *MachineDeploymentState) IsUpgrading(ctx context.Context, c client.Reader) (bool, error) {
	return check.IsMachineDeploymentUpgrading(ctx, c, md.Object)
}

// MachinePoolsStateMap holds a collection of MachinePool states.
type MachinePoolsStateMap map[string]*MachinePoolState

// Upgrading returns the list of the machine pools
// that are upgrading.
func (mps MachinePoolsStateMap) Upgrading(ctx context.Context, c client.Reader) ([]string, error) {
	names := []string{}
	for _, mp := range mps {
		upgrading, err := mp.IsUpgrading(ctx, c)
		if err != nil {
			return nil, errors.Wrap(err, "failed to list upgrading MachinePools")
		}
		if upgrading {
			names = append(names, mp.Object.Name)
		}
	}
	return names, nil
}

// MachinePoolState holds all the objects representing the state of a managed pool.
type MachinePoolState struct {
	// Object holds the MachinePool object.
	Object *expv1.MachinePool

	// BootstrapObject holds the MachinePool bootstrap object.
	BootstrapObject *unstructured.Unstructured

	// InfrastructureMachinePoolObject holds the infrastructure machine template referenced by the MachinePool object.
	InfrastructureMachinePoolObject *unstructured.Unstructured
}

// IsUpgrading determines if the MachinePool is upgrading.
// A machine pool is considered upgrading if at least one of the Machines of this
// MachinePool has a different version.
func (mp *MachinePoolState) IsUpgrading(ctx context.Context, c client.Reader) (bool, error) {
	return check.IsMachinePoolUpgrading(ctx, c, mp.Object)
}
