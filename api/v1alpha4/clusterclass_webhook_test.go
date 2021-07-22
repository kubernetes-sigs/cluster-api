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

package v1alpha4

import (
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/cluster-api/feature"
)

func TestClusterClassValidationFeatureGated(t *testing.T) {
	// NOTE: ClusterTopology feature flag is disabled by default, thus preventing to create or update ClusterClasses.

	ref := &LocalObjectReference{
		APIVersion: "foo",
		Kind:       "bar",
		Name:       "baz",
	}
	tests := []struct {
		name      string
		in        *ClusterClass
		old       *ClusterClass
		expectErr bool
	}{
		{
			name: "creation should fail if feature flag is disabled, no matter the ClusterClass is valid(or not)",
			in: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane:   LocalObjectTemplate{Ref: ref},
					Workers: WorkersClass{
						MachineDeployments: []MachineDeploymentClass{
							{
								Class: "aa",
								Template: MachineDeploymentClassTemplate{
									Bootstrap:      LocalObjectTemplate{Ref: ref},
									Infrastructure: LocalObjectTemplate{Ref: ref},
								},
							},
						},
					},
				},
			},
			expectErr: true,
		},
		{
			name: "update should fail if feature flag is disabled, no matter the ClusterClass is valid(or not)",
			old: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane:   LocalObjectTemplate{Ref: ref},
					Workers: WorkersClass{
						MachineDeployments: []MachineDeploymentClass{
							{
								Class: "aa",
								Template: MachineDeploymentClassTemplate{
									Bootstrap:      LocalObjectTemplate{Ref: ref},
									Infrastructure: LocalObjectTemplate{Ref: ref},
								},
							},
						},
					},
				},
			},
			in: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane:   LocalObjectTemplate{Ref: ref},
					Workers: WorkersClass{
						MachineDeployments: []MachineDeploymentClass{
							{
								Class: "aa",
								Template: MachineDeploymentClassTemplate{
									Metadata:       ObjectMeta{},
									Bootstrap:      LocalObjectTemplate{Ref: ref},
									Infrastructure: LocalObjectTemplate{Ref: ref},
								},
							},
						},
					},
				},
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			if tt.expectErr {
				g.Expect(tt.in.validate(tt.old)).NotTo(Succeed())
			} else {
				g.Expect(tt.in.validate(tt.old)).To(Succeed())
			}
		})
	}
}

