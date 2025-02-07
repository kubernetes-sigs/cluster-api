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
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	utilnet "k8s.io/apimachinery/pkg/util/net"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/transport"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	runtimev1 "sigs.k8s.io/cluster-api/exp/runtime/api/v1alpha1"
	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
	runtimeclient "sigs.k8s.io/cluster-api/exp/runtime/client"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	runtimemetrics "sigs.k8s.io/cluster-api/internal/runtime/metrics"
	runtimeregistry "sigs.k8s.io/cluster-api/internal/runtime/registry"
	"sigs.k8s.io/cluster-api/util"
)

type errCallingExtensionHandler error

const defaultDiscoveryTimeout = 10 * time.Second

// Options are creation options for a Client.
type Options struct {
	Catalog  *runtimecatalog.Catalog
	Registry runtimeregistry.ExtensionRegistry
	Client   ctrlclient.Client
}

// New returns a new Client.
func New(options Options) runtimeclient.Client {
	return &client{
		catalog:  options.Catalog,
		registry: options.Registry,
		client:   options.Client,
	}
}

var _ runtimeclient.Client = &client{}

type client struct {
	catalog  *runtimecatalog.Catalog
	registry runtimeregistry.ExtensionRegistry
	client   ctrlclient.Client
}

func (c *client) WarmUp(extensionConfigList *runtimev1.ExtensionConfigList) error {
	return c.registry.WarmUp(extensionConfigList)
}

func (c *client) IsReady() bool {
	return c.registry.IsReady()
}

func (c *client) Discover(ctx context.Context, extensionConfig *runtimev1.ExtensionConfig) (*runtimev1.ExtensionConfig, error) {
	log := ctrl.LoggerFrom(ctx)
	log.V(4).Info("Performing discovery for ExtensionConfig")

	hookGVH, err := c.catalog.GroupVersionHook(runtimehooksv1.Discovery)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to discover extension %q: failed to compute GVH of hook", extensionConfig.Name)
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
		return nil, errors.Wrapf(err, "failed to discover extension %q", extensionConfig.Name)
	}

	// Check to see if the response is a failure and handle the failure accordingly.
	if response.GetStatus() == runtimehooksv1.ResponseStatusFailure {
		log.Info(fmt.Sprintf("Failed to discover extension %q: got failure response with message %v", extensionConfig.Name, response.GetMessage()))
		// Don't add the message to the error as it is may be unique causing too many reconciliations. Ref: https://github.com/kubernetes-sigs/cluster-api/issues/6921
		return nil, errors.Errorf("failed to discover extension %q: got failure response", extensionConfig.Name)
	}

	// Check to see if the response is valid.
	if err = defaultAndValidateDiscoveryResponse(c.catalog, response); err != nil {
		return nil, errors.Wrapf(err, "failed to discover extension %q", extensionConfig.Name)
	}

	modifiedExtensionConfig := extensionConfig.DeepCopy()
	// Reset the handlers that were previously registered with the ExtensionConfig.
	modifiedExtensionConfig.Status.Handlers = []runtimev1.ExtensionHandler{}

	for _, handler := range response.Handlers {
		handlerName, err := NameForHandler(handler, extensionConfig)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to discover extension %q", extensionConfig.Name)
		}
		modifiedExtensionConfig.Status.Handlers = append(
			modifiedExtensionConfig.Status.Handlers,
			runtimev1.ExtensionHandler{
				Name: handlerName, // Uniquely identifies a handler of an Extension.
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
		return errors.Wrapf(err, "failed to register ExtensionConfig %q", extensionConfig.Name)
	}
	return nil
}

func (c *client) Unregister(extensionConfig *runtimev1.ExtensionConfig) error {
	if err := c.registry.Remove(extensionConfig); err != nil {
		return errors.Wrapf(err, "failed to unregister ExtensionConfig %q", extensionConfig.Name)
	}
	return nil
}

