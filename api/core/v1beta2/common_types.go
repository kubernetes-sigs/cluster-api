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
	corev1 "k8s.io/api/core/v1"
	apivalidation "k8s.io/apimachinery/pkg/api/validation"
	metav1validation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

const (
	// ClusterNameLabel is the label set on machines linked to a cluster and
	// external objects(bootstrap and infrastructure providers).
	ClusterNameLabel = "cluster.x-k8s.io/cluster-name"

	// ClusterTopologyOwnedLabel is the label set on all the object which are managed as part of a ClusterTopology.
	ClusterTopologyOwnedLabel = "topology.cluster.x-k8s.io/owned"

	// ClusterTopologyMachineDeploymentNameLabel is the label set on the generated  MachineDeployment objects
	// to track the name of the MachineDeployment topology it represents.
	ClusterTopologyMachineDeploymentNameLabel = "topology.cluster.x-k8s.io/deployment-name"

	// ClusterTopologyUpgradeStepAnnotation tracks the version of the current upgrade step.
	// It is only set when an upgrade is in progress, and it contains the control plane version computed by topology controller.
	ClusterTopologyUpgradeStepAnnotation = "topology.internal.cluster.x-k8s.io/upgrade-step"

	// ClusterTopologyHoldUpgradeSequenceAnnotation can be used to hold the entire MachineDeployment upgrade sequence.
	// If the annotation is set on a MachineDeployment topology in Cluster.spec.topology.workers, the Kubernetes upgrade
	// for this MachineDeployment topology and all subsequent ones is deferred.
	// Examples:
	// - If you want to pause upgrade after CP upgrade, this annotation should be applied to the first MachineDeployment
	//   in the list of MachineDeployments in Cluster.spec.topology. The upgrade will not be completed until the annotation
	//   is removed and all MachineDeployments are upgraded.
	// - If you want to pause upgrade after the 50th MachineDeployment, this annotation should be applied to the 51st
	//   MachineDeployment in the list.
	ClusterTopologyHoldUpgradeSequenceAnnotation = "topology.cluster.x-k8s.io/hold-upgrade-sequence"

	// ClusterTopologyDeferUpgradeAnnotation can be used to defer the Kubernetes upgrade of a single MachineDeployment topology.
	// If the annotation is set on a MachineDeployment topology in Cluster.spec.topology.workers, the Kubernetes upgrade
	// for this MachineDeployment topology is deferred. It doesn't affect other MachineDeployment topologies.
	// Example:
	// - If you want to defer the upgrades of the 3rd and 5th MachineDeployments of the list, set the annotation on them.
	//   The upgrade process will upgrade MachineDeployment in position 1,2, (skip 3), 4, (skip 5), 6 etc. The upgrade
	//   will not be completed until the annotation is removed and all MachineDeployments are upgraded.
	ClusterTopologyDeferUpgradeAnnotation = "topology.cluster.x-k8s.io/defer-upgrade"

	// ClusterTopologyUpgradeConcurrencyAnnotation can be set as top-level annotation on the Cluster object of
	// a classy Cluster to define the maximum concurrency while upgrading MachineDeployments.
	ClusterTopologyUpgradeConcurrencyAnnotation = "topology.cluster.x-k8s.io/upgrade-concurrency"

	// ClusterTopologyMachinePoolNameLabel is the label set on the generated  MachinePool objects
	// to track the name of the MachinePool topology it represents.
	ClusterTopologyMachinePoolNameLabel = "topology.cluster.x-k8s.io/pool-name"

	// ClusterTopologyUnsafeUpdateClassNameAnnotation can be used to disable the webhook check on
	// update that disallows a pre-existing Cluster to be populated with Topology information and Class.
	ClusterTopologyUnsafeUpdateClassNameAnnotation = "unsafe.topology.cluster.x-k8s.io/disable-update-class-name-check"

	// ClusterTopologyUnsafeUpdateVersionAnnotation can be used to disable the webhook checks on
	// update that disallows updating the .topology.spec.version on certain conditions.
	ClusterTopologyUnsafeUpdateVersionAnnotation = "unsafe.topology.cluster.x-k8s.io/disable-update-version-check"

	// ProviderNameLabel is the label set on components in the provider manifest.
	// This label allows to easily identify all the components belonging to a provider; the clusterctl
	// tool uses this label for implementing provider's lifecycle operations.
	ProviderNameLabel = "cluster.x-k8s.io/provider"

	// ClusterNameAnnotation is the annotation set on nodes identifying the name of the cluster the node belongs to.
	ClusterNameAnnotation = "cluster.x-k8s.io/cluster-name"

	// ClusterNamespaceAnnotation is the annotation set on nodes identifying the namespace of the cluster the node belongs to.
	ClusterNamespaceAnnotation = "cluster.x-k8s.io/cluster-namespace"

	// MachineAnnotation is the annotation set on nodes identifying the machine the node belongs to.
	MachineAnnotation = "cluster.x-k8s.io/machine"

	// OwnerKindAnnotation is the annotation set on nodes identifying the owner kind.
	OwnerKindAnnotation = "cluster.x-k8s.io/owner-kind"

	// LabelsFromMachineAnnotation is the annotation set on nodes to track the labels originated from machines.
	LabelsFromMachineAnnotation = "cluster.x-k8s.io/labels-from-machine"

	// AnnotationsFromMachineAnnotation is the annotation set on nodes to track the annotations that originated from machines.
	AnnotationsFromMachineAnnotation = "cluster.x-k8s.io/annotations-from-machine"

	// TaintsFromMachineAnnotation is the annotation set on nodes to track the taints that originated from machines.
	TaintsFromMachineAnnotation = "cluster.x-k8s.io/taints-from-machine"

	// OwnerNameAnnotation is the annotation set on nodes identifying the owner name.
	OwnerNameAnnotation = "cluster.x-k8s.io/owner-name"

	// PausedAnnotation is an annotation that can be applied to any Cluster API
	// object to prevent a controller from processing a resource.
	//
	// Controllers working with Cluster API objects must check the existence of this annotation
	// on the reconciled object.
	PausedAnnotation = "cluster.x-k8s.io/paused"

	// DisableMachineCreateAnnotation is an annotation that can be used to signal a MachineSet to stop creating new machines.
	// It is utilized in the OnDelete rollout strategy to allow the MachineDeployment controller to scale down
	// older MachineSets when Machines are deleted and add the new replicas to the latest MachineSet.
	DisableMachineCreateAnnotation = "cluster.x-k8s.io/disable-machine-create"

	// WatchLabel is a label othat can be applied to any Cluster API object.
	//
	// Controllers which allow for selective reconciliation may check this label and proceed
	// with reconciliation of the object only if this label and a configured value is present.
	WatchLabel = "cluster.x-k8s.io/watch-filter"

	// DeleteMachineAnnotation marks control plane and worker nodes that will be given priority for deletion
	// when KCP or a machineset scales down. This annotation is given top priority on all delete policies.
	DeleteMachineAnnotation = "cluster.x-k8s.io/delete-machine"

	// TemplateClonedFromNameAnnotation is the infrastructure machine annotation that stores the name of the infrastructure template resource
	// that was cloned for the machine. This annotation is set only during cloning a template. Older/adopted machines will not have this annotation.
	TemplateClonedFromNameAnnotation = "cluster.x-k8s.io/cloned-from-name"

	// TemplateClonedFromGroupKindAnnotation is the infrastructure machine annotation that stores the group-kind of the infrastructure template resource
	// that was cloned for the machine. This annotation is set only during cloning a template. Older/adopted machines will not have this annotation.
	TemplateClonedFromGroupKindAnnotation = "cluster.x-k8s.io/cloned-from-groupkind"

	// MachineSkipRemediationAnnotation is the annotation used to mark the machines that should not be considered for remediation by MachineHealthCheck reconciler.
	MachineSkipRemediationAnnotation = "cluster.x-k8s.io/skip-remediation"

	// RemediateMachineAnnotation request the MachineHealthCheck reconciler to mark a Machine as unhealthy. CAPI builtin remediation will prioritize Machines with the annotation to be remediated.
	RemediateMachineAnnotation = "cluster.x-k8s.io/remediate-machine"

	// MachineSetSkipPreflightChecksAnnotation is the annotation used to provide a comma-separated list of
	// preflight checks that should be skipped during the MachineSet reconciliation.
	// Supported items are:
	// - KubeadmVersion (skips the kubeadm version skew preflight check)
	// - KubernetesVersion (skips the kubernetes version skew preflight check)
	// - ControlPlaneStable (skips checking that the control plane is neither provisioning nor upgrading)
	// - All (skips all preflight checks)
	// Example: "machineset.cluster.x-k8s.io/skip-preflight-checks": "ControlPlaneStable,KubernetesVersion".
	// Note: The annotation can also be set on a MachineDeployment as MachineDeployment annotations are synced to
	// the MachineSet.
	MachineSetSkipPreflightChecksAnnotation = "machineset.cluster.x-k8s.io/skip-preflight-checks"

	// ClusterSecretType defines the type of secret created by core components.
	// Note: This is used by core CAPI, CAPBK, and KCP to determine whether a secret is created by the controllers
	// themselves or supplied by the user (e.g. bring your own certificates).
	ClusterSecretType corev1.SecretType = "cluster.x-k8s.io/secret" //nolint:gosec

	// InterruptibleLabel is the label used to mark the nodes that run on interruptible instances.
	InterruptibleLabel = "cluster.x-k8s.io/interruptible"

	// ManagedByAnnotation is an annotation that can be applied to InfraCluster resources to signify that
	// some external system is managing the cluster infrastructure.
	//
	// Provider InfraCluster controllers will ignore resources with this annotation.
	// An external controller must fulfill the contract of the InfraCluster resource.
	// External infrastructure providers should ensure that the annotation, once set, cannot be removed.
	ManagedByAnnotation = "cluster.x-k8s.io/managed-by"

	// TopologyDryRunAnnotation is an annotation that gets set on objects by the topology controller
	// only during a server side dry run apply operation. It is used for validating
	// update webhooks for objects which get updated by template rotation (e.g. InfrastructureMachineTemplate).
	// When the annotation is set and the admission request is a dry run, the webhook should
	// skip validation due to immutability. By that the request will succeed (without
	// any changes to the actual object because it is a dry run) and the topology controller
	// will receive the resulting object.
	TopologyDryRunAnnotation = "topology.cluster.x-k8s.io/dry-run"

	// ReplicasManagedByAnnotation is an annotation that indicates external (non-Cluster API) management of infra scaling.
	// The practical effect of this is that the capi "replica" count should be passively derived from the number of observed infra machines,
	// instead of being a source of truth for eventual consistency.
	// This annotation can be used to inform MachinePool status during in-progress scaling scenarios.
	ReplicasManagedByAnnotation = "cluster.x-k8s.io/replicas-managed-by"

	// AutoscalerMinSizeAnnotation defines the minimum node group size.
	// The annotation is used by autoscaler.
	// The annotation is copied from kubernetes/autoscaler.
	// Ref:https://github.com/kubernetes/autoscaler/blob/d8336cca37dbfa5d1cb7b7e453bd511172d6e5e7/cluster-autoscaler/cloudprovider/clusterapi/clusterapi_utils.go#L256-L259
	// Note: With the Kubernetes autoscaler it is possible to use different annotations by configuring a different
	// "Cluster API group" than "cluster.x-k8s.io" via the "CAPI_GROUP" environment variable.
	// We only handle the default group in our implementation.
	// Note: It can be used by setting as top level annotation on MachineDeployment and MachineSets.
	AutoscalerMinSizeAnnotation = "cluster.x-k8s.io/cluster-api-autoscaler-node-group-min-size"

	// AutoscalerMaxSizeAnnotation defines the maximum node group size.
	// The annotations is used by the autoscaler.
	// The annotation definition is copied from kubernetes/autoscaler.
	// Ref:https://github.com/kubernetes/autoscaler/blob/d8336cca37dbfa5d1cb7b7e453bd511172d6e5e7/cluster-autoscaler/cloudprovider/clusterapi/clusterapi_utils.go#L264-L267
	// Note: With the Kubernetes autoscaler it is possible to use different annotations by configuring a different
	// "Cluster API group" than "cluster.x-k8s.io" via the "CAPI_GROUP" environment variable.
	// We only handle the default group in our implementation.
	// Note: It can be used by setting as top level annotation on MachineDeployment and MachineSets.
	AutoscalerMaxSizeAnnotation = "cluster.x-k8s.io/cluster-api-autoscaler-node-group-max-size"

	// VariableDefinitionFromInline indicates a patch or variable was defined in the `.spec` of a ClusterClass
	// rather than from an external patch extension.
	VariableDefinitionFromInline = "inline"

	// CRDMigrationObservedGenerationAnnotation indicates on a CRD for which generation CRD migration is completed.
	CRDMigrationObservedGenerationAnnotation = "crd-migration.cluster.x-k8s.io/observed-generation"

	// BeforeClusterUpgradeHookAnnotationPrefix annotation specifies the prefix we search each annotation
	// for during the before-upgrade lifecycle hook to block propagating the new version to the control plane.
	// This hook can be used to execute pre-upgrade add-on tasks and block upgrades of the ControlPlane and Workers.
	// Note: While the upgrade is blocked changes made to the Cluster Topology will be delayed propagating to the underlying
	// objects while the object is waiting for upgrade.
	BeforeClusterUpgradeHookAnnotationPrefix = "before-upgrade.hook.cluster.cluster.x-k8s.io"
)

