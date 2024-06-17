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

package topologymutation

import (
	"testing"

	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/utils/ptr"

	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
)

func Test_GetRawTemplateVariable(t *testing.T) {
	g := NewWithT(t)

	varA := apiextensionsv1.JSON{Raw: toJSON("a")}
	tests := []struct {
		name                  string
		variables             map[string]apiextensionsv1.JSON
		variableName          string
		expectedValue         *apiextensionsv1.JSON
		expectedNotFoundError bool
		expectedErr           bool
	}{
		{
			name:                  "Fails for invalid variable reference",
			variables:             nil,
			variableName:          "invalid[",
			expectedValue:         nil,
			expectedNotFoundError: false,
			expectedErr:           true,
		},
		{
			name:                  "variable not found",
			variables:             nil,
			variableName:          "notExists",
			expectedValue:         nil,
			expectedNotFoundError: true,
			expectedErr:           true,
		},
		{
			name: "return a variable",
			variables: map[string]apiextensionsv1.JSON{
				"a": varA,
			},
			variableName:          "a",
			expectedValue:         &varA,
			expectedNotFoundError: false,
			expectedErr:           false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(*testing.T) {
			value, err := GetVariable(tt.variables, tt.variableName)

			g.Expect(value).To(BeComparableTo(tt.expectedValue))
			if tt.expectedErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}

			if tt.expectedNotFoundError {
				g.Expect(IsNotFoundError(err)).To(BeTrue())
			}
		})
	}
}

func Test_GetStringTemplateVariable(t *testing.T) {
	g := NewWithT(t)

	varA := apiextensionsv1.JSON{Raw: toJSON("a")}
	tests := []struct {
		name                  string
		variables             map[string]apiextensionsv1.JSON
		variableName          string
		expectedValue         string
		expectedNotFoundError bool
		expectedErr           bool
	}{
		{
			name:                  "Fails for invalid variable reference",
			variables:             nil,
			variableName:          "invalid[",
			expectedValue:         "",
			expectedNotFoundError: false,
			expectedErr:           true,
		},
		{
			name:                  "variable not found",
			variables:             nil,
			variableName:          "notEsists",
			expectedValue:         "",
			expectedNotFoundError: true,
			expectedErr:           true,
		},
		{
			name: "valid variable",
			variables: map[string]apiextensionsv1.JSON{
				"a": varA,
			},
			variableName:          "a",
			expectedValue:         "a",
			expectedNotFoundError: false,
			expectedErr:           false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(*testing.T) {
			value, err := GetStringVariable(tt.variables, tt.variableName)

			g.Expect(value).To(Equal(tt.expectedValue))
			if tt.expectedErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}

			if tt.expectedNotFoundError {
				g.Expect(IsNotFoundError(err)).To(BeTrue())
			}
		})
	}
}

func Test_GetBoolVariable(t *testing.T) {
	g := NewWithT(t)

	varA := apiextensionsv1.JSON{Raw: []byte(`true`)}
	tests := []struct {
		name                  string
		variables             map[string]apiextensionsv1.JSON
		variableName          string
		expectedValue         bool
		expectedNotFoundError bool
		expectedErr           bool
	}{
		{
			name:                  "Fails for invalid variable reference",
			variables:             nil,
			variableName:          "invalid[",
			expectedValue:         false,
			expectedNotFoundError: false,
			expectedErr:           true,
		},
		{
			name:                  "variable not found",
			variables:             nil,
			variableName:          "notEsists",
			expectedValue:         false,
			expectedNotFoundError: true,
			expectedErr:           true,
		},
		{
			name: "valid variable",
			variables: map[string]apiextensionsv1.JSON{
				"a": varA,
			},
			variableName:          "a",
			expectedValue:         true,
			expectedNotFoundError: false,
			expectedErr:           false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(*testing.T) {
			value, err := GetBoolVariable(tt.variables, tt.variableName)
			if tt.expectedErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			if tt.expectedNotFoundError {
				g.Expect(IsNotFoundError(err)).To(BeTrue())
			}
			g.Expect(value).To(Equal(tt.expectedValue))
		})
	}
}

