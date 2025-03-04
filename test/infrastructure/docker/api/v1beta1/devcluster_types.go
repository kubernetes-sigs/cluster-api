/*
Copyright 2024 The Kubernetes Authors.

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

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

const (
	// ListenerAnnotationName tracks the name of the listener a cluster is linked to.
	// NOTE: the annotation must be added by the components that creates the listener only if using the HotRestart feature.
	ListenerAnnotationName = "inmemorycluster.infrastructure.cluster.x-k8s.io/listener"
)

// DevCluster's v1Beta2 conditions that apply to all the supported backends.

// DevCluster's Ready condition and corresponding reasons that will be used in v1Beta2 API version.
const (
	// DevClusterReadyV1Beta2Condition is true if
	// - The DevCluster's is using a docker backend and LoadBalancerAvailable is true.
	DevClusterReadyV1Beta2Condition = clusterv1.ReadyV1Beta2Condition

	// DevClusterReadyV1Beta2Reason surfaces when the DevCluster readiness criteria is met.
	DevClusterReadyV1Beta2Reason = clusterv1.ReadyV1Beta2Reason

	// DevClusterNotReadyV1Beta2Reason surfaces when the DevCluster readiness criteria is not met.
	DevClusterNotReadyV1Beta2Reason = clusterv1.NotReadyV1Beta2Reason

	// DevClusterReadyUnknownV1Beta2Reason surfaces when at least one DevCluster readiness criteria is unknown
	// and no DevCluster readiness criteria is not met.
	DevClusterReadyUnknownV1Beta2Reason = clusterv1.ReadyUnknownV1Beta2Reason
)

// DevCluster's v1Beta2 conditions that apply to the docker backend.

// LoadBalancerAvailable condition and corresponding reasons that will be used in v1Beta2 API version for a DevCluster's docker backend.
const (
	// DevClusterDockerLoadBalancerAvailableV1Beta2Condition documents the availability of the container that implements
	// the load balancer for a DevCluster's docker backend..
	DevClusterDockerLoadBalancerAvailableV1Beta2Condition string = "LoadBalancerAvailable"

	// DevClusterDockerLoadBalancerNotAvailableV1Beta2Reason surfaces when the container that implements
	// the load balancer for a DevCluster's docker backend is not available.
	DevClusterDockerLoadBalancerNotAvailableV1Beta2Reason = clusterv1.NotAvailableV1Beta2Reason

	// DevClusterDockerLoadBalancerAvailableV1Beta2Reason surfaces when the container that implements
	// the load balancer for a DevCluster's docker backend is available.
	DevClusterDockerLoadBalancerAvailableV1Beta2Reason = clusterv1.AvailableV1Beta2Reason

	// DevClusterDockerLoadBalancerDeletingV1Beta2Reason surfaces when the container that implements
	// the load balancer for a DevCluster's docker backend is deleting.
	DevClusterDockerLoadBalancerDeletingV1Beta2Reason = clusterv1.DeletingV1Beta2Reason
)

// DevClusterSpec defines the desired state of the DevCluster infrastructure.
type DevClusterSpec struct {
	// controlPlaneEndpoint represents the endpoint used to communicate with the control plane.
	// +optional
	ControlPlaneEndpoint APIEndpoint `json:"controlPlaneEndpoint"`

	// backend defines backends for a DevCluster.
	// +required
	Backend DevClusterBackendSpec `json:"backend"`
}

// DevClusterBackendSpec defines backends for a DevCluster.
type DevClusterBackendSpec struct {
	// docker defines a backend for a DevCluster using docker containers.
	// +optional
	Docker *DockerClusterBackendSpec `json:"docker,omitempty"`

	// inMemory defines a backend for a DevCluster that runs in memory.
	// +optional
	InMemory *InMemoryClusterBackendSpec `json:"inMemory,omitempty"`
}

// DockerClusterBackendSpec  defines a backend for a DevCluster using docker containers.
type DockerClusterBackendSpec struct {
	// failureDomains are usually not defined in the spec.
	// The docker provider is special since failure domains don't mean anything in a local docker environment.
	// Instead, the docker cluster controller will simply copy these into the Status and allow the Cluster API
	// controllers to do what they will with the defined failure domains.
	// +optional
	FailureDomains clusterv1.FailureDomains `json:"failureDomains,omitempty"`

	// loadBalancer allows defining configurations for the cluster load balancer.
	// +optional
	LoadBalancer DockerLoadBalancer `json:"loadBalancer,omitempty"`
}

// InMemoryClusterBackendSpec defines backend for a DevCluster that runs in memory.
type InMemoryClusterBackendSpec struct{}

// DevClusterStatus defines the observed state of the DevCluster.
type DevClusterStatus struct {
	// ready denotes that the dev cluster infrastructure is ready.
	// +optional
	Ready bool `json:"ready"`

	// failureDomains don't mean much in CAPD since it's all local, but we can see how the rest of cluster API
	// will use this if we populate it.
	// +optional
	FailureDomains clusterv1.FailureDomains `json:"failureDomains,omitempty"`

	// conditions defines current service state of the DevCluster.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`

	// v1beta2 groups all the fields that will be added or modified in DevCluster's status with the V1Beta2 version.
	// +optional
	V1Beta2 *DevClusterV1Beta2Status `json:"v1beta2,omitempty"`
}

// DevClusterV1Beta2Status groups all the fields that will be added or modified in DevCluster with the V1Beta2 version.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type DevClusterV1Beta2Status struct {
	// conditions represents the observations of a DevCluster's current state.
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=32
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:resource:path=devclusters,scope=Namespaced,categories=cluster-api
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".metadata.labels['cluster\\.x-k8s\\.io/cluster-name']",description="Cluster"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of the DevCluster"

// DevCluster is the schema for the dev cluster infrastructure API.
type DevCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DevClusterSpec   `json:"spec,omitempty"`
	Status DevClusterStatus `json:"status,omitempty"`
}

// GetConditions returns the set of conditions for this object.
func (c *DevCluster) GetConditions() clusterv1.Conditions {
	return c.Status.Conditions
}

// SetConditions sets the conditions on this object.
func (c *DevCluster) SetConditions(conditions clusterv1.Conditions) {
	c.Status.Conditions = conditions
}

// GetV1Beta2Conditions returns the set of conditions for this object.
func (c *DevCluster) GetV1Beta2Conditions() []metav1.Condition {
	if c.Status.V1Beta2 == nil {
		return nil
	}
	return c.Status.V1Beta2.Conditions
}

// SetV1Beta2Conditions sets conditions for an API object.
func (c *DevCluster) SetV1Beta2Conditions(conditions []metav1.Condition) {
	if c.Status.V1Beta2 == nil {
		c.Status.V1Beta2 = &DevClusterV1Beta2Status{}
	}
	c.Status.V1Beta2.Conditions = conditions
}

// +kubebuilder:object:root=true

// DevClusterList contains a list of DevCluster.
type DevClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DevCluster `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &DevCluster{}, &DevClusterList{})
}
