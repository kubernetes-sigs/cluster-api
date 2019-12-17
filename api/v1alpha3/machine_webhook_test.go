/*
Copyright 2019 The Kubernetes Authors.

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

package v1alpha3

import (
	"testing"

	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

func TestMachineDefault(t *testing.T) {
	g := gomega.NewWithT(t)

	m := &Machine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "foobar",
		},
		Spec: MachineSpec{
			Bootstrap: Bootstrap{ConfigRef: &corev1.ObjectReference{}},
		},
	}

	m.Default()

	g.Expect(m.Spec.Bootstrap.ConfigRef.Namespace).To(gomega.Equal(m.Namespace))
	g.Expect(m.Spec.InfrastructureRef.Namespace).To(gomega.Equal(m.Namespace))
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
			bootstrap: Bootstrap{ConfigRef: nil, DataSecretName: pointer.StringPtr("test")},
			expectErr: false,
		},
		{
			name:      "should not return error if config ref is set",
			bootstrap: Bootstrap{ConfigRef: &corev1.ObjectReference{}, Data: nil},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomega.NewWithT(t)
			m := &Machine{
				Spec: MachineSpec{Bootstrap: tt.bootstrap},
			}
			if tt.expectErr {
				err := m.ValidateCreate()
				g.Expect(err).To(gomega.HaveOccurred())
				err = m.ValidateUpdate(nil)
				g.Expect(err).To(gomega.HaveOccurred())
			} else {
				g.Expect(m.ValidateCreate()).To(gomega.Succeed())
				g.Expect(m.ValidateUpdate(nil)).To(gomega.Succeed())
			}
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
			g := gomega.NewWithT(t)

			m := &Machine{
				ObjectMeta: metav1.ObjectMeta{Namespace: tt.namespace},
				Spec:       MachineSpec{Bootstrap: tt.bootstrap, InfrastructureRef: tt.infraRef},
			}

			if tt.expectErr {
				err := m.ValidateCreate()
				g.Expect(err).To(gomega.HaveOccurred())
				err = m.ValidateUpdate(nil)
				g.Expect(err).To(gomega.HaveOccurred())
			} else {
				err := m.ValidateCreate()
				g.Expect(err).To(gomega.Succeed())
				err = m.ValidateUpdate(nil)
				g.Expect(err).To(gomega.Succeed())
			}
		})
	}
}
