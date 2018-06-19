/*
Copyright 2018 The Kubernetes Authors.

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

package vsphere

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/golang/glog"

	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

// Contains vsphere-specific deployment logic
// intended to eventually implement ProviderDeployer interface at
// sigs.k8s.io/cluster-api/clusterctl/clusterdeployer/clusterdeployer.go
// Currently implements a subset of the machineDeployer interface at
// sigs.k8s.io/cluster-api/vsphere-deployer/deploy/machinedeployer.go
// till vsphere-deployer gets deleted
type DeploymentClient struct{}

func NewDeploymentClient() *DeploymentClient {
	return &DeploymentClient{}
}

func (*DeploymentClient) GetIP(machine *clusterv1.Machine) (string, error) {
	if machine.ObjectMeta.Annotations != nil {
		if ip, ok := machine.ObjectMeta.Annotations[VmIpAnnotationKey]; ok {
			glog.Infof("Returning IP from machine annotation %s", ip)
			return ip, nil
		}
	}

	return "", errors.New("could not get IP")
}

func (d *DeploymentClient) GetKubeConfig(master *clusterv1.Machine) (string, error) {
	ip, err := d.GetIP(master)
	if err != nil {
		return "", err
	}

	var out bytes.Buffer
	cmd := exec.Command(
		"ssh", "-i", "~/.ssh/vsphere_tmp",
		"-q",
		"-o", "StrictHostKeyChecking no",
		"-o", "UserKnownHostsFile /dev/null",
		fmt.Sprintf("ubuntu@%s", ip),
		"sudo cat /etc/kubernetes/admin.conf")
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr
	cmd.Run()
	result := strings.TrimSpace(out.String())
	return result, nil
}
