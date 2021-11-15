/*
Copyright 2019 The Kubernetes Authors.

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

package framework

import (
	"context"
	"fmt"
	"os"
	osExec "os/exec"
	"path/filepath"
	"strings"

	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kind/pkg/errors"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/infrastructure/container"
)

// DockerLogCollector collect logs from a CAPD workload cluster.
type DockerLogCollector struct{}

// machineContainerName return a container name using the same rule used in CAPD.
// NOTE: if the cluster name is already included in the machine name, the cluster name is not add thus
// avoiding \"sethostname: invalid argument\"  errors due to container name too long.
func machineContainerName(cluster, machine string) string {
	if strings.HasPrefix(machine, cluster) {
		return machine
	}
	return fmt.Sprintf("%s-%s", cluster, machine)
}

func (k DockerLogCollector) CollectMachineLog(ctx context.Context, managementClusterClient client.Client, m *clusterv1.Machine, outputPath string) error {
	containerName := machineContainerName(m.Spec.ClusterName, m.Name)
	containerRuntime, err := container.NewDockerClient()
	if err != nil {
		return err
	}
	ctx = container.RuntimeInto(ctx, containerRuntime)
	return k.collectLogsFromNode(ctx, outputPath, containerName)
}

func (k DockerLogCollector) CollectMachinePoolLog(ctx context.Context, managementClusterClient client.Client, m *expv1.MachinePool, outputPath string) error {
	containerRuntime, err := container.NewDockerClient()
	if err != nil {
		return err
	}
	ctx = container.RuntimeInto(ctx, containerRuntime)

	var errs []error
	for _, instance := range m.Status.NodeRefs {
		containerName := machineContainerName(m.Spec.ClusterName, instance.Name)
		if err := k.collectLogsFromNode(ctx, filepath.Join(outputPath, instance.Name), containerName); err != nil {
			// collecting logs is best effort so we proceed to the next instance even if we encounter an error.
			errs = append(errs, err)
		}
	}

	return kerrors.NewAggregate(errs)
}

func (k DockerLogCollector) collectLogsFromNode(ctx context.Context, outputPath string, containerName string) error {
	containerRuntime, err := container.RuntimeFrom(ctx)
	if err != nil {
		return errors.Wrap(err, "Failed to collect logs from node")
	}

	execToPathFn := func(outputFileName, command string, args ...string) func() error {
		return func() error {
			f, err := fileOnHost(filepath.Join(outputPath, outputFileName))
			if err != nil {
				return err
			}
			defer f.Close()
			execConfig := container.ExecContainerInput{
				OutputBuffer: f,
			}
			return containerRuntime.ExecContainer(ctx, containerName, &execConfig, command, args...)
		}
	}
	copyDirFn := func(containerDir, dirName string) func() error {
		return func() error {
			f, err := os.CreateTemp("", containerName)
			if err != nil {
				return err
			}

			tempfileName := f.Name()
			outputDir := filepath.Join(outputPath, dirName)

			defer os.Remove(tempfileName)

			execConfig := container.ExecContainerInput{
				OutputBuffer: f,
			}
			err = containerRuntime.ExecContainer(
				ctx,
				containerName,
				&execConfig,
				"tar", "--hard-dereference", "--dereference", "--directory", containerDir, "--create", "--file", "-", ".",
			)
			if err != nil {
				return err
			}

			err = os.MkdirAll(outputDir, os.ModePerm)
			if err != nil {
				return err
			}

			return osExec.Command("tar", "--extract", "--file", tempfileName, "--directory", outputDir).Run() //nolint:gosec // We don't care about command injection here.
		}
	}
	return errors.AggregateConcurrent([]func() error{
		execToPathFn(
			"journal.log",
			"journalctl", "--no-pager", "--output=short-precise",
		),
		execToPathFn(
			"kern.log",
			"journalctl", "--no-pager", "--output=short-precise", "-k",
		),
		execToPathFn(
			"kubelet-version.txt",
			"kubelet", "--version",
		),
		execToPathFn(
			"kubelet.log",
			"journalctl", "--no-pager", "--output=short-precise", "-u", "kubelet.service",
		),
		execToPathFn(
			"containerd-info.txt",
			"crictl", "info",
		),
		execToPathFn(
			"containerd.log",
			"journalctl", "--no-pager", "--output=short-precise", "-u", "containerd.service",
		),
		copyDirFn("/var/log/pods", "pods"),
	})
}

// fileOnHost is a helper to create a file at path
// even if the parent directory doesn't exist
// in which case it will be created with ModePerm.
func fileOnHost(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
		return nil, err
	}
	return os.Create(path)
}
