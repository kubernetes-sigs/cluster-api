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
	"testing"

	fuzz "github.com/google/gofuzz"
	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"

	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

const (
	fakeID     = "abcdef"
	fakeSecret = "abcdef0123456789"
)

// Test is disabled when the race detector is enabled (via "//go:build !race" above) because otherwise the fuzz tests would just time out.

func TestFuzzyConversion(t *testing.T) {
	t.Run("for KubeadmConfig", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &bootstrapv1.KubeadmConfig{},
		Spoke:       &KubeadmConfig{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{fuzzFuncs},
	}))
	t.Run("for KubeadmConfigTemplate", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &bootstrapv1.KubeadmConfigTemplate{},
		Spoke:       &KubeadmConfigTemplate{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{fuzzFuncs},
	}))
}

func fuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		// This custom functions are needed when ConvertTo/ConvertFrom functions
		// uses the json package to unmarshal the bootstrap token string.
		//
		// The Kubeadm BootstrapTokenString type ships with a custom
		// json string representation, in particular it supplies a customized
		// UnmarshalJSON function that can return an error if the string
		// isn't in the correct form.
		//
		// This function effectively disables any fuzzing for the token by setting
		// the values for ID and Secret to working alphanumeric values.
		kubeadmBootstrapTokenStringFuzzerV1Beta1,
		kubeadmBootstrapTokenStringFuzzerV1Alpha4,
	}
}

func kubeadmBootstrapTokenStringFuzzerV1Beta1(in *bootstrapv1.BootstrapTokenString, _ fuzz.Continue) {
	in.ID = fakeID
	in.Secret = fakeSecret
}

func kubeadmBootstrapTokenStringFuzzerV1Alpha4(in *BootstrapTokenString, _ fuzz.Continue) {
	in.ID = fakeID
	in.Secret = fakeSecret
}
