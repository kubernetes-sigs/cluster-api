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
	t.Run("without MachinePool Machines counters are derived from Nodes", func(t *testing.T) {
		g := NewWithT(t)
		mp := &clusterv1.MachinePool{
			Spec: clusterv1.MachinePoolSpec{
				Replicas:       ptr.To[int32](3),
				ProviderIDList: []string{"provider-id-1", "provider-id-2", "provider-id-3"},
				Template: clusterv1.MachineTemplateSpec{
					Spec: clusterv1.MachineSpec{MinReadySeconds: ptr.To[int32](30)},
				},
			},
			Status: clusterv1.MachinePoolStatus{
				Replicas: ptr.To[int32](3),
			},
		}
		nodeRefMap := map[string]*corev1.Node{
			"provider-id-1": readyNode("node-1", time.Now().Add(-time.Minute)),
			"provider-id-2": readyNode("node-2", time.Now()),
			"provider-id-3": {
				ObjectMeta: metav1.ObjectMeta{Name: "node-3"},
				Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionFalse,
				}}},
			},
		}

		setReplicas(mp, false, nil, nodeRefMap)

		g.Expect(mp.Status.ReadyReplicas).To(Equal(ptr.To[int32](2)))
		g.Expect(mp.Status.AvailableReplicas).To(Equal(ptr.To[int32](1)))
		g.Expect(mp.Status.UpToDateReplicas).To(BeNil())
	})

	t.Run("without MachinePool Machines no matching Nodes reports zero ready and available replicas", func(t *testing.T) {
		g := NewWithT(t)
		mp := &clusterv1.MachinePool{
			Spec: clusterv1.MachinePoolSpec{
				Replicas:       ptr.To[int32](2),
				ProviderIDList: []string{"provider-id-1", "provider-id-2"},
			},
			Status: clusterv1.MachinePoolStatus{
				Replicas:          ptr.To[int32](2),
				ReadyReplicas:     ptr.To[int32](2),
				AvailableReplicas: ptr.To[int32](2),
				UpToDateReplicas:  ptr.To[int32](2),
			},
		}

		setReplicas(mp, false, nil, map[string]*corev1.Node{})

		g.Expect(mp.Status.ReadyReplicas).To(Equal(ptr.To[int32](0)))
		g.Expect(mp.Status.AvailableReplicas).To(Equal(ptr.To[int32](0)))
		g.Expect(mp.Status.UpToDateReplicas).To(BeNil())
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

type fakeMachineOption func(m *clusterv1.Machine)

func Test_updateStatus_deletingWithoutInfraMachinePool(t *testing.T) {
	g := NewWithT(t)
	deletionTimestamp := metav1.Now()
	mp := &clusterv1.MachinePool{
		ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &deletionTimestamp, Finalizers: []string{"test"}},
		Spec:       clusterv1.MachinePoolSpec{Replicas: ptr.To[int32](3)},
		Status:     clusterv1.MachinePoolStatus{Replicas: ptr.To[int32](3), AvailableReplicas: ptr.To[int32](3)},
	}

	r := &Reconciler{}
	err := r.updateStatus(ctx, &scope{machinePool: mp, getMachinesForMachinePoolSucceeded: true})
	g.Expect(err).ToNot(HaveOccurred())

	// Deleting is surfaced even though the infrastructure object hasn't been read.
	deleting := conditions.Get(mp, clusterv1.MachinePoolDeletingCondition)
	g.Expect(deleting).ToNot(BeNil())
	g.Expect(deleting.Status).To(Equal(metav1.ConditionTrue))
	g.Expect(deleting.Reason).To(Equal(clusterv1.MachinePoolDeletingReason))

	// Scaling conditions are unknown because provider replicas were not observed during this reconcile.
	expectedScalingReasons := map[string]string{
		clusterv1.MachinePoolScalingUpCondition:   clusterv1.MachinePoolScalingUpReplicasNotObservedReason,
		clusterv1.MachinePoolScalingDownCondition: clusterv1.MachinePoolScalingDownReplicasNotObservedReason,
	}
	for conditionType, reason := range expectedScalingReasons {
		condition := conditions.Get(mp, conditionType)
		g.Expect(condition).ToNot(BeNil())
		g.Expect(condition.Status).To(Equal(metav1.ConditionUnknown))
		g.Expect(condition.Reason).To(Equal(reason))
		g.Expect(condition.Message).To(Equal("status.replicas was not observed"))
	}

	// Per-Machine aggregate conditions report unknown when Machine support cannot be determined.
	for _, conditionType := range machinePoolAggregateConditionTypes() {
		condition := conditions.Get(mp, conditionType)
		g.Expect(condition).ToNot(BeNil())
		g.Expect(condition.Status).To(Equal(metav1.ConditionUnknown))
		g.Expect(condition.Reason).To(Equal(clusterv1.MachinePoolMachineSupportUnknownReason))
		g.Expect(condition.Message).To(Equal("Machine support could not be determined"))
	}
	g.Expect(conditions.Get(mp, clusterv1.MachinePoolInfrastructureReadyCondition)).To(BeNil())
}

func Test_updateStatus_deletingWithObservedInfrastructureReplicas(t *testing.T) {
	g := NewWithT(t)
	deletionTimestamp := metav1.Now()
	mp := &clusterv1.MachinePool{
		ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &deletionTimestamp, Finalizers: []string{"test"}},
		Spec:       clusterv1.MachinePoolSpec{Replicas: ptr.To[int32](3)},
		Status:     clusterv1.MachinePoolStatus{Replicas: ptr.To[int32](3)},
	}
	infraMachinePool := &unstructured.Unstructured{Object: map[string]interface{}{
		"status": map[string]interface{}{},
	}}

	r := &Reconciler{}
	err := r.updateStatus(ctx, &scope{
		machinePool:                        mp,
		infraMachinePool:                   infraMachinePool,
		getMachinesForMachinePoolSucceeded: true,
		infrastructureReplicasObserved:     true,
	})
	g.Expect(err).ToNot(HaveOccurred())

	scalingDown := conditions.Get(mp, clusterv1.MachinePoolScalingDownCondition)
	g.Expect(scalingDown).ToNot(BeNil())
	g.Expect(scalingDown.Status).To(Equal(metav1.ConditionTrue))
	g.Expect(scalingDown.Message).To(Equal("Scaling down from 3 to 0 replicas"))
}

