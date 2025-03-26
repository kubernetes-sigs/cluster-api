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

package v1beta1

import (
	apiconversion "k8s.io/apimachinery/pkg/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	clusterv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta2"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta2"
)

func (src *MachinePool) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*expv1.MachinePool)

	return Convert_v1beta1_MachinePool_To_v1beta2_MachinePool(src, dst, nil)
}

func (dst *MachinePool) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*expv1.MachinePool)

	return Convert_v1beta2_MachinePool_To_v1beta1_MachinePool(src, dst, nil)
}

// Implement local conversion func because conversion-gen is not aware of conversion func in other packages (see https://github.com/kubernetes/code-generator/issues/94)

func Convert_v1beta1_MachineTemplateSpec_To_v1beta2_MachineTemplateSpec(in *clusterv1beta1.MachineTemplateSpec, out *clusterv1.MachineTemplateSpec, s apiconversion.Scope) error {
	return clusterv1beta1.Convert_v1beta1_MachineTemplateSpec_To_v1beta2_MachineTemplateSpec(in, out, s)
}

func Convert_v1beta2_MachineTemplateSpec_To_v1beta1_MachineTemplateSpec(in *clusterv1.MachineTemplateSpec, out *clusterv1beta1.MachineTemplateSpec, s apiconversion.Scope) error {
	return clusterv1beta1.Convert_v1beta2_MachineTemplateSpec_To_v1beta1_MachineTemplateSpec(in, out, s)
}
