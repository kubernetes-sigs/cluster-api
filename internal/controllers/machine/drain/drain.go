/*
Copyright 2024 The Kubernetes Authors.

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

// Package drain provides a helper to cordon and drain Nodes.
package drain

import (
	"context"
	"fmt"
	"maps"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/internal/webhooks"
	clog "sigs.k8s.io/cluster-api/util/log"
)

// Helper contains the parameters to control the behaviour of the drain helper.
type Helper struct {
	// Client is the client for the management cluster.
	Client client.Client

	// RemoteClient is the client for the workload cluster.
	RemoteClient client.Client

	// GracePeriodSeconds is how long to wait for a Pod to terminate.
	// IMPORTANT: 0 means "delete immediately"; set to a negative value
	// to use the pod's terminationGracePeriodSeconds.
	// GracePeriodSeconds is used when executing Eviction calls.
	GracePeriodSeconds int

	// SkipWaitForDeleteTimeoutSeconds ignores Pods that have a
	// DeletionTimeStamp > N seconds. This can be used e.g. when a Node is unreachable
	// and the Pods won't drain because of that.
	SkipWaitForDeleteTimeoutSeconds int
}

// CordonNode cordons a Node.
func (d *Helper) CordonNode(ctx context.Context, node *corev1.Node) error {
	if node.Spec.Unschedulable {
		// Node is already cordoned, nothing to do.
		return nil
	}

	log := ctrl.LoggerFrom(ctx)
	log.Info("Cordoning Node")

	patch := client.MergeFrom(node.DeepCopy())
	node.Spec.Unschedulable = true
	if err := d.RemoteClient.Patch(ctx, node, patch); err != nil {
		return errors.Wrapf(err, "failed to cordon Node")
	}

	return nil
}

// GetPodsForEviction gets Pods running on a Node and then filters and returns them as PodDeleteList,
// or error if it cannot list Pods or get DaemonSets. All Pods that have to go away can be obtained with .Pods().
func (d *Helper) GetPodsForEviction(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine, nodeName string) (*PodDeleteList, error) {
	allPods := []*corev1.Pod{}
	podList := &corev1.PodList{}
	for {
		listOpts := []client.ListOption{
			client.InNamespace(metav1.NamespaceAll),
			client.MatchingFields{"spec.nodeName": nodeName},
			client.Continue(podList.Continue),
			client.Limit(100),
		}
		if err := d.RemoteClient.List(ctx, podList, listOpts...); err != nil {
			return nil, errors.Wrapf(err, "failed to get Pods for eviction")
		}

		for _, pod := range podList.Items {
			allPods = append(allPods, &pod)
		}

		if podList.Continue == "" {
			break
		}
	}

	if len(allPods) == 0 {
		return &PodDeleteList{}, nil
	}

	// Get MachineDrainRules matching the Machine and Cluster.
	machineDrainRulesMatchingMachine, err := d.getMatchingMachineDrainRules(ctx, cluster, machine)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get Pods for eviction")
	}

	// List all Namespaces.
	// Note: Namespaces will be cached in the ClusterCache to avoid having to read them on every Reconcile.
	// Note: Because we are using the cache we don't have to use pagination.
	podNamespaces := map[string]*corev1.Namespace{}
	namespaceList := &corev1.NamespaceList{}
	if err := d.RemoteClient.List(ctx, namespaceList); err != nil {
		return nil, errors.Wrapf(err, "failed to get Pods for eviction: failed to list Namespaces")
	}
	for _, ns := range namespaceList.Items {
		podNamespaces[ns.Name] = &ns
	}

	// Note: As soon as a filter decides that a Pod should be skipped (i.e. DrainBehavior == "Skip" or "WaitCompleted")
	// other filters won't be evaluated and the Pod will be skipped.
	list := filterPods(ctx, allPods, []PodFilter{
		// Phase 1: Basic filtering (aligned to kubectl drain)

		// Skip Pods with deletionTimestamp (if Node is unreachable & time.Since(deletionTimestamp) > 1s)
		d.skipDeletedFilter,

		// Skip DaemonSet Pods (if they are not finished)
		d.daemonSetFilter,

		// Skip static Pods
		d.mirrorPodFilter,

		// Add warning for Pods with local storage (if they are not finished)
		d.localStorageFilter,

		// Add warning for Pods without a controller (if they are not finished)
		d.unreplicatedFilter,

		// Phase 2: Filtering based on drain label & MachineDrainRules

		// Skip Pods with label cluster.x-k8s.io/drain == "skip" or "wait-completed"
		d.drainLabelFilter,

		// Use drain behavior and order from first matching MachineDrainRule
		// If there is no matching MachineDrainRule, use behavior: "Drain" and order: 0
		d.machineDrainRulesFilter(machineDrainRulesMatchingMachine, podNamespaces),
	})
	if errs := list.errors(); len(errs) > 0 {
		return nil, errors.Wrapf(kerrors.NewAggregate(errs), "failed to get Pods for eviction")
	}

	return list, nil
}

func (d *Helper) getMatchingMachineDrainRules(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) ([]*clusterv1.MachineDrainRule, error) {
	// List all MachineDrainRules.
	machineDrainRuleList := &clusterv1.MachineDrainRuleList{}
	if err := d.Client.List(ctx, machineDrainRuleList, client.InNamespace(machine.Namespace)); err != nil {
		return nil, errors.Wrapf(err, "failed to list MachineDrainRules")
	}

	// Validate selectors of all MachineDrainRules (so we don't have to do it later for every Pod in machineDrainRulesFilter).
	errs := []error{}
	for _, mdr := range machineDrainRuleList.Items {
		if validationErrs := webhooks.ValidateMachineDrainRulesSelectors(&mdr); len(validationErrs) > 0 {
			errs = append(errs, errors.Wrapf(validationErrs.ToAggregate(), "invalid selectors in MachineDrainRule %s", mdr.Name))
		}
	}
	if len(errs) > 0 {
		return nil, errors.Wrapf(kerrors.NewAggregate(errs), "failed to get matching MachineDrainRules")
	}

	// Collect all MachineDrainRules that match the Machine and Cluster.
	matchingMachineDrainRules := []*clusterv1.MachineDrainRule{}
	for _, mdr := range machineDrainRuleList.Items {
		if !machineDrainRuleAppliesToMachine(&mdr, machine, cluster) {
			continue
		}
		matchingMachineDrainRules = append(matchingMachineDrainRules, &mdr)
	}

	// Sort MachineDrainRules alphabetically (so we don't have to do it later for every Pod in machineDrainRulesFilter).
	sort.Slice(matchingMachineDrainRules, func(i, j int) bool {
		return matchingMachineDrainRules[i].Name < matchingMachineDrainRules[j].Name
	})
	return matchingMachineDrainRules, nil
}

// machineDrainRuleAppliesToMachine evaluates if a MachineDrainRule applies to a Machine.
func machineDrainRuleAppliesToMachine(mdr *clusterv1.MachineDrainRule, machine *clusterv1.Machine, cluster *clusterv1.Cluster) bool {
	// If machines is empty, the MachineDrainRule applies to all Machines.
	if len(mdr.Spec.Machines) == 0 {
		return true
	}

	// The MachineDrainRule applies to a Machine if there is a MachineSelector in MachineDrainRule.spec.machines
	// for which both the selector and clusterSelector match.
	for _, selector := range mdr.Spec.Machines {
		if matchesSelector(selector.Selector, machine) &&
			matchesSelector(selector.ClusterSelector, cluster) {
			return true
		}
	}

	return false
}

func filterPods(ctx context.Context, allPods []*corev1.Pod, filters []PodFilter) *PodDeleteList {
	pods := []PodDelete{}
	for _, pod := range allPods {
		var status PodDeleteStatus
		// Collect warnings for the case where we are going to delete the Pod.
		var deleteWarnings []string
		for _, filter := range filters {
			status = filter(ctx, pod)
			if status.DrainBehavior == clusterv1.MachineDrainRuleDrainBehaviorSkip ||
				status.DrainBehavior == clusterv1.MachineDrainRuleDrainBehaviorWaitCompleted {
				// short-circuit as soon as pod is filtered out
				// at that point, there is no reason to run pod
				// through any additional filters
				break
			}
			if status.Reason == PodDeleteStatusTypeWarning {
				deleteWarnings = append(deleteWarnings, status.Message)
			}
		}

		// Note: It only makes sense to aggregate warnings if we are going ahead with the deletion.
		// If we don't, it's absolutely fine to just use the status from the filter that decided that
		// we are not going to delete the Pod.
		if status.DrainBehavior == clusterv1.MachineDrainRuleDrainBehaviorDrain &&
			(status.Reason == PodDeleteStatusTypeOkay || status.Reason == PodDeleteStatusTypeWarning) &&
			len(deleteWarnings) > 0 {
			status.Reason = PodDeleteStatusTypeWarning
			status.Message = strings.Join(deleteWarnings, ", ")
		}

		// Add the pod to PodDeleteList no matter what PodDeleteStatus is,
		// those pods whose PodDeleteStatus is false like DaemonSet will
		// be caught by list.errors()
		pod.Kind = "Pod" //nolint:goconst
		pod.APIVersion = "v1"
		pods = append(pods, PodDelete{
			Pod:    pod,
			Status: status,
		})
	}
	list := &PodDeleteList{items: pods}
	return list
}

// EvictPods evicts the pods.
func (d *Helper) EvictPods(ctx context.Context, podDeleteList *PodDeleteList) EvictionResult {
	log := ctrl.LoggerFrom(ctx)

	// Sort podDeleteList, this is important so we always deterministically evict Pods and build the EvictionResult.
	// Otherwise the condition could change with every single reconcile even if nothing changes on the Node.
	sort.Slice(podDeleteList.items, func(i, j int) bool {
		return fmt.Sprintf("%s/%s", podDeleteList.items[i].Pod.GetNamespace(), podDeleteList.items[i].Pod.GetName()) <
			fmt.Sprintf("%s/%s", podDeleteList.items[j].Pod.GetNamespace(), podDeleteList.items[j].Pod.GetName())
	})

	// Get the minimum order of all existing Pods.
	// Note: We are only going to evict or wait for termination of Pods with the minimum order.
	// This could also mean that we don't evict any additional Pods in this call, if there are still Pods with
	// deletionTimestamps that have a lower order.
	minDrainOrder := minDrainOrderOfPodsToDrain(podDeleteList.items)

	var podsToTriggerEvictionNow []PodDelete
	var podsToTriggerEvictionLater []PodDelete
	var podsWithDeletionTimestamp []PodDelete
	var podsToBeIgnored []PodDelete
	var podsToWaitCompletedNow []PodDelete
	var podsToWaitCompletedLater []PodDelete
	for _, pod := range podDeleteList.items {
		switch {
		case pod.Status.DrainBehavior == clusterv1.MachineDrainRuleDrainBehaviorDrain && pod.Pod.DeletionTimestamp.IsZero():
			if ptr.Deref(pod.Status.DrainOrder, 0) == minDrainOrder {
				podsToTriggerEvictionNow = append(podsToTriggerEvictionNow, pod)
			} else {
				podsToTriggerEvictionLater = append(podsToTriggerEvictionLater, pod)
			}
		case pod.Status.DrainBehavior == clusterv1.MachineDrainRuleDrainBehaviorWaitCompleted:
			if ptr.Deref(pod.Status.DrainOrder, 0) == minDrainOrder {
				podsToWaitCompletedNow = append(podsToWaitCompletedNow, pod)
			} else {
				podsToWaitCompletedLater = append(podsToWaitCompletedLater, pod)
			}
		case pod.Status.DrainBehavior == clusterv1.MachineDrainRuleDrainBehaviorDrain:
			podsWithDeletionTimestamp = append(podsWithDeletionTimestamp, pod)
		default:
			podsToBeIgnored = append(podsToBeIgnored, pod)
		}
	}

	log.Info("Drain not completed yet, there are still Pods on the Node that have to be drained",
		"podsToTriggerEvictionNow", podDeleteListToString(podsToTriggerEvictionNow, 5),
		"podsToTriggerEvictionLater", podDeleteListToString(podsToTriggerEvictionLater, 5),
		"podsWithDeletionTimestamp", podDeleteListToString(podsWithDeletionTimestamp, 5),
		"podsToWaitCompletedNow", podDeleteListToString(podsToWaitCompletedNow, 5),
		"podsToWaitCompletedLater", podDeleteListToString(podsToWaitCompletedLater, 5),
	)

	// Trigger evictions for at most 10s. We'll continue on the next reconcile if we hit the timeout.
	evictionTimeout := 10 * time.Second
	ctx, cancel := context.WithTimeoutCause(ctx, evictionTimeout, errors.New("eviction timeout expired"))
	defer cancel()

	res := EvictionResult{
		PodsFailedEviction: map[string][]*corev1.Pod{},
	}

	for _, pd := range podsToBeIgnored {
		log := ctrl.LoggerFrom(ctx, "Pod", klog.KObj(pd.Pod))
		if pd.Status.Reason == PodDeleteStatusTypeWarning && pd.Status.Message != "" {
			log = log.WithValues("reason", pd.Status.Message)
		}

		log.V(4).Info("Skip evicting Pod because it should be ignored")
		res.PodsIgnored = append(res.PodsIgnored, pd.Pod)
	}

	for _, pd := range podsWithDeletionTimestamp {
		log := ctrl.LoggerFrom(ctx, "Pod", klog.KObj(pd.Pod))

		log.V(4).Info("Skip triggering Pod eviction because it already has a deletionTimestamp")
		res.PodsDeletionTimestampSet = append(res.PodsDeletionTimestampSet, pd.Pod)
	}

evictionLoop:
	for _, pd := range podsToTriggerEvictionNow {
		log := ctrl.LoggerFrom(ctx, "Pod", klog.KObj(pd.Pod))
		if pd.Status.Reason == PodDeleteStatusTypeWarning && pd.Status.Message != "" {
			log = log.WithValues("warning", pd.Status.Message)
		}
		ctx := ctrl.LoggerInto(ctx, log)

		select {
		case <-ctx.Done():
			// Skip eviction if the eviction timeout is reached.
			err := fmt.Errorf("eviction timeout of %s reached, eviction will be retried", evictionTimeout)
			log.V(4).Info("Error when evicting Pod", "err", err)
			res.PodsFailedEviction[err.Error()] = append(res.PodsFailedEviction[err.Error()], pd.Pod)
			continue evictionLoop
		default:
		}

		log.V(4).Info("Evicting Pod")

		err := d.evictPod(ctx, pd.Pod)
		switch {
		case err == nil:
			log.V(4).Info("Pod eviction successfully triggered")
			res.PodsDeletionTimestampSet = append(res.PodsDeletionTimestampSet, pd.Pod)
		case apierrors.IsNotFound(err):
			// Pod doesn't exist anymore as it has been deleted in the meantime.
			log.V(4).Info("Eviction not needed, Pod doesn't exist anymore")
			res.PodsNotFound = append(res.PodsNotFound, pd.Pod)
		case apierrors.IsTooManyRequests(err):
			var statusError *apierrors.StatusError

			// Ensure the causes are also included in the error message.
			// Before: "Cannot evict pod as it would violate the pod's disruption budget."
			// After: "Cannot evict pod as it would violate the pod's disruption budget. The disruption budget nginx needs 20 healthy pods and has 20 currently"
			if ok := errors.As(err, &statusError); ok {
				errorMessage := statusError.Status().Message
				if statusError.Status().Details != nil {
					var causes []string
					for _, cause := range statusError.Status().Details.Causes {
						causes = append(causes, cause.Message)
					}
					errorMessage = fmt.Sprintf("%s %v", errorMessage, strings.Join(causes, ","))
				}
				err = errors.New(errorMessage)
			}

			log.V(4).Info("Error when evicting Pod", "err", err)
			res.PodsFailedEviction[err.Error()] = append(res.PodsFailedEviction[err.Error()], pd.Pod)
		case apierrors.IsForbidden(err) && apierrors.HasStatusCause(err, corev1.NamespaceTerminatingCause):
			// Creating an eviction resource in a terminating namespace will throw a forbidden error, e.g.:
			// "pods "pod-6-to-trigger-eviction-namespace-terminating" is forbidden: unable to create new content in namespace test-namespace because it is being terminated"
			// The kube-controller-manager is supposed to set the deletionTimestamp on the Pod and then this error will go away.
			msg := "Cannot evict pod from terminating namespace: unable to create eviction (kube-controller-manager should set deletionTimestamp)"
			log.V(4).Info(msg, "err", err)
			res.PodsFailedEviction[msg] = append(res.PodsFailedEviction[msg], pd.Pod)
		default:
			log.V(4).Info("Error when evicting Pod", "err", err)
			res.PodsFailedEviction[err.Error()] = append(res.PodsFailedEviction[err.Error()], pd.Pod)
		}
	}

	for _, pd := range podsToTriggerEvictionLater {
		res.PodsToTriggerEvictionLater = append(res.PodsToTriggerEvictionLater, pd.Pod)
	}
	for _, pd := range podsToWaitCompletedNow {
		res.PodsToWaitCompletedNow = append(res.PodsToWaitCompletedNow, pd.Pod)
	}
	for _, pd := range podsToWaitCompletedLater {
		res.PodsToWaitCompletedLater = append(res.PodsToWaitCompletedLater, pd.Pod)
	}

	return res
}

func minDrainOrderOfPodsToDrain(pds []PodDelete) int32 {
	minOrder := int32(math.MaxInt32)
	for _, pd := range pds {
		if pd.Status.DrainBehavior == clusterv1.MachineDrainRuleDrainBehaviorDrain &&
			ptr.Deref(pd.Status.DrainOrder, 0) < minOrder {
			minOrder = ptr.Deref(pd.Status.DrainOrder, 0)
		}
		if pd.Status.DrainBehavior == clusterv1.MachineDrainRuleDrainBehaviorWaitCompleted &&
			ptr.Deref(pd.Status.DrainOrder, 0) < minOrder {
			minOrder = ptr.Deref(pd.Status.DrainOrder, 0)
		}
	}
	return minOrder
}

// evictPod evicts the given Pod, or return an error if it couldn't.
func (d *Helper) evictPod(ctx context.Context, pod *corev1.Pod) error {
	delOpts := metav1.DeleteOptions{}
	if d.GracePeriodSeconds >= 0 {
		gracePeriodSeconds := int64(d.GracePeriodSeconds)
		delOpts.GracePeriodSeconds = &gracePeriodSeconds
	}

	eviction := &policyv1.Eviction{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		},
		DeleteOptions: &delOpts,
	}

	return d.RemoteClient.SubResource("eviction").Create(ctx, pod, eviction)
}

// EvictionResult contains the results of an eviction.
type EvictionResult struct {
	PodsDeletionTimestampSet   []*corev1.Pod
	PodsFailedEviction         map[string][]*corev1.Pod
	PodsToTriggerEvictionLater []*corev1.Pod
	PodsToWaitCompletedNow     []*corev1.Pod
	PodsToWaitCompletedLater   []*corev1.Pod
	PodsNotFound               []*corev1.Pod
	PodsIgnored                []*corev1.Pod
}

// DrainCompleted returns if a Node is entirely drained, i.e. if all relevant Pods have gone away.
func (r EvictionResult) DrainCompleted() bool {
	return len(r.PodsDeletionTimestampSet) == 0 && len(r.PodsFailedEviction) == 0 &&
		len(r.PodsToTriggerEvictionLater) == 0 && len(r.PodsToWaitCompletedLater) == 0 &&
		len(r.PodsToWaitCompletedNow) == 0
}

// ConditionMessage returns a condition message for the case where a drain is not completed.
func (r EvictionResult) ConditionMessage(nodeDrainStartTime metav1.Time) string {
	if r.DrainCompleted() {
		return ""
	}

	conditionMessage := fmt.Sprintf("Drain not completed yet (started at %s):", nodeDrainStartTime.Format(time.RFC3339))
	if len(r.PodsDeletionTimestampSet) > 0 {
		kind := "Pod"
		if len(r.PodsDeletionTimestampSet) > 1 {
			kind = "Pods"
		}
		// Note: the code computing stale warning for the machine deleting condition is making assumptions on the format/content of this message.
		// Same applies for other conditions where deleting is involved, e.g. MachineSet's Deleting and ScalingDown condition.
		conditionMessage = fmt.Sprintf("%s\n* %s %s: deletionTimestamp set, but still not removed from the Node",
			conditionMessage, kind, PodListToString(r.PodsDeletionTimestampSet, 3))
	}
	if len(r.PodsFailedEviction) > 0 {
		sortedFailureMessages := slices.Sorted(maps.Keys(r.PodsFailedEviction))

		skippedFailureMessages := []string{}
		if len(sortedFailureMessages) > 5 {
			skippedFailureMessages = sortedFailureMessages[5:]
			sortedFailureMessages = sortedFailureMessages[:5]
		}
		for _, failureMessage := range sortedFailureMessages {
			pods := r.PodsFailedEviction[failureMessage]
			kind := "Pod"
			if len(pods) > 1 {
				kind = "Pods"
			}
			// Note: the code computing stale warning for the machine deleting condition is making assumptions on the format/content of this message.
			// Same applies for other conditions where deleting is involved, e.g. MachineSet's Deleting and ScalingDown condition.
			failureMessage = strings.ReplaceAll(failureMessage, "Cannot evict pod as it would violate the pod's disruption budget.", "cannot evict pod as it would violate the pod's disruption budget.")
			if !strings.HasPrefix(failureMessage, "cannot evict pod as it would violate the pod's disruption budget.") {
				failureMessage = "failed to evict Pod, " + failureMessage
			}
			conditionMessage = fmt.Sprintf("%s\n* %s %s: %s", conditionMessage, kind, PodListToString(pods, 3), failureMessage)
		}
		if len(skippedFailureMessages) > 0 {
			podCount := 0
			for _, failureMessage := range skippedFailureMessages {
				podCount += len(r.PodsFailedEviction[failureMessage])
			}

			if podCount == 1 {
				conditionMessage = fmt.Sprintf("%s\n* 1 Pod with other issues", conditionMessage)
			} else {
				conditionMessage = fmt.Sprintf("%s\n* %d Pods with other issues",
					conditionMessage, podCount)
			}
		}
	}
	if len(r.PodsToWaitCompletedNow) > 0 {
		kind := "Pod"
		if len(r.PodsToWaitCompletedNow) > 1 {
			kind = "Pods"
		}
		// Note: the code computing stale warning for the machine deleting condition is making assumptions on the format/content of this message.
		// Same applies for other conditions where deleting is involved, e.g. MachineSet's Deleting and ScalingDown condition.
		conditionMessage = fmt.Sprintf("%s\n* %s %s: waiting for completion",
			conditionMessage, kind, PodListToString(r.PodsToWaitCompletedNow, 3))
	}
	if len(r.PodsToTriggerEvictionLater) > 0 {
		conditionMessage = fmt.Sprintf("%s\nAfter above Pods have been removed from the Node, the following Pods will be evicted: %s",
			conditionMessage, PodListToString(r.PodsToTriggerEvictionLater, 3))
	}
	if len(r.PodsToWaitCompletedLater) > 0 {
		conditionMessage = fmt.Sprintf("%s\nAfter above Pods have been removed from the Node, waiting for the following Pods to complete without eviction: %s",
			conditionMessage, PodListToString(r.PodsToWaitCompletedLater, 3))
	}
	return conditionMessage
}

// podDeleteListToString returns a comma-separated list of the first n entries of the PodDelete list.
func podDeleteListToString(podList []PodDelete, n int) string {
	return clog.ListToString(podList, func(pd PodDelete) string {
		return klog.KObj(pd.Pod).String()
	}, n)
}

// PodListToString returns a comma-separated list of the first n entries of the Pod list.
func PodListToString(podList []*corev1.Pod, n int) string {
	return clog.ListToString(podList, func(p *corev1.Pod) string {
		return klog.KObj(p).String()
	}, n)
}
