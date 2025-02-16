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
	"fmt"
	"time"

	jsonpatch "github.com/evanphx/json-patch/v5"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (c *cache) Get(resourceGroup string, objKey client.ObjectKey, obj client.Object) error {
	if resourceGroup == "" {
		return apierrors.NewBadRequest("resourceGroup must not be empty")
	}

	if objKey.Name == "" {
		return apierrors.NewBadRequest("objKey.Name must not be empty")
	}

	if obj == nil {
		return apierrors.NewBadRequest("object must not be nil")
	}

	objGVK, err := c.gvkGetAndSet(obj)
	if err != nil {
		return err
	}

	tracker := c.resourceGroupTracker(resourceGroup)
	if tracker == nil {
		return apierrors.NewBadRequest(fmt.Sprintf("resourceGroup %s does not exist", resourceGroup))
	}

	tracker.lock.RLock()
	defer tracker.lock.RUnlock()

	objects, ok := tracker.objects[objGVK]
	if !ok {
		return apierrors.NewNotFound(unsafeGuessGroupVersionResource(objGVK).GroupResource(), objKey.String())
	}

	trackedObj, ok := objects[objKey]
	if !ok {
		return apierrors.NewNotFound(unsafeGuessGroupVersionResource(objGVK).GroupResource(), objKey.String())
	}

	if err := c.scheme.Convert(trackedObj, obj, nil); err != nil {
		return apierrors.NewInternalError(err)
	}
	obj.GetObjectKind().SetGroupVersionKind(trackedObj.GetObjectKind().GroupVersionKind())

	return nil
}

func (c *cache) List(resourceGroup string, list client.ObjectList, opts ...client.ListOption) error {
	if resourceGroup == "" {
		return apierrors.NewBadRequest("resourceGroup must not be empty")
	}

	if list == nil {
		return apierrors.NewBadRequest("list must not be nil")
	}

	gvk, err := c.gvkGetAndSet(list)
	if err != nil {
		return err
	}

	tracker := c.resourceGroupTracker(resourceGroup)
	if tracker == nil {
		return apierrors.NewBadRequest(fmt.Sprintf("resourceGroup %s does not exist", resourceGroup))
	}

	tracker.lock.RLock()
	defer tracker.lock.RUnlock()

	items := make([]runtime.Object, 0)
	objects, ok := tracker.objects[unsafeGuessObjectKindFromList(gvk)]
	if ok {
		listOpts := client.ListOptions{}
		listOpts.ApplyOptions(opts)

		for _, obj := range objects {
			if listOpts.Namespace != "" && obj.GetNamespace() != listOpts.Namespace {
				continue
			}

			if listOpts.LabelSelector != nil && !listOpts.LabelSelector.Empty() {
				metaLabels := labels.Set(obj.GetLabels())
				if !listOpts.LabelSelector.Matches(metaLabels) {
					continue
				}
			}

			// TODO(killianmuldoon): This only matches the nodeName field for pods. No other fieldSelectors are implemented. This should return an error if another fieldselector is used.
			if pod, ok := obj.(*corev1.Pod); ok {
				if listOpts.FieldSelector != nil && !listOpts.FieldSelector.Empty() {
					if !listOpts.FieldSelector.Matches(fields.Set{"spec.nodeName": pod.Spec.NodeName}) {
						continue
					}
				}
			}

			obj := obj.DeepCopyObject().(client.Object)
			switch list.(type) {
			case *unstructured.UnstructuredList:
				unstructuredObj := &unstructured.Unstructured{}
				if err := c.scheme.Convert(obj, unstructuredObj, nil); err != nil {
					return apierrors.NewInternalError(err)
				}
				items = append(items, unstructuredObj)
			default:
				items = append(items, obj)
			}
		}
	}

	if err := meta.SetList(list, items); err != nil {
		return apierrors.NewInternalError(err)
	}

	list.SetResourceVersion(fmt.Sprintf("%d", tracker.lastResourceVersion))

	return nil
}

func (c *cache) Create(resourceGroup string, obj client.Object) error {
	return c.store(resourceGroup, obj, false)
}

func (c *cache) Update(resourceGroup string, obj client.Object) error {
	return c.store(resourceGroup, obj, true)
}

