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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	capierrors "sigs.k8s.io/cluster-api/errors"
)

const (
	// MachineFinalizer is set on PrepareForCreate callback.
	MachineFinalizer = "machine.cluster.x-k8s.io"

	// MachineControlPlaneLabel is the label set on machines or related objects that are part of a control plane.
	MachineControlPlaneLabel = "cluster.x-k8s.io/control-plane"

	// ExcludeNodeDrainingAnnotation annotation explicitly skips node draining if set.
	ExcludeNodeDrainingAnnotation = "machine.cluster.x-k8s.io/exclude-node-draining"

	// ExcludeWaitForNodeVolumeDetachAnnotation annotation explicitly skips the waiting for node volume detaching if set.
	ExcludeWaitForNodeVolumeDetachAnnotation = "machine.cluster.x-k8s.io/exclude-wait-for-node-volume-detach"

	// MachineSetNameLabel is the label set on machines if they're controlled by MachineSet.
	// Note: The value of this label may be a hash if the MachineSet name is longer than 63 characters.
	MachineSetNameLabel = "cluster.x-k8s.io/set-name"

	// MachineDeploymentNameLabel is the label set on machines if they're controlled by MachineDeployment.
	MachineDeploymentNameLabel = "cluster.x-k8s.io/deployment-name"

	// MachinePoolNameLabel is the label indicating the name of the MachinePool a Machine is controlled by.
	// Note: The value of this label may be a hash if the MachinePool name is longer than 63 characters.
	MachinePoolNameLabel = "cluster.x-k8s.io/pool-name"

	// MachineControlPlaneNameLabel is the label set on machines if they're controlled by a ControlPlane.
	// Note: The value of this label may be a hash if the control plane name is longer than 63 characters.
	MachineControlPlaneNameLabel = "cluster.x-k8s.io/control-plane-name"

	// PreDrainDeleteHookAnnotationPrefix annotation specifies the prefix we
	// search each annotation for during the pre-drain.delete lifecycle hook
	// to pause reconciliation of deletion. These hooks will prevent removal of
	// draining the associated node until all are removed.
	PreDrainDeleteHookAnnotationPrefix = "pre-drain.delete.hook.machine.cluster.x-k8s.io"

	// PreTerminateDeleteHookAnnotationPrefix annotation specifies the prefix we
	// search each annotation for during the pre-terminate.delete lifecycle hook
	// to pause reconciliation of deletion. These hooks will prevent removal of
	// an instance from an infrastructure provider until all are removed.
	//
	// Notes for Machines managed by KCP (starting with Cluster API v1.8.2):
	// * KCP adds its own pre-terminate hook on all Machines it controls. This is done to ensure it can later remove
	//   the etcd member right before Machine termination (i.e. before InfraMachine deletion).
	// * Starting with Kubernetes v1.31 the KCP pre-terminate hook will wait for all other pre-terminate hooks to finish to
	//   ensure it runs last (thus ensuring that kubelet is still working while other pre-terminate hooks run). This is done
	//   for v1.31 or above because the kubeadm ControlPlaneKubeletLocalMode was introduced with kubeadm 1.31 (graduated to GA in 1.36).
	//   This feature configures the kubelet to communicate with the local apiserver. Only because of that the kubelet immediately
	//   starts failing after the etcd member is removed. We need the ControlPlaneKubeletLocalMode feature with 1.31+ to adhere to the kubelet skew policy.
	PreTerminateDeleteHookAnnotationPrefix = "pre-terminate.delete.hook.machine.cluster.x-k8s.io"

	// MachineCertificatesExpiryDateAnnotation annotation specifies the expiry date of the machine certificates in RFC3339 format.
	// This annotation can be used on control plane machines to trigger rollout before certificates expire.
	// This annotation can be set on BootstrapConfig or Machine objects. The value set on the Machine object takes precedence.
	// This annotation can only be used on Control Plane Machines.
	MachineCertificatesExpiryDateAnnotation = "machine.cluster.x-k8s.io/certificates-expiry"

	// NodeRoleLabelPrefix is one of the CAPI managed Node label prefixes.
	NodeRoleLabelPrefix = "node-role.kubernetes.io"
	// NodeRestrictionLabelDomain is one of the CAPI managed Node label domains.
	NodeRestrictionLabelDomain = "node-restriction.kubernetes.io"
	// ManagedNodeLabelDomain is one of the CAPI managed Node label domains.
	ManagedNodeLabelDomain = "node.cluster.x-k8s.io"

	// ManagedNodeAnnotationDomain is one of the CAPI managed Node annotation domains.
	ManagedNodeAnnotationDomain = "node.cluster.x-k8s.io"

	// PendingAcknowledgeMoveAnnotation is an internal annotation added by the MS controller to a machine when being
	// moved from the oldMS to the newMS. The annotation is removed as soon as the MS controller get the acknowledgment about the
	// replica being accounted from the corresponding MD.
	// Note: The annotation is added when reconciling the oldMS, and it is removed when reconciling the newMS.
	// Note: This annotation is used in pair with AcknowledgedMoveAnnotation on MachineSets.
	PendingAcknowledgeMoveAnnotation = "in-place-updates.internal.cluster.x-k8s.io/pending-acknowledge-move"

	// UpdateInProgressAnnotation is an internal annotation added to machines by the controller owning the Machine when in-place update
	// is started, e.g. by the MachineSet controller; the annotation will be removed by the Machine controller when in-place update is completed.
	UpdateInProgressAnnotation = "in-place-updates.internal.cluster.x-k8s.io/update-in-progress"
)

