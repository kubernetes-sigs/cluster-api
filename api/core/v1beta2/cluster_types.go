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

import (
	"cmp"
	"fmt"
	"net"
	"strings"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	capierrors "sigs.k8s.io/cluster-api/errors"
)

const (
	// ClusterFinalizer is the finalizer used by the cluster controller to
	// cleanup the cluster resources when a Cluster is being deleted.
	ClusterFinalizer = "cluster.cluster.x-k8s.io"

	// ClusterKind represents the Kind of Cluster.
	ClusterKind = "Cluster"
)

// Cluster's Available condition and corresponding reasons.
const (
	// ClusterAvailableCondition is true if the Cluster is not deleted, and RemoteConnectionProbe, InfrastructureReady,
	// ControlPlaneAvailable, WorkersAvailable, TopologyReconciled (if present) conditions are true.
	// If conditions are defined in spec.availabilityGates, those conditions must be true as well.
	// Note:
	// - When summarizing TopologyReconciled, all reasons except TopologyReconcileFailed and ClusterClassNotReconciled will
	//   be treated as info. This is because even if topology is not fully reconciled, this is an expected temporary state
	//   and it doesn't impact availability.
	// - When summarizing InfrastructureReady, ControlPlaneAvailable, in case the Cluster is deleting, the absence of the
	//   referenced object won't be considered as an issue.
	ClusterAvailableCondition = AvailableCondition

	// ClusterAvailableReason surfaces when the cluster availability criteria is met.
	ClusterAvailableReason = AvailableReason

	// ClusterNotAvailableReason surfaces when the cluster availability criteria is not met (and thus the machine is not available).
	ClusterNotAvailableReason = NotAvailableReason

	// ClusterAvailableUnknownReason surfaces when at least one cluster availability criteria is unknown
	// and no availability criteria is not met.
	ClusterAvailableUnknownReason = AvailableUnknownReason

	// ClusterAvailableInternalErrorReason surfaces unexpected error when computing the Available condition.
	ClusterAvailableInternalErrorReason = InternalErrorReason
)

// Cluster's TopologyReconciled condition and corresponding reasons.
const (
	// ClusterTopologyReconciledCondition is true if the topology controller is working properly.
	// Note: This condition is added only if the Cluster is referencing a ClusterClass / defining a managed Topology.
	ClusterTopologyReconciledCondition = "TopologyReconciled"

	// ClusterTopologyReconcileSucceededReason documents the reconciliation of a Cluster topology succeeded.
	ClusterTopologyReconcileSucceededReason = "ReconcileSucceeded"

	// ClusterTopologyReconciledFailedReason documents the reconciliation of a Cluster topology
	// failing due to an error.
	ClusterTopologyReconciledFailedReason = "ReconcileFailed"

	// ClusterTopologyReconciledControlPlaneUpgradePendingReason documents reconciliation of a Cluster topology
	// not yet completed because Control Plane is not yet updated to match the desired topology spec.
	ClusterTopologyReconciledControlPlaneUpgradePendingReason = "ControlPlaneUpgradePending"

	// ClusterTopologyReconciledMachineDeploymentsCreatePendingReason documents reconciliation of a Cluster topology
	// not yet completed because at least one of the MachineDeployments is yet to be created.
	// This generally happens because new MachineDeployment creations are held off while the ControlPlane is not stable.
	ClusterTopologyReconciledMachineDeploymentsCreatePendingReason = "MachineDeploymentsCreatePending"

	// ClusterTopologyReconciledMachineDeploymentsUpgradePendingReason documents reconciliation of a Cluster topology
	// not yet completed because at least one of the MachineDeployments is not yet updated to match the desired topology spec.
	ClusterTopologyReconciledMachineDeploymentsUpgradePendingReason = "MachineDeploymentsUpgradePending"

	// ClusterTopologyReconciledMachineDeploymentsUpgradeDeferredReason documents reconciliation of a Cluster topology
	// not yet completed because the upgrade for at least one of the MachineDeployments has been deferred.
	ClusterTopologyReconciledMachineDeploymentsUpgradeDeferredReason = "MachineDeploymentsUpgradeDeferred"

	// ClusterTopologyReconciledMachinePoolsUpgradePendingReason documents reconciliation of a Cluster topology
	// not yet completed because at least one of the MachinePools is not yet updated to match the desired topology spec.
	ClusterTopologyReconciledMachinePoolsUpgradePendingReason = "MachinePoolsUpgradePending"

	// ClusterTopologyReconciledMachinePoolsCreatePendingReason documents reconciliation of a Cluster topology
	// not yet completed because at least one of the MachinePools is yet to be created.
	// This generally happens because new MachinePool creations are held off while the ControlPlane is not stable.
	ClusterTopologyReconciledMachinePoolsCreatePendingReason = "MachinePoolsCreatePending"

	// ClusterTopologyReconciledMachinePoolsUpgradeDeferredReason documents reconciliation of a Cluster topology
	// not yet completed because the upgrade for at least one of the MachinePools has been deferred.
	ClusterTopologyReconciledMachinePoolsUpgradeDeferredReason = "MachinePoolsUpgradeDeferred"

	// ClusterTopologyReconciledHookBlockingReason documents reconciliation of a Cluster topology
	// not yet completed because at least one of the lifecycle hooks is blocking.
	ClusterTopologyReconciledHookBlockingReason = "LifecycleHookBlocking"

	// ClusterTopologyReconciledClusterClassNotReconciledReason documents reconciliation of a Cluster topology not
	// yet completed because the ClusterClass has not reconciled yet. If this condition persists there may be an issue
	// with the ClusterClass surfaced in the ClusterClass status or controller logs.
	ClusterTopologyReconciledClusterClassNotReconciledReason = "ClusterClassNotReconciled"

	// ClusterTopologyReconciledDeletingReason surfaces when the Cluster is deleting because the
	// DeletionTimestamp is set.
	ClusterTopologyReconciledDeletingReason = DeletingReason

	// ClusterTopologyReconcilePausedReason surfaces when the Cluster is paused.
	ClusterTopologyReconcilePausedReason = PausedReason
)

// Cluster's InfrastructureReady condition and corresponding reasons.
const (
	// ClusterInfrastructureReadyCondition mirrors Cluster's infrastructure Ready condition.
	ClusterInfrastructureReadyCondition = InfrastructureReadyCondition

	// ClusterInfrastructureReadyReason surfaces when the cluster infrastructure is ready.
	ClusterInfrastructureReadyReason = ReadyReason

	// ClusterInfrastructureNotReadyReason surfaces when the cluster infrastructure is not ready.
	ClusterInfrastructureNotReadyReason = NotReadyReason

	// ClusterInfrastructureInvalidConditionReportedReason surfaces a infrastructure Ready condition (read from an infra cluster object) which is invalid
	// (e.g. its status is missing).
	ClusterInfrastructureInvalidConditionReportedReason = InvalidConditionReportedReason

	// ClusterInfrastructureInternalErrorReason surfaces unexpected failures when reading an infra cluster object.
	ClusterInfrastructureInternalErrorReason = InternalErrorReason

	// ClusterInfrastructureDoesNotExistReason surfaces when a referenced infrastructure object does not exist.
	// Note: this could happen when creating the Cluster. However, this state should be treated as an error if it lasts indefinitely.
	ClusterInfrastructureDoesNotExistReason = ObjectDoesNotExistReason

	// ClusterInfrastructureDeletedReason surfaces when a referenced infrastructure object has been deleted.
	// Note: controllers can't identify if the infrastructure object was deleted by the controller itself, e.g.
	// during the deletion workflow, or by a users.
	ClusterInfrastructureDeletedReason = ObjectDeletedReason
)

