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
		hubKubeadmConfigStatus,
		spokeAPIServer,
		spokeDiscovery,
		spokeKubeadmConfigSpec,
		spokeKubeadmConfigStatus,
		spokeClusterConfiguration,
		hubBootstrapTokenString,
		spokeBootstrapTokenString,
		spokeBootstrapToken,
		hubKubeadmConfigSpec,
		hubNodeRegistrationOptions,
	}
}

func KubeadmConfigTemplateFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		spokeAPIServer,
		spokeDiscovery,
		spokeKubeadmConfigSpec,
		spokeClusterConfiguration,
		spokeBootstrapTokenString,
		hubBootstrapTokenString,
		spokeBootstrapToken,
		hubKubeadmConfigSpec,
		hubNodeRegistrationOptions,
	}
}

func hubBootstrapTokenString(in *bootstrapv1.BootstrapTokenString, _ randfill.Continue) {
	in.ID = fakeID
	in.Secret = fakeSecret
}

func spokeBootstrapTokenString(in *BootstrapTokenString, _ randfill.Continue) {
	in.ID = fakeID
	in.Secret = fakeSecret
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
}

func hubKubeadmConfigSpec(in *bootstrapv1.KubeadmConfigSpec, c randfill.Continue) {
	c.FillNoCustom(in)

	// enforce ControlPlaneComponentHealthCheckSeconds to be equal on init and join configuration
	initControlPlaneComponentHealthCheckSeconds := in.InitConfiguration.Timeouts.ControlPlaneComponentHealthCheckSeconds
	if in.JoinConfiguration.IsDefined() || initControlPlaneComponentHealthCheckSeconds != nil {
		in.JoinConfiguration.Timeouts.ControlPlaneComponentHealthCheckSeconds = initControlPlaneComponentHealthCheckSeconds
	}
	if in.ClusterConfiguration.APIServer.ExtraEnvs != nil && *in.ClusterConfiguration.APIServer.ExtraEnvs == nil {
		in.ClusterConfiguration.APIServer.ExtraEnvs = nil
	}
	if in.ClusterConfiguration.ControllerManager.ExtraEnvs != nil && *in.ClusterConfiguration.ControllerManager.ExtraEnvs == nil {
		in.ClusterConfiguration.ControllerManager.ExtraEnvs = nil
	}
	if in.ClusterConfiguration.Scheduler.ExtraEnvs != nil && *in.ClusterConfiguration.Scheduler.ExtraEnvs == nil {
		in.ClusterConfiguration.Scheduler.ExtraEnvs = nil
	}
	if in.ClusterConfiguration.Etcd.Local.ExtraEnvs != nil && *in.ClusterConfiguration.Etcd.Local.ExtraEnvs == nil {
		in.ClusterConfiguration.Etcd.Local.ExtraEnvs = nil
	}

	for i, arg := range in.ClusterConfiguration.APIServer.ExtraArgs {
		if arg.Value == nil {
			arg.Value = ptr.To("")
		}
		in.ClusterConfiguration.APIServer.ExtraArgs[i] = arg
	}
	for i, arg := range in.ClusterConfiguration.ControllerManager.ExtraArgs {
		if arg.Value == nil {
			arg.Value = ptr.To("")
		}
		in.ClusterConfiguration.ControllerManager.ExtraArgs[i] = arg
	}
	for i, arg := range in.ClusterConfiguration.Scheduler.ExtraArgs {
		if arg.Value == nil {
			arg.Value = ptr.To("")
		}
		in.ClusterConfiguration.Scheduler.ExtraArgs[i] = arg
	}
	for i, arg := range in.ClusterConfiguration.Etcd.Local.ExtraArgs {
		if arg.Value == nil {
			arg.Value = ptr.To("")
		}
		in.ClusterConfiguration.Etcd.Local.ExtraArgs[i] = arg
	}
	for i, arg := range in.InitConfiguration.NodeRegistration.KubeletExtraArgs {
		if arg.Value == nil {
			arg.Value = ptr.To("")
		}
		in.InitConfiguration.NodeRegistration.KubeletExtraArgs[i] = arg
	}
	for i, arg := range in.JoinConfiguration.NodeRegistration.KubeletExtraArgs {
		if arg.Value == nil {
			arg.Value = ptr.To("")
		}
		in.JoinConfiguration.NodeRegistration.KubeletExtraArgs[i] = arg
	}

	for i, p := range in.DiskSetup.Partitions {
		if p.Layout == nil {
			p.Layout = ptr.To(false) // Layout is a required field and nil does not round trip
		}
		in.DiskSetup.Partitions[i] = p
	}
}

