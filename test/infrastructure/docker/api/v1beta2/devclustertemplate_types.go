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

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// DevClusterTemplateSpec defines the desired state of DevClusterTemplate.
type DevClusterTemplateSpec struct {
	Template DevClusterTemplateResource `json:"template"`
}

// +kubebuilder:resource:path=devclustertemplates,scope=Namespaced,categories=cluster-api
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of the DevClusterTemplate"

// DevClusterTemplate is the Schema for the DevClusterTemplate API.
type DevClusterTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec DevClusterTemplateSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// DevClusterTemplateList contains a list of DevClusterTemplate.
type DevClusterTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DevClusterTemplate `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &DevClusterTemplate{}, &DevClusterTemplateList{})
}

// DevClusterTemplateResource describes the data needed to create a DevCluster from a template.
type DevClusterTemplateResource struct {
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	ObjectMeta clusterv1.ObjectMeta `json:"metadata,omitempty,omitzero"`
	Spec       DevClusterSpec       `json:"spec"`
}
