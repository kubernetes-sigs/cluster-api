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

package types

import (
	"fmt"
	"testing"

	"github.com/blang/semver/v4"
	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/upstream"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/upstreamv1beta3"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/upstreamv1beta4"
)

func TestKubeVersionToKubeadmAPIGroupVersion(t *testing.T) {
	type args struct {
		version semver.Version
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
				version: semver.MustParse("1.12.0"),
			},
			want:    schema.GroupVersion{},
			wantErr: true,
		},
		{
			name: "fails with kubernetes version for kubeadm API v1beta1",
			args: args{
				version: semver.MustParse("1.14.99"),
			},
			wantErr: true,
		},
		{
			name: "pass with minimum kubernetes alpha version for kubeadm API v1beta3",
			args: args{
				version: semver.MustParse("1.22.0-alpha.0.734+ba502ee555924a"),
			},
			want:    upstreamv1beta3.GroupVersion,
			wantErr: false,
		},
		{
			name: "pass with minimum kubernetes version for kubeadm API v1beta3",
			args: args{
				version: semver.MustParse("1.22.0"),
			},
			want:    upstreamv1beta3.GroupVersion,
			wantErr: false,
		},
		{
			name: "pass with kubernetes version for kubeadm API v1beta3",
			args: args{
				version: semver.MustParse("1.23.99"),
			},
			want:    upstreamv1beta3.GroupVersion,
			wantErr: false,
		},
		{
			name: "pass with minimum kubernetes version for kubeadm API v1beta4",
			args: args{
				version: semver.MustParse("1.31.0"),
			},
			want:    upstreamv1beta4.GroupVersion,
			wantErr: false,
		},
		{
			name: "pass with kubernetes version for kubeadm API v1beta4",
			args: args{
				version: semver.MustParse("1.32.99"),
			},
			want:    upstreamv1beta4.GroupVersion,
			wantErr: false,
		},
		{
			name: "pass with future kubernetes version",
			args: args{
				version: semver.MustParse("99.99.99"),
			},
			want:    upstreamv1beta4.GroupVersion,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := KubeVersionToKubeadmAPIGroupVersion(tt.args.version)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(BeComparableTo(tt.want))
		})
	}
}

func TestMarshalClusterConfigurationForVersion(t *testing.T) {
	data := &upstream.AdditionalData{
		// KubernetesVersion is going to be set on a test bases.
		ClusterName:                             ptr.To("mycluster"),
		DNSDomain:                               ptr.To("myDNSDomain"),
		ServiceSubnet:                           ptr.To("myServiceSubnet"),
		PodSubnet:                               ptr.To("myPodSubnet"),
		ControlPlaneComponentHealthCheckSeconds: ptr.To[int32](30),
	}

	type args struct {
		capiObj *bootstrapv1.ClusterConfiguration
		version semver.Version
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "Generates a v1beta3 kubeadm configuration",
			args: args{
				capiObj: &bootstrapv1.ClusterConfiguration{
					ControlPlaneEndpoint: "myControlPlaneEndpoint:6443",
				},
				version: semver.MustParse("1.22.0"),
			},
			want: "apiServer:\n" +
				"  timeoutForControlPlane: 30s\n" +
				"apiVersion: kubeadm.k8s.io/v1beta3\n" +
				"clusterName: mycluster\n" +
				"controlPlaneEndpoint: myControlPlaneEndpoint:6443\n" +
				"controllerManager: {}\n" +
				"dns: {}\n" +
				"etcd: {}\n" +
				"kind: ClusterConfiguration\n" +
				"kubernetesVersion: v1.22.0\n" +
				"networking:\n" +
				"  dnsDomain: myDNSDomain\n" +
				"  podSubnet: myPodSubnet\n" +
				"  serviceSubnet: myServiceSubnet\n" +
				"scheduler: {}\n",
			wantErr: false,
		},
		{
			name: "Generates a v1beta4 kubeadm configuration",
			args: args{
				capiObj: &bootstrapv1.ClusterConfiguration{
					ControlPlaneEndpoint: "myControlPlaneEndpoint:6443",
				},
				version: semver.MustParse("1.31.0"),
			},
			want: "apiServer: {}\n" +
				"apiVersion: kubeadm.k8s.io/v1beta4\n" +
				"clusterName: mycluster\n" +
				"controlPlaneEndpoint: myControlPlaneEndpoint:6443\n" +
				"controllerManager: {}\n" +
				"dns: {}\n" +
				"etcd: {}\n" +
				"kind: ClusterConfiguration\n" +
				"kubernetesVersion: v1.31.0\n" +
				"networking:\n" +
				"  dnsDomain: myDNSDomain\n" +
				"  podSubnet: myPodSubnet\n" +
				"  serviceSubnet: myServiceSubnet\n" +
				"proxy: {}\n" +
				"scheduler: {}\n",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			data.KubernetesVersion = ptr.To(fmt.Sprintf("v%s", tt.args.version.String()))
			got, err := MarshalClusterConfigurationForVersion(tt.args.capiObj, tt.args.version, data)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(Equal(tt.want), cmp.Diff(tt.want, got))
		})
	}
}

