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
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/upstream"
)

func (src *ClusterConfiguration) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*bootstrapv1.ClusterConfiguration)
	return Convert_upstreamv1beta4_ClusterConfiguration_To_v1beta2_ClusterConfiguration(src, dst, nil)
}

func (dst *ClusterConfiguration) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*bootstrapv1.ClusterConfiguration)
	return Convert_v1beta2_ClusterConfiguration_To_upstreamv1beta4_ClusterConfiguration(src, dst, nil)
}

func (src *InitConfiguration) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*bootstrapv1.InitConfiguration)
	return Convert_upstreamv1beta4_InitConfiguration_To_v1beta2_InitConfiguration(src, dst, nil)
}

func (dst *InitConfiguration) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*bootstrapv1.InitConfiguration)
	return Convert_v1beta2_InitConfiguration_To_upstreamv1beta4_InitConfiguration(src, dst, nil)
}

func (src *JoinConfiguration) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*bootstrapv1.JoinConfiguration)
	return Convert_upstreamv1beta4_JoinConfiguration_To_v1beta2_JoinConfiguration(src, dst, nil)
}

func (dst *JoinConfiguration) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*bootstrapv1.JoinConfiguration)
	return Convert_v1beta2_JoinConfiguration_To_upstreamv1beta4_JoinConfiguration(src, dst, nil)
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

	out.ControlPlaneComponentHealthCheckSeconds = bootstrapv1.ConvertToSeconds(in.ControlPlaneComponentHealthCheck)
	out.KubeletHealthCheckSeconds = bootstrapv1.ConvertToSeconds(in.KubeletHealthCheck)
	out.KubernetesAPICallSeconds = bootstrapv1.ConvertToSeconds(in.KubernetesAPICall)
	out.DiscoverySeconds = bootstrapv1.ConvertToSeconds(in.Discovery)
	out.EtcdAPICallSeconds = bootstrapv1.ConvertToSeconds(in.EtcdAPICall)
	out.TLSBootstrapSeconds = bootstrapv1.ConvertToSeconds(in.TLSBootstrap)
	return nil
}

// Custom conversion from the hub version, CABPK v1beta1, to this API, kubeadm v1beta4.

func Convert_v1beta2_APIServer_To_upstreamv1beta4_APIServer(in *bootstrapv1.APIServer, out *APIServer, s apimachineryconversion.Scope) error {
	// Following fields do not exist in kubeadm v1beta4 version:
	// - TimeoutForControlPlane (this field has been migrated to Init/JoinConfiguration; migration is handled by ConvertFromClusterConfiguration custom converters.
	return autoConvert_v1beta2_APIServer_To_upstreamv1beta4_APIServer(in, out, s)
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

	out.ControlPlaneComponentHealthCheck = bootstrapv1.ConvertFromSeconds(in.ControlPlaneComponentHealthCheckSeconds)
	out.KubeletHealthCheck = bootstrapv1.ConvertFromSeconds(in.KubeletHealthCheckSeconds)
	out.KubernetesAPICall = bootstrapv1.ConvertFromSeconds(in.KubernetesAPICallSeconds)
	out.EtcdAPICall = bootstrapv1.ConvertFromSeconds(in.EtcdAPICallSeconds)
	out.TLSBootstrap = bootstrapv1.ConvertFromSeconds(in.TLSBootstrapSeconds)
	out.Discovery = bootstrapv1.ConvertFromSeconds(in.DiscoverySeconds)
	return nil
}

// Func to allow handling fields that only exist in upstream types.

var _ upstream.DataSetter = &ClusterConfiguration{}

func (src *ClusterConfiguration) SetUpstreamData(data upstream.Data) {
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
}

var _ upstream.DataGetter = &ClusterConfiguration{}

func (src *ClusterConfiguration) GetUpstreamData() upstream.Data {
	var data upstream.Data

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

	return data
}
