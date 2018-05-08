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
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	controllerlib "github.com/kubernetes-incubator/apiserver-builder/pkg/controller"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	core "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/common"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1/testutil"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/fake"
	v1alpha1listers "sigs.k8s.io/cluster-api/pkg/client/listers_generated/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/controller/sharedinformers"
)

var (
	noTimestamp = metav1.Time{}
)

func ms(name string, replicas int, selector map[string]string, timestamp metav1.Time) *v1alpha1.MachineSet {
	return &v1alpha1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			CreationTimestamp: timestamp,
			Namespace:         metav1.NamespaceDefault,
		},
		Spec: v1alpha1.MachineSetSpec{
			Replicas: func() *int32 { i := int32(replicas); return &i }(),
			Selector: metav1.LabelSelector{MatchLabels: selector},
			Template: v1alpha1.MachineTemplateSpec{},
		},
	}
}

func newMSWithStatus(name string, specReplicas, statusReplicas int, selector map[string]string) *v1alpha1.MachineSet {
	ms := ms(name, specReplicas, selector, noTimestamp)
	ms.Status = v1alpha1.MachineSetStatus{
		Replicas: int32(statusReplicas),
	}
	return ms
}

func newMachineDeployment(name string, replicas int, revisionHistoryLimit *int32, maxSurge, maxUnavailable *intstr.IntOrString, selector map[string]string) *v1alpha1.MachineDeployment {
	localReplicas := int32(replicas)
	localMinReadySeconds := int32(300)
	defaultMaxSurge := intstr.FromInt(0)
	defaultMaxUnavailable := intstr.FromInt(0)

	d := v1alpha1.MachineDeployment{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1alpha1/v1beta1"},
		ObjectMeta: metav1.ObjectMeta{
			UID:         uuid.NewUUID(),
			Name:        name,
			Namespace:   metav1.NamespaceDefault,
			Annotations: make(map[string]string),
		},
		Spec: v1alpha1.MachineDeploymentSpec{
			Strategy: v1alpha1.MachineDeploymentStrategy{
				Type: common.RollingUpdateMachineDeploymentStrategyType,
				RollingUpdate: &v1alpha1.MachineRollingUpdateDeployment{
					MaxUnavailable: &defaultMaxUnavailable,
					MaxSurge:       &defaultMaxSurge,
				},
			},
			Replicas: &localReplicas,
			Selector: metav1.LabelSelector{MatchLabels: selector},
			Template: v1alpha1.MachineTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: selector,
				},
				Spec: v1alpha1.MachineSpec{},
			},
			MinReadySeconds:      &localMinReadySeconds,
			RevisionHistoryLimit: revisionHistoryLimit,
		},
	}
	if maxSurge != nil {
		d.Spec.Strategy.RollingUpdate.MaxSurge = maxSurge
	}
	if maxUnavailable != nil {
		d.Spec.Strategy.RollingUpdate.MaxUnavailable = maxUnavailable
	}
	return &d
}

func newMachineSet(d *v1alpha1.MachineDeployment, name string, replicas int) *v1alpha1.MachineSet {
	return &v1alpha1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			UID:             uuid.NewUUID(),
			Namespace:       metav1.NamespaceDefault,
			Labels:          d.Spec.Selector.MatchLabels,
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(d, controllerKind)},
		},
		Spec: v1alpha1.MachineSetSpec{
			Selector: d.Spec.Selector,
			Replicas: func() *int32 { i := int32(replicas); return &i }(),
			Template: d.Spec.Template,
		},
	}
}

func newMinimalMachineSet(name string, replicas int) *v1alpha1.MachineSet {
	return &v1alpha1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			UID:       uuid.NewUUID(),
			Namespace: metav1.NamespaceDefault,
		},
		Spec: v1alpha1.MachineSetSpec{
			Replicas: func() *int32 { i := int32(replicas); return &i }(),
		},
	}
}

func addDeploymentProperties(d *v1alpha1.MachineDeployment, ms *v1alpha1.MachineSet) *v1alpha1.MachineSet {
	ms.ObjectMeta.Labels = d.Spec.Selector.MatchLabels
	ms.ObjectMeta.OwnerReferences = []metav1.OwnerReference{*metav1.NewControllerRef(d, controllerKind)}
	ms.Spec.Selector = d.Spec.Selector
	ms.Spec.Template = d.Spec.Template
	return ms
}

