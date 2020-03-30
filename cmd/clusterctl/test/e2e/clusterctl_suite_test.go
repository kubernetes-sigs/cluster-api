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
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	capde2e "sigs.k8s.io/cluster-api/test/infrastructure/docker/e2e"
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	junitPath := filepath.Join(artifactFolder, fmt.Sprintf("junit.e2e.xml"))
	junitReporter := reporters.NewJUnitReporter(junitPath)
	RunSpecsWithDefaultAndCustomReporters(t, "clusterctl-e2e", []Reporter{junitReporter})
}

var (
	ctx context.Context

	// configBasePath to be used as a base for relative paths in the e2e config file.
	configBasePath string

	// configPath for the e2e test.
	configPath string

	// artifactFolder to be used for storing e2e test artifacts.
	artifactFolder string

	// skipCleanup prevent cleanup of test resources e.g. for debug purposes.
	skipCleanup bool

	// e2eConfig for this test
	e2eConfig *clusterctl.E2EConfig

	// clusterctlConfigPath to be used for this test
	clusterctlConfigPath string

	scheme *runtime.Scheme

	// managementCluster defines the management cluster used for this test
	managementCluster framework.ManagementCluster

	managementClusterKubeConfigPath string
)

func init() {
	wd, err := os.Getwd()
	if err != nil {
		panic("failed to get configBasePath")
	}

	flag.StringVar(&configPath, "e2e.config", "dev-e2e.conf", "path to the e2e config file")
	flag.StringVar(&configBasePath, "e2e.config-base-path", wd, "base path to be used as a base for relative paths in the e2e config file")
	flag.StringVar(&artifactFolder, "e2e.artifacts-folder", "_artifacts", "path for the e2e test artifacts")
	flag.BoolVar(&skipCleanup, "e2e.skip-resource-cleanup", false, "if true, the resource cleanup after tests will be skipped")
}

var _ = BeforeSuite(func() {
	ctx = context.Background()

	configPath = "/Users/fpandini/go/src/sigs.k8s.io/cluster-api/cmd/clusterctl/test/config/aws-dev.conf"
	configBasePath = "/Users/fpandini/go/src/sigs.k8s.io/cluster-api/"
	artifactFolder = "/Users/fpandini/go/src/sigs.k8s.io/cluster-api/_artifacts"

	Expect(configPath).To(BeAnExistingFile(), "invalid argument. e2e.config should be an existing file")
	Expect(configBasePath).To(BeADirectory(), "invalid argument. e2e.config-base-path should be an existing folder")
	Expect(artifactFolder).To(BeADirectory(), "invalid argument. e2e.artifacts-folder should be an existing folder")

	By("Initializing a runtime.Scheme with all the GVK relevant for this test")

	scheme = runtime.NewScheme()
	framework.TryAddDefaultSchemes(scheme)

	By(fmt.Sprintf("Loading the e2e test configuration from %s", configPath))

	e2eConfig = clusterctl.LoadE2EConfig(ctx, clusterctl.LoadE2EConfigInput{
		BasePath:   configBasePath,
		ConfigPath: configPath,
	})
	Expect(e2eConfig).ToNot(BeNil(), "Failed to load E2E config")

	By(fmt.Sprintf("Creating a clusterctl local repository into %s", artifactFolder))

	clusterctlConfigPath = clusterctl.CreateRepository(ctx, clusterctl.CreateRepositoryInput{
		E2EConfig:     e2eConfig,
		ArtifactsPath: artifactFolder,
	})
	Expect(clusterctlConfigPath).To(BeAnExistingFile(), "Failed to get a clusterctl config file")

	By("Initializing a management cluster")
	managementCluster, managementClusterKubeConfigPath = clusterctl.InitManagementCluster(
		ctx, &clusterctl.InitManagementClusterInput{
			E2EConfig:            e2eConfig,
			ClusterctlConfigPath: clusterctlConfigPath,
			Scheme:               scheme,
			NewManagementClusterFn: func(name string, scheme *runtime.Scheme) (cluster framework.ManagementCluster, kubeConfigPath string, err error) {
				cluster, err = capde2e.NewClusterForCAPD(ctx, name, scheme)
				if err != nil {
					return nil, "", err
				}
				//TODO(fabriziopandini): this is hacky way to get the kubeconfig path. As soon as v1alpha4 opens, we should add GetKubeConfigPath method to the ManagementCluster interface and get rid of this
				kindCluster := cluster.(*capde2e.CAPDCluster)
				kubeConfigPath = kindCluster.KubeconfigPath
				return cluster, kubeConfigPath, err
			},
			LogsFolder: artifactFolder,
		},
	)
	Expect(managementCluster).ToNot(BeNil(), "Failed to create a management cluster")
})

var _ = AfterSuite(func() {
	// TODO(fabriziopandini): dump controller logs

	if skipCleanup {
		return
	}

	// Tears down the management cluster
	By("Tearing down the management cluster")

	if managementCluster != nil {
		managementCluster.Teardown(ctx)
	}
})
