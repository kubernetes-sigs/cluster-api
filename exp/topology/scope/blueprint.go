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

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// ClusterBlueprint holds all the objects required for computing the desired state of a managed Cluster topology,
// including the ClusterClass and all the referenced templates.
type ClusterBlueprint struct {
	// Topology holds the topology info from Cluster.Spec.
	Topology clusterv1.Topology

	// ClusterClass holds the ClusterClass object referenced from Cluster.Spec.Topology.
	ClusterClass *clusterv1.ClusterClass

	// InfrastructureClusterTemplate holds the InfrastructureClusterTemplate referenced from ClusterClass.
	InfrastructureClusterTemplate *unstructured.Unstructured

	// ControlPlane holds the ControlPlaneBlueprint derived from ClusterClass.
	ControlPlane *ControlPlaneBlueprint

	// MachineDeployments holds the MachineDeploymentBlueprints derived from ClusterClass.
	MachineDeployments map[string]*MachineDeploymentBlueprint

	// MachinePools holds the MachinePoolBlueprints derived from ClusterClass.
	MachinePools map[string]*MachinePoolBlueprint
}

// ControlPlaneBlueprint holds the templates required for computing the desired state of a managed control plane.
type ControlPlaneBlueprint struct {
	// Template holds the control plane template referenced from ClusterClass.
	Template *unstructured.Unstructured

	// InfrastructureMachineTemplate holds the infrastructure machine template for the control plane, if defined in the ClusterClass.
	InfrastructureMachineTemplate *unstructured.Unstructured

	// HealthCheck holds the MachineHealthCheckClass for this ControlPlane.
	// +optional
	HealthCheck clusterv1.ControlPlaneClassHealthCheck
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

	// HealthCheck holds the MachineHealthCheckClass for this MachineDeployment.
	// +optional
	HealthCheck clusterv1.MachineDeploymentClassHealthCheck
}

// MachinePoolBlueprint holds the templates required for computing the desired state of a managed MachinePool;
// it also holds a copy of the MachinePool metadata from Cluster.Topology, thus providing all the required info
// in a single place.
type MachinePoolBlueprint struct {
	// Metadata holds the metadata for a MachinePool.
	// NOTE: This is a convenience copy of the metadata field from Cluster.Spec.Topology.Workers.MachinePools[x].
	Metadata clusterv1.ObjectMeta

	// BootstrapTemplate holds the bootstrap template for a MachinePool referenced from ClusterClass.
	BootstrapTemplate *unstructured.Unstructured

	// InfrastructureMachinePoolTemplate holds the infrastructure machine pool template for a MachinePool referenced from ClusterClass.
	InfrastructureMachinePoolTemplate *unstructured.Unstructured
}

// HasControlPlaneInfrastructureMachine checks whether the clusterClass mandates the controlPlane has infrastructureMachines.
func (b *ClusterBlueprint) HasControlPlaneInfrastructureMachine() bool {
	return b.ClusterClass.Spec.ControlPlane.MachineInfrastructure.TemplateRef.IsDefined()
}

// IsControlPlaneMachineHealthCheckEnabled returns true if a MachineHealthCheck should be created for the control plane.
// Returns false otherwise.
func (b *ClusterBlueprint) IsControlPlaneMachineHealthCheckEnabled() bool {
	if !b.HasControlPlaneInfrastructureMachine() {
		return false
	}
	// If no MachineHealthCheck is defined in the ClusterClass or in the Cluster Topology then return false.
	if !b.ClusterClass.Spec.ControlPlane.HealthCheck.IsDefined() && !b.Topology.ControlPlane.HealthCheck.IsDefined() {
		return false
	}
	// If `enable` is not set then consider it as true. A MachineHealthCheck will be created from either ClusterClass or Cluster Topology.
	if b.Topology.ControlPlane.HealthCheck.Enabled == nil {
		return true
	}
	// If `enable` is explicitly set, use the value.
	return *b.Topology.ControlPlane.HealthCheck.Enabled
}

