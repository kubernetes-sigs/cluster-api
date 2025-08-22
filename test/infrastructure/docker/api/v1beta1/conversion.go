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
	"maps"
	"reflect"
	"slices"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiconversion "k8s.io/apimachinery/pkg/conversion"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta2"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

func (src *DockerCluster) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*infrav1.DockerCluster)

	if err := Convert_v1beta1_DockerCluster_To_v1beta2_DockerCluster(src, dst, nil); err != nil {
		return err
	}

	restored := &infrav1.DockerCluster{}
	ok, err := utilconversion.UnmarshalData(src, restored)
	if err != nil {
		return err
	}

	// Recover intent for bool values converted to *bool.
	initialization := infrav1.DockerClusterInitializationStatus{}
	restoredDockerClusterProvisioned := restored.Status.Initialization.Provisioned
	clusterv1.Convert_bool_To_Pointer_bool(src.Status.Ready, ok, restoredDockerClusterProvisioned, &initialization.Provisioned)
	if !reflect.DeepEqual(initialization, infrav1.DockerClusterInitializationStatus{}) {
		dst.Status.Initialization = initialization
	}

	return nil
}

func (dst *DockerCluster) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*infrav1.DockerCluster)

	if err := Convert_v1beta2_DockerCluster_To_v1beta1_DockerCluster(src, dst, nil); err != nil {
		return err
	}

	return utilconversion.MarshalData(src, dst)
}

func (src *DockerClusterTemplate) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*infrav1.DockerClusterTemplate)

	return Convert_v1beta1_DockerClusterTemplate_To_v1beta2_DockerClusterTemplate(src, dst, nil)
}

func (dst *DockerClusterTemplate) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*infrav1.DockerClusterTemplate)

	return Convert_v1beta2_DockerClusterTemplate_To_v1beta1_DockerClusterTemplate(src, dst, nil)
}

func (src *DockerMachine) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*infrav1.DockerMachine)

	if err := Convert_v1beta1_DockerMachine_To_v1beta2_DockerMachine(src, dst, nil); err != nil {
		return err
	}

	restored := &infrav1.DockerMachine{}
	ok, err := utilconversion.UnmarshalData(src, restored)
	if err != nil {
		return err
	}

	// Recover intent for bool values converted to *bool.
	initialization := infrav1.DockerMachineInitializationStatus{}
	restoredDockerMachineProvisioned := restored.Status.Initialization.Provisioned
	clusterv1.Convert_bool_To_Pointer_bool(src.Status.Ready, ok, restoredDockerMachineProvisioned, &initialization.Provisioned)
	if !reflect.DeepEqual(initialization, infrav1.DockerMachineInitializationStatus{}) {
		dst.Status.Initialization = initialization
	}

	return nil
}

func (dst *DockerMachine) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*infrav1.DockerMachine)

	if err := Convert_v1beta2_DockerMachine_To_v1beta1_DockerMachine(src, dst, nil); err != nil {
		return err
	}

	if dst.Spec.ProviderID != nil && *dst.Spec.ProviderID == "" {
		dst.Spec.ProviderID = nil
	}

	return utilconversion.MarshalData(src, dst)
}

func (src *DockerMachineTemplate) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*infrav1.DockerMachineTemplate)

	if err := Convert_v1beta1_DockerMachineTemplate_To_v1beta2_DockerMachineTemplate(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &infrav1.DockerMachineTemplate{}
	ok, err := utilconversion.UnmarshalData(src, restored)
	if err != nil {
		return err
	}

	if ok {
		dst.Status = restored.Status
	}

	return nil
}

func (dst *DockerMachineTemplate) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*infrav1.DockerMachineTemplate)

	if err := Convert_v1beta2_DockerMachineTemplate_To_v1beta1_DockerMachineTemplate(src, dst, nil); err != nil {
		return err
	}

	if dst.Spec.Template.Spec.ProviderID != nil && *dst.Spec.Template.Spec.ProviderID == "" {
		dst.Spec.Template.Spec.ProviderID = nil
	}

	return utilconversion.MarshalData(src, dst)
}

