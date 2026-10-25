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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// ClusterResourceSet's ResourcesApplied condition and corresponding reasons.
const (
	// ClusterResourceSetResourcesAppliedCondition surfaces wether the resources in the ClusterResourceSet are applied to all matching clusters.
	// This indicates all resources exist, and no errors during applying them to all clusters.
	ClusterResourceSetResourcesAppliedCondition = "ResourcesApplied"

	// ClusterResourceSetResourcesAppliedReason is the reason used when all resources in the ClusterResourceSet object got applied
	// to all matching clusters.
	ClusterResourceSetResourcesAppliedReason = "Applied"

	// ClusterResourceSetResourcesNotAppliedReason is the reason used when applying at least one of the resources to one of the matching clusters failed.
	ClusterResourceSetResourcesNotAppliedReason = "NotApplied"

	// ClusterResourceSetResourcesAppliedWrongSecretTypeReason is the reason used when the Secret's type in the resource list is not supported.
	ClusterResourceSetResourcesAppliedWrongSecretTypeReason = "WrongSecretType"

	// ClusterResourceSetResourcesAppliedInternalErrorReason surfaces unexpected failures when reconciling a ClusterResourceSet.
	ClusterResourceSetResourcesAppliedInternalErrorReason = clusterv1.InternalErrorReason
)

const (
	// ClusterResourceSetSecretType is the only accepted type of secret in resources.
	ClusterResourceSetSecretType corev1.SecretType = "addons.cluster.x-k8s.io/resource-set" //nolint:gosec

	// ClusterResourceSetFinalizer is added to the ClusterResourceSet object for additional cleanup logic on deletion.
	ClusterResourceSetFinalizer = "addons.cluster.x-k8s.io"
)

// ClusterResourceSetSpec defines the desired state of ClusterResourceSet.
type ClusterResourceSetSpec struct {
	// clusterSelector is the label selector for Clusters. The Clusters that are
	// selected by this will be the ones affected by this ClusterResourceSet.
	// It must match the Cluster labels. This field is immutable.
	// Label selector cannot be empty.
	// +required
	ClusterSelector metav1.LabelSelector `json:"clusterSelector,omitempty,omitzero"`

	// resources is a list of Secrets/ConfigMaps where each contains 1 or more resources to be applied to remote clusters.
	// +required
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	Resources []ResourceRef `json:"resources,omitempty"`

	// strategy is the strategy to be used during applying resources. Defaults to ApplyOnce. This field is immutable.
	// +kubebuilder:validation:Enum=ApplyOnce;Reconcile
	// +optional
	Strategy string `json:"strategy,omitempty"`
}

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
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name,omitempty"`

	// kind of the resource. Supported kinds are: Secrets and ConfigMaps.
	// +kubebuilder:validation:Enum=Secret;ConfigMap
	// +required
	Kind string `json:"kind,omitempty"`
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

// ClusterResourceSetStatus defines the observed state of ClusterResourceSet.
// +kubebuilder:validation:MinProperties=1
type ClusterResourceSetStatus struct {
	// conditions represents the observations of a ClusterResourceSet's current state.
	// Known condition types are ResourcesApplied.
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=32
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// observedGeneration reflects the generation of the most recently observed ClusterResourceSet.
	// +optional
	// +kubebuilder:validation:Minimum=1
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// deprecated groups all the status fields that are deprecated and will be removed when all the nested field are removed.
	// +optional
	Deprecated *ClusterResourceSetDeprecatedStatus `json:"deprecated,omitempty"`
}

// ClusterResourceSetDeprecatedStatus groups all the status fields that are deprecated and will be removed in a future version.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type ClusterResourceSetDeprecatedStatus struct {
	// v1beta1 groups all the status fields that are deprecated and will be removed when support for v1beta1 will be dropped.
	// +optional
	V1Beta1 *ClusterResourceSetV1Beta1DeprecatedStatus `json:"v1beta1,omitempty"`
}

// ClusterResourceSetV1Beta1DeprecatedStatus groups all the status fields that are deprecated and will be removed when support for v1beta1 will be dropped.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type ClusterResourceSetV1Beta1DeprecatedStatus struct {
	// conditions defines current state of the ClusterResourceSet.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

// GetV1Beta1Conditions returns the set of conditions for this object.
func (m *ClusterResourceSet) GetV1Beta1Conditions() clusterv1.Conditions {
	if m.Status.Deprecated == nil || m.Status.Deprecated.V1Beta1 == nil {
		return nil
	}
	return m.Status.Deprecated.V1Beta1.Conditions
}

// SetV1Beta1Conditions sets the conditions on this object.
func (m *ClusterResourceSet) SetV1Beta1Conditions(conditions clusterv1.Conditions) {
	if m.Status.Deprecated == nil {
		m.Status.Deprecated = &ClusterResourceSetDeprecatedStatus{}
	}
	if m.Status.Deprecated.V1Beta1 == nil {
		m.Status.Deprecated.V1Beta1 = &ClusterResourceSetV1Beta1DeprecatedStatus{}
	}
	m.Status.Deprecated.V1Beta1.Conditions = conditions
}

// GetConditions returns the set of conditions for this object.
func (m *ClusterResourceSet) GetConditions() []metav1.Condition {
	return m.Status.Conditions
}

// SetConditions sets conditions for an API object.
func (m *ClusterResourceSet) SetConditions(conditions []metav1.Condition) {
	m.Status.Conditions = conditions
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=clusterresourcesets,scope=Namespaced,categories=cluster-api
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Applied",type="string",JSONPath=`.status.conditions[?(@.type=="ResourcesApplied")].status`,description="Resource applied"
// +kubebuilder:printcolumn:name="Paused",type="string",JSONPath=`.status.conditions[?(@.type=="Paused")].status`,description="Reconciliation paused",priority=10
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of ClusterResourceSet"

// ClusterResourceSet is the Schema for the clusterresourcesets API.
// For advanced use cases an add-on provider should be used instead.
type ClusterResourceSet struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec is the desired state of ClusterResourceSet.
	// +required
	Spec ClusterResourceSetSpec `json:"spec,omitempty,omitzero"`
	// status is the observed state of ClusterResourceSet.
	// +optional
	Status ClusterResourceSetStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// ClusterResourceSetList contains a list of ClusterResourceSet.
type ClusterResourceSetList struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard list's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#lists-and-simple-kinds
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`
	// items is the list of ClusterResourceSets.
	Items []ClusterResourceSet `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &ClusterResourceSet{}, &ClusterResourceSetList{})
}
