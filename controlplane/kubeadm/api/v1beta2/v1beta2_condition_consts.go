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

import clusterv1 "sigs.k8s.io/cluster-api/api/v1beta2"

// KubeadmControlPlane's Available condition and corresponding reasons that will be used in v1Beta2 API version.
const (
	// KubeadmControlPlaneAvailableV1Beta2Condition is true if KubeadmControlPlane is not deleted, `CertificatesAvailable` is true,
	// at least one Machine with healthy control plane components, and etcd has enough operational members to meet quorum requirements.
	// More specifically, considering how kubeadm layouts components:
	// -  Kubernetes API server, scheduler and controller manager health is inferred by the status of
	//    the corresponding Pods hosted on each machine.
	// -  In case of managed etcd, also a healthy etcd Pod and a healthy etcd member must exist on the same
	//    machine with the healthy Kubernetes API server, scheduler and controller manager, otherwise the k8s control
	//    plane cannot be considered operational (if etcd is not operational on a machine, most likely also API server,
	//    scheduler and controller manager on the same machine will be impacted).
	// -  In case of external etcd, KCP cannot make any assumption on etcd status, so all the etcd checks are skipped.
	//
	// Please note that when this condition is true, partial unavailability will be surfaced in the condition message,
	// but with a 10s delay to ensure flakes do not impact condition stability.
	KubeadmControlPlaneAvailableV1Beta2Condition = clusterv1.AvailableV1Beta2Condition

	// KubeadmControlPlaneAvailableInspectionFailedV1Beta2Reason documents a failure when inspecting the status of the
	// etcd cluster hosted on KubeadmControlPlane controlled machines.
	KubeadmControlPlaneAvailableInspectionFailedV1Beta2Reason = clusterv1.InspectionFailedV1Beta2Reason

	// KubeadmControlPlaneAvailableV1Beta2Reason surfaces when the KubeadmControlPlane is available.
	KubeadmControlPlaneAvailableV1Beta2Reason = clusterv1.AvailableV1Beta2Reason

	// KubeadmControlPlaneNotAvailableV1Beta2Reason surfaces when the KubeadmControlPlane is not available.
	KubeadmControlPlaneNotAvailableV1Beta2Reason = clusterv1.NotAvailableV1Beta2Reason
)

// KubeadmControlPlane's Initialized condition and corresponding reasons that will be used in v1Beta2 API version.
const (
	// KubeadmControlPlaneInitializedV1Beta2Condition is true when the control plane is functional enough to accept
	// requests. This information is usually used as a signal for starting all the provisioning operations that
	// depend on a functional API server, but do not require a full HA control plane to exist.
	KubeadmControlPlaneInitializedV1Beta2Condition = "Initialized"

	// KubeadmControlPlaneInitializedV1Beta2Reason surfaces when the control plane is initialized.
	KubeadmControlPlaneInitializedV1Beta2Reason = "Initialized"

	// KubeadmControlPlaneNotInitializedV1Beta2Reason surfaces when the control plane is not initialized.
	KubeadmControlPlaneNotInitializedV1Beta2Reason = "NotInitialized"
)

// KubeadmControlPlane's CertificatesAvailable condition and corresponding reasons that will be used in v1Beta2 API version.
const (
	// KubeadmControlPlaneCertificatesAvailableV1Beta2Condition True if all the cluster certificates exist.
	KubeadmControlPlaneCertificatesAvailableV1Beta2Condition = "CertificatesAvailable"

	// KubeadmControlPlaneCertificatesInternalErrorV1Beta2Reason surfaces unexpected failures when reconciling cluster certificates.
	KubeadmControlPlaneCertificatesInternalErrorV1Beta2Reason = clusterv1.InternalErrorV1Beta2Reason

	// KubeadmControlPlaneCertificatesAvailableV1Beta2Reason surfaces when cluster certificates are available,
	// no matter if those certificates have been provided by the user or generated by  KubeadmControlPlane itself.
	// Cluster certificates include: certificate authorities for ca, sa, front-proxy, etcd, and if external etcd is used,
	// also the apiserver-etcd-client client certificate.
	KubeadmControlPlaneCertificatesAvailableV1Beta2Reason = clusterv1.AvailableV1Beta2Reason
)

