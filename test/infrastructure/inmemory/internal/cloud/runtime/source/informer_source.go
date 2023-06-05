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
	"fmt"

	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ccache "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/internal/cloud/runtime/cache"
	cevent "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/internal/cloud/runtime/event"
	chandler "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/internal/cloud/runtime/handler"
	cpredicate "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/internal/cloud/runtime/predicate"
)

// Informer is used to provide a source of events originating from CRUD operations inside the resourceGroup (e.g. Resource Create).
type Informer struct {
	Informer ccache.Informer
}

var _ Source = &Informer{}

// Start implements Source.
func (is *Informer) Start(_ context.Context, handler chandler.EventHandler, queue workqueue.RateLimitingInterface, prct ...cpredicate.Predicate) error {
	// Informer should have been specified by the user.
	if is.Informer == nil {
		return fmt.Errorf("must specify Informer.Informer")
	}

	return is.Informer.AddEventHandler(informerEventHandler{Queue: queue, EventHandler: handler, Predicates: prct})
}

func (is *Informer) String() string {
	return fmt.Sprintf("informer source: %p", is.Informer)
}

var _ ccache.InformEventHandler = informerEventHandler{}

// informerEventHandler handle events originated by a source.
type informerEventHandler struct {
	EventHandler chandler.EventHandler
	Queue        workqueue.RateLimitingInterface
	Predicates   []cpredicate.Predicate
}

// OnCreate creates CreateEvent and calls Create on EventHandler.
func (e informerEventHandler) OnCreate(resourceGroup string, obj client.Object) {
	c := cevent.CreateEvent{
		ResourceGroup: resourceGroup,
		Object:        obj,
	}
	for _, p := range e.Predicates {
		if !p.Create(c) {
			return
		}
	}
	e.EventHandler.Create(c, e.Queue)
}

// OnUpdate creates UpdateEvent and calls Update on EventHandler.
func (e informerEventHandler) OnUpdate(resourceGroup string, oldObj, newObj client.Object) {
	u := cevent.UpdateEvent{
		ResourceGroup: resourceGroup,
		ObjectOld:     oldObj,
		ObjectNew:     newObj,
	}
	for _, p := range e.Predicates {
		if !p.Update(u) {
			return
		}
	}
	e.EventHandler.Update(u, e.Queue)
}

// OnDelete creates DeleteEvent and calls Delete on EventHandler.
func (e informerEventHandler) OnDelete(resourceGroup string, obj client.Object) {
	d := cevent.DeleteEvent{
		ResourceGroup: resourceGroup,
		Object:        obj,
	}
	for _, p := range e.Predicates {
		if !p.Delete(d) {
			return
		}
	}
	e.EventHandler.Delete(d, e.Queue)
}

// OnGeneric creates GenericEvent and calls Generic on EventHandler.
func (e informerEventHandler) OnGeneric(resourceGroup string, obj client.Object) {
	d := cevent.GenericEvent{
		ResourceGroup: resourceGroup,
		Object:        obj,
	}
	for _, p := range e.Predicates {
		if !p.Generic(d) {
			return
		}
	}
	e.EventHandler.Generic(d, e.Queue)
}
