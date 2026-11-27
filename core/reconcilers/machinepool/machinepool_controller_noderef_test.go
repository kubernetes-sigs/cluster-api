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

package machinepool

import (
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
)

func TestMachinePoolGetNodeReference(t *testing.T) {
	r := &Reconciler{
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

			result, err := r.getNodeReferences(ctx, test.providerIDList, test.minReadySeconds, nodeRefsMap)
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
	r := &Reconciler{
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
		name             string
		machinePool      *clusterv1.MachinePool
		infraMachinePool *unstructured.Unstructured
		nodeRefs         []corev1.ObjectReference
		expectedNodes    []corev1.Node
		err              error
	}{
		{
			name: "Node with uninitialized taint should be patched when MachinePool has no Machines",
			machinePool: &clusterv1.MachinePool{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machinepool-1",
					Namespace: "my-namespace",
				},
				Spec: clusterv1.MachinePoolSpec{
					ClusterName:    "cluster-1",
					ProviderIDList: []string{"aws://us-east-1/id-node-1"},
				},
			},
			infraMachinePool: &unstructured.Unstructured{
				Object: map[string]any{},
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
			name: "Node with uninitialized taint should not be patched when MachinePool has Machines",
			machinePool: &clusterv1.MachinePool{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machinepool-with-machines",
					Namespace: "my-namespace",
				},
				Spec: clusterv1.MachinePoolSpec{
					ClusterName:    "cluster-1",
					ProviderIDList: []string{"aws://us-east-1/id-node-1"},
				},
			},
			infraMachinePool: &unstructured.Unstructured{
				Object: map[string]any{
					"status": map[string]any{
						"infrastructureMachineKind": "DockerMachine",
					},
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
							"cluster.x-k8s.io/owner-name":        "machinepool-with-machines",
						},
					},
					Spec: corev1.NodeSpec{
						Taints: []corev1.Taint{
							clusterv1.NodeUninitializedTaint,
						},
					},
				},
			},
		},
		{
			name: "Node with existing annotations and taints should be patched",
			machinePool: &clusterv1.MachinePool{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machinepool-2",
					Namespace: "my-namespace",
				},
				Spec: clusterv1.MachinePoolSpec{
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

			s := &scope{
				machinePool:      test.machinePool,
				infraMachinePool: test.infraMachinePool,
			}

			err := r.patchNodes(ctx, fakeClient, test.nodeRefs, s)
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

func TestReconcileNodeRefsEmptyProviderIDList(t *testing.T) {
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: metav1.NamespaceDefault,
		},
	}

	t.Run("deletes retired nodes and clears nodeRefs when the provider reports no instances", func(t *testing.T) {
		g := NewWithT(t)

		retiredNodes := []*corev1.Node{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-1",
				},
				Spec: corev1.NodeSpec{
					ProviderID: "aws://us-east-1/id-node-1",
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-2",
				},
				Spec: corev1.NodeSpec{
					ProviderID: "aws://us-east-1/id-node-2",
				},
			},
		}
		unrelatedNode := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "unrelated-node",
			},
			Spec: corev1.NodeSpec{
				ProviderID: "aws://us-east-1/id-unrelated",
			},
		}

		workloadObjects := []client.Object{unrelatedNode}
		nodeRefMap := make(map[string]*corev1.Node, len(retiredNodes))
		nodeRefs := make([]corev1.ObjectReference, 0, len(retiredNodes))
		for _, node := range retiredNodes {
			workloadObjects = append(workloadObjects, node)
			nodeRefMap[node.Spec.ProviderID] = node
			nodeRefs = append(nodeRefs, corev1.ObjectReference{
				APIVersion: corev1.SchemeGroupVersion.String(),
				Kind:       "Node",
				Name:       node.Name,
			})
		}

		workloadClient := fake.NewClientBuilder().WithObjects(workloadObjects...).Build()
		r := &Reconciler{
			Client:       fake.NewClientBuilder().Build(),
			ClusterCache: clustercache.NewFakeClusterCache(workloadClient, client.ObjectKeyFromObject(cluster)),
			recorder:     record.NewFakeRecorder(32),
		}

		// Desired replicas stay above zero. The provider has reported that every
		// instance is gone: an empty ProviderIDList and status.replicas == 0.
		// Previously this returned before deleteRetiredNodes because spec.replicas != 0.
		mp := &clusterv1.MachinePool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machinepool-1",
				Namespace: metav1.NamespaceDefault,
			},
			Spec: clusterv1.MachinePoolSpec{
				ClusterName:    cluster.Name,
				Replicas:       ptr.To[int32](2),
				ProviderIDList: []string{},
			},
			Status: clusterv1.MachinePoolStatus{
				Initialization: clusterv1.MachinePoolInitializationStatus{
					InfrastructureProvisioned: ptr.To(true),
				},
				Replicas: ptr.To[int32](0),
				NodeRefs: nodeRefs,
				Deprecated: &clusterv1.MachinePoolDeprecatedStatus{
					V1Beta1: &clusterv1.MachinePoolV1Beta1DeprecatedStatus{
						ReadyReplicas: 2,
					},
				},
			},
		}

		res, err := r.reconcileNodeRefs(ctx, &scope{
			cluster:     cluster,
			machinePool: mp,
			nodeRefMap:  nodeRefMap,
		})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res).To(Equal(ctrl.Result{}))
		g.Expect(mp.Status.NodeRefs).To(BeEmpty())
		g.Expect(mp.Status.Deprecated.V1Beta1.ReadyReplicas).To(Equal(int32(0)))
		g.Expect(mp.Status.Deprecated.V1Beta1.AvailableReplicas).To(Equal(int32(0)))

		for _, node := range retiredNodes {
			err := workloadClient.Get(ctx, client.ObjectKey{Name: node.Name}, &corev1.Node{})
			g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "retired Node %s should be deleted", node.Name)
		}
		g.Expect(workloadClient.Get(ctx, client.ObjectKey{Name: unrelatedNode.Name}, &corev1.Node{})).To(Succeed())

		r.reconcilePhase(mp)
		g.Expect(mp.Status.GetTypedPhase()).To(Equal(clusterv1.MachinePoolPhaseScalingUp))
	})

	t.Run("is a no-op when providerIDList and nodeRefs are empty", func(t *testing.T) {
		g := NewWithT(t)

		// spec.replicas is still set, which is the case the previous guard treated as
		// "ProviderIDs are not ready yet". With no NodeRefs there is nothing to retire.
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "preexisting-node",
			},
			Spec: corev1.NodeSpec{
				ProviderID: "aws://us-east-1/id-preexisting",
			},
		}
		workloadClient := fake.NewClientBuilder().WithObjects(node).Build()
		r := &Reconciler{
			Client:       fake.NewClientBuilder().Build(),
			ClusterCache: clustercache.NewFakeClusterCache(workloadClient, client.ObjectKeyFromObject(cluster)),
			recorder:     record.NewFakeRecorder(32),
		}

		mp := &clusterv1.MachinePool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machinepool-2",
				Namespace: metav1.NamespaceDefault,
			},
			Spec: clusterv1.MachinePoolSpec{
				ClusterName:    cluster.Name,
				Replicas:       ptr.To[int32](1),
				ProviderIDList: []string{},
			},
			Status: clusterv1.MachinePoolStatus{
				NodeRefs: []corev1.ObjectReference{},
			},
		}

		res, err := r.reconcileNodeRefs(ctx, &scope{
			cluster:     cluster,
			machinePool: mp,
			// A nil nodeRefMap skips the "already reconciled" return so this case
			// reaches the empty ProviderIDList guard.
		})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res).To(Equal(ctrl.Result{}))
		g.Expect(mp.Status.NodeRefs).To(BeEmpty())
		g.Expect(mp.Status.Deprecated).To(BeNil())
		g.Expect(workloadClient.Get(ctx, client.ObjectKey{Name: node.Name}, &corev1.Node{})).To(Succeed())
	})

	t.Run("keeps nodeRefs when infrastructure is not provisioned", func(t *testing.T) {
		g := NewWithT(t)

		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-1",
			},
			Spec: corev1.NodeSpec{
				ProviderID: "aws://us-east-1/id-node-1",
			},
		}
		nodeRefs := []corev1.ObjectReference{{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Node",
			Name:       node.Name,
		}}
		workloadClient := fake.NewClientBuilder().WithObjects(node).Build()
		r := &Reconciler{
			Client:       fake.NewClientBuilder().Build(),
			ClusterCache: clustercache.NewFakeClusterCache(workloadClient, client.ObjectKeyFromObject(cluster)),
			recorder:     record.NewFakeRecorder(32),
		}
		mp := &clusterv1.MachinePool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machinepool-3",
				Namespace: metav1.NamespaceDefault,
			},
			Spec: clusterv1.MachinePoolSpec{
				ClusterName:    cluster.Name,
				Replicas:       ptr.To[int32](1),
				ProviderIDList: []string{},
			},
			Status: clusterv1.MachinePoolStatus{
				NodeRefs: nodeRefs,
			},
		}

		res, err := r.reconcileNodeRefs(ctx, &scope{
			cluster:     cluster,
			machinePool: mp,
		})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res).To(Equal(ctrl.Result{}))
		g.Expect(mp.Status.NodeRefs).To(Equal(nodeRefs))
		g.Expect(workloadClient.Get(ctx, client.ObjectKey{Name: node.Name}, &corev1.Node{})).To(Succeed())
	})

	t.Run("keeps nodeRefs when instances exist but provider IDs are empty", func(t *testing.T) {
		g := NewWithT(t)

		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-1",
			},
			Spec: corev1.NodeSpec{
				ProviderID: "aws://us-east-1/id-node-1",
			},
		}
		nodeRefs := []corev1.ObjectReference{{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Node",
			Name:       node.Name,
		}}
		workloadClient := fake.NewClientBuilder().WithObjects(node).Build()
		r := &Reconciler{
			Client:       fake.NewClientBuilder().Build(),
			ClusterCache: clustercache.NewFakeClusterCache(workloadClient, client.ObjectKeyFromObject(cluster)),
			recorder:     record.NewFakeRecorder(32),
		}
		mp := &clusterv1.MachinePool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machinepool-4",
				Namespace: metav1.NamespaceDefault,
			},
			Spec: clusterv1.MachinePoolSpec{
				ClusterName:    cluster.Name,
				Replicas:       ptr.To[int32](1),
				ProviderIDList: []string{},
			},
			Status: clusterv1.MachinePoolStatus{
				Initialization: clusterv1.MachinePoolInitializationStatus{
					InfrastructureProvisioned: ptr.To(true),
				},
				Replicas: ptr.To[int32](1),
				NodeRefs: nodeRefs,
			},
		}

		res, err := r.reconcileNodeRefs(ctx, &scope{
			cluster:     cluster,
			machinePool: mp,
		})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res).To(Equal(ctrl.Result{}))
		g.Expect(mp.Status.NodeRefs).To(Equal(nodeRefs))
		g.Expect(workloadClient.Get(ctx, client.ObjectKey{Name: node.Name}, &corev1.Node{})).To(Succeed())
	})
}
