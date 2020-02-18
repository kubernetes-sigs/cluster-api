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
	kubeadmv1beta1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/v1beta1"
)

// Format specifies the output format of the bootstrap data
// +kubebuilder:validation:Enum=cloud-config
type Format string

const (
	// CloudConfig make the bootstrap data to be of cloud-config format
	CloudConfig Format = "cloud-config"
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
	// Files specifies extra files to be passed to user_data upon creation.
	// +optional
	Files []File `json:"files,omitempty"`
	// PreKubeadmCommands specifies extra commands to run before kubeadm runs
	// +optional
	PreKubeadmCommands []string `json:"preKubeadmCommands,omitempty"`
	// PostKubeadmCommands specifies extra commands to run after kubeadm runs
	// +optional
	PostKubeadmCommands []string `json:"postKubeadmCommands,omitempty"`
	// Users specifies extra users to add
	// +optional
	Users []User `json:"users,omitempty"`
	// NTP specifies NTP configuration
	// +optional
	NTP *NTP `json:"ntp,omitempty"`
	// Format specifies the output format of the bootstrap data
	// +optional
	Format Format `json:"format,omitempty"`
}

// KubeadmConfigStatus defines the observed state of KubeadmConfig
type KubeadmConfigStatus struct {
	// Ready indicates the BootstrapData field is ready to be consumed
	Ready bool `json:"ready,omitempty"`

	// BootstrapData will be a cloud-init script for now
	// +optional
	BootstrapData []byte `json:"bootstrapData,omitempty"`

	// ErrorReason will be set on non-retryable errors
	// +optional
	ErrorReason string `json:"errorReason,omitempty"`

	// ErrorMessage will be set on non-retryable errors
	// +optional
	ErrorMessage string `json:"errorMessage,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=kubeadmconfigs,scope=Namespaced,categories=cluster-api
// +kubebuilder:subresource:status

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

// Encoding specifies the cloud-init file encoding.
// +kubebuilder:validation:Enum=base64;gzip;gzip+base64
type Encoding string

const (
	// Base64 implies the contents of the file are encoded as base64.
	Base64 Encoding = "base64"
	// Gzip implies the contents of the file are encoded with gzip.
	Gzip Encoding = "gzip"
	// GzipBase64 implies the contents of the file are first base64 encoded and then gzip encoded.
	GzipBase64 Encoding = "gzip+base64"
)

// File defines the input for generating write_files in cloud-init.
type File struct {
	// Path specifies the full path on disk where to store the file.
	Path string `json:"path"`

	// Owner specifies the ownership of the file, e.g. "root:root".
	// +optional
	Owner string `json:"owner,omitempty"`

	// Permissions specifies the permissions to assign to the file, e.g. "0640".
	// +optional
	Permissions string `json:"permissions,omitempty"`

	// Encoding specifies the encoding of the file contents.
	// +optional
	Encoding Encoding `json:"encoding,omitempty"`

	// Content is the actual content of the file.
	Content string `json:"content"`
}

// User defines the input for a generated user in cloud-init.
type User struct {
	// Name specifies the user name
	Name string `json:"name"`

	// Gecos specifies the gecos to use for the user
	// +optional
	Gecos *string `json:"gecos,omitempty"`

	// Groups specifies the additional groups for the user
	// +optional
	Groups *string `json:"groups,omitempty"`

	// HomeDir specifies the home directory to use for the user
	// +optional
	HomeDir *string `json:"homeDir,omitempty"`

	// Inactive specifies whether to mark the user as inactive
	// +optional
	Inactive *bool `json:"inactive,omitempty"`

	// Shell specifies the user's shell
	// +optional
	Shell *string `json:"shell,omitempty"`

	// Passwd specifies a hashed password for the user
	// +optional
	Passwd *string `json:"passwd,omitempty"`

	// PrimaryGroup specifies the primary group for the user
	// +optional
	PrimaryGroup *string `json:"primaryGroup,omitempty"`

	// LockPassword specifies if password login should be disabled
	// +optional
	LockPassword *bool `json:"lockPassword,omitempty"`

	// Sudo specifies a sudo role for the user
	// +optional
	Sudo *string `json:"sudo,omitempty"`

	// SSHAuthorizedKeys specifies a list of ssh authorized keys for the user
	// +optional
	SSHAuthorizedKeys []string `json:"sshAuthorizedKeys,omitempty"`
}

// NTP defines input for generated ntp in cloud-init
type NTP struct {
	// Servers specifies which NTP servers to use
	// +optional
	Servers []string `json:"servers,omitempty"`

	// Enabled specifies whether NTP should be enabled
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}
