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

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/errors"
)

// KubeadmControlPlaneRolloutStrategyType defines the rollout strategies for a KubeadmControlPlane.
// +kubebuilder:validation:Enum=RollingUpdate
type KubeadmControlPlaneRolloutStrategyType string

const (
	// RollingUpdateStrategyType replaces the old control planes by new one using rolling update
	// i.e. gradually scale up or down the old control planes and scale up or down the new one.
	RollingUpdateStrategyType KubeadmControlPlaneRolloutStrategyType = "RollingUpdate"
)

const (
	// KubeadmControlPlaneFinalizer is the finalizer applied to KubeadmControlPlane resources
	// by its managing controller.
	KubeadmControlPlaneFinalizer = "kubeadm.controlplane.cluster.x-k8s.io"

	// SkipCoreDNSAnnotation annotation explicitly skips reconciling CoreDNS if set.
	SkipCoreDNSAnnotation = "controlplane.cluster.x-k8s.io/skip-coredns"

	// SkipKubeProxyAnnotation annotation explicitly skips reconciling kube-proxy if set.
	SkipKubeProxyAnnotation = "controlplane.cluster.x-k8s.io/skip-kube-proxy"

	// KubeadmClusterConfigurationAnnotation is a machine annotation that stores the json-marshalled string of KCP ClusterConfiguration.
	// This annotation is used to detect any changes in ClusterConfiguration and trigger machine rollout in KCP.
	KubeadmClusterConfigurationAnnotation = "controlplane.cluster.x-k8s.io/kubeadm-cluster-configuration"

	// RemediationInProgressAnnotation is used to keep track that a KCP remediation is in progress, and more
	// specifically it tracks that the system is in between having deleted an unhealthy machine and recreating its replacement.
	// NOTE: if something external to CAPI removes this annotation the system cannot detect the above situation; this can lead to
	// failures in updating remediation retry or remediation count (both counters restart from zero).
	RemediationInProgressAnnotation = "controlplane.cluster.x-k8s.io/remediation-in-progress"

	// RemediationForAnnotation is used to link a new machine to the unhealthy machine it is replacing;
	// please note that in case of retry, when also the remediating machine fails, the system keeps track of
	// the first machine of the sequence only.
	// NOTE: if something external to CAPI removes this annotation the system this can lead to
	// failures in updating remediation retry (the counter restarts from zero).
	RemediationForAnnotation = "controlplane.cluster.x-k8s.io/remediation-for"

	// PreTerminateHookCleanupAnnotation is the annotation KCP sets on Machines to ensure it can later remove the
	// etcd member right before Machine termination (i.e. before InfraMachine deletion).
	// Note: Starting with Kubernetes v1.31 this hook will wait for all other pre-terminate hooks to finish to
	// ensure it runs last (thus ensuring that kubelet is still working while other pre-terminate hooks run).
	PreTerminateHookCleanupAnnotation = clusterv1.PreTerminateDeleteHookAnnotationPrefix + "/kcp-cleanup"

	// DefaultMinHealthyPeriodSeconds defines the default minimum period before we consider a remediation on a
	// machine unrelated from the previous remediation.
	DefaultMinHealthyPeriodSeconds = int32(60 * 60)
)

// KubeadmControlPlane's Available condition and corresponding reasons.
const (
	// KubeadmControlPlaneAvailableCondition is true if KubeadmControlPlane is not deleted, `CertificatesAvailable` is true,
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
	KubeadmControlPlaneAvailableCondition = clusterv1.AvailableCondition

	// KubeadmControlPlaneAvailableInspectionFailedReason documents a failure when inspecting the status of the
	// etcd cluster hosted on KubeadmControlPlane controlled machines.
	KubeadmControlPlaneAvailableInspectionFailedReason = clusterv1.InspectionFailedReason

	// KubeadmControlPlaneAvailableReason surfaces when the KubeadmControlPlane is available.
	KubeadmControlPlaneAvailableReason = clusterv1.AvailableReason

	// KubeadmControlPlaneNotAvailableReason surfaces when the KubeadmControlPlane is not available.
	KubeadmControlPlaneNotAvailableReason = clusterv1.NotAvailableReason
)

// KubeadmControlPlane's Initialized condition and corresponding reasons.
const (
	// KubeadmControlPlaneInitializedCondition is true when the control plane is functional enough to accept
	// requests. This information is usually used as a signal for starting all the provisioning operations that
	// depend on a functional API server, but do not require a full HA control plane to exist.
	KubeadmControlPlaneInitializedCondition = "Initialized"

	// KubeadmControlPlaneInitializedReason surfaces when the control plane is initialized.
	KubeadmControlPlaneInitializedReason = "Initialized"

	// KubeadmControlPlaneNotInitializedReason surfaces when the control plane is not initialized.
	KubeadmControlPlaneNotInitializedReason = "NotInitialized"
)

// KubeadmControlPlane's CertificatesAvailable condition and corresponding reasons.
const (
	// KubeadmControlPlaneCertificatesAvailableCondition True if all the cluster certificates exist.
	KubeadmControlPlaneCertificatesAvailableCondition = "CertificatesAvailable"

	// KubeadmControlPlaneCertificatesInternalErrorReason surfaces unexpected failures when reconciling cluster certificates.
	KubeadmControlPlaneCertificatesInternalErrorReason = clusterv1.InternalErrorReason

	// KubeadmControlPlaneCertificatesAvailableReason surfaces when cluster certificates are available,
	// no matter if those certificates have been provided by the user or generated by  KubeadmControlPlane itself.
	// Cluster certificates include: certificate authorities for ca, sa, front-proxy, etcd, and if external etcd is used,
	// also the apiserver-etcd-client client certificate.
	KubeadmControlPlaneCertificatesAvailableReason = clusterv1.AvailableReason
)

