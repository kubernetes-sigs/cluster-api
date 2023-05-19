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

package cluster

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/version"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	logf "sigs.k8s.io/cluster-api/cmd/clusterctl/log"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/yaml"
)

// ResourceMutatorFunc holds the type for mutators to be applied on resources during a move operation.
type ResourceMutatorFunc func(u *unstructured.Unstructured) error

// ObjectMover defines methods for moving Cluster API objects to another management cluster.
type ObjectMover interface {
	// Move moves all the Cluster API objects existing in a namespace (or from all the namespaces if empty) to a target management cluster.
	Move(namespace string, toCluster Client, dryRun bool, mutators ...ResourceMutatorFunc) error

	// ToDirectory writes all the Cluster API objects existing in a namespace (or from all the namespaces if empty) to a target directory.
	ToDirectory(namespace string, directory string) error

	// FromDirectory reads all the Cluster API objects existing in a configured directory to a target management cluster.
	FromDirectory(toCluster Client, directory string) error
}

// objectMover implements the ObjectMover interface.
type objectMover struct {
	fromProxy             Proxy
	fromProviderInventory InventoryClient
	dryRun                bool
}

// ensure objectMover implements the ObjectMover interface.
var _ ObjectMover = &objectMover{}

func (o *objectMover) Move(namespace string, toCluster Client, dryRun bool, mutators ...ResourceMutatorFunc) error {
	log := logf.Log
	log.Info("Performing move...")
	o.dryRun = dryRun
	if o.dryRun {
		log.Info("********************************************************")
		log.Info("This is a dry-run move, will not perform any real action")
		log.Info("********************************************************")
	}

	// checks that all the required providers in place in the target cluster.
	if !o.dryRun {
		if err := o.checkTargetProviders(toCluster.ProviderInventory()); err != nil {
			return errors.Wrap(err, "failed to check providers in target cluster")
		}
	}

	objectGraph, err := o.getObjectGraph(namespace)
	if err != nil {
		return errors.Wrap(err, "failed to get object graph")
	}

	// Move the objects to the target cluster.
	var proxy Proxy
	if !o.dryRun {
		proxy = toCluster.Proxy()
	}

	return o.move(objectGraph, proxy, mutators...)
}

func (o *objectMover) ToDirectory(namespace string, directory string) error {
	log := logf.Log
	log.Info("Moving to directory...")

	objectGraph, err := o.getObjectGraph(namespace)
	if err != nil {
		return errors.Wrap(err, "failed to get object graph")
	}

	return o.toDirectory(objectGraph, directory)
}

func (o *objectMover) FromDirectory(toCluster Client, directory string) error {
	log := logf.Log
	log.Info("Moving from directory...")

	// Build an empty object graph used for the fromDirectory sequence not tied to a specific namespace
	objectGraph := newObjectGraph(o.fromProxy, o.fromProviderInventory)

	// Gets all the types defined by the CRDs installed by clusterctl plus the ConfigMap/Secret core types.
	err := objectGraph.getDiscoveryTypes()
	if err != nil {
		return errors.Wrap(err, "failed to retrieve discovery types")
	}

	objs, err := o.filesToObjs(directory)
	if err != nil {
		return errors.Wrap(err, "failed to process object files")
	}

	for i := range objs {
		if err = objectGraph.addRestoredObj(&objs[i]); err != nil {
			return err
		}
	}

	// Completes rebuilding the graph from file by searching for soft ownership relations such as secrets linked to the cluster
	// by a naming convention (without any explicit OwnerReference).
	objectGraph.setSoftOwnership()

	// Completes the graph by setting for each node the list of tenants the node belongs to.
	objectGraph.setTenants()

	// Check whether nodes are not included in GVK considered for fromDirectory.
	objectGraph.checkVirtualNode()

	// Restore the objects to the target cluster.
	proxy := toCluster.Proxy()

	return o.fromDirectory(objectGraph, proxy)
}

func (o *objectMover) filesToObjs(dir string) ([]unstructured.Unstructured, error) {
	log := logf.Log
	log.Info(fmt.Sprintf("Restoring files from %s", dir))

	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	rawYAMLs := make([][]byte, 0)
	for i := range files {
		path := filepath.Clean(filepath.Join(dir, files[i].Name()))

		byObj, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}

		rawYAMLs = append(rawYAMLs, byObj)
	}

	processedYAMLs := yaml.JoinYaml(rawYAMLs...)

	objs, err := yaml.ToUnstructured(processedYAMLs)
	if err != nil {
		return nil, err
	}

	return objs, nil
}

func (o *objectMover) getObjectGraph(namespace string) (*objectGraph, error) {
	objectGraph := newObjectGraph(o.fromProxy, o.fromProviderInventory)

	// Gets all the types defined by the CRDs installed by clusterctl plus the ConfigMap/Secret core types.
	err := objectGraph.getDiscoveryTypes()
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrieve discovery types")
	}

	// Discovery the object graph for the selected types:
	// - Nodes are defined the Kubernetes objects (Clusters, Machines etc.) identified during the discovery process.
	// - Edges are derived by the OwnerReferences between nodes.
	if err := objectGraph.Discovery(namespace); err != nil {
		return nil, errors.Wrap(err, "failed to discover the object graph")
	}

	// Checks if Cluster API has already completed the provisioning of the infrastructure for the objects involved in the move/toDirectory operation.
	// This is required because if the infrastructure is provisioned, then we can reasonably assume that the objects we are moving/backing up are
	// not currently waiting for long-running reconciliation loops, and so we can safely rely on the pause field on the Cluster object
	// for blocking any further object reconciliation on the source objects.
	if err := o.checkProvisioningCompleted(objectGraph); err != nil {
		return nil, errors.Wrap(err, "failed to check for provisioned infrastructure")
	}

	// Check whether nodes are not included in GVK considered for move
	objectGraph.checkVirtualNode()

	return objectGraph, nil
}

