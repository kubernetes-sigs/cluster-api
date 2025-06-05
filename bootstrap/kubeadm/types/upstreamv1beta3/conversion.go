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
	apimachineryconversion "k8s.io/apimachinery/pkg/conversion"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/upstream"
)

func (src *ClusterConfiguration) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*bootstrapv1.ClusterConfiguration)
	return Convert_upstreamv1beta3_ClusterConfiguration_To_v1beta2_ClusterConfiguration(src, dst, nil)
}

func (dst *ClusterConfiguration) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*bootstrapv1.ClusterConfiguration)
	return Convert_v1beta2_ClusterConfiguration_To_upstreamv1beta3_ClusterConfiguration(src, dst, nil)
}

func (src *InitConfiguration) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*bootstrapv1.InitConfiguration)
	return Convert_upstreamv1beta3_InitConfiguration_To_v1beta2_InitConfiguration(src, dst, nil)
}

func (dst *InitConfiguration) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*bootstrapv1.InitConfiguration)
	return Convert_v1beta2_InitConfiguration_To_upstreamv1beta3_InitConfiguration(src, dst, nil)
}

func (src *JoinConfiguration) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*bootstrapv1.JoinConfiguration)
	return Convert_upstreamv1beta3_JoinConfiguration_To_v1beta2_JoinConfiguration(src, dst, nil)
}

func (dst *JoinConfiguration) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*bootstrapv1.JoinConfiguration)
	return Convert_v1beta2_JoinConfiguration_To_upstreamv1beta3_JoinConfiguration(src, dst, nil)
}

// Custom conversion from this API, kubeadm v1beta3, to the hub version, CABPK v1beta1.

func Convert_upstreamv1beta3_ClusterConfiguration_To_v1beta2_ClusterConfiguration(in *ClusterConfiguration, out *bootstrapv1.ClusterConfiguration, s apimachineryconversion.Scope) error {
	return autoConvert_upstreamv1beta3_ClusterConfiguration_To_v1beta2_ClusterConfiguration(in, out, s)
}

func Convert_upstreamv1beta3_InitConfiguration_To_v1beta2_InitConfiguration(in *InitConfiguration, out *bootstrapv1.InitConfiguration, s apimachineryconversion.Scope) error {
	// InitConfiguration.CertificateKey does not exist in CABPK, because Cluster API does not use automatic copy certs.
	return autoConvert_upstreamv1beta3_InitConfiguration_To_v1beta2_InitConfiguration(in, out, s)
}

func Convert_upstreamv1beta3_JoinConfiguration_To_v1beta2_JoinConfiguration(in *JoinConfiguration, out *bootstrapv1.JoinConfiguration, s apimachineryconversion.Scope) error {
	if err := autoConvert_upstreamv1beta3_JoinConfiguration_To_v1beta2_JoinConfiguration(in, out, s); err != nil {
		return err
	}
	if in.Discovery.Timeout != nil {
		if out.Timeouts == nil {
			out.Timeouts = &bootstrapv1.Timeouts{}
		}
		out.Timeouts.TLSBootstrapSeconds = clusterv1.ConvertToSeconds(in.Discovery.Timeout)
	}
	return nil
}

func Convert_upstreamv1beta3_JoinControlPlane_To_v1beta2_JoinControlPlane(in *JoinControlPlane, out *bootstrapv1.JoinControlPlane, s apimachineryconversion.Scope) error {
	// JoinControlPlane.CertificateKey does not exist in CABPK, because Cluster API does not use automatic copy certs.
	return autoConvert_upstreamv1beta3_JoinControlPlane_To_v1beta2_JoinControlPlane(in, out, s)
}

func Convert_upstreamv1beta3_APIServer_To_v1beta2_APIServer(in *APIServer, out *bootstrapv1.APIServer, s apimachineryconversion.Scope) error {
	// APIServer.TimeoutForControlPlane does not exist in CABPK, because CABPK aligns to upstreamV1Beta4.
	return autoConvert_upstreamv1beta3_APIServer_To_v1beta2_APIServer(in, out, s)
}

func Convert_upstreamv1beta3_Discovery_To_v1beta2_Discovery(in *Discovery, out *bootstrapv1.Discovery, s apimachineryconversion.Scope) error {
	// Discovery.Timeout does not exist in CABPK, because CABPK aligns to upstreamV1Beta4.
	return autoConvert_upstreamv1beta3_Discovery_To_v1beta2_Discovery(in, out, s)
}

