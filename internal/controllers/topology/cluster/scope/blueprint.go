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
)

// ClusterBlueprint holds all the objects required for computing the desired state of a managed Cluster topology,
// including the ClusterClass and all the referenced templates.
type ClusterBlueprint struct {
	// Topology holds the topology info from Cluster.Spec.
	Topology *clusterv1.Topology

	// ClusterClass holds the ClusterClass object referenced from Cluster.Spec.Topology.
	ClusterClass *clusterv1.ClusterClass

	// InfrastructureClusterTemplate holds the InfrastructureClusterTemplate referenced from ClusterClass.
	InfrastructureClusterTemplate *unstructured.Unstructured

	// ControlPlane holds the ControlPlaneBlueprint derived from ClusterClass.
	ControlPlane *ControlPlaneBlueprint

	// MachineDeployments holds the MachineDeploymentBlueprints derived from ClusterClass.
	MachineDeployments map[string]*MachineDeploymentBlueprint
}

// ControlPlaneBlueprint holds the templates required for computing the desired state of a managed control plane.
type ControlPlaneBlueprint struct {
	// Template holds the control plane template referenced from ClusterClass.
	Template *unstructured.Unstructured

	// InfrastructureMachineTemplate holds the infrastructure machine template for the control plane, if defined in the ClusterClass.
	InfrastructureMachineTemplate *unstructured.Unstructured

	// MachineHealthCheck holds the MachineHealthCheckClass for this ControlPlane.
	// +optional
	MachineHealthCheck *clusterv1.MachineHealthCheckClass
}

// MachineDeploymentBlueprint holds the templates required for computing the desired state of a managed MachineDeployment;
// it also holds a copy of the MachineDeployment metadata from Cluster.Topology, thus providing all the required info
// in a single place.
type MachineDeploymentBlueprint struct {
	// Metadata holds the metadata for a MachineDeployment.
	// NOTE: This is a convenience copy of the metadata field from Cluster.Spec.Topology.Workers.MachineDeployments[x].
	Metadata clusterv1.ObjectMeta

	// BootstrapTemplate holds the bootstrap template for a MachineDeployment referenced from ClusterClass.
	BootstrapTemplate *unstructured.Unstructured

	// InfrastructureMachineTemplate holds the infrastructure machine template for a MachineDeployment referenced from ClusterClass.
	InfrastructureMachineTemplate *unstructured.Unstructured

	// MachineHealthCheck holds the MachineHealthCheckClass for this MachineDeployment.
	// +optional
	MachineHealthCheck *clusterv1.MachineHealthCheckClass
}

// HasControlPlaneInfrastructureMachine checks whether the clusterClass mandates the controlPlane has infrastructureMachines.
func (b *ClusterBlueprint) HasControlPlaneInfrastructureMachine() bool {
	return b.ClusterClass.Spec.ControlPlane.MachineInfrastructure != nil && b.ClusterClass.Spec.ControlPlane.MachineInfrastructure.Ref != nil
}

// HasControlPlaneMachineHealthCheck returns true if the ControlPlaneClass has both MachineInfrastructure and a MachineHealthCheck defined.
func (b *ClusterBlueprint) HasControlPlaneMachineHealthCheck() bool {
	return b.HasControlPlaneInfrastructureMachine() && b.ClusterClass.Spec.ControlPlane.MachineHealthCheck != nil
}

// HasMachineDeployments checks whether the topology has MachineDeployments.
func (b *ClusterBlueprint) HasMachineDeployments() bool {
	return b.Topology.Workers != nil && len(b.Topology.Workers.MachineDeployments) > 0
}