func (src *DevCluster) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*infrav1.DevCluster)

	if err := Convert_v1beta1_DevCluster_To_v1beta2_DevCluster(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &infrav1.DevCluster{}
	ok, err := utilconversion.UnmarshalData(src, restored)
	if err != nil {
		return err
	}

	// Recover intent for bool values converted to *bool.
	initialization := infrav1.DevClusterInitializationStatus{}
	restoredDevClusterProvisioned := restored.Status.Initialization.Provisioned
	clusterv1.Convert_bool_To_Pointer_bool(src.Status.Ready, ok, restoredDevClusterProvisioned, &initialization.Provisioned)
	if !reflect.DeepEqual(initialization, infrav1.DevClusterInitializationStatus{}) {
		dst.Status.Initialization = initialization
	}

	return nil
}

func (dst *DevCluster) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*infrav1.DevCluster)

	if err := Convert_v1beta2_DevCluster_To_v1beta1_DevCluster(src, dst, nil); err != nil {
		return err
	}

	return utilconversion.MarshalData(src, dst)
}

func (src *DevClusterTemplate) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*infrav1.DevClusterTemplate)

	return Convert_v1beta1_DevClusterTemplate_To_v1beta2_DevClusterTemplate(src, dst, nil)
}

func (dst *DevClusterTemplate) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*infrav1.DevClusterTemplate)

	return Convert_v1beta2_DevClusterTemplate_To_v1beta1_DevClusterTemplate(src, dst, nil)
}

func (src *DevMachine) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*infrav1.DevMachine)

	if err := Convert_v1beta1_DevMachine_To_v1beta2_DevMachine(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &infrav1.DevMachine{}
	ok, err := utilconversion.UnmarshalData(src, restored)
	if err != nil {
		return err
	}

	// Recover intent for bool values converted to *bool.
	initialization := infrav1.DevMachineInitializationStatus{}
	restoredDevMachineProvisioned := restored.Status.Initialization.Provisioned
	clusterv1.Convert_bool_To_Pointer_bool(src.Status.Ready, ok, restoredDevMachineProvisioned, &initialization.Provisioned)
	if !reflect.DeepEqual(initialization, infrav1.DevMachineInitializationStatus{}) {
		dst.Status.Initialization = initialization
	}

	return nil
}

func (dst *DevMachine) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*infrav1.DevMachine)

	if err := Convert_v1beta2_DevMachine_To_v1beta1_DevMachine(src, dst, nil); err != nil {
		return err
	}

	if dst.Spec.ProviderID != nil && *dst.Spec.ProviderID == "" {
		dst.Spec.ProviderID = nil
	}

	return utilconversion.MarshalData(src, dst)
}

func (src *DevMachineTemplate) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*infrav1.DevMachineTemplate)

	if err := Convert_v1beta1_DevMachineTemplate_To_v1beta2_DevMachineTemplate(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &infrav1.DevMachineTemplate{}
	ok, err := utilconversion.UnmarshalData(src, restored)
	if err != nil {
		return err
	}

	if ok {
		dst.Status = restored.Status
	}

	return nil
}

func (dst *DevMachineTemplate) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*infrav1.DevMachineTemplate)

	if err := Convert_v1beta2_DevMachineTemplate_To_v1beta1_DevMachineTemplate(src, dst, nil); err != nil {
		return err
	}

	if dst.Spec.Template.Spec.ProviderID != nil && *dst.Spec.Template.Spec.ProviderID == "" {
		dst.Spec.Template.Spec.ProviderID = nil
	}

	return utilconversion.MarshalData(src, dst)
}

func Convert_v1beta1_ObjectMeta_To_v1beta2_ObjectMeta(in *clusterv1beta1.ObjectMeta, out *clusterv1.ObjectMeta, s apiconversion.Scope) error {
	return clusterv1beta1.Convert_v1beta1_ObjectMeta_To_v1beta2_ObjectMeta(in, out, s)
}

func Convert_v1beta2_ObjectMeta_To_v1beta1_ObjectMeta(in *clusterv1.ObjectMeta, out *clusterv1beta1.ObjectMeta, s apiconversion.Scope) error {
	return clusterv1beta1.Convert_v1beta2_ObjectMeta_To_v1beta1_ObjectMeta(in, out, s)
}

