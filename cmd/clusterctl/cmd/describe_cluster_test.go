/*
Copyright 2026 The Kubernetes Authors.

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

package cmd

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestValidateShowConditions(t *testing.T) {
	tests := []struct {
		name           string
		showConditions string
		wantErr        bool
	}{
		{
			name:           "valid filters",
			showConditions: "Cluster,Machine/my-machine",
		},
		{
			name:           "leading whitespace in a filter",
			showConditions: "Cluster, Machine/my-machine",
			wantErr:        true,
		},
		{
			name:           "trailing whitespace in a filter",
			showConditions: "Cluster,Machine/my-machine ",
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(validateShowConditions(tt.showConditions) != nil).To(Equal(tt.wantErr))
		})
	}
}
