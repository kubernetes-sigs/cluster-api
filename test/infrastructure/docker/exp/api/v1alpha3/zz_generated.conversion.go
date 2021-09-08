// +build !ignore_autogenerated_capd_v1alpha3

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

	conversion "k8s.io/apimachinery/pkg/conversion"
	runtime "k8s.io/apimachinery/pkg/runtime"
	apiv1alpha3 "sigs.k8s.io/cluster-api/api/v1alpha3"
	apiv1alpha4 "sigs.k8s.io/cluster-api/api/v1alpha4"
	dockerapiv1alpha3 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1alpha3"
	dockerapiv1alpha4 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1alpha4"
	v1alpha4 "sigs.k8s.io/cluster-api/test/infrastructure/docker/exp/api/v1alpha4"
)

func init() {
	localSchemeBuilder.Register(RegisterConversions)
}

// RegisterConversions adds conversion functions to the given scheme.
// Public to allow building arbitrary schemes.
func RegisterConversions(s *runtime.Scheme) error {
	if err := s.AddGeneratedConversionFunc((*DockerMachinePool)(nil), (*v1alpha4.DockerMachinePool)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha3_DockerMachinePool_To_v1alpha4_DockerMachinePool(a.(*DockerMachinePool), b.(*v1alpha4.DockerMachinePool), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*v1alpha4.DockerMachinePool)(nil), (*DockerMachinePool)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha4_DockerMachinePool_To_v1alpha3_DockerMachinePool(a.(*v1alpha4.DockerMachinePool), b.(*DockerMachinePool), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*DockerMachinePoolInstanceStatus)(nil), (*v1alpha4.DockerMachinePoolInstanceStatus)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha3_DockerMachinePoolInstanceStatus_To_v1alpha4_DockerMachinePoolInstanceStatus(a.(*DockerMachinePoolInstanceStatus), b.(*v1alpha4.DockerMachinePoolInstanceStatus), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*v1alpha4.DockerMachinePoolInstanceStatus)(nil), (*DockerMachinePoolInstanceStatus)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha4_DockerMachinePoolInstanceStatus_To_v1alpha3_DockerMachinePoolInstanceStatus(a.(*v1alpha4.DockerMachinePoolInstanceStatus), b.(*DockerMachinePoolInstanceStatus), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*DockerMachinePoolList)(nil), (*v1alpha4.DockerMachinePoolList)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha3_DockerMachinePoolList_To_v1alpha4_DockerMachinePoolList(a.(*DockerMachinePoolList), b.(*v1alpha4.DockerMachinePoolList), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*v1alpha4.DockerMachinePoolList)(nil), (*DockerMachinePoolList)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha4_DockerMachinePoolList_To_v1alpha3_DockerMachinePoolList(a.(*v1alpha4.DockerMachinePoolList), b.(*DockerMachinePoolList), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*DockerMachinePoolSpec)(nil), (*v1alpha4.DockerMachinePoolSpec)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha3_DockerMachinePoolSpec_To_v1alpha4_DockerMachinePoolSpec(a.(*DockerMachinePoolSpec), b.(*v1alpha4.DockerMachinePoolSpec), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*v1alpha4.DockerMachinePoolSpec)(nil), (*DockerMachinePoolSpec)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha4_DockerMachinePoolSpec_To_v1alpha3_DockerMachinePoolSpec(a.(*v1alpha4.DockerMachinePoolSpec), b.(*DockerMachinePoolSpec), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*DockerMachinePoolStatus)(nil), (*v1alpha4.DockerMachinePoolStatus)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha3_DockerMachinePoolStatus_To_v1alpha4_DockerMachinePoolStatus(a.(*DockerMachinePoolStatus), b.(*v1alpha4.DockerMachinePoolStatus), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*v1alpha4.DockerMachinePoolStatus)(nil), (*DockerMachinePoolStatus)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha4_DockerMachinePoolStatus_To_v1alpha3_DockerMachinePoolStatus(a.(*v1alpha4.DockerMachinePoolStatus), b.(*DockerMachinePoolStatus), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*DockerMachinePoolTemplate)(nil), (*v1alpha4.DockerMachinePoolTemplate)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha3_DockerMachinePoolTemplate_To_v1alpha4_DockerMachinePoolTemplate(a.(*DockerMachinePoolTemplate), b.(*v1alpha4.DockerMachinePoolTemplate), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*v1alpha4.DockerMachinePoolTemplate)(nil), (*DockerMachinePoolTemplate)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha4_DockerMachinePoolTemplate_To_v1alpha3_DockerMachinePoolTemplate(a.(*v1alpha4.DockerMachinePoolTemplate), b.(*DockerMachinePoolTemplate), scope)
	}); err != nil {
		return err
	}
	return nil
}

