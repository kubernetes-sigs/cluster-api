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
	"github.com/blang/semver"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/utils/pointer"
)

var _ = Describe("When upgrading a workload cluster using ClusterClass with RuntimeSDK [PR-Informing] [ClusterClass]", func() {
	clusterUpgradeWithRuntimeSDKSpec(ctx, func() clusterUpgradeWithRuntimeSDKSpecInput {
		// "upgrades" is the same as the "topology" flavor but with an additional MachinePool.
		flavor := pointer.String("upgrades-runtimesdk")
		// For KubernetesVersionUpgradeFrom < v1.24 we have to use upgrades-cgroupfs flavor.
		// This is because kind and CAPD only support:
		// * cgroupDriver cgroupfs for Kubernetes < v1.24
		// * cgroupDriver systemd for Kubernetes >= v1.24.
		// Notes:
		// * We always use a ClusterClass-based cluster-template for the upgrade test
		// * The ClusterClass will automatically adjust the cgroupDriver for KCP and MDs.
		// * We have to handle the MachinePool ourselves
		// * The upgrades-cgroupfs flavor uses an MP which is pinned to cgroupfs
		// * During the upgrade UpgradeMachinePoolAndWait automatically drops the cgroupfs pinning
		//   when the target version is >= v1.24.
		// TODO: We can remove this after the v1.25 release as we then only test the v1.24=>v1.25 upgrade.
		version, err := semver.ParseTolerant(e2eConfig.GetVariable(KubernetesVersionUpgradeFrom))
		Expect(err).ToNot(HaveOccurred(), "Invalid argument, KUBERNETES_VERSION_UPGRADE_FROM is not a valid version")
		if version.LT(semver.MustParse("1.24.0")) {
			// "upgrades-cgroupfs" is the same as the "topology" flavor but with an additional MachinePool
			// with pinned cgroupDriver to cgroupfs.
			flavor = pointer.String("upgrades-runtimesdk-cgroupfs")
		}

		return clusterUpgradeWithRuntimeSDKSpecInput{
			E2EConfig:             e2eConfig,
			ClusterctlConfigPath:  clusterctlConfigPath,
			BootstrapClusterProxy: bootstrapClusterProxy,
			ArtifactFolder:        artifactFolder,
			SkipCleanup:           skipCleanup,
			Flavor:                flavor,
		}
	})
})
