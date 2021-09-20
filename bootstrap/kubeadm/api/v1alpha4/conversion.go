/*
Copyright 2020 The Kubernetes Authors.

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

package v1alpha4

import (
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

func (src *KubeadmConfig) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.KubeadmConfig)

	return Convert_v1alpha4_KubeadmConfig_To_v1beta1_KubeadmConfig(src, dst, nil)
}

func (dst *KubeadmConfig) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta1.KubeadmConfig)

	return Convert_v1beta1_KubeadmConfig_To_v1alpha4_KubeadmConfig(src, dst, nil)
}

func (src *KubeadmConfigList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.KubeadmConfigList)

	return Convert_v1alpha4_KubeadmConfigList_To_v1beta1_KubeadmConfigList(src, dst, nil)
}

func (dst *KubeadmConfigList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta1.KubeadmConfigList)

	return Convert_v1beta1_KubeadmConfigList_To_v1alpha4_KubeadmConfigList(src, dst, nil)
}

func (src *KubeadmConfigTemplate) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.KubeadmConfigTemplate)

	return Convert_v1alpha4_KubeadmConfigTemplate_To_v1beta1_KubeadmConfigTemplate(src, dst, nil)
}

func (dst *KubeadmConfigTemplate) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta1.KubeadmConfigTemplate)

	return Convert_v1beta1_KubeadmConfigTemplate_To_v1alpha4_KubeadmConfigTemplate(src, dst, nil)
}

func (src *KubeadmConfigTemplateList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.KubeadmConfigTemplateList)

	return Convert_v1alpha4_KubeadmConfigTemplateList_To_v1beta1_KubeadmConfigTemplateList(src, dst, nil)
}

func (dst *KubeadmConfigTemplateList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta1.KubeadmConfigTemplateList)

	return Convert_v1beta1_KubeadmConfigTemplateList_To_v1alpha4_KubeadmConfigTemplateList(src, dst, nil)
}
