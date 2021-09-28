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
	"testing"

	fuzz "github.com/google/gofuzz"
	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
	infraexpv1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/exp/api/v1beta1"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

func TestFuzzyConversion(t *testing.T) {
	t.Run("for DockerMachinePool", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &infraexpv1.DockerMachinePool{},
		Spoke:       &DockerMachinePool{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{DockerMachinePoolInstanceStatusFuzzFunc},
	}))
}

func DockerMachinePoolInstanceStatusFuzzFunc(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		DockerMachinePoolInstanceStatusFuzzer,
	}
}

func DockerMachinePoolInstanceStatusFuzzer(in *DockerMachinePoolInstanceStatus, c fuzz.Continue) {
	c.FuzzNoCustom(in)

	// Version field has been converted from *string to string in v1beta1,
	// so we're forcing valid string values to avoid round trip errors.
	if in.Version == nil {
		versionString := c.RandString()
		in.Version = &versionString
	}
}
