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
	"os"
	"time"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clusterv1 "k8s.io/kube-deploy/cluster-api/api/cluster/v1alpha1"
	"k8s.io/kube-deploy/cluster-api/client"
	"k8s.io/kube-deploy/cluster-api/util"
	"github.com/golang/glog"
)

const (
	MasterIPAttempts       = 40
	SleepSecondsPerAttempt = 5
	RetryAttempts          = 30
	DeleteAttempts         = 150
	DeleteSleepSeconds     = 5
)

func (d *deployer) createMachineCRD(machines []*clusterv1.Machine) error {
	cs, err := d.newClientSet()
	if err != nil {
		return err
	}

	success := false
	for i := 0; i <= RetryAttempts; i++ {
		if _, err = clusterv1.CreateMachinesCRD(cs); err != nil {
			glog.Infof("Failure creating Machines CRD (will retry): %v\n", err)
			time.Sleep(time.Duration(SleepSecondsPerAttempt) * time.Second)
			continue
		}
		success = true
		glog.Info("Machines CRD created successfully!")
		break
	}

	if !success {
		return fmt.Errorf("error creating Machines CRD: %v", err)
	}
	return d.createMachines(machines)
}

func (d *deployer) createMachines(machines []*clusterv1.Machine) error {
	c, err := d.newApiClient()
	if err != nil {
		return err
	}

	for _, machine := range machines {
		_, err = c.Machines().Create(machine)
		if err != nil {
			return err
		}
		glog.Infof("Added machine [%s]", machine.Name)
	}
	return nil
}

func (d *deployer) deleteMachines() error {
	c, err := d.newApiClient()
	if err != nil {
		return err
	}
	machines, err := c.Machines().List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, m := range machines.Items {
		if err := c.Machines().Delete(m.Name, &metav1.DeleteOptions{}); err != nil {
			return err
		}
		glog.Infof("Deleted machine object %s", m.Name)
	}
	return nil
}

func (d *deployer) setMasterIP(master *clusterv1.Machine) error {
	for i := 0; i < MasterIPAttempts; i++ {
		ip, err := d.actuator.GetIP(master)
		if err != nil || ip == "" {
			glog.Info("Hanging for master IP...")
			time.Sleep(time.Duration(SleepSecondsPerAttempt) * time.Second)
			continue
		}
		glog.Infof("Got master IP [%s]", ip)
		d.masterIP = ip

		return nil
	}
	return fmt.Errorf("unable to find Master IP after defined wait")
}

func (d *deployer) copyKubeConfig(master *clusterv1.Machine) error {
	for i := 0; i <= RetryAttempts; i++ {
		var config string
		var err error
		if config, err = d.actuator.GetKubeConfig(master); err != nil || config == "" {
			glog.Infof("Waiting for Kubernetes to come up...")
			time.Sleep(time.Duration(SleepSecondsPerAttempt) * time.Second)
			continue
		}

		return d.writeConfigToDisk(config)
	}
	return fmt.Errorf("timedout writing kubeconfig")
}

func (d *deployer) writeConfigToDisk(config string) error {
	filePath, err := util.GetDefaultKubeConfigPath()
	if err != nil {
		return err
	}
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	if _, err := file.WriteString(config); err != nil {
		return err
	}
	defer file.Close()

	file.Sync() // flush
	glog.Infof("wrote kubeconfig to [%s]", filePath)
	return nil
}

func (d *deployer) newClientSet() (*apiextensionsclient.Clientset, error) {
	config, err := d.getConfig()
	if err != nil {
		return nil, err
	}

	cs, err := apiextensionsclient.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return cs, nil
}

func (d *deployer) newApiClient() (*client.ClusterAPIV1Alpha1Client, error) {
	config, err := d.getConfig()
	if err != nil {
		return nil, err
	}

	c, err := client.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (d *deployer) getConfig() (*restclient.Config, error) {
	kubeconfig, err := util.GetDefaultKubeConfigPath()
	if err != nil {
		return nil, err
	}
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}
	return config, nil
}
