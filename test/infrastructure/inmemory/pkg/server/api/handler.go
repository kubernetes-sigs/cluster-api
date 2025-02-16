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
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/emicklei/go-restful/v3"
	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/apimachinery/pkg/util/httpstream/spdy"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/endpoints/metrics"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/server"
	"k8s.io/client-go/tools/portforward"
	"sigs.k8s.io/controller-runtime/pkg/client"

	inmemoryruntime "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/pkg/runtime"
	inmemoryclient "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/pkg/runtime/client"
	inmemoryportforward "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/pkg/server/api/portforward"
)

// ResourceGroupResolver defines a func that can identify which workloadCluster/resourceGroup a
// request targets.
type ResourceGroupResolver func(host string) (string, error)

// NewAPIServerHandler returns an http.Handler for a fake API server.
func NewAPIServerHandler(manager inmemoryruntime.Manager, log logr.Logger, resolver ResourceGroupResolver) http.Handler {
	apiServer := &apiServerHandler{
		container:             restful.NewContainer(),
		manager:               manager,
		log:                   log,
		resourceGroupResolver: resolver,
		requestInfoResolver: server.NewRequestInfoResolver(&server.Config{
			LegacyAPIGroupPrefixes: sets.NewString(server.DefaultLegacyAPIPrefix),
		}),
	}

	apiServer.container.Filter(apiServer.globalLogging)

	ws := new(restful.WebService)
	ws.Consumes(runtime.ContentTypeJSON)
	ws.Produces(runtime.ContentTypeJSON)

	// Health check
	ws.Route(ws.GET("/").To(apiServer.healthz))

	// Discovery endpoints
	ws.Route(ws.GET("/api").To(apiServer.apiDiscovery))
	ws.Route(ws.GET("/api/v1").To(apiServer.apiV1Discovery))
	ws.Route(ws.GET("/apis").To(apiServer.apisDiscovery))
	ws.Route(ws.GET("/apis/{group}/{version}").To(apiServer.apisDiscovery))

	// CRUD endpoints (global objects)
	ws.Route(ws.POST("/api/v1/{resource}").Consumes(runtime.ContentTypeProtobuf).To(apiServer.apiV1Create))
	ws.Route(ws.GET("/api/v1/{resource}").If(isList).To(apiServer.apiV1List))
	ws.Route(ws.GET("/api/v1/{resource}").If(isWatch).To(apiServer.apiV1Watch))
	ws.Route(ws.GET("/api/v1/{resource}/{name}").To(apiServer.apiV1Get))
	ws.Route(ws.PUT("/api/v1/{resource}/{name}").Consumes(runtime.ContentTypeProtobuf).To(apiServer.apiV1Update))
	ws.Route(ws.PATCH("/api/v1/{resource}/{name}").Consumes(string(types.MergePatchType), string(types.StrategicMergePatchType)).To(apiServer.apiV1Patch))
	ws.Route(ws.DELETE("/api/v1/{resource}/{name}").Consumes(runtime.ContentTypeProtobuf, runtime.ContentTypeJSON).To(apiServer.apiV1Delete))

	ws.Route(ws.POST("/apis/{group}/{version}/{resource}").Consumes(runtime.ContentTypeProtobuf).To(apiServer.apiV1Create))
	ws.Route(ws.GET("/apis/{group}/{version}/{resource}").If(isList).To(apiServer.apiV1List))
	ws.Route(ws.GET("/apis/{group}/{version}/{resource}").If(isWatch).To(apiServer.apiV1Watch))
	ws.Route(ws.GET("/apis/{group}/{version}/{resource}/{name}").To(apiServer.apiV1Get))
	ws.Route(ws.PUT("/apis/{group}/{version}/{resource}/{name}").Consumes(runtime.ContentTypeProtobuf).To(apiServer.apiV1Update))
	ws.Route(ws.PATCH("/apis/{group}/{version}/{resource}/{name}").Consumes(string(types.MergePatchType), string(types.StrategicMergePatchType)).To(apiServer.apiV1Patch))
	ws.Route(ws.DELETE("/apis/{group}/{version}/{resource}/{name}").Consumes(runtime.ContentTypeProtobuf, runtime.ContentTypeJSON).To(apiServer.apiV1Delete))

	// CRUD endpoints (namespaced objects)
	ws.Route(ws.POST("/api/v1/namespaces/{namespace}/{resource}").Consumes(runtime.ContentTypeProtobuf).To(apiServer.apiV1Create))
	ws.Route(ws.GET("/api/v1/namespaces/{namespace}/{resource}").If(isList).To(apiServer.apiV1List))
	ws.Route(ws.GET("/api/v1/namespaces/{namespace}/{resource}").If(isWatch).To(apiServer.apiV1Watch))
	ws.Route(ws.GET("/api/v1/namespaces/{namespace}/{resource}/{name}").To(apiServer.apiV1Get))
	ws.Route(ws.PUT("/api/v1/namespaces/{namespace}/{resource}/{name}").Consumes(runtime.ContentTypeProtobuf).To(apiServer.apiV1Update))
	ws.Route(ws.PATCH("/api/v1/namespaces/{namespace}/{resource}/{name}").Consumes(string(types.MergePatchType), string(types.StrategicMergePatchType)).To(apiServer.apiV1Patch))
	ws.Route(ws.DELETE("/api/v1/namespaces/{namespace}/{resource}/{name}").Consumes(runtime.ContentTypeProtobuf, runtime.ContentTypeJSON).To(apiServer.apiV1Delete))

	ws.Route(ws.POST("/apis/{group}/{version}/namespaces/{namespace}/{resource}").Consumes(runtime.ContentTypeProtobuf).To(apiServer.apiV1Create))
	ws.Route(ws.GET("/apis/{group}/{version}/namespaces/{namespace}/{resource}").If(isList).To(apiServer.apiV1List))
	ws.Route(ws.GET("/apis/{group}/{version}/namespaces/{namespace}/{resource}").If(isWatch).To(apiServer.apiV1Watch))
	ws.Route(ws.GET("/apis/{group}/{version}/namespaces/{namespace}/{resource}/{name}").To(apiServer.apiV1Get))
	ws.Route(ws.PUT("/apis/{group}/{version}/namespaces/{namespace}/{resource}/{name}").Consumes(runtime.ContentTypeProtobuf).To(apiServer.apiV1Update))
	ws.Route(ws.PATCH("/apis/{group}/{version}/namespaces/{namespace}/{resource}/{name}").Consumes(string(types.MergePatchType), string(types.StrategicMergePatchType)).To(apiServer.apiV1Patch))
	ws.Route(ws.DELETE("/apis/{group}/{version}/namespaces/{namespace}/{resource}/{name}").Consumes(runtime.ContentTypeProtobuf, runtime.ContentTypeJSON).To(apiServer.apiV1Delete))

	// Port forward endpoints
	ws.Route(ws.GET("/api/v1/namespaces/{namespace}/pods/{name}/portforward").To(apiServer.apiV1PortForward))
	ws.Route(ws.POST("/api/v1/namespaces/{namespace}/pods/{name}/portforward").Consumes("*/*").To(apiServer.apiV1PortForward))

	apiServer.container.Add(ws)

	return apiServer
}

