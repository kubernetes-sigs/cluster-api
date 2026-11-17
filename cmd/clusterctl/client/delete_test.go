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
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
)

const (
	namespace       = "foobar"
	providerVersion = "v1.0.0"
)

func Test_clusterctlClient_Delete(t *testing.T) {
	type fields struct {
		client *fakeClient
	}
	type args struct {
		options DeleteOptions
	}
	tests := []struct {
		name            string
		fields          fields
		args            args
		wantProviders   sets.Set[string]
		wantDeleteOrder []string
		wantErr         bool
		wantErrContains string
	}{
		{
			name: "Delete all the providers",
			fields: fields{
				client: fakeClusterForDelete(),
			},
			args: args{
				options: DeleteOptions{
					Kubeconfig:              Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"},
					IncludeNamespace:        false,
					IncludeCRDs:             false,
					SkipInventory:           false,
					CoreProvider:            "",
					BootstrapProviders:      nil,
					InfrastructureProviders: nil,
					ControlPlaneProviders:   nil,
					DeleteAll:               true, // delete all the providers
				},
			},
			wantProviders: sets.Set[string]{},
			// Core provider last, so a delete that fails part way leaves Cluster API in place.
			wantDeleteOrder: []string{
				clusterctlv1.ManifestLabel(infraProviderConfig.Name(), infraProviderConfig.Type()),
				clusterctlv1.ManifestLabel(controlPlaneProviderConfig.Name(), controlPlaneProviderConfig.Type()),
				clusterctlv1.ManifestLabel(bootstrapProviderConfig.Name(), bootstrapProviderConfig.Type()),
				clusterctlv1.ManifestLabel(capiProviderConfig.Name(), capiProviderConfig.Type()),
			},
			wantErr: false,
		},
		{
			name: "Delete all the providers including CRDs",
			fields: fields{
				client: fakeClusterForDelete(),
			},
			args: args{
				options: DeleteOptions{
					Kubeconfig:              Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"},
					IncludeNamespace:        false,
					IncludeCRDs:             true,
					SkipInventory:           false,
					CoreProvider:            "",
					BootstrapProviders:      nil,
					InfrastructureProviders: nil,
					ControlPlaneProviders:   nil,
					DeleteAll:               true, // delete all the providers
				},
			},
			wantProviders: sets.Set[string]{},
			wantErr:       false,
		},
		{
			// A previous run removed the Cluster CRD and then failed, so the
			// next run must still be able to delete what is left.
			name: "Delete all the providers when the Cluster CRD is already gone but the inventory lists providers",
			fields: fields{
				client: fakeClusterForDelete(inventoryCRD()),
			},
			args: args{
				options: DeleteOptions{
					Kubeconfig:       Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"},
					IncludeNamespace: true,
					IncludeCRDs:      true,
					DeleteAll:        true,
				},
			},
			wantProviders: sets.Set[string]{},
			wantErr:       false,
		},
		{
			name: "Refuse a management cluster with another contract",
			fields: fields{
				client: fakeClusterForDelete(&apiextensionsv1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{Name: "clusters.cluster.x-k8s.io"},
					Spec: apiextensionsv1.CustomResourceDefinitionSpec{
						Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{Name: "v1alpha3", Storage: true}},
					},
				}),
			},
			args: args{
				options: DeleteOptions{
					Kubeconfig: Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"},
					DeleteAll:  true,
				},
			},
			wantErr:         true,
			wantErrContains: "could be used only with",
		},
		{
			name: "Delete single provider auto-detect namespace",
			fields: fields{
				client: fakeClusterForDelete(),
			},
			args: args{
				options: DeleteOptions{
					Kubeconfig:              Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"},
					IncludeNamespace:        false,
					IncludeCRDs:             false,
					SkipInventory:           false,
					CoreProvider:            "",
					BootstrapProviders:      []string{bootstrapProviderConfig.Name()},
					InfrastructureProviders: nil,
					ControlPlaneProviders:   nil,
					DeleteAll:               false,
				},
			},
			wantProviders: sets.Set[string]{}.Insert(
				capiProviderConfig.Name(),
				clusterctlv1.ManifestLabel(controlPlaneProviderConfig.Name(), controlPlaneProviderConfig.Type()),
				clusterctlv1.ManifestLabel(infraProviderConfig.Name(), infraProviderConfig.Type())),
			wantErr: false,
		},
		{
			name: "Delete a provider with version mentioned",
			fields: fields{
				client: fakeClusterForDelete(),
			},
			args: args{
				options: DeleteOptions{
					Kubeconfig:              Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"},
					IncludeNamespace:        false,
					IncludeCRDs:             false,
					SkipInventory:           false,
					CoreProvider:            "",
					BootstrapProviders:      nil,
					InfrastructureProviders: []string{infraProviderConfig.Name() + ":" + providerVersion},
					ControlPlaneProviders:   nil,
					DeleteAll:               false,
				},
			},
			wantProviders: sets.Set[string]{}.Insert(
				capiProviderConfig.Name(),
				clusterctlv1.ManifestLabel(bootstrapProviderConfig.Name(), bootstrapProviderConfig.Type()),
				clusterctlv1.ManifestLabel(controlPlaneProviderConfig.Name(), controlPlaneProviderConfig.Type())),
			wantErr: false,
		},
		{
			name: "Delete a provider with wrong version",
			fields: fields{
				client: fakeClusterForDelete(),
			},
			args: args{
				options: DeleteOptions{
					Kubeconfig:              Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"},
					IncludeNamespace:        false,
					IncludeCRDs:             false,
					SkipInventory:           false,
					CoreProvider:            "",
					BootstrapProviders:      nil,
					InfrastructureProviders: []string{infraProviderConfig.Name() + ":v99.99.99"},
					ControlPlaneProviders:   nil,
					DeleteAll:               false,
				},
			},
			wantProviders: sets.Set[string]{}.Insert(
				capiProviderConfig.Name(),
				clusterctlv1.ManifestLabel(bootstrapProviderConfig.Name(), bootstrapProviderConfig.Type()),
				clusterctlv1.ManifestLabel(controlPlaneProviderConfig.Name(), controlPlaneProviderConfig.Type()),
				clusterctlv1.ManifestLabel(infraProviderConfig.Name(), infraProviderConfig.Type())),
			wantErr: true,
		},
		{
			name: "Delete multiple providers of different type",
			fields: fields{
				client: fakeClusterForDelete(),
			},
			args: args{
				options: DeleteOptions{
					Kubeconfig:              Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"},
					IncludeNamespace:        false,
					IncludeCRDs:             false,
					SkipInventory:           false,
					CoreProvider:            capiProviderConfig.Name(),
					BootstrapProviders:      []string{bootstrapProviderConfig.Name()},
					InfrastructureProviders: nil,
					ControlPlaneProviders:   nil,
					DeleteAll:               false,
				},
			},
			wantProviders: sets.Set[string]{}.Insert(
				clusterctlv1.ManifestLabel(controlPlaneProviderConfig.Name(), controlPlaneProviderConfig.Type()),
				clusterctlv1.ManifestLabel(infraProviderConfig.Name(), infraProviderConfig.Type())),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ctx := context.Background()

			input := cluster.Kubeconfig(tt.args.options.Kubeconfig)
			var deleted []string
			if tt.wantDeleteOrder != nil {
				tt.fields.client.clusters[input].Proxy().(*test.FakeProxy).WithInterceptorFuncs(interceptor.Funcs{
					Delete: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
						if obj.GetObjectKind().GroupVersionKind().Kind == "Provider" {
							deleted = append(deleted, obj.GetName())
						}
						return cl.Delete(ctx, obj, opts...)
					},
				})
			}

			err := tt.fields.client.Delete(ctx, tt.args.options)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				if tt.wantErrContains != "" {
					g.Expect(err).To(MatchError(ContainSubstring(tt.wantErrContains)))
				}
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			if tt.wantDeleteOrder != nil {
				g.Expect(deleted).To(Equal(tt.wantDeleteOrder))
			}

			proxy := tt.fields.client.clusters[input].Proxy()
			gotProviders := &clusterctlv1.ProviderList{}

			c, err := proxy.NewClient(ctx)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(c.List(ctx, gotProviders)).To(Succeed())

			gotProvidersSet := sets.Set[string]{}
			for _, gotProvider := range gotProviders.Items {
				gotProvidersSet.Insert(gotProvider.Name)
			}

			g.Expect(gotProvidersSet).To(BeComparableTo(tt.wantProviders))
		})
	}
}

