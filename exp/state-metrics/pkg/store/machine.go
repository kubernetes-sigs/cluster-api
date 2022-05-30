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

// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines,verbs=get;list;watch

var descMachineLabelsDefaultLabels = []string{"namespace", "machine", "uid"}

type machineFactory struct {
	*controllerRuntimeClientFactory
}

var _ customresource.RegistryFactory = &machineFactory{}

func (f *machineFactory) Name() string {
	return "machines"
}

func (f *machineFactory) ExpectedType() interface{} {
	return &clusterv1.Machine{}
}

func (f *machineFactory) MetricFamilyGenerators(allowAnnotationsList, allowLabelsList []string) []generator.FamilyGenerator {
	return []generator.FamilyGenerator{
		*generator.NewFamilyGenerator(
			"capi_machine_created",
			"Unix creation timestamp",
			metric.Gauge,
			"",
			wrapMachineFunc(func(m *clusterv1.Machine) *metric.Family {
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
			"capi_machine_info",
			"Information about a machine.",
			metric.Gauge,
			"",
			wrapMachineFunc(func(m *clusterv1.Machine) *metric.Family {
				labelKeys := []string{}
				labelValues := []string{}

				if m.Spec.Version != nil {
					labelKeys = append(labelKeys, "version")
					labelValues = append(labelValues, *m.Spec.Version)
				}
				if m.Spec.ProviderID != nil {
					labelKeys = append(labelKeys, "provider_id")
					labelValues = append(labelValues, *m.Spec.ProviderID)
				}
				if m.Spec.FailureDomain != nil {
					labelKeys = append(labelKeys, "failure_domain")
					labelValues = append(labelValues, *m.Spec.FailureDomain)
				}

				internalIP := ""
				for _, address := range m.Status.Addresses {
					if address.Type == "InternalIP" {
						internalIP = address.Address
					}
				}
				labelKeys = append(labelKeys, "internal_ip")
				labelValues = append(labelValues, internalIP)

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
			"capi_machine_labels",
			"Kubernetes labels converted to Prometheus labels.",
			metric.Gauge,
			"",
			wrapMachineFunc(func(m *clusterv1.Machine) *metric.Family {
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
			"capi_machine_owner",
			"Information about the machine's owner.",
			metric.Gauge,
			"",
			wrapMachineFunc(func(m *clusterv1.Machine) *metric.Family {
				return getOwnerMetric(m.GetOwnerReferences())
			}),
		),
		*generator.NewFamilyGenerator(
			"capi_machine_paused",
			"The machine is paused and not reconciled.",
			metric.Gauge,
			"",
			wrapMachineFunc(func(m *clusterv1.Machine) *metric.Family {
				paused := annotations.HasPaused(m)
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
			"capi_machine_status_condition",
			"The current status conditions of a machine.",
			metric.Gauge,
			"",
			wrapMachineFunc(func(m *clusterv1.Machine) *metric.Family {
				return getConditionMetricFamily(m.Status.Conditions)
			}),
		),
		*generator.NewFamilyGenerator(
			"capi_machine_status_noderef",
			"Information about the machine's node reference.",
			metric.Gauge,
			"",
			wrapMachineFunc(func(m *clusterv1.Machine) *metric.Family {
				nodeRef := m.Status.NodeRef

				if nodeRef == nil {
					return &metric.Family{
						Metrics: []*metric.Metric{},
					}
				}
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							LabelKeys: []string{
								"name",
							},
							LabelValues: []string{
								nodeRef.Name,
							},
							Value: 1,
						},
					},
				}
			}),
		),
		*generator.NewFamilyGenerator(
			"capi_machine_status_phase",
			"The machines current phase.",
			metric.Gauge,
			"",
			wrapMachineFunc(func(m *clusterv1.Machine) *metric.Family {
				phase := clusterv1.MachinePhase(m.Status.Phase)
				if phase == "" {
					return &metric.Family{
						Metrics: []*metric.Metric{},
					}
				}

				phases := []struct {
					v bool
					n string
				}{
					{phase == clusterv1.MachinePhasePending, string(clusterv1.MachinePhasePending)},
					{phase == clusterv1.MachinePhaseProvisioning, string(clusterv1.MachinePhaseProvisioning)},
					{phase == clusterv1.MachinePhaseProvisioned, string(clusterv1.MachinePhaseProvisioned)},
					{phase == clusterv1.MachinePhaseRunning, string(clusterv1.MachinePhaseRunning)},
					{phase == clusterv1.MachinePhaseDeleting, string(clusterv1.MachinePhaseDeleting)},
					{phase == clusterv1.MachinePhaseDeleted, string(clusterv1.MachinePhaseDeleted)},
					{phase == clusterv1.MachinePhaseFailed, string(clusterv1.MachinePhaseFailed)},
					{phase == clusterv1.MachinePhaseUnknown, string(clusterv1.MachinePhaseUnknown)},
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

func (f *machineFactory) ListWatch(customResourceClient interface{}, ns string, fieldSelector string) cache.ListerWatcher {
	ctrlClient := customResourceClient.(client.WithWatch)
	return &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			machineList := clusterv1.MachineList{}
			opts.FieldSelector = fieldSelector
			err := ctrlClient.List(context.TODO(), &machineList, &client.ListOptions{Raw: &opts, Namespace: ns})
			return &machineList, err
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			machineList := clusterv1.MachineList{}
			opts.FieldSelector = fieldSelector
			return ctrlClient.Watch(context.TODO(), &machineList, &client.ListOptions{Raw: &opts, Namespace: ns})
		},
	}
}

func wrapMachineFunc(f func(*clusterv1.Machine) *metric.Family) func(interface{}) *metric.Family {
	return func(obj interface{}) *metric.Family {
		machine := obj.(*clusterv1.Machine)

		metricFamily := f(machine)

		for _, m := range metricFamily.Metrics {
			m.LabelKeys = append(descMachineLabelsDefaultLabels, m.LabelKeys...)
			m.LabelValues = append([]string{machine.Namespace, machine.Name, string(machine.UID)}, m.LabelValues...)
		}

		return metricFamily
	}
}
