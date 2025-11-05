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
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/api/core/v1beta2/index"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/cluster-api/internal/topology/ownerrefs"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestReconcileNode(t *testing.T) {
	defaultMachine := clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine-test",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: "test-cluster",
			},
		},
		Spec: clusterv1.MachineSpec{
			ProviderID: "aws://us-east-1/test-node-1",
		},
	}

	defaultCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: metav1.NamespaceDefault,
		},
	}

	testCases := []struct {
		name               string
		machine            *clusterv1.Machine
		node               *corev1.Node
		nodeGetErr         bool
		expectResult       ctrl.Result
		expectError        bool
		expected           func(g *WithT, m *clusterv1.Machine)
		expectNodeGetError bool
	}{
		{
			name:         "No op if provider ID is not set",
			machine:      &clusterv1.Machine{},
			node:         nil,
			nodeGetErr:   false,
			expectResult: ctrl.Result{},
			expectError:  false,
		},
		{
			name:               "err reading node (something different than not found), it should return error",
			machine:            defaultMachine.DeepCopy(),
			node:               nil,
			nodeGetErr:         true,
			expectResult:       ctrl.Result{},
			expectError:        true,
			expectNodeGetError: true,
		},
		{
			name:         "waiting for the node to exist, no op",
			machine:      defaultMachine.DeepCopy(),
			node:         nil,
			nodeGetErr:   false,
			expectResult: ctrl.Result{},
			expectError:  false,
		},
		{
			name:    "node found, should surface info",
			machine: defaultMachine.DeepCopy(),
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-1",
				},
				Spec: corev1.NodeSpec{
					ProviderID: "aws://us-east-1/test-node-1",
				},
				Status: corev1.NodeStatus{
					NodeInfo: corev1.NodeSystemInfo{
						MachineID: "foo",
					},
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
			nodeGetErr:   false,
			expectResult: ctrl.Result{},
			expectError:  false,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.Status.NodeRef.Name).To(Equal("test-node-1"))
				g.Expect(m.Status.NodeInfo).ToNot(BeNil())
				g.Expect(m.Status.NodeInfo.MachineID).To(Equal("foo"))
			},
		},
		{
			name: "node not found when already seen, should error",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine-test",
					Namespace: metav1.NamespaceDefault,
					Labels: map[string]string{
						clusterv1.ClusterNameLabel: "test-cluster",
					},
				},
				Spec: clusterv1.MachineSpec{
					ProviderID: "aws://us-east-1/test-node-1",
				},
				Status: clusterv1.MachineStatus{
					NodeRef: clusterv1.MachineNodeReference{
						Name: "test-node-1",
					},
				},
			},
			node:         nil,
			nodeGetErr:   false,
			expectResult: ctrl.Result{},
			expectError:  true,
		},
		{
			name: "node not found is tolerated when machine is deleting",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine-test",
					Namespace: metav1.NamespaceDefault,
					Labels: map[string]string{
						clusterv1.ClusterNameLabel: "test-cluster",
					},
					DeletionTimestamp: ptr.To(metav1.Now()),
					Finalizers:        []string{"foo"},
				},
				Spec: clusterv1.MachineSpec{
					ProviderID: "aws://us-east-1/test-node-1",
				},
				Status: clusterv1.MachineStatus{
					NodeRef: clusterv1.MachineNodeReference{
						Name: "test-node-1",
					},
				},
			},
			node:         nil,
			nodeGetErr:   false,
			expectResult: ctrl.Result{},
			expectError:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			c := fake.NewClientBuilder().WithObjects(tc.machine).WithIndex(&corev1.Node{}, "spec.providerID", index.NodeByProviderID).Build()
			if tc.nodeGetErr {
				c = fake.NewClientBuilder().WithObjects(tc.machine).Build() // No Index
			}

			if tc.node != nil {
				g.Expect(c.Create(ctx, tc.node)).To(Succeed())
				defer func() { _ = c.Delete(ctx, tc.node) }()
			}

			r := &Reconciler{
				ClusterCache: clustercache.NewFakeClusterCache(c, client.ObjectKeyFromObject(defaultCluster)),
				Client:       c,
				recorder:     record.NewFakeRecorder(10),
			}
			s := &scope{cluster: defaultCluster, machine: tc.machine}
			result, err := r.reconcileNode(ctx, s)
			g.Expect(result).To(BeComparableTo(tc.expectResult))
			if tc.expectError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}

			if tc.expected != nil {
				tc.expected(g, tc.machine)
			}

			g.Expect(s.nodeGetError != nil).To(Equal(tc.expectNodeGetError))
		})
	}
}

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
		Spec: clusterv1.ClusterSpec{
			ControlPlaneRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: builder.ControlPlaneGroupVersion.Group,
				Kind:     builder.GenericControlPlaneKind,
				Name:     "cp1",
			},
		},
	}

	g.Expect(env.Create(ctx, testCluster)).To(Succeed())
	// Set InfrastructureReady to true so ClusterCache creates the clusterAccessor.
	patch := client.MergeFrom(testCluster.DeepCopy())
	testCluster.Status.Initialization.InfrastructureProvisioned = ptr.To(true)
	g.Expect(env.Status().Patch(ctx, testCluster, patch)).To(Succeed())

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

	clusterCache, err := clustercache.SetupWithManager(ctx, env.Manager, clustercache.Options{
		SecretClient: env.GetClient(),
		Cache: clustercache.CacheOptions{
			Indexes: []clustercache.CacheOptionsIndex{clustercache.NodeProviderIDIndex},
		},
		Client: clustercache.ClientOptions{
			UserAgent: remote.DefaultClusterAPIUserAgent("test-controller-manager"),
			Cache: clustercache.ClientCacheOptions{
				DisableFor: []client.Object{
					// Don't cache ConfigMaps & Secrets.
					&corev1.ConfigMap{},
					&corev1.Secret{},
				},
			},
		},
	}, controller.Options{MaxConcurrentReconciles: 10, SkipNameValidation: ptr.To(true)})
	if err != nil {
		panic(fmt.Sprintf("Failed to create ClusterCache: %v", err))
	}
	defer clusterCache.(interface{ Shutdown() }).Shutdown()

	r := &Reconciler{
		ClusterCache: clusterCache,
		Client:       env,
	}

	w, err := ctrl.NewControllerManagedBy(env.Manager).For(&corev1.Node{}).Build(r)
	g.Expect(err).ToNot(HaveOccurred())

	// Retry because the ClusterCache might not have immediately created the clusterAccessor.
	g.Eventually(func(g Gomega) {
		g.Expect(clusterCache.Watch(ctx, util.ObjectKey(testCluster), clustercache.NewWatcher(clustercache.WatcherOptions{
			Name:    "TestGetNode",
			Watcher: w,
			Kind:    &corev1.Node{},
			EventHandler: handler.EnqueueRequestsFromMapFunc(func(context.Context, client.Object) []reconcile.Request {
				return nil
			}),
		}))).To(Succeed())
	}, 1*time.Minute, 5*time.Second).Should(Succeed())

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			remoteClient, err := r.ClusterCache.GetClient(ctx, util.ObjectKey(testCluster))
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
		Spec: clusterv1.ClusterSpec{
			ControlPlaneRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: builder.ControlPlaneGroupVersion.Group,
				Kind:     builder.GenericControlPlaneKind,
				Name:     "cp1",
			},
		},
	}

	defaultInfraMachine := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "GenericInfrastructureMachine",
			"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
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
				ConfigRef: clusterv1.ContractVersionedObjectReference{
					APIGroup: clusterv1.GroupVersionBootstrap.Group,
					Kind:     "GenericBootstrapConfig",
					Name:     "bootstrap-config1",
				},
			},
			InfrastructureRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: clusterv1.GroupVersionInfrastructure.Group,
				Kind:     "GenericInfrastructureMachine",
				Name:     "infra-config1",
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
		machine.Spec.ProviderID = nodeProviderID

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
		g.Expect(env.CreateAndWait(ctx, defaultKubeconfigSecret)).To(Succeed())
		// Set InfrastructureReady to true so ClusterCache creates the clusterAccessor.
		patch := client.MergeFrom(cluster.DeepCopy())
		cluster.Status.Initialization.InfrastructureProvisioned = ptr.To(true)
		g.Expect(env.Status().Patch(ctx, cluster, patch)).To(Succeed())

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
			status, _ := summarizeNodeV1beta1Conditions(node)
			g.Expect(status).To(Equal(test.status))
		})
	}
}