// KubeadmControlPlane's EtcdClusterHealthy condition and corresponding reasons.
const (
	// KubeadmControlPlaneEtcdClusterHealthyCondition surfaces issues to etcd cluster hosted on machines managed by this object.
	// It is computed as aggregation of Machine's EtcdMemberHealthy conditions plus additional checks validating
	// potential issues to etcd quorum.
	// Note: this condition is not set when using an external etcd.
	KubeadmControlPlaneEtcdClusterHealthyCondition = "EtcdClusterHealthy"

	// KubeadmControlPlaneEtcdClusterInspectionFailedReason documents a failure when inspecting the status of the
	// etcd cluster hosted on KubeadmControlPlane controlled machines.
	KubeadmControlPlaneEtcdClusterInspectionFailedReason = clusterv1.InspectionFailedReason

	// KubeadmControlPlaneEtcdClusterConnectionDownReason surfaces that the connection to the workload
	// cluster is down.
	KubeadmControlPlaneEtcdClusterConnectionDownReason = clusterv1.ConnectionDownReason

	// KubeadmControlPlaneEtcdClusterHealthyReason surfaces when the etcd cluster hosted on KubeadmControlPlane
	// machines is healthy.
	KubeadmControlPlaneEtcdClusterHealthyReason = "Healthy"

	// KubeadmControlPlaneEtcdClusterNotHealthyReason surfaces when the etcd cluster hosted on KubeadmControlPlane
	// machines is not healthy.
	KubeadmControlPlaneEtcdClusterNotHealthyReason = "NotHealthy"

	// KubeadmControlPlaneEtcdClusterHealthUnknownReason surfaces when the health status of the etcd cluster hosted
	// on KubeadmControlPlane machines is unknown.
	KubeadmControlPlaneEtcdClusterHealthUnknownReason = "HealthUnknown"
)

// KubeadmControlPlane's ControlPlaneComponentsHealthy condition and corresponding reasons.
const (
	// KubeadmControlPlaneControlPlaneComponentsHealthyCondition surfaces issues to Kubernetes control plane components
	// hosted on machines managed by this object. It is computed as aggregation of Machine's `APIServerPodHealthy`,
	// `ControllerManagerPodHealthy`, `SchedulerPodHealthy`, `EtcdPodHealthy` conditions plus additional checks on
	// control plane machines and nodes.
	KubeadmControlPlaneControlPlaneComponentsHealthyCondition = "ControlPlaneComponentsHealthy"

	// KubeadmControlPlaneControlPlaneComponentsInspectionFailedReason documents a failure when inspecting the status of the
	// control plane components hosted on KubeadmControlPlane controlled machines.
	KubeadmControlPlaneControlPlaneComponentsInspectionFailedReason = clusterv1.InspectionFailedReason

	// KubeadmControlPlaneControlPlaneComponentsConnectionDownReason surfaces that the connection to the workload
	// cluster is down.
	KubeadmControlPlaneControlPlaneComponentsConnectionDownReason = clusterv1.ConnectionDownReason

	// KubeadmControlPlaneControlPlaneComponentsHealthyReason surfaces when the Kubernetes control plane components
	// hosted on KubeadmControlPlane machines are healthy.
	KubeadmControlPlaneControlPlaneComponentsHealthyReason = "Healthy"

	// KubeadmControlPlaneControlPlaneComponentsNotHealthyReason surfaces when the Kubernetes control plane components
	// hosted on KubeadmControlPlane machines are not healthy.
	KubeadmControlPlaneControlPlaneComponentsNotHealthyReason = "NotHealthy"

	// KubeadmControlPlaneControlPlaneComponentsHealthUnknownReason surfaces when the health status of the
	// Kubernetes control plane components hosted on KubeadmControlPlane machines is unknown.
	KubeadmControlPlaneControlPlaneComponentsHealthUnknownReason = "HealthUnknown"
)

// KubeadmControlPlane's MachinesReady condition and corresponding reasons.
const (
	// KubeadmControlPlaneMachinesReadyCondition surfaces detail of issues on the controlled machines, if any.
	// Please note this will include also APIServerPodHealthy, ControllerManagerPodHealthy, SchedulerPodHealthy conditions.
	// If not using an external etcd also EtcdPodHealthy, EtcdMemberHealthy conditions are included.
	KubeadmControlPlaneMachinesReadyCondition = clusterv1.MachinesReadyCondition

	// KubeadmControlPlaneMachinesReadyReason surfaces when all the controlled machine's Ready conditions are true.
	KubeadmControlPlaneMachinesReadyReason = clusterv1.ReadyReason

	// KubeadmControlPlaneMachinesNotReadyReason surfaces when at least one of the controlled machine's Ready conditions is false.
	KubeadmControlPlaneMachinesNotReadyReason = clusterv1.NotReadyReason

	// KubeadmControlPlaneMachinesReadyUnknownReason surfaces when at least one of the controlled machine's Ready conditions is unknown
	// and no one of the controlled machine's Ready conditions is false.
	KubeadmControlPlaneMachinesReadyUnknownReason = clusterv1.ReadyUnknownReason

	// KubeadmControlPlaneMachinesReadyNoReplicasReason surfaces when no machines exist for the KubeadmControlPlane.
	KubeadmControlPlaneMachinesReadyNoReplicasReason = clusterv1.NoReplicasReason

	// KubeadmControlPlaneMachinesReadyInternalErrorReason surfaces unexpected failures when computing the MachinesReady condition.
	KubeadmControlPlaneMachinesReadyInternalErrorReason = clusterv1.InternalErrorReason
)

