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
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Note: This file is still mostly kept in sync with: https://github.com/kubernetes/kubernetes/blob/v1.31.0/staging/src/k8s.io/kubectl/pkg/drain/filters.go
// Minor modifications have been made to drop branches that are never used in Cluster API and to use the
// controller-runtime Client.

const (
	daemonSetOrphanedWarning = "evicting orphaned DaemonSet-managed Pod"
	daemonSetWarning         = "ignoring DaemonSet-managed Pod"
	localStorageWarning      = "evicting Pod with local storage"
	unmanagedWarning         = "evicting Pod that have no controller"
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
		if i.Status.Delete {
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
		errs = append(errs, fmt.Errorf("cannot evict %s: %s", msg, strings.Join(pods, ", ")))
	}
	return errs
}

// PodDeleteStatus informs filters if a pod should be deleted.
type PodDeleteStatus struct {
	// Delete means that this Pod has to go away before the Node can be considered completely drained..
	Delete  bool
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
	// PodDeleteStatusTypeWarning is "Warning".
	PodDeleteStatusTypeWarning = "Warning"
	// PodDeleteStatusTypeError is "Error".
	PodDeleteStatusTypeError = "Error"
)

// MakePodDeleteStatusOkay is a helper method to return the corresponding PodDeleteStatus.
func MakePodDeleteStatusOkay() PodDeleteStatus {
	return PodDeleteStatus{
		Delete: true,
		Reason: PodDeleteStatusTypeOkay,
	}
}

// MakePodDeleteStatusSkip is a helper method to return the corresponding PodDeleteStatus.
func MakePodDeleteStatusSkip() PodDeleteStatus {
	return PodDeleteStatus{
		Delete: false,
		Reason: PodDeleteStatusTypeSkip,
	}
}

// MakePodDeleteStatusWithWarning is a helper method to return the corresponding PodDeleteStatus.
func MakePodDeleteStatusWithWarning(del bool, message string) PodDeleteStatus {
	return PodDeleteStatus{
		Delete:  del,
		Reason:  PodDeleteStatusTypeWarning,
		Message: message,
	}
}

// MakePodDeleteStatusWithError is a helper method to return the corresponding PodDeleteStatus.
func MakePodDeleteStatusWithError(message string) PodDeleteStatus {
	return PodDeleteStatus{
		Delete:  false,
		Reason:  PodDeleteStatusTypeError,
		Message: message,
	}
}

// The filters are applied in a specific order.
func (d *Helper) makeFilters() []PodFilter {
	return []PodFilter{
		d.skipDeletedFilter,
		d.daemonSetFilter,
		d.mirrorPodFilter,
		d.localStorageFilter,
		d.unreplicatedFilter,
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

	if err := d.Client.Get(ctx, client.ObjectKey{Namespace: pod.Namespace, Name: controllerRef.Name}, &appsv1.DaemonSet{}); err != nil {
		// remove orphaned pods with a warning
		if apierrors.IsNotFound(err) {
			return MakePodDeleteStatusWithWarning(true, daemonSetOrphanedWarning)
		}

		return MakePodDeleteStatusWithError(err.Error())
	}

	return MakePodDeleteStatusWithWarning(false, daemonSetWarning)
}

func (d *Helper) mirrorPodFilter(_ context.Context, pod *corev1.Pod) PodDeleteStatus {
	if _, found := pod.ObjectMeta.Annotations[corev1.MirrorPodAnnotationKey]; found {
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

	return MakePodDeleteStatusWithWarning(true, localStorageWarning)
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

	return MakePodDeleteStatusWithWarning(true, unmanagedWarning)
}

func shouldSkipPod(pod *corev1.Pod, skipDeletedTimeoutSeconds int) bool {
	return skipDeletedTimeoutSeconds > 0 &&
		!pod.ObjectMeta.DeletionTimestamp.IsZero() &&
		int(time.Since(pod.ObjectMeta.GetDeletionTimestamp().Time).Seconds()) > skipDeletedTimeoutSeconds
}

func (d *Helper) skipDeletedFilter(_ context.Context, pod *corev1.Pod) PodDeleteStatus {
	if shouldSkipPod(pod, d.SkipWaitForDeleteTimeoutSeconds) {
		return MakePodDeleteStatusSkip()
	}
	return MakePodDeleteStatusOkay()
}
