/*
Copyright 2025 The Kubernetes Authors.

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

package v1beta2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
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
	DevClusterReadyV1Beta2Condition = clusterv1.ReadyCondition

	// DevClusterReadyV1Beta2Reason surfaces when the DevCluster readiness criteria is met.
	DevClusterReadyV1Beta2Reason = clusterv1.ReadyReason

	// DevClusterNotReadyV1Beta2Reason surfaces when the DevCluster readiness criteria is not met.
	DevClusterNotReadyV1Beta2Reason = clusterv1.NotReadyReason

	// DevClusterReadyUnknownV1Beta2Reason surfaces when at least one DevCluster readiness criteria is unknown
	// and no DevCluster readiness criteria is not met.
	DevClusterReadyUnknownV1Beta2Reason = clusterv1.ReadyUnknownReason
)

// DevCluster's v1Beta2 conditions that apply to the docker backend.

// LoadBalancerAvailable condition and corresponding reasons that will be used in v1Beta2 API version for a DevCluster's docker backend.
const (
	// DevClusterDockerLoadBalancerAvailableV1Beta2Condition documents the availability of the container that implements
	// the load balancer for a DevCluster's docker backend..
	DevClusterDockerLoadBalancerAvailableV1Beta2Condition string = "LoadBalancerAvailable"

	// DevClusterDockerLoadBalancerNotAvailableV1Beta2Reason surfaces when the container that implements
	// the load balancer for a DevCluster's docker backend is not available.
	DevClusterDockerLoadBalancerNotAvailableV1Beta2Reason = clusterv1.NotAvailableReason

	// DevClusterDockerLoadBalancerAvailableV1Beta2Reason surfaces when the container that implements
	// the load balancer for a DevCluster's docker backend is available.
	DevClusterDockerLoadBalancerAvailableV1Beta2Reason = clusterv1.AvailableReason

	// DevClusterDockerLoadBalancerDeletingV1Beta2Reason surfaces when the container that implements
	// the load balancer for a DevCluster's docker backend is deleting.
	DevClusterDockerLoadBalancerDeletingV1Beta2Reason = clusterv1.DeletingReason
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
	FailureDomains clusterv1beta1.FailureDomains `json:"failureDomains,omitempty"`

	// loadBalancer allows defining configurations for the cluster load balancer.
	// +optional
	LoadBalancer DockerLoadBalancer `json:"loadBalancer,omitempty"`
}

// InMemoryClusterBackendSpec defines backend for a DevCluster that runs in memory.
type InMemoryClusterBackendSpec struct{}

// DevClusterStatus defines the observed state of the DevCluster.
type DevClusterStatus struct {
	// conditions represents the observations of a DevCluster's current state.
	// Known condition types are Ready, LoadBalancerAvailable and Paused.
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=32
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ready denotes that the dev cluster infrastructure is ready.
	// +optional
	Ready bool `json:"ready"`

	// failureDomains don't mean much in CAPD since it's all local, but we can see how the rest of cluster API
	// will use this if we populate it.
	// +optional
	FailureDomains clusterv1beta1.FailureDomains `json:"failureDomains,omitempty"`

	// deprecated groups all the status fields that are deprecated and will be removed when all the nested field are removed.
	// +optional
	Deprecated *DevClusterDeprecatedStatus `json:"deprecated,omitempty"`
}

// DevClusterDeprecatedStatus groups all the status fields that are deprecated and will be removed when support for v1beta1 will be dropped.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type DevClusterDeprecatedStatus struct {
	// v1beta1 groups all the status fields that are deprecated and will be removed when support for v1beta1 will be dropped.
	// +optional
	V1Beta1 *DevClusterV1Beta1DeprecatedStatus `json:"v1beta1,omitempty"`
}

// DevClusterV1Beta1DeprecatedStatus groups all the status fields that are deprecated and will be removed when support for v1beta1 will be dropped.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type DevClusterV1Beta1DeprecatedStatus struct {
	// conditions defines current service state of the DevCluster.
	//
	// +optional
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 is dropped.
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
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

// GetV1Beta1Conditions returns the set of conditions for this object.
func (c *DevCluster) GetV1Beta1Conditions() clusterv1.Conditions {
	if c.Status.Deprecated == nil || c.Status.Deprecated.V1Beta1 == nil {
		return nil
	}
	return c.Status.Deprecated.V1Beta1.Conditions
}

// SetV1Beta1Conditions sets the conditions on this object.
func (c *DevCluster) SetV1Beta1Conditions(conditions clusterv1.Conditions) {
	if c.Status.Deprecated == nil {
		c.Status.Deprecated = &DevClusterDeprecatedStatus{}
	}
	if c.Status.Deprecated.V1Beta1 == nil {
		c.Status.Deprecated.V1Beta1 = &DevClusterV1Beta1DeprecatedStatus{}
	}
	c.Status.Deprecated.V1Beta1.Conditions = conditions
}

// GetConditions returns the set of conditions for this object.
func (c *DevCluster) GetConditions() []metav1.Condition {
	return c.Status.Conditions
}

// SetConditions sets conditions for an API object.
func (c *DevCluster) SetConditions(conditions []metav1.Condition) {
	c.Status.Conditions = conditions
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
