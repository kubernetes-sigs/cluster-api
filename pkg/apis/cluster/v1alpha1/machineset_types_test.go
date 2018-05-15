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

	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
)

func TestValidateMachineSetStrategy(t *testing.T) {
	tests := []struct {
		name string
		machineSetToTest *cluster.MachineSet
		expectError bool
	}{
		{
			name: "scenario 1: a machine with empty selector is not valid",
			machineSetToTest: &cluster.MachineSet{},
			expectError: true,
		},
		{
			name:             "scenario 2: a machine with valid selector but with empty template.Labels is not valid",
			machineSetToTest: &cluster.MachineSet{
				Spec: cluster.MachineSetSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"foo":"bar"},
					},
				},
			},
			expectError:      true,
		},
		{
			name:             "scenario 3: a machine with valid selector and with corresponding template.Labels is valid",
			machineSetToTest: &cluster.MachineSet{
				Spec: cluster.MachineSetSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"foo":"bar"},
					},
					Template: cluster.MachineTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"foo":"bar"},
						},
					},
				},
			},
			expectError:      false,
		},
		{
			name:             "scenario 4: a machine with valid selector but w/o corresponding template.Labels is not valid",
			machineSetToTest: &cluster.MachineSet{
				Spec: cluster.MachineSetSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"foo":"bar"},
					},
					Template: cluster.MachineTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"bar":"foo"},
						},
					},
				},
			},
			expectError:      true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// setup the test scenario
			ctx := genericapirequest.NewDefaultContext()
			target := v1alpha1.MachineSetStrategy{}

			// act
			errors := target.Validate(ctx, test.machineSetToTest)

			// validate
			if len(errors) > 0 && !test.expectError {
				t.Fatalf("an unexpected error was returned = %v", errors)
			}
			if test.expectError && len(errors) ==0 {
				t.Fatal("expected an error but non was returned")
			}

		})
	}

}

func crudAccessToMachineSetClient(t *testing.T, cs *clientset.Clientset) {
	instance := v1alpha1.MachineSet{
		Spec: v1alpha1.MachineSetSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{"foo":"bar"},
			},
			Template: v1alpha1.MachineTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"foo":"bar"},
				},
			},
		},
	}
	instance.Name = "instance-1"

	expected := instance.DeepCopy()
	// Defaulted fields.
	var replicas int32 = 1
	expected.Spec.Replicas = &replicas

	// When sending a storage request for a valid config,
	// it should provide CRUD access to the object.
	client := cs.ClusterV1alpha1().MachineSets("default")

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

	// Test deleting the item for delete requests.
	actual.Finalizers = nil
	if _, updateErr := client.Update(actual); updateErr != nil {
		t.Fatal(updateErr)
	}
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