// CallAllExtensions calls all the ExtensionHandlers registered for the hook.
// The ExtensionHandlers are called sequentially. The function exits immediately after any of the ExtensionHandlers return an error.
// This ensures we don't end up waiting for timeout from multiple unreachable Extensions.
// See CallExtension for more details on when an ExtensionHandler returns an error.
// The aggregated result of the ExtensionHandlers is updated into the response object passed to the function.
func (c *client) CallAllExtensions(ctx context.Context, hook runtimecatalog.Hook, forObject metav1.Object, request runtimehooksv1.RequestObject, response runtimehooksv1.ResponseObject) error {
	hookName := runtimecatalog.HookName(hook)
	log := ctrl.LoggerFrom(ctx).WithValues("hook", hookName)
	ctx = ctrl.LoggerInto(ctx, log)
	gvh, err := c.catalog.GroupVersionHook(hook)
	if err != nil {
		return errors.Wrapf(err, "failed to call extension handlers for hook %q: failed to compute GroupVersionHook", hookName)
	}
	// Make sure the request is compatible with the hook.
	if err := c.catalog.ValidateRequest(gvh, request); err != nil {
		return errors.Wrapf(err, "failed to call extension handlers for hook %q: request object is invalid for hook", gvh.GroupHook())
	}
	// Make sure the response is compatible with the hook.
	if err := c.catalog.ValidateResponse(gvh, response); err != nil {
		return errors.Wrapf(err, "failed to call extension handlers for hook %q: response object is invalid for hook", gvh.GroupHook())
	}

	registrations, err := c.registry.List(gvh.GroupHook())
	if err != nil {
		return errors.Wrapf(err, "failed to call extension handlers for hook %q", gvh.GroupHook())
	}

	log.V(4).Info(fmt.Sprintf("Calling all extensions of hook %q", hookName))
	responses := []runtimehooksv1.ResponseObject{}
	for _, registration := range registrations {
		// Creates a new instance of the response parameter.
		responseObject, err := c.catalog.NewResponse(gvh)
		if err != nil {
			return errors.Wrapf(err, "failed to call extension handlers for hook %q: failed to call extension handler %q", gvh.GroupHook(), registration.Name)
		}
		tmpResponse := responseObject.(runtimehooksv1.ResponseObject)

		// Compute whether the object the call is being made for matches the namespaceSelector
		namespaceMatches, err := c.matchNamespace(ctx, registration.NamespaceSelector, forObject.GetNamespace())
		if err != nil {
			return errors.Wrapf(err, "failed to call extension handlers for hook %q: failed to call extension handler %q", gvh.GroupHook(), registration.Name)
		}
		// If the object namespace isn't matched by the registration NamespaceSelector skip the call.
		if !namespaceMatches {
			log.V(5).Info(fmt.Sprintf("skipping extension handler %q as object '%s/%s' does not match selector %q of ExtensionConfig", registration.Name, forObject.GetNamespace(), forObject.GetName(), registration.NamespaceSelector))
			continue
		}

		err = c.CallExtension(ctx, hook, forObject, registration.Name, request, tmpResponse)
		// If one of the extension handlers fails lets short-circuit here and return early.
		if err != nil {
			log.Error(err, "failed to call extension handlers")
			return errors.Wrapf(err, "failed to call extension handlers for hook %q", gvh.GroupHook())
		}
		responses = append(responses, tmpResponse)
	}

	// Aggregate all responses into a single response.
	// Note: we only get here if all the extension handlers succeeded.
	aggregateSuccessfulResponses(response, responses)

	return nil
}

// aggregateSuccessfulResponses aggregates all successful responses into a single response.
func aggregateSuccessfulResponses(aggregatedResponse runtimehooksv1.ResponseObject, responses []runtimehooksv1.ResponseObject) {
	// At this point the Status should always be ResponseStatusSuccess.
	aggregatedResponse.SetStatus(runtimehooksv1.ResponseStatusSuccess)

	// Note: As all responses have the same type we can assume now that
	// they all implement the RetryResponseObject interface.
	messages := []string{}
	for _, resp := range responses {
		aggregatedRetryResponse, ok := aggregatedResponse.(runtimehooksv1.RetryResponseObject)
		if ok {
			aggregatedRetryResponse.SetRetryAfterSeconds(util.LowestNonZeroInt32(
				aggregatedRetryResponse.GetRetryAfterSeconds(),
				resp.(runtimehooksv1.RetryResponseObject).GetRetryAfterSeconds(),
			))
		}
		if resp.GetMessage() != "" {
			messages = append(messages, resp.GetMessage())
		}
	}
	aggregatedResponse.SetMessage(strings.Join(messages, ", "))
}

