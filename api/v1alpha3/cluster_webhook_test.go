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

	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utildefaulting "sigs.k8s.io/cluster-api/util/defaulting"
)

func TestClusterDefault(t *testing.T) {
	g := NewWithT(t)

	c := &Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "fooboo",
		},
		Spec: ClusterSpec{
			InfrastructureRef: &corev1.ObjectReference{},
			ControlPlaneRef:   &corev1.ObjectReference{},
		},
	}

	t.Run("for Cluster", utildefaulting.DefaultValidateTest(c))
	c.Default()

	g.Expect(c.Spec.InfrastructureRef.Namespace).To(Equal(c.Namespace))
	g.Expect(c.Spec.ControlPlaneRef.Namespace).To(Equal(c.Namespace))
}

func TestClusterValidation(t *testing.T) {
	valid := &Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "foo",
		},
		Spec: ClusterSpec{
			ControlPlaneRef: &corev1.ObjectReference{
				Namespace: "foo",
			},
			InfrastructureRef: &corev1.ObjectReference{
				Namespace: "foo",
			},
		},
	}
	invalidInfraNamespace := valid.DeepCopy()
	invalidInfraNamespace.Spec.InfrastructureRef.Namespace = "bar"

	invalidCPNamespace := valid.DeepCopy()
	invalidCPNamespace.Spec.InfrastructureRef.Namespace = "baz"

	tests := []struct {
		name      string
		expectErr bool
		c         *Cluster
	}{
		{
			name:      "should return error when cluster namespace and infrastructure ref namespace mismatch",
			expectErr: true,
			c:         invalidInfraNamespace,
		},
		{
			name:      "should return error when cluster namespace and controlplane ref namespace mismatch",
			expectErr: true,
			c:         invalidCPNamespace,
		},
		{
			name:      "should succeed when namespaces match",
			expectErr: false,
			c:         valid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			if tt.expectErr {
				g.Expect(tt.c.ValidateCreate()).NotTo(Succeed())
				g.Expect(tt.c.ValidateUpdate(nil)).NotTo(Succeed())
			} else {
				g.Expect(tt.c.ValidateCreate()).To(Succeed())
				g.Expect(tt.c.ValidateUpdate(nil)).To(Succeed())
			}
		})
	}
}
