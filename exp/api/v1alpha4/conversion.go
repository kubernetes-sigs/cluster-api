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
	v1beta1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

func (src *MachinePool) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.MachinePool)

	return Convert_v1alpha4_MachinePool_To_v1beta1_MachinePool(src, dst, nil)
}

func (dst *MachinePool) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta1.MachinePool)

	return Convert_v1beta1_MachinePool_To_v1alpha4_MachinePool(src, dst, nil)
}

func (src *MachinePoolList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.MachinePoolList)

	return Convert_v1alpha4_MachinePoolList_To_v1beta1_MachinePoolList(src, dst, nil)
}

func (dst *MachinePoolList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta1.MachinePoolList)

	return Convert_v1beta1_MachinePoolList_To_v1alpha4_MachinePoolList(src, dst, nil)
}
