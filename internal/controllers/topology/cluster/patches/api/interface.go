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

// Package api contains the API definition for the patch engine.
// NOTE: We are introducing this API as a decoupling layer between the patch engine and the concrete components
// responsible for generating patches, because we aim to provide support for external patches in a future iteration.
// We also assume that this API and all the related types will be moved in a separate (versioned) package thus
// providing a versioned contract between Cluster API and the components implementing external patch extensions.
package api

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
)

// Generator defines a component that can generate patches for ClusterClass templates.
type Generator interface {
	// Generate generates patches for templates.
	// GeneratePatchesRequest contains templates and the corresponding variables.
	// GeneratePatchesResponse contains the patches which should be applied to the templates of the GenerateRequest.
	Generate(context.Context, client.Object, *runtimehooksv1.GeneratePatchesRequest) (*runtimehooksv1.GeneratePatchesResponse, error)
}

// Validator defines a component that can validate ClusterClass templates.
type Validator interface {
	// Validate validates templates..
	// ValidateTopologyRequest contains templates and the corresponding variables.
	// ValidateTopologyResponse contains the validation response.
	Validate(context.Context, client.Object, *runtimehooksv1.ValidateTopologyRequest) (*runtimehooksv1.ValidateTopologyResponse, error)
}
