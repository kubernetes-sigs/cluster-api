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

package cluster

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
)

func fakePollImmediateWaiter(_ context.Context, _, _ time.Duration, _ wait.ConditionWithContextFunc) error {
	return nil
}

func Test_inventoryClient_CheckInventoryCRDs(t *testing.T) {
	type fields struct {
		alreadyHasCRD bool
	}
	tests := []struct {
		name    string
		fields  fields
		want    bool
		wantErr bool
	}{
		{
			name: "Has not CRD",
			fields: fields{
				alreadyHasCRD: false,
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "Already has CRD",
			fields: fields{
				alreadyHasCRD: true,
			},
			want:    true,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ctx := context.Background()

			proxy := test.NewFakeProxy()
			p := newInventoryClient(proxy, fakePollImmediateWaiter, currentContractVersion)
			if tt.fields.alreadyHasCRD {
				// forcing creation of metadata before test
				g.Expect(p.EnsureCustomResourceDefinitions(ctx)).To(Succeed())
			}

			res, err := checkInventoryCRDs(ctx, proxy)
			g.Expect(res).To(Equal(tt.want))
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

var fooProvider = clusterctlv1.Provider{ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "ns1", ResourceVersion: "999"}}
var v1alpha4Contract = "v1alpha4"

func Test_inventoryClient_List(t *testing.T) {
	type fields struct {
		initObjs []client.Object
	}
	tests := []struct {
		name    string
		fields  fields
		want    []clusterctlv1.Provider
		wantErr bool
	}{
		{
			name: "Get list",
			fields: fields{
				initObjs: []client.Object{
					&fooProvider,
				},
			},
			want: []clusterctlv1.Provider{
				fooProvider,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			p := newInventoryClient(test.NewFakeProxy().WithObjs(tt.fields.initObjs...), fakePollImmediateWaiter, currentContractVersion)
			got, err := p.List(context.Background())
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got.Items).To(ConsistOf(tt.want))
		})
	}
}

func Test_inventoryClient_Create(t *testing.T) {
	type fields struct {
		proxy Proxy
	}
	type args struct {
		m clusterctlv1.Provider
	}
	providerV2 := fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v0.2.0", "")
	// since this test object is used in a Create request, wherein setting ResourceVersion should no be set
	providerV2.ResourceVersion = ""
	providerV3 := fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v0.3.0", "")

	tests := []struct {
		name          string
		fields        fields
		args          args
		wantProviders []clusterctlv1.Provider
		wantErr       bool
	}{
		{
			name: "Creates a provider",
			fields: fields{
				proxy: test.NewFakeProxy(),
			},
			args: args{
				m: providerV2,
			},
			wantProviders: []clusterctlv1.Provider{
				providerV2,
			},
			wantErr: false,
		},
		{
			name: "Patches a provider",
			fields: fields{
				proxy: test.NewFakeProxy().WithObjs(&providerV2),
			},
			args: args{
				m: providerV3,
			},
			wantProviders: []clusterctlv1.Provider{
				providerV3,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ctx := context.Background()

			p := &inventoryClient{
				proxy: tt.fields.proxy,
			}
			err := p.Create(ctx, tt.args.m)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).ToNot(HaveOccurred())

			got, err := p.List(ctx)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).ToNot(HaveOccurred())

			for i := range got.Items {
				tt.wantProviders[i].ResourceVersion = got.Items[i].ResourceVersion
			}

			g.Expect(got.Items).To(ConsistOf(tt.wantProviders))
		})
	}
}

