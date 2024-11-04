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
	"fmt"
	"net"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	capierrors "sigs.k8s.io/cluster-api/errors"
)

const (
	// ClusterFinalizer is the finalizer used by the cluster controller to
	// cleanup the cluster resources when a Cluster is being deleted.
	ClusterFinalizer = "cluster.cluster.x-k8s.io"

	// ClusterKind represents the Kind of Cluster.
	ClusterKind = "Cluster"
)

// ANCHOR: ClusterSpec

// ClusterSpec defines the desired state of Cluster.
type ClusterSpec struct {
	// paused can be used to prevent controllers from processing the Cluster and all its associated objects.
	// +optional
	Paused bool `json:"paused,omitempty"`

	// Cluster network configuration.
	// +optional
	ClusterNetwork *ClusterNetwork `json:"clusterNetwork,omitempty"`

	// controlPlaneEndpoint represents the endpoint used to communicate with the control plane.
	// +optional
	ControlPlaneEndpoint APIEndpoint `json:"controlPlaneEndpoint,omitempty"`

	// controlPlaneRef is an optional reference to a provider-specific resource that holds
	// the details for provisioning the Control Plane for a Cluster.
	// +optional
	ControlPlaneRef *corev1.ObjectReference `json:"controlPlaneRef,omitempty"`

	// infrastructureRef is a reference to a provider-specific resource that holds the details
	// for provisioning infrastructure for a cluster in said provider.
	// +optional
	InfrastructureRef *corev1.ObjectReference `json:"infrastructureRef,omitempty"`

	// This encapsulates the topology for the cluster.
	// NOTE: It is required to enable the ClusterTopology
	// feature gate flag to activate managed topologies support;
	// this feature is highly experimental, and parts of it might still be not implemented.
	// +optional
	Topology *Topology `json:"topology,omitempty"`

	// availabilityGates specifies additional conditions to include when evaluating Cluster Available condition.
	//
	// NOTE: this field is considered only for computing v1beta2 conditions.
	// +optional
	// +listType=map
	// +listMapKey=conditionType
	// +kubebuilder:validation:MaxItems=32
	AvailabilityGates []ClusterAvailabilityGate `json:"availabilityGates,omitempty"`
}

// ClusterAvailabilityGate contains the type of a Cluster condition to be used as availability gate.
type ClusterAvailabilityGate struct {
	// conditionType refers to a positive polarity condition (status true means good) with matching type in the Cluster's condition list.
	// If the conditions doesn't exist, it will be treated as unknown.
	// Note: Both Cluster API conditions or conditions added by 3rd party controllers can be used as availability gates.
	// +required
	// +kubebuilder:validation:Pattern=`^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*/)?(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])$`
	// +kubebuilder:validation:MaxLength=316
	// +kubebuilder:validation:MinLength=1
	ConditionType string `json:"conditionType"`
}

// Topology encapsulates the information of the managed resources.
type Topology struct {
	// The name of the ClusterClass object to create the topology.
	Class string `json:"class"`

	// The Kubernetes version of the cluster.
	Version string `json:"version"`

	// rolloutAfter performs a rollout of the entire cluster one component at a time,
	// control plane first and then machine deployments.
	//
	// Deprecated: This field has no function and is going to be removed in the next apiVersion.
	//
	// +optional
	RolloutAfter *metav1.Time `json:"rolloutAfter,omitempty"`

	// controlPlane describes the cluster control plane.
	// +optional
	ControlPlane ControlPlaneTopology `json:"controlPlane,omitempty"`

	// workers encapsulates the different constructs that form the worker nodes
	// for the cluster.
	// +optional
	Workers *WorkersTopology `json:"workers,omitempty"`

	// variables can be used to customize the Cluster through
	// patches. They must comply to the corresponding
	// VariableClasses defined in the ClusterClass.
	// +optional
	// +listType=map
	// +listMapKey=name
	Variables []ClusterVariable `json:"variables,omitempty"`
}

