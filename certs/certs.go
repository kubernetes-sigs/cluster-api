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
	"encoding/pem"
	"math/big"
	"net"
	"time"

	"github.com/pkg/errors"
)

const (
	rsaKeySize   = 2048
	duration365d = time.Hour * 24 * 365
)

// Certificates hold all the certificates necessary for a Kubernetes cluster
type Certificates struct {
	ClusterCA      *KeyPair
	EtcdCA         *KeyPair
	FrontProxyCA   *KeyPair
	ServiceAccount *KeyPair
}

// NewCertificates generates all the necessary CAs and KeyPairs for a Kubernetes cluster.
// nil values for the parameters will generate new KeyPairs, the same as if kubeadm generated them.
func NewCertificates() (*Certificates, error) {
	cluster, err := generateCACert()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create cluster CA")
	}
	etcd, err := generateCACert()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create Etcd CA")
	}
	frontProxy, err := generateCACert()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create frontproxy CA")
	}
	serviceAccount, err := generateServiceAccountKeys()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create service account key pair")
	}
	return &Certificates{
		ClusterCA:      cluster,
		EtcdCA:         etcd,
		FrontProxyCA:   frontProxy,
		ServiceAccount: serviceAccount,
	}, nil
}

// NewPrivateKey creates an RSA private key
func NewPrivateKey() (*rsa.PrivateKey, error) {
	pk, err := rsa.GenerateKey(rand.Reader, rsaKeySize)
	return pk, errors.WithStack(err)
}

// AltNames contains the domain names and IP addresses that will be added
// to the API Server's x509 certificate SubAltNames field. The values will
// be passed directly to the x509.Certificate object.
type AltNames struct {
	DNSNames []string
	IPs      []net.IP
}

// KeyPair holds the raw bytes for a certificate and key
type KeyPair struct {
	Cert, Key []byte
}

func generateCACert() (*KeyPair, error) {
	x509Cert, privKey, err := NewCertificateAuthority()
	if err != nil {
		return nil, err
	}
	return &KeyPair{
		Cert: EncodeCertPEM(x509Cert),
		Key:  EncodePrivateKeyPEM(privKey),
	}, nil
}

func generateServiceAccountKeys() (*KeyPair, error) {
	saCreds, err := NewPrivateKey()
	if err != nil {
		return nil, err
	}
	saPub, err := EncodePublicKeyPEM(&saCreds.PublicKey)
	if err != nil {
		return nil, err
	}
	return &KeyPair{
		Cert: saPub,
		Key:  EncodePrivateKeyPEM(saCreds),
	}, nil
}

// Config contains the basic fields required for creating a certificate
type Config struct {
	CommonName   string
	Organization []string
	AltNames     AltNames
	Usages       []x509.ExtKeyUsage
}

// NewCertificateAuthority creates new certificate and private key for the certificate authority
func NewCertificateAuthority() (*x509.Certificate, *rsa.PrivateKey, error) {
	key, err := NewPrivateKey()
	if err != nil {
		return nil, nil, err
	}

	cert, err := NewSelfSignedCACert(key)
	if err != nil {
		return nil, nil, err
	}

	return cert, key, nil
}

// NewSelfSignedCACert creates a CA certificate.
func NewSelfSignedCACert(key *rsa.PrivateKey) (*x509.Certificate, error) {
	cfg := Config{
		CommonName: "kubernetes",
	}

	now := time.Now().UTC()

	tmpl := x509.Certificate{
		SerialNumber: new(big.Int).SetInt64(0),
		Subject: pkix.Name{
			CommonName:   cfg.CommonName,
			Organization: cfg.Organization,
		},
		NotBefore:             now,
		NotAfter:              now.Add(duration365d * 10),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		MaxPathLenZero:        true,
		BasicConstraintsValid: true,
		MaxPathLen:            0,
		IsCA:                  true,
	}

	b, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, key.Public(), key)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create self signed CA certificate: %+v", tmpl)
	}

	cert, err := x509.ParseCertificate(b)
	return cert, errors.WithStack(err)
}

// EncodeCertPEM returns PEM-endcoded certificate data.
func EncodeCertPEM(cert *x509.Certificate) []byte {
	block := pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	}
	return pem.EncodeToMemory(&block)
}

// EncodePrivateKeyPEM returns PEM-encoded private key data.
func EncodePrivateKeyPEM(key *rsa.PrivateKey) []byte {
	block := pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}

	return pem.EncodeToMemory(&block)
}

// EncodePublicKeyPEM returns PEM-encoded public key data.
func EncodePublicKeyPEM(key *rsa.PublicKey) ([]byte, error) {
	der, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return []byte{}, errors.WithStack(err)
	}
	block := pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: der,
	}
	return pem.EncodeToMemory(&block), nil
}

// DecodeCertPEM attempts to return a decoded certificate or nil
// if the encoded input does not contain a certificate.
func DecodeCertPEM(encoded []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(encoded)
	if block == nil {
		return nil, nil
	}

	return x509.ParseCertificate(block.Bytes)
}

// DecodePrivateKeyPEM attempts to return a decoded key or nil
// if the encoded input does not contain a private key.
func DecodePrivateKeyPEM(encoded []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(encoded)
	if block == nil {
		return nil, nil
	}

	return x509.ParsePKCS1PrivateKey(block.Bytes)
}
