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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	return Convert_upstreamv1beta3_ClusterConfiguration_To_v1beta2_ClusterConfiguration(src, &dst.ClusterConfiguration, nil)
}

func (dst *ClusterConfiguration) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*upstreamhub.ClusterConfiguration)
	return Convert_v1beta2_ClusterConfiguration_To_upstreamv1beta3_ClusterConfiguration(&src.ClusterConfiguration, dst, nil)
}

func (src *InitConfiguration) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*upstreamhub.InitConfiguration)
	return Convert_upstreamv1beta3_InitConfiguration_To_v1beta2_InitConfiguration(src, &dst.InitConfiguration, nil)
}

func (dst *InitConfiguration) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*upstreamhub.InitConfiguration)
	return Convert_v1beta2_InitConfiguration_To_upstreamv1beta3_InitConfiguration(&src.InitConfiguration, dst, nil)
}

func (src *JoinConfiguration) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*upstreamhub.JoinConfiguration)
	return Convert_upstreamv1beta3_JoinConfiguration_To_v1beta2_JoinConfiguration(src, &dst.JoinConfiguration, nil)
}

func (dst *JoinConfiguration) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*upstreamhub.JoinConfiguration)
	return Convert_v1beta2_JoinConfiguration_To_upstreamv1beta3_JoinConfiguration(&src.JoinConfiguration, dst, nil)
}

// Custom conversion from this API, kubeadm v1beta3, to the hub version, CABPK v1beta1.

func Convert_upstreamv1beta3_ClusterConfiguration_To_v1beta2_ClusterConfiguration(in *ClusterConfiguration, out *bootstrapv1.ClusterConfiguration, s apimachineryconversion.Scope) error {
	return autoConvert_upstreamv1beta3_ClusterConfiguration_To_v1beta2_ClusterConfiguration(in, out, s)
}

func Convert_upstreamv1beta3_InitConfiguration_To_v1beta2_InitConfiguration(in *InitConfiguration, out *bootstrapv1.InitConfiguration, s apimachineryconversion.Scope) error {
	// InitConfiguration.CertificateKey does not exist in CABPK, because Cluster API does not use automatic copy certs.
	if err := autoConvert_upstreamv1beta3_InitConfiguration_To_v1beta2_InitConfiguration(in, out, s); err != nil {
		return err
	}
	if in.Patches != nil {
		if err := Convert_upstreamv1beta3_Patches_To_v1beta2_Patches(in.Patches, &out.Patches, s); err != nil {
			return err
		}
	}
	return nil
}

func Convert_upstreamv1beta3_JoinConfiguration_To_v1beta2_JoinConfiguration(in *JoinConfiguration, out *bootstrapv1.JoinConfiguration, s apimachineryconversion.Scope) error {
	if err := autoConvert_upstreamv1beta3_JoinConfiguration_To_v1beta2_JoinConfiguration(in, out, s); err != nil {
		return err
	}
	if in.Patches != nil {
		if err := Convert_upstreamv1beta3_Patches_To_v1beta2_Patches(in.Patches, &out.Patches, s); err != nil {
			return err
		}
	}
	if in.Discovery.Timeout != nil {
		out.Timeouts.TLSBootstrapSeconds = clusterv1.ConvertToSeconds(in.Discovery.Timeout)
	}
	return nil
}

func Convert_upstreamv1beta3_JoinControlPlane_To_v1beta2_JoinControlPlane(in *JoinControlPlane, out *bootstrapv1.JoinControlPlane, s apimachineryconversion.Scope) error {
	// JoinControlPlane.CertificateKey does not exist in CABPK, because Cluster API does not use automatic copy certs.
	return autoConvert_upstreamv1beta3_JoinControlPlane_To_v1beta2_JoinControlPlane(in, out, s)
}

func Convert_v1beta2_APIServer_To_upstreamv1beta3_APIServer(in *bootstrapv1.APIServer, out *APIServer, s apimachineryconversion.Scope) error {
	// APIServer.TimeoutForControlPlane does not exist in CABPK, because CABPK aligns to upstreamV1Beta4.
	// Note: there is a potential info loss when there are two values for the same arg but this is accepted because the kubeadm v1beta3 API does not allow this use case.
	out.ExtraArgs = bootstrapv1.ConvertFromArgs(in.ExtraArgs)
	if err := convert_v1beta2_ExtraVolumes_To_upstreamv1beta3_ExtraVolumes(&in.ExtraVolumes, &out.ExtraVolumes, s); err != nil {
		return err
	}
	return autoConvert_v1beta2_APIServer_To_upstreamv1beta3_APIServer(in, out, s)
}

