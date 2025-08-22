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

// Conditions and condition Reasons for the ClusterResourceSet object.
const (
	// ResourcesAppliedV1Beta1Condition documents that all resources in the ClusterResourceSet object are applied to
	// all matching clusters. This indicates all resources exist, and no errors during applying them to all clusters.
	ResourcesAppliedV1Beta1Condition clusterv1.ConditionType = "ResourcesApplied"

	// RemoteClusterClientFailedV1Beta1Reason (Severity=Error) documents failure during getting the remote cluster client.
	RemoteClusterClientFailedV1Beta1Reason = "RemoteClusterClientFailed"

	// ClusterMatchFailedV1Beta1Reason (Severity=Warning) documents failure getting clusters that match the clusterSelector.
	ClusterMatchFailedV1Beta1Reason = "ClusterMatchFailed"

	// ApplyFailedV1Beta1Reason (Severity=Warning) documents applying at least one of the resources to one of the matching clusters is failed.
	ApplyFailedV1Beta1Reason = "ApplyFailed"

	// RetrievingResourceFailedV1Beta1Reason (Severity=Warning) documents at least one of the resources are not successfully retrieved.
	RetrievingResourceFailedV1Beta1Reason = "RetrievingResourceFailed"

	// WrongSecretTypeV1Beta1Reason (Severity=Warning) documents at least one of the Secret's type in the resource list is not supported.
	WrongSecretTypeV1Beta1Reason = "WrongSecretType"
)