// Cluster's ControlPlaneInitialized condition and corresponding reasons.
const (
	// ClusterControlPlaneInitializedCondition is true when the Cluster's control plane is functional enough
	// to accept requests. This information is usually used as a signal for starting all the provisioning operations
	// that depends on a functional API server, but do not require a full HA control plane to exists.
	// Note: Once set to true, this condition will never change.
	ClusterControlPlaneInitializedCondition = "ControlPlaneInitialized"

	// ClusterControlPlaneInitializedReason surfaces when the cluster control plane is initialized.
	ClusterControlPlaneInitializedReason = "Initialized"

	// ClusterControlPlaneNotInitializedReason surfaces when the cluster control plane is not yet initialized.
	ClusterControlPlaneNotInitializedReason = "NotInitialized"

	// ClusterControlPlaneInitializedInternalErrorReason surfaces unexpected failures when computing the
	// ControlPlaneInitialized condition.
	ClusterControlPlaneInitializedInternalErrorReason = InternalErrorReason
)

// Cluster's ControlPlaneAvailable condition and corresponding reasons.
const (
	// ClusterControlPlaneAvailableCondition is a mirror of Cluster's control plane Available condition.
	ClusterControlPlaneAvailableCondition = "ControlPlaneAvailable"

	// ClusterControlPlaneAvailableReason surfaces when the cluster control plane is available.
	ClusterControlPlaneAvailableReason = AvailableReason

	// ClusterControlPlaneNotAvailableReason surfaces when the cluster control plane is not available.
	ClusterControlPlaneNotAvailableReason = NotAvailableReason

	// ClusterControlPlaneInvalidConditionReportedReason surfaces a control plane Available condition (read from a control plane object) which is invalid.
	// (e.g. its status is missing).
	ClusterControlPlaneInvalidConditionReportedReason = InvalidConditionReportedReason

	// ClusterControlPlaneInternalErrorReason surfaces unexpected failures when reading a control plane object.
	ClusterControlPlaneInternalErrorReason = InternalErrorReason

	// ClusterControlPlaneDoesNotExistReason surfaces when a referenced control plane object does not exist.
	// Note: this could happen when creating the Cluster. However, this state should be treated as an error if it lasts indefinitely.
	ClusterControlPlaneDoesNotExistReason = ObjectDoesNotExistReason

	// ClusterControlPlaneDeletedReason surfaces when a referenced control plane object has been deleted.
	// Note: controllers can't identify if the control plane object was deleted by the controller itself, e.g.
	// during the deletion workflow, or by a users.
	ClusterControlPlaneDeletedReason = ObjectDeletedReason
)

// Cluster's WorkersAvailable condition and corresponding reasons.
const (
	// ClusterWorkersAvailableCondition is the summary of MachineDeployment and MachinePool's Available conditions.
	// Note: Stand-alone MachineSets and stand-alone Machines are not included in this condition.
	ClusterWorkersAvailableCondition = "WorkersAvailable"

	// ClusterWorkersAvailableReason surfaces when all  MachineDeployment and MachinePool's Available conditions are true.
	ClusterWorkersAvailableReason = AvailableReason

	// ClusterWorkersNotAvailableReason surfaces when at least one of the  MachineDeployment and MachinePool's Available
	// conditions is false.
	ClusterWorkersNotAvailableReason = NotAvailableReason

	// ClusterWorkersAvailableUnknownReason surfaces when at least one of the  MachineDeployment and MachinePool's Available
	// conditions is unknown and none of those Available conditions is false.
	ClusterWorkersAvailableUnknownReason = AvailableUnknownReason

	// ClusterWorkersAvailableNoWorkersReason surfaces when no MachineDeployment and MachinePool exist for the Cluster.
	ClusterWorkersAvailableNoWorkersReason = "NoWorkers"

	// ClusterWorkersAvailableInternalErrorReason surfaces unexpected failures when listing MachineDeployment and MachinePool
	// or aggregating conditions from those objects.
	ClusterWorkersAvailableInternalErrorReason = InternalErrorReason
)

// Cluster's ControlPlaneMachinesReady condition and corresponding reasons.
const (
	// ClusterControlPlaneMachinesReadyCondition surfaces detail of issues on control plane machines, if any.
	ClusterControlPlaneMachinesReadyCondition = "ControlPlaneMachinesReady"

	// ClusterControlPlaneMachinesReadyReason surfaces when all control plane machine's Ready conditions are true.
	ClusterControlPlaneMachinesReadyReason = ReadyReason

	// ClusterControlPlaneMachinesNotReadyReason surfaces when at least one of control plane machine's Ready conditions is false.
	ClusterControlPlaneMachinesNotReadyReason = NotReadyReason

	// ClusterControlPlaneMachinesReadyUnknownReason surfaces when at least one of control plane machine's Ready conditions is unknown
	// and none of control plane machine's Ready conditions is false.
	ClusterControlPlaneMachinesReadyUnknownReason = ReadyUnknownReason

	// ClusterControlPlaneMachinesReadyNoReplicasReason surfaces when no control plane machines exist for the Cluster.
	ClusterControlPlaneMachinesReadyNoReplicasReason = NoReplicasReason

	// ClusterControlPlaneMachinesReadyInternalErrorReason surfaces unexpected failures when listing control plane machines
	// or aggregating control plane machine's conditions.
	ClusterControlPlaneMachinesReadyInternalErrorReason = InternalErrorReason
)

// Cluster's WorkerMachinesReady condition and corresponding reasons.
const (
	// ClusterWorkerMachinesReadyCondition surfaces detail of issues on the worker machines, if any.
	ClusterWorkerMachinesReadyCondition = "WorkerMachinesReady"

	// ClusterWorkerMachinesReadyReason surfaces when all the worker machine's Ready conditions are true.
	ClusterWorkerMachinesReadyReason = ReadyReason

	// ClusterWorkerMachinesNotReadyReason surfaces when at least one of the worker machine's Ready conditions is false.
	ClusterWorkerMachinesNotReadyReason = NotReadyReason

	// ClusterWorkerMachinesReadyUnknownReason surfaces when at least one of the worker machine's Ready conditions is unknown
	// and none of the worker machine's Ready conditions is false.
	ClusterWorkerMachinesReadyUnknownReason = ReadyUnknownReason

	// ClusterWorkerMachinesReadyNoReplicasReason surfaces when no worker machines exist for the Cluster.
	ClusterWorkerMachinesReadyNoReplicasReason = NoReplicasReason

	// ClusterWorkerMachinesReadyInternalErrorReason surfaces unexpected failures when listing worker machines
	// or aggregating worker machine's conditions.
	ClusterWorkerMachinesReadyInternalErrorReason = InternalErrorReason
)