// KubeadmControlPlane's EtcdClusterHealthy condition and corresponding reasons that will be used in v1Beta2 API version.
const (
	// KubeadmControlPlaneEtcdClusterHealthyV1Beta2Condition surfaces issues to etcd cluster hosted on machines managed by this object.
	// It is computed as aggregation of Machine's EtcdMemberHealthy conditions plus additional checks validating
	// potential issues to etcd quorum.
	// Note: this condition is not set when using an external etcd.
	KubeadmControlPlaneEtcdClusterHealthyV1Beta2Condition = "EtcdClusterHealthy"

	// KubeadmControlPlaneEtcdClusterInspectionFailedV1Beta2Reason documents a failure when inspecting the status of the
	// etcd cluster hosted on KubeadmControlPlane controlled machines.
	KubeadmControlPlaneEtcdClusterInspectionFailedV1Beta2Reason = clusterv1.InspectionFailedV1Beta2Reason

	// KubeadmControlPlaneEtcdClusterConnectionDownV1Beta2Reason surfaces that the connection to the workload
	// cluster is down.
	KubeadmControlPlaneEtcdClusterConnectionDownV1Beta2Reason = clusterv1.ConnectionDownV1Beta2Reason

	// KubeadmControlPlaneEtcdClusterHealthyV1Beta2Reason surfaces when the etcd cluster hosted on KubeadmControlPlane
	// machines is healthy.
	KubeadmControlPlaneEtcdClusterHealthyV1Beta2Reason = "Healthy"

	// KubeadmControlPlaneEtcdClusterNotHealthyV1Beta2Reason surfaces when the etcd cluster hosted on KubeadmControlPlane
	// machines is not healthy.
	KubeadmControlPlaneEtcdClusterNotHealthyV1Beta2Reason = "NotHealthy"

	// KubeadmControlPlaneEtcdClusterHealthUnknownV1Beta2Reason surfaces when the health status of the etcd cluster hosted
	// on KubeadmControlPlane machines is unknown.
	KubeadmControlPlaneEtcdClusterHealthUnknownV1Beta2Reason = "HealthUnknown"
)

// KubeadmControlPlane's ControlPlaneComponentsHealthy condition and corresponding reasons that will be used in v1Beta2 API version.
const (
	// KubeadmControlPlaneControlPlaneComponentsHealthyV1Beta2Condition surfaces issues to Kubernetes control plane components
	// hosted on machines managed by this object. It is computed as aggregation of Machine's `APIServerPodHealthy`,
	// `ControllerManagerPodHealthy`, `SchedulerPodHealthy`, `EtcdPodHealthy` conditions plus additional checks on
	// control plane machines and nodes.
	KubeadmControlPlaneControlPlaneComponentsHealthyV1Beta2Condition = "ControlPlaneComponentsHealthy"

	// KubeadmControlPlaneControlPlaneComponentsInspectionFailedV1Beta2Reason documents a failure when inspecting the status of the
	// control plane components hosted on KubeadmControlPlane controlled machines.
	KubeadmControlPlaneControlPlaneComponentsInspectionFailedV1Beta2Reason = clusterv1.InspectionFailedV1Beta2Reason

	// KubeadmControlPlaneControlPlaneComponentsConnectionDownV1Beta2Reason surfaces that the connection to the workload
	// cluster is down.
	KubeadmControlPlaneControlPlaneComponentsConnectionDownV1Beta2Reason = clusterv1.ConnectionDownV1Beta2Reason

	// KubeadmControlPlaneControlPlaneComponentsHealthyV1Beta2Reason surfaces when the Kubernetes control plane components
	// hosted on KubeadmControlPlane machines are healthy.
	KubeadmControlPlaneControlPlaneComponentsHealthyV1Beta2Reason = "Healthy"

	// KubeadmControlPlaneControlPlaneComponentsNotHealthyV1Beta2Reason surfaces when the Kubernetes control plane components
	// hosted on KubeadmControlPlane machines are not healthy.
	KubeadmControlPlaneControlPlaneComponentsNotHealthyV1Beta2Reason = "NotHealthy"

	// KubeadmControlPlaneControlPlaneComponentsHealthUnknownV1Beta2Reason surfaces when the health status of the
	// Kubernetes control plane components hosted on KubeadmControlPlane machines is unknown.
	KubeadmControlPlaneControlPlaneComponentsHealthUnknownV1Beta2Reason = "HealthUnknown"
)

