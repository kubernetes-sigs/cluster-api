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
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	bootstrapv1beta1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta1"
	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	"sigs.k8s.io/cluster-api/errors"
)

// RolloutStrategyType defines the rollout strategies for a KubeadmControlPlane.
// +kubebuilder:validation:Enum=RollingUpdate
type RolloutStrategyType string

const (
	// RollingUpdateStrategyType replaces the old control planes by new one using rolling update
	// i.e. gradually scale up or down the old control planes and scale up or down the new one.
	RollingUpdateStrategyType RolloutStrategyType = "RollingUpdate"
)

const (
	// KubeadmControlPlaneFinalizer is the finalizer applied to KubeadmControlPlane resources
	// by its managing controller.
	KubeadmControlPlaneFinalizer = "kubeadm.controlplane.cluster.x-k8s.io"

	// SkipCoreDNSAnnotation annotation explicitly skips reconciling CoreDNS if set.
	SkipCoreDNSAnnotation = "controlplane.cluster.x-k8s.io/skip-coredns"

	// SkipKubeProxyAnnotation annotation explicitly skips reconciling kube-proxy if set.
	SkipKubeProxyAnnotation = "controlplane.cluster.x-k8s.io/skip-kube-proxy"

	// KubeadmClusterConfigurationAnnotation is a machine annotation that stores the json-marshalled string of KCP ClusterConfiguration.
	// This annotation is used to detect any changes in ClusterConfiguration and trigger machine rollout in KCP.
	KubeadmClusterConfigurationAnnotation = "controlplane.cluster.x-k8s.io/kubeadm-cluster-configuration"

	// RemediationInProgressAnnotation is used to keep track that a KCP remediation is in progress, and more
	// specifically it tracks that the system is in between having deleted an unhealthy machine and recreating its replacement.
	// NOTE: if something external to CAPI removes this annotation the system cannot detect the above situation; this can lead to
	// failures in updating remediation retry or remediation count (both counters restart from zero).
	RemediationInProgressAnnotation = "controlplane.cluster.x-k8s.io/remediation-in-progress"

	// RemediationForAnnotation is used to link a new machine to the unhealthy machine it is replacing;
	// please note that in case of retry, when also the remediating machine fails, the system keeps track of
	// the first machine of the sequence only.
	// NOTE: if something external to CAPI removes this annotation the system this can lead to
	// failures in updating remediation retry (the counter restarts from zero).
	RemediationForAnnotation = "controlplane.cluster.x-k8s.io/remediation-for"

	// PreTerminateHookCleanupAnnotation is the annotation KCP sets on Machines to ensure it can later remove the
	// etcd member right before Machine termination (i.e. before InfraMachine deletion).
	// Note: Starting with Kubernetes v1.31 this hook will wait for all other pre-terminate hooks to finish to
	// ensure it runs last (thus ensuring that kubelet is still working while other pre-terminate hooks run).
	PreTerminateHookCleanupAnnotation = clusterv1beta1.PreTerminateDeleteHookAnnotationPrefix + "/kcp-cleanup"

	// DefaultMinHealthyPeriod defines the default minimum period before we consider a remediation on a
	// machine unrelated from the previous remediation.
	DefaultMinHealthyPeriod = 1 * time.Hour
)

