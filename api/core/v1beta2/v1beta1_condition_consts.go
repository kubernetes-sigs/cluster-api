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

// Common ConditionTypes used by Cluster API objects.
const (
	// ReadyV1Beta1Condition defines the Ready condition type that summarizes the operational state of a Cluster API object.
	ReadyV1Beta1Condition ConditionType = "Ready"
)

// Common ConditionReason used by Cluster API objects.
const (
	// DeletingV1Beta1Reason (Severity=Info) documents a condition not in Status=True because the underlying object it is currently being deleted.
	DeletingV1Beta1Reason = "Deleting"

	// DeletionFailedV1Beta1Reason (Severity=Warning) documents a condition not in Status=True because the underlying object
	// encountered problems during deletion. This is a warning because the reconciler will retry deletion.
	DeletionFailedV1Beta1Reason = "DeletionFailed"

	// DeletedV1Beta1Reason (Severity=Info) documents a condition not in Status=True because the underlying object was deleted.
	DeletedV1Beta1Reason = "Deleted"

	// IncorrectExternalRefV1Beta1Reason (Severity=Error) documents a CAPI object with an incorrect external object reference.
	IncorrectExternalRefV1Beta1Reason = "IncorrectExternalRef"
)

const (
	// InfrastructureReadyV1Beta1Condition reports a summary of current status of the infrastructure object defined for this cluster/machine/machinepool.
	// This condition is mirrored from the Ready condition in the infrastructure ref object, and
	// the absence of this condition might signal problems in the reconcile external loops or the fact that
	// the infrastructure provider does not implement the Ready condition yet.
	InfrastructureReadyV1Beta1Condition ConditionType = "InfrastructureReady"

	// WaitingForInfrastructureFallbackV1Beta1Reason (Severity=Info) documents a cluster/machine/machinepool waiting for the underlying infrastructure
	// to be available.
	// NOTE: This reason is used only as a fallback when the infrastructure object is not reporting its own ready condition.
	WaitingForInfrastructureFallbackV1Beta1Reason = "WaitingForInfrastructure"
)

// Conditions and condition Reasons for the ClusterClass object.
const (
	// ClusterClassVariablesReconciledV1Beta1Condition reports if the ClusterClass variables, including both inline and external
	// variables, have been successfully reconciled.
	// This signals that the ClusterClass is ready to be used to default and validate variables on Clusters using
	// this ClusterClass.
	ClusterClassVariablesReconciledV1Beta1Condition ConditionType = "VariablesReconciled"

	// VariableDiscoveryFailedV1Beta1Reason (Severity=Error) documents a ClusterClass with VariableDiscovery extensions that
	// failed.
	VariableDiscoveryFailedV1Beta1Reason = "VariableDiscoveryFailed"
)

// Conditions and condition Reasons for the Cluster object.

const (
	// ControlPlaneInitializedV1Beta1Condition reports if the cluster's control plane has been initialized such that the
	// cluster's apiserver is reachable. If no Control Plane provider is in use this condition reports that at least one
	// control plane Machine has a node reference. Once this Condition is marked true, its value is never changed. See
	// the ControlPlaneReady condition for an indication of the current readiness of the cluster's control plane.
	ControlPlaneInitializedV1Beta1Condition ConditionType = "ControlPlaneInitialized"

	// MissingNodeRefV1Beta1Reason (Severity=Info) documents a cluster waiting for at least one control plane Machine to have
	// its node reference populated.
	MissingNodeRefV1Beta1Reason = "MissingNodeRef"

	// WaitingForControlPlaneProviderInitializedV1Beta1Reason (Severity=Info) documents a cluster waiting for the control plane
	// provider to report successful control plane initialization.
	WaitingForControlPlaneProviderInitializedV1Beta1Reason = "WaitingForControlPlaneProviderInitialized"

	// ControlPlaneReadyV1Beta1Condition reports the ready condition from the control plane object defined for this cluster.
	// This condition is mirrored from the Ready condition in the control plane ref object, and
	// the absence of this condition might signal problems in the reconcile external loops or the fact that
	// the control plane provider does not implement the Ready condition yet.
	ControlPlaneReadyV1Beta1Condition ConditionType = "ControlPlaneReady"

	// WaitingForControlPlaneFallbackV1Beta1Reason (Severity=Info) documents a cluster waiting for the control plane
	// to be available.
	// NOTE: This reason is used only as a fallback when the control plane object is not reporting its own ready condition.
	WaitingForControlPlaneFallbackV1Beta1Reason = "WaitingForControlPlane"

	// WaitingForControlPlaneAvailableV1Beta1Reason (Severity=Info) documents a Cluster API object
	// waiting for the control plane machine to be available.
	//
	// NOTE: Having the control plane machine available is a pre-condition for joining additional control planes
	// or workers nodes.
	WaitingForControlPlaneAvailableV1Beta1Reason = "WaitingForControlPlaneAvailable"
)

