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

package machine

import (
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/noderefutil"
	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
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
			providerIDInput: "aws://us-west-2/test-get-node-2",
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
			providerIDInput: "gce://us-central1/test-get-node-2",
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

	r := &Reconciler{
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

func TestNodeLabelSync(t *testing.T) {
	defaultCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: metav1.NamespaceDefault,
		},
	}

	defaultMachine := clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine-test",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				clusterv1.MachineControlPlaneLabel: "",
			},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: defaultCluster.Name,
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: &corev1.ObjectReference{
					APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
					Kind:       "GenericBootstrapConfig",
					Name:       "bootstrap-config1",
				},
			},
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "GenericInfrastructureMachine",
				Name:       "infra-config1",
			},
		},
	}

	t.Run("Should sync node labels", func(t *testing.T) {
		g := NewWithT(t)

		ns, err := env.CreateNamespace(ctx, "test-node-label-sync")
		g.Expect(err).ToNot(HaveOccurred())
		defer func() {
			g.Expect(env.Cleanup(ctx, ns)).To(Succeed())
		}()

		nodeProviderID := fmt.Sprintf("test://%s", util.RandomString(6))

		cluster := defaultCluster.DeepCopy()
		cluster.Namespace = ns.Name

		machine := defaultMachine.DeepCopy()
		machine.Namespace = ns.Name
		machine.Spec.ProviderID = pointer.String(nodeProviderID)

		// Set Machine labels.
		machine.Labels = map[string]string{}
		// The expectation is that these labels will be synced to the Node.
		managedMachineLabels := map[string]string{
			clusterv1.NodeRoleLabelPrefix + "/anyRole": "",

			clusterv1.ManagedNodeLabelDomain:                                  "valueFromMachine",
			"custom-prefix." + clusterv1.ManagedNodeLabelDomain:               "valueFromMachine",
			clusterv1.ManagedNodeLabelDomain + "/anything":                    "valueFromMachine",
			"custom-prefix." + clusterv1.ManagedNodeLabelDomain + "/anything": "valueFromMachine",

			clusterv1.NodeRestrictionLabelDomain:                                  "valueFromMachine",
			"custom-prefix." + clusterv1.NodeRestrictionLabelDomain:               "valueFromMachine",
			clusterv1.NodeRestrictionLabelDomain + "/anything":                    "valueFromMachine",
			"custom-prefix." + clusterv1.NodeRestrictionLabelDomain + "/anything": "valueFromMachine",
		}
		for k, v := range managedMachineLabels {
			machine.Labels[k] = v
		}
		// The expectation is that these labels will not be synced to the Node.
		unmanagedMachineLabels := map[string]string{
			"foo":                               "",
			"bar":                               "",
			"company.xyz/node.cluster.x-k8s.io": "not-managed",
			"gpu-node.cluster.x-k8s.io":         "not-managed",
			"company.xyz/node-restriction.kubernetes.io": "not-managed",
			"gpu-node-restriction.kubernetes.io":         "not-managed",
		}
		for k, v := range unmanagedMachineLabels {
			machine.Labels[k] = v
		}

		// Create Node.
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "machine-test-node-",
			},
			Spec: corev1.NodeSpec{ProviderID: nodeProviderID},
		}

		// Set Node labels
		// The expectation is that these labels will be overwritten by the labels
		// from the Machine by the node label sync.
		node.Labels = map[string]string{}
		managedNodeLabelsToBeOverWritten := map[string]string{
			clusterv1.NodeRoleLabelPrefix + "/anyRole": "valueFromNode",

			clusterv1.ManagedNodeLabelDomain:                                  "valueFromNode",
			"custom-prefix." + clusterv1.ManagedNodeLabelDomain:               "valueFromNode",
			clusterv1.ManagedNodeLabelDomain + "/anything":                    "valueFromNode",
			"custom-prefix." + clusterv1.ManagedNodeLabelDomain + "/anything": "valueFromNode",

			clusterv1.NodeRestrictionLabelDomain:                                  "valueFromNode",
			"custom-prefix." + clusterv1.NodeRestrictionLabelDomain:               "valueFromNode",
			clusterv1.NodeRestrictionLabelDomain + "/anything":                    "valueFromNode",
			"custom-prefix." + clusterv1.NodeRestrictionLabelDomain + "/anything": "valueFromNode",
		}
		for k, v := range managedNodeLabelsToBeOverWritten {
			node.Labels[k] = v
		}
		// The expectation is that these labels will be preserved by the node label sync.
		unmanagedNodeLabelsToBePreserved := map[string]string{
			"node-role.kubernetes.io/control-plane": "",
			"label":                                 "valueFromNode",
		}
		for k, v := range unmanagedNodeLabelsToBePreserved {
			node.Labels[k] = v
		}

		g.Expect(env.Create(ctx, node)).To(Succeed())
		defer func() {
			g.Expect(env.Cleanup(ctx, node)).To(Succeed())
		}()

		g.Expect(env.Create(ctx, cluster)).To(Succeed())
		defaultKubeconfigSecret := kubeconfig.GenerateSecret(cluster, kubeconfig.FromEnvTestConfig(env.Config, cluster))
		g.Expect(env.Create(ctx, defaultKubeconfigSecret)).To(Succeed())
		g.Expect(env.Create(ctx, machine)).To(Succeed())

		// Validate that the right labels where synced to the Node.
		g.Eventually(func(g Gomega) bool {
			if err := env.Get(ctx, client.ObjectKeyFromObject(node), node); err != nil {
				return false
			}

			// Managed Machine Labels should have been synced to the Node.
			for k, v := range managedMachineLabels {
				g.Expect(node.Labels).To(HaveKeyWithValue(k, v))
			}
			// Unmanaged Machine labels should not have been synced to the Node.
			for k, v := range unmanagedMachineLabels {
				g.Expect(node.Labels).ToNot(HaveKeyWithValue(k, v))
			}

			// Pre-existing managed Node labels should have been overwritten on the Node.
			for k, v := range managedNodeLabelsToBeOverWritten {
				g.Expect(node.Labels).ToNot(HaveKeyWithValue(k, v))
			}
			// Pre-existing unmanaged Node labels should have been preserved on the Node.
			for k, v := range unmanagedNodeLabelsToBePreserved {
				g.Expect(node.Labels).To(HaveKeyWithValue(k, v))
			}

			return true
		}, 10*time.Second).Should(BeTrue())

		// Remove managed labels from Machine.
		modifiedMachine := machine.DeepCopy()
		for k := range managedMachineLabels {
			delete(modifiedMachine.Labels, k)
		}
		g.Expect(env.Patch(ctx, modifiedMachine, client.MergeFrom(machine))).To(Succeed())

		// Validate that managed Machine labels were removed from the Node and all others are not changed.
		g.Eventually(func(g Gomega) bool {
			if err := env.Get(ctx, client.ObjectKeyFromObject(node), node); err != nil {
				return false
			}

			// Managed Machine Labels should have been removed from the Node now.
			for k, v := range managedMachineLabels {
				g.Expect(node.Labels).ToNot(HaveKeyWithValue(k, v))
			}
			// Unmanaged Machine labels should not have been synced at all to the Node.
			for k, v := range unmanagedMachineLabels {
				g.Expect(node.Labels).ToNot(HaveKeyWithValue(k, v))
			}

			// Pre-existing managed Node labels have been overwritten earlier by the managed Machine labels.
			// Now that the managed Machine labels have been removed, they should still not exist.
			for k, v := range managedNodeLabelsToBeOverWritten {
				g.Expect(node.Labels).ToNot(HaveKeyWithValue(k, v))
			}
			// Pre-existing unmanaged Node labels should have been preserved on the Node.
			for k, v := range unmanagedNodeLabelsToBePreserved {
				g.Expect(node.Labels).To(HaveKeyWithValue(k, v))
			}

			return true
		}, 10*time.Second).Should(BeTrue())
	})
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

