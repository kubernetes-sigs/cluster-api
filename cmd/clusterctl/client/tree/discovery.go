/*
Copyright 2020 The Kubernetes Authors.

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

package tree

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/util"
)

// DiscoverOptions define options for the discovery process.
type DiscoverOptions struct {
	// ShowOtherConditions is a list of comma separated kind or kind/name for which we should add the ShowObjectConditionsAnnotation
	// to signal to the presentation layer to show all the conditions for the objects.
	ShowOtherConditions string

	// ShowMachineSets instructs the discovery process to include machine sets in the ObjectTree.
	ShowMachineSets bool

	// Echo displays MachineInfrastructure or BootstrapConfig objects if the object's ready condition is true
	// or it has the same Status, Severity and Reason of the parent's object ready condition (it is an echo)
	Echo bool

	// Grouping groups machine objects in case the ready conditions
	// have the same Status, Severity and Reason.
	Grouping bool
}

func (d DiscoverOptions) toObjectTreeOptions() ObjectTreeOptions {
	return ObjectTreeOptions(d)
}

// Discovery returns an object tree representing the status of a Cluster API cluster.
func Discovery(ctx context.Context, c client.Client, namespace, name string, options DiscoverOptions) (*ObjectTree, error) {
	// Fetch the Cluster instance.
	cluster := &clusterv1.Cluster{}
	clusterKey := client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}
	if err := c.Get(ctx, clusterKey, cluster); err != nil {
		return nil, err
	}

	// Enforce TypeMeta to make sure checks on GVK works properly.
	cluster.TypeMeta = metav1.TypeMeta{
		Kind:       "Cluster",
		APIVersion: clusterv1.GroupVersion.String(),
	}

	// Create an object tree with the cluster as root
	tree := NewObjectTree(cluster, options.toObjectTreeOptions())

	// Adds cluster infra
	if clusterInfra, err := external.Get(ctx, c, cluster.Spec.InfrastructureRef, cluster.Namespace); err == nil {
		tree.Add(cluster, clusterInfra, ObjectMetaName("ClusterInfrastructure"))
	}

	// Adds control plane
	controlPLane, err := external.Get(ctx, c, cluster.Spec.ControlPlaneRef, cluster.Namespace)
	if err == nil {
		tree.Add(cluster, controlPLane, ObjectMetaName("ControlPlane"), GroupingObject(true))
	}

	// Adds control plane machines.
	machinesList, err := getMachinesInCluster(ctx, c, cluster.Namespace, cluster.Name)
	if err != nil {
		return nil, err
	}
	machineMap := map[string]bool{}
	addMachineFunc := func(parent client.Object, m *clusterv1.Machine) {
		_, visible := tree.Add(parent, m)
		machineMap[m.Name] = true

		if visible {
			if machineInfra, err := external.Get(ctx, c, &m.Spec.InfrastructureRef, cluster.Namespace); err == nil {
				tree.Add(m, machineInfra, ObjectMetaName("MachineInfrastructure"), NoEcho(true))
			}

			if machineBootstrap, err := external.Get(ctx, c, m.Spec.Bootstrap.ConfigRef, cluster.Namespace); err == nil {
				tree.Add(m, machineBootstrap, ObjectMetaName("BootstrapConfig"), NoEcho(true))
			}
		}
	}

	controlPlaneMachines := selectControlPlaneMachines(machinesList)
	for i := range controlPlaneMachines {
		cp := controlPlaneMachines[i]
		addMachineFunc(controlPLane, cp)
	}

	if len(machinesList.Items) == len(controlPlaneMachines) {
		return tree, nil
	}

	workers := VirtualObject(cluster.Namespace, "WorkerGroup", "Workers")
	tree.Add(cluster, workers)

	// Adds worker machines.
	machinesDeploymentList, err := getMachineDeploymentsInCluster(ctx, c, cluster.Namespace, cluster.Name)
	if err != nil {
		return nil, err
	}

	machineSetList, err := getMachineSetsInCluster(ctx, c, cluster.Namespace, cluster.Name)
	if err != nil {
		return nil, err
	}

	for i := range machinesDeploymentList.Items {
		md := &machinesDeploymentList.Items[i]
		addOpts := make([]AddObjectOption, 0)
		if !options.ShowMachineSets {
			addOpts = append(addOpts, GroupingObject(true))
		}
		tree.Add(workers, md, addOpts...)

		machineSets := selectMachinesSetsControlledBy(machineSetList, md)
		for i := range machineSets {
			ms := machineSets[i]

			var parent client.Object = md
			if options.ShowMachineSets {
				tree.Add(md, ms, GroupingObject(true))
				parent = ms
			}

			machines := selectMachinesControlledBy(machinesList, ms)
			for _, w := range machines {
				addMachineFunc(parent, w)
			}
		}
	}

	// Handles orphan machines.
	if len(machineMap) < len(machinesList.Items) {
		other := VirtualObject(cluster.Namespace, "OtherGroup", "Other")
		tree.Add(workers, other)

		for i := range machinesList.Items {
			m := &machinesList.Items[i]
			if _, ok := machineMap[m.Name]; ok {
				continue
			}
			addMachineFunc(other, m)
		}
	}

	return tree, nil
}

func getMachinesInCluster(ctx context.Context, c client.Client, namespace, name string) (*clusterv1.MachineList, error) {
	if name == "" {
		return nil, nil
	}

	machineList := &clusterv1.MachineList{}
	labels := map[string]string{clusterv1.ClusterLabelName: name}

	if err := c.List(ctx, machineList, client.InNamespace(namespace), client.MatchingLabels(labels)); err != nil {
		return nil, err
	}

	return machineList, nil
}

func getMachineDeploymentsInCluster(ctx context.Context, c client.Client, namespace, name string) (*clusterv1.MachineDeploymentList, error) {
	if name == "" {
		return nil, nil
	}

	machineDeploymentList := &clusterv1.MachineDeploymentList{}
	labels := map[string]string{clusterv1.ClusterLabelName: name}

	if err := c.List(ctx, machineDeploymentList, client.InNamespace(namespace), client.MatchingLabels(labels)); err != nil {
		return nil, err
	}

	return machineDeploymentList, nil
}

func getMachineSetsInCluster(ctx context.Context, c client.Client, namespace, name string) (*clusterv1.MachineSetList, error) {
	if name == "" {
		return nil, nil
	}

	machineSetList := &clusterv1.MachineSetList{}
	labels := map[string]string{clusterv1.ClusterLabelName: name}

	if err := c.List(ctx, machineSetList, client.InNamespace(namespace), client.MatchingLabels(labels)); err != nil {
		return nil, err
	}

	return machineSetList, nil
}

func selectControlPlaneMachines(machineList *clusterv1.MachineList) []*clusterv1.Machine {
	machines := []*clusterv1.Machine{}
	for i := range machineList.Items {
		m := &machineList.Items[i]
		if util.IsControlPlaneMachine(m) {
			machines = append(machines, m)
		}
	}
	return machines
}

func selectMachinesSetsControlledBy(machineSetList *clusterv1.MachineSetList, controller client.Object) []*clusterv1.MachineSet {
	machineSets := []*clusterv1.MachineSet{}
	for i := range machineSetList.Items {
		m := &machineSetList.Items[i]
		if util.IsControlledBy(m, controller) {
			machineSets = append(machineSets, m)
		}
	}
	return machineSets
}

func selectMachinesControlledBy(machineList *clusterv1.MachineList, controller client.Object) []*clusterv1.Machine {
	machines := []*clusterv1.Machine{}
	for i := range machineList.Items {
		m := &machineList.Items[i]
		if util.IsControlledBy(m, controller) {
			machines = append(machines, m)
		}
	}
	return machines
}
