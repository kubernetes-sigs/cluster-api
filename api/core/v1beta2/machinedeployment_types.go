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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	// MachineDeploymentTopologyFinalizer is the finalizer used by the topology MachineDeployment controller to
	// clean up referenced template resources if necessary when a MachineDeployment is being deleted.
	MachineDeploymentTopologyFinalizer = "machinedeployment.topology.cluster.x-k8s.io"

	// MachineDeploymentFinalizer is the finalizer used by the MachineDeployment controller to
	// ensure ordered cleanup of corresponding MachineSets when a MachineDeployment is being deleted.
	MachineDeploymentFinalizer = "cluster.x-k8s.io/machinedeployment"
)

// MachineDeploymentRolloutStrategyType defines the type of MachineDeployment rollout strategies.
// +kubebuilder:validation:Enum=RollingUpdate;OnDelete
type MachineDeploymentRolloutStrategyType string

const (
	// RollingUpdateMachineDeploymentStrategyType replaces the old MachineSet by new one using rolling update
	// i.e. gradually scale down the old MachineSet and scale up the new one.
	RollingUpdateMachineDeploymentStrategyType MachineDeploymentRolloutStrategyType = "RollingUpdate"

	// OnDeleteMachineDeploymentStrategyType replaces old MachineSets when the deletion of the associated machines are completed.
	OnDeleteMachineDeploymentStrategyType MachineDeploymentRolloutStrategyType = "OnDelete"

	// RevisionAnnotation is the revision annotation of a machine deployment's machine sets which records its rollout sequence.
	RevisionAnnotation = "machinedeployment.clusters.x-k8s.io/revision"

	// DesiredReplicasAnnotation is the desired replicas for a machine deployment recorded as an annotation
	// in its machine sets. Helps in separating scaling events from the rollout process and for
	// determining if the new machine set for a deployment is really saturated.
	DesiredReplicasAnnotation = "machinedeployment.clusters.x-k8s.io/desired-replicas"

	// MaxReplicasAnnotation is the maximum replicas a deployment can have at a given point, which
	// is machinedeployment.spec.replicas + maxSurge. Used by the underlying machine sets to estimate their
	// proportions in case the deployment has surge replicas.
	MaxReplicasAnnotation = "machinedeployment.clusters.x-k8s.io/max-replicas"

	// MachineDeploymentUniqueLabel is used to uniquely identify the Machines of a MachineSet.
	// The MachineDeployment controller will set this label on a MachineSet when it is created.
	// The label is also applied to the Machines of the MachineSet and used in the MachineSet selector.
	// Note: For the lifetime of the MachineSet the label's value has to stay the same, otherwise the
	// MachineSet selector would no longer match its Machines.
	// Note: In previous Cluster API versions (< v1.4.0), the label value was the hash of the full machine template.
	// With the introduction of in-place mutation the machine template of the MachineSet can change.
	// Because of that it is impossible that the label's value to always be the hash of the full machine template.
	// (Because the hash changes when the machine template changes).
	// As a result, we use the hash of the machine template while ignoring all in-place mutable fields, i.e. the
	// machine template with only fields that could trigger a rollout for the machine-template-hash, making it
	// independent of the changes to any in-place mutable fields.
	// A random string is appended at the end of the label value (label value format is "<hash>-<random string>"))
	// to distinguish duplicate MachineSets that have the exact same spec but were created as a result of rolloutAfter.
	MachineDeploymentUniqueLabel = "machine-template-hash"
)

// MachineDeployment's Available condition and corresponding reasons.
const (
	// MachineDeploymentAvailableCondition is true if the MachineDeployment is not deleted, and it has minimum
	// availability according to parameters specified in the deployment strategy, e.g. If using RollingUpgrade strategy,
	// availableReplicas must be greater or equal than desired replicas - MaxUnavailable replicas.
	MachineDeploymentAvailableCondition = AvailableCondition

	// MachineDeploymentAvailableWaitingForReplicasSetReason surfaces when the .spec.replicas
	// field of the MachineDeployment is not set.
	MachineDeploymentAvailableWaitingForReplicasSetReason = WaitingForReplicasSetReason

	// MachineDeploymentAvailableWaitingForAvailableReplicasSetReason surfaces when the .status.v1beta2.availableReplicas
	// field of the MachineDeployment is not set.
	MachineDeploymentAvailableWaitingForAvailableReplicasSetReason = "WaitingForAvailableReplicasSet"

	// MachineDeploymentAvailableReason surfaces when a Deployment is available.
	MachineDeploymentAvailableReason = AvailableReason

	// MachineDeploymentNotAvailableReason surfaces when a Deployment is not available.
	MachineDeploymentNotAvailableReason = NotAvailableReason

	// MachineDeploymentAvailableInternalErrorReason surfaces unexpected failures when computing the Available condition.
	MachineDeploymentAvailableInternalErrorReason = InternalErrorReason
)

