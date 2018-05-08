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

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	core "k8s.io/client-go/testing"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/fake"
)

func TestMachineDeploymentController_reconcileNewMachineSet(t *testing.T) {
	// expectedNewReplicas = deploymentReplicas + maxSurge - oldReplicas - newReplicas
	tests := []struct {
		name                string
		deploymentReplicas  int
		maxSurge            intstr.IntOrString
		oldReplicas         int
		newReplicas         int
		scaleExpected       bool
		expectedNewReplicas int
	}{
		{
			name:                "scenario 1. new replicas for surge only, scale up.",
			deploymentReplicas:  10,
			maxSurge:            intstr.FromInt(2),
			oldReplicas:         10,
			newReplicas:         0,
			scaleExpected:       true,
			expectedNewReplicas: 2,
		},
		{
			name:                "scenario 2. scale up old replicas to meet desired and surge, scale up.",
			deploymentReplicas:  10,
			maxSurge:            intstr.FromInt(2),
			oldReplicas:         5,
			newReplicas:         0,
			scaleExpected:       true,
			expectedNewReplicas: 7,
		},
		{
			name:               "scenario 3. old replica meet desired and new replica meet surge, no change",
			deploymentReplicas: 10,
			maxSurge:           intstr.FromInt(2),
			oldReplicas:        10,
			newReplicas:        2,
			scaleExpected:      false,
		},
		{
			name:                "scenario 4. old replica lower than desired, new replica exceed desired and surge, scale down",
			deploymentReplicas:  10,
			maxSurge:            intstr.FromInt(2),
			oldReplicas:         2,
			newReplicas:         11,
			scaleExpected:       true,
			expectedNewReplicas: 10,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			newMS := ms("foo-v2", test.newReplicas, nil, noTimestamp)
			oldMS := ms("foo-v2", test.oldReplicas, nil, noTimestamp)
			allMSs := []*v1alpha1.MachineSet{newMS, oldMS}
			maxUnavailable := intstr.FromInt(0)
			deployment := newMachineDeployment("foo", test.deploymentReplicas, nil, &test.maxSurge, &maxUnavailable, map[string]string{"foo": "bar"})

			rObjects := []runtime.Object{}
			rObjects = append(rObjects, oldMS)

			fakeClient := fake.NewSimpleClientset(rObjects...)
			controller := &MachineDeploymentControllerImpl{}
			controller.machineClient = fakeClient

			scaled, err := controller.reconcileNewMachineSet(allMSs, newMS, deployment)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !test.scaleExpected {
				if scaled || len(fakeClient.Actions()) > 0 {
					t.Fatalf("unexpected scaling: %v", fakeClient.Actions())
				}
			}
			if test.scaleExpected {
				if !scaled {
					t.Fatalf("expected scaling to occur")
				}
				if len(fakeClient.Actions()) != 1 {
					t.Fatalf("expected 1 action during scale, got: %v", fakeClient.Actions())
				}
				updated := fakeClient.Actions()[0].(core.UpdateAction).GetObject().(*v1alpha1.MachineSet)
				if e, a := test.expectedNewReplicas, int(*(updated.Spec.Replicas)); e != a {
					t.Fatalf("expected update to %d replicas, got %d", e, a)
				}
			}
		})
	}
}

