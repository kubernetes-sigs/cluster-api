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

package client

import (
	"fmt"
	"reflect"
	"testing"

	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/config"
)

func Test_clusterctlClient_GetProvidersConfig(t *testing.T) {
	customProviderConfig := config.NewProvider("custom", "url", clusterctlv1.BootstrapProviderType)

	type field struct {
		client Client
	}
	tests := []struct {
		name          string
		field         field
		wantProviders []string
		wantErr       bool
	}{
		{
			name: "Returns default providers",
			field: field{
				client: newFakeClient(newFakeConfig()),
			},
			wantProviders: []string{
				"aws",
				config.ClusterAPIName,
				"docker",
				config.KubeadmBootstrapProviderName,
				config.KubeadmControlPlaneProviderName,
				"vsphere",
			},
			wantErr: false,
		},
		{
			name: "Returns default providers and custom providers if defined",
			field: field{
				client: newFakeClient(newFakeConfig().WithProvider(customProviderConfig)),
			},
			wantProviders: []string{
				"aws",
				config.ClusterAPIName,
				customProviderConfig.Name(),
				"docker",
				config.KubeadmBootstrapProviderName,
				config.KubeadmControlPlaneProviderName,
				"vsphere",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.field.client.GetProvidersConfig()
			if tt.wantErr != (err != nil) {
				t.Fatalf("error = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			if len(got) != len(tt.wantProviders) {
				t.Errorf("got = %v items, want %v items", len(got), len(tt.wantProviders))
				return
			}

			for i, g := range got {
				w := tt.wantProviders[i]

				if g.Name() != w {
					t.Errorf("Item[%d].Name() got = %v, want = %v ", i, g.Name(), w)
				}
			}
		})
	}
}

func Test_clusterctlClient_GetProviderComponents(t *testing.T) {
	config1 := newFakeConfig().
		WithProvider(capiProviderConfig)

	repository1 := newFakeRepository(capiProviderConfig, config1.Variables()).
		WithPaths("root", "components.yaml").
		WithDefaultVersion("v1.0.0").
		WithFile("v1.0.0", "components.yaml", componentsYAML("ns1"))

	client := newFakeClient(config1).
		WithRepository(repository1)

	type args struct {
		provider          string
		targetNameSpace   string
		watchingNamespace string
	}
	type want struct {
		provider config.Provider
		version  string
	}
	tests := []struct {
		name    string
		args    args
		want    want
		wantErr bool
	}{
		{
			name: "Pass",
			args: args{
				provider:          capiProviderConfig.Name(),
				targetNameSpace:   "ns2",
				watchingNamespace: "",
			},
			want: want{
				provider: capiProviderConfig,
				version:  "v1.0.0",
			},
			wantErr: false,
		},
		{
			name: "Fail",
			args: args{
				provider:          fmt.Sprintf("%s:v0.2.0", capiProviderConfig.Name()),
				targetNameSpace:   "ns2",
				watchingNamespace: "",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := client.GetProviderComponents(tt.args.provider, tt.args.targetNameSpace, tt.args.watchingNamespace)
			if tt.wantErr != (err != nil) {
				t.Fatalf("error = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			if got.Name() != tt.want.provider.Name() {
				t.Errorf("got.Name() = %v, want = %v ", got.Name(), tt.want.provider.Name())
			}

			if got.Version() != tt.want.version {
				t.Errorf("got.Version() = %v, want = %v ", got.Version(), tt.want.version)
			}
		})
	}
}

func Test_clusterctlClient_templateOptionsToVariables(t *testing.T) {
	type args struct {
		options GetClusterTemplateOptions
	}
	tests := []struct {
		name     string
		args     args
		wantVars map[string]string
		wantErr  bool
	}{
		{
			name: "pass (using KubernetesVersion from template options)",
			args: args{
				options: GetClusterTemplateOptions{
					ClusterName:              "foo",
					TargetNamespace:          "bar",
					KubernetesVersion:        "v1.2.3",
					ControlPlaneMachineCount: 1,
					WorkerMachineCount:       2,
				},
			},
			wantVars: map[string]string{
				"CLUSTER_NAME":                "foo",
				"NAMESPACE":                   "bar",
				"KUBERNETES_VERSION":          "v1.2.3",
				"CONTROL_PLANE_MACHINE_COUNT": "1",
				"WORKER_MACHINE_COUNT":        "2",
			},
			wantErr: false,
		},
		{
			name: "pass (using KubernetesVersion from env variables)",
			args: args{
				options: GetClusterTemplateOptions{
					ClusterName:              "foo",
					TargetNamespace:          "bar",
					KubernetesVersion:        "", // empty means to use value from env variables/config file
					ControlPlaneMachineCount: 1,
					WorkerMachineCount:       2,
				},
			},
			wantVars: map[string]string{
				"CLUSTER_NAME":                "foo",
				"NAMESPACE":                   "bar",
				"KUBERNETES_VERSION":          "v3.4.5",
				"CONTROL_PLANE_MACHINE_COUNT": "1",
				"WORKER_MACHINE_COUNT":        "2",
			},
			wantErr: false,
		},
		{
			name: "fails for invalid cluster Name",
			args: args{
				options: GetClusterTemplateOptions{
					ClusterName: "A!££%",
				},
			},
			wantErr: true,
		},
		{
			name: "fails for invalid namespace Name",
			args: args{
				options: GetClusterTemplateOptions{
					ClusterName:     "foo",
					TargetNamespace: "A!££%",
				},
			},
			wantErr: true,
		},
		{
			name: "fails for invalid version",
			args: args{
				options: GetClusterTemplateOptions{
					ClusterName:       "foo",
					TargetNamespace:   "bar",
					KubernetesVersion: "A!££%",
				},
			},
			wantErr: true,
		},
		{
			name: "fails for invalid control plane machine count",
			args: args{
				options: GetClusterTemplateOptions{
					ClusterName:              "foo",
					TargetNamespace:          "bar",
					KubernetesVersion:        "v1.2.3",
					ControlPlaneMachineCount: -1,
				},
			},
			wantErr: true,
		},
		{
			name: "fails for invalid worker machine count",
			args: args{
				options: GetClusterTemplateOptions{
					ClusterName:              "foo",
					TargetNamespace:          "bar",
					KubernetesVersion:        "v1.2.3",
					ControlPlaneMachineCount: 1,
					WorkerMachineCount:       -1,
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := newFakeConfig().
				WithVar("KUBERNETES_VERSION", "v3.4.5") // with this line we are simulating an env var

			c := &clusterctlClient{
				configClient: config,
			}
			err := c.templateOptionsToVariables(tt.args.options)
			if tt.wantErr != (err != nil) {
				t.Fatalf("error = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			for name, wantValue := range tt.wantVars {
				gotValue, err := config.Variables().Get(name)
				if err != nil {
					t.Fatalf("variable %s is not definied in config variables", name)
				}
				if gotValue != wantValue {
					t.Errorf("variable %s, got = %v, want %v", name, gotValue, wantValue)
				}
			}
		})
	}
}

func Test_clusterctlClient_GetClusterTemplate(t *testing.T) {
	config1 := newFakeConfig().
		WithProvider(infraProviderConfig)

	repository1 := newFakeRepository(infraProviderConfig, config1.Variables()).
		WithPaths("root", "components").
		WithDefaultVersion("v3.0.0").
		WithFile("v3.0.0", "cluster-template.yaml", templateYAML("ns3", "${ CLUSTER_NAME }"))

	cluster1 := newFakeCluster("kubeconfig").
		WithProviderInventory(infraProviderConfig.Name(), infraProviderConfig.Type(), "v3.0.0", "foo", "bar")

	client := newFakeClient(config1).
		WithCluster(cluster1).
		WithRepository(repository1)

	type args struct {
		options GetClusterTemplateOptions
	}

	type templateValues struct {
		name            string
		url             string
		providerType    clusterctlv1.ProviderType
		version         string
		flavor          string
		variables       []string
		targetNamespace string
		yaml            []byte
	}

	tests := []struct {
		name    string
		args    args
		want    templateValues
		wantErr bool
	}{
		{
			name: "pass",
			args: args{
				options: GetClusterTemplateOptions{
					Kubeconfig:               "kubeconfig",
					InfrastructureProvider:   "infra:v3.0.0",
					Flavor:                   "",
					ClusterName:              "test",
					TargetNamespace:          "ns1",
					ControlPlaneMachineCount: 1,
				},
			},
			want: templateValues{
				name:            "infra",
				url:             "url",
				providerType:    clusterctlv1.InfrastructureProviderType,
				version:         "v3.0.0",
				flavor:          "",
				variables:       []string{"CLUSTER_NAME"}, // variable detected
				targetNamespace: "ns1",
				yaml:            templateYAML("ns1", "test"), // original template modified with target namespace and variable replacement
			},
		},
		{
			name: "detects provider name/version if missing",
			args: args{
				options: GetClusterTemplateOptions{
					Kubeconfig:               "kubeconfig",
					InfrastructureProvider:   "", // empty triggers auto-detection of the provider name/version
					Flavor:                   "",
					ClusterName:              "test",
					TargetNamespace:          "ns1",
					ControlPlaneMachineCount: 1,
				},
			},
			want: templateValues{
				name:            "infra",
				url:             "url",
				providerType:    clusterctlv1.InfrastructureProviderType,
				version:         "v3.0.0",
				flavor:          "",
				variables:       []string{"CLUSTER_NAME"}, // variable detected
				targetNamespace: "ns1",
				yaml:            templateYAML("ns1", "test"), // original template modified with target namespace and variable replacement
			},
		},
		{
			name: "use current namespace if targetNamespace is missing",
			args: args{
				options: GetClusterTemplateOptions{
					Kubeconfig:               "kubeconfig",
					InfrastructureProvider:   "infra:v3.0.0",
					Flavor:                   "",
					ClusterName:              "test",
					TargetNamespace:          "", // empty triggers usage of the current namespace
					ControlPlaneMachineCount: 1,
				},
			},
			want: templateValues{
				name:            "infra",
				url:             "url",
				providerType:    clusterctlv1.InfrastructureProviderType,
				version:         "v3.0.0",
				flavor:          "",
				variables:       []string{"CLUSTER_NAME"}, // variable detected
				targetNamespace: "default",
				yaml:            templateYAML("default", "test"), // original template modified with target namespace and variable replacement
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := client.GetClusterTemplate(tt.args.options)
			if tt.wantErr != (err != nil) {
				t.Fatalf("error = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			if got.Name() != tt.want.name {
				t.Errorf("Name() got = %v, want %v", got.Name(), tt.want.name)
			}
			if got.URL() != tt.want.url {
				t.Errorf("URL() got = %v, want %v", got.URL(), tt.want.url)
			}
			if got.Type() != tt.want.providerType {
				t.Errorf("Type() got = %v, want %v", got.Type(), tt.want.providerType)
			}
			if got.Version() != tt.want.version {
				t.Errorf("Version() got = %v, want %v", got.Version(), tt.want.version)
			}
			if got.Flavor() != tt.want.flavor {
				t.Errorf("Flavor() got = %v, want %v", got.Flavor(), tt.want.flavor)
			}
			if !reflect.DeepEqual(got.Variables(), tt.want.variables) {
				t.Errorf("Variables() got = %v, want %v", got.Variables(), tt.want.variables)
			}
			if got.TargetNamespace() != tt.want.targetNamespace {
				t.Errorf("TargetNamespace() got = %v, want %v", got.TargetNamespace(), tt.want.targetNamespace)
			}

			gotYaml, err := got.Yaml()
			if err != nil {
				t.Fatalf("Yaml() error = %v, wantErr nil", err)
			}
			if !reflect.DeepEqual(gotYaml, tt.want.yaml) {
				t.Errorf("Yaml() got = %v, want %v", gotYaml, tt.want.yaml)
			}
		})
	}
}
