/*
Copyright 2023 The Kubernetes Authors.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

const (
	// MachineFinalizer allows ReconcileInMemoryMachine to clean up resources associated with InMemoryMachine before
	// removing it from the API server.
	MachineFinalizer = "inmemorymachine.infrastructure.cluster.x-k8s.io"
)

const (
	// VMProvisionedCondition documents the status of the provisioning VM implementing the InMemoryMachine.
	VMProvisionedCondition clusterv1.ConditionType = "VMProvisioned"

	// WaitingForClusterInfrastructureReason (Severity=Info) documents an InMemoryMachine VM waiting for the cluster
	// infrastructure to be ready.
	WaitingForClusterInfrastructureReason = "WaitingForClusterInfrastructure"

	// WaitingControlPlaneInitializedReason (Severity=Info) documents an InMemoryMachine VM waiting
	// for the control plane to be initialized.
	WaitingControlPlaneInitializedReason = "WaitingControlPlaneInitialized"

	// WaitingForBootstrapDataReason (Severity=Info) documents an InMemoryMachine VM waiting for the bootstrap
	// data to be ready before starting to create the CloudMachine/VM.
	WaitingForBootstrapDataReason = "WaitingForBootstrapData"

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

// InMemoryMachineSpec defines the desired state of InMemoryMachine.
type InMemoryMachineSpec struct {
	// ProviderID will be the container name in ProviderID format (in-memory:////<name>)
	// +optional
	ProviderID *string `json:"providerID,omitempty"`

	// Behaviour of the InMemoryMachine; this will allow to make a simulation more alike to real use cases
	// e.g. by defining the duration of the provisioning phase mimicking the performances of the target infrastructure.
	Behaviour *InMemoryMachineBehaviour `json:"behaviour,omitempty"`
}

// InMemoryMachineBehaviour defines the behaviour of the InMemoryMachine.
type InMemoryMachineBehaviour struct {
	// VM defines the behaviour of the VM implementing the InMemoryMachine.
	VM *InMemoryVMBehaviour `json:"vm,omitempty"`

	// Node defines the behaviour of the Node (the kubelet) hosted on the InMemoryMachine.
	Node *InMemoryNodeBehaviour `json:"node,omitempty"`

	// APIServer defines the behaviour of the APIServer hosted on the InMemoryMachine.
	APIServer *InMemoryAPIServerBehaviour `json:"apiServer,omitempty"`

	// Etcd defines the behaviour of the etcd member hosted on the InMemoryMachine.
	Etcd *InMemoryEtcdBehaviour `json:"etcd,omitempty"`
}

// InMemoryVMBehaviour defines the behaviour of the VM implementing the InMemoryMachine.
type InMemoryVMBehaviour struct {
	// Provisioning defines variables influencing how the VM implementing the InMemoryMachine is going to be provisioned.
	// NOTE: VM provisioning includes all the steps from creation to power-on.
	Provisioning CommonProvisioningSettings `json:"provisioning,omitempty"`
}

// InMemoryNodeBehaviour defines the behaviour of the Node (the kubelet) hosted on the InMemoryMachine.
type InMemoryNodeBehaviour struct {
	// Provisioning defines variables influencing how the Node (the kubelet) hosted on the InMemoryMachine is going to be provisioned.
	// NOTE: Node provisioning includes all the steps from starting kubelet to the node become ready, get a provider ID, and being registered in K8s.
	Provisioning CommonProvisioningSettings `json:"provisioning,omitempty"`
}

// InMemoryAPIServerBehaviour defines the behaviour of the APIServer hosted on the InMemoryMachine.
type InMemoryAPIServerBehaviour struct {
	// Provisioning defines variables influencing how the APIServer hosted on the InMemoryMachine is going to be provisioned.
	// NOTE: APIServer provisioning includes all the steps from starting the static Pod to the Pod become ready and being registered in K8s.
	Provisioning CommonProvisioningSettings `json:"provisioning,omitempty"`
}

// InMemoryEtcdBehaviour defines the behaviour of the etcd member hosted on the InMemoryMachine.
type InMemoryEtcdBehaviour struct {
	// Provisioning defines variables influencing how the etcd member hosted on the InMemoryMachine is going to be provisioned.
	// NOTE: Etcd provisioning includes all the steps from starting the static Pod to the Pod become ready and being registered in K8s.
	Provisioning CommonProvisioningSettings `json:"provisioning,omitempty"`
}

// CommonProvisioningSettings holds parameters that applies to provisioning of most of the objects.
type CommonProvisioningSettings struct {
	// StartupDuration defines the duration of the object provisioning phase.
	StartupDuration metav1.Duration `json:"startupDuration"`

	// StartupJitter adds some randomness on StartupDuration; the actual duration will be StartupDuration plus an additional
	// amount chosen uniformly at random from the interval between zero and `StartupJitter*StartupDuration`.
	// NOTE: this is modeled as string because the usage of float is highly discouraged, as support for them varies across languages.
	StartupJitter string `json:"startupJitter,omitempty"`
}

// InMemoryMachineStatus defines the observed state of InMemoryMachine.
type InMemoryMachineStatus struct {
	// Ready denotes that the machine is ready
	// +optional
	Ready bool `json:"ready"`

	// Conditions defines current service state of the InMemoryMachine.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`

	// v1beta2 groups all the fields that will be added or modified in InMemoryMachine's status with the V1Beta2 version.
	// +optional
	V1Beta2 *InMemoryMachineV1Beta2Status `json:"v1beta2,omitempty"`
}

// InMemoryMachineV1Beta2Status groups all the fields that will be added or modified in InMemoryMachine with the V1Beta2 version.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type InMemoryMachineV1Beta2Status struct {
	// conditions represents the observations of a InMemoryMachine's current state.
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=32
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:resource:path=inmemorymachines,scope=Namespaced,categories=cluster-api
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".metadata.labels['cluster\\.x-k8s\\.io/cluster-name']",description="Cluster"
// +kubebuilder:printcolumn:name="Machine",type="string",JSONPath=".metadata.ownerReferences[?(@.kind==\"Machine\")].name",description="Machine object which owns with this InMemoryMachine"
// +kubebuilder:printcolumn:name="ProviderID",type="string",JSONPath=".spec.providerID",description="Provider ID"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.ready",description="Machine ready status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of InMemoryMachine"

// InMemoryMachine is the schema for the in-memory machine API.
type InMemoryMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InMemoryMachineSpec   `json:"spec,omitempty"`
	Status InMemoryMachineStatus `json:"status,omitempty"`
}

// GetConditions returns the set of conditions for this object.
func (c *InMemoryMachine) GetConditions() clusterv1.Conditions {
	return c.Status.Conditions
}

// SetConditions sets the conditions on this object.
func (c *InMemoryMachine) SetConditions(conditions clusterv1.Conditions) {
	c.Status.Conditions = conditions
}

// GetV1Beta2Conditions returns the set of conditions for this object.
func (c *InMemoryMachine) GetV1Beta2Conditions() []metav1.Condition {
	if c.Status.V1Beta2 == nil {
		return nil
	}
	return c.Status.V1Beta2.Conditions
}

// SetV1Beta2Conditions sets conditions for an API object.
func (c *InMemoryMachine) SetV1Beta2Conditions(conditions []metav1.Condition) {
	if c.Status.V1Beta2 == nil {
		c.Status.V1Beta2 = &InMemoryMachineV1Beta2Status{}
	}
	c.Status.V1Beta2.Conditions = conditions
}

// +kubebuilder:object:root=true

// InMemoryMachineList contains a list of InMemoryMachine.
type InMemoryMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []InMemoryMachine `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &InMemoryMachine{}, &InMemoryMachineList{})
}
