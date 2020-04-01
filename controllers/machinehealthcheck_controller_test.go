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
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controllers/external"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const defaultNamespaceName = "default"

var _ = Describe("MachineHealthCheck Reconciler", func() {
	var namespace *corev1.Namespace
	var testCluster *clusterv1.Cluster

	var clusterName = "test-cluster"
	var clusterKubeconfigName = "test-cluster-kubeconfig"
	var namespaceName string

	BeforeEach(func() {
		namespace = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "mhc-test-"}}
		testCluster = &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: clusterName}}

		By("Ensuring the namespace exists")
		Expect(k8sClient.Create(ctx, namespace)).To(Succeed())
		namespaceName = namespace.Name

		By("Creating the Cluster")
		testCluster.Namespace = namespaceName
		Expect(k8sClient.Create(ctx, testCluster)).To(Succeed())

		By("Creating the remote Cluster kubeconfig")
		Expect(kubeconfig.CreateEnvTestSecret(k8sClient, cfg, testCluster)).To(Succeed())
	})

	AfterEach(func() {
		By("Deleting any Nodes")
		Expect(cleanupTestNodes(ctx, k8sClient)).To(Succeed())
		By("Deleting any Machines")
		Expect(cleanupTestMachines(ctx, k8sClient)).To(Succeed())
		By("Deleting any MachineHealthChecks")
		Expect(cleanupTestMachineHealthChecks(ctx, k8sClient)).To(Succeed())
		By("Deleting the Cluster")
		Expect(k8sClient.Delete(ctx, testCluster)).To(Succeed())
		By("Deleting the remote Cluster kubeconfig")
		remoteClusterKubeconfig := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: namespaceName, Name: clusterKubeconfigName}}
		Expect(k8sClient.Delete(ctx, remoteClusterKubeconfig)).To(Succeed())
		By("Deleting the Namespace")
		Expect(k8sClient.Delete(ctx, namespace)).To(Succeed())

		// Ensure the cluster is actually gone before moving on
		Eventually(func() error {
			c := &clusterv1.Cluster{}
			err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: clusterName}, c)
			if err != nil && apierrors.IsNotFound(err) {
				return nil
			} else if err != nil {
				return err
			}
			return errors.New("Cluster not yet deleted")
		}, timeout).Should(Succeed())
	})

	type labelTestCase struct {
		original map[string]string
		expected map[string]string
	}

	DescribeTable("should ensure the cluster-name label is correct",
		func(ltc labelTestCase) {
			By("Creating a MachineHealthCheck")
			mhcToCreate := &clusterv1.MachineHealthCheck{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-mhc",
					Namespace: namespaceName,
					Labels:    ltc.original,
				},
				Spec: clusterv1.MachineHealthCheckSpec{
					ClusterName: clusterName,
					UnhealthyConditions: []clusterv1.UnhealthyCondition{
						{
							Type:    corev1.NodeReady,
							Status:  corev1.ConditionUnknown,
							Timeout: metav1.Duration{Duration: 5 * time.Minute},
						},
					},
				},
			}
			mhcToCreate.Default()
			Expect(k8sClient.Create(ctx, mhcToCreate)).To(Succeed())

			Eventually(func() map[string]string {
				mhc := &clusterv1.MachineHealthCheck{}
				err := k8sClient.Get(ctx, util.ObjectKey(mhcToCreate), mhc)
				if err != nil {
					return nil
				}
				return mhc.GetLabels()
			}, timeout).Should(Equal(ltc.expected))
		},
		Entry("when no existing labels exist", labelTestCase{
			original: map[string]string{},
			expected: map[string]string{clusterv1.ClusterLabelName: clusterName},
		}),
		Entry("when the label has the wrong value", labelTestCase{
			original: map[string]string{clusterv1.ClusterLabelName: "wrong"},
			expected: map[string]string{clusterv1.ClusterLabelName: clusterName},
		}),
		Entry("without modifying other labels", labelTestCase{
			original: map[string]string{"other": "label"},
			expected: map[string]string{"other": "label", clusterv1.ClusterLabelName: clusterName},
		}),
	)

	type ownerReferenceTestCase struct {
		original []metav1.OwnerReference
		// Use a function so that runtime information can be populated (eg UID)
		expected func() []metav1.OwnerReference
	}

	DescribeTable("should ensure an owner reference is present",
		func(ortc ownerReferenceTestCase) {
			By("Creating a MachineHealthCheck")
			mhcToCreate := &clusterv1.MachineHealthCheck{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-mhc",
					Namespace:       namespaceName,
					OwnerReferences: ortc.original,
				},
				Spec: clusterv1.MachineHealthCheckSpec{
					ClusterName: clusterName,
					UnhealthyConditions: []clusterv1.UnhealthyCondition{
						{
							Type:    corev1.NodeReady,
							Status:  corev1.ConditionUnknown,
							Timeout: metav1.Duration{Duration: 5 * time.Minute},
						},
					},
				},
			}
			mhcToCreate.Default()
			Expect(k8sClient.Create(ctx, mhcToCreate)).To(Succeed())

			Eventually(func() []metav1.OwnerReference {
				mhc := &clusterv1.MachineHealthCheck{}
				err := k8sClient.Get(ctx, util.ObjectKey(mhcToCreate), mhc)
				if err != nil {
					return []metav1.OwnerReference{}
				}
				return mhc.GetOwnerReferences()
			}, timeout).Should(ConsistOf(ortc.expected()))
		},
		Entry("when no existing owner references exist", ownerReferenceTestCase{
			original: []metav1.OwnerReference{},
			expected: func() []metav1.OwnerReference {
				return []metav1.OwnerReference{ownerReferenceForCluster(ctx, testCluster)}
			},
		}),
		Entry("when modifying existing owner references", ownerReferenceTestCase{
			original: []metav1.OwnerReference{{Kind: "Foo", APIVersion: "foo.bar.baz/v1", Name: "Bar", UID: "12345"}},
			expected: func() []metav1.OwnerReference {
				return []metav1.OwnerReference{{Kind: "Foo", APIVersion: "foo.bar.baz/v1", Name: "Bar", UID: "12345"}, ownerReferenceForCluster(ctx, testCluster)}
			},
		}),
	)

	Context("when reconciling a MachineHealthCheck", func() {

		// createMachine creates a machine while also maintaining the status that
		// has been set on it
		createMachine := func(m *clusterv1.Machine) {
			status := m.Status
			Expect(k8sClient.Create(ctx, m)).To(Succeed())
			key := util.ObjectKey(m)
			Eventually(func() error {
				if err := k8sClient.Get(ctx, key, m); err != nil {
					return err
				}
				m.Status = status
				return k8sClient.Status().Update(ctx, m)
			}, timeout).Should(Succeed())
		}

		type reconcileTestCase struct {
			mhc                 func() *clusterv1.MachineHealthCheck
			nodes               func() []*corev1.Node
			machines            func() []*clusterv1.Machine
			expectRemediated    func() []*clusterv1.Machine
			expectNotRemediated func() []*clusterv1.Machine
			expectedStatus      clusterv1.MachineHealthCheckStatus
		}

		var labels = map[string]string{"cluster": clusterName, "nodepool": "foo"}
		var controlPlaneLabels = map[string]string{"cluster": clusterName, "nodepool": "foo", clusterv1.MachineControlPlaneLabelName: ""}
		var machineSetOR = metav1.OwnerReference{APIVersion: clusterv1.GroupVersion.String(), Kind: "MachineSet", Name: "machineset", UID: "machinset", Controller: pointer.BoolPtr(true)}
		var controlPlaneOR = metav1.OwnerReference{APIVersion: controlplanev1.GroupVersion.String(), Kind: "KubeadmControlPlane", Name: "controlplane", UID: "controlplane", Controller: pointer.BoolPtr(true)}
		var healthyNodeCondition = corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionTrue}
		var unhealthyNodeCondition = corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionUnknown, LastTransitionTime: metav1.NewTime(time.Now().Add(-10 * time.Minute))}

		// Objects for use in test cases below
		var testMHC *clusterv1.MachineHealthCheck
		var healthyNode1, healthyNode2, unhealthyNode1, unhealthyNode2, controlPlaneNode1, unhealthyControlPlaneNode1, unlabelledNode *corev1.Node
		var healthyMachine1, healthyMachine2, unhealthyMachine1, unhealthyMachine2, noNodeRefMachine1, noNodeRefMachine2, nodeGoneMachine1, controlPlaneMachine1, unhealthyControlPlaneMachine1, unlabelledMachine *clusterv1.Machine

		BeforeEach(func() {
			// Set up objects for test cases before each test
			By("Setting up resources")
			testMHC = newTestMachineHealthCheck("test-mhc", namespaceName, clusterName, labels)
			maxUnhealthy := intstr.Parse("40%")
			testMHC.Spec.MaxUnhealthy = &maxUnhealthy
			testMHC.Default()

			healthyNode1 = newTestNode("healthy-node-1")
			healthyNode1.Status.Conditions = []corev1.NodeCondition{healthyNodeCondition}
			healthyMachine1 = newTestMachine("healthy-machine-1", namespaceName, clusterName, healthyNode1.Name, labels)
			healthyMachine1.SetOwnerReferences([]metav1.OwnerReference{machineSetOR})

			healthyNode2 = newTestNode("healthy-node-2")
			healthyNode2.Status.Conditions = []corev1.NodeCondition{healthyNodeCondition}
			healthyMachine2 = newTestMachine("healthy-machine-2", namespaceName, clusterName, healthyNode2.Name, labels)
			healthyMachine2.SetOwnerReferences([]metav1.OwnerReference{machineSetOR})

			unhealthyNode1 = newTestNode("unhealthy-node-1")
			unhealthyNode1.Status.Conditions = []corev1.NodeCondition{unhealthyNodeCondition}
			unhealthyMachine1 = newTestMachine("unhealthy-machine-1", namespaceName, clusterName, unhealthyNode1.Name, labels)
			unhealthyMachine1.SetOwnerReferences([]metav1.OwnerReference{machineSetOR})

			unhealthyNode2 = newTestNode("unhealthy-node-2")
			unhealthyNode2.Status.Conditions = []corev1.NodeCondition{unhealthyNodeCondition}
			unhealthyMachine2 = newTestMachine("unhealthy-machine-2", namespaceName, clusterName, unhealthyNode2.Name, labels)
			unhealthyMachine2.SetOwnerReferences([]metav1.OwnerReference{machineSetOR})

			noNodeRefMachine1 = newTestMachine("no-node-ref-machine-1", namespaceName, clusterName, "", labels)
			noNodeRefMachine1.Status.NodeRef = nil
			noNodeRefMachine1.SetOwnerReferences([]metav1.OwnerReference{machineSetOR})
			now := metav1.NewTime(time.Now())
			noNodeRefMachine1.Status.LastUpdated = &now

			noNodeRefMachine2 = newTestMachine("no-node-ref-machine-2", namespaceName, clusterName, "", labels)
			noNodeRefMachine2.Status.NodeRef = nil
			noNodeRefMachine2.SetOwnerReferences([]metav1.OwnerReference{machineSetOR})
			lastUpdatedTwiceNodeStartupTimeout := metav1.NewTime(time.Now().Add(-2 * testMHC.Spec.NodeStartupTimeout.Duration))
			noNodeRefMachine2.Status.LastUpdated = &lastUpdatedTwiceNodeStartupTimeout

			nodeGoneMachine1 = newTestMachine("node-gone-machine-1", namespaceName, clusterName, "node-gone-node-1", labels)
			nodeGoneMachine1.SetOwnerReferences([]metav1.OwnerReference{machineSetOR})

			controlPlaneNode1 = newTestNode("control-plane-node-1")
			controlPlaneNode1.Status.Conditions = []corev1.NodeCondition{healthyNodeCondition}
			controlPlaneNode1.Labels = map[string]string{nodeControlPlaneLabel: ""}
			controlPlaneMachine1 = newTestMachine("control-plane-machine-1", namespaceName, clusterName, controlPlaneNode1.Name, controlPlaneLabels)
			controlPlaneMachine1.SetOwnerReferences([]metav1.OwnerReference{controlPlaneOR})

			unhealthyControlPlaneNode1 = newTestNode("unhealthy-control-plane-node-1")
			unhealthyControlPlaneNode1.Status.Conditions = []corev1.NodeCondition{unhealthyNodeCondition}
			unhealthyControlPlaneNode1.Labels = map[string]string{nodeControlPlaneLabel: ""}
			unhealthyControlPlaneMachine1 = newTestMachine("unhealthy-control-plane-machine-1", namespaceName, clusterName, unhealthyControlPlaneNode1.Name, controlPlaneLabels)
			unhealthyControlPlaneMachine1.SetOwnerReferences([]metav1.OwnerReference{controlPlaneOR})

			unlabelledNode = newTestNode("unlabelled-node")
			unlabelledMachine = newTestMachine("unlabelled-machine", namespaceName, clusterName, unlabelledNode.Name, map[string]string{})
		})

		DescribeTable("should remediate unhealthy nodes",
			func(rtc *reconcileTestCase) {
				By("Creating Nodes")
				for _, n := range rtc.nodes() {
					node := *n
					Expect(k8sClient.Create(ctx, &node)).To(Succeed())
				}

				By("Creating Machines")
				for _, m := range rtc.machines() {
					machine := *m
					createMachine(&machine)
				}

				By("Creating a MachineHealthCheck")
				mhc := rtc.mhc()
				mhc.Default()
				Expect(k8sClient.Create(ctx, mhc)).To(Succeed())

				By("Verifying the status has been updated")
				Eventually(func() clusterv1.MachineHealthCheckStatus {
					mhc := &clusterv1.MachineHealthCheck{}
					if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespaceName, Name: rtc.mhc().Name}, mhc); err != nil {
						return clusterv1.MachineHealthCheckStatus{}
					}
					return mhc.Status
				}, timeout).Should(Equal(rtc.expectedStatus))

				// Status has been updated, a reconcile has occurred, should no longer need async assertions

				By("Verifying Machine remediations")
				for _, m := range rtc.expectNotRemediated() {
					machine := &clusterv1.Machine{}
					key := types.NamespacedName{Namespace: m.Namespace, Name: m.Name}
					Expect(k8sClient.Get(ctx, key, machine)).To(Succeed())
					Expect(machine.GetDeletionTimestamp().IsZero()).To(BeTrue())
				}

				for _, m := range rtc.expectRemediated() {
					machine := &clusterv1.Machine{}
					key := types.NamespacedName{Namespace: m.Namespace, Name: m.Name}
					Expect(func() error {
						err := k8sClient.Get(ctx, key, machine)
						if err != nil && kerrors.IsNotFound(err) {
							// Machine has gone
							return nil
						} else if err != nil {
							return err
						}
						// Machine was not deleted
						if machine.GetDeletionTimestamp().IsZero() {
							return fmt.Errorf("Machine was not deleted")
						}
						return nil
					}()).To(Succeed())
				}
			},
			Entry("with healthy Machines", &reconcileTestCase{
				mhc:                 func() *clusterv1.MachineHealthCheck { return testMHC },
				nodes:               func() []*corev1.Node { return []*corev1.Node{healthyNode1, healthyNode2} },
				machines:            func() []*clusterv1.Machine { return []*clusterv1.Machine{healthyMachine1, healthyMachine2} },
				expectRemediated:    func() []*clusterv1.Machine { return []*clusterv1.Machine{} },
				expectNotRemediated: func() []*clusterv1.Machine { return []*clusterv1.Machine{healthyMachine1, healthyMachine2} },
				expectedStatus:      clusterv1.MachineHealthCheckStatus{ExpectedMachines: 2, CurrentHealthy: 2},
			}),
			Entry("with an unhealthy Machine", &reconcileTestCase{
				mhc:   func() *clusterv1.MachineHealthCheck { return testMHC },
				nodes: func() []*corev1.Node { return []*corev1.Node{healthyNode1, healthyNode2, unhealthyNode1} },
				machines: func() []*clusterv1.Machine {
					return []*clusterv1.Machine{healthyMachine1, healthyMachine2, unhealthyMachine1}
				},
				expectRemediated:    func() []*clusterv1.Machine { return []*clusterv1.Machine{unhealthyMachine1} },
				expectNotRemediated: func() []*clusterv1.Machine { return []*clusterv1.Machine{healthyMachine1, healthyMachine2} },
				expectedStatus:      clusterv1.MachineHealthCheckStatus{ExpectedMachines: 3, CurrentHealthy: 2},
			}),
			Entry("when the unhealthy Machines exceed MaxUnhealthy", &reconcileTestCase{
				mhc:   func() *clusterv1.MachineHealthCheck { return testMHC },
				nodes: func() []*corev1.Node { return []*corev1.Node{healthyNode1, unhealthyNode1, unhealthyNode2} },
				machines: func() []*clusterv1.Machine {
					return []*clusterv1.Machine{healthyMachine1, unhealthyMachine1, unhealthyMachine2}
				},
				expectRemediated: func() []*clusterv1.Machine { return []*clusterv1.Machine{} },
				expectNotRemediated: func() []*clusterv1.Machine {
					return []*clusterv1.Machine{healthyMachine1, unhealthyMachine1, unhealthyMachine2}
				},
				expectedStatus: clusterv1.MachineHealthCheckStatus{ExpectedMachines: 3, CurrentHealthy: 1},
			}),
			Entry("when a Machine has no Node ref for less than the NodeStartupTimeout", &reconcileTestCase{
				mhc:   func() *clusterv1.MachineHealthCheck { return testMHC },
				nodes: func() []*corev1.Node { return []*corev1.Node{healthyNode1, healthyNode2} },
				machines: func() []*clusterv1.Machine {
					return []*clusterv1.Machine{healthyMachine1, healthyMachine2, noNodeRefMachine1}
				},
				expectRemediated: func() []*clusterv1.Machine { return []*clusterv1.Machine{} },
				expectNotRemediated: func() []*clusterv1.Machine {
					return []*clusterv1.Machine{healthyMachine1, healthyMachine2, noNodeRefMachine1}
				},
				expectedStatus: clusterv1.MachineHealthCheckStatus{ExpectedMachines: 3, CurrentHealthy: 2},
			}),
			Entry("when a Machine has no Node ref for longer than the NodeStartupTimeout", &reconcileTestCase{
				mhc:   func() *clusterv1.MachineHealthCheck { return testMHC },
				nodes: func() []*corev1.Node { return []*corev1.Node{healthyNode1, healthyNode2} },
				machines: func() []*clusterv1.Machine {
					return []*clusterv1.Machine{healthyMachine1, healthyMachine2, noNodeRefMachine2}
				},
				expectRemediated: func() []*clusterv1.Machine { return []*clusterv1.Machine{noNodeRefMachine2} },
				expectNotRemediated: func() []*clusterv1.Machine {
					return []*clusterv1.Machine{healthyMachine1, healthyMachine2}
				},
				expectedStatus: clusterv1.MachineHealthCheckStatus{ExpectedMachines: 3, CurrentHealthy: 2},
			}),
			Entry("when a Machine's Node has gone away", &reconcileTestCase{
				mhc:   func() *clusterv1.MachineHealthCheck { return testMHC },
				nodes: func() []*corev1.Node { return []*corev1.Node{healthyNode1, healthyNode2} },
				machines: func() []*clusterv1.Machine {
					return []*clusterv1.Machine{healthyMachine1, healthyMachine2, nodeGoneMachine1}
				},
				expectRemediated: func() []*clusterv1.Machine { return []*clusterv1.Machine{nodeGoneMachine1} },
				expectNotRemediated: func() []*clusterv1.Machine {
					return []*clusterv1.Machine{healthyMachine1, healthyMachine2}
				},
				expectedStatus: clusterv1.MachineHealthCheckStatus{ExpectedMachines: 3, CurrentHealthy: 2},
			}),
			Entry("with a healthy control plane Machine", &reconcileTestCase{
				mhc:   func() *clusterv1.MachineHealthCheck { return testMHC },
				nodes: func() []*corev1.Node { return []*corev1.Node{healthyNode1, healthyNode2, controlPlaneNode1} },
				machines: func() []*clusterv1.Machine {
					return []*clusterv1.Machine{healthyMachine1, healthyMachine2, controlPlaneMachine1}
				},
				expectRemediated: func() []*clusterv1.Machine { return []*clusterv1.Machine{} },
				expectNotRemediated: func() []*clusterv1.Machine {
					return []*clusterv1.Machine{healthyMachine1, healthyMachine2, controlPlaneMachine1}
				},
				expectedStatus: clusterv1.MachineHealthCheckStatus{ExpectedMachines: 3, CurrentHealthy: 3},
			}),
			Entry("with an unhealthy control plane Machine", &reconcileTestCase{
				mhc: func() *clusterv1.MachineHealthCheck { return testMHC },
				nodes: func() []*corev1.Node {
					return []*corev1.Node{healthyNode1, healthyNode2, unhealthyControlPlaneNode1}
				},
				machines: func() []*clusterv1.Machine {
					return []*clusterv1.Machine{healthyMachine1, healthyMachine2, unhealthyControlPlaneMachine1}
				},
				expectRemediated:    func() []*clusterv1.Machine { return []*clusterv1.Machine{unhealthyControlPlaneMachine1} },
				expectNotRemediated: func() []*clusterv1.Machine { return []*clusterv1.Machine{healthyMachine1, healthyMachine2} },
				expectedStatus:      clusterv1.MachineHealthCheckStatus{ExpectedMachines: 3, CurrentHealthy: 2},
			}),
			Entry("when no Machines are matched by the selector", &reconcileTestCase{
				mhc:                 func() *clusterv1.MachineHealthCheck { return testMHC },
				nodes:               func() []*corev1.Node { return []*corev1.Node{unlabelledNode} },
				machines:            func() []*clusterv1.Machine { return []*clusterv1.Machine{unlabelledMachine} },
				expectRemediated:    func() []*clusterv1.Machine { return []*clusterv1.Machine{} },
				expectNotRemediated: func() []*clusterv1.Machine { return []*clusterv1.Machine{unlabelledMachine} },
				expectedStatus:      clusterv1.MachineHealthCheckStatus{ExpectedMachines: 0, CurrentHealthy: 0},
			}),
		)
	})
})

