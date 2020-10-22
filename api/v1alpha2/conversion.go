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

package v1alpha2

import (
	apiconversion "k8s.io/apimachinery/pkg/conversion"
	"sigs.k8s.io/cluster-api/api/v1alpha3"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

var (
	v2Annotations = []string{RevisionAnnotation, RevisionHistoryAnnotation, DesiredReplicasAnnotation, MaxReplicasAnnotation}
	v3Annotations = []string{v1alpha3.RevisionAnnotation, v1alpha3.RevisionHistoryAnnotation, v1alpha3.DesiredReplicasAnnotation, v1alpha3.MaxReplicasAnnotation}
)

func (src *Cluster) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha3.Cluster)
	if err := Convert_v1alpha2_Cluster_To_v1alpha3_Cluster(src, dst, nil); err != nil {
		return err
	}

	// Manually convert Status.APIEndpoints to Spec.ControlPlaneEndpoint.
	if len(src.Status.APIEndpoints) > 0 {
		endpoint := src.Status.APIEndpoints[0]
		dst.Spec.ControlPlaneEndpoint.Host = endpoint.Host
		dst.Spec.ControlPlaneEndpoint.Port = int32(endpoint.Port)
	}

	// Manually restore data.
	restored := &v1alpha3.Cluster{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	dst.Spec.ControlPlaneRef = restored.Spec.ControlPlaneRef
	dst.Status.ControlPlaneReady = restored.Status.ControlPlaneReady
	dst.Status.FailureDomains = restored.Status.FailureDomains
	dst.Spec.Paused = restored.Spec.Paused
	dst.Status.Conditions = restored.Status.Conditions
	dst.Status.ObservedGeneration = restored.Status.ObservedGeneration

	return nil
}

func (dst *Cluster) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha3.Cluster)
	if err := Convert_v1alpha3_Cluster_To_v1alpha2_Cluster(src, dst, nil); err != nil {
		return err
	}

	// Manually convert Spec.ControlPlaneEndpoint to Status.APIEndpoints.
	if !src.Spec.ControlPlaneEndpoint.IsZero() {
		dst.Status.APIEndpoints = []APIEndpoint{
			{
				Host: src.Spec.ControlPlaneEndpoint.Host,
				Port: int(src.Spec.ControlPlaneEndpoint.Port),
			},
		}
	}

	// Preserve Hub data on down-conversion except for metadata
	if err := utilconversion.MarshalData(src, dst); err != nil {
		return err
	}

	return nil
}

func (src *ClusterList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha3.ClusterList)

	return Convert_v1alpha2_ClusterList_To_v1alpha3_ClusterList(src, dst, nil)
}

func (dst *ClusterList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha3.ClusterList)

	return Convert_v1alpha3_ClusterList_To_v1alpha2_ClusterList(src, dst, nil)
}

func (src *Machine) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha3.Machine)
	if err := Convert_v1alpha2_Machine_To_v1alpha3_Machine(src, dst, nil); err != nil {
		return err
	}

	// Manually convert ExcludeNodeDrainingAnnotation annotation if set.
	if val, ok := src.Annotations[ExcludeNodeDrainingAnnotation]; ok {
		src.Annotations[v1alpha3.ExcludeNodeDrainingAnnotation] = val
		delete(src.Annotations, ExcludeNodeDrainingAnnotation)
	}

	// Manually convert ClusterName from label, if any.
	// This conversion can be overwritten when restoring the ClusterName field.
	if name, ok := src.Labels[MachineClusterLabelName]; ok {
		dst.Spec.ClusterName = name
	}

	// Manually restore data.
	restored := &v1alpha3.Machine{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}
	restoreMachineSpec(&restored.Spec, &dst.Spec)
	dst.Status.ObservedGeneration = restored.Status.ObservedGeneration
	dst.Status.Conditions = restored.Status.Conditions
	return nil
}

func restoreMachineSpec(restored *v1alpha3.MachineSpec, dst *v1alpha3.MachineSpec) {
	if restored.ClusterName != "" {
		dst.ClusterName = restored.ClusterName
	}
	dst.Bootstrap.DataSecretName = restored.Bootstrap.DataSecretName
	dst.FailureDomain = restored.FailureDomain
	dst.NodeDrainTimeout = restored.NodeDrainTimeout
}

func (dst *Machine) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha3.Machine)
	if err := Convert_v1alpha3_Machine_To_v1alpha2_Machine(src, dst, nil); err != nil {
		return err
	}

	// Manually convert ExcludeNodeDrainingAnnotation annotation if set.
	if val, ok := src.Annotations[v1alpha3.ExcludeNodeDrainingAnnotation]; ok {
		src.Annotations[ExcludeNodeDrainingAnnotation] = val
		delete(src.Annotations, v1alpha3.ExcludeNodeDrainingAnnotation)
	}

	// Preserve Hub data on down-conversion except for metadata
	if err := utilconversion.MarshalData(src, dst); err != nil {
		return err
	}

	return nil
}

func (src *MachineList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha3.MachineList)

	return Convert_v1alpha2_MachineList_To_v1alpha3_MachineList(src, dst, nil)
}

