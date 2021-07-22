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

package v1alpha4

import (
	"testing"

	. "github.com/onsi/gomega"
	utildefaulting "sigs.k8s.io/cluster-api/util/defaulting"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

func TestMachineDefault(t *testing.T) {
	g := NewWithT(t)

	m := &Machine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "foobar",
		},
		Spec: MachineSpec{
			Bootstrap: Bootstrap{ConfigRef: &LocalObjectReference{}},
			Version:   pointer.StringPtr("1.17.5"),
		},
	}
	t.Run("for Machine", utildefaulting.DefaultValidateTest(m))
	m.Default()

	g.Expect(m.Labels[ClusterLabelName]).To(Equal(m.Spec.ClusterName))
	g.Expect(*m.Spec.Version).To(Equal("v1.17.5"))
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
			bootstrap: Bootstrap{ConfigRef: &LocalObjectReference{}, DataSecretName: nil},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			m := &Machine{
				Spec: MachineSpec{Bootstrap: tt.bootstrap},
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

			newMachine := &Machine{
				Spec: MachineSpec{
					ClusterName: tt.newClusterName,
					Bootstrap:   Bootstrap{ConfigRef: &LocalObjectReference{}},
				},
			}
			oldMachine := &Machine{
				Spec: MachineSpec{
					ClusterName: tt.oldClusterName,
					Bootstrap:   Bootstrap{ConfigRef: &LocalObjectReference{}},
				},
			}

			if tt.expectErr {
				g.Expect(newMachine.ValidateUpdate(oldMachine)).NotTo(Succeed())
			} else {
				g.Expect(newMachine.ValidateUpdate(oldMachine)).To(Succeed())
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

			m := &Machine{
				Spec: MachineSpec{
					Version:   &tt.version,
					Bootstrap: Bootstrap{ConfigRef: nil, DataSecretName: pointer.StringPtr("test")},
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