type apiServerHandler struct {
	container             *restful.Container
	manager               inmemoryruntime.Manager
	log                   logr.Logger
	resourceGroupResolver ResourceGroupResolver
	requestInfoResolver   *request.RequestInfoFactory
}

func (h *apiServerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.container.ServeHTTP(w, r)
}

func (h *apiServerHandler) globalLogging(req *restful.Request, resp *restful.Response, chain *restful.FilterChain) {
	h.log.V(4).Info("Serving", "method", req.Request.Method, "url", req.Request.URL, "contentType", req.HeaderParameter("Content-Type"))

	start := time.Now()

	defer func() {
		// Note: The following is based on k8s.io/apiserver/pkg/endpoints/metrics.MonitorRequest
		requestInfo, err := h.requestInfoResolver.NewRequestInfo(req.Request)
		if err != nil {
			h.log.Error(err, "Couldn't get RequestInfo from request", "url", req.Request.URL)
			requestInfo = &request.RequestInfo{Verb: req.Request.Method, Path: req.Request.URL.Path}
		}

		// Base label values which are also available in upstream kube-apiserver metrics.
		dryRun := cleanDryRun(req.Request.URL)
		scope := metrics.CleanScope(requestInfo)
		verb := metrics.CleanVerb(metrics.CanonicalVerb(strings.ToUpper(req.Request.Method), scope), req.Request, requestInfo)
		component := metrics.APIServerComponent
		baseLabelValues := []string{verb, dryRun, requestInfo.APIGroup, requestInfo.APIVersion, requestInfo.Resource, requestInfo.Subresource, scope, component}
		requestTotalLabelValues := append(baseLabelValues, strconv.Itoa(resp.StatusCode()))
		requestLatencyLabelValues := baseLabelValues

		// Additional CAPIM specific label values.
		wclName, _ := h.resourceGroupResolver(req.Request.Host)
		userAgent := req.Request.Header.Get("User-Agent")
		requestTotalLabelValues = append(requestTotalLabelValues, req.Request.Method, req.Request.Host, req.SelectedRoutePath(), wclName, userAgent)
		requestLatencyLabelValues = append(requestLatencyLabelValues, req.Request.Method, req.Request.Host, req.SelectedRoutePath(), wclName, userAgent)

		requestTotal.WithLabelValues(requestTotalLabelValues...).Inc()
		requestLatency.WithLabelValues(requestLatencyLabelValues...).Observe(time.Since(start).Seconds())
	}()

	chain.ProcessFilter(req, resp)
}

