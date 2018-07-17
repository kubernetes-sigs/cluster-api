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

package cluster

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	core "k8s.io/client-go/testing"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/fake"
	"sigs.k8s.io/cluster-api/pkg/util"
)

func TestClusterSetControllerReconcileHandler(t *testing.T) {
	tests := []struct {
		name                      string
		objExists                 bool
		isDeleting                bool
		withFinalizer             bool
		isMaster                  bool
		ignoreDeleteCallCount     bool
		expectFinalizerRemoved    bool
		numExpectedReconcileCalls int64
		numExpectedDeleteCalls    int64
	}{
		{
			name:                      "Create cluster",
			objExists:                 false,
			numExpectedReconcileCalls: 1,
		},
		{
			name:                      "Update cluster",
			objExists:                 true,
			numExpectedReconcileCalls: 1,
		},
		{
			name:                   "Delete cluster, instance exists, with finalizer",
			objExists:              true,
			isDeleting:             true,
			withFinalizer:          true,
			expectFinalizerRemoved: true,
			numExpectedDeleteCalls: 1,
		},
		{
			// This should not be possible. Here for completeness.
			name:          "Delete cluster, instance exists without finalizer",
			objExists:     true,
			isDeleting:    true,
			withFinalizer: false,
		},
		{
			name:                   "Delete cluster, instance does not exist, with finalizer",
			objExists:              true,
			isDeleting:             true,
			withFinalizer:          true,
			ignoreDeleteCallCount:  true,
			expectFinalizerRemoved: true,
		},
		{
			name:          "Delete cluster, instance does not exist, without finalizer",
			objExists:     true,
			isDeleting:    true,
			withFinalizer: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clusterToTest := getCluster("bar", test.isDeleting, test.withFinalizer)
			knownObjects := []runtime.Object{}
			if test.objExists {
				knownObjects = append(knownObjects, clusterToTest)
			}

			clusterUpdated := false
			fakeClient := fake.NewSimpleClientset(knownObjects...)
			fakeClient.PrependReactor("update", "clusters", func(action core.Action) (bool, runtime.Object, error) {
				clusterUpdated = true
				return false, nil, nil
			})

			actuator := NewTestActuator()

			target := &ClusterControllerImpl{}
			target.actuator = actuator
			target.clientSet = fakeClient

			var err error
			err = target.Reconcile(clusterToTest)
			if err != nil {
				t.Fatal(err)
			}

			finalizerRemoved := false
			if clusterUpdated {
				updatedCluster, err := fakeClient.ClusterV1alpha1().Clusters(clusterToTest.Namespace).Get(clusterToTest.Name, metav1.GetOptions{})
				if err != nil {
					t.Fatalf("failed to get updated cluster.")
				}
				finalizerRemoved = !util.Contains(updatedCluster.ObjectMeta.Finalizers, v1alpha1.ClusterFinalizer)
			}

			if finalizerRemoved != test.expectFinalizerRemoved {
				t.Errorf("Got finalizer removed %v, expected finalizer removed %v", finalizerRemoved, test.expectFinalizerRemoved)
			}
			if actuator.ReconcileCallCount != test.numExpectedReconcileCalls {
				t.Errorf("Got %v create calls, expected %v", actuator.ReconcileCallCount, test.numExpectedReconcileCalls)
			}
			if actuator.DeleteCallCount != test.numExpectedDeleteCalls && !test.ignoreDeleteCallCount {
				t.Errorf("Got %v delete calls, expected %v", actuator.DeleteCallCount, test.numExpectedDeleteCalls)
			}

		})
	}
}

func getCluster(name string, isDeleting bool, hasFinalizer bool) *v1alpha1.Cluster {
	m := &v1alpha1.Cluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Cluster",
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
		m.ObjectMeta.SetFinalizers([]string{v1alpha1.ClusterFinalizer})
	}

	return m
}
