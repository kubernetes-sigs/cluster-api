/*
Copyright 2022 The Kubernetes Authors.

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

	addonsv1 "sigs.k8s.io/cluster-api/exp/addons/api/v1beta1"
)

func TestClusterResourceSetBindingClusterNameImmutable(t *testing.T) {
	tests := []struct {
		name           string
		oldClusterName string
		newClusterName string
		expectErr      bool
	}{
		{
			name:           "when ClusterName is empty",
			oldClusterName: "",
			newClusterName: "",
			expectErr:      false,
		},
		{
			name:           "add ClusterName field",
			oldClusterName: "",
			newClusterName: "bar",
			expectErr:      false,
		},
		{
			name:           "when the ClusterName has not changed",
			oldClusterName: "bar",
			newClusterName: "bar",
			expectErr:      false,
		},
		{
			name:           "when the ClusterName has changed",
			oldClusterName: "bar",
			newClusterName: "different",
			expectErr:      true,
		},
		{
			name:           "existing ClusterName field to be set to empty",
			oldClusterName: "bar",
			newClusterName: "",
			expectErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			newClusterResourceSetBinding := &addonsv1.ClusterResourceSetBinding{
				Spec: addonsv1.ClusterResourceSetBindingSpec{
					ClusterName: tt.newClusterName,
				},
			}

			oldClusterResourceSetBinding := &addonsv1.ClusterResourceSetBinding{
				Spec: addonsv1.ClusterResourceSetBindingSpec{
					ClusterName: tt.oldClusterName,
				},
			}
			webhook := ClusterResourceSetBinding{}

			warnings, err := webhook.ValidateCreate(ctx, newClusterResourceSetBinding)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(warnings).To(BeEmpty())
			if tt.expectErr {
				warnings, err = webhook.ValidateUpdate(ctx, oldClusterResourceSetBinding, newClusterResourceSetBinding)
				g.Expect(err).To(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
			} else {
				warnings, err = webhook.ValidateUpdate(ctx, oldClusterResourceSetBinding, newClusterResourceSetBinding)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
			}
		})
	}
}