// KubeadmControlPlaneSpec defines the desired state of KubeadmControlPlane.
type KubeadmControlPlaneSpec struct {
	// replicas is the number of desired machines. Defaults to 1. When stacked etcd is used only
	// odd numbers are permitted, as per [etcd best practice](https://etcd.io/docs/v3.3.12/faq/#why-an-odd-number-of-cluster-members).
	// This is a pointer to distinguish between explicit zero and not specified.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// version defines the desired Kubernetes version.
	// Please note that if kubeadmConfigSpec.ClusterConfiguration.imageRepository is not set
	// we don't allow upgrades to versions >= v1.22.0 for which kubeadm uses the old registry (k8s.gcr.io).
	// Please use a newer patch version with the new registry instead. The default registries of kubeadm are:
	//   * registry.k8s.io (new registry): >= v1.22.17, >= v1.23.15, >= v1.24.9, >= v1.25.0
	//   * k8s.gcr.io (old registry): all older versions
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Version string `json:"version"`

	// machineTemplate contains information about how machines
	// should be shaped when creating or updating a control plane.
	// +required
	MachineTemplate KubeadmControlPlaneMachineTemplate `json:"machineTemplate"`

	// kubeadmConfigSpec is a KubeadmConfigSpec
	// to use for initializing and joining machines to the control plane.
	// +required
	KubeadmConfigSpec bootstrapv1beta1.KubeadmConfigSpec `json:"kubeadmConfigSpec"`

	// rolloutBefore is a field to indicate a rollout should be performed
	// if the specified criteria is met.
	// +optional
	RolloutBefore *RolloutBefore `json:"rolloutBefore,omitempty"`

	// rolloutAfter is a field to indicate a rollout should be performed
	// after the specified time even if no changes have been made to the
	// KubeadmControlPlane.
	// Example: In the YAML the time can be specified in the RFC3339 format.
	// To specify the rolloutAfter target as March 9, 2023, at 9 am UTC
	// use "2023-03-09T09:00:00Z".
	// +optional
	RolloutAfter *metav1.Time `json:"rolloutAfter,omitempty"`

	// rolloutStrategy is the RolloutStrategy to use to replace control plane machines with
	// new ones.
	// +optional
	// +kubebuilder:default={type: "RollingUpdate", rollingUpdate: {maxSurge: 1}}
	RolloutStrategy *RolloutStrategy `json:"rolloutStrategy,omitempty"`

	// remediationStrategy is the RemediationStrategy that controls how control plane machine remediation happens.
	// +optional
	RemediationStrategy *RemediationStrategy `json:"remediationStrategy,omitempty"`

	// machineNamingStrategy allows changing the naming pattern used when creating Machines.
	// InfraMachines & KubeadmConfigs will use the same name as the corresponding Machines.
	// +optional
	MachineNamingStrategy *MachineNamingStrategy `json:"machineNamingStrategy,omitempty"`
}

// KubeadmControlPlaneMachineTemplate defines the template for Machines
// in a KubeadmControlPlane object.
type KubeadmControlPlaneMachineTemplate struct {
	// metadata is the standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	ObjectMeta clusterv1beta1.ObjectMeta `json:"metadata,omitempty"`

	// infrastructureRef is a required reference to a custom resource
	// offered by an infrastructure provider.
	// +required
	InfrastructureRef corev1.ObjectReference `json:"infrastructureRef"`

	// readinessGates specifies additional conditions to include when evaluating Machine Ready condition;
	// KubeadmControlPlane will always add readinessGates for the condition it is setting on the Machine:
	// APIServerPodHealthy, SchedulerPodHealthy, ControllerManagerPodHealthy, and if etcd is managed by CKP also
	// EtcdPodHealthy, EtcdMemberHealthy.
	//
	// This field can be used e.g. to instruct the machine controller to include in the computation for Machine's ready
	// computation a condition, managed by an external controllers, reporting the status of special software/hardware installed on the Machine.
	//
	// NOTE: This field is considered only for computing v1beta2 conditions.
	// +optional
	// +listType=map
	// +listMapKey=conditionType
	// +kubebuilder:validation:MaxItems=32
	ReadinessGates []clusterv1beta1.MachineReadinessGate `json:"readinessGates,omitempty"`

	// nodeDrainTimeout is the total amount of time that the controller will spend on draining a controlplane node
	// The default value is 0, meaning that the node can be drained without any time limitations.
	// NOTE: NodeDrainTimeout is different from `kubectl drain --timeout`
	// +optional
	NodeDrainTimeout *metav1.Duration `json:"nodeDrainTimeout,omitempty"`

	// nodeVolumeDetachTimeout is the total amount of time that the controller will spend on waiting for all volumes
	// to be detached. The default value is 0, meaning that the volumes can be detached without any time limitations.
	// +optional
	NodeVolumeDetachTimeout *metav1.Duration `json:"nodeVolumeDetachTimeout,omitempty"`

	// nodeDeletionTimeout defines how long the machine controller will attempt to delete the Node that the Machine
	// hosts after the Machine is marked for deletion. A duration of 0 will retry deletion indefinitely.
	// If no value is provided, the default value for this property of the Machine resource will be used.
	// +optional
	NodeDeletionTimeout *metav1.Duration `json:"nodeDeletionTimeout,omitempty"`
}

