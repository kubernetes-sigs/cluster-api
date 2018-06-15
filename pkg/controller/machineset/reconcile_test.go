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
	"fmt"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/diff"
	clienttesting "k8s.io/client-go/testing"
	core "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"

	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/fake"
	v1alpha1listers "sigs.k8s.io/cluster-api/pkg/client/listers_generated/cluster/v1alpha1"
)

const (
	labelKey = "type"
)

type fakeMachineLister struct {
	indexer cache.Indexer
}

// List lists all Machines in the indexer.
func (s *fakeMachineLister) List(selector labels.Selector) (ret []*v1alpha1.Machine, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.Machine))
	})
	return ret, err
}

// Machines returns an object that can list and get Machines.
func (s *fakeMachineLister) Machines(namespace string) v1alpha1listers.MachineNamespaceLister {
	return fakeMachineNamespaceLister{indexer: s.indexer, namespace: namespace}
}

type fakeMachineNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

func (s fakeMachineNamespaceLister) List(selector labels.Selector) (ret []*v1alpha1.Machine, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.Machine))
	})
	return ret, err
}

func (s fakeMachineNamespaceLister) Get(name string) (*v1alpha1.Machine, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha1.Resource("machine"), name)
	}
	return obj.(*v1alpha1.Machine), nil
}

