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

import "sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/test"

// VariablesClient has methods to work with environment variables and with variables defined in the clusterctl configuration file.
type VariablesClient interface {
	// Get returns a variable value. If the variable is not defined an error is returned.
	// In case the same variable is defined both within the environment variables and clusterctl configuration file,
	// the environment variables value takes precedence.
	Get(key string) (string, error)
}

// Ensures the FakeVariableClient implements VariablesClient
var _ VariablesClient = &test.FakeVariableClient{}

// variablesClient implements VariablesClient.
type variablesClient struct {
	reader Reader
}

// ensure variablesClient implements VariablesClient.
var _ VariablesClient = &variablesClient{}

func newVariablesClient(reader Reader) *variablesClient {
	return &variablesClient{
		reader: reader,
	}
}

func (p *variablesClient) Get(key string) (string, error) {
	return p.reader.GetString(key)
}
