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
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/webhooks/util"
)

func TestMachineDeploymentDefault(t *testing.T) {
	g := NewWithT(t)
	md := &clusterv1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-md",
		},
		Spec: clusterv1.MachineDeploymentSpec{
			ClusterName: "test-cluster",
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					Version: ptr.To("1.19.10"),
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())
	webhook := &MachineDeployment{
		decoder: admission.NewDecoder(scheme),
	}

	reqCtx := admission.NewContextWithRequest(ctx, admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Create,
		},
	})
	t.Run("for MachineDeployment", util.CustomDefaultValidateTest(reqCtx, md, webhook))

	g.Expect(webhook.Default(reqCtx, md)).To(Succeed())

	g.Expect(md.Labels[clusterv1.ClusterNameLabel]).To(Equal(md.Spec.ClusterName))

	g.Expect(md.Spec.MinReadySeconds).To(Equal(ptr.To[int32](0)))
	g.Expect(md.Spec.Replicas).To(Equal(ptr.To[int32](1)))
	g.Expect(md.Spec.RevisionHistoryLimit).To(Equal(ptr.To[int32](1)))
	g.Expect(md.Spec.ProgressDeadlineSeconds).To(Equal(ptr.To[int32](600)))
	g.Expect(md.Spec.Strategy).ToNot(BeNil())

	g.Expect(md.Spec.Selector.MatchLabels).To(HaveKeyWithValue(clusterv1.MachineDeploymentNameLabel, "test-md"))
	g.Expect(md.Spec.Template.Labels).To(HaveKeyWithValue(clusterv1.MachineDeploymentNameLabel, "test-md"))
	g.Expect(md.Spec.Selector.MatchLabels).To(HaveKeyWithValue(clusterv1.ClusterNameLabel, "test-cluster"))
	g.Expect(md.Spec.Template.Labels).To(HaveKeyWithValue(clusterv1.ClusterNameLabel, "test-cluster"))

	g.Expect(md.Spec.Strategy.Type).To(Equal(clusterv1.RollingUpdateMachineDeploymentStrategyType))
	g.Expect(md.Spec.Strategy.RollingUpdate).ToNot(BeNil())
	g.Expect(md.Spec.Strategy.RollingUpdate.MaxSurge.IntValue()).To(Equal(1))
	g.Expect(md.Spec.Strategy.RollingUpdate.MaxUnavailable.IntValue()).To(Equal(0))

	g.Expect(*md.Spec.Template.Spec.Version).To(Equal("v1.19.10"))
}

func TestMachineDeploymentReferenceDefault(t *testing.T) {
	g := NewWithT(t)
	md := &clusterv1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-md",
		},
		Spec: clusterv1.MachineDeploymentSpec{
			ClusterName: "test-cluster",
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					Version: ptr.To("1.19.10"),
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: &corev1.ObjectReference{},
					},
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())
	webhook := &MachineDeployment{
		decoder: admission.NewDecoder(scheme),
	}

	reqCtx := admission.NewContextWithRequest(ctx, admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Create,
		},
	})

	t.Run("for MachineDeployment", util.CustomDefaultValidateTest(reqCtx, md, webhook))

	g.Expect(webhook.Default(reqCtx, md)).To(Succeed())

	g.Expect(md.Spec.Template.Spec.InfrastructureRef.Namespace).To(Equal(md.Namespace))
	g.Expect(md.Spec.Template.Spec.Bootstrap.ConfigRef.Namespace).To(Equal(md.Namespace))
}

