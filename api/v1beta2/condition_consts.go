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

// Conditions types that are used across different objects.
const (
	// AvailableCondition reports if an object is available.
	// Note: This condition type is defined to ensure consistent naming of conditions across objects.
	// Please use object specific variants of this condition which provides more details for each context where
	// the same condition type exists.
	AvailableCondition = "Available"

	// ReadyCondition reports if an object is ready.
	// Note: This condition type is defined to ensure consistent naming of conditions across objects.
	// Please use object specific variants of this condition which provides more details for each context where
	// the same condition type exists.
	ReadyCondition = "Ready"

	// BootstrapConfigReadyCondition reports if an object's bootstrap config is ready.
	// Note: This condition type is defined to ensure consistent naming of conditions across objects.
	// Please use object specific variants of this condition which provides more details for each context where
	// the same condition type exists.
	BootstrapConfigReadyCondition = "BootstrapConfigReady"

	// InfrastructureReadyCondition reports if an object's infrastructure is ready.
	// Note: This condition type is defined to ensure consistent naming of conditions across objects.
	// Please use object specific variants of this condition which provides more details for each context where
	// the same condition type exists.
	InfrastructureReadyCondition = "InfrastructureReady"

	// MachinesReadyCondition surfaces detail of issues on the controlled machines, if any.
	// Note: This condition type is defined to ensure consistent naming of conditions across objects.
	// Please use object specific variants of this condition which provides more details for each context where
	// the same condition type exists.
	MachinesReadyCondition = "MachinesReady"

	// MachinesUpToDateCondition surfaces details of controlled machines not up to date, if any.
	// Note: This condition type is defined to ensure consistent naming of conditions across objects.
	// Please use object specific variants of this condition which provides more details for each context where
	// the same condition type exists.
	MachinesUpToDateCondition = "MachinesUpToDate"

	// RollingOutCondition reports if an object is rolling out changes to machines; Cluster API usually
	// rolls out changes to machines by replacing not up-to-date machines with new ones.
	// Note: This condition type is defined to ensure consistent naming of conditions across objects.
	// Please use object specific variants of this condition which provides more details for each context where
	// the same condition type exists.
	RollingOutCondition = "RollingOut"

	// ScalingUpCondition reports if an object is scaling up.
	// Note: This condition type is defined to ensure consistent naming of conditions across objects.
	// Please use object specific variants of this condition which provides more details for each context where
	// the same condition type exists.
	ScalingUpCondition = "ScalingUp"

	// ScalingDownCondition reports if an object is scaling down.
	// Note: This condition type is defined to ensure consistent naming of conditions across objects.
	// Please use object specific variants of this condition which provides more details for each context where
	// the same condition type exists.
	ScalingDownCondition = "ScalingDown"

	// RemediatingCondition surfaces details about ongoing remediation of the controlled machines, if any.
	// Note: This condition type is defined to ensure consistent naming of conditions across objects.
	// Please use object specific variants of this condition which provides more details for each context where
	// the same condition type exists.
	RemediatingCondition = "Remediating"

	// DeletingCondition surfaces details about progress of the object deletion workflow.
	// Note: This condition type is defined to ensure consistent naming of conditions across objects.
	// Please use object specific variants of this condition which provides more details for each context where
	// the same condition type exists.
	DeletingCondition = "Deleting"

	// PausedCondition reports if reconciliation for an object or the cluster is paused.
	// Note: This condition type is defined to ensure consistent naming of conditions across objects.
	// Please use object specific variants of this condition which provides more details for each context where
	// the same condition type exists.
	PausedCondition = "Paused"
)

