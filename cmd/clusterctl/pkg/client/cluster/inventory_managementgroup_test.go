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
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/test"
)

func Test_inventoryClient_GetManagementGroups(t *testing.T) {
	type fields struct {
		proxy Proxy
	}
	tests := []struct {
		name    string
		fields  fields
		want    []ManagementGroup
		wantErr bool
	}{
		{
			name: "Simple management cluster",
			fields: fields{ // 1 instance for each provider, watching all namespace
				proxy: test.NewFakeProxy().
					WithProviderInventory("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system", "").
					WithProviderInventory("bootstrap", clusterctlv1.BootstrapProviderType, "v1.0.0", "bootstrap-system", "").
					WithProviderInventory("infrastructure", clusterctlv1.InfrastructureProviderType, "v1.0.0", "infra-system", ""),
			},
			want: []ManagementGroup{ // One Group
				{
					CoreProvider: fakeProvider("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system", ""),
					Providers: []clusterctlv1.Provider{
						fakeProvider("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system", ""),
						fakeProvider("bootstrap", clusterctlv1.BootstrapProviderType, "v1.0.0", "bootstrap-system", ""),
						fakeProvider("infrastructure", clusterctlv1.InfrastructureProviderType, "v1.0.0", "infra-system", ""),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "1 Core, many infra (1 ManagementGroup)",
			fields: fields{ // 1 instance of core and bootstrap provider, watching all namespace; more instances of infrastructure providers, each watching dedicated ns
				proxy: test.NewFakeProxy().
					WithProviderInventory("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system", "").
					WithProviderInventory("bootstrap", clusterctlv1.BootstrapProviderType, "v1.0.0", "bootstrap-system", "").
					WithProviderInventory("infrastructure", clusterctlv1.InfrastructureProviderType, "v1.0.0", "infra-system1", "ns1").
					WithProviderInventory("infrastructure", clusterctlv1.InfrastructureProviderType, "v1.0.0", "infra-system2", "ns2"),
			},
			want: []ManagementGroup{ // One Group
				{
					CoreProvider: fakeProvider("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system", ""),
					Providers: []clusterctlv1.Provider{
						fakeProvider("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system", ""),
						fakeProvider("bootstrap", clusterctlv1.BootstrapProviderType, "v1.0.0", "bootstrap-system", ""),
						fakeProvider("infrastructure", clusterctlv1.InfrastructureProviderType, "v1.0.0", "infra-system1", "ns1"),
						fakeProvider("infrastructure", clusterctlv1.InfrastructureProviderType, "v1.0.0", "infra-system2", "ns2"),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "two ManagementGroups",
			fields: fields{ // more instances of core with related bootstrap, infrastructure
				proxy: test.NewFakeProxy().
					WithProviderInventory("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system1", "ns1").
					WithProviderInventory("bootstrap", clusterctlv1.BootstrapProviderType, "v1.0.0", "bootstrap-system1", "ns1").
					WithProviderInventory("infrastructure", clusterctlv1.InfrastructureProviderType, "v1.0.0", "infra-system1", "ns1").
					WithProviderInventory("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system2", "ns2").
					WithProviderInventory("bootstrap", clusterctlv1.BootstrapProviderType, "v1.0.0", "bootstrap-system2", "ns2").
					WithProviderInventory("infrastructure", clusterctlv1.InfrastructureProviderType, "v1.0.0", "infra-system2", "ns2"),
			},
			want: []ManagementGroup{ // Two Groups
				{
					CoreProvider: fakeProvider("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system1", "ns1"),
					Providers: []clusterctlv1.Provider{
						fakeProvider("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system1", "ns1"),
						fakeProvider("bootstrap", clusterctlv1.BootstrapProviderType, "v1.0.0", "bootstrap-system1", "ns1"),
						fakeProvider("infrastructure", clusterctlv1.InfrastructureProviderType, "v1.0.0", "infra-system1", "ns1"),
					},
				},
				{
					CoreProvider: fakeProvider("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system2", "ns2"),
					Providers: []clusterctlv1.Provider{
						fakeProvider("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system2", "ns2"),
						fakeProvider("bootstrap", clusterctlv1.BootstrapProviderType, "v1.0.0", "bootstrap-system2", "ns2"),
						fakeProvider("infrastructure", clusterctlv1.InfrastructureProviderType, "v1.0.0", "infra-system2", "ns2"),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "fails with overlapping core providers",
			fields: fields{ //two core providers watching for the same namespaces
				proxy: test.NewFakeProxy().
					WithProviderInventory("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system1", "").
					WithProviderInventory("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system2", ""),
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "fails with overlapping core providers",
			fields: fields{ //a provider watching for objects controlled by more than one core provider
				proxy: test.NewFakeProxy().
					WithProviderInventory("infrastructure", clusterctlv1.InfrastructureProviderType, "v1.0.0", "infra-system", "").
					WithProviderInventory("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system1", "ns1").
					WithProviderInventory("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system2", "ns2"),
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "fails with orphan providers",
			fields: fields{ //a provider watching for objects not controlled any core provider
				proxy: test.NewFakeProxy().
					WithProviderInventory("infrastructure", clusterctlv1.InfrastructureProviderType, "v1.0.0", "infra-system", "ns1").
					WithProviderInventory("core", clusterctlv1.CoreProviderType, "v1.0.0", "core-system1", "ns2"),
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &inventoryClient{
				proxy: tt.fields.proxy,
			}
			got, err := p.GetManagementGroups()
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			if !reflect.DeepEqual(got, tt.want) {
				spew.Dump(got, tt.want)
				t.Errorf("got = %v, want %v", got, tt.want)
			}
		})
	}
}

func fakeProvider(name string, providerType clusterctlv1.ProviderType, version, targetNamespace, watchingNamespace string) clusterctlv1.Provider {
	return clusterctlv1.Provider{
		TypeMeta: metav1.TypeMeta{
			APIVersion: clusterctlv1.GroupVersion.String(),
			Kind:       "Provider",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: targetNamespace,
			Name:      name,
			Labels: map[string]string{
				clusterctlv1.ClusterctlLabelName:     "",
				clusterv1.ProviderLabelName:          name,
				clusterctlv1.ClusterctlCoreLabelName: "inventory",
			},
		},
		Type:             string(providerType),
		Version:          version,
		WatchedNamespace: watchingNamespace,
	}
}
