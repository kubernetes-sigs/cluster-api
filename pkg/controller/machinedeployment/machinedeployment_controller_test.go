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
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/common"
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
	labels := map[string]string{"foo": "bar"}
	instance := &clusterv1alpha1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
		Spec: clusterv1alpha1.MachineDeploymentSpec{
			MinReadySeconds:      int32Ptr(0),
			Replicas:             int32Ptr(2),
			RevisionHistoryLimit: int32Ptr(0),
			Selector:             metav1.LabelSelector{MatchLabels: labels},
			Strategy: clusterv1alpha1.MachineDeploymentStrategy{
				Type: common.RollingUpdateMachineDeploymentStrategyType,
				RollingUpdate: &clusterv1alpha1.MachineRollingUpdateDeployment{
					MaxUnavailable: intstrPtr(0),
					MaxSurge:       intstrPtr(1),
				},
			},
			Template: clusterv1alpha1.MachineTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
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
	recFn, requests, errors := SetupTestReconcile(r)
	g.Expect(add(mgr, recFn, r.MachineSetToDeployments)).NotTo(gomega.HaveOccurred())
	defer close(StartTestManager(mgr, g))

	// Create the MachineDeployment object and expect Reconcile to be called.
	err = c.Create(context.TODO(), instance)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
	g.Eventually(errors, timeout).Should(gomega.Receive(gomega.BeNil()))

	// Verify that the MachineSet was created.
	machineSets := &clusterv1alpha1.MachineSetList{}
	g.Eventually(func() int {
		if err := c.List(context.TODO(), &client.ListOptions{}, machineSets); err != nil {
			return -1
		}
		return len(machineSets.Items)
	}, timeout).Should(gomega.BeEquivalentTo(1))
	ms := machineSets.Items[0]
	g.Expect(*ms.Spec.Replicas).Should(gomega.BeEquivalentTo(2))
	g.Expect(ms.Spec.Template.Spec.Versions.Kubelet).Should(gomega.Equal("1.10.3"))

	// Delete a MachineSet and expect Reconcile to be called to replace it.
	g.Expect(c.Delete(context.TODO(), &ms)).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
	g.Eventually(errors, timeout).Should(gomega.Receive(gomega.BeNil()))
	g.Eventually(func() int {
		if err := c.List(context.TODO(), &client.ListOptions{}, machineSets); err != nil {
			return -1
		}
		return len(machineSets.Items)
	}, timeout).Should(gomega.BeEquivalentTo(1))

	// Scale a MachineDeployment and expect Reconcile to be called
	err = updateMachineDeployment(c, instance, func(d *clusterv1alpha1.MachineDeployment) { d.Spec.Replicas = int32Ptr(5) })
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(c.Update(context.TODO(), instance)).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
	g.Eventually(errors, timeout).Should(gomega.Receive(gomega.BeNil()))
	g.Eventually(func() int32 {
		if err := c.List(context.TODO(), &client.ListOptions{}, machineSets); err != nil {
			return -1
		}
		if len(machineSets.Items) != 1 {
			return -1
		}
		return *machineSets.Items[0].Spec.Replicas
	}, timeout).Should(gomega.BeEquivalentTo(5))

	// Update a MachineDeployment, expect Reconcile to be called and a new MachineSet to appear
	err = updateMachineDeployment(c, instance, func(d *clusterv1alpha1.MachineDeployment) { d.Spec.Template.Labels["updated"] = "true" })
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(c.Update(context.TODO(), instance)).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
	g.Eventually(errors, timeout).Should(gomega.Receive(gomega.BeNil()))
	g.Eventually(func() int {
		if err := c.List(context.TODO(), &client.ListOptions{}, machineSets); err != nil {
			return -1
		}
		return len(machineSets.Items)
	}, timeout).Should(gomega.BeEquivalentTo(2))

	// Wait for the new MachineSet to get scaled up and set .Status.Replicas and .Status.AvailableReplicas
	// at each step
	var newMachineSet, oldMachineSet *clusterv1alpha1.MachineSet
	if machineSets.Items[0].CreationTimestamp.Before(&machineSets.Items[1].CreationTimestamp) {
		newMachineSet = &machineSets.Items[0]
		oldMachineSet = &machineSets.Items[1]
	} else {
		newMachineSet = &machineSets.Items[1]
		oldMachineSet = &machineSets.Items[0]
	}

	// Start off by setting .Status.Replicas and .Status.AvailableReplicas of the old MachineSet
	oldMachineSet.Status.AvailableReplicas = *oldMachineSet.Spec.Replicas
	oldMachineSet.Status.Replicas = *oldMachineSet.Spec.Replicas
	g.Expect(c.Status().Update(context.TODO(), oldMachineSet)).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
	g.Eventually(errors, timeout).Should(gomega.Receive(gomega.BeNil()))

	// Iterate over the scalesteps
	for i := int32(1); i < 6; i++ {
		// Wait for newMachineSet to be scaled up
		g.Eventually(func() int32 {
			if err := c.Get(context.TODO(), types.NamespacedName{
				Namespace: newMachineSet.Namespace, Name: newMachineSet.Name}, newMachineSet); err != nil {
				return -1
			}
			return *newMachineSet.Spec.Replicas
		}, timeout).Should(gomega.BeEquivalentTo(i))
		// Set its status
		newMachineSet.Status.Replicas = *newMachineSet.Spec.Replicas
		newMachineSet.Status.AvailableReplicas = *newMachineSet.Spec.Replicas
		g.Expect(c.Status().Update(context.TODO(), newMachineSet)).NotTo(gomega.HaveOccurred())
		g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
		g.Eventually(errors, timeout).Should(gomega.Receive(gomega.BeNil()))
		// Wait for oldMachineSet to be scaled down
		g.Eventually(func() int32 {
			if err := c.Get(context.TODO(), types.NamespacedName{
				Namespace: oldMachineSet.Namespace, Name: oldMachineSet.Name}, oldMachineSet); err != nil {
				return -1
			}
			return *oldMachineSet.Spec.Replicas
		}, timeout).Should(gomega.BeEquivalentTo(5 - i))
		// Set its status
		oldMachineSet.Status.Replicas = *oldMachineSet.Spec.Replicas
		oldMachineSet.Status.AvailableReplicas = *oldMachineSet.Spec.Replicas
		oldMachineSet.Status.ObservedGeneration = oldMachineSet.Generation
		g.Expect(c.Status().Update(context.TODO(), oldMachineSet)).NotTo(gomega.HaveOccurred())
		g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
		g.Eventually(errors, timeout).Should(gomega.Receive(gomega.BeNil()))
	}

	// Expect the old MachineSet to be removed
	g.Eventually(func() int {
		if err := c.List(context.TODO(), &client.ListOptions{}, machineSets); err != nil {
			return -1
		}
		return len(machineSets.Items)
	}, timeout).Should(gomega.BeEquivalentTo(1))
}

func int32Ptr(i int32) *int32 {
	return &i
}

func intstrPtr(i int32) *intstr.IntOrString {
	// FromInt takes an int that must not be greater than int32...
	intstr := intstr.FromInt(int(i))
	return &intstr
}
