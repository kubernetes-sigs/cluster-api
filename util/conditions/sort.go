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

package conditions

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
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
// | Updating                       |         |     |    |    |    | x       |
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
	clusterv1.AvailableCondition,
	clusterv1.ReadyCondition,
	clusterv1.MachineUpToDateCondition,
	clusterv1.ClusterRemoteConnectionProbeCondition,
	clusterv1.BootstrapConfigReadyCondition,
	clusterv1.InfrastructureReadyCondition,
	clusterv1.ClusterControlPlaneInitializedCondition,
	clusterv1.ClusterControlPlaneAvailableCondition,
	clusterv1.ClusterWorkersAvailableCondition,
	kubeadmControlPlaneCertificatesAvailableCondition,
	kubeadmControlPlaneInitializedCondition,
	kubeadmControlPlaneEtcdClusterHealthyCondition,
	kubeadmControlPlaneControlPlaneComponentsHealthyCondition,
	clusterv1.MachineNodeHealthyCondition,
	clusterv1.MachineNodeReadyCondition,
	kubeadmControlPlaneMachineEtcdPodHealthyCondition,
	kubeadmControlPlaneMachineEtcdMemberHealthyCondition,
	kubeadmControlPlaneMachineAPIServerPodHealthyCondition,
	kubeadmControlPlaneMachineControllerManagerPodHealthyCondition,
	kubeadmControlPlaneMachineSchedulerPodHealthyCondition,
	clusterv1.MachineHealthCheckSucceededCondition,
	clusterv1.MachineOwnerRemediatedCondition,
	clusterv1.ClusterTopologyReconciledCondition,
	clusterv1.MachineUpdatingCondition,
	clusterv1.RollingOutCondition,
	clusterv1.RemediatingCondition,
	clusterv1.ScalingDownCondition,
	clusterv1.ScalingUpCondition,
	clusterv1.MachinesReadyCondition,
	clusterv1.ClusterControlPlaneMachinesReadyCondition,
	clusterv1.ClusterWorkerMachinesReadyCondition,
	clusterv1.MachinesUpToDateCondition,
	clusterv1.ClusterControlPlaneMachinesUpToDateCondition,
	clusterv1.ClusterWorkerMachinesUpToDateCondition,
	readinessAndAvailabilityGates,
	clusterv1.PausedCondition,
	clusterv1.DeletingCondition,
}

// Constants defining a placeholder for readiness and availability gates.
const (
	readinessAndAvailabilityGates = ""
)

// Constants inlined for ordering (we want to avoid importing the KCP API package).
const (
	kubeadmControlPlaneCertificatesAvailableCondition              = "CertificatesAvailable"
	kubeadmControlPlaneInitializedCondition                        = "Initialized"
	kubeadmControlPlaneEtcdClusterHealthyCondition                 = "EtcdClusterHealthy"
	kubeadmControlPlaneControlPlaneComponentsHealthyCondition      = "ControlPlaneComponentsHealthy"
	kubeadmControlPlaneMachineAPIServerPodHealthyCondition         = "APIServerPodHealthy"
	kubeadmControlPlaneMachineControllerManagerPodHealthyCondition = "ControllerManagerPodHealthy"
	kubeadmControlPlaneMachineSchedulerPodHealthyCondition         = "SchedulerPodHealthy"
	kubeadmControlPlaneMachineEtcdPodHealthyCondition              = "EtcdPodHealthy"
	kubeadmControlPlaneMachineEtcdMemberHealthyCondition           = "EtcdMemberHealthy"
)
