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

package upstreamv1beta2

import (
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

// Test is disabled when the race detector is enabled (via "//go:build !race" above) because otherwise the fuzz tests would just time out.

func TestFuzzyConversion(t *testing.T) {
	t.Run("for ClusterConfiguration", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:   &bootstrapv1.ClusterConfiguration{},
		Spoke: &ClusterConfiguration{},
		// NOTE: Kubeadm types does not have ObjectMeta, so we are required to skip data annotation cleanup in the spoke-hub-spoke round trip test.
		SkipSpokeAnnotationCleanup: true,
		FuzzerFuncs:                []fuzzer.FuzzerFuncs{fuzzFuncs},
	}))
	t.Run("for ClusterStatus", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:   &bootstrapv1.ClusterStatus{},
		Spoke: &ClusterStatus{},
		// NOTE: Kubeadm types does not have ObjectMeta, so we are required to skip data annotation cleanup in the spoke-hub-spoke round trip test.
		SkipSpokeAnnotationCleanup: true,
		FuzzerFuncs:                []fuzzer.FuzzerFuncs{fuzzFuncs},
	}))
	t.Run("for InitConfiguration", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:   &bootstrapv1.InitConfiguration{},
		Spoke: &InitConfiguration{},
		// NOTE: Kubeadm types does not have ObjectMeta, so we are required to skip data annotation cleanup in the spoke-hub-spoke round trip test.
		SkipSpokeAnnotationCleanup: true,
		FuzzerFuncs:                []fuzzer.FuzzerFuncs{fuzzFuncs},
	}))
	t.Run("for JoinConfiguration", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:   &bootstrapv1.JoinConfiguration{},
		Spoke: &JoinConfiguration{},
		// NOTE: Kubeadm types does not have ObjectMeta, so we are required to skip data annotation cleanup in the spoke-hub-spoke round trip test.
		SkipSpokeAnnotationCleanup: true,
		FuzzerFuncs:                []fuzzer.FuzzerFuncs{fuzzFuncs},
	}))
}

func fuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		spokeClusterConfigurationFuzzer,
		spokeDNSFuzzer,
		spokeInitConfigurationFuzzer,
		spokeJoinConfigurationFuzzer,
		spokeJoinControlPlanesFuzzer,
		spokeAPIServerFuzzer,
		hubControlPlaneComponentFuzzer,
		hubLocalEtcdFuzzer,
		hubInitConfigurationFuzzer,
		hubJoinConfigurationFuzzer,
		hubNodeRegistrationOptionsFuzzer,
	}
}

// Custom fuzzers for kubeadm v1beta2 types.
// NOTES:
// - When fields do does not exist in cabpk v1beta1 types, pinning it to avoid kubeadm v1beta2 --> cabpk v1beta1 --> kubeadm v1beta2 round trip errors.

func spokeClusterConfigurationFuzzer(obj *ClusterConfiguration, c randfill.Continue) {
	c.FillNoCustom(obj)

	obj.UseHyperKubeImage = false
}

func spokeDNSFuzzer(obj *DNS, c randfill.Continue) {
	c.FillNoCustom(obj)

	obj.Type = ""
}

func spokeInitConfigurationFuzzer(obj *InitConfiguration, c randfill.Continue) {
	c.FillNoCustom(obj)

	obj.CertificateKey = ""
}

func spokeJoinControlPlanesFuzzer(obj *JoinControlPlane, c randfill.Continue) {
	c.FillNoCustom(obj)

	obj.CertificateKey = ""
}

func spokeJoinConfigurationFuzzer(obj *JoinConfiguration, c randfill.Continue) {
	c.FillNoCustom(obj)

	if obj.Discovery.Timeout != nil {
		obj.Discovery.Timeout = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
}

func spokeAPIServerFuzzer(obj *APIServer, c randfill.Continue) {
	c.FillNoCustom(obj)

	obj.TimeoutForControlPlane = nil
}

// Custom fuzzers for CABPK v1beta1 types.
// NOTES:
// - When fields do not exist in kubeadm v1beta2 types, pinning it to avoid cabpk v1beta1 --> kubeadm v1beta2 --> cabpk v1beta1 round trip errors.

func hubControlPlaneComponentFuzzer(obj *bootstrapv1.ControlPlaneComponent, c randfill.Continue) {
	c.FillNoCustom(obj)

	obj.ExtraEnvs = nil
}

func hubLocalEtcdFuzzer(obj *bootstrapv1.LocalEtcd, c randfill.Continue) {
	c.FillNoCustom(obj)

	obj.ExtraEnvs = nil
}

func hubInitConfigurationFuzzer(obj *bootstrapv1.InitConfiguration, c randfill.Continue) {
	c.FillNoCustom(obj)

	obj.Patches = nil
	obj.SkipPhases = nil
	obj.Timeouts = nil
}

func hubJoinConfigurationFuzzer(obj *bootstrapv1.JoinConfiguration, c randfill.Continue) {
	c.FillNoCustom(obj)

	obj.Patches = nil
	obj.SkipPhases = nil

	if obj.Discovery.File != nil {
		obj.Discovery.File.KubeConfig = nil
	}
	if obj.Timeouts != nil {
		if obj.Timeouts.TLSBootstrapSeconds != nil {
			obj.Timeouts = &bootstrapv1.Timeouts{TLSBootstrapSeconds: obj.Timeouts.TLSBootstrapSeconds}
		} else {
			obj.Timeouts = nil
		}
	}
}

func hubNodeRegistrationOptionsFuzzer(obj *bootstrapv1.NodeRegistrationOptions, c randfill.Continue) {
	c.FillNoCustom(obj)

	obj.ImagePullPolicy = ""
	obj.ImagePullSerial = nil
}
