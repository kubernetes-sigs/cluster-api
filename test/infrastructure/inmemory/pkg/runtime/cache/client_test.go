/*
Copyright 2023 The Kubernetes Authors.

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

package cache

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cloudv1 "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/internal/cloud/api/v1alpha1"
)

func Test_cache_client(t *testing.T) {
	t.Run("create objects", func(t *testing.T) {
		g := NewWithT(t)

		c := NewCache(scheme).(*cache)
		h := &fakeHandler{}
		iMachine, err := c.GetInformer(context.TODO(), &cloudv1.CloudMachine{})
		g.Expect(err).ToNot(HaveOccurred())
		err = iMachine.AddEventHandler(h)
		g.Expect(err).ToNot(HaveOccurred())

		c.AddResourceGroup("foo")

		t.Run("fails if resourceGroup is empty", func(t *testing.T) {
			g := NewWithT(t)

			obj := &cloudv1.CloudMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
			}
			err := c.Create("", obj)
			g.Expect(err).To(HaveOccurred())
			g.Expect(apierrors.IsBadRequest(err)).To(BeTrue())
		})

		t.Run("fails if obj is nil", func(t *testing.T) {
			g := NewWithT(t)

			err := c.Create("foo", nil)
			g.Expect(err).To(HaveOccurred())
			g.Expect(apierrors.IsBadRequest(err)).To(BeTrue())
		})

		t.Run("fails if unknown kind", func(*testing.T) {
			// TODO implement test case
		})

		t.Run("fails if resourceGroup does not exist", func(t *testing.T) {
			g := NewWithT(t)

			obj := &cloudv1.CloudMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
			}
			err := c.Create("bar", obj)
			g.Expect(err).To(HaveOccurred())
			g.Expect(apierrors.IsBadRequest(err)).To(BeTrue())
		})

		t.Run("create", func(t *testing.T) {
			g := NewWithT(t)

			obj := createMachine(t, c, "foo", "bar")

			// Check all the computed fields have been updated on the object.
			g.Expect(obj.CreationTimestamp.IsZero()).To(BeFalse())
			g.Expect(obj.ResourceVersion).ToNot(BeEmpty())
			g.Expect(obj.Annotations).To(HaveKey(lastSyncTimeAnnotation))

			// Check internal state of the tracker is as expected.
			c.lock.RLock()
			defer c.lock.RUnlock()

			g.Expect(c.resourceGroups["foo"].objects).To(HaveKey(cloudv1.GroupVersion.WithKind(cloudv1.CloudMachineKind)), "gvk must exists in object tracker for foo")
			key := types.NamespacedName{Name: "bar"}
			g.Expect(c.resourceGroups["foo"].objects[cloudv1.GroupVersion.WithKind(cloudv1.CloudMachineKind)]).To(HaveKey(key), "Object bar must exists in object tracker for foo")

			r := c.resourceGroups["foo"].objects[cloudv1.GroupVersion.WithKind(cloudv1.CloudMachineKind)][key]
			g.Expect(r.GetObjectKind().GroupVersionKind()).To(BeComparableTo(cloudv1.GroupVersion.WithKind(cloudv1.CloudMachineKind)), "gvk must be set")
			g.Expect(r.GetName()).To(Equal("bar"), "name must be equal to object tracker key")
			g.Expect(r.GetResourceVersion()).To(Equal("1"), "resourceVersion must be set")
			g.Expect(r.GetCreationTimestamp()).ToNot(BeZero(), "creation timestamp must be set")
			g.Expect(r.GetAnnotations()).To(HaveKey(lastSyncTimeAnnotation), "last sync annotation must exists")

			g.Expect(h.Events()).To(ContainElement("foo, CloudMachine=bar, Created"))
		})

		t.Run("fails if Object already exists", func(t *testing.T) {
			g := NewWithT(t)

			createMachine(t, c, "foo", "bazzz")

			obj := &cloudv1.CloudMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name: "bazzz",
				},
			}
			err := c.Create("foo", obj)
			g.Expect(err).To(HaveOccurred())
			g.Expect(apierrors.IsAlreadyExists(err)).To(BeTrue())
		})

		t.Run("Create with owner references", func(t *testing.T) {
			t.Run("fails for invalid owner reference", func(t *testing.T) {
				g := NewWithT(t)

				obj := &cloudv1.CloudMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name: "child",
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: "something/not/valid",
								Kind:       "ParentKind",
								Name:       "parent",
							},
						},
					},
				}
				err := c.Create("foo", obj)
				g.Expect(err).To(HaveOccurred())
				g.Expect(apierrors.IsBadRequest(err)).To(BeTrue())
			})
			t.Run("fails if referenced object does not exist", func(t *testing.T) {
				g := NewWithT(t)

				obj := &cloudv1.CloudMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name: "child",
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: cloudv1.GroupVersion.String(),
								Kind:       "CloudMachine",
								Name:       "parentx",
							},
						},
					},
				}
				err := c.Create("foo", obj)
				g.Expect(err).To(HaveOccurred())
				g.Expect(apierrors.IsBadRequest(err)).To(BeTrue())
			})
			t.Run("create updates ownedObjects", func(t *testing.T) {
				g := NewWithT(t)

				createMachine(t, c, "foo", "parent")
				obj := &cloudv1.CloudMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name: "child",
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: cloudv1.GroupVersion.String(),
								Kind:       "CloudMachine",
								Name:       "parent",
							},
						},
					},
				}
				err := c.Create("foo", obj)
				g.Expect(err).ToNot(HaveOccurred())

				// Check internal state of the tracker is as expected.
				c.lock.RLock()
				defer c.lock.RUnlock()

				parentRef := ownReference{gvk: cloudv1.GroupVersion.WithKind(cloudv1.CloudMachineKind), key: types.NamespacedName{Namespace: "", Name: "parent"}}
				g.Expect(c.resourceGroups["foo"].ownedObjects).To(HaveKey(parentRef), "there should be ownedObjects for parent")
				childRef := ownReference{gvk: cloudv1.GroupVersion.WithKind(cloudv1.CloudMachineKind), key: types.NamespacedName{Namespace: "", Name: "child"}}
				g.Expect(c.resourceGroups["foo"].ownedObjects[parentRef]).To(HaveKey(childRef), "parent should own child")
			})
		})
	})

	t.Run("Get objects", func(t *testing.T) {
		c := NewCache(scheme).(*cache)
		c.AddResourceGroup("foo")

		t.Run("fails if resourceGroup is empty", func(t *testing.T) {
			g := NewWithT(t)

			obj := &cloudv1.CloudMachine{}
			err := c.Get("", types.NamespacedName{Name: "foo"}, obj)
			g.Expect(err).To(HaveOccurred())
			g.Expect(apierrors.IsBadRequest(err)).To(BeTrue())
		})

		t.Run("fails if name is empty", func(t *testing.T) {
			g := NewWithT(t)

			obj := &cloudv1.CloudMachine{}
			err := c.Get("foo", types.NamespacedName{}, obj)
			g.Expect(err).To(HaveOccurred())
			g.Expect(apierrors.IsBadRequest(err)).To(BeTrue())
		})

		t.Run("fails if Object is nil", func(t *testing.T) {
			g := NewWithT(t)

			err := c.Get("foo", types.NamespacedName{Name: "foo"}, nil)
			g.Expect(err).To(HaveOccurred())
			g.Expect(apierrors.IsBadRequest(err)).To(BeTrue())
		})

		t.Run("fails if unknown kind", func(*testing.T) {
			// TODO implement test case
		})

		t.Run("fails if resourceGroup doesn't exist", func(t *testing.T) {
			g := NewWithT(t)

			obj := &cloudv1.CloudMachine{}
			err := c.Get("bar", types.NamespacedName{Name: "bar"}, obj)
			g.Expect(err).To(HaveOccurred())
			g.Expect(apierrors.IsBadRequest(err)).To(BeTrue())
		})

		t.Run("fails if gvk doesn't exist", func(t *testing.T) {
			g := NewWithT(t)

			obj := &cloudv1.CloudMachine{}
			err := c.Get("foo", types.NamespacedName{Name: "bar"}, obj)
			g.Expect(err).To(HaveOccurred())
			g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})

		t.Run("fails if Object doesn't exist", func(t *testing.T) {
			g := NewWithT(t)

			createMachine(t, c, "foo", "barz")

			obj := &cloudv1.CloudMachine{}
			err := c.Get("foo", types.NamespacedName{Name: "bar"}, obj)
			g.Expect(err).To(HaveOccurred())
			g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})

		t.Run("get", func(t *testing.T) {
			g := NewWithT(t)

			createMachine(t, c, "foo", "bar")

			obj := &cloudv1.CloudMachine{}
			err := c.Get("foo", types.NamespacedName{Name: "bar"}, obj)
			g.Expect(err).ToNot(HaveOccurred())

			// Check all the computed fields are as expected.
			g.Expect(obj.GetObjectKind().GroupVersionKind()).To(BeComparableTo(cloudv1.GroupVersion.WithKind(cloudv1.CloudMachineKind)), "gvk must be set")
			g.Expect(obj.GetName()).To(Equal("bar"), "name must be equal to object tracker key")
			g.Expect(obj.GetResourceVersion()).To(Equal("2"), "resourceVersion must be set")
			g.Expect(obj.GetCreationTimestamp()).ToNot(BeZero(), "creation timestamp must be set")
			g.Expect(obj.GetAnnotations()).To(HaveKey(lastSyncTimeAnnotation), "last sync annotation must be set")
		})
	})

	t.Run("list objects", func(t *testing.T) {
		c := NewCache(scheme).(*cache)
		c.AddResourceGroup("foo")

		t.Run("fails if resourceGroup is empty", func(t *testing.T) {
			g := NewWithT(t)

			obj := &cloudv1.CloudMachineList{}
			err := c.List("", obj)
			g.Expect(err).To(HaveOccurred())
			g.Expect(apierrors.IsBadRequest(err)).To(BeTrue())
		})

		t.Run("fails if Object is nil", func(t *testing.T) {
			g := NewWithT(t)

			err := c.List("foo", nil)
			g.Expect(err).To(HaveOccurred())
			g.Expect(apierrors.IsBadRequest(err)).To(BeTrue())
		})

		t.Run("fails if unknown kind", func(*testing.T) {
			// TODO implement test case
		})

		t.Run("fails if resourceGroup doesn't exist", func(t *testing.T) {
			g := NewWithT(t)

			obj := &cloudv1.CloudMachineList{}
			err := c.List("bar", obj)
			g.Expect(err).To(HaveOccurred())
			g.Expect(apierrors.IsBadRequest(err)).To(BeTrue())
		})

		t.Run("list", func(t *testing.T) {
			g := NewWithT(t)

			createMachine(t, c, "foo", "bar")
			createMachine(t, c, "foo", "baz")

			obj := &cloudv1.CloudMachineList{}
			err := c.List("foo", obj)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(obj.Items).To(HaveLen(2))

			i1 := obj.Items[0]
			g.Expect(i1.GetObjectKind().GroupVersionKind()).To(Equal(cloudv1.GroupVersion.WithKind(cloudv1.CloudMachineKind)), "gvk must be set")
			g.Expect(i1.GetAnnotations()).To(HaveKey(lastSyncTimeAnnotation), "last sync annotation must be present")

			i2 := obj.Items[1]
			g.Expect(i2.GetObjectKind().GroupVersionKind()).To(Equal(cloudv1.GroupVersion.WithKind(cloudv1.CloudMachineKind)), "gvk must be set")
			g.Expect(i2.GetAnnotations()).To(HaveKey(lastSyncTimeAnnotation), "last sync annotation must be present")
		})

		// TODO: test filtering by labels
	})

	t.Run("update objects", func(t *testing.T) {
		g := NewWithT(t)

		c := NewCache(scheme).(*cache)
		h := &fakeHandler{}
		i, err := c.GetInformer(context.TODO(), &cloudv1.CloudMachine{})
		g.Expect(err).ToNot(HaveOccurred())
		err = i.AddEventHandler(h)
		g.Expect(err).ToNot(HaveOccurred())

		c.AddResourceGroup("foo")

		t.Run("fails if resourceGroup is empty", func(t *testing.T) {
			g := NewWithT(t)

			obj := &cloudv1.CloudMachine{}
			err := c.Update("", obj)
			g.Expect(err).To(HaveOccurred())
			g.Expect(apierrors.IsBadRequest(err)).To(BeTrue())
		})

		t.Run("fails if Object is nil", func(t *testing.T) {
			g := NewWithT(t)

			err := c.Update("foo", nil)
			g.Expect(err).To(HaveOccurred())
			g.Expect(apierrors.IsBadRequest(err)).To(BeTrue())
		})

		t.Run("fails if unknown kind", func(*testing.T) {
			// TODO implement test case
		})

		t.Run("fails if name is empty", func(t *testing.T) {
			g := NewWithT(t)

			obj := &cloudv1.CloudMachine{}
			err := c.Update("foo", obj)
			g.Expect(err).To(HaveOccurred())
			g.Expect(apierrors.IsBadRequest(err)).To(BeTrue())
		})

		t.Run("fails if resourceGroup doesn't exist", func(t *testing.T) {
			g := NewWithT(t)

			obj := &cloudv1.CloudMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name: "bar",
				},
			}
			err := c.Update("bar", obj)
			g.Expect(err).To(HaveOccurred())
			g.Expect(apierrors.IsBadRequest(err)).To(BeTrue())
		})

		t.Run("fails if Object doesn't exist", func(t *testing.T) {
			g := NewWithT(t)

			obj := &cloudv1.CloudMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name: "bar",
				},
			}
			err := c.Update("foo", obj)
			g.Expect(err).To(HaveOccurred())
			g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})

		t.Run("update - no changes", func(t *testing.T) {
			g := NewWithT(t)

			objBefore := createMachine(t, c, "foo", "bar")

			objUpdate := objBefore.DeepCopy()
			err = c.Update("foo", objUpdate)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(objBefore).To(BeComparableTo(objUpdate), "obj before and after must be the same")

			g.Expect(h.Events()).ToNot(ContainElement("foo, CloudMachine=bar, Updated"))
		})

		t.Run("update - with changes", func(t *testing.T) {
			g := NewWithT(t)

			objBefore := createMachine(t, c, "foo", "baz")

			time.Sleep(1 * time.Second)

			objUpdate := objBefore.DeepCopy()
			objUpdate.Labels = map[string]string{"foo": "bar"}
			err = c.Update("foo", objUpdate)
			g.Expect(err).ToNot(HaveOccurred())

			// Check all the computed fields are as expected.
			g.Expect(objBefore.GetAnnotations()[lastSyncTimeAnnotation]).ToNot(Equal(objUpdate.GetAnnotations()[lastSyncTimeAnnotation]), "last sync version must be changed")
			objBefore.Annotations = objUpdate.Annotations
			g.Expect(objBefore.GetResourceVersion()).ToNot(Equal(objUpdate.GetResourceVersion()), "Object version must be changed")
			objBefore.SetResourceVersion(objUpdate.GetResourceVersion())
			objBefore.Labels = objUpdate.Labels
			g.Expect(objUpdate.GetGeneration()).To(Equal(objBefore.GetGeneration()+1), "Object Generation must increment")
			objBefore.Generation = objUpdate.GetGeneration()
			g.Expect(objBefore).To(BeComparableTo(objUpdate), "everything else must be the same")

			g.Expect(h.Events()).To(ContainElement("foo, CloudMachine=baz, Updated"))
		})

		t.Run("update - with conflict", func(t *testing.T) {
			g := NewWithT(t)

			objBefore := createMachine(t, c, "foo", "bazz")

			objUpdate1 := objBefore.DeepCopy()
			objUpdate1.Labels = map[string]string{"foo": "bar"}

			time.Sleep(1 * time.Second)

			err = c.Update("foo", objUpdate1)
			g.Expect(err).ToNot(HaveOccurred())

			objUpdate2 := objBefore.DeepCopy()
			err = c.Update("foo", objUpdate2)
			g.Expect(err).To(HaveOccurred())
			g.Expect(apierrors.IsConflict(err)).To(BeTrue())

			// TODO: check if it has been informed only once
		})

		t.Run("Update with owner references", func(t *testing.T) {
			t.Run("fails for invalid owner reference", func(t *testing.T) {
				g := NewWithT(t)

				obj := &cloudv1.CloudMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name: "child",
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: "something/not/valid",
								Kind:       "ParentKind",
								Name:       "parent",
							},
						},
					},
				}
				err := c.Update("foo", obj)
				g.Expect(err).To(HaveOccurred())
				g.Expect(apierrors.IsBadRequest(err)).To(BeTrue())
			})
			t.Run("fails if referenced object does not exists", func(t *testing.T) {
				g := NewWithT(t)

				objBefore := createMachine(t, c, "foo", "child1")

				objUpdate := objBefore.DeepCopy()
				objUpdate.OwnerReferences = []metav1.OwnerReference{
					{
						APIVersion: cloudv1.GroupVersion.String(),
						Kind:       "CloudMachine",
						Name:       "parentx",
					},
				}
				err := c.Update("foo", objUpdate)
				g.Expect(err).To(HaveOccurred())
				g.Expect(apierrors.IsBadRequest(err)).To(BeTrue())
			})
			t.Run("updates takes care of ownedObjects", func(t *testing.T) {
				g := NewWithT(t)

				createMachine(t, c, "foo", "parent1")
				createMachine(t, c, "foo", "parent2")

				objBefore := &cloudv1.CloudMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name: "child2",
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: cloudv1.GroupVersion.String(),
								Kind:       "CloudMachine",
								Name:       "parent1",
							},
						},
					},
				}
				err := c.Create("foo", objBefore)
				g.Expect(err).ToNot(HaveOccurred())

				objUpdate := objBefore.DeepCopy()
				objUpdate.OwnerReferences[0].Name = "parent2"

				err = c.Update("foo", objUpdate)
				g.Expect(err).ToNot(HaveOccurred())

				// Check internal state of the tracker
				c.lock.RLock()
				defer c.lock.RUnlock()

				parent1Ref := ownReference{gvk: cloudv1.GroupVersion.WithKind(cloudv1.CloudMachineKind), key: types.NamespacedName{Namespace: "", Name: "parent1"}}
				g.Expect(c.resourceGroups["foo"].ownedObjects).ToNot(HaveKey(parent1Ref), "there should not be ownedObjects for parent1")
				parent2Ref := ownReference{gvk: cloudv1.GroupVersion.WithKind(cloudv1.CloudMachineKind), key: types.NamespacedName{Namespace: "", Name: "parent2"}}
				g.Expect(c.resourceGroups["foo"].ownedObjects).To(HaveKey(parent2Ref), "there should be ownedObjects for parent2")
				childRef := ownReference{gvk: cloudv1.GroupVersion.WithKind(cloudv1.CloudMachineKind), key: types.NamespacedName{Namespace: "", Name: "child2"}}
				g.Expect(c.resourceGroups["foo"].ownedObjects[parent2Ref]).To(HaveKey(childRef), "parent2 should own child")
			})
		})

		// TODO: test system managed fields cannot be updated (see before update)

		// TODO: update list
	})

	// TODO: test patch

	t.Run("delete objects", func(t *testing.T) {
		g := NewWithT(t)

		c := NewCache(scheme).(*cache)
		h := &fakeHandler{}
		i, err := c.GetInformer(context.TODO(), &cloudv1.CloudMachine{})
		g.Expect(err).ToNot(HaveOccurred())
		err = i.AddEventHandler(h)
		g.Expect(err).ToNot(HaveOccurred())

		c.AddResourceGroup("foo")

		t.Run("fails if resourceGroup is empty", func(t *testing.T) {
			g := NewWithT(t)

			obj := &cloudv1.CloudMachine{}
			err := c.Delete("", obj)
			g.Expect(err).To(HaveOccurred())
			g.Expect(apierrors.IsBadRequest(err)).To(BeTrue())
		})

		t.Run("fails if Object is nil", func(t *testing.T) {
			g := NewWithT(t)

			err := c.Delete("foo", nil)
			g.Expect(err).To(HaveOccurred())
			g.Expect(apierrors.IsBadRequest(err)).To(BeTrue())
		})

		t.Run("fails if name is empty", func(t *testing.T) {
			g := NewWithT(t)

			obj := &cloudv1.CloudMachine{}
			err := c.Delete("foo", obj)
			g.Expect(err).To(HaveOccurred())
			g.Expect(apierrors.IsBadRequest(err)).To(BeTrue())
		})

		t.Run("fails if unknown kind", func(*testing.T) {
			// TODO implement test case
		})

		t.Run("fails if gvk doesn't exist", func(t *testing.T) {
			g := NewWithT(t)

			obj := &cloudv1.CloudMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name: "bar",
				},
			}
			err := c.Delete("foo", obj)
			g.Expect(err).To(HaveOccurred())
			g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})

		t.Run("fails if object doesn't exist", func(t *testing.T) {
			g := NewWithT(t)

			createMachine(t, c, "foo", "barz")

			obj := &cloudv1.CloudMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name: "bar",
				},
			}
			err := c.Delete("foo", obj)
			g.Expect(err).To(HaveOccurred())
			g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})

		t.Run("delete", func(t *testing.T) {
			g := NewWithT(t)

			obj := createMachine(t, c, "foo", "bar")

			err := c.Delete("foo", obj)
			g.Expect(err).ToNot(HaveOccurred())

			c.lock.RLock()
			defer c.lock.RUnlock()

			g.Expect(c.resourceGroups["foo"].objects).To(HaveKey(cloudv1.GroupVersion.WithKind(cloudv1.CloudMachineKind)), "gvk must exist in object tracker for foo")
			g.Expect(c.resourceGroups["foo"].objects[cloudv1.GroupVersion.WithKind(cloudv1.CloudMachineKind)]).ToNot(HaveKey(types.NamespacedName{Name: "bar"}), "Object bar must not exist in object tracker for foo")

			g.Expect(h.Events()).To(ContainElement("foo, CloudMachine=bar, Deleted"))
		})

		t.Run("delete with finalizers", func(t *testing.T) {
			g := NewWithT(t)

			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()

			c.garbageCollectorQueue = workqueue.NewTypedRateLimitingQueue[any](workqueue.DefaultTypedControllerRateLimiter[any]())
			go func() {
				<-ctx.Done()
				c.garbageCollectorQueue.ShutDown()
			}()

			objBefore := &cloudv1.CloudMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "baz",
					Finalizers: []string{"foo"},
				},
			}
			err := c.Create("foo", objBefore)
			g.Expect(err).ToNot(HaveOccurred())

			time.Sleep(1 * time.Second)

			err = c.Delete("foo", objBefore)
			g.Expect(err).ToNot(HaveOccurred())

			objAfterUpdate := &cloudv1.CloudMachine{}
			err = c.Get("foo", types.NamespacedName{Name: "baz"}, objAfterUpdate)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(objBefore.GetDeletionTimestamp().IsZero()).To(BeTrue(), "deletion timestamp before delete must not be set")
			g.Expect(objAfterUpdate.GetDeletionTimestamp().IsZero()).To(BeFalse(), "deletion timestamp after delete must be set")
			objBefore.DeletionTimestamp = objAfterUpdate.DeletionTimestamp
			g.Expect(objBefore.GetAnnotations()[lastSyncTimeAnnotation]).ToNot(Equal(objAfterUpdate.GetAnnotations()[lastSyncTimeAnnotation]), "last sync version must be changed")
			objBefore.Annotations = objAfterUpdate.Annotations
			g.Expect(objBefore.GetResourceVersion()).ToNot(Equal(objAfterUpdate.GetResourceVersion()), "Object version must be changed")
			objBefore.SetResourceVersion(objAfterUpdate.GetResourceVersion())
			objBefore.Labels = objAfterUpdate.Labels
			g.Expect(objAfterUpdate.GetGeneration()).To(Equal(objBefore.GetGeneration()+1), "Object Generation must increment")
			objBefore.Generation = objAfterUpdate.GetGeneration()
			g.Expect(objBefore).To(BeComparableTo(objAfterUpdate), "everything else must be the same")

			g.Expect(h.Events()).To(ContainElement("foo, CloudMachine=baz, Deleted"))

			cancel()
		})

		t.Run("delete with owner reference", func(t *testing.T) {
			g := NewWithT(t)

			createMachine(t, c, "foo", "parent3")

			obj := &cloudv1.CloudMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name: "child3",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: cloudv1.GroupVersion.String(),
							Kind:       "CloudMachine",
							Name:       "parent3",
						},
					},
				},
			}
			err := c.Create("foo", obj)
			g.Expect(err).ToNot(HaveOccurred())

			obj = &cloudv1.CloudMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name: "grandchild3",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: cloudv1.GroupVersion.String(),
							Kind:       "CloudMachine",
							Name:       "child3",
						},
					},
				},
			}
			err = c.Create("foo", obj)
			g.Expect(err).ToNot(HaveOccurred())

			obj = &cloudv1.CloudMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name: "parent3",
				},
			}
			err = c.Delete("foo", obj)
			g.Expect(err).ToNot(HaveOccurred())

			c.lock.RLock()
			defer c.lock.RUnlock()

			g.Expect(c.resourceGroups["foo"].objects).To(HaveKey(cloudv1.GroupVersion.WithKind(cloudv1.CloudMachineKind)), "gvk must exist in object tracker for foo")
			g.Expect(c.resourceGroups["foo"].objects[cloudv1.GroupVersion.WithKind(cloudv1.CloudMachineKind)]).ToNot(HaveKey(types.NamespacedName{Name: "parent3"}), "Object parent3 must not exist in object tracker for foo")
			g.Expect(c.resourceGroups["foo"].objects[cloudv1.GroupVersion.WithKind(cloudv1.CloudMachineKind)]).ToNot(HaveKey(types.NamespacedName{Name: "child3"}), "Object child3 must not exist in object tracker for foo")
			g.Expect(c.resourceGroups["foo"].objects[cloudv1.GroupVersion.WithKind(cloudv1.CloudMachineKind)]).ToNot(HaveKey(types.NamespacedName{Name: "grandchild3"}), "Object grandchild3 must not exist in object tracker for foo")
		})

		// TODO: test finalizers and ownner references together
	})
}

func createMachine(t *testing.T, c *cache, resourceGroup, name string) *cloudv1.CloudMachine {
	t.Helper()

	g := NewWithT(t)

	obj := &cloudv1.CloudMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	err := c.Create(resourceGroup, obj)
	g.Expect(err).ToNot(HaveOccurred())

	return obj
}

var _ Informer = &fakeInformer{}

type fakeInformer struct {
	handler InformEventHandler
}

func (i *fakeInformer) AddEventHandler(handler InformEventHandler) error {
	i.handler = handler
	return nil
}

func (i *fakeInformer) RemoveEventHandler(_ InformEventHandler) error {
	i.handler = nil
	return nil
}

func (i *fakeInformer) InformCreate(resourceGroup string, obj client.Object) {
	i.handler.OnCreate(resourceGroup, obj)
}

func (i *fakeInformer) InformUpdate(resourceGroup string, oldObj, newObj client.Object) {
	i.handler.OnUpdate(resourceGroup, oldObj, newObj)
}

func (i *fakeInformer) InformDelete(resourceGroup string, res client.Object) {
	i.handler.OnDelete(resourceGroup, res)
}

func (i *fakeInformer) InformGeneric(resourceGroup string, res client.Object) {
	i.handler.OnGeneric(resourceGroup, res)
}

var _ InformEventHandler = &fakeHandler{}

type fakeHandler struct {
	events []string
}

func (h *fakeHandler) Events() []string {
	return h.events
}

func (h *fakeHandler) OnCreate(resourceGroup string, obj client.Object) {
	h.events = append(h.events, fmt.Sprintf("%s, %s=%s, Created", resourceGroup, obj.GetObjectKind().GroupVersionKind().Kind, obj.GetName()))
}

func (h *fakeHandler) OnUpdate(resourceGroup string, _, newObj client.Object) {
	h.events = append(h.events, fmt.Sprintf("%s, %s=%s, Updated", resourceGroup, newObj.GetObjectKind().GroupVersionKind().Kind, newObj.GetName()))
}

func (h *fakeHandler) OnDelete(resourceGroup string, obj client.Object) {
	h.events = append(h.events, fmt.Sprintf("%s, %s=%s, Deleted", resourceGroup, obj.GetObjectKind().GroupVersionKind().Kind, obj.GetName()))
}

func (h *fakeHandler) OnGeneric(resourceGroup string, obj client.Object) {
	h.events = append(h.events, fmt.Sprintf("%s, %s=%s, Generic", resourceGroup, obj.GetObjectKind().GroupVersionKind().Kind, obj.GetName()))
}
