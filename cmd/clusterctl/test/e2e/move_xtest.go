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

/*
import (
	"path/filepath"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/test/framework"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/test/framework/cluster/kind"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/test/framework/clusterctl"

	clusterctlconfig "sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
)

var _ = Describe("move a workload cluster (suite-2)", func() {
	var (
		fromCluster framework.Cluster
		toCluster   framework.Cluster
	)

	BeforeEach(func() {
		// Create a kind cluster configured for running the docker infrastructure provider and with all the images defined in the E2E test configuration.
		fromCluster = kind.NewCluster(ctx, kind.CreateManagementClusterInput{Config: e2eConfig})
		Expect(fromCluster).ToNot(BeNil())

		toCluster = kind.NewCluster(ctx, kind.CreateManagementClusterInput{Config: e2eConfig})
		Expect(toCluster).ToNot(BeNil())
	})

	XIt("should move (suite-2-spec-1)", func() {
		logPath := filepath.Join(artifactPath, "logs", "suite-2-spec-1")

		clusterctl.Init(ctx, clusterctl.InitInput{
			LogPath: logPath,
			// pass reference to the management cluster hosting this test
			ManagementCluster: fromCluster,
			// setup the desired list of providers for a single-tenant management cluster
			CoreProvider:            clusterctlconfig.ClusterAPIProviderName,
			BootstrapProviders:      []string{clusterctlconfig.KubeadmBootstrapProviderName},
			ControlPlaneProviders:   []string{clusterctlconfig.KubeadmControlPlaneProviderName},
			InfrastructureProviders: []string{e2eConfig.InfraProvider()},
		})

		workloadCluster := clusterctl.ConfigCluster(ctx, clusterctl.ConfigClusterInput{
			LogPath: logPath,
			// pass reference to the management cluster hosting this test
			ManagementCluster: fromCluster,
			// define the cluster template source
			InfrastructureProvider: e2eConfig.InfraProvider(),
			// define template variables
			ClusterName:              "test-cluster",
			KubernetesVersion:        "v1.17.0",
			ControlPlaneMachineCount: 1,
			WorkerMachineCount:       1,
		})
		Expect(workloadCluster).ToNot(BeNil())
		Expect(workloadCluster.Apply(ctx)).To(Succeed())

		clusterctl.Init(ctx, clusterctl.InitInput{
			LogPath: logPath,
			// pass reference to the management cluster hosting this test
			ManagementCluster: toCluster,
			// setup the desired list of providers for a single-tenant management cluster
			CoreProvider:            clusterctlconfig.ClusterAPIProviderName,
			BootstrapProviders:      []string{clusterctlconfig.KubeadmBootstrapProviderName},
			ControlPlaneProviders:   []string{clusterctlconfig.KubeadmControlPlaneProviderName},
			InfrastructureProviders: []string{e2eConfig.InfraProvider()},
		})
		Expect(err).ToNot(HaveOccurred())

		clusterctl.ClusterctlMove(ctx, clusterctl.MoveInput{
			LogPath: logPath,
			// pass reference to the objects to be moved
			FromCluster: fromCluster,
			ToCluster:   toCluster,
			Namespace:   "default",
		})

		Expect(workloadCluster.HasMovedTo(ctx, toCluster)).To(Succeed())

		Expect(workloadCluster.Delete(ctx)).To(Succeed())
	})

	AfterEach(func() {
		if skipResourceCleanup {
			return
		}

		// Tears down the kind cluster
		fromCluster.Teardown(ctx)
		toCluster.Teardown(ctx)
	})
})
*/