// Conditions and condition Reasons for the Machine object.

const (
	// BootstrapReadyV1Beta1Condition reports a summary of current status of the bootstrap object defined for this machine.
	// This condition is mirrored from the Ready condition in the bootstrap ref object, and
	// the absence of this condition might signal problems in the reconcile external loops or the fact that
	// the bootstrap provider does not implement the Ready condition yet.
	BootstrapReadyV1Beta1Condition ConditionType = "BootstrapReady"

	// WaitingForDataSecretFallbackV1Beta1Reason (Severity=Info) documents a machine waiting for the bootstrap data secret
	// to be available.
	// NOTE: This reason is used only as a fallback when the bootstrap object is not reporting its own ready condition.
	WaitingForDataSecretFallbackV1Beta1Reason = "WaitingForDataSecret"

	// DrainingSucceededV1Beta1Condition provide evidence of the status of the node drain operation which happens during the machine
	// deletion process.
	DrainingSucceededV1Beta1Condition ConditionType = "DrainingSucceeded"

	// DrainingV1Beta1Reason (Severity=Info) documents a machine node being drained.
	DrainingV1Beta1Reason = "Draining"

	// DrainingFailedV1Beta1Reason (Severity=Warning) documents a machine node drain operation failed.
	DrainingFailedV1Beta1Reason = "DrainingFailed"

	// PreDrainDeleteHookSucceededV1Beta1Condition reports a machine waiting for a PreDrainDeleteHook before being delete.
	PreDrainDeleteHookSucceededV1Beta1Condition ConditionType = "PreDrainDeleteHookSucceeded"

	// PreTerminateDeleteHookSucceededV1Beta1Condition reports a machine waiting for a PreDrainDeleteHook before being delete.
	PreTerminateDeleteHookSucceededV1Beta1Condition ConditionType = "PreTerminateDeleteHookSucceeded"

	// WaitingExternalHookV1Beta1Reason (Severity=Info) provide evidence that we are waiting for an external hook to complete.
	WaitingExternalHookV1Beta1Reason = "WaitingExternalHook"

	// VolumeDetachSucceededV1Beta1Condition reports a machine waiting for volumes to be detached.
	VolumeDetachSucceededV1Beta1Condition ConditionType = "VolumeDetachSucceeded"

	// WaitingForVolumeDetachV1Beta1Reason (Severity=Info) provide evidence that a machine node waiting for volumes to be attached.
	WaitingForVolumeDetachV1Beta1Reason = "WaitingForVolumeDetach"
)