func Convert_v1beta2_ControllerManager_To_upstreamv1beta3_ControlPlaneComponent(in *bootstrapv1.ControllerManager, out *ControlPlaneComponent, s apimachineryconversion.Scope) error {
	// Following fields require a custom conversions.
	// Note: there is a potential info loss when there are two values for the same arg but this is accepted because the kubeadm v1beta3 API does not allow this use case.
	out.ExtraArgs = bootstrapv1.ConvertFromArgs(in.ExtraArgs)
	return convert_v1beta2_ExtraVolumes_To_upstreamv1beta3_ExtraVolumes(&in.ExtraVolumes, &out.ExtraVolumes, s)
}

func Convert_v1beta2_Scheduler_To_upstreamv1beta3_ControlPlaneComponent(in *bootstrapv1.Scheduler, out *ControlPlaneComponent, s apimachineryconversion.Scope) error {
	// Following fields require a custom conversions.
	// Note: there is a potential info loss when there are two values for the same arg but this is accepted because the kubeadm v1beta3 API does not allow this use case.
	out.ExtraArgs = bootstrapv1.ConvertFromArgs(in.ExtraArgs)
	return convert_v1beta2_ExtraVolumes_To_upstreamv1beta3_ExtraVolumes(&in.ExtraVolumes, &out.ExtraVolumes, s)
}

func convert_v1beta2_ExtraVolumes_To_upstreamv1beta3_ExtraVolumes(in *[]bootstrapv1.HostPathMount, out *[]HostPathMount, s apimachineryconversion.Scope) error {
	if in != nil && len(*in) > 0 {
		*out = make([]HostPathMount, len(*in))
		for i := range *in {
			if err := Convert_v1beta2_HostPathMount_To_upstreamv1beta3_HostPathMount(&(*in)[i], &(*out)[i], s); err != nil {
				return err
			}
		}
	} else {
		*out = nil
	}
	return nil
}

func Convert_upstreamv1beta3_Discovery_To_v1beta2_Discovery(in *Discovery, out *bootstrapv1.Discovery, s apimachineryconversion.Scope) error {
	// Discovery.Timeout does not exist in CABPK, because CABPK aligns to upstreamV1Beta4.
	if err := autoConvert_upstreamv1beta3_Discovery_To_v1beta2_Discovery(in, out, s); err != nil {
		return err
	}
	if in.BootstrapToken != nil {
		if err := Convert_upstreamv1beta3_BootstrapTokenDiscovery_To_v1beta2_BootstrapTokenDiscovery(in.BootstrapToken, &out.BootstrapToken, s); err != nil {
			return err
		}
	}
	if in.File != nil {
		if err := Convert_upstreamv1beta3_FileDiscovery_To_v1beta2_FileDiscovery(in.File, &out.File, s); err != nil {
			return err
		}
	}
	return nil
}

func Convert_upstreamv1beta3_APIServer_To_v1beta2_APIServer(in *APIServer, out *bootstrapv1.APIServer, s apimachineryconversion.Scope) error {
	// TimeoutForControlPlane has been removed in v1beta2
	out.ExtraArgs = bootstrapv1.ConvertToArgs(in.ExtraArgs)
	if err := convert_upstreamv1beta3_ExtraVolumes_To_v1beta2_ExtraVolumes(&in.ExtraVolumes, &out.ExtraVolumes, s); err != nil {
		return err
	}
	return autoConvert_upstreamv1beta3_APIServer_To_v1beta2_APIServer(in, out, s)
}

func Convert_upstreamv1beta3_ControlPlaneComponent_To_v1beta2_ControllerManager(in *ControlPlaneComponent, out *bootstrapv1.ControllerManager, s apimachineryconversion.Scope) error {
	out.ExtraArgs = bootstrapv1.ConvertToArgs(in.ExtraArgs)
	return convert_upstreamv1beta3_ExtraVolumes_To_v1beta2_ExtraVolumes(&in.ExtraVolumes, &out.ExtraVolumes, s)
}

func Convert_upstreamv1beta3_ControlPlaneComponent_To_v1beta2_Scheduler(in *ControlPlaneComponent, out *bootstrapv1.Scheduler, s apimachineryconversion.Scope) error {
	out.ExtraArgs = bootstrapv1.ConvertToArgs(in.ExtraArgs)
	return convert_upstreamv1beta3_ExtraVolumes_To_v1beta2_ExtraVolumes(&in.ExtraVolumes, &out.ExtraVolumes, s)
}

func convert_upstreamv1beta3_ExtraVolumes_To_v1beta2_ExtraVolumes(in *[]HostPathMount, out *[]bootstrapv1.HostPathMount, s apimachineryconversion.Scope) error {
	if in != nil && len(*in) > 0 {
		*out = make([]bootstrapv1.HostPathMount, len(*in))
		for i := range *in {
			if err := Convert_upstreamv1beta3_HostPathMount_To_v1beta2_HostPathMount(&(*in)[i], &(*out)[i], s); err != nil {
				return err
			}
		}
	} else {
		*out = nil
	}
	return nil
}

