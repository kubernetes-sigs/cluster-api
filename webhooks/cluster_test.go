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

package webhooks

import (
	"testing"

	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilfeature "k8s.io/component-base/featuregate/testing"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
)

func TestClusterDefaultNamespaces(t *testing.T) {
	g := NewWithT(t)

	in := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "fooboo",
		},
		Spec: clusterv1.ClusterSpec{
			InfrastructureRef: &corev1.ObjectReference{},
			ControlPlaneRef:   &corev1.ObjectReference{},
		},
	}

	webhook := &Cluster{}
	t.Run("for Cluster", customDefaultValidateTest(ctx, in, webhook))

	g.Expect(webhook.Default(ctx, in)).To(Succeed())

	g.Expect(in.Spec.InfrastructureRef.Namespace).To(Equal(in.Namespace))
	g.Expect(in.Spec.ControlPlaneRef.Namespace).To(Equal(in.Namespace))
}

func TestClusterDefaultTopologyVersion(t *testing.T) {
	// NOTE: ClusterTopology feature flag is disabled by default, thus preventing to set Cluster.Topologies.
	// Enabling the feature flag temporarily for this test.
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)()

	g := NewWithT(t)

	in := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "fooboo",
		},
		Spec: clusterv1.ClusterSpec{
			Topology: &clusterv1.Topology{
				Class:   "foo",
				Version: "1.19.1",
			},
		},
	}
	webhook := &Cluster{}
	t.Run("for Cluster", customDefaultValidateTest(ctx, in, webhook))
	g.Expect(webhook.Default(ctx, in)).To(Succeed())

	g.Expect(in.Spec.Topology.Version).To(HavePrefix("v"))
}
