/*
Copyright 2022 The Kubernetes Authors.

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

// Package metrics provides functions for creating Runtime SDK related metrics.
package metrics

import (
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
)

func init() {
	// Register the metrics at the controller-runtime metrics registry.
	ctrlmetrics.Registry.MustRegister(RequestsTotal.metric)
	ctrlmetrics.Registry.MustRegister(RequestDuration.metric)
}

// Metrics subsystem and all of the keys used by the Runtime SDK.
const (
	runtimeSDKSubsystem   = "capi_runtime_sdk"
	unknownResponseStatus = "Unknown"
)

var (
	// RequestsTotal reports request results.
	RequestsTotal = requestsTotalObserver{
		prometheus.NewCounterVec(prometheus.CounterOpts{
			Subsystem: runtimeSDKSubsystem,
			Name:      "requests_total",
			Help:      "Number of HTTP requests, partitioned by status code, host, hook and response status.",
		}, []string{"code", "host", "group", "version", "hook", "status"}),
	}
	// RequestDuration reports the request latency in seconds.
	RequestDuration = requestDurationObserver{
		prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Subsystem: runtimeSDKSubsystem,
			Name:      "request_duration_seconds",
			Help:      "Request duration in seconds, broken down by hook and host.",
			Buckets: []float64{0.005, 0.025, 0.05, 0.1, 0.2, 0.4, 0.6, 0.8, 1.0, 1.25, 1.5, 2, 3,
				4, 5, 6, 8, 10, 15, 20, 30, 45, 60},
		}, []string{"host", "group", "version", "hook"}),
	}
)

type requestsTotalObserver struct {
	metric *prometheus.CounterVec
}

// Observe observes a http request result and increments the metric for the given
// http status code, host, gvh and response.
func (m *requestsTotalObserver) Observe(req *http.Request, resp *http.Response, gvh runtimecatalog.GroupVersionHook, err error, response runtime.Object) {
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

type requestDurationObserver struct {
	metric *prometheus.HistogramVec
}

// Observe increments the request latency metric for the given host and gvh.
func (m *requestDurationObserver) Observe(gvh runtimecatalog.GroupVersionHook, u url.URL, latency time.Duration) {
	m.metric.WithLabelValues(u.Host, gvh.Group, gvh.Version, gvh.Hook).Observe(latency.Seconds())
}