// cleanDryRun gets dryrun from a URL.
// Note: This is a copy of k8s.io/apiserver/pkg/endpoints/metrics.cleanDryRun.
func cleanDryRun(u *url.URL) string {
	// avoid allocating when we don't see dryRun in the query
	if !strings.Contains(u.RawQuery, "dryRun") {
		return ""
	}
	dryRun := u.Query()["dryRun"]
	if errs := validation.ValidateDryRun(nil, dryRun); len(errs) > 0 {
		return "invalid"
	}
	// Since dryRun could be valid with any arbitrarily long length
	// we have to dedup and sort the elements before joining them together
	// TODO: this is a fairly large allocation for what it does, consider
	//   a sort and dedup in a single pass
	return strings.Join(sets.NewString(dryRun...).List(), ",")
}

func (h *apiServerHandler) apiDiscovery(_ *restful.Request, resp *restful.Response) {
	if err := resp.WriteEntity(apiVersions); err != nil {
		_ = resp.WriteErrorString(http.StatusInternalServerError, err.Error())
		return
	}
}

func (h *apiServerHandler) apiV1Discovery(_ *restful.Request, resp *restful.Response) {
	if err := resp.WriteEntity(corev1APIResourceList); err != nil {
		_ = resp.WriteErrorString(http.StatusInternalServerError, err.Error())
		return
	}
}

func (h *apiServerHandler) apisDiscovery(req *restful.Request, resp *restful.Response) {
	if req.PathParameter("group") != "" {
		if req.PathParameter("group") == "rbac.authorization.k8s.io" && req.PathParameter("version") == "v1" {
			if err := resp.WriteEntity(rbacv1APIResourceList); err != nil {
				_ = resp.WriteErrorString(http.StatusInternalServerError, err.Error())
				return
			}
			return
		}
		if req.PathParameter("group") == "apps" && req.PathParameter("version") == "v1" {
			if err := resp.WriteEntity(appsV1ResourceList); err != nil {
				_ = resp.WriteErrorString(http.StatusInternalServerError, err.Error())
				return
			}
			return
		}
		if req.PathParameter("group") == "storage.k8s.io" && req.PathParameter("version") == "v1" {
			if err := resp.WriteEntity(storageV1ResourceList); err != nil {
				_ = resp.WriteErrorString(http.StatusInternalServerError, err.Error())
				return
			}
			return
		}

		_ = resp.WriteErrorString(http.StatusInternalServerError, fmt.Sprintf("discovery info not defined for %s/%s", req.PathParameter("group"), req.PathParameter("version")))
		return
	}

	if err := resp.WriteEntity(apiGroupList); err != nil {
		_ = resp.WriteErrorString(http.StatusInternalServerError, err.Error())
		return
	}
}

