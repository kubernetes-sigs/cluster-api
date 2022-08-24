/*
Copyright 2021 The Kubernetes Authors.

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

// Package server contains the implementation of a RuntimeSDK webhook server.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
)

const tlsVersion13 = "1.3"

// DefaultPort is the default port that the webhook server serves.
var DefaultPort = 9443

// Server is a runtime webhook server.
type Server struct {
	catalog  *runtimecatalog.Catalog
	server   *webhook.Server
	handlers map[string]ExtensionHandler
}

// Options are the options for the Server.
type Options struct {
	// Catalog is the catalog used to handle requests.
	Catalog *runtimecatalog.Catalog

	// Port is the port that the webhook server serves at.
	// It is used to set webhook.Server.Port.
	Port int

	// Host is the hostname that the webhook server binds to.
	// It is used to set webhook.Server.Host.
	Host string

	// CertDir is the directory that contains the server key and certificate.
	// If not set, webhook server would look up the server key and certificate in
	// {TempDir}/k8s-webhook-server/serving-certs. The server key and certificate
	// must be named tls.key and tls.crt, respectively.
	// It is used to set webhook.Server.CertDir.
	CertDir string
}

// NewServer creates a new runtime webhook server based on the given Options.
func NewServer(options Options) (*Server, error) {
	if options.Catalog == nil {
		return nil, errors.Errorf("catalog is required")
	}
	if options.Port <= 0 {
		options.Port = DefaultPort
	}
	if options.CertDir == "" {
		options.CertDir = filepath.Join(os.TempDir(), "k8s-webhook-server", "serving-certs")
	}

	webhookServer := &webhook.Server{
		Port:          options.Port,
		Host:          options.Host,
		CertDir:       options.CertDir,
		CertName:      "tls.crt",
		KeyName:       "tls.key",
		WebhookMux:    http.NewServeMux(),
		TLSMinVersion: tlsVersion13,
	}

	return &Server{
		catalog:  options.Catalog,
		server:   webhookServer,
		handlers: map[string]ExtensionHandler{},
	}, nil
}

// ExtensionHandler represents an extension handler.
type ExtensionHandler struct {
	// gvh is the gvh of the hook corresponding to the extension handler.
	gvh runtimecatalog.GroupVersionHook
	// requestObject is a runtime object that the handler expects to receive.
	requestObject runtime.Object
	// responseObject is a runtime object that the handler expects to return.
	responseObject runtime.Object

	// Hook is the corresponding hook of the handler.
	Hook runtimecatalog.Hook

	// Name is the name of the extension handler.
	Name string

	// HandlerFunc is the handler function.
	HandlerFunc runtimecatalog.Hook

	// TimeoutSeconds is the timeout of the extension handler.
	TimeoutSeconds *int32

	// FailurePolicy is the failure policy of the extension handler
	FailurePolicy *runtimehooksv1.FailurePolicy
}

// AddExtensionHandler adds an extension handler to the server.
func (s *Server) AddExtensionHandler(handler ExtensionHandler) error {
	gvh, err := s.catalog.GroupVersionHook(handler.Hook)
	if err != nil {
		return errors.Wrapf(err, "hook %q does not exist in catalog", runtimecatalog.HookName(handler.Hook))
	}
	handler.gvh = gvh

	requestObject, err := s.catalog.NewRequest(handler.gvh)
	if err != nil {
		return err
	}
	handler.requestObject = requestObject

	responseObject, err := s.catalog.NewResponse(handler.gvh)
	if err != nil {
		return err
	}
	handler.responseObject = responseObject

	if err := s.validateHandler(handler); err != nil {
		return err
	}

	handlerPath := runtimecatalog.GVHToPath(handler.gvh, handler.Name)
	if _, ok := s.handlers[handlerPath]; ok {
		return errors.Errorf("there is already a handler registered for path %q", handlerPath)
	}

	s.handlers[handlerPath] = handler
	return nil
}

// validateHandler validates a handler.
func (s *Server) validateHandler(handler ExtensionHandler) error {
	// Get hook and handler type.
	hookFuncType := reflect.TypeOf(handler.Hook)
	handlerFuncType := reflect.TypeOf(handler.HandlerFunc)

	// Validate handler function signature.
	if handlerFuncType.Kind() != reflect.Func {
		return errors.Errorf("HandlerFunc must be a func")
	}
	if handlerFuncType.NumIn() != 3 {
		return errors.Errorf("HandlerFunc must have three input parameter")
	}
	if handlerFuncType.NumOut() != 0 {
		return errors.Errorf("HandlerFunc must have no output parameter")
	}

	// Get hook and handler request and response types.
	hookRequestType := hookFuncType.In(0)
	hookResponseType := hookFuncType.In(1)
	handlerContextType := handlerFuncType.In(0)
	handlerRequestType := handlerFuncType.In(1)
	handlerResponseType := handlerFuncType.In(2)

	// Validate handler request and response are pointers.
	if handlerRequestType.Kind() != reflect.Ptr {
		return errors.Errorf("HandlerFunc request type must be a pointer")
	}
	if handlerResponseType.Kind() != reflect.Ptr {
		return errors.Errorf("HandlerFunc response type must be a pointer")
	}

	// Validate first handler parameter is a context
	// TODO: improve check, how to check if param is a specific interface?
	if handlerContextType.Name() != "Context" {
		return errors.Errorf("HandlerFunc first parameter must be Context but is %s", handlerContextType.Name())
	}

	// Validate hook and handler request and response types are equal.
	if hookRequestType != handlerRequestType {
		return errors.Errorf("HandlerFunc request type must be *%s but is *%s", hookRequestType.Elem().Name(), handlerRequestType.Elem().Name())
	}
	if hookResponseType != handlerResponseType {
		return errors.Errorf("HandlerFunc response type must be *%s but is *%s", hookResponseType.Elem().Name(), handlerResponseType.Elem().Name())
	}

	return nil
}

// Start starts the server.
func (s *Server) Start(ctx context.Context) error {
	// Add discovery handler.
	err := s.AddExtensionHandler(ExtensionHandler{
		Hook:        runtimehooksv1.Discovery,
		HandlerFunc: discoveryHandler(s.handlers),
	})
	if err != nil {
		return err
	}

	// Add handlers to router.
	for handlerPath, h := range s.handlers {
		handler := h

		wrappedHandler := s.wrapHandler(handler)
		s.server.Register(handlerPath, http.HandlerFunc(wrappedHandler))
	}

	return s.server.StartStandalone(ctx, nil)
}

// discoveryHandler generates a discovery handler based on a list of handlers.
func discoveryHandler(handlers map[string]ExtensionHandler) func(context.Context, *runtimehooksv1.DiscoveryRequest, *runtimehooksv1.DiscoveryResponse) {
	cachedHandlers := []runtimehooksv1.ExtensionHandler{}
	for _, handler := range handlers {
		cachedHandlers = append(cachedHandlers, runtimehooksv1.ExtensionHandler{
			Name: handler.Name,
			RequestHook: runtimehooksv1.GroupVersionHook{
				APIVersion: handler.gvh.GroupVersion().String(),
				Hook:       handler.gvh.Hook,
			},
			TimeoutSeconds: handler.TimeoutSeconds,
			FailurePolicy:  handler.FailurePolicy,
		})
	}

	return func(_ context.Context, _ *runtimehooksv1.DiscoveryRequest, response *runtimehooksv1.DiscoveryResponse) {
		response.SetStatus(runtimehooksv1.ResponseStatusSuccess)
		response.Handlers = cachedHandlers
	}
}

func (s *Server) wrapHandler(handler ExtensionHandler) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		response := s.callHandler(handler, r)

		responseBody, err := json.Marshal(response)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprintf(w, "unable to marshal response: %v", err)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(responseBody)
	}
}

func (s *Server) callHandler(handler ExtensionHandler, r *http.Request) runtimehooksv1.ResponseObject {
	request := handler.requestObject.DeepCopyObject()
	response := handler.responseObject.DeepCopyObject().(runtimehooksv1.ResponseObject)

	requestBody, err := io.ReadAll(r.Body)
	if err != nil {
		response.SetStatus(runtimehooksv1.ResponseStatusFailure)
		response.SetMessage(fmt.Sprintf("error reading request: %v", err))
		return response
	}

	if err := json.Unmarshal(requestBody, request); err != nil {
		response.SetStatus(runtimehooksv1.ResponseStatusFailure)
		response.SetMessage(fmt.Sprintf("error unmarshalling request: %v", err))
		return response
	}

	// log.Log is the logger previously set via ctrl.SetLogger.
	// This implemented analog to the logger in the controller-runtime manager.
	ctx := ctrl.LoggerInto(r.Context(), log.Log)

	reflect.ValueOf(handler.HandlerFunc).Call([]reflect.Value{
		reflect.ValueOf(ctx),
		reflect.ValueOf(request),
		reflect.ValueOf(response),
	})

	return response
}
