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

	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/annotations"
)

// +kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=kubeadmcontrolplanes,verbs=get;list;watch

var descKubeadmControlPlaneLabelsDefaultLabels = []string{"namespace", "kubeadmcontrolplane", "uid"}

type kubeadmControlPlaneFactory struct {
	*controllerRuntimeClientFactory
}

var _ customresource.RegistryFactory = &kubeadmControlPlaneFactory{}

func (f *kubeadmControlPlaneFactory) Name() string {
	return "kubeadmcontrolplanes"
}

func (f *kubeadmControlPlaneFactory) ExpectedType() interface{} {
	return &controlplanev1.KubeadmControlPlane{}
}

func (f *kubeadmControlPlaneFactory) MetricFamilyGenerators(allowAnnotationsList, allowLabelsList []string) []generator.FamilyGenerator {
	return []generator.FamilyGenerator{
		*generator.NewFamilyGenerator(
			"capi_kubeadmcontrolplane_created",
			"Unix creation timestamp",
			metric.Gauge,
			"",
			wrapKubeadmControlPlaneFunc(func(kcp *controlplanev1.KubeadmControlPlane) *metric.Family {
				ms := []*metric.Metric{}

				if !kcp.CreationTimestamp.IsZero() {
					ms = append(ms, &metric.Metric{
						LabelKeys:   []string{},
						LabelValues: []string{},
						Value:       float64(kcp.CreationTimestamp.Unix()),
					})
				}

				return &metric.Family{
					Metrics: ms,
				}
			}),
		),
		*generator.NewFamilyGenerator(
			"capi_kubeadmcontrolplane_info",
			"Information about a kubeadmcontrolplane.",
			metric.Gauge,
			"",
			wrapKubeadmControlPlaneFunc(func(kcp *controlplanev1.KubeadmControlPlane) *metric.Family {
				labelKeys := []string{
					"version",
				}
				labelValues := []string{
					kcp.Spec.Version,
				}

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
			"capi_kubeadmcontrolplane_labels",
			"Kubernetes labels converted to Prometheus labels.",
			metric.Gauge,
			"",
			wrapKubeadmControlPlaneFunc(func(kcp *controlplanev1.KubeadmControlPlane) *metric.Family {
				labelKeys, labelValues := createLabelKeysValues(kcp.Labels, allowLabelsList)
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
			"capi_kubeadmcontrolplane_owner",
			"Information about the kubeadmcontrolplane's owner.",
			metric.Gauge,
			"",
			wrapKubeadmControlPlaneFunc(func(kcp *controlplanev1.KubeadmControlPlane) *metric.Family {
				return getOwnerMetric(kcp.GetOwnerReferences())
			}),
		),
		*generator.NewFamilyGenerator(
			"capi_kubeadmcontrolplane_paused",
			"The kubeadmcontrolplane is paused and not reconciled.",
			metric.Gauge,
			"",
			wrapKubeadmControlPlaneFunc(func(kcp *controlplanev1.KubeadmControlPlane) *metric.Family {
				paused := annotations.HasPaused(kcp)
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
			"capi_kubeadmcontrolplane_spec_replicas",
			"Number of desired replicas for a kubeadmcontrolplane.",
			metric.Gauge,
			"",
			wrapKubeadmControlPlaneFunc(func(kcp *controlplanev1.KubeadmControlPlane) *metric.Family {
				ms := []*metric.Metric{}

				if kcp.Spec.Replicas != nil {
					ms = append(ms, &metric.Metric{
						Value: float64(*kcp.Spec.Replicas),
					})
				}

				return &metric.Family{
					Metrics: ms,
				}
			}),
		),
		*generator.NewFamilyGenerator(
			"capi_kubeadmcontrolplane_spec_strategy_rollingupdate_max_surge",
			"Maximum number of replicas that can be scheduled above the desired number of replicas during a rolling update of a kubeadmcontrolplane.",
			metric.Gauge,
			"",
			wrapKubeadmControlPlaneFunc(func(kcp *controlplanev1.KubeadmControlPlane) *metric.Family {
				if kcp.Spec.RolloutStrategy == nil || kcp.Spec.RolloutStrategy.RollingUpdate == nil || kcp.Spec.Replicas == nil {
					return &metric.Family{}
				}

				maxSurge, err := intstr.GetScaledValueFromIntOrPercent(kcp.Spec.RolloutStrategy.RollingUpdate.MaxSurge, int(*kcp.Spec.Replicas), true)
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
			"capi_kubeadmcontrolplane_status_condition",
			"The current status conditions of a machine.",
			metric.Gauge,
			"",
			wrapKubeadmControlPlaneFunc(func(kcp *controlplanev1.KubeadmControlPlane) *metric.Family {
				return getConditionMetricFamily(kcp.Status.Conditions)
			}),
		),
		*generator.NewFamilyGenerator(
			"capi_kubeadmcontrolplane_status_replicas",
			"The number of replicas per kubeadmcontrolplane.",
			metric.Gauge,
			"",
			wrapKubeadmControlPlaneFunc(func(kcp *controlplanev1.KubeadmControlPlane) *metric.Family {
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							Value: float64(kcp.Status.Replicas),
						},
					},
				}
			}),
		),
		*generator.NewFamilyGenerator(
			"capi_kubeadmcontrolplane_status_replicas_ready",
			"The number of ready replicas per kubeadmcontrolplane.",
			metric.Gauge,
			"",
			wrapKubeadmControlPlaneFunc(func(kcp *controlplanev1.KubeadmControlPlane) *metric.Family {
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							Value: float64(kcp.Status.ReadyReplicas),
						},
					},
				}
			}),
		),
		*generator.NewFamilyGenerator(
			"capi_kubeadmcontrolplane_status_replicas_unavailable",
			"The number of unavailable replicas per kubeadmcontrolplane.",
			metric.Gauge,
			"",
			wrapKubeadmControlPlaneFunc(func(kcp *controlplanev1.KubeadmControlPlane) *metric.Family {
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							Value: float64(kcp.Status.UnavailableReplicas),
						},
					},
				}
			}),
		),
		*generator.NewFamilyGenerator(
			"capi_kubeadmcontrolplane_status_replicas_updated",
			"The number of updated replicas per kubeadmcontrolplane.",
			metric.Gauge,
			"",
			wrapKubeadmControlPlaneFunc(func(kcp *controlplanev1.KubeadmControlPlane) *metric.Family {
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							Value: float64(kcp.Status.UpdatedReplicas),
						},
					},
				}
			}),
		),
	}
}

