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
	"strings"
	"time"

	"k8s.io/client-go/kubernetes"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/deployer"
	"sigs.k8s.io/cluster-api/pkg/util"

	"github.com/golang/glog"
)

// Deprecated interface for Provider specific logic. Please do not extend or add. This interface should be removed
// once issues/158 and issues/160 below are fixed.
type ProviderDeployer interface {
	// TODO: This requirement can be removed once after: https://github.com/kubernetes-sigs/cluster-api/issues/158
	GetIP(cluster *clusterv1.Cluster, machine *clusterv1.Machine) (string, error)
	// TODO: This requirement can be removed after: https://github.com/kubernetes-sigs/cluster-api/issues/160
	GetKubeConfig(cluster *clusterv1.Cluster, master *clusterv1.Machine) (string, error)
}

// Can provision a kubernetes cluster
type ClusterProvisioner interface {
	Create() error
	Delete() error
	GetKubeconfig() (string, error)
}

// Provides interaction with a cluster
type ClusterClient interface {
	Apply(string) error
	Delete(string) error
	WaitForClusterV1alpha1Ready() error
	GetClusterObjects() ([]*clusterv1.Cluster, error)
	GetMachineDeploymentObjects() ([]*clusterv1.MachineDeployment, error)
	GetMachineSetObjects() ([]*clusterv1.MachineSet, error)
	GetMachineObjects() ([]*clusterv1.Machine, error)
	CreateClusterObject(*clusterv1.Cluster) error
	CreateMachineDeploymentObjects([]*clusterv1.MachineDeployment) error
	CreateMachineSetObjects([]*clusterv1.MachineSet) error
	CreateMachineObjects([]*clusterv1.Machine) error
	DeleteClusterObjects() error
	DeleteMachineDeploymentObjects() error
	DeleteMachineSetObjects() error
	DeleteMachineObjects() error
	UpdateClusterObjectEndpoint(string) error
	Close() error
}

// Can create cluster clients
type ClientFactory interface {
	NewClusterClientFromKubeconfig(string) (ClusterClient, error)
	NewCoreClientsetFromKubeconfigFile(string) (*kubernetes.Clientset, error)
}

type ProviderComponentsStore interface {
	Save(providerComponents string) error
	Load() (string, error)
}

type ProviderComponentsStoreFactory interface {
	NewFromCoreClientset(clientset *kubernetes.Clientset) (ProviderComponentsStore, error)
}

type ClusterDeployer struct {
	bootstrapProvisioner    ClusterProvisioner
	clientFactory           ClientFactory
	providerComponents      string
	addonComponents         string
	cleanupBootstrapCluster bool
}

func New(
	bootstrapProvisioner ClusterProvisioner,
	clientFactory ClientFactory,
	providerComponents string,
	addonComponents string,
	cleanupBootstrapCluster bool) *ClusterDeployer {
	return &ClusterDeployer{
		bootstrapProvisioner:    bootstrapProvisioner,
		clientFactory:           clientFactory,
		providerComponents:      providerComponents,
		addonComponents:         addonComponents,
		cleanupBootstrapCluster: cleanupBootstrapCluster,
	}
}

const (
	retryKubeConfigReady   = 10 * time.Second
	timeoutKubeconfigReady = 20 * time.Minute
)

// Creates the a cluster from the provided cluster definition and machine list.

