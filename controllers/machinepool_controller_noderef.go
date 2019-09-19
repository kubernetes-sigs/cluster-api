/*
Copyright 2019 The Kubernetes Authors.

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

package controllers

import (
	"fmt"
	"time"

	"github.com/pkg/errors"
	apicorev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/klog"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controllers/remote"
	capierrors "sigs.k8s.io/cluster-api/errors"
)

func (r *MachinePoolReconciler) reconcileNodeRefs(cluster *clusterv1.Cluster, machinepool *clusterv1.MachinePool) error {
	// Check that the MachinePool hasn't been deleted or in the process.
	if !machinepool.DeletionTimestamp.IsZero() {
		return nil
	}

	// Check that Cluster isn't nil.
	if cluster == nil {
		klog.V(2).Infof("MachinePool %q in namespace %q doesn't have a linked cluster, won't assign NodeRef", machinepool.Name, machinepool.Namespace)
		return nil
	}

	// Check that the MachinePool has a valid ProviderID.
	if machinepool.Spec.ProviderIDs == nil || len(machinepool.Spec.ProviderIDs) == 0 {
		klog.Warningf("MachinePool %q in namespace %q doesn't have a valid ProviderID yet", machinepool.Name, machinepool.Namespace)
		return nil
	}

	clusterClient, err := remote.NewClusterClient(r.Client, cluster)
	if err != nil {
		return err
	}

	corev1Client, err := clusterClient.CoreV1()
	if err != nil {
		return err
	}

	err = r.deleteRetiredNodes(corev1Client, machinepool.Status.NodeRefs, machinepool.Spec.ProviderIDs)
	if err != nil {
		return err
	}

	// Get the Node reference.
	nodeRefs, available, ready, err := r.getNodeReferences(corev1Client, machinepool.Spec.ProviderIDs)
	if err != nil {
		if err == ErrNodeNotFound {
			return errors.Wrapf(&capierrors.RequeueAfterError{RequeueAfter: 10 * time.Second},
				"cannot assign NodeRefs to MachinePool %q in namespace %q, no matching Nodes", machinepool.Name, machinepool.Namespace)
		}
		klog.Errorf("Failed to assign NodeRefs to MachinePool %q in namespace %q: %v", machinepool.Name, machinepool.Namespace, err)
		r.recorder.Event(machinepool, apicorev1.EventTypeWarning, "FailedSetNodeRef", err.Error())
		return err
	}

	machinepool.Status.AvailableReplicas = int32(available)
	machinepool.Status.ReadyReplicas = int32(ready)
	machinepool.Status.NodeRefs = nodeRefs
	klog.Infof("Set MachinePool's (%q in namespace %q) NodeRefs to %q", machinepool.Name, machinepool.Namespace, machinepool.Status.NodeRefs)
	r.recorder.Event(machinepool, apicorev1.EventTypeNormal, "SuccessfulSetNodeRef", fmt.Sprintf("%+v", machinepool.Status.NodeRefs))

	if machinepool.Status.ReadyReplicas == 0 || len(nodeRefs) != int(machinepool.Status.ReadyReplicas) {
		return errors.Wrapf(&capierrors.RequeueAfterError{RequeueAfter: 10 * time.Second},
			"NodeRefs != ReadyReplicas [%q != %q] for MachinePool %q in namespace %q", len(nodeRefs), machinepool.Status.ReadyReplicas, machinepool.Name, machinepool.Namespace)
	}
	return nil
}

func (r *MachinePoolReconciler) deleteRetiredNodes(client corev1.NodesGetter, nodeRefs []apicorev1.ObjectReference, providerIDs []string) error {
	nodeRefsMap := make(map[string]*apicorev1.Node)
	for _, nodeRef := range nodeRefs {
		getOpt := metav1.GetOptions{}
		node, err := client.Nodes().Get(nodeRef.Name, getOpt)
		if err != nil {
			return err
		}
		nodeRefsMap[node.Spec.ProviderID] = node
	}
	for _, providerID := range providerIDs {
		delete(nodeRefsMap, providerID)
	}
	for _, node := range nodeRefsMap {
		err := client.Nodes().Delete(node.Name, &metav1.DeleteOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *MachinePoolReconciler) getNodeReferences(client corev1.NodesGetter, providerIDs []string) ([]apicorev1.ObjectReference, int, int, error) {
	var ready, available int
	nodeRefsMap := make(map[string]apicorev1.Node)
	for {
		listOpt := metav1.ListOptions{}
		nodeList, err := client.Nodes().List(listOpt)
		if err != nil {
			return nil, 0, 0, err
		}

		for _, node := range nodeList.Items {
			nodeRefsMap[node.Spec.ProviderID] = node
		}

		listOpt.Continue = nodeList.Continue
		if listOpt.Continue == "" {
			break
		}
	}

	var nodeRefs []apicorev1.ObjectReference
	for _, providerID := range providerIDs {
		if node, ok := nodeRefsMap[providerID]; ok {
			available++
			nodeReady := isReady(&node)
			if nodeReady {
				ready++
			}
			klog.Infof("Found node %q in ready(%v) with providerID %q", node.Name, nodeReady, node.Spec.ProviderID)
			nodeRefs = append(nodeRefs, apicorev1.ObjectReference{
				Kind:       node.Kind,
				APIVersion: node.APIVersion,
				Name:       node.Name,
				UID:        node.UID,
			})
		}
	}

	if nodeRefs == nil {
		return nil, 0, 0, ErrNodeNotFound
	}
	return nodeRefs, available, ready, nil
}

func isReady(node *apicorev1.Node) bool {
	for _, n := range node.Status.Conditions {
		if n.Type == apicorev1.NodeReady {
			return n.Status == apicorev1.ConditionTrue
		}
	}
	return false
}