func (h *apiServerHandler) apiV1Create(req *restful.Request, resp *restful.Response) {
	ctx := req.Request.Context()

	// Gets the resource group the request targets (the resolver is aware of the mapping host<->resourceGroup)
	resourceGroup, err := h.resourceGroupResolver(req.Request.Host)
	if err != nil {
		_ = resp.WriteErrorString(http.StatusInternalServerError, err.Error())
		return
	}

	// Gets at client to the resource group.
	inmemoryClient := h.manager.GetResourceGroup(resourceGroup).GetClient()

	// Maps the requested resource to a gvk.
	gvk, err := requestToGVK(req)
	if err != nil {
		_ = resp.WriteErrorString(http.StatusInternalServerError, err.Error())
		return
	}

	// Gets the obj from the request.
	defer func() { _ = req.Request.Body.Close() }()
	// TODO: should we really ignore this error?
	objData, _ := io.ReadAll(req.Request.Body)

	newObj, err := h.manager.GetScheme().New(*gvk)
	if err != nil {
		_ = resp.WriteErrorString(http.StatusInternalServerError, err.Error())
		return
	}

	codecFactory := serializer.NewCodecFactory(h.manager.GetScheme())
	if err := runtime.DecodeInto(codecFactory.UniversalDecoder(), objData, newObj); err != nil {
		_ = resp.WriteErrorString(http.StatusInternalServerError, err.Error())
		return
	}

	// Create the object
	obj := newObj.(client.Object)
	// TODO: consider check vs enforce for namespace on the object - namespace on the request path
	obj.SetNamespace(req.PathParameter("namespace"))
	if err := inmemoryClient.Create(ctx, obj); err != nil {
		if status, ok := err.(apierrors.APIStatus); ok || errors.As(err, &status) {
			_ = resp.WriteHeaderAndEntity(int(status.Status().Code), status)
			return
		}
		_ = resp.WriteHeaderAndEntity(http.StatusInternalServerError, err.Error())
		return
	}
	if err := resp.WriteEntity(obj); err != nil {
		_ = resp.WriteErrorString(http.StatusInternalServerError, err.Error())
		return
	}
}

func (h *apiServerHandler) apiV1List(req *restful.Request, resp *restful.Response) {
	ctx := req.Request.Context()

	// Gets the resource group the request targets (the resolver is aware of the mapping host<->resourceGroup)
	resourceGroup, err := h.resourceGroupResolver(req.Request.Host)
	if err != nil {
		_ = resp.WriteErrorString(http.StatusInternalServerError, err.Error())
		return
	}

	// Gets at client to the resource group.
	inmemoryClient := h.manager.GetResourceGroup(resourceGroup).GetClient()

	// Maps the requested resource to a gvk.
	gvk, err := requestToGVK(req)
	if err != nil {
		_ = resp.WriteErrorString(http.StatusInternalServerError, err.Error())
		return
	}

	h.log.V(3).Info(fmt.Sprintf("Serving List for %v", req.Request.URL), "resourceGroup", resourceGroup)

	list, err := h.v1List(ctx, req, *gvk, inmemoryClient)
	if err != nil {
		if status, ok := err.(apierrors.APIStatus); ok || errors.As(err, &status) {
			_ = resp.WriteHeaderAndEntity(int(status.Status().Code), status)
			return
		}
		_ = resp.WriteErrorString(http.StatusInternalServerError, err.Error())
		return
	}

	if err := resp.WriteEntity(list); err != nil {
		_ = resp.WriteErrorString(http.StatusInternalServerError, err.Error())
		return
	}
}

func (h *apiServerHandler) v1List(ctx context.Context, req *restful.Request, gvk schema.GroupVersionKind, inmemoryClient inmemoryclient.Client) (*unstructured.UnstructuredList, error) {
	// Reads and returns the requested data.
	list := &unstructured.UnstructuredList{}
	list.SetAPIVersion(gvk.GroupVersion().String())
	list.SetKind(fmt.Sprintf("%sList", gvk.Kind))

	listOpts := []client.ListOption{}
	if req.PathParameter("namespace") != "" {
		listOpts = append(listOpts, client.InNamespace(req.PathParameter("namespace")))
	}

	// TODO: The only field Selector which works is for `spec.nodeName` on pods.
	fieldSelector, err := fields.ParseSelector(req.QueryParameter("fieldSelector"))
	if err != nil {
		return nil, err
	}
	if fieldSelector != nil {
		listOpts = append(listOpts, client.MatchingFieldsSelector{Selector: fieldSelector})
	}

	labelSelector, err := labels.Parse(req.QueryParameter("labelSelector"))
	if err != nil {
		return nil, err
	}
	if labelSelector != nil {
		listOpts = append(listOpts, client.MatchingLabelsSelector{Selector: labelSelector})
	}
	if err := inmemoryClient.List(ctx, list, listOpts...); err != nil {
		return nil, err
	}
	return list, nil
}

