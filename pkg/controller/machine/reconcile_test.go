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

package machine

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	core "k8s.io/client-go/testing"
	clustercommon "sigs.k8s.io/cluster-api/pkg/apis/cluster/common"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1/testutil"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/fake"
	"sigs.k8s.io/cluster-api/util"
)

func TestMachineSetControllerReconcileHandler(t *testing.T) {
	tests := []struct {
		name                   string
		objExists              bool
		instanceExists         bool
		isDeleting             bool
		withFinalizer          bool
		isMaster               bool
		ignoreDeleteCallCount  bool
		expectFinalizerRemoved bool
		numExpectedCreateCalls int64
		numExpectedDeleteCalls int64
		numExpectedUpdateCalls int64
		numExpectedExistsCalls int64
	}{
		{
			name:                   "Create machine",
			objExists:              false,
			instanceExists:         false,
			numExpectedCreateCalls: 1,
			numExpectedExistsCalls: 1,
		},
		{
			name:                   "Update machine",
			objExists:              true,
			instanceExists:         true,
			numExpectedUpdateCalls: 1,
			numExpectedExistsCalls: 1,
		},
		{
			name:                   "Delete machine, instance exists, with finalizer",
			objExists:              true,
			instanceExists:         true,
			isDeleting:             true,
			withFinalizer:          true,
			expectFinalizerRemoved: true,
			numExpectedDeleteCalls: 1,
		},
		{
			// This should not be possible. Here for completeness.
			name:           "Delete machine, instance exists without finalizer",
			objExists:      true,
			instanceExists: true,
			isDeleting:     true,
			withFinalizer:  false,
		},
		{
			name:                   "Delete machine, instance does not exist, with finalizer",
			objExists:              true,
			instanceExists:         false,
			isDeleting:             true,
			withFinalizer:          true,
			ignoreDeleteCallCount:  true,
			expectFinalizerRemoved: true,
		},
		{
			name:           "Delete machine, instance does not exist, without finalizer",
			objExists:      true,
			instanceExists: false,
			isDeleting:     true,
			withFinalizer:  false,
		},
		{
			name:           "Delete machine, skip master",
			objExists:      true,
			instanceExists: true,
			isDeleting:     true,
			withFinalizer:  true,
			isMaster:       true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			machineToTest := getMachine("bar", test.isDeleting, test.withFinalizer, test.isMaster)
			knownObjects := []runtime.Object{}
			if test.objExists {
				knownObjects = append(knownObjects, machineToTest)
			}

			machineUpdated := false
			fakeClient := fake.NewSimpleClientset(knownObjects...)
			fakeMachineClient := fakeClient.Cluster().Machines(metav1.NamespaceDefault)
			fakeClient.PrependReactor("update", "machines", func(action core.Action) (bool, runtime.Object, error) {
				machineUpdated = true
				return false, nil, nil
			})

			// When creating a new object, it should invoke the reconcile method.
			cluster := testutil.GetVanillaCluster()
			cluster.Name = "cluster-1"
			if _, err := fakeClient.ClusterV1alpha1().Clusters(metav1.NamespaceDefault).Create(&cluster); err != nil {
				t.Fatal(err)
			}

			actuator := NewTestActuator()
			actuator.ExistsValue = test.instanceExists

			target := &MachineControllerImpl{}
			target.actuator = actuator
			target.clientSet = fakeClient
			target.machineClient = fakeMachineClient

			var err error
			err = target.Reconcile(machineToTest)
			if err != nil {
				t.Fatal(err)
			}

			finalizerRemoved := machineUpdated && !util.Contains(machineToTest.ObjectMeta.Finalizers, v1alpha1.MachineFinalizer)

			if finalizerRemoved != test.expectFinalizerRemoved {
				t.Errorf("Got finalizer removed %v, expected finalizer removed %v", finalizerRemoved, test.expectFinalizerRemoved)
			}
			if actuator.CreateCallCount != test.numExpectedCreateCalls {
				t.Errorf("Got %v create calls, expected %v", actuator.CreateCallCount, test.numExpectedCreateCalls)
			}
			if actuator.DeleteCallCount != test.numExpectedDeleteCalls && !test.ignoreDeleteCallCount {
				t.Errorf("Got %v delete calls, expected %v", actuator.DeleteCallCount, test.numExpectedDeleteCalls)
			}
			if actuator.UpdateCallCount != test.numExpectedUpdateCalls {
				t.Errorf("Got %v update calls, expected %v", actuator.UpdateCallCount, test.numExpectedUpdateCalls)
			}
			if actuator.ExistsCallCount != test.numExpectedExistsCalls {
				t.Errorf("Got %v exists calls, expected %v", actuator.ExistsCallCount, test.numExpectedExistsCalls)
			}

		})
	}
}

func getMachine(name string, isDeleting, hasFinalizer, isMaster bool) *v1alpha1.Machine {
	m := &v1alpha1.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Machine",
			APIVersion: v1alpha1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metav1.NamespaceDefault,
		},
	}
	if isDeleting {
		now := metav1.NewTime(time.Now())
		m.ObjectMeta.SetDeletionTimestamp(&now)
	}
	if hasFinalizer {
		m.ObjectMeta.SetFinalizers([]string{v1alpha1.MachineFinalizer})
	}
	if isMaster {
		m.Spec.Roles = []clustercommon.MachineRole{clustercommon.MasterRole}
	}

	return m
}
