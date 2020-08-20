/*
Copyright 2020 The Kubernetes Authors.

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

package v1alpha3

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KubeadmIgnitionConfigTemplateSpec defines the desired state of KubeadmIgnitionConfigTemplate
type KubeadmIgnitionConfigTemplateSpec struct {
	Template KubeadmIgnitionConfigTemplateResource `json:"template"`
}

// KubeadmIgnitionConfigTemplateResource defines the Template structure
type KubeadmIgnitionConfigTemplateResource struct {
	Spec KubeadmIgnitionConfigSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=kubeadmconfigtemplates,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion

// KubeadmIgnitionConfigTemplate is the Schema for the kubeadmconfigtemplates API
type KubeadmIgnitionConfigTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec KubeadmIgnitionConfigTemplateSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// KubeadmIgnitionConfigTemplateList contains a list of KubeadmIgnitionConfigTemplate
type KubeadmIgnitionConfigTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KubeadmIgnitionConfigTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KubeadmIgnitionConfigTemplate{}, &KubeadmIgnitionConfigTemplateList{})
}
