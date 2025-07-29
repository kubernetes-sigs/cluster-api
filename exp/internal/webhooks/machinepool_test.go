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
	"context"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/internal/webhooks/util"
)

var ctx = ctrl.SetupSignalHandler()

func TestMachinePoolDefault(t *testing.T) {
	g := NewWithT(t)

	mp := &clusterv1.MachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "foobar",
		},
		Spec: clusterv1.MachinePoolSpec{
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{ConfigRef: clusterv1.ContractVersionedObjectReference{
						Name: "bootstrap",
					}},
					Version: "1.20.0",
				},
			},
		},
	}
	webhook := &MachinePool{}
	ctx = admission.NewContextWithRequest(ctx, admission.Request{})
	t.Run("for MachinePool", util.CustomDefaultValidateTest(ctx, mp, webhook))
	g.Expect(webhook.Default(ctx, mp)).To(Succeed())

	g.Expect(mp.Labels[clusterv1.ClusterNameLabel]).To(Equal(mp.Spec.ClusterName))
	g.Expect(mp.Spec.Replicas).To(Equal(ptr.To[int32](1)))
	g.Expect(mp.Spec.Template.Spec.Version).To(Equal("v1.20.0"))
	g.Expect(*mp.Spec.Template.Spec.Deletion.NodeDeletionTimeoutSeconds).To(Equal(defaultNodeDeletionTimeoutSeconds))
}

func TestCalculateMachinePoolReplicas(t *testing.T) {
	tests := []struct {
		name             string
		newMP            *clusterv1.MachinePool
		oldMP            *clusterv1.MachinePool
		expectedReplicas int32
		expectErr        bool
	}{
		{
			name: "if new MP has replicas set, keep that value",
			newMP: &clusterv1.MachinePool{
				Spec: clusterv1.MachinePoolSpec{
					Replicas: ptr.To[int32](5),
				},
			},
			expectedReplicas: 5,
		},
		{
			name:             "if new MP does not have replicas set and no annotations, use 1",
			newMP:            &clusterv1.MachinePool{},
			expectedReplicas: 1,
		},
		{
			name: "if new MP only has min size annotation, fallback to 1",
			newMP: &clusterv1.MachinePool{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						clusterv1.AutoscalerMinSizeAnnotation: "3",
					},
				},
			},
			expectedReplicas: 1,
		},
		{
			name: "if new MP only has max size annotation, fallback to 1",
			newMP: &clusterv1.MachinePool{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						clusterv1.AutoscalerMaxSizeAnnotation: "7",
					},
				},
			},
			expectedReplicas: 1,
		},
		{
			name: "if new MP has min and max size annotation and min size is invalid, fail",
			newMP: &clusterv1.MachinePool{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						clusterv1.AutoscalerMinSizeAnnotation: "abc",
						clusterv1.AutoscalerMaxSizeAnnotation: "7",
					},
				},
			},
			expectErr: true,
		},
		{
			name: "if new MP has min and max size annotation and max size is invalid, fail",
			newMP: &clusterv1.MachinePool{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						clusterv1.AutoscalerMinSizeAnnotation: "3",
						clusterv1.AutoscalerMaxSizeAnnotation: "abc",
					},
				},
			},
			expectErr: true,
		},
		{
			name: "if new MP has min and max size annotation and new MP is a new MP, use min size",
			newMP: &clusterv1.MachinePool{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						clusterv1.AutoscalerMinSizeAnnotation: "3",
						clusterv1.AutoscalerMaxSizeAnnotation: "7",
					},
				},
			},
			expectedReplicas: 3,
		},
		{
			name: "if new MP has min and max size annotation and old MP doesn't have replicas set, use min size",
			newMP: &clusterv1.MachinePool{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						clusterv1.AutoscalerMinSizeAnnotation: "3",
						clusterv1.AutoscalerMaxSizeAnnotation: "7",
					},
				},
			},
			oldMP:            &clusterv1.MachinePool{},
			expectedReplicas: 3,
		},
		{
			name: "if new MP has min and max size annotation and old MP replicas is below min size, use min size",
			newMP: &clusterv1.MachinePool{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						clusterv1.AutoscalerMinSizeAnnotation: "3",
						clusterv1.AutoscalerMaxSizeAnnotation: "7",
					},
				},
			},
			oldMP: &clusterv1.MachinePool{
				Spec: clusterv1.MachinePoolSpec{
					Replicas: ptr.To[int32](1),
				},
			},
			expectedReplicas: 3,
		},
		{
			name: "if new MP has min and max size annotation and old MP replicas is above max size, use max size",
			newMP: &clusterv1.MachinePool{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						clusterv1.AutoscalerMinSizeAnnotation: "3",
						clusterv1.AutoscalerMaxSizeAnnotation: "7",
					},
				},
			},
			oldMP: &clusterv1.MachinePool{
				Spec: clusterv1.MachinePoolSpec{
					Replicas: ptr.To[int32](15),
				},
			},
			expectedReplicas: 7,
		},
		{
			name: "if new MP has min and max size annotation and old MP replicas is between min and max size, use old MP replicas",
			newMP: &clusterv1.MachinePool{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						clusterv1.AutoscalerMinSizeAnnotation: "3",
						clusterv1.AutoscalerMaxSizeAnnotation: "7",
					},
				},
			},
			oldMP: &clusterv1.MachinePool{
				Spec: clusterv1.MachinePoolSpec{
					Replicas: ptr.To[int32](4),
				},
			},
			expectedReplicas: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			replicas, err := calculateMachinePoolReplicas(context.Background(), tt.oldMP, tt.newMP, false)

			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(replicas).To(Equal(tt.expectedReplicas))
		})
	}
}