// MachineDeployment's MachinesReady condition and corresponding reasons.
const (
	// MachineDeploymentMachinesReadyCondition surfaces detail of issues on the controlled machines, if any.
	MachineDeploymentMachinesReadyCondition = MachinesReadyCondition

	// MachineDeploymentMachinesReadyReason surfaces when all the controlled machine's Ready conditions are true.
	MachineDeploymentMachinesReadyReason = ReadyReason

	// MachineDeploymentMachinesNotReadyReason surfaces when at least one of the controlled machine's Ready conditions is false.
	MachineDeploymentMachinesNotReadyReason = NotReadyReason

	// MachineDeploymentMachinesReadyUnknownReason surfaces when at least one of the controlled machine's Ready conditions is unknown
	// and none of the controlled machine's Ready conditions is false.
	MachineDeploymentMachinesReadyUnknownReason = ReadyUnknownReason

	// MachineDeploymentMachinesReadyNoReplicasReason surfaces when no machines exist for the MachineDeployment.
	MachineDeploymentMachinesReadyNoReplicasReason = NoReplicasReason

	// MachineDeploymentMachinesReadyInternalErrorReason surfaces unexpected failures when listing machines
	// or aggregating machine's conditions.
	MachineDeploymentMachinesReadyInternalErrorReason = InternalErrorReason
)

// MachineDeployment's MachinesUpToDate condition and corresponding reasons.
const (
	// MachineDeploymentMachinesUpToDateCondition surfaces details of controlled machines not up to date, if any.
	// Note: New machines are considered 10s after machine creation. This gives time to the machine's owner controller to recognize the new machine and add the UpToDate condition.
	MachineDeploymentMachinesUpToDateCondition = MachinesUpToDateCondition

	// MachineDeploymentMachinesUpToDateReason surfaces when all the controlled machine's UpToDate conditions are true.
	MachineDeploymentMachinesUpToDateReason = UpToDateReason

	// MachineDeploymentMachinesNotUpToDateReason surfaces when at least one of the controlled machine's UpToDate conditions is false.
	MachineDeploymentMachinesNotUpToDateReason = NotUpToDateReason

	// MachineDeploymentMachinesUpToDateUnknownReason surfaces when at least one of the controlled machine's UpToDate conditions is unknown
	// and none of the controlled machine's UpToDate conditions is false.
	MachineDeploymentMachinesUpToDateUnknownReason = UpToDateUnknownReason

	// MachineDeploymentMachinesUpToDateNoReplicasReason surfaces when no machines exist for the MachineDeployment.
	MachineDeploymentMachinesUpToDateNoReplicasReason = NoReplicasReason

	// MachineDeploymentMachinesUpToDateInternalErrorReason surfaces unexpected failures when listing machines
	// or aggregating status.
	MachineDeploymentMachinesUpToDateInternalErrorReason = InternalErrorReason
)

// MachineDeployment's RollingOut condition and corresponding reasons.
const (
	// MachineDeploymentRollingOutCondition is true if there is at least one machine not up-to-date.
	MachineDeploymentRollingOutCondition = RollingOutCondition

	// MachineDeploymentRollingOutReason surfaces when there is at least one machine not up-to-date.
	MachineDeploymentRollingOutReason = RollingOutReason

	// MachineDeploymentNotRollingOutReason surfaces when all the machines are up-to-date.
	MachineDeploymentNotRollingOutReason = NotRollingOutReason

	// MachineDeploymentRollingOutInternalErrorReason surfaces unexpected failures when listing machines.
	MachineDeploymentRollingOutInternalErrorReason = InternalErrorReason
)

