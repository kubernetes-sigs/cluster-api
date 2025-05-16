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
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
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
			name: "Upgrade within the current contract",
			fields: fields{
				// config for two providers
				reader: test.NewFakeReader().
					WithProvider("cluster-api", clusterctlv1.CoreProviderType, "https://somewhere.com").
					WithProvider("infra", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				repository: map[string]repository.Repository{
					"cluster-api": repository.NewMemoryRepository().
						WithVersions("v1.0.0", "v1.0.1").
						WithMetadata("v1.0.1", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 0, Contract: currentContractVersion},
							},
						}),
					"infrastructure-infra": repository.NewMemoryRepository().
						WithVersions("v2.0.0", "v2.0.1").
						WithMetadata("v2.0.1", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 2, Minor: 0, Contract: currentContractVersion},
							},
						}),
				},
				// two providers existing in the cluster
				proxy: test.NewFakeProxy().
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
			},
			want: []UpgradePlan{
				{ // one upgrade plan with the latest releases the current contract
					Contract: currentContractVersion,
					Providers: []UpgradeItem{
						{
							Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system"),
							NextVersion: "v1.0.1",
						},
						{
							Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
							NextVersion: "v2.0.1",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Upgrade within the current and compatible contract",
			fields: fields{
				// config for two providers
				reader: test.NewFakeReader().
					WithProvider("cluster-api", clusterctlv1.CoreProviderType, "https://somewhere.com").
					WithProvider("infra", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				repository: map[string]repository.Repository{
					"cluster-api": repository.NewMemoryRepository().
						WithVersions("v1.0.0", "v1.0.1").
						WithMetadata("v1.0.1", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 0, Contract: currentContractVersion},
							},
						}),
					"infrastructure-infra": repository.NewMemoryRepository().
						WithVersions("v2.0.0", "v2.0.1").
						WithMetadata("v2.0.1", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 2, Minor: 0, Contract: oldContractVersionStillSupported},
							},
						}),
				},
				// two providers existing in the cluster
				proxy: test.NewFakeProxy().
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
			},
			want: []UpgradePlan{
				{ // one upgrade plan with the latest releases the current contract
					Contract: currentContractVersion,
					Providers: []UpgradeItem{
						{
							Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system"),
							NextVersion: "v1.0.1",
						},
						{
							Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
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
				repository: map[string]repository.Repository{
					"cluster-api": repository.NewMemoryRepository().
						WithVersions("v1.0.0", "v1.0.1").
						WithMetadata("v1.0.1", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 0, Contract: currentContractVersion},
							},
						}),
					"infrastructure-infra": repository.NewMemoryRepository().
						WithVersions("v2.0.0", "v2.0.1", "v3.0.0-alpha.0").
						WithMetadata("v2.0.1", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 2, Minor: 0, Contract: currentContractVersion},
							},
						}).
						WithMetadata("v3.0.0-alpha.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 2, Minor: 0, Contract: currentContractVersion},
								{Major: 3, Minor: 0, Contract: currentContractVersion},
							},
						}),
				},
				// two providers existing in the cluster
				proxy: test.NewFakeProxy().
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
			},
			want: []UpgradePlan{
				{ // one upgrade plan with the latest releases the current contract
					Contract: currentContractVersion,
					Providers: []UpgradeItem{
						{
							Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system"),
							NextVersion: "v1.0.1",
						},
						{
							Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
							NextVersion: "v2.0.1",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Upgrade for previous contract (not supported), current contract", // upgrade plan should report unsupported options
			fields: fields{
				// config for two providers
				reader: test.NewFakeReader().
					WithProvider("cluster-api", clusterctlv1.CoreProviderType, "https://somewhere.com").
					WithProvider("infra", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				repository: map[string]repository.Repository{
					"cluster-api": repository.NewMemoryRepository().
						WithVersions("v1.0.0", "v1.0.1", "v2.0.0").
						WithMetadata("v2.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 0, Contract: oldContractVersionNotSupportedAnymore},
								{Major: 2, Minor: 0, Contract: currentContractVersion},
							},
						}),
					"infrastructure-infra": repository.NewMemoryRepository().
						WithVersions("v2.0.0", "v2.0.1", "v3.0.0").
						WithMetadata("v3.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 2, Minor: 0, Contract: oldContractVersionNotSupportedAnymore},
								{Major: 3, Minor: 0, Contract: currentContractVersion},
							},
						}),
				},
				// two providers existing in the cluster
				proxy: test.NewFakeProxy().
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
			},
			want: []UpgradePlan{
				{ // one upgrade plan with the latest releases in the previous contract (not supported, but upgrade plan should report these options)
					Contract: oldContractVersionNotSupportedAnymore,
					Providers: []UpgradeItem{
						{
							Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system"),
							NextVersion: "v1.0.1",
						},
						{
							Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
							NextVersion: "v2.0.1",
						},
					},
				},
				{ // one upgrade plan with the latest releases in the current contract
					Contract: currentContractVersion,
					Providers: []UpgradeItem{
						{
							Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system"),
							NextVersion: "v2.0.0",
						},
						{
							Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
							NextVersion: "v3.0.0",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Upgrade for compatible contract, current contract", // upgrade plan should report unsupported options
			fields: fields{
				// config for two providers
				reader: test.NewFakeReader().
					WithProvider("cluster-api", clusterctlv1.CoreProviderType, "https://somewhere.com").
					WithProvider("infra", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				repository: map[string]repository.Repository{
					"cluster-api": repository.NewMemoryRepository().
						WithVersions("v1.0.0", "v1.0.1", "v2.0.0").
						WithMetadata("v2.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 0, Contract: oldContractVersionStillSupported},
								{Major: 2, Minor: 0, Contract: currentContractVersion},
							},
						}),
					"infrastructure-infra": repository.NewMemoryRepository().
						WithVersions("v2.0.0", "v2.0.1", "v3.0.0").
						WithMetadata("v3.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 2, Minor: 0, Contract: oldContractVersionStillSupported},
								{Major: 3, Minor: 0, Contract: oldContractVersionStillSupported},
							},
						}),
				},
				// two providers existing in the cluster
				proxy: test.NewFakeProxy().
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
			},
			want: []UpgradePlan{
				{ // one upgrade plan with the latest releases in the previous contract (not supported, but upgrade plan should report these options)
					Contract: oldContractVersionStillSupported,
					Providers: []UpgradeItem{
						{
							Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system"),
							NextVersion: "v1.0.1",
						},
						{
							Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
							NextVersion: "v3.0.0",
						},
					},
				},
				{ // one upgrade plan with the latest releases in the current contract
					Contract: currentContractVersion,
					Providers: []UpgradeItem{
						{
							Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system"),
							NextVersion: "v2.0.0",
						},
						{
							Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
							NextVersion: "v3.0.0",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Upgrade for both current contract and next contract (not supported)", // upgrade plan should report unsupported options
			fields: fields{
				// config for two providers
				reader: test.NewFakeReader().
					WithProvider("cluster-api", clusterctlv1.CoreProviderType, "https://somewhere.com").
					WithProvider("infra", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				repository: map[string]repository.Repository{
					"cluster-api": repository.NewMemoryRepository().
						WithVersions("v1.0.0", "v1.0.1", "v2.0.0").
						WithMetadata("v2.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 0, Contract: currentContractVersion},
								{Major: 2, Minor: 0, Contract: nextContractVersionNotSupportedYet},
							},
						}),
					"infrastructure-infra": repository.NewMemoryRepository().
						WithVersions("v2.0.0", "v2.0.1", "v3.0.0").
						WithMetadata("v3.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 2, Minor: 0, Contract: currentContractVersion},
								{Major: 3, Minor: 0, Contract: nextContractVersionNotSupportedYet},
							},
						}),
				},
				// two providers existing in the cluster
				proxy: test.NewFakeProxy().
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
			},
			want: []UpgradePlan{
				{ // one upgrade plan with the latest releases in the current
					Contract: currentContractVersion,
					Providers: []UpgradeItem{
						{
							Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system"),
							NextVersion: "v1.0.1",
						},
						{
							Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
							NextVersion: "v2.0.1",
						},
					},
				},
				{ // one upgrade plan with the latest releases in the next contract (not supported, but upgrade plan should report these options)
					Contract: nextContractVersionNotSupportedYet,
					Providers: []UpgradeItem{
						{
							Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system"),
							NextVersion: "v2.0.0",
						},
						{
							Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
							NextVersion: "v3.0.0",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Partial upgrades for next contract", // upgrade plan should report unsupported options
			fields: fields{
				// config for two providers
				reader: test.NewFakeReader().
					WithProvider("cluster-api", clusterctlv1.CoreProviderType, "https://somewhere.com").
					WithProvider("infra", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				repository: map[string]repository.Repository{
					"cluster-api": repository.NewMemoryRepository().
						WithVersions("v1.0.0", "v1.0.1", "v2.0.0").
						WithMetadata("v2.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 0, Contract: currentContractVersion},
								{Major: 2, Minor: 0, Contract: nextContractVersionNotSupportedYet},
							},
						}),
					"infrastructure-infra": repository.NewMemoryRepository().
						WithVersions("v2.0.0"). // no new releases available for the infra provider (only the current release exists)
						WithMetadata("v2.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 2, Minor: 0, Contract: nextContractVersionNotSupportedYet},
							},
						}),
				},
				// two providers existing in the cluster
				proxy: test.NewFakeProxy().
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
			},
			want: []UpgradePlan{
				{ // one upgrade plan with the latest releases in the current contract
					Contract: currentContractVersion,
					Providers: []UpgradeItem{
						{
							Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system"),
							NextVersion: "v1.0.1",
						},
						{
							Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
							NextVersion: "", // we are already to the latest version for the infra provider, but this is acceptable for the current contract
						},
					},
				},
				// the upgrade plan with the latest releases in the next contract should be dropped because all the provider are required to change the contract at the same time
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ctx := t.Context()

			configClient, _ := config.New(ctx, "", config.InjectReader(tt.fields.reader))

			u := &providerUpgrader{
				configClient: configClient,
				repositoryClientFactory: func(ctx context.Context, provider config.Provider, configClient config.Client, _ ...repository.Option) (repository.Client, error) {
					return repository.New(ctx, provider, configClient, repository.InjectRepository(tt.fields.repository[provider.ManifestLabel()]))
				},
				providerInventory:             newInventoryClient(tt.fields.proxy, nil, currentContractVersion),
				currentContractVersion:        currentContractVersion,
				getCompatibleContractVersions: getCompatibleContractVersions,
			}
			got, err := u.Plan(ctx)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(BeComparableTo(tt.want), cmp.Diff(got, tt.want))
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
				repository: map[string]repository.Repository{
					"cluster-api": repository.NewMemoryRepository().
						WithVersions("v1.0.0", "v1.0.1").
						WithMetadata("v1.0.1", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 0, Contract: currentContractVersion},
							},
						}),
					"infra": repository.NewMemoryRepository().
						WithVersions("v2.0.0", "v2.0.1").
						WithMetadata("v2.0.1", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 2, Minor: 0, Contract: currentContractVersion},
							},
						}),
				},
				// two providers existing in the cluster
				proxy: test.NewFakeProxy().
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
			},
			args: args{
				coreProvider: fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "", "cluster-api-system"),
				providersToUpgrade: []UpgradeItem{
					{
						Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
						NextVersion: "v2.0.1", // upgrade to next release in the current contract
					},
				},
			},
			want: &UpgradePlan{
				Contract: currentContractVersion,
				Providers: []UpgradeItem{
					{
						Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
						NextVersion: "v2.0.1",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "pass if upgrade infra provider, compatible contract",
			fields: fields{
				// config for two providers
				reader: test.NewFakeReader().
					WithProvider("cluster-api", clusterctlv1.CoreProviderType, "https://somewhere.com").
					WithProvider("infra", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				repository: map[string]repository.Repository{
					"cluster-api": repository.NewMemoryRepository().
						WithVersions("v1.0.0", "v1.0.1").
						WithMetadata("v1.0.1", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 0, Contract: currentContractVersion},
							},
						}),
					"infra": repository.NewMemoryRepository().
						WithVersions("v2.0.0", "v2.0.1").
						WithMetadata("v2.0.1", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 2, Minor: 0, Contract: oldContractVersionStillSupported},
							},
						}),
				},
				// two providers existing in the cluster
				proxy: test.NewFakeProxy().
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
			},
			args: args{
				coreProvider: fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "", "cluster-api-system"),
				providersToUpgrade: []UpgradeItem{
					{
						Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
						NextVersion: "v2.0.1", // upgrade to next release in the current contract
					},
				},
			},
			want: &UpgradePlan{
				Contract: currentContractVersion,
				Providers: []UpgradeItem{
					{
						Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
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
				repository: map[string]repository.Repository{
					"cluster-api": repository.NewMemoryRepository().
						WithVersions("v1.0.0", "v1.0.1").
						WithMetadata("v1.0.1", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 0, Contract: currentContractVersion},
							},
						}),
					"infra": repository.NewMemoryRepository().
						WithVersions("v2.0.0", "v2.0.1").
						WithMetadata("v2.0.1", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 2, Minor: 0, Contract: currentContractVersion},
							},
						}),
				},
				// two providers existing in the cluster
				proxy: test.NewFakeProxy().
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
			},
			args: args{
				coreProvider: fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "", "cluster-api-system"),
				providersToUpgrade: []UpgradeItem{
					{
						Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system"),
						NextVersion: "v1.0.1", // upgrade to next release in the current contract
					},
				},
			},
			want: &UpgradePlan{
				Contract: currentContractVersion,
				Providers: []UpgradeItem{
					{
						Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system"),
						NextVersion: "v1.0.1",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "pass if upgrade core and infra provider, same contract",
			fields: fields{
				// config for two providers
				reader: test.NewFakeReader().
					WithProvider("cluster-api", clusterctlv1.CoreProviderType, "https://somewhere.com").
					WithProvider("infra", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				repository: map[string]repository.Repository{
					"cluster-api": repository.NewMemoryRepository().
						WithVersions("v1.0.0", "v2.0.0").
						WithMetadata("v2.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 0, Contract: currentContractVersion},
								{Major: 2, Minor: 0, Contract: currentContractVersion},
							},
						}),
					"infra": repository.NewMemoryRepository().
						WithVersions("v2.0.0", "v3.0.0").
						WithMetadata("v3.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 2, Minor: 0, Contract: currentContractVersion},
								{Major: 3, Minor: 0, Contract: currentContractVersion},
							},
						}),
				},
				// two providers existing in the cluster
				proxy: test.NewFakeProxy().
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
			},
			args: args{
				coreProvider: fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "", "cluster-api-system"),
				providersToUpgrade: []UpgradeItem{
					{
						Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system"),
						NextVersion: "v2.0.0", // upgrade to next release in the next contract; not supported in current clusterctl release.
					},
					{
						Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
						NextVersion: "v3.0.0", // upgrade to next release in the next contract; not supported in current clusterctl release.
					},
				},
			},
			want: &UpgradePlan{
				Contract: currentContractVersion,
				Providers: []UpgradeItem{
					{
						Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system"),
						NextVersion: "v2.0.0",
					},
					{
						Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
						NextVersion: "v3.0.0",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "fail if upgrade infra provider alone from current to the next contract", // not supported in current clusterctl release.
			fields: fields{
				// config for two providers
				reader: test.NewFakeReader().
					WithProvider("cluster-api", clusterctlv1.CoreProviderType, "https://somewhere.com").
					WithProvider("infra", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				repository: map[string]repository.Repository{
					"cluster-api": repository.NewMemoryRepository().
						WithVersions("v1.0.0", "v2.0.0").
						WithMetadata("v2.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 0, Contract: currentContractVersion},
								{Major: 2, Minor: 0, Contract: nextContractVersionNotSupportedYet},
							},
						}),
					"infra": repository.NewMemoryRepository().
						WithVersions("v2.0.0", "v3.0.0").
						WithMetadata("v3.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 2, Minor: 0, Contract: currentContractVersion},
								{Major: 3, Minor: 0, Contract: nextContractVersionNotSupportedYet},
							},
						}),
				},
				// two providers existing in the cluster
				proxy: test.NewFakeProxy().
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
			},
			args: args{
				coreProvider: fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "", "cluster-api-system"),
				providersToUpgrade: []UpgradeItem{
					{
						Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
						NextVersion: "v3.0.0", // upgrade to next release in the next contract; not supported in current clusterctl release.
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "fail if upgrade infra provider alone from previous to the current contract",
			fields: fields{
				// config for two providers
				reader: test.NewFakeReader().
					WithProvider("cluster-api", clusterctlv1.CoreProviderType, "https://somewhere.com").
					WithProvider("infra", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				repository: map[string]repository.Repository{
					"cluster-api": repository.NewMemoryRepository().
						WithVersions("v1.0.0", "v2.0.0").
						WithMetadata("v2.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 0, Contract: oldContractVersionNotSupportedAnymore},
								{Major: 2, Minor: 0, Contract: currentContractVersion},
							},
						}),
					"infra": repository.NewMemoryRepository().
						WithVersions("v2.0.0", "v3.0.0").
						WithMetadata("v3.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 2, Minor: 0, Contract: oldContractVersionNotSupportedAnymore},
								{Major: 3, Minor: 0, Contract: currentContractVersion},
							},
						}),
				},
				// two providers existing in the cluster
				proxy: test.NewFakeProxy().
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
			},
			args: args{
				coreProvider: fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "", "cluster-api-system"),
				providersToUpgrade: []UpgradeItem{
					{
						Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
						NextVersion: "v3.0.0", // upgrade to next release in the current contract.
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "fail if upgrade core provider alone from current to the next contract", // not supported in current clusterctl release.
			fields: fields{
				// config for two providers
				reader: test.NewFakeReader().
					WithProvider("cluster-api", clusterctlv1.CoreProviderType, "https://somewhere.com").
					WithProvider("infra", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				repository: map[string]repository.Repository{
					"cluster-api": repository.NewMemoryRepository().
						WithVersions("v1.0.0", "v2.0.0").
						WithMetadata("v2.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 0, Contract: currentContractVersion},
								{Major: 2, Minor: 0, Contract: nextContractVersionNotSupportedYet},
							},
						}),
					"infra": repository.NewMemoryRepository().
						WithVersions("v2.0.0", "v3.0.0").
						WithMetadata("v3.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 2, Minor: 0, Contract: currentContractVersion},
								{Major: 3, Minor: 0, Contract: nextContractVersionNotSupportedYet},
							},
						}),
				},
				// two providers existing in the cluster
				proxy: test.NewFakeProxy().
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
			},
			args: args{
				coreProvider: fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "", "cluster-api-system"),
				providersToUpgrade: []UpgradeItem{
					{
						Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system"),
						NextVersion: "v2.0.0", // upgrade to next release in the next contract; not supported in current clusterctl release.
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "fail if upgrade core provider alone from previous to the current contract",
			fields: fields{
				// config for two providers
				reader: test.NewFakeReader().
					WithProvider("cluster-api", clusterctlv1.CoreProviderType, "https://somewhere.com").
					WithProvider("infra", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				repository: map[string]repository.Repository{
					"cluster-api": repository.NewMemoryRepository().
						WithVersions("v1.0.0", "v2.0.0").
						WithMetadata("v2.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 0, Contract: oldContractVersionNotSupportedAnymore},
								{Major: 2, Minor: 0, Contract: currentContractVersion},
							},
						}),
					"infra": repository.NewMemoryRepository().
						WithVersions("v2.0.0", "v3.0.0").
						WithMetadata("v3.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 2, Minor: 0, Contract: oldContractVersionNotSupportedAnymore},
								{Major: 3, Minor: 0, Contract: currentContractVersion},
							},
						}),
				},
				// two providers existing in the cluster
				proxy: test.NewFakeProxy().
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
			},
			args: args{
				coreProvider: fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "", "cluster-api-system"),
				providersToUpgrade: []UpgradeItem{
					{
						Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system"),
						NextVersion: "v2.0.0", // upgrade to next release in the current contract
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "pass if upgrade core provider alone from previous to the current contract and infra provider remains on a compatible version",
			fields: fields{
				// config for two providers
				reader: test.NewFakeReader().
					WithProvider("cluster-api", clusterctlv1.CoreProviderType, "https://somewhere.com").
					WithProvider("infra", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				repository: map[string]repository.Repository{
					"cluster-api": repository.NewMemoryRepository().
						WithVersions("v1.0.0", "v2.0.0").
						WithMetadata("v2.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 0, Contract: oldContractVersionNotSupportedAnymore},
								{Major: 2, Minor: 0, Contract: currentContractVersion},
							},
						}),
					"infra": repository.NewMemoryRepository().
						WithVersions("v2.0.0", "v3.0.0").
						WithMetadata("v3.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 2, Minor: 0, Contract: oldContractVersionStillSupported},
								{Major: 3, Minor: 0, Contract: currentContractVersion},
							},
						}),
				},
				// two providers existing in the cluster
				proxy: test.NewFakeProxy().
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
			},
			args: args{
				coreProvider: fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "", "cluster-api-system"),
				providersToUpgrade: []UpgradeItem{
					{
						Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system"),
						NextVersion: "v2.0.0", // upgrade to next release in the current contract
					},
				},
			},
			want: &UpgradePlan{
				Contract: currentContractVersion,
				Providers: []UpgradeItem{
					{
						Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system"),
						NextVersion: "v2.0.0",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "fail if upgrade core and infra provider to the next contract", // not supported in current clusterctl release
			fields: fields{
				// config for two providers
				reader: test.NewFakeReader().
					WithProvider("cluster-api", clusterctlv1.CoreProviderType, "https://somewhere.com").
					WithProvider("infra", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				repository: map[string]repository.Repository{
					"cluster-api": repository.NewMemoryRepository().
						WithVersions("v1.0.0", "v2.0.0").
						WithMetadata("v2.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 0, Contract: currentContractVersion},
								{Major: 2, Minor: 0, Contract: nextContractVersionNotSupportedYet},
							},
						}),
					"infra": repository.NewMemoryRepository().
						WithVersions("v2.0.0", "v3.0.0").
						WithMetadata("v3.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 2, Minor: 0, Contract: currentContractVersion},
								{Major: 3, Minor: 0, Contract: nextContractVersionNotSupportedYet},
							},
						}),
				},
				// two providers existing in the cluster
				proxy: test.NewFakeProxy().
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
			},
			args: args{
				coreProvider: fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "", "cluster-api-system"),
				providersToUpgrade: []UpgradeItem{
					{
						Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system"),
						NextVersion: "v2.0.0", // upgrade to next release in the next contract; not supported in current clusterctl release.
					},
					{
						Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
						NextVersion: "v3.0.0", // upgrade to next release in the next contract; not supported in current clusterctl release.
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "pass if upgrade core and infra provider from previous to current contract",
			fields: fields{
				// config for two providers
				reader: test.NewFakeReader().
					WithProvider("cluster-api", clusterctlv1.CoreProviderType, "https://somewhere.com").
					WithProvider("infra", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				repository: map[string]repository.Repository{
					"cluster-api": repository.NewMemoryRepository().
						WithVersions("v1.0.0", "v2.0.0").
						WithMetadata("v2.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 0, Contract: oldContractVersionNotSupportedAnymore},
								{Major: 2, Minor: 0, Contract: currentContractVersion},
							},
						}),
					"infra": repository.NewMemoryRepository().
						WithVersions("v2.0.0", "v3.0.0").
						WithMetadata("v3.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 2, Minor: 0, Contract: oldContractVersionNotSupportedAnymore},
								{Major: 3, Minor: 0, Contract: currentContractVersion},
							},
						}),
				},
				// two providers existing in the cluster
				proxy: test.NewFakeProxy().
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
			},
			args: args{
				coreProvider: fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "", "cluster-api-system"),
				providersToUpgrade: []UpgradeItem{
					{
						Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system"),
						NextVersion: "v2.0.0", // upgrade to next release in the next contract; not supported in current clusterctl release.
					},
					{
						Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
						NextVersion: "v3.0.0", // upgrade to next release in the next contract; not supported in current clusterctl release.
					},
				},
			},
			want: &UpgradePlan{
				Contract: currentContractVersion,
				Providers: []UpgradeItem{
					{
						Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system"),
						NextVersion: "v2.0.0",
					},
					{
						Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
						NextVersion: "v3.0.0",
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ctx := t.Context()

			configClient, _ := config.New(ctx, "", config.InjectReader(tt.fields.reader))

			u := &providerUpgrader{
				configClient: configClient,
				repositoryClientFactory: func(ctx context.Context, provider config.Provider, configClient config.Client, _ ...repository.Option) (repository.Client, error) {
					return repository.New(ctx, provider, configClient, repository.InjectRepository(tt.fields.repository[provider.Name()]))
				},
				providerInventory:             newInventoryClient(tt.fields.proxy, nil, currentContractVersion),
				currentContractVersion:        currentContractVersion,
				getCompatibleContractVersions: getCompatibleContractVersions,
			}
			got, err := u.createCustomPlan(ctx, tt.args.providersToUpgrade)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(BeComparableTo(tt.want))
		})
	}
}

