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

	convert "k8s.io/apimachinery/pkg/conversion"
	infraexpv1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/exp/api/v1beta1"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

func (src *DockerMachinePool) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*infraexpv1.DockerMachinePool)

	err := Convert_v1alpha4_DockerMachinePool_To_v1beta1_DockerMachinePool(src, dst, nil)
	if err != nil {
		return err
	}

	// Manually restore data.
	restored := &infraexpv1.DockerMachinePool{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	// Restore list of infrastructure references.
	dst.Spec.InfrastructureRefList = restored.Spec.InfrastructureRefList

	return nil
}

func (dst *DockerMachinePool) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*infraexpv1.DockerMachinePool)

	if err := Convert_v1beta1_DockerMachinePool_To_v1alpha4_DockerMachinePool(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion.
	if err := utilconversion.MarshalData(src, dst); err != nil {
		return err
	}

	return nil
}

func Convert_v1beta1_DockerMachinePoolSpec_To_v1alpha4_DockerMachinePoolSpec(in *infraexpv1.DockerMachinePoolSpec, out *DockerMachinePoolSpec, s convert.Scope) error {
	return autoConvert_v1beta1_DockerMachinePoolSpec_To_v1alpha4_DockerMachinePoolSpec(in, out, s)
}

func Convert_v1alpha4_DockerMachinePoolSpec_To_v1beta1_DockerMachinePoolSpec(in *DockerMachinePoolSpec, out *infraexpv1.DockerMachinePoolSpec, s convert.Scope) error {
	return autoConvert_v1alpha4_DockerMachinePoolSpec_To_v1beta1_DockerMachinePoolSpec(in, out, s)
}

func (src *DockerMachinePoolList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*infraexpv1.DockerMachinePoolList)

	return Convert_v1alpha4_DockerMachinePoolList_To_v1beta1_DockerMachinePoolList(src, dst, nil)
}

func (dst *DockerMachinePoolList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*infraexpv1.DockerMachinePoolList)

	return Convert_v1beta1_DockerMachinePoolList_To_v1alpha4_DockerMachinePoolList(src, dst, nil)
}
