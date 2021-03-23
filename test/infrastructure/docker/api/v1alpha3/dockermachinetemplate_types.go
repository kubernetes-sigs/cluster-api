/*
Copyright 2019 The Kubernetes Authors.

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

// DockerMachineTemplateSpec defines the desired state of DockerMachineTemplate.
type DockerMachineTemplateSpec struct {
	Template DockerMachineTemplateResource `json:"template"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=dockermachinetemplates,scope=Namespaced,categories=cluster-api

// DockerMachineTemplate is the Schema for the dockermachinetemplates API.
type DockerMachineTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec DockerMachineTemplateSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// DockerMachineTemplateList contains a list of DockerMachineTemplate.
type DockerMachineTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DockerMachineTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DockerMachineTemplate{}, &DockerMachineTemplateList{})
}

// DockerMachineTemplateResource describes the data needed to create a DockerMachine from a template.
type DockerMachineTemplateResource struct {
	// Spec is the specification of the desired behavior of the machine.
	Spec DockerMachineSpec `json:"spec"`
}
