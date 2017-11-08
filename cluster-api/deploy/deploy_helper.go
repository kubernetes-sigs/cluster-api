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
	"log"
	"os"
	"time"

	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clusterv1 "k8s.io/kube-deploy/cluster-api/api/cluster/v1alpha1"
	"k8s.io/kube-deploy/cluster-api/client"
	"k8s.io/kube-deploy/cluster-api/util"
)

const (
	MasterIPAttempts       = 40
	SleepSecondsPerAttempt = 5
	RetryAttempts          = 30
	DeleteAttempts         = 150
	DeleteSleepSeconds     = 5
)

func (d *deployer) createMachineCRD(machines []*clusterv1.Machine) error {
	log.Print("Creating Machines CRD...\n")
	config, err := getConfig()
	cs, err := clientset(config)
	if err != nil {
		return err
	}

	success := false
	for i := 0; i <= RetryAttempts; i++ {
		if _, err = clusterv1.CreateMachinesCRD(cs); err != nil {
			log.Printf("Failure creating Machines CRD (will retry): %v\n", err)
			time.Sleep(time.Duration(SleepSecondsPerAttempt) * time.Second)
			continue
		}
		success = true
		log.Print("Machines CRD created successfully!")
		break
	}

	if !success {
		return fmt.Errorf("error creating Machines CRD: %v", err)
	}

	client, err := client.NewForConfig(config)
	if err != nil {
		return err
	}

	for _, machine := range machines {
		_, err = client.Machines().Create(machine)
		if err != nil {
			return err
		}
		log.Printf("Added machine [%s]", machine.Name)
	}

	return nil

}

func (d *deployer) setMasterIP(master *clusterv1.Machine) error {
	for i := 0; i < MasterIPAttempts; i++ {
		ip, err := d.actuator.GetIP(master)
		if err != nil || ip == "" {
			log.Printf("Hanging for master IP...")
			time.Sleep(time.Duration(SleepSecondsPerAttempt) * time.Second)
			continue
		}
		log.Printf("Got master IP [%s]", ip)
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
			log.Print("Waiting for Kubernetes to come up...")
			time.Sleep(time.Duration(SleepSecondsPerAttempt) * time.Second)
			continue
		}
		//log.Printf("Got kubeconfig [%s]", config)

		return d.writeConfigToDisk(config)
	}
	return fmt.Errorf("timedout writing kubeconfig")
}

func (d *deployer) writeConfigToDisk(config string) error {
	filePath, err := util.GetKubeConfigPath()
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
	log.Printf("wrote kubeconfig to [%s]", filePath)
	return nil
}

func clientset(config *restclient.Config) (*apiextensionsclient.Clientset, error) {
	clientset, err := apiextensionsclient.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return clientset, nil
}

func getConfig() (*restclient.Config, error) {
	kubeconfig, err := util.GetKubeConfigPath()
	if err != nil {
		return nil, err
	}
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}
	return config, nil
}