func (f *kubeadmControlPlaneFactory) ListWatch(customResourceClient interface{}, ns string, fieldSelector string) cache.ListerWatcher {
	ctrlClient := customResourceClient.(client.WithWatch)
	return &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			kubeadmControlPlaneList := controlplanev1.KubeadmControlPlaneList{}
			opts.FieldSelector = fieldSelector
			err := ctrlClient.List(context.TODO(), &kubeadmControlPlaneList, &client.ListOptions{Raw: &opts, Namespace: ns})
			return &kubeadmControlPlaneList, err
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			kubeadmControlPlaneList := controlplanev1.KubeadmControlPlaneList{}
			opts.FieldSelector = fieldSelector
			return ctrlClient.Watch(context.TODO(), &kubeadmControlPlaneList, &client.ListOptions{Raw: &opts, Namespace: ns})
		},
	}
}

func wrapKubeadmControlPlaneFunc(f func(*controlplanev1.KubeadmControlPlane) *metric.Family) func(interface{}) *metric.Family {
	return func(obj interface{}) *metric.Family {
		kubeadmControlPlane := obj.(*controlplanev1.KubeadmControlPlane)

		metricFamily := f(kubeadmControlPlane)

		for _, m := range metricFamily.Metrics {
			m.LabelKeys = append(descKubeadmControlPlaneLabelsDefaultLabels, m.LabelKeys...)
			m.LabelValues = append([]string{kubeadmControlPlane.Namespace, kubeadmControlPlane.Name, string(kubeadmControlPlane.UID)}, m.LabelValues...)
		}

		return metricFamily
	}
}
