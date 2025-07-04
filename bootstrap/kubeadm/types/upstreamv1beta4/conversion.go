/*
Copyright 2024 The Kubernetes Authors.

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
	apimachineryconversion "k8s.io/apimachinery/pkg/conversion"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/upstream"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/upstreamhub"
)

func (src *ClusterConfiguration) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*upstreamhub.ClusterConfiguration)
	return Convert_upstreamv1beta4_ClusterConfiguration_To_v1beta2_ClusterConfiguration(src, &dst.ClusterConfiguration, nil)
}

func (dst *ClusterConfiguration) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*upstreamhub.ClusterConfiguration)
	return Convert_v1beta2_ClusterConfiguration_To_upstreamv1beta4_ClusterConfiguration(&src.ClusterConfiguration, dst, nil)
}

func (src *InitConfiguration) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*upstreamhub.InitConfiguration)
	return Convert_upstreamv1beta4_InitConfiguration_To_v1beta2_InitConfiguration(src, &dst.InitConfiguration, nil)
}

func (dst *InitConfiguration) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*upstreamhub.InitConfiguration)
	return Convert_v1beta2_InitConfiguration_To_upstreamv1beta4_InitConfiguration(&src.InitConfiguration, dst, nil)
}

func (src *JoinConfiguration) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*upstreamhub.JoinConfiguration)
	return Convert_upstreamv1beta4_JoinConfiguration_To_v1beta2_JoinConfiguration(src, &dst.JoinConfiguration, nil)
}

func (dst *JoinConfiguration) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*upstreamhub.JoinConfiguration)
	return Convert_v1beta2_JoinConfiguration_To_upstreamv1beta4_JoinConfiguration(&src.JoinConfiguration, dst, nil)
}

// Custom conversion from this API, kubeadm v1beta4, to the hub version, CABPK v1beta1.

func Convert_upstreamv1beta4_ClusterConfiguration_To_v1beta2_ClusterConfiguration(in *ClusterConfiguration, out *bootstrapv1.ClusterConfiguration, s apimachineryconversion.Scope) error {
	// Following fields do not exist in CABPK v1beta1 version:
	// - Proxy (Not supported yet)
	// - EncryptionAlgorithm (Not supported yet)
	// - CertificateValidityPeriod (Not supported yet)
	// - CACertificateValidityPeriod (Not supported yet)
	return autoConvert_upstreamv1beta4_ClusterConfiguration_To_v1beta2_ClusterConfiguration(in, out, s)
}

func Convert_upstreamv1beta4_DNS_To_v1beta2_DNS(in *DNS, out *bootstrapv1.DNS, s apimachineryconversion.Scope) error {
	// Following fields do not exist in CABPK v1beta1 version:
	// - Disabled (Not supported yet)
	return autoConvert_upstreamv1beta4_DNS_To_v1beta2_DNS(in, out, s)
}

func Convert_upstreamv1beta4_InitConfiguration_To_v1beta2_InitConfiguration(in *InitConfiguration, out *bootstrapv1.InitConfiguration, s apimachineryconversion.Scope) error {
	// Following fields do not exist in CABPK v1beta1 version:
	// - DryRun (Does not make sense for CAPBK)
	// - CertificateKey (CABPK does not use automatic copy certs)
	// - Timeouts (Not supported yet)
	return autoConvert_upstreamv1beta4_InitConfiguration_To_v1beta2_InitConfiguration(in, out, s)
}

func Convert_upstreamv1beta4_JoinConfiguration_To_v1beta2_JoinConfiguration(in *JoinConfiguration, out *bootstrapv1.JoinConfiguration, s apimachineryconversion.Scope) error {
	// Following fields do not exist in CABPK v1beta1 version:
	// - DryRun (Does not make sense for CAPBK)
	return autoConvert_upstreamv1beta4_JoinConfiguration_To_v1beta2_JoinConfiguration(in, out, s)
}

func Convert_upstreamv1beta4_JoinControlPlane_To_v1beta2_JoinControlPlane(in *JoinControlPlane, out *bootstrapv1.JoinControlPlane, s apimachineryconversion.Scope) error {
	// Following fields do not exist in CABPK v1beta1 version:
	// - CertificateKey (CABPK does not use automatic copy certs)
	return autoConvert_upstreamv1beta4_JoinControlPlane_To_v1beta2_JoinControlPlane(in, out, s)
}

func Convert_upstreamv1beta4_Timeouts_To_v1beta2_Timeouts(in *Timeouts, out *bootstrapv1.Timeouts, s apimachineryconversion.Scope) error {
	if err := autoConvert_upstreamv1beta4_Timeouts_To_v1beta2_Timeouts(in, out, s); err != nil {
		return err
	}

	out.ControlPlaneComponentHealthCheckSeconds = clusterv1.ConvertToSeconds(in.ControlPlaneComponentHealthCheck)
	out.KubeletHealthCheckSeconds = clusterv1.ConvertToSeconds(in.KubeletHealthCheck)
	out.KubernetesAPICallSeconds = clusterv1.ConvertToSeconds(in.KubernetesAPICall)
	out.DiscoverySeconds = clusterv1.ConvertToSeconds(in.Discovery)
	out.EtcdAPICallSeconds = clusterv1.ConvertToSeconds(in.EtcdAPICall)
	out.TLSBootstrapSeconds = clusterv1.ConvertToSeconds(in.TLSBootstrap)
	return nil
}

func Convert_upstreamv1beta4_BootstrapToken_To_v1beta2_BootstrapToken(in *BootstrapToken, out *bootstrapv1.BootstrapToken, s apimachineryconversion.Scope) error {
	if err := autoConvert_upstreamv1beta4_BootstrapToken_To_v1beta2_BootstrapToken(in, out, s); err != nil {
		return err
	}
	out.TTLSeconds = clusterv1.ConvertToSeconds(in.TTL)
	return nil
}

// Custom conversion from the hub version, CABPK v1beta1, to this API, kubeadm v1beta4.

func Convert_v1beta2_APIServer_To_upstreamv1beta4_APIServer(in *bootstrapv1.APIServer, out *APIServer, s apimachineryconversion.Scope) error {
	// Following fields do not exist in kubeadm v1beta4 version:
	// - TimeoutForControlPlane (this field has been migrated to Init/JoinConfiguration; migration is handled by ConvertFromClusterConfiguration custom converters.
	return autoConvert_v1beta2_APIServer_To_upstreamv1beta4_APIServer(in, out, s)
}

func Convert_v1beta2_ClusterConfiguration_To_upstreamv1beta4_ClusterConfiguration(in *bootstrapv1.ClusterConfiguration, out *ClusterConfiguration, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_ClusterConfiguration_To_upstreamv1beta4_ClusterConfiguration(in, out, s); err != nil {
		return err
	}

	if in.Proxy != nil {
		return Convert_v1beta2_Proxy_To_upstreamv1beta4_Proxy(in.Proxy, &out.Proxy, s)
	}
	return nil
}

func Convert_v1beta2_Discovery_To_upstreamv1beta4_Discovery(in *bootstrapv1.Discovery, out *Discovery, s apimachineryconversion.Scope) error {
	// Following fields do not exist in kubeadm v1beta4 version:
	// - Timeout (this field has been migrated to JoinConfiguration.Timeouts.TLSBootstrap, the conversion is handled in Convert_v1beta2_JoinConfiguration_To_upstreamv1beta4_JoinConfiguration)
	return autoConvert_v1beta2_Discovery_To_upstreamv1beta4_Discovery(in, out, s)
}

func Convert_v1beta2_FileDiscovery_To_upstreamv1beta4_FileDiscovery(in *bootstrapv1.FileDiscovery, out *FileDiscovery, s apimachineryconversion.Scope) error {
	// JoinConfiguration.Discovery.File.KubeConfig does not exist in kubeadm because it's internal to Cluster API, dropping those info.
	return autoConvert_v1beta2_FileDiscovery_To_upstreamv1beta4_FileDiscovery(in, out, s)
}

func Convert_v1beta2_Timeouts_To_upstreamv1beta4_Timeouts(in *bootstrapv1.Timeouts, out *Timeouts, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_Timeouts_To_upstreamv1beta4_Timeouts(in, out, s); err != nil {
		return err
	}

	out.ControlPlaneComponentHealthCheck = clusterv1.ConvertFromSeconds(in.ControlPlaneComponentHealthCheckSeconds)
	out.KubeletHealthCheck = clusterv1.ConvertFromSeconds(in.KubeletHealthCheckSeconds)
	out.KubernetesAPICall = clusterv1.ConvertFromSeconds(in.KubernetesAPICallSeconds)
	out.EtcdAPICall = clusterv1.ConvertFromSeconds(in.EtcdAPICallSeconds)
	out.TLSBootstrap = clusterv1.ConvertFromSeconds(in.TLSBootstrapSeconds)
	out.Discovery = clusterv1.ConvertFromSeconds(in.DiscoverySeconds)
	return nil
}

func Convert_v1beta2_BootstrapToken_To_upstreamv1beta4_BootstrapToken(in *bootstrapv1.BootstrapToken, out *BootstrapToken, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_BootstrapToken_To_upstreamv1beta4_BootstrapToken(in, out, s); err != nil {
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
	if data.DNSDomain != nil {
		src.Networking.DNSDomain = *data.DNSDomain
	}
	if data.ServiceSubnet != nil {
		src.Networking.ServiceSubnet = *data.ServiceSubnet
	}
	if data.PodSubnet != nil {
		src.Networking.PodSubnet = *data.PodSubnet
	}

	// NOTE: for kubeadm v1beta4 types we are not setting APIServer.TimeoutForControlPlane because it doesn't exist
	// (the timeout is configured in InitConfiguration / JoinConfiguration instead).
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
	if src.Networking.DNSDomain != "" {
		data.DNSDomain = ptr.To(src.Networking.DNSDomain)
	}
	if src.Networking.ServiceSubnet != "" {
		data.ServiceSubnet = ptr.To(src.Networking.ServiceSubnet)
	}
	if src.Networking.PodSubnet != "" {
		data.PodSubnet = ptr.To(src.Networking.PodSubnet)
	}

	// NOTE: for kubeadm v1beta4 types we are not reading ControlPlaneComponentHealthCheckSeconds into additional data
	// because Cluster API types are aligned with kubeadm's v1beta4 API version.
}
