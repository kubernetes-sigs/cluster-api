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
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	apiv1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth" // nolint
	tcmd "k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/clientcmd"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
	"sigs.k8s.io/cluster-api/pkg/util"
)

const (
	defaultAPIServerPort        = "443"
	retryIntervalKubectlApply   = 10 * time.Second
	retryIntervalResourceReady  = 10 * time.Second
	retryIntervalResourceDelete = 10 * time.Second
	timeoutKubectlApply         = 15 * time.Minute
	timeoutResourceReady        = 15 * time.Minute
	timeoutMachineReady         = 30 * time.Minute
	timeoutResourceDelete       = 15 * time.Minute
	machineClusterLabelName     = "cluster.k8s.io/cluster-name"
)

// Provides interaction with a cluster
type Client interface {
	Apply(string) error
	Close() error
	CreateClusterObject(*clusterv1.Cluster) error
	CreateMachineClass(*clusterv1.MachineClass) error
	CreateMachineDeployments([]*clusterv1.MachineDeployment, string) error
	CreateMachineSets([]*clusterv1.MachineSet, string) error
	CreateMachines([]*clusterv1.Machine, string) error
	Delete(string) error
	DeleteClusters(string) error
	DeleteNamespace(string) error
	DeleteMachineClasses(string) error
	DeleteMachineClass(namespace, name string) error
	DeleteMachineDeployments(string) error
	DeleteMachineSets(string) error
	DeleteMachines(string) error
	ForceDeleteCluster(namespace, name string) error
	ForceDeleteMachine(namespace, name string) error
	ForceDeleteMachineSet(namespace, name string) error
	ForceDeleteMachineDeployment(namespace, name string) error
	EnsureNamespace(string) error
	GetClusters(string) ([]*clusterv1.Cluster, error)
	GetCluster(string, string) (*clusterv1.Cluster, error)
	GetContextNamespace() string
	GetMachineClasses(namespace string) ([]*clusterv1.MachineClass, error)
	GetMachineDeployment(namespace, name string) (*clusterv1.MachineDeployment, error)
	GetMachineDeploymentsForCluster(*clusterv1.Cluster) ([]*clusterv1.MachineDeployment, error)
	GetMachineDeployments(string) ([]*clusterv1.MachineDeployment, error)
	GetMachineSet(namespace, name string) (*clusterv1.MachineSet, error)
	GetMachineSets(namespace string) ([]*clusterv1.MachineSet, error)
	GetMachineSetsForCluster(*clusterv1.Cluster) ([]*clusterv1.MachineSet, error)
	GetMachineSetsForMachineDeployment(*clusterv1.MachineDeployment) ([]*clusterv1.MachineSet, error)
	GetMachines(namespace string) ([]*clusterv1.Machine, error)
	GetMachinesForCluster(*clusterv1.Cluster) ([]*clusterv1.Machine, error)
	GetMachinesForMachineSet(*clusterv1.MachineSet) ([]*clusterv1.Machine, error)
	ScaleStatefulSet(namespace, name string, scale int32) error
	WaitForClusterV1alpha1Ready() error
	UpdateClusterObjectEndpoint(string, string, string) error
	WaitForResourceStatuses() error
}

type client struct {
	clientSet       clientset.Interface
	kubeconfigFile  string
	configOverrides tcmd.ConfigOverrides
	closeFn         func() error
}

