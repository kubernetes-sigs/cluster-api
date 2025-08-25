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

package v1alpha4

import (
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/randfill"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta2"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

// Test is disabled when the race detector is enabled (via "//go:build !race" above) because otherwise the fuzz tests would just time out.

func TestFuzzyConversion(t *testing.T) {
	t.Run("for DockerCluster", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &infrav1.DockerCluster{},
		Spoke:       &DockerCluster{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{DockerClusterFuzzFunc},
	}))

	t.Run("for DockerClusterTemplate", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &infrav1.DockerClusterTemplate{},
		Spoke:       &DockerClusterTemplate{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{DockerClusterTemplateFuzzFunc},
	}))

	t.Run("for DockerMachine", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &infrav1.DockerMachine{},
		Spoke:       &DockerMachine{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{DockerMachineFuzzFunc},
	}))

	t.Run("for DockerMachineTemplate", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &infrav1.DockerMachineTemplate{},
		Spoke:       &DockerMachineTemplate{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{DockerMachineTemplateFuzzFunc},
	}))
	t.Run("for DockerMachinePool", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &infrav1.DockerMachinePool{},
		Spoke:       &DockerMachinePool{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{DockerMachinePoolFuzzFunc},
	}))
}

func DockerClusterFuzzFunc(_ runtimeserializer.CodecFactory) []any {
	return []any{
		hubDockerClusterStatus,
		hubFailureDomain,
	}
}

func hubFailureDomain(in *clusterv1.FailureDomain, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.ControlPlane == nil {
		in.ControlPlane = ptr.To(false)
	}
}

func hubDockerClusterStatus(in *infrav1.DockerClusterStatus, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.Deprecated != nil {
		if in.Deprecated.V1Beta1 == nil || reflect.DeepEqual(in.Deprecated.V1Beta1, &infrav1.DockerClusterV1Beta1DeprecatedStatus{}) {
			in.Deprecated = nil
		}
	}
}

func DockerClusterTemplateFuzzFunc(_ runtimeserializer.CodecFactory) []any {
	return []any{
		hubFailureDomain,
	}
}

func DockerMachineFuzzFunc(_ runtimeserializer.CodecFactory) []any {
	return []any{
		hubDockerMachineStatus,
		spokeDockerMachineSpec,
	}
}

func hubDockerMachineStatus(in *infrav1.DockerMachineStatus, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.Deprecated != nil {
		if in.Deprecated.V1Beta1 == nil || reflect.DeepEqual(in.Deprecated.V1Beta1, &infrav1.DockerMachineV1Beta1DeprecatedStatus{}) {
			in.Deprecated = nil
		}
	}
}

func spokeDockerMachineSpec(in *DockerMachineSpec, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.ProviderID != nil && *in.ProviderID == "" {
		in.ProviderID = nil
	}
}

func DockerMachineTemplateFuzzFunc(_ runtimeserializer.CodecFactory) []any {
	return []any{
		spokeDockerMachineSpec,
	}
}

func DockerMachinePoolFuzzFunc(_ runtimeserializer.CodecFactory) []any {
	return []any{
		hubDockerMachinePoolStatus,
	}
}

func hubDockerMachinePoolStatus(in *infrav1.DockerMachinePoolStatus, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.Deprecated != nil {
		if in.Deprecated.V1Beta1 == nil || reflect.DeepEqual(in.Deprecated.V1Beta1, &infrav1.DockerMachinePoolV1Beta1DeprecatedStatus{}) {
			in.Deprecated = nil
		}
	}
}