// inventoryCRD is the clusterctl inventory CRD as a management cluster has it.
func inventoryCRD() client.Object {
	return &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("providers.%s", clusterctlv1.GroupVersion.Group)},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{Name: clusterctlv1.GroupVersion.Version, Storage: true}},
		},
	}
}

func Test_sortProvidersForDelete(t *testing.T) {
	g := NewWithT(t)
	provider := func(name string, providerType clusterctlv1.ProviderType) clusterctlv1.Provider {
		return clusterctlv1.Provider{ProviderName: name, Type: string(providerType)}
	}
	// The order List returns them in: by namespace and name.
	providers := []clusterctlv1.Provider{
		provider("cluster-api", clusterctlv1.CoreProviderType),
		provider("kubeadm", clusterctlv1.BootstrapProviderType),
		provider("kubeadm", clusterctlv1.ControlPlaneProviderType),
		provider("vsphere", clusterctlv1.InfrastructureProviderType),
		provider("in-cluster", clusterctlv1.IPAMProviderType),
		provider("test-extension", clusterctlv1.RuntimeExtensionProviderType),
		provider("helm", clusterctlv1.AddonProviderType),
	}
	sortProvidersForDelete(providers)
	var got []string
	for _, p := range providers {
		got = append(got, p.Type)
	}
	g.Expect(got).To(Equal([]string{
		string(clusterctlv1.AddonProviderType),
		string(clusterctlv1.RuntimeExtensionProviderType),
		string(clusterctlv1.IPAMProviderType),
		string(clusterctlv1.InfrastructureProviderType),
		string(clusterctlv1.ControlPlaneProviderType),
		string(clusterctlv1.BootstrapProviderType),
		string(clusterctlv1.CoreProviderType),
	}))
}

