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

package kubeconfig

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"fmt"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha2"
	"sigs.k8s.io/cluster-api/util/certs"
	"sigs.k8s.io/cluster-api/util/secret"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// FromSecret fetches the Kubeconfig for a Cluster.
func FromSecret(c client.Client, cluster *clusterv1.Cluster) ([]byte, error) {
	out, err := secret.Get(c, cluster, secret.Kubeconfig)
	if err != nil {
		return nil, err
	}
	data, ok := out.Data[secret.KubeconfigDataName]
	if !ok {
		return nil, errors.Errorf("missing key %q in secret data", secret.KubeconfigDataName)
	}
	return data, nil
}

// New creates a new Kubeconfig using the cluster name and specified endpoint.
func New(clusterName, endpoint string, caCert *x509.Certificate, caKey *rsa.PrivateKey) (*api.Config, error) {
	cfg := &certs.Config{
		CommonName:   "kubernetes-admin",
		Organization: []string{"system:masters"},
		Usages:       []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	clientKey, err := certs.NewPrivateKey()
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
				CertificateAuthorityData: certs.EncodeCertPEM(caCert),
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
				ClientKeyData:         certs.EncodePrivateKeyPEM(clientKey),
				ClientCertificateData: certs.EncodeCertPEM(clientCert),
			},
		},
		CurrentContext: contextName,
	}, nil
}

// CreateSecret creates the Kubeconfig secret for the given cluster.
func CreateSecret(ctx context.Context, c client.Client, cluster *clusterv1.Cluster) error {
	clusterCA, err := secret.Get(c, cluster, secret.ClusterCA)
	if err != nil {
		return err
	}

	cert, err := certs.DecodeCertPEM(clusterCA.Data[secret.TLSCrtDataName])
	if err != nil {
		return errors.Wrap(err, "failed to decode CA Cert")
	} else if cert == nil {
		return errors.New("certificate not found in config")
	}

	key, err := certs.DecodePrivateKeyPEM(clusterCA.Data[secret.TLSKeyDataName])
	if err != nil {
		return errors.Wrap(err, "failed to decode private key")
	} else if key == nil {
		return errors.New("CA private key not found")
	}

	server := fmt.Sprintf("https://%s:%d", cluster.Status.APIEndpoints[0].Host, cluster.Status.APIEndpoints[0].Port)
	cfg, err := New(cluster.Name, server, cert, key)
	if err != nil {
		return errors.Wrap(err, "failed to generate a kubeconfig")
	}

	out, err := clientcmd.Write(*cfg)
	if err != nil {
		return errors.Wrap(err, "failed to serialize config to yaml")
	}

	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secret.Name(cluster.Name, secret.Kubeconfig),
			Namespace: cluster.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: clusterv1.GroupVersion.String(),
					Kind:       "Cluster",
					Name:       cluster.Name,
					UID:        cluster.UID,
				},
			},
		},
		Data: map[string][]byte{
			secret.KubeconfigDataName: out,
		},
	}
	return c.Create(ctx, s)
}
