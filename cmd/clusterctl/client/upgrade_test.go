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

package client

import (
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
)

func Test_clusterctlClient_PlanCertUpgrade(t *testing.T) {
	// create a fake config with a provider named P1 and a variable named var
	repository1Config := config.NewProvider("p1", "url", clusterctlv1.CoreProviderType)

	config1 := newFakeConfig().
		WithVar("var", "value").
		WithProvider(repository1Config)

	// create a fake repository with some YAML files in it (usually matching
	// the list of providers defined in the config)
	repository1 := newFakeRepository(repository1Config, config1).
		WithPaths("root", "components").
		WithDefaultVersion("v1.0").
		WithFile("v1.0", "components.yaml", []byte("content"))

	certManagerPlan := CertManagerUpgradePlan{
		From:          "v0.16.1",
		To:            "v1.1.0",
		ShouldUpgrade: true,
	}
	// create a fake cluster, with a cert manager client that has an upgrade
	// plan
	cluster1 := newFakeCluster(cluster.Kubeconfig{Path: "cluster1"}, config1).
		WithCertManagerClient(newFakeCertManagerClient(nil, nil).WithCertManagerPlan(certManagerPlan))

	client := newFakeClient(config1).
		WithRepository(repository1).
		WithCluster(cluster1)

	tests := []struct {
		name      string
		client    *fakeClient
		expectErr bool
	}{
		{
			name:      "returns plan for upgrading cert-manager",
			client:    client,
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			options := PlanUpgradeOptions{
				Kubeconfig: Kubeconfig{Path: "cluster1"},
			}
			actualPlan, err := tt.client.PlanCertManagerUpgrade(options)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				g.Expect(actualPlan).To(Equal(CertManagerUpgradePlan{}))
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(actualPlan).To(Equal(certManagerPlan))
		})
	}
}

func Test_clusterctlClient_PlanUpgrade(t *testing.T) {
	type fields struct {
		client *fakeClient
	}
	type args struct {
		options PlanUpgradeOptions
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "does not return error if cluster client is found",
			fields: fields{
				client: fakeClientForUpgrade(), // core v1.0.0 (v1.0.1 available), infra v2.0.0 (v2.0.1 available)
			},
			args: args{
				options: PlanUpgradeOptions{
					Kubeconfig: Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"},
				},
			},
			wantErr: false,
		},
		{
			name: "returns an error if cluster client is not found",
			fields: fields{
				client: fakeClientForUpgrade(), // core v1.0.0 (v1.0.1 available), infra v2.0.0 (v2.0.1 available)
			},
			args: args{
				options: PlanUpgradeOptions{
					Kubeconfig: Kubeconfig{Path: "kubeconfig", Context: "some-other-context"},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			_, err := tt.fields.client.PlanUpgrade(tt.args.options)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())
		})
	}
}

