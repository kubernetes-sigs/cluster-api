/*
Copyright 2025 The Kubernetes Authors.

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

package v1beta1

import (
	"strconv"
	"testing"

	fuzz "github.com/google/gofuzz"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta2"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

// Test is disabled when the race detector is enabled (via "" above) because otherwise the fuzz tests would just time out.

func TestFuzzyConversion(t *testing.T) {
	t.Run("for Cluster", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &clusterv1.Cluster{},
		Spoke:       &Cluster{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{},
	}))
	t.Run("for ClusterClass", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &clusterv1.ClusterClass{},
		Spoke:       &ClusterClass{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{ClusterClassJSONFuzzFuncs},
	}))
	t.Run("for Machine", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &clusterv1.Machine{},
		Spoke:       &Machine{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{},
	}))

	t.Run("for MachineSet", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &clusterv1.MachineSet{},
		Spoke:       &MachineSet{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{},
	}))

	t.Run("for MachineDeployment", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &clusterv1.MachineDeployment{},
		Spoke:       &MachineDeployment{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{},
	}))

	t.Run("for MachineHealthCheck", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:   &clusterv1.MachineHealthCheck{},
		Spoke: &MachineHealthCheck{},
	}))
}

func ClusterClassJSONFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		JSONSchemaPropsFuzzer,
		JSONSchemaPropsFuzzerV1beta1,
	}
}

func JSONSchemaPropsFuzzer(in *clusterv1.JSONSchemaProps, c fuzz.Continue) {
	// NOTE: We have to fuzz the individual fields manually,
	// because we cannot call `FuzzNoCustom` as it would lead
	// to an infinite recursion.
	in.Type = c.RandString()
	for range c.Intn(10) {
		in.Required = append(in.Required, c.RandString())
	}
	in.MaxItems = ptr.To(c.Int63())
	in.MinItems = ptr.To(c.Int63())
	in.UniqueItems = c.RandBool()
	in.Format = c.RandString()
	in.MaxLength = ptr.To(c.Int63())
	in.MinLength = ptr.To(c.Int63())
	in.Pattern = c.RandString()
	in.Maximum = ptr.To(c.Int63())
	in.Maximum = ptr.To(c.Int63())
	in.ExclusiveMaximum = c.RandBool()
	in.Minimum = ptr.To(c.Int63())
	in.ExclusiveMinimum = c.RandBool()

	// Not every random byte array is valid JSON, e.g. a string without `""`,so we're setting valid values.
	in.Enum = []apiextensionsv1.JSON{
		{Raw: []byte("\"a\"")},
		{Raw: []byte("\"b\"")},
		{Raw: []byte("\"c\"")},
	}
	in.Default = &apiextensionsv1.JSON{Raw: []byte(strconv.FormatBool(c.RandBool()))}

	// We're using a copy of the current JSONSchemaProps,
	// because we cannot recursively fuzz new schemas.
	in2 := in.DeepCopy()
	in.AdditionalProperties = in2

	// We're using a copy of the current JSONSchemaProps,
	// because we cannot recursively fuzz new schemas.
	in.Properties = map[string]clusterv1.JSONSchemaProps{}
	for range c.Intn(10) {
		in.Properties[c.RandString()] = *in2
	}
	in.Items = in2
}

func JSONSchemaPropsFuzzerV1beta1(in *JSONSchemaProps, c fuzz.Continue) {
	// NOTE: We have to fuzz the individual fields manually,
	// because we cannot call `FuzzNoCustom` as it would lead
	// to an infinite recursion.
	in.Type = c.RandString()
	for range c.Intn(10) {
		in.Required = append(in.Required, c.RandString())
	}
	in.MaxItems = ptr.To(c.Int63())
	in.MinItems = ptr.To(c.Int63())
	in.UniqueItems = c.RandBool()
	in.Format = c.RandString()
	in.MaxLength = ptr.To(c.Int63())
	in.MinLength = ptr.To(c.Int63())
	in.Pattern = c.RandString()
	in.Maximum = ptr.To(c.Int63())
	in.Maximum = ptr.To(c.Int63())
	in.ExclusiveMaximum = c.RandBool()
	in.Minimum = ptr.To(c.Int63())
	in.ExclusiveMinimum = c.RandBool()

	// Not every random byte array is valid JSON, e.g. a string without `""`,so we're setting valid values.
	in.Enum = []apiextensionsv1.JSON{
		{Raw: []byte("\"a\"")},
		{Raw: []byte("\"b\"")},
		{Raw: []byte("\"c\"")},
	}
	in.Default = &apiextensionsv1.JSON{Raw: []byte(strconv.FormatBool(c.RandBool()))}

	// We're using a copy of the current JSONSchemaProps,
	// because we cannot recursively fuzz new schemas.
	in2 := in.DeepCopy()
	in.AdditionalProperties = in2

	// We're using a copy of the current JSONSchemaProps,
	// because we cannot recursively fuzz new schemas.
	in.Properties = map[string]JSONSchemaProps{}
	for range c.Intn(10) {
		in.Properties[c.RandString()] = *in2
	}
	in.Items = in2
}