// ControlPlaneTopology specifies the parameters for the control plane nodes in the cluster.
type ControlPlaneTopology struct {
	// metadata is the metadata applied to the ControlPlane and the Machines of the ControlPlane
	// if the ControlPlaneTemplate referenced by the ClusterClass is machine based. If not, it
	// is applied only to the ControlPlane.
	// At runtime this metadata is merged with the corresponding metadata from the ClusterClass.
	// +optional
	Metadata ObjectMeta `json:"metadata,omitempty"`

	// replicas is the number of control plane nodes.
	// If the value is nil, the ControlPlane object is created without the number of Replicas
	// and it's assumed that the control plane controller does not implement support for this field.
	// When specified against a control plane provider that lacks support for this field, this value will be ignored.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// machineHealthCheck allows to enable, disable and override
	// the MachineHealthCheck configuration in the ClusterClass for this control plane.
	// +optional
	MachineHealthCheck *MachineHealthCheckTopology `json:"machineHealthCheck,omitempty"`

	// nodeDrainTimeout is the total amount of time that the controller will spend on draining a node.
	// The default value is 0, meaning that the node can be drained without any time limitations.
	// NOTE: NodeDrainTimeout is different from `kubectl drain --timeout`
	// +optional
	NodeDrainTimeout *metav1.Duration `json:"nodeDrainTimeout,omitempty"`

	// nodeVolumeDetachTimeout is the total amount of time that the controller will spend on waiting for all volumes
	// to be detached. The default value is 0, meaning that the volumes can be detached without any time limitations.
	// +optional
	NodeVolumeDetachTimeout *metav1.Duration `json:"nodeVolumeDetachTimeout,omitempty"`

	// nodeDeletionTimeout defines how long the controller will attempt to delete the Node that the Machine
	// hosts after the Machine is marked for deletion. A duration of 0 will retry deletion indefinitely.
	// Defaults to 10 seconds.
	// +optional
	NodeDeletionTimeout *metav1.Duration `json:"nodeDeletionTimeout,omitempty"`

	// variables can be used to customize the ControlPlane through patches.
	// +optional
	Variables *ControlPlaneVariables `json:"variables,omitempty"`
}

// WorkersTopology represents the different sets of worker nodes in the cluster.
type WorkersTopology struct {
	// machineDeployments is a list of machine deployments in the cluster.
	// +optional
	// +listType=map
	// +listMapKey=name
	MachineDeployments []MachineDeploymentTopology `json:"machineDeployments,omitempty"`

	// machinePools is a list of machine pools in the cluster.
	// +optional
	// +listType=map
	// +listMapKey=name
	MachinePools []MachinePoolTopology `json:"machinePools,omitempty"`
}

// MachineDeploymentTopology specifies the different parameters for a set of worker nodes in the topology.
// This set of nodes is managed by a MachineDeployment object whose lifecycle is managed by the Cluster controller.
type MachineDeploymentTopology struct {
	// metadata is the metadata applied to the MachineDeployment and the machines of the MachineDeployment.
	// At runtime this metadata is merged with the corresponding metadata from the ClusterClass.
	// +optional
	Metadata ObjectMeta `json:"metadata,omitempty"`

	// class is the name of the MachineDeploymentClass used to create the set of worker nodes.
	// This should match one of the deployment classes defined in the ClusterClass object
	// mentioned in the `Cluster.Spec.Class` field.
	Class string `json:"class"`

	// name is the unique identifier for this MachineDeploymentTopology.
	// The value is used with other unique identifiers to create a MachineDeployment's Name
	// (e.g. cluster's name, etc). In case the name is greater than the allowed maximum length,
	// the values are hashed together.
	Name string `json:"name"`

	// failureDomain is the failure domain the machines will be created in.
	// Must match a key in the FailureDomains map stored on the cluster object.
	// +optional
	FailureDomain *string `json:"failureDomain,omitempty"`

	// replicas is the number of worker nodes belonging to this set.
	// If the value is nil, the MachineDeployment is created without the number of Replicas (defaulting to 1)
	// and it's assumed that an external entity (like cluster autoscaler) is responsible for the management
	// of this value.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// machineHealthCheck allows to enable, disable and override
	// the MachineHealthCheck configuration in the ClusterClass for this MachineDeployment.
	// +optional
	MachineHealthCheck *MachineHealthCheckTopology `json:"machineHealthCheck,omitempty"`

	// nodeDrainTimeout is the total amount of time that the controller will spend on draining a node.
	// The default value is 0, meaning that the node can be drained without any time limitations.
	// NOTE: NodeDrainTimeout is different from `kubectl drain --timeout`
	// +optional
	NodeDrainTimeout *metav1.Duration `json:"nodeDrainTimeout,omitempty"`

	// nodeVolumeDetachTimeout is the total amount of time that the controller will spend on waiting for all volumes
	// to be detached. The default value is 0, meaning that the volumes can be detached without any time limitations.
	// +optional
	NodeVolumeDetachTimeout *metav1.Duration `json:"nodeVolumeDetachTimeout,omitempty"`

	// nodeDeletionTimeout defines how long the controller will attempt to delete the Node that the Machine
	// hosts after the Machine is marked for deletion. A duration of 0 will retry deletion indefinitely.
	// Defaults to 10 seconds.
	// +optional
	NodeDeletionTimeout *metav1.Duration `json:"nodeDeletionTimeout,omitempty"`

	// Minimum number of seconds for which a newly created machine should
	// be ready.
	// Defaults to 0 (machine will be considered available as soon as it
	// is ready)
	// +optional
	MinReadySeconds *int32 `json:"minReadySeconds,omitempty"`

	// The deployment strategy to use to replace existing machines with
	// new ones.
	// +optional
	Strategy *MachineDeploymentStrategy `json:"strategy,omitempty"`

	// variables can be used to customize the MachineDeployment through patches.
	// +optional
	Variables *MachineDeploymentVariables `json:"variables,omitempty"`
}

