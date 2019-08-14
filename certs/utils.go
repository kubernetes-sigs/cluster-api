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

const (
	clusterCAKey             = "cluster-ca-key"
	clusterCACertificate     = "cluster-ca-cert"
	etcdCAKey                = "etcd-ca-key"
	etcdCACertificate        = "etcd-ca-cert"
	frontProxyCAKey          = "front-proxy-ca-key"
	frontProxyCACertificate  = "front-proxy-ca-cert"
	serviceAccountPublicKey  = "service-account-public-key"
	serviceAccountPrivateKey = "service-account-private-key"
)

// NewCertificatesFromMap creates Certificates from a map
func NewCertificatesFromMap(m map[string][]byte) *Certificates {
	certs := &Certificates{
		ClusterCA:      &KeyPair{},
		EtcdCA:         &KeyPair{},
		FrontProxyCA:   &KeyPair{},
		ServiceAccount: &KeyPair{},
	}

	if val, ok := m[clusterCAKey]; ok {
		certs.ClusterCA.Key = val
	}
	if val, ok := m[clusterCACertificate]; ok {
		certs.ClusterCA.Cert = val
	}
	if val, ok := m[etcdCAKey]; ok {
		certs.EtcdCA.Key = val
	}
	if val, ok := m[etcdCACertificate]; ok {
		certs.EtcdCA.Cert = val
	}
	if val, ok := m[frontProxyCAKey]; ok {
		certs.FrontProxyCA.Key = val
	}
	if val, ok := m[frontProxyCACertificate]; ok {
		certs.FrontProxyCA.Cert = val
	}
	if val, ok := m[serviceAccountPrivateKey]; ok {
		certs.ServiceAccount.Key = val
	}
	if val, ok := m[serviceAccountPublicKey]; ok {
		certs.ServiceAccount.Cert = val
	}

	return certs
}

// ToMap converts Certificates into a map
func (c *Certificates) ToMap() map[string][]byte {
	return map[string][]byte{
		clusterCAKey:             c.ClusterCA.Key,
		clusterCACertificate:     c.ClusterCA.Cert,
		etcdCAKey:                c.EtcdCA.Key,
		etcdCACertificate:        c.EtcdCA.Cert,
		frontProxyCAKey:          c.FrontProxyCA.Key,
		frontProxyCACertificate:  c.FrontProxyCA.Cert,
		serviceAccountPrivateKey: c.ServiceAccount.Key,
		serviceAccountPublicKey:  c.ServiceAccount.Cert,
	}

}
