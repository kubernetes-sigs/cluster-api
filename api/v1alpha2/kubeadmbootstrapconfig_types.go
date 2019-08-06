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

// KubeadmConfigSpec defines the desired state of KubeadmConfig.
// Either ClusterConfiguration and InitConfiguration should be defined or the JoinConfiguration should be defined.
type KubeadmConfigSpec struct {
	// ClusterConfiguration along with InitConfiguration are the configurations necessary for the init command
	// +optional
	ClusterConfiguration *kubeadmv1beta1.ClusterConfiguration `json:"clusterConfiguration,omitempty"`
	// InitConfiguration along with ClusterConfiguration are the configurations necessary for the init command
	// +optional
	InitConfiguration *kubeadmv1beta1.InitConfiguration `json:"initConfiguration,omitempty"`
	// JoinConfiguration is the kubeadm configuration for the join command
	// +optional
	JoinConfiguration *kubeadmv1beta1.JoinConfiguration `json:"joinConfiguration,omitempty"`
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

// Files defines the input for generating write_files in cloud-init.
type Files struct {
	// Path specifies the full path on disk where to store the file.
	Path string `json:"path"`

	// Owner specifies the ownership of the file, e.g. "root:root".
	// +optional
	Owner string `json:"owner,omitempty"`

	// Permissions specifies the permissions to assign to the file, e.g. "0640".
	// +optional
	Permissions string `json:"permissions,omitempty"`

	// Content is the actual content of the file.
	Content string `json:"content"`
}
