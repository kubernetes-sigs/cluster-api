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

// Conditions types that are used across different objects.
const (
	// AvailableV1Beta2Condition reports if an object is available.
	// Note: This condition type is defined to ensure consistent naming of conditions across objects.
	// Please use object specific variants of this condition which provides more details for each context where
	// the same condition type exists.
	AvailableV1Beta2Condition = "Available"

	// ReadyV1Beta2Condition reports if an object is ready.
	// Note: This condition type is defined to ensure consistent naming of conditions across objects.
	// Please use object specific variants of this condition which provides more details for each context where
	// the same condition type exists.
	ReadyV1Beta2Condition = "Ready"

	// BootstrapConfigReadyV1Beta2Condition reports if an object's bootstrap config is ready.
	// Note: This condition type is defined to ensure consistent naming of conditions across objects.
	// Please use object specific variants of this condition which provides more details for each context where
	// the same condition type exists.
	BootstrapConfigReadyV1Beta2Condition = "BootstrapConfigReady"

	// InfrastructureReadyV1Beta2Condition reports if an object's infrastructure is ready.
	// Note: This condition type is defined to ensure consistent naming of conditions across objects.
	// Please use object specific variants of this condition which provides more details for each context where
	// the same condition type exists.
	InfrastructureReadyV1Beta2Condition = "InfrastructureReady"

	// MachinesReadyV1Beta2Condition surfaces detail of issues on the controlled machines, if any.
	// Note: This condition type is defined to ensure consistent naming of conditions across objects.
	// Please use object specific variants of this condition which provides more details for each context where
	// the same condition type exists.
	MachinesReadyV1Beta2Condition = "MachinesReady"

	// MachinesUpToDateV1Beta2Condition surfaces details of controlled machines not up to date, if any.
	// Note: This condition type is defined to ensure consistent naming of conditions across objects.
	// Please use object specific variants of this condition which provides more details for each context where
	// the same condition type exists.
	MachinesUpToDateV1Beta2Condition = "MachinesUpToDate"

	// ScalingUpV1Beta2Condition reports if an object is scaling up.
	// Note: This condition type is defined to ensure consistent naming of conditions across objects.
	// Please use object specific variants of this condition which provides more details for each context where
	// the same condition type exists.
	ScalingUpV1Beta2Condition = "ScalingUp"

	// ScalingDownV1Beta2Condition reports if an object is scaling down.
	// Note: This condition type is defined to ensure consistent naming of conditions across objects.
	// Please use object specific variants of this condition which provides more details for each context where
	// the same condition type exists.
	ScalingDownV1Beta2Condition = "ScalingDown"

	// RemediatingV1Beta2Condition surfaces details about ongoing remediation of the controlled machines, if any.
	// Note: This condition type is defined to ensure consistent naming of conditions across objects.
	// Please use object specific variants of this condition which provides more details for each context where
	// the same condition type exists.
	RemediatingV1Beta2Condition = "Remediating"

	// DeletingV1Beta2Condition surfaces details about progress of the object deletion workflow.
	// Note: This condition type is defined to ensure consistent naming of conditions across objects.
	// Please use object specific variants of this condition which provides more details for each context where
	// the same condition type exists.
	DeletingV1Beta2Condition = "Deleting"

	// PausedV1Beta2Condition reports if reconciliation for an object or the cluster is paused.
	// Note: This condition type is defined to ensure consistent naming of conditions across objects.
	// Please use object specific variants of this condition which provides more details for each context where
	// the same condition type exists.
	PausedV1Beta2Condition = "Paused"
)

