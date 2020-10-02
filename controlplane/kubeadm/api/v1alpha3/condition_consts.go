/*
Copyright 2020 The Kubernetes Authors.

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

package v1alpha3

import clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"

// Conditions and condition Reasons for the KubeadmControlPlane object

const (
	// MachinesReady reports an aggregate of current status of the machines controlled by the KubeadmControlPlane.
	MachinesReadyCondition clusterv1.ConditionType = "MachinesReady"
)

const (
	// CertificatesAvailableCondition documents that cluster certificates were generated as part of the
	// processing of a a KubeadmControlPlane object.
	CertificatesAvailableCondition clusterv1.ConditionType = "CertificatesAvailable"

	// CertificatesGenerationFailedReason (Severity=Warning) documents a KubeadmControlPlane controller detecting
	// an error while generating certificates; those kind of errors are usually temporary and the controller
	// automatically recover from them.
	CertificatesGenerationFailedReason = "CertificatesGenerationFailed"
)

const (
	// AvailableCondition documents that the first control plane instance has completed the kubeadm init operation
	// and so the control plane is available and an API server instance is ready for processing requests.
	AvailableCondition clusterv1.ConditionType = "Available"

	// WaitingForKubeadmInitReason (Severity=Info) documents a KubeadmControlPlane object waiting for the first
	// control plane instance to complete the kubeadm init operation.
	WaitingForKubeadmInitReason = "WaitingForKubeadmInit"
)

const (
	// MachinesSpecUpToDateCondition documents that the spec of the machines controlled by the KubeadmControlPlane
	// is up to date. Whe this condition is false, the KubeadmControlPlane is executing a rolling upgrade.
	MachinesSpecUpToDateCondition clusterv1.ConditionType = "MachinesSpecUpToDate"

	// RollingUpdateInProgressReason (Severity=Warning) documents a KubeadmControlPlane object executing a
	// rolling upgrade for aligning the machines spec to the desired state.
	RollingUpdateInProgressReason = "RollingUpdateInProgress"
)

const (
	// ResizedCondition documents a KubeadmControlPlane that is resizing the set of controlled machines.
	ResizedCondition clusterv1.ConditionType = "Resized"

	// ScalingUpReason (Severity=Info) documents a KubeadmControlPlane that is increasing the number of replicas.
	ScalingUpReason = "ScalingUp"

	// ScalingDownReason (Severity=Info) documents a KubeadmControlPlane that is decreasing the number of replicas.
	ScalingDownReason = "ScalingDown"
)

const (
	// EtcdClusterHealthy documents the overall etcd cluster's health for the KCP-managed etcd.
	EtcdClusterHealthy clusterv1.ConditionType = "EtcdClusterHealthy"

	// EtcdClusterUnhealthyReason (Severity=Warning) is set when the etcd cluster as unhealthy due to
	// i) if etcd cluster has lost its quorum.
	// ii) if etcd cluster has alarms armed.
	// iii) if etcd pods do not match with etcd members.
	EtcdClusterUnhealthyReason = "EtcdClusterUnhealthy"
)

// Common Pod-related Condition Reasons used by Pod-related Conditions such as MachineAPIServerPodHealthyCondition etc.
const (
	// PodProvisioningReason (Severity=Info) documents a pod waiting  to be provisioned i.e., Pod is in "Pending" phase and
	// PodScheduled and Initialized conditions are not yet set to True.
	PodProvisioningReason = "PodProvisioning"

	// PodMissingReason (Severity=Warning) documents a pod does not exist.
	PodMissingReason = "PodMissing"

	// PodFailedReason (Severity=Error) documents if
	// i) a pod failed during provisioning i.e., Pod is in "Pending" phase and
	// PodScheduled and Initialized conditions are set to True but ContainersReady or Ready condition is false
	// (i.e., at least one of the containers are in waiting state(e.g CrashLoopbackOff, ImagePullBackOff)
	// ii) a pod has at least one container that is terminated with a failure and hence Pod is in "Failed" phase.
	PodFailedReason = "PodFailed"
)

// Conditions that are only for control-plane machines. KubeadmControlPlane is the owner of these conditions.

const (
	// MachineAPIServerPodHealthyCondition reports a machine's kube-apiserver's health status.
	// Set to true if kube-apiserver pod is in "Running" phase, otherwise uses Pod-related Condition Reasons.
	MachineAPIServerPodHealthyCondition clusterv1.ConditionType = "APIServerPodHealthy"

	// MachineControllerManagerHealthyCondition reports a machine's kube-controller-manager's health status.
	// Set to true if kube-controller-manager pod is in "Running" phase, otherwise uses Pod-related Condition Reasons.
	MachineControllerManagerHealthyCondition clusterv1.ConditionType = "ControllerManagerPodHealthy"

	// MachineSchedulerPodHealthyCondition reports a machine's kube-scheduler's health status.
	// Set to true if kube-scheduler pod is in "Running" phase, otherwise uses Pod-related Condition Reasons.
	MachineSchedulerPodHealthyCondition clusterv1.ConditionType = "SchedulerPodHealthy"

	// MachineEtcdPodHealthyCondition reports a machine's etcd pod's health status.
	// Set to true if etcd pod is in "Running" phase, otherwise uses Pod-related Condition Reasons.
	MachineEtcdPodHealthyCondition clusterv1.ConditionType = "EtcdPodHealthy"
)

const (
	// MachineEtcdMemberHealthyCondition documents if the machine has an healthy etcd member.
	// If not true, Pod-related Condition Reasons can be used as reasons.
	MachineEtcdMemberHealthyCondition clusterv1.ConditionType = "EtcdMemberHealthy"

	// EtcdMemberUnhealthyReason (Severity=Error) documents a Machine's etcd member is unhealthy for a number of reasons:
	// i) etcd member has alarms.
	// ii) creating etcd client fails or using the created etcd client to perform some operations fails.
	// iii) Quorum is lost
	EtcdMemberUnhealthyReason = "EtcdMemberUnhealthy"
)
