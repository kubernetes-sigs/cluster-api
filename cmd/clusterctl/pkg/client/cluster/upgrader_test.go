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

package cluster

import (
	"reflect"
	"testing"

	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/repository"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/test"
)

func Test_providerUpgrader_Plan(t *testing.T) {
	type fields struct {
		reader     config.Reader
		repository map[string]repository.Repository
		proxy      Proxy
	}
	tests := []struct {
		name    string
		fields  fields
		want    []UpgradePlan
		wantErr bool
	}{
		{
			name: "Single Management group, no multi-tenancy, upgrade within the same contact",
			fields: fields{
				// config for two providers
				reader: test.NewFakeReader().
					WithProvider("core", clusterctlv1.CoreProviderType, "https://somewhere.com").
					WithProvider("infra", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				// two provider repositories, each with a new version in the v1alpha3 contract
				repository: map[string]repository.Repository{
					"core": test.NewFakeRepository().
						WithVersions("v1.0.0", "v1.0.1").
						WithMetadata("v1.0.1", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 0, Contract: "v1alpha3"},
							},
						}),
					"infra": test.NewFakeRepository().
						WithVersions("v2.0.0", "v2.0.1").
						WithMetadata("v2.0.1", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 2, Minor: 0, Contract: "v1alpha3"},
							},
						}),
				},
				// two providers existing in the cluster
				proxy: test.NewFakeProxy().
					WithProviderInventory("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system", "").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system", ""),
			},
			want: []UpgradePlan{
				{ // one upgrade plan with the latest releases the v1alpha3 contract
					Contract:     "v1alpha3",
					CoreProvider: fakeProvider("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system", ""),
					Providers: []UpgradeItem{
						{
							Provider:    fakeProvider("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system", ""),
							NextVersion: "v1.0.1",
						},
						{
							Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system", ""),
							NextVersion: "v2.0.1",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Single Management group, no multi-tenancy, upgrade for two contacts",
			fields: fields{
				// config for two providers
				reader: test.NewFakeReader().
					WithProvider("core", clusterctlv1.CoreProviderType, "https://somewhere.com").
					WithProvider("infra", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				// two provider repositories, each with a new version for current v1alpha3 contract and a new version for the v1alpha4 contract
				repository: map[string]repository.Repository{
					"core": test.NewFakeRepository().
						WithVersions("v1.0.0", "v1.0.1", "v2.0.0").
						WithMetadata("v2.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 0, Contract: "v1alpha3"},
								{Major: 2, Minor: 0, Contract: "v1alpha4"},
							},
						}),
					"infra": test.NewFakeRepository().
						WithVersions("v2.0.0", "v2.0.1", "v3.0.0").
						WithMetadata("v3.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 2, Minor: 0, Contract: "v1alpha3"},
								{Major: 3, Minor: 0, Contract: "v1alpha4"},
							},
						}),
				},
				// two providers existing in the cluster
				proxy: test.NewFakeProxy().
					WithProviderInventory("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system", "").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system", ""),
			},
			want: []UpgradePlan{
				{ // one upgrade plan with the latest releases in the v1alpha3 contract
					Contract:     "v1alpha3",
					CoreProvider: fakeProvider("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system", ""),
					Providers: []UpgradeItem{
						{
							Provider:    fakeProvider("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system", ""),
							NextVersion: "v1.0.1",
						},
						{
							Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system", ""),
							NextVersion: "v2.0.1",
						},
					},
				},
				{ // one upgrade plan with the latest releases in the v1alpha4 contract
					Contract:     "v1alpha4",
					CoreProvider: fakeProvider("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system", ""),
					Providers: []UpgradeItem{
						{
							Provider:    fakeProvider("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system", ""),
							NextVersion: "v2.0.0",
						},
						{
							Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system", ""),
							NextVersion: "v3.0.0",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Single Management group, n-Infra multi-tenancy, upgrade within the same contact",
			fields: fields{
				// config for two providers
				reader: test.NewFakeReader().
					WithProvider("core", clusterctlv1.CoreProviderType, "https://somewhere.com").
					WithProvider("infra", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				// two provider repositories, each with a new version in the v1alpha3 contract
				repository: map[string]repository.Repository{
					"core": test.NewFakeRepository().
						WithVersions("v1.0.0", "v1.0.1").
						WithMetadata("v1.0.1", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 0, Contract: "v1alpha3"},
							},
						}),
					"infra": test.NewFakeRepository().
						WithVersions("v2.0.0", "v2.0.1").
						WithMetadata("v2.0.1", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 2, Minor: 0, Contract: "v1alpha3"},
							},
						}),
				},
				// one core and two infra providers existing in the cluster
				proxy: test.NewFakeProxy().
					WithProviderInventory("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system", "").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system1", "ns1").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system2", "ns2"),
			},
			want: []UpgradePlan{
				{ // one upgrade plan with the latest releases in the v1alpha3 contract
					Contract:     "v1alpha3",
					CoreProvider: fakeProvider("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system", ""),
					Providers: []UpgradeItem{
						{
							Provider:    fakeProvider("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system", ""),
							NextVersion: "v1.0.1",
						},
						{
							Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system1", "ns1"),
							NextVersion: "v2.0.1",
						},
						{
							Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system2", "ns2"),
							NextVersion: "v2.0.1",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Single Management group, n-Infra multi-tenancy, upgrade for two contacts",
			fields: fields{
				// config for two providers
				reader: test.NewFakeReader().
					WithProvider("core", clusterctlv1.CoreProviderType, "https://somewhere.com").
					WithProvider("infra", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				// two provider repositories, each with a new version for current v1alpha3 contract and a new version for the v1alpha4 contract
				repository: map[string]repository.Repository{
					"core": test.NewFakeRepository().
						WithVersions("v1.0.0", "v1.0.1", "v2.0.0").
						WithMetadata("v2.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 0, Contract: "v1alpha3"},
								{Major: 2, Minor: 0, Contract: "v1alpha4"},
							},
						}),
					"infra": test.NewFakeRepository().
						WithVersions("v2.0.0", "v2.0.1", "v3.0.0").
						WithMetadata("v3.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 2, Minor: 0, Contract: "v1alpha3"},
								{Major: 3, Minor: 0, Contract: "v1alpha4"},
							},
						}),
				},
				// one core and two infra providers existing in the cluster
				proxy: test.NewFakeProxy().
					WithProviderInventory("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system", "").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system1", "ns1").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system2", "ns2"),
			},
			want: []UpgradePlan{
				{ // one upgrade plan with the latest releases in the v1alpha3 contract
					Contract:     "v1alpha3",
					CoreProvider: fakeProvider("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system", ""),
					Providers: []UpgradeItem{
						{
							Provider:    fakeProvider("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system", ""),
							NextVersion: "v1.0.1",
						},
						{
							Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system1", "ns1"),
							NextVersion: "v2.0.1",
						},
						{
							Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system2", "ns2"),
							NextVersion: "v2.0.1",
						},
					},
				},
				{ // one upgrade plan with the latest releases in the v1alpha4 contract
					Contract:     "v1alpha4",
					CoreProvider: fakeProvider("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system", ""),
					Providers: []UpgradeItem{
						{
							Provider:    fakeProvider("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system", ""),
							NextVersion: "v2.0.0",
						},
						{
							Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system1", "ns1"),
							NextVersion: "v3.0.0",
						},
						{
							Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system2", "ns2"),
							NextVersion: "v3.0.0",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Single Management group, n-Core multi-tenancy, upgrade within the same contact",
			fields: fields{
				// config for two providers
				reader: test.NewFakeReader().
					WithProvider("core", clusterctlv1.CoreProviderType, "https://somewhere.com").
					WithProvider("infra", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				// two provider repositories, each with a new version in the v1alpha3 contract
				repository: map[string]repository.Repository{
					"core": test.NewFakeRepository().
						WithVersions("v1.0.0", "v1.0.1").
						WithMetadata("v1.0.1", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 0, Contract: "v1alpha3"},
							},
						}),
					"infra": test.NewFakeRepository().
						WithVersions("v2.0.0", "v2.0.1").
						WithMetadata("v2.0.1", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 2, Minor: 0, Contract: "v1alpha3"},
							},
						}),
				},
				// two management groups existing in the cluster
				proxy: test.NewFakeProxy().
					WithProviderInventory("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system1", "ns1").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system1", "ns1").
					WithProviderInventory("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system2", "ns2").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system2", "ns2"),
			},
			want: []UpgradePlan{
				{ // one upgrade plan with the latest releases in the v1alpha3 contract for the first management group
					Contract:     "v1alpha3",
					CoreProvider: fakeProvider("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system1", "ns1"),
					Providers: []UpgradeItem{
						{
							Provider:    fakeProvider("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system1", "ns1"),
							NextVersion: "v1.0.1",
						},
						{
							Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system1", "ns1"),
							NextVersion: "v2.0.1",
						},
					},
				},
				{ // one upgrade plan with the latest releases in the v1alpha3 contract for the second management group
					Contract:     "v1alpha3",
					CoreProvider: fakeProvider("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system2", "ns2"),
					Providers: []UpgradeItem{
						{
							Provider:    fakeProvider("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system2", "ns2"),
							NextVersion: "v1.0.1",
						},
						{
							Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system2", "ns2"),
							NextVersion: "v2.0.1",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Single Management group, n-Core multi-tenancy, upgrade for two contacts",
			fields: fields{
				// config for two providers
				reader: test.NewFakeReader().
					WithProvider("core", clusterctlv1.CoreProviderType, "https://somewhere.com").
					WithProvider("infra", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				// two provider repositories, each with a new version for current v1alpha3 contract and a new version for the v1alpha4 contract
				repository: map[string]repository.Repository{
					"core": test.NewFakeRepository().
						WithVersions("v1.0.0", "v1.0.1", "v2.0.0").
						WithMetadata("v2.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 0, Contract: "v1alpha3"},
								{Major: 2, Minor: 0, Contract: "v1alpha4"},
							},
						}),
					"infra": test.NewFakeRepository().
						WithVersions("v2.0.0", "v2.0.1", "v3.0.0").
						WithMetadata("v3.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 2, Minor: 0, Contract: "v1alpha3"},
								{Major: 3, Minor: 0, Contract: "v1alpha4"},
							},
						}),
				},
				// two management groups existing in the cluster
				proxy: test.NewFakeProxy().
					WithProviderInventory("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system1", "ns1").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system1", "ns1").
					WithProviderInventory("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system2", "ns2").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system2", "ns2"),
			},
			want: []UpgradePlan{
				{ // one upgrade plan with the latest releases in the v1alpha3 contract for the first management group
					Contract:     "v1alpha3",
					CoreProvider: fakeProvider("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system1", "ns1"),
					Providers: []UpgradeItem{
						{
							Provider:    fakeProvider("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system1", "ns1"),
							NextVersion: "v1.0.1",
						},
						{
							Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system1", "ns1"),
							NextVersion: "v2.0.1",
						},
					},
				},
				{ // one upgrade plan with the latest releases in the v1alpha4 contract for the first management group
					Contract:     "v1alpha4",
					CoreProvider: fakeProvider("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system1", "ns1"),
					Providers: []UpgradeItem{
						{
							Provider:    fakeProvider("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system1", "ns1"),
							NextVersion: "v2.0.0",
						},
						{
							Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system1", "ns1"),
							NextVersion: "v3.0.0",
						},
					},
				},
				{ // one upgrade plan with the latest releases in the v1alpha3 contract for the second management group
					Contract:     "v1alpha3",
					CoreProvider: fakeProvider("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system2", "ns2"),
					Providers: []UpgradeItem{
						{
							Provider:    fakeProvider("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system2", "ns2"),
							NextVersion: "v1.0.1",
						},
						{
							Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system2", "ns2"),
							NextVersion: "v2.0.1",
						},
					},
				},
				{ // one upgrade plan with the latest releases in the v1alpha4 contract for the second management group
					Contract:     "v1alpha4",
					CoreProvider: fakeProvider("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system2", "ns2"),
					Providers: []UpgradeItem{
						{
							Provider:    fakeProvider("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system2", "ns2"),
							NextVersion: "v2.0.0",
						},
						{
							Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system2", "ns2"),
							NextVersion: "v3.0.0",
						},
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configClient, _ := config.New("", config.InjectReader(tt.fields.reader))

			u := &providerUpgrader{
				configClient: configClient,
				repositoryClientFactory: func(provider config.Provider, configVariablesClient config.VariablesClient, options ...repository.Option) (repository.Client, error) {
					return repository.New(provider, configVariablesClient, repository.InjectRepository(tt.fields.repository[provider.Name()]))
				},
				providerInventory: newInventoryClient(tt.fields.proxy, nil),
			}
			got, err := u.Plan()
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
