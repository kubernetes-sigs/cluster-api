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

package structuredmerge

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PatchHelperFactoryFunc defines a func that returns a new PatchHelper.
type PatchHelperFactoryFunc func(ctx context.Context, original, modified client.Object, opts ...HelperOption) (PatchHelper, error)

// PatchHelper define the behavior for component responsible for managing  patches for Kubernetes objects
// owned by the topology controller.
// NOTE: this interface is required to allow to plug in different PatchHelper implementations, because the
// default one is based on server side apply, and it cannot be used for topology dry run, so we have a
// minimal viable replacement based on two-ways merge.
type PatchHelper interface {
	// HasChanges return true if the modified object is generating changes vs the original object.
	HasChanges() bool

	// HasSpecChanges return true if the modified object is generating spec changes vs the original object.
	HasSpecChanges() bool

	// Changes returns the changes vs the original object.
	Changes() []byte

	// Patch patches the given obj in the Kubernetes cluster.
	Patch(ctx context.Context) error
}
