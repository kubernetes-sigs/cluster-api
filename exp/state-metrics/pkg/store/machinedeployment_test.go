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

	clusterv1alpha4 "sigs.k8s.io/cluster-api/api/v1alpha4"
)

func TestMachineDeploymentStore(t *testing.T) {
	startTime := 1501569018
	metav1StartTime := metav1.Unix(int64(startTime), 0)

	cases := []generateMetricsTestCase{
		{
			Obj: &clusterv1alpha4.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "md1",
					Namespace:         "ns1",
					CreationTimestamp: metav1StartTime,
					ResourceVersion:   "10596",
					UID:               types.UID("foo"),
					OwnerReferences: []metav1.OwnerReference{
						{
							Controller: pointer.Bool(true),
							Kind:       "foo",
							Name:       "bar",
						},
					},
				},
			},
			Want: `
				# HELP capi_machinedeployment_created Unix creation timestamp
				# HELP capi_machinedeployment_labels Kubernetes labels converted to Prometheus labels.
				# HELP capi_machinedeployment_owner Information about the kubeadmcontrolplane's owner.
				# TYPE capi_machinedeployment_created gauge
				# TYPE capi_machinedeployment_labels gauge
				# TYPE capi_machinedeployment_owner gauge
				capi_machinedeployment_created{machinedeployment="md1",namespace="ns1",uid="foo"} 1.501569018e+09
				capi_machinedeployment_labels{machinedeployment="md1",namespace="ns1",uid="foo"} 1
				capi_machinedeployment_owner{machinedeployment="md1",namespace="ns1",owner_is_controller="true",owner_kind="foo",owner_name="bar",uid="foo"} 1
			`,
			MetricNames: []string{"capi_machinedeployment_labels", "capi_machinedeployment_created", "capi_machinedeployment_owner"},
		},
		{
			Obj: &clusterv1alpha4.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "md2",
					Namespace:         "ns2",
					CreationTimestamp: metav1StartTime,
					ResourceVersion:   "10596",
					UID:               types.UID("foo"),
				},
				Status: clusterv1alpha4.MachineDeploymentStatus{
					Phase: string(clusterv1alpha4.MachineDeploymentPhaseScalingDown),
				},
			},
			Want: `
				# HELP capi_machinedeployment_status_phase The machinedeployments current phase.
				# TYPE capi_machinedeployment_status_phase gauge
				capi_machinedeployment_status_phase{machinedeployment="md2",namespace="ns2",phase="Failed",uid="foo"} 0
				capi_machinedeployment_status_phase{machinedeployment="md2",namespace="ns2",phase="Running",uid="foo"} 0
				capi_machinedeployment_status_phase{machinedeployment="md2",namespace="ns2",phase="ScalingDown",uid="foo"} 1
				capi_machinedeployment_status_phase{machinedeployment="md2",namespace="ns2",phase="ScalingUp",uid="foo"} 0
				capi_machinedeployment_status_phase{machinedeployment="md2",namespace="ns2",phase="Unknown",uid="foo"} 0
			`,
			MetricNames: []string{"capi_machinedeployment_status_phase"},
		},
		{
			Obj: &clusterv1alpha4.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "md2",
					Namespace:         "ns2",
					CreationTimestamp: metav1StartTime,
					ResourceVersion:   "10596",
					UID:               types.UID("foo"),
				},
				Spec: clusterv1alpha4.MachineDeploymentSpec{
					Replicas: pointer.Int32(3),
				},
				Status: clusterv1alpha4.MachineDeploymentStatus{
					Replicas:            3,
					UpdatedReplicas:     1,
					AvailableReplicas:   1,
					ReadyReplicas:       1,
					UnavailableReplicas: 1,
				},
			},
			Want: `
				# HELP capi_machinedeployment_spec_replicas Number of desired replicas for a machinedeployment.
				# HELP capi_machinedeployment_status_replicas The number of replicas per machinedeployment.
				# HELP capi_machinedeployment_status_replicas_available The number of available replicas per machinedeployment.
				# HELP capi_machinedeployment_status_replicas_unavailable The number of unavailable replicas per machinedeployment.
				# HELP capi_machinedeployment_status_replicas_updated The number of updated replicas per machinedeployment.
				# TYPE capi_machinedeployment_spec_replicas gauge
				# TYPE capi_machinedeployment_status_replicas gauge
				# TYPE capi_machinedeployment_status_replicas_available gauge
				# TYPE capi_machinedeployment_status_replicas_unavailable gauge
				# TYPE capi_machinedeployment_status_replicas_updated gauge
				capi_machinedeployment_spec_replicas{machinedeployment="md2",namespace="ns2",uid="foo"} 3
				capi_machinedeployment_status_replicas_available{machinedeployment="md2",namespace="ns2",uid="foo"} 1
				capi_machinedeployment_status_replicas_unavailable{machinedeployment="md2",namespace="ns2",uid="foo"} 1
				capi_machinedeployment_status_replicas_updated{machinedeployment="md2",namespace="ns2",uid="foo"} 1
				capi_machinedeployment_status_replicas{machinedeployment="md2",namespace="ns2",uid="foo"} 3
			`,
			MetricNames: []string{"capi_machinedeployment_status_replicas", "capi_machinedeployment_status_replicas_available", "capi_machinedeployment_status_replicas_unavailable", "capi_machinedeployment_status_replicas_updated", "capi_machinedeployment_spec_replicas"},
		},
		{
			Obj: &clusterv1alpha4.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "md3",
					Namespace:         "ns3",
					CreationTimestamp: metav1StartTime,
					ResourceVersion:   "10596",
					UID:               types.UID("foo"),
				},
				Spec: clusterv1alpha4.MachineDeploymentSpec{
					Replicas: pointer.Int32Ptr(1),
					Strategy: &clusterv1alpha4.MachineDeploymentStrategy{
						RollingUpdate: &clusterv1alpha4.MachineRollingUpdateDeployment{
							MaxSurge:       &intstr.IntOrString{IntVal: 1},
							MaxUnavailable: &intstr.IntOrString{IntVal: 1},
						},
					},
				},
			},
			Want: `
				# HELP capi_machinedeployment_spec_strategy_rollingupdate_max_surge Maximum number of replicas that can be scheduled above the desired number of replicas during a rolling update of a machinedeployment.
				# HELP capi_machinedeployment_spec_strategy_rollingupdate_max_unavailable Maximum number of unavailable replicas during a rolling update of a machinedeployment.
				# TYPE capi_machinedeployment_spec_strategy_rollingupdate_max_surge gauge
				# TYPE capi_machinedeployment_spec_strategy_rollingupdate_max_unavailable gauge
				capi_machinedeployment_spec_strategy_rollingupdate_max_surge{machinedeployment="md3",namespace="ns3",uid="foo"} 1
				capi_machinedeployment_spec_strategy_rollingupdate_max_unavailable{machinedeployment="md3",namespace="ns3",uid="foo"} 1
			`,
			MetricNames: []string{"capi_machinedeployment_spec_strategy_rollingupdate_max_surge", "capi_machinedeployment_spec_strategy_rollingupdate_max_unavailable"},
		},
	}
	for i, c := range cases {
		f := machineDeploymentFactory{}
		c.Func = generator.ComposeMetricGenFuncs(f.MetricFamilyGenerators(nil, nil))
		c.Headers = generator.ExtractMetricFamilyHeaders(f.MetricFamilyGenerators(nil, nil))
		if err := c.run(); err != nil {
			t.Errorf("unexpected collecting result in %vth run:\n%s", i, err)
		}
	}
}
