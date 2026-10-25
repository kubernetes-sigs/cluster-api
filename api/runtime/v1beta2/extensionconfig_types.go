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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// ExtensionConfigSpec defines the desired state of ExtensionConfig.
type ExtensionConfigSpec struct {
	// clientConfig defines how to communicate with the Extension server.
	// +required
	ClientConfig ClientConfig `json:"clientConfig,omitempty,omitzero"`

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
// +kubebuilder:validation:MinProperties=1
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
	URL string `json:"url,omitempty"`

	// service is a reference to the Kubernetes service for the Extension server.
	// Note: Exactly one of `url` or `service` must be specified.
	//
	// If the Extension server is running within a cluster, then you should use `service`.
	//
	// +optional
	Service ServiceReference `json:"service,omitempty,omitzero"`

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
	Namespace string `json:"namespace,omitempty"`

	// name is the name of the service.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	Name string `json:"name,omitempty"`

	// path is an optional URL path and if present may be any string permissible in
	// a URL. If a path is set it will be used as prefix to the hook-specific path.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	Path string `json:"path,omitempty"`

	// port is the port on the service that's hosting the Extension server.
	// Defaults to 443.
	// Port should be a valid port number (1-65535, inclusive).
	// +optional
	Port *int32 `json:"port,omitempty"`
}

// IsDefined returns true if the ServiceReference is set.
func (r *ServiceReference) IsDefined() bool {
	return !reflect.DeepEqual(r, &ServiceReference{})
}

// ExtensionConfigStatus defines the observed state of ExtensionConfig.
// +kubebuilder:validation:MinProperties=1
type ExtensionConfigStatus struct {
	// conditions represents the observations of a ExtensionConfig's current state.
	// Known condition types are Discovered, Paused.
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=32
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// handlers defines the current ExtensionHandlers supported by an Extension.
	// +optional
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MaxItems=512
	Handlers []ExtensionHandler `json:"handlers,omitempty"`

	// deprecated groups all the status fields that are deprecated and will be removed when all the nested field are removed.
	// +optional
	Deprecated *ExtensionConfigDeprecatedStatus `json:"deprecated,omitempty"`
}

// ExtensionConfigDeprecatedStatus groups all the status fields that are deprecated and will be removed in a future version.
type ExtensionConfigDeprecatedStatus struct {
	// v1beta1 groups all the status fields that are deprecated and will be removed when support for v1beta1 will be dropped.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	V1Beta1 *ExtensionConfigV1Beta1DeprecatedStatus `json:"v1beta1,omitempty"`
}

// ExtensionConfigV1Beta1DeprecatedStatus groups all the status fields that are deprecated and will be removed when support for v1beta1 will be dropped.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type ExtensionConfigV1Beta1DeprecatedStatus struct {
	// conditions defines current service state of the ExtensionConfig.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

// ExtensionHandler specifies the details of a handler for a particular runtime hook registered by an Extension server.
type ExtensionHandler struct {
	// name is the unique name of the ExtensionHandler.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	Name string `json:"name,omitempty"`

	// requestHook defines the versioned runtime hook which this ExtensionHandler serves.
	// +required
	RequestHook GroupVersionHook `json:"requestHook,omitempty,omitzero"`

	// timeoutSeconds defines the timeout duration for client calls to the ExtensionHandler.
	// Defaults to 10 if not set.
	// +optional
	// +kubebuilder:validation:Minimum=1
	TimeoutSeconds int32 `json:"timeoutSeconds,omitempty"`

	// failurePolicy defines how failures in calls to the ExtensionHandler should be handled by a client.
	// Defaults to Fail if not set.
	// +optional
	FailurePolicy FailurePolicy `json:"failurePolicy,omitempty"`
}

// GroupVersionHook defines the runtime hook when the ExtensionHandler is called.
type GroupVersionHook struct {
	// apiVersion is the group and version of the Hook.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	APIVersion string `json:"apiVersion,omitempty"`

	// hook is the name of the hook.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Hook string `json:"hook,omitempty"`
}

