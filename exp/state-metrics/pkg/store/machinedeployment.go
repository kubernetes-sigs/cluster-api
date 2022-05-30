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
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	"k8s.io/kube-state-metrics/v2/pkg/customresource"
	"k8s.io/kube-state-metrics/v2/pkg/metric"
	generator "k8s.io/kube-state-metrics/v2/pkg/metric_generator"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/annotations"
)

// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinedeployments,verbs=get;list;watch

var descMachineDeploymentLabelsDefaultLabels = []string{"namespace", "machinedeployment", "uid"}

type machineDeploymentFactory struct {
	*controllerRuntimeClientFactory
}

var _ customresource.RegistryFactory = &machineDeploymentFactory{}

func (f *machineDeploymentFactory) Name() string {
	return "machinedeployments"
}

func (f *machineDeploymentFactory) ExpectedType() interface{} {
	return &clusterv1.MachineDeployment{}
}

func (f *machineDeploymentFactory) MetricFamilyGenerators(allowAnnotationsList, allowLabelsList []string) []generator.FamilyGenerator {
	return []generator.FamilyGenerator{
		*generator.NewFamilyGenerator(
			"capi_machinedeployment_created",
			"Unix creation timestamp",
			metric.Gauge,
			"",
			wrapMachineDeploymentFunc(func(md *clusterv1.MachineDeployment) *metric.Family {
				ms := []*metric.Metric{}

				if !md.CreationTimestamp.IsZero() {
					ms = append(ms, &metric.Metric{
						LabelKeys:   []string{},
						LabelValues: []string{},
						Value:       float64(md.CreationTimestamp.Unix()),
					})
				}

				return &metric.Family{
					Metrics: ms,
				}
			}),
		),
		*generator.NewFamilyGenerator(
			"capi_machinedeployment_labels",
			"Kubernetes labels converted to Prometheus labels.",
			metric.Gauge,
			"",
			wrapMachineDeploymentFunc(func(md *clusterv1.MachineDeployment) *metric.Family {
				labelKeys, labelValues := createLabelKeysValues(md.Labels, allowLabelsList)
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
			"capi_machinedeployment_owner",
			"Information about the kubeadmcontrolplane's owner.",
			metric.Gauge,
			"",
			wrapMachineDeploymentFunc(func(md *clusterv1.MachineDeployment) *metric.Family {
				return getOwnerMetric(md.GetOwnerReferences())
			}),
		),
		*generator.NewFamilyGenerator(
			"capi_machinedeployment_paused",
			"The machinedeployment is paused and not reconciled.",
			metric.Gauge,
			"",
			wrapMachineDeploymentFunc(func(md *clusterv1.MachineDeployment) *metric.Family {
				paused := annotations.HasPaused(md) || md.Spec.Paused
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
			"capi_machinedeployment_spec_replicas",
			"Number of desired replicas for a machinedeployment.",
			metric.Gauge,
			"",
			wrapMachineDeploymentFunc(func(md *clusterv1.MachineDeployment) *metric.Family {
				ms := []*metric.Metric{}

				if md.Spec.Replicas != nil {
					ms = append(ms, &metric.Metric{
						Value: float64(*md.Spec.Replicas),
					})
				}

				return &metric.Family{
					Metrics: ms,
				}
			}),
		),
		*generator.NewFamilyGenerator(
			"capi_machinedeployment_spec_strategy_rollingupdate_max_surge",
			"Maximum number of replicas that can be scheduled above the desired number of replicas during a rolling update of a machinedeployment.",
			metric.Gauge,
			"",
			wrapMachineDeploymentFunc(func(md *clusterv1.MachineDeployment) *metric.Family {
				if md.Spec.Strategy == nil || md.Spec.Strategy.RollingUpdate == nil || md.Spec.Replicas == nil {
					return &metric.Family{}
				}

				maxSurge, err := intstr.GetScaledValueFromIntOrPercent(md.Spec.Strategy.RollingUpdate.MaxSurge, int(*md.Spec.Replicas), true)
				if err != nil {
					panic(err)
				}

				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							Value: float64(maxSurge),
						},
					},
				}
			}),
		),
		*generator.NewFamilyGenerator(
			"capi_machinedeployment_spec_strategy_rollingupdate_max_unavailable",
			"Maximum number of unavailable replicas during a rolling update of a machinedeployment.",
			metric.Gauge,
			"",
			wrapMachineDeploymentFunc(func(md *clusterv1.MachineDeployment) *metric.Family {
				if md.Spec.Strategy == nil || md.Spec.Strategy.RollingUpdate == nil {
					return &metric.Family{}
				}

				maxUnavailable, err := intstr.GetScaledValueFromIntOrPercent(md.Spec.Strategy.RollingUpdate.MaxUnavailable, int(*md.Spec.Replicas), false)
				if err != nil {
					panic(err)
				}

				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							Value: float64(maxUnavailable),
						},
					},
				}
			}),
		),
		*generator.NewFamilyGenerator(
			"capi_machinedeployment_status_condition",
			"The current status conditions of a machinedeployment.",
			metric.Gauge,
			"",
			wrapMachineDeploymentFunc(func(md *clusterv1.MachineDeployment) *metric.Family {
				return getConditionMetricFamily(md.Status.Conditions)
			}),
		),
		*generator.NewFamilyGenerator(
			"capi_machinedeployment_status_phase",
			"The machinedeployments current phase.",
			metric.Gauge,
			"",
			wrapMachineDeploymentFunc(func(md *clusterv1.MachineDeployment) *metric.Family {
				phase := clusterv1.MachineDeploymentPhase(md.Status.Phase)
				if phase == "" {
					return &metric.Family{
						Metrics: []*metric.Metric{},
					}
				}

				phases := []struct {
					v bool
					n string
				}{
					{phase == clusterv1.MachineDeploymentPhaseScalingUp, string(clusterv1.MachineDeploymentPhaseScalingUp)},
					{phase == clusterv1.MachineDeploymentPhaseScalingDown, string(clusterv1.MachineDeploymentPhaseScalingDown)},
					{phase == clusterv1.MachineDeploymentPhaseRunning, string(clusterv1.MachineDeploymentPhaseRunning)},
					{phase == clusterv1.MachineDeploymentPhaseFailed, string(clusterv1.MachineDeploymentPhaseFailed)},
					{phase == clusterv1.MachineDeploymentPhaseUnknown, string(clusterv1.MachineDeploymentPhaseUnknown)},
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
		*generator.NewFamilyGenerator(
			"capi_machinedeployment_status_replicas",
			"The number of replicas per machinedeployment.",
			metric.Gauge,
			"",
			wrapMachineDeploymentFunc(func(md *clusterv1.MachineDeployment) *metric.Family {
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							Value: float64(md.Status.Replicas),
						},
					},
				}
			}),
		),
		*generator.NewFamilyGenerator(
			"capi_machinedeployment_status_replicas_available",
			"The number of available replicas per machinedeployment.",
			metric.Gauge,
			"",
			wrapMachineDeploymentFunc(func(md *clusterv1.MachineDeployment) *metric.Family {
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							Value: float64(md.Status.AvailableReplicas),
						},
					},
				}
			}),
		),
		*generator.NewFamilyGenerator(
			"capi_machinedeployment_status_replicas_unavailable",
			"The number of unavailable replicas per machinedeployment.",
			metric.Gauge,
			"",
			wrapMachineDeploymentFunc(func(md *clusterv1.MachineDeployment) *metric.Family {
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							Value: float64(md.Status.UnavailableReplicas),
						},
					},
				}
			}),
		),
		*generator.NewFamilyGenerator(
			"capi_machinedeployment_status_replicas_updated",
			"The number of updated replicas per machinedeployment.",
			metric.Gauge,
			"",
			wrapMachineDeploymentFunc(func(md *clusterv1.MachineDeployment) *metric.Family {
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							Value: float64(md.Status.UpdatedReplicas),
						},
					},
				}
			}),
		),
	}
}

