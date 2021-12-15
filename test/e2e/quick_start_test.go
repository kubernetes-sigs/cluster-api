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
	. "github.com/onsi/ginkgo"
	"k8s.io/utils/pointer"
)

var _ = Describe("When following the Cluster API quick-start [PR-Blocking]", func() {
	QuickStartSpec(ctx, func() QuickStartSpecInput {
		return QuickStartSpecInput{
			E2EConfig:             e2eConfig,
			ClusterctlConfigPath:  clusterctlConfigPath,
			BootstrapClusterProxy: bootstrapClusterProxy,
			ArtifactFolder:        artifactFolder,
			SkipCleanup:           skipCleanup,
		}
	})
})

var _ = Describe("When following the Cluster API quick-start with ClusterClass [PR-Informing]", func() {
	QuickStartSpec(ctx, func() QuickStartSpecInput {
		return QuickStartSpecInput{
			E2EConfig:             e2eConfig,
			ClusterctlConfigPath:  clusterctlConfigPath,
			BootstrapClusterProxy: bootstrapClusterProxy,
			ArtifactFolder:        artifactFolder,
			SkipCleanup:           skipCleanup,
			Flavor:                pointer.String("topology"),
		}
	})
})

// NOTE: This test requires an IPv6 management cluster (can be configured via IP_FAMILY=IPv6).
var _ = Describe("When following the Cluster API quick-start with IPv6 [IPv6] [PR-Informing]", func() {
	QuickStartSpec(ctx, func() QuickStartSpecInput {
		return QuickStartSpecInput{
			E2EConfig:             e2eConfig,
			ClusterctlConfigPath:  clusterctlConfigPath,
			BootstrapClusterProxy: bootstrapClusterProxy,
			ArtifactFolder:        artifactFolder,
			SkipCleanup:           skipCleanup,
			Flavor:                pointer.String("ipv6"),
		}
	})
})

var _ = Describe("When following the Cluster API quick-start with Ignition", func() {
	QuickStartSpec(ctx, func() QuickStartSpecInput {
		return QuickStartSpecInput{
			E2EConfig:             e2eConfig,
			ClusterctlConfigPath:  clusterctlConfigPath,
			BootstrapClusterProxy: bootstrapClusterProxy,
			ArtifactFolder:        artifactFolder,
			SkipCleanup:           skipCleanup,
			Flavor:                pointer.String("ignition"),
		}
	})
})