func TestMarshalInitConfigurationForVersion(t *testing.T) {
	type args struct {
		initConfiguration *bootstrapv1.InitConfiguration
		version           semver.Version
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "Generates a v1beta3 kubeadm init configuration",
			args: args{
				initConfiguration: &bootstrapv1.InitConfiguration{
					NodeRegistration: bootstrapv1.NodeRegistrationOptions{
						IgnorePreflightErrors: []string{"some-preflight-check"},
					},
				},
				version: semver.MustParse("1.22.0"),
			},
			want: "apiVersion: kubeadm.k8s.io/v1beta3\n" +
				"kind: InitConfiguration\n" +
				"localAPIEndpoint: {}\n" +
				"nodeRegistration:\n" +
				"  ignorePreflightErrors:\n" +
				"  - some-preflight-check\n" +
				"  taints: null\n",
			wantErr: false,
		},
		{
			name: "Generates a v1beta4 kubeadm init configuration",
			args: args{
				initConfiguration: &bootstrapv1.InitConfiguration{
					NodeRegistration: bootstrapv1.NodeRegistrationOptions{
						IgnorePreflightErrors: []string{"some-preflight-check"},
					},
					Timeouts: bootstrapv1.Timeouts{
						ControlPlaneComponentHealthCheckSeconds: ptr.To[int32](50),
					},
				},
				version: semver.MustParse("1.31.0"),
			},
			want: "apiVersion: kubeadm.k8s.io/v1beta4\n" +
				"kind: InitConfiguration\n" +
				"localAPIEndpoint: {}\n" +
				"nodeRegistration:\n" +
				"  ignorePreflightErrors:\n" +
				"  - some-preflight-check\n" +
				"  taints: null\n" +
				"timeouts:\n" +
				"  controlPlaneComponentHealthCheck: 50s\n",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := MarshalInitConfigurationForVersion(tt.args.initConfiguration, tt.args.version)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(Equal(tt.want), cmp.Diff(tt.want, got))
		})
	}
}

func TestMarshalJoinConfigurationForVersion(t *testing.T) {
	type args struct {
		joinConfiguration *bootstrapv1.JoinConfiguration
		version           semver.Version
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "Generates a v1beta3 kubeadm join configuration",
			args: args{
				joinConfiguration: &bootstrapv1.JoinConfiguration{
					NodeRegistration: bootstrapv1.NodeRegistrationOptions{
						IgnorePreflightErrors: []string{"some-preflight-check"},
					},
				},
				version: semver.MustParse("1.22.0"),
			},
			want: "apiVersion: kubeadm.k8s.io/v1beta3\n" + "" +
				"discovery: {}\n" +
				"kind: JoinConfiguration\n" +
				"nodeRegistration:\n" +
				"  ignorePreflightErrors:\n" +
				"  - some-preflight-check\n" +
				"  taints: null\n",
			wantErr: false,
		},
		{
			name: "Generates a v1beta4 kubeadm join configuration",
			args: args{
				joinConfiguration: &bootstrapv1.JoinConfiguration{
					NodeRegistration: bootstrapv1.NodeRegistrationOptions{
						IgnorePreflightErrors: []string{"some-preflight-check"},
					},
					Timeouts: bootstrapv1.Timeouts{
						ControlPlaneComponentHealthCheckSeconds: ptr.To[int32](50),
					},
				},
				version: semver.MustParse("1.31.0"),
			},
			want: "apiVersion: kubeadm.k8s.io/v1beta4\n" + "" +
				"discovery: {}\n" +
				"kind: JoinConfiguration\n" +
				"nodeRegistration:\n" +
				"  ignorePreflightErrors:\n" +
				"  - some-preflight-check\n" +
				"  taints: null\n" +
				"timeouts:\n" +
				"  controlPlaneComponentHealthCheck: 50s\n",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := MarshalJoinConfigurationForVersion(tt.args.joinConfiguration, tt.args.version)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(Equal(tt.want), cmp.Diff(tt.want, got))
		})
	}
}

