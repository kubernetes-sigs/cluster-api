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

package scope

import (
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

func TestNew(t *testing.T) {
	t.Run("should set the right maxUpgradeConcurrency in UpgradeTracker from the cluster annotation", func(t *testing.T) {
		tests := []struct {
			name    string
			cluster *clusterv1.Cluster
			want    int
		}{
			{
				name:    "if the annotation is not present the concurrency should default to 1",
				cluster: &clusterv1.Cluster{},
				want:    1,
			},
			{
				name: "if the cluster has the annotation it should set the upgrade concurrency value",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							clusterv1.ClusterTopologyUpgradeConcurrencyAnnotation: "2",
						},
					},
				},
				want: 2,
			},
		}

		for _, tt := range tests {
			g := NewWithT(t)
			s := New(tt.cluster)
			g.Expect(s.UpgradeTracker.MachineDeployments.maxUpgradeConcurrency).To(Equal(tt.want))
			g.Expect(s.UpgradeTracker.MachinePools.maxUpgradeConcurrency).To(Equal(tt.want))
		}
	})
}
