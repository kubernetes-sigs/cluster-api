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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apimachineryconversion "k8s.io/apimachinery/pkg/conversion"
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

func Convert_v1beta2_MachinePoolStatus_To_v1beta1_MachinePoolStatus(in *expv1.MachinePoolStatus, out *MachinePoolStatus, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_MachinePoolStatus_To_v1beta1_MachinePoolStatus(in, out, s); err != nil {
		return err
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta2 conditions should not be automatically be converted into legacy conditions (v1beta1).
	out.Conditions = nil

	// Reset replica counters from autogenerated conversions
	// NOTE: replica counters with a new semantic should not be automatically be converted into old replica counters.
	out.ReadyReplicas = 0
	out.AvailableReplicas = 0

	// Retrieve legacy conditions (v1beta1), failureReason and failureMessage from the deprecated field.
	if in.Deprecated != nil && in.Deprecated.V1Beta1 != nil {
		if in.Deprecated.V1Beta1.Conditions != nil {
			clusterv1beta1.Convert_v1beta2_Conditions_To_Deprecated_v1beta1_Conditions(&in.Deprecated.V1Beta1.Conditions, &out.Conditions)
		}
		out.FailureReason = in.Deprecated.V1Beta1.FailureReason
		out.FailureMessage = in.Deprecated.V1Beta1.FailureMessage
		out.ReadyReplicas = in.Deprecated.V1Beta1.ReadyReplicas
		out.AvailableReplicas = in.Deprecated.V1Beta1.AvailableReplicas
		out.UnavailableReplicas = in.Deprecated.V1Beta1.UnavailableReplicas
	}

	// Move new conditions (v1beta2) to the v1beta2 field.
	if in.Conditions == nil {
		return nil
	}
	out.V1Beta2 = &MachinePoolV1Beta2Status{}
	out.V1Beta2.Conditions = in.Conditions
	out.V1Beta2.ReadyReplicas = in.ReadyReplicas
	out.V1Beta2.AvailableReplicas = in.AvailableReplicas
	out.V1Beta2.UpToDateReplicas = in.UpToDateReplicas
	return nil
}

func Convert_v1beta1_MachinePoolStatus_To_v1beta2_MachinePoolStatus(in *MachinePoolStatus, out *expv1.MachinePoolStatus, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_MachinePoolStatus_To_v1beta2_MachinePoolStatus(in, out, s); err != nil {
		return err
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta1 conditions should not be automatically be converted into v1beta2 conditions.
	out.Conditions = nil

	// Reset replica counters from autogenerated conversions
	// NOTE: old replica counters should not be automatically be converted into replica counters with a new semantic.
	out.ReadyReplicas = nil
	out.AvailableReplicas = nil

	// Retrieve new conditions (v1beta2) from the v1beta2 field.
	if in.V1Beta2 != nil {
		out.Conditions = in.V1Beta2.Conditions
		out.ReadyReplicas = in.V1Beta2.ReadyReplicas
		out.AvailableReplicas = in.V1Beta2.AvailableReplicas
		out.UpToDateReplicas = in.V1Beta2.UpToDateReplicas
	}

	// Move legacy conditions (v1beta1), failureReason and failureMessage to the deprecated field.
	if out.Deprecated == nil {
		out.Deprecated = &expv1.MachinePoolDeprecatedStatus{}
	}
	if out.Deprecated.V1Beta1 == nil {
		out.Deprecated.V1Beta1 = &expv1.MachinePoolV1Beta1DeprecatedStatus{}
	}
	if in.Conditions != nil {
		clusterv1beta1.Convert_Deprecated_v1beta1_Conditions_To_v1beta2_Conditions(&in.Conditions, &out.Deprecated.V1Beta1.Conditions)
	}
	out.Deprecated.V1Beta1.FailureReason = in.FailureReason
	out.Deprecated.V1Beta1.FailureMessage = in.FailureMessage
	out.Deprecated.V1Beta1.ReadyReplicas = in.ReadyReplicas
	out.Deprecated.V1Beta1.AvailableReplicas = in.AvailableReplicas
	out.Deprecated.V1Beta1.UnavailableReplicas = in.UnavailableReplicas
	return nil
}

// Implement local conversion func because conversion-gen is not aware of conversion func in other packages (see https://github.com/kubernetes/code-generator/issues/94)

func Convert_v1beta1_MachineTemplateSpec_To_v1beta2_MachineTemplateSpec(in *clusterv1beta1.MachineTemplateSpec, out *clusterv1.MachineTemplateSpec, s apimachineryconversion.Scope) error {
	return clusterv1beta1.Convert_v1beta1_MachineTemplateSpec_To_v1beta2_MachineTemplateSpec(in, out, s)
}

func Convert_v1beta2_MachineTemplateSpec_To_v1beta1_MachineTemplateSpec(in *clusterv1.MachineTemplateSpec, out *clusterv1beta1.MachineTemplateSpec, s apimachineryconversion.Scope) error {
	return clusterv1beta1.Convert_v1beta2_MachineTemplateSpec_To_v1beta1_MachineTemplateSpec(in, out, s)
}

func Convert_v1_Condition_To_v1beta1_Condition(in *metav1.Condition, out *clusterv1beta1.Condition, s apimachineryconversion.Scope) error {
	return clusterv1beta1.Convert_v1_Condition_To_v1beta1_Condition(in, out, s)
}

func Convert_v1beta1_Condition_To_v1_Condition(in *clusterv1beta1.Condition, out *metav1.Condition, s apimachineryconversion.Scope) error {
	return clusterv1beta1.Convert_v1beta1_Condition_To_v1_Condition(in, out, s)
}