// CallExtension makes the call to the extension with the given name.
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
func (c *client) CallExtension(ctx context.Context, hook runtimecatalog.Hook, forObject metav1.Object, name string, request runtimehooksv1.RequestObject, response runtimehooksv1.ResponseObject, opts ...runtimeclient.CallExtensionOption) error {
	// Calculate the options.
	options := &runtimeclient.CallExtensionOptions{}
	for _, opt := range opts {
		opt.ApplyToOptions(options)
	}

	log := ctrl.LoggerFrom(ctx).WithValues("extensionHandler", name, "hook", runtimecatalog.HookName(hook))
	ctx = ctrl.LoggerInto(ctx, log)
	hookGVH, err := c.catalog.GroupVersionHook(hook)
	if err != nil {
		return errors.Wrapf(err, "failed to call extension handler %q: failed to compute GroupVersionHook", name)
	}
	// Make sure the request is compatible with the hook.
	if err := c.catalog.ValidateRequest(hookGVH, request); err != nil {
		return errors.Wrapf(err, "failed to call extension handler %q: request object is invalid for hook %q", name, hookGVH)
	}
	// Make sure the response is compatible with the hook.
	if err := c.catalog.ValidateResponse(hookGVH, response); err != nil {
		return errors.Wrapf(err, "failed to call extension handler %q: response object is invalid for hook %q", name, hookGVH)
	}

	registration, err := c.registry.Get(name)
	if err != nil {
		return errors.Wrapf(err, "failed to call extension handler %q", name)
	}
	if hookGVH.GroupHook() != registration.GroupVersionHook.GroupHook() {
		return errors.Errorf("failed to call extension handler %q: handler does not match GroupHook %q", name, hookGVH.GroupHook())
	}

	// Compute whether the object the call is being made for matches the namespaceSelector
	namespaceMatches, err := c.matchNamespace(ctx, registration.NamespaceSelector, forObject.GetNamespace())
	if err != nil {
		return errors.Errorf("failed to call extension handler %q", name)
	}
	// If the object namespace isn't matched by the registration NamespaceSelector return an error.
	if !namespaceMatches {
		return errors.Errorf("failed to call extension handler %q: namespaceSelector did not match object %s", name, util.ObjectKey(forObject))
	}

	log.V(4).Info(fmt.Sprintf("Calling extension handler %q", name))
	timeoutDuration := runtimehooksv1.DefaultHandlersTimeoutSeconds * time.Second
	if registration.TimeoutSeconds != nil {
		timeoutDuration = time.Duration(*registration.TimeoutSeconds) * time.Second
	}

	// Prepare the request by merging the settings in the registration with the settings in the request.
	request = cloneAndAddSettings(request, registration.Settings)

	var cacheKey string
	if options.WithCaching {
		// Return a cached response if response is cached.
		cacheKey = options.CacheKeyFunc(registration.Name, registration.ExtensionConfigResourceVersion, request)
		if cacheEntry, ok := options.Cache.Has(cacheKey); ok {
			// Set response to cacheEntry.Response.
			outVal := reflect.ValueOf(response)
			cacheVal := reflect.ValueOf(cacheEntry.Response)
			if !cacheVal.Type().AssignableTo(outVal.Type()) {
				return fmt.Errorf("failed to call extension handler %q: cached response of type %s instead of type %s", name, cacheVal.Type(), outVal.Type())
			}
			reflect.Indirect(outVal).Set(reflect.Indirect(cacheVal))
			return nil
		}
	}

	httpOpts := &httpCallOptions{
		catalog:         c.catalog,
		config:          registration.ClientConfig,
		registrationGVH: registration.GroupVersionHook,
		hookGVH:         hookGVH,
		name:            strings.TrimSuffix(registration.Name, "."+registration.ExtensionConfigName),
		timeout:         timeoutDuration,
	}
	err = httpCall(ctx, request, response, httpOpts)
	if err != nil {
		// If the error is errCallingExtensionHandler then apply failure policy to calculate
		// the effective result of the operation.
		ignore := *registration.FailurePolicy == runtimev1.FailurePolicyIgnore
		if _, ok := err.(errCallingExtensionHandler); ok && ignore {
			// Update the response to a default success response and return.
			log.Error(err, fmt.Sprintf("Ignoring error calling extension handler because of FailurePolicy %q", *registration.FailurePolicy))
			response.SetStatus(runtimehooksv1.ResponseStatusSuccess)
			response.SetMessage("")
			return nil
		}
		log.Error(err, "Failed to call extension handler")
		return errors.Wrapf(err, "failed to call extension handler %q", name)
	}

	// If the received response is a failure then return an error.
	if response.GetStatus() == runtimehooksv1.ResponseStatusFailure {
		log.Info(fmt.Sprintf("Failed to call extension handler %q: got failure response with message %v", name, response.GetMessage()))
		// Don't add the message to the error as it is may be unique causing too many reconciliations. Ref: https://github.com/kubernetes-sigs/cluster-api/issues/6921
		return errors.Errorf("failed to call extension handler %q: got failure response", name)
	}

	if retryResponse, ok := response.(runtimehooksv1.RetryResponseObject); ok && retryResponse.GetRetryAfterSeconds() != 0 {
		log.Info(fmt.Sprintf("Extension handler returned blocking response with retryAfterSeconds of %d", retryResponse.GetRetryAfterSeconds()))
	} else {
		log.V(4).Info("Extension handler returned success response")
	}

	if options.WithCaching {
		// Add response to the cache.
		options.Cache.Add(runtimeclient.CallExtensionCacheEntry{
			CacheKey: cacheKey,
			Response: response,
		})
	}

	// Received a successful response from the extension handler. The `response` object
	// has been populated with the result. Return no error.
	return nil
}

