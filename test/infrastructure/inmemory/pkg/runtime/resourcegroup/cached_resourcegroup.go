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

package resourcegroup

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	inmemorycache "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/pkg/runtime/cache"
	inmemoryclient "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/pkg/runtime/client"
)

var _ ResourceGroup = &cachedResourceGroup{}

type cachedResourceGroup struct {
	name  string
	cache inmemorycache.Cache
}

// NewResourceGroup returns a new resource group.
func NewResourceGroup(name string, cache inmemorycache.Cache) ResourceGroup {
	return &cachedResourceGroup{
		name:  name,
		cache: cache,
	}
}

func (cc *cachedResourceGroup) GetClient() inmemoryclient.Client {
	return &cachedClient{
		resourceGroup: cc.name,
		cache:         cc.cache,
	}
}

var _ inmemoryclient.Client = &cachedClient{}

type cachedClient struct {
	resourceGroup string
	cache         inmemorycache.Cache
}

func (c *cachedClient) Get(_ context.Context, key client.ObjectKey, obj client.Object) error {
	return c.cache.Get(c.resourceGroup, key, obj)
}

func (c *cachedClient) List(_ context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return c.cache.List(c.resourceGroup, list, opts...)
}

func (c *cachedClient) Create(_ context.Context, obj client.Object) error {
	return c.cache.Create(c.resourceGroup, obj)
}

func (c *cachedClient) Delete(_ context.Context, obj client.Object) error {
	return c.cache.Delete(c.resourceGroup, obj)
}

func (c *cachedClient) Update(_ context.Context, obj client.Object) error {
	return c.cache.Update(c.resourceGroup, obj)
}

func (c *cachedClient) Patch(_ context.Context, obj client.Object, patch client.Patch) error {
	return c.cache.Patch(c.resourceGroup, obj, patch)
}
