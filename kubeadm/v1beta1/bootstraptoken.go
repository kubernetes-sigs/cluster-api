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

package v1beta1

import (
	kubeadmv1beta1 "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/v1beta1"
)

// BootstrapTokenDiscoveryOption will modify a kubeadm BootstrapTokenDiscovery.
type BootstrapTokenDiscoveryOption func(*kubeadmv1beta1.BootstrapTokenDiscovery)

// NewBootstrapTokenDiscovery will create a new kubeadm bootstrap token discovery and run options against it.
func NewBootstrapTokenDiscovery(opts ...BootstrapTokenDiscoveryOption) *kubeadmv1beta1.BootstrapTokenDiscovery {
	discovery := &kubeadmv1beta1.BootstrapTokenDiscovery{}
	for _, opt := range opts {
		opt(discovery)
	}
	return discovery
}

// WithAPIServerEndpoint will set the kube-apiserver's endpoint for the bootstrap token discovery.
func WithAPIServerEndpoint(endpoint string) BootstrapTokenDiscoveryOption {
	return func(b *kubeadmv1beta1.BootstrapTokenDiscovery) {
		b.APIServerEndpoint = endpoint
	}
}

// WithToken will set the bootstrap token.
func WithToken(token string) BootstrapTokenDiscoveryOption {
	return func(b *kubeadmv1beta1.BootstrapTokenDiscovery) {
		b.Token = token
	}
}

// WithCACertificateHash sets the hash of CA for the bootstrap token to use.
func WithCACertificateHash(caCertHash string) BootstrapTokenDiscoveryOption {
	return func(b *kubeadmv1beta1.BootstrapTokenDiscovery) {
		b.CACertHashes = append(b.CACertHashes, caCertHash)
	}
}
