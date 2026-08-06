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

package external

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	toolscache "k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/internal/contract"
)

// FIXME: move to its own package? => drop DynamicCacheOptions => DynamicCache prefix
// FIXME: make DynamicCache an interface and maybe call it DynamicCacheClient
// FIXME: produce errors in error cases: e.g. list type not configured, not found cacheoptions id

type ObjectType string

func NewDynamicCache(mgr manager.Manager, c client.Client, dynamicCacheOptions map[ObjectType]DynamicCacheOptions, watchNamespace, controllerName string) *DynamicCache {
	return &DynamicCache{
		Manager:             mgr,
		Client:              c,
		dynamicCacheOptions: dynamicCacheOptions,
		WatchNamespace:      watchNamespace,
		ControllerName:      controllerName,
		caches:              map[schema.GroupVersionKind]*cacheEntry{},
		writers:             map[schema.GroupVersionKind]WriterWithScheme{},
	}
}

// DynamicCache is a helper struct to deal when watching external unstructured objects.
// FIXME: rethink name, API and location of this util, maybe introduce New constructur / interface etc.
type DynamicCache struct {
	cachesLock sync.RWMutex
	caches     map[schema.GroupVersionKind]*cacheEntry

	writersLock sync.RWMutex
	writers     map[schema.GroupVersionKind]WriterWithScheme

	watches sync.Map // FIXME: this is very likely to global

	Client              client.Client
	objectTypes         map[client.Object]client.ObjectList
	dynamicCacheOptions map[ObjectType]DynamicCacheOptions
	Manager             manager.Manager
	WatchNamespace      string
	ControllerName      string
}

// DynamicCacheOptions are the cache options for the caches that are created per cluster.
type DynamicCacheOptions struct {
	Transform       toolscache.TransformFunc
	Label           labels.Selector
	ContractObj     map[string]client.Object
	ContractObjList map[string]client.ObjectList
}

type cacheEntry struct {
	Scheme *runtime.Scheme // FIXME: drop this if nobody uses it (like atm)
	Cache  cache.Cache
}

func ToUnstructured(gvk schema.GroupVersionKind, obj client.Object) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(gvk)
	u.SetNamespace(obj.GetNamespace())
	u.SetName(obj.GetName())
	return u
}

type WriterWithScheme interface {
	// Writer knows how to create, delete, and update Kubernetes objects.
	client.Writer

	// Scheme returns the scheme this client is using.
	Scheme() *runtime.Scheme
}

// GetWriter TODO
func (o *DynamicCache) GetWriter(ctx context.Context, gvk schema.GroupVersionKind, obj client.Object) (WriterWithScheme, error) {
	// Create writer if necessary.
	w, err := o.getOrCreateClient(ctx, gvk, obj)
	if err != nil {
		return nil, err
	}

	return w, nil
}

// GetUnstructured TODO
func (o *DynamicCache) GetUnstructured(ctx context.Context, ref clusterv1.ContractVersionedObjectReference, namespace string, objectType ObjectType) (*unstructured.Unstructured, error) {
	if !ref.IsDefined() {
		return nil, errors.Errorf("cannot get object - object reference not set")
	}

	gvk, err := GetGVKFromContractVersionedRef(ctx, o.Client, schema.GroupKind{Group: ref.APIGroup, Kind: ref.Kind})
	if err != nil {
		return nil, err
	}

	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(gvk)
	uList := &unstructured.UnstructuredList{}
	uList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   gvk.Group,
		Version: gvk.Version,
		Kind:    gvk.Kind + "List",
	})

	// Create cache if necessary.
	ce, err := o.getOrCreateCache(ctx, gvk, u, uList, objectType)
	if err != nil {
		return nil, err
	}

	// Get object.
	//FIXME: what about initial informer sync?
	if err := ce.Cache.Get(ctx, client.ObjectKey{Namespace: namespace, Name: ref.Name}, u); err != nil {
		return nil, err
	}
	return u, nil
}

