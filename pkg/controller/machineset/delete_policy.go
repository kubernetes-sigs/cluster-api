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
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

type deletePriority int

const (
	mustDelete   deletePriority = 100
	betterDelete deletePriority = 50
	couldDelete  deletePriority = 20
	// mustNotDelete deletePriority = 0
)

type deletePriorityFunc func(machine *v1alpha1.Machine) deletePriority

func simpleDeletePriority(machine *v1alpha1.Machine) deletePriority {
	if machine.DeletionTimestamp != nil && !machine.DeletionTimestamp.IsZero() {
		return mustDelete
	}
	if machine.Status.ErrorReason != nil || machine.Status.ErrorMessage != nil {
		return betterDelete
	}
	return couldDelete
}

// TODO: Define machines deletion policies.
// see: https://github.com/kubernetes/kube-deploy/issues/625
func getMachinesToDeletePrioritized(filteredMachines []*v1alpha1.Machine, diff int, fun deletePriorityFunc) []*v1alpha1.Machine {
	if diff >= len(filteredMachines) {
		return filteredMachines
	} else if diff <= 0 {
		return []*v1alpha1.Machine{}
	}

	machines := make(map[deletePriority][]*v1alpha1.Machine)

	for _, machine := range filteredMachines {
		priority := fun(machine)
		machines[priority] = append(machines[priority], machine)
	}

	result := []*v1alpha1.Machine{}
	for _, priority := range []deletePriority{
		mustDelete,
		betterDelete,
		couldDelete,
	} {
		result = append(result, machines[priority]...)
		if len(result) >= diff {
			break
		}
	}

	return result[:diff]
}