func TestMachineSetControllerReconcileHandler(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name                string
		startingMachineSets []*v1alpha1.MachineSet
		startingMachines    []*v1alpha1.Machine
		machineSetToSync    string
		namespaceToSync     string
		confirmationTimeout *time.Duration
		deletionTimestamp   *time.Time
		expectedMachine     *v1alpha1.Machine
		expectedActions     []string
		expectedError       bool
	}{
		{
			name:                "scenario 1: the current state of the cluster is empty, thus a machine is created.",
			startingMachineSets: []*v1alpha1.MachineSet{createMachineSet(1, "foo", "bar1", "acme")},
			startingMachines:    nil,
			machineSetToSync:    "foo",
			namespaceToSync:     "acme",
			expectedActions:     []string{"create"},
			expectedMachine:     machineFromMachineSet(createMachineSet(1, "foo", "bar1", "acme"), "bar1"),
		},
		{
			name:                "scenario 2: the current state of the cluster is too small, thus a machine is created.",
			startingMachineSets: []*v1alpha1.MachineSet{createMachineSet(3, "foo", "bar3", "acme")},
			startingMachines:    []*v1alpha1.Machine{machineFromMachineSet(createMachineSet(3, "foo", "bar1", "acme"), "bar1"), machineFromMachineSet(createMachineSet(3, "foo", "bar2", "acme"), "bar2")},
			machineSetToSync:    "foo",
			namespaceToSync:     "acme",
			expectedActions:     []string{"create"},
			expectedMachine:     machineFromMachineSet(createMachineSet(3, "foo", "bar3", "acme"), "bar3"),
		},
		{
			name:                "scenario 3: the current state of the cluster is equal to the desired one, no machine resource is created.",
			startingMachineSets: []*v1alpha1.MachineSet{createMachineSet(2, "foo", "bar3", "acme")},
			startingMachines:    []*v1alpha1.Machine{machineFromMachineSet(createMachineSet(2, "foo", "bar1", "acme"), "bar1"), machineFromMachineSet(createMachineSet(2, "foo", "bar2", "acme"), "bar2")},
			machineSetToSync:    "foo",
			namespaceToSync:     "acme",
			expectedActions:     []string{},
		},
		{
			name:                "scenario 4: the current state of the cluster is bigger than the desired one, thus a machine is deleted.",
			startingMachineSets: []*v1alpha1.MachineSet{createMachineSet(0, "foo", "bar2", "acme")},
			startingMachines:    []*v1alpha1.Machine{machineFromMachineSet(createMachineSet(1, "foo", "bar1", "acme"), "bar1")},
			machineSetToSync:    "foo",
			namespaceToSync:     "acme",
			expectedActions:     []string{"delete"},
		},
		{
			name:                "scenario 5: the current state of the cluster is bigger than the desired one, thus machines are deleted.",
			startingMachineSets: []*v1alpha1.MachineSet{createMachineSet(0, "foo", "bar2", "acme")},
			startingMachines:    []*v1alpha1.Machine{machineFromMachineSet(createMachineSet(2, "foo", "bar1", "acme"), "bar1"), machineFromMachineSet(createMachineSet(2, "foo", "bar2", "acme"), "bar2")},
			machineSetToSync:    "foo",
			namespaceToSync:     "acme",
			expectedActions:     []string{"delete", "delete"},
		},
		{
			name:                "scenario 6: the current machine has different labels than the given machineSet, thus a machine is created.",
			startingMachineSets: []*v1alpha1.MachineSet{createMachineSet(1, "foo", "bar2", "acme")},
			startingMachines:    []*v1alpha1.Machine{setDifferentLabels(machineFromMachineSet(createMachineSet(1, "foo", "bar1", "acme"), "bar1"))},
			machineSetToSync:    "foo",
			namespaceToSync:     "acme",
			expectedActions:     []string{"create"},
			expectedMachine:     machineFromMachineSet(createMachineSet(1, "foo", "bar2", "acme"), "bar2"),
		},
		{
			name:                "scenario 7: the current machine is missing owner refs, machine should be adopted.",
			startingMachineSets: []*v1alpha1.MachineSet{createMachineSet(1, "foo", "bar2", "acme")},
			startingMachines:    []*v1alpha1.Machine{machineWithoutOwnerRefs(createMachineSet(1, "foo", "bar1", "acme"), "bar1")},
			machineSetToSync:    "foo",
			namespaceToSync:     "acme",
			expectedActions:     []string{"update"},
			expectedMachine:     machineFromMachineSet(createMachineSet(1, "foo", "bar2", "acme"), "bar1"),
		},
		{
			name:                "scenario 8: the current machine has different controller ref, thus a machine is created.",
			startingMachineSets: []*v1alpha1.MachineSet{createMachineSet(1, "foo", "bar2", "acme")},
			startingMachines:    []*v1alpha1.Machine{setDifferentOwnerUID(machineFromMachineSet(createMachineSet(1, "foo", "bar1", "acme"), "bar1"))},
			machineSetToSync:    "foo",
			namespaceToSync:     "acme",
			expectedActions:     []string{"create"},
			expectedMachine:     machineFromMachineSet(createMachineSet(1, "foo", "bar2", "acme"), "bar2"),
		},
		{
			name:                "scenario 9: the current machine is being deleted, is still counted towards the machine set, no machine resource is created.",
			startingMachineSets: []*v1alpha1.MachineSet{createMachineSet(1, "foo", "bar2", "acme")},
			startingMachines:    []*v1alpha1.Machine{setMachineDeleting(machineFromMachineSet(createMachineSet(1, "foo", "bar1", "acme"), "bar1"))},
			machineSetToSync:    "foo",
			namespaceToSync:     "acme",
			expectedActions:     []string{},
		},
		{
			name:                "scenario 10: the current machine has no controller refs, owner refs preserved, machine should be adopted.",
			startingMachineSets: []*v1alpha1.MachineSet{createMachineSet(1, "foo", "bar2", "acme")},
			startingMachines:    []*v1alpha1.Machine{setNonControllerRef(setDifferentOwnerUID(machineFromMachineSet(createMachineSet(1, "foo", "bar1", "acme"), "bar1")))},
			machineSetToSync:    "foo",
			namespaceToSync:     "acme",
			expectedActions:     []string{"update"},
			expectedMachine:     machineWithMultipleOwnerRefs(createMachineSet(1, "foo", "bar2", "acme"), "bar1"),
		},
		{
			name:                "scenario 11: create confirmation timed out, err.",
			startingMachineSets: []*v1alpha1.MachineSet{createMachineSet(1, "foo", "bar1", "acme")},
			startingMachines:    nil,
			machineSetToSync:    "foo",
			namespaceToSync:     "acme",
			expectedError:       true,
		},
		{
			name:                "scenario 12: delete confirmation timed out, err.",
			startingMachineSets: []*v1alpha1.MachineSet{createMachineSet(0, "foo", "bar2", "acme")},
			startingMachines:    []*v1alpha1.Machine{machineFromMachineSet(createMachineSet(1, "foo", "bar1", "acme"), "bar1")},
			machineSetToSync:    "foo",
			namespaceToSync:     "acme",
			expectedError:       true,
		},
		{
			name:                "scenario 13: delete confirmation accepts delete non-zero timestamp.",
			startingMachineSets: []*v1alpha1.MachineSet{createMachineSet(0, "foo", "bar2", "acme")},
			startingMachines:    []*v1alpha1.Machine{machineFromMachineSet(createMachineSet(1, "foo", "bar1", "acme"), "bar1")},
			machineSetToSync:    "foo",
			namespaceToSync:     "acme",
			deletionTimestamp:   &now,
			expectedActions:     []string{"delete"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stateConfirmationTimeout = 1 * time.Millisecond

			// setup the test scenario
			rObjects := []runtime.Object{}
			machinesIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
			for _, amachine := range test.startingMachines {
				err := machinesIndexer.Add(amachine)
				if err != nil {
					t.Fatal(err)
				}
				rObjects = append(rObjects, amachine)
			}
			machineSetIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
			for _, amachineset := range test.startingMachineSets {
				err := machineSetIndexer.Add(amachineset)
				if err != nil {
					t.Fatal(err)
				}
				rObjects = append(rObjects, amachineset)
			}
			fakeClient := fake.NewSimpleClientset(rObjects...)

			machineLister := fakeMachineLister{indexer: machinesIndexer}
			machineSetLister := v1alpha1listers.NewMachineSetLister(machineSetIndexer)
			target := &MachineSetControllerImpl{}
			target.clusterAPIClient = fakeClient
			target.machineSetLister = machineSetLister
			target.machineLister = &machineLister

			fakeClient.PrependReactor("create", "machines", func(action core.Action) (bool, runtime.Object, error) {
				if test.expectedError {
					return true, nil, errors.NewNotFound(v1alpha1.Resource("machine"), "somemachine")
				}
				if test.expectedMachine != nil {
					machineLister.indexer.Add(test.expectedMachine)
				}
				return true, test.expectedMachine, nil
			})

			fakeClient.PrependReactor("delete", "machines", func(action core.Action) (bool, runtime.Object, error) {
				if test.deletionTimestamp != nil {
					machineLister.indexer.Delete(test.startingMachines[0])
					m := test.startingMachines[0].DeepCopy()
					timestamp := metav1.NewTime(*test.deletionTimestamp)
					m.ObjectMeta.DeletionTimestamp = &timestamp
					machineLister.indexer.Add(m)
					return true, nil, nil
				}
				if test.expectedError {
					return false, nil, nil
				}
				for i, action := range test.expectedActions {
					if action == "delete" {
						machineLister.indexer.Delete(test.startingMachines[i])
					}
				}
				return true, nil, nil
			})

			// act
			machineSetToTest, err := target.Get(test.namespaceToSync, test.machineSetToSync)
			if err != nil {
				t.Fatal(err)
			}
			err = target.Reconcile(machineSetToTest)

			if test.expectedError != (err != nil) {
				t.Fatalf("Unexpected reconcile err: got %v, expected %v. %v", (err != nil), test.expectedError, err)
				return
			}
			if test.expectedError {
				return
			}

			// validate
			actions := fakeClient.Actions()
			actions = getFilteredActions(actions, "machines")
			if len(actions) != len(test.expectedActions) {
				t.Fatalf("got %d actions, expected %d actions; got %v actions, expected %v actions", len(actions), len(test.expectedActions), actions, test.expectedActions)
			}
			for i, verb := range test.expectedActions {
				if actions[i].GetVerb() != verb {
					t.Fatalf("unexpected action: %v, expected %s", actions[i], verb)
				}
			}

			if test.expectedMachine != nil {
				// we take only the first item in line
				var actualMachine *v1alpha1.Machine
				for _, action := range actions {
					if action.GetVerb() == "create" {
						createAction, ok := action.(clienttesting.CreateAction)
						if !ok {
							t.Fatalf("unexpected action %#v", action)
						}
						actualMachine = createAction.GetObject().(*v1alpha1.Machine)
						break
					}
					if action.GetVerb() == "update" {
						updateAction, ok := action.(clienttesting.UpdateAction)
						if !ok {
							t.Fatalf("unexpected action %#v", action)
						}
						actualMachine = updateAction.GetObject().(*v1alpha1.Machine)
						break
					}
				}

				if !equality.Semantic.DeepEqual(actualMachine, test.expectedMachine) {
					t.Fatalf("actual machine is different from the expected one: %v", diff.ObjectDiff(test.expectedMachine, actualMachine))
				}
			}
		})
	}
}

