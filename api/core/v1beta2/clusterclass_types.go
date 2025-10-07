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
	"reflect"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// ClusterClassKind represents the Kind of ClusterClass.
const ClusterClassKind = "ClusterClass"

// ClusterClass VariablesReady condition and corresponding reasons.
const (
	// ClusterClassVariablesReadyCondition is true if the ClusterClass variables, including both inline and external
	// variables, have been successfully reconciled and thus ready to be used to default and validate variables on Clusters using
	// this ClusterClass.
	ClusterClassVariablesReadyCondition = "VariablesReady"

	// ClusterClassVariablesReadyReason surfaces that the variables are ready.
	ClusterClassVariablesReadyReason = "VariablesReady"

	// ClusterClassVariablesReadyVariableDiscoveryFailedReason surfaces that variable discovery failed.
	ClusterClassVariablesReadyVariableDiscoveryFailedReason = "VariableDiscoveryFailed"
)

// ClusterClass RefVersionsUpToDate condition and corresponding reasons.
const (
	// ClusterClassRefVersionsUpToDateCondition documents if the references in the ClusterClass are
	// up-to-date (i.e. they are using the latest apiVersion of the current Cluster API contract from
	// the corresponding CRD).
	ClusterClassRefVersionsUpToDateCondition = "RefVersionsUpToDate"

	// ClusterClassRefVersionsUpToDateReason surfaces that the references in the ClusterClass are
	// up-to-date (i.e. they are using the latest apiVersion of the current Cluster API contract from
	// the corresponding CRD).
	ClusterClassRefVersionsUpToDateReason = "RefVersionsUpToDate"

	// ClusterClassRefVersionsNotUpToDateReason surfaces that the references in the ClusterClass are not
	// up-to-date (i.e. they are not using the latest apiVersion of the current Cluster API contract from
	// the corresponding CRD).
	ClusterClassRefVersionsNotUpToDateReason = "RefVersionsNotUpToDate"

	// ClusterClassRefVersionsUpToDateInternalErrorReason surfaces that an unexpected error occurred when validating
	// if the references are up-to-date.
	ClusterClassRefVersionsUpToDateInternalErrorReason = InternalErrorReason
)

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=clusterclasses,shortName=cc,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Paused",type="string",JSONPath=`.status.conditions[?(@.type=="Paused")].status`,description="Reconciliation paused",priority=10
// +kubebuilder:printcolumn:name="Variables Ready",type="string",JSONPath=`.status.conditions[?(@.type=="VariablesReady")].status`,description="Variables ready"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of ClusterClass"

// ClusterClass is a template which can be used to create managed topologies.
// NOTE: This CRD can only be used if the ClusterTopology feature gate is enabled.
type ClusterClass struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec is the desired state of ClusterClass.
	// +required
	Spec ClusterClassSpec `json:"spec,omitempty,omitzero"`
	// status is the observed state of ClusterClass.
	// +optional
	Status ClusterClassStatus `json:"status,omitempty,omitzero"`
}

// ClusterClassSpec describes the desired state of the ClusterClass.
type ClusterClassSpec struct {
	// availabilityGates specifies additional conditions to include when evaluating Cluster Available condition.
	//
	// NOTE: If a Cluster is using this ClusterClass, and this Cluster defines a custom list of availabilityGates,
	// such list overrides availabilityGates defined in this field.
	// +optional
	// +listType=map
	// +listMapKey=conditionType
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=32
	AvailabilityGates []ClusterAvailabilityGate `json:"availabilityGates,omitempty"`

	// infrastructure is a reference to a local struct that holds the details
	// for provisioning the infrastructure cluster for the Cluster.
	// +required
	Infrastructure InfrastructureClass `json:"infrastructure,omitempty,omitzero"`

	// controlPlane is a reference to a local struct that holds the details
	// for provisioning the Control Plane for the Cluster.
	// +required
	ControlPlane ControlPlaneClass `json:"controlPlane,omitempty,omitzero"`

	// workers describes the worker nodes for the cluster.
	// It is a collection of node types which can be used to create
	// the worker nodes of the cluster.
	// +optional
	Workers WorkersClass `json:"workers,omitempty,omitzero"`

	// variables defines the variables which can be configured
	// in the Cluster topology and are then used in patches.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=1000
	Variables []ClusterClassVariable `json:"variables,omitempty"`

	// patches defines the patches which are applied to customize
	// referenced templates of a ClusterClass.
	// Note: Patches will be applied in the order of the array.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=1000
	Patches []ClusterClassPatch `json:"patches,omitempty"`

	// upgrade defines the upgrade configuration for clusters using this ClusterClass.
	// +optional
	Upgrade ClusterClassUpgrade `json:"upgrade,omitempty,omitzero"`

	// kubernetesVersions is the list of Kubernetes versions that can be
	// used for clusters using this ClusterClass.
	// The list of version must be ordered from the older to the newer version, and there should be
	// at least one version for every minor in between the first and the last version.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=256
	KubernetesVersions []string `json:"kubernetesVersions,omitempty"`
}

// InfrastructureClass defines the class for the infrastructure cluster.
type InfrastructureClass struct {
	// templateRef contains the reference to a provider-specific infrastructure cluster template.
	// +required
	TemplateRef ClusterClassTemplateReference `json:"templateRef,omitempty,omitzero"`

	// naming allows changing the naming pattern used when creating the infrastructure cluster object.
	// +optional
	Naming InfrastructureClassNamingSpec `json:"naming,omitempty,omitzero"`
}

// ControlPlaneClass defines the class for the control plane.
type ControlPlaneClass struct {
	// metadata is the metadata applied to the ControlPlane and the Machines of the ControlPlane
	// if the ControlPlaneTemplate referenced is machine based. If not, it is applied only to the
	// ControlPlane.
	// At runtime this metadata is merged with the corresponding metadata from the topology.
	//
	// This field is supported if and only if the control plane provider template
	// referenced is Machine based.
	// +optional
	Metadata ObjectMeta `json:"metadata,omitempty,omitzero"`

	// templateRef contains the reference to a provider-specific control plane template.
	// +required
	TemplateRef ClusterClassTemplateReference `json:"templateRef,omitempty,omitzero"`

	// machineInfrastructure defines the metadata and infrastructure information
	// for control plane machines.
	//
	// This field is supported if and only if the control plane provider template
	// referenced above is Machine based and supports setting replicas.
	//
	// +optional
	MachineInfrastructure ControlPlaneClassMachineInfrastructureTemplate `json:"machineInfrastructure,omitempty,omitzero"`

	// healthCheck defines a MachineHealthCheck for this ControlPlaneClass.
	// This field is supported if and only if the ControlPlane provider template
	// referenced above is Machine based and supports setting replicas.
	// +optional
	HealthCheck ControlPlaneClassHealthCheck `json:"healthCheck,omitempty,omitzero"`

	// naming allows changing the naming pattern used when creating the control plane provider object.
	// +optional
	Naming ControlPlaneClassNamingSpec `json:"naming,omitempty,omitzero"`

	// deletion contains configuration options for Machine deletion.
	// +optional
	Deletion ControlPlaneClassMachineDeletionSpec `json:"deletion,omitempty,omitzero"`

	// readinessGates specifies additional conditions to include when evaluating Machine Ready condition.
	//
	// This field can be used e.g. to instruct the machine controller to include in the computation for Machine's ready
	// computation a condition, managed by an external controllers, reporting the status of special software/hardware installed on the Machine.
	//
	// NOTE: If a Cluster defines a custom list of readinessGates for the control plane,
	// such list overrides readinessGates defined in this field.
	// NOTE: Specific control plane provider implementations might automatically extend the list of readinessGates;
	// e.g. the kubeadm control provider adds ReadinessGates for the APIServerPodHealthy, SchedulerPodHealthy conditions, etc.
	// +optional
	// +listType=map
	// +listMapKey=conditionType
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=32
	ReadinessGates []MachineReadinessGate `json:"readinessGates,omitempty"`
}

// ControlPlaneClassHealthCheck defines a MachineHealthCheck for control plane machines.
// +kubebuilder:validation:MinProperties=1
type ControlPlaneClassHealthCheck struct {
	// checks are the checks that are used to evaluate if a Machine is healthy.
	//
	// Independent of this configuration the MachineHealthCheck controller will always
	// flag Machines with `cluster.x-k8s.io/remediate-machine` annotation and
	// Machines with deleted Nodes as unhealthy.
	//
	// Furthermore, if checks.nodeStartupTimeoutSeconds is not set it
	// is defaulted to 10 minutes and evaluated accordingly.
	//
	// +optional
	Checks ControlPlaneClassHealthCheckChecks `json:"checks,omitempty,omitzero"`

	// remediation configures if and how remediations are triggered if a Machine is unhealthy.
	//
	// If remediation or remediation.triggerIf is not set,
	// remediation will always be triggered for unhealthy Machines.
	//
	// If remediation or remediation.templateRef is not set,
	// the OwnerRemediated condition will be set on unhealthy Machines to trigger remediation via
	// the owner of the Machines, for example a MachineSet or a KubeadmControlPlane.
	//
	// +optional
	Remediation ControlPlaneClassHealthCheckRemediation `json:"remediation,omitempty,omitzero"`
}

