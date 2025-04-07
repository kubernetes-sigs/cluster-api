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
	"testing"

	fuzz "github.com/google/gofuzz"
	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"

	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta2"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

// Test is disabled when the race detector is enabled (via "//go:build !race" above) because otherwise the fuzz tests would just time out.

func TestFuzzyConversion(t *testing.T) {
	t.Run("for MachinePool", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &expv1.MachinePool{},
		Spoke:       &MachinePool{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{MachinePoolFuzzFuncs},
	}))
}

func MachinePoolFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		hubMachinePoolStatus,
		spokeMachinePoolStatus,
	}
}

func hubMachinePoolStatus(in *expv1.MachinePoolStatus, c fuzz.Continue) {
	c.Fuzz(in)
	// Always create struct with at least one mandatory fields.
	if in.Deprecated == nil {
		in.Deprecated = &expv1.MachinePoolDeprecatedStatus{}
	}
	if in.Deprecated.V1Beta1 == nil {
		in.Deprecated.V1Beta1 = &expv1.MachinePoolV1Beta1DeprecatedStatus{}
	}
}

func spokeMachinePoolStatus(in *MachinePoolStatus, c fuzz.Continue) {
	c.Fuzz(in)
	// Drop empty structs with only omit empty fields.
	if in.V1Beta2 != nil {
		if in.V1Beta2.Conditions == nil && in.V1Beta2.AvailableReplicas == nil && in.V1Beta2.ReadyReplicas == nil && in.V1Beta2.UpToDateReplicas == nil {
			in.V1Beta2 = nil
		}
	}
}
