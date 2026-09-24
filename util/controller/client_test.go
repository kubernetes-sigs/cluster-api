/*
Copyright 2026 The Kubernetes Authors.

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

package controller

import (
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func Test_Delete(t *testing.T) {
	ctx := t.Context()
	g := NewWithT(t)

	ns, err := env.CreateNamespace(ctx, "test-client")
	g.Expect(err).ToNot(HaveOccurred())

	ms := &clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ms",
			Namespace: ns.Name,
		},
		Spec: clusterv1.MachineSetSpec{
			ClusterName: "test1",
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					ClusterName: "test1",
					Bootstrap: clusterv1.Bootstrap{
						DataSecretName: ptr.To("data-secret"),
					},
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						APIGroup: builder.InfrastructureGroupVersion.Group,
						Kind:     builder.TestInfrastructureMachineTemplateKind,
						Name:     "inframachinetemplate",
					},
				},
			},
		},
	}
	msWithFinalizer := ms.DeepCopy()
	msWithFinalizer.Name = "ms-with-finalizer"
	msWithFinalizer.Finalizers = []string{"finalizer.example.com/test"}

	c, err := NewClientWithDeleteResponse(
		&clusterv1.MachineSet{}, schema.GroupResource{
			Group:    clusterv1.GroupVersion.Group,
			Resource: "machinesets",
		},
		env.GetScheme(), env.GetConfig(), env.GetHTTPClient())
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(env.CreateAndWait(ctx, ms)).To(Succeed())
	g.Expect(env.CreateAndWait(ctx, msWithFinalizer)).To(Succeed())

	msOriginal := ms.DeepCopy()
	deletedMS, err := c.Delete(ctx, ms)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ms).To(Equal(msOriginal))
	g.Expect(deletedMS).To(BeNil())

	msOriginal = msWithFinalizer.DeepCopy()
	deletedMS, err = c.Delete(ctx, msWithFinalizer)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(msWithFinalizer).To(Equal(msOriginal))
	g.Expect(msWithFinalizer).ToNot(BeNil())
	g.Expect(deletedMS.GetResourceVersion()).ToNot(BeEmpty())
}