// IsDefined returns true if one of checks and remediation are not zero.
func (m *ControlPlaneClassHealthCheck) IsDefined() bool {
	return !reflect.ValueOf(m.Checks).IsZero() || !reflect.ValueOf(m.Remediation).IsZero()
}

// ControlPlaneClassHealthCheckChecks are the checks that are used to evaluate if a control plane Machine is healthy.
// +kubebuilder:validation:MinProperties=1
type ControlPlaneClassHealthCheckChecks struct {
	// nodeStartupTimeoutSeconds allows to set the maximum time for MachineHealthCheck
	// to consider a Machine unhealthy if a corresponding Node isn't associated
	// through a `Spec.ProviderID` field.
	//
	// The duration set in this field is compared to the greatest of:
	// - Cluster's infrastructure ready condition timestamp (if and when available)
	// - Control Plane's initialized condition timestamp (if and when available)
	// - Machine's infrastructure ready condition timestamp (if and when available)
	// - Machine's metadata creation timestamp
	//
	// Defaults to 10 minutes.
	// If you wish to disable this feature, set the value explicitly to 0.
	// +optional
	// +kubebuilder:validation:Minimum=0
	NodeStartupTimeoutSeconds *int32 `json:"nodeStartupTimeoutSeconds,omitempty"`

	// unhealthyNodeConditions contains a list of conditions that determine
	// whether a node is considered unhealthy. The conditions are combined in a
	// logical OR, i.e. if any of the conditions is met, the node is unhealthy.
	//
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	UnhealthyNodeConditions []UnhealthyNodeCondition `json:"unhealthyNodeConditions,omitempty"`

	// unhealthyMachineConditions contains a list of the machine conditions that determine
	// whether a machine is considered unhealthy.  The conditions are combined in a
	// logical OR, i.e. if any of the conditions is met, the machine is unhealthy.
	//
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	UnhealthyMachineConditions []UnhealthyMachineCondition `json:"unhealthyMachineConditions,omitempty"`
}

// ControlPlaneClassHealthCheckRemediation configures if and how remediations are triggered if a control plane Machine is unhealthy.
// +kubebuilder:validation:MinProperties=1
type ControlPlaneClassHealthCheckRemediation struct {
	// triggerIf configures if remediations are triggered.
	// If this field is not set, remediations are always triggered.
	// +optional
	TriggerIf ControlPlaneClassHealthCheckRemediationTriggerIf `json:"triggerIf,omitempty,omitzero"`

	// templateRef is a reference to a remediation template
	// provided by an infrastructure provider.
	//
	// This field is completely optional, when filled, the MachineHealthCheck controller
	// creates a new object from the template referenced and hands off remediation of the machine to
	// a controller that lives outside of Cluster API.
	// +optional
	TemplateRef MachineHealthCheckRemediationTemplateReference `json:"templateRef,omitempty,omitzero"`
}

// ControlPlaneClassHealthCheckRemediationTriggerIf configures if remediations are triggered.
// +kubebuilder:validation:MinProperties=1
type ControlPlaneClassHealthCheckRemediationTriggerIf struct {
	// unhealthyLessThanOrEqualTo specifies that remediations are only triggered if the number of
	// unhealthy Machines is less than or equal to the configured value.
	// unhealthyInRange takes precedence if set.
	//
	// +optional
	UnhealthyLessThanOrEqualTo *intstr.IntOrString `json:"unhealthyLessThanOrEqualTo,omitempty"`

	// unhealthyInRange specifies that remediations are only triggered if the number of
	// unhealthy Machines is in the configured range.
	// Takes precedence over unhealthyLessThanOrEqualTo.
	// Eg. "[3-5]" - This means that remediation will be allowed only when:
	// (a) there are at least 3 unhealthy Machines (and)
	// (b) there are at most 5 unhealthy Machines
	//
	// +optional
	// +kubebuilder:validation:Pattern=^\[[0-9]+-[0-9]+\]$
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=32
	UnhealthyInRange string `json:"unhealthyInRange,omitempty"`
}

// ControlPlaneClassMachineDeletionSpec contains configuration options for Machine deletion.
// +kubebuilder:validation:MinProperties=1
type ControlPlaneClassMachineDeletionSpec struct {
	// nodeDrainTimeoutSeconds is the total amount of time that the controller will spend on draining a node.
	// The default value is 0, meaning that the node can be drained without any time limitations.
	// NOTE: nodeDrainTimeoutSeconds is different from `kubectl drain --timeout`
	// NOTE: This value can be overridden while defining a Cluster.Topology.
	// +optional
	// +kubebuilder:validation:Minimum=0
	NodeDrainTimeoutSeconds *int32 `json:"nodeDrainTimeoutSeconds,omitempty"`

	// nodeVolumeDetachTimeoutSeconds is the total amount of time that the controller will spend on waiting for all volumes
	// to be detached. The default value is 0, meaning that the volumes can be detached without any time limitations.
	// NOTE: This value can be overridden while defining a Cluster.Topology.
	// +optional
	// +kubebuilder:validation:Minimum=0
	NodeVolumeDetachTimeoutSeconds *int32 `json:"nodeVolumeDetachTimeoutSeconds,omitempty"`

	// nodeDeletionTimeoutSeconds defines how long the controller will attempt to delete the Node that the Machine
	// hosts after the Machine is marked for deletion. A duration of 0 will retry deletion indefinitely.
	// Defaults to 10 seconds.
	// NOTE: This value can be overridden while defining a Cluster.Topology.
	// +optional
	// +kubebuilder:validation:Minimum=0
	NodeDeletionTimeoutSeconds *int32 `json:"nodeDeletionTimeoutSeconds,omitempty"`
}

// ControlPlaneClassNamingSpec defines the naming strategy for control plane objects.
// +kubebuilder:validation:MinProperties=1
type ControlPlaneClassNamingSpec struct {
	// template defines the template to use for generating the name of the ControlPlane object.
	// If not defined, it will fallback to `{{ .cluster.name }}-{{ .random }}`.
	// If the templated string exceeds 63 characters, it will be trimmed to 58 characters and will
	// get concatenated with a random suffix of length 5.
	// The templating mechanism provides the following arguments:
	// * `.cluster.name`: The name of the cluster object.
	// * `.random`: A random alphanumeric string, without vowels, of length 5.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=1024
	Template string `json:"template,omitempty"`
}

// InfrastructureClassNamingSpec defines the naming strategy for infrastructure objects.
// +kubebuilder:validation:MinProperties=1
type InfrastructureClassNamingSpec struct {
	// template defines the template to use for generating the name of the Infrastructure object.
	// If not defined, it will fallback to `{{ .cluster.name }}-{{ .random }}`.
	// If the templated string exceeds 63 characters, it will be trimmed to 58 characters and will
	// get concatenated with a random suffix of length 5.
	// The templating mechanism provides the following arguments:
	// * `.cluster.name`: The name of the cluster object.
	// * `.random`: A random alphanumeric string, without vowels, of length 5.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=1024
	Template string `json:"template,omitempty"`
}

// WorkersClass is a collection of deployment classes.
// +kubebuilder:validation:MinProperties=1
type WorkersClass struct {
	// machineDeployments is a list of machine deployment classes that can be used to create
	// a set of worker nodes.
	// +optional
	// +listType=map
	// +listMapKey=class
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	MachineDeployments []MachineDeploymentClass `json:"machineDeployments,omitempty"`

	// machinePools is a list of machine pool classes that can be used to create
	// a set of worker nodes.
	// +optional
	// +listType=map
	// +listMapKey=class
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	MachinePools []MachinePoolClass `json:"machinePools,omitempty"`
}