func TestUnmarshalClusterConfiguration(t *testing.T) {
	type args struct {
		yaml string
	}
	tests := []struct {
		name             string
		args             args
		want             *bootstrapv1.ClusterConfiguration
		wantUpstreamData *upstream.AdditionalData
		wantErr          bool
	}{
		{
			name: "Parses a v1beta3 kubeadm configuration",
			args: args{
				yaml: "apiServer: {}\n" +
					"apiVersion: kubeadm.k8s.io/v1beta3\n" +
					"clusterName: mycluster\n" +
					"controlPlaneEndpoint: myControlPlaneEndpoint:6443\n" +
					"controllerManager: {}\n" +
					"dns: {}\n" +
					"etcd: {}\n" +
					"kind: ClusterConfiguration\n" +
					"kubernetesVersion: v1.23.1\n" +
					"networking:\n" +
					"  dnsDomain: myDNSDomain\n" +
					"  podSubnet: myPodSubnet\n" +
					"  serviceSubnet: myServiceSubnet\n" +
					"scheduler: {}\n",
			},
			want: &bootstrapv1.ClusterConfiguration{
				ControlPlaneEndpoint: "myControlPlaneEndpoint:6443",
			},
			wantUpstreamData: &upstream.AdditionalData{
				KubernetesVersion: ptr.To("v1.23.1"),
				ClusterName:       ptr.To("mycluster"),
				DNSDomain:         ptr.To("myDNSDomain"),
				ServiceSubnet:     ptr.To("myServiceSubnet"),
				PodSubnet:         ptr.To("myPodSubnet"),
			},
			wantErr: false,
		},
		{
			name: "Parses a v1beta3 kubeadm configuration with data to init configuration",
			args: args{
				yaml: "apiServer: {\n" +
					"  timeoutForControlPlane: 50s\n" +
					"}\n" +
					"apiVersion: kubeadm.k8s.io/v1beta3\n" + "" +
					"clusterName: mycluster\n" +
					"controlPlaneEndpoint: myControlPlaneEndpoint:6443\n" +
					"controllerManager: {}\n" +
					"dns: {}\n" +
					"etcd: {}\n" +
					"kind: ClusterConfiguration\n" +
					"kubernetesVersion: v1.23.1\n" +
					"networking:\n" +
					"  dnsDomain: myDNSDomain\n" +
					"  podSubnet: myPodSubnet\n" +
					"  serviceSubnet: myServiceSubnet\n" +
					"scheduler: {}\n",
			},
			want: &bootstrapv1.ClusterConfiguration{
				ControlPlaneEndpoint: "myControlPlaneEndpoint:6443",
			},
			wantUpstreamData: &upstream.AdditionalData{
				KubernetesVersion:                       ptr.To("v1.23.1"),
				ClusterName:                             ptr.To("mycluster"),
				DNSDomain:                               ptr.To("myDNSDomain"),
				ServiceSubnet:                           ptr.To("myServiceSubnet"),
				PodSubnet:                               ptr.To("myPodSubnet"),
				ControlPlaneComponentHealthCheckSeconds: ptr.To(int32(50)),
			},
			wantErr: false,
		},
		{
			name: "Parses a v1beta4 kubeadm configuration",
			args: args{
				yaml: "apiServer: {}\n" +
					"apiVersion: kubeadm.k8s.io/v1beta4\n" + "" +
					"clusterName: mycluster\n" +
					"controlPlaneEndpoint: myControlPlaneEndpoint:6443\n" +
					"controllerManager: {}\n" +
					"dns: {}\n" +
					"etcd: {}\n" +
					"kind: ClusterConfiguration\n" +
					"kubernetesVersion: v1.31.1\n" +
					"networking:\n" +
					"  dnsDomain: myDNSDomain\n" +
					"  podSubnet: myPodSubnet\n" +
					"  serviceSubnet: myServiceSubnet\n" +
					"proxy: {}\n" +
					"scheduler: {}\n",
			},
			want: &bootstrapv1.ClusterConfiguration{
				ControlPlaneEndpoint: "myControlPlaneEndpoint:6443",
			},
			wantUpstreamData: &upstream.AdditionalData{
				KubernetesVersion: ptr.To("v1.31.1"),
				ClusterName:       ptr.To("mycluster"),
				DNSDomain:         ptr.To("myDNSDomain"),
				ServiceSubnet:     ptr.To("myServiceSubnet"),
				PodSubnet:         ptr.To("myPodSubnet"),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, upstreamData, err := UnmarshalClusterConfiguration(tt.args.yaml)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(BeComparableTo(tt.want), cmp.Diff(tt.want, got))
			g.Expect(upstreamData).To(Equal(tt.wantUpstreamData))
		})
	}
}

