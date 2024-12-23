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

	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

func (src *KubeadmConfig) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*bootstrapv1.KubeadmConfig)

	if err := Convert_v1alpha4_KubeadmConfig_To_v1beta1_KubeadmConfig(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &bootstrapv1.KubeadmConfig{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}
	MergeRestoredKubeadmConfigSpec(&dst.Spec, &restored.Spec)
	dst.Status.V1Beta2 = restored.Status.V1Beta2

	return nil
}

func MergeRestoredKubeadmConfigSpec(dst *bootstrapv1.KubeadmConfigSpec, restored *bootstrapv1.KubeadmConfigSpec) {
	dst.Files = restored.Files

	dst.Users = restored.Users
	if restored.Users != nil {
		for i := range restored.Users {
			if restored.Users[i].PasswdFrom != nil {
				dst.Users[i].PasswdFrom = restored.Users[i].PasswdFrom
			}
		}
	}

	dst.BootCommands = restored.BootCommands
	dst.Ignition = restored.Ignition

	if restored.ClusterConfiguration != nil {
		if dst.ClusterConfiguration == nil {
			dst.ClusterConfiguration = &bootstrapv1.ClusterConfiguration{}
		}
		dst.ClusterConfiguration.APIServer.ExtraEnvs = restored.ClusterConfiguration.APIServer.ExtraEnvs
		dst.ClusterConfiguration.ControllerManager.ExtraEnvs = restored.ClusterConfiguration.ControllerManager.ExtraEnvs
		dst.ClusterConfiguration.Scheduler.ExtraEnvs = restored.ClusterConfiguration.Scheduler.ExtraEnvs

		if restored.ClusterConfiguration.Etcd.Local != nil {
			if dst.ClusterConfiguration.Etcd.Local == nil {
				dst.ClusterConfiguration.Etcd.Local = &bootstrapv1.LocalEtcd{}
			}
			dst.ClusterConfiguration.Etcd.Local.ExtraEnvs = restored.ClusterConfiguration.Etcd.Local.ExtraEnvs
		}
	}

	if restored.InitConfiguration != nil {
		if dst.InitConfiguration == nil {
			dst.InitConfiguration = &bootstrapv1.InitConfiguration{}
		}
		dst.InitConfiguration.Patches = restored.InitConfiguration.Patches
		dst.InitConfiguration.SkipPhases = restored.InitConfiguration.SkipPhases

		// Important! whenever adding fields to NodeRegistration, same fields must be added to hub.NodeRegistration's custom serialization func
		// otherwise those field won't exist in restored.

		dst.InitConfiguration.NodeRegistration.ImagePullPolicy = restored.InitConfiguration.NodeRegistration.ImagePullPolicy
		dst.InitConfiguration.NodeRegistration.ImagePullSerial = restored.InitConfiguration.NodeRegistration.ImagePullSerial
	}

	if restored.JoinConfiguration != nil {
		if dst.JoinConfiguration == nil {
			dst.JoinConfiguration = &bootstrapv1.JoinConfiguration{}
		}
		dst.JoinConfiguration.Patches = restored.JoinConfiguration.Patches
		dst.JoinConfiguration.SkipPhases = restored.JoinConfiguration.SkipPhases

		if restored.JoinConfiguration.Discovery.File != nil && restored.JoinConfiguration.Discovery.File.KubeConfig != nil {
			if dst.JoinConfiguration.Discovery.File == nil {
				dst.JoinConfiguration.Discovery.File = &bootstrapv1.FileDiscovery{}
			}
			dst.JoinConfiguration.Discovery.File.KubeConfig = restored.JoinConfiguration.Discovery.File.KubeConfig
		}

		// Important! whenever adding fields to NodeRegistration, same fields must be added to hub.NodeRegistration's custom serialization func
		// otherwise those field won't exist in restored.

		dst.JoinConfiguration.NodeRegistration.ImagePullPolicy = restored.JoinConfiguration.NodeRegistration.ImagePullPolicy
		dst.JoinConfiguration.NodeRegistration.ImagePullSerial = restored.JoinConfiguration.NodeRegistration.ImagePullSerial
	}
}

func (dst *KubeadmConfig) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*bootstrapv1.KubeadmConfig)

	if err := Convert_v1beta1_KubeadmConfig_To_v1alpha4_KubeadmConfig(src, dst, nil); err != nil {
		return err
	}
	// Preserve Hub data on down-conversion except for metadata.
	return utilconversion.MarshalData(src, dst)
}

func (src *KubeadmConfigList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*bootstrapv1.KubeadmConfigList)

	return Convert_v1alpha4_KubeadmConfigList_To_v1beta1_KubeadmConfigList(src, dst, nil)
}

func (dst *KubeadmConfigList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*bootstrapv1.KubeadmConfigList)

	return Convert_v1beta1_KubeadmConfigList_To_v1alpha4_KubeadmConfigList(src, dst, nil)
}

func (src *KubeadmConfigTemplate) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*bootstrapv1.KubeadmConfigTemplate)

	if err := Convert_v1alpha4_KubeadmConfigTemplate_To_v1beta1_KubeadmConfigTemplate(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &bootstrapv1.KubeadmConfigTemplate{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	dst.Spec.Template.ObjectMeta = restored.Spec.Template.ObjectMeta

	MergeRestoredKubeadmConfigSpec(&dst.Spec.Template.Spec, &restored.Spec.Template.Spec)

	return nil
}

func (dst *KubeadmConfigTemplate) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*bootstrapv1.KubeadmConfigTemplate)

	if err := Convert_v1beta1_KubeadmConfigTemplate_To_v1alpha4_KubeadmConfigTemplate(src, dst, nil); err != nil {
		return err
	}
	// Preserve Hub data on down-conversion except for metadata.
	return utilconversion.MarshalData(src, dst)
}

