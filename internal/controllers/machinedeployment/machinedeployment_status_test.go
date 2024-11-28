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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/collections"
	v1beta2conditions "sigs.k8s.io/cluster-api/util/conditions/v1beta2"
)

func Test_setReplicas(t *testing.T) {
	tests := []struct {
		name                    string
		machineSets             []*clusterv1.MachineSet
		expectReadyReplicas     *int32
		expectAvailableReplicas *int32
		expectUpToDateReplicas  *int32
	}{
		{
			name:                    "No MachineSets",
			machineSets:             nil,
			expectReadyReplicas:     nil,
			expectAvailableReplicas: nil,
			expectUpToDateReplicas:  nil,
		},
		{
			name: "MachineSets without replicas set",
			machineSets: []*clusterv1.MachineSet{
				fakeMachineSet("ms1"),
			},
			expectReadyReplicas:     nil,
			expectAvailableReplicas: nil,
			expectUpToDateReplicas:  nil,
		},
		{
			name: "MachineSets with replicas set",
			machineSets: []*clusterv1.MachineSet{
				fakeMachineSet("ms1", withStatusV1beta2ReadyReplicas(5), withStatusV1beta2AvailableReplicas(3), withStatusV1beta2UpToDateReplicas(3)),
				fakeMachineSet("ms2", withStatusV1beta2ReadyReplicas(2), withStatusV1beta2AvailableReplicas(2), withStatusV1beta2UpToDateReplicas(1)),
				fakeMachineSet("ms3"),
			},
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

			g.Expect(md.Status.V1Beta2).ToNot(BeNil())
			g.Expect(md.Status.V1Beta2.ReadyReplicas).To(Equal(tt.expectReadyReplicas))
			g.Expect(md.Status.V1Beta2.AvailableReplicas).To(Equal(tt.expectAvailableReplicas))
			g.Expect(md.Status.V1Beta2.UpToDateReplicas).To(Equal(tt.expectUpToDateReplicas))
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
				Type:    clusterv1.MachineDeploymentAvailableV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineDeploymentAvailableInternalErrorV1Beta2Reason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:              "spec.replicas not set",
			machineDeployment: &clusterv1.MachineDeployment{},
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentAvailableV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineDeploymentAvailableWaitingForReplicasSetV1Beta2Reason,
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
				Type:    clusterv1.MachineDeploymentAvailableV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineDeploymentAvailableWaitingForAvailableReplicasSetV1Beta2Reason,
				Message: "Waiting for status.v1beta2.availableReplicas set",
			},
		},
		{
			name: "all the expected replicas are available",
			machineDeployment: &clusterv1.MachineDeployment{
				Spec: clusterv1.MachineDeploymentSpec{
					Replicas: ptr.To(int32(5)),
					Strategy: &clusterv1.MachineDeploymentStrategy{
						Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
						RollingUpdate: &clusterv1.MachineRollingUpdateDeployment{
							MaxSurge:       ptr.To(intstr.FromInt32(1)),
							MaxUnavailable: ptr.To(intstr.FromInt32(0)),
						},
					},
				},
				Status: clusterv1.MachineDeploymentStatus{V1Beta2: &clusterv1.MachineDeploymentV1Beta2Status{AvailableReplicas: ptr.To(int32(5))}},
			},
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeploymentAvailableV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineDeploymentAvailableV1Beta2Reason,
			},
		},
		{
			name: "some replicas are not available, but within MaxUnavailable range",
			machineDeployment: &clusterv1.MachineDeployment{
				Spec: clusterv1.MachineDeploymentSpec{
					Replicas: ptr.To(int32(5)),
					Strategy: &clusterv1.MachineDeploymentStrategy{
						Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
						RollingUpdate: &clusterv1.MachineRollingUpdateDeployment{
							MaxSurge:       ptr.To(intstr.FromInt32(1)),
							MaxUnavailable: ptr.To(intstr.FromInt32(1)),
						},
					},
				},
				Status: clusterv1.MachineDeploymentStatus{V1Beta2: &clusterv1.MachineDeploymentV1Beta2Status{AvailableReplicas: ptr.To(int32(4))}},
			},
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeploymentAvailableV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineDeploymentAvailableV1Beta2Reason,
			},
		},
		{
			name: "some replicas are not available, more than MaxUnavailable",
			machineDeployment: &clusterv1.MachineDeployment{
				Spec: clusterv1.MachineDeploymentSpec{
					Replicas: ptr.To(int32(5)),
					Strategy: &clusterv1.MachineDeploymentStrategy{
						Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
						RollingUpdate: &clusterv1.MachineRollingUpdateDeployment{
							MaxSurge:       ptr.To(intstr.FromInt32(1)),
							MaxUnavailable: ptr.To(intstr.FromInt32(1)),
						},
					},
				},
				Status: clusterv1.MachineDeploymentStatus{V1Beta2: &clusterv1.MachineDeploymentV1Beta2Status{AvailableReplicas: ptr.To(int32(3))}},
			},
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentAvailableV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineDeploymentNotAvailableV1Beta2Reason,
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
					Strategy: &clusterv1.MachineDeploymentStrategy{
						Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
						RollingUpdate: &clusterv1.MachineRollingUpdateDeployment{
							MaxSurge:       ptr.To(intstr.FromInt32(1)),
							MaxUnavailable: ptr.To(intstr.FromInt32(1)),
						},
					},
				},
				Status: clusterv1.MachineDeploymentStatus{V1Beta2: &clusterv1.MachineDeploymentV1Beta2Status{AvailableReplicas: ptr.To(int32(0))}},
			},
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentAvailableV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineDeploymentNotAvailableV1Beta2Reason,
				Message: "Deletion in progress",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			setAvailableCondition(ctx, tt.machineDeployment, tt.getAndAdoptMachineSetsForDeploymentSucceeded)

			condition := v1beta2conditions.Get(tt.machineDeployment, clusterv1.MachineDeploymentAvailableV1Beta2Condition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(v1beta2conditions.MatchCondition(tt.expectCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func Test_setRollingOutCondition(t *testing.T) {
	upToDateCondition := metav1.Condition{
		Type:   clusterv1.MachineUpToDateV1Beta2Condition,
		Status: metav1.ConditionTrue,
		Reason: clusterv1.MachineUpToDateV1Beta2Reason,
	}

	tests := []struct {
		name                 string
		machineDeployment    *clusterv1.MachineDeployment
		machines             []*clusterv1.Machine
		getMachinesSucceeded bool
		expectCondition      metav1.Condition
	}{
		{
			name:                 "get machines failed",
			machineDeployment:    &clusterv1.MachineDeployment{},
			machines:             nil,
			getMachinesSucceeded: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentRollingOutV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineDeploymentRollingOutInternalErrorV1Beta2Reason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:                 "no machines",
			machineDeployment:    &clusterv1.MachineDeployment{},
			machines:             []*clusterv1.Machine{},
			getMachinesSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeploymentRollingOutV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineDeploymentNotRollingOutV1Beta2Reason,
			},
		},
		{
			name:              "all machines are up to date",
			machineDeployment: &clusterv1.MachineDeployment{},
			machines: []*clusterv1.Machine{
				fakeMachine("machine-1", withV1Beta2Condition(upToDateCondition)),
				fakeMachine("machine-2", withV1Beta2Condition(upToDateCondition)),
			},
			getMachinesSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeploymentRollingOutV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineDeploymentNotRollingOutV1Beta2Reason,
			},
		},
		{
			name:              "one up-to-date, two not up-to-date, one reporting up-to-date unknown",
			machineDeployment: &clusterv1.MachineDeployment{},
			machines: []*clusterv1.Machine{
				fakeMachine("machine-1", withV1Beta2Condition(upToDateCondition)),
				fakeMachine("machine-2", withV1Beta2Condition(metav1.Condition{
					Type:   clusterv1.MachineUpToDateV1Beta2Condition,
					Status: metav1.ConditionUnknown,
					Reason: clusterv1.InternalErrorV1Beta2Reason,
				})),
				fakeMachine("machine-4", withV1Beta2Condition(metav1.Condition{
					Type:   clusterv1.MachineUpToDateV1Beta2Condition,
					Status: metav1.ConditionFalse,
					Reason: clusterv1.MachineNotUpToDateV1Beta2Reason,
					Message: "* Failure domain failure-domain1, failure-domain2 required\n" +
						"* InfrastructureMachine is not up-to-date",
				})),
				fakeMachine("machine-3", withV1Beta2Condition(metav1.Condition{
					Type:    clusterv1.MachineUpToDateV1Beta2Condition,
					Status:  metav1.ConditionFalse,
					Reason:  clusterv1.MachineNotUpToDateV1Beta2Reason,
					Message: "* Version v1.25.0, v1.26.0 required",
				})),
			},
			getMachinesSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeploymentRollingOutV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineDeploymentRollingOutV1Beta2Reason,
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
			setRollingOutCondition(ctx, tt.machineDeployment, machines, tt.getMachinesSucceeded)

			condition := v1beta2conditions.Get(tt.machineDeployment, clusterv1.MachineDeploymentRollingOutV1Beta2Condition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(v1beta2conditions.MatchCondition(tt.expectCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func Test_setScalingUpCondition(t *testing.T) {
	defaultMachineDeployment := &clusterv1.MachineDeployment{
		Spec: clusterv1.MachineDeploymentSpec{
			Replicas: ptr.To[int32](0),
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: &corev1.ObjectReference{
							Kind:      "KubeadmBootstrapTemplate",
							Namespace: "some-namespace",
							Name:      "some-name",
						},
					},
					InfrastructureRef: corev1.ObjectReference{
						Kind:      "DockerMachineTemplate",
						Namespace: "some-namespace",
						Name:      "some-name",
					},
				},
			},
		},
	}

	scalingUpMachineDeploymentWith3Replicas := defaultMachineDeployment.DeepCopy()
	scalingUpMachineDeploymentWith3Replicas.Spec.Replicas = ptr.To[int32](3)

	deletingMachineDeploymentWith3Replicas := defaultMachineDeployment.DeepCopy()
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
	}{
		{
			name:                           "getAndAdoptMachineSetsForDeploymentSucceeded failed",
			machineDeployment:              defaultMachineDeployment,
			bootstrapTemplateNotFound:      false,
			infrastructureTemplateNotFound: false,
			getAndAdoptMachineSetsForDeploymentSucceeded: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentScalingUpV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineDeploymentScalingUpInternalErrorV1Beta2Reason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name: "replicas not set",
			machineDeployment: func() *clusterv1.MachineDeployment {
				md := defaultMachineDeployment.DeepCopy()
				md.Spec.Replicas = nil
				return md
			}(),
			bootstrapTemplateNotFound:                    false,
			infrastructureTemplateNotFound:               false,
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentScalingUpV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineDeploymentScalingUpWaitingForReplicasSetV1Beta2Reason,
				Message: "Waiting for spec.replicas set",
			},
		},
		{
			name:                           "not scaling up and no machines",
			machineDeployment:              defaultMachineDeployment,
			bootstrapTemplateNotFound:      false,
			infrastructureTemplateNotFound: false,
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeploymentScalingUpV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineDeploymentNotScalingUpV1Beta2Reason,
			},
		},
		{
			name:              "not scaling up with machines",
			machineDeployment: deletingMachineDeploymentWith3Replicas,
			machineSets: []*clusterv1.MachineSet{
				fakeMachineSet("ms1", withStatusReplicas(1)),
				fakeMachineSet("ms2", withStatusReplicas(2)),
			},
			bootstrapTemplateNotFound:                    false,
			infrastructureTemplateNotFound:               false,
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeploymentScalingUpV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineDeploymentNotScalingUpV1Beta2Reason,
			},
		},
		{
			name:                           "not scaling up and no machines and bootstrapConfig object not found",
			machineDeployment:              defaultMachineDeployment,
			bootstrapTemplateNotFound:      true,
			infrastructureTemplateNotFound: false,
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentScalingUpV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineDeploymentNotScalingUpV1Beta2Reason,
				Message: "Scaling up would be blocked because KubeadmBootstrapTemplate does not exist",
			},
		},
		{
			name:                           "not scaling up and no machines and infrastructure object not found",
			machineDeployment:              defaultMachineDeployment,
			bootstrapTemplateNotFound:      false,
			infrastructureTemplateNotFound: true,
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentScalingUpV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineDeploymentNotScalingUpV1Beta2Reason,
				Message: "Scaling up would be blocked because DockerMachineTemplate does not exist",
			},
		},
		{
			name:                           "not scaling up and no machines and bootstrapConfig and infrastructure object not found",
			machineDeployment:              defaultMachineDeployment,
			bootstrapTemplateNotFound:      true,
			infrastructureTemplateNotFound: true,
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentScalingUpV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineDeploymentNotScalingUpV1Beta2Reason,
				Message: "Scaling up would be blocked because KubeadmBootstrapTemplate and DockerMachineTemplate do not exist",
			},
		},
		{
			name:                           "scaling up",
			machineDeployment:              scalingUpMachineDeploymentWith3Replicas,
			bootstrapTemplateNotFound:      false,
			infrastructureTemplateNotFound: false,
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentScalingUpV1Beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineDeploymentScalingUpV1Beta2Reason,
				Message: "Scaling up from 0 to 3 replicas",
			},
		},
		{
			name:              "scaling up with machines",
			machineDeployment: scalingUpMachineDeploymentWith3Replicas,
			machineSets: []*clusterv1.MachineSet{
				fakeMachineSet("ms1", withStatusReplicas(1)),
				fakeMachineSet("ms2", withStatusReplicas(1)),
			},
			bootstrapTemplateNotFound:                    false,
			infrastructureTemplateNotFound:               false,
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentScalingUpV1Beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineDeploymentScalingUpV1Beta2Reason,
				Message: "Scaling up from 2 to 3 replicas",
			},
		},
		{
			name:                           "scaling up and blocked by bootstrap object",
			machineDeployment:              scalingUpMachineDeploymentWith3Replicas,
			bootstrapTemplateNotFound:      true,
			infrastructureTemplateNotFound: false,
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentScalingUpV1Beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineDeploymentScalingUpV1Beta2Reason,
				Message: "Scaling up from 0 to 3 replicas is blocked because KubeadmBootstrapTemplate does not exist",
			},
		},
		{
			name:                           "scaling up and blocked by infrastructure object",
			machineDeployment:              scalingUpMachineDeploymentWith3Replicas,
			bootstrapTemplateNotFound:      false,
			infrastructureTemplateNotFound: true,
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentScalingUpV1Beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineDeploymentScalingUpV1Beta2Reason,
				Message: "Scaling up from 0 to 3 replicas is blocked because DockerMachineTemplate does not exist",
			},
		},
		{
			name:                           "deleting, don't show block message when templates are not found",
			machineDeployment:              deletingMachineDeploymentWith3Replicas,
			machineSets:                    []*clusterv1.MachineSet{{}, {}, {}},
			bootstrapTemplateNotFound:      true,
			infrastructureTemplateNotFound: true,
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeploymentScalingUpV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineDeploymentNotScalingUpV1Beta2Reason,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			setScalingUpCondition(ctx, tt.machineDeployment, tt.machineSets, tt.bootstrapTemplateNotFound, tt.infrastructureTemplateNotFound, tt.getAndAdoptMachineSetsForDeploymentSucceeded)

			condition := v1beta2conditions.Get(tt.machineDeployment, clusterv1.MachineDeploymentScalingUpV1Beta2Condition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(v1beta2conditions.MatchCondition(tt.expectCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
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
	}{
		{
			name:              "getAndAdoptMachineSetsForDeploymentSucceeded failed",
			machineDeployment: defaultMachineDeployment,
			machineSets:       nil,
			getAndAdoptMachineSetsForDeploymentSucceeded: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentScalingDownV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineDeploymentScalingDownInternalErrorV1Beta2Reason,
				Message: "Please check controller logs for errors",
			},
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
				Type:    clusterv1.MachineDeploymentScalingDownV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineDeploymentScalingDownWaitingForReplicasSetV1Beta2Reason,
				Message: "Waiting for spec.replicas set",
			},
		},
		{
			name:              "not scaling down and no machines",
			machineDeployment: defaultMachineDeployment,
			machineSets:       []*clusterv1.MachineSet{},
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeploymentScalingDownV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineDeploymentNotScalingDownV1Beta2Reason,
			},
		},
		{
			name:              "not scaling down because scaling up",
			machineDeployment: machineDeploymentWith1Replica,
			machineSets:       []*clusterv1.MachineSet{},
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeploymentScalingDownV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineDeploymentNotScalingDownV1Beta2Reason,
			},
		},
		{
			name:              "scaling down to zero",
			machineDeployment: defaultMachineDeployment,
			machineSets: []*clusterv1.MachineSet{
				fakeMachineSet("ms1", withStatusReplicas(1)),
			},
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentScalingDownV1Beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineDeploymentScalingDownV1Beta2Reason,
				Message: "Scaling down from 1 to 0 replicas",
			},
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
				fakeMachine("stale-machine-1", withStaleDeletion()),
			},
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeploymentScalingDownV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineDeploymentScalingDownV1Beta2Reason,
				Message: "Scaling down from 2 to 1 replicas\n" +
					"* Machine stale-machine-1 is in deletion since more than 15m",
			},
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
				Type:   clusterv1.MachineDeploymentScalingDownV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineDeploymentScalingDownV1Beta2Reason,
				Message: "Scaling down from 4 to 1 replicas\n" +
					"* Machines stale-machine-1, stale-machine-2, stale-machine-3 are in deletion since more than 15m",
			},
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
				Type:   clusterv1.MachineDeploymentScalingDownV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineDeploymentScalingDownV1Beta2Reason,
				Message: "Scaling down from 6 to 1 replicas\n" +
					"* Machines stale-machine-1, stale-machine-2, stale-machine-3, ... (2 more) are in deletion since more than 15m",
			},
		},
		{
			name:              "deleting machine deployment without replicas",
			machineDeployment: deletingMachineDeployment,
			machineSets:       []*clusterv1.MachineSet{},
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeploymentScalingDownV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineDeploymentNotScalingDownV1Beta2Reason,
			},
		},
		{
			name:              "deleting machine deployment having 1 replica",
			machineDeployment: deletingMachineDeployment,
			machineSets: []*clusterv1.MachineSet{
				fakeMachineSet("ms1", withStatusReplicas(1)),
			},
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentScalingDownV1Beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineDeploymentScalingDownV1Beta2Reason,
				Message: "Scaling down from 1 to 0 replicas",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			setScalingDownCondition(ctx, tt.machineDeployment, tt.machineSets, collections.FromMachines(tt.machines...), tt.getAndAdoptMachineSetsForDeploymentSucceeded, true)

			condition := v1beta2conditions.Get(tt.machineDeployment, clusterv1.MachineDeploymentScalingDownV1Beta2Condition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(v1beta2conditions.MatchCondition(tt.expectCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func Test_setMachinesReadyCondition(t *testing.T) {
	readyCondition := metav1.Condition{
		Type:   clusterv1.MachineReadyV1Beta2Condition,
		Status: metav1.ConditionTrue,
		Reason: clusterv1.MachineReadyV1Beta2Reason,
	}

	tests := []struct {
		name                 string
		machineDeployment    *clusterv1.MachineDeployment
		machines             []*clusterv1.Machine
		getMachinesSucceeded bool
		expectCondition      metav1.Condition
	}{
		{
			name:                 "get machines failed",
			machineDeployment:    &clusterv1.MachineDeployment{},
			machines:             nil,
			getMachinesSucceeded: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentMachinesReadyV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineDeploymentMachinesReadyInternalErrorV1Beta2Reason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:                 "no machines",
			machineDeployment:    &clusterv1.MachineDeployment{},
			machines:             []*clusterv1.Machine{},
			getMachinesSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeploymentMachinesReadyV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineDeploymentMachinesReadyNoReplicasV1Beta2Reason,
			},
		},
		{
			name:              "all machines are ready",
			machineDeployment: &clusterv1.MachineDeployment{},
			machines: []*clusterv1.Machine{
				fakeMachine("machine-1", withV1Beta2Condition(readyCondition)),
				fakeMachine("machine-2", withV1Beta2Condition(readyCondition)),
			},
			getMachinesSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeploymentMachinesReadyV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineDeploymentMachinesReadyV1Beta2Reason,
			},
		},
		{
			name:              "one ready, one has nothing reported",
			machineDeployment: &clusterv1.MachineDeployment{},
			machines: []*clusterv1.Machine{
				fakeMachine("machine-1", withV1Beta2Condition(readyCondition)),
				fakeMachine("machine-2"),
			},
			getMachinesSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentMachinesReadyV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineDeploymentMachinesReadyUnknownV1Beta2Reason,
				Message: "* Machine machine-2: Condition Ready not yet reported",
			},
		},
		{
			name:              "one ready, one reporting not ready, one reporting unknown, one reporting deleting",
			machineDeployment: &clusterv1.MachineDeployment{},
			machines: []*clusterv1.Machine{
				fakeMachine("machine-1", withV1Beta2Condition(readyCondition)),
				fakeMachine("machine-2", withV1Beta2Condition(metav1.Condition{
					Type:    clusterv1.MachineReadyV1Beta2Condition,
					Status:  metav1.ConditionFalse,
					Reason:  "SomeReason",
					Message: "HealthCheckSucceeded: Some message",
				})),
				fakeMachine("machine-3", withV1Beta2Condition(metav1.Condition{
					Type:    clusterv1.MachineReadyV1Beta2Condition,
					Status:  metav1.ConditionUnknown,
					Reason:  "SomeUnknownReason",
					Message: "Some unknown message",
				})),
				fakeMachine("machine-4", withV1Beta2Condition(metav1.Condition{
					Type:    clusterv1.MachineReadyV1Beta2Condition,
					Status:  metav1.ConditionFalse,
					Reason:  clusterv1.MachineDeletingV1Beta2Reason,
					Message: "Deleting: Machine deletion in progress, stage: DrainingNode",
				})),
			},
			getMachinesSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeploymentMachinesReadyV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineDeploymentMachinesNotReadyV1Beta2Reason,
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
			setMachinesReadyCondition(ctx, tt.machineDeployment, machines, tt.getMachinesSucceeded)

			condition := v1beta2conditions.Get(tt.machineDeployment, clusterv1.MachineDeploymentMachinesReadyV1Beta2Condition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(v1beta2conditions.MatchCondition(tt.expectCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func Test_setMachinesUpToDateCondition(t *testing.T) {
	tests := []struct {
		name                 string
		machineDeployment    *clusterv1.MachineDeployment
		machines             []*clusterv1.Machine
		getMachinesSucceeded bool
		expectCondition      metav1.Condition
	}{
		{
			name:                 "get machines failed",
			machineDeployment:    &clusterv1.MachineDeployment{},
			machines:             nil,
			getMachinesSucceeded: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentMachinesUpToDateV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineDeploymentMachinesUpToDateInternalErrorV1Beta2Reason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:                 "no machines",
			machineDeployment:    &clusterv1.MachineDeployment{},
			machines:             []*clusterv1.Machine{},
			getMachinesSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentMachinesUpToDateV1Beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineDeploymentMachinesUpToDateNoReplicasV1Beta2Reason,
				Message: "",
			},
		},
		{
			name:              "One machine up-to-date",
			machineDeployment: &clusterv1.MachineDeployment{},
			machines: []*clusterv1.Machine{
				fakeMachine("up-to-date-1", withV1Beta2Condition(metav1.Condition{
					Type:   clusterv1.MachineUpToDateV1Beta2Condition,
					Status: metav1.ConditionTrue,
					Reason: "some-reason-1",
				})),
			},
			getMachinesSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentMachinesUpToDateV1Beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineDeploymentMachinesUpToDateV1Beta2Reason,
				Message: "",
			},
		},
		{
			name:              "One machine unknown",
			machineDeployment: &clusterv1.MachineDeployment{},
			machines: []*clusterv1.Machine{
				fakeMachine("unknown-1", withV1Beta2Condition(metav1.Condition{
					Type:    clusterv1.MachineUpToDateV1Beta2Condition,
					Status:  metav1.ConditionUnknown,
					Reason:  "some-unknown-reason-1",
					Message: "some unknown message",
				})),
			},
			getMachinesSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentMachinesUpToDateV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineDeploymentMachinesUpToDateUnknownV1Beta2Reason,
				Message: "* Machine unknown-1: some unknown message",
			},
		},
		{
			name:              "One machine not up-to-date",
			machineDeployment: &clusterv1.MachineDeployment{},
			machines: []*clusterv1.Machine{
				fakeMachine("not-up-to-date-machine-1", withV1Beta2Condition(metav1.Condition{
					Type:    clusterv1.MachineUpToDateV1Beta2Condition,
					Status:  metav1.ConditionFalse,
					Reason:  "some-not-up-to-date-reason",
					Message: "some not up-to-date message",
				})),
			},
			getMachinesSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentMachinesUpToDateV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineDeploymentMachinesNotUpToDateV1Beta2Reason,
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
			getMachinesSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentMachinesUpToDateV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineDeploymentMachinesUpToDateUnknownV1Beta2Reason,
				Message: "* Machine no-condition-machine-1: Condition UpToDate not yet reported",
			},
		},
		{
			name:              "Two machines not up-to-date, two up-to-date, two not reported",
			machineDeployment: &clusterv1.MachineDeployment{},
			machines: []*clusterv1.Machine{
				fakeMachine("up-to-date-1", withV1Beta2Condition(metav1.Condition{
					Type:   clusterv1.MachineUpToDateV1Beta2Condition,
					Status: metav1.ConditionTrue,
					Reason: "TestUpToDate",
				})),
				fakeMachine("up-to-date-2", withV1Beta2Condition(metav1.Condition{
					Type:   clusterv1.MachineUpToDateV1Beta2Condition,
					Status: metav1.ConditionTrue,
					Reason: "TestUpToDate",
				})),
				fakeMachine("not-up-to-date-machine-1", withV1Beta2Condition(metav1.Condition{
					Type:    clusterv1.MachineUpToDateV1Beta2Condition,
					Status:  metav1.ConditionFalse,
					Reason:  "TestNotUpToDate",
					Message: "This is not up-to-date message",
				})),
				fakeMachine("not-up-to-date-machine-2", withV1Beta2Condition(metav1.Condition{
					Type:    clusterv1.MachineUpToDateV1Beta2Condition,
					Status:  metav1.ConditionFalse,
					Reason:  "TestNotUpToDate",
					Message: "This is not up-to-date message",
				})),
				fakeMachine("no-condition-machine-1"),
				fakeMachine("no-condition-machine-2"),
			},
			getMachinesSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeploymentMachinesUpToDateV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineDeploymentMachinesNotUpToDateV1Beta2Reason,
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
			setMachinesUpToDateCondition(ctx, tt.machineDeployment, machines, tt.getMachinesSucceeded)

			condition := v1beta2conditions.Get(tt.machineDeployment, clusterv1.MachineDeploymentMachinesUpToDateV1Beta2Condition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(v1beta2conditions.MatchCondition(tt.expectCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func Test_setRemediatingCondition(t *testing.T) {
	healthCheckSucceeded := clusterv1.Condition{Type: clusterv1.MachineHealthCheckSucceededV1Beta2Condition, Status: corev1.ConditionTrue}
	healthCheckNotSucceeded := clusterv1.Condition{Type: clusterv1.MachineHealthCheckSucceededV1Beta2Condition, Status: corev1.ConditionFalse}
	ownerRemediated := clusterv1.Condition{Type: clusterv1.MachineOwnerRemediatedCondition, Status: corev1.ConditionFalse}
	ownerRemediatedV1Beta2 := metav1.Condition{Type: clusterv1.MachineOwnerRemediatedV1Beta2Condition, Status: metav1.ConditionFalse, Reason: clusterv1.MachineSetMachineRemediationMachineDeletingV1Beta2Reason, Message: "Machine is deleting"}

	tests := []struct {
		name                 string
		machineDeployment    *clusterv1.MachineDeployment
		machines             []*clusterv1.Machine
		getMachinesSucceeded bool
		expectCondition      metav1.Condition
	}{
		{
			name:                 "get machines failed",
			machineDeployment:    &clusterv1.MachineDeployment{},
			machines:             nil,
			getMachinesSucceeded: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentRemediatingV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineDeploymentRemediatingInternalErrorV1Beta2Reason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:              "Without unhealthy machines",
			machineDeployment: &clusterv1.MachineDeployment{},
			machines: []*clusterv1.Machine{
				fakeMachine("m1"),
				fakeMachine("m2"),
			},
			getMachinesSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeploymentRemediatingV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineDeploymentNotRemediatingV1Beta2Reason,
			},
		},
		{
			name:              "With machines to be remediated by MD/MS",
			machineDeployment: &clusterv1.MachineDeployment{},
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withConditions(healthCheckSucceeded)),    // Healthy machine
				fakeMachine("m2", withConditions(healthCheckNotSucceeded)), // Unhealthy machine, not yet marked for remediation
				fakeMachine("m3", withConditions(healthCheckNotSucceeded, ownerRemediated), withV1Beta2Condition(ownerRemediatedV1Beta2)),
			},
			getMachinesSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentRemediatingV1Beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineDeploymentRemediatingV1Beta2Reason,
				Message: "* Machine m3: Machine is deleting",
			},
		},
		{
			name:              "With one unhealthy machine not to be remediated by MD/MS",
			machineDeployment: &clusterv1.MachineDeployment{},
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withConditions(healthCheckSucceeded)),    // Healthy machine
				fakeMachine("m2", withConditions(healthCheckNotSucceeded)), // Unhealthy machine, not yet marked for remediation
				fakeMachine("m3", withConditions(healthCheckSucceeded)),    // Healthy machine
			},
			getMachinesSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentRemediatingV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineDeploymentNotRemediatingV1Beta2Reason,
				Message: "Machine m2 is not healthy (not to be remediated by MachineDeployment/MachineSet)",
			},
		},
		{
			name:              "With two unhealthy machine not to be remediated by MD/MS",
			machineDeployment: &clusterv1.MachineDeployment{},
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withConditions(healthCheckNotSucceeded)), // Unhealthy machine, not yet marked for remediation
				fakeMachine("m2", withConditions(healthCheckNotSucceeded)), // Unhealthy machine, not yet marked for remediation
				fakeMachine("m3", withConditions(healthCheckSucceeded)),    // Healthy machine
			},
			getMachinesSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentRemediatingV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineDeploymentNotRemediatingV1Beta2Reason,
				Message: "Machines m1, m2 are not healthy (not to be remediated by MachineDeployment/MachineSet)",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			var machinesToBeRemediated, unHealthyMachines collections.Machines
			if tt.getMachinesSucceeded {
				machines := collections.FromMachines(tt.machines...)
				machinesToBeRemediated = machines.Filter(collections.IsUnhealthyAndOwnerRemediated)
				unHealthyMachines = machines.Filter(collections.IsUnhealthy)
			}
			setRemediatingCondition(ctx, tt.machineDeployment, machinesToBeRemediated, unHealthyMachines, tt.getMachinesSucceeded)

			condition := v1beta2conditions.Get(tt.machineDeployment, clusterv1.MachineDeploymentRemediatingV1Beta2Condition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(v1beta2conditions.MatchCondition(tt.expectCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
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
		getMachinesSucceeded                         bool
		expectCondition                              metav1.Condition
	}{
		{
			name:              "get machine sets failed",
			machineDeployment: &clusterv1.MachineDeployment{},
			machineSets:       nil,
			getAndAdoptMachineSetsForDeploymentSucceeded: false,
			machines:             []*clusterv1.Machine{},
			getMachinesSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentDeletingV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineDeploymentDeletingInternalErrorV1Beta2Reason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:              "get machines failed",
			machineDeployment: &clusterv1.MachineDeployment{},
			machineSets:       []*clusterv1.MachineSet{},
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			machines:             nil,
			getMachinesSucceeded: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentDeletingV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineDeploymentDeletingInternalErrorV1Beta2Reason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:              "not deleting",
			machineDeployment: &clusterv1.MachineDeployment{},
			machineSets:       []*clusterv1.MachineSet{},
			getAndAdoptMachineSetsForDeploymentSucceeded: true,
			machines:             []*clusterv1.Machine{},
			getMachinesSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeploymentDeletingV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineDeploymentNotDeletingV1Beta2Reason,
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
			getMachinesSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentDeletingV1Beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineDeploymentDeletingV1Beta2Reason,
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
			getMachinesSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeploymentDeletingV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineDeploymentDeletingV1Beta2Reason,
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
			machines:             []*clusterv1.Machine{},
			getMachinesSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeploymentDeletingV1Beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineDeploymentDeletingV1Beta2Reason,
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
			setDeletingCondition(ctx, tt.machineDeployment, tt.machineSets, machines, tt.getAndAdoptMachineSetsForDeploymentSucceeded, tt.getMachinesSucceeded)

			condition := v1beta2conditions.Get(tt.machineDeployment, clusterv1.MachineDeploymentDeletingV1Beta2Condition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(v1beta2conditions.MatchCondition(tt.expectCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
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
		ms.Status.Replicas = n
	}
}

func withStatusV1beta2ReadyReplicas(n int32) fakeMachineSetOption {
	return func(ms *clusterv1.MachineSet) {
		if ms.Status.V1Beta2 == nil {
			ms.Status.V1Beta2 = &clusterv1.MachineSetV1Beta2Status{}
		}
		ms.Status.V1Beta2.ReadyReplicas = ptr.To(n)
	}
}

func withStatusV1beta2AvailableReplicas(n int32) fakeMachineSetOption {
	return func(ms *clusterv1.MachineSet) {
		if ms.Status.V1Beta2 == nil {
			ms.Status.V1Beta2 = &clusterv1.MachineSetV1Beta2Status{}
		}
		ms.Status.V1Beta2.AvailableReplicas = ptr.To(n)
	}
}

func withStatusV1beta2UpToDateReplicas(n int32) fakeMachineSetOption {
	return func(ms *clusterv1.MachineSet) {
		if ms.Status.V1Beta2 == nil {
			ms.Status.V1Beta2 = &clusterv1.MachineSetV1Beta2Status{}
		}
		ms.Status.V1Beta2.UpToDateReplicas = ptr.To(n)
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

func withV1Beta2Condition(c metav1.Condition) fakeMachinesOption {
	return func(m *clusterv1.Machine) {
		if m.Status.V1Beta2 == nil {
			m.Status.V1Beta2 = &clusterv1.MachineV1Beta2Status{}
		}
		v1beta2conditions.Set(m, c)
	}
}

func withConditions(c ...clusterv1.Condition) fakeMachinesOption {
	return func(m *clusterv1.Machine) {
		m.Status.Conditions = append(m.Status.Conditions, c...)
	}
}
