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
	"bytes"
	"fmt"
	"reflect"
	"testing"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/test"
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
		targetNamespace       string
		listVariablesOnly     bool
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
				targetNamespace:       "ns1",
				listVariablesOnly:     false,
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
				targetNamespace:       "ns1",
				listVariablesOnly:     true,
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
			got, err := NewTemplate(tt.args.rawYaml, tt.args.configVariablesClient, tt.args.targetNamespace, tt.args.listVariablesOnly)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			if !reflect.DeepEqual(got.Variables(), tt.want.variables) {
				t.Errorf("got.Variables() = %v, want = %v ", got.Variables(), tt.want.variables)
			}

			if !reflect.DeepEqual(got.TargetNamespace(), tt.want.targetNamespace) {
				t.Errorf("got.TargetNamespace() = %v, want = %v ", got.TargetNamespace(), tt.want.targetNamespace)
			}

			if tt.args.listVariablesOnly {
				return
			}

			// check variable replaced in components
			yaml, err := got.Yaml()
			if err != nil {
				t.Fatalf("got.Yaml error = %v", err)
			}

			if !bytes.Contains(yaml, []byte(fmt.Sprintf("variable: %s", variableValue))) {
				t.Error("got.Yaml without variable substitution")
			}
		})
	}
}
