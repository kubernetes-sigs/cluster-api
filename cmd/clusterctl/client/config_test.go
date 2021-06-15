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
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/repository"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
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
			// note: these will be sorted by name by the Providers() call, so be sure they are in alphabetical order here too
			wantProviders: []string{
				config.ClusterAPIProviderName,
				config.AWSEKSBootstrapProviderName,
				config.KubeadmBootstrapProviderName,
				config.TalosBootstrapProviderName,
				config.AWSEKSControlPlaneProviderName,
				config.KubeadmControlPlaneProviderName,
				config.NestedControlPlaneProviderName,
				config.TalosControlPlaneProviderName,
				config.AWSProviderName,
				config.AzureProviderName,
				config.DOProviderName,
				config.DockerProviderName,
				config.GCPProviderName,
				config.Metal3ProviderName,
				config.NestedProviderName,
				config.OpenStackProviderName,
				config.PacketProviderName,
				config.SideroProviderName,
				config.VSphereProviderName,
			},
			wantErr: false,
		},
		{
			name: "Returns default providers and custom providers if defined",
			field: field{
				client: newFakeClient(newFakeConfig().WithProvider(customProviderConfig)),
			},
			// note: these will be sorted by name by the Providers() call, so be sure they are in alphabetical order here too
			wantProviders: []string{
				config.ClusterAPIProviderName,
				config.AWSEKSBootstrapProviderName,
				customProviderConfig.Name(),
				config.KubeadmBootstrapProviderName,
				config.TalosBootstrapProviderName,
				config.AWSEKSControlPlaneProviderName,
				config.KubeadmControlPlaneProviderName,
				config.NestedControlPlaneProviderName,
				config.TalosControlPlaneProviderName,
				config.AWSProviderName,
				config.AzureProviderName,
				config.DOProviderName,
				config.DockerProviderName,
				config.GCPProviderName,
				config.Metal3ProviderName,
				config.NestedProviderName,
				config.OpenStackProviderName,
				config.PacketProviderName,
				config.SideroProviderName,
				config.VSphereProviderName,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := tt.field.client.GetProvidersConfig()
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(got).To(HaveLen(len(tt.wantProviders)))

			for i, gotProvider := range got {
				w := tt.wantProviders[i]
				g.Expect(gotProvider.Name()).To(Equal(w))
			}
		})
	}
}

func Test_clusterctlClient_GetProviderComponents(t *testing.T) {
	config1 := newFakeConfig().
		WithProvider(capiProviderConfig)

	repository1 := newFakeRepository(capiProviderConfig, config1).
		WithPaths("root", "components.yaml").
		WithDefaultVersion("v1.0.0").
		WithFile("v1.0.0", "components.yaml", componentsYAML("ns1"))

	client := newFakeClient(config1).
		WithRepository(repository1)

	type args struct {
		provider        string
		targetNameSpace string
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
				provider:        capiProviderConfig.Name(),
				targetNameSpace: "ns2",
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
				provider:        fmt.Sprintf("%s:v0.2.0", capiProviderConfig.Name()),
				targetNameSpace: "ns2",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			options := ComponentsOptions{
				TargetNamespace: tt.args.targetNameSpace,
			}
			got, err := client.GetProviderComponents(tt.args.provider, capiProviderConfig.Type(), options)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())

			g.Expect(got.Name()).To(Equal(tt.want.provider.Name()))
			g.Expect(got.Version()).To(Equal(tt.want.version))
		})
	}
}

