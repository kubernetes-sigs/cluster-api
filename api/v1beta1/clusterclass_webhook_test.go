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

package v1beta1

import (
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/cluster-api/feature"

	utildefaulting "sigs.k8s.io/cluster-api/util/defaulting"
)

func TestClusterClassDefaultNamespaces(t *testing.T) {
	// NOTE: ClusterTopology feature flag is disabled by default, thus preventing to create or update ClusterClasses.
	// Enabling the feature flag temporarily for this test.
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)()

	namespace := "default"
	ref := &corev1.ObjectReference{
		APIVersion: "foo",
		Kind:       "barTemplate",
		Name:       "baz",
	}
	in := &ClusterClass{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
		},
		Spec: ClusterClassSpec{
			Infrastructure: LocalObjectTemplate{Ref: ref},
			ControlPlane: ControlPlaneClass{
				LocalObjectTemplate:   LocalObjectTemplate{Ref: ref},
				MachineInfrastructure: &LocalObjectTemplate{Ref: ref},
			},
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
	}

	t.Run("for ClusterClass", utildefaulting.DefaultValidateTest(in))
	in.Default()

	// Namespace defaulted on references
	g := NewWithT(t)
	g.Expect(in.Spec.Infrastructure.Ref.Namespace).To(Equal(namespace))
	g.Expect(in.Spec.ControlPlane.Ref.Namespace).To(Equal(namespace))
	g.Expect(in.Spec.ControlPlane.MachineInfrastructure.Ref.Namespace).To(Equal(namespace))
	for i := range in.Spec.Workers.MachineDeployments {
		g.Expect(in.Spec.Workers.MachineDeployments[i].Template.Bootstrap.Ref.Namespace).To(Equal(namespace))
		g.Expect(in.Spec.Workers.MachineDeployments[i].Template.Infrastructure.Ref.Namespace).To(Equal(namespace))
	}
}