func (c *cache) store(resourceGroup string, obj client.Object, replaceExisting bool) error {
	if resourceGroup == "" {
		return apierrors.NewBadRequest("resourceGroup must not be empty")
	}

	if obj == nil {
		return apierrors.NewBadRequest("object must not be nil")
	}

	objGVK, err := c.gvkGetAndSet(obj)
	if err != nil {
		return err
	}

	if replaceExisting && obj.GetName() == "" {
		return apierrors.NewBadRequest("object name must not be empty")
	}

	tracker := c.resourceGroupTracker(resourceGroup)
	if tracker == nil {
		return apierrors.NewBadRequest(fmt.Sprintf("resourceGroup %s does not exist", resourceGroup))
	}

	tracker.lock.Lock()
	defer tracker.lock.Unlock()

	// Note: This just validates that all owners exist in tracker.
	for _, o := range obj.GetOwnerReferences() {
		oRef, err := newOwnReferenceFromOwnerReference(obj.GetNamespace(), o)
		if err != nil {
			return err
		}
		objects, ok := tracker.objects[oRef.gvk]
		if !ok {
			return apierrors.NewBadRequest(fmt.Sprintf("ownerReference %s, Name=%s does not exist (GVK not tracked)", oRef.gvk, oRef.key.Name))
		}
		if _, ok := objects[oRef.key]; !ok {
			return apierrors.NewBadRequest(fmt.Sprintf("ownerReference %s, Name=%s does not exist", oRef.gvk, oRef.key.Name))
		}
	}

	_, ok := tracker.objects[objGVK]
	if !ok {
		tracker.objects[objGVK] = make(map[types.NamespacedName]client.Object)
	}

	// TODO: if unstructured, convert to typed object

	objKey := client.ObjectKeyFromObject(obj)
	objRef := ownReference{gvk: objGVK, key: objKey}
	if trackedObj, ok := tracker.objects[objGVK][objKey]; ok {
		if replaceExisting {
			if trackedObj.GetResourceVersion() != obj.GetResourceVersion() {
				return apierrors.NewConflict(unsafeGuessGroupVersionResource(objGVK).GroupResource(), objKey.String(), fmt.Errorf("object has been modified"))
			}

			c.beforeUpdate(resourceGroup, trackedObj, obj, &tracker.lastResourceVersion)

			tracker.objects[objGVK][objKey] = obj.DeepCopyObject().(client.Object)
			updateTrackerOwnerReferences(tracker, trackedObj, obj, objRef)
			c.afterUpdate(resourceGroup, trackedObj, obj)
			return nil
		}
		return apierrors.NewAlreadyExists(unsafeGuessGroupVersionResource(objGVK).GroupResource(), objKey.String())
	}

	if replaceExisting {
		return apierrors.NewNotFound(unsafeGuessGroupVersionResource(objGVK).GroupResource(), objKey.String())
	}

	c.beforeCreate(resourceGroup, obj, &tracker.lastResourceVersion)

	tracker.objects[objGVK][objKey] = obj.DeepCopyObject().(client.Object)
	updateTrackerOwnerReferences(tracker, nil, obj, objRef)
	c.afterCreate(resourceGroup, obj)
	return nil
}

func updateTrackerOwnerReferences(tracker *resourceGroupTracker, oldObj, newObj client.Object, objRef ownReference) {
	if oldObj != nil {
		for _, oldOwnerRef := range oldObj.GetOwnerReferences() {
			ownerRefStillExists := false
			for _, newOwnerRef := range newObj.GetOwnerReferences() {
				if oldOwnerRef == newOwnerRef {
					ownerRefStillExists = true
					break
				}
			}

			if !ownerRefStillExists {
				// Remove ownerRef from tracker (if necessary) as it has been removed from object.
				oldRef, _ := newOwnReferenceFromOwnerReference(newObj.GetNamespace(), oldOwnerRef)
				if _, ok := tracker.ownedObjects[*oldRef]; !ok {
					continue
				}
				delete(tracker.ownedObjects[*oldRef], objRef)
				if len(tracker.ownedObjects[*oldRef]) == 0 {
					delete(tracker.ownedObjects, *oldRef)
				}
			}
		}
	}
	for _, newOwnerRef := range newObj.GetOwnerReferences() {
		ownerRefAlreadyExisted := false
		if oldObj != nil {
			for _, oldOwnerRef := range oldObj.GetOwnerReferences() {
				if newOwnerRef == oldOwnerRef {
					ownerRefAlreadyExisted = true
					break
				}
			}
		}

		if !ownerRefAlreadyExisted {
			// Add new ownerRef to tracker.
			newRef, _ := newOwnReferenceFromOwnerReference(newObj.GetNamespace(), newOwnerRef)
			if _, ok := tracker.ownedObjects[*newRef]; !ok {
				tracker.ownedObjects[*newRef] = map[ownReference]struct{}{}
			}
			tracker.ownedObjects[*newRef][objRef] = struct{}{}
		}
	}
}

func (c *cache) Patch(resourceGroup string, obj client.Object, patch client.Patch) error {
	patchData, err := patch.Data(obj)
	if err != nil {
		return apierrors.NewInternalError(err)
	}

	encoder, err := c.getEncoder(obj, obj.GetObjectKind().GroupVersionKind().GroupVersion())
	if err != nil {
		return apierrors.NewInternalError(err)
	}

	originalObjJS, err := runtime.Encode(encoder, obj)
	if err != nil {
		return apierrors.NewInternalError(err)
	}

	var changedJS []byte
	switch patch.Type() {
	case types.MergePatchType:
		changedJS, err = jsonpatch.MergePatch(originalObjJS, patchData)
		if err != nil {
			return apierrors.NewInternalError(err)
		}
	case types.StrategicMergePatchType:
		// NOTE: we are treating StrategicMergePatch as MergePatch; it is an acceptable proxy for this use case.
		changedJS, err = jsonpatch.MergePatch(originalObjJS, patchData)
		if err != nil {
			return apierrors.NewInternalError(err)
		}
	default:
		return apierrors.NewBadRequest(fmt.Sprintf("patch of type %s is not supported", patch.Type()))
	}

	codecFactory := serializer.NewCodecFactory(c.scheme)
	err = runtime.DecodeInto(codecFactory.UniversalDecoder(), changedJS, obj)
	if err != nil {
		return apierrors.NewInternalError(err)
	}

	return c.store(resourceGroup, obj, true)
}

