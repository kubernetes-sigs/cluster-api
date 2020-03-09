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
	"errors"
	"testing"

	"github.com/blang/semver"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestCluster_ReconcileKubeletRBACBinding_NoError(t *testing.T) {
	tests := []struct {
		name   string
		client ctrlclient.Client
	}{
		{
			name: "role binding and role already exist",
			client: &fakeClient{
				get: map[string]interface{}{
					"kube-system/kubeadm:kubelet-config-1.12": &rbacv1.RoleBinding{},
					"kube-system/kubeadm:kubelet-config-1.13": &rbacv1.Role{},
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
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Workload{
				Client: tt.client,
			}
			if err := c.ReconcileKubeletRBACBinding(ctx, semver.MustParse("1.12.3")); err != nil {
				t.Fatalf("did not expect error: %v", err)
			}
			if err := c.ReconcileKubeletRBACRole(ctx, semver.MustParse("1.13.3")); err != nil {
				t.Fatalf("did not expect error: %v", err)
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
			name: "client fails to retrieve an expected error or the role binding/role",
			client: &fakeClient{
				getErr: errors.New(""),
			},
		},
		{
			name: "fails to create the role binding/role",
			client: &fakeClient{
				createErr: errors.New(""),
			},
		},
	}
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Workload{
				Client: tt.client,
			}
			if err := c.ReconcileKubeletRBACBinding(ctx, semver.MustParse("1.12.3")); err == nil {
				t.Fatalf("expected an error but did not get one")
			}
			if err := c.ReconcileKubeletRBACRole(ctx, semver.MustParse("1.13.3")); err == nil {
				t.Fatalf("expected an error but did not get one")
			}
		})
	}
}

func TestUpdateKubeProxyImageInfo(t *testing.T) {
	ds := &appsv1.DaemonSet{
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Image: "k8s.gcr.io/kube-proxy:v1.16.2"}},
				},
			},
		},
	}

	dsImageInDigestFormat := ds.DeepCopy()
	dsImageInDigestFormat.Spec.Template.Spec.Containers[0].Image = "k8s.gcr.io/kube-proxy@sha256:47bfd"

	dsImageEmpty := ds.DeepCopy()
	dsImageEmpty.Spec.Template.Spec.Containers[0].Image = ""

	tests := []struct {
		name      string
		ds        *appsv1.DaemonSet
		expectErr bool
		clientGet map[string]interface{}
		patchErr  error
	}{
		{
			name:      "succeeds if patch correctly",
			ds:        ds,
			expectErr: false,
			clientGet: map[string]interface{}{
				"kube-system/" + kubeProxyDaemonSetName: ds,
			},
		},
		{
			name:      "returns error if image in kube-proxy ds was in digest format",
			ds:        dsImageInDigestFormat,
			expectErr: true,
			clientGet: map[string]interface{}{
				"kube-system/" + kubeProxyDaemonSetName: dsImageInDigestFormat,
			},
		},
		{
			name:      "returns error if image in kube-proxy ds was in wrong format",
			ds:        ds,
			expectErr: true,
			clientGet: map[string]interface{}{
				"kube-system/" + kubeProxyDaemonSetName: dsImageEmpty,
			},
		},
	}

	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			fakeClient := &fakeClient{
				get: tt.clientGet,
			}
			c := &Workload{
				Client: fakeClient,
			}
			err := c.UpdateKubeProxyImageInfo(ctx, &v1alpha3.KubeadmControlPlane{Spec: v1alpha3.KubeadmControlPlaneSpec{Version: "1.16.3"}})
			if err != nil && !tt.expectErr {
				t.Fatalf("expected no error, got %s", err)
			}
			if err == nil && tt.expectErr {
				t.Fatal("expected error but got none")
			}

		})
	}
}
