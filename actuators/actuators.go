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
	"fmt"
	"io/ioutil"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cluster-api-provider-docker/kind/actions"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/kind/pkg/cluster/constants"
	"sigs.k8s.io/kind/pkg/cluster/nodes"
)

func getRole(machine *clusterv1.Machine) string {
	// Figure out what kind of node we're making
	labels := machine.GetLabels()
	setValue, ok := labels["set"]
	if !ok {
		setValue = constants.WorkerNodeRoleValue
	}
	return setValue
}

// Cluster defines a cluster actuator object
type Cluster struct{}

// NewClusterActuator returns a new cluster actuator object
func NewClusterActuator() *Cluster {
	return &Cluster{}
}

// Reconcile setups an external load balancer for the cluster if needed
func (c *Cluster) Reconcile(cluster *clusterv1.Cluster) error {
	elb, err := getExternalLoadBalancerNode(cluster.Name)
	if err != nil {
		fmt.Printf("%+v\n", err)
		return err
	}
	if elb != nil {
		fmt.Println("External Load Balancer already exists. Nothing to do for this cluster.")
		return nil
	}
	fmt.Printf("The cluster named %q has been created! Setting up some infrastructure.\n", cluster.Name)
	_, err = actions.SetUpLoadBalancer(cluster.Name)
	return err
}

func getExternalLoadBalancerNode(clusterName string) (*nodes.Node, error) {
	elb, err := nodes.List(
		fmt.Sprintf("label=%s=%s", constants.NodeRoleKey, constants.ExternalLoadBalancerNodeRoleValue),
		fmt.Sprintf("label=%s=%s", constants.ClusterLabelKey, clusterName),
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
	return &elb[0], nil
}

// Delete can be used to delete a cluster
func (c *Cluster) Delete(cluster *clusterv1.Cluster) error {
	fmt.Println("Cluster delete is not implemented.")
	return nil
}

func kubeconfigToSecret(clusterName, namespace string) (*v1.Secret, error) {
	// open kubeconfig file
	data, err := ioutil.ReadFile(actions.KubeConfigPath(clusterName))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// write it to a secret
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("kubeconfig-%s", clusterName),
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"kubeconfig": data,
		},
	}, nil
}
