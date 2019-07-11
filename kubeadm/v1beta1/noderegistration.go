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
	"strings"

	corev1 "k8s.io/api/core/v1"
	kubeadmv1beta1 "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/v1beta1"
)

// NodeRegistrationOption is a function that sets a value on a Kubeadm NodeRegistrationOptions.
type NodeRegistrationOption func(*kubeadmv1beta1.NodeRegistrationOptions)

// SetNodeRegistrationOptions will take a kubeadmNodeRegistrationOptions and apply customizations.
func SetNodeRegistrationOptions(base *kubeadmv1beta1.NodeRegistrationOptions, opts ...NodeRegistrationOption) kubeadmv1beta1.NodeRegistrationOptions {
	for _, opt := range opts {
		opt(base)
	}
	return *base
}

// WithTaints will set the taints on the NodeRegistration.
func WithTaints(taints []corev1.Taint) NodeRegistrationOption {
	return func(n *kubeadmv1beta1.NodeRegistrationOptions) {
		n.Taints = taints
	}
}

// WithDefaultCRISocket sets the location of the container runtime socket
func WithCRISocket(socket string) NodeRegistrationOption {
	return func(n *kubeadmv1beta1.NodeRegistrationOptions) {
		n.CRISocket = socket
	}
}

// WithNodeRegistrationName sets the name of the provided node registration options struct.
func WithNodeRegistrationName(name string) NodeRegistrationOption {
	return func(n *kubeadmv1beta1.NodeRegistrationOptions) {
		n.Name = name
	}
}

// WithKubeletExtraArgs sets the extra args on the Kubelet.
// `node-labels` will be appendended to existing node-labels.
// All other other args will overwrite existing args.
func WithKubeletExtraArgs(args map[string]string) NodeRegistrationOption {
	return func(n *kubeadmv1beta1.NodeRegistrationOptions) {
		if n.KubeletExtraArgs == nil {
			n.KubeletExtraArgs = map[string]string{}
		}
		for key, value := range args {
			if key == "node-labels" {
				if val, ok := n.KubeletExtraArgs[key]; ok {
					nodeLabels := strings.Split(val, ",")
					nodeLabels = append(nodeLabels, value)
					value = strings.Join(nodeLabels, ",")
				}
			}
			n.KubeletExtraArgs[key] = value
		}
	}
}