// Machine's Available condition and corresponding reasons.
const (
	// MachineAvailableCondition is true if the machine is Ready for at least MinReadySeconds, as defined by the Machine's MinReadySeconds field.
	// Note: MinReadySeconds is assumed 0 until it will be implemented in v1beta2 API.
	MachineAvailableCondition = AvailableCondition

	// MachineWaitingForMinReadySecondsReason surfaces when a machine is ready for less than MinReadySeconds (and thus not yet available).
	MachineWaitingForMinReadySecondsReason = "WaitingForMinReadySeconds"

	// MachineAvailableReason surfaces when a machine is ready for at least MinReadySeconds.
	// Note: MinReadySeconds is assumed 0 until it will be implemented in v1beta2 API.
	MachineAvailableReason = AvailableReason

	// MachineAvailableInternalErrorReason surfaces unexpected error when computing the Available condition.
	MachineAvailableInternalErrorReason = InternalErrorReason
)

// Machine's Ready condition and corresponding reasons.
const (
	// MachineReadyCondition is true if the Machine's deletionTimestamp is not set, Machine's BootstrapConfigReady, InfrastructureReady,
	// NodeHealthy and HealthCheckSucceeded (if present) conditions are true, Updating condition is false; if other conditions are defined in spec.readinessGates,
	// these conditions must be true as well.
	// Note:
	// - When summarizing the Deleting condition:
	//   - Details about Pods stuck in draining or volumes waiting for detach are dropped, in order to improve readability & reduce flickering
	//     of the condition that bubbles up to the owning resources/ to the Cluster (it also makes it more likely this condition might be aggregated with
	//     conditions reported by other machines).
	//   - If deletion is in progress for more than 15m, this surfaces on the summary condition (hint about a possible stale deletion).
	//     - if drain is in progress for more than 5 minutes, a summery of what is blocking drain also surfaces in the message.
	// - When summarizing BootstrapConfigReady, InfrastructureReady, NodeHealthy, in case the Machine is deleting, the absence of the
	//   referenced object won't be considered as an issue.
	MachineReadyCondition = ReadyCondition

	// MachineReadyReason surfaces when the machine readiness criteria is met.
	MachineReadyReason = ReadyReason

	// MachineNotReadyReason surfaces when the machine readiness criteria is not met.
	// Note: when a machine is not ready, it is also not available.
	MachineNotReadyReason = NotReadyReason

	// MachineReadyUnknownReason surfaces when at least one machine readiness criteria is unknown
	// and no machine readiness criteria is not met.
	MachineReadyUnknownReason = ReadyUnknownReason

	// MachineReadyInternalErrorReason surfaces unexpected error when computing the Ready condition.
	MachineReadyInternalErrorReason = InternalErrorReason
)

// Machine's UpToDate condition and corresponding reasons.
// Note: UpToDate condition is set by the controller owning the machine.
const (
	// MachineUpToDateCondition is true if the Machine spec matches the spec of the Machine's owner resource, e.g. KubeadmControlPlane or MachineDeployment.
	// The Machine's owner (e.g. MachineDeployment) is authoritative to set their owned Machine's UpToDate conditions based on its current spec.
	// NOTE: The Machine's owner might use this condition to surface also other use cases when Machine is considered not up to date, e.g. when MachineDeployment spec.rolloutAfter
	// is expired and the Machine needs to be rolled out.
	MachineUpToDateCondition = "UpToDate"

	// MachineUpToDateReason surface when a Machine spec matches the spec of the Machine's owner resource, e.g. KubeadmControlPlane or MachineDeployment.
	MachineUpToDateReason = "UpToDate"

	// MachineNotUpToDateReason surface when a Machine spec does not match the spec of the Machine's owner resource, e.g. KubeadmControlPlane or MachineDeployment.
	MachineNotUpToDateReason = "NotUpToDate"

	// MachineUpToDateUpdatingReason surface when a Machine spec matches the spec of the Machine's owner resource,
	// but the Machine is still updating in-place.
	MachineUpToDateUpdatingReason = "Updating"
)

// Machine's Updating condition and corresponding reasons.
// Note: Updating condition is set by the Machine controller during in-place updates.
const (
	// MachineUpdatingCondition is true while an in-place update is in progress on the Machine.
	// The condition is owned by the Machine controller and is used to track the progress of in-place updates.
	// This condition is considered when computing the UpToDate condition.
	MachineUpdatingCondition = "Updating"

	// MachineNotUpdatingReason surfaces when the Machine is not performing an in-place update.
	MachineNotUpdatingReason = "NotUpdating"

	// MachineInPlaceUpdatingReason surfaces when the Machine is waiting for in-place update to complete.
	MachineInPlaceUpdatingReason = "InPlaceUpdating"

	// MachineInPlaceUpdateFailedReason surfaces when the in-place update has failed.
	MachineInPlaceUpdateFailedReason = "InPlaceUpdateFailed"
)

