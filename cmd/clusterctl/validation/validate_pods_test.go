/*
Copyright 2018 The Kubernetes Authors.

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

	"golang.org/x/net/context"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1/testutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func getPodWithContainerSpec(podName, namespace, containerName, containerImage string) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  containerName,
					Image: containerImage,
				},
			},
		},
	}
}

func getPodWithStatus(podName, namespace string, podPhase corev1.PodPhase, containerReadyStatus bool) corev1.Pod {
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

func TestGetPods(t *testing.T) {
	// Setup the Manager and Controller.
	mgr, err := manager.New(cfg, manager.Options{})
	if err != nil {
		t.Fatalf("error creating new manager: %v", err)
	}
	c = mgr.GetClient()
	defer close(StartTestManager(mgr, t))

	const testClusterName = "test-cluster"
	const testNamespace = "get-pods"
	cluster := testutil.GetVanillaCluster()
	cluster.Name = testClusterName
	cluster.Namespace = testNamespace
	if err := c.Create(context.TODO(), &cluster); err != nil {
		t.Fatalf("error creating cluster: %v", err)
	}
	defer c.Delete(context.TODO(), &cluster)

	pod := getPodWithContainerSpec("test-pod", testNamespace, "test-container", "test-image")
	if err := c.Create(context.TODO(), &pod); err != nil {
		t.Fatalf("Error creating pod: %v", err)
	}

	if pods, _ := getPods(c, testNamespace); len(pods.Items) != 1 {
		t.Fatalf("Expect to get one pod but get none")
	}
}

func TestValidatePodsWithNoPod(t *testing.T) {
	pods := &corev1.PodList{
		Items: []corev1.Pod{},
	}

	var b bytes.Buffer
	if err := validatePods(&b, pods); err == nil {
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
					getPodWithStatus("test-pod", "test-namespace", testcase.podPhase, testcase.containerReadyStatus),
				},
			}

			var b bytes.Buffer
			err := validatePods(&b, pods)
			if testcase.expectErr && err == nil {
				t.Errorf("Expect to get error, but got no returned error: %v", b.String())
			}
			if !testcase.expectErr && err != nil {
				t.Errorf("Expect to get no error, but got returned error: %v: %v", err, b.String())
			}
		})
	}
}
