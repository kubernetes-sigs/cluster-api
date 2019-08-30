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
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"math"
	"math/big"
	"strings"
	"time"

	"github.com/pkg/errors"
	"k8s.io/client-go/util/cert"
)

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

// CertificateHashes hash all the certificates stored in a CA certificate.
func CertificateHashes(certData []byte) ([]string, error) {
	certificates, err := cert.ParseCertsPEM(certData)
	if err != nil {
		return nil, errors.Wrap(err, "unable to parse Cluster CA Certificate")
	}
	out := make([]string, 0)
	for _, c := range certificates {
		out = append(out, HashCert(c))
	}
	return out, nil
}

// HashCert will calculate the sha256 of the incoming certificate.
func HashCert(certificate *x509.Certificate) string {
	spkiHash := sha256.Sum256(certificate.RawSubjectPublicKeyInfo)
	return "sha256:" + strings.ToLower(hex.EncodeToString(spkiHash[:]))
}
