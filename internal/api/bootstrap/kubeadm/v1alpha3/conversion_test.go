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
	"time"

	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/randfill"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
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
		FuzzerFuncs: []fuzzer.FuzzerFuncs{KubeadmConfigFuzzFuncs},
	}))
	t.Run("for KubeadmConfigTemplate", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &bootstrapv1.KubeadmConfigTemplate{},
		Spoke:       &KubeadmConfigTemplate{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{KubeadmConfigTemplateFuzzFuncs},
	}))
}

func KubeadmConfigFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		spokeKubeadmConfigSpec,
		spokeKubeadmConfigStatus,
		spokeDNS,
		spokeClusterConfiguration,
		hubKubeadmConfigStatus,
		hubBootstrapTokenString,
		spokeBootstrapTokenString,
		spokeAPIServer,
		spokeDiscovery,
		hubKubeadmConfigSpec,
	}
}

func KubeadmConfigTemplateFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		spokeKubeadmConfigSpec,
		spokeKubeadmConfigStatus,
		spokeDNS,
		spokeClusterConfiguration,
		hubBootstrapTokenString,
		spokeBootstrapTokenString,
		spokeAPIServer,
		spokeDiscovery,
		hubKubeadmConfigSpec,
	}
}

func hubKubeadmConfigSpec(in *bootstrapv1.KubeadmConfigSpec, c randfill.Continue) {
	c.FillNoCustom(in)

	// enforce ControlPlaneComponentHealthCheckSeconds to be equal on init and join configuration
	var initControlPlaneComponentHealthCheckSeconds *int32
	if in.InitConfiguration != nil && in.InitConfiguration.Timeouts != nil {
		initControlPlaneComponentHealthCheckSeconds = in.InitConfiguration.Timeouts.ControlPlaneComponentHealthCheckSeconds
	}
	if (in.JoinConfiguration != nil && in.JoinConfiguration.Timeouts != nil) || initControlPlaneComponentHealthCheckSeconds != nil {
		if in.JoinConfiguration == nil {
			in.JoinConfiguration = &bootstrapv1.JoinConfiguration{}
		}
		if in.JoinConfiguration.Timeouts == nil {
			in.JoinConfiguration.Timeouts = &bootstrapv1.Timeouts{}
		}
		in.JoinConfiguration.Timeouts.ControlPlaneComponentHealthCheckSeconds = initControlPlaneComponentHealthCheckSeconds
	}
}

func hubKubeadmConfigStatus(in *bootstrapv1.KubeadmConfigStatus, c randfill.Continue) {
	c.FillNoCustom(in)
	// Always create struct with at least one mandatory fields.
	if in.Deprecated == nil {
		in.Deprecated = &bootstrapv1.KubeadmConfigDeprecatedStatus{}
	}
	if in.Deprecated.V1Beta1 == nil {
		in.Deprecated.V1Beta1 = &bootstrapv1.KubeadmConfigV1Beta1DeprecatedStatus{}
	}

	// Drop empty structs with only omit empty fields.
	if in.Initialization != nil {
		if reflect.DeepEqual(in.Initialization, &bootstrapv1.KubeadmConfigInitializationStatus{}) {
			in.Initialization = nil
		}
	}
}

func spokeKubeadmConfigSpec(in *KubeadmConfigSpec, c randfill.Continue) {
	c.FillNoCustom(in)

	// Drop UseExperimentalRetryJoin as we intentionally don't preserve it.
	in.UseExperimentalRetryJoin = false
}

func spokeKubeadmConfigStatus(obj *KubeadmConfigStatus, c randfill.Continue) {
	c.FillNoCustom(obj)

	// KubeadmConfigStatus.BootstrapData has been removed in v1alpha4, so setting it to nil in order to avoid v1alpha3 --> <hub> --> v1alpha3 round trip errors.
	obj.BootstrapData = nil
}

func spokeDNS(obj *DNS, c randfill.Continue) {
	c.FillNoCustom(obj)

	// DNS.Type does not exists in v1alpha4, so setting it to empty string in order to avoid v1alpha3 --> <hub> --> v1alpha3 round trip errors.
	obj.Type = ""
}

func spokeClusterConfiguration(obj *ClusterConfiguration, c randfill.Continue) {
	c.FillNoCustom(obj)

	// ClusterConfiguration.UseHyperKubeImage has been removed in v1alpha4, so setting it to false in order to avoid v1beta1 --> <hub> --> v1beta1 round trip errors.
	obj.UseHyperKubeImage = false

	// Drop the following fields as they have been removed in v1beta2, so we don't have to preserve them.
	obj.Networking.ServiceSubnet = ""
	obj.Networking.PodSubnet = ""
	obj.Networking.DNSDomain = ""
	obj.KubernetesVersion = ""
	obj.ControlPlaneEndpoint = ""
	obj.ClusterName = ""
}

func hubBootstrapTokenString(in *bootstrapv1.BootstrapTokenString, _ randfill.Continue) {
	in.ID = fakeID
	in.Secret = fakeSecret
}

func spokeBootstrapTokenString(in *BootstrapTokenString, _ randfill.Continue) {
	in.ID = fakeID
	in.Secret = fakeSecret
}

func spokeAPIServer(in *APIServer, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.TimeoutForControlPlane != nil {
		in.TimeoutForControlPlane = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
}

func spokeDiscovery(in *Discovery, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.Timeout != nil {
		in.Timeout = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
}
