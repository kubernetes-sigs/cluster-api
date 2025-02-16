/*
Copyright 2024 The Kubernetes Authors.

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

package clustercache

import (
	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

func init() {
	// Register the metrics at the controller-runtime metrics registry.
	ctrlmetrics.Registry.MustRegister(healthCheck)
	ctrlmetrics.Registry.MustRegister(connectionUp)
	ctrlmetrics.Registry.MustRegister(healthChecksTotal)
}

var (
	healthCheck = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "capi_cluster_cache_healthcheck",
			Help: "Result of the last clustercache healthcheck for a cluster.",
		}, []string{
			"cluster_name", "cluster_namespace",
		},
	)
	healthChecksTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "capi_cluster_cache_healthchecks_total",
			Help: "Results of all clustercache healthchecks.",
		}, []string{
			"cluster_name", "cluster_namespace", "status",
		},
	)
	connectionUp = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "capi_cluster_cache_connection_up",
			Help: "Whether the connection to the cluster is up.",
		}, []string{
			"cluster_name", "cluster_namespace",
		},
	)
)