// Machine's BootstrapConfigReady condition and corresponding reasons.
// Note: when possible, BootstrapConfigReady condition will use reasons surfaced from the underlying bootstrap config object.
const (
	// MachineBootstrapConfigReadyCondition condition mirrors the corresponding Ready condition from the Machine's BootstrapConfig resource.
	MachineBootstrapConfigReadyCondition = BootstrapConfigReadyCondition

	// MachineBootstrapDataSecretProvidedReason surfaces when a bootstrap data secret is provided (not originated
	// from a BoostrapConfig object referenced from the machine).
	MachineBootstrapDataSecretProvidedReason = "DataSecretProvided"

	// MachineBootstrapConfigReadyReason surfaces when the machine bootstrap config is ready.
	MachineBootstrapConfigReadyReason = ReadyReason

	// MachineBootstrapConfigNotReadyReason surfaces when the machine bootstrap config is not ready.
	MachineBootstrapConfigNotReadyReason = NotReadyReason

	// MachineBootstrapConfigInvalidConditionReportedReason surfaces a BootstrapConfig Ready condition (read from a bootstrap config object) which is invalid.
	// (e.g. its status is missing).
	MachineBootstrapConfigInvalidConditionReportedReason = InvalidConditionReportedReason

	// MachineBootstrapConfigInternalErrorReason surfaces unexpected failures when reading a BootstrapConfig object.
	MachineBootstrapConfigInternalErrorReason = InternalErrorReason

	// MachineBootstrapConfigDoesNotExistReason surfaces when a referenced bootstrap config object does not exist.
	// Note: this could happen when creating the machine. However, this state should be treated as an error if it lasts indefinitely.
	MachineBootstrapConfigDoesNotExistReason = ObjectDoesNotExistReason

	// MachineBootstrapConfigDeletedReason surfaces when a referenced bootstrap config object has been deleted.
	// Note: controllers can't identify if the bootstrap config object was deleted the controller itself, e.g.
	// during the deletion workflow, or by a users.
	MachineBootstrapConfigDeletedReason = ObjectDeletedReason
)

// Machine's InfrastructureReady condition and corresponding reasons.
// Note: when possible, InfrastructureReady condition will use reasons surfaced from the underlying infra machine object.
const (
	// MachineInfrastructureReadyCondition mirrors the corresponding Ready condition from the Machine's infrastructure resource.
	MachineInfrastructureReadyCondition = InfrastructureReadyCondition

	// MachineInfrastructureReadyReason surfaces when the machine infrastructure is ready.
	MachineInfrastructureReadyReason = ReadyReason

	// MachineInfrastructureNotReadyReason surfaces when the machine infrastructure is not ready.
	MachineInfrastructureNotReadyReason = NotReadyReason

	// MachineInfrastructureInvalidConditionReportedReason surfaces a infrastructure Ready condition (read from an infra machine object) which is invalid.
	// (e.g. its status is missing).
	MachineInfrastructureInvalidConditionReportedReason = InvalidConditionReportedReason

	// MachineInfrastructureInternalErrorReason surfaces unexpected failures when reading an infra machine object.
	MachineInfrastructureInternalErrorReason = InternalErrorReason

	// MachineInfrastructureDoesNotExistReason surfaces when a referenced infrastructure object does not exist.
	// Note: this could happen when creating the machine. However, this state should be treated as an error if it lasts indefinitely.
	MachineInfrastructureDoesNotExistReason = ObjectDoesNotExistReason

	// MachineInfrastructureDeletedReason surfaces when a referenced infrastructure object has been deleted.
	// Note: controllers can't identify if the infrastructure object was deleted by the controller itself, e.g.
	// during the deletion workflow, or by a users.
	MachineInfrastructureDeletedReason = ObjectDeletedReason
)