func cleanupTestMachineHealthChecks(ctx context.Context, c client.Client) error {
	mhcList := &clusterv1.MachineHealthCheckList{}
	if err := c.List(ctx, mhcList); err != nil {
		return err
	}
	for _, mhc := range mhcList.Items {
		m := mhc
		if err := c.Delete(ctx, &m); err != nil {
			return err
		}
	}
	return nil
}

func cleanupTestMachines(ctx context.Context, c client.Client) error {
	machineList := &clusterv1.MachineList{}
	if err := c.List(ctx, machineList); err != nil {
		return err
	}
	for _, machine := range machineList.Items {
		m := machine
		if err := c.Delete(ctx, &m); err != nil && apierrors.IsNotFound(err) {
			return nil
		} else if err != nil {
			return err
		}
		Eventually(func() error {
			if err := c.Get(ctx, util.ObjectKey(&m), &m); err != nil && apierrors.IsNotFound(err) {
				return nil
			} else if err != nil {
				return err
			}
			m.SetFinalizers([]string{})
			return c.Update(ctx, &m)
		}, timeout).Should(Succeed())
	}
	return nil
}

func cleanupTestNodes(ctx context.Context, c client.Client) error {
	nodeList := &corev1.NodeList{}
	if err := c.List(ctx, nodeList); err != nil {
		return err
	}
	for _, node := range nodeList.Items {
		n := node
		if err := c.Delete(ctx, &n); err != nil {
			return err
		}
	}
	return nil
}

