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
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/blang/semver/v4"
	. "github.com/onsi/ginkgo/v2"
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
	LogFolder                 string
	ClusterctlConfigPath      string
	KubeconfigPath            string
	CoreProvider              string
	BootstrapProviders        []string
	ControlPlaneProviders     []string
	InfrastructureProviders   []string
	IPAMProviders             []string
	RuntimeExtensionProviders []string
	AddonProviders            []string
}

// Init calls clusterctl init with the list of providers defined in the local repository.
func Init(ctx context.Context, input InitInput) {
	args := calculateClusterCtlInitArgs(input, "")
	log.Logf("clusterctl %s", strings.Join(args, " "))

	initOpt := clusterctlclient.InitOptions{
		Kubeconfig: clusterctlclient.Kubeconfig{
			Path:    input.KubeconfigPath,
			Context: "",
		},
		CoreProvider:              input.CoreProvider,
		BootstrapProviders:        input.BootstrapProviders,
		ControlPlaneProviders:     input.ControlPlaneProviders,
		InfrastructureProviders:   input.InfrastructureProviders,
		IPAMProviders:             input.IPAMProviders,
		RuntimeExtensionProviders: input.RuntimeExtensionProviders,
		AddonProviders:            input.AddonProviders,
		LogUsageInstructions:      true,
		WaitProviders:             true,
	}

	clusterctlClient, log := getClusterctlClientWithLogger(ctx, input.ClusterctlConfigPath, "clusterctl-init.log", input.LogFolder)
	defer log.Close()

	_, err := clusterctlClient.Init(ctx, initOpt)
	Expect(err).ToNot(HaveOccurred(), "failed to run clusterctl init")
}

// InitWithBinary uses clusterctl binary to run init with the list of providers defined in the local repository.
func InitWithBinary(_ context.Context, binary string, input InitInput) {
	args := calculateClusterCtlInitArgs(input, binary)
	log.Logf("clusterctl %s", strings.Join(args, " "))

	cmd := exec.Command(binary, args...) //nolint:gosec // We don't care about command injection here.

	out, err := cmd.CombinedOutput()
	_ = os.WriteFile(filepath.Join(input.LogFolder, "clusterctl-init.log"), out, 0644) //nolint:gosec // this is a log file to be shared via prow artifacts
	var stdErr string
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			stdErr = string(exitErr.Stderr)
		}
	}
	Expect(err).ToNot(HaveOccurred(), "failed to run clusterctl init:\nstdout:\n%s\nstderr:\n%s", string(out), stdErr)
}

func calculateClusterCtlInitArgs(input InitInput, clusterctlBinaryPath string) []string {
	args := []string{"init", "--config", input.ClusterctlConfigPath, "--kubeconfig", input.KubeconfigPath}

	// If we use the clusterctl binary, only set --wait-providers for clusterctl >= v0.4.0.
	if clusterctlBinaryPath != "" {
		version, err := getClusterCtlVersion(clusterctlBinaryPath)
		Expect(err).ToNot(HaveOccurred())
		if version.GTE(semver.MustParse("0.4.0")) {
			args = append(args, "--wait-providers")
		}
	} else {
		args = append(args, "--wait-providers")
	}

	if input.CoreProvider != "" {
		args = append(args, "--core", input.CoreProvider)
	}
	if len(input.BootstrapProviders) > 0 {
		args = append(args, "--bootstrap", strings.Join(input.BootstrapProviders, ","))
	}
	if len(input.ControlPlaneProviders) > 0 {
		args = append(args, "--control-plane", strings.Join(input.ControlPlaneProviders, ","))
	}
	if len(input.InfrastructureProviders) > 0 {
		args = append(args, "--infrastructure", strings.Join(input.InfrastructureProviders, ","))
	}
	if len(input.IPAMProviders) > 0 {
		args = append(args, "--ipam", strings.Join(input.IPAMProviders, ","))
	}
	if len(input.RuntimeExtensionProviders) > 0 {
		args = append(args, "--runtime-extension", strings.Join(input.RuntimeExtensionProviders, ","))
	}
	if len(input.AddonProviders) > 0 {
		args = append(args, "--addon", strings.Join(input.AddonProviders, ","))
	}
	return args
}

