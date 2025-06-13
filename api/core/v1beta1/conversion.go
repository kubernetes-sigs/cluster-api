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
	"unsafe"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apimachineryconversion "k8s.io/apimachinery/pkg/conversion"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
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

	if err := Convert_v1beta1_Machine_To_v1beta2_Machine(src, dst, nil); err != nil {
		return err
	}

	restored := &clusterv1.Machine{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	dst.Spec.MinReadySeconds = restored.Spec.MinReadySeconds

	return nil
}

func (dst *Machine) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.Machine)

	if err := Convert_v1beta2_Machine_To_v1beta1_Machine(src, dst, nil); err != nil {
		return err
	}

	return utilconversion.MarshalData(src, dst)
}

func (src *MachineSet) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*clusterv1.MachineSet)

	if err := Convert_v1beta1_MachineSet_To_v1beta2_MachineSet(src, dst, nil); err != nil {
		return err
	}

	if src.Spec.MinReadySeconds == 0 {
		dst.Spec.Template.Spec.MinReadySeconds = nil
	} else {
		dst.Spec.Template.Spec.MinReadySeconds = &src.Spec.MinReadySeconds
	}

	return nil
}

func (dst *MachineSet) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.MachineSet)

	if err := Convert_v1beta2_MachineSet_To_v1beta1_MachineSet(src, dst, nil); err != nil {
		return err
	}

	dst.Spec.MinReadySeconds = ptr.Deref(src.Spec.Template.Spec.MinReadySeconds, 0)

	return nil
}

func (src *MachineDeployment) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*clusterv1.MachineDeployment)

	if err := Convert_v1beta1_MachineDeployment_To_v1beta2_MachineDeployment(src, dst, nil); err != nil {
		return err
	}

	dst.Spec.Template.Spec.MinReadySeconds = src.Spec.MinReadySeconds

	return nil
}

func (dst *MachineDeployment) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.MachineDeployment)

	if err := Convert_v1beta2_MachineDeployment_To_v1beta1_MachineDeployment(src, dst, nil); err != nil {
		return err
	}

	dst.Spec.MinReadySeconds = src.Spec.Template.Spec.MinReadySeconds

	return nil
}

func (src *MachineHealthCheck) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*clusterv1.MachineHealthCheck)

	return Convert_v1beta1_MachineHealthCheck_To_v1beta2_MachineHealthCheck(src, dst, nil)
}

func (dst *MachineHealthCheck) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.MachineHealthCheck)

	return Convert_v1beta2_MachineHealthCheck_To_v1beta1_MachineHealthCheck(src, dst, nil)
}

func (src *MachinePool) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*clusterv1.MachinePool)

	if err := Convert_v1beta1_MachinePool_To_v1beta2_MachinePool(src, dst, nil); err != nil {
		return err
	}

	dst.Spec.Template.Spec.MinReadySeconds = src.Spec.MinReadySeconds

	return nil
}

func (dst *MachinePool) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.MachinePool)

	if err := Convert_v1beta2_MachinePool_To_v1beta1_MachinePool(src, dst, nil); err != nil {
		return err
	}

	dst.Spec.MinReadySeconds = src.Spec.Template.Spec.MinReadySeconds

	return nil
}

func (src *MachineDrainRule) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*clusterv1.MachineDrainRule)
	return Convert_v1beta1_MachineDrainRule_To_v1beta2_MachineDrainRule(src, dst, nil)
}

func (dst *MachineDrainRule) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.MachineDrainRule)
	return Convert_v1beta2_MachineDrainRule_To_v1beta1_MachineDrainRule(src, dst, nil)
}

func Convert_v1beta2_ClusterClass_To_v1beta1_ClusterClass(in *clusterv1.ClusterClass, out *ClusterClass, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_ClusterClass_To_v1beta1_ClusterClass(in, out, s); err != nil {
		return err
	}

	if out.Spec.Infrastructure.Ref != nil {
		out.Spec.Infrastructure.Ref.Namespace = in.Namespace
	}
	if out.Spec.ControlPlane.Ref != nil {
		out.Spec.ControlPlane.Ref.Namespace = in.Namespace
	}
	if out.Spec.ControlPlane.MachineInfrastructure != nil && out.Spec.ControlPlane.MachineInfrastructure.Ref != nil {
		out.Spec.ControlPlane.MachineInfrastructure.Ref.Namespace = in.Namespace
	}
	for _, md := range out.Spec.Workers.MachineDeployments {
		if md.Template.Bootstrap.Ref != nil {
			md.Template.Bootstrap.Ref.Namespace = in.Namespace
		}
		if md.Template.Infrastructure.Ref != nil {
			md.Template.Infrastructure.Ref.Namespace = in.Namespace
		}
	}
	for _, mp := range out.Spec.Workers.MachinePools {
		if mp.Template.Bootstrap.Ref != nil {
			mp.Template.Bootstrap.Ref.Namespace = in.Namespace
		}
		if mp.Template.Infrastructure.Ref != nil {
			mp.Template.Infrastructure.Ref.Namespace = in.Namespace
		}
	}
	return nil
}

func Convert_v1beta2_ClusterClassSpec_To_v1beta1_ClusterClassSpec(in *clusterv1.ClusterClassSpec, out *ClusterClassSpec, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_ClusterClassSpec_To_v1beta1_ClusterClassSpec(in, out, s); err != nil {
		return err
	}

	if in.Infrastructure.NamingStrategy != nil {
		out.InfrastructureNamingStrategy = &InfrastructureNamingStrategy{
			Template: in.Infrastructure.NamingStrategy.Template,
		}
	}
	return nil
}

func Convert_v1beta2_InfrastructureClass_To_v1beta1_LocalObjectTemplate(in *clusterv1.InfrastructureClass, out *LocalObjectTemplate, s apimachineryconversion.Scope) error {
	if in == nil {
		return nil
	}

	return Convert_v1beta2_ClusterClassTemplate_To_v1beta1_LocalObjectTemplate(&in.ClusterClassTemplate, out, s)
}

func Convert_v1beta1_ClusterClassSpec_To_v1beta2_ClusterClassSpec(in *ClusterClassSpec, out *clusterv1.ClusterClassSpec, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_ClusterClassSpec_To_v1beta2_ClusterClassSpec(in, out, s); err != nil {
		return err
	}

	if in.InfrastructureNamingStrategy != nil {
		out.Infrastructure.NamingStrategy = &clusterv1.InfrastructureClassNamingStrategy{
			Template: in.InfrastructureNamingStrategy.Template,
		}
	}
	return nil
}

