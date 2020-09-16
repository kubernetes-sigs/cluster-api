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

package conformance

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cluster-api/test/e2e/internal/setup"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/bootstrap"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/test/framework/ginkgoextensions"
	"sigs.k8s.io/yaml"
)

// Test suite flags
var (
	// configPath is the path to the e2e config file.
	configPath string

	// useExistingCluster instructs the test to use the current cluster instead of creating a new one (default discovery rules apply).
	useExistingCluster bool

	// artifactFolder is the folder to store e2e test artifacts.
	artifactFolder string

	// skipCleanup prevents cleanup of test resources e.g. for debug purposes.
	skipCleanup bool
)

// Test suite global vars
var (
	// e2eConfig to be used for this test, read from configPath.
	e2eConfig *clusterctl.E2EConfig

	// clusterctlConfigPath to be used for this test, created by generating a clusterctl local repository
	// with the providers specified in the configPath.
	clusterctlConfigPath string

	// bootstrapClusterProvider manages provisioning of the the bootstrap cluster to be used for the e2e tests.
	// Please note that provisioning will be skipped if e2e.use-existing-cluster is provided.
	bootstrapClusterProvider bootstrap.ClusterProvider

	// bootstrapClusterProxy allows to interact with the bootstrap cluster to be used for the e2e tests.
	bootstrapClusterProxy framework.ClusterProxy
)

func init() {
	flag.StringVar(&configPath, "e2e.config", "", "path to the e2e config file")
	flag.StringVar(&artifactFolder, "e2e.artifacts-folder", "", "folder where e2e test artifact should be stored")
	flag.BoolVar(&skipCleanup, "e2e.skip-resource-cleanup", false, "if true, the resource cleanup after tests will be skipped")
	flag.BoolVar(&useExistingCluster, "e2e.use-existing-cluster", false, "if true, the test uses the current cluster instead of creating a new one (default discovery rules apply)")
}

func TestE2E(t *testing.T) {
	artifactFolder := framework.ResolveArtifactsDirectory(artifactFolder)
	RegisterFailHandler(Fail)
	RunSpecsWithDefaultAndCustomReporters(t, "capi-conformance", []Reporter{framework.CreateJUnitReporterForProw(artifactFolder)})
}

type suiteConfig struct {
	ArtifactFolder       string `json:"artifactFolder,omitempty"`
	ConfigPath           string `json:"configPath,omitempty"`
	ClusterctlConfigPath string `json:"clusterctlConfigPath,omitempty"`
	KubeconfigPath       string `json:"kubeconfigPath,omitempty"`
}

// Using a SynchronizedBeforeSuite for controlling how to create resources shared across ParallelNodes (~ginkgo threads).
// The local clusterctl repository & the bootstrap cluster are created once and shared across all the tests.
var _ = SynchronizedBeforeSuite(func() []byte {
	// Before all ParallelNodes.

	Expect(configPath).To(BeAnExistingFile(), "Invalid test suite argument. e2e.config should be an existing file.")
	artifactFolder := framework.ResolveArtifactsDirectory(artifactFolder)
	log := ginkgoextensions.Log.WithValues("config-path", configPath, "artifacts-directory", artifactFolder)
	Expect(os.MkdirAll(artifactFolder, 0755)).To(Succeed(), "Invalid test suite argument. Can't create e2e.artifacts-folder %q", artifactFolder)

	By("Initializing a runtime.Scheme with all the GVK relevant for this test")
	scheme := setup.InitScheme()

	log.Info("Loading the e2e test configuration")
	e2eConfig = setup.LoadE2EConfig(configPath)

	log.Info("Creating a clusterctl local repository in artifacts directory")

	setup.CreateCIArtifactsTemplate(artifactFolder, "..", e2eConfig)

	clusterctlConfigPath = setup.CreateClusterctlLocalRepository(e2eConfig, filepath.Join(artifactFolder, "repository"))

	log.Info("Setting up the bootstrap cluster")
	// e2eConfig, scheme, useExistingCluster
	bootstrapClusterProvider, bootstrapClusterProxy = setup.CreateBootstrapCluster(
		setup.CreateBootstrapClusterInput{
			E2EConfig:          e2eConfig,
			Scheme:             scheme,
			UseExistingCluster: useExistingCluster,
		},
	)

	log.Info("Initializing the bootstrap cluster")
	// setup.InitBootstrapCluster(bootstrapClusterProxy, e2eConfig, clusterctlConfigPath, artifactFolder)
	setup.InitBootstrapCluster(
		setup.InitBootstrapClusterInput{
			BootstrapClusterProxy: bootstrapClusterProxy,
			E2EConfig:             e2eConfig,
			ClusterctlConfig:      clusterctlConfigPath,
			ArtifactsDirectory:    artifactFolder,
		},
	)

	conf := suiteConfig{
		ArtifactFolder:       artifactFolder,
		ConfigPath:           configPath,
		ClusterctlConfigPath: clusterctlConfigPath,
		KubeconfigPath:       bootstrapClusterProxy.GetKubeconfigPath(),
	}

	data, err := yaml.Marshal(conf)
	Expect(err).NotTo(HaveOccurred())
	return data

}, func(data []byte) {
	// Before each ParallelNode.

	conf := &suiteConfig{}
	err := yaml.UnmarshalStrict(data, conf)
	Expect(err).NotTo(HaveOccurred())

	artifactFolder = conf.ArtifactFolder
	configPath = conf.ConfigPath
	clusterctlConfigPath = conf.ClusterctlConfigPath
	kubeconfigPath := conf.KubeconfigPath

	e2eConfig = setup.LoadE2EConfig(configPath)
	bootstrapClusterProxy = framework.NewClusterProxy("bootstrap", kubeconfigPath, initScheme())
})

// Using a SynchronizedAfterSuite for controlling how to delete resources shared across ParallelNodes (~ginkgo threads).
// The bootstrap cluster is shared across all the tests, so it should be deleted only after all ParallelNodes completes.
// The local clusterctl repository is preserved like everything else created into the artifact folder.
var _ = SynchronizedAfterSuite(func() {
	// After each ParallelNode.
}, func() {
	// After all ParallelNodes.
	log := ginkgoextensions.Log.WithValues("config-path", configPath, "artifacts-directory", artifactFolder)
	log.Info("Tearing down the management cluster")
	if !skipCleanup {
		setup.TearDown(bootstrapClusterProvider, bootstrapClusterProxy)
	}
})

func initScheme() *runtime.Scheme {
	sc := runtime.NewScheme()
	framework.TryAddDefaultSchemes(sc)
	return sc
}