func TestPatchNode(t *testing.T) {
	clusterName := "test-cluster"

	testCases := []struct {
		name                string
		oldNode             *corev1.Node
		newLabels           map[string]string
		newAnnotations      map[string]string
		expectedLabels      map[string]string
		expectedAnnotations map[string]string
		expectedTaints      []corev1.Taint
		machine             *clusterv1.Machine
		ms                  *clusterv1.MachineSet
		md                  *clusterv1.MachineDeployment
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
				clusterv1.AnnotationsFromMachineAnnotation: "",
				clusterv1.LabelsFromMachineAnnotation:      "foo",
			},
			expectedTaints: []corev1.Taint{
				{Key: "node.kubernetes.io/not-ready", Effect: "NoSchedule"}, // Added by the API server
			},
			machine: newFakeMachine(metav1.NamespaceDefault, clusterName),
			ms:      newFakeMachineSet(metav1.NamespaceDefault, clusterName),
			md:      newFakeMachineDeployment(metav1.NamespaceDefault, clusterName),
		},
		// Labels (CAPI owns a subset of labels, everything else should be preserved)
		{
			name: "Existing labels should be preserved if there are no labels from machines",
			oldNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("node-%s", util.RandomString(6)),
					Labels: map[string]string{
						"not-managed-by-capi": "foo",
					},
				},
			},
			expectedAnnotations: map[string]string{
				clusterv1.AnnotationsFromMachineAnnotation: "",
				clusterv1.LabelsFromMachineAnnotation:      "",
			},
			expectedLabels: map[string]string{
				"not-managed-by-capi": "foo",
			},
			expectedTaints: []corev1.Taint{
				{Key: "node.kubernetes.io/not-ready", Effect: "NoSchedule"}, // Added by the API server
			},
			machine: newFakeMachine(metav1.NamespaceDefault, clusterName),
			ms:      newFakeMachineSet(metav1.NamespaceDefault, clusterName),
			md:      newFakeMachineDeployment(metav1.NamespaceDefault, clusterName),
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
				clusterv1.AnnotationsFromMachineAnnotation: "",
				clusterv1.LabelsFromMachineAnnotation:      "label-from-machine",
			},
			expectedTaints: []corev1.Taint{
				{Key: "node.kubernetes.io/not-ready", Effect: "NoSchedule"}, // Added by the API server
			},
			machine: newFakeMachine(metav1.NamespaceDefault, clusterName),
			ms:      newFakeMachineSet(metav1.NamespaceDefault, clusterName),
			md:      newFakeMachineDeployment(metav1.NamespaceDefault, clusterName),
		},
		{
			name: "Add annotation must preserve existing annotations",
			oldNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("node-%s", util.RandomString(6)),
					Annotations: map[string]string{
						"not-managed-by-capi": "foo",
					},
				},
			},
			newAnnotations: map[string]string{
				"managed-by-capi": "bar",
			},
			expectedAnnotations: map[string]string{
				"not-managed-by-capi":                      "foo",
				"managed-by-capi":                          "bar",
				clusterv1.AnnotationsFromMachineAnnotation: "managed-by-capi",
				clusterv1.LabelsFromMachineAnnotation:      "",
			},
			expectedTaints: []corev1.Taint{
				{Key: "node.kubernetes.io/not-ready", Effect: "NoSchedule"}, // Added by the API server
			},
			machine: newFakeMachine(metav1.NamespaceDefault, clusterName),
			ms:      newFakeMachineSet(metav1.NamespaceDefault, clusterName),
			md:      newFakeMachineDeployment(metav1.NamespaceDefault, clusterName),
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
				clusterv1.AnnotationsFromMachineAnnotation: "",
				clusterv1.LabelsFromMachineAnnotation:      clusterv1.NodeRoleLabelPrefix,
			},
			expectedTaints: []corev1.Taint{
				{Key: "node.kubernetes.io/not-ready", Effect: "NoSchedule"}, // Added by the API server
			},
			machine: newFakeMachine(metav1.NamespaceDefault, clusterName),
			ms:      newFakeMachineSet(metav1.NamespaceDefault, clusterName),
			md:      newFakeMachineDeployment(metav1.NamespaceDefault, clusterName),
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
				clusterv1.AnnotationsFromMachineAnnotation: "",
				clusterv1.LabelsFromMachineAnnotation:      clusterv1.NodeRoleLabelPrefix,
			},
			expectedTaints: []corev1.Taint{
				{Key: "node.kubernetes.io/not-ready", Effect: "NoSchedule"}, // Added by the API server
			},
			machine: newFakeMachine(metav1.NamespaceDefault, clusterName),
			ms:      newFakeMachineSet(metav1.NamespaceDefault, clusterName),
			md:      newFakeMachineDeployment(metav1.NamespaceDefault, clusterName),
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
				clusterv1.AnnotationsFromMachineAnnotation: "",
				clusterv1.LabelsFromMachineAnnotation:      "",
			},
			expectedTaints: []corev1.Taint{
				{Key: "node.kubernetes.io/not-ready", Effect: "NoSchedule"}, // Added by the API server
			},
			machine: newFakeMachine(metav1.NamespaceDefault, clusterName),
			ms:      newFakeMachineSet(metav1.NamespaceDefault, clusterName),
			md:      newFakeMachineDeployment(metav1.NamespaceDefault, clusterName),
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
				clusterv1.AnnotationsFromMachineAnnotation: "",
				clusterv1.LabelsFromMachineAnnotation:      "",
			},
			expectedTaints: []corev1.Taint{
				{Key: "node.kubernetes.io/not-ready", Effect: "NoSchedule"}, // Added by the API server
			},
			machine: newFakeMachine(metav1.NamespaceDefault, clusterName),
			ms:      newFakeMachineSet(metav1.NamespaceDefault, clusterName),
			md:      newFakeMachineDeployment(metav1.NamespaceDefault, clusterName),
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
				clusterv1.ClusterNameAnnotation:            "foo",
				clusterv1.ClusterNamespaceAnnotation:       "bar",
				clusterv1.MachineAnnotation:                "baz",
				"not-managed-by-capi":                      "foo",
				clusterv1.AnnotationsFromMachineAnnotation: "",
				clusterv1.LabelsFromMachineAnnotation:      "",
			},
			expectedTaints: []corev1.Taint{
				{Key: "node.kubernetes.io/not-ready", Effect: "NoSchedule"}, // Added by the API server
			},
			machine: newFakeMachine(metav1.NamespaceDefault, clusterName),
			ms:      newFakeMachineSet(metav1.NamespaceDefault, clusterName),
			md:      newFakeMachineDeployment(metav1.NamespaceDefault, clusterName),
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
				clusterv1.AnnotationsFromMachineAnnotation: "",
				clusterv1.LabelsFromMachineAnnotation:      "",
			},
			expectedTaints: []corev1.Taint{
				{
					Key:    "node-role.kubernetes.io/control-plane",
					Effect: corev1.TaintEffectNoSchedule,
				},
				{Key: "node.kubernetes.io/not-ready", Effect: "NoSchedule"}, // Added by the API server
			},
			machine: newFakeMachine(metav1.NamespaceDefault, clusterName),
			ms:      newFakeMachineSet(metav1.NamespaceDefault, clusterName),
			md:      newFakeMachineDeployment(metav1.NamespaceDefault, clusterName),
		},
		{
			name: "Ensure Labels and Annotations still get patched if MachineSet and Machinedeployment cannot be found",
			oldNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("node-%s", util.RandomString(6)),
				},
			},
			newLabels: map[string]string{
				"label-from-machine": "foo",
			},
			newAnnotations: map[string]string{
				"annotation-from-machine": "foo",
			},
			expectedLabels: map[string]string{
				"label-from-machine": "foo",
			},
			expectedAnnotations: map[string]string{
				"annotation-from-machine":                  "foo",
				clusterv1.AnnotationsFromMachineAnnotation: "annotation-from-machine",
				clusterv1.LabelsFromMachineAnnotation:      "label-from-machine",
			},
			expectedTaints: []corev1.Taint{
				{Key: "node.kubernetes.io/not-ready", Effect: "NoSchedule"}, // Added by the API server
			},
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("ma-%s", util.RandomString(6)),
					Namespace: metav1.NamespaceDefault,
					Labels: map[string]string{
						clusterv1.MachineSetNameLabel:        "test-ms-missing",
						clusterv1.MachineDeploymentNameLabel: "test-md",
					},
					OwnerReferences: []metav1.OwnerReference{{
						Kind:       "MachineSet",
						Name:       "test-ms-missing",
						APIVersion: clusterv1.GroupVersion.String(),
						UID:        "uid",
					}},
				},
				Spec: newFakeMachineSpec(clusterName),
			},
			ms: nil,
			md: nil,
		},
		{
			name: "Ensure NodeOutdatedRevisionTaint to be set if a node is associated to an outdated machineset",
			oldNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("node-%s", util.RandomString(6)),
				},
			},
			expectedAnnotations: map[string]string{
				clusterv1.AnnotationsFromMachineAnnotation: "",
				clusterv1.LabelsFromMachineAnnotation:      "",
			},
			expectedTaints: []corev1.Taint{
				{Key: "node.kubernetes.io/not-ready", Effect: "NoSchedule"}, // Added by the API server
				clusterv1.NodeOutdatedRevisionTaint,
			},
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("ma-%s", util.RandomString(6)),
					Namespace: metav1.NamespaceDefault,
					Labels: map[string]string{
						clusterv1.MachineSetNameLabel:        "test-ms-outdated",
						clusterv1.MachineDeploymentNameLabel: "test-md-outdated",
					},
					OwnerReferences: []metav1.OwnerReference{{
						Kind:       "MachineSet",
						Name:       "test-ms-outdated",
						APIVersion: clusterv1.GroupVersion.String(),
						UID:        "uid",
					}},
				},
				Spec: newFakeMachineSpec(clusterName),
			},
			ms: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ms-outdated",
					Namespace: metav1.NamespaceDefault,
					Annotations: map[string]string{
						clusterv1.RevisionAnnotation: "1",
					},
				},
				Spec: clusterv1.MachineSetSpec{
					ClusterName: clusterName,
					Template: clusterv1.MachineTemplateSpec{
						Spec: newFakeMachineSpec(clusterName),
					},
				},
			},
			md: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-md-outdated",
					Namespace: metav1.NamespaceDefault,
					Annotations: map[string]string{
						clusterv1.RevisionAnnotation: "2",
					},
				},
				Spec: clusterv1.MachineDeploymentSpec{
					ClusterName: clusterName,
					Template: clusterv1.MachineTemplateSpec{
						Spec: newFakeMachineSpec(clusterName),
					},
				},
			},
		},
		{
			name: "Removes NodeOutdatedRevisionTaint if a node is associated to a non-outdated machineset",
			oldNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("node-%s", util.RandomString(6)),
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						clusterv1.NodeOutdatedRevisionTaint,
					},
				},
			},
			expectedAnnotations: map[string]string{
				clusterv1.AnnotationsFromMachineAnnotation: "",
				clusterv1.LabelsFromMachineAnnotation:      "",
			},
			expectedTaints: []corev1.Taint{
				{Key: "node.kubernetes.io/not-ready", Effect: "NoSchedule"}, // Added by the API server
			},
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("ma-%s", util.RandomString(6)),
					Namespace: metav1.NamespaceDefault,
					Labels: map[string]string{
						clusterv1.MachineSetNameLabel:        "test-ms-not-outdated",
						clusterv1.MachineDeploymentNameLabel: "test-md-not-outdated",
					},
					OwnerReferences: []metav1.OwnerReference{{
						Kind:       "MachineSet",
						Name:       "test-ms-not-outdated",
						APIVersion: clusterv1.GroupVersion.String(),
						UID:        "uid",
					}},
				},
				Spec: newFakeMachineSpec(clusterName),
			},
			ms: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ms-not-outdated",
					Namespace: metav1.NamespaceDefault,
					Annotations: map[string]string{
						clusterv1.RevisionAnnotation: "3",
					},
				},
				Spec: clusterv1.MachineSetSpec{
					ClusterName: clusterName,
					Template: clusterv1.MachineTemplateSpec{
						Spec: newFakeMachineSpec(clusterName),
					},
				},
			},
			md: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-md-not-outdated",
					Namespace: metav1.NamespaceDefault,
					Annotations: map[string]string{
						clusterv1.RevisionAnnotation: "2",
					},
				},
				Spec: clusterv1.MachineDeploymentSpec{
					ClusterName: clusterName,
					Template: clusterv1.MachineTemplateSpec{
						Spec: newFakeMachineSpec(clusterName),
					},
				},
			},
		},
	}

	r := Reconciler{
		Client: env,
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			oldNode := tc.oldNode.DeepCopy()
			machine := tc.machine.DeepCopy()
			ms := tc.ms.DeepCopy()
			md := tc.md.DeepCopy()

			g.Expect(env.CreateAndWait(ctx, oldNode)).To(Succeed())
			g.Expect(env.CreateAndWait(ctx, machine)).To(Succeed())
			if ms != nil {
				g.Expect(env.CreateAndWait(ctx, ms)).To(Succeed())
			}
			if md != nil {
				g.Expect(env.CreateAndWait(ctx, md)).To(Succeed())
			}
			t.Cleanup(func() {
				_ = env.CleanupAndWait(ctx, oldNode, machine, ms, md)
			})

			err := r.patchNode(ctx, env, oldNode, tc.newLabels, tc.newAnnotations, ms, md)
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

// TestMultiplePatchNode verifies that node metadata behaves as expected through at least two reconciliations.
func TestMultiplePatchNode(t *testing.T) {
	clusterName := "test-cluster"
	labels := map[string]string{}

	testCases := []struct {
		name                      string
		oldNode                   *corev1.Node
		newAnnotations            map[string]string
		expectedLabels            map[string]string
		firstExpectedAnnotations  map[string]string
		secondExpectedAnnotations map[string]string
	}{
		{
			name: "Managed annotations should not be in the tracking annotation when machine is synced to node multiple times",
			oldNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        fmt.Sprintf("node-%s", util.RandomString(6)),
					Annotations: map[string]string{},
				},
			},
			newAnnotations: map[string]string{
				clusterv1.ClusterNameAnnotation:      "foo",
				clusterv1.ClusterNamespaceAnnotation: "bar",
				clusterv1.MachineAnnotation:          "baz",
			},
			firstExpectedAnnotations: map[string]string{
				clusterv1.ClusterNameAnnotation:            "foo",
				clusterv1.ClusterNamespaceAnnotation:       "bar",
				clusterv1.MachineAnnotation:                "baz",
				clusterv1.AnnotationsFromMachineAnnotation: "",
				clusterv1.LabelsFromMachineAnnotation:      "",
			},
			secondExpectedAnnotations: map[string]string{
				clusterv1.ClusterNameAnnotation:            "foo",
				clusterv1.ClusterNamespaceAnnotation:       "bar",
				clusterv1.MachineAnnotation:                "baz",
				clusterv1.AnnotationsFromMachineAnnotation: "",
				clusterv1.LabelsFromMachineAnnotation:      "",
			},
		},
		{
			name: "User-managed annotations should be tracked through reconciles",
			oldNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        fmt.Sprintf("node-%s", util.RandomString(6)),
					Annotations: map[string]string{},
				},
			},
			newAnnotations: map[string]string{
				clusterv1.ClusterNameAnnotation:      "foo",
				clusterv1.ClusterNamespaceAnnotation: "bar",
				clusterv1.MachineAnnotation:          "baz",
				"node.cluster.x-k8s.io/keep-this":    "foo",
			},
			firstExpectedAnnotations: map[string]string{
				clusterv1.ClusterNameAnnotation:            "foo",
				clusterv1.ClusterNamespaceAnnotation:       "bar",
				clusterv1.MachineAnnotation:                "baz",
				clusterv1.AnnotationsFromMachineAnnotation: "node.cluster.x-k8s.io/keep-this",
				clusterv1.LabelsFromMachineAnnotation:      "",
				"node.cluster.x-k8s.io/keep-this":          "foo",
			},
			secondExpectedAnnotations: map[string]string{
				clusterv1.ClusterNameAnnotation:            "foo",
				clusterv1.ClusterNamespaceAnnotation:       "bar",
				clusterv1.MachineAnnotation:                "baz",
				clusterv1.AnnotationsFromMachineAnnotation: "node.cluster.x-k8s.io/keep-this",
				clusterv1.LabelsFromMachineAnnotation:      "",
				"node.cluster.x-k8s.io/keep-this":          "foo",
			},
		},
	}

	r := Reconciler{
		Client: env,
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			oldNode := tc.oldNode.DeepCopy()
			machine := newFakeMachine(metav1.NamespaceDefault, clusterName)

			g.Expect(env.CreateAndWait(ctx, oldNode)).To(Succeed())
			g.Expect(env.CreateAndWait(ctx, machine)).To(Succeed())
			t.Cleanup(func() {
				_ = env.CleanupAndWait(ctx, oldNode, machine)
			})

			err := r.patchNode(ctx, env, oldNode, labels, tc.newAnnotations, nil, nil)
			g.Expect(err).ToNot(HaveOccurred())

			newNode := &corev1.Node{}

			g.Eventually(func(g Gomega) {
				newNode = &corev1.Node{}
				err = env.Get(ctx, client.ObjectKeyFromObject(oldNode), newNode)
				g.Expect(err).ToNot(HaveOccurred())

				g.Expect(newNode.Annotations).To(Equal(tc.firstExpectedAnnotations))
			}, 10*time.Second).Should(Succeed())

			// Re-reconcile with the same metadata
			err = r.patchNode(ctx, env, newNode, labels, tc.newAnnotations, nil, nil)
			g.Expect(err).ToNot(HaveOccurred())

			g.Eventually(func(g Gomega) {
				gotNode := &corev1.Node{}
				err = env.Get(ctx, client.ObjectKeyFromObject(oldNode), gotNode)
				g.Expect(err).ToNot(HaveOccurred())

				g.Expect(gotNode.Annotations).To(Equal(tc.secondExpectedAnnotations))
			}, 10*time.Second).Should(Succeed())
		})
	}
}
func newFakeMachineSpec(clusterName string) clusterv1.MachineSpec {
	return clusterv1.MachineSpec{
		ClusterName: clusterName,
		Bootstrap: clusterv1.Bootstrap{
			ConfigRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: "bootstrap.cluster.x-k8s.io",
				Kind:     "KubeadmConfigTemplate",
				Name:     fmt.Sprintf("%s-md-0", clusterName),
			},
		},
		InfrastructureRef: clusterv1.ContractVersionedObjectReference{
			APIGroup: "infrastructure.cluster.x-k8s.io",
			Kind:     "FakeMachineTemplate",
			Name:     fmt.Sprintf("%s-md-0", clusterName),
		},
	}
}