func machineDeploymentControllerReconcile(t *testing.T, cs *clientset.Clientset, controller *MachineDeploymentController) {
	instance := v1alpha1.MachineDeployment{}
	instance.Name = "instance-1"
	replicas := int32(0)
	minReadySeconds := int32(0)
	instance.Spec.Replicas = &replicas
	instance.Spec.MinReadySeconds = &minReadySeconds
	instance.Spec.Selector = metav1.LabelSelector{MatchLabels: map[string]string{"foo": "barr"}}
	instance.Spec.Template.Labels = map[string]string{"foo": "barr"}

	expectedKey := "default/instance-1"

	// When creating a new object, it should invoke the reconcile method.
	cluster := testutil.GetVanillaCluster()
	cluster.Name = "cluster-1"
	if _, err := cs.ClusterV1alpha1().Clusters(metav1.NamespaceDefault).Create(&cluster); err != nil {
		t.Fatal(err)
	}
	client := cs.ClusterV1alpha1().MachineDeployments(metav1.NamespaceDefault)
	before := make(chan struct{})
	after := make(chan struct{})
	var aftOnce, befOnce sync.Once

	actualKey := ""
	var actualErr error

	// Setup test callbacks to be called when the message is reconciled.
	// Sometimes reconcile is called multiple times, so use Once to prevent closing the channels again.
	controller.BeforeReconcile = func(key string) {
		actualKey = key
		befOnce.Do(func() { close(before) })
	}
	controller.AfterReconcile = func(key string, err error) {
		actualKey = key
		actualErr = err
		aftOnce.Do(func() { close(after) })
	}

	// Create an instance
	if _, err := client.Create(&instance); err != nil {
		t.Fatal(err)
	}
	defer client.Delete(instance.Name, &metav1.DeleteOptions{})

	// Verify reconcile function is called against the correct key
	select {
	case <-before:
		if actualKey != expectedKey {
			t.Fatalf(
				"Reconcile function was not called with the correct key.\nActual:\t%+v\nExpected:\t%+v",
				actualKey, expectedKey)
		}
		if actualErr != nil {
			t.Fatal(actualErr)
		}
	case <-time.After(time.Second * 2):
		t.Fatalf("reconcile never called")
	}

	select {
	case <-after:
		if actualKey != expectedKey {
			t.Fatalf(
				"Reconcile function was not called with the correct key.\nActual:\t%+v\nExpected:\t%+v",
				actualKey, expectedKey)
		}
		if actualErr != nil {
			t.Fatal(actualErr)
		}
	case <-time.After(time.Second * 2):
		t.Fatalf("reconcile never finished")
	}
}

type fixture struct {
	t *testing.T

	client *fake.Clientset
	// Objects to put in the store.
	dLister       []*v1alpha1.MachineDeployment
	msLister      []*v1alpha1.MachineSet
	machineLister []*v1alpha1.Machine

	// Actions expected to happen on the client. Objects from here are also
	// preloaded into NewSimpleFake.
	actions []core.Action
	objects []runtime.Object
}

func (f *fixture) expectGetDeploymentAction(d *v1alpha1.MachineDeployment) {
	action := core.NewGetAction(schema.GroupVersionResource{Group: "cluster.k8s.io", Version: "v1alpha1", Resource: "machinedeployments"}, d.Namespace, d.Name)
	f.actions = append(f.actions, action)
}

func (f *fixture) expectUpdateDeploymentStatusAction(d *v1alpha1.MachineDeployment) {
	action := core.NewUpdateAction(schema.GroupVersionResource{Group: "cluster.k8s.io", Version: "v1alpha1", Resource: "machinedeployments"}, d.Namespace, d)
	action.Subresource = "status"
	f.actions = append(f.actions, action)
}

func (f *fixture) expectUpdateDeploymentAction(d *v1alpha1.MachineDeployment) {
	action := core.NewUpdateAction(schema.GroupVersionResource{Group: "cluster.k8s.io", Version: "v1alpha1", Resource: "machinedeployments"}, d.Namespace, d)
	f.actions = append(f.actions, action)
}

