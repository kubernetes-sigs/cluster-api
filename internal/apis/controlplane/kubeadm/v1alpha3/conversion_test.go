//go:build !race

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
	"reflect"
	"testing"

	fuzz "github.com/google/gofuzz"
	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/utils/ptr"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/upstreamv1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta2"
	bootstrapv1alpha3 "sigs.k8s.io/cluster-api/internal/apis/bootstrap/kubeadm/v1alpha3"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

// Test is disabled when the race detector is enabled (via "//go:build !race" above) because otherwise the fuzz tests would just time out.

func TestFuzzyConversion(t *testing.T) {
	t.Run("for KubeadmControlPlane", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &controlplanev1.KubeadmControlPlane{},
		Spoke:       &KubeadmControlPlane{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{KubeadmControlPlaneFuzzFuncs},
	}))
}

func KubeadmControlPlaneFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		hubKubeadmControlPlaneStatus,
		spokeKubeadmControlPlaneStatus,
		spokeDNS,
		spokeKubeadmClusterConfiguration,
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
		hubBootstrapTokenString,
		spokeKubeadmBootstrapTokenString,
		spokeKubeadmConfigSpec,
	}
}

func hubKubeadmControlPlaneStatus(in *controlplanev1.KubeadmControlPlaneStatus, c fuzz.Continue) {
	c.FuzzNoCustom(in)
	// Always create struct with at least one mandatory fields.
	if in.Deprecated == nil {
		in.Deprecated = &controlplanev1.KubeadmControlPlaneDeprecatedStatus{}
	}
	if in.Deprecated.V1Beta1 == nil {
		in.Deprecated.V1Beta1 = &controlplanev1.KubeadmControlPlaneV1Beta1DeprecatedStatus{}
	}

	// Drop empty structs with only omit empty fields.
	if in.Initialization != nil {
		if reflect.DeepEqual(in.Initialization, &controlplanev1.KubeadmControlPlaneInitializationStatus{}) {
			in.Initialization = nil
		}
	}

	// nil becomes &0 after hub => spoke => hub conversion
	// This is acceptable as usually Replicas is set and controllers using older apiVersions are not writing MachineSet status.
	if in.Replicas == nil {
		in.Replicas = ptr.To(int32(0))
	}
}

func spokeKubeadmControlPlaneStatus(in *KubeadmControlPlaneStatus, c fuzz.Continue) {
	c.FuzzNoCustom(in)

	// Make sure ready is consistent with ready replicas, so we can rebuild the info after the round trip.
	in.Ready = in.ReadyReplicas > 0
}

func hubBootstrapTokenString(in *bootstrapv1.BootstrapTokenString, _ fuzz.Continue) {
	in.ID = "abcdef"
	in.Secret = "abcdef0123456789"
}

func spokeKubeadmBootstrapTokenString(in *upstreamv1beta1.BootstrapTokenString, _ fuzz.Continue) {
	in.ID = "abcdef"
	in.Secret = "abcdef0123456789"
}

func spokeDNS(obj *upstreamv1beta1.DNS, c fuzz.Continue) {
	c.FuzzNoCustom(obj)

	// DNS.Type does not exists in v1alpha4, so setting it to empty string in order to avoid v1alpha3 --> v1alpha4 --> v1alpha3 round trip errors.
	obj.Type = ""
}

func spokeKubeadmClusterConfiguration(obj *upstreamv1beta1.ClusterConfiguration, c fuzz.Continue) {
	c.FuzzNoCustom(obj)

	// ClusterConfiguration.UseHyperKubeImage has been removed in v1alpha4, so setting it to false in order to avoid v1alpha3 --> v1alpha4 --> v1alpha3 round trip errors.
	obj.UseHyperKubeImage = false
}

func spokeKubeadmConfigSpec(in *bootstrapv1alpha3.KubeadmConfigSpec, c fuzz.Continue) {
	c.FuzzNoCustom(in)

	// Drop UseExperimentalRetryJoin as we intentionally don't preserve it.
	in.UseExperimentalRetryJoin = false
}