// Cluster's ControlPlaneMachinesUpToDate condition and corresponding reasons.
const (
	// ClusterControlPlaneMachinesUpToDateCondition surfaces details of control plane machines not up to date, if any.
	// Note: New machines are considered 10s after machine creation. This gives time to the machine's owner controller to recognize the new machine and add the UpToDate condition.
	ClusterControlPlaneMachinesUpToDateCondition = "ControlPlaneMachinesUpToDate"

	// ClusterControlPlaneMachinesUpToDateReason surfaces when all the control plane machine's UpToDate conditions are true.
	ClusterControlPlaneMachinesUpToDateReason = UpToDateReason

	// ClusterControlPlaneMachinesNotUpToDateReason surfaces when at least one of the control plane machine's UpToDate conditions is false.
	ClusterControlPlaneMachinesNotUpToDateReason = NotUpToDateReason

	// ClusterControlPlaneMachinesUpToDateUnknownReason surfaces when at least one of the control plane machine's UpToDate conditions is unknown
	// and none of the control plane machine's UpToDate conditions is false.
	ClusterControlPlaneMachinesUpToDateUnknownReason = UpToDateUnknownReason

	// ClusterControlPlaneMachinesUpToDateNoReplicasReason surfaces when no control plane machines exist for the Cluster.
	ClusterControlPlaneMachinesUpToDateNoReplicasReason = NoReplicasReason

	// ClusterControlPlaneMachinesUpToDateInternalErrorReason surfaces unexpected failures when listing control plane machines
	// or aggregating status.
	ClusterControlPlaneMachinesUpToDateInternalErrorReason = InternalErrorReason
)

// Cluster's WorkerMachinesUpToDate condition and corresponding reasons.
const (
	// ClusterWorkerMachinesUpToDateCondition surfaces details of worker machines not up to date, if any.
	// Note: New machines are considered 10s after machine creation. This gives time to the machine's owner controller to recognize the new machine and add the UpToDate condition.
	ClusterWorkerMachinesUpToDateCondition = "WorkerMachinesUpToDate"

	// ClusterWorkerMachinesUpToDateReason surfaces when all the worker machine's UpToDate conditions are true.
	ClusterWorkerMachinesUpToDateReason = UpToDateReason

	// ClusterWorkerMachinesNotUpToDateReason surfaces when at least one of the worker machine's UpToDate conditions is false.
	ClusterWorkerMachinesNotUpToDateReason = NotUpToDateReason

	// ClusterWorkerMachinesUpToDateUnknownReason surfaces when at least one of the worker machine's UpToDate conditions is unknown
	// and none of the worker machine's UpToDate conditions is false.
	ClusterWorkerMachinesUpToDateUnknownReason = UpToDateUnknownReason

	// ClusterWorkerMachinesUpToDateNoReplicasReason surfaces when no worker machines exist for the Cluster.
	ClusterWorkerMachinesUpToDateNoReplicasReason = NoReplicasReason

	// ClusterWorkerMachinesUpToDateInternalErrorReason surfaces unexpected failures when listing worker machines
	// or aggregating status.
	ClusterWorkerMachinesUpToDateInternalErrorReason = InternalErrorReason
)

// Cluster's RemoteConnectionProbe condition and corresponding reasons.
const (
	// ClusterRemoteConnectionProbeCondition is true when control plane can be reached; in case of connection problems.
	// The condition turns to false only if the cluster cannot be reached for 50s after the first connection problem
	// is detected (or whatever period is defined in the --remote-connection-grace-period flag).
	ClusterRemoteConnectionProbeCondition = "RemoteConnectionProbe"

	// ClusterRemoteConnectionProbeFailedReason surfaces issues with the connection to the workload cluster.
	ClusterRemoteConnectionProbeFailedReason = "ProbeFailed"

	// ClusterRemoteConnectionProbeSucceededReason is used to report a working connection with the workload cluster.
	ClusterRemoteConnectionProbeSucceededReason = "ProbeSucceeded"
)

// Cluster's RollingOut condition and corresponding reasons.
const (
	// ClusterRollingOutCondition is the summary of `RollingOut` conditions from ControlPlane, MachineDeployments
	// and MachinePools.
	ClusterRollingOutCondition = RollingOutCondition

	// ClusterRollingOutReason surfaces when at least one of the Cluster's control plane, MachineDeployments,
	// or MachinePools are rolling out.
	ClusterRollingOutReason = RollingOutReason

	// ClusterNotRollingOutReason surfaces when none of the Cluster's control plane, MachineDeployments,
	// or MachinePools are rolling out.
	ClusterNotRollingOutReason = NotRollingOutReason

	// ClusterRollingOutUnknownReason surfaces when one of the Cluster's control plane, MachineDeployments,
	// or MachinePools rolling out condition is unknown, and none true.
	ClusterRollingOutUnknownReason = "RollingOutUnknown"

	// ClusterRollingOutInternalErrorReason surfaces unexpected failures when listing machines
	// or computing the RollingOut condition.
	ClusterRollingOutInternalErrorReason = InternalErrorReason
)

// Cluster's ScalingUp condition and corresponding reasons.
const (
	// ClusterScalingUpCondition is the summary of `ScalingUp` conditions from ControlPlane, MachineDeployments,
	// MachinePools and stand-alone MachineSets.
	ClusterScalingUpCondition = ScalingUpCondition

	// ClusterScalingUpReason surfaces when at least one of the Cluster's control plane, MachineDeployments,
	// MachinePools and stand-alone MachineSets are scaling up.
	ClusterScalingUpReason = ScalingUpReason

	// ClusterNotScalingUpReason surfaces when none of the Cluster's control plane, MachineDeployments,
	// MachinePools and stand-alone MachineSets are scaling up.
	ClusterNotScalingUpReason = NotScalingUpReason

	// ClusterScalingUpUnknownReason surfaces when one of the Cluster's control plane, MachineDeployments,
	// MachinePools and stand-alone MachineSets scaling up condition is unknown, and none true.
	ClusterScalingUpUnknownReason = "ScalingUpUnknown"

	// ClusterScalingUpInternalErrorReason surfaces unexpected failures when listing machines
	// or computing the ScalingUp condition.
	ClusterScalingUpInternalErrorReason = InternalErrorReason
)