func TestMachineDeploymentController_reconcileOldMachineSets(t *testing.T) {
	tests := []struct {
		name                   string
		deploymentReplicas     int
		maxUnavailable         intstr.IntOrString
		oldReplicas            int
		newReplicas            int
		readyMachinesFromOldMS int
		readyMachinesFromNewMS int
		scaleExpected          bool
		expectedOldReplicas    int
		expectedActions        int
	}{
		{
			name:                   "scenario 1: 10 desired, oldMS at 10, 10 ready, 1 max surge, 0 max unavailable => oldMS at 10, no scaling.",
			deploymentReplicas:     10,
			maxUnavailable:         intstr.FromInt(0),
			oldReplicas:            10,
			newReplicas:            0,
			readyMachinesFromOldMS: 10,
			readyMachinesFromNewMS: 0,
			scaleExpected:          false,
		},
		{
			name:                   "scenario 2: 10 desired, oldMS at 10, 10 ready, 1 max surge, 2 max unavailable => oldMS at 8, scale down by 2.",
			deploymentReplicas:     10,
			maxUnavailable:         intstr.FromInt(2),
			oldReplicas:            10,
			newReplicas:            0,
			readyMachinesFromOldMS: 10,
			readyMachinesFromNewMS: 0,
			scaleExpected:          true,
			expectedOldReplicas:    8,
			expectedActions:        1,
		},
		{ // expect unhealthy replicas from old machine sets been cleaned up
			name:                   "scenario 3: 10 desired, oldMS at 10, 8 ready, 1 max surge, 2 max unavailable => oldMS at 8, scale down by 0.",
			deploymentReplicas:     10,
			maxUnavailable:         intstr.FromInt(2),
			oldReplicas:            10,
			newReplicas:            0,
			readyMachinesFromOldMS: 8,
			readyMachinesFromNewMS: 0,
			scaleExpected:          true,
			expectedOldReplicas:    8,
			expectedActions:        1,
		},
		{ // expect 1 unhealthy replica from old machine sets been cleaned up, and 1 ready machine been scaled down
			name:                   "scenario 4: 10 desired, oldMS at 10, 9 ready, 1 max surge, 2 max unavailable => oldMS at 8, scale down by 1.",
			deploymentReplicas:     10,
			maxUnavailable:         intstr.FromInt(2),
			oldReplicas:            10,
			newReplicas:            0,
			readyMachinesFromOldMS: 9,
			readyMachinesFromNewMS: 0,
			scaleExpected:          true,
			expectedOldReplicas:    8,
			expectedActions:        2,
		},
		{ // the unavailable machines from the newMS would not make us scale down old MSs in a further step
			name:                   "scenario 5: 10 desired, oldMS at 8, newMS at 2, 1 max surge, 8 oldMS ready, 0 newMS ready, 2 max unavailable => no scale.",
			deploymentReplicas:     10,
			maxUnavailable:         intstr.FromInt(2),
			oldReplicas:            8,
			newReplicas:            2,
			readyMachinesFromOldMS: 8,
			readyMachinesFromNewMS: 0,
			scaleExpected:          false,
		},
		{
			name:                   "scenario 6: 10 desired, oldMS at 10, newMS at 1, 1 max surge, 0 max unavailable => oldMS at 9, scale down by 1.",
			deploymentReplicas:     10,
			maxUnavailable:         intstr.FromInt(0),
			oldReplicas:            10,
			newReplicas:            1,
			readyMachinesFromOldMS: 10,
			readyMachinesFromNewMS: 1,
			scaleExpected:          true,
			expectedOldReplicas:    9,
			expectedActions:        1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Logf(test.name)

			newSelector := map[string]string{"foo": "new"}
			oldSelector := map[string]string{"foo": "old"}
			newMS := ms("foo-new", test.newReplicas, newSelector, noTimestamp)
			newMS.Status.AvailableReplicas = int32(test.readyMachinesFromNewMS)
			oldMS := ms("foo-old", test.oldReplicas, oldSelector, noTimestamp)
			oldMS.Status.AvailableReplicas = int32(test.readyMachinesFromOldMS)
			oldMSs := []*v1alpha1.MachineSet{oldMS}
			allMSs := []*v1alpha1.MachineSet{oldMS, newMS}
			maxSurge := intstr.FromInt(1)
			deployment := newMachineDeployment("foo", test.deploymentReplicas, nil, &maxSurge, &test.maxUnavailable, newSelector)

			rObjects := []runtime.Object{}
			rObjects = append(rObjects, oldMS, newMS)

			fakeClient := fake.NewSimpleClientset(rObjects...)
			controller := &MachineDeploymentControllerImpl{}
			controller.machineClient = fakeClient

			scaled, err := controller.reconcileOldMachineSets(allMSs, oldMSs, newMS, deployment)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !test.scaleExpected {
				if scaled || len(fakeClient.Actions()) > 0 {
					t.Fatalf("unexpected scaling: %v", fakeClient.Actions())
				}
			}
			if test.scaleExpected {
				if !scaled {
					t.Errorf("expected scaling to occur")
				}
				if test.expectedActions != len(fakeClient.Actions()) {
					t.Fatalf("got %d actions, expected %d; %v", len(fakeClient.Actions()), test.expectedActions, fakeClient.Actions())
				}
				updated := fakeClient.Actions()[len(fakeClient.Actions())-1].(core.UpdateAction).GetObject().(*v1alpha1.MachineSet)
				if e, a := test.expectedOldReplicas, int(*(updated.Spec.Replicas)); e != a {
					t.Fatalf("expected update to %d replicas, got %d", e, a)
				}
			}
		})
	}
}

