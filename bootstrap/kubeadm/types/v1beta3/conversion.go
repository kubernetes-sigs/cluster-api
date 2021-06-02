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

package v1beta3

import (
	apimachineryconversion "k8s.io/apimachinery/pkg/conversion"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha4"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// ConvertTo converts this ClusterConfiguration to the Hub version (v1alpha4).
func (src *ClusterConfiguration) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*bootstrapv1.ClusterConfiguration)
	return Convert_v1beta3_ClusterConfiguration_To_v1alpha4_ClusterConfiguration(src, dst, nil)
}

// ConvertFrom converts from the ClusterConfiguration Hub version (v1alpha4) to this version.
func (dst *ClusterConfiguration) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*bootstrapv1.ClusterConfiguration)
	return Convert_v1alpha4_ClusterConfiguration_To_v1beta3_ClusterConfiguration(src, dst, nil)
}

// ConvertTo converts this InitConfiguration to the Hub version (v1alpha4).
func (src *InitConfiguration) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*bootstrapv1.InitConfiguration)
	return Convert_v1beta3_InitConfiguration_To_v1alpha4_InitConfiguration(src, dst, nil)
}

// ConvertFrom converts from the InitConfiguration Hub version (v1alpha4) to this version.
func (dst *InitConfiguration) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*bootstrapv1.InitConfiguration)
	return Convert_v1alpha4_InitConfiguration_To_v1beta3_InitConfiguration(src, dst, nil)
}

// ConvertTo converts this JoinConfiguration to the Hub version (v1alpha4).
func (src *JoinConfiguration) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*bootstrapv1.JoinConfiguration)
	return Convert_v1beta3_JoinConfiguration_To_v1alpha4_JoinConfiguration(src, dst, nil)
}

// ConvertFrom converts from the JoinConfiguration Hub version (v1alpha4) to this version.
func (dst *JoinConfiguration) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*bootstrapv1.JoinConfiguration)
	return Convert_v1alpha4_JoinConfiguration_To_v1beta3_JoinConfiguration(src, dst, nil)
}

func Convert_v1beta3_InitConfiguration_To_v1alpha4_InitConfiguration(in *InitConfiguration, out *bootstrapv1.InitConfiguration, s apimachineryconversion.Scope) error {
	// InitConfiguration.CertificateKey and SkipPhases exists in v1beta3 types but not in bootstrapv1.InitConfiguration (Cluster API does not uses automatic copy certs or does not support SkipPhases for now)). Ignoring when converting.
	return autoConvert_v1beta3_InitConfiguration_To_v1alpha4_InitConfiguration(in, out, s)
}

func Convert_v1beta3_JoinConfiguration_To_v1alpha4_JoinConfiguration(in *JoinConfiguration, out *bootstrapv1.JoinConfiguration, s apimachineryconversion.Scope) error {
	// JoinConfiguration.SkipPhases exists in v1beta3 types but not in bootstrapv1.JoinConfiguration (Cluster API does not support SkipPhases for now). Ignoring when converting.
	return autoConvert_v1beta3_JoinConfiguration_To_v1alpha4_JoinConfiguration(in, out, s)
}

func Convert_v1beta3_NodeRegistrationOptions_To_v1alpha4_NodeRegistrationOptions(in *NodeRegistrationOptions, out *bootstrapv1.NodeRegistrationOptions, s apimachineryconversion.Scope) error {
	// NodeRegistrationOptions.IgnorePreflightErrors exists in v1beta3 types but not in bootstrapv1.NodeRegistrationOptions (Cluster API does not support it for now). Ignoring when converting.
	return autoConvert_v1beta3_NodeRegistrationOptions_To_v1alpha4_NodeRegistrationOptions(in, out, s)
}

func Convert_v1beta3_JoinControlPlane_To_v1alpha4_JoinControlPlane(in *JoinControlPlane, out *bootstrapv1.JoinControlPlane, s apimachineryconversion.Scope) error {
	// JoinControlPlane.CertificateKey exists in v1beta3 types but not in bootstrapv1.JoinControlPlane (Cluster API does not uses automatic copy certs). Ignoring when converting.
	return autoConvert_v1beta3_JoinControlPlane_To_v1alpha4_JoinControlPlane(in, out, s)
}
