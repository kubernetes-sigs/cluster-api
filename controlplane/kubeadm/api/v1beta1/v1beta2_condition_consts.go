/*
Copyright 2024 The Kubernetes Authors.

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

import clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

// Conditions that will be used for the KubeadmControlPlane object in v1Beta2 API version.
const (
	// KubeadmControlPlaneAvailableV1beta2Condition True if the control plane can be reached, EtcdClusterAvailable is true,
	// and CertificatesAvailable is true.
	KubeadmControlPlaneAvailableV1beta2Condition = clusterv1.AvailableV1beta2Condition

	// KubeadmControlPlaneCertificatesAvailableV1beta2Condition True if all the cluster certificates exist.
	KubeadmControlPlaneCertificatesAvailableV1beta2Condition = "CertificatesAvailable"

	// KubeadmControlPlaneEtcdClusterAvailableV1beta2Condition surfaces issues to the managed etcd cluster, if any.
	// It is computed as aggregation of Machines's EtcdMemberHealthy (if not using an external etcd) conditions plus
	// additional checks validating potential issues to etcd quorum.
	KubeadmControlPlaneEtcdClusterAvailableV1beta2Condition = "EtcdClusterAvailable"

	// KubeadmControlPlaneMachinesReadyV1beta2Condition surfaces detail of issues on the controlled machines, if any.
	// Please note this will include also APIServerPodHealthy, ControllerManagerPodHealthy, SchedulerPodHealthy conditions.
	// If not using an external etcd also EtcdPodHealthy, EtcdMemberHealthy conditions are included.
	KubeadmControlPlaneMachinesReadyV1beta2Condition = clusterv1.MachinesReadyV1beta2Condition

	// KubeadmControlPlaneMachinesUpToDateV1beta2Condition surfaces details of controlled machines not up to date, if any.
	KubeadmControlPlaneMachinesUpToDateV1beta2Condition = clusterv1.MachinesUpToDateV1beta2Condition

	// KubeadmControlPlaneScalingUpV1beta2Condition is true if available replicas < desired replicas.
	KubeadmControlPlaneScalingUpV1beta2Condition = clusterv1.ScalingUpV1beta2Condition

	// KubeadmControlPlaneScalingDownV1beta2Condition is true if replicas > desired replicas.
	KubeadmControlPlaneScalingDownV1beta2Condition = clusterv1.ScalingDownV1beta2Condition

	// KubeadmControlPlaneRemediatingV1beta2Condition surfaces details about ongoing remediation of the controlled machines, if any.
	KubeadmControlPlaneRemediatingV1beta2Condition = clusterv1.RemediatingV1beta2Condition

	// KubeadmControlPlaneDeletingV1beta2Condition surfaces details about ongoing deletion of the controlled machines.
	KubeadmControlPlaneDeletingV1beta2Condition = clusterv1.DeletingV1beta2Condition

	// KubeadmControlPlanePausedV1beta2Condition is true if this resource or the Cluster it belongs to are paused.
	KubeadmControlPlanePausedV1beta2Condition = clusterv1.PausedV1beta2Condition
)

// Conditions that will be used for the KubeadmControlPlane controlled machines in v1Beta2 API version.
const (
	// KubeadmControlPlaneMachineAPIServerPodHealthyV1beta2Condition surfaces the status of the API server pod hosted on a KubeadmControlPlane controlled machine.
	KubeadmControlPlaneMachineAPIServerPodHealthyV1beta2Condition = "APIServerPodHealthy"

	// KubeadmControlPlaneMachineControllerManagerPodHealthyV1beta2Condition surfaces the status of the controller manager pod hosted on a KubeadmControlPlane controlled machine.
	KubeadmControlPlaneMachineControllerManagerPodHealthyV1beta2Condition = "ControllerManagerPodHealthy"

	// KubeadmControlPlaneMachineSchedulerPodHealthyV1beta2Condition surfaces the status of the scheduler pod hosted on a KubeadmControlPlane controlled machine.
	KubeadmControlPlaneMachineSchedulerPodHealthyV1beta2Condition = "SchedulerPodHealthy"

	// KubeadmControlPlaneMachineEtcdPodHealthyV1beta2Condition surfaces the status of the etcd pod hosted on a KubeadmControlPlane controlled machine.
	KubeadmControlPlaneMachineEtcdPodHealthyV1beta2Condition = "EtcdPodHealthy"

	// KubeadmControlPlaneMachineEtcdMemberHealthyV1beta2Condition surfaces the status of the etcd member hosted on a KubeadmControlPlane controlled machine.
	KubeadmControlPlaneMachineEtcdMemberHealthyV1beta2Condition = "EtcdMemberHealthy"
)
