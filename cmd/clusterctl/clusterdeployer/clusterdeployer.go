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
	"strings"

	"github.com/pkg/errors"
	"k8s.io/klog"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/clusterdeployer/bootstrap"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/clusterdeployer/clusterclient"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/clusterdeployer/provider"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/phases"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

type ClusterDeployer struct {
	bootstrapProvisioner    bootstrap.ClusterProvisioner
	clientFactory           clusterclient.Factory
	providerComponents      string
	addonComponents         string
	bootstrapComponents     string
	cleanupBootstrapCluster bool
}

func New(
	bootstrapProvisioner bootstrap.ClusterProvisioner,
	clientFactory clusterclient.Factory,
	providerComponents string,
	addonComponents string,
	bootstrapComponents string,
	cleanupBootstrapCluster bool) *ClusterDeployer {
	return &ClusterDeployer{
		bootstrapProvisioner:    bootstrapProvisioner,
		clientFactory:           clientFactory,
		providerComponents:      providerComponents,
		addonComponents:         addonComponents,
		bootstrapComponents:     bootstrapComponents,
		cleanupBootstrapCluster: cleanupBootstrapCluster,
	}
}

// Create the cluster from the provided cluster definition and machine list.
func (d *ClusterDeployer) Create(cluster *clusterv1.Cluster, machines []*clusterv1.Machine, provider provider.Deployer, kubeconfigOutput string, providerComponentsStoreFactory provider.ComponentsStoreFactory) error {
	controlPlaneMachine, nodes, err := clusterclient.ExtractControlPlaneMachine(machines)
	if err != nil {
		return errors.Wrap(err, "unable to separate control plane machines from node machines")
	}

	bootstrapClient, cleanupBootstrapCluster, err := phases.CreateBootstrapCluster(d.bootstrapProvisioner, d.cleanupBootstrapCluster, d.clientFactory)
	defer cleanupBootstrapCluster()
	if err != nil {
		return errors.Wrap(err, "could not create bootstrap cluster")
	}
	defer closeClient(bootstrapClient, "bootstrap")

	if d.bootstrapComponents != "" {
		if err := phases.ApplyBootstrapComponents(bootstrapClient, d.bootstrapComponents); err != nil {
			return errors.Wrap(err, "unable to apply bootstrap components to bootstrap cluster")
		}
	}

	klog.Info("Applying Cluster API stack to bootstrap cluster")
	if err := phases.ApplyClusterAPIComponents(bootstrapClient, d.providerComponents); err != nil {
		return errors.Wrap(err, "unable to apply cluster api stack to bootstrap cluster")
	}

	klog.Info("Provisioning target cluster via bootstrap cluster")
	if err := phases.ApplyCluster(bootstrapClient, cluster); err != nil {
		return errors.Wrapf(err, "unable to create cluster %q in bootstrap cluster", cluster.Name)
	}

	if cluster.Namespace == "" {
		cluster.Namespace = bootstrapClient.GetContextNamespace()
	}

	klog.Infof("Creating control plane %v in namespace %q", controlPlaneMachine.Name, cluster.Namespace)
	if err := phases.ApplyMachines(bootstrapClient, cluster.Namespace, []*clusterv1.Machine{controlPlaneMachine}); err != nil {
		return errors.Wrap(err, "unable to create control plane machine")
	}

	klog.Infof("Updating bootstrap cluster object for cluster %v in namespace %q with control plane endpoint running on %s", cluster.Name, cluster.Namespace, controlPlaneMachine.Name)
	if err := d.updateClusterEndpoint(bootstrapClient, provider, cluster.Name, cluster.Namespace); err != nil {
		return errors.Wrap(err, "unable to update bootstrap cluster endpoint")
	}

	klog.Info("Creating target cluster")
	targetKubeconfig, err := phases.GetKubeconfig(bootstrapClient, provider, kubeconfigOutput, cluster.Name, cluster.Namespace)
	if err != nil {
		return fmt.Errorf("unable to create target cluster kubeconfig: %v", err)
	}

	targetClient, err := d.clientFactory.NewClientFromKubeconfig(targetKubeconfig)
	if err != nil {
		return errors.Wrap(err, "unable to create target cluster client")
	}
	defer closeClient(targetClient, "target")

	if d.addonComponents != "" {
		if err := phases.ApplyAddons(targetClient, d.addonComponents); err != nil {
			return errors.Wrap(err, "unable to apply addons to target cluster")
		}
	}

	klog.Info("Pivoting Cluster API stack to target cluster")
	if err := phases.Pivot(bootstrapClient, targetClient, d.providerComponents); err != nil {
		return errors.Wrap(err, "unable to pivot cluster api stack to target cluster")
	}

	klog.Info("Saving provider components to the target cluster")
	err = d.saveProviderComponentsToCluster(providerComponentsStoreFactory, kubeconfigOutput)
	if err != nil {
		return errors.Wrap(err, "unable to save provider components to target cluster")
	}

	// For some reason, endpoint doesn't get updated in bootstrap cluster sometimes. So we
	// update the target cluster endpoint as well to be sure.
	klog.Infof("Updating target cluster object with control plane endpoint running on %s", controlPlaneMachine.Name)
	if err := d.updateClusterEndpoint(targetClient, provider, cluster.Name, cluster.Namespace); err != nil {
		return errors.Wrap(err, "unable to update target cluster endpoint")
	}

	klog.Info("Creating node machines in target cluster.")
	if err := phases.ApplyMachines(targetClient, cluster.Namespace, nodes); err != nil {
		return errors.Wrap(err, "unable to create node machines")
	}

	klog.Infof("Done provisioning cluster. You can now access your cluster with kubectl --kubeconfig %v", kubeconfigOutput)

	return nil
}

