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

	"golang.org/x/net/context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var c client.Client

var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo", Namespace: "default"}}

const timeout = time.Second * 5

func TestReconcile(t *testing.T) {
	instance := &clusterv1alpha1.Machine{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
		Spec: clusterv1alpha1.MachineSpec{
			Versions: clusterv1alpha1.MachineVersionInfo{Kubelet: "1.10.3"},
		},
	}

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{})
	if err != nil {
		t.Fatalf("error creating new manager: %v", err)
	}
	c = mgr.GetClient()

	a := newTestActuator()
	recFn, requests := SetupTestReconcile(newReconciler(mgr, a))
	if err := add(mgr, recFn); err != nil {
		t.Fatalf("error adding controller to manager: %v", err)
	}
	defer close(StartTestManager(mgr, t))

	// Create the Machine object and expect Reconcile and the actuator to be called
	if err := c.Create(context.TODO(), instance); err != nil {
		t.Fatalf("error creating instance: %v", err)
	}
	defer c.Delete(context.TODO(), instance)
	select {
	case recv := <-requests:
		if recv != expectedRequest {
			t.Error("received request does not match expected request")
		}
	case <-time.After(timeout):
		t.Error("timed out waiting for request")
	}

	// TODO: Verify that the actuator is called correctly on Create
}
