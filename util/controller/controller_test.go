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

package controller

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/util/cache"
)

func TestReconcile(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ReconcilerRateLimiting, true)

	// reconcileCache has to be created outside synctest.Test, otherwise
	// the test would fail because of the cleanup go routine in the cache.
	// Note: synctest.Test below will run with a fake clock, so it will add
	// entries to the cache with a timestamp ~ 2020. We are using a high expiration
	// time here so that the cache does not expire our entries during the test run.
	reconcileCache := cache.New[reconcileCacheEntry](250 * 365 * 24 * time.Hour) // 250 years

	synctest.Test(t, func(t *testing.T) {
		g := NewWithT(t)

		rateLimitInterval := 1 * time.Second

		var reconcileCounter atomic.Int64
		r := &reconcilerWrapper{
			name:           "cluster",
			reconcileCache: reconcileCache,
			reconciler: reconcile.Func(func(_ context.Context, _ reconcile.Request) (reconcile.Result, error) {
				reconcileCounter.Add(1)
				return reconcile.Result{}, nil
			}),
			rateLimitInterval: rateLimitInterval,
			queueRateLimiter:  newTypedItemExponentialFailureRateLimiter[reconcile.Request](rateLimitInterval, 5*time.Millisecond, 1000*time.Second),
		}
		c := controllerWrapper{
			reconcileCache: reconcileCache,
		}

		// Setup an entire controller so that we can also test how the controller interacts with the queue during exponential backoff.
		ctrl, err := controller.NewTypedUnmanaged("cluster", controller.Options{
			Reconciler:  r,
			RateLimiter: r.queueRateLimiter,
		})
		g.Expect(err).ToNot(HaveOccurred())
		sourceChannel := make(chan event.GenericEvent, 1)
		defer func() {
			close(sourceChannel)
		}()
		g.Expect(ctrl.Watch(source.Channel(sourceChannel, handler.Funcs{
			GenericFunc: func(_ context.Context, e event.TypedGenericEvent[client.Object], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				q.Add(reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: e.Object.GetNamespace(),
						Name:      e.Object.GetName(),
					},
				})
			},
		}))).To(Succeed())

		ctrlCtx, ctrlCancel := context.WithCancel(t.Context())
		defer ctrlCancel()
		go func() {
			_ = ctrl.Start(ctrlCtx)
		}()

		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: metav1.NamespaceDefault,
				Name:      "cluster-1",
			},
		}
		req := reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(cluster),
		}
		genericEvent := event.TypedGenericEvent[client.Object]{
			Object: cluster,
		}

		// Reconcile will reconcile and defer next reconcile by 1s.
		res, err := r.Reconcile(t.Context(), req)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.IsZero()).To(BeTrue())
		g.Expect(reconcileCounter.Load()).To(Equal(int64(1)))
		g.Expect(counterMetricValue(reconcileTotal.WithLabelValues(r.name, labelSuccess))).To(Equal(1))

		// Reconcile will not reconcile and return RequeueAfter 1s.
		res, err = r.Reconcile(t.Context(), req)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.RequeueAfter).To(Equal(1 * time.Second))
		g.Expect(reconcileCounter.Load()).To(Equal(int64(1)))
		g.Expect(counterMetricValue(reconcileTotal.WithLabelValues(r.name, labelSuccess))).To(Equal(1))

		time.Sleep(1 * time.Second)

		// Reconcile will reconcile and defer next reconcile by 1s.
		res, err = r.Reconcile(t.Context(), req)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.IsZero()).To(BeTrue())
		g.Expect(reconcileCounter.Load()).To(Equal(int64(2)))
		g.Expect(counterMetricValue(reconcileTotal.WithLabelValues(r.name, labelSuccess))).To(Equal(2))

		// Defer next Reconcile by 11s.
		c.DeferNextReconcile(req, time.Now().Add(11*time.Second))

		// Reconcile will not reconcile and return RequeueAfter 11s.
		res, err = r.Reconcile(t.Context(), req)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.RequeueAfter).To(Equal(11 * time.Second))
		g.Expect(reconcileCounter.Load()).To(Equal(int64(2)))
		g.Expect(counterMetricValue(reconcileTotal.WithLabelValues(r.name, labelSuccess))).To(Equal(2))

		time.Sleep(4 * time.Second)

		// Reconcile will not reconcile and return RequeueAfter 7s (== 11s-4s).
		res, err = r.Reconcile(t.Context(), req)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.RequeueAfter).To(Equal(7 * time.Second))
		g.Expect(reconcileCounter.Load()).To(Equal(int64(2)))
		g.Expect(counterMetricValue(reconcileTotal.WithLabelValues(r.name, labelSuccess))).To(Equal(2))

		time.Sleep(7 * time.Second)

		// Reconcile will reconcile and defer next reconcile by 1s.
		res, err = r.Reconcile(t.Context(), req)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.IsZero()).To(BeTrue())
		g.Expect(reconcileCounter.Load()).To(Equal(int64(3)))
		g.Expect(counterMetricValue(reconcileTotal.WithLabelValues(r.name, labelSuccess))).To(Equal(3))

		// Defer next Reconcile by 55s.
		c.DeferNextReconcileForObject(cluster, time.Now().Add(55*time.Second))

		// Reconcile will not reconcile and return RequeueAfter 55s.
		res, err = r.Reconcile(t.Context(), req)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.RequeueAfter).To(Equal(55 * time.Second))
		g.Expect(reconcileCounter.Load()).To(Equal(int64(3)))
		g.Expect(counterMetricValue(reconcileTotal.WithLabelValues(r.name, labelSuccess))).To(Equal(3))

		time.Sleep(55 * time.Second)

		// Reconcile will reconcile and return RequeueAfter 1s (always at least rateLimitInterval)
		r.reconciler = reconcile.Func(func(_ context.Context, _ reconcile.Request) (reconcile.Result, error) {
			reconcileCounter.Add(1)
			return reconcile.Result{RequeueAfter: 1 * time.Millisecond}, nil // RequeueAfter should be at least rateLimitInterval (1s)
		})
		res, err = r.Reconcile(t.Context(), req)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.RequeueAfter).To(Equal(rateLimitInterval))
		g.Expect(reconcileCounter.Load()).To(Equal(int64(4)))
		g.Expect(counterMetricValue(reconcileTotal.WithLabelValues(r.name, labelRequeueAfter))).To(Equal(1))

		time.Sleep(rateLimitInterval)

		// Reconcile will reconcile and return RequeueAfter 5s (always at least rate-limiting duration)
		r.reconciler = reconcile.Func(func(_ context.Context, _ reconcile.Request) (reconcile.Result, error) {
			reconcileCounter.Add(1)
			c.DeferNextReconcileForObject(cluster, time.Now().Add(5*time.Second))
			return reconcile.Result{RequeueAfter: 1 * time.Millisecond}, nil // RequeueAfter should be at least rate-limiting duration (5s)
		})
		res, err = r.Reconcile(t.Context(), req)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.RequeueAfter).To(Equal(5 * time.Second))
		g.Expect(reconcileCounter.Load()).To(Equal(int64(5)))
		g.Expect(counterMetricValue(reconcileTotal.WithLabelValues(r.name, labelRequeueAfter))).To(Equal(2))

		time.Sleep(5 * time.Second)

		// Reconcile will reconcile and return error which will trigger exponential backoff (rate-limiting should not interfere)
		// This test is using the sourceChannel to include the controller & the priority queue because this is the only way
		// to test the exponential backoff that is computed by the priority queue.
		errorToReturn := errors.New("error")
		var errorLock sync.Mutex
		r.reconciler = reconcile.Func(func(_ context.Context, _ reconcile.Request) (reconcile.Result, error) {
			reconcileCounter.Add(1)
			errorLock.Lock()
			defer errorLock.Unlock()
			return reconcile.Result{}, errorToReturn
		})
		// Put the event into the channel
		sourceChannel <- genericEvent
		// Wait for go routines to handle the event & verify that counters & rateLimiter go up as expected
		synctest.Wait()
		g.Expect(reconcileCounter.Load()).To(Equal(int64(6)))
		g.Expect(counterMetricValue(reconcileTotal.WithLabelValues(r.name, labelError))).To(Equal(1))
		g.Expect(r.queueRateLimiter.NumRequeues(req)).To(Equal(1))
		// Wait for exp. backoff & go routines to handle the event & verify that counters & rateLimiter go up as expected
		time.Sleep(rateLimitInterval + 5*time.Millisecond)
		synctest.Wait()
		g.Expect(reconcileCounter.Load()).To(Equal(int64(7)))
		g.Expect(counterMetricValue(reconcileTotal.WithLabelValues(r.name, labelError))).To(Equal(2))
		g.Expect(r.queueRateLimiter.NumRequeues(req)).To(Equal(2))
		// Now make the Reconcile func succeed.
		errorLock.Lock()
		errorToReturn = nil
		errorLock.Unlock()
		// Wait for exp. backoff & go routines to handle the event & verify that counters go up & rateLimiter gets resetted.
		time.Sleep(rateLimitInterval + 10*time.Millisecond)
		synctest.Wait()
		g.Expect(reconcileCounter.Load()).To(Equal(int64(8)))
		g.Expect(counterMetricValue(reconcileTotal.WithLabelValues(r.name, labelSuccess))).To(Equal(4))
		g.Expect(r.queueRateLimiter.NumRequeues(req)).To(Equal(0))

		ctrlCancel()
	})
}

