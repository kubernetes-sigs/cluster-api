//go:build !race

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

package v1beta1

import (
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
	"sigs.k8s.io/randfill"

	testv1 "sigs.k8s.io/cluster-api/internal/topology/upgrade/test/t2/v1beta2"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

// Test is disabled when the race detector is enabled (via "//go:build !race" above) because otherwise the fuzz tests would just time out.

func TestFuzzyConversion(t *testing.T) {
	t.Run("for TestResourceTemplate", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &testv1.TestResourceTemplate{},
		Spoke:       &TestResourceTemplate{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{XTestResourceTemplateFuzzFuncs},
		HubAfterMutation: func(hub conversion.Hub) {
			obj := hub.(*testv1.TestResourceTemplate)
			delete(obj.Annotations, "conversionTo")
		},
		SpokeAfterMutation: func(convertible conversion.Convertible) {
			obj := convertible.(*TestResourceTemplate)
			delete(obj.Annotations, "conversionTo")
		},
	}))
	t.Run("for TestResource", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &testv1.TestResource{},
		Spoke:       &TestResource{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{XTestResourceFuzzFuncs},
		HubAfterMutation: func(hub conversion.Hub) {
			obj := hub.(*testv1.TestResource)
			delete(obj.Annotations, "conversionTo")
		},
		SpokeAfterMutation: func(convertible conversion.Convertible) {
			obj := convertible.(*TestResource)
			delete(obj.Annotations, "conversionTo")
		},
	}))
}

func XTestResourceTemplateFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		spokeTestResourceSpec,
	}
}

func XTestResourceFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		spokeTestResourceSpec,
	}
}

func spokeTestResourceSpec(in *TestResourceSpec, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.PtrStringToString != nil && *in.PtrStringToString == "" {
		in.PtrStringToString = nil
	}

	if in.DurationToPtrInt32.Nanoseconds() != 0 {
		in.DurationToPtrInt32 = metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second}
	}

	// Drop BoolRemoved as we intentionally don't preserve it.
	in.BoolRemoved = false
}