func TestClusterClassValidation(t *testing.T) {
	// NOTE: ClusterTopology feature flag is disabled by default, thus preventing to create or update ClusterClasses.
	// Enabling the feature flag temporarily for this test.
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)()

	ref := &LocalObjectReference{
		APIVersion: "foo",
		Kind:       "bar",
		Name:       "baz",
	}
	tests := []struct {
		name      string
		in        *ClusterClass
		old       *ClusterClass
		expectErr bool
	}{
		{
			name: "create pass",
			in: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane:   LocalObjectTemplate{Ref: ref},
					Workers: WorkersClass{
						MachineDeployments: []MachineDeploymentClass{
							{
								Class: "aa",
								Template: MachineDeploymentClassTemplate{
									Bootstrap:      LocalObjectTemplate{Ref: ref},
									Infrastructure: LocalObjectTemplate{Ref: ref},
								},
							},
							{
								Class: "bb",
								Template: MachineDeploymentClassTemplate{
									Bootstrap:      LocalObjectTemplate{Ref: ref},
									Infrastructure: LocalObjectTemplate{Ref: ref},
								},
							},
						},
					},
				},
			},
			expectErr: false,
		},
		{
			name: "create fail if duplicated DeploymentClasses",
			in: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane:   LocalObjectTemplate{Ref: ref},
					Workers: WorkersClass{
						MachineDeployments: []MachineDeploymentClass{
							{
								Class: "aa",
								Template: MachineDeploymentClassTemplate{
									Bootstrap:      LocalObjectTemplate{Ref: ref},
									Infrastructure: LocalObjectTemplate{Ref: ref},
								},
							},
							{
								Class: "aa",
								Template: MachineDeploymentClassTemplate{
									Bootstrap:      LocalObjectTemplate{Ref: ref},
									Infrastructure: LocalObjectTemplate{Ref: ref},
								},
							},
						},
					},
				},
			},
			expectErr: true,
		},
		{
			name: "update pass in case of no changes",
			old: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane:   LocalObjectTemplate{Ref: ref},
					Workers: WorkersClass{
						MachineDeployments: []MachineDeploymentClass{
							{
								Class: "aa",
								Template: MachineDeploymentClassTemplate{
									Bootstrap:      LocalObjectTemplate{Ref: ref},
									Infrastructure: LocalObjectTemplate{Ref: ref},
								},
							},
						},
					},
				},
			},
			in: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane:   LocalObjectTemplate{Ref: ref},
					Workers: WorkersClass{
						MachineDeployments: []MachineDeploymentClass{
							{
								Class: "aa",
								Template: MachineDeploymentClassTemplate{
									Metadata:       ObjectMeta{},
									Bootstrap:      LocalObjectTemplate{Ref: ref},
									Infrastructure: LocalObjectTemplate{Ref: ref},
								},
							},
						},
					},
				},
			},
			expectErr: false,
		},
		{
			name: "update fails if infrastructure changes",
			old: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane:   LocalObjectTemplate{Ref: ref},
					Workers: WorkersClass{
						MachineDeployments: []MachineDeploymentClass{
							{
								Class: "aa",
								Template: MachineDeploymentClassTemplate{
									Metadata:       ObjectMeta{},
									Bootstrap:      LocalObjectTemplate{Ref: ref},
									Infrastructure: LocalObjectTemplate{Ref: ref},
								},
							},
						},
					},
				},
			},
			in: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: &LocalObjectReference{
						APIVersion: "foox",
						Kind:       "barx",
						Name:       "bazx",
					}},
					ControlPlane: LocalObjectTemplate{Ref: ref},
					Workers: WorkersClass{
						MachineDeployments: []MachineDeploymentClass{
							{
								Class: "aa",
								Template: MachineDeploymentClassTemplate{
									Metadata:       ObjectMeta{},
									Bootstrap:      LocalObjectTemplate{Ref: ref},
									Infrastructure: LocalObjectTemplate{Ref: ref},
								},
							},
						},
					},
				},
			},
			expectErr: true,
		},
		{
			name: "update fails if controlPlane changes",
			old: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane:   LocalObjectTemplate{Ref: ref},
					Workers: WorkersClass{
						MachineDeployments: []MachineDeploymentClass{
							{
								Class: "aa",
								Template: MachineDeploymentClassTemplate{
									Metadata:       ObjectMeta{},
									Bootstrap:      LocalObjectTemplate{Ref: ref},
									Infrastructure: LocalObjectTemplate{Ref: ref},
								},
							},
						},
					},
				},
			},
			in: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane: LocalObjectTemplate{Ref: &LocalObjectReference{
						APIVersion: "foox",
						Kind:       "barx",
						Name:       "bazx",
					}},
					Workers: WorkersClass{
						MachineDeployments: []MachineDeploymentClass{
							{
								Class: "aa",
								Template: MachineDeploymentClassTemplate{
									Metadata:       ObjectMeta{},
									Bootstrap:      LocalObjectTemplate{Ref: ref},
									Infrastructure: LocalObjectTemplate{Ref: ref},
								},
							},
						},
					},
				},
			},
			expectErr: true,
		},
		{
			name: "update fails a machine deployment changes",
			old: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane:   LocalObjectTemplate{Ref: ref},
					Workers: WorkersClass{
						MachineDeployments: []MachineDeploymentClass{
							{
								Class: "aa",
								Template: MachineDeploymentClassTemplate{
									Metadata:       ObjectMeta{},
									Bootstrap:      LocalObjectTemplate{Ref: ref},
									Infrastructure: LocalObjectTemplate{Ref: ref},
								},
							},
						},
					},
				},
			},
			in: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane:   LocalObjectTemplate{Ref: ref},
					Workers: WorkersClass{
						MachineDeployments: []MachineDeploymentClass{
							{
								Class: "aa",
								Template: MachineDeploymentClassTemplate{
									Metadata: ObjectMeta{},
									Bootstrap: LocalObjectTemplate{Ref: &LocalObjectReference{
										APIVersion: "foox",
										Kind:       "barx",
										Name:       "bazx",
									}},
									Infrastructure: LocalObjectTemplate{Ref: ref},
								},
							},
						},
					},
				},
			},
			expectErr: true,
		},
		{
			name: "update pass if a machine deployment class gets added",
			old: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane:   LocalObjectTemplate{Ref: ref},
					Workers: WorkersClass{
						MachineDeployments: []MachineDeploymentClass{
							{
								Class: "aa",
								Template: MachineDeploymentClassTemplate{
									Bootstrap:      LocalObjectTemplate{Ref: ref},
									Infrastructure: LocalObjectTemplate{Ref: ref},
								},
							},
						},
					},
				},
			},
			in: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane:   LocalObjectTemplate{Ref: ref},
					Workers: WorkersClass{
						MachineDeployments: []MachineDeploymentClass{
							{
								Class: "aa",
								Template: MachineDeploymentClassTemplate{
									Bootstrap:      LocalObjectTemplate{Ref: ref},
									Infrastructure: LocalObjectTemplate{Ref: ref},
								},
							},
							{
								Class: "bb",
								Template: MachineDeploymentClassTemplate{
									Bootstrap:      LocalObjectTemplate{Ref: ref},
									Infrastructure: LocalObjectTemplate{Ref: ref},
								},
							},
						},
					},
				},
			},
			expectErr: false,
		},
		{
			name: "update fails if a duplicated deployment class gets added",
			old: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane:   LocalObjectTemplate{Ref: ref},
					Workers: WorkersClass{
						MachineDeployments: []MachineDeploymentClass{
							{
								Class: "aa",
								Template: MachineDeploymentClassTemplate{
									Bootstrap:      LocalObjectTemplate{Ref: ref},
									Infrastructure: LocalObjectTemplate{Ref: ref},
								},
							},
						},
					},
				},
			},
			in: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane:   LocalObjectTemplate{Ref: ref},
					Workers: WorkersClass{
						MachineDeployments: []MachineDeploymentClass{
							{
								Class: "aa",
								Template: MachineDeploymentClassTemplate{
									Bootstrap:      LocalObjectTemplate{Ref: ref},
									Infrastructure: LocalObjectTemplate{Ref: ref},
								},
							},
							{
								Class: "aa",
								Template: MachineDeploymentClassTemplate{
									Bootstrap:      LocalObjectTemplate{Ref: ref},
									Infrastructure: LocalObjectTemplate{Ref: ref},
								},
							},
						},
					},
				},
			},
			expectErr: true,
		},
		{
			name: "update fails if a machine deployment class gets removed",
			old: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane:   LocalObjectTemplate{Ref: ref},
					Workers: WorkersClass{
						MachineDeployments: []MachineDeploymentClass{
							{
								Class: "aa",
								Template: MachineDeploymentClassTemplate{
									Bootstrap:      LocalObjectTemplate{Ref: ref},
									Infrastructure: LocalObjectTemplate{Ref: ref},
								},
							},
							{
								Class: "bb",
								Template: MachineDeploymentClassTemplate{
									Bootstrap:      LocalObjectTemplate{Ref: ref},
									Infrastructure: LocalObjectTemplate{Ref: ref},
								},
							},
						},
					},
				},
			},
			in: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane:   LocalObjectTemplate{Ref: ref},
					Workers: WorkersClass{
						MachineDeployments: []MachineDeploymentClass{
							{
								Class: "aa",
								Template: MachineDeploymentClassTemplate{
									Bootstrap:      LocalObjectTemplate{Ref: ref},
									Infrastructure: LocalObjectTemplate{Ref: ref},
								},
							},
						},
					},
				},
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			if tt.expectErr {
				g.Expect(tt.in.validate(tt.old)).NotTo(Succeed())
			} else {
				g.Expect(tt.in.validate(tt.old)).To(Succeed())
			}
		})
	}
}
