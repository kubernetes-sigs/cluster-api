//go:build e2e
// +build e2e

/*
Copyright 2020 The Kubernetes Authors.

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
	"k8s.io/utils/ptr"
)

var _ = Describe("When testing the machinery for scale testing using in-memory provider", func() {
	// Note: This test does not support MachinePools.
	ScaleSpec(ctx, func() ScaleSpecInput {
		return ScaleSpecInput{
			E2EConfig:                e2eConfig,
			ClusterctlConfigPath:     clusterctlConfigPath,
			InfrastructureProvider:   ptr.To("in-memory"),
			BootstrapClusterProxy:    bootstrapClusterProxy,
			ArtifactFolder:           artifactFolder,
			ClusterCount:             ptr.To[int64](10),
			Concurrency:              ptr.To[int64](5),
			Flavor:                   ptr.To(""),
			ControlPlaneMachineCount: ptr.To[int64](1),
			MachineDeploymentCount:   ptr.To[int64](1),
			WorkerMachineCount:       ptr.To[int64](3),
			SkipCleanup:              skipCleanup,
		}
	})
})