// MachineDeploymentClass serves as a template to define a set of worker nodes of the cluster
// provisioned using the `ClusterClass`.
type MachineDeploymentClass struct {
	// metadata is the metadata applied to the MachineDeployment and the machines of the MachineDeployment.
	// At runtime this metadata is merged with the corresponding metadata from the topology.
	// +optional
	Metadata ObjectMeta `json:"metadata,omitempty,omitzero"`

	// class denotes a type of worker node present in the cluster,
	// this name MUST be unique within a ClusterClass and can be referenced
	// in the Cluster to create a managed MachineDeployment.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Class string `json:"class,omitempty"`

	// bootstrap contains the bootstrap template reference to be used
	// for the creation of worker Machines.
	// +required
	Bootstrap MachineDeploymentClassBootstrapTemplate `json:"bootstrap,omitempty,omitzero"`

	// infrastructure contains the infrastructure template reference to be used
	// for the creation of worker Machines.
	// +required
	Infrastructure MachineDeploymentClassInfrastructureTemplate `json:"infrastructure,omitempty,omitzero"`

	// healthCheck defines a MachineHealthCheck for this MachineDeploymentClass.
	// +optional
	HealthCheck MachineDeploymentClassHealthCheck `json:"healthCheck,omitempty,omitzero"`

	// failureDomain is the failure domain the machines will be created in.
	// Must match the name of a FailureDomain from the Cluster status.
	// NOTE: This value can be overridden while defining a Cluster.Topology using this MachineDeploymentClass.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	FailureDomain string `json:"failureDomain,omitempty"`

	// naming allows changing the naming pattern used when creating the MachineDeployment.
	// +optional
	Naming MachineDeploymentClassNamingSpec `json:"naming,omitempty,omitzero"`

	// deletion contains configuration options for Machine deletion.
	// +optional
	Deletion MachineDeploymentClassMachineDeletionSpec `json:"deletion,omitempty,omitzero"`

	// minReadySeconds is the minimum number of seconds for which a newly created machine should
	// be ready.
	// Defaults to 0 (machine will be considered available as soon as it
	// is ready)
	// NOTE: This value can be overridden while defining a Cluster.Topology using this MachineDeploymentClass.
	// +optional
	// +kubebuilder:validation:Minimum=0
	MinReadySeconds *int32 `json:"minReadySeconds,omitempty"`

	// readinessGates specifies additional conditions to include when evaluating Machine Ready condition.
	//
	// This field can be used e.g. to instruct the machine controller to include in the computation for Machine's ready
	// computation a condition, managed by an external controllers, reporting the status of special software/hardware installed on the Machine.
	//
	// NOTE: If a Cluster defines a custom list of readinessGates for a MachineDeployment using this MachineDeploymentClass,
	// such list overrides readinessGates defined in this field.
	// +optional
	// +listType=map
	// +listMapKey=conditionType
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=32
	ReadinessGates []MachineReadinessGate `json:"readinessGates,omitempty"`

	// rollout allows you to configure the behaviour of rolling updates to the MachineDeployment Machines.
	// It allows you to define the strategy used during rolling replacements.
	// +optional
	Rollout MachineDeploymentClassRolloutSpec `json:"rollout,omitempty,omitzero"`
}

// MachineDeploymentClassHealthCheck defines a MachineHealthCheck for MachineDeployment machines.
// +kubebuilder:validation:MinProperties=1
type MachineDeploymentClassHealthCheck struct {
	// checks are the checks that are used to evaluate if a Machine is healthy.
	//
	// Independent of this configuration the MachineHealthCheck controller will always
	// flag Machines with `cluster.x-k8s.io/remediate-machine` annotation and
	// Machines with deleted Nodes as unhealthy.
	//
	// Furthermore, if checks.nodeStartupTimeoutSeconds is not set it
	// is defaulted to 10 minutes and evaluated accordingly.
	//
	// +optional
	Checks MachineDeploymentClassHealthCheckChecks `json:"checks,omitempty,omitzero"`

	// remediation configures if and how remediations are triggered if a Machine is unhealthy.
	//
	// If remediation or remediation.triggerIf is not set,
	// remediation will always be triggered for unhealthy Machines.
	//
	// If remediation or remediation.templateRef is not set,
	// the OwnerRemediated condition will be set on unhealthy Machines to trigger remediation via
	// the owner of the Machines, for example a MachineSet or a KubeadmControlPlane.
	//
	// +optional
	Remediation MachineDeploymentClassHealthCheckRemediation `json:"remediation,omitempty,omitzero"`
}

// IsDefined returns true if one of checks and remediation are not zero.
func (m *MachineDeploymentClassHealthCheck) IsDefined() bool {
	return !reflect.ValueOf(m.Checks).IsZero() || !reflect.ValueOf(m.Remediation).IsZero()
}

// MachineDeploymentClassHealthCheckChecks are the checks that are used to evaluate if a MachineDeployment Machine is healthy.
// +kubebuilder:validation:MinProperties=1
type MachineDeploymentClassHealthCheckChecks struct {
	// nodeStartupTimeoutSeconds allows to set the maximum time for MachineHealthCheck
	// to consider a Machine unhealthy if a corresponding Node isn't associated
	// through a `Spec.ProviderID` field.
	//
	// The duration set in this field is compared to the greatest of:
	// - Cluster's infrastructure ready condition timestamp (if and when available)
	// - Control Plane's initialized condition timestamp (if and when available)
	// - Machine's infrastructure ready condition timestamp (if and when available)
	// - Machine's metadata creation timestamp
	//
	// Defaults to 10 minutes.
	// If you wish to disable this feature, set the value explicitly to 0.
	// +optional
	// +kubebuilder:validation:Minimum=0
	NodeStartupTimeoutSeconds *int32 `json:"nodeStartupTimeoutSeconds,omitempty"`

	// unhealthyNodeConditions contains a list of conditions that determine
	// whether a node is considered unhealthy. The conditions are combined in a
	// logical OR, i.e. if any of the conditions is met, the node is unhealthy.
	//
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	UnhealthyNodeConditions []UnhealthyNodeCondition `json:"unhealthyNodeConditions,omitempty"`

	// unhealthyMachineConditions contains a list of the machine conditions that determine
	// whether a machine is considered unhealthy.  The conditions are combined in a
	// logical OR, i.e. if any of the conditions is met, the machine is unhealthy.
	//
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	UnhealthyMachineConditions []UnhealthyMachineCondition `json:"unhealthyMachineConditions,omitempty"`
}

// MachineDeploymentClassHealthCheckRemediation configures if and how remediations are triggered if a MachineDeployment Machine is unhealthy.
// +kubebuilder:validation:MinProperties=1
type MachineDeploymentClassHealthCheckRemediation struct {
	// maxInFlight determines how many in flight remediations should happen at the same time.
	//
	// Remediation only happens on the MachineSet with the most current revision, while
	// older MachineSets (usually present during rollout operations) aren't allowed to remediate.
	//
	// Note: In general (independent of remediations), unhealthy machines are always
	// prioritized during scale down operations over healthy ones.
	//
	// MaxInFlight can be set to a fixed number or a percentage.
	// Example: when this is set to 20%, the MachineSet controller deletes at most 20% of
	// the desired replicas.
	//
	// If not set, remediation is limited to all machines (bounded by replicas)
	// under the active MachineSet's management.
	//
	// +optional
	MaxInFlight *intstr.IntOrString `json:"maxInFlight,omitempty"`

	// triggerIf configures if remediations are triggered.
	// If this field is not set, remediations are always triggered.
	// +optional
	TriggerIf MachineDeploymentClassHealthCheckRemediationTriggerIf `json:"triggerIf,omitempty,omitzero"`

	// templateRef is a reference to a remediation template
	// provided by an infrastructure provider.
	//
	// This field is completely optional, when filled, the MachineHealthCheck controller
	// creates a new object from the template referenced and hands off remediation of the machine to
	// a controller that lives outside of Cluster API.
	// +optional
	TemplateRef MachineHealthCheckRemediationTemplateReference `json:"templateRef,omitempty,omitzero"`
}

// MachineDeploymentClassHealthCheckRemediationTriggerIf configures if remediations are triggered.
// +kubebuilder:validation:MinProperties=1
type MachineDeploymentClassHealthCheckRemediationTriggerIf struct {
	// unhealthyLessThanOrEqualTo specifies that remediations are only triggered if the number of
	// unhealthy Machines is less than or equal to the configured value.
	// unhealthyInRange takes precedence if set.
	//
	// +optional
	UnhealthyLessThanOrEqualTo *intstr.IntOrString `json:"unhealthyLessThanOrEqualTo,omitempty"`

	// unhealthyInRange specifies that remediations are only triggered if the number of
	// unhealthy Machines is in the configured range.
	// Takes precedence over unhealthyLessThanOrEqualTo.
	// Eg. "[3-5]" - This means that remediation will be allowed only when:
	// (a) there are at least 3 unhealthy Machines (and)
	// (b) there are at most 5 unhealthy Machines
	//
	// +optional
	// +kubebuilder:validation:Pattern=^\[[0-9]+-[0-9]+\]$
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=32
	UnhealthyInRange string `json:"unhealthyInRange,omitempty"`
}

