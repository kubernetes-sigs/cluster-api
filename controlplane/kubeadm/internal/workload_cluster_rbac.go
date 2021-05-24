/*
Copyright 2020 The Kubernetes Authors.

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

package internal

import (
	"context"
	"fmt"

	"github.com/blang/semver"
	"github.com/pkg/errors"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// NodeBootstrapTokenAuthGroup specifies which group a Node Bootstrap Token should be authenticated in.
	NodeBootstrapTokenAuthGroup = "system:bootstrappers:kubeadm:default-node-token"

	// GetNodesClusterRoleName defines the name of the ClusterRole and ClusterRoleBinding to get nodes.
	GetNodesClusterRoleName = "kubeadm:get-nodes"

	// NodesGroup defines the well-known group for all nodes.
	NodesGroup = "system:nodes"

	// KubeletConfigMapRolePrefix defines base kubelet configuration ConfigMap role prefix.
	KubeletConfigMapRolePrefix = "kubeadm:"

	// KubeletConfigMapName defines base kubelet configuration ConfigMap name.
	KubeletConfigMapName = "kubelet-config-%d.%d"
)

// EnsureResource creates a resoutce if the target resource doesn't exist. If the resource exists already, this function will ignore the resource instead.
func (w *Workload) EnsureResource(ctx context.Context, obj client.Object) error {
	testObj := obj.DeepCopyObject().(client.Object)
	key := client.ObjectKeyFromObject(obj)
	if err := w.Client.Get(ctx, key, testObj); err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrapf(err, "failed to determine if resource %s/%s already exists", key.Namespace, key.Name)
	} else if err == nil {
		// If object already exists, nothing left to do
		return nil
	}
	if err := w.Client.Create(ctx, obj); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return errors.Wrapf(err, "unable to create resource %s/%s on workload cluster", key.Namespace, key.Name)
		}
	}
	return nil
}

// AllowBootstrapTokensToGetNodes creates RBAC rules to allow Node Bootstrap Tokens to list nodes.
func (w *Workload) AllowBootstrapTokensToGetNodes(ctx context.Context) error {
	if err := w.EnsureResource(ctx, &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetNodesClusterRoleName,
			Namespace: metav1.NamespaceSystem,
		},
		Rules: []rbacv1.PolicyRule{
			{
				Verbs:     []string{"get"},
				APIGroups: []string{""},
				Resources: []string{"nodes"},
			},
		},
	}); err != nil {
		return err
	}

	return w.EnsureResource(ctx, &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetNodesClusterRoleName,
			Namespace: metav1.NamespaceSystem,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     GetNodesClusterRoleName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind: rbacv1.GroupKind,
				Name: NodeBootstrapTokenAuthGroup,
			},
		},
	})
}

func generateKubeletConfigName(version semver.Version) string {
	return fmt.Sprintf(KubeletConfigMapName, version.Major, version.Minor)
}

func generateKubeletConfigRoleName(version semver.Version) string {
	return KubeletConfigMapRolePrefix + generateKubeletConfigName(version)
}

// ReconcileKubeletRBACBinding will create a RoleBinding for the new kubelet version during upgrades.
// If the role binding already exists this function is a no-op.
func (w *Workload) ReconcileKubeletRBACBinding(ctx context.Context, version semver.Version) error {
	roleName := generateKubeletConfigRoleName(version)
	return w.EnsureResource(ctx, &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceSystem,
			Name:      roleName,
		},
		Subjects: []rbacv1.Subject{
			{
				APIGroup: rbacv1.GroupName,
				Kind:     rbacv1.GroupKind,
				Name:     NodesGroup,
			},
			{
				APIGroup: rbacv1.GroupName,
				Kind:     rbacv1.GroupKind,
				Name:     NodeBootstrapTokenAuthGroup,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "Role",
			Name:     roleName,
		},
	})
}

// ReconcileKubeletRBACRole will create a Role for the new kubelet version during upgrades.
// If the role already exists this function is a no-op.
func (w *Workload) ReconcileKubeletRBACRole(ctx context.Context, version semver.Version) error {
	return w.EnsureResource(ctx, &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      generateKubeletConfigRoleName(version),
			Namespace: metav1.NamespaceSystem,
		},
		Rules: []rbacv1.PolicyRule{
			{
				Verbs:         []string{"get"},
				APIGroups:     []string{""},
				Resources:     []string{"configmaps"},
				ResourceNames: []string{generateKubeletConfigName(version)},
			},
		},
	})
}
