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

package v1alpha4

import (
	"strconv"
	"testing"

	fuzz "github.com/google/gofuzz"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/utils/pointer"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

func TestFuzzyConversion(t *testing.T) {
	t.Run("for Cluster", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &clusterv1.Cluster{},
		Spoke:       &Cluster{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{ClusterJSONFuzzFuncs},
	}))
	t.Run("for ClusterClass", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &clusterv1.ClusterClass{},
		Spoke:       &ClusterClass{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{ClusterClassJSONFuzzFuncs},
	}))

	t.Run("for Machine", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &clusterv1.Machine{},
		Spoke:       &Machine{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{MachineStatusFuzzFunc},
	}))

	t.Run("for MachineSet", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:   &clusterv1.MachineSet{},
		Spoke: &MachineSet{},
	}))

	t.Run("for MachineDeployment", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:   &clusterv1.MachineDeployment{},
		Spoke: &MachineDeployment{},
	}))

	t.Run("for MachineHealthCheck", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:   &clusterv1.MachineHealthCheck{},
		Spoke: &MachineHealthCheck{},
	}))
}

func MachineStatusFuzzFunc(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		MachineStatusFuzzer,
	}
}

func MachineStatusFuzzer(in *MachineStatus, c fuzz.Continue) {
	c.FuzzNoCustom(in)

	// These fields have been removed in v1beta1
	// data is going to be lost, so we're forcing zero values to avoid round trip errors.
	in.Version = nil
}

func ClusterJSONFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		ClusterVariableFuzzer,
	}
}

func ClusterVariableFuzzer(in *clusterv1.ClusterVariable, c fuzz.Continue) {
	c.FuzzNoCustom(in)

	// Not every random byte array is valid JSON, e.g. a string without `""`,so we're setting a valid value.
	in.Value = apiextensionsv1.JSON{Raw: []byte("\"test-string\"")}
}

func ClusterClassJSONFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		JSONPatchFuzzer,
		JSONSchemaPropsFuzzer,
	}
}

func JSONPatchFuzzer(in *clusterv1.JSONPatch, c fuzz.Continue) {
	c.FuzzNoCustom(in)

	// Not every random byte array is valid JSON, e.g. a string without `""`,so we're setting a valid value.
	in.Value = &apiextensionsv1.JSON{Raw: []byte("5")}
}

func JSONSchemaPropsFuzzer(in *clusterv1.JSONSchemaProps, c fuzz.Continue) {
	// NOTE: We have to fuzz the individual fields manually,
	// because we cannot call `FuzzNoCustom` as it would lead
	// to an infinite recursion.
	in.Type = c.RandString()
	for i := 0; i < c.Intn(10); i++ {
		in.Required = append(in.Required, c.RandString())
	}
	in.MaxItems = pointer.Int64(c.Int63())
	in.MinItems = pointer.Int64(c.Int63())
	in.UniqueItems = c.RandBool()
	in.Format = c.RandString()
	in.MaxLength = pointer.Int64(c.Int63())
	in.MinLength = pointer.Int64(c.Int63())
	in.Pattern = c.RandString()
	in.Maximum = pointer.Int64(c.Int63())
	in.Maximum = pointer.Int64(c.Int63())
	in.ExclusiveMaximum = c.RandBool()
	in.Minimum = pointer.Int64(c.Int63())
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
	in.Properties = map[string]clusterv1.JSONSchemaProps{}
	for i := 0; i < c.Intn(10); i++ {
		in.Properties[c.RandString()] = *in.DeepCopy()
	}
	in.Items = in.DeepCopy()
}
