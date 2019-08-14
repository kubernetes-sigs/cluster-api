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
	"context"
	"fmt"
	"io"
	"io/ioutil"
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
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha2"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/clientcmd"
	"sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	retryIntervalKubectlApply  = 10 * time.Second
	retryIntervalResourceReady = 10 * time.Second
	timeoutKubectlApply        = 15 * time.Minute
	timeoutResourceReady       = 15 * time.Minute
	timeoutMachineReady        = 30 * time.Minute
	machineClusterLabelName    = "cluster.k8s.io/cluster-name"
)

const (
	TimeoutMachineReady = "CLUSTER_API_MACHINE_READY_TIMEOUT"
)

var (
	ctx                     = context.Background()
	deletePropagationPolicy = ctrlclient.PropagationPolicy(metav1.DeletePropagationForeground)
)

// Provides interaction with a cluster
type Client interface {
	Apply(string) error
	Close() error
	CreateClusterObject(*clusterv1.Cluster) error
	CreateMachineDeployments([]*clusterv1.MachineDeployment, string) error
	CreateMachineSets([]*clusterv1.MachineSet, string) error
	CreateMachines([]*clusterv1.Machine, string) error
	Delete(string) error
	DeleteClusters(string) error
	DeleteNamespace(string) error
	DeleteMachineDeployments(string) error
	DeleteMachineSets(string) error
	DeleteMachines(string) error
	ForceDeleteCluster(namespace, name string) error
	ForceDeleteMachine(namespace, name string) error
	ForceDeleteMachineSet(namespace, name string) error
	ForceDeleteMachineDeployment(namespace, name string) error
	EnsureNamespace(string) error
	GetKubeconfigFromSecret(namespace, clusterName string) (string, error)
	GetClusters(string) ([]*clusterv1.Cluster, error)
	GetCluster(string, string) (*clusterv1.Cluster, error)
	GetContextNamespace() string
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
	WaitForClusterV1alpha2Ready() error
	WaitForResourceStatuses() error
}