// Cluster's ScalingDown condition and corresponding reasons.
const (
	// ClusterScalingDownCondition is the summary of `ScalingDown` conditions from ControlPlane, MachineDeployments,
	// MachinePools and stand-alone MachineSets.
	ClusterScalingDownCondition = ScalingDownCondition

	// ClusterScalingDownReason surfaces when at least one of the Cluster's control plane, MachineDeployments,
	// MachinePools and stand-alone MachineSets are scaling down.
	ClusterScalingDownReason = ScalingDownReason

	// ClusterNotScalingDownReason surfaces when none of the Cluster's control plane, MachineDeployments,
	// MachinePools and stand-alone MachineSets are scaling down.
	ClusterNotScalingDownReason = NotScalingDownReason

	// ClusterScalingDownUnknownReason surfaces when one of the Cluster's control plane, MachineDeployments,
	// MachinePools and stand-alone MachineSets scaling down condition is unknown, and none true.
	ClusterScalingDownUnknownReason = "ScalingDownUnknown"

	// ClusterScalingDownInternalErrorReason surfaces unexpected failures when listing machines
	// or computing the ScalingDown condition.
	ClusterScalingDownInternalErrorReason = InternalErrorReason
)

// Cluster's Remediating condition and corresponding reasons.
const (
	// ClusterRemediatingCondition surfaces details about ongoing remediation of the controlled machines, if any.
	ClusterRemediatingCondition = RemediatingCondition

	// ClusterRemediatingReason surfaces when the Cluster has at least one machine with HealthCheckSucceeded set to false
	// and with the OwnerRemediated condition set to false.
	ClusterRemediatingReason = RemediatingReason

	// ClusterNotRemediatingReason surfaces when the Cluster does not have any machine with HealthCheckSucceeded set to false
	// and with the OwnerRemediated condition set to false.
	ClusterNotRemediatingReason = NotRemediatingReason

	// ClusterRemediatingInternalErrorReason surfaces unexpected failures when computing the Remediating condition.
	ClusterRemediatingInternalErrorReason = InternalErrorReason
)

// Cluster's Deleting condition and corresponding reasons.
const (
	// ClusterDeletingCondition surfaces details about ongoing deletion of the cluster.
	ClusterDeletingCondition = DeletingCondition

	// ClusterNotDeletingReason surfaces when the Cluster is not deleting because the
	// DeletionTimestamp is not set.
	ClusterNotDeletingReason = NotDeletingReason

	// ClusterDeletingWaitingForBeforeDeleteHookReason surfaces when the Cluster deletion
	// waits for the ClusterDelete hooks to allow deletion to complete.
	ClusterDeletingWaitingForBeforeDeleteHookReason = "WaitingForBeforeDeleteHook"

	// ClusterDeletingWaitingForWorkersDeletionReason surfaces when the Cluster deletion
	// waits for the workers Machines and the object controlling those machines (MachinePools, MachineDeployments, MachineSets)
	// to be deleted.
	ClusterDeletingWaitingForWorkersDeletionReason = "WaitingForWorkersDeletion"

	// ClusterDeletingWaitingForControlPlaneDeletionReason surfaces when the Cluster deletion
	// waits for the ControlPlane to be deleted.
	ClusterDeletingWaitingForControlPlaneDeletionReason = "WaitingForControlPlaneDeletion"

	// ClusterDeletingWaitingForInfrastructureDeletionReason surfaces when the Cluster deletion
	// waits for the InfraCluster to be deleted.
	ClusterDeletingWaitingForInfrastructureDeletionReason = "WaitingForInfrastructureDeletion"

	// ClusterDeletingDeletionCompletedReason surfaces when the Cluster deletion has been completed.
	// This reason is set right after the `cluster.cluster.x-k8s.io` finalizer is removed.
	// This means that the object will go away (i.e. be removed from etcd), except if there are other
	// finalizers on the Cluster object.
	ClusterDeletingDeletionCompletedReason = DeletionCompletedReason

	// ClusterDeletingInternalErrorReason surfaces unexpected failures when deleting a cluster.
	ClusterDeletingInternalErrorReason = InternalErrorReason
)

// ANCHOR: ClusterSpec

// ClusterSpec defines the desired state of Cluster.
type ClusterSpec struct {
	// paused can be used to prevent controllers from processing the Cluster and all its associated objects.
	// +optional
	Paused *bool `json:"paused,omitempty"`

	// clusterNetwork represents the cluster network configuration.
	// +optional
	ClusterNetwork *ClusterNetwork `json:"clusterNetwork,omitempty"`

	// controlPlaneEndpoint represents the endpoint used to communicate with the control plane.
	// +optional
	ControlPlaneEndpoint APIEndpoint `json:"controlPlaneEndpoint,omitempty"`

	// controlPlaneRef is an optional reference to a provider-specific resource that holds
	// the details for provisioning the Control Plane for a Cluster.
	// +optional
	ControlPlaneRef *ContractVersionedObjectReference `json:"controlPlaneRef,omitempty"`

	// infrastructureRef is a reference to a provider-specific resource that holds the details
	// for provisioning infrastructure for a cluster in said provider.
	// +optional
	InfrastructureRef *ContractVersionedObjectReference `json:"infrastructureRef,omitempty"`

	// topology encapsulates the topology for the cluster.
	// NOTE: It is required to enable the ClusterTopology
	// feature gate flag to activate managed topologies support;
	// this feature is highly experimental, and parts of it might still be not implemented.
	// +optional
	Topology *Topology `json:"topology,omitempty"`

	// availabilityGates specifies additional conditions to include when evaluating Cluster Available condition.
	//
	// If this field is not defined and the Cluster implements a managed topology, availabilityGates
	// from the corresponding ClusterClass will be used, if any.
	//
	// +optional
	// +listType=map
	// +listMapKey=conditionType
	// +kubebuilder:validation:MaxItems=32
	AvailabilityGates []ClusterAvailabilityGate `json:"availabilityGates,omitempty"`
}

// ConditionPolarity defines the polarity for a metav1.Condition.
type ConditionPolarity string

const (
	// PositivePolarityCondition describe a condition with positive polarity, a condition
	// where the normal state is True. e.g. NetworkReady.
	PositivePolarityCondition ConditionPolarity = "Positive"

	// NegativePolarityCondition describe a condition with negative polarity, a condition
	// where the normal state is False. e.g. MemoryPressure.
	NegativePolarityCondition ConditionPolarity = "Negative"
)

// ClusterAvailabilityGate contains the type of a Cluster condition to be used as availability gate.
type ClusterAvailabilityGate struct {
	// conditionType refers to a condition with matching type in the Cluster's condition list.
	// If the conditions doesn't exist, it will be treated as unknown.
	// Note: Both Cluster API conditions or conditions added by 3rd party controllers can be used as availability gates.
	// +required
	// +kubebuilder:validation:Pattern=`^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*/)?(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])$`
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=316
	ConditionType string `json:"conditionType"`

	// polarity of the conditionType specified in this availabilityGate.
	// Valid values are Positive, Negative and omitted.
	// When omitted, the default behaviour will be Positive.
	// A positive polarity means that the condition should report a true status under normal conditions.
	// A negative polarity means that the condition should report a false status under normal conditions.
	// +kubebuilder:validation:Enum=Positive;Negative
	// +optional
	Polarity ConditionPolarity `json:"polarity,omitempty"`
}

