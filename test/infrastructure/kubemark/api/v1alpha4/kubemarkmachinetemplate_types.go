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

package v1alpha4

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// KubemarkMachineTemplateSpec defines the desired state of KubemarkMachineTemplate
type KubemarkMachineTemplateSpec struct {
	Template KubemarkMachineTemplateResource `json:"template"`
}

// +kubebuilder:object:root=true

// KubemarkMachineTemplate is the Schema for the kubemarkmachinetemplates API
type KubemarkMachineTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec KubemarkMachineTemplateSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// KubemarkMachineTemplateList contains a list of KubemarkMachineTemplate
type KubemarkMachineTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KubemarkMachineTemplate `json:"items"`
}

// KubemarkMachineTemplateResource describes the data needed to create am KubemarkMachine from a template
type KubemarkMachineTemplateResource struct {
	// Spec is the specification of the desired behavior of the machine.
	Spec KubemarkMachineSpec `json:"spec"`
}

func init() {
	SchemeBuilder.Register(&KubemarkMachineTemplate{}, &KubemarkMachineTemplateList{})
}