func Test_CheckCAPIContract(t *testing.T) {
	type args struct {
		options []CheckCAPIContractOption
	}
	type fields struct {
		proxy Proxy
	}

	tests := []struct {
		name            string
		fields          fields
		args            args
		wantErr         bool
		wantErrContains string
	}{
		{
			name: "Fails if Cluster API is not installed",
			fields: fields{
				proxy: test.NewFakeProxy().WithObjs(),
			},
			args:    args{},
			wantErr: true,
		},
		{
			name: "Fails if the Cluster CRD is gone and the inventory lists providers, without AllowCAPINotInstalledWithInventory",
			fields: fields{
				proxy: inventoryWithProviders(clusterctlv1.GroupVersion.Version),
			},
			args:            args{},
			wantErr:         true,
			wantErrContains: "failed to check Cluster API version",
		},
		{
			name: "Fails if the Cluster CRD is gone and there is no inventory CRD, with AllowCAPINotInstalledWithInventory",
			fields: fields{
				proxy: test.NewFakeProxy().
					WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v1.0.0", "infra-system"),
			},
			args: args{
				options: []CheckCAPIContractOption{AllowCAPINotInstalledWithInventory{}},
			},
			wantErr:         true,
			wantErrContains: "failed to check Cluster API version",
		},
		{
			name: "Fails if the Cluster CRD is gone and the inventory lists no providers, with AllowCAPINotInstalledWithInventory",
			fields: fields{
				proxy: test.NewFakeProxy().WithObjs(inventoryCRDWithVersion(clusterctlv1.GroupVersion.Version)),
			},
			args: args{
				options: []CheckCAPIContractOption{AllowCAPINotInstalledWithInventory{}},
			},
			wantErr:         true,
			wantErrContains: "failed to check Cluster API version",
		},
		{
			name: "Fails if the Cluster CRD is gone and the inventory CRD has another version, with AllowCAPINotInstalledWithInventory",
			fields: fields{
				proxy: inventoryWithProviders("v1alpha2"),
			},
			args: args{
				options: []CheckCAPIContractOption{AllowCAPINotInstalledWithInventory{}},
			},
			wantErr:         true,
			wantErrContains: "clusterctl inventory CRD does not defines the",
		},
		{
			name: "Fails if reading the Cluster CRD fails for another reason, with AllowCAPINotInstalledWithInventory",
			fields: fields{
				proxy: inventoryWithProviders(clusterctlv1.GroupVersion.Version).
					WithInterceptorFuncs(forbidGet(fmt.Sprintf("clusters.%s", clusterv1.GroupVersion.Group))),
			},
			args: args{
				options: []CheckCAPIContractOption{AllowCAPINotInstalledWithInventory{}},
			},
			wantErr:         true,
			wantErrContains: "is forbidden",
		},
		{
			name: "Fails if reading the inventory CRD fails, with AllowCAPINotInstalledWithInventory",
			fields: fields{
				proxy: inventoryWithProviders(clusterctlv1.GroupVersion.Version).
					WithInterceptorFuncs(forbidGet(fmt.Sprintf("providers.%s", clusterctlv1.GroupVersion.Group))),
			},
			args: args{
				options: []CheckCAPIContractOption{AllowCAPINotInstalledWithInventory{}},
			},
			wantErr:         true,
			wantErrContains: "failed to check if the clusterctl inventory CRD exists",
		},
		{
			name: "Fails if listing the inventory fails, with AllowCAPINotInstalledWithInventory",
			fields: fields{
				proxy: inventoryWithProviders(clusterctlv1.GroupVersion.Version).
					WithInterceptorFuncs(interceptor.Funcs{
						List: func(_ context.Context, _ client.WithWatch, _ client.ObjectList, _ ...client.ListOption) error {
							return apierrors.NewForbidden(schema.GroupResource{Group: clusterctlv1.GroupVersion.Group, Resource: "providers"}, "", nil)
						},
					}),
			},
			args: args{
				options: []CheckCAPIContractOption{AllowCAPINotInstalledWithInventory{}},
			},
			wantErr:         true,
			wantErrContains: "failed to list providers in the clusterctl inventory",
		},
		{
			name: "Pass if the Cluster CRD is gone but the inventory lists providers, with AllowCAPINotInstalledWithInventory",
			fields: fields{
				proxy: inventoryWithProviders(clusterctlv1.GroupVersion.Version),
			},
			args: args{
				options: []CheckCAPIContractOption{AllowCAPINotInstalledWithInventory{}},
			},
			wantErr: false,
		},
		{
			name: "Pass if Cluster API is not installed, but this is explicitly tolerated",
			fields: fields{
				proxy: test.NewFakeProxy().WithObjs(),
			},
			args: args{
				options: []CheckCAPIContractOption{AllowCAPINotInstalled{}},
			},
			wantErr: false,
		},
		{
			name: "Pass when Cluster API with current contract is installed",
			fields: fields{
				proxy: test.NewFakeProxy().WithObjs(&apiextensionsv1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{Name: "clusters.cluster.x-k8s.io"},
					Spec: apiextensionsv1.CustomResourceDefinitionSpec{
						Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
							{
								Name: oldContractVersionNotSupportedAnymore,
							},
							{
								Name:    currentContractVersion,
								Storage: true,
							},
						},
					},
				}),
			},
			args:    args{},
			wantErr: false,
		},
		{
			name: "Fails when Cluster API with previous contract is installed",
			fields: fields{
				proxy: test.NewFakeProxy().WithObjs(&apiextensionsv1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{Name: "clusters.cluster.x-k8s.io"},
					Spec: apiextensionsv1.CustomResourceDefinitionSpec{
						Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
							{
								Name:    oldContractVersionNotSupportedAnymore,
								Storage: true,
							},
							{
								Name: currentContractVersion,
							},
						},
					},
				}),
			},
			args:    args{},
			wantErr: true,
		},
		{
			name: "Pass when Cluster API with previous contract is installed, but this is explicitly tolerated",
			fields: fields{
				proxy: test.NewFakeProxy().WithObjs(&apiextensionsv1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{Name: "clusters.cluster.x-k8s.io"},
					Spec: apiextensionsv1.CustomResourceDefinitionSpec{
						Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
							{
								Name:    oldContractVersionNotSupportedAnymore,
								Storage: true,
							},
							{
								Name: currentContractVersion,
							},
						},
					},
				}),
			},
			args: args{
				options: []CheckCAPIContractOption{AllowCAPIContract{Contract: v1alpha4Contract}, AllowCAPIContract{Contract: oldContractVersionNotSupportedAnymore}},
			},
			wantErr: false,
		},
		{
			name: "Fails when Cluster API with next contract is installed",
			fields: fields{
				proxy: test.NewFakeProxy().WithObjs(&apiextensionsv1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{Name: "clusters.cluster.x-k8s.io"},
					Spec: apiextensionsv1.CustomResourceDefinitionSpec{
						Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
							{
								Name: currentContractVersion,
							},
							{
								Name:    nextContractVersionNotSupportedYet,
								Storage: true,
							},
						},
					},
				}),
			},
			args:    args{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			p := &inventoryClient{
				proxy:                  tt.fields.proxy,
				currentContractVersion: currentContractVersion,
			}
			err := p.CheckCAPIContract(context.Background(), tt.args.options...)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				if tt.wantErrContains != "" {
					g.Expect(err).To(MatchError(ContainSubstring(tt.wantErrContains)))
				}
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
		})
	}
}

