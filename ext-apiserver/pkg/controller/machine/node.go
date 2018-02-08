/*
Copyright 2017 The Kubernetes Authors.

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

package machine

import (
	"github.com/golang/glog"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

// We are using "machine" annotation to link node and machine resource. The "machine"
// annotation is an implementation detail of how the two objects can get linked together, but it is
// not required behavior. However, in the event that a Machine.Spec update requires replacing the
// Node, this can allow for faster turn-around time by allowing a new Node to be created with a new
// name while the old node is being deleted.
//
// Currently, these annotations are added by the node itself as part of its
// bootup script after "kubeadm join" succeeds.
func (c *MachineControllerImpl) link(node *corev1.Node) error {
	if c.linkedNodes[node.ObjectMeta.Name] {
		return nil
	}

	val, ok := node.ObjectMeta.Annotations["machine"]
	if !ok {
		return nil
	}

	machine, err := c.machineClient.Get(val, metav1.GetOptions{})
	if err != nil {
		glog.Errorf("Error getting machine %v: %v\n", val, err)
		return err
	}

	machine.Status.NodeRef = objectRef(node)
	if _, err = c.machineClient.UpdateStatus(machine); err != nil {
		glog.Errorf("Error updating machine to link to node: %v\n", err)
	} else {
		glog.Infof("Successfully linked machine %s to node %s\n",
			machine.ObjectMeta.Name, node.ObjectMeta.Name)
		c.linkedNodes[node.ObjectMeta.Name] = true
	}
	return err
}

func (c *MachineControllerImpl) unlink(node *corev1.Node) error {
	val, ok := node.ObjectMeta.Annotations["machine"]
	if !ok {
		return nil
	}

	machine, err := c.machineClient.Get(val, metav1.GetOptions{})
	if err != nil {
		glog.Errorf("Error getting machine %v: %v\n", val, err)
		return err
	}

	// This machine has no link to remove
	if machine.Status.NodeRef == nil {
		return nil
	}

	// This machine was linked to a different node, don't unlink them
	if machine.Status.NodeRef.Name != node.ObjectMeta.Name {
		glog.Warningf("Node (%v) is tring to unlink machine (%v) which is linked with node (%v).",
			node.ObjectMeta.Name, machine.ObjectMeta.Name, machine.Status.NodeRef.Name)
		return nil
	}

	machine.Status.NodeRef = nil
	if _, err = c.machineClient.UpdateStatus(machine); err != nil {
		glog.Errorf("Error updating machine %s to unlink node %s: %v\n",
			machine.ObjectMeta.Name, node.ObjectMeta.Name, err)
	} else {
		glog.Infof("Successfully unlinked node %s from machine %s\n",
			node.ObjectMeta.Name, machine.ObjectMeta.Name)
		delete(c.linkedNodes, node.ObjectMeta.Name)
	}
	return err
}

// reconcileNode is serialized by an internal queue. It has built-in rated limited retry logic.
// So as long as the reconcile loop return an error, the processing queue will retry the
// reconciliation.
func (c *MachineControllerImpl) reconcileNode(key string) error {
	_, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	n, err := c.kubernetesClientSet.CoreV1().Nodes().Get(name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		glog.Errorf("Unable to retrieve Node %v from store: %v", key, err)
		return err
	}

	if n.DeletionTimestamp.IsZero() {
		return c.link(n)
	} else {
		return c.unlink(n)
	}
}

func objectRef(node *corev1.Node) *corev1.ObjectReference {
	return &corev1.ObjectReference{
		Kind:      "Node",
		Namespace: node.ObjectMeta.Namespace,
		Name:      node.ObjectMeta.Name,
	}
}