// New creates and returns a Client, the kubeconfig argument is expected to be the string representation
// of a valid kubeconfig.
func New(kubeconfig string) (*client, error) { //nolint
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
		return errors.Wrap(err, "error creating core clientset")
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

func (c *client) ScaleStatefulSet(ns string, name string, scale int32) error {
	clientset, err := clientcmd.NewCoreClientSetForDefaultSearchPath(c.kubeconfigFile, clientcmd.NewConfigOverrides())
	if err != nil {
		return errors.Wrap(err, "error creating core clientset")
	}

	_, err = clientset.AppsV1().StatefulSets(ns).UpdateScale(name, &autoscalingv1.Scale{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
		Spec: autoscalingv1.ScaleSpec{
			Replicas: scale,
		},
	})
	if err != nil && !apierrors.IsNotFound(err) {
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
		return errors.Wrap(err, "error creating core clientset")
	}

	err = clientset.CoreV1().Namespaces().Delete(namespaceName, &metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

// NewFromDefaultSearchPath creates and returns a Client.  The kubeconfigFile argument is expected to be the path to a
// valid kubeconfig file.
func NewFromDefaultSearchPath(kubeconfigFile string, overrides tcmd.ConfigOverrides) (*client, error) { //nolint
	c, err := clientcmd.NewClusterAPIClientForDefaultSearchPath(kubeconfigFile, overrides)
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

func (c *client) GetCluster(name, ns string) (*clusterv1.Cluster, error) {
	clustersInNamespace, err := c.GetClusters(ns)
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

// ForceDeleteCluster removes the finalizer for a Cluster prior to deleting, this is used during pivot
func (c *client) ForceDeleteCluster(namespace, name string) error {
	cluster, err := c.clientSet.ClusterV1alpha1().Clusters(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "error getting cluster %s/%s", namespace, name)
	}

	cluster.ObjectMeta.SetFinalizers([]string{})

	if _, err := c.clientSet.ClusterV1alpha1().Clusters(namespace).Update(cluster); err != nil {
		return errors.Wrapf(err, "error removing finalizer on cluster %s/%s", namespace, name)
	}

	if err := c.clientSet.ClusterV1alpha1().Clusters(namespace).Delete(name, &metav1.DeleteOptions{}); err != nil {
		return errors.Wrapf(err, "error deleting cluster %s/%s", namespace, name)
	}

	return nil
}

func (c *client) GetClusters(namespace string) ([]*clusterv1.Cluster, error) {
	clusters := []*clusterv1.Cluster{}
	clusterlist, err := c.clientSet.ClusterV1alpha1().Clusters(namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "error listing cluster objects in namespace %q", namespace)
	}

	for i := 0; i < len(clusterlist.Items); i++ {
		clusters = append(clusters, &clusterlist.Items[i])
	}
	return clusters, nil
}

func (c *client) GetMachineClasses(namespace string) ([]*clusterv1.MachineClass, error) {
	machineClassesList, err := c.clientSet.ClusterV1alpha1().MachineClasses(namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "error listing MachineClasses in namespace %q", namespace)
	}
	var machineClasses []*clusterv1.MachineClass
	for i := 0; i < len(machineClassesList.Items); i++ {
		machineClasses = append(machineClasses, &machineClassesList.Items[i])
	}
	return machineClasses, nil
}

func (c *client) GetMachineDeployment(namespace, name string) (*clusterv1.MachineDeployment, error) {
	machineDeployment, err := c.clientSet.ClusterV1alpha1().MachineDeployments(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "error getting MachineDeployment: %s/%s", namespace, name)
	}
	return machineDeployment, nil
}

func (c *client) GetMachineDeploymentsForCluster(cluster *clusterv1.Cluster) ([]*clusterv1.MachineDeployment, error) {
	machineDeploymentList, err := c.clientSet.ClusterV1alpha1().MachineDeployments(cluster.Namespace).List(metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", machineClusterLabelName, cluster.Name),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "error listing MachineDeployments for Cluster %s/%s", cluster.Namespace, cluster.Name)
	}
	var machineDeployments []*clusterv1.MachineDeployment
	for _, md := range machineDeploymentList.Items {
		for _, or := range md.GetOwnerReferences() {
			if or.Kind == cluster.Kind && or.Name == cluster.Name {
				machineDeployments = append(machineDeployments, &md)
				continue
			}
		}
	}
	return machineDeployments, nil
}

func (c *client) GetMachineDeployments(namespace string) ([]*clusterv1.MachineDeployment, error) {
	machineDeploymentList, err := c.clientSet.ClusterV1alpha1().MachineDeployments(namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "error listing machine deployment objects in namespace %q", namespace)
	}
	var machineDeployments []*clusterv1.MachineDeployment
	for i := 0; i < len(machineDeploymentList.Items); i++ {
		machineDeployments = append(machineDeployments, &machineDeploymentList.Items[i])
	}
	return machineDeployments, nil
}

