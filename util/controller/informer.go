/*
Copyright 2026 The Kubernetes Authors.

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
	"strings"
	"sync"
	"time"

	"github.com/gobuffalo/flect"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// NewInformerFunc provides a new informer func that can be used for cache.Options that configures the
// informer with an identifier and an informerMetricsProvider for proper logs and metrics.
func NewInformerFunc(scheme *runtime.Scheme, controllerName string) func(cache.ListerWatcher, runtime.Object, time.Duration, cache.Indexers) cache.SharedIndexInformer {
	informerName, err := cache.NewInformerName(controllerName)
	if err != nil {
		panic("cache.NewInformerName was called twice with the same name, that should never happen")
	}

	return func(lw cache.ListerWatcher, exampleObject runtime.Object, defaultEventHandlerResyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
		// controller-runtime is using cache.NewSharedIndexInformer per default. This is duplicating
		// cache.NewSharedIndexInformer and additionally setting the Identifier and InformerMetricsProvider
		// options which are used for logging and metrics (e.g. the store_resource_version and fifo metrics).
		opts := cache.SharedIndexInformerOptions{
			ResyncPeriod: defaultEventHandlerResyncPeriod,
			Indexers:     indexers,
		}
		if gvk, err := apiutil.GVKForObject(exampleObject, scheme); err == nil {
			opts.Identifier = informerName.WithResource(schema.GroupVersionResource{
				Group:   gvk.Group,
				Version: gvk.Version,
				// Best effort, want to avoid using a restMapper here to get the correct resource.
				Resource: flect.Pluralize(strings.ToLower(gvk.Kind)),
			})
			opts.InformerMetricsProvider = informerMetricsProvider{}
		}
		return cache.NewSharedIndexInformerWithOptions(lw, exampleObject, opts)
	}
}

// Note: These metrics have been initially copied from: https://github.com/kubernetes/component-base/blob/v0.36.0/metrics/prometheus/clientgo/fifo/metrics.go.

var (
	subsystem = "informer"

	fifoQueuedItems = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: subsystem,
			Name:      "queued_items",
			Help:      "Number of items currently queued in the FIFO.",
		},
		[]string{"name", "group", "version", "resource"},
	)
	fifoProcessingLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Subsystem:                       subsystem,
			Name:                            "processing_latency_seconds",
			Help:                            "Time taken to process events after popping from the queue.",
			Buckets:                         []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: 1 * time.Hour,
		},
		[]string{"name", "group", "version", "resource"},
	)
	storeResourceVersion = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: "informer",
			Name:      "store_resource_version",
			Help:      "The 15 least significant digits of the resource version of the store.",
		},
		[]string{"name", "group", "version", "resource"},
	)

	registerOnce sync.Once
)

func init() {
	register()
}

// Register registers FIFO metrics and sets the metrics provider.
func register() {
	registerOnce.Do(func() {
		metrics.Registry.MustRegister(fifoQueuedItems)
		metrics.Registry.MustRegister(fifoProcessingLatency)
		metrics.Registry.MustRegister(storeResourceVersion)
	})
}

type informerMetricsProvider struct{}

func (informerMetricsProvider) NewQueuedItemMetric(id cache.InformerNameAndResource) cache.GaugeMetric {
	return &reservedGaugeMetric{
		id: id,
		gauge: fifoQueuedItems.WithLabelValues(
			id.Name(),
			id.GroupVersionResource().Group,
			id.GroupVersionResource().Version,
			id.GroupVersionResource().Resource,
		),
	}
}

func (informerMetricsProvider) NewProcessingLatencyMetric(id cache.InformerNameAndResource) cache.HistogramMetric {
	return &reservedHistogramMetric{
		id: id,
		histogram: fifoProcessingLatency.WithLabelValues(
			id.Name(),
			id.GroupVersionResource().Group,
			id.GroupVersionResource().Version,
			id.GroupVersionResource().Resource,
		),
	}
}

func (informerMetricsProvider) NewStoreResourceVersionMetric(id cache.InformerNameAndResource) cache.GaugeMetric {
	return &reservedGaugeMetric{
		id: id,
		gauge: storeResourceVersion.WithLabelValues(
			id.Name(),
			id.GroupVersionResource().Group,
			id.GroupVersionResource().Version,
			id.GroupVersionResource().Resource,
		),
	}
}

// reservedGaugeMetric wraps a gauge and only updates it if the identifier
// is still reserved. This supports dynamic informers (e.g., GC, ResourceQuota)
// that may shut down while the process is still running.
type reservedGaugeMetric struct {
	id    cache.InformerNameAndResource
	gauge cache.GaugeMetric
}

func (r *reservedGaugeMetric) Set(value float64) {
	if r.id.Reserved() {
		r.gauge.Set(value)
	}
}

// reservedHistogramMetric wraps a histogram and only updates it if the identifier
// is still reserved. This supports dynamic informers (e.g., GC, ResourceQuota)
// that may shut down while the process is still running.
type reservedHistogramMetric struct {
	id        cache.InformerNameAndResource
	histogram cache.HistogramMetric
}

func (r *reservedHistogramMetric) Observe(value float64) {
	if r.id.Reserved() {
		r.histogram.Observe(value)
	}
}
