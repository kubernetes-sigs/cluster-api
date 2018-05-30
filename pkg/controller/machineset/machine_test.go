/*
Copyright 2018 The Kubernetes Authors.

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

package machineset

import (
	"reflect"
	"sort"
	"testing"

	"github.com/kubernetes-incubator/apiserver-builder/pkg/controller"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/fake"
	v1alpha1listers "sigs.k8s.io/cluster-api/pkg/client/listers_generated/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/controller/sharedinformers"
)

func TestMachineSetController_reconcileMachine(t *testing.T) {
	tests := []struct {
		name                 string
		machineNotPresent    bool
		machineSetNotPresent bool
		machineSetDiffUID    bool
		machineNoCtrlRef     bool
		machineNoLabels      bool
		sameMSLabels         bool
		expectQueued         int
	}{
		{
			name:              "machine doesn't exist, noop",
			machineNotPresent: true,
		},
		{
			name:         "machine with controller ref, found machine set, 1 queued.",
			expectQueued: 1,
		},
		{
			name:                 "machine with controller ref, machine set not found, noop",
			machineSetNotPresent: true,
		},
		{
			name:              "machine with controller ref, machine set UID mismatch, noop",
			machineSetDiffUID: true,
		},
		{
			name:             "machine without controller ref, found 1 machine set with matching labels, 1 queued",
			machineNoCtrlRef: true,
			expectQueued:     1,
		},
		{
			name:             "machine without controller ref, no labels, noop",
			machineNoCtrlRef: true,
			machineNoLabels:  true,
		},
		{
			name:             "machine without controller ref, found 2 machine set with matching labels, 2 queued",
			machineNoCtrlRef: true,
			sameMSLabels:     true,
			expectQueued:     2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ms := createMachineSet(1, "foo", "bar1", "acme")
			msSameLabel := createMachineSet(1, "ms2", "bar1", "acme")
			msDiffLabel := createMachineSet(1, "ms3", "ms-bar", "acme")
			msDiffLabel.Spec.Selector.MatchLabels = map[string]string{labelKey: "DIFFLABELS"}
			msDiffLabel.Spec.Template.Labels = map[string]string{labelKey: "DIFFLABELS"}
			msDiffUID := ms.DeepCopy()
			msDiffUID.UID = "NotMe"

			m := machineFromMachineSet(ms, "bar1")

			rObjects := []runtime.Object{}
			if !test.machineNotPresent {
				rObjects = append(rObjects, m)
			}
			if test.machineNoCtrlRef {
				m.ObjectMeta.OwnerReferences = []metav1.OwnerReference{}
			}
			if test.machineNoLabels {
				m.ObjectMeta.Labels = map[string]string{}
			}

			fakeClient := fake.NewSimpleClientset(rObjects...)

			machineSetIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
			machineSetLister := v1alpha1listers.NewMachineSetLister(machineSetIndexer)

			if !test.machineSetNotPresent {
				if test.machineSetDiffUID {
					machineSetIndexer.Add(msDiffUID)
				} else {
					machineSetIndexer.Add(ms)
				}
			}
			machineSetIndexer.Add(msDiffLabel)
			if test.sameMSLabels {
				machineSetIndexer.Add(msSameLabel)
			}

			target := &MachineSetControllerImpl{}
			target.clusterAPIClient = fakeClient
			target.machineSetLister = machineSetLister
			target.informers = &sharedinformers.SharedInformers{}
			target.informers.WorkerQueues = map[string]*controller.QueueWorker{}
			target_queue := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "MachineSet")
			target.informers.WorkerQueues["MachineSet"] = &controller.QueueWorker{target_queue, 10, "MachineSet", nil}

			mKey, err := cache.MetaNamespaceKeyFunc(m)
			if err != nil {
				t.Fatalf("unable to get key for test machine, %v", err)
			}
			err = target.reconcileMachine(mKey)

			if err != nil {
				t.Fatalf("got %v error, expected %v error; %v", err != nil, false, err)
			}

			if target_queue.Len() != test.expectQueued {
				t.Fatalf("got %v queued items, expected %v queued items", target_queue.Len(), test.expectQueued)
			}

			if test.expectQueued == 1 {
				verifyQueuedKey(t, target_queue, []*v1alpha1.MachineSet{ms})
			}
			if test.expectQueued == 2 {
				verifyQueuedKey(t, target_queue, []*v1alpha1.MachineSet{ms, msSameLabel})
			}
		})
	}
}

func verifyQueuedKey(t *testing.T, queue workqueue.RateLimitingInterface, mss []*v1alpha1.MachineSet) {
	queuedLength := queue.Len()
	var queuedKeys, expectedKeys []string
	for i := 0; i < queuedLength; i++ {
		key, done := queue.Get()
		if key == nil || done {
			t.Fatalf("failed to enqueue controller.")
		}
		queuedKeys = append(queuedKeys, key.(string))
		queue.Done(key)
	}
	for _, ms := range mss {
		key, err := cache.MetaNamespaceKeyFunc(ms)
		if err != nil {
			t.Fatalf("failed to get key for deployment.")
		}
		expectedKeys = append(expectedKeys, key)
	}
	sort.Strings(queuedKeys)
	sort.Strings(expectedKeys)
	if !reflect.DeepEqual(queuedKeys, expectedKeys) {
		t.Fatalf("got %v keys, expected %v keys", queuedKeys, expectedKeys)
	}
}
