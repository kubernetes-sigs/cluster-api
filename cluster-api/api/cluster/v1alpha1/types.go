/*
Copyright 2017 The Kubernetes Authors.

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

// Package cluster contains types to represent Kubernetes cluster configuration.
package v1alpha1 // import "k8s.io/kube-deploy/cluster-api/api/cluster/v1alpha1"

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// Cluster is an API object representing a cluster's control-plane
// configuration and status.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type Cluster struct {
	metav1.ObjectMeta `json:"metadata"`
	metav1.TypeMeta   `json:",inline"`

	Spec   ClusterSpec   `json:"spec"`
	Status ClusterStatus `json:"status,omitempty"`
}

type ClusterSpec struct {
	// Nominal version of Kubernetes control-plane to run.
	KubernetesVersion KubernetesVersionInfo `json:"kubernetesVersion"`

	// Basic API configuration
	APIConfig APIServerConfig `json:"apiConfig"`

	// Cluster network configuration
	ClusterNetwork ClusterNetworkingConfig `json:"clusterNetwork"`

	// Provider-specific serialized configuration to use during
	// cluster creation. It is recommended that providers maintain
	// their own versioned API types that should be
	// serialized/deserialized from this field.
	ProviderConfig string `json:"providerConfig"`
}

type APIServerConfig struct {
	// The address for the API server to advertise.
	AdvertiseAddress string `json:"advertiseAddress"`

	// The port on which the API server binds.
	Port uint32 `json:"port"`

	// Extra Subject Alternative Names for the API server's serving cert.
	ExtraSANs []string `json:"extraSANs"`
}

type ClusterNetworkingConfig struct {
	// The subnet from which service VIPs are allocated.
	ServiceSubnet string `json:"serviceSubnet"`

	// The subnet from which POD networks are allocated.
	PodSubnet string `json:"podSubnet"`

	// Domain name for services.
	DNSDomain string `json:"dnsDomain"`
}

type KubernetesVersionInfo struct {
	// Semantic version of Kubernetes to run.
	Version string `json:"version"`
}

type ClusterStatus struct {
	// APIEndpoint represents the endpoint to communicate with the IP.
	APIEndpoint APIEndpoint `json:"apiEndpoint"`

	// A simple boolean to indicate whether the control plane was
	// successfully created.
	Ready bool `json:"ready"`

	// If set, indicates that there is a problem reconciling the
	// state, and will be set to a token value suitable for
	// programmatic interpretation.
	ErrorReason ClusterStatusError `json:"errorReason"`

	// If set, indicates that there is a problem reconciling the
	// state, and will be set to a descriptive error message.
	ErrorMessage string `json:"errorMessage"`

	// Provider-specific serialized status to use during cluster
	// creation. It is recommended that providers maintain their
	// own versioned API types that should be
	// serialized/deserialized from this field.
	ProviderStatus string `json:"providerStatus"`
}

type APIEndpoint struct {
	// The hostname on which the API server is serving.
	Host string `json:"host"`

	// The port on which the API server is serving.
	Port int `json:"port"`

	// The serving certificate for the API server.
	Cert []byte `json:"cert"`
}

//
type ClusterStatusError string

const (
	// InvalidConfigurationClusterError indicates that the cluster
	// configuration is invalid.
	InvalidConfigurationClusterError ClusterStatusError = "InvalidConfiguration"

	// UnsupportedChangeClusterError indicates that the cluster
	// spec has been updated in an unsupported way. That cannot be
	// reconciled.
	UnsupportedChangeClusterError ClusterStatusError = "UnsupportedChanged"

	// CreateClusterError indicates that an error was encountered
	// when trying to create the cluster.
	CreateClusterError ClusterStatusError = "CreateError"

	// UpdateClusterError indicates that an error was encountered
	// when trying to update the cluster.
	UpdateClusterError ClusterStatusError = "UpdateError"

	// DeleteClusterError indicates that an error was encountered
	// when trying to delete the cluster.
	DeleteClusterError ClusterStatusError = "DeleteError"
)

// This is needed to be able to list objects, even if we only expect one to be
// found at a time.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []Cluster `json:"items"`
}