// ControlPlaneMachineHealthCheckClass returns the MachineHealthCheckClass that should be used to create the MachineHealthCheck object.
func (b *ClusterBlueprint) ControlPlaneMachineHealthCheckClass() (clusterv1.MachineHealthCheckChecks, clusterv1.MachineHealthCheckRemediation) {
	if b.Topology.ControlPlane.HealthCheck.IsDefined() {
		return clusterv1.MachineHealthCheckChecks{
				NodeStartupTimeoutSeconds: b.Topology.ControlPlane.HealthCheck.Checks.NodeStartupTimeoutSeconds,
				UnhealthyNodeConditions:   b.Topology.ControlPlane.HealthCheck.Checks.UnhealthyNodeConditions,
			}, clusterv1.MachineHealthCheckRemediation{
				TriggerIf: clusterv1.MachineHealthCheckRemediationTriggerIf{
					UnhealthyLessThanOrEqualTo: b.Topology.ControlPlane.HealthCheck.Remediation.TriggerIf.UnhealthyLessThanOrEqualTo,
					UnhealthyInRange:           b.Topology.ControlPlane.HealthCheck.Remediation.TriggerIf.UnhealthyInRange,
				},
				TemplateRef: b.Topology.ControlPlane.HealthCheck.Remediation.TemplateRef,
			}
	}

	return clusterv1.MachineHealthCheckChecks{
			NodeStartupTimeoutSeconds: b.ControlPlane.HealthCheck.Checks.NodeStartupTimeoutSeconds,
			UnhealthyNodeConditions:   b.ControlPlane.HealthCheck.Checks.UnhealthyNodeConditions,
		}, clusterv1.MachineHealthCheckRemediation{
			TriggerIf: clusterv1.MachineHealthCheckRemediationTriggerIf{
				UnhealthyLessThanOrEqualTo: b.ControlPlane.HealthCheck.Remediation.TriggerIf.UnhealthyLessThanOrEqualTo,
				UnhealthyInRange:           b.ControlPlane.HealthCheck.Remediation.TriggerIf.UnhealthyInRange,
			},
			TemplateRef: b.ControlPlane.HealthCheck.Remediation.TemplateRef,
		}
}

// HasControlPlaneMachineHealthCheck returns true if the ControlPlaneClass has both MachineInfrastructure and a MachineHealthCheck defined.
func (b *ClusterBlueprint) HasControlPlaneMachineHealthCheck() bool {
	return b.HasControlPlaneInfrastructureMachine() && b.ClusterClass.Spec.ControlPlane.HealthCheck.IsDefined()
}

// IsMachineDeploymentMachineHealthCheckEnabled returns true if a MachineHealthCheck should be created for the MachineDeployment.
// Returns false otherwise.
func (b *ClusterBlueprint) IsMachineDeploymentMachineHealthCheckEnabled(md *clusterv1.MachineDeploymentTopology) bool {
	// If no MachineHealthCheck is defined in the ClusterClass or in the Cluster Topology then return false.
	if !b.MachineDeployments[md.Class].HealthCheck.IsDefined() && !md.HealthCheck.IsDefined() {
		return false
	}
	// If `enable` is not set then consider it as true. A MachineHealthCheck will be created from either ClusterClass or Cluster Topology.
	if md.HealthCheck.Enabled == nil {
		return true
	}
	// If `enable` is explicitly set, use the value.
	return *md.HealthCheck.Enabled
}

// MachineDeploymentMachineHealthCheckClass return the MachineHealthCheckClass that should be used to create the MachineHealthCheck object.
func (b *ClusterBlueprint) MachineDeploymentMachineHealthCheckClass(md *clusterv1.MachineDeploymentTopology) (clusterv1.MachineHealthCheckChecks, clusterv1.MachineHealthCheckRemediation) {
	if md.HealthCheck.IsDefined() {
		return clusterv1.MachineHealthCheckChecks{
				NodeStartupTimeoutSeconds: md.HealthCheck.Checks.NodeStartupTimeoutSeconds,
				UnhealthyNodeConditions:   md.HealthCheck.Checks.UnhealthyNodeConditions,
			}, clusterv1.MachineHealthCheckRemediation{
				TriggerIf: clusterv1.MachineHealthCheckRemediationTriggerIf{
					UnhealthyLessThanOrEqualTo: md.HealthCheck.Remediation.TriggerIf.UnhealthyLessThanOrEqualTo,
					UnhealthyInRange:           md.HealthCheck.Remediation.TriggerIf.UnhealthyInRange,
				},
				TemplateRef: md.HealthCheck.Remediation.TemplateRef,
			}
	}

	return clusterv1.MachineHealthCheckChecks{
			NodeStartupTimeoutSeconds: b.MachineDeployments[md.Class].HealthCheck.Checks.NodeStartupTimeoutSeconds,
			UnhealthyNodeConditions:   b.MachineDeployments[md.Class].HealthCheck.Checks.UnhealthyNodeConditions,
		}, clusterv1.MachineHealthCheckRemediation{
			TriggerIf: clusterv1.MachineHealthCheckRemediationTriggerIf{
				UnhealthyLessThanOrEqualTo: b.MachineDeployments[md.Class].HealthCheck.Remediation.TriggerIf.UnhealthyLessThanOrEqualTo,
				UnhealthyInRange:           b.MachineDeployments[md.Class].HealthCheck.Remediation.TriggerIf.UnhealthyInRange,
			},
			TemplateRef: b.MachineDeployments[md.Class].HealthCheck.Remediation.TemplateRef,
		}
}

// HasMachineDeployments checks whether the topology has MachineDeployments.
func (b *ClusterBlueprint) HasMachineDeployments() bool {
	return len(b.Topology.Workers.MachineDeployments) > 0
}

// HasMachinePools checks whether the topology has MachinePools.
func (b *ClusterBlueprint) HasMachinePools() bool {
	return len(b.Topology.Workers.MachinePools) > 0
}
