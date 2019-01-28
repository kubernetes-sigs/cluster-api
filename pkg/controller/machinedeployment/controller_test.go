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

package machinedeployment

import (
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestMachineSetToDeployments(t *testing.T) {
	machineDeployment := v1alpha1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withMatchingLabels",
			Namespace: "test",
		},
		Spec: v1alpha1.MachineDeploymentSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"foo": "bar",
				},
			},
		},
	}

	machineDeplopymentList := &v1alpha1.MachineDeploymentList{
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineDeploymentList",
		},
		Items: []v1alpha1.MachineDeployment{machineDeployment},
	}

	ms1 := v1alpha1.MachineSet{
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withOwnerRef",
			Namespace: "test",
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(&machineDeployment, controllerKind),
			},
		},
	}
	ms2 := v1alpha1.MachineSet{
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "noOwnerRefNoLabels",
			Namespace: "test",
		},
	}
	ms3 := v1alpha1.MachineSet{
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineSet",
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
		machineSet v1alpha1.MachineSet
		mapObject  handler.MapObject
		expected   []reconcile.Request
	}{
		{
			machineSet: ms1,
			mapObject: handler.MapObject{
				Meta:   ms1.GetObjectMeta(),
				Object: &ms1,
			},
			expected: []reconcile.Request{},
		},
		{
			machineSet: ms2,
			mapObject: handler.MapObject{
				Meta:   ms2.GetObjectMeta(),
				Object: &ms2,
			},
			expected: nil,
		},
		{
			machineSet: ms3,
			mapObject: handler.MapObject{
				Meta:   ms3.GetObjectMeta(),
				Object: &ms3,
			},
			expected: []reconcile.Request{
				{NamespacedName: client.ObjectKey{Namespace: "test", Name: "withMatchingLabels"}},
			},
		},
	}

	v1alpha1.AddToScheme(scheme.Scheme)
	r := &ReconcileMachineDeployment{
		Client: fake.NewFakeClient(&ms1, &ms2, &ms3, machineDeplopymentList),
		scheme: scheme.Scheme,
	}

	for _, tc := range testsCases {
		got := r.MachineSetToDeployments(tc.mapObject)
		if !reflect.DeepEqual(got, tc.expected) {
			t.Errorf("Case %s. Got: %v, expected: %v", tc.machineSet.Name, got, tc.expected)
		}
	}
}

func TestGetMachineDeploymentsForMachineSet(t *testing.T) {
	machineDeployment := v1alpha1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withLabels",
			Namespace: "test",
		},
		Spec: v1alpha1.MachineDeploymentSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"foo": "bar",
				},
			},
		},
	}
	machineDeplopymentList := &v1alpha1.MachineDeploymentList{
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineDeploymentList",
		},
		Items: []v1alpha1.MachineDeployment{
			machineDeployment,
		},
	}
	ms1 := v1alpha1.MachineSet{
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "NoMatchingLabels",
			Namespace: "test",
		},
	}
	ms2 := v1alpha1.MachineSet{
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withMatchingLabels",
			Namespace: "test",
			Labels: map[string]string{
				"foo": "bar",
			},
		},
	}

	testCases := []struct {
		machineDeploymentList v1alpha1.MachineDeploymentList
		machineSet            v1alpha1.MachineSet
		expected              []*v1alpha1.MachineDeployment
	}{
		{
			machineDeploymentList: *machineDeplopymentList,
			machineSet:            ms1,
			expected:              nil,
		},
		{
			machineDeploymentList: *machineDeplopymentList,
			machineSet:            ms2,
			expected:              []*v1alpha1.MachineDeployment{&machineDeployment},
		},
	}
	v1alpha1.AddToScheme(scheme.Scheme)
	r := &ReconcileMachineDeployment{
		Client: fake.NewFakeClient(&ms1, &ms2, machineDeplopymentList),
		scheme: scheme.Scheme,
	}

	for _, tc := range testCases {
		got := r.getMachineDeploymentsForMachineSet(&tc.machineSet)
		if !reflect.DeepEqual(got, tc.expected) {
			t.Errorf("Case %s. Got: %v, expected %v", tc.machineSet.Name, got, tc.expected)
		}
	}
}

func TestGetMachineSetsForDeployment(t *testing.T) {
	machineDeployment1 := v1alpha1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withMatchingOwnerRefAndLabels",
			Namespace: "test",
			UID:       "UID",
		},
		Spec: v1alpha1.MachineDeploymentSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"foo": "bar",
				},
			},
		},
	}
	machineDeployment2 := v1alpha1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withNoMatchingOwnerRef",
			Namespace: "test",
			UID:       "unMatchingUID",
		},
		Spec: v1alpha1.MachineDeploymentSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"foo": "bar",
				},
			},
		},
	}

	ms1 := v1alpha1.MachineSet{
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withNoOwnerRef",
			Namespace: "test",
			Labels: map[string]string{
				"foo": "bar",
			},
		},
	}
	ms2 := v1alpha1.MachineSet{
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withOwnerRefAndLabels",
			Namespace: "test",
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(&machineDeployment1, controllerKind),
			},
			Labels: map[string]string{
				"foo": "bar",
			},
		},
	}
	machineSetList := &v1alpha1.MachineSetList{
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineSetList",
		},
		Items: []v1alpha1.MachineSet{
			ms1,
			ms2,
		},
	}

	testCases := []struct {
		machineDeployment v1alpha1.MachineDeployment
		expected          []*v1alpha1.MachineSet
	}{
		{
			machineDeployment: machineDeployment1,
			expected:          []*v1alpha1.MachineSet{&ms2},
		},
		{
			machineDeployment: machineDeployment2,
			expected:          []*v1alpha1.MachineSet{},
		},
	}

	v1alpha1.AddToScheme(scheme.Scheme)
	r := &ReconcileMachineDeployment{
		Client: fake.NewFakeClient(machineSetList),
		scheme: scheme.Scheme,
	}
	for _, tc := range testCases {
		got, err := r.getMachineSetsForDeployment(&tc.machineDeployment)
		if err != nil {
			t.Errorf("Failed running getMachineSetsForDeployment: %v", err)
		}
		if !reflect.DeepEqual(got, tc.expected) {
			t.Errorf("Case %s. Got: %v, expected %v", tc.machineDeployment.Name, got, tc.expected)
		}
	}
}