const (
	// MachineHealthCheckSucceededV1Beta1Condition is set on machines that have passed a healthcheck by the MachineHealthCheck controller.
	// In the event that the health check fails it will be set to False.
	MachineHealthCheckSucceededV1Beta1Condition ConditionType = "HealthCheckSucceeded"

	// MachineHasFailureV1Beta1Reason is the reason used when a machine has either a FailureReason or a FailureMessage set on its status.
	MachineHasFailureV1Beta1Reason = "MachineHasFailure"

	// HasRemediateMachineAnnotationV1Beta1Reason is the reason that get's set at the MachineHealthCheckSucceededCondition when a machine
	// has the RemediateMachineAnnotation set.
	HasRemediateMachineAnnotationV1Beta1Reason = "HasRemediateMachineAnnotation"

	// NodeStartupTimeoutV1Beta1Reason is the reason used when a machine's node does not appear within the specified timeout.
	NodeStartupTimeoutV1Beta1Reason = "NodeStartupTimeout"

	// UnhealthyNodeConditionV1Beta1Reason is the reason used when a machine's node has one of the MachineHealthCheck's unhealthy conditions.
	UnhealthyNodeConditionV1Beta1Reason = "UnhealthyNode"

	// UnhealthyMachineConditionV1Beta1Reason is the reason used when a machine has one of the MachineHealthCheck's unhealthy conditions.
	// When both machine and node issues are detected, this reason takes precedence over node-related reasons
	// (NodeNotFoundV1Beta1Reason, NodeStartupTimeoutV1Beta1Reason, UnhealthyNodeConditionV1Beta1Reason).
	UnhealthyMachineConditionV1Beta1Reason = "UnhealthyMachine"
)

const (
	// MachineOwnerRemediatedV1Beta1Condition is set on machines that have failed a healthcheck by the MachineHealthCheck controller.
	// MachineOwnerRemediatedV1Beta1Condition is set to False after a health check fails, but should be changed to True by the owning controller after remediation succeeds.
	MachineOwnerRemediatedV1Beta1Condition ConditionType = "OwnerRemediated"

	// WaitingForRemediationV1Beta1Reason is the reason used when a machine fails a health check and remediation is needed.
	WaitingForRemediationV1Beta1Reason = "WaitingForRemediation"

	// RemediationFailedV1Beta1Reason is the reason used when a remediation owner fails to remediate an unhealthy machine.
	RemediationFailedV1Beta1Reason = "RemediationFailed"

	// RemediationInProgressV1Beta1Reason is the reason used when an unhealthy machine is being remediated by the remediation owner.
	RemediationInProgressV1Beta1Reason = "RemediationInProgress"

	// ExternalRemediationTemplateAvailableV1Beta1Condition is set on machinehealthchecks when MachineHealthCheck controller uses external remediation.
	// ExternalRemediationTemplateAvailableV1Beta1Condition is set to false if external remediation template is not found.
	ExternalRemediationTemplateAvailableV1Beta1Condition ConditionType = "ExternalRemediationTemplateAvailable"

	// ExternalRemediationTemplateNotFoundV1Beta1Reason is the reason used when a machine health check fails to find external remediation template.
	ExternalRemediationTemplateNotFoundV1Beta1Reason = "ExternalRemediationTemplateNotFound"

	// ExternalRemediationRequestAvailableV1Beta1Condition is set on machinehealthchecks when MachineHealthCheck controller uses external remediation.
	// ExternalRemediationRequestAvailableV1Beta1Condition is set to false if creating external remediation request fails.
	ExternalRemediationRequestAvailableV1Beta1Condition ConditionType = "ExternalRemediationRequestAvailable"

	// ExternalRemediationRequestCreationFailedV1Beta1Reason is the reason used when a machine health check fails to create external remediation request.
	ExternalRemediationRequestCreationFailedV1Beta1Reason = "ExternalRemediationRequestCreationFailed"
)

