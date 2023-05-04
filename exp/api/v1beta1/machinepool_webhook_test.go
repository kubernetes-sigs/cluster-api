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
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"k8s.io/utils/pointer"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
	utildefaulting "sigs.k8s.io/cluster-api/util/defaulting"
)

func TestMachinePoolDefault(t *testing.T) {
	// NOTE: MachinePool feature flag is disabled by default, thus preventing to create or update MachinePool.
	// Enabling the feature flag temporarily for this test.
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.MachinePool, true)()

	g := NewWithT(t)

	m := &MachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "foobar",
		},
		Spec: MachinePoolSpec{
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{ConfigRef: &corev1.ObjectReference{}},
					Version:   pointer.String("1.20.0"),
				},
			},
		},
	}
	t.Run("for MachinePool", utildefaulting.DefaultValidateTest(m))
	m.Default()

	g.Expect(m.Labels[clusterv1.ClusterNameLabel]).To(Equal(m.Spec.ClusterName))
	g.Expect(m.Spec.Replicas).To(Equal(pointer.Int32(1)))
	g.Expect(m.Spec.MinReadySeconds).To(Equal(pointer.Int32(0)))
	g.Expect(m.Spec.Template.Spec.Bootstrap.ConfigRef.Namespace).To(Equal(m.Namespace))
	g.Expect(m.Spec.Template.Spec.InfrastructureRef.Namespace).To(Equal(m.Namespace))
	g.Expect(m.Spec.Template.Spec.Version).To(Equal(pointer.String("v1.20.0")))
}

func TestMachinePoolBootstrapValidation(t *testing.T) {
	// NOTE: MachinePool feature flag is disabled by default, thus preventing to create or update MachinePool.
	// Enabling the feature flag temporarily for this test.
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.MachinePool, true)()
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
			bootstrap: clusterv1.Bootstrap{ConfigRef: nil, DataSecretName: pointer.String("test")},
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
				_, err := m.ValidateCreate()
				g.Expect(err).To(HaveOccurred())
				_, err = m.ValidateUpdate(m)
				g.Expect(err).To(HaveOccurred())
			} else {
				_, err := m.ValidateCreate()
				g.Expect(err).NotTo(HaveOccurred())
				_, err = m.ValidateUpdate(m)
				g.Expect(err).NotTo(HaveOccurred())
			}
		})
	}
}

func TestMachinePoolNamespaceValidation(t *testing.T) {
	// NOTE: MachinePool feature flag is disabled by default, thus preventing to create or update MachinePool.
	// Enabling the feature flag temporarily for this test.
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.MachinePool, true)()
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
				_, err := m.ValidateCreate()
				g.Expect(err).To(HaveOccurred())
				_, err = m.ValidateUpdate(m)
				g.Expect(err).To(HaveOccurred())
			} else {
				_, err := m.ValidateCreate()
				g.Expect(err).NotTo(HaveOccurred())
				_, err = m.ValidateUpdate(m)
				g.Expect(err).NotTo(HaveOccurred())
			}
		})
	}
}

func TestMachinePoolClusterNameImmutable(t *testing.T) {
	// NOTE: MachinePool feature flag is disabled by default, thus preventing to create or update MachinePool.
	// Enabling the feature flag temporarily for this test.
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.MachinePool, true)()
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

			_, err := newMP.ValidateUpdate(oldMP)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
		})
	}
}

func TestMachinePoolVersionValidation(t *testing.T) {
	// NOTE: MachinePool feature flag is disabled by default, thus preventing to create or update MachinePool.
	// Enabling the feature flag temporarily for this test.
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.MachinePool, true)()
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			m := &MachinePool{
				Spec: MachinePoolSpec{
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Bootstrap: clusterv1.Bootstrap{ConfigRef: &corev1.ObjectReference{}},
							Version:   &tt.version,
						},
					},
				},
			}

			if tt.expectErr {
				_, err := m.ValidateCreate()
				g.Expect(err).To(HaveOccurred())
				_, err = m.ValidateUpdate(m)
				g.Expect(err).To(HaveOccurred())
			} else {
				_, err := m.ValidateCreate()
				g.Expect(err).NotTo(HaveOccurred())
				_, err = m.ValidateUpdate(m)
				g.Expect(err).NotTo(HaveOccurred())
			}
		})
	}
}
