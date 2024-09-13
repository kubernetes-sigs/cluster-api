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
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/ptr"
)

var _ = Describe("When following the Cluster API quick-start with ClusterClass [PR-Blocking] [ClusterClass]", func() {
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
				// Note: only setting the finalizer on Machines results in properly testing foreground deletion.
				{
					Kinds:              sets.New[string]("MachineDeployment", "MachineSet"),
					KindsWithFinalizer: sets.New[string]("Machine"),
				},
				// The second phase deletes all control plane related machines.
				// Note: only setting the finalizer on Machines results in properly testing foreground deletion.
				{
					Kinds:              sets.New[string]("KubeadmControlPlane"),
					KindsWithFinalizer: sets.New[string]("Machine"),
				},
				// The third phase deletes the infrastructure cluster.
				{
					Kinds:              sets.New[string](),
					KindsWithFinalizer: sets.New[string]("DockerCluster"),
				},
			},
		}
	})
})
