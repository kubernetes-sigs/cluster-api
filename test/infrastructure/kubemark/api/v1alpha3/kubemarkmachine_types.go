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

const (
	// MachineFinalizer allows the controller to clean up resources associated with KubemarkMachine before
	// removing it from the apiserver.
	MachineFinalizer = "kubemarkmachine.infrastructure.cluster.x-k8s.io"
)

// KubemarkMachineSpec defines the desired state of KubemarkMachine
type KubemarkMachineSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of KubemarkMachine. Edit KubemarkMachine_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}

// KubemarkMachineStatus defines the observed state of KubemarkMachine
type KubemarkMachineStatus struct {
	// Ready is true when the provider resource is ready.
	// +optional
	Ready bool `json:"ready"`
}

// +kubebuilder:subresource:status
// +kubebuilder:object:root=true

// KubemarkMachine is the Schema for the kubemarkmachines API
type KubemarkMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KubemarkMachineSpec   `json:"spec,omitempty"`
	Status KubemarkMachineStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KubemarkMachineList contains a list of KubemarkMachine
type KubemarkMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KubemarkMachine `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KubemarkMachine{}, &KubemarkMachineList{})
}
