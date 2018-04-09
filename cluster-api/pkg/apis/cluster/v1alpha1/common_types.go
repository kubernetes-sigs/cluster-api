/*
Copyright 2018 The Kubernetes Authors.

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

import runtime "k8s.io/apimachinery/pkg/runtime"

// ProviderConfig defines the configuration to use during node creation.
type ProviderConfig struct {

	// No more than one of the following may be specified.

	// Value is an inlined, serialized representation of the resource
	// configuration. It is recommended that providers maintain their own
	// versioned API types that should be serialized/deserialized from this
	// field, akin to component config.
	// +optional
	Value *runtime.RawExtension `json:"value,omitempty"`

	// Source for the provider configuration. Cannot be used if value is
	// not empty.
	// +optional
	ValueFrom *ProviderConfigSource `json:valueFrom,omitempty`
}

// ProviderConfigSource represents a source for the provider-specific
// resource configuration.
type ProviderConfigSource struct {
	// TODO(roberthbailey): Fill these in later
}
