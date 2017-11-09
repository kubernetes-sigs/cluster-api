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

	clusterv1 "k8s.io/kube-deploy/cluster-api/api/cluster/v1alpha1"
	"k8s.io/kube-deploy/cluster-api/cloud"
	"k8s.io/kube-deploy/cluster-api/util"
	"github.com/golang/glog"
)

type deployer struct {
	token      string
	masterIP   string
	configPath string
	actuator   cloud.MachineActuator
}

func NewDeployer(provider string) *deployer {
	token := util.RandomToken()
	masterIP := "masterIP"

	a, err := cloud.NewMachineActuator(provider, token, masterIP)
	if err != nil {
		glog.Exit(err)
	}
	return &deployer{
		token:    token,
		masterIP: masterIP,
		actuator: a,
	}
}

// CreateCluster uses GCP APIs to create cluster
func (d *deployer) CreateCluster(c *clusterv1.Cluster, machines []*clusterv1.Machine, enableMachineController bool) error {
	master := util.GetMaster(machines)
	if master == nil {
		return fmt.Errorf("error creating master vm, no master found")
	}

	if err := d.actuator.Create(master); err != nil {
		return err
	}
	glog.Infof("Created master %s", master.Name)

	if err := d.setMasterIP(master); err != nil {
		return fmt.Errorf("unable to get master IP: %v", err)
	}

	if err := d.copyKubeConfig(master); err != nil {
		return fmt.Errorf("unable to write kubeconfig: %v", err)
	}

	if err := d.createMachineCRD(machines); err != nil {
		return err
	}

	if enableMachineController {
		if err := d.actuator.CreateMachineController(machines); err != nil {
			return err
		}
	}

	glog.Infof("The [%s] cluster has been created successfully!", c.Name)
	glog.Info("You can now `kubectl get nodes`")

	return nil
}

func (d *deployer) DeleteCluster(c *clusterv1.Cluster, machines []*clusterv1.Machine,) error {
	if err := d.deleteMachineCRDs(); err != nil {
		return err
	}
	master := util.GetMaster(machines)
	if master == nil {
		return fmt.Errorf("error deleting master vm, no master found")
	}

	if err := d.actuator.Delete(master); err != nil {
		return err
	}
	glog.Infof("Deletion successful")
	return nil
}
