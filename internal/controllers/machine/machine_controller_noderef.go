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

package machine

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/api/core/v1beta2/index"
	"sigs.k8s.io/cluster-api/internal/controllers/machinedeployment/mdutil"
	"sigs.k8s.io/cluster-api/internal/util/taints"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
	"sigs.k8s.io/cluster-api/util/labels"
)

var (
	// ErrNodeNotFound signals that a corev1.Node could not be found for the given provider id.
	ErrNodeNotFound = errors.New("cannot find node with matching ProviderID")
	// CommonNodeAnnotations is a collection of annotations common to all nodes that ClusterAPI manages.
	CommonNodeAnnotations = []string{
		clusterv1.ClusterNameAnnotation,
		clusterv1.ClusterNamespaceAnnotation,
		clusterv1.MachineAnnotation,
		clusterv1.OwnerKindAnnotation,
		clusterv1.OwnerNameAnnotation,
	}
)

func (r *Reconciler) reconcileNode(ctx context.Context, s *scope) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	cluster := s.cluster
	machine := s.machine
	infraMachine := s.infraMachine

	// Create a watch on the nodes in the Cluster.
	if err := r.watchClusterNodes(ctx, cluster); err != nil {
		s.nodeGetError = err
		return ctrl.Result{}, err
	}

	// Check that the Machine has a valid ProviderID.
	if machine.Spec.ProviderID == "" {
		log.Info("Waiting for infrastructure provider to report spec.providerID", machine.Spec.InfrastructureRef.Kind, klog.KRef(machine.Namespace, machine.Spec.InfrastructureRef.Name))
		v1beta1conditions.MarkFalse(machine, clusterv1.MachineNodeHealthyV1Beta1Condition, clusterv1.WaitingForNodeRefV1Beta1Reason, clusterv1.ConditionSeverityInfo, "")
		return ctrl.Result{}, nil
	}

	remoteClient, err := r.ClusterCache.GetClient(ctx, util.ObjectKey(cluster))
	if err != nil {
		s.nodeGetError = err
		return ctrl.Result{}, err
	}

	// Even if Status.NodeRef exists, continue to do the following checks to make sure Node is healthy
	node, err := r.getNode(ctx, remoteClient, machine.Spec.ProviderID)
	if err != nil {
		if err == ErrNodeNotFound {
			if !s.machine.DeletionTimestamp.IsZero() {
				// Tolerate node not found when the machine is being deleted.
				return ctrl.Result{}, nil
			}

			// While a NodeRef is set in the status, failing to get that node means the node is deleted.
			// If Status.NodeRef is not set before, node still can be in the provisioning state.
			if machine.Status.NodeRef.IsDefined() {
				v1beta1conditions.MarkFalse(machine, clusterv1.MachineNodeHealthyV1Beta1Condition, clusterv1.NodeNotFoundV1Beta1Reason, clusterv1.ConditionSeverityError, "")
				return ctrl.Result{}, errors.Wrapf(err, "no matching Node for Machine %q in namespace %q", machine.Name, machine.Namespace)
			}
			v1beta1conditions.MarkFalse(machine, clusterv1.MachineNodeHealthyV1Beta1Condition, clusterv1.NodeProvisioningV1Beta1Reason, clusterv1.ConditionSeverityWarning, "Waiting for a node with matching ProviderID to exist")
			log.Info("Infrastructure provider reporting spec.providerID, matching Kubernetes node is not yet available", machine.Spec.InfrastructureRef.Kind, klog.KRef(machine.Namespace, machine.Spec.InfrastructureRef.Name), "providerID", machine.Spec.ProviderID)
			// No need to requeue here. Nodes emit an event that triggers reconciliation.
			return ctrl.Result{}, nil
		}
		s.nodeGetError = err
		r.recorder.Event(machine, corev1.EventTypeWarning, "Failed to retrieve Node by ProviderID", err.Error())
		v1beta1conditions.MarkUnknown(machine, clusterv1.MachineNodeHealthyV1Beta1Condition, clusterv1.NodeInspectionFailedV1Beta1Reason, "Failed to get the Node for this Machine by ProviderID")
		return ctrl.Result{}, err
	}
	s.node = node

	// Set the Machine NodeRef.
	if !machine.Status.NodeRef.IsDefined() {
		machine.Status.NodeRef = clusterv1.MachineNodeReference{
			Name: s.node.Name,
		}
		log.Info("Infrastructure provider reporting spec.providerID, Kubernetes node is now available", machine.Spec.InfrastructureRef.Kind, klog.KRef(machine.Namespace, machine.Spec.InfrastructureRef.Name), "providerID", machine.Spec.ProviderID, "Node", klog.KRef("", machine.Status.NodeRef.Name))
		r.recorder.Event(machine, corev1.EventTypeNormal, "SuccessfulSetNodeRef", machine.Status.NodeRef.Name)
	}

	// Set the NodeSystemInfo.
	machine.Status.NodeInfo = &s.node.Status.NodeInfo

	// Compute all the annotations that CAPI is setting on nodes;
	nodeAnnotations := annotations.GetManagedAnnotations(machine, r.AdditionalSyncMachineAnnotations...)

	// Compute labels to be propagated from Machines to nodes.
	// NOTE: CAPI should manage only a subset of node labels, everything else should be preserved.
	// NOTE: Once we reconcile node labels for the first time, the NodeUninitializedTaint is removed from the node.
	nodeLabels := labels.GetManagedLabels(machine.Labels, r.AdditionalSyncMachineLabels...)

	// Get interruptible instance status from the infrastructure provider and set the interruptible label on the node.
	interruptible := false
	found := false
	if infraMachine != nil {
		interruptible, found, err = unstructured.NestedBool(infraMachine.Object, "status", "interruptible")
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to get status interruptible from infra machine %s", klog.KObj(infraMachine))
		}
		// If interruptible is set and is true add the interruptible label to the node labels.
		if found && interruptible {
			nodeLabels[clusterv1.InterruptibleLabel] = ""
		}
	}

	_, nodeHadInterruptibleLabel := s.node.Labels[clusterv1.InterruptibleLabel]

	// Reconcile node taints
	if err := r.patchNode(ctx, remoteClient, s.node, nodeLabels, nodeAnnotations, machine); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to reconcile Node %s", klog.KObj(s.node))
	}
	if !nodeHadInterruptibleLabel && interruptible {
		// If the interruptible label is added to the node then record the event.
		// Nb. Only record the event if the node previously did not have the label to avoid recording
		// the event during every reconcile.
		r.recorder.Event(machine, corev1.EventTypeNormal, "SuccessfulSetInterruptibleNodeLabel", s.node.Name)
	}

	if s.infraMachine == nil || !s.infraMachine.GetDeletionTimestamp().IsZero() {
		v1beta1conditions.MarkFalse(s.machine, clusterv1.MachineNodeHealthyV1Beta1Condition, clusterv1.DeletingV1Beta1Reason, clusterv1.ConditionSeverityInfo, "")
		return ctrl.Result{}, nil
	}

	// Do the remaining node health checks, then set the node health to true if all checks pass.
	status, message := summarizeNodeV1beta1Conditions(s.node)
	if status == corev1.ConditionFalse {
		v1beta1conditions.MarkFalse(machine, clusterv1.MachineNodeHealthyV1Beta1Condition, clusterv1.NodeConditionsFailedV1Beta1Reason, clusterv1.ConditionSeverityWarning, "%s", message)
		return ctrl.Result{}, nil
	}
	if status == corev1.ConditionUnknown {
		v1beta1conditions.MarkUnknown(machine, clusterv1.MachineNodeHealthyV1Beta1Condition, clusterv1.NodeConditionsFailedV1Beta1Reason, "%s", message)
		return ctrl.Result{}, nil
	}

	v1beta1conditions.MarkTrue(machine, clusterv1.MachineNodeHealthyV1Beta1Condition)
	return ctrl.Result{}, nil
}

