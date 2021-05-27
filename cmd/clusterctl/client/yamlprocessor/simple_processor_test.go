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
		name    string
		args    args
		want    []string
		wantErr bool
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
				data: "yaml with ${A } ${A} ${A}",
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
			name: "returns error for variables with regex metacharacters",
			args: args{
				data: "yaml with ${BA$R}\n${FOO}",
			},
			wantErr: true,
		},
		{
			name: "variables with envsubst functions are properly parsed",
			args: args{
				data: "yaml with ${C:=default}\n${B}\n${A=foobar}",
			},
			want: []string{"A", "B", "C"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			p := NewSimpleProcessor()
			actual, err := p.GetVariables([]byte(tt.args.data))
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(actual).To(Equal(tt.want))
		})
	}
}

func TestSimpleProcessor_GetVariablesMap(t *testing.T) {
	type args struct {
		data string
	}
	def := "default"
	aVar := "${A}"
	foobar := "foobar"
	quotes := `""`
	tests := []struct {
		name    string
		args    args
		want    map[string]*string
		wantErr bool
	}{
		{
			name: "variable with different spacing around the name",
			args: args{
				data: "yaml with ${A} ${ B} ${ C} ${ D }",
			},
			want: map[string]*string{"A": nil, "B": nil, "C": nil, "D": nil},
		},
		{
			name: "variables used in many places are grouped",
			args: args{
				data: "yaml with ${A } ${A} ${A}",
			},
			want: map[string]*string{"A": nil},
		},
		{
			name: "variables in multiline texts are processed",
			args: args{
				data: "yaml with ${A}\n${B}\n${C}",
			},
			want: map[string]*string{"A": nil, "B": nil, "C": nil},
		},
		{
			name: "returns error for variables with regex metacharacters",
			args: args{
				data: "yaml with ${BA$R}\n${FOO}",
			},
			wantErr: true,
		},
		{
			name: "variables with envsubst functions are properly parsed",
			args: args{
				data: `yaml with ${C:=default}\n${B}\n${A=foobar}\n${E=""}\n${D:=${A}}`,
			},
			want: map[string]*string{"A": &foobar, "B": nil, "C": &def, "D": &aVar, "E": &quotes},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			p := NewSimpleProcessor()
			actual, err := p.GetVariableMap([]byte(tt.args.data))
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(actual).To(Equal(tt.want))
		})
	}
}

func TestSimpleProcessor_Process(t *testing.T) {
	type args struct {
		yaml                  []byte
		configVariablesClient config.VariablesClient
	}
	tests := []struct {
		name             string
		args             args
		want             []byte
		wantErr          bool
		missingVariables []string
	}{
		{
			name: "replaces legacy variables names (with spaces)",
			args: args{
				yaml: []byte("foo ${ BAR }, ${BAR }, ${ BAR}"),
				configVariablesClient: test.NewFakeVariableClient().
					WithVar("BAR", "bar"),
			},
			want:    []byte("foo bar, bar, bar"),
			wantErr: false,
		},
		{
			name: "does not escape slashes used for windows named pipes",
			args: args{
				yaml: []byte(`\\ foo ${ BAR }, ${BAR }, ${ BAR}`),
				configVariablesClient: test.NewFakeVariableClient().
					WithVar("BAR", "bar"),
			},
			want:    []byte(`\\ foo bar, bar, bar`),
			wantErr: false,
		},
		{
			name: "replaces variables when variable value contains regex metacharacters",
			args: args{
				yaml: []byte("foo ${BAR}"),
				configVariablesClient: test.NewFakeVariableClient().
					WithVar("BAR", "ba$r"),
			},
			want:    []byte("foo ba$r"),
			wantErr: false,
		},
		{
			name: "uses default values if variable doesn't exist in variables client",
			args: args{
				yaml: []byte("foo ${BAR=default_bar} ${BAZ:=default_baz} ${CAR=default_car} ${CAZ:-default_caz} ${DAR=default_dar}"),
				configVariablesClient: test.NewFakeVariableClient().
					// CAZ,DAR is set but has no value
					WithVar("BAR", "ba$r").WithVar("CAZ", "").WithVar("DAR", ""),
			},
			want:    []byte("foo ba$r default_baz default_car default_caz default_dar"),
			wantErr: false,
		},
		{
			name: "uses default variables if main variable is doesn't exist",
			args: args{
				yaml: []byte("foo ${BAR=default_bar} ${BAZ:=prefix${DEF}-suffix} ${CAZ=${DEF}}"),
				configVariablesClient: test.NewFakeVariableClient().
					WithVar("BAR", "ba$r").WithVar("DEF", "football"),
			},
			want:    []byte("foo ba$r prefixfootball-suffix football"),
			wantErr: false,
		},
		{
			name: "returns error with missing template variables listed (for better ux)",
			args: args{
				yaml: []byte("foo ${ BAR} ${BAZ} ${CAR}"),
				configVariablesClient: test.NewFakeVariableClient().
					WithVar("CAR", "car"),
			},
			want:             nil,
			wantErr:          true,
			missingVariables: []string{"BAR", "BAZ"},
		},
		{
			name: "returns error when variable name contains regex metacharacters",
			args: args{
				yaml: []byte("foo ${BA$R} ${BA_R}"),
				configVariablesClient: test.NewFakeVariableClient().
					WithVar("BA$R", "bar").WithVar("BA_R", "ba_r"),
			},
			want:    []byte("foo bar ba_r"),
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
				if len(tt.missingVariables) != 0 {
					e, ok := err.(*errMissingVariables)
					g.Expect(ok).To(BeTrue())
					g.Expect(e.Missing).To(ConsistOf(tt.missingVariables))
				}
				// we want to ensure that we keep returning the original yaml
				// as per the intended behavior of Process
				g.Expect(got).To(Equal(tt.args.yaml))
				return
			}
			g.Expect(err).NotTo(HaveOccurred())

			g.Expect(got).To(Equal(tt.want))
		})
	}
}
