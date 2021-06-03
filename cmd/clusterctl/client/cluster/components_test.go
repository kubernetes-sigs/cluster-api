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

package cluster

import (
	"testing"

	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Test_providerComponents_Delete(t *testing.T) {
	labels := map[string]string{
		clusterv1.ProviderLabelName: "infrastructure-infra",
	}

	crd := unstructured.Unstructured{}
	crd.SetAPIVersion("apiextensions.k8s.io/v1beta1")
	crd.SetKind("CustomResourceDefinition")
	crd.SetName("crd1")
	crd.SetLabels(labels)

	mutatingWebhook := unstructured.Unstructured{}
	mutatingWebhook.SetAPIVersion("admissionregistration.k8s.io/v1beta1")
	mutatingWebhook.SetKind("MutatingWebhookConfiguration")
	mutatingWebhook.SetName("mwh1")
	mutatingWebhook.SetLabels(labels)

	initObjs := []client.Object{
		// Namespace (should be deleted only if includeNamespace)
		&corev1.Namespace{
			TypeMeta: metav1.TypeMeta{
				Kind: "Namespace",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:   "ns1",
				Labels: labels,
			},
		},
		// A namespaced provider component (should always be deleted)
		&corev1.Pod{
			TypeMeta: metav1.TypeMeta{
				Kind: "Pod",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns1",
				Name:      "pod1",
				Labels:    labels,
			},
		},
		// Another object in the namespace but not belonging to the provider (should go away only when deleting the namespace)
		&corev1.Pod{
			TypeMeta: metav1.TypeMeta{
				Kind: "Pod",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns1",
				Name:      "pod2",
			},
		},
		// CRDs (should be deleted only if includeCRD)
		&crd,
		&mutatingWebhook,
		// A cluster-wide provider component (should always be deleted)
		&rbacv1.ClusterRole{
			TypeMeta: metav1.TypeMeta{
				Kind: "ClusterRole",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:   "ns1-cluster-role", // global objects belonging to the provider have a namespace prefix.
				Labels: labels,
			},
		},
		// Another cluster-wide object (should never be deleted)
		&rbacv1.ClusterRole{
			TypeMeta: metav1.TypeMeta{
				Kind: "ClusterRole",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: "some-cluster-role",
			},
		},
		// Another object out of the provider namespace (should never be deleted)
		&corev1.Pod{
			TypeMeta: metav1.TypeMeta{
				Kind: "Pod",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns2",
				Name:      "pod3",
				Labels:    labels,
			},
		},
	}

	type args struct {
		provider         clusterctlv1.Provider
		includeNamespace bool
		includeCRD       bool
	}
	type wantDiff struct {
		object  corev1.ObjectReference
		deleted bool
	}

	tests := []struct {
		name     string
		args     args
		wantDiff []wantDiff
		wantErr  bool
	}{
		{
			name: "Delete provider while preserving Namespace and CRDs",
			args: args{
				provider:         clusterctlv1.Provider{ObjectMeta: metav1.ObjectMeta{Name: "infrastructure-infra", Namespace: "ns1"}, ProviderName: "infra", Type: string(clusterctlv1.InfrastructureProviderType)},
				includeNamespace: false,
				includeCRD:       false,
			},
			wantDiff: []wantDiff{
				{object: corev1.ObjectReference{APIVersion: "v1", Kind: "Namespace", Name: "ns1"}, deleted: false},                                                      // namespace should be preserved
				{object: corev1.ObjectReference{APIVersion: "apiextensions.k8s.io/v1beta1", Kind: "CustomResourceDefinition", Name: "crd1"}, deleted: false},            // crd should be preserved
				{object: corev1.ObjectReference{APIVersion: "admissionregistration.k8s.io/v1beta1", Kind: "MutatingWebhookConfiguration", Name: "mwh1"}, deleted: true}, // MutatingWebhookConfiguration goes away with the controller
				{object: corev1.ObjectReference{APIVersion: "v1", Kind: "Pod", Namespace: "ns1", Name: "pod1"}, deleted: true},                                          // provider components should be deleted
				{object: corev1.ObjectReference{APIVersion: "v1", Kind: "Pod", Namespace: "ns1", Name: "pod2"}, deleted: false},                                         // other objects in the namespace should not be deleted
				{object: corev1.ObjectReference{APIVersion: "v1", Kind: "Pod", Namespace: "ns2", Name: "pod3"}, deleted: false},                                         // this object is in another namespace, and should never be touched by delete
				{object: corev1.ObjectReference{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "ClusterRole", Name: "ns1-cluster-role"}, deleted: true},              // cluster-wide provider components should be deleted
				{object: corev1.ObjectReference{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "ClusterRole", Name: "some-cluster-role"}, deleted: false},            // other cluster-wide objects should be preserved
			},
			wantErr: false,
		},
		{
			name: "Delete provider and provider namespace, while preserving CRDs",
			args: args{
				provider:         clusterctlv1.Provider{ObjectMeta: metav1.ObjectMeta{Name: "infrastructure-infra", Namespace: "ns1"}, ProviderName: "infra", Type: string(clusterctlv1.InfrastructureProviderType)},
				includeNamespace: true,
				includeCRD:       false,
			},
			wantDiff: []wantDiff{
				{object: corev1.ObjectReference{APIVersion: "v1", Kind: "Namespace", Name: "ns1"}, deleted: true},                                                       // namespace should be deleted
				{object: corev1.ObjectReference{APIVersion: "apiextensions.k8s.io/v1beta1", Kind: "CustomResourceDefinition", Name: "crd1"}, deleted: false},            // crd should be preserved
				{object: corev1.ObjectReference{APIVersion: "admissionregistration.k8s.io/v1beta1", Kind: "MutatingWebhookConfiguration", Name: "mwh1"}, deleted: true}, // MutatingWebhookConfiguration goes away with the controller
				{object: corev1.ObjectReference{APIVersion: "v1", Kind: "Pod", Namespace: "ns1", Name: "pod1"}, deleted: true},                                          // provider components should be deleted
				{object: corev1.ObjectReference{APIVersion: "v1", Kind: "Pod", Namespace: "ns1", Name: "pod2"}, deleted: true},                                          // other objects in the namespace goes away when deleting the namespace
				{object: corev1.ObjectReference{APIVersion: "v1", Kind: "Pod", Namespace: "ns2", Name: "pod3"}, deleted: false},                                         // this object is in another namespace, and should never be touched by delete
				{object: corev1.ObjectReference{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "ClusterRole", Name: "ns1-cluster-role"}, deleted: true},              // cluster-wide provider components should be deleted
				{object: corev1.ObjectReference{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "ClusterRole", Name: "some-cluster-role"}, deleted: false},            // other cluster-wide objects should be preserved
			},
			wantErr: false,
		},
		{
			name: "Delete provider and provider CRDs, while preserving the provider namespace",
			args: args{
				provider:         clusterctlv1.Provider{ObjectMeta: metav1.ObjectMeta{Name: "infrastructure-infra", Namespace: "ns1"}, ProviderName: "infra", Type: string(clusterctlv1.InfrastructureProviderType)},
				includeNamespace: false,
				includeCRD:       true,
			},
			wantDiff: []wantDiff{
				{object: corev1.ObjectReference{APIVersion: "v1", Kind: "Namespace", Name: "ns1"}, deleted: false},                                                      // namespace should be preserved
				{object: corev1.ObjectReference{APIVersion: "apiextensions.k8s.io/v1beta1", Kind: "CustomResourceDefinition", Name: "crd1"}, deleted: true},             // crd should be deleted
				{object: corev1.ObjectReference{APIVersion: "admissionregistration.k8s.io/v1beta1", Kind: "MutatingWebhookConfiguration", Name: "mwh1"}, deleted: true}, // MutatingWebhookConfiguration should be deleted
				{object: corev1.ObjectReference{APIVersion: "v1", Kind: "Pod", Namespace: "ns1", Name: "pod1"}, deleted: true},                                          // provider components should be deleted
				{object: corev1.ObjectReference{APIVersion: "v1", Kind: "Pod", Namespace: "ns1", Name: "pod2"}, deleted: false},                                         // other objects in the namespace should not be deleted
				{object: corev1.ObjectReference{APIVersion: "v1", Kind: "Pod", Namespace: "ns2", Name: "pod3"}, deleted: false},                                         // this object is in another namespace, and should never be touched by delete
				{object: corev1.ObjectReference{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "ClusterRole", Name: "ns1-cluster-role"}, deleted: true},              // cluster-wide provider components should be deleted
				{object: corev1.ObjectReference{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "ClusterRole", Name: "some-cluster-role"}, deleted: false},            // other cluster-wide objects should be preserved
			},
			wantErr: false,
		},
		{
			name: "Delete provider, provider namespace and provider CRDs",
			args: args{
				provider:         clusterctlv1.Provider{ObjectMeta: metav1.ObjectMeta{Name: "infrastructure-infra", Namespace: "ns1"}, ProviderName: "infra", Type: string(clusterctlv1.InfrastructureProviderType)},
				includeNamespace: true,
				includeCRD:       true,
			},
			wantDiff: []wantDiff{
				{object: corev1.ObjectReference{APIVersion: "v1", Kind: "Namespace", Name: "ns1"}, deleted: true},                                                       // namespace should be deleted
				{object: corev1.ObjectReference{APIVersion: "apiextensions.k8s.io/v1beta1", Kind: "CustomResourceDefinition", Name: "crd1"}, deleted: true},             // crd should be deleted
				{object: corev1.ObjectReference{APIVersion: "admissionregistration.k8s.io/v1beta1", Kind: "MutatingWebhookConfiguration", Name: "mwh1"}, deleted: true}, // MutatingWebhookConfiguration should be deleted
				{object: corev1.ObjectReference{APIVersion: "v1", Kind: "Pod", Namespace: "ns1", Name: "pod1"}, deleted: true},                                          // provider components should be deleted
				{object: corev1.ObjectReference{APIVersion: "v1", Kind: "Pod", Namespace: "ns1", Name: "pod2"}, deleted: true},                                          // other objects in the namespace goes away when deleting the namespace
				{object: corev1.ObjectReference{APIVersion: "v1", Kind: "Pod", Namespace: "ns2", Name: "pod3"}, deleted: false},                                         // this object is in another namespace, and should never be touched by delete
				{object: corev1.ObjectReference{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "ClusterRole", Name: "ns1-cluster-role"}, deleted: true},              // cluster-wide provider components should be deleted
				{object: corev1.ObjectReference{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "ClusterRole", Name: "some-cluster-role"}, deleted: false},            // other cluster-wide objects should be preserved
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			proxy := test.NewFakeProxy().WithObjs(initObjs...)
			c := newComponentsClient(proxy)
			err := c.Delete(DeleteOptions{
				Provider:         tt.args.provider,
				IncludeNamespace: tt.args.includeNamespace,
				IncludeCRDs:      tt.args.includeCRD,
			})
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).NotTo(HaveOccurred())

			cs, err := proxy.NewClient()
			g.Expect(err).NotTo(HaveOccurred())

			for _, want := range tt.wantDiff {
				obj := &unstructured.Unstructured{}
				obj.SetAPIVersion(want.object.APIVersion)
				obj.SetKind(want.object.Kind)

				key := client.ObjectKey{
					Namespace: want.object.Namespace,
					Name:      want.object.Name,
				}

				err := cs.Get(ctx, key, obj)
				if err != nil && !apierrors.IsNotFound(err) {
					t.Fatalf("Failed to get %v from the cluster: %v", key, err)
				}

				if !want.deleted && apierrors.IsNotFound(err) {
					t.Errorf("%v deleted, expect NOT deleted", key)
				}

				if want.deleted && !apierrors.IsNotFound(err) {
					if want.object.Namespace == tt.args.provider.Namespace && tt.args.includeNamespace { // Ignoring namespaced object that should be deleted by the namespace controller.
						continue
					}
					t.Errorf("%v not deleted, expect deleted", key)
				}
			}
		})
	}
}

func Test_providerComponents_DeleteCoreProviderWebhookNamespace(t *testing.T) {
	t.Run("deletes capi-webhook-system namespace", func(t *testing.T) {
		g := NewWithT(t)
		labels := map[string]string{
			"foo": "bar",
		}
		initObjs := []client.Object{
			&corev1.Namespace{
				TypeMeta: metav1.TypeMeta{
					Kind: "Namespace",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:   "capi-webhook-system",
					Labels: labels,
				},
			},
		}

		proxy := test.NewFakeProxy().WithObjs(initObjs...)
		proxyClient, _ := proxy.NewClient()
		var nsList corev1.NamespaceList

		// assert length before deleting
		_ = proxyClient.List(ctx, &nsList)
		g.Expect(len(nsList.Items)).Should(Equal(1))

		c := newComponentsClient(proxy)
		err := c.DeleteWebhookNamespace()
		g.Expect(err).To(Not(HaveOccurred()))

		// assert length after deleting
		_ = proxyClient.List(ctx, &nsList)
		g.Expect(len(nsList.Items)).Should(Equal(0))
	})
}
