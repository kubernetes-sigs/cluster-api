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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apimachineryconversion "k8s.io/apimachinery/pkg/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta2"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta2"
	clusterv1alpha4 "sigs.k8s.io/cluster-api/internal/apis/core/v1alpha4"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

func (src *MachinePool) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*expv1.MachinePool)

	if err := Convert_v1alpha4_MachinePool_To_v1beta2_MachinePool(src, dst, nil); err != nil {
		return err
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1alpha4 conditions should not be automatically be converted into v1beta2 conditions.
	dst.Status.Conditions = nil

	// Move legacy conditions (v1alpha4), failureReason, failureMessage and replica counters to the deprecated field.
	dst.Status.Deprecated = &expv1.MachinePoolDeprecatedStatus{}
	dst.Status.Deprecated.V1Beta1 = &expv1.MachinePoolV1Beta1DeprecatedStatus{}
	if src.Status.Conditions != nil {
		clusterv1alpha4.Convert_v1alpha4_Conditions_To_v1beta2_Deprecated_V1Beta1_Conditions(&src.Status.Conditions, &dst.Status.Deprecated.V1Beta1.Conditions)
	}
	dst.Status.Deprecated.V1Beta1.FailureReason = src.Status.FailureReason
	dst.Status.Deprecated.V1Beta1.FailureMessage = src.Status.FailureMessage
	dst.Status.Deprecated.V1Beta1.ReadyReplicas = src.Status.ReadyReplicas
	dst.Status.Deprecated.V1Beta1.AvailableReplicas = src.Status.AvailableReplicas
	dst.Status.Deprecated.V1Beta1.UnavailableReplicas = src.Status.UnavailableReplicas

	// Manually restore data.
	restored := &expv1.MachinePool{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}
	dst.Spec.Template.Spec.ReadinessGates = restored.Spec.Template.Spec.ReadinessGates
	dst.Spec.Template.Spec.NodeDeletionTimeout = restored.Spec.Template.Spec.NodeDeletionTimeout
	dst.Spec.Template.Spec.NodeVolumeDetachTimeout = restored.Spec.Template.Spec.NodeVolumeDetachTimeout
	dst.Status.Conditions = restored.Status.Conditions
	dst.Status.AvailableReplicas = restored.Status.AvailableReplicas
	dst.Status.ReadyReplicas = restored.Status.ReadyReplicas
	dst.Status.UpToDateReplicas = restored.Status.UpToDateReplicas

	return nil
}

func (dst *MachinePool) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*expv1.MachinePool)

	if err := Convert_v1beta2_MachinePool_To_v1alpha4_MachinePool(src, dst, nil); err != nil {
		return err
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta2 conditions should not be automatically be converted into legacy conditions (v1alpha4).
	dst.Status.Conditions = nil

	// Reset replica counters from autogenerated conversions
	// NOTE: replica counters with a new semantic should not be automatically be converted into old replica counters.
	dst.Status.ReadyReplicas = 0
	dst.Status.AvailableReplicas = 0

	// Retrieve legacy conditions (v1alpha4), failureReason, failureMessage and replica counters from the deprecated field.
	if src.Status.Deprecated != nil {
		if src.Status.Deprecated.V1Beta1 != nil {
			if src.Status.Deprecated.V1Beta1.Conditions != nil {
				clusterv1alpha4.Convert_v1beta2_Deprecated_V1Beta1_Conditions_To_v1alpha4_Conditions(&src.Status.Deprecated.V1Beta1.Conditions, &dst.Status.Conditions)
			}
			dst.Status.FailureReason = src.Status.Deprecated.V1Beta1.FailureReason
			dst.Status.FailureMessage = src.Status.Deprecated.V1Beta1.FailureMessage
			dst.Status.ReadyReplicas = src.Status.Deprecated.V1Beta1.ReadyReplicas
			dst.Status.AvailableReplicas = src.Status.Deprecated.V1Beta1.AvailableReplicas
			dst.Status.UnavailableReplicas = src.Status.Deprecated.V1Beta1.UnavailableReplicas
		}
	}

	// Preserve Hub data on down-conversion except for metadata
	return utilconversion.MarshalData(src, dst)
}

func Convert_v1alpha4_MachineTemplateSpec_To_v1beta2_MachineTemplateSpec(in *clusterv1alpha4.MachineTemplateSpec, out *clusterv1.MachineTemplateSpec, s apimachineryconversion.Scope) error {
	return clusterv1alpha4.Convert_v1alpha4_MachineTemplateSpec_To_v1beta2_MachineTemplateSpec(in, out, s)
}

func Convert_v1beta2_MachineTemplateSpec_To_v1alpha4_MachineTemplateSpec(in *clusterv1.MachineTemplateSpec, out *clusterv1alpha4.MachineTemplateSpec, s apimachineryconversion.Scope) error {
	return clusterv1alpha4.Convert_v1beta2_MachineTemplateSpec_To_v1alpha4_MachineTemplateSpec(in, out, s)
}

func Convert_v1beta2_MachinePoolStatus_To_v1alpha4_MachinePoolStatus(in *expv1.MachinePoolStatus, out *MachinePoolStatus, s apimachineryconversion.Scope) error {
	// V1Beta2 was added in v1beta1
	return autoConvert_v1beta2_MachinePoolStatus_To_v1alpha4_MachinePoolStatus(in, out, s)
}

func Convert_v1alpha4_MachinePoolStatus_To_v1beta2_MachinePoolStatus(in *MachinePoolStatus, out *expv1.MachinePoolStatus, scope apimachineryconversion.Scope) error {
	return autoConvert_v1alpha4_MachinePoolStatus_To_v1beta2_MachinePoolStatus(in, out, scope)
}

// Implement local conversion func because conversion-gen is not aware of conversion func in other packages (see https://github.com/kubernetes/code-generator/issues/94)

func Convert_v1_Condition_To_v1alpha4_Condition(in *metav1.Condition, out *clusterv1alpha4.Condition, s apimachineryconversion.Scope) error {
	return clusterv1alpha4.Convert_v1_Condition_To_v1alpha4_Condition(in, out, s)
}

func Convert_v1alpha4_Condition_To_v1_Condition(in *clusterv1alpha4.Condition, out *metav1.Condition, s apimachineryconversion.Scope) error {
	return clusterv1alpha4.Convert_v1alpha4_Condition_To_v1_Condition(in, out, s)
}
