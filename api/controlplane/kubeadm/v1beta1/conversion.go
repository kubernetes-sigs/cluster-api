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
	"errors"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apimachineryconversion "k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	bootstrapv1beta1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta1"
	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

var apiVersionGetter = func(_ schema.GroupKind) (string, error) {
	return "", errors.New("apiVersionGetter not set")
}

func SetAPIVersionGetter(f func(gk schema.GroupKind) (string, error)) {
	apiVersionGetter = f
}

func (src *KubeadmControlPlane) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*controlplanev1.KubeadmControlPlane)
	if err := Convert_v1beta1_KubeadmControlPlane_To_v1beta2_KubeadmControlPlane(src, dst, nil); err != nil {
		return err
	}

	infraRef, err := convertToContractVersionedObjectReference(&src.Spec.MachineTemplate.InfrastructureRef)
	if err != nil {
		return err
	}
	dst.Spec.MachineTemplate.InfrastructureRef = *infraRef

	// Manually restore data.
	restored := &controlplanev1.KubeadmControlPlane{}
	ok, err := utilconversion.UnmarshalData(src, restored)
	if err != nil {
		return err
	}

	// Recover intent for bool values converted to *bool.
	initialization := controlplanev1.KubeadmControlPlaneInitializationStatus{}
	var restoredControlPlaneInitialized *bool
	if restored.Status.Initialization != nil {
		restoredControlPlaneInitialized = restored.Status.Initialization.ControlPlaneInitialized
	}
	clusterv1.Convert_bool_To_Pointer_bool(src.Status.Initialized, ok, restoredControlPlaneInitialized, &initialization.ControlPlaneInitialized)
	if !reflect.DeepEqual(initialization, controlplanev1.KubeadmControlPlaneInitializationStatus{}) {
		dst.Status.Initialization = &initialization
	}

	if err := bootstrapv1beta1.RestoreBoolIntentKubeadmConfigSpec(&src.Spec.KubeadmConfigSpec, &dst.Spec.KubeadmConfigSpec, ok, &restored.Spec.KubeadmConfigSpec); err != nil {
		return err
	}

	// Recover other values
	if ok {
		bootstrapv1beta1.RestoreKubeadmConfigSpec(&restored.Spec.KubeadmConfigSpec, &dst.Spec.KubeadmConfigSpec)
	}

	if src.Spec.RemediationStrategy != nil {
		if dst.Spec.RemediationStrategy == nil {
			dst.Spec.RemediationStrategy = &controlplanev1.RemediationStrategy{}
		}
		var restoredRetryPeriodSeconds *int32
		if restored.Spec.RemediationStrategy != nil {
			restoredRetryPeriodSeconds = restored.Spec.RemediationStrategy.RetryPeriodSeconds
		}
		clusterv1.Convert_Duration_To_Pointer_int32(src.Spec.RemediationStrategy.RetryPeriod, ok, restoredRetryPeriodSeconds, &dst.Spec.RemediationStrategy.RetryPeriodSeconds)
	}

	// Override restored data with timeouts values already existing in v1beta1 but in other structs.
	src.Spec.KubeadmConfigSpec.ConvertTo(&dst.Spec.KubeadmConfigSpec)
	return nil
}

func (dst *KubeadmControlPlane) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*controlplanev1.KubeadmControlPlane)
	if err := Convert_v1beta2_KubeadmControlPlane_To_v1beta1_KubeadmControlPlane(src, dst, nil); err != nil {
		return err
	}

	infraRef, err := convertToObjectReference(&src.Spec.MachineTemplate.InfrastructureRef, src.Namespace)
	if err != nil {
		return err
	}
	dst.Spec.MachineTemplate.InfrastructureRef = *infraRef

	// Convert timeouts moved from one struct to another.
	dst.Spec.KubeadmConfigSpec.ConvertFrom(&src.Spec.KubeadmConfigSpec)

	dropEmptyStringsKubeadmConfigSpec(&dst.Spec.KubeadmConfigSpec)
	dropEmptyStringsKubeadmControlPlaneStatus(&dst.Status)

	// Preserve Hub data on down-conversion except for metadata.
	return utilconversion.MarshalData(src, dst)
}

