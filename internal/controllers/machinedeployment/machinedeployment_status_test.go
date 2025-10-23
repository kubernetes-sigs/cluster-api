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

package machinedeployment

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
)

func Test_setReplicas(t *testing.T) {
	tests := []struct {
		name                    string
		machineSets             []*clusterv1.MachineSet
		expectReplicas          int32
		expectReadyReplicas     *int32
		expectAvailableReplicas *int32
		expectUpToDateReplicas  *int32
	}{
		{
			name:                    "No MachineSets",
			machineSets:             nil,
			expectReplicas:          0,
			expectReadyReplicas:     nil,
			expectAvailableReplicas: nil,
			expectUpToDateReplicas:  nil,
		},
		{
			name: "MachineSets without replicas set",
			machineSets: []*clusterv1.MachineSet{
				fakeMachineSet("ms1"),
			},
			expectReplicas:          0,
			expectReadyReplicas:     nil,
			expectAvailableReplicas: nil,
			expectUpToDateReplicas:  nil,
		},
		{
			name: "MachineSets with replicas set",
			machineSets: []*clusterv1.MachineSet{
				fakeMachineSet("ms1", withStatusReplicas(6), withStatusV1beta2ReadyReplicas(5), withStatusV1beta2AvailableReplicas(3), withStatusV1beta2UpToDateReplicas(3)),
				fakeMachineSet("ms2", withStatusReplicas(3), withStatusV1beta2ReadyReplicas(2), withStatusV1beta2AvailableReplicas(2), withStatusV1beta2UpToDateReplicas(1)),
				fakeMachineSet("ms3"),
			},
			expectReplicas:          9,
			expectReadyReplicas:     ptr.To(int32(7)),
			expectAvailableReplicas: ptr.To(int32(5)),
			expectUpToDateReplicas:  ptr.To(int32(4)),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			md := &clusterv1.MachineDeployment{}
			setReplicas(md, tt.machineSets)

			g.Expect(md.Status.ReadyReplicas).To(Equal(tt.expectReadyReplicas))
			g.Expect(md.Status.AvailableReplicas).To(Equal(tt.expectAvailableReplicas))
			g.Expect(md.Status.UpToDateReplicas).To(Equal(tt.expectUpToDateReplicas))
		})
	}
}