// KubeadmControlPlane's MachinesReady condition and corresponding reasons that will be used in v1Beta2 API version.
const (
	// KubeadmControlPlaneMachinesReadyV1Beta2Condition surfaces detail of issues on the controlled machines, if any.
	// Please note this will include also APIServerPodHealthy, ControllerManagerPodHealthy, SchedulerPodHealthy conditions.
	// If not using an external etcd also EtcdPodHealthy, EtcdMemberHealthy conditions are included.
	KubeadmControlPlaneMachinesReadyV1Beta2Condition = clusterv1.MachinesReadyV1Beta2Condition

	// KubeadmControlPlaneMachinesReadyV1Beta2Reason surfaces when all the controlled machine's Ready conditions are true.
	KubeadmControlPlaneMachinesReadyV1Beta2Reason = clusterv1.ReadyV1Beta2Reason

	// KubeadmControlPlaneMachinesNotReadyV1Beta2Reason surfaces when at least one of the controlled machine's Ready conditions is false.
	KubeadmControlPlaneMachinesNotReadyV1Beta2Reason = clusterv1.NotReadyV1Beta2Reason

	// KubeadmControlPlaneMachinesReadyUnknownV1Beta2Reason surfaces when at least one of the controlled machine's Ready conditions is unknown
	// and no one of the controlled machine's Ready conditions is false.
	KubeadmControlPlaneMachinesReadyUnknownV1Beta2Reason = clusterv1.ReadyUnknownV1Beta2Reason

	// KubeadmControlPlaneMachinesReadyNoReplicasV1Beta2Reason surfaces when no machines exist for the KubeadmControlPlane.
	KubeadmControlPlaneMachinesReadyNoReplicasV1Beta2Reason = clusterv1.NoReplicasV1Beta2Reason

	// KubeadmControlPlaneMachinesReadyInternalErrorV1Beta2Reason surfaces unexpected failures when computing the MachinesReady condition.
	KubeadmControlPlaneMachinesReadyInternalErrorV1Beta2Reason = clusterv1.InternalErrorV1Beta2Reason
)

// KubeadmControlPlane's MachinesUpToDate condition and corresponding reasons that will be used in v1Beta2 API version.
const (
	// KubeadmControlPlaneMachinesUpToDateV1Beta2Condition surfaces details of controlled machines not up to date, if any.
	// Note: New machines are considered 10s after machine creation. This gives time to the machine's owner controller to recognize the new machine and add the UpToDate condition.
	KubeadmControlPlaneMachinesUpToDateV1Beta2Condition = clusterv1.MachinesUpToDateV1Beta2Condition

	// KubeadmControlPlaneMachinesUpToDateV1Beta2Reason surfaces when all the controlled machine's UpToDate conditions are true.
	KubeadmControlPlaneMachinesUpToDateV1Beta2Reason = clusterv1.UpToDateV1Beta2Reason

	// KubeadmControlPlaneMachinesNotUpToDateV1Beta2Reason surfaces when at least one of the controlled machine's UpToDate conditions is false.
	KubeadmControlPlaneMachinesNotUpToDateV1Beta2Reason = clusterv1.NotUpToDateV1Beta2Reason

	// KubeadmControlPlaneMachinesUpToDateUnknownV1Beta2Reason surfaces when at least one of the controlled machine's UpToDate conditions is unknown
	// and no one of the controlled machine's UpToDate conditions is false.
	KubeadmControlPlaneMachinesUpToDateUnknownV1Beta2Reason = clusterv1.UpToDateUnknownV1Beta2Reason

	// KubeadmControlPlaneMachinesUpToDateNoReplicasV1Beta2Reason surfaces when no machines exist for the KubeadmControlPlane.
	KubeadmControlPlaneMachinesUpToDateNoReplicasV1Beta2Reason = clusterv1.NoReplicasV1Beta2Reason

	// KubeadmControlPlaneMachinesUpToDateInternalErrorV1Beta2Reason surfaces unexpected failures when computing the MachinesUpToDate condition.
	KubeadmControlPlaneMachinesUpToDateInternalErrorV1Beta2Reason = clusterv1.InternalErrorV1Beta2Reason
)

// KubeadmControlPlane's RollingOut condition and corresponding reasons that will be used in v1Beta2 API version.
const (
	// KubeadmControlPlaneRollingOutV1Beta2Condition  is true if there is at least one machine not up-to-date.
	KubeadmControlPlaneRollingOutV1Beta2Condition = clusterv1.RollingOutV1Beta2Condition

	// KubeadmControlPlaneRollingOutV1Beta2Reason  surfaces when there is at least one machine not up-to-date.
	KubeadmControlPlaneRollingOutV1Beta2Reason = clusterv1.RollingOutV1Beta2Reason

	// KubeadmControlPlaneNotRollingOutV1Beta2Reason surfaces when all the machines are up-to-date.
	KubeadmControlPlaneNotRollingOutV1Beta2Reason = clusterv1.NotRollingOutV1Beta2Reason
)