// cloneAndAddSettings creates a new request object and adds settings to it.
func cloneAndAddSettings(request runtimehooksv1.RequestObject, registrationSettings map[string]string) runtimehooksv1.RequestObject {
	// Merge the settings from registration with the settings in the request.
	// The values in request take precedence over the values in the registration.
	// Create a deepcopy object to avoid side-effects on the request object.
	request = request.DeepCopyObject().(runtimehooksv1.RequestObject)
	settings := map[string]string{}
	for k, v := range registrationSettings {
		settings[k] = v
	}
	for k, v := range request.GetSettings() {
		settings[k] = v
	}
	request.SetSettings(settings)
	return request
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
	log := ctrl.LoggerFrom(ctx)
	if opts == nil || request == nil || response == nil {
		return errors.New("http call failed: opts, request and response cannot be nil")
	}
	if opts.catalog == nil {
		return errors.New("http call failed: opts.Catalog cannot be nil")
	}

	extensionURL, err := urlForExtension(opts.config, opts.registrationGVH, opts.name)
	if err != nil {
		return errors.Wrap(err, "http call failed")
	}

	// Observe request duration metric.
	start := time.Now()
	defer func() {
		runtimemetrics.RequestDuration.Observe(opts.hookGVH, *extensionURL, time.Since(start))
	}()
	requireConversion := opts.registrationGVH.Version != opts.hookGVH.Version

	requestLocal := request
	responseLocal := response

	if requireConversion {
		log.V(5).Info(fmt.Sprintf("Hook version of supported request is %s. Converting request from %s", opts.registrationGVH, opts.hookGVH))
		// The request and response objects need to be converted to match the version supported by
		// the ExtensionHandler.
		var err error

		// Create a new hook request object that is compatible with the version of ExtensionHandler.
		requestLocal, err = opts.catalog.NewRequest(opts.registrationGVH)
		if err != nil {
			return errors.Wrap(err, "http call failed")
		}

		// Convert the request to the version supported by the ExtensionHandler.
		if err := opts.catalog.Convert(request, requestLocal, ctx); err != nil {
			return errors.Wrapf(err, "http call failed: failed to convert request from %T to %T", request, requestLocal)
		}

		// Create a new hook response object that is compatible with the version of the ExtensionHandler.
		responseLocal, err = opts.catalog.NewResponse(opts.registrationGVH)
		if err != nil {
			return errors.Wrap(err, "http call failed")
		}
	}

	// Ensure the GroupVersionKind is set to the request.
	requestGVH, err := opts.catalog.Request(opts.registrationGVH)
	if err != nil {
		return errors.Wrap(err, "http call failed")
	}
	requestLocal.GetObjectKind().SetGroupVersionKind(requestGVH)

	postBody, err := json.Marshal(requestLocal)
	if err != nil {
		return errors.Wrap(err, "http call failed: failed to marshall request object")
	}

	if opts.timeout != 0 {
		// Make the call time-bound if timeout is non-zero value.
		values := extensionURL.Query()
		values.Add("timeout", opts.timeout.String())
		extensionURL.RawQuery = values.Encode()

		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.timeout)
		defer cancel()
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, extensionURL.String(), bytes.NewBuffer(postBody))
	if err != nil {
		return errors.Wrap(err, "http call failed: failed to create http request")
	}

	// Use client-go's transport.TLSConfigureFor to ensure good defaults for tls
	client := http.DefaultClient
	tlsConfig, err := transport.TLSConfigFor(&transport.Config{
		TLS: transport.TLSConfig{
			CAData:     opts.config.CABundle,
			ServerName: extensionURL.Hostname(),
		},
	})
	if err != nil {
		return errors.Wrap(err, "http call failed: failed to create tls config")
	}
	// This also adds http2
	client.Transport = utilnet.SetTransportDefaults(&http.Transport{
		TLSClientConfig: tlsConfig,
	})

	resp, err := client.Do(httpRequest)

	// Create http request metric.
	defer func() {
		runtimemetrics.RequestsTotal.Observe(httpRequest, resp, opts.hookGVH, err, response)
	}()

	if err != nil {
		return errCallingExtensionHandler(
			errors.Wrapf(err, "http call failed"),
		)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return errCallingExtensionHandler(
				errors.Errorf("http call failed: got response with status code %d != 200: failed to read response body", resp.StatusCode),
			)
		}

		return errCallingExtensionHandler(
			errors.Errorf("http call failed: got response with status code %d != 200: response: %q", resp.StatusCode, string(respBody)),
		)
	}

	if err := json.NewDecoder(resp.Body).Decode(responseLocal); err != nil {
		return errCallingExtensionHandler(
			errors.Wrap(err, "http call failed: failed to decode response"),
		)
	}

	if requireConversion {
		log.V(5).Info(fmt.Sprintf("Hook version of received response is %s. Converting response to %s", opts.registrationGVH, opts.hookGVH))
		// Convert the received response to the original version of the response object.
		if err := opts.catalog.Convert(responseLocal, response, ctx); err != nil {
			return errors.Wrapf(err, "http call failed: failed to convert response from %T to %T", requestLocal, response)
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
		u = &url.URL{
			Scheme: "https",
			Host:   host,
		}
		if svc.Path != nil {
			u.Path = *svc.Path
		}
	} else {
		if config.URL == nil {
			return nil, errors.New("failed to compute URL: at least one of service and url should be defined in config")
		}

		var err error
		u, err = url.Parse(*config.URL)
		if err != nil {
			return nil, errors.Wrap(err, "failed to compute URL: failed to parse url from clientConfig")
		}

		if u.Scheme != "https" {
			return nil, errors.Errorf("failed to compute URL: expected https scheme, got %s", u.Scheme)
		}
	}

	// Append the ExtensionHandler path.
	u.Path = path.Join(u.Path, runtimecatalog.GVHToPath(gvh, name))
	return u, nil
}

