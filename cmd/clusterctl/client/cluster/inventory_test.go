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
	"testing"
	"time"

	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clusterctlv1old "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha4"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func fakePollImmediateWaiter(interval, timeout time.Duration, condition wait.ConditionFunc) error {
	return nil
}

func Test_inventoryClient_CheckInventoryCRDs(t *testing.T) {
	type args struct {
		tolerateContract string
	}
	type fields struct {
		hasCRDVersion string
	}
	tests := []struct {
		name       string
		args       args
		fields     fields
		wantResult bool
		wantErr    bool
	}{
		{
			name: "Has not CRD",
			args: args{},
			fields: fields{
				hasCRDVersion: "",
			},
			wantResult: false,
			wantErr:    false,
		},
		{
			name: "Already has CRD",
			args: args{},
			fields: fields{
				hasCRDVersion: clusterctlv1.GroupVersion.Version,
			},
			wantResult: true,
			wantErr:    false,
		},
		{
			name: "Already has CRD but in the old version (not explicitly tolerated by the command)",
			args: args{},
			fields: fields{
				hasCRDVersion: clusterctlv1old.GroupVersion.Version,
			},
			wantResult: true,
			wantErr:    true,
		},
		{
			name: "Already has CRD but in the old version, tolerated by the command",
			args: args{
				tolerateContract: clusterctlv1old.GroupVersion.Version,
			},
			fields: fields{
				hasCRDVersion: clusterctlv1old.GroupVersion.Version,
			},
			wantResult: true,
			wantErr:    false,
		},
		{
			name: "Already has CRD but in another version, which is not the tolerated one",
			args: args{
				tolerateContract: clusterctlv1old.GroupVersion.Version,
			},
			fields: fields{
				hasCRDVersion: "anotherVersion",
			},
			wantResult: true,
			wantErr:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			proxy := test.NewFakeProxy()
			if tt.fields.hasCRDVersion != "" {
				crd := &apiextensionsv1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{Name: "providers.clusterctl.cluster.x-k8s.io"},
					Spec: apiextensionsv1.CustomResourceDefinitionSpec{
						Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
							{
								Name: tt.fields.hasCRDVersion,
							},
						},
					},
				}
				c, err := proxy.NewClient()
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(c.Create(context.TODO(), crd)).ToNot(HaveOccurred())
			}

			res, err := checkInventoryCRDs(proxy, tt.args.tolerateContract)
			g.Expect(res).To(Equal(tt.wantResult))
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
		})
	}
}

func Test_inventoryClient_List(t *testing.T) {

	var fooProvider = clusterctlv1.Provider{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Provider",
			APIVersion: clusterctlv1.GroupVersion.String(),
		}, ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "ns1", ResourceVersion: "1",
		},
	}
	var oldFooProvider = clusterctlv1old.Provider{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Provider",
			APIVersion: clusterctlv1old.GroupVersion.String(),
		}, ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "ns1", ResourceVersion: "1",
		},
	}

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
		{
			name: "Get list when the cluster is still with old providers types",
			fields: fields{
				initObjs: []client.Object{
					&oldFooProvider,
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
	providerV2 := fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v0.2.0", "", "")
	// since this test object is used in a Create request, wherein setting ResourceVersion should no be set
	providerV2.ResourceVersion = ""
	providerV3 := fakeProvider("infra", clusterctlv1.InfrastructureProviderType, "v0.3.0", "", "")

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
