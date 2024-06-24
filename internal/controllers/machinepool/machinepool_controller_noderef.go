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

package machinepool

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/noderefutil"
	"sigs.k8s.io/cluster-api/internal/util/taints"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
)

var errNoAvailableNodes = errors.New("cannot find nodes with matching ProviderIDs in ProviderIDList")

type getNodeReferencesResult struct {
	references []corev1.ObjectReference
	available  int
	ready      int
}

func (r *Reconciler) reconcileNodeRefs(ctx context.Context, cluster *clusterv1.Cluster, mp *clusterv1.MachinePool) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// Create a watch on the nodes in the Cluster.
	if err := r.watchClusterNodes(ctx, cluster); err != nil {
		return ctrl.Result{}, err
	}

	// Check that the MachinePool hasn't been deleted or in the process.
	if !mp.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	// Check that the Machine doesn't already have a NodeRefs.
	// Return early if there is no work to do.
	if mp.Status.Replicas == mp.Status.ReadyReplicas && len(mp.Status.NodeRefs) == int(mp.Status.ReadyReplicas) {
		conditions.MarkTrue(mp, clusterv1.ReplicasReadyCondition)
		return ctrl.Result{}, nil
	}

	// Check that the MachinePool has valid ProviderIDList.
	if len(mp.Spec.ProviderIDList) == 0 && (mp.Spec.Replicas == nil || *mp.Spec.Replicas != 0) {
		log.V(2).Info("MachinePool doesn't have any ProviderIDs yet")
		return ctrl.Result{}, nil
	}

	clusterClient, err := r.Tracker.GetClient(ctx, util.ObjectKey(cluster))
	if err != nil {
		return ctrl.Result{}, err
	}

	if err = r.deleteRetiredNodes(ctx, clusterClient, mp.Status.NodeRefs, mp.Spec.ProviderIDList); err != nil {
		return ctrl.Result{}, err
	}

	// Get the Node references.
	nodeRefsResult, err := r.getNodeReferences(ctx, clusterClient, mp.Spec.ProviderIDList, mp.Spec.MinReadySeconds)
	if err != nil {
		if err == errNoAvailableNodes {
			log.Info("Cannot assign NodeRefs to MachinePool, no matching Nodes")
			// No need to requeue here. Nodes emit an event that triggers reconciliation.
			return ctrl.Result{}, nil
		}
		r.recorder.Event(mp, corev1.EventTypeWarning, "FailedSetNodeRef", err.Error())
		return ctrl.Result{}, errors.Wrapf(err, "failed to get node references")
	}

	mp.Status.ReadyReplicas = int32(nodeRefsResult.ready)
	mp.Status.AvailableReplicas = int32(nodeRefsResult.available)
	mp.Status.UnavailableReplicas = mp.Status.Replicas - mp.Status.AvailableReplicas
	mp.Status.NodeRefs = nodeRefsResult.references

	log.Info("Set MachinePool's NodeRefs", "nodeRefs", mp.Status.NodeRefs)
	r.recorder.Event(mp, corev1.EventTypeNormal, "SuccessfulSetNodeRefs", fmt.Sprintf("%+v", mp.Status.NodeRefs))

	// Reconcile node annotations and taints.
	err = r.patchNodes(ctx, clusterClient, nodeRefsResult.references, mp)
	if err != nil {
		return ctrl.Result{}, err
	}

	if mp.Status.Replicas != mp.Status.ReadyReplicas || len(nodeRefsResult.references) != int(mp.Status.ReadyReplicas) {
		log.Info("NodeRefs != ReadyReplicas", "nodeRefs", len(nodeRefsResult.references), "readyReplicas", mp.Status.ReadyReplicas)
		conditions.MarkFalse(mp, clusterv1.ReplicasReadyCondition, clusterv1.WaitingForReplicasReadyReason, clusterv1.ConditionSeverityInfo, "")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// At this point, the required number of replicas are ready
	conditions.MarkTrue(mp, clusterv1.ReplicasReadyCondition)
	return ctrl.Result{}, nil
}

