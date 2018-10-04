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
	"time"

	"github.com/onsi/gomega"
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
	g := gomega.NewGomegaWithT(t)
	replicas := int32(2)
	instance := &clusterv1alpha1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
		Spec: clusterv1alpha1.MachineDeploymentSpec{
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
	g.Expect(err).NotTo(gomega.HaveOccurred())
	c = mgr.GetClient()

	r := newReconciler(mgr)
	recFn, requests := SetupTestReconcile(r)
	g.Expect(add(mgr, recFn, r.MachineSetToDeployments)).NotTo(gomega.HaveOccurred())
	defer close(StartTestManager(mgr, g))

	// Create the MachineDeployment object and expect Reconcile to be called.
	err = c.Create(context.TODO(), instance)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	/*
		// Verify that the MachineSet was created.
		machineSets := &clusterv1alpha1.MachineSetList{}
		g.Eventually(func() int {
			if err := c.List(context.TODO(), &client.ListOptions{}, machineSets); err != nil {
				return -1
			}
			return len(machineSets.Items)
		}, timeout).Should(gomega.BeEquivalentTo(1))
		ms := machineSets.Items[0]
		g.Expect(ms.Spec.Replicas).Should(gomega.BeEquivalentTo(1))
		g.Expect(ms.Spec.Template.Spec.Versions.Kubelet).Should(gomega.Equal("1.10.3"))

		// Verify that Machines were created with the desired kubelet version.
		// TODO: remove this?
		machines := &clusterv1alpha1.MachineList{}
		g.Eventually(func() int {
			if err := c.List(context.TODO(), &client.ListOptions{}, machines); err != nil {
				return -1
			}
			return len(machines.Items)
		}, timeout).Should(gomega.BeEquivalentTo(replicas))
		for _, m := range machines.Items {
			g.Expect(m.Spec.Versions.Kubelet).Should(gomega.Equal("1.10.3"))
		}

		// Delete a MachineSet and expect Reconcile to be called to replace it.
		g.Expect(c.Delete(context.TODO(), &ms)).NotTo(gomega.HaveOccurred())
		g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	*/
}