// Reasons that are used across different objects.
const (
	// AvailableReason applies to a condition surfacing object availability.
	AvailableReason = "Available"

	// NotAvailableReason applies to a condition surfacing object not satisfying availability criteria.
	NotAvailableReason = "NotAvailable"

	// AvailableUnknownReason applies to a condition surfacing object availability unknown.
	AvailableUnknownReason = "AvailableUnknown"

	// ReadyReason applies to a condition surfacing object readiness.
	ReadyReason = "Ready"

	// NotReadyReason applies to a condition surfacing object not satisfying readiness criteria.
	NotReadyReason = "NotReady"

	// ReadyUnknownReason applies to a condition surfacing object readiness unknown.
	ReadyUnknownReason = "ReadyUnknown"

	// UpToDateReason applies to a condition surfacing object up-tp-date.
	UpToDateReason = "UpToDate"

	// NotUpToDateReason applies to a condition surfacing object not up-tp-date.
	NotUpToDateReason = "NotUpToDate"

	// UpToDateUnknownReason applies to a condition surfacing object up-tp-date unknown.
	UpToDateUnknownReason = "UpToDateUnknown"

	// RollingOutReason surfaces when an object is rolling out.
	RollingOutReason = "RollingOut"

	// NotRollingOutReason surfaces when an object is not rolling out.
	NotRollingOutReason = "NotRollingOut"

	// ScalingUpReason surfaces when an object is scaling up.
	ScalingUpReason = "ScalingUp"

	// NotScalingUpReason surfaces when an object is not scaling up.
	NotScalingUpReason = "NotScalingUp"

	// ScalingDownReason surfaces when an object is scaling down.
	ScalingDownReason = "ScalingDown"

	// NotScalingDownReason surfaces when an object is not scaling down.
	NotScalingDownReason = "NotScalingDown"

	// RemediatingReason surfaces when an object owns at least one machine with HealthCheckSucceeded
	// set to false and with the OwnerRemediated condition set to false by the MachineHealthCheck controller.
	RemediatingReason = "Remediating"

	// NotRemediatingReason surfaces when an object does not own any machines with HealthCheckSucceeded
	// set to false and with the OwnerRemediated condition set to false by the MachineHealthCheck controller.
	NotRemediatingReason = "NotRemediating"

	// NoReplicasReason surfaces when an object that manage replicas does not have any.
	NoReplicasReason = "NoReplicas"

	// WaitingForReplicasSetReason surfaces when the replica field of an object is not set.
	WaitingForReplicasSetReason = "WaitingForReplicasSet"

	// InvalidConditionReportedReason applies to a condition, usually read from an external object, that is invalid
	// (e.g. its status is missing).
	InvalidConditionReportedReason = "InvalidConditionReported"

	// InternalErrorReason surfaces unexpected errors reporting by controllers.
	// In most cases, it will be required to look at controllers logs to properly triage those issues.
	InternalErrorReason = "InternalError"

	// ObjectDoesNotExistReason surfaces when a referenced object does not exist.
	ObjectDoesNotExistReason = "ObjectDoesNotExist"

	// ObjectDeletedReason surfaces when a referenced object has been deleted.
	// Note: controllers can't identify if the object was deleted by the controller itself, e.g.
	// during the deletion workflow, or by a users.
	ObjectDeletedReason = "ObjectDeleted"

	// NotPausedReason surfaces when an object is not paused.
	NotPausedReason = "NotPaused"

	// PausedReason surfaces when an object is paused.
	PausedReason = "Paused"

	// ConnectionDownReason surfaces that the connection to the workload cluster is down.
	ConnectionDownReason = "ConnectionDown"

	// NotDeletingReason surfaces when an object is not deleting because the
	// DeletionTimestamp is not set.
	NotDeletingReason = "NotDeleting"

	// DeletingReason surfaces when an object is deleting because the
	// DeletionTimestamp is set. This reason is used if none of the more specific reasons apply.
	DeletingReason = "Deleting"

	// DeletionCompletedReason surfaces when the deletion process has been completed.
	// This reason is set right after the corresponding finalizer is removed.
	// This means that the object will go away (i.e. be removed from etcd), except if there are other
	// finalizers on the object.
	DeletionCompletedReason = "DeletionCompleted"

	// InspectionFailedReason applies to a condition when inspection of the underlying object failed.
	InspectionFailedReason = "InspectionFailed"

	// WaitingForClusterInfrastructureReadyReason documents an infra Machine waiting for the cluster
	// infrastructure to be ready.
	WaitingForClusterInfrastructureReadyReason = "WaitingForClusterInfrastructureReady"

	// WaitingForControlPlaneInitializedReason documents an infra Machine waiting
	// for the control plane to be initialized.
	WaitingForControlPlaneInitializedReason = "WaitingForControlPlaneInitialized"

	// WaitingForBootstrapDataReason documents an infra Machine waiting for the bootstrap
	// data to be ready before starting to create the Machine's infrastructure.
	WaitingForBootstrapDataReason = "WaitingForBootstrapData"

	// ProvisionedReason documents an object or a piece of infrastructure being provisioned.
	ProvisionedReason = "Provisioned"

	// NotProvisionedReason documents an object or a piece of infrastructure is not provisioned.
	NotProvisionedReason = "NotProvisioned"
)
