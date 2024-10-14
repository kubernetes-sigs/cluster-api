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
	"net/http"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestRunCordonOrUncordon(t *testing.T) {
	tests := []struct {
		name string
		node *corev1.Node
	}{
		{
			name: "Uncordoned Node should be cordoned",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-1",
				},
				Spec: corev1.NodeSpec{
					Unschedulable: false,
				},
			},
		},
		{
			name: "Cordoned Node should stay cordoned",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-1",
				},
				Spec: corev1.NodeSpec{
					Unschedulable: true,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			fakeClient := fake.NewClientBuilder().WithObjects(tt.node).Build()

			drainer := &Helper{
				Client: fakeClient,
			}

			g.Expect(drainer.CordonNode(context.Background(), tt.node)).To(Succeed())

			gotNode := tt.node.DeepCopy()
			g.Expect(fakeClient.Get(context.Background(), client.ObjectKeyFromObject(gotNode), gotNode)).To(Succeed())
			g.Expect(gotNode.Spec.Unschedulable).To(BeTrue())
		})
	}
}

func TestGetPodsForEviction(t *testing.T) {
	tests := []struct {
		name              string
		pods              []*corev1.Pod
		wantPodDeleteList PodDeleteList
	}{
		{
			name: "skipDeletedFilter",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "pod-1-skip-pod-old-deletionTimestamp",
						DeletionTimestamp: &metav1.Time{Time: time.Now().Add(time.Duration(1) * time.Minute * -1)},
						Finalizers:        []string{"block-deletion"},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "pod-2-delete-pod-new-deletionTimestamp",
						DeletionTimestamp: &metav1.Time{Time: time.Now()},
						Finalizers:        []string{"block-deletion"},
					},
				},
			},
			wantPodDeleteList: PodDeleteList{items: []PodDelete{
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-1-skip-pod-old-deletionTimestamp",
						},
					},
					// Skip this Pod because deletionTimestamp is > SkipWaitForDeleteTimeoutSeconds (=10s) ago.
					Status: PodDeleteStatus{
						Delete: false,
						Reason: PodDeleteStatusTypeSkip,
					},
				},
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-2-delete-pod-new-deletionTimestamp",
						},
					},
					// Delete this Pod because deletionTimestamp is < SkipWaitForDeleteTimeoutSeconds (=10s) ago.
					Status: PodDeleteStatus{
						Delete:  true,
						Reason:  PodDeleteStatusTypeWarning,
						Message: unmanagedWarning,
					},
				},
			}},
		},
		{
			name: "daemonSetFilter",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-1-delete-pod-with-different-controller",
						OwnerReferences: []metav1.OwnerReference{
							{
								Kind:       "Deployment",
								Controller: ptr.To(true),
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-2-delete-succeeded-daemonset-pod",
						OwnerReferences: []metav1.OwnerReference{
							{
								Kind:       "DaemonSet",
								Controller: ptr.To(true),
							},
						},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodSucceeded,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-3-delete-orphaned-daemonset-pod",
						OwnerReferences: []metav1.OwnerReference{
							{
								Kind:       "DaemonSet",
								Name:       "daemonset-does-not-exist",
								Controller: ptr.To(true),
							},
						},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-4-skip-daemonset-pod",
						OwnerReferences: []metav1.OwnerReference{
							{
								Kind:       "DaemonSet",
								Name:       "daemonset-does-exist",
								Controller: ptr.To(true),
							},
						},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
					},
				},
			},
			wantPodDeleteList: PodDeleteList{items: []PodDelete{
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-1-delete-pod-with-different-controller",
						},
					},
					// Delete this Pod because the controller is not a DaemonSet
					Status: PodDeleteStatus{
						Delete: true,
						Reason: PodDeleteStatusTypeOkay,
					},
				},
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-2-delete-succeeded-daemonset-pod",
						},
					},
					// Delete this DaemonSet Pod because it is succeeded.
					Status: PodDeleteStatus{
						Delete: true,
						Reason: PodDeleteStatusTypeOkay,
					},
				},
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-3-delete-orphaned-daemonset-pod",
						},
					},
					// Delete this DaemonSet Pod because it is orphaned.
					Status: PodDeleteStatus{
						Delete:  true,
						Reason:  PodDeleteStatusTypeWarning,
						Message: daemonSetOrphanedWarning,
					},
				},
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-4-skip-daemonset-pod",
						},
					},
					// Skip this DaemonSet Pod.
					Status: PodDeleteStatus{
						Delete:  false,
						Reason:  PodDeleteStatusTypeWarning,
						Message: daemonSetWarning,
					},
				},
			}},
		},
		{
			name: "mirrorPodFilter",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-1-skip-mirror-pod",
						Annotations: map[string]string{
							corev1.MirrorPodAnnotationKey: "some-value",
						},
					},
				},
			},
			wantPodDeleteList: PodDeleteList{items: []PodDelete{
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-1-skip-mirror-pod",
						},
					},
					// Skip this Pod because it is a mirror pod.
					Status: PodDeleteStatus{
						Delete: false,
						Reason: PodDeleteStatusTypeSkip,
					},
				},
			}},
		},
		{
			name: "localStorageFilter",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-1-delete-pod-without-local-storage",
						OwnerReferences: []metav1.OwnerReference{
							{
								Kind:       "Deployment",
								Controller: ptr.To(true),
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-2-delete-succeeded-pod-with-local-storage",
						OwnerReferences: []metav1.OwnerReference{
							{
								Kind:       "Deployment",
								Controller: ptr.To(true),
							},
						},
					},
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{
							{
								Name: "empty-dir",
								VolumeSource: corev1.VolumeSource{
									EmptyDir: &corev1.EmptyDirVolumeSource{},
								},
							},
						},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodSucceeded,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-3-delete-running-pod-with-local-storage",
						OwnerReferences: []metav1.OwnerReference{
							{
								Kind:       "Deployment",
								Controller: ptr.To(true),
							},
						},
					},
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{
							{
								Name: "empty-dir",
								VolumeSource: corev1.VolumeSource{
									EmptyDir: &corev1.EmptyDirVolumeSource{},
								},
							},
						},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
					},
				},
			},
			wantPodDeleteList: PodDeleteList{items: []PodDelete{
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-1-delete-pod-without-local-storage",
						},
					},
					// Delete regular Pod without local storage.
					Status: PodDeleteStatus{
						Delete: true,
						Reason: PodDeleteStatusTypeOkay,
					},
				},
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-2-delete-succeeded-pod-with-local-storage",
						},
					},
					// Delete succeeded Pod with local storage.
					Status: PodDeleteStatus{
						Delete: true,
						Reason: PodDeleteStatusTypeOkay,
					},
				},
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-3-delete-running-pod-with-local-storage",
						},
					},
					// Delete running Pod with local storage.
					Status: PodDeleteStatus{
						Delete:  true,
						Reason:  PodDeleteStatusTypeWarning,
						Message: localStorageWarning,
					},
				},
			}},
		},
		{
			name: "unreplicatedFilter",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-1-delete-succeeded-pod",
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodSucceeded,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-2-delete-running-deployment-pod",
						OwnerReferences: []metav1.OwnerReference{
							{
								Kind:       "Deployment",
								Controller: ptr.To(true),
							},
						},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-3-delete-running-standalone-pod",
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
					},
				},
			},
			wantPodDeleteList: PodDeleteList{items: []PodDelete{
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-1-delete-succeeded-pod",
						},
					},
					Status: PodDeleteStatus{
						Delete: true,
						Reason: PodDeleteStatusTypeOkay,
					},
				},
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-2-delete-running-deployment-pod",
						},
					},
					Status: PodDeleteStatus{
						Delete: true,
						Reason: PodDeleteStatusTypeOkay,
					},
				},
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-3-delete-running-standalone-pod",
						},
					},
					Status: PodDeleteStatus{
						Delete:  true,
						Reason:  PodDeleteStatusTypeWarning,
						Message: unmanagedWarning,
					},
				},
			}},
		},
		{
			name: "warnings from multiple filters",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-1-delete-multiple-warnings",
						OwnerReferences: []metav1.OwnerReference{
							{
								Kind:       "DaemonSet",
								Name:       "daemonset-does-not-exist",
								Controller: ptr.To(true),
							},
						},
					},
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{
							{
								Name: "empty-dir",
								VolumeSource: corev1.VolumeSource{
									EmptyDir: &corev1.EmptyDirVolumeSource{},
								},
							},
						},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
					},
				},
			},
			wantPodDeleteList: PodDeleteList{items: []PodDelete{
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-1-delete-multiple-warnings",
						},
					},
					Status: PodDeleteStatus{
						Delete:  true,
						Reason:  PodDeleteStatusTypeWarning,
						Message: daemonSetOrphanedWarning + ", " + localStorageWarning,
					},
				},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// Setting NodeName here to avoid noise in the table above.
			for i := range tt.pods {
				tt.pods[i].Spec.NodeName = "node-1"
			}

			var objs []client.Object
			for _, o := range tt.pods {
				objs = append(objs, o)
			}
			objs = append(objs, &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "daemonset-does-exist",
				},
			})

			fakeClient := fake.NewClientBuilder().
				WithObjects(objs...).
				WithIndex(&corev1.Pod{}, "spec.nodeName", podByNodeName).
				Build()
			drainer := &Helper{
				Client:                          fakeClient,
				SkipWaitForDeleteTimeoutSeconds: 10,
			}

			gotPodDeleteList, err := drainer.GetPodsForEviction(context.Background(), "node-1")
			g.Expect(err).ToNot(HaveOccurred())
			// Cleanup for easier diff.
			for i, pd := range gotPodDeleteList.items {
				gotPodDeleteList.items[i].Pod = &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: pd.Pod.Name,
					},
				}
			}
			g.Expect(gotPodDeleteList.items).To(BeComparableTo(tt.wantPodDeleteList.items))
		})
	}
}

