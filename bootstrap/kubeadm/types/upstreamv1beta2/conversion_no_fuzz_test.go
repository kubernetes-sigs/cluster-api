/*
Copyright 2025 The Kubernetes Authors.

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

package upstreamv1beta2

import (
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
)

// This test case has been moved out of conversion_test.go because it should be run with the race detector.
// Note: The tests in conversion_test.go are disabled when the race detector is enabled (via "//go:build !race") because otherwise the fuzz tests would just time out.

func TestTimeoutForControlPlaneMigration(t *testing.T) {
	t.Run("from InitConfiguration to ClusterConfiguration and back", func(t *testing.T) {
		g := NewWithT(t)

		initConfiguration := &bootstrapv1.InitConfiguration{
			Timeouts: &bootstrapv1.Timeouts{
				ControlPlaneComponentHealthCheckSeconds: ptr.To[int32](123),
			},
		}

		clusterConfiguration := &ClusterConfiguration{}
		err := clusterConfiguration.ConvertFromInitConfiguration(initConfiguration)
		g.Expect(err).ToNot(HaveOccurred())

		initConfiguration = &bootstrapv1.InitConfiguration{}
		err = clusterConfiguration.ConvertToInitConfiguration(initConfiguration)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(initConfiguration.Timeouts.ControlPlaneComponentHealthCheckSeconds).To(Equal(ptr.To[int32](123)))
	})
}