// MachineHealthCheckTopology defines a MachineHealthCheck for a group of machines.
type MachineHealthCheckTopology struct {
	// enable controls if a MachineHealthCheck should be created for the target machines.
	//
	// If false: No MachineHealthCheck will be created.
	//
	// If not set(default): A MachineHealthCheck will be created if it is defined here or
	//  in the associated ClusterClass. If no MachineHealthCheck is defined then none will be created.
	//
	// If true: A MachineHealthCheck is guaranteed to be created. Cluster validation will
	// block if `enable` is true and no MachineHealthCheck definition is available.
	// +optional
	Enable *bool `json:"enable,omitempty"`

	// MachineHealthCheckClass defines a MachineHealthCheck for a group of machines.
	// If specified (any field is set), it entirely overrides the MachineHealthCheckClass defined in ClusterClass.
	MachineHealthCheckClass `json:",inline"`
}

// MachinePoolTopology specifies the different parameters for a pool of worker nodes in the topology.
// This pool of nodes is managed by a MachinePool object whose lifecycle is managed by the Cluster controller.
type MachinePoolTopology struct {
	// metadata is the metadata applied to the MachinePool.
	// At runtime this metadata is merged with the corresponding metadata from the ClusterClass.
	// +optional
	Metadata ObjectMeta `json:"metadata,omitempty"`

	// class is the name of the MachinePoolClass used to create the pool of worker nodes.
	// This should match one of the deployment classes defined in the ClusterClass object
	// mentioned in the `Cluster.Spec.Class` field.
	Class string `json:"class"`

	// name is the unique identifier for this MachinePoolTopology.
	// The value is used with other unique identifiers to create a MachinePool's Name
	// (e.g. cluster's name, etc). In case the name is greater than the allowed maximum length,
	// the values are hashed together.
	Name string `json:"name"`

	// failureDomains is the list of failure domains the machine pool will be created in.
	// Must match a key in the FailureDomains map stored on the cluster object.
	// +optional
	FailureDomains []string `json:"failureDomains,omitempty"`

	// nodeDrainTimeout is the total amount of time that the controller will spend on draining a node.
	// The default value is 0, meaning that the node can be drained without any time limitations.
	// NOTE: NodeDrainTimeout is different from `kubectl drain --timeout`
	// +optional
	NodeDrainTimeout *metav1.Duration `json:"nodeDrainTimeout,omitempty"`

	// nodeVolumeDetachTimeout is the total amount of time that the controller will spend on waiting for all volumes
	// to be detached. The default value is 0, meaning that the volumes can be detached without any time limitations.
	// +optional
	NodeVolumeDetachTimeout *metav1.Duration `json:"nodeVolumeDetachTimeout,omitempty"`

	// nodeDeletionTimeout defines how long the controller will attempt to delete the Node that the MachinePool
	// hosts after the MachinePool is marked for deletion. A duration of 0 will retry deletion indefinitely.
	// Defaults to 10 seconds.
	// +optional
	NodeDeletionTimeout *metav1.Duration `json:"nodeDeletionTimeout,omitempty"`

	// Minimum number of seconds for which a newly created machine pool should
	// be ready.
	// Defaults to 0 (machine will be considered available as soon as it
	// is ready)
	// +optional
	MinReadySeconds *int32 `json:"minReadySeconds,omitempty"`

	// replicas is the number of nodes belonging to this pool.
	// If the value is nil, the MachinePool is created without the number of Replicas (defaulting to 1)
	// and it's assumed that an external entity (like cluster autoscaler) is responsible for the management
	// of this value.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// variables can be used to customize the MachinePool through patches.
	// +optional
	Variables *MachinePoolVariables `json:"variables,omitempty"`
}

