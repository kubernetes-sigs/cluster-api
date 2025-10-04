/*
Copyright 2025 The Kubernetes Authors.

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

package webhooks

import (
	"errors"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"

	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	clusterv1alpha3 "sigs.k8s.io/cluster-api/internal/api/core/v1alpha3"
	clusterv1alpha4 "sigs.k8s.io/cluster-api/internal/api/core/v1alpha4"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

var apiVersionGetter = func(_ schema.GroupKind) (string, error) {
	return "", errors.New("apiVersionGetter not set")
}

func SetAPIVersionGetter(f func(gk schema.GroupKind) (string, error)) {
	apiVersionGetter = f
}

func ConvertClusterV1Beta1ToHub(src *clusterv1beta1.Cluster, dst *clusterv1.Cluster) error {
	if err := clusterv1beta1.Convert_v1beta1_Cluster_To_v1beta2_Cluster(src, dst, nil); err != nil {
		return err
	}

	if src.Spec.InfrastructureRef != nil {
		infraRef, err := convertToContractVersionedObjectReference(src.Spec.InfrastructureRef)
		if err != nil {
			return err
		}
		dst.Spec.InfrastructureRef = infraRef
	}

	if src.Spec.ControlPlaneRef != nil {
		controlPlaneRef, err := convertToContractVersionedObjectReference(src.Spec.ControlPlaneRef)
		if err != nil {
			return err
		}
		dst.Spec.ControlPlaneRef = controlPlaneRef
	}

	restored := &clusterv1.Cluster{}
	ok, err := utilconversion.UnmarshalData(src, restored)
	if err != nil {
		return err
	}

	// Recover intent for bool values converted to *bool.
	clusterv1.Convert_bool_To_Pointer_bool(src.Spec.Paused, ok, restored.Spec.Paused, &dst.Spec.Paused)

	initialization := clusterv1.ClusterInitializationStatus{}
	restoredControlPlaneInitialized := restored.Status.Initialization.ControlPlaneInitialized
	restoredInfrastructureProvisioned := restored.Status.Initialization.InfrastructureProvisioned
	clusterv1.Convert_bool_To_Pointer_bool(src.Status.ControlPlaneReady, ok, restoredControlPlaneInitialized, &initialization.ControlPlaneInitialized)
	clusterv1.Convert_bool_To_Pointer_bool(src.Status.InfrastructureReady, ok, restoredInfrastructureProvisioned, &initialization.InfrastructureProvisioned)
	if !reflect.DeepEqual(initialization, clusterv1.ClusterInitializationStatus{}) {
		dst.Status.Initialization = initialization
	}
	return nil
}

func ConvertClusterHubToV1Beta1(src *clusterv1.Cluster, dst *clusterv1beta1.Cluster) error {
	if err := clusterv1beta1.Convert_v1beta2_Cluster_To_v1beta1_Cluster(src, dst, nil); err != nil {
		return err
	}

	if src.Spec.InfrastructureRef.IsDefined() {
		infraRef, err := convertToObjectReference(src.Spec.InfrastructureRef, src.Namespace)
		if err != nil {
			return err
		}
		dst.Spec.InfrastructureRef = infraRef
	}

	if src.Spec.ControlPlaneRef.IsDefined() {
		controlPlaneRef, err := convertToObjectReference(src.Spec.ControlPlaneRef, src.Namespace)
		if err != nil {
			return err
		}
		dst.Spec.ControlPlaneRef = controlPlaneRef
	}

	if dst.Spec.ClusterNetwork != nil && dst.Spec.ClusterNetwork.APIServerPort != nil &&
		*dst.Spec.ClusterNetwork.APIServerPort == 0 {
		dst.Spec.ClusterNetwork.APIServerPort = nil
	}

	if dst.Spec.Topology != nil {
		if dst.Spec.Topology.ControlPlane.MachineHealthCheck != nil && dst.Spec.Topology.ControlPlane.MachineHealthCheck.RemediationTemplate != nil {
			dst.Spec.Topology.ControlPlane.MachineHealthCheck.RemediationTemplate.Namespace = dst.Namespace
		}
		if dst.Spec.Topology.Workers != nil {
			for _, md := range dst.Spec.Topology.Workers.MachineDeployments {
				if md.MachineHealthCheck != nil && md.MachineHealthCheck.RemediationTemplate != nil {
					md.MachineHealthCheck.RemediationTemplate.Namespace = dst.Namespace
				}
			}
		}
	}

	dropEmptyStringsCluster(dst)

	return utilconversion.MarshalData(src, dst)
}

func ConvertClusterV1Alpha4ToHub(src *clusterv1alpha4.Cluster, dst *clusterv1.Cluster) error {
	if err := clusterv1alpha4.Convert_v1alpha4_Cluster_To_v1beta2_Cluster(src, dst, nil); err != nil {
		return err
	}

	if src.Spec.InfrastructureRef != nil {
		infraRef, err := convertToContractVersionedObjectReference(src.Spec.InfrastructureRef)
		if err != nil {
			return err
		}
		dst.Spec.InfrastructureRef = infraRef
	}

	if src.Spec.ControlPlaneRef != nil {
		controlPlaneRef, err := convertToContractVersionedObjectReference(src.Spec.ControlPlaneRef)
		if err != nil {
			return err
		}
		dst.Spec.ControlPlaneRef = controlPlaneRef
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1alpha4 conditions should not be automatically be converted into v1beta2 conditions.
	dst.Status.Conditions = nil

	// Move legacy conditions (v1alpha4), failureReason and failureMessage to the deprecated field.
	if src.Status.Conditions != nil || src.Status.FailureReason != nil || src.Status.FailureMessage != nil {
		dst.Status.Deprecated = &clusterv1.ClusterDeprecatedStatus{}
		dst.Status.Deprecated.V1Beta1 = &clusterv1.ClusterV1Beta1DeprecatedStatus{}
		if src.Status.Conditions != nil {
			clusterv1alpha4.Convert_v1alpha4_Conditions_To_v1beta2_Deprecated_V1Beta1_Conditions(&src.Status.Conditions, &dst.Status.Deprecated.V1Beta1.Conditions)
		}
		dst.Status.Deprecated.V1Beta1.FailureReason = src.Status.FailureReason
		dst.Status.Deprecated.V1Beta1.FailureMessage = src.Status.FailureMessage
	}

	// Move ControlPlaneReady and InfrastructureReady to initialization is implemented in ConvertTo.

	// Manually restore data.
	restored := &clusterv1.Cluster{}
	ok, err := utilconversion.UnmarshalData(src, restored)
	if err != nil {
		return err
	}

	// Recover intent for bool values converted to *bool.
	clusterv1.Convert_bool_To_Pointer_bool(src.Spec.Paused, ok, restored.Spec.Paused, &dst.Spec.Paused)

	initialization := clusterv1.ClusterInitializationStatus{}
	restoredControlPlaneInitialized := restored.Status.Initialization.ControlPlaneInitialized
	restoredInfrastructureProvisioned := restored.Status.Initialization.InfrastructureProvisioned
	clusterv1.Convert_bool_To_Pointer_bool(src.Status.ControlPlaneReady, ok, restoredControlPlaneInitialized, &initialization.ControlPlaneInitialized)
	clusterv1.Convert_bool_To_Pointer_bool(src.Status.InfrastructureReady, ok, restoredInfrastructureProvisioned, &initialization.InfrastructureProvisioned)
	if !reflect.DeepEqual(initialization, clusterv1.ClusterInitializationStatus{}) {
		dst.Status.Initialization = initialization
	}

	for i, fd := range dst.Status.FailureDomains {
		srcFD, ok := src.Status.FailureDomains[fd.Name]
		if !ok {
			return fmt.Errorf("failure domain %q not found in source data", fd.Name)
		}
		var restoredFDControlPlane *bool
		for _, restoredFD := range restored.Status.FailureDomains {
			if restoredFD.Name == fd.Name {
				restoredFDControlPlane = restoredFD.ControlPlane
				break
			}
		}
		clusterv1.Convert_bool_To_Pointer_bool(srcFD.ControlPlane, ok, restoredFDControlPlane, &fd.ControlPlane)
		dst.Status.FailureDomains[i] = fd
	}

	// Recover other values
	if ok {
		dst.Spec.AvailabilityGates = restored.Spec.AvailabilityGates
		dst.Spec.Topology.ClassRef.Namespace = restored.Spec.Topology.ClassRef.Namespace
		dst.Spec.Topology.Variables = restored.Spec.Topology.Variables
		dst.Spec.Topology.ControlPlane.Variables = restored.Spec.Topology.ControlPlane.Variables

		dst.Spec.Topology.ControlPlane.HealthCheck = restored.Spec.Topology.ControlPlane.HealthCheck

		if restored.Spec.Topology.ControlPlane.Deletion.NodeDrainTimeoutSeconds != nil {
			dst.Spec.Topology.ControlPlane.Deletion.NodeDrainTimeoutSeconds = restored.Spec.Topology.ControlPlane.Deletion.NodeDrainTimeoutSeconds
		}

		if restored.Spec.Topology.ControlPlane.Deletion.NodeVolumeDetachTimeoutSeconds != nil {
			dst.Spec.Topology.ControlPlane.Deletion.NodeVolumeDetachTimeoutSeconds = restored.Spec.Topology.ControlPlane.Deletion.NodeVolumeDetachTimeoutSeconds
		}

		if restored.Spec.Topology.ControlPlane.Deletion.NodeDeletionTimeoutSeconds != nil {
			dst.Spec.Topology.ControlPlane.Deletion.NodeDeletionTimeoutSeconds = restored.Spec.Topology.ControlPlane.Deletion.NodeDeletionTimeoutSeconds
		}
		dst.Spec.Topology.ControlPlane.ReadinessGates = restored.Spec.Topology.ControlPlane.ReadinessGates

		for i := range restored.Spec.Topology.Workers.MachineDeployments {
			dst.Spec.Topology.Workers.MachineDeployments[i].FailureDomain = restored.Spec.Topology.Workers.MachineDeployments[i].FailureDomain
			dst.Spec.Topology.Workers.MachineDeployments[i].Variables = restored.Spec.Topology.Workers.MachineDeployments[i].Variables
			dst.Spec.Topology.Workers.MachineDeployments[i].ReadinessGates = restored.Spec.Topology.Workers.MachineDeployments[i].ReadinessGates
			dst.Spec.Topology.Workers.MachineDeployments[i].Deletion.Order = restored.Spec.Topology.Workers.MachineDeployments[i].Deletion.Order
			dst.Spec.Topology.Workers.MachineDeployments[i].Deletion.NodeDrainTimeoutSeconds = restored.Spec.Topology.Workers.MachineDeployments[i].Deletion.NodeDrainTimeoutSeconds
			dst.Spec.Topology.Workers.MachineDeployments[i].Deletion.NodeVolumeDetachTimeoutSeconds = restored.Spec.Topology.Workers.MachineDeployments[i].Deletion.NodeVolumeDetachTimeoutSeconds
			dst.Spec.Topology.Workers.MachineDeployments[i].Deletion.NodeDeletionTimeoutSeconds = restored.Spec.Topology.Workers.MachineDeployments[i].Deletion.NodeDeletionTimeoutSeconds
			dst.Spec.Topology.Workers.MachineDeployments[i].MinReadySeconds = restored.Spec.Topology.Workers.MachineDeployments[i].MinReadySeconds
			dst.Spec.Topology.Workers.MachineDeployments[i].Rollout.Strategy = restored.Spec.Topology.Workers.MachineDeployments[i].Rollout.Strategy
			dst.Spec.Topology.Workers.MachineDeployments[i].HealthCheck = restored.Spec.Topology.Workers.MachineDeployments[i].HealthCheck
		}

		dst.Spec.Topology.Workers.MachinePools = restored.Spec.Topology.Workers.MachinePools

		dst.Status.Conditions = restored.Status.Conditions
		dst.Status.ControlPlane = restored.Status.ControlPlane
		dst.Status.Workers = restored.Status.Workers
	}

	return nil
}

func ConvertClusterHubToV1Alpha4(src *clusterv1.Cluster, dst *clusterv1alpha4.Cluster) error {
	if err := clusterv1alpha4.Convert_v1beta2_Cluster_To_v1alpha4_Cluster(src, dst, nil); err != nil {
		return err
	}

	if src.Spec.InfrastructureRef.IsDefined() {
		infraRef, err := convertToObjectReference(src.Spec.InfrastructureRef, src.Namespace)
		if err != nil {
			return err
		}
		dst.Spec.InfrastructureRef = infraRef
	}

	if src.Spec.ControlPlaneRef.IsDefined() {
		controlPlaneRef, err := convertToObjectReference(src.Spec.ControlPlaneRef, src.Namespace)
		if err != nil {
			return err
		}
		dst.Spec.ControlPlaneRef = controlPlaneRef
	}

	if dst.Spec.ClusterNetwork != nil && dst.Spec.ClusterNetwork.APIServerPort != nil &&
		*dst.Spec.ClusterNetwork.APIServerPort == 0 {
		dst.Spec.ClusterNetwork.APIServerPort = nil
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta2 conditions should not be automatically be converted into legacy conditions (v1alpha4).
	dst.Status.Conditions = nil

	// Retrieve legacy conditions (v1alpha4), failureReason and failureMessage from the deprecated field.
	if src.Status.Deprecated != nil {
		if src.Status.Deprecated.V1Beta1 != nil {
			if src.Status.Deprecated.V1Beta1.Conditions != nil {
				clusterv1alpha4.Convert_v1beta2_Deprecated_V1Beta1_Conditions_To_v1alpha4_Conditions(&src.Status.Deprecated.V1Beta1.Conditions, &dst.Status.Conditions)
			}
			dst.Status.FailureReason = src.Status.Deprecated.V1Beta1.FailureReason
			dst.Status.FailureMessage = src.Status.Deprecated.V1Beta1.FailureMessage
		}
	}

	// Move initialization to old fields
	dst.Status.ControlPlaneReady = ptr.Deref(src.Status.Initialization.ControlPlaneInitialized, false)
	dst.Status.InfrastructureReady = ptr.Deref(src.Status.Initialization.InfrastructureProvisioned, false)

	// Preserve Hub data on down-conversion except for metadata
	if err := utilconversion.MarshalData(src, dst); err != nil {
		return err
	}

	return nil
}

func ConvertClusterV1Alpha3ToHub(src *clusterv1alpha3.Cluster, dst *clusterv1.Cluster) error {
	if err := clusterv1alpha3.Convert_v1alpha3_Cluster_To_v1beta2_Cluster(src, dst, nil); err != nil {
		return err
	}

	if src.Spec.InfrastructureRef != nil {
		infraRef, err := convertToContractVersionedObjectReference(src.Spec.InfrastructureRef)
		if err != nil {
			return err
		}
		dst.Spec.InfrastructureRef = infraRef
	}

	if src.Spec.ControlPlaneRef != nil {
		controlPlaneRef, err := convertToContractVersionedObjectReference(src.Spec.ControlPlaneRef)
		if err != nil {
			return err
		}
		dst.Spec.ControlPlaneRef = controlPlaneRef
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1alpha3 conditions should not be automatically be converted into v1beta2 conditions.
	dst.Status.Conditions = nil

	// Move legacy conditions (v1alpha3), failureReason and failureMessage to the deprecated field.
	if src.Status.Conditions != nil || src.Status.FailureReason != nil || src.Status.FailureMessage != nil {
		dst.Status.Deprecated = &clusterv1.ClusterDeprecatedStatus{}
		dst.Status.Deprecated.V1Beta1 = &clusterv1.ClusterV1Beta1DeprecatedStatus{}
		if src.Status.Conditions != nil {
			clusterv1alpha3.Convert_v1alpha3_Conditions_To_v1beta2_Deprecated_V1Beta1_Conditions(&src.Status.Conditions, &dst.Status.Deprecated.V1Beta1.Conditions)
		}
		dst.Status.Deprecated.V1Beta1.FailureReason = src.Status.FailureReason
		dst.Status.Deprecated.V1Beta1.FailureMessage = src.Status.FailureMessage
	}

	// Given this is a bool and there is no timestamp associated with it, when this condition is set, its timestamp
	// will be "now". See https://github.com/kubernetes-sigs/cluster-api/issues/3798#issuecomment-708619826 for more
	// discussion.
	if src.Status.ControlPlaneInitialized {
		v1beta1conditions.MarkTrue(dst, clusterv1.ControlPlaneInitializedV1Beta1Condition)
	}

	// Manually restore data.
	restored := &clusterv1.Cluster{}
	ok, err := utilconversion.UnmarshalData(src, restored)
	if err != nil {
		return err
	}

	// Recover intent for bool values converted to *bool.
	clusterv1.Convert_bool_To_Pointer_bool(src.Spec.Paused, ok, restored.Spec.Paused, &dst.Spec.Paused)

	initialization := clusterv1.ClusterInitializationStatus{}
	restoredControlPlaneInitialized := restored.Status.Initialization.ControlPlaneInitialized
	restoredInfrastructureProvisioned := restored.Status.Initialization.InfrastructureProvisioned
	clusterv1.Convert_bool_To_Pointer_bool(src.Status.ControlPlaneReady, ok, restoredControlPlaneInitialized, &initialization.ControlPlaneInitialized)
	clusterv1.Convert_bool_To_Pointer_bool(src.Status.InfrastructureReady, ok, restoredInfrastructureProvisioned, &initialization.InfrastructureProvisioned)
	if !reflect.DeepEqual(initialization, clusterv1.ClusterInitializationStatus{}) {
		dst.Status.Initialization = initialization
	}

	for i, fd := range dst.Status.FailureDomains {
		srcFD, ok := src.Status.FailureDomains[fd.Name]
		if !ok {
			return fmt.Errorf("failure domain %q not found in source data", fd.Name)
		}
		var restoredFDControlPlane *bool
		for _, restoredFD := range restored.Status.FailureDomains {
			if restoredFD.Name == fd.Name {
				restoredFDControlPlane = restoredFD.ControlPlane
				break
			}
		}
		clusterv1.Convert_bool_To_Pointer_bool(srcFD.ControlPlane, ok, restoredFDControlPlane, &fd.ControlPlane)
		dst.Status.FailureDomains[i] = fd
	}

	// Recover other values
	if ok {
		dst.Spec.AvailabilityGates = restored.Spec.AvailabilityGates
		dst.Spec.Topology = restored.Spec.Topology
		dst.Status.Conditions = restored.Status.Conditions
		dst.Status.ControlPlane = restored.Status.ControlPlane
		dst.Status.Workers = restored.Status.Workers
	}

	return nil
}

func ConvertClusterHubToV1Alpha3(src *clusterv1.Cluster, dst *clusterv1alpha3.Cluster) error {
	if err := clusterv1alpha3.Convert_v1beta2_Cluster_To_v1alpha3_Cluster(src, dst, nil); err != nil {
		return err
	}

	if src.Spec.InfrastructureRef.IsDefined() {
		infraRef, err := convertToObjectReference(src.Spec.InfrastructureRef, src.Namespace)
		if err != nil {
			return err
		}
		dst.Spec.InfrastructureRef = infraRef
	}

	if src.Spec.ControlPlaneRef.IsDefined() {
		controlPlaneRef, err := convertToObjectReference(src.Spec.ControlPlaneRef, src.Namespace)
		if err != nil {
			return err
		}
		dst.Spec.ControlPlaneRef = controlPlaneRef
	}

	if dst.Spec.ClusterNetwork != nil && dst.Spec.ClusterNetwork.APIServerPort != nil &&
		*dst.Spec.ClusterNetwork.APIServerPort == 0 {
		dst.Spec.ClusterNetwork.APIServerPort = nil
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta2 conditions should not be automatically be converted into legacy conditions (v1alpha3).
	dst.Status.Conditions = nil

	// Retrieve legacy conditions (v1alpha3), failureReason and failureMessage from the deprecated field.
	if src.Status.Deprecated != nil {
		if src.Status.Deprecated.V1Beta1 != nil {
			if src.Status.Deprecated.V1Beta1.Conditions != nil {
				clusterv1alpha3.Convert_v1beta2_Deprecated_V1Beta1_Conditions_To_v1alpha3_Conditions(&src.Status.Deprecated.V1Beta1.Conditions, &dst.Status.Conditions)
			}
			dst.Status.FailureReason = src.Status.Deprecated.V1Beta1.FailureReason
			dst.Status.FailureMessage = src.Status.Deprecated.V1Beta1.FailureMessage
		}
	}

	// Set the v1alpha3 boolean status field if the v1alpha4 condition was true
	if v1beta1conditions.IsTrue(src, clusterv1.ControlPlaneInitializedV1Beta1Condition) {
		dst.Status.ControlPlaneInitialized = true
	}

	// Move initialization to old fields
	dst.Status.ControlPlaneReady = ptr.Deref(src.Status.Initialization.ControlPlaneInitialized, false)
	dst.Status.InfrastructureReady = ptr.Deref(src.Status.Initialization.InfrastructureProvisioned, false)

	// Preserve Hub data on down-conversion except for metadata
	if err := utilconversion.MarshalData(src, dst); err != nil {
		return err
	}

	return nil
}

func convertToContractVersionedObjectReference(ref *corev1.ObjectReference) (clusterv1.ContractVersionedObjectReference, error) {
	var apiGroup string
	if ref.APIVersion != "" {
		gv, err := schema.ParseGroupVersion(ref.APIVersion)
		if err != nil {
			return clusterv1.ContractVersionedObjectReference{}, fmt.Errorf("failed to convert object: failed to parse apiVersion: %v", err)
		}
		apiGroup = gv.Group
	}
	return clusterv1.ContractVersionedObjectReference{
		APIGroup: apiGroup,
		Kind:     ref.Kind,
		Name:     ref.Name,
	}, nil
}

func convertToObjectReference(ref clusterv1.ContractVersionedObjectReference, namespace string) (*corev1.ObjectReference, error) {
	apiVersion, err := apiVersionGetter(schema.GroupKind{
		Group: ref.APIGroup,
		Kind:  ref.Kind,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to convert object: %v", err)
	}
	return &corev1.ObjectReference{
		APIVersion: apiVersion,
		Kind:       ref.Kind,
		Namespace:  namespace,
		Name:       ref.Name,
	}, nil
}

func dropEmptyStringsCluster(dst *clusterv1beta1.Cluster) {
	if dst.Spec.Topology != nil {
		if dst.Spec.Topology.Workers != nil {
			for i, md := range dst.Spec.Topology.Workers.MachineDeployments {
				dropEmptyString(&md.FailureDomain)
				dst.Spec.Topology.Workers.MachineDeployments[i] = md
			}
		}
	}
}

func dropEmptyString(s **string) {
	if *s != nil && **s == "" {
		*s = nil
	}
}
