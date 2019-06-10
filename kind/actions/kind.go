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
	"strings"

	"sigs.k8s.io/kind/pkg/container/cri"

	"github.com/pkg/errors"
	"gitlab.com/chuckh/cluster-api-provider-kind/kind/kubeadm"
	"gitlab.com/chuckh/cluster-api-provider-kind/kind/loadbalancer"
	"sigs.k8s.io/kind/pkg/cluster/config"
	"sigs.k8s.io/kind/pkg/cluster/config/defaults"
	"sigs.k8s.io/kind/pkg/cluster/constants"
	"sigs.k8s.io/kind/pkg/cluster/nodes"
	"sigs.k8s.io/kind/pkg/exec"
	"sigs.k8s.io/kind/pkg/kustomize"
)

// Hi, a lot of this code is copied and modified from kind.

// KubeConfigPath returns the path to the kubeconfig file for the given cluster name.
func KubeConfigPath(clusterName string) string {
	// configDir matches the standard directory expected by kubectl etc
	configDir := "/kubeconfigs"
	// note that the file name however does not, we do not want to overwrite
	// the standard config, though in the future we may (?) merge them
	fileName := fmt.Sprintf("kind-config-%s", clusterName)
	return filepath.Join(configDir, fileName)
}

// AddControlPlane adds a control plane to a given cluster
func AddControlPlane(clusterName string) (*nodes.Node, error) {
	clusterLabel := fmt.Sprintf("%s=%s", constants.ClusterLabelKey, clusterName)
	fmt.Println("Adding a new control plane")
	controlPlane, err := nodes.CreateControlPlaneNode(
		getName(clusterName, constants.ControlPlaneNodeRoleValue),
		defaults.Image,
		clusterLabel,
		"127.0.0.1",
		6443,
		nil,
	)
	if err != nil {
		return nil, err
	}
	fmt.Println("Joining new control plane")
	if err := KubeadmJoinControlPlane(clusterName, controlPlane); err != nil {
		return nil, err
	}
	fmt.Println("Configuring load balancer")
	if err := ConfigureLoadBalancer(clusterName); err != nil {
		return nil, err
	}
	return controlPlane, nil
}

// SetUpLoadBalancer creates a load balancer but does not configure it.
func SetUpLoadBalancer(clusterName string) error {
	clusterLabel := fmt.Sprintf("%s=%s", constants.ClusterLabelKey, clusterName)
	// create load balancer
	if _, err := nodes.CreateExternalLoadBalancerNode(
		getName(clusterName, constants.ExternalLoadBalancerNodeRoleValue),
		loadbalancer.Image,
		clusterLabel,
		"0.0.0.0",
		0,
	); err != nil {
		return err
	}
	return nil
}

// CreateControlPlane is creating the first control plane and configuring the load balancer.
func CreateControlPlane(clusterName string) (*nodes.Node, error) {
	fmt.Println("Creating control plane node")
	clusterLabel := fmt.Sprintf("%s=%s", constants.ClusterLabelKey, clusterName)
	controlPlaneNode, err := nodes.CreateControlPlaneNode(
		getName(clusterName, constants.ControlPlaneNodeRoleValue),
		defaults.Image,
		clusterLabel,
		"127.0.0.1",
		0,
		[]cri.Mount{
			{
				ContainerPath: "/root/.kube",
				HostPath:      "/kubeconfigs",
			},
		},
	)
	if err != nil {
		return nil, err
	}
	fmt.Println("Configuring load balancer")
	if err := ConfigureLoadBalancer(clusterName); err != nil {
		return nil, err
	}
	fmt.Println("Configuring kubeadm")
	if err := KubeadmConfig(clusterName); err != nil {
		return nil, err
	}
	fmt.Println("Initializing cluster")
	if err := KubeadmInit(clusterName); err != nil {
		return nil, err
	}
	fmt.Println("Setting up CNI")
	if err := InstallCNI(clusterName); err != nil {
		return nil, err
	}
	fmt.Println("Created a cluster!")
	return controlPlaneNode, nil
}

