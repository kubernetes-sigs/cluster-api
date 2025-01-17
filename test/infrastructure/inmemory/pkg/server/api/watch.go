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

package api

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/emicklei/go-restful/v3"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Event records a lifecycle event for a Kubernetes object.
type Event struct {
	Type   watch.EventType `json:"type,omitempty"`
	Object client.Object   `json:"object,omitempty"`
}

// WatchEventDispatcher dispatches events for a single resourceGroup.
type WatchEventDispatcher struct {
	resourceGroup string
	events        chan *Event
}

// OnCreate dispatches Create events.
func (m *WatchEventDispatcher) OnCreate(resourceGroup string, o client.Object) {
	if resourceGroup != m.resourceGroup {
		return
	}
	m.events <- &Event{
		Type:   watch.Added,
		Object: o,
	}
}

// OnUpdate dispatches Update events.
func (m *WatchEventDispatcher) OnUpdate(resourceGroup string, _, o client.Object) {
	if resourceGroup != m.resourceGroup {
		return
	}
	m.events <- &Event{
		Type:   watch.Modified,
		Object: o,
	}
}

// OnDelete dispatches Delete events.
func (m *WatchEventDispatcher) OnDelete(resourceGroup string, o client.Object) {
	if resourceGroup != m.resourceGroup {
		return
	}
	m.events <- &Event{
		Type:   watch.Deleted,
		Object: o,
	}
}

// OnGeneric dispatches Generic events.
func (m *WatchEventDispatcher) OnGeneric(resourceGroup string, o client.Object) {
	if resourceGroup != m.resourceGroup {
		return
	}
	m.events <- &Event{
		Type:   "GENERIC",
		Object: o,
	}
}

func (h *apiServerHandler) watchForResource(req *restful.Request, resp *restful.Response, resourceGroup string, gvk schema.GroupVersionKind) (reterr error) {
	ctx := req.Request.Context()
	log := h.log.WithValues("resourceGroup", resourceGroup, "gvk", gvk.String())
	ctx = ctrl.LoggerInto(ctx, log)
	queryTimeout := req.QueryParameter("timeoutSeconds")
	resourceVersion := req.QueryParameter("resourceVersion")
	c := h.manager.GetCache()
	i, err := c.GetInformerForKind(ctx, gvk)
	if err != nil {
		return err
	}
	// With an unbuffered event channel RemoveEventHandler could be blocked because it requires a lock on the informer.
	// When Run stops reading from the channel the informer could be blocked with an unbuffered chanel and then RemoveEventHandler never goes through.
	// 1000 is used to avoid deadlocks in clusters with a higher number of Machines/Nodes.
	events := make(chan *Event, 1000)
	watcher := &WatchEventDispatcher{
		resourceGroup: resourceGroup,
		events:        events,
	}

	if err := i.AddEventHandler(watcher); err != nil {
		return err
	}

	initialEvents := []Event{}
	if resourceVersion != "" {
		parsedResourceVersion, err := strconv.ParseUint(resourceVersion, 10, 64)
		if err != nil {
			return err
		}

		// Get at client to the resource group and list all relevant objects.
		inmemoryClient := h.manager.GetResourceGroup(resourceGroup).GetClient()
		list, err := h.apiV1list(ctx, req, gvk, inmemoryClient)
		if err != nil {
			return err
		}

		// Sort the objects by resourceVersion to later write the events in order.
		sort.SliceStable(list.Items, func(i, j int) bool {
			a, err := strconv.ParseUint(list.Items[i].GetResourceVersion(), 10, 64)
			if err != nil {
				panic(err)
			}
			b, err := strconv.ParseUint(list.Items[j].GetResourceVersion(), 10, 64)
			if err != nil {
				panic(err)
			}
			return a < b
		})

		// Loop over all items and fill the list of events which were missed since the last watch.
		for _, obj := range list.Items {
			objResourceVersion, err := strconv.ParseUint(obj.GetResourceVersion(), 10, 64)
			if err != nil {
				return err
			}
			if objResourceVersion <= parsedResourceVersion {
				continue
			}
			eventType := watch.Modified
			if obj.GetGeneration() == 0 {
				eventType = watch.Added
			}
			initialEvents = append(initialEvents, Event{Type: eventType, Object: &obj})
		}
	}

	// Defer cleanup which removes the event handler and ensures the channel is empty of events.
	defer func() {
		// Doing this to ensure the channel is empty.
		// This reduces the probability of a deadlock when removing the event handler.
	L:
		for {
			select {
			case event, ok := <-events:
				if !ok {
					// End of results.
					break L
				}
				log.V(4).Info("Missed event", "eventType", event.Type, "objectName", event.Object.GetName(), "resourceVersion", event.Object.GetResourceVersion())
			default:
				break L
			}
		}
		reterr = i.RemoveEventHandler(watcher)
		// Note: After we removed the handler, no new events will be written to the events channel.
	}()

	return watcher.Run(ctx, queryTimeout, initialEvents, resp)
}