type client struct {
	clientSet       ctrlclient.Client
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

	_, err = clientset.CoreV1().Namespaces().Get(namespaceName, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if apierrors.IsForbidden(err) {
		namespaces, err := clientset.CoreV1().Namespaces().List(metav1.ListOptions{})
		if err != nil {
			return err
		}

		for _, ns := range namespaces.Items {
			if ns.Name == namespaceName {
				return nil
			}
		}
	}
	if !apierrors.IsNotFound(err) {
		return err
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

func (c *client) GetKubeconfigFromSecret(namespace, clusterName string) (string, error) {
	clientset, err := clientcmd.NewCoreClientSetForDefaultSearchPath(c.kubeconfigFile, clientcmd.NewConfigOverrides())
	if err != nil {
		return "", errors.Wrap(err, "error creating core clientset")
	}

	secretName := fmt.Sprintf("%s-kubeconfig", clusterName)
	secret, err := clientset.CoreV1().Secrets(namespace).Get(secretName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	return string(secret.Data["value"]), nil
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
	c, err := clientcmd.NewControllerRuntimeClient(kubeconfigFile, overrides)
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
	cluster := &clusterv1.Cluster{}
	if err := c.clientSet.Get(ctx, ctrlclient.ObjectKey{Name: name, Namespace: namespace}, cluster); err != nil {
		return errors.Wrapf(err, "error getting cluster %s/%s", namespace, name)
	}

	cluster.ObjectMeta.SetFinalizers([]string{})
	if err := c.clientSet.Update(ctx, cluster); err != nil {
		return errors.Wrapf(err, "error removing finalizer on cluster %s/%s", namespace, name)
	}

	if err := c.clientSet.Delete(ctx, cluster); err != nil {
		return errors.Wrapf(err, "error deleting cluster %s/%s", namespace, name)
	}

	return nil
}

func (c *client) GetClusters(namespace string) ([]*clusterv1.Cluster, error) {
	clusters := &clusterv1.ClusterList{}
	if err := c.clientSet.List(ctx, clusters); err != nil {
		return nil, errors.Wrapf(err, "error listing cluster objects in namespace %q", namespace)
	}
	items := []*clusterv1.Cluster{}
	for i := 0; i < len(clusters.Items); i++ {
		items = append(items, &clusters.Items[i])
	}
	return items, nil
}

func (c *client) GetMachineDeployment(namespace, name string) (*clusterv1.MachineDeployment, error) {
	obj := &clusterv1.MachineDeployment{}
	if err := c.clientSet.Get(ctx, ctrlclient.ObjectKey{Name: name, Namespace: namespace}, obj); err != nil {
		return nil, errors.Wrapf(err, "error getting MachineDeployment %s/%s", namespace, name)
	}
	return obj, nil
}

func (c *client) GetMachineDeploymentsForCluster(cluster *clusterv1.Cluster) ([]*clusterv1.MachineDeployment, error) {
	selectors := []ctrlclient.ListOption{
		ctrlclient.MatchingLabels{
			machineClusterLabelName: cluster.Name,
		},
		ctrlclient.InNamespace(cluster.Namespace),
	}
	machineDeploymentList := &clusterv1.MachineDeploymentList{}
	if err := c.clientSet.List(ctx, machineDeploymentList, selectors...); err != nil {
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
	machineDeploymentList := &clusterv1.MachineDeploymentList{}
	if err := c.clientSet.List(ctx, machineDeploymentList, ctrlclient.InNamespace(namespace)); err != nil {
		return nil, errors.Wrapf(err, "error listing machine deployment objects in namespace %q", namespace)
	}
	var machineDeployments []*clusterv1.MachineDeployment
	for i := 0; i < len(machineDeploymentList.Items); i++ {
		machineDeployments = append(machineDeployments, &machineDeploymentList.Items[i])
	}
	return machineDeployments, nil
}

func (c *client) GetMachineSet(namespace, name string) (*clusterv1.MachineSet, error) {
	obj := &clusterv1.MachineSet{}
	if err := c.clientSet.Get(ctx, ctrlclient.ObjectKey{Name: name, Namespace: namespace}, obj); err != nil {
		return nil, errors.Wrapf(err, "error getting MachineSet: %s/%s", namespace, name)
	}
	return obj, nil
}

func (c *client) GetMachineSets(namespace string) ([]*clusterv1.MachineSet, error) {
	machineSetList := &clusterv1.MachineSetList{}
	if err := c.clientSet.List(ctx, machineSetList, ctrlclient.InNamespace(namespace)); err != nil {
		return nil, errors.Wrapf(err, "error listing MachineSets in namespace %q", namespace)
	}
	var machineSets []*clusterv1.MachineSet
	for i := 0; i < len(machineSetList.Items); i++ {
		machineSets = append(machineSets, &machineSetList.Items[i])
	}
	return machineSets, nil
}

func (c *client) GetMachineSetsForCluster(cluster *clusterv1.Cluster) ([]*clusterv1.MachineSet, error) {
	selectors := []ctrlclient.ListOption{
		ctrlclient.MatchingLabels{
			machineClusterLabelName: cluster.Name,
		},
		ctrlclient.InNamespace(cluster.Namespace),
	}
	machineSetList := &clusterv1.MachineSetList{}
	if err := c.clientSet.List(ctx, machineSetList, selectors...); err != nil {
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

func (c *client) GetMachines(namespace string) (machines []*clusterv1.Machine, _ error) {
	machineslist := &clusterv1.MachineList{}
	if err := c.clientSet.List(ctx, machineslist, ctrlclient.InNamespace(namespace)); err != nil {
		return nil, errors.Wrapf(err, "error listing Machines in namespace %q", namespace)
	}
	for i := 0; i < len(machineslist.Items); i++ {
		machines = append(machines, &machineslist.Items[i])
	}
	return
}

func (c *client) GetMachinesForCluster(cluster *clusterv1.Cluster) ([]*clusterv1.Machine, error) {
	selectors := []ctrlclient.ListOption{
		ctrlclient.MatchingLabels{
			machineClusterLabelName: cluster.Name,
		},
		ctrlclient.InNamespace(cluster.Namespace),
	}
	machineslist := &clusterv1.MachineList{}
	if err := c.clientSet.List(ctx, machineslist, selectors...); err != nil {
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

func (c *client) CreateClusterObject(cluster *clusterv1.Cluster) error {
	namespace := c.GetContextNamespace()
	if cluster.Namespace != "" {
		namespace = cluster.Namespace
	}
	if err := c.clientSet.Create(ctx, cluster); err != nil {
		return errors.Wrapf(err, "error creating cluster in namespace %v", namespace)
	}
	return nil
}

func (c *client) CreateMachineDeployments(deployments []*clusterv1.MachineDeployment, namespace string) error {
	for _, deploy := range deployments {
		// TODO: Run in parallel https://github.com/kubernetes-sigs/cluster-api/issues/258
		if err := c.clientSet.Create(ctx, deploy); err != nil {
			return errors.Wrapf(err, "error creating a machine deployment object in namespace %q", namespace)
		}
	}
	return nil
}

func (c *client) CreateMachineSets(machineSets []*clusterv1.MachineSet, namespace string) error {
	for _, ms := range machineSets {
		// TODO: Run in parallel https://github.com/kubernetes-sigs/cluster-api/issues/258
		if err := c.clientSet.Create(ctx, ms); err != nil {
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

			if err := c.clientSet.Create(ctx, machine); err != nil {
				errOnce.Do(func() {
					gerr = errors.Wrapf(err, "error creating a machine object in namespace %v", namespace)
				})
				return
			}

			if err := waitForMachineReady(c.clientSet, machine); err != nil {
				errOnce.Do(func() { gerr = err })
			}
		}(machine)
	}
	wg.Wait()
	return gerr
}

// DeleteClusters deletes all Clusters in a namespace. If the namespace is empty then all Clusters in all namespaces are deleted.
func (c *client) DeleteClusters(namespace string) error {
	if err := c.clientSet.DeleteAllOf(ctx, &clusterv1.Cluster{}, ctrlclient.InNamespace(namespace)); err != nil {
		return errors.Wrapf(err, "error deleting Clusters in namespace %q", namespace)
	}
	return nil
}

// DeleteMachineDeployments deletes all MachineDeployments in a namespace. If the namespace is empty then all MachineDeployments in all namespaces are deleted.
func (c *client) DeleteMachineDeployments(namespace string) error {
	if err := c.clientSet.DeleteAllOf(ctx, &clusterv1.MachineDeployment{}, ctrlclient.InNamespace(namespace)); err != nil {
		return errors.Wrapf(err, "error deleting Clusters in namespace %q", namespace)
	}
	return nil
}

// DeleteMachineSets deletes all MachineSets in a namespace. If the namespace is empty then all MachineSets in all namespaces are deleted.
func (c *client) DeleteMachineSets(namespace string) error {
	if err := c.clientSet.DeleteAllOf(ctx, &clusterv1.MachineSet{}, ctrlclient.InNamespace(namespace)); err != nil {
		return errors.Wrapf(err, "error deleting Clusters in namespace %q", namespace)
	}
	return nil
}

// DeleteMachines deletes all Machines in a namespace. If the namespace is empty then all Machines in all namespaces are deleted.
func (c *client) DeleteMachines(namespace string) error {
	if err := c.clientSet.DeleteAllOf(ctx, &clusterv1.Machine{}, ctrlclient.InNamespace(namespace)); err != nil {
		return errors.Wrapf(err, "error deleting Clusters in namespace %q", namespace)
	}
	return nil
}

func (c *client) ForceDeleteMachine(namespace, name string) error {
	machine := &clusterv1.Machine{}
	if err := c.clientSet.Get(ctx, ctrlclient.ObjectKey{Name: name, Namespace: namespace}, machine); err != nil {
		return errors.Wrapf(err, "error getting Machine %s/%s", namespace, name)
	}
	machine.SetFinalizers([]string{})
	if err := c.clientSet.Update(ctx, machine); err != nil {
		return errors.Wrapf(err, "error removing finalizer for Machine %s/%s", namespace, name)
	}
	if err := c.clientSet.Delete(ctx, machine, deletePropagationPolicy); err != nil {
		return errors.Wrapf(err, "error deleting Machine %s/%s", namespace, name)
	}
	return nil
}

func (c *client) ForceDeleteMachineSet(namespace, name string) error {
	ms := &clusterv1.MachineSet{}
	if err := c.clientSet.Get(ctx, ctrlclient.ObjectKey{Name: name, Namespace: namespace}, ms); err != nil {
		return errors.Wrapf(err, "error getting MachineSet %s/%s", namespace, name)
	}
	ms.SetFinalizers([]string{})
	if err := c.clientSet.Update(ctx, ms); err != nil {
		return errors.Wrapf(err, "error removing finalizer for MachineSet %s/%s", namespace, name)
	}
	if err := c.clientSet.Delete(ctx, ms, deletePropagationPolicy); err != nil {
		return errors.Wrapf(err, "error deleting MachineSet %s/%s", namespace, name)
	}
	return nil
}

func (c *client) ForceDeleteMachineDeployment(namespace, name string) error {
	md := &clusterv1.MachineDeployment{}
	if err := c.clientSet.Get(ctx, ctrlclient.ObjectKey{Name: name, Namespace: namespace}, md); err != nil {
		return errors.Wrapf(err, "error getting MachineDeployment %s/%s", namespace, name)
	}
	md.SetFinalizers([]string{})
	if err := c.clientSet.Update(ctx, md); err != nil {
		return errors.Wrapf(err, "error removing finalizer for MachineDeployment %s/%s", namespace, name)
	}
	if err := c.clientSet.Delete(ctx, md, deletePropagationPolicy); err != nil {
		return errors.Wrapf(err, "error deleting MachineDeployment %s/%s", namespace, name)
	}
	return nil
}

func (c *client) WaitForClusterV1alpha2Ready() error {
	return waitForClusterResourceReady(c.clientSet)
}

func (c *client) WaitForResourceStatuses() error {
	deadline := time.Now().Add(timeoutResourceReady)

	timeout := time.Until(deadline)
	return util.PollImmediate(retryIntervalResourceReady, timeout, func() (bool, error) {
		klog.V(2).Info("Waiting for Cluster API resources to have statuses...")

		clusters := &clusterv1.ClusterList{}
		if err := c.clientSet.List(ctx, clusters); err != nil {
			klog.V(10).Infof("retrying: failed to list clusters: %v", err)
			return false, nil
		}
		for _, cluster := range clusters.Items {
			if reflect.DeepEqual(clusterv1.ClusterStatus{}, cluster.Status) {
				klog.V(10).Info("retrying: cluster status is empty")
				return false, nil
			}
			if !cluster.Status.InfrastructureReady {
				klog.V(10).Info("retrying: cluster.Status.InfrastructureReady is false")
				return false, nil
			}
		}

		machineDeployments := &clusterv1.MachineDeploymentList{}
		if err := c.clientSet.List(ctx, machineDeployments); err != nil {
			klog.V(10).Infof("retrying: failed to list machine deployment: %v", err)
			return false, nil
		}
		for _, md := range machineDeployments.Items {
			if reflect.DeepEqual(clusterv1.MachineDeploymentStatus{}, md.Status) {
				klog.V(10).Info("retrying: machine deployment status is empty")
				return false, nil
			}
		}

		machineSets := &clusterv1.MachineSetList{}
		if err := c.clientSet.List(ctx, machineSets); err != nil {
			klog.V(10).Infof("retrying: failed to list machinesets: %v", err)
			return false, nil
		}
		for _, ms := range machineSets.Items {
			if reflect.DeepEqual(clusterv1.MachineSetStatus{}, ms.Status) {
				klog.V(10).Info("retrying: machineset status is empty")
				return false, nil
			}
		}

		machines := &clusterv1.MachineList{}
		if err := c.clientSet.List(ctx, machines); err != nil {
			klog.V(10).Infof("retrying: failed to list machines: %v", err)
			return false, nil
		}
		for _, m := range machines.Items {
			if reflect.DeepEqual(clusterv1.MachineStatus{}, m.Status) {
				klog.V(10).Info("retrying: machine status is empty")
				return false, nil
			}
			// if m.Status.ProviderStatus == nil {
			// 	klog.V(10).Info("retrying: machine.Status.ProviderStatus is not set")
			// 	return false, nil
			// }
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
			if strings.Contains(err.Error(), io.EOF.Error()) || strings.Contains(err.Error(), "refused") || strings.Contains(err.Error(), "no such host") {
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

func waitForClusterResourceReady(cs ctrlclient.Client) error {
	deadline := time.Now().Add(timeoutResourceReady)
	timeout := time.Until(deadline)
	return util.PollImmediate(retryIntervalResourceReady, timeout, func() (bool, error) {
		klog.V(2).Info("Waiting for Cluster resources to be listable...")
		clusterList := &clusterv1.ClusterList{}
		if err := cs.List(ctx, clusterList); err == nil {
			return true, nil
		}
		return false, nil
	})
}

func waitForMachineReady(cs ctrlclient.Client, machine *clusterv1.Machine) error {
	timeout := timeoutMachineReady
	if p := os.Getenv(TimeoutMachineReady); p != "" {
		t, err := strconv.Atoi(p)
		if err == nil {
			// only valid value will be used
			timeout = time.Duration(t) * time.Minute
			klog.V(4).Info("Setting wait for machine timeout value to ", timeout)
		}
	}

	err := util.PollImmediate(retryIntervalResourceReady, timeout, func() (bool, error) {
		klog.V(2).Infof("Waiting for Machine %v to become ready...", machine.Name)
		if err := cs.Get(ctx, ctrlclient.ObjectKey{Name: machine.Name, Namespace: machine.Namespace}, machine); err != nil {
			return false, nil
		}

		// TODO: update once machine controllers have a way to indicate a machine has been provisoned. https://github.com/kubernetes-sigs/cluster-api/issues/253
		// Seeing a node cannot be purely relied upon because the provisioned control plane will not be registering with
		// the stack that provisions it.
		ready := machine.Status.NodeRef != nil || len(machine.Annotations) > 0
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

	controlPlane, nodes, err := ExtractControlPlaneMachines(machines)
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "unable to fetch control plane machine in cluster %s/%s", namespace, clusterName)
	}
	return cluster, controlPlane[0], nodes, nil
}

// ExtractControlPlaneMachines separates the machines running the control plane from the incoming machines.
// This is currently done by looking at which machine specifies the control plane version.
func ExtractControlPlaneMachines(machines []*clusterv1.Machine) ([]*clusterv1.Machine, []*clusterv1.Machine, error) {
	nodes := []*clusterv1.Machine{}
	controlPlaneMachines := []*clusterv1.Machine{}
	for _, machine := range machines {
		if util.IsControlPlaneMachine(machine) {
			controlPlaneMachines = append(controlPlaneMachines, machine)
		} else {
			nodes = append(nodes, machine)
		}
	}
	if len(controlPlaneMachines) < 1 {
		return nil, nil, errors.Errorf("expected one or more control plane machines, got: %v", len(controlPlaneMachines))
	}
	return controlPlaneMachines, nodes, nil
}
