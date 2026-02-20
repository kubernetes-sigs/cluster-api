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
	"testing"
	"testing/synctest"
	"time"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

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

		var reconcileCounter int
		r := reconcilerWrapper{
			name:           "cluster",
			reconcileCache: reconcileCache,
			reconciler: reconcile.Func(func(_ context.Context, _ reconcile.Request) (reconcile.Result, error) {
				reconcileCounter++
				return reconcile.Result{}, nil
			}),
		}
		c := controllerWrapper{
			reconcileCache: reconcileCache,
		}

		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: metav1.NamespaceDefault,
				Name:      "cluster-1",
			},
		}
		req := reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(cluster),
		}

		// Reconcile will reconcile and defer next reconcile by 1s.
		res, err := r.Reconcile(t.Context(), req)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.IsZero()).To(BeTrue())
		g.Expect(reconcileCounter).To(Equal(1))
		g.Expect(counterMetricValue(reconcileTotal.WithLabelValues(r.name, labelSuccess))).To(Equal(1))

		// Reconcile will not reconcile and return RequeueAfter 1s.
		res, err = r.Reconcile(t.Context(), req)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.RequeueAfter).To(Equal(1 * time.Second))
		g.Expect(reconcileCounter).To(Equal(1))
		g.Expect(counterMetricValue(reconcileTotal.WithLabelValues(r.name, labelSuccess))).To(Equal(1))

		time.Sleep(1 * time.Second)

		// Reconcile will reconcile and defer next reconcile by 1s.
		res, err = r.Reconcile(t.Context(), req)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.IsZero()).To(BeTrue())
		g.Expect(reconcileCounter).To(Equal(2))
		g.Expect(counterMetricValue(reconcileTotal.WithLabelValues(r.name, labelSuccess))).To(Equal(2))

		// Defer next Reconcile by 11s.
		c.DeferNextReconcile(req, time.Now().Add(11*time.Second))

		// Reconcile will not reconcile and return RequeueAfter 11s.
		res, err = r.Reconcile(t.Context(), req)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.RequeueAfter).To(Equal(11 * time.Second))
		g.Expect(reconcileCounter).To(Equal(2))
		g.Expect(counterMetricValue(reconcileTotal.WithLabelValues(r.name, labelSuccess))).To(Equal(2))

		time.Sleep(4 * time.Second)

		// Reconcile will not reconcile and return RequeueAfter 7s (== 11s-4s).
		res, err = r.Reconcile(t.Context(), req)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.RequeueAfter).To(Equal(7 * time.Second))
		g.Expect(reconcileCounter).To(Equal(2))
		g.Expect(counterMetricValue(reconcileTotal.WithLabelValues(r.name, labelSuccess))).To(Equal(2))

		time.Sleep(7 * time.Second)

		// Reconcile will reconcile and defer next reconcile by 1s.
		res, err = r.Reconcile(t.Context(), req)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.IsZero()).To(BeTrue())
		g.Expect(reconcileCounter).To(Equal(3))
		g.Expect(counterMetricValue(reconcileTotal.WithLabelValues(r.name, labelSuccess))).To(Equal(3))

		// Defer next Reconcile by 55s.
		c.DeferNextReconcileForObject(cluster, time.Now().Add(55*time.Second))

		// Reconcile will not reconcile and return RequeueAfter 55s.
		res, err = r.Reconcile(t.Context(), req)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.RequeueAfter).To(Equal(55 * time.Second))
		g.Expect(reconcileCounter).To(Equal(3))
		g.Expect(counterMetricValue(reconcileTotal.WithLabelValues(r.name, labelSuccess))).To(Equal(3))
	})
}

func TestReconcileMetrics(t *testing.T) {
	// Note: Feature gate is intentionally turned off for additional test coverage and to avoid
	// having to move the clock forward by 1s after every Reconcile call.
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ReconcilerRateLimiting, false)

	// reconcileCache has to be created outside synctest.Test, otherwise
	// the test would fail because of the cleanup go routine in the cache.
	reconcileCache := cache.New[reconcileCacheEntry](cache.DefaultTTL)

	synctest.Test(t, func(t *testing.T) {
		g := NewWithT(t)

		r := reconcilerWrapper{
			name:           "cluster",
			reconcileCache: reconcileCache,
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
