/*
Copyright 2020 The Kubernetes Authors.

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

/*
package bootstrap defines the types for dummy bootstrap provider used for tests
*/
package bootstrap

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DummyBootstrapConfigStatus struct {
	// +optional
	DataSecretName *string `json:"dataSecretName,omitempty"`
}

// +kubebuilder:object:root=true

type DummyBootstrapConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Status            DummyBootstrapConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

type DummyBootstrapConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DummyBootstrapConfig `json:"items"`
}

// +kubebuilder:object:root=true

type DummyBootstrapConfigTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
}

// +kubebuilder:object:root=true

type DummyBootstrapConfigTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DummyBootstrapConfigTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(
		&DummyBootstrapConfig{}, &DummyBootstrapConfigList{},
		&DummyBootstrapConfigTemplate{}, &DummyBootstrapConfigTemplateList{},
	)
}
