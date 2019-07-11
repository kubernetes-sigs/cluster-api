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
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	kubeadmv1beta1 "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/v1beta1"
	kubeadm "sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm/kubeadm/v1beta1"
)

func TestNewNodeRegistration(t *testing.T) {
	testcases := []struct {
		name     string
		expected kubeadmv1beta1.NodeRegistrationOptions
		actual   kubeadmv1beta1.NodeRegistrationOptions
	}{
		{
			name: "simple test",
			expected: kubeadmv1beta1.NodeRegistrationOptions{
				Name:      "test name",
				CRISocket: "/test/path/to/socket.sock",
				KubeletExtraArgs: map[string]string{
					"test key": "test value",
				},
				Taints: []corev1.Taint{
					{
						Key:    "test",
						Value:  "test value",
						Effect: "test effect",
					},
				},
			},
			actual: kubeadm.SetNodeRegistrationOptions(
				&kubeadmv1beta1.NodeRegistrationOptions{},
				kubeadm.WithNodeRegistrationName("test name"),
				kubeadm.WithCRISocket("/test/path/to/socket.sock"),
				kubeadm.WithTaints([]corev1.Taint{
					{
						Key:    "test",
						Value:  "test value",
						Effect: "test effect",
					},
				}),
				kubeadm.WithKubeletExtraArgs(map[string]string{"test key": "test value"}),
			),
		},
		{
			name: "test node-label appending",
			expected: kubeadmv1beta1.NodeRegistrationOptions{
				KubeletExtraArgs: map[string]string{
					"node-labels": "test value one,test value two",
				},
			},
			actual: kubeadm.SetNodeRegistrationOptions(
				&kubeadmv1beta1.NodeRegistrationOptions{},
				kubeadm.WithKubeletExtraArgs(map[string]string{"node-labels": "test value one"}),
				kubeadm.WithKubeletExtraArgs(map[string]string{"node-labels": "test value two"}),
			),
		},
		{
			name: "test starting with non-empty base",
			expected: kubeadmv1beta1.NodeRegistrationOptions{
				CRISocket: "/test/path/to/socket.sock",
				KubeletExtraArgs: map[string]string{
					"cni-bin-dir":  "/opt/cni/bin",
					"cni-conf-dir": "/etc/cni/net.d",
				},
			},
			actual: kubeadm.SetNodeRegistrationOptions(
				&kubeadmv1beta1.NodeRegistrationOptions{
					KubeletExtraArgs: map[string]string{
						"cni-bin-dir":  "/opt/cni/bin",
						"cni-conf-dir": "/etc/cni/net.d",
					},
				},
				kubeadm.WithCRISocket("/test/path/to/socket.sock"),
			),
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			if !reflect.DeepEqual(tc.expected, tc.actual) {
				t.Fatalf("Expected and actual: \n%v\n%v", tc.expected, tc.actual)
			}
		})
	}
}
