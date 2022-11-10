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
)

func Test_GetRawTemplateVariable(t *testing.T) {
	g := NewWithT(t)

	varA := apiextensionsv1.JSON{Raw: toJSON("a")}
	tests := []struct {
		name          string
		variables     map[string]apiextensionsv1.JSON
		variableName  string
		expectedValue *apiextensionsv1.JSON
		expectedFound bool
		expectedErr   bool
	}{
		{
			name:          "Fails for invalid variable reference",
			variables:     nil,
			variableName:  "invalid[",
			expectedValue: nil,
			expectedFound: false,
			expectedErr:   true,
		},
		{
			name:          "variable not found",
			variables:     nil,
			variableName:  "notEsists",
			expectedValue: nil,
			expectedFound: false,
			expectedErr:   false,
		},
		{
			name: "return a variable",
			variables: map[string]apiextensionsv1.JSON{
				"a": varA,
			},
			variableName:  "a",
			expectedValue: &varA,
			expectedFound: true,
			expectedErr:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, found, err := GetVariable(tt.variables, tt.variableName)

			g.Expect(value).To(Equal(tt.expectedValue))
			g.Expect(found).To(Equal(tt.expectedFound))
			if tt.expectedErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func Test_GetStringTemplateVariable(t *testing.T) {
	g := NewWithT(t)

	varA := apiextensionsv1.JSON{Raw: toJSON("a")}
	tests := []struct {
		name          string
		variables     map[string]apiextensionsv1.JSON
		variableName  string
		expectedValue string
		expectedFound bool
		expectedErr   bool
	}{
		{
			name:          "Fails for invalid variable reference",
			variables:     nil,
			variableName:  "invalid[",
			expectedValue: "",
			expectedFound: false,
			expectedErr:   true,
		},
		{
			name:          "variable not found",
			variables:     nil,
			variableName:  "notEsists",
			expectedValue: "",
			expectedFound: false,
			expectedErr:   false,
		},
		{
			name: "variable not found",
			variables: map[string]apiextensionsv1.JSON{
				"a": varA,
			},
			variableName:  "a",
			expectedValue: "a",
			expectedFound: true,
			expectedErr:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, found, err := GetStringVariable(tt.variables, tt.variableName)

			g.Expect(value).To(Equal(tt.expectedValue))
			g.Expect(found).To(Equal(tt.expectedFound))
			if tt.expectedErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}