func Test_clusterctlClient_ApplyUpgrade(t *testing.T) {
	type fields struct {
		client *fakeClient
	}
	type args struct {
		options ApplyUpgradeOptions
	}
	tests := []struct {
		name          string
		fields        fields
		args          args
		wantProviders *clusterctlv1.ProviderList
		wantErr       bool
	}{
		{
			name: "apply a plan",
			fields: fields{
				client: fakeClientForUpgrade(), // core v1.0.0 (v1.0.1 available), infra v2.0.0 (v2.0.1 available)
			},
			args: args{
				options: ApplyUpgradeOptions{
					Kubeconfig:              Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"},
					Contract:                test.CurrentCAPIContract,
					CoreProvider:            "",
					BootstrapProviders:      nil,
					ControlPlaneProviders:   nil,
					InfrastructureProviders: nil,
				},
			},
			wantProviders: &clusterctlv1.ProviderList{
				TypeMeta: metav1.TypeMeta{
					APIVersion: clusterctlv1.GroupVersion.String(),
					Kind:       "ProviderList",
				},
				ListMeta: metav1.ListMeta{},
				Items: []clusterctlv1.Provider{ // both providers should be upgraded
					fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.1", "cluster-api-system"),
					fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.1", "infra-system"),
				},
			},
			wantErr: false,
		},
		{
			name: "apply a custom plan - core provider only",
			fields: fields{
				client: fakeClientForUpgrade(), // core v1.0.0 (v1.0.1 available), infra v2.0.0 (v2.0.1 available)
			},
			args: args{
				options: ApplyUpgradeOptions{
					Kubeconfig:              Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"},
					Contract:                "",
					CoreProvider:            "cluster-api-system/cluster-api:v1.0.1",
					BootstrapProviders:      nil,
					ControlPlaneProviders:   nil,
					InfrastructureProviders: nil,
				},
			},
			wantProviders: &clusterctlv1.ProviderList{
				TypeMeta: metav1.TypeMeta{
					APIVersion: clusterctlv1.GroupVersion.String(),
					Kind:       "ProviderList",
				},
				ListMeta: metav1.ListMeta{},
				Items: []clusterctlv1.Provider{ // only one provider should be upgraded
					fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.1", "cluster-api-system"),
					fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
				},
			},
			wantErr: false,
		},
		{
			name: "apply a custom plan - infra provider only",
			fields: fields{
				client: fakeClientForUpgrade(), // core v1.0.0 (v1.0.1 available), infra v2.0.0 (v2.0.1 available)
			},
			args: args{
				options: ApplyUpgradeOptions{
					Kubeconfig:              Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"},
					Contract:                "",
					CoreProvider:            "",
					BootstrapProviders:      nil,
					ControlPlaneProviders:   nil,
					InfrastructureProviders: []string{"infra-system/infra:v2.0.1"},
				},
			},
			wantProviders: &clusterctlv1.ProviderList{
				TypeMeta: metav1.TypeMeta{
					APIVersion: clusterctlv1.GroupVersion.String(),
					Kind:       "ProviderList",
				},
				ListMeta: metav1.ListMeta{},
				Items: []clusterctlv1.Provider{ // only one provider should be upgraded
					fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system"),
					fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.1", "infra-system"),
				},
			},
			wantErr: false,
		},
		{
			name: "apply a custom plan - both providers",
			fields: fields{
				client: fakeClientForUpgrade(), // core v1.0.0 (v1.0.1 available), infra v2.0.0 (v2.0.1 available)
			},
			args: args{
				options: ApplyUpgradeOptions{
					Kubeconfig:              Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"},
					Contract:                "",
					CoreProvider:            "cluster-api-system/cluster-api:v1.0.1",
					BootstrapProviders:      nil,
					ControlPlaneProviders:   nil,
					InfrastructureProviders: []string{"infra-system/infra:v2.0.1"},
				},
			},
			wantProviders: &clusterctlv1.ProviderList{
				TypeMeta: metav1.TypeMeta{
					APIVersion: clusterctlv1.GroupVersion.String(),
					Kind:       "ProviderList",
				},
				ListMeta: metav1.ListMeta{},
				Items: []clusterctlv1.Provider{
					fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.1", "cluster-api-system"),
					fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.1", "infra-system"),
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			err := tt.fields.client.ApplyUpgrade(tt.args.options)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())

			// converting between client and cluster alias for Kubeconfig
			input := cluster.Kubeconfig(tt.args.options.Kubeconfig)
			proxy := tt.fields.client.clusters[input].Proxy()
			gotProviders := &clusterctlv1.ProviderList{}

			c, err := proxy.NewClient()
			g.Expect(err).NotTo(HaveOccurred())

			g.Expect(c.List(ctx, gotProviders)).To(Succeed())

			sort.Slice(gotProviders.Items, func(i, j int) bool {
				return gotProviders.Items[i].Name < gotProviders.Items[j].Name
			})
			sort.Slice(tt.wantProviders.Items, func(i, j int) bool {
				return tt.wantProviders.Items[i].Name < tt.wantProviders.Items[j].Name
			})
			for i := range gotProviders.Items {
				tt.wantProviders.Items[i].ResourceVersion = gotProviders.Items[i].ResourceVersion
			}
			g.Expect(gotProviders).To(Equal(tt.wantProviders), cmp.Diff(gotProviders, tt.wantProviders))
		})
	}
}

