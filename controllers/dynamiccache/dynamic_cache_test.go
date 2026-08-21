/*
Copyright The Kubernetes Authors.

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

package dynamiccache

import (
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	contractv1 "sigs.k8s.io/cluster-api/internal/contract/api/v1beta2"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestDynamicCache(t *testing.T) {
	g := NewWithT(t)

	ns, err := env.CreateNamespace(ctx, "dynamiccache")
	g.Expect(err).ToNot(HaveOccurred())

	// unstructuredObjectType is used for GetUnstructured below. It doesn't need any Go types
	// configured because GetUnstructured always works with unstructured.Unstructured.
	const unstructuredObjectType ObjectType = "InfraCluster"

	// contractObjectType is used for GetContractObject/ListContractObjects/Watch below.
	const contractObjectType ObjectType = "InfraMachine"

	byObjectTypeOptions := map[ObjectType]ByObjectTypeOptions{
		unstructuredObjectType: {},
		contractObjectType: {
			ContractObj: map[string]client.Object{
				"v1beta2": &contractv1.InfraMachine{},
			},
			ContractObjList: map[string]client.ObjectList{
				"v1beta2": &contractv1.InfraMachineList{},
			},
		},
	}

	dc := New(env.Manager, env.GetClient(), byObjectTypeOptions, "dynamiccache-test", "")

	infraClusterGVK := schema.GroupVersionKind{
		Group:   builder.InfrastructureGroupVersion.Group,
		Version: builder.InfrastructureGroupVersion.Version,
		Kind:    builder.GenericInfrastructureClusterKind,
	}
	infraClusterGK := infraClusterGVK.GroupKind()

	infraMachineGVK := schema.GroupVersionKind{
		Group:   builder.InfrastructureGroupVersion.Group,
		Version: builder.InfrastructureGroupVersion.Version,
		Kind:    builder.GenericInfrastructureMachineKind,
	}
	infraMachineGK := infraMachineGVK.GroupKind()

	// Seed a GenericInfrastructureCluster and a GenericInfrastructureMachine directly via the
	// envtest client, so DynamicCache has something to Get/List/Watch below.
	infraCluster := &unstructured.Unstructured{}
	infraCluster.SetGroupVersionKind(infraClusterGVK)
	infraCluster.SetName("test-cluster")
	infraCluster.SetNamespace(ns.Name)
	g.Expect(env.CreateAndWait(ctx, infraCluster)).To(Succeed())
	defer func() {
		g.Expect(env.CleanupAndWait(ctx, infraCluster)).To(Succeed())
	}()

	infraMachine := &unstructured.Unstructured{}
	infraMachine.SetGroupVersionKind(infraMachineGVK)
	infraMachine.SetName("test-machine")
	infraMachine.SetNamespace(ns.Name)
	g.Expect(env.CreateAndWait(ctx, infraMachine)).To(Succeed())
	defer func() {
		g.Expect(env.CleanupAndWait(ctx, infraMachine)).To(Succeed())
	}()

	infraClusterKey := client.ObjectKey{Namespace: ns.Name, Name: "test-cluster"}
	infraMachineKey := client.ObjectKey{Namespace: ns.Name, Name: "test-machine"}

	// GetCache and GetWriter must report "not found" before any cache has been created for clusterGVK.
	_, exists := dc.GetCache(ctx, infraClusterGVK)
	g.Expect(exists).To(BeFalse())

	_, exists = dc.GetWriter(ctx, infraClusterGVK)
	g.Expect(exists).To(BeFalse())

	// GetUnstructured creates the cache for infraClusterGK on first use and returns the seeded object.
	u, err := dc.GetUnstructured(ctx, infraClusterGK, infraClusterKey, unstructuredObjectType)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(u.GetName()).To(Equal("test-cluster"))
	g.Expect(u.GetNamespace()).To(Equal(ns.Name))
	g.Expect(u.GroupVersionKind()).To(Equal(infraClusterGVK))

	// GetCache and GetWriter must now report the cache created above.
	c, exists := dc.GetCache(ctx, infraClusterGVK)
	g.Expect(exists).To(BeTrue())
	g.Expect(c).ToNot(BeNil())

	w, exists := dc.GetWriter(ctx, infraClusterGVK)
	g.Expect(exists).To(BeTrue())
	g.Expect(w).ToNot(BeNil())
	g.Expect(w.Scheme()).ToNot(BeNil())

	// GetContractObject creates the cache for infraMachineGVK on first use, resolving the contract Go type.
	gotGVK, obj, err := dc.GetContractObject(ctx, infraMachineGK, infraMachineKey, contractObjectType)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(gotGVK).To(Equal(infraMachineGVK))
	infraMachineObj, ok := obj.(*contractv1.InfraMachine)
	g.Expect(ok).To(BeTrue())
	g.Expect(infraMachineObj.Name).To(Equal("test-machine"))

	// ListContractObjects lists all objects of the given GroupKind using the contract Go type,
	// reusing the cache created by GetContractObject above.
	gotGVK, objList, err := dc.ListContractObjects(ctx, infraMachineGK, contractObjectType)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(gotGVK).To(Equal(infraMachineGVK))
	infraMachineList, ok := objList.(*contractv1.InfraMachineList)
	g.Expect(ok).To(BeTrue())
	g.Expect(infraMachineList.Items).To(HaveLen(1))
	g.Expect(infraMachineList.Items[0].Name).To(Equal("test-machine"))

	// Watch adds a watch for the first call and is a no-op for subsequent calls with the same watcher name.
	watcher := &countingSourceWatcher{}
	g.Expect(dc.Watch(ctx, "test-watcher", watcher, infraMachineGK, contractObjectType, handler.Funcs{})).To(Succeed())
	g.Expect(watcher.calls).To(Equal(1))
	g.Expect(dc.Watch(ctx, "test-watcher", watcher, infraMachineGK, contractObjectType, handler.Funcs{})).To(Succeed())
	g.Expect(watcher.calls).To(Equal(1))

	// GetContractObject with an ObjectType that is not configured on the DynamicCache returns an error.
	_, _, err = dc.GetContractObject(ctx, infraMachineGK, infraMachineKey, "unconfigured")
	g.Expect(err).To(HaveOccurred())

	// GetContractObject for infraClusterGK reuses the cache entry keyed by infraClusterGVK, which was created
	// above via GetUnstructured using unstructured.Unstructured. Asking for the contract Go types
	// instead must fail rather than silently returning data through the mismatched cache.
	_, _, err = dc.GetContractObject(ctx, infraClusterGK, infraClusterKey, contractObjectType)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("cache already exists for Go types"))

	// Conversely, GetUnstructured for infraMachineGK reuses the cache entry keyed by infraMachineGVK, which was
	// created above via GetContractObject using the contract Go types. Asking for
	// unstructured.Unstructured instead must fail the same way.
	_, err = dc.GetUnstructured(ctx, infraMachineGK, infraMachineKey, unstructuredObjectType)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("cache already exists for Go types"))
}

// countingSourceWatcher is a SourceWatcher that counts how often Watch has been called, so tests
// can assert DynamicCache.Watch's de-duplication behavior.
type countingSourceWatcher struct {
	calls int
}

func (f *countingSourceWatcher) Watch(_ source.TypedSource[reconcile.Request]) error {
	f.calls++
	return nil
}

var _ SourceWatcher = &countingSourceWatcher{}
