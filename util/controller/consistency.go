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
	"fmt"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/resourceversion"
	kcache "k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Note: This util has been initially copied from: https://github.com/kubernetes/kubernetes/blob/v1.36.0/pkg/controller/util/consistency/consistency.go.

type consistencyStore interface {
	// WroteAt records a written RV for an owned resource.
	WroteAt(owner types.NamespacedName, ownerUID types.UID, gvkt GroupVersionKindType, rv string)
	// Clear wipes the owner if the UID matches, if left empty it will wipe no
	// matter what the UID is.
	Clear(owner types.NamespacedName, ownerUID types.UID)
	// EnsureReady queries the consistencyStore to check whether or not the
	// stores records are up to date, returning an error if they are not.
	EnsureReady(ctx context.Context, owner types.NamespacedName) ([]consistencyError, error)
}

// consistencyError is an error type returned by EnsureReady with information
// about the resource versions and GroupKind that caused the error.
type consistencyError struct {
	ReadRV               string
	WroteRV              string
	GroupVersionKindType GroupVersionKindType
}

var _ consistencyStore = &realConsistencyStore{}

type informerGetter interface {
	GetInformer(ctx context.Context, obj client.Object, opts ...cache.InformerGetOption) (cache.Informer, error)
}

type realConsistencyStore struct {
	// writesLock guards reads/additions/deletions to the writes map.
	// individual records are responsible for managing their own thread safety.
	writesLock sync.RWMutex
	// writes is a map of owner -> ownerRecord
	writes map[types.NamespacedName]*ownerRecord

	storesLock sync.RWMutex
	stores     map[GroupVersionKindType]kcache.Store

	scheme         *runtime.Scheme
	informerGetter informerGetter
}

func newConsistencyStore(scheme *runtime.Scheme, cache informerGetter) *realConsistencyStore {
	return &realConsistencyStore{
		writes:         map[types.NamespacedName]*ownerRecord{},
		stores:         map[GroupVersionKindType]kcache.Store{},
		scheme:         scheme,
		informerGetter: cache,
	}
}

// getWrittenRecord returns the record for the given owner, or nil if no record exists.
func (c *realConsistencyStore) getWrittenRecord(owner types.NamespacedName) *ownerRecord {
	c.writesLock.RLock()
	defer c.writesLock.RUnlock()
	return c.writes[owner]
}

// ensureWrittenRecord returns a ownerRecord for the given owner and ownerUID.
// If there is no current record, one is created.
// If there is a current record with a different ownerUID, it is replaced with an empty record for the specified ownerUID.
func (c *realConsistencyStore) ensureWrittenRecord(owner types.NamespacedName, ownerUID types.UID) *ownerRecord {
	// fast path, already exists
	if record := c.getWrittenRecord(owner); record != nil && record.ownerUID == ownerUID {
		return record
	}

	// slow path, init
	c.writesLock.Lock()
	defer c.writesLock.Unlock()
	// check again after write lock
	if record := c.writes[owner]; record != nil && record.ownerUID == ownerUID {
		return record
	}
	// initialize to the given uid
	record := newOwnerRecord(ownerUID)
	c.writes[owner] = record
	return record
}

// WroteAt writes the latest written RV if it is greater than the currently
// written RV for the owner.
func (c *realConsistencyStore) WroteAt(owner types.NamespacedName, ownerUID types.UID, gvkt GroupVersionKindType, rv string) {
	c.ensureWrittenRecord(owner, ownerUID).WroteAt(gvkt, rv)
}

// Clear deletes the record for owner if it exists and matches the specified
// ownerUID (or the specified ownerUID is empty).
func (c *realConsistencyStore) Clear(owner types.NamespacedName, ownerUID types.UID) {
	// deleted owners typically have an existing record, not worth checking the fast path for missing records
	c.writesLock.Lock()
	defer c.writesLock.Unlock()
	if record := c.writes[owner]; record != nil && (len(ownerUID) == 0 || record.ownerUID == ownerUID) {
		delete(c.writes, owner)
	}
}

// EnsureReady returns nil if observed resource versions are at least as new as
// any recorded versions for the given owner, otherwise returning the error of
// what happened. Must not be called concurrent with WroteAt for the same owner.
func (c *realConsistencyStore) EnsureReady(ctx context.Context, owner types.NamespacedName) ([]consistencyError, error) {
	record := c.getWrittenRecord(owner)
	if record == nil {
		return nil, nil
	}
	consistencyErrors, err := record.EnsureReady(ctx, c)
	if err != nil {
		return nil, err
	}

	if len(consistencyErrors) == 0 {
		c.Clear(owner, record.ownerUID)
		return nil, nil
	}
	return consistencyErrors, nil
}

type ownerRecord struct {
	// ownerUID must not be mutated after creation
	ownerUID types.UID

	versionsLock sync.Mutex
	versions     map[GroupVersionKindType]string
}

// ObjectType is the type of an object.
type ObjectType string

var (
	// ObjectTypeUnstructured is the type of an object for Unstructured.
	ObjectTypeUnstructured ObjectType = "Unstructured"

	// ObjectTypePartialObjectMetadata is the type of an object for PartialObjectMetadata.
	ObjectTypePartialObjectMetadata ObjectType = "PartialObjectMetadata"

	// ObjectTypeStructured is the type of an object for a regular structured object.
	ObjectTypeStructured ObjectType = "Structured"
)