// KubeadmControlPlane's MachinesUpToDate condition and corresponding reasons.
const (
	// KubeadmControlPlaneMachinesUpToDateCondition surfaces details of controlled machines not up to date, if any.
	// Note: New machines are considered 10s after machine creation. This gives time to the machine's owner controller to recognize the new machine and add the UpToDate condition.
	KubeadmControlPlaneMachinesUpToDateCondition = clusterv1.MachinesUpToDateCondition

	// KubeadmControlPlaneMachinesUpToDateReason surfaces when all the controlled machine's UpToDate conditions are true.
	KubeadmControlPlaneMachinesUpToDateReason = clusterv1.UpToDateReason

	// KubeadmControlPlaneMachinesNotUpToDateReason surfaces when at least one of the controlled machine's UpToDate conditions is false.
	KubeadmControlPlaneMachinesNotUpToDateReason = clusterv1.NotUpToDateReason

	// KubeadmControlPlaneMachinesUpToDateUnknownReason surfaces when at least one of the controlled machine's UpToDate conditions is unknown
	// and no one of the controlled machine's UpToDate conditions is false.
	KubeadmControlPlaneMachinesUpToDateUnknownReason = clusterv1.UpToDateUnknownReason

	// KubeadmControlPlaneMachinesUpToDateNoReplicasReason surfaces when no machines exist for the KubeadmControlPlane.
	KubeadmControlPlaneMachinesUpToDateNoReplicasReason = clusterv1.NoReplicasReason

	// KubeadmControlPlaneMachinesUpToDateInternalErrorReason surfaces unexpected failures when computing the MachinesUpToDate condition.
	KubeadmControlPlaneMachinesUpToDateInternalErrorReason = clusterv1.InternalErrorReason
)

// KubeadmControlPlane's RollingOut condition and corresponding reasons.
const (
	// KubeadmControlPlaneRollingOutCondition  is true if there is at least one machine not up-to-date.
	KubeadmControlPlaneRollingOutCondition = clusterv1.RollingOutCondition

	// KubeadmControlPlaneRollingOutReason  surfaces when there is at least one machine not up-to-date.
	KubeadmControlPlaneRollingOutReason = clusterv1.RollingOutReason

	// KubeadmControlPlaneNotRollingOutReason surfaces when all the machines are up-to-date.
	KubeadmControlPlaneNotRollingOutReason = clusterv1.NotRollingOutReason
)

// KubeadmControlPlane's ScalingUp condition and corresponding reasons.
const (
	// KubeadmControlPlaneScalingUpCondition is true if actual replicas < desired replicas.
	// Note: In case a KubeadmControlPlane preflight check is preventing scale up, this will surface in the condition message.
	KubeadmControlPlaneScalingUpCondition = clusterv1.ScalingUpCondition

	// KubeadmControlPlaneScalingUpReason surfaces when actual replicas < desired replicas.
	KubeadmControlPlaneScalingUpReason = clusterv1.ScalingUpReason

	// KubeadmControlPlaneNotScalingUpReason surfaces when actual replicas >= desired replicas.
	KubeadmControlPlaneNotScalingUpReason = clusterv1.NotScalingUpReason

	// KubeadmControlPlaneScalingUpWaitingForReplicasSetReason surfaces when the .spec.replicas
	// field of the KubeadmControlPlane is not set.
	KubeadmControlPlaneScalingUpWaitingForReplicasSetReason = clusterv1.WaitingForReplicasSetReason
)

// KubeadmControlPlane's ScalingDown condition and corresponding reasons.
const (
	// KubeadmControlPlaneScalingDownCondition is true if actual replicas > desired replicas.
	// Note: In case a KubeadmControlPlane preflight check is preventing scale down, this will surface in the condition message.
	KubeadmControlPlaneScalingDownCondition = clusterv1.ScalingDownCondition

	// KubeadmControlPlaneScalingDownReason surfaces when actual replicas > desired replicas.
	KubeadmControlPlaneScalingDownReason = clusterv1.ScalingDownReason

	// KubeadmControlPlaneNotScalingDownReason surfaces when actual replicas <= desired replicas.
	KubeadmControlPlaneNotScalingDownReason = clusterv1.NotScalingDownReason

	// KubeadmControlPlaneScalingDownWaitingForReplicasSetReason surfaces when the .spec.replicas
	// field of the KubeadmControlPlane is not set.
	KubeadmControlPlaneScalingDownWaitingForReplicasSetReason = clusterv1.WaitingForReplicasSetReason
)

// KubeadmControlPlane's Remediating condition and corresponding reasons.
const (
	// KubeadmControlPlaneRemediatingCondition surfaces details about ongoing remediation of the controlled machines, if any.
	// Note: KubeadmControlPlane only remediates machines with HealthCheckSucceeded set to false and with the OwnerRemediated condition set to false.
	KubeadmControlPlaneRemediatingCondition = clusterv1.RemediatingCondition

	// KubeadmControlPlaneRemediatingReason surfaces when the KubeadmControlPlane has at least one machine with HealthCheckSucceeded set to false
	// and with the OwnerRemediated condition set to false.
	KubeadmControlPlaneRemediatingReason = clusterv1.RemediatingReason

	// KubeadmControlPlaneNotRemediatingReason surfaces when the KubeadmControlPlane does not have any machine with HealthCheckSucceeded set to false
	// and with the OwnerRemediated condition set to false.
	KubeadmControlPlaneNotRemediatingReason = clusterv1.NotRemediatingReason

	// KubeadmControlPlaneRemediatingInternalErrorReason surfaces unexpected failures when computing the Remediating condition.
	KubeadmControlPlaneRemediatingInternalErrorReason = clusterv1.InternalErrorReason
)

