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
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

func TestSkipDeletedFilter(t *testing.T) {
	tCases := []struct {
		timeStampAgeSeconds             int
		skipWaitForDeleteTimeoutSeconds int
		expectedDrainBehavior           clusterv1.MachineDrainRuleDrainBehavior
	}{
		{
			timeStampAgeSeconds:             0,
			skipWaitForDeleteTimeoutSeconds: 20,
			expectedDrainBehavior:           clusterv1.MachineDrainRuleDrainBehaviorDrain,
		},
		{
			timeStampAgeSeconds:             1,
			skipWaitForDeleteTimeoutSeconds: 20,
			expectedDrainBehavior:           clusterv1.MachineDrainRuleDrainBehaviorDrain,
		},
		{
			timeStampAgeSeconds:             100,
			skipWaitForDeleteTimeoutSeconds: 20,
			expectedDrainBehavior:           clusterv1.MachineDrainRuleDrainBehaviorSkip,
		},
	}
	for i, tc := range tCases {
		h := &Helper{
			SkipWaitForDeleteTimeoutSeconds: tc.skipWaitForDeleteTimeoutSeconds,
		}
		pod := corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod",
				Namespace: "default",
			},
		}

		if tc.timeStampAgeSeconds > 0 {
			dTime := &metav1.Time{Time: time.Now().Add(time.Duration(tc.timeStampAgeSeconds) * time.Second * -1)}
			pod.ObjectMeta.SetDeletionTimestamp(dTime)
		}

		podDeleteStatus := h.skipDeletedFilter(context.Background(), &pod)
		if podDeleteStatus.DrainBehavior != tc.expectedDrainBehavior {
			t.Errorf("test %v: unexpected podDeleteStatus.DrainBehavior; actual %v; expected %v", i, podDeleteStatus.DrainBehavior, tc.expectedDrainBehavior)
		}
	}
}

func Test_machineDrainRuleAppliesToPod(t *testing.T) {
	tests := []struct {
		name         string
		podSelectors []clusterv1.MachineDrainRulePodSelector
		pod          *corev1.Pod
		namespace    *corev1.Namespace
		matches      bool
	}{
		{
			name:         "nil always matches",
			podSelectors: nil,
			matches:      true,
		},
		{
			name:         "empty always matches",
			podSelectors: []clusterv1.MachineDrainRulePodSelector{},
			matches:      true,
		},
		{
			name: "matches if one entire PodSelector matches",
			podSelectors: []clusterv1.MachineDrainRulePodSelector{
				{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "does-not-match",
						},
					},
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"kubernetes.io/metadata.name": "does-not-match",
						},
					},
				},
				{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "prometheus",
						},
					},
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"kubernetes.io/metadata.name": "monitoring",
						},
					},
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "prometheus",
					},
				},
			},
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"kubernetes.io/metadata.name": "monitoring",
					},
				},
			},
			matches: true,
		},
		{
			name: "does not match if only PodSelector.selector matches",
			podSelectors: []clusterv1.MachineDrainRulePodSelector{
				{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "prometheus",
						},
					},
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"kubernetes.io/metadata.name": "does-not-match",
						},
					},
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "prometheus",
					},
				},
			},
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"kubernetes.io/metadata.name": "monitoring",
					},
				},
			},
			matches: false,
		},
		{
			name: "does not match if only PodSelector.namespaceSelector matches",
			podSelectors: []clusterv1.MachineDrainRulePodSelector{
				{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "does-not-match",
						},
					},
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"kubernetes.io/metadata.name": "monitoring",
						},
					},
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "prometheus",
					},
				},
			},
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"kubernetes.io/metadata.name": "monitoring",
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
					Pods: tt.podSelectors,
				},
			}
			g.Expect(machineDrainRuleAppliesToPod(mdr, tt.pod, tt.namespace)).To(Equal(tt.matches))
		})
	}
}

func Test_matchesSelector(t *testing.T) {
	tests := []struct {
		name          string
		labelSelector *metav1.LabelSelector
		obj           client.Object
		matches       bool
	}{
		{
			name:          "nil always matches",
			labelSelector: nil,
			matches:       true,
		},
		{
			name:          "empty selector always matches",
			labelSelector: &metav1.LabelSelector{},
			matches:       true,
		},
		{
			name: "selector matches",
			labelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "prometheus",
				},
			},
			obj: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "prometheus",
					},
				},
			},
			matches: true,
		},
		{
			name: "selector doesn't match",
			labelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "logging",
				},
			},
			obj: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "prometheus",
					},
				},
			},
			matches: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(matchesSelector(tt.labelSelector, tt.obj)).To(Equal(tt.matches))
		})
	}
}
