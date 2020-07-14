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
package external defines the types for a generic external provider used for tests
*/
package external

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GenericExternalObject is an object which is not actually managed by CAPI, but we wish to move with clusterctl
// using the "move" label on the resource.
// +kubebuilder:object:root=true
type GenericExternalObject struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
}

// +kubebuilder:object:root=true

type GenericExternalObjectList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GenericExternalObject `json:"items"`
}

func init() {
	SchemeBuilder.Register(
		&GenericExternalObject{}, &GenericExternalObjectList{},
	)
}
