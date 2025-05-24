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
	"github.com/pkg/errors"
	apimachineryconversion "k8s.io/apimachinery/pkg/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

func (src *ClusterConfiguration) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*bootstrapv1.ClusterConfiguration)
	return Convert_upstreamv1beta2_ClusterConfiguration_To_v1beta2_ClusterConfiguration(src, dst, nil)
}

func (dst *ClusterConfiguration) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*bootstrapv1.ClusterConfiguration)
	return Convert_v1beta2_ClusterConfiguration_To_upstreamv1beta2_ClusterConfiguration(src, dst, nil)
}

func (src *ClusterStatus) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*bootstrapv1.ClusterStatus)
	return Convert_upstreamv1beta2_ClusterStatus_To_v1beta2_ClusterStatus(src, dst, nil)
}

func (dst *ClusterStatus) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*bootstrapv1.ClusterStatus)
	return Convert_v1beta2_ClusterStatus_To_upstreamv1beta2_ClusterStatus(src, dst, nil)
}

func (src *InitConfiguration) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*bootstrapv1.InitConfiguration)
	return Convert_upstreamv1beta2_InitConfiguration_To_v1beta2_InitConfiguration(src, dst, nil)
}

func (dst *InitConfiguration) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*bootstrapv1.InitConfiguration)
	return Convert_v1beta2_InitConfiguration_To_upstreamv1beta2_InitConfiguration(src, dst, nil)
}

func (src *JoinConfiguration) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*bootstrapv1.JoinConfiguration)
	return Convert_upstreamv1beta2_JoinConfiguration_To_v1beta2_JoinConfiguration(src, dst, nil)
}

func (dst *JoinConfiguration) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*bootstrapv1.JoinConfiguration)
	return Convert_v1beta2_JoinConfiguration_To_upstreamv1beta2_JoinConfiguration(src, dst, nil)
}

// Custom conversion from this API, kubeadm v1beta2, to the hub version, CABPK v1beta1.

func Convert_upstreamv1beta2_ClusterConfiguration_To_v1beta2_ClusterConfiguration(in *ClusterConfiguration, out *bootstrapv1.ClusterConfiguration, s apimachineryconversion.Scope) error {
	// ClusterConfiguration.UseHyperKubeImage was removed in CABPK v1alpha4 API version, dropping this info (no issue, it was not used).
	return autoConvert_upstreamv1beta2_ClusterConfiguration_To_v1beta2_ClusterConfiguration(in, out, s)
}

func Convert_upstreamv1beta2_JoinConfiguration_To_v1beta2_JoinConfiguration(in *JoinConfiguration, out *bootstrapv1.JoinConfiguration, s apimachineryconversion.Scope) error {
	if err := autoConvert_upstreamv1beta2_JoinConfiguration_To_v1beta2_JoinConfiguration(in, out, s); err != nil {
		return err
	}
	if in.Discovery.Timeout != nil {
		if out.Timeouts == nil {
			out.Timeouts = &bootstrapv1.Timeouts{}
		}
		out.Timeouts.TLSBootstrapSeconds = utilconversion.ConvertToSeconds(in.Discovery.Timeout)
	}
	return nil
}

func Convert_upstreamv1beta2_DNS_To_v1beta2_DNS(in *DNS, out *bootstrapv1.DNS, s apimachineryconversion.Scope) error {
	// DNS.Type does not exist in CABPK v1beta1 version, because it always was CoreDNS.
	return autoConvert_upstreamv1beta2_DNS_To_v1beta2_DNS(in, out, s)
}

func Convert_upstreamv1beta2_InitConfiguration_To_v1beta2_InitConfiguration(in *InitConfiguration, out *bootstrapv1.InitConfiguration, s apimachineryconversion.Scope) error {
	// InitConfiguration.CertificateKey does not exist in CABPK v1beta1 version, because Cluster API does not use automatic copy certs.
	return autoConvert_upstreamv1beta2_InitConfiguration_To_v1beta2_InitConfiguration(in, out, s)
}