func (src *KubeadmControlPlaneTemplate) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*controlplanev1.KubeadmControlPlaneTemplate)
	if err := Convert_v1beta1_KubeadmControlPlaneTemplate_To_v1beta2_KubeadmControlPlaneTemplate(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &controlplanev1.KubeadmControlPlaneTemplate{}
	ok, err := utilconversion.UnmarshalData(src, restored)
	if err != nil {
		return err
	}

	// Recover intent for bool values converted to *bool.
	if err := bootstrapv1beta1.RestoreBoolIntentKubeadmConfigSpec(&src.Spec.Template.Spec.KubeadmConfigSpec, &dst.Spec.Template.Spec.KubeadmConfigSpec, ok, &restored.Spec.Template.Spec.KubeadmConfigSpec); err != nil {
		return err
	}

	// Recover other values
	if ok {
		bootstrapv1beta1.RestoreKubeadmConfigSpec(&restored.Spec.Template.Spec.KubeadmConfigSpec, &dst.Spec.Template.Spec.KubeadmConfigSpec)
	}

	if src.Spec.Template.Spec.RemediationStrategy != nil {
		if dst.Spec.Template.Spec.RemediationStrategy == nil {
			dst.Spec.Template.Spec.RemediationStrategy = &controlplanev1.RemediationStrategy{}
		}
		var restoredRetryPeriodSeconds *int32
		if restored.Spec.Template.Spec.RemediationStrategy != nil {
			restoredRetryPeriodSeconds = restored.Spec.Template.Spec.RemediationStrategy.RetryPeriodSeconds
		}
		clusterv1.Convert_Duration_To_Pointer_int32(src.Spec.Template.Spec.RemediationStrategy.RetryPeriod, ok, restoredRetryPeriodSeconds, &dst.Spec.Template.Spec.RemediationStrategy.RetryPeriodSeconds)
	}

	// Override restored data with timeouts values already existing in v1beta1 but in other structs.
	src.Spec.Template.Spec.KubeadmConfigSpec.ConvertTo(&dst.Spec.Template.Spec.KubeadmConfigSpec)
	return nil
}

func (dst *KubeadmControlPlaneTemplate) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*controlplanev1.KubeadmControlPlaneTemplate)
	if err := Convert_v1beta2_KubeadmControlPlaneTemplate_To_v1beta1_KubeadmControlPlaneTemplate(src, dst, nil); err != nil {
		return err
	}

	// Convert timeouts moved from one struct to another.
	dst.Spec.Template.Spec.KubeadmConfigSpec.ConvertFrom(&src.Spec.Template.Spec.KubeadmConfigSpec)

	dropEmptyStringsKubeadmConfigSpec(&dst.Spec.Template.Spec.KubeadmConfigSpec)

	// Preserve Hub data on down-conversion except for metadata.
	return utilconversion.MarshalData(src, dst)
}

