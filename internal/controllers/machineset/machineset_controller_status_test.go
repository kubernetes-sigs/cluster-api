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

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/collections"
	v1beta2conditions "sigs.k8s.io/cluster-api/util/conditions/v1beta2"
)

func Test_setReplicas(t *testing.T) {
	tests := []struct {
		name                                      string
		machines                                  []*clusterv1.Machine
		getAndAdoptMachinesForMachineSetSucceeded bool
		expectedStatus                            *clusterv1.MachineSetV1Beta2Status
	}{
		{
			name:     "getAndAdoptMachines failed",
			machines: nil,
			getAndAdoptMachinesForMachineSetSucceeded: false,
			expectedStatus: nil,
		},
		{
			name:     "no machines",
			machines: nil,
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectedStatus: &clusterv1.MachineSetV1Beta2Status{
				ReadyReplicas:     ptr.To[int32](0),
				AvailableReplicas: ptr.To[int32](0),
				UpToDateReplicas:  ptr.To[int32](0),
			}},
		{
			name: "should count only ready machines",
			machines: []*clusterv1.Machine{
				{Status: clusterv1.MachineStatus{V1Beta2: &clusterv1.MachineV1Beta2Status{Conditions: []metav1.Condition{{
					Type:   clusterv1.MachineReadyV1Beta2Condition,
					Status: metav1.ConditionTrue,
				}}}}},
				{Status: clusterv1.MachineStatus{V1Beta2: &clusterv1.MachineV1Beta2Status{Conditions: []metav1.Condition{{
					Type:   clusterv1.MachineReadyV1Beta2Condition,
					Status: metav1.ConditionFalse,
				}}}}},
				{Status: clusterv1.MachineStatus{V1Beta2: &clusterv1.MachineV1Beta2Status{Conditions: []metav1.Condition{{
					Type:   clusterv1.MachineReadyV1Beta2Condition,
					Status: metav1.ConditionUnknown,
				}}}}},
			},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectedStatus: &clusterv1.MachineSetV1Beta2Status{
				ReadyReplicas:     ptr.To[int32](1),
				AvailableReplicas: ptr.To[int32](0),
				UpToDateReplicas:  ptr.To[int32](0),
			},
		},
		{
			name: "should count only available machines",
			machines: []*clusterv1.Machine{
				{Status: clusterv1.MachineStatus{V1Beta2: &clusterv1.MachineV1Beta2Status{Conditions: []metav1.Condition{{
					Type:   clusterv1.MachineAvailableV1Beta2Condition,
					Status: metav1.ConditionTrue,
				}}}}},
				{Status: clusterv1.MachineStatus{V1Beta2: &clusterv1.MachineV1Beta2Status{Conditions: []metav1.Condition{{
					Type:   clusterv1.MachineAvailableV1Beta2Condition,
					Status: metav1.ConditionFalse,
				}}}}},
				{Status: clusterv1.MachineStatus{V1Beta2: &clusterv1.MachineV1Beta2Status{Conditions: []metav1.Condition{{
					Type:   clusterv1.MachineAvailableV1Beta2Condition,
					Status: metav1.ConditionUnknown,
				}}}}},
			},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectedStatus: &clusterv1.MachineSetV1Beta2Status{
				ReadyReplicas:     ptr.To[int32](0),
				AvailableReplicas: ptr.To[int32](1),
				UpToDateReplicas:  ptr.To[int32](0),
			},
		},
		{
			name: "should count only up-to-date machines",
			machines: []*clusterv1.Machine{
				{Status: clusterv1.MachineStatus{V1Beta2: &clusterv1.MachineV1Beta2Status{Conditions: []metav1.Condition{{
					Type:   clusterv1.MachineUpToDateV1Beta2Condition,
					Status: metav1.ConditionTrue,
				}}}}},
				{Status: clusterv1.MachineStatus{V1Beta2: &clusterv1.MachineV1Beta2Status{Conditions: []metav1.Condition{{
					Type:   clusterv1.MachineUpToDateV1Beta2Condition,
					Status: metav1.ConditionFalse,
				}}}}},
				{Status: clusterv1.MachineStatus{V1Beta2: &clusterv1.MachineV1Beta2Status{Conditions: []metav1.Condition{{
					Type:   clusterv1.MachineUpToDateV1Beta2Condition,
					Status: metav1.ConditionUnknown,
				}}}}},
			},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectedStatus: &clusterv1.MachineSetV1Beta2Status{
				ReadyReplicas:     ptr.To[int32](0),
				AvailableReplicas: ptr.To[int32](0),
				UpToDateReplicas:  ptr.To[int32](1),
			},
		},
		{
			name: "should count all conditions from a machine",
			machines: []*clusterv1.Machine{
				{Status: clusterv1.MachineStatus{V1Beta2: &clusterv1.MachineV1Beta2Status{Conditions: []metav1.Condition{
					{
						Type:   clusterv1.MachineReadyV1Beta2Condition,
						Status: metav1.ConditionTrue,
					},
					{
						Type:   clusterv1.MachineAvailableV1Beta2Condition,
						Status: metav1.ConditionTrue,
					},
					{
						Type:   clusterv1.MachineUpToDateV1Beta2Condition,
						Status: metav1.ConditionTrue,
					},
				}}}},
			},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectedStatus: &clusterv1.MachineSetV1Beta2Status{
				ReadyReplicas:     ptr.To[int32](1),
				AvailableReplicas: ptr.To[int32](1),
				UpToDateReplicas:  ptr.To[int32](1),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			ms := &clusterv1.MachineSet{}
			setReplicas(ctx, ms, tt.machines, tt.getAndAdoptMachinesForMachineSetSucceeded)
			g.Expect(ms.Status.V1Beta2).To(BeEquivalentTo(tt.expectedStatus))
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
				Type:    clusterv1.MachineSetScalingUpV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineSetScalingUpInternalErrorV1Beta2Reason,
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
				Type:   clusterv1.MachineSetScalingUpV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineSetNotScalingUpV1Beta2Reason,
			},
		},
		{
			name:                         "not scaling up and no machines and bootstrapConfig object not found",
			ms:                           defaultMachineSet,
			bootstrapObjectNotFound:      true,
			infrastructureObjectNotFound: false,
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineSetScalingUpV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineSetNotScalingUpV1Beta2Reason,
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
				Type:    clusterv1.MachineSetScalingUpV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineSetNotScalingUpV1Beta2Reason,
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
				Type:    clusterv1.MachineSetScalingUpV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineSetNotScalingUpV1Beta2Reason,
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
				Type:    clusterv1.MachineSetScalingUpV1Beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineSetScalingUpV1Beta2Reason,
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
				Type:   clusterv1.MachineSetScalingUpV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineSetScalingUpV1Beta2Reason,
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
				Type:   clusterv1.MachineSetScalingUpV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineSetScalingUpV1Beta2Reason,
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
				Type:   clusterv1.MachineSetScalingUpV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineSetScalingUpV1Beta2Reason,
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
				Type:   clusterv1.MachineSetScalingUpV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineSetNotScalingUpV1Beta2Reason,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			setScalingUpCondition(ctx, tt.ms, tt.machines, tt.bootstrapObjectNotFound, tt.infrastructureObjectNotFound, tt.getAndAdoptMachinesForMachineSetSucceeded, tt.scaleUpPreflightCheckErrMessages)

			condition := v1beta2conditions.Get(tt.ms, clusterv1.MachineSetScalingUpV1Beta2Condition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(v1beta2conditions.MatchCondition(tt.expectCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
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
				Type:    clusterv1.MachineSetScalingDownV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineSetScalingDownInternalErrorV1Beta2Reason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:     "not scaling down and no machines",
			ms:       machineSet,
			machines: []*clusterv1.Machine{},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineSetScalingDownV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineSetNotScalingDownV1Beta2Reason,
			},
		},
		{
			name:     "not scaling down because scaling up",
			ms:       machineSet1Replica,
			machines: []*clusterv1.Machine{},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineSetScalingDownV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineSetNotScalingDownV1Beta2Reason,
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
				Type:    clusterv1.MachineSetScalingDownV1Beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineSetScalingDownV1Beta2Reason,
				Message: "Scaling down from 1 to 0 replicas",
			},
		},
		{
			name: "scaling down with 1 stale machine",
			ms:   machineSet1Replica,
			machines: []*clusterv1.Machine{
				fakeMachine("stale-machine-1", withStaleDeletionTimestamp()),
				fakeMachine("machine-2"),
			},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineSetScalingDownV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineSetScalingDownV1Beta2Reason,
				Message: "Scaling down from 2 to 1 replicas\n" +
					"* Machine stale-machine-1 is in deletion since more than 15m",
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
				Type:   clusterv1.MachineSetScalingDownV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineSetScalingDownV1Beta2Reason,
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
				Type:   clusterv1.MachineSetScalingDownV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineSetScalingDownV1Beta2Reason,
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
				Type:   clusterv1.MachineSetScalingDownV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineSetNotScalingDownV1Beta2Reason,
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
				Type:    clusterv1.MachineSetScalingDownV1Beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineSetScalingDownV1Beta2Reason,
				Message: "Scaling down from 1 to 0 replicas",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			setScalingDownCondition(ctx, tt.ms, tt.machines, tt.getAndAdoptMachinesForMachineSetSucceeded)

			condition := v1beta2conditions.Get(tt.ms, clusterv1.MachineSetScalingDownV1Beta2Condition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(v1beta2conditions.MatchCondition(tt.expectCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func Test_setMachinesReadyCondition(t *testing.T) {
	machineSet := &clusterv1.MachineSet{}

	readyCondition := metav1.Condition{
		Type:   clusterv1.MachineReadyV1Beta2Condition,
		Status: metav1.ConditionTrue,
		Reason: clusterv1.MachineReadyV1Beta2Reason,
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
				Type:    clusterv1.MachineSetMachinesReadyV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineSetMachinesReadyInternalErrorV1Beta2Reason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:       "no machines",
			machineSet: machineSet,
			machines:   []*clusterv1.Machine{},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineSetMachinesReadyV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineSetMachinesReadyNoReplicasV1Beta2Reason,
			},
		},
		{
			name:       "one machine is ready",
			machineSet: machineSet,
			machines: []*clusterv1.Machine{
				fakeMachine("machine-1", withV1Beta2Condition(readyCondition)),
			},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineSetMachinesReadyV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineSetMachinesReadyV1Beta2Reason,
			},
		},
		{
			name:       "all machines are ready",
			machineSet: machineSet,
			machines: []*clusterv1.Machine{
				fakeMachine("machine-1", withV1Beta2Condition(readyCondition)),
				fakeMachine("machine-2", withV1Beta2Condition(readyCondition)),
			},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineSetMachinesReadyV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineSetMachinesReadyV1Beta2Reason,
			},
		},
		{
			name:       "one ready, one has nothing reported",
			machineSet: machineSet,
			machines: []*clusterv1.Machine{
				fakeMachine("machine-1", withV1Beta2Condition(readyCondition)),
				fakeMachine("machine-2"),
			},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineSetMachinesReadyV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineSetMachinesReadyUnknownV1Beta2Reason,
				Message: "* Machine machine-2: Condition Ready not yet reported",
			},
		},
		{
			name:       "one ready, one reporting not ready, one reporting unknown, one reporting deleting",
			machineSet: machineSet,
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
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineSetMachinesReadyV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineSetMachinesNotReadyV1Beta2Reason,
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

			condition := v1beta2conditions.Get(tt.machineSet, clusterv1.MachineSetMachinesReadyV1Beta2Condition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(v1beta2conditions.MatchCondition(tt.expectCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
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
				Type:    clusterv1.MachineSetMachinesUpToDateV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineSetMachinesUpToDateInternalErrorV1Beta2Reason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:       "no machines",
			machineSet: machineSet,
			machines:   []*clusterv1.Machine{},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineSetMachinesUpToDateV1Beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineSetMachinesUpToDateNoReplicasV1Beta2Reason,
				Message: "",
			},
		},
		{
			name:       "One machine up-to-date",
			machineSet: machineSet,
			machines: []*clusterv1.Machine{
				fakeMachine("up-to-date-1", withV1Beta2Condition(metav1.Condition{
					Type:   clusterv1.MachineUpToDateV1Beta2Condition,
					Status: metav1.ConditionTrue,
					Reason: "some-reason-1",
				})),
			},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineSetMachinesUpToDateV1Beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineSetMachinesUpToDateV1Beta2Reason,
				Message: "",
			},
		},
		{
			name:       "One machine unknown",
			machineSet: machineSet,
			machines: []*clusterv1.Machine{
				fakeMachine("unknown-1", withV1Beta2Condition(metav1.Condition{
					Type:    clusterv1.MachineUpToDateV1Beta2Condition,
					Status:  metav1.ConditionUnknown,
					Reason:  "some-unknown-reason-1",
					Message: "some unknown message",
				})),
			},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineSetMachinesUpToDateV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineSetMachinesUpToDateUnknownV1Beta2Reason,
				Message: "* Machine unknown-1: some unknown message",
			},
		},
		{
			name:       "One machine not up-to-date",
			machineSet: machineSet,
			machines: []*clusterv1.Machine{
				fakeMachine("not-up-to-date-machine-1", withV1Beta2Condition(metav1.Condition{
					Type:    clusterv1.MachineUpToDateV1Beta2Condition,
					Status:  metav1.ConditionFalse,
					Reason:  "some-not-up-to-date-reason",
					Message: "some not up-to-date message",
				})),
			},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineSetMachinesUpToDateV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineSetMachinesNotUpToDateV1Beta2Reason,
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
				Type:    clusterv1.MachineSetMachinesUpToDateV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineSetMachinesUpToDateUnknownV1Beta2Reason,
				Message: "* Machine no-condition-machine-1: Condition UpToDate not yet reported",
			},
		},
		{
			name:       "Two machines not up-to-date, two up-to-date, two not reported",
			machineSet: machineSet,
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
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineSetMachinesUpToDateV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineSetMachinesNotUpToDateV1Beta2Reason,
				Message: "* Machines not-up-to-date-machine-1, not-up-to-date-machine-2: This is not up-to-date message\n" +
					"* Machines no-condition-machine-1, no-condition-machine-2: Condition UpToDate not yet reported",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			setMachinesUpToDateCondition(ctx, tt.machineSet, tt.machines, tt.getAndAdoptMachinesForMachineSetSucceeded)

			condition := v1beta2conditions.Get(tt.machineSet, clusterv1.MachineSetMachinesUpToDateV1Beta2Condition)
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
	ownerRemediatedWaitingForRemediationV1Beta2 := metav1.Condition{Type: clusterv1.MachineOwnerRemediatedV1Beta2Condition, Status: metav1.ConditionFalse, Reason: clusterv1.MachineOwnerRemediatedWaitingForRemediationV1Beta2Reason, Message: "KubeadmControlPlane ns1/cp1 is upgrading (\"ControlPlaneIsStable\" preflight check failed)"}

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
				Type:    clusterv1.MachineSetRemediatingV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineSetRemediatingInternalErrorV1Beta2Reason,
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
				Type:   clusterv1.MachineSetRemediatingV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineSetNotRemediatingV1Beta2Reason,
			},
		},
		{
			name:       "With machines to be remediated by MS",
			machineSet: &clusterv1.MachineSet{},
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withConditions(healthCheckSucceeded)),    // Healthy machine
				fakeMachine("m2", withConditions(healthCheckNotSucceeded)), // Unhealthy machine, not yet marked for remediation
				fakeMachine("m3", withConditions(healthCheckNotSucceeded, ownerRemediated), withV1Beta2Condition(ownerRemediatedV1Beta2)),
			},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineSetRemediatingV1Beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineSetRemediatingV1Beta2Reason,
				Message: "* Machine m3: Machine is deleting",
			},
		},
		{
			name:       "With machines to be remediated by MS and preflight check error",
			machineSet: &clusterv1.MachineSet{},
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withConditions(healthCheckSucceeded)),    // Healthy machine
				fakeMachine("m2", withConditions(healthCheckNotSucceeded)), // Unhealthy machine, not yet marked for remediation
				fakeMachine("m3", withConditions(healthCheckNotSucceeded, ownerRemediated), withV1Beta2Condition(ownerRemediatedV1Beta2)),
				fakeMachine("m4", withConditions(healthCheckNotSucceeded, ownerRemediated), withV1Beta2Condition(ownerRemediatedWaitingForRemediationV1Beta2)),
			},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			// This preflight check error can happen when a Machine becomes unhealthy while the control plane is upgrading.
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineSetRemediatingV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineSetRemediatingV1Beta2Reason,
				Message: "* Machine m3: Machine is deleting\n" +
					"* Machine m4: KubeadmControlPlane ns1/cp1 is upgrading (\"ControlPlaneIsStable\" preflight check failed)",
			},
		},
		{
			name:       "With one unhealthy machine not to be remediated by MS",
			machineSet: &clusterv1.MachineSet{},
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withConditions(healthCheckSucceeded)),    // Healthy machine
				fakeMachine("m2", withConditions(healthCheckNotSucceeded)), // Unhealthy machine, not yet marked for remediation
				fakeMachine("m3", withConditions(healthCheckSucceeded)),    // Healthy machine
			},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineSetRemediatingV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineSetNotRemediatingV1Beta2Reason,
				Message: "Machine m2 is not healthy (not to be remediated by MachineSet)",
			},
		},
		{
			name:       "With two unhealthy machine not to be remediated by MS",
			machineSet: &clusterv1.MachineSet{},
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withConditions(healthCheckNotSucceeded)), // Unhealthy machine, not yet marked for remediation
				fakeMachine("m2", withConditions(healthCheckNotSucceeded)), // Unhealthy machine, not yet marked for remediation
				fakeMachine("m3", withConditions(healthCheckSucceeded)),    // Healthy machine
			},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineSetRemediatingV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineSetNotRemediatingV1Beta2Reason,
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

			condition := v1beta2conditions.Get(tt.machineSet, clusterv1.MachineSetRemediatingV1Beta2Condition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(v1beta2conditions.MatchCondition(tt.expectCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
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
				Type:    clusterv1.MachineSetDeletingV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineSetDeletingInternalErrorV1Beta2Reason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:       "not deleting",
			machineSet: &clusterv1.MachineSet{},
			getAndAdoptMachinesForMachineSetSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineSetDeletingV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineSetNotDeletingV1Beta2Reason,
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
				Type:    clusterv1.MachineSetDeletingV1Beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineSetDeletingV1Beta2Reason,
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
				Type:   clusterv1.MachineSetDeletingV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineSetDeletingV1Beta2Reason,
				Message: "Deleting 3 Machines\n" +
					"* Machines m1, m2 are in deletion since more than 15m",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			setDeletingCondition(ctx, tt.machineSet, tt.machines, tt.getAndAdoptMachinesForMachineSetSucceeded)

			condition := v1beta2conditions.Get(tt.machineSet, clusterv1.MachineSetDeletingV1Beta2Condition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(v1beta2conditions.MatchCondition(tt.expectCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
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
				fakeMachine("m1", withStaleDeletionTimestamp(), withV1Beta2Condition(metav1.Condition{
					Type:   clusterv1.MachineDeletingV1Beta2Condition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineDeletingDrainingNodeV1Beta2Reason,
					Message: `Drain not completed yet (started at 2024-10-09T16:13:59Z):
* Pods pod-2-deletionTimestamp-set-1, pod-3-to-trigger-eviction-successfully-1: deletionTimestamp set, but still not removed from the Node
* Pod pod-5-to-trigger-eviction-pdb-violated-1: cannot evict pod as it would violate the pod's disruption budget. The disruption budget pod-5-pdb needs 20 healthy pods and has 20 currently
* Pod pod-6-to-trigger-eviction-some-other-error: failed to evict Pod, some other error 1
After above Pods have been removed from the Node, the following Pods will be evicted: pod-7-eviction-later, pod-8-eviction-later`,
				})),
				fakeMachine("m2", withStaleDeletionTimestamp(), withV1Beta2Condition(metav1.Condition{
					Type:   clusterv1.MachineDeletingV1Beta2Condition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineDeletingDrainingNodeV1Beta2Reason,
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
				fakeMachine("m1", withStaleDeletionTimestamp(), withV1Beta2Condition(metav1.Condition{
					Type:   clusterv1.MachineDeletingV1Beta2Condition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineDeletingDrainingNodeV1Beta2Reason,
					Message: `Drain not completed yet (started at 2024-10-09T16:13:59Z):
* Pods pod-2-deletionTimestamp-set-1, pod-3-to-trigger-eviction-successfully-1: deletionTimestamp set, but still not removed from the Node
* Pod pod-5-to-trigger-eviction-pdb-violated-1: cannot evict pod as it would violate the pod's disruption budget. The disruption budget pod-5-pdb needs 20 healthy pods and has 20 currently
* Pod pod-6-to-trigger-eviction-some-other-error: failed to evict Pod, some other error 1
After above Pods have been removed from the Node, the following Pods will be evicted: pod-7-eviction-later, pod-8-eviction-later`,
				}), withStaleDrain()),
				fakeMachine("m2", withStaleDeletionTimestamp(), withV1Beta2Condition(metav1.Condition{
					Type:   clusterv1.MachineDeletingV1Beta2Condition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineDeletingDrainingNodeV1Beta2Reason,
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

func withCreationTimestamp(t time.Time) fakeMachinesOption {
	return func(m *clusterv1.Machine) {
		m.CreationTimestamp = metav1.Time{Time: t}
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

func withStaleDrain() fakeMachinesOption {
	return func(m *clusterv1.Machine) {
		if m.Status.Deletion == nil {
			m.Status.Deletion = &clusterv1.MachineDeletionStatus{}
		}
		m.Status.Deletion.NodeDrainStartTime = ptr.To(metav1.Time{Time: time.Now().Add(-6 * time.Minute)})
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