func (d *ClusterDeployer) Create(cluster *clusterv1.Cluster, machines []*clusterv1.Machine, provider ProviderDeployer, kubeconfigOutput string, providerComponentsStoreFactory ProviderComponentsStoreFactory) error {
	master, nodes, err := extractMasterMachine(machines)
	if err != nil {
		return fmt.Errorf("unable to seperate master machines from node machines: %v", err)
	}

	glog.Info("Creating bootstrap cluster")
	bootstrapClient, cleanupBootstrapCluster, err := d.createBootstrapCluster()
	defer cleanupBootstrapCluster()
	if err != nil {
		return fmt.Errorf("could not create bootstrap cluster: %v", err)
	}
	defer closeClient(bootstrapClient, "bootstrap")

	glog.Info("Applying Cluster API stack to bootstrap cluster")
	if err := d.applyClusterAPIStack(bootstrapClient); err != nil {
		return fmt.Errorf("unable to apply cluster api stack to bootstrap cluster: %v", err)
	}

	glog.Info("Provisioning target cluster via bootstrap cluster")

	glog.Infof("Creating cluster object %v on bootstrap cluster", cluster.Name)
	if err := bootstrapClient.CreateClusterObject(cluster); err != nil {
		return fmt.Errorf("unable to create cluster object: %v", err)
	}

	glog.Infof("Creating master %v", master.Name)
	if err := bootstrapClient.CreateMachineObjects([]*clusterv1.Machine{master}); err != nil {
		return fmt.Errorf("unable to create master machine: %v", err)
	}

	glog.Infof("Updating bootstrap cluster object with master (%s) endpoint", master.Name)
	if err := d.updateClusterEndpoint(bootstrapClient, provider); err != nil {
		return fmt.Errorf("unable to update bootstrap cluster endpoint: %v", err)
	}

	glog.Info("Creating target cluster")
	targetClient, err := d.createTargetClusterClient(bootstrapClient, provider, kubeconfigOutput)
	if err != nil {
		return fmt.Errorf("unable to create target cluster: %v", err)
	}
	defer closeClient(targetClient, "target")

	glog.Info("Applying Cluster API stack to target cluster")
	if err := d.applyClusterAPIStackWithPivoting(targetClient, bootstrapClient); err != nil {
		return fmt.Errorf("unable to apply cluster api stack to target cluster: %v", err)
	}

	glog.Info("Saving provider components to the target cluster")
	err = d.saveProviderComponentsToCluster(providerComponentsStoreFactory, kubeconfigOutput)
	if err != nil {
		return fmt.Errorf("unable to save provider components to target cluster: %v", err)
	}

	// For some reason, endpoint doesn't get updated in bootstrap cluster sometimes. So we
	// update the target cluster endpoint as well to be sure.
	glog.Infof("Updating target cluster object with master (%s) endpoint", master.Name)
	if err := d.updateClusterEndpoint(targetClient, provider); err != nil {
		return fmt.Errorf("unable to update target cluster endpoint: %v", err)
	}

	glog.Info("Creating node machines in target cluster.")
	if err := targetClient.CreateMachineObjects(nodes); err != nil {
		return fmt.Errorf("unable to create node machines: %v", err)
	}

	if d.addonComponents != "" {
		glog.Info("Creating addons in target cluster.")
		if err := targetClient.Apply(d.addonComponents); err != nil {
			return fmt.Errorf("unable to apply addons: %v", err)
		}
	}

	glog.Infof("Done provisioning cluster. You can now access your cluster with kubectl --kubeconfig %v", kubeconfigOutput)

	return nil
}

func (d *ClusterDeployer) Delete(targetClient ClusterClient) error {
	glog.Info("Creating bootstrap cluster")
	bootstrapClient, cleanupBootstrapCluster, err := d.createBootstrapCluster()
	defer cleanupBootstrapCluster()
	if err != nil {
		return fmt.Errorf("could not create bootstrap cluster: %v", err)
	}
	defer closeClient(bootstrapClient, "bootstrap")

	glog.Info("Applying Cluster API stack to bootstrap cluster")
	if err = d.applyClusterAPIStack(bootstrapClient); err != nil {
		return fmt.Errorf("unable to apply cluster api stack to bootstrap cluster: %v", err)
	}

	glog.Info("Deleting Cluster API Provider Components from target cluster")
	if err = targetClient.Delete(d.providerComponents); err != nil {
		glog.Infof("error while removing provider components from target cluster: %v", err)
		glog.Infof("Continuing with a best effort delete")
	}

	glog.Info("Copying objects from target cluster to bootstrap cluster")
	if err = pivot(targetClient, bootstrapClient); err != nil {
		return fmt.Errorf("unable to copy objects from target to bootstrap cluster: %v", err)
	}

	glog.Info("Deleting objects from bootstrap cluster")
	if err = deleteObjects(bootstrapClient); err != nil {
		return fmt.Errorf("unable to finish deleting objects in bootstrap cluster, resources may have been leaked: %v", err)
	}
	glog.Info("Deletion of cluster complete")

	return nil
}

