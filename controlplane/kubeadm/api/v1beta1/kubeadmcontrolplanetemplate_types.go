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

	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
)

// KubeadmControlPlaneTemplateSpec defines the desired state of KubeadmControlPlaneTemplate.
type KubeadmControlPlaneTemplateSpec struct {
	Template KubeadmControlPlaneTemplateResource `json:"template"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=kubeadmcontrolplanetemplates,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of KubeadmControlPlaneTemplate"

// KubeadmControlPlaneTemplate is the Schema for the kubeadmcontrolplanetemplates API.
type KubeadmControlPlaneTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec KubeadmControlPlaneTemplateSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// KubeadmControlPlaneTemplateList contains a list of KubeadmControlPlaneTemplate.
type KubeadmControlPlaneTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KubeadmControlPlaneTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KubeadmControlPlaneTemplate{}, &KubeadmControlPlaneTemplateList{})
}

// KubeadmControlPlaneTemplateResource describes the data needed to create a KubeadmControlPlane from a template.
type KubeadmControlPlaneTemplateResource struct {
	Spec KubeadmControlPlaneTemplateResourceSpec `json:"spec"`
}

// KubeadmControlPlaneTemplateResourceSpec defines the desired state of KubeadmControlPlane.
// NOTE: KubeadmControlPlaneTemplateResourceSpec is similar to KubeadmControlPlaneSpec but
// omits Replicas and Version fields. These fields do not make sense on the KubeadmControlPlaneTemplate,
// because they are calculated by the Cluster topology reconciler during reconciliation and thus cannot
// be configured on the KubeadmControlPlaneTemplate.
type KubeadmControlPlaneTemplateResourceSpec struct {
	// MachineTemplate contains information about how machines
	// should be shaped when creating or updating a control plane.
	// +optional
	MachineTemplate *KubeadmControlPlaneTemplateMachineTemplate `json:"machineTemplate,omitempty"`

	// KubeadmConfigSpec is a KubeadmConfigSpec
	// to use for initializing and joining machines to the control plane.
	KubeadmConfigSpec bootstrapv1.KubeadmConfigSpec `json:"kubeadmConfigSpec"`

	// RolloutAfter is a field to indicate a rollout should be performed
	// after the specified time even if no changes have been made to the
	// KubeadmControlPlane.
	//
	// +optional
	RolloutAfter *metav1.Time `json:"rolloutAfter,omitempty"`

	// The RolloutStrategy to use to replace control plane machines with
	// new ones.
	// +optional
	// +kubebuilder:default={type: "RollingUpdate", rollingUpdate: {maxSurge: 1}}
	RolloutStrategy *RolloutStrategy `json:"rolloutStrategy,omitempty"`
}

// KubeadmControlPlaneTemplateMachineTemplate defines the template for Machines
// in a KubeadmControlPlaneTemplate object.
// NOTE: KubeadmControlPlaneTemplateMachineTemplate is similar to KubeadmControlPlaneMachineTemplate but
// omits ObjectMeta and InfrastructureRef fields. These fields do not make sense on the KubeadmControlPlaneTemplate,
// because they are calculated by the Cluster topology reconciler during reconciliation and thus cannot
// be configured on the KubeadmControlPlaneTemplate.
type KubeadmControlPlaneTemplateMachineTemplate struct {
	// NodeDrainTimeout is the total amount of time that the controller will spend on draining a controlplane node
	// The default value is 0, meaning that the node can be drained without any time limitations.
	// NOTE: NodeDrainTimeout is different from `kubectl drain --timeout`
	// +optional
	NodeDrainTimeout *metav1.Duration `json:"nodeDrainTimeout,omitempty"`
}