// Conditions and condition Reasons for the Machine's Node object.
const (
	// MachineNodeHealthyV1Beta1Condition provides info about the operational state of the Kubernetes node hosted on the machine by summarizing  node conditions.
	// If the conditions defined in a Kubernetes node (i.e., NodeReady, NodeMemoryPressure, NodeDiskPressure and NodePIDPressure) are in a healthy state, it will be set to True.
	MachineNodeHealthyV1Beta1Condition ConditionType = "NodeHealthy"

	// WaitingForNodeRefV1Beta1Reason (Severity=Info) documents a machine.spec.providerId is not assigned yet.
	WaitingForNodeRefV1Beta1Reason = "WaitingForNodeRef"

	// NodeProvisioningV1Beta1Reason (Severity=Info) documents machine in the process of provisioning a node.
	// NB. provisioning --> NodeRef == "".
	NodeProvisioningV1Beta1Reason = "NodeProvisioning"

	// NodeNotFoundV1Beta1Reason (Severity=Error) documents a machine's node has previously been observed but is now gone.
	// NB. provisioned --> NodeRef != "".
	NodeNotFoundV1Beta1Reason = "NodeNotFound"

	// NodeConditionsFailedV1Beta1Reason (Severity=Warning) documents a node is not in a healthy state due to the failed state of at least 1 Kubelet condition.
	NodeConditionsFailedV1Beta1Reason = "NodeConditionsFailed"

	// NodeInspectionFailedV1Beta1Reason documents a failure in inspecting the node.
	// This reason is used when the Machine controller is unable to list Nodes to find
	// the corresponding Node for a Machine by ProviderID.
	NodeInspectionFailedV1Beta1Reason = "NodeInspectionFailed"
)

// Conditions and condition Reasons for the MachineHealthCheck object.

const (
	// RemediationAllowedV1Beta1Condition is set on MachineHealthChecks to show the status of whether the MachineHealthCheck is
	// allowed to remediate any Machines or whether it is blocked from remediating any further.
	RemediationAllowedV1Beta1Condition ConditionType = "RemediationAllowed"

	// TooManyUnhealthyV1Beta1Reason is the reason used when too many Machines are unhealthy and the MachineHealthCheck is blocked
	// from making any further remediations.
	TooManyUnhealthyV1Beta1Reason = "TooManyUnhealthy"
)

// Conditions and condition Reasons for  MachineDeployments.

const (
	// MachineDeploymentAvailableV1Beta1Condition means the MachineDeployment is available, that is, at least the minimum available
	// machines required (i.e. Spec.Replicas-MaxUnavailable when spec.rollout.strategy.type = RollingUpdate) are up and running for at least minReadySeconds.
	MachineDeploymentAvailableV1Beta1Condition ConditionType = "Available"

	// MachineSetReadyV1Beta1Condition reports a summary of current status of the MachineSet owned by the MachineDeployment.
	MachineSetReadyV1Beta1Condition ConditionType = "MachineSetReady"

	// WaitingForMachineSetFallbackV1Beta1Reason (Severity=Info) documents a MachineDeployment waiting for the underlying MachineSet
	// to be available.
	// NOTE: This reason is used only as a fallback when the MachineSet object is not reporting its own ready condition.
	WaitingForMachineSetFallbackV1Beta1Reason = "WaitingForMachineSet"

	// WaitingForAvailableMachinesV1Beta1Reason (Severity=Warning) reflects the fact that the required minimum number of machines for a machinedeployment are not available.
	WaitingForAvailableMachinesV1Beta1Reason = "WaitingForAvailableMachines"
)

// Conditions and condition Reasons for  MachineSets.