func (c *client) GetMachineSet(namespace, name string) (*clusterv1.MachineSet, error) {
	machineSet, err := c.clientSet.ClusterV1alpha1().MachineSets(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "error getting MachineSet: %s/%s", namespace, name)
	}
	return machineSet, nil
}

func (c *client) GetMachineSets(namespace string) ([]*clusterv1.MachineSet, error) {
	machineSetList, err := c.clientSet.ClusterV1alpha1().MachineSets(namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "error listing MachineSets in namespace %q", namespace)
	}
	var machineSets []*clusterv1.MachineSet
	for i := 0; i < len(machineSetList.Items); i++ {
		machineSets = append(machineSets, &machineSetList.Items[i])
	}
	return machineSets, nil
}

func (c *client) GetMachineSetsForCluster(cluster *clusterv1.Cluster) ([]*clusterv1.MachineSet, error) {
	machineSetList, err := c.clientSet.ClusterV1alpha1().MachineSets(cluster.Namespace).List(metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", machineClusterLabelName, cluster.Name),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "error listing MachineSets for Cluster %s/%s", cluster.Namespace, cluster.Name)
	}
	var machineSets []*clusterv1.MachineSet
	for _, ms := range machineSetList.Items {
		for _, or := range ms.GetOwnerReferences() {
			if or.Kind == cluster.Kind && or.Name == cluster.Name {
				machineSets = append(machineSets, &ms)
				continue
			}
		}
	}
	return machineSets, nil
}

func (c *client) GetMachineSetsForMachineDeployment(md *clusterv1.MachineDeployment) ([]*clusterv1.MachineSet, error) {
	machineSets, err := c.GetMachineSets(md.Namespace)
	if err != nil {
		return nil, err
	}
	var controlledMachineSets []*clusterv1.MachineSet
	for _, ms := range machineSets {
		if metav1.GetControllerOf(ms) != nil && metav1.IsControlledBy(ms, md) {
			controlledMachineSets = append(controlledMachineSets, ms)
		}
	}
	return controlledMachineSets, nil
}

func (c *client) GetMachines(namespace string) ([]*clusterv1.Machine, error) {
	machines := []*clusterv1.Machine{}
	machineslist, err := c.clientSet.ClusterV1alpha1().Machines(namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "error listing Machines in namespace %q", namespace)
	}

	for i := 0; i < len(machineslist.Items); i++ {
		machines = append(machines, &machineslist.Items[i])
	}
	return machines, nil
}

func (c *client) GetMachinesForCluster(cluster *clusterv1.Cluster) ([]*clusterv1.Machine, error) {
	machineslist, err := c.clientSet.ClusterV1alpha1().Machines(cluster.Namespace).List(metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", machineClusterLabelName, cluster.Name),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "error listing Machines for Cluster %s/%s", cluster.Namespace, cluster.Name)
	}
	var machines []*clusterv1.Machine

	for i := 0; i < len(machineslist.Items); i++ {
		m := &machineslist.Items[i]

		for _, or := range m.GetOwnerReferences() {
			if or.Kind == cluster.Kind && or.Name == cluster.Name {
				machines = append(machines, m)
				break
			}
		}
	}

	return machines, nil
}

func (c *client) GetMachinesForMachineSet(ms *clusterv1.MachineSet) ([]*clusterv1.Machine, error) {
	machines, err := c.GetMachines(ms.Namespace)
	if err != nil {
		return nil, err
	}
	var controlledMachines []*clusterv1.Machine
	for _, m := range machines {
		if metav1.GetControllerOf(m) != nil && metav1.IsControlledBy(m, ms) {
			controlledMachines = append(controlledMachines, m)
		}
	}
	return controlledMachines, nil
}

func (c *client) CreateMachineClass(machineClass *clusterv1.MachineClass) error {
	_, err := c.clientSet.ClusterV1alpha1().MachineClasses(machineClass.Namespace).Create(machineClass)
	if err != nil {
		return errors.Wrapf(err, "error creating MachineClass %s/%s", machineClass.Namespace, machineClass.Name)
	}
	return nil
}