// Topology encapsulates the information of the managed resources.
type Topology struct {
	// classRef is the ref to the ClusterClass that should be used for the topology.
	// +required
	ClassRef ClusterClassRef `json:"classRef"`

	// version is the Kubernetes version of the cluster.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Version string `json:"version"`

	// controlPlane describes the cluster control plane.
	// +optional
	ControlPlane ControlPlaneTopology `json:"controlPlane,omitempty"`

	// workers encapsulates the different constructs that form the worker nodes
	// for the cluster.
	// +optional
	Workers *WorkersTopology `json:"workers,omitempty"`

	// variables can be used to customize the Cluster through
	// patches. They must comply to the corresponding
	// VariableClasses defined in the ClusterClass.
	// +optional
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MaxItems=1000
	Variables []ClusterVariable `json:"variables,omitempty"`
}

// ClusterClassRef is the ref to the ClusterClass that should be used for the topology.
type ClusterClassRef struct {
	// name is the name of the ClusterClass that should be used for the topology.
	// name must be a valid ClusterClass name and because of that be at most 253 characters in length
	// and it must consist only of lower case alphanumeric characters, hyphens (-) and periods (.), and must start
	// and end with an alphanumeric character.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
	Name string `json:"name"`

	// namespace is the namespace of the ClusterClass that should be used for the topology.
	// If namespace is empty or not set, it is defaulted to the namespace of the Cluster object.
	// namespace must be a valid namespace name and because of that be at most 63 characters in length
	// and it must consist only of lower case alphanumeric characters or hyphens (-), and must start
	// and end with an alphanumeric character.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	Namespace string `json:"namespace,omitempty"`
}

// ControlPlaneTopology specifies the parameters for the control plane nodes in the cluster.
type ControlPlaneTopology struct {
	// metadata is the metadata applied to the ControlPlane and the Machines of the ControlPlane
	// if the ControlPlaneTemplate referenced by the ClusterClass is machine based. If not, it
	// is applied only to the ControlPlane.
	// At runtime this metadata is merged with the corresponding metadata from the ClusterClass.
	// +optional
	Metadata ObjectMeta `json:"metadata,omitempty"`

	// replicas is the number of control plane nodes.
	// If the value is nil, the ControlPlane object is created without the number of Replicas
	// and it's assumed that the control plane controller does not implement support for this field.
	// When specified against a control plane provider that lacks support for this field, this value will be ignored.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// machineHealthCheck allows to enable, disable and override
	// the MachineHealthCheck configuration in the ClusterClass for this control plane.
	// +optional
	MachineHealthCheck *MachineHealthCheckTopology `json:"machineHealthCheck,omitempty"`

	// nodeDrainTimeoutSeconds is the total amount of time that the controller will spend on draining a node.
	// The default value is 0, meaning that the node can be drained without any time limitations.
	// NOTE: nodeDrainTimeoutSeconds is different from `kubectl drain --timeout`
	// +optional
	// +kubebuilder:validation:Minimum=0
	NodeDrainTimeoutSeconds *int32 `json:"nodeDrainTimeoutSeconds,omitempty"`

	// nodeVolumeDetachTimeoutSeconds is the total amount of time that the controller will spend on waiting for all volumes
	// to be detached. The default value is 0, meaning that the volumes can be detached without any time limitations.
	// +optional
	// +kubebuilder:validation:Minimum=0
	NodeVolumeDetachTimeoutSeconds *int32 `json:"nodeVolumeDetachTimeoutSeconds,omitempty"`

	// nodeDeletionTimeoutSeconds defines how long the controller will attempt to delete the Node that the Machine
	// hosts after the Machine is marked for deletion. A duration of 0 will retry deletion indefinitely.
	// Defaults to 10 seconds.
	// +optional
	// +kubebuilder:validation:Minimum=0
	NodeDeletionTimeoutSeconds *int32 `json:"nodeDeletionTimeoutSeconds,omitempty"`

	// readinessGates specifies additional conditions to include when evaluating Machine Ready condition.
	//
	// This field can be used e.g. to instruct the machine controller to include in the computation for Machine's ready
	// computation a condition, managed by an external controllers, reporting the status of special software/hardware installed on the Machine.
	//
	// If this field is not defined, readinessGates from the corresponding ControlPlaneClass will be used, if any.
	//
	// NOTE: This field is considered only for computing v1beta2 conditions.
	// NOTE: Specific control plane provider implementations might automatically extend the list of readinessGates;
	// e.g. the kubeadm control provider adds ReadinessGates for the APIServerPodHealthy, SchedulerPodHealthy conditions, etc.
	// +optional
	// +listType=map
	// +listMapKey=conditionType
	// +kubebuilder:validation:MaxItems=32
	ReadinessGates []MachineReadinessGate `json:"readinessGates,omitempty"`

	// variables can be used to customize the ControlPlane through patches.
	// +optional
	Variables *ControlPlaneVariables `json:"variables,omitempty"`
}

// WorkersTopology represents the different sets of worker nodes in the cluster.
type WorkersTopology struct {
	// machineDeployments is a list of machine deployments in the cluster.
	// +optional
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MaxItems=2000
	MachineDeployments []MachineDeploymentTopology `json:"machineDeployments,omitempty"`

	// machinePools is a list of machine pools in the cluster.
	// +optional
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MaxItems=2000
	MachinePools []MachinePoolTopology `json:"machinePools,omitempty"`
}