func (h *apiServerHandler) apiV1Watch(req *restful.Request, resp *restful.Response) {
	// Gets the resource group the request targets (the resolver is aware of the mapping host<->resourceGroup)
	resourceGroup, err := h.resourceGroupResolver(req.Request.Host)
	if err != nil {
		_ = resp.WriteErrorString(http.StatusInternalServerError, err.Error())
		return
	}

	// Maps the requested resource to a gvk.
	gvk, err := requestToGVK(req)
	if err != nil {
		_ = resp.WriteErrorString(http.StatusInternalServerError, err.Error())
		return
	}

	h.log.V(3).Info(fmt.Sprintf("Serving Watch for %v", req.Request.URL), "resourceGroup", resourceGroup)

	// If the request is a Watch handle it using watchForResource.
	err = h.watchForResource(req, resp, resourceGroup, *gvk)
	if err != nil {
		_ = resp.WriteErrorString(http.StatusInternalServerError, err.Error())
		return
	}
}

func (h *apiServerHandler) apiV1Get(req *restful.Request, resp *restful.Response) {
	ctx := req.Request.Context()

	// Gets the resource group the request targets (the resolver is aware of the mapping host<->resourceGroup)
	resourceGroup, err := h.resourceGroupResolver(req.Request.Host)
	if err != nil {
		_ = resp.WriteErrorString(http.StatusInternalServerError, err.Error())
		return
	}

	// Gets at client to the resource group.
	inmemoryClient := h.manager.GetResourceGroup(resourceGroup).GetClient()

	// Maps the requested resource to a gvk.
	gvk, err := requestToGVK(req)
	if err != nil {
		_ = resp.WriteErrorString(http.StatusInternalServerError, err.Error())
		return
	}

	// Reads and returns the requested data.
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(gvk.GroupVersion().String())
	obj.SetKind(gvk.Kind)
	obj.SetName(req.PathParameter("name"))
	obj.SetNamespace(req.PathParameter("namespace"))

	if err := inmemoryClient.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
		if status, ok := err.(apierrors.APIStatus); ok || errors.As(err, &status) {
			_ = resp.WriteHeaderAndEntity(int(status.Status().Code), status)
			return
		}
		_ = resp.WriteHeaderAndEntity(http.StatusInternalServerError, err.Error())
		return
	}
	if err := resp.WriteEntity(obj); err != nil {
		_ = resp.WriteErrorString(http.StatusInternalServerError, err.Error())
		return
	}
}

func (h *apiServerHandler) apiV1Update(req *restful.Request, resp *restful.Response) {
	ctx := req.Request.Context()

	// Gets the resource group the request targets (the resolver is aware of the mapping host<->resourceGroup)
	resourceGroup, err := h.resourceGroupResolver(req.Request.Host)
	if err != nil {
		_ = resp.WriteErrorString(http.StatusInternalServerError, err.Error())
		return
	}

	// Gets at client to the resource group.
	inmemoryClient := h.manager.GetResourceGroup(resourceGroup).GetClient()

	// Maps the requested resource to a gvk.
	gvk, err := requestToGVK(req)
	if err != nil {
		_ = resp.WriteErrorString(http.StatusInternalServerError, err.Error())
		return
	}

	// Gets the obj from the request.
	defer func() { _ = req.Request.Body.Close() }()
	// TODO: should we really ignore this error?
	objData, _ := io.ReadAll(req.Request.Body)

	newObj, err := h.manager.GetScheme().New(*gvk)
	if err != nil {
		_ = resp.WriteErrorString(http.StatusInternalServerError, err.Error())
		return
	}

	codecFactory := serializer.NewCodecFactory(h.manager.GetScheme())
	if err := runtime.DecodeInto(codecFactory.UniversalDecoder(), objData, newObj); err != nil {
		_ = resp.WriteErrorString(http.StatusInternalServerError, err.Error())
		return
	}

	// Update the object
	obj := newObj.(client.Object)
	// TODO: consider check vs enforce for namespace on the object - namespace on the request path
	obj.SetNamespace(req.PathParameter("namespace"))
	if err := inmemoryClient.Update(ctx, obj); err != nil {
		if status, ok := err.(apierrors.APIStatus); ok || errors.As(err, &status) {
			_ = resp.WriteHeaderAndEntity(int(status.Status().Code), status)
			return
		}
		_ = resp.WriteHeaderAndEntity(http.StatusInternalServerError, err.Error())
		return
	}
	if err := resp.WriteEntity(obj); err != nil {
		_ = resp.WriteErrorString(http.StatusInternalServerError, err.Error())
		return
	}
}

