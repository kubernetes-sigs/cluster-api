/*
Copyright 2019 The Kubernetes Authors.

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

package config

import (
	"github.com/pkg/errors"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/test"
)

// Client is used to interact with the clusterctl configurations.
// Clusterctl v2 handles two types of configs:
// 1. The configuration of the providers (name, type and URL of the provider repository)
// 2. Variables used when installing providers/creating clusters. Variables can be read from the environment or from the config file
type Client interface {
	// Providers provide access to provider configurations.
	Providers() ProvidersClient

	// Variables provide access to environment variables and/or variables defined in the clusterctl configuration file.
	Variables() VariablesClient
}

// configClient implements Client.
type configClient struct {
	reader Reader
}

// ensure configClient implements Client.
var _ Client = &configClient{}

func (c *configClient) Providers() ProvidersClient {
	return newProvidersClient(c.reader)
}

func (c *configClient) Variables() VariablesClient {
	return newVariablesClient(c.reader)
}

// NewOptions carries the options supported by New
type NewOptions struct {
	injectReader Reader
}

// Option is a configuration option supplied to New
type Option func(*NewOptions)

// InjectReader implements a New Option that allows to override the default configuration reader used by clusterctl.
func InjectReader(reader Reader) Option {
	return func(c *NewOptions) {
		c.injectReader = reader
	}
}

// New returns a Client for interacting with the clusterctl configuration.
func New(path string, options ...Option) (Client, error) {
	return newConfigClient(path, options...)
}

func newConfigClient(path string, options ...Option) (*configClient, error) {
	cfg := &NewOptions{}
	for _, o := range options {
		o(cfg)
	}

	reader := cfg.injectReader
	if reader == nil {
		reader = newViperReader()
	}

	if err := reader.Init(path); err != nil {
		return nil, errors.Wrap(err, "failed to initialize the configuration reader")
	}

	return &configClient{
		reader: reader,
	}, nil
}

// Reader define the behaviours of a configuration reader.
type Reader interface {
	// Init allows to initialize the configuration reader.
	Init(path string) error

	// GetString returns a configuration value of type string.
	// In case the configuration value does not exists, it returns an error.
	GetString(key string) (string, error)

	// UnmarshalKey reads a configuration value and unmarshals it into the provided value object.
	UnmarshalKey(key string, value interface{}) error
}

// Ensures the FakeReader implements reader
var _ Reader = &test.FakeReader{}
