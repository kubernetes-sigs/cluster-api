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

package v1alpha4

import (
	apiconversion "k8s.io/apimachinery/pkg/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	bootstrapv1alpha4 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha4"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

func (src *KubeadmControlPlane) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*controlplanev1.KubeadmControlPlane)

	if err := Convert_v1alpha4_KubeadmControlPlane_To_v1beta1_KubeadmControlPlane(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &controlplanev1.KubeadmControlPlane{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	dst.Spec.KubeadmConfigSpec.Ignition = restored.Spec.KubeadmConfigSpec.Ignition
	if restored.Spec.KubeadmConfigSpec.InitConfiguration != nil {
		if dst.Spec.KubeadmConfigSpec.InitConfiguration == nil {
			dst.Spec.KubeadmConfigSpec.InitConfiguration = &bootstrapv1.InitConfiguration{}
		}
		dst.Spec.KubeadmConfigSpec.InitConfiguration.Patches = restored.Spec.KubeadmConfigSpec.InitConfiguration.Patches
	}
	if restored.Spec.KubeadmConfigSpec.JoinConfiguration != nil {
		if dst.Spec.KubeadmConfigSpec.JoinConfiguration == nil {
			dst.Spec.KubeadmConfigSpec.JoinConfiguration = &bootstrapv1.JoinConfiguration{}
		}
		dst.Spec.KubeadmConfigSpec.JoinConfiguration.Patches = restored.Spec.KubeadmConfigSpec.JoinConfiguration.Patches
	}

	return nil
}

func (dst *KubeadmControlPlane) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*controlplanev1.KubeadmControlPlane)

	if err := Convert_v1beta1_KubeadmControlPlane_To_v1alpha4_KubeadmControlPlane(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion except for metadata
	return utilconversion.MarshalData(src, dst)
}

func (src *KubeadmControlPlaneList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*controlplanev1.KubeadmControlPlaneList)

	return Convert_v1alpha4_KubeadmControlPlaneList_To_v1beta1_KubeadmControlPlaneList(src, dst, nil)
}

func (dst *KubeadmControlPlaneList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*controlplanev1.KubeadmControlPlaneList)

	return Convert_v1beta1_KubeadmControlPlaneList_To_v1alpha4_KubeadmControlPlaneList(src, dst, nil)
}

func (src *KubeadmControlPlaneTemplate) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*controlplanev1.KubeadmControlPlaneTemplate)

	if err := Convert_v1alpha4_KubeadmControlPlaneTemplate_To_v1beta1_KubeadmControlPlaneTemplate(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &controlplanev1.KubeadmControlPlaneTemplate{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	dst.Spec.Template.Spec.KubeadmConfigSpec.Ignition = restored.Spec.Template.Spec.KubeadmConfigSpec.Ignition
	dst.Spec.Template.Spec.MachineTemplate = restored.Spec.Template.Spec.MachineTemplate
	if restored.Spec.Template.Spec.KubeadmConfigSpec.InitConfiguration != nil {
		if dst.Spec.Template.Spec.KubeadmConfigSpec.InitConfiguration == nil {
			dst.Spec.Template.Spec.KubeadmConfigSpec.InitConfiguration = &bootstrapv1.InitConfiguration{}
		}
		dst.Spec.Template.Spec.KubeadmConfigSpec.InitConfiguration.Patches = restored.Spec.Template.Spec.KubeadmConfigSpec.InitConfiguration.Patches
	}
	if restored.Spec.Template.Spec.KubeadmConfigSpec.JoinConfiguration != nil {
		if dst.Spec.Template.Spec.KubeadmConfigSpec.JoinConfiguration == nil {
			dst.Spec.Template.Spec.KubeadmConfigSpec.JoinConfiguration = &bootstrapv1.JoinConfiguration{}
		}
		dst.Spec.Template.Spec.KubeadmConfigSpec.JoinConfiguration.Patches = restored.Spec.Template.Spec.KubeadmConfigSpec.JoinConfiguration.Patches
	}

	return nil
}

func (dst *KubeadmControlPlaneTemplate) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*controlplanev1.KubeadmControlPlaneTemplate)

	if err := Convert_v1beta1_KubeadmControlPlaneTemplate_To_v1alpha4_KubeadmControlPlaneTemplate(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion except for metadata
	return utilconversion.MarshalData(src, dst)
}