func TestReconcileMetrics(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ReconcilerRateLimiting, true)

	// reconcileCache has to be created outside synctest.Test, otherwise
	// the test would fail because of the cleanup go routine in the cache.
	reconcileCache := cache.New[reconcileCacheEntry](cache.DefaultTTL)

	synctest.Test(t, func(t *testing.T) {
		g := NewWithT(t)

		rateLimitInterval := 1 * time.Second

		r := reconcilerWrapper{
			name:              "cluster",
			reconcileCache:    reconcileCache,
			rateLimitInterval: rateLimitInterval,
			queueRateLimiter:  newTypedItemExponentialFailureRateLimiter[reconcile.Request](rateLimitInterval, 5*time.Millisecond, 1000*time.Second),
		}

		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: metav1.NamespaceDefault,
				Name:      "cluster-1",
			},
		}

		// Reset the metrics (otherwise they have data from other tests).
		reconcileTotal.Reset()
		reconcileTime.Reset()

		// Success
		r.reconciler = reconcile.Func(func(_ context.Context, _ reconcile.Request) (reconcile.Result, error) {
			return reconcile.Result{}, nil
		})
		res, err := r.Reconcile(t.Context(), req)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.IsZero()).To(BeTrue())
		g.Expect(counterMetricValue(reconcileTotal.WithLabelValues(r.name, labelError))).To(Equal(0))
		g.Expect(counterMetricValue(reconcileTotal.WithLabelValues(r.name, labelRequeueAfter))).To(Equal(0))
		g.Expect(counterMetricValue(reconcileTotal.WithLabelValues(r.name, labelRequeue))).To(Equal(0))
		g.Expect(counterMetricValue(reconcileTotal.WithLabelValues(r.name, labelSuccess))).To(Equal(1))
		g.Expect(histogramMetricValue(reconcileTime.WithLabelValues(r.name))).To(Equal(1))

		time.Sleep(1 * time.Second)

		// Error
		r.reconciler = reconcile.Func(func(_ context.Context, _ reconcile.Request) (reconcile.Result, error) {
			return reconcile.Result{RequeueAfter: 5 * time.Second}, errors.New("error") // RequeueAfter should be dropped
		})
		res, err = r.Reconcile(t.Context(), req)
		g.Expect(err).To(HaveOccurred())
		g.Expect(res.RequeueAfter).To(Equal(time.Duration(0)))
		g.Expect(res.Requeue).To(BeFalse()) //nolint:staticcheck // We have to handle Requeue until it is removed
		g.Expect(counterMetricValue(reconcileTotal.WithLabelValues(r.name, labelError))).To(Equal(1))
		g.Expect(counterMetricValue(reconcileTotal.WithLabelValues(r.name, labelRequeueAfter))).To(Equal(0))
		g.Expect(counterMetricValue(reconcileTotal.WithLabelValues(r.name, labelRequeue))).To(Equal(0))
		g.Expect(counterMetricValue(reconcileTotal.WithLabelValues(r.name, labelSuccess))).To(Equal(1))
		g.Expect(histogramMetricValue(reconcileTime.WithLabelValues(r.name))).To(Equal(2))

		time.Sleep(1 * time.Second)

		// RequeueAfter
		r.reconciler = reconcile.Func(func(_ context.Context, _ reconcile.Request) (reconcile.Result, error) {
			return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
		})
		res, err = r.Reconcile(t.Context(), req)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.RequeueAfter).To(Equal(5 * time.Second))
		g.Expect(res.Requeue).To(BeFalse()) //nolint:staticcheck // We have to handle Requeue until it is removed
		g.Expect(counterMetricValue(reconcileTotal.WithLabelValues(r.name, labelError))).To(Equal(1))
		g.Expect(counterMetricValue(reconcileTotal.WithLabelValues(r.name, labelRequeueAfter))).To(Equal(1))
		g.Expect(counterMetricValue(reconcileTotal.WithLabelValues(r.name, labelRequeue))).To(Equal(0))
		g.Expect(counterMetricValue(reconcileTotal.WithLabelValues(r.name, labelSuccess))).To(Equal(1))
		g.Expect(histogramMetricValue(reconcileTime.WithLabelValues(r.name))).To(Equal(3))

		time.Sleep(1 * time.Second)

		// Requeue
		r.reconciler = reconcile.Func(func(_ context.Context, _ reconcile.Request) (reconcile.Result, error) {
			return reconcile.Result{Requeue: true}, nil
		})
		res, err = r.Reconcile(t.Context(), req)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.RequeueAfter).To(Equal(time.Duration(0)))
		g.Expect(res.Requeue).To(BeTrue()) //nolint:staticcheck // We have to handle Requeue until it is removed
		g.Expect(counterMetricValue(reconcileTotal.WithLabelValues(r.name, labelError))).To(Equal(1))
		g.Expect(counterMetricValue(reconcileTotal.WithLabelValues(r.name, labelRequeueAfter))).To(Equal(1))
		g.Expect(counterMetricValue(reconcileTotal.WithLabelValues(r.name, labelRequeue))).To(Equal(1))
		g.Expect(counterMetricValue(reconcileTotal.WithLabelValues(r.name, labelSuccess))).To(Equal(1))
		g.Expect(histogramMetricValue(reconcileTime.WithLabelValues(r.name))).To(Equal(4))
	})
}

