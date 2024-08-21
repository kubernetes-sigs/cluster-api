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
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
)

func (src *ClusterConfiguration) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*bootstrapv1.ClusterConfiguration)
	return Convert_upstreamv1beta3_ClusterConfiguration_To_v1beta1_ClusterConfiguration(src, dst, nil)
}

func (dst *ClusterConfiguration) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*bootstrapv1.ClusterConfiguration)
	return Convert_v1beta1_ClusterConfiguration_To_upstreamv1beta3_ClusterConfiguration(src, dst, nil)
}

func (src *InitConfiguration) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*bootstrapv1.InitConfiguration)
	return Convert_upstreamv1beta3_InitConfiguration_To_v1beta1_InitConfiguration(src, dst, nil)
}

func (dst *InitConfiguration) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*bootstrapv1.InitConfiguration)
	return Convert_v1beta1_InitConfiguration_To_upstreamv1beta3_InitConfiguration(src, dst, nil)
}

func (src *JoinConfiguration) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*bootstrapv1.JoinConfiguration)
	return Convert_upstreamv1beta3_JoinConfiguration_To_v1beta1_JoinConfiguration(src, dst, nil)
}

func (dst *JoinConfiguration) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*bootstrapv1.JoinConfiguration)
	return Convert_v1beta1_JoinConfiguration_To_upstreamv1beta3_JoinConfiguration(src, dst, nil)
}

// Custom conversion from this API, kubeadm v1beta3, to the hub version, CABPK v1beta1.

func Convert_upstreamv1beta3_InitConfiguration_To_v1beta1_InitConfiguration(in *InitConfiguration, out *bootstrapv1.InitConfiguration, s apimachineryconversion.Scope) error {
	// InitConfiguration.CertificateKey does not exist in CABPK v1beta1 version, because Cluster API does not use automatic copy certs.
	return autoConvert_upstreamv1beta3_InitConfiguration_To_v1beta1_InitConfiguration(in, out, s)
}

func Convert_upstreamv1beta3_JoinControlPlane_To_v1beta1_JoinControlPlane(in *JoinControlPlane, out *bootstrapv1.JoinControlPlane, s apimachineryconversion.Scope) error {
	// JoinControlPlane.CertificateKey does not exist in CABPK v1beta1 version, because Cluster API does not use automatic copy certs.
	return autoConvert_upstreamv1beta3_JoinControlPlane_To_v1beta1_JoinControlPlane(in, out, s)
}

// Custom conversion from the hub version, CABPK v1beta1, to this API, kubeadm v1beta3.

func Convert_v1beta1_FileDiscovery_To_upstreamv1beta3_FileDiscovery(in *bootstrapv1.FileDiscovery, out *FileDiscovery, s apimachineryconversion.Scope) error {
	// JoinConfiguration.Discovery.File.KubeConfig does not exist in kubeadm because it's internal to Cluster API, dropping those info.
	return autoConvert_v1beta1_FileDiscovery_To_upstreamv1beta3_FileDiscovery(in, out, s)
}

func Convert_v1beta1_ControlPlaneComponent_To_upstreamv1beta3_ControlPlaneComponent(in *bootstrapv1.ControlPlaneComponent, out *ControlPlaneComponent, s apimachineryconversion.Scope) error {
	// ControlPlaneComponent.ExtraEnvs does not exist in kubeadm v1beta3, dropping this info.
	return autoConvert_v1beta1_ControlPlaneComponent_To_upstreamv1beta3_ControlPlaneComponent(in, out, s)
}

func Convert_v1beta1_LocalEtcd_To_upstreamv1beta3_LocalEtcd(in *bootstrapv1.LocalEtcd, out *LocalEtcd, s apimachineryconversion.Scope) error {
	// LocalEtcd.ExtraEnvs does not exist in kubeadm v1beta3, dropping this info.
	return autoConvert_v1beta1_LocalEtcd_To_upstreamv1beta3_LocalEtcd(in, out, s)
}

func Convert_v1beta1_NodeRegistrationOptions_To_upstreamv1beta3_NodeRegistrationOptions(in *bootstrapv1.NodeRegistrationOptions, out *NodeRegistrationOptions, s apimachineryconversion.Scope) error {
	// NodeRegistrationOptions.ImagePullSerial does not exist in kubeadm v1beta3, dropping this info.
	return autoConvert_v1beta1_NodeRegistrationOptions_To_upstreamv1beta3_NodeRegistrationOptions(in, out, s)
}
