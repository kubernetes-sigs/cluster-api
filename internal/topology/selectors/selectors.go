/*
Copyright 2023 The Kubernetes Authors.

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

// Package selectors contains selectors utils.
package selectors

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// ForMachineDeploymentMHC generates a selector for MachineDeployment MHCs.
func ForMachineDeploymentMHC(md *clusterv1.MachineDeployment) *metav1.LabelSelector {
	// The selector returned here is the minimal common selector for all MachineSets belonging to a MachineDeployment.
	// It does not include any labels set in ClusterClass, Cluster Topology or elsewhere.
	return &metav1.LabelSelector{MatchLabels: map[string]string{
		clusterv1.ClusterTopologyOwnedLabel:                 "",
		clusterv1.ClusterTopologyMachineDeploymentNameLabel: md.Spec.Selector.MatchLabels[clusterv1.ClusterTopologyMachineDeploymentNameLabel],
	},
	}
}

// ForControlPlaneMHC generates a selector for control plane MHCs.
func ForControlPlaneMHC() *metav1.LabelSelector {
	// The selector returned here is the minimal common selector for all Machines belonging to the ControlPlane.
	// It does not include any labels set in ClusterClass, Cluster Topology or elsewhere.
	return &metav1.LabelSelector{
		MatchLabels: map[string]string{
			clusterv1.ClusterTopologyOwnedLabel: "",
			clusterv1.MachineControlPlaneLabel:  "",
		},
	}
}
