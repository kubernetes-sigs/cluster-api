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

package machinedeployment

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	testclient "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/fake"
	v1alpha1listers "sigs.k8s.io/cluster-api/pkg/client/listers_generated/cluster/v1alpha1"
	dutil "sigs.k8s.io/cluster-api/pkg/controller/machinedeployment/util"
)

func intOrStrP(val int) *intstr.IntOrString {
	intOrStr := intstr.FromInt(val)
	return &intOrStr
}

func TestScale(t *testing.T) {
	newTimestamp := metav1.Date(2016, 5, 20, 2, 0, 0, 0, time.UTC)
	oldTimestamp := metav1.Date(2016, 5, 20, 1, 0, 0, 0, time.UTC)
	olderTimestamp := metav1.Date(2016, 5, 20, 0, 0, 0, 0, time.UTC)

	var updatedTemplate = func(replicas int) *v1alpha1.MachineDeployment {
		d := newMachineDeployment("foo", replicas, nil, nil, nil, map[string]string{"foo": "bar"})
		d.Spec.Template.Labels["another"] = "label"
		return d
	}

	tests := []struct {
		name          string
		deployment    *v1alpha1.MachineDeployment
		oldDeployment *v1alpha1.MachineDeployment

		newMS  *v1alpha1.MachineSet
		oldMSs []*v1alpha1.MachineSet

		expectedNew  *v1alpha1.MachineSet
		expectedOld  []*v1alpha1.MachineSet
		wasntUpdated map[string]bool

		desiredReplicasAnnotations map[string]int32
	}{
		{
			name:          "normal scaling event: 10 -> 12",
			deployment:    newMachineDeployment("foo", 12, nil, nil, nil, nil),
			oldDeployment: newMachineDeployment("foo", 10, nil, nil, nil, nil),

			newMS:  ms("foo-v1", 10, nil, newTimestamp),
			oldMSs: []*v1alpha1.MachineSet{},

			expectedNew: ms("foo-v1", 12, nil, newTimestamp),
			expectedOld: []*v1alpha1.MachineSet{},
		},
		{
			name:          "normal scaling event: 10 -> 5",
			deployment:    newMachineDeployment("foo", 5, nil, nil, nil, nil),
			oldDeployment: newMachineDeployment("foo", 10, nil, nil, nil, nil),

			newMS:  ms("foo-v1", 10, nil, newTimestamp),
			oldMSs: []*v1alpha1.MachineSet{},

			expectedNew: ms("foo-v1", 5, nil, newTimestamp),
			expectedOld: []*v1alpha1.MachineSet{},
		},
		{
			name:          "proportional scaling: 5 -> 10",
			deployment:    newMachineDeployment("foo", 10, nil, nil, nil, nil),
			oldDeployment: newMachineDeployment("foo", 5, nil, nil, nil, nil),

			newMS:  ms("foo-v2", 2, nil, newTimestamp),
			oldMSs: []*v1alpha1.MachineSet{ms("foo-v1", 3, nil, oldTimestamp)},

			expectedNew: ms("foo-v2", 4, nil, newTimestamp),
			expectedOld: []*v1alpha1.MachineSet{ms("foo-v1", 6, nil, oldTimestamp)},
		},
		{
			name:          "proportional scaling: 5 -> 3",
			deployment:    newMachineDeployment("foo", 3, nil, nil, nil, nil),
			oldDeployment: newMachineDeployment("foo", 5, nil, nil, nil, nil),

			newMS:  ms("foo-v2", 2, nil, newTimestamp),
			oldMSs: []*v1alpha1.MachineSet{ms("foo-v1", 3, nil, oldTimestamp)},

			expectedNew: ms("foo-v2", 1, nil, newTimestamp),
			expectedOld: []*v1alpha1.MachineSet{ms("foo-v1", 2, nil, oldTimestamp)},
		},
		{
			name:          "proportional scaling: 9 -> 4",
			deployment:    newMachineDeployment("foo", 4, nil, nil, nil, nil),
			oldDeployment: newMachineDeployment("foo", 9, nil, nil, nil, nil),

			newMS:  ms("foo-v2", 8, nil, newTimestamp),
			oldMSs: []*v1alpha1.MachineSet{ms("foo-v1", 1, nil, oldTimestamp)},

			expectedNew: ms("foo-v2", 4, nil, newTimestamp),
			expectedOld: []*v1alpha1.MachineSet{ms("foo-v1", 0, nil, oldTimestamp)},
		},
		{
			name:          "proportional scaling: 7 -> 10",
			deployment:    newMachineDeployment("foo", 10, nil, nil, nil, nil),
			oldDeployment: newMachineDeployment("foo", 7, nil, nil, nil, nil),

			newMS:  ms("foo-v3", 2, nil, newTimestamp),
			oldMSs: []*v1alpha1.MachineSet{ms("foo-v2", 3, nil, oldTimestamp), ms("foo-v1", 2, nil, olderTimestamp)},

			expectedNew: ms("foo-v3", 3, nil, newTimestamp),
			expectedOld: []*v1alpha1.MachineSet{ms("foo-v2", 4, nil, oldTimestamp), ms("foo-v1", 3, nil, olderTimestamp)},
		},
		{
			name:          "proportional scaling: 13 -> 8",
			deployment:    newMachineDeployment("foo", 8, nil, nil, nil, nil),
			oldDeployment: newMachineDeployment("foo", 13, nil, nil, nil, nil),

			newMS:  ms("foo-v3", 2, nil, newTimestamp),
			oldMSs: []*v1alpha1.MachineSet{ms("foo-v2", 8, nil, oldTimestamp), ms("foo-v1", 3, nil, olderTimestamp)},

			expectedNew: ms("foo-v3", 1, nil, newTimestamp),
			expectedOld: []*v1alpha1.MachineSet{ms("foo-v2", 5, nil, oldTimestamp), ms("foo-v1", 2, nil, olderTimestamp)},
		},
		// Scales up the new machine set.
		{
			name:          "leftover distribution: 3 -> 4",
			deployment:    newMachineDeployment("foo", 4, nil, nil, nil, nil),
			oldDeployment: newMachineDeployment("foo", 3, nil, nil, nil, nil),

			newMS:  ms("foo-v3", 1, nil, newTimestamp),
			oldMSs: []*v1alpha1.MachineSet{ms("foo-v2", 1, nil, oldTimestamp), ms("foo-v1", 1, nil, olderTimestamp)},

			expectedNew: ms("foo-v3", 2, nil, newTimestamp),
			expectedOld: []*v1alpha1.MachineSet{ms("foo-v2", 1, nil, oldTimestamp), ms("foo-v1", 1, nil, olderTimestamp)},
		},
		// Scales down the older machine set.
		{
			name:          "leftover distribution: 3 -> 2",
			deployment:    newMachineDeployment("foo", 2, nil, nil, nil, nil),
			oldDeployment: newMachineDeployment("foo", 3, nil, nil, nil, nil),

			newMS:  ms("foo-v3", 1, nil, newTimestamp),
			oldMSs: []*v1alpha1.MachineSet{ms("foo-v2", 1, nil, oldTimestamp), ms("foo-v1", 1, nil, olderTimestamp)},

			expectedNew: ms("foo-v3", 1, nil, newTimestamp),
			expectedOld: []*v1alpha1.MachineSet{ms("foo-v2", 1, nil, oldTimestamp), ms("foo-v1", 0, nil, olderTimestamp)},
		},
		// Scales up the latest machine set first.
		{
			name:          "proportional scaling (no new rs): 4 -> 5",
			deployment:    newMachineDeployment("foo", 5, nil, nil, nil, nil),
			oldDeployment: newMachineDeployment("foo", 4, nil, nil, nil, nil),

			newMS:  nil,
			oldMSs: []*v1alpha1.MachineSet{ms("foo-v2", 2, nil, oldTimestamp), ms("foo-v1", 2, nil, olderTimestamp)},

			expectedNew: nil,
			expectedOld: []*v1alpha1.MachineSet{ms("foo-v2", 3, nil, oldTimestamp), ms("foo-v1", 2, nil, olderTimestamp)},
		},
		// Scales down to zero
		{
			name:          "proportional scaling: 6 -> 0",
			deployment:    newMachineDeployment("foo", 0, nil, nil, nil, nil),
			oldDeployment: newMachineDeployment("foo", 6, nil, nil, nil, nil),

			newMS:  ms("foo-v3", 3, nil, newTimestamp),
			oldMSs: []*v1alpha1.MachineSet{ms("foo-v2", 2, nil, oldTimestamp), ms("foo-v1", 1, nil, olderTimestamp)},

			expectedNew: ms("foo-v3", 0, nil, newTimestamp),
			expectedOld: []*v1alpha1.MachineSet{ms("foo-v2", 0, nil, oldTimestamp), ms("foo-v1", 0, nil, olderTimestamp)},
		},
		// Scales up from zero
		{
			name:          "proportional scaling: 0 -> 6",
			deployment:    newMachineDeployment("foo", 6, nil, nil, nil, nil),
			oldDeployment: newMachineDeployment("foo", 6, nil, nil, nil, nil),

			newMS:  ms("foo-v3", 0, nil, newTimestamp),
			oldMSs: []*v1alpha1.MachineSet{ms("foo-v2", 0, nil, oldTimestamp), ms("foo-v1", 0, nil, olderTimestamp)},

			expectedNew:  ms("foo-v3", 6, nil, newTimestamp),
			expectedOld:  []*v1alpha1.MachineSet{ms("foo-v2", 0, nil, oldTimestamp), ms("foo-v1", 0, nil, olderTimestamp)},
			wasntUpdated: map[string]bool{"foo-v2": true, "foo-v1": true},
		},
		// Scenario: deployment.spec.replicas == 3 ( foo-v1.spec.replicas == foo-v2.spec.replicas == foo-v3.spec.replicas == 1 )
		// Deployment is scaled to 5. foo-v3.spec.replicas and foo-v2.spec.replicas should increment by 1 but foo-v2 fails to
		// update.
		{
			name:          "failed ms update",
			deployment:    newMachineDeployment("foo", 5, nil, nil, nil, nil),
			oldDeployment: newMachineDeployment("foo", 5, nil, nil, nil, nil),

			newMS:  ms("foo-v3", 2, nil, newTimestamp),
			oldMSs: []*v1alpha1.MachineSet{ms("foo-v2", 1, nil, oldTimestamp), ms("foo-v1", 1, nil, olderTimestamp)},

			expectedNew:  ms("foo-v3", 2, nil, newTimestamp),
			expectedOld:  []*v1alpha1.MachineSet{ms("foo-v2", 2, nil, oldTimestamp), ms("foo-v1", 1, nil, olderTimestamp)},
			wasntUpdated: map[string]bool{"foo-v3": true, "foo-v1": true},

			desiredReplicasAnnotations: map[string]int32{"foo-v2": int32(3)},
		},
		{
			name:          "deployment with surge machines",
			deployment:    newMachineDeployment("foo", 20, nil, intOrStrP(2), nil, nil),
			oldDeployment: newMachineDeployment("foo", 10, nil, intOrStrP(2), nil, nil),

			newMS:  ms("foo-v2", 6, nil, newTimestamp),
			oldMSs: []*v1alpha1.MachineSet{ms("foo-v1", 6, nil, oldTimestamp)},

			expectedNew: ms("foo-v2", 11, nil, newTimestamp),
			expectedOld: []*v1alpha1.MachineSet{ms("foo-v1", 11, nil, oldTimestamp)},
		},
		{
			name:          "change both surge and size",
			deployment:    newMachineDeployment("foo", 50, nil, intOrStrP(6), nil, nil),
			oldDeployment: newMachineDeployment("foo", 10, nil, intOrStrP(3), nil, nil),

			newMS:  ms("foo-v2", 5, nil, newTimestamp),
			oldMSs: []*v1alpha1.MachineSet{ms("foo-v1", 8, nil, oldTimestamp)},

			expectedNew: ms("foo-v2", 22, nil, newTimestamp),
			expectedOld: []*v1alpha1.MachineSet{ms("foo-v1", 34, nil, oldTimestamp)},
		},
		{
			name:          "change both size and template",
			deployment:    updatedTemplate(14),
			oldDeployment: newMachineDeployment("foo", 10, nil, nil, nil, map[string]string{"foo": "bar"}),

			newMS:  nil,
			oldMSs: []*v1alpha1.MachineSet{ms("foo-v2", 7, nil, newTimestamp), ms("foo-v1", 3, nil, oldTimestamp)},

			expectedNew: nil,
			expectedOld: []*v1alpha1.MachineSet{ms("foo-v2", 10, nil, newTimestamp), ms("foo-v1", 4, nil, oldTimestamp)},
		},
		{
			name:          "saturated but broken new machine set does not affect old machines",
			deployment:    newMachineDeployment("foo", 2, nil, intOrStrP(1), intOrStrP(1), nil),
			oldDeployment: newMachineDeployment("foo", 2, nil, intOrStrP(1), intOrStrP(1), nil),

			newMS: func() *v1alpha1.MachineSet {
				ms := ms("foo-v2", 2, nil, newTimestamp)
				ms.Status.AvailableReplicas = 0
				return ms
			}(),
			oldMSs: []*v1alpha1.MachineSet{ms("foo-v1", 1, nil, oldTimestamp)},

			expectedNew: ms("foo-v2", 2, nil, newTimestamp),
			expectedOld: []*v1alpha1.MachineSet{ms("foo-v1", 1, nil, oldTimestamp)},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_ = olderTimestamp
			t.Log(test.name)
			fakeClient := fake.Clientset{}
			controller := &MachineDeploymentControllerImpl{}
			controller.machineClient = &fakeClient

			if test.newMS != nil {
				desiredReplicas := *(test.oldDeployment.Spec.Replicas)
				if desired, ok := test.desiredReplicasAnnotations[test.newMS.Name]; ok {
					desiredReplicas = desired
				}
				dutil.SetReplicasAnnotations(test.newMS, desiredReplicas, desiredReplicas+dutil.MaxSurge(*test.oldDeployment))
			}
			for i := range test.oldMSs {
				ms := test.oldMSs[i]
				if ms == nil {
					continue
				}
				desiredReplicas := *(test.oldDeployment.Spec.Replicas)
				if desired, ok := test.desiredReplicasAnnotations[ms.Name]; ok {
					desiredReplicas = desired
				}
				dutil.SetReplicasAnnotations(ms, desiredReplicas, desiredReplicas+dutil.MaxSurge(*test.oldDeployment))
			}

			if err := controller.scale(test.deployment, test.newMS, test.oldMSs); err != nil {
				t.Errorf("%s: unexpected error: %v", test.name, err)
				return
			}

			// Construct the nameToSize map that will hold all the sizes we got our of tests
			// Skip updating the map if the machine set wasn't updated since there will be
			// no update action for it.
			nameToSize := make(map[string]int32)
			if test.newMS != nil {
				nameToSize[test.newMS.Name] = *(test.newMS.Spec.Replicas)
			}
			for i := range test.oldMSs {
				ms := test.oldMSs[i]
				nameToSize[ms.Name] = *(ms.Spec.Replicas)
			}
			// Get all the UPDATE actions and update nameToSize with all the updated sizes.
			for _, action := range fakeClient.Actions() {
				ms := action.(testclient.UpdateAction).GetObject().(*v1alpha1.MachineSet)
				if !test.wasntUpdated[ms.Name] {
					nameToSize[ms.Name] = *(ms.Spec.Replicas)
				}
			}

			if test.expectedNew != nil && test.newMS != nil && *(test.expectedNew.Spec.Replicas) != nameToSize[test.newMS.Name] {
				t.Errorf("%s: expected new replicas: %d, got: %d", test.name, *(test.expectedNew.Spec.Replicas), nameToSize[test.newMS.Name])
				return
			}
			if len(test.expectedOld) != len(test.oldMSs) {
				t.Errorf("%s: expected %d old machine sets, got %d", test.name, len(test.expectedOld), len(test.oldMSs))
				return
			}
			for n := range test.oldMSs {
				ms := test.oldMSs[n]
				expected := test.expectedOld[n]
				if *(expected.Spec.Replicas) != nameToSize[ms.Name] {
					t.Errorf("%s: expected old (%s) replicas: %d, got: %d", test.name, ms.Name, *(expected.Spec.Replicas), nameToSize[ms.Name])
				}
			}
		})
	}
}