func (src *KubeadmControlPlaneTemplateList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*controlplanev1.KubeadmControlPlaneTemplateList)

	return Convert_v1alpha4_KubeadmControlPlaneTemplateList_To_v1beta1_KubeadmControlPlaneTemplateList(src, dst, nil)
}

func (dst *KubeadmControlPlaneTemplateList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*controlplanev1.KubeadmControlPlaneTemplateList)

	return Convert_v1beta1_KubeadmControlPlaneTemplateList_To_v1alpha4_KubeadmControlPlaneTemplateList(src, dst, nil)
}

func Convert_v1alpha4_KubeadmControlPlaneSpec_To_v1beta1_KubeadmControlPlaneTemplateResourceSpec(in *KubeadmControlPlaneSpec, out *controlplanev1.KubeadmControlPlaneTemplateResourceSpec, s apiconversion.Scope) error {
	out.MachineTemplate = &controlplanev1.KubeadmControlPlaneTemplateMachineTemplate{
		NodeDrainTimeout: in.MachineTemplate.NodeDrainTimeout,
	}

	if err := bootstrapv1alpha4.Convert_v1alpha4_KubeadmConfigSpec_To_v1beta1_KubeadmConfigSpec(&in.KubeadmConfigSpec, &out.KubeadmConfigSpec, s); err != nil {
		return err
	}

	out.RolloutAfter = in.RolloutAfter

	if in.RolloutStrategy != nil {
		out.RolloutStrategy = &controlplanev1.RolloutStrategy{}
		if len(in.RolloutStrategy.Type) > 0 {
			out.RolloutStrategy.Type = controlplanev1.RolloutStrategyType(in.RolloutStrategy.Type)
		}
		if in.RolloutStrategy.RollingUpdate != nil {
			out.RolloutStrategy.RollingUpdate = &controlplanev1.RollingUpdate{}

			if in.RolloutStrategy.RollingUpdate.MaxSurge != nil {
				out.RolloutStrategy.RollingUpdate.MaxSurge = in.RolloutStrategy.RollingUpdate.MaxSurge
			}
		}
	}

	return nil
}

func Convert_v1beta1_KubeadmControlPlaneTemplateResourceSpec_To_v1alpha4_KubeadmControlPlaneSpec(in *controlplanev1.KubeadmControlPlaneTemplateResourceSpec, out *KubeadmControlPlaneSpec, s apiconversion.Scope) error {
	if in.MachineTemplate != nil {
		out.MachineTemplate.NodeDrainTimeout = in.MachineTemplate.NodeDrainTimeout
	}

	if err := bootstrapv1alpha4.Convert_v1beta1_KubeadmConfigSpec_To_v1alpha4_KubeadmConfigSpec(&in.KubeadmConfigSpec, &out.KubeadmConfigSpec, s); err != nil {
		return err
	}

	out.RolloutAfter = in.RolloutAfter

	if in.RolloutStrategy != nil {
		out.RolloutStrategy = &RolloutStrategy{}
		if len(in.RolloutStrategy.Type) > 0 {
			out.RolloutStrategy.Type = RolloutStrategyType(in.RolloutStrategy.Type)
		}
		if in.RolloutStrategy.RollingUpdate != nil {
			out.RolloutStrategy.RollingUpdate = &RollingUpdate{}

			if in.RolloutStrategy.RollingUpdate.MaxSurge != nil {
				out.RolloutStrategy.RollingUpdate.MaxSurge = in.RolloutStrategy.RollingUpdate.MaxSurge
			}
		}
	}

	return nil
}