// MachineDeployment's ScalingUp condition and corresponding reasons.
const (
	// MachineDeploymentScalingUpCondition is true if actual replicas < desired replicas.
	MachineDeploymentScalingUpCondition = ScalingUpCondition

	// MachineDeploymentScalingUpReason surfaces when actual replicas < desired replicas.
	MachineDeploymentScalingUpReason = ScalingUpReason

	// MachineDeploymentNotScalingUpReason surfaces when actual replicas >= desired replicas.
	MachineDeploymentNotScalingUpReason = NotScalingUpReason

	// MachineDeploymentScalingUpInternalErrorReason surfaces unexpected failures when listing machines.
	MachineDeploymentScalingUpInternalErrorReason = InternalErrorReason

	// MachineDeploymentScalingUpWaitingForReplicasSetReason surfaces when the .spec.replicas
	// field of the MachineDeployment is not set.
	MachineDeploymentScalingUpWaitingForReplicasSetReason = WaitingForReplicasSetReason
)

// MachineDeployment's ScalingDown condition and corresponding reasons.
const (
	// MachineDeploymentScalingDownCondition is true if actual replicas > desired replicas.
	MachineDeploymentScalingDownCondition = ScalingDownCondition

	// MachineDeploymentScalingDownReason surfaces when actual replicas > desired replicas.
	MachineDeploymentScalingDownReason = ScalingDownReason

	// MachineDeploymentNotScalingDownReason surfaces when actual replicas <= desired replicas.
	MachineDeploymentNotScalingDownReason = NotScalingDownReason

	// MachineDeploymentScalingDownInternalErrorReason surfaces unexpected failures when listing machines.
	MachineDeploymentScalingDownInternalErrorReason = InternalErrorReason

	// MachineDeploymentScalingDownWaitingForReplicasSetReason surfaces when the .spec.replicas
	// field of the MachineDeployment is not set.
	MachineDeploymentScalingDownWaitingForReplicasSetReason = WaitingForReplicasSetReason
)

// MachineDeployment's Remediating condition and corresponding reasons.
const (
	// MachineDeploymentRemediatingCondition surfaces details about ongoing remediation of the controlled machines, if any.
	MachineDeploymentRemediatingCondition = RemediatingCondition

	// MachineDeploymentRemediatingReason surfaces when the MachineDeployment has at least one machine with HealthCheckSucceeded set to false
	// and with the OwnerRemediated condition set to false.
	MachineDeploymentRemediatingReason = RemediatingReason

	// MachineDeploymentNotRemediatingReason surfaces when the MachineDeployment does not have any machine with HealthCheckSucceeded set to false
	// and with the OwnerRemediated condition set to false.
	MachineDeploymentNotRemediatingReason = NotRemediatingReason

	// MachineDeploymentRemediatingInternalErrorReason surfaces unexpected failures when computing the Remediating condition.
	MachineDeploymentRemediatingInternalErrorReason = InternalErrorReason
)

// MachineDeployment's Deleting condition and corresponding reasons.
const (
	// MachineDeploymentDeletingCondition surfaces details about ongoing deletion of the controlled machines.
	MachineDeploymentDeletingCondition = DeletingCondition

	// MachineDeploymentNotDeletingReason surfaces when the MachineDeployment is not deleting because the
	// DeletionTimestamp is not set.
	MachineDeploymentNotDeletingReason = NotDeletingReason

	// MachineDeploymentDeletingReason surfaces when the MachineDeployment is deleting because the
	// DeletionTimestamp is set.
	MachineDeploymentDeletingReason = DeletingReason

	// MachineDeploymentDeletingInternalErrorReason surfaces unexpected failures when deleting a MachineDeployment.
	MachineDeploymentDeletingInternalErrorReason = InternalErrorReason
)