// Reasons that will be used for the OwnerRemediated condition set by MachineHealthCheck on KubeadmControlPlane controlled machines
// being remediated in v1Beta2 API version.
const (
	// KubeadmControlPlaneMachineRemediationInternalErrorReason surfaces unexpected failures while remediating a control plane machine.
	KubeadmControlPlaneMachineRemediationInternalErrorReason = clusterv1.InternalErrorReason

	// KubeadmControlPlaneMachineCannotBeRemediatedReason surfaces when remediation of a control plane machine can't be started.
	KubeadmControlPlaneMachineCannotBeRemediatedReason = "CannotBeRemediated"

	// KubeadmControlPlaneMachineRemediationDeferredReason surfaces when remediation of a control plane machine must be deferred.
	KubeadmControlPlaneMachineRemediationDeferredReason = "RemediationDeferred"

	// KubeadmControlPlaneMachineRemediationMachineDeletingReason surfaces when remediation of a control plane machine
	// has been completed by deleting the unhealthy machine.
	// Note: After an unhealthy machine is deleted, a new one is created by the KubeadmControlPlaneMachine as part of the
	// regular reconcile loop that ensures the correct number of replicas exist; KubeadmControlPlane machine waits for
	// the new machine to exists before removing the controlplane.cluster.x-k8s.io/remediation-in-progress annotation.
	// This is part of a series of safeguards to ensure that operation are performed sequentially on control plane machines.
	KubeadmControlPlaneMachineRemediationMachineDeletingReason = "MachineDeleting"
)

// KubeadmControlPlane's Deleting condition and corresponding reasons.
const (
	// KubeadmControlPlaneDeletingCondition surfaces details about ongoing deletion of the controlled machines.
	KubeadmControlPlaneDeletingCondition = clusterv1.DeletingCondition

	// KubeadmControlPlaneNotDeletingReason surfaces when the KCP is not deleting because the
	// DeletionTimestamp is not set.
	KubeadmControlPlaneNotDeletingReason = clusterv1.NotDeletingReason

	// KubeadmControlPlaneDeletingWaitingForWorkersDeletionReason surfaces when the KCP deletion
	// waits for the workers to be deleted.
	KubeadmControlPlaneDeletingWaitingForWorkersDeletionReason = "WaitingForWorkersDeletion"

	// KubeadmControlPlaneDeletingWaitingForMachineDeletionReason surfaces when the KCP deletion
	// waits for the control plane Machines to be deleted.
	KubeadmControlPlaneDeletingWaitingForMachineDeletionReason = "WaitingForMachineDeletion"

	// KubeadmControlPlaneDeletingDeletionCompletedReason surfaces when the KCP deletion has been completed.
	// This reason is set right after the `kubeadm.controlplane.cluster.x-k8s.io` finalizer is removed.
	// This means that the object will go away (i.e. be removed from etcd), except if there are other
	// finalizers on the KCP object.
	KubeadmControlPlaneDeletingDeletionCompletedReason = clusterv1.DeletionCompletedReason

	// KubeadmControlPlaneDeletingInternalErrorReason surfaces unexpected failures when deleting a KCP object.
	KubeadmControlPlaneDeletingInternalErrorReason = clusterv1.InternalErrorReason
)

// APIServerPodHealthy, ControllerManagerPodHealthy, SchedulerPodHealthy and EtcdPodHealthy condition and corresponding
// reasons that will be used for KubeadmControlPlane controlled machines in v1Beta2 API version.
const (
	// KubeadmControlPlaneMachineAPIServerPodHealthyCondition surfaces the status of the API server pod hosted on a KubeadmControlPlane controlled machine.
	KubeadmControlPlaneMachineAPIServerPodHealthyCondition = "APIServerPodHealthy"

	// KubeadmControlPlaneMachineControllerManagerPodHealthyCondition surfaces the status of the controller manager pod hosted on a KubeadmControlPlane controlled machine.
	KubeadmControlPlaneMachineControllerManagerPodHealthyCondition = "ControllerManagerPodHealthy"

	// KubeadmControlPlaneMachineSchedulerPodHealthyCondition surfaces the status of the scheduler pod hosted on a KubeadmControlPlane controlled machine.
	KubeadmControlPlaneMachineSchedulerPodHealthyCondition = "SchedulerPodHealthy"

	// KubeadmControlPlaneMachineEtcdPodHealthyCondition surfaces the status of the etcd pod hosted on a KubeadmControlPlane controlled machine.
	KubeadmControlPlaneMachineEtcdPodHealthyCondition = "EtcdPodHealthy"

	// KubeadmControlPlaneMachinePodRunningReason surfaces a pod hosted on a KubeadmControlPlane controlled machine that is running.
	KubeadmControlPlaneMachinePodRunningReason = "Running"

	// KubeadmControlPlaneMachinePodProvisioningReason surfaces a pod hosted on a KubeadmControlPlane controlled machine
	// waiting to be provisioned i.e., Pod is in "Pending" phase.
	KubeadmControlPlaneMachinePodProvisioningReason = "Provisioning"

	// KubeadmControlPlaneMachinePodDoesNotExistReason surfaces a when a pod hosted on a KubeadmControlPlane controlled machine
	// does not exist.
	KubeadmControlPlaneMachinePodDoesNotExistReason = "DoesNotExist"

	// KubeadmControlPlaneMachinePodFailedReason surfaces a when a pod hosted on a KubeadmControlPlane controlled machine
	// failed during provisioning, e.g. CrashLoopBackOff, ImagePullBackOff or if all the containers in a pod have terminated.
	KubeadmControlPlaneMachinePodFailedReason = "Failed"

	// KubeadmControlPlaneMachinePodInspectionFailedReason documents a failure when inspecting the status of a
	// pod hosted on a KubeadmControlPlane controlled machine.
	KubeadmControlPlaneMachinePodInspectionFailedReason = clusterv1.InspectionFailedReason

	// KubeadmControlPlaneMachinePodConnectionDownReason surfaces that the connection to the workload
	// cluster is down.
	KubeadmControlPlaneMachinePodConnectionDownReason = clusterv1.ConnectionDownReason

	// KubeadmControlPlaneMachinePodDeletingReason surfaces when the machine hosting control plane components
	// is being deleted.
	KubeadmControlPlaneMachinePodDeletingReason = "Deleting"
)

