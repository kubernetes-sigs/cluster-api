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

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/repository"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
)

func Test_providerInstaller_Validate(t *testing.T) {
	fakeReader := test.NewFakeReader().
		WithProvider("cluster-api", clusterctlv1.CoreProviderType, "https://somewhere.com").
		WithProvider("infra1", clusterctlv1.InfrastructureProviderType, "https://somewhere.com").
		WithProvider("infra2", clusterctlv1.InfrastructureProviderType, "https://somewhere.com")

	repositoryMap := map[string]repository.Repository{
		"cluster-api": test.NewFakeRepository().
			WithVersions("v0.9.0", "v1.0.0", "v1.0.1", "v2.0.0").
			WithMetadata("v0.9.0", &clusterctlv1.Metadata{
				ReleaseSeries: []clusterctlv1.ReleaseSeries{
					{Major: 0, Minor: 9, Contract: test.PreviousCAPIContractNotSupported},
				},
			}).
			WithMetadata("v1.0.0", &clusterctlv1.Metadata{
				ReleaseSeries: []clusterctlv1.ReleaseSeries{
					{Major: 0, Minor: 9, Contract: test.PreviousCAPIContractNotSupported},
					{Major: 1, Minor: 0, Contract: test.CurrentCAPIContract},
				},
			}).
			WithMetadata("v2.0.0", &clusterctlv1.Metadata{
				ReleaseSeries: []clusterctlv1.ReleaseSeries{
					{Major: 0, Minor: 9, Contract: test.PreviousCAPIContractNotSupported},
					{Major: 1, Minor: 0, Contract: test.CurrentCAPIContract},
					{Major: 2, Minor: 0, Contract: test.NextCAPIContractNotSupported},
				},
			}),
		"infrastructure-infra1": test.NewFakeRepository().
			WithVersions("v0.9.0", "v1.0.0", "v1.0.1", "v2.0.0").
			WithMetadata("v0.9.0", &clusterctlv1.Metadata{
				ReleaseSeries: []clusterctlv1.ReleaseSeries{
					{Major: 0, Minor: 9, Contract: test.PreviousCAPIContractNotSupported},
				},
			}).
			WithMetadata("v1.0.0", &clusterctlv1.Metadata{
				ReleaseSeries: []clusterctlv1.ReleaseSeries{
					{Major: 0, Minor: 9, Contract: test.PreviousCAPIContractNotSupported},
					{Major: 1, Minor: 0, Contract: test.CurrentCAPIContract},
				},
			}).
			WithMetadata("v2.0.0", &clusterctlv1.Metadata{
				ReleaseSeries: []clusterctlv1.ReleaseSeries{
					{Major: 0, Minor: 9, Contract: test.PreviousCAPIContractNotSupported},
					{Major: 1, Minor: 0, Contract: test.CurrentCAPIContract},
					{Major: 2, Minor: 0, Contract: test.NextCAPIContractNotSupported},
				},
			}),
		"infrastructure-infra2": test.NewFakeRepository().
			WithVersions("v0.9.0", "v1.0.0", "v1.0.1", "v2.0.0").
			WithMetadata("v0.9.0", &clusterctlv1.Metadata{
				ReleaseSeries: []clusterctlv1.ReleaseSeries{
					{Major: 0, Minor: 9, Contract: test.PreviousCAPIContractNotSupported},
				},
			}).
			WithMetadata("v1.0.0", &clusterctlv1.Metadata{
				ReleaseSeries: []clusterctlv1.ReleaseSeries{
					{Major: 1, Minor: 0, Contract: test.CurrentCAPIContract},
				},
			}).
			WithMetadata("v2.0.0", &clusterctlv1.Metadata{
				ReleaseSeries: []clusterctlv1.ReleaseSeries{
					{Major: 1, Minor: 0, Contract: test.CurrentCAPIContract},
					{Major: 2, Minor: 0, Contract: test.NextCAPIContractNotSupported},
				},
			}),
	}

	type fields struct {
		proxy        Proxy
		installQueue []repository.Components
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		{
			name: "install core/current contract + infra1/current contract on an empty cluster",
			fields: fields{
				proxy: test.NewFakeProxy(), //empty cluster
				installQueue: []repository.Components{
					newFakeComponents("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", ""),
					newFakeComponents("infra1", clusterctlv1.InfrastructureProviderType, "v1.0.0", "infra1-system", ""),
				},
			},
			wantErr: false,
		},
		{
			name: "install infra2/current contract on a cluster already initialized with core/current contract + infra1/current contract",
			fields: fields{
				proxy: test.NewFakeProxy().
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", "").
					WithProviderInventory("infra1", clusterctlv1.InfrastructureProviderType, "v1.0.0", "infra1-system", ""),
				installQueue: []repository.Components{
					newFakeComponents("infra2", clusterctlv1.InfrastructureProviderType, "v1.0.0", "infra2-system", ""),
				},
			},
			wantErr: false,
		},
		{
			name: "install another instance of infra1/current contract on a cluster already initialized with core/current contract + infra1/current contract, no overlaps",
			fields: fields{
				proxy: test.NewFakeProxy().
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", "").
					WithProviderInventory("infra1", clusterctlv1.InfrastructureProviderType, "v1.0.0", "ns1", "ns1"),
				installQueue: []repository.Components{
					newFakeComponents("infra2", clusterctlv1.InfrastructureProviderType, "v1.0.0", "ns2", "ns2"),
				},
			},
			wantErr: false,
		},
		{
			name: "install another instance of infra1/current contract on a cluster already initialized with core/current contract + infra1/current contract, same namespace of the existing infra1",
			fields: fields{
				proxy: test.NewFakeProxy().
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", "").
					WithProviderInventory("infra1", clusterctlv1.InfrastructureProviderType, "v1.0.0", "n1", ""),
				installQueue: []repository.Components{
					newFakeComponents("infra1", clusterctlv1.InfrastructureProviderType, "v1.0.0", "n1", ""),
				},
			},
			wantErr: true,
		},
		{
			name: "install another instance of infra1/current contract on a cluster already initialized with core/current contract + infra1/current contract, watching overlap with the existing infra1",
			fields: fields{
				proxy: test.NewFakeProxy().
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", "").
					WithProviderInventory("infra1", clusterctlv1.InfrastructureProviderType, "v1.0.0", "infra1-system", ""),
				installQueue: []repository.Components{
					newFakeComponents("infra1", clusterctlv1.InfrastructureProviderType, "v1.0.0", "infra2-system", ""),
				},
			},
			wantErr: true,
		},
		{
			name: "install another instance of infra1/current contract on a cluster already initialized with core/current contract + infra1/current contract, not part of the existing management group",
			fields: fields{
				proxy: test.NewFakeProxy().
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "ns1", "ns1").
					WithProviderInventory("infra1", clusterctlv1.InfrastructureProviderType, "v1.0.0", "ns1", "ns1"),
				installQueue: []repository.Components{
					newFakeComponents("infra1", clusterctlv1.InfrastructureProviderType, "v1.0.0", "ns2", "ns2"),
				},
			},
			wantErr: true,
		},
		{
			name: "install an instance of infra1/current contract on a cluster already initialized with two core/current contract, but it is part of two management group",
			fields: fields{
				proxy: test.NewFakeProxy(). // cluster with two core (two management groups)
								WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "ns1", "ns1").
								WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "ns2", "ns2"),
				installQueue: []repository.Components{
					newFakeComponents("infra1", clusterctlv1.InfrastructureProviderType, "v1.0.0", "infra1-system", ""),
				},
			},
			wantErr: true,
		},
		{
			name: "install core/previous contract + infra1/previous contract on an empty cluster (not supported)",
			fields: fields{
				proxy: test.NewFakeProxy(), //empty cluster
				installQueue: []repository.Components{
					newFakeComponents("cluster-api", clusterctlv1.CoreProviderType, "v0.9.0", "cluster-api-system", ""),
					newFakeComponents("infra1", clusterctlv1.InfrastructureProviderType, "v0.9.0", "infra1-system", ""),
				},
			},
			wantErr: true,
		},
		{
			name: "install core/previous contract + infra1/current contract on an empty cluster (not supported)",
			fields: fields{
				proxy: test.NewFakeProxy(), //empty cluster
				installQueue: []repository.Components{
					newFakeComponents("cluster-api", clusterctlv1.CoreProviderType, "v0.9.0", "cluster-api-system", ""),
					newFakeComponents("infra1", clusterctlv1.InfrastructureProviderType, "v1.0.0", "infra1-system", ""),
				},
			},
			wantErr: true,
		},
		{
			name: "install infra1/previous contract (not supported) on a cluster already initialized with core/current contract",
			fields: fields{
				proxy: test.NewFakeProxy().
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "ns1", "ns1"),
				installQueue: []repository.Components{
					newFakeComponents("infra1", clusterctlv1.InfrastructureProviderType, "v0.9.0", "infra1-system", ""),
				},
			},
			wantErr: true,
		},
		{
			name: "install core/next contract + infra1/next contract on an empty cluster (not supported)",
			fields: fields{
				proxy: test.NewFakeProxy(), //empty cluster
				installQueue: []repository.Components{
					newFakeComponents("cluster-api", clusterctlv1.CoreProviderType, "v2.0.0", "cluster-api-system", ""),
					newFakeComponents("infra1", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra1-system", ""),
				},
			},
			wantErr: true,
		},
		{
			name: "install core/current contract + infra1/next contract on an empty cluster (not supported)",
			fields: fields{
				proxy: test.NewFakeProxy(), //empty cluster
				installQueue: []repository.Components{
					newFakeComponents("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", ""),
					newFakeComponents("infra1", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra1-system", ""),
				},
			},
			wantErr: true,
		},
		{
			name: "install infra1/next contract (not supported) on a cluster already initialized with core/current contract",
			fields: fields{
				proxy: test.NewFakeProxy().
					WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "ns1", "ns1"),
				installQueue: []repository.Components{
					newFakeComponents("infra1", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra1-system", ""),
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			configClient, _ := config.New("", config.InjectReader(fakeReader))

			i := &providerInstaller{
				configClient:      configClient,
				proxy:             tt.fields.proxy,
				providerInventory: newInventoryClient(tt.fields.proxy, nil),
				repositoryClientFactory: func(provider config.Provider, configClient config.Client, options ...repository.Option) (repository.Client, error) {
					return repository.New(provider, configClient, repository.InjectRepository(repositoryMap[provider.ManifestLabel()]))
				},
				installQueue: tt.fields.installQueue,
			}

			err := i.Validate()
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
		})
	}
}

type fakeComponents struct {
	config.Provider
	inventoryObject clusterctlv1.Provider
}

func (c *fakeComponents) Version() string {
	panic("not implemented")
}

func (c *fakeComponents) Variables() []string {
	panic("not implemented")
}

func (c *fakeComponents) Images() []string {
	panic("not implemented")
}

func (c *fakeComponents) TargetNamespace() string {
	panic("not implemented")
}

func (c *fakeComponents) WatchingNamespace() string {
	panic("not implemented")
}

func (c *fakeComponents) InventoryObject() clusterctlv1.Provider {
	return c.inventoryObject
}

func (c *fakeComponents) InstanceObjs() []unstructured.Unstructured {
	panic("not implemented")
}

func (c *fakeComponents) SharedObjs() []unstructured.Unstructured {
	panic("not implemented")
}

func (c *fakeComponents) Yaml() ([]byte, error) {
	panic("not implemented")
}

func newFakeComponents(name string, providerType clusterctlv1.ProviderType, version, targetNamespace, watchingNamespace string) repository.Components {
	inventoryObject := fakeProvider(name, providerType, version, targetNamespace, watchingNamespace)
	return &fakeComponents{
		Provider:        config.NewProvider(inventoryObject.ProviderName, "", clusterctlv1.ProviderType(inventoryObject.Type)),
		inventoryObject: inventoryObject,
	}
}

func Test_shouldInstallSharedComponents(t *testing.T) {
	type args struct {
		providerList *clusterctlv1.ProviderList
		provider     clusterctlv1.Provider
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		{
			name: "First instance of the provider, must install shared components",
			args: args{
				providerList: &clusterctlv1.ProviderList{Items: []clusterctlv1.Provider{}}, // no core provider installed
				provider:     fakeProvider("core", clusterctlv1.CoreProviderType, "v2.0.0", "", ""),
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "Second instance of the provider, same version, must NOT install shared components",
			args: args{
				providerList: &clusterctlv1.ProviderList{Items: []clusterctlv1.Provider{
					fakeProvider("core", clusterctlv1.CoreProviderType, "v2.0.0", "", ""),
				}},
				provider: fakeProvider("core", clusterctlv1.CoreProviderType, "v2.0.0", "", ""),
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "Second instance of the provider, older version, must NOT install shared components",
			args: args{
				providerList: &clusterctlv1.ProviderList{Items: []clusterctlv1.Provider{
					fakeProvider("core", clusterctlv1.CoreProviderType, "v2.0.0", "", ""),
				}},
				provider: fakeProvider("core", clusterctlv1.CoreProviderType, "v1.0.0", "", ""),
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "Second instance of the provider, newer version, must install shared components",
			args: args{
				providerList: &clusterctlv1.ProviderList{Items: []clusterctlv1.Provider{
					fakeProvider("core", clusterctlv1.CoreProviderType, "v2.0.0", "", ""),
				}},
				provider: fakeProvider("core", clusterctlv1.CoreProviderType, "v3.0.0", "", ""),
			},
			want:    true,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := shouldInstallSharedComponents(tt.args.providerList, tt.args.provider)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(got).To(Equal(tt.want))
		})
	}
}