// MachineDeploymentClassMachineDeletionSpec contains configuration options for Machine deletion.
// +kubebuilder:validation:MinProperties=1
type MachineDeploymentClassMachineDeletionSpec struct {
	// order defines the order in which Machines are deleted when downscaling.
	// Defaults to "Random".  Valid values are "Random, "Newest", "Oldest"
	// +optional
	Order MachineSetDeletionOrder `json:"order,omitempty"`

	// nodeDrainTimeoutSeconds is the total amount of time that the controller will spend on draining a node.
	// The default value is 0, meaning that the node can be drained without any time limitations.
	// NOTE: nodeDrainTimeoutSeconds is different from `kubectl drain --timeout`
	// NOTE: This value can be overridden while defining a Cluster.Topology using this MachineDeploymentClass.
	// +optional
	// +kubebuilder:validation:Minimum=0
	NodeDrainTimeoutSeconds *int32 `json:"nodeDrainTimeoutSeconds,omitempty"`

	// nodeVolumeDetachTimeoutSeconds is the total amount of time that the controller will spend on waiting for all volumes
	// to be detached. The default value is 0, meaning that the volumes can be detached without any time limitations.
	// NOTE: This value can be overridden while defining a Cluster.Topology using this MachineDeploymentClass.
	// +optional
	// +kubebuilder:validation:Minimum=0
	NodeVolumeDetachTimeoutSeconds *int32 `json:"nodeVolumeDetachTimeoutSeconds,omitempty"`

	// nodeDeletionTimeoutSeconds defines how long the controller will attempt to delete the Node that the Machine
	// hosts after the Machine is marked for deletion. A duration of 0 will retry deletion indefinitely.
	// Defaults to 10 seconds.
	// NOTE: This value can be overridden while defining a Cluster.Topology using this MachineDeploymentClass.
	// +optional
	// +kubebuilder:validation:Minimum=0
	NodeDeletionTimeoutSeconds *int32 `json:"nodeDeletionTimeoutSeconds,omitempty"`
}

// MachineDeploymentClassNamingSpec defines the naming strategy for machine deployment objects.
// +kubebuilder:validation:MinProperties=1
type MachineDeploymentClassNamingSpec struct {
	// template defines the template to use for generating the name of the MachineDeployment object.
	// If not defined, it will fallback to `{{ .cluster.name }}-{{ .machineDeployment.topologyName }}-{{ .random }}`.
	// If the templated string exceeds 63 characters, it will be trimmed to 58 characters and will
	// get concatenated with a random suffix of length 5.
	// The templating mechanism provides the following arguments:
	// * `.cluster.name`: The name of the cluster object.
	// * `.random`: A random alphanumeric string, without vowels, of length 5.
	// * `.machineDeployment.topologyName`: The name of the MachineDeployment topology (Cluster.spec.topology.workers.machineDeployments[].name).
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=1024
	Template string `json:"template,omitempty"`
}

// MachineDeploymentClassRolloutSpec defines the rollout behavior.
// +kubebuilder:validation:MinProperties=1
type MachineDeploymentClassRolloutSpec struct {
	// strategy specifies how to roll out control plane Machines.
	// +optional
	Strategy MachineDeploymentClassRolloutStrategy `json:"strategy,omitempty,omitzero"`
}

// MachineDeploymentClassRolloutStrategy describes how to replace existing machines
// with new ones.
// +kubebuilder:validation:MinProperties=1
type MachineDeploymentClassRolloutStrategy struct {
	// type of rollout. Allowed values are RollingUpdate and OnDelete.
	// Default is RollingUpdate.
	// +required
	Type MachineDeploymentRolloutStrategyType `json:"type,omitempty"`

	// rollingUpdate is the rolling update config params. Present only if
	// type = RollingUpdate.
	// +optional
	RollingUpdate MachineDeploymentClassRolloutStrategyRollingUpdate `json:"rollingUpdate,omitempty,omitzero"`
}

