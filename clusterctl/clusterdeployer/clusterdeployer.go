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

	"github.com/golang/glog"
	"k8s.io/client-go/kubernetes"
	clustercommon "sigs.k8s.io/cluster-api/pkg/apis/cluster/common"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/util"
	"time"
)

// Provider specific logic. Logic here should eventually be optional & additive.
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
	WaitForClusterV1alpha1Ready() error
	GetClusterObjects() ([]*clusterv1.Cluster, error)
	GetMachineObjects() ([]*clusterv1.Machine, error)
	CreateClusterObject(*clusterv1.Cluster) error
	CreateMachineObjects([]*clusterv1.Machine) error
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
	externalProvisioner    ClusterProvisioner
	clientFactory          ClientFactory
	provider               ProviderDeployer
	providerComponents     string
	addonComponents        string
	kubeconfigOutput       string
	cleanupExternalCluster bool
}

func New(
	externalProvisioner ClusterProvisioner,
	clientFactory ClientFactory,
	provider ProviderDeployer,
	providerComponents string,
	addonComponents string,
	kubeconfigOutput string,
	cleanupExternalCluster bool) *ClusterDeployer {
	return &ClusterDeployer{
		externalProvisioner:    externalProvisioner,
		clientFactory:          clientFactory,
		provider:               provider,
		providerComponents:     providerComponents,
		addonComponents:        addonComponents,
		kubeconfigOutput:       kubeconfigOutput,
		cleanupExternalCluster: cleanupExternalCluster,
	}
}

const (
	retryKubeConfigReady   = 10 * time.Second
	timeoutKubeconfigReady = 20 * time.Minute
)

// Creates the a cluster from the provided cluster definition and machine list.
func (d *ClusterDeployer) Create(cluster *clusterv1.Cluster, machines []*clusterv1.Machine, providerComponentsStoreFactory ProviderComponentsStoreFactory) error {
	master, nodes, err := splitMachineRoles(machines)
	if err != nil {
		return fmt.Errorf("unable to seperate master machines from node machines: %v", err)
	}

	glog.Info("Creating external cluster")
	externalClient, cleanupExternalCluster, err := d.createExternalCluster()
	defer cleanupExternalCluster()
	if err != nil {
		return fmt.Errorf("could not create external client: %v", err)
	}
	defer func() {
		err := externalClient.Close()
		if err != nil {
			glog.Errorf("Could not close external client: %v", err)
		}
	}()

	glog.Info("Applying Cluster API stack to external cluster")
	if err := d.applyClusterAPIStack(externalClient); err != nil {
		return fmt.Errorf("unable to apply cluster api stack to external cluster: %v", err)
	}

	glog.Info("Provisioning internal cluster via external cluster")

	glog.Infof("Creating cluster object %v on external cluster", cluster.Name)
	if err := externalClient.CreateClusterObject(cluster); err != nil {
		return fmt.Errorf("unable to create cluster object: %v", err)
	}

	glog.Infof("Creating master %v", master.Name)
	if err := externalClient.CreateMachineObjects([]*clusterv1.Machine{master}); err != nil {
		return fmt.Errorf("unable to create master machine: %v", err)
	}

	glog.Infof("Updating external cluster object with master (%s) endpoint", master.Name)
	if err := d.updateClusterEndpoint(externalClient); err != nil {
		return fmt.Errorf("unable to update external cluster endpoint: %v", err)
	}

	glog.Info("Creating internal cluster")
	internalClient, err := d.createInternalCluster(externalClient)
	if err != nil {
		return fmt.Errorf("unable to create internal cluster: %v", err)
	}
	defer func() {
		err := internalClient.Close()
		if err != nil {
			glog.Errorf("Could not close internal client: %v", err)
		}
	}()

	glog.Info("Applying Cluster API stack to internal cluster")
	if err := d.applyClusterAPIStackWithPivoting(internalClient, externalClient); err != nil {
		return fmt.Errorf("unable to apply cluster api stack to internal cluster: %v", err)
	}

	glog.Info("Saving provider components to the internal cluster")
	err = d.saveProviderComponentsToCluster(providerComponentsStoreFactory, d.kubeconfigOutput)
	if err != nil {
		return fmt.Errorf("unable to save provider components to internal cluster: %v", err)
	}

	// For some reason, endpoint doesn't get updated in external cluster sometimes. So we
	// update the internal cluster endpoint as well to be sure.
	glog.Infof("Updating internal cluster object with master (%s) endpoint", master.Name)
	if err := d.updateClusterEndpoint(internalClient); err != nil {
		return fmt.Errorf("unable to update internal cluster endpoint: %v", err)
	}

	glog.Info("Creating node machines in internal cluster.")
	if err := internalClient.CreateMachineObjects(nodes); err != nil {
		return fmt.Errorf("unable to create node machines: %v", err)
	}

	if d.addonComponents != "" {
		glog.Info("Creating addons in internal cluster.")
		if err := internalClient.Apply(d.addonComponents); err != nil {
			return fmt.Errorf("unable to apply addons: %v", err)
		}
	}

	glog.Infof("Done provisioning cluster. You can now access your cluster with kubectl --kubeconfig %v", d.kubeconfigOutput)

	return nil
}