// deleteRetiredNodes deletes nodes that don't have a corresponding ProviderID in Spec.ProviderIDList.
// A MachinePool infrastructure provider indicates an instance in the set has been deleted by
// removing its ProviderID from the slice.
func (r *Reconciler) deleteRetiredNodes(ctx context.Context, c client.Client, nodeRefs []corev1.ObjectReference, providerIDList []string) error {
	log := ctrl.LoggerFrom(ctx, "providerIDList", len(providerIDList))
	nodeRefsMap := make(map[string]*corev1.Node, len(nodeRefs))
	for _, nodeRef := range nodeRefs {
		node := &corev1.Node{}
		if err := c.Get(ctx, client.ObjectKey{Name: nodeRef.Name}, node); err != nil {
			log.V(2).Error(err, "Failed to get Node, skipping", "Node", klog.KRef("", nodeRef.Name))
			continue
		}

		if node.Spec.ProviderID == "" {
			log.V(2).Info("No ProviderID detected, skipping", "providerID", node.Spec.ProviderID)
			continue
		}

		nodeRefsMap[node.Spec.ProviderID] = node
	}
	for _, providerID := range providerIDList {
		if providerID == "" {
			log.V(2).Info("No ProviderID detected, skipping", "providerID", providerID)
			continue
		}
		delete(nodeRefsMap, providerID)
	}
	for _, node := range nodeRefsMap {
		if err := c.Delete(ctx, node); err != nil {
			return errors.Wrapf(err, "failed to delete Node")
		}
	}
	return nil
}

func (r *Reconciler) getNodeReferences(ctx context.Context, c client.Client, providerIDList []string, minReadySeconds *int32) (getNodeReferencesResult, error) {
	log := ctrl.LoggerFrom(ctx, "providerIDList", len(providerIDList))

	var ready, available int
	nodeRefsMap := make(map[string]corev1.Node)
	nodeList := corev1.NodeList{}
	for {
		if err := c.List(ctx, &nodeList, client.Continue(nodeList.Continue)); err != nil {
			return getNodeReferencesResult{}, errors.Wrapf(err, "failed to List nodes")
		}

		for _, node := range nodeList.Items {
			if node.Spec.ProviderID == "" {
				log.V(2).Info("No ProviderID detected, skipping", "providerID", node.Spec.ProviderID)
				continue
			}

			nodeRefsMap[node.Spec.ProviderID] = node
		}

		if nodeList.Continue == "" {
			break
		}
	}

	var nodeRefs []corev1.ObjectReference
	for _, providerID := range providerIDList {
		if providerID == "" {
			log.V(2).Info("No ProviderID detected, skipping", "providerID", providerID)
			continue
		}
		if node, ok := nodeRefsMap[providerID]; ok {
			if noderefutil.IsNodeReady(&node) {
				ready++
				if noderefutil.IsNodeAvailable(&node, *minReadySeconds, metav1.Now()) {
					available++
				}
			}
			nodeRefs = append(nodeRefs, corev1.ObjectReference{
				APIVersion: corev1.SchemeGroupVersion.String(),
				Kind:       "Node",
				Name:       node.Name,
				UID:        node.UID,
			})
		}
	}

	if len(nodeRefs) == 0 && len(providerIDList) != 0 {
		return getNodeReferencesResult{}, errNoAvailableNodes
	}
	return getNodeReferencesResult{nodeRefs, available, ready}, nil
}

// patchNodes patches the nodes with the cluster name and cluster namespace annotations.
func (r *Reconciler) patchNodes(ctx context.Context, c client.Client, references []corev1.ObjectReference, mp *clusterv1.MachinePool) error {
	log := ctrl.LoggerFrom(ctx)
	for _, nodeRef := range references {
		node := &corev1.Node{}
		if err := c.Get(ctx, client.ObjectKey{Name: nodeRef.Name}, node); err != nil {
			log.V(2).Error(err, "Failed to get Node, skipping setting annotations", "Node", klog.KRef("", nodeRef.Name))
			continue
		}
		patchHelper, err := patch.NewHelper(node, c)
		if err != nil {
			return err
		}
		desired := map[string]string{
			clusterv1.ClusterNameAnnotation:      mp.Spec.ClusterName,
			clusterv1.ClusterNamespaceAnnotation: mp.GetNamespace(),
			clusterv1.OwnerKindAnnotation:        mp.Kind,
			clusterv1.OwnerNameAnnotation:        mp.Name,
		}
		// Add annotations and drop NodeUninitializedTaint.
		hasAnnotationChanges := annotations.AddAnnotations(node, desired)
		hasTaintChanges := taints.RemoveNodeTaint(node, clusterv1.NodeUninitializedTaint)
		// Patch the node if needed.
		if hasAnnotationChanges || hasTaintChanges {
			if err := patchHelper.Patch(ctx, node); err != nil {
				log.V(2).Error(err, "Failed patch Node to set annotations and drop taints", "Node", klog.KObj(node))
				return err
			}
		}
	}
	return nil
}