func Test_getComponentsByName_withEmptyVariables(t *testing.T) {
	g := NewWithT(t)

	// Create a fake config with a provider named P1 and a variable named foo.
	repository1Config := config.NewProvider("p1", "url", clusterctlv1.InfrastructureProviderType)

	config1 := newFakeConfig().
		WithProvider(repository1Config)

	repository1 := newFakeRepository(repository1Config, config1).
		WithPaths("root", "components.yaml").
		WithDefaultVersion("v1.0.0").
		WithFile("v1.0.0", "components.yaml", componentsYAML("${FOO}")).
		WithMetadata("v1.0.0", &clusterctlv1.Metadata{
			ReleaseSeries: []clusterctlv1.ReleaseSeries{
				{Major: 1, Minor: 0, Contract: "v1alpha3"},
			},
		})

	// Create a fake cluster, eventually adding some existing runtime objects to it.
	cluster1 := newFakeCluster(cluster.Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"}, config1).WithObjs()

	// Create a new fakeClient that allows to execute tests on the fake config,
	// the fake repositories and the fake cluster.
	client := newFakeClient(config1).
		WithRepository(repository1).
		WithCluster(cluster1)

	options := ComponentsOptions{
		TargetNamespace:     "ns1",
		SkipTemplateProcess: true,
	}
	components, err := client.GetProviderComponents(repository1Config.Name(), repository1Config.Type(), options)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(len(components.Variables())).To(Equal(1))
	g.Expect(components.Name()).To(Equal("p1"))
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
					ControlPlaneMachineCount: pointer.Int64Ptr(1),
					WorkerMachineCount:       pointer.Int64Ptr(2),
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
					ControlPlaneMachineCount: pointer.Int64Ptr(1),
					WorkerMachineCount:       pointer.Int64Ptr(2),
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
			name: "pass (using defaults for machine counts)",
			args: args{
				options: GetClusterTemplateOptions{
					ClusterName:       "foo",
					TargetNamespace:   "bar",
					KubernetesVersion: "v1.2.3",
				},
			},
			wantVars: map[string]string{
				"CLUSTER_NAME":                "foo",
				"NAMESPACE":                   "bar",
				"KUBERNETES_VERSION":          "v1.2.3",
				"CONTROL_PLANE_MACHINE_COUNT": "1",
				"WORKER_MACHINE_COUNT":        "0",
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
			name: "tolerates subdomains as cluster Name",
			args: args{
				options: GetClusterTemplateOptions{
					ClusterName:     "foo.bar",
					TargetNamespace: "baz",
				},
			},
			wantErr: false,
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
					ControlPlaneMachineCount: pointer.Int64Ptr(-1),
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
					ControlPlaneMachineCount: pointer.Int64Ptr(1),
					WorkerMachineCount:       pointer.Int64Ptr(-1),
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			config := newFakeConfig().
				WithVar("KUBERNETES_VERSION", "v3.4.5") // with this line we are simulating an env var

			c := &clusterctlClient{
				configClient: config,
			}
			err := c.templateOptionsToVariables(tt.args.options)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())

			for name, wantValue := range tt.wantVars {
				gotValue, err := config.Variables().Get(name)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(gotValue).To(Equal(wantValue))
			}
		})
	}
}

func Test_clusterctlClient_templateOptionsToVariables_withExistingMachineCountVariables(t *testing.T) {
	configClient := newFakeConfig().
		WithVar("CONTROL_PLANE_MACHINE_COUNT", "3").
		WithVar("WORKER_MACHINE_COUNT", "10")

	c := &clusterctlClient{
		configClient: configClient,
	}
	options := GetClusterTemplateOptions{
		ClusterName:       "foo",
		TargetNamespace:   "bar",
		KubernetesVersion: "v1.2.3",
	}

	wantVars := map[string]string{
		"CLUSTER_NAME":                "foo",
		"NAMESPACE":                   "bar",
		"KUBERNETES_VERSION":          "v1.2.3",
		"CONTROL_PLANE_MACHINE_COUNT": "3",
		"WORKER_MACHINE_COUNT":        "10",
	}

	if err := c.templateOptionsToVariables(options); err != nil {
		t.Fatalf("error = %v", err)
	}

	for name, wantValue := range wantVars {
		gotValue, err := configClient.Variables().Get(name)
		if err != nil {
			t.Fatalf("variable %s is not definied in config variables", name)
		}
		if gotValue != wantValue {
			t.Errorf("variable %s, got = %v, want %v", name, gotValue, wantValue)
		}
	}
}

