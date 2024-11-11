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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
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
				RemoteClient: fakeClient,
			}

			g.Expect(drainer.CordonNode(context.Background(), tt.node)).To(Succeed())

			gotNode := tt.node.DeepCopy()
			g.Expect(fakeClient.Get(context.Background(), client.ObjectKeyFromObject(gotNode), gotNode)).To(Succeed())
			g.Expect(gotNode.Spec.Unschedulable).To(BeTrue())
		})
	}
}

func TestGetPodsForEviction(t *testing.T) {
	mdrBehaviorDrain := &clusterv1.MachineDrainRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mdr-behavior-drain",
			Namespace: "test-namespace",
		},
		Spec: clusterv1.MachineDrainRuleSpec{
			Drain: clusterv1.MachineDrainRuleDrainConfig{
				Behavior: clusterv1.MachineDrainRuleDrainBehaviorDrain,
				Order:    ptr.To[int32](11),
			},
			Machines: nil, // Match all machines
			Pods: []clusterv1.MachineDrainRulePodSelector{
				{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "behavior-drain",
						},
					},
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"kubernetes.io/metadata.name": "test-namespace",
						},
					},
				},
			},
		},
	}
	mdrBehaviorSkip := &clusterv1.MachineDrainRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mdr-behavior-skip",
			Namespace: "test-namespace",
		},
		Spec: clusterv1.MachineDrainRuleSpec{
			Drain: clusterv1.MachineDrainRuleDrainConfig{
				Behavior: clusterv1.MachineDrainRuleDrainBehaviorSkip,
			},
			Machines: nil, // Match all machines
			Pods: []clusterv1.MachineDrainRulePodSelector{
				{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "behavior-skip",
						},
					},
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"kubernetes.io/metadata.name": "test-namespace",
						},
					},
				},
			},
		},
	}
	mdrBehaviorUnknown := &clusterv1.MachineDrainRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mdr-behavior-unknown",
			Namespace: "test-namespace",
		},
		Spec: clusterv1.MachineDrainRuleSpec{
			Drain: clusterv1.MachineDrainRuleDrainConfig{
				Behavior: "Unknown",
			},
			Machines: nil, // Match all machines
			Pods: []clusterv1.MachineDrainRulePodSelector{
				{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "behavior-unknown",
						},
					},
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"kubernetes.io/metadata.name": "test-namespace",
						},
					},
				},
			},
		},
	}

	tests := []struct {
		name              string
		pods              []*corev1.Pod
		machineDrainRules []*clusterv1.MachineDrainRule
		wantPodDeleteList PodDeleteList
		wantErr           string
	}{
		{
			name: "skipDeletedFilter",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "pod-1-skip-pod-old-deletionTimestamp",
						Namespace:         metav1.NamespaceDefault,
						DeletionTimestamp: &metav1.Time{Time: time.Now().Add(time.Duration(1) * time.Minute * -1)},
						Finalizers:        []string{"block-deletion"},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "pod-2-delete-pod-new-deletionTimestamp",
						Namespace:         metav1.NamespaceDefault,
						DeletionTimestamp: &metav1.Time{Time: time.Now()},
						Finalizers:        []string{"block-deletion"},
					},
				},
			},
			wantPodDeleteList: PodDeleteList{items: []PodDelete{
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "pod-1-skip-pod-old-deletionTimestamp",
							Namespace: metav1.NamespaceDefault,
						},
					},
					// Skip this Pod because deletionTimestamp is > SkipWaitForDeleteTimeoutSeconds (=10s) ago.
					Status: PodDeleteStatus{
						DrainBehavior: clusterv1.MachineDrainRuleDrainBehaviorSkip,
						Reason:        PodDeleteStatusTypeSkip,
					},
				},
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "pod-2-delete-pod-new-deletionTimestamp",
							Namespace: metav1.NamespaceDefault,
						},
					},
					// Delete this Pod because deletionTimestamp is < SkipWaitForDeleteTimeoutSeconds (=10s) ago.
					Status: PodDeleteStatus{
						DrainBehavior: clusterv1.MachineDrainRuleDrainBehaviorDrain,
						DrainOrder:    ptr.To[int32](0),
						Reason:        PodDeleteStatusTypeWarning,
						Message:       unmanagedWarning,
					},
				},
			}},
		},
		{
			name: "daemonSetFilter",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod-1-delete-pod-with-different-controller",
						Namespace: metav1.NamespaceDefault,
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
						Name:      "pod-2-delete-succeeded-daemonset-pod",
						Namespace: metav1.NamespaceDefault,
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
						Name:      "pod-3-delete-orphaned-daemonset-pod",
						Namespace: metav1.NamespaceDefault,
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
						Name:      "pod-4-skip-daemonset-pod",
						Namespace: metav1.NamespaceDefault,
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
							Name:      "pod-1-delete-pod-with-different-controller",
							Namespace: metav1.NamespaceDefault,
						},
					},
					// Delete this Pod because the controller is not a DaemonSet
					Status: PodDeleteStatus{
						DrainBehavior: clusterv1.MachineDrainRuleDrainBehaviorDrain,
						DrainOrder:    ptr.To[int32](0),
						Reason:        PodDeleteStatusTypeOkay,
					},
				},
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "pod-2-delete-succeeded-daemonset-pod",
							Namespace: metav1.NamespaceDefault,
						},
					},
					// Delete this DaemonSet Pod because it is succeeded.
					Status: PodDeleteStatus{
						DrainBehavior: clusterv1.MachineDrainRuleDrainBehaviorDrain,
						DrainOrder:    ptr.To[int32](0),
						Reason:        PodDeleteStatusTypeOkay,
					},
				},
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "pod-3-delete-orphaned-daemonset-pod",
							Namespace: metav1.NamespaceDefault,
						},
					},
					// Delete this DaemonSet Pod because it is orphaned.
					Status: PodDeleteStatus{
						DrainBehavior: clusterv1.MachineDrainRuleDrainBehaviorDrain,
						DrainOrder:    ptr.To[int32](0),
						Reason:        PodDeleteStatusTypeWarning,
						Message:       daemonSetOrphanedWarning,
					},
				},
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "pod-4-skip-daemonset-pod",
							Namespace: metav1.NamespaceDefault,
						},
					},
					// Skip this DaemonSet Pod.
					Status: PodDeleteStatus{
						DrainBehavior: clusterv1.MachineDrainRuleDrainBehaviorSkip,
						Reason:        PodDeleteStatusTypeWarning,
						Message:       daemonSetWarning,
					},
				},
			}},
		},
		{
			name: "mirrorPodFilter",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod-1-skip-mirror-pod",
						Namespace: metav1.NamespaceDefault,
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
							Name:      "pod-1-skip-mirror-pod",
							Namespace: metav1.NamespaceDefault,
						},
					},
					// Skip this Pod because it is a mirror pod.
					Status: PodDeleteStatus{
						DrainBehavior: clusterv1.MachineDrainRuleDrainBehaviorSkip,
						Reason:        PodDeleteStatusTypeSkip,
					},
				},
			}},
		},
		{
			name: "localStorageFilter",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod-1-delete-pod-without-local-storage",
						Namespace: metav1.NamespaceDefault,
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
						Name:      "pod-2-delete-succeeded-pod-with-local-storage",
						Namespace: metav1.NamespaceDefault,
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
						Name:      "pod-3-delete-running-pod-with-local-storage",
						Namespace: metav1.NamespaceDefault,
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
							Name:      "pod-1-delete-pod-without-local-storage",
							Namespace: metav1.NamespaceDefault,
						},
					},
					// Delete regular Pod without local storage.
					Status: PodDeleteStatus{
						DrainBehavior: clusterv1.MachineDrainRuleDrainBehaviorDrain,
						DrainOrder:    ptr.To[int32](0),
						Reason:        PodDeleteStatusTypeOkay,
					},
				},
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "pod-2-delete-succeeded-pod-with-local-storage",
							Namespace: metav1.NamespaceDefault,
						},
					},
					// Delete succeeded Pod with local storage.
					Status: PodDeleteStatus{
						DrainBehavior: clusterv1.MachineDrainRuleDrainBehaviorDrain,
						DrainOrder:    ptr.To[int32](0),
						Reason:        PodDeleteStatusTypeOkay,
					},
				},
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "pod-3-delete-running-pod-with-local-storage",
							Namespace: metav1.NamespaceDefault,
						},
					},
					// Delete running Pod with local storage.
					Status: PodDeleteStatus{
						DrainBehavior: clusterv1.MachineDrainRuleDrainBehaviorDrain,
						DrainOrder:    ptr.To[int32](0),
						Reason:        PodDeleteStatusTypeWarning,
						Message:       localStorageWarning,
					},
				},
			}},
		},
		{
			name: "unreplicatedFilter",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod-1-delete-succeeded-pod",
						Namespace: metav1.NamespaceDefault,
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodSucceeded,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod-2-delete-running-deployment-pod",
						Namespace: metav1.NamespaceDefault,
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
						Name:      "pod-3-delete-running-standalone-pod",
						Namespace: metav1.NamespaceDefault,
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
							Name:      "pod-1-delete-succeeded-pod",
							Namespace: metav1.NamespaceDefault,
						},
					},
					Status: PodDeleteStatus{
						DrainBehavior: clusterv1.MachineDrainRuleDrainBehaviorDrain,
						DrainOrder:    ptr.To[int32](0),
						Reason:        PodDeleteStatusTypeOkay,
					},
				},
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "pod-2-delete-running-deployment-pod",
							Namespace: metav1.NamespaceDefault,
						},
					},
					Status: PodDeleteStatus{
						DrainBehavior: clusterv1.MachineDrainRuleDrainBehaviorDrain,
						DrainOrder:    ptr.To[int32](0),
						Reason:        PodDeleteStatusTypeOkay,
					},
				},
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "pod-3-delete-running-standalone-pod",
							Namespace: metav1.NamespaceDefault,
						},
					},
					Status: PodDeleteStatus{
						DrainBehavior: clusterv1.MachineDrainRuleDrainBehaviorDrain,
						DrainOrder:    ptr.To[int32](0),
						Reason:        PodDeleteStatusTypeWarning,
						Message:       unmanagedWarning,
					},
				},
			}},
		},
		{
			name: "warnings from multiple filters",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod-1-delete-multiple-warnings",
						Namespace: metav1.NamespaceDefault,
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
							Name:      "pod-1-delete-multiple-warnings",
							Namespace: metav1.NamespaceDefault,
						},
					},
					Status: PodDeleteStatus{
						DrainBehavior: clusterv1.MachineDrainRuleDrainBehaviorDrain,
						DrainOrder:    ptr.To[int32](0),
						Reason:        PodDeleteStatusTypeWarning,
						Message:       daemonSetOrphanedWarning + ", " + localStorageWarning,
					},
				},
			}},
		},
		{
			name: "drainLabelFilter",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod-1-skip-pod-with-drain-label",
						Namespace: metav1.NamespaceDefault,
						Labels: map[string]string{
							clusterv1.PodDrainLabel: "skip",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod-2-skip-pod-with-drain-label",
						Namespace: metav1.NamespaceDefault,
						Labels: map[string]string{
							clusterv1.PodDrainLabel: "Skip",
						},
					},
				},
			},
			wantPodDeleteList: PodDeleteList{items: []PodDelete{
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "pod-1-skip-pod-with-drain-label",
							Namespace: metav1.NamespaceDefault,
						},
					},
					Status: PodDeleteStatus{
						DrainBehavior: clusterv1.MachineDrainRuleDrainBehaviorSkip,
						Reason:        PodDeleteStatusTypeSkip,
					},
				},
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "pod-2-skip-pod-with-drain-label",
							Namespace: metav1.NamespaceDefault,
						},
					},
					Status: PodDeleteStatus{
						DrainBehavior: clusterv1.MachineDrainRuleDrainBehaviorSkip,
						Reason:        PodDeleteStatusTypeSkip,
					},
				},
			}},
		},
		{
			name: "machineDrainRulesFilter - error: Namespace does not exist",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod-1-non-existing-namespace",
						Namespace: "non-existing-namespace",
					},
				},
			},
			wantErr: "failed to get Pods for eviction: Pods with error \"Pod Namespace does not exist\": non-existing-namespace/pod-1-non-existing-namespace",
		},
		{
			name: "machineDrainRulesFilter - error: unknown spec.drain.behavior",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod-1-behavior-unknown",
						Namespace: "test-namespace", // matches the Namespace of the selector in mdrBehaviorUnknown.
						Labels: map[string]string{
							"app": "behavior-unknown", // matches mdrBehaviorUnknown.
						},
					},
				},
			},
			machineDrainRules: []*clusterv1.MachineDrainRule{mdrBehaviorDrain, mdrBehaviorSkip, mdrBehaviorUnknown},
			wantErr:           "failed to get Pods for eviction: Pods with error \"MachineDrainRule \\\"mdr-behavior-unknown\\\" has unknown spec.drain.behavior: \\\"Unknown\\\"\": test-namespace/pod-1-behavior-unknown",
		},
		{
			name: "machineDrainRulesFilter",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod-1-behavior-drain",
						Namespace: "test-namespace", // matches the Namespace of the selector in mdrBehaviorDrain.
						Labels: map[string]string{
							"app": "behavior-drain", // matches mdrBehaviorDrain.
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod-2-behavior-skip",
						Namespace: "test-namespace", // matches the Namespace of the selector in mdrBehaviorSkip.
						Labels: map[string]string{
							"app": "behavior-skip", // matches mdrBehaviorSkip.
						},
					},
				},
			},
			machineDrainRules: []*clusterv1.MachineDrainRule{mdrBehaviorDrain, mdrBehaviorSkip, mdrBehaviorUnknown},
			wantPodDeleteList: PodDeleteList{items: []PodDelete{
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "pod-1-behavior-drain",
							Namespace: "test-namespace",
						},
					},
					Status: PodDeleteStatus{
						DrainBehavior: clusterv1.MachineDrainRuleDrainBehaviorDrain,
						DrainOrder:    ptr.To[int32](11),
						// Preserve warning from other filters.
						Reason:  PodDeleteStatusTypeWarning,
						Message: "evicting Pod that has no controller",
					},
				},
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "pod-2-behavior-skip",
							Namespace: "test-namespace",
						},
					},
					Status: PodDeleteStatus{
						DrainBehavior: clusterv1.MachineDrainRuleDrainBehaviorSkip,
						Reason:        PodDeleteStatusTypeSkip,
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

			var remoteObjects []client.Object
			for _, o := range tt.pods {
				remoteObjects = append(remoteObjects, o)
			}
			remoteObjects = append(remoteObjects, &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "daemonset-does-exist",
					Namespace: metav1.NamespaceDefault,
				},
			}, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: metav1.NamespaceDefault,
					Labels: map[string]string{
						"kubernetes.io/metadata.name": metav1.NamespaceDefault,
					},
				},
			}, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-namespace",
					Labels: map[string]string{
						"kubernetes.io/metadata.name": "test-namespace",
					},
				},
			})

			fakeRemoteClient := fake.NewClientBuilder().
				WithObjects(remoteObjects...).
				WithIndex(&corev1.Pod{}, "spec.nodeName", podByNodeName).
				Build()

			cluster := &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-namespace",
				},
			}

			machine := &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-machine",
					Namespace: "test-namespace",
				},
			}

			var objects []client.Object
			for _, o := range tt.machineDrainRules {
				objects = append(objects, o)
			}
			scheme := runtime.NewScheme()
			g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())
			fakeClient := fake.NewClientBuilder().
				WithObjects(objects...).
				WithScheme(scheme).
				Build()

			drainer := &Helper{
				Client:                          fakeClient,
				RemoteClient:                    fakeRemoteClient,
				SkipWaitForDeleteTimeoutSeconds: 10,
			}

			gotPodDeleteList, err := drainer.GetPodsForEviction(context.Background(), cluster, machine, "node-1")
			if tt.wantErr != "" {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(BeComparableTo(tt.wantErr))
				return
			}

			g.Expect(err).ToNot(HaveOccurred())
			// Cleanup for easier diff.
			for i, pd := range gotPodDeleteList.items {
				gotPodDeleteList.items[i].Pod = &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      pd.Pod.Name,
						Namespace: pd.Pod.Namespace,
					},
				}
			}
			g.Expect(gotPodDeleteList.items).To(BeComparableTo(tt.wantPodDeleteList.items))
		})
	}
}

