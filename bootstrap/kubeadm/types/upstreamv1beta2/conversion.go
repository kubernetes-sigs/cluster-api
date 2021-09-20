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
	apimachineryconversion "k8s.io/apimachinery/pkg/conversion"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

func (src *ClusterConfiguration) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*bootstrapv1.ClusterConfiguration)
	return Convert_upstreamv1beta2_ClusterConfiguration_To_v1beta1_ClusterConfiguration(src, dst, nil)
}

func (dst *ClusterConfiguration) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*bootstrapv1.ClusterConfiguration)
	return Convert_v1beta1_ClusterConfiguration_To_upstreamv1beta2_ClusterConfiguration(src, dst, nil)
}

func (src *ClusterStatus) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*bootstrapv1.ClusterStatus)
	return Convert_upstreamv1beta2_ClusterStatus_To_v1beta1_ClusterStatus(src, dst, nil)
}

func (dst *ClusterStatus) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*bootstrapv1.ClusterStatus)
	return Convert_v1beta1_ClusterStatus_To_upstreamv1beta2_ClusterStatus(src, dst, nil)
}

func (src *InitConfiguration) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*bootstrapv1.InitConfiguration)
	return Convert_upstreamv1beta2_InitConfiguration_To_v1beta1_InitConfiguration(src, dst, nil)
}

func (dst *InitConfiguration) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*bootstrapv1.InitConfiguration)
	return Convert_v1beta1_InitConfiguration_To_upstreamv1beta2_InitConfiguration(src, dst, nil)
}

func (src *JoinConfiguration) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*bootstrapv1.JoinConfiguration)
	return Convert_upstreamv1beta2_JoinConfiguration_To_v1beta1_JoinConfiguration(src, dst, nil)
}

func (dst *JoinConfiguration) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*bootstrapv1.JoinConfiguration)
	return Convert_v1beta1_JoinConfiguration_To_upstreamv1beta2_JoinConfiguration(src, dst, nil)
}

func Convert_upstreamv1beta2_InitConfiguration_To_v1beta1_InitConfiguration(in *InitConfiguration, out *bootstrapv1.InitConfiguration, s apimachineryconversion.Scope) error {
	// InitConfiguration.CertificateKey exists in v1beta2 types but not in bootstrapv1.InitConfiguration (Cluster API does not uses automatic copy certs). Ignoring when converting.
	return autoConvert_upstreamv1beta2_InitConfiguration_To_v1beta1_InitConfiguration(in, out, s)
}

func Convert_upstreamv1beta2_JoinControlPlane_To_v1beta1_JoinControlPlane(in *JoinControlPlane, out *bootstrapv1.JoinControlPlane, s apimachineryconversion.Scope) error {
	// JoinControlPlane.CertificateKey exists in v1beta2 types but not in bootstrapv1.JoinControlPlane (Cluster API does not uses automatic copy certs). Ignoring when converting.
	return autoConvert_upstreamv1beta2_JoinControlPlane_To_v1beta1_JoinControlPlane(in, out, s)
}

func Convert_upstreamv1beta2_DNS_To_v1beta1_DNS(in *DNS, out *bootstrapv1.DNS, s apimachineryconversion.Scope) error {
	// DNS.Type was removed in v1alpha4 because only CoreDNS is supported, dropping this info.
	return autoConvert_upstreamv1beta2_DNS_To_v1beta1_DNS(in, out, s)
}

func Convert_upstreamv1beta2_ClusterConfiguration_To_v1beta1_ClusterConfiguration(in *ClusterConfiguration, out *bootstrapv1.ClusterConfiguration, s apimachineryconversion.Scope) error {
	// ClusterConfiguration.UseHyperKubeImage was removed in kubeadm v1alpha4 API
	return autoConvert_upstreamv1beta2_ClusterConfiguration_To_v1beta1_ClusterConfiguration(in, out, s)
}