func Test_clusterctlClient_GetClusterTemplate(t *testing.T) {
	g := NewWithT(t)

	rawTemplate := templateYAML("ns3", "${ CLUSTER_NAME }")

	// Template on a file
	tmpDir, err := os.MkdirTemp("", "cc")
	g.Expect(err).NotTo(HaveOccurred())
	defer os.RemoveAll(tmpDir)

	path := filepath.Join(tmpDir, "cluster-template.yaml")
	g.Expect(os.WriteFile(path, rawTemplate, 0600)).To(Succeed())

	// Template on a repository & in a ConfigMap
	configMap := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns1",
			Name:      "my-template",
		},
		Data: map[string]string{
			"prod": string(rawTemplate),
		},
	}

	config1 := newFakeConfig().
		WithProvider(infraProviderConfig)

	repository1 := newFakeRepository(infraProviderConfig, config1).
		WithPaths("root", "components").
		WithDefaultVersion("v3.0.0").
		WithFile("v3.0.0", "cluster-template.yaml", rawTemplate)

	cluster1 := newFakeCluster(cluster.Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"}, config1).
		WithProviderInventory(infraProviderConfig.Name(), infraProviderConfig.Type(), "v3.0.0", "foo").
		WithObjs(configMap).
		WithObjs(test.FakeCAPISetupObjects()...)

	client := newFakeClient(config1).
		WithCluster(cluster1).
		WithRepository(repository1)

	type args struct {
		options GetClusterTemplateOptions
	}

	type templateValues struct {
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
			name: "repository source - pass",
			args: args{
				options: GetClusterTemplateOptions{
					Kubeconfig: Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"},
					ProviderRepositorySource: &ProviderRepositorySourceOptions{
						InfrastructureProvider: "infra:v3.0.0",
						Flavor:                 "",
					},
					ClusterName:              "test",
					TargetNamespace:          "ns1",
					ControlPlaneMachineCount: pointer.Int64Ptr(1),
				},
			},
			want: templateValues{
				variables:       []string{"CLUSTER_NAME"}, // variable detected
				targetNamespace: "ns1",
				yaml:            templateYAML("ns1", "test"), // original template modified with target namespace and variable replacement
			},
		},
		{
			name: "repository source - detects provider name/version if missing",
			args: args{
				options: GetClusterTemplateOptions{
					Kubeconfig: Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"},
					ProviderRepositorySource: &ProviderRepositorySourceOptions{
						InfrastructureProvider: "", // empty triggers auto-detection of the provider name/version
						Flavor:                 "",
					},
					ClusterName:              "test",
					TargetNamespace:          "ns1",
					ControlPlaneMachineCount: pointer.Int64Ptr(1),
				},
			},
			want: templateValues{
				variables:       []string{"CLUSTER_NAME"}, // variable detected
				targetNamespace: "ns1",
				yaml:            templateYAML("ns1", "test"), // original template modified with target namespace and variable replacement
			},
		},
		{
			name: "repository source - use current namespace if targetNamespace is missing",
			args: args{
				options: GetClusterTemplateOptions{
					Kubeconfig: Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"},
					ProviderRepositorySource: &ProviderRepositorySourceOptions{
						InfrastructureProvider: "infra:v3.0.0",
						Flavor:                 "",
					},
					ClusterName:              "test",
					TargetNamespace:          "", // empty triggers usage of the current namespace
					ControlPlaneMachineCount: pointer.Int64Ptr(1),
				},
			},
			want: templateValues{
				variables:       []string{"CLUSTER_NAME"}, // variable detected
				targetNamespace: "default",
				yaml:            templateYAML("default", "test"), // original template modified with target namespace and variable replacement
			},
		},
		{
			name: "URL source - pass",
			args: args{
				options: GetClusterTemplateOptions{
					Kubeconfig: Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"},
					URLSource: &URLSourceOptions{
						URL: path,
					},
					ClusterName:              "test",
					TargetNamespace:          "ns1",
					ControlPlaneMachineCount: pointer.Int64Ptr(1),
				},
			},
			want: templateValues{
				variables:       []string{"CLUSTER_NAME"}, // variable detected
				targetNamespace: "ns1",
				yaml:            templateYAML("ns1", "test"), // original template modified with target namespace and variable replacement
			},
		},
		{
			name: "ConfigMap source - pass",
			args: args{
				options: GetClusterTemplateOptions{
					Kubeconfig: Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"},
					ConfigMapSource: &ConfigMapSourceOptions{
						Namespace: "ns1",
						Name:      "my-template",
						DataKey:   "prod",
					},
					ClusterName:              "test",
					TargetNamespace:          "ns1",
					ControlPlaneMachineCount: pointer.Int64Ptr(1),
				},
			},
			want: templateValues{
				variables:       []string{"CLUSTER_NAME"}, // variable detected
				targetNamespace: "ns1",
				yaml:            templateYAML("ns1", "test"), // original template modified with target namespace and variable replacement
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gs := NewWithT(t)

			got, err := client.GetClusterTemplate(tt.args.options)
			if tt.wantErr {
				gs.Expect(err).To(HaveOccurred())
				return
			}
			gs.Expect(err).NotTo(HaveOccurred())

			gs.Expect(got.Variables()).To(Equal(tt.want.variables))
			gs.Expect(got.TargetNamespace()).To(Equal(tt.want.targetNamespace))

			gotYaml, err := got.Yaml()
			gs.Expect(err).NotTo(HaveOccurred())
			gs.Expect(gotYaml).To(Equal(tt.want.yaml))
		})
	}
}

