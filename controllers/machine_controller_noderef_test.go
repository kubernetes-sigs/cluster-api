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

package controllers

import (
	"testing"

	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/cluster-api/controllers/noderefutil"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetNodeReference(t *testing.T) {
	g := NewWithT(t)

	r := &MachineReconciler{
		Client:   fake.NewClientBuilder().Build(),
		recorder: record.NewFakeRecorder(32),
	}

	nodeList := []client.Object{
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-1",
			},
			Spec: corev1.NodeSpec{
				ProviderID: "aws://us-east-1/id-node-1",
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-2",
			},
			Spec: corev1.NodeSpec{
				ProviderID: "aws://us-west-2/id-node-2",
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "gce-node-2",
			},
			Spec: corev1.NodeSpec{
				ProviderID: "gce://us-central1/id-node-2",
			},
		},
	}

	client := fake.NewClientBuilder().WithObjects(nodeList...).Build()

	testCases := []struct {
		name       string
		providerID string
		expected   *corev1.ObjectReference
		err        error
	}{
		{
			name:       "valid provider id, valid aws node",
			providerID: "aws:///id-node-1",
			expected:   &corev1.ObjectReference{Name: "node-1"},
		},
		{
			name:       "valid provider id, valid aws node",
			providerID: "aws:///id-node-2",
			expected:   &corev1.ObjectReference{Name: "node-2"},
		},
		{
			name:       "valid provider id, valid gce node",
			providerID: "gce:///id-node-2",
			expected:   &corev1.ObjectReference{Name: "gce-node-2"},
		},
		{
			name:       "valid provider id, no node found",
			providerID: "aws:///id-node-100",
			expected:   nil,
			err:        ErrNodeNotFound,
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			gt := NewWithT(t)
			providerID, err := noderefutil.NewProviderID(test.providerID)
			gt.Expect(err).NotTo(HaveOccurred(), "Expected no error parsing provider id %q, got %v", test.providerID, err)

			node, err := r.getNode(ctx, client, providerID)
			if test.err == nil {
				g.Expect(err).To(BeNil())
			} else {
				gt.Expect(err).NotTo(BeNil())
				gt.Expect(err).To(Equal(test.err), "Expected error %v, got %v", test.err, err)
			}

			if test.expected == nil && node == nil {
				return
			}

			gt.Expect(node.Name).To(Equal(test.expected.Name), "Expected NodeRef's name to be %v, got %v", node.Name, test.expected.Name)
			gt.Expect(node.Namespace).To(Equal(test.expected.Namespace), "Expected NodeRef's namespace to be %v, got %v", node.Namespace, test.expected.Namespace)
		})
	}
}

func TestSummarizeNodeConditions(t *testing.T) {
	testCases := []struct {
		name       string
		conditions []corev1.NodeCondition
		status     corev1.ConditionStatus
	}{
		{
			name: "node is healthy",
			conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionFalse},
				{Type: corev1.NodeDiskPressure, Status: corev1.ConditionFalse},
				{Type: corev1.NodePIDPressure, Status: corev1.ConditionFalse},
			},
			status: corev1.ConditionTrue,
		},
		{
			name: "all conditions are unknown",
			conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionUnknown},
				{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionUnknown},
				{Type: corev1.NodeDiskPressure, Status: corev1.ConditionUnknown},
				{Type: corev1.NodePIDPressure, Status: corev1.ConditionUnknown},
			},
			status: corev1.ConditionUnknown,
		},
		{
			name: "multiple semantically failed condition",
			conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionUnknown},
				{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionTrue},
				{Type: corev1.NodeDiskPressure, Status: corev1.ConditionTrue},
				{Type: corev1.NodePIDPressure, Status: corev1.ConditionTrue},
			},
			status: corev1.ConditionFalse,
		},
		{
			name: "one positive condition when the rest is unknown",
			conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionUnknown},
				{Type: corev1.NodeDiskPressure, Status: corev1.ConditionUnknown},
				{Type: corev1.NodePIDPressure, Status: corev1.ConditionUnknown},
			},
			status: corev1.ConditionTrue,
		},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			g := NewWithT(t)
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-1",
				},
				Status: corev1.NodeStatus{
					Conditions: test.conditions,
				},
			}
			status, _ := summarizeNodeConditions(node)
			g.Expect(status).To(Equal(test.status))
		})
	}
}
