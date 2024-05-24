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
	"net/http"

	"github.com/emicklei/go-restful/v3"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"

	inmemoryruntime "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/pkg/runtime"
)

// DebugInfoProvider defines the methods the server must implement
// to provide debug info.
type DebugInfoProvider interface {
	ListListeners() map[string]string
}

// NewDebugHandler returns an http.Handler for debugging the server.
func NewDebugHandler(manager inmemoryruntime.Manager, log logr.Logger, infoProvider DebugInfoProvider) http.Handler {
	debugServer := &debugHandler{
		container:    restful.NewContainer(),
		manager:      manager,
		log:          log,
		infoProvider: infoProvider,
	}

	ws := new(restful.WebService)
	ws.Produces(runtime.ContentTypeJSON)

	// Discovery endpoints
	ws.Route(ws.GET("/listeners").To(debugServer.listenersList))

	debugServer.container.Add(ws)

	return debugServer
}

type debugHandler struct {
	container    *restful.Container
	manager      inmemoryruntime.Manager
	log          logr.Logger
	infoProvider DebugInfoProvider
}

func (h *debugHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.container.ServeHTTP(w, r)
}

func (h *debugHandler) listenersList(_ *restful.Request, resp *restful.Response) {
	listeners := h.infoProvider.ListListeners()

	if err := resp.WriteEntity(listeners); err != nil {
		_ = resp.WriteErrorString(http.StatusInternalServerError, err.Error())
		return
	}
}
