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

// Package kubeconfig implements utilities for working with kubeconfigs.
package kubeconfig

import (
	"context"
	"crypto"
	"crypto/x509"
	"fmt"
	"net/url"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/certs"
	"sigs.k8s.io/cluster-api/util/secret"
)

var (
	// ErrDependentCertificateNotFound signals that a CA secret could not be found.
	ErrDependentCertificateNotFound = errors.New("could not find secret ca")
)

// FromSecret fetches the Kubeconfig for a Cluster.
func FromSecret(ctx context.Context, c client.Reader, cluster client.ObjectKey) ([]byte, error) {
	out, err := secret.Get(ctx, c, cluster, secret.Kubeconfig)
	if err != nil {
		return nil, err
	}
	return toKubeconfigBytes(out)
}

// New creates a new Kubeconfig using the cluster name and specified endpoint.
func New(clusterName, endpoint string, caCert *x509.Certificate, caKey crypto.Signer, options ...KubeConfigOption) (*api.Config, error) {
	cfg := &certs.Config{
		CommonName:   "kubernetes-admin",
		Organization: []string{"system:masters"},
		Usages:       []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	kubeConfigOptions := &KubeConfigOptions{}
	kubeConfigOptions.ApplyOptions(options)

	clientKey, err := certs.NewSigner(kubeConfigOptions.keyEncryptionAlgorithm)
	if err != nil {
		return nil, errors.Wrap(err, "unable to create private key")
	}

	clientCert, err := cfg.NewSignedCert(clientKey, caCert, caKey)
	if err != nil {
		return nil, errors.Wrap(err, "unable to sign certificate")
	}

	encodedClientKey, err := certs.EncodePrivateKeyPEMFromSigner(clientKey)
	if err != nil {
		return nil, errors.Wrap(err, "unable to encode private key")
	}

	userName := fmt.Sprintf("%s-admin", clusterName)
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
				ClientKeyData:         encodedClientKey,
				ClientCertificateData: certs.EncodeCertPEM(clientCert),
			},
		},
		CurrentContext: contextName,
	}, nil
}

// CreateSecret creates the Kubeconfig secret for the given cluster.
func CreateSecret(ctx context.Context, c client.Client, cluster *clusterv1.Cluster, options ...KubeConfigOption) error {
	name := util.ObjectKey(cluster)
	allOptions := append(options,
		SecretAnnotations(cluster.Spec.Kubeconfig.Metadata.Annotations),
		SecretLabels(cluster.Spec.Kubeconfig.Metadata.Labels),
	)
	return CreateSecretWithOwner(ctx, c, name, cluster.Spec.ControlPlaneEndpoint.String(), metav1.OwnerReference{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Cluster",
		Name:       cluster.Name,
		UID:        cluster.UID,
	}, allOptions...)
}

// CreateSecretWithOwner creates the Kubeconfig secret for the given cluster name, namespace, endpoint, and owner reference.
func CreateSecretWithOwner(ctx context.Context, c client.Client, clusterName client.ObjectKey, endpoint string, owner metav1.OwnerReference, options ...KubeConfigOption) error {
	server, err := url.JoinPath("https://", endpoint)
	if err != nil {
		return err
	}
	out, err := generateKubeconfig(ctx, c, clusterName, server, options...)
	if err != nil {
		return err
	}

	return c.Create(ctx, GenerateSecretWithOwner(clusterName, out, owner, options...))
}

// GenerateSecret returns a Kubernetes secret for the given Cluster and kubeconfig data.
func GenerateSecret(cluster *clusterv1.Cluster, data []byte) *corev1.Secret {
	name := util.ObjectKey(cluster)
	return GenerateSecretWithOwner(name, data, metav1.OwnerReference{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Cluster",
		Name:       cluster.Name,
		UID:        cluster.UID,
	})
}

// GenerateSecretWithOwner returns a Kubernetes secret for the given Cluster name, namespace, kubeconfig data, and ownerReference.
func GenerateSecretWithOwner(clusterName client.ObjectKey, data []byte, owner metav1.OwnerReference, options ...KubeConfigOption) *corev1.Secret {
	opts := &KubeConfigOptions{}
	opts.ApplyOptions(options)

	labels := map[string]string{clusterv1.ClusterNameLabel: clusterName.Name}
	for k, v := range opts.extraLabels {
		labels[k] = v
	}
	var annotations map[string]string
	if len(opts.extraAnnotations) > 0 {
		annotations = make(map[string]string, len(opts.extraAnnotations))
		for k, v := range opts.extraAnnotations {
			annotations[k] = v
		}
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        secret.Name(clusterName.Name, secret.Kubeconfig),
			Namespace:   clusterName.Namespace,
			Labels:      labels,
			Annotations: annotations,
			OwnerReferences: []metav1.OwnerReference{
				owner,
			},
		},
		Data: map[string][]byte{
			secret.KubeconfigDataName: data,
		},
		Type: clusterv1.ClusterSecretType,
	}
}

