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

package secret

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestParseSecretName(t *testing.T) {
	type args struct {
		name string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		want1   Purpose
		wantErr bool
	}{
		{
			name: "A secret for the test cluster",
			args: args{
				name: "test-kubeconfig",
			},
			want:    "test",
			want1:   Kubeconfig,
			wantErr: false,
		},
		{
			name: "A secret for the test-capa cluster (cluster name with - in the middle)",
			args: args{
				name: "test-capa-ca",
			},
			want:    "test-capa",
			want1:   ClusterCA,
			wantErr: false,
		},
		{
			name: "Not a Cluster API secret",
			args: args{
				name: "foo",
			},
			wantErr: true,
		},
		{
			name: "Not a Cluster API secret (secret name with - in the middle)",
			args: args{
				name: "foo-bar",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			got, got1, err := ParseSecretName(tt.args.name)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(got).To(Equal(tt.want))
			g.Expect(got1).To(Equal(tt.want1))
		})
	}
}
