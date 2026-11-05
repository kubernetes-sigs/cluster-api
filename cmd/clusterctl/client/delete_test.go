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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
			wantErr:       false,
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
				client: fakeClusterForDeleteWithObjs(inventoryCRD()),
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
				client: fakeClusterForDeleteWithObjs(&apiextensionsv1.CustomResourceDefinition{
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

			err := tt.fields.client.Delete(ctx, tt.args.options)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				if tt.wantErrContains != "" {
					g.Expect(err).To(MatchError(ContainSubstring(tt.wantErrContains)))
				}
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			input := cluster.Kubeconfig(tt.args.options.Kubeconfig)
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

// Test_clusterctlClient_DeleteWithoutClusterAPI checks that delete against a cluster that has neither Cluster API
// nor the clusterctl inventory fails, as for a wrong context, and does not create the inventory CRD.
func Test_clusterctlClient_DeleteWithoutClusterAPI(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	c := fakeClusterForDeleteWithObjs()
	kubeconfig := Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"}
	g.Expect(c.Delete(ctx, DeleteOptions{Kubeconfig: kubeconfig, DeleteAll: true})).ToNot(Succeed())

	cl, err := c.clusters[cluster.Kubeconfig(kubeconfig)].Proxy().NewClient(ctx)
	g.Expect(err).ToNot(HaveOccurred())
	err = cl.Get(ctx, client.ObjectKeyFromObject(inventoryCRD()), &apiextensionsv1.CustomResourceDefinition{})
	g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "the inventory CRD was created: %v", err)
}

// Test_clusterctlClient_DeleteTwice checks that once a delete without the Cluster CRD has emptied the inventory,
// another delete fails as it does on a cluster without Cluster API, even though the inventory CRD is still there.
func Test_clusterctlClient_DeleteTwice(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	c := fakeClusterForDeleteWithObjs(inventoryCRD())
	options := DeleteOptions{Kubeconfig: Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"}, DeleteAll: true}
	g.Expect(c.Delete(ctx, options)).To(Succeed())
	g.Expect(c.Delete(ctx, options)).To(MatchError(ContainSubstring("failed to check Cluster API version")))
}

func Test_clusterctlClient_DeleteOrder(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	c := fakeClusterForDelete()
	kubeconfig := Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"}
	var deleted []string
	c.clusters[cluster.Kubeconfig(kubeconfig)].Proxy().(*test.FakeProxy).WithInterceptorFuncs(interceptor.Funcs{
		Delete: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
			if obj.GetObjectKind().GroupVersionKind().Kind == "Provider" {
				deleted = append(deleted, obj.GetName())
			}
			return cl.Delete(ctx, obj, opts...)
		},
	})
	g.Expect(c.Delete(ctx, DeleteOptions{Kubeconfig: kubeconfig, DeleteAll: true})).To(Succeed())
	g.Expect(deleted).To(Equal([]string{
		clusterctlv1.ManifestLabel(infraProviderConfig.Name(), infraProviderConfig.Type()),
		clusterctlv1.ManifestLabel(controlPlaneProviderConfig.Name(), controlPlaneProviderConfig.Type()),
		clusterctlv1.ManifestLabel(bootstrapProviderConfig.Name(), bootstrapProviderConfig.Type()),
		clusterctlv1.ManifestLabel(capiProviderConfig.Name(), capiProviderConfig.Type()),
	}))
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
func fakeClusterForDelete() *fakeClient {
	return fakeClusterForDeleteWithObjs(fakeCAPISetupObjects()...)
}

// fakeClusterForDeleteWithObjs is fakeClusterForDelete with objs in the
// management cluster instead of the Cluster API setup objects.
func fakeClusterForDeleteWithObjs(objs ...client.Object) *fakeClient {
	ctx := context.Background()

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
