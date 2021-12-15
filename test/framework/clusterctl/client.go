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
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	clusterctlclient "sigs.k8s.io/cluster-api/cmd/clusterctl/client"
	clusterctllog "sigs.k8s.io/cluster-api/cmd/clusterctl/log"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl/logger"
	"sigs.k8s.io/cluster-api/test/framework/internal/log"
)

// Provide E2E friendly wrappers for the clusterctl client library.

const (
	// DefaultFlavor for ConfigClusterInput; use it for getting the cluster-template.yaml file.
	DefaultFlavor = ""
)

const (
	// DefaultInfrastructureProvider for ConfigClusterInput; use it for using the only infrastructure provider installed in a cluster.
	DefaultInfrastructureProvider = ""
)

// InitInput is the input for Init.
type InitInput struct {
	LogFolder               string
	ClusterctlConfigPath    string
	KubeconfigPath          string
	CoreProvider            string
	BootstrapProviders      []string
	ControlPlaneProviders   []string
	InfrastructureProviders []string
}

// Init calls clusterctl init with the list of providers defined in the local repository.
func Init(ctx context.Context, input InitInput) {
	log.Logf("clusterctl init --core %s --bootstrap %s --control-plane %s --infrastructure %s",
		input.CoreProvider,
		strings.Join(input.BootstrapProviders, ","),
		strings.Join(input.ControlPlaneProviders, ","),
		strings.Join(input.InfrastructureProviders, ","),
	)

	initOpt := clusterctlclient.InitOptions{
		Kubeconfig: clusterctlclient.Kubeconfig{
			Path:    input.KubeconfigPath,
			Context: "",
		},
		CoreProvider:            input.CoreProvider,
		BootstrapProviders:      input.BootstrapProviders,
		ControlPlaneProviders:   input.ControlPlaneProviders,
		InfrastructureProviders: input.InfrastructureProviders,
		LogUsageInstructions:    true,
	}

	clusterctlClient, log := getClusterctlClientWithLogger(input.ClusterctlConfigPath, "clusterctl-init.log", input.LogFolder)
	defer log.Close()

	_, err := clusterctlClient.Init(initOpt)
	Expect(err).ToNot(HaveOccurred(), "failed to run clusterctl init")
}

// InitWithBinary uses clusterctl binary to run init with the list of providers defined in the local repository.
func InitWithBinary(_ context.Context, binary string, input InitInput) {
	log.Logf("clusterctl init --core %s --bootstrap %s --control-plane %s --infrastructure %s",
		input.CoreProvider,
		strings.Join(input.BootstrapProviders, ","),
		strings.Join(input.ControlPlaneProviders, ","),
		strings.Join(input.InfrastructureProviders, ","),
	)

	cmd := exec.Command(binary, "init", //nolint:gosec // We don't care about command injection here.
		"--core", input.CoreProvider,
		"--bootstrap", strings.Join(input.BootstrapProviders, ","),
		"--control-plane", strings.Join(input.ControlPlaneProviders, ","),
		"--infrastructure", strings.Join(input.InfrastructureProviders, ","),
		"--config", input.ClusterctlConfigPath,
		"--kubeconfig", input.KubeconfigPath,
	)

	out, err := cmd.CombinedOutput()
	_ = os.WriteFile(filepath.Join(input.LogFolder, "clusterctl-init.log"), out, 0644) //nolint:gosec // this is a log file to be shared via prow artifacts
	var stdErr string
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stdErr = string(exitErr.Stderr)
		}
	}
	Expect(err).ToNot(HaveOccurred(), "failed to run clusterctl init:\nstdout:\n%s\nstderr:\n%s", string(out), stdErr)
}

// UpgradeInput is the input for Upgrade.
type UpgradeInput struct {
	LogFolder            string
	ClusterctlConfigPath string
	KubeconfigPath       string
	Contract             string
}

// Upgrade calls clusterctl upgrade apply with the list of providers defined in the local repository.
func Upgrade(ctx context.Context, input UpgradeInput) {
	log.Logf("clusterctl upgrade apply --contract %s",
		input.Contract,
	)

	upgradeOpt := clusterctlclient.ApplyUpgradeOptions{
		Kubeconfig: clusterctlclient.Kubeconfig{
			Path:    input.KubeconfigPath,
			Context: "",
		},
		Contract: input.Contract,
	}

	clusterctlClient, log := getClusterctlClientWithLogger(input.ClusterctlConfigPath, "clusterctl-upgrade.log", input.LogFolder)
	defer log.Close()

	err := clusterctlClient.ApplyUpgrade(upgradeOpt)
	Expect(err).ToNot(HaveOccurred(), "failed to run clusterctl upgrade")
}

