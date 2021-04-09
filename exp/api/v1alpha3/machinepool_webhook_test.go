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
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	utildefaulting "sigs.k8s.io/cluster-api/util/defaulting"
)

func TestMachinePoolDefault(t *testing.T) {
	g := NewWithT(t)

	m := &MachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "foobar",
		},
		Spec: MachinePoolSpec{
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{ConfigRef: &corev1.ObjectReference{}},
				},
			},
		},
	}
	t.Run("for MachinePool", utildefaulting.DefaultValidateTest(m))
	m.Default()

	g.Expect(m.Labels[clusterv1.ClusterLabelName]).To(Equal(m.Spec.ClusterName))
	g.Expect(m.Spec.Replicas).To(Equal(pointer.Int32Ptr(1)))
	g.Expect(m.Spec.MinReadySeconds).To(Equal(pointer.Int32Ptr(0)))
	g.Expect(m.Spec.Template.Spec.Bootstrap.ConfigRef.Namespace).To(Equal(m.Namespace))
	g.Expect(m.Spec.Template.Spec.InfrastructureRef.Namespace).To(Equal(m.Namespace))
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
			bootstrap: clusterv1.Bootstrap{ConfigRef: nil, DataSecretName: pointer.StringPtr("test")},
			expectErr: false,
		},
		{
			name:      "should not return error if config ref is set",
			bootstrap: clusterv1.Bootstrap{ConfigRef: &corev1.ObjectReference{}, Data: nil},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			m := &MachinePool{
				Spec: MachinePoolSpec{
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Bootstrap: tt.bootstrap,
						},
					},
				},
			}
			if tt.expectErr {
				g.Expect(m.ValidateCreate()).NotTo(Succeed())
				g.Expect(m.ValidateUpdate(m)).NotTo(Succeed())
			} else {
				g.Expect(m.ValidateCreate()).To(Succeed())
				g.Expect(m.ValidateUpdate(m)).To(Succeed())
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

			m := &MachinePool{
				ObjectMeta: metav1.ObjectMeta{Namespace: tt.namespace},
				Spec: MachinePoolSpec{
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Bootstrap:         tt.bootstrap,
							InfrastructureRef: tt.infraRef,
						},
					},
				},
			}

			if tt.expectErr {
				g.Expect(m.ValidateCreate()).NotTo(Succeed())
				g.Expect(m.ValidateUpdate(m)).NotTo(Succeed())
			} else {
				g.Expect(m.ValidateCreate()).To(Succeed())
				g.Expect(m.ValidateUpdate(m)).To(Succeed())
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

			newMP := &MachinePool{
				Spec: MachinePoolSpec{
					ClusterName: tt.newClusterName,
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Bootstrap: clusterv1.Bootstrap{ConfigRef: &corev1.ObjectReference{}},
						},
					},
				},
			}

			oldMP := &MachinePool{
				Spec: MachinePoolSpec{
					ClusterName: tt.oldClusterName,
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Bootstrap: clusterv1.Bootstrap{ConfigRef: &corev1.ObjectReference{}},
						},
					},
				},
			}

			if tt.expectErr {
				g.Expect(newMP.ValidateUpdate(oldMP)).NotTo(Succeed())
			} else {
				g.Expect(newMP.ValidateUpdate(oldMP)).To(Succeed())
			}
		})
	}
}
