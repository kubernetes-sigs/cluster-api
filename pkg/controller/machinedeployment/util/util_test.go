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

package util

import (
	"fmt"
	"math/rand"
	"reflect"
	"sort"
	"strconv"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apiserver/pkg/storage/names"
	core "k8s.io/client-go/testing"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/common"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/fake"
)

func addListMSReactor(fakeClient *fake.Clientset, obj runtime.Object) *fake.Clientset {
	fakeClient.AddReactor("list", "machinesets", func(action core.Action) (handled bool, ret runtime.Object, err error) {
		return true, obj, nil
	})
	return fakeClient
}

func addListMachinesReactor(fakeClient *fake.Clientset, obj runtime.Object) *fake.Clientset {
	fakeClient.AddReactor("list", "machines", func(action core.Action) (handled bool, ret runtime.Object, err error) {
		return true, obj, nil
	})
	return fakeClient
}

func addGetMSReactor(fakeClient *fake.Clientset, obj runtime.Object) *fake.Clientset {
	msList, ok := obj.(*v1alpha1.MachineSetList)
	fakeClient.AddReactor("get", "machinesets", func(action core.Action) (handled bool, ret runtime.Object, err error) {
		name := action.(core.GetAction).GetName()
		if ok {
			for _, ms := range msList.Items {
				if ms.Name == name {
					return true, &ms, nil
				}
			}
		}
		return false, nil, fmt.Errorf("could not find the requested machine set: %s", name)

	})
	return fakeClient
}

func addUpdateMSReactor(fakeClient *fake.Clientset) *fake.Clientset {
	fakeClient.AddReactor("update", "machinesets", func(action core.Action) (handled bool, ret runtime.Object, err error) {
		obj := action.(core.UpdateAction).GetObject().(*v1alpha1.MachineSet)
		return true, obj, nil
	})
	return fakeClient
}

func addUpdateMachinesReactor(fakeClient *fake.Clientset) *fake.Clientset {
	fakeClient.AddReactor("update", "machines", func(action core.Action) (handled bool, ret runtime.Object, err error) {
		obj := action.(core.UpdateAction).GetObject().(*v1alpha1.Machine)
		return true, obj, nil
	})
	return fakeClient
}

func generateMSWithLabel(labels map[string]string, image string) v1alpha1.MachineSet {
	return v1alpha1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:   names.SimpleNameGenerator.GenerateName("machineset"),
			Labels: labels,
		},
		Spec: v1alpha1.MachineSetSpec{
			Replicas: func(i int32) *int32 { return &i }(1),
			Selector: metav1.LabelSelector{MatchLabels: labels},
			Template: v1alpha1.MachineTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: v1alpha1.MachineSpec{},
			},
		},
	}
}

func newDControllerRef(d *v1alpha1.MachineDeployment) *metav1.OwnerReference {
	isController := true
	return &metav1.OwnerReference{
		APIVersion: "clusters/v1alpha",
		Kind:       "MachineDeployment",
		Name:       d.GetName(),
		UID:        d.GetUID(),
		Controller: &isController,
	}
}

// generateMS creates a machine set, with the input deployment's template as its template
func generateMS(deployment v1alpha1.MachineDeployment) v1alpha1.MachineSet {
	template := deployment.Spec.Template.DeepCopy()
	return v1alpha1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			UID:             randomUID(),
			Name:            names.SimpleNameGenerator.GenerateName("machineset"),
			Labels:          template.Labels,
			OwnerReferences: []metav1.OwnerReference{*newDControllerRef(&deployment)},
		},
		Spec: v1alpha1.MachineSetSpec{
			Replicas: new(int32),
			Template: *template,
			Selector: metav1.LabelSelector{MatchLabels: template.Labels},
		},
	}
}

func randomUID() types.UID {
	return types.UID(strconv.FormatInt(rand.Int63(), 10))
}

// generateDeployment creates a deployment, with the input image as its template
func generateDeployment(image string) v1alpha1.MachineDeployment {
	machineLabels := map[string]string{"name": image}
	return v1alpha1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        image,
			Annotations: make(map[string]string),
		},
		Spec: v1alpha1.MachineDeploymentSpec{
			Replicas: func(i int32) *int32 { return &i }(1),
			Selector: metav1.LabelSelector{MatchLabels: machineLabels},
			Template: v1alpha1.MachineTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: machineLabels,
				},
				Spec: v1alpha1.MachineSpec{},
			},
		},
	}
}