func Test_inventoryClient_CheckSingleProviderInstance(t *testing.T) {
	type fields struct {
		initObjs []client.Object
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		{
			name: "Returns error when there are multiple instances of the same provider",
			fields: fields{
				initObjs: []client.Object{
					&clusterctlv1.Provider{Type: string(clusterctlv1.CoreProviderType), ProviderName: "foo", ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "ns1"}},
					&clusterctlv1.Provider{Type: string(clusterctlv1.CoreProviderType), ProviderName: "foo", ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "ns2"}},
					&clusterctlv1.Provider{Type: string(clusterctlv1.InfrastructureProviderType), ProviderName: "bar", ObjectMeta: metav1.ObjectMeta{Name: "bar", Namespace: "ns2"}},
				},
			},
			wantErr: true,
		},
		{
			name: "Does not return error when there is only single instance of all providers",
			fields: fields{
				initObjs: []client.Object{
					&clusterctlv1.Provider{Type: string(clusterctlv1.CoreProviderType), ProviderName: "foo", ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "ns1"}},
					&clusterctlv1.Provider{Type: string(clusterctlv1.CoreProviderType), ProviderName: "foo-1", ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "ns2"}},
					&clusterctlv1.Provider{Type: string(clusterctlv1.InfrastructureProviderType), ProviderName: "bar", ObjectMeta: metav1.ObjectMeta{Name: "bar", Namespace: "ns2"}},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			p := newInventoryClient(test.NewFakeProxy().WithObjs(tt.fields.initObjs...), fakePollImmediateWaiter, currentContractVersion)
			err := p.CheckSingleProviderInstance(context.Background())
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).ToNot(HaveOccurred())
		})
	}
}

// inventoryCRDWithVersion returns the clusterctl inventory CRD serving the given version.
func inventoryCRDWithVersion(version string) *apiextensionsv1.CustomResourceDefinition {
	return &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("providers.%s", clusterctlv1.GroupVersion.Group)},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{Name: version, Storage: true}},
		},
	}
}

// inventoryWithProviders returns a cluster without the Cluster CRD whose inventory, served at the given version,
// still lists a provider.
func inventoryWithProviders(version string) *test.FakeProxy {
	return test.NewFakeProxy().WithObjs(inventoryCRDWithVersion(version)).
		WithProviderInventory("infra", clusterctlv1.InfrastructureProviderType, "v1.0.0", "infra-system")
}

// forbidGet makes Get of the CRD with the given name fail as forbidden.
func forbidGet(name string) interceptor.Funcs {
	return interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if key.Name == name {
				return apierrors.NewForbidden(schema.GroupResource{Group: "apiextensions.k8s.io", Resource: "customresourcedefinitions"}, key.Name, nil)
			}
			return c.Get(ctx, key, obj, opts...)
		},
	}
}