func ownerReferenceForCluster(ctx context.Context, c *clusterv1.Cluster) metav1.OwnerReference {
	// Fetch the cluster to populate the UID
	cc := &clusterv1.Cluster{}
	Expect(k8sClient.Get(ctx, util.ObjectKey(c), cc)).To(Succeed())

	return metav1.OwnerReference{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Cluster",
		Name:       cc.Name,
		UID:        cc.UID,
	}
}

func TestClusterToMachineHealthCheck(t *testing.T) {
	// This test sets up a proper test env to allow testing of the cache index
	// that is used as part of the clusterToMachineHealthCheck map function

	// BEGIN: Set up test environment
	g := NewWithT(t)

	testEnv = &envtest.Environment{
		CRDs: []runtime.Object{
			external.TestGenericBootstrapCRD,
			external.TestGenericBootstrapTemplateCRD,
			external.TestGenericInfrastructureCRD,
			external.TestGenericInfrastructureTemplateCRD,
		},
		CRDDirectoryPaths: []string{filepath.Join("..", "config", "crd", "bases")},
	}

	var err error
	cfg, err := testEnv.Start()
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(cfg).ToNot(BeNil())
	defer func() {
		g.Expect(testEnv.Stop()).To(Succeed())
	}()

	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())

	mgr, err = manager.New(cfg, manager.Options{
		Scheme:             scheme.Scheme,
		MetricsBindAddress: "0",
	})
	g.Expect(err).ToNot(HaveOccurred())

	r := &MachineHealthCheckReconciler{
		Log:    log.Log,
		Client: mgr.GetClient(),
	}
	g.Expect(r.SetupWithManager(mgr, controller.Options{})).To(Succeed())

	doneMgr := make(chan struct{})
	go func() {
		g.Expect(mgr.Start(doneMgr)).To(Succeed())
	}()
	defer close(doneMgr)

	// END: setup test environment

	namespace := defaultNamespaceName
	clusterName := "test-cluster"
	labels := make(map[string]string)

	mhc1 := newTestMachineHealthCheck("mhc1", namespace, clusterName, labels)
	mhc1Req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: mhc1.Namespace, Name: mhc1.Name}}
	mhc2 := newTestMachineHealthCheck("mhc2", namespace, clusterName, labels)
	mhc2Req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: mhc2.Namespace, Name: mhc2.Name}}
	mhc3 := newTestMachineHealthCheck("mhc3", namespace, "othercluster", labels)
	mhc4 := newTestMachineHealthCheck("mhc4", "othernamespace", clusterName, labels)
	cluster1 := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName,
			Namespace: namespace,
		},
	}

	testCases := []struct {
		name     string
		toCreate []clusterv1.MachineHealthCheck
		object   handler.MapObject
		expected []reconcile.Request
	}{
		{
			name:     "when the object passed isn't a cluster",
			toCreate: []clusterv1.MachineHealthCheck{*mhc1},
			object: handler.MapObject{
				Object: &clusterv1.Machine{},
			},
			expected: []reconcile.Request{},
		},
		{
			name:     "when a MachineHealthCheck exists for the Cluster in the same namespace",
			toCreate: []clusterv1.MachineHealthCheck{*mhc1},
			object: handler.MapObject{
				Object: cluster1,
			},
			expected: []reconcile.Request{mhc1Req},
		},
		{
			name:     "when 2 MachineHealthChecks exists for the Cluster in the same namespace",
			toCreate: []clusterv1.MachineHealthCheck{*mhc1, *mhc2},
			object: handler.MapObject{
				Object: cluster1,
			},
			expected: []reconcile.Request{mhc1Req, mhc2Req},
		},
		{
			name:     "when a MachineHealthCheck exists for another Cluster in the same namespace",
			toCreate: []clusterv1.MachineHealthCheck{*mhc3},
			object: handler.MapObject{
				Object: cluster1,
			},
			expected: []reconcile.Request{},
		},
		{
			name:     "when a MachineHealthCheck exists for another Cluster in another namespace",
			toCreate: []clusterv1.MachineHealthCheck{*mhc4},
			object: handler.MapObject{
				Object: cluster1,
			},
			expected: []reconcile.Request{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gs := NewWithT(t)

			ctx := context.Background()
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
				gs.Eventually(getObj, timeout).Should(Succeed())
			}

			got := r.clusterToMachineHealthCheck(tc.object)
			gs.Expect(got).To(ConsistOf(tc.expected))
		})
	}
}

