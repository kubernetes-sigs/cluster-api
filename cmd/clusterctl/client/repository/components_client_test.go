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
	"fmt"
	"testing"

	. "github.com/onsi/gomega"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/util"
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
	g := NewWithT(t)

	p1 := config.NewProvider("p1", "", clusterctlv1.BootstrapProviderType)

	configClient, err := config.New("", config.InjectReader(test.NewFakeReader().WithVar(variableName, variableValue)))
	g.Expect(err).NotTo(HaveOccurred())

	type fields struct {
		provider   config.Provider
		repository Repository
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
			f := newComponentsClient(tt.fields.provider, tt.fields.repository, configClient)
			got, err := f.Get(tt.args.version, tt.args.targetNamespace, tt.args.watchingNamespace)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())

			g.Expect(got.Name()).To(Equal(tt.want.provider.Name()))
			g.Expect(got.Type()).To(Equal(tt.want.provider.Type()))
			g.Expect(got.Version()).To(Equal(tt.want.version))
			g.Expect(got.TargetNamespace()).To(Equal(tt.want.targetNamespace))
			g.Expect(got.WatchingNamespace()).To(Equal(tt.want.watchingNamespace))
			g.Expect(got.Variables()).To(Equal(tt.want.variables))

			yaml, err := got.Yaml()
			if err != nil {
				t.Errorf("got.Yaml() error = %v", err)
				return
			}

			if len(tt.want.variables) > 0 {
				g.Expect(yaml).To(ContainSubstring(variableValue))
			}

			for _, o := range got.InstanceObjs() {
				for _, v := range []string{clusterctlv1.ClusterctlLabelName, clusterv1.ProviderLabelName} {
					g.Expect(o.GetLabels()).To(HaveKey(v))
				}
			}

			for _, o := range got.SharedObjs() {
				for _, v := range []string{clusterctlv1.ClusterctlLabelName, clusterv1.ProviderLabelName, clusterctlv1.ClusterctlResourceLifecyleLabelName} {
					g.Expect(o.GetLabels()).To(HaveKey(v))
				}
			}
		})
	}
}
