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

package config

import (
	"fmt"
	"sort"
	"testing"

	. "github.com/onsi/gomega"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
)

func Test_providers_List(t *testing.T) {
	reader := test.NewFakeReader()

	p := &providersClient{
		reader: reader,
	}

	defaults := p.defaults()
	sort.Slice(defaults, func(i, j int) bool {
		return defaults[i].Less(defaults[j])
	})

	defaultsAndZZZ := append(defaults, NewProvider("zzz", "https://zzz/infrastructure-components.yaml", "InfrastructureProvider"))

	defaultsWithOverride := append([]Provider{}, defaults...)
	defaultsWithOverride[0] = NewProvider(defaults[0].Name(), "https://zzz/infrastructure-components.yaml", defaults[0].Type())

	type fields struct {
		configGetter Reader
	}
	tests := []struct {
		name    string
		fields  fields
		want    []Provider
		wantErr bool
	}{
		{
			name: "Returns default provider configurations",
			fields: fields{
				configGetter: test.NewFakeReader(),
			},
			want:    defaults,
			wantErr: false,
		},
		{
			name: "Returns user defined provider configurations",
			fields: fields{
				configGetter: test.NewFakeReader().
					WithVar(
						ProvidersConfigKey,
						"- name: \"zzz\"\n"+
							"  url: \"https://zzz/infrastructure-components.yaml\"\n"+
							"  type: \"InfrastructureProvider\"\n",
					),
			},
			want:    defaultsAndZZZ,
			wantErr: false,
		},
		{
			name: "User defined provider configurations override defaults",
			fields: fields{
				configGetter: test.NewFakeReader().
					WithVar(
						ProvidersConfigKey,
						fmt.Sprintf("- name: \"%s\"\n", defaults[0].Name())+
							"  url: \"https://zzz/infrastructure-components.yaml\"\n"+
							fmt.Sprintf("  type: \"%s\"\n", defaults[0].Type()),
					),
			},
			want:    defaultsWithOverride,
			wantErr: false,
		},
		{
			name: "Fails for invalid user defined provider configurations",
			fields: fields{
				configGetter: test.NewFakeReader().
					WithVar(
						ProvidersConfigKey,
						"- foo\n",
					),
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "Fails for invalid user defined provider configurations",
			fields: fields{
				configGetter: test.NewFakeReader().
					WithVar(
						ProvidersConfigKey,
						"- name: \"\"\n"+ // name must not be empty
							"  url: \"\"\n"+
							"  type: \"\"\n",
					),
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			p := &providersClient{
				reader: tt.fields.configGetter,
			}
			got, err := p.List()
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func Test_validateProvider(t *testing.T) {
	type args struct {
		r Provider
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "Pass",
			args: args{
				r: NewProvider("foo", "https://something.com", clusterctlv1.InfrastructureProviderType),
			},
			wantErr: false,
		},
		{
			name: "Pass (core provider)",
			args: args{
				r: NewProvider(ClusterAPIProviderName, "https://something.com", clusterctlv1.CoreProviderType),
			},
			wantErr: false,
		},
		{
			name: "Fails if cluster-api name used with wrong type",
			args: args{
				r: NewProvider(ClusterAPIProviderName, "https://something.com", clusterctlv1.BootstrapProviderType),
			},
			wantErr: true,
		},
		{
			name: "Fails if CoreProviderType used with wrong name",
			args: args{
				r: NewProvider("sss", "https://something.com", clusterctlv1.CoreProviderType),
			},
			wantErr: true,
		},
		{
			name: "Fails if name is empty",
			args: args{
				r: NewProvider("", "", ""),
			},
			wantErr: true,
		},
		{
			name: "Fails if name is not valid",
			args: args{
				r: NewProvider("FOo", "https://something.com", ""),
			},
			wantErr: true,
		},
		{
			name: "Fails if url is empty",
			args: args{
				r: NewProvider("foo", "", ""),
			},
			wantErr: true,
		},
		{
			name: "Fails if url is not valid",
			args: args{
				r: NewProvider("foo", "%gh&%ij", "bar"),
			},
			wantErr: true,
		},
		{
			name: "Fails if type is empty",
			args: args{
				r: NewProvider("foo", "https://something.com", ""),
			},
			wantErr: true,
		},
		{
			name: "Fails if type is not valid",
			args: args{
				r: NewProvider("foo", "https://something.com", "bar"),
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			err := validateProvider(tt.args.r)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
		})
	}
}

// check if Defaults returns valid provider repository configurations
// this is a safeguard for catching changes leading to formally invalid default configurations.
func Test_providers_Defaults(t *testing.T) {
	g := NewWithT(t)

	reader := test.NewFakeReader()

	p := &providersClient{
		reader: reader,
	}

	defaults := p.defaults()

	for _, d := range defaults {
		err := validateProvider(d)
		g.Expect(err).NotTo(HaveOccurred())
	}
}

func Test_providers_Get(t *testing.T) {
	reader := test.NewFakeReader()

	p := &providersClient{
		reader: reader,
	}

	defaults := p.defaults()

	type args struct {
		name         string
		providerType clusterctlv1.ProviderType
	}
	tests := []struct {
		name    string
		args    args
		want    Provider
		wantErr bool
	}{
		{
			name: "pass",
			args: args{
				name:         p.defaults()[0].Name(),
				providerType: p.defaults()[0].Type(),
			},
			want:    defaults[0],
			wantErr: false,
		},
		{
			name: "kubeadm bootstrap",
			args: args{
				name:         KubeadmBootstrapProviderName,
				providerType: clusterctlv1.BootstrapProviderType,
			},
			want:    NewProvider(KubeadmBootstrapProviderName, "https://github.com/kubernetes-sigs/cluster-api/releases/latest/bootstrap-components.yaml", clusterctlv1.BootstrapProviderType),
			wantErr: false,
		},
		{
			name: "kubeadm control-plane",
			args: args{
				name:         KubeadmControlPlaneProviderName,
				providerType: clusterctlv1.ControlPlaneProviderType,
			},
			want:    NewProvider(KubeadmControlPlaneProviderName, "https://github.com/kubernetes-sigs/cluster-api/releases/latest/control-plane-components.yaml", clusterctlv1.ControlPlaneProviderType),
			wantErr: false,
		},
		{
			name: "fails if the provider does not exists (wrong name)",
			args: args{
				name:         "foo",
				providerType: clusterctlv1.CoreProviderType,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "fails if the provider does not exists (wrong type)",
			args: args{
				name:         ClusterAPIProviderName,
				providerType: clusterctlv1.InfrastructureProviderType,
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			p := &providersClient{
				reader: reader,
			}
			got, err := p.Get(tt.args.name, tt.args.providerType)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(got).To(Equal(tt.want))
		})
	}
}
