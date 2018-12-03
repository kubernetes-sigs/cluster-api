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

package clusterclient

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	apiv1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	tcmd "k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/clientcmd"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
	"sigs.k8s.io/cluster-api/pkg/util"

	"k8s.io/klog"
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

// Provides interaction with a cluster
type Client interface {
	GetContextNamespace() string
	Apply(string) error
	Delete(string) error
	WaitForClusterV1alpha1Ready() error
	GetClusterObjectsInNamespace(string) ([]*clusterv1.Cluster, error)
	GetClusterObject(string, string) (*clusterv1.Cluster, error)
	GetMachineDeploymentObjects() ([]*clusterv1.MachineDeployment, error)
	GetMachineDeploymentObjectsInNamespace(string) ([]*clusterv1.MachineDeployment, error)
	GetMachineSetObjects() ([]*clusterv1.MachineSet, error)
	GetMachineSetObjectsInNamespace(string) ([]*clusterv1.MachineSet, error)
	GetMachineObjects() ([]*clusterv1.Machine, error)
	GetMachineObjectsInNamespace(ns string) ([]*clusterv1.Machine, error)
	CreateClusterObject(*clusterv1.Cluster) error
	CreateMachineDeploymentObjects([]*clusterv1.MachineDeployment, string) error
	CreateMachineSetObjects([]*clusterv1.MachineSet, string) error
	CreateMachineObjects([]*clusterv1.Machine, string) error
	DeleteClusterObjectsInNamespace(string) error
	DeleteClusterObjects() error
	DeleteMachineDeploymentObjectsInNamespace(string) error
	DeleteMachineDeploymentObjects() error
	DeleteMachineSetObjectsInNamespace(string) error
	DeleteMachineSetObjects() error
	DeleteMachineObjectsInNamespace(string) error
	DeleteMachineObjects() error
	UpdateClusterObjectEndpoint(string, string, string) error
	EnsureNamespace(string) error
	DeleteNamespace(string) error
	Close() error
}

type client struct {
	clientSet       clientset.Interface
	kubeconfigFile  string
	configOverrides tcmd.ConfigOverrides
	closeFn         func() error
}

// New creates and returns a Client, the kubeconfig argument is expected to be the string represenation
// of a valid kubeconfig.
func New(kubeconfig string) (*client, error) {
	f, err := createTempFile(kubeconfig)
	if err != nil {
		return nil, err
	}
	defer ifErrRemove(&err, f)
	c, err := NewFromDefaultSearchPath(f, clientcmd.NewConfigOverrides())
	if err != nil {
		return nil, err
	}
	c.closeFn = c.removeKubeconfigFile
	return c, nil
}

func (c *client) removeKubeconfigFile() error {
	return os.Remove(c.kubeconfigFile)
}

