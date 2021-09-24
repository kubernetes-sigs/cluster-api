/*
Copyright 2022 The Kubernetes Authors.

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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/scheme"
)

var (
	localScheme = scheme.Scheme
)

// changeTrackerID represents a unique identifier of an object.
type changeTrackerID struct {
	gvk schema.GroupVersionKind
	key client.ObjectKey
}

type operationType string

const (
	// Represents that a new object is created.
	opCreate operationType = "create"

	// Represents that the object is modified.
	// This could be a result of performing Patch or Update or delete and re-create operations on the object.
	opModify operationType = "modify"

	// Represents that the object is deleted.
	opDelete operationType = "delete"
)

// operation represents the final effective operation and the original object (initial state) associated
// with the operation.
type operation struct {
	originalValue client.Object
	operation     operationType
}

// changeTracker is used to track the operations performed on the objects.
// changeTracker flattens all operations performed on the same object and
// only tracks the final effective operation on the object when compared to
// the initial state. Example: If an object is created and later modified
// it is only tracked as created (effective final operation).
//
// While changeTracker tracks the operations on objects using unique object identifiers
// ChangeSummary reports all the operations and the final state of objects. ChangeSummary
// is calculated using changeTracker and fake client.
type changeTracker struct {
	changes map[changeTrackerID]*operation
}

// Client implements a dry run Client, that is a fake.Client that logs write operations.
type Client struct {
	fakeClient client.Client
	apiReader  client.Reader

	changeTracker *changeTracker
}

// PatchSummary defines the patch observed on an object.
type PatchSummary struct {
	// Initial state of the object.
	Before *unstructured.Unstructured
	// Final state of the object.
	After *unstructured.Unstructured
}

// ChangeSummary defines all the changes detected by the Dryrun execution.
// Nb. Only a single operation is reported for each object, flattening operations
// to show difference between the initial and final states.
type ChangeSummary struct {
	// Created is the list of objects that are created during the dry run execution.
	Created []*unstructured.Unstructured

	// Modified is the list of summary of objects that are modified (Updated, Patched and/or deleted and re-created) during the dry run execution.
	Modified []*PatchSummary

	// Deleted is the list of objects that are deleted during the dry run execution.
	Deleted []*unstructured.Unstructured
}

// NewClient returns a new dry run Client.
// A dry run client mocks interactions with an api server using a fake internal object tracker.
// The objects passed will be used to initialize the fake internal object tracker when creating a new dry run client.
// If an apiReader client is passed the dry run client will use it as a fall back client for read operations (Get, List)
// when the objects are not found in the internal object tracker. Typically the apiReader passed would be a reader client
// to a real Kubernetes Cluster.
func NewClient(apiReader client.Reader, objs []client.Object) *Client {
	fakeClient := fake.NewClientBuilder().WithObjects(objs...).WithScheme(localScheme).Build()
	return &Client{
		fakeClient: fakeClient,
		apiReader:  apiReader,
		changeTracker: &changeTracker{
			changes: map[changeTrackerID]*operation{},
		},
	}
}

// Get retrieves an object for the given object key from the internal object tracker.
// If the object does not exist in the internal object tracker it tries to fetch the object
// from the Kubernetes Cluster using the apiReader client (if apiReader is not nil).
func (c *Client) Get(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	if err := c.fakeClient.Get(ctx, key, obj); err != nil {
		// If the object is not found by the fake client, get the object
		// using the apiReader.
		if apierrors.IsNotFound(err) && c.apiReader != nil {
			return c.apiReader.Get(ctx, key, obj)
		}
		return err
	}
	return nil
}

// List retrieves list of objects for a given namespace and list options.
// List function returns the union of the lists from the internal object tracker and the Kubernetes Cluster.
// Nb. For objects that exist both in the internal object tracker and the Kubernetes Cluster, internal object tracker
// takes precedence.
func (c *Client) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	var gvk schema.GroupVersionKind
	if uList, ok := list.(*unstructured.UnstructuredList); ok {
		gvk = uList.GroupVersionKind()
	} else {
		var err error
		gvk, err = apiutil.GVKForObject(list, c.fakeClient.Scheme())
		if err != nil {
			return errors.Wrap(err, "failed to get GVK of target object")
		}
	}

	// Fetch lists from both fake client and the apiReader and merge the two lists.
	unstructuredFakeList := &unstructured.UnstructuredList{}
	unstructuredFakeList.SetGroupVersionKind(gvk)
	if err := c.fakeClient.List(ctx, unstructuredFakeList, opts...); err != nil {
		return err
	}
	if c.apiReader != nil {
		unstructuredReaderList := &unstructured.UnstructuredList{}
		unstructuredReaderList.SetGroupVersionKind(gvk)
		if err := c.apiReader.List(ctx, unstructuredReaderList, opts...); err != nil {
			return err
		}
		mergeLists(unstructuredFakeList, unstructuredReaderList)
	}

	if err := c.Scheme().Convert(unstructuredFakeList, list, nil); err != nil {
		return errors.Wrapf(err, "failed to convert unstructured list to %T", list)
	}
	return nil
}

// Create saves the object in the internal object tracker.
func (c *Client) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	err := c.fakeClient.Create(ctx, obj, opts...)
	if err == nil {
		id := trackerIDFor(obj)
		// If the object was previously deleted, it is now being re-created. Effectively it is a modify operation
		// on the object.
		if op, ok := c.changeTracker.changes[id]; ok {
			if op.operation == opDelete {
				op.operation = opModify
			}
		} else {
			// This is the first operation on this object. Track the create operation.
			c.changeTracker.changes[id] = &operation{
				operation: opCreate,
			}
		}
	}
	return err
}

// Delete deletes the given obj from internal object tracker.
// Delete will not affect objects in the Kubernetes Cluster.
func (c *Client) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	err := c.fakeClient.Delete(ctx, obj, opts...)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		if c.apiReader == nil {
			return err
		}
		// It is possible that we are trying to delete an object that exists in the Kubernetes Cluster but
		// not in the internal object tracker.
		// In such cases, check if the underlying object exists and if the object does
		// not exist return the original error.
		tmpObj := obj.DeepCopyObject().(client.Object)
		if getErr := c.apiReader.Get(ctx, client.ObjectKeyFromObject(obj), tmpObj); getErr != nil {
			if apierrors.IsNotFound(getErr) {
				// Delete was called on an object that does no exists in the internal object tracker and in the
				// Kubernetes Cluster. Return error.
				// Note: return the original delete error. Not the get error.
				return err
			}
			return errors.Wrap(err, "failed to check if object exists in underlying cluster")
		}
	}
	// If the object is already tracked under a different operation we need to adjust the effective
	// operation using the following rules:
	// - If the object is tracked as created, drop the tracking. Effective operation is object never existed.
	// - If the object is tracked in modified, change to deleted. Effective operation is object is deleted.
	id := trackerIDFor(obj)
	if op, ok := c.changeTracker.changes[id]; ok {
		if op.operation == opCreate {
			delete(c.changeTracker.changes, id)
		}
		if op.operation == opModify {
			op.operation = opDelete
		}
	} else {
		// The object is observed for the first time.
		// Track the delete operation on the object.
		c.changeTracker.changes[id] = &operation{
			originalValue: obj,
			operation:     opDelete,
		}
	}
	return nil
}

// Update updates the given obj in the internal object tracker.
// NOTE: Topology reconciler does not use update, so we are skipping implementation for now.
func (c *Client) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	panic("Update method is not supported by the dryrun client")
}

// Patch patches the given obj in the internal object tracker.
// The patch operation will be tracked if the object does not exist in the internal object tracker but exists in the Kubernetes Cluster.
func (c *Client) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	originalObj := obj.DeepCopyObject().(client.Object)
	// The fake client Patch operation internally makes a Get call. Therefore,
	// create the object if it does not exist in the fake object tracker using the fake client.
	// Note: Because of this operation we will nullify any real errors caused by calling Patch on an object that does no exist.
	// Such cases can only occur because of bugs in reconciler. The dry run operation is not meant to capture bugs in the reconciler
	// hence we choose to ignore such edge cases.
	if err := c.ensureObjInFakeClient(ctx, obj); err != nil {
		return errors.Wrap(err, "failed to ensure object is available in fake object tracker")
	}
	err := c.fakeClient.Patch(ctx, obj, patch, opts...)
	if err == nil {
		id := trackerIDFor(obj)
		// If the object is not already tracked, track the modify operation.
		// If the object is already tracked we don't need to perform any further action because of the following:
		// - Tracked as created - created takes precedence over modified.
		// - Tracked as modified - the object is already tracked with the correct operation.
		// - Tracked as deleted - case not possible. Object cannot be patched after it is deleted.
		if _, ok := c.changeTracker.changes[id]; !ok {
			c.changeTracker.changes[id] = &operation{
				originalValue: originalObj,
				operation:     opModify,
			}
		}
	}
	return err
}

// DeleteAllOf deletes all objects of the given type matching the given options.
// NOTE: Topology reconciler does not use DeleteAllOf, so we are skipping implementation for now.
func (c *Client) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	panic("DeleteAllOf method is not supported by the dryrun client")
}

// Status returns a client which can update the status subresource for Kubernetes objects.
func (c *Client) Status() client.StatusWriter {
	return c.fakeClient.Status()
}

// Scheme returns the scheme this client is using.
func (c *Client) Scheme() *runtime.Scheme {
	return c.fakeClient.Scheme()
}

// RESTMapper returns the rest this client is using.
func (c *Client) RESTMapper() meta.RESTMapper {
	return c.fakeClient.RESTMapper()
}

// Changes generates a summary of all the changes observed from the creation of the dry run client
// to when this function is called.
func (c *Client) Changes(ctx context.Context) (*ChangeSummary, error) {
	changes := &ChangeSummary{
		Created:  []*unstructured.Unstructured{},
		Modified: []*PatchSummary{},
		Deleted:  []*unstructured.Unstructured{},
	}

	for id, op := range c.changeTracker.changes {
		switch op.operation {
		case opCreate:
			obj := &unstructured.Unstructured{}
			obj.SetGroupVersionKind(id.gvk)
			obj.SetNamespace(id.key.Namespace)
			obj.SetName(id.key.Name)
			if err := c.fakeClient.Get(ctx, id.key, obj); err != nil {
				return nil, errors.Wrapf(err, "failed to read created object %s", id.key.String())
			}
			changes.Created = append(changes.Created, obj)
		case opModify:
			// Get the final object.
			after := &unstructured.Unstructured{}
			after.SetGroupVersionKind(id.gvk)
			after.SetNamespace(id.key.Namespace)
			after.SetName(id.key.Name)
			if err := c.fakeClient.Get(ctx, id.key, after); err != nil {
				return nil, errors.Wrapf(err, "failed to read modified object %s", id.key.String())
			}
			// Get the initial object.
			before := &unstructured.Unstructured{}
			if err := c.Scheme().Convert(op.originalValue, before, nil); err != nil {
				return nil, errors.Wrapf(err, "failed to convert %s to unstructured", client.ObjectKeyFromObject(op.originalValue).String())
			}
			changes.Modified = append(changes.Modified, &PatchSummary{
				Before: before,
				After:  after,
			})
		case opDelete:
			obj := &unstructured.Unstructured{}
			if err := c.Scheme().Convert(op.originalValue, obj, nil); err != nil {
				return nil, errors.Wrapf(err, "failed to convert %s to unstructured", client.ObjectKeyFromObject(op.originalValue).String())
			}
			changes.Deleted = append(changes.Deleted, obj)
		default:
			return nil, fmt.Errorf("untracked operation detected")
		}
	}

	return changes, nil
}

// ensureObjInFakeClient makes sure that the object is available in the fake client.
// If the object is not already available it will add it to the fake client by running a "Create"
// operation.
func (c *Client) ensureObjInFakeClient(ctx context.Context, obj client.Object) error {
	o := obj.DeepCopyObject().(client.Object)
	// During create object should not have resourceVersion.
	o.SetResourceVersion("")
	if err := c.fakeClient.Create(ctx, o); err != nil {
		if apierrors.IsAlreadyExists(err) {
			// If the object already exists it is okay for create to fail.
			return nil
		}
		return errors.Wrap(err, "failed to add object to fake object tracker")
	}
	return nil
}

// mergeLists merges the 2 lists a and b by adding every item in b
// that is not in a to list a.
// List a will be merged list.
func mergeLists(a, b *unstructured.UnstructuredList) {
	keyGen := func(u *unstructured.Unstructured) string {
		return fmt.Sprintf("%s-%s", u.GroupVersionKind().String(), client.ObjectKeyFromObject(u).String())
	}
	keys := map[string]bool{}
	// Generate all unique keys for the items in list a.
	for i := range a.Items {
		keys[keyGen(&a.Items[i])] = true
	}
	// For every item in b that is not in a add it to a.
	for i := range b.Items {
		if _, ok := keys[keyGen(&b.Items[i])]; !ok {
			a.Items = append(a.Items, b.Items[i])
		}
	}
}

func trackerIDFor(o client.Object) changeTrackerID {
	return changeTrackerID{
		gvk: o.GetObjectKind().GroupVersionKind(),
		key: client.ObjectKeyFromObject(o),
	}
}