func createMachineSet(replicas int, machineSetName string, machineName string, namespace string) *v1alpha1.MachineSet {
	replicasInt32 := int32(replicas)
	return &v1alpha1.MachineSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "MachineSet",
			APIVersion: v1alpha1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      machineSetName,
			Namespace: namespace,
		},
		Spec: v1alpha1.MachineSetSpec{
			Replicas:        &replicasInt32,
			MinReadySeconds: 600,
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{labelKey: "strongMachine"},
			},
			Template: v1alpha1.MachineTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:      machineName,
					Namespace: namespace,
					Labels:    map[string]string{labelKey: "strongMachine"},
				},
				Spec: v1alpha1.MachineSpec{
					ProviderConfig: v1alpha1.ProviderConfig{
						Value: &runtime.RawExtension{Raw: []byte("some provider specific configuration data")},
					},
				},
			},
		},
	}
}

func machineWithoutOwnerRefs(machineSet *v1alpha1.MachineSet, name string) *v1alpha1.Machine {
	m := &v1alpha1.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Machine",
			APIVersion: v1alpha1.SchemeGroupVersion.String(),
		},
		ObjectMeta: machineSet.Spec.Template.ObjectMeta,
		Spec:       machineSet.Spec.Template.Spec,
	}

	m.Name = name
	m.GenerateName = fmt.Sprintf("%s-", machineSet.Name)

	return m
}

