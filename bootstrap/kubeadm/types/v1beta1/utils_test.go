/*
Copyright 2021 The Kubernetes Authors.

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

package v1beta1

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/v1beta2"
)

func Test_KubeVersionToKubeadmAPIGroupVersion(t *testing.T) {
	type args struct {
		k8sVersion string
	}
	tests := []struct {
		name    string
		args    args
		want    schema.GroupVersion
		wantErr bool
	}{
		{
			name: "fails when kubernetes version is too old",
			args: args{
				k8sVersion: "v1.12.0",
			},
			want:    schema.GroupVersion{},
			wantErr: true,
		},
		{
			name: "pass with minimum kubernetes version for kubeadm API v1beta1",
			args: args{
				k8sVersion: "v1.13.0",
			},
			want:    GroupVersion,
			wantErr: false,
		},
		{
			name: "pass with kubernetes version for kubeadm API v1beta1",
			args: args{
				k8sVersion: "v1.14.99",
			},
			want:    GroupVersion,
			wantErr: false,
		},
		{
			name: "pass with minimum kubernetes version for kubeadm API v1beta2",
			args: args{
				k8sVersion: "v1.15.0",
			},
			want:    v1beta2.GroupVersion,
			wantErr: false,
		},
		{
			name: "pass with kubernetes version for kubeadm API v1beta2",
			args: args{
				k8sVersion: "v1.20.99",
			},
			want:    v1beta2.GroupVersion,
			wantErr: false,
		},
		{
			name: "pass with future kubernetes version",
			args: args{
				k8sVersion: "v99.99.99",
			},
			want:    v1beta2.GroupVersion,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := KubeVersionToKubeadmAPIGroupVersion(tt.args.k8sVersion)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func TestConfigurationToYAMLForVersion(t *testing.T) {
	type args struct {
		obj        *ClusterConfiguration
		k8sVersion string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "Generates a v1beta1 kubeadm configuration with Kubernetes versions < 1.15",
			args: args{
				obj:        &ClusterConfiguration{},
				k8sVersion: "v1.14.9",
			},
			want: "apiServer: {}\n" +
				"apiVersion: kubeadm.k8s.io/v1beta1\n" + "" +
				"controllerManager: {}\n" +
				"dns: {}\n" +
				"etcd: {}\n" +
				"kind: ClusterConfiguration\n" +
				"networking: {}\n" +
				"scheduler: {}\n",
			wantErr: false,
		},
		{
			name: "Generates a v1beta2 kubeadm configuration with Kubernetes versions >= 1.15",
			args: args{
				obj:        &ClusterConfiguration{},
				k8sVersion: "v1.15.0",
			},
			want: "apiServer: {}\n" +
				"apiVersion: kubeadm.k8s.io/v1beta2\n" + "" +
				"controllerManager: {}\n" +
				"dns: {}\n" +
				"etcd: {}\n" +
				"kind: ClusterConfiguration\n" +
				"networking: {}\n" +
				"scheduler: {}\n",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := ConfigurationToYAMLForVersion(tt.args.obj, tt.args.k8sVersion)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(Equal(tt.want), cmp.Diff(tt.want, got))
		})
	}
}
