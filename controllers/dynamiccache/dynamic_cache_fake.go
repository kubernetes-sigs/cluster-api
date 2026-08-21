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
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	"sigs.k8s.io/cluster-api/internal/contract"
)

// NewFakeDynamicCache creates a new fake DynamicCache that can be used by unit tests.
func NewFakeDynamicCache(client client.Client, byObjectTypeOptions map[ObjectType]ByObjectTypeOptions) DynamicCache {
	return &fakeDynamicCache{
		client:              client,
		byObjectTypeOptions: byObjectTypeOptions,
	}
}

var _ DynamicCache = &fakeDynamicCache{}

type fakeDynamicCache struct {
	DynamicCache
	client              client.Client
	byObjectTypeOptions map[ObjectType]ByObjectTypeOptions
}

func (dc *fakeDynamicCache) GetUnstructured(ctx context.Context, objGK schema.GroupKind, objKey client.ObjectKey, _ ObjectType) (*unstructured.Unstructured, error) {
	_, objGVK, err := contract.GetGVKFromGK(ctx, dc.client, objGK)
	if err != nil {
		return nil, fmt.Errorf("failed to get Unstructured %s %s: %w", objGK.Kind, klog.KRef(objKey.Namespace, objKey.Name), err)
	}

	// Construct obj.
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(objGVK)

	// Get object.
	if err := dc.client.Get(ctx, objKey, obj); err != nil {
		return nil, fmt.Errorf("failed to get Unstructured %s %s: %w", objGK.Kind, klog.KRef(objKey.Namespace, objKey.Name), err)
	}
	return obj, nil
}

func (dc *fakeDynamicCache) GetContractObject(ctx context.Context, objGK schema.GroupKind, objKey client.ObjectKey, objType ObjectType) (schema.GroupVersionKind, client.Object, error) {
	contractVersion, objGVK, err := contract.GetGVKFromGK(ctx, dc.client, objGK)
	if err != nil {
		return schema.GroupVersionKind{}, nil, fmt.Errorf("failed to get %s %s: %w", objGK.Kind, klog.KRef(objKey.Namespace, objKey.Name), err)
	}

	// Construct obj.
	obj, _, err := dc.getObjTypesFromOptions(objType, contractVersion)
	if err != nil {
		return schema.GroupVersionKind{}, nil, fmt.Errorf("failed to get %s %s: %w", objGK.Kind, klog.KRef(objKey.Namespace, objKey.Name), err)
	}

	// Get object.
	obj = obj.DeepCopyObject().(client.Object)
	if err := dc.client.Get(ctx, objKey, obj); err != nil {
		return schema.GroupVersionKind{}, nil, fmt.Errorf("failed to get %s %s: %w", objGK.Kind, klog.KRef(objKey.Namespace, objKey.Name), err)
	}
	return objGVK, obj, nil
}

func (dc *fakeDynamicCache) ListContractObjects(ctx context.Context, objGK schema.GroupKind, objType ObjectType, opts ...client.ListOption) (schema.GroupVersionKind, client.ObjectList, error) {
	contractVersion, objGVK, err := contract.GetGVKFromGK(ctx, dc.client, objGK)
	if err != nil {
		return schema.GroupVersionKind{}, nil, fmt.Errorf("failed to list %s: %w", objGK.Kind, err)
	}

	// Construct objList.
	_, objList, err := dc.getObjTypesFromOptions(objType, contractVersion)
	if err != nil {
		return schema.GroupVersionKind{}, nil, fmt.Errorf("failed to list %s: %w", objGK.Kind, err)
	}

	// List objects.
	objList = objList.DeepCopyObject().(client.ObjectList)
	if err := dc.client.List(ctx, objList, opts...); err != nil {
		return schema.GroupVersionKind{}, nil, fmt.Errorf("failed to list %s: %w", objGK.Kind, err)
	}
	return objGVK, objList, nil
}

func (dc *fakeDynamicCache) Watch(_ context.Context, _ string, _ SourceWatcher, _ schema.GroupKind, _ ObjectType, _ handler.EventHandler) error {
	return nil // No-op is enough for unit tests.
}

func (dc *fakeDynamicCache) GetCache(_ context.Context, _ schema.GroupVersionKind) (cache.Cache, bool) {
	panic("Not implemented")
}

func (dc *fakeDynamicCache) GetWriter(_ context.Context, _ schema.GroupVersionKind) (WriterWithScheme, bool) {
	return dc.client, true
}

func (dc *fakeDynamicCache) getObjTypesFromOptions(objType ObjectType, contractVersion string) (client.Object, client.ObjectList, error) {
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
