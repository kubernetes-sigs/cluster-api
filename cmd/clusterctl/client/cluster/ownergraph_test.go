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

package cluster

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
)

func TestOwnerGraphIncludesProviderNamespaceSecrets(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	objs := []client.Object{test.NewSecret("infra1-system", "credentials")}
	objs = append(objs, test.NewFakeCluster("ns1", "cluster1").Objs()...)

	moveGraph := getObjectGraphWithObjs(objs)
	g.Expect(moveGraph.getDiscoveryTypes(ctx)).To(Succeed())
	g.Expect(moveGraph.Discovery(ctx, "ns1")).To(Succeed())
	inMove := false
	for _, n := range moveGraph.uidToNode {
		if n.identity.Namespace == "infra1-system" && n.identity.Name == "credentials" {
			inMove = true
		}
	}
	g.Expect(inMove).To(BeTrue())

	ownerGraphSource := getObjectGraphWithObjs(objs)
	g.Expect(ownerGraphSource.getDiscoveryTypes(ctx)).To(Succeed())
	ownerGraph, err := discoverOwnerGraph(ctx, "ns1", ownerGraphSource, nil)
	g.Expect(err).ToNot(HaveOccurred())
	inOwnerGraph := false
	for _, n := range ownerGraph {
		if n.Object.Namespace == "infra1-system" && n.Object.Name == "credentials" {
			inOwnerGraph = true
		}
	}
	g.Expect(inOwnerGraph).To(Equal(inMove))
}
