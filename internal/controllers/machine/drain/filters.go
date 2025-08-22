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

package drain

import (
	"context"
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// Note: This file is still mostly kept in sync with: https://github.com/kubernetes/kubernetes/blob/v1.31.0/staging/src/k8s.io/kubectl/pkg/drain/filters.go
// Minor modifications have been made to drop branches that are never used in Cluster API and to use the
// controller-runtime Client.

const (
	daemonSetOrphanedWarning = "evicting orphaned DaemonSet-managed Pod"
	daemonSetWarning         = "ignoring DaemonSet-managed Pod"
	localStorageWarning      = "evicting Pod with local storage"
	unmanagedWarning         = "evicting Pod that has no controller"
)

// PodDelete informs filtering logic whether a pod should be deleted or not.
type PodDelete struct {
	Pod    *corev1.Pod
	Status PodDeleteStatus
}

// PodDeleteList is a wrapper around []PodDelete.
type PodDeleteList struct {
	items []PodDelete
}

// Pods returns a list of Pods that have to go away before the Node can be considered completely drained.
func (l *PodDeleteList) Pods() []*corev1.Pod {
	pods := []*corev1.Pod{}
	for _, i := range l.items {
		if i.Status.DrainBehavior == clusterv1.MachineDrainRuleDrainBehaviorDrain ||
			i.Status.DrainBehavior == clusterv1.MachineDrainRuleDrainBehaviorWaitCompleted {
			pods = append(pods, i.Pod)
		}
	}
	return pods
}

// SkippedPods returns a list of Pods that have to be ignored when considering if the Node has been completely drained.
// Note: As of today the following Pods are skipped:
// * DaemonSet Pods
// * static Pods
// * Pods in deletion (if `SkipWaitForDeleteTimeoutSeconds` is set)
// * Pods that have the label "cluster.x-k8s.io/drain": "skip"
// * Pods for which the first MachineDrainRule that matches specifies spec.drain.behavior = "Skip".
func (l *PodDeleteList) SkippedPods() []*corev1.Pod {
	pods := []*corev1.Pod{}
	for _, i := range l.items {
		if i.Status.DrainBehavior == clusterv1.MachineDrainRuleDrainBehaviorSkip {
			pods = append(pods, i.Pod)
		}
	}
	return pods
}

func (l *PodDeleteList) errors() []error {
	failedPods := make(map[string][]string)
	for _, i := range l.items {
		if i.Status.Reason == PodDeleteStatusTypeError {
			msg := i.Status.Message
			if msg == "" {
				msg = "unexpected error"
			}
			failedPods[msg] = append(failedPods[msg], fmt.Sprintf("%s/%s", i.Pod.Namespace, i.Pod.Name))
		}
	}
	errs := make([]error, 0, len(failedPods))
	for msg, pods := range failedPods {
		errs = append(errs, fmt.Errorf("Pods with error %q: %s", msg, strings.Join(pods, ", ")))
	}
	return errs
}

// PodDeleteStatus informs filters if a pod should be deleted.
type PodDeleteStatus struct {
	// DrainBehavior defines the drain behavior of a Pod, it is either "Skip" or "Drain".
	DrainBehavior clusterv1.MachineDrainRuleDrainBehavior

	// DrainOrder defines the order in which Pods are drained.
	// DrainOrder is only used if DrainBehavior is "Drain".
	DrainOrder *int32

	Reason  string
	Message string
}

// PodFilter takes a pod and returns a PodDeleteStatus.
type PodFilter func(context.Context, *corev1.Pod) PodDeleteStatus

const (
	// PodDeleteStatusTypeOkay is "Okay".
	PodDeleteStatusTypeOkay = "Okay"
	// PodDeleteStatusTypeSkip is "Skip".
	PodDeleteStatusTypeSkip = "Skip"
	// PodDeleteStatusTypeWaitCompleted is "WaitCompleted".
	PodDeleteStatusTypeWaitCompleted = "WaitCompleted"
	// PodDeleteStatusTypeWarning is "Warning".
	PodDeleteStatusTypeWarning = "Warning"
	// PodDeleteStatusTypeError is "Error".
	PodDeleteStatusTypeError = "Error"
)

// MakePodDeleteStatusOkay is a helper method to return the corresponding PodDeleteStatus.
func MakePodDeleteStatusOkay() PodDeleteStatus {
	return PodDeleteStatus{
		DrainBehavior: clusterv1.MachineDrainRuleDrainBehaviorDrain,
		DrainOrder:    ptr.To[int32](0),
		Reason:        PodDeleteStatusTypeOkay,
	}
}

// MakePodDeleteStatusOkayWithOrder is a helper method to return the corresponding PodDeleteStatus.
func MakePodDeleteStatusOkayWithOrder(order *int32) PodDeleteStatus {
	return PodDeleteStatus{
		DrainBehavior: clusterv1.MachineDrainRuleDrainBehaviorDrain,
		DrainOrder:    order,
		Reason:        PodDeleteStatusTypeOkay,
	}
}

// MakePodDeleteStatusSkip is a helper method to return the corresponding PodDeleteStatus.
func MakePodDeleteStatusSkip() PodDeleteStatus {
	return PodDeleteStatus{
		DrainBehavior: clusterv1.MachineDrainRuleDrainBehaviorSkip,
		Reason:        PodDeleteStatusTypeSkip,
	}
}

// MakePodDeleteStatusWaitCompleted is a helper method to return the corresponding PodDeleteStatus.
func MakePodDeleteStatusWaitCompleted() PodDeleteStatus {
	return PodDeleteStatus{
		DrainBehavior: clusterv1.MachineDrainRuleDrainBehaviorWaitCompleted,
		DrainOrder:    ptr.To[int32](0),
		Reason:        PodDeleteStatusTypeWaitCompleted,
	}
}

// MakePodDeleteStatusWithWarning is a helper method to return the corresponding PodDeleteStatus.
func MakePodDeleteStatusWithWarning(behavior clusterv1.MachineDrainRuleDrainBehavior, message string) PodDeleteStatus {
	var order *int32
	if behavior == clusterv1.MachineDrainRuleDrainBehaviorDrain {
		order = ptr.To[int32](0)
	}
	return PodDeleteStatus{
		DrainBehavior: behavior,
		DrainOrder:    order,
		Reason:        PodDeleteStatusTypeWarning,
		Message:       message,
	}
}

// MakePodDeleteStatusWithError is a helper method to return the corresponding PodDeleteStatus.
func MakePodDeleteStatusWithError(message string) PodDeleteStatus {
	return PodDeleteStatus{
		// Errors are handled separately, so Behavior is not used.
		Reason:  PodDeleteStatusTypeError,
		Message: message,
	}
}

func hasLocalStorage(pod *corev1.Pod) bool {
	for _, volume := range pod.Spec.Volumes {
		if volume.EmptyDir != nil {
			return true
		}
	}

	return false
}

func (d *Helper) daemonSetFilter(ctx context.Context, pod *corev1.Pod) PodDeleteStatus {
	// Note that we return false in cases where the pod is DaemonSet managed,
	// regardless of flags.
	//
	// The exception is for pods that are orphaned (the referencing
	// management resource - including DaemonSet - is not found).
	// Such pods will be deleted.
	controllerRef := metav1.GetControllerOf(pod)
	if controllerRef == nil || controllerRef.Kind != appsv1.SchemeGroupVersion.WithKind("DaemonSet").Kind {
		return MakePodDeleteStatusOkay()
	}
	// Any finished pod can be removed.
	if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
		return MakePodDeleteStatusOkay()
	}

	if err := d.RemoteClient.Get(ctx, client.ObjectKey{Namespace: pod.Namespace, Name: controllerRef.Name}, &appsv1.DaemonSet{}); err != nil {
		// remove orphaned pods with a warning
		if apierrors.IsNotFound(err) {
			return MakePodDeleteStatusWithWarning(clusterv1.MachineDrainRuleDrainBehaviorDrain, daemonSetOrphanedWarning)
		}

		return MakePodDeleteStatusWithError(err.Error())
	}

	log := ctrl.LoggerFrom(ctx, "Pod", klog.KObj(pod))
	log.V(4).Info("Skip evicting DaemonSet Pod")
	return MakePodDeleteStatusWithWarning(clusterv1.MachineDrainRuleDrainBehaviorSkip, daemonSetWarning)
}

