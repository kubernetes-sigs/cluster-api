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

	v1 "k8s.io/api/core/v1"
	conversion "k8s.io/apimachinery/pkg/conversion"
	runtime "k8s.io/apimachinery/pkg/runtime"
	apiv1alpha3 "sigs.k8s.io/cluster-api/api/v1alpha3"
	apiv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	errors "sigs.k8s.io/cluster-api/errors"
	v1beta1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
)

func init() {
	localSchemeBuilder.Register(RegisterConversions)
}

// RegisterConversions adds conversion functions to the given scheme.
// Public to allow building arbitrary schemes.
func RegisterConversions(s *runtime.Scheme) error {
	if err := s.AddGeneratedConversionFunc((*MachinePoolList)(nil), (*v1beta1.MachinePoolList)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha3_MachinePoolList_To_v1beta1_MachinePoolList(a.(*MachinePoolList), b.(*v1beta1.MachinePoolList), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*v1beta1.MachinePoolList)(nil), (*MachinePoolList)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta1_MachinePoolList_To_v1alpha3_MachinePoolList(a.(*v1beta1.MachinePoolList), b.(*MachinePoolList), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*MachinePoolStatus)(nil), (*v1beta1.MachinePoolStatus)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha3_MachinePoolStatus_To_v1beta1_MachinePoolStatus(a.(*MachinePoolStatus), b.(*v1beta1.MachinePoolStatus), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*v1beta1.MachinePoolStatus)(nil), (*MachinePoolStatus)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta1_MachinePoolStatus_To_v1alpha3_MachinePoolStatus(a.(*v1beta1.MachinePoolStatus), b.(*MachinePoolStatus), scope)
	}); err != nil {
		return err
	}
	if err := s.AddConversionFunc((*MachinePoolSpec)(nil), (*v1beta1.MachinePoolSpec)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha3_MachinePoolSpec_To_v1beta1_MachinePoolSpec(a.(*MachinePoolSpec), b.(*v1beta1.MachinePoolSpec), scope)
	}); err != nil {
		return err
	}
	if err := s.AddConversionFunc((*MachinePool)(nil), (*v1beta1.MachinePool)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha3_MachinePool_To_v1beta1_MachinePool(a.(*MachinePool), b.(*v1beta1.MachinePool), scope)
	}); err != nil {
		return err
	}
	if err := s.AddConversionFunc((*v1beta1.MachinePoolSpec)(nil), (*MachinePoolSpec)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta1_MachinePoolSpec_To_v1alpha3_MachinePoolSpec(a.(*v1beta1.MachinePoolSpec), b.(*MachinePoolSpec), scope)
	}); err != nil {
		return err
	}
	if err := s.AddConversionFunc((*v1beta1.MachinePool)(nil), (*MachinePool)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1beta1_MachinePool_To_v1alpha3_MachinePool(a.(*v1beta1.MachinePool), b.(*MachinePool), scope)
	}); err != nil {
		return err
	}
	return nil
}

func autoConvert_v1alpha3_MachinePool_To_v1beta1_MachinePool(in *MachinePool, out *v1beta1.MachinePool, s conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	if err := Convert_v1alpha3_MachinePoolSpec_To_v1beta1_MachinePoolSpec(&in.Spec, &out.Spec, s); err != nil {
		return err
	}
	if err := Convert_v1alpha3_MachinePoolStatus_To_v1beta1_MachinePoolStatus(&in.Status, &out.Status, s); err != nil {
		return err
	}
	return nil
}

func autoConvert_v1beta1_MachinePool_To_v1alpha3_MachinePool(in *v1beta1.MachinePool, out *MachinePool, s conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	if err := Convert_v1beta1_MachinePoolSpec_To_v1alpha3_MachinePoolSpec(&in.Spec, &out.Spec, s); err != nil {
		return err
	}
	if err := Convert_v1beta1_MachinePoolStatus_To_v1alpha3_MachinePoolStatus(&in.Status, &out.Status, s); err != nil {
		return err
	}
	return nil
}

func autoConvert_v1alpha3_MachinePoolList_To_v1beta1_MachinePoolList(in *MachinePoolList, out *v1beta1.MachinePoolList, s conversion.Scope) error {
	out.ListMeta = in.ListMeta
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]v1beta1.MachinePool, len(*in))
		for i := range *in {
			if err := Convert_v1alpha3_MachinePool_To_v1beta1_MachinePool(&(*in)[i], &(*out)[i], s); err != nil {
				return err
			}
		}
	} else {
		out.Items = nil
	}
	return nil
}

// Convert_v1alpha3_MachinePoolList_To_v1beta1_MachinePoolList is an autogenerated conversion function.
func Convert_v1alpha3_MachinePoolList_To_v1beta1_MachinePoolList(in *MachinePoolList, out *v1beta1.MachinePoolList, s conversion.Scope) error {
	return autoConvert_v1alpha3_MachinePoolList_To_v1beta1_MachinePoolList(in, out, s)
}

func autoConvert_v1beta1_MachinePoolList_To_v1alpha3_MachinePoolList(in *v1beta1.MachinePoolList, out *MachinePoolList, s conversion.Scope) error {
	out.ListMeta = in.ListMeta
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]MachinePool, len(*in))
		for i := range *in {
			if err := Convert_v1beta1_MachinePool_To_v1alpha3_MachinePool(&(*in)[i], &(*out)[i], s); err != nil {
				return err
			}
		}
	} else {
		out.Items = nil
	}
	return nil
}

