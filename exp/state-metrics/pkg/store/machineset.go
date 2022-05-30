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

// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinesets,verbs=get;list;watch

var descMachineSetLabelsDefaultLabels = []string{"namespace", "machineset", "uid"}

type machineSetFactory struct {
	*controllerRuntimeClientFactory
}

var _ customresource.RegistryFactory = &machineSetFactory{}

func (f *machineSetFactory) Name() string {
	return "machinesets"
}

func (f *machineSetFactory) ExpectedType() interface{} {
	return &clusterv1.MachineSet{}
}

func (f *machineSetFactory) MetricFamilyGenerators(allowAnnotationsList, allowLabelsList []string) []generator.FamilyGenerator {
	return []generator.FamilyGenerator{
		*generator.NewFamilyGenerator(
			"capi_machineset_created",
			"Unix creation timestamp",
			metric.Gauge,
			"",
			wrapMachineSetFunc(func(m *clusterv1.MachineSet) *metric.Family {
				ms := []*metric.Metric{}
				if !m.CreationTimestamp.IsZero() {
					ms = append(ms, &metric.Metric{
						LabelKeys:   []string{},
						LabelValues: []string{},
						Value:       float64(m.CreationTimestamp.Unix()),
					})
				}
				return &metric.Family{
					Metrics: ms,
				}
			}),
		),
		*generator.NewFamilyGenerator(
			"capi_machineset_labels",
			"Kubernetes labels converted to Prometheus labels.",
			metric.Gauge,
			"",
			wrapMachineSetFunc(func(m *clusterv1.MachineSet) *metric.Family {
				labelKeys, labelValues := createLabelKeysValues(m.Labels, allowLabelsList)
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
			"capi_machineset_owner",
			"Information about the machineset's owner.",
			metric.Gauge,
			"",
			wrapMachineSetFunc(func(m *clusterv1.MachineSet) *metric.Family {
				return getOwnerMetric(m.GetOwnerReferences())
			}),
		),
		*generator.NewFamilyGenerator(
			"capi_machineset_paused",
			"The machineset is paused and not reconciled.",
			metric.Gauge,
			"",
			wrapMachineSetFunc(func(ms *clusterv1.MachineSet) *metric.Family {
				paused := annotations.HasPaused(ms)
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
			"capi_machineset_spec_replicas",
			"Number of desired replicas for a machineset.",
			metric.Gauge,
			"",
			wrapMachineSetFunc(func(m *clusterv1.MachineSet) *metric.Family {
				ms := []*metric.Metric{}

				if m.Spec.Replicas != nil {
					ms = append(ms, &metric.Metric{
						Value: float64(*m.Spec.Replicas),
					})
				}

				return &metric.Family{
					Metrics: ms,
				}
			}),
		),
		*generator.NewFamilyGenerator(
			"capi_machineset_status_available_replicas",
			"The number of available replicas per machineset.",
			metric.Gauge,
			"",
			wrapMachineSetFunc(func(m *clusterv1.MachineSet) *metric.Family {
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							Value: float64(m.Status.AvailableReplicas),
						},
					},
				}
			}),
		),
		*generator.NewFamilyGenerator(
			"capi_machineset_status_condition",
			"The current status conditions of a machineset.",
			metric.Gauge,
			"",
			wrapMachineSetFunc(func(m *clusterv1.MachineSet) *metric.Family {
				return getConditionMetricFamily(m.Status.Conditions)
			}),
		),

		*generator.NewFamilyGenerator(
			"capi_machineset_status_fully_labeled_replicas",
			"The number of fully labeled replicas per machineset.",
			metric.Gauge,
			"",
			wrapMachineSetFunc(func(m *clusterv1.MachineSet) *metric.Family {
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							Value: float64(m.Status.FullyLabeledReplicas),
						},
					},
				}
			}),
		),
		*generator.NewFamilyGenerator(
			"capi_machineset_status_ready_replicas",
			"The number of ready replicas per machineset.",
			metric.Gauge,
			"",
			wrapMachineSetFunc(func(m *clusterv1.MachineSet) *metric.Family {
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							Value: float64(m.Status.ReadyReplicas),
						},
					},
				}
			}),
		),
		*generator.NewFamilyGenerator(
			"capi_machineset_status_replicas",
			"The number of replicas per machineset.",
			metric.Gauge,
			"",
			wrapMachineSetFunc(func(m *clusterv1.MachineSet) *metric.Family {
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							Value: float64(m.Status.Replicas),
						},
					},
				}
			}),
		),
	}
}

func (f *machineSetFactory) ListWatch(customResourceClient interface{}, ns string, fieldSelector string) cache.ListerWatcher {
	ctrlClient := customResourceClient.(client.WithWatch)
	return &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			machineSetList := clusterv1.MachineSetList{}
			opts.FieldSelector = fieldSelector
			err := ctrlClient.List(context.TODO(), &machineSetList, &client.ListOptions{Raw: &opts, Namespace: ns})
			return &machineSetList, err
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			machineSetList := clusterv1.MachineSetList{}
			opts.FieldSelector = fieldSelector
			return ctrlClient.Watch(context.TODO(), &machineSetList, &client.ListOptions{Raw: &opts, Namespace: ns})
		},
	}
}

func wrapMachineSetFunc(f func(*clusterv1.MachineSet) *metric.Family) func(interface{}) *metric.Family {
	return func(obj interface{}) *metric.Family {
		machineSet := obj.(*clusterv1.MachineSet)

		metricFamily := f(machineSet)

		for _, m := range metricFamily.Metrics {
			m.LabelKeys = append(descMachineSetLabelsDefaultLabels, m.LabelKeys...)
			m.LabelValues = append([]string{machineSet.Namespace, machineSet.Name, string(machineSet.UID)}, m.LabelValues...)
		}

		return metricFamily
	}
}
