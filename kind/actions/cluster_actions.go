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
	"html/template"
	"strings"

	"github.com/chuckha/cluster-api-provider-docker/kind/kubeadm"
	"github.com/chuckha/cluster-api-provider-docker/third_party/forked/loadbalancer"
	"github.com/pkg/errors"
	"sigs.k8s.io/kind/pkg/cluster/constants"
	"sigs.k8s.io/kind/pkg/cluster/nodes"
	"sigs.k8s.io/kind/pkg/container/docker"
	"sigs.k8s.io/kind/pkg/exec"
)

// KubeadmJoinControlPlane joins a control plane to an existing Kubernetes cluster.
func KubeadmJoinControlPlane(clusterName string, node *nodes.Node) error {
	allNodes, err := nodes.List(fmt.Sprintf("label=%s=%s", constants.ClusterLabelKey, clusterName))
	if err != nil {
		return err
	}

	// get the join address
	joinAddress, err := nodes.GetControlPlaneEndpoint(allNodes)
	if err != nil {
		return err
	}

	if err := node.Command("mkdir", "-p", "/etc/kubernetes/pki/etcd").Run(); err != nil {
		return errors.Wrap(err, "failed to join node with kubeadm")
	}

	cmd := node.Command(
		"kubeadm", "join",
		joinAddress,
		"--experimental-control-plane",
		"--token", kubeadm.Token,
		"--discovery-token-unsafe-skip-ca-verification",
		"--ignore-preflight-errors=all",
		"--certificate-key", strings.Repeat("a", 32),
		"--v=6",
		"--cri-socket=/run/containerd/containerd.sock",
	)
	lines, err := exec.CombinedOutputLines(cmd)
	if err != nil {
		for _, line := range lines {
			fmt.Println(line)
		}
		return errors.Wrap(err, "failed to join a control plane node with kubeadm")
	}

	return nil
}

