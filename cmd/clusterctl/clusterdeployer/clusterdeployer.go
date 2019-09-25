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

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha2"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/clusterdeployer/bootstrap"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/clusterdeployer/clusterclient"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/clusterdeployer/provider"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/phases"
	"sigs.k8s.io/cluster-api/util/yaml"
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
func (d *ClusterDeployer) Create(resources *yaml.ParseOutput, kubeconfigOutput string, providerComponentsStoreFactory provider.ComponentsStoreFactory) error {
	cluster := resources.Clusters[0]
	machines := resources.Machines

	controlPlaneMachines, nodes, err := clusterclient.ExtractControlPlaneMachines(machines)
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
	if err := phases.ApplyCluster(
		bootstrapClient,
		cluster,
		yaml.ExtractClusterReferences(resources, cluster)...); err != nil {
		return errors.Wrapf(err, "unable to create cluster %q in bootstrap cluster", cluster.Name)
	}

	firstControlPlane := controlPlaneMachines[0]
	klog.Infof("Creating control plane machine %q in namespace %q", firstControlPlane.Name, cluster.Namespace)
	if err := phases.ApplyMachines(
		bootstrapClient,
		cluster.Namespace,
		[]*clusterv1.Machine{firstControlPlane},
		yaml.ExtractMachineReferences(resources, firstControlPlane)...); err != nil {
		return errors.Wrap(err, "unable to create control plane machine")
	}

	klog.Info("Creating target cluster")
	targetKubeconfig, err := phases.GetKubeconfig(bootstrapClient, kubeconfigOutput, cluster.Name, cluster.Namespace)
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

	if len(controlPlaneMachines) > 1 {
		// TODO(h0tbird) Done serially until kubernetes/kubeadm#1097 is resolved and all
		// supported versions of k8s we are deploying (using kubeadm) have the fix.
		klog.Info("Creating additional control plane machines in target cluster.")
		for _, controlPlaneMachine := range controlPlaneMachines[1:] {
			if err := phases.ApplyMachines(
				targetClient,
				cluster.Namespace,
				[]*clusterv1.Machine{controlPlaneMachine},
				yaml.ExtractMachineReferences(resources, controlPlaneMachine)...,
			); err != nil {
				return errors.Wrap(err, "unable to create additional control plane machines")
			}
		}
	}

	klog.Info("Creating node machines in target cluster.")
	extraMachineResources := []*unstructured.Unstructured{}
	for _, m := range nodes {
		extraMachineResources = append(extraMachineResources, yaml.ExtractMachineReferences(resources, m)...)
	}
	if err := phases.ApplyMachines(
		targetClient,
		cluster.Namespace,
		nodes,
		extraMachineResources...,
	); err != nil {
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
	klog.Infof("Deleting Clusters in all namespaces")
	if err := client.DeleteClusters(""); err != nil {
		return errors.Errorf("error encountered deleting objects from bootstrap cluster: [error deleting Clusters: %v]", err)
	}
	return nil
}

func closeClient(client clusterclient.Client, name string) {
	if client != nil {
		if err := client.Close(); err != nil {
			klog.Errorf("Could not close %v client: %v", name, err)
		}
	}
}
