/*
Copyright 2018 The Kubernetes Authors.

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

package clusterdeployer

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"time"

	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	tcmd "k8s.io/client-go/tools/clientcmd"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
	"sigs.k8s.io/cluster-api/pkg/clientcmd"
	"sigs.k8s.io/cluster-api/pkg/util"

	"github.com/golang/glog"
)

const (
	apiServerPort               = 443
	retryIntervalKubectlApply   = 10 * time.Second
	retryIntervalResourceReady  = 10 * time.Second
	retryIntervalResourceDelete = 10 * time.Second
	timeoutKubectlApply         = 15 * time.Minute
	timeoutResourceReady        = 15 * time.Minute
	timeoutMachineReady         = 30 * time.Minute
	timeoutResourceDelete       = 15 * time.Minute
)

type clusterClient struct {
	clientSet       clientset.Interface
	kubeconfigFile  string
	configOverrides tcmd.ConfigOverrides
	closeFn         func() error
}

// NewClusterClient creates and returns the address of a clusterClient, the kubeconfig argument is expected to be the string represenattion
// of a valid kubeconfig.
func NewClusterClient(kubeconfig string) (*clusterClient, error) {
	f, err := createTempFile(kubeconfig)
	if err != nil {
		return nil, err
	}
	defer ifErrRemove(&err, f)
	c, err := NewClusterClientFromDefaultSearchPath(f, clientcmd.NewConfigOverrides())
	if err != nil {
		return nil, err
	}
	c.closeFn = c.removeKubeconfigFile
	return c, nil
}

func (c *clusterClient) removeKubeconfigFile() error {
	return os.Remove(c.kubeconfigFile)
}

// NewClusterClientFromDefaultSearchPath creates and returns the address of a clusterClient, the kubeconfigFile argument is expected to be the path to a
// valid kubeconfig file.
func NewClusterClientFromDefaultSearchPath(kubeconfigFile string, overrides tcmd.ConfigOverrides) (*clusterClient, error) {
	c, err := clientcmd.NewClusterApiClientForDefaultSearchPath(kubeconfigFile, overrides)
	if err != nil {
		return nil, err
	}

	return &clusterClient{
		kubeconfigFile:  kubeconfigFile,
		clientSet:       c,
		configOverrides: overrides,
	}, nil
}

// Frees resources associated with the cluster client
func (c *clusterClient) Close() error {
	if c.closeFn != nil {
		return c.closeFn()
	}
	return nil
}

func (c *clusterClient) Delete(manifest string) error {
	return c.kubectlDelete(manifest)
}

func (c *clusterClient) Apply(manifest string) error {
	return c.waitForKubectlApply(manifest)
}

func (c *clusterClient) GetClusterObjects() ([]*clusterv1.Cluster, error) {
	clusters := []*clusterv1.Cluster{}
	// TODO: Iterate over all namespaces where we could have Cluster API Objects https://github.com/kubernetes-sigs/cluster-api/issues/252
	clusterlist, err := c.clientSet.ClusterV1alpha1().Clusters(apiv1.NamespaceDefault).List(metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error listing cluster objects: %v", err)
	}

	for i := 0; i < len(clusterlist.Items); i++ {
		clusters = append(clusters, &clusterlist.Items[i])
	}
	return clusters, nil
}

func (c *clusterClient) GetMachineDeploymentObjects() ([]*clusterv1.MachineDeployment, error) {
	// TODO: Iterate over all namespaces where we could have Cluster API Objects https://github.com/kubernetes-sigs/cluster-api/issues/252
	machineDeploymentList, err := c.clientSet.ClusterV1alpha1().MachineDeployments(apiv1.NamespaceDefault).List(metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error listing machine deployment objects: %v", err)
	}
	var machineDeployments []*clusterv1.MachineDeployment
	for i := 0; i < len(machineDeploymentList.Items); i++ {
		machineDeployments = append(machineDeployments, &machineDeploymentList.Items[i])
	}
	return machineDeployments, nil
}

func (c *clusterClient) GetMachineSetObjects() ([]*clusterv1.MachineSet, error) {
	// TODO: Iterate over all namespaces where we could have Cluster API Objects https://github.com/kubernetes-sigs/cluster-api/issues/252
	machineSetList, err := c.clientSet.ClusterV1alpha1().MachineSets(apiv1.NamespaceDefault).List(metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error listing machine set objects: %v", err)
	}
	var machineSets []*clusterv1.MachineSet
	for i := 0; i < len(machineSetList.Items); i++ {
		machineSets = append(machineSets, &machineSetList.Items[i])
	}
	return machineSets, nil
}

func (c *clusterClient) GetMachineObjects() ([]*clusterv1.Machine, error) {
	// TODO: Iterate over all namespaces where we could have Cluster API Objects https://github.com/kubernetes-sigs/cluster-api/issues/252
	machines := []*clusterv1.Machine{}
	machineslist, err := c.clientSet.ClusterV1alpha1().Machines(apiv1.NamespaceDefault).List(metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error listing machine objects: %v", err)
	}

	for i := 0; i < len(machineslist.Items); i++ {
		machines = append(machines, &machineslist.Items[i])
	}
	return machines, nil
}

func (c *clusterClient) CreateClusterObject(cluster *clusterv1.Cluster) error {
	// TODO: Support specific namespaces https://github.com/kubernetes-sigs/cluster-api/issues/252
	_, err := c.clientSet.ClusterV1alpha1().Clusters(apiv1.NamespaceDefault).Create(cluster)
	if err != nil {
		return fmt.Errorf("error creating cluster: %v", err)
	}
	return err
}

func (c *clusterClient) CreateMachineDeploymentObjects(deployments []*clusterv1.MachineDeployment) error {
	// TODO: Support specific namespaces https://github.com/kubernetes-sigs/cluster-api/issues/252
	for _, deploy := range deployments {
		// TODO: Run in parallel https://github.com/kubernetes-sigs/cluster-api/issues/258
		_, err := c.clientSet.ClusterV1alpha1().MachineDeployments(apiv1.NamespaceDefault).Create(deploy)
		if err != nil {
			return fmt.Errorf("error creating a machine deployment object: %v", err)
		}
	}
	return nil
}

func (c *clusterClient) CreateMachineSetObjects(machineSets []*clusterv1.MachineSet) error {
	// TODO: Support specific namespaces https://github.com/kubernetes-sigs/cluster-api/issues/252
	for _, ms := range machineSets {
		// TODO: Run in parallel https://github.com/kubernetes-sigs/cluster-api/issues/258
		_, err := c.clientSet.ClusterV1alpha1().MachineSets(apiv1.NamespaceDefault).Create(ms)
		if err != nil {
			return fmt.Errorf("error creating a machine set object: %v", err)
		}
	}
	return nil
}

func (c *clusterClient) CreateMachineObjects(machines []*clusterv1.Machine) error {
	// TODO: Support specific namespaces https://github.com/kubernetes-sigs/cluster-api/issues/252
	for _, machine := range machines {
		// TODO: Run in parallel https://github.com/kubernetes-sigs/cluster-api/issues/258
		createdMachine, err := c.clientSet.ClusterV1alpha1().Machines(apiv1.NamespaceDefault).Create(machine)
		if err != nil {
			return fmt.Errorf("error creating a machine object: %v", err)
		}
		err = waitForMachineReady(c.clientSet, createdMachine)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *clusterClient) DeleteClusterObjects() error {
	err := c.clientSet.ClusterV1alpha1().Clusters(apiv1.NamespaceDefault).DeleteCollection(newDeleteOptions(), metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("error deleting cluster objects: %v", err)
	}
	err = c.waitForClusterDelete()
	if err != nil {
		return fmt.Errorf("error waiting for cluster(s) deletion to complete: %v", err)
	}
	return nil
}

func (c *clusterClient) DeleteMachineDeploymentObjects() error {
	err := c.clientSet.ClusterV1alpha1().MachineDeployments(apiv1.NamespaceDefault).DeleteCollection(newDeleteOptions(), metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("error deleting machine deployment objects: %v", err)
	}
	err = c.waitForMachineDeploymentsDelete()
	if err != nil {
		return fmt.Errorf("error waiting for machine deployment(s) deletion to complete: %v", err)
	}
	return nil
}

func (c *clusterClient) DeleteMachineSetObjects() error {
	err := c.clientSet.ClusterV1alpha1().MachineSets(apiv1.NamespaceDefault).DeleteCollection(newDeleteOptions(), metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("error deleting machine set objects: %v", err)
	}
	err = c.waitForMachineSetsDelete()
	if err != nil {
		return fmt.Errorf("error waiting for machine set(s) deletion to complete: %v", err)
	}
	return nil
}

func (c *clusterClient) DeleteMachineObjects() error {
	err := c.clientSet.ClusterV1alpha1().Machines(apiv1.NamespaceDefault).DeleteCollection(newDeleteOptions(), metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("error deleting machine objects: %v", err)
	}
	err = c.waitForMachinesDelete()
	if err != nil {
		return fmt.Errorf("error waiting for machine(s) deletion to complete: %v", err)
	}
	return nil
}

func newDeleteOptions() *metav1.DeleteOptions {
	propogationPolicy := metav1.DeletePropagationForeground
	return &metav1.DeleteOptions{
		PropagationPolicy: &propogationPolicy,
	}
}

func (c *clusterClient) UpdateClusterObjectEndpoint(masterIP string) error {
	clusters, err := c.GetClusterObjects()
	if err != nil {
		return err
	}
	if len(clusters) != 1 {
		// TODO: Do not assume default namespace nor single cluster https://github.com/kubernetes-sigs/cluster-api/issues/252
		return fmt.Errorf("More than the one expected cluster found %v", clusters)
	}
	cluster := clusters[0]
	cluster.Status.APIEndpoints = append(cluster.Status.APIEndpoints,
		clusterv1.APIEndpoint{
			Host: masterIP,
			Port: apiServerPort,
		})
	_, err = c.clientSet.ClusterV1alpha1().Clusters(apiv1.NamespaceDefault).UpdateStatus(cluster)
	return err
}

func (c *clusterClient) WaitForClusterV1alpha1Ready() error {
	return waitForClusterResourceReady(c.clientSet)
}

func (c *clusterClient) waitForClusterDelete() error {
	return util.PollImmediate(retryIntervalResourceDelete, timeoutResourceDelete, func() (bool, error) {
		glog.V(2).Infof("Waiting for cluster objects to be deleted...")
		response, err := c.clientSet.ClusterV1alpha1().Clusters(apiv1.NamespaceDefault).List(metav1.ListOptions{})
		if err != nil {
			return false, nil
		}
		if len(response.Items) > 0 {
			return false, nil
		}
		return true, nil
	})
}

func (c *clusterClient) waitForMachineDeploymentsDelete() error {
	return util.PollImmediate(retryIntervalResourceDelete, timeoutResourceDelete, func() (bool, error) {
		glog.V(2).Infof("Waiting for machine deployment objects to be deleted...")
		response, err := c.clientSet.ClusterV1alpha1().MachineDeployments(apiv1.NamespaceDefault).List(metav1.ListOptions{})
		if err != nil {
			return false, nil
		}
		if len(response.Items) > 0 {
			return false, nil
		}
		return true, nil
	})
}

func (c *clusterClient) waitForMachineSetsDelete() error {
	return util.PollImmediate(retryIntervalResourceDelete, timeoutResourceDelete, func() (bool, error) {
		glog.V(2).Infof("Waiting for machine set objects to be deleted...")
		response, err := c.clientSet.ClusterV1alpha1().MachineSets(apiv1.NamespaceDefault).List(metav1.ListOptions{})
		if err != nil {
			return false, nil
		}
		if len(response.Items) > 0 {
			return false, nil
		}
		return true, nil
	})
}

func (c *clusterClient) waitForMachinesDelete() error {
	return util.PollImmediate(retryIntervalResourceDelete, timeoutResourceDelete, func() (bool, error) {
		glog.V(2).Infof("Waiting for machine objects to be deleted...")
		response, err := c.clientSet.ClusterV1alpha1().Machines(apiv1.NamespaceDefault).List(metav1.ListOptions{})
		if err != nil {
			return false, nil
		}
		if len(response.Items) > 0 {
			return false, nil
		}
		return true, nil
	})
}

func (c *clusterClient) kubectlDelete(manifest string) error {
	return c.kubectlManifestCmd("delete", manifest)
}

func (c *clusterClient) kubectlApply(manifest string) error {
	return c.kubectlManifestCmd("apply", manifest)
}

func (c *clusterClient) kubectlManifestCmd(commandName, manifest string) error {
	cmd := exec.Command("kubectl", c.buildKubectlArgs(commandName)...)
	cmd.Stdin = strings.NewReader(manifest)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("couldn't kubectl apply: %v, output: %s", err, string(out))
	}
	return nil
}

func (c *clusterClient) buildKubectlArgs(commandName string) []string {
	args := []string{commandName}
	if c.kubeconfigFile != "" {
		args = append(args, "--kubeconfig", c.kubeconfigFile)
	}
	if c.configOverrides.Context.Cluster != "" {
		args = append(args, "--cluster", c.configOverrides.Context.Cluster)
	}
	if c.configOverrides.Context.Namespace != "" {
		args = append(args, "--namespace", c.configOverrides.Context.Namespace)
	}
	if c.configOverrides.Context.AuthInfo != "" {
		args = append(args, "--user", c.configOverrides.Context.AuthInfo)
	}
	return append(args, "-f", "-")
}

func (c *clusterClient) waitForKubectlApply(manifest string) error {
	err := util.PollImmediate(retryIntervalKubectlApply, timeoutKubectlApply, func() (bool, error) {
		glog.V(2).Infof("Waiting for kubectl apply...")
		err := c.kubectlApply(manifest)
		if err != nil {
			if strings.Contains(err.Error(), "refused") {
				// Connection was refused, probably because the API server is not ready yet.
				glog.V(4).Infof("Waiting for kubectl apply... server not yet available: %v", err)
				return false, nil
			}
			if strings.Contains(err.Error(), "unable to recognize") {
				glog.V(4).Infof("Waiting for kubectl apply... api not yet available: %v", err)
				return false, nil
			}
			if strings.Contains(err.Error(), "namespaces \"default\" not found") {
				glog.V(4).Infof("Waiting for kubectl apply... default namespace not yet available: %v", err)
				return false, nil
			}
			return false, err
		}

		return true, nil
	})

	return err
}

func waitForClusterResourceReady(cs clientset.Interface) error {
	deadline := time.Now().Add(timeoutResourceReady)
	err := util.PollImmediate(retryIntervalResourceReady, timeoutResourceReady, func() (bool, error) {
		glog.V(2).Info("Waiting for Cluster v1alpha resources to become available...")
		_, err := cs.Discovery().ServerResourcesForGroupVersion("cluster.k8s.io/v1alpha1")
		if err == nil {
			return true, nil
		}
		return false, nil
	})

	if err != nil {
		return err
	}
	timeout := time.Until(deadline)
	return util.PollImmediate(retryIntervalResourceReady, timeout, func() (bool, error) {
		glog.V(2).Info("Waiting for Cluster v1alpha resources to be listable...")
		_, err := cs.ClusterV1alpha1().Clusters(apiv1.NamespaceDefault).List(metav1.ListOptions{})
		if err == nil {
			return true, nil
		}
		return false, nil
	})
}

func waitForMachineReady(cs clientset.Interface, machine *clusterv1.Machine) error {
	err := util.PollImmediate(retryIntervalResourceReady, timeoutMachineReady, func() (bool, error) {
		glog.V(2).Infof("Waiting for Machine %v to become ready...", machine.Name)
		m, err := cs.ClusterV1alpha1().Machines(apiv1.NamespaceDefault).Get(machine.Name, metav1.GetOptions{})
		if err != nil {
			return false, nil
		}

		// TODO: update once machine controllers have a way to indicate a machine has been provisoned. https://github.com/kubernetes-sigs/cluster-api/issues/253
		// Seeing a node cannot be purely relied upon because the provisioned master will not be registering with
		// the stack that provisions it.
		ready := m.Status.NodeRef != nil || len(m.Annotations) > 0
		return ready, nil
	})

	return err
}

func createTempFile(contents string) (string, error) {
	f, err := ioutil.TempFile("", "")
	if err != nil {
		return "", err
	}
	defer ifErrRemove(&err, f.Name())
	if err = f.Close(); err != nil {
		return "", err
	}
	err = ioutil.WriteFile(f.Name(), []byte(contents), 0644)
	if err != nil {
		return "", err
	}
	return f.Name(), nil
}

func ifErrRemove(pErr *error, path string) {
	if *pErr != nil {
		if err := os.Remove(path); err != nil {
			glog.Warningf("Error removing file '%v': %v", err)
		}
	}
}