func Test_clusterctlClient_GetClusterTemplate_onEmptyCluster(t *testing.T) {
	g := NewWithT(t)

	rawTemplate := templateYAML("ns3", "${ CLUSTER_NAME }")

	// Template on a file
	tmpDir, err := os.MkdirTemp("", "cc")
	g.Expect(err).NotTo(HaveOccurred())
	defer os.RemoveAll(tmpDir)

	path := filepath.Join(tmpDir, "cluster-template.yaml")
	g.Expect(os.WriteFile(path, rawTemplate, 0600)).To(Succeed())

	// Template in a ConfigMap in a cluster not initialized
	configMap := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns1",
			Name:      "my-template",
		},
		Data: map[string]string{
			"prod": string(rawTemplate),
		},
	}

	config1 := newFakeConfig().
		WithProvider(infraProviderConfig)

	cluster1 := newFakeCluster(cluster.Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"}, config1).
		WithObjs(configMap)

	repository1 := newFakeRepository(infraProviderConfig, config1).
		WithPaths("root", "components").
		WithDefaultVersion("v3.0.0").
		WithFile("v3.0.0", "cluster-template.yaml", rawTemplate)

	client := newFakeClient(config1).
		WithCluster(cluster1).
		WithRepository(repository1)

	type args struct {
		options GetClusterTemplateOptions
	}

	type templateValues struct {
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
			name: "repository source - pass if the cluster is not initialized but infra provider:version are specified",
			args: args{
				options: GetClusterTemplateOptions{
					Kubeconfig: Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"},
					ProviderRepositorySource: &ProviderRepositorySourceOptions{
						InfrastructureProvider: "infra:v3.0.0",
						Flavor:                 "",
					},
					ClusterName:              "test",
					TargetNamespace:          "ns1",
					ControlPlaneMachineCount: pointer.Int64Ptr(1),
				},
			},
			want: templateValues{
				variables:       []string{"CLUSTER_NAME"}, // variable detected
				targetNamespace: "ns1",
				yaml:            templateYAML("ns1", "test"), // original template modified with target namespace and variable replacement
			},
		},
		{
			name: "repository source - fails if the cluster is not initialized and infra provider:version are not specified",
			args: args{
				options: GetClusterTemplateOptions{
					Kubeconfig: Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"},
					ProviderRepositorySource: &ProviderRepositorySourceOptions{
						InfrastructureProvider: "",
						Flavor:                 "",
					},
					ClusterName:              "test",
					TargetNamespace:          "ns1",
					ControlPlaneMachineCount: pointer.Int64Ptr(1),
				},
			},
			wantErr: true,
		},
		{
			name: "URL source - pass",
			args: args{
				options: GetClusterTemplateOptions{
					Kubeconfig: Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"},
					URLSource: &URLSourceOptions{
						URL: path,
					},
					ClusterName:              "test",
					TargetNamespace:          "ns1",
					ControlPlaneMachineCount: pointer.Int64Ptr(1),
				},
			},
			want: templateValues{
				variables:       []string{"CLUSTER_NAME"}, // variable detected
				targetNamespace: "ns1",
				yaml:            templateYAML("ns1", "test"), // original template modified with target namespace and variable replacement
			},
		},
		{
			name: "ConfigMap source - pass",
			args: args{
				options: GetClusterTemplateOptions{
					Kubeconfig: Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"},
					ConfigMapSource: &ConfigMapSourceOptions{
						Namespace: "ns1",
						Name:      "my-template",
						DataKey:   "prod",
					},
					ClusterName:              "test",
					TargetNamespace:          "ns1",
					ControlPlaneMachineCount: pointer.Int64Ptr(1),
				},
			},
			want: templateValues{
				variables:       []string{"CLUSTER_NAME"}, // variable detected
				targetNamespace: "ns1",
				yaml:            templateYAML("ns1", "test"), // original template modified with target namespace and variable replacement
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gs := NewWithT(t)

			got, err := client.GetClusterTemplate(tt.args.options)
			if tt.wantErr {
				gs.Expect(err).To(HaveOccurred())
				return
			}
			gs.Expect(err).NotTo(HaveOccurred())

			gs.Expect(got.Variables()).To(Equal(tt.want.variables))
			gs.Expect(got.TargetNamespace()).To(Equal(tt.want.targetNamespace))

			gotYaml, err := got.Yaml()
			gs.Expect(err).NotTo(HaveOccurred())
			gs.Expect(gotYaml).To(Equal(tt.want.yaml))
		})
	}
}

