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

package clusterctl

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/runtime"
	clusterctlconfig "sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/discovery"
)

// Provides utilities for setting up a management cluster using clusterctl.

// InitManagementClusterInput is the information required to initialize a new
// management cluster for e2e testing.
type InitManagementClusterInput struct {
	// E2EConfig defining the configuration for the E2E test.
	E2EConfig *E2EConfig

	// ClusterctlConfigPath is the path to a clusterctl config file that points to repositories to be used during "clusterctl init".
	ClusterctlConfigPath string

	// LogsFolder defines a folder where to store clusterctl logs.
	LogsFolder string

	// Scheme is used to initialize the scheme for the management cluster client.
	Scheme *runtime.Scheme

	// NewManagementClusterFn should return a new management cluster.
	NewManagementClusterFn func(name string, scheme *runtime.Scheme) (cluster framework.ManagementCluster, kubeConfigPath string, err error)
}

// InitManagementCluster returns a new cluster initialized and the path to the kubeConfig file to be used to access it.
func InitManagementCluster(ctx context.Context, input *InitManagementClusterInput) (framework.ManagementCluster, string) {
	// validate parameters and apply defaults

	Expect(input.E2EConfig).ToNot(BeNil(), "Invalid argument. input.E2EConfig can't be nil when calling InitManagementCluster")
	Expect(input.NewManagementClusterFn).ToNot(BeNil(), "Invalid argument. input.NewManagementClusterFn can't be nil when calling InitManagementCluster")
	Expect(input.ClusterctlConfigPath).To(BeAnExistingFile(), "Invalid argument. input.ClusterctlConfigPath must be an existing file")

	By(fmt.Sprintf("Creating the management cluster with name %s", input.E2EConfig.ManagementClusterName))

	managementCluster, managementClusterKubeConfigPath, err := input.NewManagementClusterFn(input.E2EConfig.ManagementClusterName, input.Scheme)
	Expect(err).ToNot(HaveOccurred(), "Failed to create the management cluster with name %s", input.E2EConfig.ManagementClusterName)
	Expect(managementCluster).ToNot(BeNil(), "The management cluster with name %s should not be nil", input.E2EConfig.ManagementClusterName)

	// Load the images into the cluster.
	if imageLoader, ok := managementCluster.(framework.ImageLoader); ok {
		By("Loading images into the management cluster")

		for _, image := range input.E2EConfig.Images {
			err := imageLoader.LoadImage(ctx, image.Name)
			switch image.LoadBehavior {
			case framework.MustLoadImage:
				Expect(err).ToNot(HaveOccurred(), "Failed to load image %s into the kind cluster", image.Name)
			case framework.TryLoadImage:
				if err != nil {
					fmt.Fprintf(GinkgoWriter, "[WARNING] Unable to load image %s into the kind cluster: %v \n", image.Name, err)
				}
			}
		}
	}

	By("Running clusterctl init")

	Init(ctx, InitInput{
		// pass reference to the management cluster hosting this test
		KubeconfigPath: managementClusterKubeConfigPath,
		// pass the clusterctl config file that points to the local provider repository created for this test,
		ClusterctlConfigPath: input.ClusterctlConfigPath,
		// setup the desired list of providers for a single-tenant management cluster
		CoreProvider:            clusterctlconfig.ClusterAPIProviderName,
		BootstrapProviders:      []string{clusterctlconfig.KubeadmBootstrapProviderName},
		ControlPlaneProviders:   []string{clusterctlconfig.KubeadmControlPlaneProviderName},
		InfrastructureProviders: []string{input.E2EConfig.InfraProvider()},
		// setup output path for clusterctl logs
		LogPath: input.LogsFolder,
	})

	By("Waiting for providers controllers to be running")

	client, err := managementCluster.GetClient()
	Expect(err).NotTo(HaveOccurred())
	controllersDeployments := discovery.GetControllerDeployments(ctx, discovery.GetControllerDeploymentsInput{
		Lister: client,
	})
	Expect(controllersDeployments).ToNot(BeNil())
	for _, deployment := range controllersDeployments {
		framework.WaitForDeploymentsAvailable(ctx, framework.WaitForDeploymentsAvailableInput{
			Getter:     client,
			Deployment: deployment,
		}, input.E2EConfig.IntervalsOrDefault("init-management-cluster/wait-controllers", "2m", "10s")...)
	}

	return managementCluster, managementClusterKubeConfigPath
}