func counterMetricValue(values prometheus.Counter) int {
	var dto dto.Metric
	if err := values.Write(&dto); err != nil {
		panic(err)
	}
	return int(*dto.GetCounter().Value)
}

func histogramMetricValue(values prometheus.Observer) int {
	var dto dto.Metric
	if err := values.(prometheus.Histogram).Write(&dto); err != nil {
		panic(err)
	}
	return int(dto.GetHistogram().GetSampleCount())
}

func TestShouldRequeue(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name             string
		now              time.Time
		reconcileAfter   time.Time
		wantRequeue      bool
		wantRequeueAfter time.Duration
	}{
		{
			name:             "Don't requeue, reconcileAfter is zero",
			now:              now,
			reconcileAfter:   time.Time{},
			wantRequeue:      false,
			wantRequeueAfter: time.Duration(0),
		},
		{
			name:             "Requeue after 15s",
			now:              now,
			reconcileAfter:   now.Add(time.Duration(15) * time.Second),
			wantRequeue:      true,
			wantRequeueAfter: time.Duration(15) * time.Second,
		},
		{
			name:             "Don't requeue, reconcileAfter is now",
			now:              now,
			reconcileAfter:   now,
			wantRequeue:      false,
			wantRequeueAfter: time.Duration(0),
		},
		{
			name:             "Don't requeue, reconcileAfter is before now",
			now:              now,
			reconcileAfter:   now.Add(-time.Duration(60) * time.Second),
			wantRequeue:      false,
			wantRequeueAfter: time.Duration(0),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			gotRequeueAfter, gotRequeue := reconcileCacheEntry{ReconcileAfter: tt.reconcileAfter}.ShouldRequeue(tt.now)
			g.Expect(gotRequeue).To(Equal(tt.wantRequeue))
			g.Expect(gotRequeueAfter).To(Equal(tt.wantRequeueAfter))
		})
	}
}
