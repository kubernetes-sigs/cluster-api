/*
Copyright 2021 The Kubernetes Authors.

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

package dryrun

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	localScheme = scheme.Scheme
)

// ClientOperation define a write operation executed by the Client.
type ClientOperation struct {
	gvk schema.GroupVersionKind
	key client.ObjectKey
	msg string
}

// Client implements a dry run Client, that is a fake.Client that logs write operations.
type Client struct {
	fakeClient client.Client
	Log        []ClientOperation
}

// NewClient returns a new dry run Client.
func NewClient(objs []client.Object) *Client {
	fakeClient := fake.NewClientBuilder().WithObjects(objs...).WithScheme(localScheme).Build()

	return &Client{
		fakeClient: fakeClient,
		Log:        nil,
	}
}

// Get retrieves an obj for the given object key from the Kubernetes Cluster.
func (c *Client) Get(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	return c.fakeClient.Get(ctx, key, obj)
}

// List retrieves list of objects for a given namespace and list options.
func (c *Client) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return c.fakeClient.List(ctx, list, opts...)
}

// Create saves the object obj in the Kubernetes cluster.
func (c *Client) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	err := c.fakeClient.Create(ctx, obj, opts...)
	if err == nil {
		c.Log = append(c.Log, ClientOperation{
			gvk: obj.GetObjectKind().GroupVersionKind(),
			key: client.ObjectKeyFromObject(obj),
			msg: "Create",
		})
	}
	return err
}

// Delete deletes the given obj from Kubernetes cluster.
func (c *Client) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	err := c.fakeClient.Delete(ctx, obj, opts...)
	if err == nil {
		c.Log = append(c.Log, ClientOperation{
			gvk: obj.GetObjectKind().GroupVersionKind(),
			key: client.ObjectKeyFromObject(obj),
			msg: "Delete",
		})
	}
	return err
}

// Update updates the given obj in the Kubernetes cluster.
// NOTE: Topology reconciler does not use update, so we are skipping implementation for now.
func (c *Client) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	panic("implement me")
}

// Patch patches the given obj in the Kubernetes cluster.
func (c *Client) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	err := c.fakeClient.Patch(ctx, obj, patch, opts...)
	if err == nil {
		c.Log = append(c.Log, ClientOperation{
			gvk: obj.GetObjectKind().GroupVersionKind(),
			key: client.ObjectKeyFromObject(obj),
			msg: "Patch",
		})
	}
	return err
}

// DeleteAllOf deletes all objects of the given type matching the given options.
// NOTE: Topology reconciler does not use DeleteAllOf, so we are skipping implementation for now.
func (c *Client) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	panic("implement me")
}

// Status returns a client which can update status subresource for kubernetes objects.
// NOTE: Topology reconciler does not use Status, so we are skipping implementation for now.
func (c *Client) Status() client.StatusWriter {
	panic("implement me")
}

// Scheme returns the scheme this client is using.
func (c *Client) Scheme() *runtime.Scheme {
	return c.fakeClient.Scheme()
}

// RESTMapper returns the rest this client is using.
func (c *Client) RESTMapper() meta.RESTMapper {
	return c.fakeClient.RESTMapper()
}

// ListModified returns a list of objects modified with this client.
// NOTE: objects are returned only once, no matter of many operations applied.
func (c *Client) ListModified(ctx context.Context) ([]unstructured.Unstructured, error) {
	objs := make([]unstructured.Unstructured, 0, len(c.Log))
	keys := map[string]bool{}
	for _, l := range c.Log {
		key := fmt.Sprintf("%s-%s", l.gvk.String(), l.key.String())
		if keys[key] {
			continue
		}
		keys[key] = true

		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(l.gvk)
		obj.SetNamespace(l.key.Namespace)
		obj.SetName(l.key.Name)
		if err := c.fakeClient.Get(ctx, l.key, obj); err != nil {
			return nil, errors.Wrapf(err, "failed to read modified object")
		}
		objs = append(objs, *obj)
	}
	return objs, nil
}
