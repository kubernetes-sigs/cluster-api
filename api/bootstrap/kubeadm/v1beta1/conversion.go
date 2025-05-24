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
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apimachineryconversion "k8s.io/apimachinery/pkg/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

func (src *KubeadmConfig) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*bootstrapv1.KubeadmConfig)
	if err := Convert_v1beta1_KubeadmConfig_To_v1beta2_KubeadmConfig(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &bootstrapv1.KubeadmConfig{}
	ok, err := utilconversion.UnmarshalData(src, restored)
	if err != nil {
		return err
	}
	if ok {
		RestoreKubeadmConfigSpec(&restored.Spec, &dst.Spec)
	}
	// Override restored data with timeouts values already existing in v1beta1 but in other structs.
	src.Spec.ConvertTo(&dst.Spec)
	return nil
}

func RestoreKubeadmConfigSpec(restored *bootstrapv1.KubeadmConfigSpec, dst *bootstrapv1.KubeadmConfigSpec) {
	// Restore fields added in v1beta2
	if restored.InitConfiguration != nil && restored.InitConfiguration.Timeouts != nil {
		if dst.InitConfiguration == nil {
			dst.InitConfiguration = &bootstrapv1.InitConfiguration{}
		}
		dst.InitConfiguration.Timeouts = restored.InitConfiguration.Timeouts
	}
	if restored.JoinConfiguration != nil && restored.JoinConfiguration.Timeouts != nil {
		if dst.JoinConfiguration == nil {
			dst.JoinConfiguration = &bootstrapv1.JoinConfiguration{}
		}
		dst.JoinConfiguration.Timeouts = restored.JoinConfiguration.Timeouts
	}
}

func (src *KubeadmConfigSpec) ConvertTo(dst *bootstrapv1.KubeadmConfigSpec) {
	// Override with timeouts values already existing in v1beta1.
	if src.ClusterConfiguration != nil && src.ClusterConfiguration.APIServer.TimeoutForControlPlane != nil {
		if dst.InitConfiguration == nil {
			dst.InitConfiguration = &bootstrapv1.InitConfiguration{}
		}
		if dst.InitConfiguration.Timeouts == nil {
			dst.InitConfiguration.Timeouts = &bootstrapv1.Timeouts{}
		}
		dst.InitConfiguration.Timeouts.ControlPlaneComponentHealthCheckSeconds = utilconversion.ConvertToSeconds(src.ClusterConfiguration.APIServer.TimeoutForControlPlane)
	}
	if src.JoinConfiguration != nil && src.JoinConfiguration.Discovery.Timeout != nil {
		if dst.JoinConfiguration == nil {
			dst.JoinConfiguration = &bootstrapv1.JoinConfiguration{}
		}
		if dst.JoinConfiguration.Timeouts == nil {
			dst.JoinConfiguration.Timeouts = &bootstrapv1.Timeouts{}
		}
		dst.JoinConfiguration.Timeouts.TLSBootstrapSeconds = utilconversion.ConvertToSeconds(src.JoinConfiguration.Discovery.Timeout)
	}

	if reflect.DeepEqual(dst.ClusterConfiguration, &bootstrapv1.ClusterConfiguration{}) {
		dst.ClusterConfiguration = nil
	}
}

func (dst *KubeadmConfig) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*bootstrapv1.KubeadmConfig)
	if err := Convert_v1beta2_KubeadmConfig_To_v1beta1_KubeadmConfig(src, dst, nil); err != nil {
		return err
	}

	// Convert timeouts moved from one struct to another.
	dst.Spec.ConvertFrom(&src.Spec)

	// Preserve Hub data on down-conversion except for metadata.
	return utilconversion.MarshalData(src, dst)
}

