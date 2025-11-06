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

package machineset

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimev1 "sigs.k8s.io/cluster-api/api/runtime/v1beta2"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
)

func Test_setReplicas(t *testing.T) {
	tests := []struct {
		name                                      string
		machines                                  []*clusterv1.Machine
		getAndAdoptMachinesForMachineSetSucceeded bool
		expectedStatus                            clusterv1.MachineSetStatus
	}{
		{
			name:     "getAndAdoptMachines failed",
			machines: nil,
			getAndAdoptMachinesForMachineSetSucceeded: false,
			expectedStatus: clusterv1.MachineSetStatus{},
		},
		{
			name:     "no machines",
			machines: nil,
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectedStatus: clusterv1.MachineSetStatus{
				Replicas:          ptr.To[int32](0),
				ReadyReplicas:     ptr.To[int32](0),
				AvailableReplicas: ptr.To[int32](0),
				UpToDateReplicas:  ptr.To[int32](0),
			}},
		{
			name: "should count only ready machines",
			machines: []*clusterv1.Machine{
				{Status: clusterv1.MachineStatus{Conditions: []metav1.Condition{{
					Type:   clusterv1.MachineReadyCondition,
					Status: metav1.ConditionTrue,
				}}}},
				{Status: clusterv1.MachineStatus{Conditions: []metav1.Condition{{
					Type:   clusterv1.MachineReadyCondition,
					Status: metav1.ConditionFalse,
				}}}},
				{Status: clusterv1.MachineStatus{Conditions: []metav1.Condition{{
					Type:   clusterv1.MachineReadyCondition,
					Status: metav1.ConditionUnknown,
				}}}},
			},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectedStatus: clusterv1.MachineSetStatus{
				Replicas:          ptr.To[int32](3),
				ReadyReplicas:     ptr.To[int32](1),
				AvailableReplicas: ptr.To[int32](0),
				UpToDateReplicas:  ptr.To[int32](0),
			},
		},
		{
			name: "should count only available machines",
			machines: []*clusterv1.Machine{
				{Status: clusterv1.MachineStatus{Conditions: []metav1.Condition{{
					Type:   clusterv1.MachineAvailableCondition,
					Status: metav1.ConditionTrue,
				}}}},
				{Status: clusterv1.MachineStatus{Conditions: []metav1.Condition{{
					Type:   clusterv1.MachineAvailableCondition,
					Status: metav1.ConditionFalse,
				}}}},
				{Status: clusterv1.MachineStatus{Conditions: []metav1.Condition{{
					Type:   clusterv1.MachineAvailableCondition,
					Status: metav1.ConditionUnknown,
				}}}},
			},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectedStatus: clusterv1.MachineSetStatus{
				Replicas:          ptr.To[int32](3),
				ReadyReplicas:     ptr.To[int32](0),
				AvailableReplicas: ptr.To[int32](1),
				UpToDateReplicas:  ptr.To[int32](0),
			},
		},
		{
			name: "should count only up-to-date machines",
			machines: []*clusterv1.Machine{
				{Status: clusterv1.MachineStatus{Conditions: []metav1.Condition{{
					Type:   clusterv1.MachineUpToDateCondition,
					Status: metav1.ConditionTrue,
				}}}},
				{Status: clusterv1.MachineStatus{Conditions: []metav1.Condition{{
					Type:   clusterv1.MachineUpToDateCondition,
					Status: metav1.ConditionFalse,
				}}}},
				{Status: clusterv1.MachineStatus{Conditions: []metav1.Condition{{
					Type:   clusterv1.MachineUpToDateCondition,
					Status: metav1.ConditionUnknown,
				}}}},
			},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectedStatus: clusterv1.MachineSetStatus{
				Replicas:          ptr.To[int32](3),
				ReadyReplicas:     ptr.To[int32](0),
				AvailableReplicas: ptr.To[int32](0),
				UpToDateReplicas:  ptr.To[int32](1),
			},
		},
		{
			name: "should count all conditions from a machine",
			machines: []*clusterv1.Machine{
				{Status: clusterv1.MachineStatus{Conditions: []metav1.Condition{
					{
						Type:   clusterv1.MachineReadyCondition,
						Status: metav1.ConditionTrue,
					},
					{
						Type:   clusterv1.MachineAvailableCondition,
						Status: metav1.ConditionTrue,
					},
					{
						Type:   clusterv1.MachineUpToDateCondition,
						Status: metav1.ConditionTrue,
					},
				}}},
			},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectedStatus: clusterv1.MachineSetStatus{
				Replicas:          ptr.To[int32](1),
				ReadyReplicas:     ptr.To[int32](1),
				AvailableReplicas: ptr.To[int32](1),
				UpToDateReplicas:  ptr.To[int32](1),
			},
		},
		{
			name: "In-place updating machines should not be counted",
			machines: []*clusterv1.Machine{
				{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							clusterv1.UpdateInProgressAnnotation: "",
						},
					},
					Status: clusterv1.MachineStatus{
						Conditions: []metav1.Condition{
							{
								Type:   clusterv1.MachineReadyCondition,
								Status: metav1.ConditionTrue,
							},
							{
								Type:   clusterv1.MachineAvailableCondition,
								Status: metav1.ConditionTrue,
							},
							{
								Type:   clusterv1.MachineUpToDateCondition,
								Status: metav1.ConditionTrue,
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							clusterv1.UpdateInProgressAnnotation: "",
							runtimev1.PendingHooksAnnotation:     "UpdateMachine",
						},
					},
					Status: clusterv1.MachineStatus{
						Conditions: []metav1.Condition{
							{
								Type:   clusterv1.MachineReadyCondition,
								Status: metav1.ConditionTrue,
							},
							{
								Type:   clusterv1.MachineAvailableCondition,
								Status: metav1.ConditionTrue,
							},
							{
								Type:   clusterv1.MachineUpToDateCondition,
								Status: metav1.ConditionTrue,
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							runtimev1.PendingHooksAnnotation: "UpdateMachine",
						},
					},
					Status: clusterv1.MachineStatus{
						Conditions: []metav1.Condition{
							{
								Type:   clusterv1.MachineReadyCondition,
								Status: metav1.ConditionTrue,
							},
							{
								Type:   clusterv1.MachineAvailableCondition,
								Status: metav1.ConditionTrue,
							},
							{
								Type:   clusterv1.MachineUpToDateCondition,
								Status: metav1.ConditionTrue,
							},
						},
					},
				},
			},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectedStatus: clusterv1.MachineSetStatus{
				Replicas:          ptr.To[int32](3),
				ReadyReplicas:     ptr.To[int32](0),
				AvailableReplicas: ptr.To[int32](0),
				UpToDateReplicas:  ptr.To[int32](0),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			ms := &clusterv1.MachineSet{}
			setReplicas(ctx, ms, tt.machines, tt.getAndAdoptMachinesForMachineSetSucceeded)
			g.Expect(ms.Status).To(BeEquivalentTo(tt.expectedStatus), cmp.Diff(tt.expectedStatus, ms.Status))
		})
	}
}

