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

package controllers

import (
	"math"
	"sort"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/internal/mdutil"
	"sigs.k8s.io/cluster-api/util/collections"
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
	if machine.Status.NodeRef == nil {
		return mustDelete
	}
	if machine.Status.FailureReason != nil || machine.Status.FailureMessage != nil {
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
	if machine.Status.NodeRef == nil {
		return mustDelete
	}
	if machine.Status.FailureReason != nil || machine.Status.FailureMessage != nil {
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
	if machine.Status.NodeRef == nil {
		return betterDelete
	}
	if machine.Status.FailureReason != nil || machine.Status.FailureMessage != nil {
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
	return m.priority(m.machines[j]) < m.priority(m.machines[i]) // high to low
}

func getMachinesToDeletePrioritized(filteredMachines []*clusterv1.Machine, diff int, fun deletePriorityFunc, inFailureDomain bool, failureDomains clusterv1.FailureDomains) ([]*clusterv1.Machine, error) {
	var err error
	if diff >= len(filteredMachines) {
		return filteredMachines, nil
	} else if diff <= 0 {
		return []*clusterv1.Machine{}, nil
	}
	if inFailureDomain {
		var machinesToDelete []*clusterv1.Machine
		machineCollection := collections.FromMachines(filteredMachines...)
		for i := 0; i < diff; i++ {
			// If a machine meets the basic deletion criteria (e.g. has deletion annotation) then remove it first.
			machineToMark := mdutil.MachineBasicDeletionCriteria(machineCollection)
			// If the basic criteria is not met, pick a machine from a FD with the most machines.
			if machineToMark == nil {
				machineToMark, err = mdutil.MachineInFailureDomainWithMostMachines(failureDomains, machineCollection)
				if err != nil {
					return nil, err
				}
			}
			machinesToDelete = append(machinesToDelete, machineToMark)
			// remove selected machine from the collection so the next itertion of the loop recalculates balance of machines across FDs.
			machineCollection = machineCollection.Difference(collections.FromMachines(machineToMark))
		}
		return machinesToDelete, nil
	} else {
		sortable := sortableMachines{
			machines: filteredMachines,
			priority: fun,
		}
		sort.Sort(sortable)
		return sortable.machines[:diff], nil
	}
}

func getDeletePriorityFunc(ms *clusterv1.MachineSet) (deletePriorityFunc, bool, error) {
	// Map the Spec.DeletePolicy value to the appropriate delete priority function
	switch msdp := clusterv1.MachineSetDeletePolicy(ms.Spec.DeletePolicy); msdp {
	case clusterv1.RandomMachineSetDeletePolicy:
		return randomDeletePolicy, false, nil
	case clusterv1.NewestMachineSetDeletePolicy:
		return newestDeletePriority, false, nil
	case clusterv1.OldestMachineSetDeletePolicy:
		return oldestDeletePriority, false, nil
	case clusterv1.FailureDomainMachineSetDeletePolicy:
		return nil, true, nil
	case "":
		return randomDeletePolicy, false, nil
	default:
		return nil, false, errors.Errorf("Unsupported delete policy %s. Must be one of 'Random', 'Newest', 'Oldest' or 'FailureDomain'", msdp)
	}
}
