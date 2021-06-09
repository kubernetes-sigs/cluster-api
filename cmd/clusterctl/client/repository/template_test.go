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

package repository

import (
	"fmt"
	"testing"

	. "github.com/onsi/gomega"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	yaml "sigs.k8s.io/cluster-api/cmd/clusterctl/client/yamlprocessor"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
)

// Nb.We are using core objects vs Machines/Cluster etc. because it is easier to test (you don't have to deal with CRDs
// or schema issues), but this is ok because a template can be any yaml that complies the clusterctl contract.
var templateMapYaml = []byte("apiVersion: v1\n" +
	"data:\n" +
	fmt.Sprintf("  variable: ${%s}\n", variableName) +
	"kind: ConfigMap\n" +
	"metadata:\n" +
	"  name: manager")

func Test_newTemplate(t *testing.T) {
	type args struct {
		rawYaml               []byte
		configVariablesClient config.VariablesClient
		processor             yaml.Processor
		targetNamespace       string
		skipTemplateProcess   bool
	}
	type want struct {
		variables       []string
		targetNamespace string
	}
	tests := []struct {
		name    string
		args    args
		want    want
		wantErr bool
	}{
		{
			name: "variable is replaced and namespace fixed",
			args: args{
				rawYaml:               templateMapYaml,
				configVariablesClient: test.NewFakeVariableClient().WithVar(variableName, variableValue),
				processor:             yaml.NewSimpleProcessor(),
				targetNamespace:       "ns1",
				skipTemplateProcess:   false,
			},
			want: want{
				variables:       []string{variableName},
				targetNamespace: "ns1",
			},
			wantErr: false,
		},
		{
			name: "List variable only",
			args: args{
				rawYaml:               templateMapYaml,
				configVariablesClient: test.NewFakeVariableClient(),
				processor:             yaml.NewSimpleProcessor(),
				targetNamespace:       "ns1",
				skipTemplateProcess:   true,
			},
			want: want{
				variables:       []string{variableName},
				targetNamespace: "ns1",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := NewTemplate(TemplateInput{
				RawArtifact:           tt.args.rawYaml,
				ConfigVariablesClient: tt.args.configVariablesClient,
				Processor:             tt.args.processor,
				TargetNamespace:       tt.args.targetNamespace,
				SkipTemplateProcess:   tt.args.skipTemplateProcess,
			})
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())

			g.Expect(got.Variables()).To(Equal(tt.want.variables))
			g.Expect(got.TargetNamespace()).To(Equal(tt.want.targetNamespace))

			if tt.args.skipTemplateProcess {
				return
			}

			// check variable replaced in components
			yml, err := got.Yaml()
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(yml).To(ContainSubstring(fmt.Sprintf("variable: %s", variableValue)))
		})
	}
}