func (c *client) EnsureNamespace(namespaceName string) error {
	clientset, err := clientcmd.NewCoreClientSetForDefaultSearchPath(c.kubeconfigFile, clientcmd.NewConfigOverrides())
	if err != nil {
		return fmt.Errorf("error creating core clientset: %v", err)
	}

	namespace := apiv1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespaceName,
		},
	}
	_, err = clientset.CoreV1().Namespaces().Create(&namespace)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func (c *client) DeleteNamespace(namespaceName string) error {
	if namespaceName == apiv1.NamespaceDefault {
		return nil
	}
	clientset, err := clientcmd.NewCoreClientSetForDefaultSearchPath(c.kubeconfigFile, clientcmd.NewConfigOverrides())
	if err != nil {
		return fmt.Errorf("error creating core clientset: %v", err)
	}

	err = clientset.CoreV1().Namespaces().Delete(namespaceName, &metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

// NewFromDefaultSearchPath creates and returns a Client.  The kubeconfigFile argument is expected to be the path to a
// valid kubeconfig file.
func NewFromDefaultSearchPath(kubeconfigFile string, overrides tcmd.ConfigOverrides) (*client, error) {
	c, err := clientcmd.NewClusterApiClientForDefaultSearchPath(kubeconfigFile, overrides)
	if err != nil {
		return nil, err
	}

	return &client{
		kubeconfigFile:  kubeconfigFile,
		clientSet:       c,
		configOverrides: overrides,
	}, nil
}

// Close frees resources associated with the cluster client
func (c *client) Close() error {
	if c.closeFn != nil {
		return c.closeFn()
	}
	return nil
}

func (c *client) Delete(manifest string) error {
	return c.kubectlDelete(manifest)
}

func (c *client) Apply(manifest string) error {
	return c.waitForKubectlApply(manifest)
}

func (c *client) GetContextNamespace() string {
	if c.configOverrides.Context.Namespace == "" {
		return apiv1.NamespaceDefault
	}
	return c.configOverrides.Context.Namespace
}

func (c *client) GetClusterObject(name, ns string) (*clusterv1.Cluster, error) {
	clustersInNamespace, err := c.GetClusterObjectsInNamespace(ns)
	if err != nil {
		return nil, err
	}
	var cluster *clusterv1.Cluster
	for _, nc := range clustersInNamespace {
		if nc.Name == name {
			cluster = nc
			break
		}
	}
	return cluster, nil
}

func (c *client) GetClusterObjectsInNamespace(namespace string) ([]*clusterv1.Cluster, error) {
	clusters := []*clusterv1.Cluster{}
	clusterlist, err := c.clientSet.ClusterV1alpha1().Clusters(namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error listing cluster objects in namespace %q: %v", namespace, err)
	}

	for i := 0; i < len(clusterlist.Items); i++ {
		clusters = append(clusters, &clusterlist.Items[i])
	}
	return clusters, nil
}

func (c *client) GetMachineDeploymentObjectsInNamespace(namespace string) ([]*clusterv1.MachineDeployment, error) {
	machineDeploymentList, err := c.clientSet.ClusterV1alpha1().MachineDeployments(namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error listing machine deployment objects in namespace %q: %v", namespace, err)
	}
	var machineDeployments []*clusterv1.MachineDeployment
	for i := 0; i < len(machineDeploymentList.Items); i++ {
		machineDeployments = append(machineDeployments, &machineDeploymentList.Items[i])
	}
	return machineDeployments, nil
}

// Deprecated API. Please do not extend or use.
func (c *client) GetMachineDeploymentObjects() ([]*clusterv1.MachineDeployment, error) {
	klog.V(2).Info("GetMachineDeploymentObjects API is deprecated, use GetMachineDeploymentObjectsInNamespace instead")
	return c.GetMachineDeploymentObjectsInNamespace(apiv1.NamespaceDefault)
}

func (c *client) GetMachineSetObjectsInNamespace(namespace string) ([]*clusterv1.MachineSet, error) {
	machineSetList, err := c.clientSet.ClusterV1alpha1().MachineSets(namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error listing machine set objects in namespace %q: %v", namespace, err)
	}
	var machineSets []*clusterv1.MachineSet
	for i := 0; i < len(machineSetList.Items); i++ {
		machineSets = append(machineSets, &machineSetList.Items[i])
	}
	return machineSets, nil
}

// Deprecated API. Please do not extend or use.
func (c *client) GetMachineSetObjects() ([]*clusterv1.MachineSet, error) {
	klog.V(2).Info("GetMachineSetObjects API is deprecated, use GetMachineSetObjectsInNamespace instead")
	return c.GetMachineSetObjectsInNamespace(apiv1.NamespaceDefault)
}

func (c *client) GetMachineObjectsInNamespace(namespace string) ([]*clusterv1.Machine, error) {
	machines := []*clusterv1.Machine{}
	machineslist, err := c.clientSet.ClusterV1alpha1().Machines(namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error listing machine objects in namespace %q: %v", namespace, err)
	}

	for i := 0; i < len(machineslist.Items); i++ {
		machines = append(machines, &machineslist.Items[i])
	}
	return machines, nil
}

// Deprecated API. Please do not extend or use.
func (c *client) GetMachineObjects() ([]*clusterv1.Machine, error) {
	klog.V(2).Info("GetMachineObjects API is deprecated, use GetMachineObjectsInNamespace instead")
	return c.GetMachineObjectsInNamespace(apiv1.NamespaceDefault)
}

func (c *client) CreateClusterObject(cluster *clusterv1.Cluster) error {
	namespace := c.GetContextNamespace()
	if cluster.Namespace != "" {
		namespace = cluster.Namespace
	}

	_, err := c.clientSet.ClusterV1alpha1().Clusters(namespace).Create(cluster)
	if err != nil {
		return fmt.Errorf("error creating cluster in namespace %v: %v", namespace, err)
	}
	return err
}

func (c *client) CreateMachineDeploymentObjects(deployments []*clusterv1.MachineDeployment, namespace string) error {
	for _, deploy := range deployments {
		// TODO: Run in parallel https://github.com/kubernetes-sigs/cluster-api/issues/258
		_, err := c.clientSet.ClusterV1alpha1().MachineDeployments(namespace).Create(deploy)
		if err != nil {
			return fmt.Errorf("error creating a machine deployment object in namespace %q: %v", namespace, err)
		}
	}
	return nil
}

func (c *client) CreateMachineSetObjects(machineSets []*clusterv1.MachineSet, namespace string) error {
	for _, ms := range machineSets {
		// TODO: Run in parallel https://github.com/kubernetes-sigs/cluster-api/issues/258
		_, err := c.clientSet.ClusterV1alpha1().MachineSets(namespace).Create(ms)
		if err != nil {
			return fmt.Errorf("error creating a machine set object in namespace %q: %v", namespace, err)
		}
	}
	return nil
}

func (c *client) CreateMachineObjects(machines []*clusterv1.Machine, namespace string) error {
	var (
		wg      sync.WaitGroup
		errOnce sync.Once
		gerr    error
	)
	// The approach to concurrency here comes from golang.org/x/sync/errgroup.
	for _, machine := range machines {
		wg.Add(1)

		go func(machine *clusterv1.Machine) {
			defer wg.Done()

			createdMachine, err := c.clientSet.ClusterV1alpha1().Machines(namespace).Create(machine)
			if err != nil {
				errOnce.Do(func() {
					gerr = fmt.Errorf("error creating a machine object in namespace %v: %v", namespace, err)
				})
				return
			}

			if err := waitForMachineReady(c.clientSet, createdMachine); err != nil {
				errOnce.Do(func() { gerr = err })
			}
		}(machine)
	}
	wg.Wait()
	return gerr
}

// Deprecated API. Please do not extend or use.
func (c *client) DeleteClusterObjects() error {
	klog.V(2).Info("DeleteClusterObjects API is deprecated, use DeleteClusterObjectsInNamespace instead")
	return c.DeleteClusterObjectsInNamespace(apiv1.NamespaceDefault)
}

func (c *client) DeleteClusterObjectsInNamespace(namespace string) error {
	err := c.clientSet.ClusterV1alpha1().Clusters(namespace).DeleteCollection(newDeleteOptions(), metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("error deleting cluster objects in namespace %q: %v", namespace, err)
	}
	err = c.waitForClusterDelete(namespace)
	if err != nil {
		return fmt.Errorf("error waiting for cluster(s) deletion to complete in namespace %q: %v", namespace, err)
	}
	return nil
}

// Deprecated API. Please do not extend or use.
func (c *client) DeleteMachineDeploymentObjects() error {
	klog.V(2).Info("DeleteMachineDeploymentObjects API is deprecated, use DeleteMachineDeploymentObjectsInNamespace instead")
	return c.DeleteMachineDeploymentObjectsInNamespace(apiv1.NamespaceDefault)
}

func (c *client) DeleteMachineDeploymentObjectsInNamespace(namespace string) error {
	err := c.clientSet.ClusterV1alpha1().MachineDeployments(namespace).DeleteCollection(newDeleteOptions(), metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("error deleting machine deployment objects in namespace %q: %v", namespace, err)
	}
	err = c.waitForMachineDeploymentsDelete(namespace)
	if err != nil {
		return fmt.Errorf("error waiting for machine deployment(s) deletion to complete in namespace %q: %v", namespace, err)
	}
	return nil
}

// Deprecated API. Please do not extend or use.
func (c *client) DeleteMachineSetObjects() error {
	klog.V(2).Info("DeleteMachineSetObjects API is deprecated, use DeleteMachineSetObjectsInNamespace instead")
	return c.DeleteMachineSetObjectsInNamespace(apiv1.NamespaceDefault)
}

func (c *client) DeleteMachineSetObjectsInNamespace(namespace string) error {
	err := c.clientSet.ClusterV1alpha1().MachineSets(namespace).DeleteCollection(newDeleteOptions(), metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("error deleting machine set objects in namespace %q: %v", namespace, err)
	}
	err = c.waitForMachineSetsDelete(namespace)
	if err != nil {
		return fmt.Errorf("error waiting for machine set(s) deletion to complete in namespace %q: %v", namespace, err)
	}
	return nil
}

// Deprecated API. Please do not extend or use.
func (c *client) DeleteMachineObjects() error {
	klog.V(2).Info("DeleteMachineObjects API is deprecated, use DeleteMachineObjectsInNamespace instead")
	return c.DeleteMachineObjectsInNamespace(apiv1.NamespaceDefault)
}

func (c *client) DeleteMachineObjectsInNamespace(namespace string) error {
	err := c.clientSet.ClusterV1alpha1().Machines(namespace).DeleteCollection(newDeleteOptions(), metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("error deleting machine objects in namespace %q: %v", namespace, err)
	}
	err = c.waitForMachinesDelete(namespace)
	if err != nil {
		return fmt.Errorf("error waiting for machine(s) deletion to complete in namespace %q: %v", namespace, err)
	}
	return nil
}

func newDeleteOptions() *metav1.DeleteOptions {
	propagationPolicy := metav1.DeletePropagationForeground
	return &metav1.DeleteOptions{
		PropagationPolicy: &propagationPolicy,
	}
}

// TODO: Test this function
func (c *client) UpdateClusterObjectEndpoint(masterIP, clusterName, namespace string) error {
	cluster, err := c.GetClusterObject(clusterName, namespace)
	if err != nil {
		return err
	}
	cluster.Status.APIEndpoints = append(cluster.Status.APIEndpoints,
		clusterv1.APIEndpoint{
			Host: masterIP,
			Port: apiServerPort,
		})
	_, err = c.clientSet.ClusterV1alpha1().Clusters(namespace).UpdateStatus(cluster)
	return err
}

func (c *client) WaitForClusterV1alpha1Ready() error {
	return waitForClusterResourceReady(c.clientSet)
}

func (c *client) waitForClusterDelete(namespace string) error {
	return util.PollImmediate(retryIntervalResourceDelete, timeoutResourceDelete, func() (bool, error) {
		klog.V(2).Infof("Waiting for cluster objects to be deleted...")
		response, err := c.clientSet.ClusterV1alpha1().Clusters(namespace).List(metav1.ListOptions{})
		if err != nil {
			return false, nil
		}
		if len(response.Items) > 0 {
			return false, nil
		}
		return true, nil
	})
}

func (c *client) waitForMachineDeploymentsDelete(namespace string) error {
	return util.PollImmediate(retryIntervalResourceDelete, timeoutResourceDelete, func() (bool, error) {
		klog.V(2).Infof("Waiting for machine deployment objects to be deleted...")
		response, err := c.clientSet.ClusterV1alpha1().MachineDeployments(namespace).List(metav1.ListOptions{})
		if err != nil {
			return false, nil
		}
		if len(response.Items) > 0 {
			return false, nil
		}
		return true, nil
	})
}

func (c *client) waitForMachineSetsDelete(namespace string) error {
	return util.PollImmediate(retryIntervalResourceDelete, timeoutResourceDelete, func() (bool, error) {
		klog.V(2).Infof("Waiting for machine set objects to be deleted...")
		response, err := c.clientSet.ClusterV1alpha1().MachineSets(namespace).List(metav1.ListOptions{})
		if err != nil {
			return false, nil
		}
		if len(response.Items) > 0 {
			return false, nil
		}
		return true, nil
	})
}

func (c *client) waitForMachinesDelete(namespace string) error {
	return util.PollImmediate(retryIntervalResourceDelete, timeoutResourceDelete, func() (bool, error) {
		klog.V(2).Infof("Waiting for machine objects to be deleted...")
		response, err := c.clientSet.ClusterV1alpha1().Machines(namespace).List(metav1.ListOptions{})
		if err != nil {
			return false, nil
		}
		if len(response.Items) > 0 {
			return false, nil
		}
		return true, nil
	})
}

func (c *client) kubectlDelete(manifest string) error {
	return c.kubectlManifestCmd("delete", manifest)
}

func (c *client) kubectlApply(manifest string) error {
	return c.kubectlManifestCmd("apply", manifest)
}

func (c *client) kubectlManifestCmd(commandName, manifest string) error {
	cmd := exec.Command("kubectl", c.buildKubectlArgs(commandName)...)
	cmd.Stdin = strings.NewReader(manifest)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("couldn't kubectl apply: %v, output: %s", err, string(out))
	}
	return nil
}

func (c *client) buildKubectlArgs(commandName string) []string {
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

func (c *client) waitForKubectlApply(manifest string) error {
	err := util.PollImmediate(retryIntervalKubectlApply, timeoutKubectlApply, func() (bool, error) {
		klog.V(2).Infof("Waiting for kubectl apply...")
		err := c.kubectlApply(manifest)
		if err != nil {
			if strings.Contains(err.Error(), "refused") {
				// Connection was refused, probably because the API server is not ready yet.
				klog.V(4).Infof("Waiting for kubectl apply... server not yet available: %v", err)
				return false, nil
			}
			if strings.Contains(err.Error(), "unable to recognize") {
				klog.V(4).Infof("Waiting for kubectl apply... api not yet available: %v", err)
				return false, nil
			}
			if strings.Contains(err.Error(), "namespaces \"default\" not found") {
				klog.V(4).Infof("Waiting for kubectl apply... default namespace not yet available: %v", err)
				return false, nil
			}
			klog.Warningf("Waiting for kubectl apply... unknown error %v", err)
			return false, err
		}

		return true, nil
	})

	return err
}

func waitForClusterResourceReady(cs clientset.Interface) error {
	deadline := time.Now().Add(timeoutResourceReady)
	err := util.PollImmediate(retryIntervalResourceReady, timeoutResourceReady, func() (bool, error) {
		klog.V(2).Info("Waiting for Cluster v1alpha resources to become available...")
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
		klog.V(2).Info("Waiting for Cluster v1alpha resources to be listable...")
		_, err := cs.ClusterV1alpha1().Clusters(apiv1.NamespaceDefault).List(metav1.ListOptions{})
		if err == nil {
			return true, nil
		}
		return false, nil
	})
}

func waitForMachineReady(cs clientset.Interface, machine *clusterv1.Machine) error {
	err := util.PollImmediate(retryIntervalResourceReady, timeoutMachineReady, func() (bool, error) {
		klog.V(2).Infof("Waiting for Machine %v to become ready...", machine.Name)
		m, err := cs.ClusterV1alpha1().Machines(machine.Namespace).Get(machine.Name, metav1.GetOptions{})
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
			klog.Warningf("Error removing file '%s': %v", path, err)
		}
	}
}