func Test_setScalingUpCondition(t *testing.T) {
	defaultMachineSet := &clusterv1.MachineSet{
		Spec: clusterv1.MachineSetSpec{
			Replicas: ptr.To[int32](0),
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: clusterv1.ContractVersionedObjectReference{
							Kind: "KubeadmBootstrapTemplate",
							Name: "some-name",
						},
					},
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						Kind: "DockerMachineTemplate",
						Name: "some-name",
					},
				},
			},
		},
	}

	scalingUpMachineSetWith3Replicas := defaultMachineSet.DeepCopy()
	scalingUpMachineSetWith3Replicas.Spec.Replicas = ptr.To[int32](3)

	deletingMachineSetWith3Replicas := defaultMachineSet.DeepCopy()
	deletingMachineSetWith3Replicas.DeletionTimestamp = ptr.To(metav1.Now())
	deletingMachineSetWith3Replicas.Spec.Replicas = ptr.To[int32](3)

	tests := []struct {
		name                                      string
		ms                                        *clusterv1.MachineSet
		machines                                  []*clusterv1.Machine
		bootstrapObjectNotFound                   bool
		infrastructureObjectNotFound              bool
		getAndAdoptMachinesForMachineSetSucceeded bool
		scaleUpPreflightCheckErrMessages          []string
		expectCondition                           metav1.Condition
	}{
		{
			name:                         "getAndAdoptMachines failed",
			ms:                           defaultMachineSet,
			bootstrapObjectNotFound:      false,
			infrastructureObjectNotFound: false,
			getAndAdoptMachinesForMachineSetSucceeded: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineSetScalingUpCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineSetScalingUpInternalErrorReason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:                         "not scaling up and no machines",
			ms:                           defaultMachineSet,
			bootstrapObjectNotFound:      false,
			infrastructureObjectNotFound: false,
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineSetScalingUpCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineSetNotScalingUpReason,
			},
		},
		{
			name:                         "not scaling up and no machines and bootstrapConfig object not found",
			ms:                           defaultMachineSet,
			bootstrapObjectNotFound:      true,
			infrastructureObjectNotFound: false,
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineSetScalingUpCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineSetNotScalingUpReason,
				Message: "Scaling up would be blocked because KubeadmBootstrapTemplate does not exist",
			},
		},
		{
			name:                         "not scaling up and no machines and infrastructure object not found",
			ms:                           defaultMachineSet,
			bootstrapObjectNotFound:      false,
			infrastructureObjectNotFound: true,
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineSetScalingUpCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineSetNotScalingUpReason,
				Message: "Scaling up would be blocked because DockerMachineTemplate does not exist",
			},
		},
		{
			name:                         "not scaling up and no machines and bootstrapConfig and infrastructure object not found",
			ms:                           defaultMachineSet,
			bootstrapObjectNotFound:      true,
			infrastructureObjectNotFound: true,
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineSetScalingUpCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineSetNotScalingUpReason,
				Message: "Scaling up would be blocked because KubeadmBootstrapTemplate and DockerMachineTemplate do not exist",
			},
		},
		{
			name:                         "scaling up",
			ms:                           scalingUpMachineSetWith3Replicas,
			bootstrapObjectNotFound:      false,
			infrastructureObjectNotFound: false,
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineSetScalingUpCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineSetScalingUpReason,
				Message: "Scaling up from 0 to 3 replicas",
			},
		},
		{
			name:                         "scaling up and blocked by bootstrap object",
			ms:                           scalingUpMachineSetWith3Replicas,
			bootstrapObjectNotFound:      true,
			infrastructureObjectNotFound: false,
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineSetScalingUpCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineSetScalingUpReason,
				Message: "Scaling up from 0 to 3 replicas is blocked because:\n" +
					"* KubeadmBootstrapTemplate does not exist",
			},
		},
		{
			name:                         "scaling up and blocked by infrastructure object",
			ms:                           scalingUpMachineSetWith3Replicas,
			bootstrapObjectNotFound:      false,
			infrastructureObjectNotFound: true,
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineSetScalingUpCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineSetScalingUpReason,
				Message: "Scaling up from 0 to 3 replicas is blocked because:\n" +
					"* DockerMachineTemplate does not exist",
			},
		},
		{
			name:                         "scaling up and blocked by bootstrap and infrastructure object and preflight checks",
			ms:                           scalingUpMachineSetWith3Replicas,
			bootstrapObjectNotFound:      true,
			infrastructureObjectNotFound: true,
			getAndAdoptMachinesForMachineSetSucceeded: true,
			// This preflight check error can happen when a MachineSet is scaling up while the control plane
			// already has a newer Kubernetes version.
			scaleUpPreflightCheckErrMessages: []string{"MachineSet version (1.25.5) and ControlPlane version (1.26.2) do not conform to kubeadm version skew policy as kubeadm only supports joining with the same major+minor version as the control plane (\"KubeadmVersionSkew\" preflight check failed)"},
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineSetScalingUpCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineSetScalingUpReason,
				Message: "Scaling up from 0 to 3 replicas is blocked because:\n" +
					"* MachineSet version (1.25.5) and ControlPlane version (1.26.2) do not conform to kubeadm version skew policy as kubeadm only supports joining with the same major+minor version as the control plane (\"KubeadmVersionSkew\" preflight check failed)\n" +
					"* KubeadmBootstrapTemplate and DockerMachineTemplate do not exist",
			},
		},
		{
			name:                         "deleting, don't show block message when templates are not found",
			ms:                           deletingMachineSetWith3Replicas,
			machines:                     []*clusterv1.Machine{{}, {}, {}},
			bootstrapObjectNotFound:      true,
			infrastructureObjectNotFound: true,
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineSetScalingUpCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineSetNotScalingUpReason,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			setScalingUpCondition(ctx, tt.ms, tt.machines, tt.bootstrapObjectNotFound, tt.infrastructureObjectNotFound, tt.getAndAdoptMachinesForMachineSetSucceeded, tt.scaleUpPreflightCheckErrMessages)

			condition := conditions.Get(tt.ms, clusterv1.MachineSetScalingUpCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tt.expectCondition, conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func Test_setScalingDownCondition(t *testing.T) {
	machineSet := &clusterv1.MachineSet{
		Spec: clusterv1.MachineSetSpec{
			Replicas: ptr.To[int32](0),
		},
	}

	machineSet1Replica := machineSet.DeepCopy()
	machineSet1Replica.Spec.Replicas = ptr.To[int32](1)

	deletingMachineSet := machineSet.DeepCopy()
	deletingMachineSet.Spec.Replicas = ptr.To[int32](1)
	deletingMachineSet.DeletionTimestamp = ptr.To(metav1.Now())

	tests := []struct {
		name                                      string
		ms                                        *clusterv1.MachineSet
		machines                                  []*clusterv1.Machine
		getAndAdoptMachinesForMachineSetSucceeded bool
		expectCondition                           metav1.Condition
	}{
		{
			name:     "getAndAdoptMachines failed",
			ms:       machineSet,
			machines: nil,
			getAndAdoptMachinesForMachineSetSucceeded: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineSetScalingDownCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineSetScalingDownInternalErrorReason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:     "not scaling down and no machines",
			ms:       machineSet,
			machines: []*clusterv1.Machine{},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineSetScalingDownCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineSetNotScalingDownReason,
			},
		},
		{
			name:     "not scaling down because scaling up",
			ms:       machineSet1Replica,
			machines: []*clusterv1.Machine{},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineSetScalingDownCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineSetNotScalingDownReason,
			},
		},
		{
			name: "scaling down to zero",
			ms:   machineSet,
			machines: []*clusterv1.Machine{
				fakeMachine("machine-1"),
			},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineSetScalingDownCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineSetScalingDownReason,
				Message: "Scaling down from 1 to 0 replicas",
			},
		},
		{
			name: "scaling down with 1 stale machine",
			ms:   machineSet1Replica,
			machines: []*clusterv1.Machine{
				fakeMachine("stale-machine-1", withStaleDeletionTimestamp(), func(m *clusterv1.Machine) {
					m.Status = clusterv1.MachineStatus{
						Conditions: []metav1.Condition{
							{
								Type:   clusterv1.MachineDeletingCondition,
								Status: metav1.ConditionTrue,
								Reason: clusterv1.MachineDeletingDrainingNodeReason,
								Message: `Drain not completed yet (started at 2024-10-09T16:13:59Z):
* Pods pod-2-deletionTimestamp-set-1, pod-3-to-trigger-eviction-successfully-1: deletionTimestamp set, but still not removed from the Node
* Pod pod-5-to-trigger-eviction-pdb-violated-1: cannot evict pod as it would violate the pod's disruption budget. The disruption budget pod-5-pdb needs 20 healthy pods and has 20 currently
* Pod pod-6-to-trigger-eviction-some-other-error: failed to evict Pod, some other error 1
* Pod pod-9-wait-completed: waiting for completion
After above Pods have been removed from the Node, the following Pods will be evicted: pod-7-eviction-later, pod-8-eviction-later`,
							},
						},
						Deletion: &clusterv1.MachineDeletionStatus{
							NodeDrainStartTime: metav1.Time{Time: time.Now().Add(-6 * time.Minute)},
						},
					}
				}),
				fakeMachine("machine-2"),
			},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineSetScalingDownCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineSetScalingDownReason,
				Message: "Scaling down from 2 to 1 replicas\n" +
					"* Machine stale-machine-1 is in deletion since more than 15m, delay likely due to PodDisruptionBudgets, Pods not terminating, Pod eviction errors, Pods not completed yet",
			},
		},
		{
			name: "scaling down with 3 stale machines",
			ms:   machineSet1Replica,
			machines: []*clusterv1.Machine{
				fakeMachine("stale-machine-2", withStaleDeletionTimestamp()),
				fakeMachine("stale-machine-1", withStaleDeletionTimestamp()),
				fakeMachine("stale-machine-3", withStaleDeletionTimestamp()),
				fakeMachine("machine-4"),
			},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineSetScalingDownCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineSetScalingDownReason,
				Message: "Scaling down from 4 to 1 replicas\n" +
					"* Machines stale-machine-1, stale-machine-2, stale-machine-3 are in deletion since more than 15m",
			},
		},
		{
			name: "scaling down with 5 stale machines",
			ms:   machineSet1Replica,
			machines: []*clusterv1.Machine{
				fakeMachine("stale-machine-5", withStaleDeletionTimestamp()),
				fakeMachine("stale-machine-4", withStaleDeletionTimestamp()),
				fakeMachine("stale-machine-2", withStaleDeletionTimestamp()),
				fakeMachine("stale-machine-3", withStaleDeletionTimestamp()),
				fakeMachine("stale-machine-1", withStaleDeletionTimestamp()),
				fakeMachine("machine-6"),
			},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineSetScalingDownCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineSetScalingDownReason,
				Message: "Scaling down from 6 to 1 replicas\n" +
					"* Machines stale-machine-1, stale-machine-2, stale-machine-3, ... (2 more) are in deletion since more than 15m",
			},
		},
		{
			name:     "deleting machineset without replicas",
			ms:       deletingMachineSet,
			machines: []*clusterv1.Machine{},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineSetScalingDownCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineSetNotScalingDownReason,
			},
		},
		{
			name: "deleting machineset having 1 replica",
			ms:   deletingMachineSet,
			machines: []*clusterv1.Machine{
				fakeMachine("machine-1"),
			},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineSetScalingDownCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineSetScalingDownReason,
				Message: "Scaling down from 1 to 0 replicas",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			setScalingDownCondition(ctx, tt.ms, tt.machines, tt.getAndAdoptMachinesForMachineSetSucceeded)

			condition := conditions.Get(tt.ms, clusterv1.MachineSetScalingDownCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tt.expectCondition, conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func Test_setMachinesReadyCondition(t *testing.T) {
	machineSet := &clusterv1.MachineSet{}

	readyCondition := metav1.Condition{
		Type:   clusterv1.MachineReadyCondition,
		Status: metav1.ConditionTrue,
		Reason: clusterv1.MachineReadyReason,
	}

	tests := []struct {
		name                                      string
		machineSet                                *clusterv1.MachineSet
		machines                                  []*clusterv1.Machine
		getAndAdoptMachinesForMachineSetSucceeded bool
		expectCondition                           metav1.Condition
	}{
		{
			name:       "getAndAdoptMachines failed",
			machineSet: machineSet,
			machines:   nil,
			getAndAdoptMachinesForMachineSetSucceeded: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineSetMachinesReadyCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineSetMachinesReadyInternalErrorReason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:       "no machines",
			machineSet: machineSet,
			machines:   []*clusterv1.Machine{},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineSetMachinesReadyCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineSetMachinesReadyNoReplicasReason,
			},
		},
		{
			name:       "one machine is ready",
			machineSet: machineSet,
			machines: []*clusterv1.Machine{
				fakeMachine("machine-1", withCondition(readyCondition)),
			},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineSetMachinesReadyCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineSetMachinesReadyReason,
			},
		},
		{
			name:       "all machines are ready",
			machineSet: machineSet,
			machines: []*clusterv1.Machine{
				fakeMachine("machine-1", withCondition(readyCondition)),
				fakeMachine("machine-2", withCondition(readyCondition)),
			},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineSetMachinesReadyCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineSetMachinesReadyReason,
			},
		},
		{
			name:       "one ready, one has nothing reported",
			machineSet: machineSet,
			machines: []*clusterv1.Machine{
				fakeMachine("machine-1", withCondition(readyCondition)),
				fakeMachine("machine-2"),
			},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineSetMachinesReadyCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineSetMachinesReadyUnknownReason,
				Message: "* Machine machine-2: Condition Ready not yet reported",
			},
		},
		{
			name:       "one ready, one reporting not ready, one reporting unknown, one reporting deleting",
			machineSet: machineSet,
			machines: []*clusterv1.Machine{
				fakeMachine("machine-1", withCondition(readyCondition)),
				fakeMachine("machine-2", withCondition(metav1.Condition{
					Type:    clusterv1.MachineReadyCondition,
					Status:  metav1.ConditionFalse,
					Reason:  "SomeReason",
					Message: "HealthCheckSucceeded: Some message",
				})),
				fakeMachine("machine-3", withCondition(metav1.Condition{
					Type:    clusterv1.MachineReadyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  "SomeUnknownReason",
					Message: "Some unknown message",
				})),
				fakeMachine("machine-4", withCondition(metav1.Condition{
					Type:    clusterv1.MachineReadyCondition,
					Status:  metav1.ConditionFalse,
					Reason:  clusterv1.MachineDeletingReason,
					Message: "Deleting: Machine deletion in progress, stage: DrainingNode",
				})),
			},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineSetMachinesReadyCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineSetMachinesNotReadyReason,
				Message: "* Machine machine-2: HealthCheckSucceeded: Some message\n" +
					"* Machine machine-4: Deleting: Machine deletion in progress, stage: DrainingNode\n" +
					"* Machine machine-3: Some unknown message",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			setMachinesReadyCondition(ctx, tt.machineSet, tt.machines, tt.getAndAdoptMachinesForMachineSetSucceeded)

			condition := conditions.Get(tt.machineSet, clusterv1.MachineSetMachinesReadyCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tt.expectCondition, conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func Test_setMachinesUpToDateCondition(t *testing.T) {
	machineSet := &clusterv1.MachineSet{}

	tests := []struct {
		name                                      string
		machineSet                                *clusterv1.MachineSet
		machines                                  []*clusterv1.Machine
		getAndAdoptMachinesForMachineSetSucceeded bool
		expectCondition                           metav1.Condition
	}{
		{
			name:       "getAndAdoptMachines failed",
			machineSet: machineSet,
			machines:   nil,
			getAndAdoptMachinesForMachineSetSucceeded: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineSetMachinesUpToDateCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineSetMachinesUpToDateInternalErrorReason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:       "no machines",
			machineSet: machineSet,
			machines:   []*clusterv1.Machine{},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineSetMachinesUpToDateCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineSetMachinesUpToDateNoReplicasReason,
				Message: "",
			},
		},
		{
			name:       "One machine up-to-date",
			machineSet: machineSet,
			machines: []*clusterv1.Machine{
				fakeMachine("up-to-date-1", withCondition(metav1.Condition{
					Type:   clusterv1.MachineUpToDateCondition,
					Status: metav1.ConditionTrue,
					Reason: "some-reason-1",
				})),
			},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineSetMachinesUpToDateCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineSetMachinesUpToDateReason,
				Message: "",
			},
		},
		{
			name:       "One machine unknown",
			machineSet: machineSet,
			machines: []*clusterv1.Machine{
				fakeMachine("unknown-1", withCondition(metav1.Condition{
					Type:    clusterv1.MachineUpToDateCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  "some-unknown-reason-1",
					Message: "some unknown message",
				})),
			},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineSetMachinesUpToDateCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineSetMachinesUpToDateUnknownReason,
				Message: "* Machine unknown-1: some unknown message",
			},
		},
		{
			name:       "One machine not up-to-date",
			machineSet: machineSet,
			machines: []*clusterv1.Machine{
				fakeMachine("not-up-to-date-machine-1", withCondition(metav1.Condition{
					Type:    clusterv1.MachineUpToDateCondition,
					Status:  metav1.ConditionFalse,
					Reason:  "some-not-up-to-date-reason",
					Message: "some not up-to-date message",
				})),
			},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineSetMachinesUpToDateCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineSetMachinesNotUpToDateReason,
				Message: "* Machine not-up-to-date-machine-1: some not up-to-date message",
			},
		},
		{
			name:       "One machine without up-to-date condition, one new Machines without up-to-date condition",
			machineSet: machineSet,
			machines: []*clusterv1.Machine{
				fakeMachine("no-condition-machine-1"),
				fakeMachine("no-condition-machine-2-new", withCreationTimestamp(time.Now().Add(-5*time.Second))), // ignored because it's new
			},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineSetMachinesUpToDateCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineSetMachinesUpToDateUnknownReason,
				Message: "* Machine no-condition-machine-1: Condition UpToDate not yet reported",
			},
		},
		{
			name:       "Two machines not up-to-date, two up-to-date, two not reported",
			machineSet: machineSet,
			machines: []*clusterv1.Machine{
				fakeMachine("up-to-date-1", withCondition(metav1.Condition{
					Type:   clusterv1.MachineUpToDateCondition,
					Status: metav1.ConditionTrue,
					Reason: "TestUpToDate",
				})),
				fakeMachine("up-to-date-2", withCondition(metav1.Condition{
					Type:   clusterv1.MachineUpToDateCondition,
					Status: metav1.ConditionTrue,
					Reason: "TestUpToDate",
				})),
				fakeMachine("not-up-to-date-machine-1", withCondition(metav1.Condition{
					Type:    clusterv1.MachineUpToDateCondition,
					Status:  metav1.ConditionFalse,
					Reason:  "TestNotUpToDate",
					Message: "This is not up-to-date message",
				})),
				fakeMachine("not-up-to-date-machine-2", withCondition(metav1.Condition{
					Type:    clusterv1.MachineUpToDateCondition,
					Status:  metav1.ConditionFalse,
					Reason:  "TestNotUpToDate",
					Message: "This is not up-to-date message",
				})),
				fakeMachine("no-condition-machine-1"),
				fakeMachine("no-condition-machine-2"),
			},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineSetMachinesUpToDateCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineSetMachinesNotUpToDateReason,
				Message: "* Machines not-up-to-date-machine-1, not-up-to-date-machine-2: This is not up-to-date message\n" +
					"* Machines no-condition-machine-1, no-condition-machine-2: Condition UpToDate not yet reported",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			setMachinesUpToDateCondition(ctx, tt.machineSet, tt.machines, tt.getAndAdoptMachinesForMachineSetSucceeded)

			condition := conditions.Get(tt.machineSet, clusterv1.MachineSetMachinesUpToDateCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tt.expectCondition, conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func Test_setRemediatingCondition(t *testing.T) {
	healthCheckSucceeded := metav1.Condition{Type: clusterv1.MachineHealthCheckSucceededCondition, Status: metav1.ConditionTrue}
	healthCheckNotSucceeded := metav1.Condition{Type: clusterv1.MachineHealthCheckSucceededCondition, Status: metav1.ConditionFalse}
	ownerRemediated := metav1.Condition{Type: clusterv1.MachineOwnerRemediatedCondition, Status: metav1.ConditionFalse, Reason: clusterv1.MachineSetMachineRemediationMachineDeletingReason, Message: "Machine is deleting"}
	ownerRemediatedWaitingForRemediation := metav1.Condition{Type: clusterv1.MachineOwnerRemediatedCondition, Status: metav1.ConditionFalse, Reason: clusterv1.MachineOwnerRemediatedWaitingForRemediationReason, Message: "KubeadmControlPlane ns1/cp1 is upgrading (\"ControlPlaneIsStable\" preflight check failed)"}

	tests := []struct {
		name                                      string
		machineSet                                *clusterv1.MachineSet
		machines                                  []*clusterv1.Machine
		getAndAdoptMachinesForMachineSetSucceeded bool
		expectCondition                           metav1.Condition
	}{
		{
			name:       "get machines failed",
			machineSet: &clusterv1.MachineSet{},
			machines:   nil,
			getAndAdoptMachinesForMachineSetSucceeded: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineSetRemediatingCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineSetRemediatingInternalErrorReason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:       "Without unhealthy machines",
			machineSet: &clusterv1.MachineSet{},
			machines: []*clusterv1.Machine{
				fakeMachine("m1"),
				fakeMachine("m2"),
			},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineSetRemediatingCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineSetNotRemediatingReason,
			},
		},
		{
			name:       "With machines to be remediated by MS",
			machineSet: &clusterv1.MachineSet{},
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withCondition(healthCheckSucceeded)),    // Healthy machine
				fakeMachine("m2", withCondition(healthCheckNotSucceeded)), // Unhealthy machine, not yet marked for remediation
				fakeMachine("m3", withCondition(healthCheckNotSucceeded), withCondition(ownerRemediated)),
			},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineSetRemediatingCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineSetRemediatingReason,
				Message: "* Machine m3: Machine is deleting",
			},
		},
		{
			name:       "With machines to be remediated by MS and preflight check error",
			machineSet: &clusterv1.MachineSet{},
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withCondition(healthCheckSucceeded)),    // Healthy machine
				fakeMachine("m2", withCondition(healthCheckNotSucceeded)), // Unhealthy machine, not yet marked for remediation
				fakeMachine("m3", withCondition(healthCheckNotSucceeded), withCondition(ownerRemediated)),
				fakeMachine("m4", withCondition(healthCheckNotSucceeded), withCondition(ownerRemediatedWaitingForRemediation)),
			},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			// This preflight check error can happen when a Machine becomes unhealthy while the control plane is upgrading.
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineSetRemediatingCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineSetRemediatingReason,
				Message: "* Machine m3: Machine is deleting\n" +
					"* Machine m4: KubeadmControlPlane ns1/cp1 is upgrading (\"ControlPlaneIsStable\" preflight check failed)",
			},
		},
		{
			name:       "With one unhealthy machine not to be remediated by MS",
			machineSet: &clusterv1.MachineSet{},
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withCondition(healthCheckSucceeded)),    // Healthy machine
				fakeMachine("m2", withCondition(healthCheckNotSucceeded)), // Unhealthy machine, not yet marked for remediation
				fakeMachine("m3", withCondition(healthCheckSucceeded)),    // Healthy machine
			},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineSetRemediatingCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineSetNotRemediatingReason,
				Message: "Machine m2 is not healthy (not to be remediated by MachineSet)",
			},
		},
		{
			name:       "With two unhealthy machine not to be remediated by MS",
			machineSet: &clusterv1.MachineSet{},
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withCondition(healthCheckNotSucceeded)), // Unhealthy machine, not yet marked for remediation
				fakeMachine("m2", withCondition(healthCheckNotSucceeded)), // Unhealthy machine, not yet marked for remediation
				fakeMachine("m3", withCondition(healthCheckSucceeded)),    // Healthy machine
			},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineSetRemediatingCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineSetNotRemediatingReason,
				Message: "Machines m1, m2 are not healthy (not to be remediated by MachineSet)",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			var machinesToBeRemediated, unHealthyMachines collections.Machines
			if tt.getAndAdoptMachinesForMachineSetSucceeded {
				machines := collections.FromMachines(tt.machines...)
				machinesToBeRemediated = machines.Filter(collections.IsUnhealthyAndOwnerRemediated)
				unHealthyMachines = machines.Filter(collections.IsUnhealthy)
			}
			setRemediatingCondition(ctx, tt.machineSet, machinesToBeRemediated, unHealthyMachines, tt.getAndAdoptMachinesForMachineSetSucceeded)

			condition := conditions.Get(tt.machineSet, clusterv1.MachineSetRemediatingCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tt.expectCondition, conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func Test_setDeletingCondition(t *testing.T) {
	tests := []struct {
		name                                      string
		machineSet                                *clusterv1.MachineSet
		getAndAdoptMachinesForMachineSetSucceeded bool
		machines                                  []*clusterv1.Machine
		expectCondition                           metav1.Condition
	}{
		{
			name:       "get machines failed",
			machineSet: &clusterv1.MachineSet{},
			machines:   nil,
			getAndAdoptMachinesForMachineSetSucceeded: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineSetDeletingCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineSetDeletingInternalErrorReason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:       "not deleting",
			machineSet: &clusterv1.MachineSet{},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineSetDeletingCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineSetNotDeletingReason,
			},
		},
		{
			name:       "Deleting with still some machine",
			machineSet: &clusterv1.MachineSet{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &metav1.Time{Time: time.Now()}}},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			machines: []*clusterv1.Machine{
				fakeMachine("m1"),
			},
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineSetDeletingCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineSetDeletingReason,
				Message: "Deleting 1 Machine",
			},
		},
		{
			name:       "Deleting with still some stale machine",
			machineSet: &clusterv1.MachineSet{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &metav1.Time{Time: time.Now()}}},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withStaleDeletionTimestamp()),
				fakeMachine("m2", withStaleDeletionTimestamp()),
				fakeMachine("m3"),
			},
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineSetDeletingCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineSetDeletingReason,
				Message: "Deleting 3 Machines\n" +
					"* Machines m1, m2 are in deletion since more than 15m",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			setDeletingCondition(ctx, tt.machineSet, tt.machines, tt.getAndAdoptMachinesForMachineSetSucceeded)

			condition := conditions.Get(tt.machineSet, clusterv1.MachineSetDeletingCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tt.expectCondition, conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func Test_aggregateStaleMachines(t *testing.T) {
	tests := []struct {
		name          string
		machines      []*clusterv1.Machine
		expectMessage string
	}{
		{
			name:          "Empty when there are no machines",
			machines:      nil,
			expectMessage: "",
		},
		{
			name: "Empty when there are no deleting machines",
			machines: []*clusterv1.Machine{
				fakeMachine("m1"),
				fakeMachine("m2"),
				fakeMachine("m3"),
			},
			expectMessage: "",
		},
		{
			name: "Empty when there are deleting machines but not yet stale",
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withDeletionTimestamp()),
				fakeMachine("m2", withDeletionTimestamp()),
				fakeMachine("m3"),
			},
			expectMessage: "",
		},
		{
			name: "Report stale machine not draining",
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withDeletionTimestamp()),
				fakeMachine("m2", withStaleDeletionTimestamp()),
				fakeMachine("m3"),
			},
			expectMessage: "Machine m2 is in deletion since more than 15m",
		},
		{
			name: "Report stale machines not draining",
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withStaleDeletionTimestamp()),
				fakeMachine("m2", withStaleDeletionTimestamp()),
				fakeMachine("m3"),
			},
			expectMessage: "Machines m1, m2 are in deletion since more than 15m",
		},
		{
			name: "Does not report details about stale machines draining since less than 5 minutes",
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withStaleDeletionTimestamp(), withCondition(metav1.Condition{
					Type:   clusterv1.MachineDeletingCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineDeletingDrainingNodeReason,
					Message: `Drain not completed yet (started at 2024-10-09T16:13:59Z):
* Pods pod-2-deletionTimestamp-set-1, pod-3-to-trigger-eviction-successfully-1: deletionTimestamp set, but still not removed from the Node
* Pod pod-5-to-trigger-eviction-pdb-violated-1: cannot evict pod as it would violate the pod's disruption budget. The disruption budget pod-5-pdb needs 20 healthy pods and has 20 currently
* Pod pod-6-to-trigger-eviction-some-other-error: failed to evict Pod, some other error 1
After above Pods have been removed from the Node, the following Pods will be evicted: pod-7-eviction-later, pod-8-eviction-later`,
				})),
				fakeMachine("m2", withStaleDeletionTimestamp(), withCondition(metav1.Condition{
					Type:   clusterv1.MachineDeletingCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineDeletingDrainingNodeReason,
					Message: `Drain not completed yet (started at 2024-10-09T16:13:59Z):
* Pods pod-2-deletionTimestamp-set-1, pod-3-to-trigger-eviction-successfully-1: deletionTimestamp set, but still not removed from the Node
After above Pods have been removed from the Node, the following Pods will be evicted: pod-7-eviction-later, pod-8-eviction-later`,
				})),
				fakeMachine("m3"),
			},
			expectMessage: "Machines m1, m2 are in deletion since more than 15m",
		},
		{
			name: "Report details about stale machines draining since more than 5 minutes",
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withStaleDeletionTimestamp(), withCondition(metav1.Condition{
					Type:   clusterv1.MachineDeletingCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineDeletingDrainingNodeReason,
					Message: `Drain not completed yet (started at 2024-10-09T16:13:59Z):
* Pods pod-2-deletionTimestamp-set-1, pod-3-to-trigger-eviction-successfully-1: deletionTimestamp set, but still not removed from the Node
* Pod pod-5-to-trigger-eviction-pdb-violated-1: cannot evict pod as it would violate the pod's disruption budget. The disruption budget pod-5-pdb needs 20 healthy pods and has 20 currently
* Pod pod-6-to-trigger-eviction-some-other-error: failed to evict Pod, some other error 1
After above Pods have been removed from the Node, the following Pods will be evicted: pod-7-eviction-later, pod-8-eviction-later`,
				}), withStaleDrain()),
				fakeMachine("m2", withStaleDeletionTimestamp(), withCondition(metav1.Condition{
					Type:   clusterv1.MachineDeletingCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineDeletingDrainingNodeReason,
					Message: `Drain not completed yet (started at 2024-10-09T16:13:59Z):
* Pods pod-2-deletionTimestamp-set-1, pod-3-to-trigger-eviction-successfully-1: deletionTimestamp set, but still not removed from the Node
After above Pods have been removed from the Node, the following Pods will be evicted: pod-7-eviction-later, pod-8-eviction-later`,
				}), withStaleDrain()),
				fakeMachine("m3"),
			},
			expectMessage: "Machines m1, m2 are in deletion since more than 15m, delay likely due to PodDisruptionBudgets, Pods not terminating, Pod eviction errors",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got := aggregateStaleMachines(tt.machines)
			g.Expect(got).To(Equal(tt.expectMessage))
		})
	}
}