func (dst *KubeadmConfigSpec) ConvertFrom(src *bootstrapv1.KubeadmConfigSpec) {
	// Convert timeouts moved from one struct to another.
	if src.InitConfiguration != nil && src.InitConfiguration.Timeouts != nil && src.InitConfiguration.Timeouts.ControlPlaneComponentHealthCheckSeconds != nil {
		if dst.ClusterConfiguration == nil {
			dst.ClusterConfiguration = &ClusterConfiguration{}
		}
		dst.ClusterConfiguration.APIServer.TimeoutForControlPlane = utilconversion.ConvertFromSeconds(src.InitConfiguration.Timeouts.ControlPlaneComponentHealthCheckSeconds)
	}
	if reflect.DeepEqual(dst.InitConfiguration, &InitConfiguration{}) {
		dst.InitConfiguration = nil
	}
	if src.JoinConfiguration != nil && src.JoinConfiguration.Timeouts != nil && src.JoinConfiguration.Timeouts.TLSBootstrapSeconds != nil {
		if dst.JoinConfiguration == nil {
			dst.JoinConfiguration = &JoinConfiguration{}
		}
		dst.JoinConfiguration.Discovery.Timeout = utilconversion.ConvertFromSeconds(src.JoinConfiguration.Timeouts.TLSBootstrapSeconds)
	}
	if reflect.DeepEqual(dst.JoinConfiguration, &JoinConfiguration{}) {
		dst.JoinConfiguration = nil
	}
}

func (src *KubeadmConfigTemplate) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*bootstrapv1.KubeadmConfigTemplate)
	if err := Convert_v1beta1_KubeadmConfigTemplate_To_v1beta2_KubeadmConfigTemplate(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &bootstrapv1.KubeadmConfigTemplate{}
	ok, err := utilconversion.UnmarshalData(src, restored)
	if err != nil {
		return err
	}
	if ok {
		RestoreKubeadmConfigSpec(&restored.Spec.Template.Spec, &dst.Spec.Template.Spec)
	}

	// Override restored data with timeouts values already existing in v1beta1 but in other structs.
	src.Spec.Template.Spec.ConvertTo(&dst.Spec.Template.Spec)
	return nil
}

func (dst *KubeadmConfigTemplate) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*bootstrapv1.KubeadmConfigTemplate)
	if err := Convert_v1beta2_KubeadmConfigTemplate_To_v1beta1_KubeadmConfigTemplate(src, dst, nil); err != nil {
		return err
	}

	// Convert timeouts moved from one struct to another.
	dst.Spec.Template.Spec.ConvertFrom(&src.Spec.Template.Spec)

	// Preserve Hub data on down-conversion except for metadata.
	return utilconversion.MarshalData(src, dst)
}

func Convert_v1beta2_InitConfiguration_To_v1beta1_InitConfiguration(in *bootstrapv1.InitConfiguration, out *InitConfiguration, s apimachineryconversion.Scope) error {
	// Timeouts requires conversion at an upper level
	return autoConvert_v1beta2_InitConfiguration_To_v1beta1_InitConfiguration(in, out, s)
}

func Convert_v1beta2_JoinConfiguration_To_v1beta1_JoinConfiguration(in *bootstrapv1.JoinConfiguration, out *JoinConfiguration, s apimachineryconversion.Scope) error {
	// Timeouts requires conversion at an upper level
	return autoConvert_v1beta2_JoinConfiguration_To_v1beta1_JoinConfiguration(in, out, s)
}