// RolloutBefore describes when a rollout should be performed on the KCP machines.
type RolloutBefore struct {
	// certificatesExpiryDays indicates a rollout needs to be performed if the
	// certificates of the machine will expire within the specified days.
	// +optional
	CertificatesExpiryDays *int32 `json:"certificatesExpiryDays,omitempty"`
}

// RolloutStrategy describes how to replace existing machines
// with new ones.
type RolloutStrategy struct {
	// type of rollout. Currently the only supported strategy is
	// "RollingUpdate".
	// Default is RollingUpdate.
	// +optional
	Type RolloutStrategyType `json:"type,omitempty"`

	// rollingUpdate is the rolling update config params. Present only if
	// RolloutStrategyType = RollingUpdate.
	// +optional
	RollingUpdate *RollingUpdate `json:"rollingUpdate,omitempty"`
}

// RollingUpdate is used to control the desired behavior of rolling update.
type RollingUpdate struct {
	// maxSurge is the maximum number of control planes that can be scheduled above or under the
	// desired number of control planes.
	// Value can be an absolute number 1 or 0.
	// Defaults to 1.
	// Example: when this is set to 1, the control plane can be scaled
	// up immediately when the rolling update starts.
	// +optional
	MaxSurge *intstr.IntOrString `json:"maxSurge,omitempty"`
}

// RemediationStrategy allows to define how control plane machine remediation happens.
type RemediationStrategy struct {
	// maxRetry is the Max number of retries while attempting to remediate an unhealthy machine.
	// A retry happens when a machine that was created as a replacement for an unhealthy machine also fails.
	// For example, given a control plane with three machines M1, M2, M3:
	//
	//	M1 become unhealthy; remediation happens, and M1-1 is created as a replacement.
	//	If M1-1 (replacement of M1) has problems while bootstrapping it will become unhealthy, and then be
	//	remediated; such operation is considered a retry, remediation-retry #1.
	//	If M1-2 (replacement of M1-1) becomes unhealthy, remediation-retry #2 will happen, etc.
	//
	// A retry could happen only after RetryPeriod from the previous retry.
	// If a machine is marked as unhealthy after MinHealthyPeriod from the previous remediation expired,
	// this is not considered a retry anymore because the new issue is assumed unrelated from the previous one.
	//
	// If not set, the remedation will be retried infinitely.
	// +optional
	MaxRetry *int32 `json:"maxRetry,omitempty"`

	// retryPeriod is the duration that KCP should wait before remediating a machine being created as a replacement
	// for an unhealthy machine (a retry).
	//
	// If not set, a retry will happen immediately.
	// +optional
	RetryPeriod metav1.Duration `json:"retryPeriod,omitempty"`

	// minHealthyPeriod defines the duration after which KCP will consider any failure to a machine unrelated
	// from the previous one. In this case the remediation is not considered a retry anymore, and thus the retry
	// counter restarts from 0. For example, assuming MinHealthyPeriod is set to 1h (default)
	//
	//	M1 become unhealthy; remediation happens, and M1-1 is created as a replacement.
	//	If M1-1 (replacement of M1) has problems within the 1hr after the creation, also
	//	this machine will be remediated and this operation is considered a retry - a problem related
	//	to the original issue happened to M1 -.
	//
	//	If instead the problem on M1-1 is happening after MinHealthyPeriod expired, e.g. four days after
	//	m1-1 has been created as a remediation of M1, the problem on M1-1 is considered unrelated to
	//	the original issue happened to M1.
	//
	// If not set, this value is defaulted to 1h.
	// +optional
	MinHealthyPeriod *metav1.Duration `json:"minHealthyPeriod,omitempty"`
}

// MachineNamingStrategy allows changing the naming pattern used when creating Machines.
// InfraMachines & KubeadmConfigs will use the same name as the corresponding Machines.
type MachineNamingStrategy struct {
	// template defines the template to use for generating the names of the Machine objects.
	// If not defined, it will fallback to `{{ .kubeadmControlPlane.name }}-{{ .random }}`.
	// If the generated name string exceeds 63 characters, it will be trimmed to 58 characters and will
	// get concatenated with a random suffix of length 5.
	// Length of the template string must not exceed 256 characters.
	// The template allows the following variables `.cluster.name`, `.kubeadmControlPlane.name` and `.random`.
	// The variable `.cluster.name` retrieves the name of the cluster object that owns the Machines being created.
	// The variable `.kubeadmControlPlane.name` retrieves the name of the KubeadmControlPlane object that owns the Machines being created.
	// The variable `.random` is substituted with random alphanumeric string, without vowels, of length 5. This variable is required
	// part of the template. If not provided, validation will fail.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Template string `json:"template,omitempty"`
}

