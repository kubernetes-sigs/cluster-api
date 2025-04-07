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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apimachineryconversion "k8s.io/apimachinery/pkg/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta2"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta2"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta2"
	bootstrapv1alpha4 "sigs.k8s.io/cluster-api/internal/apis/bootstrap/kubeadm/v1alpha4"
	clusterv1alpha4 "sigs.k8s.io/cluster-api/internal/apis/core/v1alpha4"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

func (src *KubeadmControlPlane) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*controlplanev1.KubeadmControlPlane)

	if err := Convert_v1alpha4_KubeadmControlPlane_To_v1beta2_KubeadmControlPlane(src, dst, nil); err != nil {
		return err
	}

	// Move legacy conditions (v1alpha4), failureReason, failureMessage and replica counters to the deprecated field.
	dst.Status.Deprecated = &controlplanev1.KubeadmControlPlaneDeprecatedStatus{}
	dst.Status.Deprecated.V1Beta1 = &controlplanev1.KubeadmControlPlaneV1Beta1DeprecatedStatus{}
	if src.Status.Conditions != nil {
		clusterv1alpha4.Convert_Deprecated_v1alpha4_Conditions_To_v1beta2_Conditions(&src.Status.Conditions, &dst.Status.Deprecated.V1Beta1.Conditions)
	}
	dst.Status.Deprecated.V1Beta1.FailureReason = src.Status.FailureReason
	dst.Status.Deprecated.V1Beta1.FailureMessage = src.Status.FailureMessage
	dst.Status.Deprecated.V1Beta1.ReadyReplicas = src.Status.ReadyReplicas
	dst.Status.Deprecated.V1Beta1.UpdatedReplicas = src.Status.UpdatedReplicas
	dst.Status.Deprecated.V1Beta1.UnavailableReplicas = src.Status.UnavailableReplicas

	// Manually restore data.
	restored := &controlplanev1.KubeadmControlPlane{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	dst.Spec.MachineTemplate.ReadinessGates = restored.Spec.MachineTemplate.ReadinessGates
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

	bootstrapv1alpha4.MergeRestoredKubeadmConfigSpec(&dst.Spec.KubeadmConfigSpec, &restored.Spec.KubeadmConfigSpec)
	dst.Status.Conditions = restored.Status.Conditions
	dst.Status.AvailableReplicas = restored.Status.AvailableReplicas
	dst.Status.ReadyReplicas = restored.Status.ReadyReplicas
	dst.Status.UpToDateReplicas = restored.Status.UpToDateReplicas

	return nil
}

func (dst *KubeadmControlPlane) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*controlplanev1.KubeadmControlPlane)

	if err := Convert_v1beta2_KubeadmControlPlane_To_v1alpha4_KubeadmControlPlane(src, dst, nil); err != nil {
		return err
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta2 conditions should not be automatically be converted into legacy conditions (v1alpha4).
	dst.Status.Conditions = nil

	// Reset replica counters from autogenerated conversions
	// NOTE: replica counters with a new semantic should not be automatically be converted into old replica counters.
	dst.Status.ReadyReplicas = 0

	// Retrieve legacy conditions (v1alpha4), failureReason, failureMessage and replica counters from the deprecated field.
	if src.Status.Deprecated != nil {
		if src.Status.Deprecated.V1Beta1 != nil {
			if src.Status.Deprecated.V1Beta1.Conditions != nil {
				clusterv1alpha4.Convert_v1beta2_Conditions_To_Deprecated_v1alpha4_Conditions(&src.Status.Deprecated.V1Beta1.Conditions, &dst.Status.Conditions)
			}
			dst.Status.FailureReason = src.Status.Deprecated.V1Beta1.FailureReason
			dst.Status.FailureMessage = src.Status.Deprecated.V1Beta1.FailureMessage
			dst.Status.ReadyReplicas = src.Status.Deprecated.V1Beta1.ReadyReplicas
			dst.Status.UpdatedReplicas = src.Status.Deprecated.V1Beta1.UpdatedReplicas
			dst.Status.UnavailableReplicas = src.Status.Deprecated.V1Beta1.UnavailableReplicas
		}
	}

	// Preserve Hub data on down-conversion except for metadata
	return utilconversion.MarshalData(src, dst)
}

