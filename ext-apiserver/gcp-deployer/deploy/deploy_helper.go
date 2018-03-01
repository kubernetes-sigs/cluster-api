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
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/golang/glog"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	clusterv1 "k8s.io/kube-deploy/ext-apiserver/pkg/apis/cluster/v1alpha1"
	"k8s.io/kube-deploy/ext-apiserver/util"
)

const (
	MasterIPAttempts       = 40
	SleepSecondsPerAttempt = 5
	ServiceAccountNs       = "kube-system"
	ServiceAccountName     = "default"
)

func (d *deployer) createCluster(c *clusterv1.Cluster, machines []*clusterv1.Machine, vmCreated *bool) error {
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

	if err := d.machineDeployer.Create(c, master); err != nil {
		return err
	}

	*vmCreated = true
	glog.Infof("Created master %s", master.GetName())

	masterIP, err := d.getMasterIP(master)
	if err != nil {
		return fmt.Errorf("unable to get master IP: %v", err)
	}

	if err := d.copyKubeConfig(master); err != nil {
		return fmt.Errorf("unable to write kubeconfig: %v", err)
	}

	if err := d.waitForApiserver(masterIP); err != nil {
		return fmt.Errorf("apiserver never came up: %v", err)
	}

	if err := d.initApiClient(); err != nil {
		return err
	}

	if err := d.waitForServiceAccount(); err != nil {
		return fmt.Errorf("service account %s/%s not found: %v", ServiceAccountNs, ServiceAccountName, err)
	}

	glog.Info("Deploying the addon apiserver and controller manager...")
	if err := d.machineDeployer.CreateMachineController(c, machines); err != nil {
		return fmt.Errorf("can't create machine controller: %v", err)
	}

	if err := d.waitForClusterResourceReady(); err != nil {
		return err
	}

	c, err = d.client.Clusters(apiv1.NamespaceDefault).Create(c)
	if err != nil {
		return err
	}

	c.Status.APIEndpoints = append(c.Status.APIEndpoints,
		clusterv1.APIEndpoint{
			Host: masterIP,
			Port: 443,
		})
	if _, err := d.client.Clusters(apiv1.NamespaceDefault).UpdateStatus(c); err != nil {
		return err
	}

	if err := d.createMachines(machines); err != nil {
		return err
	}
	return nil
}

func (d *deployer) waitForClusterResourceReady() error {
	err := util.Poll(500*time.Millisecond, 120*time.Second, func() (bool, error) {
		_, err := d.clientSet.Discovery().ServerResourcesForGroupVersion("cluster.k8s.io/v1alpha1")
		if err == nil {
			return true, nil
		}
		return false, nil
	})

	return err
}

func (d *deployer) createMachines(machines []*clusterv1.Machine) error {
	for _, machine := range machines {
		m, err := d.client.Machines(apiv1.NamespaceDefault).Create(machine)
		if err != nil {
			return err
		}
		glog.Infof("Added machine [%s]", m.Name)
	}
	return nil
}

func (d *deployer) createMachine(m *clusterv1.Machine) error {
	return d.createMachines([]*clusterv1.Machine{m})
}

func (d *deployer) deleteAllMachines() error {
	machines, err := d.client.Machines(apiv1.NamespaceDefault).List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, m := range machines.Items {
		if !util.IsMaster(&m) {
			if err := d.delete(m.Name); err != nil {
				return err
			}
			glog.Infof("Deleted machine object %s", m.Name)
		}
	}
	return nil
}

func (d *deployer) delete(name string) error {
	err := d.client.Machines(apiv1.NamespaceDefault).Delete(name, &metav1.DeleteOptions{})
	if err != nil {
		return err
	}
	err = util.Poll(500*time.Millisecond, 120*time.Second, func() (bool, error) {
		if _, err = d.client.Machines(apiv1.NamespaceDefault).Get(name, metav1.GetOptions{}); err == nil {
			return false, nil
		}
		return true, nil
	})
	return err
}

func (d *deployer) listMachines() ([]*clusterv1.Machine, error) {
	machines, err := d.client.Machines(apiv1.NamespaceDefault).List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return util.MachineP(machines.Items), nil
}

func (d *deployer) getCluster() (*clusterv1.Cluster, error) {
	clusters, err := d.client.Clusters(apiv1.NamespaceDefault).List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	if len(clusters.Items) != 1 {
		return nil, fmt.Errorf("cluster object count != 1")
	}
	return &clusters.Items[0], nil
}

func (d *deployer) getMasterIP(master *clusterv1.Machine) (string, error) {
	for i := 0; i < MasterIPAttempts; i++ {
		ip, err := d.machineDeployer.GetIP(master)
		if err != nil || ip == "" {
			glog.Info("Hanging for master IP...")
			time.Sleep(time.Duration(SleepSecondsPerAttempt) * time.Second)
			continue
		}
		return ip, nil
	}
	return "", fmt.Errorf("unable to find Master IP after defined wait")
}

func (d *deployer) copyKubeConfig(master *clusterv1.Machine) error {
	writeErr := util.Retry(func() (bool, error) {
		glog.Infof("Waiting for Kubernetes to come up...")
		config, err := d.machineDeployer.GetKubeConfig(master)
		if err != nil {
			glog.Errorf("Error while retriving kubeconfig %s", err)
			return false, err
		}
		if config == "" {
			return false, nil
		}
		glog.Infof("Kubernetes is up.. Writing kubeconfig to disk.")
		err = d.writeConfigToDisk(config)
		return (err == nil), nil
	}, 5)

	if writeErr != nil {
		return fmt.Errorf("timedout writing kubeconfig: %s", writeErr)
	}
	return nil
}

func (d *deployer) initApiClient() error {
	c, err := util.NewClientSet(d.configPath)
	if err != nil {
		return err
	}
	d.clientSet = c
	d.client = c.ClusterV1alpha1()
	return nil

}
func (d *deployer) writeConfigToDisk(config string) error {
	file, err := os.Create(d.configPath)
	if err != nil {
		return err
	}
	if _, err := file.WriteString(config); err != nil {
		return err
	}
	defer file.Close()

	file.Sync() // flush
	glog.Infof("wrote kubeconfig to [%s]", d.configPath)
	return nil
}

// Make sure you successfully call setMasterIp first.
func (d *deployer) waitForApiserver(master string) error {
	endpoint := fmt.Sprintf("https://%s/healthz", master)

	// Skip certificate validation since we're only looking for signs of
	// health, and we're not going to have the CA in our default chain.
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}

	waitErr := util.Retry(func() (bool, error) {
		glog.Info("Waiting for apiserver to become healthy...")
		resp, err := client.Get(endpoint)
		return (err == nil && resp.StatusCode == 200), nil
	}, 3)

	if waitErr != nil {
		glog.Errorf("Error waiting for apiserver: %s", waitErr)
		return waitErr
	}
	return nil
}

// Make sure the default service account in kube-system namespace exists.
func (d *deployer) waitForServiceAccount() error {
	client, err := util.NewKubernetesClient(d.configPath)
	if err != nil {
		return err
	}

	waitErr := util.Retry(func() (bool, error) {
		glog.Info("Waiting for the service account to exist...")
		_, err = client.CoreV1().ServiceAccounts(ServiceAccountNs).Get(ServiceAccountName, metav1.GetOptions{})
		return (err == nil), nil
	}, 5)

	if waitErr != nil {
		glog.Errorf("Error waiting for service account: %s", waitErr)
		return waitErr
	}
	return nil
}
