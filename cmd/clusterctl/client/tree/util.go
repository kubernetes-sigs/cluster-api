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

package tree

import (
	"fmt"
	"sort"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/conditions"
)

// GetReadyCondition returns the ReadyCondition for an object, if defined.
func GetReadyCondition(obj client.Object) *clusterv1.Condition {
	getter := objToGetter(obj)
	if getter == nil {
		return nil
	}
	return conditions.Get(getter, clusterv1.ReadyCondition)
}

// GetOtherConditions returns the other conditions (all the conditions except ready) for an object, if defined.
func GetOtherConditions(obj client.Object) []*clusterv1.Condition {
	getter := objToGetter(obj)
	if getter == nil {
		return nil
	}
	var conditions []*clusterv1.Condition
	for _, c := range getter.GetConditions() {
		c := c
		if c.Type != clusterv1.ReadyCondition {
			conditions = append(conditions, &c)
		}
	}
	sort.Slice(conditions, func(i, j int) bool {
		return conditions[i].Type < conditions[j].Type
	})
	return conditions
}

func setReadyCondition(obj client.Object, ready *clusterv1.Condition) {
	setter := objToSetter(obj)
	if setter == nil {
		return
	}
	conditions.Set(setter, ready)
}

func objToGetter(obj client.Object) conditions.Getter {
	if getter, ok := obj.(conditions.Getter); ok {
		return getter
	}

	objUnstructured, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return nil
	}
	getter := conditions.UnstructuredGetter(objUnstructured)
	return getter
}

func objToSetter(obj client.Object) conditions.Setter {
	if setter, ok := obj.(conditions.Setter); ok {
		return setter
	}

	objUnstructured, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return nil
	}
	setter := conditions.UnstructuredSetter(objUnstructured)
	return setter
}

// VirtualObject return a new virtual object.
func VirtualObject(namespace, kind, name string) *unstructured.Unstructured {
	gk := "virtual.cluster.x-k8s.io/v1beta1"
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": gk,
			"kind":       kind,
			"metadata": map[string]interface{}{
				"namespace": namespace,
				"name":      name,
				"annotations": map[string]interface{}{
					VirtualObjectAnnotation: "True",
				},
				"uid": fmt.Sprintf("%s, %s/%s", gk, namespace, name),
			},
		},
	}
}
