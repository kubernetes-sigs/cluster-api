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

package certs

import "sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm/api/v1alpha2"

const (
	rootOwnerValue = "root:root"
)

// CertificatesToFiles writes Certificates to files
func CertificatesToFiles(input Certificates) []v1alpha2.File {
	return []v1alpha2.File{
		{
			Path:        "/etc/kubernetes/pki/ca.crt",
			Owner:       rootOwnerValue,
			Permissions: "0640",
			Content:     string(input.ClusterCA.Cert),
		},
		{
			Path:        "/etc/kubernetes/pki/ca.key",
			Owner:       rootOwnerValue,
			Permissions: "0600",
			Content:     string(input.ClusterCA.Key),
		},
		{
			Path:        "/etc/kubernetes/pki/etcd/ca.crt",
			Owner:       rootOwnerValue,
			Permissions: "0640",
			Content:     string(input.EtcdCA.Cert),
		},
		{
			Path:        "/etc/kubernetes/pki/etcd/ca.key",
			Owner:       rootOwnerValue,
			Permissions: "0600",
			Content:     string(input.EtcdCA.Key),
		},
		{
			Path:        "/etc/kubernetes/pki/front-proxy-ca.crt",
			Owner:       rootOwnerValue,
			Permissions: "0640",
			Content:     string(input.FrontProxyCA.Cert),
		},
		{
			Path:        "/etc/kubernetes/pki/front-proxy-ca.key",
			Owner:       rootOwnerValue,
			Permissions: "0600",
			Content:     string(input.FrontProxyCA.Key),
		},
		{
			Path:        "/etc/kubernetes/pki/sa.pub",
			Owner:       rootOwnerValue,
			Permissions: "0640",
			Content:     string(input.ServiceAccount.Cert),
		},
		{
			Path:        "/etc/kubernetes/pki/sa.key",
			Owner:       rootOwnerValue,
			Permissions: "0600",
			Content:     string(input.ServiceAccount.Key),
		},
	}
}
