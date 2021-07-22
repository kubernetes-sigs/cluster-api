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

package v1alpha4

import (
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/cluster-api/feature"
	utildefaulting "sigs.k8s.io/cluster-api/util/defaulting"
)

func TestClusterDefaultTopologyVersion(t *testing.T) {
	// NOTE: ClusterTopology feature flag is disabled by default, thus preventing to set Cluster.Topologies.
	// Enabling the feature flag temporarily for this test.
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)()

	g := NewWithT(t)

	c := &Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "fooboo",
		},
		Spec: ClusterSpec{
			Topology: &Topology{
				Class:   "foo",
				Version: "1.19.1",
			},
		},
	}

	t.Run("for Cluster", utildefaulting.DefaultValidateTest(c))
	c.Default()

	g.Expect(c.Spec.Topology.Version).To(HavePrefix("v"))
}

func TestClusterValidation(t *testing.T) {
	// NOTE: ClusterTopology feature flag is disabled by default, thus preventing to set Cluster.Topologies.

	tests := []struct {
		name      string
		in        *Cluster
		old       *Cluster
		expectErr bool
	}{
		{
			name:      "should succeed when namespaces match",
			expectErr: false,
			in: &Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
				},
				Spec: ClusterSpec{
					ControlPlaneRef:   &LocalObjectReference{},
					InfrastructureRef: &LocalObjectReference{},
				},
			},
		},
		{
			name:      "fails if topology is set but feature flag is disabled",
			expectErr: true,
			in: &Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
				},
				Spec: ClusterSpec{
					ControlPlaneRef:   &LocalObjectReference{},
					InfrastructureRef: &LocalObjectReference{},
					Topology:          &Topology{},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			err := tt.in.validate(tt.old)
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
		in        *Cluster
		old       *Cluster
		expectErr bool
	}{
		{
			name:      "should return error when topology does not have class",
			expectErr: true,
			in: &Cluster{
				Spec: ClusterSpec{
					Topology: &Topology{},
				},
			},
		},
		{
			name:      "should return error when topology does not have valid version",
			expectErr: true,
			in: &Cluster{
				Spec: ClusterSpec{
					Topology: &Topology{
						Class:   "foo",
						Version: "invalid",
					},
				},
			},
		},
		{
			name:      "should return error when duplicated MachineDeployments names exists in a Topology",
			expectErr: true,
			in: &Cluster{
				Spec: ClusterSpec{
					Topology: &Topology{
						Class:   "foo",
						Version: "v1.19.1",
						Workers: &WorkersTopology{
							MachineDeployments: []MachineDeploymentTopology{
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
			in: &Cluster{
				Spec: ClusterSpec{
					Topology: &Topology{
						Class:   "foo",
						Version: "v1.19.1",
						Workers: &WorkersTopology{
							MachineDeployments: []MachineDeploymentTopology{
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
			name:      "should return error on create when both Topology and control plane ref are defined",
			expectErr: true,
			in: &Cluster{
				Spec: ClusterSpec{
					ControlPlaneRef: &LocalObjectReference{},
					Topology: &Topology{
						Class:   "foo",
						Version: "v1.19.1",
					},
				},
			},
		},
		{
			name:      "should return error on create when both Topology and infrastructure ref are defined",
			expectErr: true,
			in: &Cluster{
				Spec: ClusterSpec{
					InfrastructureRef: &LocalObjectReference{},
					Topology: &Topology{
						Class:   "foo",
						Version: "v1.19.1",
					},
				},
			},
		},
		{
			name:      "should return error on update when Topology class is changed",
			expectErr: true,
			old: &Cluster{
				Spec: ClusterSpec{
					InfrastructureRef: &LocalObjectReference{},
					Topology: &Topology{
						Class:   "foo",
						Version: "v1.19.1",
					},
				},
			},
			in: &Cluster{
				Spec: ClusterSpec{
					InfrastructureRef: &LocalObjectReference{},
					Topology: &Topology{
						Class:   "bar",
						Version: "v1.19.1",
					},
				},
			},
		},
		{
			name:      "should return error on update when Topology version is downgraded",
			expectErr: true,
			old: &Cluster{
				Spec: ClusterSpec{
					InfrastructureRef: &LocalObjectReference{},
					Topology: &Topology{
						Class:   "foo",
						Version: "v1.19.1",
					},
				},
			},
			in: &Cluster{
				Spec: ClusterSpec{
					InfrastructureRef: &LocalObjectReference{},
					Topology: &Topology{
						Class:   "foo",
						Version: "v1.19.0",
					},
				},
			},
		},
		{
			name:      "should update",
			expectErr: false,
			old: &Cluster{
				Spec: ClusterSpec{
					InfrastructureRef: &LocalObjectReference{},
					Topology: &Topology{
						Class:   "foo",
						Version: "v1.19.1",
						Workers: &WorkersTopology{
							MachineDeployments: []MachineDeploymentTopology{
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
			in: &Cluster{
				Spec: ClusterSpec{
					InfrastructureRef: &LocalObjectReference{},
					Topology: &Topology{
						Class:   "foo",
						Version: "v1.19.2",
						Workers: &WorkersTopology{
							MachineDeployments: []MachineDeploymentTopology{
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

			err := tt.in.validate(tt.old)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}
