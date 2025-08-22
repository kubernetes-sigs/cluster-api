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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
)

// GroupVersionVirtualObject is the group version for VirtualObject.
var GroupVersionVirtualObject = schema.GroupVersion{Group: "virtual.cluster.x-k8s.io", Version: clusterv1.GroupVersion.Version}

// GetReadyCondition returns the ReadyCondition for an object, if defined.
func GetReadyCondition(obj client.Object) *metav1.Condition {
	if getter, ok := obj.(conditions.Getter); ok {
		return conditions.Get(getter, clusterv1.ReadyCondition)
	}

	if objUnstructured, ok := obj.(*unstructured.Unstructured); ok {
		c, err := conditions.UnstructuredGet(objUnstructured, clusterv1.ReadyCondition)
		if err != nil {
			return nil
		}
		return c
	}

	return nil
}

// GetAvailableCondition returns the AvailableCondition for an object, if defined.
func GetAvailableCondition(obj client.Object) *metav1.Condition {
	if getter, ok := obj.(conditions.Getter); ok {
		return conditions.Get(getter, clusterv1.AvailableCondition)
	}

	if objUnstructured, ok := obj.(*unstructured.Unstructured); ok {
		c, err := conditions.UnstructuredGet(objUnstructured, clusterv1.AvailableCondition)
		if err != nil {
			return nil
		}
		return c
	}

	return nil
}

// GetMachineUpToDateCondition returns machine's UpToDate condition, if defined.
// Note: The UpToDate condition only exist on machines, so no need to support reading from unstructured.
func GetMachineUpToDateCondition(obj client.Object) *metav1.Condition {
	if getter, ok := obj.(conditions.Getter); ok {
		return conditions.Get(getter, clusterv1.MachineUpToDateCondition)
	}
	return nil
}

// GetV1Beta1ReadyCondition returns the ReadyCondition for an object, if defined.
func GetV1Beta1ReadyCondition(obj client.Object) *clusterv1.Condition {
	getter := objToGetter(obj)
	if getter == nil {
		return nil
	}
	return v1beta1conditions.Get(getter, clusterv1.ReadyV1Beta1Condition)
}

// GetConditions returns conditions for an object, if defined.
func GetConditions(obj client.Object) []metav1.Condition {
	if getter, ok := obj.(conditions.Getter); ok {
		return getter.GetConditions()
	}

	if objUnstructured, ok := obj.(*unstructured.Unstructured); ok {
		conditionList, err := conditions.UnstructuredGetAll(objUnstructured)
		if err != nil {
			return nil
		}
		return conditionList
	}

	return nil
}

// GetOtherV1Beta1Conditions returns the other conditions (all the conditions except ready) for an object, if defined.
func GetOtherV1Beta1Conditions(obj client.Object) []*clusterv1.Condition {
	getter := objToGetter(obj)
	if getter == nil {
		return nil
	}
	var conditions []*clusterv1.Condition
	for _, c := range getter.GetV1Beta1Conditions() {
		if c.Type != clusterv1.ReadyV1Beta1Condition {
			conditions = append(conditions, &c)
		}
	}
	sort.Slice(conditions, func(i, j int) bool {
		return conditions[i].Type < conditions[j].Type
	})
	return conditions
}

func setAvailableCondition(obj client.Object, available *metav1.Condition) {
	if setter, ok := obj.(conditions.Setter); ok {
		conditions.Set(setter, *available)
	}
}

func setReadyCondition(obj client.Object, ready *metav1.Condition) {
	if setter, ok := obj.(conditions.Setter); ok {
		conditions.Set(setter, *ready)
	}
}

func setUpToDateCondition(obj client.Object, upToDate *metav1.Condition) {
	if setter, ok := obj.(conditions.Setter); ok {
		conditions.Set(setter, *upToDate)
	}
}

func setReadyV1Beta1Condition(obj client.Object, ready *clusterv1.Condition) {
	setter := objToSetter(obj)
	if setter == nil {
		return
	}
	v1beta1conditions.Set(setter, ready)
}

func objToGetter(obj client.Object) v1beta1conditions.Getter {
	if getter, ok := obj.(v1beta1conditions.Getter); ok {
		return getter
	}

	objUnstructured, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return nil
	}
	getter := v1beta1conditions.UnstructuredGetter(objUnstructured)
	return getter
}

func objToSetter(obj client.Object) v1beta1conditions.Setter {
	if setter, ok := obj.(v1beta1conditions.Setter); ok {
		return setter
	}

	objUnstructured, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return nil
	}
	setter := v1beta1conditions.UnstructuredSetter(objUnstructured)
	return setter
}

// VirtualObject returns a new virtual object.
func VirtualObject(namespace, kind, name string) *NodeObject {
	return &NodeObject{
		TypeMeta: metav1.TypeMeta{
			Kind:       kind,
			APIVersion: GroupVersionVirtualObject.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
			Annotations: map[string]string{
				VirtualObjectAnnotation: "True",
			},
			UID: types.UID(fmt.Sprintf("%s, Kind=%s, %s/%s", GroupVersionVirtualObject.String(), kind, namespace, name)),
		},
		Status: NodeStatus{},
	}
}

// ObjectReferenceObject returns a new object referenced by the objectRef.
func ObjectReferenceObject(objectRef *corev1.ObjectReference) *NodeObject {
	return &NodeObject{
		TypeMeta: metav1.TypeMeta{
			Kind:       objectRef.Kind,
			APIVersion: objectRef.APIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: objectRef.Namespace,
			Name:      objectRef.Name,
			UID:       types.UID(fmt.Sprintf("%s, Kind=%s, %s/%s", objectRef.APIVersion, objectRef.Kind, objectRef.Namespace, objectRef.Name)),
		},
		Status: NodeStatus{},
	}
}