func machineFromMachineSet(machineSet *v1alpha1.MachineSet, name string) *v1alpha1.Machine {
	m := machineWithoutOwnerRefs(machineSet, name)
	m.ObjectMeta.OwnerReferences = []metav1.OwnerReference{*metav1.NewControllerRef(machineSet, controllerKind)}
	return m
}

func machineWithMultipleOwnerRefs(machineSet *v1alpha1.MachineSet, name string) *v1alpha1.Machine {
	controller := false
	existingRef := metav1.NewControllerRef(machineSet, controllerKind)
	existingRef.UID = "NotMe"
	existingRef.Controller = &controller

	m := machineWithoutOwnerRefs(machineSet, name)
	m.ObjectMeta.OwnerReferences = []metav1.OwnerReference{*existingRef, *metav1.NewControllerRef(machineSet, controllerKind)}
	return m
}

func setDifferentOwnerUID(m *v1alpha1.Machine) *v1alpha1.Machine {
	m.ObjectMeta.OwnerReferences[0].UID = "NotMe"
	return m
}

func setMachineDeleting(m *v1alpha1.Machine) *v1alpha1.Machine {
	now := metav1.NewTime(time.Now())
	m.ObjectMeta.DeletionTimestamp = &now
	return m
}

func setDifferentLabels(m *v1alpha1.Machine) *v1alpha1.Machine {
	labels := m.GetLabels()
	labels[labelKey] = "NOTME"
	m.ObjectMeta.SetLabels(labels)
	return m
}

func setNonControllerRef(m *v1alpha1.Machine) *v1alpha1.Machine {
	controller := false
	m.ObjectMeta.OwnerReferences[0].Controller = &controller
	return m
}

func getFilteredActions(actions []clienttesting.Action, resource string) []clienttesting.Action {
	var filteredActions []clienttesting.Action
	for _, action := range actions {
		if action.GetResource().Resource == resource {
			filteredActions = append(filteredActions, action)
		}
	}
	return filteredActions
}
