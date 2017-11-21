/*
Copyright 2017 The Kubernetes Authors.

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

package util

import (
	"github.com/ghodss/yaml"
	"github.com/golang/glog"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	clusterv1 "k8s.io/kube-deploy/cluster-api/api/cluster/v1alpha1"
	"k8s.io/kube-deploy/cluster-api/client"
	"k8s.io/kube-deploy/cluster-api/util"
)

type Repairer struct {
	dryRun     bool
	configPath string
}

func NewRepairer(dryRun bool, configPath string) *Repairer {
	if configPath == "" {
		configPath = util.GetDefaultKubeConfigPath()
	}
	return &Repairer{dryRun: dryRun, configPath: configPath}
}

func (r *Repairer) RepairNode() error {
	nodes, err := r.getUnhealthyNodes()
	if err != nil {
		return err
	}
	if len(nodes) > 0 {
		glog.Infof("found unhealthy nodes: %v", nodes)
	} else {
		glog.Info("All nodes are healthy")
		return nil
	}

	if r.dryRun {
		glog.Info("Running in dry run mode. Not taking any action")
		return nil
	}

	for _, node := range nodes {
		m, err := r.getMachine(node)
		if err != nil {
			glog.Info("Error retrieving machine object %s. Not taking any action on this node.", node)
			continue
		}
		if err := r.deleteMachine(m.Name); err != nil {
			return err
		}

		if err := r.createMachine(util.Copy(m)); err != nil {
			return err
		}
		glog.Infof("Recreated node %s", node)
	}

	return nil
}

func (r *Repairer) getUnhealthyNodes() ([]string, error) {
	nodeList := &v1.NodeList{}
	out := util.ExecCommand("kubectl", "get", "nodes", "-o=yaml", "--kubeconfig="+r.configPath)
	err := yaml.Unmarshal([]byte(out), nodeList)
	if err != nil {
		return nil, err
	}

	var healthy []string
	var unhealthy []string

	for _, node := range nodeList.Items {
		if util.IsNodeReady(&node) {
			healthy = append(healthy, node.Name)
		} else {
			unhealthy = append(unhealthy, node.Name)
		}
	}
	glog.Infof("healthy nodes: %v", healthy)
	glog.Infof("unhealthy nodes: %v", unhealthy)
	return unhealthy, nil
}

func (r *Repairer) createMachine(machine *clusterv1.Machine) error {
	c, err := r.newApiClient()
	if err != nil {
		return err
	}

	m, err := c.Machines().Create(machine)
	if err != nil {
		return err
	}
	glog.Infof("Added machine [%s]", m.Name)

	return nil
}

func (r *Repairer) getMachine(name string) (*clusterv1.Machine, error) {
	c, err := r.newApiClient()
	if err != nil {
		return nil, err
	}
	machine, err := c.Machines().Get(name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return machine, nil
}

func (r *Repairer) deleteMachine(name string) error {
	c, err := r.newApiClient()
	if err != nil {
		return err
	}
	return c.Machines().Delete(name, &metav1.DeleteOptions{})
}

func (r *Repairer) newApiClient() (*client.ClusterAPIV1Alpha1Client, error) {
	config, err := clientcmd.BuildConfigFromFlags("", r.configPath)
	if err != nil {
		return nil, err
	}

	c, err := client.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return c, nil
}
