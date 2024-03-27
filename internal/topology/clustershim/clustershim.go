/*
Copyright 2023 The Kubernetes Authors.

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

// Package clustershim contains clustershim utils.
package clustershim

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/topology/ownerrefs"
)

// New creates a new clustershim.
func New(c *clusterv1.Cluster) *corev1.Secret {
	shim := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-shim", c.Name),
			Namespace: c.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*ownerrefs.OwnerReferenceTo(c, clusterv1.GroupVersion.WithKind("Cluster")),
			},
		},
		Type: clusterv1.ClusterSecretType,
	}
	return shim
}
