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
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
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

	g.Expect(env.Create(ctx, testCluster)).To(Succeed())
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
		g.Expect(env.Create(ctx, tc.node)).To(Succeed())
		nodesToCleanup = append(nodesToCleanup, tc.node)
	}
	defer func(do ...client.Object) {
		g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
	}(nodesToCleanup...)

	tracker, err := remote.NewClusterCacheTracker(
		env.Manager, remote.ClusterCacheTrackerOptions{
			Indexes: []remote.Index{remote.NodeProviderIDIndex},
		},
	)
	g.Expect(err).ToNot(HaveOccurred())

	r := &Reconciler{
		Tracker:                   tracker,
		Client:                    env,
		UnstructuredCachingClient: env,
	}

	w, err := ctrl.NewControllerManagedBy(env.Manager).For(&corev1.Node{}).Build(r)
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(tracker.Watch(ctx, remote.WatchInput{
		Name:    "TestGetNode",
		Cluster: util.ObjectKey(testCluster),
		Watcher: w,
		Kind:    &corev1.Node{},
		EventHandler: handler.EnqueueRequestsFromMapFunc(func(context.Context, client.Object) []reconcile.Request {
			return nil
		}),
	})).To(Succeed())

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			remoteClient, err := r.Tracker.GetClient(ctx, util.ObjectKey(testCluster))
			g.Expect(err).ToNot(HaveOccurred())

			node, err := r.getNode(ctx, remoteClient, tc.providerIDInput)
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

	defaultInfraMachine := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "GenericInfrastructureMachine",
			"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
			"metadata": map[string]interface{}{
				"name":      "infra-config1",
				"namespace": metav1.NamespaceDefault,
			},
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

		infraMachine := defaultInfraMachine.DeepCopy()
		infraMachine.SetNamespace(ns.Name)

		interruptibleTrueInfraMachineStatus := map[string]interface{}{
			"interruptible": true,
			"ready":         true,
		}
		interruptibleFalseInfraMachineStatus := map[string]interface{}{
			"interruptible": false,
			"ready":         true,
		}

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
			Status: corev1.NodeStatus{
				Addresses: []corev1.NodeAddress{
					{
						Type:    corev1.NodeInternalIP,
						Address: "1.1.1.1",
					},
					{
						Type:    corev1.NodeInternalIP,
						Address: "2.2.2.2",
					},
				},
			},
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

		g.Expect(env.Create(ctx, infraMachine)).To(Succeed())
		// Set InfrastructureMachine .status.interruptible and .status.ready to true.
		interruptibleTrueInfraMachine := infraMachine.DeepCopy()
		g.Expect(unstructured.SetNestedMap(interruptibleTrueInfraMachine.Object, interruptibleTrueInfraMachineStatus, "status")).Should(Succeed())
		g.Expect(env.Status().Patch(ctx, interruptibleTrueInfraMachine, client.MergeFrom(infraMachine))).Should(Succeed())

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

			// Interruptible label should be set on the node.
			g.Expect(node.Labels).To(HaveKey(clusterv1.InterruptibleLabel))

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

		// Set InfrastructureMachine .status.interruptible to false.
		interruptibleFalseInfraMachine := interruptibleTrueInfraMachine.DeepCopy()
		g.Expect(unstructured.SetNestedMap(interruptibleFalseInfraMachine.Object, interruptibleFalseInfraMachineStatus, "status")).Should(Succeed())
		g.Expect(env.Status().Patch(ctx, interruptibleFalseInfraMachine, client.MergeFrom(interruptibleTrueInfraMachine))).Should(Succeed())

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

			// Interruptible label should not be on node.
			g.Expect(node.Labels).NotTo(HaveKey(clusterv1.InterruptibleLabel))

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