// MachineSetPreflightCheck defines a valid MachineSet preflight check.
type MachineSetPreflightCheck string

const (
	// MachineSetPreflightCheckAll can be used to represent all the MachineSet preflight checks.
	MachineSetPreflightCheckAll MachineSetPreflightCheck = "All"

	// MachineSetPreflightCheckKubeadmVersionSkew is the name of the preflight check
	// that verifies if the machine being created or remediated for the MachineSet conforms to the kubeadm version
	// skew policy that requires the machine to be at the same minor version as the control plane.
	// The preflight check is only run if a ControlPlane is used (controlPlaneRef must exist in the Cluster),
	// the ControlPlane has a version, the MachineSet has a version and the MachineSet uses the Kubeadm bootstrap
	// provider.
	MachineSetPreflightCheckKubeadmVersionSkew MachineSetPreflightCheck = "KubeadmVersionSkew"

	// MachineSetPreflightCheckKubernetesVersionSkew is the name of the preflight check that verifies
	// if the machines being created or remediated for the MachineSet conform to the Kubernetes version skew policy
	// that requires the machines to be at a version that is not more than 2 (< v1.28) or 3 (>= v1.28) minor
	// lower than the ControlPlane version.
	// The preflight check is only run if a ControlPlane is used (controlPlaneRef must exist in the Cluster),
	// the ControlPlane has a version and the MachineSet has a version.
	MachineSetPreflightCheckKubernetesVersionSkew MachineSetPreflightCheck = "KubernetesVersionSkew"

	// MachineSetPreflightCheckControlPlaneIsStable is the name of the preflight check
	// that verifies if the control plane is not provisioning and not upgrading.
	// For Clusters with a managed topology it also checks if a control plane upgrade is pending.
	// The preflight check is only run if a ControlPlane is used (controlPlaneRef must exist in the Cluster)
	// and the ControlPlane has a version.
	MachineSetPreflightCheckControlPlaneIsStable MachineSetPreflightCheck = "ControlPlaneIsStable"

	// MachineSetPreflightCheckControlPlaneVersionSkew is the name of the preflight check
	// that verifies if the machine being created or remediated for the MachineSet has exactly the same version
	// as the control plane.
	// The idea behind this check is that it doesn't make sense to create a Machine with an old version, if we already
	// know based on the control plane version that the Machine has to be replaced soon.
	// The preflight check is only run if the Cluster has a managed topology, a ControlPlane is used (controlPlaneRef
	// must exist in the Cluster), the ControlPlane has a version and the MachineSet has a version.
	MachineSetPreflightCheckControlPlaneVersionSkew MachineSetPreflightCheck = "ControlPlaneVersionSkew"
)

