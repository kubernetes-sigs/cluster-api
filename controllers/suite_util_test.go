/*
Copyright 2019 The Kubernetes Authors.

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

package controllers

import (
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
)

func intOrStrPtr(i int32) *intstr.IntOrString {
	// FromInt takes an int that must not be greater than int32...
	res := intstr.FromInt(int(i))
	return &res
}

func fakeInfrastructureRefReady(ref corev1.ObjectReference, base map[string]interface{}) {
	iref := (&unstructured.Unstructured{Object: base}).DeepCopy()
	Eventually(func() error {
		return k8sClient.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: ref.Namespace}, iref)
	}, timeout).Should(Succeed())

	ready, found, err := unstructured.NestedBool(iref.Object, "status", "ready")
	Expect(err).NotTo(HaveOccurred())
	if found && ready {
		return
	}

	irefPatch := client.MergeFrom(iref.DeepCopy())
	Expect(unstructured.SetNestedField(iref.Object, true, "status", "ready")).To(Succeed())
	Expect(k8sClient.Status().Patch(ctx, iref, irefPatch)).To(Succeed())
}

func fakeMachineNodeRef(m *clusterv1.Machine) {
	Eventually(func() error {
		key := client.ObjectKey{Name: m.Name, Namespace: m.Namespace}
		return k8sClient.Get(ctx, key, &clusterv1.Machine{})
	}, timeout).Should(Succeed())

	if m.Status.NodeRef != nil {
		return
	}

	// Create a new fake Node.
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: m.Name + "-",
		},
	}
	Expect(k8sClient.Create(ctx, node)).To(Succeed())

	Eventually(func() error {
		key := client.ObjectKey{Name: node.Name, Namespace: node.Namespace}
		return k8sClient.Get(ctx, key, &corev1.Node{})
	}, timeout).Should(Succeed())

	// Patch the node and make it look like ready.
	patchNode := client.MergeFrom(node.DeepCopy())
	node.Status.Conditions = append(node.Status.Conditions, corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionTrue})
	Expect(k8sClient.Status().Patch(ctx, node, patchNode)).To(Succeed())

	// Patch the Machine.
	patchMachine := client.MergeFrom(m.DeepCopy())
	m.Status.NodeRef = &corev1.ObjectReference{
		APIVersion: node.APIVersion,
		Kind:       node.Kind,
		Name:       node.Name,
		UID:        node.UID,
	}
	Expect(k8sClient.Status().Patch(ctx, m, patchMachine)).To(Succeed())
}
