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

package machine

import (
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openshift/cluster-api/pkg/apis/cluster/v1alpha1"
	"github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	_ reconcile.Reconciler = &ReconcileMachine{}
)

func TestReconcileRequest(t *testing.T) {
	machine1 := v1beta1.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind: "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       "create",
			Namespace:  "default",
			Finalizers: []string{v1beta1.MachineFinalizer, metav1.FinalizerDeleteDependents},
			Labels: map[string]string{
				v1beta1.MachineClusterLabelName: "testcluster",
				v1beta1.MachineClusterIDLabel:   "testcluster",
			},
		},
		Spec: v1beta1.MachineSpec{
			ProviderSpec: v1beta1.ProviderSpec{
				Value: &runtime.RawExtension{
					Raw: []byte("{}"),
				},
			},
		},
	}
	machine2 := v1beta1.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind: "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       "update",
			Namespace:  "default",
			Finalizers: []string{v1beta1.MachineFinalizer, metav1.FinalizerDeleteDependents},
			Labels: map[string]string{
				v1beta1.MachineClusterLabelName: "testcluster",
				v1beta1.MachineClusterIDLabel:   "testcluster",
			},
		},
		Spec: v1beta1.MachineSpec{
			ProviderSpec: v1beta1.ProviderSpec{
				Value: &runtime.RawExtension{
					Raw: []byte("{}"),
				},
			},
		},
	}
	time := metav1.Now()
	machine3 := v1beta1.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind: "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              "delete",
			Namespace:         "default",
			Finalizers:        []string{v1beta1.MachineFinalizer, metav1.FinalizerDeleteDependents},
			DeletionTimestamp: &time,
			Labels: map[string]string{
				v1alpha1.MachineClusterLabelName: "testcluster",
				v1beta1.MachineClusterIDLabel:    "testcluster",
			},
		},
		Spec: v1beta1.MachineSpec{
			ProviderSpec: v1beta1.ProviderSpec{
				Value: &runtime.RawExtension{
					Raw: []byte("{}"),
				},
			},
		},
	}
	clusterList := v1alpha1.ClusterList{
		TypeMeta: metav1.TypeMeta{
			Kind: "ClusterList",
		},
		Items: []v1alpha1.Cluster{
			{
				TypeMeta: metav1.TypeMeta{
					Kind: "Cluster",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "testcluster",
					Namespace: "default",
				},
			},
			{
				TypeMeta: metav1.TypeMeta{
					Kind: "Cluster",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rainbow",
					Namespace: "foo",
				},
			},
		},
	}

	type expected struct {
		createCallCount int64
		existCallCount  int64
		updateCallCount int64
		deleteCallCount int64
		result          reconcile.Result
		error           bool
	}
	testCases := []struct {
		request     reconcile.Request
		existsValue bool
		expected    expected
	}{
		{
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: machine1.Name, Namespace: machine1.Namespace}},
			expected: expected{
				createCallCount: 1,
				existCallCount:  1,
				updateCallCount: 0,
				deleteCallCount: 0,
				result:          reconcile.Result{},
				error:           false,
			},
		},
		{
			request:     reconcile.Request{NamespacedName: types.NamespacedName{Name: machine2.Name, Namespace: machine2.Namespace}},
			existsValue: true,
			expected: expected{
				createCallCount: 0,
				existCallCount:  1,
				updateCallCount: 1,
				deleteCallCount: 0,
				result:          reconcile.Result{},
				error:           false,
			},
		},
		{
			request:     reconcile.Request{NamespacedName: types.NamespacedName{Name: machine3.Name, Namespace: machine3.Namespace}},
			existsValue: true,
			expected: expected{
				createCallCount: 0,
				existCallCount:  0,
				updateCallCount: 0,
				deleteCallCount: 1,
				result:          reconcile.Result{},
				error:           false,
			},
		},
	}

	for _, tc := range testCases {
		act := newTestActuator()
		act.ExistsValue = tc.existsValue
		v1beta1.AddToScheme(scheme.Scheme)
		r := &ReconcileMachine{
			Client:   fake.NewFakeClient(&clusterList, &machine1, &machine2, &machine3),
			scheme:   scheme.Scheme,
			actuator: act,
		}

		result, err := r.Reconcile(tc.request)
		gotError := (err != nil)
		if tc.expected.error != gotError {
			var errorExpectation string
			if !tc.expected.error {
				errorExpectation = "no"
			}
			t.Errorf("Case: %s. Expected %s error, got: %v", tc.request.Name, errorExpectation, err)
		}

		if !reflect.DeepEqual(result, tc.expected.result) {
			t.Errorf("Case %s. Got: %v, expected %v", tc.request.Name, result, tc.expected.result)
		}

		if act.CreateCallCount != tc.expected.createCallCount {
			t.Errorf("Case %s. Got: %d createCallCount, expected %d", tc.request.Name, act.CreateCallCount, tc.expected.createCallCount)
		}

		if act.UpdateCallCount != tc.expected.updateCallCount {
			t.Errorf("Case %s. Got: %d updateCallCount, expected %d", tc.request.Name, act.UpdateCallCount, tc.expected.updateCallCount)
		}

		if act.ExistsCallCount != tc.expected.existCallCount {
			t.Errorf("Case %s. Got: %d existCallCount, expected %d", tc.request.Name, act.ExistsCallCount, tc.expected.existCallCount)
		}

		if act.DeleteCallCount != tc.expected.deleteCallCount {
			t.Errorf("Case %s. Got: %d deleteCallCount, expected %d", tc.request.Name, act.DeleteCallCount, tc.expected.deleteCallCount)
		}
	}
}
