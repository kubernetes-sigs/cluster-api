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
	"fmt"
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apimachineryconversion "k8s.io/apimachinery/pkg/conversion"
	"k8s.io/utils/ptr"
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

	// Recover intent for bool values converted to *bool.
	initialization := bootstrapv1.KubeadmConfigInitializationStatus{}
	restoredBootstrapDataSecretCreated := restored.Status.Initialization.DataSecretCreated
	clusterv1.Convert_bool_To_Pointer_bool(src.Status.Ready, ok, restoredBootstrapDataSecretCreated, &initialization.DataSecretCreated)
	if !reflect.DeepEqual(initialization, bootstrapv1.KubeadmConfigInitializationStatus{}) {
		dst.Status.Initialization = initialization
	}
	if err := RestoreBoolIntentKubeadmConfigSpec(&src.Spec, &dst.Spec, ok, &restored.Spec); err != nil {
		return err
	}

	// Recover other values
	if ok {
		RestoreKubeadmConfigSpec(&restored.Spec, &dst.Spec)
	}

	// Override restored data with timeouts values already existing in v1beta1 but in other structs.
	src.Spec.ConvertTo(&dst.Spec)
	return nil
}

func RestoreKubeadmConfigSpec(restored *bootstrapv1.KubeadmConfigSpec, dst *bootstrapv1.KubeadmConfigSpec) {
	// Restore fields added in v1beta2
	if restored.InitConfiguration.IsDefined() && !reflect.DeepEqual(restored.InitConfiguration.Timeouts, bootstrapv1.Timeouts{}) {
		dst.InitConfiguration.Timeouts = restored.InitConfiguration.Timeouts
	}
	if restored.JoinConfiguration.IsDefined() && !reflect.DeepEqual(restored.JoinConfiguration.Timeouts, bootstrapv1.Timeouts{}) {
		dst.JoinConfiguration.Timeouts = restored.JoinConfiguration.Timeouts
	}
	if restored.ClusterConfiguration.CertificateValidityPeriodDays != 0 || restored.ClusterConfiguration.CACertificateValidityPeriodDays != 0 {
		if restored.ClusterConfiguration.CertificateValidityPeriodDays != 0 {
			dst.ClusterConfiguration.CertificateValidityPeriodDays = restored.ClusterConfiguration.CertificateValidityPeriodDays
		}
		if restored.ClusterConfiguration.CACertificateValidityPeriodDays != 0 {
			dst.ClusterConfiguration.CACertificateValidityPeriodDays = restored.ClusterConfiguration.CACertificateValidityPeriodDays
		}
	}
}

