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

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	addonsv1 "sigs.k8s.io/cluster-api/exp/addons/api/v1beta1"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
)

// DiscoverOptions define options for the discovery process.
type DiscoverOptions struct {
	// ShowOtherConditions is a list of comma separated kind or kind/name for which we should add the ShowObjectConditionsAnnotation
	// to signal to the presentation layer to show all the conditions for the objects.
	ShowOtherConditions string

	// ShowMachineSets instructs the discovery process to include machine sets in the ObjectTree.
	ShowMachineSets bool

	// ShowClusterResourceSets instructs the discovery process to include cluster resource sets in the ObjectTree.
	ShowClusterResourceSets bool

	// ShowTemplates instructs the discovery process to include infrastructure and bootstrap config templates in the ObjectTree.
	ShowTemplates bool

	// AddTemplateVirtualNode instructs the discovery process to group template under a virtual node.
	AddTemplateVirtualNode bool

	// Echo displays MachineInfrastructure or BootstrapConfig objects if the object's ready condition is true
	Echo bool

	// Grouping groups machine objects in case the ready conditions
	// have the same Status, Severity and Reason.
	Grouping bool

	// V1Beta2 instructs tree to use V1Beta2 conditions.
	V1Beta2 bool
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
	clusterInfra, err := external.Get(ctx, c, cluster.Spec.InfrastructureRef)
	if err != nil {
		return nil, errors.Wrap(err, "get InfraCluster reference from Cluster")
	}
	tree.Add(cluster, clusterInfra, ObjectMetaName("ClusterInfrastructure"))

	if options.ShowClusterResourceSets {
		addClusterResourceSetsToObjectTree(ctx, c, cluster, tree)
	}

	// Adds control plane
	controlPlane, err := external.Get(ctx, c, cluster.Spec.ControlPlaneRef)
	if err == nil {
		// Keep track that this objects abides to the Cluster API control plane contract,
		// so the consumers of the ObjectTree will know which info are available on this unstructured object
		// and how to extract them.
		addAnnotation(controlPlane, ObjectContractAnnotation, "ControlPlane")
		addControlPlane(cluster, controlPlane, tree, options)
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
			if (m.Spec.InfrastructureRef != corev1.ObjectReference{}) {
				if machineInfra, err := external.Get(ctx, c, &m.Spec.InfrastructureRef); err == nil {
					tree.Add(m, machineInfra, ObjectMetaName("MachineInfrastructure"), NoEcho(true))
				}
			}

			if m.Spec.Bootstrap.ConfigRef != nil {
				if machineBootstrap, err := external.Get(ctx, c, m.Spec.Bootstrap.ConfigRef); err == nil {
					tree.Add(m, machineBootstrap, ObjectMetaName("BootstrapConfig"), NoEcho(true))
				}
			}
		}
	}

	controlPlaneMachines := selectControlPlaneMachines(machinesList)
	if controlPlane != nil {
		for i := range controlPlaneMachines {
			cp := controlPlaneMachines[i]
			addMachineFunc(controlPlane, cp)
		}
	}

	machinePoolList, err := getMachinePoolsInCluster(ctx, c, cluster.Namespace, cluster.Name)
	if err != nil {
		return nil, err
	}

	workers := VirtualObject(cluster.Namespace, "WorkerGroup", "Workers")
	// Add WorkerGroup if there are MachineDeployments or MachinePools
	if len(machinesList.Items) != len(controlPlaneMachines) || len(machinePoolList.Items) > 0 {
		tree.Add(cluster, workers)
	}

	if len(machinesList.Items) != len(controlPlaneMachines) { // Add MachineDeployment objects
		tree.Add(cluster, workers)
		err = addMachineDeploymentToObjectTree(ctx, c, cluster, workers, machinesList, tree, options, addMachineFunc)
		if err != nil {
			return nil, err
		}
	}

	if len(machinePoolList.Items) > 0 { // Add MachinePool objects
		tree.Add(cluster, workers)
		addMachinePoolsToObjectTree(ctx, c, workers, machinePoolList, machinesList, tree, addMachineFunc)
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

func addClusterResourceSetsToObjectTree(ctx context.Context, c client.Client, cluster *clusterv1.Cluster, tree *ObjectTree) {
	if resourceSetBinding, err := getResourceSetBindingInCluster(ctx, c, cluster.Namespace, cluster.Name); err == nil {
		resourceSetGroup := VirtualObject(cluster.Namespace, "ClusterResourceSetGroup", "ClusterResourceSets")
		tree.Add(cluster, resourceSetGroup)

		for _, binding := range resourceSetBinding.Spec.Bindings {
			resourceSetRefObject := ObjectReferenceObject(&corev1.ObjectReference{
				Kind:       "ClusterResourceSet",
				Namespace:  cluster.Namespace,
				Name:       binding.ClusterResourceSetName,
				APIVersion: addonsv1.GroupVersion.String(),
			})
			tree.Add(resourceSetGroup, resourceSetRefObject)
		}
	}
}

func addControlPlane(cluster *clusterv1.Cluster, controlPlane *unstructured.Unstructured, tree *ObjectTree, options DiscoverOptions) {
	tree.Add(cluster, controlPlane, ObjectMetaName("ControlPlane"), GroupingObject(true))

	if options.ShowTemplates {
		// Add control plane infrastructure ref using spec fields guaranteed in contract
		infrastructureRef, found, err := unstructured.NestedMap(controlPlane.UnstructuredContent(), "spec", "machineTemplate", "infrastructureRef")
		if err == nil && found {
			infrastructureObjectRef := &corev1.ObjectReference{
				Kind:       infrastructureRef["kind"].(string),
				Namespace:  infrastructureRef["namespace"].(string),
				Name:       infrastructureRef["name"].(string),
				APIVersion: infrastructureRef["apiVersion"].(string),
			}

			machineTemplateRefObject := ObjectReferenceObject(infrastructureObjectRef)
			var templateParent client.Object
			if options.AddTemplateVirtualNode {
				templateParent = addTemplateVirtualNode(tree, controlPlane, cluster.Namespace)
			} else {
				templateParent = controlPlane
			}
			tree.Add(templateParent, machineTemplateRefObject, ObjectMetaName("MachineInfrastructureTemplate"))
		}
	}
}

func addMachineDeploymentToObjectTree(ctx context.Context, c client.Client, cluster *clusterv1.Cluster, workers *NodeObject, machinesList *clusterv1.MachineList, tree *ObjectTree, options DiscoverOptions, addMachineFunc func(parent client.Object, m *clusterv1.Machine)) error {
	// Adds worker machines.
	machinesDeploymentList, err := getMachineDeploymentsInCluster(ctx, c, cluster.Namespace, cluster.Name)
	if err != nil {
		return err
	}

	machineSetList, err := getMachineSetsInCluster(ctx, c, cluster.Namespace, cluster.Name)
	if err != nil {
		return err
	}

	for i := range machinesDeploymentList.Items {
		md := &machinesDeploymentList.Items[i]
		addOpts := make([]AddObjectOption, 0)
		if !options.ShowMachineSets {
			addOpts = append(addOpts, GroupingObject(true))
		}
		tree.Add(workers, md, addOpts...)

		if options.ShowTemplates {
			var templateParent client.Object
			if options.AddTemplateVirtualNode {
				templateParent = addTemplateVirtualNode(tree, md, cluster.Namespace)
			} else {
				templateParent = md
			}

			// md.Spec.Template.Spec.Bootstrap.ConfigRef is optional
			if md.Spec.Template.Spec.Bootstrap.ConfigRef != nil {
				bootstrapTemplateRefObject := ObjectReferenceObject(md.Spec.Template.Spec.Bootstrap.ConfigRef)
				tree.Add(templateParent, bootstrapTemplateRefObject, ObjectMetaName("BootstrapConfigTemplate"))
			}

			machineTemplateRefObject := ObjectReferenceObject(&md.Spec.Template.Spec.InfrastructureRef)
			tree.Add(templateParent, machineTemplateRefObject, ObjectMetaName("MachineInfrastructureTemplate"))
		}

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

	return nil
}

func addMachinePoolsToObjectTree(ctx context.Context, c client.Client, workers *NodeObject, machinePoolList *expv1.MachinePoolList, machinesList *clusterv1.MachineList, tree *ObjectTree, addMachineFunc func(parent client.Object, m *clusterv1.Machine)) {
	for i := range machinePoolList.Items {
		mp := &machinePoolList.Items[i]
		_, visible := tree.Add(workers, mp, GroupingObject(true))

		if visible {
			if machinePoolBootstrap, err := external.Get(ctx, c, mp.Spec.Template.Spec.Bootstrap.ConfigRef); err == nil {
				tree.Add(mp, machinePoolBootstrap, ObjectMetaName("BootstrapConfig"), NoEcho(true))
			}

			if machinePoolInfra, err := external.Get(ctx, c, &mp.Spec.Template.Spec.InfrastructureRef); err == nil {
				tree.Add(mp, machinePoolInfra, ObjectMetaName("MachinePoolInfrastructure"), NoEcho(true))
			}
		}

		machines := selectMachinesControlledBy(machinesList, mp)
		for _, m := range machines {
			addMachineFunc(mp, m)
		}
	}
}

func getResourceSetBindingInCluster(ctx context.Context, c client.Client, namespace string, name string) (*addonsv1.ClusterResourceSetBinding, error) {
	if name == "" {
		return nil, nil
	}

	resourceSetBinding := &addonsv1.ClusterResourceSetBinding{}
	resourceSetBindingKey := client.ObjectKey{Namespace: namespace, Name: name}
	if err := c.Get(ctx, resourceSetBindingKey, resourceSetBinding); err != nil {
		return nil, err
	}
	resourceSetBinding.TypeMeta = metav1.TypeMeta{
		Kind:       "ClusterResourceSetBinding",
		APIVersion: addonsv1.GroupVersion.String(),
	}

	return resourceSetBinding, nil
}

func getMachinesInCluster(ctx context.Context, c client.Client, namespace, name string) (*clusterv1.MachineList, error) {
	if name == "" {
		return nil, nil
	}

	machineList := &clusterv1.MachineList{}
	labels := map[string]string{clusterv1.ClusterNameLabel: name}

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
	labels := map[string]string{clusterv1.ClusterNameLabel: name}

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
	labels := map[string]string{clusterv1.ClusterNameLabel: name}

	if err := c.List(ctx, machineSetList, client.InNamespace(namespace), client.MatchingLabels(labels)); err != nil {
		return nil, err
	}

	return machineSetList, nil
}

func getMachinePoolsInCluster(ctx context.Context, c client.Client, namespace, name string) (*expv1.MachinePoolList, error) {
	if name == "" {
		return nil, nil
	}

	machinePoolList := &expv1.MachinePoolList{}
	labels := map[string]string{clusterv1.ClusterNameLabel: name}

	if err := c.List(ctx, machinePoolList, client.InNamespace(namespace), client.MatchingLabels(labels)); err != nil {
		return nil, err
	}

	return machinePoolList, nil
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

func addTemplateVirtualNode(tree *ObjectTree, parent client.Object, namespace string) client.Object {
	templateNode := VirtualObject(namespace, "TemplateGroup", parent.GetName())
	addOpts := []AddObjectOption{
		ZOrder(1),
		ObjectMetaName("Templates"),
	}
	tree.Add(parent, templateNode, addOpts...)

	return templateNode
}