func TestIndexMachineHealthCheckByClusterName(t *testing.T) {
	r := &MachineHealthCheckReconciler{
		Log: log.Log,
	}

	testCases := []struct {
		name     string
		object   runtime.Object
		expected []string
	}{
		{
			name:     "when the MachineHealthCheck has no ClusterName",
			object:   &clusterv1.MachineHealthCheck{},
			expected: []string{""},
		},
		{
			name: "when the MachineHealthCheck has a ClusterName",
			object: &clusterv1.MachineHealthCheck{
				Spec: clusterv1.MachineHealthCheckSpec{
					ClusterName: "test-cluster",
				},
			},
			expected: []string{"test-cluster"},
		},
		{
			name:     "when the object passed is not a MachineHealthCheck",
			object:   &corev1.Node{},
			expected: []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			got := r.indexMachineHealthCheckByClusterName(tc.object)
			g.Expect(got).To(ConsistOf(tc.expected))
		})
	}
}

func newTestMachineHealthCheck(name, namespace, cluster string, labels map[string]string) *clusterv1.MachineHealthCheck {
	return &clusterv1.MachineHealthCheck{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: clusterv1.MachineHealthCheckSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: labels,
			},
			ClusterName: cluster,
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

func TestMachineToMachineHealthCheck(t *testing.T) {
	// This test sets up a proper test env to allow testing of the cache index
	// that is used as part of the clusterToMachineHealthCheck map function

	// BEGIN: Set up test environment
	g := NewWithT(t)

	testEnv = &envtest.Environment{
		CRDs: []runtime.Object{
			external.TestGenericBootstrapCRD,
			external.TestGenericBootstrapTemplateCRD,
			external.TestGenericInfrastructureCRD,
			external.TestGenericInfrastructureTemplateCRD,
		},
		CRDDirectoryPaths: []string{filepath.Join("..", "config", "crd", "bases")},
	}

	var err error
	cfg, err := testEnv.Start()
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(cfg).ToNot(BeNil())
	defer func() {
		g.Expect(testEnv.Stop()).To(Succeed())
	}()

	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())

	mgr, err = manager.New(cfg, manager.Options{
		Scheme:             scheme.Scheme,
		MetricsBindAddress: "0",
	})
	g.Expect(err).ToNot(HaveOccurred())

	r := &MachineHealthCheckReconciler{
		Log:    log.Log,
		Client: mgr.GetClient(),
	}
	g.Expect(r.SetupWithManager(mgr, controller.Options{})).To(Succeed())

	doneMgr := make(chan struct{})
	go func() {
		g.Expect(mgr.Start(doneMgr)).To(Succeed())
	}()
	defer close(doneMgr)

	// END: setup test environment

	namespace := defaultNamespaceName
	clusterName := "test-cluster"
	nodeName := "node1"
	labels := map[string]string{"cluster": "foo", "nodepool": "bar"}

	mhc1 := newTestMachineHealthCheck("mhc1", namespace, clusterName, labels)
	mhc1Req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: mhc1.Namespace, Name: mhc1.Name}}
	mhc2 := newTestMachineHealthCheck("mhc2", namespace, clusterName, labels)
	mhc2Req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: mhc2.Namespace, Name: mhc2.Name}}
	mhc3 := newTestMachineHealthCheck("mhc3", namespace, clusterName, map[string]string{"cluster": "foo", "nodepool": "other"})
	mhc4 := newTestMachineHealthCheck("mhc4", "othernamespace", clusterName, labels)
	machine1 := newTestMachine("machine1", namespace, clusterName, nodeName, labels)

	testCases := []struct {
		name     string
		toCreate []clusterv1.MachineHealthCheck
		object   handler.MapObject
		expected []reconcile.Request
	}{
		{
			name:     "when the object passed isn't a machine",
			toCreate: []clusterv1.MachineHealthCheck{*mhc1},
			object: handler.MapObject{
				Object: &clusterv1.Cluster{},
			},
			expected: []reconcile.Request{},
		},
		{
			name:     "when a MachineHealthCheck matches labels for the Machine in the same namespace",
			toCreate: []clusterv1.MachineHealthCheck{*mhc1},
			object: handler.MapObject{
				Object: machine1,
			},
			expected: []reconcile.Request{mhc1Req},
		},
		{
			name:     "when 2 MachineHealthChecks match labels for the Machine in the same namespace",
			toCreate: []clusterv1.MachineHealthCheck{*mhc1, *mhc2},
			object: handler.MapObject{
				Object: machine1,
			},
			expected: []reconcile.Request{mhc1Req, mhc2Req},
		},
		{
			name:     "when a MachineHealthCheck does not match labels for the Machine in the same namespace",
			toCreate: []clusterv1.MachineHealthCheck{*mhc3},
			object: handler.MapObject{
				Object: machine1,
			},
			expected: []reconcile.Request{},
		},
		{
			name:     "when a MachineHealthCheck matches labels for the Machine in another namespace",
			toCreate: []clusterv1.MachineHealthCheck{*mhc4},
			object: handler.MapObject{
				Object: machine1,
			},
			expected: []reconcile.Request{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gs := NewWithT(t)

			ctx := context.Background()
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
				gs.Eventually(getObj, timeout).Should(Succeed())
			}

			got := r.machineToMachineHealthCheck(tc.object)
			gs.Expect(got).To(ConsistOf(tc.expected))
		})
	}
}