// Convert_v1beta1_MachinePoolList_To_v1alpha3_MachinePoolList is an autogenerated conversion function.
func Convert_v1beta1_MachinePoolList_To_v1alpha3_MachinePoolList(in *v1beta1.MachinePoolList, out *MachinePoolList, s conversion.Scope) error {
	return autoConvert_v1beta1_MachinePoolList_To_v1alpha3_MachinePoolList(in, out, s)
}

func autoConvert_v1alpha3_MachinePoolSpec_To_v1beta1_MachinePoolSpec(in *MachinePoolSpec, out *v1beta1.MachinePoolSpec, s conversion.Scope) error {
	out.ClusterName = in.ClusterName
	out.Replicas = (*int32)(unsafe.Pointer(in.Replicas))
	if err := apiv1alpha3.Convert_v1alpha3_MachineTemplateSpec_To_v1beta1_MachineTemplateSpec(&in.Template, &out.Template, s); err != nil {
		return err
	}
	// WARNING: in.Strategy requires manual conversion: does not exist in peer-type
	out.MinReadySeconds = (*int32)(unsafe.Pointer(in.MinReadySeconds))
	out.ProviderIDList = *(*[]string)(unsafe.Pointer(&in.ProviderIDList))
	out.FailureDomains = *(*[]string)(unsafe.Pointer(&in.FailureDomains))
	return nil
}

func autoConvert_v1beta1_MachinePoolSpec_To_v1alpha3_MachinePoolSpec(in *v1beta1.MachinePoolSpec, out *MachinePoolSpec, s conversion.Scope) error {
	out.ClusterName = in.ClusterName
	out.Replicas = (*int32)(unsafe.Pointer(in.Replicas))
	if err := apiv1alpha3.Convert_v1beta1_MachineTemplateSpec_To_v1alpha3_MachineTemplateSpec(&in.Template, &out.Template, s); err != nil {
		return err
	}
	out.MinReadySeconds = (*int32)(unsafe.Pointer(in.MinReadySeconds))
	// WARNING: in.InfrastructureRefList requires manual conversion: does not exist in peer-type
	out.ProviderIDList = *(*[]string)(unsafe.Pointer(&in.ProviderIDList))
	out.FailureDomains = *(*[]string)(unsafe.Pointer(&in.FailureDomains))
	return nil
}

func autoConvert_v1alpha3_MachinePoolStatus_To_v1beta1_MachinePoolStatus(in *MachinePoolStatus, out *v1beta1.MachinePoolStatus, s conversion.Scope) error {
	out.NodeRefs = *(*[]v1.ObjectReference)(unsafe.Pointer(&in.NodeRefs))
	out.Replicas = in.Replicas
	out.ReadyReplicas = in.ReadyReplicas
	out.AvailableReplicas = in.AvailableReplicas
	out.UnavailableReplicas = in.UnavailableReplicas
	out.FailureReason = (*errors.MachinePoolStatusFailure)(unsafe.Pointer(in.FailureReason))
	out.FailureMessage = (*string)(unsafe.Pointer(in.FailureMessage))
	out.Phase = in.Phase
	out.BootstrapReady = in.BootstrapReady
	out.InfrastructureReady = in.InfrastructureReady
	out.ObservedGeneration = in.ObservedGeneration
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make(apiv1beta1.Conditions, len(*in))
		for i := range *in {
			if err := apiv1alpha3.Convert_v1alpha3_Condition_To_v1beta1_Condition(&(*in)[i], &(*out)[i], s); err != nil {
				return err
			}
		}
	} else {
		out.Conditions = nil
	}
	return nil
}

// Convert_v1alpha3_MachinePoolStatus_To_v1beta1_MachinePoolStatus is an autogenerated conversion function.
func Convert_v1alpha3_MachinePoolStatus_To_v1beta1_MachinePoolStatus(in *MachinePoolStatus, out *v1beta1.MachinePoolStatus, s conversion.Scope) error {
	return autoConvert_v1alpha3_MachinePoolStatus_To_v1beta1_MachinePoolStatus(in, out, s)
}

func autoConvert_v1beta1_MachinePoolStatus_To_v1alpha3_MachinePoolStatus(in *v1beta1.MachinePoolStatus, out *MachinePoolStatus, s conversion.Scope) error {
	out.NodeRefs = *(*[]v1.ObjectReference)(unsafe.Pointer(&in.NodeRefs))
	out.Replicas = in.Replicas
	out.ReadyReplicas = in.ReadyReplicas
	out.AvailableReplicas = in.AvailableReplicas
	out.UnavailableReplicas = in.UnavailableReplicas
	out.FailureReason = (*errors.MachinePoolStatusFailure)(unsafe.Pointer(in.FailureReason))
	out.FailureMessage = (*string)(unsafe.Pointer(in.FailureMessage))
	out.Phase = in.Phase
	out.BootstrapReady = in.BootstrapReady
	out.InfrastructureReady = in.InfrastructureReady
	out.ObservedGeneration = in.ObservedGeneration
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make(apiv1alpha3.Conditions, len(*in))
		for i := range *in {
			if err := apiv1alpha3.Convert_v1beta1_Condition_To_v1alpha3_Condition(&(*in)[i], &(*out)[i], s); err != nil {
				return err
			}
		}
	} else {
		out.Conditions = nil
	}
	return nil
}

// Convert_v1beta1_MachinePoolStatus_To_v1alpha3_MachinePoolStatus is an autogenerated conversion function.
func Convert_v1beta1_MachinePoolStatus_To_v1alpha3_MachinePoolStatus(in *v1beta1.MachinePoolStatus, out *MachinePoolStatus, s conversion.Scope) error {
	return autoConvert_v1beta1_MachinePoolStatus_To_v1alpha3_MachinePoolStatus(in, out, s)
}
