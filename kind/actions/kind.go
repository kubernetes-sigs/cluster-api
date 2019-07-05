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
	"strings"

	"github.com/pkg/errors"
	"sigs.k8s.io/cluster-api-provider-docker/third_party/forked/loadbalancer"
	"sigs.k8s.io/kind/pkg/cluster/config/defaults"
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

func removeExitedELBWithNameConflict(name string) error {
	exitedLB, err := nodes.List(
		fmt.Sprintf("label=%s=%s", constants.NodeRoleKey, constants.ExternalLoadBalancerNodeRoleValue),
		fmt.Sprintf("name=%s", name),
		fmt.Sprintf("status=exited"),
	)

	if err != nil {
		return errors.Wrapf(err, "failed to list exited external load balancer nodes with name %q", name)
	}

	if len(exitedLB) > 0 {
		// only one container with given name should exist, if any.
		fmt.Printf("Removing exited ELB %q\n", exitedLB[0].Name())
		return nodes.Delete(exitedLB[0])
	}
	return nil
}

// SetUpLoadBalancer creates a load balancer but does not configure it.
func SetUpLoadBalancer(clusterName string) (*nodes.Node, error) {
	clusterLabel := fmt.Sprintf("%s=%s", constants.ClusterLabelKey, clusterName)
	name := fmt.Sprintf("%s-%s", clusterName, constants.ExternalLoadBalancerNodeRoleValue)
	err := removeExitedELBWithNameConflict(name)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to delete exited load balancer node %q", name)
	}

	return nodes.CreateExternalLoadBalancerNode(
		name,
		loadbalancer.Image,
		clusterLabel,
		"0.0.0.0",
		0,
	)
}

// CreateControlPlane is creating the first control plane and configuring the load balancer.
func CreateControlPlane(clusterName, machineName, lbip, version string, mounts []cri.Mount) (*nodes.Node, error) {
	fmt.Println("Creating control plane node")
	clusterLabel := fmt.Sprintf("%s=%s", constants.ClusterLabelKey, clusterName)
	kindImage := image(version)
	controlPlaneNode, err := nodes.CreateControlPlaneNode(
		machineName,
		kindImage,
		clusterLabel,
		"127.0.0.1",
		0,
		mounts,
		nil,
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
	if err := KubeadmInit(clusterName, version); err != nil {
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

// DeleteWorker removes a worker node from a cluster
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

// ListControlPlanes returns the list of control-plane nodes for a cluster
func ListControlPlanes(clusterName string) ([]nodes.Node, error) {
	return nodes.List(
		fmt.Sprintf("label=%s=%s", constants.ClusterLabelKey, clusterName),
		fmt.Sprintf("label=%s=%s", constants.NodeRoleKey, constants.ControlPlaneNodeRoleValue))
}

// GetLoadBalancerHostAndPort returns the port on the host on which the APIServer is exposed
func GetLoadBalancerHostAndPort(allNodes []nodes.Node) (string, int32, error) {
	node, err := nodes.ExternalLoadBalancerNode(allNodes)
	if err != nil {
		return "", 0, err
	}
	ipv4, _, err := node.IP()
	if err != nil {
		return "", 0, err
	}
	port, err := node.Ports(6443)
	return ipv4, port, err
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
	if !strings.HasPrefix(version, "v") {
		version = fmt.Sprintf("v%s", version)
	}
	// valid kindest node versions, but only > v1.14.0
	switch version {
	case "v1.14.1", "v1.14.2", "v1.14.3", "v1.15.0":
		return fmt.Sprintf("kindest/node:%s", version)
	}
	return defaults.Image
}
