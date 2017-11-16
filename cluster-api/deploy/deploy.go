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

package deploy

import (
	"fmt"
	"time"

	"github.com/golang/glog"
	clusterv1 "k8s.io/kube-deploy/cluster-api/api/cluster/v1alpha1"
	"k8s.io/kube-deploy/cluster-api/cloud"
	"k8s.io/kube-deploy/cluster-api/util"
)

type deployer struct {
	token      string
	configPath string
	actuator   cloud.MachineActuator
}

//it takes path for kubeconfig file.
func NewDeployer(provider string, configPath string) *deployer {
	token := util.RandomToken()
	if configPath == "" {
		configPath = util.GetDefaultKubeConfigPath()
	}
	a, err := cloud.NewMachineActuator(provider, token)
	if err != nil {
		glog.Exit(err)
	}
	return &deployer{
		token:      token,
		actuator:   a,
		configPath: configPath,
	}
}

// CreateCluster uses GCP APIs to create cluster
func (d *deployer) CreateCluster(c *clusterv1.Cluster, machines []*clusterv1.Machine) error {
	if c.GetName() == "" {
		return fmt.Errorf("cluster name must be specified for cluster creation")
	}
	master := util.GetMaster(machines)
	if master == nil {
		return fmt.Errorf("master spec must be provided for cluster creation")
	}

	if master.GetName() == "" && master.GetGenerateName() == "" {
		return fmt.Errorf("master name must be specified for cluster creation")
	}

	if master.GetName() == "" {
		master.Name = master.GetGenerateName() + c.GetName()
	}

	glog.Infof("Starting cluster creation %s", c.GetName())

	glog.Infof("Starting master creation %s", master.GetName())

	if err := d.actuator.Create(c, master); err != nil {
		return err
	}
	glog.Infof("Created master %s", master.GetName())

	masterIP, err := d.getMasterIP(master)
	if err != nil {
		return fmt.Errorf("unable to get master IP: %v", err)
	}

	c.Status.APIEndpoints = append(c.Status.APIEndpoints,
		clusterv1.APIEndpoint{
			Host: masterIP,
			Port: 443,
		})

	if err := d.copyKubeConfig(master); err != nil {
		return fmt.Errorf("unable to write kubeconfig: %v", err)
	}

	glog.Info("Waiting for apiserver to become healthy...")
	if err := d.waitForApiserver(masterIP, 1*time.Minute); err != nil {
		return fmt.Errorf("apiserver never came up: %v", err)
	}

	if err := d.createClusterCRD(); err != nil {
		return err
	}

	if err := d.createCluster(c); err != nil {
		return err
	}

	if err := d.createMachineCRD(); err != nil {
		return err
	}

	if err := d.createMachines(machines); err != nil {
		return err
	}

	glog.Info("Starting the machine controller...")
	if err := d.actuator.CreateMachineController(machines); err != nil {
		return fmt.Errorf("can't create machine controller: %v", err)
	}

	glog.Infof("The [%s] cluster has been created successfully!", c.Name)
	glog.Info("You can now `kubectl get nodes`")

	return nil
}

// CreateCluster uses GCP APIs to create cluster
func (d *deployer) AddNodes(machines []*clusterv1.Machine) error {
	if err := d.createMachines(machines); err != nil {
		return err
	}
	return nil
}

func (d *deployer) RepairNode(dryRun bool) error {
	nodes, err := d.getUnhealthyNodes()
	if err != nil {
		return err
	}
	if len(nodes) > 0 {
		glog.Infof("found unhealthy nodes: %v", nodes)
	} else {
		glog.Info("All nodes are healthy")
		return nil
	}

	if dryRun {
		glog.Info("Running in dry run mode. Not taking any action")
		return nil
	}

	for _, node := range nodes {
		m, err := d.getMachine(node)
		if err != nil {
			glog.Info("Error retrieving machine object %s. Not taking any action on this node.", node)
			continue
		}
		if err := d.deleteMachine(m.Name); err != nil {
			return err
		}

		if err := d.createMachine(util.Copy(m)); err != nil {
			return err
		}
		glog.Infof("Recreated node %s", node)
	}

	return nil
}

func (d *deployer) DeleteCluster() error {
	machines, err := d.listMachines()
	if err != nil {
		return err
	}

	master := util.GetMaster(machines)
	if master == nil {
		return fmt.Errorf("error deleting master vm, no master found")
	}

	glog.Info("Deleting machine objects")
	if err := d.deleteAllMachines(); err != nil {
		return err
	}

	glog.Infof("Deleting mater vm %s", master.Name)
	if err := d.actuator.Delete(master); err != nil {
		return err
	}

	glog.Info("Running post delete operations")
	if err := d.actuator.PostDelete(machines); err != nil {
		return err
	}
	glog.Infof("Deletion successful")
	return nil
}
