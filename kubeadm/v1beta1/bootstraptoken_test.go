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

	kubeadmv1beta1 "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/v1beta1"
	kubeadm "sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm/kubeadm/v1beta1"
)

func TestNewBootstrapToken(t *testing.T) {
	testcases := []struct {
		name     string
		expected *kubeadmv1beta1.BootstrapTokenDiscovery
		actual   *kubeadmv1beta1.BootstrapTokenDiscovery
	}{
		{
			name: "simple test",
			expected: &kubeadmv1beta1.BootstrapTokenDiscovery{
				APIServerEndpoint: "test apiserver endpoint",
				Token:             "test token",
				CACertHashes:      []string{"test ca cert hash"},
			},
			actual: kubeadm.NewBootstrapTokenDiscovery(
				kubeadm.WithAPIServerEndpoint("test apiserver endpoint"),
				kubeadm.WithToken("test token"),
				kubeadm.WithCACertificateHash("test ca cert hash"),
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