// MachineDeploymentClassRolloutStrategyRollingUpdate is used to control the desired behavior of rolling update.
// +kubebuilder:validation:MinProperties=1
type MachineDeploymentClassRolloutStrategyRollingUpdate struct {
	// maxUnavailable is the maximum number of machines that can be unavailable during the update.
	// Value can be an absolute number (ex: 5) or a percentage of desired
	// machines (ex: 10%).
	// Absolute number is calculated from percentage by rounding down.
	// This can not be 0 if MaxSurge is 0.
	// Defaults to 0.
	// Example: when this is set to 30%, the old MachineSet can be scaled
	// down to 70% of desired machines immediately when the rolling update
	// starts. Once new machines are ready, old MachineSet can be scaled
	// down further, followed by scaling up the new MachineSet, ensuring
	// that the total number of machines available at all times
	// during the update is at least 70% of desired machines.
	// +optional
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`

	// maxSurge is the maximum number of machines that can be scheduled above the
	// desired number of machines.
	// Value can be an absolute number (ex: 5) or a percentage of
	// desired machines (ex: 10%).
	// This can not be 0 if MaxUnavailable is 0.
	// Absolute number is calculated from percentage by rounding up.
	// Defaults to 1.
	// Example: when this is set to 30%, the new MachineSet can be scaled
	// up immediately when the rolling update starts, such that the total
	// number of old and new machines do not exceed 130% of desired
	// machines. Once old machines have been killed, new MachineSet can
	// be scaled up further, ensuring that total number of machines running
	// at any time during the update is at most 130% of desired machines.
	// +optional
	MaxSurge *intstr.IntOrString `json:"maxSurge,omitempty"`
}

// MachinePoolClass serves as a template to define a pool of worker nodes of the cluster
// provisioned using `ClusterClass`.
type MachinePoolClass struct {
	// metadata is the metadata applied to the MachinePool.
	// At runtime this metadata is merged with the corresponding metadata from the topology.
	// +optional
	Metadata ObjectMeta `json:"metadata,omitempty,omitzero"`

	// class denotes a type of machine pool present in the cluster,
	// this name MUST be unique within a ClusterClass and can be referenced
	// in the Cluster to create a managed MachinePool.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Class string `json:"class,omitempty"`

	// bootstrap contains the bootstrap template reference to be used
	// for the creation of the Machines in the MachinePool.
	// +required
	Bootstrap MachinePoolClassBootstrapTemplate `json:"bootstrap,omitempty,omitzero"`

	// infrastructure contains the infrastructure template reference to be used
	// for the creation of the MachinePool.
	// +required
	Infrastructure MachinePoolClassInfrastructureTemplate `json:"infrastructure,omitempty,omitzero"`

	// failureDomains is the list of failure domains the MachinePool should be attached to.
	// Must match a key in the FailureDomains map stored on the cluster object.
	// NOTE: This value can be overridden while defining a Cluster.Topology using this MachinePoolClass.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MaxItems=100
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=256
	FailureDomains []string `json:"failureDomains,omitempty"`

	// naming allows changing the naming pattern used when creating the MachinePool.
	// +optional
	Naming MachinePoolClassNamingSpec `json:"naming,omitempty,omitzero"`

	// deletion contains configuration options for Machine deletion.
	// +optional
	Deletion MachinePoolClassMachineDeletionSpec `json:"deletion,omitempty,omitzero"`

	// minReadySeconds is the minimum number of seconds for which a newly created machine pool should
	// be ready.
	// Defaults to 0 (machine will be considered available as soon as it
	// is ready)
	// NOTE: This value can be overridden while defining a Cluster.Topology using this MachinePoolClass.
	// +optional
	// +kubebuilder:validation:Minimum=0
	MinReadySeconds *int32 `json:"minReadySeconds,omitempty"`
}

// MachinePoolClassMachineDeletionSpec contains configuration options for Machine deletion.
// +kubebuilder:validation:MinProperties=1
type MachinePoolClassMachineDeletionSpec struct {
	// nodeDrainTimeoutSeconds is the total amount of time that the controller will spend on draining a node.
	// The default value is 0, meaning that the node can be drained without any time limitations.
	// NOTE: nodeDrainTimeoutSeconds is different from `kubectl drain --timeout`
	// NOTE: This value can be overridden while defining a Cluster.Topology using this MachinePoolClass.
	// +optional
	// +kubebuilder:validation:Minimum=0
	NodeDrainTimeoutSeconds *int32 `json:"nodeDrainTimeoutSeconds,omitempty"`

	// nodeVolumeDetachTimeoutSeconds is the total amount of time that the controller will spend on waiting for all volumes
	// to be detached. The default value is 0, meaning that the volumes can be detached without any time limitations.
	// NOTE: This value can be overridden while defining a Cluster.Topology using this MachinePoolClass.
	// +optional
	// +kubebuilder:validation:Minimum=0
	NodeVolumeDetachTimeoutSeconds *int32 `json:"nodeVolumeDetachTimeoutSeconds,omitempty"`

	// nodeDeletionTimeoutSeconds defines how long the controller will attempt to delete the Node that the Machine
	// hosts after the Machine Pool is marked for deletion. A duration of 0 will retry deletion indefinitely.
	// Defaults to 10 seconds.
	// NOTE: This value can be overridden while defining a Cluster.Topology using this MachinePoolClass.
	// +optional
	// +kubebuilder:validation:Minimum=0
	NodeDeletionTimeoutSeconds *int32 `json:"nodeDeletionTimeoutSeconds,omitempty"`
}

// MachinePoolClassNamingSpec defines the naming strategy for MachinePool objects.
// +kubebuilder:validation:MinProperties=1
type MachinePoolClassNamingSpec struct {
	// template defines the template to use for generating the name of the MachinePool object.
	// If not defined, it will fallback to `{{ .cluster.name }}-{{ .machinePool.topologyName }}-{{ .random }}`.
	// If the templated string exceeds 63 characters, it will be trimmed to 58 characters and will
	// get concatenated with a random suffix of length 5.
	// The templating mechanism provides the following arguments:
	// * `.cluster.name`: The name of the cluster object.
	// * `.random`: A random alphanumeric string, without vowels, of length 5.
	// * `.machinePool.topologyName`: The name of the MachinePool topology (Cluster.spec.topology.workers.machinePools[].name).
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=1024
	Template string `json:"template,omitempty"`
}

// ClusterClassVariable defines a variable which can
// be configured in the Cluster topology and used in patches.
type ClusterClassVariable struct {
	// name of the variable.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Name string `json:"name,omitempty"`

	// required specifies if the variable is required.
	// Note: this applies to the variable as a whole and thus the
	// top-level object defined in the schema. If nested fields are
	// required, this will be specified inside the schema.
	// +required
	Required *bool `json:"required,omitempty"`

	// deprecatedV1Beta1Metadata is the metadata of a variable.
	// It can be used to add additional data for higher level tools to
	// a ClusterClassVariable.
	//
	// Deprecated: This field is deprecated and will be removed when support for v1beta1 will be dropped. Please use XMetadata in JSONSchemaProps instead.
	//
	// +optional
	DeprecatedV1Beta1Metadata ClusterClassVariableMetadata `json:"deprecatedV1Beta1Metadata,omitempty,omitzero"`

	// schema defines the schema of the variable.
	// +required
	Schema VariableSchema `json:"schema,omitempty,omitzero"`
}

// ClusterClassVariableMetadata is the metadata of a variable.
// It can be used to add additional data for higher level tools to
// a ClusterClassVariable.
//
// Deprecated: This struct is deprecated and is going to be removed in the next apiVersion.
// +kubebuilder:validation:MinProperties=1
type ClusterClassVariableMetadata struct {
	// labels is a map of string keys and values that can be used to organize and categorize
	// (scope and select) variables.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// annotations is an unstructured key value map that can be used to store and
	// retrieve arbitrary metadata.
	// They are not queryable.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// VariableSchema defines the schema of a variable.
type VariableSchema struct {
	// openAPIV3Schema defines the schema of a variable via OpenAPI v3
	// schema. The schema is a subset of the schema used in
	// Kubernetes CRDs.
	// +required
	OpenAPIV3Schema JSONSchemaProps `json:"openAPIV3Schema,omitempty,omitzero"`
}

// Adapted from https://github.com/kubernetes/apiextensions-apiserver/blob/v0.28.5/pkg/apis/apiextensions/v1/types_jsonschema.go#L40

// JSONSchemaProps is a JSON-Schema following Specification Draft 4 (http://json-schema.org/).
// This struct has been initially copied from apiextensionsv1.JSONSchemaProps, but all fields
// which are not supported in CAPI have been removed.
// +kubebuilder:validation:MinProperties=1
type JSONSchemaProps struct {
	// description is a human-readable description of this variable.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=4096
	Description string `json:"description,omitempty"`

	// example is an example for this variable.
	// +optional
	Example *apiextensionsv1.JSON `json:"example,omitempty"`

	// type is the type of the variable.
	// Valid values are: object, array, string, integer, number or boolean.
	// +optional
	// +kubebuilder:validation:Enum=object;array;string;integer;number;boolean
	Type string `json:"type,omitempty"`

	// properties specifies fields of an object.
	// NOTE: Can only be set if type is object.
	// NOTE: Properties is mutually exclusive with AdditionalProperties.
	// NOTE: This field uses PreserveUnknownFields and Schemaless,
	// because recursive validation is not possible.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Properties map[string]JSONSchemaProps `json:"properties,omitempty"`

	// additionalProperties specifies the schema of values in a map (keys are always strings).
	// NOTE: Can only be set if type is object.
	// NOTE: AdditionalProperties is mutually exclusive with Properties.
	// NOTE: This field uses PreserveUnknownFields and Schemaless,
	// because recursive validation is not possible.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	AdditionalProperties *JSONSchemaProps `json:"additionalProperties,omitempty"`

	// maxProperties is the maximum amount of entries in a map or properties in an object.
	// NOTE: Can only be set if type is object.
	// +optional
	MaxProperties *int64 `json:"maxProperties,omitempty"`

	// minProperties is the minimum amount of entries in a map or properties in an object.
	// NOTE: Can only be set if type is object.
	// +optional
	MinProperties *int64 `json:"minProperties,omitempty"`

	// required specifies which fields of an object are required.
	// NOTE: Can only be set if type is object.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=1000
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=256
	Required []string `json:"required,omitempty"`

	// items specifies fields of an array.
	// NOTE: Can only be set if type is array.
	// NOTE: This field uses PreserveUnknownFields and Schemaless,
	// because recursive validation is not possible.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Items *JSONSchemaProps `json:"items,omitempty"`

	// maxItems is the max length of an array variable.
	// NOTE: Can only be set if type is array.
	// +optional
	MaxItems *int64 `json:"maxItems,omitempty"`

	// minItems is the min length of an array variable.
	// NOTE: Can only be set if type is array.
	// +optional
	MinItems *int64 `json:"minItems,omitempty"`

	// uniqueItems specifies if items in an array must be unique.
	// NOTE: Can only be set if type is array.
	// +optional
	UniqueItems *bool `json:"uniqueItems,omitempty"`

	// format is an OpenAPI v3 format string. Unknown formats are ignored.
	// For a list of supported formats please see: (of the k8s.io/apiextensions-apiserver version we're currently using)
	// https://github.com/kubernetes/apiextensions-apiserver/blob/master/pkg/apiserver/validation/formats.go
	// NOTE: Can only be set if type is string.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=32
	Format string `json:"format,omitempty"`

	// maxLength is the max length of a string variable.
	// NOTE: Can only be set if type is string.
	// +optional
	MaxLength *int64 `json:"maxLength,omitempty"`

	// minLength is the min length of a string variable.
	// NOTE: Can only be set if type is string.
	// +optional
	MinLength *int64 `json:"minLength,omitempty"`

	// pattern is the regex which a string variable must match.
	// NOTE: Can only be set if type is string.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	Pattern string `json:"pattern,omitempty"`

	// maximum is the maximum of an integer or number variable.
	// If ExclusiveMaximum is false, the variable is valid if it is lower than, or equal to, the value of Maximum.
	// If ExclusiveMaximum is true, the variable is valid if it is strictly lower than the value of Maximum.
	// NOTE: Can only be set if type is integer or number.
	// +optional
	Maximum *int64 `json:"maximum,omitempty"`

	// exclusiveMaximum specifies if the Maximum is exclusive.
	// NOTE: Can only be set if type is integer or number.
	// +optional
	ExclusiveMaximum *bool `json:"exclusiveMaximum,omitempty"`

	// minimum is the minimum of an integer or number variable.
	// If ExclusiveMinimum is false, the variable is valid if it is greater than, or equal to, the value of Minimum.
	// If ExclusiveMinimum is true, the variable is valid if it is strictly greater than the value of Minimum.
	// NOTE: Can only be set if type is integer or number.
	// +optional
	Minimum *int64 `json:"minimum,omitempty"`

	// exclusiveMinimum specifies if the Minimum is exclusive.
	// NOTE: Can only be set if type is integer or number.
	// +optional
	ExclusiveMinimum *bool `json:"exclusiveMinimum,omitempty"`

	// x-kubernetes-preserve-unknown-fields allows setting fields in a variable object
	// which are not defined in the variable schema. This affects fields recursively,
	// except if nested properties or additionalProperties are specified in the schema.
	// +optional
	XPreserveUnknownFields *bool `json:"x-kubernetes-preserve-unknown-fields,omitempty"`

	// enum is the list of valid values of the variable.
	// NOTE: Can be set for all types.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MaxItems=100
	Enum []apiextensionsv1.JSON `json:"enum,omitempty"`

	// default is the default value of the variable.
	// NOTE: Can be set for all types.
	// +optional
	Default *apiextensionsv1.JSON `json:"default,omitempty"`

	// x-kubernetes-validations describes a list of validation rules written in the CEL expression language.
	// +optional
	// +listType=map
	// +listMapKey=rule
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	XValidations []ValidationRule `json:"x-kubernetes-validations,omitempty"`

	// x-metadata is the metadata of a variable or a nested field within a variable.
	// It can be used to add additional data for higher level tools.
	// +optional
	XMetadata VariableSchemaMetadata `json:"x-metadata,omitempty,omitzero"`

	// x-kubernetes-int-or-string specifies that this value is
	// either an integer or a string. If this is true, an empty
	// type is allowed and type as child of anyOf is permitted
	// if following one of the following patterns:
	//
	// 1) anyOf:
	//    - type: integer
	//    - type: string
	// 2) allOf:
	//    - anyOf:
	//      - type: integer
	//      - type: string
	//    - ... zero or more
	// +optional
	XIntOrString *bool `json:"x-kubernetes-int-or-string,omitempty"`

	// allOf specifies that the variable must validate against all of the subschemas in the array.
	// NOTE: This field uses PreserveUnknownFields and Schemaless,
	// because recursive validation is not possible.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	AllOf []JSONSchemaProps `json:"allOf,omitempty"`

	// oneOf specifies that the variable must validate against exactly one of the subschemas in the array.
	// NOTE: This field uses PreserveUnknownFields and Schemaless,
	// because recursive validation is not possible.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	OneOf []JSONSchemaProps `json:"oneOf,omitempty"`

	// anyOf specifies that the variable must validate against one or more of the subschemas in the array.
	// NOTE: This field uses PreserveUnknownFields and Schemaless,
	// because recursive validation is not possible.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	AnyOf []JSONSchemaProps `json:"anyOf,omitempty"`

	// not specifies that the variable must not validate against the subschema.
	// NOTE: This field uses PreserveUnknownFields and Schemaless,
	// because recursive validation is not possible.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Not *JSONSchemaProps `json:"not,omitempty"`
}

// VariableSchemaMetadata is the metadata of a variable or a nested field within a variable.
// It can be used to add additional data for higher level tools.
// +kubebuilder:validation:MinProperties=1
type VariableSchemaMetadata struct {
	// labels is a map of string keys and values that can be used to organize and categorize
	// (scope and select) variables.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// annotations is an unstructured key value map that can be used to store and
	// retrieve arbitrary metadata.
	// They are not queryable.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// ValidationRule describes a validation rule written in the CEL expression language.
type ValidationRule struct {
	// rule represents the expression which will be evaluated by CEL.
	// ref: https://github.com/google/cel-spec
	// The Rule is scoped to the location of the x-kubernetes-validations extension in the schema.
	// The `self` variable in the CEL expression is bound to the scoped value.
	// If the Rule is scoped to an object with properties, the accessible properties of the object are field selectable
	// via `self.field` and field presence can be checked via `has(self.field)`.
	// If the Rule is scoped to an object with additionalProperties (i.e. a map) the value of the map
	// are accessible via `self[mapKey]`, map containment can be checked via `mapKey in self` and all entries of the map
	// are accessible via CEL macros and functions such as `self.all(...)`.
	// If the Rule is scoped to an array, the elements of the array are accessible via `self[i]` and also by macros and
	// functions.
	// If the Rule is scoped to a scalar, `self` is bound to the scalar value.
	// Examples:
	// - Rule scoped to a map of objects: {"rule": "self.components['Widget'].priority < 10"}
	// - Rule scoped to a list of integers: {"rule": "self.values.all(value, value >= 0 && value < 100)"}
	// - Rule scoped to a string value: {"rule": "self.startsWith('kube')"}
	//
	// Unknown data preserved in custom resources via x-kubernetes-preserve-unknown-fields is not accessible in CEL
	// expressions. This includes:
	// - Unknown field values that are preserved by object schemas with x-kubernetes-preserve-unknown-fields.
	// - Object properties where the property schema is of an "unknown type". An "unknown type" is recursively defined as:
	//   - A schema with no type and x-kubernetes-preserve-unknown-fields set to true
	//   - An array where the items schema is of an "unknown type"
	//   - An object where the additionalProperties schema is of an "unknown type"
	//
	// Only property names of the form `[a-zA-Z_.-/][a-zA-Z0-9_.-/]*` are accessible.
	// Accessible property names are escaped according to the following rules when accessed in the expression:
	// - '__' escapes to '__underscores__'
	// - '.' escapes to '__dot__'
	// - '-' escapes to '__dash__'
	// - '/' escapes to '__slash__'
	// - Property names that exactly match a CEL RESERVED keyword escape to '__{keyword}__'. The keywords are:
	//	  "true", "false", "null", "in", "as", "break", "const", "continue", "else", "for", "function", "if",
	//	  "import", "let", "loop", "package", "namespace", "return".
	// Examples:
	//   - Rule accessing a property named "namespace": {"rule": "self.__namespace__ > 0"}
	//   - Rule accessing a property named "x-prop": {"rule": "self.x__dash__prop > 0"}
	//   - Rule accessing a property named "redact__d": {"rule": "self.redact__underscores__d > 0"}
	//
	//
	// If `rule` makes use of the `oldSelf` variable it is implicitly a
	// `transition rule`.
	//
	// By default, the `oldSelf` variable is the same type as `self`.
	//
	// Transition rules by default are applied only on UPDATE requests and are
	// skipped if an old value could not be found.
	//
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=4096
	Rule string `json:"rule,omitempty"`
	// message represents the message displayed when validation fails. The message is required if the Rule contains
	// line breaks. The message must not contain line breaks.
	// If unset, the message is "failed rule: {Rule}".
	// e.g. "must be a URL with the host matching spec.host"
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	Message string `json:"message,omitempty"`
	// messageExpression declares a CEL expression that evaluates to the validation failure message that is returned when this rule fails.
	// Since messageExpression is used as a failure message, it must evaluate to a string.
	// If both message and messageExpression are present on a rule, then messageExpression will be used if validation
	// fails. If messageExpression results in a runtime error, the validation failure message is produced
	// as if the messageExpression field were unset. If messageExpression evaluates to an empty string, a string with only spaces, or a string
	// that contains line breaks, then the validation failure message will also be produced as if the messageExpression field were unset.
	// messageExpression has access to all the same variables as the rule; the only difference is the return type.
	// Example:
	// "x must be less than max ("+string(self.max)+")"
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=1024
	MessageExpression string `json:"messageExpression,omitempty"`
	// reason provides a machine-readable validation failure reason that is returned to the caller when a request fails this validation rule.
	// The currently supported reasons are: "FieldValueInvalid", "FieldValueForbidden", "FieldValueRequired", "FieldValueDuplicate".
	// If not set, default to use "FieldValueInvalid".
	// All future added reasons must be accepted by clients when reading this value and unknown reasons should be treated as FieldValueInvalid.
	// +optional
	// +kubebuilder:validation:Enum=FieldValueInvalid;FieldValueForbidden;FieldValueRequired;FieldValueDuplicate
	// +kubebuilder:default=FieldValueInvalid
	// +default=ref(sigs.k8s.io/cluster-api/api/core/v1beta2.FieldValueInvalid)
	Reason FieldValueErrorReason `json:"reason,omitempty"`
	// fieldPath represents the field path returned when the validation fails.
	// It must be a relative JSON path (i.e. with array notation) scoped to the location of this x-kubernetes-validations extension in the schema and refer to an existing field.
	// e.g. when validation checks if a specific attribute `foo` under a map `testMap`, the fieldPath could be set to `.testMap.foo`
	// If the validation checks two lists must have unique attributes, the fieldPath could be set to either of the list: e.g. `.testList`
	// It does not support list numeric index.
	// It supports child operation to refer to an existing field currently. Refer to [JSONPath support in Kubernetes](https://kubernetes.io/docs/reference/kubectl/jsonpath/) for more info.
	// Numeric index of array is not supported.
	// For field name which contains special characters, use `['specialName']` to refer the field name.
	// e.g. for attribute `foo.34$` appears in a list `testList`, the fieldPath could be set to `.testList['foo.34$']`
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	FieldPath string `json:"fieldPath,omitempty"`
}

// FieldValueErrorReason is a machine-readable value providing more detail about why a field failed the validation.
type FieldValueErrorReason string

const (
	// FieldValueRequired is used to report required values that are not
	// provided (e.g. empty strings, null values, or empty arrays).
	FieldValueRequired FieldValueErrorReason = "FieldValueRequired"
	// FieldValueDuplicate is used to report collisions of values that must be
	// unique (e.g. unique IDs).
	FieldValueDuplicate FieldValueErrorReason = "FieldValueDuplicate"
	// FieldValueInvalid is used to report malformed values (e.g. failed regex
	// match, too long, out of bounds).
	FieldValueInvalid FieldValueErrorReason = "FieldValueInvalid"
	// FieldValueForbidden is used to report valid (as per formatting rules)
	// values which would be accepted under some conditions, but which are not
	// permitted by the current conditions (such as security policy).
	FieldValueForbidden FieldValueErrorReason = "FieldValueForbidden"
)

// ClusterClassPatch defines a patch which is applied to customize the referenced templates.
type ClusterClassPatch struct {
	// name of the patch.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Name string `json:"name,omitempty"`

	// description is a human-readable description of this patch.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=1024
	Description string `json:"description,omitempty"`

	// enabledIf is a Go template to be used to calculate if a patch should be enabled.
	// It can reference variables defined in .spec.variables and builtin variables.
	// The patch will be enabled if the template evaluates to `true`, otherwise it will
	// be disabled.
	// If EnabledIf is not set, the patch will be enabled per default.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	EnabledIf string `json:"enabledIf,omitempty"`

	// definitions define inline patches.
	// Note: Patches will be applied in the order of the array.
	// Note: Exactly one of Definitions or External must be set.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MaxItems=100
	Definitions []PatchDefinition `json:"definitions,omitempty"`

	// external defines an external patch.
	// Note: Exactly one of Definitions or External must be set.
	// +optional
	External *ExternalPatchDefinition `json:"external,omitempty"`
}

// ClusterClassUpgrade defines the upgrade configuration for clusters using the ClusterClass.
// +kubebuilder:validation:MinProperties=1
type ClusterClassUpgrade struct {
	// external defines external runtime extensions for upgrade operations.
	// +optional
	External ClusterClassUpgradeExternal `json:"external,omitempty,omitzero"`
}

// ClusterClassUpgradeExternal defines external runtime extensions for upgrade operations.
// +kubebuilder:validation:MinProperties=1
type ClusterClassUpgradeExternal struct {
	// generateUpgradePlanExtension references an extension which is called to generate upgrade plan.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	GenerateUpgradePlanExtension string `json:"generateUpgradePlanExtension,omitempty"`
}

// PatchDefinition defines a patch which is applied to customize the referenced templates.
type PatchDefinition struct {
	// selector defines on which templates the patch should be applied.
	// +required
	Selector PatchSelector `json:"selector,omitempty,omitzero"`

	// jsonPatches defines the patches which should be applied on the templates
	// matching the selector.
	// Note: Patches will be applied in the order of the array.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	// +required
	// +listType=atomic
	JSONPatches []JSONPatch `json:"jsonPatches,omitempty"`
}

// PatchSelector defines on which templates the patch should be applied.
// Note: Matching on APIVersion and Kind is mandatory, to enforce that the patches are
// written for the correct version. The version of the references in the ClusterClass may
// be automatically updated during reconciliation if there is a newer version for the same contract.
// Note: The results of selection based on the individual fields are ANDed.
type PatchSelector struct {
	// apiVersion filters templates by apiVersion.
	// apiVersion must be fully qualified domain name followed by / and a version.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=317
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*\/[a-z]([-a-z0-9]*[a-z0-9])?$`
	APIVersion string `json:"apiVersion,omitempty"`

	// kind filters templates by kind.
	// kind must consist of alphanumeric characters or '-', start with an alphabetic character, and end with an alphanumeric character.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-zA-Z]([-a-zA-Z0-9]*[a-zA-Z0-9])?$`
	Kind string `json:"kind,omitempty"`

	// matchResources selects templates based on where they are referenced.
	// +required
	MatchResources PatchSelectorMatch `json:"matchResources,omitempty,omitzero"`
}

