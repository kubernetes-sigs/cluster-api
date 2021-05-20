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

// Package util implements utilities.
package util

import (
	"context"

	"github.com/blang/semver"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/cluster-api/util/version"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ParseMajorMinorPatch returns a semver.Version from the string provided
// by looking only at major.minor.patch and stripping everything else out.
//
// Deprecated: Please use the function in util/version.
func ParseMajorMinorPatch(v string) (semver.Version, error) {
	return version.ParseMajorMinorPatchTolerant(v)
}

// GetMachinesForCluster returns a list of machines associated with the cluster.
//
// Deprecated: Please use util/collection GetFilteredMachinesForCluster(ctx, client, cluster).
func GetMachinesForCluster(ctx context.Context, c client.Client, cluster *clusterv1.Cluster) (*clusterv1.MachineList, error) {
	var machines clusterv1.MachineList
	if err := c.List(
		ctx,
		&machines,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels{
			clusterv1.ClusterLabelName: cluster.Name,
		},
	); err != nil {
		return nil, err
	}
	return &machines, nil
}

// GetControlPlaneMachines returns a slice containing control plane machines.
//
// Deprecated: Please use util/collection FromMachines(machine).Filter(collections.ControlPlaneMachines(cluster.Name)).
func GetControlPlaneMachines(machines []*clusterv1.Machine) (res []*clusterv1.Machine) {
	for _, machine := range machines {
		if IsControlPlaneMachine(machine) {
			res = append(res, machine)
		}
	}
	return
}

// GetControlPlaneMachinesFromList returns a slice containing control plane machines.
//
// Deprecated: Please use util/collection FromMachineList(machineList).Filter(collections.ControlPlaneMachines(cluster.Name)).
func GetControlPlaneMachinesFromList(machineList *clusterv1.MachineList) (res []*clusterv1.Machine) {
	for i := 0; i < len(machineList.Items); i++ {
		machine := machineList.Items[i]
		if IsControlPlaneMachine(&machine) {
			res = append(res, &machine)
		}
	}
	return
}
