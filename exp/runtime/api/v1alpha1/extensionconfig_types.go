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
	// ClientConfig defines how to communicate with the Extension server.
	ClientConfig ClientConfig `json:"clientConfig"`

	// NamespaceSelector decides whether to call the hook for an object based
	// on whether the namespace for that object matches the selector.
	// Defaults to the empty LabelSelector, which matches all objects.
	// +optional
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`

	// Settings defines key value pairs to be passed to all calls
	// to all supported RuntimeExtensions.
	// Note: Settings can be overridden on the ClusterClass.
	// +optional
	Settings map[string]string `json:"settings,omitempty"`
}

// ClientConfig contains the information to make a client
// connection with an Extension server.
type ClientConfig struct {
	// URL gives the location of the Extension server, in standard URL form
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
	URL *string `json:"url,omitempty"`

	// Service is a reference to the Kubernetes service for the Extension server.
	// Note: Exactly one of `url` or `service` must be specified.
	//
	// If the Extension server is running within a cluster, then you should use `service`.
	//
	// +optional
	Service *ServiceReference `json:"service,omitempty"`

	// CABundle is a PEM encoded CA bundle which will be used to validate the Extension server's server certificate.
	// +optional
	CABundle []byte `json:"caBundle,omitempty"`
}

// ServiceReference holds a reference to a Kubernetes Service of an Extension server.
type ServiceReference struct {
	// Namespace is the namespace of the service.
	Namespace string `json:"namespace"`

	// Name is the name of the service.
	Name string `json:"name"`

	// Path is an optional URL path and if present may be any string permissible in
	// a URL. If a path is set it will be used as prefix to the hook-specific path.
	// +optional
	Path *string `json:"path,omitempty"`

	// Port is the port on the service that's hosting the Extension server.
	// Defaults to 443.
	// Port should be a valid port number (1-65535, inclusive).
	// +optional
	Port *int32 `json:"port,omitempty"`
}

// ANCHOR_END: ExtensionConfigSpec

// ANCHOR: ExtensionConfigStatus

// ExtensionConfigStatus defines the observed state of ExtensionConfig.
type ExtensionConfigStatus struct {
	// Handlers defines the current ExtensionHandlers supported by an Extension.
	// +optional
	// +listType=map
	// +listMapKey=name
	Handlers []ExtensionHandler `json:"handlers,omitempty"`

	// Conditions define the current service state of the ExtensionConfig.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

// ExtensionHandler specifies the details of a handler for a particular runtime hook registered by an Extension server.
type ExtensionHandler struct {
	// Name is the unique name of the ExtensionHandler.
	Name string `json:"name"`

	// RequestHook defines the versioned runtime hook which this ExtensionHandler serves.
	RequestHook GroupVersionHook `json:"requestHook"`

	// TimeoutSeconds defines the timeout duration for client calls to the ExtensionHandler.
	// Defaults to 10 is not set.
	// +optional
	TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty"`

	// FailurePolicy defines how failures in calls to the ExtensionHandler should be handled by a client.
	// Defaults to Fail if not set.
	// +optional
	FailurePolicy *FailurePolicy `json:"failurePolicy,omitempty"`
}

// GroupVersionHook defines the runtime hook when the ExtensionHandler is called.
type GroupVersionHook struct {
	// APIVersion is the group and version of the Hook.
	APIVersion string `json:"apiVersion"`

	// Hook is the name of the hook.
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
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// ExtensionConfigSpec is the desired state of the ExtensionConfig
	Spec ExtensionConfigSpec `json:"spec,omitempty"`

	// ExtensionConfigStatus is the current state of the ExtensionConfig
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

// +kubebuilder:object:root=true

// ExtensionConfigList contains a list of ExtensionConfig.
type ExtensionConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ExtensionConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ExtensionConfig{}, &ExtensionConfigList{})
}

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
