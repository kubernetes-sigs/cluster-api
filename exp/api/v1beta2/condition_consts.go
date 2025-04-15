/*
Copyright 2025 The Kubernetes Authors.

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

package v1beta2

/*
NOTE: we are commenting const for MachinePool's V1Beta2 conditions and reasons because not yet implemented for the 1.9 CAPI release.
However, we are keeping the v1beta2 struct in the MachinePool struct because the code that will collect conditions and replica
counters at cluster level is already implemented.

// Conditions that will be used for the MachinePool object in v1Beta2 API version.
const (
	// MachinePoolAvailableCondition is true when InfrastructureReady and available replicas >= desired replicas.
	MachinePoolAvailableCondition = clusterv1.AvailableCondition

	// MachinePoolBootstrapConfigReadyCondition mirrors the corresponding condition from the MachinePool's BootstrapConfig resource.
	MachinePoolBootstrapConfigReadyCondition = clusterv1.BootstrapConfigReadyCondition

	// MachinePoolInfrastructureReadyCondition mirrors the corresponding condition from the MachinePool's Infrastructure resource.
	MachinePoolInfrastructureReadyCondition = clusterv1.InfrastructureReadyCondition

	// MachinePoolMachinesReadyCondition surfaces detail of issues on the controlled machines, if any.
	MachinePoolMachinesReadyCondition = clusterv1.MachinesReadyCondition

	// MachinePoolMachinesUpToDateCondition surfaces details of controlled machines not up to date, if any.
	MachinePoolMachinesUpToDateCondition = clusterv1.MachinesUpToDateCondition

	// MachinePoolScalingUpCondition is true if available replicas < desired replicas.
	MachinePoolScalingUpCondition = clusterv1.ScalingUpCondition

	// MachinePoolScalingDownCondition is true if replicas > desired replicas.
	MachinePoolScalingDownCondition = clusterv1.ScalingDownCondition

	// MachinePoolRemediatingCondition surfaces details about ongoing remediation of the controlled machines, if any.
	MachinePoolRemediatingCondition = clusterv1.RemediatingCondition

	// MachinePoolDeletingCondition surfaces details about ongoing deletion of the controlled machines.
	MachinePoolDeletingCondition = clusterv1.DeletingCondition
).
*/
