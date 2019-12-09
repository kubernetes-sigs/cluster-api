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

package v1alpha2

import (
	"errors"

	apiconversion "k8s.io/apimachinery/pkg/conversion"
	kubeadmbootstrapv1alpha3 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// ConvertTo converts this KubeadmConfig to the Hub version (v1alpha3).
func (src *KubeadmConfig) ConvertTo(dstRaw conversion.Hub) error { // nolint
	dst := dstRaw.(*kubeadmbootstrapv1alpha3.KubeadmConfig)
	return Convert_v1alpha2_KubeadmConfig_To_v1alpha3_KubeadmConfig(src, dst, nil)
}

// ConvertFrom converts from the KubeadmConfig Hub version (v1alpha3) to this version.
func (dst *KubeadmConfig) ConvertFrom(srcRaw conversion.Hub) error { // nolint
	src := srcRaw.(*kubeadmbootstrapv1alpha3.KubeadmConfig)
	return Convert_v1alpha3_KubeadmConfig_To_v1alpha2_KubeadmConfig(src, dst, nil)
}

// ConvertTo converts this KubeadmConfigList to the Hub version (v1alpha3).
func (src *KubeadmConfigList) ConvertTo(dstRaw conversion.Hub) error { // nolint
	dst := dstRaw.(*kubeadmbootstrapv1alpha3.KubeadmConfigList)
	return Convert_v1alpha2_KubeadmConfigList_To_v1alpha3_KubeadmConfigList(src, dst, nil)
}

// ConvertFrom converts from the KubeadmConfigList Hub version (v1alpha3) to this version.
func (dst *KubeadmConfigList) ConvertFrom(srcRaw conversion.Hub) error { // nolint
	src := srcRaw.(*kubeadmbootstrapv1alpha3.KubeadmConfigList)
	return Convert_v1alpha3_KubeadmConfigList_To_v1alpha2_KubeadmConfigList(src, dst, nil)
}

// ConvertTo converts this KubeadmConfigTemplate to the Hub version (v1alpha3).
func (src *KubeadmConfigTemplate) ConvertTo(dstRaw conversion.Hub) error { // nolint
	dst := dstRaw.(*kubeadmbootstrapv1alpha3.KubeadmConfigTemplate)
	return Convert_v1alpha2_KubeadmConfigTemplate_To_v1alpha3_KubeadmConfigTemplate(src, dst, nil)
}

// ConvertFrom converts from the KubeadmConfigTemplate Hub version (v1alpha3) to this version.
func (dst *KubeadmConfigTemplate) ConvertFrom(srcRaw conversion.Hub) error { // nolint
	src := srcRaw.(*kubeadmbootstrapv1alpha3.KubeadmConfigTemplate)
	return Convert_v1alpha3_KubeadmConfigTemplate_To_v1alpha2_KubeadmConfigTemplate(src, dst, nil)
}

// ConvertTo converts this KubeadmConfigTemplateList to the Hub version (v1alpha3).
func (src *KubeadmConfigTemplateList) ConvertTo(dstRaw conversion.Hub) error { // nolint
	dst := dstRaw.(*kubeadmbootstrapv1alpha3.KubeadmConfigTemplateList)
	return Convert_v1alpha2_KubeadmConfigTemplateList_To_v1alpha3_KubeadmConfigTemplateList(src, dst, nil)
}

// ConvertFrom converts from the KubeadmConfigTemplateList Hub version (v1alpha3) to this version.
func (dst *KubeadmConfigTemplateList) ConvertFrom(srcRaw conversion.Hub) error { // nolint
	src := srcRaw.(*kubeadmbootstrapv1alpha3.KubeadmConfigTemplateList)
	return Convert_v1alpha3_KubeadmConfigTemplateList_To_v1alpha2_KubeadmConfigTemplateList(src, dst, nil)
}

// Convert_v1alpha2_KubeadmConfigStatus_To_v1alpha3_KubeadmConfigStatus converts this KubeadmConfigStatus to the Hub version (v1alpha3).
func Convert_v1alpha2_KubeadmConfigStatus_To_v1alpha3_KubeadmConfigStatus(in *KubeadmConfigStatus, out *kubeadmbootstrapv1alpha3.KubeadmConfigStatus, s apiconversion.Scope) error { // nolint
	if err := autoConvert_v1alpha2_KubeadmConfigStatus_To_v1alpha3_KubeadmConfigStatus(in, out, s); err != nil {
		return err
	}

	// Manually convert the Error fields to the Failure fields
	out.FailureMessage = in.ErrorMessage
	out.FailureReason = in.ErrorReason

	return nil
}

// Convert_v1alpha3_KubeadmConfigStatus_To_v1alpha2_KubeadmConfigStatus converts from the Hub version (v1alpha3) of the KubeadmConfigStatus to this version.
func Convert_v1alpha3_KubeadmConfigStatus_To_v1alpha2_KubeadmConfigStatus(in *kubeadmbootstrapv1alpha3.KubeadmConfigStatus, out *KubeadmConfigStatus, s apiconversion.Scope) error { // nolint
	// We need to fail early here given that we don't want to leak information from secrets back to the inline / plaintext field in v1alpha2.
	if in.BootstrapData == nil && in.DataSecretName != nil {
		return errors.New("cannot convert KubeadmConfigStatus's bootstrap data from Secret reference to inline field")
	}

	if err := autoConvert_v1alpha3_KubeadmConfigStatus_To_v1alpha2_KubeadmConfigStatus(in, out, s); err != nil {
		return err
	}

	// Manually convert the Failure fields to the Error fields
	out.ErrorMessage = in.FailureMessage
	out.ErrorReason = in.FailureReason

	return nil
}