func generateMachineTemplateSpec(name, nodeName string, annotations, labels map[string]string) v1alpha1.MachineTemplateSpec {
	return v1alpha1.MachineTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Annotations: annotations,
			Labels:      labels,
		},
		Spec: v1alpha1.MachineSpec{},
	}
}

func TestEqualIgnoreHash(t *testing.T) {
	tests := []struct {
		Name           string
		former, latter v1alpha1.MachineTemplateSpec
		expected       bool
	}{
		{
			"Same spec, same labels",
			generateMachineTemplateSpec("foo", "foo-node", map[string]string{}, map[string]string{DefaultMachineDeploymentUniqueLabelKey: "value-1", "something": "else"}),
			generateMachineTemplateSpec("foo", "foo-node", map[string]string{}, map[string]string{DefaultMachineDeploymentUniqueLabelKey: "value-1", "something": "else"}),
			true,
		},
		{
			"Same spec, only machine-template-hash label value is different",
			generateMachineTemplateSpec("foo", "foo-node", map[string]string{}, map[string]string{DefaultMachineDeploymentUniqueLabelKey: "value-1", "something": "else"}),
			generateMachineTemplateSpec("foo", "foo-node", map[string]string{}, map[string]string{DefaultMachineDeploymentUniqueLabelKey: "value-2", "something": "else"}),
			true,
		},
		{
			"Same spec, the former doesn't have machine-template-hash label",
			generateMachineTemplateSpec("foo", "foo-node", map[string]string{}, map[string]string{"something": "else"}),
			generateMachineTemplateSpec("foo", "foo-node", map[string]string{}, map[string]string{DefaultMachineDeploymentUniqueLabelKey: "value-2", "something": "else"}),
			true,
		},
		{
			"Same spec, the label is different, the former doesn't have machine-template-hash label, same number of labels",
			generateMachineTemplateSpec("foo", "foo-node", map[string]string{}, map[string]string{"something": "else"}),
			generateMachineTemplateSpec("foo", "foo-node", map[string]string{}, map[string]string{DefaultMachineDeploymentUniqueLabelKey: "value-2"}),
			false,
		},
		{
			"Same spec, the label is different, the latter doesn't have machine-template-hash label, same number of labels",
			generateMachineTemplateSpec("foo", "foo-node", map[string]string{}, map[string]string{DefaultMachineDeploymentUniqueLabelKey: "value-1"}),
			generateMachineTemplateSpec("foo", "foo-node", map[string]string{}, map[string]string{"something": "else"}),
			false,
		},
		{
			"Same spec, the label is different, and the machine-template-hash label value is the same",
			generateMachineTemplateSpec("foo", "foo-node", map[string]string{}, map[string]string{DefaultMachineDeploymentUniqueLabelKey: "value-1"}),
			generateMachineTemplateSpec("foo", "foo-node", map[string]string{}, map[string]string{DefaultMachineDeploymentUniqueLabelKey: "value-1", "something": "else"}),
			false,
		},
		{
			"Different spec, same labels",
			generateMachineTemplateSpec("foo", "foo-node", map[string]string{"former": "value"}, map[string]string{DefaultMachineDeploymentUniqueLabelKey: "value-1", "something": "else"}),
			generateMachineTemplateSpec("foo", "foo-node", map[string]string{"latter": "value"}, map[string]string{DefaultMachineDeploymentUniqueLabelKey: "value-1", "something": "else"}),
			false,
		},
		{
			"Different spec, different machine-template-hash label value",
			generateMachineTemplateSpec("foo-1", "foo-node", map[string]string{}, map[string]string{DefaultMachineDeploymentUniqueLabelKey: "value-1", "something": "else"}),
			generateMachineTemplateSpec("foo-2", "foo-node", map[string]string{}, map[string]string{DefaultMachineDeploymentUniqueLabelKey: "value-2", "something": "else"}),
			false,
		},
		{
			"Different spec, the former doesn't have machine-template-hash label",
			generateMachineTemplateSpec("foo-1", "foo-node-1", map[string]string{}, map[string]string{"something": "else"}),
			generateMachineTemplateSpec("foo-2", "foo-node-2", map[string]string{}, map[string]string{DefaultMachineDeploymentUniqueLabelKey: "value-2", "something": "else"}),
			false,
		},
		{
			"Different spec, different labels",
			generateMachineTemplateSpec("foo", "foo-node-1", map[string]string{}, map[string]string{"something": "else"}),
			generateMachineTemplateSpec("foo", "foo-node-2", map[string]string{}, map[string]string{"nothing": "else"}),
			false,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runTest := func(t1, t2 *v1alpha1.MachineTemplateSpec, reversed bool) {
				reverseString := ""
				if reversed {
					reverseString = " (reverse order)"
				}
				// Run
				equal := EqualIgnoreHash(t1, t2)
				if equal != test.expected {
					t.Errorf("%q%s: expected %v", test.Name, reverseString, test.expected)
					return
				}
				if t1.Labels == nil || t2.Labels == nil {
					t.Errorf("%q%s: unexpected labels becomes nil", test.Name, reverseString)
				}
			}

			runTest(&test.former, &test.latter, false)
			// Test the same case in reverse order
			runTest(&test.latter, &test.former, true)
		})
	}
}