func (d *Helper) mirrorPodFilter(ctx context.Context, pod *corev1.Pod) PodDeleteStatus {
	if _, found := pod.Annotations[corev1.MirrorPodAnnotationKey]; found {
		log := ctrl.LoggerFrom(ctx, "Pod", klog.KObj(pod))
		log.V(4).Info("Skip evicting static Pod")
		return MakePodDeleteStatusSkip()
	}
	return MakePodDeleteStatusOkay()
}

func (d *Helper) localStorageFilter(_ context.Context, pod *corev1.Pod) PodDeleteStatus {
	if !hasLocalStorage(pod) {
		return MakePodDeleteStatusOkay()
	}
	// Any finished pod can be removed.
	if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
		return MakePodDeleteStatusOkay()
	}

	return MakePodDeleteStatusWithWarning(clusterv1.MachineDrainRuleDrainBehaviorDrain, localStorageWarning)
}

func (d *Helper) unreplicatedFilter(_ context.Context, pod *corev1.Pod) PodDeleteStatus {
	// any finished pod can be removed
	if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
		return MakePodDeleteStatusOkay()
	}

	controllerRef := metav1.GetControllerOf(pod)
	if controllerRef != nil {
		return MakePodDeleteStatusOkay()
	}

	return MakePodDeleteStatusWithWarning(clusterv1.MachineDrainRuleDrainBehaviorDrain, unmanagedWarning)
}

