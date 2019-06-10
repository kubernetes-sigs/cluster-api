// Copyright 2019 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0

package actions

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"gitlab.com/chuckh/cluster-api-provider-kind/kind/kubeadm"
	"gitlab.com/chuckh/cluster-api-provider-kind/kind/loadbalancer"
	"sigs.k8s.io/kind/pkg/cluster/config"
	"sigs.k8s.io/kind/pkg/cluster/constants"
	"sigs.k8s.io/kind/pkg/cluster/nodes"
	"sigs.k8s.io/kind/pkg/container/docker"
	"sigs.k8s.io/kind/pkg/exec"
	"sigs.k8s.io/kind/pkg/fs"
)

func KubeadmJoinControlPlane(clusterName string, node *nodes.Node) error {
	allNodes, err := nodes.List(fmt.Sprintf("label=%s=%s", constants.ClusterLabelKey, clusterName))
	if err != nil {
		return nil
	}

	// get the join address
	joinAddress, err := getJoinAddress(allNodes)
	if err != nil {
		return err
	}

	// creates the folder tree for pre-loading necessary cluster certificates
	// on the joining node
	if err := node.Command("mkdir", "-p", "/etc/kubernetes/pki/etcd").Run(); err != nil {
		return errors.Wrap(err, "failed to join node with kubeadm")
	}

	// define the list of necessary cluster certificates
	fileNames := []string{
		"ca.crt", "ca.key",
		"front-proxy-ca.crt", "front-proxy-ca.key",
		"sa.pub", "sa.key",
		// TODO(someone): if we gain external etcd support these will be
		// handled differently
		"etcd/ca.crt", "etcd/ca.key",
	}

	// creates a temporary folder on the host that should acts as a transit area
	// for moving necessary cluster certificates
	tmpDir, err := fs.TempDir("", "")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	err = os.MkdirAll(filepath.Join(tmpDir, "/etcd"), os.ModePerm)
	if err != nil {
		return err
	}

	// get the handle for the bootstrap control plane node (the source for necessary cluster certificates)
	controlPlaneHandle, err := nodes.BootstrapControlPlaneNode(allNodes)
	if err != nil {
		return err
	}

	// copies certificates from the bootstrap control plane node to the joining node
	for _, fileName := range fileNames {
		// sets the path of the certificate into a node
		containerPath := path.Join("/etc/kubernetes/pki", fileName)
		// set the path of the certificate into the tmp area on the host
		tmpPath := filepath.Join(tmpDir, fileName)
		// copies from bootstrap control plane node to tmp area
		if err := controlPlaneHandle.CopyFrom(containerPath, tmpPath); err != nil {
			return errors.Wrapf(err, "failed to copy certificate %s", fileName)
		}
		// copies from tmp area to joining node
		if err := node.CopyTo(tmpPath, containerPath); err != nil {
			return errors.Wrapf(err, "failed to copy certificate %s", fileName)
		}
	}

	// run kubeadm join --control-plane
	cmd := node.Command(
		"kubeadm", "join",
		// the join command uses the docker ip and a well know port that
		// are accessible only inside the docker network
		joinAddress,
		// set the node to join as control-plane
		"--experimental-control-plane",
		// uses a well known token and skips ca certification for automating TLS bootstrap process
		"--token", kubeadm.Token,
		"--discovery-token-unsafe-skip-ca-verification",
		// preflight errors are expected, in particular for swap being enabled
		// TODO(bentheelder): limit the set of acceptable errors
		"--ignore-preflight-errors=all",
		// increase verbosity for debug
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

func ConfigureLoadBalancer(clusterName string) error {
	allNodes, err := nodes.List(fmt.Sprintf("label=%s=%s", constants.ClusterLabelKey, clusterName))
	if err != nil {
		return nil
	}

	// identify external load balancer node
	loadBalancerNode, err := nodes.ExternalLoadBalancerNode(allNodes)
	if err != nil {
		return err
	}

	// collect info about the existing controlplane nodes
	var backendServers = map[string]string{}
	controlPlaneNodes, err := nodes.SelectNodesByRole(
		allNodes,
		constants.ControlPlaneNodeRoleValue,
	)
	if err != nil {
		return err
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
		return errors.Wrap(err, "failed to generate loadbalancer config data")
	}

	// create loadbalancer config on the node
	if err := loadBalancerNode.WriteFile(loadbalancer.ConfigPath, loadbalancerConfig); err != nil {
		// TODO: logging here
		return errors.Wrap(err, "failed to copy loadbalancer config to node")
	}

	// reload the config
	if err := docker.Kill("SIGHUP", loadBalancerNode.Name()); err != nil {
		return errors.Wrap(err, "failed to reload loadbalancer")
	}

	return nil
}

func KubeadmConfig(clusterName string) error {
	allNodes, err := nodes.List(fmt.Sprintf("label=%s=%s", constants.ClusterLabelKey, clusterName))
	if err != nil {
		return nil
	}

	node, err := nodes.BootstrapControlPlaneNode(allNodes)
	if err != nil {
		return err
	}

	// get installed kubernetes version from the node image
	kubeVersion, err := node.KubeVersion()
	if err != nil {
		// TODO(bentheelder): logging here
		return errors.Wrap(err, "failed to get kubernetes version from node")
	}

	// get the control plane endpoint, in case the cluster has an external load balancer in
	// front of the control-plane nodes
	controlPlaneEndpoint, err := nodes.GetControlPlaneEndpoint(allNodes)
	if err != nil {
		return err
	}

	// get kubeadm config content
	kubeadmConfig, err := getKubeadmConfig(
		&config.Cluster{},
		kubeadm.ConfigData{
			ClusterName:          clusterName,
			KubernetesVersion:    kubeVersion,
			ControlPlaneEndpoint: controlPlaneEndpoint,
			APIBindPort:          kubeadm.APIServerPort,
			APIServerAddress:     "127.0.0.1", // TODO MAYBE FIX THIS
			Token:                kubeadm.Token,
			PodSubnet:            "10.244.0.0/16",
		},
	)

	if err != nil {
		return errors.Wrap(err, "failed to generate kubeadm config content")
	}

	if err := node.WriteFile("/kind/kubeadm.conf", kubeadmConfig); err != nil {
		return errors.Wrap(err, "failed to copy kubeadm config to node")
	}

	return nil
}

func KubeadmInit(clusterName string) error {
	allNodes, err := nodes.List(fmt.Sprintf("label=%s=%s", constants.ClusterLabelKey, clusterName))
	if err != nil {
		return nil
	}

	// get the target node for this task
	node, err := nodes.BootstrapControlPlaneNode(allNodes)
	if err != nil {
		return err
	}

	// run kubeadm
	cmd := node.Command(
		// init because this is the control plane node
		"kubeadm", "init",
		// preflight errors are expected, in particular for swap being enabled
		// TODO(bentheelder): limit the set of acceptable errors
		"--ignore-preflight-errors=all",
		// specify our generated config file
		"--config=/kind/kubeadm.conf",
		"--skip-token-print",
		// increase verbosity for debugging
		"--v=6",
	)
	lines, err := exec.CombinedOutputLines(cmd)
	if err != nil {
		for _, line := range lines {
			fmt.Println(line)
		}
		return errors.Wrap(err, "failed to init node with kubeadm")
	}

	// copies the kubeconfig files locally in order to make the cluster
	// usable with kubectl.
	// the kubeconfig file created by kubeadm internally to the node
	// must be modified in order to use the random host port reserved
	// for the API server and exposed by the node

	hostPort, err := getAPIServerPort(allNodes)
	if err != nil {
		return errors.Wrap(err, "failed to get kubeconfig from node")
	}

	kubeConfigPath := KubeConfigPath(clusterName)
	if err := writeKubeConfig(node, kubeConfigPath, "127.0.0.1", hostPort); err != nil {
		return errors.Wrap(err, "failed to get kubeconfig from node")
	}

	// never remove master taint, like real clusters!
	return nil
}

func InstallCNI(clusterName string) error {
	allNodes, err := nodes.List(fmt.Sprintf("label=%s=%s", constants.ClusterLabelKey, clusterName))
	if err != nil {
		return nil
	}

	// get the target node for this task
	node, err := nodes.BootstrapControlPlaneNode(allNodes)
	if err != nil {
		return err
	}

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
	// TODO(bentheelder): optionally skip this entire action
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

	joinAddress, err := getJoinAddress(allNodes)
	if err != nil {
		return err
	}

	cmd := node.Command(
		"kubeadm", "join",
		// the join command uses the docker ip and a well know port that
		// are accessible only inside the docker network
		joinAddress,
		// uses a well known token and skipping ca certification for automating TLS bootstrap process
		"--token", kubeadm.Token,
		"--discovery-token-unsafe-skip-ca-verification",
		// preflight errors are expected, in particular for swap being enabled
		// TODO(bentheelder): limit the set of acceptable errors
		"--ignore-preflight-errors=all",
		// increase verbosity for debugging
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