func TestFindNewMachineSet(t *testing.T) {
	now := metav1.Now()
	later := metav1.Time{Time: now.Add(time.Minute)}

	deployment := generateDeployment("nginx")
	newMS := generateMS(deployment)
	newMS.Labels[DefaultMachineDeploymentUniqueLabelKey] = "hash"
	newMS.CreationTimestamp = later

	newMSDup := generateMS(deployment)
	newMSDup.Labels[DefaultMachineDeploymentUniqueLabelKey] = "different-hash"
	newMSDup.CreationTimestamp = now

	oldDeployment := generateDeployment("nginx")
	oldDeployment.Spec.Template.Spec.Name = "nginx-old-1"
	oldMS := generateMS(oldDeployment)
	oldMS.Status.FullyLabeledReplicas = *(oldMS.Spec.Replicas)

	tests := []struct {
		Name       string
		deployment v1alpha1.MachineDeployment
		msList     []*v1alpha1.MachineSet
		expected   *v1alpha1.MachineSet
	}{
		{
			Name:       "Get new MachineSet with the same template as Deployment spec but different machine-template-hash value",
			deployment: deployment,
			msList:     []*v1alpha1.MachineSet{&newMS, &oldMS},
			expected:   &newMS,
		},
		{
			Name:       "Get the oldest new MachineSet when there are more than one MachineSet with the same template",
			deployment: deployment,
			msList:     []*v1alpha1.MachineSet{&newMS, &oldMS, &newMSDup},
			expected:   &newMSDup,
		},
		{
			Name:       "Get nil new MachineSet",
			deployment: deployment,
			msList:     []*v1alpha1.MachineSet{&oldMS},
			expected:   nil,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			if ms := FindNewMachineSet(&test.deployment, test.msList); !reflect.DeepEqual(ms, test.expected) {
				t.Errorf("In test case %q, expected %#v, got %#v", test.Name, test.expected, ms)
			}
		})
	}
}

