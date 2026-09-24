/*
Copyright 2025 The Kubernetes Authors.

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

package machinepool

import (
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestHasMachinePoolMachines(t *testing.T) {
	tests := []struct {
		name      string
		infraPool *unstructured.Unstructured
		want      bool
		wantErr   bool
	}{
		{
			name:    "returns error when infraMachinePool is nil",
			want:    false,
			wantErr: true,
		},
		{
			name: "returns false when infrastructureMachineKind is absent",
			infraPool: &unstructured.Unstructured{Object: map[string]interface{}{
				"status": map[string]interface{}{},
			}},
			want:    false,
			wantErr: false,
		},
		{
			name: "returns false when infrastructureMachineKind is empty string",
			infraPool: &unstructured.Unstructured{Object: map[string]interface{}{
				"status": map[string]interface{}{
					"infrastructureMachineKind": "",
				},
			}},
			want:    false,
			wantErr: false,
		},
		{
			name: "returns true when infrastructureMachineKind is set",
			infraPool: &unstructured.Unstructured{Object: map[string]interface{}{
				"status": map[string]interface{}{
					"infrastructureMachineKind": "DockerMachine",
				},
			}},
			want:    true,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			s := &scope{infraMachinePool: tt.infraPool}
			got, err := s.hasMachinePoolMachines()

			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(got).To(Equal(tt.want))
		})
	}
}