const (
	// MachinesCreatedV1Beta1Condition documents that the machines controlled by the MachineSet are created.
	// When this condition is false, it indicates that there was an error when cloning the infrastructure/bootstrap template or
	// when generating the machine object.
	MachinesCreatedV1Beta1Condition ConditionType = "MachinesCreated"

	// MachinesReadyV1Beta1Condition reports an aggregate of current status of the machines controlled by the MachineSet.
	MachinesReadyV1Beta1Condition ConditionType = "MachinesReady"

	// PreflightCheckFailedV1Beta1Reason (Severity=Error) documents a MachineSet failing preflight checks
	// to create machine(s).
	PreflightCheckFailedV1Beta1Reason = "PreflightCheckFailed"

	// BootstrapTemplateCloningFailedV1Beta1Reason (Severity=Error) documents a MachineSet failing to
	// clone the bootstrap template.
	BootstrapTemplateCloningFailedV1Beta1Reason = "BootstrapTemplateCloningFailed"

	// InfrastructureTemplateCloningFailedV1Beta1Reason (Severity=Error) documents a MachineSet failing to
	// clone the infrastructure template.
	InfrastructureTemplateCloningFailedV1Beta1Reason = "InfrastructureTemplateCloningFailed"

	// MachineCreationFailedV1Beta1Reason (Severity=Error) documents a MachineSet failing to
	// generate a machine object.
	MachineCreationFailedV1Beta1Reason = "MachineCreationFailed"

	// ResizedV1Beta1Condition documents a MachineSet is resizing the set of controlled machines.
	ResizedV1Beta1Condition ConditionType = "Resized"

	// ScalingUpV1Beta1Reason (Severity=Info) documents a MachineSet is increasing the number of replicas.
	ScalingUpV1Beta1Reason = "ScalingUp"

	// ScalingDownV1Beta1Reason (Severity=Info) documents a MachineSet is decreasing the number of replicas.
	ScalingDownV1Beta1Reason = "ScalingDown"
)

// Conditions and condition reasons for Clusters with a managed Topology.
const (
	// TopologyReconciledV1Beta1Condition provides evidence about the reconciliation of a Cluster topology into
	// the managed objects of the Cluster.
	// Status false means that for any reason, the values defined in Cluster.spec.topology are not yet applied to
	// managed objects on the Cluster; status true means that Cluster.spec.topology have been applied to
	// the objects in the Cluster (but this does not imply those objects are already reconciled to the spec provided).
	TopologyReconciledV1Beta1Condition ConditionType = "TopologyReconciled"

	// TopologyReconcileFailedV1Beta1Reason (Severity=Error) documents the reconciliation of a Cluster topology
	// failing due to an error.
	TopologyReconcileFailedV1Beta1Reason = "TopologyReconcileFailed"

	// TopologyReconciledClusterCreatingV1Beta1Reason documents reconciliation of a Cluster topology
	// not yet created because the BeforeClusterCreate hook is blocking.
	TopologyReconciledClusterCreatingV1Beta1Reason = "ClusterCreating"

	// TopologyReconciledControlPlaneUpgradePendingV1Beta1Reason (Severity=Info) documents reconciliation of a Cluster topology
	// not yet completed because Control Plane is not yet updated to match the desired topology spec.
	//
	// Deprecated: please use ClusterUpgrading instead.
	TopologyReconciledControlPlaneUpgradePendingV1Beta1Reason = "ControlPlaneUpgradePending"

	// TopologyReconciledMachineDeploymentsCreatePendingV1Beta1Reason (Severity=Info) documents reconciliation of a Cluster topology
	// not yet completed because at least one of the MachineDeployments is yet to be created.
	// This generally happens because new MachineDeployment creations are held off while the ControlPlane is not stable.
	//
	// Deprecated: please use ClusterUpgrading instead.
	TopologyReconciledMachineDeploymentsCreatePendingV1Beta1Reason = "MachineDeploymentsCreatePending"

	// TopologyReconciledMachineDeploymentsUpgradePendingV1Beta1Reason (Severity=Info) documents reconciliation of a Cluster topology
	// not yet completed because at least one of the MachineDeployments is not yet updated to match the desired topology spec.
	//
	// Deprecated: please use ClusterUpgrading instead.
	TopologyReconciledMachineDeploymentsUpgradePendingV1Beta1Reason = "MachineDeploymentsUpgradePending"

	// TopologyReconciledMachineDeploymentsUpgradeDeferredV1Beta1Reason (Severity=Info) documents reconciliation of a Cluster topology
	// not yet completed because the upgrade for at least one of the MachineDeployments has been deferred.
	TopologyReconciledMachineDeploymentsUpgradeDeferredV1Beta1Reason = "MachineDeploymentsUpgradeDeferred"

	// TopologyReconciledMachinePoolsUpgradePendingV1Beta1Reason (Severity=Info) documents reconciliation of a Cluster topology
	// not yet completed because at least one of the MachinePools is not yet updated to match the desired topology spec.
	//
	// Deprecated: please use ClusterUpgrading instead.
	TopologyReconciledMachinePoolsUpgradePendingV1Beta1Reason = "MachinePoolsUpgradePending"

	// TopologyReconciledMachinePoolsCreatePendingV1Beta1Reason (Severity=Info) documents reconciliation of a Cluster topology
	// not yet completed because at least one of the MachinePools is yet to be created.
	// This generally happens because new MachinePool creations are held off while the ControlPlane is not stable.
	//
	// Deprecated: please use ClusterUpgrading instead.
	TopologyReconciledMachinePoolsCreatePendingV1Beta1Reason = "MachinePoolsCreatePending"

	// TopologyReconciledMachinePoolsUpgradeDeferredV1Beta1Reason (Severity=Info) documents reconciliation of a Cluster topology
	// not yet completed because the upgrade for at least one of the MachinePools has been deferred.
	TopologyReconciledMachinePoolsUpgradeDeferredV1Beta1Reason = "MachinePoolsUpgradeDeferred"

	// TopologyReconciledHookBlockingV1Beta1Reason (Severity=Info) documents reconciliation of a Cluster topology
	// not yet completed because at least one of the lifecycle hooks is blocking.
	//
	// Deprecated: please use ClusterUpgrading instead.
	TopologyReconciledHookBlockingV1Beta1Reason = "LifecycleHookBlocking"

	// TopologyReconciledClusterUpgradingV1Beta1Reason documents reconciliation of a Cluster topology
	// not yet completed because a cluster upgrade is still in progress.
	TopologyReconciledClusterUpgradingV1Beta1Reason = "ClusterUpgrading"

	// TopologyReconciledClusterClassNotReconciledV1Beta1Reason (Severity=Info) documents reconciliation of a Cluster topology not
	// yet completed because the ClusterClass has not reconciled yet. If this condition persists there may be an issue
	// with the ClusterClass surfaced in the ClusterClass status or controller logs.
	TopologyReconciledClusterClassNotReconciledV1Beta1Reason = "ClusterClassNotReconciled"

	// TopologyReconciledPausedV1Beta1Reason (Severity=Info) surfaces when the Cluster is paused.
	TopologyReconciledPausedV1Beta1Reason = "Paused"
)