func newObjectMover(fromProxy Proxy, fromProviderInventory InventoryClient) *objectMover {
	return &objectMover{
		fromProxy:             fromProxy,
		fromProviderInventory: fromProviderInventory,
	}
}

// checkProvisioningCompleted checks if Cluster API has already completed the provisioning of the infrastructure for the objects involved in the move operation.
func (o *objectMover) checkProvisioningCompleted(graph *objectGraph) error {
	if o.dryRun {
		return nil
	}
	errList := []error{}

	// Checking all the clusters have infrastructure is ready
	readClusterBackoff := newReadBackoff()
	clusters := graph.getClusters()
	for i := range clusters {
		cluster := clusters[i]
		clusterObj := &clusterv1.Cluster{}
		if err := retryWithExponentialBackoff(readClusterBackoff, func() error {
			return getClusterObj(o.fromProxy, cluster, clusterObj)
		}); err != nil {
			return err
		}

		if !clusterObj.Status.InfrastructureReady {
			errList = append(errList, errors.Errorf("cannot start the move operation while %q %s/%s is still provisioning the infrastructure", clusterObj.GroupVersionKind(), clusterObj.GetNamespace(), clusterObj.GetName()))
			continue
		}

		// Note: can't use IsFalse here because we need to handle the absence of the condition as well as false.
		if !conditions.IsTrue(clusterObj, clusterv1.ControlPlaneInitializedCondition) {
			errList = append(errList, errors.Errorf("cannot start the move operation while the control plane for %q %s/%s is not yet initialized", clusterObj.GroupVersionKind(), clusterObj.GetNamespace(), clusterObj.GetName()))
			continue
		}

		if clusterObj.Spec.ControlPlaneRef != nil && !clusterObj.Status.ControlPlaneReady {
			errList = append(errList, errors.Errorf("cannot start the move operation while the control plane for %q %s/%s is not yet ready", clusterObj.GroupVersionKind(), clusterObj.GetNamespace(), clusterObj.GetName()))
			continue
		}
	}

	// Checking all the machine have a NodeRef
	// Nb. NodeRef is considered a better signal than InfrastructureReady, because it ensures the node in the workload cluster is up and running.
	readMachinesBackoff := newReadBackoff()
	machines := graph.getMachines()
	for i := range machines {
		machine := machines[i]
		machineObj := &clusterv1.Machine{}
		if err := retryWithExponentialBackoff(readMachinesBackoff, func() error {
			return getMachineObj(o.fromProxy, machine, machineObj)
		}); err != nil {
			return err
		}

		if machineObj.Status.NodeRef == nil {
			errList = append(errList, errors.Errorf("cannot start the move operation while %q %s/%s is still provisioning the node", machineObj.GroupVersionKind(), machineObj.GetNamespace(), machineObj.GetName()))
		}
	}

	return kerrors.NewAggregate(errList)
}

// getClusterObj retrieves the clusterObj corresponding to a node with type Cluster.
func getClusterObj(proxy Proxy, cluster *node, clusterObj *clusterv1.Cluster) error {
	c, err := proxy.NewClient()
	if err != nil {
		return err
	}
	clusterObjKey := client.ObjectKey{
		Namespace: cluster.identity.Namespace,
		Name:      cluster.identity.Name,
	}

	if err := c.Get(ctx, clusterObjKey, clusterObj); err != nil {
		return errors.Wrapf(err, "error reading Cluster %s/%s",
			clusterObj.GetNamespace(), clusterObj.GetName())
	}
	return nil
}

// getMachineObj retrieves the machineObj corresponding to a node with type Machine.
func getMachineObj(proxy Proxy, machine *node, machineObj *clusterv1.Machine) error {
	c, err := proxy.NewClient()
	if err != nil {
		return err
	}
	machineObjKey := client.ObjectKey{
		Namespace: machine.identity.Namespace,
		Name:      machine.identity.Name,
	}

	if err := c.Get(ctx, machineObjKey, machineObj); err != nil {
		return errors.Wrapf(err, "error reading Machine %s/%s",
			machineObj.GetNamespace(), machineObj.GetName())
	}
	return nil
}

// Move moves all the Cluster API objects existing in a namespace (or from all the namespaces if empty) to a target management cluster.
func (o *objectMover) move(graph *objectGraph, toProxy Proxy, mutators ...ResourceMutatorFunc) error {
	log := logf.Log

	clusters := graph.getClusters()
	log.Info("Moving Cluster API objects", "Clusters", len(clusters))

	clusterClasses := graph.getClusterClasses()
	log.Info("Moving Cluster API objects", "ClusterClasses", len(clusterClasses))

	// Sets the pause field on the Cluster object in the source management cluster, so the controllers stop reconciling it.
	log.V(1).Info("Pausing the source cluster")
	if err := setClusterPause(o.fromProxy, clusters, true, o.dryRun); err != nil {
		return err
	}

	log.V(1).Info("Pausing the source ClusterClasses")
	if err := setClusterClassPause(o.fromProxy, clusterClasses, true, o.dryRun); err != nil {
		return errors.Wrap(err, "error pausing ClusterClasses")
	}

	// Nb. DO NOT call ensureNamespaces at this point because:
	// - namespace will be ensured to exist before creating the resource.
	// - If it's done here, we might create a namespace that can end up unused on target cluster (due to mutators).

	// Define the move sequence by processing the ownerReference chain, so we ensure that a Kubernetes object is moved only after its owners.
	// The sequence is bases on object graph nodes, each one representing a Kubernetes object; nodes are grouped, so bulk of nodes can be moved in parallel. e.g.
	// - All the Clusters should be moved first (group 1, processed in parallel)
	// - All the MachineDeployments should be moved second (group 1, processed in parallel)
	// - then all the MachineSets, then all the Machines, etc.
	moveSequence := getMoveSequence(graph)

	// Create all objects group by group, ensuring all the ownerReferences are re-created.
	log.Info("Creating objects in the target cluster")
	for groupIndex := 0; groupIndex < len(moveSequence.groups); groupIndex++ {
		if err := o.createGroup(moveSequence.getGroup(groupIndex), toProxy, mutators...); err != nil {
			return err
		}
	}

	// Nb. mutators used after this point (after creating the resources on target clusters) are mainly intended for
	// using the right namespace to fetch the resource from the target cluster.
	// mutators affecting non metadata fields are no-op after this point.

	// Delete all objects group by group in reverse order.
	log.Info("Deleting objects from the source cluster")
	for groupIndex := len(moveSequence.groups) - 1; groupIndex >= 0; groupIndex-- {
		if err := o.deleteGroup(moveSequence.getGroup(groupIndex)); err != nil {
			return err
		}
	}

	// Resume the ClusterClasses in the target management cluster, so the controllers start reconciling it.
	log.V(1).Info("Resuming the target ClusterClasses")
	if err := setClusterClassPause(toProxy, clusterClasses, false, o.dryRun, mutators...); err != nil {
		return errors.Wrap(err, "error resuming ClusterClasses")
	}

	// Reset the pause field on the Cluster object in the target management cluster, so the controllers start reconciling it.
	log.V(1).Info("Resuming the target cluster")
	return setClusterPause(toProxy, clusters, false, o.dryRun, mutators...)
}

