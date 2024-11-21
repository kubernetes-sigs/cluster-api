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

package v1beta2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

var orderMap = map[string]int{}

func init() {
	for i, c := range order {
		orderMap[c] = i
	}
}

// defaultSortLessFunc returns true if a condition is less than another with regards to the
// order of conditions designed for convenience of the consumer, i.e. kubectl get.
// According to this order the Available and the Ready condition always goes first, Deleting and Paused always goes last,
// and all the other conditions are sorted by Type.
func defaultSortLessFunc(i, j metav1.Condition) bool {
	fi, oki := orderMap[i.Type]
	if !oki {
		fi = orderMap[readinessAndAvailabilityGates]
	}
	fj, okj := orderMap[j.Type]
	if !okj {
		fj = orderMap[readinessAndAvailabilityGates]
	}
	return fi < fj ||
		(fi == fj && i.Type < j.Type)
}

// The order array below leads to the following condition ordering:
//
// | Condition                      | Cluster | KCP | MD | MS | MP | Machine |
// |--------------------------------|---------|-----|:---|----|----|---------|
// | -- Availability conditions --  |         |     |    |    |    |         |
// | Available                      | x       | x   | x  |    | x  | x       |
// | Ready                          |         |     |    |    |    | x       |
// | UpToDate                       |         |     |    |    |    | x       |
// | RemoteConnectionProbe          | x       |     |    |    |    |         |
// | BootstrapConfigReady           |         |     |    |    | x  | x       |
// | InfrastructureReady            | x       |     |    |    | x  | x       |
// | ControlPlaneInitialized        | x       |     |    |    |    |         |
// | ControlPlaneAvailable          | x       |     |    |    |    |         |
// | WorkersAvailable               | x       |     |    |    |    |         |
// | CertificatesAvailable          |         | x   |    |    |    |         |
// | Initialized                    |         | x   |    |    |    |         |
// | EtcdClusterHealthy             |         | x   |    |    |    |         |
// | ControlPlaneComponentsHealthy  |         | x   |    |    |    |         |
// | NodeHealthy                    |         |     |    |    |    | x       |
// | NodeReady                      |         |     |    |    |    | x       |
// | EtcdPodHealthy                 |         |     |    |    |    | x       |
// | EtcdMemberHealthy              |         |     |    |    |    | x       |
// | APIServerPodHealthy            |         |     |    |    |    | x       |
// | ControllerManagerPodHealthy    |         |     |    |    |    | x       |
// | SchedulerPodHealthy            |         |     |    |    |    | x       |
// | HealthCheckSucceeded           |         |     |    |    |    | x       |
// | OwnerRemediated                |         |     |    |    |    | x       |
// | -- Operations --               |         |     |    |    |    |         |
// | TopologyReconciled             | x       |     |    |    |    |         |
// | RollingOut                     | x       | x   | x  |    | x  |         |
// | Remediating                    | x       | x   | x  | x  | x  |         |
// | ScalingDown                    | x       | x   | x  | x  | x  |         |
// | ScalingUp                      | x       | x   | x  | x  | x  |         |
// | -- Aggregated from Machines -- |         |     |    |    |    |         |
// | MachinesReady                  |         | x   | x  | x  | x  |         |
// | ControlPlaneMachinesReady      | x       |     |    |    |    |         |
// | WorkerMachinesReady            | x       |     |    |    |    |         |
// | MachinesUpToDate               |         | x   | x  | x  | x  |         |
// | ControlPlaneMachinesUpToDate   | x       |     |    |    |    |         |
// | WorkerMachinesUpToDate         | x       |     |    |    |    |         |
// | -- From other controllers --   |         |     |    |    |    |         |
// | Readiness/Availability gates   | x       |     |    |    |    | x       |
// | -- Misc --                     |         |     |    |    |    |         |
// | Paused                         | x       | x   | x  | x  | x  | x       |
// | Deleting                       | x       | x   | x  | x  | x  | x       |
// .
var order = []string{
	clusterv1.AvailableV1Beta2Condition,
	clusterv1.ReadyV1Beta2Condition,
	clusterv1.MachineUpToDateV1Beta2Condition,
	clusterv1.ClusterRemoteConnectionProbeV1Beta2Condition,
	clusterv1.BootstrapConfigReadyV1Beta2Condition,
	clusterv1.InfrastructureReadyV1Beta2Condition,
	clusterv1.ClusterControlPlaneInitializedV1Beta2Condition,
	clusterv1.ClusterControlPlaneAvailableV1Beta2Condition,
	clusterv1.ClusterWorkersAvailableV1Beta2Condition,
	kubeadmControlPlaneCertificatesAvailableV1Beta2Condition,
	kubeadmControlPlaneInitializedV1Beta2Condition,
	kubeadmControlPlaneEtcdClusterHealthyV1Beta2Condition,
	kubeadmControlPlaneControlPlaneComponentsHealthyV1Beta2Condition,
	clusterv1.MachineNodeHealthyV1Beta2Condition,
	clusterv1.MachineNodeReadyV1Beta2Condition,
	kubeadmControlPlaneMachineEtcdPodHealthyV1Beta2Condition,
	kubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Condition,
	kubeadmControlPlaneMachineAPIServerPodHealthyV1Beta2Condition,
	kubeadmControlPlaneMachineControllerManagerPodHealthyV1Beta2Condition,
	kubeadmControlPlaneMachineSchedulerPodHealthyV1Beta2Condition,
	clusterv1.MachineHealthCheckSucceededV1Beta2Condition,
	clusterv1.MachineOwnerRemediatedV1Beta2Condition,
	clusterv1.ClusterTopologyReconciledV1Beta2Condition,
	clusterv1.RollingOutV1Beta2Condition,
	clusterv1.RemediatingV1Beta2Condition,
	clusterv1.ScalingDownV1Beta2Condition,
	clusterv1.ScalingUpV1Beta2Condition,
	clusterv1.MachinesReadyV1Beta2Condition,
	clusterv1.ClusterControlPlaneMachinesReadyV1Beta2Condition,
	clusterv1.ClusterWorkerMachinesReadyV1Beta2Condition,
	clusterv1.MachinesUpToDateV1Beta2Condition,
	clusterv1.ClusterControlPlaneMachinesUpToDateV1Beta2Condition,
	clusterv1.ClusterWorkerMachinesUpToDateV1Beta2Condition,
	readinessAndAvailabilityGates,
	clusterv1.PausedV1Beta2Condition,
	clusterv1.DeletingV1Beta2Condition,
}

// Constants defining a placeholder for readiness and availability gates.
const (
	readinessAndAvailabilityGates = ""
)

// Constants inlined for ordering (we want to avoid importing the KCP API package).
const (
	kubeadmControlPlaneCertificatesAvailableV1Beta2Condition              = "CertificatesAvailable"
	kubeadmControlPlaneInitializedV1Beta2Condition                        = "Initialized"
	kubeadmControlPlaneEtcdClusterHealthyV1Beta2Condition                 = "EtcdClusterHealthy"
	kubeadmControlPlaneControlPlaneComponentsHealthyV1Beta2Condition      = "ControlPlaneComponentsHealthy"
	kubeadmControlPlaneMachineAPIServerPodHealthyV1Beta2Condition         = "APIServerPodHealthy"
	kubeadmControlPlaneMachineControllerManagerPodHealthyV1Beta2Condition = "ControllerManagerPodHealthy"
	kubeadmControlPlaneMachineSchedulerPodHealthyV1Beta2Condition         = "SchedulerPodHealthy"
	kubeadmControlPlaneMachineEtcdPodHealthyV1Beta2Condition              = "EtcdPodHealthy"
	kubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Condition           = "EtcdMemberHealthy"
)