// ClusterVariable can be used to customize the Cluster through patches. Each ClusterVariable is associated with a
// Variable definition in the ClusterClass `status` variables.
type ClusterVariable struct {
	// name of the variable.
	Name string `json:"name"`

	// definitionFrom specifies where the definition of this Variable is from.
	//
	// Deprecated: This field is deprecated, must not be set anymore and is going to be removed in the next apiVersion.
	//
	// +optional
	DefinitionFrom string `json:"definitionFrom,omitempty"`

	// value of the variable.
	// Note: the value will be validated against the schema of the corresponding ClusterClassVariable
	// from the ClusterClass.
	// Note: We have to use apiextensionsv1.JSON instead of a custom JSON type, because controller-tools has a
	// hard-coded schema for apiextensionsv1.JSON which cannot be produced by another type via controller-tools,
	// i.e. it is not possible to have no type field.
	// Ref: https://github.com/kubernetes-sigs/controller-tools/blob/d0e03a142d0ecdd5491593e941ee1d6b5d91dba6/pkg/crd/known_types.go#L106-L111
	Value apiextensionsv1.JSON `json:"value"`
}

// ControlPlaneVariables can be used to provide variables for the ControlPlane.
type ControlPlaneVariables struct {
	// overrides can be used to override Cluster level variables.
	// +optional
	// +listType=map
	// +listMapKey=name
	Overrides []ClusterVariable `json:"overrides,omitempty"`
}

// MachineDeploymentVariables can be used to provide variables for a specific MachineDeployment.
type MachineDeploymentVariables struct {
	// overrides can be used to override Cluster level variables.
	// +optional
	// +listType=map
	// +listMapKey=name
	Overrides []ClusterVariable `json:"overrides,omitempty"`
}

// MachinePoolVariables can be used to provide variables for a specific MachinePool.
type MachinePoolVariables struct {
	// overrides can be used to override Cluster level variables.
	// +optional
	// +listType=map
	// +listMapKey=name
	Overrides []ClusterVariable `json:"overrides,omitempty"`
}

// ANCHOR_END: ClusterSpec

// ANCHOR: ClusterNetwork

// ClusterNetwork specifies the different networking
// parameters for a cluster.
type ClusterNetwork struct {
	// apiServerPort specifies the port the API Server should bind to.
	// Defaults to 6443.
	// +optional
	APIServerPort *int32 `json:"apiServerPort,omitempty"`

	// The network ranges from which service VIPs are allocated.
	// +optional
	Services *NetworkRanges `json:"services,omitempty"`

	// The network ranges from which Pod networks are allocated.
	// +optional
	Pods *NetworkRanges `json:"pods,omitempty"`

	// Domain name for services.
	// +optional
	ServiceDomain string `json:"serviceDomain,omitempty"`
}

// ANCHOR_END: ClusterNetwork

// ANCHOR: NetworkRanges

// NetworkRanges represents ranges of network addresses.
type NetworkRanges struct {
	CIDRBlocks []string `json:"cidrBlocks"`
}

func (n NetworkRanges) String() string {
	if len(n.CIDRBlocks) == 0 {
		return ""
	}
	return strings.Join(n.CIDRBlocks, ",")
}

// ANCHOR_END: NetworkRanges

// ANCHOR: ClusterStatus

