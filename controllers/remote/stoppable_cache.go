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
	"context"
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/cache"
)

// stoppableCache embeds cache.Cache and combines it with a stop channel.
type stoppableCache struct {
	cache.Cache

	lock       sync.Mutex
	stopped    bool
	cancelFunc context.CancelFunc
}

// Stop cancels the cache.Cache's context, unless it has already been stopped.
func (cc *stoppableCache) Stop() {
	cc.lock.Lock()
	defer cc.lock.Unlock()

	if cc.stopped {
		return
	}

	cc.stopped = true
	cc.cancelFunc()
}
