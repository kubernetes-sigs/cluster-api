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
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

func TestMachineDeploymentDefault(t *testing.T) {
	g := NewWithT(t)
	md := &MachineDeployment{}

	md.Default()

	g.Expect(md.Spec.Replicas).To(Equal(pointer.Int32Ptr(1)))
	g.Expect(md.Spec.MinReadySeconds).To(Equal(pointer.Int32Ptr(0)))
	g.Expect(md.Spec.RevisionHistoryLimit).To(Equal(pointer.Int32Ptr(1)))
	g.Expect(md.Spec.ProgressDeadlineSeconds).To(Equal(pointer.Int32Ptr(600)))
	g.Expect(md.Spec.Strategy).ToNot(BeNil())
	g.Expect(md.Spec.Strategy.Type).To(Equal(RollingUpdateMachineDeploymentStrategyType))
	g.Expect(md.Spec.Strategy.RollingUpdate).ToNot(BeNil())
	g.Expect(md.Spec.Strategy.RollingUpdate.MaxSurge.IntValue()).To(Equal(1))
	g.Expect(md.Spec.Strategy.RollingUpdate.MaxUnavailable.IntValue()).To(Equal(0))
}

func TestMachineDeploymentValidation(t *testing.T) {
	tests := []struct {
		name      string
		selectors map[string]string
		labels    map[string]string
		expectErr bool
	}{
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			md := &MachineDeployment{
				Spec: MachineDeploymentSpec{
					Selector: v1.LabelSelector{
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
				g.Expect(md.ValidateUpdate(nil)).NotTo(Succeed())
			} else {
				g.Expect(md.ValidateCreate()).To(Succeed())
				g.Expect(md.ValidateUpdate(nil)).To(Succeed())
			}
		})
	}
}
