/*
Copyright 2026 The Kubernetes Authors.

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

package controller

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	ctrlcache "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

func TestOwnerRecord_WroteAt(t *testing.T) {
	uid := types.UID("owner-uid-1")
	or := newOwnerRecord(uid)
	assert.Equal(t, uid, or.ownerUID)
	require.NotNil(t, or.versions)

	gvktPod := StructuredObject(schema.GroupVersion{Group: "", Version: "v1"}, "Pod")
	gvktDS := StructuredObject(schema.GroupVersion{Group: "apps", Version: "v1"}, "DaemonSet")

	// First write
	or.WroteAt(gvktPod, "5")
	assert.Equal(t, "5", or.versions[gvktPod])

	// Second write (higher)
	or.WroteAt(gvktPod, "10")
	assert.Equal(t, "10", or.versions[gvktPod])

	// Third write (lower)
	or.WroteAt(gvktPod, "8")
	assert.Equal(t, "10", or.versions[gvktPod])

	// Write to different resource
	or.WroteAt(gvktDS, "1")
	assert.Equal(t, "1", or.versions[gvktDS])
	assert.Equal(t, "10", or.versions[gvktPod], "Pod version should be unchanged")
}

func TestOwnerRecord_IsReady(t *testing.T) {
	uid := types.UID("owner-uid-1")
	or := newOwnerRecord(uid)
	gvktPod := StructuredObject(schema.GroupVersion{Group: "", Version: "v1"}, "Pod")
	gvktDS := StructuredObject(schema.GroupVersion{Group: "apps", Version: "v1"}, "DaemonSet")
	podStore := cache.NewStore(cache.MetaNamespaceKeyFunc)
	dsStore := cache.NewStore(cache.MetaNamespaceKeyFunc)
	resourceStores := map[GroupVersionKindType]cache.Store{
		gvktPod: podStore,
		gvktDS:  dsStore,
	}

	store := &realConsistencyStore{
		writes: map[types.NamespacedName]*ownerRecord{},
		stores: resourceStores,
	}

	// Case 1: No writes. Should be ready.
	consistencyErrors, err := or.EnsureReady(t.Context(), store)
	require.Nil(t, err)
	require.Empty(t, consistencyErrors, "Should be ready if no writes are recorded")

	// Add a write
	or.WroteAt(gvktPod, "10")

	// Case 2: Write exists, but no reads. Not ready.
	consistencyErrors, err = or.EnsureReady(t.Context(), store)
	require.Nil(t, err)
	require.NotEmpty(t, consistencyErrors, "Not ready if read (0) < write")

	// Add a read, but it's lower
	podStore.Bookmark("5")

	// Case 3: Write exists, read is lower. Not ready.
	consistencyErrors, err = or.EnsureReady(t.Context(), store)
	require.Nil(t, err)
	require.NotEmpty(t, consistencyErrors, "Not ready if read < write")

	// Add a read, equal
	podStore.Bookmark("10")

	// Case 4: Write exists, read is equal. Ready.
	consistencyErrors, err = or.EnsureReady(t.Context(), store)
	require.Nil(t, err)
	require.Empty(t, consistencyErrors, "Ready if read == write")

	// Add a read, higher
	podStore.Bookmark("15")

	// Case 5: Write exists, read is higher. Ready.
	consistencyErrors, err = or.EnsureReady(t.Context(), store)
	require.Nil(t, err)
	require.Empty(t, consistencyErrors, "Ready if read > write")

	// Add a second write and read
	or.WroteAt(gvktDS, "100")
	dsStore.Bookmark("50")

	// Case 6: One resource ready, one not. Not ready.
	consistencyErrors, err = or.EnsureReady(t.Context(), store)
	require.Nil(t, err)
	require.NotEmpty(t, consistencyErrors, "Not ready if one of multiple writes is not ready (no read)")

	// Make the second one ready
	dsStore.Bookmark("100")

	// Case 7: All resources ready. Ready.
	consistencyErrors, err = or.EnsureReady(t.Context(), store)
	require.Nil(t, err)
	require.Empty(t, consistencyErrors, "Ready if all writes are ready")
}

func TestConsistencyStore_New(t *testing.T) {
	store := newConsistencyStore(nil, nil)
	require.NotNil(t, store)
	require.NotNil(t, store.writes)
	assert.Empty(t, store.writes)
	require.NotNil(t, store.stores)
	assert.Empty(t, store.stores)
}

func TestConsistencyStore_EnsureWrittenRecord(t *testing.T) {
	store := newConsistencyStore(nil, nil)
	owner := types.NamespacedName{Name: "owner1"}
	uid1 := types.UID("uid-1")
	uid2 := types.UID("uid-2")

	// Create new
	r1 := store.ensureWrittenRecord(owner, uid1)
	require.NotNil(t, r1)
	assert.Equal(t, uid1, r1.ownerUID)
	assert.Same(t, r1, store.writes[owner])

	// Get existing with same UID
	r2 := store.ensureWrittenRecord(owner, uid1)
	assert.Same(t, r1, r2, "Should return existing record for same UID")

	// Get existing with different UID (should replace)
	r3 := store.ensureWrittenRecord(owner, uid2)
	require.NotNil(t, r3)
	assert.NotSame(t, r1, r3, "Should be a new record")
	assert.Equal(t, uid2, r3.ownerUID)
	assert.Same(t, r3, store.writes[owner], "New record should replace old one in map")
	assert.Empty(t, r3.versions, "New record should be empty")

	// Check that old record is detached
	gvktPod := StructuredObject(schema.GroupVersion{Group: "", Version: "v1"}, "Pod")
	r1.WroteAt(gvktPod, "10") // Write to old record
	assert.Empty(t, r3.versions, "Write to old record should not affect new record")
}

func TestConsistencyStore_EnsureWrittenRecord_Concurrent(t *testing.T) {
	store := newConsistencyStore(nil, nil)
	owner := types.NamespacedName{Name: "owner1"}
	uid1 := types.UID("uid-1")
	uid2 := types.UID("uid-2")

	wg := sync.WaitGroup{}
	numGoroutines := 50

	// Concurrent creation with same UID
	var firstRecord *ownerRecord
	var once sync.Once
	for range numGoroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := store.ensureWrittenRecord(owner, uid1)
			assert.Equal(t, uid1, r.ownerUID)
			once.Do(func() {
				firstRecord = r
			})
			assert.Same(t, firstRecord, r)
		}()
	}
	wg.Wait()
	require.NotNil(t, firstRecord)
	assert.Len(t, store.writes, 1)

	// Concurrent replacement with new UID
	var replacementRecord *ownerRecord
	var replaceOnce sync.Once
	for range numGoroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := store.ensureWrittenRecord(owner, uid2)
			assert.Equal(t, uid2, r.ownerUID)
			replaceOnce.Do(func() {
				replacementRecord = r
			})
			assert.Same(t, replacementRecord, r)
		}()
	}
	wg.Wait()
	require.NotNil(t, replacementRecord)
	assert.Len(t, store.writes, 1)
	assert.Same(t, replacementRecord, store.writes[owner])
	assert.NotSame(t, firstRecord, replacementRecord)
}

func TestConsistencyStore_WroteAt(t *testing.T) {
	store := newConsistencyStore(nil, nil)
	owner := types.NamespacedName{Name: "owner1"}
	uid1 := types.UID("uid-1")
	gvktPod := StructuredObject(schema.GroupVersion{Group: "", Version: "v1"}, "Pod")

	store.WroteAt(owner, uid1, gvktPod, "10")

	record := store.getWrittenRecord(owner)
	require.NotNil(t, record)
	assert.Equal(t, uid1, record.ownerUID)

	assert.Equal(t, "10", record.versions[gvktPod])

	// Write again
	store.WroteAt(owner, uid1, gvktPod, "20")
	assert.Equal(t, "20", record.versions[gvktPod])
}

func TestConsistencyStore_Clear(t *testing.T) {
	store := newConsistencyStore(nil, nil)
	owner1 := types.NamespacedName{Name: "owner1"}
	owner2 := types.NamespacedName{Name: "owner2"}
	uid1 := types.UID("uid-1")
	uid2 := types.UID("uid-2")

	// Setup
	r1 := store.ensureWrittenRecord(owner1, uid1)
	r2 := store.ensureWrittenRecord(owner2, uid2)
	require.Len(t, store.writes, 2)

	// Clear non-existent
	store.Clear(types.NamespacedName{Name: "non-existent"}, uid1)
	assert.Len(t, store.writes, 2)

	// Clear with wrong UID
	store.Clear(owner1, uid2)
	assert.Len(t, store.writes, 2, "Should not clear with wrong UID")
	assert.Same(t, r1, store.writes[owner1])

	// Clear with correct UID
	store.Clear(owner1, uid1)
	assert.Len(t, store.writes, 1, "Should clear with correct UID")
	assert.Nil(t, store.writes[owner1])
	assert.Same(t, r2, store.writes[owner2], "Other record should remain")

	// Re-add r1
	store.ensureWrittenRecord(owner1, uid1)
	require.Len(t, store.writes, 2)

	// Clear with empty UID
	store.Clear(owner1, "")
	assert.Len(t, store.writes, 1, "Should clear with empty UID")
	assert.Nil(t, store.writes[owner1])
	assert.Same(t, r2, store.writes[owner2])
}

func TestConsistencyStore_IsReady(t *testing.T) {
	owner1 := types.NamespacedName{Name: "owner1"}
	uid1 := types.UID("uid-1")
	gvktPod := StructuredObject(schema.GroupVersion{Group: "", Version: "v1"}, "Pod")
	podStore := cache.NewStore(cache.MetaNamespaceKeyFunc)
	resourceStores := map[GroupVersionKindType]cache.Store{
		gvktPod: podStore,
	}

	store := &realConsistencyStore{
		writes: map[types.NamespacedName]*ownerRecord{},
		stores: resourceStores,
	}

	// Case 1: No record. Ready.
	consistencyErrors, err := store.EnsureReady(t.Context(), owner1)
	require.Nil(t, err)
	require.Empty(t, consistencyErrors, "Ready if no record exists")

	// Add a write and initial read rv
	podStore.Bookmark("5")
	store.WroteAt(owner1, uid1, gvktPod, "10")

	// Case 2: Record exists, read < write. Not ready.
	consistencyErrors, err = store.EnsureReady(t.Context(), owner1)
	require.Nil(t, err)
	require.NotEmpty(t, consistencyErrors, "Not ready if read < write")

	// Add read, equal
	podStore.Bookmark("10")

	// Case 3: Record exists, read == write. Ready.
	consistencyErrors, err = store.EnsureReady(t.Context(), owner1)
	require.Nil(t, err)
	require.Empty(t, consistencyErrors, "Ready if read == write")

	// Add read, higher
	podStore.Bookmark("15")

	// Case 4: Record exists, read > write. Ready.
	consistencyErrors, err = store.EnsureReady(t.Context(), owner1)
	require.Nil(t, err)
	require.Empty(t, consistencyErrors, "Ready if read > write")

	// Assert that the record no longer exists, we no longer need to track the
	// reads as long as the read has been higher than the latest write.
	assert.Nil(t, store.getWrittenRecord(owner1), "Written record should no longer exist")
}

func TestConsistencyStore_getStore(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clusterv1.AddToScheme(scheme)

	c := newConsistencyStore(scheme, &fakeInformerGetter{})

	// Get store of *clusterv1.Machine informer.
	store, err := getStore(t.Context(), c, StructuredObject(clusterv1.GroupVersion, "Machine"))
	assert.Nil(t, err)
	_, ok := store.(*fakeStore).obj.(*clusterv1.Machine)
	assert.True(t, ok)

	// Get store of clusterv1.Machine Unstructured informer.
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(clusterv1.GroupVersion.WithKind("Machine"))
	store, err = getStore(t.Context(), c, Unstructured(u))
	assert.Nil(t, err)
	u, ok = store.(*fakeStore).obj.(*unstructured.Unstructured)
	assert.True(t, ok)
	assert.Equal(t, clusterv1.GroupVersion.WithKind("Machine"), u.GroupVersionKind())

	// Get store of clusterv1.Machine PartialObjectMeta informer.
	p := &metav1.PartialObjectMetadata{}
	p.SetGroupVersionKind(clusterv1.GroupVersion.WithKind("Machine"))
	store, err = getStore(t.Context(), c, PartialObjectMetadata(p))
	assert.Nil(t, err)
	p, ok = store.(*fakeStore).obj.(*metav1.PartialObjectMetadata)
	assert.True(t, ok)
	assert.Equal(t, clusterv1.GroupVersion.WithKind("Machine"), p.GroupVersionKind())

	assert.Len(t, c.stores, 3)
}

type fakeInformerGetter struct{}

func (f *fakeInformerGetter) GetInformer(_ context.Context, obj client.Object, _ ...ctrlcache.InformerGetOption) (ctrlcache.Informer, error) {
	return &fakeInformer{obj: obj}, nil
}

var _ cache.SharedIndexInformer = &fakeInformer{}

type fakeInformer struct {
	obj client.Object
	cache.SharedIndexInformer
}

func (f *fakeInformer) GetStore() cache.Store {
	return &fakeStore{obj: f.obj}
}

type fakeStore struct {
	obj client.Object
	cache.Store
}
