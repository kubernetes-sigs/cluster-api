/*
Copyright 2020 The Kubernetes Authors.

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

package internal

import (
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha4"
)

// ControlPlaneMachineLabelsForCluster returns a set of labels to add to a control plane machine for this specific cluster.
func ControlPlaneMachineLabelsForCluster(kcp *controlplanev1.KubeadmControlPlane, clusterName string) map[string]string {
	labels := kcp.Spec.MachineTemplate.ObjectMeta.Labels
	if labels == nil {
		labels = map[string]string{}
	}

	// Always force these labels over the ones coming from the spec.
	labels[clusterv1.ClusterLabelName] = clusterName
	labels[clusterv1.MachineControlPlaneLabelName] = ""
	return labels
}
