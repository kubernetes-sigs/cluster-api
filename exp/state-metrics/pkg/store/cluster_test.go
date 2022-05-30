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
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	generator "k8s.io/kube-state-metrics/v2/pkg/metric_generator"

	clusterv1alpha4 "sigs.k8s.io/cluster-api/api/v1alpha4"
)

func TestClusterStore(t *testing.T) {
	startTime := 1501569018
	metav1StartTime := metav1.Unix(int64(startTime), 0)

	cases := []generateMetricsTestCase{
		{
			Obj: &clusterv1alpha4.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "cluster1",
					Namespace:         "ns1",
					CreationTimestamp: metav1StartTime,
					ResourceVersion:   "10596",
					UID:               types.UID("foo"),
				},
			},
			Want: `
				# HELP capi_cluster_created Unix creation timestamp
				# HELP capi_cluster_labels Kubernetes labels converted to Prometheus labels.
				# TYPE capi_cluster_created gauge
				# TYPE capi_cluster_labels gauge
				capi_cluster_created{cluster="cluster1",namespace="ns1",uid="foo"} 1.501569018e+09
				capi_cluster_labels{cluster="cluster1",namespace="ns1",uid="foo"} 1
				`,
			MetricNames: []string{"capi_cluster_labels", "capi_cluster_created"},
		},
		{
			Obj: &clusterv1alpha4.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "cluster2",
					Namespace:         "ns2",
					CreationTimestamp: metav1StartTime,
					ResourceVersion:   "10597",
					UID:               types.UID("foo"),
				},
				Status: clusterv1alpha4.ClusterStatus{
					Phase: string(clusterv1alpha4.ClusterPhaseFailed),
				},
			},
			Want: `
				# HELP capi_cluster_status_phase The clusters current phase.
				# TYPE capi_cluster_status_phase gauge
				capi_cluster_status_phase{cluster="cluster2",namespace="ns2",phase="Deleting",uid="foo"} 0
				capi_cluster_status_phase{cluster="cluster2",namespace="ns2",phase="Failed",uid="foo"} 1
				capi_cluster_status_phase{cluster="cluster2",namespace="ns2",phase="Pending",uid="foo"} 0
				capi_cluster_status_phase{cluster="cluster2",namespace="ns2",phase="Provisioned",uid="foo"} 0
				capi_cluster_status_phase{cluster="cluster2",namespace="ns2",phase="Provisioning",uid="foo"} 0
				capi_cluster_status_phase{cluster="cluster2",namespace="ns2",phase="Unknown",uid="foo"} 0
				`,
			MetricNames: []string{"capi_cluster_status_phase"},
		},
		{
			Obj: &clusterv1alpha4.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "cluster2",
					Namespace:         "ns2",
					CreationTimestamp: metav1StartTime,
					ResourceVersion:   "10597",
					UID:               types.UID("foo"),
				},
				Status: clusterv1alpha4.ClusterStatus{
					Phase: string(clusterv1alpha4.ClusterPhaseFailed),
					Conditions: clusterv1alpha4.Conditions{
						clusterv1alpha4.Condition{
							Type:   clusterv1alpha4.InfrastructureReadyCondition,
							Status: corev1.ConditionTrue,
						},
						clusterv1alpha4.Condition{
							Type:   clusterv1alpha4.ReadyCondition,
							Status: corev1.ConditionFalse,
						},
						clusterv1alpha4.Condition{
							Type:   clusterv1alpha4.ControlPlaneInitializedCondition,
							Status: corev1.ConditionUnknown,
						},
					},
				},
			},
			Want: `
				# HELP capi_cluster_status_condition The current status conditions of a cluster.
				# TYPE capi_cluster_status_condition gauge
				capi_cluster_status_condition{cluster="cluster2",condition="ControlPlaneInitialized",namespace="ns2",status="false",uid="foo"} 0
				capi_cluster_status_condition{cluster="cluster2",condition="ControlPlaneInitialized",namespace="ns2",status="true",uid="foo"} 0
				capi_cluster_status_condition{cluster="cluster2",condition="ControlPlaneInitialized",namespace="ns2",status="unknown",uid="foo"} 1
				capi_cluster_status_condition{cluster="cluster2",condition="InfrastructureReady",namespace="ns2",status="false",uid="foo"} 0
				capi_cluster_status_condition{cluster="cluster2",condition="InfrastructureReady",namespace="ns2",status="true",uid="foo"} 1
				capi_cluster_status_condition{cluster="cluster2",condition="InfrastructureReady",namespace="ns2",status="unknown",uid="foo"} 0
				capi_cluster_status_condition{cluster="cluster2",condition="Ready",namespace="ns2",status="false",uid="foo"} 1
				capi_cluster_status_condition{cluster="cluster2",condition="Ready",namespace="ns2",status="true",uid="foo"} 0
				capi_cluster_status_condition{cluster="cluster2",condition="Ready",namespace="ns2",status="unknown",uid="foo"} 0
			`,
			MetricNames: []string{"capi_cluster_status_condition"},
		},
	}
	for i, c := range cases {
		f := clusterFactory{}
		c.Func = generator.ComposeMetricGenFuncs(f.MetricFamilyGenerators(nil, nil))
		c.Headers = generator.ExtractMetricFamilyHeaders(f.MetricFamilyGenerators(nil, nil))
		if err := c.run(); err != nil {
			t.Errorf("unexpected collecting result in %vth run:\n%s", i, err)
		}
	}
}