func Convert_upstreamv1beta3_LocalEtcd_To_v1beta2_LocalEtcd(in *LocalEtcd, out *bootstrapv1.LocalEtcd, s apimachineryconversion.Scope) error {
	out.ExtraArgs = bootstrapv1.ConvertToArgs(in.ExtraArgs)
	out.ImageRepository = in.ImageRepository
	out.ImageTag = in.ImageTag
	return autoConvert_upstreamv1beta3_LocalEtcd_To_v1beta2_LocalEtcd(in, out, s)
}

func Convert_upstreamv1beta3_NodeRegistrationOptions_To_v1beta2_NodeRegistrationOptions(in *NodeRegistrationOptions, out *bootstrapv1.NodeRegistrationOptions, s apimachineryconversion.Scope) error {
	out.KubeletExtraArgs = bootstrapv1.ConvertToArgs(in.KubeletExtraArgs)
	if in.Taints == nil {
		out.Taints = nil
	} else {
		out.Taints = ptr.To(in.Taints)
	}
	return autoConvert_upstreamv1beta3_NodeRegistrationOptions_To_v1beta2_NodeRegistrationOptions(in, out, s)
}

func Convert_upstreamv1beta3_BootstrapToken_To_v1beta2_BootstrapToken(in *BootstrapToken, out *bootstrapv1.BootstrapToken, s apimachineryconversion.Scope) error {
	if err := autoConvert_upstreamv1beta3_BootstrapToken_To_v1beta2_BootstrapToken(in, out, s); err != nil {
		return err
	}
	out.TTLSeconds = clusterv1.ConvertToSeconds(in.TTL)
	if in.Expires != nil && !reflect.DeepEqual(in.Expires, &metav1.Time{}) {
		out.Expires = *in.Expires
	}
	return nil
}

func Convert_upstreamv1beta3_DNS_To_v1beta2_DNS(in *DNS, out *bootstrapv1.DNS, _ apimachineryconversion.Scope) error {
	out.ImageRepository = in.ImageRepository
	out.ImageTag = in.ImageTag
	return nil
}

func Convert_upstreamv1beta3_Etcd_To_v1beta2_Etcd(in *Etcd, out *bootstrapv1.Etcd, s apimachineryconversion.Scope) error {
	if in.Local != nil {
		if err := Convert_upstreamv1beta3_LocalEtcd_To_v1beta2_LocalEtcd(in.Local, &out.Local, s); err != nil {
			return err
		}
	}
	if in.External != nil {
		if err := Convert_upstreamv1beta3_ExternalEtcd_To_v1beta2_ExternalEtcd(in.External, &out.External, s); err != nil {
			return err
		}
	}
	return nil
}

// Custom conversion from the hub version, CABPK v1beta1, to this API, kubeadm v1beta3.

func Convert_v1beta2_InitConfiguration_To_upstreamv1beta3_InitConfiguration(in *bootstrapv1.InitConfiguration, out *InitConfiguration, s apimachineryconversion.Scope) error {
	// InitConfiguration.Timeouts does not exist in kubeadm v1beta3, dropping this info.
	if err := autoConvert_v1beta2_InitConfiguration_To_upstreamv1beta3_InitConfiguration(in, out, s); err != nil {
		return err
	}
	if !reflect.DeepEqual(in.Patches, bootstrapv1.Patches{}) {
		out.Patches = &Patches{}
		if err := Convert_v1beta2_Patches_To_upstreamv1beta3_Patches(&in.Patches, out.Patches, s); err != nil {
			return err
		}
	}
	return nil
}

func Convert_v1beta2_JoinConfiguration_To_upstreamv1beta3_JoinConfiguration(in *bootstrapv1.JoinConfiguration, out *JoinConfiguration, s apimachineryconversion.Scope) error {
	// JoinConfiguration.Timeouts does not exist in kubeadm v1beta3, dropping this info.
	if err := autoConvert_v1beta2_JoinConfiguration_To_upstreamv1beta3_JoinConfiguration(in, out, s); err != nil {
		return err
	}
	if !reflect.DeepEqual(in.Patches, bootstrapv1.Patches{}) {
		out.Patches = &Patches{}
		if err := Convert_v1beta2_Patches_To_upstreamv1beta3_Patches(&in.Patches, out.Patches, s); err != nil {
			return err
		}
	}

	out.Discovery.Timeout = clusterv1.ConvertFromSeconds(in.Timeouts.TLSBootstrapSeconds)
	return nil
}

func Convert_v1beta2_FileDiscovery_To_upstreamv1beta3_FileDiscovery(in *bootstrapv1.FileDiscovery, out *FileDiscovery, s apimachineryconversion.Scope) error {
	// JoinConfiguration.Discovery.File.KubeConfig does not exist in kubeadm because it's internal to Cluster API, dropping those info.
	return autoConvert_v1beta2_FileDiscovery_To_upstreamv1beta3_FileDiscovery(in, out, s)
}

