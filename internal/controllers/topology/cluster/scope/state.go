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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/controllers/machinedeployment/mdutil"
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

// RollingOut returns the list of the machine deployments
// that are rolling out.
func (mds MachineDeploymentsStateMap) RollingOut() []string {
	names := []string{}
	for _, md := range mds {
		if md.IsRollingOut() {
			names = append(names, md.Object.Name)
		}
	}
	return names
}

// IsAnyRollingOut returns true if at least one of the
// machine deployments is rolling out. False, otherwise.
func (mds MachineDeploymentsStateMap) IsAnyRollingOut() bool {
	return len(mds.RollingOut()) != 0
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

// IsRollingOut determines if the machine deployment is upgrading.
// A machine deployment is considered upgrading if:
// - if any of the replicas of the the machine deployment is not ready.
func (md *MachineDeploymentState) IsRollingOut() bool {
	return !mdutil.DeploymentComplete(md.Object, &md.Object.Status) || *md.Object.Spec.Replicas != md.Object.Status.ReadyReplicas
}
