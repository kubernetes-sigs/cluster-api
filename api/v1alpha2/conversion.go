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
	"errors"

	conversion "k8s.io/apimachinery/pkg/conversion"
	v1alpha1 "sigs.k8s.io/cluster-api/pkg/apis/deprecated/v1alpha1"
)

//nolint
func Convert_v1alpha2_ClusterSpec_To_v1alpha1_ClusterSpec(in *ClusterSpec, out *v1alpha1.ClusterSpec, s conversion.Scope) error {
	return errors.New("Not implemented")
}

//nolint
func Convert_v1alpha1_ClusterNetworkingConfig_To_v1alpha2_ClusterNetwork(in *v1alpha1.ClusterNetworkingConfig, out *ClusterNetwork, s conversion.Scope) error {
	var (
		services NetworkRanges
		pods     NetworkRanges
	)

	if err := Convert_v1alpha1_NetworkRanges_To_v1alpha2_NetworkRanges(&in.Services, &services, s); err != nil {
		return err
	}

	if err := Convert_v1alpha1_NetworkRanges_To_v1alpha2_NetworkRanges(&in.Pods, &pods, s); err != nil {
		return err
	}

	out.Pods = &pods
	out.Services = &services

	return nil
}

//nolint
func Convert_v1alpha1_ClusterSpec_To_v1alpha2_ClusterSpec(in *v1alpha1.ClusterSpec, out *ClusterSpec, s conversion.Scope) error {
	var clusterNetwork ClusterNetwork

	if err := Convert_v1alpha1_ClusterNetworkingConfig_To_v1alpha2_ClusterNetwork(&in.ClusterNetwork, &clusterNetwork, s); err != nil {
		return err
	}

	out.ClusterNetwork = &clusterNetwork
	// DISCARDS:
	// ProviderSpec

	return nil
}

//nolint
func Convert_v1alpha2_ClusterStatus_To_v1alpha1_ClusterStatus(in *ClusterStatus, out *v1alpha1.ClusterStatus, s conversion.Scope) error {
	return errors.New("Not implemented")
}

//nolint
func Convert_v1alpha1_ClusterStatus_To_v1alpha2_ClusterStatus(in *v1alpha1.ClusterStatus, out *ClusterStatus, s conversion.Scope) error {
	if err := autoConvert_v1alpha1_ClusterStatus_To_v1alpha2_ClusterStatus(in, out, s); err != nil {
		return err
	}

	out.ErrorReason = &in.ErrorReason
	// DISCARDS:
	// ProviderStatus

	return nil
}

//nolint
func Convert_v1alpha2_MachineSpec_To_v1alpha1_MachineSpec(in *MachineSpec, out *v1alpha1.MachineSpec, s conversion.Scope) error {
	return errors.New("not implemented")
}

//nolint
func Convert_v1alpha1_MachineSpec_To_v1alpha2_MachineSpec(in *v1alpha1.MachineSpec, out *MachineSpec, s conversion.Scope) error {
	if err := autoConvert_v1alpha1_MachineSpec_To_v1alpha2_MachineSpec(in, out, s); err != nil {
		return err
	}

	if in.Versions.ControlPlane != "" {
		out.Version = &in.Versions.ControlPlane
	} else if in.Versions.Kubelet != "" {
		out.Version = &in.Versions.Kubelet
	}

	// DISCARDS:
	// Taints
	// ProviderSpec
	// ConfigSource

	return nil
}

//nolint
func Convert_v1alpha2_MachineStatus_To_v1alpha1_MachineStatus(in *MachineStatus, out *v1alpha1.MachineStatus, s conversion.Scope) error {
	return errors.New("not implemented")
}

//nolint
func Convert_v1alpha1_MachineStatus_To_v1alpha2_MachineStatus(in *v1alpha1.MachineStatus, out *MachineStatus, s conversion.Scope) error {
	if err := autoConvert_v1alpha1_MachineStatus_To_v1alpha2_MachineStatus(in, out, s); err != nil {
		return err
	}

	if in.Versions == nil {
		return nil
	}

	if in.Versions.ControlPlane != "" {
		out.Version = &in.Versions.ControlPlane
	} else if in.Versions.Kubelet != "" {
		out.Version = &in.Versions.Kubelet
	}

	// DISCARDS:
	// ProviderStatus
	// Conditions
	// LastOperation
	return nil
}

//nolint
func Convert_v1alpha2_MachineSetStatus_To_v1alpha1_MachineSetStatus(in *MachineSetStatus, out *v1alpha1.MachineSetStatus, s conversion.Scope) error {
	return errors.New("Not implemented")
}

//nolint
func Convert_v1alpha2_MachineDeploymentStatus_To_v1alpha1_MachineDeploymentStatus(in *MachineDeploymentStatus, out *v1alpha1.MachineDeploymentStatus, s conversion.Scope) error {
	return errors.New("Not implemented")
}