func (o *objectMover) toDirectory(graph *objectGraph, directory string) error {
	log := logf.Log

	clusters := graph.getClusters()
	log.Info("Starting move of Cluster API objects", "Clusters", len(clusters))

	clusterClasses := graph.getClusterClasses()
	log.Info("Moving Cluster API objects", "ClusterClasses", len(clusterClasses))

	// Sets the pause field on the Cluster object in the source management cluster, so the controllers stop reconciling it.
	log.V(1).Info("Pausing the source cluster")
	if err := setClusterPause(o.fromProxy, clusters, true, o.dryRun); err != nil {
		return err
	}

	log.V(1).Info("Pausing the source ClusterClasses")
	if err := setClusterClassPause(o.fromProxy, clusterClasses, true, o.dryRun); err != nil {
		return errors.Wrap(err, "error pausing ClusterClasses")
	}

	// Define the move sequence by processing the ownerReference chain, so we ensure that a Kubernetes object is moved only after its owners.
	// The sequence is bases on object graph nodes, each one representing a Kubernetes object; nodes are grouped, so bulk of nodes can be moved in parallel. e.g.
	// - All the Clusters should be moved first (group 1, processed in parallel)
	// - All the MachineDeployments should be moved second (group 1, processed in parallel)
	// - then all the MachineSets, then all the Machines, etc.
	moveSequence := getMoveSequence(graph)

	// Save all objects group by group
	log.Info(fmt.Sprintf("Saving files to %s", directory))
	for groupIndex := 0; groupIndex < len(moveSequence.groups); groupIndex++ {
		if err := o.backupGroup(moveSequence.getGroup(groupIndex), directory); err != nil {
			return err
		}
	}

	// Resume the ClusterClasses in the target management cluster, so the controllers start reconciling it.
	log.V(1).Info("Resuming the target ClusterClasses")
	if err := setClusterClassPause(o.fromProxy, clusterClasses, false, o.dryRun); err != nil {
		return errors.Wrap(err, "error resuming ClusterClasses")
	}

	// Reset the pause field on the Cluster object in the target management cluster, so the controllers start reconciling it.
	log.V(1).Info("Resuming the source cluster")
	return setClusterPause(o.fromProxy, clusters, false, o.dryRun)
}

func (o *objectMover) fromDirectory(graph *objectGraph, toProxy Proxy) error {
	log := logf.Log

	// Get clusters from graph
	clusters := graph.getClusters()
	// Get clusterclasses from graph
	clusterClasses := graph.getClusterClasses()

	// Ensure all the expected target namespaces are in place before creating objects.
	log.V(1).Info("Creating target namespaces, if missing")
	if err := o.ensureNamespaces(graph, toProxy); err != nil {
		return err
	}

	// Define the move sequence by processing the ownerReference chain, so we ensure that a Kubernetes object is moved only after its owners.
	// The sequence is bases on object graph nodes, each one representing a Kubernetes object; nodes are grouped, so bulk of nodes can be moved in parallel. e.g.
	// - All the Clusters should be moved first (group 1, processed in parallel)
	// - All the MachineDeployments should be moved second (group 1, processed in parallel)
	// - then all the MachineSets, then all the Machines, etc.
	moveSequence := getMoveSequence(graph)

	// Create all objects group by group, ensuring all the ownerReferences are re-created.
	log.Info("Restoring objects into the target cluster")
	for groupIndex := 0; groupIndex < len(moveSequence.groups); groupIndex++ {
		if err := o.restoreGroup(moveSequence.getGroup(groupIndex), toProxy); err != nil {
			return err
		}
	}

	// Resume reconciling the ClusterClasses after being restored from a backup.
	// By default, during backup, ClusterClasses are paused so they must be unpaused to be used again
	log.V(1).Info("Resuming the target ClusterClasses")
	if err := setClusterClassPause(toProxy, clusterClasses, false, o.dryRun); err != nil {
		return errors.Wrap(err, "error resuming ClusterClasses")
	}

	// Resume reconciling the Clusters after being restored from a directory.
	// By default, when moved to a directory, Clusters are paused, so they must be unpaused to be used again.
	log.V(1).Info("Resuming the target cluster")
	return setClusterPause(toProxy, clusters, false, o.dryRun)
}

// moveSequence defines a list of group of moveGroups.
type moveSequence struct {
	groups   []moveGroup
	nodesMap map[*node]empty
}

// moveGroup defines is a list of nodes read from the object graph that can be moved in parallel.
type moveGroup []*node