func (h *apiServerHandler) apiV1Patch(req *restful.Request, resp *restful.Response) {
	ctx := req.Request.Context()

	// Gets the resource group the request targets (the resolver is aware of the mapping host<->resourceGroup)
	resourceGroup, err := h.resourceGroupResolver(req.Request.Host)
	if err != nil {
		_ = resp.WriteErrorString(http.StatusInternalServerError, err.Error())
		return
	}

	// Gets at client to the resource group.
	inmemoryClient := h.manager.GetResourceGroup(resourceGroup).GetClient()

	// Maps the requested resource to a gvk.
	gvk, err := requestToGVK(req)
	if err != nil {
		_ = resp.WriteErrorString(http.StatusInternalServerError, err.Error())
		return
	}

	// Gets the patch from the request
	defer func() { _ = req.Request.Body.Close() }()
	// TODO: should we really ignore this error?
	patchData, _ := io.ReadAll(req.Request.Body)
	patchType := types.PatchType(req.HeaderParameter("Content-Type"))
	patch := client.RawPatch(patchType, patchData)

	// Apply the Patch.
	obj := &unstructured.Unstructured{}
	// TODO: consider check vs enforce for gvk on the object - gvk on the request path (same for name/namespace)
	obj.SetAPIVersion(gvk.GroupVersion().String())
	obj.SetKind(gvk.Kind)
	obj.SetName(req.PathParameter("name"))
	obj.SetNamespace(req.PathParameter("namespace"))

	if err := inmemoryClient.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
		_ = resp.WriteErrorString(http.StatusInternalServerError, err.Error())
		return
	}
	if err := inmemoryClient.Patch(ctx, obj, patch); err != nil {
		if status, ok := err.(apierrors.APIStatus); ok || errors.As(err, &status) {
			_ = resp.WriteHeaderAndEntity(int(status.Status().Code), status)
			return
		}
		_ = resp.WriteHeaderAndEntity(http.StatusInternalServerError, err.Error())
		return
	}
	if err := resp.WriteEntity(obj); err != nil {
		_ = resp.WriteErrorString(http.StatusInternalServerError, err.Error())
		return
	}
}

func (h *apiServerHandler) apiV1Delete(req *restful.Request, resp *restful.Response) {
	ctx := req.Request.Context()

	// Gets the resource group the request targets (the resolver is aware of the mapping host<->resourceGroup)
	resourceGroup, err := h.resourceGroupResolver(req.Request.Host)
	if err != nil {
		_ = resp.WriteErrorString(http.StatusInternalServerError, err.Error())
		return
	}

	// Gets at client to the resource group.
	inmemoryClient := h.manager.GetResourceGroup(resourceGroup).GetClient()

	// Maps the requested resource to a gvk.
	gvk, err := requestToGVK(req)
	if err != nil {
		_ = resp.WriteErrorString(http.StatusInternalServerError, err.Error())
		return
	}

	// Reads and returns the requested data.
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(gvk.GroupVersion().String())
	obj.SetKind(gvk.Kind)
	obj.SetName(req.PathParameter("name"))
	obj.SetNamespace(req.PathParameter("namespace"))

	if err := inmemoryClient.Delete(ctx, obj); err != nil {
		if status, ok := err.(apierrors.APIStatus); ok || errors.As(err, &status) {
			_ = resp.WriteHeaderAndEntity(int(status.Status().Code), status)
			return
		}
		_ = resp.WriteHeaderAndEntity(http.StatusInternalServerError, err.Error())
		return
	}
}

