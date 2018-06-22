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
	"strconv"
	"sync"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/typed/cluster/v1alpha1"

	"k8s.io/client-go/tools/cache"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1/testutil"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
)

func machineControllerReconcile(t *testing.T, cs *clientset.Clientset, controller *MachineController, namespace string) {
	instance := clusterv1.Machine{}
	instance.Name = "instance-1"
	expectedKey := namespace + "/instance-1"

	// When creating a new object, it should invoke the reconcile method.
	cluster := testutil.GetVanillaCluster()
	cluster.Name = "cluster-1"
	clusterClient := cs.ClusterV1alpha1().Clusters(namespace)
	if _, err := clusterClient.Create(&cluster); err != nil {
		t.Fatal(err)
	}
	defer cleanUpCluster(clusterClient, cluster)

	client := cs.ClusterV1alpha1().Machines(namespace)
	before := make(chan struct{})
	after := make(chan struct{})
	var aftOnce, befOnce sync.Once

	actualKey := ""
	var actualErr error = nil

	// Setup test callbacks to be called when the message is reconciled.
	// Sometimes reconcile is called multiple times, so use Once to prevent closing the channels again.
	controller.BeforeReconcile = func(key string) {
		ns, _, error := cache.SplitMetaNamespaceKey(key)
		if error == nil && ns == namespace {
			actualKey = key
			befOnce.Do(func() { close(before) })
		}
	}
	controller.AfterReconcile = func(key string, err error) {
		ns, _, error := cache.SplitMetaNamespaceKey(key)
		if error == nil && ns == namespace {
			actualKey = key
			actualErr = err
			aftOnce.Do(func() { close(after) })
		}
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

func machineControllerConcurrentReconcile(t *testing.T, cs *clientset.Clientset,
	controller *MachineController) {
	// Create a cluster object.
	cluster := testutil.GetVanillaCluster()
	cluster.Name = "cluster-1"
	clusterClient := cs.ClusterV1alpha1().Clusters("default")
	if _, err := clusterClient.Create(&cluster); err != nil {
		t.Fatal(err)
	}
	defer cleanUpCluster(clusterClient, cluster)

	client := cs.ClusterV1alpha1().Machines("default")

	// Direct test actuator to block on Create() call.
	ta := controller.controller.actuator.(*TestActuator)
	ta.BlockOnCreate = true
	ta.CreateCallCount = 0
	defer ta.Unblock()

	// Create a few instances
	const numMachines = 5
	for i := 0; i < numMachines; i++ {
		instance := clusterv1.Machine{}
		instance.Name = "instance" + strconv.Itoa(i)
		if _, err := client.Create(&instance); err != nil {
			t.Fatal(err)
		}
	}

	err := wait.Poll(time.Second, 10*time.Second, func() (bool, error) {
		return (ta.CreateCallCount == numMachines), nil
	})

	if err != nil {
		t.Fatalf("The reconcilation didn't run in parallel.")
	}
}

func cleanUpCluster(clusterClient v1alpha1.ClusterInterface, cluster clusterv1.Cluster) {
	// We have to delete the finalizer since the cluster
	// controller is not running
	cluster.ObjectMeta.Finalizers = []string{}
	clusterClient.Update(&cluster)
	clusterClient.Delete(cluster.Name, &metav1.DeleteOptions{})
}
