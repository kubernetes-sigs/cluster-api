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
	bootstrapv1beta1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta2"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta2"
)

func (src *KubeadmControlPlane) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*controlplanev1.KubeadmControlPlane)

	return Convert_v1beta1_KubeadmControlPlane_To_v1beta2_KubeadmControlPlane(src, dst, nil)
}

func (dst *KubeadmControlPlane) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*controlplanev1.KubeadmControlPlane)

	return Convert_v1beta2_KubeadmControlPlane_To_v1beta1_KubeadmControlPlane(src, dst, nil)
}

func (src *KubeadmControlPlaneTemplate) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*controlplanev1.KubeadmControlPlaneTemplate)

	return Convert_v1beta1_KubeadmControlPlaneTemplate_To_v1beta2_KubeadmControlPlaneTemplate(src, dst, nil)
}

func (dst *KubeadmControlPlaneTemplate) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*controlplanev1.KubeadmControlPlaneTemplate)

	return Convert_v1beta2_KubeadmControlPlaneTemplate_To_v1beta1_KubeadmControlPlaneTemplate(src, dst, nil)
}

// Implement local conversion func because conversion-gen is not aware of conversion func in other packages (see https://github.com/kubernetes/code-generator/issues/94)

func Convert_v1beta1_ObjectMeta_To_v1beta2_ObjectMeta(in *clusterv1beta1.ObjectMeta, out *clusterv1.ObjectMeta, s apiconversion.Scope) error {
	return clusterv1beta1.Convert_v1beta1_ObjectMeta_To_v1beta2_ObjectMeta(in, out, s)
}

func Convert_v1beta2_ObjectMeta_To_v1beta1_ObjectMeta(in *clusterv1.ObjectMeta, out *clusterv1beta1.ObjectMeta, s apiconversion.Scope) error {
	return clusterv1beta1.Convert_v1beta2_ObjectMeta_To_v1beta1_ObjectMeta(in, out, s)
}

func Convert_v1beta1_KubeadmConfigSpec_To_v1beta2_KubeadmConfigSpec(in *bootstrapv1beta1.KubeadmConfigSpec, out *bootstrapv1.KubeadmConfigSpec, s apiconversion.Scope) error {
	return bootstrapv1beta1.Convert_v1beta1_KubeadmConfigSpec_To_v1beta2_KubeadmConfigSpec(in, out, s)
}

func Convert_v1beta2_KubeadmConfigSpec_To_v1beta1_KubeadmConfigSpec(in *bootstrapv1.KubeadmConfigSpec, out *bootstrapv1beta1.KubeadmConfigSpec, s apiconversion.Scope) error {
	return bootstrapv1beta1.Convert_v1beta2_KubeadmConfigSpec_To_v1beta1_KubeadmConfigSpec(in, out, s)
}