// UpgradeInput is the input for Upgrade.
type UpgradeInput struct {
	LogFolder                 string
	ClusterctlConfigPath      string
	ClusterctlVariables       map[string]string
	ClusterName               string
	KubeconfigPath            string
	Contract                  string
	CoreProvider              string
	BootstrapProviders        []string
	ControlPlaneProviders     []string
	InfrastructureProviders   []string
	IPAMProviders             []string
	RuntimeExtensionProviders []string
	AddonProviders            []string
}

// Upgrade calls clusterctl upgrade apply with the list of providers defined in the local repository.
func Upgrade(ctx context.Context, input UpgradeInput) {
	if len(input.ClusterctlVariables) > 0 {
		outputPath := filepath.Join(filepath.Dir(input.ClusterctlConfigPath), fmt.Sprintf("clusterctl-upgrade-config-%s.yaml", input.ClusterName))
		Expect(CopyAndAmendClusterctlConfig(ctx, CopyAndAmendClusterctlConfigInput{
			ClusterctlConfigPath: input.ClusterctlConfigPath,
			OutputPath:           outputPath,
			Variables:            input.ClusterctlVariables,
		})).To(Succeed(), "Failed to CopyAndAmendClusterctlConfig")
		input.ClusterctlConfigPath = outputPath
	}

	args := calculateClusterCtlUpgradeArgs(input)
	log.Logf("clusterctl %s", strings.Join(args, " "))

	upgradeOpt := clusterctlclient.ApplyUpgradeOptions{
		Kubeconfig: clusterctlclient.Kubeconfig{
			Path:    input.KubeconfigPath,
			Context: "",
		},
		Contract:                  input.Contract,
		CoreProvider:              input.CoreProvider,
		BootstrapProviders:        input.BootstrapProviders,
		ControlPlaneProviders:     input.ControlPlaneProviders,
		InfrastructureProviders:   input.InfrastructureProviders,
		IPAMProviders:             input.IPAMProviders,
		RuntimeExtensionProviders: input.RuntimeExtensionProviders,
		AddonProviders:            input.AddonProviders,
		WaitProviders:             true,
	}

	clusterctlClient, log := getClusterctlClientWithLogger(ctx, input.ClusterctlConfigPath, "clusterctl-upgrade.log", input.LogFolder)
	defer log.Close()

	err := clusterctlClient.ApplyUpgrade(ctx, upgradeOpt)
	Expect(err).ToNot(HaveOccurred(), "failed to run clusterctl upgrade")
}

// UpgradeWithBinary calls clusterctl upgrade apply with the list of providers defined in the local repository.
func UpgradeWithBinary(ctx context.Context, binary string, input UpgradeInput) error {
	if len(input.ClusterctlVariables) > 0 {
		outputPath := filepath.Join(filepath.Dir(input.ClusterctlConfigPath), fmt.Sprintf("clusterctl-upgrade-config-%s.yaml", input.ClusterName))
		Expect(CopyAndAmendClusterctlConfig(ctx, CopyAndAmendClusterctlConfigInput{
			ClusterctlConfigPath: input.ClusterctlConfigPath,
			OutputPath:           outputPath,
			Variables:            input.ClusterctlVariables,
		})).To(Succeed(), "Failed to CopyAndAmendClusterctlConfig")
		input.ClusterctlConfigPath = outputPath
	}

	args := calculateClusterCtlUpgradeArgs(input)
	log.Logf("clusterctl %s", strings.Join(args, " "))

	cmd := exec.Command(binary, args...) //nolint:gosec // We don't care about command injection here.

	out, err := cmd.CombinedOutput()
	_ = os.WriteFile(filepath.Join(input.LogFolder, "clusterctl-upgrade.log"), out, 0644) //nolint:gosec // this is a log file to be shared via prow artifacts
	var stdErr string
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			stdErr = string(exitErr.Stderr)
		}
		return fmt.Errorf("failed to run clusterctl upgrade apply:\nstdout:\n%s\nstderr:\n%s", string(out), stdErr)
	}
	return nil
}