func Convert_upstreamv1beta2_JoinControlPlane_To_v1beta2_JoinControlPlane(in *JoinControlPlane, out *bootstrapv1.JoinControlPlane, s apimachineryconversion.Scope) error {
	// JoinControlPlane.CertificateKey does not exist in CABPK v1beta1 version, because Cluster API does not use automatic copy certs.
	return autoConvert_upstreamv1beta2_JoinControlPlane_To_v1beta2_JoinControlPlane(in, out, s)
}

func Convert_upstreamv1beta2_APIServer_To_v1beta2_APIServer(in *APIServer, out *bootstrapv1.APIServer, s apimachineryconversion.Scope) error {
	// APIServer.TimeoutForControlPlane does not exist in CABPK, because CABPK aligns to upstreamV1Beta4.
	return autoConvert_upstreamv1beta2_APIServer_To_v1beta2_APIServer(in, out, s)
}

func Convert_upstreamv1beta2_Discovery_To_v1beta2_Discovery(in *Discovery, out *bootstrapv1.Discovery, s apimachineryconversion.Scope) error {
	// Discovery.Timeout does not exist in CABPK, because CABPK aligns to upstreamV1Beta4.
	return autoConvert_upstreamv1beta2_Discovery_To_v1beta2_Discovery(in, out, s)
}

func Convert_upstreamv1beta2_ControlPlaneComponent_To_v1beta2_ControlPlaneComponent(in *ControlPlaneComponent, out *bootstrapv1.ControlPlaneComponent, s apimachineryconversion.Scope) error {
	out.ExtraArgs = utilconversion.ConvertToArgs(in.ExtraArgs)
	return autoConvert_upstreamv1beta2_ControlPlaneComponent_To_v1beta2_ControlPlaneComponent(in, out, s)
}

func Convert_upstreamv1beta2_LocalEtcd_To_v1beta2_LocalEtcd(in *LocalEtcd, out *bootstrapv1.LocalEtcd, s apimachineryconversion.Scope) error {
	out.ExtraArgs = utilconversion.ConvertToArgs(in.ExtraArgs)
	return autoConvert_upstreamv1beta2_LocalEtcd_To_v1beta2_LocalEtcd(in, out, s)
}

func Convert_upstreamv1beta2_NodeRegistrationOptions_To_v1beta2_NodeRegistrationOptions(in *NodeRegistrationOptions, out *bootstrapv1.NodeRegistrationOptions, s apimachineryconversion.Scope) error {
	out.KubeletExtraArgs = utilconversion.ConvertToArgs(in.KubeletExtraArgs)
	return autoConvert_upstreamv1beta2_NodeRegistrationOptions_To_v1beta2_NodeRegistrationOptions(in, out, s)
}

// Custom conversion from the hub version, CABPK v1beta1, to this API, kubeadm v1beta2.

func Convert_v1beta2_InitConfiguration_To_upstreamv1beta2_InitConfiguration(in *bootstrapv1.InitConfiguration, out *InitConfiguration, s apimachineryconversion.Scope) error {
	// InitConfiguration.SkipPhases, InitConfiguration.Timeouts and Patches do not exist in kubeadm v1beta2, dropping those info.
	return autoConvert_v1beta2_InitConfiguration_To_upstreamv1beta2_InitConfiguration(in, out, s)
}

func Convert_v1beta2_JoinConfiguration_To_upstreamv1beta2_JoinConfiguration(in *bootstrapv1.JoinConfiguration, out *JoinConfiguration, s apimachineryconversion.Scope) error {
	// JoinConfiguration.SkipPhases, JoinConfiguration.Timeouts and Patches do not exist in kubeadm v1beta2, dropping those info.
	if err := autoConvert_v1beta2_JoinConfiguration_To_upstreamv1beta2_JoinConfiguration(in, out, s); err != nil {
		return err
	}

	if in.Timeouts != nil {
		out.Discovery.Timeout = utilconversion.ConvertFromSeconds(in.Timeouts.TLSBootstrapSeconds)
	}
	return nil
}

