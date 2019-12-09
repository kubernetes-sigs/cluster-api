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
	"reflect"
	"sort"
	"testing"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/test"
)

func Test_providers_List(t *testing.T) {
	reader := test.NewFakeReader()

	p := &providersClient{
		reader: reader,
	}

	defaults := p.defaults()
	sort.Slice(defaults, func(i, j int) bool {
		return defaults[i].Name() < defaults[j].Name()
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
							"  type: \"InfrastructureProvider\"\n",
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
						"- name: \"\"\n"+ //name must not be empty
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
			p := &providersClient{
				reader: tt.fields.configGetter,
			}
			got, err := p.List()
			if (err != nil) != tt.wantErr {
				t.Errorf("List() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("List() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_validateProviderRepository(t *testing.T) {
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
				r: NewProvider("foo", "https://something.com", "CoreProvider"),
			},
			wantErr: false,
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
			if err := validateProvider(tt.args.r); (err != nil) != tt.wantErr {
				t.Errorf("validateProvider() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// check if Defaults returns valid provider repository configurations
// this is a safeguard for catching changes leading to formally invalid default configurations
func Test_providers_Defaults(t *testing.T) {
	reader := test.NewFakeReader()

	p := &providersClient{
		reader: reader,
	}

	defaults := p.defaults()

	for _, d := range defaults {
		err := validateProvider(d)
		if err != nil {
			t.Errorf("defaults() error = %v, want %v", err, nil)
		}
	}
}

func Test_providers_Get(t *testing.T) {
	reader := test.NewFakeReader()

	p := &providersClient{
		reader: reader,
	}

	defaults := p.defaults()

	type args struct {
		name string
	}
	tests := []struct {
		name    string
		args    args
		want    *Provider
		wantErr bool
	}{
		{
			name: "pass",
			args: args{
				name: p.defaults()[0].Name(),
			},
			want:    &defaults[0],
			wantErr: false,
		},
		{
			name: "fails if the provider does not exists",
			args: args{
				name: "foo",
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &providersClient{
				reader: reader,
			}
			got, err := p.Get(tt.args.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("Get() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Get() got = %v, want %v", got, tt.want)
			}
		})
	}
}