// Machine's NodeHealthy and NodeReady conditions and corresponding reasons.
// Note: when possible, NodeHealthy and NodeReady conditions will use reasons surfaced from the underlying node.
const (
	// MachineNodeHealthyCondition is true if the Machine's Node is ready and it does not report MemoryPressure, DiskPressure and PIDPressure.
	MachineNodeHealthyCondition = "NodeHealthy"

	// MachineNodeReadyCondition is true if the Machine's Node is ready.
	MachineNodeReadyCondition = "NodeReady"

	// MachineNodeReadyReason surfaces when Machine's Node Ready condition is true.
	MachineNodeReadyReason = "NodeReady"

	// MachineNodeNotReadyReason surfaces when Machine's Node Ready condition is false.
	MachineNodeNotReadyReason = "NodeNotReady"

	// MachineNodeReadyUnknownReason surfaces when Machine's Node Ready condition is unknown.
	MachineNodeReadyUnknownReason = "NodeReadyUnknown"

	// MachineNodeHealthyReason surfaces when all the node conditions report healthy state.
	MachineNodeHealthyReason = "NodeHealthy"

	// MachineNodeNotHealthyReason surfaces when at least one node conditions report not healthy state.
	MachineNodeNotHealthyReason = "NodeNotHealthy"

	// MachineNodeHealthUnknownReason surfaces when at least one node conditions report healthy state unknown
	// and no node conditions report not healthy state.
	MachineNodeHealthUnknownReason = "NodeHealthyUnknown"

	// MachineNodeInternalErrorReason surfaces unexpected failures when reading a Node object.
	MachineNodeInternalErrorReason = InternalErrorReason

	// MachineNodeDoesNotExistReason surfaces when the node hosted on the machine does not exist.
	// Note: this could happen when creating the machine. However, this state should be treated as an error if it lasts indefinitely.
	MachineNodeDoesNotExistReason = "NodeDoesNotExist"

	// MachineNodeDeletedReason surfaces when the node hosted on the machine has been deleted.
	// Note: controllers can't identify if the Node was deleted by the controller itself, e.g.
	// during the deletion workflow, or by a users.
	MachineNodeDeletedReason = "NodeDeleted"

	// MachineNodeInspectionFailedReason documents a failure when inspecting the status of a Node.
	MachineNodeInspectionFailedReason = InspectionFailedReason

	// MachineNodeConnectionDownReason surfaces that the connection to the workload cluster is down.
	MachineNodeConnectionDownReason = ConnectionDownReason
)

// Machine's HealthCheckSucceeded condition and corresponding reasons.
// Note: HealthCheckSucceeded condition is set by the MachineHealthCheck controller.
const (
	// MachineHealthCheckSucceededCondition is true if MHC instances targeting this machine report the Machine
	// is healthy according to the definition of healthy present in the spec of the MachineHealthCheck object.
	MachineHealthCheckSucceededCondition = "HealthCheckSucceeded"

	// MachineHealthCheckSucceededReason surfaces when a machine passes all the health checks defined by a MachineHealthCheck object.
	MachineHealthCheckSucceededReason = "HealthCheckSucceeded"

	// MachineHealthCheckUnhealthyNodeReason surfaces when the node hosted on the machine does not pass the health checks
	// defined by a MachineHealthCheck object.
	MachineHealthCheckUnhealthyNodeReason = "UnhealthyNode"

	// MachineHealthCheckUnhealthyMachineReason surfaces when the machine does not pass the health checks
	// defined by a MachineHealthCheck object.
	MachineHealthCheckUnhealthyMachineReason = "UnhealthyMachine"

	// MachineHealthCheckNodeStartupTimeoutReason surfaces when the node hosted on the machine does not appear within
	// the timeout defined by a MachineHealthCheck object.
	MachineHealthCheckNodeStartupTimeoutReason = "NodeStartupTimeout"

	// MachineHealthCheckNodeDeletedReason surfaces when a MachineHealthCheck detects that the node hosted on the
	// machine has been deleted while the Machine is still running.
	MachineHealthCheckNodeDeletedReason = "NodeDeleted"

	// MachineHealthCheckHasRemediateAnnotationReason surfaces when a MachineHealthCheck detects that a Machine was
	// marked for remediation via the `cluster.x-k8s.io/remediate-machine` annotation.
	MachineHealthCheckHasRemediateAnnotationReason = "HasRemediateAnnotation"
)

// Machine's OwnerRemediated conditions and corresponding reasons.
// Note: OwnerRemediated condition is initially set by the MachineHealthCheck controller; then it is up to the Machine's
// owner controller to update or delete this condition.
const (
	// MachineOwnerRemediatedCondition is only present if MHC instances targeting this machine
	// determine that the controller owning this machine should perform remediation.
	MachineOwnerRemediatedCondition = "OwnerRemediated"

	// MachineOwnerRemediatedWaitingForRemediationReason surfaces the machine is waiting for the owner controller
	// to start remediation.
	MachineOwnerRemediatedWaitingForRemediationReason = "WaitingForRemediation"
)

// Machine's ExternallyRemediated conditions and corresponding reasons.
// Note: ExternallyRemediated condition is initially set by the MachineHealthCheck controller; then it is up to the external
// remediation controller to update or delete this condition.
const (
	// MachineExternallyRemediatedCondition is only present if MHC instances targeting this machine
	// determine that an external controller should perform remediation.
	MachineExternallyRemediatedCondition = "ExternallyRemediated"

	// MachineExternallyRemediatedWaitingForRemediationReason surfaces the machine is waiting for the
	// external remediation controller to start remediation.
	MachineExternallyRemediatedWaitingForRemediationReason = "WaitingForRemediation"

	// MachineExternallyRemediatedRemediationTemplateNotFoundReason surfaces that the MachineHealthCheck cannot
	// find the template for an external remediation request.
	MachineExternallyRemediatedRemediationTemplateNotFoundReason = "RemediationTemplateNotFound"

	// MachineExternallyRemediatedRemediationRequestCreationFailedReason surfaces that the MachineHealthCheck cannot
	// create a request for the external remediation controller.
	MachineExternallyRemediatedRemediationRequestCreationFailedReason = "RemediationRequestCreationFailed"
)

