//go:build e2e
// +build e2e

/*
Copyright 2021 The Kubernetes Authors.

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
	"github.com/blang/semver/v4"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"

	clusterctlcluster "sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
	"sigs.k8s.io/cluster-api/test/framework"
)

var _ = Describe("When upgrading a workload cluster using ClusterClass with RuntimeSDK [ClusterClass]", func() {
	ClusterUpgradeWithRuntimeSDKSpec(ctx, func() ClusterUpgradeWithRuntimeSDKSpecInput {
		version, err := semver.ParseTolerant(e2eConfig.GetVariable(KubernetesVersionUpgradeFrom))
		Expect(err).ToNot(HaveOccurred(), "Invalid argument, KUBERNETES_VERSION_UPGRADE_FROM is not a valid version")
		if version.LT(semver.MustParse("1.24.0")) {
			Fail("This test only supports upgrades from Kubernetes >= v1.24.0")
		}

		return ClusterUpgradeWithRuntimeSDKSpecInput{
			E2EConfig:              e2eConfig,
			ClusterctlConfigPath:   clusterctlConfigPath,
			BootstrapClusterProxy:  bootstrapClusterProxy,
			ArtifactFolder:         artifactFolder,
			SkipCleanup:            skipCleanup,
			InfrastructureProvider: ptr.To("docker"),
			PostUpgrade: func(proxy framework.ClusterProxy, namespace, clusterName string) {
				// This check ensures that the resourceVersions are stable, i.e. it verifies there are no
				// continuous reconciles when everything should be stable.
				framework.ValidateResourceVersionStable(ctx, proxy, namespace, clusterctlcluster.FilterClusterObjectsWithNameFilter(clusterName))
			},
			// "upgrades" is the same as the "topology" flavor but with an additional MachinePool.
			Flavor: ptr.To("upgrades-runtimesdk"),
			// The runtime extension gets deployed to the test-extension-system namespace and is exposed
			// by the test-extension-webhook-service.
			// The below values are used when creating the cluster-wide ExtensionConfig to refer
			// the actual service.
			ExtensionServiceNamespace: "test-extension-system",
			ExtensionServiceName:      "test-extension-webhook-service",
		}
	})
})
