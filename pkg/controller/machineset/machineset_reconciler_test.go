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
	"context"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clusterv1alpha2 "sigs.k8s.io/cluster-api/api/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const timeout = time.Second * 10

func TestReconcile(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.TODO()

	replicas := int32(2)
	version := "1.14.2"
	instance := &clusterv1alpha2.MachineSet{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
		Spec: clusterv1alpha2.MachineSetSpec{
			Replicas: &replicas,
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"label-1": "true",
				},
			},
			Template: clusterv1alpha2.MachineTemplateSpec{
				ObjectMeta: clusterv1alpha2.ObjectMeta{
					Labels: map[string]string{
						"label-1": "true",
					},
				},
				Spec: clusterv1alpha2.MachineSpec{
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

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{MetricsBindAddress: "0"})
	Expect(err).To(BeNil())
	c := mgr.GetClient()

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

	r := newReconciler(mgr)
	if err := add(mgr, r, r.MachineToMachineSets); err != nil {
		t.Errorf("error adding controller to manager: %v", err)
	}
	defer close(StartTestManager(mgr, t))

	// Create the MachineSet object and expect Reconcile to be called and the Machines to be created.
	Expect(c.Create(ctx, instance)).To(BeNil())
	defer c.Delete(ctx, instance)

	machines := &clusterv1alpha2.MachineList{}

	// Verify that we have 2 replicas.
	Eventually(func() int {
		if err := c.List(ctx, machines); err != nil {
			return -1
		}
		return len(machines.Items)
	}, timeout).Should(BeEquivalentTo(replicas))

	// Try to delete 1 machine and check the MachineSet scales back up.
	machineToBeDeleted := machines.Items[0]
	Expect(c.Delete(ctx, &machineToBeDeleted)).To(BeNil())

	// Verify that the Machine has been deleted.
	Eventually(func() bool {
		key := client.ObjectKey{Name: machineToBeDeleted.Name, Namespace: machineToBeDeleted.Namespace}
		if err := c.Get(ctx, key, &machineToBeDeleted); apierrors.IsNotFound(err) {
			// The Machine Controller usually deletes external references upon Machine deletion.
			// Replicate the logic here to make sure there are no leftovers.
			iref := &unstructured.Unstructured{Object: infraResource}
			iref.SetName(machineToBeDeleted.Spec.InfrastructureRef.Name)
			iref.SetNamespace("default")
			Expect(r.Delete(ctx, iref)).To(BeNil())
			return true
		}
		return false
	}, timeout).Should(BeTrue())

	// Verify that we have 2 replicas.
	Eventually(func() (ready int) {
		if err := c.List(ctx, machines); err != nil {
			return -1
		}
		return len(machines.Items)
	}, timeout).Should(BeEquivalentTo(replicas))

	// Verify that each machine has the desired kubelet version,
	// create a fake node in Ready state, update NodeRef, and wait for a reconciliation request.
	for _, m := range machines.Items {
		Expect(m.Spec.Version).ToNot(BeNil())
		Expect(*m.Spec.Version).To(BeEquivalentTo("1.14.2"))

		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
		}
		Expect(c.Create(ctx, node))

		node.Status.Conditions = append(node.Status.Conditions, corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionTrue})
		Expect(c.Status().Update(ctx, node)).To(BeNil())

		m.Status.NodeRef = &corev1.ObjectReference{
			APIVersion: node.APIVersion,
			Kind:       node.Kind,
			Name:       node.Name,
			UID:        node.UID,
		}
		Expect(c.Status().Update(ctx, &m)).To(BeNil())
	}

	// Verify that we have N=replicas infrastructure references.
	infraConfigs := &unstructured.UnstructuredList{}
	infraConfigs.SetKind(infraResource["kind"].(string))
	infraConfigs.SetAPIVersion(infraResource["apiVersion"].(string))
	Eventually(func() int {
		if err := c.List(ctx, infraConfigs, client.InNamespace("default")); err != nil {
			return -1
		}
		return len(infraConfigs.Items)
	}, timeout).Should(BeEquivalentTo(replicas))

	// Verify that all Machines are Ready.
	Eventually(func() int32 {
		key := client.ObjectKey{Name: instance.Name, Namespace: instance.Namespace}
		if err := c.Get(ctx, key, instance); err != nil {
			return -1
		}
		return instance.Status.AvailableReplicas
	}, timeout).Should(BeEquivalentTo(replicas))

	Eventually(func() int {
		if err := c.List(ctx, infraConfigs, client.InNamespace("default")); err != nil {
			return -1
		}
		return len(infraConfigs.Items)
	}, timeout).Should(BeEquivalentTo(replicas))
}