func TestEvictPods(t *testing.T) {
	tests := []struct {
		name               string
		podDeleteList      *PodDeleteList
		wantEvictionResult EvictionResult
	}{
		{
			name: "EvictPods correctly",
			podDeleteList: &PodDeleteList{items: []PodDelete{
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-1-ignored",
						},
					},
					Status: PodDeleteStatus{
						Delete:  false, // Will be skipped because Delete is set to false.
						Reason:  PodDeleteStatusTypeWarning,
						Message: daemonSetWarning,
					},
				},
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:              "pod-2-deletionTimestamp-set",
							DeletionTimestamp: &metav1.Time{Time: time.Now()},
						},
					},
					Status: PodDeleteStatus{
						Delete: true,
						Reason: PodDeleteStatusTypeOkay,
					},
				},
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-3-to-trigger-eviction-successfully",
						},
					},
					Status: PodDeleteStatus{
						Delete: true,
						Reason: PodDeleteStatusTypeOkay,
					},
				},
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-4-to-trigger-eviction-pod-not-found",
						},
					},
					Status: PodDeleteStatus{
						Delete: true,
						Reason: PodDeleteStatusTypeOkay,
					},
				},
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-5-to-trigger-eviction-pdb-violated-2",
						},
					},
					Status: PodDeleteStatus{
						Delete: true,
						Reason: PodDeleteStatusTypeOkay,
					},
				},
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-5-to-trigger-eviction-pdb-violated-1",
						},
					},
					Status: PodDeleteStatus{
						Delete: true,
						Reason: PodDeleteStatusTypeOkay,
					},
				},
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-6-to-trigger-eviction-namespace-terminating",
						},
					},
					Status: PodDeleteStatus{
						Delete: true,
						Reason: PodDeleteStatusTypeOkay,
					},
				},
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-7-to-trigger-eviction-some-other-error",
						},
					},
					Status: PodDeleteStatus{
						Delete: true,
						Reason: PodDeleteStatusTypeOkay,
					},
				},
			}},
			wantEvictionResult: EvictionResult{
				PodsIgnored: []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-1-ignored",
						},
					},
				},
				PodsDeletionTimestampSet: []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-2-deletionTimestamp-set",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-3-to-trigger-eviction-successfully",
						},
					},
				},
				PodsNotFound: []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-4-to-trigger-eviction-pod-not-found",
						},
					},
				},
				PodsFailedEviction: map[string][]*corev1.Pod{
					"Cannot evict pod as it would violate the pod's disruption budget. The disruption budget pod-5-pdb needs 3 healthy pods and has 2 currently": {
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "pod-5-to-trigger-eviction-pdb-violated-1",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "pod-5-to-trigger-eviction-pdb-violated-2",
							},
						},
					},
					"Cannot evict pod from terminating namespace: unable to create eviction (kube-controller-manager should set deletionTimestamp)": {
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "pod-6-to-trigger-eviction-namespace-terminating",
							},
						},
					},
					"some other error": {
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "pod-7-to-trigger-eviction-some-other-error",
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			fakeClient := fake.NewClientBuilder().WithObjects().Build()

			podResource := schema.GroupResource{Group: corev1.GroupName, Resource: "pods"}

			fakeClient = interceptor.NewClient(fakeClient, interceptor.Funcs{
				SubResourceCreate: func(_ context.Context, _ client.Client, subResourceName string, obj client.Object, _ client.Object, _ ...client.SubResourceCreateOption) error {
					g.Expect(subResourceName).To(Equal("eviction"))
					switch name := obj.GetName(); name {
					case "pod-3-to-trigger-eviction-successfully":
						return nil // Successful eviction.
					case "pod-4-to-trigger-eviction-pod-not-found":
						return apierrors.NewNotFound(podResource, name)
					case "pod-5-to-trigger-eviction-pdb-violated-1", "pod-5-to-trigger-eviction-pdb-violated-2":
						return &apierrors.StatusError{
							ErrStatus: metav1.Status{
								Status:  metav1.StatusFailure,
								Code:    http.StatusTooManyRequests,
								Reason:  metav1.StatusReasonTooManyRequests,
								Message: "Cannot evict pod as it would violate the pod's disruption budget.",
								Details: &metav1.StatusDetails{
									Causes: []metav1.StatusCause{
										{
											Type:    "DisruptionBudget",
											Message: "The disruption budget pod-5-pdb needs 3 healthy pods and has 2 currently",
										},
									},
								},
							},
						}
					case "pod-6-to-trigger-eviction-namespace-terminating":
						return &apierrors.StatusError{
							ErrStatus: metav1.Status{
								Status:  metav1.StatusFailure,
								Code:    http.StatusForbidden,
								Reason:  metav1.StatusReasonForbidden,
								Message: "pods \"pod-6-to-trigger-eviction-namespace-terminating\" is forbidden: unable to create new content in namespace test-namespace because it is being terminated",
								Details: &metav1.StatusDetails{
									Name: "pod-6-to-trigger-eviction-namespace-terminating",
									Kind: "pods",
									Causes: []metav1.StatusCause{
										{
											Type:    corev1.NamespaceTerminatingCause,
											Message: "namespace test-namespace is being terminated",
											Field:   "metadata.namespace",
										},
									},
								},
							},
						}
					case "pod-7-to-trigger-eviction-some-other-error":
						return apierrors.NewBadRequest("some other error")
					}

					g.Fail(fmt.Sprintf("eviction behavior for Pod %q not implemented", obj.GetName()))
					return nil
				},
			})

			drainer := &Helper{
				Client: fakeClient,
			}

			gotEvictionResult := drainer.EvictPods(context.Background(), tt.podDeleteList)
			// Cleanup for easier diff.
			for i, pod := range gotEvictionResult.PodsDeletionTimestampSet {
				gotEvictionResult.PodsDeletionTimestampSet[i] = &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: pod.Name,
					},
				}
			}
			g.Expect(gotEvictionResult).To(BeComparableTo(tt.wantEvictionResult))
		})
	}
}