// summarizeNodeV1beta1Conditions summarizes a Node's conditions and returns the summary of condition statuses and concatenate failed condition messages:
// if there is at least 1 semantically-negative condition, summarized status = False;
// if there is at least 1 semantically-positive condition when there is 0 semantically negative condition, summarized status = True;
// if all conditions are unknown,  summarized status = Unknown.
// (semantically true conditions: NodeMemoryPressure/NodeDiskPressure/NodePIDPressure == false or Ready == true.)
func summarizeNodeV1beta1Conditions(node *corev1.Node) (corev1.ConditionStatus, string) {
	semanticallyFalseStatus := 0
	unknownStatus := 0

	message := ""
	for _, condition := range node.Status.Conditions {
		switch condition.Type {
		case corev1.NodeMemoryPressure, corev1.NodeDiskPressure, corev1.NodePIDPressure:
			if condition.Status != corev1.ConditionFalse {
				message += fmt.Sprintf("Node condition %s is %s", condition.Type, condition.Status) + ". "
				if condition.Status == corev1.ConditionUnknown {
					unknownStatus++
					continue
				}
				semanticallyFalseStatus++
			}
		case corev1.NodeReady:
			if condition.Status != corev1.ConditionTrue {
				message += fmt.Sprintf("Node condition %s is %s", condition.Type, condition.Status) + ". "
				if condition.Status == corev1.ConditionUnknown {
					unknownStatus++
					continue
				}
				semanticallyFalseStatus++
			}
		}
	}
	message = strings.TrimSuffix(message, ". ")
	if semanticallyFalseStatus > 0 {
		return corev1.ConditionFalse, message
	}
	if semanticallyFalseStatus+unknownStatus < 4 {
		return corev1.ConditionTrue, message
	}
	return corev1.ConditionUnknown, message
}

