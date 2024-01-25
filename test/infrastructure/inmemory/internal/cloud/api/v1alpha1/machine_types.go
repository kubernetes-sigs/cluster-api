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
)

// CloudMachineKind is the kind of CloudMachine.
const CloudMachineKind = "CloudMachine"

// CloudMachineSpec is the spec of CloudMachine.
type CloudMachineSpec struct {
}

// CloudMachineStatus is the status of CloudMachine.
type CloudMachineStatus struct {
}

// +kubebuilder:object:root=true

// CloudMachine represents a machine in memory.
type CloudMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CloudMachineSpec   `json:"spec,omitempty"`
	Status CloudMachineStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CloudMachineList contains a list of CloudMachine.
type CloudMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CloudMachine `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CloudMachine{}, &CloudMachineList{})
}