func Test_getMatchingMachineDrainRules(t *testing.T) {
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-namespace",
		},
	}
	machine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-machine",
			Namespace: "test-namespace",
		},
	}

	mdrInvalidSelector := &clusterv1.MachineDrainRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mdr-invalid-selector",
			Namespace: "test-namespace",
		},
		Spec: clusterv1.MachineDrainRuleSpec{
			Drain: clusterv1.MachineDrainRuleDrainConfig{
				Behavior: clusterv1.MachineDrainRuleDrainBehaviorSkip,
			},
			Machines: []clusterv1.MachineDrainRuleMachineSelector{
				{
					Selector: &metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Operator: "Invalid-Operator",
							},
						},
					},
				},
			},
			Pods: nil, // Match all Pods
		},
	}
	matchingMDRBehaviorDrainA := &clusterv1.MachineDrainRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mdr-behavior-drain-a",
			Namespace: "test-namespace",
		},
		Spec: clusterv1.MachineDrainRuleSpec{
			Drain: clusterv1.MachineDrainRuleDrainConfig{
				Behavior: clusterv1.MachineDrainRuleDrainBehaviorDrain,
				Order:    ptr.To[int32](11),
			},
			Machines: nil, // Match all machines
			Pods:     nil, // Match all Pods
		},
	}
	matchingMDRBehaviorDrainB := &clusterv1.MachineDrainRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mdr-behavior-drain-b",
			Namespace: "test-namespace",
		},
		Spec: clusterv1.MachineDrainRuleSpec{
			Drain: clusterv1.MachineDrainRuleDrainConfig{
				Behavior: clusterv1.MachineDrainRuleDrainBehaviorDrain,
				Order:    ptr.To[int32](15),
			},
			Machines: nil, // Match all machines
			Pods:     nil, // Match all Pods
		},
	}
	matchingMDRBehaviorSkip := &clusterv1.MachineDrainRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mdr-behavior-skip",
			Namespace: "test-namespace",
		},
		Spec: clusterv1.MachineDrainRuleSpec{
			Drain: clusterv1.MachineDrainRuleDrainConfig{
				Behavior: clusterv1.MachineDrainRuleDrainBehaviorSkip,
			},
			Machines: nil, // Match all machines
			Pods:     nil, // Match all Pods
		},
	}
	matchingMDRBehaviorUnknown := &clusterv1.MachineDrainRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mdr-behavior-unknown",
			Namespace: "test-namespace",
		},
		Spec: clusterv1.MachineDrainRuleSpec{
			Drain: clusterv1.MachineDrainRuleDrainConfig{
				Behavior: "Unknown",
			},
			Machines: nil, // Match all machines
			Pods:     nil, // Match all Pods
		},
	}
	notMatchingMDRDifferentNamespace := &clusterv1.MachineDrainRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mdr-not-matching-different-namespace",
			Namespace: "different-namespace",
		},
		Spec: clusterv1.MachineDrainRuleSpec{
			Drain: clusterv1.MachineDrainRuleDrainConfig{
				Behavior: clusterv1.MachineDrainRuleDrainBehaviorSkip,
			},
			Machines: nil, // Match all machines
			Pods:     nil, // Match all Pods
		},
	}
	notMatchingMDRNotMatchingSelector := &clusterv1.MachineDrainRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mdr-not-matching-not-matching-selector",
			Namespace: "test-namespace",
		},
		Spec: clusterv1.MachineDrainRuleSpec{
			Drain: clusterv1.MachineDrainRuleDrainConfig{
				Behavior: clusterv1.MachineDrainRuleDrainBehaviorSkip,
			},
			Machines: []clusterv1.MachineDrainRuleMachineSelector{
				{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"os": "does-not-match",
						},
					},
				},
			},
			Pods: nil, // Match all Pods
		},
	}

	tests := []struct {
		name                  string
		machineDrainRules     []*clusterv1.MachineDrainRule
		wantMachineDrainRules []*clusterv1.MachineDrainRule
		wantErr               string
	}{
		{
			name: "Return error for MachineDrainRules with invalid selector",
			machineDrainRules: []*clusterv1.MachineDrainRule{
				mdrInvalidSelector,
			},
			wantErr: "failed to get matching MachineDrainRules: invalid selectors in MachineDrainRule mdr-invalid-selector",
		},
		{
			name: "Return matching MachineDrainRules in correct order",
			machineDrainRules: []*clusterv1.MachineDrainRule{
				// Intentionally passing in A & B in inverse alphabetical order to validate sorting
				matchingMDRBehaviorDrainB,
				matchingMDRBehaviorDrainA,
				matchingMDRBehaviorSkip,
				matchingMDRBehaviorUnknown,
				notMatchingMDRDifferentNamespace,
				notMatchingMDRNotMatchingSelector,
			},
			wantMachineDrainRules: []*clusterv1.MachineDrainRule{
				matchingMDRBehaviorDrainA,
				matchingMDRBehaviorDrainB,
				matchingMDRBehaviorSkip,
				matchingMDRBehaviorUnknown,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			var objects []client.Object
			for _, o := range tt.machineDrainRules {
				objects = append(objects, o)
			}
			scheme := runtime.NewScheme()
			g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())
			fakeClient := fake.NewClientBuilder().
				WithObjects(objects...).
				WithScheme(scheme).
				Build()

			drainer := &Helper{
				Client: fakeClient,
			}

			gotMachineDrainRules, err := drainer.getMatchingMachineDrainRules(context.Background(), cluster, machine)
			if tt.wantErr != "" {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(tt.wantErr))
				return
			}

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(gotMachineDrainRules).To(BeComparableTo(tt.wantMachineDrainRules))
		})
	}
}

