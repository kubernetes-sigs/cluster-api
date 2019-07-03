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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo", Namespace: "default"}}

const timeout = time.Second * 5

func TestReconcile(t *testing.T) {
	instance := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
		Spec: clusterv1.MachineSpec{
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: "infrastructure.clusters.k8s.io/v1alpha1",
				Kind:       "InfrastructureRef",
				Name:       "machine-infrastructure",
			},
		},
	}

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{MetricsBindAddress: "0"})
	if err != nil {
		t.Fatalf("error creating new manager: %v", err)
	}
	c := mgr.GetClient()

	reconciler := newReconciler(mgr)
	recFn, requests := SetupTestReconcile(reconciler)
	controller, err := addController(mgr, recFn)
	if err != nil {
		t.Fatalf("error adding controller to manager: %v", err)
	}
	reconciler.controller = controller
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
}
