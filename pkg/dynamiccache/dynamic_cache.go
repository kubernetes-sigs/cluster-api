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
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlcache "sigs.k8s.io/controller-runtime/pkg/cache"
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
	Transform toolscache.TransformFunc
	Label     labels.Selector

	// IsUnstructured defines if this object type should be cached and returned as Unstructured
	// or via the types from TypesByContract.
	//
	// Note: This field must be set.
	//
	// If IsUnstructured is true:
	// - TypesByContract must not be set.
	// - Only GetUnstructured can be used with this object type.
	// - The cache/informer will be created with Unstructured.
	// If IsUnstructured is false:
	// - TypesByContract must be set.
	// - Only GetContractObject, ListContractObjects and Watch can be used with this object type
	// - The cache/informer will be created with types from TypesByContract.
	IsUnstructured *bool

	// TypesByContract defines which types to use by contract version.
	// Note: This must be set only if IsUnstructured is false.
	TypesByContract map[string]TypesByContract
}

// TypesByContract defines which types to use by contract version.
type TypesByContract struct {
	// Obj defines which type to use.
	Obj client.Object
	// ObjList defines which list type to use.
	ObjList client.ObjectList
}

// DynamicCache dynamically creates caches when one of the Get, List or Watch methods is called
// and makes them available for read, write and watch requests.
// The cache configuration is determined based on the ObjectType passed into these methods.
// DynamicCache will create at most one cache per GroupVersionKind to avoid wasting memory.
// Note: It is also possible to have multiple caches for the same ObjectType if they have different
// GroupVersionKinds.
//
// When Unstructured is used it is possible:
//   - to customize the cache configuration via ByObjectTypeOptions when calling dynmiccache.New
//     Note: For GVKs that are not known at that time this is not possible with the regular cache through ctrl.NewManager
//
// When a contract object is used it is possible:
//   - to customize the cache configuration via ByObjectTypeOptions when calling dynmiccache.New
//     Note: For GVKs that are not known at that time this is not possible with the regular cache through ctrl.NewManager
//   - to only cache a subset of the fields of an object (the fields that exist in the contract object)
//
// Some example use cases:
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
	// GetUnstructured gets an object based on the given GroupKind, ObjectKey and ObjectType as Unstructured.
	// It will automatically look up the apiVersion based on supported contracts.
	// It also creates a Cache if it does not exist already and uses the cache configuration configured
	// for this ObjectType.
	GetUnstructured(ctx context.Context, objType ObjectType, objGK schema.GroupKind, objKey client.ObjectKey) (obj *unstructured.Unstructured, err error)

	// GetContractObject gets an object based on the given GroupKind, ObjectKey and ObjectType.
	// It will automatically look up the apiVersion based on supported contracts.
	// Then it will determine the right Go types to use based on the ObjectType and contract.
	// It also creates a Cache if it does not exist already and uses the cache configuration configured
	// for this ObjectType.
	GetContractObject(ctx context.Context, objType ObjectType, objGK schema.GroupKind, objKey client.ObjectKey) (objGVK schema.GroupVersionKind, obj client.Object, err error)

	// ListContractObjects lists objects based on the given GroupKind, ObjectKey and ObjectType.
	// It will automatically look up the apiVersion based on supported contracts.
	// Then it will determine the right Go types to use based on the ObjectType and contract.
	// It also creates a Cache if it does not exist already and uses the cache configuration configured
	// for this ObjectType.
	ListContractObjects(ctx context.Context, objType ObjectType, objGK schema.GroupKind, opts ...client.ListOption) (objGVK schema.GroupVersionKind, objList client.ObjectList, err error)

	// Watch adds a watch to the given watcher, if not done already.
	// It will automatically look up the apiVersion based on supported contracts.
	// Then it will determine the right Go types to use based on the ObjectType and contract.
	// It also creates a Cache if it does not exist already and uses the cache configuration configured
	// for this ObjectType.
	// Then it will call Watch on the watcher to subscribe the Watcher to events from this Cache.
	// The watch is only added once per GroupVersionKind and watcherName.
	Watch(ctx context.Context, watcherName string, watcher SourceWatcher, objType ObjectType, objGK schema.GroupKind, handler handler.EventHandler) error

	// GetCache returns a Cache for the given GroupVersionKind.
	GetCache(ctx context.Context, objGVK schema.GroupVersionKind) (ctrlcache.Cache, bool)

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
func New(mgr manager.Manager, byObjectTypeOptions map[ObjectType]ByObjectTypeOptions, controllerName, watchNamespace string) (DynamicCache, error) {
	informerName, err := toolscache.NewInformerName(controllerName + "-dynamic-cache")
	if err != nil {
		return nil, errors.New("cache.NewInformerName was called twice with the same name, that should never happen")
	}

	for _, byObjectType := range byObjectTypeOptions {
		if byObjectType.IsUnstructured == nil {
			return nil, errors.New("byObjectTypeOptions.IsUnstructured must be set")
		}
		if ptr.Deref(byObjectType.IsUnstructured, false) {
			if len(byObjectType.TypesByContract) > 0 {
				return nil, errors.New("if byObjectTypeOptions.IsUnstructured is true, byObjectTypeOptions.TypesByContract must not be set")
			}
		} else {
			if len(byObjectType.TypesByContract) == 0 {
				return nil, errors.New("if byObjectTypeOptions.IsUnstructured is false, byObjectTypeOptions.TypesByContract must be set")
			}
		}
	}

	return &dynamicCache{
		manager:             mgr,
		client:              mgr.GetClient(),
		watchNamespace:      watchNamespace,
		informerName:        informerName,
		byObjectTypeOptions: byObjectTypeOptions,
		caches:              map[schema.GroupVersionKind]*cacheEntry{},
	}, nil
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
	Cache  ctrlcache.Cache

	// objType and objListType are the Go types that were used to create this cache entry.
	// They are used to detect and fail on subsequent calls for the same GroupVersionKind that
	// use a different Go type, given that a single cache can only serve one Go type per GroupVersionKind.
	// We intentionally want to block cases where we would have multiple caches for the same GroupVersionKind
	// as that would waste memory.
	objType     reflect.Type
	objListType reflect.Type
}