func Convert_v1beta2_KubeadmConfigStatus_To_v1beta1_KubeadmConfigStatus(in *bootstrapv1.KubeadmConfigStatus, out *KubeadmConfigStatus, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_KubeadmConfigStatus_To_v1beta1_KubeadmConfigStatus(in, out, s); err != nil {
		return err
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta2 conditions should not be automatically be converted into legacy conditions (v1beta1).
	out.Conditions = nil

	// Retrieve legacy conditions (v1beta1), failureReason and failureMessage from the deprecated field.
	if in.Deprecated != nil && in.Deprecated.V1Beta1 != nil {
		if in.Deprecated.V1Beta1.Conditions != nil {
			clusterv1beta1.Convert_v1beta2_Deprecated_V1Beta1_Conditions_To_v1beta1_Conditions(&in.Deprecated.V1Beta1.Conditions, &out.Conditions)
		}
		out.FailureReason = in.Deprecated.V1Beta1.FailureReason
		out.FailureMessage = in.Deprecated.V1Beta1.FailureMessage
	}

	// Move initialization to old fields
	if in.Initialization != nil {
		out.Ready = in.Initialization.DataSecretCreated
	}

	// Move new conditions (v1beta2) to the v1beta2 field.
	if in.Conditions == nil {
		return nil
	}
	out.V1Beta2 = &KubeadmConfigV1Beta2Status{}
	out.V1Beta2.Conditions = in.Conditions
	return nil
}

func Convert_v1beta2_ControlPlaneComponent_To_v1beta1_ControlPlaneComponent(in *bootstrapv1.ControlPlaneComponent, out *ControlPlaneComponent, s apimachineryconversion.Scope) error {
	// Following fields require a custom conversions.
	out.ExtraArgs = utilconversion.ConvertFromArgs(in.ExtraArgs)
	return autoConvert_v1beta2_ControlPlaneComponent_To_v1beta1_ControlPlaneComponent(in, out, s)
}

func Convert_v1beta2_LocalEtcd_To_v1beta1_LocalEtcd(in *bootstrapv1.LocalEtcd, out *LocalEtcd, s apimachineryconversion.Scope) error {
	// Following fields require a custom conversions.
	out.ExtraArgs = utilconversion.ConvertFromArgs(in.ExtraArgs)
	return autoConvert_v1beta2_LocalEtcd_To_v1beta1_LocalEtcd(in, out, s)
}

func Convert_v1beta2_NodeRegistrationOptions_To_v1beta1_NodeRegistrationOptions(in *bootstrapv1.NodeRegistrationOptions, out *NodeRegistrationOptions, s apimachineryconversion.Scope) error {
	// Following fields require a custom conversions.
	out.KubeletExtraArgs = utilconversion.ConvertFromArgs(in.KubeletExtraArgs)
	return autoConvert_v1beta2_NodeRegistrationOptions_To_v1beta1_NodeRegistrationOptions(in, out, s)
}

func Convert_v1beta1_APIServer_To_v1beta2_APIServer(in *APIServer, out *bootstrapv1.APIServer, s apimachineryconversion.Scope) error {
	// TimeoutForControlPlane has been removed in v1beta2
	return autoConvert_v1beta1_APIServer_To_v1beta2_APIServer(in, out, s)
}

func Convert_v1beta1_Discovery_To_v1beta2_Discovery(in *Discovery, out *bootstrapv1.Discovery, s apimachineryconversion.Scope) error {
	// Timeout has been removed in v1beta2
	return autoConvert_v1beta1_Discovery_To_v1beta2_Discovery(in, out, s)
}

func Convert_v1beta1_KubeadmConfigSpec_To_v1beta2_KubeadmConfigSpec(in *KubeadmConfigSpec, out *bootstrapv1.KubeadmConfigSpec, s apimachineryconversion.Scope) error {
	// NOTE: v1beta2 KubeadmConfigSpec does not have UseExperimentalRetryJoin anymore, so it's fine to just lose this field.
	return autoConvert_v1beta1_KubeadmConfigSpec_To_v1beta2_KubeadmConfigSpec(in, out, s)
}

func Convert_v1beta1_ControlPlaneComponent_To_v1beta2_ControlPlaneComponent(in *ControlPlaneComponent, out *bootstrapv1.ControlPlaneComponent, s apimachineryconversion.Scope) error {
	out.ExtraArgs = utilconversion.ConvertToArgs(in.ExtraArgs)
	return autoConvert_v1beta1_ControlPlaneComponent_To_v1beta2_ControlPlaneComponent(in, out, s)
}

func Convert_v1beta1_LocalEtcd_To_v1beta2_LocalEtcd(in *LocalEtcd, out *bootstrapv1.LocalEtcd, s apimachineryconversion.Scope) error {
	out.ExtraArgs = utilconversion.ConvertToArgs(in.ExtraArgs)
	return autoConvert_v1beta1_LocalEtcd_To_v1beta2_LocalEtcd(in, out, s)
}

func Convert_v1beta1_NodeRegistrationOptions_To_v1beta2_NodeRegistrationOptions(in *NodeRegistrationOptions, out *bootstrapv1.NodeRegistrationOptions, s apimachineryconversion.Scope) error {
	out.KubeletExtraArgs = utilconversion.ConvertToArgs(in.KubeletExtraArgs)
	return autoConvert_v1beta1_NodeRegistrationOptions_To_v1beta2_NodeRegistrationOptions(in, out, s)
}

func Convert_v1beta1_KubeadmConfigStatus_To_v1beta2_KubeadmConfigStatus(in *KubeadmConfigStatus, out *bootstrapv1.KubeadmConfigStatus, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_KubeadmConfigStatus_To_v1beta2_KubeadmConfigStatus(in, out, s); err != nil {
		return err
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta1 conditions should not be automatically be converted into v1beta2 conditions.
	out.Conditions = nil

	// Retrieve new conditions (v1beta2) from the v1beta2 field.
	if in.V1Beta2 != nil {
		out.Conditions = in.V1Beta2.Conditions
	}

	// Move legacy conditions (v1beta1), failureReason and failureMessage to the deprecated field.
	if out.Deprecated == nil {
		out.Deprecated = &bootstrapv1.KubeadmConfigDeprecatedStatus{}
	}
	if out.Deprecated.V1Beta1 == nil {
		out.Deprecated.V1Beta1 = &bootstrapv1.KubeadmConfigV1Beta1DeprecatedStatus{}
	}
	if in.Conditions != nil {
		clusterv1beta1.Convert_v1beta1_Conditions_To_v1beta2_Deprecated_V1Beta1_Conditions(&in.Conditions, &out.Deprecated.V1Beta1.Conditions)
	}
	out.Deprecated.V1Beta1.FailureReason = in.FailureReason
	out.Deprecated.V1Beta1.FailureMessage = in.FailureMessage

	// Move ready to Initialization
	if in.Ready {
		if out.Initialization == nil {
			out.Initialization = &bootstrapv1.KubeadmConfigInitializationStatus{}
		}
		out.Initialization.DataSecretCreated = in.Ready
	}
	return nil
}

// Implement local conversion func because conversion-gen is not aware of conversion func in other packages (see https://github.com/kubernetes/code-generator/issues/94)

func Convert_v1beta1_ObjectMeta_To_v1beta2_ObjectMeta(in *clusterv1beta1.ObjectMeta, out *clusterv1.ObjectMeta, s apimachineryconversion.Scope) error {
	return clusterv1beta1.Convert_v1beta1_ObjectMeta_To_v1beta2_ObjectMeta(in, out, s)
}

func Convert_v1beta2_ObjectMeta_To_v1beta1_ObjectMeta(in *clusterv1.ObjectMeta, out *clusterv1beta1.ObjectMeta, s apimachineryconversion.Scope) error {
	return clusterv1beta1.Convert_v1beta2_ObjectMeta_To_v1beta1_ObjectMeta(in, out, s)
}

func Convert_v1_Condition_To_v1beta1_Condition(in *metav1.Condition, out *clusterv1beta1.Condition, s apimachineryconversion.Scope) error {
	return clusterv1beta1.Convert_v1_Condition_To_v1beta1_Condition(in, out, s)
}

func Convert_v1beta1_Condition_To_v1_Condition(in *clusterv1beta1.Condition, out *metav1.Condition, s apimachineryconversion.Scope) error {
	return clusterv1beta1.Convert_v1beta1_Condition_To_v1_Condition(in, out, s)
}
