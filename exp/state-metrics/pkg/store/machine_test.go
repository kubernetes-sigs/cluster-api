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
	"k8s.io/utils/pointer"

	clusterv1alpha4 "sigs.k8s.io/cluster-api/api/v1alpha4"
)

func TestMachineStore(t *testing.T) {
	startTime := 1501569018
	metav1StartTime := metav1.Unix(int64(startTime), 0)

	cases := []generateMetricsTestCase{
		{
			Obj: &clusterv1alpha4.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "m1",
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
				# HELP capi_machine_created Unix creation timestamp
				# HELP capi_machine_labels Kubernetes labels converted to Prometheus labels.
				# HELP capi_machine_owner Information about the machine's owner.
				# TYPE capi_machine_created gauge
				# TYPE capi_machine_labels gauge
				# TYPE capi_machine_owner gauge
				capi_machine_created{machine="m1",namespace="ns1",uid="foo"} 1.501569018e+09
				capi_machine_labels{machine="m1",namespace="ns1",uid="foo"} 1
        capi_machine_owner{machine="m1",namespace="ns1",owner_is_controller="true",owner_kind="foo",owner_name="bar",uid="foo"} 1
		`,
			MetricNames: []string{"capi_machine_labels", "capi_machine_created", "capi_machine_owner"},
		},
		{
			Obj: &clusterv1alpha4.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "m2",
					Namespace:         "ns2",
					CreationTimestamp: metav1StartTime,
					ResourceVersion:   "10597",
					UID:               types.UID("foo"),
				},
				Status: clusterv1alpha4.MachineStatus{
					Phase: string(clusterv1alpha4.MachinePhaseProvisioning),
				},
			},
			Want: `
				# HELP capi_machine_status_phase The machines current phase.
				# TYPE capi_machine_status_phase gauge
				capi_machine_status_phase{machine="m2",namespace="ns2",phase="Deleted",uid="foo"} 0
				capi_machine_status_phase{machine="m2",namespace="ns2",phase="Deleting",uid="foo"} 0
				capi_machine_status_phase{machine="m2",namespace="ns2",phase="Failed",uid="foo"} 0
				capi_machine_status_phase{machine="m2",namespace="ns2",phase="Pending",uid="foo"} 0
				capi_machine_status_phase{machine="m2",namespace="ns2",phase="Provisioned",uid="foo"} 0
				capi_machine_status_phase{machine="m2",namespace="ns2",phase="Provisioning",uid="foo"} 1
				capi_machine_status_phase{machine="m2",namespace="ns2",phase="Running",uid="foo"} 0
				capi_machine_status_phase{machine="m2",namespace="ns2",phase="Unknown",uid="foo"} 0
			`,
			MetricNames: []string{"capi_machine_status_phase"},
		},
		{
			Obj: &clusterv1alpha4.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "m2",
					Namespace:         "ns2",
					CreationTimestamp: metav1StartTime,
					ResourceVersion:   "10597",
					UID:               types.UID("foo"),
				},
				Status: clusterv1alpha4.MachineStatus{
					Conditions: clusterv1alpha4.Conditions{
						clusterv1alpha4.Condition{
							Type:   clusterv1alpha4.PreDrainDeleteHookSucceededCondition,
							Status: corev1.ConditionTrue,
						},
						clusterv1alpha4.Condition{
							Type:   clusterv1alpha4.PreTerminateDeleteHookSucceededCondition,
							Status: corev1.ConditionFalse,
						},
						clusterv1alpha4.Condition{
							Type:   clusterv1alpha4.MachineNodeHealthyCondition,
							Status: corev1.ConditionUnknown,
						},
					},
				},
			},
			Want: `
				# HELP capi_machine_status_condition The current status conditions of a machine.
				# TYPE capi_machine_status_condition gauge
				capi_machine_status_condition{condition="NodeHealthy",machine="m2",namespace="ns2",status="false",uid="foo"} 0
				capi_machine_status_condition{condition="NodeHealthy",machine="m2",namespace="ns2",status="true",uid="foo"} 0
				capi_machine_status_condition{condition="NodeHealthy",machine="m2",namespace="ns2",status="unknown",uid="foo"} 1
				capi_machine_status_condition{condition="PreDrainDeleteHookSucceeded",machine="m2",namespace="ns2",status="false",uid="foo"} 0
				capi_machine_status_condition{condition="PreDrainDeleteHookSucceeded",machine="m2",namespace="ns2",status="true",uid="foo"} 1
				capi_machine_status_condition{condition="PreDrainDeleteHookSucceeded",machine="m2",namespace="ns2",status="unknown",uid="foo"} 0
				capi_machine_status_condition{condition="PreTerminateDeleteHookSucceeded",machine="m2",namespace="ns2",status="false",uid="foo"} 1
				capi_machine_status_condition{condition="PreTerminateDeleteHookSucceeded",machine="m2",namespace="ns2",status="true",uid="foo"} 0
				capi_machine_status_condition{condition="PreTerminateDeleteHookSucceeded",machine="m2",namespace="ns2",status="unknown",uid="foo"} 0
		`,
			MetricNames: []string{"capi_machine_status_condition"},
		},
		{
			Obj: &clusterv1alpha4.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "m4",
					Namespace:         "ns4",
					CreationTimestamp: metav1StartTime,
					ResourceVersion:   "10596",
					UID:               types.UID("foo"),
				},
				Status: clusterv1alpha4.MachineStatus{
					NodeRef: &corev1.ObjectReference{
						APIVersion: "v1",
						Kind:       "Node",
						Name:       "foo-m-somehash",
					},
				},
			},
			Want: `
				# HELP capi_machine_status_noderef Information about the machine's node reference.
				# TYPE capi_machine_status_noderef gauge
				capi_machine_status_noderef{machine="m4",name="foo-m-somehash",namespace="ns4",uid="foo"} 1
	`,
			MetricNames: []string{"capi_machine_status_noderef"},
		},
		{
			Obj: &clusterv1alpha4.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "m5",
					Namespace:         "ns5",
					CreationTimestamp: metav1StartTime,
					ResourceVersion:   "10596",
					UID:               types.UID("foo"),
				},
				Spec: clusterv1alpha4.MachineSpec{
					ProviderID:    pointer.String("openstack:///m5"),
					FailureDomain: pointer.String("foo"),
				},
				Status: clusterv1alpha4.MachineStatus{
					Version: pointer.String("v9.9.9"),
					Addresses: clusterv1alpha4.MachineAddresses{
						clusterv1alpha4.MachineAddress{
							Type:    clusterv1alpha4.MachineInternalIP,
							Address: "192.168.0.2",
						},
					},
				},
			},
			Want: `
				# HELP capi_machine_info Information about a machine.
				# TYPE capi_machine_info gauge
				capi_machine_info{failure_domain="foo",internal_ip="192.168.0.2",machine="m5",namespace="ns5",provider_id="openstack:///m5",version="v9.9.9",uid="foo"} 1
			`,
			MetricNames: []string{"capi_machine_info"},
		},
	}
	for i, c := range cases {
		f := machineFactory{}
		c.Func = generator.ComposeMetricGenFuncs(f.MetricFamilyGenerators(nil, nil))
		c.Headers = generator.ExtractMetricFamilyHeaders(f.MetricFamilyGenerators(nil, nil))
		if err := c.run(); err != nil {
			t.Errorf("unexpected collecting result in %vth run:\n%s", i, err)
		}
	}
}
