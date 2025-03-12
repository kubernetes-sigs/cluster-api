/*
Copyright 2022 The Kubernetes Authors.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// ANCHOR: ExtensionConfigSpec

// ExtensionConfigSpec defines the desired state of ExtensionConfig.
type ExtensionConfigSpec struct {
	// clientConfig defines how to communicate with the Extension server.
	// +required
	ClientConfig ClientConfig `json:"clientConfig"`

	// namespaceSelector decides whether to call the hook for an object based
	// on whether the namespace for that object matches the selector.
	// Defaults to the empty LabelSelector, which matches all objects.
	// +optional
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`

	// settings defines key value pairs to be passed to all calls
	// to all supported RuntimeExtensions.
	// Note: Settings can be overridden on the ClusterClass.
	// +optional
	Settings map[string]string `json:"settings,omitempty"`
}

// ClientConfig contains the information to make a client
// connection with an Extension server.
type ClientConfig struct {
	// url gives the location of the Extension server, in standard URL form
	// (`scheme://host:port/path`).
	// Note: Exactly one of `url` or `service` must be specified.
	//
	// The scheme must be "https".
	//
	// The `host` should not refer to a service running in the cluster; use
	// the `service` field instead.
	//
	// A path is optional, and if present may be any string permissible in
	// a URL. If a path is set it will be used as prefix to the hook-specific path.
	//
	// Attempting to use a user or basic auth e.g. "user:password@" is not
	// allowed. Fragments ("#...") and query parameters ("?...") are not
	// allowed either.
	//
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	URL *string `json:"url,omitempty"`

	// service is a reference to the Kubernetes service for the Extension server.
	// Note: Exactly one of `url` or `service` must be specified.
	//
	// If the Extension server is running within a cluster, then you should use `service`.
	//
	// +optional
	Service *ServiceReference `json:"service,omitempty"`

	// caBundle is a PEM encoded CA bundle which will be used to validate the Extension server's server certificate.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=51200
	CABundle []byte `json:"caBundle,omitempty"`
}

// ServiceReference holds a reference to a Kubernetes Service of an Extension server.
type ServiceReference struct {
	// namespace is the namespace of the service.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	Namespace string `json:"namespace"`

	// name is the name of the service.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	Name string `json:"name"`

	// path is an optional URL path and if present may be any string permissible in
	// a URL. If a path is set it will be used as prefix to the hook-specific path.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	Path *string `json:"path,omitempty"`

	// port is the port on the service that's hosting the Extension server.
	// Defaults to 443.
	// Port should be a valid port number (1-65535, inclusive).
	// +optional
	Port *int32 `json:"port,omitempty"`
}

// ANCHOR_END: ExtensionConfigSpec

// ANCHOR: ExtensionConfigStatus

// ExtensionConfigStatus defines the observed state of ExtensionConfig.
type ExtensionConfigStatus struct {
	// handlers defines the current ExtensionHandlers supported by an Extension.
	// +optional
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MaxItems=512
	Handlers []ExtensionHandler `json:"handlers,omitempty"`

	// conditions define the current service state of the ExtensionConfig.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`

	// v1beta2 groups all the fields that will be added or modified in ExtensionConfig's status with the V1Beta2 version.
	// +optional
	V1Beta2 *ExtensionConfigV1Beta2Status `json:"v1beta2,omitempty"`
}

// ExtensionConfigV1Beta2Status groups all the fields that will be added or modified in ExtensionConfig with the V1Beta2 version.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type ExtensionConfigV1Beta2Status struct {
	// conditions represents the observations of a ExtensionConfig's current state.
	// Known condition types are Discovered, Paused.
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=32
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ExtensionHandler specifies the details of a handler for a particular runtime hook registered by an Extension server.
type ExtensionHandler struct {
	// name is the unique name of the ExtensionHandler.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	Name string `json:"name"`

	// requestHook defines the versioned runtime hook which this ExtensionHandler serves.
	// +required
	RequestHook GroupVersionHook `json:"requestHook"`

	// timeoutSeconds defines the timeout duration for client calls to the ExtensionHandler.
	// Defaults to 10 is not set.
	// +optional
	TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty"`

	// failurePolicy defines how failures in calls to the ExtensionHandler should be handled by a client.
	// Defaults to Fail if not set.
	// +optional
	// +kubebuilder:validation:Enum=Ignore;Fail
	FailurePolicy *FailurePolicy `json:"failurePolicy,omitempty"`
}

// GroupVersionHook defines the runtime hook when the ExtensionHandler is called.
type GroupVersionHook struct {
	// apiVersion is the group and version of the Hook.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	APIVersion string `json:"apiVersion"`

	// hook is the name of the hook.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Hook string `json:"hook"`
}

// FailurePolicy specifies how unrecognized errors when calling the ExtensionHandler are handled.
// FailurePolicy helps with extensions not working consistently, e.g. due to an intermittent network issue.
// The following type of errors are never ignored by FailurePolicy Ignore:
// - Misconfigurations (e.g. incompatible types)
// - Extension explicitly returns a Status Failure.
type FailurePolicy string

const (
	// FailurePolicyIgnore means that an error when calling the extension is ignored.
	FailurePolicyIgnore FailurePolicy = "Ignore"

	// FailurePolicyFail means that an error when calling the extension is propagated as an error.
	FailurePolicyFail FailurePolicy = "Fail"
)

// ANCHOR_END: ExtensionConfigStatus

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=extensionconfigs,shortName=ext,scope=Cluster,categories=cluster-api
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of ExtensionConfig"

// ExtensionConfig is the Schema for the ExtensionConfig API.
type ExtensionConfig struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec is the desired state of the ExtensionConfig.
	// +optional
	Spec ExtensionConfigSpec `json:"spec,omitempty"`

	// status is the current state of the ExtensionConfig
	// +optional
	Status ExtensionConfigStatus `json:"status,omitempty"`
}

// GetConditions returns the set of conditions for this object.
func (e *ExtensionConfig) GetConditions() clusterv1.Conditions {
	return e.Status.Conditions
}

// SetConditions sets the conditions on this object.
func (e *ExtensionConfig) SetConditions(conditions clusterv1.Conditions) {
	e.Status.Conditions = conditions
}

// GetV1Beta2Conditions returns the set of conditions for this object.
func (e *ExtensionConfig) GetV1Beta2Conditions() []metav1.Condition {
	if e.Status.V1Beta2 == nil {
		return nil
	}
	return e.Status.V1Beta2.Conditions
}

// SetV1Beta2Conditions sets conditions for an API object.
func (e *ExtensionConfig) SetV1Beta2Conditions(conditions []metav1.Condition) {
	if e.Status.V1Beta2 == nil {
		e.Status.V1Beta2 = &ExtensionConfigV1Beta2Status{}
	}
	e.Status.V1Beta2.Conditions = conditions
}

// +kubebuilder:object:root=true

// ExtensionConfigList contains a list of ExtensionConfig.
type ExtensionConfigList struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard list's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#lists-and-simple-kinds
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`
	// items is the list of ExtensionConfigs.
	Items []ExtensionConfig `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &ExtensionConfig{}, &ExtensionConfigList{})
}

// ExtensionConfig's Discovered conditions and corresponding reasons that will be used in v1Beta2 API version.
const (
	// ExtensionConfigDiscoveredV1Beta2Condition is true if the runtime extension has been successfully discovered.
	ExtensionConfigDiscoveredV1Beta2Condition = "Discovered"

	// ExtensionConfigDiscoveredV1Beta2Reason surfaces that the runtime extension has been successfully discovered.
	ExtensionConfigDiscoveredV1Beta2Reason = "Discovered"

	// ExtensionConfigNotDiscoveredV1Beta2Reason surfaces that the runtime extension has not been successfully discovered.
	ExtensionConfigNotDiscoveredV1Beta2Reason = "NotDiscovered"
)

const (
	// RuntimeExtensionDiscoveredCondition is a condition set on an ExtensionConfig object once it has been discovered by the Runtime SDK client.
	RuntimeExtensionDiscoveredCondition clusterv1.ConditionType = "Discovered"

	// DiscoveryFailedReason documents failure of a Discovery call.
	DiscoveryFailedReason string = "DiscoveryFailed"

	// InjectCAFromSecretAnnotation is the annotation that specifies that an ExtensionConfig
	// object wants injection of CAs. The value is a reference to a Secret
	// as <namespace>/<name>.
	InjectCAFromSecretAnnotation string = "runtime.cluster.x-k8s.io/inject-ca-from-secret"

	// PendingHooksAnnotation is the annotation used to keep track of pending runtime hooks.
	// The annotation will be used to track the intent to call a hook as soon as an operation completes;
	// the intent will be removed as soon as the hook call completes successfully.
	PendingHooksAnnotation string = "runtime.cluster.x-k8s.io/pending-hooks"

	// OkToDeleteAnnotation is the annotation used to indicate if a cluster is ready to be fully deleted.
	// This annotation is added to the cluster after the BeforeClusterDelete hook has passed.
	OkToDeleteAnnotation string = "runtime.cluster.x-k8s.io/ok-to-delete"
)