func Test_setAvailableCondition(t *testing.T) {
	tests := []struct {
		name                                         string
		machineDeployment                            *clusterv1.MachineDeployment
		getAndAdoptMachineSetsForDeploymentSucceeded bool
		expectCondition                              metav1.Condition
	}{
		{
			name:              "getAndAdoptMachineSetsForDeploymentSucceeded failed",
			machineDeployment: &clusterv1.MachineDeployment{},
			getAndAdoptMachineSetsForDeploymentSucceeded: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentAvailableCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineDeploymentAvailableInternalErrorReason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:              "spec.replicas not set",
			machineDeployment: &clusterv1.MachineDeployment{},
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentAvailableCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineDeploymentAvailableWaitingForReplicasSetReason,
				Message: "Waiting for spec.replicas set",
			},
		},
		{
			name: "status.v1beta2 not set",
			machineDeployment: &clusterv1.MachineDeployment{
				Spec: clusterv1.MachineDeploymentSpec{Replicas: ptr.To(int32(5))},
			},
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentAvailableCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineDeploymentAvailableWaitingForAvailableReplicasSetReason,
				Message: "Waiting for status.availableReplicas set",
			},
		},
		{
			name: "all the expected replicas are available",
			machineDeployment: &clusterv1.MachineDeployment{
				Spec: clusterv1.MachineDeploymentSpec{
					Replicas: ptr.To(int32(5)),
					Rollout: clusterv1.MachineDeploymentRolloutSpec{
						Strategy: clusterv1.MachineDeploymentRolloutStrategy{
							Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
							RollingUpdate: clusterv1.MachineDeploymentRolloutStrategyRollingUpdate{
								MaxSurge:       ptr.To(intstr.FromInt32(1)),
								MaxUnavailable: ptr.To(intstr.FromInt32(0)),
							},
						},
					},
				},
				Status: clusterv1.MachineDeploymentStatus{AvailableReplicas: ptr.To(int32(5))},
			},
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeploymentAvailableCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineDeploymentAvailableReason,
			},
		},
		{
			name: "some replicas are not available, but within MaxUnavailable range",
			machineDeployment: &clusterv1.MachineDeployment{
				Spec: clusterv1.MachineDeploymentSpec{
					Replicas: ptr.To(int32(5)),
					Rollout: clusterv1.MachineDeploymentRolloutSpec{
						Strategy: clusterv1.MachineDeploymentRolloutStrategy{
							Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
							RollingUpdate: clusterv1.MachineDeploymentRolloutStrategyRollingUpdate{
								MaxSurge:       ptr.To(intstr.FromInt32(1)),
								MaxUnavailable: ptr.To(intstr.FromInt32(1)),
							},
						},
					},
				},
				Status: clusterv1.MachineDeploymentStatus{AvailableReplicas: ptr.To(int32(4))},
			},
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeploymentAvailableCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineDeploymentAvailableReason,
			},
		},
		{
			name: "some replicas are not available, more than MaxUnavailable",
			machineDeployment: &clusterv1.MachineDeployment{
				Spec: clusterv1.MachineDeploymentSpec{
					Replicas: ptr.To(int32(5)),
					Rollout: clusterv1.MachineDeploymentRolloutSpec{
						Strategy: clusterv1.MachineDeploymentRolloutStrategy{
							Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
							RollingUpdate: clusterv1.MachineDeploymentRolloutStrategyRollingUpdate{
								MaxSurge:       ptr.To(intstr.FromInt32(1)),
								MaxUnavailable: ptr.To(intstr.FromInt32(1)),
							},
						},
					},
				},
				Status: clusterv1.MachineDeploymentStatus{AvailableReplicas: ptr.To(int32(3))},
			},
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentAvailableCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineDeploymentNotAvailableReason,
				Message: "3 available replicas, at least 4 required (spec.strategy.rollout.maxUnavailable is 1, spec.replicas is 5)",
			},
		},
		{
			name: "When deleting, don't show required replicas",
			machineDeployment: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					DeletionTimestamp: ptr.To(metav1.Now()),
				},
				Spec: clusterv1.MachineDeploymentSpec{
					Replicas: ptr.To(int32(5)),
					Rollout: clusterv1.MachineDeploymentRolloutSpec{
						Strategy: clusterv1.MachineDeploymentRolloutStrategy{
							Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
							RollingUpdate: clusterv1.MachineDeploymentRolloutStrategyRollingUpdate{
								MaxSurge:       ptr.To(intstr.FromInt32(1)),
								MaxUnavailable: ptr.To(intstr.FromInt32(1)),
							},
						},
					},
				},
				Status: clusterv1.MachineDeploymentStatus{AvailableReplicas: ptr.To(int32(0))},
			},
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentAvailableCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineDeploymentNotAvailableReason,
				Message: "Deletion in progress",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			setAvailableCondition(ctx, tt.machineDeployment, tt.getAndAdoptMachineSetsForDeploymentSucceeded)

			condition := conditions.Get(tt.machineDeployment, clusterv1.MachineDeploymentAvailableCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tt.expectCondition, conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func Test_setRollingOutCondition(t *testing.T) {
	upToDateCondition := metav1.Condition{
		Type:   clusterv1.MachineUpToDateCondition,
		Status: metav1.ConditionTrue,
		Reason: clusterv1.MachineUpToDateReason,
	}

	tests := []struct {
		name              string
		machineDeployment *clusterv1.MachineDeployment
		machines          []*clusterv1.Machine
		expectCondition   metav1.Condition
	}{
		{
			name:              "no machines",
			machineDeployment: &clusterv1.MachineDeployment{},
			machines:          []*clusterv1.Machine{},
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeploymentRollingOutCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineDeploymentNotRollingOutReason,
			},
		},
		{
			name:              "all machines are up to date",
			machineDeployment: &clusterv1.MachineDeployment{},
			machines: []*clusterv1.Machine{
				fakeMachine("machine-1", withCondition(upToDateCondition)),
				fakeMachine("machine-2", withCondition(upToDateCondition)),
			},
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeploymentRollingOutCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineDeploymentNotRollingOutReason,
			},
		},
		{
			name:              "one up-to-date, two not up-to-date, one reporting up-to-date unknown",
			machineDeployment: &clusterv1.MachineDeployment{},
			machines: []*clusterv1.Machine{
				fakeMachine("machine-1", withCondition(upToDateCondition)),
				fakeMachine("machine-2", withCondition(metav1.Condition{
					Type:   clusterv1.MachineUpToDateCondition,
					Status: metav1.ConditionUnknown,
					Reason: clusterv1.InternalErrorReason,
				})),
				fakeMachine("machine-4", withCondition(metav1.Condition{
					Type:   clusterv1.MachineUpToDateCondition,
					Status: metav1.ConditionFalse,
					Reason: clusterv1.MachineNotUpToDateReason,
					Message: "* Failure domain failure-domain1, failure-domain2 required\n" +
						"* InfrastructureMachine is not up-to-date",
				})),
				fakeMachine("machine-3", withCondition(metav1.Condition{
					Type:    clusterv1.MachineUpToDateCondition,
					Status:  metav1.ConditionFalse,
					Reason:  clusterv1.MachineNotUpToDateReason,
					Message: "* Version v1.25.0, v1.26.0 required",
				})),
			},
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeploymentRollingOutCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineDeploymentRollingOutReason,
				Message: "Rolling out 2 not up-to-date replicas\n" +
					"* Version v1.25.0, v1.26.0 required\n" +
					"* Failure domain failure-domain1, failure-domain2 required\n" +
					"* InfrastructureMachine is not up-to-date",
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
			setRollingOutCondition(ctx, tt.machineDeployment, machines)

			condition := conditions.Get(tt.machineDeployment, clusterv1.MachineDeploymentRollingOutCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tt.expectCondition, conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func Test_setScalingUpCondition(t *testing.T) {
	machineDeploymentWith0Replicas := &clusterv1.MachineDeployment{
		Spec: clusterv1.MachineDeploymentSpec{
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

	machineDeploymentWith3Replicas := machineDeploymentWith0Replicas.DeepCopy()
	machineDeploymentWith3Replicas.Spec.Replicas = ptr.To[int32](3)

	deletingMachineDeploymentWith3Replicas := machineDeploymentWith0Replicas.DeepCopy()
	deletingMachineDeploymentWith3Replicas.DeletionTimestamp = ptr.To(metav1.Now())
	deletingMachineDeploymentWith3Replicas.Spec.Replicas = ptr.To[int32](3)

	tests := []struct {
		name                                         string
		machineDeployment                            *clusterv1.MachineDeployment
		machineSets                                  []*clusterv1.MachineSet
		bootstrapTemplateNotFound                    bool
		infrastructureTemplateNotFound               bool
		getAndAdoptMachineSetsForDeploymentSucceeded bool
		expectCondition                              metav1.Condition
		expectedPhase                                clusterv1.MachineDeploymentPhase
	}{
		{
			name:                           "getAndAdoptMachineSetsForDeploymentSucceeded failed",
			machineDeployment:              machineDeploymentWith0Replicas.DeepCopy(),
			bootstrapTemplateNotFound:      false,
			infrastructureTemplateNotFound: false,
			getAndAdoptMachineSetsForDeploymentSucceeded: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentScalingUpCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineDeploymentScalingUpInternalErrorReason,
				Message: "Please check controller logs for errors",
			},
			expectedPhase: clusterv1.MachineDeploymentPhaseUnknown,
		},
		{
			name: "replicas not set",
			machineDeployment: func() *clusterv1.MachineDeployment {
				md := machineDeploymentWith0Replicas.DeepCopy()
				md.Spec.Replicas = nil
				return md
			}(),
			bootstrapTemplateNotFound:                    false,
			infrastructureTemplateNotFound:               false,
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentScalingUpCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineDeploymentScalingUpWaitingForReplicasSetReason,
				Message: "Waiting for spec.replicas set",
			},
			expectedPhase: clusterv1.MachineDeploymentPhaseUnknown,
		},
		{
			name:                           "not scaling up and no machines",
			machineDeployment:              machineDeploymentWith0Replicas.DeepCopy(),
			bootstrapTemplateNotFound:      false,
			infrastructureTemplateNotFound: false,
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeploymentScalingUpCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineDeploymentNotScalingUpReason,
			},
			expectedPhase: clusterv1.MachineDeploymentPhaseRunning,
		},
		{
			name:              "not scaling up with machines",
			machineDeployment: machineDeploymentWith3Replicas.DeepCopy(),
			machineSets: []*clusterv1.MachineSet{
				fakeMachineSet("ms1", withStatusReplicas(1)),
				fakeMachineSet("ms2", withStatusReplicas(2)),
			},
			bootstrapTemplateNotFound:                    false,
			infrastructureTemplateNotFound:               false,
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeploymentScalingUpCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineDeploymentNotScalingUpReason,
			},
			expectedPhase: clusterv1.MachineDeploymentPhaseRunning,
		},
		{
			name:                           "not scaling up and no machines and bootstrapConfig object not found",
			machineDeployment:              machineDeploymentWith0Replicas.DeepCopy(),
			bootstrapTemplateNotFound:      true,
			infrastructureTemplateNotFound: false,
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentScalingUpCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineDeploymentNotScalingUpReason,
				Message: "Scaling up would be blocked because KubeadmBootstrapTemplate does not exist",
			},
			expectedPhase: clusterv1.MachineDeploymentPhaseRunning,
		},
		{
			name:                           "not scaling up and no machines and infrastructure object not found",
			machineDeployment:              machineDeploymentWith0Replicas.DeepCopy(),
			bootstrapTemplateNotFound:      false,
			infrastructureTemplateNotFound: true,
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentScalingUpCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineDeploymentNotScalingUpReason,
				Message: "Scaling up would be blocked because DockerMachineTemplate does not exist",
			},
			expectedPhase: clusterv1.MachineDeploymentPhaseRunning,
		},
		{
			name:                           "not scaling up and no machines and bootstrapConfig and infrastructure object not found",
			machineDeployment:              machineDeploymentWith0Replicas.DeepCopy(),
			bootstrapTemplateNotFound:      true,
			infrastructureTemplateNotFound: true,
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentScalingUpCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineDeploymentNotScalingUpReason,
				Message: "Scaling up would be blocked because KubeadmBootstrapTemplate and DockerMachineTemplate do not exist",
			},
			expectedPhase: clusterv1.MachineDeploymentPhaseRunning,
		},
		{
			name:                           "scaling up",
			machineDeployment:              machineDeploymentWith3Replicas.DeepCopy(),
			bootstrapTemplateNotFound:      false,
			infrastructureTemplateNotFound: false,
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentScalingUpCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineDeploymentScalingUpReason,
				Message: "Scaling up from 0 to 3 replicas",
			},
			expectedPhase: clusterv1.MachineDeploymentPhaseScalingUp,
		},
		{
			name:              "scaling up with machines",
			machineDeployment: machineDeploymentWith3Replicas.DeepCopy(),
			machineSets: []*clusterv1.MachineSet{
				fakeMachineSet("ms1", withStatusReplicas(1)),
				fakeMachineSet("ms2", withStatusReplicas(1)),
			},
			bootstrapTemplateNotFound:                    false,
			infrastructureTemplateNotFound:               false,
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentScalingUpCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineDeploymentScalingUpReason,
				Message: "Scaling up from 2 to 3 replicas",
			},
			expectedPhase: clusterv1.MachineDeploymentPhaseScalingUp,
		},
		{
			name:                           "scaling up and blocked by bootstrap object",
			machineDeployment:              machineDeploymentWith3Replicas.DeepCopy(),
			bootstrapTemplateNotFound:      true,
			infrastructureTemplateNotFound: false,
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentScalingUpCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineDeploymentScalingUpReason,
				Message: "Scaling up from 0 to 3 replicas is blocked because KubeadmBootstrapTemplate does not exist",
			},
			expectedPhase: clusterv1.MachineDeploymentPhaseScalingUp,
		},
		{
			name:                           "scaling up and blocked by infrastructure object",
			machineDeployment:              machineDeploymentWith3Replicas.DeepCopy(),
			bootstrapTemplateNotFound:      false,
			infrastructureTemplateNotFound: true,
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentScalingUpCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineDeploymentScalingUpReason,
				Message: "Scaling up from 0 to 3 replicas is blocked because DockerMachineTemplate does not exist",
			},
			expectedPhase: clusterv1.MachineDeploymentPhaseScalingUp,
		},
		{
			name:                           "deleting, don't show block message when templates are not found",
			machineDeployment:              deletingMachineDeploymentWith3Replicas.DeepCopy(),
			machineSets:                    []*clusterv1.MachineSet{{}, {}, {}},
			bootstrapTemplateNotFound:      true,
			infrastructureTemplateNotFound: true,
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeploymentScalingUpCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineDeploymentNotScalingUpReason,
			},
			expectedPhase: clusterv1.MachineDeploymentPhaseScalingDown,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			setScalingUpCondition(ctx, tt.machineDeployment, tt.machineSets, tt.bootstrapTemplateNotFound, tt.infrastructureTemplateNotFound, tt.getAndAdoptMachineSetsForDeploymentSucceeded)
			setPhase(ctx, tt.machineDeployment, tt.machineSets, tt.getAndAdoptMachineSetsForDeploymentSucceeded)

			condition := conditions.Get(tt.machineDeployment, clusterv1.MachineDeploymentScalingUpCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tt.expectCondition, conditions.IgnoreLastTransitionTime(true)))

			g.Expect(tt.machineDeployment.Status.Phase).To(Equal(string(tt.expectedPhase)))
		})
	}
}