func TestMachinePoolBootstrapValidation(t *testing.T) {
	tests := []struct {
		name      string
		bootstrap clusterv1.Bootstrap
		expectErr bool
	}{
		{
			name:      "should return error if configref and data are nil",
			bootstrap: clusterv1.Bootstrap{ConfigRef: clusterv1.ContractVersionedObjectReference{}, DataSecretName: nil},
			expectErr: true,
		},
		{
			name:      "should not return error if dataSecretName is set",
			bootstrap: clusterv1.Bootstrap{ConfigRef: clusterv1.ContractVersionedObjectReference{}, DataSecretName: ptr.To("test")},
			expectErr: false,
		},
		{
			name:      "should not return error if config ref is set",
			bootstrap: clusterv1.Bootstrap{ConfigRef: clusterv1.ContractVersionedObjectReference{Name: "bootstrap1"}, DataSecretName: nil},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			webhook := &MachinePool{}
			mp := &clusterv1.MachinePool{
				Spec: clusterv1.MachinePoolSpec{
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Bootstrap: tt.bootstrap,
						},
					},
				},
			}

			if tt.expectErr {
				warnings, err := webhook.ValidateCreate(ctx, mp)
				g.Expect(err).To(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
				warnings, err = webhook.ValidateUpdate(ctx, mp, mp)
				g.Expect(err).To(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
			} else {
				warnings, err := webhook.ValidateCreate(ctx, mp)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
				warnings, err = webhook.ValidateUpdate(ctx, mp, mp)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
			}
		})
	}
}

func TestMachinePoolClusterNameImmutable(t *testing.T) {
	tests := []struct {
		name           string
		oldClusterName string
		newClusterName string
		expectErr      bool
	}{
		{
			name:           "when the cluster name has not changed",
			oldClusterName: "foo",
			newClusterName: "foo",
			expectErr:      false,
		},
		{
			name:           "when the cluster name has changed",
			oldClusterName: "foo",
			newClusterName: "bar",
			expectErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			newMP := &clusterv1.MachinePool{
				Spec: clusterv1.MachinePoolSpec{
					ClusterName: tt.newClusterName,
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							ClusterName: tt.newClusterName,
							Bootstrap: clusterv1.Bootstrap{ConfigRef: clusterv1.ContractVersionedObjectReference{
								Name: "bootstrap",
							}},
						},
					},
				},
			}

			oldMP := &clusterv1.MachinePool{
				Spec: clusterv1.MachinePoolSpec{
					ClusterName: tt.oldClusterName,
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							ClusterName: tt.oldClusterName,
							Bootstrap: clusterv1.Bootstrap{ConfigRef: clusterv1.ContractVersionedObjectReference{
								Name: "bootstrap",
							}},
						},
					},
				},
			}

			webhook := MachinePool{}
			warnings, err := webhook.ValidateUpdate(ctx, oldMP, newMP)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(warnings).To(BeEmpty())
		})
	}
}

