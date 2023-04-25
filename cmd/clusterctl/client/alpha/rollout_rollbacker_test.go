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

package alpha

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/version"
)

func Test_ObjectRollbacker(t *testing.T) {
	labels := map[string]string{
		clusterv1.ClusterNameLabel:           "test",
		clusterv1.MachineDeploymentNameLabel: "test-md-0",
	}
	currentVersion := "v1.19.3"
	rollbackVersion := "v1.19.1"
	cluster := &clusterv1.Cluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Cluster",
			APIVersion: clusterv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: clusterv1.ClusterSpec{
			ControlPlaneRef: &corev1.ObjectReference{
				Namespace: "default",
				Name:      "test-kcp",
				Kind:      "KubeadmControlPlane",
			},
		},
	}
	kcp := &controlplanev1.KubeadmControlPlane{
		TypeMeta: metav1.TypeMeta{
			Kind: "KubeadmControlPlane",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-kcp",
		},
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			Version: currentVersion,
		},
	}
	deployment := &clusterv1.MachineDeployment{
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineDeployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-md-0",
			Namespace: "default",
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: "test",
			},
			Annotations: map[string]string{
				clusterv1.RevisionAnnotation: "2",
			},
		},
		Spec: clusterv1.MachineDeploymentSpec{
			ClusterName: "test",
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					clusterv1.ClusterNameLabel: "test",
				},
			},
			Template: clusterv1.MachineTemplateSpec{
				ObjectMeta: clusterv1.ObjectMeta{
					Labels: labels,
				},
				Spec: clusterv1.MachineSpec{
					ClusterName: "test",
					Version:     &currentVersion,
					InfrastructureRef: corev1.ObjectReference{
						APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
						Kind:       "InfrastructureMachineTemplate",
						Name:       "md-template",
					},
					Bootstrap: clusterv1.Bootstrap{
						DataSecretName: ptr.To("data-secret-name"),
					},
				},
			},
		},
	}
	type fields struct {
		objs       []client.Object
		ref        corev1.ObjectReference
		toRevision int64
		force      bool
	}
	tests := []struct {
		name                   string
		fields                 fields
		wantErr                bool
		wantVersion            string
		wantInfraTemplate      string
		wantBootsrapSecretName string
	}{
		{
			name: "machinedeployment should rollback to revision=1",
			fields: fields{
				objs: []client.Object{
					deployment,
					&clusterv1.MachineSet{
						TypeMeta: metav1.TypeMeta{
							Kind: "MachineSet",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      "ms-rev-2",
							Namespace: "default",
							OwnerReferences: []metav1.OwnerReference{
								*metav1.NewControllerRef(deployment, clusterv1.GroupVersion.WithKind("MachineDeployment")),
							},
							Labels: map[string]string{
								clusterv1.ClusterNameLabel: "test",
							},
							Annotations: map[string]string{
								clusterv1.RevisionAnnotation: "2",
							},
						},
					},
					&clusterv1.MachineSet{
						TypeMeta: metav1.TypeMeta{
							Kind: "MachineSet",
						},
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "ms-rev-1",
							OwnerReferences: []metav1.OwnerReference{
								*metav1.NewControllerRef(deployment, clusterv1.GroupVersion.WithKind("MachineDeployment")),
							},
							Labels: map[string]string{
								clusterv1.ClusterNameLabel: "test",
							},
							Annotations: map[string]string{
								clusterv1.RevisionAnnotation: "999",
							},
						},
						Spec: clusterv1.MachineSetSpec{
							ClusterName: "test",
							Selector: metav1.LabelSelector{
								MatchLabels: map[string]string{
									clusterv1.ClusterNameLabel: "test",
								},
							},
							Template: clusterv1.MachineTemplateSpec{
								ObjectMeta: clusterv1.ObjectMeta{
									Labels: labels,
								},
								Spec: clusterv1.MachineSpec{
									ClusterName: "test",
									Version:     &rollbackVersion,
									InfrastructureRef: corev1.ObjectReference{
										APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
										Kind:       "InfrastructureMachineTemplate",
										Name:       "md-template-rollback",
									},
									Bootstrap: clusterv1.Bootstrap{
										DataSecretName: ptr.To("data-secret-name-rollback"),
									},
								},
							},
						},
					},
					cluster,
					kcp,
				},
				ref: corev1.ObjectReference{
					Kind:      MachineDeployment,
					Name:      "test-md-0",
					Namespace: "default",
				},
				toRevision: int64(999),
				force:      false,
			},
			wantErr:                false,
			wantVersion:            rollbackVersion,
			wantInfraTemplate:      "md-template-rollback",
			wantBootsrapSecretName: "data-secret-name-rollback",
		},
		{
			name: "machinedeployment should not rollback because there is no previous revision",
			fields: fields{
				objs: []client.Object{
					deployment,
					&clusterv1.MachineSet{
						TypeMeta: metav1.TypeMeta{
							Kind: "MachineSet",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      "ms-rev-2",
							Namespace: "default",
							OwnerReferences: []metav1.OwnerReference{
								*metav1.NewControllerRef(deployment, clusterv1.GroupVersion.WithKind("MachineDeployment")),
							},
							Labels: map[string]string{
								clusterv1.ClusterNameLabel: "test",
							},
							Annotations: map[string]string{
								clusterv1.RevisionAnnotation: "2",
							},
						},
					},
					cluster,
					kcp,
				},
				ref: corev1.ObjectReference{
					Kind:      MachineDeployment,
					Name:      "test-md-0",
					Namespace: "default",
				},
				toRevision: int64(0),
				force:      false,
			},
			wantErr: true,
		},
		{
			name: "machinedeployment should not rollback because the specified version does not exist",
			fields: fields{
				objs: []client.Object{
					deployment,
					&clusterv1.MachineSet{
						TypeMeta: metav1.TypeMeta{
							Kind: "MachineSet",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      "ms-rev-2",
							Namespace: "default",
							OwnerReferences: []metav1.OwnerReference{
								*metav1.NewControllerRef(deployment, clusterv1.GroupVersion.WithKind("MachineDeployment")),
							},
							Labels: map[string]string{
								clusterv1.ClusterNameLabel: "test",
							},
							Annotations: map[string]string{
								clusterv1.RevisionAnnotation: "2",
							},
						},
					},
					cluster,
					kcp,
				},
				ref: corev1.ObjectReference{
					Kind:      MachineDeployment,
					Name:      "test-md-0",
					Namespace: "default",
				},
				toRevision: int64(999),
				force:      false,
			},
			wantErr: true,
		},
		{
			name: "machinedeployment should not rollback because of version skew policy violation",
			fields: fields{
				objs: []client.Object{
					deployment,
					&clusterv1.MachineSet{
						TypeMeta: metav1.TypeMeta{
							Kind: "MachineSet",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      "ms-rev-2",
							Namespace: "default",
							OwnerReferences: []metav1.OwnerReference{
								*metav1.NewControllerRef(deployment, clusterv1.GroupVersion.WithKind("MachineDeployment")),
							},
							Labels: map[string]string{
								clusterv1.ClusterNameLabel: "test",
							},
							Annotations: map[string]string{
								clusterv1.RevisionAnnotation: "999",
							},
						},
						Spec: clusterv1.MachineSetSpec{
							ClusterName: "test",
							Selector: metav1.LabelSelector{
								MatchLabels: map[string]string{
									clusterv1.ClusterNameLabel: "test",
								},
							},
							Template: clusterv1.MachineTemplateSpec{
								ObjectMeta: clusterv1.ObjectMeta{
									Labels: labels,
								},
								Spec: clusterv1.MachineSpec{
									ClusterName: "test",
									Version:     pointer.String("v0.0.0"),
									InfrastructureRef: corev1.ObjectReference{
										APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
										Kind:       "InfrastructureMachineTemplate",
										Name:       "md-template-rollback",
									},
									Bootstrap: clusterv1.Bootstrap{
										DataSecretName: pointer.String("data-secret-name-rollback"),
									},
								},
							},
						},
					},
					cluster,
					kcp,
				},
				ref: corev1.ObjectReference{
					Kind:      MachineDeployment,
					Name:      "test-md-0",
					Namespace: "default",
				},
				toRevision: int64(999),
				force:      false,
			},
			wantErr: true,
		},
		{
			name: "machinedeployment should not rollback because verion field of MachineSet is not set",
			fields: fields{
				objs: []client.Object{
					deployment,
					&clusterv1.MachineSet{
						TypeMeta: metav1.TypeMeta{
							Kind: "MachineSet",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      "ms-rev-2",
							Namespace: "default",
							OwnerReferences: []metav1.OwnerReference{
								*metav1.NewControllerRef(deployment, clusterv1.GroupVersion.WithKind("MachineDeployment")),
							},
							Labels: map[string]string{
								clusterv1.ClusterNameLabel: "test",
							},
							Annotations: map[string]string{
								clusterv1.RevisionAnnotation: "999",
							},
						},
						Spec: clusterv1.MachineSetSpec{
							ClusterName: "test",
							Selector: metav1.LabelSelector{
								MatchLabels: map[string]string{
									clusterv1.ClusterNameLabel: "test",
								},
							},
							Template: clusterv1.MachineTemplateSpec{
								ObjectMeta: clusterv1.ObjectMeta{
									Labels: labels,
								},
								Spec: clusterv1.MachineSpec{
									ClusterName: "test",
									InfrastructureRef: corev1.ObjectReference{
										APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
										Kind:       "InfrastructureMachineTemplate",
										Name:       "md-template-rollback",
									},
									Bootstrap: clusterv1.Bootstrap{
										DataSecretName: pointer.String("data-secret-name-rollback"),
									},
								},
							},
						},
					},
					cluster,
					kcp,
				},
				ref: corev1.ObjectReference{
					Kind:      MachineDeployment,
					Name:      "test-md-0",
					Namespace: "default",
				},
				toRevision: int64(999),
				force:      false,
			},
			wantErr: true,
		},
		{
			name: "machinedeployment should rollback to revision=1",
			fields: fields{
				objs: []client.Object{
					deployment,
					&clusterv1.MachineSet{
						TypeMeta: metav1.TypeMeta{
							Kind: "MachineSet",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      "ms-rev-2",
							Namespace: "default",
							OwnerReferences: []metav1.OwnerReference{
								*metav1.NewControllerRef(deployment, clusterv1.GroupVersion.WithKind("MachineDeployment")),
							},
							Labels: map[string]string{
								clusterv1.ClusterNameLabel: "test",
							},
							Annotations: map[string]string{
								clusterv1.RevisionAnnotation: "2",
							},
						},
					},
					&clusterv1.MachineSet{
						TypeMeta: metav1.TypeMeta{
							Kind: "MachineSet",
						},
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "ms-rev-1",
							OwnerReferences: []metav1.OwnerReference{
								*metav1.NewControllerRef(deployment, clusterv1.GroupVersion.WithKind("MachineDeployment")),
							},
							Labels: map[string]string{
								clusterv1.ClusterNameLabel: "test",
							},
							Annotations: map[string]string{
								clusterv1.RevisionAnnotation: "999",
							},
						},
						Spec: clusterv1.MachineSetSpec{
							ClusterName: "test",
							Selector: metav1.LabelSelector{
								MatchLabels: map[string]string{
									clusterv1.ClusterNameLabel: "test",
								},
							},
							Template: clusterv1.MachineTemplateSpec{
								ObjectMeta: clusterv1.ObjectMeta{
									Labels: labels,
								},
								Spec: clusterv1.MachineSpec{
									ClusterName: "test",
									Version:     pointer.String("v0.0.0"),
									InfrastructureRef: corev1.ObjectReference{
										APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
										Kind:       "InfrastructureMachineTemplate",
										Name:       "md-template-rollback",
									},
									Bootstrap: clusterv1.Bootstrap{
										DataSecretName: pointer.String("data-secret-name-rollback"),
									},
								},
							},
						},
					},
					cluster,
					kcp,
				},
				ref: corev1.ObjectReference{
					Kind:      MachineDeployment,
					Name:      "test-md-0",
					Namespace: "default",
				},
				toRevision: int64(999),
				force:      true,
			},
			wantErr:                false,
			wantVersion:            "v0.0.0",
			wantInfraTemplate:      "md-template-rollback",
			wantBootsrapSecretName: "data-secret-name-rollback",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			r := newRolloutClient()
			proxy := test.NewFakeProxy().WithObjs(tt.fields.objs...)
			err := r.ObjectRollbacker(context.Background(), proxy, tt.fields.ref, tt.fields.toRevision, tt.fields.force)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			cl, err := proxy.NewClient(context.Background())
			g.Expect(err).ToNot(HaveOccurred())
			key := client.ObjectKeyFromObject(deployment)
			md := &clusterv1.MachineDeployment{}
			err = cl.Get(context.TODO(), key, md)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(*md.Spec.Template.Spec.Version).To(Equal(tt.wantVersion))
			g.Expect(md.Spec.Template.Spec.InfrastructureRef.Name).To(Equal(tt.wantInfraTemplate))
			g.Expect(*md.Spec.Template.Spec.Bootstrap.DataSecretName).To(Equal(tt.wantBootsrapSecretName))
		})
	}
}