// MachineDeploymentTopology specifies the different parameters for a set of worker nodes in the topology.
// This set of nodes is managed by a MachineDeployment object whose lifecycle is managed by the Cluster controller.
type MachineDeploymentTopology struct {
	// metadata is the metadata applied to the MachineDeployment and the machines of the MachineDeployment.
	// At runtime this metadata is merged with the corresponding metadata from the ClusterClass.
	// +optional
	Metadata ObjectMeta `json:"metadata,omitempty"`

	// class is the name of the MachineDeploymentClass used to create the set of worker nodes.
	// This should match one of the deployment classes defined in the ClusterClass object
	// mentioned in the `Cluster.Spec.Class` field.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Class string `json:"class"`

	// name is the unique identifier for this MachineDeploymentTopology.
	// The value is used with other unique identifiers to create a MachineDeployment's Name
	// (e.g. cluster's name, etc). In case the name is greater than the allowed maximum length,
	// the values are hashed together.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	Name string `json:"name"`

	// failureDomain is the failure domain the machines will be created in.
	// Must match a key in the FailureDomains map stored on the cluster object.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	FailureDomain string `json:"failureDomain,omitempty"`

	// replicas is the number of worker nodes belonging to this set.
	// If the value is nil, the MachineDeployment is created without the number of Replicas (defaulting to 1)
	// and it's assumed that an external entity (like cluster autoscaler) is responsible for the management
	// of this value.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// machineHealthCheck allows to enable, disable and override
	// the MachineHealthCheck configuration in the ClusterClass for this MachineDeployment.
	// +optional
	MachineHealthCheck *MachineHealthCheckTopology `json:"machineHealthCheck,omitempty"`

	// nodeDrainTimeoutSeconds is the total amount of time that the controller will spend on draining a node.
	// The default value is 0, meaning that the node can be drained without any time limitations.
	// NOTE: nodeDrainTimeoutSeconds is different from `kubectl drain --timeout`
	// +optional
	// +kubebuilder:validation:Minimum=0
	NodeDrainTimeoutSeconds *int32 `json:"nodeDrainTimeoutSeconds,omitempty"`

	// nodeVolumeDetachTimeoutSeconds is the total amount of time that the controller will spend on waiting for all volumes
	// to be detached. The default value is 0, meaning that the volumes can be detached without any time limitations.
	// +optional
	// +kubebuilder:validation:Minimum=0
	NodeVolumeDetachTimeoutSeconds *int32 `json:"nodeVolumeDetachTimeoutSeconds,omitempty"`

	// nodeDeletionTimeoutSeconds defines how long the controller will attempt to delete the Node that the Machine
	// hosts after the Machine is marked for deletion. A duration of 0 will retry deletion indefinitely.
	// Defaults to 10 seconds.
	// +optional
	// +kubebuilder:validation:Minimum=0
	NodeDeletionTimeoutSeconds *int32 `json:"nodeDeletionTimeoutSeconds,omitempty"`

	// minReadySeconds is the minimum number of seconds for which a newly created machine should
	// be ready.
	// Defaults to 0 (machine will be considered available as soon as it
	// is ready)
	// +optional
	MinReadySeconds *int32 `json:"minReadySeconds,omitempty"`

	// readinessGates specifies additional conditions to include when evaluating Machine Ready condition.
	//
	// This field can be used e.g. to instruct the machine controller to include in the computation for Machine's ready
	// computation a condition, managed by an external controllers, reporting the status of special software/hardware installed on the Machine.
	//
	// If this field is not defined, readinessGates from the corresponding MachineDeploymentClass will be used, if any.
	//
	// NOTE: This field is considered only for computing v1beta2 conditions.
	// +optional
	// +listType=map
	// +listMapKey=conditionType
	// +kubebuilder:validation:MaxItems=32
	ReadinessGates []MachineReadinessGate `json:"readinessGates,omitempty"`

	// strategy is the deployment strategy to use to replace existing machines with
	// new ones.
	// +optional
	Strategy *MachineDeploymentStrategy `json:"strategy,omitempty"`

	// variables can be used to customize the MachineDeployment through patches.
	// +optional
	Variables *MachineDeploymentVariables `json:"variables,omitempty"`
}

// MachineHealthCheckTopology defines a MachineHealthCheck for a group of machines.
type MachineHealthCheckTopology struct {
	// enable controls if a MachineHealthCheck should be created for the target machines.
	//
	// If false: No MachineHealthCheck will be created.
	//
	// If not set(default): A MachineHealthCheck will be created if it is defined here or
	//  in the associated ClusterClass. If no MachineHealthCheck is defined then none will be created.
	//
	// If true: A MachineHealthCheck is guaranteed to be created. Cluster validation will
	// block if `enable` is true and no MachineHealthCheck definition is available.
	// +optional
	Enable *bool `json:"enable,omitempty"`

	// MachineHealthCheckClass defines a MachineHealthCheck for a group of machines.
	// If specified (any field is set), it entirely overrides the MachineHealthCheckClass defined in ClusterClass.
	MachineHealthCheckClass `json:",inline"`
}

// MachinePoolTopology specifies the different parameters for a pool of worker nodes in the topology.
// This pool of nodes is managed by a MachinePool object whose lifecycle is managed by the Cluster controller.
type MachinePoolTopology struct {
	// metadata is the metadata applied to the MachinePool.
	// At runtime this metadata is merged with the corresponding metadata from the ClusterClass.
	// +optional
	Metadata ObjectMeta `json:"metadata,omitempty"`

	// class is the name of the MachinePoolClass used to create the pool of worker nodes.
	// This should match one of the deployment classes defined in the ClusterClass object
	// mentioned in the `Cluster.Spec.Class` field.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Class string `json:"class"`

	// name is the unique identifier for this MachinePoolTopology.
	// The value is used with other unique identifiers to create a MachinePool's Name
	// (e.g. cluster's name, etc). In case the name is greater than the allowed maximum length,
	// the values are hashed together.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	Name string `json:"name"`

	// failureDomains is the list of failure domains the machine pool will be created in.
	// Must match a key in the FailureDomains map stored on the cluster object.
	// +optional
	// +kubebuilder:validation:MaxItems=100
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=256
	FailureDomains []string `json:"failureDomains,omitempty"`

	// nodeDrainTimeoutSeconds is the total amount of time that the controller will spend on draining a node.
	// The default value is 0, meaning that the node can be drained without any time limitations.
	// NOTE: nodeDrainTimeoutSeconds is different from `kubectl drain --timeout`
	// +optional
	// +kubebuilder:validation:Minimum=0
	NodeDrainTimeoutSeconds *int32 `json:"nodeDrainTimeoutSeconds,omitempty"`

	// nodeVolumeDetachTimeoutSeconds is the total amount of time that the controller will spend on waiting for all volumes
	// to be detached. The default value is 0, meaning that the volumes can be detached without any time limitations.
	// +optional
	// +kubebuilder:validation:Minimum=0
	NodeVolumeDetachTimeoutSeconds *int32 `json:"nodeVolumeDetachTimeoutSeconds,omitempty"`

	// nodeDeletionTimeoutSeconds defines how long the controller will attempt to delete the Node that the MachinePool
	// hosts after the MachinePool is marked for deletion. A duration of 0 will retry deletion indefinitely.
	// Defaults to 10 seconds.
	// +optional
	// +kubebuilder:validation:Minimum=0
	NodeDeletionTimeoutSeconds *int32 `json:"nodeDeletionTimeoutSeconds,omitempty"`

	// minReadySeconds is the minimum number of seconds for which a newly created machine pool should
	// be ready.
	// Defaults to 0 (machine will be considered available as soon as it
	// is ready)
	// +optional
	MinReadySeconds *int32 `json:"minReadySeconds,omitempty"`

	// replicas is the number of nodes belonging to this pool.
	// If the value is nil, the MachinePool is created without the number of Replicas (defaulting to 1)
	// and it's assumed that an external entity (like cluster autoscaler) is responsible for the management
	// of this value.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// variables can be used to customize the MachinePool through patches.
	// +optional
	Variables *MachinePoolVariables `json:"variables,omitempty"`
}