func (s *moveSequence) addGroup(group moveGroup) {
	// Add the group
	s.groups = append(s.groups, group)
	// Add all the nodes in the group to the nodeMap so we can check if a node is already in the move sequence or not
	for _, n := range group {
		s.nodesMap[n] = empty{}
	}
}

func (s *moveSequence) hasNode(n *node) bool {
	_, ok := s.nodesMap[n]
	return ok
}

func (s *moveSequence) getGroup(i int) moveGroup {
	return s.groups[i]
}

// Define the move sequence by processing the ownerReference chain.
func getMoveSequence(graph *objectGraph) *moveSequence {
	moveSequence := &moveSequence{
		groups:   []moveGroup{},
		nodesMap: make(map[*node]empty),
	}

	for {
		// Determine the next move group by processing all the nodes in the graph that belong to a Cluster.
		// NB. it is necessary to filter out nodes not belonging to a cluster because e.g. discovery reads all the secrets,
		// but only few of them are related to Clusters/Machines etc.
		moveGroup := moveGroup{}

		for _, n := range graph.getMoveNodes() {
			// If the node was already included in the moveSequence, skip it.
			if moveSequence.hasNode(n) {
				continue
			}

			// Check if all the ownerReferences are already included in the move sequence; if yes, add the node to move group,
			// otherwise skip it (the node will be re-processed in the next group).
			ownersInPlace := true
			for owner := range n.owners {
				if !moveSequence.hasNode(owner) {
					ownersInPlace = false
					break
				}
			}
			for owner := range n.softOwners {
				if !moveSequence.hasNode(owner) {
					ownersInPlace = false
					break
				}
			}
			if ownersInPlace {
				moveGroup = append(moveGroup, n)
			}
		}

		// If the resulting move group is empty it means that all the nodes are already in the sequence, so exit.
		if len(moveGroup) == 0 {
			break
		}
		moveSequence.addGroup(moveGroup)
	}
	return moveSequence
}

// setClusterPause sets the paused field on nodes referring to Cluster objects.
func setClusterPause(proxy Proxy, clusters []*node, value bool, dryRun bool, mutators ...ResourceMutatorFunc) error {
	if dryRun {
		return nil
	}

	log := logf.Log
	patchValue := "true"
	if !value {
		// If the `value` is false lets drop the field.
		// This makes sure that clusterctl does now own the field and would avoid any ownership conflicts.
		patchValue = "null"
	}
	patch := client.RawPatch(types.MergePatchType, []byte(fmt.Sprintf("{\"spec\":{\"paused\":%s}}", patchValue)))

	setClusterPauseBackoff := newWriteBackoff()
	for i := range clusters {
		cluster := clusters[i]
		log.V(5).Info("Set Cluster.Spec.Paused", "paused", value, "Cluster", klog.KRef(cluster.identity.Namespace, cluster.identity.Name))

		// Nb. The operation is wrapped in a retry loop to make setClusterPause more resilient to unexpected conditions.
		if err := retryWithExponentialBackoff(setClusterPauseBackoff, func() error {
			return patchCluster(proxy, cluster, patch, mutators...)
		}); err != nil {
			return errors.Wrapf(err, "error setting Cluster.Spec.Paused=%t", value)
		}
	}
	return nil
}

// setClusterClassPause sets the paused annotation on nodes referring to ClusterClass objects.
func setClusterClassPause(proxy Proxy, clusterclasses []*node, pause bool, dryRun bool, mutators ...ResourceMutatorFunc) error {
	if dryRun {
		return nil
	}

	log := logf.Log

	setClusterClassPauseBackoff := newWriteBackoff()
	for i := range clusterclasses {
		clusterclass := clusterclasses[i]
		if pause {
			log.V(5).Info("Set Paused annotation", "ClusterClass", clusterclass.identity.Name, "Namespace", clusterclass.identity.Namespace)
		} else {
			log.V(5).Info("Remove Paused annotation", "ClusterClass", clusterclass.identity.Name, "Namespace", clusterclass.identity.Namespace)
		}

		// Nb. The operation is wrapped in a retry loop to make setClusterClassPause more resilient to unexpected conditions.
		if err := retryWithExponentialBackoff(setClusterClassPauseBackoff, func() error {
			return pauseClusterClass(proxy, clusterclass, pause, mutators...)
		}); err != nil {
			return errors.Wrapf(err, "error updating ClusterClass %s/%s", clusterclass.identity.Namespace, clusterclass.identity.Name)
		}
	}
	return nil
}

// patchCluster applies a patch to a node referring to a Cluster object.
func patchCluster(proxy Proxy, n *node, patch client.Patch, mutators ...ResourceMutatorFunc) error {
	cFrom, err := proxy.NewClient()
	if err != nil {
		return err
	}

	// Since the patch has been generated already in caller of this function, the ONLY affect that mutators can have
	// here is on namespace of the resource.
	clusterObj, err := applyMutators(&clusterv1.Cluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       clusterv1.ClusterKind,
			APIVersion: clusterv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      n.identity.Name,
			Namespace: n.identity.Namespace,
		},
	}, mutators...)
	if err != nil {
		return err
	}

	if err := cFrom.Get(ctx, client.ObjectKeyFromObject(clusterObj), clusterObj); err != nil {
		return errors.Wrapf(err, "error reading Cluster %s/%s",
			clusterObj.GetNamespace(), clusterObj.GetName())
	}

	if err := cFrom.Patch(ctx, clusterObj, patch); err != nil {
		return errors.Wrapf(err, "error patching Cluster %s/%s",
			clusterObj.GetNamespace(), clusterObj.GetName())
	}

	return nil
}

