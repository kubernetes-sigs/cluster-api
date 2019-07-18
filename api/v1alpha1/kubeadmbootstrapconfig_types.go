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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeadmv1beta1 "sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm/kubeadm/v1beta1"
)

// Phase defines KubeadmBootstrapConfig phases
type Phase string

// Ready defines the KubeadmBootstrapConfig Ready Phase
const (
	// Ready indicates the config is ready to be used by a Machine.
	Ready Phase = "Ready"
)

// KubeadmBootstrapConfigSpec defines the desired state of KubeadmBootstrapConfig
type KubeadmBootstrapConfigSpec struct {
	ClusterConfiguration kubeadmv1beta1.ClusterConfiguration `json:"clusterConfiguration"`
	InitConfiguration    kubeadmv1beta1.InitConfiguration    `json:"initConfiguration,omitempty"`
	JoinConfiguration    kubeadmv1beta1.JoinConfiguration    `json:"joinConfiguration,omitempty"`
}

// KubeadmBootstrapConfigStatus defines the observed state of KubeadmBootstrapConfig
type KubeadmBootstrapConfigStatus struct {
	// Phase is the state of the KubeadmBootstrapConfig object
	Phase Phase `json:"phase"`

	// BootstrapData will be a cloud-init script for now
	// +optional
	BootstrapData []byte `json:"bootstrapData,omitempty"`
}

// +kubebuilder:object:root=true

// KubeadmBootstrapConfig is the Schema for the kubeadmbootstrapconfigs API
type KubeadmBootstrapConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KubeadmBootstrapConfigSpec   `json:"spec,omitempty"`
	Status KubeadmBootstrapConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KubeadmBootstrapConfigList contains a list of KubeadmBootstrapConfig
type KubeadmBootstrapConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KubeadmBootstrapConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KubeadmBootstrapConfig{}, &KubeadmBootstrapConfigList{})
}
