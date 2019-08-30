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
	"fmt"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	bootstrapv1 "sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm/api/v1alpha2"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha2"
	"sigs.k8s.io/cluster-api/util/secret"
)

// NewSecretsFromCertificates returns a list of secrets, 1 for each certificate.
func NewSecretsFromCertificates(cluster *clusterv1.Cluster, config *bootstrapv1.KubeadmConfig, c *Certificates) []*corev1.Secret {
	return []*corev1.Secret{
		KeyPairToSecret(cluster, config, string(secret.ClusterCA), c.ClusterCA),
		KeyPairToSecret(cluster, config, EtcdCAName, c.EtcdCA),
		KeyPairToSecret(cluster, config, FrontProxyCAName, c.FrontProxyCA),
		KeyPairToSecret(cluster, config, ServiceAccountName, c.ServiceAccount),
	}
}

// SecretToKeyPair extracts a KeyPair from a Secret.
func SecretToKeyPair(s *corev1.Secret) (*KeyPair, error) {
	cert, exists := s.Data[secret.TLSCrtDataName]
	if !exists {
		return nil, errors.Errorf("missing data for key %s", secret.TLSCrtDataName)
	}

	key, exists := s.Data[secret.TLSKeyDataName]
	if !exists {
		return nil, errors.Errorf("missing data for key %s", secret.TLSKeyDataName)
	}

	return &KeyPair{
		Cert: cert,
		Key:  key,
	}, nil
}

// KeyPairToSecret creates a Secret from a KeyPair.
func KeyPairToSecret(cluster *clusterv1.Cluster, config *bootstrapv1.KubeadmConfig, name string, keyPair *KeyPair) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cluster.Namespace,
			Name:      fmt.Sprintf("%s-%s", cluster.Name, name),
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: bootstrapv1.GroupVersion.String(),
					Kind:       "KubeadmConfig",
					Name:       config.Name,
					UID:        config.UID,
				},
			},
			Labels: map[string]string{
				clusterv1.MachineClusterLabelName: cluster.Name,
			},
		},
		Data: map[string][]byte{
			secret.TLSKeyDataName: keyPair.Key,
			secret.TLSCrtDataName: keyPair.Cert,
		},
	}
}
