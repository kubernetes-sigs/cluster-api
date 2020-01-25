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
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ObjectMover defines methods for moving Cluster API objects to another management cluster.
type ObjectMover interface {
	// Move moves all the Cluster API objects existing in a namespace (or from all the namespaces if empty) to a target management cluster.
	Move(namespace string, toCluster Client) error
}

// MachineWaiter defines a function that waits for a machine to be reconciled and linked with the corresponding node.
type MachineWaiter func(proxy Proxy, machineKey client.ObjectKey) error

// objectMover implements the ObjectMover interface.
type objectMover struct {
	fromProxy     Proxy
	machineWaiter MachineWaiter
	log           logr.Logger
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

func newObjectMover(fromProxy Proxy, machineWaiter MachineWaiter, log logr.Logger) *objectMover {
	return &objectMover{
		fromProxy:     fromProxy,
		machineWaiter: machineWaiter,
		log:           log,
	}
}

const (
	retryIntervalResourceReady = 10 * time.Second
	timeoutMachineReady        = 30 * time.Minute
)

// waitForMachineReady waits for a machine just moved to a new management cluster to be reconciled and linked with the corresponding node.
func waitForMachineReady(proxy Proxy, machineKey client.ObjectKey) error {
	machine := new(clusterv1.Machine)
	c, err := proxy.NewClient()
	if err != nil {
		return err
	}

	err = wait.PollImmediate(retryIntervalResourceReady, timeoutMachineReady, func() (bool, error) {
		if err := c.Get(ctx, machineKey, machine); err != nil {
			return false, err
		}

		// Return true if the Machine has a reference to a Node.
		return machine.Status.NodeRef != nil, nil
	})

	return err
}

// Move moves all the Cluster API objects existing in a namespace (or from all the namespaces if empty) to a target management cluster
func (o *objectMover) move(graph *objectGraph, toProxy Proxy) error {
	clusters := graph.getClusters()
	o.log.Info("Clusters to move", "Count", len(clusters))

	// Move Cluster by Cluster, ensuring all the dependent objects are moved as well and that the OwnerReferences relations are rebuilt in the target cluster.
	// Nb. We are moving clusters one by one in sequential order for the sake of readability of the logs.
	for _, cluster := range clusters {
		o.log.Info("Moving", "Cluster", cluster.identity.Name, "Namespace", cluster.identity.Namespace)

		// Sets the pause field on the Cluster object in the source management cluster, so the controllers stop reconciling it.
		if err := setClusterPause(o.fromProxy, cluster, o.log); err != nil {
			return err
		}

		// Create the Cluster object and all the dependent object tree into the target management cluster
		// NB. we are doing all he create first, so the process can be restarted from the beginning time in case of problems.
		if err := o.createTreeIntoTarget(nil, cluster, toProxy); err != nil {
			return err
		}

		// Delete the Cluster object and all the dependent object tree from the source management cluster
		// NB. we are doing delete after all the create completes, so the process can be restarted from the beginning time in case of problems.
		if err := o.deleteTreeFromSource(cluster); err != nil {
			return err
		}
	}

	return nil
}

var (
	clusterPausePatch = client.ConstantPatch(types.MergePatchType, []byte("{\"spec\":{\"paused\":true}}"))
)

// setClusterPause sets the paused field on a Cluster object.
func setClusterPause(proxy Proxy, cluster *node, log logr.Logger) error {
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

	if err := cFrom.Patch(ctx, clusterObj, clusterPausePatch); err != nil {
		return errors.Wrapf(err, "error pausing reconciliation for %q %s/%s",
			clusterObj.GroupVersionKind(), clusterObj.GetNamespace(), clusterObj.GetName())
	}
	return nil
}

const (
	retryCreateTargetObject         = 3
	retryIntervalCreateTargetObject = 1 * time.Second
)

// createTreeIntoTarget creates a node and its dependent node tree into the target management cluster.
func (o *objectMover) createTreeIntoTarget(ownerNode, nodeToCreate *node, toProxy Proxy) error {
	// Creates the Kubernetes object corresponding to the nodeToCreate.
	// Nb. The operation is wrapped in a retry loop to make move more resilient to unexpected conditions.
	err := retry(retryCreateTargetObject, retryIntervalCreateTargetObject, o.log, func() error {
		return o.createTargetObject(ownerNode, nodeToCreate, toProxy)
	})
	if err != nil {
		return err
	}

	// Creates - in parallel - the dependents nodes and the softDependents nodes (with the respective object tree).
	var wg sync.WaitGroup
	errList := []error{}
	errCh := make(chan error)
	defer close(errCh)

	go func() {
		for e := range errCh {
			errList = append(errList, e)
		}
	}()

	for dependent := range nodeToCreate.dependents {
		wg.Add(1)
		go func(dependent *node) {
			defer wg.Done()
			if err := o.createTreeIntoTarget(nodeToCreate, dependent, toProxy); err != nil {
				errCh <- err
			}
		}(dependent)
	}

	for dependent := range nodeToCreate.softDependents {
		wg.Add(1)
		go func(dependent *node) {
			defer wg.Done()
			if err := o.createTreeIntoTarget(nodeToCreate, dependent, toProxy); err != nil {
				errCh <- err
			}
		}(dependent)
	}

	wg.Wait()
	if len(errList) > 0 {
		return kerrors.NewAggregate(errList)
	}

	// If the nodeToCreate is a machine, wait for the machine to be reconciled and linked with the corresponding node.
	// This gives a good signal about the new set of ClusterAPI objects being now successfully linked to the workload cluster.
	if nodeToCreate.identity.GroupVersionKind().GroupKind() == clusterv1.GroupVersion.WithKind("Machine").GroupKind() {
		o.log.V(1).Info("Waiting for reconciliation with the workload cluster node", "Machine", nodeToCreate.identity.Name, "Namespace", nodeToCreate.identity.Namespace)
		machineKey := client.ObjectKey{Namespace: nodeToCreate.identity.Namespace, Name: nodeToCreate.identity.Name}
		if err := o.machineWaiter(toProxy, machineKey); err != nil {
			return err
		}
	}

	return nil
}

// createTargetObject creates the Kubernetes object corresponding to the node in the target Management cluster, taking care of restoring the OwnerReference with the owner node, if any.
func (o *objectMover) createTargetObject(ownerNode, nodeToCreate *node, toProxy Proxy) error {
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

	// Creates a new OwnerReferences towards the OwnerNode, if any.
	if ownerNode != nil {
		ownerRef := metav1.OwnerReference{
			APIVersion: ownerNode.identity.APIVersion,
			Kind:       ownerNode.identity.Kind,
			Name:       ownerNode.identity.Name,
			UID:        ownerNode.newUID, // Use the owner's newUID read from the target management cluster (instead of the UID read during discovery).
		}

		// Restores the attributes of the OwnerReference.
		if attributes, ok := ownerNode.dependents[nodeToCreate]; ok {
			ownerRef.Controller = attributes.Controller
			ownerRef.BlockOwnerDeletion = attributes.BlockOwnerDeletion
		}

		obj.SetOwnerReferences([]metav1.OwnerReference{ownerRef})
	}

	// If the nodeToCreate is a cluster, reset Cluster.Spec.Paused it will be properly reconciled in the target management cluster
	if obj.GroupVersionKind().GroupKind() == clusterv1.GroupVersion.WithKind("Cluster").GroupKind() {
		if err := unstructured.SetNestedField(obj.Object, false, "spec", "paused"); err != nil {
			return errors.Wrapf(err, "failed to reset Cluster.Spec.Paused for  %q %s/%s",
				obj.GroupVersionKind(), obj.GetNamespace(), obj.GetName())
		}
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

// deleteTreeFromSource deletes a node and all the dependent nodes from the source management cluster.
func (o *objectMover) deleteTreeFromSource(nodeToDelete *node) error {
	// Deletes - in parallel - the dependents nodes and the softDependents nodes (with the respective object tree).
	var wg sync.WaitGroup
	errList := []error{}
	errCh := make(chan error)
	defer close(errCh)

	go func() {
		for e := range errCh {
			errList = append(errList, e)
		}
	}()

	for dependent := range nodeToDelete.dependents {
		wg.Add(1)
		go func(dependent *node) {
			defer wg.Done()
			if err := o.deleteTreeFromSource(dependent); err != nil {
				errCh <- err
			}
		}(dependent)
	}

	for dependent := range nodeToDelete.softDependents {
		wg.Add(1)
		go func(dependent *node) {
			defer wg.Done()
			if err := o.deleteTreeFromSource(dependent); err != nil {
				errCh <- err
			}
		}(dependent)
	}

	wg.Wait()
	if len(errList) > 0 {
		return kerrors.NewAggregate(errList)
	}

	// Delete the Kubernetes object corresponding to the current node.
	// Nb. The operation is wrapped in a retry loop to make move more resilient to unexpected conditions.
	err := retry(retryDeleteSourceObject, retryIntervalDeleteSourceObject, o.log, func() error {
		return o.deleteSourceObject(nodeToDelete)
	})

	return err
}

var (
	removeFinalizersPatch = client.ConstantPatch(types.MergePatchType, []byte("{\"metadata\":{\"finalizers\":[]}}"))
)

// deleteSourceObject deletes the object corresponding to the node from the source management cluster, taking care of removing all the finalizers so
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
