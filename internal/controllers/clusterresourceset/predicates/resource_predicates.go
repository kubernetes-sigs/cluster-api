/*
Copyright 2020 The Kubernetes Authors.

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

// Package predicates implements predicate functionality.
package predicates

import (
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// TypedResourceCreateOrUpdate returns a predicate that returns true for create and update events.
func TypedResourceCreateOrUpdate[T client.Object](_ logr.Logger) predicate.TypedFuncs[T] {
	return predicate.TypedFuncs[T]{
		CreateFunc:  func(event.TypedCreateEvent[T]) bool { return true },
		UpdateFunc:  func(event.TypedUpdateEvent[T]) bool { return true },
		DeleteFunc:  func(event.TypedDeleteEvent[T]) bool { return false },
		GenericFunc: func(event.TypedGenericEvent[T]) bool { return false },
	}
}
