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
)

func TestMachineBootstrapValidation(t *testing.T) {
	data := "some bootstrap data"
	tests := []struct {
		name      string
		bootstrap Bootstrap
		expectErr bool
	}{
		{
			name:      "should return error if configref and data are nil",
			bootstrap: Bootstrap{ConfigRef: nil, Data: nil},
			expectErr: true,
		},
		{
			name:      "should not return error if data is set",
			bootstrap: Bootstrap{ConfigRef: nil, Data: &data},
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