// NodeOutdatedRevisionTaint can be added to Nodes at rolling updates in general triggered by updating MachineDeployment
// This taint is used to prevent unnecessary pod churn, i.e., as the first node is drained, pods previously running on
// that node are scheduled onto nodes who have yet to be replaced, but will be torn down soon.
var NodeOutdatedRevisionTaint = corev1.Taint{
	Key:    "node.cluster.x-k8s.io/outdated-revision",
	Effect: corev1.TaintEffectPreferNoSchedule,
}

// NodeUninitializedTaint can be added to Nodes at creation by the bootstrap provider, e.g. the
// KubeadmBootstrap provider will add the taint.
// This taint is used to prevent workloads to be scheduled on Nodes before the node is initialized by Cluster API.
// As of today the Node initialization consists of syncing labels from Machines to Nodes. Once the labels
// have been initially synced the taint is removed from the Node.
var NodeUninitializedTaint = corev1.Taint{
	Key:    "node.cluster.x-k8s.io/uninitialized",
	Effect: corev1.TaintEffectNoSchedule,
}

const (
	// TemplateSuffix is the object kind suffix used by template types.
	TemplateSuffix = "Template"
)

// MachineAddressType describes a valid MachineAddress type.
// +kubebuilder:validation:Enum=Hostname;ExternalIP;InternalIP;ExternalDNS;InternalDNS
type MachineAddressType string