func (h *apiServerHandler) apiV1PortForward(req *restful.Request, resp *restful.Response) {
	// In order to handle a port forward request the current connection has to be upgraded
	// to become compliant with the SPDY protocol.
	// This implies two steps:
	// - Adding support for handling multiple http streams, used for subsequent operations over
	//   the forwarded connection.
	// - Opening a connection to the target endpoint, the endpoint to port forward to, and setting up
	//   a bidirectional copy of data because the server acts as a man in the middle.

	podName := req.PathParameter("name")
	podNamespace := req.PathParameter("namespace")

	// Perform a sub protocol negotiation, ensuring that client and server agree on how
	// to handle communications over the port forwarded connection.
	request := req.Request
	respWriter := resp.ResponseWriter
	_, err := httpstream.Handshake(request, respWriter, []string{portforward.PortForwardProtocolV1Name})
	if err != nil {
		_ = resp.WriteErrorString(http.StatusInternalServerError, err.Error())
		return
	}

	// Create a channel to handle http streams that will be generated for each subsequent
	// operations over the port forwarded connection.
	streamChan := make(chan httpstream.Stream, 1)

	// Upgrade the connection specifying what to do when a new http stream is received.
	// After being received, the new stream will be published into the stream channel for handling.
	upgrader := spdy.NewResponseUpgrader()
	conn := upgrader.UpgradeResponse(respWriter, request, inmemoryportforward.HTTPStreamReceived(streamChan))
	if conn == nil {
		_ = resp.WriteErrorString(http.StatusInternalServerError, "failed to get upgraded connection")
		return
	}
	defer func() {
		_ = conn.Close()
	}()
	conn.SetIdleTimeout(10 * time.Minute)

	// Start the process handling streams that are published in the stream channel, please note that:
	// - The connection with the target will be established only when the first operation will be executed
	// - Following operations will re-use the same connection.
	streamHandler := inmemoryportforward.NewHTTPStreamHandler(
		conn,
		streamChan,
		podName,
		podNamespace,
		func(ctx context.Context, _, _, _ string, stream io.ReadWriteCloser) error {
			// Given that in the in-memory provider there is no real infrastructure, and thus no real workload cluster,
			// we are going to forward all the connection back to the same server (the CAPIM controller pod).
			return h.doPortForward(ctx, req.Request.Host, stream)
		},
	)

	// TODO: Consider using req.Request.Context()
	streamHandler.Run(context.Background())
}

// doPortForward establish a connection to the target of the port forward operation,  and sets up
// a bidirectional copy of data.
// In the case of this provider, the target endpoint is always on the same server (the CAPIM controller pod).
func (h *apiServerHandler) doPortForward(ctx context.Context, address string, stream io.ReadWriteCloser) error {
	// Get a connection to the target of the port forward operation.
	dial, err := net.Dial("tcp", address)
	if err != nil {
		return fmt.Errorf("failed to dial %q: %w", address, err)
	}
	defer func() {
		_ = dial.Close()
	}()

	// Create a tunnel for bidirectional copy of data between the stream
	// originated from the initiator of the port forward operation and the target.
	return inmemoryportforward.HTTPStreamTunnel(ctx, stream, dial)
}

func (h *apiServerHandler) healthz(_ *restful.Request, resp *restful.Response) {
	resp.WriteHeader(http.StatusOK)
}

func requestToGVK(req *restful.Request) (*schema.GroupVersionKind, error) {
	resourceList := getAPIResourceList(req)
	if resourceList == nil {
		return nil, fmt.Errorf("no APIResourceList defined for %s", req.PathParameters())
	}
	gv, err := schema.ParseGroupVersion(resourceList.GroupVersion)
	if err != nil {
		return nil, fmt.Errorf("invalid group version in APIResourceList: %s", resourceList.GroupVersion)
	}

	resource := req.PathParameter("resource")
	for _, r := range resourceList.APIResources {
		if r.Name == resource {
			gvk := gv.WithKind(r.Kind)
			return &gvk, nil
		}
	}
	return nil, fmt.Errorf("resource %s is not defined in the APIResourceList for %s", resource, req.PathParameters())
}

func getAPIResourceList(req *restful.Request) *metav1.APIResourceList {
	if req.PathParameter("group") != "" {
		if req.PathParameter("group") == "rbac.authorization.k8s.io" && req.PathParameter("version") == "v1" {
			return rbacv1APIResourceList
		}
		if req.PathParameter("group") == "apps" && req.PathParameter("version") == "v1" {
			return appsV1ResourceList
		}
		if req.PathParameter("group") == "storage.k8s.io" && req.PathParameter("version") == "v1" {
			return storageV1ResourceList
		}
		return nil
	}
	return corev1APIResourceList
}

// isWatch is true if the request contains `watch="true"` as a query parameter.
func isWatch(req *http.Request) bool {
	return req.URL.Query().Get("watch") == "true"
}

// isList is true if the request does not have `watch="true` as a query parameter.
func isList(req *http.Request) bool {
	return !isWatch(req)
}