func newFakeMachine(namespace, clusterName string) *clusterv1.Machine {
	return &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("ma-%s", util.RandomString(6)),
			Namespace: namespace,
		},
		Spec: newFakeMachineSpec(clusterName),
	}
}

func newFakeMachineSet(namespace, clusterName string) *clusterv1.MachineSet {
	return &clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("ms-%s", util.RandomString(6)),
			Namespace: namespace,
		},
		Spec: clusterv1.MachineSetSpec{
			ClusterName: clusterName,
			Template: clusterv1.MachineTemplateSpec{
				Spec: newFakeMachineSpec(clusterName),
			},
		},
	}
}

func newFakeMachineDeployment(namespace, clusterName string) *clusterv1.MachineDeployment {
	return &clusterv1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("md-%s", util.RandomString(6)),
			Namespace: namespace,
		},
		Spec: clusterv1.MachineDeploymentSpec{
			ClusterName: clusterName,
			Template: clusterv1.MachineTemplateSpec{
				Spec: newFakeMachineSpec(clusterName),
			},
		},
	}
}

func Test_shouldNodeHaveOutdatedTaint(t *testing.T) {
	namespaceName := "test"

	testMachineDeployment := builder.MachineDeployment(namespaceName, "my-md").
		WithAnnotations(map[string]string{clusterv1.RevisionAnnotation: "1"}).
		Build()
	testMachineDeploymentNew := testMachineDeployment.DeepCopy()
	testMachineDeploymentNew.Annotations = map[string]string{clusterv1.RevisionAnnotation: "2"}

	testMachineSet := builder.MachineSet(namespaceName, "my-ms").
		WithOwnerReferences([]metav1.OwnerReference{*ownerrefs.OwnerReferenceTo(testMachineDeployment, clusterv1.GroupVersion.WithKind("MachineDeployment"))}).
		Build()
	testMachineSet.Annotations = map[string]string{clusterv1.RevisionAnnotation: "1"}

	tests := []struct {
		name              string
		machineSet        *clusterv1.MachineSet
		machineDeployment *clusterv1.MachineDeployment
		wantOutdated      bool
		wantErr           bool
	}{
		{
			name:              "Machineset not outdated",
			machineSet:        testMachineSet,
			machineDeployment: testMachineDeployment,
			wantOutdated:      false,
			wantErr:           false,
		},
		{
			name:              "Machineset outdated",
			machineSet:        testMachineSet,
			machineDeployment: testMachineDeploymentNew,
			wantOutdated:      true,
			wantErr:           false,
		},
		{
			name:              "Stand-alone machine",
			machineSet:        nil,
			machineDeployment: nil,
			wantOutdated:      false,
			wantErr:           false,
		},
		{
			name:              "Stand-alone machine set",
			machineSet:        testMachineSet,
			machineDeployment: nil,
			wantOutdated:      false,
			wantErr:           false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOutdated, err := shouldNodeHaveOutdatedTaint(tt.machineSet, tt.machineDeployment)
			if (err != nil) != tt.wantErr {
				t.Errorf("shouldNodeHaveOutdatedTaint() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotOutdated != tt.wantOutdated {
				t.Errorf("shouldNodeHaveOutdatedTaint() = %v, want %v", gotOutdated, tt.wantOutdated)
			}
		})
	}
}