func TestPatchNode(t *testing.T) {
	testCases := []struct {
		name                string
		oldNode             *corev1.Node
		newLabels           map[string]string
		newAnnotations      map[string]string
		expectedLabels      map[string]string
		expectedAnnotations map[string]string
		expectedTaints      []corev1.Taint
	}{
		{
			name: "Check that patch works even if there are Status.Addresses with the same key",
			oldNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("node-%s", util.RandomString(6)),
				},
				Status: corev1.NodeStatus{
					Addresses: []corev1.NodeAddress{
						{
							Type:    corev1.NodeInternalIP,
							Address: "1.1.1.1",
						},
						{
							Type:    corev1.NodeInternalIP,
							Address: "2.2.2.2",
						},
					},
				},
			},
			newLabels:      map[string]string{"foo": "bar"},
			expectedLabels: map[string]string{"foo": "bar"},
			expectedAnnotations: map[string]string{
				clusterv1.LabelsFromMachineAnnotation: "foo",
			},
			expectedTaints: []corev1.Taint{
				{Key: "node.kubernetes.io/not-ready", Effect: "NoSchedule"}, // Added by the API server
			},
		},
		// Labels (CAPI owns a subset of labels, everything else should be preserved)
		{
			name: "Existing labels should be preserved if there are no label from machines",
			oldNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("node-%s", util.RandomString(6)),
					Labels: map[string]string{
						"not-managed-by-capi": "foo",
					},
				},
			},
			expectedLabels: map[string]string{
				"not-managed-by-capi": "foo",
			},
			expectedTaints: []corev1.Taint{
				{Key: "node.kubernetes.io/not-ready", Effect: "NoSchedule"}, // Added by the API server
			},
		},
		{
			name: "Add label must preserve existing labels",
			oldNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("node-%s", util.RandomString(6)),
					Labels: map[string]string{
						"not-managed-by-capi": "foo",
					},
				},
			},
			newLabels: map[string]string{
				"label-from-machine": "foo",
			},
			expectedLabels: map[string]string{
				"not-managed-by-capi": "foo",
				"label-from-machine":  "foo",
			},
			expectedAnnotations: map[string]string{
				clusterv1.LabelsFromMachineAnnotation: "label-from-machine",
			},
			expectedTaints: []corev1.Taint{
				{Key: "node.kubernetes.io/not-ready", Effect: "NoSchedule"}, // Added by the API server
			},
		},
		{
			name: "CAPI takes ownership of existing labels if they are set from machines",
			oldNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("node-%s", util.RandomString(6)),
					Labels: map[string]string{
						clusterv1.NodeRoleLabelPrefix: "foo",
					},
				},
			},
			newLabels: map[string]string{
				clusterv1.NodeRoleLabelPrefix: "control-plane",
			},
			expectedLabels: map[string]string{
				clusterv1.NodeRoleLabelPrefix: "control-plane",
			},
			expectedAnnotations: map[string]string{
				clusterv1.LabelsFromMachineAnnotation: clusterv1.NodeRoleLabelPrefix,
			},
			expectedTaints: []corev1.Taint{
				{Key: "node.kubernetes.io/not-ready", Effect: "NoSchedule"}, // Added by the API server
			},
		},
		{
			name: "change a label previously set from machines",
			oldNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("node-%s", util.RandomString(6)),
					Labels: map[string]string{
						clusterv1.NodeRoleLabelPrefix: "foo",
					},
					Annotations: map[string]string{
						clusterv1.LabelsFromMachineAnnotation: clusterv1.NodeRoleLabelPrefix,
					},
				},
			},
			newLabels: map[string]string{
				clusterv1.NodeRoleLabelPrefix: "control-plane",
			},
			expectedLabels: map[string]string{
				clusterv1.NodeRoleLabelPrefix: "control-plane",
			},
			expectedAnnotations: map[string]string{
				clusterv1.LabelsFromMachineAnnotation: clusterv1.NodeRoleLabelPrefix,
			},
			expectedTaints: []corev1.Taint{
				{Key: "node.kubernetes.io/not-ready", Effect: "NoSchedule"}, // Added by the API server
			},
		},
		{
			name: "Delete a label previously set from machines",
			oldNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("node-%s", util.RandomString(6)),
					Labels: map[string]string{
						clusterv1.NodeRoleLabelPrefix: "foo",
						"not-managed-by-capi":         "foo",
					},
					Annotations: map[string]string{
						clusterv1.LabelsFromMachineAnnotation: clusterv1.NodeRoleLabelPrefix,
					},
				},
			},
			expectedLabels: map[string]string{
				"not-managed-by-capi": "foo",
			},
			expectedAnnotations: map[string]string{
				clusterv1.LabelsFromMachineAnnotation: "",
			},
			expectedTaints: []corev1.Taint{
				{Key: "node.kubernetes.io/not-ready", Effect: "NoSchedule"}, // Added by the API server
			},
		},
		{
			name: "Label previously set from machine, already removed out of band, annotation should be cleaned up",
			oldNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("node-%s", util.RandomString(6)),
					Annotations: map[string]string{
						clusterv1.LabelsFromMachineAnnotation: clusterv1.NodeRoleLabelPrefix,
					},
				},
			},
			expectedAnnotations: map[string]string{
				clusterv1.LabelsFromMachineAnnotation: "",
			},
			expectedTaints: []corev1.Taint{
				{Key: "node.kubernetes.io/not-ready", Effect: "NoSchedule"}, // Added by the API server
			},
		},
		// Add annotations (CAPI only enforces some annotations and never changes or removes them)
		{
			name: "Add CAPI annotations",
			oldNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("node-%s", util.RandomString(6)),
					Annotations: map[string]string{
						"not-managed-by-capi": "foo",
					},
				},
			},
			newAnnotations: map[string]string{
				clusterv1.ClusterNameAnnotation:      "foo",
				clusterv1.ClusterNamespaceAnnotation: "bar",
				clusterv1.MachineAnnotation:          "baz",
			},
			expectedAnnotations: map[string]string{
				clusterv1.ClusterNameAnnotation:       "foo",
				clusterv1.ClusterNamespaceAnnotation:  "bar",
				clusterv1.MachineAnnotation:           "baz",
				"not-managed-by-capi":                 "foo",
				clusterv1.LabelsFromMachineAnnotation: "",
			},
			expectedTaints: []corev1.Taint{
				{Key: "node.kubernetes.io/not-ready", Effect: "NoSchedule"}, // Added by the API server
			},
		},
		// Taint (CAPI only remove one taint if it exists, other taints should be preserved)
		{
			name: "Removes NodeUninitializedTaint if present",
			oldNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("node-%s", util.RandomString(6)),
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
			expectedAnnotations: map[string]string{
				clusterv1.LabelsFromMachineAnnotation: "",
			},
			expectedTaints: []corev1.Taint{
				{
					Key:    "node-role.kubernetes.io/control-plane",
					Effect: corev1.TaintEffectNoSchedule,
				},
				{Key: "node.kubernetes.io/not-ready", Effect: "NoSchedule"}, // Added by the API server
			},
		},
	}

	r := Reconciler{
		Client:                    env,
		UnstructuredCachingClient: env,
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			oldNode := tc.oldNode.DeepCopy()

			g.Expect(env.Create(ctx, oldNode)).To(Succeed())
			t.Cleanup(func() {
				_ = env.Cleanup(ctx, oldNode)
			})

			err := r.patchNode(ctx, env, oldNode, tc.newLabels, tc.newAnnotations)
			g.Expect(err).ToNot(HaveOccurred())

			g.Eventually(func(g Gomega) {
				gotNode := &corev1.Node{}
				err = env.Get(ctx, client.ObjectKeyFromObject(oldNode), gotNode)
				g.Expect(err).ToNot(HaveOccurred())

				g.Expect(gotNode.Labels).To(BeComparableTo(tc.expectedLabels))
				g.Expect(gotNode.Annotations).To(BeComparableTo(tc.expectedAnnotations))
				g.Expect(gotNode.Spec.Taints).To(BeComparableTo(tc.expectedTaints))
			}, 10*time.Second).Should(Succeed())
		})
	}
}
