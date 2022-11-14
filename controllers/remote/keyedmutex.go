/*
Copyright 2021 The Kubernetes Authors.

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

package remote

import (
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// keyedMutex is a mutex locking on the key provided to the Lock function.
// Only one caller can hold the lock for a specific key at a time.
// A second Lock call if the lock is already held for a key returns false.
type keyedMutex struct {
	locksMtx sync.Mutex
	locks    map[client.ObjectKey]struct{}
}

// newKeyedMutex creates a new keyed mutex ready for use.
func newKeyedMutex() *keyedMutex {
	return &keyedMutex{
		locks: make(map[client.ObjectKey]struct{}),
	}
}

// TryLock locks the passed in key if it's not already locked.
// A second Lock call if the lock is already held for a key returns false.
// In the ClusterCacheTracker case the key is the ObjectKey for a cluster.
func (k *keyedMutex) TryLock(key client.ObjectKey) bool {
	k.locksMtx.Lock()
	defer k.locksMtx.Unlock()

	// Check if there is already a lock for this key (e.g. Cluster).
	if _, ok := k.locks[key]; ok {
		// There is already a lock, return false.
		return false
	}

	// Lock doesn't exist yet, create the lock.
	k.locks[key] = struct{}{}

	return true
}

// Unlock unlocks the key.
func (k *keyedMutex) Unlock(key client.ObjectKey) {
	k.locksMtx.Lock()
	defer k.locksMtx.Unlock()

	// Remove the lock if it exists.
	delete(k.locks, key)
}
