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
	"errors"
	"testing"

	"github.com/blang/semver"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCluster_ReconcileKubeletRBACBinding_NoError(t *testing.T) {
	type wantRBAC struct {
		role        ctrlclient.ObjectKey
		roleBinding ctrlclient.ObjectKey
	}
	tests := []struct {
		name    string
		client  ctrlclient.Client
		version semver.Version
		want    *wantRBAC
	}{
		{
			name:    "creates role and role binding for Kubernetes/kubeadm < v1.24",
			client:  fake.NewClientBuilder().Build(),
			version: semver.MustParse("1.23.3"),
			want: &wantRBAC{
				role:        ctrlclient.ObjectKey{Namespace: metav1.NamespaceSystem, Name: "kubeadm:kubelet-config-1.23"},
				roleBinding: ctrlclient.ObjectKey{Namespace: metav1.NamespaceSystem, Name: "kubeadm:kubelet-config-1.23"},
			},
		},
		{
			name: "tolerates existing role binding for Kubernetes/kubeadm < v1.24",
			client: fake.NewClientBuilder().WithObjects(
				&rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceSystem, Name: "kubeadm:kubelet-config-1.23"}, RoleRef: rbacv1.RoleRef{
					Name: "kubeadm:kubelet-config-1.23",
				}},
				&rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceSystem, Name: "kubeadm:kubelet-config-1.23"}, Rules: []rbacv1.PolicyRule{{
					Verbs:         []string{"get"},
					APIGroups:     []string{""},
					Resources:     []string{"configmaps"},
					ResourceNames: []string{"kubelet-config-1.23"},
				}}},
			).Build(),
			version: semver.MustParse("1.23.3"),
			want: &wantRBAC{
				role:        ctrlclient.ObjectKey{Namespace: metav1.NamespaceSystem, Name: "kubeadm:kubelet-config-1.23"},
				roleBinding: ctrlclient.ObjectKey{Namespace: metav1.NamespaceSystem, Name: "kubeadm:kubelet-config-1.23"},
			},
		},
		{
			name:    "creates role and role binding for Kubernetes/kubeadm >= v1.24",
			client:  fake.NewClientBuilder().Build(),
			version: semver.MustParse("1.24.0"),
			want: &wantRBAC{
				role:        ctrlclient.ObjectKey{Namespace: metav1.NamespaceSystem, Name: "kubeadm:kubelet-config"},
				roleBinding: ctrlclient.ObjectKey{Namespace: metav1.NamespaceSystem, Name: "kubeadm:kubelet-config"},
			},
		},
		{
			name:    "creates role and role binding for Kubernetes/kubeadm >= v1.24 ignoring pre-release and build tags",
			client:  fake.NewClientBuilder().Build(),
			version: semver.MustParse("1.24.0-alpha.1+xyz.1"),
			want: &wantRBAC{
				role:        ctrlclient.ObjectKey{Namespace: metav1.NamespaceSystem, Name: "kubeadm:kubelet-config"},
				roleBinding: ctrlclient.ObjectKey{Namespace: metav1.NamespaceSystem, Name: "kubeadm:kubelet-config"},
			},
		},
		{
			name: "tolerates existing role binding for Kubernetes/kubeadm >= v1.24",
			client: fake.NewClientBuilder().WithObjects(
				&rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceSystem, Name: "kubeadm:kubelet-config"}, RoleRef: rbacv1.RoleRef{
					Name: "kubeadm:kubelet-config",
				}},
				&rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceSystem, Name: "kubeadm:kubelet-config"}, Rules: []rbacv1.PolicyRule{{
					Verbs:         []string{"get"},
					APIGroups:     []string{""},
					Resources:     []string{"configmaps"},
					ResourceNames: []string{"kubelet-config"},
				}}},
			).Build(),
			version: semver.MustParse("1.24.1"),
			want: &wantRBAC{
				role:        ctrlclient.ObjectKey{Namespace: metav1.NamespaceSystem, Name: "kubeadm:kubelet-config"},
				roleBinding: ctrlclient.ObjectKey{Namespace: metav1.NamespaceSystem, Name: "kubeadm:kubelet-config"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			c := &Workload{
				Client: tt.client,
			}
			g.Expect(c.ReconcileKubeletRBACBinding(ctx, tt.version)).To(Succeed())
			g.Expect(c.ReconcileKubeletRBACRole(ctx, tt.version)).To(Succeed())
			if tt.want != nil {
				r := &rbacv1.Role{}
				// Role exists
				g.Expect(tt.client.Get(ctx, tt.want.role, r)).To(Succeed())
				// Role ensure grants for the KubeletConfig config map
				g.Expect(r.Rules).To(Equal([]rbacv1.PolicyRule{
					{
						Verbs:         []string{"get"},
						APIGroups:     []string{""},
						Resources:     []string{"configmaps"},
						ResourceNames: []string{generateKubeletConfigName(tt.version)},
					},
				}))
				// RoleBinding exists
				b := &rbacv1.RoleBinding{}
				// RoleBinding refers to the role
				g.Expect(tt.client.Get(ctx, tt.want.roleBinding, b)).To(Succeed())
				g.Expect(b.RoleRef.Name).To(Equal(tt.want.role.Name))
			}
		})
	}
}

func TestCluster_ReconcileKubeletRBACBinding_Error(t *testing.T) {
	tests := []struct {
		name   string
		client ctrlclient.Client
	}{
		{
			name: "client fails to update an expected error or the role binding/role",
			client: &fakeClient{
				createErr: errors.New(""),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			c := &Workload{
				Client: tt.client,
			}
			g.Expect(c.ReconcileKubeletRBACBinding(ctx, semver.MustParse("1.12.3"))).NotTo(Succeed())
			g.Expect(c.ReconcileKubeletRBACRole(ctx, semver.MustParse("1.13.3"))).NotTo(Succeed())
		})
	}
}

func TestCluster_AllowBootstrapTokensToGetNodes_NoError(t *testing.T) {
	tests := []struct {
		name   string
		client ctrlclient.Client
	}{
		{
			name: "role binding and role already exist",
			client: &fakeClient{
				get: map[string]interface{}{
					GetNodesClusterRoleName: &rbacv1.ClusterRoleBinding{},
				},
			},
		},
		{
			name:   "role binding and role don't exist",
			client: &fakeClient{},
		},
		{
			name: "create returns an already exists error",
			client: &fakeClient{
				createErr: apierrors.NewAlreadyExists(schema.GroupResource{}, ""),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			c := &Workload{
				Client: tt.client,
			}
			g.Expect(c.AllowBootstrapTokensToGetNodes(ctx)).To(Succeed())
		})
	}
}

func TestCluster_AllowBootstrapTokensToGetNodes_Error(t *testing.T) {
	tests := []struct {
		name   string
		client ctrlclient.Client
	}{
		{
			name: "client fails to retrieve an expected error or the cluster role binding/role",
			client: &fakeClient{
				createErr: errors.New(""),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			c := &Workload{
				Client: tt.client,
			}
			g.Expect(c.AllowBootstrapTokensToGetNodes(ctx)).NotTo(Succeed())
		})
	}
}
