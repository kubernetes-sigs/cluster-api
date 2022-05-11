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
	runtimecatalog "sigs.k8s.io/cluster-api/internal/runtime/catalog"
	runtimeregistry "sigs.k8s.io/cluster-api/internal/runtime/registry"
)

type errCallingExtensionHandler error

const defaultDiscoveryTimeout = 10 * time.Second

// Options are creation options for a Client.
type Options struct {
	Catalog  *runtimecatalog.Catalog
	Registry runtimeregistry.ExtensionRegistry
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

	// CallAllExtensions calls all the ExtensionHandler registered for the hook.
	CallAllExtensions(ctx context.Context, hook runtimecatalog.Hook, request runtime.Object, response runtimehooksv1.ResponseObject) error

	// CallExtension calls only the ExtensionHandler with the given name.
	CallExtension(ctx context.Context, hook runtimecatalog.Hook, name string, request runtime.Object, response runtimehooksv1.ResponseObject) error
}

var _ Client = &client{}

type client struct {
	catalog  *runtimecatalog.Catalog
	registry runtimeregistry.ExtensionRegistry
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
	hookGVH, err := c.catalog.GroupVersionHook(runtimehooksv1.Discovery)
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute GVH of hook")
	}

	request := &runtimehooksv1.DiscoveryRequest{}
	response := &runtimehooksv1.DiscoveryResponse{}
	opts := &httpCallOptions{
		catalog:         c.catalog,
		config:          extensionConfig.Spec.ClientConfig,
		registrationGVH: hookGVH,
		hookGVH:         hookGVH,
		timeout:         defaultDiscoveryTimeout,
	}
	if err := httpCall(ctx, request, response, opts); err != nil {
		return nil, errors.Wrap(err, "failed to call the Discovery endpoint")
	}
	// Check to see if the response is a failure and handle the failure accordingly.
	if response.GetStatus() == runtimehooksv1.ResponseStatusFailure {
		return nil, errors.Errorf("discovery failed with %v", response.GetMessage())
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

// CallAllExtensions calls all the ExtensionHandlers registered for the hook.
// The ExtensionHandler are called sequentially. The function exits immediately after any of the ExtensionHandlers return an error.
// This ensures we don't end up waiting for timeout from multiple unreachable Extensions.
// See CallExtension for more details on when an ExtensionHandler returns an error.
// The aggregate result of the ExtensionHandlers is updated into the response object passed to the function.
func (c *client) CallAllExtensions(ctx context.Context, hook runtimecatalog.Hook, request runtime.Object, response runtimehooksv1.ResponseObject) error {
	gvh, err := c.catalog.GroupVersionHook(hook)
	if err != nil {
		return errors.Wrap(err, "failed to compute GroupVersionHook")
	}
	// Make sure the request is compatible with the version of the hook expected by the ExtensionHandler.
	if err := c.catalog.ValidateRequest(gvh, request); err != nil {
		return errors.Wrapf(err, "request object is invalid for hook %s", gvh)
	}
	// Make sure the response is compatible with the version of the hook expected by the ExtensionHandler.
	if err := c.catalog.ValidateResponse(gvh, response); err != nil {
		return errors.Wrapf(err, "response object is invalid for hook %s", gvh)
	}
	registrations, err := c.registry.List(gvh.GroupHook())
	if err != nil {
		return errors.Wrapf(err, "failed to retrieve ExtensionHandlers for %s", gvh.GroupHook())
	}
	responses := []runtimehooksv1.ResponseObject{}
	for _, registration := range registrations {
		// Creates a new instance of the response parameter.
		responseObject, err := c.catalog.NewResponse(gvh)
		if err != nil {
			return errors.Wrapf(err, "ExtensionHandler %s failed", registration.Name)
		}
		tmpResponse := responseObject.(runtimehooksv1.ResponseObject)
		err = c.CallExtension(ctx, hook, registration.Name, request, tmpResponse)
		// If one of the extension handlers fails lets short-circuit here and return early.
		if err != nil {
			return errors.Wrapf(err, "ExtensionHandler %s failed", registration.Name)
		}
		responses = append(responses, tmpResponse)
	}
	if err := aggregateResponses(responses); err != nil {
		return errors.Wrap(err, "failed to aggregate responses")
	}
	return nil
}

func aggregateResponses(responses []runtimehooksv1.ResponseObject) error {
	// TODO:(killianmuldoon) implement proper aggregation logic.
	return nil
}

// CallExtension make the call to the extension with the given name.
// The response object passed will be updated with the response of the call.
// An error is returned if the extension is not compatible with the hook.
// If the ExtensionHandler returns a response with `Status` set to `Failure` the function returns an error
// and the response object is updated with the response received from the extension handler.
//
// FailurePolicy of the ExtensionHandler is used to handle errors that occur when performing the external call to the extension.
// - If FailurePolicy is set to Ignore, the error is ignored and the response object is updated to be the default success response.
// - If FailurePolicy is set to Fail, an error is returned and the response object may or may not be updated.
// Nb. FailurePolicy does not affect the following kinds of errors:
// - Internal errors. Examples: hooks is incompatible with ExtensionHandler, ExtensionHandler information is missing.
// - Error when ExtensionHandler returns a response with `Status` set to `Failure`.
func (c *client) CallExtension(ctx context.Context, hook runtimecatalog.Hook, name string, request runtime.Object, response runtimehooksv1.ResponseObject) error {
	hookGVH, err := c.catalog.GroupVersionHook(hook)
	if err != nil {
		return errors.Wrap(err, "failed to compute GroupVersionHook")
	}
	// Make sure the request is compatible with the version of the hook expected by the ExtensionHandler.
	if err := c.catalog.ValidateRequest(hookGVH, request); err != nil {
		return errors.Wrapf(err, "request object is invalid for hook %s", hookGVH)
	}
	// Make sure the response is compatible with the version of the hook expected by the ExtensionHandler.
	if err := c.catalog.ValidateResponse(hookGVH, response); err != nil {
		return errors.Wrapf(err, "response object is invalid for hook %s", hookGVH)
	}
	registration, err := c.registry.Get(name)
	if err != nil {
		return errors.Wrapf(err, "failed to retrieve ExtensionHandler with name %q", name)
	}
	if hookGVH.GroupHook() != registration.GroupVersionHook.GroupHook() {
		return errors.Errorf("ExtensionHandler %q does not match group %s, hook %s", name, hookGVH.Group, hookGVH.Hook)
	}
	var timeoutDuration time.Duration
	if registration.TimeoutSeconds != nil {
		timeoutDuration = time.Duration(*registration.TimeoutSeconds) * time.Second
	}
	opts := &httpCallOptions{
		catalog:         c.catalog,
		config:          registration.ClientConfig,
		registrationGVH: registration.GroupVersionHook,
		hookGVH:         hookGVH,
		name:            strings.TrimSuffix(registration.Name, "."+registration.ExtensionConfigName),
		timeout:         timeoutDuration,
	}
	err = httpCall(ctx, request, response, opts)
	if err != nil {
		// If the error is errCallingExtensionHandler then apply failure policy to calculate
		// the effective result of the operation.
		ignore := *registration.FailurePolicy == runtimev1.FailurePolicyIgnore
		if _, ok := err.(errCallingExtensionHandler); ok && ignore {
			// Update the response to a default success response and return.
			response.SetStatus(runtimehooksv1.ResponseStatusSuccess)
			response.SetMessage("")
			return nil
		}
		return errors.Wrap(err, "failed to call ExtensionHandler")
	}
	// If the received response is a failure then return an error.
	if response.GetStatus() == runtimehooksv1.ResponseStatusFailure {
		return errors.Errorf("ExtensionHandler %s failed with message %s", name, response.GetMessage())
	}
	// Received a successful response from the extension handler. The `response` object
	// is populated with the result. Return no error.
	return nil
}

type httpCallOptions struct {
	catalog         *runtimecatalog.Catalog
	config          runtimev1.ClientConfig
	registrationGVH runtimecatalog.GroupVersionHook
	hookGVH         runtimecatalog.GroupVersionHook
	name            string
	timeout         time.Duration
}

func httpCall(ctx context.Context, request, response runtime.Object, opts *httpCallOptions) error {
	if opts == nil || request == nil || response == nil {
		return errors.New("opts, request and response cannot be nil")
	}
	if opts.catalog == nil {
		return errors.New("opts.Catalog cannot be nil")
	}

	extensionURL, err := urlForExtension(opts.config, opts.registrationGVH, opts.name)
	if err != nil {
		return errors.Wrapf(err, "failed to compute URL of the extension handler %q", opts.name)
	}

	requireConversion := opts.registrationGVH.Version != opts.hookGVH.Version

	requestLocal := request
	responseLocal := response

	if requireConversion {
		// The request and response objects need to be converted to match the version supported by
		// the ExtensionHandler.
		var err error

		// Create a new hook request object that is compatible with the version of ExtensionHandler.
		requestLocal, err = opts.catalog.NewRequest(opts.registrationGVH)
		if err != nil {
			return errors.Wrapf(err, "failed to create new request for hook %s", opts.registrationGVH)
		}

		if err := opts.catalog.Convert(request, requestLocal, ctx); err != nil {
			return errors.Wrapf(err, "failed to convert request from %T to %T", request, requestLocal)
		}

		// Create a new hook response object that is compatible with the version of the ExtensionHandler.
		responseLocal, err = opts.catalog.NewResponse(opts.registrationGVH)
		if err != nil {
			return errors.Wrapf(err, "failed to create new response for hook %s", opts.registrationGVH)
		}
	}

	// Ensure the correct GroupVersionKind is set to the request.
	requestGVH, err := opts.catalog.Request(opts.registrationGVH)
	if err != nil {
		return errors.Wrap(err, "failed to create request object")
	}

	requestLocal.GetObjectKind().SetGroupVersionKind(requestGVH)

	postBody, err := json.Marshal(requestLocal)
	if err != nil {
		return errors.Wrap(err, "failed to marshall request object")
	}

	if opts.timeout != 0 {
		// Make the call timebound if timeout is non-zero value.
		values := extensionURL.Query()
		values.Add("timeout", opts.timeout.String())
		extensionURL.RawQuery = values.Encode()

		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.timeout)
		defer cancel()
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, extensionURL.String(), bytes.NewBuffer(postBody))
	if err != nil {
		return errors.Wrap(err, "failed to create http request")
	}

	// use client-go's transport.TLSConfigureFor to ensure good defaults for tls
	client := http.DefaultClient
	if opts.config.CABundle != nil {
		tlsConfig, err := transport.TLSConfigFor(&transport.Config{
			TLS: transport.TLSConfig{
				CAData:     opts.config.CABundle,
				ServerName: extensionURL.Hostname(),
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
		return errCallingExtensionHandler(
			errors.Wrapf(err, "failed to call ExtensionHandler: %q", opts.name),
		)
	}

	if resp.StatusCode != http.StatusOK {
		return errCallingExtensionHandler(
			errors.Errorf("non 200 response code, %q, not accepted", resp.StatusCode),
		)
	}

	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(responseLocal); err != nil {
		return errCallingExtensionHandler(
			errors.Wrap(err, "failed to decode message"),
		)
	}

	if requireConversion {
		// Convert the received response to the original version of the response object.
		if err := opts.catalog.Convert(responseLocal, response, ctx); err != nil {
			return errors.Wrapf(err, "failed to convert response from %T to %T", requestLocal, response)
		}
	}

	return nil
}

func urlForExtension(config runtimev1.ClientConfig, gvh runtimecatalog.GroupVersionHook, name string) (*url.URL, error) {
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
	u.Path = path.Join(u.Path, runtimecatalog.GVHToPath(gvh, name))
	return u, nil
}