// TODO add tests  for success scenarios.
func Test_providerUpgrader_ApplyPlan(t *testing.T) {
	type fields struct {
		reader     config.Reader
		repository map[string]repository.Repository
		proxy      Proxy
	}

	tests := []struct {
		name     string
		fields   fields
		contract string
		wantErr  bool
		errorMsg string
		opts     UpgradeOptions
	}{
		{
			name: "fails to upgrade to current contract when there are multiple instances of the core provider",
			fields: fields{
				// config for two providers
				reader: test.NewFakeReader().
					WithProvider("cluster-api", clusterctlv1.CoreProviderType, "https://somewhere.com").
					WithProvider("infra", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				// two provider repositories, with current v1alpha3 contract and new versions for v1alpha4 contract
				repository: map[string]repository.Repository{
					"cluster-api": repository.NewMemoryRepository().
						WithVersions("v1.0.0", "v1.0.1", "v2.0.0").
						WithMetadata("v2.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 0, Contract: oldContractVersionNotSupportedAnymore},
								{Major: 2, Minor: 0, Contract: currentContractVersion},
							},
						}),
					"infrastructure-infra": repository.NewMemoryRepository().
						WithVersions("v2.0.0", "v2.0.1", "v3.0.0").
						WithMetadata("v3.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 2, Minor: 0, Contract: oldContractVersionNotSupportedAnymore},
								{Major: 3, Minor: 0, Contract: currentContractVersion},
							},
						}),
				},
				// two providers with multiple instances existing in the cluster
				proxy: test.NewFakeProxy().
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system").
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system-1").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
			},
			contract: currentContractVersion,
			wantErr:  true,
			errorMsg: "detected multiple instances of the same provider",
			opts:     UpgradeOptions{},
		},
		{
			name: "fails to upgrade to current contract when there are multiple instances of the infra provider",
			fields: fields{
				// config for two providers
				reader: test.NewFakeReader().
					WithProvider("cluster-api", clusterctlv1.CoreProviderType, "https://somewhere.com").
					WithProvider("infra", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				// two provider repositories, with current v1alpha3 contract and new versions for v1alpha4 contract
				repository: map[string]repository.Repository{
					"cluster-api": repository.NewMemoryRepository().
						WithVersions("v1.0.0", "v1.0.1", "v2.0.0").
						WithMetadata("v2.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 0, Contract: oldContractVersionNotSupportedAnymore},
								{Major: 2, Minor: 0, Contract: currentContractVersion},
							},
						}),
					"infrastructure-infra": repository.NewMemoryRepository().
						WithVersions("v2.0.0", "v2.0.1", "v3.0.0").
						WithMetadata("v3.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 2, Minor: 0, Contract: oldContractVersionNotSupportedAnymore},
								{Major: 3, Minor: 0, Contract: currentContractVersion},
							},
						}),
				},
				// two providers with multiple instances existing in the cluster
				proxy: test.NewFakeProxy().
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system-1"),
			},
			contract: currentContractVersion,
			wantErr:  true,
			errorMsg: "detected multiple instances of the same provider",
			opts:     UpgradeOptions{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ctx := t.Context()

			configClient, _ := config.New(ctx, "", config.InjectReader(tt.fields.reader))

			u := &providerUpgrader{
				configClient: configClient,
				repositoryClientFactory: func(ctx context.Context, provider config.Provider, configClient config.Client, _ ...repository.Option) (repository.Client, error) {
					return repository.New(ctx, provider, configClient, repository.InjectRepository(tt.fields.repository[provider.ManifestLabel()]))
				},
				providerInventory:             newInventoryClient(tt.fields.proxy, nil, currentContractVersion),
				currentContractVersion:        currentContractVersion,
				getCompatibleContractVersions: getCompatibleContractVersions,
			}
			err := u.ApplyPlan(ctx, tt.opts, tt.contract)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).Should(ContainSubstring(tt.errorMsg))
				return
			}

			g.Expect(err).ToNot(HaveOccurred())
		})
	}
}