// KubeadmControlPlaneStatus defines the observed state of KubeadmControlPlane.
type KubeadmControlPlaneStatus struct {
	// selector is the label selector in string format to avoid introspection
	// by clients, and is used to provide the CRD-based integration for the
	// scale subresource and additional integrations for things like kubectl
	// describe.. The string will be in the same format as the query-param syntax.
	// More info about label selectors: http://kubernetes.io/docs/user-guide/labels#label-selectors
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=4096
	Selector string `json:"selector,omitempty"`

	// replicas is the total number of non-terminated machines targeted by this control plane
	// (their labels match the selector).
	// +optional
	Replicas int32 `json:"replicas"`

	// version represents the minimum Kubernetes version for the control plane machines
	// in the cluster.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Version *string `json:"version,omitempty"`

	// updatedReplicas is the total number of non-terminated machines targeted by this control plane
	// that have the desired template spec.
	// +optional
	UpdatedReplicas int32 `json:"updatedReplicas"`

	// readyReplicas is the total number of fully running and ready control plane machines.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas"`

	// unavailableReplicas is the total number of unavailable machines targeted by this control plane.
	// This is the total number of machines that are still required for
	// the deployment to have 100% available capacity. They may either
	// be machines that are running but not yet ready or machines
	// that still have not been created.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	UnavailableReplicas int32 `json:"unavailableReplicas"`

	// initialized denotes that the KubeadmControlPlane API Server is initialized and thus
	// it can accept requests.
	// NOTE: this field is part of the Cluster API contract and it is used to orchestrate provisioning.
	// The value of this field is never updated after provisioning is completed. Please use conditions
	// to check the operational state of the control plane.
	// +optional
	Initialized bool `json:"initialized"`

	// ready denotes that the KubeadmControlPlane API Server became ready during initial provisioning
	// to receive requests.
	// NOTE: this field is part of the Cluster API contract and it is used to orchestrate provisioning.
	// The value of this field is never updated after provisioning is completed. Please use conditions
	// to check the operational state of the control plane.
	// +optional
	Ready bool `json:"ready"`

	// failureReason indicates that there is a terminal problem reconciling the
	// state, and will be set to a token value suitable for
	// programmatic interpretation.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	FailureReason errors.KubeadmControlPlaneStatusError `json:"failureReason,omitempty"`

	// failureMessage indicates that there is a terminal problem reconciling the
	// state, and will be set to a descriptive error message.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=10240
	FailureMessage *string `json:"failureMessage,omitempty"`

	// observedGeneration is the latest generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// conditions defines current service state of the KubeadmControlPlane.
	// +optional
	Conditions clusterv1beta1.Conditions `json:"conditions,omitempty"`

	// lastRemediation stores info about last remediation performed.
	// +optional
	LastRemediation *LastRemediationStatus `json:"lastRemediation,omitempty"`

	// v1beta2 groups all the fields that will be added or modified in KubeadmControlPlane's status with the V1Beta2 version.
	// +optional
	V1Beta2 *KubeadmControlPlaneV1Beta2Status `json:"v1beta2,omitempty"`
}

// KubeadmControlPlaneV1Beta2Status Groups all the fields that will be added or modified in KubeadmControlPlane with the V1Beta2 version.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type KubeadmControlPlaneV1Beta2Status struct {
	// conditions represents the observations of a KubeadmControlPlane's current state.
	// Known condition types are Available, CertificatesAvailable, EtcdClusterAvailable, MachinesReady, MachinesUpToDate,
	// ScalingUp, ScalingDown, Remediating, Deleting, Paused.
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=32
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// readyReplicas is the number of ready replicas for this KubeadmControlPlane. A machine is considered ready when Machine's Ready condition is true.
	// +optional
	ReadyReplicas *int32 `json:"readyReplicas,omitempty"`

	// availableReplicas is the number of available replicas targeted by this KubeadmControlPlane. A machine is considered available when Machine's Available condition is true.
	// +optional
	AvailableReplicas *int32 `json:"availableReplicas,omitempty"`

	// upToDateReplicas is the number of up-to-date replicas targeted by this KubeadmControlPlane. A machine is considered up-to-date when Machine's UpToDate condition is true.
	// +optional
	UpToDateReplicas *int32 `json:"upToDateReplicas,omitempty"`
}