func TestUnmarshalInitConfiguration(t *testing.T) {
	type args struct {
		yaml string
	}
	tests := []struct {
		name    string
		args    args
		want    *bootstrapv1.InitConfiguration
		wantErr bool
	}{
		{
			name: "Parses a v1beta3 kubeadm configuration",
			args: args{
				yaml: "apiVersion: kubeadm.k8s.io/v1beta3\n" + "" +
					"kind: InitConfiguration\n" +
					"localAPIEndpoint: {}\n" +
					"nodeRegistration: {}\n",
			},
			want:    &bootstrapv1.InitConfiguration{},
			wantErr: false,
		},
		{
			name: "Parses a v1beta4 kubeadm configuration",
			args: args{
				yaml: "apiVersion: kubeadm.k8s.io/v1beta4\n" + "" +
					"kind: InitConfiguration\n" +
					"localAPIEndpoint: {}\n" +
					"nodeRegistration: {}\n" +
					"timeouts: {\n" +
					"  controlPlaneComponentHealthCheck: 50s\n" +
					"}\n",
			},
			want: &bootstrapv1.InitConfiguration{
				Timeouts: bootstrapv1.Timeouts{
					ControlPlaneComponentHealthCheckSeconds: ptr.To[int32](50),
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := UnmarshalInitConfiguration(tt.args.yaml)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(BeComparableTo(tt.want), cmp.Diff(tt.want, got))
		})
	}
}

func TestUnmarshalJoinConfiguration(t *testing.T) {
	type args struct {
		yaml string
	}
	tests := []struct {
		name    string
		args    args
		want    *bootstrapv1.JoinConfiguration
		wantErr bool
	}{
		{
			name: "Parses a v1beta3 kubeadm configuration",
			args: args{
				yaml: "apiVersion: kubeadm.k8s.io/v1beta3\n" + "" +
					"caCertPath: \"\"\n" +
					"discovery: {}\n" +
					"kind: JoinConfiguration\n",
			},
			want:    &bootstrapv1.JoinConfiguration{},
			wantErr: false,
		},
		{
			name: "Parses a v1beta4 kubeadm configuration",
			args: args{
				yaml: "apiVersion: kubeadm.k8s.io/v1beta4\n" + "" +
					"caCertPath: \"\"\n" +
					"discovery: {}\n" +
					"kind: JoinConfiguration\n" +
					"timeouts: {\n" +
					"  controlPlaneComponentHealthCheck: 50s\n" +
					"}\n",
			},
			want: &bootstrapv1.JoinConfiguration{
				Timeouts: bootstrapv1.Timeouts{
					ControlPlaneComponentHealthCheckSeconds: ptr.To[int32](50),
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := UnmarshalJoinConfiguration(tt.args.yaml)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(BeComparableTo(tt.want), cmp.Diff(tt.want, got))
		})
	}
}