func (d *ClusterDeployer) createBootstrapCluster() (ClusterClient, func(), error) {
	cleanupFn := func() {}
	if err := d.bootstrapProvisioner.Create(); err != nil {
		return nil, cleanupFn, fmt.Errorf("could not create bootstrap control plane: %v", err)
	}

	if d.cleanupBootstrapCluster {
		cleanupFn = func() {
			glog.Info("Cleaning up bootstrap cluster.")
			d.bootstrapProvisioner.Delete()
		}
	}

	bootstrapKubeconfig, err := d.bootstrapProvisioner.GetKubeconfig()
	if err != nil {
		return nil, cleanupFn, fmt.Errorf("unable to get bootstrap cluster kubeconfig: %v", err)
	}
	bootstrapClient, err := d.clientFactory.NewClusterClientFromKubeconfig(bootstrapKubeconfig)
	if err != nil {
		return nil, cleanupFn, fmt.Errorf("unable to create bootstrap client: %v", err)
	}

	return bootstrapClient, cleanupFn, nil
}

func (d *ClusterDeployer) createTargetClusterClient(bootstrapClient ClusterClient, provider ProviderDeployer, kubeconfigOutput string) (ClusterClient, error) {
	cluster, master, _, err := getClusterAPIObjects(bootstrapClient)
	if err != nil {
		return nil, err
	}

	glog.V(1).Info("Getting target cluster kubeconfig.")
	targetKubeconfig, err := waitForKubeconfigReady(provider, cluster, master)
	if err != nil {
		return nil, fmt.Errorf("unable to get target cluster kubeconfig: %v", err)
	}

	if err = d.writeKubeconfig(targetKubeconfig, kubeconfigOutput); err != nil {
		return nil, err
	}

	targetClient, err := d.clientFactory.NewClusterClientFromKubeconfig(targetKubeconfig)
	if err != nil {
		return nil, fmt.Errorf("unable to create target cluster client: %v", err)
	}

	return targetClient, nil
}

func (d *ClusterDeployer) updateClusterEndpoint(client ClusterClient, provider ProviderDeployer) error {
	// Update cluster endpoint. Needed till this logic moves into cluster controller.
	// TODO: https://github.com/kubernetes-sigs/cluster-api/issues/158
	// Fetch fresh objects.
	cluster, master, _, err := getClusterAPIObjects(client)
	if err != nil {
		return err
	}
	masterIP, err := provider.GetIP(cluster, master)
	if err != nil {
		return fmt.Errorf("unable to get master IP: %v", err)
	}
	err = client.UpdateClusterObjectEndpoint(masterIP)
	if err != nil {
		return fmt.Errorf("unable to update cluster endpoint: %v", err)
	}
	return nil
}

func (d *ClusterDeployer) saveProviderComponentsToCluster(factory ProviderComponentsStoreFactory, kubeconfigPath string) error {
	clientset, err := d.clientFactory.NewCoreClientsetFromKubeconfigFile(kubeconfigPath)
	if err != nil {
		return fmt.Errorf("error creating core clientset: %v", err)
	}
	pcStore, err := factory.NewFromCoreClientset(clientset)
	if err != nil {
		return fmt.Errorf("unable to create provider components store: %v", err)
	}
	err = pcStore.Save(d.providerComponents)
	if err != nil {
		return fmt.Errorf("error saving provider components: %v", err)
	}
	return nil
}

