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

package config

import (
	"github.com/pkg/errors"
	"sigs.k8s.io/yaml"

	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
)

// MemoryReader provides a reader implementation backed by a map.
// This is to be used by the operator to place config from a secret
// and the ProviderSpec.Fetchconfig.
type MemoryReader struct {
	variables map[string]string
	providers []configProvider
}

var _ Reader = &MemoryReader{}

// NewMemoryReader return a new MemoryReader.
func NewMemoryReader() *MemoryReader {
	return &MemoryReader{
		variables: map[string]string{},
		providers: []configProvider{},
	}
}

// Init initialize the reader.
func (f *MemoryReader) Init(_ string) error {
	data, err := yaml.Marshal(f.providers)
	if err != nil {
		return err
	}
	f.variables["providers"] = string(data)

	// images is not used by the operator, but it is read by the clusterctrl
	// code, so we need a correct empty "images".
	data, err = yaml.Marshal(map[string]imageMeta{})
	if err != nil {
		return err
	}
	f.variables["images"] = string(data)
	return nil
}

// Get gets a value for the given key.
func (f *MemoryReader) Get(key string) (string, error) {
	if val, ok := f.variables[key]; ok {
		return val, nil
	}
	return "", errors.Errorf("value for variable %q is not set", key)
}

// Set sets a value for the given key.
func (f *MemoryReader) Set(key, value string) {
	f.variables[key] = value
}

// UnmarshalKey gets a value for the given key, then unmarshal it.
func (f *MemoryReader) UnmarshalKey(key string, rawval interface{}) error {
	data, err := f.Get(key)
	if err != nil {
		return nil //nolint:nilerr // We expect to not error if the key is not present
	}
	return yaml.Unmarshal([]byte(data), rawval)
}

// AddProvider adds the given provider to the "providers" map entry and returns any errors.
func (f *MemoryReader) AddProvider(name string, ttype clusterctlv1.ProviderType, url string) (*MemoryReader, error) {
	f.providers = append(f.providers, configProvider{
		Name: name,
		URL:  url,
		Type: ttype,
	})

	yaml, err := yaml.Marshal(f.providers)
	if err != nil {
		return f, err
	}
	f.variables["providers"] = string(yaml)

	return f, nil
}
