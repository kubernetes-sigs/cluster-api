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

package deploy

import (
	"k8s.io/client-go/kubernetes"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/controller/machine"
)

// Provider-specific machine logic the deployer needs.
type machineDeployer interface {
	machine.Actuator
	GetIP(cluster *clusterv1.Cluster, machine *clusterv1.Machine) (string, error)
	GetKubeConfig(cluster *clusterv1.Cluster, master *clusterv1.Machine) (string, error)

	// Provision infrastructure that the cluster needs before it
	// can be created
	ProvisionClusterDependencies(*clusterv1.Cluster) error
	// Create and start the machine controller. The list of initial
	// machines don't have to be reconciled as part of this function, but
	// are provided in case the function wants to refer to them (and their
	// ProviderConfigs) to know how to configure the machine controller.
	// Not idempotent.
	CreateMachineController(cluster *clusterv1.Cluster, initialMachines []*clusterv1.Machine, clientSet kubernetes.Clientset) error
	// Create GCE and kubernetes resources after the cluster is created
	PostCreate(cluster *clusterv1.Cluster) error
	PostDelete(cluster *clusterv1.Cluster) error
}
