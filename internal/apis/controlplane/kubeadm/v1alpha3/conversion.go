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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apimachineryconversion "k8s.io/apimachinery/pkg/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta2"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta2"
	bootstrapv1alpha3 "sigs.k8s.io/cluster-api/internal/apis/bootstrap/kubeadm/v1alpha3"
	clusterv1alpha3 "sigs.k8s.io/cluster-api/internal/apis/core/v1alpha3"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

func (src *KubeadmControlPlane) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*controlplanev1.KubeadmControlPlane)

	if err := Convert_v1alpha3_KubeadmControlPlane_To_v1beta2_KubeadmControlPlane(src, dst, nil); err != nil {
		return err
	}

	// Move legacy conditions (v1alpha3), failureReason, failureMessage and replica counters to the deprecated field.
	dst.Status.Deprecated = &controlplanev1.KubeadmControlPlaneDeprecatedStatus{}
	dst.Status.Deprecated.V1Beta1 = &controlplanev1.KubeadmControlPlaneV1Beta1DeprecatedStatus{}
	if src.Status.Conditions != nil {
		clusterv1alpha3.Convert_Deprecated_v1alpha3_Conditions_To_v1beta2_Conditions(&src.Status.Conditions, &dst.Status.Deprecated.V1Beta1.Conditions)
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

	dst.Spec.MachineTemplate.ObjectMeta = restored.Spec.MachineTemplate.ObjectMeta
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

	bootstrapv1alpha3.MergeRestoredKubeadmConfigSpec(&dst.Spec.KubeadmConfigSpec, &restored.Spec.KubeadmConfigSpec)

	dst.Status.Version = restored.Status.Version
	dst.Status.Conditions = restored.Status.Conditions
	dst.Status.AvailableReplicas = restored.Status.AvailableReplicas
	dst.Status.ReadyReplicas = restored.Status.ReadyReplicas
	dst.Status.UpToDateReplicas = restored.Status.UpToDateReplicas

	return nil
}

func (dst *KubeadmControlPlane) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*controlplanev1.KubeadmControlPlane)

	if err := Convert_v1beta2_KubeadmControlPlane_To_v1alpha3_KubeadmControlPlane(src, dst, nil); err != nil {
		return err
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta2 conditions should not be automatically be converted into legacy conditions (v1alpha3).
	dst.Status.Conditions = nil

	// Reset replica counters from autogenerated conversions
	// NOTE: replica counters with a new semantic should not be automatically be converted into old replica counters.
	dst.Status.ReadyReplicas = 0

	// Retrieve legacy conditions (v1alpha3), failureReason, failureMessage and replica counters from the deprecated field.
	if src.Status.Deprecated != nil {
		if src.Status.Deprecated.V1Beta1 != nil {
			if src.Status.Deprecated.V1Beta1.Conditions != nil {
				clusterv1alpha3.Convert_v1beta2_Conditions_To_Deprecated_v1alpha3_Conditions(&src.Status.Deprecated.V1Beta1.Conditions, &dst.Status.Conditions)
			}
			dst.Status.FailureReason = src.Status.Deprecated.V1Beta1.FailureReason
			dst.Status.FailureMessage = src.Status.Deprecated.V1Beta1.FailureMessage
			dst.Status.ReadyReplicas = src.Status.Deprecated.V1Beta1.ReadyReplicas
			dst.Status.UpdatedReplicas = src.Status.Deprecated.V1Beta1.UpdatedReplicas
			dst.Status.UnavailableReplicas = src.Status.Deprecated.V1Beta1.UnavailableReplicas
		}
	}

	// Preserve Hub data on down-conversion except for metadata
	if err := utilconversion.MarshalData(src, dst); err != nil {
		return err
	}

	return nil
}

func Convert_v1beta2_KubeadmControlPlaneSpec_To_v1alpha3_KubeadmControlPlaneSpec(in *controlplanev1.KubeadmControlPlaneSpec, out *KubeadmControlPlaneSpec, s apimachineryconversion.Scope) error {
	out.UpgradeAfter = in.RolloutAfter
	out.InfrastructureTemplate = in.MachineTemplate.InfrastructureRef
	out.NodeDrainTimeout = in.MachineTemplate.NodeDrainTimeout
	return autoConvert_v1beta2_KubeadmControlPlaneSpec_To_v1alpha3_KubeadmControlPlaneSpec(in, out, s)
}

func Convert_v1beta2_KubeadmControlPlaneStatus_To_v1alpha3_KubeadmControlPlaneStatus(in *controlplanev1.KubeadmControlPlaneStatus, out *KubeadmControlPlaneStatus, s apimachineryconversion.Scope) error {
	// NOTE: custom conversion func is required because status.Version does not exist in v1alpha3.
	// .V1Beta2 was added in v1beta1.
	return autoConvert_v1beta2_KubeadmControlPlaneStatus_To_v1alpha3_KubeadmControlPlaneStatus(in, out, s)
}

func Convert_v1alpha3_KubeadmControlPlaneSpec_To_v1beta2_KubeadmControlPlaneSpec(in *KubeadmControlPlaneSpec, out *controlplanev1.KubeadmControlPlaneSpec, s apimachineryconversion.Scope) error {
	out.RolloutAfter = in.UpgradeAfter
	out.MachineTemplate.InfrastructureRef = in.InfrastructureTemplate
	out.MachineTemplate.NodeDrainTimeout = in.NodeDrainTimeout
	return autoConvert_v1alpha3_KubeadmControlPlaneSpec_To_v1beta2_KubeadmControlPlaneSpec(in, out, s)
}

func Convert_v1alpha3_KubeadmControlPlaneStatus_To_v1beta2_KubeadmControlPlaneStatus(in *KubeadmControlPlaneStatus, out *controlplanev1.KubeadmControlPlaneStatus, s apimachineryconversion.Scope) error {
	return autoConvert_v1alpha3_KubeadmControlPlaneStatus_To_v1beta2_KubeadmControlPlaneStatus(in, out, s)
}

// Implement local conversion func because conversion-gen is not aware of conversion func in other packages (see https://github.com/kubernetes/code-generator/issues/94)

func Convert_v1_Condition_To_v1alpha3_Condition(_ *metav1.Condition, _ *clusterv1alpha3.Condition, _ apimachineryconversion.Scope) error {
	// NOTE: v1beta2 conditions should not be automatically converted into v1beta1 conditions.
	return nil
}

func Convert_v1alpha3_Condition_To_v1_Condition(_ *clusterv1alpha3.Condition, _ *metav1.Condition, _ apimachineryconversion.Scope) error {
	// NOTE: v1beta1 conditions should not be automatically converted into v1beta2 conditions.
	return nil
}

func Convert_v1beta2_KubeadmConfigSpec_To_v1alpha3_KubeadmConfigSpec(in *bootstrapv1.KubeadmConfigSpec, out *bootstrapv1alpha3.KubeadmConfigSpec, s apimachineryconversion.Scope) error {
	return bootstrapv1alpha3.Convert_v1beta2_KubeadmConfigSpec_To_v1alpha3_KubeadmConfigSpec(in, out, s)
}

func Convert_v1alpha3_KubeadmConfigSpec_To_v1beta2_KubeadmConfigSpec(in *bootstrapv1alpha3.KubeadmConfigSpec, out *bootstrapv1.KubeadmConfigSpec, s apimachineryconversion.Scope) error {
	return bootstrapv1alpha3.Convert_v1alpha3_KubeadmConfigSpec_To_v1beta2_KubeadmConfigSpec(in, out, s)
}