// LastRemediationStatus  stores info about last remediation performed.
// NOTE: if for any reason information about last remediation are lost, RetryCount is going to restart from 0 and thus
// more remediations than expected might happen.
type LastRemediationStatus struct {
	// machine is the machine name of the latest machine being remediated.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Machine string `json:"machine"`

	// timestamp is when last remediation happened. It is represented in RFC3339 form and is in UTC.
	// +required
	Timestamp metav1.Time `json:"timestamp"`

	// retryCount used to keep track of remediation retry for the last remediated machine.
	// A retry happens when a machine that was created as a replacement for an unhealthy machine also fails.
	// +required
	RetryCount int32 `json:"retryCount"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=kubeadmcontrolplanes,shortName=kcp,scope=Namespaced,categories=cluster-api
// +kubebuilder:deprecatedversion
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas,selectorpath=.status.selector
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".metadata.labels['cluster\\.x-k8s\\.io/cluster-name']",description="Cluster"
// +kubebuilder:printcolumn:name="Initialized",type=boolean,JSONPath=".status.initialized",description="This denotes whether or not the control plane has the uploaded kubeadm-config configmap"
// +kubebuilder:printcolumn:name="API Server Available",type=boolean,JSONPath=".status.ready",description="KubeadmControlPlane API Server is ready to receive requests"
// +kubebuilder:printcolumn:name="Desired",type=integer,JSONPath=".spec.replicas",description="Total number of machines desired by this control plane",priority=10
// +kubebuilder:printcolumn:name="Replicas",type=integer,JSONPath=".status.replicas",description="Total number of non-terminated machines targeted by this control plane"
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=".status.readyReplicas",description="Total number of fully running and ready control plane machines"
// +kubebuilder:printcolumn:name="Updated",type=integer,JSONPath=".status.updatedReplicas",description="Total number of non-terminated machines targeted by this control plane that have the desired template spec"
// +kubebuilder:printcolumn:name="Unavailable",type=integer,JSONPath=".status.unavailableReplicas",description="Total number of unavailable machines targeted by this control plane"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of KubeadmControlPlane"
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=".spec.version",description="Kubernetes version associated with this control plane"

// KubeadmControlPlane is the Schema for the KubeadmControlPlane API.
type KubeadmControlPlane struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec is the desired state of KubeadmControlPlane.
	// +optional
	Spec KubeadmControlPlaneSpec `json:"spec,omitempty"`
	// status is the observed state of KubeadmControlPlane.
	// +optional
	Status KubeadmControlPlaneStatus `json:"status,omitempty"`
}

// GetConditions returns the set of conditions for this object.
func (in *KubeadmControlPlane) GetConditions() clusterv1beta1.Conditions {
	return in.Status.Conditions
}

// SetConditions sets the conditions on this object.
func (in *KubeadmControlPlane) SetConditions(conditions clusterv1beta1.Conditions) {
	in.Status.Conditions = conditions
}

// GetV1Beta2Conditions returns the set of conditions for this object.
func (in *KubeadmControlPlane) GetV1Beta2Conditions() []metav1.Condition {
	if in.Status.V1Beta2 == nil {
		return nil
	}
	return in.Status.V1Beta2.Conditions
}

// SetV1Beta2Conditions sets conditions for an API object.
func (in *KubeadmControlPlane) SetV1Beta2Conditions(conditions []metav1.Condition) {
	if in.Status.V1Beta2 == nil {
		in.Status.V1Beta2 = &KubeadmControlPlaneV1Beta2Status{}
	}
	in.Status.V1Beta2.Conditions = conditions
}

// +kubebuilder:object:root=true

// KubeadmControlPlaneList contains a list of KubeadmControlPlane.
type KubeadmControlPlaneList struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard list's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#lists-and-simple-kinds
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`
	// items is the list of KubeadmControlPlanes.
	Items []KubeadmControlPlane `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &KubeadmControlPlane{}, &KubeadmControlPlaneList{})
}
