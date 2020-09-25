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
	"path/filepath"
	"strings"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kind/pkg/errors"
	"sigs.k8s.io/kind/pkg/exec"
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
	execToPathFn := func(outputFileName, command string, args ...string) func() error {
		return func() error {
			f, err := fileOnHost(filepath.Join(outputPath, outputFileName))
			if err != nil {
				return err
			}
			defer f.Close()
			return execOnContainer(containerName, f, command, args...)
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
	})
}

// fileOnHost is a helper to create a file at path
// even if the parent directory doesn't exist
// in which case it will be created with ModePerm
func fileOnHost(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
		return nil, err
	}
	return os.Create(path)
}

// execOnContainer is an helper that runs a command on a CAPD node/container
func execOnContainer(containerName string, fileOnHost *os.File, command string, args ...string) error {
	dockerArgs := []string{
		"exec",
		// run with privileges so we can remount etc..
		// this might not make sense in the most general sense, but it is
		// important to many kind commands
		"--privileged",
	}
	// specify the container and command, after this everything will be
	// args the the command in the container rather than to docker
	dockerArgs = append(
		dockerArgs,
		containerName, // ... against the container
		command,       // with the command specified
	)
	dockerArgs = append(
		dockerArgs,
		// finally, with the caller args
		args...,
	)

	cmd := exec.Command("docker", dockerArgs...)
	cmd.SetEnv("PATH", os.Getenv("PATH"))

	cmd.SetStderr(fileOnHost)
	cmd.SetStdout(fileOnHost)

	return errors.WithStack(cmd.Run())
}
