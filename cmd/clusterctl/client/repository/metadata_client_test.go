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
	"testing"

	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
)

var metadataYaml = []byte("apiVersion: clusterctl.cluster.x-k8s.io/v1alpha3\n" +
	"kind: Metadata\n" +
	"releaseSeries:\n" +
	" - major: 1\n" +
	"   minor: 2\n" +
	"   contract: v1alpha3\n" +
	"")

func Test_metadataClient_Get(t *testing.T) {
	type fields struct {
		provider   config.Provider
		version    string
		repository Repository
	}
	tests := []struct {
		name    string
		fields  fields
		want    *clusterctlv1.Metadata
		wantErr bool
	}{
		{
			name: "Pass",
			fields: fields{
				provider: config.NewProvider("p1", "", clusterctlv1.CoreProviderType),
				version:  "v1.0.0",
				repository: test.NewFakeRepository().
					WithPaths("root", "").
					WithDefaultVersion("v1.0.0").
					WithFile("v1.0.0", "metadata.yaml", metadataYaml),
			},
			want: &clusterctlv1.Metadata{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "clusterctl.cluster.x-k8s.io/v1alpha3",
					Kind:       "Metadata",
				},
				ReleaseSeries: []clusterctlv1.ReleaseSeries{
					{
						Major:    1,
						Minor:    2,
						Contract: "v1alpha3",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Pass with embedded metadata",
			fields: fields{
				provider: config.NewProvider(config.ClusterAPIProviderName, "", clusterctlv1.CoreProviderType),
				version:  "v1.0.0",
				repository: test.NewFakeRepository(). //repository without a metadata file
									WithPaths("root", "").
									WithDefaultVersion("v1.0.0"),
			},
			want: &clusterctlv1.Metadata{
				TypeMeta: metav1.TypeMeta{
					APIVersion: clusterctlv1.GroupVersion.String(),
					Kind:       "Metadata",
				},
				ReleaseSeries: []clusterctlv1.ReleaseSeries{
					{Major: 0, Minor: 3, Contract: "v1alpha3"},
					{Major: 0, Minor: 2, Contract: "v1alpha2"},
				},
			},
			wantErr: false,
		},
		{
			name: "Fails if the file does not exists",
			fields: fields{
				provider: config.NewProvider("p1", "", clusterctlv1.CoreProviderType),
				version:  "v1.0.0",
				repository: test.NewFakeRepository(). //repository without a metadata file
									WithPaths("root", "").
									WithDefaultVersion("v1.0.0"),
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "Fails if the file does not exists for the current version",
			fields: fields{
				provider: config.NewProvider("p1", "", clusterctlv1.CoreProviderType),
				version:  "v1.0.0",
				repository: test.NewFakeRepository().
					WithPaths("root", "").
					WithDefaultVersion("v2.0.0").
					WithFile("v2.0.0", "metadata.yaml", metadataYaml), // metadata file exists for version 2.0.0, while we are checking metadata for v1.0.0
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "Fails if the file isn't a valid metadata",
			fields: fields{
				provider: config.NewProvider("p1", "", clusterctlv1.CoreProviderType),
				version:  "v1.0.0",
				repository: test.NewFakeRepository().
					WithPaths("root", "").
					WithDefaultVersion("v2.0.0").
					WithFile("v2.0.0", "metadata.yaml", []byte("not a valid metadata file!")), // metadata file exists but is invalid
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			f := &metadataClient{
				configVarClient: test.NewFakeVariableClient(),
				provider:        tt.fields.provider,
				version:         tt.fields.version,
				repository:      tt.fields.repository,
			}
			got, err := f.Get()
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())

			g.Expect(got).To(Equal(tt.want))
		})
	}
}
