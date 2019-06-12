// Copyright 2019 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0

package actions

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"regexp"

	"github.com/pkg/errors"
	"gitlab.com/chuckh/cluster-api-provider-kind/third_party/forked/loadbalancer"
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
func AddControlPlane(clusterName string) (*nodes.Node, error) {
	clusterLabel := fmt.Sprintf("%s=%s", constants.ClusterLabelKey, clusterName)
	// This function exposes a port (makes sense for kind) that is not needed in capk scenarios.
	controlPlane, err := nodes.CreateControlPlaneNode(
		getName(clusterName, constants.ControlPlaneNodeRoleValue),
		defaults.Image,
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
	if err := ConfigureLoadBalancer(clusterName); err != nil {
		return nil, err
	}
	return controlPlane, nil
}

// SetUpLoadBalancer creates a load balancer but does not configure it.
func SetUpLoadBalancer(clusterName string) (*nodes.Node, error) {
	clusterLabel := fmt.Sprintf("%s=%s", constants.ClusterLabelKey, clusterName)
	lb, err := nodes.CreateExternalLoadBalancerNode(
		getName(clusterName, constants.ExternalLoadBalancerNodeRoleValue),
		loadbalancer.Image,
		clusterLabel,
		"0.0.0.0",
		0,
	)
	if err != nil {
		return nil, err
	}
	return lb, nil
}

// CreateControlPlane is creating the first control plane and configuring the load balancer.
func CreateControlPlane(clusterName, lbip string) (*nodes.Node, error) {
	fmt.Println("Creating control plane node")
	clusterLabel := fmt.Sprintf("%s=%s", constants.ClusterLabelKey, clusterName)
	controlPlaneNode, err := nodes.CreateControlPlaneNode(
		getName(clusterName, constants.ControlPlaneNodeRoleValue),
		defaults.Image,
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
	fmt.Println("Setting up CNI")
	if err := InstallCNI(controlPlaneNode, clusterName); err != nil {
		return nil, err
	}
	fmt.Println("Created a cluster!")
	return controlPlaneNode, nil
}

// AddWorker adds a worker to a given cluster.
func AddWorker(clusterName string) (*nodes.Node, error) {
	clusterLabel := fmt.Sprintf("%s=%s", constants.ClusterLabelKey, clusterName)
	worker, err := nodes.CreateWorkerNode(
		getName(clusterName, constants.WorkerNodeRoleValue),
		defaults.Image,
		clusterLabel,
		nil,
	)
	if err != nil {
		return nil, err
	}
	if err := KubeadmJoin(clusterName, worker); err != nil {
		return nil, err
	}
	return worker, nil
}

// DeleteNode removes a node from a cluster and cleans up docker.
func DeleteNode(clusterName, nodeName string) error {
	ns, err := nodes.List(
		fmt.Sprintf("label=%s=%s", constants.ClusterLabelKey, clusterName),
		fmt.Sprintf("name=%s", nodeName))
	if err != nil {
		panic(err)
	}
	if len(ns) == 0 {
		fmt.Println("Could not find node", nodeName)
	}
	deleteCmd := exec.Command("kubectl", "delete", "node", nodeName, "--kubeconfig", KubeConfigPath(clusterName))
	if err := deleteCmd.SetStderr(os.Stdout).SetStdout(os.Stdout).Run(); err != nil {
		fmt.Printf("%+v\n", err)
		panic(err)
	}
	if err := nodes.Delete(ns...); err != nil {
		panic(err)
	}
	fmt.Println("Deleted a node")
	return nil
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

func getName(clusterName, role string) string {
	ns, err := nodes.List(
		fmt.Sprintf("label=%s=%s", constants.ClusterLabelKey, clusterName),
		fmt.Sprintf("label=%s=%s", constants.NodeRoleKey, role))
	if err != nil {
		panic(err)
	}
	count := len(ns)
	suffix := fmt.Sprintf("%d", count)
	if count == 0 {
		suffix = ""
	}
	return fmt.Sprintf("%s-%s%s", clusterName, role, suffix)
}