// DeleteInput is the input for Delete.
type DeleteInput struct {
	LogFolder            string
	ClusterctlConfigPath string
	KubeconfigPath       string
}

// Delete calls clusterctl delete --all.
func Delete(_ context.Context, input DeleteInput) {
	log.Logf("clusterctl delete --all")

	deleteOpts := clusterctlclient.DeleteOptions{
		Kubeconfig: clusterctlclient.Kubeconfig{
			Path:    input.KubeconfigPath,
			Context: "",
		},
		DeleteAll: true,
	}

	clusterctlClient, log := getClusterctlClientWithLogger(input.ClusterctlConfigPath, "clusterctl-delete.log", input.LogFolder)
	defer log.Close()

	err := clusterctlClient.Delete(deleteOpts)
	Expect(err).ToNot(HaveOccurred(), "failed to run clusterctl upgrade")
}

// ConfigClusterInput is the input for ConfigCluster.
type ConfigClusterInput struct {
	LogFolder                string
	ClusterctlConfigPath     string
	KubeconfigPath           string
	InfrastructureProvider   string
	Namespace                string
	ClusterName              string
	KubernetesVersion        string
	ControlPlaneMachineCount *int64
	WorkerMachineCount       *int64
	Flavor                   string
}

// ConfigCluster gets a workload cluster based on a template.
func ConfigCluster(ctx context.Context, input ConfigClusterInput) []byte {
	log.Logf("clusterctl config cluster %s --infrastructure %s --kubernetes-version %s --control-plane-machine-count %d --worker-machine-count %d --flavor %s",
		input.ClusterName,
		valueOrDefault(input.InfrastructureProvider),
		input.KubernetesVersion,
		*input.ControlPlaneMachineCount,
		*input.WorkerMachineCount,
		valueOrDefault(input.Flavor),
	)

	templateOptions := clusterctlclient.GetClusterTemplateOptions{
		Kubeconfig: clusterctlclient.Kubeconfig{
			Path:    input.KubeconfigPath,
			Context: "",
		},
		ProviderRepositorySource: &clusterctlclient.ProviderRepositorySourceOptions{
			InfrastructureProvider: input.InfrastructureProvider,
			Flavor:                 input.Flavor,
		},
		ClusterName:              input.ClusterName,
		KubernetesVersion:        input.KubernetesVersion,
		ControlPlaneMachineCount: input.ControlPlaneMachineCount,
		WorkerMachineCount:       input.WorkerMachineCount,
		TargetNamespace:          input.Namespace,
	}

	clusterctlClient, log := getClusterctlClientWithLogger(input.ClusterctlConfigPath, fmt.Sprintf("%s-cluster-template.yaml", input.ClusterName), input.LogFolder)
	defer log.Close()

	template, err := clusterctlClient.GetClusterTemplate(templateOptions)
	Expect(err).ToNot(HaveOccurred(), "Failed to run clusterctl config cluster")

	yaml, err := template.Yaml()
	Expect(err).ToNot(HaveOccurred(), "Failed to generate yaml for the workload cluster template")

	_, _ = log.WriteString(string(yaml))
	return yaml
}

