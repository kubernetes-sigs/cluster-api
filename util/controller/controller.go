/*
Copyright 2025 The Kubernetes Authors.

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

// Package controller provides utils for controller-runtime.
package controller

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/util/cache"
)

type reconcilerWrapper struct {
	name              string
	reconcileCache    cache.Cache[reconcileCacheEntry]
	reconciler        reconcile.Reconciler
	rateLimitInterval time.Duration
	queueRateLimiter  *typedItemExponentialFailureRateLimiter[reconcile.Request]
}

func (r *reconcilerWrapper) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	if !feature.Gates.Enabled(feature.ReconcilerRateLimiting) {
		return r.reconciler.Reconcile(ctx, req)
	}

	reconcileStartTime := time.Now()

	// Check reconcileCache to ensure we won't run reconcile too frequently.
	if cacheEntry, ok := r.reconcileCache.Has(reconcileCacheEntry{Request: req}.Key()); ok {
		if requeueAfter, requeue := cacheEntry.ShouldRequeue(reconcileStartTime); requeue {
			return ctrl.Result{RequeueAfter: requeueAfter}, nil
		}
	}

	// Add entry to the reconcileCache so we won't run Reconcile more than once per second.
	// Under certain circumstances the ReconcileAfter time will be set to a later time via DeferNextReconcile /
	// DeferNextReconcileForObject, e.g. when we're waiting for Pods to terminate during node drain or
	// volumes to detach. This is done to ensure we're not spamming the workload cluster API server.
	r.reconcileCache.Add(reconcileCacheEntry{Request: req, ReconcileAfter: reconcileStartTime.Add(r.rateLimitInterval)})

	// Update metrics after processing each item
	defer func() {
		reconcileTime.WithLabelValues(r.name).Observe(time.Since(reconcileStartTime).Seconds())
	}()

	result, err := r.reconciler.Reconcile(ctx, req)
	if err != nil {
		// Note: controller-runtime logs a warning if an error is returned in combination with
		// RequeueAfter / Requeue. Dropping RequeueAfter and Requeue here to avoid this warning
		// (while preserving Priority).
		result.RequeueAfter = 0
		result.Requeue = false //nolint:staticcheck // We have to handle Requeue until it is removed
	}
	switch {
	case err != nil:
		reconcileTotal.WithLabelValues(r.name, labelError).Inc()
	case result.RequeueAfter > 0:
		// It does not make sense to use a requeueAfter that is lower than the rate-limiting enforced via
		// the reconcileCache, so we use that as a minimum.
		// Note: Evaluating this here includes the reconcileCache entry set above, but also the entry that
		// might have been added during Reconcile via DeferNextReconcile / DeferNextReconcileForObject.
		// TODO: It would also be possible to extend r.queueRateLimiter to set a request-specific minimum requeueAfter.
		// This would allow us to also enforce a minimum requeueAfter for the err != nil and Requeue cases.
		minimumRequeueAfter := r.rateLimitInterval
		if cacheEntry, ok := r.reconcileCache.Has(reconcileCacheEntry{Request: req}.Key()); ok {
			if requeueAfter, requeue := cacheEntry.ShouldRequeue(time.Now()); requeue {
				minimumRequeueAfter = requeueAfter
			}
		}
		result.RequeueAfter = max(result.RequeueAfter, minimumRequeueAfter)
		r.queueRateLimiter.ActualForget(req)
		reconcileTotal.WithLabelValues(r.name, labelRequeueAfter).Inc()
	case result.Requeue: //nolint: staticcheck // We have to handle Requeue until it is removed
		reconcileTotal.WithLabelValues(r.name, labelRequeue).Inc()
	default:
		r.queueRateLimiter.ActualForget(req)
		reconcileTotal.WithLabelValues(r.name, labelSuccess).Inc()
	}

	return result, err
}

type controllerWrapper struct {
	controller.TypedController[reconcile.Request]
	reconcileCache cache.Cache[reconcileCacheEntry]
}

func (c *controllerWrapper) DeferNextReconcile(req reconcile.Request, reconcileAfter time.Time) {
	c.reconcileCache.Add(reconcileCacheEntry{
		Request:        req,
		ReconcileAfter: reconcileAfter,
	})
}

func (c *controllerWrapper) DeferNextReconcileForObject(obj metav1.Object, reconcileAfter time.Time) {
	c.DeferNextReconcile(reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: obj.GetNamespace(),
			Name:      obj.GetName(),
		}}, reconcileAfter)
}

// reconcileCacheEntry is an Entry for the Cache that stores the
// earliest time after which the next Reconcile should be executed.
type reconcileCacheEntry struct {
	Request        reconcile.Request
	ReconcileAfter time.Time
}

var _ cache.Entry = &reconcileCacheEntry{}

// Key returns the cache key of a reconcileCacheEntry.
func (r reconcileCacheEntry) Key() string {
	return r.Request.String()
}

// ShouldRequeue returns if the current Reconcile should be requeued.
func (r reconcileCacheEntry) ShouldRequeue(now time.Time) (requeueAfter time.Duration, requeue bool) {
	if r.ReconcileAfter.IsZero() {
		return time.Duration(0), false
	}

	if r.ReconcileAfter.After(now) {
		return r.ReconcileAfter.Sub(now), true
	}

	return time.Duration(0), false
}