func Convert_v1beta1_DevClusterStatus_To_v1beta2_DevClusterStatus(in *DevClusterStatus, out *infrav1.DevClusterStatus, s apiconversion.Scope) error {
	if err := autoConvert_v1beta1_DevClusterStatus_To_v1beta2_DevClusterStatus(in, out, s); err != nil {
		return err
	}

	if in.FailureDomains != nil {
		out.FailureDomains = []clusterv1.FailureDomain{}
		domainNames := slices.Collect(maps.Keys(in.FailureDomains))
		sort.Strings(domainNames)
		for _, name := range domainNames {
			domain := in.FailureDomains[name]
			out.FailureDomains = append(out.FailureDomains, clusterv1.FailureDomain{
				Name:         name,
				ControlPlane: ptr.To(domain.ControlPlane),
				Attributes:   domain.Attributes,
			})
		}
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta1 conditions should not be automatically be converted into v1beta2 conditions.
	out.Conditions = nil

	// Retrieve new conditions (v1beta2) from the v1beta2 field.
	if in.V1Beta2 != nil {
		out.Conditions = in.V1Beta2.Conditions
	}

	// Move legacy conditions (v1beta1) to the deprecated field.
	if in.Conditions == nil {
		return nil
	}

	if out.Deprecated == nil {
		out.Deprecated = &infrav1.DevClusterDeprecatedStatus{}
	}
	if out.Deprecated.V1Beta1 == nil {
		out.Deprecated.V1Beta1 = &infrav1.DevClusterV1Beta1DeprecatedStatus{}
	}
	clusterv1beta1.Convert_v1beta1_Conditions_To_v1beta2_Deprecated_V1Beta1_Conditions(&in.Conditions, &out.Deprecated.V1Beta1.Conditions)

	return nil
}

func Convert_v1beta2_DevClusterStatus_To_v1beta1_DevClusterStatus(in *infrav1.DevClusterStatus, out *DevClusterStatus, s apiconversion.Scope) error {
	if err := autoConvert_v1beta2_DevClusterStatus_To_v1beta1_DevClusterStatus(in, out, s); err != nil {
		return err
	}

	out.Ready = ptr.Deref(in.Initialization.Provisioned, false)

	if in.FailureDomains != nil {
		out.FailureDomains = clusterv1beta1.FailureDomains{}
		for _, fd := range in.FailureDomains {
			out.FailureDomains[fd.Name] = clusterv1beta1.FailureDomainSpec{
				ControlPlane: ptr.Deref(fd.ControlPlane, false),
				Attributes:   fd.Attributes,
			}
		}
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta2 conditions should not be automatically be converted into legacy conditions (v1beta1).
	out.Conditions = nil

	// Retrieve legacy conditions (v1beta1) from the deprecated field.
	if in.Deprecated != nil && in.Deprecated.V1Beta1 != nil && in.Deprecated.V1Beta1.Conditions != nil {
		clusterv1beta1.Convert_v1beta2_Deprecated_V1Beta1_Conditions_To_v1beta1_Conditions(&in.Deprecated.V1Beta1.Conditions, &out.Conditions)
	}

	// Move new conditions (v1beta2) to the v1beta2 field.
	if in.Conditions == nil {
		return nil
	}

	out.V1Beta2 = &DevClusterV1Beta2Status{}
	out.V1Beta2.Conditions = in.Conditions
	return nil
}

func Convert_v1beta1_DevMachineStatus_To_v1beta2_DevMachineStatus(in *DevMachineStatus, out *infrav1.DevMachineStatus, s apiconversion.Scope) error {
	if err := autoConvert_v1beta1_DevMachineStatus_To_v1beta2_DevMachineStatus(in, out, s); err != nil {
		return err
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta1 conditions should not be automatically be converted into v1beta2 conditions.
	out.Conditions = nil

	// Retrieve new conditions (v1beta2) from the v1beta2 field.
	if in.V1Beta2 != nil {
		out.Conditions = in.V1Beta2.Conditions
	}

	// Move legacy conditions (v1beta1) to the deprecated field.
	if in.Conditions == nil {
		return nil
	}

	if out.Deprecated == nil {
		out.Deprecated = &infrav1.DevMachineDeprecatedStatus{}
	}
	if out.Deprecated.V1Beta1 == nil {
		out.Deprecated.V1Beta1 = &infrav1.DevMachineV1Beta1DeprecatedStatus{}
	}
	clusterv1beta1.Convert_v1beta1_Conditions_To_v1beta2_Deprecated_V1Beta1_Conditions(&in.Conditions, &out.Deprecated.V1Beta1.Conditions)

	return nil
}

func Convert_v1beta2_DevMachineStatus_To_v1beta1_DevMachineStatus(in *infrav1.DevMachineStatus, out *DevMachineStatus, s apiconversion.Scope) error {
	if err := autoConvert_v1beta2_DevMachineStatus_To_v1beta1_DevMachineStatus(in, out, s); err != nil {
		return err
	}

	out.Ready = ptr.Deref(in.Initialization.Provisioned, false)

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta2 conditions should not be automatically be converted into legacy conditions (v1beta1).
	out.Conditions = nil

	// Retrieve legacy conditions (v1beta1) from the deprecated field.
	if in.Deprecated != nil && in.Deprecated.V1Beta1 != nil && in.Deprecated.V1Beta1.Conditions != nil {
		clusterv1beta1.Convert_v1beta2_Deprecated_V1Beta1_Conditions_To_v1beta1_Conditions(&in.Deprecated.V1Beta1.Conditions, &out.Conditions)
	}

	// Move new conditions (v1beta2) to the v1beta2 field.
	if in.Conditions == nil {
		return nil
	}

	out.V1Beta2 = &DevMachineV1Beta2Status{}
	out.V1Beta2.Conditions = in.Conditions

	return nil
}

func Convert_v1beta1_DockerClusterStatus_To_v1beta2_DockerClusterStatus(in *DockerClusterStatus, out *infrav1.DockerClusterStatus, s apiconversion.Scope) error {
	if err := autoConvert_v1beta1_DockerClusterStatus_To_v1beta2_DockerClusterStatus(in, out, s); err != nil {
		return err
	}

	if in.FailureDomains != nil {
		out.FailureDomains = []clusterv1.FailureDomain{}
		domainNames := slices.Collect(maps.Keys(in.FailureDomains))
		sort.Strings(domainNames)
		for _, name := range domainNames {
			domain := in.FailureDomains[name]
			out.FailureDomains = append(out.FailureDomains, clusterv1.FailureDomain{
				Name:         name,
				ControlPlane: ptr.To(domain.ControlPlane),
				Attributes:   domain.Attributes,
			})
		}
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta1 conditions should not be automatically be converted into v1beta2 conditions.
	out.Conditions = nil

	// Retrieve new conditions (v1beta2) from the v1beta2 field.
	if in.V1Beta2 != nil {
		out.Conditions = in.V1Beta2.Conditions
	}

	// Move legacy conditions (v1beta1) to the deprecated field.
	if in.Conditions == nil {
		return nil
	}

	if out.Deprecated == nil {
		out.Deprecated = &infrav1.DockerClusterDeprecatedStatus{}
	}
	if out.Deprecated.V1Beta1 == nil {
		out.Deprecated.V1Beta1 = &infrav1.DockerClusterV1Beta1DeprecatedStatus{}
	}
	clusterv1beta1.Convert_v1beta1_Conditions_To_v1beta2_Deprecated_V1Beta1_Conditions(&in.Conditions, &out.Deprecated.V1Beta1.Conditions)

	return nil
}

func Convert_v1beta2_DockerClusterStatus_To_v1beta1_DockerClusterStatus(in *infrav1.DockerClusterStatus, out *DockerClusterStatus, s apiconversion.Scope) error {
	if err := autoConvert_v1beta2_DockerClusterStatus_To_v1beta1_DockerClusterStatus(in, out, s); err != nil {
		return err
	}

	out.Ready = ptr.Deref(in.Initialization.Provisioned, false)

	if in.FailureDomains != nil {
		out.FailureDomains = clusterv1beta1.FailureDomains{}
		for _, fd := range in.FailureDomains {
			out.FailureDomains[fd.Name] = clusterv1beta1.FailureDomainSpec{
				ControlPlane: ptr.Deref(fd.ControlPlane, false),
				Attributes:   fd.Attributes,
			}
		}
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta2 conditions should not be automatically be converted into legacy conditions (v1beta1).
	out.Conditions = nil

	// Retrieve legacy conditions (v1beta1) from the deprecated field.
	if in.Deprecated != nil && in.Deprecated.V1Beta1 != nil && in.Deprecated.V1Beta1.Conditions != nil {
		clusterv1beta1.Convert_v1beta2_Deprecated_V1Beta1_Conditions_To_v1beta1_Conditions(&in.Deprecated.V1Beta1.Conditions, &out.Conditions)
	}

	// Move new conditions (v1beta2) to the v1beta2 field.
	if in.Conditions == nil {
		return nil
	}

	out.V1Beta2 = &DockerClusterV1Beta2Status{}
	out.V1Beta2.Conditions = in.Conditions
	return nil
}

func Convert_v1beta1_DockerMachineStatus_To_v1beta2_DockerMachineStatus(in *DockerMachineStatus, out *infrav1.DockerMachineStatus, s apiconversion.Scope) error {
	if err := autoConvert_v1beta1_DockerMachineStatus_To_v1beta2_DockerMachineStatus(in, out, s); err != nil {
		return err
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta1 conditions should not be automatically be converted into v1beta2 conditions.
	out.Conditions = nil

	// Retrieve new conditions (v1beta2) from the v1beta2 field.
	if in.V1Beta2 != nil {
		out.Conditions = in.V1Beta2.Conditions
	}

	// Move legacy conditions (v1beta1) to the deprecated field.
	if in.Conditions == nil {
		return nil
	}

	if out.Deprecated == nil {
		out.Deprecated = &infrav1.DockerMachineDeprecatedStatus{}
	}
	if out.Deprecated.V1Beta1 == nil {
		out.Deprecated.V1Beta1 = &infrav1.DockerMachineV1Beta1DeprecatedStatus{}
	}
	clusterv1beta1.Convert_v1beta1_Conditions_To_v1beta2_Deprecated_V1Beta1_Conditions(&in.Conditions, &out.Deprecated.V1Beta1.Conditions)

	return nil
}

func Convert_v1beta2_DockerMachineStatus_To_v1beta1_DockerMachineStatus(in *infrav1.DockerMachineStatus, out *DockerMachineStatus, s apiconversion.Scope) error {
	if err := autoConvert_v1beta2_DockerMachineStatus_To_v1beta1_DockerMachineStatus(in, out, s); err != nil {
		return err
	}

	if in.Initialization.Provisioned != nil {
		out.Ready = ptr.Deref(in.Initialization.Provisioned, false)
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta2 conditions should not be automatically be converted into legacy conditions (v1beta1).
	out.Conditions = nil

	// Retrieve legacy conditions (v1beta1) from the deprecated field.
	if in.Deprecated != nil && in.Deprecated.V1Beta1 != nil && in.Deprecated.V1Beta1.Conditions != nil {
		clusterv1beta1.Convert_v1beta2_Deprecated_V1Beta1_Conditions_To_v1beta1_Conditions(&in.Deprecated.V1Beta1.Conditions, &out.Conditions)
	}

	// Move new conditions (v1beta2) to the v1beta2 field.
	if in.Conditions == nil {
		return nil
	}

	out.V1Beta2 = &DockerMachineV1Beta2Status{}
	out.V1Beta2.Conditions = in.Conditions

	return nil
}

// Implement local conversion func because conversion-gen is not aware of conversion func in other packages (see https://github.com/kubernetes/code-generator/issues/94)

func Convert_v1_Condition_To_v1beta1_Condition(in *metav1.Condition, out *clusterv1beta1.Condition, s apiconversion.Scope) error {
	return clusterv1beta1.Convert_v1_Condition_To_v1beta1_Condition(in, out, s)
}

func Convert_v1beta1_Condition_To_v1_Condition(in *clusterv1beta1.Condition, out *metav1.Condition, s apiconversion.Scope) error {
	return clusterv1beta1.Convert_v1beta1_Condition_To_v1_Condition(in, out, s)
}

func Convert_v1beta1_DockerClusterSpec_To_v1beta2_DockerClusterSpec(in *DockerClusterSpec, out *infrav1.DockerClusterSpec, s apiconversion.Scope) error {
	if err := autoConvert_v1beta1_DockerClusterSpec_To_v1beta2_DockerClusterSpec(in, out, s); err != nil {
		return err
	}

	// Move FailureDomains
	if in.FailureDomains != nil {
		out.FailureDomains = make([]clusterv1.FailureDomain, 0, len(in.FailureDomains))
		domainNames := slices.Collect(maps.Keys(in.FailureDomains))
		sort.Strings(domainNames)
		for _, name := range domainNames {
			domain := in.FailureDomains[name]
			out.FailureDomains = append(out.FailureDomains, clusterv1.FailureDomain{
				Name:         name,
				ControlPlane: ptr.To(domain.ControlPlane),
				Attributes:   domain.Attributes,
			})
		}
	}

	return nil
}

func Convert_v1beta2_DockerClusterSpec_To_v1beta1_DockerClusterSpec(in *infrav1.DockerClusterSpec, out *DockerClusterSpec, s apiconversion.Scope) error {
	if err := autoConvert_v1beta2_DockerClusterSpec_To_v1beta1_DockerClusterSpec(in, out, s); err != nil {
		return err
	}

	// Move FailureDomains
	if in.FailureDomains != nil {
		out.FailureDomains = clusterv1beta1.FailureDomains{}
		for _, fd := range in.FailureDomains {
			out.FailureDomains[fd.Name] = clusterv1beta1.FailureDomainSpec{
				ControlPlane: ptr.Deref(fd.ControlPlane, false),
				Attributes:   fd.Attributes,
			}
		}
	}

	return nil
}

func Convert_v1beta2_DockerClusterBackendSpec_To_v1beta1_DockerClusterBackendSpec(in *infrav1.DockerClusterBackendSpec, out *DockerClusterBackendSpec, s apiconversion.Scope) error {
	if err := autoConvert_v1beta2_DockerClusterBackendSpec_To_v1beta1_DockerClusterBackendSpec(in, out, s); err != nil {
		return err
	}

	// Move FailureDomains
	if in.FailureDomains != nil {
		out.FailureDomains = clusterv1beta1.FailureDomains{}
		for _, fd := range in.FailureDomains {
			out.FailureDomains[fd.Name] = clusterv1beta1.FailureDomainSpec{
				ControlPlane: ptr.Deref(fd.ControlPlane, false),
				Attributes:   fd.Attributes,
			}
		}
	}

	return nil
}

func Convert_v1beta1_DockerClusterBackendSpec_To_v1beta2_DockerClusterBackendSpec(in *DockerClusterBackendSpec, out *infrav1.DockerClusterBackendSpec, s apiconversion.Scope) error {
	if err := autoConvert_v1beta1_DockerClusterBackendSpec_To_v1beta2_DockerClusterBackendSpec(in, out, s); err != nil {
		return err
	}

	// Move FailureDomains
	if in.FailureDomains != nil {
		out.FailureDomains = make([]clusterv1.FailureDomain, 0, len(in.FailureDomains))
		domainNames := slices.Collect(maps.Keys(in.FailureDomains))
		sort.Strings(domainNames)
		for _, name := range domainNames {
			domain := in.FailureDomains[name]
			out.FailureDomains = append(out.FailureDomains, clusterv1.FailureDomain{
				Name:         name,
				ControlPlane: ptr.To(domain.ControlPlane),
				Attributes:   domain.Attributes,
			})
		}
	}

	return nil
}

func Convert_v1beta2_DockerMachineTemplate_To_v1beta1_DockerMachineTemplate(in *infrav1.DockerMachineTemplate, out *DockerMachineTemplate, s apiconversion.Scope) error {
	return autoConvert_v1beta2_DockerMachineTemplate_To_v1beta1_DockerMachineTemplate(in, out, s)
}

func Convert_v1beta2_DevMachineTemplate_To_v1beta1_DevMachineTemplate(in *infrav1.DevMachineTemplate, out *DevMachineTemplate, s apiconversion.Scope) error {
	return autoConvert_v1beta2_DevMachineTemplate_To_v1beta1_DevMachineTemplate(in, out, s)
}
