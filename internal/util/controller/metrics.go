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
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// Note: Additional metrics have been added because the ReconcileTotal & ReconcileTime
// controller-runtime metrics are also counting the Requeues for rate-limiting.
// Note: The following controller-runtime metrics are correct even with rate-limiting:
// ReconcileErrors, TerminalReconcileErrors, ReconcilePanics, WorkerCount, ActiveWorkers.

var (
	// reconcileTotal is a prometheus counter metrics which holds the total
	// number of reconciliations per controller. It has two labels. controller label refers
	// to the controller name and result label refers to the reconcile result i.e.
	// success, error, requeue, requeue_after.
	// Note: The difference between the CAPI and the controller-runtime metric is that the
	//       CAPI metric excludes rate-limiting related reconciles.
	reconcileTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "capi_reconcile_total",
		Help: "Total number of reconciliations per controller",
	}, []string{"controller", "result"})

	// reconcileTime is a prometheus metric which keeps track of the duration
	// of reconciliations.
	// Note: The difference between the CAPI and the controller-runtime metric is that the
	//       CAPI metric excludes rate-limiting related reconciles.
	reconcileTime = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "capi_reconcile_time_seconds",
		Help: "Length of time per reconciliation per controller",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.15, 0.2, 0.25, 0.3, 0.35, 0.4, 0.45, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0,
			1.25, 1.5, 1.75, 2.0, 2.5, 3.0, 3.5, 4.0, 4.5, 5, 6, 7, 8, 9, 10, 15, 20, 25, 30, 40, 50, 60},
		NativeHistogramBucketFactor:     1.1,
		NativeHistogramMaxBucketNumber:  100,
		NativeHistogramMinResetDuration: 1 * time.Hour,
	}, []string{"controller"})
)

const (
	labelError        = "error"
	labelRequeueAfter = "requeue_after"
	labelRequeue      = "requeue"
	labelSuccess      = "success"
)

func init() {
	metrics.Registry.MustRegister(reconcileTotal, reconcileTime)
}
