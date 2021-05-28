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
	"testing"
	"time"

	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func fakePollImmediateWaiter(interval, timeout time.Duration, condition wait.ConditionFunc) error {
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

			proxy := test.NewFakeProxy()
			p := newInventoryClient(proxy, fakePollImmediateWaiter)
			if tt.fields.alreadyHasCRD {
				// forcing creation of metadata before test
				g.Expect(p.EnsureCustomResourceDefinitions()).To(Succeed())
			}

			res, err := checkInventoryCRDs(proxy)
			g.Expect(res).To(Equal(tt.want))
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
		})
	}
}

var fooProvider = clusterctlv1.Provider{ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "ns1", ResourceVersion: "999"}}

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

			p := newInventoryClient(test.NewFakeProxy().WithObjs(tt.fields.initObjs...), fakePollImmediateWaiter)
			got, err := p.List()
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).NotTo(HaveOccurred())
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

			p := &inventoryClient{
				proxy: tt.fields.proxy,
			}
			err := p.Create(tt.args.m)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).NotTo(HaveOccurred())

			got, err := p.List()
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).NotTo(HaveOccurred())

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
		name    string
		fields  fields
		args    args
		wantErr bool
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
								Name: test.PreviousCAPIContractNotSupported,
							},
							{
								Name:    test.CurrentCAPIContract,
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
								Name:    test.PreviousCAPIContractNotSupported,
								Storage: true,
							},
							{
								Name: test.CurrentCAPIContract,
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
								Name:    test.PreviousCAPIContractNotSupported,
								Storage: true,
							},
							{
								Name: test.CurrentCAPIContract,
							},
						},
					},
				}),
			},
			args: args{
				options: []CheckCAPIContractOption{AllowCAPIContract{Contract: test.PreviousCAPIContractNotSupported}},
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
								Name: test.CurrentCAPIContract,
							},
							{
								Name:    test.NextCAPIContractNotSupported,
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
				proxy: tt.fields.proxy,
			}
			err := p.CheckCAPIContract(tt.args.options...)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())
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

			p := newInventoryClient(test.NewFakeProxy().WithObjs(tt.fields.initObjs...), fakePollImmediateWaiter)
			err := p.CheckSingleProviderInstance()
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).NotTo(HaveOccurred())
		})
	}
}
