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
	"context"
	"testing"

	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func TestMachineDeploymentDefault(t *testing.T) {
	g := NewWithT(t)
	md := &MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-md",
		},
		Spec: MachineDeploymentSpec{
			ClusterName: "test-cluster",
			Template: MachineTemplateSpec{
				Spec: MachineSpec{
					Version: pointer.String("1.19.10"),
				},
			},
		},
	}

	scheme, err := SchemeBuilder.Build()
	g.Expect(err).ToNot(HaveOccurred())
	defaulter := MachineDeploymentDefaulter(scheme)

	t.Run("for MachineDeployment", defaultValidateTestCustomDefaulter(md, defaulter))

	ctx := admission.NewContextWithRequest(context.Background(), admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Create,
		},
	})
	g.Expect(defaulter.Default(ctx, md)).To(Succeed())

	g.Expect(md.Labels[ClusterNameLabel]).To(Equal(md.Spec.ClusterName))

	g.Expect(md.Spec.MinReadySeconds).To(Equal(pointer.Int32(0)))
	g.Expect(md.Spec.Replicas).To(Equal(pointer.Int32(1)))
	g.Expect(md.Spec.RevisionHistoryLimit).To(Equal(pointer.Int32(1)))
	g.Expect(md.Spec.ProgressDeadlineSeconds).To(Equal(pointer.Int32(600)))
	g.Expect(md.Spec.Strategy).ToNot(BeNil())

	g.Expect(md.Spec.Selector.MatchLabels).To(HaveKeyWithValue(MachineDeploymentNameLabel, "test-md"))
	g.Expect(md.Spec.Template.Labels).To(HaveKeyWithValue(MachineDeploymentNameLabel, "test-md"))
	g.Expect(md.Spec.Selector.MatchLabels).To(HaveKeyWithValue(ClusterNameLabel, "test-cluster"))
	g.Expect(md.Spec.Template.Labels).To(HaveKeyWithValue(ClusterNameLabel, "test-cluster"))

	g.Expect(md.Spec.Strategy.Type).To(Equal(RollingUpdateMachineDeploymentStrategyType))
	g.Expect(md.Spec.Strategy.RollingUpdate).ToNot(BeNil())
	g.Expect(md.Spec.Strategy.RollingUpdate.MaxSurge.IntValue()).To(Equal(1))
	g.Expect(md.Spec.Strategy.RollingUpdate.MaxUnavailable.IntValue()).To(Equal(0))

	g.Expect(*md.Spec.Template.Spec.Version).To(Equal("v1.19.10"))
}

