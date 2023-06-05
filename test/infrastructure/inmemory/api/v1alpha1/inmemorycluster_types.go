/*
Copyright 2023 The Kubernetes Authors.

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

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

const (
	// ResourceGroupAnnotationName tracks the name of a resource group a InMemoryCluster cluster is linked to.
	ResourceGroupAnnotationName = "inmemorycluster.infrastructure.cluster.x-k8s.io/resource-group"

	// ClusterFinalizer allows InMemoryClusterReconciler to clean up resources associated with InMemoryCluster before
	// removing it from the API server.
	ClusterFinalizer = "inmemorycluster.infrastructure.cluster.x-k8s.io"
)

// InMemoryClusterSpec defines the desired state of the InMemoryCluster.
type InMemoryClusterSpec struct {
	// ControlPlaneEndpoint represents the endpoint used to communicate with the control plane.
	// +optional
	ControlPlaneEndpoint APIEndpoint `json:"controlPlaneEndpoint"`
}

// InMemoryClusterStatus defines the observed state of the InMemoryCluster.
type InMemoryClusterStatus struct {
	// Ready denotes that the in-memory cluster (infrastructure) is ready.
	// +optional
	Ready bool `json:"ready"`

	// Conditions defines current service state of the InMemoryCluster.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

// APIEndpoint represents a reachable Kubernetes API endpoint.
type APIEndpoint struct {
	// Host is the hostname on which the API server is serving.
	Host string `json:"host"`

	// Port is the port on which the API server is serving.
	// Defaults to 6443 if not set.
	Port int `json:"port"`
}

// +kubebuilder:resource:path=inmemoryclusters,scope=Namespaced,categories=cluster-api
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".metadata.labels['cluster\\.x-k8s\\.io/cluster-name']",description="Cluster"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of InMemoryCluster"

// InMemoryCluster is the schema for the in-memory cluster API.
type InMemoryCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InMemoryClusterSpec   `json:"spec,omitempty"`
	Status InMemoryClusterStatus `json:"status,omitempty"`
}

// GetConditions returns the set of conditions for this object.
func (c *InMemoryCluster) GetConditions() clusterv1.Conditions {
	return c.Status.Conditions
}

// SetConditions sets the conditions on this object.
func (c *InMemoryCluster) SetConditions(conditions clusterv1.Conditions) {
	c.Status.Conditions = conditions
}

// +kubebuilder:object:root=true

// InMemoryClusterList contains a list of InMemoryCluster.
type InMemoryClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []InMemoryCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&InMemoryCluster{}, &InMemoryClusterList{})
}
