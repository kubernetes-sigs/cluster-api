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
	apiconversion "k8s.io/apimachinery/pkg/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta2"
)

func (src *DockerCluster) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*infrav1.DockerCluster)

	return Convert_v1beta1_DockerCluster_To_v1beta2_DockerCluster(src, dst, nil)
}

func (dst *DockerCluster) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*infrav1.DockerCluster)

	return Convert_v1beta2_DockerCluster_To_v1beta1_DockerCluster(src, dst, nil)
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

	return Convert_v1beta1_DockerMachine_To_v1beta2_DockerMachine(src, dst, nil)
}

func (dst *DockerMachine) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*infrav1.DockerMachine)

	return Convert_v1beta2_DockerMachine_To_v1beta1_DockerMachine(src, dst, nil)
}

func (src *DockerMachineTemplate) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*infrav1.DockerMachineTemplate)

	return Convert_v1beta1_DockerMachineTemplate_To_v1beta2_DockerMachineTemplate(src, dst, nil)
}

func (dst *DockerMachineTemplate) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*infrav1.DockerMachineTemplate)

	return Convert_v1beta2_DockerMachineTemplate_To_v1beta1_DockerMachineTemplate(src, dst, nil)
}

func (src *DevCluster) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*infrav1.DevCluster)

	return Convert_v1beta1_DevCluster_To_v1beta2_DevCluster(src, dst, nil)
}

func (dst *DevCluster) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*infrav1.DevCluster)

	return Convert_v1beta2_DevCluster_To_v1beta1_DevCluster(src, dst, nil)
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

	return Convert_v1beta1_DevMachine_To_v1beta2_DevMachine(src, dst, nil)
}

func (dst *DevMachine) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*infrav1.DevMachine)

	return Convert_v1beta2_DevMachine_To_v1beta1_DevMachine(src, dst, nil)
}

func (src *DevMachineTemplate) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*infrav1.DevMachineTemplate)

	return Convert_v1beta1_DevMachineTemplate_To_v1beta2_DevMachineTemplate(src, dst, nil)
}

func (dst *DevMachineTemplate) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*infrav1.DevMachineTemplate)

	return Convert_v1beta2_DevMachineTemplate_To_v1beta1_DevMachineTemplate(src, dst, nil)
}

func Convert_v1beta1_ObjectMeta_To_v1beta2_ObjectMeta(in *clusterv1beta1.ObjectMeta, out *clusterv1.ObjectMeta, s apiconversion.Scope) error {
	return clusterv1beta1.Convert_v1beta1_ObjectMeta_To_v1beta2_ObjectMeta(in, out, s)
}

func Convert_v1beta2_ObjectMeta_To_v1beta1_ObjectMeta(in *clusterv1.ObjectMeta, out *clusterv1beta1.ObjectMeta, s apiconversion.Scope) error {
	return clusterv1beta1.Convert_v1beta2_ObjectMeta_To_v1beta1_ObjectMeta(in, out, s)
}