func Convert_v1beta1_MachineHealthCheckClass_To_v1beta2_MachineHealthCheckClass(in *MachineHealthCheckClass, out *clusterv1.MachineHealthCheckClass, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_MachineHealthCheckClass_To_v1beta2_MachineHealthCheckClass(in, out, s); err != nil {
		return err
	}

	for _, c := range in.UnhealthyConditions {
		out.UnhealthyNodeConditions = append(out.UnhealthyNodeConditions, clusterv1.UnhealthyNodeCondition{
			Type:           c.Type,
			Status:         c.Status,
			TimeoutSeconds: ptr.Deref(clusterv1.ConvertToSeconds(&c.Timeout), 0),
		})
	}
	out.NodeStartupTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeStartupTimeout)

	return nil
}

func Convert_v1beta2_MachineHealthCheckClass_To_v1beta1_MachineHealthCheckClass(in *clusterv1.MachineHealthCheckClass, out *MachineHealthCheckClass, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_MachineHealthCheckClass_To_v1beta1_MachineHealthCheckClass(in, out, s); err != nil {
		return err
	}

	for _, c := range in.UnhealthyNodeConditions {
		out.UnhealthyConditions = append(out.UnhealthyConditions, UnhealthyCondition{
			Type:    c.Type,
			Status:  c.Status,
			Timeout: ptr.Deref(clusterv1.ConvertFromSeconds(&c.TimeoutSeconds), metav1.Duration{}),
		})
	}
	out.NodeStartupTimeout = clusterv1.ConvertFromSeconds(in.NodeStartupTimeoutSeconds)

	return nil
}

func Convert_v1beta1_LocalObjectTemplate_To_v1beta2_InfrastructureClass(in *LocalObjectTemplate, out *clusterv1.InfrastructureClass, s apimachineryconversion.Scope) error {
	if in == nil {
		return nil
	}

	return Convert_v1beta1_LocalObjectTemplate_To_v1beta2_ClusterClassTemplate(in, &out.ClusterClassTemplate, s)
}

func Convert_v1beta1_ControlPlaneClass_To_v1beta2_ControlPlaneClass(in *ControlPlaneClass, out *clusterv1.ControlPlaneClass, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_ControlPlaneClass_To_v1beta2_ControlPlaneClass(in, out, s); err != nil {
		return err
	}
	out.NodeDrainTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeDrainTimeout)
	out.NodeVolumeDetachTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeVolumeDetachTimeout)
	out.NodeDeletionTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeDeletionTimeout)
	return Convert_v1beta1_LocalObjectTemplate_To_v1beta2_ClusterClassTemplate(&in.LocalObjectTemplate, &out.ClusterClassTemplate, s)
}

func Convert_v1beta2_ControlPlaneClass_To_v1beta1_ControlPlaneClass(in *clusterv1.ControlPlaneClass, out *ControlPlaneClass, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_ControlPlaneClass_To_v1beta1_ControlPlaneClass(in, out, s); err != nil {
		return err
	}
	out.NodeDrainTimeout = clusterv1.ConvertFromSeconds(in.NodeDrainTimeoutSeconds)
	out.NodeVolumeDetachTimeout = clusterv1.ConvertFromSeconds(in.NodeVolumeDetachTimeoutSeconds)
	out.NodeDeletionTimeout = clusterv1.ConvertFromSeconds(in.NodeDeletionTimeoutSeconds)
	return Convert_v1beta2_ClusterClassTemplate_To_v1beta1_LocalObjectTemplate(&in.ClusterClassTemplate, &out.LocalObjectTemplate, s)
}

func Convert_v1beta1_ControlPlaneTopology_To_v1beta2_ControlPlaneTopology(in *ControlPlaneTopology, out *clusterv1.ControlPlaneTopology, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_ControlPlaneTopology_To_v1beta2_ControlPlaneTopology(in, out, s); err != nil {
		return err
	}
	out.NodeDrainTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeDrainTimeout)
	out.NodeVolumeDetachTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeVolumeDetachTimeout)
	out.NodeDeletionTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeDeletionTimeout)
	return nil
}

func Convert_v1beta2_ControlPlaneTopology_To_v1beta1_ControlPlaneTopology(in *clusterv1.ControlPlaneTopology, out *ControlPlaneTopology, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_ControlPlaneTopology_To_v1beta1_ControlPlaneTopology(in, out, s); err != nil {
		return err
	}
	out.NodeDrainTimeout = clusterv1.ConvertFromSeconds(in.NodeDrainTimeoutSeconds)
	out.NodeVolumeDetachTimeout = clusterv1.ConvertFromSeconds(in.NodeVolumeDetachTimeoutSeconds)
	out.NodeDeletionTimeout = clusterv1.ConvertFromSeconds(in.NodeDeletionTimeoutSeconds)
	return nil
}

func Convert_v1beta1_MachineDeploymentClass_To_v1beta2_MachineDeploymentClass(in *MachineDeploymentClass, out *clusterv1.MachineDeploymentClass, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_MachineDeploymentClass_To_v1beta2_MachineDeploymentClass(in, out, s); err != nil {
		return err
	}
	out.NodeDrainTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeDrainTimeout)
	out.NodeVolumeDetachTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeVolumeDetachTimeout)
	out.NodeDeletionTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeDeletionTimeout)
	return nil
}

func Convert_v1beta2_MachineDeploymentClass_To_v1beta1_MachineDeploymentClass(in *clusterv1.MachineDeploymentClass, out *MachineDeploymentClass, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_MachineDeploymentClass_To_v1beta1_MachineDeploymentClass(in, out, s); err != nil {
		return err
	}
	out.NodeDrainTimeout = clusterv1.ConvertFromSeconds(in.NodeDrainTimeoutSeconds)
	out.NodeVolumeDetachTimeout = clusterv1.ConvertFromSeconds(in.NodeVolumeDetachTimeoutSeconds)
	out.NodeDeletionTimeout = clusterv1.ConvertFromSeconds(in.NodeDeletionTimeoutSeconds)
	return nil
}

func Convert_v1beta1_MachineDeploymentTopology_To_v1beta2_MachineDeploymentTopology(in *MachineDeploymentTopology, out *clusterv1.MachineDeploymentTopology, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_MachineDeploymentTopology_To_v1beta2_MachineDeploymentTopology(in, out, s); err != nil {
		return err
	}
	out.NodeDrainTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeDrainTimeout)
	out.NodeVolumeDetachTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeVolumeDetachTimeout)
	out.NodeDeletionTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeDeletionTimeout)
	return nil
}

