/*
Copyright 2026 The Kubernetes Authors.

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

package machinepool

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/conditions"
)

func Test_setReplicas(t *testing.T) {
	t.Run("without MachinePool Machines versions are not surfaced", func(t *testing.T) {
		g := NewWithT(t)
		mp := &clusterv1.MachinePool{
			Spec: clusterv1.MachinePoolSpec{
				Replicas: ptr.To[int32](3),
			},
			Status: clusterv1.MachinePoolStatus{
				Replicas: ptr.To[int32](2),
			},
		}

		setReplicas(mp, false, nil, nil)

		g.Expect(mp.Status.ReadyReplicas).To(Equal(ptr.To[int32](2)))
		g.Expect(mp.Status.AvailableReplicas).To(Equal(ptr.To[int32](2)))
		g.Expect(mp.Status.UpToDateReplicas).To(Equal(ptr.To[int32](3)))
		g.Expect(mp.Status.Versions).To(BeNil())
	})

	t.Run("without MachinePool Machines versions are aggregated from node refs", func(t *testing.T) {
		g := NewWithT(t)
		mp := &clusterv1.MachinePool{
			Status: clusterv1.MachinePoolStatus{
				NodeRefs: []corev1.ObjectReference{
					{Name: "node-1"},
					{Name: "node-2"},
					{Name: "node-3"},
				},
			},
		}
		nodeRefMap := map[string]*corev1.Node{
			"provider-id-1": {
				ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
				Status:     corev1.NodeStatus{NodeInfo: corev1.NodeSystemInfo{KubeletVersion: "v1.31.1"}},
			},
			"provider-id-2": {
				ObjectMeta: metav1.ObjectMeta{Name: "node-2"},
				Status:     corev1.NodeStatus{NodeInfo: corev1.NodeSystemInfo{KubeletVersion: "v1.31.1"}},
			},
			"provider-id-3": {
				ObjectMeta: metav1.ObjectMeta{Name: "node-3"},
				Status:     corev1.NodeStatus{NodeInfo: corev1.NodeSystemInfo{KubeletVersion: "v1.32.0"}},
			},
		}

		setReplicas(mp, false, nil, nodeRefMap)

		g.Expect(mp.Status.Versions).To(Equal([]clusterv1.StatusVersion{
			{Version: "v1.31.1", Replicas: 2},
			{Version: "v1.32.0", Replicas: 1},
		}))
	})

	t.Run("with MachinePool Machines versions are aggregated from machine spec versions", func(t *testing.T) {
		g := NewWithT(t)
		mp := &clusterv1.MachinePool{}
		machines := []*clusterv1.Machine{
			{
				Spec: clusterv1.MachineSpec{
					Version: "v1.31.1",
				},
			},
			{
				Spec: clusterv1.MachineSpec{
					Version: "v1.31.1",
				},
			},
			{
				Spec: clusterv1.MachineSpec{
					Version: "v1.32.0",
				},
			},
		}

		setReplicas(mp, true, machines, nil)

		g.Expect(mp.Status.Versions).To(Equal([]clusterv1.StatusVersion{
			{Version: "v1.31.1", Replicas: 2},
			{Version: "v1.32.0", Replicas: 1},
		}))
	})
}

func Test_updateStatus_machineListFailurePreservesCounters(t *testing.T) {
	g := NewWithT(t)
	mp := &clusterv1.MachinePool{
		Status: clusterv1.MachinePoolStatus{
			ReadyReplicas:     ptr.To[int32](2),
			AvailableReplicas: ptr.To[int32](2),
			UpToDateReplicas:  ptr.To[int32](1),
		},
	}
	infraMachinePool := &unstructured.Unstructured{Object: map[string]interface{}{
		"status": map[string]interface{}{
			"infrastructureMachineKind": "GenericInfrastructureMachine",
		},
	}}

	r := &Reconciler{}
	err := r.updateStatus(ctx, &scope{
		machinePool:      mp,
		infraMachinePool: infraMachinePool,
	})
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(mp.Status.ReadyReplicas).To(Equal(ptr.To[int32](2)))
	g.Expect(mp.Status.AvailableReplicas).To(Equal(ptr.To[int32](2)))
	g.Expect(mp.Status.UpToDateReplicas).To(Equal(ptr.To[int32](1)))
	condition := conditions.Get(mp, clusterv1.MachinePoolMachinesUpToDateCondition)
	g.Expect(condition).ToNot(BeNil())
	g.Expect(condition.Status).To(Equal(metav1.ConditionUnknown))
	g.Expect(condition.Reason).To(Equal(clusterv1.MachinePoolMachinesUpToDateInternalErrorReason))
}

func Test_updateStatus_knownMachinelessPoolRemovesMachinesUpToDateCondition(t *testing.T) {
	g := NewWithT(t)
	mp := &clusterv1.MachinePool{}
	conditions.Set(mp, metav1.Condition{
		Type:   clusterv1.MachinePoolMachinesUpToDateCondition,
		Status: metav1.ConditionTrue,
		Reason: "PreviouslyReported",
	})

	r := &Reconciler{}
	err := r.updateStatus(ctx, &scope{
		machinePool:                        mp,
		infraMachinePool:                   &unstructured.Unstructured{},
		getMachinesForMachinePoolSucceeded: true,
	})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(conditions.Get(mp, clusterv1.MachinePoolMachinesUpToDateCondition)).To(BeNil())
}

func Test_updateStatus_unknownMachinePoolMachinesStateReportsUnknown(t *testing.T) {
	g := NewWithT(t)
	mp := &clusterv1.MachinePool{}

	r := &Reconciler{}
	err := r.updateStatus(ctx, &scope{
		machinePool:                        mp,
		getMachinesForMachinePoolSucceeded: true,
	})
	g.Expect(err).ToNot(HaveOccurred())

	condition := conditions.Get(mp, clusterv1.MachinePoolMachinesUpToDateCondition)
	g.Expect(condition).ToNot(BeNil())
	g.Expect(condition.Status).To(Equal(metav1.ConditionUnknown))
	g.Expect(condition.Reason).To(Equal(clusterv1.MachinePoolMachineSupportUnknownReason))
	g.Expect(condition.Message).To(Equal("Machine support could not be determined"))
}

func Test_updateStatus_existingMachinesProveMachineBacked(t *testing.T) {
	g := NewWithT(t)
	mp := &clusterv1.MachinePool{}
	machine := machinePoolTestMachine("machine-1", metav1.Condition{
		Type:   clusterv1.MachineUpToDateCondition,
		Status: metav1.ConditionTrue,
		Reason: clusterv1.MachineUpToDateReason,
	})

	r := &Reconciler{}
	err := r.updateStatus(ctx, &scope{
		machinePool:                        mp,
		machines:                           []*clusterv1.Machine{machine},
		getMachinesForMachinePoolSucceeded: true,
	})
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(mp.Status.UpToDateReplicas).To(Equal(ptr.To[int32](1)))
	condition := conditions.Get(mp, clusterv1.MachinePoolMachinesUpToDateCondition)
	g.Expect(condition).ToNot(BeNil())
	g.Expect(condition.Status).To(Equal(metav1.ConditionTrue))
	g.Expect(condition.Reason).To(Equal(clusterv1.MachinePoolMachinesUpToDateReason))
}

func Test_updateStatus_malformedInfrastructureMachineKind(t *testing.T) {
	g := NewWithT(t)
	mp := &clusterv1.MachinePool{}
	infraMachinePool := &unstructured.Unstructured{Object: map[string]interface{}{
		"status": map[string]interface{}{
			"infrastructureMachineKind": int64(1),
		},
	}}

	r := &Reconciler{}
	err := r.updateStatus(ctx, &scope{
		machinePool:                        mp,
		infraMachinePool:                   infraMachinePool,
		getMachinesForMachinePoolSucceeded: true,
	})
	g.Expect(err).To(HaveOccurred())

	condition := conditions.Get(mp, clusterv1.MachinePoolMachinesUpToDateCondition)
	g.Expect(condition).ToNot(BeNil())
	g.Expect(condition.Status).To(Equal(metav1.ConditionUnknown))
	g.Expect(condition.Reason).To(Equal(clusterv1.MachinePoolMachinesUpToDateInternalErrorReason))
}

func Test_setMachinesUpToDateConditionDeletingWithoutMachineObservation(t *testing.T) {
	g := NewWithT(t)
	deletionTimestamp := metav1.Now()
	mp := &clusterv1.MachinePool{
		ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &deletionTimestamp, Finalizers: []string{"test"}},
	}
	conditions.Set(mp, metav1.Condition{
		Type:   clusterv1.MachinePoolMachinesUpToDateCondition,
		Status: metav1.ConditionTrue,
		Reason: clusterv1.MachinePoolMachinesUpToDateReason,
	})

	setMachinesUpToDateCondition(ctx, mp, nil, machinePoolMachinesStateUnknown, nil, false)

	condition := conditions.Get(mp, clusterv1.MachinePoolMachinesUpToDateCondition)
	g.Expect(condition).ToNot(BeNil())
	g.Expect(condition.Status).To(Equal(metav1.ConditionTrue))
	g.Expect(condition.Reason).To(Equal(clusterv1.MachinePoolMachinesUpToDateReason))
}

func Test_setMachinesUpToDateCondition(t *testing.T) {
	upToDateCondition := metav1.Condition{
		Type:   clusterv1.MachineUpToDateCondition,
		Status: metav1.ConditionTrue,
		Reason: clusterv1.MachineUpToDateReason,
	}

	tests := []struct {
		name                     string
		machines                 []*clusterv1.Machine
		machinePoolMachinesState machinePoolMachinesState
		getMachinesSucceeded     bool
		initialCondition         *metav1.Condition
		expectSet                bool
		expectCondition          metav1.Condition
	}{
		{
			name:                     "known machineless pool removes a stale condition",
			machinePoolMachinesState: machinePoolMachinesStateNotSupported,
			getMachinesSucceeded:     true,
			initialCondition: &metav1.Condition{
				Type:   clusterv1.MachinePoolMachinesUpToDateCondition,
				Status: metav1.ConditionTrue,
				Reason: "PreviouslyReported",
			},
			expectSet: false,
		},
		{
			name:                     "unknown Machine support",
			machinePoolMachinesState: machinePoolMachinesStateUnknown,
			getMachinesSucceeded:     true,
			expectSet:                true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolMachinesUpToDateCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachinePoolMachineSupportUnknownReason,
				Message: "Machine support could not be determined",
			},
		},
		{
			name:                     "Machine list failure",
			machinePoolMachinesState: machinePoolMachinesStateSupported,
			getMachinesSucceeded:     false,
			expectSet:                true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolMachinesUpToDateCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachinePoolMachinesUpToDateInternalErrorReason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:                     "no Machines",
			machinePoolMachinesState: machinePoolMachinesStateSupported,
			getMachinesSucceeded:     true,
			expectSet:                true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachinePoolMachinesUpToDateCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachinePoolMachinesUpToDateNoReplicasReason,
			},
		},
		{
			name: "recently created Machine without condition is ignored",
			machines: []*clusterv1.Machine{{
				ObjectMeta: metav1.ObjectMeta{Name: "machine-new", CreationTimestamp: metav1.NewTime(time.Now())},
			}},
			machinePoolMachinesState: machinePoolMachinesStateSupported,
			getMachinesSucceeded:     true,
			expectSet:                true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachinePoolMachinesUpToDateCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachinePoolMachinesUpToDateNoReplicasReason,
			},
		},
		{
			name: "all Machines up to date",
			machines: []*clusterv1.Machine{
				machinePoolTestMachine("machine-1", upToDateCondition),
			},
			machinePoolMachinesState: machinePoolMachinesStateSupported,
			getMachinesSucceeded:     true,
			expectSet:                true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachinePoolMachinesUpToDateCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachinePoolMachinesUpToDateReason,
			},
		},
		{
			name: "one Machine not up to date",
			machines: []*clusterv1.Machine{
				machinePoolTestMachine("machine-1", metav1.Condition{
					Type:    clusterv1.MachineUpToDateCondition,
					Status:  metav1.ConditionFalse,
					Reason:  "VersionNotUpToDate",
					Message: "Version is not up to date",
				}),
			},
			machinePoolMachinesState: machinePoolMachinesStateSupported,
			getMachinesSucceeded:     true,
			expectSet:                true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolMachinesUpToDateCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachinePoolMachinesNotUpToDateReason,
				Message: "* Machine machine-1: Version is not up to date",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			mp := &clusterv1.MachinePool{}
			if tt.initialCondition != nil {
				conditions.Set(mp, *tt.initialCondition)
			}

			setMachinesUpToDateCondition(ctx, mp, tt.machines, tt.machinePoolMachinesState, nil, tt.getMachinesSucceeded)

			condition := conditions.Get(mp, clusterv1.MachinePoolMachinesUpToDateCondition)
			if !tt.expectSet {
				g.Expect(condition).To(BeNil())
				return
			}
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tt.expectCondition, conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func machinePoolTestMachine(name string, machineConditions ...metav1.Condition) *clusterv1.Machine {
	machine := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: name}}
	for _, condition := range machineConditions {
		conditions.Set(machine, condition)
	}
	return machine
}