// FailurePolicy specifies how unrecognized errors when calling the ExtensionHandler are handled.
// FailurePolicy helps with extensions not working consistently, e.g. due to an intermittent network issue.
// The following type of errors are never ignored by FailurePolicy Ignore:
// - Misconfigurations (e.g. incompatible types)
// - Extension explicitly returns a Status Failure.
// +kubebuilder:validation:Enum=Ignore;Fail
type FailurePolicy string

const (
	// FailurePolicyIgnore means that an error when calling the extension is ignored.
	FailurePolicyIgnore FailurePolicy = "Ignore"

	// FailurePolicyFail means that an error when calling the extension is propagated as an error.
	FailurePolicyFail FailurePolicy = "Fail"
)

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=extensionconfigs,shortName=ext,scope=Cluster,categories=cluster-api
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Paused",type="string",JSONPath=`.status.conditions[?(@.type=="Paused")].status`,description="Reconciliation paused",priority=10
// +kubebuilder:printcolumn:name="Discovered",type="string",JSONPath=`.status.conditions[?(@.type=="Discovered")].status`,description="ExtensionConfig discovered"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of ExtensionConfig"

// ExtensionConfig is the Schema for the ExtensionConfig API.
// NOTE: This CRD can only be used if the RuntimeSDK feature gate is enabled.
type ExtensionConfig struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec is the desired state of the ExtensionConfig.
	// +required
	Spec ExtensionConfigSpec `json:"spec,omitempty,omitzero"`

	// status is the current state of the ExtensionConfig
	// +optional
	Status ExtensionConfigStatus `json:"status,omitempty,omitzero"`
}

// GetV1Beta1Conditions returns the set of conditions for this object.
func (m *ExtensionConfig) GetV1Beta1Conditions() clusterv1.Conditions {
	if m.Status.Deprecated == nil || m.Status.Deprecated.V1Beta1 == nil {
		return nil
	}
	return m.Status.Deprecated.V1Beta1.Conditions
}

// SetV1Beta1Conditions sets the conditions on this object.
func (m *ExtensionConfig) SetV1Beta1Conditions(conditions clusterv1.Conditions) {
	if m.Status.Deprecated == nil {
		m.Status.Deprecated = &ExtensionConfigDeprecatedStatus{}
	}
	if m.Status.Deprecated.V1Beta1 == nil {
		m.Status.Deprecated.V1Beta1 = &ExtensionConfigV1Beta1DeprecatedStatus{}
	}
	m.Status.Deprecated.V1Beta1.Conditions = conditions
}

// GetConditions returns the set of conditions for this object.
func (m *ExtensionConfig) GetConditions() []metav1.Condition {
	return m.Status.Conditions
}

// SetConditions sets conditions for an API object.
func (m *ExtensionConfig) SetConditions(conditions []metav1.Condition) {
	m.Status.Conditions = conditions
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
	// ExtensionConfigDiscoveredCondition is true if the runtime extension has been successfully discovered.
	ExtensionConfigDiscoveredCondition = "Discovered"

	// ExtensionConfigDiscoveredReason surfaces that the runtime extension has been successfully discovered.
	ExtensionConfigDiscoveredReason = "Discovered"

	// ExtensionConfigNotDiscoveredReason surfaces that the runtime extension has not been successfully discovered.
	ExtensionConfigNotDiscoveredReason = "NotDiscovered"
)

const (
	// RuntimeExtensionDiscoveredV1Beta1Condition is a condition set on an ExtensionConfig object once it has been discovered by the Runtime SDK client.
	RuntimeExtensionDiscoveredV1Beta1Condition clusterv1.ConditionType = "Discovered"

	// DiscoveryFailedV1Beta1Reason documents failure of a Discovery call.
	DiscoveryFailedV1Beta1Reason string = "DiscoveryFailed"

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