// Conditions that will be used for the Machine object in v1Beta2 API version.
const (
	// MachineAvailableV1Beta2Condition is true if the machine is Ready for at least MinReadySeconds, as defined by the Machine's MinReadySeconds field.
	MachineAvailableV1Beta2Condition = AvailableV1Beta2Condition

	// MachineReadyV1Beta2Condition is true if the Machine is not deleted, Machine's BootstrapConfigReady, InfrastructureReady,
	// NodeHealthy and HealthCheckSucceeded (if present) are true; if other conditions are defined in spec.readinessGates,
	// these conditions must be true as well.
	MachineReadyV1Beta2Condition = ReadyV1Beta2Condition

	// MachineUpToDateV1Beta2Condition is true if the Machine spec matches the spec of the Machine's owner resource, e.g. KubeadmControlPlane or MachineDeployment.
	// The Machine's owner (e.g MachineDeployment) is authoritative to set their owned Machine's UpToDate conditions based on its current spec.
	MachineUpToDateV1Beta2Condition = "UpToDate"

	// MachineBootstrapConfigReadyV1Beta2Condition condition mirrors the corresponding Ready condition from the Machine's BootstrapConfig resource.
	MachineBootstrapConfigReadyV1Beta2Condition = BootstrapConfigReadyV1Beta2Condition

	// MachineInfrastructureReadyV1Beta2Condition mirrors the corresponding Ready condition from the Machine's Infrastructure resource.
	MachineInfrastructureReadyV1Beta2Condition = InfrastructureReadyV1Beta2Condition

	// MachineNodeHealthyV1Beta2Condition is true if the Machine's Node is ready and it does not report MemoryPressure, DiskPressure and PIDPressure.
	MachineNodeHealthyV1Beta2Condition = "NodeHealthy"

	// MachineNodeReadyV1Beta2Condition is true if the Machine's Node is ready.
	MachineNodeReadyV1Beta2Condition = "NodeReady"

	// MachineHealthCheckSucceededV1Beta2Condition is true if MHC instances targeting this machine report the Machine
	// is healthy according to the definition of healthy present in the spec of the MachineHealthCheck object.
	MachineHealthCheckSucceededV1Beta2Condition = "HealthCheckSucceeded"

	// MachineOwnerRemediatedV1Beta2Condition is only present if MHC instances targeting this machine
	// determine that the controller owning this machine should perform remediation.
	MachineOwnerRemediatedV1Beta2Condition = "OwnerRemediated"

	// MachineDeletingV1Beta2Condition surfaces details about progress in the machine deletion workflow.
	MachineDeletingV1Beta2Condition = DeletingV1Beta2Condition

	// MachinePausedV1Beta2Condition is true if the Machine or the Cluster it belongs to are paused.
	MachinePausedV1Beta2Condition = PausedV1Beta2Condition
)

// Conditions that will be used for the MachineSet object in v1Beta2 API version.
const (
	// MachineSetMachinesReadyV1Beta2Condition surfaces detail of issues on the controlled machines, if any.
	MachineSetMachinesReadyV1Beta2Condition = MachinesReadyV1Beta2Condition

	// MachineSetMachinesUpToDateV1Beta2Condition surfaces details of controlled machines not up to date, if any.
	MachineSetMachinesUpToDateV1Beta2Condition = MachinesUpToDateV1Beta2Condition

	// MachineSetScalingUpV1Beta2Condition is true if available replicas < desired replicas.
	MachineSetScalingUpV1Beta2Condition = ScalingUpV1Beta2Condition

	// MachineSetScalingDownV1Beta2Condition is true if replicas > desired replicas.
	MachineSetScalingDownV1Beta2Condition = ScalingDownV1Beta2Condition

	// MachineSetRemediatingV1Beta2Condition surfaces details about ongoing remediation of the controlled machines, if any.
	MachineSetRemediatingV1Beta2Condition = RemediatingV1Beta2Condition

	// MachineSetDeletingV1Beta2Condition surfaces details about ongoing deletion of the controlled machines.
	MachineSetDeletingV1Beta2Condition = DeletingV1Beta2Condition

	// MachineSetPausedV1Beta2Condition is true if this MachineSet or the Cluster it belongs to are paused.
	MachineSetPausedV1Beta2Condition = PausedV1Beta2Condition
)

// Conditions that will be used for the MachineDeployment object in v1Beta2 API version.
const (
	// MachineDeploymentAvailableV1Beta2Condition is true if the MachineDeployment is not deleted, and it has minimum
	// availability according to parameters specified in the deployment strategy, e.g. If using RollingUpgrade strategy,
	// availableReplicas must be greater or equal than desired replicas - MaxUnavailable replicas.
	MachineDeploymentAvailableV1Beta2Condition = AvailableV1Beta2Condition

	// MachineDeploymentMachinesReadyV1Beta2Condition surfaces detail of issues on the controlled machines, if any.
	MachineDeploymentMachinesReadyV1Beta2Condition = MachinesReadyV1Beta2Condition

	// MachineDeploymentMachinesUpToDateV1Beta2Condition surfaces details of controlled machines not up to date, if any.
	MachineDeploymentMachinesUpToDateV1Beta2Condition = MachinesUpToDateV1Beta2Condition

	// MachineDeploymentScalingUpV1Beta2Condition is true if available replicas < desired replicas.
	MachineDeploymentScalingUpV1Beta2Condition = ScalingUpV1Beta2Condition

	// MachineDeploymentScalingDownV1Beta2Condition is true if replicas > desired replicas.
	MachineDeploymentScalingDownV1Beta2Condition = ScalingDownV1Beta2Condition

	// MachineDeploymentRemediatingV1Beta2Condition details about ongoing remediation of the controlled machines, if any.
	MachineDeploymentRemediatingV1Beta2Condition = RemediatingV1Beta2Condition

	// MachineDeploymentDeletingV1Beta2Condition surfaces details about ongoing deletion of the controlled machines.
	MachineDeploymentDeletingV1Beta2Condition = DeletingV1Beta2Condition

	// MachineDeploymentPausedV1Beta2Condition is true if this MachineDeployment or the Cluster it belongs to are paused.
	MachineDeploymentPausedV1Beta2Condition = PausedV1Beta2Condition
)

