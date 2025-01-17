/*
Copyright 2023 The Kubernetes Authors.

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

package cache

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// Cache implements an in-memory cache.
type Cache interface {
	Start(ctx context.Context) error

	AddResourceGroup(name string)
	DeleteResourceGroup(name string)

	Get(resourceGroup string, key client.ObjectKey, obj client.Object) error
	List(resourceGroup string, list client.ObjectList, opts ...client.ListOption) error
	Create(resourceGroup string, obj client.Object) error
	Delete(resourceGroup string, obj client.Object) error
	Update(resourceGroup string, obj client.Object) error
	Patch(resourceGroup string, obj client.Object, patch client.Patch) error

	GetInformer(ctx context.Context, obj client.Object) (Informer, error)
	GetInformerForKind(ctx context.Context, gvk schema.GroupVersionKind) (Informer, error)
}

// Informer forwards events to event handlers.
type Informer interface {
	AddEventHandler(handler InformEventHandler) error
	RemoveEventHandler(handler InformEventHandler) error
}

// InformEventHandler handle events originated by a source.
type InformEventHandler interface {
	OnCreate(resourceGroup string, obj client.Object)
	OnUpdate(resourceGroup string, oldObj, newObj client.Object)
	OnDelete(resourceGroup string, obj client.Object)
	OnGeneric(resourceGroup string, obj client.Object)
}

type cache struct {
	scheme *runtime.Scheme

	lock           sync.RWMutex
	resourceGroups map[string]*resourceGroupTracker
	informers      map[schema.GroupVersionKind]Informer

	garbageCollectorRequeueAfter             time.Duration
	garbageCollectorRequeueAfterJitterFactor float64
	garbageCollectorConcurrency              int
	garbageCollectorQueue                    workqueue.TypedRateLimitingInterface[any]

	syncPeriod      time.Duration
	syncConcurrency int
	syncQueue       workqueue.TypedRateLimitingInterface[any]

	started bool
}

type resourceGroupTracker struct {
	lock    sync.RWMutex
	objects map[schema.GroupVersionKind]map[types.NamespacedName]client.Object
	// ownedObjects tracks ownership. Key is the owner, values are the owned objects.
	ownedObjects        map[ownReference]map[ownReference]struct{}
	lastResourceVersion uint64
}

type ownReference struct {
	gvk schema.GroupVersionKind
	key types.NamespacedName
}

func newOwnReferenceFromOwnerReference(namespace string, owner metav1.OwnerReference) (*ownReference, error) {
	gv, err := schema.ParseGroupVersion(owner.APIVersion)
	if err != nil {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("invalid APIVersion in ownerReferences: %s", owner.APIVersion))
	}
	ownerGVK := gv.WithKind(owner.Kind)
	ownerKey := types.NamespacedName{
		// TODO: check if there is something to do for namespaced objects owned by global objects
		Namespace: namespace,
		Name:      owner.Name,
	}
	return &ownReference{gvk: ownerGVK, key: ownerKey}, nil
}

var _ Cache = &cache{}

// NewCache returns a new Cache.
func NewCache(scheme *runtime.Scheme) Cache {
	return &cache{
		scheme:                                   scheme,
		resourceGroups:                           map[string]*resourceGroupTracker{},
		informers:                                map[schema.GroupVersionKind]Informer{},
		garbageCollectorRequeueAfter:             30 * time.Second, // TODO:Expose as option
		garbageCollectorRequeueAfterJitterFactor: 0.3,              // TODO: Expose as option
		garbageCollectorConcurrency:              1,                // TODO: Expose as option
		syncPeriod:                               10 * time.Minute, // TODO:Expose as option
		syncConcurrency:                          1,                // TODO: Expose as option
	}
}

func (c *cache) Start(ctx context.Context) error {
	log := ctrl.LoggerFrom(ctx)

	if ctx == nil {
		return fmt.Errorf("context cannot be nil")
	}

	if c.started {
		return fmt.Errorf("cache started more than once")
	}

	log.Info("Starting cache")

	if err := c.startGarbageCollector(ctx); err != nil {
		return err
	}
	if err := c.startSyncer(ctx); err != nil {
		return err
	}

	c.started = true
	log.Info("Cache successfully started!")
	return nil
}

func (c *cache) AddResourceGroup(name string) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if _, ok := c.resourceGroups[name]; ok {
		return
	}
	c.resourceGroups[name] = &resourceGroupTracker{
		objects:             map[schema.GroupVersionKind]map[types.NamespacedName]client.Object{},
		ownedObjects:        map[ownReference]map[ownReference]struct{}{},
		lastResourceVersion: 0,
	}
}

func (c *cache) DeleteResourceGroup(name string) {
	c.lock.Lock()
	defer c.lock.Unlock()
	delete(c.resourceGroups, name)
}

func (c *cache) resourceGroupTracker(resourceGroup string) *resourceGroupTracker {
	c.lock.RLock()
	defer c.lock.RUnlock()
	return c.resourceGroups[resourceGroup]
}

func (c *cache) gvkGetAndSet(obj runtime.Object) (schema.GroupVersionKind, error) {
	gvk, err := apiutil.GVKForObject(obj, c.scheme)
	if err != nil {
		return schema.GroupVersionKind{}, apierrors.NewInternalError(err)
	}

	obj.GetObjectKind().SetGroupVersionKind(gvk)
	return gvk, nil
}

// unsafeGuessGroupVersionResource assumes Resource is equal to Kind which is not the case in normal Kubernetes.
func unsafeGuessGroupVersionResource(gvk schema.GroupVersionKind) schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: gvk.Group, Version: gvk.Version, Resource: gvk.Kind}
}

func unsafeGuessObjectKindFromList(gvk schema.GroupVersionKind) schema.GroupVersionKind {
	return schema.GroupVersionKind{Group: gvk.Group, Version: gvk.Version, Kind: strings.TrimSuffix(gvk.Kind, "List")}
}