func (f *fixture) expectCreateMSAction(ms *v1alpha1.MachineSet) {
	f.actions = append(f.actions, core.NewCreateAction(schema.GroupVersionResource{Group: "cluster.k8s.io", Version: "v1alpha1", Resource: "machinesets"}, ms.Namespace, ms))
}

func newFixture(t *testing.T) *fixture {
	f := &fixture{}
	f.t = t
	f.objects = []runtime.Object{}
	return f
}

func (f *fixture) newController() *MachineDeploymentControllerImpl {
	machineIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	machineLister := v1alpha1listers.NewMachineLister(machineIndexer)
	machineSetIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	machineSetLister := v1alpha1listers.NewMachineSetLister(machineSetIndexer)
	machineDeploymentIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	machineDeploymentLister := v1alpha1listers.NewMachineDeploymentLister(machineDeploymentIndexer)

	fakeClient := fake.NewSimpleClientset(f.objects...)
	f.client = fakeClient
	controller := &MachineDeploymentControllerImpl{}
	controller.machineClient = fakeClient
	controller.mdLister = machineDeploymentLister
	controller.msLister = machineSetLister
	controller.mLister = machineLister
	controller.informers = &sharedinformers.SharedInformers{}
	controller.informers.WorkerQueues = map[string]*controllerlib.QueueWorker{}
	queue := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "MachineDeployment")
	controller.informers.WorkerQueues["MachineDeployment"] = &controllerlib.QueueWorker{queue, 10, "MachineDeployment", nil}

	for _, d := range f.dLister {
		err := machineDeploymentIndexer.Add(d)
		if err != nil {
			f.t.Fatal(err)
		}
	}
	for _, ms := range f.msLister {
		err := machineSetIndexer.Add(ms)
		if err != nil {
			f.t.Fatal(err)
		}
	}
	for _, machine := range f.machineLister {
		err := machineIndexer.Add(machine)
		if err != nil {
			f.t.Fatal(err)
		}
	}
	return controller
}

func (f *fixture) runExpectError(deploymentName *v1alpha1.MachineDeployment, startInformers bool) {
	f.runParams(deploymentName, startInformers, true)
}

func (f *fixture) run(deploymentName *v1alpha1.MachineDeployment) {
	f.runParams(deploymentName, true, false)
}

func (f *fixture) runParams(deploymentName *v1alpha1.MachineDeployment, startInformers bool, expectError bool) {
	c := f.newController()

	err := c.Reconcile(deploymentName)
	if !expectError && err != nil {
		f.t.Errorf("error syncing deployment: %v", err)
	} else if expectError && err == nil {
		f.t.Error("expected error syncing deployment, got nil")
	}

	actions := filterInformerActions(f.client.Actions())

	if len(actions) != len(f.actions) {
		f.t.Errorf("Got %d actions, expected %d actions", len(actions), len(f.actions))
	}

	for i, action := range actions {
		if len(f.actions) < i+1 {
			f.t.Errorf("%d unexpected actions: %+v", len(actions)-len(f.actions), actions[i:])
			break
		}

		expectedAction := f.actions[i]
		if !(expectedAction.Matches(action.GetVerb(), action.GetResource().Resource) && action.GetSubresource() == expectedAction.GetSubresource()) {
			f.t.Errorf("Expected\n\t%#v\ngot\n\t%#v", expectedAction, action)
			continue
		}
	}

	if len(f.actions) > len(actions) {
		f.t.Errorf("%d additional expected actions:%+v", len(f.actions)-len(actions), f.actions[len(actions):])
	}
}

func filterInformerActions(actions []core.Action) []core.Action {
	ret := []core.Action{}
	for _, action := range actions {
		if len(action.GetNamespace()) == 0 &&
			(action.Matches("list", "machines") ||
				action.Matches("list", "machinedeployments") ||
				action.Matches("list", "machinesets") ||
				action.Matches("watch", "machines") ||
				action.Matches("watch", "machinedeployments") ||
				action.Matches("watch", "machinesets")) {
			continue
		}
		ret = append(ret, action)
	}

	return ret
}

