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

package node

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/controller/noderefutil"
	"sigs.k8s.io/cluster-api/pkg/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// MachineAnnotationKey annotation used to link node and machine resource
	MachineAnnotationKey = "cluster.k8s.io/machine"
)

// We are using "cluster.k8s.io/machine" annotation to link node and machine resource. The "cluster.k8s.io/machine"
// annotation is an implementation detail of how the two objects can get linked together, but it is
// not required behavior. However, in the event that a Machine.Spec update requires replacing the
// Node, this can allow for faster turn-around time by allowing a new Node to be created with a new
// name while the old node is being deleted.
//
// Currently, these annotations are added by the node itself as part of its
// bootup script after "kubeadm join" succeeds.
func (c *ReconcileNode) link(node *corev1.Node) error {
	nodeReady := noderefutil.IsNodeReady(node)

	// skip update if cached and no change in readiness.
	if c.linkedNodes[node.ObjectMeta.Name] {
		if cachedReady, ok := c.cachedReadiness[node.ObjectMeta.Name]; ok && cachedReady == nodeReady {
			return nil
		}
	}

	val, ok := node.ObjectMeta.Annotations[MachineAnnotationKey]
	if !ok {
		return nil
	}

	namespace, mach, err := cache.SplitMetaNamespaceKey(val)
	if err != nil {
		klog.Errorf("Machine annotation format is incorrect %v: %v\n", val, err)
		return err
	}
	namespace = util.GetNamespaceOrDefault(namespace)
	key := client.ObjectKey{Namespace: namespace, Name: mach}

	machine := &v1alpha1.Machine{}
	if err = c.Client.Get(context.Background(), key, machine); err != nil {
		klog.Errorf("Error getting machine %v: %v\n", mach, err)
		return err
	}

	t := metav1.Now()
	machine.Status.LastUpdated = &t
	machine.Status.NodeRef = objectRef(node)
	if err = c.Client.Status().Update(context.Background(), machine); err != nil {
		klog.Errorf("Error updating machine to link to node: %v\n", err)
	} else {
		klog.Infof("Successfully linked machine %s to node %s\n",
			machine.ObjectMeta.Name, node.ObjectMeta.Name)
		c.linkedNodes[node.ObjectMeta.Name] = true
		c.cachedReadiness[node.ObjectMeta.Name] = nodeReady
	}
	return err
}

func (c *ReconcileNode) unlink(node *corev1.Node) error {
	val, ok := node.ObjectMeta.Annotations[MachineAnnotationKey]
	if !ok {
		return nil
	}

	namespace, mach, err := cache.SplitMetaNamespaceKey(val)
	if err != nil {
		klog.Errorf("Machine annotation format is incorrect %v: %v\n", val, err)
		return err
	}
	namespace = util.GetNamespaceOrDefault(namespace)
	key := client.ObjectKey{Namespace: namespace, Name: mach}

	machine := &v1alpha1.Machine{}
	if err = c.Client.Get(context.Background(), key, machine); err != nil {
		klog.Errorf("Error getting machine %v: %v\n", mach, err)
		return err
	}

	// This machine has no link to remove
	if machine.Status.NodeRef == nil {
		return nil
	}

	// This machine was linked to a different node, don't unlink them
	if machine.Status.NodeRef.Name != node.ObjectMeta.Name {
		klog.Warningf("Node (%v) is tring to unlink machine (%v) which is linked with node (%v).",
			node.ObjectMeta.Name, machine.ObjectMeta.Name, machine.Status.NodeRef.Name)
		return nil
	}

	t := metav1.Now()
	machine.Status.LastUpdated = &t
	machine.Status.NodeRef = nil
	if err = c.Client.Status().Update(context.Background(), machine); err != nil {
		klog.Errorf("Error updating machine %s to unlink node %s: %v\n",
			machine.ObjectMeta.Name, node.ObjectMeta.Name, err)
	} else {
		klog.Infof("Successfully unlinked node %s from machine %s\n",
			node.ObjectMeta.Name, machine.ObjectMeta.Name)
		delete(c.cachedReadiness, node.ObjectMeta.Name)
		delete(c.linkedNodes, node.ObjectMeta.Name)
	}
	return err
}

func objectRef(node *corev1.Node) *corev1.ObjectReference {
	return &corev1.ObjectReference{
		Kind: "Node",
		Name: node.ObjectMeta.Name,
		UID:  node.UID,
	}
}