// Machine's Deleting condition and corresponding reasons.
const (
	// MachineDeletingCondition surfaces details about progress in the machine deletion workflow.
	MachineDeletingCondition = DeletingCondition

	// MachineNotDeletingReason surfaces when the Machine is not deleting because the
	// DeletionTimestamp is not set.
	MachineNotDeletingReason = NotDeletingReason

	// MachineDeletingReason surfaces when the Machine is deleting because the
	// DeletionTimestamp is set. This reason is used if none of the more specific reasons apply.
	MachineDeletingReason = DeletingReason

	// MachineDeletingInternalErrorReason surfaces unexpected failures when deleting a Machine.
	MachineDeletingInternalErrorReason = InternalErrorReason

	// MachineDeletingWaitingForPreDrainHookReason surfaces when the Machine deletion
	// waits for pre-drain hooks to complete. I.e. it waits until there are no annotations
	// with the `pre-drain.delete.hook.machine.cluster.x-k8s.io` prefix on the Machine anymore.
	MachineDeletingWaitingForPreDrainHookReason = "WaitingForPreDrainHook"

	// MachineDeletingDrainingNodeReason surfaces when the Machine deletion is draining the Node.
	MachineDeletingDrainingNodeReason = "DrainingNode"

	// MachineDeletingWaitingForVolumeDetachReason surfaces when the Machine deletion is
	// waiting for volumes to detach from the Node.
	MachineDeletingWaitingForVolumeDetachReason = "WaitingForVolumeDetach"

	// MachineDeletingWaitingForPreTerminateHookReason surfaces when the Machine deletion
	// waits for pre-terminate hooks to complete. I.e. it waits until there are no annotations
	// with the `pre-terminate.delete.hook.machine.cluster.x-k8s.io` prefix on the Machine anymore.
	MachineDeletingWaitingForPreTerminateHookReason = "WaitingForPreTerminateHook"

	// MachineDeletingWaitingForInfrastructureDeletionReason surfaces when the Machine deletion
	// waits for InfraMachine deletion to complete.
	MachineDeletingWaitingForInfrastructureDeletionReason = "WaitingForInfrastructureDeletion"

	// MachineDeletingWaitingForBootstrapDeletionReason surfaces when the Machine deletion
	// waits for BootstrapConfig deletion to complete.
	MachineDeletingWaitingForBootstrapDeletionReason = "WaitingForBootstrapDeletion"

	// MachineDeletingDeletingNodeReason surfaces when the Machine deletion is
	// deleting the Node.
	MachineDeletingDeletingNodeReason = "DeletingNode"

	// MachineDeletingDeletionCompletedReason surfaces when the Machine deletion has been completed.
	// This reason is set right after the `machine.cluster.x-k8s.io` finalizer is removed.
	// This means that the object will go away (i.e. be removed from etcd), except if there are other
	// finalizers on the Machine object.
	MachineDeletingDeletionCompletedReason = DeletionCompletedReason
)

// MachineSpec defines the desired state of Machine.
type MachineSpec struct {
	// clusterName is the name of the Cluster this object belongs to.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	ClusterName string `json:"clusterName,omitempty"`

	// bootstrap is a reference to a local struct which encapsulates
	// fields to configure the Machine’s bootstrapping mechanism.
	// +required
	Bootstrap Bootstrap `json:"bootstrap,omitempty,omitzero"`

	// infrastructureRef is a required reference to a custom resource
	// offered by an infrastructure provider.
	// +required
	InfrastructureRef ContractVersionedObjectReference `json:"infrastructureRef,omitempty,omitzero"`

	// version defines the desired Kubernetes version.
	// This field is meant to be optionally used by bootstrap providers.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Version string `json:"version,omitempty"`

	// providerID is the identification ID of the machine provided by the provider.
	// This field must match the provider ID as seen on the node object corresponding to this machine.
	// This field is required by higher level consumers of cluster-api. Example use case is cluster autoscaler
	// with cluster-api as provider. Clean-up logic in the autoscaler compares machines to nodes to find out
	// machines at provider which could not get registered as Kubernetes nodes. With cluster-api as a
	// generic out-of-tree provider for autoscaler, this field is required by autoscaler to be
	// able to have a provider view of the list of machines. Another list of nodes is queried from the k8s apiserver
	// and then a comparison is done to find out unregistered machines and are marked for delete.
	// This field will be set by the actuators and consumed by higher level entities like autoscaler that will
	// be interfacing with cluster-api as generic provider.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	ProviderID string `json:"providerID,omitempty"`

	// failureDomain is the failure domain the machine will be created in.
	// Must match the name of a FailureDomain from the Cluster status.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	FailureDomain string `json:"failureDomain,omitempty"`

	// minReadySeconds is the minimum number of seconds for which a Machine should be ready before considering it available.
	// Defaults to 0 (Machine will be considered available as soon as the Machine is ready)
	// +optional
	// +kubebuilder:validation:Minimum=0
	MinReadySeconds *int32 `json:"minReadySeconds,omitempty"`

	// readinessGates specifies additional conditions to include when evaluating Machine Ready condition.
	//
	// This field can be used e.g. by Cluster API control plane providers to extend the semantic of the
	// Ready condition for the Machine they control, like the kubeadm control provider adding ReadinessGates
	// for the APIServerPodHealthy, SchedulerPodHealthy conditions, etc.
	//
	// Another example are external controllers, e.g. responsible to install special software/hardware on the Machines;
	// they can include the status of those components with a new condition and add this condition to ReadinessGates.
	//
	// NOTE: In case readinessGates conditions start with the APIServer, ControllerManager, Scheduler prefix, and all those
	// readiness gates condition are reporting the same message, when computing the Machine's Ready condition those
	// readinessGates will be replaced by a single entry reporting "Control plane components: " + message.
	// This helps to improve readability of conditions bubbling up to the Machine's owner resource / to the Cluster).
	// +optional
	// +listType=map
	// +listMapKey=conditionType
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=32
	ReadinessGates []MachineReadinessGate `json:"readinessGates,omitempty"`

	// deletion contains configuration options for Machine deletion.
	// +optional
	Deletion MachineDeletionSpec `json:"deletion,omitempty,omitzero"`

	// taints are the node taints that Cluster API will manage.
	// This list is not necessarily complete: other Kubernetes components may add or remove other taints from nodes,
	// e.g. the node controller might add the node.kubernetes.io/not-ready taint.
	// Only those taints defined in this list will be added or removed by core Cluster API controllers.
	//
	// There can be at most 64 taints.
	// A pod would have to tolerate all existing taints to run on the corresponding node.
	//
	// NOTE: This list is implemented as a "map" type, meaning that individual elements can be managed by different owners.
	// +optional
	// +listType=map
	// +listMapKey=key
	// +listMapKey=effect
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=64
	Taints []MachineTaint `json:"taints,omitempty"`
}