// defaultAndValidateDiscoveryResponse defaults unset values and runs a set of validations on the Discovery Response.
// If any of these checks fails the response is invalid and an error is returned.
func defaultAndValidateDiscoveryResponse(cat *runtimecatalog.Catalog, discovery *runtimehooksv1.DiscoveryResponse) error {
	if discovery == nil {
		return errors.New("failed to validate discovery response: response is nil")
	}

	discovery = defaultDiscoveryResponse(discovery)

	var errs []error
	names := make(map[string]bool)
	for _, handler := range discovery.Handlers {
		// Names should be unique.
		if _, ok := names[handler.Name]; ok {
			errs = append(errs, errors.Errorf("duplicate name for handler %s found", handler.Name))
		}
		names[handler.Name] = true

		// Name should match Kubernetes naming conventions - validated based on DNS1123 label rules.
		if errStrings := validation.IsDNS1123Label(handler.Name); len(errStrings) > 0 {
			errs = append(errs, errors.Errorf("handler name %s is not valid: %s", handler.Name, errStrings))
		}

		// Timeout should be a positive integer not greater than 30.
		if *handler.TimeoutSeconds < 0 || *handler.TimeoutSeconds > 30 {
			errs = append(errs, errors.Errorf("handler %s timeoutSeconds %d must be between 0 and 30", handler.Name, *handler.TimeoutSeconds))
		}

		// FailurePolicy must be one of Ignore or Fail.
		if *handler.FailurePolicy != runtimehooksv1.FailurePolicyFail && *handler.FailurePolicy != runtimehooksv1.FailurePolicyIgnore {
			errs = append(errs, errors.Errorf("handler %s failurePolicy %s must equal \"Ignore\" or \"Fail\"", handler.Name, *handler.FailurePolicy))
		}

		gv, err := schema.ParseGroupVersion(handler.RequestHook.APIVersion)
		if err != nil {
			errs = append(errs, errors.Wrapf(err, "handler %s requestHook APIVersion %s is not valid", handler.Name, handler.RequestHook.APIVersion))
		} else if !cat.IsHookRegistered(runtimecatalog.GroupVersionHook{
			Group:   gv.Group,
			Version: gv.Version,
			Hook:    handler.RequestHook.Hook,
		}) {
			errs = append(errs, errors.Errorf("handler %s requestHook %s/%s is not in the Runtime SDK catalog", handler.Name, handler.RequestHook.APIVersion, handler.RequestHook.Hook))
		}
	}

	return errors.Wrapf(kerrors.NewAggregate(errs), "failed to validate discovery response")
}

