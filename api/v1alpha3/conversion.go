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
	"sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/cluster-api/util/conditions"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

func (src *Cluster) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha4.Cluster)

	if err := Convert_v1alpha3_Cluster_To_v1alpha4_Cluster(src, dst, nil); err != nil {
		return err
	}

	// Given this is a bool and there is no timestamp associated with it, when this condition is set, its timestamp
	// will be "now". See https://github.com/kubernetes-sigs/cluster-api/issues/3798#issuecomment-708619826 for more
	// discussion.
	if src.Status.ControlPlaneInitialized {
		conditions.MarkTrue(dst, v1alpha4.ControlPlaneInitializedCondition)
	}

	return nil
}

func (dst *Cluster) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha4.Cluster)

	if err := Convert_v1alpha4_Cluster_To_v1alpha3_Cluster(src, dst, nil); err != nil {
		return err
	}

	// Set the v1alpha3 boolean status field if the v1alpha4 condition was true
	if conditions.IsTrue(src, v1alpha4.ControlPlaneInitializedCondition) {
		dst.Status.ControlPlaneInitialized = true
	}

	return nil
}

func (src *ClusterList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha4.ClusterList)

	return Convert_v1alpha3_ClusterList_To_v1alpha4_ClusterList(src, dst, nil)
}

func (dst *ClusterList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha4.ClusterList)

	return Convert_v1alpha4_ClusterList_To_v1alpha3_ClusterList(src, dst, nil)
}

func (src *Machine) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha4.Machine)

	return Convert_v1alpha3_Machine_To_v1alpha4_Machine(src, dst, nil)
}

func (dst *Machine) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha4.Machine)

	return Convert_v1alpha4_Machine_To_v1alpha3_Machine(src, dst, nil)
}

func (src *MachineList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha4.MachineList)

	return Convert_v1alpha3_MachineList_To_v1alpha4_MachineList(src, dst, nil)
}

func (dst *MachineList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha4.MachineList)

	return Convert_v1alpha4_MachineList_To_v1alpha3_MachineList(src, dst, nil)
}

func (src *MachineSet) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha4.MachineSet)

	return Convert_v1alpha3_MachineSet_To_v1alpha4_MachineSet(src, dst, nil)
}

func (dst *MachineSet) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha4.MachineSet)

	return Convert_v1alpha4_MachineSet_To_v1alpha3_MachineSet(src, dst, nil)
}

func (src *MachineSetList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha4.MachineSetList)

	return Convert_v1alpha3_MachineSetList_To_v1alpha4_MachineSetList(src, dst, nil)
}

func (dst *MachineSetList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha4.MachineSetList)

	return Convert_v1alpha4_MachineSetList_To_v1alpha3_MachineSetList(src, dst, nil)
}

func (src *MachineDeployment) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha4.MachineDeployment)

	if err := Convert_v1alpha3_MachineDeployment_To_v1alpha4_MachineDeployment(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &v1alpha4.MachineDeployment{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	if restored.Spec.Strategy != nil && restored.Spec.Strategy.RollingUpdate != nil {
		if dst.Spec.Strategy == nil {
			dst.Spec.Strategy = &v1alpha4.MachineDeploymentStrategy{}
		}
		if dst.Spec.Strategy.RollingUpdate == nil {
			dst.Spec.Strategy.RollingUpdate = &v1alpha4.MachineRollingUpdateDeployment{}
		}
		dst.Spec.Strategy.RollingUpdate.DeletePolicy = restored.Spec.Strategy.RollingUpdate.DeletePolicy

	}

	return nil
}

func (dst *MachineDeployment) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha4.MachineDeployment)

	if err := Convert_v1alpha4_MachineDeployment_To_v1alpha3_MachineDeployment(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion except for metadata
	if err := utilconversion.MarshalData(src, dst); err != nil {
		return err
	}

	return nil
}

func (src *MachineDeploymentList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha4.MachineDeploymentList)

	return Convert_v1alpha3_MachineDeploymentList_To_v1alpha4_MachineDeploymentList(src, dst, nil)
}

func (dst *MachineDeploymentList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha4.MachineDeploymentList)

	return Convert_v1alpha4_MachineDeploymentList_To_v1alpha3_MachineDeploymentList(src, dst, nil)
}

func (src *MachineHealthCheck) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha4.MachineHealthCheck)

	if err := Convert_v1alpha3_MachineHealthCheck_To_v1alpha4_MachineHealthCheck(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &v1alpha4.MachineHealthCheck{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	if restored.Spec.UnhealthyRange != nil {
		dst.Spec.UnhealthyRange = restored.Spec.UnhealthyRange
	}

	return nil
}

func (dst *MachineHealthCheck) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha4.MachineHealthCheck)

	if err := Convert_v1alpha4_MachineHealthCheck_To_v1alpha3_MachineHealthCheck(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion except for metadata
	if err := utilconversion.MarshalData(src, dst); err != nil {
		return err
	}

	return nil
}

func (src *MachineHealthCheckList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha4.MachineHealthCheckList)

	return Convert_v1alpha3_MachineHealthCheckList_To_v1alpha4_MachineHealthCheckList(src, dst, nil)
}

func (dst *MachineHealthCheckList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha4.MachineHealthCheckList)

	return Convert_v1alpha4_MachineHealthCheckList_To_v1alpha3_MachineHealthCheckList(src, dst, nil)
}

// Convert_v1alpha3_Bootstrap_To_v1alpha4_Bootstrap is an autogenerated conversion function.
func Convert_v1alpha3_Bootstrap_To_v1alpha4_Bootstrap(in *Bootstrap, out *v1alpha4.Bootstrap, s apiconversion.Scope) error { //nolint
	return autoConvert_v1alpha3_Bootstrap_To_v1alpha4_Bootstrap(in, out, s)
}

func Convert_v1alpha4_MachineRollingUpdateDeployment_To_v1alpha3_MachineRollingUpdateDeployment(in *v1alpha4.MachineRollingUpdateDeployment, out *MachineRollingUpdateDeployment, s apiconversion.Scope) error {
	return autoConvert_v1alpha4_MachineRollingUpdateDeployment_To_v1alpha3_MachineRollingUpdateDeployment(in, out, s)
}

func Convert_v1alpha4_MachineHealthCheckSpec_To_v1alpha3_MachineHealthCheckSpec(in *v1alpha4.MachineHealthCheckSpec, out *MachineHealthCheckSpec, s apiconversion.Scope) error {
	return autoConvert_v1alpha4_MachineHealthCheckSpec_To_v1alpha3_MachineHealthCheckSpec(in, out, s)
}

func Convert_v1alpha3_ClusterStatus_To_v1alpha4_ClusterStatus(in *ClusterStatus, out *v1alpha4.ClusterStatus, s apiconversion.Scope) error {
	return autoConvert_v1alpha3_ClusterStatus_To_v1alpha4_ClusterStatus(in, out, s)
}

func Convert_v1alpha3_ObjectMeta_To_v1alpha4_ObjectMeta(in *ObjectMeta, out *v1alpha4.ObjectMeta, s apiconversion.Scope) error {
	return autoConvert_v1alpha3_ObjectMeta_To_v1alpha4_ObjectMeta(in, out, s)
}
