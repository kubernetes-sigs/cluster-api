//go:build !ignore_autogenerated_capd
// +build !ignore_autogenerated_capd

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

package v1beta1

import (
	unsafe "unsafe"

	conversion "k8s.io/apimachinery/pkg/conversion"
	runtime "k8s.io/apimachinery/pkg/runtime"
	apiv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	dockerapiv1beta1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1"
	apiv1beta2 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta2"
	v1beta2 "sigs.k8s.io/cluster-api/test/infrastructure/docker/exp/api/v1beta2"
)

func init() {
	localSchemeBuilder.Register(RegisterConversions)
}

// RegisterConversions adds conversion functions to the given scheme.
// Public to allow building arbitrary schemes.
func RegisterConversions(s *runtime.Scheme) error {
	if err := s.AddGeneratedConversionFunc((*DockerMachinePool)(nil), (*v1beta2.DockerMachinePool)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta1_DockerMachinePool_To_v1beta2_DockerMachinePool(a.(*DockerMachinePool), b.(*v1beta2.DockerMachinePool), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*v1beta2.DockerMachinePool)(nil), (*DockerMachinePool)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta2_DockerMachinePool_To_v1beta1_DockerMachinePool(a.(*v1beta2.DockerMachinePool), b.(*DockerMachinePool), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*DockerMachinePoolInstanceStatus)(nil), (*v1beta2.DockerMachinePoolInstanceStatus)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta1_DockerMachinePoolInstanceStatus_To_v1beta2_DockerMachinePoolInstanceStatus(a.(*DockerMachinePoolInstanceStatus), b.(*v1beta2.DockerMachinePoolInstanceStatus), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*v1beta2.DockerMachinePoolInstanceStatus)(nil), (*DockerMachinePoolInstanceStatus)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta2_DockerMachinePoolInstanceStatus_To_v1beta1_DockerMachinePoolInstanceStatus(a.(*v1beta2.DockerMachinePoolInstanceStatus), b.(*DockerMachinePoolInstanceStatus), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*DockerMachinePoolList)(nil), (*v1beta2.DockerMachinePoolList)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta1_DockerMachinePoolList_To_v1beta2_DockerMachinePoolList(a.(*DockerMachinePoolList), b.(*v1beta2.DockerMachinePoolList), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*v1beta2.DockerMachinePoolList)(nil), (*DockerMachinePoolList)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta2_DockerMachinePoolList_To_v1beta1_DockerMachinePoolList(a.(*v1beta2.DockerMachinePoolList), b.(*DockerMachinePoolList), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*DockerMachinePoolMachineTemplate)(nil), (*v1beta2.DockerMachinePoolMachineTemplate)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta1_DockerMachinePoolMachineTemplate_To_v1beta2_DockerMachinePoolMachineTemplate(a.(*DockerMachinePoolMachineTemplate), b.(*v1beta2.DockerMachinePoolMachineTemplate), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*v1beta2.DockerMachinePoolMachineTemplate)(nil), (*DockerMachinePoolMachineTemplate)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta2_DockerMachinePoolMachineTemplate_To_v1beta1_DockerMachinePoolMachineTemplate(a.(*v1beta2.DockerMachinePoolMachineTemplate), b.(*DockerMachinePoolMachineTemplate), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*DockerMachinePoolSpec)(nil), (*v1beta2.DockerMachinePoolSpec)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta1_DockerMachinePoolSpec_To_v1beta2_DockerMachinePoolSpec(a.(*DockerMachinePoolSpec), b.(*v1beta2.DockerMachinePoolSpec), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*v1beta2.DockerMachinePoolSpec)(nil), (*DockerMachinePoolSpec)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta2_DockerMachinePoolSpec_To_v1beta1_DockerMachinePoolSpec(a.(*v1beta2.DockerMachinePoolSpec), b.(*DockerMachinePoolSpec), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*DockerMachinePoolStatus)(nil), (*v1beta2.DockerMachinePoolStatus)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta1_DockerMachinePoolStatus_To_v1beta2_DockerMachinePoolStatus(a.(*DockerMachinePoolStatus), b.(*v1beta2.DockerMachinePoolStatus), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*v1beta2.DockerMachinePoolStatus)(nil), (*DockerMachinePoolStatus)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta2_DockerMachinePoolStatus_To_v1beta1_DockerMachinePoolStatus(a.(*v1beta2.DockerMachinePoolStatus), b.(*DockerMachinePoolStatus), scope)
	}); err != nil {
		return err
	}
	return nil
}

