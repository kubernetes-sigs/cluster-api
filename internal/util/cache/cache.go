/*
Copyright 2024 The Kubernetes Authors.

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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kcache "k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	// ttl is the duration for which we keep entries in the cache.
	ttl = 10 * time.Minute

	// expirationInterval is the interval in which we will remove expired entries
	// from the cache.
	expirationInterval = 10 * time.Hour
)

// Entry is the interface for the type of Cache entries.
type Entry interface {
	// Key returns the cache key of an Entry.
	Key() string
}

// Cache caches objects of type Entry.
type Cache[E Entry] interface {
	// Add adds the given entry to the Cache.
	// Note: entries expire after the ttl.
	Add(entry E)

	// Has checks if the given key (still) exists in the Cache.
	// Note: entries expire after the ttl.
	Has(key string) (E, bool)
}

// New creates a new cache.
func New[E Entry]() Cache[E] {
	r := &cache[E]{
		Store: kcache.NewTTLStore(func(obj interface{}) (string, error) {
			// We only add objects of type E to the cache, so it's safe to cast to E.
			return obj.(E).Key(), nil
		}, ttl),
	}
	go func() {
		for {
			// Call list to clear the cache of expired items.
			// We have to do this periodically as the cache itself only expires
			// items lazily. If we don't do this the cache grows indefinitely.
			r.List()

			time.Sleep(expirationInterval)
		}
	}()
	return r
}

type cache[E Entry] struct {
	kcache.Store
}

// Add adds the given entry to the Cache.
// Note: entries expire after the ttl.
func (r *cache[E]) Add(entry E) {
	// Note: We can ignore the error here because the key func we pass into NewTTLStore never
	// returns errors.
	_ = r.Store.Add(entry)
}

// Has checks if the given key (still) exists in the Cache.
// Note: entries expire after the ttl.
func (r *cache[E]) Has(key string) (E, bool) {
	// Note: We can ignore the error here because GetByKey never returns an error.
	item, exists, _ := r.Store.GetByKey(key)
	if exists {
		return item.(E), true
	}
	return *new(E), false
}

// ReconcileEntry is an Entry for the Cache that stores the
// earliest time after which the next Reconcile should be executed.
type ReconcileEntry struct {
	Request        ctrl.Request
	ReconcileAfter time.Time
}

// NewReconcileEntry creates a new ReconcileEntry based on an object and a reconcileAfter time.
func NewReconcileEntry(obj metav1.Object, reconcileAfter time.Time) ReconcileEntry {
	return ReconcileEntry{
		Request: ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: obj.GetNamespace(),
				Name:      obj.GetName(),
			},
		},
		ReconcileAfter: reconcileAfter,
	}
}

// NewReconcileEntryKey returns the key of a ReconcileEntry based on an object.
func NewReconcileEntryKey(obj metav1.Object) string {
	return ReconcileEntry{
		Request: ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: obj.GetNamespace(),
				Name:      obj.GetName(),
			},
		},
	}.Key()
}

var _ Entry = &ReconcileEntry{}

// Key returns the cache key of a ReconcileEntry.
func (r ReconcileEntry) Key() string {
	return r.Request.String()
}

// ShouldRequeue returns if the current Reconcile should be requeued.
func (r ReconcileEntry) ShouldRequeue(now time.Time) (requeueAfter time.Duration, requeue bool) {
	if r.ReconcileAfter.IsZero() {
		return time.Duration(0), false
	}

	if r.ReconcileAfter.After(now) {
		return r.ReconcileAfter.Sub(now), true
	}

	return time.Duration(0), false
}
