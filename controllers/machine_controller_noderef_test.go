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
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/cluster-api/controllers/noderefutil"
	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/cluster-api/util"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestGetNode(t *testing.T) {
	g := NewWithT(t)

	ns, err := env.CreateNamespace(ctx, "test-get-node")
	g.Expect(err).ToNot(HaveOccurred())

	// Set up cluster to test against.
	testCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "test-get-node-",
			Namespace:    ns.Name,
		},
	}

	g.Expect(env.Create(ctx, testCluster)).To(BeNil())
	g.Expect(env.CreateKubeconfigSecret(ctx, testCluster)).To(Succeed())
	defer func(do ...client.Object) {
		g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
	}(ns, testCluster)

	testCases := []struct {
		name            string
		node            *corev1.Node
		providerIDInput string
		error           error
	}{
		{
			name: "full providerID matches",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-get-node-node-1",
				},
				Spec: corev1.NodeSpec{
					ProviderID: "aws://us-east-1/test-get-node-1",
				},
			},
			providerIDInput: "aws://us-east-1/test-get-node-1",
		},
		{
			name: "aws prefix: cloudProvider and ID matches",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-get-node-node-2",
				},
				Spec: corev1.NodeSpec{
					ProviderID: "aws://us-west-2/test-get-node-2",
				},
			},
			providerIDInput: "aws:///test-get-node-2",
		},
		{
			name: "gce prefix, cloudProvider and ID matches",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-get-node-gce-node-2",
				},
				Spec: corev1.NodeSpec{
					ProviderID: "gce://us-central1/test-get-node-2",
				},
			},
			providerIDInput: "gce:///test-get-node-2",
		},
		{
			name: "Node is not found",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-get-node-not-found",
				},
				Spec: corev1.NodeSpec{
					ProviderID: "gce://us-central1/anything",
				},
			},
			providerIDInput: "gce://not-found",
			error:           ErrNodeNotFound,
		},
	}

	nodesToCleanup := make([]client.Object, 0, len(testCases))
	for _, tc := range testCases {
		g.Expect(env.Create(ctx, tc.node)).To(BeNil())
		nodesToCleanup = append(nodesToCleanup, tc.node)
	}
	defer func(do ...client.Object) {
		g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
	}(nodesToCleanup...)

	tracker, err := remote.NewClusterCacheTracker(
		env.Manager, remote.ClusterCacheTrackerOptions{
			Indexes: remote.DefaultIndexes,
		},
	)
	g.Expect(err).ToNot(HaveOccurred())

	r := &MachineReconciler{
		Tracker: tracker,
		Client:  env,
	}

	w, err := ctrl.NewControllerManagedBy(env.Manager).For(&corev1.Node{}).Build(r)
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(tracker.Watch(ctx, remote.WatchInput{
		Name:    "TestGetNode",
		Cluster: util.ObjectKey(testCluster),
		Watcher: w,
		Kind:    &corev1.Node{},
		EventHandler: handler.EnqueueRequestsFromMapFunc(func(client.Object) []reconcile.Request {
			return nil
		}),
	})).To(Succeed())

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			remoteClient, err := r.Tracker.GetClient(ctx, util.ObjectKey(testCluster))
			g.Expect(err).ToNot(HaveOccurred())

			providerID, err := noderefutil.NewProviderID(tc.providerIDInput)
			g.Expect(err).ToNot(HaveOccurred())

			node, err := r.getNode(ctx, remoteClient, providerID)
			if tc.error != nil {
				g.Expect(err).To(Equal(tc.error))
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(node.Name).To(Equal(tc.node.Name))
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