func TestGetManagedLabels(t *testing.T) {
	// Create managedLabels map from known managed prefixes.
	managedLabels := map[string]string{
		clusterv1.NodeRoleLabelPrefix + "/anyRole": "",

		clusterv1.ManagedNodeLabelDomain:                                  "",
		"custom-prefix." + clusterv1.ManagedNodeLabelDomain:               "",
		clusterv1.ManagedNodeLabelDomain + "/anything":                    "",
		"custom-prefix." + clusterv1.ManagedNodeLabelDomain + "/anything": "",

		clusterv1.NodeRestrictionLabelDomain:                                  "",
		"custom-prefix." + clusterv1.NodeRestrictionLabelDomain:               "",
		clusterv1.NodeRestrictionLabelDomain + "/anything":                    "",
		"custom-prefix." + clusterv1.NodeRestrictionLabelDomain + "/anything": "",
	}

	// Append arbitrary labels.
	allLabels := map[string]string{
		"foo":                               "",
		"bar":                               "",
		"company.xyz/node.cluster.x-k8s.io": "not-managed",
		"gpu-node.cluster.x-k8s.io":         "not-managed",
		"company.xyz/node-restriction.kubernetes.io": "not-managed",
		"gpu-node-restriction.kubernetes.io":         "not-managed",
	}
	for k, v := range managedLabels {
		allLabels[k] = v
	}

	g := NewWithT(t)
	got := getManagedLabels(allLabels)
	g.Expect(got).To(BeEquivalentTo(managedLabels))
}

func TestReconcileNodeTaints(t *testing.T) {
	tests := []struct {
		name string
		node *corev1.Node
	}{
		{
			name: "if the node has the uninitialized taint it should be dropped from the node",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-1",
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						clusterv1.NodeUninitializedTaint,
					},
				},
			},
		},
		{
			name: "if the node is a control plane and has the uninitialized taint it should be dropped from the node",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-1",
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    "node-role.kubernetes.io/control-plane",
							Effect: corev1.TaintEffectNoSchedule,
						},
						clusterv1.NodeUninitializedTaint,
					},
				},
			},
		},
		{
			name: "if the node does not have the uninitialized taint it should remain absent from the node",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-1",
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			fakeClient := fake.NewClientBuilder().WithObjects(tt.node).WithScheme(fakeScheme).Build()
			nodeBefore := tt.node.DeepCopy()
			r := &Reconciler{}
			g.Expect(r.reconcileNodeTaints(ctx, fakeClient, tt.node)).Should(Succeed())
			// Verify the NodeUninitializedTaint is dropped.
			g.Expect(tt.node.Spec.Taints).ShouldNot(ContainElement(clusterv1.NodeUninitializedTaint))
			// Verify all other taints are same.
			for _, taint := range nodeBefore.Spec.Taints {
				if !taint.MatchTaint(&clusterv1.NodeUninitializedTaint) {
					g.Expect(tt.node.Spec.Taints).Should(ContainElement(taint))
				}
			}
		})
	}
}