func Convert_v1beta2_KubeadmControlPlaneStatus_To_v1beta1_KubeadmControlPlaneStatus(in *controlplanev1.KubeadmControlPlaneStatus, out *KubeadmControlPlaneStatus, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_KubeadmControlPlaneStatus_To_v1beta1_KubeadmControlPlaneStatus(in, out, s); err != nil {
		return err
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta2 conditions should not be automatically be converted into legacy conditions (v1beta1).
	out.Conditions = nil

	// Reset replica counters from autogenerated conversions
	// NOTE: replica counters with a new semantic should not be automatically be converted into old replica counters.
	out.ReadyReplicas = 0

	// Retrieve legacy conditions (v1beta1), failureReason, failureMessage and replica counters from the deprecated field.
	if in.Deprecated != nil && in.Deprecated.V1Beta1 != nil {
		if in.Deprecated.V1Beta1.Conditions != nil {
			clusterv1beta1.Convert_v1beta2_Deprecated_V1Beta1_Conditions_To_v1beta1_Conditions(&in.Deprecated.V1Beta1.Conditions, &out.Conditions)
		}
		out.FailureReason = in.Deprecated.V1Beta1.FailureReason
		out.FailureMessage = in.Deprecated.V1Beta1.FailureMessage
		out.UpdatedReplicas = in.Deprecated.V1Beta1.UpdatedReplicas
		out.ReadyReplicas = in.Deprecated.V1Beta1.ReadyReplicas
		out.UnavailableReplicas = in.Deprecated.V1Beta1.UnavailableReplicas
	}

	// Move initialized to ControlPlaneInitialized, rebuild ready
	if in.Initialization != nil {
		out.Initialized = ptr.Deref(in.Initialization.ControlPlaneInitialized, false)
	}
	out.Ready = out.ReadyReplicas > 0

	// Move new conditions (v1beta2) and replica counter to the v1beta2 field.
	if in.Conditions == nil && in.ReadyReplicas == nil && in.AvailableReplicas == nil && in.UpToDateReplicas == nil {
		return nil
	}
	out.V1Beta2 = &KubeadmControlPlaneV1Beta2Status{}
	out.V1Beta2.Conditions = in.Conditions
	out.V1Beta2.ReadyReplicas = in.ReadyReplicas
	out.V1Beta2.AvailableReplicas = in.AvailableReplicas
	out.V1Beta2.UpToDateReplicas = in.UpToDateReplicas
	return nil
}

func Convert_v1beta1_KubeadmControlPlaneStatus_To_v1beta2_KubeadmControlPlaneStatus(in *KubeadmControlPlaneStatus, out *controlplanev1.KubeadmControlPlaneStatus, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_KubeadmControlPlaneStatus_To_v1beta2_KubeadmControlPlaneStatus(in, out, s); err != nil {
		return err
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta1 conditions should not be automatically be converted into v1beta2 conditions.
	out.Conditions = nil

	// Reset replica counters from autogenerated conversions
	// NOTE: old replica counters should not be automatically be converted into replica counters with a new semantic.
	out.ReadyReplicas = nil

	// Retrieve new conditions (v1beta2) and replica counter from the v1beta2 field.
	if in.V1Beta2 != nil {
		out.Conditions = in.V1Beta2.Conditions
		out.ReadyReplicas = in.V1Beta2.ReadyReplicas
		out.AvailableReplicas = in.V1Beta2.AvailableReplicas
		out.UpToDateReplicas = in.V1Beta2.UpToDateReplicas
	}

	// Move legacy conditions (v1beta1), failureReason, failureMessage and replica counters to the deprecated field.
	if out.Deprecated == nil {
		out.Deprecated = &controlplanev1.KubeadmControlPlaneDeprecatedStatus{}
	}
	if out.Deprecated.V1Beta1 == nil {
		out.Deprecated.V1Beta1 = &controlplanev1.KubeadmControlPlaneV1Beta1DeprecatedStatus{}
	}
	if in.Conditions != nil {
		clusterv1beta1.Convert_v1beta1_Conditions_To_v1beta2_Deprecated_V1Beta1_Conditions(&in.Conditions, &out.Deprecated.V1Beta1.Conditions)
	}
	out.Deprecated.V1Beta1.FailureReason = in.FailureReason
	out.Deprecated.V1Beta1.FailureMessage = in.FailureMessage
	out.Deprecated.V1Beta1.UpdatedReplicas = in.UpdatedReplicas
	out.Deprecated.V1Beta1.ReadyReplicas = in.ReadyReplicas
	out.Deprecated.V1Beta1.UnavailableReplicas = in.UnavailableReplicas

	// Move initialized to ControlPlaneInitialized is implemented in ConvertTo.
	return nil
}

func Convert_v1beta1_KubeadmControlPlaneMachineTemplate_To_v1beta2_KubeadmControlPlaneMachineTemplate(in *KubeadmControlPlaneMachineTemplate, out *controlplanev1.KubeadmControlPlaneMachineTemplate, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_KubeadmControlPlaneMachineTemplate_To_v1beta2_KubeadmControlPlaneMachineTemplate(in, out, s); err != nil {
		return err
	}
	out.NodeDrainTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeDrainTimeout)
	out.NodeVolumeDetachTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeVolumeDetachTimeout)
	out.NodeDeletionTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeDeletionTimeout)
	return nil
}
func Convert_v1beta2_KubeadmControlPlaneMachineTemplate_To_v1beta1_KubeadmControlPlaneMachineTemplate(in *controlplanev1.KubeadmControlPlaneMachineTemplate, out *KubeadmControlPlaneMachineTemplate, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_KubeadmControlPlaneMachineTemplate_To_v1beta1_KubeadmControlPlaneMachineTemplate(in, out, s); err != nil {
		return err
	}
	out.NodeDrainTimeout = clusterv1.ConvertFromSeconds(in.NodeDrainTimeoutSeconds)
	out.NodeVolumeDetachTimeout = clusterv1.ConvertFromSeconds(in.NodeVolumeDetachTimeoutSeconds)
	out.NodeDeletionTimeout = clusterv1.ConvertFromSeconds(in.NodeDeletionTimeoutSeconds)
	return nil
}

