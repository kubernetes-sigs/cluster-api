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

package v1alpha3

import (
	apiconversion "k8s.io/apimachinery/pkg/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	bootstrapv1alpha3 "sigs.k8s.io/cluster-api/internal/apis/bootstrap/kubeadm/v1alpha3"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

func (src *KubeadmControlPlane) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*controlplanev1.KubeadmControlPlane)

	if err := Convert_v1alpha3_KubeadmControlPlane_To_v1beta1_KubeadmControlPlane(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &controlplanev1.KubeadmControlPlane{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	dst.Spec.MachineTemplate.ObjectMeta = restored.Spec.MachineTemplate.ObjectMeta
	dst.Spec.MachineTemplate.NodeDeletionTimeout = restored.Spec.MachineTemplate.NodeDeletionTimeout
	dst.Spec.MachineTemplate.NodeVolumeDetachTimeout = restored.Spec.MachineTemplate.NodeVolumeDetachTimeout
	dst.Spec.RolloutBefore = restored.Spec.RolloutBefore

	if restored.Spec.RemediationStrategy != nil {
		dst.Spec.RemediationStrategy = restored.Spec.RemediationStrategy
	}
	if restored.Status.LastRemediation != nil {
		dst.Status.LastRemediation = restored.Status.LastRemediation
	}

	if restored.Spec.MachineNamingStrategy != nil {
		dst.Spec.MachineNamingStrategy = restored.Spec.MachineNamingStrategy
	}

	bootstrapv1alpha3.MergeRestoredKubeadmConfigSpec(&dst.Spec.KubeadmConfigSpec, &restored.Spec.KubeadmConfigSpec)

	dst.Status.Version = restored.Status.Version
	dst.Status.V1Beta2 = restored.Status.V1Beta2

	return nil
}

func (dst *KubeadmControlPlane) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*controlplanev1.KubeadmControlPlane)

	if err := Convert_v1beta1_KubeadmControlPlane_To_v1alpha3_KubeadmControlPlane(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion except for metadata
	if err := utilconversion.MarshalData(src, dst); err != nil {
		return err
	}

	return nil
}

func (src *KubeadmControlPlaneList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*controlplanev1.KubeadmControlPlaneList)

	return Convert_v1alpha3_KubeadmControlPlaneList_To_v1beta1_KubeadmControlPlaneList(src, dst, nil)
}

func (dst *KubeadmControlPlaneList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*controlplanev1.KubeadmControlPlaneList)

	return Convert_v1beta1_KubeadmControlPlaneList_To_v1alpha3_KubeadmControlPlaneList(src, dst, nil)
}

func Convert_v1beta1_KubeadmControlPlaneSpec_To_v1alpha3_KubeadmControlPlaneSpec(in *controlplanev1.KubeadmControlPlaneSpec, out *KubeadmControlPlaneSpec, s apiconversion.Scope) error {
	out.UpgradeAfter = in.RolloutAfter
	out.InfrastructureTemplate = in.MachineTemplate.InfrastructureRef
	out.NodeDrainTimeout = in.MachineTemplate.NodeDrainTimeout
	return autoConvert_v1beta1_KubeadmControlPlaneSpec_To_v1alpha3_KubeadmControlPlaneSpec(in, out, s)
}

func Convert_v1beta1_KubeadmControlPlaneStatus_To_v1alpha3_KubeadmControlPlaneStatus(in *controlplanev1.KubeadmControlPlaneStatus, out *KubeadmControlPlaneStatus, s apiconversion.Scope) error {
	// NOTE: custom conversion func is required because status.Version does not exist in v1alpha3.
	// .V1Beta2 was added in v1beta1.
	return autoConvert_v1beta1_KubeadmControlPlaneStatus_To_v1alpha3_KubeadmControlPlaneStatus(in, out, s)
}

func Convert_v1alpha3_KubeadmControlPlaneSpec_To_v1beta1_KubeadmControlPlaneSpec(in *KubeadmControlPlaneSpec, out *controlplanev1.KubeadmControlPlaneSpec, s apiconversion.Scope) error {
	out.RolloutAfter = in.UpgradeAfter
	out.MachineTemplate.InfrastructureRef = in.InfrastructureTemplate
	out.MachineTemplate.NodeDrainTimeout = in.NodeDrainTimeout
	return autoConvert_v1alpha3_KubeadmControlPlaneSpec_To_v1beta1_KubeadmControlPlaneSpec(in, out, s)
}

func Convert_v1beta1_KubeadmConfigSpec_To_v1alpha3_KubeadmConfigSpec(in *bootstrapv1.KubeadmConfigSpec, out *bootstrapv1alpha3.KubeadmConfigSpec, s apiconversion.Scope) error {
	return bootstrapv1alpha3.Convert_v1beta1_KubeadmConfigSpec_To_v1alpha3_KubeadmConfigSpec(in, out, s)
}

func Convert_v1alpha3_KubeadmConfigSpec_To_v1beta1_KubeadmConfigSpec(in *bootstrapv1alpha3.KubeadmConfigSpec, out *bootstrapv1.KubeadmConfigSpec, s apiconversion.Scope) error {
	return bootstrapv1alpha3.Convert_v1alpha3_KubeadmConfigSpec_To_v1beta1_KubeadmConfigSpec(in, out, s)
}
