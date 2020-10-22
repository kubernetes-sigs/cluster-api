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
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
)

const (
	// MachineFinalizer allows the controller to clean up resources associated with KubemarkMachine before
	// removing it from the apiserver.
	MachineFinalizer = "kubemarkmachine.infrastructure.cluster.x-k8s.io"
)

// KubemarkMachineSpec defines the desired state of KubemarkMachine
type KubemarkMachineSpec struct {
}

// KubemarkMachineStatus defines the observed state of KubemarkMachine
type KubemarkMachineStatus struct {
	// Ready is true when the provider resource is ready.
	// +optional
	Ready bool `json:"ready"`

	// Conditions defines current service state of the DockerMachine.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
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

func (c *KubemarkMachine) GetConditions() clusterv1.Conditions {
	return c.Status.Conditions
}

func (c *KubemarkMachine) SetConditions(conditions clusterv1.Conditions) {
	c.Status.Conditions = conditions
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
