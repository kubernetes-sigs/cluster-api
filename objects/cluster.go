/*
Copyright 2019 The Kubernetes Authors.

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

package objects

import (
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	capi "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

const controlPlaneSet = "controlplane"

// GetMachineDeployment returns a worker node machine deployment object
func GetMachineDeployment(name, namespace, clusterName, kubeletVersion string, replicas int32) capi.MachineDeployment {
	labels := map[string]string{
		"cluster.k8s.io/cluster-name": clusterName,
		"set":                         "node",
	}
	return capi.MachineDeployment{
		ObjectMeta: meta.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: capi.MachineDeploymentSpec{
			Replicas: &replicas,
			Selector: meta.LabelSelector{
				MatchLabels: labels,
			},
			Template: capi.MachineTemplateSpec{
				ObjectMeta: meta.ObjectMeta{
					Labels: labels,
				},
				Spec: capi.MachineSpec{
					ProviderSpec: capi.ProviderSpec{},
					Versions: capi.MachineVersionInfo{
						Kubelet: kubeletVersion,
					},
				},
			},
		},
	}
}

// GetCluster returns a cluster object with the given name and namespace
func GetCluster(clusterName, namespace string) capi.Cluster {
	return capi.Cluster{
		ObjectMeta: meta.ObjectMeta{
			Name:      clusterName,
			Namespace: namespace,
		},
		Spec: capi.ClusterSpec{
			ClusterNetwork: capi.ClusterNetworkingConfig{
				Services: capi.NetworkRanges{
					CIDRBlocks: []string{"10.96.0.0/12"},
				},
				Pods: capi.NetworkRanges{
					CIDRBlocks: []string{"192.168.0.0/16"},
				},
				ServiceDomain: "cluster.local",
			},
		},
	}
}

// GetMachine returns a machine with the given parameters
func GetMachine(name, namespace, clusterName, set, version string) capi.Machine {
	machine := capi.Machine{
		ObjectMeta: meta.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"cluster.k8s.io/cluster-name": clusterName,
				"set":                         set,
			},
		},
		Spec: capi.MachineSpec{
			ProviderSpec: capi.ProviderSpec{},
		},
	}
	if set == controlPlaneSet {
		machine.Spec.Versions.ControlPlane = version
	}
	if set == "worker" {
		machine.Spec.Versions.Kubelet = version
	}

	return machine
}
