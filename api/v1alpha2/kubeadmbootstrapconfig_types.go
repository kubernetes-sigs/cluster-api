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

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeadmv1beta1 "sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm/kubeadm/v1beta1"
)

// KubeadmConfigSpec defines the desired state of KubeadmConfig
type KubeadmConfigSpec struct {
	ClusterConfiguration kubeadmv1beta1.ClusterConfiguration `json:"clusterConfiguration"`
	InitConfiguration    kubeadmv1beta1.InitConfiguration    `json:"initConfiguration,omitempty"`
	JoinConfiguration    kubeadmv1beta1.JoinConfiguration    `json:"joinConfiguration,omitempty"`
}

// KubeadmConfigStatus defines the observed state of KubeadmConfig
type KubeadmConfigStatus struct {
	// Ready indicates the BootstrapData field is ready to be consumed
	Ready bool `json:"ready,omitempty"`

	// BootstrapData will be a cloud-init script for now
	// +optional
	BootstrapData []byte `json:"bootstrapData,omitempty"`
}

// +kubebuilder:object:root=true

// KubeadmConfig is the Schema for the kubeadmconfigs API
type KubeadmConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KubeadmConfigSpec   `json:"spec,omitempty"`
	Status KubeadmConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KubeadmConfigList contains a list of KubeadmConfig
type KubeadmConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KubeadmConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KubeadmConfig{}, &KubeadmConfigList{})
}