func Convert_v1beta1_KubeadmControlPlaneTemplateMachineTemplate_To_v1beta2_KubeadmControlPlaneTemplateMachineTemplate(in *KubeadmControlPlaneTemplateMachineTemplate, out *controlplanev1.KubeadmControlPlaneTemplateMachineTemplate, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_KubeadmControlPlaneTemplateMachineTemplate_To_v1beta2_KubeadmControlPlaneTemplateMachineTemplate(in, out, s); err != nil {
		return err
	}
	out.NodeDrainTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeDrainTimeout)
	out.NodeVolumeDetachTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeVolumeDetachTimeout)
	out.NodeDeletionTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeDeletionTimeout)
	return nil
}

func Convert_v1beta2_KubeadmControlPlaneTemplateMachineTemplate_To_v1beta1_KubeadmControlPlaneTemplateMachineTemplate(in *controlplanev1.KubeadmControlPlaneTemplateMachineTemplate, out *KubeadmControlPlaneTemplateMachineTemplate, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_KubeadmControlPlaneTemplateMachineTemplate_To_v1beta1_KubeadmControlPlaneTemplateMachineTemplate(in, out, s); err != nil {
		return err
	}
	out.NodeDrainTimeout = clusterv1.ConvertFromSeconds(in.NodeDrainTimeoutSeconds)
	out.NodeVolumeDetachTimeout = clusterv1.ConvertFromSeconds(in.NodeVolumeDetachTimeoutSeconds)
	out.NodeDeletionTimeout = clusterv1.ConvertFromSeconds(in.NodeDeletionTimeoutSeconds)
	return nil
}

func Convert_v1beta1_RemediationStrategy_To_v1beta2_RemediationStrategy(in *RemediationStrategy, out *controlplanev1.RemediationStrategy, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_RemediationStrategy_To_v1beta2_RemediationStrategy(in, out, s); err != nil {
		return err
	}
	out.MinHealthyPeriodSeconds = clusterv1.ConvertToSeconds(in.MinHealthyPeriod)
	out.RetryPeriodSeconds = clusterv1.ConvertToSeconds(&in.RetryPeriod)
	return nil
}

func Convert_v1beta2_RemediationStrategy_To_v1beta1_RemediationStrategy(in *controlplanev1.RemediationStrategy, out *RemediationStrategy, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_RemediationStrategy_To_v1beta1_RemediationStrategy(in, out, s); err != nil {
		return err
	}
	out.MinHealthyPeriod = clusterv1.ConvertFromSeconds(in.MinHealthyPeriodSeconds)
	out.RetryPeriod = ptr.Deref(clusterv1.ConvertFromSeconds(in.RetryPeriodSeconds), metav1.Duration{})
	return nil
}

// Implement local conversion func because conversion-gen is not aware of conversion func in other packages (see https://github.com/kubernetes/code-generator/issues/94)

func Convert_v1beta1_ObjectMeta_To_v1beta2_ObjectMeta(in *clusterv1beta1.ObjectMeta, out *clusterv1.ObjectMeta, s apimachineryconversion.Scope) error {
	return clusterv1beta1.Convert_v1beta1_ObjectMeta_To_v1beta2_ObjectMeta(in, out, s)
}

func Convert_v1beta2_ObjectMeta_To_v1beta1_ObjectMeta(in *clusterv1.ObjectMeta, out *clusterv1beta1.ObjectMeta, s apimachineryconversion.Scope) error {
	return clusterv1beta1.Convert_v1beta2_ObjectMeta_To_v1beta1_ObjectMeta(in, out, s)
}

func Convert_v1beta1_KubeadmConfigSpec_To_v1beta2_KubeadmConfigSpec(in *bootstrapv1beta1.KubeadmConfigSpec, out *bootstrapv1.KubeadmConfigSpec, s apimachineryconversion.Scope) error {
	return bootstrapv1beta1.Convert_v1beta1_KubeadmConfigSpec_To_v1beta2_KubeadmConfigSpec(in, out, s)
}

func Convert_v1beta2_KubeadmConfigSpec_To_v1beta1_KubeadmConfigSpec(in *bootstrapv1.KubeadmConfigSpec, out *bootstrapv1beta1.KubeadmConfigSpec, s apimachineryconversion.Scope) error {
	return bootstrapv1beta1.Convert_v1beta2_KubeadmConfigSpec_To_v1beta1_KubeadmConfigSpec(in, out, s)
}

