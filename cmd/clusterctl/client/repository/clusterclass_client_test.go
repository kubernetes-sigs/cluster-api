/*
Copyright 2021 The Kubernetes Authors.

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

package repository

import (
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	yaml "sigs.k8s.io/cluster-api/cmd/clusterctl/client/yamlprocessor"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
)

func Test_ClusterClassClient_Get(t *testing.T) {
	p1 := config.NewProvider("p1", "", clusterctlv1.BootstrapProviderType)

	type fields struct {
		version               string
		provider              config.Provider
		repository            Repository
		configVariablesClient config.VariablesClient
		processor             yaml.Processor
	}
	type args struct {
		name              string
		targetNamespace   string
		listVariablesOnly bool
	}
	type want struct {
		variables       []string
		targetNamespace string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    want
		wantErr bool
	}{
		{
			name: "pass if clusterclass of name exists",
			fields: fields{
				version:  "v1.0",
				provider: p1,
				repository: NewMemoryRepository().
					WithPaths("root", "").
					WithDefaultVersion("v1.0").
					WithFile("v1.0", "clusterclass-dev.yaml", templateMapYaml),
				configVariablesClient: test.NewFakeVariableClient().WithVar(variableName, variableValue),
				processor:             yaml.NewSimpleProcessor(),
			},
			args: args{
				name:              "dev",
				targetNamespace:   "ns1",
				listVariablesOnly: false,
			},
			want: want{
				variables:       []string{variableName},
				targetNamespace: "ns1",
			},
			wantErr: false,
		},
		{
			name: "fails if clusterclass does not exists",
			fields: fields{
				version:  "v1.0",
				provider: p1,
				repository: NewMemoryRepository().
					WithPaths("root", "").
					WithDefaultVersion("v1.0"),
				configVariablesClient: test.NewFakeVariableClient().WithVar(variableName, variableValue),
				processor:             yaml.NewSimpleProcessor(),
			},
			args: args{
				name:              "dev",
				targetNamespace:   "ns1",
				listVariablesOnly: false,
			},
			wantErr: true,
		},
		{
			name: "fails if variables does not exists",
			fields: fields{
				version:  "v1.0",
				provider: p1,
				repository: NewMemoryRepository().
					WithPaths("root", "").
					WithDefaultVersion("v1.0").
					WithFile("v1.0", "clusterclass-dev.yaml", templateMapYaml),
				configVariablesClient: test.NewFakeVariableClient(),
				processor:             yaml.NewSimpleProcessor(),
			},
			args: args{
				name:              "dev",
				targetNamespace:   "ns1",
				listVariablesOnly: false,
			},
			wantErr: true,
		},
		{
			name: "pass if variables does not exists but skipTemplateProcess flag is set",
			fields: fields{
				version:  "v1.0",
				provider: p1,
				repository: NewMemoryRepository().
					WithPaths("root", "").
					WithDefaultVersion("v1.0").
					WithFile("v1.0", "clusterclass-dev.yaml", templateMapYaml),
				configVariablesClient: test.NewFakeVariableClient(),
				processor:             yaml.NewSimpleProcessor(),
			},
			args: args{
				name:              "dev",
				targetNamespace:   "ns1",
				listVariablesOnly: true,
			},
			want: want{
				variables:       []string{variableName},
				targetNamespace: "ns1",
			},
			wantErr: false,
		},
		{
			name: "returns error if processor is unable to get variables",
			fields: fields{
				version:  "v1.0",
				provider: p1,
				repository: NewMemoryRepository().
					WithPaths("root", "").
					WithDefaultVersion("v1.0").
					WithFile("v1.0", "clusterclass-dev.yaml", templateMapYaml),
				configVariablesClient: test.NewFakeVariableClient().WithVar(variableName, variableValue),
				processor:             test.NewFakeProcessor().WithGetVariablesErr(errors.New("cannot get vars")).WithTemplateName("clusterclass-dev.yaml"),
			},
			args: args{
				targetNamespace:   "ns1",
				listVariablesOnly: true,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			f := newClusterClassClient(
				ClusterClassClientInput{
					version:               tt.fields.version,
					provider:              tt.fields.provider,
					repository:            tt.fields.repository,
					configVariablesClient: tt.fields.configVariablesClient,
					processor:             tt.fields.processor,
				},
			)
			got, err := f.Get(tt.args.name, tt.args.targetNamespace, tt.args.listVariablesOnly)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())

			g.Expect(got.Variables()).To(Equal(tt.want.variables))
			g.Expect(got.TargetNamespace()).To(Equal(tt.want.targetNamespace))

			// check variable replaced in yaml
			yaml, err := got.Yaml()
			g.Expect(err).NotTo(HaveOccurred())

			if !tt.args.listVariablesOnly {
				g.Expect(yaml).To(ContainSubstring((fmt.Sprintf("variable: %s", variableValue))))
			}

			// check if target namespace is set
			for _, o := range got.Objs() {
				g.Expect(o.GetNamespace()).To(Equal(tt.want.targetNamespace))
			}
		})
	}
}
