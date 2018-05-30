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

	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1/testutil"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
	"sync"
)

func clusterControllerReconcile(t *testing.T, cs *clientset.Clientset, controller *ClusterController) {
	instance := testutil.GetVanillaCluster()
	instance.Name = "instance-1"
	expectedKey := "cluster-controller-test-handler/instance-1"

	// When creating a new object, it should invoke the reconcile method.
	client := cs.ClusterV1alpha1().Clusters("cluster-controller-test-handler")
	before := make(chan struct{})
	after := make(chan struct{})

	actualKey := ""
	var actualErr error = nil

	var aftOnce, befOnce sync.Once

	// Setup test callbacks to be called when the message is reconciled
	controller.BeforeReconcile = func(key string) {
		actualKey = key
		befOnce.Do(func() { close(before) })
	}
	controller.AfterReconcile = func(key string, err error) {
		actualKey = key
		actualErr = err
		aftOnce.Do(func() { close(after) })
	}

	// Create an instance
	if _, err := client.Create(&instance); err != nil {
		t.Fatal(err)
	}
	defer client.Delete(instance.Name, &metav1.DeleteOptions{})
	// Verify reconcile function is called against the correct key
	select {
	case <-before:
		if actualKey != expectedKey {
			t.Fatalf(
				"Reconcile function was not called with the correct key.\nActual:\t%+v\nExpected:\t%+v",
				actualKey, expectedKey)
		}
		if actualErr != nil {
			t.Fatal(actualErr)
		}
	case <-time.After(time.Second * 2):
		t.Fatalf("reconcile never called")
	}

	select {
	case <-after:
		if actualKey != expectedKey {
			t.Fatalf(
				"Reconcile function was not called with the correct key.\nActual:\t%+v\nExpected:\t%+v",
				actualKey, expectedKey)
		}
		if actualErr != nil {
			t.Fatal(actualErr)
		}
	case <-time.After(time.Second * 2):
		t.Fatalf("reconcile never finished")
	}
}
