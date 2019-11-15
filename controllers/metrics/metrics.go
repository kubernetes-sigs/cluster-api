/*
Copyright 2019 The Kubernetes Authors.

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

// Package metrics defines the metrics available for the cluster api
// controllers.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// ClusterControlPlaneReady is a metric that is set to 1 if the cluster
	// control plane is ready and 0 if it is not.
	ClusterControlPlaneReady = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "capi_cluster_control_plane_ready",
			Help: "Cluster control plane is ready if set to 1 and not if 0.",
		},
		[]string{"cluster", "namespace"},
	)

	// ClusterInfrastructureReady is a metric that is set to 1 if the cluster
	// infrastructure is ready and 0 if it is not.
	ClusterInfrastructureReady = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "capi_cluster_infrastructure_ready",
			Help: "Cluster infrastructure is ready if set to 1 and not if 0.",
		},
		[]string{"cluster", "namespace"},
	)

	// ClusterKubeconfigReady  is a metric that is set to 1 if the cluster
	// kubeconfig secret has been created and 0 if it is not.
	ClusterKubeconfigReady = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "capi_cluster_kubeconfig_ready",
			Help: "Cluster kubeconfig is ready if set to 1 and not if 0.",
		},
		[]string{"cluster", "namespace"},
	)

	// ClusterFailureSet is a metric that is set to 1 if the cluster FailureReason
	// or FailureMessage is set and 0 if it is not.
	ClusterFailureSet = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "capi_cluster_failure_set",
			Help: "Cluster failure message or reason is set if metric is 1.",
		},
		[]string{"cluster", "namespace"},
	)

	// MachineBootstrapReady is a metric that is set to 1 if machine bootstrap
	// is ready and 0 if it is not.
	MachineBootstrapReady = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "capi_machine_bootstrap_ready",
			Help: "Machine Boostrap is ready if set to 1 and not if 0.",
		},
		[]string{"machine", "namespace", "cluster"},
	)

	// MachineInfrastructureReady  is a metric that is set to 1 if machine
	// infrastructure is ready and 0 if it is not.
	MachineInfrastructureReady = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "capi_machine_infrastructure_ready",
			Help: "Machine InfrastructureRef is ready if set to 1 and not if 0.",
		},
		[]string{"machine", "namespace", "cluster"},
	)

	// MachineNodeReady  is a metric that is set to 1 if machine node is ready
	// and 0 if it is not.
	MachineNodeReady = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "capi_machine_node_ready",
			Help: "Machine NodeRef is ready if set to 1 and not if 0.",
		},
		[]string{"machine", "namespace", "cluster"},
	)
)

func init() {
	metrics.Registry.MustRegister(
		ClusterControlPlaneReady,
		ClusterInfrastructureReady,
		ClusterKubeconfigReady,
		ClusterFailureSet,
		MachineBootstrapReady,
		MachineInfrastructureReady,
		MachineNodeReady,
	)
}
