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

package machineset

import (
	"reflect"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	machinev1 "sigs.k8s.io/cluster-api/pkg/apis/machine/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestMachineSetToMachines(t *testing.T) {
	machineSetList := &machinev1.MachineSetList{
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineSetList",
		},
		Items: []machinev1.MachineSet{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "withMatchingLabels",
					Namespace: "test",
				},
				Spec: machinev1.MachineSetSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"foo": "bar",
						},
					},
				},
			},
		},
	}
	controller := true
	m := machinev1.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind: "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withOwnerRef",
			Namespace: "test",
			OwnerReferences: []metav1.OwnerReference{
				{
					Name:       "Owner",
					Kind:       "MachineSet",
					Controller: &controller,
				},
			},
		},
	}
	m2 := machinev1.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind: "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "noOwnerRefNoLabels",
			Namespace: "test",
		},
	}
	m3 := machinev1.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind: "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withMatchingLabels",
			Namespace: "test",
			Labels: map[string]string{
				"foo": "bar",
			},
		},
	}
	testsCases := []struct {
		machine   machinev1.Machine
		mapObject handler.MapObject
		expected  []reconcile.Request
	}{
		{
			machine: m,
			mapObject: handler.MapObject{
				Meta:   m.GetObjectMeta(),
				Object: &m,
			},
			expected: []reconcile.Request{},
		},
		{
			machine: m2,
			mapObject: handler.MapObject{
				Meta:   m2.GetObjectMeta(),
				Object: &m2,
			},
			expected: nil,
		},
		{
			machine: m3,
			mapObject: handler.MapObject{
				Meta:   m3.GetObjectMeta(),
				Object: &m3,
			},
			expected: []reconcile.Request{
				{NamespacedName: client.ObjectKey{Namespace: "test", Name: "withMatchingLabels"}},
			},
		},
	}

	machinev1.AddToScheme(scheme.Scheme)
	r := &ReconcileMachineSet{
		Client: fake.NewFakeClient(&m, &m2, &m3, machineSetList),
		scheme: scheme.Scheme,
	}

	for _, tc := range testsCases {
		got := r.MachineToMachineSets(tc.mapObject)
		if !reflect.DeepEqual(got, tc.expected) {
			t.Errorf("Case %s. Got: %v, expected: %v", tc.machine.Name, got, tc.expected)
		}
	}
}

func TestShouldExcludeMachine(t *testing.T) {
	controller := true
	testCases := []struct {
		machineSet machinev1.MachineSet
		machine    machinev1.Machine
		expected   bool
	}{
		{
			machineSet: machinev1.MachineSet{},
			machine: machinev1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "withNoMatchingOwnerRef",
					Namespace: "test",
					OwnerReferences: []metav1.OwnerReference{
						{
							Name:       "Owner",
							Kind:       "MachineSet",
							Controller: &controller,
						},
					},
				},
			},
			expected: true,
		},
		{
			machineSet: machinev1.MachineSet{
				Spec: machinev1.MachineSetSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"foo": "bar",
						},
					},
				},
			},
			machine: machinev1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "withMatchingLabels",
					Namespace: "test",
					Labels: map[string]string{
						"foo": "bar",
					},
				},
			},
			expected: false,
		},
		{
			machineSet: machinev1.MachineSet{},
			machine: machinev1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "withDeletionTimestamp",
					Namespace:         "test",
					DeletionTimestamp: &metav1.Time{Time: time.Now()},
					Labels: map[string]string{
						"foo": "bar",
					},
				},
			},
			expected: true,
		},
	}

	for _, tc := range testCases {
		got := shouldExcludeMachine(&tc.machineSet, &tc.machine)
		if got != tc.expected {
			t.Errorf("Case %s. Got: %v, expected: %v", tc.machine.Name, got, tc.expected)
		}
	}
}

func TestAdoptOrphan(t *testing.T) {
	m := machinev1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "orphanMachine",
		},
	}
	ms := machinev1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "adoptOrphanMachine",
		},
	}
	controller := true
	blockOwnerDeletion := true
	testCases := []struct {
		machineSet machinev1.MachineSet
		machine    machinev1.Machine
		expected   []metav1.OwnerReference
	}{
		{
			machine:    m,
			machineSet: ms,
			expected: []metav1.OwnerReference{
				{
					APIVersion:         machinev1.SchemeGroupVersion.String(),
					Kind:               "MachineSet",
					Name:               "adoptOrphanMachine",
					UID:                "",
					Controller:         &controller,
					BlockOwnerDeletion: &blockOwnerDeletion,
				},
			},
		},
	}

	machinev1.AddToScheme(scheme.Scheme)
	r := &ReconcileMachineSet{
		Client: fake.NewFakeClient(&m),
		scheme: scheme.Scheme,
	}
	for _, tc := range testCases {
		r.adoptOrphan(&tc.machineSet, &tc.machine)
		got := tc.machine.GetOwnerReferences()
		if !reflect.DeepEqual(got, tc.expected) {
			t.Errorf("Case %s. Got: %+v, expected: %+v", tc.machine.Name, got, tc.expected)
		}
	}
}
