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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	generator "k8s.io/kube-state-metrics/v2/pkg/metric_generator"
	"k8s.io/utils/pointer"

	controlplanev1alpha4 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha4"
)

func TestKubeadmControlPlaneStore(t *testing.T) {
	startTime := 1501569018
	metav1StartTime := metav1.Unix(int64(startTime), 0)

	cases := []generateMetricsTestCase{
		{
			Obj: &controlplanev1alpha4.KubeadmControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "kcp1",
					Namespace:         "ns1",
					CreationTimestamp: metav1StartTime,
					ResourceVersion:   "10596",
					UID:               types.UID("foo"),
				},
			},
			Want: `
				# HELP capi_kubeadmcontrolplane_created Unix creation timestamp
				# HELP capi_kubeadmcontrolplane_labels Kubernetes labels converted to Prometheus labels.
				# HELP capi_kubeadmcontrolplane_owner Information about the kubeadmcontrolplane's owner.
				# TYPE capi_kubeadmcontrolplane_created gauge
				# TYPE capi_kubeadmcontrolplane_labels gauge
				# TYPE capi_kubeadmcontrolplane_owner gauge
				capi_kubeadmcontrolplane_created{kubeadmcontrolplane="kcp1",namespace="ns1",uid="foo"} 1.501569018e+09
				capi_kubeadmcontrolplane_labels{kubeadmcontrolplane="kcp1",namespace="ns1",uid="foo"} 1
				capi_kubeadmcontrolplane_owner{kubeadmcontrolplane="kcp1",namespace="ns1",owner_is_controller="<none>",owner_kind="<none>",owner_name="<none>",uid="foo"} 1
			`,
			MetricNames: []string{"capi_kubeadmcontrolplane_labels", "capi_kubeadmcontrolplane_created", "capi_kubeadmcontrolplane_owner"},
		},
		{
			Obj: &controlplanev1alpha4.KubeadmControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "kcp2",
					Namespace:         "ns2",
					CreationTimestamp: metav1StartTime,
					ResourceVersion:   "10596",
					UID:               types.UID("foo"),
				},
				Spec: controlplanev1alpha4.KubeadmControlPlaneSpec{
					Replicas: pointer.Int32(2),
				},
				Status: controlplanev1alpha4.KubeadmControlPlaneStatus{
					Replicas:            2,
					ReadyReplicas:       1,
					UnavailableReplicas: 1,
					UpdatedReplicas:     1,
				},
			},
			Want: `
				# HELP capi_kubeadmcontrolplane_spec_replicas Number of desired replicas for a kubeadmcontrolplane.
				# HELP capi_kubeadmcontrolplane_status_replicas The number of replicas per kubeadmcontrolplane.
				# HELP capi_kubeadmcontrolplane_status_replicas_ready The number of ready replicas per kubeadmcontrolplane.
				# HELP capi_kubeadmcontrolplane_status_replicas_unavailable The number of unavailable replicas per kubeadmcontrolplane.
				# HELP capi_kubeadmcontrolplane_status_replicas_updated The number of updated replicas per kubeadmcontrolplane.
				# TYPE capi_kubeadmcontrolplane_spec_replicas gauge
				# TYPE capi_kubeadmcontrolplane_status_replicas gauge
				# TYPE capi_kubeadmcontrolplane_status_replicas_ready gauge
				# TYPE capi_kubeadmcontrolplane_status_replicas_unavailable gauge
				# TYPE capi_kubeadmcontrolplane_status_replicas_updated gauge
				capi_kubeadmcontrolplane_spec_replicas{kubeadmcontrolplane="kcp2",namespace="ns2",uid="foo"} 2
				capi_kubeadmcontrolplane_status_replicas_ready{kubeadmcontrolplane="kcp2",namespace="ns2",uid="foo"} 1
				capi_kubeadmcontrolplane_status_replicas_unavailable{kubeadmcontrolplane="kcp2",namespace="ns2",uid="foo"} 1
				capi_kubeadmcontrolplane_status_replicas_updated{kubeadmcontrolplane="kcp2",namespace="ns2",uid="foo"} 1
				capi_kubeadmcontrolplane_status_replicas{kubeadmcontrolplane="kcp2",namespace="ns2",uid="foo"} 2
			`,
			MetricNames: []string{"capi_kubeadmcontrolplane_status_replicas", "capi_kubeadmcontrolplane_status_replicas_ready", "capi_kubeadmcontrolplane_status_replicas_unavailable", "capi_kubeadmcontrolplane_status_replicas_updated", "capi_kubeadmcontrolplane_spec_replicas"},
		},
		{
			Obj: &controlplanev1alpha4.KubeadmControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "kcp3",
					Namespace:         "ns3",
					CreationTimestamp: metav1StartTime,
					ResourceVersion:   "10596",
					UID:               types.UID("foo"),
				},
				Spec: controlplanev1alpha4.KubeadmControlPlaneSpec{
					Replicas: pointer.Int32(1),
					RolloutStrategy: &controlplanev1alpha4.RolloutStrategy{
						RollingUpdate: &controlplanev1alpha4.RollingUpdate{
							MaxSurge: &intstr.IntOrString{IntVal: 1},
						},
					},
				},
			},
			Want: `
				# HELP capi_kubeadmcontrolplane_spec_strategy_rollingupdate_max_surge Maximum number of replicas that can be scheduled above the desired number of replicas during a rolling update of a kubeadmcontrolplane.
				# TYPE capi_kubeadmcontrolplane_spec_strategy_rollingupdate_max_surge gauge
				capi_kubeadmcontrolplane_spec_strategy_rollingupdate_max_surge{kubeadmcontrolplane="kcp3",namespace="ns3",uid="foo"} 1
			`,
			MetricNames: []string{"capi_kubeadmcontrolplane_spec_strategy_rollingupdate_max_surge"},
		},
		{
			Obj: &controlplanev1alpha4.KubeadmControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "kcp4",
					Namespace:         "ns4",
					CreationTimestamp: metav1StartTime,
					ResourceVersion:   "10596",
					UID:               types.UID("foo"),
				},
				Spec: controlplanev1alpha4.KubeadmControlPlaneSpec{
					Version: "v9.9.9",
				},
			},
			Want: `
				# HELP capi_kubeadmcontrolplane_info Information about a kubeadmcontrolplane.
				# TYPE capi_kubeadmcontrolplane_info gauge
        capi_kubeadmcontrolplane_info{kubeadmcontrolplane="kcp4",namespace="ns4",version="v9.9.9",uid="foo"} 1
			`,
			MetricNames: []string{"capi_kubeadmcontrolplane_info"},
		},
	}
	for i, c := range cases {
		f := kubeadmControlPlaneFactory{}
		c.Func = generator.ComposeMetricGenFuncs(f.MetricFamilyGenerators(nil, nil))
		c.Headers = generator.ExtractMetricFamilyHeaders(f.MetricFamilyGenerators(nil, nil))
		if err := c.run(); err != nil {
			t.Errorf("unexpected collecting result in %vth run:\n%s", i, err)
		}
	}
}