func shouldSkipPod(pod *corev1.Pod, skipDeletedTimeoutSeconds int) bool {
	return skipDeletedTimeoutSeconds > 0 &&
		!pod.DeletionTimestamp.IsZero() &&
		int(time.Since(pod.ObjectMeta.GetDeletionTimestamp().Time).Seconds()) > skipDeletedTimeoutSeconds
}

func (d *Helper) skipDeletedFilter(ctx context.Context, pod *corev1.Pod) PodDeleteStatus {
	if shouldSkipPod(pod, d.SkipWaitForDeleteTimeoutSeconds) {
		log := ctrl.LoggerFrom(ctx, "Pod", klog.KObj(pod))
		log.V(4).Info(fmt.Sprintf("Skip evicting Pod, because Pod deletionTimestamp is more than %ds ago", d.SkipWaitForDeleteTimeoutSeconds))
		return MakePodDeleteStatusSkip()
	}
	return MakePodDeleteStatusOkay()
}

func (d *Helper) drainLabelFilter(ctx context.Context, pod *corev1.Pod) PodDeleteStatus {
	log := ctrl.LoggerFrom(ctx, "Pod", klog.KObj(pod))
	if labelValue, found := pod.Labels[clusterv1.PodDrainLabel]; found {
		switch {
		case strings.EqualFold(labelValue, string(clusterv1.MachineDrainRuleDrainBehaviorSkip)):
			log.V(4).Info(fmt.Sprintf("Skip evicting Pod, because Pod has %s label with %s value", clusterv1.PodDrainLabel, labelValue))
			return MakePodDeleteStatusSkip()
		case strings.EqualFold(strings.Replace(labelValue, "-", "", 1), string(clusterv1.MachineDrainRuleDrainBehaviorWaitCompleted)):
			log.V(4).Info(fmt.Sprintf("Skip evicting Pod, because Pod has %s label with %s value", clusterv1.PodDrainLabel, labelValue))
			return MakePodDeleteStatusWaitCompleted()
		default:
			log.V(4).Info(fmt.Sprintf("Warning: Pod has %s label with an unknown value: %s", clusterv1.PodDrainLabel, labelValue))
		}
	}
	return MakePodDeleteStatusOkay()
}

