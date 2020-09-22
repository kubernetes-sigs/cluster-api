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
