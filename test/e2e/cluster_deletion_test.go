//go:build e2e
// +build e2e

/*
Copyright 2024 The Kubernetes Authors.

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

package e2e

import (
	. "github.com/onsi/ginkgo/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/ptr"

	clusterctlcluster "sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
)

var _ = Describe("When performing cluster deletion with ClusterClass [ClusterClass]", func() {
	ClusterDeletionSpec(ctx, func() ClusterDeletionSpecInput {
		return ClusterDeletionSpecInput{
			E2EConfig:                e2eConfig,
			ClusterctlConfigPath:     clusterctlConfigPath,
			BootstrapClusterProxy:    bootstrapClusterProxy,
			ArtifactFolder:           artifactFolder,
			SkipCleanup:              skipCleanup,
			ControlPlaneMachineCount: ptr.To[int64](3),
			Flavor:                   ptr.To("topology"),
			InfrastructureProvider:   ptr.To("docker"),

			ClusterDeletionPhases: []ClusterDeletionPhase{
				// The first phase deletes all worker related machines.
				{
					// All Machines owned by MachineDeployments or MachineSets and themselves should be deleted in this phase.
					DeletionSelector: func(node clusterctlcluster.OwnerGraphNode) bool {
						if node.Object.Kind == "Machine" && hasOwner(node.Owners, "MachineDeployment", "MachineSet") {
							return true
						}
						return sets.New[string]("MachineDeployment", "MachineSet").Has(node.Object.Kind)
					},
					// Of the above, only Machines should be considered to be blocking to test foreground deletion.
					IsBlocking: func(node clusterctlcluster.OwnerGraphNode) bool {
						return node.Object.Kind == "Machine"
					},
				},
				// The second phase deletes all control plane related machines.
				{
					// All Machines owned by KubeadmControlPlane and themselves should be deleted in this phase.
					DeletionSelector: func(node clusterctlcluster.OwnerGraphNode) bool {
						if node.Object.Kind == "Machine" && hasOwner(node.Owners, "KubeadmControlPlane") {
							return true
						}
						return node.Object.Kind == "KubeadmControlPlane"
					},
					// Of the above, only Machines should be considered to be blocking to test foreground deletion.
					IsBlocking: func(node clusterctlcluster.OwnerGraphNode) bool {
						return node.Object.Kind == "Machine"
					},
				},
				// The third phase deletes the infrastructure cluster.
				{
					// The DockerCluster should be deleted in this phase.
					DeletionSelector: func(node clusterctlcluster.OwnerGraphNode) bool {
						return node.Object.Kind == "DockerCluster"
					},
					// Of the above, the DockerCluster should be considered to be blocking.
					IsBlocking: func(node clusterctlcluster.OwnerGraphNode) bool {
						return node.Object.Kind == "DockerCluster"
					},
				},
			},
		}
	})
})

func hasOwner(ownerReferences []metav1.OwnerReference, owners ...string) bool {
	ownersSet := sets.New[string](owners...)
	for _, owner := range ownerReferences {
		if ownersSet.Has(owner.Kind) {
			return true
		}
	}
	return false
}
