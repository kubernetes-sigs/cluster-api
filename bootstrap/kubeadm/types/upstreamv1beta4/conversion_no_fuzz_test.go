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

package upstreamv1beta4

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
)

// This test case has been moved out of conversion_test.go because it should be run with the race detector.
// Note: The tests in conversion_test.go are disabled when the race detector is enabled (via "//go:build !race") because otherwise the fuzz tests would just time out.

func TestTimeoutForControlPlaneMigration(t *testing.T) {
	timeout := metav1.Duration{Duration: 10 * time.Second}
	t.Run("from ClusterConfiguration to InitConfiguration and back", func(t *testing.T) {
		g := NewWithT(t)

		clusterConfiguration := &bootstrapv1.ClusterConfiguration{
			APIServer: bootstrapv1.APIServer{TimeoutForControlPlane: &timeout},
		}

		initConfiguration := &InitConfiguration{}
		err := initConfiguration.ConvertFromClusterConfiguration(clusterConfiguration)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(initConfiguration.Timeouts.ControlPlaneComponentHealthCheck).To(Equal(&timeout))

		clusterConfiguration = &bootstrapv1.ClusterConfiguration{}
		err = initConfiguration.ConvertToClusterConfiguration(clusterConfiguration)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(clusterConfiguration.APIServer.TimeoutForControlPlane).To(Equal(&timeout))
	})
	t.Run("from ClusterConfiguration to JoinConfiguration and back", func(t *testing.T) {
		g := NewWithT(t)

		clusterConfiguration := &bootstrapv1.ClusterConfiguration{
			APIServer: bootstrapv1.APIServer{TimeoutForControlPlane: &timeout},
		}

		joinConfiguration := &JoinConfiguration{}
		err := joinConfiguration.ConvertFromClusterConfiguration(clusterConfiguration)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(joinConfiguration.Timeouts.ControlPlaneComponentHealthCheck).To(Equal(&timeout))

		clusterConfiguration = &bootstrapv1.ClusterConfiguration{}
		err = joinConfiguration.ConvertToClusterConfiguration(clusterConfiguration)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(clusterConfiguration.APIServer.TimeoutForControlPlane).To(Equal(&timeout))
	})
}
