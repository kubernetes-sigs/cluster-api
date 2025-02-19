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
	// VMProvisionedCondition documents the status of the provisioning VM implementing the InMemoryMachine.
	VMProvisionedCondition clusterv1.ConditionType = "VMProvisioned"

	// WaitingForClusterInfrastructureReason (Severity=Info) documents an InMemoryMachine VM waiting for the cluster
	// infrastructure to be ready.
	// WaitingForClusterInfrastructureReason = "WaitingForClusterInfrastructure".

	// WaitingControlPlaneInitializedReason (Severity=Info) documents an InMemoryMachine VM waiting
	// for the control plane to be initialized.
	WaitingControlPlaneInitializedReason = "WaitingControlPlaneInitialized"

	// WaitingForBootstrapDataReason (Severity=Info) documents an InMemoryMachine VM waiting for the bootstrap
	// data to be ready before starting to create the CloudMachine/VM.
	// WaitingForBootstrapDataReason = "WaitingForBootstrapData".

	// VMWaitingForStartupTimeoutReason (Severity=Info) documents a InMemoryMachine VM provisioning.
	VMWaitingForStartupTimeoutReason = "WaitingForStartupTimeout"
)

const (
	// NodeProvisionedCondition documents the status of the provisioning of the node hosted on the InMemoryMachine.
	NodeProvisionedCondition clusterv1.ConditionType = "NodeProvisioned"

	// NodeWaitingForStartupTimeoutReason (Severity=Info) documents a InMemoryMachine Node provisioning.
	NodeWaitingForStartupTimeoutReason = "WaitingForStartupTimeout"
)

const (
	// EtcdProvisionedCondition documents the status of the provisioning of the etcd member hosted on the InMemoryMachine.
	EtcdProvisionedCondition clusterv1.ConditionType = "EtcdProvisioned"

	// EtcdWaitingForStartupTimeoutReason (Severity=Info) documents a InMemoryMachine etcd pod provisioning.
	EtcdWaitingForStartupTimeoutReason = "WaitingForStartupTimeout"
)

const (
	// APIServerProvisionedCondition documents the status of the provisioning of the APIServer instance hosted on the InMemoryMachine.
	APIServerProvisionedCondition clusterv1.ConditionType = "APIServerProvisioned"

	// APIServerWaitingForStartupTimeoutReason (Severity=Info) documents a InMemoryMachine API server pod provisioning.
	APIServerWaitingForStartupTimeoutReason = "WaitingForStartupTimeout"
)

// DevMachineSpec defines the desired state of DevMachine.
type DevMachineSpec struct {
	// providerID used to link this machine with the node hosted on it.
	// +optional
	ProviderID *string `json:"providerID,omitempty"`

	// backend defines backends for a DevMachine.
	// +required
	Backend DevMachineBackendSpec `json:"backend"`
}

// DevMachineBackendSpec defines backends for a DevMachine.
type DevMachineBackendSpec struct {
	// docker defines a backend for a DevMachine using docker containers.
	// +optional
	Docker *DockerMachineBackendSpec `json:"docker,omitempty"`

	// inMemory defines a backend for a DevMachine that runs in memory.
	// +optional
	InMemory *InMemoryMachineBackendSpec `json:"inMemory,omitempty"`
}

// DockerMachineBackendSpec defines a backend for a DevMachine using docker containers.
type DockerMachineBackendSpec struct {
	// customImage allows customizing the container image that is used for
	// running the machine
	// +optional
	CustomImage string `json:"customImage,omitempty"`

	// preLoadImages allows to pre-load images in a newly created machine. This can be used to
	// speed up tests by avoiding e.g. to download CNI images on all the containers.
	// +optional
	PreLoadImages []string `json:"preLoadImages,omitempty"`

	// extraMounts describes additional mount points for the node container
	// These may be used to bind a hostPath
	// +optional
	ExtraMounts []Mount `json:"extraMounts,omitempty"`

	// bootstrapped is true when the kubeadm bootstrapping has been run
	// against this machine
	//
	// Deprecated: This field will be removed in the next apiVersion.
	// When removing also remove from staticcheck exclude-rules for SA1019 in golangci.yml.
	// +optional
	Bootstrapped bool `json:"bootstrapped,omitempty"`

	// bootstrapTimeout is the total amount of time to wait for the machine to bootstrap before timing out.
	// The default value is 3m.
	// +optional
	BootstrapTimeout *metav1.Duration `json:"bootstrapTimeout,omitempty"`
}

// InMemoryMachineBackendSpec defines a backend for a DevMachine that runs in memory.
type InMemoryMachineBackendSpec struct {
	// vm defines the behaviour of the VM implementing the InMemoryMachine.
	VM *InMemoryVMSpec `json:"vm,omitempty"`

	// node defines the behaviour of the Node (the kubelet) hosted on the InMemoryMachine.
	Node *InMemoryNodeSpec `json:"node,omitempty"`

	// apiServer defines the behaviour of the APIServer hosted on the InMemoryMachine.
	APIServer *InMemoryAPIServerSpec `json:"apiServer,omitempty"`

	// etcd defines the behaviour of the etcd member hosted on the InMemoryMachine.
	Etcd *InMemoryEtcdSpec `json:"etcd,omitempty"`
}

// InMemoryVMSpec defines the behaviour of the VM implementing the InMemoryMachine.
type InMemoryVMSpec struct {
	// provisioning defines variables influencing how the VM implementing the InMemoryMachine is going to be provisioned.
	// NOTE: VM provisioning includes all the steps from creation to power-on.
	Provisioning CommonProvisioningSettings `json:"provisioning,omitempty"`
}

