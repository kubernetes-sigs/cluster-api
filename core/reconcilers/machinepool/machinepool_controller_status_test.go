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
	contractapi "sigs.k8s.io/cluster-api/internal/contract/api"
	contractv1beta1 "sigs.k8s.io/cluster-api/internal/contract/api/v1beta1"
	contractv1 "sigs.k8s.io/cluster-api/internal/contract/api/v1beta2"
	"sigs.k8s.io/cluster-api/util/conditions"
)

func Test_updateStatus(t *testing.T) {
	machinePool := &clusterv1.MachinePool{
		Spec: clusterv1.MachinePoolSpec{
			Replicas: ptr.To[int32](3),
		},
		Status: clusterv1.MachinePoolStatus{
			Replicas:          ptr.To[int32](2),
			ReadyReplicas:     ptr.To[int32](2),
			AvailableReplicas: ptr.To[int32](2),
			UpToDateReplicas:  ptr.To[int32](2),
			Versions:          []clusterv1.StatusVersion{{Version: "v1.31.1", Replicas: 2}},
		},
	}
	previousCondition := metav1.Condition{
		Type:               clusterv1.MachinePoolMachinesUpToDateCondition,
		Status:             metav1.ConditionTrue,
		Reason:             "PreviouslyReported",
		LastTransitionTime: metav1.NewTime(time.Now().Add(-time.Minute)),
	}
	machine := machinePoolTestMachine("machine-1", metav1.Condition{
		Type:   clusterv1.MachineUpToDateCondition,
		Status: metav1.ConditionTrue,
		Reason: clusterv1.MachineUpToDateReason,
	})
	infraPool := &unstructured.Unstructured{Object: map[string]interface{}{
		"status": map[string]interface{}{
			"infrastructureMachineKind": "GenericInfrastructureMachine",
		},
	}}
	malformedInfraPool := &unstructured.Unstructured{Object: map[string]interface{}{
		"status": map[string]interface{}{
			"infrastructureMachineKind": int64(1),
		},
	}}

	tests := []struct {
		name                 string
		infraPool            *unstructured.Unstructured
		machines             []*clusterv1.Machine
		getMachinesSucceeded bool
		deleting             bool
		expectErr            bool
		expectStatus         clusterv1.MachinePoolStatus
		expectCondition      *metav1.Condition
	}{
		{
			name:         "Machine list failure preserves counters and versions",
			infraPool:    infraPool,
			expectStatus: machinePool.Status,
			expectCondition: &metav1.Condition{
				Type:    clusterv1.MachinePoolMachinesUpToDateCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachinePoolMachinesUpToDateInternalErrorReason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:                 "machineless pool removes a stale condition",
			infraPool:            &unstructured.Unstructured{},
			getMachinesSucceeded: true,
			expectStatus: clusterv1.MachinePoolStatus{
				Replicas:          ptr.To[int32](2),
				ReadyReplicas:     ptr.To[int32](2),
				AvailableReplicas: ptr.To[int32](2),
				UpToDateReplicas:  ptr.To[int32](3),
			},
		},
		{
			name:                 "missing infrastructure preserves status",
			getMachinesSucceeded: true,
			expectStatus:         machinePool.Status,
			expectCondition:      &previousCondition,
		},
		{
			name:                 "missing infrastructure with existing Machines preserves status",
			machines:             []*clusterv1.Machine{machine},
			getMachinesSucceeded: true,
			expectStatus:         machinePool.Status,
			expectCondition:      &previousCondition,
		},
		{
			name:            "deleting without infrastructure preserves status",
			deleting:        true,
			expectStatus:    machinePool.Status,
			expectCondition: &previousCondition,
		},
		{
			name:                 "machine-backed pool aggregates status",
			infraPool:            infraPool,
			machines:             []*clusterv1.Machine{machine},
			getMachinesSucceeded: true,
			expectStatus: clusterv1.MachinePoolStatus{
				Replicas:          ptr.To[int32](2),
				ReadyReplicas:     ptr.To[int32](0),
				AvailableReplicas: ptr.To[int32](0),
				UpToDateReplicas:  ptr.To[int32](1),
			},
			expectCondition: &metav1.Condition{
				Type:   clusterv1.MachinePoolMachinesUpToDateCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachinePoolMachinesUpToDateReason,
			},
		},
		{
			name:                 "malformed infrastructureMachineKind preserves status",
			infraPool:            malformedInfraPool,
			getMachinesSucceeded: true,
			expectErr:            true,
			expectStatus:         machinePool.Status,
			expectCondition:      &previousCondition,
		},
		{
			name:                 "malformed infrastructureMachineKind with existing Machines preserves status",
			infraPool:            malformedInfraPool,
			machines:             []*clusterv1.Machine{machine},
			getMachinesSucceeded: true,
			expectErr:            true,
			expectStatus:         machinePool.Status,
			expectCondition:      &previousCondition,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			mp := machinePool.DeepCopy()
			if tt.deleting {
				deletionTimestamp := metav1.Now()
				mp.DeletionTimestamp = &deletionTimestamp
			}
			conditions.Set(mp, previousCondition)
			original := mp.DeepCopy()

			r := &Reconciler{}
			err := r.updateStatus(ctx, &scope{
				machinePool:                        mp,
				infraMachinePool:                   tt.infraPool,
				machines:                           tt.machines,
				getMachinesForMachinePoolSucceeded: tt.getMachinesSucceeded,
			})
			if tt.expectErr {
				g.Expect(err).To(MatchError(ContainSubstring(
					"determining if there are machine pool machines: failed to lookup infrastructureMachineKind",
				)))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}

			status := mp.Status
			status.Conditions = nil
			g.Expect(status).To(Equal(tt.expectStatus))
			condition := conditions.Get(mp, clusterv1.MachinePoolMachinesUpToDateCondition)
			if tt.expectCondition == nil {
				g.Expect(condition).To(BeNil())
			} else {
				g.Expect(condition).ToNot(BeNil())
				g.Expect(*condition).To(conditions.MatchCondition(
					*tt.expectCondition,
					conditions.IgnoreLastTransitionTime(true),
				))
			}
			if tt.infraPool == nil || tt.expectErr {
				withoutMirrors := mp.DeepCopy()
				conditions.Delete(withoutMirrors, clusterv1.MachinePoolBootstrapConfigReadyCondition)
				conditions.Delete(withoutMirrors, clusterv1.MachinePoolInfrastructureReadyCondition)
				g.Expect(withoutMirrors.Status).To(Equal(original.Status))
			}
		})
	}
}

