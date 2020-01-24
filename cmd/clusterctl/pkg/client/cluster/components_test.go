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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/test"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Test_providerComponents_Delete(t *testing.T) {
	labels := map[string]string{
		clusterctlv1.ClusterctlProviderLabelName: "aws",
	}

	crd := unstructured.Unstructured{}
	crd.SetAPIVersion("apiextensions.k8s.io/v1beta1")
	crd.SetKind("CustomResourceDefinition")
	crd.SetName("crd1")
	crd.SetLabels(labels)

	initObjs := []runtime.Object{
		// Namespace (should be deleted only if forceDeleteNamespace)
		&corev1.Namespace{
			TypeMeta: metav1.TypeMeta{
				Kind: "Namespace",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:   "ns1",
				Labels: labels,
			},
		},
		// A provider component (should always be deleted)
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
		// CRDs (should be deleted only if forceDeleteCRD)
		&crd,
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
		provider             clusterctlv1.Provider
		forceDeleteNamespace bool
		forceDeleteCRD       bool
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
				provider:             clusterctlv1.Provider{ObjectMeta: metav1.ObjectMeta{Name: "aws", Namespace: "ns1"}},
				forceDeleteNamespace: false,
				forceDeleteCRD:       false,
			},
			wantDiff: []wantDiff{
				{object: corev1.ObjectReference{APIVersion: "v1", Kind: "Namespace", Name: "ns1"}, deleted: false},                                           //namespace should be preserved
				{object: corev1.ObjectReference{APIVersion: "apiextensions.k8s.io/v1beta1", Kind: "CustomResourceDefinition", Name: "crd1"}, deleted: false}, //crd should be preserved
				{object: corev1.ObjectReference{APIVersion: "v1", Kind: "Pod", Namespace: "ns1", Name: "pod1"}, deleted: true},                               // provider components should be deleted
				{object: corev1.ObjectReference{APIVersion: "v1", Kind: "Pod", Namespace: "ns1", Name: "pod2"}, deleted: false},                              // other objects in the namespace should not be deleted
				{object: corev1.ObjectReference{APIVersion: "v1", Kind: "Pod", Namespace: "ns2", Name: "pod3"}, deleted: false},                              // this object is in another namespace, and should never be touched by delete
			},
			wantErr: false,
		},
		{
			name: "Delete provider and provider namespace, while preserving CRDs",
			args: args{
				provider:             clusterctlv1.Provider{ObjectMeta: metav1.ObjectMeta{Name: "aws", Namespace: "ns1"}},
				forceDeleteNamespace: true,
				forceDeleteCRD:       false,
			},
			wantDiff: []wantDiff{
				{object: corev1.ObjectReference{APIVersion: "v1", Kind: "Namespace", Name: "ns1"}, deleted: true},                                            //namespace should be deleted
				{object: corev1.ObjectReference{APIVersion: "apiextensions.k8s.io/v1beta1", Kind: "CustomResourceDefinition", Name: "crd1"}, deleted: false}, //crd should be preserved
				{object: corev1.ObjectReference{APIVersion: "v1", Kind: "Pod", Namespace: "ns1", Name: "pod1"}, deleted: true},                               // provider components should be deleted
				{object: corev1.ObjectReference{APIVersion: "v1", Kind: "Pod", Namespace: "ns1", Name: "pod2"}, deleted: true},                               // other objects in the namespace goes away when deleting the namespace
				{object: corev1.ObjectReference{APIVersion: "v1", Kind: "Pod", Namespace: "ns2", Name: "pod3"}, deleted: false},                              // this object is in another namespace, and should never be touched by delete
			},
			wantErr: false,
		},

		{
			name: "Delete provider and provider CRDs, while preserving the provider namespace",
			args: args{
				provider:             clusterctlv1.Provider{ObjectMeta: metav1.ObjectMeta{Name: "aws", Namespace: "ns1"}},
				forceDeleteNamespace: false,
				forceDeleteCRD:       true,
			},
			wantDiff: []wantDiff{
				{object: corev1.ObjectReference{APIVersion: "v1", Kind: "Namespace", Name: "ns1"}, deleted: false},                                          //namespace should be preserved
				{object: corev1.ObjectReference{APIVersion: "apiextensions.k8s.io/v1beta1", Kind: "CustomResourceDefinition", Name: "crd1"}, deleted: true}, //crd should be deleted
				{object: corev1.ObjectReference{APIVersion: "v1", Kind: "Pod", Namespace: "ns1", Name: "pod1"}, deleted: true},                              // provider components should be deleted
				{object: corev1.ObjectReference{APIVersion: "v1", Kind: "Pod", Namespace: "ns1", Name: "pod2"}, deleted: false},                             // other objects in the namespace should not be deleted
				{object: corev1.ObjectReference{APIVersion: "v1", Kind: "Pod", Namespace: "ns2", Name: "pod3"}, deleted: false},                             // this object is in another namespace, and should never be touched by delete
			},
			wantErr: false,
		},

		{
			name: "Delete provider, provider namespace and provider CRDs",
			args: args{
				provider:             clusterctlv1.Provider{ObjectMeta: metav1.ObjectMeta{Name: "aws", Namespace: "ns1"}},
				forceDeleteNamespace: true,
				forceDeleteCRD:       true,
			},
			wantDiff: []wantDiff{
				{object: corev1.ObjectReference{APIVersion: "v1", Kind: "Namespace", Name: "ns1"}, deleted: true},                                           //namespace should be deleted
				{object: corev1.ObjectReference{APIVersion: "apiextensions.k8s.io/v1beta1", Kind: "CustomResourceDefinition", Name: "crd1"}, deleted: true}, //crd should be deleted
				{object: corev1.ObjectReference{APIVersion: "v1", Kind: "Pod", Namespace: "ns1", Name: "pod1"}, deleted: true},                              // provider components should be deleted
				{object: corev1.ObjectReference{APIVersion: "v1", Kind: "Pod", Namespace: "ns1", Name: "pod2"}, deleted: true},                              // other objects in the namespace goes away when deleting the namespace
				{object: corev1.ObjectReference{APIVersion: "v1", Kind: "Pod", Namespace: "ns2", Name: "pod3"}, deleted: false},                             // this object is in another namespace, and should never be touched by delete
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proxy := test.NewFakeProxy().WithObjs(initObjs...)
			c := newComponentsClient(proxy)
			err := c.Delete(DeleteOptions{
				Provider:             tt.args.provider,
				ForceDeleteNamespace: tt.args.forceDeleteNamespace,
				ForceDeleteCRD:       tt.args.forceDeleteCRD,
			})
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			cs, err := proxy.NewClient()
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}
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
					if want.object.Namespace == tt.args.provider.Namespace && tt.args.forceDeleteNamespace { // Ignoring namespaced object that should be deleted by the namespace controller.
						continue
					}
					t.Errorf("%v not deleted, expect deleted", key)
				}
			}
		})
	}
}