func Test_updateStatus_machineListFailurePreservesCounters(t *testing.T) {
	g := NewWithT(t)
	mp := &clusterv1.MachinePool{
		Spec: clusterv1.MachinePoolSpec{
			Replicas: ptr.To[int32](3),
		},
		Status: clusterv1.MachinePoolStatus{
			Replicas:          ptr.To[int32](3),
			ReadyReplicas:     ptr.To[int32](2),
			AvailableReplicas: ptr.To[int32](2),
			UpToDateReplicas:  ptr.To[int32](1),
			Initialization: clusterv1.MachinePoolInitializationStatus{
				InfrastructureProvisioned: ptr.To(true),
			},
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

	for _, conditionType := range machinePoolAggregateConditionTypes() {
		condition := conditions.Get(mp, conditionType)
		g.Expect(condition).ToNot(BeNil())
		g.Expect(condition.Status).To(Equal(metav1.ConditionUnknown))
		g.Expect(condition.Reason).To(Equal(clusterv1.InternalErrorReason))
	}

	available := conditions.Get(mp, clusterv1.MachinePoolAvailableCondition)
	g.Expect(available).ToNot(BeNil())
	g.Expect(available.Status).To(Equal(metav1.ConditionUnknown))
	g.Expect(available.Reason).To(Equal(clusterv1.MachinePoolAvailableReplicaCountersNotObservedReason))
	g.Expect(available.Message).To(Equal("Replica counters were not observed"))
}

func Test_updateStatus_machinelessPoolUsesNodeCounters(t *testing.T) {
	g := NewWithT(t)
	mp := &clusterv1.MachinePool{
		Spec: clusterv1.MachinePoolSpec{
			Replicas:       ptr.To[int32](1),
			ProviderIDList: []string{"provider-id-1"},
		},
		Status: clusterv1.MachinePoolStatus{
			Replicas: ptr.To[int32](1),
		},
	}
	infraMachinePool := fakeExternalObject("GenericInfrastructureMachinePool", metav1.ConditionTrue, "")

	r := &Reconciler{}
	err := r.updateStatus(ctx, &scope{
		machinePool:                          mp,
		infraMachinePool:                     infraMachinePool,
		nodeRefMap:                           map[string]*corev1.Node{},
		nodeRefMapObserved:                   true,
		infrastructureProviderIDListObserved: true,
		infrastructureReplicasObserved:       true,
		getMachinesForMachinePoolSucceeded:   true,
	})
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(mp.Status.ReadyReplicas).To(Equal(ptr.To[int32](0)))
	g.Expect(mp.Status.AvailableReplicas).To(Equal(ptr.To[int32](0)))
	g.Expect(mp.Status.UpToDateReplicas).To(BeNil())
	available := conditions.Get(mp, clusterv1.MachinePoolAvailableCondition)
	g.Expect(available).ToNot(BeNil())
	g.Expect(available.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(available.Reason).To(Equal(clusterv1.MachinePoolNotAvailableReason))
}

func Test_updateStatus_knownMachinelessPoolRemovesAggregateConditions(t *testing.T) {
	g := NewWithT(t)
	mp := &clusterv1.MachinePool{
		Spec: clusterv1.MachinePoolSpec{
			Replicas: ptr.To[int32](3),
		},
		Status: clusterv1.MachinePoolStatus{
			Replicas: ptr.To[int32](3),
		},
	}
	for _, conditionType := range machinePoolAggregateConditionTypes() {
		conditions.Set(mp, metav1.Condition{
			Type:   conditionType,
			Status: metav1.ConditionTrue,
			Reason: "PreviouslyReported",
		})
	}

	r := &Reconciler{}
	err := r.updateStatus(ctx, &scope{
		machinePool:                        mp,
		infraMachinePool:                   &unstructured.Unstructured{},
		getMachinesForMachinePoolSucceeded: true,
	})
	g.Expect(err).ToNot(HaveOccurred())

	for _, conditionType := range machinePoolAggregateConditionTypes() {
		g.Expect(conditions.Get(mp, conditionType)).To(BeNil())
	}
}

func Test_updateStatus_unknownMachinePoolMachinesStateReportsUnknown(t *testing.T) {
	g := NewWithT(t)
	mp := &clusterv1.MachinePool{
		Spec: clusterv1.MachinePoolSpec{
			Replicas: ptr.To[int32](3),
		},
	}
	for _, conditionType := range machinePoolAggregateConditionTypes() {
		conditions.Set(mp, metav1.Condition{
			Type:   conditionType,
			Status: metav1.ConditionTrue,
			Reason: "PreviouslyReported",
		})
	}

	r := &Reconciler{}
	err := r.updateStatus(ctx, &scope{
		machinePool:                        mp,
		getMachinesForMachinePoolSucceeded: true,
	})
	g.Expect(err).ToNot(HaveOccurred())

	for _, conditionType := range machinePoolAggregateConditionTypes() {
		condition := conditions.Get(mp, conditionType)
		g.Expect(condition).ToNot(BeNil())
		g.Expect(condition.Status).To(Equal(metav1.ConditionUnknown))
		g.Expect(condition.Reason).To(Equal(clusterv1.MachinePoolMachineSupportUnknownReason))
		g.Expect(condition.Message).To(Equal("Machine support could not be determined"))
	}
}

func Test_updateStatus_existingMachinesProveMachineBacked(t *testing.T) {
	g := NewWithT(t)
	mp := &clusterv1.MachinePool{
		Spec: clusterv1.MachinePoolSpec{
			Replicas: ptr.To[int32](1),
		},
	}
	machine := fakeMachine("machine-1",
		withMachineCondition(metav1.Condition{
			Type:   clusterv1.MachineReadyCondition,
			Status: metav1.ConditionTrue,
			Reason: clusterv1.MachineReadyReason,
		}),
		withMachineCondition(metav1.Condition{
			Type:   clusterv1.MachineAvailableCondition,
			Status: metav1.ConditionTrue,
			Reason: clusterv1.MachineAvailableReason,
		}),
		withMachineCondition(metav1.Condition{
			Type:   clusterv1.MachineUpToDateCondition,
			Status: metav1.ConditionTrue,
			Reason: clusterv1.MachineUpToDateReason,
		}),
	)

	r := &Reconciler{}
	err := r.updateStatus(ctx, &scope{
		machinePool:                        mp,
		machines:                           []*clusterv1.Machine{machine},
		getMachinesForMachinePoolSucceeded: true,
	})
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(mp.Status.ReadyReplicas).To(Equal(ptr.To[int32](1)))
	g.Expect(mp.Status.AvailableReplicas).To(Equal(ptr.To[int32](1)))
	g.Expect(mp.Status.UpToDateReplicas).To(Equal(ptr.To[int32](1)))
	g.Expect(conditions.IsTrue(mp, clusterv1.MachinePoolMachinesReadyCondition)).To(BeTrue())
	g.Expect(conditions.IsTrue(mp, clusterv1.MachinePoolMachinesUpToDateCondition)).To(BeTrue())
}

func Test_updateStatus_malformedInfrastructureMachineKind(t *testing.T) {
	g := NewWithT(t)
	mp := &clusterv1.MachinePool{
		Spec: clusterv1.MachinePoolSpec{
			Replicas: ptr.To[int32](1),
		},
	}
	infraMachinePool := &unstructured.Unstructured{Object: map[string]interface{}{
		"status": map[string]interface{}{
			"infrastructureMachineKind": int64(1),
		},
	}}

	r := &Reconciler{}
	err := r.updateStatus(ctx, &scope{
		machinePool:      mp,
		infraMachinePool: infraMachinePool,
	})
	g.Expect(err).To(HaveOccurred())

	for _, conditionType := range machinePoolAggregateConditionTypes() {
		condition := conditions.Get(mp, conditionType)
		g.Expect(condition).ToNot(BeNil())
		g.Expect(condition.Status).To(Equal(metav1.ConditionUnknown))
		g.Expect(condition.Reason).To(Equal(clusterv1.InternalErrorReason))
	}
}

func Test_aggregateConditions_internalErrorWhenMachinesListFailed(t *testing.T) {
	// When the pool is machine-backed but the machines couldn't be listed, the per-Machine
	// aggregate conditions must surface an internal error instead of a false healthy state.
	tests := []struct {
		name          string
		set           func(mp *clusterv1.MachinePool)
		conditionType string
	}{
		{
			name: clusterv1.MachinePoolMachinesReadyCondition,
			set: func(mp *clusterv1.MachinePool) {
				setMachinesReadyCondition(ctx, mp, nil, machinePoolMachinesStateSupported, false)
			},
			conditionType: clusterv1.MachinePoolMachinesReadyCondition,
		},
		{
			name: clusterv1.MachinePoolMachinesUpToDateCondition,
			set: func(mp *clusterv1.MachinePool) {
				setMachinesUpToDateCondition(ctx, mp, nil, machinePoolMachinesStateSupported, false)
			},
			conditionType: clusterv1.MachinePoolMachinesUpToDateCondition,
		},
		{
			name: clusterv1.MachinePoolRollingOutCondition,
			set: func(mp *clusterv1.MachinePool) {
				setRollingOutCondition(ctx, mp, nil, machinePoolMachinesStateSupported, false)
			},
			conditionType: clusterv1.MachinePoolRollingOutCondition,
		},
		{
			name: clusterv1.MachinePoolRemediatingCondition,
			set: func(mp *clusterv1.MachinePool) {
				setRemediatingCondition(ctx, mp, nil, machinePoolMachinesStateSupported, false)
			},
			conditionType: clusterv1.MachinePoolRemediatingCondition,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			mp := &clusterv1.MachinePool{}
			tt.set(mp)

			condition := conditions.Get(mp, tt.conditionType)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionUnknown))
			g.Expect(condition.Message).To(Equal("Please check controller logs for errors"))
		})
	}
}

