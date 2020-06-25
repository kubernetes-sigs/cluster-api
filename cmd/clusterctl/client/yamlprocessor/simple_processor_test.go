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
package yamlprocessor

import (
	"testing"

	. "github.com/onsi/gomega"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
)

func TestSimpleProcessor_GetTemplateName(t *testing.T) {
	g := NewWithT(t)
	p := NewSimpleProcessor()
	g.Expect(p.GetTemplateName("some-version", "some-flavor")).To(Equal("cluster-template-some-flavor.yaml"))
	g.Expect(p.GetTemplateName("", "")).To(Equal("cluster-template.yaml"))
}

func TestSimpleProcessor_GetVariables(t *testing.T) {
	type args struct {
		data string
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			name: "variable with different spacing around the name",
			args: args{
				data: "yaml with ${A} ${ B} ${ C} ${ D }",
			},
			want: []string{"A", "B", "C", "D"},
		},
		{
			name: "variables used in many places are grouped",
			args: args{
				data: "yaml with ${A} ${A} ${A}",
			},
			want: []string{"A"},
		},
		{
			name: "variables in multiline texts are processed",
			args: args{
				data: "yaml with ${A}\n${B}\n${C}",
			},
			want: []string{"A", "B", "C"},
		},
		{
			name: "variables are sorted",
			args: args{
				data: "yaml with ${C}\n${B}\n${A}",
			},
			want: []string{"A", "B", "C"},
		},
		{
			name: "variables with regex metacharacters",
			args: args{
				data: "yaml with ${BA$R}\n${FOO}",
			},
			want: []string{"BA$R", "FOO"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			p := NewSimpleProcessor()

			g.Expect(p.GetVariables([]byte(tt.args.data))).To(Equal(tt.want))
		})
	}
}

func TestSimpleProcessor_Process(t *testing.T) {
	type args struct {
		yaml                  []byte
		configVariablesClient config.VariablesClient
	}
	tests := []struct {
		name    string
		args    args
		want    []byte
		wantErr bool
	}{
		{
			name: "pass and replaces variables",
			args: args{
				yaml: []byte("foo ${ BAR }"),
				configVariablesClient: test.NewFakeVariableClient().
					WithVar("BAR", "bar"),
			},
			want:    []byte("foo bar"),
			wantErr: false,
		},
		{
			name: "pass and replaces variables when variable name contains regex metacharacters",
			args: args{
				yaml: []byte("foo ${ BA$R }"),
				configVariablesClient: test.NewFakeVariableClient().
					WithVar("BA$R", "bar"),
			},
			want:    []byte("foo bar"),
			wantErr: false,
		},
		{
			name: "pass and replaces variables when variable value contains regex metacharacters",
			args: args{
				yaml: []byte("foo ${ BAR }"),
				configVariablesClient: test.NewFakeVariableClient().
					WithVar("BAR", "ba$r"),
			},
			want:    []byte("foo ba$r"),
			wantErr: false,
		},
		{
			name: "returns error when missing values for template variables",
			args: args{
				yaml:                  []byte("foo ${ BAR } ${ BAZ }"),
				configVariablesClient: test.NewFakeVariableClient(),
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			p := NewSimpleProcessor()

			got, err := p.Process(tt.args.yaml, tt.args.configVariablesClient.Get)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())

			g.Expect(got).To(Equal(tt.want))
		})
	}
}
