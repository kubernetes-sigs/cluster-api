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
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	clusterv1exp "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

func TestFuzzyConversion(t *testing.T) {
	t.Run("for MachinePool", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &clusterv1exp.MachinePool{},
		Spoke:       &MachinePool{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{fuzzFuncs},
	}))
}

func fuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		BootstrapFuzzer,
		MachinePoolSpecFuzzer,
		ObjectMetaFuzzer,
	}
}

func BootstrapFuzzer(in *clusterv1.Bootstrap, c fuzz.Continue) {
	c.FuzzNoCustom(in)

	// Bootstrap.Data has been removed in v1alpha4, so setting it to nil in order to avoid v1alpha3 --> <hub> --> v1alpha3 round trip errors.
	in.Data = nil
}

func ObjectMetaFuzzer(in *clusterv1.ObjectMeta, c fuzz.Continue) {
	c.FuzzNoCustom(in)

	// These fields have been removed in v1beta1
	// data is going to be lost, so we're forcing zero values here.
	in.Name = ""
	in.GenerateName = ""
	in.Namespace = ""
	in.OwnerReferences = nil
}

func MachinePoolSpecFuzzer(in *MachinePoolSpec, c fuzz.Continue) {
	c.Fuzz(in)

	// These fields have been removed in v1beta1
	// data is going to be lost, so we're forcing zero values here.
	in.Strategy = nil
}