func TestCalculateMachineDeploymentReplicas(t *testing.T) {
	tests := []struct {
		name             string
		newMD            *MachineDeployment
		oldMD            *MachineDeployment
		expectedReplicas int32
		expectErr        bool
	}{
		{
			name: "if new MD has replicas set, keep that value",
			newMD: &MachineDeployment{
				Spec: MachineDeploymentSpec{
					Replicas: pointer.Int32(5),
				},
			},
			expectedReplicas: 5,
		},
		{
			name:             "if new MD does not have replicas set and no annotations, use 1",
			newMD:            &MachineDeployment{},
			expectedReplicas: 1,
		},
		{
			name: "if new MD only has min size annotation, fallback to 1",
			newMD: &MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						autoscalerMinSize: "3",
					},
				},
			},
			expectedReplicas: 1,
		},
		{
			name: "if new MD only has max size annotation, fallback to 1",
			newMD: &MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						autoscalerMaxSize: "7",
					},
				},
			},
			expectedReplicas: 1,
		},
		{
			name: "if new MD has min and max size annotation and min size is invalid, fail",
			newMD: &MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						autoscalerMinSize: "abc",
						autoscalerMaxSize: "7",
					},
				},
			},
			expectErr: true,
		},
		{
			name: "if new MD has min and max size annotation and max size is invalid, fail",
			newMD: &MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						autoscalerMinSize: "3",
						autoscalerMaxSize: "abc",
					},
				},
			},
			expectErr: true,
		},
		{
			name: "if new MD has min and max size annotation and new MD is a new MD, use min size",
			newMD: &MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						autoscalerMinSize: "3",
						autoscalerMaxSize: "7",
					},
				},
			},
			expectedReplicas: 3,
		},
		{
			name: "if new MD has min and max size annotation and old MD doesn't have replicas set, use min size",
			newMD: &MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						autoscalerMinSize: "3",
						autoscalerMaxSize: "7",
					},
				},
			},
			oldMD:            &MachineDeployment{},
			expectedReplicas: 3,
		},
		{
			name: "if new MD has min and max size annotation and old MD replicas is below min size, use min size",
			newMD: &MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						autoscalerMinSize: "3",
						autoscalerMaxSize: "7",
					},
				},
			},
			oldMD: &MachineDeployment{
				Spec: MachineDeploymentSpec{
					Replicas: pointer.Int32(1),
				},
			},
			expectedReplicas: 3,
		},
		{
			name: "if new MD has min and max size annotation and old MD replicas is above max size, use max size",
			newMD: &MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						autoscalerMinSize: "3",
						autoscalerMaxSize: "7",
					},
				},
			},
			oldMD: &MachineDeployment{
				Spec: MachineDeploymentSpec{
					Replicas: pointer.Int32(15),
				},
			},
			expectedReplicas: 7,
		},
		{
			name: "if new MD has min and max size annotation and old MD replicas is between min and max size, use old MD replicas",
			newMD: &MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						autoscalerMinSize: "3",
						autoscalerMaxSize: "7",
					},
				},
			},
			oldMD: &MachineDeployment{
				Spec: MachineDeploymentSpec{
					Replicas: pointer.Int32(4),
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

	goodMaxSurgePercentage := intstr.FromString("1%")
	goodMaxUnavailablePercentage := intstr.FromString("0%")

	goodMaxSurgeInt := intstr.FromInt(1)
	goodMaxUnavailableInt := intstr.FromInt(0)
	tests := []struct {
		name      string
		md        MachineDeployment
		mdName    string
		selectors map[string]string
		labels    map[string]string
		strategy  MachineDeploymentStrategy
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
			strategy: MachineDeploymentStrategy{
				Type: RollingUpdateMachineDeploymentStrategyType,
				RollingUpdate: &MachineRollingUpdateDeployment{
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
			strategy: MachineDeploymentStrategy{
				Type: RollingUpdateMachineDeploymentStrategyType,
				RollingUpdate: &MachineRollingUpdateDeployment{
					MaxUnavailable: &badMaxUnavailable,
					MaxSurge:       &goodMaxSurgeInt,
				},
			},
			expectErr: true,
		},
		{
			name:      "should not return error for valid int maxSurge and maxUnavailable",
			selectors: map[string]string{"foo": "bar"},
			labels:    map[string]string{"foo": "bar"},
			strategy: MachineDeploymentStrategy{
				Type: RollingUpdateMachineDeploymentStrategyType,
				RollingUpdate: &MachineRollingUpdateDeployment{
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
			strategy: MachineDeploymentStrategy{
				Type: RollingUpdateMachineDeploymentStrategyType,
				RollingUpdate: &MachineRollingUpdateDeployment{
					MaxUnavailable: &goodMaxUnavailablePercentage,
					MaxSurge:       &goodMaxSurgePercentage,
				},
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			md := &MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: tt.mdName,
				},
				Spec: MachineDeploymentSpec{
					Strategy: &tt.strategy,
					Selector: metav1.LabelSelector{
						MatchLabels: tt.selectors,
					},
					Template: MachineTemplateSpec{
						ObjectMeta: ObjectMeta{
							Labels: tt.labels,
						},
					},
				},
			}
			if tt.expectErr {
				g.Expect(md.ValidateCreate()).NotTo(Succeed())
				g.Expect(md.ValidateUpdate(md)).NotTo(Succeed())
			} else {
				g.Expect(md.ValidateCreate()).To(Succeed())
				g.Expect(md.ValidateUpdate(md)).To(Succeed())
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

			md := &MachineDeployment{
				Spec: MachineDeploymentSpec{

					Template: MachineTemplateSpec{
						Spec: MachineSpec{
							Version: pointer.String(tt.version),
						},
					},
				},
			}

			if tt.expectErr {
				g.Expect(md.ValidateCreate()).NotTo(Succeed())
				g.Expect(md.ValidateUpdate(md)).NotTo(Succeed())
			} else {
				g.Expect(md.ValidateCreate()).To(Succeed())
				g.Expect(md.ValidateUpdate(md)).To(Succeed())
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

			newMD := &MachineDeployment{
				Spec: MachineDeploymentSpec{
					ClusterName: tt.newClusterName,
				},
			}

			oldMD := &MachineDeployment{
				Spec: MachineDeploymentSpec{
					ClusterName: tt.oldClusterName,
				},
			}

			if tt.expectErr {
				g.Expect(newMD.ValidateUpdate(oldMD)).NotTo(Succeed())
			} else {
				g.Expect(newMD.ValidateUpdate(oldMD)).To(Succeed())
			}
		})
	}
}

// defaultValidateTestCustomDefaulter returns a new testing function to be used in tests to
// make sure defaulting webhooks also pass validation tests on create, update and delete.
// Note: The difference to util/defaulting.DefaultValidateTest is that this function takes an additional
// CustomDefaulter as the defaulting is not implemented on the object directly.
func defaultValidateTestCustomDefaulter(object admission.Validator, customDefaulter admission.CustomDefaulter) func(*testing.T) {
	return func(t *testing.T) {
		t.Helper()

		createCopy := object.DeepCopyObject().(admission.Validator)
		updateCopy := object.DeepCopyObject().(admission.Validator)
		deleteCopy := object.DeepCopyObject().(admission.Validator)
		defaultingUpdateCopy := updateCopy.DeepCopyObject().(admission.Validator)

		ctx := admission.NewContextWithRequest(context.Background(), admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
			},
		})

		t.Run("validate-on-create", func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(customDefaulter.Default(ctx, createCopy)).To(Succeed())
			g.Expect(createCopy.ValidateCreate()).To(Succeed())
		})
		t.Run("validate-on-update", func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(customDefaulter.Default(ctx, defaultingUpdateCopy)).To(Succeed())
			g.Expect(customDefaulter.Default(ctx, updateCopy)).To(Succeed())
			g.Expect(defaultingUpdateCopy.ValidateUpdate(updateCopy)).To(Succeed())
		})
		t.Run("validate-on-delete", func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(customDefaulter.Default(ctx, deleteCopy)).To(Succeed())
			g.Expect(deleteCopy.ValidateDelete()).To(Succeed())
		})
	}
}