func pauseClusterClass(proxy Proxy, n *node, pause bool, mutators ...ResourceMutatorFunc) error {
	cFrom, err := proxy.NewClient()
	if err != nil {
		return errors.Wrap(err, "error creating client")
	}

	// Get a mutated copy of the ClusterClass to identify the target namespace.
	// The ClusterClass could have been moved to a different namespace after the move.
	mutatedClusterClass, err := applyMutators(&clusterv1.ClusterClass{
		TypeMeta: metav1.TypeMeta{
			Kind:       clusterv1.ClusterClassKind,
			APIVersion: clusterv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      n.identity.Name,
			Namespace: n.identity.Namespace,
		}}, mutators...)
	if err != nil {
		return err
	}

	clusterClass := &clusterv1.ClusterClass{}
	// Construct an object key using the mutatedClusterClass reflecting any changes to the namespace.
	clusterClassObjKey := client.ObjectKey{
		Name:      mutatedClusterClass.GetName(),
		Namespace: mutatedClusterClass.GetNamespace(),
	}
	// Get a copy of the ClusterClass.
	// This will ensure that any other changes from the mutator are ignored here as we work with a fresh copy of the cluster class.
	if err := cFrom.Get(ctx, clusterClassObjKey, clusterClass); err != nil {
		return errors.Wrapf(err, "error reading ClusterClass %s/%s", n.identity.Namespace, n.identity.Name)
	}

	patchHelper, err := patch.NewHelper(clusterClass, cFrom)
	if err != nil {
		return errors.Wrapf(err, "error creating patcher for ClusterClass %s/%s", n.identity.Namespace, n.identity.Name)
	}

	// Update the annotation to the desired state
	ccAnnotations := clusterClass.GetAnnotations()
	if ccAnnotations == nil {
		ccAnnotations = map[string]string{}
	}
	if pause {
		// Set the pause annotation.
		ccAnnotations[clusterv1.PausedAnnotation] = ""
	} else {
		// Delete the pause annotation.
		delete(ccAnnotations, clusterv1.PausedAnnotation)
	}

	// Update the ClusterClass with the new annotations.
	clusterClass.SetAnnotations(ccAnnotations)
	if err := patchHelper.Patch(ctx, clusterClass); err != nil {
		return errors.Wrapf(err, "error patching ClusterClass %s/%s", n.identity.Namespace, n.identity.Name)
	}

	return nil
}

// ensureNamespaces ensures all the expected target namespaces are in place before creating objects.
func (o *objectMover) ensureNamespaces(graph *objectGraph, toProxy Proxy) error {
	if o.dryRun {
		return nil
	}

	ensureNamespaceBackoff := newWriteBackoff()
	namespaces := sets.Set[string]{}
	for _, node := range graph.getMoveNodes() {
		// ignore global/cluster-wide objects
		if node.isGlobal {
			continue
		}

		namespace := node.identity.Namespace

		// If the namespace was already processed, skip it.
		if namespaces.Has(namespace) {
			continue
		}
		namespaces.Insert(namespace)

		if err := retryWithExponentialBackoff(ensureNamespaceBackoff, func() error {
			return o.ensureNamespace(toProxy, namespace)
		}); err != nil {
			return err
		}
	}

	return nil
}