func hubNodeRegistrationOptions(in *bootstrapv1.NodeRegistrationOptions, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.Taints != nil && *in.Taints == nil {
		in.Taints = nil
	}
}

func spokeKubeadmConfigSpec(in *KubeadmConfigSpec, c randfill.Continue) {
	c.FillNoCustom(in)

	// Drop UseExperimentalRetryJoin as we intentionally don't preserve it.
	in.UseExperimentalRetryJoin = false

	dropEmptyStringsKubeadmConfigSpec(in)

	if in.InitConfiguration != nil && in.InitConfiguration.Patches != nil && reflect.DeepEqual(in.InitConfiguration.Patches, &Patches{}) {
		in.InitConfiguration.Patches = nil
	}
	if in.JoinConfiguration != nil && in.JoinConfiguration.Patches != nil && reflect.DeepEqual(in.JoinConfiguration.Patches, &Patches{}) {
		in.JoinConfiguration.Patches = nil
	}
	if in.Ignition != nil {
		if in.Ignition.ContainerLinuxConfig != nil && reflect.DeepEqual(in.Ignition.ContainerLinuxConfig, &ContainerLinuxConfig{}) {
			in.Ignition.ContainerLinuxConfig = nil
		}
		if reflect.DeepEqual(in.Ignition, &IgnitionSpec{}) {
			in.Ignition = nil
		}
	}
	if in.DiskSetup != nil && reflect.DeepEqual(in.DiskSetup, &DiskSetup{}) {
		in.DiskSetup = nil
	}
	if in.NTP != nil && reflect.DeepEqual(in.NTP, &NTP{}) {
		in.NTP = nil
	}
	for i, file := range in.Files {
		if file.ContentFrom != nil && reflect.DeepEqual(file.ContentFrom, &FileSource{}) {
			file.ContentFrom = nil
		}
		in.Files[i] = file
	}
	for i, user := range in.Users {
		if user.PasswdFrom != nil && reflect.DeepEqual(user.PasswdFrom, &PasswdSource{}) {
			user.PasswdFrom = nil
		}
		in.Users[i] = user
	}
}

func spokeClusterConfiguration(in *ClusterConfiguration, c randfill.Continue) {
	c.FillNoCustom(in)

	// Drop the following fields as they have been removed in v1beta2, so we don't have to preserve them.
	in.Networking.ServiceSubnet = ""
	in.Networking.PodSubnet = ""
	in.Networking.DNSDomain = ""
	in.KubernetesVersion = ""
	in.ClusterName = ""

	if in.Etcd.Local != nil && reflect.DeepEqual(in.Etcd.Local, &LocalEtcd{}) {
		in.Etcd.Local = nil
	}
	if in.Etcd.External != nil && reflect.DeepEqual(in.Etcd.External, &ExternalEtcd{}) {
		in.Etcd.External = nil
	}
}

func spokeAPIServer(in *APIServer, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.TimeoutForControlPlane != nil {
		in.TimeoutForControlPlane = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
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

func spokeDiscovery(in *Discovery, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.Timeout != nil {
		in.Timeout = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
	if in.File != nil {
		if in.File.KubeConfig != nil {
			if in.File.KubeConfig.Cluster != nil && reflect.DeepEqual(in.File.KubeConfig.Cluster, &KubeConfigCluster{}) {
				in.File.KubeConfig.Cluster = nil
			}
			if in.File.KubeConfig.User.AuthProvider != nil && reflect.DeepEqual(in.File.KubeConfig.User.AuthProvider, &KubeConfigAuthProvider{}) {
				in.File.KubeConfig.User.AuthProvider = nil
			}
			if in.File.KubeConfig.User.Exec != nil && reflect.DeepEqual(in.File.KubeConfig.User.Exec, &KubeConfigAuthExec{}) {
				in.File.KubeConfig.User.Exec = nil
			}
			if reflect.DeepEqual(in.File.KubeConfig, &FileDiscoveryKubeConfig{}) {
				in.File.KubeConfig = nil
			}
		}
		if reflect.DeepEqual(in.File, &FileDiscovery{}) {
			in.File = nil
		}
	}
	if in.BootstrapToken != nil && reflect.DeepEqual(in.BootstrapToken, &BootstrapTokenDiscovery{}) {
		in.BootstrapToken = nil
	}
}

func spokeKubeadmConfigStatus(in *KubeadmConfigStatus, c randfill.Continue) {
	c.FillNoCustom(in)
	// Drop empty structs with only omit empty fields.
	if in.V1Beta2 != nil {
		if reflect.DeepEqual(in.V1Beta2, &KubeadmConfigV1Beta2Status{}) {
			in.V1Beta2 = nil
		}
	}

	dropEmptyStringsKubeadmConfigStatus(in)
}
