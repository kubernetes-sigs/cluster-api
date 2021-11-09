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

package webhookplayground

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	structuralschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	structuraldefaulting "k8s.io/apiextensions-apiserver/pkg/apiserver/schema/defaulting"
	structuralpruning "k8s.io/apiextensions-apiserver/pkg/apiserver/schema/pruning"
)

func TestDefaultingCRDs(t *testing.T) {

	tests := []struct {
		name   string
		schema string
		value              string
		wantDefaultedValue string
	}{
		{
			name: "default unset field",
			schema: `{
	"type": "object",
	"properties": {
		"bar": {
			"type": "string",
			"default": "defaultBar"
		}
	}
			}`,
			value: `{
}`,
			wantDefaultedValue: `{
	"bar": "defaultBar"
}`,
		},
		{
			name: "don't default empty field",
			schema: `{
	"type": "object",
	"properties": {
		"bar": {
			"type": "string",
			"default": "defaultBar"
		}
	}
			}`,
			value: `{
	"bar": ""
}`,
			wantDefaultedValue: `{
	"bar": ""
}`,
		},
		{
			name: "don't default if intermediate field does not exist",
			schema: `{
	"type": "object",
	"properties": {
		"bar": {
			"type": "object",
			"properties": {
				"baz": {
					"type": "string",
					"default": "defaultBar"
				}
			}
		}
	}
			}`,
			value: `{
}`,
			wantDefaultedValue: `{
}`,
		},
		{
			name: "default if intermediate field does exist",
			schema: `{
	"type": "object",
	"properties": {
		"bar": {
			"type": "object",
			"properties": {
				"baz": {
					"type": "string",
					"default": "defaultBar"
				}
			}
		}
	}
			}`,
			value: `{
	"bar": {}
}`,
			wantDefaultedValue: `{
	"bar": {
		"baz": "defaultBar"
	}
}`,
		},
	}

	// Context CRDs default if parent exist
	//
	// Defaulting
	// Variable does not exist in Cluster.spec.topology.variables:
	// 1) add the variable and set its default value (consistent with CRDs)
	// 2) change the Value to *apiextensions.JSON and only default if it is nil

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			schema := toSchema(g, tt.schema)
			value := toValue(g, tt.value)

			// Run structural schema defaulting.
			ss, err := structuralschema.NewStructural(schema)
			g.Expect(err).To(BeNil())

			structuralpruning.Prune(value, ss, false)
			structuraldefaulting.PruneNonNullableNullsWithoutDefaults(value, ss)
			structuraldefaulting.Default(value, ss)

			// Remove whitespace from wantDefaultedVariableTopologies JSON.
			var buffer bytes.Buffer
			if err := json.Compact(&buffer, []byte(tt.wantDefaultedValue)); err != nil {
				fmt.Println(err)
			}
			wantValueCompact := buffer.String()


			gotDefaultedValue, err := json.Marshal(value)
			g.Expect(err).To(BeNil())
			g.Expect(string(gotDefaultedValue)).To(Equal(wantValueCompact))
		})
	}

}

func toSchema(g *WithT, rawJSON string) *apiextensions.JSONSchemaProps {
	ret := apiextensions.JSONSchemaProps{}
	g.Expect(json.Unmarshal([]byte(rawJSON), &ret)).To(Succeed())
	return &ret
}

func toValue(g *WithT, rawJSON string) interface{} {
	var ret map[string]interface{}
	g.Expect(json.Unmarshal([]byte(rawJSON), &ret)).To(Succeed())
	return ret
}