func newFakeClientWithoutCluster(configClient config.Client) *fakeClient {
	fake := &fakeClient{
		configClient: configClient,
		repositories: map[string]repository.Client{},
	}

	var err error
	fake.internalClient, err = newClusterctlClient("fake-config",
		InjectConfig(fake.configClient),
		InjectRepositoryFactory(func(input RepositoryClientFactoryInput) (repository.Client, error) {
			if _, ok := fake.repositories[input.Provider.ManifestLabel()]; !ok {
				return nil, errors.Errorf("Repository for kubeconfig %q does not exist.", input.Provider.ManifestLabel())
			}
			return fake.repositories[input.Provider.ManifestLabel()], nil
		}),
	)
	if err != nil {
		panic(err)
	}

	return fake
}

func Test_clusterctlClient_GetClusterTemplate_withoutCluster(t *testing.T) {
	rawTemplate := templateYAML("ns3", "${ CLUSTER_NAME }")

	config1 := newFakeConfig().
		WithProvider(infraProviderConfig)

	repository1 := newFakeRepository(infraProviderConfig, config1).
		WithPaths("root", "components").
		WithDefaultVersion("v3.0.0").
		WithFile("v3.0.0", "cluster-template.yaml", rawTemplate)

	client := newFakeClientWithoutCluster(config1).
		WithRepository(repository1)

	type args struct {
		options GetClusterTemplateOptions
	}

	type templateValues struct {
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
			name: "repository source - pass without kubeconfig but infra provider:version are specified",
			args: args{
				options: GetClusterTemplateOptions{
					Kubeconfig: Kubeconfig{Path: "", Context: ""},
					ProviderRepositorySource: &ProviderRepositorySourceOptions{
						InfrastructureProvider: "infra:v3.0.0",
						Flavor:                 "",
					},
					ClusterName:              "test",
					TargetNamespace:          "ns1",
					ControlPlaneMachineCount: pointer.Int64Ptr(1),
				},
			},
			want: templateValues{
				variables:       []string{"CLUSTER_NAME"}, // variable detected
				targetNamespace: "ns1",
				yaml:            templateYAML("ns1", "test"), // original template modified with target namespace and variable replacement
			},
		},
		{
			name: "repository source - fails without kubeconfig and infra provider:version are not specified",
			args: args{
				options: GetClusterTemplateOptions{
					Kubeconfig: Kubeconfig{Path: "", Context: ""},
					ProviderRepositorySource: &ProviderRepositorySourceOptions{
						InfrastructureProvider: "",
						Flavor:                 "",
					},
					ClusterName:              "test",
					TargetNamespace:          "ns1",
					ControlPlaneMachineCount: pointer.Int64Ptr(1),
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gs := NewWithT(t)

			got, err := client.GetClusterTemplate(tt.args.options)
			if tt.wantErr {
				gs.Expect(err).To(HaveOccurred())
				return
			}
			gs.Expect(err).NotTo(HaveOccurred())

			gs.Expect(got.Variables()).To(Equal(tt.want.variables))
			gs.Expect(got.TargetNamespace()).To(Equal(tt.want.targetNamespace))

			gotYaml, err := got.Yaml()
			gs.Expect(err).NotTo(HaveOccurred())
			gs.Expect(gotYaml).To(Equal(tt.want.yaml))
		})
	}
}

