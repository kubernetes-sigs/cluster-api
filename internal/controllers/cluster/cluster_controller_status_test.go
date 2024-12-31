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
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/util/collections"
	v1beta2conditions "sigs.k8s.io/cluster-api/util/conditions/v1beta2"
)

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
	}{
		{
			name:                   "counters should be nil for a cluster with control plane, control plane does not exist",
			cluster:                fakeCluster("c", controlPlaneRef{}),
			controlPlane:           nil,
			controlPlaneIsNotFound: true,
		},
		{
			name:                   "counters should be nil for a cluster with control plane, unexpected error reading the CP",
			cluster:                fakeCluster("c", controlPlaneRef{}),
			controlPlane:           nil,
			controlPlaneIsNotFound: false,
		},
		{
			name:         "counters should be nil for a cluster with control plane, but not reporting counters",
			cluster:      fakeCluster("c", controlPlaneRef{}),
			controlPlane: fakeControlPlane("cp"),
		},
		{
			name:                    "set counters for cluster with control plane, reporting counters",
			cluster:                 fakeCluster("c", controlPlaneRef{}),
			controlPlane:            fakeControlPlane("cp", desiredReplicas(3), currentReplicas(2), v1beta2ReadyReplicas(1), v1beta2AvailableReplicas(2), v1beta2UpToDateReplicas(0)),
			expectDesiredReplicas:   ptr.To(int32(3)),
			expectReplicas:          ptr.To(int32(2)),
			expectReadyReplicas:     ptr.To(int32(1)),
			expectAvailableReplicas: ptr.To(int32(2)),
			expectUpToDateReplicas:  ptr.To(int32(0)),
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
				fakeMachine("cp1", v1beta2Condition{Type: clusterv1.MachineAvailableV1Beta2Condition, Status: metav1.ConditionTrue}, v1beta2Condition{Type: clusterv1.MachineReadyV1Beta2Condition, Status: metav1.ConditionTrue}, v1beta2Condition{Type: clusterv1.MachineUpToDateV1Beta2Condition, Status: metav1.ConditionTrue}),
				fakeMachine("cp2", v1beta2Condition{Type: clusterv1.MachineAvailableV1Beta2Condition, Status: metav1.ConditionFalse}, v1beta2Condition{Type: clusterv1.MachineReadyV1Beta2Condition, Status: metav1.ConditionTrue}, v1beta2Condition{Type: clusterv1.MachineUpToDateV1Beta2Condition, Status: metav1.ConditionTrue}),
				fakeMachine("cp3", v1beta2Condition{Type: clusterv1.MachineAvailableV1Beta2Condition, Status: metav1.ConditionFalse}, v1beta2Condition{Type: clusterv1.MachineReadyV1Beta2Condition, Status: metav1.ConditionFalse}, v1beta2Condition{Type: clusterv1.MachineUpToDateV1Beta2Condition, Status: metav1.ConditionTrue}),
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

			setControlPlaneReplicas(ctx, tt.cluster, tt.controlPlane, tt.machines, tt.controlPlaneIsNotFound, tt.getDescendantsSucceeded)

			g.Expect(tt.cluster.Status.V1Beta2).ToNot(BeNil())
			g.Expect(tt.cluster.Status.V1Beta2.ControlPlane).ToNot(BeNil())
			g.Expect(tt.cluster.Status.V1Beta2.ControlPlane.DesiredReplicas).To(Equal(tt.expectDesiredReplicas))
			g.Expect(tt.cluster.Status.V1Beta2.ControlPlane.Replicas).To(Equal(tt.expectReplicas))
			g.Expect(tt.cluster.Status.V1Beta2.ControlPlane.ReadyReplicas).To(Equal(tt.expectReadyReplicas))
			g.Expect(tt.cluster.Status.V1Beta2.ControlPlane.AvailableReplicas).To(Equal(tt.expectAvailableReplicas))
			g.Expect(tt.cluster.Status.V1Beta2.ControlPlane.UpToDateReplicas).To(Equal(tt.expectUpToDateReplicas))
		})
	}
}

