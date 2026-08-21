/*
Copyright The Kubernetes Authors.

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

package dynamiccache

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	toolscache "k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"sigs.k8s.io/cluster-api/internal/contract"
	capicontrollerutil "sigs.k8s.io/cluster-api/util/controller"
)

// ObjectType identifies the type of an object.
type ObjectType string

// ByObjectTypeOptions configures how the DynamicCache should handle a specific ObjectType.
type ByObjectTypeOptions struct {
	Transform       toolscache.TransformFunc
	Label           labels.Selector
	ContractObj     map[string]client.Object
	ContractObjList map[string]client.ObjectList
}

// DynamicCache manages caches per GroupVersionKind and makes them available for read, write and watch requests.
// DynamicCache can be used in the following cases:
//   - Dynamic cache configuration: controller-runtime only allows to configure the regular Cache once during manager
//     creation. But e.g. if we want to configure the cache for InfraMachines we don't have the GVK that is needed
//     for that configuration available at that time.
//   - When using typed Go API types based on the contract: the regular Cache in controller-runtime needs all Go API types
//     registered at startup because the scheme cannot be mutated later without concurrent map access errors. In
//     this scenario we are using BootstrapConfig / InfraMachine Go API types that have been modeled after the contract.
//     They will be used e.g. in the Machine controller and we have to create informers for them when we encounter the
//     first Machine. Other Machines might use different infra / bootstrap providers which will then require additional
//     informers with the same Go API types but different GroupVersionKind.
type DynamicCache interface {
	// GetUnstructured gets an Unstructured based on the given GroupKind, ObjectKey and ObjectType.
	// It will automatically look up the apiVersion based on supported contracts.
	// It also creates a Cache if it does not exist already and uses the cache configuration configured
	// for this ObjectType.
	GetUnstructured(ctx context.Context, objGK schema.GroupKind, objKey client.ObjectKey, objType ObjectType) (obj *unstructured.Unstructured, err error)

	// GetContractObject gets an object based on the given GroupKind, ObjectKey and ObjectType.
	// It will automatically look up the apiVersion based on supported contracts.
	// Then it will determine the right Go types to use based on the ObjectType and contract.
	// It also creates a Cache if it does not exist already and uses the cache configuration configured
	// for this ObjectType.
	GetContractObject(ctx context.Context, objGK schema.GroupKind, objKey client.ObjectKey, objType ObjectType) (objGVK schema.GroupVersionKind, obj client.Object, err error)

	// ListContractObjects lists objects based on the given GroupKind, ObjectKey and ObjectType.
	// It will automatically look up the apiVersion based on supported contracts.
	// Then it will determine the right Go types to use based on the ObjectType and contract.
	// It also creates a Cache if it does not exist already and uses the cache configuration configured
	// for this ObjectType.
	ListContractObjects(ctx context.Context, objGK schema.GroupKind, objType ObjectType, opts ...client.ListOption) (objGVK schema.GroupVersionKind, objList client.ObjectList, err error)

	// Watch adds a watch to the given watcher, if not done already.
	// It will automatically look up the apiVersion based on supported contracts.
	// Then it will determine the right Go types to use based on the ObjectType and contract.
	// It also creates a Cache if it does not exist already and uses the cache configuration configured
	// for this ObjectType.
	// Then it will call Watch on the watcher to subscribe the Watcher to events from this Cache.
	// The watch is only added once per GroupVersionKind and watcherName.
	Watch(ctx context.Context, watcherName string, watcher SourceWatcher, objGK schema.GroupKind, objType ObjectType, handler handler.EventHandler) error

	// GetCache returns a Cache for the given GroupVersionKind.
	GetCache(ctx context.Context, objGVK schema.GroupVersionKind) (cache.Cache, bool)

	// GetWriter returns a WriterWithScheme for the given GroupVersionKind.
	// The writer will have the necessary API type added to its scheme.
	GetWriter(ctx context.Context, objGVK schema.GroupVersionKind) (WriterWithScheme, bool)
}

// WriterWithScheme is a client.Writer that also exposes its Scheme.
type WriterWithScheme interface {
	// Writer knows how to create, delete, and update Kubernetes objects.
	client.Writer

	// Scheme returns the scheme this client is using.
	Scheme() *runtime.Scheme
}

// SourceWatcher is a scoped-down interface from Controller that only has the Watch func.
type SourceWatcher interface {
	Watch(src source.TypedSource[reconcile.Request]) error
}

// New creates a new DynamicCache.
// This func takes per ObjectType options for how to handle specific ObjectTypes.
// This configuration is then used in methods like GetUnstructured, GetContractObject, ... to determine which
// Go types and which cache configuration should be used.
func New(mgr manager.Manager, c client.Client, byObjectTypeOptions map[ObjectType]ByObjectTypeOptions, controllerName, watchNamespace string) DynamicCache {
	informerName, err := toolscache.NewInformerName(controllerName + "-dynamic-cache")
	if err != nil {
		panic("cache.NewInformerName was called twice with the same name, that should never happen")
	}

	return &dynamicCache{
		manager:             mgr,
		client:              c,
		watchNamespace:      watchNamespace,
		informerName:        informerName,
		byObjectTypeOptions: byObjectTypeOptions,
		caches:              map[schema.GroupVersionKind]*cacheEntry{},
	}
}

var _ DynamicCache = &dynamicCache{}

type dynamicCache struct {
	manager manager.Manager
	client  client.Client

	watchNamespace      string
	informerName        *toolscache.InformerName
	byObjectTypeOptions map[ObjectType]ByObjectTypeOptions

	cachesLock sync.RWMutex
	caches     map[schema.GroupVersionKind]*cacheEntry

	watches sync.Map
}

type cacheEntry struct {
	Writer WriterWithScheme
	Cache  cache.Cache

	// objGoType and objListGoType are the Go types that were used to create this cache entry.
	// They are used to detect and fail on subsequent calls for the same GroupVersionKind that
	// use a different Go type, given that a single cache can only serve one Go type per GroupVersionKind.
	// We intentionally want to block cases where we would have multiple caches for the same GroupVersionKind
	objGoType     reflect.Type
	objListGoType reflect.Type
}

// checkGoTypes returns an error if objGoType or objListGoType don't match the Go types that were
// used to create this cacheEntry.
func (ce *cacheEntry) checkGoTypes(objGVK schema.GroupVersionKind, objGoType, objListGoType reflect.Type) error {
	if ce.objGoType != objGoType || ce.objListGoType != objListGoType {
		return fmt.Errorf("failed to get cache for %s: cache already exists for Go types %s/%s, cannot use it for Go types %s/%s",
			objGVK.Kind, ce.objGoType, ce.objListGoType, objGoType, objListGoType)
	}
	return nil
}

func (dc *dynamicCache) GetUnstructured(ctx context.Context, objGK schema.GroupKind, objKey client.ObjectKey, objType ObjectType) (*unstructured.Unstructured, error) {
	_, objGVK, err := contract.GetGVKFromGK(ctx, dc.client, objGK)
	if err != nil {
		return nil, fmt.Errorf("failed to get Unstructured %s %s: %w", objGK.Kind, klog.KRef(objKey.Namespace, objKey.Name), err)
	}

	// Construct obj and objList.
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(objGVK)
	objList := &unstructured.UnstructuredList{}
	objList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   objGVK.Group,
		Version: objGVK.Version,
		Kind:    objGVK.Kind + "List",
	})

	// Create cache if necessary.
	ce, err := dc.getOrCreateCache(ctx, objGVK, obj, objList, objType)
	if err != nil {
		return nil, fmt.Errorf("failed to get Unstructured %s %s: %w", objGK.Kind, klog.KRef(objKey.Namespace, objKey.Name), err)
	}

	// Get object.
	// Note: If the informer is not synced in time this will return an error like:
	// "Timeout: failed waiting for ... Informer to sync"
	if err := ce.Cache.Get(ctx, objKey, obj); err != nil {
		return nil, fmt.Errorf("failed to get Unstructured %s %s: %w", objGK.Kind, klog.KRef(objKey.Namespace, objKey.Name), err)
	}
	return obj, nil
}

func (dc *dynamicCache) GetContractObject(ctx context.Context, objGK schema.GroupKind, objKey client.ObjectKey, objType ObjectType) (schema.GroupVersionKind, client.Object, error) {
	contractVersion, objGVK, err := contract.GetGVKFromGK(ctx, dc.client, objGK)
	if err != nil {
		return schema.GroupVersionKind{}, nil, fmt.Errorf("failed to get %s %s: %w", objGK.Kind, klog.KRef(objKey.Namespace, objKey.Name), err)
	}

	// Construct obj and objList.
	obj, objList, err := dc.getObjTypesFromOptions(objType, contractVersion)
	if err != nil {
		return schema.GroupVersionKind{}, nil, fmt.Errorf("failed to get %s %s: %w", objGK.Kind, klog.KRef(objKey.Namespace, objKey.Name), err)
	}

	// Create cache if necessary.
	ce, err := dc.getOrCreateCache(ctx, objGVK, obj, objList, objType)
	if err != nil {
		return schema.GroupVersionKind{}, nil, fmt.Errorf("failed to get %s %s: %w", objGK.Kind, klog.KRef(objKey.Namespace, objKey.Name), err)
	}

	// Get object.
	// Note: If the informer is not synced in time this will return an error like:
	// "Timeout: failed waiting for ... Informer to sync"
	obj = obj.DeepCopyObject().(client.Object)
	if err := ce.Cache.Get(ctx, objKey, obj); err != nil {
		return schema.GroupVersionKind{}, nil, fmt.Errorf("failed to get %s %s: %w", objGK.Kind, klog.KRef(objKey.Namespace, objKey.Name), err)
	}
	return objGVK, obj, nil
}

func (dc *dynamicCache) ListContractObjects(ctx context.Context, objGK schema.GroupKind, objType ObjectType, opts ...client.ListOption) (schema.GroupVersionKind, client.ObjectList, error) {
	contractVersion, objGVK, err := contract.GetGVKFromGK(ctx, dc.client, objGK)
	if err != nil {
		return schema.GroupVersionKind{}, nil, fmt.Errorf("failed to list %s: %w", objGK.Kind, err)
	}

	// Construct obj and objList.
	obj, objList, err := dc.getObjTypesFromOptions(objType, contractVersion)
	if err != nil {
		return schema.GroupVersionKind{}, nil, fmt.Errorf("failed to list %s: %w", objGK.Kind, err)
	}

	// Create cache if necessary.
	ce, err := dc.getOrCreateCache(ctx, objGVK, obj, objList, objType)
	if err != nil {
		return schema.GroupVersionKind{}, nil, fmt.Errorf("failed to list %s: %w", objGK.Kind, err)
	}

	// List objects.
	// Note: If the informer is not synced in time this will return an error like:
	// "Timeout: failed waiting for ... Informer to sync"
	objList = objList.DeepCopyObject().(client.ObjectList)
	if err := ce.Cache.List(ctx, objList, opts...); err != nil {
		return schema.GroupVersionKind{}, nil, fmt.Errorf("failed to list %s: %w", objGK.Kind, err)
	}
	return objGVK, objList, nil
}

func (dc *dynamicCache) Watch(ctx context.Context, watcherName string, watcher SourceWatcher, objGK schema.GroupKind, objType ObjectType, handler handler.EventHandler) error {
	log := ctrl.LoggerFrom(ctx)

	contractVersion, objGVK, err := contract.GetGVKFromGK(ctx, dc.client, objGK)
	if err != nil {
		return fmt.Errorf("failed to add %s watch for watcher: %s: %w", objGK.Kind, watcherName, err)
	}

	watchName := fmt.Sprintf("%s-%s", watcherName, objGVK)

	// Construct obj and objList.
	obj, objList, err := dc.getObjTypesFromOptions(objType, contractVersion)
	if err != nil {
		return fmt.Errorf("failed to add %s watch for watcher: %s: %w", objGK.Kind, watcherName, err)
	}

	// Create cache if necessary.
	ce, err := dc.getOrCreateCache(ctx, objGVK, obj, objList, objType)
	if err != nil {
		return fmt.Errorf("failed to add %s watch for watcher: %s: %w", objGK.Kind, watcherName, err)
	}

	// Add watch if necessary.
	if _, loaded := dc.watches.LoadOrStore(watchName, struct{}{}); loaded {
		return nil
	}

	// ResourceIsChanged predicate is not needed because Resync is turned off with syncPeriod: 0.
	log.Info(fmt.Sprintf("Creating %s watch for watcher: %s", objGVK.Kind, watcherName))
	if err = watcher.Watch(source.Kind(ce.Cache, obj, handler)); err != nil {
		dc.watches.Delete(watchName)
		return fmt.Errorf("failed to add %s watch for watcher: %s: %w", objGK.Kind, watcherName, err)
	}
	return nil
}

func (dc *dynamicCache) GetCache(_ context.Context, objGVK schema.GroupVersionKind) (cache.Cache, bool) {
	dc.cachesLock.RLock()
	defer dc.cachesLock.RUnlock()

	ce, exists := dc.caches[objGVK]
	if !exists {
		return nil, false
	}
	return ce.Cache, true
}

func (dc *dynamicCache) GetWriter(_ context.Context, objGVK schema.GroupVersionKind) (WriterWithScheme, bool) {
	dc.cachesLock.RLock()
	defer dc.cachesLock.RUnlock()

	ce, exists := dc.caches[objGVK]
	if !exists {
		return nil, false
	}
	return ce.Writer, true
}

func (dc *dynamicCache) getObjTypesFromOptions(objType ObjectType, contractVersion string) (client.Object, client.ObjectList, error) {
	dynamicCacheOpts, ok := dc.byObjectTypeOptions[objType]
	if !ok {
		return nil, nil, fmt.Errorf("objectType %s is not configured in the DynamicCache", objType)
	}
	obj, ok := dynamicCacheOpts.ContractObj[contractVersion]
	if !ok {
		return nil, nil, fmt.Errorf("objectType %s does not have a type configured for contract %s in the DynamicCache", objType, contractVersion)
	}
	objList, ok := dynamicCacheOpts.ContractObjList[contractVersion]
	if !ok {
		return nil, nil, fmt.Errorf("objectType %s does not have a list type configured for contract %s in the DynamicCache", objType, contractVersion)
	}
	return obj, objList, nil
}

func (dc *dynamicCache) getOrCreateCache(ctx context.Context, objGVK schema.GroupVersionKind, obj client.Object, objList client.ObjectList, objType ObjectType) (*cacheEntry, error) {
	objGoType := reflect.TypeOf(obj)
	objListGoType := reflect.TypeOf(objList)

	dc.cachesLock.RLock()
	ce, exists := dc.caches[objGVK]
	dc.cachesLock.RUnlock()

	if exists {
		if err := ce.checkGoTypes(objGVK, objGoType, objListGoType); err != nil {
			return nil, err
		}
		return ce, nil
	}

	dc.cachesLock.Lock()
	defer dc.cachesLock.Unlock()

	// Check again now that we have the write lock.
	ce, exists = dc.caches[objGVK]
	if exists {
		if err := ce.checkGoTypes(objGVK, objGoType, objListGoType); err != nil {
			return nil, err
		}
		return ce, nil
	}

	objGVKList := schema.GroupVersionKind{
		Group:   objGVK.Group,
		Version: objGVK.Version,
		Kind:    objGVK.Kind + "List",
	}

	if objType == "" {
		return nil, fmt.Errorf("failed to create cache for %s: objectType is not set", objGVK.Kind)
	}
	dynamicCacheOpts, ok := dc.byObjectTypeOptions[objType]
	if !ok {
		return nil, fmt.Errorf("failed to create cache for %s: objectType %s is not configured in the DynamicCache", objGVK.Kind, objType)
	}

	scheme := runtime.NewScheme()
	metav1.AddToGroupVersion(scheme, objGVK.GroupVersion())

	switch obj.(type) {
	case *unstructured.Unstructured:
		// Nothing to do
	case *metav1.PartialObjectMetadata:
		return nil, fmt.Errorf("failed to create cache for %s: PartialObjectMetadata is not supported", objGVK.Kind)
	default:
		scheme.AddKnownTypeWithName(objGVK, obj)
		scheme.AddKnownTypeWithName(objGVKList, objList)
	}

	writer, err := client.New(dc.manager.GetConfig(), client.Options{
		Scheme:     scheme,
		Mapper:     dc.manager.GetRESTMapper(),
		HTTPClient: dc.manager.GetHTTPClient(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create cache for %s: failed to create writer: %w", objGVK.Kind, err)
	}

	var watchNamespaces map[string]cache.Config
	if dc.watchNamespace != "" {
		watchNamespaces = map[string]cache.Config{
			dc.watchNamespace: {},
		}
	}
	c, err := cache.New(dc.manager.GetConfig(), cache.Options{
		DefaultNamespaces: watchNamespaces,
		SyncPeriod:        new(time.Duration(0)), // Turn off resync.
		NewInformer:       capicontrollerutil.NewInformerFunc(scheme, dc.informerName),
		Scheme:            scheme,
		Mapper:            dc.manager.GetRESTMapper(),
		HTTPClient:        dc.manager.GetHTTPClient(),
		ByObject: map[client.Object]cache.ByObject{
			obj: {
				Transform: dynamicCacheOpts.Transform,
				Label:     dynamicCacheOpts.Label,
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create cache for %s: %w", objGVK.Kind, err)
	}

	// Add the cache to the manager (which also starts it).
	if err := dc.manager.Add(&cacheRunnable{Cache: c}); err != nil {
		return nil, fmt.Errorf("failed to create cache for %s: %w", objGVK.Kind, err)
	}

	// Note: Intentionally adding the cacheEntry before calling WaitForCacheSync to avoid
	// creating duplicate caches if WaitForCacheSync times out.
	dc.caches[objGVK] = &cacheEntry{
		Writer:        writer,
		Cache:         c,
		objGoType:     objGoType,
		objListGoType: objListGoType,
	}

	// Wait until the cache is initially synced.
	// Note: This should never time out because the cache does not have any informers at this point, but
	// we have to wait here so subsequent Get/List calls do not hit ErrCacheNotStarted.
	cacheSyncCtx, cacheSyncCtxCancel := context.WithTimeoutCause(ctx, 10*time.Second, errors.New("initial sync timeout expired"))
	defer cacheSyncCtxCancel()
	if !c.WaitForCacheSync(cacheSyncCtx) {
		return nil, fmt.Errorf("failed to wait for cache for %s to sync: %w", objGVK.Kind, cacheSyncCtx.Err())
	}

	return dc.caches[objGVK], nil
}

// cacheRunnable embeds cache.Cache and implements the manager.hasCache interface.
// This ensures this cache gets stopped at the right phase of the Manager shutdown together
// with other caches in Manager.
type cacheRunnable struct {
	cache.Cache
}

func (cc *cacheRunnable) GetCache() cache.Cache {
	return cc.Cache
}