// Conditions that will be used for the Cluster object in v1Beta2 API version.
const (
	// ClusterAvailableV1Beta2Condition is true if the Cluster's is not deleted, and RemoteConnectionProbe, InfrastructureReady,
	// ControlPlaneAvailable, WorkersAvailable, TopologyReconciled (if present) conditions are true.
	// If conditions are defined in spec.availabilityGates, those conditions must be true as well.
	ClusterAvailableV1Beta2Condition = AvailableV1Beta2Condition

	// ClusterTopologyReconciledV1Beta2Condition is true if the topology controller is working properly.
	// Note:This condition is added only if the Cluster is referencing a ClusterClass / defining a managed Topology.
	ClusterTopologyReconciledV1Beta2Condition = "TopologyReconciled"

	// ClusterInfrastructureReadyV1Beta2Condition Mirror of Cluster's infrastructure Ready condition.
	ClusterInfrastructureReadyV1Beta2Condition = InfrastructureReadyV1Beta2Condition

	// ClusterControlPlaneInitializedV1Beta2Condition is true when the Cluster's control plane is functional enough
	// to accept requests. This information is usually used as a signal for starting all the provisioning operations
	// that depends on a functional API server, but do not require a full HA control plane to exists.
	ClusterControlPlaneInitializedV1Beta2Condition = "ControlPlaneInitialized"

	// ClusterControlPlaneAvailableV1Beta2Condition is a mirror of Cluster's control plane Available condition.
	ClusterControlPlaneAvailableV1Beta2Condition = "ControlPlaneAvailable"

	// ClusterWorkersAvailableV1Beta2Condition is the summary of MachineDeployment and MachinePool's Available conditions.
	ClusterWorkersAvailableV1Beta2Condition = "WorkersAvailable"

	// ClusterMachinesReadyV1Beta2Condition surfaces detail of issues on the controlled machines, if any.
	ClusterMachinesReadyV1Beta2Condition = MachinesReadyV1Beta2Condition

	// ClusterMachinesUpToDateV1Beta2Condition surfaces details of Cluster's machines not up to date, if any.
	ClusterMachinesUpToDateV1Beta2Condition = MachinesUpToDateV1Beta2Condition

	// ClusterRemoteConnectionProbeV1Beta2Condition is true when control plane can be reached; in case of connection problems.
	// The condition turns to false only if the cluster cannot be reached for 50s after the first connection problem
	// is detected (or whatever period is defined in the --remote-connection-grace-period flag).
	ClusterRemoteConnectionProbeV1Beta2Condition = "RemoteConnectionProbe"

	// ClusterScalingUpV1Beta2Condition is true if available replicas < desired replicas.
	ClusterScalingUpV1Beta2Condition = ScalingUpV1Beta2Condition

	// ClusterScalingDownV1Beta2Condition is true if replicas > desired replicas.
	ClusterScalingDownV1Beta2Condition = ScalingDownV1Beta2Condition

	// ClusterRemediatingV1Beta2Condition surfaces details about ongoing remediation of the controlled machines, if any.
	ClusterRemediatingV1Beta2Condition = RemediatingV1Beta2Condition

	// ClusterDeletingV1Beta2Condition surfaces details about ongoing deletion of the cluster.
	ClusterDeletingV1Beta2Condition = DeletingV1Beta2Condition

	// ClusterPausedV1Beta2Condition is true if Cluster and all the resources being part of it are paused.
	ClusterPausedV1Beta2Condition = PausedV1Beta2Condition
)

// Conditions that will be used for the MachineHealthCheck object in v1Beta2 API version.
const (
	// MachineHealthCheckRemediationAllowedV1Beta2Condition surfaces whether the MachineHealthCheck is
	// allowed to remediate any Machines or whether it is blocked from remediating any further.
	MachineHealthCheckRemediationAllowedV1Beta2Condition = "RemediationAllowed"

	// MachineHealthCheckPausedV1Beta2Condition is true if this MachineHealthCheck or the Cluster it belongs to are paused.
	MachineHealthCheckPausedV1Beta2Condition = PausedV1Beta2Condition
)

// Conditions that will be used for the ClusterClass object in v1Beta2 API version.
const (
	// ClusterClassVariablesReadyV1Beta2Condition is true if the ClusterClass variables, including both inline and external
	// variables, have been successfully reconciled and thus ready to be used to default and validate variables on Clusters using
	// this ClusterClass.
	ClusterClassVariablesReadyV1Beta2Condition = "VariablesReady"

	// ClusterClassRefVersionsUpToDateV1Beta2Condition documents if the references in the ClusterClass are
	// up-to-date (i.e. they are using the latest apiVersion of the current Cluster API contract from
	// the corresponding CRD).
	ClusterClassRefVersionsUpToDateV1Beta2Condition = "RefVersionsUpToDate"

	// ClusterClassPausedV1Beta2Condition is true if this ClusterClass is paused.
	ClusterClassPausedV1Beta2Condition = PausedV1Beta2Condition
)