func autoConvert_v1beta1_DockerMachinePool_To_v1beta2_DockerMachinePool(in *DockerMachinePool, out *v1beta2.DockerMachinePool, s conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	if err := Convert_v1beta1_DockerMachinePoolSpec_To_v1beta2_DockerMachinePoolSpec(&in.Spec, &out.Spec, s); err != nil {
		return err
	}
	if err := Convert_v1beta1_DockerMachinePoolStatus_To_v1beta2_DockerMachinePoolStatus(&in.Status, &out.Status, s); err != nil {
		return err
	}
	return nil
}

// Convert_v1beta1_DockerMachinePool_To_v1beta2_DockerMachinePool is an autogenerated conversion function.
func Convert_v1beta1_DockerMachinePool_To_v1beta2_DockerMachinePool(in *DockerMachinePool, out *v1beta2.DockerMachinePool, s conversion.Scope) error {
	return autoConvert_v1beta1_DockerMachinePool_To_v1beta2_DockerMachinePool(in, out, s)
}

func autoConvert_v1beta2_DockerMachinePool_To_v1beta1_DockerMachinePool(in *v1beta2.DockerMachinePool, out *DockerMachinePool, s conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	if err := Convert_v1beta2_DockerMachinePoolSpec_To_v1beta1_DockerMachinePoolSpec(&in.Spec, &out.Spec, s); err != nil {
		return err
	}
	if err := Convert_v1beta2_DockerMachinePoolStatus_To_v1beta1_DockerMachinePoolStatus(&in.Status, &out.Status, s); err != nil {
		return err
	}
	return nil
}

// Convert_v1beta2_DockerMachinePool_To_v1beta1_DockerMachinePool is an autogenerated conversion function.
func Convert_v1beta2_DockerMachinePool_To_v1beta1_DockerMachinePool(in *v1beta2.DockerMachinePool, out *DockerMachinePool, s conversion.Scope) error {
	return autoConvert_v1beta2_DockerMachinePool_To_v1beta1_DockerMachinePool(in, out, s)
}

func autoConvert_v1beta1_DockerMachinePoolInstanceStatus_To_v1beta2_DockerMachinePoolInstanceStatus(in *DockerMachinePoolInstanceStatus, out *v1beta2.DockerMachinePoolInstanceStatus, s conversion.Scope) error {
	out.Addresses = *(*[]apiv1beta1.MachineAddress)(unsafe.Pointer(&in.Addresses))
	out.InstanceName = in.InstanceName
	out.ProviderID = (*string)(unsafe.Pointer(in.ProviderID))
	out.Version = (*string)(unsafe.Pointer(in.Version))
	out.Ready = in.Ready
	out.Bootstrapped = in.Bootstrapped
	return nil
}

// Convert_v1beta1_DockerMachinePoolInstanceStatus_To_v1beta2_DockerMachinePoolInstanceStatus is an autogenerated conversion function.
func Convert_v1beta1_DockerMachinePoolInstanceStatus_To_v1beta2_DockerMachinePoolInstanceStatus(in *DockerMachinePoolInstanceStatus, out *v1beta2.DockerMachinePoolInstanceStatus, s conversion.Scope) error {
	return autoConvert_v1beta1_DockerMachinePoolInstanceStatus_To_v1beta2_DockerMachinePoolInstanceStatus(in, out, s)
}

func autoConvert_v1beta2_DockerMachinePoolInstanceStatus_To_v1beta1_DockerMachinePoolInstanceStatus(in *v1beta2.DockerMachinePoolInstanceStatus, out *DockerMachinePoolInstanceStatus, s conversion.Scope) error {
	out.Addresses = *(*[]apiv1beta1.MachineAddress)(unsafe.Pointer(&in.Addresses))
	out.InstanceName = in.InstanceName
	out.ProviderID = (*string)(unsafe.Pointer(in.ProviderID))
	out.Version = (*string)(unsafe.Pointer(in.Version))
	out.Ready = in.Ready
	out.Bootstrapped = in.Bootstrapped
	return nil
}

// Convert_v1beta2_DockerMachinePoolInstanceStatus_To_v1beta1_DockerMachinePoolInstanceStatus is an autogenerated conversion function.
func Convert_v1beta2_DockerMachinePoolInstanceStatus_To_v1beta1_DockerMachinePoolInstanceStatus(in *v1beta2.DockerMachinePoolInstanceStatus, out *DockerMachinePoolInstanceStatus, s conversion.Scope) error {
	return autoConvert_v1beta2_DockerMachinePoolInstanceStatus_To_v1beta1_DockerMachinePoolInstanceStatus(in, out, s)
}