func (src *KubeadmControlPlaneTemplate) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*controlplanev1.KubeadmControlPlaneTemplate)

	if err := Convert_v1alpha4_KubeadmControlPlaneTemplate_To_v1beta2_KubeadmControlPlaneTemplate(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &controlplanev1.KubeadmControlPlaneTemplate{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	dst.Spec.Template.Spec.MachineTemplate = restored.Spec.Template.Spec.MachineTemplate

	dst.Spec.Template.ObjectMeta = restored.Spec.Template.ObjectMeta
	if restored.Spec.Template.Spec.MachineTemplate != nil {
		dst.Spec.Template.Spec.MachineTemplate.ObjectMeta = restored.Spec.Template.Spec.MachineTemplate.ObjectMeta
	}

	if dst.Spec.Template.Spec.MachineTemplate == nil {
		dst.Spec.Template.Spec.MachineTemplate = restored.Spec.Template.Spec.MachineTemplate
	} else if restored.Spec.Template.Spec.MachineTemplate != nil {
		dst.Spec.Template.Spec.MachineTemplate.NodeDeletionTimeout = restored.Spec.Template.Spec.MachineTemplate.NodeDeletionTimeout
		dst.Spec.Template.Spec.MachineTemplate.NodeVolumeDetachTimeout = restored.Spec.Template.Spec.MachineTemplate.NodeVolumeDetachTimeout
	}

	dst.Spec.Template.Spec.RolloutBefore = restored.Spec.Template.Spec.RolloutBefore

	if restored.Spec.Template.Spec.RemediationStrategy != nil {
		dst.Spec.Template.Spec.RemediationStrategy = restored.Spec.Template.Spec.RemediationStrategy
	}

	if restored.Spec.Template.Spec.MachineNamingStrategy != nil {
		dst.Spec.Template.Spec.MachineNamingStrategy = restored.Spec.Template.Spec.MachineNamingStrategy
	}

	bootstrapv1alpha4.MergeRestoredKubeadmConfigSpec(&dst.Spec.Template.Spec.KubeadmConfigSpec, &restored.Spec.Template.Spec.KubeadmConfigSpec)

	return nil
}

func (dst *KubeadmControlPlaneTemplate) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*controlplanev1.KubeadmControlPlaneTemplate)

	if err := Convert_v1beta2_KubeadmControlPlaneTemplate_To_v1alpha4_KubeadmControlPlaneTemplate(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion except for metadata
	return utilconversion.MarshalData(src, dst)
}

