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

	c := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "fooboo",
		},
		Spec: clusterv1.ClusterSpec{
			InfrastructureRef: &corev1.ObjectReference{},
			ControlPlaneRef:   &corev1.ObjectReference{},
		},
	}
	webhook := &Cluster{}
	t.Run("for Cluster", customDefaultValidateTest(ctx, c, webhook))
	g.Expect(webhook.Default(ctx, c)).To(Succeed())

	g.Expect(c.Spec.InfrastructureRef.Namespace).To(Equal(c.Namespace))
	g.Expect(c.Spec.ControlPlaneRef.Namespace).To(Equal(c.Namespace))
}

func TestClusterDefaultTopologyVersion(t *testing.T) {
	// NOTE: ClusterTopology feature flag is disabled by default, thus preventing to set Cluster.Topologies.
	// Enabling the feature flag temporarily for this test.
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)()

	g := NewWithT(t)

	c := &clusterv1.Cluster{
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
	t.Run("for Cluster", customDefaultValidateTest(ctx, c, webhook))
	g.Expect(webhook.Default(ctx, c)).To(Succeed())

	g.Expect(c.Spec.Topology.Version).To(HavePrefix("v"))
}

func TestClusterValidation(t *testing.T) {
	// NOTE: ClusterTopology feature flag is disabled by default, thus preventing to set Cluster.Topologies.

	tests := []struct {
		name      string
		in        *clusterv1.Cluster
		old       *clusterv1.Cluster
		expectErr bool
	}{
		{
			name:      "should return error when cluster namespace and infrastructure ref namespace mismatch",
			expectErr: true,
			in: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
				},
				Spec: clusterv1.ClusterSpec{
					InfrastructureRef: &corev1.ObjectReference{
						Namespace: "bar",
					},
					ControlPlaneRef: &corev1.ObjectReference{
						Namespace: "foo",
					},
				},
			},
		},
		{
			name:      "should return error when cluster namespace and controlplane ref namespace mismatch",
			expectErr: true,
			in: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
				},
				Spec: clusterv1.ClusterSpec{
					InfrastructureRef: &corev1.ObjectReference{
						Namespace: "foo",
					},
					ControlPlaneRef: &corev1.ObjectReference{
						Namespace: "bar",
					},
				},
			},
		},
		{
			name:      "should succeed when namespaces match",
			expectErr: false,
			in: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
				},
				Spec: clusterv1.ClusterSpec{
					ControlPlaneRef: &corev1.ObjectReference{
						Namespace: "foo",
					},
					InfrastructureRef: &corev1.ObjectReference{
						Namespace: "foo",
					},
				},
			},
		},
		{
			name:      "fails if topology is set but feature flag is disabled",
			expectErr: true,
			in: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
				},
				Spec: clusterv1.ClusterSpec{
					ControlPlaneRef: &corev1.ObjectReference{
						Namespace: "foo",
					},
					InfrastructureRef: &corev1.ObjectReference{
						Namespace: "foo",
					},
					Topology: &clusterv1.Topology{},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			webhook := Cluster{}
			err := webhook.validate(tt.old, tt.in)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func TestClusterTopologyValidation(t *testing.T) {
	// NOTE: ClusterTopology feature flag is disabled by default, thus preventing to set Cluster.Topologies.
	// Enabling the feature flag temporarily for this test.
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)()

	tests := []struct {
		name      string
		in        *clusterv1.Cluster
		old       *clusterv1.Cluster
		expectErr bool
	}{
		{
			name:      "should return error when topology does not have class",
			expectErr: true,
			in: &clusterv1.Cluster{
				Spec: clusterv1.ClusterSpec{
					Topology: &clusterv1.Topology{},
				},
			},
		},
		{
			name:      "should return error when topology does not have valid version",
			expectErr: true,
			in: &clusterv1.Cluster{
				Spec: clusterv1.ClusterSpec{
					Topology: &clusterv1.Topology{
						Class:   "foo",
						Version: "invalid",
					},
				},
			},
		},
		{
			name:      "should return error when downgrading topology version - major",
			expectErr: true,
			old: &clusterv1.Cluster{
				Spec: clusterv1.ClusterSpec{
					Topology: &clusterv1.Topology{
						Class:   "foo",
						Version: "v2.2.3",
					},
				},
			},
			in: &clusterv1.Cluster{
				Spec: clusterv1.ClusterSpec{
					Topology: &clusterv1.Topology{
						Class:   "foo",
						Version: "v1.2.3",
					},
				},
			},
		},
		{
			name:      "should return error when downgrading topology version - minor",
			expectErr: true,
			old: &clusterv1.Cluster{
				Spec: clusterv1.ClusterSpec{
					Topology: &clusterv1.Topology{
						Class:   "foo",
						Version: "v1.2.3",
					},
				},
			},
			in: &clusterv1.Cluster{
				Spec: clusterv1.ClusterSpec{
					Topology: &clusterv1.Topology{
						Class:   "foo",
						Version: "v1.1.3",
					},
				},
			},
		},
		{
			name:      "should return error when downgrading topology version - patch",
			expectErr: true,
			old: &clusterv1.Cluster{
				Spec: clusterv1.ClusterSpec{
					Topology: &clusterv1.Topology{
						Class:   "foo",
						Version: "v1.2.3",
					},
				},
			},
			in: &clusterv1.Cluster{
				Spec: clusterv1.ClusterSpec{
					Topology: &clusterv1.Topology{
						Class:   "foo",
						Version: "v1.2.2",
					},
				},
			},
		},
		{
			name:      "should return error when downgrading topology version - pre-release",
			expectErr: true,
			old: &clusterv1.Cluster{
				Spec: clusterv1.ClusterSpec{
					Topology: &clusterv1.Topology{
						Class:   "foo",
						Version: "v1.2.3-xyz.2",
					},
				},
			},
			in: &clusterv1.Cluster{
				Spec: clusterv1.ClusterSpec{
					Topology: &clusterv1.Topology{
						Class:   "foo",
						Version: "v1.2.3-xyz.1",
					},
				},
			},
		},
		{
			name:      "should return error when downgrading topology version - build tag",
			expectErr: true,
			old: &clusterv1.Cluster{
				Spec: clusterv1.ClusterSpec{
					Topology: &clusterv1.Topology{
						Class:   "foo",
						Version: "v1.2.3+xyz.2",
					},
				},
			},
			in: &clusterv1.Cluster{
				Spec: clusterv1.ClusterSpec{
					Topology: &clusterv1.Topology{
						Class:   "foo",
						Version: "v1.2.3+xyz.1",
					},
				},
			},
		},
		{
			name:      "should return error when duplicated MachineDeployments names exists in a Topology",
			expectErr: true,
			in: &clusterv1.Cluster{
				Spec: clusterv1.ClusterSpec{
					Topology: &clusterv1.Topology{
						Class:   "foo",
						Version: "v1.19.1",
						Workers: &clusterv1.WorkersTopology{
							MachineDeployments: []clusterv1.MachineDeploymentTopology{
								{
									Name: "aa",
								},
								{
									Name: "aa",
								},
							},
						},
					},
				},
			},
		},
		{
			name:      "should pass when MachineDeployments names in a Topology are unique",
			expectErr: false,
			in: &clusterv1.Cluster{
				Spec: clusterv1.ClusterSpec{
					Topology: &clusterv1.Topology{
						Class:   "foo",
						Version: "v1.19.1",
						Workers: &clusterv1.WorkersTopology{
							MachineDeployments: []clusterv1.MachineDeploymentTopology{
								{
									Name: "aa",
								},
								{
									Name: "bb",
								},
							},
						},
					},
				},
			},
		},
		{
			name:      "should return error on update when Topology class is changed",
			expectErr: true,
			old: &clusterv1.Cluster{
				Spec: clusterv1.ClusterSpec{
					InfrastructureRef: &corev1.ObjectReference{},
					Topology: &clusterv1.Topology{
						Class:   "foo",
						Version: "v1.19.1",
					},
				},
			},
			in: &clusterv1.Cluster{
				Spec: clusterv1.ClusterSpec{
					InfrastructureRef: &corev1.ObjectReference{},
					Topology: &clusterv1.Topology{
						Class:   "bar",
						Version: "v1.19.1",
					},
				},
			},
		},
		{
			name:      "should return error on update when Topology version is downgraded",
			expectErr: true,
			old: &clusterv1.Cluster{
				Spec: clusterv1.ClusterSpec{
					InfrastructureRef: &corev1.ObjectReference{},
					Topology: &clusterv1.Topology{
						Class:   "foo",
						Version: "v1.19.1",
					},
				},
			},
			in: &clusterv1.Cluster{
				Spec: clusterv1.ClusterSpec{
					InfrastructureRef: &corev1.ObjectReference{},
					Topology: &clusterv1.Topology{
						Class:   "foo",
						Version: "v1.19.0",
					},
				},
			},
		},
		{
			name:      "should update",
			expectErr: false,
			old: &clusterv1.Cluster{
				Spec: clusterv1.ClusterSpec{
					InfrastructureRef: &corev1.ObjectReference{},
					Topology: &clusterv1.Topology{
						Class:   "foo",
						Version: "v1.19.1",
						Workers: &clusterv1.WorkersTopology{
							MachineDeployments: []clusterv1.MachineDeploymentTopology{
								{
									Name: "aa",
								},
								{
									Name: "bb",
								},
							},
						},
					},
				},
			},
			in: &clusterv1.Cluster{
				Spec: clusterv1.ClusterSpec{
					InfrastructureRef: &corev1.ObjectReference{},
					Topology: &clusterv1.Topology{
						Class:   "foo",
						Version: "v1.19.2",
						Workers: &clusterv1.WorkersTopology{
							MachineDeployments: []clusterv1.MachineDeploymentTopology{
								{
									Name: "aa",
								},
								{
									Name: "bb",
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			webhook := Cluster{}
			err := webhook.validate(tt.old, tt.in)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}
