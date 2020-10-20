/*
Copyright 2020 The Kubernetes Authors.

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
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const defaultNamespaceName = "default"

func TestMachineHealthCheck_Reconcile(t *testing.T) {
	t.Run("it should ensure the correct cluster-name label when no existing labels exist", func(t *testing.T) {
		g := NewWithT(t)
		cluster := createNamespaceAndCluster(g)

		mhc := newMachineHealthCheck(cluster.Namespace, cluster.Name)
		mhc.Labels = map[string]string{}

		g.Expect(testEnv.Create(ctx, mhc)).To(Succeed())
		defer func(do ...client.Object) {
			g.Expect(testEnv.Cleanup(ctx, do...)).To(Succeed())
		}(cluster, mhc)

		g.Eventually(func() map[string]string {
			err := testEnv.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				return nil
			}
			return mhc.GetLabels()
		}).Should(HaveKeyWithValue(clusterv1.ClusterLabelName, cluster.Name))
	})

	t.Run("it should ensure the correct cluster-name label when the label has the wrong value", func(t *testing.T) {
		g := NewWithT(t)
		cluster := createNamespaceAndCluster(g)

		mhc := newMachineHealthCheck(cluster.Namespace, cluster.Name)
		mhc.Labels = map[string]string{
			clusterv1.ClusterLabelName: "wrong-cluster",
		}

		g.Expect(testEnv.Create(ctx, mhc)).To(Succeed())
		defer func(do ...client.Object) {
			g.Expect(testEnv.Cleanup(ctx, do...)).To(Succeed())
		}(cluster, mhc)

		g.Eventually(func() map[string]string {
			err := testEnv.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				return nil
			}
			return mhc.GetLabels()
		}).Should(HaveKeyWithValue(clusterv1.ClusterLabelName, cluster.Name))
	})

	t.Run("it should ensure the correct cluster-name label when other labels are present", func(t *testing.T) {
		g := NewWithT(t)
		cluster := createNamespaceAndCluster(g)

		mhc := newMachineHealthCheck(cluster.Namespace, cluster.Name)
		mhc.Labels = map[string]string{
			"extra-label": "1",
		}

		g.Expect(testEnv.Create(ctx, mhc)).To(Succeed())
		defer func(do ...client.Object) {
			g.Expect(testEnv.Cleanup(ctx, do...)).To(Succeed())
		}(cluster, mhc)

		g.Eventually(func() map[string]string {
			err := testEnv.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				return nil
			}
			return mhc.GetLabels()
		}).Should(And(
			HaveKeyWithValue(clusterv1.ClusterLabelName, cluster.Name),
			HaveKeyWithValue("extra-label", "1"),
			HaveLen(2),
		))
	})

	t.Run("it should ensure an owner reference is present when no existing ones exist", func(t *testing.T) {
		g := NewWithT(t)
		cluster := createNamespaceAndCluster(g)

		mhc := newMachineHealthCheck(cluster.Namespace, cluster.Name)
		mhc.OwnerReferences = []metav1.OwnerReference{}

		g.Expect(testEnv.Create(ctx, mhc)).To(Succeed())
		defer func(do ...client.Object) {
			g.Expect(testEnv.Cleanup(ctx, do...)).To(Succeed())
		}(cluster, mhc)

		g.Eventually(func() []metav1.OwnerReference {
			err := testEnv.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				fmt.Printf("error cannot retrieve mhc in ctx: %v", err)
				return nil
			}
			return mhc.GetOwnerReferences()
		}, timeout, 100*time.Millisecond).Should(And(
			HaveLen(1),
			ContainElement(ownerReferenceForCluster(ctx, g, cluster)),
		))
	})

	t.Run("it should ensure an owner reference is present when modifying existing ones", func(t *testing.T) {
		g := NewWithT(t)
		cluster := createNamespaceAndCluster(g)

		mhc := newMachineHealthCheck(cluster.Namespace, cluster.Name)
		mhc.OwnerReferences = []metav1.OwnerReference{
			{Kind: "Foo", APIVersion: "foo.bar.baz/v1", Name: "Bar", UID: "12345"},
		}

		g.Expect(testEnv.Create(ctx, mhc)).To(Succeed())
		defer func(do ...client.Object) {
			g.Expect(testEnv.Cleanup(ctx, do...)).To(Succeed())
		}(cluster, mhc)

		g.Eventually(func() []metav1.OwnerReference {
			err := testEnv.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				return nil
			}
			return mhc.GetOwnerReferences()
		}, timeout, 100*time.Millisecond).Should(And(
			ContainElements(
				metav1.OwnerReference{Kind: "Foo", APIVersion: "foo.bar.baz/v1", Name: "Bar", UID: "12345"},
				ownerReferenceForCluster(ctx, g, cluster)),
			HaveLen(2),
		))
	})

	t.Run("it doesn't mark anything unhealthy when all Machines are healthy", func(t *testing.T) {
		g := NewWithT(t)
		cluster := createNamespaceAndCluster(g)

		mhc := newMachineHealthCheck(cluster.Namespace, cluster.Name)

		g.Expect(testEnv.Create(ctx, mhc)).To(Succeed())
		defer func(do ...client.Object) {
			g.Expect(testEnv.Cleanup(ctx, do...)).To(Succeed())
		}(cluster, mhc)

		// Healthy nodes and machines.
		_, machines, cleanup := createMachinesWithNodes(g, cluster,
			count(2),
			createNodeRefForMachine(true),
			markNodeAsHealthy(true),
			machineLabels(mhc.Spec.Selector.MatchLabels),
		)
		defer cleanup()
		targetMachines := make([]string, len(machines))
		for i, m := range machines {
			targetMachines[i] = m.Name
		}
		// Make sure the status matches.
		g.Eventually(func() *clusterv1.MachineHealthCheckStatus {
			err := testEnv.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				return nil
			}
			return &mhc.Status
		}).Should(MatchMachineHealthCheckStatus(&clusterv1.MachineHealthCheckStatus{
			ExpectedMachines:   2,
			CurrentHealthy:     2,
			ObservedGeneration: 1,
			Targets:            targetMachines},
		))
	})

	t.Run("it marks unhealthy machines for remediation when there is one unhealthy Machine", func(t *testing.T) {
		g := NewWithT(t)
		cluster := createNamespaceAndCluster(g)

		mhc := newMachineHealthCheck(cluster.Namespace, cluster.Name)

		g.Expect(testEnv.Create(ctx, mhc)).To(Succeed())
		defer func(do ...client.Object) {
			g.Expect(testEnv.Cleanup(ctx, do...)).To(Succeed())
		}(cluster, mhc)

		// Healthy nodes and machines.
		_, machines, cleanup1 := createMachinesWithNodes(g, cluster,
			count(2),
			createNodeRefForMachine(true),
			markNodeAsHealthy(true),
			machineLabels(mhc.Spec.Selector.MatchLabels),
		)
		defer cleanup1()
		// Unhealthy nodes and machines.
		_, unhealthyMachines, cleanup2 := createMachinesWithNodes(g, cluster,
			count(1),
			createNodeRefForMachine(true),
			markNodeAsHealthy(false),
			machineLabels(mhc.Spec.Selector.MatchLabels),
		)
		defer cleanup2()
		machines = append(machines, unhealthyMachines...)
		targetMachines := make([]string, len(machines))
		for i, m := range machines {
			targetMachines[i] = m.Name
		}

		// Make sure the status matches.
		g.Eventually(func() *clusterv1.MachineHealthCheckStatus {
			err := testEnv.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				return nil
			}
			return &mhc.Status
		}).Should(MatchMachineHealthCheckStatus(&clusterv1.MachineHealthCheckStatus{
			ExpectedMachines:   3,
			CurrentHealthy:     2,
			ObservedGeneration: 1,
			Targets:            targetMachines,
		}))
	})

	t.Run("it marks unhealthy machines for remediation when the unhealthy Machines exceed MaxUnhealthy", func(t *testing.T) {
		g := NewWithT(t)
		cluster := createNamespaceAndCluster(g)

		mhc := newMachineHealthCheck(cluster.Namespace, cluster.Name)
		maxUnhealthy := intstr.Parse("40%")
		mhc.Spec.MaxUnhealthy = &maxUnhealthy

		g.Expect(testEnv.Create(ctx, mhc)).To(Succeed())
		defer func(do ...client.Object) {
			g.Expect(testEnv.Cleanup(ctx, do...)).To(Succeed())
		}(cluster, mhc)

		// Healthy nodes and machines.
		_, machines, cleanup1 := createMachinesWithNodes(g, cluster,
			count(1),
			createNodeRefForMachine(true),
			markNodeAsHealthy(true),
			machineLabels(mhc.Spec.Selector.MatchLabels),
		)
		defer cleanup1()
		// Unhealthy nodes and machines.
		_, unhealthyMachines, cleanup2 := createMachinesWithNodes(g, cluster,
			count(2),
			createNodeRefForMachine(true),
			markNodeAsHealthy(false),
			machineLabels(mhc.Spec.Selector.MatchLabels),
		)
		defer cleanup2()
		machines = append(machines, unhealthyMachines...)
		targetMachines := make([]string, len(machines))
		for i, m := range machines {
			targetMachines[i] = m.Name
		}

		// Make sure the status matches.
		g.Eventually(func() *clusterv1.MachineHealthCheckStatus {
			err := testEnv.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				return nil
			}
			return &mhc.Status
		}).Should(MatchMachineHealthCheckStatus(&clusterv1.MachineHealthCheckStatus{
			ExpectedMachines:   3,
			CurrentHealthy:     1,
			ObservedGeneration: 1,
			Targets:            targetMachines},
		))

		// Calculate how many Machines have health check succeeded = false.
		g.Eventually(func() (unhealthy int) {
			machines := &clusterv1.MachineList{}
			err := testEnv.List(ctx, machines, client.MatchingLabels{
				"selector": mhc.Spec.Selector.MatchLabels["selector"],
			})
			if err != nil {
				return -1
			}

			for i := range machines.Items {
				if conditions.IsFalse(&machines.Items[i], clusterv1.MachineHealthCheckSuccededCondition) {
					unhealthy++
				}
			}
			return
		}).Should(Equal(2))

		// Calculate how many Machines have been remediated.
		g.Eventually(func() (remediated int) {
			machines := &clusterv1.MachineList{}
			err := testEnv.List(ctx, machines, client.MatchingLabels{
				"selector": mhc.Spec.Selector.MatchLabels["selector"],
			})
			if err != nil {
				return -1
			}

			for i := range machines.Items {
				if conditions.Get(&machines.Items[i], clusterv1.MachineOwnerRemediatedCondition) != nil {
					remediated++
				}
			}
			return
		}).Should(Equal(0))
	})

	t.Run("when a Machine has no Node ref for less than the NodeStartupTimeout", func(t *testing.T) {
		g := NewWithT(t)
		cluster := createNamespaceAndCluster(g)

		mhc := newMachineHealthCheck(cluster.Namespace, cluster.Name)
		mhc.Spec.NodeStartupTimeout = &metav1.Duration{Duration: 5 * time.Hour}

		g.Expect(testEnv.Create(ctx, mhc)).To(Succeed())
		defer func(do ...client.Object) {
			g.Expect(testEnv.Cleanup(ctx, do...)).To(Succeed())
		}(cluster, mhc)

		// Healthy nodes and machines.
		_, machines, cleanup1 := createMachinesWithNodes(g, cluster,
			count(2),
			createNodeRefForMachine(true),
			markNodeAsHealthy(true),
			machineLabels(mhc.Spec.Selector.MatchLabels),
		)
		defer cleanup1()
		// Unhealthy nodes and machines.
		_, unhealthyMachines, cleanup2 := createMachinesWithNodes(g, cluster,
			count(1),
			createNodeRefForMachine(false),
			markNodeAsHealthy(false),
			machineLabels(mhc.Spec.Selector.MatchLabels),
		)
		defer cleanup2()
		machines = append(machines, unhealthyMachines...)
		targetMachines := make([]string, len(machines))
		for i, m := range machines {
			targetMachines[i] = m.Name
		}

		// Make sure the status matches.
		g.Eventually(func() *clusterv1.MachineHealthCheckStatus {
			err := testEnv.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				return nil
			}
			return &mhc.Status
		}).Should(MatchMachineHealthCheckStatus(&clusterv1.MachineHealthCheckStatus{
			ExpectedMachines:   3,
			CurrentHealthy:     2,
			ObservedGeneration: 1,
			Targets:            targetMachines},
		))

		// Calculate how many Machines have health check succeeded = false.
		g.Eventually(func() (unhealthy int) {
			machines := &clusterv1.MachineList{}
			err := testEnv.List(ctx, machines, client.MatchingLabels{
				"selector": mhc.Spec.Selector.MatchLabels["selector"],
			})
			if err != nil {
				return -1
			}

			for i := range machines.Items {
				if conditions.IsFalse(&machines.Items[i], clusterv1.MachineHealthCheckSuccededCondition) {
					unhealthy++
				}
			}
			return
		}).Should(Equal(0))

		// Calculate how many Machines have been remediated.
		g.Eventually(func() (remediated int) {
			machines := &clusterv1.MachineList{}
			err := testEnv.List(ctx, machines, client.MatchingLabels{
				"selector": mhc.Spec.Selector.MatchLabels["selector"],
			})
			if err != nil {
				return -1
			}

			for i := range machines.Items {
				if conditions.Get(&machines.Items[i], clusterv1.MachineOwnerRemediatedCondition) != nil {
					remediated++
				}
			}
			return
		}).Should(Equal(0))
	})

	t.Run("when a Machine has no Node ref for longer than the NodeStartupTimeout", func(t *testing.T) {
		// FIXME: Resolve flaky/failing test
		t.Skip("skipping until made stable")
		g := NewWithT(t)
		cluster := createNamespaceAndCluster(g)

		mhc := newMachineHealthCheck(cluster.Namespace, cluster.Name)
		mhc.Spec.NodeStartupTimeout = &metav1.Duration{Duration: time.Second}

		g.Expect(testEnv.Create(ctx, mhc)).To(Succeed())
		defer func(do ...client.Object) {
			g.Expect(testEnv.Cleanup(ctx, do...)).To(Succeed())
		}(cluster, mhc)

		// Healthy nodes and machines.
		_, machines, cleanup1 := createMachinesWithNodes(g, cluster,
			count(2),
			createNodeRefForMachine(true),
			markNodeAsHealthy(true),
			machineLabels(mhc.Spec.Selector.MatchLabels),
		)
		defer cleanup1()
		// Unhealthy nodes and machines.
		_, unhealthyMachines, cleanup2 := createMachinesWithNodes(g, cluster,
			count(1),
			createNodeRefForMachine(false),
			markNodeAsHealthy(false),
			machineLabels(mhc.Spec.Selector.MatchLabels),
		)
		defer cleanup2()
		machines = append(machines, unhealthyMachines...)

		targetMachines := make([]string, len(machines))
		for i, m := range machines {
			targetMachines[i] = m.Name
		}

		// Make sure the MHC status matches. We have two healthy machines and
		// one unhealthy.
		g.Eventually(func() *clusterv1.MachineHealthCheckStatus {
			err := testEnv.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				fmt.Printf("error retrieving mhc: %v", err)
				return nil
			}
			return &mhc.Status
		}, timeout, 100*time.Millisecond).Should(MatchMachineHealthCheckStatus(&clusterv1.MachineHealthCheckStatus{
			ExpectedMachines:   3,
			CurrentHealthy:     2,
			ObservedGeneration: 1,
			Targets:            targetMachines},
		))

		// Calculate how many Machines have health check succeeded = false.
		g.Eventually(func() (unhealthy int) {
			machines := &clusterv1.MachineList{}
			err := testEnv.List(ctx, machines, client.MatchingLabels{
				"selector": mhc.Spec.Selector.MatchLabels["selector"],
			})
			if err != nil {
				fmt.Printf("error retrieving list: %v", err)
				return -1
			}

			for i := range machines.Items {
				if conditions.IsFalse(&machines.Items[i], clusterv1.MachineHealthCheckSuccededCondition) {
					unhealthy++
				}
			}
			return
		}, timeout, 100*time.Millisecond).Should(Equal(1))

		// Calculate how many Machines have been remediated.
		g.Eventually(func() (remediated int) {
			machines := &clusterv1.MachineList{}
			err := testEnv.List(ctx, machines, client.MatchingLabels{
				"selector": mhc.Spec.Selector.MatchLabels["selector"],
			})
			if err != nil {
				return -1
			}

			for i := range machines.Items {
				if conditions.Get(&machines.Items[i], clusterv1.MachineOwnerRemediatedCondition) != nil {
					remediated++
				}
			}
			return
		}, timeout, 100*time.Millisecond).Should(Equal(1))
	})

	t.Run("when a Machine's Node has gone away", func(t *testing.T) {
		// FIXME: Resolve flaky/failing test
		t.Skip("skipping until made stable")
		g := NewWithT(t)
		cluster := createNamespaceAndCluster(g)

		mhc := newMachineHealthCheck(cluster.Namespace, cluster.Name)

		g.Expect(testEnv.Create(ctx, mhc)).To(Succeed())
		defer func(do ...client.Object) {
			g.Expect(testEnv.Cleanup(ctx, do...)).To(Succeed())
		}(cluster, mhc)

		// Healthy nodes and machines.
		nodes, machines, cleanup := createMachinesWithNodes(g, cluster,
			count(3),
			createNodeRefForMachine(true),
			markNodeAsHealthy(true),
			machineLabels(mhc.Spec.Selector.MatchLabels),
		)
		defer cleanup()
		targetMachines := make([]string, len(machines))
		for i, m := range machines {
			targetMachines[i] = m.Name
		}

		// Forcibly remove the last machine's node.
		g.Eventually(func() bool {
			nodeToBeRemoved := nodes[2]
			if err := testEnv.Delete(ctx, nodeToBeRemoved); err != nil {
				return apierrors.IsNotFound(err)
			}
			return apierrors.IsNotFound(testEnv.Get(ctx, util.ObjectKey(nodeToBeRemoved), nodeToBeRemoved))
		}).Should(BeTrue())

		// Make sure the status matches.
		g.Eventually(func() *clusterv1.MachineHealthCheckStatus {
			err := testEnv.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				return nil
			}
			return &mhc.Status
		}).Should(MatchMachineHealthCheckStatus(&clusterv1.MachineHealthCheckStatus{
			ExpectedMachines:   3,
			CurrentHealthy:     2,
			ObservedGeneration: 1,
			Targets:            targetMachines},
		))

		// Calculate how many Machines have health check succeeded = false.
		g.Eventually(func() (unhealthy int) {
			machines := &clusterv1.MachineList{}
			err := testEnv.List(ctx, machines, client.MatchingLabels{
				"selector": mhc.Spec.Selector.MatchLabels["selector"],
			})
			if err != nil {
				return -1
			}

			for i := range machines.Items {
				if conditions.IsFalse(&machines.Items[i], clusterv1.MachineHealthCheckSuccededCondition) {
					unhealthy++
				}
			}
			return
		}).Should(Equal(1))

		// Calculate how many Machines have been remediated.
		g.Eventually(func() (remediated int) {
			machines := &clusterv1.MachineList{}
			err := testEnv.List(ctx, machines, client.MatchingLabels{
				"selector": mhc.Spec.Selector.MatchLabels["selector"],
			})
			if err != nil {
				return -1
			}

			for i := range machines.Items {
				if conditions.Get(&machines.Items[i], clusterv1.MachineOwnerRemediatedCondition) != nil {
					remediated++
				}
			}
			return
		}, timeout, 100*time.Millisecond).Should(Equal(1))
	})

	t.Run("should react when a Node transitions to unhealthy", func(t *testing.T) {
		g := NewWithT(t)
		cluster := createNamespaceAndCluster(g)

		mhc := newMachineHealthCheck(cluster.Namespace, cluster.Name)

		g.Expect(testEnv.Create(ctx, mhc)).To(Succeed())
		defer func(do ...client.Object) {
			g.Expect(testEnv.Cleanup(ctx, do...)).To(Succeed())
		}(cluster, mhc)

		// Healthy nodes and machines.
		nodes, machines, cleanup := createMachinesWithNodes(g, cluster,
			count(1),
			createNodeRefForMachine(true),
			markNodeAsHealthy(true),
			machineLabels(mhc.Spec.Selector.MatchLabels),
		)
		defer cleanup()
		targetMachines := make([]string, len(machines))
		for i, m := range machines {
			targetMachines[i] = m.Name
		}

		// Make sure the status matches.
		g.Eventually(func() *clusterv1.MachineHealthCheckStatus {
			err := testEnv.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				return nil
			}
			return &mhc.Status
		}).Should(MatchMachineHealthCheckStatus(&clusterv1.MachineHealthCheckStatus{
			ExpectedMachines:   1,
			CurrentHealthy:     1,
			ObservedGeneration: 1,
			Targets:            targetMachines},
		))

		// Transition the node to unhealthy.
		node := nodes[0]
		nodePatch := client.MergeFrom(node.DeepCopy())
		node.Status.Conditions = []corev1.NodeCondition{
			{
				Type:               corev1.NodeReady,
				Status:             corev1.ConditionUnknown,
				LastTransitionTime: metav1.NewTime(time.Now().Add(-10 * time.Minute)),
			},
		}
		g.Expect(testEnv.Status().Patch(ctx, node, nodePatch)).To(Succeed())

		// Make sure the status matches.
		g.Eventually(func() *clusterv1.MachineHealthCheckStatus {
			err := testEnv.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				return nil
			}
			return &mhc.Status
		}).Should(MatchMachineHealthCheckStatus(&clusterv1.MachineHealthCheckStatus{
			ExpectedMachines:   1,
			CurrentHealthy:     0,
			ObservedGeneration: 1,
			Targets:            targetMachines},
		))

		// Calculate how many Machines have health check succeeded = false.
		g.Eventually(func() (unhealthy int) {
			machines := &clusterv1.MachineList{}
			err := testEnv.List(ctx, machines, client.MatchingLabels{
				"selector": mhc.Spec.Selector.MatchLabels["selector"],
			})
			if err != nil {
				return -1
			}

			for i := range machines.Items {
				if conditions.IsFalse(&machines.Items[i], clusterv1.MachineHealthCheckSuccededCondition) {
					unhealthy++
				}
			}
			return
		}).Should(Equal(1))

		// Calculate how many Machines have been remediated.
		g.Eventually(func() (remediated int) {
			machines := &clusterv1.MachineList{}
			err := testEnv.List(ctx, machines, client.MatchingLabels{
				"selector": mhc.Spec.Selector.MatchLabels["selector"],
			})
			if err != nil {
				return -1
			}

			for i := range machines.Items {
				if conditions.Get(&machines.Items[i], clusterv1.MachineOwnerRemediatedCondition) != nil {
					remediated++
				}
			}
			return
		}).Should(Equal(1))
	})

	t.Run("when in a MachineSet, unhealthy machines should be deleted", func(t *testing.T) {
		g := NewWithT(t)
		cluster := createNamespaceAndCluster(g)

		mhc := newMachineHealthCheck(cluster.Namespace, cluster.Name)
		// Create infrastructure template resource.
		infraResource := map[string]interface{}{
			"kind":       "InfrastructureMachine",
			"apiVersion": "infrastructure.cluster.x-k8s.io/v1alpha3",
			"metadata":   map[string]interface{}{},
			"spec": map[string]interface{}{
				"size": "3xlarge",
			},
		}
		infraTmpl := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": infraResource,
				},
			},
		}
		infraTmpl.SetKind("InfrastructureMachineTemplate")
		infraTmpl.SetAPIVersion("infrastructure.cluster.x-k8s.io/v1alpha3")
		infraTmpl.SetGenerateName("mhc-ms-template-")
		infraTmpl.SetNamespace(mhc.Namespace)

		g.Expect(testEnv.Create(ctx, infraTmpl)).To(Succeed())

		machineSet := &clusterv1.MachineSet{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "mhc-ms-",
				Namespace:    mhc.Namespace,
			},
			Spec: clusterv1.MachineSetSpec{
				ClusterName: cluster.Name,
				Replicas:    pointer.Int32Ptr(1),
				Selector:    mhc.Spec.Selector,
				Template: clusterv1.MachineTemplateSpec{
					ObjectMeta: clusterv1.ObjectMeta{
						Labels: mhc.Spec.Selector.MatchLabels,
					},
					Spec: clusterv1.MachineSpec{
						ClusterName: cluster.Name,
						Bootstrap: clusterv1.Bootstrap{
							DataSecretName: pointer.StringPtr("test-data-secret-name"),
						},
						InfrastructureRef: corev1.ObjectReference{
							APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha3",
							Kind:       "InfrastructureMachineTemplate",
							Name:       infraTmpl.GetName(),
						},
					},
				},
			},
		}
		machineSet.Default()
		g.Expect(testEnv.Create(ctx, machineSet)).To(Succeed())

		// Ensure machines have been created.
		g.Eventually(func() int {
			machines := &clusterv1.MachineList{}
			err := testEnv.List(ctx, machines, client.MatchingLabels{
				"selector": mhc.Spec.Selector.MatchLabels["selector"],
			})
			if err != nil {
				return -1
			}
			return len(machines.Items)
		}, timeout, 100*time.Millisecond).Should(Equal(1))

		// Create the MachineHealthCheck instance.
		mhc.Spec.NodeStartupTimeout = &metav1.Duration{Duration: time.Second}

		g.Expect(testEnv.Create(ctx, mhc)).To(Succeed())
		// defer cleanup for all the objects that have been created
		defer func(do ...client.Object) {
			g.Expect(testEnv.Cleanup(ctx, do...)).To(Succeed())
		}(cluster, mhc, infraTmpl, machineSet)

		// Pause the MachineSet reconciler to delay the deletion of the
		// Machine, because the MachineSet controller deletes the Machine when
		// it is marked unhealthy by MHC.
		machineSetPatch := client.MergeFrom(machineSet.DeepCopy())
		machineSet.Annotations = map[string]string{
			clusterv1.PausedAnnotation: "",
		}
		g.Expect(testEnv.Patch(ctx, machineSet, machineSetPatch)).To(Succeed())

		// Calculate how many Machines have health check succeeded = false.
		g.Eventually(func() (unhealthy int) {
			machines := &clusterv1.MachineList{}
			err := testEnv.List(ctx, machines, client.MatchingLabels{
				"selector": mhc.Spec.Selector.MatchLabels["selector"],
			})
			if err != nil {
				return -1
			}

			for i := range machines.Items {
				if conditions.IsFalse(&machines.Items[i], clusterv1.MachineHealthCheckSuccededCondition) {
					unhealthy++
				}
			}
			return
		}, timeout, 100*time.Millisecond).Should(Equal(1))

		// Calculate how many Machines should be remediated.
		var unhealthyMachine *clusterv1.Machine
		g.Eventually(func() (remediated int) {
			machines := &clusterv1.MachineList{}
			err := testEnv.List(ctx, machines, client.MatchingLabels{
				"selector": mhc.Spec.Selector.MatchLabels["selector"],
			})
			if err != nil {
				return -1
			}

			for i := range machines.Items {
				if conditions.Get(&machines.Items[i], clusterv1.MachineOwnerRemediatedCondition) != nil {
					unhealthyMachine = machines.Items[i].DeepCopy()
					remediated++
				}
			}
			return
		}, timeout, 100*time.Millisecond).Should(Equal(1))

		// Unpause the MachineSet reconciler.
		machineSetPatch = client.MergeFrom(machineSet.DeepCopy())
		delete(machineSet.Annotations, clusterv1.PausedAnnotation)
		g.Expect(testEnv.Patch(ctx, machineSet, machineSetPatch)).To(Succeed())

		// Make sure the Machine gets deleted.
		g.Eventually(func() bool {
			machine := unhealthyMachine.DeepCopy()
			err := testEnv.Get(ctx, util.ObjectKey(unhealthyMachine), machine)
			return apierrors.IsNotFound(err) || !machine.DeletionTimestamp.IsZero()
		}, timeout, 100*time.Millisecond)
	})

	t.Run("when a machine is paused", func(t *testing.T) {
		// FIXME: Resolve flaky/failing test
		t.Skip("skipping until made stable")
		g := NewWithT(t)
		cluster := createNamespaceAndCluster(g)

		mhc := newMachineHealthCheck(cluster.Namespace, cluster.Name)

		g.Expect(testEnv.Create(ctx, mhc)).To(Succeed())
		defer func(do ...client.Object) {
			g.Expect(testEnv.Cleanup(ctx, do...)).To(Succeed())
		}(cluster, mhc)

		// Healthy nodes and machines.
		nodes, machines, cleanup := createMachinesWithNodes(g, cluster,
			count(1),
			createNodeRefForMachine(true),
			markNodeAsHealthy(true),
			machineLabels(mhc.Spec.Selector.MatchLabels),
		)
		defer cleanup()
		targetMachines := make([]string, len(machines))
		for i, m := range machines {
			targetMachines[i] = m.Name
		}

		// Make sure the status matches.
		g.Eventually(func() *clusterv1.MachineHealthCheckStatus {
			err := testEnv.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				return nil
			}
			return &mhc.Status
		}).Should(MatchMachineHealthCheckStatus(&clusterv1.MachineHealthCheckStatus{
			ExpectedMachines:   1,
			CurrentHealthy:     1,
			ObservedGeneration: 1,
			Targets:            targetMachines},
		))

		// Pause the machine
		machinePatch := client.MergeFrom(machines[0].DeepCopy())
		machines[0].Annotations = map[string]string{
			clusterv1.PausedAnnotation: "",
		}
		g.Expect(testEnv.Patch(ctx, machines[0], machinePatch)).To(Succeed())

		// Transition the node to unhealthy.
		node := nodes[0]
		nodePatch := client.MergeFrom(node.DeepCopy())
		node.Status.Conditions = []corev1.NodeCondition{
			{
				Type:               corev1.NodeReady,
				Status:             corev1.ConditionUnknown,
				LastTransitionTime: metav1.NewTime(time.Now().Add(-10 * time.Minute)),
			},
		}
		g.Expect(testEnv.Status().Patch(ctx, node, nodePatch)).To(Succeed())

		// Make sure the status matches.
		g.Eventually(func() *clusterv1.MachineHealthCheckStatus {
			err := testEnv.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				return nil
			}
			return &mhc.Status
		}).Should(MatchMachineHealthCheckStatus(&clusterv1.MachineHealthCheckStatus{
			ExpectedMachines:   1,
			CurrentHealthy:     0,
			ObservedGeneration: 1,
			Targets:            targetMachines},
		))

		// Calculate how many Machines have health check succeeded = false.
		g.Eventually(func() (unhealthy int) {
			machines := &clusterv1.MachineList{}
			err := testEnv.List(ctx, machines, client.MatchingLabels{
				"selector": mhc.Spec.Selector.MatchLabels["selector"],
			})
			if err != nil {
				return -1
			}

			for i := range machines.Items {
				if conditions.IsFalse(&machines.Items[i], clusterv1.MachineHealthCheckSuccededCondition) {
					unhealthy++
				}
			}
			return
		}).Should(Equal(1))

		// Calculate how many Machines have been remediated.
		g.Eventually(func() (remediated int) {
			machines := &clusterv1.MachineList{}
			err := testEnv.List(ctx, machines, client.MatchingLabels{
				"selector": mhc.Spec.Selector.MatchLabels["selector"],
			})
			if err != nil {
				return -1
			}

			for i := range machines.Items {
				if conditions.Get(&machines.Items[i], clusterv1.MachineOwnerRemediatedCondition) != nil {
					remediated++
				}
			}
			return
		}).Should(Equal(0))
	})
}

func TestClusterToMachineHealthCheck(t *testing.T) {
	_ = clusterv1.AddToScheme(scheme.Scheme)
	fakeClient := fake.NewFakeClient()

	r := &MachineHealthCheckReconciler{
		Client: fakeClient,
	}

	namespace := defaultNamespaceName
	clusterName := "test-cluster"
	labels := make(map[string]string)

	mhc1 := newMachineHealthCheckWithLabels("mhc1", namespace, clusterName, labels)
	mhc1Req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: mhc1.Namespace, Name: mhc1.Name}}
	mhc2 := newMachineHealthCheckWithLabels("mhc2", namespace, clusterName, labels)
	mhc2Req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: mhc2.Namespace, Name: mhc2.Name}}
	mhc3 := newMachineHealthCheckWithLabels("mhc3", namespace, "othercluster", labels)
	mhc4 := newMachineHealthCheckWithLabels("mhc4", "othernamespace", clusterName, labels)
	cluster1 := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName,
			Namespace: namespace,
		},
	}

	testCases := []struct {
		name     string
		toCreate []clusterv1.MachineHealthCheck
		object   client.Object
		expected []reconcile.Request
	}{
		{
			name:     "when a MachineHealthCheck exists for the Cluster in the same namespace",
			toCreate: []clusterv1.MachineHealthCheck{*mhc1},
			object:   cluster1,
			expected: []reconcile.Request{mhc1Req},
		},
		{
			name:     "when 2 MachineHealthChecks exists for the Cluster in the same namespace",
			toCreate: []clusterv1.MachineHealthCheck{*mhc1, *mhc2},
			object:   cluster1,
			expected: []reconcile.Request{mhc1Req, mhc2Req},
		},
		{
			name:     "when a MachineHealthCheck exists for another Cluster in the same namespace",
			toCreate: []clusterv1.MachineHealthCheck{*mhc3},
			object:   cluster1,
			expected: []reconcile.Request{},
		},
		{
			name:     "when a MachineHealthCheck exists for another Cluster in another namespace",
			toCreate: []clusterv1.MachineHealthCheck{*mhc4},
			object:   cluster1,
			expected: []reconcile.Request{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gs := NewWithT(t)

			for _, obj := range tc.toCreate {
				o := obj
				gs.Expect(r.Client.Create(ctx, &o)).To(Succeed())
				defer func() {
					gs.Expect(r.Client.Delete(ctx, &o)).To(Succeed())
				}()
				// Check the cache is populated
				getObj := func() error {
					return r.Client.Get(ctx, util.ObjectKey(&o), &clusterv1.MachineHealthCheck{})
				}
				gs.Eventually(getObj).Should(Succeed())
			}

			got := r.clusterToMachineHealthCheck(tc.object)
			gs.Expect(got).To(ConsistOf(tc.expected))
		})
	}
}

func TestMachineToMachineHealthCheck(t *testing.T) {
	_ = clusterv1.AddToScheme(scheme.Scheme)
	fakeClient := fake.NewFakeClient()

	r := &MachineHealthCheckReconciler{
		Client: fakeClient,
	}

	namespace := defaultNamespaceName
	clusterName := "test-cluster"
	nodeName := "node1"
	labels := map[string]string{"cluster": "foo", "nodepool": "bar"}

	mhc1 := newMachineHealthCheckWithLabels("mhc1", namespace, clusterName, labels)
	mhc1Req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: mhc1.Namespace, Name: mhc1.Name}}
	mhc2 := newMachineHealthCheckWithLabels("mhc2", namespace, clusterName, labels)
	mhc2Req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: mhc2.Namespace, Name: mhc2.Name}}
	mhc3 := newMachineHealthCheckWithLabels("mhc3", namespace, clusterName, map[string]string{"cluster": "foo", "nodepool": "other"})
	mhc4 := newMachineHealthCheckWithLabels("mhc4", "othernamespace", clusterName, labels)
	machine1 := newTestMachine("machine1", namespace, clusterName, nodeName, labels)

	testCases := []struct {
		name     string
		toCreate []clusterv1.MachineHealthCheck
		object   client.Object
		expected []reconcile.Request
	}{
		{
			name:     "when a MachineHealthCheck matches labels for the Machine in the same namespace",
			toCreate: []clusterv1.MachineHealthCheck{*mhc1},
			object:   machine1,
			expected: []reconcile.Request{mhc1Req},
		},
		{
			name:     "when 2 MachineHealthChecks match labels for the Machine in the same namespace",
			toCreate: []clusterv1.MachineHealthCheck{*mhc1, *mhc2},
			object:   machine1,
			expected: []reconcile.Request{mhc1Req, mhc2Req},
		},
		{
			name:     "when a MachineHealthCheck does not match labels for the Machine in the same namespace",
			toCreate: []clusterv1.MachineHealthCheck{*mhc3},
			object:   machine1,
			expected: []reconcile.Request{},
		},
		{
			name:     "when a MachineHealthCheck matches labels for the Machine in another namespace",
			toCreate: []clusterv1.MachineHealthCheck{*mhc4},
			object:   machine1,
			expected: []reconcile.Request{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gs := NewWithT(t)

			for _, obj := range tc.toCreate {
				o := obj
				gs.Expect(r.Client.Create(ctx, &o)).To(Succeed())
				defer func() {
					gs.Expect(r.Client.Delete(ctx, &o)).To(Succeed())
				}()
				// Check the cache is populated
				getObj := func() error {
					return r.Client.Get(ctx, util.ObjectKey(&o), &clusterv1.MachineHealthCheck{})
				}
				gs.Eventually(getObj).Should(Succeed())
			}

			got := r.machineToMachineHealthCheck(tc.object)
			gs.Expect(got).To(ConsistOf(tc.expected))
		})
	}
}

func TestNodeToMachineHealthCheck(t *testing.T) {
	_ = clusterv1.AddToScheme(scheme.Scheme)
	fakeClient := fake.NewFakeClient()

	r := &MachineHealthCheckReconciler{
		Client: fakeClient,
	}

	namespace := defaultNamespaceName
	clusterName := "test-cluster"
	nodeName := "node1"
	labels := map[string]string{"cluster": "foo", "nodepool": "bar"}

	mhc1 := newMachineHealthCheckWithLabels("mhc1", namespace, clusterName, labels)
	mhc1Req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: mhc1.Namespace, Name: mhc1.Name}}
	mhc2 := newMachineHealthCheckWithLabels("mhc2", namespace, clusterName, labels)
	mhc2Req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: mhc2.Namespace, Name: mhc2.Name}}
	mhc3 := newMachineHealthCheckWithLabels("mhc3", namespace, "othercluster", labels)
	mhc4 := newMachineHealthCheckWithLabels("mhc4", "othernamespace", clusterName, labels)

	machine1 := newTestMachine("machine1", namespace, clusterName, nodeName, labels)
	machine2 := newTestMachine("machine2", namespace, clusterName, nodeName, labels)

	node1 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
		},
	}

	testCases := []struct {
		name        string
		mhcToCreate []clusterv1.MachineHealthCheck
		mToCreate   []clusterv1.Machine
		object      client.Object
		expected    []reconcile.Request
	}{
		{
			name:        "when no Machine exists for the Node",
			mhcToCreate: []clusterv1.MachineHealthCheck{*mhc1},
			mToCreate:   []clusterv1.Machine{},
			object:      node1,
			expected:    []reconcile.Request{},
		},
		{
			name:        "when two Machines exist for the Node",
			mhcToCreate: []clusterv1.MachineHealthCheck{*mhc1},
			mToCreate:   []clusterv1.Machine{*machine1, *machine2},
			object:      node1,
			expected:    []reconcile.Request{},
		},
		{
			name:        "when no MachineHealthCheck exists for the Node in the Machine's namespace",
			mhcToCreate: []clusterv1.MachineHealthCheck{*mhc4},
			mToCreate:   []clusterv1.Machine{*machine1},
			object:      node1,
			expected:    []reconcile.Request{},
		},
		{
			name:        "when a MachineHealthCheck exists for the Node in the Machine's namespace",
			mhcToCreate: []clusterv1.MachineHealthCheck{*mhc1},
			mToCreate:   []clusterv1.Machine{*machine1},
			object:      node1,
			expected:    []reconcile.Request{mhc1Req},
		},
		{
			name:        "when two MachineHealthChecks exist for the Node in the Machine's namespace",
			mhcToCreate: []clusterv1.MachineHealthCheck{*mhc1, *mhc2},
			mToCreate:   []clusterv1.Machine{*machine1},
			object:      node1,
			expected:    []reconcile.Request{mhc1Req, mhc2Req},
		},
		{
			name:        "when a MachineHealthCheck exists for the Node, but not in the Machine's cluster",
			mhcToCreate: []clusterv1.MachineHealthCheck{*mhc3},
			mToCreate:   []clusterv1.Machine{*machine1},
			object:      node1,
			expected:    []reconcile.Request{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gs := NewWithT(t)

			for _, obj := range tc.mhcToCreate {
				o := obj
				gs.Expect(r.Client.Create(ctx, &o)).To(Succeed())
				defer func() {
					gs.Expect(r.Client.Delete(ctx, &o)).To(Succeed())
				}()
				// Check the cache is populated
				key := util.ObjectKey(&o)
				getObj := func() error {
					return r.Client.Get(ctx, key, &clusterv1.MachineHealthCheck{})
				}
				gs.Eventually(getObj).Should(Succeed())
			}
			for _, obj := range tc.mToCreate {
				o := obj
				gs.Expect(r.Client.Create(ctx, &o)).To(Succeed())
				defer func() {
					gs.Expect(r.Client.Delete(ctx, &o)).To(Succeed())
				}()
				// Ensure the status is set (required for matching node to machine)
				o.Status = obj.Status
				gs.Expect(r.Client.Status().Update(ctx, &o)).To(Succeed())

				// Check the cache is up to date with the status update
				key := util.ObjectKey(&o)
				checkStatus := func() clusterv1.MachineStatus {
					m := &clusterv1.Machine{}
					err := r.Client.Get(ctx, key, m)
					if err != nil {
						return clusterv1.MachineStatus{}
					}
					return m.Status
				}
				gs.Eventually(checkStatus).Should(Equal(o.Status))
			}

			got := r.nodeToMachineHealthCheck(tc.object)
			gs.Expect(got).To(ConsistOf(tc.expected))
		})
	}
}

func TestIsAllowedRemediation(t *testing.T) {
	testCases := []struct {
		name               string
		maxUnhealthy       *intstr.IntOrString
		expectedMachines   int32
		currentHealthy     int32
		allowed            bool
		observedGeneration int64
	}{
		{
			name:             "when maxUnhealthy is not set",
			maxUnhealthy:     nil,
			expectedMachines: int32(3),
			currentHealthy:   int32(0),
			allowed:          true,
		},
		{
			name:             "when maxUnhealthy is not an int or percentage",
			maxUnhealthy:     &intstr.IntOrString{Type: intstr.String, StrVal: "abcdef"},
			expectedMachines: int32(5),
			currentHealthy:   int32(2),
			allowed:          false,
		},
		{
			name:             "when maxUnhealthy is an int less than current unhealthy",
			maxUnhealthy:     &intstr.IntOrString{Type: intstr.Int, IntVal: int32(1)},
			expectedMachines: int32(3),
			currentHealthy:   int32(1),
			allowed:          false,
		},
		{
			name:             "when maxUnhealthy is an int equal to current unhealthy",
			maxUnhealthy:     &intstr.IntOrString{Type: intstr.Int, IntVal: int32(2)},
			expectedMachines: int32(3),
			currentHealthy:   int32(1),
			allowed:          true,
		},
		{
			name:             "when maxUnhealthy is an int greater than current unhealthy",
			maxUnhealthy:     &intstr.IntOrString{Type: intstr.Int, IntVal: int32(3)},
			expectedMachines: int32(3),
			currentHealthy:   int32(1),
			allowed:          true,
		},
		{
			name:             "when maxUnhealthy is a percentage less than current unhealthy",
			maxUnhealthy:     &intstr.IntOrString{Type: intstr.String, StrVal: "50%"},
			expectedMachines: int32(5),
			currentHealthy:   int32(2),
			allowed:          false,
		},
		{
			name:             "when maxUnhealthy is a percentage equal to current unhealthy",
			maxUnhealthy:     &intstr.IntOrString{Type: intstr.String, StrVal: "60%"},
			expectedMachines: int32(5),
			currentHealthy:   int32(2),
			allowed:          true,
		},
		{
			name:             "when maxUnhealthy is a percentage greater than current unhealthy",
			maxUnhealthy:     &intstr.IntOrString{Type: intstr.String, StrVal: "70%"},
			expectedMachines: int32(5),
			currentHealthy:   int32(2),
			allowed:          true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			mhc := &clusterv1.MachineHealthCheck{
				Spec: clusterv1.MachineHealthCheckSpec{
					MaxUnhealthy:       tc.maxUnhealthy,
					NodeStartupTimeout: &metav1.Duration{Duration: 1 * time.Millisecond},
				},
				Status: clusterv1.MachineHealthCheckStatus{
					ExpectedMachines:   tc.expectedMachines,
					CurrentHealthy:     tc.currentHealthy,
					ObservedGeneration: tc.observedGeneration,
				},
			}

			g.Expect(isAllowedRemediation(mhc)).To(Equal(tc.allowed))
		})
	}
}

func ownerReferenceForCluster(ctx context.Context, g *WithT, c *clusterv1.Cluster) metav1.OwnerReference {
	// Fetch the cluster to populate the UID
	cc := &clusterv1.Cluster{}
	g.Expect(testEnv.GetClient().Get(ctx, util.ObjectKey(c), cc)).To(Succeed())

	return metav1.OwnerReference{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Cluster",
		Name:       cc.Name,
		UID:        cc.UID,
	}
}

// createNamespaceAndCluster creates a namespace in the test environment. It
// then creates a Cluster and KubeconfigSecret for that cluster in said
// namespace.
func createNamespaceAndCluster(g *WithT) *clusterv1.Cluster {
	ns, err := testEnv.CreateNamespace(ctx, "test-mhc")
	g.Expect(err).ToNot(HaveOccurred())
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "test-cluster-",
			Namespace:    ns.Name,
		},
	}
	g.Expect(testEnv.Create(ctx, cluster)).To(Succeed())
	g.Eventually(func() error {
		var cl clusterv1.Cluster
		return testEnv.Get(ctx, util.ObjectKey(cluster), &cl)
	}, timeout, 100*time.Millisecond).Should(Succeed())

	g.Expect(testEnv.CreateKubeconfigSecret(ctx, cluster)).To(Succeed())

	return cluster
}

// newRunningMachine creates a Machine object with a Status.Phase == Running.
func newRunningMachine(c *clusterv1.Cluster, labels map[string]string) *clusterv1.Machine {
	return &clusterv1.Machine{
		TypeMeta: metav1.TypeMeta{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "test-mhc-machine-",
			Namespace:    c.Namespace,
			Labels:       labels,
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: c.Name,
			Bootstrap: clusterv1.Bootstrap{
				DataSecretName: pointer.StringPtr("data-secret-name"),
			},
		},
		Status: clusterv1.MachineStatus{
			InfrastructureReady: true,
			BootstrapReady:      true,
			Phase:               string(clusterv1.MachinePhaseRunning),
			ObservedGeneration:  1,
		},
	}
}

// newNode creaetes a Node object with node condition Ready == True
func newNode() *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "test-mhc-node-",
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
		},
	}
}

func setNodeUnhealthy(node *corev1.Node) {
	node.Status.Conditions[0] = corev1.NodeCondition{
		Type:               corev1.NodeReady,
		Status:             corev1.ConditionUnknown,
		LastTransitionTime: metav1.NewTime(time.Now().Add(-10 * time.Minute)),
	}
}

func newInfraMachine(machine *clusterv1.Machine) (*unstructured.Unstructured, string) {
	providerID := fmt.Sprintf("test:////%v", uuid.NewUUID())
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "infrastructure.cluster.x-k8s.io/v1alpha3",
			"kind":       "InfrastructureMachine",
			"metadata": map[string]interface{}{
				"generateName": "test-mhc-machine-infra-",
				"namespace":    machine.Namespace,
			},
			"spec": map[string]interface{}{
				"providerID": providerID,
			},
		},
	}, providerID
}

type machinesWithNodes struct {
	count                   int
	markNodeAsHealthy       bool
	createNodeRefForMachine bool
	labels                  map[string]string
}

type machineWithNodesOption func(m *machinesWithNodes)

func count(n int) machineWithNodesOption {
	return func(m *machinesWithNodes) {
		m.count = n
	}
}

func markNodeAsHealthy(b bool) machineWithNodesOption {
	return func(m *machinesWithNodes) {
		m.markNodeAsHealthy = b
	}
}

func createNodeRefForMachine(b bool) machineWithNodesOption {
	return func(m *machinesWithNodes) {
		m.createNodeRefForMachine = b
	}
}

func machineLabels(l map[string]string) machineWithNodesOption {
	return func(m *machinesWithNodes) {
		m.labels = l
	}
}

func createMachinesWithNodes(
	g *WithT,
	c *clusterv1.Cluster,
	opts ...machineWithNodesOption,
) ([]*corev1.Node, []*clusterv1.Machine, func()) {

	o := &machinesWithNodes{}
	for _, op := range opts {
		op(o)
	}

	var (
		nodes         []*corev1.Node
		machines      []*clusterv1.Machine
		infraMachines []*unstructured.Unstructured
	)

	for i := 0; i < o.count; i++ {
		machine := newRunningMachine(c, o.labels)
		infraMachine, providerID := newInfraMachine(machine)
		g.Expect(testEnv.Create(ctx, infraMachine)).To(Succeed())
		infraMachines = append(infraMachines, infraMachine)
		fmt.Printf("inframachine created: %s\n", infraMachine.GetName())
		// Patch the status of the InfraMachine and mark it as ready.
		// NB. Status cannot be set during object creation so we need to patch
		// it separately.
		infraMachinePatch := client.MergeFrom(infraMachine.DeepCopy())
		g.Expect(unstructured.SetNestedField(infraMachine.Object, true, "status", "ready")).To(Succeed())
		g.Expect(testEnv.Status().Patch(ctx, infraMachine, infraMachinePatch)).To(Succeed())

		machine.Spec.InfrastructureRef = corev1.ObjectReference{
			APIVersion: infraMachine.GetAPIVersion(),
			Kind:       infraMachine.GetKind(),
			Name:       infraMachine.GetName(),
		}
		g.Expect(testEnv.Create(ctx, machine)).To(Succeed())
		fmt.Printf("machine created: %s\n", machine.GetName())

		// Before moving on we want to ensure that the machine has a valid
		// status. That is, LastUpdated should not be nil.
		g.Eventually(func() *metav1.Time {
			k := client.ObjectKey{
				Name:      machine.GetName(),
				Namespace: machine.GetNamespace(),
			}
			err := testEnv.Get(ctx, k, machine)
			if err != nil {
				return nil
			}
			return machine.Status.LastUpdated
		}, timeout, 100*time.Millisecond).ShouldNot(BeNil())

		if o.createNodeRefForMachine {
			node := newNode()
			if !o.markNodeAsHealthy {
				setNodeUnhealthy(node)
			}
			machineStatus := machine.Status
			node.Spec.ProviderID = providerID
			nodeStatus := node.Status
			g.Expect(testEnv.Create(ctx, node)).To(Succeed())
			fmt.Printf("node created: %s\n", node.GetName())

			nodePatch := client.MergeFrom(node.DeepCopy())
			node.Status = nodeStatus
			g.Expect(testEnv.Status().Patch(ctx, node, nodePatch)).To(Succeed())
			nodes = append(nodes, node)

			machinePatch := client.MergeFrom(machine.DeepCopy())
			machine.Status = machineStatus
			machine.Status.NodeRef = &corev1.ObjectReference{
				Name: node.Name,
			}

			// Adding one second to ensure there is a difference from the
			// original time so that the patch works. That is, ensure the
			// precision isn't lost during conversions.
			lastUp := metav1.NewTime(machine.Status.LastUpdated.Add(time.Second))
			machine.Status.LastUpdated = &lastUp
			g.Expect(testEnv.Status().Patch(ctx, machine, machinePatch)).To(Succeed())
		}

		machines = append(machines, machine)
	}

	cleanup := func() {
		fmt.Println("Cleaning up nodes, machines and infra machines.")
		for _, n := range nodes {
			if err := testEnv.Delete(ctx, n); !apierrors.IsNotFound(err) {
				g.Expect(err).NotTo(HaveOccurred())
			}
		}
		for _, m := range machines {
			g.Expect(testEnv.Delete(ctx, m)).To(Succeed())
		}
		for _, im := range infraMachines {
			if err := testEnv.Delete(ctx, im); !apierrors.IsNotFound(err) {
				g.Expect(err).NotTo(HaveOccurred())
			}
		}
	}

	return nodes, machines, cleanup
}

func newMachineHealthCheckWithLabels(name, namespace, cluster string, labels map[string]string) *clusterv1.MachineHealthCheck {
	l := make(map[string]string, len(labels))
	for k, v := range labels {
		l[k] = v
	}
	l[clusterv1.ClusterLabelName] = cluster

	mhc := newMachineHealthCheck(namespace, cluster)
	mhc.SetName(name)
	mhc.Labels = l
	mhc.Spec.Selector.MatchLabels = l

	return mhc
}

func newMachineHealthCheck(namespace, clusterName string) *clusterv1.MachineHealthCheck {
	return &clusterv1.MachineHealthCheck{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "test-mhc-",
			Namespace:    namespace,
		},
		Spec: clusterv1.MachineHealthCheckSpec{
			ClusterName: clusterName,
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"selector": string(uuid.NewUUID()),
				},
			},
			NodeStartupTimeout: &metav1.Duration{Duration: 1 * time.Millisecond},
			UnhealthyConditions: []clusterv1.UnhealthyCondition{
				{
					Type:    corev1.NodeReady,
					Status:  corev1.ConditionUnknown,
					Timeout: metav1.Duration{Duration: 5 * time.Minute},
				},
			},
		},
	}
}