func TestSyncDeploymentCreatesMachineSet(t *testing.T) {
	f := newFixture(t)

	d := newMachineDeployment("foo", 1, nil, nil, nil, map[string]string{"foo": "bar"})
	f.dLister = append(f.dLister, d)
	f.objects = append(f.objects, d)

	ms := newMachineSet(d, "randomName", 1)

	f.expectCreateMSAction(ms)
	f.expectUpdateDeploymentStatusAction(d)

	f.run(d)
}

func TestSyncDeploymentDontDoAnythingDuringDeletion(t *testing.T) {
	f := newFixture(t)

	d := newMachineDeployment("foo", 1, nil, nil, nil, map[string]string{"foo": "bar"})
	now := metav1.Now()
	d.DeletionTimestamp = &now
	f.dLister = append(f.dLister, d)
	f.objects = append(f.objects, d)

	f.run(d)
}

func TestGetMachineSetsForDeployment(t *testing.T) {
	tests := []struct {
		name                 string
		noDeploymentSelector bool
		diffCtrlRef          bool
		diffLabels           bool
		noCtrlRef            bool
		expectedMachineSets  int
	}{
		{
			name:                "scenario 1. machine set returned.",
			expectedMachineSets: 1,
		},
		{
			name:                "scenario 2. machine set with diff controller ref, machine set not returned.",
			diffCtrlRef:         true,
			expectedMachineSets: 0,
		},
		{
			name:                 "scenario 3. deployment with no selector, machine set not returned.",
			noDeploymentSelector: true,
			expectedMachineSets:  0,
		},
		{
			name:                "scenario 4. machine set with non-matching labels not returned.",
			diffLabels:          true,
			expectedMachineSets: 0,
		},
		{
			name:                "scenario 5. machine set with no controller ref, machine set not returned.",
			noCtrlRef:           true,
			expectedMachineSets: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Log(test.name)
			d1 := newMachineDeployment("d1", 1, nil, nil, nil, map[string]string{"foo": "bar"})
			d2 := newMachineDeployment("d2", 1, nil, nil, nil, map[string]string{"foo": "bar2"})
			ms := newMachineSet(d1, "ms", 1)

			if test.diffCtrlRef {
				ms.ObjectMeta.OwnerReferences = []metav1.OwnerReference{*metav1.NewControllerRef(d2, controllerKind)}
			}
			if test.noDeploymentSelector {
				d1.Spec.Selector = metav1.LabelSelector{}
			}
			if test.diffLabels {
				ms.ObjectMeta.Labels = map[string]string{"foo": "bar2"}
			}
			if test.noCtrlRef {
				ms.ObjectMeta.OwnerReferences = nil
			}

			f := newFixture(t)

			f.dLister = append(f.dLister, d1, d2)
			f.msLister = append(f.msLister, ms)
			f.objects = append(f.objects, d1, d2, ms)

			c := f.newController()

			msList, err := c.getMachineSetsForDeployment(d1)
			if err != nil {
				t.Fatalf("unexpected err calling getMachineSetsForDeployment, %v", err)
			}
			if test.expectedMachineSets != len(msList) {
				t.Fatalf("got %v machine sets, expected %v machine sets, %v", len(msList), test.expectedMachineSets, msList)
			}
		})
	}
}

func hasExpectedMachineNames(t *testing.T, mList *v1alpha1.MachineList, msName string, numMSMachines int) {
	var names, expectedNames []string
	for _, m := range mList.Items {
		names = append(names, m.Name)
	}
	sort.Strings(names)

	for i := 0; i < numMSMachines; i++ {
		expectedNames = append(expectedNames, fmt.Sprintf("%v-machine-%v", msName, i))
	}
	if !reflect.DeepEqual(names, expectedNames) {
		t.Fatalf("got %v machine names, expected %v machine names for %v", names, expectedNames, msName)
	}
}

