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

package controlplane

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GenericControlPlaneSpec contains a generic control plane spec.
type GenericControlPlaneSpec struct {
	InfrastructureTemplate corev1.ObjectReference `json:"infrastructureTemplate"`
}

// +kubebuilder:object:root=true

// GenericControlPlane is a generic representation of a control plane.
type GenericControlPlane struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              GenericControlPlaneSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// GenericControlPlaneList is list of generic control planes.
type GenericControlPlaneList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GenericControlPlane `json:"items"`
}

func init() {
	SchemeBuilder.Register(
		&GenericControlPlane{}, &GenericControlPlaneList{},
	)
}