// EtcdMemberHealthy condition and corresponding reasons that will be used for KubeadmControlPlane controlled machines in v1Beta2 API version.
const (
	// KubeadmControlPlaneMachineEtcdMemberHealthyCondition surfaces the status of the etcd member hosted on a KubeadmControlPlane controlled machine.
	KubeadmControlPlaneMachineEtcdMemberHealthyCondition = "EtcdMemberHealthy"

	// KubeadmControlPlaneMachineEtcdMemberNotHealthyReason surfaces when the etcd member hosted on a KubeadmControlPlane controlled machine is not healthy.
	KubeadmControlPlaneMachineEtcdMemberNotHealthyReason = "NotHealthy"

	// KubeadmControlPlaneMachineEtcdMemberHealthyReason surfaces when the etcd member hosted on a KubeadmControlPlane controlled machine is healthy.
	KubeadmControlPlaneMachineEtcdMemberHealthyReason = "Healthy"

	// KubeadmControlPlaneMachineEtcdMemberInspectionFailedReason documents a failure when inspecting the status of an
	// etcd member hosted on a KubeadmControlPlane controlled machine.
	KubeadmControlPlaneMachineEtcdMemberInspectionFailedReason = clusterv1.InspectionFailedReason

	// KubeadmControlPlaneMachineEtcdMemberConnectionDownReason surfaces that the connection to the workload
	// cluster is down.
	KubeadmControlPlaneMachineEtcdMemberConnectionDownReason = clusterv1.ConnectionDownReason

	// KubeadmControlPlaneMachineEtcdMemberDeletingReason surfaces when the machine hosting an etcd member
	// is being deleted.
	KubeadmControlPlaneMachineEtcdMemberDeletingReason = "Deleting"
)

// KubeadmControlPlaneSpec defines the desired state of KubeadmControlPlane.
type KubeadmControlPlaneSpec struct {
	// replicas is the number of desired machines. Defaults to 1. When stacked etcd is used only
	// odd numbers are permitted, as per [etcd best practice](https://etcd.io/docs/v3.3.12/faq/#why-an-odd-number-of-cluster-members).
	// This is a pointer to distinguish between explicit zero and not specified.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// version defines the desired Kubernetes version.
	// Please note that if kubeadmConfigSpec.ClusterConfiguration.imageRepository is not set
	// we don't allow upgrades to versions >= v1.22.0 for which kubeadm uses the old registry (k8s.gcr.io).
	// Please use a newer patch version with the new registry instead. The default registries of kubeadm are:
	//   * registry.k8s.io (new registry): >= v1.22.17, >= v1.23.15, >= v1.24.9, >= v1.25.0
	//   * k8s.gcr.io (old registry): all older versions
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Version string `json:"version,omitempty"`

	// machineTemplate contains information about how machines
	// should be shaped when creating or updating a control plane.
	// +required
	MachineTemplate KubeadmControlPlaneMachineTemplate `json:"machineTemplate,omitempty,omitzero"`

	// kubeadmConfigSpec is a KubeadmConfigSpec
	// to use for initializing and joining machines to the control plane.
	// +optional
	KubeadmConfigSpec bootstrapv1.KubeadmConfigSpec `json:"kubeadmConfigSpec,omitempty,omitzero"`

	// rollout allows you to configure the behaviour of rolling updates to the control plane Machines.
	// It allows you to require that all Machines are replaced before or after a certain time,
	// and allows you to define the strategy used during rolling replacements.
	// +optional
	Rollout KubeadmControlPlaneRolloutSpec `json:"rollout,omitempty,omitzero"`

	// remediation controls how unhealthy Machines are remediated.
	// +optional
	Remediation KubeadmControlPlaneRemediationSpec `json:"remediation,omitempty,omitzero"`

	// machineNaming allows changing the naming pattern used when creating Machines.
	// InfraMachines & KubeadmConfigs will use the same name as the corresponding Machines.
	// +optional
	MachineNaming MachineNamingSpec `json:"machineNaming,omitempty,omitzero"`
}

// KubeadmControlPlaneMachineTemplate defines the template for Machines
// in a KubeadmControlPlane object.
type KubeadmControlPlaneMachineTemplate struct {
	// metadata is the standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	ObjectMeta clusterv1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the spec for Machines
	// in a KubeadmControlPlane object.
	// +required
	Spec KubeadmControlPlaneMachineTemplateSpec `json:"spec,omitempty,omitzero"`
}

// KubeadmControlPlaneMachineTemplateSpec defines the spec for Machines
// in a KubeadmControlPlane object.
type KubeadmControlPlaneMachineTemplateSpec struct {
	// infrastructureRef is a required reference to a custom resource
	// offered by an infrastructure provider.
	// +required
	InfrastructureRef clusterv1.ContractVersionedObjectReference `json:"infrastructureRef,omitempty,omitzero"`

	// readinessGates specifies additional conditions to include when evaluating Machine Ready condition;
	// KubeadmControlPlane will always add readinessGates for the condition it is setting on the Machine:
	// APIServerPodHealthy, SchedulerPodHealthy, ControllerManagerPodHealthy, and if etcd is managed by CKP also
	// EtcdPodHealthy, EtcdMemberHealthy.
	//
	// This field can be used e.g. to instruct the machine controller to include in the computation for Machine's ready
	// computation a condition, managed by an external controllers, reporting the status of special software/hardware installed on the Machine.
	//
	// +optional
	// +listType=map
	// +listMapKey=conditionType
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=32
	ReadinessGates []clusterv1.MachineReadinessGate `json:"readinessGates,omitempty"`

	// deletion contains configuration options for Machine deletion.
	// +optional
	Deletion KubeadmControlPlaneMachineTemplateDeletionSpec `json:"deletion,omitempty,omitzero"`
}

