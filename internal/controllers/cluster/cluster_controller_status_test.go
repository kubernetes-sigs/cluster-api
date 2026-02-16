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

package cluster

import (
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
)

func TestSetPhases(t *testing.T) {
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-cluster",
		},
		Status: clusterv1.ClusterStatus{},
		Spec:   clusterv1.ClusterSpec{},
	}

	tests := []struct {
		name      string
		cluster   *clusterv1.Cluster
		wantPhase clusterv1.ClusterPhase
	}{
		{
			name:      "cluster not provisioned",
			cluster:   cluster,
			wantPhase: clusterv1.ClusterPhasePending,
		},
		{
			name: "cluster has infrastructureRef",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cluster",
				},
				Status: clusterv1.ClusterStatus{},
				Spec: clusterv1.ClusterSpec{
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						Name: "infra1",
					},
				},
			},

			wantPhase: clusterv1.ClusterPhaseProvisioning,
		},
		{
			name: "cluster infrastructure is ready",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cluster",
				},
				Status: clusterv1.ClusterStatus{
					Initialization: clusterv1.ClusterInitializationStatus{InfrastructureProvisioned: ptr.To(true)},
				},
				Spec: clusterv1.ClusterSpec{
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						Name: "infra1",
					},
				},
			},

			wantPhase: clusterv1.ClusterPhaseProvisioning,
		},
		{
			name: "cluster infrastructure is ready and ControlPlaneEndpoint is set",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cluster",
				},
				Spec: clusterv1.ClusterSpec{
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						Name: "infra1",
					},
					ControlPlaneEndpoint: clusterv1.APIEndpoint{
						Host: "1.2.3.4",
						Port: 8443,
					},
				},
				Status: clusterv1.ClusterStatus{
					Initialization: clusterv1.ClusterInitializationStatus{InfrastructureProvisioned: ptr.To(true)},
				},
			},

			wantPhase: clusterv1.ClusterPhaseProvisioned,
		},
		{
			name: "no cluster infrastructure, control plane ready and ControlPlaneEndpoint is set",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cluster",
				},
				Spec: clusterv1.ClusterSpec{
					ControlPlaneEndpoint: clusterv1.APIEndpoint{ // This is set by the control plane ref controller when the cluster endpoint is available.
						Host: "1.2.3.4",
						Port: 8443,
					},
					ControlPlaneRef: clusterv1.ContractVersionedObjectReference{
						Name: "cp1",
					},
				},
				Status: clusterv1.ClusterStatus{
					Initialization: clusterv1.ClusterInitializationStatus{InfrastructureProvisioned: ptr.To(true)}, // Note, this is automatically set when there is no cluster infrastructure (no-op).
				},
			},

			wantPhase: clusterv1.ClusterPhaseProvisioned,
		},
		{
			name: "cluster has deletion timestamp",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-cluster",
					DeletionTimestamp: &metav1.Time{Time: time.Now().UTC()},
					Finalizers:        []string{clusterv1.ClusterFinalizer},
				},
				Status: clusterv1.ClusterStatus{
					Initialization: clusterv1.ClusterInitializationStatus{InfrastructureProvisioned: ptr.To(true)},
				},
				Spec: clusterv1.ClusterSpec{
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						Name: "infra1",
					},
				},
			},

			wantPhase: clusterv1.ClusterPhaseDeleting,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			setPhase(ctx, tt.cluster)
			g.Expect(tt.cluster.Status.GetTypedPhase()).To(Equal(tt.wantPhase))
		})
	}
}

func TestSetControlPlaneReplicas(t *testing.T) {
	tests := []struct {
		name                    string
		cluster                 *clusterv1.Cluster
		controlPlane            *unstructured.Unstructured
		controlPlaneIsNotFound  bool
		machines                collections.Machines
		getDescendantsSucceeded bool
		expectDesiredReplicas   *int32
		expectReplicas          *int32
		expectReadyReplicas     *int32
		expectAvailableReplicas *int32
		expectUpToDateReplicas  *int32
		expectVersions          []clusterv1.StatusVersion
	}{
		{
			name:                   "counters should be nil for a cluster with control plane, control plane does not exist",
			cluster:                fakeCluster("c", controlPlaneRef{Name: "cp"}),
			controlPlane:           nil,
			controlPlaneIsNotFound: true,
		},
		{
			name:                   "counters should be nil for a cluster with control plane, unexpected error reading the CP",
			cluster:                fakeCluster("c", controlPlaneRef{Name: "cp"}),
			controlPlane:           nil,
			controlPlaneIsNotFound: false,
		},
		{
			name:         "counters should be nil for a cluster with control plane, but not reporting counters",
			cluster:      fakeCluster("c", controlPlaneRef{Name: "cp"}),
			controlPlane: fakeControlPlane("cp"),
		},
		{
			name:                    "set counters for cluster with control plane, reporting counters",
			cluster:                 fakeCluster("c", controlPlaneRef{Name: "cp"}),
			controlPlane:            fakeControlPlane("cp", desiredReplicas(3), currentReplicas(2), readyReplicas(1), availableReplicas(2), upToDateReplicas(0), statusVersions{{Version: "v1.31.1", Replicas: ptr.To[int32](2)}, {Version: "v1.32.0", Replicas: ptr.To[int32](1)}}),
			expectDesiredReplicas:   ptr.To(int32(3)),
			expectReplicas:          ptr.To(int32(2)),
			expectReadyReplicas:     ptr.To(int32(1)),
			expectAvailableReplicas: ptr.To(int32(2)),
			expectUpToDateReplicas:  ptr.To(int32(0)),
			expectVersions:          []clusterv1.StatusVersion{{Version: "v1.31.1", Replicas: ptr.To[int32](2)}, {Version: "v1.32.0", Replicas: ptr.To[int32](1)}},
		},
		{
			name:                    "counters should be nil for a cluster with stand alone CP machines, failed to read descendants",
			cluster:                 fakeCluster("c"),
			getDescendantsSucceeded: false,
		},
		{
			name:                    "cluster with stand alone CP machines, no machines yet",
			cluster:                 fakeCluster("c"),
			getDescendantsSucceeded: true,
		},
		{
			name:                    "cluster with stand alone CP machines, with machines",
			cluster:                 fakeCluster("c"),
			getDescendantsSucceeded: true,
			machines: collections.FromMachines(
				fakeMachine("cp1", condition{Type: clusterv1.MachineAvailableCondition, Status: metav1.ConditionTrue}, condition{Type: clusterv1.MachineReadyCondition, Status: metav1.ConditionTrue}, condition{Type: clusterv1.MachineUpToDateCondition, Status: metav1.ConditionTrue}),
				fakeMachine("cp2", condition{Type: clusterv1.MachineAvailableCondition, Status: metav1.ConditionFalse}, condition{Type: clusterv1.MachineReadyCondition, Status: metav1.ConditionTrue}, condition{Type: clusterv1.MachineUpToDateCondition, Status: metav1.ConditionTrue}),
				fakeMachine("cp3", condition{Type: clusterv1.MachineAvailableCondition, Status: metav1.ConditionFalse}, condition{Type: clusterv1.MachineReadyCondition, Status: metav1.ConditionFalse}, condition{Type: clusterv1.MachineUpToDateCondition, Status: metav1.ConditionTrue}),
				fakeMachine("cp4"),
				fakeMachine("cp5"),
			),
			expectDesiredReplicas:   ptr.To(int32(5)),
			expectReplicas:          ptr.To(int32(5)),
			expectReadyReplicas:     ptr.To(int32(2)),
			expectAvailableReplicas: ptr.To(int32(1)),
			expectUpToDateReplicas:  ptr.To(int32(3)),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			setControlPlaneReplicas(ctx, tt.cluster, tt.controlPlane, contract.Version, tt.machines, tt.controlPlaneIsNotFound, tt.getDescendantsSucceeded)

			g.Expect(tt.cluster.Status.ControlPlane).ToNot(BeNil())
			g.Expect(tt.cluster.Status.ControlPlane.DesiredReplicas).To(Equal(tt.expectDesiredReplicas))
			g.Expect(tt.cluster.Status.ControlPlane.Replicas).To(Equal(tt.expectReplicas))
			g.Expect(tt.cluster.Status.ControlPlane.ReadyReplicas).To(Equal(tt.expectReadyReplicas))
			g.Expect(tt.cluster.Status.ControlPlane.AvailableReplicas).To(Equal(tt.expectAvailableReplicas))
			g.Expect(tt.cluster.Status.ControlPlane.UpToDateReplicas).To(Equal(tt.expectUpToDateReplicas))
			g.Expect(tt.cluster.Status.ControlPlane.Versions).To(Equal(tt.expectVersions))
		})
	}
}

func TestSetWorkersReplicas(t *testing.T) {
	tests := []struct {
		name                    string
		cluster                 *clusterv1.Cluster
		machinePools            clusterv1.MachinePoolList
		machineDeployments      clusterv1.MachineDeploymentList
		machineSets             clusterv1.MachineSetList
		workerMachines          collections.Machines
		getDescendantsSucceeded bool
		expectDesiredReplicas   *int32
		expectReplicas          *int32
		expectReadyReplicas     *int32
		expectAvailableReplicas *int32
		expectUpToDateReplicas  *int32
		expectVersions          []clusterv1.StatusVersion
	}{
		{
			name:                    "counters should be nil if failed to get descendants",
			cluster:                 fakeCluster("c", controlPlaneRef{Name: "cp"}),
			getDescendantsSucceeded: false,
		},
		{
			name:                    "counter should be nil there are no objects to count",
			cluster:                 fakeCluster("c", controlPlaneRef{Name: "cp"}),
			getDescendantsSucceeded: true,
		},
		{
			name:    "counter should be nil if descendants are not reporting counters",
			cluster: fakeCluster("c", controlPlaneRef{Name: "cp"}),
			machinePools: clusterv1.MachinePoolList{Items: []clusterv1.MachinePool{
				*fakeMachinePool("mp1"),
			}},
			machineDeployments: clusterv1.MachineDeploymentList{Items: []clusterv1.MachineDeployment{
				*fakeMachineDeployment("md1"),
			}},
			machineSets: clusterv1.MachineSetList{Items: []clusterv1.MachineSet{
				*fakeMachineSet("ms1", OwnedByCluster("c")),
				*fakeMachineSet("ms2"), // not owned by the cluster
			}},
			workerMachines: collections.FromMachines( //
				fakeMachine("m1"), // not owned by the cluster
			),
			getDescendantsSucceeded: true,
			expectReplicas:          nil,
		},
		{
			name:    "should count workers from different objects",
			cluster: fakeCluster("c", controlPlaneRef{Name: "cp"}),
			machinePools: clusterv1.MachinePoolList{Items: []clusterv1.MachinePool{
				*fakeMachinePool("mp1", desiredReplicas(1), currentReplicas(2), readyReplicas(3), availableReplicas(4), upToDateReplicas(5), statusVersions{{Version: "v1.31.0", Replicas: ptr.To[int32](2)}}),
			}},
			machineDeployments: clusterv1.MachineDeploymentList{Items: []clusterv1.MachineDeployment{
				*fakeMachineDeployment("md1", desiredReplicas(11), currentReplicas(12), readyReplicas(13), availableReplicas(14), upToDateReplicas(15), statusVersions{{Version: "v1.31.0", Replicas: ptr.To[int32](3)}, {Version: "v1.32.0", Replicas: ptr.To[int32](9)}}),
			}},
			machineSets: clusterv1.MachineSetList{Items: []clusterv1.MachineSet{
				*fakeMachineSet("ms1", OwnedByCluster("c"), desiredReplicas(21), currentReplicas(22), readyReplicas(23), availableReplicas(24), upToDateReplicas(25), statusVersions{{Version: "v1.32.0", Replicas: ptr.To[int32](22)}}),
				*fakeMachineSet("ms2", desiredReplicas(31), currentReplicas(32), readyReplicas(33), availableReplicas(34), upToDateReplicas(35), statusVersions{{Version: "v1.30.0", Replicas: ptr.To[int32](32)}}), // not owned by the cluster
			}},
			workerMachines: collections.FromMachines( // 4 replicas, 2 Ready, 3 Available, 1 UpToDate
				fakeMachine("m1", OwnedByCluster("c"), kubeletVersion("v1.31.0"), condition{Type: clusterv1.MachineAvailableCondition, Status: metav1.ConditionTrue}, condition{Type: clusterv1.MachineReadyCondition, Status: metav1.ConditionTrue}, condition{Type: clusterv1.MachineUpToDateCondition, Status: metav1.ConditionTrue}),
				fakeMachine("m2", OwnedByCluster("c"), kubeletVersion("v1.33.0"), condition{Type: clusterv1.MachineAvailableCondition, Status: metav1.ConditionTrue}, condition{Type: clusterv1.MachineReadyCondition, Status: metav1.ConditionFalse}, condition{Type: clusterv1.MachineUpToDateCondition, Status: metav1.ConditionFalse}),
				fakeMachine("m3", OwnedByCluster("c"), kubeletVersion("v1.33.0"), condition{Type: clusterv1.MachineAvailableCondition, Status: metav1.ConditionTrue}, condition{Type: clusterv1.MachineReadyCondition, Status: metav1.ConditionTrue}, condition{Type: clusterv1.MachineUpToDateCondition, Status: metav1.ConditionFalse}),
				fakeMachine("m4", OwnedByCluster("c")),
				fakeMachine("m5", kubeletVersion("v1.29.0")), // not owned by the cluster
			),
			getDescendantsSucceeded: true,
			expectDesiredReplicas:   ptr.To(int32(37)),
			expectReplicas:          ptr.To(int32(40)),
			expectReadyReplicas:     ptr.To(int32(41)),
			expectAvailableReplicas: ptr.To(int32(45)),
			expectUpToDateReplicas:  ptr.To(int32(46)),
			expectVersions: []clusterv1.StatusVersion{
				{Version: "v1.31.0", Replicas: ptr.To[int32](6)},
				{Version: "v1.32.0", Replicas: ptr.To[int32](31)},
				{Version: "v1.33.0", Replicas: ptr.To[int32](2)},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			setWorkersReplicas(ctx, tt.cluster, tt.machinePools, tt.machineDeployments, tt.machineSets, tt.workerMachines, tt.getDescendantsSucceeded)

			g.Expect(tt.cluster.Status.Workers).ToNot(BeNil())
			g.Expect(tt.cluster.Status.Workers.DesiredReplicas).To(Equal(tt.expectDesiredReplicas))
			g.Expect(tt.cluster.Status.Workers.Replicas).To(Equal(tt.expectReplicas))
			g.Expect(tt.cluster.Status.Workers.ReadyReplicas).To(Equal(tt.expectReadyReplicas))
			g.Expect(tt.cluster.Status.Workers.AvailableReplicas).To(Equal(tt.expectAvailableReplicas))
			g.Expect(tt.cluster.Status.Workers.UpToDateReplicas).To(Equal(tt.expectUpToDateReplicas))
			g.Expect(tt.cluster.Status.Workers.Versions).To(Equal(tt.expectVersions))
		})
	}
}