func TestFindOldMachineSets(t *testing.T) {
	now := metav1.Now()
	later := metav1.Time{Time: now.Add(time.Minute)}
	before := metav1.Time{Time: now.Add(-time.Minute)}

	deployment := generateDeployment("nginx")
	newMS := generateMS(deployment)
	*(newMS.Spec.Replicas) = 1
	newMS.Labels[DefaultMachineDeploymentUniqueLabelKey] = "hash"
	newMS.CreationTimestamp = later

	newMSDup := generateMS(deployment)
	newMSDup.Labels[DefaultMachineDeploymentUniqueLabelKey] = "different-hash"
	newMSDup.CreationTimestamp = now

	oldDeployment := generateDeployment("nginx")
	oldDeployment.Spec.Template.Spec.Name = "nginx-old-1"
	oldMS := generateMS(oldDeployment)
	oldMS.Status.FullyLabeledReplicas = *(oldMS.Spec.Replicas)
	oldMS.CreationTimestamp = before

	tests := []struct {
		Name            string
		deployment      v1alpha1.MachineDeployment
		msList          []*v1alpha1.MachineSet
		machineList     *v1alpha1.MachineList
		expected        []*v1alpha1.MachineSet
		expectedRequire []*v1alpha1.MachineSet
	}{
		{
			Name:            "Get old MachineSets",
			deployment:      deployment,
			msList:          []*v1alpha1.MachineSet{&newMS, &oldMS},
			expected:        []*v1alpha1.MachineSet{&oldMS},
			expectedRequire: nil,
		},
		{
			Name:            "Get old MachineSets with no new MachineSet",
			deployment:      deployment,
			msList:          []*v1alpha1.MachineSet{&oldMS},
			expected:        []*v1alpha1.MachineSet{&oldMS},
			expectedRequire: nil,
		},
		{
			Name:            "Get old MachineSets with two new MachineSets, only the oldest new MachineSet is seen as new MachineSet",
			deployment:      deployment,
			msList:          []*v1alpha1.MachineSet{&oldMS, &newMS, &newMSDup},
			expected:        []*v1alpha1.MachineSet{&oldMS, &newMS},
			expectedRequire: []*v1alpha1.MachineSet{&newMS},
		},
		{
			Name:            "Get empty old MachineSets",
			deployment:      deployment,
			msList:          []*v1alpha1.MachineSet{&newMS},
			expected:        nil,
			expectedRequire: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			requireMS, allMS := FindOldMachineSets(&test.deployment, test.msList)
			sort.Sort(MachineSetsByCreationTimestamp(allMS))
			sort.Sort(MachineSetsByCreationTimestamp(test.expected))
			if !reflect.DeepEqual(allMS, test.expected) {
				t.Errorf("In test case %q, expected %#v, got %#v", test.Name, test.expected, allMS)
			}
			// MSs are getting filtered correctly by ms.spec.replicas
			if !reflect.DeepEqual(requireMS, test.expectedRequire) {
				t.Errorf("In test case %q, expected %#v, got %#v", test.Name, test.expectedRequire, requireMS)
			}
		})
	}
}

// equal compares the equality of two MachineSet slices regardless of their ordering
func equal(mss1, mss2 []*v1alpha1.MachineSet) bool {
	if reflect.DeepEqual(mss1, mss2) {
		return true
	}
	if mss1 == nil || mss2 == nil || len(mss1) != len(mss2) {
		return false
	}
	count := 0
	for _, ms1 := range mss1 {
		for _, ms2 := range mss2 {
			if reflect.DeepEqual(ms1, ms2) {
				count++
				break
			}
		}
	}
	return count == len(mss1)
}

func TestGetReplicaCountForMachineSets(t *testing.T) {
	ms1 := generateMS(generateDeployment("foo"))
	*(ms1.Spec.Replicas) = 1
	ms1.Status.Replicas = 2
	ms2 := generateMS(generateDeployment("bar"))
	*(ms2.Spec.Replicas) = 2
	ms2.Status.Replicas = 3

	tests := []struct {
		Name           string
		sets           []*v1alpha1.MachineSet
		expectedCount  int32
		expectedActual int32
	}{
		{
			"1:2 Replicas",
			[]*v1alpha1.MachineSet{&ms1},
			1,
			2,
		},
		{
			"3:5 Replicas",
			[]*v1alpha1.MachineSet{&ms1, &ms2},
			3,
			5,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			ms := GetReplicaCountForMachineSets(test.sets)
			if ms != test.expectedCount {
				t.Errorf("In test case %s, expectedCount %+v, got %+v", test.Name, test.expectedCount, ms)
			}
			ms = GetActualReplicaCountForMachineSets(test.sets)
			if ms != test.expectedActual {
				t.Errorf("In test case %s, expectedActual %+v, got %+v", test.Name, test.expectedActual, ms)
			}
		})
	}
}