func Test_aggregateConditions_internalErrorWhenMachinesListFailsDuringDeletion(t *testing.T) {
	deletionTimestamp := metav1.Now()
	tests := []struct {
		name          string
		set           func(mp *clusterv1.MachinePool)
		conditionType string
	}{
		{
			name: clusterv1.MachinePoolMachinesReadyCondition,
			set: func(mp *clusterv1.MachinePool) {
				setMachinesReadyCondition(ctx, mp, nil, machinePoolMachinesStateUnknown, false)
			},
			conditionType: clusterv1.MachinePoolMachinesReadyCondition,
		},
		{
			name: clusterv1.MachinePoolMachinesUpToDateCondition,
			set: func(mp *clusterv1.MachinePool) {
				setMachinesUpToDateCondition(ctx, mp, nil, machinePoolMachinesStateUnknown, false)
			},
			conditionType: clusterv1.MachinePoolMachinesUpToDateCondition,
		},
		{
			name: clusterv1.MachinePoolRollingOutCondition,
			set: func(mp *clusterv1.MachinePool) {
				setRollingOutCondition(ctx, mp, nil, machinePoolMachinesStateUnknown, false)
			},
			conditionType: clusterv1.MachinePoolRollingOutCondition,
		},
		{
			name: clusterv1.MachinePoolRemediatingCondition,
			set: func(mp *clusterv1.MachinePool) {
				setRemediatingCondition(ctx, mp, nil, machinePoolMachinesStateUnknown, false)
			},
			conditionType: clusterv1.MachinePoolRemediatingCondition,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			mp := &clusterv1.MachinePool{
				ObjectMeta: metav1.ObjectMeta{
					DeletionTimestamp: &deletionTimestamp,
					Finalizers:        []string{"test"},
				},
			}
			tt.set(mp)

			condition := conditions.Get(mp, tt.conditionType)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionUnknown))
			g.Expect(condition.Reason).To(Equal(clusterv1.InternalErrorReason))
		})
	}
}

