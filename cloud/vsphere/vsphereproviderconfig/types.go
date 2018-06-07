/*
Copyright 2017 The Kubernetes Authors.

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

package vsphereproviderconfig

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VsphereMachineProviderConfig struct {
	metav1.TypeMeta `json:",inline"`

	// Name of the machine that's registered in the NamedMachines ConfigMap.
	VsphereMachine string `json:"vsphereMachine"`
	// List of variables for the chosen machine.
	MachineVariables map[string]string `json:"machineVariables"`
}

// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VsphereClusterProviderConfig struct {
	metav1.TypeMeta `json:",inline"`

	VsphereUser     string `json:"vsphereUser"`
	VspherePassword string `json:"vspherePassword"`
	VsphereServer   string `json:"vsphereServer"`
}