func Test_setScalingDownCondition(t *testing.T) {
	defaultMachineDeployment := &clusterv1.MachineDeployment{
		Spec: clusterv1.MachineDeploymentSpec{
			Replicas: ptr.To[int32](0),
		},
	}

	machineDeploymentWith1Replica := defaultMachineDeployment.DeepCopy()
	machineDeploymentWith1Replica.Spec.Replicas = ptr.To[int32](1)

	deletingMachineDeployment := defaultMachineDeployment.DeepCopy()
	deletingMachineDeployment.Spec.Replicas = ptr.To[int32](1)
	deletingMachineDeployment.DeletionTimestamp = ptr.To(metav1.Now())

	tests := []struct {
		name                                         string
		machineDeployment                            *clusterv1.MachineDeployment
		machineSets                                  []*clusterv1.MachineSet
		machines                                     []*clusterv1.Machine
		getAndAdoptMachineSetsForDeploymentSucceeded bool
		expectCondition                              metav1.Condition
		expectedPhase                                clusterv1.MachineDeploymentPhase
	}{
		{
			name:              "getAndAdoptMachineSetsForDeploymentSucceeded failed",
			machineDeployment: defaultMachineDeployment,
			machineSets:       nil,
			getAndAdoptMachineSetsForDeploymentSucceeded: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentScalingDownCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineDeploymentScalingDownInternalErrorReason,
				Message: "Please check controller logs for errors",
			},
			expectedPhase: clusterv1.MachineDeploymentPhaseUnknown,
		},
		{
			name: "replicas not set",
			machineDeployment: func() *clusterv1.MachineDeployment {
				md := defaultMachineDeployment.DeepCopy()
				md.Spec.Replicas = nil
				return md
			}(),
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentScalingDownCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineDeploymentScalingDownWaitingForReplicasSetReason,
				Message: "Waiting for spec.replicas set",
			},
			expectedPhase: clusterv1.MachineDeploymentPhaseUnknown,
		},
		{
			name:              "not scaling down and no machines",
			machineDeployment: defaultMachineDeployment,
			machineSets:       []*clusterv1.MachineSet{},
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeploymentScalingDownCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineDeploymentNotScalingDownReason,
			},
			expectedPhase: clusterv1.MachineDeploymentPhaseRunning,
		},
		{
			name:              "not scaling down because scaling up",
			machineDeployment: machineDeploymentWith1Replica,
			machineSets:       []*clusterv1.MachineSet{},
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeploymentScalingDownCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineDeploymentNotScalingDownReason,
			},
			expectedPhase: clusterv1.MachineDeploymentPhaseScalingUp,
		},
		{
			name:              "scaling down to zero",
			machineDeployment: defaultMachineDeployment,
			machineSets: []*clusterv1.MachineSet{
				fakeMachineSet("ms1", withStatusReplicas(1)),
			},
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentScalingDownCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineDeploymentScalingDownReason,
				Message: "Scaling down from 1 to 0 replicas",
			},
			expectedPhase: clusterv1.MachineDeploymentPhaseScalingDown,
		},
		{
			name:              "scaling down with 1 stale machine",
			machineDeployment: machineDeploymentWith1Replica,
			machineSets: []*clusterv1.MachineSet{
				fakeMachineSet("ms1", withStatusReplicas(1)),
				fakeMachineSet("ms2", withStatusReplicas(1)),
			},
			machines: []*clusterv1.Machine{
				fakeMachine("m1"),
				fakeMachine("stale-machine-1", withStaleDeletion(), func(m *clusterv1.Machine) {
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
			},
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeploymentScalingDownCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineDeploymentScalingDownReason,
				Message: "Scaling down from 2 to 1 replicas\n" +
					"* Machine stale-machine-1 is in deletion since more than 15m, delay likely due to PodDisruptionBudgets, Pods not terminating, Pod eviction errors, Pods not completed yet",
			},
			expectedPhase: clusterv1.MachineDeploymentPhaseScalingDown,
		},
		{
			name:              "scaling down with 3 stale machines",
			machineDeployment: machineDeploymentWith1Replica,
			machineSets: []*clusterv1.MachineSet{
				fakeMachineSet("ms1", withStatusReplicas(3)),
				fakeMachineSet("ms2", withStatusReplicas(1)),
			},
			machines: []*clusterv1.Machine{
				fakeMachine("m1"),
				fakeMachine("stale-machine-1", withStaleDeletion()),
				fakeMachine("stale-machine-2", withStaleDeletion()),
				fakeMachine("stale-machine-3", withStaleDeletion()),
			},
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeploymentScalingDownCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineDeploymentScalingDownReason,
				Message: "Scaling down from 4 to 1 replicas\n" +
					"* Machines stale-machine-1, stale-machine-2, stale-machine-3 are in deletion since more than 15m",
			},
			expectedPhase: clusterv1.MachineDeploymentPhaseScalingDown,
		},
		{
			name:              "scaling down with 5 stale machines",
			machineDeployment: machineDeploymentWith1Replica,
			machineSets: []*clusterv1.MachineSet{
				fakeMachineSet("ms1", withStatusReplicas(5)),
				fakeMachineSet("ms2", withStatusReplicas(1)),
			},
			machines: []*clusterv1.Machine{
				fakeMachine("m1"),
				fakeMachine("stale-machine-1", withStaleDeletion()),
				fakeMachine("stale-machine-2", withStaleDeletion()),
				fakeMachine("stale-machine-3", withStaleDeletion()),
				fakeMachine("stale-machine-4", withStaleDeletion()),
				fakeMachine("stale-machine-5", withStaleDeletion()),
			},
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeploymentScalingDownCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineDeploymentScalingDownReason,
				Message: "Scaling down from 6 to 1 replicas\n" +
					"* Machines stale-machine-1, stale-machine-2, stale-machine-3, ... (2 more) are in deletion since more than 15m",
			},
			expectedPhase: clusterv1.MachineDeploymentPhaseScalingDown,
		},
		{
			name:              "deleting machine deployment without replicas",
			machineDeployment: deletingMachineDeployment,
			machineSets:       []*clusterv1.MachineSet{},
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeploymentScalingDownCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineDeploymentNotScalingDownReason,
			},
			expectedPhase: clusterv1.MachineDeploymentPhaseScalingDown,
		},
		{
			name:              "deleting machine deployment having 1 replica",
			machineDeployment: deletingMachineDeployment,
			machineSets: []*clusterv1.MachineSet{
				fakeMachineSet("ms1", withStatusReplicas(1)),
			},
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentScalingDownCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineDeploymentScalingDownReason,
				Message: "Scaling down from 1 to 0 replicas",
			},
			expectedPhase: clusterv1.MachineDeploymentPhaseScalingDown,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			setScalingDownCondition(ctx, tt.machineDeployment, tt.machineSets, collections.FromMachines(tt.machines...), tt.getAndAdoptMachineSetsForDeploymentSucceeded)
			setPhase(ctx, tt.machineDeployment, tt.machineSets, tt.getAndAdoptMachineSetsForDeploymentSucceeded)

			condition := conditions.Get(tt.machineDeployment, clusterv1.MachineDeploymentScalingDownCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tt.expectCondition, conditions.IgnoreLastTransitionTime(true)))

			g.Expect(tt.machineDeployment.Status.Phase).To(Equal(string(tt.expectedPhase)))
		})
	}
}

