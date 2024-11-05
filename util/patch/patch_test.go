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
	"testing"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta2conditions "sigs.k8s.io/cluster-api/util/conditions/v1beta2"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestPatchHelper(t *testing.T) {
	ns, err := env.CreateNamespace(ctx, "test-patch-helper")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := env.Delete(ctx, ns); err != nil {
			t.Fatal(err)
		}
	}()

	t.Run("should patch an unstructured object", func(t *testing.T) {
		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"kind":       "GenericBootstrapConfig",
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1beta1",
				"metadata": map[string]interface{}{
					"generateName": "test-bootstrap-",
					"namespace":    ns.Name,
				},
			},
		}

		t.Run("adding an owner reference, preserving its status", func(t *testing.T) {
			g := NewWithT(t)

			t.Log("Creating the unstructured object")
			g.Expect(env.Create(ctx, obj)).To(Succeed())
			defer func() {
				g.Expect(env.Delete(ctx, obj)).To(Succeed())
			}()
			key := client.ObjectKey{Name: obj.GetName(), Namespace: obj.GetNamespace()}

			t.Log("Checking that the object has been created")
			g.Eventually(func() error {
				obj := obj.DeepCopy()
				return env.Get(ctx, key, obj)
			}).Should(Succeed())

			obj.Object["status"] = map[string]interface{}{
				"ready": true,
			}
			g.Expect(env.Status().Update(ctx, obj)).To(Succeed())

			t.Log("Creating a new patch helper")
			patcher, err := NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Modifying the OwnerReferences")
			refs := []metav1.OwnerReference{
				{
					APIVersion: "cluster.x-k8s.io/v1beta1",
					Kind:       "Cluster",
					Name:       "test",
					UID:        types.UID("fake-uid"),
				},
			}
			obj.SetOwnerReferences(refs)

			t.Log("Patching the unstructured object")
			g.Expect(patcher.Patch(ctx, obj)).To(Succeed())

			t.Log("Validating that the status has been preserved")
			ready, err := external.IsReady(obj)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(ready).To(BeTrue())

			t.Log("Validating the object has been updated")
			g.Eventually(func() bool {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return false
				}
				return cmp.Equal(obj.GetOwnerReferences(), objAfter.GetOwnerReferences())
			}, timeout).Should(BeTrue())
		})
	})

	t.Run("Should patch conditions", func(t *testing.T) {
		t.Run("on a corev1.Node object", func(t *testing.T) {
			g := NewWithT(t)

			conditionTime := metav1.Date(2015, 1, 1, 12, 0, 0, 0, metav1.Now().Location())
			obj := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "node-patch-test-",
					Namespace:    ns.Name,
					Annotations: map[string]string{
						"test": "1",
					},
				},
			}

			t.Log("Creating a Node object")
			g.Expect(env.Create(ctx, obj)).To(Succeed())
			defer func() {
				g.Expect(env.Delete(ctx, obj)).To(Succeed())
			}()
			key := client.ObjectKey{Name: obj.GetName()}

			t.Log("Checking that the object has been created")
			g.Eventually(func() error {
				obj := obj.DeepCopy()
				return env.Get(ctx, key, obj)
			}).Should(Succeed())

			t.Log("Creating a new patch helper")
			patcher, err := NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Appending a new condition")
			condition := corev1.NodeCondition{
				Type:               "CustomCondition",
				Status:             corev1.ConditionTrue,
				LastHeartbeatTime:  conditionTime,
				LastTransitionTime: conditionTime,
				Reason:             "reason",
				Message:            "message",
			}
			obj.Status.Conditions = append(obj.Status.Conditions, condition)

			t.Log("Patching the Node")
			g.Expect(patcher.Patch(ctx, obj)).To(Succeed())

			t.Log("Validating the object has been updated")
			g.Eventually(func() bool {
				objAfter := obj.DeepCopy()
				g.Expect(env.Get(ctx, key, objAfter)).To(Succeed())

				ok, _ := ContainElement(condition).Match(objAfter.Status.Conditions)
				return ok
			}, timeout).Should(BeTrue())
		})

		t.Run("on a clusterv1.Cluster object", func(t *testing.T) {
			obj := &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-",
					Namespace:    ns.Name,
				},
			}

			t.Run("should mark it ready", func(t *testing.T) {
				g := NewWithT(t)

				obj := obj.DeepCopy()

				t.Log("Creating the object")
				g.Expect(env.Create(ctx, obj)).To(Succeed())
				defer func() {
					g.Expect(env.Delete(ctx, obj)).To(Succeed())
				}()
				key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

				t.Log("Checking that the object has been created")
				g.Eventually(func() error {
					obj := obj.DeepCopy()
					return env.Get(ctx, key, obj)
				}).Should(Succeed())

				t.Log("Creating a new patch helper")
				patcher, err := NewHelper(obj, env)
				g.Expect(err).ToNot(HaveOccurred())

				t.Log("Marking Ready=True")
				conditions.MarkTrue(obj, clusterv1.ReadyCondition)

				t.Log("Patching the object")
				g.Expect(patcher.Patch(ctx, obj)).To(Succeed())

				t.Log("Validating the object has been updated")
				g.Eventually(func() clusterv1.Conditions {
					objAfter := obj.DeepCopy()
					if err := env.Get(ctx, key, objAfter); err != nil {
						return clusterv1.Conditions{}
					}
					return objAfter.Status.Conditions
				}, timeout).Should(conditions.MatchConditions(obj.Status.Conditions))
			})

			t.Run("should recover if there is a resolvable conflict", func(t *testing.T) {
				g := NewWithT(t)

				obj := obj.DeepCopy()

				t.Log("Creating the object")
				g.Expect(env.Create(ctx, obj)).To(Succeed())
				defer func() {
					g.Expect(env.Delete(ctx, obj)).To(Succeed())
				}()
				key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

				t.Log("Checking that the object has been created")
				g.Eventually(func() error {
					obj := obj.DeepCopy()
					return env.Get(ctx, key, obj)
				}).Should(Succeed())

				objCopy := obj.DeepCopy()

				t.Log("Marking TestCondition=False")
				conditions.MarkFalse(objCopy, clusterv1.ConditionType("TestCondition"), "reason", clusterv1.ConditionSeverityInfo, "message")
				g.Expect(env.Status().Update(ctx, objCopy)).To(Succeed())

				t.Log("Validating that the local object's resource version is behind")
				g.Expect(obj.ResourceVersion).NotTo(Equal(objCopy.ResourceVersion))

				t.Log("Creating a new patch helper")
				patcher, err := NewHelper(obj, env)
				g.Expect(err).ToNot(HaveOccurred())

				t.Log("Marking Ready=True")
				conditions.MarkTrue(obj, clusterv1.ReadyCondition)

				t.Log("Patching the object")
				g.Expect(patcher.Patch(ctx, obj)).To(Succeed())

				t.Log("Validating the object has been updated")
				g.Eventually(func() bool {
					objAfter := obj.DeepCopy()
					if err := env.Get(ctx, key, objAfter); err != nil {
						return false
					}

					testConditionCopy := conditions.Get(objCopy, "TestCondition")
					testConditionAfter := conditions.Get(objAfter, "TestCondition")
					if testConditionCopy == nil || testConditionAfter == nil {
						return false
					}
					ok, err := conditions.MatchCondition(*testConditionCopy).Match(*testConditionAfter)
					if err != nil || !ok {
						return false
					}

					readyBefore := conditions.Get(obj, clusterv1.ReadyCondition)
					readyAfter := conditions.Get(objAfter, clusterv1.ReadyCondition)
					if readyBefore == nil || readyAfter == nil {
						return false
					}
					ok, err = conditions.MatchCondition(*readyBefore).Match(*readyAfter)
					if err != nil || !ok {
						return false
					}

					return true
				}, timeout).Should(BeTrue())
			})

			t.Run("should recover if there is a resolvable conflict, incl. patch spec and status", func(t *testing.T) {
				g := NewWithT(t)

				obj := obj.DeepCopy()

				t.Log("Creating the object")
				g.Expect(env.Create(ctx, obj)).To(Succeed())
				defer func() {
					g.Expect(env.Delete(ctx, obj)).To(Succeed())
				}()
				key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

				t.Log("Checking that the object has been created")
				g.Eventually(func() error {
					obj := obj.DeepCopy()
					return env.Get(ctx, key, obj)
				}).Should(Succeed())

				objCopy := obj.DeepCopy()

				t.Log("Marking TestCondition=False")
				conditions.MarkFalse(objCopy, clusterv1.ConditionType("TestCondition"), "reason", clusterv1.ConditionSeverityInfo, "message")
				g.Expect(env.Status().Update(ctx, objCopy)).To(Succeed())

				t.Log("Validating that the local object's resource version is behind")
				g.Expect(obj.ResourceVersion).NotTo(Equal(objCopy.ResourceVersion))

				t.Log("Creating a new patch helper")
				patcher, err := NewHelper(obj, env)
				g.Expect(err).ToNot(HaveOccurred())

				t.Log("Changing the object spec, status, and adding Ready=True condition")
				obj.Spec.Paused = true
				obj.Spec.ControlPlaneEndpoint.Host = "test://endpoint"
				obj.Spec.ControlPlaneEndpoint.Port = 8443
				obj.Status.Phase = "custom-phase"
				conditions.MarkTrue(obj, clusterv1.ReadyCondition)

				t.Log("Patching the object")
				g.Expect(patcher.Patch(ctx, obj)).To(Succeed())

				t.Log("Validating the object has been updated")
				objAfter := obj.DeepCopy()
				g.Eventually(func() bool {
					if err := env.Get(ctx, key, objAfter); err != nil {
						return false
					}

					testConditionCopy := conditions.Get(objCopy, "TestCondition")
					testConditionAfter := conditions.Get(objAfter, "TestCondition")
					if testConditionCopy == nil || testConditionAfter == nil {
						return false
					}
					ok, err := conditions.MatchCondition(*testConditionCopy).Match(*testConditionAfter)
					if err != nil || !ok {
						return false
					}

					readyBefore := conditions.Get(obj, clusterv1.ReadyCondition)
					readyAfter := conditions.Get(objAfter, clusterv1.ReadyCondition)
					if readyBefore == nil || readyAfter == nil {
						return false
					}
					ok, err = conditions.MatchCondition(*readyBefore).Match(*readyAfter)
					if err != nil || !ok {
						return false
					}

					return obj.Spec.Paused == objAfter.Spec.Paused &&
						obj.Spec.ControlPlaneEndpoint == objAfter.Spec.ControlPlaneEndpoint &&
						obj.Status.Phase == objAfter.Status.Phase
				}, timeout).Should(BeTrue(), cmp.Diff(obj, objAfter))
			})

			t.Run("should return an error if there is an unresolvable conflict", func(t *testing.T) {
				g := NewWithT(t)

				obj := obj.DeepCopy()

				t.Log("Creating the object")
				g.Expect(env.Create(ctx, obj)).To(Succeed())
				defer func() {
					g.Expect(env.Delete(ctx, obj)).To(Succeed())
				}()
				key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

				t.Log("Checking that the object has been created")
				g.Eventually(func() error {
					obj := obj.DeepCopy()
					return env.Get(ctx, key, obj)
				}).Should(Succeed())

				objCopy := obj.DeepCopy()

				t.Log("Marking Ready=False")
				conditions.MarkFalse(objCopy, clusterv1.ReadyCondition, "reason", clusterv1.ConditionSeverityInfo, "message")
				g.Expect(env.Status().Update(ctx, objCopy)).To(Succeed())

				t.Log("Validating that the local object's resource version is behind")
				g.Expect(obj.ResourceVersion).NotTo(Equal(objCopy.ResourceVersion))

				t.Log("Creating a new patch helper")
				patcher, err := NewHelper(obj, env)
				g.Expect(err).ToNot(HaveOccurred())

				t.Log("Marking Ready=True")
				conditions.MarkTrue(obj, clusterv1.ReadyCondition)

				t.Log("Patching the object")
				g.Expect(patcher.Patch(ctx, obj)).NotTo(Succeed())

				t.Log("Validating the object has not been updated")
				g.Eventually(func() bool {
					objAfter := obj.DeepCopy()
					if err := env.Get(ctx, key, objAfter); err != nil {
						return false
					}

					for _, afterCondition := range objAfter.Status.Conditions {
						ok, err := conditions.MatchCondition(objCopy.Status.Conditions[0]).Match(afterCondition)
						if err == nil && ok {
							return true
						}
					}

					return false
				}, timeout).Should(BeTrue())
			})

			t.Run("should not return an error if there is an unresolvable conflict but the conditions is owned by the controller", func(t *testing.T) {
				g := NewWithT(t)

				obj := obj.DeepCopy()

				t.Log("Creating the object")
				g.Expect(env.Create(ctx, obj)).To(Succeed())
				defer func() {
					g.Expect(env.Delete(ctx, obj)).To(Succeed())
				}()
				key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

				t.Log("Checking that the object has been created")
				g.Eventually(func() error {
					obj := obj.DeepCopy()
					return env.Get(ctx, key, obj)
				}).Should(Succeed())

				objCopy := obj.DeepCopy()

				t.Log("Marking Ready=False")
				conditions.MarkFalse(objCopy, clusterv1.ReadyCondition, "reason", clusterv1.ConditionSeverityInfo, "message")
				g.Expect(env.Status().Update(ctx, objCopy)).To(Succeed())

				t.Log("Validating that the local object's resource version is behind")
				g.Expect(obj.ResourceVersion).NotTo(Equal(objCopy.ResourceVersion))

				t.Log("Creating a new patch helper")
				patcher, err := NewHelper(obj, env)
				g.Expect(err).ToNot(HaveOccurred())

				t.Log("Marking Ready=True")
				conditions.MarkTrue(obj, clusterv1.ReadyCondition)

				t.Log("Patching the object")
				g.Expect(patcher.Patch(ctx, obj, WithOwnedConditions{Conditions: []clusterv1.ConditionType{clusterv1.ReadyCondition}})).To(Succeed())

				t.Log("Validating the object has been updated")
				readyBefore := conditions.Get(obj, clusterv1.ReadyCondition)
				g.Eventually(func() clusterv1.Condition {
					objAfter := obj.DeepCopy()
					if err := env.Get(ctx, key, objAfter); err != nil {
						return clusterv1.Condition{}
					}

					return *conditions.Get(objAfter, clusterv1.ReadyCondition)
				}, timeout).Should(conditions.MatchCondition(*readyBefore))
			})

			t.Run("should not return an error if there is an unresolvable conflict when force overwrite is enabled", func(t *testing.T) {
				g := NewWithT(t)

				obj := obj.DeepCopy()

				t.Log("Creating the object")
				g.Expect(env.Create(ctx, obj)).To(Succeed())
				defer func() {
					g.Expect(env.Delete(ctx, obj)).To(Succeed())
				}()
				key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

				t.Log("Checking that the object has been created")
				g.Eventually(func() error {
					obj := obj.DeepCopy()
					return env.Get(ctx, key, obj)
				}).Should(Succeed())

				objCopy := obj.DeepCopy()

				t.Log("Marking Ready=False")
				conditions.MarkFalse(objCopy, clusterv1.ReadyCondition, "reason", clusterv1.ConditionSeverityInfo, "message")
				g.Expect(env.Status().Update(ctx, objCopy)).To(Succeed())

				t.Log("Validating that the local object's resource version is behind")
				g.Expect(obj.ResourceVersion).NotTo(Equal(objCopy.ResourceVersion))

				t.Log("Creating a new patch helper")
				patcher, err := NewHelper(obj, env)
				g.Expect(err).ToNot(HaveOccurred())

				t.Log("Marking Ready=True")
				conditions.MarkTrue(obj, clusterv1.ReadyCondition)

				t.Log("Patching the object")
				g.Expect(patcher.Patch(ctx, obj, WithForceOverwriteConditions{})).To(Succeed())

				t.Log("Validating the object has been updated")
				readyBefore := conditions.Get(obj, clusterv1.ReadyCondition)
				g.Eventually(func() clusterv1.Condition {
					objAfter := obj.DeepCopy()
					if err := env.Get(ctx, key, objAfter); err != nil {
						return clusterv1.Condition{}
					}

					return *conditions.Get(objAfter, clusterv1.ReadyCondition)
				}, timeout).Should(conditions.MatchCondition(*readyBefore))
			})
		})
	})

	t.Run("Should patch a clusterv1.Cluster", func(t *testing.T) {
		obj := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
				Namespace:    ns.Name,
			},
		}

		t.Run("add a finalizer", func(t *testing.T) {
			g := NewWithT(t)

			obj := obj.DeepCopy()

			t.Log("Creating the object")
			g.Expect(env.Create(ctx, obj)).To(Succeed())
			defer func() {
				g.Expect(env.Delete(ctx, obj)).To(Succeed())
			}()
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

			t.Log("Checking that the object has been created")
			g.Eventually(func() error {
				obj := obj.DeepCopy()
				return env.Get(ctx, key, obj)
			}).Should(Succeed())

			t.Log("Creating a new patch helper")
			patcher, err := NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Adding a finalizer")
			obj.Finalizers = append(obj.Finalizers, clusterv1.ClusterFinalizer)

			t.Log("Patching the object")
			g.Expect(patcher.Patch(ctx, obj)).To(Succeed())

			t.Log("Validating the object has been updated")
			g.Eventually(func() bool {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return false
				}

				return cmp.Equal(obj.Finalizers, objAfter.Finalizers)
			}, timeout).Should(BeTrue())
		})

		t.Run("removing finalizers", func(t *testing.T) {
			g := NewWithT(t)

			obj := obj.DeepCopy()
			obj.Finalizers = append(obj.Finalizers, clusterv1.ClusterFinalizer)

			t.Log("Creating the object")
			g.Expect(env.Create(ctx, obj)).To(Succeed())
			defer func() {
				g.Expect(env.Delete(ctx, obj)).To(Succeed())
			}()
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

			t.Log("Checking that the object has been created")
			g.Eventually(func() error {
				obj := obj.DeepCopy()
				return env.Get(ctx, key, obj)
			}).Should(Succeed())

			t.Log("Creating a new patch helper")
			patcher, err := NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Removing the finalizers")
			obj.SetFinalizers(nil)

			t.Log("Patching the object")
			g.Expect(patcher.Patch(ctx, obj)).To(Succeed())

			t.Log("Validating the object has been updated")
			g.Eventually(func() bool {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return false
				}

				return len(objAfter.Finalizers) == 0
			}, timeout).Should(BeTrue())
		})

		t.Run("updating spec", func(t *testing.T) {
			g := NewWithT(t)

			obj := obj.DeepCopy()

			t.Log("Creating the object")
			g.Expect(env.Create(ctx, obj)).To(Succeed())
			defer func() {
				g.Expect(env.Delete(ctx, obj)).To(Succeed())
			}()
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

			t.Log("Checking that the object has been created")
			g.Eventually(func() error {
				obj := obj.DeepCopy()
				return env.Get(ctx, key, obj)
			}).Should(Succeed())

			t.Log("Creating a new patch helper")
			patcher, err := NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Updating the object spec")
			obj.Spec.Paused = true
			obj.Spec.InfrastructureRef = &corev1.ObjectReference{
				Kind:      "test-kind",
				Name:      "test-ref",
				Namespace: ns.Name,
			}

			t.Log("Patching the object")
			g.Expect(patcher.Patch(ctx, obj)).To(Succeed())

			t.Log("Validating the object has been updated")
			g.Eventually(func() bool {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return false
				}

				return objAfter.Spec.Paused &&
					cmp.Equal(obj.Spec.InfrastructureRef, objAfter.Spec.InfrastructureRef)
			}, timeout).Should(BeTrue())
		})

		t.Run("updating status", func(t *testing.T) {
			g := NewWithT(t)

			obj := obj.DeepCopy()

			t.Log("Creating the object")
			g.Expect(env.Create(ctx, obj)).To(Succeed())
			defer func() {
				g.Expect(env.Delete(ctx, obj)).To(Succeed())
			}()
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

			t.Log("Checking that the object has been created")
			g.Eventually(func() error {
				obj := obj.DeepCopy()
				return env.Get(ctx, key, obj)
			}).Should(Succeed())

			t.Log("Creating a new patch helper")
			patcher, err := NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Updating the object status")
			obj.Status.InfrastructureReady = true

			t.Log("Patching the object")
			g.Expect(patcher.Patch(ctx, obj)).To(Succeed())

			t.Log("Validating the object has been updated")
			g.Eventually(func() bool {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return false
				}
				return cmp.Equal(objAfter.Status, obj.Status)
			}, timeout).Should(BeTrue())
		})

		t.Run("updating both spec, status, and adding a condition", func(t *testing.T) {
			g := NewWithT(t)

			obj := obj.DeepCopy()

			t.Log("Creating the object")
			g.Expect(env.Create(ctx, obj)).To(Succeed())
			defer func() {
				g.Expect(env.Delete(ctx, obj)).To(Succeed())
			}()
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

			t.Log("Checking that the object has been created")
			g.Eventually(func() error {
				obj := obj.DeepCopy()
				return env.Get(ctx, key, obj)
			}).Should(Succeed())

			t.Log("Creating a new patch helper")
			patcher, err := NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Updating the object spec")
			obj.Spec.Paused = true
			obj.Spec.InfrastructureRef = &corev1.ObjectReference{
				Kind:      "test-kind",
				Name:      "test-ref",
				Namespace: ns.Name,
			}

			t.Log("Updating the object status")
			obj.Status.InfrastructureReady = true

			t.Log("Setting Ready condition")
			conditions.MarkTrue(obj, clusterv1.ReadyCondition)

			t.Log("Patching the object")
			g.Expect(patcher.Patch(ctx, obj)).To(Succeed())

			t.Log("Validating the object has been updated")
			g.Eventually(func() bool {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return false
				}

				return obj.Status.InfrastructureReady == objAfter.Status.InfrastructureReady &&
					conditions.IsTrue(objAfter, clusterv1.ReadyCondition) &&
					cmp.Equal(obj.Spec, objAfter.Spec)
			}, timeout).Should(BeTrue())
		})
	})

	t.Run("should patch a corev1.ConfigMap object", func(t *testing.T) {
		g := NewWithT(t)

		obj := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "node-patch-test-",
				Namespace:    ns.Name,
				Annotations: map[string]string{
					"test": "1",
				},
			},
			Data: map[string]string{
				"1": "value",
			},
		}

		t.Log("Creating a ConfigMap object")
		g.Expect(env.Create(ctx, obj)).To(Succeed())
		defer func() {
			g.Expect(env.Delete(ctx, obj)).To(Succeed())
		}()
		key := util.ObjectKey(obj)

		t.Log("Checking that the object has been created")
		g.Eventually(func() error {
			obj := obj.DeepCopy()
			return env.Get(ctx, key, obj)
		}).Should(Succeed())

		t.Log("Creating a new patch helper")
		patcher, err := NewHelper(obj, env)
		g.Expect(err).ToNot(HaveOccurred())

		t.Log("Adding a new Data value")
		obj.Data["1"] = "value1"
		obj.Data["2"] = "value2"

		t.Log("Patching the ConfigMap")
		g.Expect(patcher.Patch(ctx, obj)).To(Succeed())

		t.Log("Validating the object has been updated")
		objAfter := &corev1.ConfigMap{}
		g.Eventually(func() bool {
			g.Expect(env.Get(ctx, key, objAfter)).To(Succeed())
			return len(objAfter.Data) == 2
		}, timeout).Should(BeTrue())
		g.Expect(objAfter.Data["1"]).To(Equal("value1"))
		g.Expect(objAfter.Data["2"]).To(Equal("value2"))
	})

	t.Run("Should update Status.ObservedGeneration when using WithStatusObservedGeneration option", func(t *testing.T) {
		obj := &clusterv1.MachineSet{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-ms",
				Namespace:    ns.Name,
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

		t.Run("when updating spec", func(t *testing.T) {
			g := NewWithT(t)

			obj := obj.DeepCopy()

			t.Log("Creating the MachineSet object")
			g.Expect(env.Create(ctx, obj)).To(Succeed())
			defer func() {
				g.Expect(env.Delete(ctx, obj)).To(Succeed())
			}()
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

			t.Log("Checking that the object has been created")
			g.Eventually(func() error {
				obj := obj.DeepCopy()
				return env.Get(ctx, key, obj)
			}).Should(Succeed())

			t.Log("Creating a new patch helper")
			patcher, err := NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Updating the object spec")
			obj.Spec.Replicas = ptr.To[int32](10)

			t.Log("Patching the object")
			g.Expect(patcher.Patch(ctx, obj, WithStatusObservedGeneration{})).To(Succeed())

			t.Log("Validating the object has been updated")
			g.Eventually(func() bool {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return false
				}

				return cmp.Equal(obj.Spec, objAfter.Spec) &&
					obj.GetGeneration() == objAfter.Status.ObservedGeneration
			}, timeout).Should(BeTrue())
		})

		t.Run("when updating spec, status, and metadata", func(t *testing.T) {
			g := NewWithT(t)

			obj := obj.DeepCopy()

			t.Log("Creating the MachineSet object")
			g.Expect(env.Create(ctx, obj)).To(Succeed())
			defer func() {
				g.Expect(env.Delete(ctx, obj)).To(Succeed())
			}()
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

			t.Log("Checking that the object has been created")
			g.Eventually(func() error {
				obj := obj.DeepCopy()
				return env.Get(ctx, key, obj)
			}).Should(Succeed())

			t.Log("Creating a new patch helper")
			patcher, err := NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Updating the object spec")
			obj.Spec.Replicas = ptr.To[int32](10)

			t.Log("Updating the object status")
			obj.Status.AvailableReplicas = 6
			obj.Status.ReadyReplicas = 6

			t.Log("Updating the object metadata")
			obj.ObjectMeta.Annotations = map[string]string{
				"test1": "annotation",
			}

			t.Log("Patching the object")
			g.Expect(patcher.Patch(ctx, obj, WithStatusObservedGeneration{})).To(Succeed())

			t.Log("Validating the object has been updated")
			g.Eventually(func() bool {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return false
				}

				return cmp.Equal(obj.Spec, objAfter.Spec) &&
					cmp.Equal(obj.Status, objAfter.Status) &&
					obj.GetGeneration() == objAfter.Status.ObservedGeneration
			}, timeout).Should(BeTrue())
		})

		t.Run("without any changes", func(t *testing.T) {
			g := NewWithT(t)

			obj := obj.DeepCopy()

			t.Log("Creating the MachineSet object")
			g.Expect(env.Create(ctx, obj)).To(Succeed())
			defer func() {
				g.Expect(env.Delete(ctx, obj)).To(Succeed())
			}()
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

			t.Log("Checking that the object has been created")
			g.Eventually(func() error {
				obj := obj.DeepCopy()
				return env.Get(ctx, key, obj)
			}).Should(Succeed())

			obj.Status.ObservedGeneration = obj.GetGeneration()
			lastGeneration := obj.GetGeneration()
			g.Expect(env.Status().Update(ctx, obj)).To(Succeed())

			t.Log("Creating a new patch helper")
			patcher, err := NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Patching the object")
			g.Expect(patcher.Patch(ctx, obj, WithStatusObservedGeneration{})).To(Succeed())

			t.Log("Validating the object has been updated")
			g.Eventually(func() bool {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return false
				}
				return lastGeneration == objAfter.Status.ObservedGeneration
			}, timeout).Should(BeTrue())
		})
	})

	t.Run("Should error if the object isn't the same", func(t *testing.T) {
		g := NewWithT(t)

		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
				Namespace:    ns.Name,
			},
		}

		machineSet := &clusterv1.MachineSet{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-ms",
				Namespace:    ns.Name,
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

		g.Expect(env.Create(ctx, cluster)).To(Succeed())
		defer func() {
			g.Expect(env.Delete(ctx, cluster)).To(Succeed())
		}()
		g.Expect(env.Create(ctx, machineSet)).To(Succeed())
		defer func() {
			g.Expect(env.Delete(ctx, machineSet)).To(Succeed())
		}()

		patcher, err := NewHelper(cluster, env)
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(patcher.Patch(ctx, machineSet)).NotTo(Succeed())
	})

	t.Run("Should not error if there are no finalizers and deletion timestamp is not nil", func(t *testing.T) {
		g := NewWithT(t)
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-cluster",
				Namespace:  ns.Name,
				Finalizers: []string{"block-deletion"},
			},
			Status: clusterv1.ClusterStatus{},
		}
		key := client.ObjectKey{Name: cluster.GetName(), Namespace: cluster.GetNamespace()}
		g.Expect(env.Create(ctx, cluster)).To(Succeed())
		g.Expect(env.Delete(ctx, cluster)).To(Succeed())

		// Ensure cluster still exists & get Cluster with deletionTimestamp set
		// Note: Using the APIReader to ensure we get the cluster with deletionTimestamp is set.
		// This is realistic because finalizers are removed in reconcileDelete code and that is only
		// run if the deletionTimestamp is set.
		g.Expect(env.GetAPIReader().Get(ctx, key, cluster)).To(Succeed())

		// Patch helper will first remove the finalizer and then it will get a not found error when
		// trying to patch status. This test validates that the not found error is ignored.
		patcher, err := NewHelper(cluster, env)
		g.Expect(err).ToNot(HaveOccurred())
		cluster.Finalizers = []string{}
		cluster.Status.Phase = "Running"
		g.Expect(patcher.Patch(ctx, cluster)).To(Succeed())
	})
}