func calculateClusterCtlUpgradeArgs(input UpgradeInput) []string {
	args := []string{"upgrade", "apply", "--config", input.ClusterctlConfigPath, "--kubeconfig", input.KubeconfigPath, "--wait-providers"}

	// Check if the user want a custom upgrade
	isCustomUpgrade := input.CoreProvider != "" ||
		len(input.BootstrapProviders) > 0 ||
		len(input.ControlPlaneProviders) > 0 ||
		len(input.InfrastructureProviders) > 0 ||
		len(input.IPAMProviders) > 0 ||
		len(input.RuntimeExtensionProviders) > 0 ||
		len(input.AddonProviders) > 0

	Expect((input.Contract != "" && !isCustomUpgrade) || (input.Contract == "" && isCustomUpgrade)).To(BeTrue(), `Invalid arguments. Either the input.Contract parameter or at least one of the following providers has to be set:
		input.CoreProvider, input.BootstrapProviders, input.ControlPlaneProviders, input.InfrastructureProviders, input.IPAMProviders, input.RuntimeExtensionProviders, input.AddonProviders`)

	if isCustomUpgrade {
		if input.CoreProvider != "" {
			args = append(args, "--core", input.CoreProvider)
		}
		if len(input.BootstrapProviders) > 0 {
			args = append(args, "--bootstrap", strings.Join(input.BootstrapProviders, ","))
		}
		if len(input.ControlPlaneProviders) > 0 {
			args = append(args, "--control-plane", strings.Join(input.ControlPlaneProviders, ","))
		}
		if len(input.InfrastructureProviders) > 0 {
			args = append(args, "--infrastructure", strings.Join(input.InfrastructureProviders, ","))
		}
		if len(input.IPAMProviders) > 0 {
			args = append(args, "--ipam", strings.Join(input.IPAMProviders, ","))
		}
		if len(input.RuntimeExtensionProviders) > 0 {
			args = append(args, "--runtime-extension", strings.Join(input.RuntimeExtensionProviders, ","))
		}
		if len(input.AddonProviders) > 0 {
			args = append(args, "--addon", strings.Join(input.AddonProviders, ","))
		}
	} else {
		args = append(args, "--contract", input.Contract)
	}

	return args
}

// DeleteInput is the input for Delete.
type DeleteInput struct {
	LogFolder            string
	ClusterctlConfigPath string
	KubeconfigPath       string
}

// Delete calls clusterctl delete --all.
func Delete(ctx context.Context, input DeleteInput) {
	log.Logf("clusterctl delete --all")

	deleteOpts := clusterctlclient.DeleteOptions{
		Kubeconfig: clusterctlclient.Kubeconfig{
			Path:    input.KubeconfigPath,
			Context: "",
		},
		DeleteAll: true,
	}

	clusterctlClient, log := getClusterctlClientWithLogger(ctx, input.ClusterctlConfigPath, "clusterctl-delete.log", input.LogFolder)
	defer log.Close()

	err := clusterctlClient.Delete(ctx, deleteOpts)
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
	ClusterctlVariables      map[string]string
}

