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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/ptr"
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
				{
					// All Machines owned by MachineDeployments or MachineSets, MachineDeployments and MachineSets should be deleted in this phase.
					ObjectFilter: func(objectReference corev1.ObjectReference, objectOwnerReferences []metav1.OwnerReference) bool {
						if objectReference.Kind == "Machine" && hasOwner(objectOwnerReferences, "MachineDeployment", "MachineSet") {
							return true
						}
						return objectReference.Kind == "MachineDeployment" || objectReference.Kind == "MachineSet"
					},
					// Of the above, only Machines should be considered to be blocking to test foreground deletion.
					BlockingObjectFilter: func(objectReference corev1.ObjectReference, _ []metav1.OwnerReference) bool {
						return objectReference.Kind == "Machine"
					},
				},
				{
					// All Machines owned by KubeadmControlPlane and KubeadmControlPlane should be deleted in this phase.
					ObjectFilter: func(objectReference corev1.ObjectReference, objectOwnerReferences []metav1.OwnerReference) bool {
						if objectReference.Kind == "Machine" && hasOwner(objectOwnerReferences, "KubeadmControlPlane") {
							return true
						}
						return objectReference.Kind == "KubeadmControlPlane"
					},
					// Of the above, only Machines should be considered to be blocking to test foreground deletion.
					BlockingObjectFilter: func(objectReference corev1.ObjectReference, _ []metav1.OwnerReference) bool {
						return objectReference.Kind == "Machine"
					},
				},
				{
					// The DockerCluster should be deleted in this phase.
					ObjectFilter: func(objectReference corev1.ObjectReference, _ []metav1.OwnerReference) bool {
						return objectReference.Kind == "DockerCluster"
					},
					// Of the above, the DockerCluster should be considered to be blocking.
					BlockingObjectFilter: func(objectReference corev1.ObjectReference, _ []metav1.OwnerReference) bool {
						return objectReference.Kind == "DockerCluster"
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