func autoConvert_v1alpha3_DockerMachinePool_To_v1alpha4_DockerMachinePool(in *DockerMachinePool, out *v1alpha4.DockerMachinePool, s conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	if err := Convert_v1alpha3_DockerMachinePoolSpec_To_v1alpha4_DockerMachinePoolSpec(&in.Spec, &out.Spec, s); err != nil {
		return err
	}
	if err := Convert_v1alpha3_DockerMachinePoolStatus_To_v1alpha4_DockerMachinePoolStatus(&in.Status, &out.Status, s); err != nil {
		return err
	}
	return nil
}

// Convert_v1alpha3_DockerMachinePool_To_v1alpha4_DockerMachinePool is an autogenerated conversion function.
func Convert_v1alpha3_DockerMachinePool_To_v1alpha4_DockerMachinePool(in *DockerMachinePool, out *v1alpha4.DockerMachinePool, s conversion.Scope) error {
	return autoConvert_v1alpha3_DockerMachinePool_To_v1alpha4_DockerMachinePool(in, out, s)
}

func autoConvert_v1alpha4_DockerMachinePool_To_v1alpha3_DockerMachinePool(in *v1alpha4.DockerMachinePool, out *DockerMachinePool, s conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	if err := Convert_v1alpha4_DockerMachinePoolSpec_To_v1alpha3_DockerMachinePoolSpec(&in.Spec, &out.Spec, s); err != nil {
		return err
	}
	if err := Convert_v1alpha4_DockerMachinePoolStatus_To_v1alpha3_DockerMachinePoolStatus(&in.Status, &out.Status, s); err != nil {
		return err
	}
	return nil
}

// Convert_v1alpha4_DockerMachinePool_To_v1alpha3_DockerMachinePool is an autogenerated conversion function.
func Convert_v1alpha4_DockerMachinePool_To_v1alpha3_DockerMachinePool(in *v1alpha4.DockerMachinePool, out *DockerMachinePool, s conversion.Scope) error {
	return autoConvert_v1alpha4_DockerMachinePool_To_v1alpha3_DockerMachinePool(in, out, s)
}

func autoConvert_v1alpha3_DockerMachinePoolInstanceStatus_To_v1alpha4_DockerMachinePoolInstanceStatus(in *DockerMachinePoolInstanceStatus, out *v1alpha4.DockerMachinePoolInstanceStatus, s conversion.Scope) error {
	if in.Addresses != nil {
		in, out := &in.Addresses, &out.Addresses
		*out = make([]apiv1alpha4.MachineAddress, len(*in))
		for i := range *in {
			if err := apiv1alpha3.Convert_v1alpha3_MachineAddress_To_v1alpha4_MachineAddress(&(*in)[i], &(*out)[i], s); err != nil {
				return err
			}
		}
	} else {
		out.Addresses = nil
	}
	out.InstanceName = in.InstanceName
	out.ProviderID = (*string)(unsafe.Pointer(in.ProviderID))
	out.Version = (*string)(unsafe.Pointer(in.Version))
	out.Ready = in.Ready
	out.Bootstrapped = in.Bootstrapped
	return nil
}