// ClusterStatus defines the observed state of Cluster.
type ClusterStatus struct {
	// failureDomains is a slice of failure domain objects synced from the infrastructure provider.
	// +optional
	FailureDomains FailureDomains `json:"failureDomains,omitempty"`

	// failureReason indicates that there is a fatal problem reconciling the
	// state, and will be set to a token value suitable for
	// programmatic interpretation.
	//
	// Deprecated: This field is deprecated and is going to be removed in the next apiVersion. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	FailureReason *capierrors.ClusterStatusError `json:"failureReason,omitempty"`

	// failureMessage indicates that there is a fatal problem reconciling the
	// state, and will be set to a descriptive error message.
	//
	// Deprecated: This field is deprecated and is going to be removed in the next apiVersion. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	FailureMessage *string `json:"failureMessage,omitempty"`

	// phase represents the current phase of cluster actuation.
	// E.g. Pending, Running, Terminating, Failed etc.
	// +optional
	Phase string `json:"phase,omitempty"`

	// infrastructureReady is the state of the infrastructure provider.
	// +optional
	InfrastructureReady bool `json:"infrastructureReady"`

	// controlPlaneReady denotes if the control plane became ready during initial provisioning
	// to receive requests.
	// NOTE: this field is part of the Cluster API contract and it is used to orchestrate provisioning.
	// The value of this field is never updated after provisioning is completed. Please use conditions
	// to check the operational state of the control plane.
	// +optional
	ControlPlaneReady bool `json:"controlPlaneReady"`

	// conditions defines current service state of the cluster.
	// +optional
	Conditions Conditions `json:"conditions,omitempty"`

	// observedGeneration is the latest generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// v1beta2 groups all the fields that will be added or modified in Cluster's status with the V1Beta2 version.
	// +optional
	V1Beta2 *ClusterV1Beta2Status `json:"v1beta2,omitempty"`
}

// ClusterV1Beta2Status groups all the fields that will be added or modified in Cluster with the V1Beta2 version.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type ClusterV1Beta2Status struct {
	// conditions represents the observations of a Cluster's current state.
	// Known condition types are Available, InfrastructureReady, ControlPlaneInitialized, ControlPlaneAvailable, WorkersAvailable, MachinesReady
	// MachinesUpToDate, RemoteConnectionProbe, ScalingUp, ScalingDown, Remediating, Deleting, Paused.
	// Additionally, a TopologyReconciled condition will be added in case the Cluster is referencing a ClusterClass / defining a managed Topology.
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=32
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// controlPlane groups all the observations about Cluster's ControlPlane current state.
	// +optional
	ControlPlane *ClusterControlPlaneStatus `json:"controlPlane,omitempty"`

	// workers groups all the observations about Cluster's Workers current state.
	// +optional
	Workers *WorkersStatus `json:"workers,omitempty"`
}

// ClusterControlPlaneStatus groups all the observations about control plane current state.
type ClusterControlPlaneStatus struct {
	// desiredReplicas is the total number of desired control plane machines in this cluster.
	// +optional
	DesiredReplicas *int32 `json:"desiredReplicas,omitempty"`

	// replicas is the total number of control plane machines in this cluster.
	// NOTE: replicas also includes machines still being provisioned or being deleted.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// upToDateReplicas is the number of up-to-date control plane machines in this cluster. A machine is considered up-to-date when Machine's UpToDate condition is true.
	// +optional
	UpToDateReplicas *int32 `json:"upToDateReplicas,omitempty"`

	// readyReplicas is the total number of ready control plane machines in this cluster. A machine is considered ready when Machine's Ready condition is true.
	// +optional
	ReadyReplicas *int32 `json:"readyReplicas,omitempty"`

	// availableReplicas is the total number of available control plane machines in this cluster. A machine is considered available when Machine's Available condition is true.
	// +optional
	AvailableReplicas *int32 `json:"availableReplicas,omitempty"`
}