func TestResolveFenceposts(t *testing.T) {
	tests := []struct {
		maxSurge          string
		maxUnavailable    string
		desired           int32
		expectSurge       int32
		expectUnavailable int32
		expectError       bool
	}{
		{
			maxSurge:          "0%",
			maxUnavailable:    "0%",
			desired:           0,
			expectSurge:       0,
			expectUnavailable: 1,
			expectError:       false,
		},
		{
			maxSurge:          "39%",
			maxUnavailable:    "39%",
			desired:           10,
			expectSurge:       4,
			expectUnavailable: 3,
			expectError:       false,
		},
		{
			maxSurge:          "oops",
			maxUnavailable:    "39%",
			desired:           10,
			expectSurge:       0,
			expectUnavailable: 0,
			expectError:       true,
		},
		{
			maxSurge:          "55%",
			maxUnavailable:    "urg",
			desired:           10,
			expectSurge:       0,
			expectUnavailable: 0,
			expectError:       true,
		},
	}

	for num, test := range tests {
		t.Run("maxSurge="+test.maxSurge, func(t *testing.T) {
			maxSurge := intstr.FromString(test.maxSurge)
			maxUnavail := intstr.FromString(test.maxUnavailable)
			surge, unavail, err := ResolveFenceposts(&maxSurge, &maxUnavail, test.desired)
			if err != nil && !test.expectError {
				t.Errorf("unexpected error %v", err)
			}
			if err == nil && test.expectError {
				t.Error("expected error")
			}
			if surge != test.expectSurge || unavail != test.expectUnavailable {
				t.Errorf("#%v got %v:%v, want %v:%v", num, surge, unavail, test.expectSurge, test.expectUnavailable)
			}
		})
	}
}

func TestNewMSNewReplicas(t *testing.T) {
	tests := []struct {
		Name          string
		strategyType  common.MachineDeploymentStrategyType
		depReplicas   int32
		newMSReplicas int32
		maxSurge      int
		expected      int32
	}{
		{
			"can not scale up - to newMSReplicas",
			common.RollingUpdateMachineDeploymentStrategyType,
			1, 5, 1, 5,
		},
		{
			"scale up - to depReplicas",
			common.RollingUpdateMachineDeploymentStrategyType,
			6, 2, 10, 6,
		},
	}
	newDeployment := generateDeployment("nginx")
	newRC := generateMS(newDeployment)
	rs5 := generateMS(newDeployment)
	*(rs5.Spec.Replicas) = 5

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			*(newDeployment.Spec.Replicas) = test.depReplicas
			newDeployment.Spec.Strategy = v1alpha1.MachineDeploymentStrategy{Type: test.strategyType}
			newDeployment.Spec.Strategy.RollingUpdate = &v1alpha1.MachineRollingUpdateDeployment{
				MaxUnavailable: func(i int) *intstr.IntOrString {
					x := intstr.FromInt(i)
					return &x
				}(1),
				MaxSurge: func(i int) *intstr.IntOrString {
					x := intstr.FromInt(i)
					return &x
				}(test.maxSurge),
			}
			*(newRC.Spec.Replicas) = test.newMSReplicas
			ms, err := NewMSNewReplicas(&newDeployment, []*v1alpha1.MachineSet{&rs5}, &newRC)
			if err != nil {
				t.Errorf("In test case %s, got unexpected error %v", test.Name, err)
			}
			if ms != test.expected {
				t.Errorf("In test case %s, expected %+v, got %+v", test.Name, test.expected, ms)
			}
		})
	}
}

