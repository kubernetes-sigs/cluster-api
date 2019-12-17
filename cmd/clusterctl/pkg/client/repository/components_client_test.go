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

package repository

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/test"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/util"
)

const (
	variableName  = "FOO"
	variableValue = "foo"
)

var controllerYaml = []byte("apiVersion: apps/v1\n" +
	"kind: Deployment\n" +
	"metadata:\n" +
	"  name: my-controller\n" +
	"spec:\n" +
	"  template:\n" +
	"    spec:\n" +
	"      containers:\n" +
	"      - name: manager\n")

const namespaceName = "capa-system"

var namespaceYaml = []byte("apiVersion: v1\n" +
	"kind: Namespace\n" +
	"metadata:\n" +
	fmt.Sprintf("  name: %s", namespaceName))

var configMapYaml = []byte("apiVersion: v1\n" +
	"data:\n" +
	fmt.Sprintf("  variable: ${%s}\n", variableName) +
	"kind: ConfigMap\n" +
	"metadata:\n" +
	"  name: manager")

func Test_componentsClient_Get(t *testing.T) {
	p1 := config.NewProvider("p1", "", clusterctlv1.BootstrapProviderType)

	type fields struct {
		provider              config.Provider
		repository            Repository
		configVariablesClient config.VariablesClient
	}
	type args struct {
		version           string
		targetNamespace   string
		watchingNamespace string
	}
	type want struct {
		provider          config.Provider
		version           string
		targetNamespace   string
		watchingNamespace string
		variables         []string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    want
		wantErr bool
	}{
		{
			name: "Pass",
			fields: fields{
				provider: p1,
				repository: test.NewFakeRepository().
					WithPaths("root", "components.yaml").
					WithDefaultVersion("v1.0.0").
					WithFile("v1.0.0", "components.yaml", util.JoinYaml(namespaceYaml, controllerYaml, configMapYaml)),
				configVariablesClient: test.NewFakeVariableClient().WithVar(variableName, variableValue),
			},
			args: args{
				version:           "v1.0.0",
				targetNamespace:   "",
				watchingNamespace: "",
			},
			want: want{
				provider:          p1,
				version:           "v1.0.0",      // version detected
				targetNamespace:   namespaceName, // default targetNamespace detected
				watchingNamespace: "",
				variables:         []string{variableName}, // variable detected
			},
			wantErr: false,
		},
		{
			name: "targetNamespace overrides default targetNamespace",
			fields: fields{
				provider: p1,
				repository: test.NewFakeRepository().
					WithPaths("root", "components.yaml").
					WithDefaultVersion("v1.0.0").
					WithFile("v1.0.0", "components.yaml", util.JoinYaml(namespaceYaml, controllerYaml, configMapYaml)),
				configVariablesClient: test.NewFakeVariableClient().WithVar(variableName, variableValue),
			},
			args: args{
				version:           "v1.0.0",
				targetNamespace:   "ns2",
				watchingNamespace: "",
			},
			want: want{
				provider:          p1,
				version:           "v1.0.0", // version detected
				targetNamespace:   "ns2",    // targetNamespace overrides default targetNamespace
				watchingNamespace: "",
				variables:         []string{variableName}, // variable detected
			},
			wantErr: false,
		},
		{
			name: "watchingNamespace overrides default watchingNamespace",
			fields: fields{
				provider: p1,
				repository: test.NewFakeRepository().
					WithPaths("root", "components.yaml").
					WithDefaultVersion("v1.0.0").
					WithFile("v1.0.0", "components.yaml", util.JoinYaml(namespaceYaml, controllerYaml, configMapYaml)),
				configVariablesClient: test.NewFakeVariableClient().WithVar(variableName, variableValue),
			},
			args: args{
				version:           "v1.0.0",
				targetNamespace:   "",
				watchingNamespace: "ns2",
			},
			want: want{
				provider:          p1,
				version:           "v1.0.0",               // version detected
				targetNamespace:   namespaceName,          // default targetNamespace detected
				watchingNamespace: "ns2",                  // watchingNamespace overrides default watchingNamespace
				variables:         []string{variableName}, // variable detected
			},
			wantErr: false,
		},
		{
			name: "Fails if components file does not exists",
			fields: fields{
				provider: p1,
				repository: test.NewFakeRepository().
					WithPaths("root", "components.yaml").
					WithDefaultVersion("v1.0.0"),
				configVariablesClient: test.NewFakeVariableClient().WithVar(variableName, variableValue),
			},
			args: args{
				version:           "v1.0.0",
				targetNamespace:   "",
				watchingNamespace: "",
			},
			wantErr: true,
		},
		{
			name: "Fails if default targetNamespace does not exists",
			fields: fields{
				provider: p1,
				repository: test.NewFakeRepository().
					WithPaths("root", "components.yaml").
					WithDefaultVersion("v1.0.0").
					WithFile("v1.0.0", "components.yaml", util.JoinYaml(controllerYaml, configMapYaml)),
				configVariablesClient: test.NewFakeVariableClient().WithVar(variableName, variableValue),
			},
			args: args{
				version:           "v1.0.0",
				targetNamespace:   "",
				watchingNamespace: "",
			},
			wantErr: true,
		},
		{
			name: "Pass if default targetNamespace does not exists but a target targetNamespace is set",
			fields: fields{
				provider: p1,
				repository: test.NewFakeRepository().
					WithPaths("root", "components.yaml").
					WithDefaultVersion("v1.0.0").
					WithFile("v1.0.0", "components.yaml", util.JoinYaml(controllerYaml, configMapYaml)),
				configVariablesClient: test.NewFakeVariableClient().WithVar(variableName, variableValue),
			},
			args: args{
				version:           "v1.0.0",
				targetNamespace:   "ns2",
				watchingNamespace: "",
			},
			want: want{
				provider:          p1,
				version:           "v1.0.0", // version detected
				targetNamespace:   "ns2",    // target targetNamespace applied
				watchingNamespace: "",
				variables:         []string{variableName}, // variable detected
			},
			wantErr: false,
		},
		{
			name: "Fails if requested version does not exists",
			fields: fields{
				provider: p1,
				repository: test.NewFakeRepository().
					WithPaths("root", "components.yaml").
					WithDefaultVersion("v1.0.0").
					WithFile("v1.0.0", "components.yaml", util.JoinYaml(controllerYaml, configMapYaml)),
				configVariablesClient: test.NewFakeVariableClient().WithVar(variableName, variableValue),
			},
			args: args{
				version:           "v2.0.0",
				targetNamespace:   "",
				watchingNamespace: "",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newComponentsClient(tt.fields.provider, tt.fields.repository, tt.fields.configVariablesClient)
			got, err := f.Get(tt.args.version, tt.args.targetNamespace, tt.args.watchingNamespace)
			if (err != nil) != tt.wantErr {
				t.Errorf("Get() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			if got.Name() != tt.want.provider.Name() {
				t.Errorf("Get().Name() got = %v, want = %v ", got.Name(), tt.want.provider.Name())
			}

			if got.Type() != tt.want.provider.Type() {
				t.Errorf("Get().Type()  got = %v, want = %v ", got.Type(), tt.want.provider.Type())
			}

			if got.Version() != tt.want.version {
				t.Errorf("Get().Version() got = %v, want = %v ", got.Version(), tt.want.version)
			}

			if got.TargetNamespace() != tt.want.targetNamespace {
				t.Errorf("Get().TargetNamespace() got = %v, want = %v ", got.TargetNamespace(), tt.want.targetNamespace)
			}

			if got.WatchingNamespace() != tt.want.watchingNamespace {
				t.Errorf("Get().WatchingNamespace() got = %v, want = %v ", got.WatchingNamespace(), tt.want.watchingNamespace)
			}

			if !reflect.DeepEqual(got.Variables(), tt.want.variables) {
				t.Errorf("Get().Variables() got = %v, want = %v ", got.WatchingNamespace(), tt.want.watchingNamespace)
			}

			yaml, err := got.Yaml()
			if err != nil {
				t.Errorf("got.Yaml() error = %v", err)
				return
			}

			if len(tt.want.variables) > 0 && !bytes.Contains(yaml, []byte(variableValue)) {
				t.Errorf("Get().Yaml() does not containt value %s that is a replacement of %s variable", variableValue, variableName)
			}

			if len(tt.want.variables) > 0 && !bytes.Contains(yaml, []byte(variableValue)) {
				t.Errorf("Get().Yaml() does not containt value %s that is a replacement of %s variable", variableValue, variableName)
			}

			for _, o := range got.Objs() {
				for _, v := range []string{clusterctlv1.ClusterctlLabelName, clusterctlv1.ClusterctlProviderLabelName} {
					if _, ok := o.GetLabels()[v]; !ok {
						t.Errorf("Get().Objs() object %s does not contains %s label", o.GetName(), v)
					}
				}
			}
		})
	}
}