func machinePoolAggregateConditionTypes() []string {
	return []string{
		clusterv1.MachinePoolMachinesReadyCondition,
		clusterv1.MachinePoolMachinesUpToDateCondition,
		clusterv1.MachinePoolRollingOutCondition,
		clusterv1.MachinePoolRemediatingCondition,
	}
}

func Test_setBootstrapConfigReadyCondition(t *testing.T) {
	tests := []struct {
		name                      string
		machinePool               *clusterv1.MachinePool
		bootstrapConfig           *unstructured.Unstructured
		bootstrapConfigIsNotFound bool
		expectNotSet              bool
		expectCondition           metav1.Condition
	}{
		{
			name: "user-provided data secret (no ConfigRef)",
			machinePool: &clusterv1.MachinePool{
				Spec: clusterv1.MachinePoolSpec{
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Bootstrap: clusterv1.Bootstrap{DataSecretName: ptr.To("secret")},
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachinePoolBootstrapConfigReadyCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachinePoolBootstrapDataSecretProvidedReason,
			},
		},
		{
			name:            "mirror Ready condition from bootstrap config",
			machinePool:     defaultMachinePoolWithBootstrap(),
			bootstrapConfig: fakeExternalObject("GenericBootstrapConfig", metav1.ConditionTrue, "some message"),
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolBootstrapConfigReadyCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachinePoolBootstrapConfigReadyReason,
				Message: "some message",
			},
		},
		{
			name:                      "bootstrap config not found",
			machinePool:               defaultMachinePoolWithBootstrap(),
			bootstrapConfigIsNotFound: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolBootstrapConfigReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachinePoolBootstrapConfigDoesNotExistReason,
				Message: "GenericBootstrapConfig does not exist",
			},
		},
		{
			name:        "bootstrap config internal error when read failed",
			machinePool: defaultMachinePoolWithBootstrap(),
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolBootstrapConfigReadyCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachinePoolBootstrapConfigInternalErrorReason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name: "condition left unchanged while deleting and bootstrap config not read",
			machinePool: func() *clusterv1.MachinePool {
				mp := defaultMachinePoolWithBootstrap()
				now := metav1.Now()
				mp.DeletionTimestamp = &now
				mp.Finalizers = []string{"test"}
				return mp
			}(),
			expectNotSet: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			setBootstrapConfigReadyCondition(ctx, tt.machinePool, tt.bootstrapConfig, tt.bootstrapConfigIsNotFound)

			condition := conditions.Get(tt.machinePool, clusterv1.MachinePoolBootstrapConfigReadyCondition)
			if tt.expectNotSet {
				g.Expect(condition).To(BeNil())
				return
			}
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tt.expectCondition, conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func Test_setInfrastructureReadyCondition(t *testing.T) {
	tests := []struct {
		name                       string
		machinePool                *clusterv1.MachinePool
		infraMachinePool           *unstructured.Unstructured
		infraMachinePoolIsNotFound bool
		expectNotSet               bool
		expectCondition            metav1.Condition
	}{
		{
			name:             "mirror Ready condition from infra machine pool",
			machinePool:      defaultMachinePoolWithInfra(),
			infraMachinePool: fakeExternalObject("GenericInfrastructureMachinePool", metav1.ConditionTrue, "some message"),
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolInfrastructureReadyCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachinePoolInfrastructureReadyReason,
				Message: "some message",
			},
		},
		{
			name:                       "infra machine pool not found and not provisioned",
			machinePool:                defaultMachinePoolWithInfra(),
			infraMachinePoolIsNotFound: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolInfrastructureReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachinePoolInfrastructureDoesNotExistReason,
				Message: "GenericInfrastructureMachinePool does not exist",
			},
		},
		{
			name: "infra machine pool deleted after being provisioned",
			machinePool: func() *clusterv1.MachinePool {
				mp := defaultMachinePoolWithInfra()
				mp.Status.Initialization.InfrastructureProvisioned = ptr.To(true)
				return mp
			}(),
			infraMachinePoolIsNotFound: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolInfrastructureReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachinePoolInfrastructureDeletedReason,
				Message: "GenericInfrastructureMachinePool has been deleted",
			},
		},
		{
			name: "condition left unchanged while deleting and infra machine pool not read",
			machinePool: func() *clusterv1.MachinePool {
				mp := defaultMachinePoolWithInfra()
				now := metav1.Now()
				mp.DeletionTimestamp = &now
				mp.Finalizers = []string{"test"}
				return mp
			}(),
			expectNotSet: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			setInfrastructureReadyCondition(ctx, tt.machinePool, tt.infraMachinePool, tt.infraMachinePoolIsNotFound)

			condition := conditions.Get(tt.machinePool, clusterv1.MachinePoolInfrastructureReadyCondition)
			if tt.expectNotSet {
				g.Expect(condition).To(BeNil())
				return
			}
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tt.expectCondition, conditions.IgnoreLastTransitionTime(true)))
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
		name                     string
		machines                 []*clusterv1.Machine
		machinePoolMachinesState machinePoolMachinesState
		expectSet                bool
		expectCondition          metav1.Condition
	}{
		{
			name:                     "machineless pool does not set the condition",
			machines:                 nil,
			machinePoolMachinesState: machinePoolMachinesStateNotSupported,
			expectSet:                false,
		},
		{
			name:                     "no machines",
			machines:                 []*clusterv1.Machine{},
			machinePoolMachinesState: machinePoolMachinesStateSupported,
			expectSet:                true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachinePoolMachinesReadyCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachinePoolMachinesReadyNoReplicasReason,
			},
		},
		{
			name: "all machines ready",
			machines: []*clusterv1.Machine{
				fakeMachine("machine-1", withMachineCondition(readyCondition)),
				fakeMachine("machine-2", withMachineCondition(readyCondition)),
			},
			machinePoolMachinesState: machinePoolMachinesStateSupported,
			expectSet:                true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachinePoolMachinesReadyCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachinePoolMachinesReadyReason,
			},
		},
		{
			name: "one machine not ready",
			machines: []*clusterv1.Machine{
				fakeMachine("machine-1", withMachineCondition(readyCondition)),
				fakeMachine("machine-2", withMachineCondition(metav1.Condition{
					Type:    clusterv1.MachineReadyCondition,
					Status:  metav1.ConditionFalse,
					Reason:  "SomeReason",
					Message: "Some message",
				})),
			},
			machinePoolMachinesState: machinePoolMachinesStateSupported,
			expectSet:                true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolMachinesReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachinePoolMachinesNotReadyReason,
				Message: "* Machine machine-2: Some message",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			mp := &clusterv1.MachinePool{}
			setMachinesReadyCondition(ctx, mp, tt.machines, tt.machinePoolMachinesState, true)

			condition := conditions.Get(mp, clusterv1.MachinePoolMachinesReadyCondition)
			if !tt.expectSet {
				g.Expect(condition).To(BeNil())
				return
			}
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tt.expectCondition, conditions.IgnoreLastTransitionTime(true)))
		})
	}
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
		expectSet                bool
		expectCondition          metav1.Condition
	}{
		{
			name:                     "machineless pool does not set the condition",
			machines:                 nil,
			machinePoolMachinesState: machinePoolMachinesStateNotSupported,
			expectSet:                false,
		},
		{
			name:                     "no machines",
			machines:                 []*clusterv1.Machine{},
			machinePoolMachinesState: machinePoolMachinesStateSupported,
			expectSet:                true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachinePoolMachinesUpToDateCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachinePoolMachinesUpToDateNoReplicasReason,
			},
		},
		{
			name: "recently created machine without condition is ignored",
			machines: []*clusterv1.Machine{
				fakeMachine("machine-new", withCreationTimestamp(time.Now())),
			},
			machinePoolMachinesState: machinePoolMachinesStateSupported,
			expectSet:                true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachinePoolMachinesUpToDateCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachinePoolMachinesUpToDateNoReplicasReason,
			},
		},
		{
			name: "all machines up to date",
			machines: []*clusterv1.Machine{
				fakeMachine("machine-1", withMachineCondition(upToDateCondition)),
			},
			machinePoolMachinesState: machinePoolMachinesStateSupported,
			expectSet:                true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachinePoolMachinesUpToDateCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachinePoolMachinesUpToDateReason,
			},
		},
		{
			name: "one machine not up to date",
			machines: []*clusterv1.Machine{
				fakeMachine("machine-1", withMachineCondition(metav1.Condition{
					Type:    clusterv1.MachineUpToDateCondition,
					Status:  metav1.ConditionFalse,
					Reason:  "SomeReason",
					Message: "Version is not up to date",
				})),
			},
			machinePoolMachinesState: machinePoolMachinesStateSupported,
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
			setMachinesUpToDateCondition(ctx, mp, tt.machines, tt.machinePoolMachinesState, true)

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

func Test_setRollingOutCondition(t *testing.T) {
	tests := []struct {
		name                     string
		machines                 []*clusterv1.Machine
		machinePoolMachinesState machinePoolMachinesState
		expectSet                bool
		expectCondition          metav1.Condition
	}{
		{
			name:                     "machineless pool does not set the condition",
			machines:                 nil,
			machinePoolMachinesState: machinePoolMachinesStateNotSupported,
			expectSet:                false,
		},
		{
			name: "all machines up to date, not rolling out",
			machines: []*clusterv1.Machine{
				fakeMachine("machine-1", withMachineCondition(metav1.Condition{
					Type:   clusterv1.MachineUpToDateCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineUpToDateReason,
				})),
			},
			machinePoolMachinesState: machinePoolMachinesStateSupported,
			expectSet:                true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachinePoolRollingOutCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachinePoolNotRollingOutReason,
			},
		},
		{
			name: "one machine not up to date, rolling out",
			machines: []*clusterv1.Machine{
				fakeMachine("machine-1", withMachineCondition(metav1.Condition{
					Type:    clusterv1.MachineUpToDateCondition,
					Status:  metav1.ConditionFalse,
					Reason:  "SomeReason",
					Message: "* Version v1.30.0, v1.31.0 required",
				})),
			},
			machinePoolMachinesState: machinePoolMachinesStateSupported,
			expectSet:                true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolRollingOutCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachinePoolRollingOutReason,
				Message: "Rolling out 1 not up-to-date replicas\n* Version v1.30.0, v1.31.0 required",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			mp := &clusterv1.MachinePool{}
			setRollingOutCondition(ctx, mp, tt.machines, tt.machinePoolMachinesState, true)

			condition := conditions.Get(mp, clusterv1.MachinePoolRollingOutCondition)
			if !tt.expectSet {
				g.Expect(condition).To(BeNil())
				return
			}
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tt.expectCondition, conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func Test_setRemediatingCondition(t *testing.T) {
	healthCheckSucceeded := metav1.Condition{Type: clusterv1.MachineHealthCheckSucceededCondition, Status: metav1.ConditionTrue}
	healthCheckNotSucceeded := metav1.Condition{Type: clusterv1.MachineHealthCheckSucceededCondition, Status: metav1.ConditionFalse}
	ownerRemediated := metav1.Condition{Type: clusterv1.MachineOwnerRemediatedCondition, Status: metav1.ConditionFalse, Message: "Remediation in progress"}

	tests := []struct {
		name                     string
		machines                 []*clusterv1.Machine
		machinePoolMachinesState machinePoolMachinesState
		expectSet                bool
		expectCondition          metav1.Condition
	}{
		{
			name:                     "machineless pool does not set the condition",
			machines:                 nil,
			machinePoolMachinesState: machinePoolMachinesStateNotSupported,
			expectSet:                false,
		},
		{
			name: "no machines to be remediated",
			machines: []*clusterv1.Machine{
				fakeMachine("machine-1", withMachineCondition(healthCheckSucceeded)),
			},
			machinePoolMachinesState: machinePoolMachinesStateSupported,
			expectSet:                true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachinePoolRemediatingCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachinePoolNotRemediatingReason,
			},
		},
		{
			name: "one machine being remediated",
			machines: []*clusterv1.Machine{
				fakeMachine("machine-1", withMachineCondition(healthCheckNotSucceeded), withMachineCondition(ownerRemediated)),
			},
			machinePoolMachinesState: machinePoolMachinesStateSupported,
			expectSet:                true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolRemediatingCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachinePoolRemediatingReason,
				Message: "* Machine machine-1: Remediation in progress",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			mp := &clusterv1.MachinePool{}
			setRemediatingCondition(ctx, mp, tt.machines, tt.machinePoolMachinesState, true)

			condition := conditions.Get(mp, clusterv1.MachinePoolRemediatingCondition)
			if !tt.expectSet {
				g.Expect(condition).To(BeNil())
				return
			}
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tt.expectCondition, conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func Test_setScalingUpCondition(t *testing.T) {
	tests := []struct {
		name             string
		machinePool      *clusterv1.MachinePool
		replicasObserved bool
		expectCondition  metav1.Condition
	}{
		{
			name:             "replicas not observed",
			machinePool:      fakeMachinePoolReplicas(3, 1),
			replicasObserved: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolScalingUpCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachinePoolScalingUpReplicasNotObservedReason,
				Message: "status.replicas was not observed",
			},
		},
		{
			name:             "spec.replicas not set",
			machinePool:      &clusterv1.MachinePool{},
			replicasObserved: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolScalingUpCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachinePoolScalingUpWaitingForReplicasSetReason,
				Message: "Waiting for spec.replicas set",
			},
		},
		{
			name:             "scaling up",
			machinePool:      fakeMachinePoolReplicas(3, 1),
			replicasObserved: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolScalingUpCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachinePoolScalingUpReason,
				Message: "Scaling up from 1 to 3 replicas",
			},
		},
		{
			name:             "not scaling up",
			machinePool:      fakeMachinePoolReplicas(3, 3),
			replicasObserved: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachinePoolScalingUpCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachinePoolNotScalingUpReason,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			setScalingUpCondition(ctx, tt.machinePool, tt.replicasObserved)

			condition := conditions.Get(tt.machinePool, clusterv1.MachinePoolScalingUpCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tt.expectCondition, conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func Test_setScalingDownCondition(t *testing.T) {
	tests := []struct {
		name                 string
		machinePool          *clusterv1.MachinePool
		machines             []*clusterv1.Machine
		getMachinesSucceeded bool
		replicasObserved     bool
		expectCondition      metav1.Condition
	}{
		{
			name:                 "replicas not observed",
			machinePool:          fakeMachinePoolReplicas(3, 1),
			getMachinesSucceeded: true,
			replicasObserved:     false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolScalingDownCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachinePoolScalingDownReplicasNotObservedReason,
				Message: "status.replicas was not observed",
			},
		},
		{
			name:                 "spec.replicas not set",
			machinePool:          &clusterv1.MachinePool{},
			getMachinesSucceeded: true,
			replicasObserved:     true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolScalingDownCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachinePoolScalingDownWaitingForReplicasSetReason,
				Message: "Waiting for spec.replicas set",
			},
		},
		{
			name:                 "scaling down",
			machinePool:          fakeMachinePoolReplicas(1, 3),
			getMachinesSucceeded: true,
			replicasObserved:     true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolScalingDownCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachinePoolScalingDownReason,
				Message: "Scaling down from 3 to 1 replicas",
			},
		},
		{
			name:        "scaling down with a stale Machine",
			machinePool: fakeMachinePoolReplicas(1, 3),
			machines: []*clusterv1.Machine{
				fakeMachine("machine-1", withDeletionTimestamp(time.Now().Add(-time.Hour))),
			},
			getMachinesSucceeded: true,
			replicasObserved:     true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolScalingDownCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachinePoolScalingDownReason,
				Message: "Scaling down from 3 to 1 replicas\n* Machine machine-1 is in deletion since more than 15m",
			},
		},
		{
			name:                 "not scaling down",
			machinePool:          fakeMachinePoolReplicas(3, 3),
			getMachinesSucceeded: true,
			replicasObserved:     true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachinePoolScalingDownCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachinePoolNotScalingDownReason,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			setScalingDownCondition(ctx, tt.machinePool, tt.machines, tt.getMachinesSucceeded, tt.replicasObserved)

			condition := conditions.Get(tt.machinePool, clusterv1.MachinePoolScalingDownCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tt.expectCondition, conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func Test_setDeletingCondition(t *testing.T) {
	deletionTimestamp := metav1.Now()

	tests := []struct {
		name                 string
		machinePool          *clusterv1.MachinePool
		machines             []*clusterv1.Machine
		getMachinesSucceeded bool
		expectCondition      metav1.Condition
	}{
		{
			name:                 "not deleting",
			machinePool:          &clusterv1.MachinePool{},
			getMachinesSucceeded: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachinePoolDeletingCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachinePoolNotDeletingReason,
			},
		},
		{
			name: "deleting machineless pool",
			machinePool: &clusterv1.MachinePool{
				ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &deletionTimestamp, Finalizers: []string{"test"}},
			},
			getMachinesSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolDeletingCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachinePoolDeletingReason,
				Message: "Deletion in progress",
			},
		},
		{
			name: "deleting pool with Nodes",
			machinePool: &clusterv1.MachinePool{
				ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &deletionTimestamp, Finalizers: []string{"test"}},
				Status: clusterv1.MachinePoolStatus{
					NodeRefs: []corev1.ObjectReference{{Name: "node-1"}, {Name: "node-2"}},
				},
			},
			getMachinesSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolDeletingCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachinePoolDeletingReason,
				Message: "Deleting 2 Nodes",
			},
		},
		{
			name: "deleting pool with machines",
			machinePool: &clusterv1.MachinePool{
				ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &deletionTimestamp, Finalizers: []string{"test"}},
			},
			machines:             []*clusterv1.Machine{fakeMachine("machine-1"), fakeMachine("machine-2")},
			getMachinesSucceeded: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolDeletingCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachinePoolDeletingReason,
				Message: "Deleting 2 Machines",
			},
		},
		{
			name: "deleting pool when Machines cannot be listed",
			machinePool: &clusterv1.MachinePool{
				ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &deletionTimestamp, Finalizers: []string{"test"}},
			},
			getMachinesSucceeded: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolDeletingCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachinePoolDeletingInternalErrorReason,
				Message: "Please check controller logs for errors",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			setDeletingCondition(ctx, tt.machinePool, tt.machines, tt.getMachinesSucceeded)

			condition := conditions.Get(tt.machinePool, clusterv1.MachinePoolDeletingCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tt.expectCondition, conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func Test_setAvailableCondition(t *testing.T) {
	infraReady := metav1.Condition{Type: clusterv1.MachinePoolInfrastructureReadyCondition, Status: metav1.ConditionTrue, Reason: clusterv1.MachinePoolInfrastructureReadyReason}

	tests := []struct {
		name                    string
		machinePool             *clusterv1.MachinePool
		replicaCountersObserved bool
		expectCondition         metav1.Condition
	}{
		{
			name:                    "spec.replicas not set",
			machinePool:             &clusterv1.MachinePool{},
			replicaCountersObserved: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolAvailableCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachinePoolAvailableWaitingForReplicasSetReason,
				Message: "Waiting for spec.replicas set",
			},
		},
		{
			name: "status.availableReplicas not set",
			machinePool: &clusterv1.MachinePool{
				Spec: clusterv1.MachinePoolSpec{Replicas: ptr.To[int32](3)},
			},
			replicaCountersObserved: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolAvailableCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachinePoolAvailableWaitingForAvailableReplicasSetReason,
				Message: "Waiting for status.availableReplicas set",
			},
		},
		{
			name: "replica counters not observed",
			machinePool: &clusterv1.MachinePool{
				Spec:   clusterv1.MachinePoolSpec{Replicas: ptr.To[int32](3)},
				Status: clusterv1.MachinePoolStatus{AvailableReplicas: ptr.To[int32](3)},
			},
			replicaCountersObserved: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolAvailableCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachinePoolAvailableReplicaCountersNotObservedReason,
				Message: "Replica counters were not observed",
			},
		},
		{
			name: "not available when infrastructure not ready",
			machinePool: &clusterv1.MachinePool{
				Spec:   clusterv1.MachinePoolSpec{Replicas: ptr.To[int32](3)},
				Status: clusterv1.MachinePoolStatus{AvailableReplicas: ptr.To[int32](3)},
			},
			replicaCountersObserved: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolAvailableCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachinePoolNotAvailableReason,
				Message: "Infrastructure not ready",
			},
		},
		{
			name: "available when infrastructure ready and enough available replicas",
			machinePool: func() *clusterv1.MachinePool {
				mp := &clusterv1.MachinePool{
					Spec:   clusterv1.MachinePoolSpec{Replicas: ptr.To[int32](3)},
					Status: clusterv1.MachinePoolStatus{AvailableReplicas: ptr.To[int32](3)},
				}
				conditions.Set(mp, infraReady)
				return mp
			}(),
			replicaCountersObserved: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachinePoolAvailableCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachinePoolAvailableReason,
			},
		},
		{
			name: "not available when not enough available replicas",
			machinePool: func() *clusterv1.MachinePool {
				mp := &clusterv1.MachinePool{
					Spec:   clusterv1.MachinePoolSpec{Replicas: ptr.To[int32](3)},
					Status: clusterv1.MachinePoolStatus{AvailableReplicas: ptr.To[int32](1)},
				}
				conditions.Set(mp, infraReady)
				return mp
			}(),
			replicaCountersObserved: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolAvailableCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachinePoolNotAvailableReason,
				Message: "1 available replicas, at least 3 required",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			setAvailableCondition(ctx, tt.machinePool, tt.replicaCountersObserved)

			condition := conditions.Get(tt.machinePool, clusterv1.MachinePoolAvailableCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tt.expectCondition, conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func readyNode(name string, transitionTime time.Time) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{
			Type:               corev1.NodeReady,
			Status:             corev1.ConditionTrue,
			LastTransitionTime: metav1.Time{Time: transitionTime},
		}}},
	}
}

