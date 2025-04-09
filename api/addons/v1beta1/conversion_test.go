//go:build !race

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
	"reflect"
	"testing"

	fuzz "github.com/google/gofuzz"
	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"

	addonsv1 "sigs.k8s.io/cluster-api/api/addons/v1beta2"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

// Test is disabled when the race detector is enabled (via "//go:build !race" above) because otherwise the fuzz tests would just time out.

func TestFuzzyConversion(t *testing.T) {
	t.Run("for ClusterResourceSet", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &addonsv1.ClusterResourceSet{},
		Spoke:       &ClusterResourceSet{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{ClusterResourceSetFuzzFuncs},
	}))
	t.Run("for ClusterResourceSetBinding", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:   &addonsv1.ClusterResourceSetBinding{},
		Spoke: &ClusterResourceSetBinding{},
	}))
}

func ClusterResourceSetFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		hubClusterResourceSetStatus,
		spokeClusterResourceSetStatus,
	}
}

func hubClusterResourceSetStatus(in *addonsv1.ClusterResourceSetStatus, c fuzz.Continue) {
	c.FuzzNoCustom(in)
	// Drop empty structs with only omit empty fields.
	if in.Deprecated != nil {
		if in.Deprecated.V1Beta1 == nil || reflect.DeepEqual(in.Deprecated.V1Beta1, &addonsv1.ClusterResourceSetV1Beta1DeprecatedStatus{}) {
			in.Deprecated = nil
		}
	}
}

func spokeClusterResourceSetStatus(in *ClusterResourceSetStatus, c fuzz.Continue) {
	c.FuzzNoCustom(in)
	// Drop empty structs with only omit empty fields.
	if in.V1Beta2 != nil {
		if reflect.DeepEqual(in.V1Beta2, &ClusterResourceSetV1Beta2Status{}) {
			in.V1Beta2 = nil
		}
	}
}