func (f *machineDeploymentFactory) ListWatch(customResourceClient interface{}, ns string, fieldSelector string) cache.ListerWatcher {
	ctrlClient := customResourceClient.(client.WithWatch)
	return &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			machineDeploymentList := clusterv1.MachineDeploymentList{}
			opts.FieldSelector = fieldSelector
			err := ctrlClient.List(context.TODO(), &machineDeploymentList, &client.ListOptions{Raw: &opts, Namespace: ns})
			return &machineDeploymentList, err
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			machineDeploymentList := clusterv1.MachineDeploymentList{}
			opts.FieldSelector = fieldSelector
			return ctrlClient.Watch(context.TODO(), &machineDeploymentList, &client.ListOptions{Raw: &opts, Namespace: ns})
		},
	}
}

func wrapMachineDeploymentFunc(f func(*clusterv1.MachineDeployment) *metric.Family) func(interface{}) *metric.Family {
	return func(obj interface{}) *metric.Family {
		machineDeployment := obj.(*clusterv1.MachineDeployment)

		metricFamily := f(machineDeployment)

		for _, m := range metricFamily.Metrics {
			m.LabelKeys = append(descMachineDeploymentLabelsDefaultLabels, m.LabelKeys...)
			m.LabelValues = append([]string{machineDeployment.Namespace, machineDeployment.Name, string(machineDeployment.UID)}, m.LabelValues...)
		}

		return metricFamily
	}
}
