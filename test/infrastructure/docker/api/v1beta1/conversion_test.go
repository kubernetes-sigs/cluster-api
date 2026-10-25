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

	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/randfill"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta2"
	conversionutil "sigs.k8s.io/cluster-api/util/conversion"
)

// Test is disabled when the race detector is enabled (via "//go:build !race" above) because otherwise the fuzz tests would just time out.

func TestFuzzyConversion(t *testing.T) {
	t.Run("for DevCluster", conversionutil.FuzzTestFunc(conversionutil.FuzzTestFuncInput{
		Hub:         &infrav1.DevCluster{},
		Spoke:       &DevCluster{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{DevClusterFuzzFunc},
	}))

	t.Run("for DevClusterTemplate", conversionutil.FuzzTestFunc(conversionutil.FuzzTestFuncInput{
		Hub:         &infrav1.DevClusterTemplate{},
		Spoke:       &DevClusterTemplate{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{DevClusterTemplateFuzzFunc},
	}))

	t.Run("for DevMachine", conversionutil.FuzzTestFunc(conversionutil.FuzzTestFuncInput{
		Hub:         &infrav1.DevMachine{},
		Spoke:       &DevMachine{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{DevMachineFuzzFunc},
	}))

	t.Run("for DevMachineTemplate", conversionutil.FuzzTestFunc(conversionutil.FuzzTestFuncInput{
		Hub:         &infrav1.DevMachineTemplate{},
		Spoke:       &DevMachineTemplate{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{DevMachineTemplateFuzzFunc},
	}))
}

func hubFailureDomain(in *clusterv1.FailureDomain, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.ControlPlane == nil {
		in.ControlPlane = ptr.To(false)
	}
}

func DevClusterFuzzFunc(_ runtimeserializer.CodecFactory) []any {
	return []any{
		hubDevClusterStatus,
		hubFailureDomain,
		spokeDevClusterStatus,
	}
}

func hubDevClusterStatus(in *infrav1.DevClusterStatus, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.Deprecated != nil {
		if in.Deprecated.V1Beta1 == nil || reflect.DeepEqual(in.Deprecated.V1Beta1, &infrav1.DevClusterV1Beta1DeprecatedStatus{}) {
			in.Deprecated = nil
		}
	}
}

func spokeDevClusterStatus(in *DevClusterStatus, c randfill.Continue) {
	c.FillNoCustom(in)

	// Drop empty structs with only omit empty fields.
	if in.V1Beta2 != nil {
		if reflect.DeepEqual(in.V1Beta2, &DevClusterV1Beta2Status{}) {
			in.V1Beta2 = nil
		}
	}
}

func DevClusterTemplateFuzzFunc(_ runtimeserializer.CodecFactory) []any {
	return []any{
		hubFailureDomain,
	}
}

func DevMachineFuzzFunc(_ runtimeserializer.CodecFactory) []any {
	return []any{
		hubDevMachineStatus,
		spokeDevMachineSpec,
		spokeDevMachineStatus,
	}
}

func hubDevMachineStatus(in *infrav1.DevMachineStatus, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.Deprecated != nil {
		if in.Deprecated.V1Beta1 == nil || reflect.DeepEqual(in.Deprecated.V1Beta1, &infrav1.DevMachineV1Beta1DeprecatedStatus{}) {
			in.Deprecated = nil
		}
	}
}

func spokeDevMachineSpec(in *DevMachineSpec, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.ProviderID != nil && *in.ProviderID == "" {
		in.ProviderID = nil
	}
}

func spokeDevMachineStatus(in *DevMachineStatus, c randfill.Continue) {
	c.FillNoCustom(in)

	// Drop empty structs with only omit empty fields.
	if in.V1Beta2 != nil {
		if reflect.DeepEqual(in.V1Beta2, &DevMachineV1Beta2Status{}) {
			in.V1Beta2 = nil
		}
	}
}

func DevMachineTemplateFuzzFunc(_ runtimeserializer.CodecFactory) []any {
	return []any{
		spokeDevMachineSpec,
	}
}