// MachineDeletionSpec contains configuration options for Machine deletion.
// +kubebuilder:validation:MinProperties=1
type MachineDeletionSpec struct {
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
}

// MachineReadinessGate contains the type of a Machine condition to be used as a readiness gate.
type MachineReadinessGate struct {
	// conditionType refers to a condition with matching type in the Machine's condition list.
	// If the conditions doesn't exist, it will be treated as unknown.
	// Note: Both Cluster API conditions or conditions added by 3rd party controllers can be used as readiness gates.
	// +required
	// +kubebuilder:validation:Pattern=`^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*/)?(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])$`
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=316
	ConditionType string `json:"conditionType,omitempty"`

	// polarity of the conditionType specified in this readinessGate.
	// Valid values are Positive, Negative and omitted.
	// When omitted, the default behaviour will be Positive.
	// A positive polarity means that the condition should report a true status under normal conditions.
	// A negative polarity means that the condition should report a false status under normal conditions.
	// +optional
	Polarity ConditionPolarity `json:"polarity,omitempty"`
}

// MachineStatus defines the observed state of Machine.
// +kubebuilder:validation:MinProperties=1
type MachineStatus struct {
	// conditions represents the observations of a Machine's current state.
	// Known condition types are Available, Ready, UpToDate, BootstrapConfigReady, InfrastructureReady, NodeReady,
	// NodeHealthy, Updating, Deleting, Paused.
	// If a MachineHealthCheck is targeting this machine, also HealthCheckSucceeded, OwnerRemediated conditions are added.
	// Additionally control plane Machines controlled by KubeadmControlPlane will have following additional conditions:
	// APIServerPodHealthy, ControllerManagerPodHealthy, SchedulerPodHealthy, EtcdPodHealthy, EtcdMemberHealthy.
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=32
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// initialization provides observations of the Machine initialization process.
	// NOTE: Fields in this struct are part of the Cluster API contract and are used to orchestrate initial Machine provisioning.
	// +optional
	Initialization MachineInitializationStatus `json:"initialization,omitempty,omitzero"`

	// nodeRef will point to the corresponding Node if it exists.
	// +optional
	NodeRef MachineNodeReference `json:"nodeRef,omitempty,omitzero"`

	// nodeInfo is a set of ids/uuids to uniquely identify the node.
	// More info: https://kubernetes.io/docs/concepts/nodes/node/#info
	// +optional
	NodeInfo *corev1.NodeSystemInfo `json:"nodeInfo,omitempty"`

	// lastUpdated identifies when the phase of the Machine last transitioned.
	// +optional
	LastUpdated metav1.Time `json:"lastUpdated,omitempty,omitzero"`

	// addresses is a list of addresses assigned to the machine.
	// This field is copied from the infrastructure provider reference.
	// +optional
	Addresses MachineAddresses `json:"addresses,omitempty"`

	// phase represents the current phase of machine actuation.
	// +optional
	// +kubebuilder:validation:Enum=Pending;Provisioning;Provisioned;Running;Updating;Deleting;Deleted;Failed;Unknown
	Phase string `json:"phase,omitempty"`

	// certificatesExpiryDate is the expiry date of the machine certificates.
	// This value is only set for control plane machines.
	// +optional
	CertificatesExpiryDate metav1.Time `json:"certificatesExpiryDate,omitempty,omitzero"`

	// observedGeneration is the latest generation observed by the controller.
	// +optional
	// +kubebuilder:validation:Minimum=1
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// deletion contains information relating to removal of the Machine.
	// Only present when the Machine has a deletionTimestamp and drain or wait for volume detach started.
	// +optional
	Deletion *MachineDeletionStatus `json:"deletion,omitempty"`

	// deprecated groups all the status fields that are deprecated and will be removed when all the nested field are removed.
	// +optional
	Deprecated *MachineDeprecatedStatus `json:"deprecated,omitempty"`
}

