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
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/test"
)

func Test_inventoryClient_EnsureCustomResourceDefinitions(t *testing.T) {
	type fields struct {
		alreadyHasCRD bool
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		{
			name: "Has not CRD",
			fields: fields{
				alreadyHasCRD: false,
			},
			wantErr: false,
		},
		{
			name: "Already has CRD",
			fields: fields{
				alreadyHasCRD: true,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newInventoryClient(test.NewFakeK8SProxy())
			if tt.fields.alreadyHasCRD {
				//forcing creation of metadata before test
				if err := p.EnsureCustomResourceDefinitions(); err != nil {
					t.Errorf("EnsureCustomResourceDefinitions() error = %v", err)
					return
				}
			}

			err := p.EnsureCustomResourceDefinitions()
			if (err != nil) != tt.wantErr {
				t.Errorf("EnsureCustomResourceDefinitions() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

var fooProvider = clusterctlv1.Provider{ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "ns1"}}

func Test_inventoryClient_List(t *testing.T) {
	type fields struct {
		initObjs []runtime.Object
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
				initObjs: []runtime.Object{
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
			p := newInventoryClient(test.NewFakeK8SProxy().WithObjs(tt.fields.initObjs...))
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