// KubeadmControlPlaneMachineTemplateDeletionSpec contains configuration options for Machine deletion.
// +kubebuilder:validation:MinProperties=1
type KubeadmControlPlaneMachineTemplateDeletionSpec struct {
	// nodeDrainTimeoutSeconds is the total amount of time that the controller will spend on draining a controlplane node
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

	// nodeDeletionTimeoutSeconds defines how long the machine controller will attempt to delete the Node that the Machine
	// hosts after the Machine is marked for deletion. A duration of 0 will retry deletion indefinitely.
	// If no value is provided, the default value for this property of the Machine resource will be used.
	// +optional
	// +kubebuilder:validation:Minimum=0
	NodeDeletionTimeoutSeconds *int32 `json:"nodeDeletionTimeoutSeconds,omitempty"`
}

// KubeadmControlPlaneRolloutSpec allows you to configure the behaviour of rolling updates to the control plane Machines.
// It allows you to require that all Machines are replaced before or after a certain time,
// and allows you to define the strategy used during rolling replacements.
// +kubebuilder:validation:MinProperties=1
type KubeadmControlPlaneRolloutSpec struct {
	// before is a field to indicate a rollout should be performed
	// if the specified criteria is met.
	// +optional
	Before KubeadmControlPlaneRolloutBeforeSpec `json:"before,omitempty,omitzero"`

	// after is a field to indicate a rollout should be performed
	// after the specified time even if no changes have been made to the
	// KubeadmControlPlane.
	// Example: In the YAML the time can be specified in the RFC3339 format.
	// To specify the rolloutAfter target as March 9, 2023, at 9 am UTC
	// use "2023-03-09T09:00:00Z".
	// +optional
	After metav1.Time `json:"after,omitempty,omitzero"`

	// strategy specifies how to roll out control plane Machines.
	// +optional
	Strategy KubeadmControlPlaneRolloutStrategy `json:"strategy,omitempty,omitzero"`
}

// KubeadmControlPlaneRolloutBeforeSpec describes when a rollout should be performed on the KCP machines.
// +kubebuilder:validation:MinProperties=1
type KubeadmControlPlaneRolloutBeforeSpec struct {
	// certificatesExpiryDays indicates a rollout needs to be performed if the
	// certificates of the machine will expire within the specified days.
	// The minimum for this field is 7.
	// +optional
	// +kubebuilder:validation:Minimum=7
	CertificatesExpiryDays int32 `json:"certificatesExpiryDays,omitempty"`
}

// KubeadmControlPlaneRolloutStrategy describes how to replace existing machines
// with new ones.
// +kubebuilder:validation:MinProperties=1
type KubeadmControlPlaneRolloutStrategy struct {
	// type of rollout. Currently the only supported strategy is
	// "RollingUpdate".
	// Default is RollingUpdate.
	// +required
	Type KubeadmControlPlaneRolloutStrategyType `json:"type,omitempty"`

	// rollingUpdate is the rolling update config params. Present only if
	// type = RollingUpdate.
	// +optional
	RollingUpdate KubeadmControlPlaneRolloutStrategyRollingUpdate `json:"rollingUpdate,omitempty,omitzero"`
}

// KubeadmControlPlaneRolloutStrategyRollingUpdate is used to control the desired behavior of rolling update.
// +kubebuilder:validation:MinProperties=1
type KubeadmControlPlaneRolloutStrategyRollingUpdate struct {
	// maxSurge is the maximum number of control planes that can be scheduled above or under the
	// desired number of control planes.
	// Value can be an absolute number 1 or 0.
	// Defaults to 1.
	// Example: when this is set to 1, the control plane can be scaled
	// up immediately when the rolling update starts.
	// +optional
	MaxSurge *intstr.IntOrString `json:"maxSurge,omitempty"`
}

// KubeadmControlPlaneRemediationSpec controls how unhealthy control plane Machines are remediated.
// +kubebuilder:validation:MinProperties=1
type KubeadmControlPlaneRemediationSpec struct {
	// maxRetry is the Max number of retries while attempting to remediate an unhealthy machine.
	// A retry happens when a machine that was created as a replacement for an unhealthy machine also fails.
	// For example, given a control plane with three machines M1, M2, M3:
	//
	//	M1 become unhealthy; remediation happens, and M1-1 is created as a replacement.
	//	If M1-1 (replacement of M1) has problems while bootstrapping it will become unhealthy, and then be
	//	remediated; such operation is considered a retry, remediation-retry #1.
	//	If M1-2 (replacement of M1-1) becomes unhealthy, remediation-retry #2 will happen, etc.
	//
	// A retry could happen only after retryPeriodSeconds from the previous retry.
	// If a machine is marked as unhealthy after minHealthyPeriodSeconds from the previous remediation expired,
	// this is not considered a retry anymore because the new issue is assumed unrelated from the previous one.
	//
	// If not set, the remedation will be retried infinitely.
	// +optional
	MaxRetry *int32 `json:"maxRetry,omitempty"`

	// retryPeriodSeconds is the duration that KCP should wait before remediating a machine being created as a replacement
	// for an unhealthy machine (a retry).
	//
	// If not set, a retry will happen immediately.
	// +optional
	// +kubebuilder:validation:Minimum=0
	RetryPeriodSeconds *int32 `json:"retryPeriodSeconds,omitempty"`

	// minHealthyPeriodSeconds defines the duration after which KCP will consider any failure to a machine unrelated
	// from the previous one. In this case the remediation is not considered a retry anymore, and thus the retry
	// counter restarts from 0. For example, assuming minHealthyPeriodSeconds is set to 1h (default)
	//
	//	M1 become unhealthy; remediation happens, and M1-1 is created as a replacement.
	//	If M1-1 (replacement of M1) has problems within the 1hr after the creation, also
	//	this machine will be remediated and this operation is considered a retry - a problem related
	//	to the original issue happened to M1 -.
	//
	//	If instead the problem on M1-1 is happening after minHealthyPeriodSeconds expired, e.g. four days after
	//	m1-1 has been created as a remediation of M1, the problem on M1-1 is considered unrelated to
	//	the original issue happened to M1.
	//
	// If not set, this value is defaulted to 1h.
	// +optional
	// +kubebuilder:validation:Minimum=0
	MinHealthyPeriodSeconds *int32 `json:"minHealthyPeriodSeconds,omitempty"`
}

