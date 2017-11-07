package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/api"
)

// deepcopy-gen can be installed with:
// go get k8s.io/gengo/examples/deepcopy-gen
//go:generate deepcopy-gen -i . -O zz_generated.deepcopy

const MachineResourcePlural = "machines"

// Machine represents a single Node that should exist (whether it does or
// not yet). In this model, there is no grouping of nodes to scale with a
// numeric field. Each Machine exists independently, and grouping can only
// be inferred via label selectors.
//
// In order for a new Node to be created, one can generically create a new
// Machine object, possibly copying the spec from an existing Machine
// or a template. To scale down the cluster, delete specific instances of
// Machines and the underlying Nodes will be unregistered/deprovisioned.
// Separate provider-specific controllers will watch Machine objects they can
// act on (like a GCE cloud controller watching for only Machines destined for
// GCE) and take the appropriate actions.
//
// Any updates to the MachineSpec will be actuated to change the Node in
// place or replace the Node with one conforming to the spec. In this model,
// the fact that a controller is able to upgrade a node via in-place upgrades
// or via a cloud replacement is an implementation detail without API controls.
//
// It is recommended, but not required, that provider-specific controllers add
// finalizers to Machine objects so that they can be triggered on deletion to
// release the necessary external resources, reporting any errors encountered.

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type Machine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   MachineSpec   `json:"spec"`
	Status MachineStatus `json:"status,omitempty"`
}

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

type MachineStatus struct {
	// If the corresponding Node exists, this will point to its object.
	// +optional
	NodeRef *api.ObjectReference `json:"nodeRef,omitempty"`

	// When was this status last observed
	// +optional
	LastUpdated metav1.Time `json:"lastUpdated,omitempty"`

	// Indicates whether or not the Machine is fully reconciled. When a
	// controller observes that the spec has changed and no longer matches
	// reality, it should update Ready to false before reconciling the
	// state, and then set back to true when the state matches the spec.
	Ready bool `json:"ready"`

	// If set, indicates that there is a problem reconciling state, and
	// will be set to a token value suitable for machine interpretation.
	// +optional
	ErrorReason *MachineStatusError `json:"errorReason,omitempty"`

	// +optional
	// If set, indicates that there is a problem reconciling state, and
	// will be set to a human readable string to indicate the problem.
	ErrorMessage *string `json:"errorMessage,omitempty"`
}

type MachineStatusError string

const (
	// Represents that the combination of configuration in the MachineSpec
	// is not supported by this cluster. This is not a transient error, but
	// indicates a state that must be fixed before progress can be made.
	//
	// Example: the ProviderConfig specifies an instance type that doesn't exist,
	InvalidConfigurationMachineError MachineStatusError = "InvalidConfiguration"

	// This indicates that the MachineSpec has been updated in a way that
	// is not supported for reconciliation on this cluster. The spec may be
	// completely valid from a configuration standpoint, but the controller
	// does not support changing the real world state to match the new
	// spec.
	//
	// Example: the responsible controller is not capable of changing the
	// container runtime from docker to rkt.
	UnsupportedChangeMachineError MachineStatusError = "UnsupportedChange"

	// This generally refers to exceeding one's quota in a cloud provider,
	// or running out of physical machines in an on-premise environment.
	InsufficientResourcesMachineError MachineStatusError = "InsufficientResources"

	// There was an error while trying to create a Node to match this
	// Machine. This may indicate a transient problem that will be fixed
	// automatically with time, such as a service outage, or a terminal
	// error during creation that doesn't match a more specific
	// MachineStatusError value.
	//
	// Example: timeout trying to connect to GCE.
	CreateMachineError MachineStatusError = "CreateError"

	// An error was encountered while trying to delete the Node that this
	// Machine represents. This could be a transient or terminal error, but
	// will only be observable if the provider's Machine controller has
	// added a finalizer to the object to more gracefully handle deletions.
	//
	// Example: cannot resolve EC2 IP address.
	DeleteMachineError MachineStatusError = "DeleteError"
)

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

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type MachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []Machine `json:"items"`
}