// PatchSelectorMatch selects templates based on where they are referenced.
// Note: The selector must match at least one template.
// Note: The results of selection based on the individual fields are ORed.
// +kubebuilder:validation:MinProperties=1
type PatchSelectorMatch struct {
	// controlPlane selects templates referenced in .spec.ControlPlane.
	// Note: this will match the controlPlane and also the controlPlane
	// machineInfrastructure (depending on the kind and apiVersion).
	// +optional
	ControlPlane *bool `json:"controlPlane,omitempty"`

	// infrastructureCluster selects templates referenced in .spec.infrastructure.
	// +optional
	InfrastructureCluster *bool `json:"infrastructureCluster,omitempty"`

	// machineDeploymentClass selects templates referenced in specific MachineDeploymentClasses in
	// .spec.workers.machineDeployments.
	// +optional
	MachineDeploymentClass *PatchSelectorMatchMachineDeploymentClass `json:"machineDeploymentClass,omitempty"`

	// machinePoolClass selects templates referenced in specific MachinePoolClasses in
	// .spec.workers.machinePools.
	// +optional
	MachinePoolClass *PatchSelectorMatchMachinePoolClass `json:"machinePoolClass,omitempty"`
}

// PatchSelectorMatchMachineDeploymentClass selects templates referenced
// in specific MachineDeploymentClasses in .spec.workers.machineDeployments.
type PatchSelectorMatchMachineDeploymentClass struct {
	// names selects templates by class names.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MaxItems=100
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=256
	Names []string `json:"names,omitempty"`
}