// GroupVersionKindType described the GVK and type of an object.
type GroupVersionKindType struct {
	schema.GroupVersionKind
	Type ObjectType
}

// StructuredObject is a util that allows to create a GroupVersionKindType for a structured object.
func StructuredObject(gv schema.GroupVersion, kind string) GroupVersionKindType {
	return GroupVersionKindType{
		GroupVersionKind: schema.GroupVersionKind{
			Group:   gv.Group,
			Version: gv.Version,
			Kind:    kind,
		},
		Type: ObjectTypeStructured,
	}
}

// Unstructured is a util that allows to create a GroupVersionKindType for an Unstructured object.
func Unstructured(u *unstructured.Unstructured) GroupVersionKindType {
	return GroupVersionKindType{
		GroupVersionKind: u.GroupVersionKind(),
		Type:             ObjectTypeUnstructured,
	}
}

// PartialObjectMetadata is a util that allows to create a GroupVersionKindType for an PartialObjectMetadata object.
func PartialObjectMetadata(u *metav1.PartialObjectMetadata) GroupVersionKindType {
	return GroupVersionKindType{
		GroupVersionKind: u.GroupVersionKind(),
		Type:             ObjectTypePartialObjectMetadata,
	}
}

func newOwnerRecord(ownerUID types.UID) *ownerRecord {
	return &ownerRecord{ownerUID: ownerUID, versions: map[GroupVersionKindType]string{}}
}

// WroteAt increments the written resource version of an ownerRecord if it is
// the newest seen resource version for that resource.
func (w *ownerRecord) WroteAt(gvkt GroupVersionKindType, rv string) {
	w.versionsLock.Lock()
	defer w.versionsLock.Unlock()
	if _, ok := w.versions[gvkt]; !ok {
		w.versions[gvkt] = rv
		return
	}
	cmp, err := resourceversion.CompareResourceVersion(w.versions[gvkt], rv)
	if err == nil && cmp >= 0 {
		return
	}
	w.versions[gvkt] = rv
}

// EnsureReady checks whether or not the ownerRecord is ready compared to the
// read resource versions in the consistency store.
func (w *ownerRecord) EnsureReady(ctx context.Context, c *realConsistencyStore) ([]consistencyError, error) {
	w.versionsLock.Lock()
	defer w.versionsLock.Unlock()
	var errs []consistencyError
	for gvkt, wroteRV := range w.versions {
		store, err := getStore(ctx, c, gvkt)
		if err != nil {
			return nil, err
		}
		readRV := store.LastStoreSyncResourceVersion()
		if readRV == "" {
			// As we are also creating Informers dynamically in Reconcile funcs there might be informers that are
			// not synced yet. In that case we want to wait for informers to sync and catch up.
			errs = append(errs, consistencyError{
				WroteRV:              wroteRV,
				ReadRV:               readRV,
				GroupVersionKindType: gvkt,
			})
			continue
		}
		i, err := resourceversion.CompareResourceVersion(wroteRV, readRV)
		if err != nil {
			// comparison errors indicate there's a data problem with resource versions, continue so we don't block syncing
			continue
		}
		if i > 0 {
			// read version is not as new as owner version, not ready
			errs = append(errs, consistencyError{
				WroteRV:              wroteRV,
				ReadRV:               readRV,
				GroupVersionKindType: gvkt,
			})
		}
	}
	return errs, nil
}

func getStore(ctx context.Context, c *realConsistencyStore, gvkt GroupVersionKindType) (kcache.Store, error) {
	c.storesLock.RLock()
	store, exists := c.stores[gvkt]
	c.storesLock.RUnlock()

	if exists {
		return store, nil
	}

	c.storesLock.Lock()
	defer c.storesLock.Unlock()

	// Check again now that we have the write lock.
	store, exists = c.stores[gvkt]
	if exists {
		return store, nil
	}

	var obj client.Object
	switch gvkt.Type {
	case ObjectTypeUnstructured:
		obj = &unstructured.Unstructured{}
		obj.GetObjectKind().SetGroupVersionKind(gvkt.GroupVersionKind)
	case ObjectTypePartialObjectMetadata:
		obj = &metav1.PartialObjectMetadata{}
		obj.GetObjectKind().SetGroupVersionKind(gvkt.GroupVersionKind)
	case ObjectTypeStructured:
		runtimeObj, err := c.scheme.New(gvkt.GroupVersionKind)
		if err != nil {
			return nil, fmt.Errorf("failed to create %s object: %w", gvkt.Kind, err)
		}
		var ok bool
		obj, ok = runtimeObj.(client.Object)
		if !ok {
			return nil, fmt.Errorf("failed to cast %s object to client.Object: %w", gvkt.Kind, err)
		}
	}

	// Note: This creates the informer if it doesn't exist already, but it doesn't  wait for the cache to sync.
	informer, err := c.informerGetter.GetInformer(ctx, obj, cache.BlockUntilSynced(false))
	if err != nil {
		return nil, fmt.Errorf("failed to create %s informer: %w", gvkt.Kind, err)
	}
	sharedIndexInformer, ok := informer.(kcache.SharedIndexInformer)
	if !ok {
		// Note: This should never happen as controller-runtime only uses SharedIndexInformer.
		return nil, fmt.Errorf("failed to cast %s informer to SharedIndexInformer", gvkt.Kind)
	}

	c.stores[gvkt] = sharedIndexInformer.GetStore()
	return c.stores[gvkt], nil
}
