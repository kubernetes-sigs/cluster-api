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
	Catalog *catalog.Catalog
	// TOOD: Given that the registry is a singleton right now it doesn't make sense
	// to have an option for the registry here.
	// We can reconsider once we decided if registry should really be a singleton.
	// Registry registry.ExtensionRegistry
}

func New(options Options) Client {
	return &client{
		catalog:  options.Catalog,
		registry: registry.Extensions(),
	}
}

type Client interface {
	// IsReady returns true if the extension information is ready for usage and this happens
	// after WarmUp is called at least once.
	IsReady() bool

	// WarmUp can be used to initialize a "cold" client with all the known extensions at a given time.
	// After WarmUp completes the client is considered ready.
	WarmUp(ext *runtimev1.ExtensionConfigList) error

	Hook(hook catalog.Hook) HookClient

	Extension(ext *runtimev1.ExtensionConfig) ExtensionClient
}

var _ Client = &client{}

type client struct {
	catalog  *catalog.Catalog
	registry registry.ExtensionRegistry
}

func (c *client) IsReady() bool {
	return c.registry.IsReady()
}

func (c *client) WarmUp(ext *runtimev1.ExtensionConfigList) error {
	return c.registry.WarmUp(ext)
}

func (c *client) Hook(hook catalog.Hook) HookClient {
	return &hookClient{
		client: c,
		hook:   hook,
	}
}

func (c *client) Extension(ext *runtimev1.ExtensionConfig) ExtensionClient {
	return &extensionClient{
		client: c,
		ext:    ext,
	}
}

type HookClient interface {
	// CallAll calls all the extension registered for the hook.
	CallAll(ctx context.Context, request, response runtime.Object) error

	// Call calls only the extension with the given name.
	Call(ctx context.Context, name string, request, response runtime.Object) error
}

var _ HookClient = &hookClient{}

type hookClient struct {
	client *client
	hook   catalog.Hook
}

func (h *hookClient) CallAll(ctx context.Context, request, response runtime.Object) error {
	gvh, err := h.client.catalog.GroupVersionHook(h.hook)
	if err != nil {
		return err
	}
	registrations, err := h.client.registry.List(gvh)
	if err != nil {
		return err
	}
	responses := []runtime.Object{}
	for _, registration := range registrations {
		response, err := h.client.catalog.NewResponse(gvh)
		if err != nil {
			return err
		}
		// Follow-up: Call all extensions irrespective of error. Don't short-circuit on an error.
		if err := h.Call(ctx, registration.Name, request, response); err != nil {
			return err
		}
		responses = append(responses, response)
	}
	h.aggregateResponses(responses, response)
	return nil
}

func (h *hookClient) aggregateResponses(list []runtime.Object, into runtime.Object) {
	// TODO; panic("implement a response aggregation mechanism")
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
		catalog:       h.client.catalog,
		config:        registration.ClientConfig,
		gvh:           registration.GroupVersionHook,
		name:          strings.TrimSuffix(registration.Name, "."+registration.RegistrationName),
		timeout:       timeoutDuration,
		failurePolicy: registration.FailurePolicy,
	}
	if err := httpCall(ctx, request, response, opts); err != nil {
		return errors.Wrapf(err, "failed to call extension '%s'", name)
	}
	return nil
}

// ExtensionClient
// TODO: Discuss API here. Decide whether to use a builder pattern or to pass the extension as a function parameter.
type ExtensionClient interface {
	// Discover makes the discovery call on the extension and updates the runtime extensions
	// information in the extension status.
	// TODO: Need a final decision on if we also want to run register inside discover.
	Discover(context.Context) (*runtimev1.ExtensionConfig, error)

	// Register registers the extension with the client.
	Register() error

	//Unregister unregisters the extension with the client.
	Unregister() error
}

var _ ExtensionClient = &extensionClient{}

type extensionClient struct {
	client *client
	ext    *runtimev1.ExtensionConfig
}

