/*
Copyright 2026 The Kubernetes Authors.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=devmachinepooltemplates,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of DevMachinePoolTemplate"

// DevMachinePoolTemplate is the Schema for the devmachinepooltemplates API.
type DevMachinePoolTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec DevMachinePoolTemplateSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// DevMachinePoolTemplateList contains a list of DevMachinePoolTemplate.
type DevMachinePoolTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DevMachinePoolTemplate `json:"items"`
}

// DevMachinePoolTemplateSpec defines the desired state of DevMachinePoolTemplate.
type DevMachinePoolTemplateSpec struct {
	Template DevMachinePoolTemplateResource `json:"template"`
}

// DevMachinePoolTemplateResource describes the data needed to create a DevMachine from a template.
type DevMachinePoolTemplateResource struct {
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	ObjectMeta clusterv1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	Spec DevMachinePoolSpec `json:"spec"`
}

func init() {
	objectTypes = append(objectTypes, &DevMachinePoolTemplate{}, &DevMachinePoolTemplateList{})
}
