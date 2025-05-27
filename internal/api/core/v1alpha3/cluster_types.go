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

package v1alpha3

import (
	"fmt"
	"net"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	capierrors "sigs.k8s.io/cluster-api/errors"
)

const (
	// ClusterFinalizer is the finalizer used by the cluster controller to
	// cleanup the cluster resources when a Cluster is being deleted.
	ClusterFinalizer = "cluster.cluster.x-k8s.io"
)

// ANCHOR: ClusterSpec

// ClusterSpec defines the desired state of Cluster.
type ClusterSpec struct {
	// paused can be used to prevent controllers from processing the Cluster and all its associated objects.
	// +optional
	Paused bool `json:"paused,omitempty"`

	// clusterNetwork is the cluster network configuration.
	// +optional
	ClusterNetwork *ClusterNetwork `json:"clusterNetwork,omitempty"`

	// controlPlaneEndpoint represents the endpoint used to communicate with the control plane.
	// +optional
	ControlPlaneEndpoint APIEndpoint `json:"controlPlaneEndpoint"`

	// controlPlaneRef is an optional reference to a provider-specific resource that holds
	// the details for provisioning the Control Plane for a Cluster.
	// +optional
	ControlPlaneRef *corev1.ObjectReference `json:"controlPlaneRef,omitempty"`

	// infrastructureRef is a reference to a provider-specific resource that holds the details
	// for provisioning infrastructure for a cluster in said provider.
	// +optional
	InfrastructureRef *corev1.ObjectReference `json:"infrastructureRef,omitempty"`
}

// ANCHOR_END: ClusterSpec

// ANCHOR: ClusterNetwork

// ClusterNetwork specifies the different networking
// parameters for a cluster.
type ClusterNetwork struct {
	// apiServerPort specifies the port the API Server should bind to.
	// Defaults to 6443.
	// +optional
	APIServerPort *int32 `json:"apiServerPort,omitempty"`

	// services is the network ranges from which service VIPs are allocated.
	// +optional
	Services *NetworkRanges `json:"services,omitempty"`

	// pods is the network ranges from which Pod networks are allocated.
	// +optional
	Pods *NetworkRanges `json:"pods,omitempty"`

	// serviceDomain is the domain name for services.
	// +optional
	ServiceDomain string `json:"serviceDomain,omitempty"`
}

// ANCHOR_END: ClusterNetwork

// ANCHOR: NetworkRanges

// NetworkRanges represents ranges of network addresses.
type NetworkRanges struct {
	// cidrBlocks is a list of CIDR blocks.
	CIDRBlocks []string `json:"cidrBlocks"`
}

func (n *NetworkRanges) String() string {
	if n == nil {
		return ""
	}
	return strings.Join(n.CIDRBlocks, ",")
}

// ANCHOR_END: NetworkRanges

// ANCHOR: ClusterStatus