func Convert_v1beta2_MachineDeploymentTopology_To_v1beta1_MachineDeploymentTopology(in *clusterv1.MachineDeploymentTopology, out *MachineDeploymentTopology, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_MachineDeploymentTopology_To_v1beta1_MachineDeploymentTopology(in, out, s); err != nil {
		return err
	}
	out.NodeDrainTimeout = clusterv1.ConvertFromSeconds(in.NodeDrainTimeoutSeconds)
	out.NodeVolumeDetachTimeout = clusterv1.ConvertFromSeconds(in.NodeVolumeDetachTimeoutSeconds)
	out.NodeDeletionTimeout = clusterv1.ConvertFromSeconds(in.NodeDeletionTimeoutSeconds)
	return nil
}

func Convert_v1beta1_MachinePoolClass_To_v1beta2_MachinePoolClass(in *MachinePoolClass, out *clusterv1.MachinePoolClass, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_MachinePoolClass_To_v1beta2_MachinePoolClass(in, out, s); err != nil {
		return err
	}
	out.NodeDrainTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeDrainTimeout)
	out.NodeVolumeDetachTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeVolumeDetachTimeout)
	out.NodeDeletionTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeDeletionTimeout)
	return nil
}

func Convert_v1beta2_MachinePoolClass_To_v1beta1_MachinePoolClass(in *clusterv1.MachinePoolClass, out *MachinePoolClass, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_MachinePoolClass_To_v1beta1_MachinePoolClass(in, out, s); err != nil {
		return err
	}
	out.NodeDrainTimeout = clusterv1.ConvertFromSeconds(in.NodeDrainTimeoutSeconds)
	out.NodeVolumeDetachTimeout = clusterv1.ConvertFromSeconds(in.NodeVolumeDetachTimeoutSeconds)
	out.NodeDeletionTimeout = clusterv1.ConvertFromSeconds(in.NodeDeletionTimeoutSeconds)
	return nil
}

func Convert_v1beta1_MachinePoolTopology_To_v1beta2_MachinePoolTopology(in *MachinePoolTopology, out *clusterv1.MachinePoolTopology, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_MachinePoolTopology_To_v1beta2_MachinePoolTopology(in, out, s); err != nil {
		return err
	}
	out.NodeDrainTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeDrainTimeout)
	out.NodeVolumeDetachTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeVolumeDetachTimeout)
	out.NodeDeletionTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeDeletionTimeout)
	return nil
}

func Convert_v1beta2_MachinePoolTopology_To_v1beta1_MachinePoolTopology(in *clusterv1.MachinePoolTopology, out *MachinePoolTopology, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_MachinePoolTopology_To_v1beta1_MachinePoolTopology(in, out, s); err != nil {
		return err
	}
	out.NodeDrainTimeout = clusterv1.ConvertFromSeconds(in.NodeDrainTimeoutSeconds)
	out.NodeVolumeDetachTimeout = clusterv1.ConvertFromSeconds(in.NodeVolumeDetachTimeoutSeconds)
	out.NodeDeletionTimeout = clusterv1.ConvertFromSeconds(in.NodeDeletionTimeoutSeconds)
	return nil
}

func Convert_v1beta1_MachineSpec_To_v1beta2_MachineSpec(in *MachineSpec, out *clusterv1.MachineSpec, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_MachineSpec_To_v1beta2_MachineSpec(in, out, s); err != nil {
		return err
	}
	out.NodeDrainTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeDrainTimeout)
	out.NodeVolumeDetachTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeVolumeDetachTimeout)
	out.NodeDeletionTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeDeletionTimeout)
	return nil
}

