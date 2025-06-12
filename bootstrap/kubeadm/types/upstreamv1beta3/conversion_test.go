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

package upstreamv1beta3

import (
	"reflect"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/randfill"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/upstreamhub"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

// Test is disabled when the race detector is enabled (via "//go:build !race" above) because otherwise the fuzz tests would just time out.

func TestFuzzyConversion(t *testing.T) {
	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(AddToScheme(scheme)).To(Succeed())
	g.Expect(bootstrapv1.AddToScheme(scheme)).To(Succeed())

	t.Run("for ClusterConfiguration", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme: scheme,
		Hub:    &upstreamhub.ClusterConfiguration{},
		Spoke:  &ClusterConfiguration{},
		// NOTE: Kubeadm types does not have ObjectMeta, so we are required to skip data annotation cleanup in the spoke-hub-spoke round trip test.
		SkipSpokeAnnotationCleanup: true,
		FuzzerFuncs:                []fuzzer.FuzzerFuncs{fuzzFuncs, clusterConfigurationFuzzFuncs},
	}))
	t.Run("for InitConfiguration", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme: scheme,
		Hub:    &upstreamhub.InitConfiguration{},
		Spoke:  &InitConfiguration{},
		// NOTE: Kubeadm types does not have ObjectMeta, so we are required to skip data annotation cleanup in the spoke-hub-spoke round trip test.
		SkipSpokeAnnotationCleanup: true,
		FuzzerFuncs:                []fuzzer.FuzzerFuncs{fuzzFuncs, initConfigurationFuzzFuncs},
	}))
	t.Run("for JoinConfiguration", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme: scheme,
		Hub:    &upstreamhub.JoinConfiguration{},
		Spoke:  &JoinConfiguration{},
		// NOTE: Kubeadm types does not have ObjectMeta, so we are required to skip data annotation cleanup in the spoke-hub-spoke round trip test.
		SkipSpokeAnnotationCleanup: true,
		FuzzerFuncs:                []fuzzer.FuzzerFuncs{fuzzFuncs},
	}))
}

func fuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		spokeInitConfigurationFuzzer,
		spokeJoinConfigurationFuzzer,
		spokeAPIServerFuzzer,
		hubJoinConfigurationFuzzer,
		spokeNodeRegistrationOptionsFuzzer,
		spokeJoinControlPlanesFuzzer,
		hubInitConfigurationFuzzer,
		hubAPIServerFuzzer,
		hubControllerManagerFuzzer,
		hubSchedulerFuzzer,
		hubLocalEtcdFuzzer,
		hubNodeRegistrationOptionsFuzzer,
		hubHostPathMountFuzzer,
		hubBootstrapTokenDiscoveryFuzzer,
		hubClusterConfigurationFuzzer,
	}
}

func clusterConfigurationFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		spokeClusterConfigurationFuzzer,
	}
}

func initConfigurationFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		spokeBootstrapToken,
	}
}

// Custom fuzzers for kubeadm v1beta3 types.
// NOTES:
// - When fields do not exist in cabpk v1beta1 types, pinning it to avoid kubeadm v1beta3 --> cabpk v1beta1 --> kubeadm v1beta3 round trip errors.

func spokeClusterConfigurationFuzzer(in *ClusterConfiguration, c randfill.Continue) {
	c.FillNoCustom(in)

	// Drop the following fields as they have been removed in v1beta2, so we don't have to preserve them.
	in.Networking.ServiceSubnet = ""
	in.Networking.PodSubnet = ""
	in.Networking.DNSDomain = ""
	in.KubernetesVersion = ""
	in.ClusterName = ""
}

func spokeInitConfigurationFuzzer(obj *InitConfiguration, c randfill.Continue) {
	c.FillNoCustom(obj)

	obj.CertificateKey = ""
	obj.SkipPhases = nil
}

func spokeJoinConfigurationFuzzer(obj *JoinConfiguration, c randfill.Continue) {
	c.FillNoCustom(obj)

	obj.SkipPhases = nil
	if obj.Discovery.Timeout != nil {
		obj.Discovery.Timeout = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
}

func spokeAPIServerFuzzer(obj *APIServer, c randfill.Continue) {
	c.FillNoCustom(obj)

	obj.TimeoutForControlPlane = nil
}

func hubJoinConfigurationFuzzer(obj *bootstrapv1.JoinConfiguration, c randfill.Continue) {
	c.FillNoCustom(obj)

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

func spokeNodeRegistrationOptionsFuzzer(obj *NodeRegistrationOptions, c randfill.Continue) {
	c.FillNoCustom(obj)

	obj.IgnorePreflightErrors = nil
}

func spokeJoinControlPlanesFuzzer(obj *JoinControlPlane, c randfill.Continue) {
	c.FillNoCustom(obj)

	obj.CertificateKey = ""
}

func spokeBootstrapToken(in *BootstrapToken, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.TTL != nil {
		in.TTL = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
	if reflect.DeepEqual(in.Expires, &metav1.Time{}) {
		in.Expires = nil
	}
}

func hubAPIServerFuzzer(obj *bootstrapv1.APIServer, c randfill.Continue) {
	c.FillNoCustom(obj)

	obj.ExtraEnvs = nil
}

func hubControllerManagerFuzzer(obj *bootstrapv1.ControllerManager, c randfill.Continue) {
	c.FillNoCustom(obj)

	obj.ExtraEnvs = nil
}

func hubSchedulerFuzzer(obj *bootstrapv1.Scheduler, c randfill.Continue) {
	c.FillNoCustom(obj)

	obj.ExtraEnvs = nil
}

func hubLocalEtcdFuzzer(obj *bootstrapv1.LocalEtcd, c randfill.Continue) {
	c.FillNoCustom(obj)

	obj.ExtraEnvs = nil
}

func hubNodeRegistrationOptionsFuzzer(obj *bootstrapv1.NodeRegistrationOptions, c randfill.Continue) {
	c.FillNoCustom(obj)

	obj.ImagePullSerial = nil

	if obj.Taints != nil && *obj.Taints == nil {
		obj.Taints = nil
	}
}

func hubInitConfigurationFuzzer(obj *bootstrapv1.InitConfiguration, c randfill.Continue) {
	c.FillNoCustom(obj)

	obj.Timeouts = nil
}

func hubHostPathMountFuzzer(obj *bootstrapv1.HostPathMount, c randfill.Continue) {
	c.FillNoCustom(obj)

	if obj.ReadOnly == nil {
		obj.ReadOnly = ptr.To(false)
	}
}

func hubBootstrapTokenDiscoveryFuzzer(obj *bootstrapv1.BootstrapTokenDiscovery, c randfill.Continue) {
	c.FillNoCustom(obj)

	if obj.UnsafeSkipCAVerification == nil {
		obj.UnsafeSkipCAVerification = ptr.To(false)
	}
}

func hubClusterConfigurationFuzzer(obj *bootstrapv1.ClusterConfiguration, c randfill.Continue) {
	c.FillNoCustom(obj)

	obj.CertificateValidityPeriodDays = 0
	obj.CACertificateValidityPeriodDays = 0
}