func TestCalculateMachineDeploymentReplicas(t *testing.T) {
	tests := []struct {
		name             string
		newMD            *clusterv1.MachineDeployment
		oldMD            *clusterv1.MachineDeployment
		expectedReplicas int32
		expectErr        bool
	}{
		{
			name: "if new MD has replicas set, keep that value",
			newMD: &clusterv1.MachineDeployment{
				Spec: clusterv1.MachineDeploymentSpec{
					Replicas: ptr.To[int32](5),
				},
			},
			expectedReplicas: 5,
		},
		{
			name:             "if new MD does not have replicas set and no annotations, use 1",
			newMD:            &clusterv1.MachineDeployment{},
			expectedReplicas: 1,
		},
		{
			name: "if new MD only has min size annotation, fallback to 1",
			newMD: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						clusterv1.AutoscalerMinSizeAnnotation: "3",
					},
				},
			},
			expectedReplicas: 1,
		},
		{
			name: "if new MD only has max size annotation, fallback to 1",
			newMD: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						clusterv1.AutoscalerMaxSizeAnnotation: "7",
					},
				},
			},
			expectedReplicas: 1,
		},
		{
			name: "if new MD has min and max size annotation and min size is invalid, fail",
			newMD: &clusterv1.MachineDeployment{
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
			name: "if new MD has min and max size annotation and max size is invalid, fail",
			newMD: &clusterv1.MachineDeployment{
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
			name: "if new MD has min and max size annotation and new MD is a new MD, use min size",
			newMD: &clusterv1.MachineDeployment{
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
			name: "if new MD has min and max size annotation and old MD doesn't have replicas set, use min size",
			newMD: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						clusterv1.AutoscalerMinSizeAnnotation: "3",
						clusterv1.AutoscalerMaxSizeAnnotation: "7",
					},
				},
			},
			oldMD:            &clusterv1.MachineDeployment{},
			expectedReplicas: 3,
		},
		{
			name: "if new MD has min and max size annotation and old MD replicas is below min size, use min size",
			newMD: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						clusterv1.AutoscalerMinSizeAnnotation: "3",
						clusterv1.AutoscalerMaxSizeAnnotation: "7",
					},
				},
			},
			oldMD: &clusterv1.MachineDeployment{
				Spec: clusterv1.MachineDeploymentSpec{
					Replicas: ptr.To[int32](1),
				},
			},
			expectedReplicas: 3,
		},
		{
			name: "if new MD has min and max size annotation and old MD replicas is above max size, use max size",
			newMD: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						clusterv1.AutoscalerMinSizeAnnotation: "3",
						clusterv1.AutoscalerMaxSizeAnnotation: "7",
					},
				},
			},
			oldMD: &clusterv1.MachineDeployment{
				Spec: clusterv1.MachineDeploymentSpec{
					Replicas: ptr.To[int32](15),
				},
			},
			expectedReplicas: 7,
		},
		{
			name: "if new MD has min and max size annotation and old MD replicas is between min and max size, use old MD replicas",
			newMD: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						clusterv1.AutoscalerMinSizeAnnotation: "3",
						clusterv1.AutoscalerMaxSizeAnnotation: "7",
					},
				},
			},
			oldMD: &clusterv1.MachineDeployment{
				Spec: clusterv1.MachineDeploymentSpec{
					Replicas: ptr.To[int32](4),
				},
			},
			expectedReplicas: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			replicas, err := calculateMachineDeploymentReplicas(context.Background(), tt.oldMD, tt.newMD, false)

			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(replicas).To(Equal(tt.expectedReplicas))
		})
	}
}