func fakeClientForUpgrade() *fakeClient {
	core := config.NewProvider("cluster-api", "https://somewhere.com", clusterctlv1.CoreProviderType)
	infra := config.NewProvider("infra", "https://somewhere.com", clusterctlv1.InfrastructureProviderType)

	config1 := newFakeConfig().
		WithProvider(core).
		WithProvider(infra)

	repository1 := newFakeRepository(core, config1).
		WithPaths("root", "components.yaml").
		WithDefaultVersion("v1.0.1").
		WithFile("v1.0.1", "components.yaml", componentsYAML("ns2")).
		WithVersions("v1.0.0", "v1.0.1").
		WithMetadata("v1.0.1", &clusterctlv1.Metadata{
			ReleaseSeries: []clusterctlv1.ReleaseSeries{
				{Major: 1, Minor: 0, Contract: test.CurrentCAPIContract},
			},
		})
	repository2 := newFakeRepository(infra, config1).
		WithPaths("root", "components.yaml").
		WithDefaultVersion("v2.0.0").
		WithFile("v2.0.1", "components.yaml", componentsYAML("ns2")).
		WithVersions("v2.0.0", "v2.0.1").
		WithMetadata("v2.0.1", &clusterctlv1.Metadata{
			ReleaseSeries: []clusterctlv1.ReleaseSeries{
				{Major: 2, Minor: 0, Contract: test.CurrentCAPIContract},
			},
		})

	cluster1 := newFakeCluster(cluster.Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"}, config1).
		WithRepository(repository1).
		WithRepository(repository2).
		WithProviderInventory(core.Name(), core.Type(), "v1.0.0", "cluster-api-system").
		WithProviderInventory(infra.Name(), infra.Type(), "v2.0.0", "infra-system").
		WithObjs(test.FakeCAPISetupObjects()...)

	client := newFakeClient(config1).
		WithRepository(repository1).
		WithRepository(repository2).
		WithCluster(cluster1)

	return client
}

func fakeProvider(name string, providerType clusterctlv1.ProviderType, version, targetNamespace string) clusterctlv1.Provider {
	return clusterctlv1.Provider{
		TypeMeta: metav1.TypeMeta{
			APIVersion: clusterctlv1.GroupVersion.String(),
			Kind:       "Provider",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: targetNamespace,
			Name:      clusterctlv1.ManifestLabel(name, providerType),
			Labels: map[string]string{
				clusterctlv1.ClusterctlLabelName:     "",
				clusterv1.ProviderLabelName:          clusterctlv1.ManifestLabel(name, providerType),
				clusterctlv1.ClusterctlCoreLabelName: clusterctlv1.ClusterctlCoreLabelInventoryValue,
			},
		},
		ProviderName:     name,
		Type:             string(providerType),
		Version:          version,
		WatchedNamespace: "",
	}
}

func Test_parseUpgradeItem(t *testing.T) {
	type args struct {
		provider string
	}
	tests := []struct {
		name    string
		args    args
		want    *cluster.UpgradeItem
		wantErr bool
	}{
		{
			name: "namespace/provider",
			args: args{
				provider: "namespace/provider",
			},
			want: &cluster.UpgradeItem{
				Provider: clusterctlv1.Provider{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      clusterctlv1.ManifestLabel("provider", clusterctlv1.CoreProviderType),
					},
					ProviderName: "provider",
					Type:         string(clusterctlv1.CoreProviderType),
				},
				NextVersion: "",
			},
			wantErr: false,
		},
		{
			name: "namespace/provider:version",
			args: args{
				provider: "namespace/provider:version",
			},
			want: &cluster.UpgradeItem{
				Provider: clusterctlv1.Provider{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      clusterctlv1.ManifestLabel("provider", clusterctlv1.CoreProviderType),
					},
					ProviderName: "provider",
					Type:         string(clusterctlv1.CoreProviderType),
				},
				NextVersion: "version",
			},
			wantErr: false,
		},
		{
			name: "namespace missing",
			args: args{
				provider: "provider:version",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "namespace empty",
			args: args{
				provider: "/provider:version",
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := parseUpgradeItem(tt.args.provider, clusterctlv1.CoreProviderType)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())

			g.Expect(got).To(Equal(tt.want))
		})
	}
}
