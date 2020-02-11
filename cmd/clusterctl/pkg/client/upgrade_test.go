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
	"context"
	"reflect"
	"sort"
	"testing"

	"github.com/davecgh/go-spew/spew"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/cluster"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/config"
)

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
				client: fakeClientFoUpgrade(), // core v1.0.0 (v1.0.1 available), infra v2.0.0 (v2.0.1 available)
			},
			args: args{
				options: ApplyUpgradeOptions{
					Kubeconfig:      "kubeconfig",
					ManagementGroup: "core-system/core",
					Contract:        "v1alpha3",
				},
			},
			wantProviders: &clusterctlv1.ProviderList{
				TypeMeta: metav1.TypeMeta{
					APIVersion: clusterctlv1.GroupVersion.String(),
					Kind:       "ProviderList",
				},
				ListMeta: metav1.ListMeta{},
				Items: []clusterctlv1.Provider{ // both providers should be upgraded
					fakeProvider("core", clusterctlv1.CoreProviderType, "v1.0.1", "core-system"),
					fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.1", "infra-system"),
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fields.client.ApplyUpgrade(tt.args.options); (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			proxy := tt.fields.client.clusters["kubeconfig"].Proxy()
			gotProviders := &clusterctlv1.ProviderList{}

			c, err := proxy.NewClient()
			if err != nil {
				t.Fatalf("failed to create client %v", err)
			}

			if err := c.List(context.TODO(), gotProviders); err != nil {
				t.Fatalf("failed to read providers %v", err)
			}

			sort.Slice(gotProviders.Items, func(i, j int) bool {
				return gotProviders.Items[i].Name < gotProviders.Items[j].Name
			})
			sort.Slice(tt.wantProviders.Items, func(i, j int) bool {
				return tt.wantProviders.Items[i].Name < tt.wantProviders.Items[j].Name
			})
			for i := range gotProviders.Items {
				tt.wantProviders.Items[i].ResourceVersion = gotProviders.Items[i].ResourceVersion
			}

			if !reflect.DeepEqual(gotProviders, tt.wantProviders) {
				spew.Dump(gotProviders, tt.wantProviders)
				t.Errorf("got = %v, want %v", gotProviders, tt.wantProviders)
			}
		})
	}
}

func fakeClientFoUpgrade() *fakeClient {
	core := config.NewProvider("core", "https://somewhere.com", clusterctlv1.CoreProviderType)
	infra := config.NewProvider("infra", "https://somewhere.com", clusterctlv1.InfrastructureProviderType)

	config1 := newFakeConfig().
		WithProvider(core).
		WithProvider(infra)

	repository1 := newFakeRepository(core, config1.Variables()).
		WithPaths("root", "components.yaml").
		WithDefaultVersion("v1.0.1").
		WithFile("v1.0.1", "components.yaml", componentsYAML("ns2")).
		WithVersions("v1.0.0", "v1.0.1").
		WithMetadata("v1.0.1", &clusterctlv1.Metadata{
			ReleaseSeries: []clusterctlv1.ReleaseSeries{
				{Major: 1, Minor: 0, Contract: "v1alpha3"},
			},
		})
	repository2 := newFakeRepository(infra, config1.Variables()).
		WithPaths("root", "components.yaml").
		WithDefaultVersion("v2.0.0").
		WithFile("v2.0.1", "components.yaml", componentsYAML("ns2")).
		WithVersions("v2.0.0", "v2.0.1").
		WithMetadata("v2.0.1", &clusterctlv1.Metadata{
			ReleaseSeries: []clusterctlv1.ReleaseSeries{
				{Major: 2, Minor: 0, Contract: "v1alpha3"},
			},
		})

	cluster1 := newFakeCluster("kubeconfig", config1).
		WithRepository(repository1).
		WithRepository(repository2).
		WithProviderInventory(core.Name(), core.Type(), "v1.0.0", "core-system", "").
		WithProviderInventory(infra.Name(), infra.Type(), "v2.0.0", "infra-system", "")

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
			Name:      name,
			Labels: map[string]string{
				clusterctlv1.ClusterctlLabelName:     "",
				clusterv1.ProviderLabelName:          name,
				clusterctlv1.ClusterctlCoreLabelName: "inventory",
			},
		},
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
						Name:      "provider",
					},
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
						Name:      "provider",
					},
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
			got, err := parseUpgradeItem(tt.args.provider)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got = %v, want %v", got, tt.want)
			}

		})
	}
}
