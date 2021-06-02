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
)

// Client is used to interact with the clusterctl configurations.
// Clusterctl v2 handles the following configurations:
// 1. The cert manager configuration (URL of the repository)
// 2. The configuration of the providers (name, type and URL of the provider repository)
// 3. Variables used when installing providers/creating clusters. Variables can be read from the environment or from the config file
// 4. The configuration about image overrides.
type Client interface {
	// CertManager provide access to the cert-manager configurations.
	CertManager() CertManagerClient

	// Providers provide access to provider configurations.
	Providers() ProvidersClient

	// Variables provide access to environment variables and/or variables defined in the clusterctl configuration file.
	Variables() VariablesClient

	// ImageMeta provide access to to image meta configurations.
	ImageMeta() ImageMetaClient
}

// configClient implements Client.
type configClient struct {
	reader Reader
}

// ensure configClient implements Client.
var _ Client = &configClient{}

func (c *configClient) CertManager() CertManagerClient {
	return newCertManagerClient(c.reader)
}

func (c *configClient) Providers() ProvidersClient {
	return newProvidersClient(c.reader)
}

func (c *configClient) Variables() VariablesClient {
	return newVariablesClient(c.reader)
}

func (c *configClient) ImageMeta() ImageMetaClient {
	return newImageMetaClient(c.reader)
}

// Option is a configuration option supplied to New.
type Option func(*configClient)

// InjectReader allows to override the default configuration reader used by clusterctl.
func InjectReader(reader Reader) Option {
	return func(c *configClient) {
		c.reader = reader
	}
}

// New returns a Client for interacting with the clusterctl configuration.
func New(path string, options ...Option) (Client, error) {
	return newConfigClient(path, options...)
}

func newConfigClient(path string, options ...Option) (*configClient, error) {
	client := &configClient{}
	for _, o := range options {
		o(client)
	}

	// if there is an injected reader, use it, otherwise use a default one
	if client.reader == nil {
		client.reader = newViperReader()
		if err := client.reader.Init(path); err != nil {
			return nil, errors.Wrap(err, "failed to initialize the configuration reader")
		}
	}

	return client, nil
}

// Reader define the behaviours of a configuration reader.
type Reader interface {
	// Init allows to initialize the configuration reader.
	Init(path string) error

	// Get returns a configuration value of type string.
	// In case the configuration value does not exists, it returns an error.
	Get(key string) (string, error)

	// Set allows to set an explicit override for a config value.
	// e.g. It is used to set an override from a flag value over environment/config file variables.
	Set(key, value string)

	// UnmarshalKey reads a configuration value and unmarshals it into the provided value object.
	UnmarshalKey(key string, value interface{}) error
}
