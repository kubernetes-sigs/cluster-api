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
package cluster // import "k8s.io/kube-deploy/cluster-api/cluster"

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// Cluster is an API object representing a cluster's control-plane
// configuration and status.
type Cluster struct {
	metav1.ObjectMeta
	metav1.TypeMeta

	Spec   ClusterSpec
	Status ClusterStatus
}

type ClusterSpec struct {
	// Nominal version of Kubernetes control-plane to run.
	KubernetesVersion KubernetesVersionInfo

	// Basic API configuration
	APIConfig APIServerConfig

	// Cluster network configuration
	ClusterNetwork ClusterNetworkingConfig

	// Provider-specific serialized configuration to use during
	// cluster creation. It is recommended that providers maintain
	// their own versioned API types that should be
	// serialized/deserialized from this field.
	ProviderConfig string
}

type APIServerConfig struct {
	// The address for the API server to advertise.
	AdvertiseAddress string

	// The port on which the API server binds.
	Port uint32

	// Extra Subject Alternative Names for the API server's serving cert.
	ExtraSANs []string
}

type ClusterNetworkingConfig struct {
	// The subnet from which service VIPs are allocated.
	ServiceSubnet string

	// The subnet from which POD networks are allocated.
	PodSubnet string

	// Domain name for services.
	DNSDomain string
}

type KubernetesVersionInfo struct {
	// Semantic version of Kubernetes to run.
	Version string
}

type ClusterStatus struct {
	// APIEndpoint represents the endpoint to communicate with the IP.
	APIEndpoint APIEndpoint

	// A simple boolean to indicate whether the control plane was
	// successfully created.
	Ready bool

	// If set, indicates that there is a problem reconciling the
	// state, and will be set to a token value suitable for
	// programmatic interpretation.
	ErrorReason ClusterStatusError

	// If set, indicates that there is a problem reconciling the
	// state, and will be set to a descriptive error message.
	ErrorMessage string

	// Provider-specific serialized status to use during cluster
	// creation. It is recommended that providers maintain their
	// own versioned API types that should be
	// serialized/deserialized from this field.
	ProviderStatus string
}

type APIEndpoint struct {
	// The hostname on which the API server is serving.
	Host string

	// The port on which the API server is serving.
	Port int

	// The serving certificate for the API server.
	Cert []byte
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
