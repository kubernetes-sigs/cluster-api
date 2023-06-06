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

package source

import (
	"context"

	"k8s.io/client-go/util/workqueue"

	chandler "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/internal/cloud/runtime/handler"
	cpredicate "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/internal/cloud/runtime/predicate"
)

// Source is a source of events (e.g. Create, Update, Delete operations on resources)
// which should be processed by event.EventHandlers to enqueue reconcile.Requests.
type Source interface {
	// Start is internal and should be called only by the Controller to register an EventHandler with the Informer
	// to enqueue reconcile.Requests.
	Start(context.Context, chandler.EventHandler, workqueue.RateLimitingInterface, ...cpredicate.Predicate) error
}