func Test_setMachinesReadyCondition(t *testing.T) {
	readyCondition := metav1.Condition{
		Type:   clusterv1.MachineReadyCondition,
		Status: metav1.ConditionTrue,
		Reason: clusterv1.MachineReadyReason,
	}

	tests := []struct {
		name              string
		machineDeployment *clusterv1.MachineDeployment
		machines          []*clusterv1.Machine
		expectCondition   metav1.Condition
	}{
		{
			name:              "no machines",
			machineDeployment: &clusterv1.MachineDeployment{},
			machines:          []*clusterv1.Machine{},
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeploymentMachinesReadyCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineDeploymentMachinesReadyNoReplicasReason,
			},
		},
		{
			name:              "all machines are ready",
			machineDeployment: &clusterv1.MachineDeployment{},
			machines: []*clusterv1.Machine{
				fakeMachine("machine-1", withCondition(readyCondition)),
				fakeMachine("machine-2", withCondition(readyCondition)),
			},
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeploymentMachinesReadyCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineDeploymentMachinesReadyReason,
			},
		},
		{
			name:              "one ready, one has nothing reported",
			machineDeployment: &clusterv1.MachineDeployment{},
			machines: []*clusterv1.Machine{
				fakeMachine("machine-1", withCondition(readyCondition)),
				fakeMachine("machine-2"),
			},
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentMachinesReadyCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineDeploymentMachinesReadyUnknownReason,
				Message: "* Machine machine-2: Condition Ready not yet reported",
			},
		},
		{
			name:              "one ready, one reporting not ready, one reporting unknown, one reporting deleting",
			machineDeployment: &clusterv1.MachineDeployment{},
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
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeploymentMachinesReadyCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineDeploymentMachinesNotReadyReason,
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
			setMachinesReadyCondition(ctx, tt.machineDeployment, machines)

			condition := conditions.Get(tt.machineDeployment, clusterv1.MachineDeploymentMachinesReadyCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tt.expectCondition, conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func Test_setMachinesUpToDateCondition(t *testing.T) {
	tests := []struct {
		name              string
		machineDeployment *clusterv1.MachineDeployment
		machines          []*clusterv1.Machine
		expectCondition   metav1.Condition
	}{
		{
			name:              "no machines",
			machineDeployment: &clusterv1.MachineDeployment{},
			machines:          []*clusterv1.Machine{},
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentMachinesUpToDateCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineDeploymentMachinesUpToDateNoReplicasReason,
				Message: "",
			},
		},
		{
			name:              "One machine up-to-date",
			machineDeployment: &clusterv1.MachineDeployment{},
			machines: []*clusterv1.Machine{
				fakeMachine("up-to-date-1", withCondition(metav1.Condition{
					Type:   clusterv1.MachineUpToDateCondition,
					Status: metav1.ConditionTrue,
					Reason: "some-reason-1",
				})),
			},
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentMachinesUpToDateCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineDeploymentMachinesUpToDateReason,
				Message: "",
			},
		},
		{
			name:              "One machine unknown",
			machineDeployment: &clusterv1.MachineDeployment{},
			machines: []*clusterv1.Machine{
				fakeMachine("unknown-1", withCondition(metav1.Condition{
					Type:    clusterv1.MachineUpToDateCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  "some-unknown-reason-1",
					Message: "some unknown message",
				})),
			},
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentMachinesUpToDateCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineDeploymentMachinesUpToDateUnknownReason,
				Message: "* Machine unknown-1: some unknown message",
			},
		},
		{
			name:              "One machine not up-to-date",
			machineDeployment: &clusterv1.MachineDeployment{},
			machines: []*clusterv1.Machine{
				fakeMachine("not-up-to-date-machine-1", withCondition(metav1.Condition{
					Type:    clusterv1.MachineUpToDateCondition,
					Status:  metav1.ConditionFalse,
					Reason:  "some-not-up-to-date-reason",
					Message: "some not up-to-date message",
				})),
			},
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentMachinesUpToDateCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineDeploymentMachinesNotUpToDateReason,
				Message: "* Machine not-up-to-date-machine-1: some not up-to-date message",
			},
		},
		{
			name:              "One machine without up-to-date condition, one new Machines without up-to-date condition",
			machineDeployment: &clusterv1.MachineDeployment{},
			machines: []*clusterv1.Machine{
				fakeMachine("no-condition-machine-1"),
				fakeMachine("no-condition-machine-2-new", withCreationTimestamp(time.Now().Add(-5*time.Second))), // ignored because it's new
			},
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentMachinesUpToDateCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineDeploymentMachinesUpToDateUnknownReason,
				Message: "* Machine no-condition-machine-1: Condition UpToDate not yet reported",
			},
		},
		{
			name:              "Two machines not up-to-date, two up-to-date, two not reported",
			machineDeployment: &clusterv1.MachineDeployment{},
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
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeploymentMachinesUpToDateCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineDeploymentMachinesNotUpToDateReason,
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
			setMachinesUpToDateCondition(ctx, tt.machineDeployment, machines)

			condition := conditions.Get(tt.machineDeployment, clusterv1.MachineDeploymentMachinesUpToDateCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tt.expectCondition, conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func Test_setRemediatingCondition(t *testing.T) {
	healthCheckSucceeded := metav1.Condition{Type: clusterv1.MachineHealthCheckSucceededCondition, Status: metav1.ConditionTrue}
	healthCheckNotSucceeded := metav1.Condition{Type: clusterv1.MachineHealthCheckSucceededCondition, Status: metav1.ConditionFalse}
	ownerRemediated := metav1.Condition{Type: clusterv1.MachineOwnerRemediatedCondition, Status: metav1.ConditionFalse, Reason: clusterv1.MachineSetMachineRemediationMachineDeletingReason, Message: "Machine is deleting"}

	tests := []struct {
		name              string
		machineDeployment *clusterv1.MachineDeployment
		machines          []*clusterv1.Machine
		expectCondition   metav1.Condition
	}{
		{
			name:              "Without unhealthy machines",
			machineDeployment: &clusterv1.MachineDeployment{},
			machines: []*clusterv1.Machine{
				fakeMachine("m1"),
				fakeMachine("m2"),
			},
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeploymentRemediatingCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineDeploymentNotRemediatingReason,
			},
		},
		{
			name:              "With machines to be remediated by MD/MS",
			machineDeployment: &clusterv1.MachineDeployment{},
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withCondition(healthCheckSucceeded)),    // Healthy machine
				fakeMachine("m2", withCondition(healthCheckNotSucceeded)), // Unhealthy machine, not yet marked for remediation
				fakeMachine("m3", withCondition(healthCheckNotSucceeded), withCondition(ownerRemediated)),
			},
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentRemediatingCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineDeploymentRemediatingReason,
				Message: "* Machine m3: Machine is deleting",
			},
		},
		{
			name:              "With one unhealthy machine not to be remediated by MD/MS",
			machineDeployment: &clusterv1.MachineDeployment{},
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withCondition(healthCheckSucceeded)),    // Healthy machine
				fakeMachine("m2", withCondition(healthCheckNotSucceeded)), // Unhealthy machine, not yet marked for remediation
				fakeMachine("m3", withCondition(healthCheckSucceeded)),    // Healthy machine
			},
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentRemediatingCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineDeploymentNotRemediatingReason,
				Message: "Machine m2 is not healthy (not to be remediated by MachineDeployment/MachineSet)",
			},
		},
		{
			name:              "With two unhealthy machine not to be remediated by MD/MS",
			machineDeployment: &clusterv1.MachineDeployment{},
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withCondition(healthCheckNotSucceeded)), // Unhealthy machine, not yet marked for remediation
				fakeMachine("m2", withCondition(healthCheckNotSucceeded)), // Unhealthy machine, not yet marked for remediation
				fakeMachine("m3", withCondition(healthCheckSucceeded)),    // Healthy machine
			},
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentRemediatingCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineDeploymentNotRemediatingReason,
				Message: "Machines m1, m2 are not healthy (not to be remediated by MachineDeployment/MachineSet)",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			machines := collections.FromMachines(tt.machines...)
			machinesToBeRemediated := machines.Filter(collections.IsUnhealthyAndOwnerRemediated)
			unHealthyMachines := machines.Filter(collections.IsUnhealthy)
			setRemediatingCondition(ctx, tt.machineDeployment, machinesToBeRemediated, unHealthyMachines)

			condition := conditions.Get(tt.machineDeployment, clusterv1.MachineDeploymentRemediatingCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tt.expectCondition, conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func Test_setDeletingCondition(t *testing.T) {
	tests := []struct {
		name                                         string
		machineDeployment                            *clusterv1.MachineDeployment
		machineSets                                  []*clusterv1.MachineSet
		getAndAdoptMachineSetsForDeploymentSucceeded bool
		machines                                     []*clusterv1.Machine
		expectCondition                              metav1.Condition
	}{
		{
			name:              "get machine sets failed",
			machineDeployment: &clusterv1.MachineDeployment{},
			machineSets:       nil,
			getAndAdoptMachineSetsForDeploymentSucceeded: false,
			machines: []*clusterv1.Machine{},
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentDeletingCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineDeploymentDeletingInternalErrorReason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:              "not deleting",
			machineDeployment: &clusterv1.MachineDeployment{},
			machineSets:       []*clusterv1.MachineSet{},
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			machines: []*clusterv1.Machine{},
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeploymentDeletingCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineDeploymentNotDeletingReason,
			},
		},
		{
			name:              "Deleting with still some machine",
			machineDeployment: &clusterv1.MachineDeployment{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &metav1.Time{Time: time.Now()}}},
			machineSets: []*clusterv1.MachineSet{
				fakeMachineSet("ms1", withStatusReplicas(1)),
			},
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			machines: []*clusterv1.Machine{
				fakeMachine("m1"),
			},
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentDeletingCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineDeploymentDeletingReason,
				Message: "Deleting 1 Machine",
			},
		},
		{
			name:              "Deleting with still some stale machine",
			machineDeployment: &clusterv1.MachineDeployment{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &metav1.Time{Time: time.Now()}}},
			machineSets: []*clusterv1.MachineSet{
				fakeMachineSet("ms1", withStatusReplicas(1)),
			},
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withStaleDeletion()),
			},
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeploymentDeletingCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineDeploymentDeletingReason,
				Message: "Deleting 1 Machine\n" +
					"* Machine m1 is in deletion since more than 15m",
			},
		},
		{
			name:              "Deleting with no machines and a machine set still around",
			machineDeployment: &clusterv1.MachineDeployment{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &metav1.Time{Time: time.Now()}}},
			machineSets: []*clusterv1.MachineSet{
				fakeMachineSet("ms1", withStatusReplicas(1)),
			},
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			machines: []*clusterv1.Machine{},
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentDeletingCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineDeploymentDeletingReason,
				Message: "Deleting 1 MachineSets",
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
			setDeletingCondition(ctx, tt.machineDeployment, tt.machineSets, machines, tt.getAndAdoptMachineSetsForDeploymentSucceeded)

			condition := conditions.Get(tt.machineDeployment, clusterv1.MachineDeploymentDeletingCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tt.expectCondition, conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

type fakeMachineSetOption func(ms *clusterv1.MachineSet)

func fakeMachineSet(name string, options ...fakeMachineSetOption) *clusterv1.MachineSet {
	p := &clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	for _, opt := range options {
		opt(p)
	}
	return p
}

func withStatusReplicas(n int32) fakeMachineSetOption {
	return func(ms *clusterv1.MachineSet) {
		ms.Status.Replicas = ptr.To(n)
	}
}

func withStatusV1beta2ReadyReplicas(n int32) fakeMachineSetOption {
	return func(ms *clusterv1.MachineSet) {
		ms.Status.ReadyReplicas = ptr.To(n)
	}
}

func withStatusV1beta2AvailableReplicas(n int32) fakeMachineSetOption {
	return func(ms *clusterv1.MachineSet) {
		ms.Status.AvailableReplicas = ptr.To(n)
	}
}

func withStatusV1beta2UpToDateReplicas(n int32) fakeMachineSetOption {
	return func(ms *clusterv1.MachineSet) {
		ms.Status.UpToDateReplicas = ptr.To(n)
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

func withCreationTimestamp(t time.Time) fakeMachinesOption {
	return func(m *clusterv1.Machine) {
		m.CreationTimestamp = metav1.Time{Time: t}
	}
}

func withStaleDeletion() fakeMachinesOption {
	return func(m *clusterv1.Machine) {
		m.DeletionTimestamp = ptr.To(metav1.Time{Time: time.Now().Add(-1 * time.Hour)})
	}
}

func withCondition(c metav1.Condition) fakeMachinesOption {
	return func(m *clusterv1.Machine) {
		conditions.Set(m, c)
	}
}
