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

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
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
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const defaultNamespaceName = "default"

var _ = Describe("MachineHealthCheck", func() {
	Context("on reconciliation", func() {
		var (
			cluster *clusterv1.Cluster
			mhc     *clusterv1.MachineHealthCheck
		)

		var (
			nodes             []*corev1.Node
			machines          []*clusterv1.Machine
			fakeNodesMachines = func(n int, healthy bool, nodeRef bool) {
				machine := clusterv1.Machine{
					TypeMeta: metav1.TypeMeta{
						APIVersion: clusterv1.GroupVersion.String(),
						Kind:       "Machine",
					},
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "test-mhc-machine-",
						Namespace:    mhc.Namespace,
						Labels:       mhc.Spec.Selector.MatchLabels,
					},
					Spec: clusterv1.MachineSpec{
						ClusterName: cluster.Name,
						Bootstrap: clusterv1.Bootstrap{
							Data: pointer.StringPtr("data"),
						},
					},
					Status: clusterv1.MachineStatus{
						InfrastructureReady: true,
						BootstrapReady:      true,
						Phase:               string(clusterv1.MachinePhaseRunning),
					},
				}

				node := corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "test-mhc-node-",
					},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
						},
					},
				}
				if !healthy {
					node.Status.Conditions[0] = corev1.NodeCondition{
						Type:               corev1.NodeReady,
						Status:             corev1.ConditionUnknown,
						LastTransitionTime: metav1.NewTime(time.Now().Add(-10 * time.Minute)),
					}
				}

				for i := 0; i < n; i++ {
					providerID := fmt.Sprintf("test:////%v", uuid.NewUUID())

					infraMachine := &unstructured.Unstructured{
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
					}
					Expect(testEnv.Create(ctx, infraMachine)).To(Succeed())
					infraMachinePatch := client.MergeFrom(infraMachine.DeepCopy())
					Expect(unstructured.SetNestedField(infraMachine.Object, true, "status", "ready")).To(Succeed())
					Expect(testEnv.Status().Patch(ctx, infraMachine, infraMachinePatch)).To(Succeed())

					machine := machine.DeepCopy()
					machine.Spec.InfrastructureRef = corev1.ObjectReference{
						APIVersion: infraMachine.GetAPIVersion(),
						Kind:       infraMachine.GetKind(),
						Name:       infraMachine.GetName(),
					}
					machineStatus := machine.Status
					Expect(testEnv.Create(ctx, machine)).To(Succeed())

					if nodeRef {
						node := node.DeepCopy()
						node.Spec.ProviderID = providerID
						nodeStatus := node.Status
						Expect(testEnv.Create(ctx, node)).To(Succeed())
						nodePatch := client.MergeFrom(node.DeepCopy())
						node.Status = nodeStatus
						Expect(testEnv.Status().Patch(ctx, node, nodePatch)).To(Succeed())
						nodes = append(nodes, node)

						machinePatch := client.MergeFrom(machine.DeepCopy())
						machine.Status = machineStatus
						machine.Status.NodeRef = &corev1.ObjectReference{
							Name: node.Name,
						}

						now := metav1.NewTime(time.Now())
						machine.Status.LastUpdated = &now
						Expect(testEnv.Status().Patch(ctx, machine, machinePatch)).To(Succeed())
					}

					machines = append(machines, machine)
				}
			}
		)

		BeforeEach(func() {
			nodes = []*corev1.Node{}
			machines = []*clusterv1.Machine{}
			cluster = &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-cluster-",
					Namespace:    "default",
				},
			}
			Expect(testEnv.Create(ctx, cluster)).To(Succeed())
			Expect(testEnv.CreateKubeconfigSecret(cluster)).To(Succeed())

			mhc = &clusterv1.MachineHealthCheck{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-mhc-",
					Namespace:    cluster.Namespace,
				},
				Spec: clusterv1.MachineHealthCheckSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"selector": string(uuid.NewUUID()),
						},
					},
					UnhealthyConditions: []clusterv1.UnhealthyCondition{
						{
							Type:    corev1.NodeReady,
							Status:  corev1.ConditionUnknown,
							Timeout: metav1.Duration{Duration: 5 * time.Minute},
						},
					},
				},
			}
			mhc.Default()
		})

		AfterEach(func() {
			for _, node := range nodes {
				if err := testEnv.Delete(ctx, node); !apierrors.IsNotFound(err) {
					Expect(err).NotTo(HaveOccurred())
				}
			}
			for _, machine := range machines {
				Expect(testEnv.Delete(ctx, machine)).To(Succeed())
			}
			Expect(testEnv.Delete(ctx, mhc)).To(Succeed())
			Expect(testEnv.Delete(ctx, cluster)).To(Succeed())
		})

		Context("it should ensure the correct cluster-name label", func() {
			Specify("with no existing labels exist", func() {
				mhc.Spec.ClusterName = cluster.Name
				mhc.Labels = map[string]string{}
				Expect(testEnv.Create(ctx, mhc)).To(Succeed())

				Eventually(func() map[string]string {
					err := testEnv.Get(ctx, util.ObjectKey(mhc), mhc)
					if err != nil {
						return nil
					}
					return mhc.GetLabels()
				}).Should(HaveKeyWithValue(clusterv1.ClusterLabelName, cluster.Name))
			})

			Specify("when the label has the wrong value", func() {
				mhc.Spec.ClusterName = cluster.Name
				mhc.Labels = map[string]string{
					clusterv1.ClusterLabelName: "wrong-cluster",
				}
				Expect(testEnv.Create(ctx, mhc)).To(Succeed())

				Eventually(func() map[string]string {
					err := testEnv.Get(ctx, util.ObjectKey(mhc), mhc)
					if err != nil {
						return nil
					}
					return mhc.GetLabels()
				}).Should(HaveKeyWithValue(clusterv1.ClusterLabelName, cluster.Name))
			})

			Specify("when other labels are present", func() {
				mhc.Spec.ClusterName = cluster.Name
				mhc.Labels = map[string]string{
					"extra-label": "1",
				}
				Expect(testEnv.Create(ctx, mhc)).To(Succeed())

				Eventually(func() map[string]string {
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
		})

		Context("it should ensure an owner reference is present", func() {
			Specify("when no existing ones exist", func() {
				mhc.Spec.ClusterName = cluster.Name
				mhc.OwnerReferences = nil
				Expect(testEnv.Create(ctx, mhc)).To(Succeed())

				Eventually(func() []metav1.OwnerReference {
					err := testEnv.Get(ctx, util.ObjectKey(mhc), mhc)
					if err != nil {
						return nil
					}
					return mhc.GetOwnerReferences()
				}).Should(And(
					ContainElement(ownerReferenceForCluster(ctx, cluster)),
					HaveLen(1),
				))
			})

			Specify("when modifying existing ones", func() {
				mhc.Spec.ClusterName = cluster.Name
				mhc.OwnerReferences = []metav1.OwnerReference{
					{Kind: "Foo", APIVersion: "foo.bar.baz/v1", Name: "Bar", UID: "12345"},
				}
				Expect(testEnv.Create(ctx, mhc)).To(Succeed())

				Eventually(func() []metav1.OwnerReference {
					err := testEnv.Get(ctx, util.ObjectKey(mhc), mhc)
					if err != nil {
						return nil
					}
					return mhc.GetOwnerReferences()
				}).Should(And(
					ContainElements(
						metav1.OwnerReference{Kind: "Foo", APIVersion: "foo.bar.baz/v1", Name: "Bar", UID: "12345"},
						ownerReferenceForCluster(ctx, cluster)),
					HaveLen(2),
				))
			})
		})

		Context("it marks unhealthy machines for remediation", func() {
			Specify("when all Machines are healthy", func() {
				mhc.Spec.ClusterName = cluster.Name
				Expect(testEnv.Create(ctx, mhc)).To(Succeed())

				// Healthy nodes and machines.
				fakeNodesMachines(2, true, true)
				targetMachines := make([]string, len(machines))
				for i, m := range machines {
					targetMachines[i] = m.Name
				}
				// Make sure the status matches.
				Eventually(func() *clusterv1.MachineHealthCheckStatus {
					err := testEnv.Get(ctx, util.ObjectKey(mhc), mhc)
					if err != nil {
						return nil
					}
					return &mhc.Status
				}).Should(Equal(&clusterv1.MachineHealthCheckStatus{
					ExpectedMachines: 2,
					CurrentHealthy:   2,
					Targets:          targetMachines},
				))
			})

			Specify("when there is one unhealthy Machine", func() {
				mhc.Spec.ClusterName = cluster.Name
				Expect(testEnv.Create(ctx, mhc)).To(Succeed())

				// Healthy nodes and machines.
				fakeNodesMachines(2, true, true)
				// Unhealthy nodes and machines.
				fakeNodesMachines(1, false, true)
				targetMachines := make([]string, len(machines))
				for i, m := range machines {
					targetMachines[i] = m.Name
				}

				// Make sure the status matches.
				Eventually(func() *clusterv1.MachineHealthCheckStatus {
					err := testEnv.Get(ctx, util.ObjectKey(mhc), mhc)
					if err != nil {
						return nil
					}
					return &mhc.Status
				}).Should(MatchMachineHealthCheckStatus(&clusterv1.MachineHealthCheckStatus{
					ExpectedMachines: 3,
					CurrentHealthy:   2,
					Targets:          targetMachines,
				}))
			})

			Specify("when the unhealthy Machines exceed MaxUnhealthy", func() {
				mhc.Spec.ClusterName = cluster.Name
				maxUnhealthy := intstr.Parse("40%")
				mhc.Spec.MaxUnhealthy = &maxUnhealthy
				Expect(testEnv.Create(ctx, mhc)).To(Succeed())

				// Healthy nodes and machines.
				fakeNodesMachines(1, true, true)
				// Unhealthy nodes and machines.
				fakeNodesMachines(2, false, true)
				targetMachines := make([]string, len(machines))
				for i, m := range machines {
					targetMachines[i] = m.Name
				}

				// Make sure the status matches.
				Eventually(func() *clusterv1.MachineHealthCheckStatus {
					err := testEnv.Get(ctx, util.ObjectKey(mhc), mhc)
					if err != nil {
						return nil
					}
					return &mhc.Status
				}).Should(MatchMachineHealthCheckStatus(&clusterv1.MachineHealthCheckStatus{
					ExpectedMachines: 3,
					CurrentHealthy:   1,
					Targets:          targetMachines},
				))

				// Calculate how many Machines have health check succeeded = false.
				Eventually(func() (unhealthy int) {
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
				Eventually(func() (remediated int) {
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
		})

		Specify("when a Machine has no Node ref for less than the NodeStartupTimeout", func() {
			mhc.Spec.ClusterName = cluster.Name
			Expect(testEnv.Create(ctx, mhc)).To(Succeed())

			// Healthy nodes and machines.
			fakeNodesMachines(2, true, true)
			// Unhealthy nodes and machines.
			fakeNodesMachines(1, false, false)
			targetMachines := make([]string, len(machines))
			for i, m := range machines {
				targetMachines[i] = m.Name
			}

			// Make sure the status matches.
			Eventually(func() *clusterv1.MachineHealthCheckStatus {
				err := testEnv.Get(ctx, util.ObjectKey(mhc), mhc)
				if err != nil {
					return nil
				}
				return &mhc.Status
			}).Should(MatchMachineHealthCheckStatus(&clusterv1.MachineHealthCheckStatus{
				ExpectedMachines: 3,
				CurrentHealthy:   2,
				Targets:          targetMachines},
			))

			// Calculate how many Machines have health check succeeded = false.
			Eventually(func() (unhealthy int) {
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
			Eventually(func() (remediated int) {
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

		Specify("when a Machine has no Node ref for longer than the NodeStartupTimeout", func() {
			mhc.Spec.ClusterName = cluster.Name
			mhc.Spec.NodeStartupTimeout = &metav1.Duration{Duration: 5 * time.Second}
			Expect(testEnv.Create(ctx, mhc)).To(Succeed())

			// Healthy nodes and machines.
			fakeNodesMachines(2, true, true)
			// Unhealthy nodes and machines.
			fakeNodesMachines(1, false, false)
			targetMachines := make([]string, len(machines))
			for i, m := range machines {
				targetMachines[i] = m.Name
			}

			// Make sure the status matches.
			Eventually(func() *clusterv1.MachineHealthCheckStatus {
				err := testEnv.Get(ctx, util.ObjectKey(mhc), mhc)
				if err != nil {
					return nil
				}
				return &mhc.Status
			}).Should(MatchMachineHealthCheckStatus(&clusterv1.MachineHealthCheckStatus{
				ExpectedMachines: 3,
				CurrentHealthy:   2,
				Targets:          targetMachines},
			))

			// Calculate how many Machines have health check succeeded = false.
			Eventually(func() (unhealthy int) {
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
			Eventually(func() (remediated int) {
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

		Specify("when a Machine's Node has gone away", func() {
			mhc.Spec.ClusterName = cluster.Name
			Expect(testEnv.Create(ctx, mhc)).To(Succeed())

			// Healthy nodes and machines.
			fakeNodesMachines(3, true, true)
			targetMachines := make([]string, len(machines))
			for i, m := range machines {
				targetMachines[i] = m.Name
			}

			// Forcibly remove the last machine's node.
			Eventually(func() bool {
				nodeToBeRemoved := nodes[2]
				if err := testEnv.Delete(ctx, nodeToBeRemoved); err != nil {
					return apierrors.IsNotFound(err)
				}
				return apierrors.IsNotFound(testEnv.Get(ctx, util.ObjectKey(nodeToBeRemoved), nodeToBeRemoved))
			}).Should(BeTrue())

			// Make sure the status matches.
			Eventually(func() *clusterv1.MachineHealthCheckStatus {
				err := testEnv.Get(ctx, util.ObjectKey(mhc), mhc)
				if err != nil {
					return nil
				}
				return &mhc.Status
			}).Should(MatchMachineHealthCheckStatus(&clusterv1.MachineHealthCheckStatus{
				ExpectedMachines: 3,
				CurrentHealthy:   2,
				Targets:          targetMachines},
			))

			// Calculate how many Machines have health check succeeded = false.
			Eventually(func() (unhealthy int) {
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
			Eventually(func() (remediated int) {
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

		Specify("should react when a Node transitions to unhealthy", func() {
			mhc.Spec.ClusterName = cluster.Name
			Expect(testEnv.Create(ctx, mhc)).To(Succeed())

			// Healthy nodes and machines.
			fakeNodesMachines(1, true, true)
			targetMachines := make([]string, len(machines))
			for i, m := range machines {
				targetMachines[i] = m.Name
			}

			// Make sure the status matches.
			Eventually(func() *clusterv1.MachineHealthCheckStatus {
				err := testEnv.Get(ctx, util.ObjectKey(mhc), mhc)
				if err != nil {
					return nil
				}
				return &mhc.Status
			}).Should(MatchMachineHealthCheckStatus(&clusterv1.MachineHealthCheckStatus{
				ExpectedMachines: 1,
				CurrentHealthy:   1,
				Targets:          targetMachines},
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
			Expect(testEnv.Status().Patch(ctx, node, nodePatch)).To(Succeed())

			// Make sure the status matches.
			Eventually(func() *clusterv1.MachineHealthCheckStatus {
				err := testEnv.Get(ctx, util.ObjectKey(mhc), mhc)
				if err != nil {
					return nil
				}
				return &mhc.Status
			}).Should(MatchMachineHealthCheckStatus(&clusterv1.MachineHealthCheckStatus{
				ExpectedMachines: 1,
				CurrentHealthy:   0,
				Targets:          targetMachines},
			))

			// Calculate how many Machines have health check succeeded = false.
			Eventually(func() (unhealthy int) {
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
			Eventually(func() (remediated int) {
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

		Specify("when in a MachineSet, unhealthy machines should be deleted", func() {

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
			Expect(testEnv.Create(ctx, infraTmpl)).To(Succeed())

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
								Data: pointer.StringPtr("test"),
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
			Expect(testEnv.Create(ctx, machineSet)).To(Succeed())

			// Calculate how many Machines have health check succeeded = false.
			Eventually(func() int {
				machines := &clusterv1.MachineList{}
				err := testEnv.List(ctx, machines, client.MatchingLabels{
					"selector": mhc.Spec.Selector.MatchLabels["selector"],
				})
				if err != nil {
					return -1
				}
				return len(machines.Items)
			}).Should(Equal(1))

			// Create the MachineHealthCheck instance.
			mhc.Spec.ClusterName = cluster.Name
			mhc.Spec.NodeStartupTimeout = &metav1.Duration{Duration: 1 * time.Second}
			Expect(testEnv.Create(ctx, mhc)).To(Succeed())

			// Pause the MachineSet reconciler.
			machineSetPatch := client.MergeFrom(machineSet.DeepCopy())
			machineSet.Annotations = map[string]string{
				clusterv1.PausedAnnotation: "",
			}
			Expect(testEnv.Patch(ctx, machineSet, machineSetPatch)).To(Succeed())

			// Calculate how many Machines should be remediated.
			var unhealthyMachine *clusterv1.Machine
			Eventually(func() (remediated int) {
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
			}).Should(Equal(1))

			// Unpause the MachineSet reconciler.
			machineSetPatch = client.MergeFrom(machineSet.DeepCopy())
			delete(machineSet.Annotations, clusterv1.PausedAnnotation)
			Expect(testEnv.Patch(ctx, machineSet, machineSetPatch)).To(Succeed())

			// Make sure the Machine gets deleted.
			Eventually(func() bool {
				machine := unhealthyMachine.DeepCopy()
				err := testEnv.Get(ctx, util.ObjectKey(unhealthyMachine), machine)
				return apierrors.IsNotFound(err) || !machine.DeletionTimestamp.IsZero()
			})
		})
	})
})

func TestClusterToMachineHealthCheck(t *testing.T) {
	_ = clusterv1.AddToScheme(scheme.Scheme)
	fakeClient := fake.NewFakeClient()

	r := &MachineHealthCheckReconciler{
		Log:    log.Log,
		Client: fakeClient,
	}

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
				gs.Eventually(getObj).Should(Succeed())
			}

			got := r.clusterToMachineHealthCheck(tc.object)
			gs.Expect(got).To(ConsistOf(tc.expected))
		})
	}
}

func newTestMachineHealthCheck(name, namespace, cluster string, labels map[string]string) *clusterv1.MachineHealthCheck {
	l := make(map[string]string, len(labels))
	for k, v := range labels {
		l[k] = v
	}
	l[clusterv1.ClusterLabelName] = cluster

	return &clusterv1.MachineHealthCheck{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    l,
		},
		Spec: clusterv1.MachineHealthCheckSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: l,
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
	_ = clusterv1.AddToScheme(scheme.Scheme)
	fakeClient := fake.NewFakeClient()

	r := &MachineHealthCheckReconciler{
		Log:    log.Log,
		Client: fakeClient,
	}

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
		Log:    log.Log,
		Client: fakeClient,
	}

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
			name:        "when a MachineHealthCheck exists for the Node, but not in the Machine's cluster",
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

func TestIsAllowedRemediation(t *testing.T) {
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

func ownerReferenceForCluster(ctx context.Context, c *clusterv1.Cluster) metav1.OwnerReference {
	// Fetch the cluster to populate the UID
	cc := &clusterv1.Cluster{}
	Expect(testEnv.GetClient().Get(ctx, util.ObjectKey(c), cc)).To(Succeed())

	return metav1.OwnerReference{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Cluster",
		Name:       cc.Name,
		UID:        cc.UID,
	}
}
