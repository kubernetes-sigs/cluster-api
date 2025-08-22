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

	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
)

// DefaultHandlersTimeoutSeconds defines the default timeout duration for client calls to ExtensionHandlers.
const DefaultHandlersTimeoutSeconds = 10

// DiscoveryRequest is the request of the Discovery hook.
// +kubebuilder:object:root=true
type DiscoveryRequest struct {
	metav1.TypeMeta `json:",inline"`
}

var _ ResponseObject = &DiscoveryResponse{}

// DiscoveryResponse is the response of the Discovery hook.
// +kubebuilder:object:root=true
type DiscoveryResponse struct {
	metav1.TypeMeta `json:",inline"`

	// CommonResponse contains Status and Message fields common to all response types.
	CommonResponse `json:",inline"`

	// handlers defines the current ExtensionHandlers supported by an Extension.
	// +listType=map
	// +listMapKey=name
	// +optional
	Handlers []ExtensionHandler `json:"handlers,omitempty"`
}

// ExtensionHandler represents the discovery information for an extension handler which includes
// the hook it supports.
type ExtensionHandler struct {
	// name is the name of the ExtensionHandler.
	// +required
	Name string `json:"name"`

	// requestHook defines the versioned runtime hook which this ExtensionHandler serves.
	// +required
	RequestHook GroupVersionHook `json:"requestHook"`

	// timeoutSeconds defines the timeout duration for client calls to the ExtensionHandler.
	// This is defaulted to 10 if left undefined.
	// +optional
	TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty"`

	// failurePolicy defines how failures in calls to the ExtensionHandler should be handled by a client.
	// This is defaulted to FailurePolicyFail if not defined.
	// +optional
	FailurePolicy *FailurePolicy `json:"failurePolicy,omitempty"`
}

// GroupVersionHook defines the runtime hook when the ExtensionHandler is called.
type GroupVersionHook struct {
	// apiVersion is the group and version of the Hook
	// +required
	APIVersion string `json:"apiVersion"`

	// hook is the name of the hook
	// +required
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

	// FailurePolicyFail means that an error when calling the extension is not ignored.
	FailurePolicyFail FailurePolicy = "Fail"
)

// Discovery represents the discovery hook.
func Discovery(*DiscoveryRequest, *DiscoveryResponse) {}

func init() {
	catalogBuilder.RegisterHook(Discovery, &runtimecatalog.HookMeta{
		Tags:    []string{"Discovery"},
		Summary: "Cluster API Runtime will call this hook when an ExtensionConfig is reconciled",
		Description: "Cluster API Runtime will call this hook when an ExtensionConfig is reconciled. " +
			"Runtime Extension implementers must use this hook to inform the Cluster API runtime about all the handlers " +
			"that are defined in an external component implementing Runtime Extensions.\n" +
			"\n" +
			"Notes:\n" +
			"- When using Runtime SDK utils, a handler for this hook is automatically generated",
		Singleton: true,
	})
}