func TestDeploymentController_cleanupDeployment(t *testing.T) {
	selector := map[string]string{"foo": "bar"}
	alreadyDeleted := newMSWithStatus("foo-1", 0, 0, selector)
	now := metav1.Now()
	alreadyDeleted.DeletionTimestamp = &now

	tests := []struct {
		name                 string
		oldMSs               []*v1alpha1.MachineSet
		revisionHistoryLimit int32
		expectedDeletions    int
	}{
		{
			name: "3 machine set qualifies for deletion, limit to keep 1, delete 2.",
			oldMSs: []*v1alpha1.MachineSet{
				newMSWithStatus("foo-1", 0, 0, selector),
				newMSWithStatus("foo-2", 0, 0, selector),
				newMSWithStatus("foo-3", 0, 0, selector),
			},
			revisionHistoryLimit: 1,
			expectedDeletions:    2,
		},
		{
			// Only delete the machine set with Spec.Replicas = Status.Replicas = 0.
			name: "1 machine set qualifies for deletion, limit 0, delete 1.",
			oldMSs: []*v1alpha1.MachineSet{
				newMSWithStatus("foo-1", 0, 0, selector),
				newMSWithStatus("foo-2", 0, 1, selector),
				newMSWithStatus("foo-3", 1, 0, selector),
				newMSWithStatus("foo-4", 1, 1, selector),
			},
			revisionHistoryLimit: 0,
			expectedDeletions:    1,
		},
		{
			name: "2 machine set qualfiies for deletion, limit 0, delete 2.",
			oldMSs: []*v1alpha1.MachineSet{
				newMSWithStatus("foo-1", 0, 0, selector),
				newMSWithStatus("foo-2", 0, 0, selector),
			},
			revisionHistoryLimit: 0,
			expectedDeletions:    2,
		},
		{
			name: "0 machine set qualifies for deletion, limit 0, delete 0.",
			oldMSs: []*v1alpha1.MachineSet{
				newMSWithStatus("foo-1", 1, 1, selector),
				newMSWithStatus("foo-2", 1, 1, selector),
			},
			revisionHistoryLimit: 0,
			expectedDeletions:    0,
		},
		{
			name: "1 machine set qualifies for deletion, already deleting, limit 0, delete 0.",
			oldMSs: []*v1alpha1.MachineSet{
				alreadyDeleted,
			},
			revisionHistoryLimit: 0,
			expectedDeletions:    0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Log(test.name)

			rObjects := []runtime.Object{}
			machineSetIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
			for _, ms := range test.oldMSs {
				err := machineSetIndexer.Add(ms)
				if err != nil {
					t.Fatal(err)
				}
				rObjects = append(rObjects, ms)
			}
			machineSetLister := v1alpha1listers.NewMachineSetLister(machineSetIndexer)

			fakeClient := fake.NewSimpleClientset(rObjects...)
			controller := &MachineDeploymentControllerImpl{}
			controller.machineClient = fakeClient
			controller.msLister = machineSetLister

			d := newMachineDeployment("foo", 1, &test.revisionHistoryLimit, nil, nil, map[string]string{"foo": "bar"})
			controller.cleanupDeployment(test.oldMSs, d)

			gotDeletions := 0
			for _, action := range fakeClient.Actions() {
				if "delete" == action.GetVerb() {
					gotDeletions++
				}
			}
			if gotDeletions != test.expectedDeletions {
				t.Errorf("expect %v old machine sets been deleted, but got %v", test.expectedDeletions, gotDeletions)
			}
		})
	}
}
