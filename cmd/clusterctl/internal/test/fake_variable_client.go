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
	"fmt"

	"github.com/pkg/errors"
)

var (
	// PrivateGithubDomain is the private github enterprise domain used int tests.
	PrivateGithubDomain = "github.corp.co"
	// PrivateGithubToken is the test token for github enterprise client.
	PrivateGithubToken = "token"
)

// FakeVariableClient provides a VariableClient backed by a map.
type FakeVariableClient struct {
	variables map[string]string
}

func (f FakeVariableClient) Get(key string) (string, error) {
	if val, ok := f.variables[key]; ok {
		return val, nil
	}
	return "", errors.Errorf("value for variable %q is not set", key)
}

func (f FakeVariableClient) Set(key, value string) {
	f.variables[key] = value
}

func (f *FakeVariableClient) WithVar(key, value string) *FakeVariableClient {
	f.variables[key] = value
	return f
}

func NewFakeVariableClient() *FakeVariableClient {
	return &FakeVariableClient{
		variables: map[string]string{
			fmt.Sprintf("github-token-%s", PrivateGithubDomain): PrivateGithubToken,
		},
	}
}
