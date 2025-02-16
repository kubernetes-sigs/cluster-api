/*
Copyright 2024 The Kubernetes Authors.

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

package v1beta1

import clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

// KubeadmConfig's Ready condition and corresponding reasons that will be used in v1Beta2 API version.
const (
	// KubeadmConfigReadyV1Beta2Condition is true if the KubeadmConfig is not deleted,
	// and both DataSecretCreated, CertificatesAvailable conditions are true.
	KubeadmConfigReadyV1Beta2Condition = clusterv1.ReadyV1Beta2Condition

	// KubeadmConfigReadyV1Beta2Reason surfaces when the KubeadmConfig is ready.
	KubeadmConfigReadyV1Beta2Reason = clusterv1.ReadyV1Beta2Reason

	// KubeadmConfigNotReadyV1Beta2Reason surfaces when the KubeadmConfig is not ready.
	KubeadmConfigNotReadyV1Beta2Reason = clusterv1.NotReadyV1Beta2Reason

	// KubeadmConfigReadyUnknownV1Beta2Reason surfaces when KubeadmConfig readiness is unknown.
	KubeadmConfigReadyUnknownV1Beta2Reason = clusterv1.ReadyUnknownV1Beta2Reason
)

// KubeadmConfig's CertificatesAvailable condition and corresponding reasons that will be used in v1Beta2 API version.
const (
	// KubeadmConfigCertificatesAvailableV1Beta2Condition documents that cluster certificates required
	// for generating the bootstrap data secret are available.
	KubeadmConfigCertificatesAvailableV1Beta2Condition = "CertificatesAvailable"

	// KubeadmConfigCertificatesAvailableV1Beta2Reason surfaces when certificates required for machine bootstrap are is available.
	KubeadmConfigCertificatesAvailableV1Beta2Reason = clusterv1.AvailableV1Beta2Reason

	// KubeadmConfigCertificatesAvailableInternalErrorV1Beta2Reason surfaces unexpected failures when reading or
	// generating certificates required for machine bootstrap.
	KubeadmConfigCertificatesAvailableInternalErrorV1Beta2Reason = clusterv1.InternalErrorV1Beta2Reason
)

// KubeadmConfig's DataSecretAvailable condition and corresponding reasons that will be used in v1Beta2 API version.
const (
	// KubeadmConfigDataSecretAvailableV1Beta2Condition is true if the bootstrap secret is available.
	KubeadmConfigDataSecretAvailableV1Beta2Condition = "DataSecretAvailable"

	// KubeadmConfigDataSecretAvailableV1Beta2Reason surfaces when the bootstrap secret is available.
	KubeadmConfigDataSecretAvailableV1Beta2Reason = clusterv1.AvailableV1Beta2Reason

	// KubeadmConfigDataSecretNotAvailableV1Beta2Reason surfaces when the bootstrap secret is not available.
	KubeadmConfigDataSecretNotAvailableV1Beta2Reason = clusterv1.NotAvailableV1Beta2Reason
)
