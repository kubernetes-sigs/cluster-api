/*
Copyright 2020 The Kubernetes Authors.

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
package certificate

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"

	"k8s.io/client-go/util/certificate"
)

type MemoryStore struct {
	Certificate *tls.Certificate
}

func (m *MemoryStore) Current() (*tls.Certificate, error) {
	if m.Certificate == nil {
		noKeyErr := certificate.NoCertKeyError("derp")

		return nil, &noKeyErr
	}

	return m.Certificate, nil
}

func (m *MemoryStore) Update(certData, keyData []byte) (*tls.Certificate, error) {
	cert, err := tls.X509KeyPair(certData, keyData)
	if err != nil {
		return nil, err
	}

	certs, err := x509.ParseCertificates(cert.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("unable to parse certificate data: %w", err)
	}

	cert.Leaf = certs[0]
	m.Certificate = &cert

	return &cert, err
}
