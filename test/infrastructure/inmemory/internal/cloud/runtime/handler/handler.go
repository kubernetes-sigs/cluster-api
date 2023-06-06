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

package handler

import (
	"k8s.io/client-go/util/workqueue"

	cevent "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/internal/cloud/runtime/event"
)

// EventHandler enqueues reconcile.Requests in response to events (e.g. object Create). EventHandlers map an Event
// for one object to trigger Reconciles for either the same object or different objects - e.g. if there is an
// Event for object with type Foo then reconcile one or more object(s) with type Bar.
type EventHandler interface {
	// Create is called in response to an create event - e.g. VM Creation.
	Create(cevent.CreateEvent, workqueue.RateLimitingInterface)

	// Update is called in response to an update event -  e.g. VM Updated.
	Update(cevent.UpdateEvent, workqueue.RateLimitingInterface)

	// Delete is called in response to a delete event - e.g. VM Deleted.
	Delete(cevent.DeleteEvent, workqueue.RateLimitingInterface)

	// Generic is called in response to an event of an unknown type or a synthetic event triggered as a cron or
	// external trigger request - e.g. reconcile re-sync.
	Generic(cevent.GenericEvent, workqueue.RateLimitingInterface)
}