func TestGetMachineMapForMachineSets(t *testing.T) {
	tests := []struct {
		name                    string
		numMS1Machines          int
		numMS2Machines          int
		addMachineWithNoCtrlRef bool
		expectedUIDs            int
	}{
		{
			name:           "scenario 1. multiple machine sets, one populated, one empty",
			numMS1Machines: 3,
			numMS2Machines: 0,
			expectedUIDs:   2,
		},
		{
			name:           "scenario 2. multiple machine sets, two populated",
			numMS1Machines: 3,
			numMS2Machines: 2,
			expectedUIDs:   2,
		},
		{
			name:           "scenario 3. multiple machine sets, both empty",
			numMS1Machines: 0,
			numMS2Machines: 0,
			expectedUIDs:   2,
		},
		{
			name:                    "scenario 4. skip machine with no controller ref.",
			numMS1Machines:          3,
			numMS2Machines:          2,
			addMachineWithNoCtrlRef: true,
			expectedUIDs:            2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Log(test.name)
			d1 := newMachineDeployment("foo", 1, nil, nil, nil, map[string]string{"foo": "bar"})
			ms1 := newMachineSet(d1, "ms1", 1)
			ms2 := newMachineSet(d1, "ms2", 1)

			f := newFixture(t)

			for i := 0; i < test.numMS1Machines; i++ {
				m := generateMachineFromMS(ms1, i)
				f.machineLister = append(f.machineLister, m)
				f.objects = append(f.objects, m)
			}

			for i := 0; i < test.numMS2Machines; i++ {
				m := generateMachineFromMS(ms2, i)
				f.machineLister = append(f.machineLister, m)
				f.objects = append(f.objects, m)
			}

			f.dLister = append(f.dLister, d1)
			f.msLister = append(f.msLister, ms1, ms2)
			f.objects = append(f.objects, d1, ms1, ms2)

			if test.addMachineWithNoCtrlRef {
				m := generateMachineFromMS(ms1, 99)
				m.ObjectMeta.OwnerReferences = nil
				f.machineLister = append(f.machineLister, m)
				f.objects = append(f.objects, m)
			}

			c := f.newController()

			machineMap, err := c.getMachineMapForDeployment(d1, f.msLister)
			if err != nil {
				t.Fatalf("getMachineMapForDeployment() error: %v", err)
			}

			if test.expectedUIDs != len(machineMap) {
				t.Fatalf("got %v machine set UIDs, expected %v machine set UIDs", test.expectedUIDs, len(machineMap))
			}

			if test.numMS1Machines != len(machineMap[ms1.UID].Items) {
				t.Fatalf("got %v machines, expected %v machines for ms1", test.numMS1Machines, len(machineMap[ms1.UID].Items))
			}
			if test.numMS2Machines != len(machineMap[ms2.UID].Items) {
				t.Fatalf("got %v machines, expected %v machines for ms2", test.numMS2Machines, len(machineMap[ms2.UID].Items))
			}

			hasExpectedMachineNames(t, machineMap[ms1.UID], ms1.Name, test.numMS1Machines)
			hasExpectedMachineNames(t, machineMap[ms2.UID], ms2.Name, test.numMS2Machines)
		})
	}
}

func getMachineSetActions(actions []core.Action) []core.Action {
	var filteredActions []core.Action
	for _, action := range actions {
		if action.GetResource().Resource == "machinesets" {
			filteredActions = append(filteredActions, action)
		}
	}
	return filteredActions
}

func TestAddMachineSet(t *testing.T) {
	tests := []struct {
		name             string
		stripOwnerRef    bool
		ownerDoesntExist bool
		isDeleting       bool
		diffLabel        bool
		expectCreation   bool
	}{
		{
			name:           "scenario 1. machine set with controller ref.",
			expectCreation: true,
		},
		{
			name:           "scenario 2. machine set with no controller ref.",
			stripOwnerRef:  true,
			expectCreation: true,
		},
		{
			name:           "scenario 3. machine set that is being deleted.",
			stripOwnerRef:  true,
			isDeleting:     true,
			expectCreation: false,
		},
		{
			name:             "scenario 4. machine set with controller ref that controller doesn't exist.",
			ownerDoesntExist: true,
			expectCreation:   false,
		},
		{
			name:           "scenario 5. machine set with no controller ref, no matching deployment.",
			stripOwnerRef:  true,
			diffLabel:      true,
			expectCreation: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Log(test.name)
			d := newMachineDeployment("d", 1, nil, nil, nil, map[string]string{"foo": "bar"})
			ms := newMachineSet(d, "ms", 1)

			if test.stripOwnerRef {
				ms.ObjectMeta.OwnerReferences = nil
			}
			if test.isDeleting {
				now := metav1.Now()
				ms.DeletionTimestamp = &now
			}
			if test.diffLabel {
				ms.ObjectMeta.Labels = map[string]string{"foo": "bar2"}
			}

			f := newFixture(t)

			if !test.ownerDoesntExist {
				f.dLister = append(f.dLister, d)
			}
			f.objects = append(f.objects, d, ms)

			c := f.newController()

			c.addMachineSet(ms)

			queue := c.informers.WorkerQueues["MachineDeployment"].Queue

			if !test.expectCreation {
				if queue.Len() != 0 {
					t.Fatalf("got %d queued items, expected %d queued items", queue.Len(), 0)
				}
			}
			if test.expectCreation {
				if queue.Len() != 1 {
					t.Fatalf("got %d queued items, expected %d queued items", queue.Len(), 1)
				}
				verifyQueuedKey(t, queue, d)
			}
			if test.isDeleting {
				if queue.Len() != 0 {
					t.Fatalf("got %d queued items, expected %d queued items", queue.Len(), 0)
				}

			}

		})
	}
}

