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

	"github.com/pkg/errors"
	"sigs.k8s.io/cluster-api-provider-docker/cloudinit"
	constkind "sigs.k8s.io/cluster-api-provider-docker/docker/constants"
	"sigs.k8s.io/cluster-api-provider-docker/third_party/forked/loadbalancer"
	"sigs.k8s.io/kind/pkg/cluster/constants"
	"sigs.k8s.io/kind/pkg/cluster/nodes"
	"sigs.k8s.io/kind/pkg/container/docker"
	"sigs.k8s.io/kind/pkg/exec"
)

// NodeCloudConfig runs cloud-config on a node, this is generally `kubeadm <init|join>`
func NodeCloudConfig(node *nodes.Node, cloudConfig []byte) error {
	lines, err := cloudinit.Run(cloudConfig, node.Cmder())
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
		controlPlaneIPv4, _, err := n.IP()
		if err != nil {
			return errors.Wrapf(err, "failed to get IP for node %s", n.Name())
		}
		backendServers[n.Name()] = fmt.Sprintf("%s:%d", controlPlaneIPv4, constkind.APIServerPort)
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

// InstallCNI installs a CNI plugin from a node
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

// SetNodeProviderRef patches a node with docker://node-name as the providerID
func SetNodeProviderRef(clusterName, nodeName string) error {
	allNodes, err := nodes.List(fmt.Sprintf("label=%s=%s", constants.ClusterLabelKey, clusterName))
	if err != nil {
		return err
	}

	node, err := nodes.BootstrapControlPlaneNode(allNodes)
	if err != nil {
		return err
	}

	patch := fmt.Sprintf(`{"spec": {"providerID": "%s"}}`, ProviderID(nodeName))
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

// DeleteClusterNode will remove the kubernetes node from the list of nodes (during a kubectl get nodes).
func DeleteClusterNode(clusterName, nodeName string) error {
	// get all control plane nodes
	allControlPlanes, err := nodes.List(
		fmt.Sprintf("label=%s=%s", constants.ClusterLabelKey, clusterName),
		fmt.Sprintf("label=%s=%s", constants.NodeRoleKey, constants.ControlPlaneNodeRoleValue),
	)
	if err != nil {
		return errors.Wrap(err, "failed to list all control planes")
	}
	// If there is only one control plane being deleted do not update the corev1.Node list
	if len(allControlPlanes) == 1 {
		return nil
	}
	var node nodes.Node
	// pick one that doesn't match the node name we are trying to delete
	for _, n := range allControlPlanes {
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
		return errors.Wrap(err, "failed to delete cluster node")
	}
	return nil
}

// KubeadmReset will run `kubeadm reset` on the control plane to remove.
func KubeadmReset(clusterName, nodeName string) error {
	nodeList, err := nodes.List(
		fmt.Sprintf("label=%s=%s", constants.ClusterLabelKey, clusterName),
		fmt.Sprintf("label=%s=%s", constants.NodeRoleKey, constants.ControlPlaneNodeRoleValue),
		fmt.Sprintf("name=^%s$", nodeName),
	)
	if err != nil {
		return errors.Wrap(err, "could not list nodes")
	}
	if len(nodeList) < 1 {
		return errors.Errorf("could not find node %q", nodeName)
	}
	node := nodeList[0]

	cmd := node.Command("kubeadm", "reset", "--force")
	lines, err := exec.CombinedOutputLines(cmd)
	if err != nil {
		for _, line := range lines {
			fmt.Println(line)
		}
		return errors.Wrap(err, "failed to reset node")
	}

	return nil
}

// ProviderID formats the provider id needed to set on the node
func ProviderID(name string) string {
	return fmt.Sprintf("docker:////%s", name)
}