func (c *client) DeleteMachineClass(namespace, name string) error {
	if err := c.clientSet.ClusterV1alpha1().MachineClasses(namespace).Delete(name, newDeleteOptions()); err != nil {
		return errors.Wrapf(err, "error deleting MachineClass %s/%s", namespace, name)
	}
	return nil
}

func (c *client) CreateClusterObject(cluster *clusterv1.Cluster) error {
	namespace := c.GetContextNamespace()
	if cluster.Namespace != "" {
		namespace = cluster.Namespace
	}

	_, err := c.clientSet.ClusterV1alpha1().Clusters(namespace).Create(cluster)
	if err != nil {
		return errors.Wrapf(err, "error creating cluster in namespace %v", namespace)
	}
	return nil
}

func (c *client) CreateMachineDeployments(deployments []*clusterv1.MachineDeployment, namespace string) error {
	for _, deploy := range deployments {
		// TODO: Run in parallel https://github.com/kubernetes-sigs/cluster-api/issues/258
		_, err := c.clientSet.ClusterV1alpha1().MachineDeployments(namespace).Create(deploy)
		if err != nil {
			return errors.Wrapf(err, "error creating a machine deployment object in namespace %q", namespace)
		}
	}
	return nil
}

func (c *client) CreateMachineSets(machineSets []*clusterv1.MachineSet, namespace string) error {
	for _, ms := range machineSets {
		// TODO: Run in parallel https://github.com/kubernetes-sigs/cluster-api/issues/258
		_, err := c.clientSet.ClusterV1alpha1().MachineSets(namespace).Create(ms)
		if err != nil {
			return errors.Wrapf(err, "error creating a machine set object in namespace %q", namespace)
		}
	}
	return nil
}