// Convert_v1alpha3_DockerMachinePoolInstanceStatus_To_v1alpha4_DockerMachinePoolInstanceStatus is an autogenerated conversion function.
func Convert_v1alpha3_DockerMachinePoolInstanceStatus_To_v1alpha4_DockerMachinePoolInstanceStatus(in *DockerMachinePoolInstanceStatus, out *v1alpha4.DockerMachinePoolInstanceStatus, s conversion.Scope) error {
	return autoConvert_v1alpha3_DockerMachinePoolInstanceStatus_To_v1alpha4_DockerMachinePoolInstanceStatus(in, out, s)
}

func autoConvert_v1alpha4_DockerMachinePoolInstanceStatus_To_v1alpha3_DockerMachinePoolInstanceStatus(in *v1alpha4.DockerMachinePoolInstanceStatus, out *DockerMachinePoolInstanceStatus, s conversion.Scope) error {
	if in.Addresses != nil {
		in, out := &in.Addresses, &out.Addresses
		*out = make([]apiv1alpha3.MachineAddress, len(*in))
		for i := range *in {
			if err := apiv1alpha3.Convert_v1alpha4_MachineAddress_To_v1alpha3_MachineAddress(&(*in)[i], &(*out)[i], s); err != nil {
				return err
			}
		}
	} else {
		out.Addresses = nil
	}
	out.InstanceName = in.InstanceName
	out.ProviderID = (*string)(unsafe.Pointer(in.ProviderID))
	out.Version = (*string)(unsafe.Pointer(in.Version))
	out.Ready = in.Ready
	out.Bootstrapped = in.Bootstrapped
	return nil
}

// Convert_v1alpha4_DockerMachinePoolInstanceStatus_To_v1alpha3_DockerMachinePoolInstanceStatus is an autogenerated conversion function.
func Convert_v1alpha4_DockerMachinePoolInstanceStatus_To_v1alpha3_DockerMachinePoolInstanceStatus(in *v1alpha4.DockerMachinePoolInstanceStatus, out *DockerMachinePoolInstanceStatus, s conversion.Scope) error {
	return autoConvert_v1alpha4_DockerMachinePoolInstanceStatus_To_v1alpha3_DockerMachinePoolInstanceStatus(in, out, s)
}

func autoConvert_v1alpha3_DockerMachinePoolList_To_v1alpha4_DockerMachinePoolList(in *DockerMachinePoolList, out *v1alpha4.DockerMachinePoolList, s conversion.Scope) error {
	out.ListMeta = in.ListMeta
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]v1alpha4.DockerMachinePool, len(*in))
		for i := range *in {
			if err := Convert_v1alpha3_DockerMachinePool_To_v1alpha4_DockerMachinePool(&(*in)[i], &(*out)[i], s); err != nil {
				return err
			}
		}
	} else {
		out.Items = nil
	}
	return nil
}

// Convert_v1alpha3_DockerMachinePoolList_To_v1alpha4_DockerMachinePoolList is an autogenerated conversion function.
func Convert_v1alpha3_DockerMachinePoolList_To_v1alpha4_DockerMachinePoolList(in *DockerMachinePoolList, out *v1alpha4.DockerMachinePoolList, s conversion.Scope) error {
	return autoConvert_v1alpha3_DockerMachinePoolList_To_v1alpha4_DockerMachinePoolList(in, out, s)
}

func autoConvert_v1alpha4_DockerMachinePoolList_To_v1alpha3_DockerMachinePoolList(in *v1alpha4.DockerMachinePoolList, out *DockerMachinePoolList, s conversion.Scope) error {
	out.ListMeta = in.ListMeta
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]DockerMachinePool, len(*in))
		for i := range *in {
			if err := Convert_v1alpha4_DockerMachinePool_To_v1alpha3_DockerMachinePool(&(*in)[i], &(*out)[i], s); err != nil {
				return err
			}
		}
	} else {
		out.Items = nil
	}
	return nil
}

// Convert_v1alpha4_DockerMachinePoolList_To_v1alpha3_DockerMachinePoolList is an autogenerated conversion function.
func Convert_v1alpha4_DockerMachinePoolList_To_v1alpha3_DockerMachinePoolList(in *v1alpha4.DockerMachinePoolList, out *DockerMachinePoolList, s conversion.Scope) error {
	return autoConvert_v1alpha4_DockerMachinePoolList_To_v1alpha3_DockerMachinePoolList(in, out, s)
}

