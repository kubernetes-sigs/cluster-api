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

const (
	// MachineFinalizer allows ReconcileDockerMachine to clean up resources associated with AWSMachine before
	// removing it from the apiserver.
	MachineFinalizer = "dockermachine.infrastructure.cluster.x-k8s.io"
)

// DockerMachineSpec defines the desired state of DockerMachine
type DockerMachineSpec struct {
	// ProviderID will be the container name in ProviderID format (docker:////<containername>)
	// +optional
	ProviderID *string `json:"providerID,omitempty"`

	// CustomImage allows customizing the container image that is used for
	// running the machine
	// +optional
	CustomImage string `json:"customImage,omitempty"`
}

// DockerMachineStatus defines the observed state of DockerMachine
type DockerMachineStatus struct {
	// Ready denotes that the machine (docker container) is ready
	Ready bool `json:"ready"`
}

// +kubebuilder:resource:path=dockermachines,scope=Namespaced,categories=cluster-api
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:subresource:status

// DockerMachine is the Schema for the dockermachines API
type DockerMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DockerMachineSpec   `json:"spec,omitempty"`
	Status DockerMachineStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DockerMachineList contains a list of DockerMachine
type DockerMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DockerMachine `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DockerMachine{}, &DockerMachineList{})
}
