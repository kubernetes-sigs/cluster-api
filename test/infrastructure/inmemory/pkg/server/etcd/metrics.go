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

package etcd

import (
	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

func init() {
	// Note: It probably makes sense to check if we can expose the same grpc metrics as etcd itself
	// via some existing util.

	// Register the metrics at the controller-runtime metrics registry.
	ctrlmetrics.Registry.MustRegister(requestLatency)
}

var (
	requestLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "capim_etcd_request_duration_seconds",
			Help: "Request latency in seconds.",
			Buckets: []float64{0.005, 0.025, 0.05, 0.1, 0.2, 0.4, 0.6, 0.8, 1.0, 1.25, 1.5, 2, 3,
				4, 5, 6, 8, 10, 15, 20, 30, 45, 60},
		}, []string{"grpc_method", "cluster_name"},
	)
)