func Convert_v1beta2_LocalEtcd_To_upstreamv1beta3_LocalEtcd(in *bootstrapv1.LocalEtcd, out *LocalEtcd, s apimachineryconversion.Scope) error {
	// LocalEtcd.ExtraEnvs does not exist in kubeadm v1beta3, dropping this info.

	// Following fields require a custom conversions.
	// Note: there is a potential info loss when there are two values for the same arg but this is accepted because the kubeadm v1beta3 API does not allow this use case.
	out.ExtraArgs = bootstrapv1.ConvertFromArgs(in.ExtraArgs)
	out.ImageRepository = in.ImageRepository
	out.ImageTag = in.ImageTag
	return autoConvert_v1beta2_LocalEtcd_To_upstreamv1beta3_LocalEtcd(in, out, s)
}

func Convert_v1beta2_NodeRegistrationOptions_To_upstreamv1beta3_NodeRegistrationOptions(in *bootstrapv1.NodeRegistrationOptions, out *NodeRegistrationOptions, s apimachineryconversion.Scope) error {
	// NodeRegistrationOptions.ImagePullSerial does not exist in kubeadm v1beta3, dropping this info.

	// Following fields require a custom conversions.
	// Note: there is a potential info loss when there are two values for the same arg but this is accepted because the kubeadm v1beta3 API does not allow this use case.
	out.KubeletExtraArgs = bootstrapv1.ConvertFromArgs(in.KubeletExtraArgs)
	if in.Taints == nil {
		out.Taints = nil
	} else {
		out.Taints = *in.Taints
	}
	return autoConvert_v1beta2_NodeRegistrationOptions_To_upstreamv1beta3_NodeRegistrationOptions(in, out, s)
}

func Convert_v1beta2_BootstrapToken_To_upstreamv1beta3_BootstrapToken(in *bootstrapv1.BootstrapToken, out *BootstrapToken, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_BootstrapToken_To_upstreamv1beta3_BootstrapToken(in, out, s); err != nil {
		return err
	}
	out.TTL = clusterv1.ConvertFromSeconds(in.TTLSeconds)
	if !reflect.DeepEqual(in.Expires, metav1.Time{}) {
		out.Expires = ptr.To(in.Expires)
	}
	return nil
}

func Convert_v1beta2_DNS_To_upstreamv1beta3_DNS(in *bootstrapv1.DNS, out *DNS, _ apimachineryconversion.Scope) error {
	out.ImageRepository = in.ImageRepository
	out.ImageTag = in.ImageTag
	return nil
}

func Convert_v1beta2_Etcd_To_upstreamv1beta3_Etcd(in *bootstrapv1.Etcd, out *Etcd, s apimachineryconversion.Scope) error {
	if in.Local.IsDefined() {
		out.Local = &LocalEtcd{}
		if err := Convert_v1beta2_LocalEtcd_To_upstreamv1beta3_LocalEtcd(&in.Local, out.Local, s); err != nil {
			return err
		}
	}
	if in.External.IsDefined() {
		out.External = &ExternalEtcd{}
		if err := Convert_v1beta2_ExternalEtcd_To_upstreamv1beta3_ExternalEtcd(&in.External, out.External, s); err != nil {
			return err
		}
	}
	return nil
}

func Convert_v1beta2_Discovery_To_upstreamv1beta3_Discovery(in *bootstrapv1.Discovery, out *Discovery, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_Discovery_To_upstreamv1beta3_Discovery(in, out, s); err != nil {
		return err
	}
	if !reflect.DeepEqual(in.BootstrapToken, bootstrapv1.BootstrapTokenDiscovery{}) {
		out.BootstrapToken = &BootstrapTokenDiscovery{}
		if err := Convert_v1beta2_BootstrapTokenDiscovery_To_upstreamv1beta3_BootstrapTokenDiscovery(&in.BootstrapToken, out.BootstrapToken, s); err != nil {
			return err
		}
	}
	if !reflect.DeepEqual(in.File, bootstrapv1.FileDiscovery{}) {
		out.File = &FileDiscovery{}
		if err := Convert_v1beta2_FileDiscovery_To_upstreamv1beta3_FileDiscovery(&in.File, out.File, s); err != nil {
			return err
		}
	}
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

func Convert_v1beta2_ClusterConfiguration_To_upstreamv1beta3_ClusterConfiguration(in *bootstrapv1.ClusterConfiguration, out *ClusterConfiguration, s apimachineryconversion.Scope) error {
	return autoConvert_v1beta2_ClusterConfiguration_To_upstreamv1beta3_ClusterConfiguration(in, out, s)
}