func (c *client) CreateMachines(machines []*clusterv1.Machine, namespace string) error {
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
					gerr = errors.Wrapf(err, "error creating a machine object in namespace %v", namespace)
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

// DeleteClusters deletes all Clusters in a namespace. If the namespace is empty then all Clusters in all namespaces are deleted.
func (c *client) DeleteClusters(namespace string) error {
	seen := make(map[string]bool)

	if namespace != "" {
		seen[namespace] = true
	} else {
		clusters, err := c.clientSet.ClusterV1alpha1().Clusters("").List(metav1.ListOptions{})
		if err != nil {
			return errors.Wrap(err, "error listing Clusters in all namespaces")
		}
		for _, cluster := range clusters.Items {
			if _, ok := seen[cluster.Namespace]; !ok {
				seen[cluster.Namespace] = true
			}
		}
	}
	for ns := range seen {
		err := c.clientSet.ClusterV1alpha1().Clusters(ns).DeleteCollection(newDeleteOptions(), metav1.ListOptions{})
		if err != nil {
			return errors.Wrapf(err, "error deleting Clusters in namespace %q", ns)
		}
		err = c.waitForClusterDelete(ns)
		if err != nil {
			return errors.Wrapf(err, "error waiting for Cluster(s) deletion to complete in namespace %q", ns)
		}
	}

	return nil
}

// DeleteMachineClasses deletes all MachineClasses in a namespace. If the namespace is empty then all MachineClasses in all namespaces are deleted.
func (c *client) DeleteMachineClasses(namespace string) error {
	seen := make(map[string]bool)

	if namespace != "" {
		seen[namespace] = true
	} else {
		machineClasses, err := c.clientSet.ClusterV1alpha1().MachineClasses("").List(metav1.ListOptions{})
		if err != nil {
			return errors.Wrap(err, "error listing MachineClasses in all namespaces")
		}
		for _, mc := range machineClasses.Items {
			if _, ok := seen[mc.Namespace]; !ok {
				seen[mc.Namespace] = true
			}
		}
	}

	for ns := range seen {
		if err := c.DeleteMachineClasses(ns); err != nil {
			return err
		}
		err := c.clientSet.ClusterV1alpha1().MachineClasses(ns).DeleteCollection(newDeleteOptions(), metav1.ListOptions{})
		if err != nil {
			return errors.Wrapf(err, "error deleting MachineClasses in namespace %q", ns)
		}
		err = c.waitForMachineClassesDelete(ns)
		if err != nil {
			return errors.Wrapf(err, "error waiting for MachineClass(es) deletion to complete in ns %q", ns)
		}
	}

	return nil
}

// DeleteMachineDeployments deletes all MachineDeployments in a namespace. If the namespace is empty then all MachineDeployments in all namespaces are deleted.
func (c *client) DeleteMachineDeployments(namespace string) error {
	seen := make(map[string]bool)

	if namespace != "" {
		seen[namespace] = true
	} else {
		machineDeployments, err := c.clientSet.ClusterV1alpha1().MachineDeployments("").List(metav1.ListOptions{})
		if err != nil {
			return errors.Wrap(err, "error listing MachineDeployments in all namespaces")
		}
		for _, md := range machineDeployments.Items {
			if _, ok := seen[md.Namespace]; !ok {
				seen[md.Namespace] = true
			}
		}
	}
	for ns := range seen {
		err := c.clientSet.ClusterV1alpha1().MachineDeployments(ns).DeleteCollection(newDeleteOptions(), metav1.ListOptions{})
		if err != nil {
			return errors.Wrapf(err, "error deleting MachineDeployments in namespace %q", ns)
		}
		err = c.waitForMachineDeploymentsDelete(ns)
		if err != nil {
			return errors.Wrapf(err, "error waiting for MachineDeployment(s) deletion to complete in namespace %q", ns)
		}
	}

	return nil
}

// DeleteMachineSets deletes all MachineSets in a namespace. If the namespace is empty then all MachineSets in all namespaces are deleted.
func (c *client) DeleteMachineSets(namespace string) error {
	seen := make(map[string]bool)

	if namespace != "" {
		seen[namespace] = true
	} else {
		machineSets, err := c.clientSet.ClusterV1alpha1().MachineSets("").List(metav1.ListOptions{})
		if err != nil {
			return errors.Wrap(err, "error listing MachineSets in all namespaces")
		}
		for _, ms := range machineSets.Items {
			if _, ok := seen[ms.Namespace]; !ok {
				seen[ms.Namespace] = true
			}
		}
	}
	for ns := range seen {
		err := c.clientSet.ClusterV1alpha1().MachineSets(ns).DeleteCollection(newDeleteOptions(), metav1.ListOptions{})
		if err != nil {
			return errors.Wrapf(err, "error deleting MachineSets in namespace %q", ns)
		}
		err = c.waitForMachineSetsDelete(ns)
		if err != nil {
			return errors.Wrapf(err, "error waiting for MachineSet(s) deletion to complete in namespace %q", ns)
		}
	}

	return nil
}

// DeleteMachines deletes all Machines in a namespace. If the namespace is empty then all Machines in all namespaces are deleted.
func (c *client) DeleteMachines(namespace string) error {
	seen := make(map[string]bool)

	if namespace != "" {
		seen[namespace] = true
	} else {
		machines, err := c.clientSet.ClusterV1alpha1().Machines("").List(metav1.ListOptions{})
		if err != nil {
			return errors.Wrap(err, "error listing Machines in all namespaces")
		}
		for _, m := range machines.Items {
			if _, ok := seen[m.Namespace]; !ok {
				seen[m.Namespace] = true
			}
		}
	}
	for ns := range seen {
		err := c.clientSet.ClusterV1alpha1().Machines(ns).DeleteCollection(newDeleteOptions(), metav1.ListOptions{})
		if err != nil {
			return errors.Wrapf(err, "error deleting Machines in namespace %q", ns)
		}
		err = c.waitForMachinesDelete(ns)
		if err != nil {
			return errors.Wrapf(err, "error waiting for Machine(s) deletion to complete in namespace %q", ns)
		}
	}

	return nil
}

func (c *client) ForceDeleteMachine(namespace, name string) error {
	machine, err := c.clientSet.ClusterV1alpha1().Machines(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "error getting Machine %s/%s", namespace, name)
	}
	machine.SetFinalizers([]string{})
	if _, err := c.clientSet.ClusterV1alpha1().Machines(namespace).Update(machine); err != nil {
		return errors.Wrapf(err, "error removing finalizer for Machine %s/%s", namespace, name)
	}
	if err := c.clientSet.ClusterV1alpha1().Machines(namespace).Delete(name, newDeleteOptions()); err != nil {
		return errors.Wrapf(err, "error deleting Machine %s/%s", namespace, name)
	}
	return nil
}