// PatchSelectorMatchMachinePoolClass selects templates referenced
// in specific MachinePoolClasses in .spec.workers.machinePools.
type PatchSelectorMatchMachinePoolClass struct {
	// names selects templates by class names.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MaxItems=100
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=256
	Names []string `json:"names,omitempty"`
}

// JSONPatch defines a JSON patch.
type JSONPatch struct {
	// op defines the operation of the patch.
	// Note: Only `add`, `replace` and `remove` are supported.
	// +required
	// +kubebuilder:validation:Enum=add;replace;remove
	Op string `json:"op,omitempty"`

	// path defines the path of the patch.
	// Note: Only the spec of a template can be patched, thus the path has to start with /spec/.
	// Note: For now the only allowed array modifications are `append` and `prepend`, i.e.:
	// * for op: `add`: only index 0 (prepend) and - (append) are allowed
	// * for op: `replace` or `remove`: no indexes are allowed
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	Path string `json:"path,omitempty"`

	// value defines the value of the patch.
	// Note: Either Value or ValueFrom is required for add and replace
	// operations. Only one of them is allowed to be set at the same time.
	// Note: We have to use apiextensionsv1.JSON instead of our JSON type,
	// because controller-tools has a hard-coded schema for apiextensionsv1.JSON
	// which cannot be produced by another type (unset type field).
	// Ref: https://github.com/kubernetes-sigs/controller-tools/blob/d0e03a142d0ecdd5491593e941ee1d6b5d91dba6/pkg/crd/known_types.go#L106-L111
	// +optional
	Value *apiextensionsv1.JSON `json:"value,omitempty"`

	// valueFrom defines the value of the patch.
	// Note: Either Value or ValueFrom is required for add and replace
	// operations. Only one of them is allowed to be set at the same time.
	// +optional
	ValueFrom *JSONPatchValue `json:"valueFrom,omitempty"`
}

// JSONPatchValue defines the value of a patch.
// Note: Only one of the fields is allowed to be set at the same time.
type JSONPatchValue struct {
	// variable is the variable to be used as value.
	// Variable can be one of the variables defined in .spec.variables or a builtin variable.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Variable string `json:"variable,omitempty"`

	// template is the Go template to be used to calculate the value.
	// A template can reference variables defined in .spec.variables and builtin variables.
	// Note: The template must evaluate to a valid YAML or JSON value.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=10240
	Template string `json:"template,omitempty"`
}

