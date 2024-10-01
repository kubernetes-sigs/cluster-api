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

// Conditions that will be used for the ClusterResourceSet object in v1Beta2 API version.
const (
	// ClusterResourceSetReadyV1Beta2Condition is true if the ClusterClass is not deleted,
	// and ResourceSetApplied condition is true.
	ClusterResourceSetReadyV1Beta2Condition = clusterv1.ReadyV1beta2Condition

	// ClusterResourceSetResourceSetAppliedV1Beta2Condition documents that all resources in the ClusterResourceSet object
	// are applied to all matching clusters. This indicates all resources exist, and no errors during applying them to all clusters.
	ClusterResourceSetResourceSetAppliedV1Beta2Condition = "ResourceSetApplied"

	// ClusterResourceSetDeletingV1Beta2Condition surfaces details about ongoing deletion of the controlled machines.
	ClusterResourceSetDeletingV1Beta2Condition = clusterv1.DeletingV1beta2Condition

	// ClusterResourceSetPausedV1Beta2Condition is true if this ClusterResourceSet or the Cluster it belongs to are paused.
	ClusterResourceSetPausedV1Beta2Condition = clusterv1.PausedV1beta2Condition
)