// KubeadmControlPlane's ScalingUp condition and corresponding reasons that will be used in v1Beta2 API version.
const (
	// KubeadmControlPlaneScalingUpV1Beta2Condition is true if actual replicas < desired replicas.
	// Note: In case a KubeadmControlPlane preflight check is preventing scale up, this will surface in the condition message.
	KubeadmControlPlaneScalingUpV1Beta2Condition = clusterv1.ScalingUpV1Beta2Condition

	// KubeadmControlPlaneScalingUpV1Beta2Reason surfaces when actual replicas < desired replicas.
	KubeadmControlPlaneScalingUpV1Beta2Reason = clusterv1.ScalingUpV1Beta2Reason

	// KubeadmControlPlaneNotScalingUpV1Beta2Reason surfaces when actual replicas >= desired replicas.
	KubeadmControlPlaneNotScalingUpV1Beta2Reason = clusterv1.NotScalingUpV1Beta2Reason

	// KubeadmControlPlaneScalingUpWaitingForReplicasSetV1Beta2Reason surfaces when the .spec.replicas
	// field of the KubeadmControlPlane is not set.
	KubeadmControlPlaneScalingUpWaitingForReplicasSetV1Beta2Reason = clusterv1.WaitingForReplicasSetV1Beta2Reason
)

// KubeadmControlPlane's ScalingDown condition and corresponding reasons that will be used in v1Beta2 API version.
const (
	// KubeadmControlPlaneScalingDownV1Beta2Condition is true if actual replicas > desired replicas.
	// Note: In case a KubeadmControlPlane preflight check is preventing scale down, this will surface in the condition message.
	KubeadmControlPlaneScalingDownV1Beta2Condition = clusterv1.ScalingDownV1Beta2Condition

	// KubeadmControlPlaneScalingDownV1Beta2Reason surfaces when actual replicas > desired replicas.
	KubeadmControlPlaneScalingDownV1Beta2Reason = clusterv1.ScalingDownV1Beta2Reason

	// KubeadmControlPlaneNotScalingDownV1Beta2Reason surfaces when actual replicas <= desired replicas.
	KubeadmControlPlaneNotScalingDownV1Beta2Reason = clusterv1.NotScalingDownV1Beta2Reason

	// KubeadmControlPlaneScalingDownWaitingForReplicasSetV1Beta2Reason surfaces when the .spec.replicas
	// field of the KubeadmControlPlane is not set.
	KubeadmControlPlaneScalingDownWaitingForReplicasSetV1Beta2Reason = clusterv1.WaitingForReplicasSetV1Beta2Reason
)

// KubeadmControlPlane's Remediating condition and corresponding reasons that will be used in v1Beta2 API version.
const (
	// KubeadmControlPlaneRemediatingV1Beta2Condition surfaces details about ongoing remediation of the controlled machines, if any.
	// Note: KubeadmControlPlane only remediates machines with HealthCheckSucceeded set to false and with the OwnerRemediated condition set to false.
	KubeadmControlPlaneRemediatingV1Beta2Condition = clusterv1.RemediatingV1Beta2Condition

	// KubeadmControlPlaneRemediatingV1Beta2Reason surfaces when the KubeadmControlPlane has at least one machine with HealthCheckSucceeded set to false
	// and with the OwnerRemediated condition set to false.
	KubeadmControlPlaneRemediatingV1Beta2Reason = clusterv1.RemediatingV1Beta2Reason

	// KubeadmControlPlaneNotRemediatingV1Beta2Reason surfaces when the KubeadmControlPlane does not have any machine with HealthCheckSucceeded set to false
	// and with the OwnerRemediated condition set to false.
	KubeadmControlPlaneNotRemediatingV1Beta2Reason = clusterv1.NotRemediatingV1Beta2Reason

	// KubeadmControlPlaneRemediatingInternalErrorV1Beta2Reason surfaces unexpected failures when computing the Remediating condition.
	KubeadmControlPlaneRemediatingInternalErrorV1Beta2Reason = clusterv1.InternalErrorV1Beta2Reason
)

