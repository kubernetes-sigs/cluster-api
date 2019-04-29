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

	"github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ reconcile.Reconciler = &ReconcileMachineSet{}

func TestMachineSetToMachines(t *testing.T) {
	machineSetList := &v1beta1.MachineSetList{
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineSetList",
		},
		Items: []v1beta1.MachineSet{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "withMatchingLabels",
					Namespace: "test",
				},
				Spec: v1beta1.MachineSetSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"foo":                           "bar",
							v1beta1.MachineClusterLabelName: "test-cluster",
						},
					},
				},
			},
		},
	}
	controller := true
	m := v1beta1.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind: "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withOwnerRef",
			Namespace: "test",
			Labels: map[string]string{
				v1beta1.MachineClusterLabelName: "test-cluster",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					Name:       "Owner",
					Kind:       "MachineSet",
					Controller: &controller,
				},
			},
		},
	}
	m2 := v1beta1.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind: "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "noOwnerRefNoLabels",
			Namespace: "test",
			Labels: map[string]string{
				v1beta1.MachineClusterLabelName: "test-cluster",
			},
		},
	}
	m3 := v1beta1.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind: "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withMatchingLabels",
			Namespace: "test",
			Labels: map[string]string{
				"foo":                           "bar",
				v1beta1.MachineClusterLabelName: "test-cluster",
			},
		},
	}
	testsCases := []struct {
		machine   v1beta1.Machine
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

	v1beta1.AddToScheme(scheme.Scheme)
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
		machineSet v1beta1.MachineSet
		machine    v1beta1.Machine
		expected   bool
	}{
		{
			machineSet: v1beta1.MachineSet{},
			machine: v1beta1.Machine{
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
			machineSet: v1beta1.MachineSet{
				Spec: v1beta1.MachineSetSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"foo": "bar",
						},
					},
				},
			},
			machine: v1beta1.Machine{
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
			machineSet: v1beta1.MachineSet{},
			machine: v1beta1.Machine{
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
	m := v1beta1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "orphanMachine",
		},
	}
	ms := v1beta1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "adoptOrphanMachine",
		},
	}
	controller := true
	blockOwnerDeletion := true
	testCases := []struct {
		machineSet v1beta1.MachineSet
		machine    v1beta1.Machine
		expected   []metav1.OwnerReference
	}{
		{
			machine:    m,
			machineSet: ms,
			expected: []metav1.OwnerReference{
				{
					APIVersion:         v1beta1.SchemeGroupVersion.String(),
					Kind:               "MachineSet",
					Name:               "adoptOrphanMachine",
					UID:                "",
					Controller:         &controller,
					BlockOwnerDeletion: &blockOwnerDeletion,
				},
			},
		},
	}

	v1beta1.AddToScheme(scheme.Scheme)
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
