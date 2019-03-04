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
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (c *ReconcileMachineSet) getMachineSetsForMachine(m *v1alpha1.Machine) []*v1alpha1.MachineSet {
	if len(m.Labels) == 0 {
		klog.Warningf("No machine sets found for Machine %v because it has no labels", m.Name)
		return nil
	}

	msList := &v1alpha1.MachineSetList{}
	listOptions := &client.ListOptions{
		Namespace: m.Namespace,
	}

	err := c.Client.List(context.Background(), listOptions, msList)
	if err != nil {
		klog.Errorf("Failed to list machine sets, %v", err)
		return nil
	}

	var mss []*v1alpha1.MachineSet
	for idx := range msList.Items {
		ms := &msList.Items[idx]
		if hasMatchingLabels(ms, m) {
			mss = append(mss, ms)
		}
	}

	return mss
}

func hasMatchingLabels(machineSet *v1alpha1.MachineSet, machine *v1alpha1.Machine) bool {
	selector, err := metav1.LabelSelectorAsSelector(&machineSet.Spec.Selector)
	if err != nil {
		klog.Warningf("unable to convert selector: %v", err)
		return false
	}

	// If a deployment with a nil or empty selector creeps in, it should match nothing, not everything.
	if selector.Empty() {
		klog.V(2).Infof("%v machineset has empty selector", machineSet.Name)
		return false
	}

	if !selector.Matches(labels.Set(machine.Labels)) {
		klog.V(4).Infof("%v machine has mismatch labels", machine.Name)
		return false
	}

	return true
}