func (src *KubeadmConfigTemplateList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*bootstrapv1.KubeadmConfigTemplateList)

	return Convert_v1alpha4_KubeadmConfigTemplateList_To_v1beta1_KubeadmConfigTemplateList(src, dst, nil)
}

func (dst *KubeadmConfigTemplateList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*bootstrapv1.KubeadmConfigTemplateList)

	return Convert_v1beta1_KubeadmConfigTemplateList_To_v1alpha4_KubeadmConfigTemplateList(src, dst, nil)
}

// Convert_v1beta1_KubeadmConfigSpec_To_v1alpha4_KubeadmConfigSpec is an autogenerated conversion function.
func Convert_v1beta1_KubeadmConfigSpec_To_v1alpha4_KubeadmConfigSpec(in *bootstrapv1.KubeadmConfigSpec, out *KubeadmConfigSpec, s apiconversion.Scope) error {
	// KubeadmConfigSpec.Ignition does not exist in kubeadm v1alpha4 API.
	return autoConvert_v1beta1_KubeadmConfigSpec_To_v1alpha4_KubeadmConfigSpec(in, out, s)
}

func Convert_v1beta1_InitConfiguration_To_v1alpha4_InitConfiguration(in *bootstrapv1.InitConfiguration, out *InitConfiguration, s apiconversion.Scope) error {
	// InitConfiguration.Patches does not exist in kubeadm v1alpha4 API.
	return autoConvert_v1beta1_InitConfiguration_To_v1alpha4_InitConfiguration(in, out, s)
}

func Convert_v1beta1_JoinConfiguration_To_v1alpha4_JoinConfiguration(in *bootstrapv1.JoinConfiguration, out *JoinConfiguration, s apiconversion.Scope) error {
	// InitConfiguration.Patches does not exist in kubeadm v1alpha4 API.
	return autoConvert_v1beta1_JoinConfiguration_To_v1alpha4_JoinConfiguration(in, out, s)
}

func Convert_v1beta1_File_To_v1alpha4_File(in *bootstrapv1.File, out *File, s apiconversion.Scope) error {
	// File.Append does not exist in kubeadm v1alpha4 API.
	return autoConvert_v1beta1_File_To_v1alpha4_File(in, out, s)
}

func Convert_v1beta1_User_To_v1alpha4_User(in *bootstrapv1.User, out *User, s apiconversion.Scope) error {
	// User.PasswdFrom does not exist in kubeadm v1alpha4 API.
	return autoConvert_v1beta1_User_To_v1alpha4_User(in, out, s)
}

func Convert_v1beta1_NodeRegistrationOptions_To_v1alpha4_NodeRegistrationOptions(in *bootstrapv1.NodeRegistrationOptions, out *NodeRegistrationOptions, s apiconversion.Scope) error {
	// NodeRegistrationOptions.ImagePullPolicy does not exit in
	// kubeadm v1alpha4 API.
	return autoConvert_v1beta1_NodeRegistrationOptions_To_v1alpha4_NodeRegistrationOptions(in, out, s)
}

func Convert_v1beta1_KubeadmConfigTemplateResource_To_v1alpha4_KubeadmConfigTemplateResource(in *bootstrapv1.KubeadmConfigTemplateResource, out *KubeadmConfigTemplateResource, s apiconversion.Scope) error {
	// KubeadmConfigTemplateResource.metadata does not exist in kubeadm v1alpha4.
	return autoConvert_v1beta1_KubeadmConfigTemplateResource_To_v1alpha4_KubeadmConfigTemplateResource(in, out, s)
}

func Convert_v1beta1_FileDiscovery_To_v1alpha4_FileDiscovery(in *bootstrapv1.FileDiscovery, out *FileDiscovery, s apiconversion.Scope) error {
	// JoinConfiguration.Discovery.File.KubeConfig does not exist in v1alpha4 APIs.
	return autoConvert_v1beta1_FileDiscovery_To_v1alpha4_FileDiscovery(in, out, s)
}

func Convert_v1beta1_ControlPlaneComponent_To_v1alpha4_ControlPlaneComponent(in *bootstrapv1.ControlPlaneComponent, out *ControlPlaneComponent, s apiconversion.Scope) error {
	// ControlPlaneComponent.ExtraEnvs does not exist in v1alpha4 APIs.
	return autoConvert_v1beta1_ControlPlaneComponent_To_v1alpha4_ControlPlaneComponent(in, out, s)
}

func Convert_v1beta1_LocalEtcd_To_v1alpha4_LocalEtcd(in *bootstrapv1.LocalEtcd, out *LocalEtcd, s apiconversion.Scope) error {
	// LocalEtcd.ExtraEnvs does not exist in v1alpha4 APIs.
	return autoConvert_v1beta1_LocalEtcd_To_v1alpha4_LocalEtcd(in, out, s)
}

func Convert_v1beta1_KubeadmConfigStatus_To_v1alpha4_KubeadmConfigStatus(in *bootstrapv1.KubeadmConfigStatus, out *KubeadmConfigStatus, s apiconversion.Scope) error {
	// V1Beta2 was added in v1beta1.
	return autoConvert_v1beta1_KubeadmConfigStatus_To_v1alpha4_KubeadmConfigStatus(in, out, s)
}
