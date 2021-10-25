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
	"testing"

	fuzz "github.com/google/gofuzz"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
	"sigs.k8s.io/cluster-api/api/v1beta1"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

func TestFuzzyConversion(t *testing.T) {
	t.Run("for Cluster", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &v1beta1.Cluster{},
		Spoke:       &Cluster{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{ClusterJSONFuzzFuncs},
	}))
	t.Run("for ClusterClass", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &v1beta1.ClusterClass{},
		Spoke:       &ClusterClass{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{ClusterClassJSONFuzzFuncs},
	}))

	t.Run("for Machine", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &v1beta1.Machine{},
		Spoke:       &Machine{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{MachineStatusFuzzFunc},
	}))

	t.Run("for MachineSet", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:   &v1beta1.MachineSet{},
		Spoke: &MachineSet{},
	}))

	t.Run("for MachineDeployment", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:   &v1beta1.MachineDeployment{},
		Spoke: &MachineDeployment{},
	}))

	t.Run("for MachineHealthCheck", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:   &v1beta1.MachineHealthCheck{},
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

func ClusterVariableFuzzer(in *v1beta1.ClusterVariable, c fuzz.Continue) {
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

func JSONPatchFuzzer(in *v1beta1.JSONPatch, c fuzz.Continue) {
	c.FuzzNoCustom(in)

	// Not every random byte array is valid JSON, e.g. a string without `""`,so we're setting a valid value.
	in.Value = &apiextensionsv1.JSON{Raw: []byte("5")}
}

func JSONSchemaPropsFuzzer(in *v1beta1.JSONSchemaProps, c fuzz.Continue) {
	c.FuzzNoCustom(in)

	// Not every random byte array is valid JSON, e.g. a string without `""`,so we're setting valid values.
	in.Enum = []apiextensionsv1.JSON{
		{Raw: []byte("\"a\"")},
		{Raw: []byte("\"b\"")},
		{Raw: []byte("\"c\"")},
	}
	in.Default = &apiextensionsv1.JSON{Raw: []byte("true")}
}
