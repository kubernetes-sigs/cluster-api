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
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ObjectMover defines methods for moving Cluster API objects to another management cluster.
type ObjectMover interface {
	// Move moves all the Cluster API objects existing in a namespace (or from all the namespaces if empty) to a target management cluster.
	Move(namespace string, toCluster Client) error
}

// objectMover implements the ObjectMover interface.
type objectMover struct {
	fromProxy Proxy
	log       logr.Logger
}

// ensure objectMover implements the ObjectMover interface.
var _ ObjectMover = &objectMover{}

func (o *objectMover) Move(namespace string, toCluster Client) error {
	objectGraph := newObjectGraph(o.fromProxy, o.log)

	//TODO: implement preflight checks ensuring the target cluster has all the required providers in place

	// Gets all the types defines by the CRDs installed by clusterctl plus the ConfigMap/Secret core types.
	types, err := objectGraph.getDiscoveryTypes()
	if err != nil {
		return err
	}

	// Discovery the object graph for the selected types:
	// - Nodes are defined the Kubernetes objects (Clusters, Machines etc.) identified during the discovery process.
	// - Edges are derived by the OwnerReferences between nodes.
	if err := objectGraph.Discovery(namespace, types); err != nil {
		return err
	}

	//TODO: add a preflight check ensuring all the Clusters/Machines are already provisioned.
	//TODO: consider if to add additional preflight checks ensuring the object graph is complete (no virtual nodes left)
	//TODO: consider if to add additional preflight checks ensuring there are no nodes shared across clusters (or implement support for shared nodes, potentially required by CAPV)

	// Move the objects to the target cluster.
	if err := o.move(objectGraph, toCluster.Proxy()); err != nil {
		return err
	}

	return nil
}

func newObjectMover(fromProxy Proxy, log logr.Logger) *objectMover {
	return &objectMover{
		fromProxy: fromProxy,
		log:       log,
	}
}

// Move moves all the Cluster API objects existing in a namespace (or from all the namespaces if empty) to a target management cluster
func (o *objectMover) move(graph *objectGraph, toProxy Proxy) error {
	clusters := graph.getClusters()
	o.log.Info("Clusters to move", "Count", len(clusters))

	// Sets the pause field on the Cluster object in the source management cluster, so the controllers stop reconciling it.
	if err := setClusterPause(o.fromProxy, clusters, true, o.log); err != nil {
		return err
	}

	// Ensure all the expected target namespaces are in place before creating objects.
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
	for groupIndex := 0; groupIndex < len(moveSequence.groups); groupIndex++ {
		if err := o.createGroup(moveSequence.getGroup(groupIndex), toProxy); err != nil {
			return err
		}
	}

	// Delete all objects group by group in reverse order.
	for groupIndex := len(moveSequence.groups) - 1; groupIndex >= 0; groupIndex-- {
		if err := o.deleteGroup(moveSequence.getGroup(groupIndex)); err != nil {
			return err
		}
	}

	// Reset the pause field on the Cluster object in the target management cluster, so the controllers start reconciling it.
	if err := setClusterPause(toProxy, clusters, false, o.log); err != nil {
		return err
	}

	return nil
}

