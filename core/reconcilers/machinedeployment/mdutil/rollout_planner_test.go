/*
Copyright The Kubernetes Authors.

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

package mdutil

import (
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	"sigs.k8s.io/cluster-api/internal/hooks"
)

func TestCheckOrCleanupAcknowledgeMove(t *testing.T) {
	tests := []struct {
		name              string
		ms                *clusterv1.MachineSet
		machine           *clusterv1.Machine
		wantAcknowledged  bool
		wantAnnotationSet bool
	}{
		{
			name: "Machine not updating in place",
			ms:   &clusterv1.MachineSet{},
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Name: "m1"},
			},
			wantAcknowledged:  false,
			wantAnnotationSet: false,
		},
		{
			name: "Machine updating in place but UpdateMachine hook still pending",
			ms:   &clusterv1.MachineSet{},
			machine: machineWithPendingHook(&clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name: "m1",
					Annotations: map[string]string{
						clusterv1.UpdateInProgressAnnotation: "{}",
					},
				},
			}),
			wantAcknowledged:  false,
			wantAnnotationSet: false,
		},
		{
			name: "Machine updating in place, no pending-acknowledge-move annotation",
			ms:   &clusterv1.MachineSet{},
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name: "m1",
					Annotations: map[string]string{
						clusterv1.UpdateInProgressAnnotation: "{}",
					},
				},
			},
			wantAcknowledged:  true,
			wantAnnotationSet: false,
		},
		{
			name: "Machine pending acknowledge, MS not accepting machines from other MachineSets: annotation is dropped",
			ms: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{Name: "ms1"},
			},
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name: "m1",
					Annotations: map[string]string{
						clusterv1.UpdateInProgressAnnotation:       "{}",
						clusterv1.PendingAcknowledgeMoveAnnotation: "true",
					},
				},
			},
			wantAcknowledged:  true,
			wantAnnotationSet: false,
		},
		{
			name: "Machine pending acknowledge, MS accepting machines from other MachineSets but machine not yet acknowledged",
			ms: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ms1",
					Annotations: map[string]string{
						clusterv1.MachineSetReceiveMachinesFromMachineSetsAnnotation: "ms0",
					},
				},
			},
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name: "m1",
					Annotations: map[string]string{
						clusterv1.UpdateInProgressAnnotation:       "{}",
						clusterv1.PendingAcknowledgeMoveAnnotation: "true",
					},
				},
			},
			wantAcknowledged:  false,
			wantAnnotationSet: true,
		},
		{
			name: "Machine pending acknowledge, MS accepting machines from other MachineSets and machine already acknowledged",
			ms: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ms1",
					Annotations: map[string]string{
						clusterv1.MachineSetReceiveMachinesFromMachineSetsAnnotation: "ms0",
						clusterv1.AcknowledgedMoveAnnotation:                         "m1,m2",
					},
				},
			},
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name: "m1",
					Annotations: map[string]string{
						clusterv1.UpdateInProgressAnnotation:       "{}",
						clusterv1.PendingAcknowledgeMoveAnnotation: "true",
					},
				},
			},
			wantAcknowledged:  true,
			wantAnnotationSet: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			acknowledged := CheckOrCleanupAcknowledgeMove(ctx, tt.ms, tt.machine)

			g.Expect(acknowledged).To(Equal(tt.wantAcknowledged))
			_, hasAnnotation := tt.machine.Annotations[clusterv1.PendingAcknowledgeMoveAnnotation]
			g.Expect(hasAnnotation).To(Equal(tt.wantAnnotationSet))
		})
	}
}

func machineWithPendingHook(m *clusterv1.Machine) *clusterv1.Machine {
	hooks.MarkObjectAsPending(m, runtimehooksv1.UpdateMachine)
	return m
}

func TestSyncMachinesPlanner(t *testing.T) {
	tests := []struct {
		name       string
		ms         *clusterv1.MachineSet
		machines   []*clusterv1.Machine
		wantResult SyncMachinesPlannerResult
		wantErr    bool
	}{
		{
			name: "Not enough machines: create missing machines",
			ms: &clusterv1.MachineSet{
				Spec: clusterv1.MachineSetSpec{Replicas: ptr.To[int32](3)},
			},
			machines: []*clusterv1.Machine{
				{ObjectMeta: metav1.ObjectMeta{Name: "m1"}},
			},
			wantResult: SyncMachinesPlannerResult{MachinesToAdd: 2},
		},
		{
			name: "Not enough machines but machine creation disabled",
			ms: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{clusterv1.DisableMachineCreateAnnotation: "true"},
				},
				Spec: clusterv1.MachineSetSpec{Replicas: ptr.To[int32](3)},
			},
			machines: []*clusterv1.Machine{
				{ObjectMeta: metav1.ObjectMeta{Name: "m1"}},
			},
			wantResult: SyncMachinesPlannerResult{MachineCreationDisabled: true},
		},
		{
			name: "Exact number of machines: no-op",
			ms: &clusterv1.MachineSet{
				Spec: clusterv1.MachineSetSpec{Replicas: ptr.To[int32](2)},
			},
			machines: []*clusterv1.Machine{
				{ObjectMeta: metav1.ObjectMeta{Name: "m1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m2"}},
			},
			wantResult: SyncMachinesPlannerResult{},
		},
		{
			name: "Too many machines, no move annotation: delete exceeding machines",
			ms: &clusterv1.MachineSet{
				Spec: clusterv1.MachineSetSpec{Replicas: ptr.To[int32](1)},
			},
			machines: []*clusterv1.Machine{
				{ObjectMeta: metav1.ObjectMeta{Name: "m1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m2"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m3"}},
			},
			wantResult: SyncMachinesPlannerResult{MachinesToDelete: 2},
		},
		{
			name: "Too many machines, move annotation set: move exceeding machines",
			ms: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						clusterv1.MachineSetMoveMachinesToMachineSetAnnotation: `{"name":"ms-target","affectsAvailability":false}`,
					},
				},
				Spec: clusterv1.MachineSetSpec{Replicas: ptr.To[int32](1)},
			},
			machines: []*clusterv1.Machine{
				{ObjectMeta: metav1.ObjectMeta{Name: "m1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m2"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m3"}},
			},
			wantResult: SyncMachinesPlannerResult{
				MachinesToMove:          2,
				MoveTargetMSName:        "ms-target",
				MoveAffectsAvailability: ptr.To(false),
			},
		},
		{
			name: "Too many machines, move annotation set with legacy plain-text format",
			ms: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						clusterv1.MachineSetMoveMachinesToMachineSetAnnotation: "ms-target",
					},
				},
				Spec: clusterv1.MachineSetSpec{Replicas: ptr.To[int32](1)},
			},
			machines: []*clusterv1.Machine{
				{ObjectMeta: metav1.ObjectMeta{Name: "m1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m2"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m3"}},
			},
			wantResult: SyncMachinesPlannerResult{
				MachinesToMove:   2,
				MoveTargetMSName: "ms-target",
			},
		},
		{
			name: "Too many machines, move annotation set but with empty name: fall back to delete",
			ms: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						clusterv1.MachineSetMoveMachinesToMachineSetAnnotation: `{"name":""}`,
					},
				},
				Spec: clusterv1.MachineSetSpec{Replicas: ptr.To[int32](1)},
			},
			machines: []*clusterv1.Machine{
				{ObjectMeta: metav1.ObjectMeta{Name: "m1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m2"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m3"}},
			},
			wantResult: SyncMachinesPlannerResult{MachinesToDelete: 2},
		},
		{
			name: "Too many machines, invalid move annotation: error",
			ms: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						clusterv1.MachineSetMoveMachinesToMachineSetAnnotation: `{invalid`,
					},
				},
				Spec: clusterv1.MachineSetSpec{Replicas: ptr.To[int32](1)},
			},
			machines: []*clusterv1.Machine{
				{ObjectMeta: metav1.ObjectMeta{Name: "m1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m2"}},
			},
			wantErr: true,
		},
		{
			name: "Machine pending acknowledge move is not counted for delete/move computation",
			ms: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						clusterv1.MachineSetReceiveMachinesFromMachineSetsAnnotation: "ms0",
					},
				},
				Spec: clusterv1.MachineSetSpec{Replicas: ptr.To[int32](2)},
			},
			machines: []*clusterv1.Machine{
				{ObjectMeta: metav1.ObjectMeta{Name: "m1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m2"}},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "m3",
						Annotations: map[string]string{
							clusterv1.PendingAcknowledgeMoveAnnotation: "true",
						},
					},
				},
			},
			wantResult: SyncMachinesPlannerResult{
				MachinesPendingAcknowledgeMove: []string{"m3"},
			},
		},
		{
			name: "Machines pending acknowledge move still leave exceeding machines to delete",
			ms: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						clusterv1.MachineSetReceiveMachinesFromMachineSetsAnnotation: "ms0",
					},
				},
				Spec: clusterv1.MachineSetSpec{Replicas: ptr.To[int32](1)},
			},
			machines: []*clusterv1.Machine{
				{ObjectMeta: metav1.ObjectMeta{Name: "m1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m2"}},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "m3",
						Annotations: map[string]string{
							clusterv1.PendingAcknowledgeMoveAnnotation: "true",
						},
					},
				},
			},
			wantResult: SyncMachinesPlannerResult{
				MachinesPendingAcknowledgeMove: []string{"m3"},
				MachinesToDelete:               1,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			res, err := SyncMachinesPlanner(ctx, tt.ms, tt.machines)

			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(res).To(Equal(tt.wantResult))
		})
	}
}
