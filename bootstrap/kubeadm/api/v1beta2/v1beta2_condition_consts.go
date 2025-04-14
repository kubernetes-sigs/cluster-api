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

// KubeadmConfig's Ready condition and corresponding reasons that will be used in v1Beta2 API version.
const (
	// KubeadmConfigReadyCondition is true if the KubeadmConfig is not deleted,
	// and both DataSecretCreated, CertificatesAvailable conditions are true.
	KubeadmConfigReadyCondition = clusterv1.ReadyCondition

	// KubeadmConfigReadyReason surfaces when the KubeadmConfig is ready.
	KubeadmConfigReadyReason = clusterv1.ReadyReason

	// KubeadmConfigNotReadyReason surfaces when the KubeadmConfig is not ready.
	KubeadmConfigNotReadyReason = clusterv1.NotReadyReason

	// KubeadmConfigReadyUnknownReason surfaces when KubeadmConfig readiness is unknown.
	KubeadmConfigReadyUnknownReason = clusterv1.ReadyUnknownReason
)

// KubeadmConfig's CertificatesAvailable condition and corresponding reasons that will be used in v1Beta2 API version.
const (
	// KubeadmConfigCertificatesAvailableCondition documents that cluster certificates required
	// for generating the bootstrap data secret are available.
	KubeadmConfigCertificatesAvailableCondition = "CertificatesAvailable"

	// KubeadmConfigCertificatesAvailableReason surfaces when certificates required for machine bootstrap are is available.
	KubeadmConfigCertificatesAvailableReason = clusterv1.AvailableReason

	// KubeadmConfigCertificatesAvailableInternalErrorReason surfaces unexpected failures when reading or
	// generating certificates required for machine bootstrap.
	KubeadmConfigCertificatesAvailableInternalErrorReason = clusterv1.InternalErrorReason
)

// KubeadmConfig's DataSecretAvailable condition and corresponding reasons that will be used in v1Beta2 API version.
const (
	// KubeadmConfigDataSecretAvailableCondition is true if the bootstrap secret is available.
	KubeadmConfigDataSecretAvailableCondition = "DataSecretAvailable"

	// KubeadmConfigDataSecretAvailableReason surfaces when the bootstrap secret is available.
	KubeadmConfigDataSecretAvailableReason = clusterv1.AvailableReason

	// KubeadmConfigDataSecretNotAvailableReason surfaces when the bootstrap secret is not available.
	KubeadmConfigDataSecretNotAvailableReason = clusterv1.NotAvailableReason
)
