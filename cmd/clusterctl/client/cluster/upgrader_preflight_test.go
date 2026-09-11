/*
Copyright 2025 The Kubernetes Authors.

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

	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/repository"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
)

func fakeCRD(name, group, kind string, versions ...string) *apiextensionsv1.CustomResourceDefinition {
	crd := &apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			APIVersion: apiextensionsv1.SchemeGroupVersion.String(),
			Kind:       "CustomResourceDefinition",
		},
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: group,
			Names: apiextensionsv1.CustomResourceDefinitionNames{Kind: kind},
		},
	}
	for _, v := range versions {
		crd.Spec.Versions = append(crd.Spec.Versions, apiextensionsv1.CustomResourceDefinitionVersion{
			Name:    v,
			Served:  true,
			Storage: v == versions[len(versions)-1],
		})
	}
	return crd
}

func fakeCRDYAML(name, group, kind string, versions ...string) []byte {
	out, _ := yaml.Marshal(fakeCRD(name, group, kind, versions...))
	return out
}

func Test_providerUpgrader_checkClusterClassRefs(t *testing.T) {
	const (
		infraGroup = "infrastructure.cluster.x-k8s.io"
		infraKind  = "TestInfraClusterTemplate"
		crdName    = "testinfraclustertemplates.infrastructure.cluster.x-k8s.io"
	)

	infraRepo := func(newVersions ...string) *repository.MemoryRepository {
		return repository.NewMemoryRepository().
			WithPaths("", "components.yaml").
			WithVersions("v2.0.0", "v2.0.1").
			WithFile("v2.0.1", "components.yaml", fakeCRDYAML(crdName, infraGroup, infraKind, newVersions...)).
			WithMetadata("v2.0.1", &clusterctlv1.Metadata{
				ReleaseSeries: []clusterctlv1.ReleaseSeries{
					{Major: 2, Minor: 0, Contract: currentContractVersion},
				},
			})
	}

	infraPlan := &UpgradePlan{
		Providers: []UpgradeItem{{
			Provider:    fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system"),
			NextVersion: "v2.0.1",
		}},
	}

	reader := test.NewFakeReader().
		WithProvider("infra", clusterctlv1.InfrastructureProviderType, "https://somewhere.com")

	tests := []struct {
		name     string
		repo     *repository.MemoryRepository
		proxy    *test.FakeProxy
		plan     *UpgradePlan
		wantErr  bool
		errorMsg string
	}{
		{
			name: "no versions dropped: no error",
			repo: infraRepo("v1beta1", "v1beta2"),
			proxy: test.NewFakeProxy().
				WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system").
				WithObjs(fakeCRD(crdName, infraGroup, infraKind, "v1beta1", "v1beta2")),
			plan:    infraPlan,
			wantErr: false,
		},
		{
			name: "version dropped, no ClusterClasses: no error",
			repo: infraRepo("v1beta2"),
			proxy: test.NewFakeProxy().
				WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system").
				WithObjs(fakeCRD(crdName, infraGroup, infraKind, "v1beta1", "v1beta2")),
			plan:    infraPlan,
			wantErr: false,
		},
		{
			name: "version dropped, ClusterClass pins dropped version: error",
			repo: infraRepo("v1beta2"),
			proxy: test.NewFakeProxy().
				WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system").
				WithObjs(
					fakeCRD(crdName, infraGroup, infraKind, "v1beta1", "v1beta2"),
					&clusterv1.ClusterClass{
						ObjectMeta: metav1.ObjectMeta{Name: "cc1", Namespace: "default"},
						Spec: clusterv1.ClusterClassSpec{
							Infrastructure: clusterv1.InfrastructureClass{
								TemplateRef: clusterv1.ClusterClassTemplateReference{
									APIVersion: infraGroup + "/v1beta1",
									Kind:       infraKind,
								},
							},
						},
					},
				),
			plan:     infraPlan,
			wantErr:  true,
			errorMsg: "default/cc1",
		},
		{
			name: "version dropped, ClusterClass uses newer version: no error",
			repo: infraRepo("v1beta2"),
			proxy: test.NewFakeProxy().
				WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v2.0.0", "infra-system").
				WithObjs(
					fakeCRD(crdName, infraGroup, infraKind, "v1beta1", "v1beta2"),
					&clusterv1.ClusterClass{
						ObjectMeta: metav1.ObjectMeta{Name: "cc1", Namespace: "default"},
						Spec: clusterv1.ClusterClassSpec{
							Infrastructure: clusterv1.InfrastructureClass{
								TemplateRef: clusterv1.ClusterClassTemplateReference{
									APIVersion: infraGroup + "/v1beta2",
									Kind:       infraKind,
								},
							},
						},
					},
				),
			plan:    infraPlan,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ctx := context.Background()

			configClient, _ := config.New(ctx, "", config.InjectReader(reader))

			u := &providerUpgrader{
				configClient: configClient,
				repositoryClientFactory: func(ctx context.Context, provider config.Provider, configClient config.Client, _ ...repository.Option) (repository.Client, error) {
					return repository.New(ctx, provider, configClient, repository.InjectRepository(tt.repo))
				},
				providerInventory:             newInventoryClient(tt.proxy, nil, currentContractVersion),
				proxy:                         tt.proxy,
				currentContractVersion:        currentContractVersion,
				getCompatibleContractVersions: getCompatibleContractVersions,
			}

			err := u.checkClusterClassRefs(ctx, tt.plan)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).Should(ContainSubstring(tt.errorMsg))
				return
			}

			g.Expect(err).ToNot(HaveOccurred())
		})
	}
}
