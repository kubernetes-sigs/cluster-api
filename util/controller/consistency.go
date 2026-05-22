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
	"fmt"
	"sync"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/resourceversion"
)

// Note: This util has been initially copied from: https://github.com/kubernetes/kubernetes/blob/v1.36.0/pkg/controller/util/consistency/consistency.go.

type consistencyStore interface {
	// WroteAt records a written RV for an owned resource.
	WroteAt(owner types.NamespacedName, ownerUID types.UID, resource schema.GroupResource, rv string)
	// Clear wipes the owner if the UID matches, if left empty it will wipe no
	// matter what the UID is.
	Clear(owner types.NamespacedName, ownerUID types.UID)
	// EnsureReady queries the consistencyStore to check whether or not the
	// stores records are up to date, returning an error if they are not.
	EnsureReady(owner types.NamespacedName) []consistencyError
}

// consistencyError is an error type returned by EnsureReady with information
// about the resource versions and GroupKind that caused the error.
type consistencyError struct {
	ReadRV        string
	WroteRV       string
	GroupResource schema.GroupResource
}

func (c *consistencyError) Error() string {
	return fmt.Sprintf("read version: %s is not as new as written version: %s for group resource %s", c.ReadRV, c.WroteRV, c.GroupResource.String())
}

var _ consistencyStore = &realConsistencyStore{}

type lastSyncRVGetter interface {
	LastStoreSyncResourceVersion() string
}

type realConsistencyStore struct {
	// writesLock guards reads/additions/deletions to the writes map.
	// individual records are responsible for managing their own thread safety.
	writesLock sync.RWMutex
	// writes is a map of owner -> ownerRecord
	writes map[types.NamespacedName]*ownerRecord

	stores map[schema.GroupResource]lastSyncRVGetter
}

func newConsistencyStore(stores map[schema.GroupResource]lastSyncRVGetter) *realConsistencyStore {
	return &realConsistencyStore{
		writes: map[types.NamespacedName]*ownerRecord{},
		stores: stores,
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
func (c *realConsistencyStore) WroteAt(owner types.NamespacedName, ownerUID types.UID, resource schema.GroupResource, rv string) {
	c.ensureWrittenRecord(owner, ownerUID).WroteAt(resource, rv)
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
func (c *realConsistencyStore) EnsureReady(owner types.NamespacedName) []consistencyError {
	record := c.getWrittenRecord(owner)
	if record == nil {
		return nil
	}
	err := record.EnsureReady(c)
	if err == nil {
		c.Clear(owner, record.ownerUID)
		return nil
	}
	return err
}

type ownerRecord struct {
	// ownerUID must not be mutated after creation
	ownerUID types.UID

	versionsLock sync.Mutex
	versions     map[schema.GroupResource]string
}

func newOwnerRecord(ownerUID types.UID) *ownerRecord {
	return &ownerRecord{ownerUID: ownerUID, versions: map[schema.GroupResource]string{}}
}

// WroteAt increments the written resource version of an ownerRecord if it is
// the newest seen resource version for that resource.
func (w *ownerRecord) WroteAt(resource schema.GroupResource, rv string) {
	w.versionsLock.Lock()
	defer w.versionsLock.Unlock()
	if _, ok := w.versions[resource]; !ok {
		w.versions[resource] = rv
		return
	}
	cmp, err := resourceversion.CompareResourceVersion(w.versions[resource], rv)
	if err == nil && cmp >= 0 {
		return
	}
	w.versions[resource] = rv
}

// EnsureReady checks whether or not the ownerRecord is ready compared to the
// read resource versions in the consistency store.
func (w *ownerRecord) EnsureReady(c *realConsistencyStore) []consistencyError {
	w.versionsLock.Lock()
	defer w.versionsLock.Unlock()
	var errs []consistencyError
	for gr, wroteRV := range w.versions {
		store, exists := c.stores[gr]
		if !exists || store == nil {
			continue
		}
		readRV := store.LastStoreSyncResourceVersion()
		if readRV == "" {
			// Since we wait for the store to be ready, the only time "" is if the
			// LastStoreSyncResourceVersion() feature is not enabled.
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
				WroteRV:       wroteRV,
				ReadRV:        readRV,
				GroupResource: gr,
			})
		}
	}
	return errs
}
