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
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/component-base/featuregate/testing"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	ctx        = ctrl.SetupSignalHandler()
	fakeScheme = runtime.NewScheme()
)

func init() {
	_ = clusterv1.AddToScheme(fakeScheme)
}

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
	in := &clusterv1.ClusterClass{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
		},
		Spec: clusterv1.ClusterClassSpec{
			Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
			ControlPlane: clusterv1.ControlPlaneClass{
				LocalObjectTemplate:   clusterv1.LocalObjectTemplate{Ref: ref},
				MachineInfrastructure: &clusterv1.LocalObjectTemplate{Ref: ref},
			},
			Workers: clusterv1.WorkersClass{
				MachineDeployments: []clusterv1.MachineDeploymentClass{
					{
						Class: "aa",
						Template: clusterv1.MachineDeploymentClassTemplate{
							Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
							Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
						},
					},
				},
			},
		},
	}

	webhook := &ClusterClass{}
	t.Run("for ClusterClass", customDefaultValidateTest(ctx, in, webhook))

	g := NewWithT(t)
	g.Expect(webhook.Default(ctx, in)).To(Succeed())

	// Namespace defaulted on references
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
		in        *clusterv1.ClusterClass
		old       *clusterv1.ClusterClass
		expectErr bool
	}{
		{
			name: "creation should fail if feature flag is disabled, no matter the ClusterClass is valid(or not)",
			in: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{Ref: ref},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
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
			old: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{Ref: ref},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
								},
							},
						},
					},
				},
			},
			in: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{Ref: ref},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Metadata:       clusterv1.ObjectMeta{},
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
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
			webhook := &ClusterClass{}
			if tt.expectErr {
				g.Expect(webhook.validate(tt.old, tt.in)).NotTo(Succeed())
			} else {
				g.Expect(webhook.validate(tt.old, tt.in)).To(Succeed())
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
		in        *clusterv1.ClusterClass
		old       *clusterv1.ClusterClass
		expectErr bool
	}{

		/*
			CREATE Tests
		*/

		{
			name: "create pass",
			in: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{Ref: ref},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
								},
							},
							{
								Class: "bb",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
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
			in: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: refEmptyName},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{Ref: ref},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
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
			in: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{Ref: refEmptyName},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
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
			in: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate:   clusterv1.LocalObjectTemplate{Ref: ref},
						MachineInfrastructure: &clusterv1.LocalObjectTemplate{Ref: refEmptyName},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
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
			in: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{Ref: ref},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: refEmptyName},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
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
			in: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{Ref: ref},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: refEmptyName},
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
			in: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: refInAnotherNamespace},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{Ref: ref},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
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
			in: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{Ref: refInAnotherNamespace},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
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
			in: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate:   clusterv1.LocalObjectTemplate{Ref: ref},
						MachineInfrastructure: &clusterv1.LocalObjectTemplate{Ref: refInAnotherNamespace},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
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
			in: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{Ref: ref},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: refInAnotherNamespace},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
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
			in: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{Ref: ref},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: refInAnotherNamespace},
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
			in: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: refBadTemplate},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{Ref: ref},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
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
			in: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate:   clusterv1.LocalObjectTemplate{Ref: ref},
						MachineInfrastructure: &clusterv1.LocalObjectTemplate{Ref: refBadTemplate},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
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
			in: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{Ref: refBadTemplate},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
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
			in: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{Ref: ref},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: refBadTemplate},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
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
			in: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{Ref: ref},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: refBadTemplate},
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
			in: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{Ref: refBadAPIVersion},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
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
			in: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate:   clusterv1.LocalObjectTemplate{Ref: ref},
						MachineInfrastructure: &clusterv1.LocalObjectTemplate{Ref: refBadAPIVersion},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
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
			in: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: refBadAPIVersion},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{Ref: ref},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
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
			in: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{Ref: ref},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: refBadAPIVersion},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
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
			in: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{Ref: ref},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: refBadAPIVersion},
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
			in: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{Ref: ref},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
								},
							},
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
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
			old: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{Ref: ref},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
								},
							},
						},
					},
				},
			},
			in: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{Ref: ref},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Metadata:       clusterv1.ObjectMeta{},
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
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
			old: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{Ref: ref},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Metadata:       clusterv1.ObjectMeta{},
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
								},
							},
						},
					},
				},
			},
			in: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: compatibleRef},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{Ref: ref},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Metadata:       clusterv1.ObjectMeta{},
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
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
			old: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{Ref: ref},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Metadata:       clusterv1.ObjectMeta{},
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
								},
							},
						},
					},
				},
			},
			in: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: incompatibleRef},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{Ref: ref},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Metadata:       clusterv1.ObjectMeta{},
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
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
			old: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate:   clusterv1.LocalObjectTemplate{Ref: ref},
						MachineInfrastructure: &clusterv1.LocalObjectTemplate{Ref: ref},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Metadata:       clusterv1.ObjectMeta{},
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
								},
							},
						},
					},
				},
			},
			in: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
					ControlPlane: clusterv1.ControlPlaneClass{
						Metadata: clusterv1.ObjectMeta{
							Labels:      map[string]string{"foo": "bar"},
							Annotations: map[string]string{"foo": "bar"},
						},
						LocalObjectTemplate:   clusterv1.LocalObjectTemplate{Ref: compatibleRef},
						MachineInfrastructure: &clusterv1.LocalObjectTemplate{Ref: compatibleRef},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Metadata:       clusterv1.ObjectMeta{},
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
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
			old: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{Ref: ref},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Metadata:       clusterv1.ObjectMeta{},
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
								},
							},
						},
					},
				},
			},
			in: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{Ref: incompatibleRef},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Metadata:       clusterv1.ObjectMeta{},
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
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
			old: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate:   clusterv1.LocalObjectTemplate{Ref: ref},
						MachineInfrastructure: &clusterv1.LocalObjectTemplate{Ref: ref},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Metadata:       clusterv1.ObjectMeta{},
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
								},
							},
						},
					},
				},
			},
			in: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate:   clusterv1.LocalObjectTemplate{Ref: ref},
						MachineInfrastructure: &clusterv1.LocalObjectTemplate{Ref: incompatibleRef},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Metadata:       clusterv1.ObjectMeta{},
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
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
			old: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{Ref: ref},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Metadata:       clusterv1.ObjectMeta{},
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
								},
							},
						},
					},
				},
			},
			in: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{Ref: ref},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Metadata: clusterv1.ObjectMeta{
										Labels:      map[string]string{"foo": "bar"},
										Annotations: map[string]string{"foo": "bar"},
									},
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: incompatibleRef}, // NOTE: this should be tolerated
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: compatibleRef},
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
			old: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{Ref: ref},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Metadata:       clusterv1.ObjectMeta{},
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
								},
							},
						},
					},
				},
			},
			in: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{Ref: ref},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Metadata:       clusterv1.ObjectMeta{},
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: incompatibleRef},
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
			old: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{Ref: ref},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
								},
							},
						},
					},
				},
			},
			in: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{Ref: ref},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
								},
							},
							{
								Class: "bb",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
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
			old: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{Ref: ref},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
								},
							},
						},
					},
				},
			},
			in: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{Ref: ref},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
								},
							},
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
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
			old: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{Ref: ref},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
								},
							},
							{
								Class: "bb",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
								},
							},
						},
					},
				},
			},
			in: &clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: clusterv1.ClusterClassSpec{
					Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{Ref: ref},
					},
					Workers: clusterv1.WorkersClass{
						MachineDeployments: []clusterv1.MachineDeploymentClass{
							{
								Class: "aa",
								Template: clusterv1.MachineDeploymentClassTemplate{
									Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
									Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
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
			webhook := &ClusterClass{}
			if tt.expectErr {
				g.Expect(webhook.validate(tt.old, tt.in)).NotTo(Succeed())
			} else {
				g.Expect(webhook.validate(tt.old, tt.in)).To(Succeed())
			}
		})
	}
}
