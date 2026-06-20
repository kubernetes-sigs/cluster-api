/*
Copyright 2025 The Kubernetes Authors.

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

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

func TestSetReplicas(t *testing.T) {
	tests := []struct {
		name                   string
		hasMachinePoolMachines bool
		machinePool            *clusterv1.MachinePool
		machines               []*clusterv1.Machine
		wantReady              *int32
		wantAvailable          *int32
		wantUpToDate           *int32
	}{
		{
			name:                   "provider does not support individual machines: copy from status",
			hasMachinePoolMachines: false,
			machinePool: &clusterv1.MachinePool{
				Spec: clusterv1.MachinePoolSpec{
					Replicas: ptr.To[int32](3),
				},
				Status: clusterv1.MachinePoolStatus{
					Replicas: ptr.To[int32](3),
				},
			},
			wantReady:     ptr.To[int32](3),
			wantAvailable: ptr.To[int32](3),
			wantUpToDate:  ptr.To[int32](3),
		},
		{
			name:                   "provider supports individual machines: no machines listed",
			hasMachinePoolMachines: true,
			machinePool:            &clusterv1.MachinePool{},
			machines:               []*clusterv1.Machine{},
			wantReady:              ptr.To[int32](0),
			wantAvailable:          ptr.To[int32](0),
			wantUpToDate:           ptr.To[int32](0),
		},
		{
			name:                   "provider supports individual machines: list machines",
			hasMachinePoolMachines: true,
			machinePool:            &clusterv1.MachinePool{},
			machines: []*clusterv1.Machine{
				machineWithConditions(
					condition(clusterv1.MachineReadyCondition, true),
					condition(clusterv1.MachineAvailableCondition, true),
					condition(clusterv1.MachineUpToDateCondition, true),
				),
				machineWithConditions(
					condition(clusterv1.MachineReadyCondition, true),
					condition(clusterv1.MachineAvailableCondition, false),
					condition(clusterv1.MachineUpToDateCondition, false),
				),
				machineWithConditions(
					condition(clusterv1.MachineReadyCondition, false),
					condition(clusterv1.MachineAvailableCondition, false),
					condition(clusterv1.MachineUpToDateCondition, false),
				),
			},
			wantReady:     ptr.To[int32](2),
			wantAvailable: ptr.To[int32](1),
			wantUpToDate:  ptr.To[int32](1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			setReplicas(tt.machinePool, tt.hasMachinePoolMachines, tt.machines)

			g.Expect(tt.machinePool.Status.ReadyReplicas).To(Equal(tt.wantReady))
			g.Expect(tt.machinePool.Status.AvailableReplicas).To(Equal(tt.wantAvailable))
			g.Expect(tt.machinePool.Status.UpToDateReplicas).To(Equal(tt.wantUpToDate))
		})
	}
}

func machineWithConditions(conditions ...metav1.Condition) *clusterv1.Machine {
	return &clusterv1.Machine{
		Status: clusterv1.MachineStatus{
			Conditions: conditions,
		},
	}
}

func condition(condType string, status bool) metav1.Condition {
	conditionStatus := metav1.ConditionTrue
	if !status {
		conditionStatus = metav1.ConditionFalse
	}
	return metav1.Condition{
		Type:   condType,
		Status: conditionStatus,
	}
}
