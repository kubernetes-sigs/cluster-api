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

package admission

import (
	"testing"

	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

func TestCustomResourceDefinitionWebhookValidateUpdate(t *testing.T) {
	crd := func(group, kind string, versions ...string) *apiextensionsv1.CustomResourceDefinition {
		var specVersions []apiextensionsv1.CustomResourceDefinitionVersion
		for _, v := range versions {
			specVersions = append(specVersions, apiextensionsv1.CustomResourceDefinitionVersion{Name: v, Served: true})
		}
		return &apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: kind + "." + group},
			Spec: apiextensionsv1.CustomResourceDefinitionSpec{
				Group:    group,
				Names:    apiextensionsv1.CustomResourceDefinitionNames{Kind: kind},
				Versions: specVersions,
			},
		}
	}

	const (
		infraGroup = "infrastructure.cluster.x-k8s.io"
		infraKind  = "AWSClusterTemplate"
		cpGroup    = "controlplane.cluster.x-k8s.io"
		cpKind     = "KubeadmControlPlaneTemplate"
	)

	infraV1beta1 := infraGroup + "/v1beta1"
	infraV1beta2 := infraGroup + "/v1beta2"
	cpV1beta1 := cpGroup + "/v1beta1"

	tests := []struct {
		name           string
		oldCRD         *apiextensionsv1.CustomResourceDefinition
		newCRD         *apiextensionsv1.CustomResourceDefinition
		clusterClasses []*clusterv1.ClusterClass
		wantErr        bool
		wantErrSubstr  string
	}{
		// Sanity checks for allowed changes to CRD versions
		{
			name:    "no versions dropped: allowed",
			oldCRD:  crd(infraGroup, infraKind, "v1beta1", "v1beta2"),
			newCRD:  crd(infraGroup, infraKind, "v1beta1", "v1beta2"),
			wantErr: false,
		},
		{
			name:    "version added only: allowed",
			oldCRD:  crd(infraGroup, infraKind, "v1beta1"),
			newCRD:  crd(infraGroup, infraKind, "v1beta1", "v1beta2"),
			wantErr: false,
		},
		{
			name:    "version dropped, no ClusterClasses: allowed",
			oldCRD:  crd(infraGroup, infraKind, "v1beta1", "v1beta2"),
			newCRD:  crd(infraGroup, infraKind, "v1beta2"),
			wantErr: false,
		},
		{
			name:   "version dropped, different group/kind in ClusterClass: allowed",
			oldCRD: crd(infraGroup, infraKind, "v1beta1", "v1beta2"),
			newCRD: crd(infraGroup, infraKind, "v1beta2"),
			clusterClasses: []*clusterv1.ClusterClass{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "cc1", Namespace: "default"},
					Spec: clusterv1.ClusterClassSpec{
						Infrastructure: clusterv1.InfrastructureClass{
							TemplateRef: clusterv1.ClusterClassTemplateReference{
								APIVersion: cpV1beta1, // different group
								Kind:       cpKind,
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name:   "version dropped, ClusterClass uses newer version of same kind: allowed",
			oldCRD: crd(infraGroup, infraKind, "v1beta1", "v1beta2"),
			newCRD: crd(infraGroup, infraKind, "v1beta2"),
			clusterClasses: []*clusterv1.ClusterClass{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "cc1", Namespace: "default"},
					Spec: clusterv1.ClusterClassSpec{
						Infrastructure: clusterv1.InfrastructureClass{
							TemplateRef: clusterv1.ClusterClassTemplateReference{
								APIVersion: infraV1beta2, // uses v1beta2, not the dropped v1beta1
								Kind:       infraKind,
							},
						},
					},
				},
			},
			wantErr: false,
		},
		// Check deny behavior for all ClusterClass template references that can reference a CRD
		{
			name:   "version dropped, ClusterClass infrastructure ref matches: denied",
			oldCRD: crd(infraGroup, infraKind, "v1beta1", "v1beta2"),
			newCRD: crd(infraGroup, infraKind, "v1beta2"),
			clusterClasses: []*clusterv1.ClusterClass{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "cc1", Namespace: "default"},
					Spec: clusterv1.ClusterClassSpec{
						Infrastructure: clusterv1.InfrastructureClass{
							TemplateRef: clusterv1.ClusterClassTemplateReference{
								APIVersion: infraV1beta1,
								Kind:       infraKind,
							},
						},
					},
				},
			},
			wantErr:       true,
			wantErrSubstr: "default/cc1",
		},
		{
			name:   "version dropped, ClusterClass controlPlane ref matches: denied",
			oldCRD: crd(cpGroup, cpKind, "v1beta1", "v1beta2"),
			newCRD: crd(cpGroup, cpKind, "v1beta2"),
			clusterClasses: []*clusterv1.ClusterClass{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "cc1", Namespace: "default"},
					Spec: clusterv1.ClusterClassSpec{
						ControlPlane: clusterv1.ControlPlaneClass{
							TemplateRef: clusterv1.ClusterClassTemplateReference{
								APIVersion: cpV1beta1,
								Kind:       cpKind,
							},
						},
					},
				},
			},
			wantErr:       true,
			wantErrSubstr: "default/cc1",
		},
		{
			name:   "version dropped, ClusterClass controlPlane machineInfrastructure ref matches: denied",
			oldCRD: crd(infraGroup, infraKind, "v1beta1", "v1beta2"),
			newCRD: crd(infraGroup, infraKind, "v1beta2"),
			clusterClasses: []*clusterv1.ClusterClass{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "cc1", Namespace: "default"},
					Spec: clusterv1.ClusterClassSpec{
						ControlPlane: clusterv1.ControlPlaneClass{
							MachineInfrastructure: clusterv1.ControlPlaneClassMachineInfrastructureTemplate{
								TemplateRef: clusterv1.ClusterClassTemplateReference{
									APIVersion: infraV1beta1,
									Kind:       infraKind,
								},
							},
						},
					},
				},
			},
			wantErr:       true,
			wantErrSubstr: "default/cc1",
		},
		{
			name:   "version dropped, ClusterClass MachineDeployment bootstrap ref matches: denied",
			oldCRD: crd(infraGroup, "KubeadmConfigTemplate", "v1beta1", "v1beta2"),
			newCRD: crd(infraGroup, "KubeadmConfigTemplate", "v1beta2"),
			clusterClasses: []*clusterv1.ClusterClass{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "cc1", Namespace: "default"},
					Spec: clusterv1.ClusterClassSpec{
						Workers: clusterv1.WorkersClass{
							MachineDeployments: []clusterv1.MachineDeploymentClass{
								{
									Class: "worker",
									Bootstrap: clusterv1.MachineDeploymentClassBootstrapTemplate{
										TemplateRef: clusterv1.ClusterClassTemplateReference{
											APIVersion: infraGroup + "/v1beta1",
											Kind:       "KubeadmConfigTemplate",
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr:       true,
			wantErrSubstr: "default/cc1",
		},
		{
			name:   "version dropped, ClusterClass MachineDeployment infrastructure ref matches: denied",
			oldCRD: crd(infraGroup, infraKind, "v1beta1", "v1beta2"),
			newCRD: crd(infraGroup, infraKind, "v1beta2"),
			clusterClasses: []*clusterv1.ClusterClass{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "cc1", Namespace: "default"},
					Spec: clusterv1.ClusterClassSpec{
						Workers: clusterv1.WorkersClass{
							MachineDeployments: []clusterv1.MachineDeploymentClass{
								{
									Class: "worker",
									Infrastructure: clusterv1.MachineDeploymentClassInfrastructureTemplate{
										TemplateRef: clusterv1.ClusterClassTemplateReference{
											APIVersion: infraV1beta1,
											Kind:       infraKind,
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr:       true,
			wantErrSubstr: "default/cc1",
		},
		{
			name:   "version dropped, ClusterClass MachineDeployment MHC remediation ref matches: denied",
			oldCRD: crd(infraGroup, "RemediationTemplate", "v1beta1", "v1beta2"),
			newCRD: crd(infraGroup, "RemediationTemplate", "v1beta2"),
			clusterClasses: []*clusterv1.ClusterClass{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "cc1", Namespace: "default"},
					Spec: clusterv1.ClusterClassSpec{
						Workers: clusterv1.WorkersClass{
							MachineDeployments: []clusterv1.MachineDeploymentClass{
								{
									Class: "worker",
									HealthCheck: clusterv1.MachineDeploymentClassHealthCheck{
										Remediation: clusterv1.MachineDeploymentClassHealthCheckRemediation{
											TemplateRef: clusterv1.MachineHealthCheckRemediationTemplateReference{
												APIVersion: infraGroup + "/v1beta1",
												Kind:       "RemediationTemplate",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr:       true,
			wantErrSubstr: "default/cc1",
		},
		{
			name:   "version dropped, ClusterClass MachinePool bootstrap ref matches: denied",
			oldCRD: crd(infraGroup, "KubeadmConfigTemplate", "v1beta1", "v1beta2"),
			newCRD: crd(infraGroup, "KubeadmConfigTemplate", "v1beta2"),
			clusterClasses: []*clusterv1.ClusterClass{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "cc1", Namespace: "default"},
					Spec: clusterv1.ClusterClassSpec{
						Workers: clusterv1.WorkersClass{
							MachinePools: []clusterv1.MachinePoolClass{
								{
									Class: "pool",
									Bootstrap: clusterv1.MachinePoolClassBootstrapTemplate{
										TemplateRef: clusterv1.ClusterClassTemplateReference{
											APIVersion: infraGroup + "/v1beta1",
											Kind:       "KubeadmConfigTemplate",
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr:       true,
			wantErrSubstr: "default/cc1",
		},
		{
			name:   "version dropped, ClusterClass MachinePool infrastructure ref matches: denied",
			oldCRD: crd(infraGroup, infraKind, "v1beta1", "v1beta2"),
			newCRD: crd(infraGroup, infraKind, "v1beta2"),
			clusterClasses: []*clusterv1.ClusterClass{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "cc1", Namespace: "default"},
					Spec: clusterv1.ClusterClassSpec{
						Workers: clusterv1.WorkersClass{
							MachinePools: []clusterv1.MachinePoolClass{
								{
									Class: "pool",
									Infrastructure: clusterv1.MachinePoolClassInfrastructureTemplate{
										TemplateRef: clusterv1.ClusterClassTemplateReference{
											APIVersion: infraV1beta1,
											Kind:       infraKind,
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr:       true,
			wantErrSubstr: "default/cc1",
		},
		{
			name:   "version dropped, ClusterClass MHC remediation ref matches: denied",
			oldCRD: crd(infraGroup, "RemediationTemplate", "v1beta1", "v1beta2"),
			newCRD: crd(infraGroup, "RemediationTemplate", "v1beta2"),
			clusterClasses: []*clusterv1.ClusterClass{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "cc1", Namespace: "default"},
					Spec: clusterv1.ClusterClassSpec{
						ControlPlane: clusterv1.ControlPlaneClass{
							HealthCheck: clusterv1.ControlPlaneClassHealthCheck{
								Remediation: clusterv1.ControlPlaneClassHealthCheckRemediation{
									TemplateRef: clusterv1.MachineHealthCheckRemediationTemplateReference{
										APIVersion: infraGroup + "/v1beta1",
										Kind:       "RemediationTemplate",
									},
								},
							},
						},
					},
				},
			},
			wantErr:       true,
			wantErrSubstr: "default/cc1",
		},
		{
			name:   "version dropped, multiple ClusterClasses affected: all named in error",
			oldCRD: crd(infraGroup, infraKind, "v1beta1", "v1beta2"),
			newCRD: crd(infraGroup, infraKind, "v1beta2"),
			clusterClasses: []*clusterv1.ClusterClass{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "cc1", Namespace: "default"},
					Spec: clusterv1.ClusterClassSpec{
						Infrastructure: clusterv1.InfrastructureClass{
							TemplateRef: clusterv1.ClusterClassTemplateReference{
								APIVersion: infraV1beta1,
								Kind:       infraKind,
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "cc2", Namespace: "other"},
					Spec: clusterv1.ClusterClassSpec{
						Infrastructure: clusterv1.InfrastructureClass{
							TemplateRef: clusterv1.ClusterClassTemplateReference{
								APIVersion: infraV1beta1,
								Kind:       infraKind,
							},
						},
					},
				},
			},
			wantErr:       true,
			wantErrSubstr: "cc1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			objs := make([]clusterv1.ClusterClass, 0, len(tt.clusterClasses))
			for _, cc := range tt.clusterClasses {
				objs = append(objs, *cc)
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(fakeScheme).
				WithLists(&clusterv1.ClusterClassList{Items: objs}).
				Build()

			webhook := &CustomResourceDefinitionWebhook{Client: fakeClient}
			_, err := webhook.ValidateUpdate(ctx, tt.oldCRD, tt.newCRD)

			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				if tt.wantErrSubstr != "" {
					g.Expect(err.Error()).To(ContainSubstring(tt.wantErrSubstr))
				}
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}
