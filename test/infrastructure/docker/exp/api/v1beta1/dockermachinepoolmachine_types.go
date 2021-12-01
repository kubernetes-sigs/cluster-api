/*
Copyright 2022 The Kubernetes Authors.

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
	// MachinePoolMachineFinalizer is used to ensure deletion of dependencies (nodes, infra).
	MachinePoolMachineFinalizer = "dockermachinepoolmachine.infrastructure.cluster.x-k8s.io"
)

type (

	// DockerMachinePoolMachineSpec defines the desired state of DockerMachinePoolMachine.
	DockerMachinePoolMachineSpec struct {
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
		// +optional
		Bootstrapped bool `json:"bootstrapped,omitempty"`
	}

	// Mount specifies a host volume to mount into a container.
	// This is a simplified version of kind v1alpha4.Mount types.
	Mount struct {
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

	// DockerMachinePoolMachineStatus defines the observed state of DockerMachinePoolMachine.
	DockerMachinePoolMachineStatus struct {
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
	}

	// +kubebuilder:object:root=true
	// +kubebuilder:subresource:status
	// +kubebuilder:resource:path=dockermachinepoolmachines,scope=Namespaced,categories=cluster-api,shortName=dmpm
	// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.ready",description="Flag indicating infrastructure is successfully provisioned"
	// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".metadata.labels['cluster\\.x-k8s\\.io/cluster-name']",description="Cluster"
	// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of DockerMachine"
	// +kubebuilder:storageversion

	// DockerMachinePoolMachine is the Schema for the dockermachinepoolmachines API.
	DockerMachinePoolMachine struct {
		metav1.TypeMeta   `json:",inline"`
		metav1.ObjectMeta `json:"metadata,omitempty"`

		Spec   DockerMachinePoolMachineSpec   `json:"spec,omitempty"`
		Status DockerMachinePoolMachineStatus `json:"status,omitempty"`
	}

	// +kubebuilder:object:root=true

	// DockerMachinePoolMachineList contains a list of DockerMachinePoolMachines.
	DockerMachinePoolMachineList struct {
		metav1.TypeMeta `json:",inline"`
		metav1.ListMeta `json:"metadata,omitempty"`
		Items           []DockerMachinePoolMachine `json:"items"`
	}
)

// GetConditions returns the list of conditions for a DockerMachinePoolMachine API object.
func (c *DockerMachinePoolMachine) GetConditions() clusterv1.Conditions {
	return c.Status.Conditions
}

// SetConditions will set the given conditions on a DockerMachinePoolMachine object.
func (c *DockerMachinePoolMachine) SetConditions(conditions clusterv1.Conditions) {
	c.Status.Conditions = conditions
}

func init() {
	SchemeBuilder.Register(&DockerMachinePoolMachine{}, &DockerMachinePoolMachineList{})
}
