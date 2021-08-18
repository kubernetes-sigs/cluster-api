/*
Copyright 2021 The Kubernetes Authors.

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

package topology

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
)

// clusterTopologyClass holds all the objects required for computing the desired state of a managed Cluster topology.
type clusterTopologyClass struct {
	clusterClass                  *clusterv1.ClusterClass
	infrastructureClusterTemplate *unstructured.Unstructured
	controlPlane                  *controlPlaneTopologyClass
	machineDeployments            map[string]*machineDeploymentTopologyClass
}

// controlPlaneTopologyClass holds the templates required for computing the desired state of a managed control plane.
type controlPlaneTopologyClass struct {
	template                      *unstructured.Unstructured
	infrastructureMachineTemplate *unstructured.Unstructured
}

// machineDeploymentTopologyClass holds the templates required for computing the desired state of a managed deployment.
type machineDeploymentTopologyClass struct {
	metadata                      clusterv1.ObjectMeta
	bootstrapTemplate             *unstructured.Unstructured
	infrastructureMachineTemplate *unstructured.Unstructured
}

// clusterTopologyState holds all the objects representing the state of a managed Cluster topology.
// NOTE: please note that we are going to deal with two different type state, the current state as read from the API server,
// and the desired state resulting from processing the clusterTopologyClass.
type clusterTopologyState struct {
	cluster               *clusterv1.Cluster
	infrastructureCluster *unstructured.Unstructured
	controlPlane          *controlPlaneTopologyState
	machineDeployments    map[string]*machineDeploymentTopologyState
}

// controlPlaneTopologyState all the objects representing the state of a managed control plane.
type controlPlaneTopologyState struct {
	object                        *unstructured.Unstructured
	infrastructureMachineTemplate *unstructured.Unstructured
}

// machineDeploymentTopologyState all the objects representing the state of a managed deployment.
type machineDeploymentTopologyState struct {
	object                        *clusterv1.MachineDeployment
	bootstrapTemplate             *unstructured.Unstructured
	infrastructureMachineTemplate *unstructured.Unstructured
}