// ensureNamespace ensures a target namespaces is in place before creating objects.
func (o *objectMover) ensureNamespace(toProxy Proxy, namespace string) error {
	log := logf.Log

	cs, err := toProxy.NewClient()
	if err != nil {
		return err
	}

	// Otherwise check if namespace exists (also dealing with RBAC restrictions).
	ns := &corev1.Namespace{}
	key := client.ObjectKey{
		Name: namespace,
	}

	err = cs.Get(ctx, key, ns)
	if err == nil {
		return nil
	}
	if apierrors.IsForbidden(err) {
		namespaces := &corev1.NamespaceList{}
		namespaceExists := false
		for {
			if err := cs.List(ctx, namespaces, client.Continue(namespaces.Continue)); err != nil {
				return err
			}

			for _, ns := range namespaces.Items {
				if ns.Name == namespace {
					namespaceExists = true
					break
				}
			}

			if namespaces.Continue == "" {
				break
			}
		}
		if namespaceExists {
			return nil
		}
	}
	if !apierrors.IsNotFound(err) {
		return err
	}

	// If the namespace does not exist, create it.
	ns = &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	log.V(1).Info("Creating", ns.Kind, ns.Name)
	if err := cs.Create(ctx, ns); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

// createGroup creates all the Kubernetes objects into the target management cluster corresponding to the object graph nodes in a moveGroup.
func (o *objectMover) createGroup(group moveGroup, toProxy Proxy, mutators ...ResourceMutatorFunc) error {
	createTargetObjectBackoff := newWriteBackoff()
	errList := []error{}

	// Maintain a cache of namespaces that have been verified to already exist.
	// Nb. This prevents us from making repetitive (and expensive) calls in listing all namespaces to ensure a namespace exists before creating a resource.
	existingNamespaces := sets.New[string]()
	for _, nodeToCreate := range group {
		// Creates the Kubernetes object corresponding to the nodeToCreate.
		// Nb. The operation is wrapped in a retry loop to make move more resilient to unexpected conditions.
		err := retryWithExponentialBackoff(createTargetObjectBackoff, func() error {
			return o.createTargetObject(nodeToCreate, toProxy, mutators, existingNamespaces)
		})
		if err != nil {
			errList = append(errList, err)
		}
	}

	if len(errList) > 0 {
		return kerrors.NewAggregate(errList)
	}

	return nil
}

func (o *objectMover) backupGroup(group moveGroup, directory string) error {
	backupTargetObjectBackoff := newWriteBackoff()
	errList := []error{}

	for _, nodeToBackup := range group {
		// Backs-up the Kubernetes object corresponding to the nodeToBackup.
		// Nb. The operation is wrapped in a retry loop to make move more resilient to unexpected conditions.
		err := retryWithExponentialBackoff(backupTargetObjectBackoff, func() error {
			return o.backupTargetObject(nodeToBackup, directory)
		})
		if err != nil {
			errList = append(errList, err)
		}
	}

	if len(errList) > 0 {
		return kerrors.NewAggregate(errList)
	}

	return nil
}

func (o *objectMover) restoreGroup(group moveGroup, toProxy Proxy) error {
	restoreTargetObjectBackoff := newWriteBackoff()
	errList := []error{}

	for _, nodeToRestore := range group {
		// Creates the Kubernetes object corresponding to the nodeToRestore.
		// Nb. The operation is wrapped in a retry loop to make move more resilient to unexpected conditions.
		err := retryWithExponentialBackoff(restoreTargetObjectBackoff, func() error {
			return o.restoreTargetObject(nodeToRestore, toProxy)
		})
		if err != nil {
			errList = append(errList, err)
		}
	}

	if len(errList) > 0 {
		return kerrors.NewAggregate(errList)
	}

	return nil
}

// createTargetObject creates the Kubernetes object in the target Management cluster corresponding to the object graph node, taking care of restoring the OwnerReference with the owner nodes, if any.
func (o *objectMover) createTargetObject(nodeToCreate *node, toProxy Proxy, mutators []ResourceMutatorFunc, existingNamespaces sets.Set[string]) error {
	log := logf.Log
	log.V(1).Info("Creating", nodeToCreate.identity.Kind, nodeToCreate.identity.Name, "Namespace", nodeToCreate.identity.Namespace)

	if o.dryRun {
		return nil
	}

	cFrom, err := o.fromProxy.NewClient()
	if err != nil {
		return err
	}

	// Get the source object
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(nodeToCreate.identity.APIVersion)
	obj.SetKind(nodeToCreate.identity.Kind)
	objKey := client.ObjectKey{
		Namespace: nodeToCreate.identity.Namespace,
		Name:      nodeToCreate.identity.Name,
	}

	if err := cFrom.Get(ctx, objKey, obj); err != nil {
		return errors.Wrapf(err, "error reading %q %s/%s",
			obj.GroupVersionKind(), obj.GetNamespace(), obj.GetName())
	}

	// New objects cannot have a specified resource version. Clear it out.
	obj.SetResourceVersion("")

	// Removes current OwnerReferences
	obj.SetOwnerReferences(nil)

	// Rebuild the owner reference chain
	o.buildOwnerChain(obj, nodeToCreate)

	// FIXME Workaround for https://github.com/kubernetes/kubernetes/issues/32220. Remove when the issue is fixed.
	// If the resource already exists, the API server ordinarily returns an AlreadyExists error. Due to the above issue, if the resource has a non-empty metadata.generateName field, the API server returns a ServerTimeoutError. To ensure that the API server returns an AlreadyExists error, we set the metadata.generateName field to an empty string.
	if len(obj.GetName()) > 0 && len(obj.GetGenerateName()) > 0 {
		obj.SetGenerateName("")
	}

	// Creates the targetObj into the target management cluster.
	cTo, err := toProxy.NewClient()
	if err != nil {
		return err
	}

	obj, err = applyMutators(obj, mutators...)
	if err != nil {
		return err
	}
	// Applying mutators MAY change the namespace, so ensure the namespace exists before creating the resource.
	if !nodeToCreate.isGlobal && !existingNamespaces.Has(obj.GetNamespace()) {
		if err = o.ensureNamespace(toProxy, obj.GetNamespace()); err != nil {
			return err
		}
		existingNamespaces.Insert(obj.GetNamespace())
	}
	oldManagedFields := obj.GetManagedFields()
	if err := cTo.Create(ctx, obj); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return errors.Wrapf(err, "error creating %q %s/%s",
				obj.GroupVersionKind(), obj.GetNamespace(), obj.GetName())
		}

		// If the object already exists, try to update it if it is node a global object / something belonging to a global object hierarchy (e.g. a secrets owned by a global identity object).
		if nodeToCreate.isGlobal || nodeToCreate.isGlobalHierarchy {
			log.V(5).Info("Object already exists, skipping upgrade because it is global/it is owned by a global object", nodeToCreate.identity.Kind, nodeToCreate.identity.Name, "Namespace", nodeToCreate.identity.Namespace)
		} else {
			// Nb. This should not happen, but it is supported to make move more resilient to unexpected interrupt/restarts of the move process.
			log.V(5).Info("Object already exists, updating", nodeToCreate.identity.Kind, nodeToCreate.identity.Name, "Namespace", nodeToCreate.identity.Namespace)

			// Retrieve the UID and the resource version for the update.
			existingTargetObj := &unstructured.Unstructured{}
			existingTargetObj.SetAPIVersion(obj.GetAPIVersion())
			existingTargetObj.SetKind(obj.GetKind())
			if err := cTo.Get(ctx, objKey, existingTargetObj); err != nil {
				return errors.Wrapf(err, "error reading resource for %q %s/%s",
					existingTargetObj.GroupVersionKind(), existingTargetObj.GetNamespace(), existingTargetObj.GetName())
			}

			obj.SetUID(existingTargetObj.GetUID())
			obj.SetResourceVersion(existingTargetObj.GetResourceVersion())
			if err := cTo.Update(ctx, obj); err != nil {
				return errors.Wrapf(err, "error updating %q %s/%s",
					obj.GroupVersionKind(), obj.GetNamespace(), obj.GetName())
			}
		}
	}

	// Stores the newUID assigned to the newly created object.
	nodeToCreate.newUID = obj.GetUID()

	if err := patchTopologyManagedFields(ctx, oldManagedFields, obj, cTo); err != nil {
		return errors.Wrap(err, "error patching the managed fields")
	}

	return nil
}

