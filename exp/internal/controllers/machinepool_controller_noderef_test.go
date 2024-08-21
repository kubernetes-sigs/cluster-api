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
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
)

func TestMachinePoolGetNodeReference(t *testing.T) {
	r := &MachinePoolReconciler{
		Client:   fake.NewClientBuilder().Build(),
		recorder: record.NewFakeRecorder(32),
	}
	nodeRefsMap := map[string]*corev1.Node{
		"aws://us-east-1/id-node-1": {
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-1",
			},
			Spec: corev1.NodeSpec{
				ProviderID: "aws://us-east-1/id-node-1",
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		},
		"aws://us-west-2/id-node-2": {
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-2",
			},
			Spec: corev1.NodeSpec{
				ProviderID: "aws://us-west-2/id-node-2",
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		},
		"aws://us-west-2/id-node-3": {
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-3",
			},
			Spec: corev1.NodeSpec{
				ProviderID: "aws://us-west-2/id-node-3",
			},
		},
		"gce://us-central1/gce-id-node-2": {
			ObjectMeta: metav1.ObjectMeta{
				Name: "gce-node-2",
			},
			Spec: corev1.NodeSpec{
				ProviderID: "gce://us-central1/gce-id-node-2",
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		},
		"azure://westus2/id-node-4": {
			ObjectMeta: metav1.ObjectMeta{
				Name: "azure-node-4",
			},
			Spec: corev1.NodeSpec{
				ProviderID: "azure://westus2/id-node-4",
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		},
		"azure://westus2/id-nodepool1/0": {
			ObjectMeta: metav1.ObjectMeta{
				Name: "azure-nodepool1-0",
			},
			Spec: corev1.NodeSpec{
				ProviderID: "azure://westus2/id-nodepool1/0",
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		},
		"azure://westus2/id-nodepool2/0": {
			ObjectMeta: metav1.ObjectMeta{
				Name: "azure-nodepool2-0",
			},
			Spec: corev1.NodeSpec{
				ProviderID: "azure://westus2/id-nodepool2/0",
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		},
	}

	testCases := []struct {
		name            string
		providerIDList  []string
		expected        *getNodeReferencesResult
		err             error
		minReadySeconds int32
	}{
		{
			name:           "valid provider id, valid aws node",
			providerIDList: []string{"aws://us-east-1/id-node-1"},
			expected: &getNodeReferencesResult{
				references: []corev1.ObjectReference{
					{Name: "node-1"},
				},
				available: 1,
				ready:     1,
			},
		},
		{
			name:           "valid provider id, valid aws node",
			providerIDList: []string{"aws://us-west-2/id-node-2"},
			expected: &getNodeReferencesResult{
				references: []corev1.ObjectReference{
					{Name: "node-2"},
				},
				available: 1,
				ready:     1,
			},
		},
		{
			name:           "valid provider id, valid aws node, nodeReady condition set to false",
			providerIDList: []string{"aws://us-west-2/id-node-3"},
			expected: &getNodeReferencesResult{
				references: []corev1.ObjectReference{
					{Name: "node-3"},
				},
				available: 0,
				ready:     0,
			},
		},
		{
			name:           "valid provider id, valid gce node",
			providerIDList: []string{"gce://us-central1/gce-id-node-2"},
			expected: &getNodeReferencesResult{
				references: []corev1.ObjectReference{
					{Name: "gce-node-2"},
				},
				available: 1,
				ready:     1,
			},
		},
		{
			name:           "valid provider id, valid azure node",
			providerIDList: []string{"azure://westus2/id-node-4"},
			expected: &getNodeReferencesResult{
				references: []corev1.ObjectReference{
					{Name: "azure-node-4"},
				},
				available: 1,
				ready:     1,
			},
		},
		{
			name:           "valid provider ids, valid azure and aws nodes",
			providerIDList: []string{"aws://us-east-1/id-node-1", "azure://westus2/id-node-4"},
			expected: &getNodeReferencesResult{
				references: []corev1.ObjectReference{
					{Name: "node-1"},
					{Name: "azure-node-4"},
				},
				available: 2,
				ready:     2,
			},
		},
		{
			name:           "valid provider id, no node found",
			providerIDList: []string{"aws:///id-node-100"},
			expected:       nil,
			err:            errNoAvailableNodes,
		},
		{
			name:           "no provider id, no node found",
			providerIDList: []string{},
			expected: &getNodeReferencesResult{
				references: []corev1.ObjectReference{},
				available:  0,
				ready:      0,
			},
		},
		{
			name:           "valid provider id with non-unique instance id, valid azure node",
			providerIDList: []string{"azure://westus2/id-nodepool1/0"},
			expected: &getNodeReferencesResult{
				references: []corev1.ObjectReference{
					{Name: "azure-nodepool1-0"},
				},
				available: 1,
				ready:     1,
			},
		},
		{
			name:           "valid provider ids with same instance ids, valid azure nodes",
			providerIDList: []string{"azure://westus2/id-nodepool1/0", "azure://westus2/id-nodepool2/0"},
			expected: &getNodeReferencesResult{
				references: []corev1.ObjectReference{
					{Name: "azure-nodepool1-0"},
					{Name: "azure-nodepool2-0"},
				},
				available: 2,
				ready:     2,
			},
		},
		{
			name:           "valid provider id, valid aws node, with minReadySeconds",
			providerIDList: []string{"aws://us-east-1/id-node-1"},
			expected: &getNodeReferencesResult{
				references: []corev1.ObjectReference{{Name: "node-1"}},
				available:  0,
				ready:      1,
			},
			minReadySeconds: 20,
		},
		{
			name:           "valid provider id, valid aws node, with minReadySeconds equals 0",
			providerIDList: []string{"aws://us-east-1/id-node-1"},
			expected: &getNodeReferencesResult{
				references: []corev1.ObjectReference{{Name: "node-1"}},
				available:  1,
				ready:      1,
			},
			minReadySeconds: 0,
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			g := NewWithT(t)

			result, err := r.getNodeReferences(ctx, test.providerIDList, ptr.To(test.minReadySeconds), nodeRefsMap)
			if test.err == nil {
				g.Expect(err).ToNot(HaveOccurred())
			} else {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err).To(Equal(test.err), "Expected error %v, got %v", test.err, err)
			}

			if test.expected == nil && len(result.references) == 0 {
				return
			}

			g.Expect(result.references).To(HaveLen(len(test.expected.references)), "Expected NodeRef count to be %v, got %v", len(result.references), len(test.expected.references))

			g.Expect(result.available).To(Equal(test.expected.available), "Expected available node count to be %v, got %v", test.expected.available, result.available)
			g.Expect(result.ready).To(Equal(test.expected.ready), "Expected ready node count to be %v, got %v", test.expected.ready, result.ready)

			for n := range test.expected.references {
				g.Expect(result.references[n].Name).To(Equal(test.expected.references[n].Name), "Expected NodeRef's name to be %v, got %v", result.references[n].Name, test.expected.references[n].Name)
				g.Expect(result.references[n].Namespace).To(Equal(test.expected.references[n].Namespace), "Expected NodeRef's namespace to be %v, got %v", result.references[n].Namespace, test.expected.references[n].Namespace)
			}
		})
	}
}

func TestMachinePoolPatchNodes(t *testing.T) {
	r := &MachinePoolReconciler{
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
				Taints: []corev1.Taint{
					clusterv1.NodeUninitializedTaint,
				},
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-2",
				Annotations: map[string]string{
					"foo": "bar",
				},
			},
			Spec: corev1.NodeSpec{
				ProviderID: "aws://us-west-2/id-node-2",
				Taints: []corev1.Taint{
					{
						Key:   "some-other-taint",
						Value: "SomeEffect",
					},
					clusterv1.NodeUninitializedTaint,
				},
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-3",
				Annotations: map[string]string{
					"cluster.x-k8s.io/cluster-name":      "cluster-1",
					"cluster.x-k8s.io/cluster-namespace": "my-namespace",
					"cluster.x-k8s.io/owner-kind":        "MachinePool",
					"cluster.x-k8s.io/owner-name":        "machinepool-3",
				},
			},
			Spec: corev1.NodeSpec{
				ProviderID: "aws://us-west-2/id-node-3",
				Taints: []corev1.Taint{
					{
						Key:   "some-other-taint",
						Value: "SomeEffect",
					},
					clusterv1.NodeUninitializedTaint,
				},
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-4",
				Annotations: map[string]string{
					"cluster.x-k8s.io/cluster-name":      "cluster-1",
					"cluster.x-k8s.io/cluster-namespace": "my-namespace",
				},
			},
			Spec: corev1.NodeSpec{
				ProviderID: "aws://us-west-2/id-node-4",
				Taints: []corev1.Taint{
					{
						Key:   "some-other-taint",
						Value: "SomeEffect",
					},
				},
			},
		},
	}

	testCases := []struct {
		name          string
		machinePool   *expv1.MachinePool
		nodeRefs      []corev1.ObjectReference
		expectedNodes []corev1.Node
		err           error
	}{
		{
			name: "Node with uninitialized taint should be patched",
			machinePool: &expv1.MachinePool{
				TypeMeta: metav1.TypeMeta{
					Kind: "MachinePool",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machinepool-1",
					Namespace: "my-namespace",
				},
				Spec: expv1.MachinePoolSpec{
					ClusterName:    "cluster-1",
					ProviderIDList: []string{"aws://us-east-1/id-node-1"},
				},
			},
			nodeRefs: []corev1.ObjectReference{
				{Name: "node-1"},
			},
			expectedNodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-1",
						Annotations: map[string]string{
							"cluster.x-k8s.io/cluster-name":      "cluster-1",
							"cluster.x-k8s.io/cluster-namespace": "my-namespace",
							"cluster.x-k8s.io/owner-kind":        "MachinePool",
							"cluster.x-k8s.io/owner-name":        "machinepool-1",
						},
					},
					Spec: corev1.NodeSpec{
						Taints: nil,
					},
				},
			},
		},
		{
			name: "Node with existing annotations and taints should be patched",
			machinePool: &expv1.MachinePool{
				TypeMeta: metav1.TypeMeta{
					Kind: "MachinePool",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machinepool-2",
					Namespace: "my-namespace",
				},
				Spec: expv1.MachinePoolSpec{
					ClusterName:    "cluster-1",
					ProviderIDList: []string{"aws://us-west-2/id-node-2"},
				},
			},
			nodeRefs: []corev1.ObjectReference{
				{Name: "node-2"},
				{Name: "node-3"},
				{Name: "node-4"},
			},
			expectedNodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-2",
						Annotations: map[string]string{
							"cluster.x-k8s.io/cluster-name":      "cluster-1",
							"cluster.x-k8s.io/cluster-namespace": "my-namespace",
							"cluster.x-k8s.io/owner-kind":        "MachinePool",
							"cluster.x-k8s.io/owner-name":        "machinepool-2",
							"foo":                                "bar",
						},
					},
					Spec: corev1.NodeSpec{
						Taints: []corev1.Taint{
							{
								Key:   "some-other-taint",
								Value: "SomeEffect",
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-3",
						Annotations: map[string]string{
							"cluster.x-k8s.io/cluster-name":      "cluster-1",
							"cluster.x-k8s.io/cluster-namespace": "my-namespace",
							"cluster.x-k8s.io/owner-kind":        "MachinePool",
							"cluster.x-k8s.io/owner-name":        "machinepool-2",
						},
					},
					Spec: corev1.NodeSpec{
						Taints: []corev1.Taint{
							{
								Key:   "some-other-taint",
								Value: "SomeEffect",
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-4",
						Annotations: map[string]string{
							"cluster.x-k8s.io/cluster-name":      "cluster-1",
							"cluster.x-k8s.io/cluster-namespace": "my-namespace",
							"cluster.x-k8s.io/owner-kind":        "MachinePool",
							"cluster.x-k8s.io/owner-name":        "machinepool-2",
						},
					},
					Spec: corev1.NodeSpec{
						Taints: []corev1.Taint{
							{
								Key:   "some-other-taint",
								Value: "SomeEffect",
							},
						},
					},
				},
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			g := NewWithT(t)

			fakeClient := fake.NewClientBuilder().WithObjects(nodeList...).Build()

			err := r.patchNodes(ctx, fakeClient, test.nodeRefs, test.machinePool)
			if test.err == nil {
				g.Expect(err).ToNot(HaveOccurred())
			} else {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err).To(Equal(test.err), "Expected error %v, got %v", test.err, err)
			}

			// Check that the nodes have the desired taints and annotations
			for _, expected := range test.expectedNodes {
				node := &corev1.Node{}
				err := fakeClient.Get(ctx, client.ObjectKey{Name: expected.Name}, node)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(node.Annotations).To(Equal(expected.Annotations))
				g.Expect(node.Spec.Taints).To(BeComparableTo(expected.Spec.Taints))
			}
		})
	}
}
