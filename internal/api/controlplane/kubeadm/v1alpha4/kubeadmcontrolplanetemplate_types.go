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

package v1alpha4

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KubeadmControlPlaneTemplateSpec defines the desired state of KubeadmControlPlaneTemplate.
type KubeadmControlPlaneTemplateSpec struct {
	// template defines the desired state of KubeadmControlPlaneTemplate.
	Template KubeadmControlPlaneTemplateResource `json:"template"`
}

// +kubebuilder:object:root=true
// +kubebuilder:unservedversion
// +kubebuilder:deprecatedversion
// +kubebuilder:resource:path=kubeadmcontrolplanetemplates,scope=Namespaced,categories=cluster-api
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of KubeadmControlPlaneTemplate"

// KubeadmControlPlaneTemplate is the Schema for the kubeadmcontrolplanetemplates API.
//
// Deprecated: This type will be removed in one of the next releases.
type KubeadmControlPlaneTemplate struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec is the desired state of KubeadmControlPlaneTemplate.
	Spec KubeadmControlPlaneTemplateSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// KubeadmControlPlaneTemplateList contains a list of KubeadmControlPlaneTemplate.
//
// Deprecated: This type will be removed in one of the next releases.
type KubeadmControlPlaneTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard list's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#lists-and-simple-kinds
	metav1.ListMeta `json:"metadata,omitempty"`
	// items is the list of KubeadmControlPlaneTemplates.
	Items []KubeadmControlPlaneTemplate `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &KubeadmControlPlaneTemplate{}, &KubeadmControlPlaneTemplateList{})
}

// KubeadmControlPlaneTemplateResource describes the data needed to create a KubeadmControlPlane from a template.
type KubeadmControlPlaneTemplateResource struct {
	// spec is the desired state of KubeadmControlPlane.
	Spec KubeadmControlPlaneSpec `json:"spec"`
}