func (o *objectMover) backupTargetObject(nodeToCreate *node, directory string) error {
	log := logf.Log
	log.V(1).Info("Saving", nodeToCreate.identity.Kind, nodeToCreate.identity.Name, "Namespace", nodeToCreate.identity.Namespace)

	cFrom, err := o.fromProxy.NewClient()
	if err != nil {
		return err
	}

	// Get the source object
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(nodeToCreate.identity.APIVersion)
	obj.SetKind(nodeToCreate.identity.Kind)
	objKey := client.ObjectKey{
		Namespace: nodeToCreate.identity.Namespace,
		Name:      nodeToCreate.identity.Name,
	}

	if err := cFrom.Get(ctx, objKey, obj); err != nil {
		return errors.Wrapf(err, "error reading %q %s/%s",
			obj.GroupVersionKind(), obj.GetNamespace(), obj.GetName())
	}

	// Get JSON for object and write it into the configured directory
	byObj, err := obj.MarshalJSON()
	if err != nil {
		return err
	}

	filenameObj := nodeToCreate.getFilename()
	objectFile := filepath.Join(directory, filenameObj)

	// If file exists, then remove it to be written again
	_, err = os.Stat(objectFile)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil {
		if err := os.Remove(objectFile); err != nil {
			return err
		}
	}

	err = os.WriteFile(objectFile, byObj, 0600)
	if err != nil {
		return err
	}

	return nil
}

func (o *objectMover) restoreTargetObject(nodeToCreate *node, toProxy Proxy) error {
	log := logf.Log
	log.V(1).Info("Restoring", nodeToCreate.identity.Kind, nodeToCreate.identity.Name, "Namespace", nodeToCreate.identity.Namespace)

	// Creates the targetObj into the target management cluster.
	cTo, err := toProxy.NewClient()
	if err != nil {
		return err
	}

	// Attempt to retrieve an existing object. If it exists, update the UID to rebuild the owner chain
	objKey := client.ObjectKey{
		Namespace: nodeToCreate.identity.Namespace,
		Name:      nodeToCreate.identity.Name,
	}

	existingTargetObj := &unstructured.Unstructured{}
	existingTargetObj.SetAPIVersion(nodeToCreate.restoreObject.GetAPIVersion())
	existingTargetObj.SetKind(nodeToCreate.restoreObject.GetKind())
	if err := cTo.Get(ctx, objKey, existingTargetObj); err == nil {
		log.V(5).Info("Object already exists, skipping moving from directory", nodeToCreate.identity.Kind, nodeToCreate.identity.Name, "Namespace", nodeToCreate.identity.Namespace)

		// Update the nodes UID since it already exists. Any nodes owned by this existing node will be updated when the owner chain is rebuilt
		nodeToCreate.newUID = existingTargetObj.GetUID()

		// Return early since the object already exists
		return nil
	}

	// Rebuild the source object
	obj := nodeToCreate.restoreObject

	obj.SetAPIVersion(nodeToCreate.identity.APIVersion)
	obj.SetKind(nodeToCreate.identity.Kind)

	// New objects cannot have a specified resource version. Clear it out.
	obj.SetResourceVersion("")

	// Removes current OwnerReferences
	obj.SetOwnerReferences(nil)

	// Rebuild the owner reference chain
	o.buildOwnerChain(obj, nodeToCreate)

	if err := cTo.Create(ctx, obj); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return errors.Wrapf(err, "error creating %q %s/%s",
				obj.GroupVersionKind(), obj.GetNamespace(), obj.GetName())
		}
	}

	// Stores the newUID assigned to the newly created object.
	nodeToCreate.newUID = obj.GetUID()

	return nil
}

// Recreate all the OwnerReferences using the newUID of the owner nodes.
func (o *objectMover) buildOwnerChain(obj *unstructured.Unstructured, n *node) {
	if len(n.owners) > 0 {
		ownerRefs := []metav1.OwnerReference{}
		for ownerNode := range n.owners {
			ownerRef := metav1.OwnerReference{
				APIVersion: ownerNode.identity.APIVersion,
				Kind:       ownerNode.identity.Kind,
				Name:       ownerNode.identity.Name,
				UID:        ownerNode.newUID, // Use the owner's newUID read from the target management cluster (instead of the UID read during discovery).
			}

			// Restores the attributes of the OwnerReference.
			if attributes, ok := n.owners[ownerNode]; ok {
				ownerRef.Controller = attributes.Controller
				ownerRef.BlockOwnerDeletion = attributes.BlockOwnerDeletion
			}

			ownerRefs = append(ownerRefs, ownerRef)
		}
		obj.SetOwnerReferences(ownerRefs)
	}
}

// deleteGroup deletes all the Kubernetes objects from the source management cluster corresponding to the object graph nodes in a moveGroup.
func (o *objectMover) deleteGroup(group moveGroup) error {
	deleteSourceObjectBackoff := newWriteBackoff()
	errList := []error{}
	for i := range group {
		nodeToDelete := group[i]

		// Delete the Kubernetes object corresponding to the current node.
		// Nb. The operation is wrapped in a retry loop to make move more resilient to unexpected conditions.
		err := retryWithExponentialBackoff(deleteSourceObjectBackoff, func() error {
			return o.deleteSourceObject(nodeToDelete)
		})

		if err != nil {
			errList = append(errList, err)
		}
	}

	return kerrors.NewAggregate(errList)
}

var (
	removeFinalizersPatch           = client.RawPatch(types.MergePatchType, []byte("{\"metadata\":{\"finalizers\":[]}}"))
	addDeleteForMoveAnnotationPatch = client.RawPatch(types.JSONPatchType, []byte(fmt.Sprintf("[{\"op\": \"add\", \"path\":\"/metadata/annotations\", \"value\":{%q:\"\"}}]", clusterctlv1.DeleteForMoveAnnotation)))
)