// checkTypes returns an error if objType or objListType don't match the Go types that were
// used to create this cacheEntry. Accordingly, this method acts as a safeguard that we don't use a cache
// with the wrong types to serve requests (e.g. use a cache with contract types to serve Unstructured requests).
// Through these errors we should be able to avoid using GetUnstructured and GetContract* for the same GroupVersionKind.
func (ce *cacheEntry) checkTypes(objGVK schema.GroupVersionKind, objType, objListType reflect.Type) error {
	if ce.objType != objType || ce.objListType != objListType {
		return fmt.Errorf("failed to get cache for %s: cache already exists for types %s/%s, cannot use it for types %s/%s",
			objGVK.Kind, ce.objType, ce.objListType, objType, objListType)
	}
	return nil
}

func (dc *dynamicCache) GetUnstructured(ctx context.Context, objType ObjectType, objGK schema.GroupKind, objKey client.ObjectKey) (*unstructured.Unstructured, error) {
	_, objGVK, err := contract.GetGVKFromGK(ctx, dc.client, objGK)
	if err != nil {
		return nil, fmt.Errorf("failed to get Unstructured %s %s: %w", objGK.Kind, klog.KRef(objKey.Namespace, objKey.Name), err)
	}

	dynamicCacheOpts, ok := dc.byObjectTypeOptions[objType]
	if !ok {
		return nil, fmt.Errorf("objectType %s is not configured in the DynamicCache", objType)
	}
	if !ptr.Deref(dynamicCacheOpts.IsUnstructured, false) {
		return nil, fmt.Errorf("objectType %s is configured with IsUnstructured: false, but request is for an Unstructured", objType)
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

func (dc *dynamicCache) GetContractObject(ctx context.Context, objType ObjectType, objGK schema.GroupKind, objKey client.ObjectKey) (schema.GroupVersionKind, client.Object, error) {
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

func (dc *dynamicCache) ListContractObjects(ctx context.Context, objType ObjectType, objGK schema.GroupKind, opts ...client.ListOption) (schema.GroupVersionKind, client.ObjectList, error) {
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

func (dc *dynamicCache) Watch(ctx context.Context, watcherName string, watcher SourceWatcher, objType ObjectType, objGK schema.GroupKind, handler handler.EventHandler) error {
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

func (dc *dynamicCache) GetCache(_ context.Context, objGVK schema.GroupVersionKind) (ctrlcache.Cache, bool) {
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

	if ptr.Deref(dynamicCacheOpts.IsUnstructured, false) {
		return nil, nil, fmt.Errorf("objectType %s is configured with IsUnstructured: true, but request is for a contract object", objType)
	}

	types, ok := dynamicCacheOpts.TypesByContract[contractVersion]
	if !ok {
		return nil, nil, fmt.Errorf("objectType %s does not have types configured for contract %s in the DynamicCache", objType, contractVersion)
	}

	return types.Obj, types.ObjList, nil
}

func (dc *dynamicCache) getOrCreateCache(ctx context.Context, objGVK schema.GroupVersionKind, obj client.Object, objList client.ObjectList, objectType ObjectType) (*cacheEntry, error) {
	if objectType == "" {
		return nil, fmt.Errorf("failed to create cache for %s: objectType is not set", objGVK.Kind)
	}
	dynamicCacheOpts, ok := dc.byObjectTypeOptions[objectType]
	if !ok {
		return nil, fmt.Errorf("failed to create cache for %s: objectType %s is not configured in the DynamicCache", objGVK.Kind, objectType)
	}

	objType := reflect.TypeOf(obj)
	objListType := reflect.TypeOf(objList)

	dc.cachesLock.RLock()
	ce, exists := dc.caches[objGVK]
	dc.cachesLock.RUnlock()

	if exists {
		if err := ce.checkTypes(objGVK, objType, objListType); err != nil {
			return nil, err
		}
		return ce, nil
	}

	dc.cachesLock.Lock()
	defer dc.cachesLock.Unlock()

	// Check again now that we have the write lock.
	ce, exists = dc.caches[objGVK]
	if exists {
		if err := ce.checkTypes(objGVK, objType, objListType); err != nil {
			return nil, err
		}
		return ce, nil
	}

	objGVKList := schema.GroupVersionKind{
		Group:   objGVK.Group,
		Version: objGVK.Version,
		Kind:    objGVK.Kind + "List",
	}

	// Note: Every cache has to use its own scheme because it needs to contain the correct GVK <=> type mapping
	// (one type cannot map to multiple GKVs). It's also not possible to mutate schemes later as we would otherwise
	// get concurrent map access errors.
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

	var watchNamespaces map[string]ctrlcache.Config
	if dc.watchNamespace != "" {
		watchNamespaces = map[string]ctrlcache.Config{
			dc.watchNamespace: {},
		}
	}
	c, err := ctrlcache.New(dc.manager.GetConfig(), ctrlcache.Options{
		DefaultNamespaces: watchNamespaces,
		// Turn off resync.
		// If we would not do this we would get additional resyncs for all objects that a controller watches,
		// but it's enough that a controller gets a resync for its primary object, we don't need multiple resyncs
		// that would just waste resources.
		SyncPeriod:  new(time.Duration(0)),
		NewInformer: capicontrollerutil.NewInformerFunc(scheme, dc.informerName),
		Scheme:      scheme,
		Mapper:      dc.manager.GetRESTMapper(),
		HTTPClient:  dc.manager.GetHTTPClient(),
		ByObject: map[client.Object]ctrlcache.ByObject{
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
		Writer:      writer,
		Cache:       c,
		objType:     objType,
		objListType: objListType,
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
	ctrlcache.Cache
}

func (cc *cacheRunnable) GetCache() ctrlcache.Cache {
	return cc.Cache
}
