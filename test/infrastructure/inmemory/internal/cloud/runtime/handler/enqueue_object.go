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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"

	cevent "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/internal/cloud/runtime/event"
	creconciler "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/internal/cloud/runtime/reconcile"
)

var _ EventHandler = &EnqueueRequestForObject{}

// EnqueueRequestForObject enqueues a Request containing the ResourceGroup, Name and Namespace of the object that is the source of the Event.
// handler.EnqueueRequestForObject is used by almost all Controllers that have associated Resources to reconcile.
type EnqueueRequestForObject struct{}

// Create implements EventHandler.
func (e *EnqueueRequestForObject) Create(evt cevent.CreateEvent, q workqueue.RateLimitingInterface) {
	if evt.Object == nil {
		return
	}
	q.Add(creconciler.Request{
		ResourceGroup: evt.ResourceGroup,
		NamespacedName: types.NamespacedName{
			Namespace: evt.Object.GetNamespace(),
			Name:      evt.Object.GetName(),
		},
	})
}

// Update implements EventHandler.
func (e *EnqueueRequestForObject) Update(evt cevent.UpdateEvent, q workqueue.RateLimitingInterface) {
	switch {
	case evt.ObjectNew != nil:
		q.Add(creconciler.Request{
			ResourceGroup: evt.ResourceGroup,
			NamespacedName: types.NamespacedName{
				Namespace: evt.ObjectNew.GetNamespace(),
				Name:      evt.ObjectNew.GetName(),
			},
		})
	case evt.ObjectOld != nil:
		q.Add(creconciler.Request{
			ResourceGroup: evt.ResourceGroup,
			NamespacedName: types.NamespacedName{
				Namespace: evt.ObjectOld.GetNamespace(),
				Name:      evt.ObjectOld.GetName(),
			},
		})
	}
}

// Delete implements EventHandler.
func (e *EnqueueRequestForObject) Delete(evt cevent.DeleteEvent, q workqueue.RateLimitingInterface) {
	if evt.Object == nil {
		return
	}
	q.Add(creconciler.Request{
		ResourceGroup: evt.ResourceGroup,
		NamespacedName: types.NamespacedName{
			Namespace: evt.Object.GetNamespace(),
			Name:      evt.Object.GetName(),
		},
	})
}

// Generic implements EventHandler.
func (e *EnqueueRequestForObject) Generic(evt cevent.GenericEvent, q workqueue.RateLimitingInterface) {
	if evt.Object == nil {
		return
	}
	q.Add(creconciler.Request{
		ResourceGroup: evt.ResourceGroup,
		NamespacedName: types.NamespacedName{
			Namespace: evt.Object.GetNamespace(),
			Name:      evt.Object.GetName(),
		},
	})
}
