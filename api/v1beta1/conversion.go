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
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta2"
)

func (src *Cluster) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*clusterv1.Cluster)

	return Convert_v1beta1_Cluster_To_v1beta2_Cluster(src, dst, nil)
}

func (dst *Cluster) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.Cluster)

	return Convert_v1beta2_Cluster_To_v1beta1_Cluster(src, dst, nil)
}

func (src *ClusterClass) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*clusterv1.ClusterClass)

	return Convert_v1beta1_ClusterClass_To_v1beta2_ClusterClass(src, dst, nil)
}

func (dst *ClusterClass) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.ClusterClass)

	return Convert_v1beta2_ClusterClass_To_v1beta1_ClusterClass(src, dst, nil)
}

func (src *Machine) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*clusterv1.Machine)

	return Convert_v1beta1_Machine_To_v1beta2_Machine(src, dst, nil)
}

func (dst *Machine) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.Machine)

	return Convert_v1beta2_Machine_To_v1beta1_Machine(src, dst, nil)
}

func (src *MachineSet) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*clusterv1.MachineSet)

	return Convert_v1beta1_MachineSet_To_v1beta2_MachineSet(src, dst, nil)
}

func (dst *MachineSet) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.MachineSet)

	return Convert_v1beta2_MachineSet_To_v1beta1_MachineSet(src, dst, nil)
}

func (src *MachineDeployment) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*clusterv1.MachineDeployment)

	return Convert_v1beta1_MachineDeployment_To_v1beta2_MachineDeployment(src, dst, nil)
}

func (dst *MachineDeployment) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.MachineDeployment)

	return Convert_v1beta2_MachineDeployment_To_v1beta1_MachineDeployment(src, dst, nil)
}

func (src *MachineHealthCheck) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*clusterv1.MachineHealthCheck)

	return Convert_v1beta1_MachineHealthCheck_To_v1beta2_MachineHealthCheck(src, dst, nil)
}

func (dst *MachineHealthCheck) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.MachineHealthCheck)

	return Convert_v1beta2_MachineHealthCheck_To_v1beta1_MachineHealthCheck(src, dst, nil)
}