func TestClusterClassValidationFeatureGated(t *testing.T) {
	// NOTE: ClusterTopology feature flag is disabled by default, thus preventing to create or update ClusterClasses.

	ref := &corev1.ObjectReference{
		APIVersion: "foo",
		Kind:       "barTemplate",
		Name:       "baz",
		Namespace:  "default",
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
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate: LocalObjectTemplate{Ref: ref},
					},
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
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate: LocalObjectTemplate{Ref: ref},
					},
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
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate: LocalObjectTemplate{Ref: ref},
					},
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

	ref := &corev1.ObjectReference{
		APIVersion: "group.test.io/foo",
		Kind:       "barTemplate",
		Name:       "baz",
		Namespace:  "default",
	}
	refInAnotherNamespace := &corev1.ObjectReference{
		APIVersion: "group.test.io/foo",
		Kind:       "barTemplate",
		Name:       "baz",
		Namespace:  "another-namespace",
	}
	refBadTemplate := &corev1.ObjectReference{
		APIVersion: "group.test.io/foo",
		Kind:       "bar",
		Name:       "baz",
		Namespace:  "default",
	}
	refBadAPIVersion := &corev1.ObjectReference{
		APIVersion: "group/test.io/v1/foo",
		Kind:       "barTemplate",
		Name:       "baz",
		Namespace:  "default",
	}
	refEmptyName := &corev1.ObjectReference{
		APIVersion: "group.test.io/foo",
		Namespace:  "default",
		Kind:       "barTemplate",
	}
	incompatibleRef := &corev1.ObjectReference{
		APIVersion: "group.test.io/foo",
		Kind:       "another-barTemplate",
		Name:       "baz",
		Namespace:  "default",
	}
	compatibleRef := &corev1.ObjectReference{
		APIVersion: "group.test.io/another-foo",
		Kind:       "barTemplate",
		Name:       "another-baz",
		Namespace:  "default",
	}

	tests := []struct {
		name      string
		in        *ClusterClass
		old       *ClusterClass
		expectErr bool
	}{

		/*
			CREATE Tests
		*/

		{
			name: "create pass",
			in: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate: LocalObjectTemplate{Ref: ref},
					},
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

		// empty name in ref tests
		{
			name: "create fail infrastructure has empty name",
			in: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: refEmptyName},
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate: LocalObjectTemplate{Ref: ref},
					},
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
			name: "create fail control plane class has empty name",
			in: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate: LocalObjectTemplate{Ref: refEmptyName},
					},
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
			name: "create fail control plane class machineinfrastructure has empty name",
			in: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate:   LocalObjectTemplate{Ref: ref},
						MachineInfrastructure: &LocalObjectTemplate{Ref: refEmptyName},
					},
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
			name: "create fail machine deployment bootstrap has empty name",
			in: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate: LocalObjectTemplate{Ref: ref},
					},
					Workers: WorkersClass{
						MachineDeployments: []MachineDeploymentClass{
							{
								Class: "aa",
								Template: MachineDeploymentClassTemplate{
									Bootstrap:      LocalObjectTemplate{Ref: refEmptyName},
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
			name: "create fail machine deployment infrastructure has empty name",
			in: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate: LocalObjectTemplate{Ref: ref},
					},
					Workers: WorkersClass{
						MachineDeployments: []MachineDeploymentClass{
							{
								Class: "aa",
								Template: MachineDeploymentClassTemplate{
									Bootstrap:      LocalObjectTemplate{Ref: ref},
									Infrastructure: LocalObjectTemplate{Ref: refEmptyName},
								},
							},
						},
					},
				},
			},
			expectErr: true,
		},

		// inconsistent namespace in ref tests
		{
			name: "create fail if infrastructure has inconsistent namespace",
			in: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: refInAnotherNamespace},
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate: LocalObjectTemplate{Ref: ref},
					},
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
			name: "create fail if control plane has inconsistent namespace",
			in: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate: LocalObjectTemplate{Ref: refInAnotherNamespace},
					},
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
			name: "create fail if control plane class machineinfrastructure has inconsistent namespace",
			in: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate:   LocalObjectTemplate{Ref: ref},
						MachineInfrastructure: &LocalObjectTemplate{Ref: refInAnotherNamespace},
					},
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
			name: "create fail if machine deployment / bootstrap has inconsistent namespace",
			in: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate: LocalObjectTemplate{Ref: ref},
					},
					Workers: WorkersClass{
						MachineDeployments: []MachineDeploymentClass{
							{
								Class: "aa",
								Template: MachineDeploymentClassTemplate{
									Bootstrap:      LocalObjectTemplate{Ref: refInAnotherNamespace},
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
			name: "create fail if machine deployment / infrastructure has inconsistent namespace",
			in: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate: LocalObjectTemplate{Ref: ref},
					},
					Workers: WorkersClass{
						MachineDeployments: []MachineDeploymentClass{
							{
								Class: "aa",
								Template: MachineDeploymentClassTemplate{
									Bootstrap:      LocalObjectTemplate{Ref: ref},
									Infrastructure: LocalObjectTemplate{Ref: refInAnotherNamespace},
								},
							},
						},
					},
				},
			},
			expectErr: true,
		},

		// bad template in ref tests
		{
			name: "create fail if bad template in control plane",
			in: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: refBadTemplate},
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate: LocalObjectTemplate{Ref: ref},
					},
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
			old:       nil,
			expectErr: true,
		},
		{
			name: "create fail if bad template in control plane class machineinfrastructure",
			in: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate:   LocalObjectTemplate{Ref: ref},
						MachineInfrastructure: &LocalObjectTemplate{Ref: refBadTemplate},
					},
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
			old:       nil,
			expectErr: true,
		},
		{
			name: "create fail if bad template in infrastructure",
			in: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate: LocalObjectTemplate{Ref: refBadTemplate},
					},
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
			old:       nil,
			expectErr: true,
		},
		{
			name: "create fail if bad template in machine deployment bootstrap",
			in: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate: LocalObjectTemplate{Ref: ref},
					},
					Workers: WorkersClass{
						MachineDeployments: []MachineDeploymentClass{
							{
								Class: "aa",
								Template: MachineDeploymentClassTemplate{
									Bootstrap:      LocalObjectTemplate{Ref: refBadTemplate},
									Infrastructure: LocalObjectTemplate{Ref: ref},
								},
							},
						},
					},
				},
			},
			old:       nil,
			expectErr: true,
		},
		{
			name: "create fail if bad template in machine deployment infrastructure",
			in: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate: LocalObjectTemplate{Ref: ref},
					},
					Workers: WorkersClass{
						MachineDeployments: []MachineDeploymentClass{
							{
								Class: "aa",
								Template: MachineDeploymentClassTemplate{
									Bootstrap:      LocalObjectTemplate{Ref: ref},
									Infrastructure: LocalObjectTemplate{Ref: refBadTemplate},
								},
							},
						},
					},
				},
			},
			old:       nil,
			expectErr: true,
		},

		// bad apiVersion in ref tests
		{
			name: "create fail if bad apiVersion in control plane",
			in: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate: LocalObjectTemplate{Ref: refBadAPIVersion},
					},
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
			old:       nil,
			expectErr: true,
		},
		{
			name: "create fail if bad apiVersion in control plane class machineinfrastructure",
			in: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate:   LocalObjectTemplate{Ref: ref},
						MachineInfrastructure: &LocalObjectTemplate{Ref: refBadAPIVersion},
					},
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
			old:       nil,
			expectErr: true,
		},
		{
			name: "create fail if bad apiVersion in infrastructure",
			in: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: refBadAPIVersion},
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate: LocalObjectTemplate{Ref: ref},
					},
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
			old:       nil,
			expectErr: true,
		},
		{
			name: "create fail if bad apiVersion in machine deployment bootstrap",
			in: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate: LocalObjectTemplate{Ref: ref},
					},
					Workers: WorkersClass{
						MachineDeployments: []MachineDeploymentClass{
							{
								Class: "aa",
								Template: MachineDeploymentClassTemplate{
									Bootstrap:      LocalObjectTemplate{Ref: refBadAPIVersion},
									Infrastructure: LocalObjectTemplate{Ref: ref},
								},
							},
						},
					},
				},
			},
			old:       nil,
			expectErr: true,
		},
		{
			name: "create fail if bad apiVersion in machine deployment infrastructure",
			in: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate: LocalObjectTemplate{Ref: ref},
					},
					Workers: WorkersClass{
						MachineDeployments: []MachineDeploymentClass{
							{
								Class: "aa",
								Template: MachineDeploymentClassTemplate{
									Bootstrap:      LocalObjectTemplate{Ref: ref},
									Infrastructure: LocalObjectTemplate{Ref: refBadAPIVersion},
								},
							},
						},
					},
				},
			},
			old:       nil,
			expectErr: true,
		},

		// create test
		{
			name: "create fail if duplicated DeploymentClasses",
			in: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate: LocalObjectTemplate{Ref: ref},
					},
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

		/*
			UPDATE Tests
		*/

		{
			name: "update pass in case of no changes",
			old: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate: LocalObjectTemplate{Ref: ref},
					},
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
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate: LocalObjectTemplate{Ref: ref},
					},
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
			name: "update pass if infrastructure changes in a compatible way",
			old: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate: LocalObjectTemplate{Ref: ref},
					},
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
					Infrastructure: LocalObjectTemplate{Ref: compatibleRef},
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate: LocalObjectTemplate{Ref: ref},
					},
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
			name: "update fails if infrastructure changes in an incompatible way",
			old: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate: LocalObjectTemplate{Ref: ref},
					},
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
					Infrastructure: LocalObjectTemplate{Ref: incompatibleRef},
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate: LocalObjectTemplate{Ref: ref},
					},
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
			name: "update pass if controlPlane changes in a compatible way",
			old: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate:   LocalObjectTemplate{Ref: ref},
						MachineInfrastructure: &LocalObjectTemplate{Ref: ref},
					},
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
					ControlPlane: ControlPlaneClass{
						Metadata: ObjectMeta{
							Labels:      map[string]string{"foo": "bar"},
							Annotations: map[string]string{"foo": "bar"},
						},
						LocalObjectTemplate:   LocalObjectTemplate{Ref: compatibleRef},
						MachineInfrastructure: &LocalObjectTemplate{Ref: compatibleRef},
					},
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
			name: "update fails if controlPlane changes in an incompatible way (control plane template)",
			old: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate: LocalObjectTemplate{Ref: ref},
					},
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
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate: LocalObjectTemplate{Ref: incompatibleRef},
					},
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
			name: "update fails if controlPlane changes in an incompatible way (control plane infrastructure machine template)",
			old: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate:   LocalObjectTemplate{Ref: ref},
						MachineInfrastructure: &LocalObjectTemplate{Ref: ref},
					},
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
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate:   LocalObjectTemplate{Ref: ref},
						MachineInfrastructure: &LocalObjectTemplate{Ref: incompatibleRef},
					},
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
			name: "update pass if a machine deployment changes in a compatible way",
			old: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate: LocalObjectTemplate{Ref: ref},
					},
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
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate: LocalObjectTemplate{Ref: ref},
					},
					Workers: WorkersClass{
						MachineDeployments: []MachineDeploymentClass{
							{
								Class: "aa",
								Template: MachineDeploymentClassTemplate{
									Metadata: ObjectMeta{
										Labels:      map[string]string{"foo": "bar"},
										Annotations: map[string]string{"foo": "bar"},
									},
									Bootstrap:      LocalObjectTemplate{Ref: incompatibleRef}, // NOTE: this should be tolerated
									Infrastructure: LocalObjectTemplate{Ref: compatibleRef},
								},
							},
						},
					},
				},
			},
			expectErr: false,
		},
		{
			name: "update fails a machine deployment changes in an incompatible way",
			old: &ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: ClusterClassSpec{
					Infrastructure: LocalObjectTemplate{Ref: ref},
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate: LocalObjectTemplate{Ref: ref},
					},
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
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate: LocalObjectTemplate{Ref: ref},
					},
					Workers: WorkersClass{
						MachineDeployments: []MachineDeploymentClass{
							{
								Class: "aa",
								Template: MachineDeploymentClassTemplate{
									Metadata:       ObjectMeta{},
									Bootstrap:      LocalObjectTemplate{Ref: ref},
									Infrastructure: LocalObjectTemplate{Ref: incompatibleRef},
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
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate: LocalObjectTemplate{Ref: ref},
					},
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
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate: LocalObjectTemplate{Ref: ref},
					},
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
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate: LocalObjectTemplate{Ref: ref},
					},
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
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate: LocalObjectTemplate{Ref: ref},
					},
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
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate: LocalObjectTemplate{Ref: ref},
					},
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
					ControlPlane: ControlPlaneClass{
						LocalObjectTemplate: LocalObjectTemplate{Ref: ref},
					},
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
