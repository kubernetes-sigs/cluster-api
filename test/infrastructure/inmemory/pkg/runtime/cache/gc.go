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
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
)

type gcRequest struct {
	resourceGroup string
	gvk           schema.GroupVersionKind
	key           types.NamespacedName
}

func (c *cache) startGarbageCollector(ctx context.Context) error {
	log := ctrl.LoggerFrom(ctx).WithValues("controller", "gc") // TODO: consider if to use something different than controller
	ctx = ctrl.LoggerInto(ctx, log)

	log.Info("Starting garbage collector queue")
	c.garbageCollectorQueue = workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[any]())
	go func() {
		<-ctx.Done()
		c.garbageCollectorQueue.ShutDown()
	}()

	var workers atomic.Int64
	go func() {
		log.Info("Starting garbage collector workers", "count", c.garbageCollectorConcurrency)
		wg := &sync.WaitGroup{}
		wg.Add(c.garbageCollectorConcurrency)
		for range c.garbageCollectorConcurrency {
			go func() {
				workers.Add(1)
				defer wg.Done()
				for c.processGarbageCollectorWorkItem(ctx) {
				}
			}()
		}
		<-ctx.Done()
		wg.Wait()
	}()

	if err := wait.PollUntilContextTimeout(ctx, 50*time.Millisecond, 5*time.Second, false, func(context.Context) (done bool, err error) {
		if workers.Load() < int64(c.garbageCollectorConcurrency) {
			return false, nil
		}
		return true, nil
	}); err != nil {
		return fmt.Errorf("failed to start garbage collector workers: %v", err)
	}
	return nil
}

func (c *cache) processGarbageCollectorWorkItem(ctx context.Context) bool {
	log := ctrl.LoggerFrom(ctx)

	item, shutdown := c.garbageCollectorQueue.Get()
	if shutdown {
		return false
	}

	// TODO(Fabrizio): Why are we calling the same in defer and directly
	defer func() {
		c.garbageCollectorQueue.Done(item)
	}()
	c.garbageCollectorQueue.Done(item)

	gcr, ok := item.(gcRequest)
	if !ok {
		c.garbageCollectorQueue.Forget(item)
		return true
	}

	deleted, err := c.tryDelete(gcr.resourceGroup, gcr.gvk, gcr.key)
	if err != nil {
		log.Error(err, "Error garbage collecting object", "resourceGroup", gcr.resourceGroup, gcr.gvk.Kind, gcr.key)
	}

	if err == nil && deleted {
		c.garbageCollectorQueue.Forget(item)
		log.Info("Object garbage collected", "resourceGroup", gcr.resourceGroup, gcr.gvk.Kind, gcr.key)
		return true
	}

	c.garbageCollectorQueue.Forget(item)

	requeueAfter := wait.Jitter(c.garbageCollectorRequeueAfter, c.garbageCollectorRequeueAfterJitterFactor)
	c.garbageCollectorQueue.AddAfter(item, requeueAfter)
	return true
}