// Reasons that will be used for the OwnerRemediated condition set by MachineHealthCheck on KubeadmControlPlane controlled machines
// being remediated in v1Beta2 API version.
const (
	// KubeadmControlPlaneMachineRemediationInternalErrorV1Beta2Reason surfaces unexpected failures while remediating a control plane machine.
	KubeadmControlPlaneMachineRemediationInternalErrorV1Beta2Reason = clusterv1.InternalErrorV1Beta2Reason

	// KubeadmControlPlaneMachineCannotBeRemediatedV1Beta2Reason surfaces when remediation of a control plane machine can't be started.
	KubeadmControlPlaneMachineCannotBeRemediatedV1Beta2Reason = "CannotBeRemediated"

	// KubeadmControlPlaneMachineRemediationDeferredV1Beta2Reason surfaces when remediation of a control plane machine must be deferred.
	KubeadmControlPlaneMachineRemediationDeferredV1Beta2Reason = "RemediationDeferred"

	// KubeadmControlPlaneMachineRemediationMachineDeletingV1Beta2Reason surfaces when remediation of a control plane machine
	// has been completed by deleting the unhealthy machine.
	// Note: After an unhealthy machine is deleted, a new one is created by the KubeadmControlPlaneMachine as part of the
	// regular reconcile loop that ensures the correct number of replicas exist; KubeadmControlPlane machine waits for
	// the new machine to exists before removing the controlplane.cluster.x-k8s.io/remediation-in-progress annotation.
	// This is part of a series of safeguards to ensure that operation are performed sequentially on control plane machines.
	KubeadmControlPlaneMachineRemediationMachineDeletingV1Beta2Reason = "MachineDeleting"
)

// KubeadmControlPlane's Deleting condition and corresponding reasons that will be used in v1Beta2 API version.
const (
	// KubeadmControlPlaneDeletingV1Beta2Condition surfaces details about ongoing deletion of the controlled machines.
	KubeadmControlPlaneDeletingV1Beta2Condition = clusterv1.DeletingV1Beta2Condition

	// KubeadmControlPlaneNotDeletingV1Beta2Reason surfaces when the KCP is not deleting because the
	// DeletionTimestamp is not set.
	KubeadmControlPlaneNotDeletingV1Beta2Reason = clusterv1.NotDeletingV1Beta2Reason

	// KubeadmControlPlaneDeletingWaitingForWorkersDeletionV1Beta2Reason surfaces when the KCP deletion
	// waits for the workers to be deleted.
	KubeadmControlPlaneDeletingWaitingForWorkersDeletionV1Beta2Reason = "WaitingForWorkersDeletion"

	// KubeadmControlPlaneDeletingWaitingForMachineDeletionV1Beta2Reason surfaces when the KCP deletion
	// waits for the control plane Machines to be deleted.
	KubeadmControlPlaneDeletingWaitingForMachineDeletionV1Beta2Reason = "WaitingForMachineDeletion"

	// KubeadmControlPlaneDeletingDeletionCompletedV1Beta2Reason surfaces when the KCP deletion has been completed.
	// This reason is set right after the `kubeadm.controlplane.cluster.x-k8s.io` finalizer is removed.
	// This means that the object will go away (i.e. be removed from etcd), except if there are other
	// finalizers on the KCP object.
	KubeadmControlPlaneDeletingDeletionCompletedV1Beta2Reason = clusterv1.DeletionCompletedV1Beta2Reason

	// KubeadmControlPlaneDeletingInternalErrorV1Beta2Reason surfaces unexpected failures when deleting a KCP object.
	KubeadmControlPlaneDeletingInternalErrorV1Beta2Reason = clusterv1.InternalErrorV1Beta2Reason
)

