/*
Copyright 2020 The Kubernetes Authors.

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

package controllers

import (
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	controlplanev1webhooks "sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/webhooks"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta2conditions "sigs.k8s.io/cluster-api/util/conditions/v1beta2"
)

func TestSetReplicas(t *testing.T) {
	g := NewWithT(t)
	readyTrue := metav1.Condition{Type: clusterv1.MachineReadyV1Beta2Condition, Status: metav1.ConditionTrue}
	readyFalse := metav1.Condition{Type: clusterv1.MachineReadyV1Beta2Condition, Status: metav1.ConditionFalse}
	readyUnknown := metav1.Condition{Type: clusterv1.MachineReadyV1Beta2Condition, Status: metav1.ConditionUnknown}

	availableTrue := metav1.Condition{Type: clusterv1.MachineAvailableV1Beta2Condition, Status: metav1.ConditionTrue}
	availableFalse := metav1.Condition{Type: clusterv1.MachineAvailableV1Beta2Condition, Status: metav1.ConditionFalse}
	availableUnknown := metav1.Condition{Type: clusterv1.MachineAvailableV1Beta2Condition, Status: metav1.ConditionUnknown}

	upToDateTrue := metav1.Condition{Type: clusterv1.MachineUpToDateV1Beta2Condition, Status: metav1.ConditionTrue}
	upToDateFalse := metav1.Condition{Type: clusterv1.MachineUpToDateV1Beta2Condition, Status: metav1.ConditionFalse}
	upToDateUnknown := metav1.Condition{Type: clusterv1.MachineUpToDateV1Beta2Condition, Status: metav1.ConditionUnknown}

	kcp := &controlplanev1.KubeadmControlPlane{}
	c := &internal.ControlPlane{
		KCP: kcp,
		Machines: collections.FromMachines(
			&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m1"}, Status: clusterv1.MachineStatus{V1Beta2: &clusterv1.MachineV1Beta2Status{Conditions: []metav1.Condition{readyTrue, availableTrue, upToDateTrue}}}},
			&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m2"}, Status: clusterv1.MachineStatus{V1Beta2: &clusterv1.MachineV1Beta2Status{Conditions: []metav1.Condition{readyTrue, availableTrue, upToDateTrue}}}},
			&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m3"}, Status: clusterv1.MachineStatus{V1Beta2: &clusterv1.MachineV1Beta2Status{Conditions: []metav1.Condition{readyFalse, availableFalse, upToDateTrue}}}},
			&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m4"}, Status: clusterv1.MachineStatus{V1Beta2: &clusterv1.MachineV1Beta2Status{Conditions: []metav1.Condition{readyTrue, availableFalse, upToDateTrue}}}},
			&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m5"}, Status: clusterv1.MachineStatus{V1Beta2: &clusterv1.MachineV1Beta2Status{Conditions: []metav1.Condition{readyFalse, availableFalse, upToDateFalse}}}},
			&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m6"}, Status: clusterv1.MachineStatus{V1Beta2: &clusterv1.MachineV1Beta2Status{Conditions: []metav1.Condition{readyUnknown, availableUnknown, upToDateUnknown}}}},
		),
	}

	setReplicas(ctx, c.KCP, c.Machines)

	g.Expect(kcp.Status.V1Beta2).ToNot(BeNil())
	g.Expect(kcp.Status.V1Beta2.ReadyReplicas).ToNot(BeNil())
	g.Expect(*kcp.Status.V1Beta2.ReadyReplicas).To(Equal(int32(3)))
	g.Expect(kcp.Status.V1Beta2.AvailableReplicas).ToNot(BeNil())
	g.Expect(*kcp.Status.V1Beta2.AvailableReplicas).To(Equal(int32(2)))
	g.Expect(kcp.Status.V1Beta2.UpToDateReplicas).ToNot(BeNil())
	g.Expect(*kcp.Status.V1Beta2.UpToDateReplicas).To(Equal(int32(4)))
}

func Test_setInitializedCondition(t *testing.T) {
	tests := []struct {
		name            string
		controlPlane    *internal.ControlPlane
		expectCondition metav1.Condition
	}{
		{
			name: "KCP not initialized",
			controlPlane: &internal.ControlPlane{
				KCP: &controlplanev1.KubeadmControlPlane{},
			},
			expectCondition: metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneInitializedV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: controlplanev1.KubeadmControlPlaneNotInitializedV1Beta2Reason,
			},
		},
		{
			name: "KCP initialized",
			controlPlane: &internal.ControlPlane{
				KCP: &controlplanev1.KubeadmControlPlane{
					Status: controlplanev1.KubeadmControlPlaneStatus{Initialized: true},
				},
			},
			expectCondition: metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneInitializedV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: controlplanev1.KubeadmControlPlaneInitializedV1Beta2Reason,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			setInitializedCondition(ctx, tt.controlPlane.KCP)

			condition := v1beta2conditions.Get(tt.controlPlane.KCP, controlplanev1.KubeadmControlPlaneInitializedV1Beta2Condition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(v1beta2conditions.MatchCondition(tt.expectCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func Test_setScalingUpCondition(t *testing.T) {
	tests := []struct {
		name            string
		controlPlane    *internal.ControlPlane
		expectCondition metav1.Condition
	}{
		{
			name: "Replica not set",
			controlPlane: &internal.ControlPlane{
				KCP: &controlplanev1.KubeadmControlPlane{},
			},
			expectCondition: metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneScalingUpV1Beta2Condition,
				Status: metav1.ConditionUnknown,
				Reason: controlplanev1.KubeadmControlPlaneScalingUpWaitingForReplicasSetV1Beta2Reason,
			},
		},
		{
			name: "Not scaling up",
			controlPlane: &internal.ControlPlane{
				KCP: &controlplanev1.KubeadmControlPlane{
					Spec:   controlplanev1.KubeadmControlPlaneSpec{Replicas: ptr.To(int32(3))},
					Status: controlplanev1.KubeadmControlPlaneStatus{Replicas: 3},
				},
				Machines: collections.FromMachines(
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m1"}},
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m2"}},
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m3"}},
				),
			},
			expectCondition: metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneScalingUpV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: controlplanev1.KubeadmControlPlaneNotScalingUpV1Beta2Reason,
			},
		},
		{
			name: "Not scaling up, infra template not found",
			controlPlane: &internal.ControlPlane{
				KCP: &controlplanev1.KubeadmControlPlane{
					Spec:   controlplanev1.KubeadmControlPlaneSpec{Replicas: ptr.To(int32(3)), MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{InfrastructureRef: corev1.ObjectReference{Kind: "AWSTemplate"}}},
					Status: controlplanev1.KubeadmControlPlaneStatus{Replicas: 3},
				},
				Machines: collections.FromMachines(
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m1"}},
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m2"}},
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m3"}},
				),
				InfraMachineTemplateIsNotFound: true,
			},
			expectCondition: metav1.Condition{
				Type:    controlplanev1.KubeadmControlPlaneScalingUpV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneNotScalingUpV1Beta2Reason,
				Message: "Scaling up would be blocked because AWSTemplate does not exist",
			},
		},
		{
			name: "Scaling up",
			controlPlane: &internal.ControlPlane{
				KCP: &controlplanev1.KubeadmControlPlane{
					Spec:   controlplanev1.KubeadmControlPlaneSpec{Replicas: ptr.To(int32(5))},
					Status: controlplanev1.KubeadmControlPlaneStatus{Replicas: 3},
				},
				Machines: collections.FromMachines(
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m1"}},
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m2"}},
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m3"}},
				),
			},
			expectCondition: metav1.Condition{
				Type:    controlplanev1.KubeadmControlPlaneScalingUpV1Beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  controlplanev1.KubeadmControlPlaneScalingUpV1Beta2Reason,
				Message: "Scaling up from 3 to 5 replicas",
			},
		},
		{
			name: "Scaling up is always false when kcp is deleted",
			controlPlane: &internal.ControlPlane{
				KCP: &controlplanev1.KubeadmControlPlane{
					ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: ptr.To(metav1.Time{Time: time.Now()})},
					Spec:       controlplanev1.KubeadmControlPlaneSpec{Replicas: ptr.To(int32(5))},
					Status:     controlplanev1.KubeadmControlPlaneStatus{Replicas: 3},
				},
				Machines: collections.FromMachines(
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m1"}},
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m2"}},
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m3"}},
				),
			},
			expectCondition: metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneScalingUpV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: controlplanev1.KubeadmControlPlaneNotScalingUpV1Beta2Reason,
			},
		},
		{
			name: "Scaling up, infra template not found",
			controlPlane: &internal.ControlPlane{
				KCP: &controlplanev1.KubeadmControlPlane{
					Spec:   controlplanev1.KubeadmControlPlaneSpec{Replicas: ptr.To(int32(5)), MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{InfrastructureRef: corev1.ObjectReference{Kind: "AWSTemplate"}}},
					Status: controlplanev1.KubeadmControlPlaneStatus{Replicas: 3},
				},
				Machines: collections.FromMachines(
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m1"}},
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m2"}},
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m3"}},
				),
				InfraMachineTemplateIsNotFound: true,
			},
			expectCondition: metav1.Condition{
				Type:    controlplanev1.KubeadmControlPlaneScalingUpV1Beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  controlplanev1.KubeadmControlPlaneScalingUpV1Beta2Reason,
				Message: "Scaling up from 3 to 5 replicas is blocked because AWSTemplate does not exist",
			},
		},
		{
			name: "Scaling up, preflight checks blocking",
			controlPlane: &internal.ControlPlane{
				KCP: &controlplanev1.KubeadmControlPlane{
					Spec:   controlplanev1.KubeadmControlPlaneSpec{Replicas: ptr.To(int32(5))},
					Status: controlplanev1.KubeadmControlPlaneStatus{Replicas: 3},
				},
				Machines: collections.FromMachines(
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m1"}},
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m2"}},
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m3"}},
				),
				PreflightCheckResults: internal.PreflightCheckResults{
					HasDeletingMachine:               true,
					ControlPlaneComponentsNotHealthy: true,
					EtcdClusterNotHealthy:            true,
				},
			},
			expectCondition: metav1.Condition{
				Type:    controlplanev1.KubeadmControlPlaneScalingUpV1Beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  controlplanev1.KubeadmControlPlaneScalingUpV1Beta2Reason,
				Message: "Scaling up from 3 to 5 replicas; waiting for Machine being deleted; waiting for control plane components to be healthy; waiting for etcd cluster to be healthy",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			setScalingUpCondition(ctx, tt.controlPlane.KCP, tt.controlPlane.Machines, tt.controlPlane.InfraMachineTemplateIsNotFound, tt.controlPlane.PreflightCheckResults)

			condition := v1beta2conditions.Get(tt.controlPlane.KCP, controlplanev1.KubeadmControlPlaneScalingUpV1Beta2Condition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(v1beta2conditions.MatchCondition(tt.expectCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func Test_setScalingDownCondition(t *testing.T) {
	tests := []struct {
		name            string
		controlPlane    *internal.ControlPlane
		expectCondition metav1.Condition
	}{
		{
			name: "Replica not set",
			controlPlane: &internal.ControlPlane{
				KCP: &controlplanev1.KubeadmControlPlane{},
			},
			expectCondition: metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneScalingDownV1Beta2Condition,
				Status: metav1.ConditionUnknown,
				Reason: controlplanev1.KubeadmControlPlaneScalingDownWaitingForReplicasSetV1Beta2Reason,
			},
		},
		{
			name: "Not scaling down",
			controlPlane: &internal.ControlPlane{
				KCP: &controlplanev1.KubeadmControlPlane{
					Spec:   controlplanev1.KubeadmControlPlaneSpec{Replicas: ptr.To(int32(3))},
					Status: controlplanev1.KubeadmControlPlaneStatus{Replicas: 3},
				},
				Machines: collections.FromMachines(
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m1"}},
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m2"}},
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m3"}},
				),
			},
			expectCondition: metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneScalingDownV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: controlplanev1.KubeadmControlPlaneNotScalingDownV1Beta2Reason,
			},
		},
		{
			name: "Scaling down",
			controlPlane: &internal.ControlPlane{
				KCP: &controlplanev1.KubeadmControlPlane{
					Spec:   controlplanev1.KubeadmControlPlaneSpec{Replicas: ptr.To(int32(3))},
					Status: controlplanev1.KubeadmControlPlaneStatus{Replicas: 5},
				},
				Machines: collections.FromMachines(
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m1"}},
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m2"}},
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m3"}},
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m4"}},
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m5"}},
				),
			},
			expectCondition: metav1.Condition{
				Type:    controlplanev1.KubeadmControlPlaneScalingDownV1Beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  controlplanev1.KubeadmControlPlaneScalingDownV1Beta2Reason,
				Message: "Scaling down from 5 to 3 replicas",
			},
		},
		{
			name: "Scaling down to zero when kcp is deleted",
			controlPlane: &internal.ControlPlane{
				KCP: &controlplanev1.KubeadmControlPlane{
					ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: ptr.To(metav1.Time{Time: time.Now()})},
					Spec:       controlplanev1.KubeadmControlPlaneSpec{Replicas: ptr.To(int32(3))},
					Status:     controlplanev1.KubeadmControlPlaneStatus{Replicas: 5},
				},
				Machines: collections.FromMachines(
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m1"}},
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m2"}},
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m3"}},
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m4"}},
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m5"}},
				),
			},
			expectCondition: metav1.Condition{
				Type:    controlplanev1.KubeadmControlPlaneScalingDownV1Beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  controlplanev1.KubeadmControlPlaneScalingDownV1Beta2Reason,
				Message: "Scaling down from 5 to 0 replicas",
			},
		},
		{
			name: "Scaling down with one stale machine",
			controlPlane: &internal.ControlPlane{
				KCP: &controlplanev1.KubeadmControlPlane{
					Spec:   controlplanev1.KubeadmControlPlaneSpec{Replicas: ptr.To(int32(1))},
					Status: controlplanev1.KubeadmControlPlaneStatus{Replicas: 3},
				},
				Machines: collections.FromMachines(
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m1", DeletionTimestamp: ptr.To(metav1.Time{Time: time.Now().Add(-1 * time.Hour)})}},
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m2"}},
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m3"}},
				),
			},
			expectCondition: metav1.Condition{
				Type:    controlplanev1.KubeadmControlPlaneScalingDownV1Beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  controlplanev1.KubeadmControlPlaneScalingDownV1Beta2Reason,
				Message: "Scaling down from 3 to 1 replicas; Machine m1 is in deletion since more than 30m",
			},
		},
		{
			name: "Scaling down with two stale machine",
			controlPlane: &internal.ControlPlane{
				KCP: &controlplanev1.KubeadmControlPlane{
					Spec:   controlplanev1.KubeadmControlPlaneSpec{Replicas: ptr.To(int32(1))},
					Status: controlplanev1.KubeadmControlPlaneStatus{Replicas: 3},
				},
				Machines: collections.FromMachines(
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m1", DeletionTimestamp: ptr.To(metav1.Time{Time: time.Now().Add(-1 * time.Hour)})}},
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m2", DeletionTimestamp: ptr.To(metav1.Time{Time: time.Now().Add(-1 * time.Hour)})}},
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m3"}},
				),
			},
			expectCondition: metav1.Condition{
				Type:    controlplanev1.KubeadmControlPlaneScalingDownV1Beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  controlplanev1.KubeadmControlPlaneScalingDownV1Beta2Reason,
				Message: "Scaling down from 3 to 1 replicas; Machines m1, m2 are in deletion since more than 30m",
			},
		},
		{
			name: "Scaling down, preflight checks blocking",
			controlPlane: &internal.ControlPlane{
				KCP: &controlplanev1.KubeadmControlPlane{
					Spec:   controlplanev1.KubeadmControlPlaneSpec{Replicas: ptr.To(int32(1))},
					Status: controlplanev1.KubeadmControlPlaneStatus{Replicas: 3},
				},
				Machines: collections.FromMachines(
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m1"}},
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m2"}},
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m3"}},
				),
				PreflightCheckResults: internal.PreflightCheckResults{
					HasDeletingMachine:               true,
					ControlPlaneComponentsNotHealthy: true,
					EtcdClusterNotHealthy:            true,
				},
			},
			expectCondition: metav1.Condition{
				Type:    controlplanev1.KubeadmControlPlaneScalingDownV1Beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  controlplanev1.KubeadmControlPlaneScalingDownV1Beta2Reason,
				Message: "Scaling down from 3 to 1 replicas; waiting for Machine being deleted; waiting for control plane components to be healthy; waiting for etcd cluster to be healthy",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			setScalingDownCondition(ctx, tt.controlPlane.KCP, tt.controlPlane.Machines, tt.controlPlane.PreflightCheckResults)

			condition := v1beta2conditions.Get(tt.controlPlane.KCP, controlplanev1.KubeadmControlPlaneScalingDownV1Beta2Condition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(v1beta2conditions.MatchCondition(tt.expectCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func Test_setMachinesReadyAndMachinesUpToDate(t *testing.T) {
	readyTrue := metav1.Condition{Type: clusterv1.MachineReadyV1Beta2Condition, Status: metav1.ConditionTrue}
	readyFalse := metav1.Condition{Type: clusterv1.MachineReadyV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "SomeReason", Message: "NotReady"}

	upToDateTrue := metav1.Condition{Type: clusterv1.MachineUpToDateV1Beta2Condition, Status: metav1.ConditionTrue}
	upToDateFalse := metav1.Condition{Type: clusterv1.MachineUpToDateV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "SomeReason", Message: "NotUpToDate"}

	tests := []struct {
		name                            string
		controlPlane                    *internal.ControlPlane
		expectMachinesReadyCondition    metav1.Condition
		expectMachinesUpToDateCondition metav1.Condition
	}{
		{
			name: "Without machines",
			controlPlane: &internal.ControlPlane{
				KCP:      &controlplanev1.KubeadmControlPlane{},
				Machines: collections.FromMachines(),
			},
			expectMachinesReadyCondition: metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneMachinesReadyV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: controlplanev1.KubeadmControlPlaneMachinesReadyNoReplicasV1Beta2Reason,
			},
			expectMachinesUpToDateCondition: metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneMachinesUpToDateV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: controlplanev1.KubeadmControlPlaneMachinesUpToDateNoReplicasV1Beta2Reason,
			},
		},
		{
			name: "With machines",
			controlPlane: &internal.ControlPlane{
				KCP: &controlplanev1.KubeadmControlPlane{},
				Machines: collections.FromMachines(
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m1"}, Status: clusterv1.MachineStatus{V1Beta2: &clusterv1.MachineV1Beta2Status{Conditions: []metav1.Condition{readyTrue, upToDateTrue}}}},
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m2"}, Status: clusterv1.MachineStatus{V1Beta2: &clusterv1.MachineV1Beta2Status{Conditions: []metav1.Condition{readyTrue, upToDateFalse}}}},
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m3"}, Status: clusterv1.MachineStatus{V1Beta2: &clusterv1.MachineV1Beta2Status{Conditions: []metav1.Condition{readyFalse, upToDateFalse}}}},
				),
			},
			expectMachinesReadyCondition: metav1.Condition{
				Type:    controlplanev1.KubeadmControlPlaneMachinesReadyV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  "SomeReason", // There is only one machine reporting issues, using the reason from that machine.
				Message: "NotReady from Machine m3",
			},
			expectMachinesUpToDateCondition: metav1.Condition{
				Type:    controlplanev1.KubeadmControlPlaneMachinesUpToDateV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  v1beta2conditions.MultipleIssuesReportedReason, // There are many machines reporting issues, using a generic reason.
				Message: "NotUpToDate from Machines m2, m3",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			setMachinesReadyCondition(ctx, tt.controlPlane.KCP, tt.controlPlane.Machines)
			setMachinesUpToDateCondition(ctx, tt.controlPlane.KCP, tt.controlPlane.Machines)

			readyCondition := v1beta2conditions.Get(tt.controlPlane.KCP, controlplanev1.KubeadmControlPlaneMachinesReadyV1Beta2Condition)
			g.Expect(readyCondition).ToNot(BeNil())
			g.Expect(*readyCondition).To(v1beta2conditions.MatchCondition(tt.expectMachinesReadyCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))

			upToDateCondition := v1beta2conditions.Get(tt.controlPlane.KCP, controlplanev1.KubeadmControlPlaneMachinesUpToDateV1Beta2Condition)
			g.Expect(upToDateCondition).ToNot(BeNil())
			g.Expect(*upToDateCondition).To(v1beta2conditions.MatchCondition(tt.expectMachinesUpToDateCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func Test_setRemediatingCondition(t *testing.T) {
	healthCheckSucceeded := clusterv1.Condition{Type: clusterv1.MachineHealthCheckSucceededV1Beta2Condition, Status: corev1.ConditionTrue}
	healthCheckNotSucceeded := clusterv1.Condition{Type: clusterv1.MachineHealthCheckSucceededV1Beta2Condition, Status: corev1.ConditionFalse}
	ownerRemediated := clusterv1.Condition{Type: clusterv1.MachineOwnerRemediatedCondition, Status: corev1.ConditionFalse}
	ownerRemediatedV1Beta2 := metav1.Condition{Type: clusterv1.MachineOwnerRemediatedV1Beta2Condition, Status: metav1.ConditionFalse, Message: "Remediation in progress"}

	tests := []struct {
		name            string
		controlPlane    *internal.ControlPlane
		expectCondition metav1.Condition
	}{
		{
			name: "Without unhealthy machines",
			controlPlane: &internal.ControlPlane{
				KCP: &controlplanev1.KubeadmControlPlane{},
				Machines: collections.FromMachines(
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m1"}},
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m2"}},
				),
			},
			expectCondition: metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneRemediatingV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: controlplanev1.KubeadmControlPlaneNotRemediatingV1Beta2Reason,
			},
		},
		{
			name: "With machines to be remediated by KCP",
			controlPlane: &internal.ControlPlane{
				KCP: &controlplanev1.KubeadmControlPlane{},
				Machines: collections.FromMachines(
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m1"}, Status: clusterv1.MachineStatus{Conditions: clusterv1.Conditions{healthCheckSucceeded}}},    // Healthy machine
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m2"}, Status: clusterv1.MachineStatus{Conditions: clusterv1.Conditions{healthCheckNotSucceeded}}}, // Unhealthy machine, not yet marked for remediation
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m3"}, Status: clusterv1.MachineStatus{Conditions: clusterv1.Conditions{healthCheckNotSucceeded, ownerRemediated}, V1Beta2: &clusterv1.MachineV1Beta2Status{Conditions: []metav1.Condition{ownerRemediatedV1Beta2}}}},
				),
			},
			expectCondition: metav1.Condition{
				Type:    controlplanev1.KubeadmControlPlaneRemediatingV1Beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  controlplanev1.KubeadmControlPlaneRemediatingV1Beta2Reason,
				Message: "Remediation in progress from Machine m3",
			},
		},
		{
			name: "With one unhealthy machine not to be remediated by KCP",
			controlPlane: &internal.ControlPlane{
				KCP: &controlplanev1.KubeadmControlPlane{},
				Machines: collections.FromMachines(
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m1"}, Status: clusterv1.MachineStatus{Conditions: clusterv1.Conditions{healthCheckSucceeded}}},    // Healthy machine
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m2"}, Status: clusterv1.MachineStatus{Conditions: clusterv1.Conditions{healthCheckNotSucceeded}}}, // Unhealthy machine, not yet marked for remediation
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m3"}, Status: clusterv1.MachineStatus{Conditions: clusterv1.Conditions{healthCheckSucceeded}}},    // Healthy machine
				),
			},
			expectCondition: metav1.Condition{
				Type:    controlplanev1.KubeadmControlPlaneRemediatingV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneNotRemediatingV1Beta2Reason,
				Message: "Machine m2 is not healthy (not to be remediated by KCP)",
			},
		},
		{
			name: "With two unhealthy machine not to be remediated by KCP",
			controlPlane: &internal.ControlPlane{
				KCP: &controlplanev1.KubeadmControlPlane{},
				Machines: collections.FromMachines(
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m1"}, Status: clusterv1.MachineStatus{Conditions: clusterv1.Conditions{healthCheckNotSucceeded}}}, // Unhealthy machine, not yet marked for remediation
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m2"}, Status: clusterv1.MachineStatus{Conditions: clusterv1.Conditions{healthCheckNotSucceeded}}}, // Unhealthy machine, not yet marked for remediation
					&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m3"}, Status: clusterv1.MachineStatus{Conditions: clusterv1.Conditions{healthCheckSucceeded}}},    // Healthy machine
				),
			},
			expectCondition: metav1.Condition{
				Type:    controlplanev1.KubeadmControlPlaneRemediatingV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneNotRemediatingV1Beta2Reason,
				Message: "Machines m1, m2 are not healthy (not to be remediated by KCP)",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			setRemediatingCondition(ctx, tt.controlPlane.KCP, tt.controlPlane.MachinesToBeRemediatedByKCP(), tt.controlPlane.UnhealthyMachines())

			condition := v1beta2conditions.Get(tt.controlPlane.KCP, controlplanev1.KubeadmControlPlaneRemediatingV1Beta2Condition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(v1beta2conditions.MatchCondition(tt.expectCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func TestKubeadmControlPlaneReconciler_updateStatusNoMachines(t *testing.T) {
	g := NewWithT(t)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: metav1.NamespaceDefault,
		},
	}

	kcp := &controlplanev1.KubeadmControlPlane{
		TypeMeta: metav1.TypeMeta{
			Kind:       "KubeadmControlPlane",
			APIVersion: controlplanev1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cluster.Namespace,
			Name:      "foo",
		},
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			Version: "v1.16.6",
			MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					APIVersion: "test/v1alpha1",
					Kind:       "UnknownInfraMachine",
					Name:       "foo",
				},
			},
		},
	}
	webhook := &controlplanev1webhooks.KubeadmControlPlane{}
	g.Expect(webhook.Default(ctx, kcp)).To(Succeed())
	_, err := webhook.ValidateCreate(ctx, kcp)
	g.Expect(err).ToNot(HaveOccurred())

	fakeClient := newFakeClient(kcp.DeepCopy(), cluster.DeepCopy())

	r := &KubeadmControlPlaneReconciler{
		Client: fakeClient,
		managementCluster: &fakeManagementCluster{
			Machines: map[string]*clusterv1.Machine{},
			Workload: &fakeWorkloadCluster{},
		},
		recorder: record.NewFakeRecorder(32),
	}

	controlPlane := &internal.ControlPlane{
		KCP:     kcp,
		Cluster: cluster,
	}
	controlPlane.InjectTestManagementCluster(r.managementCluster)

	g.Expect(r.updateStatus(ctx, controlPlane)).To(Succeed())
	g.Expect(kcp.Status.Replicas).To(BeEquivalentTo(0))
	g.Expect(kcp.Status.ReadyReplicas).To(BeEquivalentTo(0))
	g.Expect(kcp.Status.UnavailableReplicas).To(BeEquivalentTo(0))
	g.Expect(kcp.Status.Initialized).To(BeFalse())
	g.Expect(kcp.Status.Ready).To(BeFalse())
	g.Expect(kcp.Status.Selector).NotTo(BeEmpty())
	g.Expect(kcp.Status.FailureMessage).To(BeNil())
	g.Expect(kcp.Status.FailureReason).To(BeEquivalentTo(""))
}

func TestKubeadmControlPlaneReconciler_updateStatusAllMachinesNotReady(t *testing.T) {
	g := NewWithT(t)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: metav1.NamespaceDefault,
		},
	}

	kcp := &controlplanev1.KubeadmControlPlane{
		TypeMeta: metav1.TypeMeta{
			Kind:       "KubeadmControlPlane",
			APIVersion: controlplanev1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cluster.Namespace,
			Name:      "foo",
		},
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			Version: "v1.16.6",
			MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					APIVersion: "test/v1alpha1",
					Kind:       "UnknownInfraMachine",
					Name:       "foo",
				},
			},
		},
	}
	webhook := &controlplanev1webhooks.KubeadmControlPlane{}
	g.Expect(webhook.Default(ctx, kcp)).To(Succeed())
	_, err := webhook.ValidateCreate(ctx, kcp)
	g.Expect(err).ToNot(HaveOccurred())

	machines := map[string]*clusterv1.Machine{}
	objs := []client.Object{cluster.DeepCopy(), kcp.DeepCopy()}
	for i := range 3 {
		name := fmt.Sprintf("test-%d", i)
		m, n := createMachineNodePair(name, cluster, kcp, false)
		objs = append(objs, n, m)
		machines[m.Name] = m
	}

	fakeClient := newFakeClient(objs...)

	r := &KubeadmControlPlaneReconciler{
		Client: fakeClient,
		managementCluster: &fakeManagementCluster{
			Machines: machines,
			Workload: &fakeWorkloadCluster{},
		},
		recorder: record.NewFakeRecorder(32),
	}

	controlPlane := &internal.ControlPlane{
		KCP:      kcp,
		Cluster:  cluster,
		Machines: machines,
	}
	controlPlane.InjectTestManagementCluster(r.managementCluster)

	g.Expect(r.updateStatus(ctx, controlPlane)).To(Succeed())
	g.Expect(kcp.Status.Replicas).To(BeEquivalentTo(3))
	g.Expect(kcp.Status.ReadyReplicas).To(BeEquivalentTo(0))
	g.Expect(kcp.Status.UnavailableReplicas).To(BeEquivalentTo(3))
	g.Expect(kcp.Status.Selector).NotTo(BeEmpty())
	g.Expect(kcp.Status.FailureMessage).To(BeNil())
	g.Expect(kcp.Status.FailureReason).To(BeEquivalentTo(""))
	g.Expect(kcp.Status.Initialized).To(BeFalse())
	g.Expect(kcp.Status.Ready).To(BeFalse())
}

func TestKubeadmControlPlaneReconciler_updateStatusAllMachinesReady(t *testing.T) {
	g := NewWithT(t)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceDefault,
			Name:      "foo",
		},
	}

	kcp := &controlplanev1.KubeadmControlPlane{
		TypeMeta: metav1.TypeMeta{
			Kind:       "KubeadmControlPlane",
			APIVersion: controlplanev1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cluster.Namespace,
			Name:      "foo",
		},
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			Version: "v1.16.6",
			MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					APIVersion: "test/v1alpha1",
					Kind:       "UnknownInfraMachine",
					Name:       "foo",
				},
			},
		},
	}
	webhook := &controlplanev1webhooks.KubeadmControlPlane{}
	g.Expect(webhook.Default(ctx, kcp)).To(Succeed())
	_, err := webhook.ValidateCreate(ctx, kcp)
	g.Expect(err).ToNot(HaveOccurred())

	objs := []client.Object{cluster.DeepCopy(), kcp.DeepCopy(), kubeadmConfigMap()}
	machines := map[string]*clusterv1.Machine{}
	for i := range 3 {
		name := fmt.Sprintf("test-%d", i)
		m, n := createMachineNodePair(name, cluster, kcp, true)
		objs = append(objs, n, m)
		machines[m.Name] = m
	}

	fakeClient := newFakeClient(objs...)

	r := &KubeadmControlPlaneReconciler{
		Client: fakeClient,
		managementCluster: &fakeManagementCluster{
			Machines: machines,
			Workload: &fakeWorkloadCluster{
				Status: internal.ClusterStatus{
					Nodes:            3,
					ReadyNodes:       3,
					HasKubeadmConfig: true,
				},
			},
		},
		recorder: record.NewFakeRecorder(32),
	}

	controlPlane := &internal.ControlPlane{
		KCP:      kcp,
		Cluster:  cluster,
		Machines: machines,
	}
	controlPlane.InjectTestManagementCluster(r.managementCluster)

	g.Expect(r.updateStatus(ctx, controlPlane)).To(Succeed())
	g.Expect(kcp.Status.Replicas).To(BeEquivalentTo(3))
	g.Expect(kcp.Status.ReadyReplicas).To(BeEquivalentTo(3))
	g.Expect(kcp.Status.UnavailableReplicas).To(BeEquivalentTo(0))
	g.Expect(kcp.Status.Selector).NotTo(BeEmpty())
	g.Expect(kcp.Status.FailureMessage).To(BeNil())
	g.Expect(kcp.Status.FailureReason).To(BeEquivalentTo(""))
	g.Expect(kcp.Status.Initialized).To(BeTrue())
	g.Expect(conditions.IsTrue(kcp, controlplanev1.AvailableCondition)).To(BeTrue())
	g.Expect(conditions.IsTrue(kcp, controlplanev1.MachinesCreatedCondition)).To(BeTrue())
	g.Expect(kcp.Status.Ready).To(BeTrue())
}

func TestKubeadmControlPlaneReconciler_updateStatusMachinesReadyMixed(t *testing.T) {
	g := NewWithT(t)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: metav1.NamespaceDefault,
		},
	}

	kcp := &controlplanev1.KubeadmControlPlane{
		TypeMeta: metav1.TypeMeta{
			Kind:       "KubeadmControlPlane",
			APIVersion: controlplanev1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cluster.Namespace,
			Name:      "foo",
		},
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			Version: "v1.16.6",
			MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					APIVersion: "test/v1alpha1",
					Kind:       "UnknownInfraMachine",
					Name:       "foo",
				},
			},
		},
	}
	webhook := &controlplanev1webhooks.KubeadmControlPlane{}
	g.Expect(webhook.Default(ctx, kcp)).To(Succeed())
	_, err := webhook.ValidateCreate(ctx, kcp)
	g.Expect(err).ToNot(HaveOccurred())
	machines := map[string]*clusterv1.Machine{}
	objs := []client.Object{cluster.DeepCopy(), kcp.DeepCopy()}
	for i := range 4 {
		name := fmt.Sprintf("test-%d", i)
		m, n := createMachineNodePair(name, cluster, kcp, false)
		machines[m.Name] = m
		objs = append(objs, n, m)
	}
	m, n := createMachineNodePair("testReady", cluster, kcp, true)
	objs = append(objs, n, m, kubeadmConfigMap())
	machines[m.Name] = m
	fakeClient := newFakeClient(objs...)

	r := &KubeadmControlPlaneReconciler{
		Client: fakeClient,
		managementCluster: &fakeManagementCluster{
			Machines: machines,
			Workload: &fakeWorkloadCluster{
				Status: internal.ClusterStatus{
					Nodes:            5,
					ReadyNodes:       1,
					HasKubeadmConfig: true,
				},
			},
		},
		recorder: record.NewFakeRecorder(32),
	}

	controlPlane := &internal.ControlPlane{
		KCP:      kcp,
		Cluster:  cluster,
		Machines: machines,
	}
	controlPlane.InjectTestManagementCluster(r.managementCluster)

	g.Expect(r.updateStatus(ctx, controlPlane)).To(Succeed())
	g.Expect(kcp.Status.Replicas).To(BeEquivalentTo(5))
	g.Expect(kcp.Status.ReadyReplicas).To(BeEquivalentTo(1))
	g.Expect(kcp.Status.UnavailableReplicas).To(BeEquivalentTo(4))
	g.Expect(kcp.Status.Selector).NotTo(BeEmpty())
	g.Expect(kcp.Status.FailureMessage).To(BeNil())
	g.Expect(kcp.Status.FailureReason).To(BeEquivalentTo(""))
	g.Expect(kcp.Status.Initialized).To(BeTrue())
	g.Expect(kcp.Status.Ready).To(BeTrue())
}

func TestKubeadmControlPlaneReconciler_machinesCreatedIsIsTrueEvenWhenTheNodesAreNotReady(t *testing.T) {
	g := NewWithT(t)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: metav1.NamespaceDefault,
		},
	}

	kcp := &controlplanev1.KubeadmControlPlane{
		TypeMeta: metav1.TypeMeta{
			Kind:       "KubeadmControlPlane",
			APIVersion: controlplanev1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cluster.Namespace,
			Name:      "foo",
		},
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			Version:  "v1.16.6",
			Replicas: ptr.To[int32](3),
			MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					APIVersion: "test/v1alpha1",
					Kind:       "UnknownInfraMachine",
					Name:       "foo",
				},
			},
		},
	}
	webhook := &controlplanev1webhooks.KubeadmControlPlane{}
	g.Expect(webhook.Default(ctx, kcp)).To(Succeed())
	_, err := webhook.ValidateCreate(ctx, kcp)
	g.Expect(err).ToNot(HaveOccurred())
	machines := map[string]*clusterv1.Machine{}
	objs := []client.Object{cluster.DeepCopy(), kcp.DeepCopy()}
	// Create the desired number of machines
	for i := range 3 {
		name := fmt.Sprintf("test-%d", i)
		m, n := createMachineNodePair(name, cluster, kcp, false)
		machines[m.Name] = m
		objs = append(objs, n, m)
	}

	fakeClient := newFakeClient(objs...)

	// Set all the machines to `not ready`
	r := &KubeadmControlPlaneReconciler{
		Client: fakeClient,
		managementCluster: &fakeManagementCluster{
			Machines: machines,
			Workload: &fakeWorkloadCluster{
				Status: internal.ClusterStatus{
					Nodes:            0,
					ReadyNodes:       0,
					HasKubeadmConfig: true,
				},
			},
		},
		recorder: record.NewFakeRecorder(32),
	}

	controlPlane := &internal.ControlPlane{
		KCP:      kcp,
		Cluster:  cluster,
		Machines: machines,
	}
	controlPlane.InjectTestManagementCluster(r.managementCluster)

	g.Expect(r.updateStatus(ctx, controlPlane)).To(Succeed())
	g.Expect(kcp.Status.Replicas).To(BeEquivalentTo(3))
	g.Expect(kcp.Status.ReadyReplicas).To(BeEquivalentTo(0))
	g.Expect(kcp.Status.UnavailableReplicas).To(BeEquivalentTo(3))
	g.Expect(kcp.Status.Ready).To(BeFalse())
	g.Expect(conditions.IsTrue(kcp, controlplanev1.MachinesCreatedCondition)).To(BeTrue())
}

func kubeadmConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kubeadm-config",
			Namespace: metav1.NamespaceSystem,
		},
	}
}