// MachineNamingSpec allows changing the naming pattern used when creating Machines.
// InfraMachines & KubeadmConfigs will use the same name as the corresponding Machines.
// +kubebuilder:validation:MinProperties=1
type MachineNamingSpec struct {
	// template defines the template to use for generating the names of the Machine objects.
	// If not defined, it will fallback to `{{ .kubeadmControlPlane.name }}-{{ .random }}`.
	// If the generated name string exceeds 63 characters, it will be trimmed to 58 characters and will
	// get concatenated with a random suffix of length 5.
	// Length of the template string must not exceed 256 characters.
	// The template allows the following variables `.cluster.name`, `.kubeadmControlPlane.name` and `.random`.
	// The variable `.cluster.name` retrieves the name of the cluster object that owns the Machines being created.
	// The variable `.kubeadmControlPlane.name` retrieves the name of the KubeadmControlPlane object that owns the Machines being created.
	// The variable `.random` is substituted with random alphanumeric string, without vowels, of length 5. This variable is required
	// part of the template. If not provided, validation will fail.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Template string `json:"template,omitempty"`
}

// KubeadmControlPlaneStatus defines the observed state of KubeadmControlPlane.
// +kubebuilder:validation:MinProperties=1
type KubeadmControlPlaneStatus struct {
	// conditions represents the observations of a KubeadmControlPlane's current state.
	// Known condition types are Available, CertificatesAvailable, EtcdClusterAvailable, MachinesReady, MachinesUpToDate,
	// ScalingUp, ScalingDown, Remediating, Deleting, Paused.
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=32
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// initialization provides observations of the KubeadmControlPlane initialization process.
	// NOTE: Fields in this struct are part of the Cluster API contract and are used to orchestrate initial Machine provisioning.
	// +optional
	Initialization KubeadmControlPlaneInitializationStatus `json:"initialization,omitempty,omitzero"`

	// selector is the label selector in string format to avoid introspection
	// by clients, and is used to provide the CRD-based integration for the
	// scale subresource and additional integrations for things like kubectl
	// describe.. The string will be in the same format as the query-param syntax.
	// More info about label selectors: http://kubernetes.io/docs/user-guide/labels#label-selectors
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=4096
	Selector string `json:"selector,omitempty"`

	// replicas is the total number of non-terminated machines targeted by this control plane
	// (their labels match the selector).
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// readyReplicas is the number of ready replicas for this KubeadmControlPlane. A machine is considered ready when Machine's Ready condition is true.
	// +optional
	ReadyReplicas *int32 `json:"readyReplicas,omitempty"`

	// availableReplicas is the number of available replicas targeted by this KubeadmControlPlane. A machine is considered available when Machine's Available condition is true.
	// +optional
	AvailableReplicas *int32 `json:"availableReplicas,omitempty"`

	// upToDateReplicas is the number of up-to-date replicas targeted by this KubeadmControlPlane. A machine is considered up-to-date when Machine's UpToDate condition is true.
	// +optional
	UpToDateReplicas *int32 `json:"upToDateReplicas,omitempty"`

	// version represents the minimum Kubernetes version for the control plane machines
	// in the cluster.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Version string `json:"version,omitempty"`

	// observedGeneration is the latest generation observed by the controller.
	// +optional
	// +kubebuilder:validation:Minimum=1
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// lastRemediation stores info about last remediation performed.
	// +optional
	LastRemediation LastRemediationStatus `json:"lastRemediation,omitempty,omitzero"`

	// deprecated groups all the status fields that are deprecated and will be removed when all the nested field are removed.
	// +optional
	Deprecated *KubeadmControlPlaneDeprecatedStatus `json:"deprecated,omitempty"`
}

// KubeadmControlPlaneInitializationStatus provides observations of the KubeadmControlPlane initialization process.
// +kubebuilder:validation:MinProperties=1
type KubeadmControlPlaneInitializationStatus struct {
	// controlPlaneInitialized is true when the KubeadmControlPlane provider reports that the Kubernetes control plane is initialized;
	// A control plane is considered initialized when it can accept requests, no matter if this happens before
	// the control plane is fully provisioned or not.
	// NOTE: this field is part of the Cluster API contract, and it is used to orchestrate initial Machine provisioning.
	// +optional
	ControlPlaneInitialized *bool `json:"controlPlaneInitialized,omitempty"`
}

// KubeadmControlPlaneDeprecatedStatus groups all the status fields that are deprecated and will be removed in a future version.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type KubeadmControlPlaneDeprecatedStatus struct {
	// v1beta1 groups all the status fields that are deprecated and will be removed when support for v1beta1 will be dropped.
	// +optional
	V1Beta1 *KubeadmControlPlaneV1Beta1DeprecatedStatus `json:"v1beta1,omitempty"`
}

