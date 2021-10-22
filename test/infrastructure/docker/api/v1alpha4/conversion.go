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

package v1alpha4

import (
	apiconversion "k8s.io/apimachinery/pkg/conversion"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

func (src *DockerCluster) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.DockerCluster)

	return Convert_v1alpha4_DockerCluster_To_v1beta1_DockerCluster(src, dst, nil)
}

func (dst *DockerCluster) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta1.DockerCluster)

	return Convert_v1beta1_DockerCluster_To_v1alpha4_DockerCluster(src, dst, nil)
}

func (src *DockerClusterList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.DockerClusterList)

	return Convert_v1alpha4_DockerClusterList_To_v1beta1_DockerClusterList(src, dst, nil)
}

func (dst *DockerClusterList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta1.DockerClusterList)

	return Convert_v1beta1_DockerClusterList_To_v1alpha4_DockerClusterList(src, dst, nil)
}

func (src *DockerClusterTemplate) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.DockerClusterTemplate)

	if err := Convert_v1alpha4_DockerClusterTemplate_To_v1beta1_DockerClusterTemplate(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &v1beta1.DockerClusterTemplate{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	dst.Spec.Template.ObjectMeta = restored.Spec.Template.ObjectMeta

	return nil
}

func (dst *DockerClusterTemplate) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta1.DockerClusterTemplate)

	if err := Convert_v1beta1_DockerClusterTemplate_To_v1alpha4_DockerClusterTemplate(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion except for metadata
	if err := utilconversion.MarshalData(src, dst); err != nil {
		return err
	}

	return nil
}

func (src *DockerClusterTemplateList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.DockerClusterTemplateList)

	return Convert_v1alpha4_DockerClusterTemplateList_To_v1beta1_DockerClusterTemplateList(src, dst, nil)
}

func (dst *DockerClusterTemplateList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta1.DockerClusterTemplateList)

	return Convert_v1beta1_DockerClusterTemplateList_To_v1alpha4_DockerClusterTemplateList(src, dst, nil)
}

func (src *DockerMachine) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.DockerMachine)

	return Convert_v1alpha4_DockerMachine_To_v1beta1_DockerMachine(src, dst, nil)
}

func (dst *DockerMachine) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta1.DockerMachine)

	return Convert_v1beta1_DockerMachine_To_v1alpha4_DockerMachine(src, dst, nil)
}

func (src *DockerMachineList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.DockerMachineList)

	return Convert_v1alpha4_DockerMachineList_To_v1beta1_DockerMachineList(src, dst, nil)
}

func (dst *DockerMachineList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta1.DockerMachineList)

	return Convert_v1beta1_DockerMachineList_To_v1alpha4_DockerMachineList(src, dst, nil)
}

func (src *DockerMachineTemplate) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.DockerMachineTemplate)

	if err := Convert_v1alpha4_DockerMachineTemplate_To_v1beta1_DockerMachineTemplate(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &v1beta1.DockerMachineTemplate{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	dst.Spec.Template.ObjectMeta = restored.Spec.Template.ObjectMeta

	return nil
}

func (dst *DockerMachineTemplate) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta1.DockerMachineTemplate)

	if err := Convert_v1beta1_DockerMachineTemplate_To_v1alpha4_DockerMachineTemplate(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion except for metadata
	if err := utilconversion.MarshalData(src, dst); err != nil {
		return err
	}

	return nil
}

func (src *DockerMachineTemplateList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.DockerMachineTemplateList)

	return Convert_v1alpha4_DockerMachineTemplateList_To_v1beta1_DockerMachineTemplateList(src, dst, nil)
}

func (dst *DockerMachineTemplateList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta1.DockerMachineTemplateList)

	return Convert_v1beta1_DockerMachineTemplateList_To_v1alpha4_DockerMachineTemplateList(src, dst, nil)
}

func Convert_v1beta1_DockerClusterTemplateResource_To_v1alpha4_DockerClusterTemplateResource(in *v1beta1.DockerClusterTemplateResource, out *DockerClusterTemplateResource, s apiconversion.Scope) error {
	// NOTE: custom conversion func is required because spec.template.metadata has been added in v1beta1.
	return autoConvert_v1beta1_DockerClusterTemplateResource_To_v1alpha4_DockerClusterTemplateResource(in, out, s)
}

func Convert_v1beta1_DockerMachineTemplateResource_To_v1alpha4_DockerMachineTemplateResource(in *v1beta1.DockerMachineTemplateResource, out *DockerMachineTemplateResource, s apiconversion.Scope) error {
	// NOTE: custom conversion func is required because spec.template.metadata has been added in v1beta1.
	return autoConvert_v1beta1_DockerMachineTemplateResource_To_v1alpha4_DockerMachineTemplateResource(in, out, s)
}
