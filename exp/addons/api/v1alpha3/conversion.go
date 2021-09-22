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

package v1alpha3

import (
	v1beta1 "sigs.k8s.io/cluster-api/exp/addons/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

func (src *ClusterResourceSet) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.ClusterResourceSet)

	return Convert_v1alpha3_ClusterResourceSet_To_v1beta1_ClusterResourceSet(src, dst, nil)
}

func (dst *ClusterResourceSet) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta1.ClusterResourceSet)

	return Convert_v1beta1_ClusterResourceSet_To_v1alpha3_ClusterResourceSet(src, dst, nil)
}

func (src *ClusterResourceSetList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.ClusterResourceSetList)

	return Convert_v1alpha3_ClusterResourceSetList_To_v1beta1_ClusterResourceSetList(src, dst, nil)
}

func (dst *ClusterResourceSetList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta1.ClusterResourceSetList)

	return Convert_v1beta1_ClusterResourceSetList_To_v1alpha3_ClusterResourceSetList(src, dst, nil)
}

func (src *ClusterResourceSetBinding) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.ClusterResourceSetBinding)

	return Convert_v1alpha3_ClusterResourceSetBinding_To_v1beta1_ClusterResourceSetBinding(src, dst, nil)
}

func (dst *ClusterResourceSetBinding) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta1.ClusterResourceSetBinding)

	return Convert_v1beta1_ClusterResourceSetBinding_To_v1alpha3_ClusterResourceSetBinding(src, dst, nil)
}

func (src *ClusterResourceSetBindingList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.ClusterResourceSetBindingList)

	return Convert_v1alpha3_ClusterResourceSetBindingList_To_v1beta1_ClusterResourceSetBindingList(src, dst, nil)
}

func (dst *ClusterResourceSetBindingList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta1.ClusterResourceSetBindingList)

	return Convert_v1beta1_ClusterResourceSetBindingList_To_v1alpha3_ClusterResourceSetBindingList(src, dst, nil)
}