func TestMachineDeploymentController_cleanupUnhealthyReplicas(t *testing.T) {
	tests := []struct {
		name                 string
		oldReplicas          int
		readyMachines        int
		unHealthyMachines    int
		maxCleanupCount      int
		cleanupCountExpected int
	}{
		{
			name:                 "scenario 1. 2 unhealthy, max 1 cleanup => 1 cleanup.",
			oldReplicas:          10,
			readyMachines:        8,
			unHealthyMachines:    2,
			maxCleanupCount:      1,
			cleanupCountExpected: 1,
		},
		{
			name:                 "scenario 2. 2 unhealthy, max 3 cleanup => 2 cleanup.",
			oldReplicas:          10,
			readyMachines:        8,
			unHealthyMachines:    2,
			maxCleanupCount:      3,
			cleanupCountExpected: 2,
		},
		{
			name:                 "scenario 3. 2 unhealthy, max 0 cleanup => 0 cleanup.",
			oldReplicas:          10,
			readyMachines:        8,
			unHealthyMachines:    2,
			maxCleanupCount:      0,
			cleanupCountExpected: 0,
		},
		{
			name:                 "scenario 4. 0 unhealthy, max 3 cleanup => 0 cleanup.",
			oldReplicas:          10,
			readyMachines:        10,
			unHealthyMachines:    0,
			maxCleanupCount:      3,
			cleanupCountExpected: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Log(test.name)
			oldMS := ms("foo-v2", test.oldReplicas, nil, noTimestamp)
			oldMS.Status.AvailableReplicas = int32(test.readyMachines)
			oldMSs := []*v1alpha1.MachineSet{oldMS}
			maxSurge := intstr.FromInt(2)
			maxUnavailable := intstr.FromInt(2)
			deployment := newMachineDeployment("foo", 10, nil, &maxSurge, &maxUnavailable, nil)

			rObjects := []runtime.Object{}
			rObjects = append(rObjects, oldMS)

			fakeClient := fake.NewSimpleClientset(rObjects...)
			controller := &MachineDeploymentControllerImpl{}
			controller.machineClient = fakeClient

			_, cleanupCount, err := controller.cleanupUnhealthyReplicas(oldMSs, deployment, int32(test.maxCleanupCount))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if int(cleanupCount) != test.cleanupCountExpected {
				t.Fatalf("got %d clean up count, expected %d clean up count", cleanupCount, test.cleanupCountExpected)
			}
		})
	}
}

