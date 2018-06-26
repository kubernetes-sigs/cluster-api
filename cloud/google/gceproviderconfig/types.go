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

package gceproviderconfig

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type GCEMachineProviderConfig struct {
	metav1.TypeMeta `json:",inline"`

	// A list of roles for this Machine to use.
	Roles []MachineRole `json:"roles,omitempty"`

	Zone        string `json:"zone"`
	MachineType string `json:"machineType"`

	// The name of the OS to be installed on the machine.
	OS    string `json:"os"`
	Disks []Disk `json:"disks"`
}

// The MachineRole indicates the purpose of the Machine, and will determine
// what software and configuration will be used when provisioning and managing
// the Machine. A single Machine may have more than one role, and the list and
// definitions of supported roles is expected to evolve over time.
//
// Currently, only two roles are supported: Master and Node. In the future, we
// expect user needs to drive the evolution and granularity of these roles,
// with new additions accommodating common cluster patterns, like dedicated
// etcd Machines.
//
//                 +-----------------------+------------------------+
//                 | Master present        | Master absent          |
// +---------------+-----------------------+------------------------|
// | Node present: | Install control plane | Join the cluster as    |
// |               | and be schedulable    | just a node            |
// |---------------+-----------------------+------------------------|
// | Node absent:  | Install control plane | Invalid configuration  |
// |               | and be unschedulable  |                        |
// +---------------+-----------------------+------------------------+
type MachineRole string

const (
	MasterRole MachineRole = "Master"
	NodeRole   MachineRole = "Node"
)

type Disk struct {
	InitializeParams DiskInitializeParams `json:"initializeParams"`
}

type DiskInitializeParams struct {
	DiskSizeGb int64  `json:"diskSizeGb"`
	DiskType   string `json:"diskType"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type GCEClusterProviderConfig struct {
	metav1.TypeMeta `json:",inline"`

	Project     string `json:"project"`
}