func Convert_v1_Condition_To_v1beta1_Condition(in *metav1.Condition, out *clusterv1beta1.Condition, s apimachineryconversion.Scope) error {
	return clusterv1beta1.Convert_v1_Condition_To_v1beta1_Condition(in, out, s)
}

func Convert_v1beta1_Condition_To_v1_Condition(in *clusterv1beta1.Condition, out *metav1.Condition, s apimachineryconversion.Scope) error {
	return clusterv1beta1.Convert_v1beta1_Condition_To_v1_Condition(in, out, s)
}

func Convert_v1_ObjectReference_To_v1beta2_ContractVersionedObjectReference(_ *corev1.ObjectReference, _ *clusterv1.ContractVersionedObjectReference, _ apimachineryconversion.Scope) error {
	// This is implemented in ConvertTo/ConvertFrom as we have all necessary information available there.
	return nil
}

func Convert_v1beta2_ContractVersionedObjectReference_To_v1_ObjectReference(_ *clusterv1.ContractVersionedObjectReference, _ *corev1.ObjectReference, _ apimachineryconversion.Scope) error {
	// This is implemented in ConvertTo/ConvertFrom as we have all necessary information available there.
	return nil
}

func Convert_v1beta1_LastRemediationStatus_To_v1beta2_LastRemediationStatus(in *LastRemediationStatus, out *controlplanev1.LastRemediationStatus, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_LastRemediationStatus_To_v1beta2_LastRemediationStatus(in, out, s); err != nil {
		return err
	}
	out.Time = in.Timestamp
	return nil
}

func Convert_v1beta2_LastRemediationStatus_To_v1beta1_LastRemediationStatus(in *controlplanev1.LastRemediationStatus, out *LastRemediationStatus, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_LastRemediationStatus_To_v1beta1_LastRemediationStatus(in, out, s); err != nil {
		return err
	}
	out.Timestamp = in.Time
	return nil
}

func convertToContractVersionedObjectReference(ref *corev1.ObjectReference) (*clusterv1.ContractVersionedObjectReference, error) {
	var apiGroup string
	if ref.APIVersion != "" {
		gv, err := schema.ParseGroupVersion(ref.APIVersion)
		if err != nil {
			return nil, fmt.Errorf("failed to convert object: failed to parse apiVersion: %v", err)
		}
		apiGroup = gv.Group
	}
	return &clusterv1.ContractVersionedObjectReference{
		APIGroup: apiGroup,
		Kind:     ref.Kind,
		Name:     ref.Name,
	}, nil
}

func convertToObjectReference(ref *clusterv1.ContractVersionedObjectReference, namespace string) (*corev1.ObjectReference, error) {
	apiVersion, err := apiVersionGetter(schema.GroupKind{
		Group: ref.APIGroup,
		Kind:  ref.Kind,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to convert object: %v", err)
	}
	return &corev1.ObjectReference{
		APIVersion: apiVersion,
		Kind:       ref.Kind,
		Namespace:  namespace,
		Name:       ref.Name,
	}, nil
}

func dropEmptyStringsKubeadmConfigSpec(dst *bootstrapv1beta1.KubeadmConfigSpec) {
	for i, u := range dst.Users {
		dropEmptyString(&u.Gecos)
		dropEmptyString(&u.Groups)
		dropEmptyString(&u.HomeDir)
		dropEmptyString(&u.Shell)
		dropEmptyString(&u.Passwd)
		dropEmptyString(&u.PrimaryGroup)
		dropEmptyString(&u.Sudo)
		dst.Users[i] = u
	}

	if dst.DiskSetup != nil {
		for i, p := range dst.DiskSetup.Partitions {
			dropEmptyString(&p.TableType)
			dst.DiskSetup.Partitions[i] = p
		}
		for i, f := range dst.DiskSetup.Filesystems {
			dropEmptyString(&f.Partition)
			dropEmptyString(&f.ReplaceFS)
			dst.DiskSetup.Filesystems[i] = f
		}
	}
}

func dropEmptyStringsKubeadmControlPlaneStatus(dst *KubeadmControlPlaneStatus) {
	dropEmptyString(&dst.Version)
}

func dropEmptyString(s **string) {
	if *s != nil && **s == "" {
		*s = nil
	}
}