func (d *ClusterDeployer) Delete(targetClient clusterclient.Client) error {
	klog.Info("Creating bootstrap cluster")
	bootstrapClient, cleanupBootstrapCluster, err := phases.CreateBootstrapCluster(d.bootstrapProvisioner, d.cleanupBootstrapCluster, d.clientFactory)
	defer cleanupBootstrapCluster()
	if err != nil {
		return errors.Wrap(err, "could not create bootstrap cluster")
	}
	defer closeClient(bootstrapClient, "bootstrap")

	klog.Info("Pivoting Cluster API stack to bootstrap cluster")
	if err := phases.Pivot(targetClient, bootstrapClient, d.providerComponents); err != nil {
		return errors.Wrap(err, "unable to pivot Cluster API stack to bootstrap cluster")
	}

	// Verify that all pivoted resources have a status
	if err := bootstrapClient.WaitForResourceStatuses(); err != nil {
		return errors.Wrap(err, "error while waiting for Cluster API resources to contain statuses")
	}

	klog.Info("Deleting objects from bootstrap cluster")
	if err := deleteClusterAPIObjectsInAllNamespaces(bootstrapClient); err != nil {
		return errors.Wrap(err, "unable to finish deleting objects in bootstrap cluster, resources may have been leaked")
	}

	klog.Info("Deletion of cluster complete")

	return nil
}

func (d *ClusterDeployer) updateClusterEndpoint(client clusterclient.Client, provider provider.Deployer, clusterName, namespace string) error {
	// Update cluster endpoint. Needed till this logic moves into cluster controller.
	// TODO: https://github.com/kubernetes-sigs/cluster-api/issues/158
	// Fetch fresh objects.
	cluster, controlPlane, _, err := clusterclient.GetClusterAPIObject(client, clusterName, namespace)
	if err != nil {
		return err
	}
	clusterEndpoint, err := provider.GetIP(cluster, controlPlane)
	if err != nil {
		return errors.Wrap(err, "unable to get cluster endpoint")
	}
	err = client.UpdateClusterObjectEndpoint(clusterEndpoint, clusterName, namespace)
	if err != nil {
		return errors.Wrap(err, "unable to update cluster endpoint")
	}
	return nil
}

func (d *ClusterDeployer) saveProviderComponentsToCluster(factory provider.ComponentsStoreFactory, kubeconfigPath string) error {
	clientset, err := d.clientFactory.NewCoreClientsetFromKubeconfigFile(kubeconfigPath)
	if err != nil {
		return errors.Wrap(err, "error creating core clientset")
	}
	pcStore, err := factory.NewFromCoreClientset(clientset)
	if err != nil {
		return errors.Wrap(err, "unable to create provider components store")
	}
	err = pcStore.Save(d.providerComponents)
	if err != nil {
		return errors.Wrap(err, "error saving provider components")
	}
	return nil
}

func deleteClusterAPIObjectsInAllNamespaces(client clusterclient.Client) error {
	var errorList []string
	klog.Infof("Deleting MachineDeployments in all namespaces")
	if err := client.DeleteMachineDeployments(""); err != nil {
		err = errors.Wrap(err, "error deleting MachineDeployments")
		errorList = append(errorList, err.Error())
	}
	klog.Infof("Deleting MachineSets in all namespaces")
	if err := client.DeleteMachineSets(""); err != nil {
		err = errors.Wrap(err, "error deleting MachineSets")
		errorList = append(errorList, err.Error())
	}
	klog.Infof("Deleting Machines in all namespaces")
	if err := client.DeleteMachines(""); err != nil {
		err = errors.Wrap(err, "error deleting Machines")
		errorList = append(errorList, err.Error())
	}
	klog.Infof("Deleting MachineClasses in all namespaces")
	if err := client.DeleteMachineClasses(""); err != nil {
		err = errors.Wrap(err, "error deleting MachineClasses")
		errorList = append(errorList, err.Error())
	}
	klog.Infof("Deleting Clusters in all namespaces")
	if err := client.DeleteClusters(""); err != nil {
		err = errors.Wrap(err, "error deleting Clusters")
		errorList = append(errorList, err.Error())
	}
	if len(errorList) > 0 {
		return errors.Errorf("error(s) encountered deleting objects from bootstrap cluster: [%v]", strings.Join(errorList, ", "))
	}
	return nil
}

func closeClient(client clusterclient.Client, name string) {
	if err := client.Close(); err != nil {
		klog.Errorf("Could not close %v client: %v", name, err)
	}
}