func Test_GetVariableObjectWithNestedType(t *testing.T) {
	type AddressesFromPool struct {
		APIGroup string `json:"apiGroup"`
		Kind     string `json:"kind"`
		Name     string `json:"name"`
	}
	type Network struct {
		AddressesFromPools *[]AddressesFromPool `json:"addressesFromPools,omitempty"`
		Ipv6Primary        *bool                `json:"ipv6Primary,omitempty"`
	}
	type WorkerKubeletExtraArgs map[string]string

	g := NewWithT(t)

	tests := []struct {
		name                   string
		variables              map[string]apiextensionsv1.JSON
		variableName           string
		expectedNotFoundError  bool
		expectedErr            bool
		object                 interface{}
		expectedVariableObject interface{}
	}{
		{
			name:                   "Fails for invalid variable reference",
			variables:              nil,
			variableName:           "invalid[",
			expectedNotFoundError:  false,
			object:                 &Network{},
			expectedVariableObject: &Network{},
			expectedErr:            true,
		},
		{
			name:                   "variable not found",
			variables:              nil,
			variableName:           "notEsists",
			expectedNotFoundError:  true,
			object:                 &Network{},
			expectedVariableObject: &Network{},
			expectedErr:            true,
		},
		{
			name: "unmarshal error",
			variables: map[string]apiextensionsv1.JSON{
				"node":    {Raw: []byte(`{"name": "aadfasdfasd`)},
				"network": {Raw: []byte(`{"ipv6Primary": true, "addressesFromPools":[{"name":"name"}]asdfasdf`)},
			},
			variableName:           "network",
			expectedNotFoundError:  false,
			object:                 &Network{},
			expectedVariableObject: &Network{},
			expectedErr:            true,
		},
		{
			name: "valid variable",
			variables: map[string]apiextensionsv1.JSON{
				"node":    {Raw: []byte(`{"name": "a"}`)},
				"network": {Raw: []byte(`{"ipv6Primary": true, "addressesFromPools":[{"name":"name"}]}`)},
			},
			variableName:          "network",
			expectedNotFoundError: false,
			expectedErr:           false,
			object:                &Network{},
			expectedVariableObject: &Network{
				Ipv6Primary: ptr.To(true),
				AddressesFromPools: &[]AddressesFromPool{
					{
						Name: "name",
					},
				},
			},
		},
		{
			name: "valid variable with encoded character",
			variables: map[string]apiextensionsv1.JSON{
				// Note: When a user uses `<` in a string in a variable it will be encoded as `\u003c`
				// This is already done by the APIserver and e.g. visible when doing a simple get cluster call.
				// This test case makes sure that variables that contain `<` are unmarshalled correctly.
				"workerKubeletExtraArgs": {Raw: []byte(`{"eviction-hard":"memory.available\u003c512M,nodefs.available\u003c5%","eviction-soft":"memory.available\u003c1024M,nodefs.available\u003c10%"}`)},
			},
			variableName:          "workerKubeletExtraArgs",
			expectedNotFoundError: false,
			expectedErr:           false,
			object:                &WorkerKubeletExtraArgs{},
			expectedVariableObject: &WorkerKubeletExtraArgs{
				"eviction-hard": "memory.available<512M,nodefs.available<5%",
				"eviction-soft": "memory.available<1024M,nodefs.available<10%",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(*testing.T) {
			err := GetObjectVariableInto(tt.variables, tt.variableName, tt.object)
			if tt.expectedErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			if tt.expectedNotFoundError {
				g.Expect(IsNotFoundError(err)).To(BeTrue())
			}
			g.Expect(tt.object).To(Equal(tt.expectedVariableObject))
		})
	}
}

func TestMergeVariables(t *testing.T) {
	t.Run("Merge variables", func(t *testing.T) {
		g := NewWithT(t)

		m, err := MergeVariableMaps(
			map[string]apiextensionsv1.JSON{
				runtimehooksv1.BuiltinsName: {Raw: []byte(`{"cluster":{"name":"cluster-name","namespace":"default","topology":{"class":"clusterClass1","version":"v1.21.1"}}}`)},
				"a":                         {Raw: []byte("a-different")},
				"c":                         {Raw: []byte("c")},
			},
			map[string]apiextensionsv1.JSON{
				// Verify that builtin variables are merged correctly and
				// the latter variables take precedent ("cluster-name-overwrite").
				runtimehooksv1.BuiltinsName: {Raw: []byte(`{"controlPlane":{"replicas":3},"cluster":{"name":"cluster-name-overwrite"}}`)},
				"a":                         {Raw: []byte("a")},
				"b":                         {Raw: []byte("b")},
			},
		)
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(m).To(HaveKeyWithValue(runtimehooksv1.BuiltinsName, apiextensionsv1.JSON{Raw: []byte(`{"cluster":{"name":"cluster-name-overwrite","namespace":"default","topology":{"version":"v1.21.1","class":"clusterClass1"}},"controlPlane":{"replicas":3}}`)}))
		g.Expect(m).To(HaveKeyWithValue("a", apiextensionsv1.JSON{Raw: []byte("a")}))
		g.Expect(m).To(HaveKeyWithValue("b", apiextensionsv1.JSON{Raw: []byte("b")}))
		g.Expect(m).To(HaveKeyWithValue("c", apiextensionsv1.JSON{Raw: []byte("c")}))
	})
}
