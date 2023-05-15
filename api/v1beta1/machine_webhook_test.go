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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func TestMachineDefault(t *testing.T) {
	g := NewWithT(t)

	m := &Machine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "foobar",
		},
		Spec: MachineSpec{
			Bootstrap: Bootstrap{ConfigRef: &corev1.ObjectReference{}},
			Version:   pointer.String("1.17.5"),
		},
	}
	scheme, err := SchemeBuilder.Build()
	g.Expect(err).ToNot(HaveOccurred())
	validator := MachineValidator(scheme)
	t.Run("for Machine", defaultDefaulterTestCustomValidator(m, validator))
	m.Default()

	g.Expect(m.Labels[ClusterNameLabel]).To(Equal(m.Spec.ClusterName))
	g.Expect(m.Spec.Bootstrap.ConfigRef.Namespace).To(Equal(m.Namespace))
	g.Expect(m.Spec.InfrastructureRef.Namespace).To(Equal(m.Namespace))
	g.Expect(*m.Spec.Version).To(Equal("v1.17.5"))
	g.Expect(m.Spec.NodeDeletionTimeout.Duration).To(Equal(defaultNodeDeletionTimeout))
}

func TestMachineBootstrapValidation(t *testing.T) {
	tests := []struct {
		name      string
		bootstrap Bootstrap
		expectErr bool
	}{
		{
			name:      "should return error if configref and data are nil",
			bootstrap: Bootstrap{ConfigRef: nil, DataSecretName: nil},
			expectErr: true,
		},
		{
			name:      "should not return error if dataSecretName is set",
			bootstrap: Bootstrap{ConfigRef: nil, DataSecretName: pointer.String("test")},
			expectErr: false,
		},
		{
			name:      "should not return error if config ref is set",
			bootstrap: Bootstrap{ConfigRef: &corev1.ObjectReference{}, DataSecretName: nil},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			scheme, err := SchemeBuilder.Build()
			g.Expect(err).ToNot(HaveOccurred())
			validator := MachineValidator(scheme)

			ctx := admission.NewContextWithRequest(context.Background(), admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
				},
			})

			m := &Machine{
				Spec: MachineSpec{Bootstrap: tt.bootstrap},
			}
			if tt.expectErr {
				_, err := validator.ValidateCreate(ctx, m)
				g.Expect(err).To(HaveOccurred())
				_, err = validator.ValidateUpdate(ctx, m, m)
				g.Expect(err).To(HaveOccurred())
			} else {
				_, err := validator.ValidateCreate(ctx, m)
				g.Expect(err).ToNot(HaveOccurred())
				_, err = validator.ValidateUpdate(ctx, m, m)
				g.Expect(err).ToNot(HaveOccurred())
			}
			// 	g.Expect(validator.ValidateCreate(ctx, m)).NotTo(Succeed())
			// 	g.Expect(validator.ValidateUpdate(ctx, m, m)).NotTo(Succeed())
			// } else {
			// 	g.Expect(validator.ValidateCreate(ctx, m)).To(Succeed())
			// 	g.Expect(validator.ValidateUpdate(ctx, m, m)).To(Succeed())
			// }
		})
	}
}

