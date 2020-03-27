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

package v1alpha3

import (
	"testing"

	. "github.com/onsi/gomega"
)

func Test_Provider_ManifestLabel(t *testing.T) {
	type fields struct {
		provider     string
		providerType ProviderType
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name: "core provider remains the same",
			fields: fields{
				provider:     "cluster-api",
				providerType: CoreProviderType,
			},
			want: "cluster-api",
		},
		{
			name: "kubeadm bootstrap",
			fields: fields{
				provider:     "kubeadm",
				providerType: BootstrapProviderType,
			},
			want: "bootstrap-kubeadm",
		},
		{
			name: "other bootstrap providers gets prefix",
			fields: fields{
				provider:     "xx",
				providerType: BootstrapProviderType,
			},
			want: "bootstrap-xx",
		},
		{
			name: "kubeadm control-plane",
			fields: fields{
				provider:     "kubeadm",
				providerType: ControlPlaneProviderType,
			},
			want: "control-plane-kubeadm",
		},
		{
			name: "other control-plane providers gets prefix",
			fields: fields{
				provider:     "xx",
				providerType: ControlPlaneProviderType,
			},
			want: "control-plane-xx",
		},
		{
			name: "infrastructure providers gets prefix",
			fields: fields{
				provider:     "xx",
				providerType: InfrastructureProviderType,
			},
			want: "infrastructure-xx",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			p := &Provider{
				ProviderName: tt.fields.provider,
				Type:         string(tt.fields.providerType),
			}
			g.Expect(p.ManifestLabel()).To(Equal(tt.want))
		})
	}
}