func (d *Helper) machineDrainRulesFilter(machineDrainRules []*clusterv1.MachineDrainRule, namespaces map[string]*corev1.Namespace) PodFilter {
	return func(ctx context.Context, pod *corev1.Pod) PodDeleteStatus {
		// Get the namespace of the Pod
		namespace, ok := namespaces[pod.Namespace]
		if !ok {
			return MakePodDeleteStatusWithError("Pod Namespace does not exist")
		}

		// Iterate through the MachineDrainRules (they are already alphabetically sorted).
		for _, mdr := range machineDrainRules {
			if !machineDrainRuleAppliesToPod(mdr, pod, namespace) {
				continue
			}

			// If the pod selector matches, use the drain behavior from the MachineDrainRule.
			log := ctrl.LoggerFrom(ctx, "Pod", klog.KObj(pod))
			switch mdr.Spec.Drain.Behavior {
			case clusterv1.MachineDrainRuleDrainBehaviorDrain:
				return MakePodDeleteStatusOkayWithOrder(mdr.Spec.Drain.Order)
			case clusterv1.MachineDrainRuleDrainBehaviorSkip:
				log.V(4).Info(fmt.Sprintf("Skip evicting Pod, because MachineDrainRule %s with behavior %s applies to the Pod", mdr.Name, clusterv1.MachineDrainRuleDrainBehaviorSkip))
				return MakePodDeleteStatusSkip()
			case clusterv1.MachineDrainRuleDrainBehaviorWaitCompleted:
				log.V(4).Info(fmt.Sprintf("Skip evicting Pod, because MachineDrainRule %s with behavior %s applies to the Pod", mdr.Name, clusterv1.MachineDrainRuleDrainBehaviorWaitCompleted))
				return MakePodDeleteStatusWaitCompleted()
			default:
				return MakePodDeleteStatusWithError(
					fmt.Sprintf("MachineDrainRule %q has unknown spec.drain.behavior: %q",
						mdr.Name, mdr.Spec.Drain.Behavior))
			}
		}

		// If no MachineDrainRule matches, use behavior: "Drain" and order: 0
		return MakePodDeleteStatusOkay()
	}
}

// machineDrainRuleAppliesToPod evaluates if a MachineDrainRule applies to a Pod.
func machineDrainRuleAppliesToPod(mdr *clusterv1.MachineDrainRule, pod *corev1.Pod, namespace *corev1.Namespace) bool {
	// If pods is empty, the MachineDrainRule applies to all Pods.
	if len(mdr.Spec.Pods) == 0 {
		return true
	}

	// The MachineDrainRule applies to a Pod if there is a PodSelector in MachineDrainRule.spec.pods
	// for which both the selector and namespaceSelector match.
	for _, selector := range mdr.Spec.Pods {
		if matchesSelector(selector.Selector, pod) &&
			matchesSelector(selector.NamespaceSelector, namespace) {
			return true
		}
	}

	return false
}

// matchesSelector evaluates if the labelSelector matches an object.
// The labelSelector matches if:
// * it is nil
// * it is empty
// * the selector matches the labels of the object.
func matchesSelector(labelSelector *metav1.LabelSelector, obj client.Object) bool {
	if labelSelector == nil {
		return true
	}

	// Ignoring the error, labelSelector was already validated before calling matchesSelector.
	selector, _ := metav1.LabelSelectorAsSelector(labelSelector)
	if selector.Empty() {
		return true
	}

	return selector.Matches(labels.Set(obj.GetLabels()))
}