func Test_clusterctlClient_ProcessYAML(t *testing.T) {
	g := NewWithT(t)
	template := `v1: ${VAR1:=default1}
v2: ${VAR2=default2}
v3: ${VAR3:-default3}`
	dir, err := os.MkdirTemp("", "clusterctl")
	g.Expect(err).NotTo(HaveOccurred())
	defer os.RemoveAll(dir)

	templateFile := filepath.Join(dir, "template.yaml")
	g.Expect(os.WriteFile(templateFile, []byte(template), 0600)).To(Succeed())

	inputReader := strings.NewReader(template)

	tests := []struct {
		name         string
		options      ProcessYAMLOptions
		expectErr    bool
		expectedYaml string
		expectedVars []string
	}{
		{
			name: "returns the expected yaml and variables",
			options: ProcessYAMLOptions{
				URLSource: &URLSourceOptions{
					URL: templateFile,
				},
				SkipTemplateProcess: false,
			},
			expectErr: false,
			expectedYaml: `v1: default1
v2: default2
v3: default3`,
			expectedVars: []string{"VAR1", "VAR2", "VAR3"},
		},
		{
			name: "returns the expected variables only if SkipTemplateProcess is set",
			options: ProcessYAMLOptions{
				URLSource: &URLSourceOptions{
					URL: templateFile,
				},
				SkipTemplateProcess: true,
			},
			expectErr:    false,
			expectedYaml: ``,
			expectedVars: []string{"VAR1", "VAR2", "VAR3"},
		},
		{
			name:      "returns error if no source was specified",
			options:   ProcessYAMLOptions{},
			expectErr: true,
		},
		{
			name: "processes yaml from specified reader",
			options: ProcessYAMLOptions{
				ReaderSource: &ReaderSourceOptions{
					Reader: inputReader,
				},
				SkipTemplateProcess: false,
			},
			expectErr: false,
			expectedYaml: `v1: default1
v2: default2
v3: default3`,
			expectedVars: []string{"VAR1", "VAR2", "VAR3"},
		},
		{
			name: "returns error if unable to read from reader",
			options: ProcessYAMLOptions{
				ReaderSource: &ReaderSourceOptions{
					Reader: &errReader{},
				},
				SkipTemplateProcess: false,
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config1 := newFakeConfig().
				WithProvider(infraProviderConfig)
			cluster1 := newFakeCluster(cluster.Kubeconfig{}, config1)

			client := newFakeClient(config1).WithCluster(cluster1)

			printer, err := client.ProcessYAML(tt.options)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			expectedYaml, err := printer.Yaml()
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(string(expectedYaml)).To(Equal(tt.expectedYaml))

			expectedVars := printer.Variables()
			g.Expect(expectedVars).To(ConsistOf(tt.expectedVars))
		})
	}
}

// errReader returns a non-EOF error on the first read.
type errReader struct{}

func (e *errReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("read error")
}
