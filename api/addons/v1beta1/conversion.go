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
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	addonsv1 "sigs.k8s.io/cluster-api/api/addons/v1beta2"
)

func (src *ClusterResourceSet) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*addonsv1.ClusterResourceSet)

	return Convert_v1beta1_ClusterResourceSet_To_v1beta2_ClusterResourceSet(src, dst, nil)
}

func (dst *ClusterResourceSet) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*addonsv1.ClusterResourceSet)

	return Convert_v1beta2_ClusterResourceSet_To_v1beta1_ClusterResourceSet(src, dst, nil)
}

func (src *ClusterResourceSetBinding) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*addonsv1.ClusterResourceSetBinding)

	return Convert_v1beta1_ClusterResourceSetBinding_To_v1beta2_ClusterResourceSetBinding(src, dst, nil)
}

func (dst *ClusterResourceSetBinding) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*addonsv1.ClusterResourceSetBinding)

	return Convert_v1beta2_ClusterResourceSetBinding_To_v1beta1_ClusterResourceSetBinding(src, dst, nil)
}