// ClusterVariable can be used to customize the Cluster through patches. Each ClusterVariable is associated with a
// Variable definition in the ClusterClass `status` variables.
type ClusterVariable struct {
	// name of the variable.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Name string `json:"name"`

	// value of the variable.
	// Note: the value will be validated against the schema of the corresponding ClusterClassVariable
	// from the ClusterClass.
	// Note: We have to use apiextensionsv1.JSON instead of a custom JSON type, because controller-tools has a
	// hard-coded schema for apiextensionsv1.JSON which cannot be produced by another type via controller-tools,
	// i.e. it is not possible to have no type field.
	// Ref: https://github.com/kubernetes-sigs/controller-tools/blob/d0e03a142d0ecdd5491593e941ee1d6b5d91dba6/pkg/crd/known_types.go#L106-L111
	// +required
	Value apiextensionsv1.JSON `json:"value"`
}

// ControlPlaneVariables can be used to provide variables for the ControlPlane.
type ControlPlaneVariables struct {
	// overrides can be used to override Cluster level variables.
	// +optional
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MaxItems=1000
	Overrides []ClusterVariable `json:"overrides,omitempty"`
}

// MachineDeploymentVariables can be used to provide variables for a specific MachineDeployment.
type MachineDeploymentVariables struct {
	// overrides can be used to override Cluster level variables.
	// +optional
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MaxItems=1000
	Overrides []ClusterVariable `json:"overrides,omitempty"`
}

// MachinePoolVariables can be used to provide variables for a specific MachinePool.
type MachinePoolVariables struct {
	// overrides can be used to override Cluster level variables.
	// +optional
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MaxItems=1000
	Overrides []ClusterVariable `json:"overrides,omitempty"`
}

// ANCHOR_END: ClusterSpec

// ANCHOR: ClusterNetwork

// ClusterNetwork specifies the different networking
// parameters for a cluster.
type ClusterNetwork struct {
	// apiServerPort specifies the port the API Server should bind to.
	// Defaults to 6443.
	// +optional
	APIServerPort *int32 `json:"apiServerPort,omitempty"`

	// services is the network ranges from which service VIPs are allocated.
	// +optional
	Services *NetworkRanges `json:"services,omitempty"`

	// pods is the network ranges from which Pod networks are allocated.
	// +optional
	Pods *NetworkRanges `json:"pods,omitempty"`

	// serviceDomain is the domain name for services.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	ServiceDomain string `json:"serviceDomain,omitempty"`
}

// ANCHOR_END: ClusterNetwork

// ANCHOR: NetworkRanges

// NetworkRanges represents ranges of network addresses.
type NetworkRanges struct {
	// cidrBlocks is a list of CIDR blocks.
	// +required
	// +kubebuilder:validation:MaxItems=100
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=43
	CIDRBlocks []string `json:"cidrBlocks"`
}

func (n NetworkRanges) String() string {
	if len(n.CIDRBlocks) == 0 {
		return ""
	}
	return strings.Join(n.CIDRBlocks, ",")
}

// ANCHOR_END: NetworkRanges

// ANCHOR: ClusterStatus

// ClusterStatus defines the observed state of Cluster.
type ClusterStatus struct {
	// conditions represents the observations of a Cluster's current state.
	// Known condition types are Available, InfrastructureReady, ControlPlaneInitialized, ControlPlaneAvailable, WorkersAvailable, MachinesReady
	// MachinesUpToDate, RemoteConnectionProbe, ScalingUp, ScalingDown, Remediating, Deleting, Paused.
	// Additionally, a TopologyReconciled condition will be added in case the Cluster is referencing a ClusterClass / defining a managed Topology.
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=32
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// initialization provides observations of the Cluster initialization process.
	// NOTE: Fields in this struct are part of the Cluster API contract and are used to orchestrate initial Cluster provisioning.
	// +optional
	Initialization *ClusterInitializationStatus `json:"initialization,omitempty"`

	// controlPlane groups all the observations about Cluster's ControlPlane current state.
	// +optional
	ControlPlane *ClusterControlPlaneStatus `json:"controlPlane,omitempty"`

	// workers groups all the observations about Cluster's Workers current state.
	// +optional
	Workers *WorkersStatus `json:"workers,omitempty"`

	// failureDomains is a slice of failure domain objects synced from the infrastructure provider.
	// +optional
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	FailureDomains []FailureDomain `json:"failureDomains,omitempty"`

	// phase represents the current phase of cluster actuation.
	// +optional
	// +kubebuilder:validation:Enum=Pending;Provisioning;Provisioned;Deleting;Failed;Unknown
	Phase string `json:"phase,omitempty"`

	// observedGeneration is the latest generation observed by the controller.
	// +optional
	// +kubebuilder:validation:Minimum=1
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// deprecated groups all the status fields that are deprecated and will be removed when all the nested field are removed.
	// +optional
	Deprecated *ClusterDeprecatedStatus `json:"deprecated,omitempty"`
}

// ClusterInitializationStatus provides observations of the Cluster initialization process.
// NOTE: Fields in this struct are part of the Cluster API contract and are used to orchestrate initial Cluster provisioning.

// ClusterInitializationStatus provides observations of the Cluster initialization process.
type ClusterInitializationStatus struct {
	// infrastructureProvisioned is true when the infrastructure provider reports that Cluster's infrastructure is fully provisioned.
	// NOTE: this field is part of the Cluster API contract, and it is used to orchestrate provisioning.
	// The value of this field is never updated after provisioning is completed.
	// +optional
	InfrastructureProvisioned *bool `json:"infrastructureProvisioned,omitempty"`

	// controlPlaneInitialized denotes when the control plane is functional enough to accept requests.
	// This information is usually used as a signal for starting all the provisioning operations that depends on
	// a functional API server, but do not require a full HA control plane to exists, like e.g. join worker Machines,
	// install core addons like CNI, CPI, CSI etc.
	// NOTE: this field is part of the Cluster API contract, and it is used to orchestrate provisioning.
	// The value of this field is never updated after initialization is completed.
	// +optional
	ControlPlaneInitialized *bool `json:"controlPlaneInitialized,omitempty"`
}

// ClusterDeprecatedStatus groups all the status fields that are deprecated and will be removed in a future version.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type ClusterDeprecatedStatus struct {
	// v1beta1 groups all the status fields that are deprecated and will be removed when support for v1beta1 will be dropped.
	// +optional
	V1Beta1 *ClusterV1Beta1DeprecatedStatus `json:"v1beta1,omitempty"`
}

// ClusterV1Beta1DeprecatedStatus groups all the status fields that are deprecated and will be removed when support for v1beta1 will be dropped.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type ClusterV1Beta1DeprecatedStatus struct {
	// conditions defines current service state of the cluster.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	Conditions Conditions `json:"conditions,omitempty"`

	// failureReason indicates that there is a fatal problem reconciling the
	// state, and will be set to a token value suitable for
	// programmatic interpretation.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	FailureReason *capierrors.ClusterStatusError `json:"failureReason,omitempty"`

	// failureMessage indicates that there is a fatal problem reconciling the
	// state, and will be set to a descriptive error message.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=10240
	FailureMessage *string `json:"failureMessage,omitempty"` //nolint:kubeapilinter // field will be removed when v1beta1 is removed
}