// NeedsClientCertRotation returns whether any of the Kubeconfig secret's client certificates will expire before the given threshold.
func NeedsClientCertRotation(configSecret *corev1.Secret, threshold time.Duration) (bool, error) {
	now := time.Now()

	data, err := toKubeconfigBytes(configSecret)
	if err != nil {
		return false, err
	}

	config, err := clientcmd.Load(data)
	if err != nil {
		return false, errors.Wrap(err, "failed to convert kubeconfig Secret into a clientcmdapi.Config")
	}

	for _, authInfo := range config.AuthInfos {
		cert, err := certs.DecodeCertPEM(authInfo.ClientCertificateData)
		if err != nil {
			return false, errors.Wrap(err, "failed to decode kubeconfig client certificate")
		}
		if cert.NotAfter.Sub(now) < threshold {
			return true, nil
		}
	}

	return false, nil
}

// RegenerateSecret creates and stores a new Kubeconfig in the given secret.
func RegenerateSecret(ctx context.Context, c client.Client, configSecret *corev1.Secret, cluster *clusterv1.Cluster, options ...KubeConfigOption) error {
	clusterName, _, err := secret.ParseSecretName(configSecret.Name)
	if err != nil {
		return errors.Wrap(err, "failed to parse secret name")
	}
	data, err := toKubeconfigBytes(configSecret)
	if err != nil {
		return err
	}

	config, err := clientcmd.Load(data)
	if err != nil {
		return errors.Wrap(err, "failed to convert kubeconfig Secret into a clientcmdapi.Config")
	}
	endpoint := config.Clusters[clusterName].Server
	key := client.ObjectKey{Name: clusterName, Namespace: configSecret.Namespace}
	out, err := generateKubeconfig(ctx, c, key, endpoint, options...)
	if err != nil {
		return err
	}
	configSecret.Data[secret.KubeconfigDataName] = out
	for k, v := range cluster.Spec.Kubeconfig.Metadata.Labels {
		if configSecret.Labels == nil {
			configSecret.Labels = make(map[string]string)
		}
		configSecret.Labels[k] = v
	}
	for k, v := range cluster.Spec.Kubeconfig.Metadata.Annotations {
		if configSecret.Annotations == nil {
			configSecret.Annotations = make(map[string]string)
		}
		configSecret.Annotations[k] = v
	}
	return c.Update(ctx, configSecret)
}

// ReconcileSecretMetadata reconciles labels and annotations on an existing kubeconfig secret
// to match spec.kubeconfig.metadata. Additive only — keys not in spec are not removed.
func ReconcileSecretMetadata(ctx context.Context, c client.Client, configSecret *corev1.Secret, cluster *clusterv1.Cluster) error {
	desired := cluster.Spec.Kubeconfig.Metadata
	if len(desired.Labels) == 0 && len(desired.Annotations) == 0 {
		return nil
	}
	needsUpdate := false
	for k, v := range desired.Labels {
		if configSecret.Labels[k] != v {
			needsUpdate = true
			break
		}
	}
	if !needsUpdate {
		for k, v := range desired.Annotations {
			if configSecret.Annotations[k] != v {
				needsUpdate = true
				break
			}
		}
	}
	if !needsUpdate {
		return nil
	}
	original := configSecret.DeepCopy()
	if configSecret.Labels == nil {
		configSecret.Labels = make(map[string]string)
	}
	for k, v := range desired.Labels {
		configSecret.Labels[k] = v
	}
	if len(desired.Annotations) > 0 {
		if configSecret.Annotations == nil {
			configSecret.Annotations = make(map[string]string)
		}
		for k, v := range desired.Annotations {
			configSecret.Annotations[k] = v
		}
	}
	return c.Patch(ctx, configSecret, client.MergeFrom(original))
}

func generateKubeconfig(ctx context.Context, c client.Client, clusterName client.ObjectKey, endpoint string, options ...KubeConfigOption) ([]byte, error) {
	clusterCA, err := secret.GetFromNamespacedName(ctx, c, clusterName, secret.ClusterCA)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ErrDependentCertificateNotFound
		}
		return nil, err
	}

	cert, err := certs.DecodeCertPEM(clusterCA.Data[secret.TLSCrtDataName])
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode CA Cert")
	} else if cert == nil {
		return nil, errors.New("certificate not found in config")
	}

	key, err := certs.DecodePrivateKeyPEM(clusterCA.Data[secret.TLSKeyDataName])
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode private key")
	} else if key == nil {
		return nil, errors.New("CA private key not found")
	}

	cfg, err := New(clusterName.Name, endpoint, cert, key, options...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate a kubeconfig")
	}

	out, err := clientcmd.Write(*cfg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to serialize config to yaml")
	}
	return out, nil
}

func toKubeconfigBytes(out *corev1.Secret) ([]byte, error) {
	data, ok := out.Data[secret.KubeconfigDataName]
	if !ok {
		return nil, errors.Errorf("missing key %q in secret data", secret.KubeconfigDataName)
	}
	return data, nil
}