// GetContractObject TODO
func (o *DynamicCache) GetContractObject(ctx context.Context, ref clusterv1.ContractVersionedObjectReference, namespace string, objectType ObjectType) (schema.GroupVersionKind, client.Object, error) {
	if !ref.IsDefined() {
		return schema.GroupVersionKind{}, nil, errors.Errorf("cannot get object - object reference not set")
	}

	gvk, err := GetGVKFromContractVersionedRef(ctx, o.Client, schema.GroupKind{Group: ref.APIGroup, Kind: ref.Kind})
	if err != nil {
		return schema.GroupVersionKind{}, nil, err
	}

	dynamicCacheOptions, ok := o.dynamicCacheOptions[objectType]
	if !ok {
		return schema.GroupVersionKind{}, nil, fmt.Errorf("unknown objectType: %s", objectType)
	}

	// Determine contract version used by the InfraMachine.
	contractVersion, err := contract.GetContractVersion(ctx, o.Client, schema.GroupKind{Group: ref.APIGroup, Kind: ref.Kind})
	if err != nil {
		return schema.GroupVersionKind{}, nil, err
	}

	obj, ok := dynamicCacheOptions.ContractObj[contractVersion]
	if !ok {
		return schema.GroupVersionKind{}, nil, fmt.Errorf("no Object for objectType: %s contract: %s", objectType, contractVersion)
	}
	objList, ok := dynamicCacheOptions.ContractObjList[contractVersion]
	if !ok {
		return schema.GroupVersionKind{}, nil, fmt.Errorf("no ObjectList for objectType: %s contract: %s", objectType, contractVersion)
	}

	// Create cache if necessary.
	ce, err := o.getOrCreateCache(ctx, gvk, obj, objList, objectType)
	if err != nil {
		return schema.GroupVersionKind{}, nil, err
	}

	// Get object.
	//FIXME: what about initial informer sync?
	obj = obj.DeepCopyObject().(client.Object)
	if err := ce.Cache.Get(ctx, client.ObjectKey{Namespace: namespace, Name: ref.Name}, obj); err != nil {
		return schema.GroupVersionKind{}, nil, err
	}
	return gvk, obj, nil
}

// ListContractObject TODO
func (o *DynamicCache) ListContractObject(ctx context.Context, gk schema.GroupKind, objectType ObjectType, opts ...client.ListOption) (schema.GroupVersionKind, client.ObjectList, error) {
	if gk.Empty() {
		return schema.GroupVersionKind{}, nil, errors.Errorf("cannot get object - GroupKind is not set")
	}

	gvk, err := GetGVKFromContractVersionedRef(ctx, o.Client, gk)
	if err != nil {
		return schema.GroupVersionKind{}, nil, err
	}

	dynamicCacheOptions, ok := o.dynamicCacheOptions[objectType]
	if !ok {
		return schema.GroupVersionKind{}, nil, fmt.Errorf("unknown objectType: %s", objectType)
	}

	// Determine contract version used by the InfraMachine.
	contractVersion, err := contract.GetContractVersion(ctx, o.Client, schema.GroupKind{Group: gk.Group, Kind: gk.Kind})
	if err != nil {
		return schema.GroupVersionKind{}, nil, err
	}

	obj, ok := dynamicCacheOptions.ContractObj[contractVersion]
	if !ok {
		return schema.GroupVersionKind{}, nil, fmt.Errorf("no Object for objectType: %s contract: %s", objectType, contractVersion)
	}
	objList, ok := dynamicCacheOptions.ContractObjList[contractVersion]
	if !ok {
		return schema.GroupVersionKind{}, nil, fmt.Errorf("no ObjectList for objectType: %s contract: %s", objectType, contractVersion)
	}

	// Create cache if necessary.
	ce, err := o.getOrCreateCache(ctx, gvk, obj, objList, objectType)
	if err != nil {
		return schema.GroupVersionKind{}, nil, err
	}

	// Get object.
	//FIXME: what about initial informer sync?
	objList = objList.DeepCopyObject().(client.ObjectList)
	if err := ce.Cache.List(ctx, objList, opts...); err != nil {
		return schema.GroupVersionKind{}, nil, fmt.Errorf("failed to list %s objects: %w", gvk.Kind, err)
	}
	return gvk, objList, nil
}

// Watch uses the controller to issue a Watch only if the object hasn't been seen before.
func (o *DynamicCache) Watch(ctx context.Context, controller controller.Controller, gvk schema.GroupVersionKind, handler handler.EventHandler, objectType ObjectType) error {
	log := ctrl.LoggerFrom(ctx)

	// Determine contract version used by the InfraMachine.
	contractVersion, err := contract.GetContractVersion(ctx, o.Client, gvk.GroupKind())
	if err != nil {
		return err
	}

	dynamicCacheOptions, ok := o.dynamicCacheOptions[objectType]
	if !ok {
		return fmt.Errorf("unknown objectType: %s", objectType)
	}

	obj, ok := dynamicCacheOptions.ContractObj[contractVersion]
	if !ok {
		return fmt.Errorf("no Object for objectType: %s contract: %s", objectType, contractVersion)
	}
	objList, ok := dynamicCacheOptions.ContractObjList[contractVersion]
	if !ok {
		return fmt.Errorf("no ObjectList for objectType: %s contract: %s", objectType, contractVersion)
	}

	// Create cache if necessary.
	ce, err := o.getOrCreateCache(ctx, gvk, obj, objList, objectType) // FIXME: not ideal that we create a cache without options here => think about changing this
	if err != nil {
		return err
	}

	// Add watch if necessary.
	if _, watchAdded := o.watches.LoadOrStore(gvk, struct{}{}); watchAdded { // FIXME: double check if per gvk and not per gk is correct
		return err
	}

	log.Info(fmt.Sprintf("Adding watch on external object %q", gvk.String()))
	err = controller.Watch(source.Kind(
		ce.Cache,
		obj,
		handler,
		// ResourceIsChanged predicate is not needed because Resync is turned off with syncPeriod: 0
	))
	if err != nil {
		o.watches.Delete(gvk)
		return errors.Wrapf(err, "failed to add watch on external object %q", gvk.String())
	}
	return nil
}

