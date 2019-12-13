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

package client

import (
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/config"
)

// Client is exposes the clusterctl high-level client library
type Client interface {
	GetProvidersConfig() ([]Provider, error)
}

// clusterctlClient implements Client.
type clusterctlClient struct {
	configClient config.Client
}

// ensure clusterctlClient implements Client.
var _ Client = &clusterctlClient{}

// NewOptions carries the options supported by New
type NewOptions struct {
	injectConfig config.Client
}

// Option is a configuration option supplied to New
type Option func(*NewOptions)

// InjectConfig implements a New Option that allows to override the default configuration client used by clusterctl.
func InjectConfig(config config.Client) Option {
	return func(c *NewOptions) {
		c.injectConfig = config
	}
}

// New returns a configClient.
func New(path string, options ...Option) (Client, error) {
	return newClusterctlClient(path, options...)
}

func newClusterctlClient(path string, options ...Option) (*clusterctlClient, error) {
	cfg := &NewOptions{}
	for _, o := range options {
		o(cfg)
	}

	configClient := cfg.injectConfig
	if configClient == nil {
		c, err := config.New(path)
		if err != nil {
			return nil, err
		}
		configClient = c
	}

	return &clusterctlClient{
		configClient: configClient,
	}, nil
}
