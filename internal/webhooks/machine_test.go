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
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/internal/webhooks/util"
)

func TestMachineDefault(t *testing.T) {
	g := NewWithT(t)

	m := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "foobar",
		},
		Spec: clusterv1.MachineSpec{
			Bootstrap: clusterv1.Bootstrap{ConfigRef: clusterv1.ContractVersionedObjectReference{
				Name: "bootstrap1",
			}},
			Version: "1.17.5",
		},
	}

	webhook := &Machine{}

	t.Run("for Machine", util.CustomDefaultValidateTest(ctx, m, webhook))
	g.Expect(webhook.Default(ctx, m)).To(Succeed())

	g.Expect(m.Labels[clusterv1.ClusterNameLabel]).To(Equal(m.Spec.ClusterName))
	g.Expect(m.Spec.Version).To(Equal("v1.17.5"))
	g.Expect(*m.Spec.Deletion.NodeDeletionTimeoutSeconds).To(Equal(defaultNodeDeletionTimeoutSeconds))
}

func TestMachineBootstrapValidation(t *testing.T) {
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
			name:      "should not return error if dataSecretName is set",
			bootstrap: clusterv1.Bootstrap{ConfigRef: clusterv1.ContractVersionedObjectReference{}, DataSecretName: ptr.To("")},
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
			m := &clusterv1.Machine{
				Spec: clusterv1.MachineSpec{Bootstrap: tt.bootstrap},
			}
			webhook := &Machine{}

			if tt.expectErr {
				warnings, err := webhook.ValidateCreate(ctx, m)
				g.Expect(err).To(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
				warnings, err = webhook.ValidateUpdate(ctx, m, m)
				g.Expect(err).To(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
			} else {
				warnings, err := webhook.ValidateCreate(ctx, m)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
				warnings, err = webhook.ValidateUpdate(ctx, m, m)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
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

			newMachine := &clusterv1.Machine{
				Spec: clusterv1.MachineSpec{
					ClusterName: tt.newClusterName,
					Bootstrap: clusterv1.Bootstrap{ConfigRef: clusterv1.ContractVersionedObjectReference{
						Name: "bootstrap1",
					}},
				},
			}
			oldMachine := &clusterv1.Machine{
				Spec: clusterv1.MachineSpec{
					ClusterName: tt.oldClusterName,
					Bootstrap: clusterv1.Bootstrap{ConfigRef: clusterv1.ContractVersionedObjectReference{
						Name: "bootstrap1",
					}},
				},
			}

			warnings, err := (&Machine{}).ValidateUpdate(ctx, oldMachine, newMachine)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
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

	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			m := &clusterv1.Machine{
				Spec: clusterv1.MachineSpec{
					Version:   tt.version,
					Bootstrap: clusterv1.Bootstrap{ConfigRef: clusterv1.ContractVersionedObjectReference{}, DataSecretName: ptr.To("test")},
				},
			}
			webhook := &Machine{}

			if tt.expectErr {
				warnings, err := webhook.ValidateCreate(ctx, m)
				g.Expect(err).To(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
				warnings, err = webhook.ValidateUpdate(ctx, m, m)
				g.Expect(err).To(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
			} else {
				warnings, err := webhook.ValidateCreate(ctx, m)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
				warnings, err = webhook.ValidateUpdate(ctx, m, m)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
			}
		})
	}
}
