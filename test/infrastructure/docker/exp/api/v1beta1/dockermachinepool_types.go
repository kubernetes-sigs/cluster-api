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
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1"
)

const (
	// MachinePoolFinalizer allows ReconcileDockerMachinePool to clean up resources.
	MachinePoolFinalizer = "dockermachinepool.infrastructure.cluster.x-k8s.io"
)

// DockerMachinePoolMachineTemplate defines the desired state of DockerMachine.
type DockerMachinePoolMachineTemplate struct {
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
	ExtraMounts []infrav1.Mount `json:"extraMounts,omitempty"`
}

// DockerMachinePoolSpec defines the desired state of DockerMachinePool.
type DockerMachinePoolSpec struct {
	// Template contains the details used to build a replica machine within the Machine Pool
	// +optional
	Template DockerMachinePoolMachineTemplate `json:"template"`

	// ProviderID is the identification ID of the Machine Pool
	// +optional
	ProviderID string `json:"providerID,omitempty"`

	// ProviderIDList is the list of identification IDs of machine instances managed by this Machine Pool
	//+optional
	ProviderIDList []string `json:"providerIDList,omitempty"`
}

// DockerMachinePoolStatus defines the observed state of DockerMachinePool.
type DockerMachinePoolStatus struct {
	// Ready denotes that the machine pool is ready
	// +optional
	Ready bool `json:"ready"`

	// Replicas is the most recently observed number of replicas.
	// +optional
	Replicas int32 `json:"replicas"`

	// The generation observed by the deployment controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Instances contains the status for each instance in the pool
	// +optional
	Instances []DockerMachinePoolInstanceStatus `json:"instances,omitempty"`

	// Conditions defines current service state of the DockerMachinePool.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`

	// InfrastructureMachineKind is the kind of the infrastructure resources behind MachinePool Machines.
	// +optional
	InfrastructureMachineKind string `json:"infrastructureMachineKind,omitempty"`
}

// DockerMachinePoolInstanceStatus contains status information about a DockerMachinePool.
type DockerMachinePoolInstanceStatus struct {
	// Addresses contains the associated addresses for the docker machine.
	// +optional
	Addresses []clusterv1.MachineAddress `json:"addresses,omitempty"`

	// InstanceName is the identification of the Machine Instance within the Machine Pool
	InstanceName string `json:"instanceName,omitempty"`

	// ProviderID is the provider identification of the Machine Pool Instance
	// +optional
	ProviderID *string `json:"providerID,omitempty"`

	// Version defines the Kubernetes version for the Machine Instance
	// +optional
	Version *string `json:"version,omitempty"`

	// Ready denotes that the machine (docker container) is ready
	// +optional
	Ready bool `json:"ready"`

	// Bootstrapped is true when the kubeadm bootstrapping has been run
	// against this machine
	//
	// Deprecated: This field will be removed in the next apiVersion.
	// When removing also remove from staticcheck exclude-rules for SA1019 in golangci.yml
	// +optional
	Bootstrapped bool `json:"bootstrapped,omitempty"`
}

// +kubebuilder:resource:path=dockermachinepools,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of DockerMachinePool"

// DockerMachinePool is the Schema for the dockermachinepools API.
type DockerMachinePool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DockerMachinePoolSpec   `json:"spec,omitempty"`
	Status DockerMachinePoolStatus `json:"status,omitempty"`
}

// GetConditions returns the set of conditions for this object.
func (d *DockerMachinePool) GetConditions() clusterv1.Conditions {
	return d.Status.Conditions
}

// SetConditions sets the conditions on this object.
func (d *DockerMachinePool) SetConditions(conditions clusterv1.Conditions) {
	d.Status.Conditions = conditions
}

// +kubebuilder:object:root=true

// DockerMachinePoolList contains a list of DockerMachinePool.
type DockerMachinePoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DockerMachinePool `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &DockerMachinePool{}, &DockerMachinePoolList{})
}