// Define the MachineAddressType constants.
const (
	MachineHostName    MachineAddressType = "Hostname"
	MachineExternalIP  MachineAddressType = "ExternalIP"
	MachineInternalIP  MachineAddressType = "InternalIP"
	MachineExternalDNS MachineAddressType = "ExternalDNS"
	MachineInternalDNS MachineAddressType = "InternalDNS"
)

// MachineAddress contains information for the node's address.
type MachineAddress struct {
	// type is the machine address type, one of Hostname, ExternalIP, InternalIP, ExternalDNS or InternalDNS.
	// +required
	Type MachineAddressType `json:"type,omitempty"`

	// address is the machine address.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Address string `json:"address,omitempty"`
}

// MachineAddresses is a slice of MachineAddress items to be used by infrastructure providers.
// +kubebuilder:validation:MaxItems=128
// +listType=atomic
type MachineAddresses []MachineAddress

// ObjectMeta is metadata that all persisted resources must have, which includes all objects
// users must create. This is a copy of customizable fields from metav1.ObjectMeta.
//
// ObjectMeta is embedded in `Machine.Spec`, `MachineDeployment.Template` and `MachineSet.Template`,
// which are not top-level Kubernetes objects. Given that metav1.ObjectMeta has lots of special cases
// and read-only fields which end up in the generated CRD validation, having it as a subset simplifies
// the API and some issues that can impact user experience.
//
// During the [upgrade to controller-tools@v2](https://github.com/kubernetes-sigs/cluster-api/pull/1054)
// for v1alpha2, we noticed a failure would occur running Cluster API test suite against the new CRDs,
// specifically `spec.metadata.creationTimestamp in body must be of type string: "null"`.
// The investigation showed that `controller-tools@v2` behaves differently than its previous version
// when handling types from [metav1](k8s.io/apimachinery/pkg/apis/meta/v1) package.
//
// In more details, we found that embedded (non-top level) types that embedded `metav1.ObjectMeta`
// had validation properties, including for `creationTimestamp` (metav1.Time).
// The `metav1.Time` type specifies a custom json marshaller that, when IsZero() is true, returns `null`
// which breaks validation because the field isn't marked as nullable.
//
// In future versions, controller-tools@v2 might allow overriding the type and validation for embedded
// types. When that happens, this hack should be revisited.
// +kubebuilder:validation:MinProperties=1
type ObjectMeta struct {
	// labels is a map of string keys and values that can be used to organize and categorize
	// (scope and select) objects. May match selectors of replication controllers
	// and services.
	// More info: http://kubernetes.io/docs/user-guide/labels
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// annotations is an unstructured key value map stored with a resource that may be
	// set by external tools to store and retrieve arbitrary metadata. They are not
	// queryable and should be preserved when modifying objects.
	// More info: http://kubernetes.io/docs/user-guide/annotations
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// Validate validates the labels and annotations in ObjectMeta.
func (metadata *ObjectMeta) Validate(parent *field.Path) field.ErrorList {
	allErrs := metav1validation.ValidateLabels(
		metadata.Labels,
		parent.Child("labels"),
	)
	allErrs = append(allErrs, apivalidation.ValidateAnnotations(
		metadata.Annotations,
		parent.Child("annotations"),
	)...)
	return allErrs
}

// ContractVersionedObjectReference is a reference to a resource for which the version is inferred from contract labels.
type ContractVersionedObjectReference struct {
	// kind of the resource being referenced.
	// kind must consist of alphanumeric characters or '-', start with an alphabetic character, and end with an alphanumeric character.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-zA-Z]([-a-zA-Z0-9]*[a-zA-Z0-9])?$`
	Kind string `json:"kind,omitempty"`

	// name of the resource being referenced.
	// name must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
	Name string `json:"name,omitempty"`

	// apiGroup is the group of the resource being referenced.
	// apiGroup must be fully qualified domain name.
	// The corresponding version for this reference will be looked up from the contract
	// labels of the corresponding CRD of the resource being referenced.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
	APIGroup string `json:"apiGroup,omitempty"`
}

// IsDefined returns true if the ContractVersionedObjectReference is set.
func (r *ContractVersionedObjectReference) IsDefined() bool {
	if r == nil {
		return false
	}
	return r.Kind != "" || r.Name != "" || r.APIGroup != ""
}

// GroupKind returns the GroupKind of the reference.
func (r *ContractVersionedObjectReference) GroupKind() schema.GroupKind {
	return schema.GroupKind{
		Group: r.APIGroup,
		Kind:  r.Kind,
	}
}

// MachineTaint defines a taint equivalent to corev1.Taint, but additionally having a propagation field.
type MachineTaint struct {
	// key is the taint key to be applied to a node.
	// Must be a valid qualified name of maximum size 63 characters
	// with an optional subdomain prefix of maximum size 253 characters,
	// separated by a `/`.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=317
	// +kubebuilder:validation:Pattern=^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*\/)?([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]$
	// +kubebuilder:validation:XValidation:rule="self.contains('/') ? ( self.split('/') [0].size() <= 253 && self.split('/') [1].size() <= 63 && self.split('/').size() == 2 ) : self.size() <= 63",message="key must be a valid qualified name of max size 63 characters with an optional subdomain prefix of max size 253 characters"
	Key string `json:"key,omitempty"`

	// value is the taint value corresponding to the taint key.
	// It must be a valid label value of maximum size 63 characters.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=^(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])?$
	Value string `json:"value,omitempty"`

	// effect is the effect for the taint. Valid values are NoSchedule, PreferNoSchedule and NoExecute.
	// +required
	// +kubebuilder:validation:Enum=NoSchedule;PreferNoSchedule;NoExecute
	Effect corev1.TaintEffect `json:"effect,omitempty"`

	// propagation defines how this taint should be propagated to nodes.
	// Valid values are 'Always' and 'OnInitialization'.
	// Always: The taint will be continuously reconciled. If it is not set for a node, it will be added during reconciliation.
	// OnInitialization: The taint will be added during node initialization. If it gets removed from the node later on it will not get added again.
	// +required
	Propagation MachineTaintPropagation `json:"propagation,omitempty"`
}

// MachineTaintPropagation defines when a taint should be propagated to nodes.
// +kubebuilder:validation:Enum=Always;OnInitialization
type MachineTaintPropagation string

const (
	// MachineTaintPropagationAlways means the taint should be continuously reconciled and kept on the node.
	// - If an Always taint is added to the Machine, the taint will be added to the node.
	// - If an Always taint is removed from the Machine, the taint will be removed from the node.
	// - If an OnInitialization taint is changed to Always, the Machine controller will ensure the taint is set on the node.
	// - If an Always taint is removed from the node, it will be re-added during reconciliation.
	MachineTaintPropagationAlways MachineTaintPropagation = "Always"

	// MachineTaintPropagationOnInitialization means the taint should be set once during initialization and then
	// left alone.
	// - If an OnInitialization taint is added to the Machine, the taint will only be added to the node on initialization.
	// - If an OnInitialization taint is removed from the Machine nothing will be changed on the node.
	// - If an Always taint is changed to OnInitialization, the taint will only be added to the node on initialization.
	// - If an OnInitialization taint is removed from the node, it will not be re-added during reconciliation.
	MachineTaintPropagationOnInitialization MachineTaintPropagation = "OnInitialization"
)
