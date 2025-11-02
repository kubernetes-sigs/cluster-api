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

// Package certs implements cert handling utilities.
package certs

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/util/errors"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
)

// NewPrivateKey creates an RSA private key.
func NewPrivateKey() (*rsa.PrivateKey, error) {
	pk, err := rsa.GenerateKey(rand.Reader, DefaultRSAKeySize)
	return pk, errors.WithStack(err)
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
		return nil, errors.New("unable to decode PEM data")
	}

	return x509.ParseCertificate(block.Bytes)
}

// DecodePrivateKeyPEM attempts to return a decoded key or nil
// if the encoded input does not contain a private key.
func DecodePrivateKeyPEM(encoded []byte) (crypto.Signer, error) {
	block, _ := pem.Decode(encoded)
	if block == nil {
		return nil, errors.New("unable to decode PEM data")
	}

	errs := []error{}
	pkcs1Key, pkcs1Err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if pkcs1Err == nil {
		return crypto.Signer(pkcs1Key), nil
	}
	errs = append(errs, pkcs1Err)

	// ParsePKCS1PrivateKey will fail with errors.New for many reasons
	// including if the format is wrong, so we can retry with PKCS8 or EC
	// https://golang.org/src/crypto/x509/pkcs1.go#L58
	pkcs8Key, pkcs8Err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if pkcs8Err == nil {
		pkcs8Signer, ok := pkcs8Key.(crypto.Signer)
		if !ok {
			return nil, errors.New("x509: certificate private key does not implement crypto.Signer")
		}
		return pkcs8Signer, nil
	}
	errs = append(errs, pkcs8Err)

	ecKey, ecErr := x509.ParseECPrivateKey(block.Bytes)
	if ecErr == nil {
		return crypto.Signer(ecKey), nil
	}
	errs = append(errs, ecErr)

	return nil, kerrors.NewAggregate(errs)
}

// NewSigner creates a private key based on the provided encryption key algorithm.
func NewSigner(keyEncryptionAlgorithm bootstrapv1.EncryptionAlgorithmType) (crypto.Signer, error) {
	switch keyEncryptionAlgorithm {
	case bootstrapv1.EncryptionAlgorithmECDSAP256:
		return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	case bootstrapv1.EncryptionAlgorithmECDSAP384:
		return ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	}
	rsaKeySize := rsaKeySizeFromAlgorithmType(keyEncryptionAlgorithm)
	if rsaKeySize == 0 {
		return nil, errors.Errorf("cannot obtain key size from unknown RSA algorithm: %q", keyEncryptionAlgorithm)
	}
	return rsa.GenerateKey(rand.Reader, rsaKeySize)
}

// EncodePrivateKeyPEMFromSigner converts a known private key type of RSA or ECDSA to
// a PEM encoded block or returns an error.
func EncodePrivateKeyPEMFromSigner(key crypto.PrivateKey) ([]byte, error) {
	switch t := key.(type) {
	case *ecdsa.PrivateKey:
		derBytes, err := x509.MarshalECPrivateKey(t)
		if err != nil {
			return nil, err
		}
		block := &pem.Block{
			Type:  "EC PRIVATE KEY",
			Bytes: derBytes,
		}
		return pem.EncodeToMemory(block), nil
	case *rsa.PrivateKey:
		block := &pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(t),
		}
		return pem.EncodeToMemory(block), nil
	default:
		return nil, fmt.Errorf("private key is not a recognized type: %T", key)
	}
}

// EncodePublicKeyPEMFromSigner returns PEM-encoded public key data.
func EncodePublicKeyPEMFromSigner(key crypto.PublicKey) ([]byte, error) {
	der, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return []byte{}, err
	}
	block := pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: der,
	}
	return pem.EncodeToMemory(&block), nil
}

// rsaKeySizeFromAlgorithmType takes a known RSA algorithm defined in the kubeadm API and returns its key size.
// For unknown types it returns 0.
// For an empty type ("") which is the default (zero value) on the API field it returns the default size of 2048.
func rsaKeySizeFromAlgorithmType(keyEncryptionAlgorithm bootstrapv1.EncryptionAlgorithmType) int {
	switch keyEncryptionAlgorithm {
	case bootstrapv1.EncryptionAlgorithmRSA2048, "":
		return 2048
	case bootstrapv1.EncryptionAlgorithmRSA3072:
		return 3072
	case bootstrapv1.EncryptionAlgorithmRSA4096:
		return 4096
	default:
		return 0
	}
}
