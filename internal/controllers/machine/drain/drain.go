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
	"sort"
	"strings"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/exp/maps"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Helper contains the parameters to control the behaviour of the drain helper.
type Helper struct {
	Client client.Client

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
	if err := d.Client.Patch(ctx, node, patch); err != nil {
		return errors.Wrapf(err, "failed to cordon Node")
	}

	return nil
}

// GetPodsForEviction gets Pods running on a Node and then filters and returns them as PodDeleteList,
// or error if it cannot list Pods or get DaemonSets. All Pods that have to go away can be obtained with .Pods().
func (d *Helper) GetPodsForEviction(ctx context.Context, nodeName string) (*PodDeleteList, error) {
	allPods := []*corev1.Pod{}
	podList := &corev1.PodList{}
	for {
		listOpts := []client.ListOption{
			client.InNamespace(metav1.NamespaceAll),
			client.MatchingFields{"spec.nodeName": nodeName},
			client.Continue(podList.Continue),
			client.Limit(100),
		}
		if err := d.Client.List(ctx, podList, listOpts...); err != nil {
			return nil, errors.Wrapf(err, "failed to get Pods for eviction")
		}

		for _, pod := range podList.Items {
			allPods = append(allPods, &pod)
		}

		if podList.Continue == "" {
			break
		}
	}

	list := filterPods(ctx, allPods, d.makeFilters())
	if errs := list.errors(); len(errs) > 0 {
		return nil, errors.Wrapf(kerrors.NewAggregate(errs), "failed to get Pods for eviction")
	}

	return list, nil
}

func filterPods(ctx context.Context, allPods []*corev1.Pod, filters []PodFilter) *PodDeleteList {
	pods := []PodDelete{}
	for _, pod := range allPods {
		var status PodDeleteStatus
		// Collect warnings for the case where we are going to delete the Pod.
		var deleteWarnings []string
		for _, filter := range filters {
			status = filter(ctx, pod)
			if !status.Delete {
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
		if status.Delete &&
			(status.Reason == PodDeleteStatusTypeOkay || status.Reason == PodDeleteStatusTypeWarning) &&
			len(deleteWarnings) > 0 {
			status.Reason = PodDeleteStatusTypeWarning
			status.Message = strings.Join(deleteWarnings, ", ")
		}

		// Add the pod to PodDeleteList no matter what PodDeleteStatus is,
		// those pods whose PodDeleteStatus is false like DaemonSet will
		// be caught by list.errors()
		pod.Kind = "Pod"
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

	var podsToTriggerEviction []PodDelete
	var podsWithDeletionTimestamp []PodDelete
	var podsToBeIgnored []PodDelete
	for _, pod := range podDeleteList.items {
		switch {
		case pod.Status.Delete && pod.Pod.DeletionTimestamp.IsZero():
			podsToTriggerEviction = append(podsToTriggerEviction, pod)
		case pod.Status.Delete:
			podsWithDeletionTimestamp = append(podsWithDeletionTimestamp, pod)
		default:
			podsToBeIgnored = append(podsToBeIgnored, pod)
		}
	}

	log.Info("Drain not completed yet, there are still Pods on the Node that have to be drained",
		"podsToTriggerEviction", podDeleteListToString(podsToTriggerEviction, 5),
		"podsWithDeletionTimestamp", podDeleteListToString(podsWithDeletionTimestamp, 5),
	)

	// Trigger evictions for at most 10s. We'll continue on the next reconcile if we hit the timeout.
	evictionTimeout := 10 * time.Second
	ctx, cancel := context.WithTimeout(ctx, evictionTimeout)
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
	for _, pd := range podsToTriggerEviction {
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

	return res
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

	return d.Client.SubResource("eviction").Create(ctx, pod, eviction)
}

// EvictionResult contains the results of an eviction.
type EvictionResult struct {
	PodsDeletionTimestampSet []*corev1.Pod
	PodsFailedEviction       map[string][]*corev1.Pod
	PodsNotFound             []*corev1.Pod
	PodsIgnored              []*corev1.Pod
}

// DrainCompleted returns if a Node is entirely drained, i.e. if all relevant Pods have gone away.
func (r EvictionResult) DrainCompleted() bool {
	return len(r.PodsDeletionTimestampSet) == 0 && len(r.PodsFailedEviction) == 0
}

// ConditionMessage returns a condition message for the case where a drain is not completed.
func (r EvictionResult) ConditionMessage() string {
	if r.DrainCompleted() {
		return ""
	}

	conditionMessage := "Drain not completed yet:"
	if len(r.PodsDeletionTimestampSet) > 0 {
		conditionMessage = fmt.Sprintf("%s\n* Pods with deletionTimestamp that still exist: %s",
			conditionMessage, PodListToString(r.PodsDeletionTimestampSet, 5))
	}
	if len(r.PodsFailedEviction) > 0 {
		sortedFailureMessages := maps.Keys(r.PodsFailedEviction)
		sort.Strings(sortedFailureMessages)

		conditionMessage = fmt.Sprintf("%s\n* Pods with eviction failed:", conditionMessage)

		skippedFailureMessages := []string{}
		if len(sortedFailureMessages) > 5 {
			skippedFailureMessages = sortedFailureMessages[5:]
			sortedFailureMessages = sortedFailureMessages[:5]
		}
		for _, failureMessage := range sortedFailureMessages {
			pods := r.PodsFailedEviction[failureMessage]
			conditionMessage = fmt.Sprintf("%s\n  * %s: %s", conditionMessage, failureMessage, PodListToString(pods, 3))
		}
		if len(skippedFailureMessages) > 0 {
			skippedFailureMessagesCount := len(skippedFailureMessages)
			podCount := 0
			for _, failureMessage := range skippedFailureMessages {
				podCount += len(r.PodsFailedEviction[failureMessage])
			}

			conditionMessage = fmt.Sprintf("%s\n  * ... ", conditionMessage)
			if skippedFailureMessagesCount == 1 {
				conditionMessage += "(1 more error "
			} else {
				conditionMessage += fmt.Sprintf("(%d more errors ", skippedFailureMessagesCount)
			}
			if podCount == 1 {
				conditionMessage += "applying to 1 Pod)"
			} else {
				conditionMessage += fmt.Sprintf("applying to %d Pods)", podCount)
			}
		}
	}
	return conditionMessage
}

// podDeleteListToString returns a comma-separated list of the first n entries of the PodDelete list.
func podDeleteListToString(podList []PodDelete, n int) string {
	return listToString(podList, func(pd PodDelete) string {
		return klog.KObj(pd.Pod).String()
	}, n)
}

// PodListToString returns a comma-separated list of the first n entries of the Pod list.
func PodListToString(podList []*corev1.Pod, n int) string {
	return listToString(podList, func(p *corev1.Pod) string {
		return klog.KObj(p).String()
	}, n)
}

// listToString returns a comma-separated list of the first n entries of the list (strings are calculated via stringFunc).
func listToString[T any](list []T, stringFunc func(T) string, n int) string {
	shortenedBy := 0
	if len(list) > n {
		shortenedBy = len(list) - n
		list = list[:n]
	}
	stringList := []string{}
	for _, p := range list {
		stringList = append(stringList, stringFunc(p))
	}

	if shortenedBy > 0 {
		stringList = append(stringList, fmt.Sprintf("... (%d more)", shortenedBy))
	}

	return strings.Join(stringList, ", ")
}
