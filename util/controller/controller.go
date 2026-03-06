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
	"errors"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/priorityqueue"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/util/cache"
)

type reconcilerWrapper struct {
	name           string
	reconcileCache cache.Cache[reconcileCacheEntry]
	reconciler     reconcile.Reconciler
	Queue          *trackingPriorityQueue
	rateLimiter    workqueue.TypedRateLimiter[reconcile.Request]
}

const atMostReconcileEvery = 1 * time.Second

func (r *reconcilerWrapper) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	reconcileStartTime := time.Now()

	var priority *int
	if p, ok := r.Queue.PriorityMap.Load(req); ok {
		if pInt, ok := p.(int); ok {
			priority = ptr.To(pInt)
		}
	}

	// Check reconcileCache to ensure we won't run reconcile too frequently.
	if cacheEntry, ok := r.reconcileCache.Has(reconcileCacheEntry{Request: req}.Key()); ok {
		if requeueAfter, requeue := cacheEntry.ShouldRequeue(reconcileStartTime); requeue {
			r.Queue.AddWithOpts(priorityqueue.AddOpts{After: requeueAfter, Priority: priority}, req)
			return ctrl.Result{}, nil
		}
	}

	log := ctrl.LoggerFrom(ctx)

	if feature.Gates.Enabled(feature.ReconcilerRateLimiting) { // FIXME: figure out what to do with the feature gate
		// Add entry to the reconcileCache so we won't run Reconcile more than once per second.
		// Under certain circumstances the ReconcileAfter time will be set to a later time via DeferNextReconcile /
		// DeferNextReconcileForObject, e.g. when we're waiting for Pods to terminate during node drain or
		// volumes to detach. This is done to ensure we're not spamming the workload cluster API server.
		r.reconcileCache.Add(reconcileCacheEntry{Request: req, ReconcileAfter: reconcileStartTime.Add(atMostReconcileEvery)})
	}

	// Update metrics after processing each item
	defer func() {
		reconcileTime.WithLabelValues(r.name).Observe(time.Since(reconcileStartTime).Seconds())
	}()

	result, err := r.reconciler.Reconcile(ctx, req) // FIXME(verify): check that the priority is actually preserved via the trackingPriorityQueue
	if result.Priority != nil {
		priority = result.Priority
	}

	// TODO: surface details of the requeueAfter computation in the log statements below (also for error etc.)
	minimumRequeueAfter := atMostReconcileEvery
	if cacheEntry, ok := r.reconcileCache.Has(reconcileCacheEntry{Request: req}.Key()); ok {
		if requeueAfter, requeue := cacheEntry.ShouldRequeue(time.Now()); requeue {
			minimumRequeueAfter = requeueAfter
		}
	}

	switch {
	case err != nil:
		if errors.Is(err, reconcile.TerminalError(nil)) { // FIXME: add logs etc
			// ctrlmetrics.TerminalReconcileErrors.WithLabelValues(c.Name).Inc()
		} else {
			requeueAfter := max(atMostReconcileEvery+r.rateLimiter.When(req), minimumRequeueAfter)
			r.Queue.AddWithOpts(priorityqueue.AddOpts{After: requeueAfter, Priority: priority}, req)
		}
		//ctrlmetrics.ReconcileErrors.WithLabelValues(c.Name).Inc()
		reconcileTotal.WithLabelValues(r.name, labelError).Inc()
		log.Error(err, "Reconciler error")
	case result.RequeueAfter > 0:
		requeueAfter := max(result.RequeueAfter, minimumRequeueAfter)
		log.V(5).Info(fmt.Sprintf("Reconcile done, requeueing after %s", requeueAfter))
		// The result.RequeueAfter request will be lost, if it is returned
		// along with a non-nil error. But this is intended as
		// We need to drive to stable reconcile loops before queuing due
		// to result.RequestAfter
		r.rateLimiter.Forget(req)
		r.Queue.AddWithOpts(priorityqueue.AddOpts{After: requeueAfter, Priority: priority}, req)
		reconcileTotal.WithLabelValues(r.name, labelRequeueAfter).Inc()
	case result.Requeue: //nolint: staticcheck // We have to handle Requeue until it is removed
		log.V(5).Info("Reconcile done, requeueing")
		requeueAfter := max(atMostReconcileEvery+r.rateLimiter.When(req), minimumRequeueAfter)
		r.Queue.AddWithOpts(priorityqueue.AddOpts{After: requeueAfter, Priority: priority}, req)
		reconcileTotal.WithLabelValues(r.name, labelRequeue).Inc()
	default:
		log.V(5).Info("Reconcile successful")
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		r.rateLimiter.Forget(req)
		reconcileTotal.WithLabelValues(r.name, labelSuccess).Inc()
	}

	return ctrl.Result{}, nil
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

// Key returns the cache key of a reconcileCacheEntry. // TODO maybe make this pointer receiver
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
