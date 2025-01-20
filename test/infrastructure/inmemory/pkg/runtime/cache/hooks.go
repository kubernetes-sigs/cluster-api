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
	"reflect"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (c *cache) beforeCreate(_ string, obj client.Object, resourceVersion *uint64) {
	now := time.Now().UTC()
	obj.SetCreationTimestamp(metav1.Time{Time: now})
	// TODO: UID
	obj.SetAnnotations(appendAnnotations(obj, lastSyncTimeAnnotation, now.Format(time.RFC3339)))
	*resourceVersion++
	obj.SetResourceVersion(fmt.Sprintf("%d", *resourceVersion))
	obj.SetGeneration(1)
}

func (c *cache) afterCreate(resourceGroup string, obj client.Object) {
	c.informCreate(resourceGroup, obj)
}

func (c *cache) beforeUpdate(_ string, oldObj, newObj client.Object, resourceVersion *uint64) {
	newObj.SetCreationTimestamp(oldObj.GetCreationTimestamp())
	newObj.SetResourceVersion(oldObj.GetResourceVersion())
	newObj.SetGeneration(oldObj.GetGeneration())
	// TODO: UID
	newObj.SetAnnotations(appendAnnotations(newObj, lastSyncTimeAnnotation, oldObj.GetAnnotations()[lastSyncTimeAnnotation]))
	if !oldObj.GetDeletionTimestamp().IsZero() {
		newObj.SetDeletionTimestamp(oldObj.GetDeletionTimestamp())
	}
	if !reflect.DeepEqual(newObj, oldObj) {
		now := time.Now().UTC()
		newObj.SetAnnotations(appendAnnotations(newObj, lastSyncTimeAnnotation, now.Format(time.RFC3339)))

		*resourceVersion++
		newObj.SetResourceVersion(fmt.Sprintf("%d", *resourceVersion))
		newObj.SetGeneration(oldObj.GetGeneration() + 1)
	}
}

func (c *cache) afterUpdate(resourceGroup string, oldObj, newObj client.Object) {
	if oldObj.GetDeletionTimestamp().IsZero() && !newObj.GetDeletionTimestamp().IsZero() {
		c.informDelete(resourceGroup, newObj)
		return
	}
	if !reflect.DeepEqual(newObj, oldObj) {
		c.informUpdate(resourceGroup, oldObj, newObj)
	}
}

func (c *cache) beforeDelete(_ string, _ client.Object) error {
	return nil
}

func (c *cache) afterDelete(_ string, _ client.Object) {
}

func appendAnnotations(obj client.Object, kayValuePair ...string) map[string]string {
	newAnnotations := map[string]string{}
	for k, v := range obj.GetAnnotations() {
		newAnnotations[k] = v
	}
	for i := 0; i < len(kayValuePair)-1; i += 2 {
		k := kayValuePair[i]
		v := kayValuePair[i+1]
		newAnnotations[k] = v
	}
	return newAnnotations
}
