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

// Package v1alpha1 contains types to represent Kubernetes cluster configuration.
package v1alpha1 // import "k8s.io/kube-deploy/cluster-api/api/cluster/v1alpha1"

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// deepcopy-gen can be installed with:
// go get k8s.io/gengo/examples/deepcopy-gen
//go:generate deepcopy-gen -i . -O zz_generated.deepcopy

// Cluster is an API object representing cluster-wide configuration
// parameters and status.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type Cluster struct {
	metav1.ObjectMeta `json:"metadata"`
	metav1.TypeMeta   `json:",inline"`

	Spec   ClusterSpec   `json:"spec"`
	Status ClusterStatus `json:"status,omitempty"`
}

type ClusterSpec struct {
	// Cluster network configuration
	ClusterNetwork ClusterNetworkingConfig `json:"clusterNetwork"`

	// Provider-specific serialized configuration to use during
	// cluster creation. It is recommended that providers maintain
	// their own versioned API types that should be
	// serialized/deserialized from this field.
	ProviderConfig string `json:"providerConfig"`
}

// ClusterNetworkingConfig specifies the different networking
// parameters for a cluster.
type ClusterNetworkingConfig struct {
	// The subnet CIDR from which service VIPs are allocated.
	ServiceSubnet string `json:"serviceSubnet"`

	// The subnet CIDR from which POD networks are allocated.
	PodSubnet string `json:"podSubnet"`

	// Domain name for services.
	DNSDomain string `json:"dnsDomain"`
}

// ClusterStatus represents the current status of the cluster.
type ClusterStatus struct {
	// APIEndpoint represents the endpoint to communicate with the IP.
	APIEndpoints []APIEndpoint `json:"apiEndpoints"`

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

// APIEndpoint represents a reachable Kubernetes API endpoint.
type APIEndpoint struct {
	// The hostname on which the API server is serving.
	Host string `json:"host"`

	// The port on which the API server is serving.
	Port int `json:"port"`
}

type ClusterStatusError string

const (
	// InvalidConfigurationClusterError indicates that the cluster
	// configuration is invalid.
	InvalidConfigurationClusterError ClusterStatusError = "InvalidConfiguration"

	// UnsupportedChangeClusterError indicates that the cluster
	// spec has been updated in an unsupported way. That cannot be
	// reconciled.
	UnsupportedChangeClusterError ClusterStatusError = "UnsupportedChange"

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