// moveSequence defines a list of group of moveGroups
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
		for _, n := range graph.getNodesWithClusterTenants() {
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

// setClusterPause sets the paused field on a Cluster object.
func setClusterPause(proxy Proxy, clusters []*node, value bool, log logr.Logger) error {
	patch := client.ConstantPatch(types.MergePatchType, []byte(fmt.Sprintf("{\"spec\":{\"paused\":%t}}", value)))

	for _, cluster := range clusters {
		log.V(1).Info("Set Cluster.Spec.Paused", "Cluster", cluster.identity.Name, "Namespace", cluster.identity.Namespace)

		cFrom, err := proxy.NewClient()
		if err != nil {
			return err
		}

		clusterObj := &clusterv1.Cluster{}
		clusterObjKey := client.ObjectKey{
			Namespace: cluster.identity.Namespace,
			Name:      cluster.identity.Name,
		}

		if err := cFrom.Get(ctx, clusterObjKey, clusterObj); err != nil {
			return errors.Wrapf(err, "error reading %q %s/%s",
				clusterObj.GroupVersionKind(), clusterObj.GetNamespace(), clusterObj.GetName())
		}

		if err := cFrom.Patch(ctx, clusterObj, patch); err != nil {
			return errors.Wrapf(err, "error pausing reconciliation for %q %s/%s",
				clusterObj.GroupVersionKind(), clusterObj.GetNamespace(), clusterObj.GetName())
		}

	}
	return nil
}

// ensureNamespaces ensures all the expected target namespaces are in place before creating objects.
func (o *objectMover) ensureNamespaces(graph *objectGraph, toProxy Proxy) error {
	cs, err := toProxy.NewClient()
	if err != nil {
		return err
	}

	namespaces := sets.NewString()
	for _, node := range graph.getNodesWithClusterTenants() {
		namespace := node.identity.Namespace

		// If the namespace was already processed, skip it.
		if namespaces.Has(namespace) {
			continue
		}
		namespaces.Insert(namespace)

		// Otherwise check if namespace exists (also dealing with RBAC restrictions).
		ns := &corev1.Namespace{}
		key := client.ObjectKey{
			Name: namespace,
		}

		if err := cs.Get(ctx, key, ns); err == nil {
			return nil
		}
		if apierrors.IsForbidden(err) {
			namespaces := &corev1.NamespaceList{}
			if err := cs.List(ctx, namespaces); err != nil {
				return err
			}

			namespaceExists := false
			for _, ns := range namespaces.Items {
				if ns.Name == namespace {
					namespaceExists = true
					break
				}
			}
			if namespaceExists {
				continue
			}
		}
		if !apierrors.IsNotFound(err) {
			return err
		}

		// If the namespace does not exists, create it.
		ns = &corev1.Namespace{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "Namespace",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
		o.log.V(1).Info("Creating", ns.Kind, ns.Name)
		if err := cs.Create(ctx, ns); err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}
	}

	return nil
}

const (
	retryCreateTargetObject         = 3
	retryIntervalCreateTargetObject = 1 * time.Second
)

// createGroup creates all the Kubernetes objects into the target management cluster corresponding to the object graph nodes in a moveGroup.
func (o *objectMover) createGroup(group moveGroup, toProxy Proxy) error {
	errList := []error{}
	for i := range group {
		nodeToCreate := group[i]

		// Creates the Kubernetes object corresponding to the nodeToCreate.
		// Nb. The operation is wrapped in a retry loop to make move more resilient to unexpected conditions.
		err := retry(retryCreateTargetObject, retryIntervalCreateTargetObject, o.log, func() error {
			return o.createTargetObject(nodeToCreate, toProxy)
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
func (o *objectMover) createTargetObject(nodeToCreate *node, toProxy Proxy) error {
	o.log.V(1).Info("Creating", nodeToCreate.identity.Kind, nodeToCreate.identity.Name, "Namespace", nodeToCreate.identity.Namespace)

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

	// Recreate all the OwnerReferences using the newUID of the owner nodes.
	if len(nodeToCreate.owners) > 0 {
		ownerRefs := []metav1.OwnerReference{}
		for ownerNode := range nodeToCreate.owners {
			ownerRef := metav1.OwnerReference{
				APIVersion: ownerNode.identity.APIVersion,
				Kind:       ownerNode.identity.Kind,
				Name:       ownerNode.identity.Name,
				UID:        ownerNode.newUID, // Use the owner's newUID read from the target management cluster (instead of the UID read during discovery).
			}

			// Restores the attributes of the OwnerReference.
			if attributes, ok := nodeToCreate.owners[ownerNode]; ok {
				ownerRef.Controller = attributes.Controller
				ownerRef.BlockOwnerDeletion = attributes.BlockOwnerDeletion
			}

			ownerRefs = append(ownerRefs, ownerRef)
		}
		obj.SetOwnerReferences(ownerRefs)

	}

	// Creates the targetObj into the target management cluster.
	cTo, err := toProxy.NewClient()
	if err != nil {
		return err
	}

	if err := cTo.Create(ctx, obj); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return errors.Wrapf(err, "error creating %q %s/%s",
				obj.GroupVersionKind(), obj.GetNamespace(), obj.GetName())
		}

		// If the object already exists, try to update it.
		// Nb. This should not happen, but it is supported to make move more resilient to unexpected interrupt/restarts of the move process.
		o.log.V(2).Info("Object already exists, updating", nodeToCreate.identity.Kind, nodeToCreate.identity.Name, "Namespace", nodeToCreate.identity.Namespace)

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

	// Stores the newUID assigned to the newly created object.
	nodeToCreate.newUID = obj.GetUID()

	return nil
}

const (
	retryDeleteSourceObject         = 3
	retryIntervalDeleteSourceObject = 1 * time.Second
)

// deleteGroup deletes all the Kubernetes objects from the source management cluster corresponding to the object graph nodes in a moveGroup.
func (o *objectMover) deleteGroup(group moveGroup) error {
	errList := []error{}
	for i := range group {
		nodeToDelete := group[i]

		// Delete the Kubernetes object corresponding to the current node.
		// Nb. The operation is wrapped in a retry loop to make move more resilient to unexpected conditions.
		err := retry(retryDeleteSourceObject, retryIntervalDeleteSourceObject, o.log, func() error {
			return o.deleteSourceObject(nodeToDelete)
		})

		if err != nil {
			errList = append(errList, err)
		}
	}

	return kerrors.NewAggregate(errList)
}

var (
	removeFinalizersPatch = client.ConstantPatch(types.MergePatchType, []byte("{\"metadata\":{\"finalizers\":[]}}"))
)

// deleteSourceObject deletes the Kubernetes object corresponding to the node from the source management cluster, taking care of removing all the finalizers so
// the objects gets immediately deleted (force delete).
func (o *objectMover) deleteSourceObject(nodeToDelete *node) error {
	o.log.V(1).Info("Deleting", nodeToDelete.identity.Kind, nodeToDelete.identity.Name, "Namespace", nodeToDelete.identity.Namespace)

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
			//If the object is already deleted, move on.
			o.log.V(2).Info("Object already deleted, skipping delete for", nodeToDelete.identity.Kind, nodeToDelete.identity.Name, "Namespace", nodeToDelete.identity.Namespace)
			return nil
		}
		return errors.Wrapf(err, "error reading %q %s/%s",
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

func retry(attempts int, interval time.Duration, log logr.Logger, action func() error) error {
	var errorToReturn error
	for i := 0; i < attempts; i++ {
		if err := action(); err != nil {
			errorToReturn = err

			log.V(2).Info("Operation failed, retry")
			pause := wait.Jitter(interval, 1)
			time.Sleep(pause)
			continue
		}
		return nil
	}
	return errors.Wrapf(errorToReturn, "action failed after %d attempts", attempts)
}