// ClusterStatus defines the observed state of Cluster.
type ClusterStatus struct {
	// failureDomains is a slice of failure domain objects synced from the infrastructure provider.
	FailureDomains FailureDomains `json:"failureDomains,omitempty"`

	// failureReason indicates that there is a fatal problem reconciling the
	// state, and will be set to a token value suitable for
	// programmatic interpretation.
	// +optional
	FailureReason *capierrors.ClusterStatusError `json:"failureReason,omitempty"`

	// failureMessage indicates that there is a fatal problem reconciling the
	// state, and will be set to a descriptive error message.
	// +optional
	FailureMessage *string `json:"failureMessage,omitempty"`

	// phase represents the current phase of cluster actuation.
	// E.g. Pending, Running, Terminating, Failed etc.
	// +optional
	Phase string `json:"phase,omitempty"`

	// infrastructureReady is the state of the infrastructure provider.
	// +optional
	InfrastructureReady bool `json:"infrastructureReady"`

	// controlPlaneInitialized defines if the control plane has been initialized.
	// +optional
	ControlPlaneInitialized bool `json:"controlPlaneInitialized"`

	// controlPlaneReady defines if the control plane is ready.
	// +optional
	ControlPlaneReady bool `json:"controlPlaneReady,omitempty"`

	// conditions defines current service state of the cluster.
	// +optional
	Conditions Conditions `json:"conditions,omitempty"`

	// observedGeneration is the latest generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// ANCHOR_END: ClusterStatus

// SetTypedPhase sets the Phase field to the string representation of ClusterPhase.
func (c *ClusterStatus) SetTypedPhase(p ClusterPhase) {
	c.Phase = string(p)
}

// GetTypedPhase attempts to parse the Phase field and return
// the typed ClusterPhase representation as described in `machine_phase_types.go`.
func (c *ClusterStatus) GetTypedPhase() ClusterPhase {
	switch phase := ClusterPhase(c.Phase); phase {
	case
		ClusterPhasePending,
		ClusterPhaseProvisioning,
		ClusterPhaseProvisioned,
		ClusterPhaseDeleting,
		ClusterPhaseFailed:
		return phase
	default:
		return ClusterPhaseUnknown
	}
}

// ANCHOR: APIEndpoint

// APIEndpoint represents a reachable Kubernetes API endpoint.
type APIEndpoint struct {
	// host is the hostname on which the API server is serving.
	Host string `json:"host"`

	// port is the port on which the API server is serving.
	Port int32 `json:"port"`
}

// IsZero returns true if both host and port are zero values.
func (v APIEndpoint) IsZero() bool {
	return v.Host == "" && v.Port == 0
}

// IsValid returns true if both host and port are non-zero values.
func (v APIEndpoint) IsValid() bool {
	return v.Host != "" && v.Port != 0
}

// String returns a formatted version HOST:PORT of this APIEndpoint.
func (v APIEndpoint) String() string {
	return net.JoinHostPort(v.Host, fmt.Sprintf("%d", v.Port))
}

// ANCHOR_END: APIEndpoint

// +kubebuilder:object:root=true
// +kubebuilder:unservedversion
// +kubebuilder:deprecatedversion
// +kubebuilder:resource:path=clusters,shortName=cl,scope=Namespaced,categories=cluster-api
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase",description="Cluster status such as Pending/Provisioning/Provisioned/Deleting/Failed"

// Cluster is the Schema for the clusters API.
type Cluster struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec is the desired state of Cluster.
	Spec ClusterSpec `json:"spec,omitempty"`
	// status is the observed state of Cluster.
	Status ClusterStatus `json:"status,omitempty"`
}

// GetConditions returns the set of conditions for this object.
func (c *Cluster) GetConditions() Conditions {
	return c.Status.Conditions
}

// SetConditions sets the conditions on this object.
func (c *Cluster) SetConditions(conditions Conditions) {
	c.Status.Conditions = conditions
}

// +kubebuilder:object:root=true

// ClusterList contains a list of Cluster.
type ClusterList struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard list's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#lists-and-simple-kinds
	metav1.ListMeta `json:"metadata,omitempty"`
	// items is the list of Clusters.
	Items []Cluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Cluster{}, &ClusterList{})
}

// FailureDomains is a slice of FailureDomains.
type FailureDomains map[string]FailureDomainSpec

// FilterControlPlane returns a FailureDomain slice containing only the domains suitable to be used
// for control plane nodes.
func (in FailureDomains) FilterControlPlane() FailureDomains {
	res := make(FailureDomains)
	for id, spec := range in {
		if spec.ControlPlane {
			res[id] = spec
		}
	}
	return res
}

// GetIDs returns a slice containing the ids for failure domains.
func (in FailureDomains) GetIDs() []*string {
	ids := make([]*string, 0, len(in))
	for id := range in {
		ids = append(ids, ptr.To(id))
	}
	return ids
}

// FailureDomainSpec is the Schema for Cluster API failure domains.
// It allows controllers to understand how many failure domains a cluster can optionally span across.
type FailureDomainSpec struct {
	// controlPlane determines if this failure domain is suitable for use by control plane machines.
	// +optional
	ControlPlane bool `json:"controlPlane"`

	// attributes is a free form map of attributes an infrastructure provider might use or require.
	// +optional
	Attributes map[string]string `json:"attributes,omitempty"`
}