// deleteSourceObject deletes the Kubernetes object corresponding to the node from the source management cluster, taking care of removing all the finalizers so
// the objects gets immediately deleted (force delete).
func (o *objectMover) deleteSourceObject(nodeToDelete *node) error {
	// Don't delete cluster-wide nodes or nodes that are below a hierarchy that starts with a global object (e.g. a secrets owned by a global identity object).
	if nodeToDelete.isGlobal || nodeToDelete.isGlobalHierarchy {
		return nil
	}

	log := logf.Log
	log.V(1).Info("Deleting", nodeToDelete.identity.Kind, nodeToDelete.identity.Name, "Namespace", nodeToDelete.identity.Namespace)

	if o.dryRun {
		return nil
	}

	cFrom, err := o.fromProxy.NewClient()
	if err != nil {
		return err
	}

	// Get the source object
	sourceObj := &unstructured.Unstructured{}
	sourceObj.SetAPIVersion(nodeToDelete.identity.APIVersion)
	sourceObj.SetKind(nodeToDelete.identity.Kind)
	sourceObjKey := client.ObjectKey{
		Namespace: nodeToDelete.identity.Namespace,
		Name:      nodeToDelete.identity.Name,
	}

	if err := cFrom.Get(ctx, sourceObjKey, sourceObj); err != nil {
		if apierrors.IsNotFound(err) {
			// If the object is already deleted, move on.
			log.V(5).Info("Object already deleted, skipping delete for", nodeToDelete.identity.Kind, nodeToDelete.identity.Name, "Namespace", nodeToDelete.identity.Namespace)
			return nil
		}
		return errors.Wrapf(err, "error reading %q %s/%s",
			sourceObj.GroupVersionKind(), sourceObj.GetNamespace(), sourceObj.GetName())
	}

	if err := cFrom.Patch(ctx, sourceObj, addDeleteForMoveAnnotationPatch); err != nil {
		return errors.Wrapf(err, "error adding delete-for-move annotation from %q %s/%s",
			sourceObj.GroupVersionKind(), sourceObj.GetNamespace(), sourceObj.GetName())
	}

	if len(sourceObj.GetFinalizers()) > 0 {
		if err := cFrom.Patch(ctx, sourceObj, removeFinalizersPatch); err != nil {
			return errors.Wrapf(err, "error removing finalizers from %q %s/%s",
				sourceObj.GroupVersionKind(), sourceObj.GetNamespace(), sourceObj.GetName())
		}
	}

	if err := cFrom.Delete(ctx, sourceObj); err != nil {
		return errors.Wrapf(err, "error deleting %q %s/%s",
			sourceObj.GroupVersionKind(), sourceObj.GetNamespace(), sourceObj.GetName())
	}

	return nil
}

// checkTargetProviders checks that all the providers installed in the source cluster exists in the target cluster as well (with a version >= of the current version).
func (o *objectMover) checkTargetProviders(toInventory InventoryClient) error {
	if o.dryRun {
		return nil
	}

	// Gets the list of providers in the source/target cluster.
	fromProviders, err := o.fromProviderInventory.List()
	if err != nil {
		return errors.Wrapf(err, "failed to get provider list from the source cluster")
	}

	toProviders, err := toInventory.List()
	if err != nil {
		return errors.Wrapf(err, "failed to get provider list from the target cluster")
	}

	// Checks all the providers installed in the source cluster
	errList := []error{}
	for _, sourceProvider := range fromProviders.Items {
		sourceVersion, err := version.ParseSemantic(sourceProvider.Version)
		if err != nil {
			return errors.Wrapf(err, "unable to parse version %q for the %s provider in the source cluster", sourceProvider.Version, sourceProvider.InstanceName())
		}

		// Check corresponding providers in the target cluster and gets the latest version installed.
		var maxTargetVersion *version.Version
		for _, targetProvider := range toProviders.Items {
			// Skips other providers.
			if !sourceProvider.SameAs(targetProvider) {
				continue
			}

			targetVersion, err := version.ParseSemantic(targetProvider.Version)
			if err != nil {
				return errors.Wrapf(err, "unable to parse version %q for the %s provider in the target cluster", targetProvider.Version, targetProvider.InstanceName())
			}
			if maxTargetVersion == nil || maxTargetVersion.LessThan(targetVersion) {
				maxTargetVersion = targetVersion
			}
		}
		if maxTargetVersion == nil {
			errList = append(errList, errors.Errorf("provider %s not found in the target cluster", sourceProvider.Name))
			continue
		}

		if !maxTargetVersion.AtLeast(sourceVersion) {
			errList = append(errList, errors.Errorf("provider %s in the target cluster is older than in the source cluster (source: %s, target: %s)", sourceProvider.Name, sourceVersion.String(), maxTargetVersion.String()))
		}
	}

	return kerrors.NewAggregate(errList)
}

// patchTopologyManagedFields patches the managed fields of obj.
// Without patching the managed fields, clusterctl would be the owner of the fields
// which would lead to co-ownership and preventing other controllers using SSA from deleting fields.
func patchTopologyManagedFields(ctx context.Context, oldManagedFields []metav1.ManagedFieldsEntry, obj *unstructured.Unstructured, cTo client.Client) error {
	base := obj.DeepCopy()
	obj.SetManagedFields(oldManagedFields)

	if err := cTo.Patch(ctx, obj, client.MergeFrom(base)); err != nil {
		return errors.Wrapf(err, "error patching managed fields %q %s/%s",
			obj.GroupVersionKind(), obj.GetNamespace(), obj.GetName())
	}
	return nil
}

func applyMutators(object client.Object, mutators ...ResourceMutatorFunc) (*unstructured.Unstructured, error) {
	if object == nil {
		return nil, nil
	}
	u := &unstructured.Unstructured{}
	to, err := runtime.DefaultUnstructuredConverter.ToUnstructured(object)
	if err != nil {
		return nil, err
	}
	u.SetUnstructuredContent(to)
	for _, mutator := range mutators {
		if err := mutator(u); err != nil {
			return nil, errors.Wrapf(err, "error applying resource mutator to %q %s/%s",
				u.GroupVersionKind(), object.GetNamespace(), object.GetName())
		}
	}
	return u, nil
}
