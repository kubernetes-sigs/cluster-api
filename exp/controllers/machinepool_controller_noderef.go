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
	"context"
	"fmt"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/pkg/errors"
	apicorev1 "k8s.io/api/core/v1"
	corev1 "k8s.io/api/core/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controllers/noderefutil"
	"sigs.k8s.io/cluster-api/controllers/remote"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1alpha3"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	ErrNoAvailableNodes = errors.New("cannot find nodes with matching ProviderIDs in ProviderIDList")
)

type getNodeReferencesResult struct {
	references []apicorev1.ObjectReference
	available  int
	ready      int
}

func (r *MachinePoolReconciler) reconcileNodeRefs(ctx context.Context, cluster *clusterv1.Cluster, mp *expv1.MachinePool) (ctrl.Result, error) {
	logger := r.Log.WithValues("cluster", cluster.Name, "machinepool", mp.Name, "namespace", mp.Namespace)
	// Check that the MachinePool hasn't been deleted or in the process.
	if !mp.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	// Check that the Machine doesn't already have a NodeRefs.
	if mp.Status.Replicas == mp.Status.ReadyReplicas && len(mp.Status.NodeRefs) == int(mp.Status.ReadyReplicas) {
		conditions.MarkTrue(mp, expv1.ReplicasReadyCondition)
		return ctrl.Result{}, nil
	}

	// Check that Cluster isn't nil.
	if cluster == nil {
		logger.V(2).Info("MachinePool doesn't have a linked cluster, won't assign NodeRef")
		return ctrl.Result{}, nil
	}

	logger = logger.WithValues("cluster", cluster.Name)

	// Check that the MachinePool has valid ProviderIDList.
	if len(mp.Spec.ProviderIDList) == 0 {
		logger.V(2).Info("MachinePool doesn't have any ProviderIDs yet")
		return ctrl.Result{}, nil
	}

	clusterClient, err := remote.NewClusterClient(ctx, r.Client, util.ObjectKey(cluster), r.scheme)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err = r.deleteRetiredNodes(ctx, clusterClient, mp.Status.NodeRefs, mp.Spec.ProviderIDList); err != nil {
		return ctrl.Result{}, err
	}

	// Get the Node references.
	nodeRefsResult, err := r.getNodeReferences(ctx, clusterClient, mp.Spec.ProviderIDList)
	if err != nil {
		if err == ErrNoAvailableNodes {
			r.Log.Info("Cannot assign NodeRefs to MachinePool, no matching Nodes")
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		r.recorder.Event(mp, apicorev1.EventTypeWarning, "FailedSetNodeRef", err.Error())
		return ctrl.Result{}, errors.Wrapf(err, "failed to get node references")
	}

	mp.Status.ReadyReplicas = int32(nodeRefsResult.ready)
	mp.Status.AvailableReplicas = int32(nodeRefsResult.available)
	mp.Status.UnavailableReplicas = mp.Status.Replicas - mp.Status.AvailableReplicas
	mp.Status.NodeRefs = nodeRefsResult.references

	logger.Info("Set MachinePools's NodeRefs", "noderefs", mp.Status.NodeRefs)
	r.recorder.Event(mp, apicorev1.EventTypeNormal, "SuccessfulSetNodeRefs", fmt.Sprintf("%+v", mp.Status.NodeRefs))

	if mp.Status.Replicas != mp.Status.ReadyReplicas || len(nodeRefsResult.references) != int(mp.Status.ReadyReplicas) {
		r.Log.Info("NodeRefs != ReadyReplicas", "NodeRefs", len(nodeRefsResult.references), "ReadyReplicas", mp.Status.ReadyReplicas)
		conditions.MarkFalse(mp, expv1.ReplicasReadyCondition, expv1.WaitingForReplicasReadyReason, clusterv1.ConditionSeverityInfo, "")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// At this point, the required number of replicas are ready
	conditions.MarkTrue(mp, expv1.ReplicasReadyCondition)
	return ctrl.Result{}, nil
}

// deleteRetiredNodes deletes nodes that don't have a corresponding ProviderID in Spec.ProviderIDList.
// A MachinePool infrastructure provider indicates an instance in the set has been deleted by
// removing its ProviderID from the slice.
func (r *MachinePoolReconciler) deleteRetiredNodes(ctx context.Context, c client.Client, nodeRefs []apicorev1.ObjectReference, providerIDList []string) error {
	logger := r.Log.WithValues("providerIDList", len(providerIDList))
	nodeRefsMap := make(map[string]*apicorev1.Node, len(nodeRefs))
	for _, nodeRef := range nodeRefs {
		node := &corev1.Node{}
		if err := c.Get(ctx, client.ObjectKey{Name: nodeRef.Name}, node); err != nil {
			logger.V(2).Info("Failed to get Node, skipping", "err", err, "nodeRef.Name", nodeRef.Name)
			continue
		}

		nodeProviderID, err := noderefutil.NewProviderID(node.Spec.ProviderID)
		if err != nil {
			logger.V(2).Info("Failed to parse ProviderID, skipping", "err", err, "providerID", node.Spec.ProviderID)
			continue
		}

		nodeRefsMap[nodeProviderID.ID()] = node
	}
	for _, providerID := range providerIDList {
		pid, err := noderefutil.NewProviderID(providerID)
		if err != nil {
			logger.V(2).Info("Failed to parse ProviderID, skipping", "err", err, "providerID", providerID)
			continue
		}
		delete(nodeRefsMap, pid.ID())
	}
	for _, node := range nodeRefsMap {
		if err := c.Delete(ctx, node); err != nil {
			return errors.Wrapf(err, "failed to delete Node")
		}
	}
	return nil
}

func (r *MachinePoolReconciler) getNodeReferences(ctx context.Context, c client.Client, providerIDList []string) (getNodeReferencesResult, error) {
	logger := r.Log.WithValues("providerIDList", len(providerIDList))

	var ready, available int
	nodeRefsMap := make(map[string]apicorev1.Node)
	nodeList := apicorev1.NodeList{}
	for {
		if err := c.List(ctx, &nodeList, client.Continue(nodeList.Continue)); err != nil {
			return getNodeReferencesResult{}, errors.Wrapf(err, "failed to List nodes")
		}

		for _, node := range nodeList.Items {
			nodeProviderID, err := noderefutil.NewProviderID(node.Spec.ProviderID)
			if err != nil {
				logger.V(2).Info("Failed to parse ProviderID, skipping", "err", err, "providerID", node.Spec.ProviderID)
				continue
			}

			nodeRefsMap[nodeProviderID.ID()] = node
		}

		if nodeList.Continue == "" {
			break
		}
	}

	var nodeRefs []apicorev1.ObjectReference
	for _, providerID := range providerIDList {
		pid, err := noderefutil.NewProviderID(providerID)
		if err != nil {
			logger.V(2).Info("Failed to parse ProviderID, skipping", "err", err, "providerID", providerID)
			continue
		}
		if node, ok := nodeRefsMap[pid.ID()]; ok {
			available++
			if nodeIsReady(&node) {
				ready++
			}
			nodeRefs = append(nodeRefs, apicorev1.ObjectReference{
				Kind:       node.Kind,
				APIVersion: node.APIVersion,
				Name:       node.Name,
				UID:        node.UID,
			})
		}
	}

	if len(nodeRefs) == 0 {
		return getNodeReferencesResult{}, ErrNoAvailableNodes
	}
	return getNodeReferencesResult{nodeRefs, available, ready}, nil
}

func nodeIsReady(node *apicorev1.Node) bool {
	for _, n := range node.Status.Conditions {
		if n.Type == apicorev1.NodeReady {
			return n.Status == apicorev1.ConditionTrue
		}
	}
	return false
}
