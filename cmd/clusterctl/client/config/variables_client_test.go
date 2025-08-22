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

package config

import (
	"testing"

	. "github.com/onsi/gomega"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
)

// Ensures FakeReader implements the Reader interface.
var _ Reader = &test.FakeReader{}

// Ensures the FakeVariableClient implements VariablesClient.
var _ VariablesClient = &test.FakeVariableClient{}

func Test_variables_Get(t *testing.T) {
	reader := test.NewFakeReader().WithVar("foo", "bar")

	type args struct {
		key string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "Returns value if the variable exists",
			args: args{
				key: "foo",
			},
			want:    "bar",
			wantErr: false,
		},
		{
			name: "Returns error if the variable does not exist",
			args: args{
				key: "baz",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			p := &variablesClient{
				reader: reader,
			}
			got, err := p.Get(tt.args.key)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(Equal(tt.want))
		})
	}
}
