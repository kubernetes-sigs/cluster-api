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

package ssa

import (
	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

func init() {
	// Register the metrics at the controller-runtime metrics registry.
	ctrlmetrics.Registry.MustRegister(cacheHits)
	ctrlmetrics.Registry.MustRegister(cacheMisses)
}

var (
	cacheHits = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "capi_ssa_cache_hits_total",
		Help: "Total number of ssa cache hits.",
	}, []string{
		"kind", "controller",
	})

	cacheMisses = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "capi_ssa_cache_misses_total",
		Help: "Total number of ssa cache misses.",
	}, []string{
		"kind", "controller",
	})
)