// AddWorker adds a worker to a given cluster.
func AddWorker(clusterName string) (*nodes.Node, error) {
	clusterLabel := fmt.Sprintf("%s=%s", constants.ClusterLabelKey, clusterName)
	fmt.Println("Adding a new worker")
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

// getJoinAddress return the join address thas is the control plane endpoint in case the cluster has
// an external load balancer in front of the control-plane nodes, otherwise the address of the
// boostrap control plane node.
func getJoinAddress(allNodes []nodes.Node) (string, error) {
	// get the control plane endpoint, in case the cluster has an external load balancer in
	// front of the control-plane nodes
	controlPlaneEndpoint, err := nodes.GetControlPlaneEndpoint(allNodes)
	if err != nil {
		// TODO(bentheelder): logging here
		return "", err
	}

	// if the control plane endpoint is defined we are using it as a join address
	if controlPlaneEndpoint != "" {
		return controlPlaneEndpoint, nil
	}

	// otherwise, get the BootStrapControlPlane node
	controlPlaneHandle, err := nodes.BootstrapControlPlaneNode(allNodes)
	if err != nil {
		return "", err
	}

	// get the IP of the bootstrap control plane node
	controlPlaneIP, err := controlPlaneHandle.IP()
	if err != nil {
		return "", errors.Wrap(err, "failed to get IP for node")
	}

	return fmt.Sprintf("%s:%d", controlPlaneIP, kubeadm.APIServerPort), nil
}

// getKubeadmConfig generates the kubeadm config contents for the cluster
// by running data through the template.
func getKubeadmConfig(cfg *config.Cluster, data kubeadm.ConfigData) (path string, err error) {
	// generate the config contents
	c, err := kubeadm.Config(data)
	if err != nil {
		return "", err
	}
	// fix all the patches to have name metadata matching the generated config
	patches, jsonPatches := setPatchNames(
		allPatchesFromConfig(cfg),
	)
	// apply patches
	// TODO(bentheelder): this does not respect per node patches at all
	// either make patches cluster wide, or change this
	patched, err := kustomize.Build([]string{c}, patches, jsonPatches)
	if err != nil {
		return "", err
	}
	return removeMetadata(patched), nil
}

// trims out the metadata.name we put in the config for kustomize matching,
// kubeadm will complain about this otherwise
func removeMetadata(kustomized string) string {
	return strings.Replace(
		kustomized,
		`metadata:
  name: config
`,
		"",
		-1,
	)
}

func allPatchesFromConfig(cfg *config.Cluster) (patches []string, jsonPatches []kustomize.PatchJSON6902) {
	return cfg.KubeadmConfigPatches, cfg.KubeadmConfigPatchesJSON6902
}

// setPatchNames sets the targeted object name on every patch to be the fixed
// name we use when generating config objects (we have one of each type, all of
// which have the same fixed name)
func setPatchNames(patches []string, jsonPatches []kustomize.PatchJSON6902) ([]string, []kustomize.PatchJSON6902) {
	fixedPatches := make([]string, len(patches))
	fixedJSONPatches := make([]kustomize.PatchJSON6902, len(jsonPatches))
	for i, patch := range patches {
		// insert the generated name metadata
		fixedPatches[i] = fmt.Sprintf("metadata:\nname: %s\n%s", kubeadm.ObjectName, patch)
	}
	for i, patch := range jsonPatches {
		// insert the generated name metadata
		patch.Name = kubeadm.ObjectName
		fixedJSONPatches[i] = patch
	}
	return fixedPatches, fixedJSONPatches
}

// getAPIServerPort returns the port on the host on which the APIServer
// is exposed
func getAPIServerPort(allNodes []nodes.Node) (int32, error) {
	// select the external loadbalancer first
	node, err := nodes.ExternalLoadBalancerNode(allNodes)
	if err != nil {
		return 0, err
	}
	// node will be nil if there is no load balancer
	if node != nil {
		return node.Ports(loadbalancer.ControlPlanePort)
	}

	// fallback to the bootstrap control plane
	node, err = nodes.BootstrapControlPlaneNode(allNodes)
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
