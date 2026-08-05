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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

const (
	// ClusterFinalizer allows DockerClusterReconciler to clean up resources associated with DevCluster before
	// removing it from the apiserver.
	ClusterFinalizer = "dockercluster.infrastructure.cluster.x-k8s.io"

	// ListenerAnnotationName tracks the name of the listener a cluster is linked to.
	// NOTE: the annotation must be added by the components that creates the listener only if using the HotRestart feature.
	ListenerAnnotationName = "inmemorycluster.infrastructure.cluster.x-k8s.io/listener"
)

// Deprecated v1beta1 conditions and reasons for DevCluster's docker backend.
const (
	// LoadBalancerAvailableV1Beta1Condition documents the availability of the container that implements the cluster load balancer.
	//
	// Deprecated: This condition is deprecated and is going to be removed when support for v1beta1 is dropped.
	LoadBalancerAvailableV1Beta1Condition clusterv1.ConditionType = "LoadBalancerAvailable"

	// LoadBalancerProvisioningFailedV1Beta1Reason documents a DockerCluster controller detecting
	// an error while provisioning the container that provides the cluster load balancer.
	//
	// Deprecated: This reason is deprecated and is going to be removed when support for v1beta1 is dropped.
	LoadBalancerProvisioningFailedV1Beta1Reason = "LoadBalancerProvisioningFailed"
)

// ImageMeta allows customizing the image used for components that are not
// originated from the Kubernetes/Kubernetes release process.
type ImageMeta struct {
	// ImageRepository sets the container registry to pull the haproxy image from.
	// if not set, "kindest" will be used instead.
	// +optional
	ImageRepository string `json:"imageRepository,omitempty"`

	// ImageTag allows to specify a tag for the haproxy image.
	// if not set, "v20210715-a6da3463" will be used instead.
	// +optional
	ImageTag string `json:"imageTag,omitempty"`
}

// DockerLoadBalancer allows defining configurations for the cluster load balancer.
type DockerLoadBalancer struct {
	// ImageMeta allows customizing the image used for the cluster load balancer.
	ImageMeta `json:",inline"`

	// CustomHAProxyConfigTemplateRef allows you to replace the default HAProxy config file.
	// This field is a reference to a config map that contains the configuration template. The key of the config map should be equal to 'value'.
	// The content of the config map will be processed and will replace the default HAProxy config file. Please use it with caution, as there are
	// no checks to ensure the validity of the configuration. This template will support the following variables that will be passed by the controller:
	// $IPv6 (bool) indicates if the cluster is IPv6, $FrontendControlPlanePort (string) indicates the frontend control plane port,
	// $BackendControlPlanePort (string) indicates the backend control plane port, $BackendServers (map[string]string) indicates the backend server
	// where the key is the server name and the value is the address. This map is dynamic and is updated every time a new control plane
	// node is added or removed. The template will also support the JoinHostPort function to join the host and port of the backend server.
	// +optional
	CustomHAProxyConfigTemplateRef *corev1.LocalObjectReference `json:"customHAProxyConfigTemplateRef,omitempty"`
}

// DevCluster's conditions that apply to all the supported backends.

// DevCluster's Ready condition and corresponding reasons.
const (
	// DevClusterReadyCondition is true if
	// - The DevCluster's is using a docker backend and LoadBalancerAvailable is true.
	DevClusterReadyCondition = clusterv1.ReadyCondition

	// DevClusterReadyReason surfaces when the DevCluster readiness criteria is met.
	DevClusterReadyReason = clusterv1.ReadyReason

	// DevClusterNotReadyReason surfaces when the DevCluster readiness criteria is not met.
	DevClusterNotReadyReason = clusterv1.NotReadyReason

	// DevClusterReadyUnknownReason surfaces when at least one DevCluster readiness criteria is unknown
	// and no DevCluster readiness criteria is not met.
	DevClusterReadyUnknownReason = clusterv1.ReadyUnknownReason
)

// DevCluster's conditions that apply to the docker backend.

// LoadBalancerAvailable condition and corresponding reasons for a DevCluster's docker backend.
const (
	// DevClusterDockerLoadBalancerAvailableCondition documents the availability of the container that implements
	// the load balancer for a DevCluster's docker backend..
	DevClusterDockerLoadBalancerAvailableCondition string = "LoadBalancerAvailable"

	// DevClusterDockerLoadBalancerNotAvailableReason surfaces when the container that implements
	// the load balancer for a DevCluster's docker backend is not available.
	DevClusterDockerLoadBalancerNotAvailableReason = clusterv1.NotAvailableReason

	// DevClusterDockerLoadBalancerAvailableReason surfaces when the container that implements
	// the load balancer for a DevCluster's docker backend is available.
	DevClusterDockerLoadBalancerAvailableReason = clusterv1.AvailableReason

	// DevClusterDockerLoadBalancerDeletingReason surfaces when the container that implements
	// the load balancer for a DevCluster's docker backend is deleting.
	DevClusterDockerLoadBalancerDeletingReason = clusterv1.DeletingReason
)

// DevClusterSpec defines the desired state of the DevCluster infrastructure.
type DevClusterSpec struct {
	// controlPlaneEndpoint represents the endpoint used to communicate with the control plane.
	// +optional
	ControlPlaneEndpoint APIEndpoint `json:"controlPlaneEndpoint,omitempty,omitzero"`

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

// DockerClusterBackendSpec defines a backend for a DevCluster using docker containers.
type DockerClusterBackendSpec struct {
	// failureDomains are usually not defined in the spec.
	// The docker provider is special since failure domains don't mean anything in a local docker environment.
	// Instead, the docker cluster controller will simply copy these into the Status and allow the Cluster API
	// controllers to do what they will with the defined failure domains.
	// +optional
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	FailureDomains []clusterv1.FailureDomain `json:"failureDomains,omitempty"`

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

	// initialization provides observations of the DevCluster initialization process.
	// NOTE: Fields in this struct are part of the Cluster API contract and are used to orchestrate initial Cluster provisioning.
	// +optional
	Initialization DevClusterInitializationStatus `json:"initialization,omitempty,omitzero"`

	// failureDomains is a list of failure domain objects synced from the infrastructure provider.
	// It don't mean much in CAPD since it's all local, but we can see how the rest of cluster API
	// will use this if we populate it.
	// +optional
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	FailureDomains []clusterv1.FailureDomain `json:"failureDomains,omitempty"`

	// deprecated groups all the status fields that are deprecated and will be removed when all the nested field are removed.
	// +optional
	Deprecated *DevClusterDeprecatedStatus `json:"deprecated,omitempty"`
}

// DevClusterInitializationStatus provides observations of the DevCluster initialization process.
// +kubebuilder:validation:MinProperties=1
type DevClusterInitializationStatus struct {
	// provisioned is true when the infrastructure provider reports that the Cluster's infrastructure is fully provisioned.
	// NOTE: this field is part of the Cluster API contract, and it is used to orchestrate initial Cluster provisioning.
	// +optional
	Provisioned *bool `json:"provisioned,omitempty"`
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

// APIEndpoint represents a reachable Kubernetes API endpoint.
// +kubebuilder:validation:MinProperties=1
type APIEndpoint struct {
	// host is the hostname on which the API server is serving.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	Host string `json:"host,omitempty"`

	// port is the port on which the API server is serving.
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port,omitempty"`
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
