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

// Package metrics provides Prometheus metrics for Runtime SDK extension calls.
// Consumers call [Register] once at startup to choose the subsystem prefix and
// Prometheus registry, then use the returned [Metrics] value to observe
// request outcomes.
package metrics

import (
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/runtime"

	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
)

const (
	unknownResponseStatus = "Unknown"
)

// Metrics holds the Prometheus collectors for Runtime SDK extension calls.
type Metrics struct {
	RequestsTotal   RequestsTotalObserver
	RequestDuration RequestDurationObserver
}

// Register creates Runtime SDK metrics under the given subsystem name
// (e.g. "capi_runtime_sdk") and registers
// them with the provided Prometheus registerer.
// Callers must invoke this exactly once per registerer/subsystem pair.
func Register(registerer prometheus.Registerer, subsystem string) Metrics {
	requestsTotal := RequestsTotalObserver{
		metric: prometheus.NewCounterVec(prometheus.CounterOpts{
			Subsystem: subsystem,
			Name:      "requests_total",
			Help:      "Number of HTTP requests, partitioned by status code, host, hook and response status.",
		}, []string{"code", "host", "group", "version", "hook", "status"}),
	}
	requestDuration := RequestDurationObserver{
		metric: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Subsystem: subsystem,
			Name:      "request_duration_seconds",
			Help:      "Request duration in seconds, broken down by hook and host.",
			Buckets: []float64{0.005, 0.025, 0.05, 0.1, 0.2, 0.4, 0.6, 0.8, 1.0, 1.25, 1.5, 2, 3,
				4, 5, 6, 8, 10, 15, 20, 30, 45, 60},
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: 1 * time.Hour,
		}, []string{"host", "group", "version", "hook"}),
	}

	registerer.MustRegister(requestsTotal.metric)
	registerer.MustRegister(requestDuration.metric)

	return Metrics{
		RequestsTotal:   requestsTotal,
		RequestDuration: requestDuration,
	}
}

// RequestsTotalObserver observes HTTP request results and increments the
// corresponding counter.
type RequestsTotalObserver struct {
	metric *prometheus.CounterVec
}

// Observe records an HTTP request result, incrementing the metric for the given
// HTTP status code, host, GroupVersionHook and response status.
func (m *RequestsTotalObserver) Observe(req *http.Request, resp *http.Response, gvh runtimecatalog.GroupVersionHook, err error, response runtime.Object) {
	host := req.URL.Host

	// Errors can be arbitrary strings. Unbound label cardinality is not suitable for a metric
	// system so they are reported as `<error>`.
	code := "<error>"
	if err == nil {
		code = strconv.Itoa(resp.StatusCode)
	}

	status := unknownResponseStatus
	if responseObject, ok := response.(runtimehooksv1.ResponseObject); ok && responseObject.GetStatus() != "" {
		status = string(responseObject.GetStatus())
	}

	m.metric.WithLabelValues(code, host, gvh.Group, gvh.Version, gvh.Hook, status).Inc()
}

// RequestDurationObserver observes request latency.
type RequestDurationObserver struct {
	metric *prometheus.HistogramVec
}

// Observe records the request latency for the given host and GroupVersionHook.
func (m *RequestDurationObserver) Observe(gvh runtimecatalog.GroupVersionHook, u url.URL, latency time.Duration) {
	m.metric.WithLabelValues(u.Host, gvh.Group, gvh.Version, gvh.Hook).Observe(latency.Seconds())
}
