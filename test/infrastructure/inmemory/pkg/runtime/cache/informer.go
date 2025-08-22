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
	"sync"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

type informer struct {
	handlers []InformEventHandler
	lock     sync.RWMutex
}

func (i *informer) AddEventHandler(handler InformEventHandler) error {
	i.lock.Lock()
	defer i.lock.Unlock()

	i.handlers = append(i.handlers, handler)
	return nil
}

func (i *informer) RemoveEventHandler(handler InformEventHandler) error {
	i.lock.Lock()
	defer i.lock.Unlock()
	for j, h := range i.handlers {
		if h == handler {
			i.handlers = append(i.handlers[:j], i.handlers[j+1:]...)
		}
	}
	return nil
}

func (c *cache) GetInformer(ctx context.Context, obj client.Object) (Informer, error) {
	gvk, err := apiutil.GVKForObject(obj, c.scheme)
	if err != nil {
		return nil, err
	}
	return c.GetInformerForKind(ctx, gvk)
}

func (c *cache) GetInformerForKind(_ context.Context, gvk schema.GroupVersionKind) (Informer, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if _, ok := c.informers[gvk]; !ok {
		c.informers[gvk] = &informer{}
	}
	return c.informers[gvk], nil
}

func (c *cache) informCreate(resourceGroup string, obj client.Object) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	if i, ok := c.informers[obj.GetObjectKind().GroupVersionKind()]; ok {
		i := i.(*informer)
		i.lock.RLock()
		defer i.lock.RUnlock()

		for _, h := range i.handlers {
			h.OnCreate(resourceGroup, obj)
		}
	}
}

func (c *cache) informUpdate(resourceGroup string, oldObj, newObj client.Object) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	if i, ok := c.informers[newObj.GetObjectKind().GroupVersionKind()]; ok {
		i := i.(*informer)
		i.lock.RLock()
		defer i.lock.RUnlock()

		for _, h := range i.handlers {
			h.OnUpdate(resourceGroup, oldObj, newObj)
		}
	}
}

func (c *cache) informDelete(resourceGroup string, obj client.Object) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	if i, ok := c.informers[obj.GetObjectKind().GroupVersionKind()]; ok {
		i := i.(*informer)
		i.lock.RLock()
		defer i.lock.RUnlock()

		for _, h := range i.handlers {
			h.OnDelete(resourceGroup, obj)
		}
	}
}

func (c *cache) informSync(resourceGroup string, obj client.Object) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	if i, ok := c.informers[obj.GetObjectKind().GroupVersionKind()]; ok {
		i := i.(*informer)
		i.lock.RLock()
		defer i.lock.RUnlock()

		for _, h := range i.handlers {
			h.OnUpdate(resourceGroup, obj, obj)
		}
	}
}
