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

import "sync"

// keyedMutex is a mutex locking on the key provided to the Lock function.
// Only one caller can hold the lock for a specific key at a time.
type keyedMutex struct {
	locksMtx sync.Mutex
	locks    map[interface{}]*keyLock
}

// newKeyedMutex creates a new keyed mutex ready for use.
func newKeyedMutex() *keyedMutex {
	return &keyedMutex{
		locks: make(map[interface{}]*keyLock),
	}
}

// keyLock is the lock for a single specific key.
type keyLock struct {
	sync.Mutex
	// users is the number of callers attempting to acquire the mutex, including the one currently holding it.
	users uint
}

// unlock unlocks a currently locked key.
type unlock func()

// Lock locks the passed in key, blocking if the key is locked.
// Returns the unlock function to release the lock on the key.
func (k *keyedMutex) Lock(key interface{}) unlock {
	// Get an existing keyLock for the key or create a new one and increase the number of users.
	l := func() *keyLock {
		k.locksMtx.Lock()
		defer k.locksMtx.Unlock()

		l, ok := k.locks[key]
		if !ok {
			l = &keyLock{}
			k.locks[key] = l
		}
		l.users++
		return l
	}()

	l.Lock()

	// Unlocks the keyLock for the key, decreases the counter and removes the keyLock from the map if there are no more users left.
	return func() {
		k.locksMtx.Lock()
		defer k.locksMtx.Unlock()

		l.Unlock()
		l.users--
		if l.users == 0 {
			delete(k.locks, key)
		}
	}
}
