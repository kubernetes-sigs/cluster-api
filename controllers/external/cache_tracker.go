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

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/util/predicates"
)

func NewContractObjectCache(mgr manager.Manager, controller controller.Controller, c client.Client, predicateLog *logr.Logger, watchNamespace, controllerName string) *ContractObjectCache {
	return &ContractObjectCache{
		Manager:         mgr,
		Controller:      controller,
		Client:          c,
		PredicateLogger: predicateLog,
		WatchNamespace:  watchNamespace,
		ControllerName:  controllerName,
		caches:          map[schema.GroupVersionKind]*cacheEntry{},
	}
}

// ContractObjectCache is a helper struct to deal when watching external unstructured objects.
// FIXME: rethink name, API and location of this util, maybe introduce New constructur / interface etc.
type ContractObjectCache struct {
	cachesLock sync.RWMutex
	caches     map[schema.GroupVersionKind]*cacheEntry

	watches sync.Map

	Controller      controller.Controller
	Client          client.Client
	Manager         manager.Manager
	PredicateLogger *logr.Logger
	WatchNamespace  string
	ControllerName  string
}

type cacheEntry struct {
	Scheme *runtime.Scheme
	Cache  cache.Cache
}

// Get uses the controller to issue a Watch only if the object hasn't been seen before.
func (o *ContractObjectCache) Get(ctx context.Context, ref clusterv1.ContractVersionedObjectReference, namespace string, obj client.Object, objList client.ObjectList) (client.Object, error) {
	// Get GVK.
	gvk, err := o.getGVK(ctx, ref)
	if err != nil {
		return nil, err
	}

	// Create cache if necessary.
	ce, err := o.getOrCreateCache(ctx, gvk, obj, objList)
	if err != nil {
		return nil, err
	}

	// Get object. // FIXME: what about informer sync?
	obj = obj.DeepCopyObject().(client.Object)
	objKey := client.ObjectKey{
		Namespace: namespace,
		Name:      ref.Name,
	}
	if err := ce.Cache.Get(ctx, objKey, obj); err != nil {
		return nil, err
	}
	return obj, nil
}

// Watch uses the controller to issue a Watch only if the object hasn't been seen before.
func (o *ContractObjectCache) Watch(ctx context.Context, ref clusterv1.ContractVersionedObjectReference, obj client.Object, objList client.ObjectList, handler handler.EventHandler) error {
	log := ctrl.LoggerFrom(ctx)

	// Get GVK.
	gvk, err := o.getGVK(ctx, ref)
	if err != nil {
		return err
	}

	// Create cache if necessary.
	ce, err := o.getOrCreateCache(ctx, gvk, obj, objList)
	if err != nil {
		return err
	}

	// Add watch if necessary.
	if _, watchAdded := o.watches.LoadOrStore(gvk, struct{}{}); watchAdded { // FIXME: double check if per gvk and not per gk is correct
		return err
	}

	log.Info(fmt.Sprintf("Adding watch on external object %q", gvk.String()))
	err = o.Controller.Watch(source.Kind(
		ce.Cache,
		obj,
		handler,
		// ResourceIsChanged is not needed because Resync is turned off with syncPeriod: 0
		predicates.ResourceNotPaused(ce.Scheme, *o.PredicateLogger),
	))
	if err != nil {
		o.watches.Delete(gvk)
		return errors.Wrapf(err, "failed to add watch on external object %q", gvk.String())
	}
	return nil
}

func (o *ContractObjectCache) getOrCreateCache(_ context.Context, gvk schema.GroupVersionKind, obj client.Object, objList client.ObjectList) (*cacheEntry, error) {
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

	scheme := runtime.NewScheme()
	metav1.AddToGroupVersion(scheme, gvk.GroupVersion())
	scheme.AddKnownTypeWithName(gvk, obj)
	scheme.AddKnownTypeWithName(gvkList, objList)

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

func (o *ContractObjectCache) getGVK(ctx context.Context, ref clusterv1.ContractVersionedObjectReference) (schema.GroupVersionKind, error) {
	metadata, err := contract.GetGKMetadata(ctx, o.Client, schema.GroupKind{Group: ref.APIGroup, Kind: ref.Kind})
	if err != nil {
		if apierrors.IsNotFound(err) {
			// We want to surface the NotFound error only for the referenced object, so we use a generic error in case CRD is not found.
			return schema.GroupVersionKind{}, errors.Errorf("failed to get object from ref: %v", err.Error())
		}
		return schema.GroupVersionKind{}, errors.Wrapf(err, "failed to get object from ref")
	}

	_, latestAPIVersion, err := contract.GetLatestContractAndAPIVersionFromContract(metadata, contract.Version)
	if err != nil {
		return schema.GroupVersionKind{}, errors.Wrapf(err, "failed to get object from ref")
	}

	gvk := schema.GroupVersionKind{
		Group:   ref.APIGroup,
		Version: latestAPIVersion,
		Kind:    ref.Kind,
	}
	return gvk, nil
}