func fakeMachine(name string, options ...fakeMachineOption) *clusterv1.Machine {
	m := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
	for _, opt := range options {
		opt(m)
	}
	return m
}

func withMachineCondition(c metav1.Condition) fakeMachineOption {
	return func(m *clusterv1.Machine) {
		conditions.Set(m, c)
	}
}

func withCreationTimestamp(t time.Time) fakeMachineOption {
	return func(m *clusterv1.Machine) {
		m.CreationTimestamp = metav1.Time{Time: t}
	}
}

func withDeletionTimestamp(t time.Time) fakeMachineOption {
	return func(m *clusterv1.Machine) {
		m.DeletionTimestamp = ptr.To(metav1.Time{Time: t})
		m.Finalizers = []string{"test"}
	}
}

func fakeMachinePoolReplicas(specReplicas, statusReplicas int32) *clusterv1.MachinePool {
	return &clusterv1.MachinePool{
		Spec:   clusterv1.MachinePoolSpec{Replicas: ptr.To(specReplicas)},
		Status: clusterv1.MachinePoolStatus{Replicas: ptr.To(statusReplicas)},
	}
}

func defaultMachinePoolWithBootstrap() *clusterv1.MachinePool {
	return &clusterv1.MachinePool{
		Spec: clusterv1.MachinePoolSpec{
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: clusterv1.ContractVersionedObjectReference{
							APIGroup: clusterv1.GroupVersionBootstrap.Group,
							Kind:     "GenericBootstrapConfig",
							Name:     "bootstrap-config",
						},
					},
				},
			},
		},
	}
}

func defaultMachinePoolWithInfra() *clusterv1.MachinePool {
	return &clusterv1.MachinePool{
		Spec: clusterv1.MachinePoolSpec{
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						APIGroup: clusterv1.GroupVersionInfrastructure.Group,
						Kind:     "GenericInfrastructureMachinePool",
						Name:     "infra-machine-pool",
					},
				},
			},
		},
	}
}

func fakeExternalObject(kind string, readyStatus metav1.ConditionStatus, message string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"kind":       kind,
		"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
		"metadata": map[string]interface{}{
			"name":      "external-object",
			"namespace": metav1.NamespaceDefault,
		},
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{
					"type":    "Ready",
					"status":  string(readyStatus),
					"message": message,
				},
			},
		},
	}}
}