func Test_machineDrainRuleAppliesToMachine(t *testing.T) {
	tests := []struct {
		name             string
		machineSelectors []clusterv1.MachineDrainRuleMachineSelector
		machine          *clusterv1.Machine
		cluster          *clusterv1.Cluster
		matches          bool
	}{
		{
			name:             "nil always matches",
			machineSelectors: nil,
			matches:          true,
		},
		{
			name:             "empty always matches",
			machineSelectors: []clusterv1.MachineDrainRuleMachineSelector{},
			matches:          true,
		},
		{
			name: "matches if one entire MachineSelector matches",
			machineSelectors: []clusterv1.MachineDrainRuleMachineSelector{
				{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"os": "does-not-match",
						},
					},
					ClusterSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"stage": "does-not-match",
						},
					},
				},
				{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"os": "linux",
						},
					},
					ClusterSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"stage": "production",
						},
					},
				},
			},
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"os": "linux",
					},
				},
			},
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"stage": "production",
					},
				},
			},
			matches: true,
		},
		{
			name: "does not match if only MachineSelector.selector matches",
			machineSelectors: []clusterv1.MachineDrainRuleMachineSelector{
				{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"os": "linux",
						},
					},
					ClusterSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"stage": "does-not-match",
						},
					},
				},
			},
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"os": "linux",
					},
				},
			},
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"stage": "production",
					},
				},
			},
			matches: false,
		},
		{
			name: "does not match if only MachineSelector.clusterSelector matches",
			machineSelectors: []clusterv1.MachineDrainRuleMachineSelector{
				{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"os": "does-not-match",
						},
					},
					ClusterSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"stage": "production",
						},
					},
				},
			},
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"os": "linux",
					},
				},
			},
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"stage": "production",
					},
				},
			},
			matches: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			mdr := &clusterv1.MachineDrainRule{
				Spec: clusterv1.MachineDrainRuleSpec{
					Machines: tt.machineSelectors,
				},
			}
			g.Expect(machineDrainRuleAppliesToMachine(mdr, tt.machine, tt.cluster)).To(Equal(tt.matches))
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
						DrainBehavior: clusterv1.MachineDrainRuleDrainBehaviorSkip, // Will be skipped because DrainBehavior is set to Skip
						Reason:        PodDeleteStatusTypeWarning,
						Message:       daemonSetWarning,
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
						DrainBehavior: clusterv1.MachineDrainRuleDrainBehaviorDrain,
						DrainOrder:    ptr.To[int32](5), // min DrainOrder is 5 => will be evicted now
						Reason:        PodDeleteStatusTypeOkay,
					},
				},
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-3-to-trigger-eviction-successfully",
						},
					},
					Status: PodDeleteStatus{
						DrainBehavior: clusterv1.MachineDrainRuleDrainBehaviorDrain,
						DrainOrder:    ptr.To[int32](5), // min DrainOrder is 5 => will be evicted now
						Reason:        PodDeleteStatusTypeOkay,
					},
				},
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-4-to-trigger-eviction-pod-not-found",
						},
					},
					Status: PodDeleteStatus{
						DrainBehavior: clusterv1.MachineDrainRuleDrainBehaviorDrain,
						DrainOrder:    ptr.To[int32](5), // min DrainOrder is 5 => will be evicted now
						Reason:        PodDeleteStatusTypeOkay,
					},
				},
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-5-to-trigger-eviction-pdb-violated-2",
						},
					},
					Status: PodDeleteStatus{
						DrainBehavior: clusterv1.MachineDrainRuleDrainBehaviorDrain,
						DrainOrder:    ptr.To[int32](5), // min DrainOrder is 5 => will be evicted now
						Reason:        PodDeleteStatusTypeOkay,
					},
				},
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-5-to-trigger-eviction-pdb-violated-1",
						},
					},
					Status: PodDeleteStatus{
						DrainBehavior: clusterv1.MachineDrainRuleDrainBehaviorDrain,
						DrainOrder:    ptr.To[int32](5), // min DrainOrder is 5 => will be evicted now
						Reason:        PodDeleteStatusTypeOkay,
					},
				},
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-6-to-trigger-eviction-namespace-terminating",
						},
					},
					Status: PodDeleteStatus{
						DrainBehavior: clusterv1.MachineDrainRuleDrainBehaviorDrain,
						DrainOrder:    ptr.To[int32](5), // min DrainOrder is 5 => will be evicted now
						Reason:        PodDeleteStatusTypeOkay,
					},
				},
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-7-to-trigger-eviction-some-other-error",
						},
					},
					Status: PodDeleteStatus{
						DrainBehavior: clusterv1.MachineDrainRuleDrainBehaviorDrain,
						DrainOrder:    ptr.To[int32](5), // min DrainOrder is 5 => will be evicted now
						Reason:        PodDeleteStatusTypeOkay,
					},
				},
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-8-to-trigger-eviction-later",
						},
					},
					Status: PodDeleteStatus{
						DrainBehavior: clusterv1.MachineDrainRuleDrainBehaviorDrain,
						DrainOrder:    ptr.To[int32](6), // min DrainOrder is 5 => will be evicted later
						Reason:        PodDeleteStatusTypeOkay,
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
				PodsToTriggerEvictionLater: []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-8-to-trigger-eviction-later",
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
				RemoteClient: fakeClient,
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
				PodsToTriggerEvictionLater: []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-7-eviction-later",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-8-eviction-later",
						},
					},
				},
			},
			wantConditionMessage: `Drain not completed yet (started at 2024-10-09T16:13:59Z):
* Pods pod-2-deletionTimestamp-set-1, pod-3-to-trigger-eviction-successfully-1: deletionTimestamp set, but still not removed from the Node
* Pod pod-5-to-trigger-eviction-pdb-violated-1: cannot evict pod as it would violate the pod's disruption budget. The disruption budget pod-5-pdb needs 20 healthy pods and has 20 currently
* Pod pod-6-to-trigger-eviction-some-other-error: failed to evict Pod, some other error 1
After above Pods have been removed from the Node, the following Pods will be evicted: pod-7-eviction-later, pod-8-eviction-later`,
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
							Name: "pod-3-to-trigger-eviction-successfully-1", // only first 3.
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-3-to-trigger-eviction-successfully-2", // only first 3.
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-3-to-trigger-eviction-successfully-3-should-not-be-included", // only first 3.
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-3-to-trigger-eviction-successfully-4-should-not-be-included", // only first 3.
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
				PodsToTriggerEvictionLater: []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-11-eviction-later",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-12-eviction-later",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-13-eviction-later",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-14-eviction-later", // only first 3
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-15-eviction-later", // only first 3
						},
					},
				},
			},
			wantConditionMessage: `Drain not completed yet (started at 2024-10-09T16:13:59Z):
* Pods pod-2-deletionTimestamp-set-1, pod-2-deletionTimestamp-set-2, pod-2-deletionTimestamp-set-3, ... (4 more): deletionTimestamp set, but still not removed from the Node
* Pods pod-5-to-trigger-eviction-pdb-violated-1, pod-5-to-trigger-eviction-pdb-violated-2, pod-5-to-trigger-eviction-pdb-violated-3, ... (3 more): cannot evict pod as it would violate the pod's disruption budget. The disruption budget pod-5-pdb needs 20 healthy pods and has 20 currently
* Pod pod-6-to-trigger-eviction-some-other-error: failed to evict Pod, some other error 1
* Pod pod-7-to-trigger-eviction-some-other-error: failed to evict Pod, some other error 2
* Pod pod-8-to-trigger-eviction-some-other-error: failed to evict Pod, some other error 3
* Pod pod-9-to-trigger-eviction-some-other-error: failed to evict Pod, some other error 4
* 1 Pod with other issues
After above Pods have been removed from the Node, the following Pods will be evicted: pod-11-eviction-later, pod-12-eviction-later, pod-13-eviction-later, ... (2 more)`,
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
* Pod pod-1-to-trigger-eviction-some-other-error: failed to evict Pod, some other error 1
* Pod pod-2-to-trigger-eviction-some-other-error: failed to evict Pod, some other error 2
* Pod pod-3-to-trigger-eviction-some-other-error: failed to evict Pod, some other error 3
* Pod pod-4-to-trigger-eviction-some-other-error: failed to evict Pod, some other error 4
* Pod pod-5-to-trigger-eviction-some-other-error: failed to evict Pod, some other error 5
* 4 Pods with other issues`,
		},
	}

	nodeDrainStartTime, err := time.Parse(time.RFC3339, "2024-10-09T16:13:59Z")
	g.Expect(err).ToNot(HaveOccurred())

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			g.Expect(tt.evictionResult.ConditionMessage(&metav1.Time{Time: nodeDrainStartTime})).To(BeComparableTo(tt.wantConditionMessage))
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