func TestNodeToMachineHealthCheck(t *testing.T) {
	// This test sets up a proper test env to allow testing of the cache index
	// that is used as part of the clusterToMachineHealthCheck map function

	// BEGIN: Set up test environment
	g := NewWithT(t)

	testEnv = &envtest.Environment{
		CRDs: []runtime.Object{
			external.TestGenericBootstrapCRD,
			external.TestGenericBootstrapTemplateCRD,
			external.TestGenericInfrastructureCRD,
			external.TestGenericInfrastructureTemplateCRD,
		},
		CRDDirectoryPaths: []string{filepath.Join("..", "config", "crd", "bases")},
	}

	var err error
	cfg, err := testEnv.Start()
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(cfg).ToNot(BeNil())
	defer func() {
		g.Expect(testEnv.Stop()).To(Succeed())
	}()

	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())

	mgr, err = manager.New(cfg, manager.Options{
		Scheme:             scheme.Scheme,
		MetricsBindAddress: "0",
	})
	g.Expect(err).ToNot(HaveOccurred())

	r := &MachineHealthCheckReconciler{
		Log:    log.Log,
		Client: mgr.GetClient(),
	}
	g.Expect(r.SetupWithManager(mgr, controller.Options{})).To(Succeed())

	doneMgr := make(chan struct{})
	go func() {
		g.Expect(mgr.Start(doneMgr)).To(Succeed())
	}()
	defer close(doneMgr)

	// END: setup test environment

	namespace := defaultNamespaceName
	clusterName := "test-cluster"
	nodeName := "node1"
	labels := map[string]string{"cluster": "foo", "nodepool": "bar"}

	mhc1 := newTestMachineHealthCheck("mhc1", namespace, clusterName, labels)
	mhc1Req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: mhc1.Namespace, Name: mhc1.Name}}
	mhc2 := newTestMachineHealthCheck("mhc2", namespace, clusterName, labels)
	mhc2Req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: mhc2.Namespace, Name: mhc2.Name}}
	mhc3 := newTestMachineHealthCheck("mhc3", namespace, "othercluster", labels)
	mhc4 := newTestMachineHealthCheck("mhc4", "othernamespace", clusterName, labels)

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
		object      handler.MapObject
		expected    []reconcile.Request
	}{
		{
			name:        "when the object passed isn't a Node",
			mhcToCreate: []clusterv1.MachineHealthCheck{*mhc1},
			mToCreate:   []clusterv1.Machine{*machine1},
			object: handler.MapObject{
				Object: &clusterv1.Machine{},
			},
			expected: []reconcile.Request{},
		},
		{
			name:        "when no Machine exists for the Node",
			mhcToCreate: []clusterv1.MachineHealthCheck{*mhc1},
			mToCreate:   []clusterv1.Machine{},
			object: handler.MapObject{
				Object: node1,
			},
			expected: []reconcile.Request{},
		},
		{
			name:        "when two Machines exist for the Node",
			mhcToCreate: []clusterv1.MachineHealthCheck{*mhc1},
			mToCreate:   []clusterv1.Machine{*machine1, *machine2},
			object: handler.MapObject{
				Object: node1,
			},
			expected: []reconcile.Request{},
		},
		{
			name:        "when no MachineHealthCheck exists for the Node in the Machine's namespace",
			mhcToCreate: []clusterv1.MachineHealthCheck{*mhc4},
			mToCreate:   []clusterv1.Machine{*machine1},
			object: handler.MapObject{
				Object: node1,
			},
			expected: []reconcile.Request{},
		},
		{
			name:        "when a MachineHealthCheck exists for the Node in the Machine's namespace",
			mhcToCreate: []clusterv1.MachineHealthCheck{*mhc1},
			mToCreate:   []clusterv1.Machine{*machine1},
			object: handler.MapObject{
				Object: node1,
			},
			expected: []reconcile.Request{mhc1Req},
		},
		{
			name:        "when two MachineHealthChecks exist for the Node in the Machine's namespace",
			mhcToCreate: []clusterv1.MachineHealthCheck{*mhc1, *mhc2},
			mToCreate:   []clusterv1.Machine{*machine1},
			object: handler.MapObject{
				Object: node1,
			},
			expected: []reconcile.Request{mhc1Req, mhc2Req},
		},
		{
			name:        "when a MachineHealthCheck exists for the Node, but not in the Machine's namespace",
			mhcToCreate: []clusterv1.MachineHealthCheck{*mhc3},
			mToCreate:   []clusterv1.Machine{*machine1},
			object: handler.MapObject{
				Object: node1,
			},
			expected: []reconcile.Request{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gs := NewWithT(t)

			ctx := context.Background()
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
				gs.Eventually(getObj, timeout).Should(Succeed())
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
				gs.Eventually(checkStatus, timeout).Should(Equal(o.Status))
			}

			got := r.nodeToMachineHealthCheck(tc.object)
			gs.Expect(got).To(ConsistOf(tc.expected))
		})
	}
}