func (c *cache) getEncoder(obj runtime.Object, gv runtime.GroupVersioner) (runtime.Encoder, error) {
	codecs := serializer.NewCodecFactory(c.scheme)

	info, ok := runtime.SerializerInfoForMediaType(codecs.SupportedMediaTypes(), runtime.ContentTypeJSON)
	if !ok {
		return nil, fmt.Errorf("failed to create serializer for %T", obj)
	}

	encoder := codecs.EncoderForVersion(info.Serializer, gv)
	return encoder, nil
}

func (c *cache) Delete(resourceGroup string, obj client.Object) error {
	if resourceGroup == "" {
		return apierrors.NewBadRequest("resourceGroup must not be empty")
	}

	if obj == nil {
		return apierrors.NewBadRequest("object must not be nil")
	}

	if obj.GetName() == "" {
		return apierrors.NewBadRequest("object name must not be empty")
	}

	objGVK, err := c.gvkGetAndSet(obj)
	if err != nil {
		return err
	}

	obj = obj.DeepCopyObject().(client.Object)

	objKey := client.ObjectKeyFromObject(obj)
	deleted, err := c.tryDelete(resourceGroup, objGVK, objKey)
	if err != nil {
		return err
	}
	if !deleted {
		c.garbageCollectorQueue.Add(gcRequest{
			resourceGroup: resourceGroup,
			gvk:           objGVK,
			key:           objKey,
		})
	}
	return nil
}

func (c *cache) tryDelete(resourceGroup string, gvk schema.GroupVersionKind, key types.NamespacedName) (bool, error) {
	tracker := c.resourceGroupTracker(resourceGroup)
	if tracker == nil {
		return true, apierrors.NewBadRequest(fmt.Sprintf("resourceGroup %s does not exist", resourceGroup))
	}

	tracker.lock.Lock()
	defer tracker.lock.Unlock()

	return c.doTryDeleteLocked(resourceGroup, tracker, gvk, key)
}

// doTryDeleteLocked tries to delete an objects.
// Note: The tracker must bve already locked when calling this method.
func (c *cache) doTryDeleteLocked(resourceGroup string, tracker *resourceGroupTracker, objGVK schema.GroupVersionKind, objKey types.NamespacedName) (bool, error) {
	objects, ok := tracker.objects[objGVK]
	if !ok {
		return true, apierrors.NewNotFound(unsafeGuessGroupVersionResource(objGVK).GroupResource(), objKey.String())
	}

	obj, ok := tracker.objects[objGVK][objKey]
	if !ok {
		return true, apierrors.NewNotFound(unsafeGuessGroupVersionResource(objGVK).GroupResource(), objKey.String())
	}

	// Loop through objects that are owned by obj and try to delete them.
	// TODO: Consider only deleting the hierarchy if the obj doesn't have any finalizers.
	if ownedReferences, ok := tracker.ownedObjects[ownReference{gvk: objGVK, key: objKey}]; ok {
		for ref := range ownedReferences {
			deleted, err := c.doTryDeleteLocked(resourceGroup, tracker, ref.gvk, ref.key)
			if err != nil {
				return false, err
			}
			if !deleted {
				c.garbageCollectorQueue.Add(gcRequest{
					resourceGroup: resourceGroup,
					gvk:           ref.gvk,
					key:           ref.key,
				})
			}
		}
		delete(tracker.ownedObjects, ownReference{gvk: objGVK, key: objKey})
	}

	// Set the deletion timestamp if not already set.
	if obj.GetDeletionTimestamp().IsZero() {
		if err := c.beforeDelete(resourceGroup, obj); err != nil {
			return false, apierrors.NewBadRequest(err.Error())
		}

		oldObj := obj.DeepCopyObject().(client.Object)
		now := metav1.Time{Time: time.Now().UTC()}
		obj.SetDeletionTimestamp(&now)
		c.beforeUpdate(resourceGroup, oldObj, obj, &tracker.lastResourceVersion)

		objects[objKey] = obj
		c.afterUpdate(resourceGroup, oldObj, obj)
	}

	// If the object still has finalizers return early.
	if len(obj.GetFinalizers()) > 0 {
		return false, nil
	}

	// Object doesn't have finalizers, delete it.
	// Note: we don't call informDelete here because we couldn't reconcile it
	// because the object is already gone from the tracker.
	delete(objects, objKey)
	c.afterDelete(resourceGroup, obj)
	return true, nil
}
