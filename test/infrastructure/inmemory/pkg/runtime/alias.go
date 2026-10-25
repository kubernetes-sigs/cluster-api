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

package runtime

import (
	"sigs.k8s.io/controller-runtime/pkg/client"

	inmemoryclient "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/pkg/runtime/client"
	inmemorymanager "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/pkg/runtime/manager"
)

// Client knows how to perform CRUD operations on resources in a resource group.
type Client inmemoryclient.Client

// Object represents an object.
type Object client.Object

// Manager initializes shared dependencies such as Caches and Clients, and provides them to Runnables.
// A Manager is required to create Controllers.
type Manager inmemorymanager.Manager

var (
	// NewManager returns a new Manager for creating Controllers.
	NewManager = inmemorymanager.New
)