func TestEvictionResult_ConditionMessage(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name                 string
		evictionResult       EvictionResult
		wantConditionMessage string
	}{
		{
			name:                 "Compute no condition message correctly",
			evictionResult:       EvictionResult{}, // Drain completed.
			wantConditionMessage: ``,
		},
		{
			name: "Compute short condition message correctly",
			evictionResult: EvictionResult{
				PodsDeletionTimestampSet: []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-2-deletionTimestamp-set-1",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-3-to-trigger-eviction-successfully-1",
						},
					},
				},
				PodsFailedEviction: map[string][]*corev1.Pod{
					"Cannot evict pod as it would violate the pod's disruption budget. The disruption budget pod-5-pdb needs 20 healthy pods and has 20 currently": {
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "pod-5-to-trigger-eviction-pdb-violated-1",
							},
						},
					},
					"some other error 1": {
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "pod-6-to-trigger-eviction-some-other-error",
							},
						},
					},
				},
			},
			wantConditionMessage: `Drain not completed yet (started at 2024-10-09T16:13:59Z):
* Pods with deletionTimestamp that still exist: pod-2-deletionTimestamp-set-1, pod-3-to-trigger-eviction-successfully-1
* Pods with eviction failed:
  * Cannot evict pod as it would violate the pod's disruption budget. The disruption budget pod-5-pdb needs 20 healthy pods and has 20 currently: pod-5-to-trigger-eviction-pdb-violated-1
  * some other error 1: pod-6-to-trigger-eviction-some-other-error`,
		},
		{
			name: "Compute long condition message correctly",
			evictionResult: EvictionResult{
				PodsIgnored: []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-1-ignored-should-not-be-included",
						},
					},
				},
				PodsDeletionTimestampSet: []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-2-deletionTimestamp-set-1",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-2-deletionTimestamp-set-2",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-2-deletionTimestamp-set-3",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-3-to-trigger-eviction-successfully-1",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-3-to-trigger-eviction-successfully-2",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-3-to-trigger-eviction-successfully-3-should-not-be-included", // only first 5.
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-3-to-trigger-eviction-successfully-4-should-not-be-included", // only first 5.
						},
					},
				},
				PodsNotFound: []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-4-to-trigger-eviction-pod-not-found-should-not-be-included",
						},
					},
				},
				PodsFailedEviction: map[string][]*corev1.Pod{
					"Cannot evict pod as it would violate the pod's disruption budget. The disruption budget pod-5-pdb needs 20 healthy pods and has 20 currently": {
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "pod-5-to-trigger-eviction-pdb-violated-1",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "pod-5-to-trigger-eviction-pdb-violated-2",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "pod-5-to-trigger-eviction-pdb-violated-3",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "pod-5-to-trigger-eviction-pdb-violated-4-should-not-be-included", // only first 3
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "pod-5-to-trigger-eviction-pdb-violated-5-should-not-be-included", // only first 3
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "pod-5-to-trigger-eviction-pdb-violated-6-should-not-be-included", // only first 3
							},
						},
					},
					"some other error 1": {
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "pod-6-to-trigger-eviction-some-other-error",
							},
						},
					},
					"some other error 2": {
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "pod-7-to-trigger-eviction-some-other-error",
							},
						},
					},
					"some other error 3": {
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "pod-8-to-trigger-eviction-some-other-error",
							},
						},
					},
					"some other error 4": {
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "pod-9-to-trigger-eviction-some-other-error",
							},
						},
					},
					"some other error 5 should not be included": { // only first 5
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "pod-10-to-trigger-eviction-some-other-error",
							},
						},
					},
				},
			},
			wantConditionMessage: `Drain not completed yet (started at 2024-10-09T16:13:59Z):
* Pods with deletionTimestamp that still exist: pod-2-deletionTimestamp-set-1, pod-2-deletionTimestamp-set-2, pod-2-deletionTimestamp-set-3, pod-3-to-trigger-eviction-successfully-1, pod-3-to-trigger-eviction-successfully-2, ... (2 more)
* Pods with eviction failed:
  * Cannot evict pod as it would violate the pod's disruption budget. The disruption budget pod-5-pdb needs 20 healthy pods and has 20 currently: pod-5-to-trigger-eviction-pdb-violated-1, pod-5-to-trigger-eviction-pdb-violated-2, pod-5-to-trigger-eviction-pdb-violated-3, ... (3 more)
  * some other error 1: pod-6-to-trigger-eviction-some-other-error
  * some other error 2: pod-7-to-trigger-eviction-some-other-error
  * some other error 3: pod-8-to-trigger-eviction-some-other-error
  * some other error 4: pod-9-to-trigger-eviction-some-other-error
  * ... (1 more error applying to 1 Pod)`,
		},
		{
			name: "Compute long condition message correctly with more skipped errors",
			evictionResult: EvictionResult{
				PodsFailedEviction: map[string][]*corev1.Pod{
					"some other error 1": {
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "pod-1-to-trigger-eviction-some-other-error",
							},
						},
					},
					"some other error 2": {
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "pod-2-to-trigger-eviction-some-other-error",
							},
						},
					},
					"some other error 3": {
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "pod-3-to-trigger-eviction-some-other-error",
							},
						},
					},
					"some other error 4": {
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "pod-4-to-trigger-eviction-some-other-error",
							},
						},
					},
					"some other error 5": {
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "pod-5-to-trigger-eviction-some-other-error",
							},
						},
					},
					"some other error 6 should not be included": { // only first 5
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "pod-6-to-trigger-eviction-some-other-error-1",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "pod-6-to-trigger-eviction-some-other-error-2",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "pod-6-to-trigger-eviction-some-other-error-3",
							},
						},
					},
					"some other error 7 should not be included": { // only first 5
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "pod-7-to-trigger-eviction-some-other-error",
							},
						},
					},
				},
			},
			wantConditionMessage: `Drain not completed yet (started at 2024-10-09T16:13:59Z):
* Pods with eviction failed:
  * some other error 1: pod-1-to-trigger-eviction-some-other-error
  * some other error 2: pod-2-to-trigger-eviction-some-other-error
  * some other error 3: pod-3-to-trigger-eviction-some-other-error
  * some other error 4: pod-4-to-trigger-eviction-some-other-error
  * some other error 5: pod-5-to-trigger-eviction-some-other-error
  * ... (2 more errors applying to 4 Pods)`,
		},
	}

	nodeDrainStartTime, err := time.Parse(time.RFC3339, "2024-10-09T16:13:59Z")
	g.Expect(err).ToNot(HaveOccurred())

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			g.Expect(tt.evictionResult.ConditionMessage(&metav1.Time{Time: nodeDrainStartTime})).To(Equal(tt.wantConditionMessage))
		})
	}
}

func podByNodeName(o client.Object) []string {
	pod, ok := o.(*corev1.Pod)
	if !ok {
		panic(fmt.Sprintf("Expected a Pod but got a %T", o))
	}

	if pod.Spec.NodeName == "" {
		return nil
	}

	return []string{pod.Spec.NodeName}
}
