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
	"github.com/pkg/errors"
	apimachineryconversion "k8s.io/apimachinery/pkg/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
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
	// - Timeouts (Not supported yet)
	err := autoConvert_upstreamv1beta4_JoinConfiguration_To_v1beta2_JoinConfiguration(in, out, s)

	// Handle migration of JoinConfiguration.Timeouts.TLSBootstrap to Discovery.Timeout.
	if in.Timeouts != nil && in.Timeouts.TLSBootstrap != nil {
		out.Discovery.Timeout = in.Timeouts.TLSBootstrap
	}

	return err
}

func Convert_upstreamv1beta4_JoinControlPlane_To_v1beta2_JoinControlPlane(in *JoinControlPlane, out *bootstrapv1.JoinControlPlane, s apimachineryconversion.Scope) error {
	// Following fields do not exist in CABPK v1beta1 version:
	// - CertificateKey (CABPK does not use automatic copy certs)
	return autoConvert_upstreamv1beta4_JoinControlPlane_To_v1beta2_JoinControlPlane(in, out, s)
}

// Custom conversion from the hub version, CABPK v1beta1, to this API, kubeadm v1beta4.

func Convert_v1beta2_APIServer_To_upstreamv1beta4_APIServer(in *bootstrapv1.APIServer, out *APIServer, s apimachineryconversion.Scope) error {
	// Following fields do not exist in kubeadm v1beta4 version:
	// - TimeoutForControlPlane (this field has been migrated to Init/JoinConfiguration; migration is handled by ConvertFromClusterConfiguration custom converters.
	return autoConvert_v1beta2_APIServer_To_upstreamv1beta4_APIServer(in, out, s)
}

func Convert_v1beta2_JoinConfiguration_To_upstreamv1beta4_JoinConfiguration(in *bootstrapv1.JoinConfiguration, out *JoinConfiguration, s apimachineryconversion.Scope) error {
	err := autoConvert_v1beta2_JoinConfiguration_To_upstreamv1beta4_JoinConfiguration(in, out, s)

	// Handle migration of Discovery.Timeout to JoinConfiguration.Timeouts.TLSBootstrap.
	if in.Discovery.Timeout != nil {
		if out.Timeouts == nil {
			out.Timeouts = &Timeouts{}
		}
		out.Timeouts.TLSBootstrap = in.Discovery.Timeout
	}
	return err
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

// Custom conversions to handle fields migrated from ClusterConfiguration to Init and JoinConfiguration in the kubeadm v1beta4 API version.

func (dst *InitConfiguration) ConvertFromClusterConfiguration(clusterConfiguration *bootstrapv1.ClusterConfiguration) error {
	if clusterConfiguration == nil || clusterConfiguration.APIServer.TimeoutForControlPlane == nil {
		return nil
	}

	if dst.Timeouts == nil {
		dst.Timeouts = &Timeouts{}
	}
	dst.Timeouts.ControlPlaneComponentHealthCheck = clusterConfiguration.APIServer.TimeoutForControlPlane
	return nil
}

func (dst *JoinConfiguration) ConvertFromClusterConfiguration(clusterConfiguration *bootstrapv1.ClusterConfiguration) error {
	if clusterConfiguration == nil || clusterConfiguration.APIServer.TimeoutForControlPlane == nil {
		return nil
	}

	if dst.Timeouts == nil {
		dst.Timeouts = &Timeouts{}
	}
	dst.Timeouts.ControlPlaneComponentHealthCheck = clusterConfiguration.APIServer.TimeoutForControlPlane
	return nil
}

func (src *InitConfiguration) ConvertToClusterConfiguration(clusterConfiguration *bootstrapv1.ClusterConfiguration) error {
	if src.Timeouts == nil || src.Timeouts.ControlPlaneComponentHealthCheck == nil {
		return nil
	}

	if clusterConfiguration == nil {
		return errors.New("cannot convert InitConfiguration to a nil ClusterConfiguration")
	}
	clusterConfiguration.APIServer.TimeoutForControlPlane = src.Timeouts.ControlPlaneComponentHealthCheck
	return nil
}

func (src *JoinConfiguration) ConvertToClusterConfiguration(clusterConfiguration *bootstrapv1.ClusterConfiguration) error {
	if src.Timeouts == nil || src.Timeouts.ControlPlaneComponentHealthCheck == nil {
		return nil
	}

	if clusterConfiguration == nil {
		return errors.New("cannot convert JoinConfiguration to a nil ClusterConfiguration")
	}
	clusterConfiguration.APIServer.TimeoutForControlPlane = src.Timeouts.ControlPlaneComponentHealthCheck
	return nil
}
