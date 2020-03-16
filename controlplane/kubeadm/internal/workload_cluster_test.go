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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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

func newKubeProxyDS() appsv1.DaemonSet {
	return appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubeProxyKey,
			Namespace: metav1.NamespaceSystem,
		},
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Image: "k8s.gcr.io/kube-proxy:v1.16.2",
							Name:  "kube-proxy",
						},
					},
				},
			},
		},
	}
}

func newKubeProxyDSWithImage(image string) appsv1.DaemonSet {
	ds := newKubeProxyDS()
	ds.Spec.Template.Spec.Containers[0].Image = image
	return ds
}

func TestUpdateKubeProxyImageInfo(t *testing.T) {

	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("unable to setup scheme: %s", err)
	}

	tests := []struct {
		name        string
		ds          appsv1.DaemonSet
		expectErr   bool
		expectImage string
		clientGet   map[string]interface{}
		patchErr    error
		KCP         *v1alpha3.KubeadmControlPlane
	}{
		{
			name:        "succeeds if patch correctly",
			ds:          newKubeProxyDS(),
			expectErr:   false,
			expectImage: "k8s.gcr.io/kube-proxy:v1.16.3",
			KCP:         &v1alpha3.KubeadmControlPlane{Spec: v1alpha3.KubeadmControlPlaneSpec{Version: "v1.16.3"}},
		},
		{
			name:        "returns error if image in kube-proxy ds was in digest format",
			ds:          newKubeProxyDSWithImage("k8s.gcr.io/kube-proxy@sha256:47bfd"),
			expectErr:   true,
			expectImage: "k8s.gcr.io/kube-proxy@sha256:47bfd",
			KCP:         &v1alpha3.KubeadmControlPlane{Spec: v1alpha3.KubeadmControlPlaneSpec{Version: "v1.16.3"}},
		},
		{
			name:        "expects OCI compatible format of tag",
			ds:          newKubeProxyDS(),
			expectErr:   false,
			expectImage: "k8s.gcr.io/kube-proxy:v1.16.3_build1",
			KCP:         &v1alpha3.KubeadmControlPlane{Spec: v1alpha3.KubeadmControlPlaneSpec{Version: "v1.16.3+build1"}},
		},
		{
			name:      "returns error if image in kube-proxy ds was in wrong format",
			ds:        newKubeProxyDSWithImage(""),
			expectErr: true,
			KCP:       &v1alpha3.KubeadmControlPlane{Spec: v1alpha3.KubeadmControlPlaneSpec{Version: "v1.16.3"}},
		},
	}

	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objects := []runtime.Object{
				&tt.ds,
			}
			fakeClient := fake.NewFakeClientWithScheme(scheme, objects...)
			w := &Workload{
				Client: fakeClient,
			}
			err := w.UpdateKubeProxyImageInfo(ctx, tt.KCP)
			if err != nil && !tt.expectErr {
				t.Fatalf("expected no error, got %s", err)
			}
			if err == nil && tt.expectErr {
				t.Fatal("expected error but got none")
			}
			proxyImage, err := getProxyImageInfo(ctx, w.Client)
			if err != nil {
				t.Fatalf("expected no error, got %s", err)
			}
			if proxyImage != tt.expectImage {
				t.Fatalf("expected proxy image %s, got %s", tt.expectImage, proxyImage)
			}
		})
	}
}

func getProxyImageInfo(ctx context.Context, client ctrlclient.Client) (string, error) {
	ds := &appsv1.DaemonSet{}

	if err := client.Get(ctx, ctrlclient.ObjectKey{Name: kubeProxyKey, Namespace: metav1.NamespaceSystem}, ds); err != nil {
		if apierrors.IsNotFound(err) {
			return "", errors.New("no image found")
		}
		return "", errors.New("failed to determine if daemonset already exists")
	}
	container := findKubeProxyContainer(ds)
	if container == nil {
		return "", errors.New("unable to find container")
	}
	return container.Image, nil
}
