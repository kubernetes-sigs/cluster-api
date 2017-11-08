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
	"k8s.io/kube-deploy/cluster-api/api"
	"k8s.io/kube-deploy/cluster-api/api/machines/v1alpha1"
	"k8s.io/kube-deploy/cluster-api/util"
	"k8s.io/kube-deploy/cluster-api/cloud"
	"k8s.io/kube-deploy/cluster-api/cloud/google"
	"log"
)

type deployer struct {
	token string
	masterIP string
	configPath string
	actuator cloud.MachineActuator
}

func NewDeployer() *deployer {
	token := util.RandomToken()
	a, err := google.NewMachineActuator(token, "mastetrip")
	if err != nil {
		log.Fatal(err)
	}
	return &deployer{
		token: token,
		masterIP: "mastetrip",
		actuator: a,
	}
}
// CreateCluster uses GCP APIs to create cluster
func (d *deployer) CreateCluster(c *api.Cluster, machines []v1alpha1.Machine, enableMachineController bool) error {

	master := util.GetMaster(machines)
	if master == nil {
		return fmt.Errorf("error creating master vm, no master found")
	}

	if err := d.actuator.Create(master); err != nil {
		return err
	}
	log.Printf("Created master %s", master.Name)


	if err := d.setMasterIP(master); err != nil {
		return fmt.Errorf("unable to get master IP: %v", err)
	}

	if err := d.copyKubeConfig(master); err != nil {
		return fmt.Errorf("unable to write kubeconfig: %v", err)
	}

	if err := d.createMachineCRD(machines); err != nil {
		return err
	}


	if enableMachineController && c.Spec.Cloud == "google" {
		if err := google.CreateMachineControllerServiceAccount(c.Spec.Project); err != nil {
			return err
		}
		if err := google.CreateMachineControllerPod(d.token); err != nil {
			return err
		}
	}

	log.Printf("The [%s] cluster has been created successfully!", c.Name)
	log.Print("You can now `kubectl get nodes`")

	return nil
}


func (d *deployer) DeleteCluster(c *api.Cluster) error {
	return fmt.Errorf("not implemented yet")

}
