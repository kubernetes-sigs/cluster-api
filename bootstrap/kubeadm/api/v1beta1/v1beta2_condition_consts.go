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

// Conditions that will be used for the KubeadmConfig object in v1Beta2 API version.
const (
	// KubeadmConfigReadyV1Beta2Condition is true if the KubeadmConfig is not deleted,
	// and both DataSecretCreated, CertificatesAvailable conditions are true.
	KubeadmConfigReadyV1Beta2Condition = clusterv1.ReadyV1Beta2Condition

	// CertificatesAvailableV1Beta2Condition documents that cluster certificates required
	// for generating the bootstrap data secret are available.
	CertificatesAvailableV1Beta2Condition = "CertificatesAvailable"

	// DataSecretAvailableV1Beta2Condition is true if the bootstrap secret is available.
	DataSecretAvailableV1Beta2Condition = "DataSecretAvailable"
)
