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

package actions

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"regexp"

	"sigs.k8s.io/kind/pkg/cluster/config/defaults"

	"github.com/chuckha/cluster-api-provider-docker/third_party/forked/loadbalancer"
	"github.com/pkg/errors"
	"sigs.k8s.io/kind/pkg/cluster/constants"
	"sigs.k8s.io/kind/pkg/cluster/nodes"
	"sigs.k8s.io/kind/pkg/container/cri"
	"sigs.k8s.io/kind/pkg/exec"
)

// KubeConfigPath returns the path to the kubeconfig file for the given cluster name.
func KubeConfigPath(clusterName string) string {
	configDir := filepath.Join(os.Getenv("HOME"), ".kube")
	fileName := fmt.Sprintf("kind-config-%s", clusterName)
	return filepath.Join(configDir, fileName)
}

// AddControlPlane adds a control plane to a given cluster
func AddControlPlane(clusterName, machineName, version string) (*nodes.Node, error) {
	clusterLabel := fmt.Sprintf("%s=%s", constants.ClusterLabelKey, clusterName)
	kindImage := image(version)
	// This function exposes a port (makes sense for kind) that is not needed in capd scenarios.
	controlPlane, err := nodes.CreateControlPlaneNode(
		machineName,
		kindImage,
		clusterLabel,
		"127.0.0.1",
		0,
		nil,
	)
	if err != nil {
		return nil, err
	}
	if err := KubeadmJoinControlPlane(clusterName, controlPlane); err != nil {
		return nil, err
	}
	fmt.Println("Updating node providerID")
	if err := SetNodeProviderRef(clusterName, machineName); err != nil {
		return nil, err
	}
	if err := ConfigureLoadBalancer(clusterName); err != nil {
		return nil, err
	}
	return controlPlane, nil
}

// SetUpLoadBalancer creates a load balancer but does not configure it.
func SetUpLoadBalancer(clusterName string) (*nodes.Node, error) {
	clusterLabel := fmt.Sprintf("%s=%s", constants.ClusterLabelKey, clusterName)
	name := fmt.Sprintf("%s-%s", clusterName, constants.ExternalLoadBalancerNodeRoleValue)
	return nodes.CreateExternalLoadBalancerNode(
		name,
		loadbalancer.Image,
		clusterLabel,
		"0.0.0.0",
		0,
	)
}

// CreateControlPlane is creating the first control plane and configuring the load balancer.
func CreateControlPlane(clusterName, machineName, lbip, version string) (*nodes.Node, error) {
	fmt.Println("Creating control plane node")
	clusterLabel := fmt.Sprintf("%s=%s", constants.ClusterLabelKey, clusterName)
	kindImage := image(version)
	controlPlaneNode, err := nodes.CreateControlPlaneNode(
		machineName,
		kindImage,
		clusterLabel,
		"127.0.0.1",
		0,
		[]cri.Mount{},
	)
	if err != nil {
		return nil, err
	}
	fmt.Println("Configuring load balancer")
	if err := ConfigureLoadBalancer(clusterName); err != nil {
		return nil, err
	}
	fmt.Println("Configuring kubeadm")
	if err := KubeadmConfig(controlPlaneNode, clusterName, lbip); err != nil {
		return nil, err
	}
	fmt.Println("Initializing cluster")
	if err := KubeadmInit(clusterName); err != nil {
		return nil, err
	}
	fmt.Println("Updating node providerID")
	if err := SetNodeProviderRef(clusterName, machineName); err != nil {
		return nil, err
	}
	fmt.Println("Setting up CNI")
	if err := InstallCNI(controlPlaneNode); err != nil {
		return nil, err
	}
	fmt.Println("Created a cluster!")
	return controlPlaneNode, nil
}

// AddWorker adds a worker to a given cluster.
func AddWorker(clusterName, machineName, version string) (*nodes.Node, error) {
	clusterLabel := fmt.Sprintf("%s=%s", constants.ClusterLabelKey, clusterName)
	kindImage := image(version)
	worker, err := nodes.CreateWorkerNode(
		machineName,
		kindImage,
		clusterLabel,
		nil,
	)
	if err != nil {
		return nil, err
	}
	if err := KubeadmJoin(clusterName, worker); err != nil {
		return nil, err
	}
	fmt.Println("Updating node providerID")
	if err := SetNodeProviderRef(clusterName, machineName); err != nil {
		return nil, err
	}

	return worker, nil
}

