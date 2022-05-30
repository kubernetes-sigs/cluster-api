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

package store

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	"k8s.io/kube-state-metrics/v2/pkg/customresource"
	"k8s.io/kube-state-metrics/v2/pkg/metric"
	generator "k8s.io/kube-state-metrics/v2/pkg/metric_generator"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/annotations"
)

// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters,verbs=get;list;watch

var descClusterLabelsDefaultLabels = []string{"namespace", "cluster", "uid"}

type clusterFactory struct {
	*controllerRuntimeClientFactory
}

var _ customresource.RegistryFactory = &clusterFactory{}

func (f *clusterFactory) Name() string {
	return "clusters"
}

func (f *clusterFactory) ExpectedType() interface{} {
	return &clusterv1.Cluster{}
}

func (f *clusterFactory) MetricFamilyGenerators(allowAnnotationsList, allowLabelsList []string) []generator.FamilyGenerator {
	return []generator.FamilyGenerator{
		*generator.NewFamilyGenerator(
			"capi_cluster_created",
			"Unix creation timestamp",
			metric.Gauge,
			"",
			wrapClusterFunc(func(c *clusterv1.Cluster) *metric.Family {
				ms := []*metric.Metric{}

				if !c.CreationTimestamp.IsZero() {
					ms = append(ms, &metric.Metric{
						LabelKeys:   []string{},
						LabelValues: []string{},
						Value:       float64(c.CreationTimestamp.Unix()),
					})
				}

				return &metric.Family{
					Metrics: ms,
				}
			}),
		),
		*generator.NewFamilyGenerator(
			"capi_cluster_labels",
			"Kubernetes labels converted to Prometheus labels.",
			metric.Gauge,
			"",
			wrapClusterFunc(func(c *clusterv1.Cluster) *metric.Family {
				labelKeys, labelValues := createLabelKeysValues(c.Labels, allowLabelsList)
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							LabelKeys:   labelKeys,
							LabelValues: labelValues,
							Value:       1,
						},
					},
				}
			}),
		),
		*generator.NewFamilyGenerator(
			"capi_cluster_paused",
			"The cluster is paused and not reconciled.",
			metric.Gauge,
			"",
			wrapClusterFunc(func(c *clusterv1.Cluster) *metric.Family {
				paused := annotations.HasPaused(c) || c.Spec.Paused
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							LabelKeys:   []string{},
							LabelValues: []string{},
							Value:       boolFloat64(paused),
						},
					},
				}
			}),
		),
		*generator.NewFamilyGenerator(
			"capi_cluster_status_condition",
			"The current status conditions of a cluster.",
			metric.Gauge,
			"",
			wrapClusterFunc(func(c *clusterv1.Cluster) *metric.Family {
				return getConditionMetricFamily(c.Status.Conditions)
			}),
		),
		*generator.NewFamilyGenerator(
			"capi_cluster_status_phase",
			"The clusters current phase.",
			metric.Gauge,
			"",
			wrapClusterFunc(func(c *clusterv1.Cluster) *metric.Family {
				phase := clusterv1.ClusterPhase(c.Status.Phase)
				if phase == "" {
					return &metric.Family{
						Metrics: []*metric.Metric{},
					}
				}

				phases := []struct {
					v bool
					n string
				}{
					{phase == clusterv1.ClusterPhasePending, string(clusterv1.ClusterPhasePending)},
					{phase == clusterv1.ClusterPhaseProvisioning, string(clusterv1.ClusterPhaseProvisioning)},
					{phase == clusterv1.ClusterPhaseProvisioned, string(clusterv1.ClusterPhaseProvisioned)},
					{phase == clusterv1.ClusterPhaseDeleting, string(clusterv1.ClusterPhaseDeleting)},
					{phase == clusterv1.ClusterPhaseFailed, string(clusterv1.ClusterPhaseFailed)},
					{phase == clusterv1.ClusterPhaseUnknown, string(clusterv1.ClusterPhaseUnknown)},
				}

				ms := make([]*metric.Metric, len(phases))

				for i, p := range phases {
					ms[i] = &metric.Metric{
						LabelKeys:   []string{"phase"},
						LabelValues: []string{p.n},
						Value:       boolFloat64(p.v),
					}
				}

				return &metric.Family{
					Metrics: ms,
				}
			}),
		),
	}
}

func (f *clusterFactory) ListWatch(customResourceClient interface{}, ns string, fieldSelector string) cache.ListerWatcher {
	ctrlClient := customResourceClient.(client.WithWatch)
	return &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			clusterList := clusterv1.ClusterList{}
			opts.FieldSelector = fieldSelector
			err := ctrlClient.List(context.TODO(), &clusterList, &client.ListOptions{Raw: &opts, Namespace: ns})
			return &clusterList, err
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			clusterList := clusterv1.ClusterList{}
			opts.FieldSelector = fieldSelector
			return ctrlClient.Watch(context.TODO(), &clusterList, &client.ListOptions{Raw: &opts, Namespace: ns})
		},
	}
}

func wrapClusterFunc(f func(*clusterv1.Cluster) *metric.Family) func(interface{}) *metric.Family {
	return func(obj interface{}) *metric.Family {
		cluster := obj.(*clusterv1.Cluster)

		metricFamily := f(cluster)

		for _, m := range metricFamily.Metrics {
			m.LabelKeys = append(descClusterLabelsDefaultLabels, m.LabelKeys...)
			m.LabelValues = append([]string{cluster.Namespace, cluster.Name, string(cluster.UID)}, m.LabelValues...)
		}

		return metricFamily
	}
}
