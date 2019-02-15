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

package validation

import (
	"bytes"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func podWithStatus(podName, namespace string, podPhase corev1.PodPhase, containerReadyStatus bool) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{},
		Status: corev1.PodStatus{
			Phase: podPhase,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Ready: containerReadyStatus,
				},
			},
		},
	}
}

func TestValidatePodsWithNoPod(t *testing.T) {
	pods := &corev1.PodList{Items: []corev1.Pod{}}

	var b bytes.Buffer
	if err := validatePods(&b, pods, "test-namespace"); err == nil {
		t.Errorf("Expected error but didn't get one")
	}
}

func TestValidatePodsWithOnePod(t *testing.T) {
	var testcases = []struct {
		name                 string
		podPhase             corev1.PodPhase
		containerReadyStatus bool
		expectErr            bool
	}{
		{
			name:                 "Pods include terminating pod",
			podPhase:             corev1.PodSucceeded,
			containerReadyStatus: false,
			expectErr:            false,
		},
		{
			name:                 "Pods include pending pod",
			podPhase:             corev1.PodPending,
			containerReadyStatus: false,
			expectErr:            true,
		},
		{
			name:                 "Pods include failed pod",
			podPhase:             corev1.PodFailed,
			containerReadyStatus: false,
			expectErr:            true,
		},
		{
			name:                 "Pods include unknown pod",
			podPhase:             corev1.PodUnknown,
			containerReadyStatus: false,
			expectErr:            true,
		},
		{
			name:                 "Pods include pod with non-ready container",
			podPhase:             corev1.PodRunning,
			containerReadyStatus: false,
			expectErr:            true,
		},
		{
			name:                 "Pods are all ready",
			podPhase:             corev1.PodRunning,
			containerReadyStatus: true,
			expectErr:            false,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			pods := &corev1.PodList{
				Items: []corev1.Pod{
					podWithStatus("test-pod", "test-namespace", testcase.podPhase, testcase.containerReadyStatus),
				},
			}

			var b bytes.Buffer
			err := validatePods(&b, pods, "test-namespace")
			if testcase.expectErr && err == nil {
				t.Errorf("Expect to get error, but got no returned error: %v", b.String())
			}
			if !testcase.expectErr && err != nil {
				t.Errorf("Expect to get no error, but got returned error: %v: %v", err, b.String())
			}
		})
	}
}

func TestValidatePodsWithNPods(t *testing.T) {
	var testcases = []struct {
		name string
		pods []corev1.Pod

		expectErr bool
	}{
		{
			name: "Pods start with failed pod",
			pods: []corev1.Pod{
				podWithStatus("test-pod-1", "test-namespace", corev1.PodFailed, false),
				podWithStatus("test-pod-2", "test-namespace", corev1.PodRunning, true),
			},
			expectErr: true,
		},
		{
			name: "Pods end with failed pod",
			pods: []corev1.Pod{
				podWithStatus("test-pod-1", "test-namespace", corev1.PodRunning, true),
				podWithStatus("test-pod-2", "test-namespace", corev1.PodFailed, false),
			},
			expectErr: true,
		},
		{
			name: "Pods include pod with non-ready container",
			pods: []corev1.Pod{
				podWithStatus("test-pod-1", "test-namespace", corev1.PodRunning, false),
				podWithStatus("test-pod-2", "test-namespace", corev1.PodRunning, true),
			},
			expectErr: true,
		},
		{
			name: "Pods are all failing",
			pods: []corev1.Pod{
				podWithStatus("test-pod-1", "test-namespace", corev1.PodFailed, false),
				podWithStatus("test-pod-2", "test-namespace", corev1.PodFailed, false),
			},
			expectErr: true,
		},
		{
			name: "Pods are all ready",
			pods: []corev1.Pod{
				podWithStatus("test-pod-1", "test-namespace", corev1.PodRunning, true),
				podWithStatus("test-pod-2", "test-namespace", corev1.PodRunning, true),
			},
			expectErr: false,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			pods := &corev1.PodList{
				Items: testcase.pods,
			}

			var b bytes.Buffer
			err := validatePods(&b, pods, "test-namespace")
			if testcase.expectErr && err == nil {
				t.Errorf("Expect to get error, but got no returned error: %v", b.String())
			}
			if !testcase.expectErr && err != nil {
				t.Errorf("Expect to get no error, but got returned error: %v: %v", err, b.String())
			}
		})
	}
}

func componentConditionWithStatus(status corev1.ConditionStatus) corev1.ComponentCondition {
	return corev1.ComponentCondition{
		Status: status,
	}
}

func componentStatusListWithCondition(componentConditions []corev1.ComponentCondition) *corev1.ComponentStatusList {
	return &corev1.ComponentStatusList{
		Items: []corev1.ComponentStatus{
			{
				Conditions: componentConditions,
			},
		},
	}
}

func TestValidateComponentsWithNoComponent(t *testing.T) {
	components := &corev1.ComponentStatusList{Items: []corev1.ComponentStatus{}}

	var b bytes.Buffer
	if err := validateComponents(&b, components); err == nil {
		t.Errorf("Expected error but didn't get one")
	}
}

func TestValidateComponents(t *testing.T) {
	var testcases = []struct {
		name            string
		conditionStatus corev1.ConditionStatus

		expectErr bool
	}{
		{
			name:            "Components include unknown status",
			conditionStatus: corev1.ConditionUnknown,
			expectErr:       true,
		},
		{
			name:            "Components include not ready status",
			conditionStatus: corev1.ConditionFalse,
			expectErr:       true,
		},
		{
			name:            "Components are all ready",
			conditionStatus: corev1.ConditionTrue,
			expectErr:       false,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			components := componentStatusListWithCondition(
				[]corev1.ComponentCondition{
					componentConditionWithStatus(testcase.conditionStatus),
				},
			)

			var b bytes.Buffer
			err := validateComponents(&b, components)
			if testcase.expectErr && err == nil {
				t.Errorf("Expect to get error, but got no returned error: %v", b.String())
			}
			if !testcase.expectErr && err != nil {
				t.Errorf("Expect to get no error, but got returned error: %v: %v", err, b.String())
			}
		})
	}
}
