/*
Copyright 2022 The Kubernetes Authors.

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

// Package client provides the Runtime SDK client.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	utilnet "k8s.io/apimachinery/pkg/util/net"
	"k8s.io/client-go/transport"

	runtimev1 "sigs.k8s.io/cluster-api/exp/runtime/api/v1alpha1"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	"sigs.k8s.io/cluster-api/internal/runtime/catalog"
	"sigs.k8s.io/cluster-api/internal/runtime/registry"
)

const defaultDiscoveryTimeout = 10 * time.Second

// Options are creation options for a Client.
type Options struct {
	Catalog  *catalog.Catalog
	Registry registry.ExtensionRegistry
}

// New returns a new Client.
func New(options Options) Client {
	return &client{
		catalog:  options.Catalog,
		registry: options.Registry,
	}
}

// Client is the runtime client to interact with hooks and extensions.
type Client interface {
	// WarmUp can be used to initialize a "cold" RuntimeClient with all
	// known runtimev1.ExtensionConfigs at a given time.
	// After WarmUp completes the RuntimeClient is considered ready.
	WarmUp(extensionConfigList *runtimev1.ExtensionConfigList) error

	// IsReady return true after the RuntimeClient finishes warmup.
	IsReady() bool

	// Discover makes the discovery call on the extension and returns an updated ExtensionConfig
	// with extension handlers information in the extension status.
	Discover(context.Context, *runtimev1.ExtensionConfig) (*runtimev1.ExtensionConfig, error)

	// Register registers the ExtensionConfig.
	Register(extensionConfig *runtimev1.ExtensionConfig) error

	// Unregister unregisters the ExtensionConfig.
	Unregister(extensionConfig *runtimev1.ExtensionConfig) error

	// Hook returns a HookClient that is used to interact with the extension handlers for a specific hook.
	Hook(hook catalog.Hook) HookClient
}

var _ Client = &client{}

type client struct {
	catalog  *catalog.Catalog
	registry registry.ExtensionRegistry
}

func (c *client) WarmUp(extensionConfigList *runtimev1.ExtensionConfigList) error {
	if err := c.registry.WarmUp(extensionConfigList); err != nil {
		return errors.Wrap(err, "failed to warm up")
	}
	return nil
}

func (c *client) IsReady() bool {
	return c.registry.IsReady()
}

func (c *client) Discover(ctx context.Context, extensionConfig *runtimev1.ExtensionConfig) (*runtimev1.ExtensionConfig, error) {
	gvh, err := c.catalog.GroupVersionHook(runtimehooksv1.Discovery)
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute GVH of hook")
	}

	request := &runtimehooksv1.DiscoveryRequest{}
	response := &runtimehooksv1.DiscoveryResponse{}
	opts := &httpCallOptions{
		catalog: c.catalog,
		config:  extensionConfig.Spec.ClientConfig,
		gvh:     gvh,
		timeout: defaultDiscoveryTimeout,
	}
	if err := httpCall(ctx, request, response, opts); err != nil {
		return nil, errors.Wrap(err, "failed to call the Discovery endpoint")
	}
	// Check to see if the response is a failure and handle the failure accordingly.
	if response.Status == runtimehooksv1.ResponseStatusFailure {
		return nil, fmt.Errorf("discovery failed with %v", response.Message)
	}

	modifiedExtensionConfig := extensionConfig.DeepCopy()
	// Reset the handlers that were previously registered with the ExtensionConfig.
	modifiedExtensionConfig.Status.Handlers = []runtimev1.ExtensionHandler{}

	for _, handler := range response.Handlers {
		modifiedExtensionConfig.Status.Handlers = append(
			modifiedExtensionConfig.Status.Handlers,
			runtimev1.ExtensionHandler{
				Name: handler.Name + "." + extensionConfig.Name, // Uniquely identifies a handler of an Extension.
				RequestHook: runtimev1.GroupVersionHook{
					APIVersion: handler.RequestHook.APIVersion,
					Hook:       handler.RequestHook.Hook,
				},
				TimeoutSeconds: handler.TimeoutSeconds,
				FailurePolicy:  (*runtimev1.FailurePolicy)(handler.FailurePolicy),
			},
		)
	}

	return modifiedExtensionConfig, nil
}

func (c *client) Register(extensionConfig *runtimev1.ExtensionConfig) error {
	if err := c.registry.Add(extensionConfig); err != nil {
		return errors.Wrap(err, "failed to register ExtensionConfig")
	}
	return nil
}

func (c *client) Unregister(extensionConfig *runtimev1.ExtensionConfig) error {
	if err := c.registry.Remove(extensionConfig); err != nil {
		return errors.Wrap(err, "failed to unregister ExtensionConfig")
	}
	return nil
}

func (c *client) Hook(hook catalog.Hook) HookClient {
	return &hookClient{
		client: c,
		hook:   hook,
	}
}

type HookClient interface {
	// CallAll calls all the extension registered for the hook.
	CallAll(ctx context.Context, request runtime.Object, response runtimehooksv1.AggregatableResponse) error

	// Call calls only the extension with the given name.
	Call(ctx context.Context, name string, request, response runtime.Object) error
}

type hookClient struct {
	client *client
	hook   catalog.Hook
}

func (h *hookClient) CallAll(ctx context.Context, request runtime.Object, response runtimehooksv1.AggregatableResponse) error {
	gvh, err := h.client.catalog.GroupVersionHook(h.hook)
	if err != nil {
		return errors.Wrap(err, "failed to compute GroupVersionHook")
	}
	registrations, err := h.client.registry.List(catalog.GroupHook{Group: gvh.Group, Hook: gvh.Hook})
	if err != nil {
		return errors.Wrap(err, "failed to retrieve ExtensionHandlers information")
	}
	responses := []*runtimehooksv1.ExtensionHandlerResponse{}
	// Future-work: Call the extension handlers concurrently.
	for _, registration := range registrations {
		tmpResponse, err := h.client.catalog.NewResponse(gvh)
		if err != nil {
			return errors.Wrap(err, "failed to create respons object")
		}
		err = h.Call(ctx, registration.Name, request, tmpResponse)
		// If at least once of the extension handlers failed lets short-circuit here and return early.
		if err != nil {
			return errors.Wrapf(err, "ExtensionHandler %s failed", registration.Name)
		}
		responses = append(responses, &runtimehooksv1.ExtensionHandlerResponse{
			Name:     registration.Name,
			Response: tmpResponse,
		})
	}
	if err := response.Aggregate(responses); err != nil {
		return errors.Wrap(err, "failed to aggregate responses")
	}
	return nil
}

func (h *hookClient) Call(ctx context.Context, name string, request, response runtime.Object) error {
	registration, err := h.client.registry.Get(name)
	if err != nil {
		return err
	}
	var timeoutDuration time.Duration
	if registration.TimeoutSeconds != nil {
		timeoutDuration = time.Duration(*registration.TimeoutSeconds) * time.Second
	}
	opts := &httpCallOptions{
		catalog: h.client.catalog,
		config:  registration.ClientConfig,
		gvh:     registration.GroupVersionHook,
		name:    strings.TrimSuffix(registration.Name, "."+registration.ExtensionConfigName),
		timeout: timeoutDuration,
	}
	err = httpCall(ctx, request, response, opts)
	if err != nil {
		// If the error is errCallingExtensionHandler then apply failure policy to calculate
		// the effective result of the operation.
		ignore := *registration.FailurePolicy == runtimev1.FailurePolicyIgnore
		if _, ok := err.(*errCallingExtensionHandler); ok && ignore {
			// Update the response to a default success response and return.
			runtimehooksv1.SetStatus(response, runtimehooksv1.ResponseStatusSuccess)
			return nil
		}
		return errors.Wrap(err, "failed to call extension")
	}
	// If the received response is a failure then return an error.
	failure, err := runtimehooksv1.IsFailure(response)
	if err != nil {
		return errors.Wrapf(err, "failed to process response")
	}
	if failure {
		msg, err := runtimehooksv1.GetMessage(response)
		if err != nil {
			return errors.Wrap(err, "failed to process response")
		}
		return fmt.Errorf("extensionHandler %s failed with message %s", name, msg)
	}
	// Received a successfull response from the extension handler. The `response` object
	// is populated with the result. Return no error.
	return nil
}

type httpCallOptions struct {
	catalog *catalog.Catalog
	config  runtimev1.ClientConfig
	gvh     catalog.GroupVersionHook
	name    string
	timeout time.Duration
}

func httpCall(ctx context.Context, request, response runtime.Object, opts *httpCallOptions) error {
	if opts == nil || request == nil || response == nil {
		return fmt.Errorf("opts, request and response cannot be nil")
	}
	if opts.catalog == nil {
		return fmt.Errorf("opts.Catalog cannot be nil")
	}

	url, err := urlForExtension(opts.config, opts.gvh, opts.name)
	if err != nil {
		return errors.Wrapf(err, "failed to compute URL of the extension handler %q", opts.name)
	}

	requireConversion := opts.gvh.Version != request.GetObjectKind().GroupVersionKind().Version

	requestLocal := request
	responseLocal := response

	if requireConversion {
		// The request and response objects need to be converted to match the version supported by
		// the ExtensionHandler.
		var err error

		// Create a new hook request object that is compatible with the version of ExtensionHandler.
		requestLocal, err = opts.catalog.NewRequest(opts.gvh)
		if err != nil {
			return errors.Wrapf(err, "failed to create new request for hook %s", opts.gvh)
		}

		if err := opts.catalog.Convert(request, requestLocal, ctx); err != nil {
			return errors.Wrapf(err, "failed to convert request from %T to %T", request, requestLocal)
		}

		// Create a new hook response object that is compatible with the version of the ExtensionHandler.
		responseLocal, err = opts.catalog.NewResponse(opts.gvh)
		if err != nil {
			return errors.Wrapf(err, "failed to create new response for hook %s", opts.gvh)
		}
	}

	// Make sure the request is compatible with the version of the hook  expected by the ExtensionHandler.
	if err := opts.catalog.ValidateRequest(opts.gvh, requestLocal); err != nil {
		return errors.Wrapf(err, "request object is invalid for hook %s", opts.gvh)
	}
	// Make sure the response is compatible with the version of the hook  expected by the ExtensionHandler.
	if err := opts.catalog.ValidateResponse(opts.gvh, responseLocal); err != nil {
		return errors.Wrapf(err, "response object is invalid for hook %s", opts.gvh)
	}

	postBody, err := json.Marshal(requestLocal)
	if err != nil {
		return errors.Wrap(err, "failed to marshall request object")
	}

	if opts.timeout != 0 {
		// Make the call timebound if timeout is non-zero value.
		values := url.Query()
		values.Add("timeout", opts.timeout.String())
		url.RawQuery = values.Encode()

		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.timeout)
		defer cancel()
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, url.String(), bytes.NewBuffer(postBody))
	if err != nil {
		return errors.Wrap(err, "failed to create http request")
	}

	// use client-go's transport.TLSConfigureFor to ensure good defaults for tls
	client := http.DefaultClient
	if opts.config.CABundle != nil {
		tlsConfig, err := transport.TLSConfigFor(&transport.Config{
			TLS: transport.TLSConfig{
				CAData:     opts.config.CABundle,
				ServerName: url.Hostname(),
			},
		})
		if err != nil {
			return errors.Wrap(err, "failed to create tls config")
		}
		// this also adds http2
		client.Transport = utilnet.SetTransportDefaults(&http.Transport{
			TLSClientConfig: tlsConfig,
		})
	}
	resp, err := client.Do(httpRequest)
	if err != nil {
		return &errCallingExtensionHandler{
			extensionHandlerName: opts.name,
			err:                  errors.Wrap(err, "failed to make the http call"),
		}
	}

	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(responseLocal); err != nil {
		return &errCallingExtensionHandler{
			extensionHandlerName: opts.name,
			err:                  errors.Wrap(err, "failed to decode response"),
		}
	}

	if requireConversion {
		// Convert the received response to the original version of the response object.
		if err := opts.catalog.Convert(responseLocal, response, ctx); err != nil {
			return errors.Wrapf(err, "failed to convert response from %T to %T", requestLocal, response)
		}
	}

	return nil
}

func urlForExtension(config runtimev1.ClientConfig, gvh catalog.GroupVersionHook, name string) (*url.URL, error) {
	var u *url.URL
	if config.Service != nil {
		// The Extension's ClientConfig points ot a service. Construct the URL to the service.
		svc := config.Service
		host := svc.Name + "." + svc.Namespace + ".svc"
		if svc.Port != nil {
			host = net.JoinHostPort(host, strconv.Itoa(int(*svc.Port)))
		}
		scheme := "http"
		if len(config.CABundle) > 0 {
			scheme = "https"
		}
		u = &url.URL{
			Scheme: scheme,
			Host:   host,
		}
		if svc.Path != nil {
			u.Path = *svc.Path
		}
	} else {
		if config.URL == nil {
			return nil, errors.New("at least one of Service and URL should be defined in config")
		}
		var err error
		u, err = url.Parse(*config.URL)
		if err != nil {
			return nil, errors.Wrap(err, "URL in ClientConfig is invalid")
		}
	}
	// Add the subpatch to the ExtensionHandler for the given hook.
	u.Path = path.Join(u.Path, catalog.GVHToPath(gvh, name))
	return u, nil
}

type errCallingExtensionHandler struct {
	extensionHandlerName string
	err                  error
}

func (e *errCallingExtensionHandler) Error() string {
	// FIXME: find a better error message.
	return fmt.Sprintf("failed processing handler %s of extension %s with error: %s", e.extensionHandlerName, e.err)
}
