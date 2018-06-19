/*
Copyright 2018 The Kubernetes Authors.

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

package providercomponents_test

import (
	"fmt"
	"testing"

	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/cluster-api/clusterctl/providercomponents"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

func TestLoadFromConfigMap(t *testing.T) {
	providerComponentsContent := "content\nmore content >>"
	configMapName := "clusterctl"
	providerComponentsKey := "provider-components"
	testCases := []struct {
		name                 string
		getResult            *core.ConfigMap
		getErr               error
		expectedErrorMessage string
	}{
		{"config map exists;key exists", newConfigMap(configMapName, map[string]string{providerComponentsKey: providerComponentsContent}), nil, ""},
		{"get error", nil, fmt.Errorf("this is the error string"), "error getting configmap named 'clusterctl': this is the error string"},
		{"config map exists;key doesn't exist", newConfigMap(configMapName, map[string]string{}), nil, "configmap 'clusterctl' does not contain the provider components key 'provider-components'"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockConfigMap := newMockConfigMap()
			mockConfigMap.GetResult = tc.getResult
			mockConfigMap.GetErr = tc.getErr
			store, err := providercomponents.NewFromConfigMap(mockConfigMap)
			if err != nil {
				t.Fatalf("error creating provider components store: %v", err)
			}
			value, err := store.Load()
			if err == nil {
				if tc.expectedErrorMessage != "" {
					t.Fatalf("error mismatch: got '%v', want '%v'", err, tc.expectedErrorMessage)
				}
				if value != providerComponentsContent {
					t.Errorf("provider components content mismatch: got '%v', want '%v'", value, providerComponentsContent)
				}
			} else {
				if err.Error() != tc.expectedErrorMessage {
					t.Errorf("error message mismatch: got '%v', want '%v'", err, tc.expectedErrorMessage)
				}
			}
		})
	}
}

func TestSaveToConfigMap(t *testing.T) {
	providerComponentsContent := "content\nmore content >>"
	configMapName := "clusterctl"
	providerComponentsKey := "provider-components"
	notFoundErr := errors.NewNotFound(v1alpha1.Resource("configmap"), configMapName)
	testCases := []struct {
		name                 string
		getResult            *core.ConfigMap
		getErr               error
		createErr            error
		updateErr            error
		expectedDataLen      int
		expectedErrorMessage string
	}{
		{"random error retrieving config map", nil, fmt.Errorf("random config map error"), nil, nil, 1, "unable to get configmap 'clusterctl': random config map error"},
		{"new config map, success", nil, notFoundErr, nil, nil, 1, ""},
		{"new config map, error", nil, notFoundErr, fmt.Errorf("create has failed"), nil, 0, "error creating config map 'clusterctl': create has failed"},
		{"existing config map, error", newConfigMap(configMapName, nil), nil, nil, fmt.Errorf("update has failed"), 1, "error updating config map 'clusterctl': update has failed"},
		{"existing config map with nil map", newConfigMap(configMapName, nil), nil, nil, nil, 1, ""},
		{"existing config map with existing, different key", newConfigMap(configMapName, map[string]string{providerComponentsKey: "different value"}), nil, nil, nil, 1, ""},
		{"existing config map with existing, same key", newConfigMap(configMapName, map[string]string{providerComponentsKey: "different value", "another-key": "another-value"}), nil, nil, nil, 2, ""},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockConfigMap := newMockConfigMap()
			mockConfigMap.GetResult = tc.getResult
			mockConfigMap.GetErr = tc.getErr
			mockConfigMap.CreateErr = tc.createErr
			mockConfigMap.UpdateErr = tc.updateErr
			store, err := providercomponents.NewFromConfigMap(mockConfigMap)
			if err != nil {
				t.Fatalf("error creating provider components store: %v", err)
			}
			err = store.Save(providerComponentsContent)
			if err != nil || tc.expectedErrorMessage != "" {
				if err == nil {
					t.Fatalf("expected error")
				}
				if err.Error() != tc.expectedErrorMessage {
					t.Errorf("error mismatch: got '%v', want '%v'", err, tc.expectedErrorMessage)
				}
				return
			}
			var capturedConfigMap core.ConfigMap
			if tc.getErr == nil {
				capturedConfigMap = mockConfigMap.CapturedUpdateArg
			} else {
				capturedConfigMap = mockConfigMap.CapturedCreateArg
			}
			if capturedConfigMap.Name != configMapName {
				t.Errorf("unexpected config map name: got '%v', want '%v'", mockConfigMap.CapturedCreateArg.Name, configMapName)
			}
			if capturedConfigMap.Data == nil {
				t.Fatalf("create argument's 'data' field is nil")
			}
			if len(capturedConfigMap.Data) != tc.expectedDataLen {
				t.Errorf("data map length mismatch: got %v want %v", len(capturedConfigMap.Data), tc.expectedDataLen)
			}
			value, ok := capturedConfigMap.Data[providerComponentsKey]
			if !ok {
				t.Errorf("missing a value for '%v'", providerComponentsKey)
			}
			if value != providerComponentsContent {
				t.Errorf("provider components content mismatch: got '%v', want '%v'", value, providerComponentsContent)
			}
		})
	}
}

func newMockConfigMap() *MockConfigMap {
	return &MockConfigMap{}
}

func newConfigMap(name string, data map[string]string) *core.ConfigMap {
	return &core.ConfigMap{
		ObjectMeta: meta.ObjectMeta{
			Name: name,
		},
		Data: data,
	}
}

type MockConfigMap struct {
	CapturedGetNameArg    string
	CapturedGetOptionsArg meta.GetOptions
	GetResult             *core.ConfigMap
	GetErr                error
	CapturedCreateArg     core.ConfigMap
	CreateResult          *core.ConfigMap
	CreateErr             error
	CapturedUpdateArg     core.ConfigMap
	UpdateResult          *core.ConfigMap
	UpdateErr             error
}

func (c *MockConfigMap) Get(name string, options meta.GetOptions) (*core.ConfigMap, error) {
	c.CapturedGetNameArg = name
	c.CapturedGetOptionsArg = options
	return c.GetResult, c.GetErr
}

func (c *MockConfigMap) List(opts meta.ListOptions) (result *core.ConfigMapList, err error) {
	return
}

func (c *MockConfigMap) Watch(opts meta.ListOptions) (w watch.Interface, err error) {
	return
}

func (c *MockConfigMap) Create(configMap *core.ConfigMap) (*core.ConfigMap, error) {
	c.CapturedCreateArg = *configMap
	return c.CreateResult, c.CreateErr
}

func (c *MockConfigMap) Update(configMap *core.ConfigMap) (*core.ConfigMap, error) {
	c.CapturedUpdateArg = *configMap
	return c.UpdateResult, c.UpdateErr
}

func (c *MockConfigMap) Delete(name string, options *meta.DeleteOptions) (err error) {
	return
}

func (c *MockConfigMap) DeleteCollection(options *meta.DeleteOptions, listOptions meta.ListOptions) (err error) {
	return
}

// Patch applies the patch and returns the patched configMap.
func (c *MockConfigMap) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *core.ConfigMap, err error) {
	return
}
