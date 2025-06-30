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

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/utils/ptr"
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
}