func autoConvert_v1alpha3_DockerMachinePoolSpec_To_v1alpha4_DockerMachinePoolSpec(in *DockerMachinePoolSpec, out *v1alpha4.DockerMachinePoolSpec, s conversion.Scope) error {
	if err := Convert_v1alpha3_DockerMachinePoolTemplate_To_v1alpha4_DockerMachinePoolTemplate(&in.Template, &out.Template, s); err != nil {
		return err
	}
	out.ProviderID = in.ProviderID
	out.ProviderIDList = *(*[]string)(unsafe.Pointer(&in.ProviderIDList))
	return nil
}

// Convert_v1alpha3_DockerMachinePoolSpec_To_v1alpha4_DockerMachinePoolSpec is an autogenerated conversion function.
func Convert_v1alpha3_DockerMachinePoolSpec_To_v1alpha4_DockerMachinePoolSpec(in *DockerMachinePoolSpec, out *v1alpha4.DockerMachinePoolSpec, s conversion.Scope) error {
	return autoConvert_v1alpha3_DockerMachinePoolSpec_To_v1alpha4_DockerMachinePoolSpec(in, out, s)
}

func autoConvert_v1alpha4_DockerMachinePoolSpec_To_v1alpha3_DockerMachinePoolSpec(in *v1alpha4.DockerMachinePoolSpec, out *DockerMachinePoolSpec, s conversion.Scope) error {
	if err := Convert_v1alpha4_DockerMachinePoolTemplate_To_v1alpha3_DockerMachinePoolTemplate(&in.Template, &out.Template, s); err != nil {
		return err
	}
	out.ProviderID = in.ProviderID
	out.ProviderIDList = *(*[]string)(unsafe.Pointer(&in.ProviderIDList))
	return nil
}

// Convert_v1alpha4_DockerMachinePoolSpec_To_v1alpha3_DockerMachinePoolSpec is an autogenerated conversion function.
func Convert_v1alpha4_DockerMachinePoolSpec_To_v1alpha3_DockerMachinePoolSpec(in *v1alpha4.DockerMachinePoolSpec, out *DockerMachinePoolSpec, s conversion.Scope) error {
	return autoConvert_v1alpha4_DockerMachinePoolSpec_To_v1alpha3_DockerMachinePoolSpec(in, out, s)
}

func autoConvert_v1alpha3_DockerMachinePoolStatus_To_v1alpha4_DockerMachinePoolStatus(in *DockerMachinePoolStatus, out *v1alpha4.DockerMachinePoolStatus, s conversion.Scope) error {
	out.Ready = in.Ready
	out.Replicas = in.Replicas
	out.ObservedGeneration = in.ObservedGeneration
	if in.Instances != nil {
		in, out := &in.Instances, &out.Instances
		*out = make([]v1alpha4.DockerMachinePoolInstanceStatus, len(*in))
		for i := range *in {
			if err := Convert_v1alpha3_DockerMachinePoolInstanceStatus_To_v1alpha4_DockerMachinePoolInstanceStatus(&(*in)[i], &(*out)[i], s); err != nil {
				return err
			}
		}
	} else {
		out.Instances = nil
	}
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make(apiv1alpha4.Conditions, len(*in))
		for i := range *in {
			if err := apiv1alpha3.Convert_v1alpha3_Condition_To_v1alpha4_Condition(&(*in)[i], &(*out)[i], s); err != nil {
				return err
			}
		}
	} else {
		out.Conditions = nil
	}
	return nil
}

// Convert_v1alpha3_DockerMachinePoolStatus_To_v1alpha4_DockerMachinePoolStatus is an autogenerated conversion function.
func Convert_v1alpha3_DockerMachinePoolStatus_To_v1alpha4_DockerMachinePoolStatus(in *DockerMachinePoolStatus, out *v1alpha4.DockerMachinePoolStatus, s conversion.Scope) error {
	return autoConvert_v1alpha3_DockerMachinePoolStatus_To_v1alpha4_DockerMachinePoolStatus(in, out, s)
}

