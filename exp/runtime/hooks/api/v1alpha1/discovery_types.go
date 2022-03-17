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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/cluster-api/internal/runtime/catalog"
)

type FailurePolicy string

const (
	// FailurePolicyIgnore means that an error calling the extension is ignored.
	FailurePolicyIgnore FailurePolicy = "Ignore"

	// FailurePolicyFail means that an error calling the extension causes the admission to fail.
	FailurePolicyFail FailurePolicy = "Fail"
)

type Hook struct {
	// APIVersion is the Version of the Hook
	APIVersion string `json:"apiVersion"`

	// Name is the name of the hook
	Name string `json:"name"`
}

type RuntimeExtension struct {
	// Name is the name of the RuntimeExtension
	Name string `json:"name"`

	// Hook defines the specific runtime event for which this RuntimeExtension calls.
	Hook Hook `json:"hook"`

	// TimeoutSeconds defines the timeout duration for client calls to the Hook
	TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty"`

	// FailurePolicy defines how failures in calls to the Hook should be handled by a client.
	FailurePolicy *FailurePolicy `json:"failurePolicy,omitempty"`
}

// DiscoveryHookRequest foo bar baz.
// +kubebuilder:object:root=true
type DiscoveryHookRequest struct {
	metav1.TypeMeta `json:",inline"`
}

// DiscoveryHookResponse foo bar baz.
// +kubebuilder:object:root=true
type DiscoveryHookResponse struct {
	metav1.TypeMeta `json:",inline"`

	// Status of the call. One of "Success" or "Failure".
	Status ResponseStatus `json:"status"`

	// A human-readable description of the status of the call.
	Message string `json:"message"`

	Extensions []RuntimeExtension `json:"extensions"`
}

func Discovery(*DiscoveryHookRequest, *DiscoveryHookResponse) {}

func init() {
	catalogBuilder.RegisterHook(Discovery, &catalog.HookMeta{
		Tags:        []string{"Discovery"},
		Summary:     "Discovery endpoint",
		Description: "Discovery endpoint discovers...",
		Singleton:   true,
	})
}
