//go:build e2e
// +build e2e

package e2e

import (
	. "github.com/onsi/ginkgo/v2"
	"k8s.io/utils/pointer"
)

var _ = Describe("When following the Cluster API quick-start with ClusterClass check owner references are correctly reconciled [ClusterClass]", func() {
	OwnerReferencesSpec(ctx, func() OwnerReferencesInputSpec {
		return OwnerReferencesInputSpec{
			E2EConfig:             e2eConfig,
			ClusterctlConfigPath:  clusterctlConfigPath,
			BootstrapClusterProxy: bootstrapClusterProxy,
			ArtifactFolder:        artifactFolder,
			SkipCleanup:           skipCleanup,
			Flavor:                pointer.String("topology"),
		}
	})
})

var _ = Describe("When following the Cluster API quick-start check owner references are correctly reconciled", func() {
	OwnerReferencesSpec(ctx, func() OwnerReferencesInputSpec {
		return OwnerReferencesInputSpec{
			E2EConfig:             e2eConfig,
			ClusterctlConfigPath:  clusterctlConfigPath,
			BootstrapClusterProxy: bootstrapClusterProxy,
			ArtifactFolder:        artifactFolder,
			SkipCleanup:           skipCleanup,
		}
	})
})
