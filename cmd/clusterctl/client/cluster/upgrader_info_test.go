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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/version"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/repository"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
)

func Test_providerUpgrader_getUpgradeInfo(t *testing.T) {
	type fields struct {
		reader     config.Reader
		repository repository.Repository
	}
	type args struct {
		provider clusterctlv1.Provider
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *upgradeInfo
		wantErr bool
	}{
		{
			name: "pass when current and next version are current contract",
			fields: fields{
				reader: test.NewFakeReader().
					WithProvider("p1", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				repository: test.NewFakeRepository().
					WithVersions("v1.0.0", "v1.0.1", "v1.0.2", "v1.1.0").
					WithMetadata("v1.1.0", &clusterctlv1.Metadata{
						ReleaseSeries: []clusterctlv1.ReleaseSeries{
							{Major: 1, Minor: 0, Contract: test.CurrentCAPIContract},
							{Major: 1, Minor: 1, Contract: test.CurrentCAPIContract},
						},
					}),
			},
			args: args{
				provider: fakeProvider("p1", clusterctlv1.InfrastructureProviderType, "v1.0.1", "p1-system", ""),
			},
			want: &upgradeInfo{
				metadata: &clusterctlv1.Metadata{
					TypeMeta: metav1.TypeMeta{
						APIVersion: clusterctlv1.GroupVersion.String(),
						Kind:       "Metadata",
					},
					ReleaseSeries: []clusterctlv1.ReleaseSeries{
						{Major: 1, Minor: 0, Contract: test.CurrentCAPIContract},
						{Major: 1, Minor: 1, Contract: test.CurrentCAPIContract},
					},
				},
				currentVersion:  version.MustParseSemantic("v1.0.1"),
				currentContract: test.CurrentCAPIContract,
				nextVersions: []version.Version{
					// v1.0.1 (the current version) and older are ignored
					*version.MustParseSemantic("v1.0.2"),
					*version.MustParseSemantic("v1.1.0"),
				},
			},
			wantErr: false,
		},
		{
			name: "pass when current version is in previous contract (Not supported), next version in current contract", // upgrade plan should report unsupported options
			fields: fields{
				reader: test.NewFakeReader().
					WithProvider("p1", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				repository: test.NewFakeRepository().
					WithVersions("v1.0.0", "v1.0.1", "v1.0.2", "v1.1.0").
					WithMetadata("v1.1.0", &clusterctlv1.Metadata{
						ReleaseSeries: []clusterctlv1.ReleaseSeries{
							{Major: 1, Minor: 0, Contract: test.PreviousCAPIContractNotSupported},
							{Major: 1, Minor: 1, Contract: test.CurrentCAPIContract},
						},
					}),
			},
			args: args{
				provider: fakeProvider("p1", clusterctlv1.InfrastructureProviderType, "v1.0.1", "p1-system", ""),
			},
			want: &upgradeInfo{
				metadata: &clusterctlv1.Metadata{
					TypeMeta: metav1.TypeMeta{
						APIVersion: clusterctlv1.GroupVersion.String(),
						Kind:       "Metadata",
					},
					ReleaseSeries: []clusterctlv1.ReleaseSeries{
						{Major: 1, Minor: 0, Contract: test.PreviousCAPIContractNotSupported},
						{Major: 1, Minor: 1, Contract: test.CurrentCAPIContract},
					},
				},
				currentVersion:  version.MustParseSemantic("v1.0.1"),
				currentContract: test.PreviousCAPIContractNotSupported,
				nextVersions: []version.Version{
					// v1.0.1 (the current version) and older are ignored
					*version.MustParseSemantic("v1.0.2"), // not supported, but upgrade plan should report these options
					*version.MustParseSemantic("v1.1.0"),
				},
			},
			wantErr: false,
		},
		{
			name: "pass when current version is current contract, next version is in next contract", // upgrade plan should report unsupported options
			fields: fields{
				reader: test.NewFakeReader().
					WithProvider("p1", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				repository: test.NewFakeRepository().
					WithVersions("v1.0.0", "v1.0.1", "v1.0.2", "v1.1.0").
					WithMetadata("v1.1.0", &clusterctlv1.Metadata{
						ReleaseSeries: []clusterctlv1.ReleaseSeries{
							{Major: 1, Minor: 0, Contract: test.CurrentCAPIContract},
							{Major: 1, Minor: 1, Contract: test.NextCAPIContractNotSupported},
						},
					}),
			},
			args: args{
				provider: fakeProvider("p1", clusterctlv1.InfrastructureProviderType, "v1.0.1", "p1-system", ""),
			},
			want: &upgradeInfo{
				metadata: &clusterctlv1.Metadata{
					TypeMeta: metav1.TypeMeta{
						APIVersion: clusterctlv1.GroupVersion.String(),
						Kind:       "Metadata",
					},
					ReleaseSeries: []clusterctlv1.ReleaseSeries{
						{Major: 1, Minor: 0, Contract: test.CurrentCAPIContract},
						{Major: 1, Minor: 1, Contract: test.NextCAPIContractNotSupported},
					},
				},
				currentVersion:  version.MustParseSemantic("v1.0.1"),
				currentContract: test.CurrentCAPIContract,
				nextVersions: []version.Version{
					// v1.0.1 (the current version) and older are ignored
					*version.MustParseSemantic("v1.0.2"),
					*version.MustParseSemantic("v1.1.0"), // not supported, but upgrade plan should report these options
				},
			},
			wantErr: false,
		},
		{
			name: "fails if a metadata file for upgrades cannot be found",
			fields: fields{
				reader: test.NewFakeReader().
					WithProvider("p1", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				repository: test.NewFakeRepository(). // without metadata
									WithVersions("v1.0.0", "v1.0.1"),
			},
			args: args{
				provider: fakeProvider("p1", clusterctlv1.InfrastructureProviderType, "v1.0.0", "p1-system", ""),
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "fails if a metadata file for upgrades cannot be found",
			fields: fields{
				reader: test.NewFakeReader().
					WithProvider("p1", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				repository: test.NewFakeRepository(). // with metadata but only for versions <= current version (not for next versions)
									WithVersions("v1.0.0", "v1.0.1").
									WithMetadata("v1.0.0", &clusterctlv1.Metadata{}),
			},
			args: args{
				provider: fakeProvider("p1", clusterctlv1.InfrastructureProviderType, "v1.0.0", "p1-system", ""),
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "fails if when current version does not match any release series in metadata",
			fields: fields{
				reader: test.NewFakeReader().
					WithProvider("p1", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				repository: test.NewFakeRepository(). // without metadata
									WithVersions("v1.0.0", "v1.0.1").
									WithMetadata("v1.0.1", &clusterctlv1.Metadata{}),
			},
			args: args{
				provider: fakeProvider("p1", clusterctlv1.InfrastructureProviderType, "v1.0.0", "p1-system", ""),
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "fails if available version does not match release series",
			fields: fields{
				reader: test.NewFakeReader().
					WithProvider("p1", clusterctlv1.InfrastructureProviderType, "https://somewhere.com"),
				repository: test.NewFakeRepository(). // without metadata
									WithVersions("v1.0.0", "v1.0.1", "v1.1.1").
									WithMetadata("v1.1.1", &clusterctlv1.Metadata{
						ReleaseSeries: []clusterctlv1.ReleaseSeries{
							{Major: 1, Minor: 0, Contract: test.CurrentCAPIContract},
							// missing 1.1 series
						},
					}),
			},
			args: args{
				provider: fakeProvider("p1", clusterctlv1.InfrastructureProviderType, "v1.0.0", "p1-system", ""),
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			configClient, _ := config.New("", config.InjectReader(tt.fields.reader))

			u := &providerUpgrader{
				configClient: configClient,
				repositoryClientFactory: func(provider config.Provider, configClient config.Client, options ...repository.Option) (repository.Client, error) {
					return repository.New(provider, configClient, repository.InjectRepository(tt.fields.repository))
				},
			}
			got, err := u.getUpgradeInfo(tt.args.provider)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func Test_upgradeInfo_getContractsForUpgrade(t *testing.T) {
	type field struct {
		currentVersion string
		metadata       *clusterctlv1.Metadata
	}
	tests := []struct {
		name  string
		field field
		want  []string
	}{
		{
			name: "One contract, current",
			field: field{
				metadata: &clusterctlv1.Metadata{ // metadata defining more release series, all linked to a single contract
					ReleaseSeries: []clusterctlv1.ReleaseSeries{
						{Major: 0, Minor: 1, Contract: test.CurrentCAPIContract},
						{Major: 0, Minor: 2, Contract: test.CurrentCAPIContract},
						{Major: 0, Minor: 3, Contract: test.CurrentCAPIContract},
					},
				},
				currentVersion: "v0.2.1", // current version belonging of one of the above series
			},
			want: []string{test.CurrentCAPIContract},
		},
		{
			name: "Multiple contracts (previous and current), all valid for upgrades", // upgrade plan should report unsupported options
			field: field{
				metadata: &clusterctlv1.Metadata{ // metadata defining more release series, linked to different contracts
					ReleaseSeries: []clusterctlv1.ReleaseSeries{
						{Major: 0, Minor: 1, Contract: test.PreviousCAPIContractNotSupported},
						{Major: 0, Minor: 2, Contract: test.CurrentCAPIContract},
					},
				},
				currentVersion: "v0.1.1", // current version linked to the first contract
			},
			want: []string{test.PreviousCAPIContractNotSupported, test.CurrentCAPIContract},
		},
		{
			name: "Multiple contracts (current and next), all valid for upgrades", // upgrade plan should report unsupported options
			field: field{
				metadata: &clusterctlv1.Metadata{ // metadata defining more release series, linked to different contracts
					ReleaseSeries: []clusterctlv1.ReleaseSeries{
						{Major: 0, Minor: 1, Contract: test.CurrentCAPIContract},
						{Major: 0, Minor: 2, Contract: test.NextCAPIContractNotSupported},
					},
				},
				currentVersion: "v0.1.1", // current version linked to the first contract
			},
			want: []string{test.CurrentCAPIContract, test.NextCAPIContractNotSupported},
		},
		{
			name: "Multiple contract supported (current and next), only one valid for upgrades", // upgrade plan should report unsupported options
			field: field{
				metadata: &clusterctlv1.Metadata{ // metadata defining more release series, linked to different contracts
					ReleaseSeries: []clusterctlv1.ReleaseSeries{
						{Major: 0, Minor: 1, Contract: test.PreviousCAPIContractNotSupported},
						{Major: 0, Minor: 2, Contract: test.CurrentCAPIContract},
					},
				},
				currentVersion: "v0.2.1", // current version linked to the second/the last contract, so the first one is not anymore valid for upgrades
			},
			want: []string{test.CurrentCAPIContract},
		},
		{
			name: "Current version does not match the release series",
			field: field{
				metadata:       &clusterctlv1.Metadata{},
				currentVersion: "v0.2.1",
			},
			want: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			upgradeInfo := newUpgradeInfo(tt.field.metadata, version.MustParseSemantic(tt.field.currentVersion), nil)

			got := upgradeInfo.getContractsForUpgrade()
			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func Test_upgradeInfo_getLatestNextVersion(t *testing.T) {
	type field struct {
		currentVersion string
		nextVersions   []string
		metadata       *clusterctlv1.Metadata
	}
	type args struct {
		contract string
	}
	tests := []struct {
		name  string
		field field
		args  args
		want  string
	}{
		{
			name: "Already up-to-date, no upgrade version",
			field: field{
				currentVersion: "v1.2.3",
				nextVersions:   []string{}, // Next versions empty
				metadata: &clusterctlv1.Metadata{
					ReleaseSeries: []clusterctlv1.ReleaseSeries{
						{Major: 1, Minor: 2, Contract: test.CurrentCAPIContract},
					},
				},
			},
			args: args{
				contract: test.CurrentCAPIContract,
			},
			want: "",
		},
		{
			name: "Find an upgrade version in the same release series, current contract",
			field: field{
				currentVersion: "v1.2.3",
				nextVersions:   []string{"v1.2.4", "v1.2.5"},
				metadata: &clusterctlv1.Metadata{
					ReleaseSeries: []clusterctlv1.ReleaseSeries{
						{Major: 1, Minor: 2, Contract: test.CurrentCAPIContract},
					},
				},
			},
			args: args{
				contract: test.CurrentCAPIContract,
			},
			want: "v1.2.5", // skipping v1.2.4 because it is not the latest version available
		},
		{
			name: "Find an upgrade version in the next release series, current contract",
			field: field{
				currentVersion: "v1.2.3",
				nextVersions:   []string{"v1.2.4", "v1.3.1", "v2.0.1", "v2.0.2"},
				metadata: &clusterctlv1.Metadata{
					ReleaseSeries: []clusterctlv1.ReleaseSeries{
						{Major: 1, Minor: 2, Contract: test.CurrentCAPIContract},
						{Major: 1, Minor: 3, Contract: test.CurrentCAPIContract},
						{Major: 2, Minor: 0, Contract: test.NextCAPIContractNotSupported},
					},
				},
			},
			args: args{
				contract: test.CurrentCAPIContract,
			},
			want: "v1.3.1", // skipping v1.2.4 because it is not the latest version available; ignoring v2.0.* because linked to a different contract
		},
		{
			name: "Find an upgrade version in the next contract", // upgrade plan should report unsupported options
			field: field{
				currentVersion: "v1.2.3",
				nextVersions:   []string{"v1.2.4", "v1.3.1", "v2.0.1", "v2.0.2"},
				metadata: &clusterctlv1.Metadata{
					ReleaseSeries: []clusterctlv1.ReleaseSeries{
						{Major: 1, Minor: 2, Contract: test.CurrentCAPIContract},
						{Major: 1, Minor: 3, Contract: test.CurrentCAPIContract},
						{Major: 2, Minor: 0, Contract: test.NextCAPIContractNotSupported},
					},
				},
			},
			args: args{
				contract: test.NextCAPIContractNotSupported,
			},
			want: "v2.0.2", // skipping v2.0.1 because it is not the latest version available; ignoring v1.* because linked to a different contract
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			upgradeInfo := newUpgradeInfo(tt.field.metadata, version.MustParseSemantic(tt.field.currentVersion), toSemanticVersions(tt.field.nextVersions))

			got := upgradeInfo.getLatestNextVersion(tt.args.contract)
			g.Expect(versionTag(got)).To(Equal(tt.want))
		})
	}
}

func toSemanticVersions(versions []string) []version.Version {
	semanticVersions := []version.Version{}
	for _, v := range versions {
		semanticVersions = append(semanticVersions, *version.MustParseSemantic(v))
	}
	return semanticVersions
}