func TestDeploymentComplete(t *testing.T) {
	deployment := func(desired, current, updated, available, maxUnavailable, maxSurge int32) *v1alpha1.MachineDeployment {
		return &v1alpha1.MachineDeployment{
			Spec: v1alpha1.MachineDeploymentSpec{
				Replicas: &desired,
				Strategy: v1alpha1.MachineDeploymentStrategy{
					RollingUpdate: &v1alpha1.MachineRollingUpdateDeployment{
						MaxUnavailable: func(i int) *intstr.IntOrString { x := intstr.FromInt(i); return &x }(int(maxUnavailable)),
						MaxSurge:       func(i int) *intstr.IntOrString { x := intstr.FromInt(i); return &x }(int(maxSurge)),
					},
					Type: common.RollingUpdateMachineDeploymentStrategyType,
				},
			},
			Status: v1alpha1.MachineDeploymentStatus{
				Replicas:          current,
				UpdatedReplicas:   updated,
				AvailableReplicas: available,
			},
		}
	}

	tests := []struct {
		name string

		d *v1alpha1.MachineDeployment

		expected bool
	}{
		{
			name: "not complete: min but not all machines become available",

			d:        deployment(5, 5, 5, 4, 1, 0),
			expected: false,
		},
		{
			name: "not complete: min availability is not honored",

			d:        deployment(5, 5, 5, 3, 1, 0),
			expected: false,
		},
		{
			name: "complete",

			d:        deployment(5, 5, 5, 5, 0, 0),
			expected: true,
		},
		{
			name: "not complete: all machines are available but not updated",

			d:        deployment(5, 5, 4, 5, 0, 0),
			expected: false,
		},
		{
			name: "not complete: still running old machines",

			// old machine set: spec.replicas=1, status.replicas=1, status.availableReplicas=1
			// new machine set: spec.replicas=1, status.replicas=1, status.availableReplicas=0
			d:        deployment(1, 2, 1, 1, 0, 1),
			expected: false,
		},
		{
			name: "not complete: one replica deployment never comes up",

			d:        deployment(1, 1, 1, 0, 1, 1),
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got, exp := DeploymentComplete(test.d, &test.d.Status), test.expected; got != exp {
				t.Errorf("expected complete: %t, got: %t", exp, got)
			}
		})
	}
}

func TestMaxUnavailable(t *testing.T) {
	deployment := func(replicas int32, maxUnavailable intstr.IntOrString) v1alpha1.MachineDeployment {
		return v1alpha1.MachineDeployment{
			Spec: v1alpha1.MachineDeploymentSpec{
				Replicas: func(i int32) *int32 { return &i }(replicas),
				Strategy: v1alpha1.MachineDeploymentStrategy{
					RollingUpdate: &v1alpha1.MachineRollingUpdateDeployment{
						MaxSurge:       func(i int) *intstr.IntOrString { x := intstr.FromInt(i); return &x }(int(1)),
						MaxUnavailable: &maxUnavailable,
					},
					Type: common.RollingUpdateMachineDeploymentStrategyType,
				},
			},
		}
	}
	tests := []struct {
		name       string
		deployment v1alpha1.MachineDeployment
		expected   int32
	}{
		{
			name:       "maxUnavailable less than replicas",
			deployment: deployment(10, intstr.FromInt(5)),
			expected:   int32(5),
		},
		{
			name:       "maxUnavailable equal replicas",
			deployment: deployment(10, intstr.FromInt(10)),
			expected:   int32(10),
		},
		{
			name:       "maxUnavailable greater than replicas",
			deployment: deployment(5, intstr.FromInt(10)),
			expected:   int32(5),
		},
		{
			name:       "maxUnavailable with replicas is 0",
			deployment: deployment(0, intstr.FromInt(10)),
			expected:   int32(0),
		},
		{
			name:       "maxUnavailable less than replicas with percents",
			deployment: deployment(10, intstr.FromString("50%")),
			expected:   int32(5),
		},
		{
			name:       "maxUnavailable equal replicas with percents",
			deployment: deployment(10, intstr.FromString("100%")),
			expected:   int32(10),
		},
		{
			name:       "maxUnavailable greater than replicas with percents",
			deployment: deployment(5, intstr.FromString("100%")),
			expected:   int32(5),
		},
	}

	for _, test := range tests {
		t.Log(test.name)
		t.Run(test.name, func(t *testing.T) {
			maxUnavailable := MaxUnavailable(test.deployment)
			if test.expected != maxUnavailable {
				t.Fatalf("expected:%v, got:%v", test.expected, maxUnavailable)
			}
		})
	}
}