func (c *client) ForceDeleteMachineSet(namespace, name string) error {
	ms, err := c.clientSet.ClusterV1alpha1().MachineSets(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "error getting MachineSet %s/%s", namespace, name)
	}
	ms.SetFinalizers([]string{})
	if _, err := c.clientSet.ClusterV1alpha1().MachineSets(namespace).Update(ms); err != nil {
		return errors.Wrapf(err, "error removing finalizer for MachineSet %s/%s", namespace, name)
	}
	if err := c.clientSet.ClusterV1alpha1().MachineSets(namespace).Delete(name, newDeleteOptions()); err != nil {
		return errors.Wrapf(err, "error deleting MachineSet %s/%s", namespace, name)
	}
	return nil
}

func (c *client) ForceDeleteMachineDeployment(namespace, name string) error {
	md, err := c.clientSet.ClusterV1alpha1().MachineDeployments(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "error getting MachineDeployment %s/%s", namespace, name)
	}
	md.SetFinalizers([]string{})
	if _, err := c.clientSet.ClusterV1alpha1().MachineDeployments(namespace).Update(md); err != nil {
		return errors.Wrapf(err, "error removing finalizer for MachineDeployment %s/%s", namespace, name)
	}
	if err := c.clientSet.ClusterV1alpha1().MachineDeployments(namespace).Delete(name, newDeleteOptions()); err != nil {
		return errors.Wrapf(err, "error deleting MachineDeployment %s/%s", namespace, name)
	}
	return nil
}

func newDeleteOptions() *metav1.DeleteOptions {
	propagationPolicy := metav1.DeletePropagationForeground
	return &metav1.DeleteOptions{
		PropagationPolicy: &propagationPolicy,
	}
}

// UpdateClusterObjectEndpoint updates the status of a cluster API endpoint, clusterEndpoint
// can be passed as hostname or hostname:port, if port is not present the default port 443 is applied.
// TODO: Test this function
func (c *client) UpdateClusterObjectEndpoint(clusterEndpoint, clusterName, namespace string) error {
	cluster, err := c.GetCluster(clusterName, namespace)
	if err != nil {
		return err
	}
	endpointHost, endpointPort, err := net.SplitHostPort(clusterEndpoint)
	if err != nil {
		// We rely on provider.GetControlPlaneEndpoint to provide a correct hostname/IP, no
		// further validation is done.
		endpointHost = clusterEndpoint
		endpointPort = defaultAPIServerPort
	}
	endpointPortInt, err := strconv.Atoi(endpointPort)
	if err != nil {
		return errors.Wrapf(err, "error while converting cluster endpoint port %q", endpointPort)
	}
	cluster.Status.APIEndpoints = append(cluster.Status.APIEndpoints,
		clusterv1.APIEndpoint{
			Host: endpointHost,
			Port: endpointPortInt,
		})
	_, err = c.clientSet.ClusterV1alpha1().Clusters(namespace).UpdateStatus(cluster)
	return err
}

func (c *client) WaitForClusterV1alpha1Ready() error {
	return waitForClusterResourceReady(c.clientSet)
}