func Test_setBootstrapConfigReadyCondition(t *testing.T) {
	tests := []struct {
		name             string
		config           contractapi.BootstrapConfig
		notFound         bool
		initialized      bool
		deleting         bool
		directSecret     bool
		initialCondition *metav1.Condition
		expectCondition  metav1.Condition
	}{
		{
			name:         "direct data secret",
			directSecret: true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachinePoolBootstrapConfigReadyCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachinePoolBootstrapDataSecretProvidedReason,
			},
		},
		{
			name: "mirror current provider",
			config: &contractv1.BootstrapConfig{
				Status: contractv1.BootstrapConfigStatus{
					Conditions: contractapi.Conditions{
						{
							Type:    clusterv1.ReadyCondition,
							Status:  metav1.ConditionFalse,
							Reason:  "ProviderNotReady",
							Message: "waiting for data",
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolBootstrapConfigReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  "ProviderNotReady",
				Message: "waiting for data",
			},
		},
		{
			name: "mirror legacy provider v1beta2 condition",
			config: &contractv1beta1.BootstrapConfig{
				Status: contractv1beta1.BootstrapConfigStatus{
					V1Beta2: &contractv1beta1.BootstrapConfigV1Beta2Status{
						Conditions: contractapi.Conditions{
							{
								Type:    clusterv1.ReadyCondition,
								Status:  metav1.ConditionTrue,
								Message: "data created",
							},
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolBootstrapConfigReadyCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachinePoolBootstrapConfigReadyReason,
				Message: "data created",
			},
		},
		{
			name: "legacy ready fallback",
			config: &contractv1beta1.BootstrapConfig{
				Status: contractv1beta1.BootstrapConfigStatus{
					Ready: true,
				},
			},
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachinePoolBootstrapConfigReadyCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachinePoolBootstrapConfigReadyReason,
			},
		},
		{
			name: "current fallback uses current provider value",
			config: &contractv1.BootstrapConfig{
				Status: contractv1.BootstrapConfigStatus{
					Initialization: contractv1.BootstrapConfigInitializationStatus{
						DataSecretCreated: ptr.To(false),
					},
				},
			},
			initialized: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolBootstrapConfigReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachinePoolBootstrapConfigNotReadyReason,
				Message: "GenericBootstrapConfig status.initialization.dataSecretCreated is false",
			},
		},
		{
			name: "invalid condition",
			config: &contractv1.BootstrapConfig{
				Status: contractv1.BootstrapConfigStatus{
					Conditions: contractapi.Conditions{
						{
							Type:   clusterv1.ReadyCondition,
							Status: "Invalid",
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolBootstrapConfigReadyCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachinePoolBootstrapConfigInvalidConditionReportedReason,
				Message: "status for the Ready condition must be one of True, False, Unknown",
			},
		},
		{
			name:     "missing object",
			notFound: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolBootstrapConfigReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachinePoolBootstrapConfigDoesNotExistReason,
				Message: "GenericBootstrapConfig does not exist",
			},
		},
		{
			name:        "deleted initialized object",
			notFound:    true,
			initialized: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolBootstrapConfigReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachinePoolBootstrapConfigDeletedReason,
				Message: "GenericBootstrapConfig has been deleted",
			},
		},
		{
			name:        "transient error after initialization",
			initialized: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolBootstrapConfigReadyCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachinePoolBootstrapConfigInternalErrorReason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:     "deletion preserves last condition",
			deleting: true,
			notFound: true,
			initialCondition: &metav1.Condition{
				Type:    clusterv1.MachinePoolBootstrapConfigReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  "PreviouslyReported",
				Message: "last observation",
			},
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolBootstrapConfigReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  "PreviouslyReported",
				Message: "last observation",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			mp := &clusterv1.MachinePool{
				Spec: clusterv1.MachinePoolSpec{
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Bootstrap: clusterv1.Bootstrap{
								ConfigRef: clusterv1.ContractVersionedObjectReference{
									APIGroup: clusterv1.GroupVersionBootstrap.Group,
									Kind:     "GenericBootstrapConfig",
									Name:     "bootstrap",
								},
							},
						},
					},
				},
				Status: clusterv1.MachinePoolStatus{
					Initialization: clusterv1.MachinePoolInitializationStatus{
						BootstrapDataSecretCreated: ptr.To(tt.initialized),
					},
				},
			}
			if tt.directSecret {
				mp.Spec.Template.Spec.Bootstrap = clusterv1.Bootstrap{
					DataSecretName: ptr.To("data"),
				}
			}
			if tt.deleting {
				mp.DeletionTimestamp = ptr.To(metav1.Now())
			}
			if tt.initialCondition != nil {
				conditions.Set(mp, *tt.initialCondition)
			}

			setBootstrapConfigReadyCondition(ctx, mp, tt.config, tt.notFound)

			condition := conditions.Get(mp, clusterv1.MachinePoolBootstrapConfigReadyCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tt.expectCondition, conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func Test_setInfrastructureReadyCondition(t *testing.T) {
	tests := []struct {
		name             string
		providerStatus   map[string]interface{}
		provisioned      *bool
		notFound         bool
		initialized      bool
		deleting         bool
		initialCondition *metav1.Condition
		expectCondition  metav1.Condition
	}{
		{
			name: "mirror current provider",
			providerStatus: map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":    "Ready",
						"status":  "False",
						"reason":  "ProviderNotReady",
						"message": "waiting for instances",
					},
				},
			},
			provisioned: ptr.To(true),
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolInfrastructureReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  "ProviderNotReady",
				Message: "waiting for instances",
			},
		},
		{
			name: "mirror legacy Ready without reason",
			providerStatus: map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":    "Ready",
						"status":  "True",
						"message": "instances ready",
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolInfrastructureReadyCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachinePoolInfrastructureReadyReason,
				Message: "instances ready",
			},
		},
		{
			name:           "provisioned fallback",
			providerStatus: map[string]interface{}{},
			provisioned:    ptr.To(true),
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachinePoolInfrastructureReadyCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachinePoolInfrastructureReadyReason,
			},
		},
		{
			name:           "not provisioned fallback ignores latched initialization",
			providerStatus: map[string]interface{}{},
			provisioned:    ptr.To(false),
			initialized:    true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolInfrastructureReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachinePoolInfrastructureNotReadyReason,
				Message: "GenericInfrastructureMachinePool status.initialization.provisioned is false",
			},
		},
		{
			name:           "failed provisioning observation ignores latched initialization",
			providerStatus: map[string]interface{}{},
			initialized:    true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolInfrastructureReadyCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachinePoolInfrastructureInternalErrorReason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name: "invalid condition status",
			providerStatus: map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "Ready",
						"status": "Invalid",
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolInfrastructureReadyCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachinePoolInfrastructureInvalidConditionReportedReason,
				Message: "failed to convert status.conditions from GenericInfrastructureMachinePool to []metav1.Condition: status for the Ready condition must be one of True, False, Unknown",
			},
		},
		{
			name:     "missing object",
			notFound: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolInfrastructureReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachinePoolInfrastructureDoesNotExistReason,
				Message: "GenericInfrastructureMachinePool does not exist",
			},
		},
		{
			name:        "deleted initialized object",
			notFound:    true,
			initialized: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolInfrastructureReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachinePoolInfrastructureDeletedReason,
				Message: "GenericInfrastructureMachinePool has been deleted while the MachinePool still exists",
			},
		},
		{
			name:        "transient error after initialization",
			initialized: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolInfrastructureReadyCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachinePoolInfrastructureInternalErrorReason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:     "deletion preserves last condition",
			deleting: true,
			notFound: true,
			initialCondition: &metav1.Condition{
				Type:    clusterv1.MachinePoolInfrastructureReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  "PreviouslyReported",
				Message: "last observation",
			},
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolInfrastructureReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  "PreviouslyReported",
				Message: "last observation",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			mp := &clusterv1.MachinePool{
				Spec: clusterv1.MachinePoolSpec{
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							InfrastructureRef: clusterv1.ContractVersionedObjectReference{
								APIGroup: clusterv1.GroupVersionInfrastructure.Group,
								Kind:     "GenericInfrastructureMachinePool",
								Name:     "infra",
							},
						},
					},
				},
				Status: clusterv1.MachinePoolStatus{
					Initialization: clusterv1.MachinePoolInitializationStatus{
						InfrastructureProvisioned: ptr.To(tt.initialized),
					},
				},
			}
			if tt.deleting {
				mp.DeletionTimestamp = ptr.To(metav1.Now())
			}
			if tt.initialCondition != nil {
				conditions.Set(mp, *tt.initialCondition)
			}
			var infraMachinePool *unstructured.Unstructured
			if tt.providerStatus != nil {
				infraMachinePool = &unstructured.Unstructured{
					Object: map[string]interface{}{
						"kind":   "GenericInfrastructureMachinePool",
						"status": tt.providerStatus,
					},
				}
			}

			setInfrastructureReadyCondition(ctx, mp, infraMachinePool, tt.notFound, tt.provisioned)

			condition := conditions.Get(mp, clusterv1.MachinePoolInfrastructureReadyCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tt.expectCondition, conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

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

func Test_setMachinesUpToDateCondition(t *testing.T) {
	upToDateCondition := metav1.Condition{
		Type:   clusterv1.MachineUpToDateCondition,
		Status: metav1.ConditionTrue,
		Reason: clusterv1.MachineUpToDateReason,
	}

	tests := []struct {
		name                   string
		machines               []*clusterv1.Machine
		hasMachinePoolMachines bool
		getMachinesSucceeded   bool
		initialCondition       *metav1.Condition
		expectSet              bool
		expectCondition        metav1.Condition
	}{
		{
			name:                   "known machineless pool removes a stale condition",
			hasMachinePoolMachines: false,
			getMachinesSucceeded:   true,
			initialCondition: &metav1.Condition{
				Type:   clusterv1.MachinePoolMachinesUpToDateCondition,
				Status: metav1.ConditionTrue,
				Reason: "PreviouslyReported",
			},
			expectSet: false,
		},
		{
			name:                   "Machine list failure",
			hasMachinePoolMachines: true,
			getMachinesSucceeded:   false,
			expectSet:              true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolMachinesUpToDateCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachinePoolMachinesUpToDateInternalErrorReason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name:                   "no Machines",
			hasMachinePoolMachines: true,
			getMachinesSucceeded:   true,
			expectSet:              true,
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
			hasMachinePoolMachines: true,
			getMachinesSucceeded:   true,
			expectSet:              true,
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
			hasMachinePoolMachines: true,
			getMachinesSucceeded:   true,
			expectSet:              true,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachinePoolMachinesUpToDateCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachinePoolMachinesUpToDateReason,
			},
		},
		{
			name: "one Machine reports unknown",
			machines: []*clusterv1.Machine{
				machinePoolTestMachine("machine-1", metav1.Condition{
					Type:    clusterv1.MachineUpToDateCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  "NotYetKnown",
					Message: "UpToDate is not yet known",
				}),
			},
			hasMachinePoolMachines: true,
			getMachinesSucceeded:   true,
			expectSet:              true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolMachinesUpToDateCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachinePoolMachinesUpToDateUnknownReason,
				Message: "* Machine machine-1: UpToDate is not yet known",
			},
		},
		{
			name: "older Machine without condition is reported while a new Machine is ignored",
			machines: []*clusterv1.Machine{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "machine-old",
						CreationTimestamp: metav1.NewTime(time.Now().Add(-20 * time.Second)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "machine-new",
						CreationTimestamp: metav1.NewTime(time.Now()),
					},
				},
			},
			hasMachinePoolMachines: true,
			getMachinesSucceeded:   true,
			expectSet:              true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachinePoolMachinesUpToDateCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachinePoolMachinesUpToDateUnknownReason,
				Message: "* Machine machine-old: Condition UpToDate not yet reported",
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
			hasMachinePoolMachines: true,
			getMachinesSucceeded:   true,
			expectSet:              true,
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

			setMachinesUpToDateCondition(ctx, mp, tt.machines, tt.hasMachinePoolMachines, tt.getMachinesSucceeded)

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
