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

package kubeconfig

import bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"

// KubeConfigOption helps to modify KubeConfigOptions.
type KubeConfigOption interface { //nolint:revive
	// ApplyKubeConfigOption applies this options to the given KubeConfigOptions options.
	ApplyKubeConfigOption(*KubeConfigOptions)
}

// KubeConfigOptions allows to set options for generating a kubeconfig.
type KubeConfigOptions struct { //nolint:revive
	keyEncryptionAlgorithm bootstrapv1.EncryptionAlgorithmType
}

// ApplyOptions applies the given list options on these options,
// and then returns itself (for convenient chaining).
func (o *KubeConfigOptions) ApplyOptions(opts []KubeConfigOption) {
	for _, opt := range opts {
		opt.ApplyKubeConfigOption(o)
	}
}

// KeyEncryptionAlgorithm allows to specify the key encryption algorithm type.
type KeyEncryptionAlgorithm bootstrapv1.EncryptionAlgorithmType

// ApplyKubeConfigOption applies this configuration to the given kube configuration options.
func (t KeyEncryptionAlgorithm) ApplyKubeConfigOption(opts *KubeConfigOptions) {
	opts.keyEncryptionAlgorithm = bootstrapv1.EncryptionAlgorithmType(t)
}
