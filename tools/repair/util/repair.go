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
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/typed/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/clientcmd"
	"sigs.k8s.io/cluster-api/pkg/util"
)

type repairer struct {
	dryRun        bool
	configPath    string
	machInterface v1alpha1.MachineInterface
}

func NewRepairer(dryRun bool, configPath string) (*repairer, error) {
	if configPath == "" {
		configPath = util.GetDefaultKubeConfigPath()
	}

	c, err := clientcmd.NewClusterApiClientForDefaultSearchPath(configPath, clientcmd.NewConfigOverrides())
	if err != nil {
		return nil, err
	}

	return &repairer{dryRun: dryRun,
		configPath:    configPath,
		machInterface: c.ClusterV1alpha1().Machines(v1.NamespaceDefault)}, nil
}

func (r *repairer) RepairNode() error {
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
		m, err := r.machInterface.Get(node, metav1.GetOptions{})

		if err != nil {
			glog.Info("Error retrieving machine object %v. Not taking any action on this node.", node)
			continue
		}
		if util.IsMaster(m) {
			glog.Infof("Found master node %s, skipping repair for it", m.Name)
			continue
		}
		if err := r.machInterface.Delete(node, &metav1.DeleteOptions{}); err != nil {
			return err
		}

		glog.Infof("Deleted node %s", node)

		if _, err := r.machInterface.Create(util.Copy(m)); err != nil {
			return err
		}

		glog.Infof("Recreated node %s", node)
	}

	return nil
}

func (r *repairer) getUnhealthyNodes() ([]string, error) {
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