func (o *DynamicCache) getOrCreateCache(_ context.Context, gvk schema.GroupVersionKind, obj client.Object, objList client.ObjectList, objectType ObjectType) (*cacheEntry, error) {
	o.cachesLock.RLock()
	ce, exists := o.caches[gvk]
	o.cachesLock.RUnlock()

	if exists {
		return ce, nil
	}

	o.cachesLock.Lock()
	defer o.cachesLock.Unlock()

	// Check again now that we have the write lock.
	ce, exists = o.caches[gvk]
	if exists {
		return ce, nil
	}

	gvkList := schema.GroupVersionKind{
		Group:   gvk.Group,
		Version: gvk.Version,
		Kind:    gvk.Kind + "List",
	}

	var dynamicCacheOptions DynamicCacheOptions
	if objectType != "" {
		dynamicCacheOptions = o.dynamicCacheOptions[objectType]
	}

	scheme := runtime.NewScheme()
	metav1.AddToGroupVersion(scheme, gvk.GroupVersion())

	switch obj.(type) {
	case *unstructured.Unstructured:
		// Nothing to do
	case *metav1.PartialObjectMetadata:
		return nil, fmt.Errorf("PartialObjectMetadata is not supported")
	default:
		scheme.AddKnownTypeWithName(gvk, obj.DeepCopyObject())
		scheme.AddKnownTypeWithName(gvkList, objList.DeepCopyObject())
	}

	var watchNamespaces map[string]cache.Config
	if o.WatchNamespace != "" {
		watchNamespaces = map[string]cache.Config{
			o.WatchNamespace: {},
		}
	}

	c, err := cache.New(o.Manager.GetConfig(), cache.Options{
		DefaultNamespaces: watchNamespaces,
		SyncPeriod:        new(time.Duration(0)), // FIXME: verify this turns resync off
		//NewInformer:       capicontrollerutil.NewInformerFunc(scheme, o.ControllerName), // FIXME: can't call this twice, reuse somehow
		Scheme:     scheme,
		Mapper:     o.Manager.GetRESTMapper(),
		HTTPClient: o.Manager.GetHTTPClient(),

		DefaultTransform:     dynamicCacheOptions.Transform,
		DefaultLabelSelector: dynamicCacheOptions.Label,
	})
	if err != nil {
		return nil, err
	}

	// FIXME: figure out how to cleanly shutdown the cache during mgr shutdown, maybe we just have to add it ot the manager? (maybe that also takes care of calling start
	// Start the cache!
	go c.Start(context.Background()) //nolint:errcheck // FIXME: figure out which ctx to use

	o.caches[gvk] = &cacheEntry{
		Scheme: scheme,
		Cache:  c,
	}
	return o.caches[gvk], nil
}

func (o *DynamicCache) GetCache(_ context.Context, gvk schema.GroupVersionKind) (cache.Cache, bool) {
	o.cachesLock.RLock()
	defer o.cachesLock.RUnlock()

	ce, exists := o.caches[gvk]
	return ce.Cache, exists
}

func (o *DynamicCache) getOrCreateClient(_ context.Context, gvk schema.GroupVersionKind, obj client.Object) (WriterWithScheme, error) {
	o.writersLock.RLock()
	w, exists := o.writers[gvk]
	o.writersLock.RUnlock()

	if exists {
		return w, nil
	}

	o.writersLock.Lock()
	defer o.writersLock.Unlock()

	// Check again now that we have the write lock.
	w, exists = o.writers[gvk]
	if exists {
		return w, nil
	}

	scheme := runtime.NewScheme()
	metav1.AddToGroupVersion(scheme, gvk.GroupVersion())

	switch obj.(type) {
	case *unstructured.Unstructured:
		// Nothing to do
	case *metav1.PartialObjectMetadata:
		return nil, fmt.Errorf("PartialObjectMetadata is not supported")
	default:
		scheme.AddKnownTypeWithName(gvk, obj.DeepCopyObject()) // FIXME: ensure this is a clean empty object (check other cases above)
	}

	writer, err := client.New(o.Manager.GetConfig(), client.Options{
		Scheme:     scheme,
		Mapper:     o.Manager.GetRESTMapper(),
		HTTPClient: o.Manager.GetHTTPClient(),
	})
	if err != nil {
		return nil, errors.Errorf("error creating uncached client: %w", err)
	}
	o.writers[gvk] = writer
	return o.writers[gvk], nil
}
