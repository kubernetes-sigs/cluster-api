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

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// KubeadmControlPlaneTemplateSpec defines the desired state of KubeadmControlPlaneTemplate.
type KubeadmControlPlaneTemplateSpec struct {
	// template defines the desired state of KubeadmControlPlaneTemplate.
	// +required
	Template KubeadmControlPlaneTemplateResource `json:"template,omitempty,omitzero"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=kubeadmcontrolplanetemplates,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="ClusterClass",type="string",JSONPath=`.metadata.ownerReferences[?(@.kind=="ClusterClass")].name`,description="Name of the ClusterClass owning this template"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of KubeadmControlPlaneTemplate"

// KubeadmControlPlaneTemplate is the Schema for the kubeadmcontrolplanetemplates API.
// NOTE: This CRD can only be used if the ClusterTopology feature gate is enabled.
type KubeadmControlPlaneTemplate struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec is the desired state of KubeadmControlPlaneTemplate.
	// +optional
	Spec KubeadmControlPlaneTemplateSpec `json:"spec,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// KubeadmControlPlaneTemplateList contains a list of KubeadmControlPlaneTemplate.
type KubeadmControlPlaneTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard list's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#lists-and-simple-kinds
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`
	// items is the list of KubeadmControlPlaneTemplates.
	Items []KubeadmControlPlaneTemplate `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &KubeadmControlPlaneTemplate{}, &KubeadmControlPlaneTemplateList{})
}

// KubeadmControlPlaneTemplateResource describes the data needed to create a KubeadmControlPlane from a template.
// +kubebuilder:validation:MinProperties=1
type KubeadmControlPlaneTemplateResource struct {
	// metadata is the standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	ObjectMeta clusterv1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec is the desired state of KubeadmControlPlaneTemplateResource.
	// +optional
	Spec KubeadmControlPlaneTemplateResourceSpec `json:"spec,omitempty,omitzero"`
}

// KubeadmControlPlaneTemplateResourceSpec defines the desired state of KubeadmControlPlane.
// NOTE: KubeadmControlPlaneTemplateResourceSpec is similar to KubeadmControlPlaneSpec but
// omits Replicas and Version fields. These fields do not make sense on the KubeadmControlPlaneTemplate,
// because they are calculated by the Cluster topology reconciler during reconciliation and thus cannot
// be configured on the KubeadmControlPlaneTemplate.
// +kubebuilder:validation:MinProperties=1
type KubeadmControlPlaneTemplateResourceSpec struct {
	// machineTemplate contains information about how machines
	// should be shaped when creating or updating a control plane.
	// +optional
	MachineTemplate KubeadmControlPlaneTemplateMachineTemplate `json:"machineTemplate,omitempty,omitzero"`

	// kubeadmConfigSpec is a KubeadmConfigSpec
	// to use for initializing and joining machines to the control plane.
	// +optional
	KubeadmConfigSpec bootstrapv1.KubeadmConfigSpec `json:"kubeadmConfigSpec,omitempty,omitzero"`

	// rollout allows you to configure the behaviour of rolling updates to the control plane Machines.
	// It allows you to require that all Machines are replaced before or after a certain time,
	// and allows you to define the strategy used during rolling replacements.
	// +optional
	Rollout KubeadmControlPlaneRolloutSpec `json:"rollout,omitempty,omitzero"`

	// remediation controls how unhealthy Machines are remediated.
	// +optional
	Remediation KubeadmControlPlaneRemediationSpec `json:"remediation,omitempty,omitzero"`

	// machineNaming allows changing the naming pattern used when creating Machines.
	// InfraMachines & KubeadmConfigs will use the same name as the corresponding Machines.
	// +optional
	MachineNaming MachineNamingSpec `json:"machineNaming,omitempty,omitzero"`
}

// KubeadmControlPlaneTemplateMachineTemplate defines the template for Machines
// in a KubeadmControlPlaneTemplate object.
// NOTE: KubeadmControlPlaneTemplateMachineTemplate is similar to KubeadmControlPlaneMachineTemplate but
// omits ObjectMeta and InfrastructureRef fields. These fields do not make sense on the KubeadmControlPlaneTemplate,
// because they are calculated by the Cluster topology reconciler during reconciliation and thus cannot
// be configured on the KubeadmControlPlaneTemplate.
// +kubebuilder:validation:MinProperties=1
type KubeadmControlPlaneTemplateMachineTemplate struct {
	// metadata is the standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	ObjectMeta clusterv1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the spec for Machines
	// in a KubeadmControlPlane object.
	// +optional
	Spec KubeadmControlPlaneTemplateMachineTemplateSpec `json:"spec,omitempty,omitzero"`
}

// KubeadmControlPlaneTemplateMachineTemplateSpec defines the spec for Machines
// in a KubeadmControlPlane object.
// +kubebuilder:validation:MinProperties=1
type KubeadmControlPlaneTemplateMachineTemplateSpec struct {
	// deletion contains configuration options for Machine deletion.
	// +optional
	Deletion KubeadmControlPlaneTemplateMachineTemplateDeletionSpec `json:"deletion,omitempty,omitzero"`
}

// KubeadmControlPlaneTemplateMachineTemplateDeletionSpec contains configuration options for Machine deletion.
// +kubebuilder:validation:MinProperties=1
type KubeadmControlPlaneTemplateMachineTemplateDeletionSpec struct {
	// nodeDrainTimeoutSeconds is the total amount of time that the controller will spend on draining a controlplane node
	// The default value is 0, meaning that the node can be drained without any time limitations.
	// NOTE: nodeDrainTimeoutSeconds is different from `kubectl drain --timeout`
	// +optional
	// +kubebuilder:validation:Minimum=0
	NodeDrainTimeoutSeconds *int32 `json:"nodeDrainTimeoutSeconds,omitempty"`

	// nodeVolumeDetachTimeoutSeconds is the total amount of time that the controller will spend on waiting for all volumes
	// to be detached. The default value is 0, meaning that the volumes can be detached without any time limitations.
	// +optional
	// +kubebuilder:validation:Minimum=0
	NodeVolumeDetachTimeoutSeconds *int32 `json:"nodeVolumeDetachTimeoutSeconds,omitempty"`

	// nodeDeletionTimeoutSeconds defines how long the machine controller will attempt to delete the Node that the Machine
	// hosts after the Machine is marked for deletion. A duration of 0 will retry deletion indefinitely.
	// If no value is provided, the default value for this property of the Machine resource will be used.
	// +optional
	// +kubebuilder:validation:Minimum=0
	NodeDeletionTimeoutSeconds *int32 `json:"nodeDeletionTimeoutSeconds,omitempty"`
}
