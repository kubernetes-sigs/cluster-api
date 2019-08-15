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

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math"
	"math/big"
	"time"

	"github.com/pkg/errors"
	"k8s.io/client-go/tools/clientcmd/api"
)

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

// NewKubeconfig creates a new Kubeconfig where endpoint is the ELB endpoint.
func NewKubeconfig(clusterName, endpoint string, caCert *x509.Certificate, caKey *rsa.PrivateKey) (*api.Config, error) {
	cfg := &Config{
		CommonName:   "kubernetes-admin",
		Organization: []string{"system:masters"},
		Usages:       []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	clientKey, err := NewPrivateKey()
	if err != nil {
		return nil, errors.Wrap(err, "unable to create private key")
	}

	clientCert, err := cfg.NewSignedCert(clientKey, caCert, caKey)
	if err != nil {
		return nil, errors.Wrap(err, "unable to sign certificate")
	}

	userName := "kubernetes-admin"
	contextName := fmt.Sprintf("%s@%s", userName, clusterName)

	return &api.Config{
		Clusters: map[string]*api.Cluster{
			clusterName: {
				Server:                   endpoint,
				CertificateAuthorityData: EncodeCertPEM(caCert),
			},
		},
		Contexts: map[string]*api.Context{
			contextName: {
				Cluster:  clusterName,
				AuthInfo: userName,
			},
		},
		AuthInfos: map[string]*api.AuthInfo{
			userName: {
				ClientKeyData:         EncodePrivateKeyPEM(clientKey),
				ClientCertificateData: EncodeCertPEM(clientCert),
			},
		},
		CurrentContext: contextName,
	}, nil
}

// NewSignedCert creates a signed certificate using the given CA certificate and key
func (cfg *Config) NewSignedCert(key *rsa.PrivateKey, caCert *x509.Certificate, caKey *rsa.PrivateKey) (*x509.Certificate, error) {
	serial, err := rand.Int(rand.Reader, new(big.Int).SetInt64(math.MaxInt64))
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate random integer for signed cerficate")
	}

	if len(cfg.CommonName) == 0 {
		return nil, errors.New("must specify a CommonName")
	}

	if len(cfg.Usages) == 0 {
		return nil, errors.New("must specify at least one ExtKeyUsage")
	}

	tmpl := x509.Certificate{
		Subject: pkix.Name{
			CommonName:   cfg.CommonName,
			Organization: cfg.Organization,
		},
		DNSNames:     cfg.AltNames.DNSNames,
		IPAddresses:  cfg.AltNames.IPs,
		SerialNumber: serial,
		NotBefore:    caCert.NotBefore,
		NotAfter:     time.Now().Add(duration365d).UTC(),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  cfg.Usages,
	}

	b, err := x509.CreateCertificate(rand.Reader, &tmpl, caCert, key.Public(), caKey)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create signed certificate: %+v", tmpl)
	}

	return x509.ParseCertificate(b)
}
