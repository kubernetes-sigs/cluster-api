/*
Copyright 2018 The Kubernetes Authors.

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

package v1alpha1_test

import (
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/common"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
)

func TestMachineDeploymentValidationStrategy(t *testing.T) {
	var badReplicaCount int32 = -1
	var goodReplicaCount int32
	var badStrategyType common.MachineDeploymentStrategyType = "BAD-STRATEGY"
	var goodStrategyType common.MachineDeploymentStrategyType = "RollingUpdate"
	badCount := intstr.FromInt(-3)
	badPercent := intstr.FromString("-50%")
	goodCount := intstr.FromInt(2)
	goodPercent := intstr.FromString("15%")
	zeroCount := intstr.FromInt(0)
	zeroPercent := intstr.FromString("0%")
	over100Percent := intstr.FromString("101%")

	tests := []struct {
		name                    string
		machineDeploymentToTest *cluster.MachineDeployment
		expectError             bool
	}{
		{
			name: "scenario 1: a machine deployment with empty selector is not valid",
			machineDeploymentToTest: &cluster.MachineDeployment{
				Spec: cluster.MachineDeploymentSpec{
					Replicas: &goodReplicaCount,
				},
			},
			expectError:             true,
		},
		{
			name: "scenario 2: a machine deployment with valid selector but with empty template.Labels is not valid",
			machineDeploymentToTest: &cluster.MachineDeployment{
				Spec: cluster.MachineDeploymentSpec{
					Replicas: &goodReplicaCount,
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"foo": "bar"},
					},
				},
			},
			expectError: true,
		},
		{
			name: "scenario 3: a machine deployment with valid selector and with corresponding template.Labels is valid",
			machineDeploymentToTest: &cluster.MachineDeployment{
				Spec: cluster.MachineDeploymentSpec{
					Replicas: &goodReplicaCount,
					Strategy: cluster.MachineDeploymentStrategy{
						Type: "RollingUpdate",
					},
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"foo": "bar"},
					},
					Template: cluster.MachineTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"foo": "bar"},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "scenario 4: a machine deployment with valid selector but w/o corresponding template.Labels is not valid",
			machineDeploymentToTest: &cluster.MachineDeployment{
				Spec: cluster.MachineDeploymentSpec{
					Replicas: &goodReplicaCount,
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"foo": "bar"},
					},
					Template: cluster.MachineTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"bar": "foo"},
						},
					},
				},
			},
			expectError: true,
		},
		{
			name: "scenario 5: a machine deployment with bad replicas",
			machineDeploymentToTest: &cluster.MachineDeployment{
				Spec: cluster.MachineDeploymentSpec{
					Replicas: &badReplicaCount,
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"foo": "bar"},
					},
					Template: cluster.MachineTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"foo": "bar"},
						},
					},
				},
			},
			expectError: true,
		},
		{
			name: "scenario 6: a machine deployment with good replicas",
			machineDeploymentToTest: &cluster.MachineDeployment{
				Spec: cluster.MachineDeploymentSpec{
					Replicas: &goodReplicaCount,
					Strategy: cluster.MachineDeploymentStrategy{
						Type: "RollingUpdate",
					},
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"foo": "bar"},
					},
					Template: cluster.MachineTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"foo": "bar"},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "scenario 7: a machine deployment with bad strategy",
			machineDeploymentToTest: &cluster.MachineDeployment{
				Spec: cluster.MachineDeploymentSpec{
					Replicas: &goodReplicaCount,
					Strategy: cluster.MachineDeploymentStrategy{
						Type: badStrategyType,
					},
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"foo": "bar"},
					},
					Template: cluster.MachineTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"foo": "bar"},
						},
					},
				},
			},
			expectError: true,
		},
		{
			name: "scenario 8: a machine deployment with good strategy",
			machineDeploymentToTest: &cluster.MachineDeployment{
				Spec: cluster.MachineDeploymentSpec{
					Replicas: &goodReplicaCount,
					Strategy: cluster.MachineDeploymentStrategy{
						Type: goodStrategyType,
					},
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"foo": "bar"},
					},
					Template: cluster.MachineTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"foo": "bar"},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "scenario 9: a machine deployment with bad MaxUnavailable count",
			machineDeploymentToTest: getRollingUpdateMachineDeployment(&badCount, nil),
			expectError: true,
		},
		{
			name: "scenario 10: a machine deployment with bad MaxUnavailable percent",
			machineDeploymentToTest: getRollingUpdateMachineDeployment(&badPercent, nil),
			expectError: true,
		},
		{
			name: "scenario 11: a machine deployment with good MaxUnavailable count",
			machineDeploymentToTest: getRollingUpdateMachineDeployment(&goodCount, nil),
			expectError: false,
		},
		{
			name: "scenario 12: a machine deployment with good MaxUnavailable percent",
			machineDeploymentToTest: getRollingUpdateMachineDeployment(&goodPercent, nil),
			expectError: false,
		},
		{
			name: "scenario 13: a machine deployment with over 100 MaxUnavailable percent",
			machineDeploymentToTest: getRollingUpdateMachineDeployment(&over100Percent, nil),
			expectError: true,
		},
		{
			name: "scenario 14: a machine deployment with zero MaxUnavailable count",
			machineDeploymentToTest: getRollingUpdateMachineDeployment(&zeroCount, nil),
			expectError: false,
		},
		{
			name: "scenario 15: a machine deployment with zero MaxUnavailable percent",
			machineDeploymentToTest: getRollingUpdateMachineDeployment(&zeroPercent, nil),
			expectError: false,
		},
		{
			name: "scenario 16: a machine deployment with bad MaxSurge count",
			machineDeploymentToTest: getRollingUpdateMachineDeployment(nil, &badCount),
			expectError: true,
		},
		{
			name: "scenario 17: a machine deployment with bad MaxSurge percent",
			machineDeploymentToTest: getRollingUpdateMachineDeployment(nil, &badPercent),
			expectError: true,
		},
		{
			name: "scenario 18: a machine deployment with good MaxSurge count",
			machineDeploymentToTest: getRollingUpdateMachineDeployment(nil, &goodCount),
			expectError: false,
		},
		{
			name: "scenario 19: a machine deployment with good MaxSurge percent",
			machineDeploymentToTest: getRollingUpdateMachineDeployment(nil, &goodPercent),
			expectError: false,
		},
		{
			name: "scenario 20: a machine deployment with over 100 MaxSurge percent",
			machineDeploymentToTest: getRollingUpdateMachineDeployment(nil, &over100Percent),
			expectError: false,
		},
		{
			name: "scenario 21: a machine deployment with zero MaxSurge count",
			machineDeploymentToTest: getRollingUpdateMachineDeployment(nil, &zeroCount),
			expectError: false,
		},
		{
			name: "scenario 22: a machine deployment with zero MaxSurge percent",
			machineDeploymentToTest: getRollingUpdateMachineDeployment(nil, &zeroPercent),
			expectError: false,
		},
		{
			name: "scenario 23: a machine deployment with bad MaxUnavailable/MaxSurge both 0",
			machineDeploymentToTest: getRollingUpdateMachineDeployment(&zeroCount, &zeroCount),
			expectError: true,
		},
		{
			name: "scenario 24: a machine deployment with bad MaxUnavailable/MaxSurge both 0%",
			machineDeploymentToTest: getRollingUpdateMachineDeployment(&zeroPercent, &zeroPercent),
			expectError: true,
		},
		{
			name: "scenario 25: a machine deployment with good MaxUnavailable count, MaxSurge percent",
			machineDeploymentToTest: getRollingUpdateMachineDeployment(&goodCount, &goodPercent),
			expectError: false,
		},
		{
			name: "scenario 26: a machine deployment with good MaxUnavailable percent, MaxSurge count",
			machineDeploymentToTest: getRollingUpdateMachineDeployment(&goodPercent, &goodCount),
			expectError: false,
		},
		{
			name: "scenario 27: a machine deployment with good MaxUnavailable count, MaxSurge count",
			machineDeploymentToTest: getRollingUpdateMachineDeployment(&goodCount, &goodCount),
			expectError: false,
		},
		{
			name: "scenario 28: a machine deployment with good MaxUnavailable percent, MaxSurge percent",
			machineDeploymentToTest: getRollingUpdateMachineDeployment(&goodPercent, &goodPercent),
			expectError: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// setup the test scenario
			ctx := genericapirequest.NewDefaultContext()
			target := v1alpha1.MachineDeploymentValidationStrategy{}

			// act
			errors := target.Validate(ctx, test.machineDeploymentToTest)

			// validate
			if len(errors) > 0 && !test.expectError {
				t.Fatalf("an unexpected error was returned = %v", errors)
			}
			if test.expectError && len(errors) == 0 {
				t.Fatal("expected an error but non was returned")
			}

		})
	}
}

func getRollingUpdateMachineDeployment(unavailable, surge *intstr.IntOrString) *cluster.MachineDeployment {
	var goodReplicaCount int32
	d := cluster.MachineDeployment{
		Spec: cluster.MachineDeploymentSpec{
			Replicas: &goodReplicaCount,
			Strategy: cluster.MachineDeploymentStrategy{
				Type: "RollingUpdate",
				RollingUpdate: &cluster.MachineRollingUpdateDeployment{
				},
			},
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{"foo": "bar"},
			},
			Template: cluster.MachineTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"foo": "bar"},
				},
			},
		},
	}
	if unavailable != nil {
		d.Spec.Strategy.RollingUpdate.MaxUnavailable = unavailable
	}
	if surge != nil {
		d.Spec.Strategy.RollingUpdate.MaxSurge = surge
	}
	return &d
}

func crudAccessToMachineDeploymentClient(t *testing.T, cs *clientset.Clientset) {
	instance := v1alpha1.MachineDeployment{
		Spec: v1alpha1.MachineDeploymentSpec{

			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{"foo": "bar"},
			},
			Template: v1alpha1.MachineTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"foo": "bar"},
				},
			},
		},
	}
	instance.Name = "instance-1"

	expected := instance.DeepCopy()
	// Defaulted fields.
	var replicas int32 = 1
	expected.Spec.Replicas = &replicas
	var minReadySeconds int32
	expected.Spec.MinReadySeconds = &minReadySeconds
	var limit int32 = 1
	expected.Spec.RevisionHistoryLimit = &limit
	var deadline int32 = 600
	expected.Spec.ProgressDeadlineSeconds = &deadline
	expected.Spec.Strategy.Type = common.RollingUpdateMachineDeploymentStrategyType
	unavailable := intstr.FromInt(0)
	surge := intstr.FromInt(1)
	rollingUpdate := v1alpha1.MachineRollingUpdateDeployment{
		MaxUnavailable: &unavailable,
		MaxSurge: &surge,
	}
	expected.Spec.Strategy.RollingUpdate = &rollingUpdate

	// When sending a storage request for a valid config,
	// it should provide CRUD access to the object.
	client := cs.ClusterV1alpha1().MachineDeployments("default")

	// Test that the create request returns success.
	actual, err := client.Create(&instance)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Delete(instance.Name, &metav1.DeleteOptions{})
	if !reflect.DeepEqual(actual.Spec, expected.Spec) {
		t.Fatalf(
			"Default fields were not set correctly.\nActual:\t%+v\nExpected:\t%+v",
			actual.Spec, expected.Spec)
	}

	// Test getting the created item for list requests.
	result, err := client.List(metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if itemLength := len(result.Items); itemLength != 1 {
		t.Fatalf("Number of items in Items list should be 1, but is %d.", itemLength)
	}
	if resultSpec := result.Items[0].Spec; !reflect.DeepEqual(resultSpec, expected.Spec) {
		t.Fatalf(
			"Item returned from list is not equal to the expected item.\nActual:\t%+v\nExpected:\t%+v",
			resultSpec, expected.Spec)
	}

	// Test getting the created item for get requests.
	actual, err = client.Get(instance.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual.Spec, expected.Spec) {
		t.Fatalf(
			"Item returned from get is not equal to the expected item.\nActual:\t%+v\nExpected:\t%+v",
			actual.Spec, expected.Spec)
	}

	actual.Finalizers = nil
	// Test updating the item, removing the finalizer.
	if _, updateErr := client.Update(actual); updateErr != nil {
		t.Fatal(updateErr)
	}
	// Test deleting the item for delete requests.
	if deleteErr := client.Delete(instance.Name, &metav1.DeleteOptions{}); deleteErr != nil {
		t.Fatal(deleteErr)
	}
	result, err = client.List(metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if itemLength := len(result.Items); itemLength != 0 {
		t.Fatalf("Number of items in Items list should be 0, but is %d.", itemLength)
	}
}
