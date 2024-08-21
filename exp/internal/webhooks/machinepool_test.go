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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/webhooks/util"
)

var ctx = ctrl.SetupSignalHandler()

func TestMachinePoolDefault(t *testing.T) {
	g := NewWithT(t)

	mp := &expv1.MachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "foobar",
		},
		Spec: expv1.MachinePoolSpec{
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{ConfigRef: &corev1.ObjectReference{}},
					Version:   ptr.To("1.20.0"),
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
	g.Expect(mp.Spec.MinReadySeconds).To(Equal(ptr.To[int32](0)))
	g.Expect(mp.Spec.Template.Spec.Bootstrap.ConfigRef.Namespace).To(Equal(mp.Namespace))
	g.Expect(mp.Spec.Template.Spec.InfrastructureRef.Namespace).To(Equal(mp.Namespace))
	g.Expect(mp.Spec.Template.Spec.Version).To(Equal(ptr.To("v1.20.0")))
	g.Expect(mp.Spec.Template.Spec.NodeDeletionTimeout).To(Equal(&metav1.Duration{Duration: defaultNodeDeletionTimeout}))
}

func TestCalculateMachinePoolReplicas(t *testing.T) {
	tests := []struct {
		name             string
		newMP            *expv1.MachinePool
		oldMP            *expv1.MachinePool
		expectedReplicas int32
		expectErr        bool
	}{
		{
			name: "if new MP has replicas set, keep that value",
			newMP: &expv1.MachinePool{
				Spec: expv1.MachinePoolSpec{
					Replicas: ptr.To[int32](5),
				},
			},
			expectedReplicas: 5,
		},
		{
			name:             "if new MP does not have replicas set and no annotations, use 1",
			newMP:            &expv1.MachinePool{},
			expectedReplicas: 1,
		},
		{
			name: "if new MP only has min size annotation, fallback to 1",
			newMP: &expv1.MachinePool{
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
			newMP: &expv1.MachinePool{
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
			newMP: &expv1.MachinePool{
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
			newMP: &expv1.MachinePool{
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
			newMP: &expv1.MachinePool{
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
			newMP: &expv1.MachinePool{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						clusterv1.AutoscalerMinSizeAnnotation: "3",
						clusterv1.AutoscalerMaxSizeAnnotation: "7",
					},
				},
			},
			oldMP:            &expv1.MachinePool{},
			expectedReplicas: 3,
		},
		{
			name: "if new MP has min and max size annotation and old MP replicas is below min size, use min size",
			newMP: &expv1.MachinePool{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						clusterv1.AutoscalerMinSizeAnnotation: "3",
						clusterv1.AutoscalerMaxSizeAnnotation: "7",
					},
				},
			},
			oldMP: &expv1.MachinePool{
				Spec: expv1.MachinePoolSpec{
					Replicas: ptr.To[int32](1),
				},
			},
			expectedReplicas: 3,
		},
		{
			name: "if new MP has min and max size annotation and old MP replicas is above max size, use max size",
			newMP: &expv1.MachinePool{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						clusterv1.AutoscalerMinSizeAnnotation: "3",
						clusterv1.AutoscalerMaxSizeAnnotation: "7",
					},
				},
			},
			oldMP: &expv1.MachinePool{
				Spec: expv1.MachinePoolSpec{
					Replicas: ptr.To[int32](15),
				},
			},
			expectedReplicas: 7,
		},
		{
			name: "if new MP has min and max size annotation and old MP replicas is between min and max size, use old MP replicas",
			newMP: &expv1.MachinePool{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						clusterv1.AutoscalerMinSizeAnnotation: "3",
						clusterv1.AutoscalerMaxSizeAnnotation: "7",
					},
				},
			},
			oldMP: &expv1.MachinePool{
				Spec: expv1.MachinePoolSpec{
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
			bootstrap: clusterv1.Bootstrap{ConfigRef: nil, DataSecretName: nil},
			expectErr: true,
		},
		{
			name:      "should not return error if dataSecretName is set",
			bootstrap: clusterv1.Bootstrap{ConfigRef: nil, DataSecretName: ptr.To("test")},
			expectErr: false,
		},
		{
			name:      "should not return error if config ref is set",
			bootstrap: clusterv1.Bootstrap{ConfigRef: &corev1.ObjectReference{}, DataSecretName: nil},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			webhook := &MachinePool{}
			mp := &expv1.MachinePool{
				Spec: expv1.MachinePoolSpec{
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

func TestMachinePoolNamespaceValidation(t *testing.T) {
	tests := []struct {
		name      string
		expectErr bool
		bootstrap clusterv1.Bootstrap
		infraRef  corev1.ObjectReference
		namespace string
	}{
		{
			name:      "should succeed if all namespaces match",
			expectErr: false,
			namespace: "foobar",
			bootstrap: clusterv1.Bootstrap{ConfigRef: &corev1.ObjectReference{Namespace: "foobar"}},
			infraRef:  corev1.ObjectReference{Namespace: "foobar"},
		},
		{
			name:      "should return error if namespace and bootstrap namespace don't match",
			expectErr: true,
			namespace: "foobar",
			bootstrap: clusterv1.Bootstrap{ConfigRef: &corev1.ObjectReference{Namespace: "foobar123"}},
			infraRef:  corev1.ObjectReference{Namespace: "foobar"},
		},
		{
			name:      "should return error if namespace and infrastructure ref namespace don't match",
			expectErr: true,
			namespace: "foobar",
			bootstrap: clusterv1.Bootstrap{ConfigRef: &corev1.ObjectReference{Namespace: "foobar"}},
			infraRef:  corev1.ObjectReference{Namespace: "foobar123"},
		},
		{
			name:      "should return error if no namespaces match",
			expectErr: true,
			namespace: "foobar1",
			bootstrap: clusterv1.Bootstrap{ConfigRef: &corev1.ObjectReference{Namespace: "foobar2"}},
			infraRef:  corev1.ObjectReference{Namespace: "foobar3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			webhook := &MachinePool{}
			mp := &expv1.MachinePool{
				ObjectMeta: metav1.ObjectMeta{Namespace: tt.namespace},
				Spec: expv1.MachinePoolSpec{
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Bootstrap:         tt.bootstrap,
							InfrastructureRef: tt.infraRef,
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

			newMP := &expv1.MachinePool{
				Spec: expv1.MachinePoolSpec{
					ClusterName: tt.newClusterName,
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Bootstrap: clusterv1.Bootstrap{ConfigRef: &corev1.ObjectReference{}},
						},
					},
				},
			}

			oldMP := &expv1.MachinePool{
				Spec: expv1.MachinePoolSpec{
					ClusterName: tt.oldClusterName,
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Bootstrap: clusterv1.Bootstrap{ConfigRef: &corev1.ObjectReference{}},
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

			mp := &expv1.MachinePool{
				Spec: expv1.MachinePoolSpec{
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Bootstrap: clusterv1.Bootstrap{ConfigRef: &corev1.ObjectReference{}},
							Version:   &tt.version,
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
			mp := &expv1.MachinePool{
				Spec: expv1.MachinePoolSpec{
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
