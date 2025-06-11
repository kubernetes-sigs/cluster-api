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

var _ = Describe("When in-place updating a workload cluster using ClusterClass", Label("ClusterClass"), func() {
	ClusterInPlaceUpdateSpec(ctx, func() ClusterInPlaceUpdateSpecInput {
		return ClusterInPlaceUpdateSpecInput{
			E2EConfig:             e2eConfig,
			ClusterctlConfigPath:  clusterctlConfigPath,
			BootstrapClusterProxy: bootstrapClusterProxy,
			ArtifactFolder:        artifactFolder,
			SkipCleanup:           skipCleanup,
			Flavor:                ptr.To("topology-in-place"),
			// The runtime extension gets deployed to the test-extension-system namespace and is exposed
			// by the test-extension-webhook-service.
			// The below values are used when creating the cluster-wide ExtensionConfig to refer
			// the actual service.
			ExtensionServiceNamespace: "test-extension-system",
			ExtensionServiceName:      "test-extension-webhook-service",
		}
	})
})