// InMemoryNodeSpec defines the behaviour of the Node (the kubelet) hosted on the InMemoryMachine.
type InMemoryNodeSpec struct {
	// provisioning defines variables influencing how the Node (the kubelet) hosted on the InMemoryMachine is going to be provisioned.
	// NOTE: Node provisioning includes all the steps from starting kubelet to the node become ready, get a provider ID, and being registered in K8s.
	Provisioning CommonProvisioningSettings `json:"provisioning,omitempty"`
}

// InMemoryAPIServerSpec defines the behaviour of the APIServer hosted on the InMemoryMachine.
type InMemoryAPIServerSpec struct {
	// provisioning defines variables influencing how the APIServer hosted on the InMemoryMachine is going to be provisioned.
	// NOTE: APIServer provisioning includes all the steps from starting the static Pod to the Pod become ready and being registered in K8s.
	Provisioning CommonProvisioningSettings `json:"provisioning,omitempty"`
}

// InMemoryEtcdSpec defines the behaviour of the etcd member hosted on the InMemoryMachine.
type InMemoryEtcdSpec struct {
	// provisioning defines variables influencing how the etcd member hosted on the InMemoryMachine is going to be provisioned.
	// NOTE: Etcd provisioning includes all the steps from starting the static Pod to the Pod become ready and being registered in K8s.
	Provisioning CommonProvisioningSettings `json:"provisioning,omitempty"`
}

// CommonProvisioningSettings holds parameters that applies to provisioning of most of the objects.
type CommonProvisioningSettings struct {
	// startupDuration defines the duration of the object provisioning phase.
	StartupDuration metav1.Duration `json:"startupDuration"`

	// startupJitter adds some randomness on StartupDuration; the actual duration will be StartupDuration plus an additional
	// amount chosen uniformly at random from the interval between zero and `StartupJitter*StartupDuration`.
	// NOTE: this is modeled as string because the usage of float is highly discouraged, as support for them varies across languages.
	StartupJitter string `json:"startupJitter,omitempty"`
}

// DevMachineStatus defines the observed state of DevMachine.
type DevMachineStatus struct {
	// ready denotes that the machine is ready
	// +optional
	Ready bool `json:"ready"`

	// addresses contains the associated addresses for the dev machine.
	// +optional
	Addresses []clusterv1.MachineAddress `json:"addresses,omitempty"`

	// conditions defines current service state of the DevMachine.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`

	// v1beta2 groups all the fields that will be added or modified in DevMachine's status with the V1Beta2 version.
	// +optional
	V1Beta2 *DevMachineV1Beta2Status `json:"v1beta2,omitempty"`

	// backend defines backends status for a DevMachine.
	// +optional
	Backend *DevMachineBackendStatus `json:"backend,omitempty"`
}

// DevMachineBackendStatus define backend status for a DevMachine.
type DevMachineBackendStatus struct {
	// docker define backend status for a DevMachine for a machine using docker containers.
	// +optional
	Docker *DockerMachineBackendStatus `json:"docker,omitempty"`
}

// DockerMachineBackendStatus define backend status for a DevMachine for a machine using docker containers.
type DockerMachineBackendStatus struct {
	// loadBalancerConfigured denotes that the machine has been
	// added to the load balancer
	// +optional
	LoadBalancerConfigured bool `json:"loadBalancerConfigured"`
}

// DevMachineV1Beta2Status groups all the fields that will be added or modified in DevMachine with the V1Beta2 version.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type DevMachineV1Beta2Status struct {
	// conditions represents the observations of a DevMachine's current state.
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=32
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:resource:path=devmachines,scope=Namespaced,categories=cluster-api
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".metadata.labels['cluster\\.x-k8s\\.io/cluster-name']",description="Cluster"
// +kubebuilder:printcolumn:name="Machine",type="string",JSONPath=".metadata.ownerReferences[?(@.kind==\"Machine\")].name",description="Machine object which owns with this DevMachine"
// +kubebuilder:printcolumn:name="ProviderID",type="string",JSONPath=".spec.providerID",description="Provider ID"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.ready",description="Machine ready status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of the DevMachine"

// DevMachine is the schema for the dev machine infrastructure API.
type DevMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DevMachineSpec   `json:"spec,omitempty"`
	Status DevMachineStatus `json:"status,omitempty"`
}

// GetConditions returns the set of conditions for this object.
func (c *DevMachine) GetConditions() clusterv1.Conditions {
	return c.Status.Conditions
}

// SetConditions sets the conditions on this object.
func (c *DevMachine) SetConditions(conditions clusterv1.Conditions) {
	c.Status.Conditions = conditions
}

// GetV1Beta2Conditions returns the set of conditions for this object.
func (c *DevMachine) GetV1Beta2Conditions() []metav1.Condition {
	if c.Status.V1Beta2 == nil {
		return nil
	}
	return c.Status.V1Beta2.Conditions
}

// SetV1Beta2Conditions sets conditions for an API object.
func (c *DevMachine) SetV1Beta2Conditions(conditions []metav1.Condition) {
	if c.Status.V1Beta2 == nil {
		c.Status.V1Beta2 = &DevMachineV1Beta2Status{}
	}
	c.Status.V1Beta2.Conditions = conditions
}

// +kubebuilder:object:root=true

// DevMachineList contains a list of DevMachine.
type DevMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DevMachine `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &DevMachine{}, &DevMachineList{})
}
