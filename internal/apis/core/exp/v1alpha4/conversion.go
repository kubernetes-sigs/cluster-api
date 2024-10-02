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
	apimachineryconversion "k8s.io/apimachinery/pkg/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	clusterv1alpha4 "sigs.k8s.io/cluster-api/internal/apis/core/v1alpha4"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

func (src *MachinePool) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*expv1.MachinePool)

	if err := Convert_v1alpha4_MachinePool_To_v1beta1_MachinePool(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &expv1.MachinePool{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}
	dst.Spec.Template.Spec.ReadinessGates = restored.Spec.Template.Spec.ReadinessGates
	dst.Spec.Template.Spec.NodeDeletionTimeout = restored.Spec.Template.Spec.NodeDeletionTimeout
	dst.Spec.Template.Spec.NodeVolumeDetachTimeout = restored.Spec.Template.Spec.NodeVolumeDetachTimeout
	dst.Status.V1Beta2 = restored.Status.V1Beta2

	return nil
}

func (dst *MachinePool) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*expv1.MachinePool)

	if err := Convert_v1beta1_MachinePool_To_v1alpha4_MachinePool(src, dst, nil); err != nil {
		return err
	}
	return utilconversion.MarshalData(src, dst)
}

func (src *MachinePoolList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*expv1.MachinePoolList)

	return Convert_v1alpha4_MachinePoolList_To_v1beta1_MachinePoolList(src, dst, nil)
}

func (dst *MachinePoolList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*expv1.MachinePoolList)

	return Convert_v1beta1_MachinePoolList_To_v1alpha4_MachinePoolList(src, dst, nil)
}

func Convert_v1alpha4_MachineTemplateSpec_To_v1beta1_MachineTemplateSpec(in *clusterv1alpha4.MachineTemplateSpec, out *clusterv1.MachineTemplateSpec, s apimachineryconversion.Scope) error {
	return clusterv1alpha4.Convert_v1alpha4_MachineTemplateSpec_To_v1beta1_MachineTemplateSpec(in, out, s)
}

func Convert_v1beta1_MachineTemplateSpec_To_v1alpha4_MachineTemplateSpec(in *clusterv1.MachineTemplateSpec, out *clusterv1alpha4.MachineTemplateSpec, s apimachineryconversion.Scope) error {
	return clusterv1alpha4.Convert_v1beta1_MachineTemplateSpec_To_v1alpha4_MachineTemplateSpec(in, out, s)
}

func Convert_v1beta1_MachinePoolStatus_To_v1alpha4_MachinePoolStatus(in *expv1.MachinePoolStatus, out *MachinePoolStatus, s apimachineryconversion.Scope) error {
	// V1Beta2 was added in v1beta1
	return autoConvert_v1beta1_MachinePoolStatus_To_v1alpha4_MachinePoolStatus(in, out, s)
}
