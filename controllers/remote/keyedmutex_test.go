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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestKeyedMutex(t *testing.T) {
	t.Run("Lock a Cluster and ensures the second Lock on the same Cluster returns false", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		cluster1 := client.ObjectKey{Namespace: metav1.NamespaceDefault, Name: "Cluster1"}
		km := newKeyedMutex()

		// Try to lock cluster1.
		// Should work as nobody currently holds the lock for cluster1.
		g.Expect(km.TryLock(cluster1)).To(BeTrue())

		// Try to lock cluster1 again.
		// Shouldn't work as cluster1 is already locked.
		g.Expect(km.TryLock(cluster1)).To(BeFalse())

		// Unlock cluster1.
		km.Unlock(cluster1)

		// Ensure that the lock was cleaned up from the internal map.
		g.Expect(km.locks).To(BeEmpty())
	})

	t.Run("Can lock different Clusters in parallel but each one only once", func(t *testing.T) {
		g := NewWithT(t)
		km := newKeyedMutex()
		clusters := []client.ObjectKey{
			{Namespace: metav1.NamespaceDefault, Name: "Cluster1"},
			{Namespace: metav1.NamespaceDefault, Name: "Cluster2"},
			{Namespace: metav1.NamespaceDefault, Name: "Cluster3"},
			{Namespace: metav1.NamespaceDefault, Name: "Cluster4"},
		}

		// Run this twice to ensure Clusters can be locked again
		// after they have been unlocked.
		for range 2 {
			// Lock all Clusters (should work).
			for _, key := range clusters {
				g.Expect(km.TryLock(key)).To(BeTrue())
			}

			// Ensure Clusters can't be locked again.
			for _, key := range clusters {
				g.Expect(km.TryLock(key)).To(BeFalse())
			}

			// Unlock all Clusters.
			for _, key := range clusters {
				km.Unlock(key)
			}
		}

		// Ensure that the lock was cleaned up from the internal map.
		g.Expect(km.locks).To(BeEmpty())
	})
}
