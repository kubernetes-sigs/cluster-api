/*
Copyright 2018 The Kubernetes Authors.

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
	"log"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/endpoints/request"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"sigs.k8s.io/cluster-api/pkg/apis/cluster"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/common"
)

const ClusterFinalizer string = "cluster.cluster.k8s.io"

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Cluster
// +k8s:openapi-gen=true
// +resource:path=clusters,strategy=ClusterStrategy
type Cluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterSpec   `json:"spec,omitempty"`
	Status ClusterStatus `json:"status,omitempty"`
}

// ClusterSpec defines the desired state of Cluster
type ClusterSpec struct {
	// Cluster network configuration
	ClusterNetwork ClusterNetworkingConfig `json:"clusterNetwork"`

	// Provider-specific serialized configuration to use during
	// cluster creation. It is recommended that providers maintain
	// their own versioned API types that should be
	// serialized/deserialized from this field.
	// +optional
	ProviderConfig ProviderConfig `json:"providerConfig"`
}

// ClusterNetworkingConfig specifies the different networking
// parameters for a cluster.
type ClusterNetworkingConfig struct {
	// The network ranges from which service VIPs are allocated.
	Services NetworkRanges `json:"services"`

	// The network ranges from which POD networks are allocated.
	Pods NetworkRanges `json:"pods"`

	// Domain name for services.
	ServiceDomain string `json:"serviceDomain"`
}

// NetworkRanges represents ranges of network addresses.
type NetworkRanges struct {
	CIDRBlocks []string `json:"cidrBlocks"`
}

// ClusterStatus defines the observed state of Cluster
type ClusterStatus struct {
	// APIEndpoint represents the endpoint to communicate with the IP.
	APIEndpoints []APIEndpoint `json:"apiEndpoints"`

	// NB: Eventually we will redefine ErrorReason as ClusterStatusError once the
	// following issue is fixed.
	// https://github.com/kubernetes-incubator/apiserver-builder/issues/176

	// If set, indicates that there is a problem reconciling the
	// state, and will be set to a token value suitable for
	// programmatic interpretation.
	ErrorReason common.ClusterStatusError `json:"errorReason"`

	// If set, indicates that there is a problem reconciling the
	// state, and will be set to a descriptive error message.
	ErrorMessage string `json:"errorMessage"`

	// Provider-specific status.
	// It is recommended that providers maintain their
	// own versioned API types that should be
	// serialized/deserialized from this field.
	ProviderStatus *runtime.RawExtension `json:"providerStatus"`
}

// APIEndpoint represents a reachable Kubernetes API endpoint.
type APIEndpoint struct {
	// The hostname on which the API server is serving.
	Host string `json:"host"`

	// The port on which the API server is serving.
	Port int `json:"port"`
}

// Validate checks that an instance of Cluster is well formed
func (ClusterStrategy) Validate(ctx request.Context, obj runtime.Object) field.ErrorList {
	o := obj.(*cluster.Cluster)
	log.Printf("Validating fields for Cluster %s\n", o.Name)
	errors := field.ErrorList{}
	// perform validation here and add to errors using field.Invalid
	if o.Spec.ClusterNetwork.ServiceDomain == "" {
		errors = append(errors, field.Invalid(
			field.NewPath("Spec", "ClusterNetwork", "ServiceDomain"),
			o.Spec.ClusterNetwork.ServiceDomain,
			"invalid cluster configuration: missing Cluster.Spec.ClusterNetwork.ServiceDomain"))
	}
	if len(o.Spec.ClusterNetwork.Pods.CIDRBlocks) == 0 {
		errors = append(errors, field.Invalid(
			field.NewPath("Spec", "ClusterNetwork", "Pods"),
			o.Spec.ClusterNetwork.Pods,
			"invalid cluster configuration: missing Cluster.Spec.ClusterNetwork.Pods"))
	}
	if len(o.Spec.ClusterNetwork.Services.CIDRBlocks) == 0 {
		errors = append(errors, field.Invalid(
			field.NewPath("Spec", "ClusterNetwork", "Services"),
			o.Spec.ClusterNetwork.Services,
			"invalid cluster configuration: missing Cluster.Spec.ClusterNetwork.Services"))
	}
	return errors
}

// PrepareForCreate clears fields that are not allowed to be set by end users on creation.
func (c ClusterStrategy) PrepareForCreate(ctx request.Context, obj runtime.Object) {
	// Invoke the parent implementation to strip the Status
	c.DefaultStorageStrategy.PrepareForCreate(ctx, obj)

	// Cast the element and set finalizer
	o := obj.(*cluster.Cluster)
	o.ObjectMeta.Finalizers = append(o.ObjectMeta.Finalizers, ClusterFinalizer)
}

// DefaultingFunction sets default Cluster field values
func (ClusterSchemeFns) DefaultingFunction(o interface{}) {
	obj := o.(*Cluster)
	// set default field values here
	log.Printf("Defaulting fields for Cluster %s\n", obj.Name)
}