// MachineNodeReference is a reference to the node running on the machine.
type MachineNodeReference struct {
	// name of the node.
	// name must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
	Name string `json:"name,omitempty"`
}

// IsDefined returns true if the MachineNodeReference is set.
func (r *MachineNodeReference) IsDefined() bool {
	if r == nil {
		return false
	}
	return r.Name != ""
}

// MachineInitializationStatus provides observations of the Machine initialization process.
// NOTE: Fields in this struct are part of the Cluster API contract and are used to orchestrate initial Machine provisioning.
// +kubebuilder:validation:MinProperties=1
type MachineInitializationStatus struct {
	// infrastructureProvisioned is true when the infrastructure provider reports that Machine's infrastructure is fully provisioned.
	// NOTE: this field is part of the Cluster API contract, and it is used to orchestrate provisioning.
	// The value of this field is never updated after provisioning is completed.
	// +optional
	InfrastructureProvisioned *bool `json:"infrastructureProvisioned,omitempty"`

	// bootstrapDataSecretCreated is true when the bootstrap provider reports that the Machine's boostrap secret is created.
	// NOTE: this field is part of the Cluster API contract, and it is used to orchestrate provisioning.
	// The value of this field is never updated after provisioning is completed.
	// +optional
	BootstrapDataSecretCreated *bool `json:"bootstrapDataSecretCreated,omitempty"`
}

// MachineDeprecatedStatus groups all the status fields that are deprecated and will be removed in a future version.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type MachineDeprecatedStatus struct {
	// v1beta1 groups all the status fields that are deprecated and will be removed when support for v1beta1 will be dropped.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	V1Beta1 *MachineV1Beta1DeprecatedStatus `json:"v1beta1,omitempty"`
}

// MachineV1Beta1DeprecatedStatus groups all the status fields that are deprecated and will be removed when support for v1beta1 will be dropped.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type MachineV1Beta1DeprecatedStatus struct {
	// conditions defines current service state of the Machine.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	Conditions Conditions `json:"conditions,omitempty"`

	// failureReason will be set in the event that there is a terminal problem
	// reconciling the Machine and will contain a succinct value suitable
	// for machine interpretation.
	//
	// This field should not be set for transitive errors that a controller
	// faces that are expected to be fixed automatically over
	// time (like service outages), but instead indicate that something is
	// fundamentally wrong with the Machine's spec or the configuration of
	// the controller, and that manual intervention is required. Examples
	// of terminal errors would be invalid combinations of settings in the
	// spec, values that are unsupported by the controller, or the
	// responsible controller itself being critically misconfigured.
	//
	// Any transient errors that occur during the reconciliation of Machines
	// can be added as events to the Machine object and/or logged in the
	// controller's output.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	FailureReason *capierrors.MachineStatusError `json:"failureReason,omitempty"`

	// failureMessage will be set in the event that there is a terminal problem
	// reconciling the Machine and will contain a more verbose string suitable
	// for logging and human consumption.
	//
	// This field should not be set for transitive errors that a controller
	// faces that are expected to be fixed automatically over
	// time (like service outages), but instead indicate that something is
	// fundamentally wrong with the Machine's spec or the configuration of
	// the controller, and that manual intervention is required. Examples
	// of terminal errors would be invalid combinations of settings in the
	// spec, values that are unsupported by the controller, or the
	// responsible controller itself being critically misconfigured.
	//
	// Any transient errors that occur during the reconciliation of Machines
	// can be added as events to the Machine object and/or logged in the
	// controller's output.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=10240
	FailureMessage *string `json:"failureMessage,omitempty"` //nolint:kubeapilinter // field will be removed when v1beta1 is removed
}

// MachineDeletionStatus is the deletion state of the Machine.
type MachineDeletionStatus struct {
	// nodeDrainStartTime is the time when the drain of the node started and is used to determine
	// if the nodeDrainTimeoutSeconds is exceeded.
	// Only present when the Machine has a deletionTimestamp and draining the node had been started.
	// +optional
	NodeDrainStartTime metav1.Time `json:"nodeDrainStartTime,omitempty,omitzero"`

	// waitForNodeVolumeDetachStartTime is the time when waiting for volume detachment started
	// and is used to determine if the nodeVolumeDetachTimeoutSeconds is exceeded.
	// Detaching volumes from nodes is usually done by CSI implementations and the current state
	// is observed from the node's `.Status.VolumesAttached` field.
	// Only present when the Machine has a deletionTimestamp and waiting for volume detachments had been started.
	// +optional
	WaitForNodeVolumeDetachStartTime metav1.Time `json:"waitForNodeVolumeDetachStartTime,omitempty,omitzero"`
}

