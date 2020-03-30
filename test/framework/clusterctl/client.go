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
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	clusterctlclient "sigs.k8s.io/cluster-api/cmd/clusterctl/client"
	clusterctllog "sigs.k8s.io/cluster-api/cmd/clusterctl/log"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl/logger"
)

// Provide E2E friendly wrappers for the clusterctl client library.

// InitInput is the input for Init.
type InitInput struct {
	LogPath                 string
	ClusterctlConfigPath    string
	KubeconfigPath          string
	CoreProvider            string
	BootstrapProviders      []string
	ControlPlaneProviders   []string
	InfrastructureProviders []string
}

// Init calls clusterctl init with the list of providers defined in the local repository
func Init(ctx context.Context, input InitInput) {
	By(fmt.Sprintf("clusterctl init --core %s --bootstrap %s --control-plane %s --infrastructure %s",
		input.CoreProvider,
		strings.Join(input.BootstrapProviders, ", "),
		strings.Join(input.ControlPlaneProviders, ", "),
		strings.Join(input.InfrastructureProviders, ", "),
	))

	initOpt := clusterctlclient.InitOptions{
		Kubeconfig:              input.KubeconfigPath,
		CoreProvider:            input.CoreProvider,
		BootstrapProviders:      input.BootstrapProviders,
		ControlPlaneProviders:   input.ControlPlaneProviders,
		InfrastructureProviders: input.InfrastructureProviders,
		LogUsageInstructions:    true,
	}

	clusterctlClient, log := getClusterctlClientWithLogger(input.ClusterctlConfigPath, "clusterctl-init.log", input.LogPath)
	defer log.Close()

	_, err := clusterctlClient.Init(initOpt)
	Expect(err).ToNot(HaveOccurred(), "failed to run clusterctl init")
}

// ConfigClusterInput is the input for ConfigCluster.
type ConfigClusterInput struct {
	LogPath                  string
	ClusterctlConfigPath     string
	KubeconfigPath           string
	InfrastructureProvider   string
	ClusterName              string
	KubernetesVersion        string
	ControlPlaneMachineCount int
	WorkerMachineCount       int
}

// ConfigCluster gets a workload cluster based on a template.
func ConfigCluster(ctx context.Context, input ConfigClusterInput) []byte {
	By(fmt.Sprintf("clusterctl config cluster %s --infrastructure %s --kubernetes-version %s --control-plane-machine-count %d --worker-machine-count %s",
		input.ClusterName,
		input.InfrastructureProvider,
		input.KubernetesVersion,
		input.ControlPlaneMachineCount,
		input.WorkerMachineCount,
	))

	templateOptions := clusterctlclient.GetClusterTemplateOptions{
		Kubeconfig: input.KubeconfigPath,
		ProviderRepositorySource: &clusterctlclient.ProviderRepositorySourceOptions{
			InfrastructureProvider: input.InfrastructureProvider,
			Flavor:                 "",
		},
		ClusterName:              input.ClusterName,
		KubernetesVersion:        input.KubernetesVersion,
		ControlPlaneMachineCount: input.ControlPlaneMachineCount,
		WorkerMachineCount:       input.WorkerMachineCount,
	}

	clusterctlClient, log := getClusterctlClientWithLogger(input.ClusterctlConfigPath, "clusterctl-config-cluster.log", input.LogPath)
	defer log.Close()

	template, err := clusterctlClient.GetClusterTemplate(templateOptions)
	Expect(err).ToNot(HaveOccurred(), "Failed to run clusterctl config cluster")

	yaml, err := template.Yaml()
	Expect(err).ToNot(HaveOccurred(), "Failed to generate yaml for the workload cluster template")

	log.WriteString(string(yaml))

	return yaml
}

// MoveInput is the input for ClusterctlMove.
type MoveInput struct {
	LogPath              string
	ClusterctlConfigPath string
	FromKubeconfigPath   string
	ToKubeconfigPath     string
	Namespace            string
}

// ClusterctlMove moves a workloads clusters.
func ClusterctlMove(ctx context.Context, input MoveInput) {
	By("Moving workload clusters")

	clusterctlClient, log := getClusterctlClientWithLogger(input.ClusterctlConfigPath, "clusterctl-move.log", input.LogPath)
	defer log.Close()

	options := clusterctlclient.MoveOptions{
		FromKubeconfig: input.FromKubeconfigPath,
		ToKubeconfig:   input.ToKubeconfigPath,
		Namespace:      input.Namespace,
	}

	Expect(clusterctlClient.Move(options)).To(Succeed(), "Failed to run clusterctl move")
}

func getClusterctlClientWithLogger(configPath, logName, logPath string) (clusterctlclient.Client, *logger.LogFile) {
	log := logger.CreateLogFile(logger.LogFileMeta{
		LogPath: logPath,
		Name:    logName,
	})
	clusterctllog.SetLogger(log.Logger())

	c, err := clusterctlclient.New(configPath)
	Expect(err).ToNot(HaveOccurred(), "Failed to create the clusterctl client library")
	return c, log
}
