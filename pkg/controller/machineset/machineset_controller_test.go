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

	machinev1beta1 "github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
	"golang.org/x/net/context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var c client.Client

var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo", Namespace: "default"}}

const timeout = time.Second * 5

func TestReconcile(t *testing.T) {
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

	replicas := int32(2)
	labels := map[string]string{"foo": "bar"}

	testCases := []struct {
		name            string
		instance        *machinev1beta1.MachineSet
		expectedRequest reconcile.Request
		verifyFnc       func()
	}{
		{
			name: "Refuse invalid machineset (with invalid matching labels)",
			instance: &machinev1beta1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{Name: "invalidfoo", Namespace: "default"},
				Spec: machinev1beta1.MachineSetSpec{
					Replicas: &replicas,
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"foo": "bar"},
					},
					Template: machinev1beta1.MachineTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"foo": "bar2"},
						},
						Spec: machinev1beta1.MachineSpec{
							Versions: machinev1beta1.MachineVersionInfo{Kubelet: "1.10.3"},
						},
					},
				},
			},
			expectedRequest: reconcile.Request{NamespacedName: types.NamespacedName{Name: "invalidfoo", Namespace: "default"}},
			verifyFnc: func() {
				// expecting machineset validation error
				if _, err := r.Reconcile(reconcile.Request{
					NamespacedName: types.NamespacedName{Name: "invalidfoo", Namespace: "default"},
				}); err == nil {
					t.Errorf("expected validation error did not occur")
				}
			},
		},
		{
			name: "Create the MachineSet object and expect Reconcile to be called and the Machines to be created",
			instance: &machinev1beta1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
				Spec: machinev1beta1.MachineSetSpec{
					Replicas: &replicas,
					Selector: metav1.LabelSelector{
						MatchLabels: labels,
					},
					Template: machinev1beta1.MachineTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: labels,
						},
						Spec: machinev1beta1.MachineSpec{
							Versions: machinev1beta1.MachineVersionInfo{Kubelet: "1.10.3"},
						},
					},
				},
			},
			expectedRequest: reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo", Namespace: "default"}},
			// Verify machines are created and recreated after deletion
			verifyFnc: func() {
				machines := &machinev1beta1.MachineList{}

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
			},
		},
	}

	for _, tc := range testCases {
		t.Logf("Running %q testcase", tc.name)
		func() {
			if err := c.Create(context.TODO(), tc.instance); err != nil {
				t.Errorf("error creating instance: %v", err)
			}

			defer func() {
				c.Delete(context.TODO(), tc.instance)
				select {
				case recv := <-requests:
					if recv != tc.expectedRequest {
						t.Error("received request does not match expected request")
					}
				case <-time.After(timeout):
					t.Error("timed out waiting for request")
				}
			}()

			select {
			case recv := <-requests:
				if recv != tc.expectedRequest {
					t.Error("received request does not match expected request")
				}
			case <-time.After(timeout):
				t.Error("timed out waiting for request")
			}

			tc.verifyFnc()
		}()
	}
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