// ConfigClusterWithBinary uses clusterctl binary to run config cluster or generate cluster.
// NOTE: This func detects the clusterctl version and uses config cluster or generate cluster
// accordingly. We can drop the detection when we don't have to support clusterctl v0.3.x anymore.
func ConfigClusterWithBinary(_ context.Context, clusterctlBinaryPath string, input ConfigClusterInput) []byte {
	log.Logf("Detect clusterctl version via: clusterctl version")

	out, err := exec.Command(clusterctlBinaryPath, "version").Output()
	Expect(err).ToNot(HaveOccurred(), "error running clusterctl version")
	var clusterctlSupportsGenerateCluster bool
	if strings.Contains(string(out), "Major:\"1\"") {
		log.Logf("Detected clusterctl v1.x")
		clusterctlSupportsGenerateCluster = true
	}

	var cmd *exec.Cmd
	if clusterctlSupportsGenerateCluster {
		log.Logf("clusterctl generate cluster %s --infrastructure %s --kubernetes-version %s --control-plane-machine-count %d --worker-machine-count %d --flavor %s",
			input.ClusterName,
			valueOrDefault(input.InfrastructureProvider),
			input.KubernetesVersion,
			*input.ControlPlaneMachineCount,
			*input.WorkerMachineCount,
			valueOrDefault(input.Flavor),
		)
		cmd = exec.Command(clusterctlBinaryPath, "generate", "cluster", //nolint:gosec // We don't care about command injection here.
			input.ClusterName,
			"--infrastructure", input.InfrastructureProvider,
			"--kubernetes-version", input.KubernetesVersion,
			"--control-plane-machine-count", fmt.Sprint(*input.ControlPlaneMachineCount),
			"--worker-machine-count", fmt.Sprint(*input.WorkerMachineCount),
			"--flavor", input.Flavor,
			"--target-namespace", input.Namespace,
			"--config", input.ClusterctlConfigPath,
			"--kubeconfig", input.KubeconfigPath,
		)
	} else {
		log.Logf("clusterctl config cluster %s --infrastructure %s --kubernetes-version %s --control-plane-machine-count %d --worker-machine-count %d --flavor %s",
			input.ClusterName,
			valueOrDefault(input.InfrastructureProvider),
			input.KubernetesVersion,
			*input.ControlPlaneMachineCount,
			*input.WorkerMachineCount,
			valueOrDefault(input.Flavor),
		)
		cmd = exec.Command(clusterctlBinaryPath, "config", "cluster", //nolint:gosec // We don't care about command injection here.
			input.ClusterName,
			"--infrastructure", input.InfrastructureProvider,
			"--kubernetes-version", input.KubernetesVersion,
			"--control-plane-machine-count", fmt.Sprint(*input.ControlPlaneMachineCount),
			"--worker-machine-count", fmt.Sprint(*input.WorkerMachineCount),
			"--flavor", input.Flavor,
			"--target-namespace", input.Namespace,
			"--config", input.ClusterctlConfigPath,
			"--kubeconfig", input.KubeconfigPath,
		)
	}

	out, err = cmd.Output()
	_ = os.WriteFile(filepath.Join(input.LogFolder, fmt.Sprintf("%s-cluster-template.yaml", input.ClusterName)), out, 0644) //nolint:gosec // this is a log file to be shared via prow artifacts
	var stdErr string
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stdErr = string(exitErr.Stderr)
		}
	}
	Expect(err).ToNot(HaveOccurred(), "failed to run clusterctl config cluster:\nstdout:\n%s\nstderr:\n%s", string(out), stdErr)

	return out
}

// MoveInput is the input for ClusterctlMove.
type MoveInput struct {
	LogFolder            string
	ClusterctlConfigPath string
	FromKubeconfigPath   string
	ToKubeconfigPath     string
	Namespace            string
}

// Move moves workload clusters.
func Move(ctx context.Context, input MoveInput) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for Move")
	Expect(input.ClusterctlConfigPath).To(BeAnExistingFile(), "Invalid argument. input.ClusterctlConfigPath must be an existing file when calling Move")
	Expect(input.FromKubeconfigPath).To(BeAnExistingFile(), "Invalid argument. input.FromKubeconfigPath must be an existing file when calling Move")
	Expect(input.ToKubeconfigPath).To(BeAnExistingFile(), "Invalid argument. input.ToKubeconfigPath must be an existing file when calling Move")
	Expect(os.MkdirAll(input.LogFolder, 0750)).To(Succeed(), "Invalid argument. input.LogFolder can't be created for Move")

	By("Moving workload clusters")

	clusterctlClient, log := getClusterctlClientWithLogger(input.ClusterctlConfigPath, "clusterctl-move.log", input.LogFolder)
	defer log.Close()
	options := clusterctlclient.MoveOptions{
		FromKubeconfig: clusterctlclient.Kubeconfig{Path: input.FromKubeconfigPath, Context: ""},
		ToKubeconfig:   clusterctlclient.Kubeconfig{Path: input.ToKubeconfigPath, Context: ""},
		Namespace:      input.Namespace,
	}

	Expect(clusterctlClient.Move(options)).To(Succeed(), "Failed to run clusterctl move")
}

func getClusterctlClientWithLogger(configPath, logName, logFolder string) (clusterctlclient.Client, *logger.LogFile) {
	log := logger.CreateLogFile(logger.CreateLogFileInput{
		LogFolder: logFolder,
		Name:      logName,
	})
	clusterctllog.SetLogger(log.Logger())

	c, err := clusterctlclient.New(configPath)
	Expect(err).ToNot(HaveOccurred(), "Failed to create the clusterctl client library")
	return c, log
}

func valueOrDefault(v string) string {
	if v != "" {
		return v
	}
	return "(default)"
}