// clusterctl client for a management cluster with capi and bootstrap provider.
// objs, if any, are in the management cluster instead of the Cluster API setup objects.
func fakeClusterForDelete(objs ...client.Object) *fakeClient {
	ctx := context.Background()

	if len(objs) == 0 {
		objs = fakeCAPISetupObjects()
	}

	config1 := newFakeConfig(ctx).
		WithVar("var", "value").
		WithProvider(capiProviderConfig).
		WithProvider(bootstrapProviderConfig).
		WithProvider(controlPlaneProviderConfig).
		WithProvider(infraProviderConfig)

	repository1 := newFakeRepository(ctx, capiProviderConfig, config1)
	repository2 := newFakeRepository(ctx, bootstrapProviderConfig, config1)
	repository3 := newFakeRepository(ctx, controlPlaneProviderConfig, config1)
	repository4 := newFakeRepository(ctx, infraProviderConfig, config1)

	cluster1 := newFakeCluster(cluster.Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"}, config1)
	cluster1.fakeProxy.WithProviderInventory(capiProviderConfig.Name(), capiProviderConfig.Type(), providerVersion, "capi-system")
	cluster1.fakeProxy.WithProviderInventory(bootstrapProviderConfig.Name(), bootstrapProviderConfig.Type(), providerVersion, "capi-kubeadm-bootstrap-system")
	cluster1.fakeProxy.WithProviderInventory(controlPlaneProviderConfig.Name(), controlPlaneProviderConfig.Type(), providerVersion, "capi-kubeadm-control-plane-system")
	cluster1.fakeProxy.WithProviderInventory(infraProviderConfig.Name(), infraProviderConfig.Type(), providerVersion, namespace)
	cluster1.fakeProxy.WithObjs(objs...)

	client := newFakeClient(ctx, config1).
		// fake repository for capi, bootstrap, controlplane and infra provider (matching provider's config)
		WithRepository(repository1).
		WithRepository(repository2).
		WithRepository(repository3).
		WithRepository(repository4).
		// fake empty cluster
		WithCluster(cluster1)

	return client
}