func TestUpdateMachineSet(t *testing.T) {
	tests := []struct {
		name                string
		sameResourceVersion bool
		noOldCtrlRef        bool
		noNewCtrlRef        bool
		diffNewCtrlRef      bool
		diffNewCtrlExists   bool
		diffNewLabels       bool
		expectOldReconcile  bool
		expectNewReconcile  bool
	}{
		{
			name:                "scenario 1. same resource version, no-op",
			sameResourceVersion: true,
		},
		{
			name:               "scenario 2. no change to controller ref, queue old controller ref",
			expectOldReconcile: true,
		},
		{
			name:               "scenario 3. old controller ref to new different controller ref that exists, queue old controller ref and new controller ref",
			diffNewCtrlRef:     true,
			diffNewCtrlExists:  true,
			expectOldReconcile: true,
			expectNewReconcile: true,
		},
		{
			name:               "scenario 4. old controller ref to new different controller ref that doesn't exist, queue old controller ref",
			diffNewCtrlRef:     true,
			diffNewCtrlExists:  false,
			expectOldReconcile: true,
		},
		{
			name:               "scenario 5. no old controller ref, to new controller ref exists, queue new controller ref",
			noOldCtrlRef:       true,
			diffNewCtrlRef:     true,
			diffNewCtrlExists:  true,
			expectNewReconcile: true,
		},
		{
			name:              "scenario 6. no old controller ref, to new controller ref doesn't exist, no controller ref to queue",
			noOldCtrlRef:      true,
			diffNewCtrlRef:    true,
			diffNewCtrlExists: false,
		},
		{
			name:               "scenario 7. old controller ref, to no new controller ref, orphaned, queue old controller ref",
			noNewCtrlRef:       true,
			expectOldReconcile: true,
		},
		{
			name:         "scenario 8. no controller ref, to no new controller ref, no controller ref to queue",
			noOldCtrlRef: true,
			noNewCtrlRef: true,
		},
		{
			name:               "scenario 9. no controller ref, to no new controller ref, label change, found deployment by label, queue found deployment",
			noOldCtrlRef:       true,
			noNewCtrlRef:       true,
			diffNewCtrlExists:  true,
			diffNewLabels:      true,
			expectNewReconcile: true,
		},
		{
			name:              "scenario 10. no controller ref, to no new controller ref, label change, deployment not found, no deployment to queue",
			noOldCtrlRef:      true,
			noNewCtrlRef:      true,
			diffNewCtrlExists: false,
			diffNewLabels:     true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Log(test.name)

			d1 := newMachineDeployment("d1", 1, nil, nil, nil, map[string]string{"foo": "bar"})
			d2 := newMachineDeployment("d2", 1, nil, nil, nil, map[string]string{"foo": "bar2"})
			oriMS := newMachineSet(d1, "ms", 1)

			oldMS := *oriMS
			newMS := *oriMS

			if !test.sameResourceVersion {
				bumpResourceVersion(&newMS)
			}
			if test.noOldCtrlRef {
				oldMS.ObjectMeta.OwnerReferences = nil
			}
			if test.noNewCtrlRef {
				newMS.ObjectMeta.OwnerReferences = nil
			}
			if test.diffNewCtrlRef {
				newMS.ObjectMeta.OwnerReferences = []metav1.OwnerReference{*metav1.NewControllerRef(d2, controllerKind)}
			}
			if test.diffNewLabels {
				newMS.ObjectMeta.Labels = map[string]string{"foo": "bar2"}
			}

			f := newFixture(t)

			f.dLister = append(f.dLister, d1)
			if test.diffNewCtrlExists {
				f.dLister = append(f.dLister, d2)
			}
			f.msLister = append(f.msLister, &oldMS)
			f.objects = append(f.objects, d1, &oldMS)

			c := f.newController()

			c.updateMachineSet(&oldMS, &newMS)

			expectedReconcileCount := 0
			if test.expectOldReconcile {
				expectedReconcileCount++
			}
			if test.expectNewReconcile {
				expectedReconcileCount++
			}

			queue := c.informers.WorkerQueues["MachineDeployment"].Queue

			if queue.Len() != expectedReconcileCount {
				t.Fatalf("got %d queued items, expected %d queued items", queue.Len(), expectedReconcileCount)
			}
			if test.expectOldReconcile {
				verifyQueuedKey(t, queue, d1)
			}
			if test.expectNewReconcile {
				verifyQueuedKey(t, queue, d2)
			}
		})
	}
}

