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
	"testing"
	"time"

	fuzz "github.com/google/gofuzz"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"

	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

func TestFuzzyConversion(t *testing.T) {
	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(AddToScheme(scheme)).To(Succeed())
	g.Expect(bootstrapv1.AddToScheme(scheme)).To(Succeed())

	t.Run("for ClusterConfiguration", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme: scheme,
		Hub:    &bootstrapv1.ClusterConfiguration{},
		Spoke:  &ClusterConfiguration{},
		// NOTE: Kubeadm types does not have ObjectMeta, so we are required to skip data annotation cleanup in the spoke-hub-spoke round trip test.
		SkipSpokeAnnotationCleanup: true,
		FuzzerFuncs:                []fuzzer.FuzzerFuncs{fuzzFuncs},
	}))
	t.Run("for InitConfiguration", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme: scheme,
		Hub:    &bootstrapv1.InitConfiguration{},
		Spoke:  &InitConfiguration{},
		// NOTE: Kubeadm types does not have ObjectMeta, so we are required to skip data annotation cleanup in the spoke-hub-spoke round trip test.
		SkipSpokeAnnotationCleanup: true,
		FuzzerFuncs:                []fuzzer.FuzzerFuncs{fuzzFuncs},
	}))
	t.Run("for JoinConfiguration", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme: scheme,
		Hub:    &bootstrapv1.JoinConfiguration{},
		Spoke:  &JoinConfiguration{},
		// NOTE: Kubeadm types does not have ObjectMeta, so we are required to skip data annotation cleanup in the spoke-hub-spoke round trip test.
		SkipSpokeAnnotationCleanup: true,
		FuzzerFuncs:                []fuzzer.FuzzerFuncs{fuzzFuncs},
	}))
}

func fuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		clusterConfigurationFuzzer,
		controlPlaneComponentFuzzer,
		dnsFuzzer,
		localEtcdFuzzer,
		initConfigurationFuzzer,
		joinConfigurationFuzzer,
		nodeRegistrationOptionsFuzzer,
		joinControlPlaneFuzzer,
		bootstrapv1APIServerFuzzer,
	}
}

// Custom fuzzers for kubeadm v1beta4 types.
// NOTES:
// - When fields do not exist in cabpk v1beta1 types, pinning them to avoid kubeadm v1beta4 --> cabpk v1beta1 --> kubeadm v1beta4 round trip errors.

func clusterConfigurationFuzzer(obj *ClusterConfiguration, c fuzz.Continue) {
	c.FuzzNoCustom(obj)

	obj.Proxy = Proxy{}
	obj.EncryptionAlgorithm = ""
	obj.CACertificateValidityPeriod = nil
	obj.CertificateValidityPeriod = nil
}

func controlPlaneComponentFuzzer(obj *ControlPlaneComponent, c fuzz.Continue) {
	c.FuzzNoCustom(obj)

	obj.ExtraEnvs = nil
}

func dnsFuzzer(obj *DNS, c fuzz.Continue) {
	c.FuzzNoCustom(obj)

	obj.Disabled = false
}

func localEtcdFuzzer(obj *LocalEtcd, c fuzz.Continue) {
	c.FuzzNoCustom(obj)

	obj.ExtraEnvs = nil
}

func initConfigurationFuzzer(obj *InitConfiguration, c fuzz.Continue) {
	c.FuzzNoCustom(obj)

	obj.DryRun = false
	obj.CertificateKey = ""
	obj.Timeouts = nil
}

func joinConfigurationFuzzer(obj *JoinConfiguration, c fuzz.Continue) {
	c.FuzzNoCustom(obj)

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

func nodeRegistrationOptionsFuzzer(obj *NodeRegistrationOptions, c fuzz.Continue) {
	c.FuzzNoCustom(obj)

	obj.ImagePullSerial = nil
}

func joinControlPlaneFuzzer(obj *JoinControlPlane, c fuzz.Continue) {
	c.FuzzNoCustom(obj)

	obj.CertificateKey = ""
}

// Custom fuzzers for CABPK v1beta1 types.
// NOTES:
// - When fields do not exist in kubeadm v1beta4 types, pinning them to avoid cabpk v1beta1 --> kubeadm v1beta4 --> cabpk v1beta1 round trip errors.

func bootstrapv1APIServerFuzzer(obj *bootstrapv1.APIServer, c fuzz.Continue) {
	c.FuzzNoCustom(obj)

	obj.TimeoutForControlPlane = nil
}

func TestTimeoutForControlPlaneMigration(t *testing.T) {
	timeout := metav1.Duration{Duration: 10 * time.Second}
	t.Run("from ClusterConfiguration to InitConfiguration and back", func(t *testing.T) {
		g := NewWithT(t)

		clusterConfiguration := &bootstrapv1.ClusterConfiguration{
			APIServer: bootstrapv1.APIServer{TimeoutForControlPlane: &timeout},
		}

		initConfiguration := &InitConfiguration{}
		err := initConfiguration.ConvertFromClusterConfiguration(clusterConfiguration)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(initConfiguration.Timeouts.ControlPlaneComponentHealthCheck).To(Equal(&timeout))

		clusterConfiguration = &bootstrapv1.ClusterConfiguration{}
		err = initConfiguration.ConvertToClusterConfiguration(clusterConfiguration)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(clusterConfiguration.APIServer.TimeoutForControlPlane).To(Equal(&timeout))
	})
	t.Run("from ClusterConfiguration to JoinConfiguration and back", func(t *testing.T) {
		g := NewWithT(t)

		clusterConfiguration := &bootstrapv1.ClusterConfiguration{
			APIServer: bootstrapv1.APIServer{TimeoutForControlPlane: &timeout},
		}

		joinConfiguration := &JoinConfiguration{}
		err := joinConfiguration.ConvertFromClusterConfiguration(clusterConfiguration)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(joinConfiguration.Timeouts.ControlPlaneComponentHealthCheck).To(Equal(&timeout))

		clusterConfiguration = &bootstrapv1.ClusterConfiguration{}
		err = joinConfiguration.ConvertToClusterConfiguration(clusterConfiguration)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(clusterConfiguration.APIServer.TimeoutForControlPlane).To(Equal(&timeout))
	})
}
