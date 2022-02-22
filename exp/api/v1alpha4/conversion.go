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
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
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
	dst.Spec.Template.Spec.NodeDeletionTimeout = restored.Spec.Template.Spec.NodeDeletionTimeout
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