// WorkersStatus groups all the observations about workers current state.
type WorkersStatus struct {
	// desiredReplicas is the total number of desired worker machines in this cluster.
	// +optional
	DesiredReplicas *int32 `json:"desiredReplicas,omitempty"`

	// replicas is the total number of worker machines in this cluster.
	// NOTE: replicas also includes machines still being provisioned or being deleted.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// upToDateReplicas is the number of up-to-date worker machines in this cluster. A machine is considered up-to-date when Machine's UpToDate condition is true.
	// +optional
	UpToDateReplicas *int32 `json:"upToDateReplicas,omitempty"`

	// readyReplicas is the total number of ready worker machines in this cluster. A machine is considered ready when Machine's Ready condition is true.
	// +optional
	ReadyReplicas *int32 `json:"readyReplicas,omitempty"`

	// availableReplicas is the total number of available worker machines in this cluster. A machine is considered available when Machine's Available condition is true.
	// +optional
	AvailableReplicas *int32 `json:"availableReplicas,omitempty"`
}

// ANCHOR_END: ClusterStatus

// SetTypedPhase sets the Phase field to the string representation of ClusterPhase.
func (c *ClusterStatus) SetTypedPhase(p ClusterPhase) {
	c.Phase = string(p)
}

// GetTypedPhase attempts to parse the Phase field and return
// the typed ClusterPhase representation as described in `machine_phase_types.go`.
func (c *ClusterStatus) GetTypedPhase() ClusterPhase {
	switch phase := ClusterPhase(c.Phase); phase {
	case
		ClusterPhasePending,
		ClusterPhaseProvisioning,
		ClusterPhaseProvisioned,
		ClusterPhaseDeleting,
		ClusterPhaseFailed:
		return phase
	default:
		return ClusterPhaseUnknown
	}
}

// ANCHOR: APIEndpoint

// APIEndpoint represents a reachable Kubernetes API endpoint.
type APIEndpoint struct {
	// The hostname on which the API server is serving.
	Host string `json:"host"`

	// The port on which the API server is serving.
	Port int32 `json:"port"`
}

// IsZero returns true if both host and port are zero values.
func (v APIEndpoint) IsZero() bool {
	return v.Host == "" && v.Port == 0
}

// IsValid returns true if both host and port are non-zero values.
func (v APIEndpoint) IsValid() bool {
	return v.Host != "" && v.Port != 0
}

// String returns a formatted version HOST:PORT of this APIEndpoint.
func (v APIEndpoint) String() string {
	return net.JoinHostPort(v.Host, fmt.Sprintf("%d", v.Port))
}

// ANCHOR_END: APIEndpoint

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=clusters,shortName=cl,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="ClusterClass",type="string",JSONPath=".spec.topology.class",description="ClusterClass of this Cluster, empty if the Cluster is not using a ClusterClass"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase",description="Cluster status such as Pending/Provisioning/Provisioned/Deleting/Failed"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of Cluster"
// +kubebuilder:printcolumn:name="Version",type="string",JSONPath=".spec.topology.version",description="Kubernetes version associated with this Cluster"

// Cluster is the Schema for the clusters API.
type Cluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterSpec   `json:"spec,omitempty"`
	Status ClusterStatus `json:"status,omitempty"`
}

// GetClassKey returns the namespaced name for the class associated with this object.
func (c *Cluster) GetClassKey() types.NamespacedName {
	if c.Spec.Topology == nil {
		return types.NamespacedName{}
	}
	return types.NamespacedName{Namespace: c.GetNamespace(), Name: c.Spec.Topology.Class}
}

// GetConditions returns the set of conditions for this object.
func (c *Cluster) GetConditions() Conditions {
	return c.Status.Conditions
}

// SetConditions sets the conditions on this object.
func (c *Cluster) SetConditions(conditions Conditions) {
	c.Status.Conditions = conditions
}

// GetV1Beta2Conditions returns the set of conditions for this object.
func (c *Cluster) GetV1Beta2Conditions() []metav1.Condition {
	if c.Status.V1Beta2 == nil {
		return nil
	}
	return c.Status.V1Beta2.Conditions
}

// SetV1Beta2Conditions sets conditions for an API object.
func (c *Cluster) SetV1Beta2Conditions(conditions []metav1.Condition) {
	if c.Status.V1Beta2 == nil {
		c.Status.V1Beta2 = &ClusterV1Beta2Status{}
	}
	c.Status.V1Beta2.Conditions = conditions
}

