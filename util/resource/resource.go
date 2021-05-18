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

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

var defaultCreatePriorities = []string{
	// Namespaces go first because all namespaced resources depend on them.
	"Namespace",
	// Custom Resource Definitions come before Custom Resource so that they can be
	// restored with their corresponding CRD.
	"CustomResourceDefinition",
	// Storage Classes are needed to create PVs and PVCs correctly.
	"StorageClass",
	// PVs go before PVCs because PVCs depend on them.
	"PersistentVolume",
	// PVCs go before pods or controllers so they can be mounted as volumes.
	"PersistentVolumeClaim",
	// Secrets and ConfigMaps go before pods or controllers so they can be mounted as volumes.
	"Secret",
	"ConfigMap",
	// Service accounts go before pods or controllers so pods can use them.
	"ServiceAccount",
	// Limit ranges go before pods or controllers so pods can use them.
	"LimitRange",
	// Pods go before ReplicaSets
	"Pods",
	// ReplicaSets go before Deployments
	"ReplicaSet",
	// Endpoints go before Services
	"Endpoints",
}

// SortForCreate sorts objects by creation priority.
func SortForCreate(resources []unstructured.Unstructured) []unstructured.Unstructured {
	var ret []unstructured.Unstructured

	// First get resources by priority
	for _, p := range defaultCreatePriorities {
		for _, o := range resources {
			if o.GetKind() == p {
				ret = append(ret, o)
			}
		}
	}

	// Then get all the other resources
	for _, o := range resources {
		found := false
		for _, r := range ret {
			if o.GroupVersionKind() == r.GroupVersionKind() && o.GetNamespace() == r.GetNamespace() && o.GetName() == r.GetName() {
				found = true
				break
			}
		}
		if !found {
			ret = append(ret, o)
		}
	}

	return ret
}