// DeleteControlPlane will take all steps necessary to remove a control plane node from a cluster.
func DeleteControlPlane(clusterName, nodeName string) error {
	nodeList, err := nodes.List(
		fmt.Sprintf("label=%s=%s", constants.ClusterLabelKey, clusterName),
		fmt.Sprintf("name=%s$", nodeName),
	)
	if err != nil {
		return nil
	}

	// assume it's already deleted
	if len(nodeList) < 1 {
		return nil
	}

	// pick the first one, but there should never be multiples since docker requires name to be unique.
	node := nodeList[0]
	if err := DeleteClusterNode(clusterName, nodeName); err != nil {
		return err
	}
	if err := KubeadmReset(clusterName, nodeName); err != nil {
		return err
	}

	// Delete the infrastructure
	if err := nodes.Delete(node); err != nil {
		return err
	}
	return ConfigureLoadBalancer(clusterName)
}

func DeleteWorker(clusterName, nodeName string) error {
	nodeList, err := nodes.List(
		fmt.Sprintf("label=%s=%s", constants.ClusterLabelKey, clusterName),
		fmt.Sprintf("name=%s$", nodeName),
	)
	if err != nil {
		return nil
	}

	// assume it's already deleted
	if len(nodeList) < 1 {
		return nil
	}
	// pick the first one, but there should never be multiples since docker requires name to be unique.
	node := nodeList[0]

	if err := DeleteClusterNode(clusterName, nodeName); err != nil {
		return err
	}
	// Delete the infrastructure
	return nodes.Delete(node)
}

func ListControlPlanes(clusterName string) ([]nodes.Node, error) {
	return nodes.List(
		fmt.Sprintf("label=%s=%s", constants.ClusterLabelKey, clusterName),
		fmt.Sprintf("label=%s=%s", constants.NodeRoleKey, constants.ControlPlaneNodeRoleValue))
}

// getLoadBalancerPort returns the port on the host on which the APIServer is exposed
func getLoadBalancerPort(allNodes []nodes.Node) (int32, error) {
	node, err := nodes.ExternalLoadBalancerNode(allNodes)
	if err != nil {
		return 0, err
	}
	return node.Ports(6443)
}

// matches kubeconfig server entry like:
//    server: https://172.17.0.2:6443
// which we rewrite to:
//    server: https://$ADDRESS:$PORT
var serverAddressRE = regexp.MustCompile(`^(\s+server:) https://.*:\d+$`)

// writeKubeConfig writes a fixed KUBECONFIG to dest
// this should only be called on a control plane node
// While copyng to the host machine the control plane address
// is replaced with local host and the control plane port with
// a randomly generated port reserved during node creation.
func writeKubeConfig(n *nodes.Node, dest string, hostAddress string, hostPort int32) error {
	cmd := n.Command("cat", "/etc/kubernetes/admin.conf")
	lines, err := exec.CombinedOutputLines(cmd)
	if err != nil {
		return errors.Wrap(err, "failed to get kubeconfig from node")
	}

	// fix the config file, swapping out the server for the forwarded localhost:port
	var buff bytes.Buffer
	for _, line := range lines {
		match := serverAddressRE.FindStringSubmatch(line)
		if len(match) > 1 {
			addr := net.JoinHostPort(hostAddress, fmt.Sprintf("%d", hostPort))
			line = fmt.Sprintf("%s https://%s", match[1], addr)
		}
		buff.WriteString(line)
		buff.WriteString("\n")
	}

	// create the directory to contain the KUBECONFIG file.
	// 0755 is taken from client-go's config handling logic: https://github.com/kubernetes/client-go/blob/5d107d4ebc00ee0ea606ad7e39fd6ce4b0d9bf9e/tools/clientcmd/loader.go#L412
	err = os.MkdirAll(filepath.Dir(dest), 0755)
	if err != nil {
		return errors.Wrap(err, "failed to create kubeconfig output directory")
	}

	return ioutil.WriteFile(dest, buff.Bytes(), 0600)
}

func image(version string) string {
	// valid kindest node versions, but only >= v1.14.0
	switch version {
	case "v1.14.2":
	case "v1.14.1":
		return fmt.Sprintf("kindest/node:%s", version)
	}
	return defaults.Image
}
