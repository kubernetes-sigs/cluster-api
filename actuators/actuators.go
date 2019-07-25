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

package actuators

import (
	"bytes"
	"fmt"
	"io/ioutil"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cluster-api-provider-docker/kind/actions"
	capiv1alpha2 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha2"
	"sigs.k8s.io/kind/pkg/cluster/constants"
	"sigs.k8s.io/kind/pkg/cluster/nodes"
)

const (
	containerRunningStatus = "running"
)

func getRole(machine *capiv1alpha2.Machine) string {
	// Figure out what kind of node we're making
	labels := machine.GetLabels()
	setValue, ok := labels["set"]
	if !ok {
		setValue = constants.WorkerNodeRoleValue
	}
	return setValue
}

func getExternalLoadBalancerNode(clusterName string, log logr.Logger) (*nodes.Node, error) {
	log.Info("Getting external load balancer node for cluster", "cluster-name", clusterName)
	elb, err := nodes.List(
		fmt.Sprintf("label=%s=%s", constants.NodeRoleKey, constants.ExternalLoadBalancerNodeRoleValue),
		fmt.Sprintf("label=%s=%s", constants.ClusterLabelKey, clusterName),
		fmt.Sprintf("status=%s", containerRunningStatus),
	)
	if err != nil {
		return nil, err
	}
	if len(elb) == 0 {
		return nil, nil
	}
	if len(elb) > 1 {
		return nil, errors.New("too many external load balancers")
	}
	log.Info("External loadbalancer node for cluster", "cluster-name", clusterName, "elb", elb[0])
	return &elb[0], nil
}

func kubeconfigToSecret(clusterName, namespace string) (*v1.Secret, error) {
	// open kubeconfig file
	data, err := ioutil.ReadFile(actions.KubeConfigPath(clusterName))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	allNodes, err := nodes.List(fmt.Sprintf("label=%s=%s", constants.ClusterLabelKey, clusterName))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// TODO: Clean this up at some point
	// The Management cluster, running the NodeRef controller, needs to talk to the child clusters.
	// The management cluster and child cluster must communicate over DockerIP address/ports.
	// The load balancer listens on <docker_ip>:6443 and exposes a port on the host at some random open port.
	// Any traffic directed to the nginx container will get round-robined to a control plane node in the cluster.
	// Since the NodeRef controller is running inside a container, it must reference the child cluster load balancer
	// host by using the Docker IP address and port 6443, but us, running on the docker host, must use the localhost
	// and random port the LB is exposing to our system.
	// Right now the secret that contains the kubeconfig will work only for the node ref controller. In order for *us*
	// to interact with the child clusters via kubeconfig we must take the secret uploaded,
	// rewrite the kube-apiserver-address to be 127.0.0.1:<randomly-assigned-by-docker-port>.
	// It's not perfect but it works to at least play with cluster-api v0.1.4
	lbip, _, err := actions.GetLoadBalancerHostAndPort(allNodes)
	lines := bytes.Split(data, []byte("\n"))
	for i, line := range lines {
		if bytes.Contains(line, []byte("https://")) {
			lines[i] = []byte(fmt.Sprintf("    server: https://%s:%d", lbip, 6443))
		}
	}

	// write it to a secret
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			// TODO pull in the kubeconfig secret function from cluster api
			Name:      fmt.Sprintf("%s-kubeconfig", clusterName),
			Namespace: namespace,
		},
		Data: map[string][]byte{
			// TODO pull in constant from cluster api
			"value": bytes.Join(lines, []byte("\n")),
		},
	}, nil
}