// ClusterControlPlaneStatus groups all the observations about control plane current state.
type ClusterControlPlaneStatus struct {
	// desiredReplicas is the total number of desired control plane machines in this cluster.
	// +optional
	DesiredReplicas *int32 `json:"desiredReplicas,omitempty"`

	// replicas is the total number of control plane machines in this cluster.
	// NOTE: replicas also includes machines still being provisioned or being deleted.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// upToDateReplicas is the number of up-to-date control plane machines in this cluster. A machine is considered up-to-date when Machine's UpToDate condition is true.
	// +optional
	UpToDateReplicas *int32 `json:"upToDateReplicas,omitempty"`

	// readyReplicas is the total number of ready control plane machines in this cluster. A machine is considered ready when Machine's Ready condition is true.
	// +optional
	ReadyReplicas *int32 `json:"readyReplicas,omitempty"`

	// availableReplicas is the total number of available control plane machines in this cluster. A machine is considered available when Machine's Available condition is true.
	// +optional
	AvailableReplicas *int32 `json:"availableReplicas,omitempty"`
}

// WorkersStatus groups all the observations about workers current state.
type WorkersStatus struct {
	// desiredReplicas is the total number of desired worker machines in this cluster.
	// +optional
	DesiredReplicas *int32 `json:"desiredReplicas,omitempty"`

	// replicas is the total number of worker machines in this cluster.
	// NOTE: replicas also includes machines still being provisioned or being deleted.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// upToDateReplicas is the number of up-to-date worker machines in this cluster. A machine is considered up-to-date when Machine's UpToDate condition is true.
	// +optional
	UpToDateReplicas *int32 `json:"upToDateReplicas,omitempty"`

	// readyReplicas is the total number of ready worker machines in this cluster. A machine is considered ready when Machine's Ready condition is true.
	// +optional
	ReadyReplicas *int32 `json:"readyReplicas,omitempty"`

	// availableReplicas is the total number of available worker machines in this cluster. A machine is considered available when Machine's Available condition is true.
	// +optional
	AvailableReplicas *int32 `json:"availableReplicas,omitempty"`
}

// ANCHOR_END: ClusterStatus

// SetTypedPhase sets the Phase field to the string representation of ClusterPhase.
func (c *ClusterStatus) SetTypedPhase(p ClusterPhase) {
	c.Phase = string(p)
}

// GetTypedPhase attempts to parse the Phase field and return
// the typed ClusterPhase representation as described in `machine_phase_types.go`.
func (c *ClusterStatus) GetTypedPhase() ClusterPhase {
	switch phase := ClusterPhase(c.Phase); phase {
	case
		ClusterPhasePending,
		ClusterPhaseProvisioning,
		ClusterPhaseProvisioned,
		ClusterPhaseDeleting,
		ClusterPhaseFailed:
		return phase
	default:
		return ClusterPhaseUnknown
	}
}

// ANCHOR: APIEndpoint

// APIEndpoint represents a reachable Kubernetes API endpoint.
type APIEndpoint struct {
	// host is the hostname on which the API server is serving.
	// TODO: Can't set MinLength=1 for now, because this struct is not always used in pointer fields so today we have cases where host is set to an empty string.
	// +required
	// +kubebuilder:validation:MaxLength=512
	Host string `json:"host"`

	// port is the port on which the API server is serving.
	// +required
	Port int32 `json:"port"`
}

// IsZero returns true if both host and port are zero values.
func (v APIEndpoint) IsZero() bool {
	return v.Host == "" && v.Port == 0
}

// IsValid returns true if both host and port are non-zero values.
func (v APIEndpoint) IsValid() bool {
	return v.Host != "" && v.Port != 0
}

// String returns a formatted version HOST:PORT of this APIEndpoint.
func (v APIEndpoint) String() string {
	return net.JoinHostPort(v.Host, fmt.Sprintf("%d", v.Port))
}

// ANCHOR_END: APIEndpoint

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=clusters,shortName=cl,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="ClusterClass",type="string",JSONPath=".spec.topology.class",description="ClusterClass of this Cluster, empty if the Cluster is not using a ClusterClass"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase",description="Cluster status such as Pending/Provisioning/Provisioned/Deleting/Failed"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of Cluster"
// +kubebuilder:printcolumn:name="Version",type="string",JSONPath=".spec.topology.version",description="Kubernetes version associated with this Cluster"

// Cluster is the Schema for the clusters API.
type Cluster struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec is the desired state of Cluster.
	// +optional
	Spec ClusterSpec `json:"spec,omitempty"`
	// status is the observed state of Cluster.
	// +optional
	Status ClusterStatus `json:"status,omitempty"`
}

// GetClassKey returns the namespaced name for the class associated with this object.
func (c *Cluster) GetClassKey() types.NamespacedName {
	if c.Spec.Topology == nil {
		return types.NamespacedName{}
	}

	namespace := cmp.Or(c.Spec.Topology.ClassRef.Namespace, c.Namespace)
	return types.NamespacedName{Namespace: namespace, Name: c.Spec.Topology.ClassRef.Name}
}

// GetV1Beta1Conditions returns the set of conditions for this object.
func (c *Cluster) GetV1Beta1Conditions() Conditions {
	if c.Status.Deprecated == nil || c.Status.Deprecated.V1Beta1 == nil {
		return nil
	}
	return c.Status.Deprecated.V1Beta1.Conditions
}

// SetV1Beta1Conditions sets the conditions on this object.
func (c *Cluster) SetV1Beta1Conditions(conditions Conditions) {
	if c.Status.Deprecated == nil {
		c.Status.Deprecated = &ClusterDeprecatedStatus{}
	}
	if c.Status.Deprecated.V1Beta1 == nil {
		c.Status.Deprecated.V1Beta1 = &ClusterV1Beta1DeprecatedStatus{}
	}
	c.Status.Deprecated.V1Beta1.Conditions = conditions
}

// GetConditions returns the set of conditions for this object.
func (c *Cluster) GetConditions() []metav1.Condition {
	return c.Status.Conditions
}

// SetConditions sets conditions for an API object.
func (c *Cluster) SetConditions(conditions []metav1.Condition) {
	c.Status.Conditions = conditions
}

// +kubebuilder:object:root=true

// ClusterList contains a list of Cluster.
type ClusterList struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard list's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#lists-and-simple-kinds
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`
	// items is the list of Clusters.
	Items []Cluster `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &Cluster{}, &ClusterList{})
}

// FailureDomain is the Schema for Cluster API failure domains.
// It allows controllers to understand how many failure domains a cluster can optionally span across.
type FailureDomain struct {
	// name is the name of the failure domain.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Name string `json:"name"`

	// controlPlane determines if this failure domain is suitable for use by control plane machines.
	// +optional
	ControlPlane *bool `json:"controlPlane,omitempty"`

	// attributes is a free form map of attributes an infrastructure provider might use or require.
	// +optional
	Attributes map[string]string `json:"attributes,omitempty"`
}