func TestMachinePoolClusterNamesEqual(t *testing.T) {
	tests := []struct {
		name                        string
		specClusterName             string
		specTemplateSpecClusterName string
		expectErr                   bool
	}{
		{
			name:                        "clusterName fields are set to the same value",
			specClusterName:             "foo",
			specTemplateSpecClusterName: "foo",
			expectErr:                   false,
		},
		{
			name:                        "clusterName fields are set to different values",
			specClusterName:             "foo",
			specTemplateSpecClusterName: "bar",
			expectErr:                   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ms := &clusterv1.MachinePool{
				Spec: clusterv1.MachinePoolSpec{
					ClusterName: tt.specClusterName,
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							ClusterName: tt.specTemplateSpecClusterName,
							Bootstrap: clusterv1.Bootstrap{
								DataSecretName: ptr.To("data-secret"),
							},
						},
					},
				},
			}

			warnings, err := (&MachinePool{}).ValidateCreate(ctx, ms)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(warnings).To(BeEmpty())
		})
	}
}

func TestMachinePoolVersionValidation(t *testing.T) {
	tests := []struct {
		name      string
		expectErr bool
		version   string
	}{
		{
			name:      "should succeed version is a valid kube semver",
			expectErr: false,
			version:   "v1.23.3",
		},
		{
			name:      "should succeed version is a valid pre-release",
			expectErr: false,
			version:   "v1.19.0-alpha.1",
		},
		{
			name:      "should fail if version is not a valid semver",
			expectErr: true,
			version:   "v1.1",
		},
		{
			name:      "should fail if version is missing a v prefix",
			expectErr: true,
			version:   "1.20.0",
		},
	}

	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			mp := &clusterv1.MachinePool{
				Spec: clusterv1.MachinePoolSpec{
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Bootstrap: clusterv1.Bootstrap{ConfigRef: clusterv1.ContractVersionedObjectReference{
								Name: "bootstrap",
							}},
							Version: tt.version,
						},
					},
				},
			}
			webhook := &MachinePool{}

			if tt.expectErr {
				warnings, err := webhook.ValidateCreate(ctx, mp)
				g.Expect(err).To(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
				warnings, err = webhook.ValidateUpdate(ctx, mp, mp)
				g.Expect(err).To(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
			} else {
				warnings, err := webhook.ValidateCreate(ctx, mp)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
				warnings, err = webhook.ValidateUpdate(ctx, mp, mp)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
			}
		})
	}
}

func TestMachinePoolMetadataValidation(t *testing.T) {
	tests := []struct {
		name        string
		labels      map[string]string
		annotations map[string]string
		expectErr   bool
	}{
		{
			name: "should return error for invalid labels and annotations",
			labels: map[string]string{
				"foo":          "$invalid-key",
				"bar":          strings.Repeat("a", 64) + "too-long-value",
				"/invalid-key": "foo",
			},
			annotations: map[string]string{
				"/invalid-key": "foo",
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			mp := &clusterv1.MachinePool{
				Spec: clusterv1.MachinePoolSpec{
					Template: clusterv1.MachineTemplateSpec{
						ObjectMeta: clusterv1.ObjectMeta{
							Labels:      tt.labels,
							Annotations: tt.annotations,
						},
					},
				},
			}
			webhook := &MachinePool{}
			if tt.expectErr {
				warnings, err := webhook.ValidateCreate(ctx, mp)
				g.Expect(err).To(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
				warnings, err = webhook.ValidateUpdate(ctx, mp, mp)
				g.Expect(err).To(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
			} else {
				warnings, err := webhook.ValidateCreate(ctx, mp)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
				warnings, err = webhook.ValidateUpdate(ctx, mp, mp)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
			}
		})
	}
}