func (r *Reconciler) getNode(ctx context.Context, c client.Reader, providerID string) (*corev1.Node, error) {
	nodeList := corev1.NodeList{}
	if err := c.List(ctx, &nodeList, client.MatchingFields{index.NodeProviderIDField: providerID}); err != nil {
		return nil, err
	}
	if len(nodeList.Items) == 0 {
		// If for whatever reason the index isn't registered or available, we fallback to loop over the whole list.
		nl := corev1.NodeList{}
		// Note: We don't use pagination as this is a cached client and a cached client doesn't support pagination.
		if err := c.List(ctx, &nl); err != nil {
			return nil, err
		}

		for _, node := range nl.Items {
			if providerID == node.Spec.ProviderID {
				return &node, nil
			}
		}

		return nil, ErrNodeNotFound
	}

	if len(nodeList.Items) != 1 {
		return nil, fmt.Errorf("unexpectedly found more than one Node matching the providerID %s", providerID)
	}

	return &nodeList.Items[0], nil
}

// PatchNode is required to workaround an issue on Node.Status.Address which is incorrectly annotated as patchStrategy=merge
// and this causes SSA patch to fail in case there are two addresses with the same key https://github.com/kubernetes-sigs/cluster-api/issues/8417
func (r *Reconciler) patchNode(ctx context.Context, remoteClient client.Client, node *corev1.Node, newLabels, newAnnotations map[string]string, m *clusterv1.Machine) error {
	newNode := node.DeepCopy()

	// Adds the annotations from the Machine.
	// NOTE: in order to handle deletion we are tracking the annotations set from the Machine in an annotation
	// at the next reconcile we are going to use this for deleting annotations previously set by the Machine, but
	// not present anymore. Annotations not set from machines should be always preserved.
	if newNode.Annotations == nil {
		newNode.Annotations = make(map[string]string)
	}
	hasAnnotationChanges := false
	annotationsFromPreviousReconcile := strings.Split(newNode.Annotations[clusterv1.AnnotationsFromMachineAnnotation], ",")
	if len(annotationsFromPreviousReconcile) == 1 && annotationsFromPreviousReconcile[0] == "" {
		annotationsFromPreviousReconcile = []string{}
	}
	// append well known names
	annotationsFromPreviousReconcile = append(annotationsFromPreviousReconcile, CommonNodeAnnotations...)

	annotationsFromCurrentReconcile := []string{}
	for k, v := range newAnnotations {
		if cur, ok := newNode.Annotations[k]; !ok || cur != v {
			newNode.Annotations[k] = v
			hasAnnotationChanges = true
		}
		annotationsFromCurrentReconcile = append(annotationsFromCurrentReconcile, k)
	}

	// Make sure any annotations that were in the previous reconcile but aren't in the current set are removed.
	for _, k := range annotationsFromPreviousReconcile {
		// Don't include the annotation used to track other annotations
		if k == clusterv1.AnnotationsFromMachineAnnotation {
			continue
		}
		if _, ok := newAnnotations[k]; !ok {
			delete(newNode.Annotations, k)
			hasAnnotationChanges = true
		}
	}

	// Adds the labels from the Machine.
	// NOTE: in order to handle deletion we are tracking the labels set from the Machine in an annotation.
	// At the next reconcile we are going to use this for deleting labels previously set by the Machine, but
	// not present anymore. Labels not set from machines should be always preserved.
	if newNode.Labels == nil {
		newNode.Labels = make(map[string]string)
	}
	hasLabelChanges := false
	labelsFromPreviousReconcile := strings.Split(newNode.Annotations[clusterv1.LabelsFromMachineAnnotation], ",")
	if len(labelsFromPreviousReconcile) == 1 && labelsFromPreviousReconcile[0] == "" {
		labelsFromPreviousReconcile = []string{}
	}
	labelsFromCurrentReconcile := []string{}
	for k, v := range newLabels {
		if cur, ok := newNode.Labels[k]; !ok || cur != v {
			newNode.Labels[k] = v
			hasLabelChanges = true
		}
		labelsFromCurrentReconcile = append(labelsFromCurrentReconcile, k)
	}
	for _, k := range labelsFromPreviousReconcile {
		if _, ok := newLabels[k]; !ok {
			delete(newNode.Labels, k)
			hasLabelChanges = true
		}
	}

	// drop the well known annotations before setting the value of AnnotationsFromMachineAnnotation so we're not double-accounting
	// our own metadata
	finalAnnotationsFromCurrentReconcile := []string{}
	for _, entry := range annotationsFromCurrentReconcile {
		if slices.Contains(CommonNodeAnnotations, entry) {
			continue
		}
		finalAnnotationsFromCurrentReconcile = append(finalAnnotationsFromCurrentReconcile, entry)
	}

	// Sort entries so that comparisons in tests are determinate
	slices.Sort(finalAnnotationsFromCurrentReconcile)
	slices.Sort(labelsFromCurrentReconcile)

	finalAnnotationsFromMachine := strings.Join(finalAnnotationsFromCurrentReconcile, ",")
	newLabelsFromMachine := strings.Join(labelsFromCurrentReconcile, ",")

	annotations.AddAnnotations(newNode, map[string]string{clusterv1.LabelsFromMachineAnnotation: newLabelsFromMachine})
	annotations.AddAnnotations(newNode, map[string]string{clusterv1.AnnotationsFromMachineAnnotation: finalAnnotationsFromMachine})

	// Drop the NodeUninitializedTaint taint on the node given that we are reconciling labels.
	hasTaintChanges := taints.RemoveNodeTaint(newNode, clusterv1.NodeUninitializedTaint)

	// Set Taint to a node in an old MachineSet and unset Taint from a node in a new MachineSet
	isOutdated, notFound, err := shouldNodeHaveOutdatedTaint(ctx, r.Client, m)
	if err != nil {
		return errors.Wrapf(err, "failed to check if Node %s is outdated", klog.KRef("", node.Name))
	}

	// It is only possible to identify if we have to set or remove the NodeOutdatedRevisionTaint if shouldNodeHaveOutdatedTaint
	// found all relevant objects.
	// Example: when the MachineDeployment or Machineset can't be found due to a background deletion of objects.
	if !notFound {
		if isOutdated {
			hasTaintChanges = taints.EnsureNodeTaint(newNode, clusterv1.NodeOutdatedRevisionTaint) || hasTaintChanges
		} else {
			hasTaintChanges = taints.RemoveNodeTaint(newNode, clusterv1.NodeOutdatedRevisionTaint) || hasTaintChanges
		}
	}

	if !hasAnnotationChanges && !hasLabelChanges && !hasTaintChanges {
		return nil
	}

	return remoteClient.Patch(ctx, newNode, client.StrategicMergeFrom(node))
}

