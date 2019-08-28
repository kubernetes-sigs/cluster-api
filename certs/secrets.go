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
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
)

// NewSecretsFromCertificates returns a list of new secrets, 1 for each certificate
func NewSecretsFromCertificates(input *Certificates) []*corev1.Secret {
	return []*corev1.Secret{
		toSecret(ClusterCAName, input.ClusterCA),
		toSecret(EtcdCAName, input.EtcdCA),
		toSecret(ServiceAccountName, input.ServiceAccount),
		toSecret(FrontProxyCAName, input.FrontProxyCA),
	}
}

// NewCertificatesFromSecrets returns the certificates for the cluster by retrieving
// each certificate from 1 secret
func NewCertificatesFromSecrets(secrets *corev1.SecretList) (*Certificates, error) {
	var certs Certificates
	for _, secret := range secrets.Items {
		certs.Set(getCertName(secret.ObjectMeta.Name), getKeyPair(secret))
	}
	if certs.ClusterCA == nil {
		return nil, errors.Errorf("unable to find secret for ClusterCA, got secrets: %s", getSecretNames(secrets))
	}
	if certs.EtcdCA == nil {
		return nil, errors.Errorf("unable to find secret for EtcdCA, got secrets: %s", getSecretNames(secrets))
	}
	if certs.FrontProxyCA == nil {
		return nil, errors.Errorf("unable to find secret for FrontProxyCA, got secrets: %s", getSecretNames(secrets))
	}
	if certs.ServiceAccount == nil {
		return nil, errors.Errorf("unable to find secret for ServiceAccount, got secrets: %s", getSecretNames(secrets))
	}
	return &certs, nil
}

func getSecretNames(secrets *corev1.SecretList) string {
	var names []string
	for _, secret := range secrets.Items {
		names = append(names, secret.ObjectMeta.Name)
	}
	return strings.Join(names, ", ")
}

func getKeyPair(secret corev1.Secret) *KeyPair {
	return &KeyPair{
		Cert: secret.Data[tlsCrt],
		Key:  secret.Data[tlsKey],
	}
}

func getCertName(name string) string {
	parts := strings.Split(name, "-")
	return parts[len(parts)-1]
}

func toSecret(name string, keyPair *KeyPair) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: make(map[string]string),
		},
		Data: map[string][]byte{
			tlsKey: keyPair.Key,
			tlsCrt: keyPair.Cert,
		},
	}
}
