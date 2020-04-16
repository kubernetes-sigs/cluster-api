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

package framework

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
)

// WaitForMachinesToBeUpgradedInput is the input for WaitForMachinesToBeUpgraded.
type WaitForMachinesToBeUpgradedInput struct {
	Lister                   Lister
	Cluster                  *clusterv1.Cluster
	KubernetesUpgradeVersion string
	MachineCount             int
}

// WaitForMachinesToBeUpgraded waits until all machines are upgraded to the correct kubernetes version.
func WaitForMachinesToBeUpgraded(ctx context.Context, input WaitForMachinesToBeUpgradedInput, intervals ...interface{}) {
	By("ensuring all machines have upgraded kubernetes version")
	Eventually(func() (int, error) {
		machines := GetControlPlaneMachinesByCluster(context.TODO(), GetMachinesByClusterInput{
			Lister:      input.Lister,
			ClusterName: input.Cluster.Name,
			Namespace:   input.Cluster.Namespace,
		})

		upgraded := 0
		for _, machine := range machines {
			if *machine.Spec.Version == input.KubernetesUpgradeVersion {
				upgraded++
			}
		}
		if len(machines) > upgraded {
			return 0, errors.New("old nodes remain")
		}
		return upgraded, nil
	}, intervals...).Should(Equal(input.MachineCount))
}