//Set of simple tests for annotation related util functions
func TestAnnotationUtils(t *testing.T) {

	//Setup
	tDeployment := generateDeployment("nginx")
	tMS := generateMS(tDeployment)
	tDeployment.Annotations[RevisionAnnotation] = "1"

	//Test Case 1: Check if anotations are copied properly from deployment to MS
	t.Run("SetNewMachineSetAnnotations", func(t *testing.T) {
		//Try to set the increment revision from 1 through 20
		for i := 0; i < 20; i++ {

			nextRevision := fmt.Sprintf("%d", i+1)
			SetNewMachineSetAnnotations(&tDeployment, &tMS, nextRevision, true)
			//Now the MachineSets Revision Annotation should be i+1

			if tMS.Annotations[RevisionAnnotation] != nextRevision {
				t.Errorf("Revision Expected=%s Obtained=%s", nextRevision, tMS.Annotations[RevisionAnnotation])
			}
		}
	})

	//Test Case 2:  Check if annotations are set properly
	t.Run("SetReplicasAnnotations", func(t *testing.T) {
		updated := SetReplicasAnnotations(&tMS, 10, 11)
		if !updated {
			t.Errorf("SetReplicasAnnotations() failed")
		}
		value, ok := tMS.Annotations[DesiredReplicasAnnotation]
		if !ok {
			t.Errorf("SetReplicasAnnotations did not set DesiredReplicasAnnotation")
		}
		if value != "10" {
			t.Errorf("SetReplicasAnnotations did not set DesiredReplicasAnnotation correctly value=%s", value)
		}
		if value, ok = tMS.Annotations[MaxReplicasAnnotation]; !ok {
			t.Errorf("SetReplicasAnnotations did not set DesiredReplicasAnnotation")
		}
		if value != "11" {
			t.Errorf("SetReplicasAnnotations did not set MaxReplicasAnnotation correctly value=%s", value)
		}
	})

	//Test Case 3:  Check if annotations reflect deployments state
	tMS.Annotations[DesiredReplicasAnnotation] = "1"
	tMS.Status.AvailableReplicas = 1
	tMS.Spec.Replicas = new(int32)
	*tMS.Spec.Replicas = 1

	t.Run("IsSaturated", func(t *testing.T) {
		saturated := IsSaturated(&tDeployment, &tMS)
		if !saturated {
			t.Errorf("SetReplicasAnnotations Expected=true Obtained=false")
		}
	})
	//Tear Down
}

func TestReplicasAnnotationsNeedUpdate(t *testing.T) {

	desiredReplicas := fmt.Sprintf("%d", int32(10))
	maxReplicas := fmt.Sprintf("%d", int32(20))

	tests := []struct {
		name       string
		machineSet *v1alpha1.MachineSet
		expected   bool
	}{
		{
			name: "test Annotations nil",
			machineSet: &v1alpha1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{Name: "hello", Namespace: "test"},
				Spec: v1alpha1.MachineSetSpec{
					Selector: metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
				},
			},
			expected: true,
		},
		{
			name: "test desiredReplicas update",
			machineSet: &v1alpha1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "hello",
					Namespace:   "test",
					Annotations: map[string]string{DesiredReplicasAnnotation: "8", MaxReplicasAnnotation: maxReplicas},
				},
				Spec: v1alpha1.MachineSetSpec{
					Selector: metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
				},
			},
			expected: true,
		},
		{
			name: "test maxReplicas update",
			machineSet: &v1alpha1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "hello",
					Namespace:   "test",
					Annotations: map[string]string{DesiredReplicasAnnotation: desiredReplicas, MaxReplicasAnnotation: "16"},
				},
				Spec: v1alpha1.MachineSetSpec{
					Selector: metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
				},
			},
			expected: true,
		},
		{
			name: "test needn't update",
			machineSet: &v1alpha1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "hello",
					Namespace:   "test",
					Annotations: map[string]string{DesiredReplicasAnnotation: desiredReplicas, MaxReplicasAnnotation: maxReplicas},
				},
				Spec: v1alpha1.MachineSetSpec{
					Selector: metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
				},
			},
			expected: false,
		},
	}

	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := ReplicasAnnotationsNeedUpdate(test.machineSet, 10, 20)
			if result != test.expected {
				t.Errorf("case[%d]:%s Expected %v, Got: %v", i, test.name, test.expected, result)
			}
		})
	}
}
