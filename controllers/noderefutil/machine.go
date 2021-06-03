/*
Copyright 2021 The Kubernetes Authors.

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

package noderefutil

import (
	"context"

	"github.com/pkg/errors"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetMachineFromNode retrieves the machine with a nodeRef to nodeName
// There should at most one machine with a given nodeRef, returns an error otherwise.
func GetMachineFromNode(ctx context.Context, c client.Client, nodeName string) (*clusterv1.Machine, error) {
	machineList := &clusterv1.MachineList{}
	if err := c.List(
		ctx,
		machineList,
		client.MatchingFields{clusterv1.MachineNodeNameIndex: nodeName},
	); err != nil {
		return nil, errors.Wrap(err, "failed getting machine list")
	}
	// TODO(vincepri): Remove this loop once controller runtime fake client supports
	// adding indexes on objects.
	items := []*clusterv1.Machine{}
	for i := range machineList.Items {
		machine := &machineList.Items[i]
		if machine.Status.NodeRef != nil && machine.Status.NodeRef.Name == nodeName {
			items = append(items, machine)
		}
	}
	if len(items) != 1 {
		return nil, errors.Errorf("expecting one machine for node %v, got %v", nodeName, machineNames(items))
	}
	return items[0], nil
}

func machineNames(machines []*clusterv1.Machine) []string {
	result := make([]string, 0, len(machines))
	for _, m := range machines {
		result = append(result, m.Name)
	}
	return result
}
