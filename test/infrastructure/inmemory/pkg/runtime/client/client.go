/*
Copyright 2023 The Kubernetes Authors.

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

package client

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Reader knows how to read and list resources in a resource group.
type Reader interface {
	// Get retrieves a resource for the given object key.
	Get(ctx context.Context, key client.ObjectKey, obj client.Object) error

	// List retrieves list of objects for a given namespace and list options.
	List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error
}

// Writer knows how to create, delete, and update resources in a resource group.
type Writer interface {
	// Create saves a resource in a resource group.
	Create(ctx context.Context, obj client.Object) error

	// Delete deletes a resource from a resource group.
	Delete(ctx context.Context, obj client.Object) error

	// Update updates a resource in a resource group.
	Update(ctx context.Context, obj client.Object) error

	// Patch patches a resource in a resource group.
	Patch(ctx context.Context, obj client.Object, patch client.Patch) error
}

// Client knows how to perform CRUD operations on resources in a resource group.
type Client interface {
	Reader
	Writer
}
