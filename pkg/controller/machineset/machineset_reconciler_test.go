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
	replicas := int32(2)
	instance := &clusterv1alpha1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
		Spec: clusterv1alpha1.MachineSetSpec{
			Replicas: &replicas,
			Template: clusterv1alpha1.MachineTemplateSpec{
				Spec: clusterv1alpha1.MachineSpec{
					Versions: clusterv1alpha1.MachineVersionInfo{Kubelet: "1.10.3"},
				},
			},
		},
	}

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{})
	if err != nil {
		t.Errorf("error creating new manager: %v", err)
	}
	c = mgr.GetClient()

	r := newReconciler(mgr)
	recFn, requests := SetupTestReconcile(r)
	if err := add(mgr, recFn, r.MachineToMachineSets); err != nil {
		t.Errorf("error adding controller to manager: %v", err)
	}
	defer close(StartTestManager(mgr, t))

	// Create the MachineSet object and expect Reconcile to be called and the Machines to be created.
	if err := c.Create(context.TODO(), instance); err != nil {
		t.Errorf("error creating instance: %v", err)
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

	machines := &clusterv1alpha1.MachineList{}

	// TODO(joshuarubin) there seems to be a race here. If expectInt sleeps
	// briefly, even 10ms, the number of replicas is 4 and not 2 as expected
	expectInt(t, int(replicas), func(ctx context.Context) int {
		if err := c.List(ctx, &client.ListOptions{}, machines); err != nil {
			return -1
		}
		return len(machines.Items)
	})

	// Verify that each machine has the desired kubelet version.
	for _, m := range machines.Items {
		if k := m.Spec.Versions.Kubelet; k != "1.10.3" {
			t.Errorf("kubelet was %q not '1.10.3'", k)
		}
	}

	// Delete a Machine and expect Reconcile to be called to replace it.
	m := machines.Items[0]
	if err := c.Delete(context.TODO(), &m); err != nil {
		t.Errorf("error deleting machine: %v", err)
	}
	select {
	case recv := <-requests:
		if recv != expectedRequest {
			t.Error("received request does not match expected request")
		}
	case <-time.After(timeout):
		t.Error("timed out waiting for request")
	}

	// TODO (robertbailey): Figure out why the control loop isn't working as expected.
	/*
		g.Eventually(func() int {
			if err := c.List(context.TODO(), &client.ListOptions{}, machines); err != nil {
				return -1
			}
			return len(machines.Items)
		}, timeout).Should(gomega.BeEquivalentTo(replicas))
	*/
}

func expectInt(t *testing.T, expect int, fn func(context.Context) int) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.TODO(), timeout)
	defer cancel()

	intCh := make(chan int)

	go func() { intCh <- fn(ctx) }()

	select {
	case n := <-intCh:
		if n != expect {
			t.Errorf("go unexpectef value %d, expected %d", n, expect)
		}
	case <-ctx.Done():
		t.Errorf("timed out waiting for value: %d", expect)
	}
}