func RestoreBoolIntentKubeadmConfigSpec(src *KubeadmConfigSpec, dst *bootstrapv1.KubeadmConfigSpec, hasRestored bool, restored *bootstrapv1.KubeadmConfigSpec) error {
	if dst.JoinConfiguration.Discovery.BootstrapToken.IsDefined() {
		restoredUnsafeSkipCAVerification := restored.JoinConfiguration.Discovery.BootstrapToken.UnsafeSkipCAVerification
		clusterv1.Convert_bool_To_Pointer_bool(src.JoinConfiguration.Discovery.BootstrapToken.UnsafeSkipCAVerification, hasRestored, restoredUnsafeSkipCAVerification, &dst.JoinConfiguration.Discovery.BootstrapToken.UnsafeSkipCAVerification)
	}

	if dst.JoinConfiguration.Discovery.File.IsDefined() && dst.JoinConfiguration.Discovery.File.KubeConfig.IsDefined() {
		if dst.JoinConfiguration.Discovery.File.KubeConfig.Cluster.IsDefined() {
			restoredInsecureSkipTLSVerify := restored.JoinConfiguration.Discovery.File.KubeConfig.Cluster.InsecureSkipTLSVerify
			clusterv1.Convert_bool_To_Pointer_bool(src.JoinConfiguration.Discovery.File.KubeConfig.Cluster.InsecureSkipTLSVerify, hasRestored, restoredInsecureSkipTLSVerify, &dst.JoinConfiguration.Discovery.File.KubeConfig.Cluster.InsecureSkipTLSVerify)
		}
		if dst.JoinConfiguration.Discovery.File.KubeConfig.User.Exec.IsDefined() {
			restoredExecProvideClusterInfo := restored.JoinConfiguration.Discovery.File.KubeConfig.User.Exec.ProvideClusterInfo
			clusterv1.Convert_bool_To_Pointer_bool(src.JoinConfiguration.Discovery.File.KubeConfig.User.Exec.ProvideClusterInfo, hasRestored, restoredExecProvideClusterInfo, &dst.JoinConfiguration.Discovery.File.KubeConfig.User.Exec.ProvideClusterInfo)
		}
	}

	for i, volume := range dst.ClusterConfiguration.APIServer.ExtraVolumes {
		var srcVolume *HostPathMount
		if src.ClusterConfiguration != nil {
			for _, v := range src.ClusterConfiguration.APIServer.ExtraVolumes {
				if v.HostPath == volume.HostPath {
					srcVolume = &v
					break
				}
			}
		}
		if srcVolume == nil {
			return fmt.Errorf("apiServer extraVolume with hostPath %q not found in source data", volume.HostPath)
		}
		var restoredVolumeReadOnly *bool
		for _, v := range restored.ClusterConfiguration.APIServer.ExtraVolumes {
			if v.HostPath == volume.HostPath {
				restoredVolumeReadOnly = v.ReadOnly
				break
			}
		}
		clusterv1.Convert_bool_To_Pointer_bool(srcVolume.ReadOnly, hasRestored, restoredVolumeReadOnly, &volume.ReadOnly)
		dst.ClusterConfiguration.APIServer.ExtraVolumes[i] = volume
	}
	for i, volume := range dst.ClusterConfiguration.ControllerManager.ExtraVolumes {
		var srcVolume *HostPathMount
		if src.ClusterConfiguration != nil {
			for _, v := range src.ClusterConfiguration.ControllerManager.ExtraVolumes {
				if v.HostPath == volume.HostPath {
					srcVolume = &v
					break
				}
			}
		}
		if srcVolume == nil {
			return fmt.Errorf("controllerManager extraVolume with hostPath %q not found in source data", volume.HostPath)
		}
		var restoredVolumeReadOnly *bool
		for _, v := range restored.ClusterConfiguration.ControllerManager.ExtraVolumes {
			if v.HostPath == volume.HostPath {
				restoredVolumeReadOnly = v.ReadOnly
				break
			}
		}
		clusterv1.Convert_bool_To_Pointer_bool(srcVolume.ReadOnly, hasRestored, restoredVolumeReadOnly, &volume.ReadOnly)
		dst.ClusterConfiguration.ControllerManager.ExtraVolumes[i] = volume
	}
	for i, volume := range dst.ClusterConfiguration.Scheduler.ExtraVolumes {
		var srcVolume *HostPathMount
		if src.ClusterConfiguration != nil {
			for _, v := range src.ClusterConfiguration.Scheduler.ExtraVolumes {
				if v.HostPath == volume.HostPath {
					srcVolume = &v
					break
				}
			}
		}
		if srcVolume == nil {
			return fmt.Errorf("scheduler extraVolume with hostPath %q not found in source data", volume.HostPath)
		}
		var restoredVolumeReadOnly *bool
		for _, v := range restored.ClusterConfiguration.Scheduler.ExtraVolumes {
			if v.HostPath == volume.HostPath {
				restoredVolumeReadOnly = v.ReadOnly
				break
			}
		}
		clusterv1.Convert_bool_To_Pointer_bool(srcVolume.ReadOnly, hasRestored, restoredVolumeReadOnly, &volume.ReadOnly)
		dst.ClusterConfiguration.Scheduler.ExtraVolumes[i] = volume
	}

	for i, file := range dst.Files {
		var srcFile *File
		for _, f := range src.Files {
			if f.Path == file.Path {
				srcFile = &f
				break
			}
		}
		if srcFile == nil {
			return fmt.Errorf("file with path %q not found in source data", file.Path)
		}
		var restoredFileAppend *bool
		for _, f := range restored.Files {
			if f.Path == file.Path {
				restoredFileAppend = f.Append
				break
			}
		}
		clusterv1.Convert_bool_To_Pointer_bool(srcFile.Append, hasRestored, restoredFileAppend, &file.Append)
		dst.Files[i] = file
	}

	if dst.Ignition.IsDefined() && dst.Ignition.ContainerLinuxConfig.IsDefined() {
		restoredIgnitionStrict := restored.Ignition.ContainerLinuxConfig.Strict
		clusterv1.Convert_bool_To_Pointer_bool(src.Ignition.ContainerLinuxConfig.Strict, hasRestored, restoredIgnitionStrict, &dst.Ignition.ContainerLinuxConfig.Strict)
	}
	return nil
}

func (src *KubeadmConfigSpec) ConvertTo(dst *bootstrapv1.KubeadmConfigSpec) {
	// Override with timeouts values already existing in v1beta1.
	var initControlPlaneComponentHealthCheckSeconds *int32
	if src.ClusterConfiguration != nil && src.ClusterConfiguration.APIServer.TimeoutForControlPlane != nil {
		dst.InitConfiguration.Timeouts.ControlPlaneComponentHealthCheckSeconds = clusterv1.ConvertToSeconds(src.ClusterConfiguration.APIServer.TimeoutForControlPlane)
		initControlPlaneComponentHealthCheckSeconds = dst.InitConfiguration.Timeouts.ControlPlaneComponentHealthCheckSeconds
	}
	if (src.JoinConfiguration != nil && src.JoinConfiguration.Discovery.Timeout != nil) || initControlPlaneComponentHealthCheckSeconds != nil {
		dst.JoinConfiguration.Timeouts.ControlPlaneComponentHealthCheckSeconds = initControlPlaneComponentHealthCheckSeconds
		if src.JoinConfiguration != nil && src.JoinConfiguration.Discovery.Timeout != nil {
			dst.JoinConfiguration.Timeouts.TLSBootstrapSeconds = clusterv1.ConvertToSeconds(src.JoinConfiguration.Discovery.Timeout)
		}
	}
}

func (dst *KubeadmConfig) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*bootstrapv1.KubeadmConfig)
	if err := Convert_v1beta2_KubeadmConfig_To_v1beta1_KubeadmConfig(src, dst, nil); err != nil {
		return err
	}

	// Convert timeouts moved from one struct to another.
	dst.Spec.ConvertFrom(&src.Spec)

	dropEmptyStringsKubeadmConfigSpec(&dst.Spec)
	dropEmptyStringsKubeadmConfigStatus(&dst.Status)

	// Preserve Hub data on down-conversion except for metadata.
	return utilconversion.MarshalData(src, dst)
}

