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

	runtimecatalog "sigs.k8s.io/cluster-api/internal/runtime/catalog"
)

// DiscoveryRequest represents the object of a discovery request.
// +kubebuilder:object:root=true
type DiscoveryRequest struct {
	metav1.TypeMeta `json:",inline"`
}

var _ ResponseObject = &DiscoveryResponse{}

// DiscoveryResponse represents the object received as a discovery response.
// +kubebuilder:object:root=true
type DiscoveryResponse struct {
	metav1.TypeMeta `json:",inline"`

	CommonResponse `json:",inline"`
	// Handlers defines the current ExtensionHandlers supported by an Extension.
	// +listType=map
	// +listMapKey=name
	Handlers []ExtensionHandler `json:"handlers"`
}

// ExtensionHandler represents the discovery information of the extension which includes
// the hook it supports.
type ExtensionHandler struct {
	// Name is the name of the ExtensionHandler.
	Name string `json:"name"`

	// RequestHook defines the versioned runtime hook which this ExtensionHandler serves.
	RequestHook GroupVersionHook `json:"requestHook"`

	// TimeoutSeconds defines the timeout duration for client calls to the ExtensionHandler.
	TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty"`

	// FailurePolicy defines how failures in calls to the ExtensionHandler should be handled by a client.
	FailurePolicy *FailurePolicy `json:"failurePolicy,omitempty"`
}

// GroupVersionHook defines the runtime hook when the ExtensionHandler is called.
type GroupVersionHook struct {
	// APIVersion is the Version of the Hook
	APIVersion string `json:"apiVersion"`

	// Hook is the name of the hook
	Hook string `json:"hook"`
}

// FailurePolicy specifies how unrecognized errors from the admission endpoint are handled.
// FailurePolicy helps with extensions not working consistently, e.g. due to an intermittent network issue.
// The following type of errors are always considered blocking Failures:
// - Misconfigurations (e.g. incompatible types)
// - Extension explicitly reports a Status Failure.
type FailurePolicy string

const (
	// FailurePolicyIgnore means that an error calling the extension is ignored.
	FailurePolicyIgnore FailurePolicy = "Ignore"

	// FailurePolicyFail means that an error calling the extension is not ignored.
	FailurePolicyFail FailurePolicy = "Fail"
)

// Discovery represents the discovery hook.
func Discovery(*DiscoveryRequest, *DiscoveryResponse) {}

func init() {
	catalogBuilder.RegisterHook(Discovery, &runtimecatalog.HookMeta{
		Tags:        []string{"Discovery"},
		Summary:     "Discovery endpoint",
		Description: "Discovery endpoint discovers the supported hooks of a RuntimeExtension",
		Singleton:   true,
	})
}
