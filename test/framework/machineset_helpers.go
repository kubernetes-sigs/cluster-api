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

package framework

import (
	"context"

	. "github.com/onsi/gomega"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// GetMachineSetsByDeploymentInput is the input for GetMachineSetsByDeployment.
type GetMachineSetsByDeploymentInput struct {
	Lister    Lister
	MDName    string
	Namespace string
}

// GetMachineSetsByDeployment returns the MachineSets objects for a MachineDeployment.
// Important! this method relies on labels that are created by the CAPI controllers during the first reconciliation, so
// it is necessary to ensure this is already happened before calling it.
func GetMachineSetsByDeployment(ctx context.Context, input GetMachineSetsByDeploymentInput) []*clusterv1.MachineSet {
	machineSetList := &clusterv1.MachineSetList{}
	Eventually(func() error {
		return input.Lister.List(ctx, machineSetList, client.InNamespace(input.Namespace), client.MatchingLabels{clusterv1.MachineDeploymentNameLabel: input.MDName})
	}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to list MachineSets for MachineDeployment %s", klog.KRef(input.Namespace, input.MDName))

	machineSets := make([]*clusterv1.MachineSet, len(machineSetList.Items))
	for i := range machineSetList.Items {
		machineSets[i] = &machineSetList.Items[i]
	}
	return machineSets
}