// ConfigureLoadBalancer updates the external load balancer with new control plane nodes.
// This should be run after every KubeadmJoinControlPlane
func ConfigureLoadBalancer(clusterName string) error {
	allNodes, err := nodes.List(fmt.Sprintf("label=%s=%s", constants.ClusterLabelKey, clusterName))
	if err != nil {
		return errors.WithStack(err)
	}

	// identify external load balancer node
	loadBalancerNode, err := nodes.ExternalLoadBalancerNode(allNodes)
	if err != nil {
		return errors.WithStack(err)
	}

	// collect info about the existing controlplane nodes
	var backendServers = map[string]string{}
	controlPlaneNodes, err := nodes.SelectNodesByRole(
		allNodes,
		constants.ControlPlaneNodeRoleValue,
	)
	if err != nil {
		return errors.WithStack(err)
	}
	for _, n := range controlPlaneNodes {
		controlPlaneIP, err := n.IP()
		if err != nil {
			return errors.Wrapf(err, "failed to get IP for node %s", n.Name())
		}
		backendServers[n.Name()] = fmt.Sprintf("%s:%d", controlPlaneIP, 6443)
	}

	// create loadbalancer config data
	loadbalancerConfig, err := loadbalancer.Config(&loadbalancer.ConfigData{
		ControlPlanePort: loadbalancer.ControlPlanePort,
		BackendServers:   backendServers,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	if err := loadBalancerNode.WriteFile(loadbalancer.ConfigPath, loadbalancerConfig); err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(docker.Kill("SIGHUP", loadBalancerNode.Name()))
}

func KubeadmConfig(node *nodes.Node, clusterName, lbip string) error {
	// get installed kubernetes version from the node image
	kubeVersion, err := node.KubeVersion()
	if err != nil {
		return errors.Wrap(err, "failed to get kubernetes version from node")
	}

	kubeadmConfig, err := kubeadm.InitConifguration(kubeVersion, clusterName, fmt.Sprintf("%s:%d", lbip, 6443))

	if err != nil {
		return errors.Wrap(err, "failed to generate kubeadm config content")
	}

	if err := node.WriteFile("/kind/kubeadm.conf", string(kubeadmConfig)); err != nil {
		return errors.Wrap(err, "failed to copy kubeadm config to node")
	}

	return nil
}

func KubeadmInit(clusterName string) error {
	allNodes, err := nodes.List(fmt.Sprintf("label=%s=%s", constants.ClusterLabelKey, clusterName))
	if err != nil {
		return nil
	}

	node, err := nodes.BootstrapControlPlaneNode(allNodes)
	if err != nil {
		return err
	}

	cmd := node.Command(
		"kubeadm", "init",
		"--ignore-preflight-errors=all",
		"--config=/kind/kubeadm.conf",
		"--skip-token-print",
		"--experimental-upload-certs",
		"--certificate-key", strings.Repeat("a", 32),
		"--v=6",
	)
	lines, err := exec.CombinedOutputLines(cmd)
	if err != nil {
		for _, line := range lines {
			fmt.Println(line)
		}
		return errors.Wrap(err, "failed to init node with kubeadm")
	}

	// save the kubeconfig on the host with the loadbalancer endpoint
	hostPort, err := getLoadBalancerPort(allNodes)
	if err != nil {
		return errors.Wrap(err, "failed to get kubeconfig from node")
	}
	dest := KubeConfigPath(clusterName)
	if err := writeKubeConfig(node, dest, "127.0.0.1", hostPort); err != nil {
		return errors.Wrap(err, "failed to get kubeconfig from node")
	}
	return nil
}

func InstallCNI(node *nodes.Node) error {
	// read the manifest from the node
	var raw bytes.Buffer
	if err := node.Command("cat", "/kind/manifests/default-cni.yaml").SetStdout(&raw).Run(); err != nil {
		return errors.Wrap(err, "failed to read CNI manifest")
	}
	manifest := raw.String()
	if strings.Contains(manifest, "would you kindly template this file") {
		t, err := template.New("cni-manifest").Parse(manifest)
		if err != nil {
			return errors.Wrap(err, "failed to parse CNI manifest template")
		}
		var out bytes.Buffer
		err = t.Execute(&out, &struct {
			PodSubnet string
		}{
			PodSubnet: "10.244.0.0/16",
		})
		if err != nil {
			return errors.Wrap(err, "failed to execute CNI manifest template")
		}
		manifest = out.String()
	}

	// install the manifest
	if err := node.Command(
		"kubectl", "create", "--kubeconfig=/etc/kubernetes/admin.conf",
		"-f", "-",
	).SetStdin(strings.NewReader(manifest)).Run(); err != nil {
		return errors.Wrap(err, "failed to apply overlay network")
	}
	return nil
}

func KubeadmJoin(clusterName string, node *nodes.Node) error {
	allNodes, err := nodes.List(fmt.Sprintf("label=%s=%s", constants.ClusterLabelKey, clusterName))
	if err != nil {
		return nil
	}

	joinAddress, err := nodes.GetControlPlaneEndpoint(allNodes)
	if err != nil {
		return err
	}

	cmd := node.Command(
		"kubeadm", "join",
		joinAddress,
		"--token", kubeadm.Token,
		"--discovery-token-unsafe-skip-ca-verification",
		"--ignore-preflight-errors=all",
		"--certificate-key", strings.Repeat("a", 32),
		"--v=6",
		"--cri-socket=/run/containerd/containerd.sock",
	)
	lines, err := exec.CombinedOutputLines(cmd)
	if err != nil {
		for _, line := range lines {
			fmt.Println(line)
		}
		return errors.Wrap(err, "failed to join node with kubeadm")
	}

	return nil
}

func SetNodeProviderRef(clusterName, nodeName string) error {
	allNodes, err := nodes.List(fmt.Sprintf("label=%s=%s", constants.ClusterLabelKey, clusterName))
	if err != nil {
		return err
	}

	node, err := nodes.BootstrapControlPlaneNode(allNodes)
	if err != nil {
		return err
	}

	patch := fmt.Sprintf(`{"spec": {"providerID": "docker://%s"}}`, nodeName)
	fmt.Println("trying to apply:", patch)
	cmd := node.Command(
		"kubectl",
		"--kubeconfig", "/etc/kubernetes/admin.conf",
		"patch",
		"node", nodeName,
		"--patch", patch,
	)
	lines, err := exec.CombinedOutputLines(cmd)
	if err != nil {
		for _, line := range lines {
			fmt.Println(line)
		}
		return errors.Wrap(err, "failed update providerID")
	}

	return nil
}

func GetNodeRefUID(clusterName, nodeName string) (string, error) {
	// 	k get nodes my-cluster-worker -o custom-columns=UID:.metadata.uid --no-headers
	allNodes, err := nodes.List(fmt.Sprintf("label=%s=%s", constants.ClusterLabelKey, clusterName))
	if err != nil {
		return "", err
	}

	node, err := nodes.BootstrapControlPlaneNode(allNodes)
	if err != nil {
		return "", err
	}

	patch := fmt.Sprintf(`{"spec": {"providerID": "docker://%s"}}`, nodeName)
	fmt.Println("trying to apply:", patch)
	cmd := node.Command(
		"kubectl",
		"--kubeconfig", "/etc/kubernetes/admin.conf",
		"get",
		"node", nodeName,
		"--output=custom-columns=UID:.metadata.uid",
		"--no-headers",
	)
	lines, err := exec.CombinedOutputLines(cmd)
	if err != nil {
		for _, line := range lines {
			fmt.Println(line)
		}
		return "", errors.Wrap(err, "failed get node ref UID")
	}
	return strings.TrimSpace(lines[0]), nil
}

func DeleteClusterNode(clusterName, nodeName string) error {
	// get all control plane nodes
	allNodes, err := nodes.List(fmt.Sprintf("label=%s=%s", constants.ClusterLabelKey, clusterName))
	if err != nil {
		return err
	}
	var node nodes.Node
	// pick one that doesn't match the node name we are trying to delete
	for _, n := range allNodes {
		if n.Name() != nodeName {
			node = n
			break
		}
	}
	cmd := node.Command(
		"kubectl",
		"--kubeconfig", "/etc/kubernetes/admin.conf",
		"delete", "node", nodeName,
	)
	lines, err := exec.CombinedOutputLines(cmd)
	if err != nil {
		for _, line := range lines {
			fmt.Println(line)
		}
		return errors.Wrap(err, "failed update providerID")
	}

	return nil
}