type fakeMachinesOption func(m *clusterv1.Machine)

func fakeMachine(name string, options ...fakeMachinesOption) *clusterv1.Machine {
	p := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	for _, opt := range options {
		opt(p)
	}
	return p
}

func withOwnerMachineSet(msName string) fakeMachinesOption {
	return func(m *clusterv1.Machine) {
		m.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: clusterv1.GroupVersion.String(),
				Kind:       "MachineSet",
				Name:       msName,
				Controller: ptr.To(true),
			},
		}
	}
}

func withCreationTimestamp(t time.Time) fakeMachinesOption {
	return func(m *clusterv1.Machine) {
		m.CreationTimestamp = metav1.Time{Time: t}
	}
}

func withMachineFinalizer() fakeMachinesOption {
	return func(m *clusterv1.Machine) {
		m.Finalizers = []string{clusterv1.MachineFinalizer}
	}
}

func withDeletionTimestamp() fakeMachinesOption {
	return func(m *clusterv1.Machine) {
		m.DeletionTimestamp = ptr.To(metav1.Time{Time: time.Now()})
	}
}

func withStaleDeletionTimestamp() fakeMachinesOption {
	return func(m *clusterv1.Machine) {
		m.DeletionTimestamp = ptr.To(metav1.Time{Time: time.Now().Add(-1 * time.Hour)})
	}
}

