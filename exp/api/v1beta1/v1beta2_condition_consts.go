/*
Copyright 2024 The Kubernetes Authors.

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

package v1beta1

/*
NOTE: we are commenting const for MachinePool's V1Beta2 conditions and reasons because not yet implemented for the 1.9 CAPI release.
However, we are keeping the v1beta2 struct in the MachinePool struct because the code that will collect conditions and replica
counters at cluster level is already implemented.

// Conditions that will be used for the MachinePool object in v1Beta2 API version.
const (
	// MachinePoolAvailableV1Beta2Condition is true when InfrastructureReady and available replicas >= desired replicas.
	MachinePoolAvailableV1Beta2Condition = clusterv1.AvailableV1Beta2Condition

	// MachinePoolBootstrapConfigReadyV1Beta2Condition mirrors the corresponding condition from the MachinePool's BootstrapConfig resource.
	MachinePoolBootstrapConfigReadyV1Beta2Condition = clusterv1.BootstrapConfigReadyV1Beta2Condition

	// MachinePoolInfrastructureReadyV1Beta2Condition mirrors the corresponding condition from the MachinePool's Infrastructure resource.
	MachinePoolInfrastructureReadyV1Beta2Condition = clusterv1.InfrastructureReadyV1Beta2Condition

	// MachinePoolMachinesReadyV1Beta2Condition surfaces detail of issues on the controlled machines, if any.
	MachinePoolMachinesReadyV1Beta2Condition = clusterv1.MachinesReadyV1Beta2Condition

	// MachinePoolMachinesUpToDateV1Beta2Condition surfaces details of controlled machines not up to date, if any.
	MachinePoolMachinesUpToDateV1Beta2Condition = clusterv1.MachinesUpToDateV1Beta2Condition

	// MachinePoolScalingUpV1Beta2Condition is true if available replicas < desired replicas.
	MachinePoolScalingUpV1Beta2Condition = clusterv1.ScalingUpV1Beta2Condition

	// MachinePoolScalingDownV1Beta2Condition is true if replicas > desired replicas.
	MachinePoolScalingDownV1Beta2Condition = clusterv1.ScalingDownV1Beta2Condition

	// MachinePoolRemediatingV1Beta2Condition surfaces details about ongoing remediation of the controlled machines, if any.
	MachinePoolRemediatingV1Beta2Condition = clusterv1.RemediatingV1Beta2Condition

	// MachinePoolDeletingV1Beta2Condition surfaces details about ongoing deletion of the controlled machines.
	MachinePoolDeletingV1Beta2Condition = clusterv1.DeletingV1Beta2Condition
).
*/
