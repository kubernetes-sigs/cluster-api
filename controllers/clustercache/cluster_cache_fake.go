/*
Copyright 2024 The Kubernetes Authors.

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

package clustercache

import (
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewFakeClusterCache creates a new fake ClusterCache that can be used by unit tests.
func NewFakeClusterCache(workloadClient client.Client, clusterKey client.ObjectKey, watchObjects ...string) ClusterCache {
	testCacheTracker := &clusterCache{
		clusterAccessors: make(map[client.ObjectKey]*clusterAccessor),
	}

	testCacheTracker.clusterAccessors[clusterKey] = &clusterAccessor{
		lockedState: clusterAccessorLockedState{
			connection: &clusterAccessorLockedConnectionState{
				cachedClient: workloadClient,
				watches:      sets.Set[string]{}.Insert(watchObjects...),
			},
		},
	}
	return testCacheTracker
}
