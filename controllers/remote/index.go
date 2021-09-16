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

package remote

import (
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/cluster-api/api/v1beta1/index"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Index is a helper to model the info passed to cache.IndexField.
type Index struct {
	Object       client.Object
	Field        string
	ExtractValue client.IndexerFunc
}

var nodeProviderIDIndex = Index{
	Object:       &corev1.Node{},
	Field:        index.NodeProviderIDField,
	ExtractValue: index.NodeByProviderID,
}

// DefaultIndexes is the default list of indexes on a ClusterCacheTracker.
var DefaultIndexes = []Index{nodeProviderIDIndex}
