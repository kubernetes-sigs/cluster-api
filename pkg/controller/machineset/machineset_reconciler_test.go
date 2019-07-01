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

	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"golang.org/x/net/context"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1alpha2 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var c client.Client

var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo", Namespace: "default"}}

const timeout = time.Second * 5

func TestReconcile(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	infraResource := new(unstructured.Unstructured)
	infraResource.SetKind("InfrastructureRef")
	infraResource.SetAPIVersion("infrastructure.cluster.sigs.k8s.io/v1alpha1")
	infraResource.SetName("foo-template")
	infraResource.SetNamespace("default")

	replicas := int32(2)
	version := "1.14.2"
	instance := &clusterv1alpha2.MachineSet{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
		Spec: clusterv1alpha2.MachineSetSpec{
			Replicas: &replicas,
			Template: clusterv1alpha2.MachineTemplateSpec{
				Spec: clusterv1alpha2.MachineSpec{
					Version: &version,
					InfrastructureRef: corev1.ObjectReference{
						APIVersion: "infrastructure.cluster.sigs.k8s.io/v1alpha1",
						Kind:       "InfrastructureRef",
						Name:       "foo-template",
					},
				},
			},
		},
	}

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{})
	g.Expect(err).To(gomega.BeNil())

	c = mgr.GetClient()
	g.Expect(c.Create(context.TODO(), infraResource)).To(gomega.BeNil())

	r := newReconciler(mgr)
	recFn, requests := SetupTestReconcile(r)
	if err := add(mgr, recFn, r.MachineToMachineSets); err != nil {
		t.Errorf("error adding controller to manager: %v", err)
	}
	defer close(StartTestManager(mgr, t))

	// Create the MachineSet object and expect Reconcile to be called and the Machines to be created.
	g.Expect(c.Create(context.TODO(), instance)).To(gomega.BeNil())
	defer c.Delete(context.TODO(), instance)
	select {
	case recv := <-requests:
		if recv != expectedRequest {
			t.Error("received request does not match expected request")
		}
	case <-time.After(timeout):
		t.Error("timed out waiting for request")
	}

	machines := &clusterv1alpha2.MachineList{}

	// TODO(joshuarubin) there seems to be a race here. If expectInt sleeps
	// briefly, even 10ms, the number of replicas is 4 and not 2 as expected
	expectInt(t, int(replicas), func(ctx context.Context) int {
		if err := c.List(ctx, machines, client.InNamespace("default")); err != nil {
			return -1
		}
		return len(machines.Items)
	})

	// Verify that each machine has the desired kubelet version.
	for _, m := range machines.Items {
		if k := m.Spec.Version; k == nil || *k != "1.14.2" {
			t.Errorf("kubelet was %v not '1.14.2'", k)
		}
	}

	// Verify that we have 3 infrastructure references: 1 template + 2 machines.
	infraConfigs := &unstructured.UnstructuredList{}
	infraConfigs.SetKind("InfrastructureRef")
	infraConfigs.SetAPIVersion("infrastructure.cluster.sigs.k8s.io/v1alpha1")
	expectInt(t, 3, func(ctx context.Context) int {
		if err := c.List(ctx, infraConfigs, client.InNamespace("default")); err != nil {
			return -1
		}
		return len(infraConfigs.Items)
	})

	// TODO (robertbailey): Figure out why the control loop isn't working as expected.
	// Delete a Machine and expect Reconcile to be called to replace it.
	//
	// More information: https://github.com/kubernetes-sigs/cluster-api/issues/1099
	//
	// m := machines.Items[0]
	// g.Expect(c.Delete(context.TODO(), &m)).To(gomega.BeNil())
	// select {
	// case recv := <-requests:
	// 	if recv != expectedRequest {
	// 		t.Error("received request does not match expected request")
	// 	}
	// case <-time.After(timeout):
	// 	t.Error("timed out waiting for request")
	// }
	// g.Eventually(func() int {
	// 	if err := c.List(context.TODO(), machines); err != nil {
	// 		return -1
	// 	}
	// 	return len(machines.Items)
	// }, timeout).Should(gomega.BeEquivalentTo(replicas))
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