// KubeadmControlPlaneV1Beta1DeprecatedStatus groups all the status fields that are deprecated and will be removed when support for v1beta1 will be dropped.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type KubeadmControlPlaneV1Beta1DeprecatedStatus struct {
	// conditions defines current service state of the KubeadmControlPlane.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`

	// failureReason indicates that there is a terminal problem reconciling the
	// state, and will be set to a token value suitable for
	// programmatic interpretation.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	FailureReason errors.KubeadmControlPlaneStatusError `json:"failureReason,omitempty"`

	// failureMessage indicates that there is a terminal problem reconciling the
	// state, and will be set to a descriptive error message.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=10240
	FailureMessage *string `json:"failureMessage,omitempty"` //nolint:kubeapilinter // field will be removed when v1beta1 is removed

	// updatedReplicas is the total number of non-terminated machines targeted by this control plane
	// that have the desired template spec.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	UpdatedReplicas int32 `json:"updatedReplicas"` //nolint:kubeapilinter // field will be removed when v1beta1 is removed

	// readyReplicas is the total number of fully running and ready control plane machines.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	ReadyReplicas int32 `json:"readyReplicas"` //nolint:kubeapilinter // field will be removed when v1beta1 is removed

	// unavailableReplicas is the total number of unavailable machines targeted by this control plane.
	// This is the total number of machines that are still required for
	// the deployment to have 100% available capacity. They may either
	// be machines that are running but not yet ready or machines
	// that still have not been created.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	UnavailableReplicas int32 `json:"unavailableReplicas"` //nolint:kubeapilinter // field will be removed when v1beta1 is removed
}

// LastRemediationStatus  stores info about last remediation performed.
// NOTE: if for any reason information about last remediation are lost, RetryCount is going to restart from 0 and thus
// more remediations than expected might happen.
type LastRemediationStatus struct {
	// machine is the machine name of the latest machine being remediated.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Machine string `json:"machine,omitempty"`

	// time is when last remediation happened. It is represented in RFC3339 form and is in UTC.
	// +required
	Time metav1.Time `json:"time,omitempty,omitzero"`

	// retryCount used to keep track of remediation retry for the last remediated machine.
	// A retry happens when a machine that was created as a replacement for an unhealthy machine also fails.
	// +required
	// +kubebuilder:validation:Minimum=0
	RetryCount *int32 `json:"retryCount,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=kubeadmcontrolplanes,shortName=kcp,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas,selectorpath=.status.selector
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".metadata.labels['cluster\\.x-k8s\\.io/cluster-name']",description="Cluster"
// +kubebuilder:printcolumn:name="Available",type="string",JSONPath=`.status.conditions[?(@.type=="Available")].status`,description="Cluster pass all availability checks"
// +kubebuilder:printcolumn:name="Desired",type=integer,JSONPath=".spec.replicas",description="The desired number of machines"
// +kubebuilder:printcolumn:name="Current",type="integer",JSONPath=".status.replicas",description="The number of machines"
// +kubebuilder:printcolumn:name="Ready",type="integer",JSONPath=".status.readyReplicas",description="The number of machines with Ready condition true"
// +kubebuilder:printcolumn:name="Available",type=integer,JSONPath=".status.availableReplicas",description="The number of machines with Available condition true"
// +kubebuilder:printcolumn:name="Up-to-date",type=integer,JSONPath=".status.upToDateReplicas",description="The number of machines with UpToDate condition true"
// +kubebuilder:printcolumn:name="Paused",type="string",JSONPath=`.status.conditions[?(@.type=="Paused")].status`,description="Reconciliation paused",priority=10
// +kubebuilder:printcolumn:name="Initialized",type=boolean,JSONPath=".status.initialization.controlPlaneInitialized",description="This denotes whether or not the control plane can accept requests"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of KubeadmControlPlane"
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=".spec.version",description="Kubernetes version associated with this control plane"

// KubeadmControlPlane is the Schema for the KubeadmControlPlane API.
type KubeadmControlPlane struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec is the desired state of KubeadmControlPlane.
	// +required
	Spec KubeadmControlPlaneSpec `json:"spec,omitempty,omitzero"`
	// status is the observed state of KubeadmControlPlane.
	// +optional
	Status KubeadmControlPlaneStatus `json:"status,omitempty,omitzero"`
}

// GetV1Beta1Conditions returns the set of conditions for this object.
func (in *KubeadmControlPlane) GetV1Beta1Conditions() clusterv1.Conditions {
	if in.Status.Deprecated == nil || in.Status.Deprecated.V1Beta1 == nil {
		return nil
	}
	return in.Status.Deprecated.V1Beta1.Conditions
}

// SetV1Beta1Conditions sets the conditions on this object.
func (in *KubeadmControlPlane) SetV1Beta1Conditions(conditions clusterv1.Conditions) {
	if in.Status.Deprecated == nil {
		in.Status.Deprecated = &KubeadmControlPlaneDeprecatedStatus{}
	}
	if in.Status.Deprecated.V1Beta1 == nil {
		in.Status.Deprecated.V1Beta1 = &KubeadmControlPlaneV1Beta1DeprecatedStatus{}
	}
	in.Status.Deprecated.V1Beta1.Conditions = conditions
}

// GetConditions returns the set of conditions for this object.
func (in *KubeadmControlPlane) GetConditions() []metav1.Condition {
	return in.Status.Conditions
}

// SetConditions sets conditions for an API object.
func (in *KubeadmControlPlane) SetConditions(conditions []metav1.Condition) {
	in.Status.Conditions = conditions
}

// +kubebuilder:object:root=true

// KubeadmControlPlaneList contains a list of KubeadmControlPlane.
type KubeadmControlPlaneList struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard list's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#lists-and-simple-kinds
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`
	// items is the list of KubeadmControlPlanes.
	Items []KubeadmControlPlane `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &KubeadmControlPlane{}, &KubeadmControlPlaneList{})
}