func (dst *MachineList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha3.MachineList)

	return Convert_v1alpha3_MachineList_To_v1alpha2_MachineList(src, dst, nil)
}

func (src *MachineSet) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha3.MachineSet)
	if err := Convert_v1alpha2_MachineSet_To_v1alpha3_MachineSet(src, dst, nil); err != nil {
		return err
	}

	// Manually convert ClusterName from label, if any.
	// This conversion can be overwritten when restoring the ClusterName field.
	if name, ok := src.Labels[MachineClusterLabelName]; ok {
		dst.Spec.ClusterName = name
		dst.Spec.Template.Spec.ClusterName = name
	}

	// Manually convert annotations
	for i := range v2Annotations {
		convertAnnotations(v2Annotations[i], v3Annotations[i], dst.Annotations)
	}

	// Manually restore data.
	restored := &v1alpha3.MachineSet{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	if restored.Spec.ClusterName != "" {
		dst.Spec.ClusterName = restored.Spec.ClusterName
	}
	restoreMachineSpec(&restored.Spec.Template.Spec, &dst.Spec.Template.Spec)

	return nil
}

func (dst *MachineSet) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha3.MachineSet)
	if err := Convert_v1alpha3_MachineSet_To_v1alpha2_MachineSet(src, dst, nil); err != nil {
		return err
	}

	// Manually convert annotations
	for i := range v3Annotations {
		convertAnnotations(v3Annotations[i], v2Annotations[i], dst.Annotations)
	}

	// Preserve Hub data on down-conversion except for metadata
	if err := utilconversion.MarshalData(src, dst); err != nil {
		return err
	}

	return nil
}

func (src *MachineSetList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha3.MachineSetList)

	return Convert_v1alpha2_MachineSetList_To_v1alpha3_MachineSetList(src, dst, nil)
}

func (dst *MachineSetList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha3.MachineSetList)

	return Convert_v1alpha3_MachineSetList_To_v1alpha2_MachineSetList(src, dst, nil)
}

func (src *MachineDeployment) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha3.MachineDeployment)
	if err := Convert_v1alpha2_MachineDeployment_To_v1alpha3_MachineDeployment(src, dst, nil); err != nil {
		return err
	}

	// Manually convert ClusterName from label, if any.
	// This conversion can be overwritten when restoring the ClusterName field.
	if name, ok := src.Labels[MachineClusterLabelName]; ok {
		dst.Spec.ClusterName = name
		dst.Spec.Template.Spec.ClusterName = name
	}

	// Manually convert annotations
	for i := range v2Annotations {
		convertAnnotations(v2Annotations[i], v3Annotations[i], dst.Annotations)
	}

	// Manually restore data.
	restored := &v1alpha3.MachineDeployment{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	if restored.Spec.ClusterName != "" {
		dst.Spec.ClusterName = restored.Spec.ClusterName
	}
	dst.Spec.Paused = restored.Spec.Paused
	dst.Status.Phase = restored.Status.Phase
	restoreMachineSpec(&restored.Spec.Template.Spec, &dst.Spec.Template.Spec)

	return nil
}

func (dst *MachineDeployment) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha3.MachineDeployment)
	if err := Convert_v1alpha3_MachineDeployment_To_v1alpha2_MachineDeployment(src, dst, nil); err != nil {
		return err
	}

	// Manually convert annotations
	for i := range v3Annotations {
		convertAnnotations(v3Annotations[i], v2Annotations[i], dst.Annotations)
	}

	// Preserve Hub data on down-conversion except for metadata
	if err := utilconversion.MarshalData(src, dst); err != nil {
		return err
	}

	return nil
}

func convertAnnotations(fromAnnotation string, toAnnotation string, annotations map[string]string) {
	if value, ok := annotations[fromAnnotation]; ok {
		delete(annotations, fromAnnotation)
		annotations[toAnnotation] = value
	}
}

func (src *MachineDeploymentList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha3.MachineDeploymentList)

	return Convert_v1alpha2_MachineDeploymentList_To_v1alpha3_MachineDeploymentList(src, dst, nil)
}

func (dst *MachineDeploymentList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha3.MachineDeploymentList)

	return Convert_v1alpha3_MachineDeploymentList_To_v1alpha2_MachineDeploymentList(src, dst, nil)
}

func Convert_v1alpha2_MachineSpec_To_v1alpha3_MachineSpec(in *MachineSpec, out *v1alpha3.MachineSpec, s apiconversion.Scope) error {
	if err := autoConvert_v1alpha2_MachineSpec_To_v1alpha3_MachineSpec(in, out, s); err != nil {
		return err
	}

	// Discards unused ObjectMeta

	return nil
}

func Convert_v1alpha2_ClusterSpec_To_v1alpha3_ClusterSpec(in *ClusterSpec, out *v1alpha3.ClusterSpec, s apiconversion.Scope) error {
	if err := autoConvert_v1alpha2_ClusterSpec_To_v1alpha3_ClusterSpec(in, out, s); err != nil {
		return err
	}

	return nil
}

