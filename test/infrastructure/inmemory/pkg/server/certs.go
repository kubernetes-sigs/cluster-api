/*
Copyright 2023 The Kubernetes Authors.

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

package server

import (
	"crypto/rsa"
	"crypto/x509"
	"net"

	"github.com/pkg/errors"

	"sigs.k8s.io/cluster-api/util/certs"
)

var key *rsa.PrivateKey

func init() {
	// Create a private key only once, since this is a slow operation and it is ok
	// to reuse it for all the certificates in a test provider.
	var err error
	key, err = certs.NewPrivateKey()
	if err != nil {
		panic(errors.Wrap(err, "unable to create private key").Error())
	}
}

func newCertAndKey(caCert *x509.Certificate, caKey *rsa.PrivateKey, config *certs.Config) (*x509.Certificate, *rsa.PrivateKey, error) {
	cert, err := config.NewSignedCert(key, caCert, caKey)
	if err != nil {
		return nil, nil, errors.Wrap(err, "unable to create certificate")
	}

	return cert, key, nil
}

// apiServerCertificateConfig returns the config for an API server serving certificate.
func apiServerCertificateConfig(controlPlaneIP string) *certs.Config {
	altNames := &certs.AltNames{
		DNSNames: []string{
			// NOTE: DNS names for the kubernetes service are not required (the API
			// server will never be accessed via the service); same for the podName
			"localhost",
		},
		IPs: []net.IP{
			// NOTE: PodIP is not required (it is the same as the control plane IP)
			net.IPv4(127, 0, 0, 1),
			net.IPv6loopback,
			net.ParseIP(controlPlaneIP),
		},
	}

	return &certs.Config{
		CommonName: "kube-apiserver",
		AltNames:   *altNames,
		Usages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
}

// adminClientCertificateConfig returns the config for an admin client certificate
// to be used for access to the API server.
func adminClientCertificateConfig() *certs.Config {
	return &certs.Config{
		CommonName:   "kubernetes-admin",
		Organization: []string{"system:masters"},
		Usages:       []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
}

// etcdServerCertificateConfig returns the config for an etcd member serving certificate.
func etcdServerCertificateConfig(podName, podIP string) *certs.Config {
	altNames := certs.AltNames{
		DNSNames: []string{
			"localhost",
			podName,
		},
		IPs: []net.IP{
			net.IPv4(127, 0, 0, 1),
			net.IPv6loopback,
			net.ParseIP(podIP),
		},
	}

	return &certs.Config{
		CommonName: podName,
		AltNames:   altNames,
		Usages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
}
