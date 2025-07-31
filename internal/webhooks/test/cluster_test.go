/*
Copyright 2024 The Kubernetes Authors.

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

package test

import (
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func Test_ValidateCluster(t *testing.T) {
	tests := []struct {
		name           string
		mdTopologyName string
		mpTopologyName string
		cluster        *clusterv1.Cluster
		wantErrs       []string
	}{
		{
			name:           "fail if names are not valid Kubernetes resource names",
			mdTopologyName: "under_score",
			mpTopologyName: "under_score",
			wantErrs: []string{
				"spec.topology.workers.machineDeployments[0].name: Invalid value: \"under_score\": spec.topology.workers.machineDeployments[0].name in body should match",
				"spec.topology.workers.machinePools[0].name: Invalid value: \"under_score\": spec.topology.workers.machinePools[0].name in body should match",
			},
		},
		{
			name:           "fail if names are not valid Kubernetes label values",
			mdTopologyName: "forward/slash",
			mpTopologyName: "forward/slash",
			wantErrs: []string{
				"spec.topology.workers.machineDeployments[0].name: Invalid value: \"forward/slash\": spec.topology.workers.machineDeployments[0].name in body should match",
				"spec.topology.workers.machinePools[0].name: Invalid value: \"forward/slash\": spec.topology.workers.machinePools[0].name in body should match",
			},
		},
		{
			name:           "fail if names are longer than 63 characters (the maximum length of a Kubernetes label value)",
			mdTopologyName: "machine-deployment-topology-name-that-has-longerthan63characterlooooooooooooooooooooooongname",
			mpTopologyName: "machine-pool-topology-name-that-has-longerthan63characterlooooooooooooooooooooooongname",
			wantErrs: []string{
				"spec.topology.workers.machineDeployments[0].name: Too long: may not be more than 63 bytes",
				"spec.topology.workers.machinePools[0].name: Too long: may not be more than 63 bytes",
			},
		},
		{
			name:           "succeed with valid name",
			mdTopologyName: "valid-machine-deployment-topology-name",
			mpTopologyName: "valid-machine-pool-topology-name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			cluster := builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(
					builder.ClusterTopology().
						WithClass("class1").
						WithVersion("v1.22.2").
						WithMachineDeployment(
							builder.MachineDeploymentTopology(tt.mdTopologyName).
								WithClass("aa").
								Build()).
						WithMachinePool(
							builder.MachinePoolTopology(tt.mpTopologyName).
								WithClass("aa").
								Build()).
						Build()).
				Build()

			err := env.CreateAndWait(ctx, cluster)

			if len(tt.wantErrs) > 0 {
				g.Expect(err).To(HaveOccurred())
				for _, wantErr := range tt.wantErrs {
					g.Expect(err.Error()).To(ContainSubstring(wantErr))
				}
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(env.CleanupAndWait(ctx, cluster)).To(Succeed())
			}
		})
	}
}