func (e *extensionClient) Discover(ctx context.Context) (*runtimev1.ExtensionConfig, error) {
	gvh, err := e.client.catalog.GroupVersionHook(runtimehooksv1.Discovery)
	if err != nil {
		return nil, err
	}

	request := &runtimehooksv1.DiscoveryHookRequest{}
	response := &runtimehooksv1.DiscoveryHookResponse{}

	// Future work: The discovery runtime extension could be operating on a different hook version than
	// the latest. We will have to loop through different versions of the discover hook here to actually
	// finish discovery.
	opts := &httpCallOptions{
		catalog: e.client.catalog,
		config:  e.ext.Spec.ClientConfig,
		gvh:     gvh,
		timeout: defaultDiscoveryTimeout,
	}

	if err := httpCall(ctx, request, response, opts); err != nil {
		return nil, errors.Wrap(err, "failed to call the discovery extension")
	}

	modifiedExtension := &runtimev1.ExtensionConfig{}
	e.ext.DeepCopyInto(modifiedExtension)
	modifiedExtension.Status.Handlers = []runtimev1.ExtensionHandler{}
	for _, extension := range response.Extensions {
		modifiedExtension.Status.Handlers = append(
			modifiedExtension.Status.Handlers,
			runtimev1.ExtensionHandler{
				Name: extension.Name + "." + e.ext.Name,
				RequestHook: runtimev1.GroupVersionHook{
					APIVersion: extension.Hook.APIVersion,
					// TODO: we should probably change Name to Hook in the Discovery API as well
					Hook: extension.Hook.Name,
				},
				TimeoutSeconds: extension.TimeoutSeconds,
				FailurePolicy:  (*runtimev1.FailurePolicy)(extension.FailurePolicy),
			},
		)
	}

	// TODO: Decide if we also want to register the extension inside this function.

	return modifiedExtension, nil
}

func (e *extensionClient) Register() error {
	return e.client.registry.Add(e.ext)
}

func (e *extensionClient) Unregister() error {
	return e.client.registry.Remove(e.ext)
}

type httpCallOptions struct {
	catalog       *catalog.Catalog
	config        runtimev1.ClientConfig
	gvh           catalog.GroupVersionHook
	name          string
	timeout       time.Duration
	failurePolicy *runtimev1.FailurePolicy
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
		return err
	}

	// Follow-up: do make conversion decision per request and response. Although it might never
	// happen that the request and response are of different versions within this codebase
	// lets not make that assumption and make conversion decision per request and response object.
	requireConversion := opts.gvh.Version != request.GetObjectKind().GroupVersionKind().Version

	requestLocal := request
	responseLocal := response

	if requireConversion {
		var err error
		requestLocal, err = opts.catalog.NewRequest(opts.gvh)
		if err != nil {
			return err
		}

		if err := opts.catalog.Convert(request, requestLocal, ctx); err != nil {
			return err
		}

		responseLocal, err = opts.catalog.NewResponse(opts.gvh)
		if err != nil {
			return err
		}
	}

	if err := opts.catalog.ValidateRequest(opts.gvh, requestLocal); err != nil {
		return errors.Wrapf(err, "request object is invalid for hook %v", opts.gvh)
	}
	if err := opts.catalog.ValidateResponse(opts.gvh, responseLocal); err != nil {
		return errors.Wrapf(err, "response object is invalid for hook %v", opts.gvh)
	}

	postBody, err := json.Marshal(requestLocal)
	if err != nil {
		return errors.Wrap(err, "failed to marshall request object")
	}

	if opts.timeout != 0 {
		values := url.Query()
		values.Add("timeout", opts.timeout.String())
		url.RawQuery = values.Encode()

		ctx, _ = context.WithTimeout(ctx, opts.timeout)
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
	// TODO:  handle error in conjunction with FailurePolicy
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(responseLocal); err != nil {
		return errors.Wrap(err, "failed to decode response")
	}

	if requireConversion {
		if err := opts.catalog.Convert(responseLocal, response, ctx); err != nil {
			return err
		}
	}

	return nil
}

func urlForExtension(config runtimev1.ClientConfig, gvh catalog.GroupVersionHook, name string) (*url.URL, error) {
	var u *url.URL
	// TODO: Add additional validation here - webhook should make this safe, but for now it might be good to do url validation in here.
	if config.Service != nil {
		svc := config.Service
		host := svc.Name + "." + svc.Namespace + ".svc"
		if svc.Port != nil {
			host = net.JoinHostPort(host, strconv.Itoa(int(*svc.Port)))
		}
		// TODO: decide if we want to enforce https
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
			return nil, errors.Wrap(err, "URL in config is invalid")
		}
	}
	u.Path = path.Join(u.Path, catalog.GVHToPath(gvh, name))
	return u, nil
}
