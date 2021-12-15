/*
Copyright 2019 The Kubernetes Authors.

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

package v1alpha3

import (
	apiconversion "k8s.io/apimachinery/pkg/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/conditions"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

func (src *Cluster) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*clusterv1.Cluster)

	if err := Convert_v1alpha3_Cluster_To_v1beta1_Cluster(src, dst, nil); err != nil {
		return err
	}

	// Given this is a bool and there is no timestamp associated with it, when this condition is set, its timestamp
	// will be "now". See https://github.com/kubernetes-sigs/cluster-api/issues/3798#issuecomment-708619826 for more
	// discussion.
	if src.Status.ControlPlaneInitialized {
		conditions.MarkTrue(dst, clusterv1.ControlPlaneInitializedCondition)
	}

	// Manually restore data.
	restored := &clusterv1.Cluster{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	if restored.Spec.Topology != nil {
		dst.Spec.Topology = restored.Spec.Topology
	}

	return nil
}

func (dst *Cluster) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.Cluster)

	if err := Convert_v1beta1_Cluster_To_v1alpha3_Cluster(src, dst, nil); err != nil {
		return err
	}

	// Set the v1alpha3 boolean status field if the v1alpha4 condition was true
	if conditions.IsTrue(src, clusterv1.ControlPlaneInitializedCondition) {
		dst.Status.ControlPlaneInitialized = true
	}

	// Preserve Hub data on down-conversion except for metadata
	if err := utilconversion.MarshalData(src, dst); err != nil {
		return err
	}

	return nil
}

func (src *ClusterList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*clusterv1.ClusterList)

	return Convert_v1alpha3_ClusterList_To_v1beta1_ClusterList(src, dst, nil)
}

func (dst *ClusterList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.ClusterList)

	return Convert_v1beta1_ClusterList_To_v1alpha3_ClusterList(src, dst, nil)
}

func (src *Machine) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*clusterv1.Machine)

	if err := Convert_v1alpha3_Machine_To_v1beta1_Machine(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &clusterv1.Machine{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	dst.Status.NodeInfo = restored.Status.NodeInfo
	return nil
}

func (dst *Machine) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.Machine)

	if err := Convert_v1beta1_Machine_To_v1alpha3_Machine(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion except for metadata
	if err := utilconversion.MarshalData(src, dst); err != nil {
		return err
	}

	return nil
}

func (src *MachineList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*clusterv1.MachineList)

	return Convert_v1alpha3_MachineList_To_v1beta1_MachineList(src, dst, nil)
}

func (dst *MachineList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.MachineList)

	return Convert_v1beta1_MachineList_To_v1alpha3_MachineList(src, dst, nil)
}

func (src *MachineSet) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*clusterv1.MachineSet)

	if err := Convert_v1alpha3_MachineSet_To_v1beta1_MachineSet(src, dst, nil); err != nil {
		return err
	}
	// Manually restore data.
	restored := &clusterv1.MachineSet{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}
	dst.Status.Conditions = restored.Status.Conditions
	return nil
}

func (dst *MachineSet) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.MachineSet)

	if err := Convert_v1beta1_MachineSet_To_v1alpha3_MachineSet(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion except for metadata
	if err := utilconversion.MarshalData(src, dst); err != nil {
		return err
	}
	return nil
}

func (src *MachineSetList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*clusterv1.MachineSetList)

	return Convert_v1alpha3_MachineSetList_To_v1beta1_MachineSetList(src, dst, nil)
}

func (dst *MachineSetList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.MachineSetList)

	return Convert_v1beta1_MachineSetList_To_v1alpha3_MachineSetList(src, dst, nil)
}

func (src *MachineDeployment) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*clusterv1.MachineDeployment)

	if err := Convert_v1alpha3_MachineDeployment_To_v1beta1_MachineDeployment(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &clusterv1.MachineDeployment{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	if restored.Spec.Strategy != nil && restored.Spec.Strategy.RollingUpdate != nil {
		if dst.Spec.Strategy == nil {
			dst.Spec.Strategy = &clusterv1.MachineDeploymentStrategy{}
		}
		if dst.Spec.Strategy.RollingUpdate == nil {
			dst.Spec.Strategy.RollingUpdate = &clusterv1.MachineRollingUpdateDeployment{}
		}
		dst.Spec.Strategy.RollingUpdate.DeletePolicy = restored.Spec.Strategy.RollingUpdate.DeletePolicy
	}

	dst.Status.Conditions = restored.Status.Conditions
	return nil
}

func (dst *MachineDeployment) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.MachineDeployment)

	if err := Convert_v1beta1_MachineDeployment_To_v1alpha3_MachineDeployment(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion except for metadata
	if err := utilconversion.MarshalData(src, dst); err != nil {
		return err
	}

	return nil
}

func (src *MachineDeploymentList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*clusterv1.MachineDeploymentList)

	return Convert_v1alpha3_MachineDeploymentList_To_v1beta1_MachineDeploymentList(src, dst, nil)
}

func (dst *MachineDeploymentList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.MachineDeploymentList)

	return Convert_v1beta1_MachineDeploymentList_To_v1alpha3_MachineDeploymentList(src, dst, nil)
}

func (src *MachineHealthCheck) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*clusterv1.MachineHealthCheck)

	if err := Convert_v1alpha3_MachineHealthCheck_To_v1beta1_MachineHealthCheck(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &clusterv1.MachineHealthCheck{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	if restored.Spec.UnhealthyRange != nil {
		dst.Spec.UnhealthyRange = restored.Spec.UnhealthyRange
	}

	return nil
}

func (dst *MachineHealthCheck) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.MachineHealthCheck)

	if err := Convert_v1beta1_MachineHealthCheck_To_v1alpha3_MachineHealthCheck(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion except for metadata
	if err := utilconversion.MarshalData(src, dst); err != nil {
		return err
	}

	return nil
}

func (src *MachineHealthCheckList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*clusterv1.MachineHealthCheckList)

	return Convert_v1alpha3_MachineHealthCheckList_To_v1beta1_MachineHealthCheckList(src, dst, nil)
}

func (dst *MachineHealthCheckList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.MachineHealthCheckList)

	return Convert_v1beta1_MachineHealthCheckList_To_v1alpha3_MachineHealthCheckList(src, dst, nil)
}

func Convert_v1beta1_MachineSetStatus_To_v1alpha3_MachineSetStatus(in *clusterv1.MachineSetStatus, out *MachineSetStatus, s apiconversion.Scope) error {
	// Status.Conditions was introduced in v1alpha4, thus requiring a custom conversion function; the values is going to be preserved in an annotation thus allowing roundtrip without loosing informations
	return autoConvert_v1beta1_MachineSetStatus_To_v1alpha3_MachineSetStatus(in, out, nil)
}

func Convert_v1beta1_ClusterSpec_To_v1alpha3_ClusterSpec(in *clusterv1.ClusterSpec, out *ClusterSpec, s apiconversion.Scope) error {
	// NOTE: custom conversion func is required because spec.Topology does not exists in v1alpha3
	return autoConvert_v1beta1_ClusterSpec_To_v1alpha3_ClusterSpec(in, out, s)
}

func Convert_v1alpha3_Bootstrap_To_v1beta1_Bootstrap(in *Bootstrap, out *clusterv1.Bootstrap, s apiconversion.Scope) error {
	return autoConvert_v1alpha3_Bootstrap_To_v1beta1_Bootstrap(in, out, s)
}

func Convert_v1beta1_MachineRollingUpdateDeployment_To_v1alpha3_MachineRollingUpdateDeployment(in *clusterv1.MachineRollingUpdateDeployment, out *MachineRollingUpdateDeployment, s apiconversion.Scope) error {
	return autoConvert_v1beta1_MachineRollingUpdateDeployment_To_v1alpha3_MachineRollingUpdateDeployment(in, out, s)
}

func Convert_v1beta1_MachineHealthCheckSpec_To_v1alpha3_MachineHealthCheckSpec(in *clusterv1.MachineHealthCheckSpec, out *MachineHealthCheckSpec, s apiconversion.Scope) error {
	return autoConvert_v1beta1_MachineHealthCheckSpec_To_v1alpha3_MachineHealthCheckSpec(in, out, s)
}

func Convert_v1alpha3_ClusterStatus_To_v1beta1_ClusterStatus(in *ClusterStatus, out *clusterv1.ClusterStatus, s apiconversion.Scope) error {
	return autoConvert_v1alpha3_ClusterStatus_To_v1beta1_ClusterStatus(in, out, s)
}

func Convert_v1alpha3_ObjectMeta_To_v1beta1_ObjectMeta(in *ObjectMeta, out *clusterv1.ObjectMeta, s apiconversion.Scope) error {
	return autoConvert_v1alpha3_ObjectMeta_To_v1beta1_ObjectMeta(in, out, s)
}

func Convert_v1beta1_MachineStatus_To_v1alpha3_MachineStatus(in *clusterv1.MachineStatus, out *MachineStatus, s apiconversion.Scope) error {
	return autoConvert_v1beta1_MachineStatus_To_v1alpha3_MachineStatus(in, out, s)
}

func Convert_v1beta1_MachineDeploymentStatus_To_v1alpha3_MachineDeploymentStatus(in *clusterv1.MachineDeploymentStatus, out *MachineDeploymentStatus, s apiconversion.Scope) error {
	// Status.Conditions was introduced in v1alpha4, thus requiring a custom conversion function; the values is going to be preserved in an annotation thus allowing roundtrip without loosing informations
	return autoConvert_v1beta1_MachineDeploymentStatus_To_v1alpha3_MachineDeploymentStatus(in, out, s)
}

func Convert_v1alpha3_MachineStatus_To_v1beta1_MachineStatus(in *MachineStatus, out *clusterv1.MachineStatus, s apiconversion.Scope) error {
	// Status.version has been removed in v1beta1, thus requiring custom conversion function. the information will be dropped.
	return autoConvert_v1alpha3_MachineStatus_To_v1beta1_MachineStatus(in, out, s)
}
