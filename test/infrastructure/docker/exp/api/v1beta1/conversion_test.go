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
	"sigs.k8s.io/randfill"

	infraexpv1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/exp/api/v1beta2"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

// Test is disabled when the race detector is enabled (via "//go:build !race" above) because otherwise the fuzz tests would just time out.

func TestFuzzyConversion(t *testing.T) {
	t.Run("for DockerMachinePool", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &infraexpv1.DockerMachinePool{},
		Spoke:       &DockerMachinePool{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{DockerMachinePoolFuzzFunc},
	}))

	t.Run("for DockerMachinePoolTemplate", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:   &infraexpv1.DockerMachinePoolTemplate{},
		Spoke: &DockerMachinePoolTemplate{},
	}))
}

func DockerMachinePoolFuzzFunc(_ runtimeserializer.CodecFactory) []any {
	return []any{
		hubDockerClusterStatus,
	}
}

func hubDockerClusterStatus(in *infraexpv1.DockerMachinePoolStatus, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.Deprecated != nil {
		if in.Deprecated.V1Beta1 == nil || reflect.DeepEqual(in.Deprecated.V1Beta1, &infraexpv1.DockerMachinePoolV1Beta1DeprecatedStatus{}) {
			in.Deprecated = nil
		}
	}
}