func TestDeleteMachineSet(t *testing.T) {
	tests := []struct {
		name             string
		stripOwnerRef    bool
		ownerDoesntExist bool
		expectDelete     bool
	}{
		{
			name:         "scenario 1. has controller ref that exists",
			expectDelete: true,
		},
		{
			name:             "scenario 2. has controller ref that does not exist",
			ownerDoesntExist: true,
			expectDelete:     false,
		},
		{
			name:          "scenario 3. no controller ref",
			stripOwnerRef: true,
			expectDelete:  false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Log(test.name)
			d := newMachineDeployment("foo", 1, nil, nil, nil, map[string]string{"foo": "bar"})
			ms := newMachineSet(d, "ms", 1)

			if test.stripOwnerRef {
				ms.ObjectMeta.OwnerReferences = nil
			}

			f := newFixture(t)

			if !test.ownerDoesntExist {
				f.dLister = append(f.dLister, d)
			}
			f.objects = append(f.objects, d, ms)

			c := f.newController()

			c.deleteMachineSet(ms)

			queue := c.informers.WorkerQueues["MachineDeployment"].Queue

			if !test.expectDelete {
				if queue.Len() != 0 {
					t.Fatalf("got %d queued items, expected %d queued items", queue.Len(), 0)
				}
			}
			if test.expectDelete {
				if queue.Len() != 1 {
					t.Fatalf("got %d queued items, expected %d queued items", queue.Len(), 1)
				}
				verifyQueuedKey(t, queue, d)
			}
		})
	}
}

func bumpResourceVersion(obj metav1.Object) {
	ver, _ := strconv.ParseInt(obj.GetResourceVersion(), 10, 32)
	obj.SetResourceVersion(strconv.FormatInt(ver+1, 10))
}

// generateMachineFromMS creates a machine, with the input MachineSet's selector and its template
func generateMachineFromMS(ms *v1alpha1.MachineSet, count int) *v1alpha1.Machine {
	trueVar := true
	return &v1alpha1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%v-machine-%v", ms.Name, count),
			Namespace: ms.Namespace,
			Labels:    ms.Spec.Selector.MatchLabels,
			OwnerReferences: []metav1.OwnerReference{
				{UID: ms.UID, APIVersion: "v1alpha1", Kind: "MachineSet", Name: ms.Name, Controller: &trueVar},
			},
		},
		Spec: ms.Spec.Template.Spec,
	}
}

func verifyQueuedKey(t *testing.T, queue workqueue.RateLimitingInterface, d *v1alpha1.MachineDeployment) {
	key, done := queue.Get()
	if key == nil || done {
		t.Fatalf("failed to enqueue controller.")
	}
	expectedKey, err := cache.MetaNamespaceKeyFunc(d)
	if err != nil {
		t.Fatalf("failed to get key for deployment.")
	}
	if expectedKey != key {
		t.Fatalf("got %v key, expected %v key", key, expectedKey)
	}
}