// shouldNodeHaveOutdatedTaint tries to compare the revision of the owning MachineSet to the MachineDeployment.
// It returns notFound = true if the OwnerReference is not set or the APIServer returns NotFound for the MachineSet or MachineDeployment.
// Note: This three cases could happen during background deletion of objects.
func shouldNodeHaveOutdatedTaint(ctx context.Context, c client.Client, m *clusterv1.Machine) (outdated bool, notFound bool, err error) {
	if _, hasLabel := m.Labels[clusterv1.MachineDeploymentNameLabel]; !hasLabel {
		return false, false, nil
	}

	// Resolve the MachineSet name via owner references because the label value
	// could also be a hash.
	objKey, notFound, err := getOwnerMachineSetObjectKey(m.ObjectMeta)
	if err != nil || notFound {
		return false, notFound, err
	}
	ms := &clusterv1.MachineSet{}
	if err := c.Get(ctx, *objKey, ms); err != nil {
		if apierrors.IsNotFound(err) {
			return false, true, nil
		}
		return false, false, err
	}
	md := &clusterv1.MachineDeployment{}
	objKey = &client.ObjectKey{
		Namespace: m.Namespace,
		Name:      m.Labels[clusterv1.MachineDeploymentNameLabel],
	}
	if err := c.Get(ctx, *objKey, md); err != nil {
		if apierrors.IsNotFound(err) {
			return false, true, nil
		}
		return false, false, err
	}
	msRev, err := mdutil.Revision(ms)
	if err != nil {
		return false, false, err
	}
	mdRev, err := mdutil.Revision(md)
	if err != nil {
		return false, false, err
	}
	if msRev < mdRev {
		return true, false, nil
	}
	return false, false, nil
}

func getOwnerMachineSetObjectKey(obj metav1.ObjectMeta) (*client.ObjectKey, bool, error) {
	for _, ref := range obj.GetOwnerReferences() {
		gv, err := schema.ParseGroupVersion(ref.APIVersion)
		if err != nil {
			return nil, false, err
		}
		if ref.Kind == "MachineSet" && gv.Group == clusterv1.GroupVersion.Group {
			return &client.ObjectKey{Namespace: obj.Namespace, Name: ref.Name}, false, nil
		}
	}
	return nil, true, nil
}