func TestSetRemoteConnectionProbeCondition(t *testing.T) {
	now := time.Now()
	remoteConnectionGracePeriod := 5 * time.Minute

	testCases := []struct {
		name                string
		cluster             *clusterv1.Cluster
		healthCheckingState clustercache.HealthCheckingState
		expectCondition     metav1.Condition
	}{
		{
			name:    "connection down, did not try to connect yet",
			cluster: fakeCluster("c"),
			healthCheckingState: clustercache.HealthCheckingState{
				LastProbeTime:        time.Time{},
				LastProbeSuccessTime: time.Time{},
				ConsecutiveFailures:  0,
			},
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterRemoteConnectionProbeCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterRemoteConnectionProbeFailedReason,
				Message: "Remote connection not established yet",
			},
		},
		{
			name: "connection down, did not try to connect yet (preserve existing condition)",
			cluster: func() *clusterv1.Cluster {
				c := fakeCluster("c")
				conditions.Set(c, metav1.Condition{
					Type:   clusterv1.ClusterRemoteConnectionProbeCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.ClusterRemoteConnectionProbeSucceededReason,
				})
				return c
			}(),
			healthCheckingState: clustercache.HealthCheckingState{
				LastProbeTime:        time.Time{},
				LastProbeSuccessTime: time.Time{},
				ConsecutiveFailures:  0,
			},
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterRemoteConnectionProbeCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterRemoteConnectionProbeSucceededReason,
			},
		},
		{
			name:    "connection down, tried to connect, but failed",
			cluster: fakeCluster("c"),
			healthCheckingState: clustercache.HealthCheckingState{
				LastProbeTime:        time.Now(),
				LastProbeSuccessTime: time.Time{},
				ConsecutiveFailures:  4,
			},
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterRemoteConnectionProbeCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterRemoteConnectionProbeFailedReason,
				Message: "Remote connection not established yet",
			},
		},
		{
			name:    "connection down, tried to connect, but failed >=5 times",
			cluster: fakeCluster("c"),
			healthCheckingState: clustercache.HealthCheckingState{
				LastProbeTime:        time.Now(),
				LastProbeSuccessTime: time.Time{},
				ConsecutiveFailures:  5,
			},
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterRemoteConnectionProbeCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterRemoteConnectionProbeFailedReason,
				Message: "Remote connection probe failed",
			},
		},
		{
			name:    "connection down, last probe success more than remote connection grace period ago",
			cluster: fakeCluster("c"),
			healthCheckingState: clustercache.HealthCheckingState{
				LastProbeTime:        time.Now(),
				LastProbeSuccessTime: now.Add(-remoteConnectionGracePeriod - time.Second),
				ConsecutiveFailures:  2,
			},
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterRemoteConnectionProbeCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterRemoteConnectionProbeFailedReason,
				Message: fmt.Sprintf("Remote connection probe failed, probe last succeeded at %s", (now.Add(-remoteConnectionGracePeriod - time.Second)).Format(time.RFC3339)),
			},
		},
		{
			name:    "connection up, last probe succeeded within remote connection grace period",
			cluster: fakeCluster("c"),
			healthCheckingState: clustercache.HealthCheckingState{
				LastProbeTime:        time.Now(),
				LastProbeSuccessTime: now.Add(-remoteConnectionGracePeriod + time.Second),
				ConsecutiveFailures:  2,
			},
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterRemoteConnectionProbeCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterRemoteConnectionProbeSucceededReason,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			setRemoteConnectionProbeCondition(ctx, tc.cluster, tc.healthCheckingState, remoteConnectionGracePeriod)

			condition := conditions.Get(tc.cluster, clusterv1.ClusterRemoteConnectionProbeCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tc.expectCondition, conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func TestSetInfrastructureReadyCondition(t *testing.T) {
	testCases := []struct {
		name                   string
		cluster                *clusterv1.Cluster
		infraCluster           *unstructured.Unstructured
		infraClusterIsNotFound bool
		expectCondition        metav1.Condition
	}{
		{
			name:                   "topology not yet reconcile",
			cluster:                fakeCluster("c", topology(true)),
			infraCluster:           nil,
			infraClusterIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterInfrastructureReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterInfrastructureDoesNotExistReason,
				Message: "Waiting for cluster topology to be reconciled",
			},
		},
		{
			name:                   "topology not yet reconcile, cluster deleted",
			cluster:                fakeCluster("c", topology(true), deleted(true)),
			infraCluster:           nil,
			infraClusterIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterInfrastructureReadyCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.ClusterInfrastructureDoesNotExistReason,
			},
		},
		{
			name:                   "mirror Ready condition from infra cluster",
			cluster:                fakeCluster("c", infrastructureRef{Kind: "FakeInfraCluster"}),
			infraCluster:           fakeInfraCluster("i1", condition{Type: "Ready", Status: "False", Message: "some message", Reason: "SomeReason"}),
			infraClusterIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterInfrastructureReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  "SomeReason",
				Message: "some message",
			},
		},
		{
			name:                   "mirror Ready condition from infra cluster (true)",
			cluster:                fakeCluster("c", infrastructureRef{Kind: "FakeInfraCluster"}),
			infraCluster:           fakeInfraCluster("i1", condition{Type: "Ready", Status: "True", Message: "some message"}), // reason not set for v1beta1 conditions
			infraClusterIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterInfrastructureReadyCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.ClusterInfrastructureReadyReason, // reason fixed up
				Message: "some message",
			},
		},
		{
			name:                   "Use status.InfrastructureReady flag as a fallback Ready condition from infra cluster is missing",
			cluster:                fakeCluster("c", infrastructureRef{Kind: "FakeInfraCluster"}),
			infraCluster:           fakeInfraCluster("i1", provisioned(false)),
			infraClusterIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterInfrastructureReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterInfrastructureNotReadyReason,
				Message: "FakeInfraCluster status.initialization.provisioned is false",
			},
		},
		{
			name:                   "Use status.InfrastructureReady flag as a fallback Ready condition from infra cluster is missing (ready true)",
			cluster:                fakeCluster("c", infrastructureRef{Kind: "FakeInfraCluster"}, infrastructureProvisioned(true)),
			infraCluster:           fakeInfraCluster("i1", provisioned(true)),
			infraClusterIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterInfrastructureReadyCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterInfrastructureReadyReason,
			},
		},
		{
			name:                   "invalid Ready condition from infra cluster",
			cluster:                fakeCluster("c", infrastructureRef{Kind: "FakeInfraCluster"}),
			infraCluster:           fakeInfraCluster("i1", condition{Type: "Ready"}),
			infraClusterIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterInfrastructureReadyCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterInfrastructureInvalidConditionReportedReason,
				Message: "failed to convert status.conditions from FakeInfraCluster to []metav1.Condition: status must be set for the Ready condition",
			},
		},
		{
			name:                   "failed to get infra cluster",
			cluster:                fakeCluster("c", infrastructureRef{Kind: "FakeInfraCluster"}, deleted(true)),
			infraCluster:           nil,
			infraClusterIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterInfrastructureReadyCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterInfrastructureInternalErrorReason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:                   "infra cluster that was ready not found while cluster is deleting",
			cluster:                fakeCluster("c", infrastructureRef{Kind: "FakeInfraCluster"}, infrastructureProvisioned(true), deleted(true)),
			infraCluster:           nil,
			infraClusterIsNotFound: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterInfrastructureReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterInfrastructureDeletedReason,
				Message: "FakeInfraCluster has been deleted",
			},
		},
		{
			name:                   "infra cluster not found while cluster is deleting",
			cluster:                fakeCluster("c", infrastructureRef{Kind: "FakeInfraCluster"}, deleted(true)),
			infraCluster:           nil,
			infraClusterIsNotFound: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterInfrastructureReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterInfrastructureDoesNotExistReason,
				Message: "FakeInfraCluster does not exist",
			},
		},
		{
			name:                   "infra cluster not found after the cluster has been initialized",
			cluster:                fakeCluster("c", infrastructureRef{Kind: "FakeInfraCluster"}, infrastructureProvisioned(true)),
			infraCluster:           nil,
			infraClusterIsNotFound: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterInfrastructureReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterInfrastructureDeletedReason,
				Message: "FakeInfraCluster has been deleted while the cluster still exists",
			},
		},
		{
			name:                   "infra cluster not found",
			cluster:                fakeCluster("c", infrastructureRef{Kind: "FakeInfraCluster"}),
			infraCluster:           nil,
			infraClusterIsNotFound: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterInfrastructureReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterInfrastructureDoesNotExistReason,
				Message: "FakeInfraCluster does not exist",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			setInfrastructureReadyCondition(ctx, tc.cluster, tc.infraCluster, tc.infraClusterIsNotFound)

			condition := conditions.Get(tc.cluster, clusterv1.ClusterInfrastructureReadyCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tc.expectCondition, conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func TestSetControlPlaneAvailableCondition(t *testing.T) {
	testCases := []struct {
		name                   string
		cluster                *clusterv1.Cluster
		controlPlane           *unstructured.Unstructured
		controlPlaneIsNotFound bool
		expectCondition        metav1.Condition
	}{
		{
			name:                   "topology not yet reconcile",
			cluster:                fakeCluster("c", topology(true)),
			controlPlane:           nil,
			controlPlaneIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneAvailableCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterControlPlaneDoesNotExistReason,
				Message: "Waiting for cluster topology to be reconciled",
			},
		},
		{
			name:                   "topology not yet reconcile, cluster deleted",
			cluster:                fakeCluster("c", topology(true), deleted(true)),
			controlPlane:           nil,
			controlPlaneIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterControlPlaneAvailableCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.ClusterControlPlaneDoesNotExistReason,
			},
		},
		{
			name:                   "mirror Available condition from control plane",
			cluster:                fakeCluster("c", controlPlaneRef{Kind: "FakeControlPlane"}),
			controlPlane:           fakeControlPlane("cp1", condition{Type: "Available", Status: "False", Message: "some message", Reason: "SomeReason"}),
			controlPlaneIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneAvailableCondition,
				Status:  metav1.ConditionFalse,
				Reason:  "SomeReason",
				Message: "some message",
			},
		},
		{
			name:                   "mirror Available condition from control plane (true)",
			cluster:                fakeCluster("c", controlPlaneRef{Kind: "FakeControlPlane"}),
			controlPlane:           fakeControlPlane("cp1", condition{Type: "Available", Status: "True", Message: "some message"}), // reason not set for v1beta1 conditions
			controlPlaneIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneAvailableCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.ClusterControlPlaneAvailableReason, // reason fixed up
				Message: "some message",
			},
		},
		{
			name:                   "Use status.controlPlaneReady flag as a fallback Available condition from control plane is missing",
			cluster:                fakeCluster("c", controlPlaneRef{Kind: "FakeControlPlane"}),
			controlPlane:           fakeControlPlane("cp1", initialized(false)),
			controlPlaneIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneAvailableCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterControlPlaneNotAvailableReason,
				Message: "FakeControlPlane status.initialization.controlPlaneInitialized is false",
			},
		},
		{
			name:                   "Use status.controlPlaneReady flag as a fallback Available condition from control plane is missing (ready true)",
			cluster:                fakeCluster("c", controlPlaneRef{Kind: "FakeControlPlane"}, controlPlaneInitialized(true)),
			controlPlane:           fakeControlPlane("cp1", initialized(true)),
			controlPlaneIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterControlPlaneAvailableCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterControlPlaneAvailableReason,
			},
		},
		{
			name:                   "invalid Available condition from control plane",
			cluster:                fakeCluster("c", controlPlaneRef{Kind: "FakeControlPlane"}),
			controlPlane:           fakeControlPlane("cp1", condition{Type: "Available"}),
			controlPlaneIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneAvailableCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterControlPlaneInvalidConditionReportedReason,
				Message: "failed to convert status.conditions from FakeControlPlane to []metav1.Condition: status must be set for the Available condition",
			},
		},
		{
			name:                   "failed to get control planer",
			cluster:                fakeCluster("c", controlPlaneRef{Kind: "FakeControlPlane"}, deleted(true)),
			controlPlane:           nil,
			controlPlaneIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneAvailableCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterControlPlaneInternalErrorReason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:                   "control plane that was ready not found while cluster is deleting",
			cluster:                fakeCluster("c", controlPlaneRef{Kind: "FakeControlPlane"}, controlPlaneInitialized(true), deleted(true)),
			controlPlane:           nil,
			controlPlaneIsNotFound: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneAvailableCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterControlPlaneDeletedReason,
				Message: "FakeControlPlane has been deleted",
			},
		},
		{
			name:                   "control plane not found while cluster is deleting",
			cluster:                fakeCluster("c", controlPlaneRef{Kind: "FakeControlPlane"}, deleted(true)),
			controlPlane:           nil,
			controlPlaneIsNotFound: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneAvailableCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterControlPlaneDoesNotExistReason,
				Message: "FakeControlPlane does not exist",
			},
		},
		{
			name:                   "control plane not found after the cluster has been initialized",
			cluster:                fakeCluster("c", controlPlaneRef{Kind: "FakeControlPlane"}, controlPlaneInitialized(true)),
			controlPlane:           nil,
			controlPlaneIsNotFound: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneAvailableCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterControlPlaneDeletedReason,
				Message: "FakeControlPlane has been deleted while the cluster still exists",
			},
		},
		{
			name:                   "control plane not found",
			cluster:                fakeCluster("c", controlPlaneRef{Kind: "FakeControlPlane"}),
			controlPlane:           nil,
			controlPlaneIsNotFound: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneAvailableCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterControlPlaneDoesNotExistReason,
				Message: "FakeControlPlane does not exist",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			setControlPlaneAvailableCondition(ctx, tc.cluster, tc.controlPlane, tc.controlPlaneIsNotFound)

			condition := conditions.Get(tc.cluster, clusterv1.ClusterControlPlaneAvailableCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tc.expectCondition, conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func TestSetControlPlaneInitialized(t *testing.T) {
	tests := []struct {
		name                    string
		cluster                 *clusterv1.Cluster
		controlPlane            *unstructured.Unstructured
		controlPlaneIsNotFound  bool
		machines                collections.Machines
		getDescendantsSucceeded bool
		expectCondition         metav1.Condition
	}{
		{
			name:                   "topology not yet reconcile",
			cluster:                fakeCluster("c", topology(true)),
			controlPlane:           nil,
			controlPlaneIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneInitializedCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterControlPlaneDoesNotExistReason,
				Message: "Waiting for cluster topology to be reconciled",
			},
		},
		{
			name:                   "topology not yet reconcile, cluster deleted",
			cluster:                fakeCluster("c", topology(true), deleted(true)),
			controlPlane:           nil,
			controlPlaneIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterControlPlaneInitializedCondition,
				Status: metav1.ConditionUnknown,
				Reason: clusterv1.ClusterControlPlaneDoesNotExistReason,
			},
		},
		{
			name:                   "cluster with control plane, control plane does not exist",
			cluster:                fakeCluster("c", controlPlaneRef{Kind: "FakeControlPlane"}),
			controlPlane:           nil,
			controlPlaneIsNotFound: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneInitializedCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterControlPlaneDoesNotExistReason,
				Message: "FakeControlPlane does not exist",
			},
		},
		{
			name:                   "cluster with control plane, unexpected error reading the CP",
			cluster:                fakeCluster("c", controlPlaneRef{Kind: "FakeControlPlane"}),
			controlPlane:           nil,
			controlPlaneIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneInitializedCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterControlPlaneInitializedInternalErrorReason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:         "cluster with control plane, not yet initialized",
			cluster:      fakeCluster("c", controlPlaneRef{Kind: "FakeControlPlane"}),
			controlPlane: fakeControlPlane("cp"),
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneInitializedCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterControlPlaneNotInitializedReason,
				Message: "Control plane not yet initialized",
			},
		},
		{
			name:         "cluster with control plane, initialized",
			cluster:      fakeCluster("c", controlPlaneRef{Kind: "FakeControlPlane"}),
			controlPlane: fakeControlPlane("cp", initialized(true)),
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterControlPlaneInitializedCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterControlPlaneInitializedReason,
			},
		},
		{
			name:                    "cluster with stand alone CP machines, failed to read descendants",
			cluster:                 fakeCluster("c"),
			getDescendantsSucceeded: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneInitializedCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterControlPlaneInitializedInternalErrorReason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:                    "cluster with stand alone CP machines, no machines with a node yet",
			cluster:                 fakeCluster("c"),
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneInitializedCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterControlPlaneNotInitializedReason,
				Message: "Waiting for the first control plane machine to have status.nodeRef set",
			},
		},
		{
			name:                    "cluster with stand alone CP machines, at least one machine with a node",
			cluster:                 fakeCluster("c"),
			getDescendantsSucceeded: true,
			machines: collections.FromMachines(
				fakeMachine("cp1", nodeRef{Name: "Foo"}),
			),
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterControlPlaneInitializedCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterControlPlaneInitializedReason,
			},
		},
		{
			name:         "initialized never flips back to false",
			cluster:      fakeCluster("c", controlPlaneRef{Kind: "FakeControlPlane"}, condition{Type: clusterv1.ClusterControlPlaneInitializedCondition, Status: metav1.ConditionTrue, Reason: clusterv1.ClusterControlPlaneInitializedReason}),
			controlPlane: fakeControlPlane("cp", initialized(false)),
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterControlPlaneInitializedCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterControlPlaneInitializedReason,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			setControlPlaneInitializedCondition(ctx, tt.cluster, tt.controlPlane, "v1beta2", tt.machines, tt.controlPlaneIsNotFound, tt.getDescendantsSucceeded)

			condition := conditions.Get(tt.cluster, clusterv1.ClusterControlPlaneInitializedCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tt.expectCondition, conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func TestSetWorkersAvailableCondition(t *testing.T) {
	tests := []struct {
		name                    string
		cluster                 *clusterv1.Cluster
		machinePools            clusterv1.MachinePoolList
		machineDeployments      clusterv1.MachineDeploymentList
		getDescendantsSucceeded bool
		expectCondition         metav1.Condition
	}{
		{
			name:                    "unknown if failed to get descendants",
			cluster:                 fakeCluster("c", controlPlaneRef{Name: "cp"}),
			getDescendantsSucceeded: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterWorkersAvailableCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterWorkersAvailableInternalErrorReason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:                    "true if no descendants",
			cluster:                 fakeCluster("c", controlPlaneRef{Name: "cp"}),
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterWorkersAvailableCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterWorkersAvailableNoWorkersReason,
			},
		},
		{
			name:    "descendants do not report available",
			cluster: fakeCluster("c", controlPlaneRef{Name: "cp"}),
			machinePools: clusterv1.MachinePoolList{Items: []clusterv1.MachinePool{
				*fakeMachinePool("mp1"),
			}},
			machineDeployments: clusterv1.MachineDeploymentList{Items: []clusterv1.MachineDeployment{
				*fakeMachineDeployment("md1"),
			}},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterWorkersAvailableCondition,
				Status: metav1.ConditionUnknown,
				Reason: clusterv1.ClusterWorkersAvailableUnknownReason,
				Message: "* MachineDeployment md1: Condition Available not yet reported\n" +
					"* MachinePool mp1: Condition Available not yet reported",
			},
		},
		{
			name:    "descendants report available",
			cluster: fakeCluster("c", controlPlaneRef{Name: "cp"}),
			machinePools: clusterv1.MachinePoolList{Items: []clusterv1.MachinePool{
				*fakeMachinePool("mp1", condition{
					Type:    clusterv1.MachineDeploymentAvailableCondition,
					Status:  metav1.ConditionFalse,
					Reason:  "Foo",
					Message: "2 available replicas, at least 3 required (spec.strategy.rollout.maxUnavailable is 1, spec.replicas is 4)",
				}),
			}},
			machineDeployments: clusterv1.MachineDeploymentList{Items: []clusterv1.MachineDeployment{
				*fakeMachineDeployment("md1", condition{
					Type:    clusterv1.MachineDeploymentAvailableCondition,
					Status:  metav1.ConditionFalse,
					Reason:  "Foo",
					Message: "3 available replicas, at least 4 required (spec.strategy.rollout.maxUnavailable is 1, spec.replicas is 5)",
				}),
			}},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterWorkersAvailableCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.ClusterWorkersNotAvailableReason,
				Message: "* MachineDeployment md1: 3 available replicas, at least 4 required (spec.strategy.rollout.maxUnavailable is 1, spec.replicas is 5)\n" +
					"* MachinePool mp1: 2 available replicas, at least 3 required (spec.strategy.rollout.maxUnavailable is 1, spec.replicas is 4)",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			setWorkersAvailableCondition(ctx, tt.cluster, tt.machinePools, tt.machineDeployments, tt.getDescendantsSucceeded)

			condition := conditions.Get(tt.cluster, clusterv1.ClusterWorkersAvailableCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tt.expectCondition, conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func TestSetControlPlaneMachinesReadyCondition(t *testing.T) {
	readyCondition := metav1.Condition{
		Type:   clusterv1.MachineReadyCondition,
		Status: metav1.ConditionTrue,
		Reason: clusterv1.MachineReadyReason,
	}

	tests := []struct {
		name                    string
		cluster                 *clusterv1.Cluster
		machines                []*clusterv1.Machine
		getDescendantsSucceeded bool
		expectCondition         metav1.Condition
	}{
		{
			name:                    "get descendant failed",
			cluster:                 fakeCluster("c"),
			machines:                nil,
			getDescendantsSucceeded: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneMachinesReadyCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterControlPlaneMachinesReadyInternalErrorReason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:                    "no machines",
			cluster:                 fakeCluster("c"),
			machines:                []*clusterv1.Machine{},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterControlPlaneMachinesReadyCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterControlPlaneMachinesReadyNoReplicasReason,
			},
		},
		{
			name:    "all machines are ready",
			cluster: fakeCluster("c"),
			machines: []*clusterv1.Machine{
				fakeMachine("machine-1", controlPlane(true), condition(readyCondition)),
				fakeMachine("machine-2", controlPlane(true), condition(readyCondition)),
			},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterControlPlaneMachinesReadyCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterControlPlaneMachinesReadyReason,
			},
		},
		{
			name:    "one ready, one has nothing reported",
			cluster: fakeCluster("c"),
			machines: []*clusterv1.Machine{
				fakeMachine("machine-1", controlPlane(true), condition(readyCondition)),
				fakeMachine("machine-2", controlPlane(true)),
			},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneMachinesReadyCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterControlPlaneMachinesReadyUnknownReason,
				Message: "* Machine machine-2: Condition Ready not yet reported",
			},
		},
		{
			name:    "one ready, one reporting not ready, one reporting unknown, one reporting deleting",
			cluster: fakeCluster("c"),
			machines: []*clusterv1.Machine{
				fakeMachine("machine-1", controlPlane(true), condition(readyCondition)),
				fakeMachine("machine-2", controlPlane(true), condition(metav1.Condition{
					Type:    clusterv1.MachineReadyCondition,
					Status:  metav1.ConditionFalse,
					Reason:  "SomeReason",
					Message: "HealthCheckSucceeded: Some message",
				})),
				fakeMachine("machine-3", controlPlane(true), condition(metav1.Condition{
					Type:    clusterv1.MachineReadyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  "SomeUnknownReason",
					Message: "Some unknown message",
				})),
				fakeMachine("machine-4", controlPlane(true), condition(metav1.Condition{
					Type:    clusterv1.MachineReadyCondition,
					Status:  metav1.ConditionFalse,
					Reason:  clusterv1.MachineDeletingReason,
					Message: "Deleting: Machine deletion in progress, stage: DrainingNode",
				})),
			},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterControlPlaneMachinesReadyCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.ClusterControlPlaneMachinesNotReadyReason,
				Message: "* Machine machine-2: HealthCheckSucceeded: Some message\n" +
					"* Machine machine-4: Deleting: Machine deletion in progress, stage: DrainingNode\n" +
					"* Machine machine-3: Some unknown message",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			var machines collections.Machines
			if tt.machines != nil {
				machines = collections.FromMachines(tt.machines...)
			}
			setControlPlaneMachinesReadyCondition(ctx, tt.cluster, machines, tt.getDescendantsSucceeded)

			condition := conditions.Get(tt.cluster, clusterv1.ClusterControlPlaneMachinesReadyCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tt.expectCondition, conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func TestSetWorkerMachinesReadyCondition(t *testing.T) {
	readyCondition := metav1.Condition{
		Type:   clusterv1.MachineReadyCondition,
		Status: metav1.ConditionTrue,
		Reason: clusterv1.MachineReadyReason,
	}

	tests := []struct {
		name                    string
		cluster                 *clusterv1.Cluster
		machines                []*clusterv1.Machine
		getDescendantsSucceeded bool
		expectCondition         metav1.Condition
	}{
		{
			name:                    "get descendant failed",
			cluster:                 fakeCluster("c"),
			machines:                nil,
			getDescendantsSucceeded: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterWorkerMachinesReadyCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterWorkerMachinesReadyInternalErrorReason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:                    "no machines",
			cluster:                 fakeCluster("c"),
			machines:                []*clusterv1.Machine{},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterWorkerMachinesReadyCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterWorkerMachinesReadyNoReplicasReason,
			},
		},
		{
			name:    "all machines are ready",
			cluster: fakeCluster("c"),
			machines: []*clusterv1.Machine{
				fakeMachine("machine-1", condition(readyCondition)),
				fakeMachine("machine-2", condition(readyCondition)),
			},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterWorkerMachinesReadyCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterWorkerMachinesReadyReason,
			},
		},
		{
			name:    "one ready, one has nothing reported",
			cluster: fakeCluster("c"),
			machines: []*clusterv1.Machine{
				fakeMachine("machine-1", condition(readyCondition)),
				fakeMachine("machine-2"),
			},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterWorkerMachinesReadyCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterWorkerMachinesReadyUnknownReason,
				Message: "* Machine machine-2: Condition Ready not yet reported",
			},
		},
		{
			name:    "one ready, one reporting not ready, one reporting unknown, one reporting deleting",
			cluster: fakeCluster("c"),
			machines: []*clusterv1.Machine{
				fakeMachine("machine-1", condition(readyCondition)),
				fakeMachine("machine-2", condition(metav1.Condition{
					Type:    clusterv1.MachineReadyCondition,
					Status:  metav1.ConditionFalse,
					Reason:  "SomeReason",
					Message: "HealthCheckSucceeded: Some message",
				})),
				fakeMachine("machine-3", condition(metav1.Condition{
					Type:    clusterv1.MachineReadyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  "SomeUnknownReason",
					Message: "Some unknown message",
				})),
				fakeMachine("machine-4", condition(metav1.Condition{
					Type:    clusterv1.MachineReadyCondition,
					Status:  metav1.ConditionFalse,
					Reason:  clusterv1.MachineDeletingReason,
					Message: "Deleting: Machine deletion in progress, stage: DrainingNode",
				})),
			},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterWorkerMachinesReadyCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.ClusterWorkerMachinesNotReadyReason,
				Message: "* Machine machine-2: HealthCheckSucceeded: Some message\n" +
					"* Machine machine-4: Deleting: Machine deletion in progress, stage: DrainingNode\n" +
					"* Machine machine-3: Some unknown message",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			var machines collections.Machines
			if tt.machines != nil {
				machines = collections.FromMachines(tt.machines...)
			}
			setWorkerMachinesReadyCondition(ctx, tt.cluster, machines, tt.getDescendantsSucceeded)

			condition := conditions.Get(tt.cluster, clusterv1.ClusterWorkerMachinesReadyCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tt.expectCondition, conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func TestSetControlPlaneMachinesUpToDateCondition(t *testing.T) {
	tests := []struct {
		name                    string
		cluster                 *clusterv1.Cluster
		machines                []*clusterv1.Machine
		getDescendantsSucceeded bool
		expectCondition         metav1.Condition
	}{
		{
			name:                    "get descendant failed",
			cluster:                 fakeCluster("c"),
			machines:                nil,
			getDescendantsSucceeded: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneMachinesUpToDateCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterControlPlaneMachinesUpToDateInternalErrorReason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:                    "no machines",
			cluster:                 fakeCluster("c"),
			machines:                []*clusterv1.Machine{},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneMachinesUpToDateCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.ClusterControlPlaneMachinesUpToDateNoReplicasReason,
				Message: "",
			},
		},
		{
			name:    "One machine up-to-date",
			cluster: fakeCluster("c"),
			machines: []*clusterv1.Machine{
				fakeMachine("up-to-date-1", controlPlane(true), condition(metav1.Condition{
					Type:   clusterv1.MachineUpToDateCondition,
					Status: metav1.ConditionTrue,
					Reason: "some-reason-1",
				})),
			},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneMachinesUpToDateCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.ClusterControlPlaneMachinesUpToDateReason,
				Message: "",
			},
		},
		{
			name:    "One machine unknown",
			cluster: fakeCluster("c"),
			machines: []*clusterv1.Machine{
				fakeMachine("unknown-1", controlPlane(true), condition(metav1.Condition{
					Type:    clusterv1.MachineUpToDateCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  "some-unknown-reason-1",
					Message: "some unknown message",
				})),
			},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneMachinesUpToDateCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterControlPlaneMachinesUpToDateUnknownReason,
				Message: "* Machine unknown-1: some unknown message",
			},
		},
		{
			name:    "One machine not up-to-date",
			cluster: fakeCluster("c"),
			machines: []*clusterv1.Machine{
				fakeMachine("not-up-to-date-machine-1", controlPlane(true), condition(metav1.Condition{
					Type:    clusterv1.MachineUpToDateCondition,
					Status:  metav1.ConditionFalse,
					Reason:  "some-not-up-to-date-reason",
					Message: "some not up-to-date message",
				})),
			},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneMachinesUpToDateCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterControlPlaneMachinesNotUpToDateReason,
				Message: "* Machine not-up-to-date-machine-1: some not up-to-date message",
			},
		},
		{
			name:    "One machine without up-to-date condition, one new Machines without up-to-date condition",
			cluster: fakeCluster("c"),
			machines: []*clusterv1.Machine{
				fakeMachine("no-condition-machine-1", controlPlane(true)),
				fakeMachine("no-condition-machine-2-new", controlPlane(true), creationTimestamp{Time: time.Now().Add(-5 * time.Second)}), // ignored because it's new
			},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneMachinesUpToDateCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterControlPlaneMachinesUpToDateUnknownReason,
				Message: "* Machine no-condition-machine-1: Condition UpToDate not yet reported",
			},
		},
		{
			name:    "Two machines not up-to-date, two up-to-date, two not reported",
			cluster: fakeCluster("c"),
			machines: []*clusterv1.Machine{
				fakeMachine("up-to-date-1", controlPlane(true), condition(metav1.Condition{
					Type:   clusterv1.MachineUpToDateCondition,
					Status: metav1.ConditionTrue,
					Reason: "TestUpToDate",
				})),
				fakeMachine("up-to-date-2", controlPlane(true), condition(metav1.Condition{
					Type:   clusterv1.MachineUpToDateCondition,
					Status: metav1.ConditionTrue,
					Reason: "TestUpToDate",
				})),
				fakeMachine("not-up-to-date-machine-1", controlPlane(true), condition(metav1.Condition{
					Type:    clusterv1.MachineUpToDateCondition,
					Status:  metav1.ConditionFalse,
					Reason:  "TestNotUpToDate",
					Message: "This is not up-to-date message",
				})),
				fakeMachine("not-up-to-date-machine-2", controlPlane(true), condition(metav1.Condition{
					Type:    clusterv1.MachineUpToDateCondition,
					Status:  metav1.ConditionFalse,
					Reason:  "TestNotUpToDate",
					Message: "This is not up-to-date message",
				})),
				fakeMachine("no-condition-machine-1", controlPlane(true)),
				fakeMachine("no-condition-machine-2", controlPlane(true)),
			},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterControlPlaneMachinesUpToDateCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.ClusterControlPlaneMachinesNotUpToDateReason,
				Message: "* Machines not-up-to-date-machine-1, not-up-to-date-machine-2: This is not up-to-date message\n" +
					"* Machines no-condition-machine-1, no-condition-machine-2: Condition UpToDate not yet reported",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			var machines collections.Machines
			if tt.machines != nil {
				machines = collections.FromMachines(tt.machines...)
			}
			setControlPlaneMachinesUpToDateCondition(ctx, tt.cluster, machines, tt.getDescendantsSucceeded)

			condition := conditions.Get(tt.cluster, clusterv1.ClusterControlPlaneMachinesUpToDateCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tt.expectCondition, conditions.IgnoreLastTransitionTime(true)))
		})
	}
}
func TestSetWorkerMachinesUpToDateCondition(t *testing.T) {
	tests := []struct {
		name                    string
		cluster                 *clusterv1.Cluster
		machines                []*clusterv1.Machine
		getDescendantsSucceeded bool
		expectCondition         metav1.Condition
	}{
		{
			name:                    "get descendant failed",
			cluster:                 fakeCluster("c"),
			machines:                nil,
			getDescendantsSucceeded: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterWorkerMachinesUpToDateCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterWorkerMachinesUpToDateInternalErrorReason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:                    "no machines",
			cluster:                 fakeCluster("c"),
			machines:                []*clusterv1.Machine{},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterWorkerMachinesUpToDateCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.ClusterWorkerMachinesUpToDateNoReplicasReason,
				Message: "",
			},
		},
		{
			name:    "One machine up-to-date",
			cluster: fakeCluster("c"),
			machines: []*clusterv1.Machine{
				fakeMachine("up-to-date-1", condition(metav1.Condition{
					Type:   clusterv1.MachineUpToDateCondition,
					Status: metav1.ConditionTrue,
					Reason: "some-reason-1",
				})),
			},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterWorkerMachinesUpToDateCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.ClusterWorkerMachinesUpToDateReason,
				Message: "",
			},
		},
		{
			name:    "One machine unknown",
			cluster: fakeCluster("c"),
			machines: []*clusterv1.Machine{
				fakeMachine("unknown-1", condition(metav1.Condition{
					Type:    clusterv1.MachineUpToDateCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  "some-unknown-reason-1",
					Message: "some unknown message",
				})),
			},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterWorkerMachinesUpToDateCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterWorkerMachinesUpToDateUnknownReason,
				Message: "* Machine unknown-1: some unknown message",
			},
		},
		{
			name:    "One machine not up-to-date",
			cluster: fakeCluster("c"),
			machines: []*clusterv1.Machine{
				fakeMachine("not-up-to-date-machine-1", condition(metav1.Condition{
					Type:    clusterv1.MachineUpToDateCondition,
					Status:  metav1.ConditionFalse,
					Reason:  "some-not-up-to-date-reason",
					Message: "some not up-to-date message",
				})),
			},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterWorkerMachinesUpToDateCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterWorkerMachinesNotUpToDateReason,
				Message: "* Machine not-up-to-date-machine-1: some not up-to-date message",
			},
		},
		{
			name:    "One machine without up-to-date condition, one new Machines without up-to-date condition",
			cluster: fakeCluster("c"),
			machines: []*clusterv1.Machine{
				fakeMachine("no-condition-machine-1"),
				fakeMachine("no-condition-machine-2-new", creationTimestamp{Time: time.Now().Add(-5 * time.Second)}), // ignored because it's new
			},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterWorkerMachinesUpToDateCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterWorkerMachinesUpToDateUnknownReason,
				Message: "* Machine no-condition-machine-1: Condition UpToDate not yet reported",
			},
		},
		{
			name:    "Two machines not up-to-date, two up-to-date, two not reported",
			cluster: fakeCluster("c"),
			machines: []*clusterv1.Machine{
				fakeMachine("up-to-date-1", condition(metav1.Condition{
					Type:   clusterv1.MachineUpToDateCondition,
					Status: metav1.ConditionTrue,
					Reason: "TestUpToDate",
				})),
				fakeMachine("up-to-date-2", condition(metav1.Condition{
					Type:   clusterv1.MachineUpToDateCondition,
					Status: metav1.ConditionTrue,
					Reason: "TestUpToDate",
				})),
				fakeMachine("not-up-to-date-machine-1", condition(metav1.Condition{
					Type:    clusterv1.MachineUpToDateCondition,
					Status:  metav1.ConditionFalse,
					Reason:  "TestNotUpToDate",
					Message: "This is not up-to-date message",
				})),
				fakeMachine("not-up-to-date-machine-2", condition(metav1.Condition{
					Type:    clusterv1.MachineUpToDateCondition,
					Status:  metav1.ConditionFalse,
					Reason:  "TestNotUpToDate",
					Message: "This is not up-to-date message",
				})),
				fakeMachine("no-condition-machine-1"),
				fakeMachine("no-condition-machine-2"),
			},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterWorkerMachinesUpToDateCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.ClusterWorkerMachinesNotUpToDateReason,
				Message: "* Machines not-up-to-date-machine-1, not-up-to-date-machine-2: This is not up-to-date message\n" +
					"* Machines no-condition-machine-1, no-condition-machine-2: Condition UpToDate not yet reported",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			var machines collections.Machines
			if tt.machines != nil {
				machines = collections.FromMachines(tt.machines...)
			}
			setWorkerMachinesUpToDateCondition(ctx, tt.cluster, machines, tt.getDescendantsSucceeded)

			condition := conditions.Get(tt.cluster, clusterv1.ClusterWorkerMachinesUpToDateCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tt.expectCondition, conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func TestSetRollingOutCondition(t *testing.T) {
	tests := []struct {
		name                    string
		cluster                 *clusterv1.Cluster
		controlPlane            *unstructured.Unstructured
		controlPlaneIsNotFound  bool
		machinePools            clusterv1.MachinePoolList
		machineDeployments      clusterv1.MachineDeploymentList
		getDescendantsSucceeded bool
		expectCondition         metav1.Condition
	}{
		{
			name:                    "cluster with controlplane, unknown if failed to get descendants",
			cluster:                 fakeCluster("c", controlPlaneRef{Name: "cp"}),
			controlPlane:            nil,
			controlPlaneIsNotFound:  true,
			getDescendantsSucceeded: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterRollingOutCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterRollingOutInternalErrorReason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:                    "cluster with controlplane, unknown if failed to get control plane",
			cluster:                 fakeCluster("c", controlPlaneRef{Name: "cp"}),
			controlPlane:            nil,
			controlPlaneIsNotFound:  false,
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterRollingOutCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterRollingOutInternalErrorReason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:                    "cluster with controlplane, false if no control plane & descendants",
			cluster:                 fakeCluster("c", controlPlaneRef{Name: "cp"}),
			controlPlaneIsNotFound:  true,
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterRollingOutCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.ClusterNotRollingOutReason,
			},
		},
		{
			name:         "cluster with controlplane, control plane & descendants do not report rolling out",
			cluster:      fakeCluster("c", controlPlaneRef{Name: "cp"}),
			controlPlane: fakeControlPlane("cp1"),
			machinePools: clusterv1.MachinePoolList{Items: []clusterv1.MachinePool{
				*fakeMachinePool("mp1"),
			}},
			machineDeployments: clusterv1.MachineDeploymentList{Items: []clusterv1.MachineDeployment{
				*fakeMachineDeployment("md1"),
			}},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterRollingOutCondition,
				Status: metav1.ConditionUnknown,
				Reason: clusterv1.ClusterRollingOutUnknownReason,
				Message: "* MachineDeployment md1: Condition RollingOut not yet reported\n" +
					"* MachinePool mp1: Condition RollingOut not yet reported",
			},
		},
		{
			name:    "cluster with controlplane, control plane and descendants report rolling out",
			cluster: fakeCluster("c", controlPlaneRef{Name: "cp"}),
			controlPlane: fakeControlPlane("cp1", condition{
				Type:    clusterv1.ClusterRollingOutCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.RollingOutReason,
				Message: "Rolling out 3 not up-to-date replicas",
			}),
			machinePools: clusterv1.MachinePoolList{Items: []clusterv1.MachinePool{
				*fakeMachinePool("mp1", condition{
					Type:    clusterv1.RollingOutCondition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.RollingOutReason,
					Message: "Rolling out 5 not up-to-date replicas",
				}),
			}},
			machineDeployments: clusterv1.MachineDeploymentList{Items: []clusterv1.MachineDeployment{
				*fakeMachineDeployment("md1", condition{
					Type:    clusterv1.MachineDeploymentRollingOutCondition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.MachineDeploymentRollingOutReason,
					Message: "Rolling out 4 not up-to-date replicas",
				}),
			}},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterRollingOutCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterRollingOutReason,
				Message: "* FakeControlPlane cp1: Rolling out 3 not up-to-date replicas\n" +
					"* MachineDeployment md1: Rolling out 4 not up-to-date replicas\n" +
					"* MachinePool mp1: Rolling out 5 not up-to-date replicas",
			},
		},
		{
			name:         "cluster with controlplane, control plane not reporting conditions, descendants report rolling out",
			cluster:      fakeCluster("c", controlPlaneRef{Name: "cp"}),
			controlPlane: fakeControlPlane("cp1"),
			machinePools: clusterv1.MachinePoolList{Items: []clusterv1.MachinePool{
				*fakeMachinePool("mp1", condition{
					Type:    clusterv1.RollingOutCondition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.RollingOutReason,
					Message: "Rolling out 3 not up-to-date replicas",
				}),
			}},
			machineDeployments: clusterv1.MachineDeploymentList{Items: []clusterv1.MachineDeployment{
				*fakeMachineDeployment("md1", condition{
					Type:    clusterv1.MachineDeploymentRollingOutCondition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.MachineDeploymentRollingOutReason,
					Message: "Rolling out 5 not up-to-date replicas",
				}),
			}},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterRollingOutCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterRollingOutReason,
				Message: "* MachineDeployment md1: Rolling out 5 not up-to-date replicas\n" +
					"* MachinePool mp1: Rolling out 3 not up-to-date replicas",
			},
		},
		{
			name:                    "cluster without controlplane, unknown if failed to get descendants",
			cluster:                 fakeCluster("c"),
			getDescendantsSucceeded: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterRollingOutCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterRollingOutInternalErrorReason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:                    "cluster without controlplane, false if no descendants",
			cluster:                 fakeCluster("c"),
			controlPlaneIsNotFound:  true,
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterRollingOutCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.ClusterNotRollingOutReason,
			},
		},
		{
			name:    "cluster without controlplane, descendants report rolling out",
			cluster: fakeCluster("c"),
			machinePools: clusterv1.MachinePoolList{Items: []clusterv1.MachinePool{
				*fakeMachinePool("mp1", condition{
					Type:    clusterv1.RollingOutCondition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.RollingOutReason,
					Message: "Rolling out 3 not up-to-date replicas",
				}),
			}},
			machineDeployments: clusterv1.MachineDeploymentList{Items: []clusterv1.MachineDeployment{
				*fakeMachineDeployment("md1", condition{
					Type:    clusterv1.MachineDeploymentRollingOutCondition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.MachineDeploymentRollingOutReason,
					Message: "Rolling out 5 not up-to-date replicas",
				}),
			}},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterRollingOutCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterRollingOutReason,
				Message: "* MachineDeployment md1: Rolling out 5 not up-to-date replicas\n" +
					"* MachinePool mp1: Rolling out 3 not up-to-date replicas",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			setRollingOutCondition(ctx, tt.cluster, tt.controlPlane, tt.machinePools, tt.machineDeployments, tt.controlPlaneIsNotFound, tt.getDescendantsSucceeded)

			condition := conditions.Get(tt.cluster, clusterv1.ClusterRollingOutCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tt.expectCondition, conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func TestSetScalingUpCondition(t *testing.T) {
	tests := []struct {
		name                    string
		cluster                 *clusterv1.Cluster
		controlPlane            *unstructured.Unstructured
		controlPlaneIsNotFound  bool
		machinePools            clusterv1.MachinePoolList
		machineDeployments      clusterv1.MachineDeploymentList
		machineSets             clusterv1.MachineSetList
		getDescendantsSucceeded bool
		expectCondition         metav1.Condition
	}{
		{
			name:                    "cluster with controlplane, unknown if failed to get descendants",
			cluster:                 fakeCluster("c", controlPlaneRef{Name: "cp"}),
			controlPlane:            nil,
			controlPlaneIsNotFound:  true,
			getDescendantsSucceeded: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterScalingUpCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterScalingUpInternalErrorReason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:                    "cluster with controlplane, unknown if failed to get control plane",
			cluster:                 fakeCluster("c", controlPlaneRef{Name: "cp"}),
			controlPlane:            nil,
			controlPlaneIsNotFound:  false,
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterScalingUpCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterScalingUpInternalErrorReason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:                    "cluster with controlplane, false if no control plane & descendants",
			cluster:                 fakeCluster("c", controlPlaneRef{Name: "cp"}),
			controlPlaneIsNotFound:  true,
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterScalingUpCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.ClusterNotScalingUpReason,
			},
		},
		{
			name:         "cluster with controlplane, control plane & descendants do not report scaling up",
			cluster:      fakeCluster("c", controlPlaneRef{Name: "cp"}),
			controlPlane: fakeControlPlane("cp1"),
			machinePools: clusterv1.MachinePoolList{Items: []clusterv1.MachinePool{
				*fakeMachinePool("mp1"),
			}},
			machineDeployments: clusterv1.MachineDeploymentList{Items: []clusterv1.MachineDeployment{
				*fakeMachineDeployment("md1"),
			}},
			machineSets: clusterv1.MachineSetList{Items: []clusterv1.MachineSet{
				*fakeMachineSet("ms1", OwnedByCluster("c")),
				*fakeMachineSet("ms2", condition{
					Type:    clusterv1.ClusterScalingUpCondition,
					Status:  metav1.ConditionFalse,
					Reason:  "Foo",
					Message: "foo",
				}), // not owned by the cluster
			}},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterScalingUpCondition,
				Status: metav1.ConditionUnknown,
				Reason: clusterv1.ClusterScalingUpUnknownReason,
				Message: "* MachineDeployment md1: Condition ScalingUp not yet reported\n" +
					"* MachinePool mp1: Condition ScalingUp not yet reported\n" +
					"* MachineSet ms1: Condition ScalingUp not yet reported",
			},
		},
		{
			name:    "cluster with controlplane, control plane and descendants report scaling up",
			cluster: fakeCluster("c", controlPlaneRef{Name: "cp"}),
			controlPlane: fakeControlPlane("cp1", condition{
				Type:    clusterv1.ClusterScalingUpCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.ScalingUpReason,
				Message: "Scaling up from 0 to 3 replicas",
			}),
			machinePools: clusterv1.MachinePoolList{Items: []clusterv1.MachinePool{
				*fakeMachinePool("mp1", condition{
					Type:    clusterv1.ScalingUpCondition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.ScalingUpReason,
					Message: "Scaling up from 0 to 3 replicas",
				}),
			}},
			machineDeployments: clusterv1.MachineDeploymentList{Items: []clusterv1.MachineDeployment{
				*fakeMachineDeployment("md1", condition{
					Type:    clusterv1.ClusterScalingUpCondition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.MachineDeploymentScalingUpReason,
					Message: "Scaling up from 1 to 5 replicas",
				}),
			}},
			machineSets: clusterv1.MachineSetList{Items: []clusterv1.MachineSet{
				*fakeMachineSet("ms1", OwnedByCluster("c"), condition{
					Type:    clusterv1.ClusterScalingUpCondition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.MachineSetScalingUpReason,
					Message: "Scaling up from 2 to 7 replicas",
				}),
				*fakeMachineSet("ms2", condition{
					Type:    clusterv1.ClusterScalingUpCondition,
					Status:  metav1.ConditionTrue,
					Reason:  "Foo",
					Message: "foo",
				}), // not owned by the cluster
			}},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterScalingUpCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterScalingUpReason,
				Message: "* FakeControlPlane cp1: Scaling up from 0 to 3 replicas\n" +
					"* MachineDeployment md1: Scaling up from 1 to 5 replicas\n" +
					"* MachinePool mp1: Scaling up from 0 to 3 replicas\n" +
					"And 1 MachineSet with other issues",
			},
		},
		{
			name:         "cluster with controlplane, control plane not reporting conditions, descendants report scaling up",
			cluster:      fakeCluster("c", controlPlaneRef{Name: "cp"}),
			controlPlane: fakeControlPlane("cp1"),
			machinePools: clusterv1.MachinePoolList{Items: []clusterv1.MachinePool{
				*fakeMachinePool("mp1", condition{
					Type:    clusterv1.ScalingUpCondition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.ScalingUpReason,
					Message: "Scaling up from 0 to 3 replicas",
				}),
			}},
			machineDeployments: clusterv1.MachineDeploymentList{Items: []clusterv1.MachineDeployment{
				*fakeMachineDeployment("md1", condition{
					Type:    clusterv1.ClusterScalingUpCondition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.MachineDeploymentScalingUpReason,
					Message: "Scaling up from 1 to 5 replicas",
				}),
			}},
			machineSets: clusterv1.MachineSetList{Items: []clusterv1.MachineSet{
				*fakeMachineSet("ms1", OwnedByCluster("c"), condition{
					Type:    clusterv1.ClusterScalingUpCondition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.MachineSetScalingUpReason,
					Message: "Scaling up from 2 to 7 replicas",
				}),
				*fakeMachineSet("ms2", condition{
					Type:    clusterv1.ClusterScalingUpCondition,
					Status:  metav1.ConditionTrue,
					Reason:  "Foo",
					Message: "foo",
				}), // not owned by the cluster
			}},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterScalingUpCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterScalingUpReason,
				Message: "* MachineDeployment md1: Scaling up from 1 to 5 replicas\n" +
					"* MachinePool mp1: Scaling up from 0 to 3 replicas\n" +
					"* MachineSet ms1: Scaling up from 2 to 7 replicas",
			},
		},
		{
			name:                    "cluster without controlplane, unknown if failed to get descendants",
			cluster:                 fakeCluster("c"),
			getDescendantsSucceeded: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterScalingUpCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterScalingUpInternalErrorReason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:                    "cluster without controlplane, false if no descendants",
			cluster:                 fakeCluster("c"),
			controlPlaneIsNotFound:  true,
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterScalingUpCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.ClusterNotScalingUpReason,
			},
		},
		{
			name:    "cluster without controlplane, descendants report scaling up",
			cluster: fakeCluster("c"),
			machinePools: clusterv1.MachinePoolList{Items: []clusterv1.MachinePool{
				*fakeMachinePool("mp1", condition{
					Type:    clusterv1.ScalingUpCondition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.ScalingUpReason,
					Message: "Scaling up from 0 to 3 replicas",
				}),
			}},
			machineDeployments: clusterv1.MachineDeploymentList{Items: []clusterv1.MachineDeployment{
				*fakeMachineDeployment("md1", condition{
					Type:    clusterv1.ClusterScalingUpCondition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.MachineDeploymentScalingUpReason,
					Message: "Scaling up from 1 to 5 replicas",
				}),
			}},
			machineSets: clusterv1.MachineSetList{Items: []clusterv1.MachineSet{
				*fakeMachineSet("ms1", OwnedByCluster("c"), condition{
					Type:    clusterv1.ClusterScalingUpCondition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.MachineSetScalingUpReason,
					Message: "Scaling up from 2 to 7 replicas",
				}),
				*fakeMachineSet("ms2", condition{
					Type:    clusterv1.ClusterScalingUpCondition,
					Status:  metav1.ConditionTrue,
					Reason:  "Foo",
					Message: "foo",
				}), // not owned by the cluster
			}},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterScalingUpCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterScalingUpReason,
				Message: "* MachineDeployment md1: Scaling up from 1 to 5 replicas\n" +
					"* MachinePool mp1: Scaling up from 0 to 3 replicas\n" +
					"* MachineSet ms1: Scaling up from 2 to 7 replicas",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			setScalingUpCondition(ctx, tt.cluster, tt.controlPlane, tt.machinePools, tt.machineDeployments, tt.machineSets, tt.controlPlaneIsNotFound, tt.getDescendantsSucceeded)

			condition := conditions.Get(tt.cluster, clusterv1.ClusterScalingUpCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tt.expectCondition, conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func TestSetScalingDownCondition(t *testing.T) {
	tests := []struct {
		name                    string
		cluster                 *clusterv1.Cluster
		controlPlane            *unstructured.Unstructured
		controlPlaneIsNotFound  bool
		machinePools            clusterv1.MachinePoolList
		machineDeployments      clusterv1.MachineDeploymentList
		machineSets             clusterv1.MachineSetList
		getDescendantsSucceeded bool
		expectCondition         metav1.Condition
	}{
		{
			name:                    "cluster with controlplane, unknown if failed to get descendants",
			cluster:                 fakeCluster("c", controlPlaneRef{Name: "cp"}),
			controlPlane:            nil,
			controlPlaneIsNotFound:  true,
			getDescendantsSucceeded: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterScalingDownCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterScalingDownInternalErrorReason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:                    "cluster with controlplane, unknown if failed to get control plane",
			cluster:                 fakeCluster("c", controlPlaneRef{Name: "cp"}),
			controlPlane:            nil,
			controlPlaneIsNotFound:  false,
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterScalingDownCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterScalingDownInternalErrorReason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:                    "cluster with controlplane, false if no control plane & descendants",
			cluster:                 fakeCluster("c", controlPlaneRef{Name: "cp"}),
			controlPlaneIsNotFound:  true,
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterScalingDownCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.ClusterNotScalingDownReason,
			},
		},
		{
			name:         "cluster with controlplane, control plane & descendants do not report scaling down",
			cluster:      fakeCluster("c", controlPlaneRef{Name: "cp"}),
			controlPlane: fakeControlPlane("cp1"),
			machinePools: clusterv1.MachinePoolList{Items: []clusterv1.MachinePool{
				*fakeMachinePool("mp1"),
			}},
			machineDeployments: clusterv1.MachineDeploymentList{Items: []clusterv1.MachineDeployment{
				*fakeMachineDeployment("md1"),
			}},
			machineSets: clusterv1.MachineSetList{Items: []clusterv1.MachineSet{
				*fakeMachineSet("ms1", OwnedByCluster("c")),
				*fakeMachineSet("ms2", condition{
					Type:    clusterv1.ClusterScalingDownCondition,
					Status:  metav1.ConditionFalse,
					Reason:  "Foo",
					Message: "foo",
				}), // not owned by the cluster
			}},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterScalingDownCondition,
				Status: metav1.ConditionUnknown,
				Reason: clusterv1.ClusterScalingDownUnknownReason,
				Message: "* MachineDeployment md1: Condition ScalingDown not yet reported\n" +
					"* MachinePool mp1: Condition ScalingDown not yet reported\n" +
					"* MachineSet ms1: Condition ScalingDown not yet reported",
			},
		},
		{
			name:    "cluster with controlplane, control plane and descendants report scaling down",
			cluster: fakeCluster("c", controlPlaneRef{Name: "cp"}),
			controlPlane: fakeControlPlane("cp1", condition{
				Type:    clusterv1.ClusterScalingDownCondition,
				Status:  metav1.ConditionTrue,
				Reason:  "Foo",
				Message: "Scaling down from 0 to 3 replicas",
			}),
			machinePools: clusterv1.MachinePoolList{Items: []clusterv1.MachinePool{
				*fakeMachinePool("mp1", condition{
					Type:    clusterv1.ClusterScalingDownCondition,
					Status:  metav1.ConditionTrue,
					Reason:  "Foo",
					Message: "Scaling down from 0 to 3 replicas",
				}),
			}},
			machineDeployments: clusterv1.MachineDeploymentList{Items: []clusterv1.MachineDeployment{
				*fakeMachineDeployment("md1", condition{
					Type:    clusterv1.ClusterScalingDownCondition,
					Status:  metav1.ConditionTrue,
					Reason:  "Foo",
					Message: "Scaling down from 1 to 5 replicas",
				}),
			}},
			machineSets: clusterv1.MachineSetList{Items: []clusterv1.MachineSet{
				*fakeMachineSet("ms1", OwnedByCluster("c"), condition{
					Type:    clusterv1.ClusterScalingDownCondition,
					Status:  metav1.ConditionTrue,
					Reason:  "Foo",
					Message: "Scaling down from 2 to 7 replicas",
				}),
				*fakeMachineSet("ms2", condition{
					Type:    clusterv1.ClusterScalingDownCondition,
					Status:  metav1.ConditionTrue,
					Reason:  "Foo",
					Message: "foo",
				}), // not owned by the cluster
			}},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterScalingDownCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterScalingDownReason,
				Message: "* FakeControlPlane cp1: Scaling down from 0 to 3 replicas\n" +
					"* MachineDeployment md1: Scaling down from 1 to 5 replicas\n" +
					"* MachinePool mp1: Scaling down from 0 to 3 replicas\n" +
					"And 1 MachineSet with other issues",
			},
		},
		{
			name:         "cluster with controlplane, control plane not reporting conditions, descendants report scaling down",
			cluster:      fakeCluster("c", controlPlaneRef{Name: "cp"}),
			controlPlane: fakeControlPlane("cp1"),
			machinePools: clusterv1.MachinePoolList{Items: []clusterv1.MachinePool{
				*fakeMachinePool("mp1", condition{
					Type:    clusterv1.ClusterScalingDownCondition,
					Status:  metav1.ConditionTrue,
					Reason:  "Foo",
					Message: "Scaling down from 0 to 3 replicas",
				}),
			}},
			machineDeployments: clusterv1.MachineDeploymentList{Items: []clusterv1.MachineDeployment{
				*fakeMachineDeployment("md1", condition{
					Type:    clusterv1.ClusterScalingDownCondition,
					Status:  metav1.ConditionTrue,
					Reason:  "Foo",
					Message: "Scaling down from 1 to 5 replicas",
				}),
			}},
			machineSets: clusterv1.MachineSetList{Items: []clusterv1.MachineSet{
				*fakeMachineSet("ms1", OwnedByCluster("c"), condition{
					Type:    clusterv1.ClusterScalingDownCondition,
					Status:  metav1.ConditionTrue,
					Reason:  "Foo",
					Message: "Scaling down from 2 to 7 replicas",
				}),
				*fakeMachineSet("ms2", condition{
					Type:    clusterv1.ClusterScalingDownCondition,
					Status:  metav1.ConditionTrue,
					Reason:  "Foo",
					Message: "foo",
				}), // not owned by the cluster
			}},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterScalingDownCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterScalingDownReason,
				Message: "* MachineDeployment md1: Scaling down from 1 to 5 replicas\n" +
					"* MachinePool mp1: Scaling down from 0 to 3 replicas\n" +
					"* MachineSet ms1: Scaling down from 2 to 7 replicas",
			},
		},
		{
			name:                    "cluster without controlplane, unknown if failed to get descendants",
			cluster:                 fakeCluster("c"),
			getDescendantsSucceeded: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterScalingDownCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterScalingDownInternalErrorReason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:                    "cluster without controlplane, false if no descendants",
			cluster:                 fakeCluster("c"),
			controlPlaneIsNotFound:  true,
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterScalingDownCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.ClusterNotScalingDownReason,
			},
		},
		{
			name:    "cluster without controlplane, descendants report scaling down",
			cluster: fakeCluster("c"),
			machinePools: clusterv1.MachinePoolList{Items: []clusterv1.MachinePool{
				*fakeMachinePool("mp1", condition{
					Type:    clusterv1.ClusterScalingDownCondition,
					Status:  metav1.ConditionTrue,
					Reason:  "Foo",
					Message: "Scaling down from 0 to 3 replicas",
				}),
			}},
			machineDeployments: clusterv1.MachineDeploymentList{Items: []clusterv1.MachineDeployment{
				*fakeMachineDeployment("md1", condition{
					Type:    clusterv1.ClusterScalingDownCondition,
					Status:  metav1.ConditionTrue,
					Reason:  "Foo",
					Message: "Scaling down from 1 to 5 replicas",
				}),
			}},
			machineSets: clusterv1.MachineSetList{Items: []clusterv1.MachineSet{
				*fakeMachineSet("ms1", OwnedByCluster("c"), condition{
					Type:    clusterv1.ClusterScalingDownCondition,
					Status:  metav1.ConditionTrue,
					Reason:  "Foo",
					Message: "Scaling down from 2 to 7 replicas",
				}),
				*fakeMachineSet("ms2", condition{
					Type:    clusterv1.ClusterScalingDownCondition,
					Status:  metav1.ConditionTrue,
					Reason:  "Foo",
					Message: "foo",
				}), // not owned by the cluster
			}},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterScalingDownCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterScalingDownReason,
				Message: "* MachineDeployment md1: Scaling down from 1 to 5 replicas\n" +
					"* MachinePool mp1: Scaling down from 0 to 3 replicas\n" +
					"* MachineSet ms1: Scaling down from 2 to 7 replicas",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			setScalingDownCondition(ctx, tt.cluster, tt.controlPlane, tt.machinePools, tt.machineDeployments, tt.machineSets, tt.controlPlaneIsNotFound, tt.getDescendantsSucceeded)

			condition := conditions.Get(tt.cluster, clusterv1.ClusterScalingDownCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tt.expectCondition, conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func TestSetRemediatingCondition(t *testing.T) {
	healthCheckSucceeded := metav1.Condition{Type: clusterv1.MachineHealthCheckSucceededCondition, Status: metav1.ConditionTrue}
	healthCheckNotSucceeded := metav1.Condition{Type: clusterv1.MachineHealthCheckSucceededCondition, Status: metav1.ConditionFalse}
	ownerRemediated := metav1.Condition{Type: clusterv1.MachineOwnerRemediatedCondition, Status: metav1.ConditionFalse, Reason: clusterv1.MachineSetMachineRemediationMachineDeletingReason, Message: "Machine is deleting"}

	tests := []struct {
		name                    string
		cluster                 *clusterv1.Cluster
		machines                []*clusterv1.Machine
		getDescendantsSucceeded bool
		expectCondition         metav1.Condition
	}{
		{
			name:                    "get descendant failed",
			cluster:                 fakeCluster("c"),
			machines:                nil,
			getDescendantsSucceeded: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterRemediatingCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterRemediatingInternalErrorReason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:    "Without unhealthy machines",
			cluster: fakeCluster("c"),
			machines: []*clusterv1.Machine{
				fakeMachine("m1"),
				fakeMachine("m2"),
			},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterRemediatingCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.ClusterNotRemediatingReason,
			},
		},
		{
			name:    "With machines to be remediated",
			cluster: fakeCluster("c"),
			machines: []*clusterv1.Machine{
				fakeMachine("m1", condition(healthCheckSucceeded)),    // Healthy machine
				fakeMachine("m2", condition(healthCheckNotSucceeded)), // Unhealthy machine, not yet marked for remediation
				fakeMachine("m3", condition(healthCheckNotSucceeded), condition(ownerRemediated)),
			},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterRemediatingCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.ClusterRemediatingReason,
				Message: "* Machine m3: Machine is deleting",
			},
		},
		{
			name:    "With one unhealthy machine not to be remediated",
			cluster: fakeCluster("c"),
			machines: []*clusterv1.Machine{
				fakeMachine("m1", condition(healthCheckSucceeded)),    // Healthy machine
				fakeMachine("m2", condition(healthCheckNotSucceeded)), // Unhealthy machine, not yet marked for remediation
				fakeMachine("m3", condition(healthCheckSucceeded)),    // Healthy machine
			},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterRemediatingCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterNotRemediatingReason,
				Message: "Machine m2 is not healthy (not to be remediated)",
			},
		},
		{
			name:    "With two unhealthy machine not to be remediated",
			cluster: fakeCluster("c"),
			machines: []*clusterv1.Machine{
				fakeMachine("m1", condition(healthCheckNotSucceeded)), // Unhealthy machine, not yet marked for remediation
				fakeMachine("m2", condition(healthCheckNotSucceeded)), // Unhealthy machine, not yet marked for remediation
				fakeMachine("m3", condition(healthCheckSucceeded)),    // Healthy machine
			},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterRemediatingCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterNotRemediatingReason,
				Message: "Machines m1, m2 are not healthy (not to be remediated)",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			var machinesToBeRemediated, unHealthyMachines collections.Machines
			if tt.getDescendantsSucceeded {
				machines := collections.FromMachines(tt.machines...)
				machinesToBeRemediated = machines.Filter(collections.IsUnhealthyAndOwnerRemediated)
				unHealthyMachines = machines.Filter(collections.IsUnhealthy)
			}
			setRemediatingCondition(ctx, tt.cluster, machinesToBeRemediated, unHealthyMachines, tt.getDescendantsSucceeded)

			condition := conditions.Get(tt.cluster, clusterv1.ClusterRemediatingCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tt.expectCondition, conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func TestDeletingCondition(t *testing.T) {
	testCases := []struct {
		name            string
		cluster         *clusterv1.Cluster
		deletingReason  string
		deletingMessage string
		expectCondition metav1.Condition
	}{
		{
			name:            "deletionTimestamp not set",
			cluster:         fakeCluster("c"),
			deletingReason:  "",
			deletingMessage: "",
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterDeletingCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.ClusterNotDeletingReason,
			},
		},
		{
			name:           "deletionTimestamp set (some reason/message reported)",
			cluster:        fakeCluster("c", deleted(true)),
			deletingReason: clusterv1.ClusterDeletingWaitingForWorkersDeletionReason,
			deletingMessage: "* Control plane Machines: cp1, cp2, cp3\n" +
				"* MachineDeployments: md1, md2\n" +
				"* MachineSets: ms1, ms2\n" +
				"* Worker Machines: w1, w2, w3, w4, w5, ... (3 more)",
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterDeletingCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterDeletingWaitingForWorkersDeletionReason,
				Message: "* Control plane Machines: cp1, cp2, cp3\n" +
					"* MachineDeployments: md1, md2\n" +
					"* MachineSets: ms1, ms2\n" +
					"* Worker Machines: w1, w2, w3, w4, w5, ... (3 more)",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			setDeletingCondition(ctx, tc.cluster, tc.deletingReason, tc.deletingMessage)

			deletingCondition := conditions.Get(tc.cluster, clusterv1.ClusterDeletingCondition)
			g.Expect(deletingCondition).ToNot(BeNil())
			g.Expect(*deletingCondition).To(conditions.MatchCondition(tc.expectCondition, conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func TestSetAvailableCondition(t *testing.T) {
	testCases := []struct {
		name            string
		cluster         *clusterv1.Cluster
		clusterClass    *clusterv1.ClusterClass
		expectCondition metav1.Condition
	}{
		{
			name: "Takes into account all the required conditions",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine-test",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: clusterv1.ClusterSpec{
					Topology: clusterv1.Topology{}, // not using CC
				},
				Status: clusterv1.ClusterStatus{
					Conditions: []metav1.Condition{
						// No condition reported yet, the required ones should be reported as missing.
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterAvailableCondition,
				Status: metav1.ConditionUnknown,
				Reason: clusterv1.ClusterAvailableUnknownReason,
				Message: "* Deleting: Condition not yet reported\n" +
					"* RemoteConnectionProbe: Condition not yet reported\n" +
					"* InfrastructureReady: Condition not yet reported\n" +
					"* ControlPlaneAvailable: Condition not yet reported\n" +
					"* WorkersAvailable: Condition not yet reported",
			},
		},
		{
			name: "TopologyReconciled is ignored when the cluster is not using CC",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine-test",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: clusterv1.ClusterSpec{
					Topology: clusterv1.Topology{}, // not using CC
				},
				Status: clusterv1.ClusterStatus{
					Conditions: []metav1.Condition{
						{
							Type:   clusterv1.ClusterInfrastructureReadyCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.ClusterControlPlaneAvailableCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.ClusterWorkersAvailableCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.ClusterRemoteConnectionProbeCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.ClusterDeletingCondition,
							Status: metav1.ConditionFalse,
							Reason: "Foo",
						},
						// TopologyReconciled missing
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterAvailableCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.ClusterAvailableReason,
				Message: "",
			},
		},
		{
			name: "Handles multiline conditions",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine-test",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: clusterv1.ClusterSpec{
					Topology: clusterv1.Topology{}, // not using CC
				},
				Status: clusterv1.ClusterStatus{
					Conditions: []metav1.Condition{
						{
							Type:   clusterv1.ClusterInfrastructureReadyCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.ClusterControlPlaneAvailableCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.ClusterWorkersAvailableCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.ClusterRemoteConnectionProbeCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.ClusterDeletingCondition,
							Status: metav1.ConditionTrue,
							Reason: clusterv1.ClusterDeletingWaitingForWorkersDeletionReason,
							Message: "* Control plane Machines: cp1, cp2, cp3\n" +
								"* MachineDeployments: md1, md2\n" +
								"* MachineSets: ms1, ms2\n" +
								"* Worker Machines: w1, w2, w3, w4, w5, ... (3 more)",
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterAvailableCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.ClusterNotAvailableReason,
				Message: "* Deleting:\n" +
					"  * Control plane Machines: cp1, cp2, cp3\n" +
					"  * MachineDeployments: md1, md2\n" +
					"  * MachineSets: ms1, ms2\n" +
					"  * Worker Machines: w1, w2, w3, w4, w5, ... (3 more)",
			},
		},
		{
			name: "TopologyReconciled is required when the cluster is using CC",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine-test",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: clusterv1.ClusterSpec{
					Topology: clusterv1.Topology{ // using CC
						ClassRef: clusterv1.ClusterClassRef{
							Name: "class",
						},
					},
				},
				Status: clusterv1.ClusterStatus{
					Conditions: []metav1.Condition{
						{
							Type:   clusterv1.ClusterInfrastructureReadyCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.ClusterControlPlaneAvailableCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.ClusterWorkersAvailableCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.ClusterRemoteConnectionProbeCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.ClusterDeletingCondition,
							Status: metav1.ConditionFalse,
							Reason: "Foo",
						},
						// TopologyReconciled missing
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterAvailableCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterAvailableUnknownReason,
				Message: "* TopologyReconciled: Condition not yet reported",
			},
		},
		{
			name: "Takes into account Availability gates when defined on the cluster",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine-test",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: clusterv1.ClusterSpec{
					AvailabilityGates: []clusterv1.ClusterAvailabilityGate{
						{
							ConditionType: "MyAvailabilityGate",
						},
						{
							ConditionType: "MyAvailabilityGateWithNegativePolarity",
							Polarity:      clusterv1.NegativePolarityCondition,
						},
					},
				},
				Status: clusterv1.ClusterStatus{
					Conditions: []metav1.Condition{
						{
							Type:   clusterv1.ClusterInfrastructureReadyCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.ClusterControlPlaneAvailableCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.ClusterWorkersAvailableCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.ClusterRemoteConnectionProbeCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.ClusterDeletingCondition,
							Status: metav1.ConditionFalse,
							Reason: "Foo",
						},
						{
							Type:    "MyAvailabilityGate",
							Status:  metav1.ConditionFalse,
							Reason:  "SomeReason",
							Message: "Some message",
						},
						{
							Type:    "MyAvailabilityGateWithNegativePolarity",
							Status:  metav1.ConditionTrue,
							Reason:  "SomeReason",
							Message: "Some other message",
						},
					},
				},
			},
			clusterClass: &clusterv1.ClusterClass{
				Spec: clusterv1.ClusterClassSpec{
					AvailabilityGates: []clusterv1.ClusterAvailabilityGate{
						{
							ConditionType: "MyClusterClassAvailabilityGate",
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterAvailableCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.ClusterNotAvailableReason,
				Message: "* MyAvailabilityGate: Some message\n" +
					"* MyAvailabilityGateWithNegativePolarity: Some other message",
			},
		},
		{
			name: "Takes into account Availability gates when defined on the cluster class",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine-test",
					Namespace: metav1.NamespaceDefault,
				},
				Status: clusterv1.ClusterStatus{
					Conditions: []metav1.Condition{
						{
							Type:   clusterv1.ClusterInfrastructureReadyCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.ClusterControlPlaneAvailableCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.ClusterWorkersAvailableCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.ClusterRemoteConnectionProbeCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.ClusterDeletingCondition,
							Status: metav1.ConditionFalse,
							Reason: "Foo",
						},
						{
							Type:    "MyClusterClassAvailabilityGate",
							Status:  metav1.ConditionFalse,
							Reason:  "SomeReason",
							Message: "Some message",
						},
						{
							Type:    "MyClusterClassAvailabilityGateWithNegativePolarity",
							Status:  metav1.ConditionTrue,
							Reason:  "SomeReason",
							Message: "Some other message",
						},
					},
				},
			},
			clusterClass: &clusterv1.ClusterClass{
				Spec: clusterv1.ClusterClassSpec{
					AvailabilityGates: []clusterv1.ClusterAvailabilityGate{
						{
							ConditionType: "MyClusterClassAvailabilityGate",
						},
						{
							ConditionType: "MyClusterClassAvailabilityGateWithNegativePolarity",
							Polarity:      clusterv1.NegativePolarityCondition,
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterAvailableCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.ClusterNotAvailableReason,
				Message: "* MyClusterClassAvailabilityGate: Some message\n" +
					"* MyClusterClassAvailabilityGateWithNegativePolarity: Some other message",
			},
		},
		{
			name: "Tolerates InfraCluster and ControlPlane do not exists while the cluster is deleting",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "machine-test",
					Namespace:         metav1.NamespaceDefault,
					DeletionTimestamp: &metav1.Time{Time: time.Now()},
				},
				Status: clusterv1.ClusterStatus{
					Conditions: []metav1.Condition{
						{
							Type:   clusterv1.ClusterInfrastructureReadyCondition,
							Status: metav1.ConditionFalse,
							Reason: clusterv1.ClusterInfrastructureDeletedReason,
						},
						{
							Type:   clusterv1.ClusterControlPlaneAvailableCondition,
							Status: metav1.ConditionFalse,
							Reason: clusterv1.ClusterControlPlaneDeletedReason,
						},
						{
							Type:   clusterv1.ClusterWorkersAvailableCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.ClusterRemoteConnectionProbeCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:    clusterv1.ClusterDeletingCondition,
							Status:  metav1.ConditionTrue,
							Reason:  clusterv1.ClusterDeletingWaitingForBeforeDeleteHookReason,
							Message: "Some message",
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterAvailableCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterNotAvailableReason,
				Message: "* Deleting: Some message",
			},
		},
		{
			name: "Surfaces message from TopologyReconciled for reason that doesn't affect availability (no other issues)",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine-test",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: clusterv1.ClusterSpec{
					Topology: clusterv1.Topology{ // using CC
						ClassRef: clusterv1.ClusterClassRef{
							Name: "class",
						},
					},
				},
				Status: clusterv1.ClusterStatus{
					Conditions: []metav1.Condition{
						{
							Type:   clusterv1.ClusterInfrastructureReadyCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.ClusterControlPlaneAvailableCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.ClusterWorkersAvailableCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.ClusterRemoteConnectionProbeCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.ClusterDeletingCondition,
							Status: metav1.ConditionFalse,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.ClusterTopologyReconciledCondition,
							Status: metav1.ConditionFalse,
							Reason: clusterv1.ClusterTopologyReconciledClusterUpgradingReason,
							Message: "Cluster is upgrading to v1.22.0\n" +
								"  * MachineDeployment md1 upgrading to version v1.22.0",
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterAvailableCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterAvailableReason,
				Message: "* TopologyReconciled:\n" +
					"  * Cluster is upgrading to v1.22.0\n" +
					"      * MachineDeployment md1 upgrading to version v1.22.0",
			},
		},
		{
			name: "Drops messages from TopologyReconciled for reason that doesn't affect availability (when there is another issue)",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine-test",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: clusterv1.ClusterSpec{
					Topology: clusterv1.Topology{ // using CC
						ClassRef: clusterv1.ClusterClassRef{
							Name: "class",
						},
					},
				},
				Status: clusterv1.ClusterStatus{
					Conditions: []metav1.Condition{
						{
							Type:   clusterv1.ClusterInfrastructureReadyCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.ClusterControlPlaneAvailableCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:    clusterv1.ClusterWorkersAvailableCondition,
							Status:  metav1.ConditionFalse,
							Reason:  clusterv1.ClusterWorkersNotAvailableReason,
							Message: "3 available replicas, at least 4 required (spec.strategy.rollout.maxUnavailable is 1, spec.replicas is 5) from MachineDeployment md1; 2 available replicas, at least 3 required (spec.strategy.rollout.maxUnavailable is 1, spec.replicas is 4) from MachinePool mp1",
						},
						{
							Type:   clusterv1.ClusterRemoteConnectionProbeCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.ClusterDeletingCondition,
							Status: metav1.ConditionFalse,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.ClusterTopologyReconciledCondition,
							Status: metav1.ConditionFalse,
							Reason: clusterv1.ClusterTopologyReconciledClusterUpgradingReason,
							Message: "Cluster is upgrading to v1.22.0\n" +
								"  * MachineDeployment md1 upgrading to version v1.22.0",
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterAvailableCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterNotAvailableReason,
				Message: "* WorkersAvailable: 3 available replicas, at least 4 required (spec.strategy.rollout.maxUnavailable is 1, spec.replicas is 5) from MachineDeployment md1; 2 available replicas, at least 3 required (spec.strategy.rollout.maxUnavailable is 1, spec.replicas is 4) from MachinePool mp1",
			},
		},
		{
			name: "Takes into account messages from TopologyReconciled for reason that affects availability (no other issues)",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine-test",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: clusterv1.ClusterSpec{
					Topology: clusterv1.Topology{ // using CC
						ClassRef: clusterv1.ClusterClassRef{
							Name: "class",
						},
					},
				},
				Status: clusterv1.ClusterStatus{
					Conditions: []metav1.Condition{
						{
							Type:   clusterv1.ClusterInfrastructureReadyCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.ClusterControlPlaneAvailableCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.ClusterWorkersAvailableCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.ClusterRemoteConnectionProbeCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.ClusterDeletingCondition,
							Status: metav1.ConditionFalse,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.ClusterTopologyReconciledCondition,
							Status: metav1.ConditionFalse,
							Reason: clusterv1.ClusterTopologyReconciledClusterClassNotReconciledReason,
							Message: "ClusterClass not reconciled. If this condition persists please check ClusterClass status. A ClusterClass is reconciled if" +
								".status.observedGeneration == .metadata.generation is true. If this is not the case either ClusterClass reconciliation failed or the ClusterClass is paused",
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterAvailableCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.ClusterNotAvailableReason,
				Message: "* TopologyReconciled: ClusterClass not reconciled. If this condition persists please check ClusterClass status. A ClusterClass is reconciled if" +
					".status.observedGeneration == .metadata.generation is true. If this is not the case either ClusterClass reconciliation failed or the ClusterClass is paused",
			},
		},
		{
			name: "Takes into account messages from TopologyReconciled for reason that affects availability (when there is another issue)",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine-test",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: clusterv1.ClusterSpec{
					Topology: clusterv1.Topology{ // using CC
						ClassRef: clusterv1.ClusterClassRef{
							Name: "class",
						},
					},
				},
				Status: clusterv1.ClusterStatus{
					Conditions: []metav1.Condition{
						{
							Type:   clusterv1.ClusterInfrastructureReadyCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.ClusterControlPlaneAvailableCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:    clusterv1.ClusterWorkersAvailableCondition,
							Status:  metav1.ConditionFalse,
							Reason:  clusterv1.ClusterWorkersNotAvailableReason,
							Message: "3 available replicas, at least 4 required (spec.strategy.rollout.maxUnavailable is 1, spec.replicas is 5) from MachineDeployment md1; 2 available replicas, at least 3 required (spec.strategy.rollout.maxUnavailable is 1, spec.replicas is 4) from MachinePool mp1",
						},
						{
							Type:   clusterv1.ClusterRemoteConnectionProbeCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.ClusterDeletingCondition,
							Status: metav1.ConditionFalse,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.ClusterTopologyReconciledCondition,
							Status: metav1.ConditionFalse,
							Reason: clusterv1.ClusterTopologyReconciledClusterClassNotReconciledReason,
							Message: "ClusterClass not reconciled. If this condition persists please check ClusterClass status. A ClusterClass is reconciled if" +
								".status.observedGeneration == .metadata.generation is true. If this is not the case either ClusterClass reconciliation failed or the ClusterClass is paused",
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterAvailableCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.ClusterNotAvailableReason,
				Message: "* WorkersAvailable: 3 available replicas, at least 4 required (spec.strategy.rollout.maxUnavailable is 1, spec.replicas is 5) from MachineDeployment md1; 2 available replicas, at least 3 required (spec.strategy.rollout.maxUnavailable is 1, spec.replicas is 4) from MachinePool mp1\n" +
					"* TopologyReconciled: ClusterClass not reconciled. If this condition persists please check ClusterClass status. A ClusterClass is reconciled if.status.observedGeneration == .metadata.generation is true. If this is not the case either ClusterClass reconciliation failed or the ClusterClass is paused",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			setAvailableCondition(ctx, tc.cluster, tc.clusterClass)

			condition := conditions.Get(tc.cluster, clusterv1.ClusterAvailableCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tc.expectCondition, conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

type fakeClusterOption interface {
	ApplyToCluster(c *clusterv1.Cluster)
}

func fakeCluster(name string, options ...fakeClusterOption) *clusterv1.Cluster {
	c := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	for _, opt := range options {
		opt.ApplyToCluster(c)
	}
	return c
}

type fakeControlPlaneOption interface {
	ApplyToControlPlane(cp *unstructured.Unstructured)
}

func fakeControlPlane(name string, options ...fakeControlPlaneOption) *unstructured.Unstructured {
	cp := &unstructured.Unstructured{Object: map[string]interface{}{
		"kind": "FakeControlPlane",
		"metadata": map[string]interface{}{
			"name": name,
		},
	}}

	for _, opt := range options {
		opt.ApplyToControlPlane(cp)
	}
	return cp
}

type fakeInfraClusterOption interface {
	ApplyToInfraCluster(cp *unstructured.Unstructured)
}

func fakeInfraCluster(name string, options ...fakeInfraClusterOption) *unstructured.Unstructured {
	cp := &unstructured.Unstructured{Object: map[string]interface{}{
		"kind": "FakeInfraCluster",
		"metadata": map[string]interface{}{
			"name": name,
		},
	}}

	for _, opt := range options {
		opt.ApplyToInfraCluster(cp)
	}
	return cp
}

type fakeMachinePoolOption interface {
	ApplyToMachinePool(mp *clusterv1.MachinePool)
}

func fakeMachinePool(name string, options ...fakeMachinePoolOption) *clusterv1.MachinePool {
	mp := &clusterv1.MachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	for _, opt := range options {
		opt.ApplyToMachinePool(mp)
	}
	return mp
}

type fakeMachineDeploymentOption interface {
	ApplyToMachineDeployment(md *clusterv1.MachineDeployment)
}

func fakeMachineDeployment(name string, options ...fakeMachineDeploymentOption) *clusterv1.MachineDeployment {
	md := &clusterv1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	for _, opt := range options {
		opt.ApplyToMachineDeployment(md)
	}
	return md
}

type fakeMachineSetOption interface {
	ApplyToMachineSet(ms *clusterv1.MachineSet)
}

func fakeMachineSet(name string, options ...fakeMachineSetOption) *clusterv1.MachineSet {
	ms := &clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	for _, opt := range options {
		opt.ApplyToMachineSet(ms)
	}
	return ms
}

type fakeMachineOption interface {
	ApplyToMachine(ms *clusterv1.Machine)
}

func fakeMachine(name string, options ...fakeMachineOption) *clusterv1.Machine {
	m := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	for _, opt := range options {
		opt.ApplyToMachine(m)
	}
	return m
}

type controlPlaneRef clusterv1.ContractVersionedObjectReference

func (r controlPlaneRef) ApplyToCluster(c *clusterv1.Cluster) {
	c.Spec.ControlPlaneRef = clusterv1.ContractVersionedObjectReference(r)
}

type infrastructureRef clusterv1.ContractVersionedObjectReference

func (r infrastructureRef) ApplyToCluster(c *clusterv1.Cluster) {
	c.Spec.InfrastructureRef = clusterv1.ContractVersionedObjectReference(r)
}

type infrastructureProvisioned bool

func (r infrastructureProvisioned) ApplyToCluster(c *clusterv1.Cluster) {
	c.Status.Initialization.InfrastructureProvisioned = ptr.To(bool(r))
}

type controlPlaneInitialized bool

func (r controlPlaneInitialized) ApplyToCluster(c *clusterv1.Cluster) {
	c.Status.Initialization.ControlPlaneInitialized = ptr.To(bool(r))
}

type desiredReplicas int32

func (r desiredReplicas) ApplyToControlPlane(cp *unstructured.Unstructured) {
	_ = contract.ControlPlane().Replicas().Set(cp, int32(r))
}

func (r desiredReplicas) ApplyToMachinePool(mp *clusterv1.MachinePool) {
	mp.Spec.Replicas = ptr.To(int32(r))
}

func (r desiredReplicas) ApplyToMachineDeployment(md *clusterv1.MachineDeployment) {
	md.Spec.Replicas = ptr.To(int32(r))
}

func (r desiredReplicas) ApplyToMachineSet(ms *clusterv1.MachineSet) {
	ms.Spec.Replicas = ptr.To(int32(r))
}

type currentReplicas int32

func (r currentReplicas) ApplyToControlPlane(cp *unstructured.Unstructured) {
	_ = contract.ControlPlane().StatusReplicas().Set(cp, int32(r))
}

func (r currentReplicas) ApplyToMachinePool(mp *clusterv1.MachinePool) {
	mp.Status.Replicas = ptr.To(int32(r))
}

func (r currentReplicas) ApplyToMachineDeployment(md *clusterv1.MachineDeployment) {
	md.Status.Replicas = ptr.To(int32(r))
}

func (r currentReplicas) ApplyToMachineSet(ms *clusterv1.MachineSet) {
	ms.Status.Replicas = ptr.To(int32(r))
}

type readyReplicas int32

func (r readyReplicas) ApplyToControlPlane(cp *unstructured.Unstructured) {
	_ = contract.ControlPlane().ReadyReplicas().Set(cp, int32(r))
}

func (r readyReplicas) ApplyToMachinePool(mp *clusterv1.MachinePool) {
	mp.Status.ReadyReplicas = ptr.To(int32(r))
}

func (r readyReplicas) ApplyToMachineDeployment(md *clusterv1.MachineDeployment) {
	md.Status.ReadyReplicas = ptr.To(int32(r))
}

func (r readyReplicas) ApplyToMachineSet(ms *clusterv1.MachineSet) {
	ms.Status.ReadyReplicas = ptr.To(int32(r))
}

type availableReplicas int32

func (r availableReplicas) ApplyToControlPlane(cp *unstructured.Unstructured) {
	_ = contract.ControlPlane().AvailableReplicas().Set(cp, int32(r))
}

func (r availableReplicas) ApplyToMachinePool(mp *clusterv1.MachinePool) {
	mp.Status.AvailableReplicas = ptr.To(int32(r))
}

func (r availableReplicas) ApplyToMachineDeployment(md *clusterv1.MachineDeployment) {
	md.Status.AvailableReplicas = ptr.To(int32(r))
}

func (r availableReplicas) ApplyToMachineSet(ms *clusterv1.MachineSet) {
	ms.Status.AvailableReplicas = ptr.To(int32(r))
}

type upToDateReplicas int32

func (r upToDateReplicas) ApplyToControlPlane(cp *unstructured.Unstructured) {
	_ = contract.ControlPlane().UpToDateReplicas(contract.Version).Set(cp, int32(r))
}

func (r upToDateReplicas) ApplyToMachinePool(mp *clusterv1.MachinePool) {
	mp.Status.UpToDateReplicas = ptr.To(int32(r))
}

func (r upToDateReplicas) ApplyToMachineDeployment(md *clusterv1.MachineDeployment) {
	md.Status.UpToDateReplicas = ptr.To(int32(r))
}

func (r upToDateReplicas) ApplyToMachineSet(ms *clusterv1.MachineSet) {
	ms.Status.UpToDateReplicas = ptr.To(int32(r))
}

type statusVersions []clusterv1.StatusVersion

func (v statusVersions) ApplyToControlPlane(cp *unstructured.Unstructured) {
	_ = contract.ControlPlane().StatusVersions().Set(cp, []clusterv1.StatusVersion(v))
}

func (v statusVersions) ApplyToMachinePool(mp *clusterv1.MachinePool) {
	mp.Status.Versions = []clusterv1.StatusVersion(v)
}

func (v statusVersions) ApplyToMachineDeployment(md *clusterv1.MachineDeployment) {
	md.Status.Versions = []clusterv1.StatusVersion(v)
}

func (v statusVersions) ApplyToMachineSet(ms *clusterv1.MachineSet) {
	ms.Status.Versions = []clusterv1.StatusVersion(v)
}

type deleted bool

func (s deleted) ApplyToCluster(c *clusterv1.Cluster) {
	if s {
		c.SetDeletionTimestamp(ptr.To(metav1.Time{Time: time.Now()}))
		return
	}
	c.SetDeletionTimestamp(nil)
}

type condition metav1.Condition

func (c condition) ApplyToCluster(cluster *clusterv1.Cluster) {
	conditions.Set(cluster, metav1.Condition(c))
}

func (c condition) ApplyToMachinePool(mp *clusterv1.MachinePool) {
	conditions.Set(mp, metav1.Condition(c))
}

func (c condition) ApplyToMachineDeployment(md *clusterv1.MachineDeployment) {
	conditions.Set(md, metav1.Condition(c))
}

func (c condition) ApplyToMachineSet(ms *clusterv1.MachineSet) {
	conditions.Set(ms, metav1.Condition(c))
}

func (c condition) ApplyToMachine(m *clusterv1.Machine) {
	conditions.Set(m, metav1.Condition(c))
}

func (c condition) ApplyToControlPlane(cp *unstructured.Unstructured) {
	c.applyToUnstructured(cp)
}

func (c condition) ApplyToInfraCluster(i *unstructured.Unstructured) {
	c.applyToUnstructured(i)
}

func (c condition) applyToUnstructured(i *unstructured.Unstructured) {
	conditions := []interface{}{}
	if value, exists, _ := unstructured.NestedFieldNoCopy(i.Object, "status", "conditions"); exists {
		conditions = value.([]interface{})
	}
	t, _ := c.LastTransitionTime.MarshalQueryParameter()
	conditions = append(conditions, map[string]interface{}{
		"type":               c.Type,
		"status":             string(c.Status),
		"reason":             c.Reason,
		"message":            c.Message,
		"lastTransitionTime": t,
	})
	_ = unstructured.SetNestedSlice(i.Object, conditions, "status", "conditions")
}

type OwnedByCluster string

func (c OwnedByCluster) ApplyToMachineSet(ms *clusterv1.MachineSet) {
	ms.OwnerReferences = append(ms.OwnerReferences, metav1.OwnerReference{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Cluster",
		Name:       string(c),
	})
}

func (c OwnedByCluster) ApplyToMachine(m *clusterv1.Machine) {
	m.OwnerReferences = append(m.OwnerReferences, metav1.OwnerReference{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Cluster",
		Name:       string(c),
	})
}

type kubeletVersion string

func (v kubeletVersion) ApplyToMachine(m *clusterv1.Machine) {
	if m.Status.NodeInfo == nil {
		m.Status.NodeInfo = &corev1.NodeSystemInfo{}
	}
	m.Status.NodeInfo.KubeletVersion = string(v)
}

type provisioned bool

func (r provisioned) ApplyToInfraCluster(i *unstructured.Unstructured) {
	_ = contract.InfrastructureCluster().Provisioned("v1beta2").Set(i, bool(r))
}

type initialized bool

func (r initialized) ApplyToControlPlane(cp *unstructured.Unstructured) {
	_ = contract.ControlPlane().Initialized("v1beta2").Set(cp, bool(r))
}

type creationTimestamp metav1.Time

func (t creationTimestamp) ApplyToMachine(m *clusterv1.Machine) {
	m.CreationTimestamp = metav1.Time(t)
}

type nodeRef clusterv1.MachineNodeReference

func (r nodeRef) ApplyToMachine(m *clusterv1.Machine) {
	m.Status.NodeRef = clusterv1.MachineNodeReference(r)
}

type topology bool

func (r topology) ApplyToCluster(c *clusterv1.Cluster) {
	c.Spec.Topology = clusterv1.Topology{
		ClassRef: clusterv1.ClusterClassRef{
			Name: "class",
		},
	}
}

type controlPlane bool

func (c controlPlane) ApplyToMachine(m *clusterv1.Machine) {
	if c {
		labels := m.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}
		labels[clusterv1.MachineControlPlaneLabel] = ""
		m.SetLabels(labels)
	}
}
