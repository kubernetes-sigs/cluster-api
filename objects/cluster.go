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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

const controlPlaneSet = "controlplane"

// GetMachineDeployment returns a worker node machine deployment object
func GetMachineDeployment(name, namespace, clusterName, kubeletVersion string, replicas int32) clusterv1alpha1.MachineDeployment {
	labels := map[string]string{
		"cluster.k8s.io/cluster-name": clusterName,
		"set":                         "node",
	}
	return clusterv1alpha1.MachineDeployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "MachineDeployment",
			APIVersion: "cluster.k8s.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: clusterv1alpha1.MachineDeploymentSpec{
			Replicas: &replicas,
			Selector: metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: clusterv1alpha1.MachineTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: clusterv1alpha1.MachineSpec{
					ProviderSpec: clusterv1alpha1.ProviderSpec{},
					Versions: clusterv1alpha1.MachineVersionInfo{
						Kubelet: kubeletVersion,
					},
				},
			},
		},
	}
}

// GetCluster returns a cluster object with the given name and namespace
func GetCluster(clusterName, namespace string) clusterv1alpha1.Cluster {
	return clusterv1alpha1.Cluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Cluster",
			APIVersion: "cluster.k8s.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName,
			Namespace: namespace,
		},
		Spec: clusterv1alpha1.ClusterSpec{
			ClusterNetwork: clusterv1alpha1.ClusterNetworkingConfig{
				Services: clusterv1alpha1.NetworkRanges{
					CIDRBlocks: []string{"10.96.0.0/12"},
				},
				Pods: clusterv1alpha1.NetworkRanges{
					CIDRBlocks: []string{"192.168.0.0/16"},
				},
				ServiceDomain: "cluster.local",
			},
		},
	}
}

// GetMachine returns a machine with the given parameters
func GetMachine(name, namespace, clusterName, set, version string) clusterv1alpha1.Machine {
	machine := clusterv1alpha1.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Machine",
			APIVersion: "cluster.k8s.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"cluster.k8s.io/cluster-name": clusterName,
				"set":                         set,
			},
		},
		Spec: clusterv1alpha1.MachineSpec{
			ProviderSpec: clusterv1alpha1.ProviderSpec{},
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
