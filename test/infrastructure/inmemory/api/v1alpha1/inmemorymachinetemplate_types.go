/*
Copyright 2023 The Kubernetes Authors.

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// InMemoryMachineTemplateSpec defines the desired state of InMemoryMachineTemplate.
type InMemoryMachineTemplateSpec struct {
	Template InMemoryMachineTemplateResource `json:"template"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=inmemorymachinetemplates,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of InMemoryMachineTemplate"

// InMemoryMachineTemplate is the schema for the in-memory machine template API.
type InMemoryMachineTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec InMemoryMachineTemplateSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// InMemoryMachineTemplateList contains a list of InMemoryMachineTemplate.
type InMemoryMachineTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []InMemoryMachineTemplate `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &InMemoryMachineTemplate{}, &InMemoryMachineTemplateList{})
}

// InMemoryMachineTemplateResource describes the data needed to create a InMemoryMachine from a template.
type InMemoryMachineTemplateResource struct {
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	ObjectMeta clusterv1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the specification of the desired behavior of the machine.
	Spec InMemoryMachineSpec `json:"spec"`
}
