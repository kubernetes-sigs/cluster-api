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
	"testing"

	. "github.com/onsi/gomega"

	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/repository"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
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
			name: "Single Management group, no multi-tenancy, upgrade within the same contract",
			fields: fields{
				// config for two providers
				reader: test.NewFakeReader().
					WithProvider("cluster-api", clusterctlv1.CoreProviderType, "https://somewhere.com").
					WithProvider("infra", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				// two provider repositories, each with a new version in the v1alpha3 contract
				repository: map[string]repository.Repository{
					"cluster-api": test.NewFakeRepository().
						WithVersions("v1.0.0", "v1.0.1").
						WithMetadata("v1.0.1", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 0, Contract: "v1alpha3"},
							},
						}),
					"infrastructure-infra": test.NewFakeRepository().
						WithVersions("v2.0.0", "v2.0.1").
						WithMetadata("v2.0.1", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 2, Minor: 0, Contract: "v1alpha3"},
							},
						}),
				},
				// two providers existing in the cluster
				proxy: test.NewFakeProxy().
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", "").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system", ""),
			},
			want: []UpgradePlan{
				{ // one upgrade plan with the latest releases the v1alpha3 contract
					Contract:     "v1alpha3",
					CoreProvider: fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", ""),
					Providers: []UpgradeItem{
						{
							Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", ""),
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
			name: "pre-releases should be ignored",
			fields: fields{
				// config for two providers
				reader: test.NewFakeReader().
					WithProvider("cluster-api", clusterctlv1.CoreProviderType, "https://somewhere.com").
					WithProvider("infra", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				// two provider repositories, each with a new version in the v1alpha3 contract
				repository: map[string]repository.Repository{
					"cluster-api": test.NewFakeRepository().
						WithVersions("v1.0.0", "v1.0.1").
						WithMetadata("v1.0.1", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 0, Contract: "v1alpha3"},
							},
						}),
					"infrastructure-infra": test.NewFakeRepository().
						WithVersions("v2.0.0", "v2.0.1", "v3.0.0-alpha.0").
						WithMetadata("v2.0.1", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 2, Minor: 0, Contract: "v1alpha3"},
							},
						}).
						WithMetadata("v3.0.0-alpha.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 2, Minor: 0, Contract: "v1alpha3"},
								{Major: 3, Minor: 0, Contract: "v1alpha3"},
							},
						}),
				},
				// two providers existing in the cluster
				proxy: test.NewFakeProxy().
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", "").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system", ""),
			},
			want: []UpgradePlan{
				{ // one upgrade plan with the latest releases the v1alpha3 contract
					Contract:     "v1alpha3",
					CoreProvider: fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", ""),
					Providers: []UpgradeItem{
						{
							Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", ""),
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
			name: "Single Management group, no multi-tenancy, upgrade for two contracts",
			fields: fields{
				// config for two providers
				reader: test.NewFakeReader().
					WithProvider("cluster-api", clusterctlv1.CoreProviderType, "https://somewhere.com").
					WithProvider("infra", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				// two provider repositories, each with a new version for current v1alpha3 contract and a new version for the v1alpha4 contract
				repository: map[string]repository.Repository{
					"cluster-api": test.NewFakeRepository().
						WithVersions("v1.0.0", "v1.0.1", "v2.0.0").
						WithMetadata("v2.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 0, Contract: "v1alpha3"},
								{Major: 2, Minor: 0, Contract: "v1alpha4"},
							},
						}),
					"infrastructure-infra": test.NewFakeRepository().
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
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", "").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system", ""),
			},
			want: []UpgradePlan{
				{ // one upgrade plan with the latest releases in the v1alpha3 contract
					Contract:     "v1alpha3",
					CoreProvider: fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", ""),
					Providers: []UpgradeItem{
						{
							Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", ""),
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
					CoreProvider: fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", ""),
					Providers: []UpgradeItem{
						{
							Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", ""),
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
			name: "Single Management group, n-Infra multi-tenancy, upgrade within the same contract",
			fields: fields{
				// config for two providers
				reader: test.NewFakeReader().
					WithProvider("cluster-api", clusterctlv1.CoreProviderType, "https://somewhere.com").
					WithProvider("infra", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				// two provider repositories, each with a new version in the v1alpha3 contract
				repository: map[string]repository.Repository{
					"cluster-api": test.NewFakeRepository().
						WithVersions("v1.0.0", "v1.0.1").
						WithMetadata("v1.0.1", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 0, Contract: "v1alpha3"},
							},
						}),
					"infrastructure-infra": test.NewFakeRepository().
						WithVersions("v2.0.0", "v2.0.1").
						WithMetadata("v2.0.1", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 2, Minor: 0, Contract: "v1alpha3"},
							},
						}),
				},
				// one core and two infra providers existing in the cluster
				proxy: test.NewFakeProxy().
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", "").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system1", "ns1").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system2", "ns2"),
			},
			want: []UpgradePlan{
				{ // one upgrade plan with the latest releases in the v1alpha3 contract
					Contract:     "v1alpha3",
					CoreProvider: fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", ""),
					Providers: []UpgradeItem{
						{
							Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", ""),
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
			name: "Single Management group, n-Infra multi-tenancy, upgrade for two contracts",
			fields: fields{
				// config for two providers
				reader: test.NewFakeReader().
					WithProvider("cluster-api", clusterctlv1.CoreProviderType, "https://somewhere.com").
					WithProvider("infra", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				// two provider repositories, each with a new version for current v1alpha3 contract and a new version for the v1alpha4 contract
				repository: map[string]repository.Repository{
					"cluster-api": test.NewFakeRepository().
						WithVersions("v1.0.0", "v1.0.1", "v2.0.0").
						WithMetadata("v2.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 0, Contract: "v1alpha3"},
								{Major: 2, Minor: 0, Contract: "v1alpha4"},
							},
						}),
					"infrastructure-infra": test.NewFakeRepository().
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
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", "").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system1", "ns1").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system2", "ns2"),
			},
			want: []UpgradePlan{
				{ // one upgrade plan with the latest releases in the v1alpha3 contract
					Contract:     "v1alpha3",
					CoreProvider: fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", ""),
					Providers: []UpgradeItem{
						{
							Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", ""),
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
					CoreProvider: fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", ""),
					Providers: []UpgradeItem{
						{
							Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", ""),
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
			name: "Single Management group, n-Core multi-tenancy, upgrade within the same contract",
			fields: fields{
				// config for two providers
				reader: test.NewFakeReader().
					WithProvider("cluster-api", clusterctlv1.CoreProviderType, "https://somewhere.com").
					WithProvider("infra", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				// two provider repositories, each with a new version in the v1alpha3 contract
				repository: map[string]repository.Repository{
					"cluster-api": test.NewFakeRepository().
						WithVersions("v1.0.0", "v1.0.1").
						WithMetadata("v1.0.1", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 0, Contract: "v1alpha3"},
							},
						}),
					"infrastructure-infra": test.NewFakeRepository().
						WithVersions("v2.0.0", "v2.0.1").
						WithMetadata("v2.0.1", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 2, Minor: 0, Contract: "v1alpha3"},
							},
						}),
				},
				// two management groups existing in the cluster
				proxy: test.NewFakeProxy().
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system1", "ns1").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system1", "ns1").
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system2", "ns2").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system2", "ns2"),
			},
			want: []UpgradePlan{
				{ // one upgrade plan with the latest releases in the v1alpha3 contract for the first management group
					Contract:     "v1alpha3",
					CoreProvider: fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system1", "ns1"),
					Providers: []UpgradeItem{
						{
							Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system1", "ns1"),
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
					CoreProvider: fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system2", "ns2"),
					Providers: []UpgradeItem{
						{
							Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system2", "ns2"),
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
			name: "Single Management group, n-Core multi-tenancy, upgrade for two contracts",
			fields: fields{
				// config for two providers
				reader: test.NewFakeReader().
					WithProvider("cluster-api", clusterctlv1.CoreProviderType, "https://somewhere.com").
					WithProvider("infra", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				// two provider repositories, each with a new version for current v1alpha3 contract and a new version for the v1alpha4 contract
				repository: map[string]repository.Repository{
					"cluster-api": test.NewFakeRepository().
						WithVersions("v1.0.0", "v1.0.1", "v2.0.0").
						WithMetadata("v2.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 0, Contract: "v1alpha3"},
								{Major: 2, Minor: 0, Contract: "v1alpha4"},
							},
						}),
					"infrastructure-infra": test.NewFakeRepository().
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
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system1", "ns1").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system1", "ns1").
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system2", "ns2").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system2", "ns2"),
			},
			want: []UpgradePlan{
				{ // one upgrade plan with the latest releases in the v1alpha3 contract for the first management group
					Contract:     "v1alpha3",
					CoreProvider: fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system1", "ns1"),
					Providers: []UpgradeItem{
						{
							Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system1", "ns1"),
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
					CoreProvider: fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system1", "ns1"),
					Providers: []UpgradeItem{
						{
							Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system1", "ns1"),
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
					CoreProvider: fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system2", "ns2"),
					Providers: []UpgradeItem{
						{
							Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system2", "ns2"),
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
					CoreProvider: fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system2", "ns2"),
					Providers: []UpgradeItem{
						{
							Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system2", "ns2"),
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
		{
			name: "Single Management group, no multi-tenancy, upgrade for two contracts, but the upgrade for the second one is partially available",
			fields: fields{
				// config for two providers
				reader: test.NewFakeReader().
					WithProvider("cluster-api", clusterctlv1.CoreProviderType, "https://somewhere.com").
					WithProvider("infra", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				// two provider repositories, the first with a new version for current v1alpha3 contract and a new version for the v1alpha4 contract, the second without new releases
				repository: map[string]repository.Repository{
					"cluster-api": test.NewFakeRepository().
						WithVersions("v1.0.0", "v1.0.1", "v2.0.0").
						WithMetadata("v2.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 0, Contract: "v1alpha3"},
								{Major: 2, Minor: 0, Contract: "v1alpha4"},
							},
						}),
					"infrastructure-infra": test.NewFakeRepository().
						WithVersions("v2.0.0"). // no v1alpha3 or v1alpha3 new releases yet available for the infra provider (only the current release exists)
						WithMetadata("v2.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 2, Minor: 0, Contract: "v1alpha3"},
							},
						}),
				},
				// two providers existing in the cluster
				proxy: test.NewFakeProxy().
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", "").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system", ""),
			},
			want: []UpgradePlan{
				{ // one upgrade plan with the latest releases in the v1alpha3 contract
					Contract:     "v1alpha3",
					CoreProvider: fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", ""),
					Providers: []UpgradeItem{
						{
							Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", ""),
							NextVersion: "v1.0.1",
						},
						{
							Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system", ""),
							NextVersion: "", // we are already to the latest version for the infra provider in the v1alpha3 contract, but this is acceptable for the current contract
						},
					},
				},
				// the upgrade plan with the latest releases in the v1alpha4 contract should be dropped because all the provider are required to change the contract at the same time
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			configClient, _ := config.New("", config.InjectReader(tt.fields.reader))

			u := &providerUpgrader{
				configClient: configClient,
				repositoryClientFactory: func(provider config.Provider, configClient config.Client, options ...repository.Option) (repository.Client, error) {
					return repository.New(provider, configClient, repository.InjectRepository(tt.fields.repository[provider.ManifestLabel()]))
				},
				providerInventory: newInventoryClient(tt.fields.proxy, nil),
			}
			got, err := u.Plan()
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func Test_providerUpgrader_createCustomPlan(t *testing.T) {
	type fields struct {
		reader     config.Reader
		repository map[string]repository.Repository
		proxy      Proxy
	}
	type args struct {
		coreProvider       clusterctlv1.Provider
		providersToUpgrade []UpgradeItem
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *UpgradePlan
		wantErr bool
	}{
		{
			name: "pass if upgrade infra provider, same contract",
			fields: fields{
				// config for two providers
				reader: test.NewFakeReader().
					WithProvider("cluster-api", clusterctlv1.CoreProviderType, "https://somewhere.com").
					WithProvider("infra", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				// two provider repositories, each with a new version in the v1alpha3 contract
				repository: map[string]repository.Repository{
					"cluster-api": test.NewFakeRepository().
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
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", "").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system", ""),
			},
			args: args{
				coreProvider: fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "", "cluster-api-system", ""),
				providersToUpgrade: []UpgradeItem{
					{
						Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system", ""),
						NextVersion: "v2.0.1", // upgrade to next release in the v1alpha3 contract
					},
				},
			},
			want: &UpgradePlan{
				Contract:     "v1alpha3",
				CoreProvider: fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", ""),
				Providers: []UpgradeItem{
					{
						Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system", ""),
						NextVersion: "v2.0.1",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "pass if upgrade core provider, same contract",
			fields: fields{
				// config for two providers
				reader: test.NewFakeReader().
					WithProvider("cluster-api", clusterctlv1.CoreProviderType, "https://somewhere.com").
					WithProvider("infra", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				// two provider repositories, each with a new version in the v1alpha3 contract
				repository: map[string]repository.Repository{
					"cluster-api": test.NewFakeRepository().
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
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", "").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system", ""),
			},
			args: args{
				coreProvider: fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "", "cluster-api-system", ""),
				providersToUpgrade: []UpgradeItem{
					{
						Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", ""),
						NextVersion: "v1.0.1", // upgrade to next release in the v1alpha3 contract
					},
				},
			},
			want: &UpgradePlan{
				Contract:     "v1alpha3",
				CoreProvider: fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", ""),
				Providers: []UpgradeItem{
					{
						Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", ""),
						NextVersion: "v1.0.1",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "fail if upgrade infra provider, changing contract",
			fields: fields{
				// config for two providers
				reader: test.NewFakeReader().
					WithProvider("cluster-api", clusterctlv1.CoreProviderType, "https://somewhere.com").
					WithProvider("infra", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				// two provider repositories, each with current version in v1alpha3 and a new version in the v1alpha4 contract
				repository: map[string]repository.Repository{
					"cluster-api": test.NewFakeRepository().
						WithVersions("v1.0.0", "v2.0.0").
						WithMetadata("v2.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 0, Contract: "v1alpha3"},
								{Major: 2, Minor: 0, Contract: "v1alpha4"},
							},
						}),
					"infra": test.NewFakeRepository().
						WithVersions("v2.0.0", "v3.0.0").
						WithMetadata("v3.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 2, Minor: 0, Contract: "v1alpha3"},
								{Major: 3, Minor: 0, Contract: "v1alpha4"},
							},
						}),
				},
				// two providers existing in the cluster
				proxy: test.NewFakeProxy().
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", "").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system", ""),
			},
			args: args{
				coreProvider: fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "", "cluster-api-system", ""),
				providersToUpgrade: []UpgradeItem{
					{
						Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system", ""),
						NextVersion: "v3.0.0", // upgrade to next release in the v1alpha4 contract
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "fail if upgrade core provider, changing contract",
			fields: fields{
				// config for two providers
				reader: test.NewFakeReader().
					WithProvider("cluster-api", clusterctlv1.CoreProviderType, "https://somewhere.com").
					WithProvider("infra", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				// two provider repositories, each with current version in v1alpha3 and a new version in the v1alpha4 contract
				repository: map[string]repository.Repository{
					"cluster-api": test.NewFakeRepository().
						WithVersions("v1.0.0", "v2.0.0").
						WithMetadata("v2.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 0, Contract: "v1alpha3"},
								{Major: 2, Minor: 0, Contract: "v1alpha4"},
							},
						}),
					"infra": test.NewFakeRepository().
						WithVersions("v2.0.0", "v3.0.0").
						WithMetadata("v3.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 2, Minor: 0, Contract: "v1alpha3"},
								{Major: 3, Minor: 0, Contract: "v1alpha4"},
							},
						}),
				},
				// two providers existing in the cluster
				proxy: test.NewFakeProxy().
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", "").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system", ""),
			},
			args: args{
				coreProvider: fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "", "cluster-api-system", ""),
				providersToUpgrade: []UpgradeItem{
					{
						Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", ""),
						NextVersion: "v2.0.0", // upgrade to next release in the v1alpha4 contract
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "pass if upgrade core and infra provider, changing contract",
			fields: fields{
				// config for two providers
				reader: test.NewFakeReader().
					WithProvider("cluster-api", clusterctlv1.CoreProviderType, "https://somewhere.com").
					WithProvider("infra", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				// two provider repositories, each with current version in v1alpha3 and a new version in the v1alpha4 contract
				repository: map[string]repository.Repository{
					"cluster-api": test.NewFakeRepository().
						WithVersions("v1.0.0", "v2.0.0").
						WithMetadata("v2.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 0, Contract: "v1alpha3"},
								{Major: 2, Minor: 0, Contract: "v1alpha4"},
							},
						}),
					"infra": test.NewFakeRepository().
						WithVersions("v2.0.0", "v3.0.0").
						WithMetadata("v3.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 2, Minor: 0, Contract: "v1alpha3"},
								{Major: 3, Minor: 0, Contract: "v1alpha4"},
							},
						}),
				},
				// two providers existing in the cluster
				proxy: test.NewFakeProxy().
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", "").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system", ""),
			},
			args: args{
				coreProvider: fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "", "cluster-api-system", ""),
				providersToUpgrade: []UpgradeItem{
					{
						Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", ""),
						NextVersion: "v2.0.0", // upgrade to next release in the v1alpha4 contract
					},
					{
						Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system", ""),
						NextVersion: "v3.0.0", // upgrade to next release in the v1alpha4 contract
					},
				},
			},
			want: &UpgradePlan{
				Contract:     "v1alpha4",
				CoreProvider: fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", ""),
				Providers: []UpgradeItem{
					{
						Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", ""),
						NextVersion: "v2.0.0", // upgrade to next release in the v1alpha4 contract
					},
					{
						Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system", ""),
						NextVersion: "v3.0.0", // upgrade to next release in the v1alpha4 contract
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			configClient, _ := config.New("", config.InjectReader(tt.fields.reader))

			u := &providerUpgrader{
				configClient: configClient,
				repositoryClientFactory: func(provider config.Provider, configClient config.Client, options ...repository.Option) (repository.Client, error) {
					return repository.New(provider, configClient, repository.InjectRepository(tt.fields.repository[provider.Name()]))
				},
				providerInventory: newInventoryClient(tt.fields.proxy, nil),
			}
			got, err := u.createCustomPlan(tt.args.coreProvider, tt.args.providersToUpgrade)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(got).To(Equal(tt.want))
		})
	}
}