func Convert_upstreamv1beta3_ControlPlaneComponent_To_v1beta2_ControlPlaneComponent(in *ControlPlaneComponent, out *bootstrapv1.ControlPlaneComponent, s apimachineryconversion.Scope) error {
	out.ExtraArgs = bootstrapv1.ConvertToArgs(in.ExtraArgs)
	return autoConvert_upstreamv1beta3_ControlPlaneComponent_To_v1beta2_ControlPlaneComponent(in, out, s)
}

func Convert_upstreamv1beta3_LocalEtcd_To_v1beta2_LocalEtcd(in *LocalEtcd, out *bootstrapv1.LocalEtcd, s apimachineryconversion.Scope) error {
	out.ExtraArgs = bootstrapv1.ConvertToArgs(in.ExtraArgs)
	return autoConvert_upstreamv1beta3_LocalEtcd_To_v1beta2_LocalEtcd(in, out, s)
}

func Convert_upstreamv1beta3_NodeRegistrationOptions_To_v1beta2_NodeRegistrationOptions(in *NodeRegistrationOptions, out *bootstrapv1.NodeRegistrationOptions, s apimachineryconversion.Scope) error {
	out.KubeletExtraArgs = bootstrapv1.ConvertToArgs(in.KubeletExtraArgs)
	return autoConvert_upstreamv1beta3_NodeRegistrationOptions_To_v1beta2_NodeRegistrationOptions(in, out, s)
}

func Convert_upstreamv1beta3_BootstrapToken_To_v1beta2_BootstrapToken(in *BootstrapToken, out *bootstrapv1.BootstrapToken, s apimachineryconversion.Scope) error {
	if err := autoConvert_upstreamv1beta3_BootstrapToken_To_v1beta2_BootstrapToken(in, out, s); err != nil {
		return err
	}
	out.TTLSeconds = clusterv1.ConvertToSeconds(in.TTL)
	return nil
}

// Custom conversion from the hub version, CABPK v1beta1, to this API, kubeadm v1beta3.

func Convert_v1beta2_InitConfiguration_To_upstreamv1beta3_InitConfiguration(in *bootstrapv1.InitConfiguration, out *InitConfiguration, s apimachineryconversion.Scope) error {
	// InitConfiguration.Timeouts does not exist in kubeadm v1beta3, dropping this info.
	return autoConvert_v1beta2_InitConfiguration_To_upstreamv1beta3_InitConfiguration(in, out, s)
}

func Convert_v1beta2_JoinConfiguration_To_upstreamv1beta3_JoinConfiguration(in *bootstrapv1.JoinConfiguration, out *JoinConfiguration, s apimachineryconversion.Scope) error {
	// JoinConfiguration.Timeouts does not exist in kubeadm v1beta3, dropping this info.
	if err := autoConvert_v1beta2_JoinConfiguration_To_upstreamv1beta3_JoinConfiguration(in, out, s); err != nil {
		return err
	}

	if in.Timeouts != nil {
		out.Discovery.Timeout = clusterv1.ConvertFromSeconds(in.Timeouts.TLSBootstrapSeconds)
	}
	return nil
}

func Convert_v1beta2_FileDiscovery_To_upstreamv1beta3_FileDiscovery(in *bootstrapv1.FileDiscovery, out *FileDiscovery, s apimachineryconversion.Scope) error {
	// JoinConfiguration.Discovery.File.KubeConfig does not exist in kubeadm because it's internal to Cluster API, dropping those info.
	return autoConvert_v1beta2_FileDiscovery_To_upstreamv1beta3_FileDiscovery(in, out, s)
}

func Convert_v1beta2_ControlPlaneComponent_To_upstreamv1beta3_ControlPlaneComponent(in *bootstrapv1.ControlPlaneComponent, out *ControlPlaneComponent, s apimachineryconversion.Scope) error {
	// ControlPlaneComponent.ExtraEnvs does not exist in kubeadm v1beta3, dropping this info.

	// Following fields require a custom conversions.
	// Note: there is a potential info loss when there are two values for the same arg but this is accepted because the kubeadm v1beta3 API does not allow this use case.
	out.ExtraArgs = bootstrapv1.ConvertFromArgs(in.ExtraArgs)
	return autoConvert_v1beta2_ControlPlaneComponent_To_upstreamv1beta3_ControlPlaneComponent(in, out, s)
}

