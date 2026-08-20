/*
Copyright 2026 The Kubernetes Authors.

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

package conversion

import (
	"context"
	"reflect"

	"sigs.k8s.io/controller-runtime/pkg/webhook/conversion"

	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	conversionutil "sigs.k8s.io/cluster-api/util/conversion"
)

// Cluster is a HubSpokeConverter for the Cluster API type.
var Cluster = conversion.NewHubSpokeConverter(&clusterv1.Cluster{},
	conversion.NewSpokeConverter(&clusterv1beta1.Cluster{}, ConvertClusterHubToV1Beta1, ConvertClusterV1Beta1ToHub),
)

// ConvertClusterV1Beta1ToHub converts a v1beta1 Cluster to a hub Cluster.
func ConvertClusterV1Beta1ToHub(_ context.Context, src *clusterv1beta1.Cluster, dst *clusterv1.Cluster) error {
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
	ok, err := conversionutil.UnmarshalData(src, restored)
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

	// Recover fields that do not exist in v1beta1.
	if ok {
		restoreWorkerVersions(restored, dst)
	}
	return nil
}

// restoreWorkerVersions restores the MachineDeployment/MachinePool topology versions, which do not
// exist in v1beta1. Topologies are matched by name, because v1beta1 clients can reorder the lists.
func restoreWorkerVersions(restored, dst *clusterv1.Cluster) {
	restoredMDVersions := map[string]string{}
	for _, md := range restored.Spec.Topology.Workers.MachineDeployments {
		restoredMDVersions[md.Name] = md.Version
	}
	for i, md := range dst.Spec.Topology.Workers.MachineDeployments {
		dst.Spec.Topology.Workers.MachineDeployments[i].Version = restoredMDVersions[md.Name]
	}

	restoredMPVersions := map[string]string{}
	for _, mp := range restored.Spec.Topology.Workers.MachinePools {
		restoredMPVersions[mp.Name] = mp.Version
	}
	for i, mp := range dst.Spec.Topology.Workers.MachinePools {
		dst.Spec.Topology.Workers.MachinePools[i].Version = restoredMPVersions[mp.Name]
	}
}

// ConvertClusterHubToV1Beta1 converts a hub Cluster to a v1beta1 Cluster.
func ConvertClusterHubToV1Beta1(ctx context.Context, src *clusterv1.Cluster, dst *clusterv1beta1.Cluster) error {
	if err := clusterv1beta1.Convert_v1beta2_Cluster_To_v1beta1_Cluster(src, dst, nil); err != nil {
		return err
	}

	if src.Spec.InfrastructureRef.IsDefined() {
		infraRef, err := convertToObjectReference(ctx, src.Spec.InfrastructureRef, src.Namespace)
		if err != nil {
			return err
		}
		dst.Spec.InfrastructureRef = infraRef
	}

	if src.Spec.ControlPlaneRef.IsDefined() {
		controlPlaneRef, err := convertToObjectReference(ctx, src.Spec.ControlPlaneRef, src.Namespace)
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

	return conversionutil.MarshalDataUnsafeNoCopy(src, dst)
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
