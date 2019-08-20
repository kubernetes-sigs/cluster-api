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

package controllers

import (
	"context"
	"errors"
	"fmt"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clusterv2 "sigs.k8s.io/cluster-api/api/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

func TestControlPlaneInitLockerAcquire(t *testing.T) {

	clusterName := "test-cluster"
	clusterNamespace := "test-namespace"
	uid := types.UID("test-uid")

	tests := []struct {
		name          string
		context       context.Context
		client        client.Client
		shouldAcquire bool
	}{
		{
			name:    "acquire lock",
			context: context.Background(),
			client: &fakeClient{
				getError: apierrors.NewNotFound(schema.GroupResource{Group: "", Resource: "configmaps"}, fmt.Sprintf("%s-controlplane", uid)),
			},
			shouldAcquire: true,
		},
		{
			name:          "should not acquire lock if already exits",
			context:       context.Background(),
			client:        &fakeClient{},
			shouldAcquire: false,
		},
		{
			name:    "shoult not acquire lock if cannot create config map",
			context: context.Background(),
			client: &fakeClient{
				getError:    apierrors.NewNotFound(schema.GroupResource{Group: "", Resource: "configmaps"}, fmt.Sprintf("%s-controlplane", uid)),
				createError: errors.New("create error"),
			},
			shouldAcquire: false,
		},
		{
			name:    "should not acquire lock if config map alredy exists while creating",
			context: context.Background(),
			client: &fakeClient{
				getError:    apierrors.NewNotFound(schema.GroupResource{Group: "", Resource: "configmaps"}, fmt.Sprintf("%s-controlplane", uid)),
				createError: apierrors.NewAlreadyExists(schema.GroupResource{Group: "", Resource: "configmaps"}, fmt.Sprintf("%s-controlplane", uid)),
			},
			shouldAcquire: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			l := &controlPlaneInitLocker{
				log:    log.ZapLogger(true),
				ctx:    context.Background(),
				client: tc.client,
			}

			cluster := &clusterv2.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: clusterNamespace,
					Name:      clusterName,
					UID:       uid,
				},
			}

			acquired := l.Acquire(cluster)
			if acquired != tc.shouldAcquire {
				t.Fatalf("acquired was %v, but it should be %v\n", acquired, tc.shouldAcquire)
			}

		})
	}
}

func TestControlPlaneInitLockerRelease(t *testing.T) {

	clusterName := "test-cluster"
	clusterNamespace := "test-namespace"
	uid := types.UID("test-uid")

	tests := []struct {
		name          string
		context       context.Context
		client        client.Client
		shouldRelease bool
	}{
		{
			name:          "should release lock by deleting config map",
			context:       context.Background(),
			client:        &fakeClient{},
			shouldRelease: true,
		},
		{
			name:    "should not release lock if cannot delete config map",
			context: context.Background(),
			client: &fakeClient{
				deleteError: errors.New("delete error"),
			},
			shouldRelease: false,
		},
		{
			name:    "should release lock if config map does not exist",
			context: context.Background(),
			client: &fakeClient{
				getError: apierrors.NewNotFound(schema.GroupResource{Group: "", Resource: "configmaps"}, fmt.Sprintf("%s-controlplane", uid)),
			},
			shouldRelease: true,
		},
		{
			name:    "should not release lock if error while getting config map",
			context: context.Background(),
			client: &fakeClient{
				getError: errors.New("get error"),
			},
			shouldRelease: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			l := &controlPlaneInitLocker{
				log:    log.ZapLogger(true),
				ctx:    context.Background(),
				client: tc.client,
			}

			cluster := &clusterv2.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: clusterNamespace,
					Name:      clusterName,
					UID:       uid,
				},
			}

			released := l.Release(cluster)
			if released != tc.shouldRelease {
				t.Fatalf("released was %v, but it should be %v\n", released, tc.shouldRelease)
			}

		})
	}
}

type fakeStatusWriter struct{}

func (fsw *fakeStatusWriter) Update(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
	panic("not implemented")
}

func (fsw *fakeStatusWriter) Patch(ctx context.Context, obj runtime.Object, patch client.Patch, opts ...client.PatchOption) error {
	panic("not implemented")
}

var _ client.Client = &fakeClient{}

type fakeClient struct {
	getError    error
	createError error
	deleteError error
}

func (fc *fakeClient) Get(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
	return fc.getError
}

func (fc *fakeClient) List(ctx context.Context, list runtime.Object, opts ...client.ListOption) error {
	panic("not implemented")
}

func (fc *fakeClient) Create(ctx context.Context, obj runtime.Object, opts ...client.CreateOption) error {
	return fc.createError
}

func (fc *fakeClient) Delete(ctx context.Context, obj runtime.Object, opts ...client.DeleteOption) error {
	return fc.deleteError
}

func (fc *fakeClient) Update(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
	panic("not implemented")
}

func (fc *fakeClient) Patch(ctx context.Context, obj runtime.Object, patch client.Patch, opts ...client.PatchOption) error {
	panic("not implemented")
}

func (fc *fakeClient) DeleteAllOf(ctx context.Context, obj runtime.Object, opts ...client.DeleteAllOfOption) error {
	panic("not implemented")
}

func (fc *fakeClient) Status() client.StatusWriter {
	return &fakeStatusWriter{}
}
