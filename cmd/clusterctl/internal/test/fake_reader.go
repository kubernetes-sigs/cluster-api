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

package test

import (
	"github.com/pkg/errors"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/yaml"
)

// FakeReader provider a reader implementation backed by a map
type FakeReader struct {
	initialized bool
	variables   map[string]string
	providers   []configProvider
	imageMetas  map[string]imageMeta
}

// configProvider is a mirror of config.Provider, re-implemented here in order to
// avoid circular dependencies between pkg/client/config and pkg/internal/test
type configProvider struct {
	Name string                    `json:"name,omitempty"`
	URL  string                    `json:"url,omitempty"`
	Type clusterctlv1.ProviderType `json:"type,omitempty"`
}

// imageMeta is a mirror of config.imageMeta, re-implemented here in order to
// avoid circular dependencies between pkg/client/config and pkg/internal/test
type imageMeta struct {
	Repository string `json:"repository,omitempty"`
	Tag        string `json:"tag,omitempty"`
}

func (f *FakeReader) Init(config string) error {
	f.initialized = true
	return nil
}

func (f *FakeReader) Get(key string) (string, error) {
	if val, ok := f.variables[key]; ok {
		return val, nil
	}
	return "", errors.Errorf("value for variable %q is not set", key)
}

func (f *FakeReader) Set(key, value string) {
	f.variables[key] = value
}

func (f *FakeReader) UnmarshalKey(key string, rawval interface{}) error {
	data, err := f.Get(key)
	if err != nil {
		return nil
	}
	return yaml.Unmarshal([]byte(data), rawval)
}

func NewFakeReader() *FakeReader {
	return &FakeReader{
		variables:  map[string]string{},
		imageMetas: map[string]imageMeta{},
	}
}

func (f *FakeReader) WithVar(key, value string) *FakeReader {
	f.variables[key] = value
	return f
}

func (f *FakeReader) WithProvider(name string, ttype clusterctlv1.ProviderType, url string) *FakeReader {
	f.providers = append(f.providers, configProvider{
		Name: name,
		URL:  url,
		Type: ttype,
	})

	yaml, _ := yaml.Marshal(f.providers)
	f.variables["providers"] = string(yaml)

	return f
}

func (f *FakeReader) WithImageMeta(component, repository, tag string) *FakeReader {
	f.imageMetas[component] = imageMeta{
		Repository: repository,
		Tag:        tag,
	}

	yaml, _ := yaml.Marshal(f.imageMetas)
	f.variables["images"] = string(yaml)

	return f
}
