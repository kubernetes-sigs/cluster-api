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
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestMachinePoolClusterNameImmutable(t *testing.T) {
	g := NewWithT(t)

	mp := builder.MachinePool("default", "machinepool-clustername-immutable").
		WithClusterName("cluster1").
		WithBootstrap(builder.BootstrapTemplate("default", "bootstrap-template-mp-clustername-immutable").Build()).
		Build()

	g.Expect(env.CreateAndWait(ctx, mp)).To(Succeed())
	t.Cleanup(func() {
		g.Expect(env.CleanupAndWait(ctx, mp)).To(Succeed())
	})

	actualMP := &clusterv1.MachinePool{}
	g.Expect(env.Get(ctx, client.ObjectKey{Namespace: mp.Namespace, Name: mp.Name}, actualMP)).To(Succeed())

	// Changing spec.clusterName on update must be rejected by the CEL immutability rule.
	actualMP.Spec.ClusterName = "cluster2"
	err := env.Update(ctx, actualMP)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("field is immutable"))

	// An update that leaves spec.clusterName unchanged must still be allowed.
	g.Expect(env.Get(ctx, client.ObjectKey{Namespace: mp.Namespace, Name: mp.Name}, actualMP)).To(Succeed())
	actualMP.Spec.Template.Spec.Version = "v1.20.0"
	g.Expect(env.Update(ctx, actualMP)).To(Succeed())
}