func (dst *KubeadmConfigSpec) ConvertFrom(src *bootstrapv1.KubeadmConfigSpec) {
	// Convert timeouts moved from one struct to another.
	if src.InitConfiguration.Timeouts.ControlPlaneComponentHealthCheckSeconds != nil {
		if dst.ClusterConfiguration == nil {
			dst.ClusterConfiguration = &ClusterConfiguration{}
		}
		dst.ClusterConfiguration.APIServer.TimeoutForControlPlane = clusterv1.ConvertFromSeconds(src.InitConfiguration.Timeouts.ControlPlaneComponentHealthCheckSeconds)
	}
	if reflect.DeepEqual(dst.InitConfiguration, &InitConfiguration{}) {
		dst.InitConfiguration = nil
	}
	if src.JoinConfiguration.Timeouts.TLSBootstrapSeconds != nil {
		if dst.JoinConfiguration == nil {
			dst.JoinConfiguration = &JoinConfiguration{}
		}
		dst.JoinConfiguration.Discovery.Timeout = clusterv1.ConvertFromSeconds(src.JoinConfiguration.Timeouts.TLSBootstrapSeconds)
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
	// Recover intent for bool values converted to *bool.
	if err := RestoreBoolIntentKubeadmConfigSpec(&src.Spec.Template.Spec, &dst.Spec.Template.Spec, ok, &restored.Spec.Template.Spec); err != nil {
		return err
	}

	// Recover other values
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

	dropEmptyStringsKubeadmConfigSpec(&dst.Spec.Template.Spec)

	// Preserve Hub data on down-conversion except for metadata.
	return utilconversion.MarshalData(src, dst)
}

func Convert_v1beta2_InitConfiguration_To_v1beta1_InitConfiguration(in *bootstrapv1.InitConfiguration, out *InitConfiguration, s apimachineryconversion.Scope) error {
	// Timeouts requires conversion at an upper level
	if err := autoConvert_v1beta2_InitConfiguration_To_v1beta1_InitConfiguration(in, out, s); err != nil {
		return err
	}
	if !reflect.DeepEqual(in.Patches, bootstrapv1.Patches{}) {
		out.Patches = &Patches{}
		if err := Convert_v1beta2_Patches_To_v1beta1_Patches(&in.Patches, out.Patches, s); err != nil {
			return err
		}
	}
	return nil
}

func Convert_v1beta2_JoinConfiguration_To_v1beta1_JoinConfiguration(in *bootstrapv1.JoinConfiguration, out *JoinConfiguration, s apimachineryconversion.Scope) error {
	// Timeouts requires conversion at an upper level
	if err := autoConvert_v1beta2_JoinConfiguration_To_v1beta1_JoinConfiguration(in, out, s); err != nil {
		return err
	}
	if !reflect.DeepEqual(in.Patches, bootstrapv1.Patches{}) {
		out.Patches = &Patches{}
		if err := Convert_v1beta2_Patches_To_v1beta1_Patches(&in.Patches, out.Patches, s); err != nil {
			return err
		}
	}
	return nil
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
	out.Ready = ptr.Deref(in.Initialization.DataSecretCreated, false)

	// Move new conditions (v1beta2) to the v1beta2 field.
	if in.Conditions == nil {
		return nil
	}
	out.V1Beta2 = &KubeadmConfigV1Beta2Status{}
	out.V1Beta2.Conditions = in.Conditions
	return nil
}

func Convert_v1beta2_APIServer_To_v1beta1_APIServer(in *bootstrapv1.APIServer, out *APIServer, s apimachineryconversion.Scope) error {
	// Following fields require a custom conversions.
	out.ExtraArgs = bootstrapv1.ConvertFromArgs(in.ExtraArgs)
	if in.ExtraEnvs == nil {
		out.ExtraEnvs = nil
	} else {
		out.ExtraEnvs = make([]EnvVar, len(*in.ExtraEnvs))
		for i := range *in.ExtraEnvs {
			if err := Convert_v1beta2_EnvVar_To_v1beta1_EnvVar(&(*in.ExtraEnvs)[i], &(out.ExtraEnvs)[i], s); err != nil {
				return err
			}
		}
	}
	if err := convert_v1beta2_ExtraVolumes_To_v1beta1_ExtraVolumes(&in.ExtraVolumes, &out.ExtraVolumes, s); err != nil {
		return err
	}
	return autoConvert_v1beta2_APIServer_To_v1beta1_APIServer(in, out, s)
}

func Convert_v1beta2_ControllerManager_To_v1beta1_ControlPlaneComponent(in *bootstrapv1.ControllerManager, out *ControlPlaneComponent, s apimachineryconversion.Scope) error {
	// Following fields require a custom conversions.
	out.ExtraArgs = bootstrapv1.ConvertFromArgs(in.ExtraArgs)
	if in.ExtraEnvs == nil {
		out.ExtraEnvs = nil
	} else {
		out.ExtraEnvs = make([]EnvVar, len(*in.ExtraEnvs))
		for i := range *in.ExtraEnvs {
			if err := Convert_v1beta2_EnvVar_To_v1beta1_EnvVar(&(*in.ExtraEnvs)[i], &(out.ExtraEnvs)[i], s); err != nil {
				return err
			}
		}
	}
	return convert_v1beta2_ExtraVolumes_To_v1beta1_ExtraVolumes(&in.ExtraVolumes, &out.ExtraVolumes, s)
}

func Convert_v1beta2_Scheduler_To_v1beta1_ControlPlaneComponent(in *bootstrapv1.Scheduler, out *ControlPlaneComponent, s apimachineryconversion.Scope) error {
	// Following fields require a custom conversions.
	out.ExtraArgs = bootstrapv1.ConvertFromArgs(in.ExtraArgs)
	if in.ExtraEnvs == nil {
		out.ExtraEnvs = nil
	} else {
		out.ExtraEnvs = make([]EnvVar, len(*in.ExtraEnvs))
		for i := range *in.ExtraEnvs {
			if err := Convert_v1beta2_EnvVar_To_v1beta1_EnvVar(&(*in.ExtraEnvs)[i], &(out.ExtraEnvs)[i], s); err != nil {
				return err
			}
		}
	}
	return convert_v1beta2_ExtraVolumes_To_v1beta1_ExtraVolumes(&in.ExtraVolumes, &out.ExtraVolumes, s)
}

func convert_v1beta2_ExtraVolumes_To_v1beta1_ExtraVolumes(in *[]bootstrapv1.HostPathMount, out *[]HostPathMount, s apimachineryconversion.Scope) error {
	if in != nil && len(*in) > 0 {
		*out = make([]HostPathMount, len(*in))
		for i := range *in {
			if err := Convert_v1beta2_HostPathMount_To_v1beta1_HostPathMount(&(*in)[i], &(*out)[i], s); err != nil {
				return err
			}
		}
	} else {
		*out = nil
	}
	return nil
}

func Convert_v1beta2_LocalEtcd_To_v1beta1_LocalEtcd(in *bootstrapv1.LocalEtcd, out *LocalEtcd, s apimachineryconversion.Scope) error {
	// Following fields require a custom conversions.
	out.ExtraArgs = bootstrapv1.ConvertFromArgs(in.ExtraArgs)
	if in.ExtraEnvs == nil {
		out.ExtraEnvs = nil
	} else {
		out.ExtraEnvs = make([]EnvVar, len(*in.ExtraEnvs))
		for i := range *in.ExtraEnvs {
			if err := Convert_v1beta2_EnvVar_To_v1beta1_EnvVar(&(*in.ExtraEnvs)[i], &(out.ExtraEnvs)[i], s); err != nil {
				return err
			}
		}
	}
	out.ImageRepository = in.ImageRepository
	out.ImageTag = in.ImageTag
	return autoConvert_v1beta2_LocalEtcd_To_v1beta1_LocalEtcd(in, out, s)
}

func Convert_v1beta2_NodeRegistrationOptions_To_v1beta1_NodeRegistrationOptions(in *bootstrapv1.NodeRegistrationOptions, out *NodeRegistrationOptions, s apimachineryconversion.Scope) error {
	// Following fields require a custom conversions.
	out.KubeletExtraArgs = bootstrapv1.ConvertFromArgs(in.KubeletExtraArgs)
	if in.Taints == nil {
		out.Taints = nil
	} else {
		out.Taints = *in.Taints
	}
	return autoConvert_v1beta2_NodeRegistrationOptions_To_v1beta1_NodeRegistrationOptions(in, out, s)
}

func Convert_v1beta2_BootstrapToken_To_v1beta1_BootstrapToken(in *bootstrapv1.BootstrapToken, out *BootstrapToken, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_BootstrapToken_To_v1beta1_BootstrapToken(in, out, s); err != nil {
		return err
	}
	out.TTL = clusterv1.ConvertFromSeconds(in.TTLSeconds)
	if !reflect.DeepEqual(in.Expires, metav1.Time{}) {
		out.Expires = ptr.To(in.Expires)
	}
	if !reflect.DeepEqual(in.Token, bootstrapv1.BootstrapTokenString{}) {
		out.Token = &BootstrapTokenString{}
		if err := autoConvert_v1beta2_BootstrapTokenString_To_v1beta1_BootstrapTokenString(&in.Token, out.Token, s); err != nil {
			return err
		}
	}
	return nil
}

func Convert_v1beta1_APIServer_To_v1beta2_APIServer(in *APIServer, out *bootstrapv1.APIServer, s apimachineryconversion.Scope) error {
	// TimeoutForControlPlane has been removed in v1beta2
	out.ExtraArgs = bootstrapv1.ConvertToArgs(in.ExtraArgs)
	if in.ExtraEnvs == nil {
		out.ExtraEnvs = nil
	} else {
		out.ExtraEnvs = ptr.To(make([]bootstrapv1.EnvVar, len(in.ExtraEnvs)))
		for i := range in.ExtraEnvs {
			if err := Convert_v1beta1_EnvVar_To_v1beta2_EnvVar(&(in.ExtraEnvs)[i], &(*out.ExtraEnvs)[i], s); err != nil {
				return err
			}
		}
	}
	if err := convert_v1beta1_ExtraVolumes_To_v1beta2_ExtraVolumes(&in.ExtraVolumes, &out.ExtraVolumes, s); err != nil {
		return err
	}
	return autoConvert_v1beta1_APIServer_To_v1beta2_APIServer(in, out, s)
}

func Convert_v1beta1_ControlPlaneComponent_To_v1beta2_ControllerManager(in *ControlPlaneComponent, out *bootstrapv1.ControllerManager, s apimachineryconversion.Scope) error {
	out.ExtraArgs = bootstrapv1.ConvertToArgs(in.ExtraArgs)
	if in.ExtraEnvs == nil {
		out.ExtraEnvs = nil
	} else {
		out.ExtraEnvs = ptr.To(make([]bootstrapv1.EnvVar, len(in.ExtraEnvs)))
		for i := range in.ExtraEnvs {
			if err := Convert_v1beta1_EnvVar_To_v1beta2_EnvVar(&(in.ExtraEnvs)[i], &(*out.ExtraEnvs)[i], s); err != nil {
				return err
			}
		}
	}
	return convert_v1beta1_ExtraVolumes_To_v1beta2_ExtraVolumes(&in.ExtraVolumes, &out.ExtraVolumes, s)
}

func Convert_v1beta1_ControlPlaneComponent_To_v1beta2_Scheduler(in *ControlPlaneComponent, out *bootstrapv1.Scheduler, s apimachineryconversion.Scope) error {
	out.ExtraArgs = bootstrapv1.ConvertToArgs(in.ExtraArgs)
	if in.ExtraEnvs == nil {
		out.ExtraEnvs = nil
	} else {
		out.ExtraEnvs = ptr.To(make([]bootstrapv1.EnvVar, len(in.ExtraEnvs)))
		for i := range in.ExtraEnvs {
			if err := Convert_v1beta1_EnvVar_To_v1beta2_EnvVar(&(in.ExtraEnvs)[i], &(*out.ExtraEnvs)[i], s); err != nil {
				return err
			}
		}
	}
	return convert_v1beta1_ExtraVolumes_To_v1beta2_ExtraVolumes(&in.ExtraVolumes, &out.ExtraVolumes, s)
}

func convert_v1beta1_ExtraVolumes_To_v1beta2_ExtraVolumes(in *[]HostPathMount, out *[]bootstrapv1.HostPathMount, s apimachineryconversion.Scope) error {
	if in != nil && len(*in) > 0 {
		*out = make([]bootstrapv1.HostPathMount, len(*in))
		for i := range *in {
			if err := Convert_v1beta1_HostPathMount_To_v1beta2_HostPathMount(&(*in)[i], &(*out)[i], s); err != nil {
				return err
			}
		}
	} else {
		*out = nil
	}
	return nil
}

func Convert_v1beta1_Discovery_To_v1beta2_Discovery(in *Discovery, out *bootstrapv1.Discovery, s apimachineryconversion.Scope) error {
	// Timeout has been removed in v1beta2
	if err := autoConvert_v1beta1_Discovery_To_v1beta2_Discovery(in, out, s); err != nil {
		return err
	}
	if in.BootstrapToken != nil {
		if err := Convert_v1beta1_BootstrapTokenDiscovery_To_v1beta2_BootstrapTokenDiscovery(in.BootstrapToken, &out.BootstrapToken, s); err != nil {
			return err
		}
	}
	if in.File != nil {
		if err := Convert_v1beta1_FileDiscovery_To_v1beta2_FileDiscovery(in.File, &out.File, s); err != nil {
			return err
		}
	}
	return nil
}

func Convert_v1beta1_ClusterConfiguration_To_v1beta2_ClusterConfiguration(in *ClusterConfiguration, out *bootstrapv1.ClusterConfiguration, s apimachineryconversion.Scope) error {
	return autoConvert_v1beta1_ClusterConfiguration_To_v1beta2_ClusterConfiguration(in, out, s)
}

func Convert_v1beta1_LocalEtcd_To_v1beta2_LocalEtcd(in *LocalEtcd, out *bootstrapv1.LocalEtcd, s apimachineryconversion.Scope) error {
	out.ExtraArgs = bootstrapv1.ConvertToArgs(in.ExtraArgs)
	if in.ExtraEnvs == nil {
		out.ExtraEnvs = nil
	} else {
		out.ExtraEnvs = ptr.To(make([]bootstrapv1.EnvVar, len(in.ExtraEnvs)))
		for i := range in.ExtraEnvs {
			if err := Convert_v1beta1_EnvVar_To_v1beta2_EnvVar(&(in.ExtraEnvs)[i], &(*out.ExtraEnvs)[i], s); err != nil {
				return err
			}
		}
	}
	out.ImageRepository = in.ImageRepository
	out.ImageTag = in.ImageTag
	return autoConvert_v1beta1_LocalEtcd_To_v1beta2_LocalEtcd(in, out, s)
}

func Convert_v1beta1_NodeRegistrationOptions_To_v1beta2_NodeRegistrationOptions(in *NodeRegistrationOptions, out *bootstrapv1.NodeRegistrationOptions, s apimachineryconversion.Scope) error {
	out.KubeletExtraArgs = bootstrapv1.ConvertToArgs(in.KubeletExtraArgs)
	if in.Taints == nil {
		out.Taints = nil
	} else {
		out.Taints = ptr.To(in.Taints)
	}
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

	// Move ready to Initialization is implemented in ConvertTo
	return nil
}

func Convert_v1beta1_BootstrapToken_To_v1beta2_BootstrapToken(in *BootstrapToken, out *bootstrapv1.BootstrapToken, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_BootstrapToken_To_v1beta2_BootstrapToken(in, out, s); err != nil {
		return err
	}
	out.TTLSeconds = clusterv1.ConvertToSeconds(in.TTL)
	if in.Expires != nil && !reflect.DeepEqual(in.Expires, &metav1.Time{}) {
		out.Expires = *in.Expires
	}
	if in.Token != nil {
		if err := autoConvert_v1beta1_BootstrapTokenString_To_v1beta2_BootstrapTokenString(in.Token, &out.Token, s); err != nil {
			return err
		}
	}
	return nil
}

func Convert_v1beta1_InitConfiguration_To_v1beta2_InitConfiguration(in *InitConfiguration, out *bootstrapv1.InitConfiguration, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_InitConfiguration_To_v1beta2_InitConfiguration(in, out, s); err != nil {
		return err
	}
	if in.Patches != nil {
		if err := Convert_v1beta1_Patches_To_v1beta2_Patches(in.Patches, &out.Patches, s); err != nil {
			return err
		}
	}
	return nil
}

func Convert_v1beta1_JoinConfiguration_To_v1beta2_JoinConfiguration(in *JoinConfiguration, out *bootstrapv1.JoinConfiguration, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_JoinConfiguration_To_v1beta2_JoinConfiguration(in, out, s); err != nil {
		return err
	}
	if in.Patches != nil {
		if err := Convert_v1beta1_Patches_To_v1beta2_Patches(in.Patches, &out.Patches, s); err != nil {
			return err
		}
	}
	return nil
}

func Convert_v1beta1_DNS_To_v1beta2_DNS(in *DNS, out *bootstrapv1.DNS, _ apimachineryconversion.Scope) error {
	out.ImageRepository = in.ImageRepository
	out.ImageTag = in.ImageTag
	return nil
}

func Convert_v1beta2_DNS_To_v1beta1_DNS(in *bootstrapv1.DNS, out *DNS, _ apimachineryconversion.Scope) error {
	out.ImageRepository = in.ImageRepository
	out.ImageTag = in.ImageTag
	return nil
}

func Convert_v1beta1_Etcd_To_v1beta2_Etcd(in *Etcd, out *bootstrapv1.Etcd, s apimachineryconversion.Scope) error {
	if in.Local != nil {
		if err := Convert_v1beta1_LocalEtcd_To_v1beta2_LocalEtcd(in.Local, &out.Local, s); err != nil {
			return err
		}
	}
	if in.External != nil {
		if err := Convert_v1beta1_ExternalEtcd_To_v1beta2_ExternalEtcd(in.External, &out.External, s); err != nil {
			return err
		}
	}
	return nil
}

func Convert_v1beta2_Etcd_To_v1beta1_Etcd(in *bootstrapv1.Etcd, out *Etcd, s apimachineryconversion.Scope) error {
	if in.Local.IsDefined() {
		out.Local = &LocalEtcd{}
		if err := Convert_v1beta2_LocalEtcd_To_v1beta1_LocalEtcd(&in.Local, out.Local, s); err != nil {
			return err
		}
	}
	if in.External.IsDefined() {
		out.External = &ExternalEtcd{}
		if err := Convert_v1beta2_ExternalEtcd_To_v1beta1_ExternalEtcd(&in.External, out.External, s); err != nil {
			return err
		}
	}
	return nil
}

func Convert_v1beta2_Discovery_To_v1beta1_Discovery(in *bootstrapv1.Discovery, out *Discovery, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_Discovery_To_v1beta1_Discovery(in, out, s); err != nil {
		return err
	}
	if !reflect.DeepEqual(in.BootstrapToken, bootstrapv1.BootstrapTokenDiscovery{}) {
		out.BootstrapToken = &BootstrapTokenDiscovery{}
		if err := Convert_v1beta2_BootstrapTokenDiscovery_To_v1beta1_BootstrapTokenDiscovery(&in.BootstrapToken, out.BootstrapToken, s); err != nil {
			return err
		}
	}
	if !reflect.DeepEqual(in.File, bootstrapv1.FileDiscovery{}) {
		out.File = &FileDiscovery{}
		if err := Convert_v1beta2_FileDiscovery_To_v1beta1_FileDiscovery(&in.File, out.File, s); err != nil {
			return err
		}
	}
	return nil
}

func Convert_v1beta1_FileDiscovery_To_v1beta2_FileDiscovery(in *FileDiscovery, out *bootstrapv1.FileDiscovery, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_FileDiscovery_To_v1beta2_FileDiscovery(in, out, s); err != nil {
		return err
	}
	if in.KubeConfig != nil {
		if err := Convert_v1beta1_FileDiscoveryKubeConfig_To_v1beta2_FileDiscoveryKubeConfig(in.KubeConfig, &out.KubeConfig, s); err != nil {
			return err
		}
	}
	return nil
}

func Convert_v1beta1_FileDiscoveryKubeConfig_To_v1beta2_FileDiscoveryKubeConfig(in *FileDiscoveryKubeConfig, out *bootstrapv1.FileDiscoveryKubeConfig, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_FileDiscoveryKubeConfig_To_v1beta2_FileDiscoveryKubeConfig(in, out, s); err != nil {
		return err
	}
	if in.Cluster != nil {
		if err := Convert_v1beta1_KubeConfigCluster_To_v1beta2_KubeConfigCluster(in.Cluster, &out.Cluster, s); err != nil {
			return err
		}
	}
	return nil
}

func Convert_v1beta1_KubeConfigUser_To_v1beta2_KubeConfigUser(in *KubeConfigUser, out *bootstrapv1.KubeConfigUser, s apimachineryconversion.Scope) error {
	if in.AuthProvider != nil {
		if err := Convert_v1beta1_KubeConfigAuthProvider_To_v1beta2_KubeConfigAuthProvider(in.AuthProvider, &out.AuthProvider, s); err != nil {
			return err
		}
	}
	if in.Exec != nil {
		if err := Convert_v1beta1_KubeConfigAuthExec_To_v1beta2_KubeConfigAuthExec(in.Exec, &out.Exec, s); err != nil {
			return err
		}
	}
	return nil
}

func Convert_v1beta2_FileDiscovery_To_v1beta1_FileDiscovery(in *bootstrapv1.FileDiscovery, out *FileDiscovery, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_FileDiscovery_To_v1beta1_FileDiscovery(in, out, s); err != nil {
		return err
	}
	if !reflect.DeepEqual(in.KubeConfig, bootstrapv1.FileDiscoveryKubeConfig{}) {
		out.KubeConfig = &FileDiscoveryKubeConfig{}
		if err := Convert_v1beta2_FileDiscoveryKubeConfig_To_v1beta1_FileDiscoveryKubeConfig(&in.KubeConfig, out.KubeConfig, s); err != nil {
			return err
		}
	}
	return nil
}

func Convert_v1beta2_FileDiscoveryKubeConfig_To_v1beta1_FileDiscoveryKubeConfig(in *bootstrapv1.FileDiscoveryKubeConfig, out *FileDiscoveryKubeConfig, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_FileDiscoveryKubeConfig_To_v1beta1_FileDiscoveryKubeConfig(in, out, s); err != nil {
		return err
	}
	if !reflect.DeepEqual(in.Cluster, bootstrapv1.KubeConfigCluster{}) {
		out.Cluster = &KubeConfigCluster{}
		if err := Convert_v1beta2_KubeConfigCluster_To_v1beta1_KubeConfigCluster(&in.Cluster, out.Cluster, s); err != nil {
			return err
		}
	}
	return nil
}

func Convert_v1beta2_KubeConfigUser_To_v1beta1_KubeConfigUser(in *bootstrapv1.KubeConfigUser, out *KubeConfigUser, s apimachineryconversion.Scope) error {
	if !reflect.DeepEqual(in.AuthProvider, bootstrapv1.KubeConfigAuthProvider{}) {
		out.AuthProvider = &KubeConfigAuthProvider{}
		if err := Convert_v1beta2_KubeConfigAuthProvider_To_v1beta1_KubeConfigAuthProvider(&in.AuthProvider, out.AuthProvider, s); err != nil {
			return err
		}
	}
	if !reflect.DeepEqual(in.Exec, bootstrapv1.KubeConfigAuthExec{}) {
		out.Exec = &KubeConfigAuthExec{}
		if err := Convert_v1beta2_KubeConfigAuthExec_To_v1beta1_KubeConfigAuthExec(&in.Exec, out.Exec, s); err != nil {
			return err
		}
	}
	return nil
}

func Convert_v1beta1_KubeadmConfigSpec_To_v1beta2_KubeadmConfigSpec(in *KubeadmConfigSpec, out *bootstrapv1.KubeadmConfigSpec, s apimachineryconversion.Scope) error {
	// NOTE: v1beta2 KubeadmConfigSpec does not have UseExperimentalRetryJoin anymore, so it's fine to just lose this field.
	if err := autoConvert_v1beta1_KubeadmConfigSpec_To_v1beta2_KubeadmConfigSpec(in, out, s); err != nil {
		return err
	}
	if in.ClusterConfiguration != nil {
		if err := Convert_v1beta1_ClusterConfiguration_To_v1beta2_ClusterConfiguration(in.ClusterConfiguration, &out.ClusterConfiguration, s); err != nil {
			return err
		}
	}
	if in.InitConfiguration != nil {
		if err := Convert_v1beta1_InitConfiguration_To_v1beta2_InitConfiguration(in.InitConfiguration, &out.InitConfiguration, s); err != nil {
			return err
		}
	}
	if in.JoinConfiguration != nil {
		if err := Convert_v1beta1_JoinConfiguration_To_v1beta2_JoinConfiguration(in.JoinConfiguration, &out.JoinConfiguration, s); err != nil {
			return err
		}
	}
	if in.DiskSetup != nil {
		if err := Convert_v1beta1_DiskSetup_To_v1beta2_DiskSetup(in.DiskSetup, &out.DiskSetup, s); err != nil {
			return err
		}
	}
	if in.NTP != nil {
		if err := Convert_v1beta1_NTP_To_v1beta2_NTP(in.NTP, &out.NTP, s); err != nil {
			return err
		}
	}
	if in.Ignition != nil {
		if err := Convert_v1beta1_IgnitionSpec_To_v1beta2_IgnitionSpec(in.Ignition, &out.Ignition, s); err != nil {
			return err
		}
	}
	return nil
}

func Convert_v1beta2_KubeadmConfigSpec_To_v1beta1_KubeadmConfigSpec(in *bootstrapv1.KubeadmConfigSpec, out *KubeadmConfigSpec, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_KubeadmConfigSpec_To_v1beta1_KubeadmConfigSpec(in, out, s); err != nil {
		return err
	}
	if !reflect.DeepEqual(in.ClusterConfiguration, bootstrapv1.ClusterConfiguration{}) {
		out.ClusterConfiguration = &ClusterConfiguration{}
		if err := Convert_v1beta2_ClusterConfiguration_To_v1beta1_ClusterConfiguration(&in.ClusterConfiguration, out.ClusterConfiguration, s); err != nil {
			return err
		}
	}
	if !reflect.DeepEqual(in.InitConfiguration, bootstrapv1.InitConfiguration{}) {
		out.InitConfiguration = &InitConfiguration{}
		if err := Convert_v1beta2_InitConfiguration_To_v1beta1_InitConfiguration(&in.InitConfiguration, out.InitConfiguration, s); err != nil {
			return err
		}
	}
	if !reflect.DeepEqual(in.JoinConfiguration, bootstrapv1.JoinConfiguration{}) {
		out.JoinConfiguration = &JoinConfiguration{}
		if err := Convert_v1beta2_JoinConfiguration_To_v1beta1_JoinConfiguration(&in.JoinConfiguration, out.JoinConfiguration, s); err != nil {
			return err
		}
	}
	if !reflect.DeepEqual(in.DiskSetup, bootstrapv1.DiskSetup{}) {
		out.DiskSetup = &DiskSetup{}
		if err := Convert_v1beta2_DiskSetup_To_v1beta1_DiskSetup(&in.DiskSetup, out.DiskSetup, s); err != nil {
			return err
		}
	}
	if !reflect.DeepEqual(in.NTP, bootstrapv1.NTP{}) {
		out.NTP = &NTP{}
		if err := Convert_v1beta2_NTP_To_v1beta1_NTP(&in.NTP, out.NTP, s); err != nil {
			return err
		}
	}
	if !reflect.DeepEqual(in.Ignition, bootstrapv1.IgnitionSpec{}) {
		out.Ignition = &IgnitionSpec{}
		if err := Convert_v1beta2_IgnitionSpec_To_v1beta1_IgnitionSpec(&in.Ignition, out.Ignition, s); err != nil {
			return err
		}
	}
	return nil
}

func Convert_v1beta1_IgnitionSpec_To_v1beta2_IgnitionSpec(in *IgnitionSpec, out *bootstrapv1.IgnitionSpec, s apimachineryconversion.Scope) error {
	if in.ContainerLinuxConfig != nil {
		if err := Convert_v1beta1_ContainerLinuxConfig_To_v1beta2_ContainerLinuxConfig(in.ContainerLinuxConfig, &out.ContainerLinuxConfig, s); err != nil {
			return err
		}
	}
	return nil
}

func Convert_v1beta2_IgnitionSpec_To_v1beta1_IgnitionSpec(in *bootstrapv1.IgnitionSpec, out *IgnitionSpec, s apimachineryconversion.Scope) error {
	if !reflect.DeepEqual(in.ContainerLinuxConfig, bootstrapv1.ContainerLinuxConfig{}) {
		out.ContainerLinuxConfig = &ContainerLinuxConfig{}
		if err := Convert_v1beta2_ContainerLinuxConfig_To_v1beta1_ContainerLinuxConfig(&in.ContainerLinuxConfig, out.ContainerLinuxConfig, s); err != nil {
			return err
		}
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

func Convert_v1beta2_ClusterConfiguration_To_v1beta1_ClusterConfiguration(in *bootstrapv1.ClusterConfiguration, out *ClusterConfiguration, s apimachineryconversion.Scope) error {
	return autoConvert_v1beta2_ClusterConfiguration_To_v1beta1_ClusterConfiguration(in, out, s)
}

func Convert_v1beta1_User_To_v1beta2_User(in *User, out *bootstrapv1.User, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_User_To_v1beta2_User(in, out, s); err != nil {
		return err
	}
	if in.PasswdFrom != nil {
		if err := Convert_v1beta1_PasswdSource_To_v1beta2_PasswdSource(in.PasswdFrom, &out.PasswdFrom, s); err != nil {
			return err
		}
	}
	return nil
}

func Convert_v1beta2_User_To_v1beta1_User(in *bootstrapv1.User, out *User, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_User_To_v1beta1_User(in, out, s); err != nil {
		return err
	}
	if in.PasswdFrom.IsDefined() {
		out.PasswdFrom = &PasswdSource{}
		if err := Convert_v1beta2_PasswdSource_To_v1beta1_PasswdSource(&in.PasswdFrom, out.PasswdFrom, s); err != nil {
			return err
		}
	}
	return nil
}

func Convert_v1beta1_File_To_v1beta2_File(in *File, out *bootstrapv1.File, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_File_To_v1beta2_File(in, out, s); err != nil {
		return err
	}
	if in.ContentFrom != nil {
		if err := Convert_v1beta1_FileSource_To_v1beta2_FileSource(in.ContentFrom, &out.ContentFrom, s); err != nil {
			return err
		}
	}
	return nil
}

func Convert_v1beta2_File_To_v1beta1_File(in *bootstrapv1.File, out *File, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_File_To_v1beta1_File(in, out, s); err != nil {
		return err
	}
	if in.ContentFrom.IsDefined() {
		out.ContentFrom = &FileSource{}
		if err := Convert_v1beta2_FileSource_To_v1beta1_FileSource(&in.ContentFrom, out.ContentFrom, s); err != nil {
			return err
		}
	}
	return nil
}

func dropEmptyStringsKubeadmConfigSpec(dst *KubeadmConfigSpec) {
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

func dropEmptyStringsKubeadmConfigStatus(dst *KubeadmConfigStatus) {
	dropEmptyString(&dst.DataSecretName)
}

func dropEmptyString(s **string) {
	if *s != nil && **s == "" {
		*s = nil
	}
}