func Convert_v1alpha4_KubeadmControlPlaneSpec_To_v1beta2_KubeadmControlPlaneTemplateResourceSpec(in *KubeadmControlPlaneSpec, out *controlplanev1.KubeadmControlPlaneTemplateResourceSpec, s apimachineryconversion.Scope) error {
	out.MachineTemplate = &controlplanev1.KubeadmControlPlaneTemplateMachineTemplate{
		NodeDrainTimeout: in.MachineTemplate.NodeDrainTimeout,
	}

	if err := bootstrapv1alpha4.Convert_v1alpha4_KubeadmConfigSpec_To_v1beta2_KubeadmConfigSpec(&in.KubeadmConfigSpec, &out.KubeadmConfigSpec, s); err != nil {
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

func Convert_v1beta2_KubeadmControlPlaneTemplateResourceSpec_To_v1alpha4_KubeadmControlPlaneSpec(in *controlplanev1.KubeadmControlPlaneTemplateResourceSpec, out *KubeadmControlPlaneSpec, s apimachineryconversion.Scope) error {
	if in.MachineTemplate != nil {
		out.MachineTemplate.NodeDrainTimeout = in.MachineTemplate.NodeDrainTimeout
	}

	if err := bootstrapv1alpha4.Convert_v1beta2_KubeadmConfigSpec_To_v1alpha4_KubeadmConfigSpec(&in.KubeadmConfigSpec, &out.KubeadmConfigSpec, s); err != nil {
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

func Convert_v1beta2_KubeadmControlPlaneMachineTemplate_To_v1alpha4_KubeadmControlPlaneMachineTemplate(in *controlplanev1.KubeadmControlPlaneMachineTemplate, out *KubeadmControlPlaneMachineTemplate, s apimachineryconversion.Scope) error {
	// .NodeDrainTimeout was added in v1beta1.
	return autoConvert_v1beta2_KubeadmControlPlaneMachineTemplate_To_v1alpha4_KubeadmControlPlaneMachineTemplate(in, out, s)
}

func Convert_v1beta2_KubeadmControlPlaneSpec_To_v1alpha4_KubeadmControlPlaneSpec(in *controlplanev1.KubeadmControlPlaneSpec, out *KubeadmControlPlaneSpec, scope apimachineryconversion.Scope) error {
	// .RolloutBefore was added in v1beta1.
	// .RemediationStrategy was added in v1beta1.
	return autoConvert_v1beta2_KubeadmControlPlaneSpec_To_v1alpha4_KubeadmControlPlaneSpec(in, out, scope)
}

func Convert_v1beta2_KubeadmControlPlaneStatus_To_v1alpha4_KubeadmControlPlaneStatus(in *controlplanev1.KubeadmControlPlaneStatus, out *KubeadmControlPlaneStatus, scope apimachineryconversion.Scope) error {
	// .LastRemediation was added in v1beta1.
	// .V1Beta2 was added in v1beta1.
	return autoConvert_v1beta2_KubeadmControlPlaneStatus_To_v1alpha4_KubeadmControlPlaneStatus(in, out, scope)
}

func Convert_v1beta2_KubeadmControlPlaneTemplateResource_To_v1alpha4_KubeadmControlPlaneTemplateResource(in *controlplanev1.KubeadmControlPlaneTemplateResource, out *KubeadmControlPlaneTemplateResource, scope apimachineryconversion.Scope) error {
	// .metadata and .spec.machineTemplate.metadata was added in v1beta1.
	return autoConvert_v1beta2_KubeadmControlPlaneTemplateResource_To_v1alpha4_KubeadmControlPlaneTemplateResource(in, out, scope)
}

func Convert_v1alpha4_KubeadmControlPlaneStatus_To_v1beta2_KubeadmControlPlaneStatus(in *KubeadmControlPlaneStatus, out *controlplanev1.KubeadmControlPlaneStatus, scope apimachineryconversion.Scope) error {
	return autoConvert_v1alpha4_KubeadmControlPlaneStatus_To_v1beta2_KubeadmControlPlaneStatus(in, out, scope)
}

// Implement local conversion func because conversion-gen is not aware of conversion func in other packages (see https://github.com/kubernetes/code-generator/issues/94)

func Convert_v1beta2_KubeadmConfigSpec_To_v1alpha4_KubeadmConfigSpec(in *bootstrapv1.KubeadmConfigSpec, out *bootstrapv1alpha4.KubeadmConfigSpec, s apimachineryconversion.Scope) error {
	return bootstrapv1alpha4.Convert_v1beta2_KubeadmConfigSpec_To_v1alpha4_KubeadmConfigSpec(in, out, s)
}

func Convert_v1alpha4_KubeadmConfigSpec_To_v1beta2_KubeadmConfigSpec(in *bootstrapv1alpha4.KubeadmConfigSpec, out *bootstrapv1.KubeadmConfigSpec, s apimachineryconversion.Scope) error {
	return bootstrapv1alpha4.Convert_v1alpha4_KubeadmConfigSpec_To_v1beta2_KubeadmConfigSpec(in, out, s)
}

func Convert_v1alpha4_ObjectMeta_To_v1beta2_ObjectMeta(in *clusterv1alpha4.ObjectMeta, out *clusterv1.ObjectMeta, s apimachineryconversion.Scope) error {
	return clusterv1alpha4.Convert_v1alpha4_ObjectMeta_To_v1beta2_ObjectMeta(in, out, s)
}

func Convert_v1beta2_ObjectMeta_To_v1alpha4_ObjectMeta(in *clusterv1.ObjectMeta, out *clusterv1alpha4.ObjectMeta, s apimachineryconversion.Scope) error {
	return clusterv1alpha4.Convert_v1beta2_ObjectMeta_To_v1alpha4_ObjectMeta(in, out, s)
}

func Convert_v1_Condition_To_v1alpha4_Condition(in *metav1.Condition, out *clusterv1alpha4.Condition, s apimachineryconversion.Scope) error {
	return clusterv1alpha4.Convert_v1_Condition_To_v1alpha4_Condition(in, out, s)
}

func Convert_v1alpha4_Condition_To_v1_Condition(in *clusterv1alpha4.Condition, out *metav1.Condition, s apimachineryconversion.Scope) error {
	return clusterv1alpha4.Convert_v1alpha4_Condition_To_v1_Condition(in, out, s)
}