// ConfigCluster gets a workload cluster based on a template.
func ConfigCluster(ctx context.Context, input ConfigClusterInput) []byte {
	var workerMachineCountArg string
	if input.WorkerMachineCount != nil {
		workerMachineCountArg = fmt.Sprintf("--worker-machine-count %d ", *input.WorkerMachineCount)
	}
	log.Logf("clusterctl config cluster %s --infrastructure %s --kubernetes-version %s --control-plane-machine-count %d %s--flavor %s",
		input.ClusterName,
		valueOrDefault(input.InfrastructureProvider),
		input.KubernetesVersion,
		*input.ControlPlaneMachineCount,
		workerMachineCountArg,
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

	if len(input.ClusterctlVariables) > 0 {
		outputPath := filepath.Join(filepath.Dir(input.ClusterctlConfigPath), fmt.Sprintf("clusterctl-upgrade-config-%s.yaml", input.ClusterName))
		Expect(CopyAndAmendClusterctlConfig(ctx, CopyAndAmendClusterctlConfigInput{
			ClusterctlConfigPath: input.ClusterctlConfigPath,
			OutputPath:           outputPath,
			Variables:            input.ClusterctlVariables,
		})).To(Succeed(), "Failed to CopyAndAmendClusterctlConfig")
		input.ClusterctlConfigPath = outputPath
	}

	clusterctlClient, log := getClusterctlClientWithLogger(ctx, input.ClusterctlConfigPath, fmt.Sprintf("%s-cluster-template.yaml", input.ClusterName), input.LogFolder)
	defer log.Close()

	template, err := clusterctlClient.GetClusterTemplate(ctx, templateOptions)
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
	version, err := getClusterCtlVersion(clusterctlBinaryPath)
	Expect(err).ToNot(HaveOccurred())
	clusterctlSupportsGenerateCluster := version.GTE(semver.MustParse("1.0.0"))

	var command string
	if clusterctlSupportsGenerateCluster {
		command = "generate"
	} else {
		command = "config"
	}

	args := []string{command, "cluster",
		input.ClusterName,
		"--infrastructure", input.InfrastructureProvider,
		"--kubernetes-version", input.KubernetesVersion,
		"--worker-machine-count", fmt.Sprint(*input.WorkerMachineCount),
		"--flavor", input.Flavor,
		"--target-namespace", input.Namespace,
		"--config", input.ClusterctlConfigPath,
		"--kubeconfig", input.KubeconfigPath,
	}
	if input.ControlPlaneMachineCount != nil && *input.ControlPlaneMachineCount > 0 {
		args = append(args, "--control-plane-machine-count", fmt.Sprint(*input.ControlPlaneMachineCount))
	}
	log.Logf("clusterctl %s", strings.Join(args, " "))

	cmd := exec.Command(clusterctlBinaryPath, args...) //nolint:gosec // We don't care about command injection here.
	out, err := cmd.Output()
	_ = os.WriteFile(filepath.Join(input.LogFolder, fmt.Sprintf("%s-cluster-template.yaml", input.ClusterName)), out, 0644) //nolint:gosec // this is a log file to be shared via prow artifacts
	var stdErr string
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
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
	logDir := path.Join(input.LogFolder, "logs", input.Namespace)
	Expect(os.MkdirAll(logDir, 0750)).To(Succeed(), "Invalid argument. input.LogFolder can't be created for Move")

	By("Moving workload clusters")
	log.Logf("clusterctl move --from-kubeconfig %s --to-kubeconfig %s --namespace %s",
		input.FromKubeconfigPath,
		input.ToKubeconfigPath,
		input.Namespace,
	)

	clusterctlClient, log := getClusterctlClientWithLogger(ctx, input.ClusterctlConfigPath, "clusterctl-move.log", logDir)
	defer log.Close()
	options := clusterctlclient.MoveOptions{
		FromKubeconfig: clusterctlclient.Kubeconfig{Path: input.FromKubeconfigPath, Context: ""},
		ToKubeconfig:   clusterctlclient.Kubeconfig{Path: input.ToKubeconfigPath, Context: ""},
		Namespace:      input.Namespace,
	}

	Expect(clusterctlClient.Move(ctx, options)).To(Succeed(), "Failed to run clusterctl move")
}

func getClusterctlClientWithLogger(ctx context.Context, configPath, logName, logFolder string) (clusterctlclient.Client, *logger.LogFile) {
	log := logger.OpenLogFile(logger.OpenLogFileInput{
		LogFolder: logFolder,
		Name:      logName,
	})
	clusterctllog.SetLogger(log.Logger())

	c, err := clusterctlclient.New(ctx, configPath)
	Expect(err).ToNot(HaveOccurred(), "Failed to create the clusterctl client library")
	return c, log
}

func valueOrDefault(v string) string {
	if v != "" {
		return v
	}
	return "(default)"
}