func (c *client) WaitForResourceStatuses() error {
	deadline := time.Now().Add(timeoutResourceReady)

	timeout := time.Until(deadline)
	return util.PollImmediate(retryIntervalResourceReady, timeout, func() (bool, error) {
		klog.V(2).Info("Waiting for Cluster API resources to have statuses...")
		clusters, err := c.clientSet.ClusterV1alpha1().Clusters("").List(metav1.ListOptions{})
		if err != nil {
			return false, nil
		}
		for _, cluster := range clusters.Items {
			if reflect.DeepEqual(clusterv1.ClusterStatus{}, cluster.Status) {
				return false, nil
			}
			if cluster.Status.ProviderStatus == nil {
				return false, nil
			}
		}
		machineDeployments, err := c.clientSet.ClusterV1alpha1().MachineDeployments("").List(metav1.ListOptions{})
		if err != nil {
			return false, nil
		}
		for _, md := range machineDeployments.Items {
			if reflect.DeepEqual(clusterv1.MachineDeploymentStatus{}, md.Status) {
				return false, nil
			}
		}
		machineSets, err := c.clientSet.ClusterV1alpha1().MachineSets("").List(metav1.ListOptions{})
		if err != nil {
			return false, nil
		}
		for _, ms := range machineSets.Items {
			if reflect.DeepEqual(clusterv1.MachineSetStatus{}, ms.Status) {
				return false, nil
			}
		}
		machines, err := c.clientSet.ClusterV1alpha1().Machines("").List(metav1.ListOptions{})
		if err != nil {
			return false, nil
		}
		for _, m := range machines.Items {
			if reflect.DeepEqual(clusterv1.MachineStatus{}, m.Status) {
				return false, nil
			}
			if m.Status.ProviderStatus == nil {
				return false, nil
			}
		}

		return true, nil
	})
}

func (c *client) waitForClusterDelete(namespace string) error {
	return util.PollImmediate(retryIntervalResourceDelete, timeoutResourceDelete, func() (bool, error) {
		klog.V(2).Infof("Waiting for Clusters to be deleted...")
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

func (c *client) waitForMachineClassesDelete(namespace string) error {
	return util.PollImmediate(retryIntervalResourceDelete, timeoutResourceDelete, func() (bool, error) {
		klog.V(2).Infof("Waiting for MachineClasses to be deleted...")
		response, err := c.clientSet.ClusterV1alpha1().MachineClasses(namespace).List(metav1.ListOptions{})
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
		klog.V(2).Infof("Waiting for MachineDeployments to be deleted...")
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
		klog.V(2).Infof("Waiting for MachineSets to be deleted...")
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
		klog.V(2).Infof("Waiting for Machines to be deleted...")
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

func (c *client) waitForMachineDelete(namespace, name string) error {
	return util.PollImmediate(retryIntervalResourceDelete, timeoutResourceDelete, func() (bool, error) {
		klog.V(2).Infof("Waiting for Machine %s/%s to be deleted...", namespace, name)
		response, err := c.clientSet.ClusterV1alpha1().Machines(namespace).Get(name, metav1.GetOptions{})
		if err != nil {
			return false, nil
		}
		if response != nil {
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
		return errors.Wrapf(err, "couldn't kubectl apply, output: %s", string(out))
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
			if strings.Contains(err.Error(), io.EOF.Error()) || strings.Contains(err.Error(), "refused") {
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
		// Seeing a node cannot be purely relied upon because the provisioned control plane will not be registering with
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

func GetClusterAPIObject(client Client, clusterName, namespace string) (*clusterv1.Cluster, *clusterv1.Machine, []*clusterv1.Machine, error) {
	machines, err := client.GetMachines(namespace)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "unable to fetch machines")
	}
	cluster, err := client.GetCluster(clusterName, namespace)
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "unable to fetch cluster %s/%s", namespace, clusterName)
	}

	controlPlane, nodes, err := ExtractControlPlaneMachine(machines)
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "unable to fetch control plane machine in cluster %s/%s", namespace, clusterName)
	}
	return cluster, controlPlane, nodes, nil
}

// ExtractControlPlaneMachine separates the machines running the control plane (singular) from the incoming machines.
// This is currently done by looking at which machine specifies the control plane version.
// TODO: Cleanup.
func ExtractControlPlaneMachine(machines []*clusterv1.Machine) (*clusterv1.Machine, []*clusterv1.Machine, error) {
	nodes := []*clusterv1.Machine{}
	controlPlaneMachines := []*clusterv1.Machine{}
	for _, machine := range machines {
		if util.IsControlPlaneMachine(machine) {
			controlPlaneMachines = append(controlPlaneMachines, machine)
		} else {
			nodes = append(nodes, machine)
		}
	}
	if len(controlPlaneMachines) != 1 {
		return nil, nil, errors.Errorf("expected one control plane machine, got: %v", len(controlPlaneMachines))
	}
	return controlPlaneMachines[0], nodes, nil
}
