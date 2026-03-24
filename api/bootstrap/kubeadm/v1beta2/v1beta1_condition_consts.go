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

import clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"

// Conditions and condition Reasons for the KubeadmConfig object.

const (
	// DataSecretAvailableV1Beta1Condition documents the status of the bootstrap secret generation process.
	//
	// NOTE: When the DataSecret generation starts the process completes immediately and within the
	// same reconciliation, so the user will always see a transition from Wait to Generated without having
	// evidence that BootstrapSecret generation is started/in progress.
	DataSecretAvailableV1Beta1Condition clusterv1.ConditionType = "DataSecretAvailable"

	// WaitingForClusterInfrastructureV1Beta1Reason (Severity=Info) document a bootstrap secret generation process
	// waiting for the cluster infrastructure to be ready.
	//
	// NOTE: Having the cluster infrastructure ready is a pre-condition for starting to create machines;
	// the KubeadmConfig controller ensure this pre-condition is satisfied.
	WaitingForClusterInfrastructureV1Beta1Reason = "WaitingForClusterInfrastructure"

	// DataSecretGenerationFailedV1Beta1Reason (Severity=Warning) documents a KubeadmConfig controller detecting
	// an error while generating a data secret; those kind of errors are usually due to misconfigurations
	// and user intervention is required to get them fixed.
	DataSecretGenerationFailedV1Beta1Reason = "DataSecretGenerationFailed"

	// ControlPlaneKubernetesVersionAvailableV1Beta1Condition documents whether the controller could resolve the
	// Kubernetes version used for worker join from the Cluster's control plane reference (when applicable).
	ControlPlaneKubernetesVersionAvailableV1Beta1Condition clusterv1.ConditionType = "ControlPlaneKubernetesVersionAvailable"

	// ControlPlaneKubernetesVersionResolutionFailedV1Beta1Reason (Severity=Warning) documents a failure to read
	// the control plane object or its Kubernetes version while resolving join bootstrap data.
	ControlPlaneKubernetesVersionResolutionFailedV1Beta1Reason = "ControlPlaneKubernetesVersionResolutionFailed"

	// ControlPlaneKubernetesVersionFromControlPlaneV1Beta1Reason documents that the Kubernetes version for worker
	// join was read from the Cluster's control plane reference.
	ControlPlaneKubernetesVersionFromControlPlaneV1Beta1Reason = "ControlPlaneKubernetesVersionFromControlPlane"

	// ControlPlaneKubernetesVersionFromMachineV1Beta1Reason documents that the Kubernetes version for worker join
	// uses the Machine's spec.version because the control plane reference is unset or does not expose a version.
	ControlPlaneKubernetesVersionFromMachineV1Beta1Reason = "ControlPlaneKubernetesVersionFromMachine"
)

const (
	// CertificatesAvailableV1Beta1Condition documents that cluster certificates are available.
	//
	// NOTE: Cluster certificates are generated only for the KubeadmConfig object linked to the initial control plane
	// machine, if the cluster is not using a control plane ref object, if the certificates are not provided
	// by the users.
	// IMPORTANT: This condition won't be re-created after clusterctl move.
	CertificatesAvailableV1Beta1Condition clusterv1.ConditionType = "CertificatesAvailable"

	// CertificatesGenerationFailedV1Beta1Reason (Severity=Warning) documents a KubeadmConfig controller detecting
	// an error while generating certificates; those kind of errors are usually temporary and the controller
	// automatically recover from them.
	CertificatesGenerationFailedV1Beta1Reason = "CertificatesGenerationFailed"

	// CertificatesCorruptedV1Beta1Reason (Severity=Error) documents a KubeadmConfig controller detecting
	// an error while retrieving certificates for a joining node.
	CertificatesCorruptedV1Beta1Reason = "CertificatesCorrupted"
)
