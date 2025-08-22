/*
Copyright 2019 The Kubernetes Authors.

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

// Package resource implements resource utilites.
package resource

import (
	"sort"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// orderMapping maps resource kinds to their creation order,
// the lower the number, the earlier the resource should be created.
var orderMapping = map[string]int{
	// Namespaces go first because all namespaced resources depend on them.
	"Namespace": -100,
	// Custom Resource Definitions come before Custom Resource so that they can be
	// restored with their corresponding CRD.
	"CustomResourceDefinition": -99,
	// Storage Classes are needed to create PVs and PVCs correctly.
	"StorageClass": -98,
	// PVs go before PVCs because PVCs depend on them.
	"PersistentVolume": -97,
	// PVCs go before pods or controllers so they can be mounted as volumes.
	"PersistentVolumeClaim": -96,
	// Secrets and ConfigMaps go before pods or controllers so they can be mounted as volumes.
	"Secret":    -95,
	"ConfigMap": -94,
	// Service accounts go before pods or controllers so pods can use them.
	"ServiceAccount": -93,
	// Limit ranges go before pods or controllers so pods can use them.
	"LimitRange": -92,
	// Pods go before ReplicaSets
	"Pod": -91,
	// ReplicaSets go before Deployments
	"ReplicaSet": -90,
	// Endpoints go before Services (and everything else)
	"Endpoint": -89,
}

// priorityLess returns true if o1 should be created before o2.
// To be used in sort.{Slice,SliceStable} to sort manifests in place.
func priorityLess(o1, o2 unstructured.Unstructured) bool {
	p1 := orderMapping[o1.GetKind()]
	p2 := orderMapping[o2.GetKind()]
	return p1 < p2
}

// SortForCreate sorts objects by creation priority.
func SortForCreate(resources []unstructured.Unstructured) []unstructured.Unstructured {
	ret := make([]unstructured.Unstructured, len(resources))
	copy(ret, resources)
	sort.SliceStable(ret, func(i, j int) bool {
		return priorityLess(ret[i], ret[j])
	})

	return ret
}
