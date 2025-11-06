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

package client

import (
	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

func init() {
	// Register the metrics at the controller-runtime metrics registry.
	ctrlmetrics.Registry.MustRegister(waitDurationMetric)
}

var (
	waitDurationMetric = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "capi_client_cache_wait_duration_seconds",
		Help:    "Duration that we waited for the cache to be up-to-date in seconds, broken down by kind and status",
		Buckets: []float64{0.00001, 0.000025, 0.00005, 0.0001, 0.00025, 0.0005, 0.001, 0.005, 0.025, 0.05, 0.1, 0.2, 0.4, 0.6, 0.8, 1.0, 2.5, 5, 10},
	}, []string{"kind", "status"})
)