func autoConvert_v1beta1_DockerMachinePoolList_To_v1beta2_DockerMachinePoolList(in *DockerMachinePoolList, out *v1beta2.DockerMachinePoolList, s conversion.Scope) error {
	out.ListMeta = in.ListMeta
	out.Items = *(*[]v1beta2.DockerMachinePool)(unsafe.Pointer(&in.Items))
	return nil
}

// Convert_v1beta1_DockerMachinePoolList_To_v1beta2_DockerMachinePoolList is an autogenerated conversion function.
func Convert_v1beta1_DockerMachinePoolList_To_v1beta2_DockerMachinePoolList(in *DockerMachinePoolList, out *v1beta2.DockerMachinePoolList, s conversion.Scope) error {
	return autoConvert_v1beta1_DockerMachinePoolList_To_v1beta2_DockerMachinePoolList(in, out, s)
}

func autoConvert_v1beta2_DockerMachinePoolList_To_v1beta1_DockerMachinePoolList(in *v1beta2.DockerMachinePoolList, out *DockerMachinePoolList, s conversion.Scope) error {
	out.ListMeta = in.ListMeta
	out.Items = *(*[]DockerMachinePool)(unsafe.Pointer(&in.Items))
	return nil
}

// Convert_v1beta2_DockerMachinePoolList_To_v1beta1_DockerMachinePoolList is an autogenerated conversion function.
func Convert_v1beta2_DockerMachinePoolList_To_v1beta1_DockerMachinePoolList(in *v1beta2.DockerMachinePoolList, out *DockerMachinePoolList, s conversion.Scope) error {
	return autoConvert_v1beta2_DockerMachinePoolList_To_v1beta1_DockerMachinePoolList(in, out, s)
}

func autoConvert_v1beta1_DockerMachinePoolMachineTemplate_To_v1beta2_DockerMachinePoolMachineTemplate(in *DockerMachinePoolMachineTemplate, out *v1beta2.DockerMachinePoolMachineTemplate, s conversion.Scope) error {
	out.CustomImage = in.CustomImage
	out.PreLoadImages = *(*[]string)(unsafe.Pointer(&in.PreLoadImages))
	out.ExtraMounts = *(*[]apiv1beta2.Mount)(unsafe.Pointer(&in.ExtraMounts))
	return nil
}

// Convert_v1beta1_DockerMachinePoolMachineTemplate_To_v1beta2_DockerMachinePoolMachineTemplate is an autogenerated conversion function.
func Convert_v1beta1_DockerMachinePoolMachineTemplate_To_v1beta2_DockerMachinePoolMachineTemplate(in *DockerMachinePoolMachineTemplate, out *v1beta2.DockerMachinePoolMachineTemplate, s conversion.Scope) error {
	return autoConvert_v1beta1_DockerMachinePoolMachineTemplate_To_v1beta2_DockerMachinePoolMachineTemplate(in, out, s)
}

func autoConvert_v1beta2_DockerMachinePoolMachineTemplate_To_v1beta1_DockerMachinePoolMachineTemplate(in *v1beta2.DockerMachinePoolMachineTemplate, out *DockerMachinePoolMachineTemplate, s conversion.Scope) error {
	out.CustomImage = in.CustomImage
	out.PreLoadImages = *(*[]string)(unsafe.Pointer(&in.PreLoadImages))
	out.ExtraMounts = *(*[]dockerapiv1beta1.Mount)(unsafe.Pointer(&in.ExtraMounts))
	return nil
}

// Convert_v1beta2_DockerMachinePoolMachineTemplate_To_v1beta1_DockerMachinePoolMachineTemplate is an autogenerated conversion function.
func Convert_v1beta2_DockerMachinePoolMachineTemplate_To_v1beta1_DockerMachinePoolMachineTemplate(in *v1beta2.DockerMachinePoolMachineTemplate, out *DockerMachinePoolMachineTemplate, s conversion.Scope) error {
	return autoConvert_v1beta2_DockerMachinePoolMachineTemplate_To_v1beta1_DockerMachinePoolMachineTemplate(in, out, s)
}

