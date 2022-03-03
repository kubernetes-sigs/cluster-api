/*
Copyright 2018 The Kubernetes Authors.

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
	"math"
	"sort"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/conditions"
)

type (
	deletePriority     float64
	deletePriorityFunc func(machine *clusterv1.Machine) deletePriority
)

const (
	mustDelete    deletePriority = 100.0
	betterDelete  deletePriority = 50.0
	couldDelete   deletePriority = 20.0
	mustNotDelete deletePriority = 0.0

	secondsPerTenDays float64 = 864000
)

// maps the creation timestamp onto the 0-100 priority range.
func oldestDeletePriority(machine *clusterv1.Machine) deletePriority {
	if !machine.DeletionTimestamp.IsZero() {
		return mustDelete
	}
	if _, ok := machine.ObjectMeta.Annotations[clusterv1.DeleteMachineAnnotation]; ok {
		return mustDelete
	}
	if !isMachineHealthy(machine) {
		return mustDelete
	}
	if machine.ObjectMeta.CreationTimestamp.Time.IsZero() {
		return mustNotDelete
	}
	d := metav1.Now().Sub(machine.ObjectMeta.CreationTimestamp.Time)
	if d.Seconds() < 0 {
		return mustNotDelete
	}
	return deletePriority(float64(mustDelete) * (1.0 - math.Exp(-d.Seconds()/secondsPerTenDays)))
}

func newestDeletePriority(machine *clusterv1.Machine) deletePriority {
	if !machine.DeletionTimestamp.IsZero() {
		return mustDelete
	}
	if _, ok := machine.ObjectMeta.Annotations[clusterv1.DeleteMachineAnnotation]; ok {
		return mustDelete
	}
	if !isMachineHealthy(machine) {
		return mustDelete
	}
	return mustDelete - oldestDeletePriority(machine)
}

func randomDeletePolicy(machine *clusterv1.Machine) deletePriority {
	if !machine.DeletionTimestamp.IsZero() {
		return mustDelete
	}
	if _, ok := machine.ObjectMeta.Annotations[clusterv1.DeleteMachineAnnotation]; ok {
		return betterDelete
	}
	if !isMachineHealthy(machine) {
		return betterDelete
	}
	return couldDelete
}

type sortableMachines struct {
	machines []*clusterv1.Machine
	priority deletePriorityFunc
}

func (m sortableMachines) Len() int      { return len(m.machines) }
func (m sortableMachines) Swap(i, j int) { m.machines[i], m.machines[j] = m.machines[j], m.machines[i] }
func (m sortableMachines) Less(i, j int) bool {
	priorityI, priorityJ := m.priority(m.machines[i]), m.priority(m.machines[j])
	if priorityI == priorityJ {
		// In cases where the priority is identical, it should be ensured that the same machine order is returned each time.
		// Ordering by name is a simple way to do this.
		return m.machines[i].Name < m.machines[j].Name
	}
	return priorityJ < priorityI // high to low
}

func getMachinesToDeletePrioritized(filteredMachines []*clusterv1.Machine, diff int, fun deletePriorityFunc) []*clusterv1.Machine {
	if diff >= len(filteredMachines) {
		return filteredMachines
	} else if diff <= 0 {
		return []*clusterv1.Machine{}
	}

	sortable := sortableMachines{
		machines: filteredMachines,
		priority: fun,
	}
	sort.Sort(sortable)

	return sortable.machines[:diff]
}

func getDeletePriorityFunc(ms *clusterv1.MachineSet) (deletePriorityFunc, error) {
	// Map the Spec.DeletePolicy value to the appropriate delete priority function
	switch msdp := clusterv1.MachineSetDeletePolicy(ms.Spec.DeletePolicy); msdp {
	case clusterv1.RandomMachineSetDeletePolicy:
		return randomDeletePolicy, nil
	case clusterv1.NewestMachineSetDeletePolicy:
		return newestDeletePriority, nil
	case clusterv1.OldestMachineSetDeletePolicy:
		return oldestDeletePriority, nil
	case "":
		return randomDeletePolicy, nil
	default:
		return nil, errors.Errorf("Unsupported delete policy %s. Must be one of 'Random', 'Newest', or 'Oldest'", msdp)
	}
}

func isMachineHealthy(machine *clusterv1.Machine) bool {
	if machine.Status.NodeRef == nil {
		return false
	}
	if machine.Status.FailureReason != nil || machine.Status.FailureMessage != nil {
		return false
	}
	nodeHealthyCondition := conditions.Get(machine, clusterv1.MachineNodeHealthyCondition)
	if nodeHealthyCondition != nil && nodeHealthyCondition.Status != corev1.ConditionTrue {
		return false
	}
	return true
}
