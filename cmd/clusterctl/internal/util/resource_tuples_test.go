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

package util

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestResourceTypeAndNameArgs(t *testing.T) {

	tests := []struct {
		name    string
		args    []string
		want    []ResourceTuple
		wantErr bool
	}{
		{
			name: "valid",
			args: []string{"machinedeployment/foo"},
			want: []ResourceTuple{
				{
					Resource: "machinedeployment",
					Name:     "foo",
				},
			},
			wantErr: false,
		},
		{
			name: "valid multiple with name indirection",
			args: []string{"machinedeployment/foo", "machinedeployment/bar"},
			want: []ResourceTuple{
				{
					Resource: "machinedeployment",
					Name:     "foo",
				},
				{
					Resource: "machinedeployment",
					Name:     "bar",
				},
			},
			wantErr: false,
		},
		{
			name:    "no name but with slash",
			args:    []string{",machinedeployment/"},
			wantErr: true,
		},
		{
			name:    "no name w/o slash",
			args:    []string{",machinedeployment"},
			wantErr: true,
		},
		{
			name:    "trailing slash",
			args:    []string{",foo/"},
			wantErr: true,
		},
		{
			name:    "leading slash",
			args:    []string{"/foo"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			got, err := ResourceTypeAndNameArgs(tt.args...)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(len(got)).To(Equal(len(tt.want)))
			for i := range got {
				g.Expect(got[i].Resource).To(Equal(tt.want[i].Resource))
				g.Expect(got[i].Name).To(Equal(tt.want[i].Name))
			}
		})
	}
}
