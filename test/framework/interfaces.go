/*
Copyright 2019 The Kubernetes Authors.

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

package framework

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Interfaces to scope down client.Client.

// Getter can get resources.
type Getter interface {
	Get(ctx context.Context, key client.ObjectKey, obj client.Object) error
}

// Creator can creates resources.
type Creator interface {
	Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error
}

// Lister can lists resources.
type Lister interface {
	List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error
}

// Deleter can delete resources.
type Deleter interface {
	Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error
}

// GetLister can get and list resources.
type GetLister interface {
	Getter
	Lister
}