func Test_validateVersionSkewPolicy(t *testing.T) {
	tests := []struct {
		name, cpVersion, msVersion string
		wantErr                    bool
	}{
		{
			name:      "MS has the same version as CP",
			cpVersion: "v1.26.0",
			msVersion: "v1.26.0",
			wantErr:   false,
		},
		{
			name:      "MS has a newer patch version",
			cpVersion: "v1.26.0",
			msVersion: "v1.26.1",
			wantErr:   false,
		},
		{
			name:      "MS is two minor version behind CP",
			cpVersion: "v1.26.0",
			msVersion: "v1.24.0",
			wantErr:   false,
		},
		{
			name:      "MS is three minor version behind CP",
			cpVersion: "v1.26.0",
			msVersion: "v1.23.0",
			wantErr:   true,
		},
		{
			name:      "MS is newer than CP",
			cpVersion: "v1.26.0",
			msVersion: "v1.27.0",
			wantErr:   true,
		},
		{
			name:      "Major versions are different",
			cpVersion: "v1.26.0",
			msVersion: "v0.26.0",
			wantErr:   true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			g := NewWithT(t)
			cpVersion, _ := version.ParseMajorMinorPatch(test.cpVersion)
			msVersion, _ := version.ParseMajorMinorPatch(test.msVersion)
			err := validateVersionSkewPolicy(cpVersion, msVersion)
			if test.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
		})
	}
}
