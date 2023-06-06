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

package event

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateEvent is an event where a cloud object was created.
type CreateEvent struct {
	ResourceGroup string
	Object        client.Object
}

// UpdateEvent is an event where a cloud object was updated.
type UpdateEvent struct {
	ResourceGroup string
	ObjectOld     client.Object
	ObjectNew     client.Object
}

// DeleteEvent is an event where a cloud object was deleted.
type DeleteEvent struct {
	ResourceGroup string
	Object        client.Object
}

// GenericEvent is an event where the operation type is unknown (e.g. periodic sync).
type GenericEvent struct {
	ResourceGroup string
	Object        client.Object
}