// TODO add tests  for success scenarios.
func Test_providerUpgrader_ApplyCustomPlan(t *testing.T) {
	type fields struct {
		reader     config.Reader
		repository map[string]repository.Repository
		proxy      Proxy
	}

	tests := []struct {
		name               string
		fields             fields
		providersToUpgrade []UpgradeItem
		wantErr            bool
		errorMsg           string
		opts               UpgradeOptions
	}{
		{
			name: "fails to upgrade when there are multiple instances of the core provider",
			fields: fields{
				// config for two providers
				reader: test.NewFakeReader().
					WithProvider("cluster-api", clusterctlv1.CoreProviderType, "https://somewhere.com").
					WithProvider("infra", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				// two provider repositories, with current v1alpha3 contract and new versions for v1alpha4 contract
				repository: map[string]repository.Repository{
					"cluster-api": repository.NewMemoryRepository().
						WithVersions("v1.0.0", "v1.0.1", "v2.0.0").
						WithMetadata("v2.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 0, Contract: "v1alpha3"},
								{Major: 2, Minor: 0, Contract: currentContractVersion},
							},
						}),
					"infrastructure-infra": repository.NewMemoryRepository().
						WithVersions("v2.0.0", "v2.0.1", "v3.0.0").
						WithMetadata("v3.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 2, Minor: 0, Contract: oldContractVersionNotSupportedAnymore},
								{Major: 3, Minor: 0, Contract: currentContractVersion},
							},
						}),
				},
				// two providers with multiple instances existing in the cluster
				proxy: test.NewFakeProxy().
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system").
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system-1").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
			},
			providersToUpgrade: []UpgradeItem{
				{
					Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system"),
					NextVersion: "v2.0.0",
				},
				{
					Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
					NextVersion: "v3.0.0",
				},
			},
			wantErr:  true,
			errorMsg: "invalid management cluster: there must be one core provider, found 2",
			opts:     UpgradeOptions{},
		},
		{
			name: "fails to upgrade when there are multiple instances of the infra provider",
			fields: fields{
				// config for two providers
				reader: test.NewFakeReader().
					WithProvider("cluster-api", clusterctlv1.CoreProviderType, "https://somewhere.com").
					WithProvider("infra", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				// two provider repositories, with current v1alpha3 contract and new versions for v1alpha4 contract
				repository: map[string]repository.Repository{
					"cluster-api": repository.NewMemoryRepository().
						WithVersions("v1.0.0", "v1.0.1", "v2.0.0").
						WithMetadata("v2.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 0, Contract: oldContractVersionNotSupportedAnymore},
								{Major: 2, Minor: 0, Contract: currentContractVersion},
							},
						}),
					"infrastructure-infra": repository.NewMemoryRepository().
						WithVersions("v2.0.0", "v2.0.1", "v3.0.0").
						WithMetadata("v3.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 2, Minor: 0, Contract: oldContractVersionNotSupportedAnymore},
								{Major: 3, Minor: 0, Contract: currentContractVersion},
							},
						}),
				},
				// two providers with multiple instances existing in the cluster
				proxy: test.NewFakeProxy().
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system").
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system-1"),
			},
			providersToUpgrade: []UpgradeItem{
				{
					Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system"),
					NextVersion: "v2.0.0",
				},
				{
					Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
					NextVersion: "v3.0.0",
				},
				{
					Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system-1"),
					NextVersion: "v3.0.0",
				},
			},
			wantErr:  true,
			errorMsg: "detected multiple instances of the same provider",
			opts:     UpgradeOptions{},
		},
		{
			name: "fails to upgrade to current contract when violating core provider skip version rules",
			fields: fields{
				reader: test.NewFakeReader().
					WithProvider("cluster-api", clusterctlv1.CoreProviderType, "https://somewhere.com"),
				repository: map[string]repository.Repository{
					"cluster-api": repository.NewMemoryRepository().
						WithVersions("v1.10.0", "v1.11.0", "v1.12.0", "v1.13.0", "v1.14.0").
						WithMetadata("v1.14.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 10, Contract: oldContractVersionNotSupportedAnymore},
								{Major: 1, Minor: 11, Contract: currentContractVersion},
								{Major: 1, Minor: 12, Contract: currentContractVersion},
								{Major: 1, Minor: 13, Contract: currentContractVersion},
								{Major: 1, Minor: 14, Contract: currentContractVersion},
							},
						}),
				},
				proxy: test.NewFakeProxy().
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.10.0", "cluster-api-system"),
			},
			providersToUpgrade: []UpgradeItem{
				{
					Provider:    fakeProvider("cluster-api", clusterctlv1.CoreProviderType, "v1.10.0", "cluster-api-system"),
					NextVersion: "v1.14.0",
				},
			},
			wantErr:  true,
			errorMsg: "upgrade for cluster-api-system/cluster-api provider can't skip more than 3 versions",
			opts:     UpgradeOptions{},
		},
		{
			name: "fails to upgrade to current contract when violating kubeadm bootstrap provider skip version rules",
			fields: fields{
				reader: test.NewFakeReader().
					WithProvider("cluster-api", clusterctlv1.CoreProviderType, "https://somewhere.com").
					WithProvider(config.KubeadmBootstrapProviderName, clusterctlv1.BootstrapProviderType, "https://somewhere.com"),
				repository: map[string]repository.Repository{
					"cluster-api": repository.NewMemoryRepository().
						WithVersions("v1.0.0").
						WithMetadata("v1.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 0, Contract: currentContractVersion},
							},
						}),
					"bootstrap-kubeadm": repository.NewMemoryRepository().
						WithVersions("v1.10.0", "v1.11.0", "v1.12.0", "v1.13.0", "v1.14.0").
						WithMetadata("v1.14.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 10, Contract: oldContractVersionNotSupportedAnymore},
								{Major: 1, Minor: 11, Contract: currentContractVersion},
								{Major: 1, Minor: 12, Contract: currentContractVersion},
								{Major: 1, Minor: 13, Contract: currentContractVersion},
								{Major: 1, Minor: 14, Contract: currentContractVersion},
							},
						}),
				},
				proxy: test.NewFakeProxy().
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system").
					WithProviderInventory(config.KubeadmBootstrapProviderName, clusterctlv1.BootstrapProviderType, "v1.10.0", "cluster-api-system"),
			},
			providersToUpgrade: []UpgradeItem{
				{
					Provider:    fakeProvider(config.KubeadmBootstrapProviderName, clusterctlv1.BootstrapProviderType, "v1.10.0", "cluster-api-system"),
					NextVersion: "v1.14.0",
				},
			},
			wantErr:  true,
			errorMsg: "upgrade for cluster-api-system/bootstrap-kubeadm provider can't skip more than 3 versions",
			opts:     UpgradeOptions{},
		},
		{
			name: "fails to upgrade to current contract when violating kubeadm control plane provider skip version rules",
			fields: fields{
				reader: test.NewFakeReader().
					WithProvider("cluster-api", clusterctlv1.CoreProviderType, "https://somewhere.com").
					WithProvider(config.KubeadmControlPlaneProviderName, clusterctlv1.ControlPlaneProviderType, "https://somewhere.com"),
				repository: map[string]repository.Repository{
					"cluster-api": repository.NewMemoryRepository().
						WithVersions("v1.0.0").
						WithMetadata("v1.0.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 0, Contract: currentContractVersion},
							},
						}),
					"control-plane-kubeadm": repository.NewMemoryRepository().
						WithVersions("v1.10.0", "v1.11.0", "v1.12.0", "v1.13.0", "v1.14.0").
						WithMetadata("v1.14.0", &clusterctlv1.Metadata{
							ReleaseSeries: []clusterctlv1.ReleaseSeries{
								{Major: 1, Minor: 10, Contract: oldContractVersionNotSupportedAnymore},
								{Major: 1, Minor: 11, Contract: currentContractVersion},
								{Major: 1, Minor: 12, Contract: currentContractVersion},
								{Major: 1, Minor: 13, Contract: currentContractVersion},
								{Major: 1, Minor: 14, Contract: currentContractVersion},
							},
						}),
				},
				proxy: test.NewFakeProxy().
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system").
					WithProviderInventory(config.KubeadmControlPlaneProviderName, clusterctlv1.ControlPlaneProviderType, "v1.10.0", "cluster-api-system"),
			},
			providersToUpgrade: []UpgradeItem{
				{
					Provider:    fakeProvider(config.KubeadmControlPlaneProviderName, clusterctlv1.ControlPlaneProviderType, "v1.10.0", "cluster-api-system"),
					NextVersion: "v1.14.0",
				},
			},
			wantErr:  true,
			errorMsg: "upgrade for cluster-api-system/control-plane-kubeadm provider can't skip more than 3 versions",
			opts:     UpgradeOptions{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ctx := t.Context()

			configClient, _ := config.New(ctx, "", config.InjectReader(tt.fields.reader))

			u := &providerUpgrader{
				configClient: configClient,
				repositoryClientFactory: func(ctx context.Context, provider config.Provider, configClient config.Client, _ ...repository.Option) (repository.Client, error) {
					return repository.New(ctx, provider, configClient, repository.InjectRepository(tt.fields.repository[provider.ManifestLabel()]))
				},
				providerInventory:             newInventoryClient(tt.fields.proxy, nil, currentContractVersion),
				currentContractVersion:        currentContractVersion,
				getCompatibleContractVersions: getCompatibleContractVersions,
			}
			err := u.ApplyCustomPlan(ctx, tt.opts, tt.providersToUpgrade...)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).Should(ContainSubstring(tt.errorMsg))
				return
			}

			g.Expect(err).ToNot(HaveOccurred())
		})
	}
}
