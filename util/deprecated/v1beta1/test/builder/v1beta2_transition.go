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

package builder

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// This files provide types to validate transition from clusterv1.Conditions in v1Beta1 API to the metav1.Conditions in the v1Beta2 API.
// Please refer to util/conditions/v1beta2/doc.go for more context.

var (
	// TestGroupVersion is group version used for test CRDs used for validating the v1beta2 transition.
	TestGroupVersion = schema.GroupVersion{Group: "test.cluster.x-k8s.io", Version: "v1alpha1"}

	// schemeBuilder is used to add go types to the GroupVersionKind scheme.
	schemeBuilder = runtime.NewSchemeBuilder(addTransitionV1beta2Types)

	// AddTransitionV1Beta2ToScheme adds the types for validating the transition to v1Beta2 in this group-version to the given scheme.
	AddTransitionV1Beta2ToScheme = schemeBuilder.AddToScheme
)

func addTransitionV1beta2Types(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(TestGroupVersion,
		&Phase0Obj{}, &Phase0ObjList{},
		&Phase1Obj{}, &Phase1ObjList{},
		&Phase2Obj{}, &Phase2ObjList{},
		&Phase3Obj{}, &Phase3ObjList{},
	)
	metav1.AddToGroupVersion(scheme, TestGroupVersion)
	return nil
}

// Phase0ObjList is a list of Phase0Obj.
// +kubebuilder:object:root=true
type Phase0ObjList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Phase0Obj `json:"items"`
}

// Phase0Obj defines an object with clusterv1.Conditions.
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=phase0obj,scope=Namespaced,categories=cluster-api
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
type Phase0Obj struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              Phase0ObjSpec   `json:"spec,omitempty"`
	Status            Phase0ObjStatus `json:"status,omitempty"`
}

// Phase0ObjSpec defines the spec of a Phase0Obj.
type Phase0ObjSpec struct {
	// +optional
	Foo string `json:"foo,omitempty"`
}

// Phase0ObjStatus defines the status of a Phase0Obj.
type Phase0ObjStatus struct {
	// +optional
	Bar string `json:"bar,omitempty"`

	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

// GetConditions returns the set of conditions for this object.
func (o *Phase0Obj) GetConditions() clusterv1.Conditions {
	return o.Status.Conditions
}

// SetConditions sets the conditions on this object.
func (o *Phase0Obj) SetConditions(conditions clusterv1.Conditions) {
	o.Status.Conditions = conditions
}

// Phase1ObjList is a list of Phase1Obj.
// +kubebuilder:object:root=true
type Phase1ObjList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Phase1Obj `json:"items"`
}

// Phase1Obj defines an object with conditions and experimental conditions.
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=phase1obj,scope=Namespaced,categories=cluster-api
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
type Phase1Obj struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              Phase1ObjSpec   `json:"spec,omitempty"`
	Status            Phase1ObjStatus `json:"status,omitempty"`
}

// Phase1ObjSpec defines the spec of a Phase1Obj.
type Phase1ObjSpec struct {
	// +optional
	Foo string `json:"foo,omitempty"`
}

