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

package upstreamv1beta4

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
		FuzzerFuncs:                []fuzzer.FuzzerFuncs{fuzzFuncs},
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
		spokeClusterConfigurationFuzzer,
		spokeDNSFuzzer,
		spokeInitConfigurationFuzzer,
		spokeJoinConfigurationFuzzer,
		spokeJoinControlPlaneFuzzer,
		spokeTimeoutsFuzzer,
		hubJoinConfigurationFuzzer,
		hubHostPathMountFuzzer,
		hubBootstrapTokenDiscoveryFuzzer,
		hubNodeRegistrationOptionsFuzzer,
		hubClusterConfigurationFuzzer,
	}
}

func initConfigurationFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		spokeBootstrapToken,
	}
}

// Custom fuzzers for kubeadm v1beta4 types.
// NOTES:
// - When fields do not exist in cabpk v1beta1 types, pinning them to avoid kubeadm v1beta4 --> cabpk v1beta1 --> kubeadm v1beta4 round trip errors.

func spokeClusterConfigurationFuzzer(obj *ClusterConfiguration, c randfill.Continue) {
	c.FillNoCustom(obj)

	obj.Proxy = Proxy{}
	obj.EncryptionAlgorithm = ""
	obj.CertificateValidityPeriod = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31n(3*365)+1) * time.Hour * 24})
	obj.CACertificateValidityPeriod = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31n(100*365)+1) * time.Hour * 24})

	// Drop the following fields as they have been removed in v1beta2, so we don't have to preserve them.
	obj.Networking.ServiceSubnet = ""
	obj.Networking.PodSubnet = ""
	obj.Networking.DNSDomain = ""
	obj.KubernetesVersion = ""
	obj.ClusterName = ""
}

func spokeDNSFuzzer(obj *DNS, c randfill.Continue) {
	c.FillNoCustom(obj)

	obj.Disabled = false
}

func spokeInitConfigurationFuzzer(obj *InitConfiguration, c randfill.Continue) {
	c.FillNoCustom(obj)

	obj.DryRun = false
	obj.CertificateKey = ""
	obj.Timeouts = nil
}

func spokeJoinConfigurationFuzzer(obj *JoinConfiguration, c randfill.Continue) {
	c.FillNoCustom(obj)

	obj.DryRun = false

	// If timeouts have been set, unset unsupported timeouts (only TLSBootstrap is supported - corresponds to JoinConfiguration.Discovery.Timeout in cabpk v1beta1 API
	if obj.Timeouts == nil {
		return
	}

	var supportedTimeouts *Timeouts
	if obj.Timeouts.TLSBootstrap != nil {
		supportedTimeouts = &Timeouts{
			TLSBootstrap: obj.Timeouts.TLSBootstrap,
		}
	}
	obj.Timeouts = supportedTimeouts
}

func spokeJoinControlPlaneFuzzer(obj *JoinControlPlane, c randfill.Continue) {
	c.FillNoCustom(obj)

	obj.CertificateKey = ""
}

func spokeTimeoutsFuzzer(obj *Timeouts, c randfill.Continue) {
	c.FillNoCustom(obj)

	if c.Bool() {
		obj.ControlPlaneComponentHealthCheck = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
		obj.KubeletHealthCheck = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
		obj.KubernetesAPICall = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
		obj.EtcdAPICall = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
		obj.TLSBootstrap = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
		obj.Discovery = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	} else {
		obj.ControlPlaneComponentHealthCheck = nil
		obj.KubeletHealthCheck = nil
		obj.KubernetesAPICall = nil
		obj.EtcdAPICall = nil
		obj.TLSBootstrap = nil
		obj.Discovery = nil
	}
	obj.UpgradeManifests = nil
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

func hubJoinConfigurationFuzzer(obj *bootstrapv1.JoinConfiguration, c randfill.Continue) {
	c.FillNoCustom(obj)

	if obj.Discovery.File != nil {
		obj.Discovery.File.KubeConfig = nil
	}
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

func hubNodeRegistrationOptionsFuzzer(obj *bootstrapv1.NodeRegistrationOptions, c randfill.Continue) {
	c.FillNoCustom(obj)

	if obj.Taints != nil && *obj.Taints == nil {
		obj.Taints = nil
	}
}

func hubClusterConfigurationFuzzer(obj *bootstrapv1.ClusterConfiguration, c randfill.Continue) {
	c.FillNoCustom(obj)

	obj.CertificateValidityPeriodDays = c.Int31n(3*365 + 1)
	obj.CACertificateValidityPeriodDays = c.Int31n(100*365 + 1)
}
