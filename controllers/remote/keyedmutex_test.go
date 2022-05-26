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
	"testing"

	. "github.com/onsi/gomega"
)

func TestKeyedMutex(t *testing.T) {
	t.Run("blocks on a locked key until unlocked", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		routineStarted := make(chan bool)
		routineCompleted := make(chan bool)
		key := "key1"

		km := newKeyedMutex()
		unlock := km.Lock(key)

		// start a routine which tries to lock the same key
		go func() {
			routineStarted <- true
			unlock := km.Lock(key)
			unlock()
			routineCompleted <- true
		}()

		<-routineStarted
		g.Consistently(routineCompleted).ShouldNot(Receive())

		// routine should be able to acquire the lock for the key after we unlock
		unlock()
		g.Eventually(routineCompleted).Should(Receive())

		// ensure that the lock was cleaned up from the internal map
		g.Expect(km.locks).To(HaveLen(0))
	})

	t.Run("can lock different keys without blocking", func(t *testing.T) {
		g := NewWithT(t)
		km := newKeyedMutex()
		keys := []string{"a", "b", "c", "d"}
		unlocks := make([]unlock, 0, len(keys))

		// lock all keys
		for _, key := range keys {
			unlocks = append(unlocks, km.Lock(key))
		}

		// unlock all keys
		for _, unlock := range unlocks {
			unlock()
		}

		// ensure that the lock was cleaned up from the internal map
		g.Expect(km.locks).To(HaveLen(0))
	})
}
