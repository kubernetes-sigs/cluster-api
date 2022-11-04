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

package controllers

import (
	"context"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1 "sigs.k8s.io/cluster-api/exp/addons/api/v1beta1"
)

// resourceReconcileScope contains the scope for a CRS's resource
// reconciliation request.
type resourceReconcileScope interface {
	// needsApply determines if a resource needs to be applied to the target cluster
	// based on the strategy.
	needsApply() bool
	// apply reconciles the resource to the target cluster following a different process
	// depending on the strategy.
	apply(ctx context.Context, c client.Client, obj *unstructured.Unstructured) error
	// objs returns all the Kubernetes objects defined in the resource.
	objs() []unstructured.Unstructured
	// hash returns a computed hash of the defined objects in the resource. It is consistent
	// between runs.
	hash() string
}

func reconcileScopeForResource(
	crs *addonsv1.ClusterResourceSet,
	resourceRef addonsv1.ResourceRef,
	resourceSetBinding *addonsv1.ResourceSetBinding,
	resource *unstructured.Unstructured,
) (resourceReconcileScope, error) {
	normalizedData, err := normalizeData(resource)
	if err != nil {
		return nil, err
	}

	objs, err := objsFromYamlData(normalizedData)
	if err != nil {
		return nil, err
	}

	return newResourceReconcileScope(crs, resourceRef, resourceSetBinding, normalizedData, objs), nil
}

func newResourceReconcileScope(
	clusterResourceSet *addonsv1.ClusterResourceSet,
	resourceRef addonsv1.ResourceRef,
	resourceSetBinding *addonsv1.ResourceSetBinding,
	normalizedData [][]byte,
	objs []unstructured.Unstructured,
) resourceReconcileScope {
	if clusterResourceSet.IsStrategy(addonsv1.ClusterResourceSetStrategyApplyOnce) {
		return &reconcileApplyOnceScope{
			baseResourceReconcileScope{
				clusterResourceSet: clusterResourceSet,
				resourceRef:        resourceRef,
				resourceSetBinding: resourceSetBinding,
				data:               normalizedData,
				normalizedObjs:     objs,
				computedHash:       computeHash(normalizedData),
			},
		}
	} else if clusterResourceSet.IsStrategy(addonsv1.ClusterResourceSetStrategyReconcile) {
		return &reconcileStrategyScope{
			baseResourceReconcileScope{
				clusterResourceSet: clusterResourceSet,
				resourceRef:        resourceRef,
				resourceSetBinding: resourceSetBinding,
				data:               normalizedData,
				normalizedObjs:     objs,
				computedHash:       computeHash(normalizedData),
			},
		}
	}

	return nil
}

type baseResourceReconcileScope struct {
	clusterResourceSet *addonsv1.ClusterResourceSet
	resourceRef        addonsv1.ResourceRef
	resourceSetBinding *addonsv1.ResourceSetBinding
	normalizedObjs     []unstructured.Unstructured
	data               [][]byte
	computedHash       string
}

func (b baseResourceReconcileScope) objs() []unstructured.Unstructured {
	return b.normalizedObjs
}

func (b baseResourceReconcileScope) hash() string {
	return b.computedHash
}

type reconcileStrategyScope struct {
	baseResourceReconcileScope
}

func (r *reconcileStrategyScope) needsApply() bool {
	resourceBinding := r.resourceSetBinding.GetResourceBinding(r.resourceRef)

	return resourceBinding == nil || !resourceBinding.Applied || resourceBinding.Hash != r.computedHash
}

func (r *reconcileStrategyScope) apply(ctx context.Context, c client.Client, obj *unstructured.Unstructured) error {
	currentObj := &unstructured.Unstructured{}
	currentObj.SetAPIVersion(obj.GetAPIVersion())
	currentObj.SetKind(obj.GetKind())
	err := c.Get(ctx, client.ObjectKeyFromObject(obj), currentObj)
	if apierrors.IsNotFound(err) {
		return createUnstructured(ctx, c, obj)
	}
	if err != nil {
		return errors.Wrapf(
			err,
			"reading object %s %s/%s",
			obj.GroupVersionKind(),
			obj.GetNamespace(),
			obj.GetName(),
		)
	}

	patch := client.MergeFrom(currentObj.DeepCopy())
	if err = c.Patch(ctx, obj, patch); err != nil {
		return errors.Wrapf(
			err,
			"patching object %s %s/%s",
			obj.GroupVersionKind(),
			obj.GetNamespace(),
			obj.GetName(),
		)
	}

	return nil
}

type reconcileApplyOnceScope struct {
	baseResourceReconcileScope
}

func (r *reconcileApplyOnceScope) needsApply() bool {
	return !r.resourceSetBinding.IsApplied(r.resourceRef)
}

func (r *reconcileApplyOnceScope) apply(ctx context.Context, c client.Client, obj *unstructured.Unstructured) error {
	// The create call is idempotent, so if the object already exists
	// then do not consider it to be an error.
	if err := createUnstructured(ctx, c, obj); !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}
