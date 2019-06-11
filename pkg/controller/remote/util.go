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

package remote

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	kubeconfigSecretKey = "value"
)

var (
	ErrSecretNotFound     = errors.New("secret not found")
	ErrSecretMissingValue = errors.New("missing value in secret")
)

// KubeConfigSecretName generates the expected name for the Kubeconfig secret
// to access a remote cluster given the cluster's name.
func KubeConfigSecretName(cluster string) string {
	return fmt.Sprintf("%s-kubeconfig", cluster)
}

// GetKubeConfigSecret retrieves the KubeConfig Secret (if any)
// from the given cluster name and namespace.
func GetKubeConfigSecret(c client.Client, cluster, namespace string) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	secretKey := client.ObjectKey{
		Namespace: namespace,
		Name:      KubeConfigSecretName(cluster),
	}

	if err := c.Get(context.TODO(), secretKey, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ErrSecretNotFound
		}
		return nil, err
	}

	return secret, nil
}

// DecodeKubeConfigSecret uses the Secret to retrieve and decode the data.
func DecodeKubeConfigSecret(secret *corev1.Secret) ([]byte, error) {
	encodedKubeconfig, ok := secret.Data[kubeconfigSecretKey]
	if !ok {
		return nil, ErrSecretMissingValue
	}

	kubeconfig, err := base64.StdEncoding.DecodeString(string(encodedKubeconfig))
	if err != nil {
		return nil, err
	}

	return kubeconfig, nil
}
