/*
Copyright 2017 The Kubernetes Authors.

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

package patch

import (
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Patch Helper", func() {

	It("Should patch an unstructured object", func() {
		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"kind":       "BootstrapMachine",
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha3",
				"metadata": map[string]interface{}{
					"generateName": "test-bootstrap-",
					"namespace":    "default",
				},
			},
		}

		Context("adding an owner reference, preserving its status", func() {
			obj := obj.DeepCopy()

			By("Creating the unstructured object")
			Expect(testEnv.Create(ctx, obj)).ToNot(HaveOccurred())
			key := client.ObjectKey{Name: obj.GetName(), Namespace: obj.GetNamespace()}
			defer func() {
				Expect(testEnv.Delete(ctx, obj)).To(Succeed())
			}()
			obj.Object["status"] = map[string]interface{}{
				"ready": true,
			}
			Expect(testEnv.Status().Update(ctx, obj)).To(Succeed())

			By("Creating a new patch helper")
			patcher, err := NewHelper(obj, testEnv)
			Expect(err).NotTo(HaveOccurred())

			By("Modifying the OwnerReferences")
			refs := []metav1.OwnerReference{
				{
					APIVersion: "cluster.x-k8s.io/v1alpha3",
					Kind:       "Cluster",
					Name:       "test",
					UID:        types.UID("fake-uid"),
				},
			}
			obj.SetOwnerReferences(refs)

			By("Patching the unstructured object")
			Expect(patcher.Patch(ctx, obj)).To(Succeed())

			By("Validating that the status has been preserved")
			ready, err := external.IsReady(obj)
			Expect(err).ToNot(HaveOccurred())
			Expect(ready).To(BeTrue())

			By("Validating the object has been updated")
			Eventually(func() bool {
				objAfter := obj.DeepCopy()
				if err := testEnv.Get(ctx, key, objAfter); err != nil {
					return false
				}

				return reflect.DeepEqual(obj.GetOwnerReferences(), objAfter.GetOwnerReferences())
			}, timeout).Should(BeTrue())
		})
	})

	Describe("Should patch conditions", func() {
		Specify("on a corev1.Node object", func() {
			conditionTime := metav1.Date(2015, 1, 1, 12, 0, 0, 0, metav1.Now().Location())

			obj := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "node-patch-test-",
					Annotations: map[string]string{
						"test": "1",
					},
				},
			}

			By("Creating a Node object")
			Expect(testEnv.Create(ctx, obj)).ToNot(HaveOccurred())
			key := client.ObjectKey{Name: obj.GetName()}
			defer func() {
				Expect(testEnv.Delete(ctx, obj)).To(Succeed())
			}()

			By("Creating a new patch helper")
			patcher, err := NewHelper(obj, testEnv)
			Expect(err).NotTo(HaveOccurred())

			By("Appending a new condition")
			condition := corev1.NodeCondition{
				Type:               "CustomCondition",
				Status:             corev1.ConditionTrue,
				LastHeartbeatTime:  conditionTime,
				LastTransitionTime: conditionTime,
				Reason:             "reason",
				Message:            "message",
			}
			obj.Status.Conditions = append(obj.Status.Conditions, condition)

			By("Patching the Node")
			Expect(patcher.Patch(ctx, obj)).To(Succeed())

			By("Validating the object has been updated")
			Eventually(func() bool {
				objAfter := obj.DeepCopy()
				Expect(testEnv.Get(ctx, key, objAfter)).To(Succeed())

				ok, _ := ContainElement(condition).Match(objAfter.Status.Conditions)
				return ok
			}, timeout).Should(BeTrue())
		})

		Describe("on a clusterv1.Cluster object", func() {
			obj := &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-",
					Namespace:    "default",
				},
			}

			Specify("should mark it ready", func() {
				obj := obj.DeepCopy()

				By("Creating the object")
				Expect(testEnv.Create(ctx, obj)).ToNot(HaveOccurred())
				key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}
				defer func() {
					Expect(testEnv.Delete(ctx, obj)).To(Succeed())
				}()

				By("Creating a new patch helper")
				patcher, err := NewHelper(obj, testEnv)
				Expect(err).NotTo(HaveOccurred())

				By("Marking Ready=True")
				conditions.MarkTrue(obj, clusterv1.ReadyCondition)

				By("Patching the object")
				Expect(patcher.Patch(ctx, obj)).To(Succeed())

				By("Validating the object has been updated")
				Eventually(func() bool {
					objAfter := obj.DeepCopy()
					if err := testEnv.Get(ctx, key, objAfter); err != nil {
						return false
					}
					return cmp.Equal(obj.Status.Conditions, objAfter.Status.Conditions)
				}, timeout).Should(BeTrue())
			})

			Specify("should recover if there is a resolvable conflict", func() {
				obj := obj.DeepCopy()

				By("Creating the object")
				Expect(testEnv.Create(ctx, obj)).ToNot(HaveOccurred())
				key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}
				defer func() {
					Expect(testEnv.Delete(ctx, obj)).To(Succeed())
				}()
				objCopy := obj.DeepCopy()

				By("Marking a custom condition to be false")
				conditions.MarkFalse(objCopy, clusterv1.ConditionType("TestCondition"), "reason", clusterv1.ConditionSeverityInfo, "message")
				Expect(testEnv.Status().Update(ctx, objCopy)).To(Succeed())

				By("Validating that the local object's resource version is behind")
				Expect(obj.ResourceVersion).ToNot(Equal(objCopy.ResourceVersion))

				By("Creating a new patch helper")
				patcher, err := NewHelper(obj, testEnv)
				Expect(err).NotTo(HaveOccurred())

				By("Marking Ready=True")
				conditions.MarkTrue(obj, clusterv1.ReadyCondition)

				By("Patching the object")
				Expect(patcher.Patch(ctx, obj)).To(Succeed())

				By("Validating the object has been updated")
				Eventually(func() bool {
					objAfter := obj.DeepCopy()
					if err := testEnv.Get(ctx, key, objAfter); err != nil {
						return false
					}

					testConditionCopy := conditions.Get(objCopy, "TestCondition")
					testConditionAfter := conditions.Get(objAfter, "TestCondition")

					readyBefore := conditions.Get(obj, clusterv1.ReadyCondition)
					readyAfter := conditions.Get(objAfter, clusterv1.ReadyCondition)

					return cmp.Equal(testConditionCopy, testConditionAfter) && cmp.Equal(readyBefore, readyAfter)
				}, timeout).Should(BeTrue())
			})

			Specify("should recover if there is a resolvable conflict, incl. patch spec and status", func() {
				obj := obj.DeepCopy()

				By("Creating the object")
				Expect(testEnv.Create(ctx, obj)).ToNot(HaveOccurred())
				key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}
				defer func() {
					Expect(testEnv.Delete(ctx, obj)).To(Succeed())
				}()
				objCopy := obj.DeepCopy()

				By("Marking a custom condition to be false")
				conditions.MarkFalse(objCopy, clusterv1.ConditionType("TestCondition"), "reason", clusterv1.ConditionSeverityInfo, "message")
				Expect(testEnv.Status().Update(ctx, objCopy)).To(Succeed())

				By("Validating that the local object's resource version is behind")
				Expect(obj.ResourceVersion).ToNot(Equal(objCopy.ResourceVersion))

				By("Creating a new patch helper")
				patcher, err := NewHelper(obj, testEnv)
				Expect(err).NotTo(HaveOccurred())

				By("Changing the object spec, status, and adding Ready=True condition")
				obj.Spec.Paused = true
				obj.Spec.ControlPlaneEndpoint.Host = "test://endpoint"
				obj.Spec.ControlPlaneEndpoint.Port = 8443
				obj.Status.Phase = "custom-phase"
				conditions.MarkTrue(obj, clusterv1.ReadyCondition)

				By("Patching the object")
				Expect(patcher.Patch(ctx, obj)).To(Succeed())

				By("Validating the object has been updated")
				objAfter := obj.DeepCopy()
				Eventually(func() bool {
					if err := testEnv.Get(ctx, key, objAfter); err != nil {
						return false
					}

					testConditionCopy := conditions.Get(objCopy, "TestCondition")
					testConditionAfter := conditions.Get(objAfter, "TestCondition")

					readyBefore := conditions.Get(obj, clusterv1.ReadyCondition)
					readyAfter := conditions.Get(objAfter, clusterv1.ReadyCondition)

					return cmp.Equal(testConditionCopy, testConditionAfter) && cmp.Equal(readyBefore, readyAfter) &&
						obj.Spec.Paused == objAfter.Spec.Paused &&
						obj.Spec.ControlPlaneEndpoint == objAfter.Spec.ControlPlaneEndpoint &&
						obj.Status.Phase == objAfter.Status.Phase
				}, timeout).Should(BeTrue(), cmp.Diff(obj, objAfter))
			})

			Specify("should return an error if there is an unresolvable conflict", func() {
				obj := obj.DeepCopy()

				By("Creating the object")
				Expect(testEnv.Create(ctx, obj)).ToNot(HaveOccurred())
				key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}
				defer func() {
					Expect(testEnv.Delete(ctx, obj)).To(Succeed())
				}()
				objCopy := obj.DeepCopy()

				By("Marking a custom condition to be false")
				conditions.MarkFalse(objCopy, clusterv1.ReadyCondition, "reason", clusterv1.ConditionSeverityInfo, "message")
				Expect(testEnv.Status().Update(ctx, objCopy)).To(Succeed())

				By("Validating that the local object's resource version is behind")
				Expect(obj.ResourceVersion).ToNot(Equal(objCopy.ResourceVersion))

				By("Creating a new patch helper")
				patcher, err := NewHelper(obj, testEnv)
				Expect(err).NotTo(HaveOccurred())

				By("Marking Ready=True")
				conditions.MarkTrue(obj, clusterv1.ReadyCondition)

				By("Patching the object")
				Expect(patcher.Patch(ctx, obj)).ToNot(Succeed())

				By("Validating the object has not been updated")
				Eventually(func() bool {
					objAfter := obj.DeepCopy()
					if err := testEnv.Get(ctx, key, objAfter); err != nil {
						return false
					}
					ok, _ := ContainElement(objCopy.Status.Conditions[0]).Match(objAfter.Status.Conditions)
					return ok
				}, timeout).Should(BeTrue())
			})

			Specify("should not return an error if there is an unresolvable conflict but the conditions is owned by the controller", func() {
				obj := obj.DeepCopy()

				By("Creating the object")
				Expect(testEnv.Create(ctx, obj)).ToNot(HaveOccurred())
				key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}
				defer func() {
					Expect(testEnv.Delete(ctx, obj)).To(Succeed())
				}()
				objCopy := obj.DeepCopy()

				By("Marking a custom condition to be false")
				conditions.MarkFalse(objCopy, clusterv1.ReadyCondition, "reason", clusterv1.ConditionSeverityInfo, "message")
				Expect(testEnv.Status().Update(ctx, objCopy)).To(Succeed())

				By("Validating that the local object's resource version is behind")
				Expect(obj.ResourceVersion).ToNot(Equal(objCopy.ResourceVersion))

				By("Creating a new patch helper")
				patcher, err := NewHelper(obj, testEnv)
				Expect(err).NotTo(HaveOccurred())

				By("Marking Ready=True")
				conditions.MarkTrue(obj, clusterv1.ReadyCondition)

				By("Patching the object")
				Expect(patcher.Patch(ctx, obj, WithOwnedConditions{Conditions: []clusterv1.ConditionType{clusterv1.ReadyCondition}})).To(Succeed())

				By("Validating the object has been updated")
				Eventually(func() bool {
					objAfter := obj.DeepCopy()
					if err := testEnv.Get(ctx, key, objAfter); err != nil {
						return false
					}

					readyBefore := conditions.Get(obj, clusterv1.ReadyCondition)
					readyAfter := conditions.Get(objAfter, clusterv1.ReadyCondition)

					return cmp.Equal(readyBefore, readyAfter)
				}, timeout).Should(BeTrue())
			})

			Specify("should not return an error if there is an unresolvable conflict when force overwrite is enabled", func() {
				obj := obj.DeepCopy()

				By("Creating the object")
				Expect(testEnv.Create(ctx, obj)).ToNot(HaveOccurred())
				key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}
				defer func() {
					Expect(testEnv.Delete(ctx, obj)).To(Succeed())
				}()
				objCopy := obj.DeepCopy()

				By("Marking a custom condition to be false")
				conditions.MarkFalse(objCopy, clusterv1.ReadyCondition, "reason", clusterv1.ConditionSeverityInfo, "message")
				Expect(testEnv.Status().Update(ctx, objCopy)).To(Succeed())

				By("Validating that the local object's resource version is behind")
				Expect(obj.ResourceVersion).ToNot(Equal(objCopy.ResourceVersion))

				By("Creating a new patch helper")
				patcher, err := NewHelper(obj, testEnv)
				Expect(err).NotTo(HaveOccurred())

				By("Marking Ready=True")
				conditions.MarkTrue(obj, clusterv1.ReadyCondition)

				By("Patching the object")
				Expect(patcher.Patch(ctx, obj, WithForceOverwriteConditions{})).To(Succeed())

				By("Validating the object has been updated")
				Eventually(func() bool {
					objAfter := obj.DeepCopy()
					if err := testEnv.Get(ctx, key, objAfter); err != nil {
						return false
					}

					readyBefore := conditions.Get(obj, clusterv1.ReadyCondition)
					readyAfter := conditions.Get(objAfter, clusterv1.ReadyCondition)

					return cmp.Equal(readyBefore, readyAfter)
				}, timeout).Should(BeTrue())
			})

		})
	})

	Describe("Should patch a clusterv1.Cluster", func() {
		obj := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
				Namespace:    "test-namespace",
			},
		}

		Specify("add a finalizers", func() {
			obj := obj.DeepCopy()

			By("Creating the object")
			Expect(testEnv.Create(ctx, obj)).ToNot(HaveOccurred())
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}
			defer func() {
				Expect(testEnv.Delete(ctx, obj)).To(Succeed())
			}()

			By("Creating a new patch helper")
			patcher, err := NewHelper(obj, testEnv)
			Expect(err).NotTo(HaveOccurred())

			By("Adding a finalizer")
			obj.Finalizers = append(obj.Finalizers, clusterv1.ClusterFinalizer)

			By("Patching the object")
			Expect(patcher.Patch(ctx, obj)).To(Succeed())

			By("Validating the object has been updated")
			Eventually(func() bool {
				objAfter := obj.DeepCopy()
				if err := testEnv.Get(ctx, key, objAfter); err != nil {
					return false
				}

				return reflect.DeepEqual(obj.Finalizers, objAfter.Finalizers)
			}, timeout).Should(BeTrue())
		})

		Specify("removing finalizers", func() {
			obj := obj.DeepCopy()
			obj.Finalizers = append(obj.Finalizers, clusterv1.ClusterFinalizer)

			By("Creating the object")
			Expect(testEnv.Create(ctx, obj)).ToNot(HaveOccurred())
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}
			defer func() {
				Expect(testEnv.Delete(ctx, obj)).To(Succeed())
			}()

			By("Creating a new patch helper")
			patcher, err := NewHelper(obj, testEnv)
			Expect(err).NotTo(HaveOccurred())

			By("Removing the finalizers")
			obj.SetFinalizers(nil)

			By("Patching the object")
			Expect(patcher.Patch(ctx, obj)).To(Succeed())

			By("Validating the object has been updated")
			Eventually(func() bool {
				objAfter := obj.DeepCopy()
				if err := testEnv.Get(ctx, key, objAfter); err != nil {
					return false
				}

				return len(objAfter.Finalizers) == 0
			}, timeout).Should(BeTrue())
		})

		Specify("updating spec", func() {
			obj := obj.DeepCopy()
			obj.ObjectMeta.Namespace = "test-namespace"

			By("Creating the object")
			Expect(testEnv.Create(ctx, obj)).ToNot(HaveOccurred())
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}
			defer func() {
				Expect(testEnv.Delete(ctx, obj)).To(Succeed())
			}()

			By("Creating a new patch helper")
			patcher, err := NewHelper(obj, testEnv)
			Expect(err).NotTo(HaveOccurred())

			By("Updating the object spec")
			obj.Spec.Paused = true
			obj.Spec.InfrastructureRef = &corev1.ObjectReference{
				Kind:      "test-kind",
				Name:      "test-ref",
				Namespace: "test-namespace",
			}

			By("Patching the object")
			Expect(patcher.Patch(ctx, obj)).To(Succeed())

			By("Validating the object has been updated")
			Eventually(func() bool {
				objAfter := obj.DeepCopy()
				if err := testEnv.Get(ctx, key, objAfter); err != nil {
					return false
				}

				return objAfter.Spec.Paused == true &&
					reflect.DeepEqual(obj.Spec.InfrastructureRef, objAfter.Spec.InfrastructureRef)
			}, timeout).Should(BeTrue())
		})

		Specify("updating status", func() {
			obj := obj.DeepCopy()

			By("Creating the object")
			Expect(testEnv.Create(ctx, obj)).ToNot(HaveOccurred())
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}
			defer func() {
				Expect(testEnv.Delete(ctx, obj)).To(Succeed())
			}()

			By("Creating a new patch helper")
			patcher, err := NewHelper(obj, testEnv)
			Expect(err).NotTo(HaveOccurred())

			By("Updating the object status")
			obj.Status.InfrastructureReady = true

			By("Patching the object")
			Expect(patcher.Patch(ctx, obj)).To(Succeed())

			By("Validating the object has been updated")
			Eventually(func() bool {
				objAfter := obj.DeepCopy()
				if err := testEnv.Get(ctx, key, objAfter); err != nil {
					return false
				}
				return reflect.DeepEqual(objAfter.Status, obj.Status)
			}, timeout).Should(BeTrue())
		})

		Specify("updating both spec, status, and adding a condition", func() {
			obj := obj.DeepCopy()
			obj.ObjectMeta.Namespace = "test-namespace"

			By("Creating the object")
			Expect(testEnv.Create(ctx, obj)).ToNot(HaveOccurred())
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}
			defer func() {
				Expect(testEnv.Delete(ctx, obj)).To(Succeed())
			}()

			By("Creating a new patch helper")
			patcher, err := NewHelper(obj, testEnv)
			Expect(err).NotTo(HaveOccurred())

			By("Updating the object spec")
			obj.Spec.Paused = true
			obj.Spec.InfrastructureRef = &corev1.ObjectReference{
				Kind:      "test-kind",
				Name:      "test-ref",
				Namespace: "test-namespace",
			}

			By("Updating the object status")
			obj.Status.InfrastructureReady = true

			By("Setting Ready condition")
			conditions.MarkTrue(obj, clusterv1.ReadyCondition)

			By("Patching the object")
			Expect(patcher.Patch(ctx, obj)).To(Succeed())

			By("Validating the object has been updated")
			Eventually(func() bool {
				objAfter := obj.DeepCopy()
				if err := testEnv.Get(ctx, key, objAfter); err != nil {
					return false
				}

				return obj.Status.InfrastructureReady == objAfter.Status.InfrastructureReady &&
					conditions.IsTrue(objAfter, clusterv1.ReadyCondition) &&
					reflect.DeepEqual(obj.Spec, objAfter.Spec)
			}, timeout).Should(BeTrue())
		})
	})

	It("Should update Status.ObservedGeneration when using WithStatusObservedGeneration option", func() {
		obj := &clusterv1.MachineSet{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-ms",
				Namespace:    "test-namespace",
			},
			Spec: clusterv1.MachineSetSpec{
				ClusterName: "test1",
				Template: clusterv1.MachineTemplateSpec{
					Spec: clusterv1.MachineSpec{
						ClusterName: "test1",
					},
				},
			},
		}

		Context("when updating spec", func() {
			obj := obj.DeepCopy()

			By("Creating the MachineSet object")
			Expect(testEnv.Create(ctx, obj)).ToNot(HaveOccurred())
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}
			defer func() {
				Expect(testEnv.Delete(ctx, obj)).To(Succeed())
			}()

			By("Creating a new patch helper")
			patcher, err := NewHelper(obj, testEnv)
			Expect(err).NotTo(HaveOccurred())

			By("Updating the object spec")
			obj.Spec.Replicas = pointer.Int32Ptr(10)

			By("Patching the object")
			Expect(patcher.Patch(ctx, obj, WithStatusObservedGeneration{})).To(Succeed())

			By("Validating the object has been updated")
			Eventually(func() bool {
				objAfter := obj.DeepCopy()
				if err := testEnv.Get(ctx, key, objAfter); err != nil {
					return false
				}

				return reflect.DeepEqual(obj.Spec, objAfter.Spec) &&
					obj.GetGeneration() == objAfter.Status.ObservedGeneration
			}, timeout).Should(BeTrue())
		})

		Context("when updating spec, status, and metadata", func() {
			obj := obj.DeepCopy()

			By("Creating the MachineSet object")
			Expect(testEnv.Create(ctx, obj)).ToNot(HaveOccurred())
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}
			defer func() {
				Expect(testEnv.Delete(ctx, obj)).To(Succeed())
			}()

			By("Creating a new patch helper")
			patcher, err := NewHelper(obj, testEnv)
			Expect(err).NotTo(HaveOccurred())

			By("Updating the object spec")
			obj.Spec.Replicas = pointer.Int32Ptr(10)

			By("Updating the object status")
			obj.Status.AvailableReplicas = 6
			obj.Status.ReadyReplicas = 6

			By("Updating the object metadata")
			obj.ObjectMeta.Annotations = map[string]string{
				"test1": "annotation",
			}

			By("Patching the object")
			Expect(patcher.Patch(ctx, obj, WithStatusObservedGeneration{})).To(Succeed())

			By("Validating the object has been updated")
			Eventually(func() bool {
				objAfter := obj.DeepCopy()
				if err := testEnv.Get(ctx, key, objAfter); err != nil {
					return false
				}

				return reflect.DeepEqual(obj.Spec, objAfter.Spec) &&
					reflect.DeepEqual(obj.Status, objAfter.Status) &&
					obj.GetGeneration() == objAfter.Status.ObservedGeneration
			}, timeout).Should(BeTrue())
		})

		Context("without any changes", func() {
			obj := obj.DeepCopy()

			By("Creating the MachineSet object")
			Expect(testEnv.Create(ctx, obj)).ToNot(HaveOccurred())
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}
			defer func() {
				Expect(testEnv.Delete(ctx, obj)).To(Succeed())
			}()
			obj.Status.ObservedGeneration = obj.GetGeneration()
			lastGeneration := obj.GetGeneration()
			Expect(testEnv.Status().Update(ctx, obj))

			By("Creating a new patch helper")
			patcher, err := NewHelper(obj, testEnv)
			Expect(err).NotTo(HaveOccurred())

			By("Patching the object")
			Expect(patcher.Patch(ctx, obj, WithStatusObservedGeneration{})).To(Succeed())

			By("Validating the object has been updated")
			Eventually(func() bool {
				objAfter := obj.DeepCopy()
				if err := testEnv.Get(ctx, key, objAfter); err != nil {
					return false
				}

				return lastGeneration == objAfter.Status.ObservedGeneration
			}, timeout).Should(BeTrue())
		})
	})
})

func TestNewHelperNil(t *testing.T) {
	var x *appsv1.Deployment
	g := NewWithT(t)
	_, err := NewHelper(x, nil)
	g.Expect(err).ToNot(BeNil())
	_, err = NewHelper(nil, nil)
	g.Expect(err).ToNot(BeNil())
}
