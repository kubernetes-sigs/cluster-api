/*
Copyright 2020 The Kubernetes Authors.

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

	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/upstreamv1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

func TestFuzzyConversion(t *testing.T) {
	t.Run("for KubeadmControlPlane", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &controlplanev1.KubeadmControlPlane{},
		Spoke:       &KubeadmControlPlane{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{fuzzFuncs},
	}))
}

func fuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	// This custom function is needed when ConvertTo/ConvertFrom functions
	// uses the json package to unmarshal the bootstrap token string.
	//
	// The Kubeadm v1beta1.BootstrapTokenString type ships with a custom
	// json string representation, in particular it supplies a customized
	// UnmarshalJSON function that can return an error if the string
	// isn't in the correct form.
	//
	// This function effectively disables any fuzzing for the token by setting
	// the values for ID and Secret to working alphanumeric values.
	return []interface{}{
		kubeadmBootstrapTokenStringFuzzer,
		cabpkBootstrapTokenStringFuzzer,
		dnsFuzzer,
		kubeadmClusterConfigurationFuzzer,
	}
}

func kubeadmBootstrapTokenStringFuzzer(in *upstreamv1beta1.BootstrapTokenString, _ fuzz.Continue) {
	in.ID = "abcdef"
	in.Secret = "abcdef0123456789"
}
func cabpkBootstrapTokenStringFuzzer(in *bootstrapv1.BootstrapTokenString, _ fuzz.Continue) {
	in.ID = "abcdef"
	in.Secret = "abcdef0123456789"
}

func dnsFuzzer(obj *upstreamv1beta1.DNS, c fuzz.Continue) {
	c.FuzzNoCustom(obj)

	// DNS.Type does not exists in v1alpha4, so setting it to empty string in order to avoid v1alpha3 --> v1alpha4 --> v1alpha3 round trip errors.
	obj.Type = ""
}

func kubeadmClusterConfigurationFuzzer(obj *upstreamv1beta1.ClusterConfiguration, c fuzz.Continue) {
	c.FuzzNoCustom(obj)

	// ClusterConfiguration.UseHyperKubeImage has been removed in v1alpha4, so setting it to false in order to avoid v1alpha3 --> v1alpha4 --> v1alpha3 round trip errors.
	obj.UseHyperKubeImage = false
}
