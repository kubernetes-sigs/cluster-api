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

package v1beta1_test

import (
	"encoding/json"
	"reflect"
	"testing"

	kubeadmv1beta1 "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/v1beta1"
	kubeadm "sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm/kubeadm/v1beta1"
)

func TestInitConfiguration(t *testing.T) {
	testcases := []struct {
		name     string
		expected *kubeadmv1beta1.InitConfiguration
		options  []kubeadm.InitConfigurationOption
	}{
		{
			name: "simple",
			expected: &kubeadmv1beta1.InitConfiguration{
				NodeRegistration: kubeadmv1beta1.NodeRegistrationOptions{},
			},
			options: []kubeadm.InitConfigurationOption{
				kubeadm.WithNodeRegistrationOptions(
					kubeadm.SetNodeRegistrationOptions(&kubeadmv1beta1.NodeRegistrationOptions{}),
				),
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			actual := &kubeadmv1beta1.InitConfiguration{}
			kubeadm.SetInitConfigurationOptions(actual, tc.options...)
			if !reflect.DeepEqual(actual, tc.expected) {
				e, _ := json.Marshal(tc.expected)
				a, _ := json.Marshal(actual)
				t.Fatalf("expected vs actual:\n%+v\n%+v", string(e), string(a))
			}
		})
	}
}

func TestClusterConfiguration(t *testing.T) {
	testcases := []struct {
		name     string
		expected *kubeadmv1beta1.ClusterConfiguration
		options  []kubeadm.ClusterConfigurationOption
	}{
		{
			name: "simple",
			expected: &kubeadmv1beta1.ClusterConfiguration{
				ClusterName:       "test name",
				KubernetesVersion: "test version",
				APIServer: kubeadmv1beta1.APIServer{
					ControlPlaneComponent: kubeadmv1beta1.ControlPlaneComponent{
						ExtraArgs: map[string]string{
							"test key": "test value",
						},
					},
					CertSANs: []string{
						"test san one",
						"test san two",
					},
				},
				ControllerManager: kubeadmv1beta1.ControlPlaneComponent{
					ExtraArgs: map[string]string{"test cm key": "test cm value"},
				},
				Networking: kubeadmv1beta1.Networking{
					DNSDomain:     "test dns domain",
					PodSubnet:     "test pod subnet",
					ServiceSubnet: "test service subnet",
				},
				ControlPlaneEndpoint: "test control plane endpoint",
			},
			options: []kubeadm.ClusterConfigurationOption{
				kubeadm.WithClusterName("test name"),
				kubeadm.WithKubernetesVersion("test version"),
				kubeadm.WithAPIServerExtraArgs(map[string]string{"test key": "test value"}),
				kubeadm.WithControllerManagerExtraArgs(map[string]string{"test cm key": "test cm value"}),
				kubeadm.WithClusterNetworkFromClusterNetworkingConfig("test dns domain", "test pod subnet", "test service subnet"),
				kubeadm.WithControlPlaneEndpoint("test control plane endpoint"),
				kubeadm.WithAPIServerCertificateSANs("test san one", "test san two"),
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			actual := &kubeadmv1beta1.ClusterConfiguration{}
			kubeadm.SetClusterConfigurationOptions(actual, tc.options...)
			if !reflect.DeepEqual(actual, tc.expected) {
				e, _ := json.Marshal(tc.expected)
				a, _ := json.Marshal(actual)
				t.Fatalf("expected vs actual:\n%+v\n%+v", string(e), string(a))
			}
		})
	}

}

func TestJoinConfiguration(t *testing.T) {
	testcases := []struct {
		name     string
		expected *kubeadmv1beta1.JoinConfiguration
		options  []kubeadm.JoinConfigurationOption
	}{
		{
			name: "simple",
			expected: &kubeadmv1beta1.JoinConfiguration{
				Discovery: kubeadmv1beta1.Discovery{
					BootstrapToken: &kubeadmv1beta1.BootstrapTokenDiscovery{},
				},
				ControlPlane: &kubeadmv1beta1.JoinControlPlane{
					LocalAPIEndpoint: kubeadmv1beta1.APIEndpoint{
						AdvertiseAddress: "test endpoint",
						BindPort:         int32(1234),
					},
				},
				NodeRegistration: kubeadmv1beta1.NodeRegistrationOptions{
					Name: "test name",
				},
			},
			options: []kubeadm.JoinConfigurationOption{
				kubeadm.WithBootstrapTokenDiscovery(
					kubeadm.NewBootstrapTokenDiscovery(),
				),
				kubeadm.WithLocalAPIEndpointAndPort("test endpoint", 1234),
				kubeadm.WithJoinNodeRegistrationOptions(
					kubeadmv1beta1.NodeRegistrationOptions{
						Name: "test name",
					},
				),
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			actual := &kubeadmv1beta1.JoinConfiguration{}
			kubeadm.SetJoinConfigurationOptions(actual, tc.options...)
			if !reflect.DeepEqual(actual, tc.expected) {
				e, _ := json.Marshal(tc.expected)
				a, _ := json.Marshal(actual)
				t.Fatalf("expected vs actual:\n%+v\n%+v", string(e), string(a))
			}
		})
	}
}