func autoConvert_v1beta1_DockerMachinePoolSpec_To_v1beta2_DockerMachinePoolSpec(in *DockerMachinePoolSpec, out *v1beta2.DockerMachinePoolSpec, s conversion.Scope) error {
	if err := Convert_v1beta1_DockerMachinePoolMachineTemplate_To_v1beta2_DockerMachinePoolMachineTemplate(&in.Template, &out.Template, s); err != nil {
		return err
	}
	out.ProviderID = in.ProviderID
	out.ProviderIDList = *(*[]string)(unsafe.Pointer(&in.ProviderIDList))
	return nil
}

// Convert_v1beta1_DockerMachinePoolSpec_To_v1beta2_DockerMachinePoolSpec is an autogenerated conversion function.
func Convert_v1beta1_DockerMachinePoolSpec_To_v1beta2_DockerMachinePoolSpec(in *DockerMachinePoolSpec, out *v1beta2.DockerMachinePoolSpec, s conversion.Scope) error {
	return autoConvert_v1beta1_DockerMachinePoolSpec_To_v1beta2_DockerMachinePoolSpec(in, out, s)
}

func autoConvert_v1beta2_DockerMachinePoolSpec_To_v1beta1_DockerMachinePoolSpec(in *v1beta2.DockerMachinePoolSpec, out *DockerMachinePoolSpec, s conversion.Scope) error {
	if err := Convert_v1beta2_DockerMachinePoolMachineTemplate_To_v1beta1_DockerMachinePoolMachineTemplate(&in.Template, &out.Template, s); err != nil {
		return err
	}
	out.ProviderID = in.ProviderID
	out.ProviderIDList = *(*[]string)(unsafe.Pointer(&in.ProviderIDList))
	return nil
}

// Convert_v1beta2_DockerMachinePoolSpec_To_v1beta1_DockerMachinePoolSpec is an autogenerated conversion function.
func Convert_v1beta2_DockerMachinePoolSpec_To_v1beta1_DockerMachinePoolSpec(in *v1beta2.DockerMachinePoolSpec, out *DockerMachinePoolSpec, s conversion.Scope) error {
	return autoConvert_v1beta2_DockerMachinePoolSpec_To_v1beta1_DockerMachinePoolSpec(in, out, s)
}

func autoConvert_v1beta1_DockerMachinePoolStatus_To_v1beta2_DockerMachinePoolStatus(in *DockerMachinePoolStatus, out *v1beta2.DockerMachinePoolStatus, s conversion.Scope) error {
	out.Ready = in.Ready
	out.Replicas = in.Replicas
	out.ObservedGeneration = in.ObservedGeneration
	out.Instances = *(*[]v1beta2.DockerMachinePoolInstanceStatus)(unsafe.Pointer(&in.Instances))
	out.Conditions = *(*apiv1beta1.Conditions)(unsafe.Pointer(&in.Conditions))
	return nil
}

// Convert_v1beta1_DockerMachinePoolStatus_To_v1beta2_DockerMachinePoolStatus is an autogenerated conversion function.
func Convert_v1beta1_DockerMachinePoolStatus_To_v1beta2_DockerMachinePoolStatus(in *DockerMachinePoolStatus, out *v1beta2.DockerMachinePoolStatus, s conversion.Scope) error {
	return autoConvert_v1beta1_DockerMachinePoolStatus_To_v1beta2_DockerMachinePoolStatus(in, out, s)
}

func autoConvert_v1beta2_DockerMachinePoolStatus_To_v1beta1_DockerMachinePoolStatus(in *v1beta2.DockerMachinePoolStatus, out *DockerMachinePoolStatus, s conversion.Scope) error {
	out.Ready = in.Ready
	out.Replicas = in.Replicas
	out.ObservedGeneration = in.ObservedGeneration
	out.Instances = *(*[]DockerMachinePoolInstanceStatus)(unsafe.Pointer(&in.Instances))
	out.Conditions = *(*apiv1beta1.Conditions)(unsafe.Pointer(&in.Conditions))
	return nil
}

// Convert_v1beta2_DockerMachinePoolStatus_To_v1beta1_DockerMachinePoolStatus is an autogenerated conversion function.
func Convert_v1beta2_DockerMachinePoolStatus_To_v1beta1_DockerMachinePoolStatus(in *v1beta2.DockerMachinePoolStatus, out *DockerMachinePoolStatus, s conversion.Scope) error {
	return autoConvert_v1beta2_DockerMachinePoolStatus_To_v1beta1_DockerMachinePoolStatus(in, out, s)
}
