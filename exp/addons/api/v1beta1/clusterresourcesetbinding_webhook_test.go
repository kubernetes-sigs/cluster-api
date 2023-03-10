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

package v1beta1

import (
	"testing"

	. "github.com/onsi/gomega"
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

			newClusterResourceSetBinding := &ClusterResourceSetBinding{
				Spec: ClusterResourceSetBindingSpec{
					ClusterName: tt.newClusterName,
				},
			}

			oldClusterResourceSetBinding := &ClusterResourceSetBinding{
				Spec: ClusterResourceSetBindingSpec{
					ClusterName: tt.oldClusterName,
				},
			}

			if tt.expectErr {
				g.Expect(newClusterResourceSetBinding.ValidateCreate()).To(Succeed())
				g.Expect(newClusterResourceSetBinding.ValidateUpdate(oldClusterResourceSetBinding)).NotTo(Succeed())
				return
			}
			g.Expect(newClusterResourceSetBinding.ValidateCreate()).To(Succeed())
			g.Expect(newClusterResourceSetBinding.ValidateUpdate(oldClusterResourceSetBinding)).To(Succeed())
		})
	}
}