func Convert_v1beta2_ClusterClassStatus_To_v1beta1_ClusterClassStatus(in *clusterv1.ClusterClassStatus, out *ClusterClassStatus, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_ClusterClassStatus_To_v1beta1_ClusterClassStatus(in, out, s); err != nil {
		return err
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta2 conditions should not be automatically be converted into legacy conditions (v1beta1).
	out.Conditions = nil

	// Retrieve legacy conditions (v1beta1) from the deprecated field.
	if in.Deprecated != nil && in.Deprecated.V1Beta1 != nil {
		if in.Deprecated.V1Beta1.Conditions != nil {
			Convert_v1beta2_Deprecated_V1Beta1_Conditions_To_v1beta1_Conditions(&in.Deprecated.V1Beta1.Conditions, &out.Conditions)
		}
	}

	// Move new conditions (v1beta2) to the v1beta2 field.
	if in.Conditions == nil {
		return nil
	}
	out.V1Beta2 = &ClusterClassV1Beta2Status{}
	out.V1Beta2.Conditions = in.Conditions
	return nil
}

func Convert_v1beta1_ClusterClassStatus_To_v1beta2_ClusterClassStatus(in *ClusterClassStatus, out *clusterv1.ClusterClassStatus, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_ClusterClassStatus_To_v1beta2_ClusterClassStatus(in, out, s); err != nil {
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
		out.Deprecated = &clusterv1.ClusterClassDeprecatedStatus{}
	}
	if out.Deprecated.V1Beta1 == nil {
		out.Deprecated.V1Beta1 = &clusterv1.ClusterClassV1Beta1DeprecatedStatus{}
	}
	if in.Conditions != nil {
		Convert_v1beta1_Conditions_To_v1beta2_Deprecated_V1Beta1_Conditions(&in.Conditions, &out.Deprecated.V1Beta1.Conditions)
	}
	return nil
}

func Convert_v1beta2_ClusterStatus_To_v1beta1_ClusterStatus(in *clusterv1.ClusterStatus, out *ClusterStatus, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_ClusterStatus_To_v1beta1_ClusterStatus(in, out, s); err != nil {
		return err
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta2 conditions should not be automatically be converted into legacy conditions (v1beta1).
	out.Conditions = nil

	// Retrieve legacy conditions (v1beta1) from the deprecated field.
	if in.Deprecated != nil && in.Deprecated.V1Beta1 != nil {
		if in.Deprecated.V1Beta1.Conditions != nil {
			Convert_v1beta2_Deprecated_V1Beta1_Conditions_To_v1beta1_Conditions(&in.Deprecated.V1Beta1.Conditions, &out.Conditions)
		}
		out.FailureReason = in.Deprecated.V1Beta1.FailureReason
		out.FailureMessage = in.Deprecated.V1Beta1.FailureMessage
	}

	// Move initialization to old fields
	if in.Initialization != nil {
		out.ControlPlaneReady = in.Initialization.ControlPlaneInitialized
		out.InfrastructureReady = in.Initialization.InfrastructureProvisioned
	}

	// Move FailureDomains
	if in.FailureDomains != nil {
		out.FailureDomains = FailureDomains{}
		for _, fd := range in.FailureDomains {
			out.FailureDomains[fd.Name] = FailureDomainSpec{
				ControlPlane: fd.ControlPlane,
				Attributes:   fd.Attributes,
			}
		}
	}

	// Move new conditions (v1beta2), controlPlane and workers counters to the v1beta2 field.
	if in.Conditions == nil && in.ControlPlane == nil && in.Workers == nil {
		return nil
	}
	out.V1Beta2 = &ClusterV1Beta2Status{}
	out.V1Beta2.Conditions = in.Conditions
	if in.ControlPlane != nil {
		out.V1Beta2.ControlPlane = &ClusterControlPlaneStatus{
			DesiredReplicas:   in.ControlPlane.DesiredReplicas,
			Replicas:          in.ControlPlane.Replicas,
			UpToDateReplicas:  in.ControlPlane.UpToDateReplicas,
			ReadyReplicas:     in.ControlPlane.ReadyReplicas,
			AvailableReplicas: in.ControlPlane.AvailableReplicas,
		}
	}
	if in.Workers != nil {
		out.V1Beta2.Workers = &WorkersStatus{
			DesiredReplicas:   in.Workers.DesiredReplicas,
			Replicas:          in.Workers.Replicas,
			UpToDateReplicas:  in.Workers.UpToDateReplicas,
			ReadyReplicas:     in.Workers.ReadyReplicas,
			AvailableReplicas: in.Workers.AvailableReplicas,
		}
	}
	return nil
}

func Convert_v1beta1_Topology_To_v1beta2_Topology(in *Topology, out *clusterv1.Topology, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_Topology_To_v1beta2_Topology(in, out, s); err != nil {
		return err
	}

	out.ClassRef.Name = in.Class
	out.ClassRef.Namespace = in.ClassNamespace
	return nil
}

func Convert_v1beta1_ClusterStatus_To_v1beta2_ClusterStatus(in *ClusterStatus, out *clusterv1.ClusterStatus, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_ClusterStatus_To_v1beta2_ClusterStatus(in, out, s); err != nil {
		return err
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta1 conditions should not be automatically be converted into v1beta2 conditions.
	out.Conditions = nil

	// Retrieve new conditions (v1beta2), controlPlane and workers counters from the v1beta2 field.
	if in.V1Beta2 != nil {
		out.Conditions = in.V1Beta2.Conditions
		if in.V1Beta2.ControlPlane != nil {
			out.ControlPlane = &clusterv1.ClusterControlPlaneStatus{
				DesiredReplicas:   in.V1Beta2.ControlPlane.DesiredReplicas,
				Replicas:          in.V1Beta2.ControlPlane.Replicas,
				UpToDateReplicas:  in.V1Beta2.ControlPlane.UpToDateReplicas,
				ReadyReplicas:     in.V1Beta2.ControlPlane.ReadyReplicas,
				AvailableReplicas: in.V1Beta2.ControlPlane.AvailableReplicas,
			}
		}
		if in.V1Beta2.Workers != nil {
			out.Workers = &clusterv1.WorkersStatus{
				DesiredReplicas:   in.V1Beta2.Workers.DesiredReplicas,
				Replicas:          in.V1Beta2.Workers.Replicas,
				UpToDateReplicas:  in.V1Beta2.Workers.UpToDateReplicas,
				ReadyReplicas:     in.V1Beta2.Workers.ReadyReplicas,
				AvailableReplicas: in.V1Beta2.Workers.AvailableReplicas,
			}
		}
	}

	// Move ControlPlaneReady and InfrastructureReady to Initialization
	if in.ControlPlaneReady || in.InfrastructureReady {
		if out.Initialization == nil {
			out.Initialization = &clusterv1.ClusterInitializationStatus{}
		}
		out.Initialization.ControlPlaneInitialized = in.ControlPlaneReady
		out.Initialization.InfrastructureProvisioned = in.InfrastructureReady
	}

	// Move FailureDomains
	if in.FailureDomains != nil {
		out.FailureDomains = []clusterv1.FailureDomain{}
		for name, fd := range in.FailureDomains {
			out.FailureDomains = append(out.FailureDomains, clusterv1.FailureDomain{
				Name:         name,
				ControlPlane: fd.ControlPlane,
				Attributes:   fd.Attributes,
			})
		}
	}

	// Move legacy conditions (v1beta1), FailureReason and FailureMessage to the deprecated field.
	if in.Conditions == nil && in.FailureReason == nil && in.FailureMessage == nil {
		return nil
	}

	if out.Deprecated == nil {
		out.Deprecated = &clusterv1.ClusterDeprecatedStatus{}
	}
	if out.Deprecated.V1Beta1 == nil {
		out.Deprecated.V1Beta1 = &clusterv1.ClusterV1Beta1DeprecatedStatus{}
	}
	if in.Conditions != nil {
		Convert_v1beta1_Conditions_To_v1beta2_Deprecated_V1Beta1_Conditions(&in.Conditions, &out.Deprecated.V1Beta1.Conditions)
	}
	out.Deprecated.V1Beta1.FailureReason = in.FailureReason
	out.Deprecated.V1Beta1.FailureMessage = in.FailureMessage
	return nil
}

func Convert_v1beta2_Topology_To_v1beta1_Topology(in *clusterv1.Topology, out *Topology, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_Topology_To_v1beta1_Topology(in, out, s); err != nil {
		return err
	}

	out.Class = in.ClassRef.Name
	out.ClassNamespace = in.ClassRef.Namespace
	return nil
}

func Convert_v1beta1_MachineDeploymentSpec_To_v1beta2_MachineDeploymentSpec(in *MachineDeploymentSpec, out *clusterv1.MachineDeploymentSpec, s apimachineryconversion.Scope) error {
	// NOTE: v1beta2 MachineDeploymentSpec does not have ProgressDeadlineSeconds anymore. But it's fine to just lose this field it was never used.
	return autoConvert_v1beta1_MachineDeploymentSpec_To_v1beta2_MachineDeploymentSpec(in, out, s)
}

func Convert_v1beta2_MachineDeploymentStatus_To_v1beta1_MachineDeploymentStatus(in *clusterv1.MachineDeploymentStatus, out *MachineDeploymentStatus, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_MachineDeploymentStatus_To_v1beta1_MachineDeploymentStatus(in, out, s); err != nil {
		return err
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta2 conditions should not be automatically be converted into legacy conditions (v1beta1).
	out.Conditions = nil

	// Reset replica counters from autogenerated conversions
	// NOTE: replica counters with a new semantic should not be automatically be converted into old replica counters.
	out.AvailableReplicas = 0
	out.ReadyReplicas = 0

	// Retrieve legacy conditions (v1beta1) and replica counters from the deprecated field.
	if in.Deprecated != nil && in.Deprecated.V1Beta1 != nil {
		if in.Deprecated.V1Beta1.Conditions != nil {
			Convert_v1beta2_Deprecated_V1Beta1_Conditions_To_v1beta1_Conditions(&in.Deprecated.V1Beta1.Conditions, &out.Conditions)
		}
		out.AvailableReplicas = in.Deprecated.V1Beta1.AvailableReplicas
		out.UnavailableReplicas = in.Deprecated.V1Beta1.UnavailableReplicas
		out.UpdatedReplicas = in.Deprecated.V1Beta1.UpdatedReplicas
		out.ReadyReplicas = in.Deprecated.V1Beta1.ReadyReplicas
	}

	// Move new conditions (v1beta2) and replica counters to the v1beta2 field.
	if in.Conditions == nil && in.ReadyReplicas == nil && in.AvailableReplicas == nil && in.UpToDateReplicas == nil {
		return nil
	}
	out.V1Beta2 = &MachineDeploymentV1Beta2Status{}
	out.V1Beta2.Conditions = in.Conditions
	out.V1Beta2.ReadyReplicas = in.ReadyReplicas
	out.V1Beta2.AvailableReplicas = in.AvailableReplicas
	out.V1Beta2.UpToDateReplicas = in.UpToDateReplicas
	return nil
}

func Convert_v1beta1_MachineDeploymentStatus_To_v1beta2_MachineDeploymentStatus(in *MachineDeploymentStatus, out *clusterv1.MachineDeploymentStatus, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_MachineDeploymentStatus_To_v1beta2_MachineDeploymentStatus(in, out, s); err != nil {
		return err
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta1 conditions should not be automatically be converted into v1beta2 conditions.
	out.Conditions = nil

	// Reset replica counters from autogenerated conversions
	// NOTE: old replica counters should not be automatically be converted into replica counters with a new semantic.
	out.AvailableReplicas = nil
	out.ReadyReplicas = nil

	// Retrieve new conditions (v1beta2) and replica counters from the v1beta2 field.
	if in.V1Beta2 != nil {
		out.Conditions = in.V1Beta2.Conditions
		out.ReadyReplicas = in.V1Beta2.ReadyReplicas
		out.AvailableReplicas = in.V1Beta2.AvailableReplicas
		out.UpToDateReplicas = in.V1Beta2.UpToDateReplicas
	}

	// Move legacy conditions (v1beta1) and replica counters to the deprecated field.
	if out.Deprecated == nil {
		out.Deprecated = &clusterv1.MachineDeploymentDeprecatedStatus{}
	}
	if out.Deprecated.V1Beta1 == nil {
		out.Deprecated.V1Beta1 = &clusterv1.MachineDeploymentV1Beta1DeprecatedStatus{}
	}
	if in.Conditions != nil {
		Convert_v1beta1_Conditions_To_v1beta2_Deprecated_V1Beta1_Conditions(&in.Conditions, &out.Deprecated.V1Beta1.Conditions)
	}
	out.Deprecated.V1Beta1.AvailableReplicas = in.AvailableReplicas
	out.Deprecated.V1Beta1.UnavailableReplicas = in.UnavailableReplicas
	out.Deprecated.V1Beta1.UpdatedReplicas = in.UpdatedReplicas
	out.Deprecated.V1Beta1.ReadyReplicas = in.ReadyReplicas
	return nil
}

func Convert_v1beta1_MachineHealthCheckSpec_To_v1beta2_MachineHealthCheckSpec(in *MachineHealthCheckSpec, out *clusterv1.MachineHealthCheckSpec, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_MachineHealthCheckSpec_To_v1beta2_MachineHealthCheckSpec(in, out, s); err != nil {
		return err
	}

	for _, c := range in.UnhealthyConditions {
		out.UnhealthyNodeConditions = append(out.UnhealthyNodeConditions, clusterv1.UnhealthyNodeCondition{
			Type:           c.Type,
			Status:         c.Status,
			TimeoutSeconds: ptr.Deref(clusterv1.ConvertToSeconds(&c.Timeout), 0),
		})
	}
	out.NodeStartupTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeStartupTimeout)

	return nil
}

func Convert_v1beta2_MachineHealthCheckSpec_To_v1beta1_MachineHealthCheckSpec(in *clusterv1.MachineHealthCheckSpec, out *MachineHealthCheckSpec, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_MachineHealthCheckSpec_To_v1beta1_MachineHealthCheckSpec(in, out, s); err != nil {
		return err
	}

	for _, c := range in.UnhealthyNodeConditions {
		out.UnhealthyConditions = append(out.UnhealthyConditions, UnhealthyCondition{
			Type:    c.Type,
			Status:  c.Status,
			Timeout: ptr.Deref(clusterv1.ConvertFromSeconds(&c.TimeoutSeconds), metav1.Duration{}),
		})
	}
	out.NodeStartupTimeout = clusterv1.ConvertFromSeconds(in.NodeStartupTimeoutSeconds)

	return nil
}

func Convert_v1beta2_MachineHealthCheckStatus_To_v1beta1_MachineHealthCheckStatus(in *clusterv1.MachineHealthCheckStatus, out *MachineHealthCheckStatus, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_MachineHealthCheckStatus_To_v1beta1_MachineHealthCheckStatus(in, out, s); err != nil {
		return err
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta2 conditions should not be automatically be converted into legacy conditions (v1beta1).
	out.Conditions = nil

	// Retrieve legacy conditions (v1beta1) from the deprecated field.
	if in.Deprecated != nil && in.Deprecated.V1Beta1 != nil {
		if in.Deprecated.V1Beta1.Conditions != nil {
			Convert_v1beta2_Deprecated_V1Beta1_Conditions_To_v1beta1_Conditions(&in.Deprecated.V1Beta1.Conditions, &out.Conditions)
		}
	}

	// Move new conditions (v1beta2) to the v1beta2 field.
	if in.Conditions == nil {
		return nil
	}
	out.V1Beta2 = &MachineHealthCheckV1Beta2Status{}
	out.V1Beta2.Conditions = in.Conditions
	return nil
}

func Convert_v1beta1_MachineHealthCheckStatus_To_v1beta2_MachineHealthCheckStatus(in *MachineHealthCheckStatus, out *clusterv1.MachineHealthCheckStatus, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_MachineHealthCheckStatus_To_v1beta2_MachineHealthCheckStatus(in, out, s); err != nil {
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
		out.Deprecated = &clusterv1.MachineHealthCheckDeprecatedStatus{}
	}
	if out.Deprecated.V1Beta1 == nil {
		out.Deprecated.V1Beta1 = &clusterv1.MachineHealthCheckV1Beta1DeprecatedStatus{}
	}
	if in.Conditions != nil {
		Convert_v1beta1_Conditions_To_v1beta2_Deprecated_V1Beta1_Conditions(&in.Conditions, &out.Deprecated.V1Beta1.Conditions)
	}
	return nil
}

func Convert_v1beta2_MachineSetStatus_To_v1beta1_MachineSetStatus(in *clusterv1.MachineSetStatus, out *MachineSetStatus, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_MachineSetStatus_To_v1beta1_MachineSetStatus(in, out, s); err != nil {
		return err
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta2 conditions should not be automatically be converted into legacy conditions (v1beta1).
	out.Conditions = nil

	// Reset replica counters from autogenerated conversions
	// NOTE: replica counters with a new semantic should not be automatically be converted into old replica counters.
	out.AvailableReplicas = 0
	out.ReadyReplicas = 0

	// Retrieve legacy conditions (v1beta1), failureReason, failureMessage, and replica counters from the deprecated field.
	if in.Deprecated != nil && in.Deprecated.V1Beta1 != nil {
		if in.Deprecated.V1Beta1.Conditions != nil {
			Convert_v1beta2_Deprecated_V1Beta1_Conditions_To_v1beta1_Conditions(&in.Deprecated.V1Beta1.Conditions, &out.Conditions)
		}
		out.AvailableReplicas = in.Deprecated.V1Beta1.AvailableReplicas
		out.FullyLabeledReplicas = in.Deprecated.V1Beta1.FullyLabeledReplicas
		out.ReadyReplicas = in.Deprecated.V1Beta1.ReadyReplicas
		out.FailureReason = in.Deprecated.V1Beta1.FailureReason
		out.FailureMessage = in.Deprecated.V1Beta1.FailureMessage
	}

	// Move new conditions (v1beta2) and replica counters to the v1beta2 field.
	if in.Conditions == nil && in.ReadyReplicas == nil && in.AvailableReplicas == nil && in.UpToDateReplicas == nil {
		return nil
	}
	out.V1Beta2 = &MachineSetV1Beta2Status{}
	out.V1Beta2.Conditions = in.Conditions
	out.V1Beta2.ReadyReplicas = in.ReadyReplicas
	out.V1Beta2.AvailableReplicas = in.AvailableReplicas
	out.V1Beta2.UpToDateReplicas = in.UpToDateReplicas
	return nil
}

func Convert_v1beta1_MachineSetStatus_To_v1beta2_MachineSetStatus(in *MachineSetStatus, out *clusterv1.MachineSetStatus, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_MachineSetStatus_To_v1beta2_MachineSetStatus(in, out, s); err != nil {
		return err
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta1 conditions should not be automatically be converted into v1beta2 conditions.
	out.Conditions = nil

	// Reset replica counters from autogenerated conversions
	// NOTE: old replica counters should not be automatically be converted into replica counters with a new semantic.
	out.AvailableReplicas = nil
	out.ReadyReplicas = nil

	// Retrieve new conditions (v1beta2) and replica counters from the v1beta2 field.
	if in.V1Beta2 != nil {
		out.Conditions = in.V1Beta2.Conditions
		out.ReadyReplicas = in.V1Beta2.ReadyReplicas
		out.AvailableReplicas = in.V1Beta2.AvailableReplicas
		out.UpToDateReplicas = in.V1Beta2.UpToDateReplicas
	}

	// Move legacy conditions (v1beta1), failureReason, failureMessage, and replica counters to the deprecated field.
	if out.Deprecated == nil {
		out.Deprecated = &clusterv1.MachineSetDeprecatedStatus{}
	}
	if out.Deprecated.V1Beta1 == nil {
		out.Deprecated.V1Beta1 = &clusterv1.MachineSetV1Beta1DeprecatedStatus{}
	}
	if in.Conditions != nil {
		Convert_v1beta1_Conditions_To_v1beta2_Deprecated_V1Beta1_Conditions(&in.Conditions, &out.Deprecated.V1Beta1.Conditions)
	}
	out.Deprecated.V1Beta1.AvailableReplicas = in.AvailableReplicas
	out.Deprecated.V1Beta1.FullyLabeledReplicas = in.FullyLabeledReplicas
	out.Deprecated.V1Beta1.ReadyReplicas = in.ReadyReplicas
	out.Deprecated.V1Beta1.FailureReason = in.FailureReason
	out.Deprecated.V1Beta1.FailureMessage = in.FailureMessage
	return nil
}

func Convert_v1beta2_MachineStatus_To_v1beta1_MachineStatus(in *clusterv1.MachineStatus, out *MachineStatus, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_MachineStatus_To_v1beta1_MachineStatus(in, out, s); err != nil {
		return err
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta2 conditions should not be automatically be converted into legacy conditions (v1beta1).
	out.Conditions = nil

	// Retrieve legacy conditions (v1beta1), failureReason and failureMessage from the deprecated field.
	if in.Deprecated != nil && in.Deprecated.V1Beta1 != nil {
		if in.Deprecated.V1Beta1.Conditions != nil {
			Convert_v1beta2_Deprecated_V1Beta1_Conditions_To_v1beta1_Conditions(&in.Deprecated.V1Beta1.Conditions, &out.Conditions)
		}
		out.FailureReason = in.Deprecated.V1Beta1.FailureReason
		out.FailureMessage = in.Deprecated.V1Beta1.FailureMessage
	}

	// Move initialization to old fields
	if in.Initialization != nil {
		out.BootstrapReady = in.Initialization.BootstrapDataSecretCreated
		out.InfrastructureReady = in.Initialization.InfrastructureProvisioned
	}

	// Move new conditions (v1beta2) to the v1beta2 field.
	if in.Conditions == nil {
		return nil
	}
	out.V1Beta2 = &MachineV1Beta2Status{}
	out.V1Beta2.Conditions = in.Conditions
	return nil
}

func Convert_v1beta1_MachineStatus_To_v1beta2_MachineStatus(in *MachineStatus, out *clusterv1.MachineStatus, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_MachineStatus_To_v1beta2_MachineStatus(in, out, s); err != nil {
		return err
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta1 conditions should not be automatically be converted into v1beta2 conditions.
	out.Conditions = nil

	// Retrieve new conditions (v1beta2) from the v1beta2 field.
	if in.V1Beta2 != nil {
		out.Conditions = in.V1Beta2.Conditions
	}

	// Move BootstrapReady and InfrastructureReady to Initialization
	if in.BootstrapReady || in.InfrastructureReady {
		if out.Initialization == nil {
			out.Initialization = &clusterv1.MachineInitializationStatus{}
		}
		out.Initialization.BootstrapDataSecretCreated = in.BootstrapReady
		out.Initialization.InfrastructureProvisioned = in.InfrastructureReady
	}

	// Move legacy conditions (v1beta1), failureReason and failureMessage to the deprecated field.
	if in.Conditions == nil && in.FailureReason == nil && in.FailureMessage == nil {
		return nil
	}

	if out.Deprecated == nil {
		out.Deprecated = &clusterv1.MachineDeprecatedStatus{}
	}
	if out.Deprecated.V1Beta1 == nil {
		out.Deprecated.V1Beta1 = &clusterv1.MachineV1Beta1DeprecatedStatus{}
	}
	if in.Conditions != nil {
		Convert_v1beta1_Conditions_To_v1beta2_Deprecated_V1Beta1_Conditions(&in.Conditions, &out.Deprecated.V1Beta1.Conditions)
	}
	out.Deprecated.V1Beta1.FailureReason = in.FailureReason
	out.Deprecated.V1Beta1.FailureMessage = in.FailureMessage
	return nil
}

func Convert_v1beta2_Deprecated_V1Beta1_Conditions_To_v1beta1_Conditions(in *clusterv1.Conditions, out *Conditions) {
	*out = make(Conditions, len(*in))
	for i := range *in {
		(*out)[i] = *(*Condition)(unsafe.Pointer(&(*in)[i])) //nolint:gosec
	}
}

func Convert_v1beta1_Conditions_To_v1beta2_Deprecated_V1Beta1_Conditions(in *Conditions, out *clusterv1.Conditions) {
	*out = make(clusterv1.Conditions, len(*in))
	for i := range *in {
		(*out)[i] = *(*clusterv1.Condition)(unsafe.Pointer(&(*in)[i])) //nolint:gosec
	}
}

func Convert_v1_Condition_To_v1beta1_Condition(_ *metav1.Condition, _ *Condition, _ apimachineryconversion.Scope) error {
	// NOTE: v1beta2 conditions should not be automatically converted into legacy (v1beta1) conditions.
	return nil
}

func Convert_v1beta1_Condition_To_v1_Condition(_ *Condition, _ *metav1.Condition, _ apimachineryconversion.Scope) error {
	// NOTE: legacy (v1beta1) conditions should not be automatically converted into v1beta2 conditions.
	return nil
}

func Convert_v1beta1_ClusterVariable_To_v1beta2_ClusterVariable(in *ClusterVariable, out *clusterv1.ClusterVariable, s apimachineryconversion.Scope) error {
	// NOTE: v1beta2 ClusterVariable does not have DefinitionFrom anymore. But it's fine to just lose this field,
	// because it was already not possible to set it anymore with v1beta1.
	return autoConvert_v1beta1_ClusterVariable_To_v1beta2_ClusterVariable(in, out, s)
}

func Convert_v1beta1_MachineSetSpec_To_v1beta2_MachineSetSpec(in *MachineSetSpec, out *clusterv1.MachineSetSpec, s apimachineryconversion.Scope) error {
	return autoConvert_v1beta1_MachineSetSpec_To_v1beta2_MachineSetSpec(in, out, s)
}

func Convert_v1beta2_MachineSpec_To_v1beta1_MachineSpec(in *clusterv1.MachineSpec, out *MachineSpec, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_MachineSpec_To_v1beta1_MachineSpec(in, out, s); err != nil {
		return err
	}
	out.NodeDrainTimeout = clusterv1.ConvertFromSeconds(in.NodeDrainTimeoutSeconds)
	out.NodeVolumeDetachTimeout = clusterv1.ConvertFromSeconds(in.NodeVolumeDetachTimeoutSeconds)
	out.NodeDeletionTimeout = clusterv1.ConvertFromSeconds(in.NodeDeletionTimeoutSeconds)
	return nil
}

func Convert_v1beta2_MachinePoolStatus_To_v1beta1_MachinePoolStatus(in *clusterv1.MachinePoolStatus, out *MachinePoolStatus, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_MachinePoolStatus_To_v1beta1_MachinePoolStatus(in, out, s); err != nil {
		return err
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta2 conditions should not be automatically be converted into legacy conditions (v1beta1).
	out.Conditions = nil

	// Reset replica counters from autogenerated conversions
	// NOTE: replica counters with a new semantic should not be automatically be converted into old replica counters.
	out.ReadyReplicas = 0
	out.AvailableReplicas = 0

	// Retrieve legacy conditions (v1beta1), failureReason, failureMessage and replica counters from the deprecated field.
	if in.Deprecated != nil && in.Deprecated.V1Beta1 != nil {
		if in.Deprecated.V1Beta1.Conditions != nil {
			Convert_v1beta2_Deprecated_V1Beta1_Conditions_To_v1beta1_Conditions(&in.Deprecated.V1Beta1.Conditions, &out.Conditions)
		}
		out.FailureReason = in.Deprecated.V1Beta1.FailureReason
		out.FailureMessage = in.Deprecated.V1Beta1.FailureMessage
		out.ReadyReplicas = in.Deprecated.V1Beta1.ReadyReplicas
		out.AvailableReplicas = in.Deprecated.V1Beta1.AvailableReplicas
		out.UnavailableReplicas = in.Deprecated.V1Beta1.UnavailableReplicas
	}

	// Move initialization to old fields
	if in.Initialization != nil {
		out.BootstrapReady = in.Initialization.BootstrapDataSecretCreated
		out.InfrastructureReady = in.Initialization.InfrastructureProvisioned
	}

	// Move new conditions (v1beta2) and replica counters to the v1beta2 field.
	if in.Conditions == nil && in.ReadyReplicas == nil && in.AvailableReplicas == nil && in.UpToDateReplicas == nil {
		return nil
	}
	out.V1Beta2 = &MachinePoolV1Beta2Status{}
	out.V1Beta2.Conditions = in.Conditions
	out.V1Beta2.ReadyReplicas = in.ReadyReplicas
	out.V1Beta2.AvailableReplicas = in.AvailableReplicas
	out.V1Beta2.UpToDateReplicas = in.UpToDateReplicas
	return nil
}

func Convert_v1beta1_MachinePoolStatus_To_v1beta2_MachinePoolStatus(in *MachinePoolStatus, out *clusterv1.MachinePoolStatus, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_MachinePoolStatus_To_v1beta2_MachinePoolStatus(in, out, s); err != nil {
		return err
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta1 conditions should not be automatically be converted into v1beta2 conditions.
	out.Conditions = nil

	// Reset replica counters from autogenerated conversions
	// NOTE: old replica counters should not be automatically be converted into replica counters with a new semantic.
	out.ReadyReplicas = nil
	out.AvailableReplicas = nil

	// Retrieve new conditions (v1beta2) and replica counters from the v1beta2 field.
	if in.V1Beta2 != nil {
		out.Conditions = in.V1Beta2.Conditions
		out.ReadyReplicas = in.V1Beta2.ReadyReplicas
		out.AvailableReplicas = in.V1Beta2.AvailableReplicas
		out.UpToDateReplicas = in.V1Beta2.UpToDateReplicas
	}

	// Move BootstrapReady and InfrastructureReady to Initialization
	if in.BootstrapReady || in.InfrastructureReady {
		if out.Initialization == nil {
			out.Initialization = &clusterv1.MachinePoolInitializationStatus{}
		}
		out.Initialization.BootstrapDataSecretCreated = in.BootstrapReady
		out.Initialization.InfrastructureProvisioned = in.InfrastructureReady
	}

	// Move legacy conditions (v1beta1), failureReason, failureMessage and replica counters to the deprecated field.
	if out.Deprecated == nil {
		out.Deprecated = &clusterv1.MachinePoolDeprecatedStatus{}
	}
	if out.Deprecated.V1Beta1 == nil {
		out.Deprecated.V1Beta1 = &clusterv1.MachinePoolV1Beta1DeprecatedStatus{}
	}
	if in.Conditions != nil {
		Convert_v1beta1_Conditions_To_v1beta2_Deprecated_V1Beta1_Conditions(&in.Conditions, &out.Deprecated.V1Beta1.Conditions)
	}
	out.Deprecated.V1Beta1.FailureReason = in.FailureReason
	out.Deprecated.V1Beta1.FailureMessage = in.FailureMessage
	out.Deprecated.V1Beta1.ReadyReplicas = in.ReadyReplicas
	out.Deprecated.V1Beta1.AvailableReplicas = in.AvailableReplicas
	out.Deprecated.V1Beta1.UnavailableReplicas = in.UnavailableReplicas
	return nil
}

func Convert_v1beta1_MachinePoolSpec_To_v1beta2_MachinePoolSpec(in *MachinePoolSpec, out *clusterv1.MachinePoolSpec, s apimachineryconversion.Scope) error {
	return autoConvert_v1beta1_MachinePoolSpec_To_v1beta2_MachinePoolSpec(in, out, s)
}

func Convert_v1beta1_ClusterClassStatusVariableDefinition_To_v1beta2_ClusterClassStatusVariableDefinition(in *ClusterClassStatusVariableDefinition, out *clusterv1.ClusterClassStatusVariableDefinition, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_ClusterClassStatusVariableDefinition_To_v1beta2_ClusterClassStatusVariableDefinition(in, out, s); err != nil {
		return err
	}
	return autoConvert_v1beta1_ClusterClassVariableMetadata_To_v1beta2_ClusterClassVariableMetadata(&in.Metadata, &out.DeprecatedV1Beta1Metadata, s)
}

func Convert_v1beta2_ClusterClassStatusVariableDefinition_To_v1beta1_ClusterClassStatusVariableDefinition(in *clusterv1.ClusterClassStatusVariableDefinition, out *ClusterClassStatusVariableDefinition, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_ClusterClassStatusVariableDefinition_To_v1beta1_ClusterClassStatusVariableDefinition(in, out, s); err != nil {
		return err
	}
	return autoConvert_v1beta2_ClusterClassVariableMetadata_To_v1beta1_ClusterClassVariableMetadata(&in.DeprecatedV1Beta1Metadata, &out.Metadata, s)
}

func Convert_v1beta1_ClusterClassVariable_To_v1beta2_ClusterClassVariable(in *ClusterClassVariable, out *clusterv1.ClusterClassVariable, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_ClusterClassVariable_To_v1beta2_ClusterClassVariable(in, out, s); err != nil {
		return err
	}
	return autoConvert_v1beta1_ClusterClassVariableMetadata_To_v1beta2_ClusterClassVariableMetadata(&in.Metadata, &out.DeprecatedV1Beta1Metadata, s)
}

func Convert_v1beta2_ClusterClassVariable_To_v1beta1_ClusterClassVariable(in *clusterv1.ClusterClassVariable, out *ClusterClassVariable, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_ClusterClassVariable_To_v1beta1_ClusterClassVariable(in, out, s); err != nil {
		return err
	}
	return autoConvert_v1beta2_ClusterClassVariableMetadata_To_v1beta1_ClusterClassVariableMetadata(&in.DeprecatedV1Beta1Metadata, &out.Metadata, s)
}

func Convert_v1beta1_ExternalPatchDefinition_To_v1beta2_ExternalPatchDefinition(in *ExternalPatchDefinition, out *clusterv1.ExternalPatchDefinition, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_ExternalPatchDefinition_To_v1beta2_ExternalPatchDefinition(in, out, s); err != nil {
		return err
	}

	out.GeneratePatchesExtension = in.GenerateExtension
	out.ValidateTopologyExtension = in.ValidateExtension
	return nil
}

func Convert_v1beta2_ExternalPatchDefinition_To_v1beta1_ExternalPatchDefinition(in *clusterv1.ExternalPatchDefinition, out *ExternalPatchDefinition, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_ExternalPatchDefinition_To_v1beta1_ExternalPatchDefinition(in, out, s); err != nil {
		return err
	}

	out.GenerateExtension = in.GeneratePatchesExtension
	out.ValidateExtension = in.ValidateTopologyExtension
	return nil
}

func Convert_v1_ObjectReference_To_v1beta2_MachineNodeReference(in *corev1.ObjectReference, out *clusterv1.MachineNodeReference, _ apimachineryconversion.Scope) error {
	out.Name = in.Name
	return nil
}

func Convert_v1beta2_MachineNodeReference_To_v1_ObjectReference(in *clusterv1.MachineNodeReference, out *corev1.ObjectReference, _ apimachineryconversion.Scope) error {
	out.Name = in.Name
	out.APIVersion = corev1.SchemeGroupVersion.String()
	out.Kind = "Node"
	return nil
}

func Convert_v1beta1_LocalObjectTemplate_To_v1beta2_ClusterClassTemplate(in *LocalObjectTemplate, out *clusterv1.ClusterClassTemplate, _ apimachineryconversion.Scope) error {
	if in.Ref == nil {
		return nil
	}

	out.Ref = &clusterv1.ClusterClassTemplateReference{
		Kind:       in.Ref.Kind,
		Name:       in.Ref.Name,
		APIVersion: in.Ref.APIVersion,
	}
	return nil
}

func Convert_v1beta2_ClusterClassTemplate_To_v1beta1_LocalObjectTemplate(in *clusterv1.ClusterClassTemplate, out *LocalObjectTemplate, _ apimachineryconversion.Scope) error {
	if in.Ref == nil {
		return nil
	}

	out.Ref = &corev1.ObjectReference{
		Kind:       in.Ref.Kind,
		Name:       in.Ref.Name,
		APIVersion: in.Ref.APIVersion,
	}
	return nil
}