func TestMachineDeploymentController_scaleDownOldMachineSetsForRollingUpdate(t *testing.T) {
	tests := []struct {
		name                string
		deploymentReplicas  int
		maxUnavailable      intstr.IntOrString
		readyMachines       int
		oldReplicas         int
		scaleExpected       bool
		expectedOldReplicas int
	}{
		{
			name:               "scenario 1. 10 desired, oldMS at 10, 10 ready, max unavailable 0 => oldMS at 10, no scaling.",
			deploymentReplicas: 10,
			maxUnavailable:     intstr.FromInt(0),
			readyMachines:      10,
			oldReplicas:        10,
			scaleExpected:      false,
		},
		{
			name:                "scenario 2. 10 desired, oldMS at 10, 10 ready, max unavailable 2 => oldMS at 8, scale down by 2.",
			deploymentReplicas:  10,
			maxUnavailable:      intstr.FromInt(2),
			readyMachines:       10,
			oldReplicas:         10,
			scaleExpected:       true,
			expectedOldReplicas: 8,
		},
		{
			name:               "scenario 3. 10 desired, oldMS at 8, 8 ready, max unavailable 2 => oldMS at 8, no scaling.",
			deploymentReplicas: 10,
			maxUnavailable:     intstr.FromInt(2),
			readyMachines:      8,
			oldReplicas:        10,
			scaleExpected:      false,
		},
		{
			name:               "scenario 4. 10 desired, oldMS at 0, 10 ready, max unavailable 2 => oldMS at 0, no scaling.",
			deploymentReplicas: 10,
			maxUnavailable:     intstr.FromInt(2),
			readyMachines:      10,
			oldReplicas:        0,
			scaleExpected:      false,
		},
		{
			name:               "scenario 5. 10 desired, oldMS at 10, 1 ready, max unavailable 2 => oldMS at 10, no scaling.",
			deploymentReplicas: 10,
			maxUnavailable:     intstr.FromInt(2),
			readyMachines:      1,
			oldReplicas:        10,
			scaleExpected:      false,
		},
		{
			name:                "scenario 6. 10 desired, oldMS at 11, 11 ready, max unavailable 0 => oldMS at 10, scale down by 1.",
			deploymentReplicas:  10,
			maxUnavailable:      intstr.FromInt(0),
			readyMachines:       11,
			oldReplicas:         11,
			scaleExpected:       true,
			expectedOldReplicas: 10,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Logf(test.name)
			oldMS := ms("foo-v2", test.oldReplicas, nil, noTimestamp)
			oldMS.Status.AvailableReplicas = int32(test.readyMachines)
			allMSs := []*v1alpha1.MachineSet{oldMS}
			oldMSs := []*v1alpha1.MachineSet{oldMS}
			maxSurge := intstr.FromInt(1)
			deployment := newMachineDeployment("foo", test.deploymentReplicas, nil, &maxSurge, &test.maxUnavailable, map[string]string{"foo": "bar"})

			rObjects := []runtime.Object{}
			rObjects = append(rObjects, oldMS)

			fakeClient := fake.NewSimpleClientset(rObjects...)
			controller := &MachineDeploymentControllerImpl{}
			controller.machineClient = fakeClient

			scaled, err := controller.scaleDownOldMachineSetsForRollingUpdate(allMSs, oldMSs, deployment)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !test.scaleExpected {
				if scaled != 0 {
					t.Fatalf("unexpected scaling: %v", fakeClient.Actions())
				}
			}
			if test.scaleExpected {
				if scaled == 0 {
					t.Fatalf("expected scaling to occur; actions: %v", fakeClient.Actions())
				}
				// There are both list and update actions logged, so extract the update
				// action for verification.
				var updateAction core.UpdateAction
				for _, action := range fakeClient.Actions() {
					switch a := action.(type) {
					case core.UpdateAction:
						if updateAction != nil {
							t.Errorf("expected only 1 update action; had %v and found %v", updateAction, a)
						} else {
							updateAction = a
						}
					}
				}
				if updateAction == nil {
					t.Fatalf("expected an update action")
				}
				updated := updateAction.GetObject().(*v1alpha1.MachineSet)
				if e, a := test.expectedOldReplicas, int(*(updated.Spec.Replicas)); e != a {
					t.Fatalf("got %d replicas, expected %d replicas updated", a, e)
				}
			}
		})
	}
}
