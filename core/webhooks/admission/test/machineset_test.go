/*
Copyright 2025 The Kubernetes Authors.

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

package test

import (
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestMachineSetClusterNameImmutable(t *testing.T) {
	g := NewWithT(t)

	ms := builder.MachineSet("default", "machineset-clustername-immutable").
		WithClusterName("cluster1").
		WithBootstrapTemplate(builder.BootstrapTemplate("default", "bootstrap-template-ms-clustername-immutable").Build()).
		Build()

	g.Expect(env.CreateAndWait(ctx, ms)).To(Succeed())
	t.Cleanup(func() {
		g.Expect(env.CleanupAndWait(ctx, ms)).To(Succeed())
	})

	actualMS := &clusterv1.MachineSet{}
	g.Expect(env.Get(ctx, client.ObjectKey{Namespace: ms.Namespace, Name: ms.Name}, actualMS)).To(Succeed())

	// Changing spec.clusterName on update must be rejected by the CEL immutability rule.
	actualMS.Spec.ClusterName = "cluster2"
	err := env.Update(ctx, actualMS)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("field is immutable"))

	// An update that leaves spec.clusterName unchanged must still be allowed.
	g.Expect(env.Get(ctx, client.ObjectKey{Namespace: ms.Namespace, Name: ms.Name}, actualMS)).To(Succeed())
	actualMS.Spec.Template.Spec.Version = "v1.20.0"
	g.Expect(env.Update(ctx, actualMS)).To(Succeed())
}

func TestMachineSetClusterNameMatchesTemplate(t *testing.T) {
	g := NewWithT(t)

	ms := builder.MachineSet("default", "machineset-clustername-mismatch").
		WithBootstrapTemplate(builder.BootstrapTemplate("default", "bootstrap-template-ms-clustername-mismatch").Build()).
		Build()
	ms.Spec.ClusterName = "cluster1"
	ms.Spec.Template.Spec.ClusterName = "cluster2"

	// A MachineSet whose spec.clusterName does not match spec.template.spec.clusterName must be rejected by the CEL rule.
	err := env.CreateAndWait(ctx, ms)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("spec.clusterName must match spec.template.spec.clusterName"))
}

func TestMachineSetSelectorImmutable(t *testing.T) {
	g := NewWithT(t)

	ms := builder.MachineSet("default", "machineset-selector-immutable").
		WithClusterName("cluster1").
		WithBootstrapTemplate(builder.BootstrapTemplate("default", "bootstrap-template-ms-selector-immutable").Build()).
		Build()
	ms.Spec.Selector = metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}}
	ms.Spec.Template.Labels = map[string]string{"foo": "bar"}

	g.Expect(env.CreateAndWait(ctx, ms)).To(Succeed())
	t.Cleanup(func() {
		g.Expect(env.CleanupAndWait(ctx, ms)).To(Succeed())
	})

	actualMS := &clusterv1.MachineSet{}
	g.Expect(env.Get(ctx, client.ObjectKey{Namespace: ms.Namespace, Name: ms.Name}, actualMS)).To(Succeed())

	// Changing spec.selector on update must be rejected by the CEL immutability rule.
	// This is the fix for the orphaned-Machines bug (OCPBUGS-38218): previously, changing
	// spec.selector together with spec.template.metadata.labels caused existing Machines to
	// fall out of the selector's match set and become orphaned from the MachineSet.
	actualMS.Spec.Selector = metav1.LabelSelector{MatchLabels: map[string]string{"foo": "different"}}
	err := env.Update(ctx, actualMS)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("field is immutable"))

	// An update that leaves spec.selector unchanged must still be allowed.
	g.Expect(env.Get(ctx, client.ObjectKey{Namespace: ms.Namespace, Name: ms.Name}, actualMS)).To(Succeed())
	actualMS.Spec.Template.Spec.Version = "v1.20.0"
	g.Expect(env.Update(ctx, actualMS)).To(Succeed())
}
