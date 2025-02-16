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
	"testing"
	"time"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

func TestCache(t *testing.T) {
	g := NewWithT(t)

	c := New[ReconcileEntry]()

	machine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-machine",
		},
	}
	entry := NewReconcileEntry(machine, time.Now())

	_, ok := c.Has(entry.Key())
	g.Expect(ok).To(BeFalse())

	c.Add(entry)

	entryFromCache, ok := c.Has(entry.Key())
	g.Expect(ok).To(BeTrue())
	g.Expect(entryFromCache).To(Equal(entry))
}

func TestShouldRequeueDrain(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name             string
		now              time.Time
		reconcileAfter   time.Time
		wantRequeue      bool
		wantRequeueAfter time.Duration
	}{
		{
			name:             "Don't requeue, reconcileAfter is zero",
			now:              now,
			reconcileAfter:   time.Time{},
			wantRequeue:      false,
			wantRequeueAfter: time.Duration(0),
		},
		{
			name:             "Requeue after 15s",
			now:              now,
			reconcileAfter:   now.Add(time.Duration(15) * time.Second),
			wantRequeue:      true,
			wantRequeueAfter: time.Duration(15) * time.Second,
		},
		{
			name:             "Don't requeue, reconcileAfter is now",
			now:              now,
			reconcileAfter:   now,
			wantRequeue:      false,
			wantRequeueAfter: time.Duration(0),
		},
		{
			name:             "Don't requeue, reconcileAfter is before now",
			now:              now,
			reconcileAfter:   now.Add(-time.Duration(60) * time.Second),
			wantRequeue:      false,
			wantRequeueAfter: time.Duration(0),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			gotRequeueAfter, gotRequeue := ReconcileEntry{ReconcileAfter: tt.reconcileAfter}.ShouldRequeue(tt.now)
			g.Expect(gotRequeue).To(Equal(tt.wantRequeue))
			g.Expect(gotRequeueAfter).To(Equal(tt.wantRequeueAfter))
		})
	}
}