// GetIPFamily returns a ClusterIPFamily from the configuration provided.
//
// Deprecated: IPFamily is not a concept in Kubernetes. It was originally introduced in CAPI for CAPD.
// IPFamily will be dropped in a future release. More details at https://github.com/kubernetes-sigs/cluster-api/issues/7521
func (c *Cluster) GetIPFamily() (ClusterIPFamily, error) {
	var podCIDRs, serviceCIDRs []string
	if c.Spec.ClusterNetwork != nil {
		if c.Spec.ClusterNetwork.Pods != nil {
			podCIDRs = c.Spec.ClusterNetwork.Pods.CIDRBlocks
		}
		if c.Spec.ClusterNetwork.Services != nil {
			serviceCIDRs = c.Spec.ClusterNetwork.Services.CIDRBlocks
		}
	}
	if len(podCIDRs) == 0 && len(serviceCIDRs) == 0 {
		return IPv4IPFamily, nil
	}

	podsIPFamily, err := ipFamilyForCIDRStrings(podCIDRs)
	if err != nil {
		return InvalidIPFamily, fmt.Errorf("pods: %s", err)
	}
	if len(serviceCIDRs) == 0 {
		return podsIPFamily, nil
	}

	servicesIPFamily, err := ipFamilyForCIDRStrings(serviceCIDRs)
	if err != nil {
		return InvalidIPFamily, fmt.Errorf("services: %s", err)
	}
	if len(podCIDRs) == 0 {
		return servicesIPFamily, nil
	}

	if podsIPFamily == DualStackIPFamily {
		return DualStackIPFamily, nil
	} else if podsIPFamily != servicesIPFamily {
		return InvalidIPFamily, errors.New("pods and services IP family mismatch")
	}

	return podsIPFamily, nil
}

func ipFamilyForCIDRStrings(cidrs []string) (ClusterIPFamily, error) {
	if len(cidrs) > 2 {
		return InvalidIPFamily, errors.New("too many CIDRs specified")
	}
	var foundIPv4 bool
	var foundIPv6 bool
	for _, cidr := range cidrs {
		ip, _, err := net.ParseCIDR(cidr)
		if err != nil {
			return InvalidIPFamily, fmt.Errorf("could not parse CIDR: %s", err)
		}
		if ip.To4() != nil {
			foundIPv4 = true
		} else {
			foundIPv6 = true
		}
	}
	switch {
	case foundIPv4 && foundIPv6:
		return DualStackIPFamily, nil
	case foundIPv4:
		return IPv4IPFamily, nil
	case foundIPv6:
		return IPv6IPFamily, nil
	default:
		return InvalidIPFamily, nil
	}
}

// ClusterIPFamily defines the types of supported IP families.
type ClusterIPFamily int

// Define the ClusterIPFamily constants.
const (
	InvalidIPFamily ClusterIPFamily = iota
	IPv4IPFamily
	IPv6IPFamily
	DualStackIPFamily
)

func (f ClusterIPFamily) String() string {
	return [...]string{"InvalidIPFamily", "IPv4IPFamily", "IPv6IPFamily", "DualStackIPFamily"}[f]
}

// +kubebuilder:object:root=true

// ClusterList contains a list of Cluster.
type ClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Cluster `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &Cluster{}, &ClusterList{})
}

// FailureDomains is a slice of FailureDomains.
type FailureDomains map[string]FailureDomainSpec

// FilterControlPlane returns a FailureDomain slice containing only the domains suitable to be used
// for control plane nodes.
func (in FailureDomains) FilterControlPlane() FailureDomains {
	res := make(FailureDomains)
	for id, spec := range in {
		if spec.ControlPlane {
			res[id] = spec
		}
	}
	return res
}

// GetIDs returns a slice containing the ids for failure domains.
func (in FailureDomains) GetIDs() []*string {
	ids := make([]*string, 0, len(in))
	for id := range in {
		ids = append(ids, ptr.To(id))
	}
	return ids
}

// FailureDomainSpec is the Schema for Cluster API failure domains.
// It allows controllers to understand how many failure domains a cluster can optionally span across.
type FailureDomainSpec struct {
	// controlPlane determines if this failure domain is suitable for use by control plane machines.
	// +optional
	ControlPlane bool `json:"controlPlane,omitempty"`

	// attributes is a free form map of attributes an infrastructure provider might use or require.
	// +optional
	Attributes map[string]string `json:"attributes,omitempty"`
}