// APIServerPodHealthy, ControllerManagerPodHealthy, SchedulerPodHealthy and EtcdPodHealthy condition and corresponding
// reasons that will be used for KubeadmControlPlane controlled machines in v1Beta2 API version.
const (
	// KubeadmControlPlaneMachineAPIServerPodHealthyV1Beta2Condition surfaces the status of the API server pod hosted on a KubeadmControlPlane controlled machine.
	KubeadmControlPlaneMachineAPIServerPodHealthyV1Beta2Condition = "APIServerPodHealthy"

	// KubeadmControlPlaneMachineControllerManagerPodHealthyV1Beta2Condition surfaces the status of the controller manager pod hosted on a KubeadmControlPlane controlled machine.
	KubeadmControlPlaneMachineControllerManagerPodHealthyV1Beta2Condition = "ControllerManagerPodHealthy"

	// KubeadmControlPlaneMachineSchedulerPodHealthyV1Beta2Condition surfaces the status of the scheduler pod hosted on a KubeadmControlPlane controlled machine.
	KubeadmControlPlaneMachineSchedulerPodHealthyV1Beta2Condition = "SchedulerPodHealthy"

	// KubeadmControlPlaneMachineEtcdPodHealthyV1Beta2Condition surfaces the status of the etcd pod hosted on a KubeadmControlPlane controlled machine.
	KubeadmControlPlaneMachineEtcdPodHealthyV1Beta2Condition = "EtcdPodHealthy"

	// KubeadmControlPlaneMachinePodRunningV1Beta2Reason surfaces a pod hosted on a KubeadmControlPlane controlled machine that is running.
	KubeadmControlPlaneMachinePodRunningV1Beta2Reason = "Running"

	// KubeadmControlPlaneMachinePodProvisioningV1Beta2Reason surfaces a pod hosted on a KubeadmControlPlane controlled machine
	// waiting to be provisioned i.e., Pod is in "Pending" phase.
	KubeadmControlPlaneMachinePodProvisioningV1Beta2Reason = "Provisioning"

	// KubeadmControlPlaneMachinePodDoesNotExistV1Beta2Reason surfaces a when a pod hosted on a KubeadmControlPlane controlled machine
	// does not exist.
	KubeadmControlPlaneMachinePodDoesNotExistV1Beta2Reason = "DoesNotExist"

	// KubeadmControlPlaneMachinePodFailedV1Beta2Reason surfaces a when a pod hosted on a KubeadmControlPlane controlled machine
	// failed during provisioning, e.g. CrashLoopBackOff, ImagePullBackOff or if all the containers in a pod have terminated.
	KubeadmControlPlaneMachinePodFailedV1Beta2Reason = "Failed"

	// KubeadmControlPlaneMachinePodInspectionFailedV1Beta2Reason documents a failure when inspecting the status of a
	// pod hosted on a KubeadmControlPlane controlled machine.
	KubeadmControlPlaneMachinePodInspectionFailedV1Beta2Reason = clusterv1.InspectionFailedV1Beta2Reason

	// KubeadmControlPlaneMachinePodConnectionDownV1Beta2Reason surfaces that the connection to the workload
	// cluster is down.
	KubeadmControlPlaneMachinePodConnectionDownV1Beta2Reason = clusterv1.ConnectionDownV1Beta2Reason

	// KubeadmControlPlaneMachinePodDeletingV1Beta2Reason surfaces when the machine hosting control plane components
	// is being deleted.
	KubeadmControlPlaneMachinePodDeletingV1Beta2Reason = "Deleting"
)

// EtcdMemberHealthy condition and corresponding reasons that will be used for KubeadmControlPlane controlled machines in v1Beta2 API version.
const (
	// KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Condition surfaces the status of the etcd member hosted on a KubeadmControlPlane controlled machine.
	KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Condition = "EtcdMemberHealthy"

	// KubeadmControlPlaneMachineEtcdMemberNotHealthyV1Beta2Reason surfaces when the etcd member hosted on a KubeadmControlPlane controlled machine is not healthy.
	KubeadmControlPlaneMachineEtcdMemberNotHealthyV1Beta2Reason = "NotHealthy"

	// KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Reason surfaces when the etcd member hosted on a KubeadmControlPlane controlled machine is healthy.
	KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Reason = "Healthy"

	// KubeadmControlPlaneMachineEtcdMemberInspectionFailedV1Beta2Reason documents a failure when inspecting the status of an
	// etcd member hosted on a KubeadmControlPlane controlled machine.
	KubeadmControlPlaneMachineEtcdMemberInspectionFailedV1Beta2Reason = clusterv1.InspectionFailedV1Beta2Reason

	// KubeadmControlPlaneMachineEtcdMemberConnectionDownV1Beta2Reason surfaces that the connection to the workload
	// cluster is down.
	KubeadmControlPlaneMachineEtcdMemberConnectionDownV1Beta2Reason = clusterv1.ConnectionDownV1Beta2Reason

	// KubeadmControlPlaneMachineEtcdMemberDeletingV1Beta2Reason surfaces when the machine hosting an etcd member
	// is being deleted.
	KubeadmControlPlaneMachineEtcdMemberDeletingV1Beta2Reason = "Deleting"
)