func (d *ClusterDeployer) applyClusterAPIStack(client ClusterClient) error {
	glog.Info("Applying Cluster API APIServer")
	err := d.applyClusterAPIApiserver(client)
	if err != nil {
		return fmt.Errorf("unable to apply cluster apiserver: %v", err)
	}

	glog.Info("Applying Cluster API Provider Components")
	err = d.applyClusterAPIControllers(client)
	if err != nil {
		return fmt.Errorf("unable to apply cluster api controllers: %v", err)
	}
	return nil
}

func (d *ClusterDeployer) applyClusterAPIStackWithPivoting(client ClusterClient, source ClusterClient) error {
	glog.Info("Applying Cluster API APIServer")
	err := d.applyClusterAPIApiserver(client)
	if err != nil {
		return fmt.Errorf("unable to apply cluster api apiserver: %v", err)
	}

	glog.Info("Pivoting Cluster API objects from bootstrap to target cluster.")
	err = pivot(source, client)
	if err != nil {
		return fmt.Errorf("unable to pivot cluster API objects: %v", err)
	}

	glog.Info("Applying Cluster API Provider Components.")
	err = d.applyClusterAPIControllers(client)
	if err != nil {
		return fmt.Errorf("unable to apply cluster api controllers: %v", err)
	}

	return nil
}

func (d *ClusterDeployer) applyClusterAPIApiserver(client ClusterClient) error {
	yaml, err := deployer.GetApiServerYaml()
	if err != nil {
		return fmt.Errorf("unable to generate apiserver yaml: %v", err)
	}

	err = client.Apply(yaml)
	if err != nil {
		return fmt.Errorf("unable to apply apiserver yaml: %v", err)
	}
	return client.WaitForClusterV1alpha1Ready()
}

func (d *ClusterDeployer) applyClusterAPIControllers(client ClusterClient) error {
	return client.Apply(d.providerComponents)
}

func (d *ClusterDeployer) writeKubeconfig(kubeconfig string, kubeconfigOutput string) error {
	const fileMode = 0666
	os.Remove(kubeconfigOutput)
	return ioutil.WriteFile(kubeconfigOutput, []byte(kubeconfig), fileMode)
}

func waitForKubeconfigReady(provider ProviderDeployer, cluster *clusterv1.Cluster, machine *clusterv1.Machine) (string, error) {
	kubeconfig := ""
	err := util.PollImmediate(retryKubeConfigReady, timeoutKubeconfigReady, func() (bool, error) {
		glog.V(2).Infof("Waiting for kubeconfig on %v to become ready...", machine.Name)
		k, err := provider.GetKubeConfig(cluster, machine)
		if err != nil {
			glog.V(4).Infof("error getting kubeconfig: %v", err)
			return false, nil
		}
		if k == "" {
			return false, nil
		}
		kubeconfig = k
		return true, nil
	})

	return kubeconfig, err
}