// Conditions and condition reasons for ClusterClass.
const (
	// ClusterClassRefVersionsUpToDateV1Beta1Condition documents if the references in the ClusterClass are
	// up-to-date (i.e. they are using the latest apiVersion of the current Cluster API contract from
	// the corresponding CRD).
	ClusterClassRefVersionsUpToDateV1Beta1Condition ConditionType = "RefVersionsUpToDate"

	// ClusterClassOutdatedRefVersionsV1Beta1Reason (Severity=Warning) that the references in the ClusterClass are not
	// up-to-date (i.e. they are not using the latest apiVersion of the current Cluster API contract from
	// the corresponding CRD).
	ClusterClassOutdatedRefVersionsV1Beta1Reason = "OutdatedRefVersions"

	// ClusterClassRefVersionsUpToDateInternalErrorV1Beta1Reason (Severity=Warning) surfaces that an unexpected error occurred when validating
	// if the references are up-to-date.
	ClusterClassRefVersionsUpToDateInternalErrorV1Beta1Reason = "InternalError"
)

// Conditions and condition Reasons for the MachinePool object.

const (
	// ReplicasReadyV1Beta1Condition reports an aggregate of current status of the replicas controlled by the MachinePool.
	ReplicasReadyV1Beta1Condition ConditionType = "ReplicasReady"

	// WaitingForReplicasReadyV1Beta1Reason (Severity=Info) documents a machinepool waiting for the required replicas
	// to be ready.
	WaitingForReplicasReadyV1Beta1Reason = "WaitingForReplicasReady"
)