func autoConvert_v1alpha4_DockerMachinePoolStatus_To_v1alpha3_DockerMachinePoolStatus(in *v1alpha4.DockerMachinePoolStatus, out *DockerMachinePoolStatus, s conversion.Scope) error {
	out.Ready = in.Ready
	out.Replicas = in.Replicas
	out.ObservedGeneration = in.ObservedGeneration
	if in.Instances != nil {
		in, out := &in.Instances, &out.Instances
		*out = make([]DockerMachinePoolInstanceStatus, len(*in))
		for i := range *in {
			if err := Convert_v1alpha4_DockerMachinePoolInstanceStatus_To_v1alpha3_DockerMachinePoolInstanceStatus(&(*in)[i], &(*out)[i], s); err != nil {
				return err
			}
		}
	} else {
		out.Instances = nil
	}
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make(apiv1alpha3.Conditions, len(*in))
		for i := range *in {
			if err := apiv1alpha3.Convert_v1alpha4_Condition_To_v1alpha3_Condition(&(*in)[i], &(*out)[i], s); err != nil {
				return err
			}
		}
	} else {
		out.Conditions = nil
	}
	return nil
}

// Convert_v1alpha4_DockerMachinePoolStatus_To_v1alpha3_DockerMachinePoolStatus is an autogenerated conversion function.
func Convert_v1alpha4_DockerMachinePoolStatus_To_v1alpha3_DockerMachinePoolStatus(in *v1alpha4.DockerMachinePoolStatus, out *DockerMachinePoolStatus, s conversion.Scope) error {
	return autoConvert_v1alpha4_DockerMachinePoolStatus_To_v1alpha3_DockerMachinePoolStatus(in, out, s)
}

func autoConvert_v1alpha3_DockerMachinePoolTemplate_To_v1alpha4_DockerMachinePoolTemplate(in *DockerMachinePoolTemplate, out *v1alpha4.DockerMachinePoolTemplate, s conversion.Scope) error {
	out.CustomImage = in.CustomImage
	out.PreLoadImages = *(*[]string)(unsafe.Pointer(&in.PreLoadImages))
	out.ExtraMounts = *(*[]dockerapiv1alpha4.Mount)(unsafe.Pointer(&in.ExtraMounts))
	return nil
}

// Convert_v1alpha3_DockerMachinePoolTemplate_To_v1alpha4_DockerMachinePoolTemplate is an autogenerated conversion function.
func Convert_v1alpha3_DockerMachinePoolTemplate_To_v1alpha4_DockerMachinePoolTemplate(in *DockerMachinePoolTemplate, out *v1alpha4.DockerMachinePoolTemplate, s conversion.Scope) error {
	return autoConvert_v1alpha3_DockerMachinePoolTemplate_To_v1alpha4_DockerMachinePoolTemplate(in, out, s)
}

func autoConvert_v1alpha4_DockerMachinePoolTemplate_To_v1alpha3_DockerMachinePoolTemplate(in *v1alpha4.DockerMachinePoolTemplate, out *DockerMachinePoolTemplate, s conversion.Scope) error {
	out.CustomImage = in.CustomImage
	out.PreLoadImages = *(*[]string)(unsafe.Pointer(&in.PreLoadImages))
	out.ExtraMounts = *(*[]dockerapiv1alpha3.Mount)(unsafe.Pointer(&in.ExtraMounts))
	return nil
}

// Convert_v1alpha4_DockerMachinePoolTemplate_To_v1alpha3_DockerMachinePoolTemplate is an autogenerated conversion function.
func Convert_v1alpha4_DockerMachinePoolTemplate_To_v1alpha3_DockerMachinePoolTemplate(in *v1alpha4.DockerMachinePoolTemplate, out *DockerMachinePoolTemplate, s conversion.Scope) error {
	return autoConvert_v1alpha4_DockerMachinePoolTemplate_To_v1alpha3_DockerMachinePoolTemplate(in, out, s)
}
