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

package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"

	"k8s.io/apimachinery/pkg/runtime"

	runtimev1 "sigs.k8s.io/cluster-api/exp/runtime/api/v1beta1"
	catalog2 "sigs.k8s.io/cluster-api/internal/runtime/catalog"
	"sigs.k8s.io/cluster-api/internal/runtime/registry"
)

// Options are creation options for a Client.
type Options struct {
	Catalog  *catalog2.Catalog
	Registry registry.Registry
}

func New(options Options) Client {
	return &client{
		catalog:  options.Catalog,
		registry: options.Registry,
	}
}

const (
	StatusOK = http.StatusOK
)

type Client interface {
	Extension(ext *runtimev1.Extension) ExtensionClient
	Hook(service catalog2.Hook) HookClient

	ServiceOld(service catalog2.Hook, opts ...ServiceOption) ServiceClient
}

type client struct {
	catalog  *catalog2.Catalog
	registry registry.Registry

	host     string
	basePath string
	// TLS config
}

var _ Client = &client{}

type ExtensionClient interface {
	Discover() ([]runtimev1.RuntimeExtension, error)
}

func (c *client) Extension(ext *runtimev1.Extension) ExtensionClient {
	return &extensionClient{
		client: c,
		ext:    ext,
	}
}

type extensionClient struct {
	client *client
	ext    *runtimev1.Extension
	opts   []ServiceOption
}

func (e extensionClient) Discover() ([]runtimev1.RuntimeExtension, error) {
	panic("implement me")
}

type HookClient interface {
	Call(ctx context.Context, name string, in, out runtime.Object) error
	CallAll(ctx context.Context, in, out runtime.Object) error
}

func (c *client) Hook(hook catalog2.Hook) HookClient {
	return &hookClient{
		client: c,
		hook:   hook,
	}
}

type hookClient struct {
	client *client
	hook   catalog2.Hook
	opts   []ServiceOption
}

func (h hookClient) Call(ctx context.Context, name string, in, out runtime.Object) error {
	gvh, err := h.client.catalog.HookKind(h.hook)
	if err != nil {
		return err
	}

	registration := h.client.registry.GetRuntimeExtension(gvh, name)

	c := createHttpClient(registration)

	return c.Call(ctx, in, out)
}

func (h hookClient) CallAll(ctx context.Context, in, out runtime.Object) error {
	gvh, err := h.client.catalog.HookKind(h.hook)
	if err != nil {
		return err
	}

	registrations := h.client.registry.GetRuntimeExtensions(gvh)
	for _, registration := range registrations {
		c := createHttpClient(registration)

		if err := c.Call(ctx, in, out); err != nil {
			return err
		}
	}

	return nil
}

func createHttpClient(registration registry.RuntimeExtensionRegistration) httpClient {
	return httpClient{}
}

type httpClient struct {
}

func (h *httpClient) Call(ctx context.Context, in, out runtime.Object) error {
	return nil
}

type ServiceOption interface {
	ApplyToServiceOptions(*ServiceOptions)
}

type SpecVersion string

func (o SpecVersion) ApplyToServiceOptions(opts *ServiceOptions) {
	opts.Version = string(o)
}

type ServiceOptions struct {
	Version string // TODO: pointer?
}

type ServiceClient interface {
	Invoke(ctx context.Context, in, out runtime.Object) error
}

type serviceClient struct {
	client *client
	svc    catalog2.Hook
	opts   []ServiceOption
}

var _ ServiceClient = &serviceClient{}

func (c *client) ServiceOld(svc catalog2.Hook, opts ...ServiceOption) ServiceClient {
	return &serviceClient{
		client: c,
		svc:    svc,
		opts:   opts,
	}
}

func (s *serviceClient) Invoke(ctx context.Context, in, out runtime.Object) error {
	serviceOpts := &ServiceOptions{}
	for _, o := range s.opts {
		o.ApplyToServiceOptions(serviceOpts)
	}

	gvh, err := s.client.catalog.HookKind(s.svc)
	if err != nil {
		return err
	}

	requireConversion := serviceOpts.Version != gvh.Version

	inLocal := in
	outLocal := out
	if requireConversion {
		gvh.Version = serviceOpts.Version
		// TODO: validate gvh exists

		inLocal, err = s.client.catalog.NewInput(gvh)
		if err != nil {
			return err
		}

		if err := s.client.catalog.Convert(in, inLocal, ctx); err != nil {
			return err
		}

		outLocal, err = s.client.catalog.NewOutput(gvh)
		if err != nil {
			return err
		}
	}

	if _, err := s.client.catalog.RequestKind(gvh, catalog2.ValidateObject{Obj: inLocal}); err != nil {
		return err
	}

	if _, err := s.client.catalog.ResponseKind(gvh, catalog2.ValidateObject{Obj: outLocal}); err != nil {
		return err
	}

	postBody, err := json.Marshal(inLocal)
	if err != nil {
		// TODO: wrap err
		return err
	}

	// TODO: https + refactor how we are computing the url
	url := fmt.Sprintf("%s%s", s.client.host, path.Join(s.client.basePath, GVSToPath(gvh)))
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(postBody))
	if err != nil {
		// TODO: wrap err
		return err
	}
	defer resp.Body.Close()

	// TODO: get unstructured and convert to target gvk.
	if err := json.NewDecoder(resp.Body).Decode(&outLocal); err != nil {
		// TODO: wrap err
		return err
	}

	if requireConversion {
		if err := s.client.catalog.Convert(outLocal, out, ctx); err != nil {
			return err
		}
	}

	return nil
}