func Convert_v1beta2_NodeRegistrationOptions_To_upstreamv1beta2_NodeRegistrationOptions(in *bootstrapv1.NodeRegistrationOptions, out *NodeRegistrationOptions, s apimachineryconversion.Scope) error {
	// NodeRegistrationOptions.ImagePullPolicy and ImagePullSerial do not exist in kubeadm v1beta2, dropping those info.

	// Following fields require a custom conversions.
	// Note: there is a potential info loss when there are two values for the same arg but this is accepted because the kubeadm v1beta3 API does not allow this use case.
	out.KubeletExtraArgs = utilconversion.ConvertFromArgs(in.KubeletExtraArgs)
	return autoConvert_v1beta2_NodeRegistrationOptions_To_upstreamv1beta2_NodeRegistrationOptions(in, out, s)
}

func Convert_v1beta2_ControlPlaneComponent_To_upstreamv1beta2_ControlPlaneComponent(in *bootstrapv1.ControlPlaneComponent, out *ControlPlaneComponent, s apimachineryconversion.Scope) error {
	// ControlPlaneComponent.ExtraEnvs does not exist in kubeadm v1beta2, dropping this info.

	// Following fields require a custom conversions.
	// Note: there is a potential info loss when there are two values for the same arg but this is accepted because the kubeadm v1beta3 API does not allow this use case.
	out.ExtraArgs = utilconversion.ConvertFromArgs(in.ExtraArgs)
	return autoConvert_v1beta2_ControlPlaneComponent_To_upstreamv1beta2_ControlPlaneComponent(in, out, s)
}

func Convert_v1beta2_LocalEtcd_To_upstreamv1beta2_LocalEtcd(in *bootstrapv1.LocalEtcd, out *LocalEtcd, s apimachineryconversion.Scope) error {
	// LocalEtcd.ExtraEnvs does not exist in kubeadm v1beta2, dropping this info.

	// Following fields require a custom conversions.
	// Note: there is a potential info loss when there are two values for the same arg but this is accepted because the kubeadm v1beta3 API does not allow this use case.
	out.ExtraArgs = utilconversion.ConvertFromArgs(in.ExtraArgs)
	return autoConvert_v1beta2_LocalEtcd_To_upstreamv1beta2_LocalEtcd(in, out, s)
}

func Convert_v1beta2_FileDiscovery_To_upstreamv1beta2_FileDiscovery(in *bootstrapv1.FileDiscovery, out *FileDiscovery, s apimachineryconversion.Scope) error {
	// JoinConfiguration.Discovery.File.KubeConfig does not exist in kubeadm because it's internal to Cluster API, dropping those info.
	return autoConvert_v1beta2_FileDiscovery_To_upstreamv1beta2_FileDiscovery(in, out, s)
}

// Custom conversions to handle fields migrated from ClusterConfiguration to Init and JoinConfiguration in the kubeadm v1beta4 API version.

func (dst *ClusterConfiguration) ConvertFromInitConfiguration(initConfiguration *bootstrapv1.InitConfiguration) error {
	if initConfiguration == nil || initConfiguration.Timeouts == nil || initConfiguration.Timeouts.ControlPlaneComponentHealthCheckSeconds == nil {
		return nil
	}

	dst.APIServer.TimeoutForControlPlane = utilconversion.ConvertFromSeconds(initConfiguration.Timeouts.ControlPlaneComponentHealthCheckSeconds)
	return nil
}

func (src *ClusterConfiguration) ConvertToInitConfiguration(initConfiguration *bootstrapv1.InitConfiguration) error {
	if src.APIServer.TimeoutForControlPlane == nil {
		return nil
	}

	if initConfiguration == nil {
		return errors.New("cannot convert ClusterConfiguration to a nil InitConfiguration")
	}
	if initConfiguration.Timeouts == nil {
		initConfiguration.Timeouts = &bootstrapv1.Timeouts{}
	}
	initConfiguration.Timeouts.ControlPlaneComponentHealthCheckSeconds = utilconversion.ConvertToSeconds(src.APIServer.TimeoutForControlPlane)
	return nil
}