func TestNewHelperNil(t *testing.T) {
	var x *appsv1.Deployment
	g := NewWithT(t)
	_, err := NewHelper(x, nil)
	g.Expect(err).To(HaveOccurred())
	_, err = NewHelper(nil, nil)
	g.Expect(err).To(HaveOccurred())
}

func TestPatchHelperForV1beta2Transition(t *testing.T) {
	now := metav1.Now().Rfc3339Copy()

	t.Run("Should patch conditions on a v1beta1 object with conditions (phase 0)", func(t *testing.T) {
		obj := &builder.Phase0Obj{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
				Namespace:    metav1.NamespaceDefault,
			},
		}

		t.Run("should mark it ready", func(t *testing.T) {
			g := NewWithT(t)

			obj := obj.DeepCopy()

			t.Log("Creating the object")
			err := env.Create(ctx, obj)
			g.Expect(err).To(Succeed())
			defer func() {
				g.Expect(env.Delete(ctx, obj)).To(Succeed())
			}()
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

			t.Log("Checking that the object has been created")
			g.Eventually(func() error {
				obj := obj.DeepCopy()
				return env.Get(ctx, key, obj)
			}).Should(Succeed())

			t.Log("Creating a new patch helper")
			patcher, err := NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Marking Ready=True")
			conditions.MarkTrue(obj, clusterv1.ReadyCondition)

			t.Log("Patching the object")
			g.Expect(patcher.Patch(ctx, obj)).To(Succeed())

			t.Log("Validating the object has been updated")
			g.Eventually(func() clusterv1.Conditions {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return clusterv1.Conditions{}
				}
				return objAfter.Status.Conditions
			}, timeout).Should(conditions.MatchConditions(obj.Status.Conditions))
		})

		t.Run("should mark it ready when passing Clusterv1ConditionsFieldPath", func(t *testing.T) {
			g := NewWithT(t)

			obj := obj.DeepCopy()

			t.Log("Creating the object")
			err := env.Create(ctx, obj)
			g.Expect(err).To(Succeed())
			defer func() {
				g.Expect(env.Delete(ctx, obj)).To(Succeed())
			}()
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

			t.Log("Checking that the object has been created")
			g.Eventually(func() error {
				obj := obj.DeepCopy()
				return env.Get(ctx, key, obj)
			}).Should(Succeed())

			t.Log("Creating a new patch helper")
			patcher, err := NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Marking Ready=True")
			conditions.MarkTrue(obj, clusterv1.ReadyCondition)

			t.Log("Patching the object")
			g.Expect(patcher.Patch(ctx, obj, Clusterv1ConditionsFieldPath{"status", "conditions"})).To(Succeed())

			t.Log("Validating the object has been updated")
			g.Eventually(func() clusterv1.Conditions {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return clusterv1.Conditions{}
				}
				return objAfter.Status.Conditions
			}, timeout).Should(conditions.MatchConditions(obj.Status.Conditions))
		})

		t.Run("should recover if there is a resolvable conflict", func(t *testing.T) {
			g := NewWithT(t)

			obj := obj.DeepCopy()

			t.Log("Creating the object")
			g.Expect(env.Create(ctx, obj)).To(Succeed())
			defer func() {
				g.Expect(env.Delete(ctx, obj)).To(Succeed())
			}()
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

			t.Log("Checking that the object has been created")
			g.Eventually(func() error {
				obj := obj.DeepCopy()
				return env.Get(ctx, key, obj)
			}).Should(Succeed())

			objCopy := obj.DeepCopy()

			t.Log("Marking a custom condition to be false")
			conditions.MarkFalse(objCopy, clusterv1.ConditionType("TestCondition"), "reason", clusterv1.ConditionSeverityInfo, "message")
			g.Expect(env.Status().Update(ctx, objCopy)).To(Succeed())

			t.Log("Validating that the local object's resource version is behind")
			g.Expect(obj.ResourceVersion).NotTo(Equal(objCopy.ResourceVersion))

			t.Log("Creating a new patch helper")
			patcher, err := NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Marking Ready=True")
			conditions.MarkTrue(obj, clusterv1.ReadyCondition)

			t.Log("Patching the object")
			g.Expect(patcher.Patch(ctx, obj)).To(Succeed())

			t.Log("Validating the object has been updated")
			g.Eventually(func() bool {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return false
				}

				testConditionCopy := conditions.Get(objCopy, "TestCondition")
				testConditionAfter := conditions.Get(objAfter, "TestCondition")
				if testConditionCopy == nil || testConditionAfter == nil {
					return false
				}
				ok, err := conditions.MatchCondition(*testConditionCopy).Match(*testConditionAfter)
				if err != nil || !ok {
					return false
				}

				readyBefore := conditions.Get(obj, clusterv1.ReadyCondition)
				readyAfter := conditions.Get(objAfter, clusterv1.ReadyCondition)
				if readyBefore == nil || readyAfter == nil {
					return false
				}
				ok, err = conditions.MatchCondition(*readyBefore).Match(*readyAfter)
				if err != nil || !ok {
					return false
				}

				return true
			}, timeout).Should(BeTrue())
		})

		t.Run("should recover if there is a resolvable conflict, incl. patch spec and status", func(t *testing.T) {
			g := NewWithT(t)

			obj := obj.DeepCopy()

			t.Log("Creating the object")
			g.Expect(env.Create(ctx, obj)).To(Succeed())
			defer func() {
				g.Expect(env.Delete(ctx, obj)).To(Succeed())
			}()
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

			t.Log("Checking that the object has been created")
			g.Eventually(func() error {
				obj := obj.DeepCopy()
				return env.Get(ctx, key, obj)
			}).Should(Succeed())

			objCopy := obj.DeepCopy()

			t.Log("Marking a custom condition to be false")
			conditions.MarkFalse(objCopy, clusterv1.ConditionType("TestCondition"), "reason", clusterv1.ConditionSeverityInfo, "message")
			g.Expect(env.Status().Update(ctx, objCopy)).To(Succeed())

			t.Log("Validating that the local object's resource version is behind")
			g.Expect(obj.ResourceVersion).NotTo(Equal(objCopy.ResourceVersion))

			t.Log("Creating a new patch helper")
			patcher, err := NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Changing the object spec, status, and adding Ready=True condition")
			obj.Spec.Foo = "foo"
			obj.Status.Bar = "bat"
			conditions.MarkTrue(obj, clusterv1.ReadyCondition)

			t.Log("Patching the object")
			g.Expect(patcher.Patch(ctx, obj)).To(Succeed())

			t.Log("Validating the object has been updated")
			objAfter := obj.DeepCopy()
			g.Eventually(func() bool {
				if err := env.Get(ctx, key, objAfter); err != nil {
					return false
				}

				testConditionCopy := conditions.Get(objCopy, "TestCondition")
				testConditionAfter := conditions.Get(objAfter, "TestCondition")
				if testConditionCopy == nil || testConditionAfter == nil {
					return false
				}
				ok, err := conditions.MatchCondition(*testConditionCopy).Match(*testConditionAfter)
				if err != nil || !ok {
					return false
				}

				readyBefore := conditions.Get(obj, clusterv1.ReadyCondition)
				readyAfter := conditions.Get(objAfter, clusterv1.ReadyCondition)
				if readyBefore == nil || readyAfter == nil {
					return false
				}
				ok, err = conditions.MatchCondition(*readyBefore).Match(*readyAfter)
				if err != nil || !ok {
					return false
				}

				return obj.Spec.Foo == objAfter.Spec.Foo &&
					obj.Status.Bar == objAfter.Status.Bar
			}, timeout).Should(BeTrue(), cmp.Diff(obj, objAfter))
		})

		t.Run("should return an error if there is an unresolvable conflict", func(t *testing.T) {
			g := NewWithT(t)

			obj := obj.DeepCopy()

			t.Log("Creating the object")
			g.Expect(env.Create(ctx, obj)).To(Succeed())
			defer func() {
				g.Expect(env.Delete(ctx, obj)).To(Succeed())
			}()
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

			t.Log("Checking that the object has been created")
			g.Eventually(func() error {
				obj := obj.DeepCopy()
				return env.Get(ctx, key, obj)
			}).Should(Succeed())

			objCopy := obj.DeepCopy()

			t.Log("Marking Ready=False")
			conditions.MarkFalse(objCopy, clusterv1.ReadyCondition, "reason", clusterv1.ConditionSeverityInfo, "message")
			g.Expect(env.Status().Update(ctx, objCopy)).To(Succeed())

			t.Log("Validating that the local object's resource version is behind")
			g.Expect(obj.ResourceVersion).NotTo(Equal(objCopy.ResourceVersion))

			t.Log("Creating a new patch helper")
			patcher, err := NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Marking Ready=True")
			conditions.MarkTrue(obj, clusterv1.ReadyCondition)

			t.Log("Patching the object")
			g.Expect(patcher.Patch(ctx, obj)).NotTo(Succeed())

			t.Log("Validating the object has not been updated")
			g.Eventually(func() bool {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return false
				}

				for _, afterCondition := range objAfter.Status.Conditions {
					ok, err := conditions.MatchCondition(objCopy.Status.Conditions[0]).Match(afterCondition)
					if err == nil && ok {
						return true
					}
				}

				return false
			}, timeout).Should(BeTrue())
		})

		t.Run("should not return an error if there is an unresolvable conflict but the condition is owned by the controller", func(t *testing.T) {
			g := NewWithT(t)

			obj := obj.DeepCopy()

			t.Log("Creating the object")
			g.Expect(env.Create(ctx, obj)).To(Succeed())
			defer func() {
				g.Expect(env.Delete(ctx, obj)).To(Succeed())
			}()
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

			t.Log("Checking that the object has been created")
			g.Eventually(func() error {
				obj := obj.DeepCopy()
				return env.Get(ctx, key, obj)
			}).Should(Succeed())

			objCopy := obj.DeepCopy()

			t.Log("Marking Ready=False")
			conditions.MarkFalse(objCopy, clusterv1.ReadyCondition, "reason", clusterv1.ConditionSeverityInfo, "message")
			g.Expect(env.Status().Update(ctx, objCopy)).To(Succeed())

			t.Log("Validating that the local object's resource version is behind")
			g.Expect(obj.ResourceVersion).NotTo(Equal(objCopy.ResourceVersion))

			t.Log("Creating a new patch helper")
			patcher, err := NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Marking Ready=True")
			conditions.MarkTrue(obj, clusterv1.ReadyCondition)

			t.Log("Patching the object")
			g.Expect(patcher.Patch(ctx, obj, WithOwnedConditions{Conditions: []clusterv1.ConditionType{clusterv1.ReadyCondition}})).To(Succeed())

			t.Log("Validating the object has been updated")
			readyBefore := conditions.Get(obj, clusterv1.ReadyCondition)
			g.Eventually(func() clusterv1.Condition {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return clusterv1.Condition{}
				}

				return *conditions.Get(objAfter, clusterv1.ReadyCondition)
			}, timeout).Should(conditions.MatchCondition(*readyBefore))
		})

		t.Run("should not return an error if there is an unresolvable conflict when force overwrite is enabled", func(t *testing.T) {
			g := NewWithT(t)

			obj := obj.DeepCopy()

			t.Log("Creating the object")
			g.Expect(env.Create(ctx, obj)).To(Succeed())
			defer func() {
				g.Expect(env.Delete(ctx, obj)).To(Succeed())
			}()
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

			t.Log("Checking that the object has been created")
			g.Eventually(func() error {
				obj := obj.DeepCopy()
				return env.Get(ctx, key, obj)
			}).Should(Succeed())

			objCopy := obj.DeepCopy()

			t.Log("Marking Ready=False")
			conditions.MarkFalse(objCopy, clusterv1.ReadyCondition, "reason", clusterv1.ConditionSeverityInfo, "message")
			g.Expect(env.Status().Update(ctx, objCopy)).To(Succeed())

			t.Log("Validating that the local object's resource version is behind")
			g.Expect(obj.ResourceVersion).NotTo(Equal(objCopy.ResourceVersion))

			t.Log("Creating a new patch helper")
			patcher, err := NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Marking Ready=True")
			conditions.MarkTrue(obj, clusterv1.ReadyCondition)

			t.Log("Patching the object")
			g.Expect(patcher.Patch(ctx, obj, WithForceOverwriteConditions{})).To(Succeed())

			t.Log("Validating the object has been updated")
			readyBefore := conditions.Get(obj, clusterv1.ReadyCondition)
			g.Eventually(func() clusterv1.Condition {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return clusterv1.Condition{}
				}

				return *conditions.Get(objAfter, clusterv1.ReadyCondition)
			}, timeout).Should(conditions.MatchCondition(*readyBefore))
		})
	})

	t.Run("Should patch conditions on a v1beta1 object with both clusterv1.conditions and metav1.conditions (phase 1)", func(t *testing.T) {
		obj := &builder.Phase1Obj{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
				Namespace:    metav1.NamespaceDefault,
			},
		}

		t.Run("should mark it ready and sort conditions", func(t *testing.T) {
			g := NewWithT(t)

			obj := obj.DeepCopy()

			t.Log("Creating the object")
			err := env.Create(ctx, obj)
			g.Expect(err).To(Succeed())
			defer func() {
				g.Expect(env.Delete(ctx, obj)).To(Succeed())
			}()
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

			t.Log("Checking that the object has been created")
			g.Eventually(func() error {
				obj := obj.DeepCopy()
				return env.Get(ctx, key, obj)
			}).Should(Succeed())

			// Adding Ready first
			t.Log("Creating a new patch helper")
			patcher, err := NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Marking clusterv1.conditions and metav1.conditions Ready=True")
			conditions.MarkTrue(obj, clusterv1.ReadyCondition)
			v1beta2conditions.Set(obj, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "AllGood", LastTransitionTime: now})

			t.Log("Patching the object")
			g.Expect(patcher.Patch(ctx, obj)).To(Succeed())

			// Adding Available as a second condition, but it should be sorted as first
			t.Log("Creating a new patch helper")
			patcher, err = NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Marking metav1.conditions Available=True")
			v1beta2conditions.Set(obj, metav1.Condition{Type: "Available", Status: metav1.ConditionTrue, Reason: "AllGood", LastTransitionTime: now})

			t.Log("Patching the object")
			g.Expect(patcher.Patch(ctx, obj)).To(Succeed())

			t.Log("Validating the object has been updated")
			g.Eventually(func() clusterv1.Conditions {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return clusterv1.Conditions{}
				}
				return objAfter.Status.Conditions
			}, timeout).Should(conditions.MatchConditions(obj.Status.Conditions))
			g.Eventually(func() []metav1.Condition {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return nil
				}
				if len(objAfter.Status.V1Beta2.Conditions) != 2 {
					return nil
				}
				if objAfter.Status.V1Beta2.Conditions[0].Type != "Available" || objAfter.Status.V1Beta2.Conditions[1].Type != "Ready" {
					return nil
				}

				return objAfter.Status.V1Beta2.Conditions
			}, timeout).Should(v1beta2conditions.MatchConditions(obj.Status.V1Beta2.Conditions))
		})

		t.Run("should mark it ready when passing Clusterv1ConditionsFieldPath and Metav1ConditionsFieldPath", func(t *testing.T) {
			g := NewWithT(t)

			obj := obj.DeepCopy()

			t.Log("Creating the object")
			err := env.Create(ctx, obj)
			g.Expect(err).To(Succeed())
			defer func() {
				g.Expect(env.Delete(ctx, obj)).To(Succeed())
			}()
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

			t.Log("Checking that the object has been created")
			g.Eventually(func() error {
				obj := obj.DeepCopy()
				return env.Get(ctx, key, obj)
			}).Should(Succeed())

			t.Log("Creating a new patch helper")
			patcher, err := NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Marking clusterv1.conditions and metav1.conditions Ready=True")
			conditions.MarkTrue(obj, clusterv1.ReadyCondition)
			v1beta2conditions.Set(obj, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "AllGood", LastTransitionTime: now})

			t.Log("Patching the object")
			g.Expect(patcher.Patch(ctx, obj, Clusterv1ConditionsFieldPath{"status", "conditions"}, Metav1ConditionsFieldPath{"status", "v1beta2", "conditions"})).To(Succeed())

			t.Log("Validating the object has been updated")
			g.Eventually(func() clusterv1.Conditions {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return clusterv1.Conditions{}
				}
				return objAfter.Status.Conditions
			}, timeout).Should(conditions.MatchConditions(obj.Status.Conditions))
			g.Eventually(func() []metav1.Condition {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return nil
				}
				return objAfter.Status.V1Beta2.Conditions
			}, timeout).Should(v1beta2conditions.MatchConditions(obj.Status.V1Beta2.Conditions))
		})

		t.Run("should recover if there is a resolvable conflict", func(t *testing.T) {
			g := NewWithT(t)

			obj := obj.DeepCopy()

			t.Log("Creating the object")
			g.Expect(env.Create(ctx, obj)).To(Succeed())
			defer func() {
				g.Expect(env.Delete(ctx, obj)).To(Succeed())
			}()
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

			t.Log("Checking that the object has been created")
			g.Eventually(func() error {
				obj := obj.DeepCopy()
				return env.Get(ctx, key, obj)
			}).Should(Succeed())

			objCopy := obj.DeepCopy()

			t.Log("Marking clusterv1.conditions and metav1.conditions Test=False")
			conditions.MarkFalse(objCopy, clusterv1.ConditionType("Test"), "reason", clusterv1.ConditionSeverityInfo, "message")
			v1beta2conditions.Set(objCopy, metav1.Condition{Type: "Test", Status: metav1.ConditionFalse, Reason: "reason", Message: "message", LastTransitionTime: now})
			g.Expect(env.Status().Update(ctx, objCopy)).To(Succeed())

			t.Log("Validating that the local object's resource version is behind")
			g.Expect(obj.ResourceVersion).NotTo(Equal(objCopy.ResourceVersion))

			t.Log("Creating a new patch helper")
			patcher, err := NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Marking clusterv1.conditions and metav1.conditions Ready=True")
			conditions.MarkTrue(obj, clusterv1.ReadyCondition)
			v1beta2conditions.Set(obj, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "AllGood", LastTransitionTime: now})

			t.Log("Patching the object")
			g.Expect(patcher.Patch(ctx, obj)).To(Succeed())

			t.Log("Validating the object has been updated")
			g.Eventually(func() bool {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return false
				}

				testConditionCopy := conditions.Get(objCopy, "Test")
				testConditionAfter := conditions.Get(objAfter, "Test")
				if testConditionCopy == nil || testConditionAfter == nil {
					return false
				}
				ok, err := conditions.MatchCondition(*testConditionCopy).Match(*testConditionAfter)
				if err != nil || !ok {
					return false
				}

				readyBefore := conditions.Get(obj, clusterv1.ReadyCondition)
				readyAfter := conditions.Get(objAfter, clusterv1.ReadyCondition)
				if readyBefore == nil || readyAfter == nil {
					return false
				}
				ok, err = conditions.MatchCondition(*readyBefore).Match(*readyAfter)
				if err != nil || !ok {
					return false
				}

				testV1Beta2ConditionCopy := v1beta2conditions.Get(objCopy, "Test")
				testV1Beta2ConditionAfter := v1beta2conditions.Get(objAfter, "Test")
				if testV1Beta2ConditionCopy == nil || testV1Beta2ConditionAfter == nil {
					return false
				}
				ok, err = v1beta2conditions.MatchCondition(*testV1Beta2ConditionCopy).Match(*testV1Beta2ConditionAfter)
				if err != nil || !ok {
					return false
				}

				readyV1Beta2Before := v1beta2conditions.Get(obj, "Ready")
				readyV1Beta2After := v1beta2conditions.Get(objAfter, "Ready")
				if readyV1Beta2Before == nil || readyV1Beta2After == nil {
					return false
				}
				ok, err = v1beta2conditions.MatchCondition(*readyV1Beta2Before).Match(*readyV1Beta2After)
				if err != nil || !ok {
					return false
				}

				return true
			}, timeout).Should(BeTrue())
		})

		t.Run("should recover if there is a resolvable conflict, incl. patch spec and status", func(t *testing.T) {
			g := NewWithT(t)

			obj := obj.DeepCopy()

			t.Log("Creating the object")
			g.Expect(env.Create(ctx, obj)).To(Succeed())
			defer func() {
				g.Expect(env.Delete(ctx, obj)).To(Succeed())
			}()
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

			t.Log("Checking that the object has been created")
			g.Eventually(func() error {
				obj := obj.DeepCopy()
				return env.Get(ctx, key, obj)
			}).Should(Succeed())

			objCopy := obj.DeepCopy()

			t.Log("Marking clusterv1.conditions and metav1.conditions Test=False")
			conditions.MarkFalse(objCopy, clusterv1.ConditionType("Test"), "reason", clusterv1.ConditionSeverityInfo, "message")
			v1beta2conditions.Set(objCopy, metav1.Condition{Type: "Test", Status: metav1.ConditionFalse, Reason: "reason", Message: "message", LastTransitionTime: now})
			g.Expect(env.Status().Update(ctx, objCopy)).To(Succeed())

			t.Log("Validating that the local object's resource version is behind")
			g.Expect(obj.ResourceVersion).NotTo(Equal(objCopy.ResourceVersion))

			t.Log("Creating a new patch helper")
			patcher, err := NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Changing the object spec, status, and marking clusterv1.condition and metav1.conditions Ready=True")
			obj.Spec.Foo = "foo"
			obj.Status.Bar = "bat"
			conditions.MarkTrue(obj, clusterv1.ReadyCondition)
			v1beta2conditions.Set(obj, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "AllGood", LastTransitionTime: now})

			t.Log("Patching the object")
			g.Expect(patcher.Patch(ctx, obj)).To(Succeed())

			t.Log("Validating the object has been updated")
			objAfter := obj.DeepCopy()
			g.Eventually(func() bool {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return false
				}

				testConditionCopy := conditions.Get(objCopy, "Test")
				testConditionAfter := conditions.Get(objAfter, "Test")
				if testConditionCopy == nil || testConditionAfter == nil {
					return false
				}
				ok, err := conditions.MatchCondition(*testConditionCopy).Match(*testConditionAfter)
				if err != nil || !ok {
					return false
				}

				readyBefore := conditions.Get(obj, clusterv1.ReadyCondition)
				readyAfter := conditions.Get(objAfter, clusterv1.ReadyCondition)
				if readyBefore == nil || readyAfter == nil {
					return false
				}
				ok, err = conditions.MatchCondition(*readyBefore).Match(*readyAfter)
				if err != nil || !ok {
					return false
				}

				testV1Beta2ConditionCopy := v1beta2conditions.Get(objCopy, "Test")
				testV1Beta2ConditionAfter := v1beta2conditions.Get(objAfter, "Test")
				if testV1Beta2ConditionCopy == nil || testV1Beta2ConditionAfter == nil {
					return false
				}
				ok, err = v1beta2conditions.MatchCondition(*testV1Beta2ConditionCopy).Match(*testV1Beta2ConditionAfter)
				if err != nil || !ok {
					return false
				}

				readyV1Beta2Before := v1beta2conditions.Get(obj, "Ready")
				readyV1Beta2After := v1beta2conditions.Get(objAfter, "Ready")
				if readyV1Beta2Before == nil || readyV1Beta2After == nil {
					return false
				}
				ok, err = v1beta2conditions.MatchCondition(*readyV1Beta2Before).Match(*readyV1Beta2After)
				if err != nil || !ok {
					return false
				}

				return obj.Spec.Foo == objAfter.Spec.Foo &&
					obj.Status.Bar == objAfter.Status.Bar
			}, timeout).Should(BeTrue(), cmp.Diff(obj, objAfter))
		})

		t.Run("should return an error if there is an unresolvable conflict on conditions", func(t *testing.T) {
			g := NewWithT(t)

			obj := obj.DeepCopy()

			t.Log("Creating the object")
			g.Expect(env.Create(ctx, obj)).To(Succeed())
			defer func() {
				g.Expect(env.Delete(ctx, obj)).To(Succeed())
			}()
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

			t.Log("Checking that the object has been created")
			g.Eventually(func() error {
				obj := obj.DeepCopy()
				return env.Get(ctx, key, obj)
			}).Should(Succeed())

			objCopy := obj.DeepCopy()

			t.Log("Marking a Ready condition to be false")
			conditions.MarkFalse(objCopy, clusterv1.ReadyCondition, "reason", clusterv1.ConditionSeverityInfo, "message")
			g.Expect(env.Status().Update(ctx, objCopy)).To(Succeed())

			t.Log("Validating that the local object's resource version is behind")
			g.Expect(obj.ResourceVersion).NotTo(Equal(objCopy.ResourceVersion))

			t.Log("Creating a new patch helper")
			patcher, err := NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Marking Ready=True")
			conditions.MarkTrue(obj, clusterv1.ReadyCondition)

			t.Log("Patching the object")
			g.Expect(patcher.Patch(ctx, obj)).NotTo(Succeed())

			t.Log("Validating the object has not been updated")
			g.Eventually(func() clusterv1.Conditions {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return clusterv1.Conditions{}
				}
				return objAfter.Status.Conditions
			}, timeout).Should(conditions.MatchConditions(objCopy.Status.Conditions))
		})

		t.Run("should return an error if there is an unresolvable conflict on v1beta2.conditions", func(t *testing.T) {
			g := NewWithT(t)

			obj := obj.DeepCopy()

			t.Log("Creating the object")
			g.Expect(env.Create(ctx, obj)).To(Succeed())
			defer func() {
				g.Expect(env.Delete(ctx, obj)).To(Succeed())
			}()
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

			t.Log("Checking that the object has been created")
			g.Eventually(func() error {
				obj := obj.DeepCopy()
				return env.Get(ctx, key, obj)
			}).Should(Succeed())

			objCopy := obj.DeepCopy()

			t.Log("Marking a Ready condition to be false")
			v1beta2conditions.Set(obj, metav1.Condition{Type: "Ready", Status: metav1.ConditionFalse, Reason: "NotGood", LastTransitionTime: now})
			g.Expect(env.Status().Update(ctx, objCopy)).To(Succeed())

			t.Log("Validating that the local object's resource version is behind")
			g.Expect(obj.ResourceVersion).NotTo(Equal(objCopy.ResourceVersion))

			t.Log("Creating a new patch helper")
			patcher, err := NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Marking condition Ready=True")
			v1beta2conditions.Set(obj, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "AllGood", LastTransitionTime: now})

			t.Log("Patching the object")
			g.Expect(patcher.Patch(ctx, obj)).NotTo(Succeed())

			t.Log("Validating the object has not been updated")
			g.Eventually(func() *builder.Phase1ObjStatusV1Beta2 {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return nil
				}
				return objAfter.Status.V1Beta2
			}, timeout).Should(Equal(objCopy.Status.V1Beta2))
		})

		t.Run("should not return an error if there is an unresolvable conflict but the conditions is owned by the controller", func(t *testing.T) {
			g := NewWithT(t)

			obj := obj.DeepCopy()

			t.Log("Creating the object")
			g.Expect(env.Create(ctx, obj)).To(Succeed())
			defer func() {
				g.Expect(env.Delete(ctx, obj)).To(Succeed())
			}()
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

			t.Log("Checking that the object has been created")
			g.Eventually(func() error {
				obj := obj.DeepCopy()
				return env.Get(ctx, key, obj)
			}).Should(Succeed())

			objCopy := obj.DeepCopy()

			t.Log("Marking a Ready clusterv1.condition and metav1.conditions to be false")
			conditions.MarkFalse(objCopy, clusterv1.ReadyCondition, "reason", clusterv1.ConditionSeverityInfo, "message")
			v1beta2conditions.Set(objCopy, metav1.Condition{Type: "Ready", Status: metav1.ConditionFalse, Reason: "NotGood", LastTransitionTime: now})
			g.Expect(env.Status().Update(ctx, objCopy)).To(Succeed())

			t.Log("Validating that the local object's resource version is behind")
			g.Expect(obj.ResourceVersion).NotTo(Equal(objCopy.ResourceVersion))

			t.Log("Creating a new patch helper")
			patcher, err := NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Marking Ready clusterv1.condition and metav1.conditions True")
			conditions.MarkTrue(obj, clusterv1.ReadyCondition)
			v1beta2conditions.Set(obj, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "AllGood", LastTransitionTime: now})

			t.Log("Patching the object")
			g.Expect(patcher.Patch(ctx, obj, WithOwnedConditions{Conditions: []clusterv1.ConditionType{clusterv1.ReadyCondition}}, WithOwnedV1Beta2Conditions{Conditions: []string{"Ready"}})).To(Succeed())

			t.Log("Validating the object has been updated")
			g.Eventually(func() bool {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return false
				}

				readyBefore := conditions.Get(obj, clusterv1.ReadyCondition)
				readyAfter := conditions.Get(objAfter, clusterv1.ReadyCondition)
				if readyBefore == nil || readyAfter == nil {
					return false
				}
				ok, err := conditions.MatchCondition(*readyBefore).Match(*readyAfter)
				if err != nil || !ok {
					return false
				}

				readyV1Beta2Before := v1beta2conditions.Get(obj, "Ready")
				readyV1Beta2After := v1beta2conditions.Get(objAfter, "Ready")
				if readyV1Beta2Before == nil || readyV1Beta2After == nil {
					return false
				}
				ok, err = v1beta2conditions.MatchCondition(*readyV1Beta2Before).Match(*readyV1Beta2After)
				if err != nil || !ok {
					return false
				}

				return true
			}, timeout).Should(BeTrue())
		})

		t.Run("should not return an error if there is an unresolvable conflict when force overwrite is enabled", func(t *testing.T) {
			g := NewWithT(t)

			obj := obj.DeepCopy()

			t.Log("Creating the object")
			g.Expect(env.Create(ctx, obj)).To(Succeed())
			defer func() {
				g.Expect(env.Delete(ctx, obj)).To(Succeed())
			}()
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

			t.Log("Checking that the object has been created")
			g.Eventually(func() error {
				obj := obj.DeepCopy()
				return env.Get(ctx, key, obj)
			}).Should(Succeed())

			objCopy := obj.DeepCopy()

			t.Log("Marking a Ready clusterv1.condition and metav1.conditions to be false")
			conditions.MarkFalse(objCopy, clusterv1.ReadyCondition, "reason", clusterv1.ConditionSeverityInfo, "message")
			v1beta2conditions.Set(objCopy, metav1.Condition{Type: "Ready", Status: metav1.ConditionFalse, Reason: "NotGood", LastTransitionTime: now})
			g.Expect(env.Status().Update(ctx, objCopy)).To(Succeed())

			t.Log("Validating that the local object's resource version is behind")
			g.Expect(obj.ResourceVersion).NotTo(Equal(objCopy.ResourceVersion))

			t.Log("Creating a new patch helper")
			patcher, err := NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Marking Ready clusterv1.condition and metav1.conditions True")
			conditions.MarkTrue(obj, clusterv1.ReadyCondition)
			v1beta2conditions.Set(obj, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "AllGood", LastTransitionTime: now})

			t.Log("Patching the object")
			g.Expect(patcher.Patch(ctx, obj, WithForceOverwriteConditions{})).To(Succeed())

			t.Log("Validating the object has been updated")
			g.Eventually(func() bool {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return false
				}

				readyBefore := conditions.Get(obj, clusterv1.ReadyCondition)
				readyAfter := conditions.Get(objAfter, clusterv1.ReadyCondition)
				if readyBefore == nil || readyAfter == nil {
					return false
				}
				ok, err := conditions.MatchCondition(*readyBefore).Match(*readyAfter)
				if err != nil || !ok {
					return false
				}

				readyV1Beta2Before := v1beta2conditions.Get(obj, "Ready")
				readyV1Beta2After := v1beta2conditions.Get(objAfter, "Ready")
				if readyV1Beta2Before == nil || readyV1Beta2After == nil {
					return false
				}
				ok, err = v1beta2conditions.MatchCondition(*readyV1Beta2Before).Match(*readyV1Beta2After)
				if err != nil || !ok {
					return false
				}

				return true
			}, timeout).Should(BeTrue())
		})
	})

	t.Run("Should patch conditions on a v1beta2 object with both conditions and backward compatible conditions (phase 2)", func(t *testing.T) {
		obj := &builder.Phase2Obj{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
				Namespace:    metav1.NamespaceDefault,
			},
		}

		t.Run("should mark it ready and sort conditions", func(t *testing.T) {
			g := NewWithT(t)

			obj := obj.DeepCopy()

			t.Log("Creating the object")
			err := env.Create(ctx, obj)
			g.Expect(err).To(Succeed())
			defer func() {
				g.Expect(env.Delete(ctx, obj)).To(Succeed())
			}()
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

			t.Log("Checking that the object has been created")
			g.Eventually(func() error {
				obj := obj.DeepCopy()
				return env.Get(ctx, key, obj)
			}).Should(Succeed())

			// Adding Ready first
			t.Log("Creating a new patch helper")
			patcher, err := NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Marking condition and back compatibility condition Ready=True")
			conditions.MarkTrue(obj, clusterv1.ReadyCondition)
			v1beta2conditions.Set(obj, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "AllGood", LastTransitionTime: now})

			t.Log("Patching the object")
			g.Expect(patcher.Patch(ctx, obj)).To(Succeed())

			// Adding Available as a second condition, but it should be sorted as first
			t.Log("Creating a new patch helper")
			patcher, err = NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Marking condition Available=True")
			v1beta2conditions.Set(obj, metav1.Condition{Type: "Available", Status: metav1.ConditionTrue, Reason: "AllGood", LastTransitionTime: now})

			t.Log("Patching the object")
			g.Expect(patcher.Patch(ctx, obj)).To(Succeed())

			t.Log("Validating the object has been updated")
			g.Eventually(func() clusterv1.Conditions {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return clusterv1.Conditions{}
				}
				return objAfter.Status.Deprecated.V1Beta1.Conditions
			}, timeout).Should(conditions.MatchConditions(obj.Status.Deprecated.V1Beta1.Conditions))
			g.Eventually(func() []metav1.Condition {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return nil
				}
				if len(objAfter.Status.Conditions) != 2 {
					return nil
				}
				if objAfter.Status.Conditions[0].Type != "Available" || objAfter.Status.Conditions[1].Type != "Ready" {
					return nil
				}

				return objAfter.Status.Conditions
			}, timeout).Should(v1beta2conditions.MatchConditions(obj.Status.Conditions))
		})

		t.Run("should mark it ready when passing Clusterv1ConditionsFieldPath and Metav1ConditionsFieldPath", func(t *testing.T) {
			g := NewWithT(t)

			obj := obj.DeepCopy()

			t.Log("Creating the object")
			err := env.Create(ctx, obj)
			g.Expect(err).To(Succeed())
			defer func() {
				g.Expect(env.Delete(ctx, obj)).To(Succeed())
			}()
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

			t.Log("Checking that the object has been created")
			g.Eventually(func() error {
				obj := obj.DeepCopy()
				return env.Get(ctx, key, obj)
			}).Should(Succeed())

			t.Log("Creating a new patch helper")
			patcher, err := NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Marking condition and back compatibility condition Ready=True")
			conditions.MarkTrue(obj, clusterv1.ReadyCondition)
			v1beta2conditions.Set(obj, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "AllGood", LastTransitionTime: now})

			t.Log("Patching the object")
			g.Expect(patcher.Patch(ctx, obj, Clusterv1ConditionsFieldPath{"status", "deprecated", "v1beta1", "conditions"}, Metav1ConditionsFieldPath{"status", "conditions"})).To(Succeed())

			t.Log("Validating the object has been updated")
			g.Eventually(func() clusterv1.Conditions {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return clusterv1.Conditions{}
				}
				return objAfter.Status.Deprecated.V1Beta1.Conditions
			}, timeout).Should(conditions.MatchConditions(obj.Status.Deprecated.V1Beta1.Conditions))
			g.Eventually(func() []metav1.Condition {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return nil
				}
				return objAfter.Status.Conditions
			}, timeout).Should(v1beta2conditions.MatchConditions(obj.Status.Conditions))
		})

		t.Run("should recover if there is a resolvable conflict", func(t *testing.T) {
			g := NewWithT(t)

			obj := obj.DeepCopy()

			t.Log("Creating the object")
			g.Expect(env.Create(ctx, obj)).To(Succeed())
			defer func() {
				g.Expect(env.Delete(ctx, obj)).To(Succeed())
			}()
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

			t.Log("Checking that the object has been created")
			g.Eventually(func() error {
				obj := obj.DeepCopy()
				return env.Get(ctx, key, obj)
			}).Should(Succeed())

			objCopy := obj.DeepCopy()

			t.Log("Marking condition and back compatibility condition Test=False")
			conditions.MarkFalse(objCopy, clusterv1.ConditionType("Test"), "reason", clusterv1.ConditionSeverityInfo, "message")
			v1beta2conditions.Set(objCopy, metav1.Condition{Type: "Test", Status: metav1.ConditionFalse, Reason: "reason", Message: "message", LastTransitionTime: now})
			g.Expect(env.Status().Update(ctx, objCopy)).To(Succeed())

			t.Log("Validating that the local object's resource version is behind")
			g.Expect(obj.ResourceVersion).NotTo(Equal(objCopy.ResourceVersion))

			t.Log("Creating a new patch helper")
			patcher, err := NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Marking condition and back compatibility condition Ready=True")
			conditions.MarkTrue(obj, clusterv1.ReadyCondition)
			v1beta2conditions.Set(obj, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "AllGood", LastTransitionTime: now})

			t.Log("Patching the object")
			g.Expect(patcher.Patch(ctx, obj)).To(Succeed())

			t.Log("Validating the object has been updated")
			g.Eventually(func() bool {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return false
				}

				testBackCompatibilityCopy := conditions.Get(objCopy, "Test")
				testBackCompatibilityAfter := conditions.Get(objAfter, "Test")
				if testBackCompatibilityCopy == nil || testBackCompatibilityAfter == nil {
					return false
				}
				ok, err := conditions.MatchCondition(*testBackCompatibilityCopy).Match(*testBackCompatibilityAfter)
				if err != nil || !ok {
					return false
				}

				readyBackCompatibilityBefore := conditions.Get(obj, clusterv1.ReadyCondition)
				readyBackCompatibilityAfter := conditions.Get(objAfter, clusterv1.ReadyCondition)
				if readyBackCompatibilityBefore == nil || readyBackCompatibilityAfter == nil {
					return false
				}
				ok, err = conditions.MatchCondition(*readyBackCompatibilityBefore).Match(*readyBackCompatibilityAfter)
				if err != nil || !ok {
					return false
				}

				testConditionCopy := v1beta2conditions.Get(objCopy, "Test")
				testConditionAfter := v1beta2conditions.Get(objAfter, "Test")
				if testConditionCopy == nil || testConditionAfter == nil {
					return false
				}
				ok, err = v1beta2conditions.MatchCondition(*testConditionCopy).Match(*testConditionAfter)
				if err != nil || !ok {
					return false
				}

				readyBefore := v1beta2conditions.Get(obj, "Ready")
				readyAfter := v1beta2conditions.Get(objAfter, "Ready")
				if readyBefore == nil || readyAfter == nil {
					return false
				}
				ok, err = v1beta2conditions.MatchCondition(*readyBefore).Match(*readyAfter)
				if err != nil || !ok {
					return false
				}

				return true
			}, timeout).Should(BeTrue())
		})

		t.Run("should recover if there is a resolvable conflict, incl. patch spec and status", func(t *testing.T) {
			g := NewWithT(t)

			obj := obj.DeepCopy()

			t.Log("Creating the object")
			g.Expect(env.Create(ctx, obj)).To(Succeed())
			defer func() {
				g.Expect(env.Delete(ctx, obj)).To(Succeed())
			}()
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

			t.Log("Checking that the object has been created")
			g.Eventually(func() error {
				obj := obj.DeepCopy()
				return env.Get(ctx, key, obj)
			}).Should(Succeed())

			objCopy := obj.DeepCopy()

			t.Log("Marking condition and back compatibility condition Test=False")
			conditions.MarkFalse(objCopy, clusterv1.ConditionType("Test"), "reason", clusterv1.ConditionSeverityInfo, "message")
			v1beta2conditions.Set(objCopy, metav1.Condition{Type: "Test", Status: metav1.ConditionFalse, Reason: "reason", Message: "message", LastTransitionTime: now})
			g.Expect(env.Status().Update(ctx, objCopy)).To(Succeed())

			t.Log("Validating that the local object's resource version is behind")
			g.Expect(obj.ResourceVersion).NotTo(Equal(objCopy.ResourceVersion))

			t.Log("Creating a new patch helper")
			patcher, err := NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Changing the object spec, status, and marking condition and back compatibility condition Ready=True")
			obj.Spec.Foo = "foo"
			obj.Status.Bar = "bat"
			conditions.MarkTrue(obj, clusterv1.ReadyCondition)
			v1beta2conditions.Set(obj, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "AllGood", LastTransitionTime: now})

			t.Log("Patching the object")
			g.Expect(patcher.Patch(ctx, obj)).To(Succeed())

			t.Log("Validating the object has been updated")
			objAfter := obj.DeepCopy()
			g.Eventually(func() bool {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return false
				}

				testBackCompatibilityCopy := conditions.Get(objCopy, "Test")
				testBackCompatibilityAfter := conditions.Get(objAfter, "Test")
				if testBackCompatibilityCopy == nil || testBackCompatibilityAfter == nil {
					return false
				}
				ok, err := conditions.MatchCondition(*testBackCompatibilityCopy).Match(*testBackCompatibilityAfter)
				if err != nil || !ok {
					return false
				}

				readyBackCompatibilityBefore := conditions.Get(obj, clusterv1.ReadyCondition)
				readyBackCompatibilityAfter := conditions.Get(objAfter, clusterv1.ReadyCondition)
				if readyBackCompatibilityBefore == nil || readyBackCompatibilityAfter == nil {
					return false
				}
				ok, err = conditions.MatchCondition(*readyBackCompatibilityBefore).Match(*readyBackCompatibilityAfter)
				if err != nil || !ok {
					return false
				}

				testConditionCopy := v1beta2conditions.Get(objCopy, "Test")
				testConditionAfter := v1beta2conditions.Get(objAfter, "Test")
				if testConditionCopy == nil || testConditionAfter == nil {
					return false
				}
				ok, err = v1beta2conditions.MatchCondition(*testConditionCopy).Match(*testConditionAfter)
				if err != nil || !ok {
					return false
				}

				readyBefore := v1beta2conditions.Get(obj, "Ready")
				readyAfter := v1beta2conditions.Get(objAfter, "Ready")
				if readyBefore == nil || readyAfter == nil {
					return false
				}
				ok, err = v1beta2conditions.MatchCondition(*readyBefore).Match(*readyAfter)
				if err != nil || !ok {
					return false
				}

				return obj.Spec.Foo == objAfter.Spec.Foo &&
					obj.Status.Bar == objAfter.Status.Bar
			}, timeout).Should(BeTrue(), cmp.Diff(obj, objAfter))
		})

		t.Run("should return an error if there is an unresolvable conflict on back compatibility conditions", func(t *testing.T) {
			g := NewWithT(t)

			obj := obj.DeepCopy()

			t.Log("Creating the object")
			g.Expect(env.Create(ctx, obj)).To(Succeed())
			defer func() {
				g.Expect(env.Delete(ctx, obj)).To(Succeed())
			}()
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

			t.Log("Checking that the object has been created")
			g.Eventually(func() error {
				obj := obj.DeepCopy()
				return env.Get(ctx, key, obj)
			}).Should(Succeed())

			objCopy := obj.DeepCopy()

			t.Log("Marking a Ready condition to be false")
			conditions.MarkFalse(objCopy, clusterv1.ReadyCondition, "reason", clusterv1.ConditionSeverityInfo, "message")
			g.Expect(env.Status().Update(ctx, objCopy)).To(Succeed())

			t.Log("Validating that the local object's resource version is behind")
			g.Expect(obj.ResourceVersion).NotTo(Equal(objCopy.ResourceVersion))

			t.Log("Creating a new patch helper")
			patcher, err := NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Marking Ready=True")
			conditions.MarkTrue(obj, clusterv1.ReadyCondition)

			t.Log("Patching the object")
			g.Expect(patcher.Patch(ctx, obj)).NotTo(Succeed())

			t.Log("Validating the object has not been updated")
			g.Eventually(func() clusterv1.Conditions {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return clusterv1.Conditions{}
				}
				return objAfter.Status.Deprecated.V1Beta1.Conditions
			}, timeout).Should(conditions.MatchConditions(objCopy.Status.Deprecated.V1Beta1.Conditions))
		})

		t.Run("should return an error if there is an unresolvable conflict on conditions", func(t *testing.T) {
			g := NewWithT(t)

			obj := obj.DeepCopy()

			t.Log("Creating the object")
			g.Expect(env.Create(ctx, obj)).To(Succeed())
			defer func() {
				g.Expect(env.Delete(ctx, obj)).To(Succeed())
			}()
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

			t.Log("Checking that the object has been created")
			g.Eventually(func() error {
				obj := obj.DeepCopy()
				return env.Get(ctx, key, obj)
			}).Should(Succeed())

			objCopy := obj.DeepCopy()

			t.Log("Marking a Ready condition to be false")
			v1beta2conditions.Set(obj, metav1.Condition{Type: "Ready", Status: metav1.ConditionFalse, Reason: "NotGood", LastTransitionTime: now})
			g.Expect(env.Status().Update(ctx, objCopy)).To(Succeed())

			t.Log("Validating that the local object's resource version is behind")
			g.Expect(obj.ResourceVersion).NotTo(Equal(objCopy.ResourceVersion))

			t.Log("Creating a new patch helper")
			patcher, err := NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Marking condition Ready=True")
			v1beta2conditions.Set(obj, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "AllGood", LastTransitionTime: now})

			t.Log("Patching the object")
			g.Expect(patcher.Patch(ctx, obj)).NotTo(Succeed())

			t.Log("Validating the object has not been updated")
			g.Eventually(func() []metav1.Condition {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return nil
				}
				return objAfter.Status.Conditions
			}, timeout).Should(v1beta2conditions.MatchConditions(objCopy.Status.Conditions))
		})

		t.Run("should not return an error if there is an unresolvable conflict but the conditions is owned by the controller", func(t *testing.T) {
			g := NewWithT(t)

			obj := obj.DeepCopy()

			t.Log("Creating the object")
			g.Expect(env.Create(ctx, obj)).To(Succeed())
			defer func() {
				g.Expect(env.Delete(ctx, obj)).To(Succeed())
			}()
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

			t.Log("Checking that the object has been created")
			g.Eventually(func() error {
				obj := obj.DeepCopy()
				return env.Get(ctx, key, obj)
			}).Should(Succeed())

			objCopy := obj.DeepCopy()

			t.Log("Marking a Ready condition and back compatibility condition to be false")
			conditions.MarkFalse(objCopy, clusterv1.ReadyCondition, "reason", clusterv1.ConditionSeverityInfo, "message")
			v1beta2conditions.Set(objCopy, metav1.Condition{Type: "Ready", Status: metav1.ConditionFalse, Reason: "NotGood", LastTransitionTime: now})
			g.Expect(env.Status().Update(ctx, objCopy)).To(Succeed())

			t.Log("Validating that the local object's resource version is behind")
			g.Expect(obj.ResourceVersion).NotTo(Equal(objCopy.ResourceVersion))

			t.Log("Creating a new patch helper")
			patcher, err := NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Marking Ready condition and back compatibility condition True")
			conditions.MarkTrue(obj, clusterv1.ReadyCondition)
			v1beta2conditions.Set(obj, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "AllGood", LastTransitionTime: now})

			t.Log("Patching the object")
			g.Expect(patcher.Patch(ctx, obj, WithOwnedConditions{Conditions: []clusterv1.ConditionType{clusterv1.ReadyCondition}}, WithOwnedV1Beta2Conditions{Conditions: []string{"Ready"}})).To(Succeed())

			t.Log("Validating the object has been updated")
			g.Eventually(func() bool {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return false
				}

				readyBackCompatibilityBefore := conditions.Get(obj, clusterv1.ReadyCondition)
				readyBackCompatibilityAfter := conditions.Get(objAfter, clusterv1.ReadyCondition)
				if readyBackCompatibilityBefore == nil || readyBackCompatibilityAfter == nil {
					return false
				}
				ok, err := conditions.MatchCondition(*readyBackCompatibilityBefore).Match(*readyBackCompatibilityAfter)
				if err != nil || !ok {
					return false
				}

				readyBefore := v1beta2conditions.Get(obj, "Ready")
				readyAfter := v1beta2conditions.Get(objAfter, "Ready")
				if readyBefore == nil || readyAfter == nil {
					return false
				}
				ok, err = v1beta2conditions.MatchCondition(*readyBefore).Match(*readyAfter)
				if err != nil || !ok {
					return false
				}

				return true
			}, timeout).Should(BeTrue())
		})

		t.Run("should not return an error if there is an unresolvable conflict when force overwrite is enabled", func(t *testing.T) {
			g := NewWithT(t)

			obj := obj.DeepCopy()

			t.Log("Creating the object")
			g.Expect(env.Create(ctx, obj)).To(Succeed())
			defer func() {
				g.Expect(env.Delete(ctx, obj)).To(Succeed())
			}()
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

			t.Log("Checking that the object has been created")
			g.Eventually(func() error {
				obj := obj.DeepCopy()
				return env.Get(ctx, key, obj)
			}).Should(Succeed())

			objCopy := obj.DeepCopy()

			t.Log("Marking a Ready condition and back compatibility condition to be false")
			conditions.MarkFalse(objCopy, clusterv1.ReadyCondition, "reason", clusterv1.ConditionSeverityInfo, "message")
			v1beta2conditions.Set(objCopy, metav1.Condition{Type: "Ready", Status: metav1.ConditionFalse, Reason: "NotGood", LastTransitionTime: now})
			g.Expect(env.Status().Update(ctx, objCopy)).To(Succeed())

			t.Log("Validating that the local object's resource version is behind")
			g.Expect(obj.ResourceVersion).NotTo(Equal(objCopy.ResourceVersion))

			t.Log("Creating a new patch helper")
			patcher, err := NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Marking Ready condition and back compatibility condition True")
			conditions.MarkTrue(obj, clusterv1.ReadyCondition)
			v1beta2conditions.Set(obj, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "AllGood", LastTransitionTime: now})

			t.Log("Patching the object")
			g.Expect(patcher.Patch(ctx, obj, WithForceOverwriteConditions{})).To(Succeed())

			t.Log("Validating the object has been updated")
			g.Eventually(func() bool {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return false
				}

				readyBackCompatibilityBefore := conditions.Get(obj, clusterv1.ReadyCondition)
				readyBackCompatibilityAfter := conditions.Get(objAfter, clusterv1.ReadyCondition)
				if readyBackCompatibilityBefore == nil || readyBackCompatibilityAfter == nil {
					return false
				}
				ok, err := conditions.MatchCondition(*readyBackCompatibilityBefore).Match(*readyBackCompatibilityAfter)
				if err != nil || !ok {
					return false
				}

				readyBefore := v1beta2conditions.Get(obj, "Ready")
				readyAfter := v1beta2conditions.Get(objAfter, "Ready")
				if readyBefore == nil || readyAfter == nil {
					return false
				}
				ok, err = v1beta2conditions.MatchCondition(*readyBefore).Match(*readyAfter)
				if err != nil || !ok {
					return false
				}

				return true
			}, timeout).Should(BeTrue())
		})
	})

	t.Run("Should patch conditions on a v1beta2 object with conditions (phase 3)", func(t *testing.T) {
		obj := &builder.Phase3Obj{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
				Namespace:    metav1.NamespaceDefault,
			},
		}

		t.Run("should mark it ready and sort conditions", func(t *testing.T) {
			g := NewWithT(t)

			obj := obj.DeepCopy()

			t.Log("Creating the object")
			err := env.Create(ctx, obj)
			g.Expect(err).To(Succeed())
			defer func() {
				g.Expect(env.Delete(ctx, obj)).To(Succeed())
			}()
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

			t.Log("Checking that the object has been created")
			g.Eventually(func() error {
				obj := obj.DeepCopy()
				return env.Get(ctx, key, obj)
			}).Should(Succeed())

			// Adding Ready first
			t.Log("Creating a new patch helper")
			patcher, err := NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Marking condition Ready=True")
			v1beta2conditions.Set(obj, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "AllGood", LastTransitionTime: now})

			t.Log("Patching the object")
			g.Expect(patcher.Patch(ctx, obj)).To(Succeed())

			// Adding Available as a second condition, but it should be sorted as first
			t.Log("Creating a new patch helper")
			patcher, err = NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Marking condition Available=True")
			v1beta2conditions.Set(obj, metav1.Condition{Type: "Available", Status: metav1.ConditionTrue, Reason: "AllGood", LastTransitionTime: now})

			t.Log("Patching the object")
			g.Expect(patcher.Patch(ctx, obj)).To(Succeed())

			t.Log("Validating the object has been updated")
			g.Eventually(func() []metav1.Condition {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return nil
				}
				if len(objAfter.Status.Conditions) != 2 {
					return nil
				}
				if objAfter.Status.Conditions[0].Type != "Available" || objAfter.Status.Conditions[1].Type != "Ready" {
					return nil
				}

				return objAfter.Status.Conditions
			}, timeout).Should(v1beta2conditions.MatchConditions(obj.Status.Conditions))
		})

		t.Run("should mark it ready when passing Metav1ConditionsFieldPath", func(t *testing.T) {
			g := NewWithT(t)

			obj := obj.DeepCopy()

			t.Log("Creating the object")
			err := env.Create(ctx, obj)
			g.Expect(err).To(Succeed())
			defer func() {
				g.Expect(env.Delete(ctx, obj)).To(Succeed())
			}()
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

			t.Log("Checking that the object has been created")
			g.Eventually(func() error {
				obj := obj.DeepCopy()
				return env.Get(ctx, key, obj)
			}).Should(Succeed())

			t.Log("Creating a new patch helper")
			patcher, err := NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Marking condition Ready=True")
			v1beta2conditions.Set(obj, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "AllGood", LastTransitionTime: now})

			t.Log("Patching the object")
			g.Expect(patcher.Patch(ctx, obj, Metav1ConditionsFieldPath{"status", "conditions"})).To(Succeed())

			t.Log("Validating the object has been updated")
			g.Eventually(func() []metav1.Condition {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return nil
				}
				return objAfter.Status.Conditions
			}, timeout).Should(v1beta2conditions.MatchConditions(obj.Status.Conditions))
		})

		t.Run("should recover if there is a resolvable conflict", func(t *testing.T) {
			g := NewWithT(t)

			obj := obj.DeepCopy()

			t.Log("Creating the object")
			g.Expect(env.Create(ctx, obj)).To(Succeed())
			defer func() {
				g.Expect(env.Delete(ctx, obj)).To(Succeed())
			}()
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

			t.Log("Checking that the object has been created")
			g.Eventually(func() error {
				obj := obj.DeepCopy()
				return env.Get(ctx, key, obj)
			}).Should(Succeed())

			objCopy := obj.DeepCopy()

			t.Log("Marking condition Test=False")
			v1beta2conditions.Set(objCopy, metav1.Condition{Type: "Test", Status: metav1.ConditionFalse, Reason: "reason", Message: "message", LastTransitionTime: now})
			g.Expect(env.Status().Update(ctx, objCopy)).To(Succeed())

			t.Log("Validating that the local object's resource version is behind")
			g.Expect(obj.ResourceVersion).NotTo(Equal(objCopy.ResourceVersion))

			t.Log("Creating a new patch helper")
			patcher, err := NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Marking condition Ready=True")
			v1beta2conditions.Set(obj, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "AllGood", LastTransitionTime: now})

			t.Log("Patching the object")
			g.Expect(patcher.Patch(ctx, obj)).To(Succeed())

			t.Log("Validating the object has been updated")
			g.Eventually(func() bool {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return false
				}

				testConditionCopy := v1beta2conditions.Get(objCopy, "Test")
				testConditionAfter := v1beta2conditions.Get(objAfter, "Test")
				if testConditionCopy == nil || testConditionAfter == nil {
					return false
				}
				ok, err := v1beta2conditions.MatchCondition(*testConditionCopy).Match(*testConditionAfter)
				if err != nil || !ok {
					return false
				}

				readyBefore := v1beta2conditions.Get(obj, "Ready")
				readyAfter := v1beta2conditions.Get(objAfter, "Ready")
				if readyBefore == nil || readyAfter == nil {
					return false
				}
				ok, err = v1beta2conditions.MatchCondition(*readyBefore).Match(*readyAfter)
				if err != nil || !ok {
					return false
				}

				return true
			}, timeout).Should(BeTrue())
		})

		t.Run("should recover if there is a resolvable conflict, incl. patch spec and status", func(t *testing.T) {
			g := NewWithT(t)

			obj := obj.DeepCopy()

			t.Log("Creating the object")
			g.Expect(env.Create(ctx, obj)).To(Succeed())
			defer func() {
				g.Expect(env.Delete(ctx, obj)).To(Succeed())
			}()
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

			t.Log("Checking that the object has been created")
			g.Eventually(func() error {
				obj := obj.DeepCopy()
				return env.Get(ctx, key, obj)
			}).Should(Succeed())

			objCopy := obj.DeepCopy()

			t.Log("Marking condition Test=False")
			v1beta2conditions.Set(objCopy, metav1.Condition{Type: "Test", Status: metav1.ConditionFalse, Reason: "reason", Message: "message", LastTransitionTime: now})
			g.Expect(env.Status().Update(ctx, objCopy)).To(Succeed())

			t.Log("Validating that the local object's resource version is behind")
			g.Expect(obj.ResourceVersion).NotTo(Equal(objCopy.ResourceVersion))

			t.Log("Creating a new patch helper")
			patcher, err := NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Changing the object spec, status, and marking condition Ready=True")
			obj.Spec.Foo = "foo"
			obj.Status.Bar = "bat"
			v1beta2conditions.Set(obj, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "AllGood", LastTransitionTime: now})

			t.Log("Patching the object")
			g.Expect(patcher.Patch(ctx, obj)).To(Succeed())

			t.Log("Validating the object has been updated")
			objAfter := obj.DeepCopy()
			g.Eventually(func() bool {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return false
				}

				testConditionCopy := v1beta2conditions.Get(objCopy, "Test")
				testConditionAfter := v1beta2conditions.Get(objAfter, "Test")
				if testConditionCopy == nil || testConditionAfter == nil {
					return false
				}
				ok, err := v1beta2conditions.MatchCondition(*testConditionCopy).Match(*testConditionAfter)
				if err != nil || !ok {
					return false
				}

				readyBefore := v1beta2conditions.Get(obj, "Ready")
				readyAfter := v1beta2conditions.Get(objAfter, "Ready")
				if readyBefore == nil || readyAfter == nil {
					return false
				}
				ok, err = v1beta2conditions.MatchCondition(*readyBefore).Match(*readyAfter)
				if err != nil || !ok {
					return false
				}

				return obj.Spec.Foo == objAfter.Spec.Foo &&
					obj.Status.Bar == objAfter.Status.Bar
			}, timeout).Should(BeTrue(), cmp.Diff(obj, objAfter))
		})

		t.Run("should return an error if there is an unresolvable conflict on conditions", func(t *testing.T) {
			g := NewWithT(t)

			obj := obj.DeepCopy()

			t.Log("Creating the object")
			g.Expect(env.Create(ctx, obj)).To(Succeed())
			defer func() {
				g.Expect(env.Delete(ctx, obj)).To(Succeed())
			}()
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

			t.Log("Checking that the object has been created")
			g.Eventually(func() error {
				obj := obj.DeepCopy()
				return env.Get(ctx, key, obj)
			}).Should(Succeed())

			objCopy := obj.DeepCopy()

			t.Log("Marking a Ready condition to be false")
			v1beta2conditions.Set(obj, metav1.Condition{Type: "Ready", Status: metav1.ConditionFalse, Reason: "NotGood", LastTransitionTime: now})
			g.Expect(env.Status().Update(ctx, objCopy)).To(Succeed())

			t.Log("Validating that the local object's resource version is behind")
			g.Expect(obj.ResourceVersion).NotTo(Equal(objCopy.ResourceVersion))

			t.Log("Creating a new patch helper")
			patcher, err := NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Marking condition Ready=True")
			v1beta2conditions.Set(obj, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "AllGood", LastTransitionTime: now})

			t.Log("Patching the object")
			g.Expect(patcher.Patch(ctx, obj)).NotTo(Succeed())

			t.Log("Validating the object has not been updated")
			g.Eventually(func() []metav1.Condition {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return nil
				}
				return objAfter.Status.Conditions
			}, timeout).Should(v1beta2conditions.MatchConditions(objCopy.Status.Conditions))
		})

		t.Run("should not return an error if there is an unresolvable conflict but the conditions is owned by the controller", func(t *testing.T) {
			g := NewWithT(t)

			obj := obj.DeepCopy()

			t.Log("Creating the object")
			g.Expect(env.Create(ctx, obj)).To(Succeed())
			defer func() {
				g.Expect(env.Delete(ctx, obj)).To(Succeed())
			}()
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

			t.Log("Checking that the object has been created")
			g.Eventually(func() error {
				obj := obj.DeepCopy()
				return env.Get(ctx, key, obj)
			}).Should(Succeed())

			objCopy := obj.DeepCopy()

			t.Log("Marking a Ready condition to be false")
			v1beta2conditions.Set(objCopy, metav1.Condition{Type: "Ready", Status: metav1.ConditionFalse, Reason: "NotGood", LastTransitionTime: now})
			g.Expect(env.Status().Update(ctx, objCopy)).To(Succeed())

			t.Log("Validating that the local object's resource version is behind")
			g.Expect(obj.ResourceVersion).NotTo(Equal(objCopy.ResourceVersion))

			t.Log("Creating a new patch helper")
			patcher, err := NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Marking Ready condition True")
			v1beta2conditions.Set(obj, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "AllGood", LastTransitionTime: now})

			t.Log("Patching the object")
			g.Expect(patcher.Patch(ctx, obj, WithOwnedV1Beta2Conditions{Conditions: []string{"Ready"}})).To(Succeed())

			t.Log("Validating the object has been updated")
			g.Eventually(func() bool {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return false
				}

				readyBefore := v1beta2conditions.Get(obj, "Ready")
				readyAfter := v1beta2conditions.Get(objAfter, "Ready")
				if readyBefore == nil || readyAfter == nil {
					return false
				}
				ok, err := v1beta2conditions.MatchCondition(*readyBefore).Match(*readyAfter)
				if err != nil || !ok {
					return false
				}

				return true
			}, timeout).Should(BeTrue())
		})

		t.Run("should not return an error if there is an unresolvable conflict when force overwrite is enabled", func(t *testing.T) {
			g := NewWithT(t)

			obj := obj.DeepCopy()

			t.Log("Creating the object")
			g.Expect(env.Create(ctx, obj)).To(Succeed())
			defer func() {
				g.Expect(env.Delete(ctx, obj)).To(Succeed())
			}()
			key := client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}

			t.Log("Checking that the object has been created")
			g.Eventually(func() error {
				obj := obj.DeepCopy()
				return env.Get(ctx, key, obj)
			}).Should(Succeed())

			objCopy := obj.DeepCopy()

			t.Log("Marking a Ready condition to be false")
			v1beta2conditions.Set(objCopy, metav1.Condition{Type: "Ready", Status: metav1.ConditionFalse, Reason: "NotGood", LastTransitionTime: now})
			g.Expect(env.Status().Update(ctx, objCopy)).To(Succeed())

			t.Log("Validating that the local object's resource version is behind")
			g.Expect(obj.ResourceVersion).NotTo(Equal(objCopy.ResourceVersion))

			t.Log("Creating a new patch helper")
			patcher, err := NewHelper(obj, env)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Marking Ready condition True")
			v1beta2conditions.Set(obj, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "AllGood", LastTransitionTime: now})

			t.Log("Patching the object")
			g.Expect(patcher.Patch(ctx, obj, WithForceOverwriteConditions{})).To(Succeed())

			t.Log("Validating the object has been updated")
			g.Eventually(func() bool {
				objAfter := obj.DeepCopy()
				if err := env.Get(ctx, key, objAfter); err != nil {
					return false
				}

				readyBefore := v1beta2conditions.Get(obj, "Ready")
				readyAfter := v1beta2conditions.Get(objAfter, "Ready")
				if readyBefore == nil || readyAfter == nil {
					return false
				}
				ok, err := v1beta2conditions.MatchCondition(*readyBefore).Match(*readyAfter)
				if err != nil || !ok {
					return false
				}

				return true
			}, timeout).Should(BeTrue())
		})
	})
}