func TestMachineDeploymentValidation(t *testing.T) {
	badMaxSurge := intstr.FromString("1")
	badMaxUnavailable := intstr.FromString("0")
	badMaxInFlight := intstr.FromString("1")

	goodMaxSurgePercentage := intstr.FromString("1%")
	goodMaxUnavailablePercentage := intstr.FromString("0%")
	goodMaxInFlightPercentage := intstr.FromString("20%")

	goodMaxSurgeInt := intstr.FromInt(1)
	goodMaxUnavailableInt := intstr.FromInt(0)
	goodMaxInFlightInt := intstr.FromInt(5)
	tests := []struct {
		name      string
		md        *clusterv1.MachineDeployment
		mdName    string
		selectors map[string]string
		labels    map[string]string
		strategy  clusterv1.MachineDeploymentStrategy
		expectErr bool
	}{
		{
			name:      "pass with name of under 63 characters",
			mdName:    "short-name",
			expectErr: false,
		},
		{
			name:      "pass with _, -, . characters in name",
			mdName:    "thisNameContains.A_Non-Alphanumeric",
			expectErr: false,
		},
		{
			name:      "error with name of more than 63 characters",
			mdName:    "thisNameIsReallyMuchLongerThanTheMaximumLengthOfSixtyThreeCharacters",
			expectErr: true,
		},
		{
			name:      "error when name starts with NonAlphanumeric character",
			mdName:    "-thisNameStartsWithANonAlphanumeric",
			expectErr: true,
		},
		{
			name:      "error when name ends with NonAlphanumeric character",
			mdName:    "thisNameEndsWithANonAlphanumeric.",
			expectErr: true,
		},
		{
			name:      "error when name contains invalid NonAlphanumeric character",
			mdName:    "thisNameContainsInvalid!@NonAlphanumerics",
			expectErr: true,
		},
		{
			name:      "should return error on mismatch",
			selectors: map[string]string{"foo": "bar"},
			labels:    map[string]string{"foo": "baz"},
			expectErr: true,
		},
		{
			name:      "should return error on missing labels",
			selectors: map[string]string{"foo": "bar"},
			labels:    map[string]string{"": ""},
			expectErr: true,
		},
		{
			name:      "should return error if all selectors don't match",
			selectors: map[string]string{"foo": "bar", "hello": "world"},
			labels:    map[string]string{"foo": "bar"},
			expectErr: true,
		},
		{
			name:      "should not return error on match",
			selectors: map[string]string{"foo": "bar"},
			labels:    map[string]string{"foo": "bar"},
			expectErr: false,
		},
		{
			name:      "should return error for invalid selector",
			selectors: map[string]string{"-123-foo": "bar"},
			labels:    map[string]string{"-123-foo": "bar"},
			expectErr: true,
		},
		{
			name:      "should return error for invalid maxSurge",
			selectors: map[string]string{"foo": "bar"},
			labels:    map[string]string{"foo": "bar"},
			strategy: clusterv1.MachineDeploymentStrategy{
				Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
				RollingUpdate: &clusterv1.MachineRollingUpdateDeployment{
					MaxUnavailable: &goodMaxUnavailableInt,
					MaxSurge:       &badMaxSurge,
				},
			},
			expectErr: true,
		},
		{
			name:      "should return error for invalid maxUnavailable",
			selectors: map[string]string{"foo": "bar"},
			labels:    map[string]string{"foo": "bar"},
			strategy: clusterv1.MachineDeploymentStrategy{
				Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
				RollingUpdate: &clusterv1.MachineRollingUpdateDeployment{
					MaxUnavailable: &badMaxUnavailable,
					MaxSurge:       &goodMaxSurgeInt,
				},
			},
			expectErr: true,
		},
		{
			name:      "should return error for invalid remediation maxInFlight",
			selectors: map[string]string{"foo": "bar"},
			labels:    map[string]string{"foo": "bar"},
			strategy: clusterv1.MachineDeploymentStrategy{
				Remediation: &clusterv1.RemediationStrategy{
					MaxInFlight: &badMaxInFlight,
				},
			},
			expectErr: true,
		},
		{
			name:      "should not return error for valid percentage remediation maxInFlight",
			selectors: map[string]string{"foo": "bar"},
			labels:    map[string]string{"foo": "bar"},
			strategy: clusterv1.MachineDeploymentStrategy{
				Remediation: &clusterv1.RemediationStrategy{
					MaxInFlight: &goodMaxInFlightPercentage,
				},
			},
			expectErr: false,
		},
		{
			name:      "should not return error for valid int remediation maxInFlight",
			selectors: map[string]string{"foo": "bar"},
			labels:    map[string]string{"foo": "bar"},
			strategy: clusterv1.MachineDeploymentStrategy{
				Remediation: &clusterv1.RemediationStrategy{
					MaxInFlight: &goodMaxInFlightInt,
				},
			},
			expectErr: false,
		},
		{
			name:      "should not return error for valid int maxSurge and maxUnavailable",
			selectors: map[string]string{"foo": "bar"},
			labels:    map[string]string{"foo": "bar"},
			strategy: clusterv1.MachineDeploymentStrategy{
				Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
				RollingUpdate: &clusterv1.MachineRollingUpdateDeployment{
					MaxUnavailable: &goodMaxUnavailableInt,
					MaxSurge:       &goodMaxSurgeInt,
				},
			},
			expectErr: false,
		},
		{
			name:      "should not return error for valid percentage string maxSurge and maxUnavailable",
			selectors: map[string]string{"foo": "bar"},
			labels:    map[string]string{"foo": "bar"},
			strategy: clusterv1.MachineDeploymentStrategy{
				Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
				RollingUpdate: &clusterv1.MachineRollingUpdateDeployment{
					MaxUnavailable: &goodMaxUnavailablePercentage,
					MaxSurge:       &goodMaxSurgePercentage,
				},
			},
			expectErr: false,
		},
	}

	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			md := &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: tt.mdName,
				},
				Spec: clusterv1.MachineDeploymentSpec{
					Strategy: &tt.strategy,
					Selector: metav1.LabelSelector{
						MatchLabels: tt.selectors,
					},
					Template: clusterv1.MachineTemplateSpec{
						ObjectMeta: clusterv1.ObjectMeta{
							Labels: tt.labels,
						},
					},
				},
			}

			scheme := runtime.NewScheme()
			g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())
			webhook := MachineDeployment{
				decoder: admission.NewDecoder(scheme),
			}

			if tt.expectErr {
				warnings, err := webhook.ValidateCreate(ctx, md)
				g.Expect(err).To(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
				warnings, err = webhook.ValidateUpdate(ctx, md, md)
				g.Expect(err).To(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
			} else {
				warnings, err := webhook.ValidateCreate(ctx, md)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
				warnings, err = webhook.ValidateUpdate(ctx, md, md)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
			}
		})
	}
}