func Convert_v1alpha2_ClusterStatus_To_v1alpha3_ClusterStatus(in *ClusterStatus, out *v1alpha3.ClusterStatus, s apiconversion.Scope) error {
	if err := autoConvert_v1alpha2_ClusterStatus_To_v1alpha3_ClusterStatus(in, out, s); err != nil {
		return err
	}

	// Manually convert the Error fields to the Failure fields
	out.FailureMessage = in.ErrorMessage
	out.FailureReason = in.ErrorReason

	return nil
}

func Convert_v1alpha3_ClusterStatus_To_v1alpha2_ClusterStatus(in *v1alpha3.ClusterStatus, out *ClusterStatus, s apiconversion.Scope) error {
	if err := autoConvert_v1alpha3_ClusterStatus_To_v1alpha2_ClusterStatus(in, out, s); err != nil {
		return err
	}

	// Manually convert the Failure fields to the Error fields
	out.ErrorMessage = in.FailureMessage
	out.ErrorReason = in.FailureReason

	return nil
}

func Convert_v1alpha2_MachineSetStatus_To_v1alpha3_MachineSetStatus(in *MachineSetStatus, out *v1alpha3.MachineSetStatus, s apiconversion.Scope) error {
	if err := autoConvert_v1alpha2_MachineSetStatus_To_v1alpha3_MachineSetStatus(in, out, s); err != nil {
		return err
	}

	// Manually convert the Error fields to the Failure fields
	out.FailureMessage = in.ErrorMessage
	out.FailureReason = in.ErrorReason

	return nil
}

func Convert_v1alpha3_MachineSetStatus_To_v1alpha2_MachineSetStatus(in *v1alpha3.MachineSetStatus, out *MachineSetStatus, s apiconversion.Scope) error {
	if err := autoConvert_v1alpha3_MachineSetStatus_To_v1alpha2_MachineSetStatus(in, out, s); err != nil {
		return err
	}

	// Manually convert the Failure fields to the Error fields
	out.ErrorMessage = in.FailureMessage
	out.ErrorReason = in.FailureReason

	return nil
}

func Convert_v1alpha2_MachineStatus_To_v1alpha3_MachineStatus(in *MachineStatus, out *v1alpha3.MachineStatus, s apiconversion.Scope) error {
	if err := autoConvert_v1alpha2_MachineStatus_To_v1alpha3_MachineStatus(in, out, s); err != nil {
		return err
	}

	// Manually convert the Error fields to the Failure fields
	out.FailureMessage = in.ErrorMessage
	out.FailureReason = in.ErrorReason

	return nil
}

func Convert_v1alpha3_ClusterSpec_To_v1alpha2_ClusterSpec(in *v1alpha3.ClusterSpec, out *ClusterSpec, s apiconversion.Scope) error {
	if err := autoConvert_v1alpha3_ClusterSpec_To_v1alpha2_ClusterSpec(in, out, s); err != nil {
		return err
	}
	return nil
}

func Convert_v1alpha3_MachineStatus_To_v1alpha2_MachineStatus(in *v1alpha3.MachineStatus, out *MachineStatus, s apiconversion.Scope) error {
	if err := autoConvert_v1alpha3_MachineStatus_To_v1alpha2_MachineStatus(in, out, s); err != nil {
		return err
	}

	// Manually convert the Failure fields to the Error fields
	out.ErrorMessage = in.FailureMessage
	out.ErrorReason = in.FailureReason

	return nil
}

func Convert_v1alpha3_MachineDeploymentSpec_To_v1alpha2_MachineDeploymentSpec(in *v1alpha3.MachineDeploymentSpec, out *MachineDeploymentSpec, s apiconversion.Scope) error {
	return autoConvert_v1alpha3_MachineDeploymentSpec_To_v1alpha2_MachineDeploymentSpec(in, out, s)
}

func Convert_v1alpha3_MachineDeploymentStatus_To_v1alpha2_MachineDeploymentStatus(in *v1alpha3.MachineDeploymentStatus, out *MachineDeploymentStatus, s apiconversion.Scope) error {
	return autoConvert_v1alpha3_MachineDeploymentStatus_To_v1alpha2_MachineDeploymentStatus(in, out, s)
}

func Convert_v1alpha3_MachineSetSpec_To_v1alpha2_MachineSetSpec(in *v1alpha3.MachineSetSpec, out *MachineSetSpec, s apiconversion.Scope) error {
	return autoConvert_v1alpha3_MachineSetSpec_To_v1alpha2_MachineSetSpec(in, out, s)
}

func Convert_v1alpha3_MachineSpec_To_v1alpha2_MachineSpec(in *v1alpha3.MachineSpec, out *MachineSpec, s apiconversion.Scope) error {
	return autoConvert_v1alpha3_MachineSpec_To_v1alpha2_MachineSpec(in, out, s)
}

func Convert_v1alpha3_Bootstrap_To_v1alpha2_Bootstrap(in *v1alpha3.Bootstrap, out *Bootstrap, s apiconversion.Scope) error {
	return autoConvert_v1alpha3_Bootstrap_To_v1alpha2_Bootstrap(in, out, s)
}