func TestSetWorkersReplicas(t *testing.T) {
	tests := []struct {
		name                    string
		cluster                 *clusterv1.Cluster
		machinePools            expv1.MachinePoolList
		machineDeployments      clusterv1.MachineDeploymentList
		machineSets             clusterv1.MachineSetList
		workerMachines          collections.Machines
		getDescendantsSucceeded bool
		expectDesiredReplicas   *int32
		expectReplicas          *int32
		expectReadyReplicas     *int32
		expectAvailableReplicas *int32
		expectUpToDateReplicas  *int32
	}{
		{
			name:                    "counters should be nil if failed to get descendants",
			cluster:                 fakeCluster("c", controlPlaneRef{}),
			getDescendantsSucceeded: false,
		},
		{
			name:                    "counter should be nil there are no objects to count",
			cluster:                 fakeCluster("c", controlPlaneRef{}),
			getDescendantsSucceeded: true,
		},
		{
			name:    "counter should be nil if descendants are not reporting counters",
			cluster: fakeCluster("c", controlPlaneRef{}),
			machinePools: expv1.MachinePoolList{Items: []expv1.MachinePool{
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
			expectReplicas:          ptr.To(int32(0)), // Note: currently this is still not a pointer in v1beta1, but it should change in v1beta2
		},
		{
			name:    "should count workers from different objects",
			cluster: fakeCluster("c", controlPlaneRef{}),
			machinePools: expv1.MachinePoolList{Items: []expv1.MachinePool{
				*fakeMachinePool("mp1", desiredReplicas(1), currentReplicas(2), v1beta2ReadyReplicas(3), v1beta2AvailableReplicas(4), v1beta2UpToDateReplicas(5)),
			}},
			machineDeployments: clusterv1.MachineDeploymentList{Items: []clusterv1.MachineDeployment{
				*fakeMachineDeployment("md1", desiredReplicas(11), currentReplicas(12), v1beta2ReadyReplicas(13), v1beta2AvailableReplicas(14), v1beta2UpToDateReplicas(15)),
			}},
			machineSets: clusterv1.MachineSetList{Items: []clusterv1.MachineSet{
				*fakeMachineSet("ms1", OwnedByCluster("c"), desiredReplicas(21), currentReplicas(22), v1beta2ReadyReplicas(23), v1beta2AvailableReplicas(24), v1beta2UpToDateReplicas(25)),
				*fakeMachineSet("ms2", desiredReplicas(31), currentReplicas(32), v1beta2ReadyReplicas(33), v1beta2AvailableReplicas(34), v1beta2UpToDateReplicas(35)), // not owned by the cluster
			}},
			workerMachines: collections.FromMachines( // 4 replicas, 2 Ready, 3 Available, 1 UpToDate
				fakeMachine("m1", OwnedByCluster("c"), v1beta2Condition{Type: clusterv1.MachineAvailableV1Beta2Condition, Status: metav1.ConditionTrue}, v1beta2Condition{Type: clusterv1.MachineReadyV1Beta2Condition, Status: metav1.ConditionTrue}, v1beta2Condition{Type: clusterv1.MachineUpToDateV1Beta2Condition, Status: metav1.ConditionTrue}),
				fakeMachine("m2", OwnedByCluster("c"), v1beta2Condition{Type: clusterv1.MachineAvailableV1Beta2Condition, Status: metav1.ConditionTrue}, v1beta2Condition{Type: clusterv1.MachineReadyV1Beta2Condition, Status: metav1.ConditionFalse}, v1beta2Condition{Type: clusterv1.MachineUpToDateV1Beta2Condition, Status: metav1.ConditionFalse}),
				fakeMachine("m3", OwnedByCluster("c"), v1beta2Condition{Type: clusterv1.MachineAvailableV1Beta2Condition, Status: metav1.ConditionTrue}, v1beta2Condition{Type: clusterv1.MachineReadyV1Beta2Condition, Status: metav1.ConditionTrue}, v1beta2Condition{Type: clusterv1.MachineUpToDateV1Beta2Condition, Status: metav1.ConditionFalse}),
				fakeMachine("m4", OwnedByCluster("c")),
				fakeMachine("m5"), // not owned by the cluster
			),
			getDescendantsSucceeded: true,
			expectDesiredReplicas:   ptr.To(int32(37)),
			expectReplicas:          ptr.To(int32(40)),
			expectReadyReplicas:     ptr.To(int32(41)),
			expectAvailableReplicas: ptr.To(int32(45)),
			expectUpToDateReplicas:  ptr.To(int32(46)),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			setWorkersReplicas(ctx, tt.cluster, tt.machinePools, tt.machineDeployments, tt.machineSets, tt.workerMachines, tt.getDescendantsSucceeded)

			g.Expect(tt.cluster.Status.V1Beta2).ToNot(BeNil())
			g.Expect(tt.cluster.Status.V1Beta2.Workers).ToNot(BeNil())
			g.Expect(tt.cluster.Status.V1Beta2.Workers.DesiredReplicas).To(Equal(tt.expectDesiredReplicas))
			g.Expect(tt.cluster.Status.V1Beta2.Workers.Replicas).To(Equal(tt.expectReplicas))
			g.Expect(tt.cluster.Status.V1Beta2.Workers.ReadyReplicas).To(Equal(tt.expectReadyReplicas))
			g.Expect(tt.cluster.Status.V1Beta2.Workers.AvailableReplicas).To(Equal(tt.expectAvailableReplicas))
			g.Expect(tt.cluster.Status.V1Beta2.Workers.UpToDateReplicas).To(Equal(tt.expectUpToDateReplicas))
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
				Type:    clusterv1.ClusterInfrastructureReadyV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterInfrastructureDoesNotExistV1Beta2Reason,
				Message: "Waiting for cluster topology to be reconciled",
			},
		},
		{
			name:                   "topology not yet reconcile, cluster deleted",
			cluster:                fakeCluster("c", topology(true), deleted(true)),
			infraCluster:           nil,
			infraClusterIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterInfrastructureReadyV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.ClusterInfrastructureDoesNotExistV1Beta2Reason,
			},
		},
		{
			name:                   "mirror Ready condition from infra cluster",
			cluster:                fakeCluster("c", infrastructureRef{Kind: "FakeInfraCluster"}),
			infraCluster:           fakeInfraCluster("i1", condition{Type: "Ready", Status: "False", Message: "some message", Reason: "SomeReason"}),
			infraClusterIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterInfrastructureReadyV1Beta2Condition,
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
				Type:    clusterv1.ClusterInfrastructureReadyV1Beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.ClusterInfrastructureReadyV1Beta2Reason, // reason fixed up
				Message: "some message",
			},
		},
		{
			name:                   "Use status.InfrastructureReady flag as a fallback Ready condition from infra cluster is missing",
			cluster:                fakeCluster("c", infrastructureRef{Kind: "FakeInfraCluster"}),
			infraCluster:           fakeInfraCluster("i1", ready(false)),
			infraClusterIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterInfrastructureReadyV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterInfrastructureNotReadyV1Beta2Reason,
				Message: "FakeInfraCluster status.ready is false",
			},
		},
		{
			name:                   "Use status.InfrastructureReady flag as a fallback Ready condition from infra cluster is missing (ready true)",
			cluster:                fakeCluster("c", infrastructureRef{Kind: "FakeInfraCluster"}, infrastructureReady(true)),
			infraCluster:           fakeInfraCluster("i1", ready(true)),
			infraClusterIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterInfrastructureReadyV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterInfrastructureReadyV1Beta2Reason,
			},
		},
		{
			name:                   "invalid Ready condition from infra cluster",
			cluster:                fakeCluster("c", infrastructureRef{Kind: "FakeInfraCluster"}),
			infraCluster:           fakeInfraCluster("i1", condition{Type: "Ready"}),
			infraClusterIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterInfrastructureReadyV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterInfrastructureInvalidConditionReportedV1Beta2Reason,
				Message: "failed to convert status.conditions from FakeInfraCluster to []metav1.Condition: status must be set for the Ready condition",
			},
		},
		{
			name:                   "failed to get infra cluster",
			cluster:                fakeCluster("c", infrastructureRef{Kind: "FakeInfraCluster"}, deleted(true)),
			infraCluster:           nil,
			infraClusterIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterInfrastructureReadyV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterInfrastructureInternalErrorV1Beta2Reason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:                   "infra cluster that was ready not found while cluster is deleting",
			cluster:                fakeCluster("c", infrastructureRef{Kind: "FakeInfraCluster"}, infrastructureReady(true), deleted(true)),
			infraCluster:           nil,
			infraClusterIsNotFound: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterInfrastructureReadyV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterInfrastructureDeletedV1Beta2Reason,
				Message: "FakeInfraCluster has been deleted",
			},
		},
		{
			name:                   "infra cluster not found while cluster is deleting",
			cluster:                fakeCluster("c", infrastructureRef{Kind: "FakeInfraCluster"}, deleted(true)),
			infraCluster:           nil,
			infraClusterIsNotFound: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterInfrastructureReadyV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterInfrastructureDoesNotExistV1Beta2Reason,
				Message: "FakeInfraCluster does not exist",
			},
		},
		{
			name:                   "infra cluster not found after the cluster has been initialized",
			cluster:                fakeCluster("c", infrastructureRef{Kind: "FakeInfraCluster"}, infrastructureReady(true)),
			infraCluster:           nil,
			infraClusterIsNotFound: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterInfrastructureReadyV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterInfrastructureDeletedV1Beta2Reason,
				Message: "FakeInfraCluster has been deleted while the cluster still exists",
			},
		},
		{
			name:                   "infra cluster not found",
			cluster:                fakeCluster("c", infrastructureRef{Kind: "FakeInfraCluster"}),
			infraCluster:           nil,
			infraClusterIsNotFound: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterInfrastructureReadyV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterInfrastructureDoesNotExistV1Beta2Reason,
				Message: "FakeInfraCluster does not exist",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			setInfrastructureReadyCondition(ctx, tc.cluster, tc.infraCluster, tc.infraClusterIsNotFound)

			condition := v1beta2conditions.Get(tc.cluster, clusterv1.ClusterInfrastructureReadyV1Beta2Condition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(v1beta2conditions.MatchCondition(tc.expectCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
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
				Type:    clusterv1.ClusterControlPlaneAvailableV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterControlPlaneDoesNotExistV1Beta2Reason,
				Message: "Waiting for cluster topology to be reconciled",
			},
		},
		{
			name:                   "topology not yet reconcile, cluster deleted",
			cluster:                fakeCluster("c", topology(true), deleted(true)),
			controlPlane:           nil,
			controlPlaneIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterControlPlaneAvailableV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.ClusterControlPlaneDoesNotExistV1Beta2Reason,
			},
		},
		{
			name:                   "mirror Available condition from control plane",
			cluster:                fakeCluster("c", controlPlaneRef{Kind: "FakeControlPlane"}),
			controlPlane:           fakeControlPlane("cp1", condition{Type: "Available", Status: "False", Message: "some message", Reason: "SomeReason"}),
			controlPlaneIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneAvailableV1Beta2Condition,
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
				Type:    clusterv1.ClusterControlPlaneAvailableV1Beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.ClusterControlPlaneAvailableV1Beta2Reason, // reason fixed up
				Message: "some message",
			},
		},
		{
			name:                   "Use status.controlPlaneReady flag as a fallback Available condition from control plane is missing",
			cluster:                fakeCluster("c", controlPlaneRef{Kind: "FakeControlPlane"}),
			controlPlane:           fakeControlPlane("cp1", ready(false)),
			controlPlaneIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneAvailableV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterControlPlaneNotAvailableV1Beta2Reason,
				Message: "FakeControlPlane status.ready is false",
			},
		},
		{
			name:                   "Use status.controlPlaneReady flag as a fallback Available condition from control plane is missing (ready true)",
			cluster:                fakeCluster("c", controlPlaneRef{Kind: "FakeControlPlane"}, controlPlaneReady(true)),
			controlPlane:           fakeControlPlane("cp1", ready(true)),
			controlPlaneIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterControlPlaneAvailableV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterControlPlaneAvailableV1Beta2Reason,
			},
		},
		{
			name:                   "invalid Available condition from control plane",
			cluster:                fakeCluster("c", controlPlaneRef{Kind: "FakeControlPlane"}),
			controlPlane:           fakeControlPlane("cp1", condition{Type: "Available"}),
			controlPlaneIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneAvailableV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterControlPlaneInvalidConditionReportedV1Beta2Reason,
				Message: "failed to convert status.conditions from FakeControlPlane to []metav1.Condition: status must be set for the Available condition",
			},
		},
		{
			name:                   "failed to get control planer",
			cluster:                fakeCluster("c", controlPlaneRef{Kind: "FakeControlPlane"}, deleted(true)),
			controlPlane:           nil,
			controlPlaneIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneAvailableV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterControlPlaneInternalErrorV1Beta2Reason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:                   "control plane that was ready not found while cluster is deleting",
			cluster:                fakeCluster("c", controlPlaneRef{Kind: "FakeControlPlane"}, controlPlaneReady(true), deleted(true)),
			controlPlane:           nil,
			controlPlaneIsNotFound: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneAvailableV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterControlPlaneDeletedV1Beta2Reason,
				Message: "FakeControlPlane has been deleted",
			},
		},
		{
			name:                   "control plane not found while cluster is deleting",
			cluster:                fakeCluster("c", controlPlaneRef{Kind: "FakeControlPlane"}, deleted(true)),
			controlPlane:           nil,
			controlPlaneIsNotFound: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneAvailableV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterControlPlaneDoesNotExistV1Beta2Reason,
				Message: "FakeControlPlane does not exist",
			},
		},
		{
			name:                   "control plane not found after the cluster has been initialized",
			cluster:                fakeCluster("c", controlPlaneRef{Kind: "FakeControlPlane"}, controlPlaneReady(true)),
			controlPlane:           nil,
			controlPlaneIsNotFound: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneAvailableV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterControlPlaneDeletedV1Beta2Reason,
				Message: "FakeControlPlane has been deleted while the cluster still exists",
			},
		},
		{
			name:                   "control plane not found",
			cluster:                fakeCluster("c", controlPlaneRef{Kind: "FakeControlPlane"}),
			controlPlane:           nil,
			controlPlaneIsNotFound: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneAvailableV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterControlPlaneDoesNotExistV1Beta2Reason,
				Message: "FakeControlPlane does not exist",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			setControlPlaneAvailableCondition(ctx, tc.cluster, tc.controlPlane, tc.controlPlaneIsNotFound)

			condition := v1beta2conditions.Get(tc.cluster, clusterv1.ClusterControlPlaneAvailableV1Beta2Condition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(v1beta2conditions.MatchCondition(tc.expectCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
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
				Type:    clusterv1.ClusterControlPlaneInitializedV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterControlPlaneDoesNotExistV1Beta2Reason,
				Message: "Waiting for cluster topology to be reconciled",
			},
		},
		{
			name:                   "topology not yet reconcile, cluster deleted",
			cluster:                fakeCluster("c", topology(true), deleted(true)),
			controlPlane:           nil,
			controlPlaneIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterControlPlaneInitializedV1Beta2Condition,
				Status: metav1.ConditionUnknown,
				Reason: clusterv1.ClusterControlPlaneDoesNotExistV1Beta2Reason,
			},
		},
		{
			name:                   "cluster with control plane, control plane does not exist",
			cluster:                fakeCluster("c", controlPlaneRef{Kind: "FakeControlPlane"}),
			controlPlane:           nil,
			controlPlaneIsNotFound: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneInitializedV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterControlPlaneDoesNotExistV1Beta2Reason,
				Message: "FakeControlPlane does not exist",
			},
		},
		{
			name:                   "cluster with control plane, unexpected error reading the CP",
			cluster:                fakeCluster("c", controlPlaneRef{Kind: "FakeControlPlane"}),
			controlPlane:           nil,
			controlPlaneIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneInitializedV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterControlPlaneInitializedInternalErrorV1Beta2Reason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:         "cluster with control plane, not yet initialized",
			cluster:      fakeCluster("c", controlPlaneRef{Kind: "FakeControlPlane"}),
			controlPlane: fakeControlPlane("cp"),
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneInitializedV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterControlPlaneNotInitializedV1Beta2Reason,
				Message: "Control plane not yet initialized",
			},
		},
		{
			name:         "cluster with control plane, initialized",
			cluster:      fakeCluster("c", controlPlaneRef{Kind: "FakeControlPlane"}),
			controlPlane: fakeControlPlane("cp", initialized(true)),
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterControlPlaneInitializedV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterControlPlaneInitializedV1Beta2Reason,
			},
		},
		{
			name:                    "cluster with stand alone CP machines, failed to read descendants",
			cluster:                 fakeCluster("c"),
			getDescendantsSucceeded: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneInitializedV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterControlPlaneInitializedInternalErrorV1Beta2Reason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:                    "cluster with stand alone CP machines, no machines with a node yet",
			cluster:                 fakeCluster("c"),
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneInitializedV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterControlPlaneNotInitializedV1Beta2Reason,
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
				Type:   clusterv1.ClusterControlPlaneInitializedV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterControlPlaneInitializedV1Beta2Reason,
			},
		},
		{
			name:         "initialized never flips back to false",
			cluster:      fakeCluster("c", controlPlaneRef{Kind: "FakeControlPlane"}, v1beta2Condition{Type: clusterv1.ClusterControlPlaneInitializedV1Beta2Condition, Status: metav1.ConditionTrue, Reason: clusterv1.ClusterControlPlaneInitializedV1Beta2Reason}),
			controlPlane: fakeControlPlane("cp", initialized(false)),
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterControlPlaneInitializedV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterControlPlaneInitializedV1Beta2Reason,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			setControlPlaneInitializedCondition(ctx, tt.cluster, tt.controlPlane, tt.machines, tt.controlPlaneIsNotFound, tt.getDescendantsSucceeded)

			condition := v1beta2conditions.Get(tt.cluster, clusterv1.ClusterControlPlaneInitializedV1Beta2Condition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(v1beta2conditions.MatchCondition(tt.expectCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func TestSetWorkersAvailableCondition(t *testing.T) {
	tests := []struct {
		name                    string
		cluster                 *clusterv1.Cluster
		machinePools            expv1.MachinePoolList
		machineDeployments      clusterv1.MachineDeploymentList
		getDescendantsSucceeded bool
		expectCondition         metav1.Condition
	}{
		{
			name:                    "unknown if failed to get descendants",
			cluster:                 fakeCluster("c", controlPlaneRef{}),
			getDescendantsSucceeded: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterWorkersAvailableV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterWorkersAvailableInternalErrorV1Beta2Reason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:                    "true if no descendants",
			cluster:                 fakeCluster("c", controlPlaneRef{}),
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterWorkersAvailableV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterWorkersAvailableNoWorkersV1Beta2Reason,
			},
		},
		{
			name:    "descendants do not report available",
			cluster: fakeCluster("c", controlPlaneRef{}),
			machinePools: expv1.MachinePoolList{Items: []expv1.MachinePool{
				*fakeMachinePool("mp1"),
			}},
			machineDeployments: clusterv1.MachineDeploymentList{Items: []clusterv1.MachineDeployment{
				*fakeMachineDeployment("md1"),
			}},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterWorkersAvailableV1Beta2Condition,
				Status: metav1.ConditionUnknown,
				Reason: clusterv1.ClusterWorkersAvailableUnknownV1Beta2Reason,
				Message: "* MachineDeployment md1: Condition Available not yet reported\n" +
					"* MachinePool mp1: Condition Available not yet reported",
			},
		},
		{
			name:    "descendants report available",
			cluster: fakeCluster("c", controlPlaneRef{}),
			machinePools: expv1.MachinePoolList{Items: []expv1.MachinePool{
				*fakeMachinePool("mp1", v1beta2Condition{
					Type:    clusterv1.MachineDeploymentAvailableV1Beta2Condition,
					Status:  metav1.ConditionFalse,
					Reason:  "Foo",
					Message: "2 available replicas, at least 3 required (spec.strategy.rollout.maxUnavailable is 1, spec.replicas is 4)",
				}),
			}},
			machineDeployments: clusterv1.MachineDeploymentList{Items: []clusterv1.MachineDeployment{
				*fakeMachineDeployment("md1", v1beta2Condition{
					Type:    clusterv1.MachineDeploymentAvailableV1Beta2Condition,
					Status:  metav1.ConditionFalse,
					Reason:  "Foo",
					Message: "3 available replicas, at least 4 required (spec.strategy.rollout.maxUnavailable is 1, spec.replicas is 5)",
				}),
			}},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterWorkersAvailableV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.ClusterWorkersNotAvailableV1Beta2Reason,
				Message: "* MachineDeployment md1: 3 available replicas, at least 4 required (spec.strategy.rollout.maxUnavailable is 1, spec.replicas is 5)\n" +
					"* MachinePool mp1: 2 available replicas, at least 3 required (spec.strategy.rollout.maxUnavailable is 1, spec.replicas is 4)",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			setWorkersAvailableCondition(ctx, tt.cluster, tt.machinePools, tt.machineDeployments, tt.getDescendantsSucceeded)

			condition := v1beta2conditions.Get(tt.cluster, clusterv1.ClusterWorkersAvailableV1Beta2Condition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(v1beta2conditions.MatchCondition(tt.expectCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func TestSetControlPlaneMachinesReadyCondition(t *testing.T) {
	readyCondition := metav1.Condition{
		Type:   clusterv1.MachineReadyV1Beta2Condition,
		Status: metav1.ConditionTrue,
		Reason: clusterv1.MachineReadyV1Beta2Reason,
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
				Type:    clusterv1.ClusterControlPlaneMachinesReadyV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterControlPlaneMachinesReadyInternalErrorV1Beta2Reason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:                    "no machines",
			cluster:                 fakeCluster("c"),
			machines:                []*clusterv1.Machine{},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterControlPlaneMachinesReadyV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterControlPlaneMachinesReadyNoReplicasV1Beta2Reason,
			},
		},
		{
			name:    "all machines are ready",
			cluster: fakeCluster("c"),
			machines: []*clusterv1.Machine{
				fakeMachine("machine-1", controlPlane(true), v1beta2Condition(readyCondition)),
				fakeMachine("machine-2", controlPlane(true), v1beta2Condition(readyCondition)),
			},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterControlPlaneMachinesReadyV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterControlPlaneMachinesReadyV1Beta2Reason,
			},
		},
		{
			name:    "one ready, one has nothing reported",
			cluster: fakeCluster("c"),
			machines: []*clusterv1.Machine{
				fakeMachine("machine-1", controlPlane(true), v1beta2Condition(readyCondition)),
				fakeMachine("machine-2", controlPlane(true)),
			},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneMachinesReadyV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterControlPlaneMachinesReadyUnknownV1Beta2Reason,
				Message: "* Machine machine-2: Condition Ready not yet reported",
			},
		},
		{
			name:    "one ready, one reporting not ready, one reporting unknown, one reporting deleting",
			cluster: fakeCluster("c"),
			machines: []*clusterv1.Machine{
				fakeMachine("machine-1", controlPlane(true), v1beta2Condition(readyCondition)),
				fakeMachine("machine-2", controlPlane(true), v1beta2Condition(metav1.Condition{
					Type:    clusterv1.MachineReadyV1Beta2Condition,
					Status:  metav1.ConditionFalse,
					Reason:  "SomeReason",
					Message: "HealthCheckSucceeded: Some message",
				})),
				fakeMachine("machine-3", controlPlane(true), v1beta2Condition(metav1.Condition{
					Type:    clusterv1.MachineReadyV1Beta2Condition,
					Status:  metav1.ConditionUnknown,
					Reason:  "SomeUnknownReason",
					Message: "Some unknown message",
				})),
				fakeMachine("machine-4", controlPlane(true), v1beta2Condition(metav1.Condition{
					Type:    clusterv1.MachineReadyV1Beta2Condition,
					Status:  metav1.ConditionFalse,
					Reason:  clusterv1.MachineDeletingV1Beta2Reason,
					Message: "Deleting: Machine deletion in progress, stage: DrainingNode",
				})),
			},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterControlPlaneMachinesReadyV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.ClusterControlPlaneMachinesNotReadyV1Beta2Reason,
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

			condition := v1beta2conditions.Get(tt.cluster, clusterv1.ClusterControlPlaneMachinesReadyV1Beta2Condition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(v1beta2conditions.MatchCondition(tt.expectCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func TestSetWorkerMachinesReadyCondition(t *testing.T) {
	readyCondition := metav1.Condition{
		Type:   clusterv1.MachineReadyV1Beta2Condition,
		Status: metav1.ConditionTrue,
		Reason: clusterv1.MachineReadyV1Beta2Reason,
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
				Type:    clusterv1.ClusterWorkerMachinesReadyV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterWorkerMachinesReadyInternalErrorV1Beta2Reason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:                    "no machines",
			cluster:                 fakeCluster("c"),
			machines:                []*clusterv1.Machine{},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterWorkerMachinesReadyV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterWorkerMachinesReadyNoReplicasV1Beta2Reason,
			},
		},
		{
			name:    "all machines are ready",
			cluster: fakeCluster("c"),
			machines: []*clusterv1.Machine{
				fakeMachine("machine-1", v1beta2Condition(readyCondition)),
				fakeMachine("machine-2", v1beta2Condition(readyCondition)),
			},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterWorkerMachinesReadyV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterWorkerMachinesReadyV1Beta2Reason,
			},
		},
		{
			name:    "one ready, one has nothing reported",
			cluster: fakeCluster("c"),
			machines: []*clusterv1.Machine{
				fakeMachine("machine-1", v1beta2Condition(readyCondition)),
				fakeMachine("machine-2"),
			},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterWorkerMachinesReadyV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterWorkerMachinesReadyUnknownV1Beta2Reason,
				Message: "* Machine machine-2: Condition Ready not yet reported",
			},
		},
		{
			name:    "one ready, one reporting not ready, one reporting unknown, one reporting deleting",
			cluster: fakeCluster("c"),
			machines: []*clusterv1.Machine{
				fakeMachine("machine-1", v1beta2Condition(readyCondition)),
				fakeMachine("machine-2", v1beta2Condition(metav1.Condition{
					Type:    clusterv1.MachineReadyV1Beta2Condition,
					Status:  metav1.ConditionFalse,
					Reason:  "SomeReason",
					Message: "HealthCheckSucceeded: Some message",
				})),
				fakeMachine("machine-3", v1beta2Condition(metav1.Condition{
					Type:    clusterv1.MachineReadyV1Beta2Condition,
					Status:  metav1.ConditionUnknown,
					Reason:  "SomeUnknownReason",
					Message: "Some unknown message",
				})),
				fakeMachine("machine-4", v1beta2Condition(metav1.Condition{
					Type:    clusterv1.MachineReadyV1Beta2Condition,
					Status:  metav1.ConditionFalse,
					Reason:  clusterv1.MachineDeletingV1Beta2Reason,
					Message: "Deleting: Machine deletion in progress, stage: DrainingNode",
				})),
			},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterWorkerMachinesReadyV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.ClusterWorkerMachinesNotReadyV1Beta2Reason,
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

			condition := v1beta2conditions.Get(tt.cluster, clusterv1.ClusterWorkerMachinesReadyV1Beta2Condition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(v1beta2conditions.MatchCondition(tt.expectCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
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
				Type:    clusterv1.ClusterControlPlaneMachinesUpToDateV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterControlPlaneMachinesUpToDateInternalErrorV1Beta2Reason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:                    "no machines",
			cluster:                 fakeCluster("c"),
			machines:                []*clusterv1.Machine{},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneMachinesUpToDateV1Beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.ClusterControlPlaneMachinesUpToDateNoReplicasV1Beta2Reason,
				Message: "",
			},
		},
		{
			name:    "One machine up-to-date",
			cluster: fakeCluster("c"),
			machines: []*clusterv1.Machine{
				fakeMachine("up-to-date-1", controlPlane(true), v1beta2Condition(metav1.Condition{
					Type:   clusterv1.MachineUpToDateV1Beta2Condition,
					Status: metav1.ConditionTrue,
					Reason: "some-reason-1",
				})),
			},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneMachinesUpToDateV1Beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.ClusterControlPlaneMachinesUpToDateV1Beta2Reason,
				Message: "",
			},
		},
		{
			name:    "One machine unknown",
			cluster: fakeCluster("c"),
			machines: []*clusterv1.Machine{
				fakeMachine("unknown-1", controlPlane(true), v1beta2Condition(metav1.Condition{
					Type:    clusterv1.MachineUpToDateV1Beta2Condition,
					Status:  metav1.ConditionUnknown,
					Reason:  "some-unknown-reason-1",
					Message: "some unknown message",
				})),
			},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneMachinesUpToDateV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterControlPlaneMachinesUpToDateUnknownV1Beta2Reason,
				Message: "* Machine unknown-1: some unknown message",
			},
		},
		{
			name:    "One machine not up-to-date",
			cluster: fakeCluster("c"),
			machines: []*clusterv1.Machine{
				fakeMachine("not-up-to-date-machine-1", controlPlane(true), v1beta2Condition(metav1.Condition{
					Type:    clusterv1.MachineUpToDateV1Beta2Condition,
					Status:  metav1.ConditionFalse,
					Reason:  "some-not-up-to-date-reason",
					Message: "some not up-to-date message",
				})),
			},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneMachinesUpToDateV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterControlPlaneMachinesNotUpToDateV1Beta2Reason,
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
				Type:    clusterv1.ClusterControlPlaneMachinesUpToDateV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterControlPlaneMachinesUpToDateUnknownV1Beta2Reason,
				Message: "* Machine no-condition-machine-1: Condition UpToDate not yet reported",
			},
		},
		{
			name:    "Two machines not up-to-date, two up-to-date, two not reported",
			cluster: fakeCluster("c"),
			machines: []*clusterv1.Machine{
				fakeMachine("up-to-date-1", controlPlane(true), v1beta2Condition(metav1.Condition{
					Type:   clusterv1.MachineUpToDateV1Beta2Condition,
					Status: metav1.ConditionTrue,
					Reason: "TestUpToDate",
				})),
				fakeMachine("up-to-date-2", controlPlane(true), v1beta2Condition(metav1.Condition{
					Type:   clusterv1.MachineUpToDateV1Beta2Condition,
					Status: metav1.ConditionTrue,
					Reason: "TestUpToDate",
				})),
				fakeMachine("not-up-to-date-machine-1", controlPlane(true), v1beta2Condition(metav1.Condition{
					Type:    clusterv1.MachineUpToDateV1Beta2Condition,
					Status:  metav1.ConditionFalse,
					Reason:  "TestNotUpToDate",
					Message: "This is not up-to-date message",
				})),
				fakeMachine("not-up-to-date-machine-2", controlPlane(true), v1beta2Condition(metav1.Condition{
					Type:    clusterv1.MachineUpToDateV1Beta2Condition,
					Status:  metav1.ConditionFalse,
					Reason:  "TestNotUpToDate",
					Message: "This is not up-to-date message",
				})),
				fakeMachine("no-condition-machine-1", controlPlane(true)),
				fakeMachine("no-condition-machine-2", controlPlane(true)),
			},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterControlPlaneMachinesUpToDateV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.ClusterControlPlaneMachinesNotUpToDateV1Beta2Reason,
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

			condition := v1beta2conditions.Get(tt.cluster, clusterv1.ClusterControlPlaneMachinesUpToDateV1Beta2Condition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(v1beta2conditions.MatchCondition(tt.expectCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
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
				Type:    clusterv1.ClusterWorkerMachinesUpToDateV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterWorkerMachinesUpToDateInternalErrorV1Beta2Reason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:                    "no machines",
			cluster:                 fakeCluster("c"),
			machines:                []*clusterv1.Machine{},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterWorkerMachinesUpToDateV1Beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.ClusterWorkerMachinesUpToDateNoReplicasV1Beta2Reason,
				Message: "",
			},
		},
		{
			name:    "One machine up-to-date",
			cluster: fakeCluster("c"),
			machines: []*clusterv1.Machine{
				fakeMachine("up-to-date-1", v1beta2Condition(metav1.Condition{
					Type:   clusterv1.MachineUpToDateV1Beta2Condition,
					Status: metav1.ConditionTrue,
					Reason: "some-reason-1",
				})),
			},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterWorkerMachinesUpToDateV1Beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.ClusterWorkerMachinesUpToDateV1Beta2Reason,
				Message: "",
			},
		},
		{
			name:    "One machine unknown",
			cluster: fakeCluster("c"),
			machines: []*clusterv1.Machine{
				fakeMachine("unknown-1", v1beta2Condition(metav1.Condition{
					Type:    clusterv1.MachineUpToDateV1Beta2Condition,
					Status:  metav1.ConditionUnknown,
					Reason:  "some-unknown-reason-1",
					Message: "some unknown message",
				})),
			},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterWorkerMachinesUpToDateV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterWorkerMachinesUpToDateUnknownV1Beta2Reason,
				Message: "* Machine unknown-1: some unknown message",
			},
		},
		{
			name:    "One machine not up-to-date",
			cluster: fakeCluster("c"),
			machines: []*clusterv1.Machine{
				fakeMachine("not-up-to-date-machine-1", v1beta2Condition(metav1.Condition{
					Type:    clusterv1.MachineUpToDateV1Beta2Condition,
					Status:  metav1.ConditionFalse,
					Reason:  "some-not-up-to-date-reason",
					Message: "some not up-to-date message",
				})),
			},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterWorkerMachinesUpToDateV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterWorkerMachinesNotUpToDateV1Beta2Reason,
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
				Type:    clusterv1.ClusterWorkerMachinesUpToDateV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterWorkerMachinesUpToDateUnknownV1Beta2Reason,
				Message: "* Machine no-condition-machine-1: Condition UpToDate not yet reported",
			},
		},
		{
			name:    "Two machines not up-to-date, two up-to-date, two not reported",
			cluster: fakeCluster("c"),
			machines: []*clusterv1.Machine{
				fakeMachine("up-to-date-1", v1beta2Condition(metav1.Condition{
					Type:   clusterv1.MachineUpToDateV1Beta2Condition,
					Status: metav1.ConditionTrue,
					Reason: "TestUpToDate",
				})),
				fakeMachine("up-to-date-2", v1beta2Condition(metav1.Condition{
					Type:   clusterv1.MachineUpToDateV1Beta2Condition,
					Status: metav1.ConditionTrue,
					Reason: "TestUpToDate",
				})),
				fakeMachine("not-up-to-date-machine-1", v1beta2Condition(metav1.Condition{
					Type:    clusterv1.MachineUpToDateV1Beta2Condition,
					Status:  metav1.ConditionFalse,
					Reason:  "TestNotUpToDate",
					Message: "This is not up-to-date message",
				})),
				fakeMachine("not-up-to-date-machine-2", v1beta2Condition(metav1.Condition{
					Type:    clusterv1.MachineUpToDateV1Beta2Condition,
					Status:  metav1.ConditionFalse,
					Reason:  "TestNotUpToDate",
					Message: "This is not up-to-date message",
				})),
				fakeMachine("no-condition-machine-1"),
				fakeMachine("no-condition-machine-2"),
			},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterWorkerMachinesUpToDateV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.ClusterWorkerMachinesNotUpToDateV1Beta2Reason,
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

			condition := v1beta2conditions.Get(tt.cluster, clusterv1.ClusterWorkerMachinesUpToDateV1Beta2Condition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(v1beta2conditions.MatchCondition(tt.expectCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func TestSetRollingOutCondition(t *testing.T) {
	tests := []struct {
		name                    string
		cluster                 *clusterv1.Cluster
		controlPlane            *unstructured.Unstructured
		controlPlaneIsNotFound  bool
		machinePools            expv1.MachinePoolList
		machineDeployments      clusterv1.MachineDeploymentList
		getDescendantsSucceeded bool
		expectCondition         metav1.Condition
	}{
		{
			name:                    "cluster with controlplane, unknown if failed to get descendants",
			cluster:                 fakeCluster("c", controlPlaneRef{}),
			controlPlane:            nil,
			controlPlaneIsNotFound:  true,
			getDescendantsSucceeded: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterRollingOutV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterRollingOutInternalErrorV1Beta2Reason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:                    "cluster with controlplane, unknown if failed to get control plane",
			cluster:                 fakeCluster("c", controlPlaneRef{}),
			controlPlane:            nil,
			controlPlaneIsNotFound:  false,
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterRollingOutV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterRollingOutInternalErrorV1Beta2Reason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:                    "cluster with controlplane, false if no control plane & descendants",
			cluster:                 fakeCluster("c", controlPlaneRef{}),
			controlPlaneIsNotFound:  true,
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterRollingOutV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.ClusterNotRollingOutV1Beta2Reason,
			},
		},
		{
			name:         "cluster with controlplane, control plane & descendants do not report rolling out",
			cluster:      fakeCluster("c", controlPlaneRef{}),
			controlPlane: fakeControlPlane("cp1"),
			machinePools: expv1.MachinePoolList{Items: []expv1.MachinePool{
				*fakeMachinePool("mp1"),
			}},
			machineDeployments: clusterv1.MachineDeploymentList{Items: []clusterv1.MachineDeployment{
				*fakeMachineDeployment("md1"),
			}},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterRollingOutV1Beta2Condition,
				Status: metav1.ConditionUnknown,
				Reason: clusterv1.ClusterRollingOutUnknownV1Beta2Reason,
				Message: "* MachineDeployment md1: Condition RollingOut not yet reported\n" +
					"* MachinePool mp1: Condition RollingOut not yet reported",
			},
		},
		{
			name:    "cluster with controlplane, control plane and descendants report rolling out",
			cluster: fakeCluster("c", controlPlaneRef{}),
			controlPlane: fakeControlPlane("cp1", condition{
				Type:    clusterv1.ClusterRollingOutV1Beta2Condition,
				Status:  corev1.ConditionTrue,
				Reason:  clusterv1.RollingOutV1Beta2Reason,
				Message: "Rolling out 3 not up-to-date replicas",
			}),
			machinePools: expv1.MachinePoolList{Items: []expv1.MachinePool{
				*fakeMachinePool("mp1", v1beta2Condition{
					Type:    clusterv1.RollingOutV1Beta2Condition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.RollingOutV1Beta2Reason,
					Message: "Rolling out 5 not up-to-date replicas",
				}),
			}},
			machineDeployments: clusterv1.MachineDeploymentList{Items: []clusterv1.MachineDeployment{
				*fakeMachineDeployment("md1", v1beta2Condition{
					Type:    clusterv1.MachineDeploymentRollingOutV1Beta2Condition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.MachineDeploymentRollingOutV1Beta2Reason,
					Message: "Rolling out 4 not up-to-date replicas",
				}),
			}},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterRollingOutV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterRollingOutV1Beta2Reason,
				Message: "* FakeControlPlane cp1: Rolling out 3 not up-to-date replicas\n" +
					"* MachineDeployment md1: Rolling out 4 not up-to-date replicas\n" +
					"* MachinePool mp1: Rolling out 5 not up-to-date replicas",
			},
		},
		{
			name:         "cluster with controlplane, control plane not reporting conditions, descendants report rolling out",
			cluster:      fakeCluster("c", controlPlaneRef{}),
			controlPlane: fakeControlPlane("cp1"),
			machinePools: expv1.MachinePoolList{Items: []expv1.MachinePool{
				*fakeMachinePool("mp1", v1beta2Condition{
					Type:    clusterv1.RollingOutV1Beta2Condition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.RollingOutV1Beta2Reason,
					Message: "Rolling out 3 not up-to-date replicas",
				}),
			}},
			machineDeployments: clusterv1.MachineDeploymentList{Items: []clusterv1.MachineDeployment{
				*fakeMachineDeployment("md1", v1beta2Condition{
					Type:    clusterv1.MachineDeploymentRollingOutV1Beta2Condition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.MachineDeploymentRollingOutV1Beta2Reason,
					Message: "Rolling out 5 not up-to-date replicas",
				}),
			}},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterRollingOutV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterRollingOutV1Beta2Reason,
				Message: "* MachineDeployment md1: Rolling out 5 not up-to-date replicas\n" +
					"* MachinePool mp1: Rolling out 3 not up-to-date replicas",
			},
		},
		{
			name:                    "cluster without controlplane, unknown if failed to get descendants",
			cluster:                 fakeCluster("c"),
			getDescendantsSucceeded: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterRollingOutV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterRollingOutInternalErrorV1Beta2Reason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:                    "cluster without controlplane, false if no descendants",
			cluster:                 fakeCluster("c"),
			controlPlaneIsNotFound:  true,
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterRollingOutV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.ClusterNotRollingOutV1Beta2Reason,
			},
		},
		{
			name:    "cluster without controlplane, descendants report rolling out",
			cluster: fakeCluster("c"),
			machinePools: expv1.MachinePoolList{Items: []expv1.MachinePool{
				*fakeMachinePool("mp1", v1beta2Condition{
					Type:    clusterv1.RollingOutV1Beta2Condition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.RollingOutV1Beta2Reason,
					Message: "Rolling out 3 not up-to-date replicas",
				}),
			}},
			machineDeployments: clusterv1.MachineDeploymentList{Items: []clusterv1.MachineDeployment{
				*fakeMachineDeployment("md1", v1beta2Condition{
					Type:    clusterv1.MachineDeploymentRollingOutV1Beta2Condition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.MachineDeploymentRollingOutV1Beta2Reason,
					Message: "Rolling out 5 not up-to-date replicas",
				}),
			}},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterRollingOutV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterRollingOutV1Beta2Reason,
				Message: "* MachineDeployment md1: Rolling out 5 not up-to-date replicas\n" +
					"* MachinePool mp1: Rolling out 3 not up-to-date replicas",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			setRollingOutCondition(ctx, tt.cluster, tt.controlPlane, tt.machinePools, tt.machineDeployments, tt.controlPlaneIsNotFound, tt.getDescendantsSucceeded)

			condition := v1beta2conditions.Get(tt.cluster, clusterv1.ClusterRollingOutV1Beta2Condition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(v1beta2conditions.MatchCondition(tt.expectCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func TestSetScalingUpCondition(t *testing.T) {
	tests := []struct {
		name                    string
		cluster                 *clusterv1.Cluster
		controlPlane            *unstructured.Unstructured
		controlPlaneIsNotFound  bool
		machinePools            expv1.MachinePoolList
		machineDeployments      clusterv1.MachineDeploymentList
		machineSets             clusterv1.MachineSetList
		getDescendantsSucceeded bool
		expectCondition         metav1.Condition
	}{
		{
			name:                    "cluster with controlplane, unknown if failed to get descendants",
			cluster:                 fakeCluster("c", controlPlaneRef{}),
			controlPlane:            nil,
			controlPlaneIsNotFound:  true,
			getDescendantsSucceeded: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterScalingUpV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterScalingUpInternalErrorV1Beta2Reason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:                    "cluster with controlplane, unknown if failed to get control plane",
			cluster:                 fakeCluster("c", controlPlaneRef{}),
			controlPlane:            nil,
			controlPlaneIsNotFound:  false,
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterScalingUpV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterScalingUpInternalErrorV1Beta2Reason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:                    "cluster with controlplane, false if no control plane & descendants",
			cluster:                 fakeCluster("c", controlPlaneRef{}),
			controlPlaneIsNotFound:  true,
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterScalingUpV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.ClusterNotScalingUpV1Beta2Reason,
			},
		},
		{
			name:         "cluster with controlplane, control plane & descendants do not report scaling up",
			cluster:      fakeCluster("c", controlPlaneRef{}),
			controlPlane: fakeControlPlane("cp1"),
			machinePools: expv1.MachinePoolList{Items: []expv1.MachinePool{
				*fakeMachinePool("mp1"),
			}},
			machineDeployments: clusterv1.MachineDeploymentList{Items: []clusterv1.MachineDeployment{
				*fakeMachineDeployment("md1"),
			}},
			machineSets: clusterv1.MachineSetList{Items: []clusterv1.MachineSet{
				*fakeMachineSet("ms1", OwnedByCluster("c")),
				*fakeMachineSet("ms2", v1beta2Condition{
					Type:    clusterv1.ClusterScalingUpV1Beta2Condition,
					Status:  metav1.ConditionFalse,
					Reason:  "Foo",
					Message: "foo",
				}), // not owned by the cluster
			}},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterScalingUpV1Beta2Condition,
				Status: metav1.ConditionUnknown,
				Reason: clusterv1.ClusterScalingUpUnknownV1Beta2Reason,
				Message: "* MachineDeployment md1: Condition ScalingUp not yet reported\n" +
					"* MachinePool mp1: Condition ScalingUp not yet reported\n" +
					"* MachineSet ms1: Condition ScalingUp not yet reported",
			},
		},
		{
			name:    "cluster with controlplane, control plane and descendants report scaling up",
			cluster: fakeCluster("c", controlPlaneRef{}),
			controlPlane: fakeControlPlane("cp1", condition{
				Type:    clusterv1.ClusterScalingUpV1Beta2Condition,
				Status:  corev1.ConditionTrue,
				Reason:  clusterv1.ScalingUpV1Beta2Reason,
				Message: "Scaling up from 0 to 3 replicas",
			}),
			machinePools: expv1.MachinePoolList{Items: []expv1.MachinePool{
				*fakeMachinePool("mp1", v1beta2Condition{
					Type:    clusterv1.ScalingUpV1Beta2Condition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.ScalingUpV1Beta2Reason,
					Message: "Scaling up from 0 to 3 replicas",
				}),
			}},
			machineDeployments: clusterv1.MachineDeploymentList{Items: []clusterv1.MachineDeployment{
				*fakeMachineDeployment("md1", v1beta2Condition{
					Type:    clusterv1.ClusterScalingUpV1Beta2Condition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.MachineDeploymentScalingUpV1Beta2Reason,
					Message: "Scaling up from 1 to 5 replicas",
				}),
			}},
			machineSets: clusterv1.MachineSetList{Items: []clusterv1.MachineSet{
				*fakeMachineSet("ms1", OwnedByCluster("c"), v1beta2Condition{
					Type:    clusterv1.ClusterScalingUpV1Beta2Condition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.MachineSetScalingUpV1Beta2Reason,
					Message: "Scaling up from 2 to 7 replicas",
				}),
				*fakeMachineSet("ms2", v1beta2Condition{
					Type:    clusterv1.ClusterScalingUpV1Beta2Condition,
					Status:  metav1.ConditionTrue,
					Reason:  "Foo",
					Message: "foo",
				}), // not owned by the cluster
			}},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterScalingUpV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterScalingUpV1Beta2Reason,
				Message: "* FakeControlPlane cp1: Scaling up from 0 to 3 replicas\n" +
					"* MachineDeployment md1: Scaling up from 1 to 5 replicas\n" +
					"* MachinePool mp1: Scaling up from 0 to 3 replicas\n" +
					"And 1 MachineSet with other issues",
			},
		},
		{
			name:         "cluster with controlplane, control plane not reporting conditions, descendants report scaling up",
			cluster:      fakeCluster("c", controlPlaneRef{}),
			controlPlane: fakeControlPlane("cp1"),
			machinePools: expv1.MachinePoolList{Items: []expv1.MachinePool{
				*fakeMachinePool("mp1", v1beta2Condition{
					Type:    clusterv1.ScalingUpV1Beta2Condition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.ScalingUpV1Beta2Reason,
					Message: "Scaling up from 0 to 3 replicas",
				}),
			}},
			machineDeployments: clusterv1.MachineDeploymentList{Items: []clusterv1.MachineDeployment{
				*fakeMachineDeployment("md1", v1beta2Condition{
					Type:    clusterv1.ClusterScalingUpV1Beta2Condition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.MachineDeploymentScalingUpV1Beta2Reason,
					Message: "Scaling up from 1 to 5 replicas",
				}),
			}},
			machineSets: clusterv1.MachineSetList{Items: []clusterv1.MachineSet{
				*fakeMachineSet("ms1", OwnedByCluster("c"), v1beta2Condition{
					Type:    clusterv1.ClusterScalingUpV1Beta2Condition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.MachineSetScalingUpV1Beta2Reason,
					Message: "Scaling up from 2 to 7 replicas",
				}),
				*fakeMachineSet("ms2", v1beta2Condition{
					Type:    clusterv1.ClusterScalingUpV1Beta2Condition,
					Status:  metav1.ConditionTrue,
					Reason:  "Foo",
					Message: "foo",
				}), // not owned by the cluster
			}},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterScalingUpV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterScalingUpV1Beta2Reason,
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
				Type:    clusterv1.ClusterScalingUpV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterScalingUpInternalErrorV1Beta2Reason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:                    "cluster without controlplane, false if no descendants",
			cluster:                 fakeCluster("c"),
			controlPlaneIsNotFound:  true,
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterScalingUpV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.ClusterNotScalingUpV1Beta2Reason,
			},
		},
		{
			name:    "cluster without controlplane, descendants report scaling up",
			cluster: fakeCluster("c"),
			machinePools: expv1.MachinePoolList{Items: []expv1.MachinePool{
				*fakeMachinePool("mp1", v1beta2Condition{
					Type:    clusterv1.ScalingUpV1Beta2Condition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.ScalingUpV1Beta2Reason,
					Message: "Scaling up from 0 to 3 replicas",
				}),
			}},
			machineDeployments: clusterv1.MachineDeploymentList{Items: []clusterv1.MachineDeployment{
				*fakeMachineDeployment("md1", v1beta2Condition{
					Type:    clusterv1.ClusterScalingUpV1Beta2Condition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.MachineDeploymentScalingUpV1Beta2Reason,
					Message: "Scaling up from 1 to 5 replicas",
				}),
			}},
			machineSets: clusterv1.MachineSetList{Items: []clusterv1.MachineSet{
				*fakeMachineSet("ms1", OwnedByCluster("c"), v1beta2Condition{
					Type:    clusterv1.ClusterScalingUpV1Beta2Condition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.MachineSetScalingUpV1Beta2Reason,
					Message: "Scaling up from 2 to 7 replicas",
				}),
				*fakeMachineSet("ms2", v1beta2Condition{
					Type:    clusterv1.ClusterScalingUpV1Beta2Condition,
					Status:  metav1.ConditionTrue,
					Reason:  "Foo",
					Message: "foo",
				}), // not owned by the cluster
			}},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterScalingUpV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterScalingUpV1Beta2Reason,
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

			condition := v1beta2conditions.Get(tt.cluster, clusterv1.ClusterScalingUpV1Beta2Condition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(v1beta2conditions.MatchCondition(tt.expectCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func TestSetScalingDownCondition(t *testing.T) {
	tests := []struct {
		name                    string
		cluster                 *clusterv1.Cluster
		controlPlane            *unstructured.Unstructured
		controlPlaneIsNotFound  bool
		machinePools            expv1.MachinePoolList
		machineDeployments      clusterv1.MachineDeploymentList
		machineSets             clusterv1.MachineSetList
		getDescendantsSucceeded bool
		expectCondition         metav1.Condition
	}{
		{
			name:                    "cluster with controlplane, unknown if failed to get descendants",
			cluster:                 fakeCluster("c", controlPlaneRef{}),
			controlPlane:            nil,
			controlPlaneIsNotFound:  true,
			getDescendantsSucceeded: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterScalingDownV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterScalingDownInternalErrorV1Beta2Reason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:                    "cluster with controlplane, unknown if failed to get control plane",
			cluster:                 fakeCluster("c", controlPlaneRef{}),
			controlPlane:            nil,
			controlPlaneIsNotFound:  false,
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterScalingDownV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterScalingDownInternalErrorV1Beta2Reason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:                    "cluster with controlplane, false if no control plane & descendants",
			cluster:                 fakeCluster("c", controlPlaneRef{}),
			controlPlaneIsNotFound:  true,
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterScalingDownV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.ClusterNotScalingDownV1Beta2Reason,
			},
		},
		{
			name:         "cluster with controlplane, control plane & descendants do not report scaling down",
			cluster:      fakeCluster("c", controlPlaneRef{}),
			controlPlane: fakeControlPlane("cp1"),
			machinePools: expv1.MachinePoolList{Items: []expv1.MachinePool{
				*fakeMachinePool("mp1"),
			}},
			machineDeployments: clusterv1.MachineDeploymentList{Items: []clusterv1.MachineDeployment{
				*fakeMachineDeployment("md1"),
			}},
			machineSets: clusterv1.MachineSetList{Items: []clusterv1.MachineSet{
				*fakeMachineSet("ms1", OwnedByCluster("c")),
				*fakeMachineSet("ms2", v1beta2Condition{
					Type:    clusterv1.ClusterScalingDownV1Beta2Condition,
					Status:  metav1.ConditionFalse,
					Reason:  "Foo",
					Message: "foo",
				}), // not owned by the cluster
			}},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterScalingDownV1Beta2Condition,
				Status: metav1.ConditionUnknown,
				Reason: clusterv1.ClusterScalingDownUnknownV1Beta2Reason,
				Message: "* MachineDeployment md1: Condition ScalingDown not yet reported\n" +
					"* MachinePool mp1: Condition ScalingDown not yet reported\n" +
					"* MachineSet ms1: Condition ScalingDown not yet reported",
			},
		},
		{
			name:    "cluster with controlplane, control plane and descendants report scaling down",
			cluster: fakeCluster("c", controlPlaneRef{}),
			controlPlane: fakeControlPlane("cp1", condition{
				Type:    clusterv1.ClusterScalingDownV1Beta2Condition,
				Status:  corev1.ConditionTrue,
				Reason:  "Foo",
				Message: "Scaling down from 0 to 3 replicas",
			}),
			machinePools: expv1.MachinePoolList{Items: []expv1.MachinePool{
				*fakeMachinePool("mp1", v1beta2Condition{
					Type:    clusterv1.ClusterScalingDownV1Beta2Condition,
					Status:  metav1.ConditionTrue,
					Reason:  "Foo",
					Message: "Scaling down from 0 to 3 replicas",
				}),
			}},
			machineDeployments: clusterv1.MachineDeploymentList{Items: []clusterv1.MachineDeployment{
				*fakeMachineDeployment("md1", v1beta2Condition{
					Type:    clusterv1.ClusterScalingDownV1Beta2Condition,
					Status:  metav1.ConditionTrue,
					Reason:  "Foo",
					Message: "Scaling down from 1 to 5 replicas",
				}),
			}},
			machineSets: clusterv1.MachineSetList{Items: []clusterv1.MachineSet{
				*fakeMachineSet("ms1", OwnedByCluster("c"), v1beta2Condition{
					Type:    clusterv1.ClusterScalingDownV1Beta2Condition,
					Status:  metav1.ConditionTrue,
					Reason:  "Foo",
					Message: "Scaling down from 2 to 7 replicas",
				}),
				*fakeMachineSet("ms2", v1beta2Condition{
					Type:    clusterv1.ClusterScalingDownV1Beta2Condition,
					Status:  metav1.ConditionTrue,
					Reason:  "Foo",
					Message: "foo",
				}), // not owned by the cluster
			}},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterScalingDownV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterScalingDownV1Beta2Reason,
				Message: "* FakeControlPlane cp1: Scaling down from 0 to 3 replicas\n" +
					"* MachineDeployment md1: Scaling down from 1 to 5 replicas\n" +
					"* MachinePool mp1: Scaling down from 0 to 3 replicas\n" +
					"And 1 MachineSet with other issues",
			},
		},
		{
			name:         "cluster with controlplane, control plane not reporting conditions, descendants report scaling down",
			cluster:      fakeCluster("c", controlPlaneRef{}),
			controlPlane: fakeControlPlane("cp1"),
			machinePools: expv1.MachinePoolList{Items: []expv1.MachinePool{
				*fakeMachinePool("mp1", v1beta2Condition{
					Type:    clusterv1.ClusterScalingDownV1Beta2Condition,
					Status:  metav1.ConditionTrue,
					Reason:  "Foo",
					Message: "Scaling down from 0 to 3 replicas",
				}),
			}},
			machineDeployments: clusterv1.MachineDeploymentList{Items: []clusterv1.MachineDeployment{
				*fakeMachineDeployment("md1", v1beta2Condition{
					Type:    clusterv1.ClusterScalingDownV1Beta2Condition,
					Status:  metav1.ConditionTrue,
					Reason:  "Foo",
					Message: "Scaling down from 1 to 5 replicas",
				}),
			}},
			machineSets: clusterv1.MachineSetList{Items: []clusterv1.MachineSet{
				*fakeMachineSet("ms1", OwnedByCluster("c"), v1beta2Condition{
					Type:    clusterv1.ClusterScalingDownV1Beta2Condition,
					Status:  metav1.ConditionTrue,
					Reason:  "Foo",
					Message: "Scaling down from 2 to 7 replicas",
				}),
				*fakeMachineSet("ms2", v1beta2Condition{
					Type:    clusterv1.ClusterScalingDownV1Beta2Condition,
					Status:  metav1.ConditionTrue,
					Reason:  "Foo",
					Message: "foo",
				}), // not owned by the cluster
			}},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterScalingDownV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterScalingDownV1Beta2Reason,
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
				Type:    clusterv1.ClusterScalingDownV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterScalingDownInternalErrorV1Beta2Reason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:                    "cluster without controlplane, false if no descendants",
			cluster:                 fakeCluster("c"),
			controlPlaneIsNotFound:  true,
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterScalingDownV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.ClusterNotScalingDownV1Beta2Reason,
			},
		},
		{
			name:    "cluster without controlplane, descendants report scaling down",
			cluster: fakeCluster("c"),
			machinePools: expv1.MachinePoolList{Items: []expv1.MachinePool{
				*fakeMachinePool("mp1", v1beta2Condition{
					Type:    clusterv1.ClusterScalingDownV1Beta2Condition,
					Status:  metav1.ConditionTrue,
					Reason:  "Foo",
					Message: "Scaling down from 0 to 3 replicas",
				}),
			}},
			machineDeployments: clusterv1.MachineDeploymentList{Items: []clusterv1.MachineDeployment{
				*fakeMachineDeployment("md1", v1beta2Condition{
					Type:    clusterv1.ClusterScalingDownV1Beta2Condition,
					Status:  metav1.ConditionTrue,
					Reason:  "Foo",
					Message: "Scaling down from 1 to 5 replicas",
				}),
			}},
			machineSets: clusterv1.MachineSetList{Items: []clusterv1.MachineSet{
				*fakeMachineSet("ms1", OwnedByCluster("c"), v1beta2Condition{
					Type:    clusterv1.ClusterScalingDownV1Beta2Condition,
					Status:  metav1.ConditionTrue,
					Reason:  "Foo",
					Message: "Scaling down from 2 to 7 replicas",
				}),
				*fakeMachineSet("ms2", v1beta2Condition{
					Type:    clusterv1.ClusterScalingDownV1Beta2Condition,
					Status:  metav1.ConditionTrue,
					Reason:  "Foo",
					Message: "foo",
				}), // not owned by the cluster
			}},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterScalingDownV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterScalingDownV1Beta2Reason,
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

			condition := v1beta2conditions.Get(tt.cluster, clusterv1.ClusterScalingDownV1Beta2Condition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(v1beta2conditions.MatchCondition(tt.expectCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func TestSetRemediatingCondition(t *testing.T) {
	healthCheckSucceeded := clusterv1.Condition{Type: clusterv1.MachineHealthCheckSucceededV1Beta2Condition, Status: corev1.ConditionTrue}
	healthCheckNotSucceeded := clusterv1.Condition{Type: clusterv1.MachineHealthCheckSucceededV1Beta2Condition, Status: corev1.ConditionFalse}
	ownerRemediated := clusterv1.Condition{Type: clusterv1.MachineOwnerRemediatedCondition, Status: corev1.ConditionFalse}
	ownerRemediatedV1Beta2 := metav1.Condition{Type: clusterv1.MachineOwnerRemediatedV1Beta2Condition, Status: metav1.ConditionFalse, Reason: clusterv1.MachineSetMachineRemediationMachineDeletingV1Beta2Reason, Message: "Machine is deleting"}

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
				Type:    clusterv1.ClusterRemediatingV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterRemediatingInternalErrorV1Beta2Reason,
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
				Type:   clusterv1.ClusterRemediatingV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.ClusterNotRemediatingV1Beta2Reason,
			},
		},
		{
			name:    "With machines to be remediated",
			cluster: fakeCluster("c"),
			machines: []*clusterv1.Machine{
				fakeMachine("m1", condition(healthCheckSucceeded)),    // Healthy machine
				fakeMachine("m2", condition(healthCheckNotSucceeded)), // Unhealthy machine, not yet marked for remediation
				fakeMachine("m3", condition(healthCheckNotSucceeded), condition(ownerRemediated), v1beta2Condition(ownerRemediatedV1Beta2)),
			},
			getDescendantsSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterRemediatingV1Beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.ClusterRemediatingV1Beta2Reason,
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
				Type:    clusterv1.ClusterRemediatingV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterNotRemediatingV1Beta2Reason,
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
				Type:    clusterv1.ClusterRemediatingV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterNotRemediatingV1Beta2Reason,
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

			condition := v1beta2conditions.Get(tt.cluster, clusterv1.ClusterRemediatingV1Beta2Condition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(v1beta2conditions.MatchCondition(tt.expectCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
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
				Type:   clusterv1.ClusterDeletingV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.ClusterNotDeletingV1Beta2Reason,
			},
		},
		{
			name:           "deletionTimestamp set (some reason/message reported)",
			cluster:        fakeCluster("c", deleted(true)),
			deletingReason: clusterv1.ClusterDeletingWaitingForWorkersDeletionV1Beta2Reason,
			deletingMessage: "* Control plane Machines: cp1, cp2, cp3\n" +
				"* MachineDeployments: md1, md2\n" +
				"* MachineSets: ms1, ms2\n" +
				"* Worker Machines: w1, w2, w3, w4, w5, ... (3 more)",
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterDeletingV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterDeletingWaitingForWorkersDeletionV1Beta2Reason,
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

			deletingCondition := v1beta2conditions.Get(tc.cluster, clusterv1.ClusterDeletingV1Beta2Condition)
			g.Expect(deletingCondition).ToNot(BeNil())
			g.Expect(*deletingCondition).To(v1beta2conditions.MatchCondition(tc.expectCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func TestSetAvailableCondition(t *testing.T) {
	testCases := []struct {
		name            string
		cluster         *clusterv1.Cluster
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
					Topology: nil, // not using CC
				},
				Status: clusterv1.ClusterStatus{
					V1Beta2: &clusterv1.ClusterV1Beta2Status{
						Conditions: []metav1.Condition{
							// No condition reported yet, the required ones should be reported as missing.
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterAvailableV1Beta2Condition,
				Status: metav1.ConditionUnknown,
				Reason: clusterv1.ClusterAvailableUnknownV1Beta2Reason,
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
					Topology: nil, // not using CC
				},
				Status: clusterv1.ClusterStatus{
					V1Beta2: &clusterv1.ClusterV1Beta2Status{
						Conditions: []metav1.Condition{
							{
								Type:   clusterv1.ClusterInfrastructureReadyV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.ClusterControlPlaneAvailableV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.ClusterWorkersAvailableV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.ClusterRemoteConnectionProbeV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.ClusterDeletingV1Beta2Condition,
								Status: metav1.ConditionFalse,
								Reason: "Foo",
							},
							// TopologyReconciled missing
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterAvailableV1Beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.ClusterAvailableV1Beta2Reason,
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
					Topology: nil, // not using CC
				},
				Status: clusterv1.ClusterStatus{
					V1Beta2: &clusterv1.ClusterV1Beta2Status{
						Conditions: []metav1.Condition{
							{
								Type:   clusterv1.ClusterInfrastructureReadyV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.ClusterControlPlaneAvailableV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.ClusterWorkersAvailableV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.ClusterRemoteConnectionProbeV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.ClusterDeletingV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: clusterv1.ClusterDeletingWaitingForWorkersDeletionV1Beta2Reason,
								Message: "* Control plane Machines: cp1, cp2, cp3\n" +
									"* MachineDeployments: md1, md2\n" +
									"* MachineSets: ms1, ms2\n" +
									"* Worker Machines: w1, w2, w3, w4, w5, ... (3 more)",
							},
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterAvailableV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.ClusterNotAvailableV1Beta2Reason,
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
					Topology: &clusterv1.Topology{}, // using CC
				},
				Status: clusterv1.ClusterStatus{
					V1Beta2: &clusterv1.ClusterV1Beta2Status{
						Conditions: []metav1.Condition{
							{
								Type:   clusterv1.ClusterInfrastructureReadyV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.ClusterControlPlaneAvailableV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.ClusterWorkersAvailableV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.ClusterRemoteConnectionProbeV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.ClusterDeletingV1Beta2Condition,
								Status: metav1.ConditionFalse,
								Reason: "Foo",
							},
							// TopologyReconciled missing
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterAvailableV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterAvailableUnknownV1Beta2Reason,
				Message: "* TopologyReconciled: Condition not yet reported",
			},
		},
		{
			name: "Takes into account Availability gates when defined",
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
					},
				},
				Status: clusterv1.ClusterStatus{
					V1Beta2: &clusterv1.ClusterV1Beta2Status{
						Conditions: []metav1.Condition{
							{
								Type:   clusterv1.ClusterInfrastructureReadyV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.ClusterControlPlaneAvailableV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.ClusterWorkersAvailableV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.ClusterRemoteConnectionProbeV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.ClusterDeletingV1Beta2Condition,
								Status: metav1.ConditionFalse,
								Reason: "Foo",
							},
							{
								Type:    "MyAvailabilityGate",
								Status:  metav1.ConditionFalse,
								Reason:  "SomeReason",
								Message: "Some message",
							},
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterAvailableV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterNotAvailableV1Beta2Reason,
				Message: "* MyAvailabilityGate: Some message",
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
					V1Beta2: &clusterv1.ClusterV1Beta2Status{
						Conditions: []metav1.Condition{
							{
								Type:   clusterv1.ClusterInfrastructureReadyV1Beta2Condition,
								Status: metav1.ConditionFalse,
								Reason: clusterv1.ClusterInfrastructureDeletedV1Beta2Reason,
							},
							{
								Type:   clusterv1.ClusterControlPlaneAvailableV1Beta2Condition,
								Status: metav1.ConditionFalse,
								Reason: clusterv1.ClusterControlPlaneDeletedV1Beta2Reason,
							},
							{
								Type:   clusterv1.ClusterWorkersAvailableV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.ClusterRemoteConnectionProbeV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:    clusterv1.ClusterDeletingV1Beta2Condition,
								Status:  metav1.ConditionTrue,
								Reason:  clusterv1.ClusterDeletingWaitingForBeforeDeleteHookV1Beta2Reason,
								Message: "Some message",
							},
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterAvailableV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterNotAvailableV1Beta2Reason,
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
					Topology: &clusterv1.Topology{}, // using CC
				},
				Status: clusterv1.ClusterStatus{
					V1Beta2: &clusterv1.ClusterV1Beta2Status{
						Conditions: []metav1.Condition{
							{
								Type:   clusterv1.ClusterInfrastructureReadyV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.ClusterControlPlaneAvailableV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.ClusterWorkersAvailableV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.ClusterRemoteConnectionProbeV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.ClusterDeletingV1Beta2Condition,
								Status: metav1.ConditionFalse,
								Reason: "Foo",
							},
							{
								Type:    clusterv1.ClusterTopologyReconciledV1Beta2Condition,
								Status:  metav1.ConditionFalse,
								Reason:  clusterv1.ClusterTopologyReconciledControlPlaneUpgradePendingV1Beta2Reason,
								Message: "Control plane rollout and upgrade to version v1.29.0 on hold.",
							},
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterAvailableV1Beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.ClusterAvailableV1Beta2Reason,
				Message: "* TopologyReconciled: Control plane rollout and upgrade to version v1.29.0 on hold.",
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
					Topology: &clusterv1.Topology{}, // using CC
				},
				Status: clusterv1.ClusterStatus{
					V1Beta2: &clusterv1.ClusterV1Beta2Status{
						Conditions: []metav1.Condition{
							{
								Type:   clusterv1.ClusterInfrastructureReadyV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.ClusterControlPlaneAvailableV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:    clusterv1.ClusterWorkersAvailableV1Beta2Condition,
								Status:  metav1.ConditionFalse,
								Reason:  clusterv1.ClusterWorkersNotAvailableV1Beta2Reason,
								Message: "3 available replicas, at least 4 required (spec.strategy.rollout.maxUnavailable is 1, spec.replicas is 5) from MachineDeployment md1; 2 available replicas, at least 3 required (spec.strategy.rollout.maxUnavailable is 1, spec.replicas is 4) from MachinePool mp1",
							},
							{
								Type:   clusterv1.ClusterRemoteConnectionProbeV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.ClusterDeletingV1Beta2Condition,
								Status: metav1.ConditionFalse,
								Reason: "Foo",
							},
							{
								Type:    clusterv1.ClusterTopologyReconciledV1Beta2Condition,
								Status:  metav1.ConditionFalse,
								Reason:  clusterv1.ClusterTopologyReconciledControlPlaneUpgradePendingV1Beta2Reason,
								Message: "Control plane rollout and upgrade to version v1.29.0 on hold.",
							},
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterAvailableV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterNotAvailableV1Beta2Reason,
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
					Topology: &clusterv1.Topology{}, // using CC
				},
				Status: clusterv1.ClusterStatus{
					V1Beta2: &clusterv1.ClusterV1Beta2Status{
						Conditions: []metav1.Condition{
							{
								Type:   clusterv1.ClusterInfrastructureReadyV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.ClusterControlPlaneAvailableV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.ClusterWorkersAvailableV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.ClusterRemoteConnectionProbeV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.ClusterDeletingV1Beta2Condition,
								Status: metav1.ConditionFalse,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.ClusterTopologyReconciledV1Beta2Condition,
								Status: metav1.ConditionFalse,
								Reason: clusterv1.ClusterTopologyReconciledClusterClassNotReconciledV1Beta2Reason,
								Message: "ClusterClass not reconciled. If this condition persists please check ClusterClass status. A ClusterClass is reconciled if" +
									".status.observedGeneration == .metadata.generation is true. If this is not the case either ClusterClass reconciliation failed or the ClusterClass is paused",
							},
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterAvailableV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.ClusterNotAvailableV1Beta2Reason,
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
					Topology: &clusterv1.Topology{}, // using CC
				},
				Status: clusterv1.ClusterStatus{
					V1Beta2: &clusterv1.ClusterV1Beta2Status{
						Conditions: []metav1.Condition{
							{
								Type:   clusterv1.ClusterInfrastructureReadyV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.ClusterControlPlaneAvailableV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:    clusterv1.ClusterWorkersAvailableV1Beta2Condition,
								Status:  metav1.ConditionFalse,
								Reason:  clusterv1.ClusterWorkersNotAvailableV1Beta2Reason,
								Message: "3 available replicas, at least 4 required (spec.strategy.rollout.maxUnavailable is 1, spec.replicas is 5) from MachineDeployment md1; 2 available replicas, at least 3 required (spec.strategy.rollout.maxUnavailable is 1, spec.replicas is 4) from MachinePool mp1",
							},
							{
								Type:   clusterv1.ClusterRemoteConnectionProbeV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.ClusterDeletingV1Beta2Condition,
								Status: metav1.ConditionFalse,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.ClusterTopologyReconciledV1Beta2Condition,
								Status: metav1.ConditionFalse,
								Reason: clusterv1.ClusterTopologyReconciledClusterClassNotReconciledV1Beta2Reason,
								Message: "ClusterClass not reconciled. If this condition persists please check ClusterClass status. A ClusterClass is reconciled if" +
									".status.observedGeneration == .metadata.generation is true. If this is not the case either ClusterClass reconciliation failed or the ClusterClass is paused",
							},
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterAvailableV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.ClusterNotAvailableV1Beta2Reason,
				Message: "* WorkersAvailable: 3 available replicas, at least 4 required (spec.strategy.rollout.maxUnavailable is 1, spec.replicas is 5) from MachineDeployment md1; 2 available replicas, at least 3 required (spec.strategy.rollout.maxUnavailable is 1, spec.replicas is 4) from MachinePool mp1\n" +
					"* TopologyReconciled: ClusterClass not reconciled. If this condition persists please check ClusterClass status. A ClusterClass is reconciled if.status.observedGeneration == .metadata.generation is true. If this is not the case either ClusterClass reconciliation failed or the ClusterClass is paused",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			setAvailableCondition(ctx, tc.cluster)

			condition := v1beta2conditions.Get(tc.cluster, clusterv1.ClusterAvailableV1Beta2Condition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(v1beta2conditions.MatchCondition(tc.expectCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

type fakeClusterOption interface {
	ApplyToCluster(c *clusterv1.Cluster)
}

func fakeCluster(name string, options ...fakeClusterOption) *clusterv1.Cluster {
	c := &clusterv1.Cluster{
		TypeMeta: metav1.TypeMeta{
			// Note: this is required by ownerRef checks
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "Cluster",
		},
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
	ApplyToMachinePool(mp *expv1.MachinePool)
}

func fakeMachinePool(name string, options ...fakeMachinePoolOption) *expv1.MachinePool {
	mp := &expv1.MachinePool{
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

type controlPlaneRef corev1.ObjectReference

func (r controlPlaneRef) ApplyToCluster(c *clusterv1.Cluster) {
	c.Spec.ControlPlaneRef = ptr.To(corev1.ObjectReference(r))
}

type infrastructureRef corev1.ObjectReference

func (r infrastructureRef) ApplyToCluster(c *clusterv1.Cluster) {
	c.Spec.InfrastructureRef = ptr.To(corev1.ObjectReference(r))
}

type infrastructureReady bool

func (r infrastructureReady) ApplyToCluster(c *clusterv1.Cluster) {
	c.Status.InfrastructureReady = bool(r)
}

type controlPlaneReady bool

func (r controlPlaneReady) ApplyToCluster(c *clusterv1.Cluster) {
	c.Status.ControlPlaneReady = bool(r)
}

type desiredReplicas int32

func (r desiredReplicas) ApplyToControlPlane(cp *unstructured.Unstructured) {
	_ = contract.ControlPlane().Replicas().Set(cp, int64(r))
}

func (r desiredReplicas) ApplyToMachinePool(mp *expv1.MachinePool) {
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
	_ = contract.ControlPlane().StatusReplicas().Set(cp, int64(r))
}

func (r currentReplicas) ApplyToMachinePool(mp *expv1.MachinePool) {
	mp.Status.Replicas = int32(r)
}

func (r currentReplicas) ApplyToMachineDeployment(md *clusterv1.MachineDeployment) {
	md.Status.Replicas = int32(r)
}

func (r currentReplicas) ApplyToMachineSet(ms *clusterv1.MachineSet) {
	ms.Status.Replicas = int32(r)
}

type v1beta2ReadyReplicas int32

func (r v1beta2ReadyReplicas) ApplyToControlPlane(cp *unstructured.Unstructured) {
	_ = contract.ControlPlane().V1Beta2ReadyReplicas().Set(cp, int64(r))
}

func (r v1beta2ReadyReplicas) ApplyToMachinePool(mp *expv1.MachinePool) {
	if mp.Status.V1Beta2 == nil {
		mp.Status.V1Beta2 = &expv1.MachinePoolV1Beta2Status{}
	}
	mp.Status.V1Beta2.ReadyReplicas = ptr.To(int32(r))
}

func (r v1beta2ReadyReplicas) ApplyToMachineDeployment(md *clusterv1.MachineDeployment) {
	if md.Status.V1Beta2 == nil {
		md.Status.V1Beta2 = &clusterv1.MachineDeploymentV1Beta2Status{}
	}
	md.Status.V1Beta2.ReadyReplicas = ptr.To(int32(r))
}

func (r v1beta2ReadyReplicas) ApplyToMachineSet(ms *clusterv1.MachineSet) {
	if ms.Status.V1Beta2 == nil {
		ms.Status.V1Beta2 = &clusterv1.MachineSetV1Beta2Status{}
	}
	ms.Status.V1Beta2.ReadyReplicas = ptr.To(int32(r))
}

type v1beta2AvailableReplicas int32

func (r v1beta2AvailableReplicas) ApplyToControlPlane(cp *unstructured.Unstructured) {
	_ = contract.ControlPlane().V1Beta2AvailableReplicas().Set(cp, int64(r))
}

func (r v1beta2AvailableReplicas) ApplyToMachinePool(mp *expv1.MachinePool) {
	if mp.Status.V1Beta2 == nil {
		mp.Status.V1Beta2 = &expv1.MachinePoolV1Beta2Status{}
	}
	mp.Status.V1Beta2.AvailableReplicas = ptr.To(int32(r))
}

func (r v1beta2AvailableReplicas) ApplyToMachineDeployment(md *clusterv1.MachineDeployment) {
	if md.Status.V1Beta2 == nil {
		md.Status.V1Beta2 = &clusterv1.MachineDeploymentV1Beta2Status{}
	}
	md.Status.V1Beta2.AvailableReplicas = ptr.To(int32(r))
}

func (r v1beta2AvailableReplicas) ApplyToMachineSet(ms *clusterv1.MachineSet) {
	if ms.Status.V1Beta2 == nil {
		ms.Status.V1Beta2 = &clusterv1.MachineSetV1Beta2Status{}
	}
	ms.Status.V1Beta2.AvailableReplicas = ptr.To(int32(r))
}

type v1beta2UpToDateReplicas int32

func (r v1beta2UpToDateReplicas) ApplyToControlPlane(cp *unstructured.Unstructured) {
	_ = contract.ControlPlane().V1Beta2UpToDateReplicas().Set(cp, int64(r))
}

func (r v1beta2UpToDateReplicas) ApplyToMachinePool(mp *expv1.MachinePool) {
	if mp.Status.V1Beta2 == nil {
		mp.Status.V1Beta2 = &expv1.MachinePoolV1Beta2Status{}
	}
	mp.Status.V1Beta2.UpToDateReplicas = ptr.To(int32(r))
}

func (r v1beta2UpToDateReplicas) ApplyToMachineDeployment(md *clusterv1.MachineDeployment) {
	if md.Status.V1Beta2 == nil {
		md.Status.V1Beta2 = &clusterv1.MachineDeploymentV1Beta2Status{}
	}
	md.Status.V1Beta2.UpToDateReplicas = ptr.To(int32(r))
}

func (r v1beta2UpToDateReplicas) ApplyToMachineSet(ms *clusterv1.MachineSet) {
	if ms.Status.V1Beta2 == nil {
		ms.Status.V1Beta2 = &clusterv1.MachineSetV1Beta2Status{}
	}
	ms.Status.V1Beta2.UpToDateReplicas = ptr.To(int32(r))
}

type deleted bool

func (s deleted) ApplyToCluster(c *clusterv1.Cluster) {
	if s {
		c.SetDeletionTimestamp(ptr.To(metav1.Time{Time: time.Now()}))
		return
	}
	c.SetDeletionTimestamp(nil)
}

type v1beta2Condition metav1.Condition

func (c v1beta2Condition) ApplyToCluster(cluster *clusterv1.Cluster) {
	if cluster.Status.V1Beta2 == nil {
		cluster.Status.V1Beta2 = &clusterv1.ClusterV1Beta2Status{}
	}
	v1beta2conditions.Set(cluster, metav1.Condition(c))
}

func (c v1beta2Condition) ApplyToMachinePool(mp *expv1.MachinePool) {
	if mp.Status.V1Beta2 == nil {
		mp.Status.V1Beta2 = &expv1.MachinePoolV1Beta2Status{}
	}
	v1beta2conditions.Set(mp, metav1.Condition(c))
}

func (c v1beta2Condition) ApplyToMachineDeployment(md *clusterv1.MachineDeployment) {
	if md.Status.V1Beta2 == nil {
		md.Status.V1Beta2 = &clusterv1.MachineDeploymentV1Beta2Status{}
	}
	v1beta2conditions.Set(md, metav1.Condition(c))
}

func (c v1beta2Condition) ApplyToMachineSet(ms *clusterv1.MachineSet) {
	if ms.Status.V1Beta2 == nil {
		ms.Status.V1Beta2 = &clusterv1.MachineSetV1Beta2Status{}
	}
	v1beta2conditions.Set(ms, metav1.Condition(c))
}

func (c v1beta2Condition) ApplyToMachine(m *clusterv1.Machine) {
	if m.Status.V1Beta2 == nil {
		m.Status.V1Beta2 = &clusterv1.MachineV1Beta2Status{}
	}
	v1beta2conditions.Set(m, metav1.Condition(c))
}

type condition clusterv1.Condition

func (c condition) ApplyToMachine(m *clusterv1.Machine) {
	m.Status.Conditions = append(m.Status.Conditions, clusterv1.Condition(c))
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
		"type":               string(c.Type),
		"status":             string(c.Status),
		"reason":             c.Reason,
		"message":            c.Message,
		"severity":           string(c.Severity),
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

type ready bool

func (r ready) ApplyToControlPlane(cp *unstructured.Unstructured) {
	_ = contract.ControlPlane().Ready().Set(cp, bool(r))
}

func (r ready) ApplyToInfraCluster(i *unstructured.Unstructured) {
	_ = contract.InfrastructureCluster().Ready().Set(i, bool(r))
}

type initialized bool

func (r initialized) ApplyToControlPlane(cp *unstructured.Unstructured) {
	_ = contract.ControlPlane().Initialized().Set(cp, bool(r))
}

type creationTimestamp metav1.Time

func (t creationTimestamp) ApplyToMachine(m *clusterv1.Machine) {
	m.CreationTimestamp = metav1.Time(t)
}

type nodeRef corev1.ObjectReference

func (r nodeRef) ApplyToMachine(m *clusterv1.Machine) {
	m.Status.NodeRef = ptr.To(corev1.ObjectReference(r))
}

type topology bool

func (r topology) ApplyToCluster(c *clusterv1.Cluster) {
	c.Spec.Topology = &clusterv1.Topology{}
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