func (d *ClusterDeployer) createExternalCluster() (ClusterClient, func(), error) {
	cleanupFn := func() {}
	if err := d.externalProvisioner.Create(); err != nil {
		return nil, cleanupFn, fmt.Errorf("could not create external control plane: %v", err)
	}
	if d.cleanupExternalCluster {
		cleanupFn = func() {
			glog.Info("Cleaning up external cluster.")
			d.externalProvisioner.Delete()
		}
	}
	externalKubeconfig, err := d.externalProvisioner.GetKubeconfig()
	if err != nil {
		return nil, cleanupFn, fmt.Errorf("unable to get external cluster kubeconfig: %v", err)
	}
	externalClient, err := d.clientFactory.NewClusterClientFromKubeconfig(externalKubeconfig)
	if err != nil {
		return nil, cleanupFn, fmt.Errorf("unable to create external client: %v", err)
	}
	return externalClient, cleanupFn, nil
}

func (d *ClusterDeployer) createInternalCluster(externalClient ClusterClient) (ClusterClient, error) {
	cluster, master, _, err := getClusterAPIObjects(externalClient)
	if err != nil {
		return nil, err
	}

	glog.V(1).Info("Getting internal cluster kubeconfig.")
	internalKubeconfig, err := waitForKubeconfigReady(d.provider, cluster, master)
	if err != nil {
		return nil, fmt.Errorf("unable to get internal cluster kubeconfig: %v", err)
	}

	err = d.writeKubeconfig(internalKubeconfig)
	if err != nil {
		return nil, err
	}

	internalClient, err := d.clientFactory.NewClusterClientFromKubeconfig(internalKubeconfig)
	if err != nil {
		return nil, fmt.Errorf("unable to create internal cluster client: %v", err)
	}
	return internalClient, nil
}

func (d *ClusterDeployer) updateClusterEndpoint(client ClusterClient) error {
	// Update cluster endpoint. Needed till this logic moves into cluster controller.
	// TODO: https://github.com/kubernetes-sigs/cluster-api/issues/158
	// Fetch fresh objects.
	cluster, master, _, err := getClusterAPIObjects(client)
	if err != nil {
		return err
	}
	masterIP, err := d.provider.GetIP(cluster, master)
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

	glog.Info("Pivoting Cluster API objects from external to internal cluster.")
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
	yaml, err := getApiServerYaml()
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

func (d *ClusterDeployer) writeKubeconfig(kubeconfig string) error {
	const fileMode = 0666
	os.Remove(d.kubeconfigOutput)
	return ioutil.WriteFile(d.kubeconfigOutput, []byte(kubeconfig), fileMode)
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
		return fmt.Errorf("Cluster v1aplpha1 resource not ready on source cluster.")
	}

	if err := to.WaitForClusterV1alpha1Ready(); err != nil {
		return fmt.Errorf("Cluster v1aplpha1 resource not ready on target cluster.")
	}

	clusters, err := from.GetClusterObjects()
	if err != nil {
		return err
	}

	for _, cluster := range clusters {
		// New objects cannot have a specified resource version. Clear it out.
		cluster.SetResourceVersion("")
		err = to.CreateClusterObject(cluster)
		if err != nil {
			return err
		}
		glog.Infof("Moved Cluster '%s'", cluster.GetName())
	}

	machines, err := from.GetMachineObjects()
	if err != nil {
		return err
	}

	for _, machine := range machines {
		// New objects cannot have a specified resource version. Clear it out.
		machine.SetResourceVersion("")
		err = to.CreateMachineObjects([]*clusterv1.Machine{machine})
		if err != nil {
			return err
		}
		glog.Infof("Moved Machine '%s'", machine.GetName())
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
	master, nodes, err := splitMachineRoles(machines)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("unable to fetch master machine: %v", err)
	}
	return cluster, master, nodes, nil
}

// Split the incoming machine set into the master and the non-masters
func splitMachineRoles(machines []*clusterv1.Machine) (*clusterv1.Machine, []*clusterv1.Machine, error) {
	nodes := []*clusterv1.Machine{}
	masters := []*clusterv1.Machine{}
	for _, machine := range machines {
		if containsMasterRole(machine.Spec.Roles) {
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

func containsMasterRole(roles []clustercommon.MachineRole) bool {
	for _, role := range roles {
		if role == clustercommon.MasterRole {
			return true
		}
	}
	return false
}
