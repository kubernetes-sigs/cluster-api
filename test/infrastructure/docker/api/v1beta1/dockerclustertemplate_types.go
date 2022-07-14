/*
Copyright 2021 The Kubernetes Authors.

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

// DockerClusterTemplateSpec defines the desired state of DockerClusterTemplate.
type DockerClusterTemplateSpec struct {
	Template DockerClusterTemplateResource `json:"template"`
}

// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of DockerClusterTemplate"

// DockerClusterTemplate is the Schema for the dockerclustertemplates API.
type DockerClusterTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec DockerClusterTemplateSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=dockerclustertemplates,scope=Namespaced,categories=cluster-api

// DockerClusterTemplateList contains a list of DockerClusterTemplate.
type DockerClusterTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DockerClusterTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DockerClusterTemplate{}, &DockerClusterTemplateList{})
}

// DockerClusterTemplateResource describes the data needed to create a DockerCluster from a template.
type DockerClusterTemplateResource struct {
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	ObjectMeta clusterv1.ObjectMeta `json:"metadata,omitempty"`

	Spec DockerClusterTemplateNewSpec `json:"spec"`
}

// DockerClusterTemplateNewSpec defines the desired state of DockerCluster.
type DockerClusterTemplateNewSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// ControlPlaneEndpoint represents the endpoint used to communicate with the control plane.
	// +optional
	ControlPlaneEndpoint APIEndpoint `json:"controlPlaneEndpoint"`

	// FailureDomains are usually not defined in the spec.
	// The docker provider is special since failure domains don't mean anything in a local docker environment.
	// Instead, the docker cluster controller will simply copy these into the Status and allow the Cluster API
	// controllers to do what they will with the defined failure domains.
	// +optional
	FailureDomains clusterv1.FailureDomains `json:"failureDomains,omitempty"`

	// LoadBalancer allows defining configurations for the cluster load balancer.
	// +optional
	LoadBalancer DockerLoadBalancer `json:"loadBalancer,omitempty"`

	Subnets1 DockerClusterTemplateSubnets1 `json:"subnets1,omitempty"`
	Subnets2 DockerClusterTemplateSubnets2 `json:"subnets2,omitempty"`
	Subnets3 DockerClusterTemplateSubnets3 `json:"subnets3,omitempty"`
	Subnets4 DockerClusterTemplateSubnets4 `json:"subnets4,omitempty"`
}

// DockerClusterTemplateSubnets1 .
// +listType=map
// +listMapKey=uuid
type DockerClusterTemplateSubnets1 []DockerClusterTemplateSubnets1Spec

// DockerClusterTemplateSubnets1Spec .
type DockerClusterTemplateSubnets1Spec struct {
	// +kubebuilder:default="dummy"
	UUID *string `json:"uuid,omitempty"`

	// ID defines a unique identifier to reference this resource.
	ID string `json:"id,omitempty"`

	TopologyField      string `json:"topologyField,omitempty"`
	DockerClusterField string `json:"dockerClusterField,omitempty"`
}

// DockerClusterTemplateSubnets2 .
type DockerClusterTemplateSubnets2 []DockerClusterTemplateSubnets2Spec

// DockerClusterTemplateSubnets2Spec .
type DockerClusterTemplateSubnets2Spec struct {
	// ID defines a unique identifier to reference this resource.
	ID string `json:"id,omitempty"`

	TopologyField      string `json:"topologyField,omitempty"`
	DockerClusterField string `json:"dockerClusterField,omitempty"`
}

// DockerClusterTemplateSubnets3 .
// +listType=map
// +listMapKey=uuid
type DockerClusterTemplateSubnets3 []DockerClusterTemplateSubnets3Spec

// DockerClusterTemplateSubnets3Spec .
type DockerClusterTemplateSubnets3Spec struct {
	UUID string `json:"uuid"`

	// ID defines a unique identifier to reference this resource.
	ID string `json:"id,omitempty"`

	TopologyField      string `json:"topologyField,omitempty"`
	DockerClusterField string `json:"dockerClusterField,omitempty"`
}

// DockerClusterTemplateSubnets4 .
type DockerClusterTemplateSubnets4 []DockerClusterTemplateSubnets4Spec

// DockerClusterTemplateSubnets4Spec .
type DockerClusterTemplateSubnets4Spec struct {
	// ID defines a unique identifier to reference this resource.
	ID string `json:"id,omitempty"`

	TopologyField      string `json:"topologyField,omitempty"`
	DockerClusterField string `json:"dockerClusterField,omitempty"`
}
