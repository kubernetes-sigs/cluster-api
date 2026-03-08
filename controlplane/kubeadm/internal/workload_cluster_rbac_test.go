/*
Copyright 2026 The Kubernetes Authors.

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
	"testing"

	"github.com/blang/semver/v4"
	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestEnsureKubeadmPermissions(t *testing.T) {
	tests := []struct {
		name          string
		objs          []client.Object
		targetVersion semver.Version
		wantObjs      []client.Object
	}{
		{
			name:          "Add kubeadm:cluster-admins and kubeadm:apiserver-kubelet-client ClusterRoleBinding for K8s <= 1.37",
			objs:          nil,
			targetVersion: semver.MustParse("1.37.5"),
			wantObjs: []client.Object{
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: ClusterAdminsGroupAndClusterRoleBinding,
					},
					RoleRef: rbacv1.RoleRef{
						APIGroup: rbacv1.GroupName,
						Kind:     "ClusterRole",
						Name:     "cluster-admin",
					},
					Subjects: []rbacv1.Subject{
						{
							Kind: rbacv1.GroupKind,
							Name: ClusterAdminsGroupAndClusterRoleBinding,
						},
					},
				},
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: KubeletAPIAdminClusterRoleBindingName,
					},
					RoleRef: rbacv1.RoleRef{
						APIGroup: rbacv1.GroupName,
						Kind:     "ClusterRole",
						Name:     KubeletAPIAdminClusterRoleName,
					},
					Subjects: []rbacv1.Subject{
						{
							Kind: rbacv1.UserKind,
							Name: APIServerKubeletClientCertCommonName,
						},
					},
				},
			},
		},
		{
			name: "Ignore kubeadm:cluster-admins and kubeadm:apiserver-kubelet-client ClusterRoleBinding it they already exist for K8s <= 1.37 (kubeadm started adding those roles in patch versions)",
			objs: []client.Object{
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: ClusterAdminsGroupAndClusterRoleBinding,
					},
					// Intentionally using a different ClusterRoleBinding to check that it is not changed.
				},
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: KubeletAPIAdminClusterRoleBindingName,
					},
					// Intentionally using a different ClusterRoleBinding to check that it is not changed.
				},
			},
			targetVersion: semver.MustParse("1.37.0"),
			wantObjs: []client.Object{
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: ClusterAdminsGroupAndClusterRoleBinding,
					},
				},
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: KubeletAPIAdminClusterRoleBindingName,
					},
				},
			},
		},
		{
			name: "Ignore kubeadm:cluster-admins and kubeadm:apiserver-kubelet-client ClusterRoleBinding it they already exist for K8s >= 1.38 (kubeadm should add those roles or KCP in a previous update)",
			objs: []client.Object{
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: ClusterAdminsGroupAndClusterRoleBinding,
					},
					// Intentionally using a different ClusterRoleBinding to check that it is not changed.
				},
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: KubeletAPIAdminClusterRoleBindingName,
					},
					// Intentionally using a different ClusterRoleBinding to check that it is not changed.
				},
			},
			targetVersion: semver.MustParse("1.38.0"),
			wantObjs: []client.Object{
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: ClusterAdminsGroupAndClusterRoleBinding,
					},
				},
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: KubeletAPIAdminClusterRoleBindingName,
					},
				},
			},
		},
		{
			name:          "Do not add kubeadm:cluster-admins and kubeadm:apiserver-kubelet-client ClusterRoleBinding for K8s >= 1.38 (this should never happen, kubeadm should add those roles or KCP in a previous update)",
			objs:          nil,
			targetVersion: semver.MustParse("1.38.0"),
			wantObjs:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			fakeClient := fake.NewClientBuilder().WithObjects(tt.objs...).Build()

			w := &Workload{
				Client: fakeClient,
			}
			err := w.EnsureKubeadmPermissions(t.Context(), tt.targetVersion)
			g.Expect(err).ToNot(HaveOccurred())

			crbList := &rbacv1.ClusterRoleBindingList{}
			err = fakeClient.List(t.Context(), crbList)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(crbList.Items).To(HaveLen(len(tt.wantObjs)))

			for _, o := range tt.wantObjs {
				obj := o.DeepCopyObject().(client.Object)
				err := fakeClient.Get(t.Context(), client.ObjectKeyFromObject(obj), obj)
				g.Expect(err).ToNot(HaveOccurred())

				o.SetResourceVersion(obj.GetResourceVersion())
				g.Expect(obj).To(Equal(o), cmp.Diff(obj, o))
			}
		})
	}
}