func Convert_v1beta2_LocalEtcd_To_upstreamv1beta3_LocalEtcd(in *bootstrapv1.LocalEtcd, out *LocalEtcd, s apimachineryconversion.Scope) error {
	// LocalEtcd.ExtraEnvs does not exist in kubeadm v1beta3, dropping this info.

	// Following fields require a custom conversions.
	// Note: there is a potential info loss when there are two values for the same arg but this is accepted because the kubeadm v1beta3 API does not allow this use case.
	out.ExtraArgs = bootstrapv1.ConvertFromArgs(in.ExtraArgs)
	return autoConvert_v1beta2_LocalEtcd_To_upstreamv1beta3_LocalEtcd(in, out, s)
}

func Convert_v1beta2_NodeRegistrationOptions_To_upstreamv1beta3_NodeRegistrationOptions(in *bootstrapv1.NodeRegistrationOptions, out *NodeRegistrationOptions, s apimachineryconversion.Scope) error {
	// NodeRegistrationOptions.ImagePullSerial does not exist in kubeadm v1beta3, dropping this info.

	// Following fields require a custom conversions.
	// Note: there is a potential info loss when there are two values for the same arg but this is accepted because the kubeadm v1beta3 API does not allow this use case.
	out.KubeletExtraArgs = bootstrapv1.ConvertFromArgs(in.KubeletExtraArgs)
	return autoConvert_v1beta2_NodeRegistrationOptions_To_upstreamv1beta3_NodeRegistrationOptions(in, out, s)
}

func Convert_v1beta2_BootstrapToken_To_upstreamv1beta3_BootstrapToken(in *bootstrapv1.BootstrapToken, out *BootstrapToken, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_BootstrapToken_To_upstreamv1beta3_BootstrapToken(in, out, s); err != nil {
		return err
	}
	out.TTL = clusterv1.ConvertFromSeconds(in.TTLSeconds)
	return nil
}

// Func to allow handling fields that only exist in upstream types.

var _ upstream.AdditionalDataSetter = &ClusterConfiguration{}

func (src *ClusterConfiguration) SetAdditionalData(data upstream.AdditionalData) {
	if src == nil {
		return
	}

	if data.KubernetesVersion != nil {
		src.KubernetesVersion = *data.KubernetesVersion
	}
	if data.ClusterName != nil {
		src.ClusterName = *data.ClusterName
	}
	if data.ControlPlaneEndpoint != nil {
		src.ControlPlaneEndpoint = *data.ControlPlaneEndpoint
	}
	if data.DNSDomain != nil {
		src.Networking.DNSDomain = *data.DNSDomain
	}
	if data.ServiceSubnet != nil {
		src.Networking.ServiceSubnet = *data.ServiceSubnet
	}
	if data.PodSubnet != nil {
		src.Networking.PodSubnet = *data.PodSubnet
	}
	if data.ControlPlaneComponentHealthCheckSeconds != nil {
		src.APIServer.TimeoutForControlPlane = clusterv1.ConvertFromSeconds(data.ControlPlaneComponentHealthCheckSeconds)
	}
}

var _ upstream.AdditionalDataGetter = &ClusterConfiguration{}

func (src *ClusterConfiguration) GetAdditionalData(data *upstream.AdditionalData) {
	if src == nil {
		return
	}
	if data == nil {
		return
	}

	if src.KubernetesVersion != "" {
		data.KubernetesVersion = ptr.To(src.KubernetesVersion)
	}
	if src.ClusterName != "" {
		data.ClusterName = ptr.To(src.ClusterName)
	}
	if src.ControlPlaneEndpoint != "" {
		data.ControlPlaneEndpoint = ptr.To(src.ControlPlaneEndpoint)
	}
	if src.Networking.DNSDomain != "" {
		data.DNSDomain = ptr.To(src.Networking.DNSDomain)
	}
	if src.Networking.ServiceSubnet != "" {
		data.ServiceSubnet = ptr.To(src.Networking.ServiceSubnet)
	}
	if src.Networking.PodSubnet != "" {
		data.PodSubnet = ptr.To(src.Networking.PodSubnet)
	}
	if src.APIServer.TimeoutForControlPlane != nil {
		data.ControlPlaneComponentHealthCheckSeconds = clusterv1.ConvertToSeconds(src.APIServer.TimeoutForControlPlane)
	}
}
