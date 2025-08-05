/*
Copyright 2025 The Kubernetes Authors.

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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// DockerMachineTemplateSpec defines the desired state of DockerMachineTemplate.
type DockerMachineTemplateSpec struct {
	Template DockerMachineTemplateResource `json:"template"`
}

// DockerMachineTemplateStatus defines the observed state of a DockerMachineTemplate.
type DockerMachineTemplateStatus struct {
	// capacity defines the resource capacity for this DockerMachineTemplate.
	// This value is used for autoscaling from zero operations as defined in:
	// https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20210310-opt-in-autoscaling-from-zero.md
	// +optional
	Capacity corev1.ResourceList `json:"capacity,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=dockermachinetemplates,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of DockerMachineTemplate"

// DockerMachineTemplate is the Schema for the dockermachinetemplates API.
type DockerMachineTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DockerMachineTemplateSpec   `json:"spec,omitempty"`
	Status DockerMachineTemplateStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DockerMachineTemplateList contains a list of DockerMachineTemplate.
type DockerMachineTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DockerMachineTemplate `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &DockerMachineTemplate{}, &DockerMachineTemplateList{})
}

// DockerMachineTemplateResource describes the data needed to create a DockerMachine from a template.
type DockerMachineTemplateResource struct {
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	ObjectMeta clusterv1.ObjectMeta `json:"metadata,omitempty,omitzero"`
	// Spec is the specification of the desired behavior of the machine.
	Spec DockerMachineSpec `json:"spec"`
}
