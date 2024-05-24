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
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewTestClusterCacheTracker creates a new fake ClusterCacheTracker that can be used by unit tests with fake client.
func NewTestClusterCacheTracker(log logr.Logger, cl client.Client, remoteClient client.Client, scheme *runtime.Scheme, objKey client.ObjectKey, watchObjects ...string) *ClusterCacheTracker {
	testCacheTracker := &ClusterCacheTracker{
		log:              log,
		client:           cl,
		scheme:           scheme,
		clusterAccessors: make(map[client.ObjectKey]*clusterAccessor),
		clusterLock:      newKeyedMutex(),
	}

	testCacheTracker.clusterAccessors[objKey] = &clusterAccessor{
		cache:   nil,
		client:  remoteClient,
		watches: sets.Set[string]{}.Insert(watchObjects...),
	}
	return testCacheTracker
}