// ExternalPatchDefinition defines an external patch.
// Note: At least one of GeneratePatchesExtension or ValidateTopologyExtension must be set.
type ExternalPatchDefinition struct {
	// generatePatchesExtension references an extension which is called to generate patches.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	GeneratePatchesExtension string `json:"generatePatchesExtension,omitempty"`

	// validateTopologyExtension references an extension which is called to validate the topology.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	ValidateTopologyExtension string `json:"validateTopologyExtension,omitempty"`

	// discoverVariablesExtension references an extension which is called to discover variables.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	DiscoverVariablesExtension string `json:"discoverVariablesExtension,omitempty"`

	// settings defines key value pairs to be passed to the extensions.
	// Values defined here take precedence over the values defined in the
	// corresponding ExtensionConfig.
	// +optional
	Settings map[string]string `json:"settings,omitempty"`
}

// ControlPlaneClassMachineInfrastructureTemplate defines the template for a MachineInfrastructure of a ControlPlane.
type ControlPlaneClassMachineInfrastructureTemplate struct {
	// templateRef is a required reference to the template for a MachineInfrastructure of a ControlPlane.
	// +required
	TemplateRef ClusterClassTemplateReference `json:"templateRef,omitempty,omitzero"`
}

// MachineDeploymentClassBootstrapTemplate defines the BootstrapTemplate for a MachineDeployment.
type MachineDeploymentClassBootstrapTemplate struct {
	// templateRef is a required reference to the BootstrapTemplate for a MachineDeployment.
	// +required
	TemplateRef ClusterClassTemplateReference `json:"templateRef,omitempty,omitzero"`
}

// MachineDeploymentClassInfrastructureTemplate defines the InfrastructureTemplate for a MachineDeployment.
type MachineDeploymentClassInfrastructureTemplate struct {
	// templateRef is a required reference to the InfrastructureTemplate for a MachineDeployment.
	// +required
	TemplateRef ClusterClassTemplateReference `json:"templateRef,omitempty,omitzero"`
}

// MachinePoolClassBootstrapTemplate defines the BootstrapTemplate for a MachinePool.
type MachinePoolClassBootstrapTemplate struct {
	// templateRef is a required reference to the BootstrapTemplate for a MachinePool.
	// +required
	TemplateRef ClusterClassTemplateReference `json:"templateRef,omitempty,omitzero"`
}

// MachinePoolClassInfrastructureTemplate defines the InfrastructureTemplate for a MachinePool.
type MachinePoolClassInfrastructureTemplate struct {
	// templateRef is a required reference to the InfrastructureTemplate for a MachinePool.
	// +required
	TemplateRef ClusterClassTemplateReference `json:"templateRef,omitempty,omitzero"`
}

// ClusterClassTemplateReference is a reference to a ClusterClass template.
type ClusterClassTemplateReference struct {
	// kind of the template.
	// kind must consist of alphanumeric characters or '-', start with an alphabetic character, and end with an alphanumeric character.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-zA-Z]([-a-zA-Z0-9]*[a-zA-Z0-9])?$`
	Kind string `json:"kind,omitempty"`

	// name of the template.
	// name must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
	Name string `json:"name,omitempty"`

	// apiVersion of the template.
	// apiVersion must be fully qualified domain name followed by / and a version.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=317
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*\/[a-z]([-a-z0-9]*[a-z0-9])?$`
	APIVersion string `json:"apiVersion,omitempty"`
}

// IsDefined returns true if the ClusterClassTemplateReference is set.
func (r *ClusterClassTemplateReference) IsDefined() bool {
	if r == nil {
		return false
	}
	return r.Kind != "" || r.Name != "" || r.APIVersion != ""
}

// ToObjectReference returns an object reference for the ClusterClassTemplateReference in a given namespace.
func (r *ClusterClassTemplateReference) ToObjectReference(namespace string) *corev1.ObjectReference {
	if r == nil || !r.IsDefined() {
		return nil
	}
	return &corev1.ObjectReference{
		APIVersion: r.APIVersion,
		Kind:       r.Kind,
		Namespace:  namespace,
		Name:       r.Name,
	}
}

// GroupVersionKind gets the GroupVersionKind for a ClusterClassTemplateReference.
func (r *ClusterClassTemplateReference) GroupVersionKind() schema.GroupVersionKind {
	return schema.FromAPIVersionAndKind(r.APIVersion, r.Kind)
}

// ClusterClassStatus defines the observed state of the ClusterClass.
// +kubebuilder:validation:MinProperties=1
type ClusterClassStatus struct {
	// conditions represents the observations of a ClusterClass's current state.
	// Known condition types are VariablesReady, RefVersionsUpToDate, Paused.
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=32
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// variables is a list of ClusterClassStatusVariable that are defined for the ClusterClass.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MaxItems=1000
	Variables []ClusterClassStatusVariable `json:"variables,omitempty"`

	// observedGeneration is the latest generation observed by the controller.
	// +optional
	// +kubebuilder:validation:Minimum=1
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// deprecated groups all the status fields that are deprecated and will be removed when all the nested field are removed.
	// +optional
	Deprecated *ClusterClassDeprecatedStatus `json:"deprecated,omitempty"`
}

// ClusterClassDeprecatedStatus groups all the status fields that are deprecated and will be removed in a future version.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type ClusterClassDeprecatedStatus struct {
	// v1beta1 groups all the status fields that are deprecated and will be removed when support for v1beta1 will be dropped.
	// +optional
	V1Beta1 *ClusterClassV1Beta1DeprecatedStatus `json:"v1beta1,omitempty"`
}

// ClusterClassV1Beta1DeprecatedStatus groups all the status fields that are deprecated and will be removed when support for v1beta1 will be dropped.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type ClusterClassV1Beta1DeprecatedStatus struct {
	// conditions defines current observed state of the ClusterClass.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	Conditions Conditions `json:"conditions,omitempty"`
}

// ClusterClassStatusVariable defines a variable which appears in the status of a ClusterClass.
type ClusterClassStatusVariable struct {
	// name is the name of the variable.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Name string `json:"name,omitempty"`

	// definitionsConflict specifies whether or not there are conflicting definitions for a single variable name.
	// +optional
	DefinitionsConflict *bool `json:"definitionsConflict,omitempty"`

	// definitions is a list of definitions for a variable.
	// +required
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	Definitions []ClusterClassStatusVariableDefinition `json:"definitions,omitempty"`
}

// ClusterClassStatusVariableDefinition defines a variable which appears in the status of a ClusterClass.
type ClusterClassStatusVariableDefinition struct {
	// from specifies the origin of the variable definition.
	// This will be `inline` for variables defined in the ClusterClass or the name of a patch defined in the ClusterClass
	// for variables discovered from a DiscoverVariables runtime extensions.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	From string `json:"from,omitempty"`

	// required specifies if the variable is required.
	// Note: this applies to the variable as a whole and thus the
	// top-level object defined in the schema. If nested fields are
	// required, this will be specified inside the schema.
	// +required
	Required *bool `json:"required,omitempty"`

	// deprecatedV1Beta1Metadata is the metadata of a variable.
	// It can be used to add additional data for higher level tools to
	// a ClusterClassVariable.
	//
	// Deprecated: This field is deprecated and will be removed when support for v1beta1 will be dropped. Please use XMetadata in JSONSchemaProps instead.
	//
	// +optional
	DeprecatedV1Beta1Metadata ClusterClassVariableMetadata `json:"deprecatedV1Beta1Metadata,omitempty,omitzero"`

	// schema defines the schema of the variable.
	// +required
	Schema VariableSchema `json:"schema,omitempty,omitzero"`
}

// GetV1Beta1Conditions returns the set of conditions for this object.
func (c *ClusterClass) GetV1Beta1Conditions() Conditions {
	if c.Status.Deprecated == nil || c.Status.Deprecated.V1Beta1 == nil {
		return nil
	}
	return c.Status.Deprecated.V1Beta1.Conditions
}

// SetV1Beta1Conditions sets the conditions on this object.
func (c *ClusterClass) SetV1Beta1Conditions(conditions Conditions) {
	if c.Status.Deprecated == nil {
		c.Status.Deprecated = &ClusterClassDeprecatedStatus{}
	}
	if c.Status.Deprecated.V1Beta1 == nil {
		c.Status.Deprecated.V1Beta1 = &ClusterClassV1Beta1DeprecatedStatus{}
	}
	c.Status.Deprecated.V1Beta1.Conditions = conditions
}

// GetConditions returns the set of conditions for this object.
func (c *ClusterClass) GetConditions() []metav1.Condition {
	return c.Status.Conditions
}

// SetConditions sets conditions for an API object.
func (c *ClusterClass) SetConditions(conditions []metav1.Condition) {
	c.Status.Conditions = conditions
}

// +kubebuilder:object:root=true

// ClusterClassList contains a list of Cluster.
type ClusterClassList struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard list's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#lists-and-simple-kinds
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`
	// items is the list of ClusterClasses.
	Items []ClusterClass `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &ClusterClass{}, &ClusterClassList{})
}
