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

	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta2"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

// Test is disabled when the race detector is enabled (via "//go:build !race" above) because otherwise the fuzz tests would just time out.

func TestFuzzyConversion(t *testing.T) {
	t.Run("for IPAddress", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:   &ipamv1.IPAddress{},
		Spoke: &IPAddress{},
	}))
	t.Run("for IPAddressClaim", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &ipamv1.IPAddressClaim{},
		Spoke:       &IPAddressClaim{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{IPAddressClaimFuzzFuncs},
	}))
}

func IPAddressClaimFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		hubIPAddressClaimStatus,
		spokeIPAddressClaimStatus,
	}
}

func hubIPAddressClaimStatus(in *ipamv1.IPAddressClaimStatus, c randfill.Continue) {
	c.FillNoCustom(in)
	// Drop empty structs with only omit empty fields.
	if in.Deprecated != nil {
		if in.Deprecated.V1Beta1 == nil || reflect.DeepEqual(in.Deprecated.V1Beta1, &ipamv1.IPAddressClaimV1Beta1DeprecatedStatus{}) {
			in.Deprecated = nil
		}
	}
}

func spokeIPAddressClaimStatus(in *IPAddressClaimStatus, c randfill.Continue) {
	c.FillNoCustom(in)
	// Drop empty structs with only omit empty fields.
	if in.V1Beta2 != nil {
		if reflect.DeepEqual(in.V1Beta2, &IPAddressClaimV1Beta2Status{}) {
			in.V1Beta2 = nil
		}
	}
}
