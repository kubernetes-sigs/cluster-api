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
	clustercommon "k8s.io/kube-deploy/ext-apiserver/pkg/apis/cluster/common"
)

// Finalizer is set on PreareForCreate callback
const MachineFinalizer string = "machine.cluster.k8s.io"

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

	// The full, authoritative list of taints to apply to the corresponding
	// Node. This list will overwrite any modifications made to the Node on
	// an ongoing basis.
	// +optional
	Taints []corev1.Taint `json:"taints,omitempty"`

	// Provider-specific serialized configuration to use during node
	// creation. It is recommended that providers maintain their own
	// versioned API types that should be serialized/deserialized from this
	// field, akin to component config.
	// +optional
	ProviderConfig string `json:"providerConfig"`

	// A list of roles for this Machine to use.
	Roles []clustercommon.MachineRole `json:"roles,omitempty"`

	// Versions of key software to use. This field is optional at cluster
	// creation time, and omitting the field indicates that the cluster
	// installation tool should select defaults for the user. These
	// defaults may differ based on the cluster installer, but the tool
	// should populate the values it uses when persisting Machine objects.
	// A Machine spec missing this field at runtime is invalid.
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

	// The current versions of software on the corresponding Node (if it
	// exists). This is provided for a few reasons:
	//
	// 1) It is more convenient than checking the NodeRef, traversing it to
	//    the Node, and finding the appropriate field in Node.Status.NodeInfo
	//    (which uses different field names and formatting).
	// 2) It removes some of the dependency on the structure of the Node,
	//    so that if the structure of Node.Status.NodeInfo changes, only
	//    machine controllers need to be updated, rather than every client
	//    of the Machines API.
	// 3) There is no other way simple way to check the ControlPlane
	//    version. A client would have to connect directly to the apiserver
	//    running on the target node in order to find out its version.
	// +optional
	Versions *MachineVersionInfo `json:"versions,omitempty"`

	// In the event that there is a terminal problem reconciling the
	// Machine, both ErrorReason and ErrorMessage will be set. ErrorReason
	// will be populated with a succinct value suitable for machine
	// interpretation, while ErrorMessage will contain a more verbose
	// string suitable for logging and human consumption.
	//
	// These fields should not be set for transitive errors that a
	// controller faces that are expected to be fixed automatically over
	// time (like service outages), but instead indicate that something is
	// fundamentally wrong with the Machine's spec or the configuration of
	// the controller, and that manual intervention is required. Examples
	// of terminal errors would be invalid combinations of settings in the
	// spec, values that are unsupported by the controller, or the
	// responsible controller itself being critically misconfigured.
	//
	// Any transient errors that occur during the reconcilation of Machines
	// can be added as events to the Machine object and/or logged in the
	// controller's output.
	// +optional
	ErrorReason *clustercommon.MachineStatusError `json:"errorReason,omitempty"`
	// +optional
	ErrorMessage *string `json:"errorMessage,omitempty"`
}

type MachineVersionInfo struct {
	// Semantic version of kubelet to run
	Kubelet string `json:"kubelet"`

	// Semantic version of the Kubernetes control plane to
	// run. This should only be populated when the machine is a
	// master.
	// +optional
	ControlPlane string `json:"controlPlane,omitempty"`

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

// PrepareForCreate clears fields that are not allowed to be set by end users on creation.
func (m MachineStrategy) PrepareForCreate(ctx request.Context, obj runtime.Object) {
	// Invoke the parent implementation to strip the Status
	m.DefaultStorageStrategy.PrepareForCreate(ctx, obj)

	// Cast the element and set finalizer
	o := obj.(*cluster.Machine)
	o.ObjectMeta.Finalizers = append(o.ObjectMeta.Finalizers, MachineFinalizer)
}

// DefaultingFunction sets default Machine field values
func (MachineSchemeFns) DefaultingFunction(o interface{}) {
	obj := o.(*Machine)
	// set default field values here
	log.Printf("Defaulting fields for Machine %s\n", obj.Name)
}