func withMachineLabels(labels map[string]string) fakeMachinesOption {
	return func(m *clusterv1.Machine) {
		m.Labels = labels
	}
}

func withMachineAnnotations(annotations map[string]string) fakeMachinesOption {
	return func(m *clusterv1.Machine) {
		m.Annotations = annotations
	}
}

func withStaleDrain() fakeMachinesOption {
	return func(m *clusterv1.Machine) {
		if m.Status.Deletion == nil {
			m.Status.Deletion = &clusterv1.MachineDeletionStatus{}
		}
		m.Status.Deletion.NodeDrainStartTime = metav1.Time{Time: time.Now().Add(-6 * time.Minute)}
	}
}

func withCondition(c metav1.Condition) fakeMachinesOption {
	return func(m *clusterv1.Machine) {
		conditions.Set(m, c)
	}
}

func withHealthyNode() fakeMachinesOption {
	// Note: This is what is required by delete priority functions to consider the machine healthy.
	return func(m *clusterv1.Machine) {
		m.Status = clusterv1.MachineStatus{
			NodeRef: clusterv1.MachineNodeReference{Name: "some-node"},
			Conditions: []metav1.Condition{
				{
					Type:   clusterv1.MachineNodeHealthyCondition,
					Status: metav1.ConditionTrue,
				},
			},
		}
	}
}