// defaultDiscoveryResponse defaults FailurePolicy and TimeoutSeconds for all discovered handlers.
func defaultDiscoveryResponse(discovery *runtimehooksv1.DiscoveryResponse) *runtimehooksv1.DiscoveryResponse {
	for i, handler := range discovery.Handlers {
		// If FailurePolicy is not defined set to "Fail".
		if handler.FailurePolicy == nil {
			defaultFailPolicy := runtimehooksv1.FailurePolicyFail
			handler.FailurePolicy = &defaultFailPolicy
		}

		// If TimeoutSeconds is not defined set to 10.
		if handler.TimeoutSeconds == nil {
			handler.TimeoutSeconds = ptr.To[int32](runtimehooksv1.DefaultHandlersTimeoutSeconds)
		}

		discovery.Handlers[i] = handler
	}
	return discovery
}

// matchNamespace returns true if the passed namespace matches the selector. It returns an error if the namespace does
// not exist in the API server.
func (c *client) matchNamespace(ctx context.Context, selector labels.Selector, namespace string) (bool, error) {
	// Return early if the selector is empty.
	if selector.Empty() {
		return true, nil
	}

	ns := &metav1.PartialObjectMetadata{}
	ns.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Namespace",
	})
	if err := c.client.Get(ctx, ctrlclient.ObjectKey{Name: namespace}, ns); err != nil {
		return false, errors.Wrapf(err, "failed to match namespace: failed to get namespace %s", namespace)
	}

	return selector.Matches(labels.Set(ns.GetLabels())), nil
}

// NameForHandler constructs a canonical name for a registered runtime extension handler.
func NameForHandler(handler runtimehooksv1.ExtensionHandler, extensionConfig *runtimev1.ExtensionConfig) (string, error) {
	if extensionConfig == nil {
		return "", errors.New("extensionConfig was nil")
	}
	return handler.Name + "." + extensionConfig.Name, nil
}

// ExtensionNameFromHandlerName extracts the extension name from the canonical name of a registered runtime extension handler.
func ExtensionNameFromHandlerName(registeredHandlerName string) (string, error) {
	parts := strings.Split(registeredHandlerName, ".")
	if len(parts) != 2 {
		return "", errors.Errorf("registered handler name %s was not in the expected format (`HANDLER_NAME.EXTENSION_NAME)", registeredHandlerName)
	}
	return parts[1], nil
}
