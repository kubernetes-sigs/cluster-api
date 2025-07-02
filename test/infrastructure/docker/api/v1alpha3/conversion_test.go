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

package v1alpha3

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

	t.Run("for DockerMachine", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &infrav1.DockerMachine{},
		Spoke:       &DockerMachine{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{DockerMachineFuzzFunc},
	}))

	t.Run("for DockerMachineTemplate", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:   &infrav1.DockerMachineTemplate{},
		Spoke: &DockerMachineTemplate{},
	}))
}

func DockerClusterFuzzFunc(_ runtimeserializer.CodecFactory) []any {
	return []any{
		hubDockerClusterStatus,
		hubFailureDomain,
	}
}

func hubDockerClusterStatus(in *infrav1.DockerClusterStatus, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.Deprecated != nil {
		if in.Deprecated.V1Beta1 == nil || reflect.DeepEqual(in.Deprecated.V1Beta1, &infrav1.DockerClusterV1Beta1DeprecatedStatus{}) {
			in.Deprecated = nil
		}
	}

	if in.Initialization != nil {
		if reflect.DeepEqual(in.Initialization, &infrav1.DockerClusterInitializationStatus{}) {
			in.Initialization = nil
		}
	}
}

func hubFailureDomain(in *clusterv1.FailureDomain, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.ControlPlane == nil {
		in.ControlPlane = ptr.To(false)
	}
}

func DockerMachineFuzzFunc(_ runtimeserializer.CodecFactory) []any {
	return []any{
		hubDockerMachineStatus,
	}
}

func hubDockerMachineStatus(in *infrav1.DockerMachineStatus, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.Deprecated != nil {
		if in.Deprecated.V1Beta1 == nil || reflect.DeepEqual(in.Deprecated.V1Beta1, &infrav1.DockerMachineV1Beta1DeprecatedStatus{}) {
			in.Deprecated = nil
		}
	}

	if in.Initialization != nil {
		if reflect.DeepEqual(in.Initialization, &infrav1.DockerMachineInitializationStatus{}) {
			in.Initialization = nil
		}
	}
}
