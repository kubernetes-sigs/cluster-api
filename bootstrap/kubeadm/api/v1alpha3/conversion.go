/*
Copyright 2019 The Kubernetes Authors.

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

package v1alpha3

import (
	apiconversion "k8s.io/apimachinery/pkg/conversion"
	kubeadmbootstrapv1alpha4 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha4"
	kubeadmbootstrapv1beta1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/v1beta1"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// ConvertTo converts this KubeadmConfig to the Hub version (v1alpha4).
func (src *KubeadmConfig) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*kubeadmbootstrapv1alpha4.KubeadmConfig)

	if err := Convert_v1alpha3_KubeadmConfig_To_v1alpha4_KubeadmConfig(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &kubeadmbootstrapv1alpha4.KubeadmConfig{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	if restored.Spec.JoinConfiguration != nil && restored.Spec.JoinConfiguration.NodeRegistration.IgnorePreflightErrors != nil {
		if dst.Spec.JoinConfiguration == nil {
			dst.Spec.JoinConfiguration = &kubeadmbootstrapv1alpha4.JoinConfiguration{}
		}
		dst.Spec.JoinConfiguration.NodeRegistration.IgnorePreflightErrors = restored.Spec.JoinConfiguration.NodeRegistration.IgnorePreflightErrors
	}

	if restored.Spec.InitConfiguration != nil && restored.Spec.InitConfiguration.NodeRegistration.IgnorePreflightErrors != nil {
		if dst.Spec.InitConfiguration == nil {
			dst.Spec.InitConfiguration = &kubeadmbootstrapv1alpha4.InitConfiguration{}
		}
		dst.Spec.InitConfiguration.NodeRegistration.IgnorePreflightErrors = restored.Spec.InitConfiguration.NodeRegistration.IgnorePreflightErrors
	}

	return nil
}

// ConvertFrom converts from the KubeadmConfig Hub version (v1alpha4) to this version.
func (dst *KubeadmConfig) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*kubeadmbootstrapv1alpha4.KubeadmConfig)

	if err := Convert_v1alpha4_KubeadmConfig_To_v1alpha3_KubeadmConfig(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion except for metadata
	if err := utilconversion.MarshalData(src, dst); err != nil {
		return err
	}

	return nil
}

// ConvertTo converts this KubeadmConfigList to the Hub version (v1alpha4).
func (src *KubeadmConfigList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*kubeadmbootstrapv1alpha4.KubeadmConfigList)
	return Convert_v1alpha3_KubeadmConfigList_To_v1alpha4_KubeadmConfigList(src, dst, nil)
}

// ConvertFrom converts from the KubeadmConfigList Hub version (v1alpha4) to this version.
func (dst *KubeadmConfigList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*kubeadmbootstrapv1alpha4.KubeadmConfigList)
	return Convert_v1alpha4_KubeadmConfigList_To_v1alpha3_KubeadmConfigList(src, dst, nil)
}

// ConvertTo converts this KubeadmConfigTemplate to the Hub version (v1alpha4).
func (src *KubeadmConfigTemplate) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*kubeadmbootstrapv1alpha4.KubeadmConfigTemplate)

	if err := Convert_v1alpha3_KubeadmConfigTemplate_To_v1alpha4_KubeadmConfigTemplate(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &kubeadmbootstrapv1alpha4.KubeadmConfigTemplate{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	if restored.Spec.Template.Spec.JoinConfiguration != nil && restored.Spec.Template.Spec.JoinConfiguration.NodeRegistration.IgnorePreflightErrors != nil {
		if dst.Spec.Template.Spec.JoinConfiguration == nil {
			dst.Spec.Template.Spec.JoinConfiguration = &kubeadmbootstrapv1alpha4.JoinConfiguration{}
		}
		dst.Spec.Template.Spec.JoinConfiguration.NodeRegistration.IgnorePreflightErrors = restored.Spec.Template.Spec.JoinConfiguration.NodeRegistration.IgnorePreflightErrors
	}

	if restored.Spec.Template.Spec.InitConfiguration != nil && restored.Spec.Template.Spec.InitConfiguration.NodeRegistration.IgnorePreflightErrors != nil {
		if dst.Spec.Template.Spec.InitConfiguration == nil {
			dst.Spec.Template.Spec.InitConfiguration = &kubeadmbootstrapv1alpha4.InitConfiguration{}
		}
		dst.Spec.Template.Spec.InitConfiguration.NodeRegistration.IgnorePreflightErrors = restored.Spec.Template.Spec.InitConfiguration.NodeRegistration.IgnorePreflightErrors
	}

	return nil
}

// ConvertFrom converts from the KubeadmConfigTemplate Hub version (v1alpha4) to this version.
func (dst *KubeadmConfigTemplate) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*kubeadmbootstrapv1alpha4.KubeadmConfigTemplate)

	if err := Convert_v1alpha4_KubeadmConfigTemplate_To_v1alpha3_KubeadmConfigTemplate(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion except for metadata
	if err := utilconversion.MarshalData(src, dst); err != nil {
		return err
	}

	return nil
}

// ConvertTo converts this KubeadmConfigTemplateList to the Hub version (v1alpha3).
func (src *KubeadmConfigTemplateList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*kubeadmbootstrapv1alpha4.KubeadmConfigTemplateList)
	return Convert_v1alpha3_KubeadmConfigTemplateList_To_v1alpha4_KubeadmConfigTemplateList(src, dst, nil)
}

// ConvertFrom converts from the KubeadmConfigTemplateList Hub version (v1alpha3) to this version.
func (dst *KubeadmConfigTemplateList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*kubeadmbootstrapv1alpha4.KubeadmConfigTemplateList)
	return Convert_v1alpha4_KubeadmConfigTemplateList_To_v1alpha3_KubeadmConfigTemplateList(src, dst, nil)
}

// Convert_v1alpha3_KubeadmConfigStatus_To_v1alpha4_KubeadmConfigStatus is an autogenerated conversion function.
func Convert_v1alpha3_KubeadmConfigStatus_To_v1alpha4_KubeadmConfigStatus(in *KubeadmConfigStatus, out *kubeadmbootstrapv1alpha4.KubeadmConfigStatus, s apiconversion.Scope) error { //nolint
	// KubeadmConfigStatus.BootstrapData has been removed in v1alpha4 because its content has been moved to the bootstrap data secret, value will be lost during conversion.
	return autoConvert_v1alpha3_KubeadmConfigStatus_To_v1alpha4_KubeadmConfigStatus(in, out, s)
}

func Convert_v1alpha4_ClusterConfiguration_To_v1beta1_ClusterConfiguration(in *kubeadmbootstrapv1alpha4.ClusterConfiguration, out *kubeadmbootstrapv1beta1.ClusterConfiguration, s apiconversion.Scope) error {
	// DNS.Type was removed in v1alpha4 because only CoreDNS is supported; the information will be left to empty (kubeadm defaults it to CoredDNS);
	// Existing clusters using kube-dns or other DNS solutions will continue to be managed/supported via the skip-coredns annotation.

	// ClusterConfiguration.UseHyperKubeImage was removed in kubeadm v1alpha4 API
	return kubeadmbootstrapv1beta1.Convert_v1alpha4_ClusterConfiguration_To_v1beta1_ClusterConfiguration(in, out, s)
}

func Convert_v1beta1_ClusterConfiguration_To_v1alpha4_ClusterConfiguration(in *kubeadmbootstrapv1beta1.ClusterConfiguration, out *kubeadmbootstrapv1alpha4.ClusterConfiguration, s apiconversion.Scope) error {
	// DNS.Type was removed in v1alpha4 because only CoreDNS is supported; the information will be left to empty (kubeadm defaults it to CoredDNS);
	// ClusterConfiguration.UseHyperKubeImage was removed in kubeadm v1alpha4 API
	return kubeadmbootstrapv1beta1.Convert_v1beta1_ClusterConfiguration_To_v1alpha4_ClusterConfiguration(in, out, s)
}

func Convert_v1alpha4_InitConfiguration_To_v1beta1_InitConfiguration(in *kubeadmbootstrapv1alpha4.InitConfiguration, out *kubeadmbootstrapv1beta1.InitConfiguration, s apiconversion.Scope) error {
	// NodeRegistrationOptions.IgnorePreflightErrors does not exist in kubeadm v1beta1 API
	return kubeadmbootstrapv1beta1.Convert_v1alpha4_InitConfiguration_To_v1beta1_InitConfiguration(in, out, s)
}

func Convert_v1beta1_InitConfiguration_To_v1alpha4_InitConfiguration(in *kubeadmbootstrapv1beta1.InitConfiguration, out *kubeadmbootstrapv1alpha4.InitConfiguration, s apiconversion.Scope) error {
	// NodeRegistrationOptions.IgnorePreflightErrors does not exist in kubeadm v1beta1 API
	return kubeadmbootstrapv1beta1.Convert_v1beta1_InitConfiguration_To_v1alpha4_InitConfiguration(in, out, s)
}

func Convert_v1alpha4_JoinConfiguration_To_v1beta1_JoinConfiguration(in *kubeadmbootstrapv1alpha4.JoinConfiguration, out *kubeadmbootstrapv1beta1.JoinConfiguration, s apiconversion.Scope) error {
	// NodeRegistrationOptions.IgnorePreflightErrors does not exist in kubeadm v1beta1 API
	return kubeadmbootstrapv1beta1.Convert_v1alpha4_JoinConfiguration_To_v1beta1_JoinConfiguration(in, out, s)
}

func Convert_v1beta1_JoinConfiguration_To_v1alpha4_JoinConfiguration(in *kubeadmbootstrapv1beta1.JoinConfiguration, out *kubeadmbootstrapv1alpha4.JoinConfiguration, s apiconversion.Scope) error {
	// NodeRegistrationOptions.IgnorePreflightErrors does not exist in kubeadm v1beta1 API
	return kubeadmbootstrapv1beta1.Convert_v1beta1_JoinConfiguration_To_v1alpha4_JoinConfiguration(in, out, s)
}