// Phase1ObjStatus defines the status of a Phase1Obj.
type Phase1ObjStatus struct {
	// +optional
	Bar string `json:"bar,omitempty"`

	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`

	// +optional
	V1Beta2 *Phase1ObjV1Beta2Status `json:"v1beta2,omitempty"`
}

// Phase1ObjV1Beta2Status defines the status.V1Beta2 of a Phase1Obj.
type Phase1ObjV1Beta2Status struct {

	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=32
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// GetConditions returns the set of conditions for this object.
func (o *Phase1Obj) GetConditions() clusterv1.Conditions {
	return o.Status.Conditions
}

// SetConditions sets the conditions on this object.
func (o *Phase1Obj) SetConditions(conditions clusterv1.Conditions) {
	o.Status.Conditions = conditions
}

// GetV1Beta2Conditions returns the set of conditions for this object.
func (o *Phase1Obj) GetV1Beta2Conditions() []metav1.Condition {
	if o.Status.V1Beta2 == nil {
		return nil
	}
	return o.Status.V1Beta2.Conditions
}

// SetV1Beta2Conditions sets conditions for an API object.
func (o *Phase1Obj) SetV1Beta2Conditions(conditions []metav1.Condition) {
	if o.Status.V1Beta2 == nil {
		o.Status.V1Beta2 = &Phase1ObjV1Beta2Status{}
	}
	o.Status.V1Beta2.Conditions = conditions
}

// Phase2ObjList is a list of Phase2Obj.
// +kubebuilder:object:root=true
type Phase2ObjList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Phase2Obj `json:"items"`
}

// Phase2Obj defines an object with conditions and back compatibility conditions.
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=phase2obj,scope=Namespaced,categories=cluster-api
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
type Phase2Obj struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              Phase2ObjSpec   `json:"spec,omitempty"`
	Status            Phase2ObjStatus `json:"status,omitempty"`
}

// Phase2ObjSpec defines the spec of a Phase2Obj.
type Phase2ObjSpec struct {
	// +optional
	Foo string `json:"foo,omitempty"`
}

// Phase2ObjStatus defines the status of a Phase2Obj.
type Phase2ObjStatus struct {
	// +optional
	Bar string `json:"bar,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=32
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// +optional
	Deprecated *Phase2ObjDeprecatedStatus `json:"deprecated,omitempty"`
}

// Phase2ObjDeprecatedStatus defines the status.Deprecated of a Phase2Obj.
type Phase2ObjDeprecatedStatus struct {

	// +optional
	V1Beta1 *Phase2ObjDeprecatedV1Beta1Status `json:"v1beta1,omitempty"`
}

// Phase2ObjDeprecatedV1Beta1Status defines the status.Deprecated.V1Beta2 of a Phase2Obj.
type Phase2ObjDeprecatedV1Beta1Status struct {

	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

// GetConditions returns the set of conditions for this object.
func (o *Phase2Obj) GetConditions() clusterv1.Conditions {
	if o.Status.Deprecated == nil || o.Status.Deprecated.V1Beta1 == nil {
		return nil
	}
	return o.Status.Deprecated.V1Beta1.Conditions
}

// SetConditions sets the conditions on this object.
func (o *Phase2Obj) SetConditions(conditions clusterv1.Conditions) {
	if o.Status.Deprecated == nil {
		o.Status.Deprecated = &Phase2ObjDeprecatedStatus{V1Beta1: &Phase2ObjDeprecatedV1Beta1Status{}}
	}
	if o.Status.Deprecated.V1Beta1 == nil {
		o.Status.Deprecated.V1Beta1 = &Phase2ObjDeprecatedV1Beta1Status{}
	}
	o.Status.Deprecated.V1Beta1.Conditions = conditions
}

// GetV1Beta2Conditions returns the set of conditions for this object.
func (o *Phase2Obj) GetV1Beta2Conditions() []metav1.Condition {
	return o.Status.Conditions
}

// SetV1Beta2Conditions sets conditions for an API object.
func (o *Phase2Obj) SetV1Beta2Conditions(conditions []metav1.Condition) {
	o.Status.Conditions = conditions
}

// Phase3ObjList is a list of Phase3Obj.
// +kubebuilder:object:root=true
type Phase3ObjList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Phase3Obj `json:"items"`
}

// Phase3Obj defines an object with metav1.conditions.
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=phase3obj,scope=Namespaced,categories=cluster-api
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
type Phase3Obj struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              Phase3ObjSpec   `json:"spec,omitempty"`
	Status            Phase3ObjStatus `json:"status,omitempty"`
}

// Phase3ObjSpec defines the spec of a Phase3Obj.
type Phase3ObjSpec struct {
	// +optional
	Foo string `json:"foo,omitempty"`
}

// Phase3ObjStatus defines the status of a Phase3Obj.
type Phase3ObjStatus struct {
	// +optional
	Bar string `json:"bar,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=32
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// GetV1Beta2Conditions returns the set of conditions for this object.
func (o *Phase3Obj) GetV1Beta2Conditions() []metav1.Condition {
	return o.Status.Conditions
}

// SetV1Beta2Conditions sets conditions for an API object.
func (o *Phase3Obj) SetV1Beta2Conditions(conditions []metav1.Condition) {
	o.Status.Conditions = conditions
}
