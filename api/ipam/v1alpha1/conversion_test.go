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

package v1alpha1

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/randfill"

	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta2"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

// Test is disabled when the race detector is enabled (via "//go:build !race" above) because otherwise the fuzz tests would just time out.

func TestFuzzyConversion(t *testing.T) {
	t.Run("for IPAddress", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &ipamv1.IPAddress{},
		Spoke:       &IPAddress{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{IPAddressFuzzFuncs},
	}))
	t.Run("for IPAddressClaim", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &ipamv1.IPAddressClaim{},
		Spoke:       &IPAddressClaim{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{IPAddressClaimFuzzFuncs},
	}))
}

func IPAddressFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		hubIPAddressSpec,
		spokeTypedLocalObjectReference,
	}
}

func hubIPAddressSpec(in *ipamv1.IPAddressSpec, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.Prefix == nil {
		in.Prefix = ptr.To(int32(0)) // Prefix is a required field and nil does not round trip
	}
}

func spokeTypedLocalObjectReference(in *corev1.TypedLocalObjectReference, c randfill.Continue) {
	c.FillNoCustom(in)
	if in.APIGroup != nil && *in.APIGroup == "" {
		in.APIGroup = nil
	}
}

func IPAddressClaimFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		hubIPAddressClaimStatus,
		spokeTypedLocalObjectReference,
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
