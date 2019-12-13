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

package secret

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Get retrieves the specified Secret (if any) from the given
// cluster name and namespace.
func Get(c client.Client, cluster *clusterv1.Cluster, purpose Purpose) (*corev1.Secret, error) {
	name := types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}
	return GetFromNamespacedName(c, name, purpose)
}

// GetFromNamespacedName retrieves the specified Secret (if any) from the given
// cluster name and namespace.
func GetFromNamespacedName(c client.Client, clusterName types.NamespacedName, purpose Purpose) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	secretKey := client.ObjectKey{
		Namespace: clusterName.Namespace,
		Name:      Name(clusterName.Name, purpose),
	}

	if err := c.Get(context.TODO(), secretKey, secret); err != nil {
		return nil, err
	}

	return secret, nil
}

// Name returns the name of the secret for a cluster.
func Name(cluster string, suffix Purpose) string {
	return fmt.Sprintf("%s-%s", cluster, suffix)
}
