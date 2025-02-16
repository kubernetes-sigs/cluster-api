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

const (
	// MachineFinalizer allows ReconcileDockerMachine to clean up resources associated with DockerMachine before
	// removing it from the apiserver.
	MachineFinalizer = "dockermachine.infrastructure.cluster.x-k8s.io"
)

// DockerMachineSpec defines the desired state of DockerMachine.
type DockerMachineSpec struct {
	// ProviderID will be the container name in ProviderID format (docker:////<containername>)
	// +optional
	ProviderID *string `json:"providerID,omitempty"`

	// CustomImage allows customizing the container image that is used for
	// running the machine
	// +optional
	CustomImage string `json:"customImage,omitempty"`

	// PreLoadImages allows to pre-load images in a newly created machine. This can be used to
	// speed up tests by avoiding e.g. to download CNI images on all the containers.
	// +optional
	PreLoadImages []string `json:"preLoadImages,omitempty"`

	// ExtraMounts describes additional mount points for the node container
	// These may be used to bind a hostPath
	// +optional
	ExtraMounts []Mount `json:"extraMounts,omitempty"`

	// Bootstrapped is true when the kubeadm bootstrapping has been run
	// against this machine
	//
	// Deprecated: This field will be removed in the next apiVersion.
	// When removing also remove from staticcheck exclude-rules for SA1019 in golangci.yml.
	// +optional
	Bootstrapped bool `json:"bootstrapped,omitempty"`

	// BootstrapTimeout is the total amount of time to wait for the machine to bootstrap before timing out.
	// The default value is 3m.
	// +optional
	BootstrapTimeout *metav1.Duration `json:"bootstrapTimeout,omitempty"`
}

// Mount specifies a host volume to mount into a container.
// This is a simplified version of kind v1alpha4.Mount types.
type Mount struct {
	// Path of the mount within the container.
	ContainerPath string `json:"containerPath,omitempty"`

	// Path of the mount on the host. If the hostPath doesn't exist, then runtimes
	// should report error. If the hostpath is a symbolic link, runtimes should
	// follow the symlink and mount the real destination to container.
	HostPath string `json:"hostPath,omitempty"`

	// If set, the mount is read-only.
	// +optional
	Readonly bool `json:"readOnly,omitempty"`
}

// DockerMachineStatus defines the observed state of DockerMachine.
type DockerMachineStatus struct {
	// Ready denotes that the machine (docker container) is ready
	// +optional
	Ready bool `json:"ready"`

	// LoadBalancerConfigured denotes that the machine has been
	// added to the load balancer
	// +optional
	LoadBalancerConfigured bool `json:"loadBalancerConfigured"`

	// Addresses contains the associated addresses for the docker machine.
	// +optional
	Addresses []clusterv1.MachineAddress `json:"addresses,omitempty"`

	// Conditions defines current service state of the DockerMachine.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`

	// v1beta2 groups all the fields that will be added or modified in DockerMachine's status with the V1Beta2 version.
	// +optional
	V1Beta2 *DockerMachineV1Beta2Status `json:"v1beta2,omitempty"`
}

// DockerMachineV1Beta2Status groups all the fields that will be added or modified in DockerMachine with the V1Beta2 version.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type DockerMachineV1Beta2Status struct {
	// conditions represents the observations of a DockerMachine's current state.
	// Known condition types are Ready, ContainerProvisioned, BootstrapExecSucceeded, Deleting, Paused.
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=32
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:resource:path=dockermachines,scope=Namespaced,categories=cluster-api
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".metadata.labels['cluster\\.x-k8s\\.io/cluster-name']",description="Cluster"
// +kubebuilder:printcolumn:name="Machine",type="string",JSONPath=".metadata.ownerReferences[?(@.kind==\"Machine\")].name",description="Machine object which owns with this DockerMachine"
// +kubebuilder:printcolumn:name="ProviderID",type="string",JSONPath=".spec.providerID",description="Provider ID"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.ready",description="Machine ready status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of DockerMachine"

// DockerMachine is the Schema for the dockermachines API.
type DockerMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DockerMachineSpec   `json:"spec,omitempty"`
	Status DockerMachineStatus `json:"status,omitempty"`
}

// GetConditions returns the set of conditions for this object.
func (c *DockerMachine) GetConditions() clusterv1.Conditions {
	return c.Status.Conditions
}

// SetConditions sets the conditions on this object.
func (c *DockerMachine) SetConditions(conditions clusterv1.Conditions) {
	c.Status.Conditions = conditions
}

// GetV1Beta2Conditions returns the set of conditions for this object.
func (c *DockerMachine) GetV1Beta2Conditions() []metav1.Condition {
	if c.Status.V1Beta2 == nil {
		return nil
	}
	return c.Status.V1Beta2.Conditions
}

// SetV1Beta2Conditions sets conditions for an API object.
func (c *DockerMachine) SetV1Beta2Conditions(conditions []metav1.Condition) {
	if c.Status.V1Beta2 == nil {
		c.Status.V1Beta2 = &DockerMachineV1Beta2Status{}
	}
	c.Status.V1Beta2.Conditions = conditions
}

// +kubebuilder:object:root=true

// DockerMachineList contains a list of DockerMachine.
type DockerMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DockerMachine `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &DockerMachine{}, &DockerMachineList{})
}