// Run serves a series of encoded events via HTTP with Transfer-Encoding: chunked.
func (m *WatchEventDispatcher) Run(ctx context.Context, timeout string, initialEvents []Event, w http.ResponseWriter) error {
	log := ctrl.LoggerFrom(ctx)
	flusher, ok := w.(http.Flusher)
	if !ok {
		return errors.New("can't start Watch: can't get http.Flusher")
	}
	resp, ok := w.(*restful.Response)
	if !ok {
		return errors.New("can't start Watch: can't get restful.Response")
	}
	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)
	// Write all object events which happened since the last resourceVersion.
	for _, event := range initialEvents {
		if err := resp.WriteEntity(event); err != nil {
			log.Error(err, "Writing old event", "eventType", event.Type, "objectName", event.Object.GetName(), "resourceVersion", event.Object.GetResourceVersion())
			_ = resp.WriteErrorString(http.StatusInternalServerError, err.Error())
		} else {
			log.V(4).Info("Wrote old event", "eventType", event.Type, "objectName", event.Object.GetName(), "resourceVersion", event.Object.GetResourceVersion())
		}
	}
	flusher.Flush()

	timeoutTimer, seconds, err := setTimer(timeout)
	if err != nil {
		return errors.Wrapf(err, "can't start Watch: could not set timeout")
	}

	ctx, cancel := context.WithTimeout(ctx, seconds)
	defer cancel()
	defer timeoutTimer.Stop()

	// Determine the highest written resourceVersion so we can filter out duplicated events from the channel.
	minResourceVersion := uint64(0)
	if len(initialEvents) > 0 {
		minResourceVersion, err = strconv.ParseUint(initialEvents[len(initialEvents)-1].Object.GetResourceVersion(), 10, 64)
		if err != nil {
			return err
		}
		minResourceVersion++
	}

	var objResourceVersion uint64
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timeoutTimer.C:
			return nil
		case event, ok := <-m.events:
			if !ok {
				// End of results.
				return nil
			}

			// Parse and check if the object has a higher resource version than we allow.
			objResourceVersion, err = strconv.ParseUint(event.Object.GetResourceVersion(), 10, 64)
			if err != nil {
				log.Error(err, "Parsing object resource version", "eventType", event.Type, "objectName", event.Object.GetName(), "resourceVersion", event.Object.GetResourceVersion())
				_ = resp.WriteErrorString(http.StatusInternalServerError, err.Error())
				continue
			}

			// Skip objects which were already written.
			if objResourceVersion < minResourceVersion {
				continue
			}

			if err := resp.WriteEntity(event); err != nil {
				log.Error(err, "Writing event", "eventType", event.Type, "objectName", event.Object.GetName(), "resourceVersion", event.Object.GetResourceVersion())
				_ = resp.WriteErrorString(http.StatusInternalServerError, err.Error())
			} else {
				log.V(4).Info("Wrote event", "eventType", event.Type, "objectName", event.Object.GetName(), "resourceVersion", event.Object.GetResourceVersion())
			}
			if len(m.events) == 0 {
				flusher.Flush()
			}
		}
	}
}

// setTimer creates a time.Timer with the passed `timeout` or a default timeout of 120 seconds if `timeout` is empty.
func setTimer(timeout string) (*time.Timer, time.Duration, error) {
	var defaultTimeout = 120 * time.Second
	if timeout == "" {
		t := time.NewTimer(defaultTimeout)
		return t, defaultTimeout, nil
	}
	seconds, err := time.ParseDuration(fmt.Sprintf("%ss", timeout))
	if err != nil {
		return nil, 0, errors.Wrapf(err, "Could not parse request timeout %s", timeout)
	}
	t := time.NewTimer(seconds)
	return t, seconds, nil
}