func TestMachineNamespaceValidation(t *testing.T) {
	tests := []struct {
		name      string
		expectErr bool
		bootstrap Bootstrap
		infraRef  corev1.ObjectReference
		namespace string
	}{
		{
			name:      "should succeed if all namespaces match",
			expectErr: false,
			namespace: "foobar",
			bootstrap: Bootstrap{ConfigRef: &corev1.ObjectReference{Namespace: "foobar"}},
			infraRef:  corev1.ObjectReference{Namespace: "foobar"},
		},
		{
			name:      "should return error if namespace and bootstrap namespace don't match",
			expectErr: true,
			namespace: "foobar",
			bootstrap: Bootstrap{ConfigRef: &corev1.ObjectReference{Namespace: "foobar123"}},
			infraRef:  corev1.ObjectReference{Namespace: "foobar"},
		},
		{
			name:      "should return error if namespace and infrastructure ref namespace don't match",
			expectErr: true,
			namespace: "foobar",
			bootstrap: Bootstrap{ConfigRef: &corev1.ObjectReference{Namespace: "foobar"}},
			infraRef:  corev1.ObjectReference{Namespace: "foobar123"},
		},
		{
			name:      "should return error if no namespaces match",
			expectErr: true,
			namespace: "foobar1",
			bootstrap: Bootstrap{ConfigRef: &corev1.ObjectReference{Namespace: "foobar2"}},
			infraRef:  corev1.ObjectReference{Namespace: "foobar3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			scheme, err := SchemeBuilder.Build()
			g.Expect(err).ToNot(HaveOccurred())
			validator := MachineValidator(scheme)

			ctx := admission.NewContextWithRequest(context.Background(), admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
				},
			})

			m := &Machine{
				ObjectMeta: metav1.ObjectMeta{Namespace: tt.namespace},
				Spec:       MachineSpec{Bootstrap: tt.bootstrap, InfrastructureRef: tt.infraRef},
			}

			if tt.expectErr {
				_, err := validator.ValidateCreate(ctx, m)
				g.Expect(err).To(HaveOccurred())
				_, err = validator.ValidateUpdate(ctx, m, m)
				g.Expect(err).To(HaveOccurred())
			} else {
				_, err := validator.ValidateCreate(ctx, m)
				g.Expect(err).ToNot(HaveOccurred())
				_, err = validator.ValidateUpdate(ctx, m, m)
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func TestMachineClusterNameImmutable(t *testing.T) {
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
			scheme, err := SchemeBuilder.Build()
			g.Expect(err).ToNot(HaveOccurred())
			validator := MachineValidator(scheme)

			ctx := admission.NewContextWithRequest(context.Background(), admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
				},
			})

			newMachine := &Machine{
				Spec: MachineSpec{
					ClusterName: tt.newClusterName,
					Bootstrap:   Bootstrap{ConfigRef: &corev1.ObjectReference{}},
				},
			}
			oldMachine := &Machine{
				Spec: MachineSpec{
					ClusterName: tt.oldClusterName,
					Bootstrap:   Bootstrap{ConfigRef: &corev1.ObjectReference{}},
				},
			}

			_, err = validator.ValidateUpdate(ctx, oldMachine, newMachine)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
		})
	}
}

func TestMachineVersionValidation(t *testing.T) {
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
			scheme, err := SchemeBuilder.Build()
			g.Expect(err).ToNot(HaveOccurred())
			validator := MachineValidator(scheme)

			ctx := admission.NewContextWithRequest(context.Background(), admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
				},
			})

			m := &Machine{
				Spec: MachineSpec{
					Version:   &tt.version,
					Bootstrap: Bootstrap{ConfigRef: nil, DataSecretName: pointer.String("test")},
				},
			}

			if tt.expectErr {
				_, err := validator.ValidateCreate(ctx, m)
				g.Expect(err).To(HaveOccurred())
				_, err = validator.ValidateUpdate(ctx, m, m)
				g.Expect(err).To(HaveOccurred())
			} else {
				_, err := validator.ValidateCreate(ctx, m)
				g.Expect(err).ToNot(HaveOccurred())
				_, err = validator.ValidateUpdate(ctx, m, m)
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

// defaultDefaulterTestCustomVAlidator returns a new testing function to be used in tests to
// make sure defaulting webhooks also pass validation tests on create, update and delete.
// Note: The difference to util/defaulting.DefaultValidateTest is that this function takes an additional
// CustomValidator as the validation is not implemented on the object directly.
func defaultDefaulterTestCustomValidator(object admission.Defaulter, customValidator admission.CustomValidator) func(*testing.T) {
	return func(t *testing.T) {
		t.Helper()

		createCopy := object.DeepCopyObject().(admission.Defaulter)
		updateCopy := object.DeepCopyObject().(admission.Defaulter)
		deleteCopy := object.DeepCopyObject().(admission.Defaulter)
		defaultingUpdateCopy := updateCopy.DeepCopyObject().(admission.Defaulter)

		ctx := admission.NewContextWithRequest(context.Background(), admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
			},
		})

		t.Run("validate-on-create", func(t *testing.T) {
			g := NewWithT(t)
			createCopy.Default()
			g.Expect(customValidator.ValidateCreate(ctx, createCopy)).To(Succeed())
		})
		t.Run("validate-on-update", func(t *testing.T) {
			g := NewWithT(t)
			defaultingUpdateCopy.Default()
			updateCopy.Default()
			g.Expect(customValidator.ValidateUpdate(ctx, defaultingUpdateCopy, updateCopy)).To(Succeed())
		})
		t.Run("validate-on-delete", func(t *testing.T) {
			g := NewWithT(t)
			deleteCopy.Default()
			g.Expect(customValidator.ValidateDelete(ctx, deleteCopy)).To(Succeed())
		})
	}
}
