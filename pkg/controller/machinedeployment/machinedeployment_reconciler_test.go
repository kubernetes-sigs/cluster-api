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

	. "github.com/onsi/gomega"
	"golang.org/x/net/context"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const timeout = time.Second * 10

func TestReconcile(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.TODO()

	labels := map[string]string{"foo": "bar"}
	version := "1.10.3"
	deployment := &clusterv1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
		Spec: clusterv1.MachineDeploymentSpec{
			MinReadySeconds:      pointer.Int32Ptr(0),
			Replicas:             pointer.Int32Ptr(2),
			RevisionHistoryLimit: pointer.Int32Ptr(0),
			Selector:             metav1.LabelSelector{MatchLabels: labels},
			Strategy: &clusterv1.MachineDeploymentStrategy{
				Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
				RollingUpdate: &clusterv1.MachineRollingUpdateDeployment{
					MaxUnavailable: intOrStrPtr(0),
					MaxSurge:       intOrStrPtr(1),
				},
			},
			Template: clusterv1.MachineTemplateSpec{
				ObjectMeta: clusterv1.ObjectMeta{
					Labels: labels,
				},
				Spec: clusterv1.MachineSpec{
					Version: &version,
					InfrastructureRef: corev1.ObjectReference{
						APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha2",
						Kind:       "InfrastructureMachineTemplate",
						Name:       "foo-template",
					},
				},
			},
		},
	}
	msListOpts := []client.ListOption{
		client.InNamespace("default"),
		client.MatchingLabels(labels),
	}

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{MetricsBindAddress: "0"})
	Expect(err).To(BeNil())
	c := mgr.GetClient()

	r := newReconciler(mgr)
	Expect(add(mgr, r, r.MachineSetToDeployments)).To(BeNil())
	defer close(StartTestManager(mgr, t))

	// Create infrastructure template resource.
	infraResource := map[string]interface{}{
		"kind":       "InfrastructureMachine",
		"apiVersion": "infrastructure.cluster.x-k8s.io/v1alpha2",
		"metadata":   map[string]interface{}{},
		"spec": map[string]interface{}{
			"size": "3xlarge",
		},
	}
	infraTmpl := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"spec": map[string]interface{}{
				"template": infraResource,
			},
		},
	}
	infraTmpl.SetKind("InfrastructureMachineTemplate")
	infraTmpl.SetAPIVersion("infrastructure.cluster.x-k8s.io/v1alpha2")
	infraTmpl.SetName("foo-template")
	infraTmpl.SetNamespace("default")
	Expect(c.Create(ctx, infraTmpl)).To(BeNil())

	// Create the MachineDeployment object and expect Reconcile to be called.
	Expect(c.Create(ctx, deployment)).To(BeNil())
	defer c.Delete(ctx, deployment)

	// Verify that the MachineSet was created.
	machineSets := &clusterv1.MachineSetList{}
	Eventually(func() int {
		if err := c.List(ctx, machineSets, msListOpts...); err != nil {
			return -1
		}
		return len(machineSets.Items)
	}, timeout).Should(BeEquivalentTo(1))

	firstMachineSet := machineSets.Items[0]
	Expect(*firstMachineSet.Spec.Replicas).To(BeEquivalentTo(2))
	Expect(*firstMachineSet.Spec.Template.Spec.Version).To(BeEquivalentTo("1.10.3"))

	//
	// Delete firstMachineSet and expect Reconcile to be called to replace it.
	//
	Expect(c.Delete(ctx, &firstMachineSet)).To(BeNil())
	Eventually(func() bool {
		if err := c.List(ctx, machineSets, msListOpts...); err != nil {
			return false
		}
		for _, ms := range machineSets.Items {
			if ms.UID == firstMachineSet.UID {
				return false
			}
		}
		return len(machineSets.Items) > 0
	}, timeout).Should(BeTrue())

	//
	// Scale the MachineDeployment and expect Reconcile to be called.
	//
	secondMachineSet := machineSets.Items[0]
	if err := updateMachineDeployment(c, deployment, func(d *clusterv1.MachineDeployment) { d.Spec.Replicas = pointer.Int32Ptr(5) }); err != nil {
		t.Errorf("error scaling machinedeployment: %v", err)
	}
	Eventually(func() int {
		key := client.ObjectKey{Name: secondMachineSet.Name, Namespace: secondMachineSet.Namespace}
		if err := c.Get(ctx, key, &secondMachineSet); err != nil {
			return -1
		}
		return int(*secondMachineSet.Spec.Replicas)
	}, timeout).Should(BeEquivalentTo(5))

	//
	// Update a MachineDeployment, expect Reconcile to be called and a new MachineSet to appear.
	//
	if err := updateMachineDeployment(c, deployment, func(d *clusterv1.MachineDeployment) { d.Spec.Template.Labels["updated"] = "true" }); err != nil {
		t.Errorf("error scaling machinedeployment: %v", err)
	}
	Eventually(func() int {
		if err := c.List(ctx, machineSets, msListOpts...); err != nil {
			return -1
		}
		return len(machineSets.Items)
	}, timeout).Should(BeEquivalentTo(2))

	//
	// Wait for the new MachineSet to get scaled up and set .Status.Replicas and .Status.AvailableReplicas at each step.
	//
	var newMachineSet, oldMachineSet *clusterv1.MachineSet
	for _, ms := range machineSets.Items {
		if ms.UID == secondMachineSet.UID {
			oldMachineSet = ms.DeepCopy()
		} else {
			newMachineSet = ms.DeepCopy()
		}
	}

	// Start off by setting .Status.Replicas and .Status.AvailableReplicas of the old MachineSet.
	oldMachineSetPatch := client.MergeFrom(oldMachineSet.DeepCopy())
	oldMachineSet.Status.AvailableReplicas = *oldMachineSet.Spec.Replicas
	oldMachineSet.Status.Replicas = *oldMachineSet.Spec.Replicas
	Expect(c.Status().Patch(ctx, oldMachineSet, oldMachineSetPatch)).To(BeNil())

	// Iterate over the scalesteps.
	step := 1
	for step < 5 {
		// Wait for newMachineSet to be scaled up
		Eventually(func() bool {
			key := types.NamespacedName{Namespace: newMachineSet.Namespace, Name: newMachineSet.Name}
			if err := c.Get(ctx, key, newMachineSet); err != nil {
				return false
			}

			currentReplicas := int(*newMachineSet.Spec.Replicas)
			step = currentReplicas
			return currentReplicas >= step && currentReplicas < 6
		}, timeout).Should(BeTrue())

		newMachineSetPatch := client.MergeFrom(newMachineSet.DeepCopy())
		newMachineSet.Status.Replicas = *newMachineSet.Spec.Replicas
		newMachineSet.Status.AvailableReplicas = *newMachineSet.Spec.Replicas
		Expect(c.Status().Patch(ctx, newMachineSet, newMachineSetPatch)).To(BeNil())

		// Wait for oldMachineSet to be scaled down
		updateOldMachineSet := true
		Eventually(func() bool {
			key := types.NamespacedName{Namespace: oldMachineSet.Namespace, Name: oldMachineSet.Name}
			if err := c.Get(ctx, key, oldMachineSet); err != nil {
				if apierrors.IsNotFound(err) {
					updateOldMachineSet = false
					return true
				}
				return false
			}
			return int(*oldMachineSet.Spec.Replicas) <= 5-step
		}, timeout).Should(BeTrue())

		if updateOldMachineSet {
			oldMachineSetPatch := client.MergeFrom(oldMachineSet.DeepCopy())
			oldMachineSet.Status.Replicas = *oldMachineSet.Spec.Replicas
			oldMachineSet.Status.AvailableReplicas = *oldMachineSet.Spec.Replicas
			oldMachineSet.Status.ObservedGeneration = oldMachineSet.Generation
			Expect(c.Status().Patch(ctx, oldMachineSet, oldMachineSetPatch)).To(BeNil())
		}
	}

	Eventually(func() int {
		if err := c.List(ctx, machineSets, msListOpts...); err != nil {
			return -1
		}
		return len(machineSets.Items)
	}, timeout).Should(BeEquivalentTo(1))
}

func intOrStrPtr(i int32) *intstr.IntOrString {
	// FromInt takes an int that must not be greater than int32...
	intstr := intstr.FromInt(int(i))
	return &intstr
}