func pivot(from, to ClusterClient) error {
	if err := from.WaitForClusterV1alpha1Ready(); err != nil {
		return fmt.Errorf("cluster v1alpha1 resource not ready on source cluster")
	}

	if err := to.WaitForClusterV1alpha1Ready(); err != nil {
		return fmt.Errorf("cluster v1alpha1 resource not ready on target cluster")
	}

	clusters, err := from.GetClusterObjects()
	if err != nil {
		return err
	}

	for _, cluster := range clusters {
		// New objects cannot have a specified resource version. Clear it out.
		cluster.SetResourceVersion("")
		if err = to.CreateClusterObject(cluster); err != nil {
			return fmt.Errorf("error moving Cluster '%v': %v", cluster.GetName(), err)
		}
		glog.Infof("Moved Cluster '%s'", cluster.GetName())
	}

	fromDeployments, err := from.GetMachineDeploymentObjects()
	if err != nil {
		return err
	}
	for _, deployment := range fromDeployments {
		// New objects cannot have a specified resource version. Clear it out.
		deployment.SetResourceVersion("")
		if err = to.CreateMachineDeploymentObjects([]*clusterv1.MachineDeployment{deployment}); err != nil {
			return fmt.Errorf("error moving MachineDeployment '%v': %v", deployment.GetName(), err)
		}
		glog.Infof("Moved MachineDeployment %v", deployment.GetName())
	}

	fromMachineSets, err := from.GetMachineSetObjects()
	if err != nil {
		return err
	}
	for _, machineSet := range fromMachineSets {
		// New objects cannot have a specified resource version. Clear it out.
		machineSet.SetResourceVersion("")
		if err := to.CreateMachineSetObjects([]*clusterv1.MachineSet{machineSet}); err != nil {
			return fmt.Errorf("error moving MachineSet '%v': %v", machineSet.GetName(), err)
		}
		glog.Infof("Moved MachineSet %v", machineSet.GetName())
	}

	machines, err := from.GetMachineObjects()
	if err != nil {
		return err
	}

	for _, machine := range machines {
		// New objects cannot have a specified resource version. Clear it out.
		machine.SetResourceVersion("")
		if err = to.CreateMachineObjects([]*clusterv1.Machine{machine}); err != nil {
			return fmt.Errorf("error moving Machine '%v': %v", machine.GetName(), err)
		}
		glog.Infof("Moved Machine '%s'", machine.GetName())
	}
	return nil
}

func deleteObjects(client ClusterClient) error {
	var errors []string
	glog.Infof("Deleting machine deployments")
	if err := client.DeleteMachineDeploymentObjects(); err != nil {
		err = fmt.Errorf("error deleting machine deployments: %v", err)
		errors = append(errors, err.Error())
	}
	glog.Infof("Deleting machine sets")
	if err := client.DeleteMachineSetObjects(); err != nil {
		err = fmt.Errorf("error deleting machine sets: %v", err)
		errors = append(errors, err.Error())
	}
	glog.Infof("Deleting machines")
	if err := client.DeleteMachineObjects(); err != nil {
		err = fmt.Errorf("error deleting machines: %v", err)
		errors = append(errors, err.Error())
	}
	glog.Infof("Deleting clusters")
	if err := client.DeleteClusterObjects(); err != nil {
		err = fmt.Errorf("error deleting clusters: %v", err)
		errors = append(errors, err.Error())
	}
	if len(errors) > 0 {
		return fmt.Errorf("error(s) encountered deleting objects from bootstrap cluster: [%v]", strings.Join(errors, ", "))
	}
	return nil
}

func getClusterAPIObjects(client ClusterClient) (*clusterv1.Cluster, *clusterv1.Machine, []*clusterv1.Machine, error) {
	machines, err := client.GetMachineObjects()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("unable to fetch machines: %v", err)
	}
	clusters, err := client.GetClusterObjects()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("unable to fetch clusters: %v", err)
	}
	if len(clusters) != 1 {
		return nil, nil, nil, fmt.Errorf("fetched not exactly one cluster object. Count %v", len(clusters))
	}
	cluster := clusters[0]
	master, nodes, err := extractMasterMachine(machines)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("unable to fetch master machine: %v", err)
	}
	return cluster, master, nodes, nil
}

// extractMasterMachine separates the master (singular) from the incoming machines.
// This is currently done by looking at which machine specifies the control plane version
// (which implies that it is a master). This should be cleaned up in the future.
func extractMasterMachine(machines []*clusterv1.Machine) (*clusterv1.Machine, []*clusterv1.Machine, error) {
	nodes := []*clusterv1.Machine{}
	masters := []*clusterv1.Machine{}
	for _, machine := range machines {
		if util.IsMaster(machine) {
			masters = append(masters, machine)
		} else {
			nodes = append(nodes, machine)
		}
	}
	if len(masters) != 1 {
		return nil, nil, fmt.Errorf("expected one master, got: %v", len(masters))
	}
	return masters[0], nodes, nil
}

func closeClient(client ClusterClient, name string) {
	if err := client.Close(); err != nil {
		glog.Errorf("Could not close %v client: %v", name, err)
	}
}
