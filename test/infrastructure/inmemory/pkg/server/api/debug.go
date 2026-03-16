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
	"io"
	"net/http"
	"strings"

	"github.com/emicklei/go-restful/v3"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"

	inmemoryruntime "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/pkg/runtime"
)

// DebugInfoProvider defines the methods the server must implement
// to provide debug info.
type DebugInfoProvider interface {
	ListListeners() map[string]string
	GetCluster(wclName string) *ClusterDebugInfo
	StartListener(wclName string) error
	StopListener(wclName string) error
}

// ClusterDebugInfo defines debug info for a cluster.
type ClusterDebugInfo struct {
	Host           string
	Port           int32
	ListenerActive bool
	ResourceGroup  string
	APIServers     []string
	EtcdMembers    []string
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
	ws.Route(ws.GET("/").To(debugServer.ping))
	ws.Route(ws.GET("/listeners").To(debugServer.listenersList))
	ws.Route(ws.GET("/clusters/{namespace}/{name}").To(debugServer.getCluster))
	ws.Route(ws.POST("/clusters/{namespace}/{name}/listener").To(debugServer.startOrStopListener))

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

func (h *debugHandler) ping(_ *restful.Request, resp *restful.Response) {
	if err := resp.WriteEntity("ok"); err != nil {
		_ = resp.WriteErrorString(http.StatusInternalServerError, err.Error())
		return
	}
}

func (h *debugHandler) getCluster(req *restful.Request, resp *restful.Response) {
	clusterDebugInfo := h.infoProvider.GetCluster(klog.KRef(req.PathParameter("namespace"), req.PathParameter("name")).String())
	if clusterDebugInfo == nil {
		_ = resp.WriteHeaderAndEntity(http.StatusNotFound, "cluster not found")
		return
	}

	if err := resp.WriteEntity(clusterDebugInfo); err != nil {
		_ = resp.WriteErrorString(http.StatusInternalServerError, err.Error())
		return
	}
}

func (h *debugHandler) startOrStopListener(req *restful.Request, resp *restful.Response) {
	defer func() { _ = req.Request.Body.Close() }()

	reqBody, err := io.ReadAll(req.Request.Body)
	if err != nil {
		_ = resp.WriteHeaderAndEntity(http.StatusInternalServerError, err.Error())
		return
	}

	switch action := string(reqBody); strings.ToLower(action) {
	case "start":
		if err := h.infoProvider.StartListener(klog.KRef(req.PathParameter("namespace"), req.PathParameter("name")).String()); err != nil {
			_ = resp.WriteHeaderAndEntity(http.StatusInternalServerError, err.Error())
			return
		}
	case "stop":
		if err := h.infoProvider.StopListener(klog.KRef(req.PathParameter("namespace"), req.PathParameter("name")).String()); err != nil {
			_ = resp.WriteHeaderAndEntity(http.StatusInternalServerError, err.Error())
			return
		}
	}

	h.getCluster(req, resp)
}
