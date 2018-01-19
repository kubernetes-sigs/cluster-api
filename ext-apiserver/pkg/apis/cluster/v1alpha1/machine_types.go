
/*
Copyright 2018 The Kubernetes Authors.

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
	"log"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/endpoints/request"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"k8s.io/kube-deploy/ext-apiserver/pkg/apis/cluster"
	"k8s.io/kube-deploy/ext-apiserver/pkg/apis/cluster/common"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Machine
// +k8s:openapi-gen=true
// +resource:path=machines,strategy=MachineStrategy
type Machine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MachineSpec   `json:"spec,omitempty"`
	Status MachineStatus `json:"status,omitempty"`
}

// MachineSpec defines the desired state of Machine
type MachineSpec struct {
	// This ObjectMeta will autopopulate the Node created. Use this to
	// indicate what labels, annotations, name prefix, etc., should be used
	// when creating the Node.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Provider-specific serialized configuration to use during node
	// creation. It is recommended that providers maintain their own
	// versioned API types that should be serialized/deserialized from this
	// field, akin to component config.
	// +optional
	ProviderConfig string `json:"providerConfig"`

	// A list consisting of "Master" and/or "Node".
	//
	//                 +-----------------------+------------------------+
	//                 | Master present        | Master absent          |
	// +---------------+-----------------------+------------------------|
	// | Node present: | Install control plane | Join the cluster as    |
	// |               | and be schedulable    | just a node            |
	// |---------------+-----------------------+------------------------|
	// | Node absent:  | Install control plane | Invalid configuration  |
	// |               | and be unscheduleable |                        |
	// +---------------+-----------------------+------------------------+
	Roles []string `json:"roles,omitempty"`

	// Versions of key software to use.
	// +optional
	Versions MachineVersionInfo `json:"versions,omitempty"`

	// To populate in the associated Node for dynamic kubelet config. This
	// field already exists in Node, so any updates to it in the Machine
	// spec will be automatially copied to the linked NodeRef from the
	// status. The rest of dynamic kubelet config support should then work
	// as-is.
	// +optional
	ConfigSource *corev1.NodeConfigSource `json:"configSource,omitempty"`
}

// MachineStatus defines the observed state of Machine
type MachineStatus struct {
	// If the corresponding Node exists, this will point to its object.
	// +optional
	NodeRef *corev1.ObjectReference `json:"nodeRef,omitempty"`

	// When was this status last observed
	// +optional
	LastUpdated metav1.Time `json:"lastUpdated,omitempty"`

	// Indicates whether or not the Machine is fully reconciled. When a
	// controller observes that the spec has changed and no longer matches
	// reality, it should update Ready to false before reconciling the
	// state, and then set back to true when the state matches the spec.
	Ready bool `json:"ready"`

	// NB: Eventually we will redefine ErrorReason as MachineStatusError once the
	// following issue is fixed.
	// https://github.com/kubernetes-incubator/apiserver-builder/issues/176

	// If set, indicates that there is a problem reconciling state, and
	// will be set to a token value suitable for machine interpretation.
	// +optional
	ErrorReason *common.MachineStatusError `json:"errorReason,omitempty"`

	// +optional
	// If set, indicates that there is a problem reconciling state, and
	// will be set to a human readable string to indicate the problem.
	ErrorMessage *string `json:"errorMessage,omitempty"`
}

type MachineVersionInfo struct {
	// Semantic version of kubelet to run
	Kubelet string `json:"kubelet"`

	// Semantic version of the Kubernetes control plane to
	// run. This should only be populated when the machine is a
	// master.
	ControlPlane string `json:"controlPlane"`

	// Name/version of container runtime
	ContainerRuntime ContainerRuntimeInfo `json:"containerRuntime"`
}

type ContainerRuntimeInfo struct {
	// docker, rkt, containerd, ...
	Name string `json:"name"`

	// Semantic version of the container runtime to use
	Version string `json:"version"`
}

// Validate checks that an instance of Machine is well formed
func (MachineStrategy) Validate(ctx request.Context, obj runtime.Object) field.ErrorList {
	o := obj.(*cluster.Machine)
	log.Printf("Validating fields for Machine %s\n", o.Name)
	errors := field.ErrorList{}
	// perform validation here and add to errors using field.Invalid
	return errors
}

// DefaultingFunction sets default Machine field values
func (MachineSchemeFns) DefaultingFunction(o interface{}) {
	obj := o.(*Machine)
	// set default field values here
	log.Printf("Defaulting fields for Machine %s\n", obj.Name)
}
