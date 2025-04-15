//go:build !ignore_autogenerated
// +build !ignore_autogenerated

/*
Copyright The Kubernetes Authors.

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

// Code generated by conversion-gen. DO NOT EDIT.

package v1alpha3

import (
	unsafe "unsafe"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	conversion "k8s.io/apimachinery/pkg/conversion"
	runtime "k8s.io/apimachinery/pkg/runtime"
	v1beta2 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta2"
	upstreamv1beta1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/upstreamv1beta1"
	corev1alpha3 "sigs.k8s.io/cluster-api/internal/apis/core/v1alpha3"
)

func init() {
	localSchemeBuilder.Register(RegisterConversions)
}

// RegisterConversions adds conversion functions to the given scheme.
// Public to allow building arbitrary schemes.
func RegisterConversions(s *runtime.Scheme) error {
	if err := s.AddGeneratedConversionFunc((*DiskSetup)(nil), (*v1beta2.DiskSetup)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha3_DiskSetup_To_v1beta2_DiskSetup(a.(*DiskSetup), b.(*v1beta2.DiskSetup), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*v1beta2.DiskSetup)(nil), (*DiskSetup)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta2_DiskSetup_To_v1alpha3_DiskSetup(a.(*v1beta2.DiskSetup), b.(*DiskSetup), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*File)(nil), (*v1beta2.File)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha3_File_To_v1beta2_File(a.(*File), b.(*v1beta2.File), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*FileSource)(nil), (*v1beta2.FileSource)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha3_FileSource_To_v1beta2_FileSource(a.(*FileSource), b.(*v1beta2.FileSource), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*v1beta2.FileSource)(nil), (*FileSource)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta2_FileSource_To_v1alpha3_FileSource(a.(*v1beta2.FileSource), b.(*FileSource), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*Filesystem)(nil), (*v1beta2.Filesystem)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha3_Filesystem_To_v1beta2_Filesystem(a.(*Filesystem), b.(*v1beta2.Filesystem), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*v1beta2.Filesystem)(nil), (*Filesystem)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta2_Filesystem_To_v1alpha3_Filesystem(a.(*v1beta2.Filesystem), b.(*Filesystem), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*KubeadmConfig)(nil), (*v1beta2.KubeadmConfig)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha3_KubeadmConfig_To_v1beta2_KubeadmConfig(a.(*KubeadmConfig), b.(*v1beta2.KubeadmConfig), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*v1beta2.KubeadmConfig)(nil), (*KubeadmConfig)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta2_KubeadmConfig_To_v1alpha3_KubeadmConfig(a.(*v1beta2.KubeadmConfig), b.(*KubeadmConfig), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*KubeadmConfigList)(nil), (*v1beta2.KubeadmConfigList)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha3_KubeadmConfigList_To_v1beta2_KubeadmConfigList(a.(*KubeadmConfigList), b.(*v1beta2.KubeadmConfigList), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*v1beta2.KubeadmConfigList)(nil), (*KubeadmConfigList)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta2_KubeadmConfigList_To_v1alpha3_KubeadmConfigList(a.(*v1beta2.KubeadmConfigList), b.(*KubeadmConfigList), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*KubeadmConfigSpec)(nil), (*v1beta2.KubeadmConfigSpec)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha3_KubeadmConfigSpec_To_v1beta2_KubeadmConfigSpec(a.(*KubeadmConfigSpec), b.(*v1beta2.KubeadmConfigSpec), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*KubeadmConfigTemplate)(nil), (*v1beta2.KubeadmConfigTemplate)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha3_KubeadmConfigTemplate_To_v1beta2_KubeadmConfigTemplate(a.(*KubeadmConfigTemplate), b.(*v1beta2.KubeadmConfigTemplate), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*v1beta2.KubeadmConfigTemplate)(nil), (*KubeadmConfigTemplate)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta2_KubeadmConfigTemplate_To_v1alpha3_KubeadmConfigTemplate(a.(*v1beta2.KubeadmConfigTemplate), b.(*KubeadmConfigTemplate), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*KubeadmConfigTemplateList)(nil), (*v1beta2.KubeadmConfigTemplateList)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha3_KubeadmConfigTemplateList_To_v1beta2_KubeadmConfigTemplateList(a.(*KubeadmConfigTemplateList), b.(*v1beta2.KubeadmConfigTemplateList), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*v1beta2.KubeadmConfigTemplateList)(nil), (*KubeadmConfigTemplateList)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta2_KubeadmConfigTemplateList_To_v1alpha3_KubeadmConfigTemplateList(a.(*v1beta2.KubeadmConfigTemplateList), b.(*KubeadmConfigTemplateList), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*KubeadmConfigTemplateResource)(nil), (*v1beta2.KubeadmConfigTemplateResource)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha3_KubeadmConfigTemplateResource_To_v1beta2_KubeadmConfigTemplateResource(a.(*KubeadmConfigTemplateResource), b.(*v1beta2.KubeadmConfigTemplateResource), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*KubeadmConfigTemplateSpec)(nil), (*v1beta2.KubeadmConfigTemplateSpec)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha3_KubeadmConfigTemplateSpec_To_v1beta2_KubeadmConfigTemplateSpec(a.(*KubeadmConfigTemplateSpec), b.(*v1beta2.KubeadmConfigTemplateSpec), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*v1beta2.KubeadmConfigTemplateSpec)(nil), (*KubeadmConfigTemplateSpec)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta2_KubeadmConfigTemplateSpec_To_v1alpha3_KubeadmConfigTemplateSpec(a.(*v1beta2.KubeadmConfigTemplateSpec), b.(*KubeadmConfigTemplateSpec), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*NTP)(nil), (*v1beta2.NTP)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha3_NTP_To_v1beta2_NTP(a.(*NTP), b.(*v1beta2.NTP), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*v1beta2.NTP)(nil), (*NTP)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta2_NTP_To_v1alpha3_NTP(a.(*v1beta2.NTP), b.(*NTP), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*Partition)(nil), (*v1beta2.Partition)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha3_Partition_To_v1beta2_Partition(a.(*Partition), b.(*v1beta2.Partition), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*v1beta2.Partition)(nil), (*Partition)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta2_Partition_To_v1alpha3_Partition(a.(*v1beta2.Partition), b.(*Partition), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*SecretFileSource)(nil), (*v1beta2.SecretFileSource)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha3_SecretFileSource_To_v1beta2_SecretFileSource(a.(*SecretFileSource), b.(*v1beta2.SecretFileSource), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*v1beta2.SecretFileSource)(nil), (*SecretFileSource)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta2_SecretFileSource_To_v1alpha3_SecretFileSource(a.(*v1beta2.SecretFileSource), b.(*SecretFileSource), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*User)(nil), (*v1beta2.User)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha3_User_To_v1beta2_User(a.(*User), b.(*v1beta2.User), scope)
	}); err != nil {
		return err
	}
	if err := s.AddConversionFunc((*upstreamv1beta1.ClusterConfiguration)(nil), (*v1beta2.ClusterConfiguration)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_upstreamv1beta1_ClusterConfiguration_To_v1beta2_ClusterConfiguration(a.(*upstreamv1beta1.ClusterConfiguration), b.(*v1beta2.ClusterConfiguration), scope)
	}); err != nil {
		return err
	}
	if err := s.AddConversionFunc((*upstreamv1beta1.InitConfiguration)(nil), (*v1beta2.InitConfiguration)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_upstreamv1beta1_InitConfiguration_To_v1beta2_InitConfiguration(a.(*upstreamv1beta1.InitConfiguration), b.(*v1beta2.InitConfiguration), scope)
	}); err != nil {
		return err
	}
	if err := s.AddConversionFunc((*upstreamv1beta1.JoinConfiguration)(nil), (*v1beta2.JoinConfiguration)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_upstreamv1beta1_JoinConfiguration_To_v1beta2_JoinConfiguration(a.(*upstreamv1beta1.JoinConfiguration), b.(*v1beta2.JoinConfiguration), scope)
	}); err != nil {
		return err
	}
	if err := s.AddConversionFunc((*v1.Condition)(nil), (*corev1alpha3.Condition)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1_Condition_To_v1alpha3_Condition(a.(*v1.Condition), b.(*corev1alpha3.Condition), scope)
	}); err != nil {
		return err
	}
	if err := s.AddConversionFunc((*corev1alpha3.Condition)(nil), (*v1.Condition)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha3_Condition_To_v1_Condition(a.(*corev1alpha3.Condition), b.(*v1.Condition), scope)
	}); err != nil {
		return err
	}
	if err := s.AddConversionFunc((*KubeadmConfigStatus)(nil), (*v1beta2.KubeadmConfigStatus)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha3_KubeadmConfigStatus_To_v1beta2_KubeadmConfigStatus(a.(*KubeadmConfigStatus), b.(*v1beta2.KubeadmConfigStatus), scope)
	}); err != nil {
		return err
	}
	if err := s.AddConversionFunc((*v1beta2.ClusterConfiguration)(nil), (*upstreamv1beta1.ClusterConfiguration)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta2_ClusterConfiguration_To_upstreamv1beta1_ClusterConfiguration(a.(*v1beta2.ClusterConfiguration), b.(*upstreamv1beta1.ClusterConfiguration), scope)
	}); err != nil {
		return err
	}
	if err := s.AddConversionFunc((*v1beta2.File)(nil), (*File)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta2_File_To_v1alpha3_File(a.(*v1beta2.File), b.(*File), scope)
	}); err != nil {
		return err
	}
	if err := s.AddConversionFunc((*v1beta2.InitConfiguration)(nil), (*upstreamv1beta1.InitConfiguration)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta2_InitConfiguration_To_upstreamv1beta1_InitConfiguration(a.(*v1beta2.InitConfiguration), b.(*upstreamv1beta1.InitConfiguration), scope)
	}); err != nil {
		return err
	}
	if err := s.AddConversionFunc((*v1beta2.JoinConfiguration)(nil), (*upstreamv1beta1.JoinConfiguration)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta2_JoinConfiguration_To_upstreamv1beta1_JoinConfiguration(a.(*v1beta2.JoinConfiguration), b.(*upstreamv1beta1.JoinConfiguration), scope)
	}); err != nil {
		return err
	}
	if err := s.AddConversionFunc((*v1beta2.KubeadmConfigSpec)(nil), (*KubeadmConfigSpec)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta2_KubeadmConfigSpec_To_v1alpha3_KubeadmConfigSpec(a.(*v1beta2.KubeadmConfigSpec), b.(*KubeadmConfigSpec), scope)
	}); err != nil {
		return err
	}
	if err := s.AddConversionFunc((*v1beta2.KubeadmConfigStatus)(nil), (*KubeadmConfigStatus)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta2_KubeadmConfigStatus_To_v1alpha3_KubeadmConfigStatus(a.(*v1beta2.KubeadmConfigStatus), b.(*KubeadmConfigStatus), scope)
	}); err != nil {
		return err
	}
	if err := s.AddConversionFunc((*v1beta2.KubeadmConfigTemplateResource)(nil), (*KubeadmConfigTemplateResource)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta2_KubeadmConfigTemplateResource_To_v1alpha3_KubeadmConfigTemplateResource(a.(*v1beta2.KubeadmConfigTemplateResource), b.(*KubeadmConfigTemplateResource), scope)
	}); err != nil {
		return err
	}
	if err := s.AddConversionFunc((*v1beta2.User)(nil), (*User)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta2_User_To_v1alpha3_User(a.(*v1beta2.User), b.(*User), scope)
	}); err != nil {
		return err
	}
	return nil
}

func autoConvert_v1alpha3_DiskSetup_To_v1beta2_DiskSetup(in *DiskSetup, out *v1beta2.DiskSetup, s conversion.Scope) error {
	out.Partitions = *(*[]v1beta2.Partition)(unsafe.Pointer(&in.Partitions))
	out.Filesystems = *(*[]v1beta2.Filesystem)(unsafe.Pointer(&in.Filesystems))
	return nil
}

// Convert_v1alpha3_DiskSetup_To_v1beta2_DiskSetup is an autogenerated conversion function.
func Convert_v1alpha3_DiskSetup_To_v1beta2_DiskSetup(in *DiskSetup, out *v1beta2.DiskSetup, s conversion.Scope) error {
	return autoConvert_v1alpha3_DiskSetup_To_v1beta2_DiskSetup(in, out, s)
}

func autoConvert_v1beta2_DiskSetup_To_v1alpha3_DiskSetup(in *v1beta2.DiskSetup, out *DiskSetup, s conversion.Scope) error {
	out.Partitions = *(*[]Partition)(unsafe.Pointer(&in.Partitions))
	out.Filesystems = *(*[]Filesystem)(unsafe.Pointer(&in.Filesystems))
	return nil
}

// Convert_v1beta2_DiskSetup_To_v1alpha3_DiskSetup is an autogenerated conversion function.
func Convert_v1beta2_DiskSetup_To_v1alpha3_DiskSetup(in *v1beta2.DiskSetup, out *DiskSetup, s conversion.Scope) error {
	return autoConvert_v1beta2_DiskSetup_To_v1alpha3_DiskSetup(in, out, s)
}

func autoConvert_v1alpha3_File_To_v1beta2_File(in *File, out *v1beta2.File, s conversion.Scope) error {
	out.Path = in.Path
	out.Owner = in.Owner
	out.Permissions = in.Permissions
	out.Encoding = v1beta2.Encoding(in.Encoding)
	out.Content = in.Content
	out.ContentFrom = (*v1beta2.FileSource)(unsafe.Pointer(in.ContentFrom))
	return nil
}

// Convert_v1alpha3_File_To_v1beta2_File is an autogenerated conversion function.
func Convert_v1alpha3_File_To_v1beta2_File(in *File, out *v1beta2.File, s conversion.Scope) error {
	return autoConvert_v1alpha3_File_To_v1beta2_File(in, out, s)
}

func autoConvert_v1beta2_File_To_v1alpha3_File(in *v1beta2.File, out *File, s conversion.Scope) error {
	out.Path = in.Path
	out.Owner = in.Owner
	out.Permissions = in.Permissions
	out.Encoding = Encoding(in.Encoding)
	// WARNING: in.Append requires manual conversion: does not exist in peer-type
	out.Content = in.Content
	out.ContentFrom = (*FileSource)(unsafe.Pointer(in.ContentFrom))
	return nil
}

func autoConvert_v1alpha3_FileSource_To_v1beta2_FileSource(in *FileSource, out *v1beta2.FileSource, s conversion.Scope) error {
	if err := Convert_v1alpha3_SecretFileSource_To_v1beta2_SecretFileSource(&in.Secret, &out.Secret, s); err != nil {
		return err
	}
	return nil
}

// Convert_v1alpha3_FileSource_To_v1beta2_FileSource is an autogenerated conversion function.
func Convert_v1alpha3_FileSource_To_v1beta2_FileSource(in *FileSource, out *v1beta2.FileSource, s conversion.Scope) error {
	return autoConvert_v1alpha3_FileSource_To_v1beta2_FileSource(in, out, s)
}

func autoConvert_v1beta2_FileSource_To_v1alpha3_FileSource(in *v1beta2.FileSource, out *FileSource, s conversion.Scope) error {
	if err := Convert_v1beta2_SecretFileSource_To_v1alpha3_SecretFileSource(&in.Secret, &out.Secret, s); err != nil {
		return err
	}
	return nil
}

// Convert_v1beta2_FileSource_To_v1alpha3_FileSource is an autogenerated conversion function.
func Convert_v1beta2_FileSource_To_v1alpha3_FileSource(in *v1beta2.FileSource, out *FileSource, s conversion.Scope) error {
	return autoConvert_v1beta2_FileSource_To_v1alpha3_FileSource(in, out, s)
}

func autoConvert_v1alpha3_Filesystem_To_v1beta2_Filesystem(in *Filesystem, out *v1beta2.Filesystem, s conversion.Scope) error {
	out.Device = in.Device
	out.Filesystem = in.Filesystem
	out.Label = in.Label
	out.Partition = (*string)(unsafe.Pointer(in.Partition))
	out.Overwrite = (*bool)(unsafe.Pointer(in.Overwrite))
	out.ReplaceFS = (*string)(unsafe.Pointer(in.ReplaceFS))
	out.ExtraOpts = *(*[]string)(unsafe.Pointer(&in.ExtraOpts))
	return nil
}

// Convert_v1alpha3_Filesystem_To_v1beta2_Filesystem is an autogenerated conversion function.
func Convert_v1alpha3_Filesystem_To_v1beta2_Filesystem(in *Filesystem, out *v1beta2.Filesystem, s conversion.Scope) error {
	return autoConvert_v1alpha3_Filesystem_To_v1beta2_Filesystem(in, out, s)
}

func autoConvert_v1beta2_Filesystem_To_v1alpha3_Filesystem(in *v1beta2.Filesystem, out *Filesystem, s conversion.Scope) error {
	out.Device = in.Device
	out.Filesystem = in.Filesystem
	out.Label = in.Label
	out.Partition = (*string)(unsafe.Pointer(in.Partition))
	out.Overwrite = (*bool)(unsafe.Pointer(in.Overwrite))
	out.ReplaceFS = (*string)(unsafe.Pointer(in.ReplaceFS))
	out.ExtraOpts = *(*[]string)(unsafe.Pointer(&in.ExtraOpts))
	return nil
}

// Convert_v1beta2_Filesystem_To_v1alpha3_Filesystem is an autogenerated conversion function.
func Convert_v1beta2_Filesystem_To_v1alpha3_Filesystem(in *v1beta2.Filesystem, out *Filesystem, s conversion.Scope) error {
	return autoConvert_v1beta2_Filesystem_To_v1alpha3_Filesystem(in, out, s)
}

func autoConvert_v1alpha3_KubeadmConfig_To_v1beta2_KubeadmConfig(in *KubeadmConfig, out *v1beta2.KubeadmConfig, s conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	if err := Convert_v1alpha3_KubeadmConfigSpec_To_v1beta2_KubeadmConfigSpec(&in.Spec, &out.Spec, s); err != nil {
		return err
	}
	if err := Convert_v1alpha3_KubeadmConfigStatus_To_v1beta2_KubeadmConfigStatus(&in.Status, &out.Status, s); err != nil {
		return err
	}
	return nil
}

// Convert_v1alpha3_KubeadmConfig_To_v1beta2_KubeadmConfig is an autogenerated conversion function.
func Convert_v1alpha3_KubeadmConfig_To_v1beta2_KubeadmConfig(in *KubeadmConfig, out *v1beta2.KubeadmConfig, s conversion.Scope) error {
	return autoConvert_v1alpha3_KubeadmConfig_To_v1beta2_KubeadmConfig(in, out, s)
}

func autoConvert_v1beta2_KubeadmConfig_To_v1alpha3_KubeadmConfig(in *v1beta2.KubeadmConfig, out *KubeadmConfig, s conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	if err := Convert_v1beta2_KubeadmConfigSpec_To_v1alpha3_KubeadmConfigSpec(&in.Spec, &out.Spec, s); err != nil {
		return err
	}
	if err := Convert_v1beta2_KubeadmConfigStatus_To_v1alpha3_KubeadmConfigStatus(&in.Status, &out.Status, s); err != nil {
		return err
	}
	return nil
}

// Convert_v1beta2_KubeadmConfig_To_v1alpha3_KubeadmConfig is an autogenerated conversion function.
func Convert_v1beta2_KubeadmConfig_To_v1alpha3_KubeadmConfig(in *v1beta2.KubeadmConfig, out *KubeadmConfig, s conversion.Scope) error {
	return autoConvert_v1beta2_KubeadmConfig_To_v1alpha3_KubeadmConfig(in, out, s)
}

func autoConvert_v1alpha3_KubeadmConfigList_To_v1beta2_KubeadmConfigList(in *KubeadmConfigList, out *v1beta2.KubeadmConfigList, s conversion.Scope) error {
	out.ListMeta = in.ListMeta
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]v1beta2.KubeadmConfig, len(*in))
		for i := range *in {
			if err := Convert_v1alpha3_KubeadmConfig_To_v1beta2_KubeadmConfig(&(*in)[i], &(*out)[i], s); err != nil {
				return err
			}
		}
	} else {
		out.Items = nil
	}
	return nil
}

// Convert_v1alpha3_KubeadmConfigList_To_v1beta2_KubeadmConfigList is an autogenerated conversion function.
func Convert_v1alpha3_KubeadmConfigList_To_v1beta2_KubeadmConfigList(in *KubeadmConfigList, out *v1beta2.KubeadmConfigList, s conversion.Scope) error {
	return autoConvert_v1alpha3_KubeadmConfigList_To_v1beta2_KubeadmConfigList(in, out, s)
}

func autoConvert_v1beta2_KubeadmConfigList_To_v1alpha3_KubeadmConfigList(in *v1beta2.KubeadmConfigList, out *KubeadmConfigList, s conversion.Scope) error {
	out.ListMeta = in.ListMeta
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]KubeadmConfig, len(*in))
		for i := range *in {
			if err := Convert_v1beta2_KubeadmConfig_To_v1alpha3_KubeadmConfig(&(*in)[i], &(*out)[i], s); err != nil {
				return err
			}
		}
	} else {
		out.Items = nil
	}
	return nil
}

// Convert_v1beta2_KubeadmConfigList_To_v1alpha3_KubeadmConfigList is an autogenerated conversion function.
func Convert_v1beta2_KubeadmConfigList_To_v1alpha3_KubeadmConfigList(in *v1beta2.KubeadmConfigList, out *KubeadmConfigList, s conversion.Scope) error {
	return autoConvert_v1beta2_KubeadmConfigList_To_v1alpha3_KubeadmConfigList(in, out, s)
}

func autoConvert_v1alpha3_KubeadmConfigSpec_To_v1beta2_KubeadmConfigSpec(in *KubeadmConfigSpec, out *v1beta2.KubeadmConfigSpec, s conversion.Scope) error {
	if in.ClusterConfiguration != nil {
		in, out := &in.ClusterConfiguration, &out.ClusterConfiguration
		*out = new(v1beta2.ClusterConfiguration)
		if err := Convert_upstreamv1beta1_ClusterConfiguration_To_v1beta2_ClusterConfiguration(*in, *out, s); err != nil {
			return err
		}
	} else {
		out.ClusterConfiguration = nil
	}
	if in.InitConfiguration != nil {
		in, out := &in.InitConfiguration, &out.InitConfiguration
		*out = new(v1beta2.InitConfiguration)
		if err := Convert_upstreamv1beta1_InitConfiguration_To_v1beta2_InitConfiguration(*in, *out, s); err != nil {
			return err
		}
	} else {
		out.InitConfiguration = nil
	}
	if in.JoinConfiguration != nil {
		in, out := &in.JoinConfiguration, &out.JoinConfiguration
		*out = new(v1beta2.JoinConfiguration)
		if err := Convert_upstreamv1beta1_JoinConfiguration_To_v1beta2_JoinConfiguration(*in, *out, s); err != nil {
			return err
		}
	} else {
		out.JoinConfiguration = nil
	}
	if in.Files != nil {
		in, out := &in.Files, &out.Files
		*out = make([]v1beta2.File, len(*in))
		for i := range *in {
			if err := Convert_v1alpha3_File_To_v1beta2_File(&(*in)[i], &(*out)[i], s); err != nil {
				return err
			}
		}
	} else {
		out.Files = nil
	}
	out.DiskSetup = (*v1beta2.DiskSetup)(unsafe.Pointer(in.DiskSetup))
	out.Mounts = *(*[]v1beta2.MountPoints)(unsafe.Pointer(&in.Mounts))
	out.PreKubeadmCommands = *(*[]string)(unsafe.Pointer(&in.PreKubeadmCommands))
	out.PostKubeadmCommands = *(*[]string)(unsafe.Pointer(&in.PostKubeadmCommands))
	if in.Users != nil {
		in, out := &in.Users, &out.Users
		*out = make([]v1beta2.User, len(*in))
		for i := range *in {
			if err := Convert_v1alpha3_User_To_v1beta2_User(&(*in)[i], &(*out)[i], s); err != nil {
				return err
			}
		}
	} else {
		out.Users = nil
	}
	out.NTP = (*v1beta2.NTP)(unsafe.Pointer(in.NTP))
	out.Format = v1beta2.Format(in.Format)
	out.Verbosity = (*int32)(unsafe.Pointer(in.Verbosity))
	out.UseExperimentalRetryJoin = in.UseExperimentalRetryJoin
	return nil
}

// Convert_v1alpha3_KubeadmConfigSpec_To_v1beta2_KubeadmConfigSpec is an autogenerated conversion function.
func Convert_v1alpha3_KubeadmConfigSpec_To_v1beta2_KubeadmConfigSpec(in *KubeadmConfigSpec, out *v1beta2.KubeadmConfigSpec, s conversion.Scope) error {
	return autoConvert_v1alpha3_KubeadmConfigSpec_To_v1beta2_KubeadmConfigSpec(in, out, s)
}

func autoConvert_v1beta2_KubeadmConfigSpec_To_v1alpha3_KubeadmConfigSpec(in *v1beta2.KubeadmConfigSpec, out *KubeadmConfigSpec, s conversion.Scope) error {
	if in.ClusterConfiguration != nil {
		in, out := &in.ClusterConfiguration, &out.ClusterConfiguration
		*out = new(upstreamv1beta1.ClusterConfiguration)
		if err := Convert_v1beta2_ClusterConfiguration_To_upstreamv1beta1_ClusterConfiguration(*in, *out, s); err != nil {
			return err
		}
	} else {
		out.ClusterConfiguration = nil
	}
	if in.InitConfiguration != nil {
		in, out := &in.InitConfiguration, &out.InitConfiguration
		*out = new(upstreamv1beta1.InitConfiguration)
		if err := Convert_v1beta2_InitConfiguration_To_upstreamv1beta1_InitConfiguration(*in, *out, s); err != nil {
			return err
		}
	} else {
		out.InitConfiguration = nil
	}
	if in.JoinConfiguration != nil {
		in, out := &in.JoinConfiguration, &out.JoinConfiguration
		*out = new(upstreamv1beta1.JoinConfiguration)
		if err := Convert_v1beta2_JoinConfiguration_To_upstreamv1beta1_JoinConfiguration(*in, *out, s); err != nil {
			return err
		}
	} else {
		out.JoinConfiguration = nil
	}
	if in.Files != nil {
		in, out := &in.Files, &out.Files
		*out = make([]File, len(*in))
		for i := range *in {
			if err := Convert_v1beta2_File_To_v1alpha3_File(&(*in)[i], &(*out)[i], s); err != nil {
				return err
			}
		}
	} else {
		out.Files = nil
	}
	out.DiskSetup = (*DiskSetup)(unsafe.Pointer(in.DiskSetup))
	out.Mounts = *(*[]MountPoints)(unsafe.Pointer(&in.Mounts))
	// WARNING: in.BootCommands requires manual conversion: does not exist in peer-type
	out.PreKubeadmCommands = *(*[]string)(unsafe.Pointer(&in.PreKubeadmCommands))
	out.PostKubeadmCommands = *(*[]string)(unsafe.Pointer(&in.PostKubeadmCommands))
	if in.Users != nil {
		in, out := &in.Users, &out.Users
		*out = make([]User, len(*in))
		for i := range *in {
			if err := Convert_v1beta2_User_To_v1alpha3_User(&(*in)[i], &(*out)[i], s); err != nil {
				return err
			}
		}
	} else {
		out.Users = nil
	}
	out.NTP = (*NTP)(unsafe.Pointer(in.NTP))
	out.Format = Format(in.Format)
	out.Verbosity = (*int32)(unsafe.Pointer(in.Verbosity))
	out.UseExperimentalRetryJoin = in.UseExperimentalRetryJoin
	// WARNING: in.Ignition requires manual conversion: does not exist in peer-type
	return nil
}

func autoConvert_v1alpha3_KubeadmConfigStatus_To_v1beta2_KubeadmConfigStatus(in *KubeadmConfigStatus, out *v1beta2.KubeadmConfigStatus, s conversion.Scope) error {
	// WARNING: in.Ready requires manual conversion: does not exist in peer-type
	out.DataSecretName = (*string)(unsafe.Pointer(in.DataSecretName))
	// WARNING: in.BootstrapData requires manual conversion: does not exist in peer-type
	// WARNING: in.FailureReason requires manual conversion: does not exist in peer-type
	// WARNING: in.FailureMessage requires manual conversion: does not exist in peer-type
	out.ObservedGeneration = in.ObservedGeneration
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]v1.Condition, len(*in))
		for i := range *in {
			if err := Convert_v1alpha3_Condition_To_v1_Condition(&(*in)[i], &(*out)[i], s); err != nil {
				return err
			}
		}
	} else {
		out.Conditions = nil
	}
	return nil
}

func autoConvert_v1beta2_KubeadmConfigStatus_To_v1alpha3_KubeadmConfigStatus(in *v1beta2.KubeadmConfigStatus, out *KubeadmConfigStatus, s conversion.Scope) error {
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make(corev1alpha3.Conditions, len(*in))
		for i := range *in {
			if err := Convert_v1_Condition_To_v1alpha3_Condition(&(*in)[i], &(*out)[i], s); err != nil {
				return err
			}
		}
	} else {
		out.Conditions = nil
	}
	// WARNING: in.Initialization requires manual conversion: does not exist in peer-type
	out.DataSecretName = (*string)(unsafe.Pointer(in.DataSecretName))
	out.ObservedGeneration = in.ObservedGeneration
	// WARNING: in.Deprecated requires manual conversion: does not exist in peer-type
	return nil
}

func autoConvert_v1alpha3_KubeadmConfigTemplate_To_v1beta2_KubeadmConfigTemplate(in *KubeadmConfigTemplate, out *v1beta2.KubeadmConfigTemplate, s conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	if err := Convert_v1alpha3_KubeadmConfigTemplateSpec_To_v1beta2_KubeadmConfigTemplateSpec(&in.Spec, &out.Spec, s); err != nil {
		return err
	}
	return nil
}

// Convert_v1alpha3_KubeadmConfigTemplate_To_v1beta2_KubeadmConfigTemplate is an autogenerated conversion function.
func Convert_v1alpha3_KubeadmConfigTemplate_To_v1beta2_KubeadmConfigTemplate(in *KubeadmConfigTemplate, out *v1beta2.KubeadmConfigTemplate, s conversion.Scope) error {
	return autoConvert_v1alpha3_KubeadmConfigTemplate_To_v1beta2_KubeadmConfigTemplate(in, out, s)
}

func autoConvert_v1beta2_KubeadmConfigTemplate_To_v1alpha3_KubeadmConfigTemplate(in *v1beta2.KubeadmConfigTemplate, out *KubeadmConfigTemplate, s conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	if err := Convert_v1beta2_KubeadmConfigTemplateSpec_To_v1alpha3_KubeadmConfigTemplateSpec(&in.Spec, &out.Spec, s); err != nil {
		return err
	}
	return nil
}

// Convert_v1beta2_KubeadmConfigTemplate_To_v1alpha3_KubeadmConfigTemplate is an autogenerated conversion function.
func Convert_v1beta2_KubeadmConfigTemplate_To_v1alpha3_KubeadmConfigTemplate(in *v1beta2.KubeadmConfigTemplate, out *KubeadmConfigTemplate, s conversion.Scope) error {
	return autoConvert_v1beta2_KubeadmConfigTemplate_To_v1alpha3_KubeadmConfigTemplate(in, out, s)
}

func autoConvert_v1alpha3_KubeadmConfigTemplateList_To_v1beta2_KubeadmConfigTemplateList(in *KubeadmConfigTemplateList, out *v1beta2.KubeadmConfigTemplateList, s conversion.Scope) error {
	out.ListMeta = in.ListMeta
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]v1beta2.KubeadmConfigTemplate, len(*in))
		for i := range *in {
			if err := Convert_v1alpha3_KubeadmConfigTemplate_To_v1beta2_KubeadmConfigTemplate(&(*in)[i], &(*out)[i], s); err != nil {
				return err
			}
		}
	} else {
		out.Items = nil
	}
	return nil
}

// Convert_v1alpha3_KubeadmConfigTemplateList_To_v1beta2_KubeadmConfigTemplateList is an autogenerated conversion function.
func Convert_v1alpha3_KubeadmConfigTemplateList_To_v1beta2_KubeadmConfigTemplateList(in *KubeadmConfigTemplateList, out *v1beta2.KubeadmConfigTemplateList, s conversion.Scope) error {
	return autoConvert_v1alpha3_KubeadmConfigTemplateList_To_v1beta2_KubeadmConfigTemplateList(in, out, s)
}

func autoConvert_v1beta2_KubeadmConfigTemplateList_To_v1alpha3_KubeadmConfigTemplateList(in *v1beta2.KubeadmConfigTemplateList, out *KubeadmConfigTemplateList, s conversion.Scope) error {
	out.ListMeta = in.ListMeta
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]KubeadmConfigTemplate, len(*in))
		for i := range *in {
			if err := Convert_v1beta2_KubeadmConfigTemplate_To_v1alpha3_KubeadmConfigTemplate(&(*in)[i], &(*out)[i], s); err != nil {
				return err
			}
		}
	} else {
		out.Items = nil
	}
	return nil
}

// Convert_v1beta2_KubeadmConfigTemplateList_To_v1alpha3_KubeadmConfigTemplateList is an autogenerated conversion function.
func Convert_v1beta2_KubeadmConfigTemplateList_To_v1alpha3_KubeadmConfigTemplateList(in *v1beta2.KubeadmConfigTemplateList, out *KubeadmConfigTemplateList, s conversion.Scope) error {
	return autoConvert_v1beta2_KubeadmConfigTemplateList_To_v1alpha3_KubeadmConfigTemplateList(in, out, s)
}

func autoConvert_v1alpha3_KubeadmConfigTemplateResource_To_v1beta2_KubeadmConfigTemplateResource(in *KubeadmConfigTemplateResource, out *v1beta2.KubeadmConfigTemplateResource, s conversion.Scope) error {
	if err := Convert_v1alpha3_KubeadmConfigSpec_To_v1beta2_KubeadmConfigSpec(&in.Spec, &out.Spec, s); err != nil {
		return err
	}
	return nil
}

// Convert_v1alpha3_KubeadmConfigTemplateResource_To_v1beta2_KubeadmConfigTemplateResource is an autogenerated conversion function.
func Convert_v1alpha3_KubeadmConfigTemplateResource_To_v1beta2_KubeadmConfigTemplateResource(in *KubeadmConfigTemplateResource, out *v1beta2.KubeadmConfigTemplateResource, s conversion.Scope) error {
	return autoConvert_v1alpha3_KubeadmConfigTemplateResource_To_v1beta2_KubeadmConfigTemplateResource(in, out, s)
}

func autoConvert_v1beta2_KubeadmConfigTemplateResource_To_v1alpha3_KubeadmConfigTemplateResource(in *v1beta2.KubeadmConfigTemplateResource, out *KubeadmConfigTemplateResource, s conversion.Scope) error {
	// WARNING: in.ObjectMeta requires manual conversion: does not exist in peer-type
	if err := Convert_v1beta2_KubeadmConfigSpec_To_v1alpha3_KubeadmConfigSpec(&in.Spec, &out.Spec, s); err != nil {
		return err
	}
	return nil
}

func autoConvert_v1alpha3_KubeadmConfigTemplateSpec_To_v1beta2_KubeadmConfigTemplateSpec(in *KubeadmConfigTemplateSpec, out *v1beta2.KubeadmConfigTemplateSpec, s conversion.Scope) error {
	if err := Convert_v1alpha3_KubeadmConfigTemplateResource_To_v1beta2_KubeadmConfigTemplateResource(&in.Template, &out.Template, s); err != nil {
		return err
	}
	return nil
}

// Convert_v1alpha3_KubeadmConfigTemplateSpec_To_v1beta2_KubeadmConfigTemplateSpec is an autogenerated conversion function.
func Convert_v1alpha3_KubeadmConfigTemplateSpec_To_v1beta2_KubeadmConfigTemplateSpec(in *KubeadmConfigTemplateSpec, out *v1beta2.KubeadmConfigTemplateSpec, s conversion.Scope) error {
	return autoConvert_v1alpha3_KubeadmConfigTemplateSpec_To_v1beta2_KubeadmConfigTemplateSpec(in, out, s)
}

func autoConvert_v1beta2_KubeadmConfigTemplateSpec_To_v1alpha3_KubeadmConfigTemplateSpec(in *v1beta2.KubeadmConfigTemplateSpec, out *KubeadmConfigTemplateSpec, s conversion.Scope) error {
	if err := Convert_v1beta2_KubeadmConfigTemplateResource_To_v1alpha3_KubeadmConfigTemplateResource(&in.Template, &out.Template, s); err != nil {
		return err
	}
	return nil
}

// Convert_v1beta2_KubeadmConfigTemplateSpec_To_v1alpha3_KubeadmConfigTemplateSpec is an autogenerated conversion function.
func Convert_v1beta2_KubeadmConfigTemplateSpec_To_v1alpha3_KubeadmConfigTemplateSpec(in *v1beta2.KubeadmConfigTemplateSpec, out *KubeadmConfigTemplateSpec, s conversion.Scope) error {
	return autoConvert_v1beta2_KubeadmConfigTemplateSpec_To_v1alpha3_KubeadmConfigTemplateSpec(in, out, s)
}

func autoConvert_v1alpha3_NTP_To_v1beta2_NTP(in *NTP, out *v1beta2.NTP, s conversion.Scope) error {
	out.Servers = *(*[]string)(unsafe.Pointer(&in.Servers))
	out.Enabled = (*bool)(unsafe.Pointer(in.Enabled))
	return nil
}

// Convert_v1alpha3_NTP_To_v1beta2_NTP is an autogenerated conversion function.
func Convert_v1alpha3_NTP_To_v1beta2_NTP(in *NTP, out *v1beta2.NTP, s conversion.Scope) error {
	return autoConvert_v1alpha3_NTP_To_v1beta2_NTP(in, out, s)
}

func autoConvert_v1beta2_NTP_To_v1alpha3_NTP(in *v1beta2.NTP, out *NTP, s conversion.Scope) error {
	out.Servers = *(*[]string)(unsafe.Pointer(&in.Servers))
	out.Enabled = (*bool)(unsafe.Pointer(in.Enabled))
	return nil
}

// Convert_v1beta2_NTP_To_v1alpha3_NTP is an autogenerated conversion function.
func Convert_v1beta2_NTP_To_v1alpha3_NTP(in *v1beta2.NTP, out *NTP, s conversion.Scope) error {
	return autoConvert_v1beta2_NTP_To_v1alpha3_NTP(in, out, s)
}

func autoConvert_v1alpha3_Partition_To_v1beta2_Partition(in *Partition, out *v1beta2.Partition, s conversion.Scope) error {
	out.Device = in.Device
	out.Layout = in.Layout
	out.Overwrite = (*bool)(unsafe.Pointer(in.Overwrite))
	out.TableType = (*string)(unsafe.Pointer(in.TableType))
	return nil
}

// Convert_v1alpha3_Partition_To_v1beta2_Partition is an autogenerated conversion function.
func Convert_v1alpha3_Partition_To_v1beta2_Partition(in *Partition, out *v1beta2.Partition, s conversion.Scope) error {
	return autoConvert_v1alpha3_Partition_To_v1beta2_Partition(in, out, s)
}

func autoConvert_v1beta2_Partition_To_v1alpha3_Partition(in *v1beta2.Partition, out *Partition, s conversion.Scope) error {
	out.Device = in.Device
	out.Layout = in.Layout
	out.Overwrite = (*bool)(unsafe.Pointer(in.Overwrite))
	out.TableType = (*string)(unsafe.Pointer(in.TableType))
	return nil
}

// Convert_v1beta2_Partition_To_v1alpha3_Partition is an autogenerated conversion function.
func Convert_v1beta2_Partition_To_v1alpha3_Partition(in *v1beta2.Partition, out *Partition, s conversion.Scope) error {
	return autoConvert_v1beta2_Partition_To_v1alpha3_Partition(in, out, s)
}

func autoConvert_v1alpha3_SecretFileSource_To_v1beta2_SecretFileSource(in *SecretFileSource, out *v1beta2.SecretFileSource, s conversion.Scope) error {
	out.Name = in.Name
	out.Key = in.Key
	return nil
}

// Convert_v1alpha3_SecretFileSource_To_v1beta2_SecretFileSource is an autogenerated conversion function.
func Convert_v1alpha3_SecretFileSource_To_v1beta2_SecretFileSource(in *SecretFileSource, out *v1beta2.SecretFileSource, s conversion.Scope) error {
	return autoConvert_v1alpha3_SecretFileSource_To_v1beta2_SecretFileSource(in, out, s)
}

func autoConvert_v1beta2_SecretFileSource_To_v1alpha3_SecretFileSource(in *v1beta2.SecretFileSource, out *SecretFileSource, s conversion.Scope) error {
	out.Name = in.Name
	out.Key = in.Key
	return nil
}

// Convert_v1beta2_SecretFileSource_To_v1alpha3_SecretFileSource is an autogenerated conversion function.
func Convert_v1beta2_SecretFileSource_To_v1alpha3_SecretFileSource(in *v1beta2.SecretFileSource, out *SecretFileSource, s conversion.Scope) error {
	return autoConvert_v1beta2_SecretFileSource_To_v1alpha3_SecretFileSource(in, out, s)
}

func autoConvert_v1alpha3_User_To_v1beta2_User(in *User, out *v1beta2.User, s conversion.Scope) error {
	out.Name = in.Name
	out.Gecos = (*string)(unsafe.Pointer(in.Gecos))
	out.Groups = (*string)(unsafe.Pointer(in.Groups))
	out.HomeDir = (*string)(unsafe.Pointer(in.HomeDir))
	out.Inactive = (*bool)(unsafe.Pointer(in.Inactive))
	out.Shell = (*string)(unsafe.Pointer(in.Shell))
	out.Passwd = (*string)(unsafe.Pointer(in.Passwd))
	out.PrimaryGroup = (*string)(unsafe.Pointer(in.PrimaryGroup))
	out.LockPassword = (*bool)(unsafe.Pointer(in.LockPassword))
	out.Sudo = (*string)(unsafe.Pointer(in.Sudo))
	out.SSHAuthorizedKeys = *(*[]string)(unsafe.Pointer(&in.SSHAuthorizedKeys))
	return nil
}

// Convert_v1alpha3_User_To_v1beta2_User is an autogenerated conversion function.
func Convert_v1alpha3_User_To_v1beta2_User(in *User, out *v1beta2.User, s conversion.Scope) error {
	return autoConvert_v1alpha3_User_To_v1beta2_User(in, out, s)
}

func autoConvert_v1beta2_User_To_v1alpha3_User(in *v1beta2.User, out *User, s conversion.Scope) error {
	out.Name = in.Name
	out.Gecos = (*string)(unsafe.Pointer(in.Gecos))
	out.Groups = (*string)(unsafe.Pointer(in.Groups))
	out.HomeDir = (*string)(unsafe.Pointer(in.HomeDir))
	out.Inactive = (*bool)(unsafe.Pointer(in.Inactive))
	out.Shell = (*string)(unsafe.Pointer(in.Shell))
	out.Passwd = (*string)(unsafe.Pointer(in.Passwd))
	// WARNING: in.PasswdFrom requires manual conversion: does not exist in peer-type
	out.PrimaryGroup = (*string)(unsafe.Pointer(in.PrimaryGroup))
	out.LockPassword = (*bool)(unsafe.Pointer(in.LockPassword))
	out.Sudo = (*string)(unsafe.Pointer(in.Sudo))
	out.SSHAuthorizedKeys = *(*[]string)(unsafe.Pointer(&in.SSHAuthorizedKeys))
	return nil
}