// SetTypedPhase sets the Phase field to the string representation of MachinePhase.
func (m *MachineStatus) SetTypedPhase(p MachinePhase) {
	m.Phase = string(p)
}

// GetTypedPhase attempts to parse the Phase field and return
// the typed MachinePhase representation as described in `machine_phase_types.go`.
func (m *MachineStatus) GetTypedPhase() MachinePhase {
	switch phase := MachinePhase(m.Phase); phase {
	case
		MachinePhasePending,
		MachinePhaseProvisioning,
		MachinePhaseProvisioned,
		MachinePhaseRunning,
		MachinePhaseUpdating,
		MachinePhaseDeleting,
		MachinePhaseDeleted,
		MachinePhaseFailed:
		return phase
	default:
		return MachinePhaseUnknown
	}
}

// Bootstrap encapsulates fields to configure the Machine’s bootstrapping mechanism.
type Bootstrap struct {
	// configRef is a reference to a bootstrap provider-specific resource
	// that holds configuration details. The reference is optional to
	// allow users/operators to specify Bootstrap.DataSecretName without
	// the need of a controller.
	// +optional
	ConfigRef ContractVersionedObjectReference `json:"configRef,omitempty,omitzero"`

	// dataSecretName is the name of the secret that stores the bootstrap data script.
	// If nil, the Machine should remain in the Pending state.
	// +optional
	// +kubebuilder:validation:MinLength=0
	// +kubebuilder:validation:MaxLength=253
	DataSecretName *string `json:"dataSecretName,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=machines,shortName=ma,scope=Namespaced,categories=cluster-api
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".spec.clusterName",description="Cluster"
// +kubebuilder:printcolumn:name="Node Name",type="string",JSONPath=".status.nodeRef.name",description="Node name associated with this machine"
// +kubebuilder:printcolumn:name="Provider ID",type="string",JSONPath=".spec.providerID",description="Provider ID",priority=10
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=`.status.conditions[?(@.type=="Ready")].status`,description="Machine pass all readiness checks"
// +kubebuilder:printcolumn:name="Available",type="string",JSONPath=`.status.conditions[?(@.type=="Available")].status`,description="Machine is Ready for at least MinReadySeconds"
// +kubebuilder:printcolumn:name="Up-to-date",type="string",JSONPath=`.status.conditions[?(@.type=="UpToDate")].status`,description=" Machine spec matches the spec of the Machine's owner resource, e.g. MachineDeployment"
// +kubebuilder:printcolumn:name="Internal-IP",type="string",JSONPath=`.status.addresses[?(@.type=="InternalIP")].address`,description="Internal IP of the machine",priority=10
// +kubebuilder:printcolumn:name="External-IP",type="string",JSONPath=`.status.addresses[?(@.type=="ExternalIP")].address`,description="External IP of the machine",priority=10
// +kubebuilder:printcolumn:name="OS-Image",type="string",JSONPath=`.status.nodeInfo.osImage`,description="OS Image reported by the node",priority=10
// +kubebuilder:printcolumn:name="Paused",type="string",JSONPath=`.status.conditions[?(@.type=="Paused")].status`,description="Reconciliation paused",priority=10
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase",description="Machine status such as Terminating/Pending/Running/Failed etc"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of Machine"
// +kubebuilder:printcolumn:name="Version",type="string",JSONPath=".spec.version",description="Kubernetes version associated with this Machine"

// Machine is the Schema for the machines API.
type Machine struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec is the desired state of Machine.
	// +required
	Spec MachineSpec `json:"spec,omitempty,omitzero"`
	// status is the observed state of Machine.
	// +optional
	Status MachineStatus `json:"status,omitempty,omitzero"`
}

// GetV1Beta1Conditions returns the set of conditions for this object.
func (m *Machine) GetV1Beta1Conditions() Conditions {
	if m.Status.Deprecated == nil || m.Status.Deprecated.V1Beta1 == nil {
		return nil
	}
	return m.Status.Deprecated.V1Beta1.Conditions
}

// SetV1Beta1Conditions sets the conditions on this object.
func (m *Machine) SetV1Beta1Conditions(conditions Conditions) {
	if m.Status.Deprecated == nil {
		m.Status.Deprecated = &MachineDeprecatedStatus{}
	}
	if m.Status.Deprecated.V1Beta1 == nil {
		m.Status.Deprecated.V1Beta1 = &MachineV1Beta1DeprecatedStatus{}
	}
	m.Status.Deprecated.V1Beta1.Conditions = conditions
}

// GetConditions returns the set of conditions for this object.
func (m *Machine) GetConditions() []metav1.Condition {
	return m.Status.Conditions
}

// SetConditions sets conditions for an API object.
func (m *Machine) SetConditions(conditions []metav1.Condition) {
	m.Status.Conditions = conditions
}

// +kubebuilder:object:root=true

// MachineList contains a list of Machine.
type MachineList struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard list's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#lists-and-simple-kinds
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`
	// items is the list of Machines.
	Items []Machine `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &Machine{}, &MachineList{})
}