// MachineDeploymentSpec defines the desired state of MachineDeployment.
type MachineDeploymentSpec struct {
	// clusterName is the name of the Cluster this object belongs to.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	ClusterName string `json:"clusterName,omitempty"`

	// replicas is the number of desired machines.
	// This is a pointer to distinguish between explicit zero and not specified.
	//
	// Defaults to:
	// * if the Kubernetes autoscaler min size and max size annotations are set:
	//   - if it's a new MachineDeployment, use min size
	//   - if the replicas field of the old MachineDeployment is < min size, use min size
	//   - if the replicas field of the old MachineDeployment is > max size, use max size
	//   - if the replicas field of the old MachineDeployment is in the (min size, max size) range, keep the value from the oldMD
	// * otherwise use 1
	// Note: Defaulting will be run whenever the replicas field is not set:
	// * A new MachineDeployment is created with replicas not set.
	// * On an existing MachineDeployment the replicas field was first set and is now unset.
	// Those cases are especially relevant for the following Kubernetes autoscaler use cases:
	// * A new MachineDeployment is created and replicas should be managed by the autoscaler
	// * An existing MachineDeployment which initially wasn't controlled by the autoscaler
	//   should be later controlled by the autoscaler
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// rollout allows you to configure the behaviour of rolling updates to the MachineDeployment Machines.
	// It allows you to require that all Machines are replaced after a certain time,
	// and allows you to define the strategy used during rolling replacements.
	// +optional
	Rollout MachineDeploymentRolloutSpec `json:"rollout,omitempty,omitzero"`

	// selector is the label selector for machines. Existing MachineSets whose machines are
	// selected by this will be the ones affected by this deployment.
	// It must match the machine template's labels.
	// +required
	Selector metav1.LabelSelector `json:"selector,omitempty,omitzero"`

	// template describes the machines that will be created.
	// +required
	Template MachineTemplateSpec `json:"template,omitempty,omitzero"`

	// machineNaming allows changing the naming pattern used when creating Machines.
	// Note: InfraMachines & BootstrapConfigs will use the same name as the corresponding Machines.
	// +optional
	MachineNaming MachineNamingSpec `json:"machineNaming,omitempty,omitzero"`

	// remediation controls how unhealthy Machines are remediated.
	// +optional
	Remediation MachineDeploymentRemediationSpec `json:"remediation,omitempty,omitzero"`

	// deletion contains configuration options for MachineDeployment deletion.
	// +optional
	Deletion MachineDeploymentDeletionSpec `json:"deletion,omitempty,omitzero"`

	// paused indicates that the deployment is paused.
	// +optional
	Paused *bool `json:"paused,omitempty"`
}

// MachineDeploymentRolloutSpec defines the rollout behavior.
// +kubebuilder:validation:MinProperties=1
type MachineDeploymentRolloutSpec struct {
	// after is a field to indicate a rollout should be performed
	// after the specified time even if no changes have been made to the
	// MachineDeployment.
	// Example: In the YAML the time can be specified in the RFC3339 format.
	// To specify the rolloutAfter target as March 9, 2023, at 9 am UTC
	// use "2023-03-09T09:00:00Z".
	// +optional
	After metav1.Time `json:"after,omitempty,omitzero"`

	// strategy specifies how to roll out control plane Machines.
	// +optional
	Strategy MachineDeploymentRolloutStrategy `json:"strategy,omitempty,omitzero"`
}

// MachineDeploymentRolloutStrategy describes how to replace existing machines
// with new ones.
// +kubebuilder:validation:MinProperties=1
type MachineDeploymentRolloutStrategy struct {
	// type of rollout. Allowed values are RollingUpdate and OnDelete.
	// Default is RollingUpdate.
	// +required
	Type MachineDeploymentRolloutStrategyType `json:"type,omitempty"`

	// rollingUpdate is the rolling update config params. Present only if
	// type = RollingUpdate.
	// +optional
	RollingUpdate MachineDeploymentRolloutStrategyRollingUpdate `json:"rollingUpdate,omitempty,omitzero"`
}

