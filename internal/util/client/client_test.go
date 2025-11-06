/*
Copyright 2025 The Kubernetes Authors.

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

// Package client provides utils for usage with the controller-runtime client.
package client

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

func Test_WaitForCacheToBeUpToDate(t *testing.T) {
	// Modify timeout to speed up test
	waitBackoff = wait.Backoff{
		Duration: 25 * time.Microsecond,
		Cap:      2 * time.Second,
		Factor:   1.2,
		Steps:    5,
	}

	tests := []struct {
		name            string
		objs            []client.Object
		clientResponses map[client.ObjectKey][]client.Object
		wantErr         string
	}{
		{
			name: "no-op if no objects are passed in",
		},
		{
			name: "error if passed in objects have invalid resourceVersion",
			objs: []client.Object{
				machine("machine-1", "invalidResourceVersion", nil),
				machine("machine-2", "invalidResourceVersion", nil),
			},
			clientResponses: map[client.ObjectKey][]client.Object{
				{Namespace: metav1.NamespaceDefault, Name: "machine-1"}: {
					machine("machine-1", "1", nil),
				},
				{Namespace: metav1.NamespaceDefault, Name: "machine-2"}: {
					machine("machine-2", "2", nil),
				},
			},
			wantErr: "failed to wait for up-to-date Machine objects in the cache after Machine creation: " +
				"default/machine-1: cannot compare with invalid resourceVersion: current: 1, expected to be >= invalidResourceVersion: resource version is not well formed: invalidResourceVersion",
		},
		{
			name: "error if objects from cache have invalid resourceVersion",
			objs: []client.Object{
				machine("machine-1", "1", nil),
				machine("machine-2", "2", nil),
			},
			clientResponses: map[client.ObjectKey][]client.Object{
				{Namespace: metav1.NamespaceDefault, Name: "machine-1"}: {
					machine("machine-1", "invalidResourceVersion", nil),
				},
				{Namespace: metav1.NamespaceDefault, Name: "machine-2"}: {
					machine("machine-2", "invalidResourceVersion", nil),
				},
			},
			wantErr: "failed to wait for up-to-date Machine objects in the cache after Machine creation: " +
				"default/machine-1: cannot compare with invalid resourceVersion: current: invalidResourceVersion, expected to be >= 1: resource version is not well formed: invalidResourceVersion",
		},
		{
			name: "error if objects never show up in the cache",
			objs: []client.Object{
				machine("machine-1", "1", nil),
				machine("machine-2", "2", nil),
				machine("machine-3", "3", nil),
				machine("machine-4", "4", nil),
			},
			clientResponses: map[client.ObjectKey][]client.Object{},
			wantErr: "failed to wait for up-to-date Machine objects in the cache after Machine creation: timed out: [" +
				"machines.cluster.x-k8s.io \"machine-1\" not found, " +
				"machines.cluster.x-k8s.io \"machine-2\" not found, " +
				"machines.cluster.x-k8s.io \"machine-3\" not found, " +
				"machines.cluster.x-k8s.io \"machine-4\" not found]",
		},
		{
			name: "success if objects are instantly up-to-date",
			objs: []client.Object{
				machine("machine-1", "", nil),
				machine("machine-2", "2", nil),
				machine("machine-3", "3", nil),
				machine("machine-4", "4", nil),
			},
			clientResponses: map[client.ObjectKey][]client.Object{
				{Namespace: metav1.NamespaceDefault, Name: "machine-1"}: {
					// For this object it's enough if it shows up, exact resourceVersion doesn't matter.
					machine("machine-1", "5", nil),
				},
				{Namespace: metav1.NamespaceDefault, Name: "machine-2"}: {
					machine("machine-2", "2", nil),
				},
				{Namespace: metav1.NamespaceDefault, Name: "machine-3"}: {
					machine("machine-3", "3", nil),
				},
				{Namespace: metav1.NamespaceDefault, Name: "machine-4"}: {
					// This object has an even newer resourceVersion.
					machine("machine-4", "6", nil),
				},
			},
		},
		{
			name: "success if objects are up-to-date after a few tries",
			objs: []client.Object{
				machine("machine-1", "", nil),
				machine("machine-2", "10", nil),
				machine("machine-3", "11", nil),
				machine("machine-4", "12", nil),
			},
			clientResponses: map[client.ObjectKey][]client.Object{
				{Namespace: metav1.NamespaceDefault, Name: "machine-1"}: {
					// For this object it's enough if it shows up, exact resourceVersion doesn't matter.
					machine("machine-1", "4", nil),
				},
				{Namespace: metav1.NamespaceDefault, Name: "machine-2"}: {
					machine("machine-2", "1", nil),
					machine("machine-2", "5", nil),
					machine("machine-2", "10", nil),
				},
				{Namespace: metav1.NamespaceDefault, Name: "machine-3"}: {
					machine("machine-3", "2", nil),
					machine("machine-3", "3", nil),
					machine("machine-3", "7", nil),
					machine("machine-3", "11", nil),
				},
				{Namespace: metav1.NamespaceDefault, Name: "machine-4"}: {
					machine("machine-4", "3", nil),
					machine("machine-4", "6", nil),
					machine("machine-4", "8", nil),
					machine("machine-4", "13", nil),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			scheme := runtime.NewScheme()
			_ = clusterv1.AddToScheme(scheme)

			callCounter := map[client.ObjectKey]int{}
			fakeClient := interceptor.NewClient(fake.NewClientBuilder().WithScheme(scheme).Build(), interceptor.Funcs{
				Get: func(ctx context.Context, _ client.WithWatch, key client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
					if len(tt.clientResponses) == 0 || len(tt.clientResponses[key]) == 0 {
						return apierrors.NewNotFound(schema.GroupResource{
							Group:    clusterv1.GroupVersion.Group,
							Resource: "machines",
						}, key.Name)
					}

					currentCall := callCounter[key]
					currentCall = min(currentCall, len(tt.clientResponses[key])-1)

					// Write back the modified object so callers can access the patched object.
					if err := scheme.Convert(tt.clientResponses[key][currentCall], obj, ctx); err != nil {
						return errors.Wrapf(err, "unexpected error: failed to get")
					}

					callCounter[key]++

					return nil
				},
			})

			err := WaitForCacheToBeUpToDate(t.Context(), fakeClient, "Machine creation", tt.objs...)
			if tt.wantErr != "" {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(Equal(tt.wantErr))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func Test_WaitForObjectsToBeDeletedFromTheCache(t *testing.T) {
	// Modify timeout to speed up test
	waitBackoff = wait.Backoff{
		Duration: 25 * time.Microsecond,
		Cap:      2 * time.Second,
		Factor:   1.2,
		Steps:    5,
	}

	tests := []struct {
		name            string
		objs            []client.Object
		clientResponses map[client.ObjectKey][]client.Object
		wantErr         string
	}{
		{
			name: "no-op if no objects are passed in",
		},
		{
			name: "error if Unstructured is used",
			objs: []client.Object{
				&unstructured.Unstructured{},
			},
			wantErr: "failed to wait for up-to-date objects in the cache after Machine deletion: Unstructured is not supported",
		},
		{
			name: "success if objects are going away instantly (not found)",
			objs: []client.Object{
				machine("machine-1", "", nil),
				machine("machine-2", "2", nil),
				machine("machine-3", "3", nil),
				machine("machine-4", "4", nil),
			},
			clientResponses: map[client.ObjectKey][]client.Object{},
		},
		{
			name: "success if objects are going away instantly (deletionTimestamp)",
			objs: []client.Object{
				machine("machine-1", "1", nil),
				machine("machine-2", "2", nil),
				machine("machine-3", "3", nil),
				machine("machine-4", "4", nil),
			},
			clientResponses: map[client.ObjectKey][]client.Object{
				{Namespace: metav1.NamespaceDefault, Name: "machine-1"}: {
					machine("machine-1", "1", ptr.To(metav1.Now())),
				},
				{Namespace: metav1.NamespaceDefault, Name: "machine-2"}: {
					machine("machine-2", "2", ptr.To(metav1.Now())),
				},
				{Namespace: metav1.NamespaceDefault, Name: "machine-3"}: {
					machine("machine-3", "3", ptr.To(metav1.Now())),
				},
				{Namespace: metav1.NamespaceDefault, Name: "machine-4"}: {
					machine("machine-4", "4", ptr.To(metav1.Now())),
				},
			},
		},
		{
			name: "success if objects are going away after a few tries (deletionTimestamp)",
			objs: []client.Object{
				machine("machine-1", "1", nil),
				machine("machine-2", "2", nil),
				machine("machine-3", "3", nil),
				machine("machine-4", "4", nil),
			},
			clientResponses: map[client.ObjectKey][]client.Object{
				{Namespace: metav1.NamespaceDefault, Name: "machine-1"}: {
					machine("machine-1", "1", ptr.To(metav1.Now())),
				},
				{Namespace: metav1.NamespaceDefault, Name: "machine-2"}: {
					machine("machine-2", "2", nil),
					machine("machine-2", "2", nil),
					machine("machine-2", "5", ptr.To(metav1.Now())),
				},
				{Namespace: metav1.NamespaceDefault, Name: "machine-3"}: {
					machine("machine-3", "3", nil),
					machine("machine-3", "3", nil),
					machine("machine-3", "3", nil),
					machine("machine-3", "6", ptr.To(metav1.Now())),
				},
				{Namespace: metav1.NamespaceDefault, Name: "machine-4"}: {
					machine("machine-4", "4", nil),
					machine("machine-4", "4", nil),
					machine("machine-4", "4", nil),
					machine("machine-4", "7", ptr.To(metav1.Now())),
				},
			},
		},
		{
			name: "error if objects are not going away after a few tries",
			objs: []client.Object{
				machine("machine-1", "1", nil),
				machine("machine-2", "2", nil),
				machine("machine-3", "3", nil),
				machine("machine-4", "4", nil),
			},
			clientResponses: map[client.ObjectKey][]client.Object{
				{Namespace: metav1.NamespaceDefault, Name: "machine-1"}: {
					machine("machine-1", "1", nil),
				},
				{Namespace: metav1.NamespaceDefault, Name: "machine-2"}: {
					machine("machine-2", "2", nil),
					machine("machine-2", "2", nil),
					machine("machine-2", "5", nil),
				},
				{Namespace: metav1.NamespaceDefault, Name: "machine-3"}: {
					machine("machine-3", "3", nil),
					machine("machine-3", "3", nil),
					machine("machine-3", "3", nil),
					machine("machine-3", "6", nil),
				},
				{Namespace: metav1.NamespaceDefault, Name: "machine-4"}: {
					machine("machine-4", "4", nil),
					machine("machine-4", "4", nil),
					machine("machine-4", "4", nil),
					machine("machine-4", "7", nil),
				},
			},
			wantErr: "failed to wait for up-to-date Machine objects in the cache after Machine deletion: timed out: [" +
				"default/machine-1 still exists, " +
				"default/machine-2 still exists, " +
				"default/machine-3 still exists, " +
				"default/machine-4 still exists]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			scheme := runtime.NewScheme()
			_ = clusterv1.AddToScheme(scheme)

			callCounter := map[client.ObjectKey]int{}
			fakeClient := interceptor.NewClient(fake.NewClientBuilder().WithScheme(scheme).Build(), interceptor.Funcs{
				Get: func(ctx context.Context, _ client.WithWatch, key client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
					if len(tt.clientResponses) == 0 || len(tt.clientResponses[key]) == 0 {
						return apierrors.NewNotFound(schema.GroupResource{
							Group:    clusterv1.GroupVersion.Group,
							Resource: "machines",
						}, key.Name)
					}

					currentCall := callCounter[key]
					currentCall = min(currentCall, len(tt.clientResponses[key])-1)

					// Write back the modified object so callers can access the patched object.
					if err := scheme.Convert(tt.clientResponses[key][currentCall], obj, ctx); err != nil {
						return errors.Wrapf(err, "unexpected error: failed to get")
					}

					callCounter[key]++

					return nil
				},
			})

			err := WaitForObjectsToBeDeletedFromTheCache(t.Context(), fakeClient, "Machine deletion", tt.objs...)
			if tt.wantErr != "" {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(Equal(tt.wantErr))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func machine(name, resourceVersion string, deletionTimestamp *metav1.Time) client.Object {
	return &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         metav1.NamespaceDefault,
			Name:              name,
			ResourceVersion:   resourceVersion,
			DeletionTimestamp: deletionTimestamp,
		},
	}
}
