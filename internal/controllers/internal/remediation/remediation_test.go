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

package remediation

import (
	"fmt"
	"sort"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

func TestSortMachinesToRemediate(t *testing.T) {
	unhealthyMachinesWithAnnotations := []*clusterv1.Machine{}
	for i := range 4 {
		unhealthyMachinesWithAnnotations = append(unhealthyMachinesWithAnnotations, &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:              fmt.Sprintf("unhealthy-annotated-machine-%d", i),
				Namespace:         "default",
				CreationTimestamp: metav1.Time{Time: metav1.Now().Add(time.Duration(i) * time.Second)},
				Annotations: map[string]string{
					clusterv1.RemediateMachineAnnotation: "",
				},
			},
			Status: clusterv1.MachineStatus{
				Conditions: []metav1.Condition{
					{
						Type:    clusterv1.MachineOwnerRemediatedCondition,
						Status:  metav1.ConditionFalse,
						Reason:  clusterv1.MachineOwnerRemediatedWaitingForRemediationReason,
						Message: "Waiting for remediation",
					},
					{
						Type:    clusterv1.MachineHealthCheckSucceededCondition,
						Status:  metav1.ConditionFalse,
						Reason:  clusterv1.MachineHealthCheckHasRemediateAnnotationReason,
						Message: "Marked for remediation via cluster.x-k8s.io/remediate-machine annotation",
					},
				},
			},
		})
	}

	unhealthyMachines := []*clusterv1.Machine{}
	for i := range 4 {
		unhealthyMachines = append(unhealthyMachines, &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:              fmt.Sprintf("unhealthy-machine-%d", i),
				Namespace:         "default",
				CreationTimestamp: metav1.Time{Time: metav1.Now().Add(time.Duration(i) * time.Second)},
			},
			Status: clusterv1.MachineStatus{
				Conditions: []metav1.Condition{
					{
						Type:    clusterv1.MachineOwnerRemediatedCondition,
						Status:  metav1.ConditionFalse,
						Reason:  clusterv1.MachineOwnerRemediatedWaitingForRemediationReason,
						Message: "Waiting for remediation",
					},
					{
						Type:    clusterv1.MachineHealthCheckSucceededCondition,
						Status:  metav1.ConditionFalse,
						Reason:  clusterv1.MachineHealthCheckHasRemediateAnnotationReason,
						Message: "Marked for remediation via cluster.x-k8s.io/remediate-machine annotation",
					},
				},
			},
		})
	}

	t.Run("remediation machines should be sorted with newest first", func(t *testing.T) {
		g := NewWithT(t)
		machines := make([]*clusterv1.Machine, len(unhealthyMachines))
		copy(machines, unhealthyMachines)
		SortMachinesToRemediate(machines)
		sort.SliceStable(unhealthyMachines, func(i, j int) bool {
			return unhealthyMachines[i].CreationTimestamp.After(unhealthyMachines[j].CreationTimestamp.Time)
		})
		g.Expect(unhealthyMachines).To(Equal(machines))
	})

	t.Run("remediation machines with annotation should be prioritised over other machines", func(t *testing.T) {
		g := NewWithT(t)

		machines := make([]*clusterv1.Machine, len(unhealthyMachines))
		copy(machines, unhealthyMachines)
		machines = append(machines, unhealthyMachinesWithAnnotations...)
		SortMachinesToRemediate(machines)

		sort.SliceStable(unhealthyMachines, func(i, j int) bool {
			return unhealthyMachines[i].CreationTimestamp.After(unhealthyMachines[j].CreationTimestamp.Time)
		})
		sort.SliceStable(unhealthyMachinesWithAnnotations, func(i, j int) bool {
			return unhealthyMachinesWithAnnotations[i].CreationTimestamp.After(unhealthyMachinesWithAnnotations[j].CreationTimestamp.Time)
		})
		g.Expect(machines).To(Equal(append(unhealthyMachinesWithAnnotations, unhealthyMachines...)))
	})
}
