/*
Copyright 2021 The Kubernetes Authors.

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

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// ClusterResourceSet's ResourcesApplied condition and corresponding reasons that will be used in v1Beta2 API version.
const (
	// ResourcesAppliedV1Beta2Condition surfaces wether the resources in the ClusterResourceSet are applied to all matching clusters.
	// This indicates all resources exist, and no errors during applying them to all clusters.
	ResourcesAppliedV1Beta2Condition = "ResourcesApplied"

	// ResourcesAppliedV1beta2Reason is the reason used when all resources in the ClusterResourceSet object got applied
	// to all matching clusters.
	ResourcesAppliedV1beta2Reason = "Applied"

	// ResourcesNotAppliedV1Beta2Reason is the reason used when applying at least one of the resources to one of the matching clusters failed.
	ResourcesNotAppliedV1Beta2Reason = "NotApplied"

	// ResourcesAppliedWrongSecretTypeV1Beta2Reason is the reason used when the Secret's type in the resource list is not supported.
	ResourcesAppliedWrongSecretTypeV1Beta2Reason = "WrongSecretType"

	// ResourcesAppliedInternalErrorV1Beta2Reason surfaces unexpected failures when reconciling a ClusterResourceSet.
	ResourcesAppliedInternalErrorV1Beta2Reason = clusterv1.InternalErrorV1Beta2Reason
)

const (
	// ClusterResourceSetSecretType is the only accepted type of secret in resources.
	ClusterResourceSetSecretType corev1.SecretType = "addons.cluster.x-k8s.io/resource-set" //nolint:gosec

	// ClusterResourceSetFinalizer is added to the ClusterResourceSet object for additional cleanup logic on deletion.
	ClusterResourceSetFinalizer = "addons.cluster.x-k8s.io"
)

// ANCHOR: ClusterResourceSetSpec

// ClusterResourceSetSpec defines the desired state of ClusterResourceSet.
type ClusterResourceSetSpec struct {
	// Label selector for Clusters. The Clusters that are
	// selected by this will be the ones affected by this ClusterResourceSet.
	// It must match the Cluster labels. This field is immutable.
	// Label selector cannot be empty.
	ClusterSelector metav1.LabelSelector `json:"clusterSelector"`

	// resources is a list of Secrets/ConfigMaps where each contains 1 or more resources to be applied to remote clusters.
	// +optional
	Resources []ResourceRef `json:"resources,omitempty"`

	// strategy is the strategy to be used during applying resources. Defaults to ApplyOnce. This field is immutable.
	// +kubebuilder:validation:Enum=ApplyOnce;Reconcile
	// +optional
	Strategy string `json:"strategy,omitempty"`
}

// ANCHOR_END: ClusterResourceSetSpec

// ClusterResourceSetResourceKind is a string representation of a ClusterResourceSet resource kind.
type ClusterResourceSetResourceKind string

// Define the ClusterResourceSetResourceKind constants.
const (
	SecretClusterResourceSetResourceKind    ClusterResourceSetResourceKind = "Secret"
	ConfigMapClusterResourceSetResourceKind ClusterResourceSetResourceKind = "ConfigMap"
)

// ResourceRef specifies a resource.
type ResourceRef struct {
	// name of the resource that is in the same namespace with ClusterResourceSet object.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// kind of the resource. Supported kinds are: Secrets and ConfigMaps.
	// +kubebuilder:validation:Enum=Secret;ConfigMap
	Kind string `json:"kind"`
}

// ClusterResourceSetStrategy is a string representation of a ClusterResourceSet Strategy.
type ClusterResourceSetStrategy string

const (
	// ClusterResourceSetStrategyApplyOnce is the default strategy a ClusterResourceSet strategy is assigned by
	// ClusterResourceSet controller after being created if not specified by user.
	ClusterResourceSetStrategyApplyOnce ClusterResourceSetStrategy = "ApplyOnce"
	// ClusterResourceSetStrategyReconcile reapplies the resources managed by a ClusterResourceSet
	// if their normalized hash changes.
	ClusterResourceSetStrategyReconcile ClusterResourceSetStrategy = "Reconcile"
)

// SetTypedStrategy sets the Strategy field to the string representation of ClusterResourceSetStrategy.
func (c *ClusterResourceSetSpec) SetTypedStrategy(p ClusterResourceSetStrategy) {
	c.Strategy = string(p)
}

// ANCHOR: ClusterResourceSetStatus

// ClusterResourceSetStatus defines the observed state of ClusterResourceSet.
type ClusterResourceSetStatus struct {
	// observedGeneration reflects the generation of the most recently observed ClusterResourceSet.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// conditions defines current state of the ClusterResourceSet.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`

	// v1beta2 groups all the fields that will be added or modified in ClusterResourceSet's status with the V1Beta2 version.
	// +optional
	V1Beta2 *ClusterResourceSetV1Beta2Status `json:"v1beta2,omitempty"`
}

// ClusterResourceSetV1Beta2Status groups all the fields that will be added or modified in ClusterResourceSet with the V1Beta2 version.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type ClusterResourceSetV1Beta2Status struct {
	// conditions represents the observations of a ClusterResourceSet's current state.
	// Known condition types are ResourceSetApplied, Deleting.
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=32
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ANCHOR_END: ClusterResourceSetStatus

// GetConditions returns the set of conditions for this object.
func (m *ClusterResourceSet) GetConditions() clusterv1.Conditions {
	return m.Status.Conditions
}

// SetConditions sets the conditions on this object.
func (m *ClusterResourceSet) SetConditions(conditions clusterv1.Conditions) {
	m.Status.Conditions = conditions
}

// GetV1Beta2Conditions returns the set of conditions for this object.
func (m *ClusterResourceSet) GetV1Beta2Conditions() []metav1.Condition {
	if m.Status.V1Beta2 == nil {
		return nil
	}
	return m.Status.V1Beta2.Conditions
}

// SetV1Beta2Conditions sets conditions for an API object.
func (m *ClusterResourceSet) SetV1Beta2Conditions(conditions []metav1.Condition) {
	if m.Status.V1Beta2 == nil {
		m.Status.V1Beta2 = &ClusterResourceSetV1Beta2Status{}
	}
	m.Status.V1Beta2.Conditions = conditions
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=clusterresourcesets,scope=Namespaced,categories=cluster-api
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of ClusterResourceSet"

// ClusterResourceSet is the Schema for the clusterresourcesets API.
type ClusterResourceSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterResourceSetSpec   `json:"spec,omitempty"`
	Status ClusterResourceSetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterResourceSetList contains a list of ClusterResourceSet.
type ClusterResourceSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterResourceSet `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &ClusterResourceSet{}, &ClusterResourceSetList{})
}
