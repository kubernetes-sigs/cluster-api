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

	"github.com/ghodss/yaml"
	"github.com/golang/glog"
	"k8s.io/api/core/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func (d *deployer) createClusterCRD() error {
	cs, err := d.newClientSet()
	if err != nil {
		return err
	}

	success := false
	for i := 0; i <= RetryAttempts; i++ {
		if _, err = clusterv1.CreateClustersCRD(cs); err != nil {
			glog.Info("Failure creating Clusters CRD (will retry).")
			time.Sleep(SleepSecondsPerAttempt * time.Second)
			continue
		}
		success = true
		glog.Info("Clusters CRD created succuessfully!")
		break
	}

	if !success {
		return fmt.Errorf("error creating Clusters CRD: %v", err)
	}
	return nil
}

func (d *deployer) createMachineCRD() error {
	cs, err := d.newClientSet()
	if err != nil {
		return err
	}

	success := false
	for i := 0; i <= RetryAttempts; i++ {
		if _, err = clusterv1.CreateMachinesCRD(cs); err != nil {
			glog.Info("Failure creating Machines CRD (will retry).")
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
	return nil
}

func (d *deployer) createCluster(cluster *clusterv1.Cluster) error {
	c, err := d.newApiClient()
	if err != nil {
		return err
	}

	_, err = c.Clusters().Create(cluster)
	return err
}

func (d *deployer) createMachines(machines []*clusterv1.Machine) error {
	c, err := d.newApiClient()
	if err != nil {
		return err
	}

	for _, machine := range machines {
		m, err := c.Machines().Create(machine)
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
	c, err := d.newApiClient()
	if err != nil {
		return err
	}
	machines, err := c.Machines().List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, m := range machines.Items {
		if err := d.delete(c, m.Name); err != nil {
			return err
		}
		glog.Infof("Deleted machine object %s", m.Name)
	}
	return nil
}

func (d *deployer) deleteMachine(name string) error {
	c, err := d.newApiClient()
	if err != nil {
		return err
	}
	return d.delete(c, name)
}

func (d *deployer) delete(c *client.ClusterAPIV1Alpha1Client, name string) error {
	// TODO  https://github.com/kubernetes/kube-deploy/issues/390
	return c.Machines().Delete(name, &metav1.DeleteOptions{})
}

func (d *deployer) getMachine(name string) (*clusterv1.Machine, error) {
	c, err := d.newApiClient()
	if err != nil {
		return nil, err
	}
	machine, err := c.Machines().Get(name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return machine, nil
}

func (d *deployer) updateMachine(machine *clusterv1.Machine) error {
	c, err := d.newApiClient()
	if err != nil {
		return err
	}

	if _, err := c.Machines().Update(machine); err != nil {
		return err
	}
	return nil
}

func (d *deployer) listMachines() ([]*clusterv1.Machine, error) {
	c, err := d.newApiClient()
	if err != nil {
		return nil, err
	}

	machines, err := c.Machines().List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return util.MachineP(machines.Items), nil
}

func (d *deployer) getMasterIP(master *clusterv1.Machine) (string, error) {
	for i := 0; i < MasterIPAttempts; i++ {
		ip, err := d.actuator.GetIP(master)
		if err != nil || ip == "" {
			glog.Info("Hanging for master IP...")
			time.Sleep(time.Duration(SleepSecondsPerAttempt) * time.Second)
			continue
		}
		return ip, nil
	}
	return "", fmt.Errorf("unable to find Master IP after defined wait")
}

func (d *deployer) getUnhealthyNodes() ([]string, error) {
	nodeList := &v1.NodeList{}
	out := util.ExecCommand("kubectl", "get", "nodes", "-o=yaml")
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
	config, err := clientcmd.BuildConfigFromFlags("", d.configPath)
	if err != nil {
		return nil, err
	}
	return config, nil
}

// Make sure you successfully call setMasterIp first.
func (d *deployer) waitForApiserver(master string, timeout time.Duration) error {
	endpoint := fmt.Sprintf("https://%s/healthz", master)

	// Skip certificate validation since we're only looking for signs of
	// health, and we're not going to have the CA in our default chain.
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}

	startTime := time.Now()

	var err error
	var resp *http.Response
	for time.Now().Sub(startTime) < timeout {
		resp, err = client.Get(endpoint)
		if err == nil && resp.StatusCode == 200 {
			return nil
		}
		time.Sleep(1 * time.Second)
	}

	return err
}