func TestIndexMachineByNodeName(t *testing.T) {
	r := &MachineHealthCheckReconciler{
		Log: log.Log,
	}

	testCases := []struct {
		name     string
		object   runtime.Object
		expected []string
	}{
		{
			name:     "when the machine has no NodeRef",
			object:   &clusterv1.Machine{},
			expected: []string{},
		},
		{
			name: "when the machine has valid a NodeRef",
			object: &clusterv1.Machine{
				Status: clusterv1.MachineStatus{
					NodeRef: &corev1.ObjectReference{
						Name: "node1",
					},
				},
			},
			expected: []string{"node1"},
		},
		{
			name:     "when the object passed is not a Machine",
			object:   &corev1.Node{},
			expected: []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			got := r.indexMachineByNodeName(tc.object)
			g.Expect(got).To(ConsistOf(tc.expected))
		})
	}
}

func TestIsAllowedRedmediation(t *testing.T) {
	testCases := []struct {
		name             string
		maxUnhealthy     *intstr.IntOrString
		expectedMachines int32
		currentHealthy   int32
		allowed          bool
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
					MaxUnhealthy: tc.maxUnhealthy,
				},
				Status: clusterv1.MachineHealthCheckStatus{
					ExpectedMachines: tc.expectedMachines,
					CurrentHealthy:   tc.currentHealthy,
				},
			}

			g.Expect(isAllowedRemediation(mhc)).To(Equal(tc.allowed))
		})
	}
}
