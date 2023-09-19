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
	ctrl "sigs.k8s.io/controller-runtime"

	addonsv1 "sigs.k8s.io/cluster-api/exp/addons/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/webhooks/util"
)

var ctx = ctrl.SetupSignalHandler()

func TestClusterResourcesetDefault(t *testing.T) {
	g := NewWithT(t)
	clusterResourceSet := &addonsv1.ClusterResourceSet{}
	defaultingValidationCRS := clusterResourceSet.DeepCopy()
	defaultingValidationCRS.Spec.ClusterSelector = metav1.LabelSelector{
		MatchLabels: map[string]string{"foo": "bar"},
	}
	webhook := ClusterResourceSet{}
	t.Run("for ClusterResourceSet", util.CustomDefaultValidateTest(ctx, defaultingValidationCRS, &webhook))
	g.Expect(webhook.Default(ctx, clusterResourceSet)).To(Succeed())

	g.Expect(clusterResourceSet.Spec.Strategy).To(Equal(string(addonsv1.ClusterResourceSetStrategyApplyOnce)))
}

func TestClusterResourceSetLabelSelectorAsSelectorValidation(t *testing.T) {
	tests := []struct {
		name      string
		selectors map[string]string
		expectErr bool
	}{
		{
			name:      "should not return error for valid selector",
			selectors: map[string]string{"foo": "bar"},
			expectErr: false,
		},
		{
			name:      "should return error for invalid selector",
			selectors: map[string]string{"-123-foo": "bar"},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			clusterResourceSet := &addonsv1.ClusterResourceSet{
				Spec: addonsv1.ClusterResourceSetSpec{
					ClusterSelector: metav1.LabelSelector{
						MatchLabels: tt.selectors,
					},
				},
			}
			webhook := ClusterResourceSet{}
			if tt.expectErr {
				warnings, err := webhook.ValidateCreate(ctx, clusterResourceSet)
				g.Expect(err).To(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
				warnings, err = webhook.ValidateUpdate(ctx, clusterResourceSet, clusterResourceSet)
				g.Expect(err).To(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
			} else {
				warnings, err := webhook.ValidateCreate(ctx, clusterResourceSet)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
				warnings, err = webhook.ValidateUpdate(ctx, clusterResourceSet, clusterResourceSet)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
			}
		})
	}
}

func TestClusterResourceSetStrategyImmutable(t *testing.T) {
	tests := []struct {
		name        string
		oldStrategy string
		newStrategy string
		expectErr   bool
	}{
		{
			name:        "when the Strategy has not changed",
			oldStrategy: string(addonsv1.ClusterResourceSetStrategyApplyOnce),
			newStrategy: string(addonsv1.ClusterResourceSetStrategyApplyOnce),
			expectErr:   false,
		},
		{
			name:        "when the Strategy has changed",
			oldStrategy: string(addonsv1.ClusterResourceSetStrategyApplyOnce),
			newStrategy: "",
			expectErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			newClusterResourceSet := &addonsv1.ClusterResourceSet{
				Spec: addonsv1.ClusterResourceSetSpec{
					ClusterSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"test": "test",
						},
					},
					Strategy: tt.newStrategy,
				},
			}

			oldClusterResourceSet := &addonsv1.ClusterResourceSet{
				Spec: addonsv1.ClusterResourceSetSpec{
					ClusterSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"test": "test",
						},
					},
					Strategy: tt.oldStrategy,
				},
			}
			webhook := ClusterResourceSet{}

			warnings, err := webhook.ValidateUpdate(ctx, oldClusterResourceSet, newClusterResourceSet)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(warnings).To(BeEmpty())
		})
	}
}

func TestClusterResourceSetClusterSelectorImmutable(t *testing.T) {
	tests := []struct {
		name               string
		oldClusterSelector map[string]string
		newClusterSelector map[string]string
		expectErr          bool
	}{
		{
			name:               "when the ClusterSelector has not changed",
			oldClusterSelector: map[string]string{"foo": "bar"},
			newClusterSelector: map[string]string{"foo": "bar"},
			expectErr:          false,
		},
		{
			name:               "when the ClusterSelector has changed",
			oldClusterSelector: map[string]string{"foo": "bar"},
			newClusterSelector: map[string]string{"foo": "different"},
			expectErr:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			newClusterResourceSet := &addonsv1.ClusterResourceSet{
				Spec: addonsv1.ClusterResourceSetSpec{
					ClusterSelector: metav1.LabelSelector{
						MatchLabels: tt.newClusterSelector,
					},
				},
			}

			oldClusterResourceSet := &addonsv1.ClusterResourceSet{
				Spec: addonsv1.ClusterResourceSetSpec{
					ClusterSelector: metav1.LabelSelector{
						MatchLabels: tt.oldClusterSelector,
					},
				},
			}
			webhook := ClusterResourceSet{}

			warnings, err := webhook.ValidateUpdate(ctx, oldClusterResourceSet, newClusterResourceSet)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(warnings).To(BeEmpty())
		})
	}
}

func TestClusterResourceSetSelectorNotEmptyValidation(t *testing.T) {
	g := NewWithT(t)
	clusterResourceSet := &addonsv1.ClusterResourceSet{}
	webhook := ClusterResourceSet{}
	err := webhook.validate(nil, clusterResourceSet)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("selector must not be empty"))
}
