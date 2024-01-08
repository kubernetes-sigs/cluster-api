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
	"time"

	"github.com/emicklei/go-restful/v3"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Event records a lifecycle event for a Kubernetes object.
type Event struct {
	Type   watch.EventType `json:"type,omitempty"`
	Object runtime.Object  `json:"object,omitempty"`
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
	queryTimeout := req.QueryParameter("timeoutSeconds")
	c := h.manager.GetCache()
	i, err := c.GetInformerForKind(ctx, gvk)
	if err != nil {
		return err
	}
	h.log.Info(fmt.Sprintf("Serving Watch for %v", req.Request.URL))
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

	// Defer cleanup which removes the event handler and ensures the channel is empty of events.
	defer func() {
		// Doing this to ensure the channel is empty.
		// This reduces the probability of a deadlock when removing the event handler.
	L:
		for {
			select {
			case <-events:
			default:
				break L
			}
		}
		reterr = i.RemoveEventHandler(watcher)
		// Note: After we removed the handler, no new events will be written to the events channel.
	}()

	return watcher.Run(ctx, queryTimeout, resp)
}

// Run serves a series of encoded events via HTTP with Transfer-Encoding: chunked.
func (m *WatchEventDispatcher) Run(ctx context.Context, timeout string, w http.ResponseWriter) error {
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
	flusher.Flush()

	timeoutTimer, seconds, err := setTimer(timeout)
	if err != nil {
		return errors.Wrapf(err, "can't start Watch: could not set timeout")
	}

	ctx, cancel := context.WithTimeout(ctx, seconds)
	defer cancel()
	defer timeoutTimer.Stop()
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
			if err := resp.WriteEntity(event); err != nil {
				_ = resp.WriteErrorString(http.StatusInternalServerError, err.Error())
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