// MachineDeploymentRolloutStrategyRollingUpdate is used to control the desired behavior of rolling update.
// +kubebuilder:validation:MinProperties=1
type MachineDeploymentRolloutStrategyRollingUpdate struct {
	// maxUnavailable is the maximum number of machines that can be unavailable during the update.
	// Value can be an absolute number (ex: 5) or a percentage of desired
	// machines (ex: 10%).
	// Absolute number is calculated from percentage by rounding down.
	// This can not be 0 if MaxSurge is 0.
	// Defaults to 0.
	// Example: when this is set to 30%, the old MachineSet can be scaled
	// down to 70% of desired machines immediately when the rolling update
	// starts. Once new machines are ready, old MachineSet can be scaled
	// down further, followed by scaling up the new MachineSet, ensuring
	// that the total number of machines available at all times
	// during the update is at least 70% of desired machines.
	// +optional
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`

	// maxSurge is the maximum number of machines that can be scheduled above the
	// desired number of machines.
	// Value can be an absolute number (ex: 5) or a percentage of
	// desired machines (ex: 10%).
	// This can not be 0 if MaxUnavailable is 0.
	// Absolute number is calculated from percentage by rounding up.
	// Defaults to 1.
	// Example: when this is set to 30%, the new MachineSet can be scaled
	// up immediately when the rolling update starts, such that the total
	// number of old and new machines do not exceed 130% of desired
	// machines. Once old machines have been killed, new MachineSet can
	// be scaled up further, ensuring that total number of machines running
	// at any time during the update is at most 130% of desired machines.
	// +optional
	MaxSurge *intstr.IntOrString `json:"maxSurge,omitempty"`
}

// MachineDeploymentRemediationSpec controls how unhealthy Machines are remediated.
// +kubebuilder:validation:MinProperties=1
type MachineDeploymentRemediationSpec struct {
	// maxInFlight determines how many in flight remediations should happen at the same time.
	//
	// Remediation only happens on the MachineSet with the most current revision, while
	// older MachineSets (usually present during rollout operations) aren't allowed to remediate.
	//
	// Note: In general (independent of remediations), unhealthy machines are always
	// prioritized during scale down operations over healthy ones.
	//
	// MaxInFlight can be set to a fixed number or a percentage.
	// Example: when this is set to 20%, the MachineSet controller deletes at most 20% of
	// the desired replicas.
	//
	// If not set, remediation is limited to all machines (bounded by replicas)
	// under the active MachineSet's management.
	//
	// +optional
	MaxInFlight *intstr.IntOrString `json:"maxInFlight,omitempty"`
}

// MachineNamingSpec allows changing the naming pattern used when creating
// Machines.
// Note: InfraMachines & BootstrapConfigs will use the same name as the corresponding Machines.
// +kubebuilder:validation:MinProperties=1
type MachineNamingSpec struct {
	// template defines the template to use for generating the names of the
	// Machine objects.
	// If not defined, it will fallback to `{{ .machineSet.name }}-{{ .random }}`.
	// If the generated name string exceeds 63 characters, it will be trimmed to
	// 58 characters and will
	// get concatenated with a random suffix of length 5.
	// Length of the template string must not exceed 256 characters.
	// The template allows the following variables `.cluster.name`,
	// `.machineSet.name` and `.random`.
	// The variable `.cluster.name` retrieves the name of the cluster object
	// that owns the Machines being created.
	// The variable `.machineSet.name` retrieves the name of the MachineSet
	// object that owns the Machines being created.
	// The variable `.random` is substituted with random alphanumeric string,
	// without vowels, of length 5. This variable is required part of the
	// template. If not provided, validation will fail.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Template string `json:"template,omitempty"`
}

// MachineDeploymentDeletionSpec contains configuration options for MachineDeployment deletion.
// +kubebuilder:validation:MinProperties=1
type MachineDeploymentDeletionSpec struct {
	// order defines the order in which Machines are deleted when downscaling.
	// Defaults to "Random".  Valid values are "Random, "Newest", "Oldest"
	// +optional
	Order MachineSetDeletionOrder `json:"order,omitempty"`
}

// MachineDeploymentStatus defines the observed state of MachineDeployment.
// +kubebuilder:validation:MinProperties=1
type MachineDeploymentStatus struct {
	// conditions represents the observations of a MachineDeployment's current state.
	// Known condition types are Available, MachinesReady, MachinesUpToDate, ScalingUp, ScalingDown, Remediating, Deleting, Paused.
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=32
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// observedGeneration is the generation observed by the deployment controller.
	// +optional
	// +kubebuilder:validation:Minimum=1
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// selector is the same as the label selector but in the string format to avoid introspection
	// by clients. The string will be in the same format as the query-param syntax.
	// More info about label selectors: http://kubernetes.io/docs/user-guide/labels#label-selectors
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=4096
	Selector string `json:"selector,omitempty"`

	// replicas is the total number of non-terminated machines targeted by this deployment
	// (their labels match the selector).
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// readyReplicas is the number of ready replicas for this MachineDeployment. A machine is considered ready when Machine's Ready condition is true.
	// +optional
	ReadyReplicas *int32 `json:"readyReplicas,omitempty"`

	// availableReplicas is the number of available replicas for this MachineDeployment. A machine is considered available when Machine's Available condition is true.
	// +optional
	AvailableReplicas *int32 `json:"availableReplicas,omitempty"`

	// upToDateReplicas is the number of up-to-date replicas targeted by this deployment. A machine is considered up-to-date when Machine's UpToDate condition is true.
	// +optional
	UpToDateReplicas *int32 `json:"upToDateReplicas,omitempty"`

	// phase represents the current phase of a MachineDeployment (ScalingUp, ScalingDown, Running, Failed, or Unknown).
	// +optional
	// +kubebuilder:validation:Enum=ScalingUp;ScalingDown;Running;Failed;Unknown
	Phase string `json:"phase,omitempty"`

	// deprecated groups all the status fields that are deprecated and will be removed when all the nested field are removed.
	// +optional
	Deprecated *MachineDeploymentDeprecatedStatus `json:"deprecated,omitempty"`
}

// MachineDeploymentDeprecatedStatus groups all the status fields that are deprecated and will be removed in a future version.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type MachineDeploymentDeprecatedStatus struct {
	// v1beta1 groups all the status fields that are deprecated and will be removed when support for v1beta1 will be dropped.
	// +optional
	V1Beta1 *MachineDeploymentV1Beta1DeprecatedStatus `json:"v1beta1,omitempty"`
}

// MachineDeploymentV1Beta1DeprecatedStatus groups all the status fields that are deprecated and will be removed when support for v1beta1 will be dropped.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type MachineDeploymentV1Beta1DeprecatedStatus struct {
	// conditions defines current service state of the MachineDeployment.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	Conditions Conditions `json:"conditions,omitempty"`

	// updatedReplicas is the total number of non-terminated machines targeted by this deployment
	// that have the desired template spec.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	UpdatedReplicas int32 `json:"updatedReplicas"` //nolint:kubeapilinter // field will be removed when v1beta1 is removed

	// readyReplicas is the total number of ready machines targeted by this deployment.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	ReadyReplicas int32 `json:"readyReplicas"` //nolint:kubeapilinter // field will be removed when v1beta1 is removed

	// availableReplicas is the total number of available machines (ready for at least minReadySeconds)
	// targeted by this deployment.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	AvailableReplicas int32 `json:"availableReplicas"` //nolint:kubeapilinter // field will be removed when v1beta1 is removed

	// unavailableReplicas is the total number of unavailable machines targeted by this deployment.
	// This is the total number of machines that are still required for
	// the deployment to have 100% available capacity. They may either
	// be machines that are running but not yet available or machines
	// that still have not been created.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	UnavailableReplicas int32 `json:"unavailableReplicas"` //nolint:kubeapilinter // field will be removed when v1beta1 is removed
}

// MachineDeploymentPhase indicates the progress of the machine deployment.
type MachineDeploymentPhase string

const (
	// MachineDeploymentPhaseScalingUp indicates the MachineDeployment is scaling up.
	MachineDeploymentPhaseScalingUp = MachineDeploymentPhase("ScalingUp")

	// MachineDeploymentPhaseScalingDown indicates the MachineDeployment is scaling down.
	MachineDeploymentPhaseScalingDown = MachineDeploymentPhase("ScalingDown")

	// MachineDeploymentPhaseRunning indicates scaling has completed and all Machines are running.
	MachineDeploymentPhaseRunning = MachineDeploymentPhase("Running")

	// MachineDeploymentPhaseFailed indicates there was a problem scaling and user intervention might be required.
	//
	// Deprecated: This enum value is deprecated; the Failed phase won't be set anymore by controllers, and it is preserved only
	// for conversion from v1beta1 objects; the Failed phase is going to be removed when support for v1beta1 will be dropped.
	// Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	MachineDeploymentPhaseFailed = MachineDeploymentPhase("Failed")

	// MachineDeploymentPhaseUnknown indicates the state of the MachineDeployment cannot be determined.
	MachineDeploymentPhaseUnknown = MachineDeploymentPhase("Unknown")
)

// SetTypedPhase sets the Phase field to the string representation of MachineDeploymentPhase.
func (md *MachineDeploymentStatus) SetTypedPhase(p MachineDeploymentPhase) {
	md.Phase = string(p)
}

// GetTypedPhase attempts to parse the Phase field and return
// the typed MachineDeploymentPhase representation.
func (md *MachineDeploymentStatus) GetTypedPhase() MachineDeploymentPhase {
	switch phase := MachineDeploymentPhase(md.Phase); phase {
	case
		MachineDeploymentPhaseScalingDown,
		MachineDeploymentPhaseScalingUp,
		MachineDeploymentPhaseRunning,
		MachineDeploymentPhaseFailed:
		return phase
	default:
		return MachineDeploymentPhaseUnknown
	}
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=machinedeployments,shortName=md,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas,selectorpath=.status.selector
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".spec.clusterName",description="Cluster"
// +kubebuilder:printcolumn:name="Available",type="string",JSONPath=`.status.conditions[?(@.type=="Available")].status`,description="Cluster pass all availability checks"
// +kubebuilder:printcolumn:name="Desired",type=integer,JSONPath=".spec.replicas",description="The desired number of machines"
// +kubebuilder:printcolumn:name="Current",type="integer",JSONPath=".status.replicas",description="The number of machines"
// +kubebuilder:printcolumn:name="Ready",type="integer",JSONPath=".status.readyReplicas",description="The number of machines with Ready condition true"
// +kubebuilder:printcolumn:name="Available",type=integer,JSONPath=".status.availableReplicas",description="The number of machines with Available condition true"
// +kubebuilder:printcolumn:name="Up-to-date",type=integer,JSONPath=".status.upToDateReplicas",description="The number of machines with UpToDate condition true"
// +kubebuilder:printcolumn:name="Paused",type="string",JSONPath=`.status.conditions[?(@.type=="Paused")].status`,description="Reconciliation paused",priority=10
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase",description="MachineDeployment status such as ScalingUp/ScalingDown/Running/Failed/Unknown"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of MachineDeployment"
// +kubebuilder:printcolumn:name="Version",type="string",JSONPath=".spec.template.spec.version",description="Kubernetes version associated with this MachineDeployment"

// MachineDeployment is the Schema for the machinedeployments API.
type MachineDeployment struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec is the desired state of MachineDeployment.
	// +required
	Spec MachineDeploymentSpec `json:"spec,omitempty,omitzero"`
	// status is the observed state of MachineDeployment.
	// +optional
	Status MachineDeploymentStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// MachineDeploymentList contains a list of MachineDeployment.
type MachineDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard list's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#lists-and-simple-kinds
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`
	// items is the list of MachineDeployments.
	Items []MachineDeployment `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &MachineDeployment{}, &MachineDeploymentList{})
}

// GetV1Beta1Conditions returns the set of conditions for the machinedeployment.
func (m *MachineDeployment) GetV1Beta1Conditions() Conditions {
	if m.Status.Deprecated == nil || m.Status.Deprecated.V1Beta1 == nil {
		return nil
	}
	return m.Status.Deprecated.V1Beta1.Conditions
}

// SetV1Beta1Conditions updates the set of conditions on the machinedeployment.
func (m *MachineDeployment) SetV1Beta1Conditions(conditions Conditions) {
	if m.Status.Deprecated == nil {
		m.Status.Deprecated = &MachineDeploymentDeprecatedStatus{}
	}
	if m.Status.Deprecated.V1Beta1 == nil {
		m.Status.Deprecated.V1Beta1 = &MachineDeploymentV1Beta1DeprecatedStatus{}
	}
	m.Status.Deprecated.V1Beta1.Conditions = conditions
}

// GetConditions returns the set of conditions for this object.
func (m *MachineDeployment) GetConditions() []metav1.Condition {
	return m.Status.Conditions
}

// SetConditions sets conditions for an API object.
func (m *MachineDeployment) SetConditions(conditions []metav1.Condition) {
	m.Status.Conditions = conditions
}