func TestMachineDeploymentVersionValidation(t *testing.T) {
	tests := []struct {
		name      string
		version   string
		expectErr bool
	}{
		{
			name:      "should succeed when given a valid semantic version with prepended 'v'",
			version:   "v1.17.2",
			expectErr: false,
		},
		{
			name:      "should return error when given a valid semantic version without 'v'",
			version:   "1.17.2",
			expectErr: true,
		},
		{
			name:      "should return error when given an invalid semantic version",
			version:   "1",
			expectErr: true,
		},
		{
			name:      "should return error when given an invalid semantic version",
			version:   "v1",
			expectErr: true,
		},
		{
			name:      "should return error when given an invalid semantic version",
			version:   "wrong_version",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			md := &clusterv1.MachineDeployment{
				Spec: clusterv1.MachineDeploymentSpec{
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Version: ptr.To(tt.version),
						},
					},
				},
			}

			scheme := runtime.NewScheme()
			g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())
			webhook := MachineDeployment{
				decoder: admission.NewDecoder(scheme),
			}

			if tt.expectErr {
				warnings, err := webhook.ValidateCreate(ctx, md)
				g.Expect(err).To(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
				warnings, err = webhook.ValidateUpdate(ctx, md, md)
				g.Expect(err).To(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
			} else {
				warnings, err := webhook.ValidateCreate(ctx, md)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
				warnings, err = webhook.ValidateUpdate(ctx, md, md)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
			}
		})
	}
}

func TestMachineDeploymentClusterNameImmutable(t *testing.T) {
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

			newMD := &clusterv1.MachineDeployment{
				Spec: clusterv1.MachineDeploymentSpec{
					ClusterName: tt.newClusterName,
				},
			}

			oldMD := &clusterv1.MachineDeployment{
				Spec: clusterv1.MachineDeploymentSpec{
					ClusterName: tt.oldClusterName,
				},
			}

			scheme := runtime.NewScheme()
			g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())
			webhook := MachineDeployment{
				decoder: admission.NewDecoder(scheme),
			}

			warnings, err := webhook.ValidateUpdate(ctx, oldMD, newMD)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(warnings).To(BeEmpty())
		})
	}
}

func TestMachineDeploymentTemplateMetadataValidation(t *testing.T) {
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
			md := &clusterv1.MachineDeployment{
				Spec: clusterv1.MachineDeploymentSpec{
					Template: clusterv1.MachineTemplateSpec{
						ObjectMeta: clusterv1.ObjectMeta{
							Labels:      tt.labels,
							Annotations: tt.annotations,
						},
					},
				},
			}

			scheme := runtime.NewScheme()
			g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())
			webhook := MachineDeployment{
				decoder: admission.NewDecoder(scheme),
			}

			if tt.expectErr {
				warnings, err := webhook.ValidateCreate(ctx, md)
				g.Expect(err).To(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
				warnings, err = webhook.ValidateUpdate(ctx, md, md)
				g.Expect(err).To(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
			} else {
				warnings, err := webhook.ValidateCreate(ctx, md)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
				warnings, err = webhook.ValidateUpdate(ctx, md, md)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
			}
		})
	}
}
